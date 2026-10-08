package operations

import (
	"context"
	"database/sql"
	"strconv"

	"portico.local/server/internal/apievents"
	"portico.local/server/internal/dbwork"
)

// AppendOperationTx publishes a committed operation revision in the same
// transaction as the operation change. An unchanged revision has no event.
func AppendOperationTx(tx *sql.Tx, id string) error {
	var revision int64
	if err := tx.QueryRow(`SELECT revision FROM console_operations WHERE id=?`, id).Scan(&revision); err != nil {
		return err
	}
	return apievents.Append(tx, apievents.AdminAudience, "operation.updated", "operation", id, strconv.FormatInt(revision, 10), nil)
}

func updateOperationTx(tx *sql.Tx, id, query string, args ...any) (bool, error) {
	result, err := tx.Exec(query, args...)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	return true, AppendOperationTx(tx, id)
}

func updateOperation(ctx context.Context, db *sql.DB, class dbwork.Class, id, query string, args ...any) (bool, error) {
	updated := false
	err := dbwork.WithWriteTx(ctx, db, class, func(tx *sql.Tx) error {
		var err error
		updated, err = updateOperationTx(tx, id, query, args...)
		return err
	})
	return updated, err
}
