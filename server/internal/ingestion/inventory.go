package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/notify"
	"portico.local/server/internal/storage"
	"time"
)

func (s *Service) readySource(ctx context.Context, job string, source *catalog.LibrarySource) error {
	var observed storage.Snapshot
	var err error
	remote := s.storage.IsRemote(source.ResolvedPath)
	if source.Kind != "local" && source.Kind != "remote" {
		return errors.New("inventory_adapter_unavailable")
	}
	if source.Kind == "remote" && !remote {
		return errors.New(sourceUnavailable)
	}
	if remote {
		if err = s.storage.Remote.CheckScan(ctx, job, source.ResolvedPath); err != nil {
			return err
		}
		if source.Kind != "remote" {
			if _, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `UPDATE library_sources SET kind='remote',classification='network' WHERE id=? AND generation=?`, source.ID, source.Generation); err != nil {
				return err
			}
			source.Kind = "remote"
		}
	}
	if s.storage != nil {
		observed, err = s.storage.InspectRoot(ctx, source.ResolvedPath)
	} else {
		observed, err = storage.LocalInventoryDirectory(storage.InventoryRequest{Root: source.ResolvedPath, RelativePath: "."})
	}
	if err != nil {
		if errors.Is(err, storage.ErrBusy) || ctx.Err() != nil {
			return err
		}
		return errors.New(sourceUnavailable)
	}
	actual := storage.RootIdentity(observed)
	if actual == "" {
		return catalog.ErrSourceChanged
	}
	if source.RootIdentity != "" && actual != source.RootIdentity {
		if !storage.SameRoot(source.RootIdentity, actual) {
			return catalog.ErrSourceChanged
		}
		// The same folder under a device number this boot handed out afresh.
		if err = s.catalog.AdoptRootIdentity(ctx, job, *source, actual); err != nil {
			return err
		}
		source.RootIdentity = actual
	}
	if source.RootIdentity == "" {
		gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
		if e != nil {
			return e
		}
		tx := gated.Tx()
		defer gated.Rollback()
		var state string
		if e = tx.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id=?`, job).Scan(&state); e != nil {
			return e
		}
		if state != "running" {
			return context.Canceled
		}
		if _, e = tx.ExecContext(ctx, `UPDATE library_sources SET root_identity=? WHERE id=? AND root_identity='' AND generation=?`, actual, source.ID, source.Generation); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE inventory_runs SET root_identity=? WHERE job_id=? AND root_identity=''`, actual, job); e != nil {
			return e
		}
		if e = gated.Commit(); e != nil {
			return e
		}
		source.RootIdentity = actual
	}
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if e != nil {
		return e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	if _, e = catalog.InventoryFenceTx(ctx, tx, job); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE library_sources SET health='inventory_running' WHERE id=? AND health!='inventory_running'`, source.ID); e != nil {
		return e
	}
	return gated2.Commit()
}
func (s *Service) inventoryPage(ctx context.Context, job string, source catalog.LibrarySource) error {
	var relative string
	err := s.db.QueryRowContext(ctx, `SELECT relative_path FROM inventory_directories WHERE job_id=? AND state='pending' ORDER BY relative_path LIMIT 1`, job).Scan(&relative)
	if errors.Is(err, sql.ErrNoRows) {
		var mode string
		if e := s.db.QueryRowContext(ctx, `SELECT scan_mode FROM inventory_runs WHERE job_id=?`, job).Scan(&mode); e != nil {
			return e
		}
		if mode == "incremental" {
			return s.phase(ctx, job, "verifying", true)
		}
		return s.phase(ctx, job, "legacy_inventory", true)
	}
	if err != nil {
		return err
	}
	err = s.readDirectoryPage(ctx, job, source, relative)
	if err == nil || ctx.Err() != nil || errors.Is(err, storage.ErrBusy) {
		return err
	}
	var read *directoryReadError
	if !errors.As(err, &read) && !errors.Is(err, storage.ErrInventoryChanged) {
		return stoppedAt(relative, err)
	}
	if relative == "." && !errors.Is(err, storage.ErrInventoryChanged) {
		return stoppedAt(relative, errors.New(sourceUnavailable))
	}
	if errors.Is(err, storage.ErrRemoteOffline) || errors.Is(err, storage.ErrRemoteCredentials) || errors.Is(err, storage.ErrRemoteChanged) || errors.Is(err, storage.ErrRemoteBinary) {
		return stoppedAt(relative, err)
	}
	if errors.Is(err, os.ErrNotExist) || source.Kind == "remote" && !errors.Is(err, storage.ErrRemoteCursor) {
		if widened, e := s.widenTarget(ctx, job, relative); e != nil {
			return e
		} else if widened {
			return nil
		}
	}
	reason := "directory_unreadable"
	if errors.Is(err, storage.ErrInventoryChanged) {
		reason = "directory_changed"
	}
	return s.excludeDirectory(ctx, job, relative, reason)

}
func (s *Service) readDirectoryPage(ctx context.Context, job string, source catalog.LibrarySource, relative string) error {
	var cursor, revision string
	if err := s.db.QueryRowContext(ctx, `SELECT cursor,revision FROM inventory_directories WHERE job_id=? AND relative_path=? AND state='pending'`, job, relative).Scan(&cursor, &revision); err != nil {
		return err
	}
	if s.storage.IsRemote(source.ResolvedPath) {
		directory := filepath.Join(source.ResolvedPath, relative)
		remote, err := s.storage.Remote.ListPage(ctx, job, directory, cursor)
		if err != nil {
			return &directoryReadError{err}
		}
		next := remote.Next
		if remote.Complete {
			next = ""
		}
		page := storage.InventoryPage{RootIdentity: source.RootIdentity, DirectoryIdentity: storage.RemoteDirectoryIdentity(remote.SourceID, directory), DirectoryRevision: remote.ID, Entries: remote.Entries, NextCursor: next, Complete: remote.Complete, Pending: remote.Pending, Remote: &remote}
		return s.catalog.CommitInventoryPage(ctx, job, relative, cursor, page)
	}
	req := storage.InventoryRequest{Root: source.ResolvedPath, RelativePath: relative, RootIdentity: source.RootIdentity, DirectoryRevision: revision, Cursor: cursor, Limit: s.inventoryBatch(ctx), FollowSymlinks: source.FollowSymlinks}
	var page storage.InventoryPage
	var err error
	if s.storage != nil {
		page, err = s.storage.InventoryPage(ctx, source.ID, req)
	} else {
		page, err = storage.LocalInventoryPage(req)
	}
	if err != nil {
		return &directoryReadError{err}
	}
	if err = s.catalog.CommitInventoryPage(ctx, job, relative, cursor, page); err != nil {
		return err
	}
	s.noteDirectoryPage(job, relative, cursor, page)
	return nil
}
func (s *Service) phase(ctx context.Context, job, phase string, reset bool) error {
	gated3, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	if _, err = catalog.InventoryFenceTx(ctx, tx, job); err != nil {
		return err
	}
	if reset {
		_, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET phase=?,verify_cursor='',reconcile_cursor='' WHERE job_id=?`, phase, job)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET phase=? WHERE job_id=?`, phase, job)
	}
	if err != nil {
		return err
	}
	return gated3.Commit()
}
func (s *Service) verifyPage(ctx context.Context, job string, source catalog.LibrarySource) error {
	rows, err := s.db.QueryContext(ctx, `SELECT d.relative_path,d.identity,d.revision FROM inventory_directories d JOIN inventory_runs r ON r.job_id=d.job_id WHERE d.job_id=? AND d.relative_path>r.verify_cursor AND d.state='done' ORDER BY d.relative_path LIMIT ?`, job, s.verificationBatch(source))
	if err != nil {
		return err
	}
	type directory struct{ path, identity, revision string }
	dirs := []directory{}
	for rows.Next() {
		var d directory
		if err = rows.Scan(&d.path, &d.identity, &d.revision); err != nil {
			rows.Close()
			return err
		}
		dirs = append(dirs, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if s.storage.IsRemote(source.ResolvedPath) {
			verifier, ok := s.storage.Remote.(storage.RemoteInventoryVerifier)
			if !ok {
				return storage.ErrRemoteConfig
			}
			if err = verifier.VerifyInventoryDirectory(ctx, job, filepath.Join(source.ResolvedPath, d.path), d.revision); err != nil {
				if errors.Is(err, storage.ErrBusy) || ctx.Err() != nil {
					return err
				}
				if e := s.excludeDirectory(ctx, job, d.path, "directory_changed"); e != nil {
					return e
				}
			}
			continue
		}
		req := storage.InventoryRequest{Root: source.ResolvedPath, RootIdentity: source.RootIdentity, RelativePath: d.path, FollowSymlinks: source.FollowSymlinks}
		var v storage.Snapshot
		if s.storage != nil {
			v, err = s.storage.InventoryDirectory(ctx, source.ID, req)
		} else {
			v, err = storage.LocalInventoryDirectory(req)
		}
		if err != nil {
			if errors.Is(err, storage.ErrBusy) || ctx.Err() != nil {
				return err
			}
			if e := s.excludeDirectory(ctx, job, d.path, "directory_unreadable"); e != nil {
				return e
			}
			continue
		}
		if v.ObjectIdentity != d.identity || storage.InventoryRevision(v) != d.revision {
			if e := s.excludeDirectory(ctx, job, d.path, "directory_changed"); e != nil {
				return e
			}
		}
	}
	gated4, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated4.Tx()
	defer gated4.Rollback()
	if _, err = catalog.InventoryFenceTx(ctx, tx, job); err != nil {
		return err
	}
	if len(dirs) > 0 {
		_, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET verify_cursor=? WHERE job_id=?`, dirs[len(dirs)-1].path, job)
	} else {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		// Baseline path-only sources have no trustworthy pre-upgrade root identity.
		// Publish their observations, but require owner confirmation before absence.
		if !source.IdentityConfirmed {
			if _, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET phase='analysis_enqueue',warnings=warnings+1,inventory_completed_at=?,reconcile_cursor='' WHERE job_id=?`, now, job); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_changes(source_id,job_id,kind,created_at) VALUES(?,?,'root_identity_confirmation_required',?)`, source.ID, job, now); err != nil {
				return err
			}
			return gated4.Commit()
		}
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET phase='reconciling',authoritative=1,inventory_completed_at=?,reconcile_cursor='' WHERE job_id=? AND NOT EXISTS(SELECT 1 FROM inventory_directories WHERE job_id=? AND state='pending')`, now, job, job); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE library_sources SET last_complete_job=?,last_complete_at=?,last_integrity_at=CASE WHEN (SELECT scan_mode FROM inventory_runs WHERE job_id=?) IN('first-import','full','integrity') THEN ? ELSE last_integrity_at END WHERE id=?`, job, now, job, now, source.ID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_changes(source_id,job_id,kind,created_at) VALUES(?,?,'inventory_completed',?)`, source.ID, job, now)
	}
	if err != nil {
		return err
	}
	return gated4.Commit()
}
func (s *Service) reconcilePage(ctx context.Context, job string, source catalog.LibrarySource) error {
	// An empty folder where there was media is an unmounted disk, not a list
	// of deleted files: the scan stops before anything is marked.
	if unmounted, err := s.catalog.SourceLooksUnmounted(ctx, job, source.ID); err != nil {
		return err
	} else if unmounted {
		return catalog.ErrSourceLooksUnmounted
	}
	rows, err := s.db.QueryContext(ctx, `SELECT o.id,o.asset_id,o.revision,o.missing_since,o.missing_observations,o.last_missing_job,o.relative_path,o.state FROM inventory_objects o JOIN inventory_runs r ON r.source_id=o.source_id WHERE r.job_id=? AND r.authoritative=1 AND o.root_incarnation=r.root_incarnation AND o.id>r.reconcile_cursor AND o.retired=0 AND o.seen_job!=? AND `+targetScopeSQL+` AND `+unprotectedObjectSQL+` AND NOT(o.state='available' AND o.missing_observations=0 AND EXISTS(SELECT 1 FROM inventory_directories d WHERE d.job_id=r.job_id AND d.relative_path=o.directory_path AND d.skipped=1 AND d.state='done')) ORDER BY o.id LIMIT ?`, job, job, string(filepath.Separator), string(filepath.Separator), s.reconciliationBatch(source))
	if err != nil {
		return err
	}
	type missing struct {
		id, asset, revision, since, last, relative, state string
		observations                                      int
	}
	batch := []missing{}
	for rows.Next() {
		var v missing
		if err = rows.Scan(&v.id, &v.asset, &v.revision, &v.since, &v.observations, &v.last, &v.relative, &v.state); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// No filesystem observation occurs under the catalog writer. Recheck absence
	// immediately before each bounded publication, not just at enumeration end.
	check, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	checked := map[string]bool{}
	unsafe := map[string]bool{}
	for _, v := range batch {
		parent := filepath.Dir(v.relative)
		if !checked[parent] {
			if err = s.catalog.VerifyInventoryAbsence(check, source, job, v.relative); err != nil {
				if errors.Is(err, storage.ErrBusy) || check.Err() != nil {
					return err
				}
				unsafe[parent] = true
				if e := s.excludeDirectory(ctx, job, parent, "directory_changed"); e != nil {
					return e
				}
			}
			checked[parent] = true
		}
	}
	gated5, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated5.Tx()
	defer gated5.Rollback()
	if _, err = catalog.InventoryFenceTx(ctx, tx, job); err != nil {
		return err
	}
	// A file a complete, verified listing of its folder did not contain is
	// missing from now. It stays in the catalogue, out of use, until the
	// owner's rule for missing titles removes it or the file comes back.
	now := time.Now().UTC()
	for _, v := range batch {
		if unsafe[filepath.Dir(v.relative)] {
			continue
		}
		if err = catalog.MarkInventoryMissingTx(ctx, tx, source.ID, job, v.id, now); err != nil {
			return err
		}
	}
	if len(batch) > 0 {
		_, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET reconcile_cursor=? WHERE job_id=?`, batch[len(batch)-1].id, job)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET phase='sidecars',reconcile_cursor='' WHERE job_id=?`, job)
	}
	if err != nil {
		return err
	}
	return gated5.Commit()
}

// stopError says which folder, relative to the source's root, a scan step was
// reading when it failed. It changes nothing about how the failure is
// classified; it only lets the owner's notice say where to look.
type stopError struct {
	path  string
	cause error
}

func (e *stopError) Error() string { return e.cause.Error() }
func (e *stopError) Unwrap() error { return e.cause }
func stoppedAt(path string, err error) error {
	if err == nil {
		return nil
	}
	return &stopError{path: path, cause: err}
}

func (s *Service) fail(ctx context.Context, job, source string, cause error) {
	code := "scan_inventory_incomplete"
	health := "degraded"
	if errors.Is(cause, catalog.ErrSourceChanged) {
		code = "scan_root_changed"
		health = "root_changed"
	} else if errors.Is(cause, storage.ErrRemoteChanged) {
		code = "scan_remote_configuration_changed"
		health = "root_changed"
	} else if errors.Is(cause, catalog.ErrSourceLooksUnmounted) || errors.Is(cause, storage.ErrRemoteOffline) || errors.Is(cause, storage.ErrRemoteCredentials) || errors.Is(cause, storage.ErrRemoteBinary) || cause.Error() == sourceUnavailable {
		code = sourceUnavailable
		health = "offline"
	}
	gated6, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return
	}
	tx := gated6.Tx()
	defer gated6.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE jobs SET status='failed',error=? WHERE id=? AND status='running'`, code, job)
	if err != nil {
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return
	}
	s.forgetListing(job)
	if _, err = tx.ExecContext(ctx, `DELETE FROM inventory_source_active WHERE job_id=?`, job); err != nil {
		return
	}
	if _, err = tx.ExecContext(ctx, `UPDATE library_sources SET health=? WHERE id=?`, health, source); err != nil {
		return
	}
	// A stopped scan is the one ingestion outcome an administrator has to act on.
	// It is deduped by source, so a source failing every hour is one standing
	// notice rather than a pile.
	var library string
	if err = tx.QueryRowContext(ctx, `SELECT library_id FROM library_sources WHERE id=?`, source).Scan(&library); err != nil {
		return
	}
	where := ""
	var stop *stopError
	if errors.As(cause, &stop) {
		where = stop.path
	}
	if _, err = notify.NotifyScanFailure(tx, time.Now().UnixMilli(), library, source, code, where); err != nil {
		return
	}
	_ = gated6.Commit()
}

// Every due timestamp is persisted. A paused source claim suppresses scheduled
// duplicates; fairness and admission are handled by the same scan worker pool.
func (s *Service) schedule(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM library_sources WHERE enabled=1 AND NOT EXISTS(SELECT 1 FROM dvr_private_libraries d WHERE d.library_id=library_sources.library_id) AND interval_seconds>0 AND next_scan_at!='' AND next_scan_at<=? ORDER BY next_scan_at,id LIMIT 8`, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if len(ids) == 0 {
		return
	}
	if s.Admission != nil {
		allowed, err := s.Admission(ctx, nil, "library-scan")
		if err != nil || !allowed {
			return
		}
	}
	for _, id := range ids {
		gated7, e := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
		if e != nil {
			continue
		}
		tx := gated7.Tx()
		var interval int64
		var due string
		e = tx.QueryRowContext(ctx, `SELECT interval_seconds,next_scan_at FROM library_sources WHERE id=? AND enabled=1`, id).Scan(&interval, &due)
		now := time.Now().UTC()
		if e == nil && interval > 0 && due != "" && due <= now.Format(time.RFC3339Nano) {
			_, e = queueSourceTx(ctx, tx, id, false)
			// Deterministic bounded jitter for the next cycle, not each UI read.
			jitter := int64(0)
			for _, b := range []byte(id + due) {
				jitter = (jitter*31 + int64(b)) % 101
			}
			delay := interval + (interval*(jitter-50))/1000
			if e == nil {
				_, e = tx.ExecContext(ctx, `UPDATE library_sources SET next_scan_at=? WHERE id=?`, now.Add(time.Duration(delay)*time.Second).Format(time.RFC3339Nano), id)
			}
		}
		if e == nil {
			_ = gated7.Commit()
		} else {
			_ = gated7.Rollback()
		}
	}
}
func relativeObjectPath(source catalog.LibrarySource, relative string) (string, error) {
	if !filepath.IsLocal(relative) || relative == "." {
		return "", catalog.ErrAdminQuery
	}
	return filepath.Join(source.ResolvedPath, relative), nil
}

func (s *Service) verificationBatch(source catalog.LibrarySource) int {
	if s.storage.IsRemote(source.ResolvedPath) {
		return 1
	}
	return 8
}
func (s *Service) reconciliationBatch(source catalog.LibrarySource) int {
	// One potentially remote parent proof per turn prevents partial verification
	// of a large batch from starving durable forward progress.
	if s.storage.IsRemote(source.ResolvedPath) {
		return 1
	}
	return 128
}

// No configured schedules means no periodic scan query. Writes to source
// configuration wake Run; a configured source supplies its own due timestamp.
func (s *Service) nextScheduledScan(ctx context.Context) time.Duration {
	var stamp sql.NullString
	if e := s.db.QueryRowContext(ctx, `SELECT min(next_scan_at) FROM library_sources WHERE enabled=1 AND interval_seconds>0 AND next_scan_at<>'' AND NOT EXISTS(SELECT 1 FROM dvr_private_libraries d WHERE d.library_id=library_sources.library_id)`).Scan(&stamp); e != nil {
		return time.Minute
	}
	if !stamp.Valid {
		return 0
	}
	due, e := time.Parse(time.RFC3339Nano, stamp.String)
	if e != nil {
		return time.Minute
	}
	wait := time.Until(due)
	if wait < time.Minute {
		return time.Minute
	}
	return wait
}
