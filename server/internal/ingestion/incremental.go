package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/storage"
	"strings"
	"time"
)

type scanModeKey struct{}

// QueueMode preserves the existing queue and authorization boundary. Empty mode
// chooses first import, weekly integrity, or incremental automatically.
func (s *Service) QueueMode(ctx context.Context, library, source, mode string, authorize func(*sql.Tx) error) (Job, error) {
	if mode != "" && mode != "incremental" && mode != "full" && mode != "integrity" {
		return Job{}, errors.New("invalid scan mode")
	}
	return s.QueueContext(context.WithValue(ctx, scanModeKey{}, mode), library, source, authorize)
}
func scanModeTx(ctx context.Context, tx *sql.Tx, source string) (string, error) {
	mode, _ := ctx.Value(scanModeKey{}).(string)
	if mode == "full" || mode == "integrity" {
		return mode, nil
	}
	var complete, integrity string
	err := tx.QueryRowContext(ctx, `SELECT last_complete_at,COALESCE((SELECT max(inventory_completed_at) FROM inventory_runs WHERE source_id=s.id AND scan_mode IN('first-import','full','integrity')),'') FROM library_sources s WHERE id=?`, source).Scan(&complete, &integrity)
	if err != nil {
		return "", err
	}
	if complete == "" {
		return "first-import", nil
	}
	checked, err := time.Parse(time.RFC3339Nano, integrity)
	if err != nil || time.Since(checked) >= 7*24*time.Hour {
		return "integrity", nil
	}
	return "incremental", nil
}

// A directory fingerprint describes direct entries, not descendants. A skipped
// parent still schedules every known direct child, in pages, so a file added in
// a nested folder is discovered even when the root's mtime did not change.
func (s *Service) skipUnchangedDirectory(ctx context.Context, job string, source catalog.LibrarySource, relative, cursor string) (bool, error) {
	var identity, revision string
	err := s.db.QueryRowContext(ctx, `SELECT c.identity,c.revision FROM inventory_directory_cache c JOIN inventory_runs r ON r.source_id=c.source_id AND r.root_incarnation=c.incarnation WHERE r.job_id=? AND r.scan_mode='incremental' AND c.relative_path=? AND c.revision<>''`, job, relative).Scan(&identity, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if cursor != "" && !strings.HasPrefix(cursor, "skip:") {
		return false, nil
	}
	req := storage.InventoryRequest{Root: source.ResolvedPath, RootIdentity: source.RootIdentity, RelativePath: relative, FollowSymlinks: source.FollowSymlinks}
	var current storage.Snapshot
	if s.storage != nil {
		current, err = s.storage.InventoryDirectory(ctx, source.ID, req)
	} else {
		current, err = storage.LocalInventoryDirectory(req)
	}
	if err != nil {
		return false, err
	}
	if current.ObjectIdentity != identity || storage.InventoryRevision(current) != revision {
		if strings.HasPrefix(cursor, "skip:") {
			return false, storage.ErrInventoryChanged
		}
		return false, nil
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return false, err
	}
	defer gated.Rollback()
	tx := gated.Tx()
	if _, err = catalog.InventoryFenceTx(ctx, tx, job); err != nil {
		return false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT relative_path FROM inventory_directory_cache WHERE source_id=? AND incarnation=? AND parent_path=? AND parent_revision=? AND relative_path<>? AND relative_path>? ORDER BY relative_path LIMIT 33`, source.ID, source.Incarnation, relative, revision, relative, strings.TrimPrefix(cursor, "skip:"))
	if err != nil {
		return false, err
	}
	children := []string{}
	for rows.Next() {
		var child string
		if err = rows.Scan(&child); err != nil {
			rows.Close()
			return false, err
		}
		children = append(children, child)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	state, next := "done", ""
	if len(children) > 32 {
		children = children[:32]
		state = "pending"
		next = "skip:" + children[len(children)-1]
	}
	for _, child := range children {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_directories(job_id,relative_path) VALUES(?,?)`, job, child); err != nil {
			return false, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE inventory_directories SET skipped=1,identity=?,revision=?,cursor=?,state=? WHERE job_id=? AND relative_path=? AND cursor=?`, identity, revision, next, state, job, relative, cursor)
	if err != nil {
		return false, err
	}
	return true, gated.Commit()
}
