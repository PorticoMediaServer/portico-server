package mediaanalysis

import (
	"context"
	"database/sql"
	"portico.local/server/internal/dbwork"
)

// Read at stage/checkpoint boundaries, never for individual frames. Explicit
// embedding budgets remain the fallback when no owner document is stored.
func (s *Service) storageLimit(ctx context.Context, key string, unit, fallback int64) int64 {
	if s.db == nil {
		return fallback
	}
	var value sql.NullInt64
	column := "analysisStorageGiB"
	if key == "analysisStageStorageMiB" {
		column = key
	} else if key != column {
		return fallback
	}
	if dbwork.QueryRow(ctx, s.db, `SELECT "`+column+`" FROM server_settings WHERE singleton=1`).Scan(&value) != nil || !value.Valid || value.Int64 < 1 {
		return fallback
	}
	return value.Int64 * unit
}
func (s *Service) cacheLimit(ctx context.Context) int64 {
	return s.storageLimit(ctx, "analysisStorageGiB", 1<<30, s.options.MaxCacheBytes)
}
func (s *Service) stageLimit(ctx context.Context) int64 {
	return s.storageLimit(ctx, "analysisStageStorageMiB", 1<<20, s.options.MaxGeneratedBytes)
}
