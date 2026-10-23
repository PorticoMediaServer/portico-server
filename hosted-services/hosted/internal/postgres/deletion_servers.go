package postgres

import (
	"context"
	"database/sql"
	"portico.local/hosted/internal/identity"
)

// Account deletion ends only central identity. The server receives a departure
// and keeps its member's profiles and data until its own owner removes them.
func prepareServerDepartureTx(ctx context.Context, tx *sql.Tx, account, server string) error {
	var owned, live bool
	if e := tx.QueryRowContext(ctx, `SELECT COALESCE(owner_account_id=$2,false),revoked_at IS NULL FROM servers WHERE id=$1 FOR UPDATE`, server, account).Scan(&owned, &live); e != nil {
		return e
	}
	if owned {
		return identity.ErrOwnedServers
	}
	if _, e := tx.ExecContext(ctx, `INSERT INTO server_member_departures(server_id,account_id) VALUES($1,$2) ON CONFLICT(server_id,account_id) DO UPDATE SET left_at=clock_timestamp(),pending=true`, server, account); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `DELETE FROM server_members WHERE server_id=$1 AND account_id=$2`, server, account); e != nil {
		return e
	}
	if live {
		var revision int64
		if e := tx.QueryRowContext(ctx, `UPDATE servers SET policy_revision=CASE WHEN policy_revision<9223372036854775807 THEN policy_revision+1 ELSE policy_revision END WHERE id=$1 RETURNING policy_revision`, server).Scan(&revision); e != nil {
			return e
		}
		return enqueueWake(ctx, tx, server, revision)
	}
	return nil
}
