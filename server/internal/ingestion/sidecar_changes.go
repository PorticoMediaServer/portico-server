package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/storage"
)

// Deletions are observed only under complete, stable coverage. Failed or
// changing paths keep their prior evidence, just as media files do.
func (s *Service) reconcileSidecars(ctx context.Context, job string, source catalog.LibrarySource) error {
	if more, err := s.pruneDirectoryCachePage(ctx, job, source); more || err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.relative_path FROM inventory_sidecars c JOIN inventory_runs r ON r.source_id=c.source_id WHERE r.job_id=? AND r.authoritative=1 AND c.incarnation=r.root_incarnation AND c.seen_job<>? AND
 (NOT EXISTS(SELECT 1 FROM inventory_run_targets t WHERE t.job_id=r.job_id) OR EXISTS(SELECT 1 FROM inventory_run_targets t WHERE t.job_id=r.job_id AND (c.directory_path=t.relative_path OR substr(c.directory_path,1,length(t.relative_path)+1)=t.relative_path||?))) AND
 NOT EXISTS(SELECT 1 FROM inventory_scan_issues x WHERE x.job_id=r.job_id AND x.protect=1 AND (x.relative_path='.' OR c.relative_path=x.relative_path OR substr(c.relative_path,1,length(x.relative_path)+1)=x.relative_path||?)) ORDER BY c.relative_path LIMIT 32`, job, job, string(filepath.Separator), string(filepath.Separator))
	if err != nil {
		return err
	}
	paths := []string{}
	for rows.Next() {
		var path string
		if err = rows.Scan(&path); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, path)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	safe := map[string]bool{}
	for _, path := range paths {
		folder := filepath.Dir(path)
		if _, ok := safe[folder]; ok {
			continue
		}
		err = s.catalog.VerifyInventoryAbsence(ctx, source, job, path)
		if err != nil {
			if errors.Is(err, storage.ErrBusy) || ctx.Err() != nil {
				return err
			}
			if err = s.excludeDirectory(ctx, job, folder, "directory_changed"); err != nil {
				return err
			}
			safe[folder] = false
		} else {
			safe[folder] = true
		}
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer gated.Rollback()
	tx := gated.Tx()
	if _, err = catalog.InventoryFenceTx(ctx, tx, job); err != nil {
		return err
	}
	for _, path := range paths {
		if !safe[filepath.Dir(path)] {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_sidecar_changes(job_id,relative_path) VALUES(?,?)`, job, filepath.Dir(path)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM inventory_sidecars WHERE source_id=? AND incarnation=? AND relative_path=? AND seen_job<>?`, source.ID, source.Incarnation, path, job); err != nil {
			return err
		}
	}
	if len(paths) == 0 {
		_, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET phase='analysis_enqueue' WHERE job_id=?`, job)
	}
	if err != nil {
		return err
	}
	return gated.Commit()
}

// A show/album sidecar may govern child seasons/discs. Queue those files in
// bounded pages; a changed sidecar never invalidates technical/deep facts.
func (s *Service) enqueueSidecarChanges(ctx context.Context, job string, source catalog.LibrarySource) (bool, error) {
	var folder, cursor string
	err := s.db.QueryRowContext(ctx, `SELECT relative_path,cursor FROM inventory_sidecar_changes WHERE job_id=? ORDER BY relative_path LIMIT 1`, job).Scan(&folder, &cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if s.LocalMetadata != nil {
		s.LocalMetadata.InvalidateSidecars(filepath.Join(source.ResolvedPath, folder))
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
	rows, err := tx.QueryContext(ctx, `SELECT id FROM inventory_objects WHERE source_id=? AND root_incarnation=? AND state='available' AND retired=0 AND id>? AND (?='.' OR directory_path=? OR substr(directory_path,1,length(?)+1)=?||?) ORDER BY id LIMIT 64`, source.ID, source.Incarnation, cursor, folder, folder, folder, folder, string(filepath.Separator))
	if err != nil {
		return false, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET analysis_state='pending',analysis_revision='',analysis_operations_json=COALESCE((SELECT json_group_array(value) FROM json_each(analysis_operations_json) WHERE value NOT IN('local_metadata','subtitles')),'[]') WHERE id=?`, id); err != nil {
			return false, err
		}
	}
	if len(ids) > 0 {
		_, err = tx.ExecContext(ctx, `UPDATE inventory_sidecar_changes SET cursor=? WHERE job_id=? AND relative_path=?`, ids[len(ids)-1], job, folder)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM inventory_sidecar_changes WHERE job_id=? AND relative_path=?`, job, folder)
	}
	if err != nil {
		return false, err
	}
	return true, gated.Commit()
}
