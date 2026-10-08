package metadata

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// One optional preview per target/role/subject can be in flight. Originals are
// never retained for browsing; the same bounded decoder publishes thumbnails.
func queueArtworkPreview(ctx context.Context, tx *sql.Tx, t RepairTarget, candidate, actor, now string) error {
	entity, err := artworkEntity(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	var role, subject, origin, provider, fence string
	if err := tx.QueryRowContext(ctx, `SELECT role,subject,origin,provider,source_fence FROM artwork_candidates WHERE id=? AND kind=? AND entity_id=?`, candidate, t.Kind, entity).Scan(&role, &subject, &origin, &provider, &fence); err != nil {
		return err
	}
	current, err := artworkFence(ctx, tx, t)
	if err != nil {
		return err
	}
	if current != fence {
		return ErrRepairConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO artwork_jobs(id,kind,entity_id,role,subject,candidate_id,origin,provider,source_fence,actor,created_at,preview) VALUES(?,?,?,?,?,?,?,?,?,?,?,1) ON CONFLICT(kind,entity_id,role,subject,preview) DO UPDATE SET id=excluded.id,candidate_id=excluded.candidate_id,origin=excluded.origin,provider=excluded.provider,source_fence=excluded.source_fence,status='pending',attempts=0,next_attempt='',lease='',lease_until='',error='',actor=excluded.actor,created_at=excluded.created_at`, identity.Token(), t.Kind, entity, role, subject, candidate, origin, provider, fence, actor, now)
	return err
}

// Caller authenticates a current owner. This route only opens installed bytes.
// width is one of the display buckets (400, 800, 1920): a preview larger than
// the bucket is reduced with the same resizer the variant path uses, after the
// read snapshot is released, so no lock is held across the resize that is not
// already held today. Previews are thumbnails already, so the resize only fires
// for oversize rows and never upscales.
func (s *Service) PreviewArtwork(ctx context.Context, t RepairTarget, candidate string, width int) (*os.File, string, error) {
	if !artDigest(candidate) {
		return nil, "", ErrRepairInput
	}
	if width != 400 && width != 800 && width != 1920 {
		return nil, "", ErrRepairInput
	}
	gated, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return nil, "", err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	current, err := artworkFence(ctx, tx, t)
	if err != nil {
		return nil, "", err
	}
	entity, err := artworkEntity(ctx, tx, t.ID)
	if err != nil {
		return nil, "", err
	}
	var digest string
	if err = tx.QueryRowContext(ctx, `SELECT p.digest FROM artwork_previews p JOIN artwork_candidates c ON c.id=p.candidate_id JOIN artwork_objects o ON o.digest=p.digest AND o.status='ready' WHERE c.kind=? AND c.entity_id=? AND c.id=? AND p.source_fence=?`, t.Kind, entity, candidate, current).Scan(&digest); err != nil {
		return nil, "", ErrArtworkPending
	}
	if !artDigest(digest) {
		return nil, "", ErrRepairInput
	}
	var haveW, haveH int
	if err = tx.QueryRowContext(ctx, `SELECT width,height FROM artwork_objects WHERE digest=? AND status='ready'`, digest).Scan(&haveW, &haveH); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, "", err
		}
		haveW, haveH = 0, 0
	}
	if haveW > width || haveH > width {
		if err = gated.Commit(); err != nil {
			return nil, "", err
		}
		if err = s.ensureArtworkVariant(ctx, digest, width, ""); err != nil {
			return nil, "", err
		}
		return s.openPreviewVariant(ctx, digest, width)
	}
	f, err := s.openArtworkObject(digest)
	if err != nil {
		return nil, "", ErrArtworkPending
	}
	if err = gated.Commit(); err != nil {
		f.Close()
		return nil, "", err
	}
	return f, "image/png", nil
}

// openPreviewVariant opens a resized preview bucket published by
// ensureArtworkVariant. A missing row or file is a pending preview, never a
// different candidate's bytes.
func (s *Service) openPreviewVariant(ctx context.Context, source string, width int) (*os.File, string, error) {
	var digest, mime string
	if err := s.db.QueryRowContext(ctx, `SELECT o.digest,o.mime FROM artwork_variants v JOIN artwork_objects o ON o.digest=v.digest AND o.status='ready' WHERE v.source_digest=? AND v.width=?`, source, width).Scan(&digest, &mime); err != nil {
		return nil, "", ErrArtworkPending
	}
	if !artDigest(digest) {
		return nil, "", ErrRepairInput
	}
	f, err := s.openArtworkObject(digest)
	if err != nil {
		return nil, "", ErrArtworkPending
	}
	return f, mime, nil
}
