package identity

import (
	"context"
	"database/sql"
)

// ownerSwitchCountSQL visits at most 10001 other account keys through the
// primary-key index. The two ranges exclude the owner without joining profiles,
// memberships or invitations: retained disabled/Waiting accounts still count.
const ownerSwitchCountSQL = `SELECT count(*) FROM (
 SELECT id FROM accounts WHERE id<?1
 UNION ALL
 SELECT id FROM accounts WHERE id>?1
 LIMIT 10001
)`

// captureOwnerSwitchCountTx must use the authenticated switch preview's own
// transaction. Nil means the exact informational count exceeds the bound; it
// must be omitted, never truncated or interpreted as zero.
func captureOwnerSwitchCountTx(ctx context.Context, tx *sql.Tx, owner string) (*int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, ownerSwitchCountSQL, owner).Scan(&count); err != nil {
		return nil, err
	}
	if count > 10000 {
		return nil, nil
	}
	return &count, nil
}
