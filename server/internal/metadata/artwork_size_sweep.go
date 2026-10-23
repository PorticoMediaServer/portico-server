package metadata

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
)

// GlobalArtworkSizeStep records at most 32 selected pictures per pass, then
// admits at most 32 retained refresh intents. It never fetches inside a write
// transaction. The ordinary artwork worker rechecks consent, source and the
// exact selection revision before publishing bytes.
func (s *Service) GlobalArtworkSizeStep(ctx context.Context) error {
	w, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer w.Rollback()
	tx := w.Tx()
	var edge int
	if err = tx.QueryRowContext(ctx, `SELECT artwork_max_edge FROM metadata_runtime_settings WHERE singleton=1`).Scan(&edge); err != nil {
		return err
	}
	var running bool
	var kind, role, subject string
	var entity int64
	if err = tx.QueryRowContext(ctx, `SELECT running,cursor_kind,cursor_id,cursor_role,cursor_subject FROM metadata_artwork_size_sweep WHERE singleton=1`).Scan(&running, &kind, &entity, &role, &subject); err != nil {
		return err
	}
	type request struct {
		kind, role, subject, candidate, digest, provider string
		entity, revision                                 int64
		edge                                             int
	}
	if running {
		rows, e := tx.QueryContext(ctx, `SELECT a.kind,a.entity_id,a.role,a.subject,a.candidate_id,a.digest,a.revision,c.provider FROM artwork_selections a JOIN artwork_candidates c ON c.id=a.candidate_id WHERE (a.kind,a.entity_id,a.role,a.subject)>(?,?,?,?) ORDER BY a.kind,a.entity_id,a.role,a.subject LIMIT 32`, kind, entity, role, subject)
		if e != nil {
			return e
		}
		batch := []request{}
		for rows.Next() {
			var r request
			if err = rows.Scan(&r.kind, &r.entity, &r.role, &r.subject, &r.candidate, &r.digest, &r.revision, &r.provider); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, r := range batch {
			kind, entity, role, subject = r.kind, r.entity, r.role, r.subject
			if r.provider == "upload" {
				continue
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO metadata_artwork_size_requests(kind,entity_id,role,subject,candidate_id,digest,selection_revision,edge) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(kind,entity_id,role,subject) DO UPDATE SET candidate_id=excluded.candidate_id,digest=excluded.digest,selection_revision=excluded.selection_revision,edge=excluded.edge,next_attempt=''`, r.kind, r.entity, r.role, r.subject, r.candidate, r.digest, r.revision, edge); err != nil {
				return err
			}
			kind, entity, role, subject = r.kind, r.entity, r.role, r.subject
		}
		if _, err = tx.ExecContext(ctx, `UPDATE metadata_artwork_size_sweep SET running=?,cursor_kind=?,cursor_id=?,cursor_role=?,cursor_subject=? WHERE singleton=1`, len(batch) == 32, kind, entity, role, subject); err != nil {
			return err
		}
	}
	stamp := s.publicationTime().Format(time.RFC3339Nano)
	rows, err := tx.QueryContext(ctx, `SELECT kind,entity_id,role,subject,candidate_id,digest,selection_revision,edge FROM metadata_artwork_size_requests INDEXED BY metadata_artwork_size_requests_due WHERE next_attempt<=? ORDER BY next_attempt,kind,entity_id,role,subject LIMIT 32`, stamp)
	if err != nil {
		return err
	}
	requests := []request{}
	for rows.Next() {
		var r request
		if err = rows.Scan(&r.kind, &r.entity, &r.role, &r.subject, &r.candidate, &r.digest, &r.revision, &r.edge); err != nil {
			rows.Close()
			return err
		}
		requests = append(requests, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range requests {
		remove := func() error {
			_, e := tx.ExecContext(ctx, `DELETE FROM metadata_artwork_size_requests WHERE kind=? AND entity_id=? AND role=? AND subject=?`, r.kind, r.entity, r.role, r.subject)
			return e
		}
		deferUntil := func(delay time.Duration) error {
			_, e := tx.ExecContext(ctx, `UPDATE metadata_artwork_size_requests SET next_attempt=? WHERE kind=? AND entity_id=? AND role=? AND subject=?`, s.publicationTime().Add(delay).Format(time.RFC3339Nano), r.kind, r.entity, r.role, r.subject)
			return e
		}
		var candidate, digest, actor, provider string
		var revision int64
		var locked bool
		var width, height int
		err = tx.QueryRowContext(ctx, `SELECT a.candidate_id,a.digest,a.revision,a.locked,a.actor,c.provider,o.width,o.height FROM artwork_selections a JOIN artwork_candidates c ON c.id=a.candidate_id JOIN artwork_objects o ON o.digest=a.digest WHERE a.kind=? AND a.entity_id=? AND a.role=? AND a.subject=?`, r.kind, r.entity, r.role, r.subject).Scan(&candidate, &digest, &revision, &locked, &actor, &provider, &width, &height)
		if errors.Is(err, sql.ErrNoRows) || err == nil && (r.edge != edge || candidate != r.candidate || digest != r.digest || revision != r.revision || max(width, height) >= edge || provider == "upload") {
			if err = remove(); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		public, e := entityid.Public(ctx, tx, r.entity)
		if errors.Is(e, entityid.ErrNotFound) {
			if err = remove(); err != nil {
				return err
			}
			continue
		}
		if e != nil {
			return e
		}
		target := RepairTarget{Kind: r.kind, ID: public}
		var busy bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_jobs WHERE kind=? AND entity_id=? AND role=? AND subject=? AND preview=0 AND status IN('pending','running','retry'))`, r.kind, r.entity, r.role, r.subject).Scan(&busy); err != nil {
			return err
		}
		if busy {
			if err = deferUntil(time.Minute); err != nil {
				return err
			}
			continue
		}
		enabled, e := s.artworkJobEnabled(ctx, tx, artworkWork{target: target, role: r.role, provider: provider, actor: actor})
		if errors.Is(e, sql.ErrNoRows) {
			if err = remove(); err != nil {
				return err
			}
			continue
		}
		if e != nil {
			return e
		}
		if !enabled {
			if err = deferUntil(5 * time.Minute); err != nil {
				return err
			}
			continue
		}
		fence, e := artworkFence(ctx, tx, target)
		if errors.Is(e, sql.ErrNoRows) {
			if err = remove(); err != nil {
				return err
			}
			continue
		}
		if e != nil {
			return e
		}
		// Refresh exactly the retained choice; never choose another candidate
		// or manufacture a new owner selection while changing its dimensions.
		if _, err = tx.ExecContext(ctx, `UPDATE artwork_candidates SET source_fence=? WHERE id=?`, fence, candidate); err != nil {
			return err
		}
		if err = queueArtworkCandidate(ctx, tx, target, candidate, actor, stamp, locked, true); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE artwork_jobs SET size_refresh=1 WHERE kind=? AND entity_id=? AND role=? AND subject=? AND preview=0`, r.kind, r.entity, r.role, r.subject); err != nil {
			return err
		}
		// Keep intent until a new selection revision confirms publication.
		// Consent withdrawn after queueing must not lose the later catch-up.
		if err = deferUntil(time.Minute); err != nil {
			return err
		}
	}
	return w.Commit()
}
