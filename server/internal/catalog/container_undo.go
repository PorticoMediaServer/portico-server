package catalog

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/personalstate"
)

// A container action is atomic and bounded; an oversized selector must be split
// into seasons. The journal expires quickly and holds only watched/resume state.
const containerActionMaximum = 5000
const containerUndoLifetime = 30 * time.Second

type ContainerUndoMutation struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	UndoToken        string `json:"undoToken"`
}

func containerMemberIDs(ctx context.Context, tx *sql.Tx, members string, container int64) ([]int64, error) {
	rows, e := tx.QueryContext(ctx, `SELECT entity_id FROM (`+members+`) ORDER BY entity_id LIMIT ?`, container, containerActionMaximum+1)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(ids) > containerActionMaximum {
		return nil, jobInvalid("mark smaller containers separately")
	}
	return ids, nil
}

func captureContainerUndo(ctx context.Context, tx *sql.Tx, profile, kind string, container int64, previous ContainerPersonal, clear string, out *ContainerPersonal, ids []int64) error {
	// One live receipt per container; expired snapshots are removed on subsequent
	// actions. Bound each cleanup so a busy household cannot monopolize a write.
	if _, e := tx.ExecContext(ctx, `DELETE FROM container_personal_undo WHERE token IN(SELECT token FROM container_personal_undo WHERE expires_ms<? LIMIT 64) OR (profile_id=? AND kind=? AND container_id=?)`, time.Now().UnixMilli(), profile, kind, container); e != nil {
		return e
	}
	until := time.Now().UTC().Add(containerUndoLifetime)
	out.UndoUntil = until.Format("2006-01-02T15:04:05.000Z")
	if _, e := tx.ExecContext(ctx, `INSERT INTO container_personal_undo(token,profile_id,kind,container_id,revision,previous_watched,previous_watermark,previous_clear,watermark,expires_ms) VALUES(?,?,?,?,?,?,?,?,?,?)`, out.UndoToken, profile, kind, container, out.Revision, previous.Watched, previous.Watermark, clear, out.Watermark, until.UnixMilli()); e != nil {
		return e
	}
	for _, id := range ids {
		if _, e := tx.ExecContext(ctx, `INSERT INTO container_personal_undo_items(token,item_id,previous_watched,after_revision,position,unit,playback_id,activity_at,activity_state,book_position)
 SELECT ?,e.id,COALESCE(`+personalstate.SQL("?", "e.id")+`,0),COALESCE(pi.revision,0)+1,p.position,p.unit,p.playback_id,a.updated_at,a.state,
 (SELECT b.position FROM book_resume b WHERE b.profile_id=? AND b.item_id=e.id)
 FROM catalog_entities e LEFT JOIN personal_items pi ON pi.profile_id=? AND pi.item_id=e.id
 LEFT JOIN progress p ON p.profile_id=? AND p.item_id=e.id
 LEFT JOIN progress_activity a ON a.profile_id=? AND a.item_id=e.id WHERE e.id=?`, out.UndoToken, profile, profile, profile, profile, profile, id); e != nil {
			return e
		}
	}
	return nil
}

func invalidateContainer(ctx context.Context, tx *sql.Tx, profile string, container int64) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO viewer_revisions(profile_id,library_id,revision) SELECT ?,cl.library_id,1 FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.id=? ON CONFLICT(profile_id,library_id) DO UPDATE SET revision=revision+1`, profile, container)
	return e
}

func (s *Service) UndoContainerPersonal(ctx context.Context, p identity.Principal, kind, id string, m ContainerUndoMutation, access BulkAccess) (out ContainerPersonal, err error) {
	if !personalOperation.MatchString(m.UndoToken) || m.ExpectedRevision < 1 || m.ExpectedRevision > 9007199254740991 {
		return out, jobInvalid("invalid container undo")
	}
	members, e := containerMembers(kind)
	if e != nil {
		return out, e
	}
	profile := identity.PersonalKey(p.Viewer)
	err = dbwork.WithWriteTx(ctx, s.db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		container, e := entityid.Resolve(ctx, tx, id)
		if errors.Is(e, entityid.ErrNotFound) {
			return sql.ErrNoRows
		}
		if e != nil {
			return e
		}
		visible, bound, e := access(ctx, tx, p, "i.id")
		if e != nil {
			return e
		}
		var present int
		if e = tx.QueryRowContext(ctx, `SELECT 1 FROM (`+members+`) m JOIN catalog_entities i ON i.id=m.entity_id WHERE `+visible+` LIMIT 1`, append([]any{container}, bound...)...).Scan(&present); e != nil {
			return e
		}
		var previousWatermark, clear, mark string
		var expires, revision int64
		e = tx.QueryRowContext(ctx, `SELECT u.previous_watched,u.previous_watermark,u.previous_clear,u.watermark,u.expires_ms,c.revision FROM container_personal_undo u JOIN container_personal_state c ON c.profile_id=u.profile_id AND c.kind=u.kind AND c.container_id=u.container_id AND c.revision=u.revision WHERE u.token=? AND u.profile_id=? AND u.kind=? AND u.container_id=?`, m.UndoToken, profile, kind, container).Scan(&out.Watched, &previousWatermark, &clear, &mark, &expires, &revision)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrPersonalConflict
		}
		if e != nil {
			return e
		}
		if expires < time.Now().UnixMilli() {
			return ErrOperationExpired
		}
		if revision != m.ExpectedRevision {
			return ErrPersonalConflict
		}
		ids, e := containerMemberIDs(ctx, tx, members, container)
		if e != nil {
			return e
		}
		// Refuse to overwrite later item actions, playback, nested container marks,
		// or changed membership. Restoring only some members would not be a true Undo.
		var changed int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM container_personal_undo_items u LEFT JOIN personal_items pi ON pi.profile_id=? AND pi.item_id=u.item_id LEFT JOIN progress p ON p.profile_id=? AND p.item_id=u.item_id LEFT JOIN progress_activity a ON a.profile_id=? AND a.item_id=u.item_id WHERE u.token=? AND (pi.revision IS NOT u.after_revision OR p.position IS NOT CASE WHEN u.position IS NULL THEN NULL ELSE 0 END OR p.unit IS NOT u.unit OR p.playback_id IS NOT u.playback_id OR a.updated_at IS NOT u.activity_at OR a.state IS NOT CASE WHEN u.activity_state IS NULL THEN NULL ELSE 'ended' END OR EXISTS(SELECT 1 FROM personal_watched_intents wi WHERE wi.profile_id=? AND wi.item_id=u.item_id AND wi.authored_at>?))`, profile, profile, profile, m.UndoToken, profile, mark).Scan(&changed); e != nil {
			return e
		}
		var count int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM container_personal_undo_items WHERE token=?`, m.UndoToken).Scan(&count); e != nil {
			return e
		}
		if changed != 0 || count != len(ids) {
			return ErrPersonalConflict
		}
		for _, item := range ids {
			var saved bool
			if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM container_personal_undo_items WHERE token=? AND item_id=?)`, m.UndoToken, item).Scan(&saved); e != nil {
				return e
			}
			if !saved {
				return ErrPersonalConflict
			}
			// An action on an overlapping container also changes the effective state.
			var nested int
			if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM container_personal_state c WHERE c.profile_id=? AND c.watermark>? AND ((c.kind='show' AND c.container_id=(SELECT show_id FROM catalog_episodes WHERE entity_id=?)) OR (c.kind='season' AND c.container_id=(SELECT season_id FROM catalog_episodes WHERE entity_id=?)) OR (c.kind='album' AND c.container_id=(SELECT album_id FROM catalog_songs WHERE entity_id=?)) OR (c.kind='book' AND c.container_id=(SELECT book_id FROM catalog_book_files WHERE entity_id=?)))`, profile, mark, item, item, item, item).Scan(&nested); e != nil {
				return e
			}
			if nested != 0 {
				return ErrPersonalConflict
			}
		}
		out.Revision = revision + 1
		out.Watermark = previousWatermark
		if _, e = tx.ExecContext(ctx, `UPDATE container_personal_state SET watched=?,watermark=?,cleared_through=?,revision=? WHERE profile_id=? AND kind=? AND container_id=?`, out.Watched, previousWatermark, clear, out.Revision, profile, kind, container); e != nil {
			return e
		}
		for _, item := range ids {
			before, e := readPersonalByID(tx, profile, item, personalstate.SQL("?", "?"))
			if e != nil {
				return e
			}
			if e = snapshotPersonalByID(tx, profile, item, before); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE personal_items SET watched=(SELECT previous_watched FROM container_personal_undo_items WHERE token=? AND item_id=?),revision=revision+1 WHERE profile_id=? AND item_id=?`, m.UndoToken, item, profile, item); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE progress SET position=(SELECT position FROM container_personal_undo_items WHERE token=? AND item_id=?) WHERE profile_id=? AND item_id=?`, m.UndoToken, item, profile, item); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE progress_activity SET state=(SELECT activity_state FROM container_personal_undo_items WHERE token=? AND item_id=?) WHERE profile_id=? AND item_id=?`, m.UndoToken, item, profile, item); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE book_resume SET position=(SELECT book_position FROM container_personal_undo_items WHERE token=? AND item_id=?) WHERE profile_id=? AND item_id=? AND EXISTS(SELECT 1 FROM container_personal_undo_items WHERE token=? AND item_id=? AND book_position IS NOT NULL)`, m.UndoToken, item, profile, item, m.UndoToken, item); e != nil {
				return e
			}
			restored, e := readPersonalByID(tx, profile, item, personalstate.SQL("?", "?"))
			if e != nil {
				return e
			}
			if e = snapshotPersonalByID(tx, profile, item, restored); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM container_personal_undo WHERE token=?`, m.UndoToken); e != nil {
			return e
		}
		if e = invalidateContainer(ctx, tx, profile, container); e != nil {
			return e
		}
		out.Watchlisted, out.Favorite, e = readContainerSaved(ctx, tx, profile, container)
		return e
	})
	return
}
