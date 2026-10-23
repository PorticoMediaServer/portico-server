package remotesources

import (
	"context"
	"encoding/json"
	"path/filepath"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

// buildServeSnapshot adopts the complete-body listing fallback. The existing
// private snapshot tables and bounded keyset reader remain the publication
// boundary. A failed parse never changes ready or commits partial child/deletion
// evidence. Callers retain source/process admission across this entire producer.
func (s *Service) buildServeSnapshot(ctx context.Context, endpoint *remotemedia.ServeEndpoint, b binding, relative, id, generation string) error {
	if endpoint == nil {
		return storage.ErrRemoteConfig
	}
	if err := s.check(ctx, b, generation); err != nil {
		return err
	}
	page, err := endpoint.ListSnapshot(ctx, relative)
	if err != nil {
		return mapError(err)
	}
	if err = s.check(ctx, b, generation); err != nil {
		return err
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer gated.Rollback()
	tx := gated.Tx()
	// Use the same live generation derivation as scan/reference fences, including
	// the existing rclone config incarnation while composition replaces mounts.
	var current string
	var removed bool
	err = tx.QueryRowContext(ctx, `SELECT CASE WHEN r.kind='rclone' THEN CAST(m.generation AS TEXT) ELSE CAST(r.generation AS TEXT) END,r.removed FROM remote_sources r LEFT JOIN mount_backend_configs m ON m.mount_id=r.mount_id WHERE r.id=?`, b.ID).Scan(&current, &removed)
	if err != nil || removed || current != generation {
		return storage.ErrRemoteChanged
	}
	var valid bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM remote_listing_sessions WHERE id=? AND source_id=? AND generation=? AND directory=? AND ready=0)`, id, b.ID, generation, filepath.Join(b.Root, filepath.FromSlash(relative))).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return storage.ErrRemoteCursor
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM remote_listing_entries WHERE listing_id=?`, id); err != nil {
		return err
	}
	for _, entry := range page.Entries {
		if entry.Relative == relative {
			continue
		}
		v := davSnapshot(entry, b.Root)
		if filepath.Dir(v.Path) != filepath.Join(b.Root, filepath.FromSlash(relative)) {
			return storage.ErrRemoteConfig
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO remote_listing_entries VALUES(?,?,?)`, id, v.Path, string(raw)); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE remote_listing_sessions SET ready=1 WHERE id=? AND generation=? AND ready=0`, id, generation); err != nil {
		return err
	}
	return gated.Commit()
}
