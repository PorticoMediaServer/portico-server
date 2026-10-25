package metadata

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/imagework"
)

// AdoptReadingCover accepts an image from an already admitted Reading source.
// Its source guard is mandatory in the publishing transaction. Automatic
// discovery only fills an absent cover and never replaces an owner's choice.
func (s *Service) AdoptReadingCover(ctx context.Context, item, sourceRevision string, raw []byte, authorize func(*sql.Tx) error) error {
	if authorize == nil || len(sourceRevision) != 64 || item == "" {
		return ErrRepairInput
	}
	if s == nil || s.cacheRoot == "" {
		return ErrArtworkPending
	}
	if len(raw) == 0 || len(raw) > UploadBytes || sniffArtworkUpload(raw) == "" {
		return ErrArtworkUpload
	}
	ctx, checkPolicy, err := s.CaptureArtworkContext(ctx)
	if err != nil {
		return err
	}
	original, thumb, w, h, err := normalizeArtworkFormatsContext(ctx, raw, map[string]bool{"jpeg": true, "png": true, "webp": true})
	if err != nil {
		if errors.Is(err, imagework.ErrBusy) || ctx.Err() != nil {
			return err
		}
		return ErrArtworkUpload
	}
	full, err := s.installArtworkContext(ctx, original, w, h)
	if err != nil {
		return err
	}
	shape, _, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		return ErrArtworkUpload
	}
	small, err := s.installArtworkContext(ctx, thumb, shape.Width, shape.Height)
	if err != nil {
		return err
	}
	gate, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer gate.Rollback()
	tx := gate.Tx()
	if err = authorize(tx); err != nil {
		return err
	}
	if err = checkPolicy(ctx, tx); err != nil {
		return err
	}
	target := RepairTarget{Kind: "item", ID: item}
	entity, err := artworkEntity(ctx, tx, item)
	if err != nil {
		return err
	}
	var eligible, selected bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id JOIN libraries lib ON lib.id=cl.library_id WHERE e.id=? AND e.retired=0 AND cl.retired=0 AND ((e.kind=20 AND lib.kind='book') OR (e.kind=21 AND lib.kind='comic'))),EXISTS(SELECT 1 FROM artwork_selections WHERE kind='item' AND entity_id=? AND role='cover' AND subject='')`, entity, entity).Scan(&eligible, &selected); err != nil {
		return err
	}
	if !eligible {
		return sql.ErrNoRows
	}
	if selected {
		return nil
	}
	fence, err := artworkFence(ctx, tx, target)
	if err != nil {
		return err
	}
	stamp := s.publicationTime().Format(time.RFC3339Nano)
	for _, object := range []artworkInstalled{full, small} {
		if err = recordArtworkObject(ctx, tx, object, stamp); err != nil {
			return err
		}
	}
	if err = linkArtworkBuckets(ctx, tx, full, small); err != nil {
		return err
	}
	candidate, err := insertArtworkCandidate(ctx, tx, target, "cover", "", "local", full.digest, "reading:"+sourceRevision, "", "Artwork from the publication", fence, stamp, 100)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO artwork_previews VALUES(?,?,?,?) ON CONFLICT(candidate_id) DO UPDATE SET digest=excluded.digest,source_fence=excluded.source_fence,created_at=excluded.created_at`, candidate, small.digest, fence, stamp); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,thumbnail_digest,locked,revision,actor,observed_at) VALUES('item',?,'cover','',?,?,?,0,1,'automatic',?) ON CONFLICT(kind,entity_id,role,subject) DO NOTHING`, entity, candidate, full.digest, small.digest, stamp); err != nil {
		return err
	}
	if err = projectSelectedArtwork(ctx, tx, target, "cover"); err != nil {
		return err
	}
	if err = authorize(tx); err != nil {
		return err
	}
	if err = gate.Commit(); err != nil {
		return err
	}
	return s.ensureArtworkFiles(full, small)
}
