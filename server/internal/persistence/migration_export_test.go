package persistence

import (
	"context"
	"database/sql"
)

// External tests exercise the real copy boundary with the identity scrubber.
func BackupBeforeMigrationsForTest(ctx context.Context, db *sql.DB) error {
	return backupBeforeMigrations(ctx, db)
}
