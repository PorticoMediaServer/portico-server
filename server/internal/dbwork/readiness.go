package dbwork

import (
	"context"
	"database/sql"
)

// CheckWriteReady checks the same writer and priority gate that sign-in and
// other mutations need. A read-pool ping can succeed while this path is stuck.
// Nothing is changed or committed; the caller supplies a short probe deadline.
func CheckWriteReady(ctx context.Context, db *sql.DB) error {
	w, err := Begin(ctx, db, ClassInteractive)
	if err != nil {
		return err
	}
	if err = w.Rollback(); err != nil {
		return err
	}
	return ctx.Err()
}
