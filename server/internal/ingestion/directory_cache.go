package ingestion

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
)

// The directory cache describes the current tree, not a history of every
// folder name. A fully listed, verified parent proves which old children went.
// Protected subtrees keep their checkpoints until they can be observed again.
func (s *Service) pruneDirectoryCachePage(ctx context.Context, job string, source catalog.LibrarySource) (bool, error) {
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return false, err
	}
	defer gated.Rollback()
	tx := gated.Tx()
	if _, err = catalog.InventoryFenceTx(ctx, tx, job); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM inventory_directory_cache WHERE rowid IN(
 SELECT c.rowid FROM inventory_directory_cache c WHERE c.source_id=? AND c.incarnation=? AND c.relative_path<>'.' AND (
 EXISTS(SELECT 1 FROM inventory_directories d WHERE d.job_id=? AND d.state='done' AND d.skipped=0 AND d.relative_path=c.parent_path AND (d.revision<>c.parent_revision OR c.seen_job<>?))
 OR NOT EXISTS(SELECT 1 FROM inventory_directory_cache p WHERE p.source_id=c.source_id AND p.incarnation=c.incarnation AND p.relative_path=c.parent_path))
 AND NOT EXISTS(SELECT 1 FROM inventory_scan_issues x WHERE x.job_id=? AND x.protect=1 AND (x.relative_path='.' OR c.relative_path=x.relative_path OR substr(c.relative_path,1,length(x.relative_path)+1)=x.relative_path||?)) ORDER BY length(c.relative_path) LIMIT 256)`, source.ID, source.Incarnation, job, job, job, string(filepath.Separator))
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, gated.Commit()
}
