package postgres

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/hosted/internal/security"
)

// A missing cleanup channel never prevents Hosted account removal. The opaque
// obligation stays pending; only an exact original-key proof or its still-current
// installed authority can deliver it. No successor bearer inherits old jobs.
func prepareServerErasureTx(ctx context.Context, tx *sql.Tx, deletion, account string, profiles []string, disposition, server string) error {
	var operation sql.NullString
	var claim, credential sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT o.id,o.claim_generation,o.credential_generation FROM claim_operations o JOIN servers s ON s.id=o.server_id WHERE s.id=$1 AND o.claim_generation IS NOT NULL AND (
 ((s.owner_account_id=$2 OR (EXISTS(SELECT 1 FROM memberships m WHERE m.server_id=s.id AND m.account_id=$2 AND NOT m.revoked) OR EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.account_id=$2))) AND s.claim_generation=o.claim_generation AND s.credential_generation=o.credential_generation)
 OR (NOT COALESCE(s.owner_account_id=$2,false) AND NOT (EXISTS(SELECT 1 FROM memberships m WHERE m.server_id=s.id AND m.account_id=$2 AND NOT m.revoked) OR EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.account_id=$2)) AND o.state='cancelled'))
 ORDER BY o.claim_generation DESC LIMIT 1 FOR SHARE OF o`, server, account).Scan(&operation, &claim, &credential)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var owned, live bool
	var revision int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(owner_account_id=$2,false),revoked_at IS NULL,policy_revision FROM servers WHERE id=$1 FOR UPDATE`, server, account).Scan(&owned, &live, &revision); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO account_erasure_jobs(id,deletion_id,server_id,account_id,profile_ids,disposition,claim_operation_id,claim_generation,credential_generation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, security.RandomID("erase_"), deletion, server, account, profiles, disposition, operation, claim, credential); err != nil {
		return err
	}
	if owned {
		if operation.Valid {
			if err = withdrawClaimAddressesTx(ctx, tx, server, operation.String); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE servers SET owner_account_id=NULL,token_hash=NULL,token_ciphertext=NULL,revoked_at=clock_timestamp(),base_url=NULL,endpoint_verified=false,presence_expires_at=NULL,name='Retired server',policy_revision=CASE WHEN policy_revision<9223372036854775807 THEN policy_revision+1 ELSE policy_revision END,policy_issued_at=clock_timestamp() WHERE id=$1 AND owner_account_id=$2`, server, account); err != nil {
			return err
		}
		for _, q := range []string{`UPDATE memberships SET revoked=true,role='member' WHERE server_id=$1`, `DELETE FROM attachment_tickets WHERE server_id=$1`, `UPDATE invitations SET status='cancelled' WHERE server_id=$1 AND status='pending'`, `DELETE FROM wake_state WHERE server_id=$1`} {
			if _, err = tx.ExecContext(ctx, q, server); err != nil {
				return err
			}
		}
		if err = dropServerIndexTx(ctx, tx, server, ""); err != nil {
			return err
		}
	} else if live {
		if err = tx.QueryRowContext(ctx, `UPDATE servers SET policy_revision=CASE WHEN policy_revision<9223372036854775807 THEN policy_revision+1 ELSE policy_revision END,policy_issued_at=clock_timestamp() WHERE id=$1 RETURNING policy_revision`, server).Scan(&revision); err != nil {
			return err
		}
		return enqueueWake(ctx, tx, server, revision)
	}
	return nil
}
