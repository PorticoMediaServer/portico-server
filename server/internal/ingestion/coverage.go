package ingestion

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/storage"
	"time"
)

type directoryReadError struct{ error }

func (e *directoryReadError) Unwrap() error { return e.error }

// A failed path protects itself and its descendants, not the entire source.
// The separator is a bound argument, so wildcard characters in names are literal.
const unprotectedObjectSQL = `NOT EXISTS(SELECT 1 FROM inventory_scan_issues x WHERE x.job_id=r.job_id AND x.protect=1 AND (x.relative_path='.' OR o.relative_path=x.relative_path OR substr(o.relative_path,1,length(x.relative_path)+1)=x.relative_path||?))`

func (s *Service) excludeDirectory(ctx context.Context, job, relative, reason string) error {
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer gated.Rollback()
	tx := gated.Tx()
	source, err := catalog.InventoryFenceTx(ctx, tx, job)
	if err != nil {
		return err
	}
	if err = catalog.InventoryIssueTx(ctx, tx, job, storage.InventoryIssue{Path: filepath.Clean(relative), Reason: reason, Protect: true, Warning: true}); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inventory_directories SET state='blocked',cursor='' WHERE job_id=? AND relative_path=?`, job, relative); err != nil {
		return err
	}
	if reason == "directory_changed" {
		if err = recordRunNoticeTx(ctx, tx, job, source.ID, relative, time.Now().UnixMilli()); err != nil {
			return err
		}
	}
	return gated.Commit()
}

func (s *Service) inventoryBatch(ctx context.Context) int {
	switch dbwork.BackgroundSpeed(ctx) {
	case dbwork.SpeedGentle:
		return 32
	case dbwork.SpeedFast:
		return 128
	}
	return 64
}

func waitScanPace(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
