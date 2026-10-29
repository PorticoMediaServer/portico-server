package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/compactcatalog"
	"testing"
)

func settleInventoryAnalysis(t *testing.T, s *catalog.Service, db *sql.DB, ctx context.Context, job, object, revision string, policy int64, f assets.Facts) error {
	t.Helper()
	return settleInventoryInfo(t, db, ctx, func() (compactcatalog.InfoAcceptReceipt, error) {
		return s.AcceptInventoryAnalysis(ctx, job, object, revision, policy, f)
	})
}
func settleInventoryLocalRefresh(t *testing.T, s *catalog.Service, db *sql.DB, ctx context.Context, job, object, revision string, policy int64, f assets.Facts) error {
	t.Helper()
	return settleInventoryInfo(t, db, ctx, func() (compactcatalog.InfoAcceptReceipt, error) {
		return s.AcceptInventoryLocalRefresh(ctx, job, object, revision, policy, f)
	})
}
func settleInventoryInfo(t *testing.T, db *sql.DB, ctx context.Context, accept func() (compactcatalog.InfoAcceptReceipt, error)) error {
	t.Helper()
	for turn := 0; turn < 8; turn++ {
		receipt, err := accept()
		if err != nil {
			return err
		}
		if receipt.State == compactcatalog.InfoInputApplied {
			return nil
		}
		if err = compactcatalog.Drain(ctx, db); err != nil {
			return err
		}
	}
	return errors.New("informational source did not apply")
}
