package postgres

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/hosted/internal/claims"
)

// LeaveRegistration consumes an already verified original-key notice. The
// receipt contains only the payload digest, never a server or account identity.
// A delayed notice cannot delete a successor, even with the same server key.
func (s *Store) LeaveRegistration(ctx context.Context, w claims.LeaveWire, digest [32]byte) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Serialize exact notice retries without changing the account/server lock
	// order used by claims, retirement, check-ins and certificate workers.
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(encode($1::bytea,'hex'),1941028))`, digest[:]); e != nil {
		return e
	}
	var replay bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM server_leave_receipts WHERE notice_digest=$1)`, digest[:]).Scan(&replay); e != nil {
		return e
	}
	if replay {
		return tx.Commit()
	}
	if e = lockClaimAccounts(ctx, tx, "", w.ServerID); e != nil {
		return e
	}
	var operation string
	if e = tx.QueryRowContext(ctx, `SELECT id FROM claim_operations WHERE id=$1 AND server_id=$2 AND public_key=decode(translate($3,'-_','+/')||repeat('=',(4-length($3)%4)%4),'base64') AND claim_generation=$4::bigint AND credential_generation=$5::bigint AND state IN('installed','committed_pending_install') FOR UPDATE`, w.OperationID, w.ServerID, w.PublicKey, w.ClaimGeneration, w.CredentialGeneration).Scan(&operation); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return claims.ErrStale
		}
		return e
	}
	if e = lockClaimServer(ctx, tx, w.ServerID); e != nil {
		return e
	}
	var owner string
	if e = tx.QueryRowContext(ctx, `SELECT owner_account_id FROM servers WHERE id=$1 AND public_key=$2 AND claim_generation=$3::bigint AND credential_generation=$4::bigint AND revoked_at IS NULL FOR UPDATE`, w.ServerID, w.PublicKey, w.ClaimGeneration, w.CredentialGeneration).Scan(&owner); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return claims.ErrStale
		}
		return e
	}
	if e = dropServerIndexTx(ctx, tx, w.ServerID, owner); e != nil {
		return e
	}
	// Feed readers may be offline longer than the server. Preserve only public
	// names and a monotonic sequence so their next bounded read removes them.
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(directAddressLock)); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO direct_address_withdrawals(namespace,member_label,previous_member_label,sequence) SELECT namespace,member_label,previous_member_label,nextval('direct_address_sequence') FROM certificate_namespaces WHERE server_identity=$1 ON CONFLICT(namespace) DO UPDATE SET member_label=EXCLUDED.member_label,previous_member_label=EXCLUDED.previous_member_label,sequence=EXCLUDED.sequence`, w.ServerID); e != nil {
		return e
	}
	for _, q := range []string{
		`DELETE FROM certificate_challenges WHERE order_id IN(SELECT id FROM certificate_orders WHERE server_id=$1)`,
		`DELETE FROM certificate_order_admissions WHERE order_id IN(SELECT id FROM certificate_orders WHERE server_id=$1)`,
		`DELETE FROM certificate_orders WHERE server_id=$1`,
		`DELETE FROM ct_observations WHERE namespace IN(SELECT namespace FROM certificate_namespaces WHERE server_identity=$1)`,
		`DELETE FROM certificate_namespaces WHERE server_identity=$1`,
		`DELETE FROM direct_address_events WHERE server_id=$1`,
		`DELETE FROM member_label_rotations WHERE server_id=$1`,
		`DELETE FROM current_server_endpoints WHERE server_id=$1`,
		`DELETE FROM networking_route_state WHERE server_id=$1`,
		`DELETE FROM mutation_receipts WHERE server_id=$1`,
		`DELETE FROM wake_state WHERE server_id=$1`,
		`DELETE FROM account_notification_events WHERE server_id=$1`,
		`DELETE FROM server_ownership_transfers WHERE server_id=$1`,
		`DELETE FROM claim_approvals WHERE operation_id IN(SELECT id FROM claim_operations WHERE server_id=$1)`,
		`DELETE FROM claim_nonces WHERE operation_id IN(SELECT id FROM claim_operations WHERE server_id=$1)`,
		`DELETE FROM claim_cancellations WHERE operation_id IN(SELECT id FROM claim_operations WHERE server_id=$1)`,
		`DELETE FROM claim_web_pending WHERE server_id=$1`,
		`DELETE FROM server_returns WHERE server_id=$1`,
		`DELETE FROM security_audit WHERE target_id=$1 OR target_id IN(SELECT id FROM claim_operations WHERE server_id=$1)`,
		`DELETE FROM claim_operations WHERE server_id=$1`,
		`DELETE FROM servers WHERE id=$1`,
	} {
		if _, e = tx.ExecContext(ctx, q, w.ServerID); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO server_leave_receipts(notice_digest) VALUES($1)`, digest[:]); e != nil {
		return e
	}
	return tx.Commit()
}
