package webhooks

import (
	"context"
	"database/sql"
)

// Pruning uses the same order as the log and dispatcher. Pending deliveries
// retain their independent retry lifetime and are never removed by log limits.
func pruneDeliveryLogTx(ctx context.Context, tx *sql.Tx, webhook string, keep int) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM webhook_deliveries WHERE webhook_id=? AND state<>'pending' AND id NOT IN(SELECT id FROM webhook_deliveries WHERE webhook_id=? AND state<>'pending' ORDER BY created_ms DESC,rowid DESC LIMIT ?)`, webhook, webhook, keep)
	return err
}
