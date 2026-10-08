package postgres

import (
	"context"
	"database/sql"
	"errors"
)

var ErrMembershipBusy = errors.New("account or server membership operation is in progress")

// invalidateAccountAttachmentTx drops what an account could still redeem on a
// server under the retired ticket model.
func invalidateAccountAttachmentTx(ctx context.Context, tx *sql.Tx, server, account string) error {
	if _, e := tx.ExecContext(ctx, `DELETE FROM attachment_tickets WHERE server_id=$1 AND account_id=$2 AND consumed_at IS NULL`, server, account); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `DELETE FROM profile_trusted_selections WHERE server_id=$1 AND account_id=$2`, server, account)
	return e
}
