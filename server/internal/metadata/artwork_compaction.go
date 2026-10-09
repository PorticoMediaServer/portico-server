package metadata

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"io"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/imagework"
)

// CompactArtworkStep upgrades one legacy object without provider IO. Selected,
// uploaded and restorable artwork keep their identity/locks; only their bounded
// representation changes. Publication is atomic and old open readers remain valid.
func (s *Service) CompactArtworkStep(ctx context.Context) error {
	if s.cacheRoot == "" {
		return nil
	}
	var old string
	// The literals match the partial index artwork_objects_compaction (0103):
	// the step reads only candidates, never the whole object table.
	err := s.db.QueryRowContext(ctx, `SELECT o.digest FROM artwork_objects o INDEXED BY artwork_objects_compaction WHERE o.status='ready' AND (o.bytes>500000 OR o.width>1920 OR o.height>1920) AND NOT EXISTS(SELECT 1 FROM artwork_compaction_failures f WHERE f.digest=o.digest) ORDER BY o.bytes DESC,o.digest LIMIT 1`).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = s.compactArtworkObject(ctx, old); err != nil && ctx.Err() == nil {
		var bad *artworkCompactionFailure
		if errors.As(err, &bad) {
			// Record it and move on: one undecodable file must not hold the
			// head of the queue forever.
			_, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `INSERT INTO artwork_compaction_failures VALUES(?,?,?) ON CONFLICT(digest) DO UPDATE SET reason=excluded.reason,failed_at=excluded.failed_at`, old, bad.reason, s.publicationTime().Format(time.RFC3339Nano))
		}
	}
	return err
}

type artworkCompactionFailure struct{ reason string }

func (e *artworkCompactionFailure) Error() string { return "artwork compaction: " + e.reason }

func (s *Service) compactArtworkObject(ctx context.Context, old string) error {
	fail := func(reason string) error { return &artworkCompactionFailure{reason} }
	f, err := s.openArtworkObject(old)
	if err != nil {
		return fail("file_unreadable")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	f.Close()
	if err != nil {
		return fail("file_unreadable")
	}
	if len(raw) > 32<<20 {
		return fail("decode_bound")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return fail("undecodable")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 10000 || cfg.Height > 10000 || int64(cfg.Width)*int64(cfg.Height) > 24000000 {
		return fail("dimension_limit")
	}
	release, err := imagework.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	decoded, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return fail("undecodable")
	}
	var small bool
	err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_selections WHERE thumbnail_digest=?) OR EXISTS(SELECT 1 FROM artwork_previews WHERE digest=?) OR EXISTS(SELECT 1 FROM artwork_uploads WHERE thumbnail_digest=?)`, old, old, old).Scan(&small)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	edge := artworkLargeEdge
	if small {
		edge = artworkSmallEdge
	}
	data, w, h, err := encodeDisplayArtworkContext(ctx, decoded, edge, format == "jpeg")
	if err != nil {
		return err
	}
	release()
	installed, err := s.installArtworkContext(ctx, data, w, h)
	if err != nil {
		return err
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer gated.Rollback()
	tx := gated.Tx()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_objects WHERE digest=? AND status='ready')`, old).Scan(&exists); err != nil || !exists {
		return err
	}
	now := s.publicationTime().Format(time.RFC3339Nano)
	if err = recordArtworkObject(ctx, tx, installed, now); err != nil {
		return err
	}
	// Closed registry: every relational reference and both undo snapshots must
	// switch in the same transaction. Unknown future FK references fail closed.
	for _, ref := range []struct{ table, column string }{
		{"artwork_selections", "digest"}, {"artwork_selections", "thumbnail_digest"},
		{"artwork_previews", "digest"}, {"artwork_uploads", "digest"}, {"artwork_uploads", "thumbnail_digest"},
	} {
		suffix := ""
		if ref.table == "artwork_selections" {
			suffix = ",revision=revision+1"
		}
		if _, err = tx.ExecContext(ctx, `UPDATE `+ref.table+` SET `+ref.column+`=?`+suffix+` WHERE `+ref.column+`=?`, installed.digest, old); err != nil {
			return err
		}
	}
	for _, column := range []string{"before_json", "after_json"} {
		if _, err = tx.ExecContext(ctx, `UPDATE metadata_owner_history SET `+column+`=replace(`+column+`,?,?) WHERE instr(`+column+`,?)>0`, `"`+old+`"`, `"`+installed.digest+`"`, `"`+old+`"`); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM artwork_objects WHERE digest=?`, old); err != nil {
		return err
	}
	if err = gated.Commit(); err != nil {
		return err
	}
	if err = s.ensureArtworkFiles(installed); err != nil {
		return err
	}
	// A crash here leaves an unreferenced immutable file; normal GC removes it.
	// Windows may defer unlink until an old reader closes. Never truncate in place.
	_, err = s.retireArtworkFile(ctx, old, func(ctx context.Context) (bool, error) {
		var known bool
		err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_objects WHERE digest=?)`, old).Scan(&known)
		return !known, err
	})
	return err
}
