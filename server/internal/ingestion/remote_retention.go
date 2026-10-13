package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"time"
)

// Keep one current ready listing per live cached directory for inherited ignore
// configuration; retire older run/verification listings in small chunks. A
// terminal job cannot publish another native listing batch through its fence.
func pruneRemoteInventoryTx(ctx context.Context, tx *sql.Tx, source, cutoff string) (bool, error) {
	var listing, listingJob string
	err := tx.QueryRowContext(ctx, `SELECT l.id,l.job_id FROM remote_listing_sessions l WHERE
 (l.job_id IN(SELECT r.job_id FROM inventory_runs r JOIN jobs j ON j.id=r.job_id WHERE r.source_id=? AND j.status NOT IN('queued','running','paused') AND COALESCE(nullif(r.finished_at,''),j.created_at)<?)
 OR l.job_id IN(SELECT c.check_job FROM remote_inventory_checks c JOIN inventory_runs r ON r.job_id=c.job_id JOIN jobs j ON j.id=r.job_id WHERE r.source_id=? AND j.status NOT IN('queued','running','paused') AND COALESCE(nullif(r.finished_at,''),j.created_at)<?))
 AND NOT(l.ready=1 AND EXISTS(SELECT 1 FROM inventory_directory_cache d JOIN library_sources s ON s.id=d.source_id WHERE s.id=? AND s.enabled=1 AND d.incarnation=s.incarnation AND l.directory=CASE WHEN d.relative_path='.' THEN s.root ELSE s.root||?||d.relative_path END)
 AND l.id=(SELECT x.id FROM remote_listing_sessions x WHERE x.source_id=l.source_id AND x.directory=l.directory AND x.ready=1 ORDER BY x.created_at DESC,x.id DESC LIMIT 1))
 ORDER BY l.created_at,l.id LIMIT 1`, source, cutoff, source, cutoff, source, string(filepath.Separator)).Scan(&listing, &listingJob)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM remote_listing_entries WHERE rowid IN(SELECT rowid FROM remote_listing_entries WHERE listing_id=? LIMIT ?)`, listing, scanRetentionBatch)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		return true, nil
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM remote_inventory_checks WHERE listing_id=? OR check_job=?`, listing, listingJob); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM remote_listing_sessions WHERE id=?`, listing); err != nil {
		return false, err
	}
	return true, nil
}

// Latest retained directory snapshots may outlive their run summary; sweep old
// orphaned sessions once their directory is no longer cached or a newer ready
// listing exists. Live jobs and paused verification work remain protected.
func (s *Service) pruneOrphanRemoteInventory(ctx context.Context, tx *sql.Tx, source string) (bool, error) {
	var root string
	if err := tx.QueryRowContext(ctx, `SELECT root FROM library_sources WHERE id=?`, source).Scan(&root); err != nil {
		return false, err
	}
	var id string
	err := tx.QueryRowContext(ctx, `SELECT l.id FROM remote_listing_sessions l WHERE l.created_at<? AND (l.directory=? OR substr(l.directory,1,length(?)+1)=?||?)
 AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.id=l.job_id)
 AND NOT EXISTS(SELECT 1 FROM remote_inventory_checks c JOIN jobs j ON j.id=c.job_id WHERE c.check_job=l.job_id AND j.status IN('queued','running','paused'))
 AND (l.ready=0 OR EXISTS(SELECT 1 FROM remote_listing_sessions newer WHERE newer.source_id=l.source_id AND newer.directory=l.directory AND newer.ready=1 AND (newer.created_at>l.created_at OR newer.created_at=l.created_at AND newer.id>l.id))
 OR NOT EXISTS(SELECT 1 FROM inventory_directory_cache d JOIN library_sources s ON s.id=d.source_id WHERE s.id=? AND d.incarnation=s.incarnation AND s.enabled=1 AND l.directory=CASE WHEN d.relative_path='.' THEN s.root ELSE s.root||?||d.relative_path END)) ORDER BY l.created_at,l.id LIMIT 1`, time.Now().Add(-15*time.Minute).Unix(), root, root, root, string(filepath.Separator), source, string(filepath.Separator)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM remote_listing_entries WHERE rowid IN(SELECT rowid FROM remote_listing_entries WHERE listing_id=? LIMIT ?)`, id, scanRetentionBatch)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		return true, nil
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM remote_listing_sessions WHERE id=?`, id)
	return true, err
}
