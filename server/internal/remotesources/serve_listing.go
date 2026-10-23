package remotesources

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/rcloneprogram"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

type serveListingCapture struct {
	job     string
	source  catalog.LibrarySource
	profile SourceReadProfile
}

func (c serveListingCapture) same(other serveListingCapture) bool {
	a, b := c.source, other.source
	return c.job == other.job && c.profile == other.profile && a.ID == b.ID && a.LibraryID == b.LibraryID && a.Generation == b.Generation && a.Incarnation == b.Incarnation && a.RootIdentity == b.RootIdentity && a.ResolvedPath == b.ResolvedPath && a.IdentityConfirmed == b.IdentityConfirmed && a.FollowSymlinks == b.FollowSymlinks
}

// serveListingFenceTx resolves the actual running inventory, including the
// original job behind a verification listing. A private listing ID or a source
// picker record is never inventory authority. No network runs in this method.
func (s *Service) serveListingFenceTx(ctx context.Context, tx *sql.Tx, b binding, relative, id, generation string) (serveListingCapture, error) {
	var capture serveListingCapture
	directory := filepath.Join(b.Root, filepath.FromSlash(relative))
	err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT c.job_id FROM remote_inventory_checks c WHERE c.check_job=l.job_id),l.job_id) FROM remote_listing_sessions l WHERE l.id=? AND l.source_id=? AND l.generation=? AND l.directory=? AND l.ready=0`, id, b.ID, generation, directory).Scan(&capture.job)
	if err != nil {
		return capture, storage.ErrRemoteCursor
	}
	capture.source, err = catalog.InventoryFenceTx(ctx, tx, capture.job)
	if err != nil {
		return capture, err
	}
	source := capture.source
	child, err := filepath.Rel(source.ResolvedPath, directory)
	if err != nil || source.Kind != "remote" || source.RootIdentity == "" || source.Incarnation == "" || !filepath.IsLocal(child) {
		return capture, storage.ErrRemoteChanged
	}
	var capturedPolicy, currentPolicy int64
	if err = tx.QueryRowContext(ctx, `SELECT r.policy_revision,p.revision FROM inventory_runs r JOIN library_scan_policies p ON p.library_id=? WHERE r.job_id=?`, source.LibraryID, capture.job).Scan(&capturedPolicy, &currentPolicy); err != nil {
		return capture, err
	}
	if capturedPolicy < 1 || capturedPolicy != currentPolicy {
		return capture, storage.ErrRemoteChanged
	}
	if err = s.ValidateScan(ctx, tx, capture.job, source.ResolvedPath); err != nil {
		return capture, err
	}
	current, err := observationGenerationTx(ctx, tx, b.ID)
	if err != nil {
		return capture, err
	}
	if current != generation {
		return capture, storage.ErrRemoteChanged
	}
	var root string
	if err = tx.QueryRowContext(ctx, `SELECT root FROM remote_sources WHERE id=? AND removed=0`, b.ID).Scan(&root); err != nil {
		return capture, err
	}
	actual, _, ok := s.find(directory)
	if root != b.Root || !ok || actual.ID != b.ID || actual.Removed {
		return capture, storage.ErrRemoteChanged
	}
	capture.profile, err = SourceReadProfileTx(ctx, tx, b.ID)
	return capture, err
}

// buildServeSnapshot retains the old two-million-child/512MiB private staging
// budget, with only 128 entries held per SQL batch. No partial body, duplicate,
// failed job/profile/process fence or late parse error publishes ready. This is
// directory evidence, not a file observation or content/decode permission.
func (s *Service) buildServeSnapshot(ctx context.Context, process *rcloneprogram.ServeProcess, b binding, relative, id, generation string) error {
	if process == nil {
		return storage.ErrRemoteConfig
	}
	endpoint, err := process.Endpoint(ctx)
	if err != nil {
		return err
	}
	initial, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	capture, err := s.serveListingFenceTx(ctx, initial.Tx(), b, relative, id, generation)
	if err == nil {
		err = process.CheckLocal(ctx)
	}
	if err == nil {
		_, err = initial.Tx().ExecContext(ctx, `DELETE FROM remote_listing_entries WHERE listing_id=?`, id)
	}
	if err == nil {
		err = initial.Commit()
	} else {
		initial.Rollback()
	}
	if err != nil {
		return err
	}
	fence := func(ctx context.Context, tx *sql.Tx) error {
		current, err := s.serveListingFenceTx(ctx, tx, b, relative, id, generation)
		if err != nil {
			return err
		}
		if !current.same(capture) {
			return storage.ErrRemoteChanged
		}
		return process.CheckLocal(ctx)
	}
	check := func(ctx context.Context) error {
		if err := process.Check(ctx); err != nil {
			return err
		}
		view, err := dbwork.BeginSnapshot(ctx, s.db)
		if err != nil {
			return err
		}
		defer view.Rollback()
		return fence(ctx, view.Tx())
	}
	var count, bytes int64
	batch := make([]storage.Snapshot, 0, 128)
	flush := func(ready bool) error {
		writer, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
		if err != nil {
			return err
		}
		defer writer.Rollback()
		if err = fence(ctx, writer.Tx()); err != nil {
			return err
		}
		for _, entry := range batch {
			raw, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			count++
			bytes += int64(len(raw))
			if count > 2_000_000 || bytes > 512<<20 {
				return storage.ErrRemoteLimit
			}
			if _, err = writer.Tx().ExecContext(ctx, `INSERT INTO remote_listing_entries VALUES(?,?,?)`, id, entry.Path, string(raw)); err != nil {
				return storage.ErrRemoteConfig
			}
		}
		if ready {
			result, err := writer.Tx().ExecContext(ctx, `UPDATE remote_listing_sessions SET ready=1 WHERE id=? AND source_id=? AND generation=? AND ready=0`, id, b.ID, generation)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil || n != 1 {
				return storage.ErrRemoteChanged
			}
		}
		if err = writer.Commit(); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}
	err = endpoint.VisitSnapshot(ctx, relative, check, func(entry remotemedia.DAVEntry) error {
		value := davSnapshot(entry, b.Root)
		if filepath.Dir(value.Path) != filepath.Join(b.Root, filepath.FromSlash(relative)) {
			return storage.ErrRemoteConfig
		}
		batch = append(batch, value)
		if len(batch) == cap(batch) {
			return flush(false)
		}
		return nil
	})
	if err != nil {
		return mapError(err)
	}
	return flush(true)
}
