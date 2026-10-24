package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"portico.local/hosted/internal/claims"
	"strconv"
)

// CancelClaim accepts only a verified original-key deny-only assertion. It never
// needs an approving account session, and a late A assertion cannot clear B.
func (s *Store) CancelClaim(ctx context.Context, w claims.CancelWire, digest [32]byte) (claims.CancellationResult, error) {
	var empty claims.CancellationResult
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return empty, e
	}
	defer tx.Rollback()
	// Receipt-only replay is privacy-limited and remains valid after account
	// erasure. It grants no current authority and only acknowledges exact bytes.
	var request string
	var saved []byte
	e = tx.QueryRowContext(ctx, `SELECT request_id,request_digest FROM claim_cancellations WHERE operation_id=$1`, w.OperationID).Scan(&request, &saved)
	if e == nil {
		if request != w.RequestID || !bytes.Equal(saved, digest[:]) {
			return empty, claims.ErrStale
		}
		if e = tx.Commit(); e != nil {
			return empty, e
		}
		return claims.CancellationResult{OperationID: w.OperationID, RequestID: w.RequestID}, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return empty, e
	}
	var account, origin string
	if e = tx.QueryRowContext(ctx, `SELECT account_id,origin_account_id FROM claim_operations WHERE id=$1`, w.OperationID).Scan(&account, &origin); e != nil {
		return empty, claimAuthError(e)
	}
	if origin != w.AccountID {
		return empty, claims.ErrStale
	}
	var locked string
	if e = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&locked); e != nil {
		return empty, claimAuthError(e)
	}
	// Match the common ordering without turning approval/session expiry into a
	// prerequisite for revoking a credential.
	if _, e = readClaimApproval(ctx, tx, w.OperationID, true); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return empty, e
	}
	var opAccount, server, state string
	var key []byte
	var local int64
	var generation, credential sql.NullInt64
	if e = tx.QueryRowContext(ctx, `SELECT account_id,server_id,public_key,local_generation,state,claim_generation,credential_generation,origin_account_id FROM claim_operations WHERE id=$1 FOR UPDATE`, w.OperationID).Scan(&opAccount, &server, &key, &local, &state, &generation, &credential, &origin); e != nil {
		return empty, e
	}
	if opAccount != account || origin != w.AccountID || server != w.ServerID || base64.RawURLEncoding.EncodeToString(key) != w.PublicKey || local != w.LocalGeneration {
		return empty, claims.ErrStale
	}
	if w.Mode == "committed" && (!generation.Valid || strconv.FormatInt(generation.Int64, 10) != w.ClaimGeneration) {
		return empty, claims.ErrStale
	}
	if state == "cancelled" {
		// A concurrent cancel may have committed while this request waited for the
		// account lock. Re-read the exact immutable receipt before acknowledging.
		if e = tx.QueryRowContext(ctx, `SELECT request_id,request_digest FROM claim_cancellations WHERE operation_id=$1`, w.OperationID).Scan(&request, &saved); e != nil {
			return empty, e
		}
		if request != w.RequestID || !bytes.Equal(saved, digest[:]) {
			return empty, claims.ErrStale
		}
		if e = tx.Commit(); e != nil {
			return empty, e
		}
		return claims.CancellationResult{OperationID: w.OperationID, RequestID: w.RequestID}, nil
	}
	if e = lockClaimServer(ctx, tx, server); e != nil {
		return empty, e
	}
	if generation.Valid {
		// Guards include both generations and key, so even a retained cancellation
		// for an earlier operation cannot touch a later claim's credential/grants.
		result, e := tx.ExecContext(ctx, `UPDATE servers SET owner_account_id=NULL,token_hash=NULL,token_ciphertext=NULL,revoked_at=clock_timestamp(),base_url=NULL,endpoint_verified=false,presence_expires_at=NULL,policy_revision=CASE WHEN policy_revision<9223372036854775807 THEN policy_revision+1 ELSE policy_revision END,policy_issued_at=clock_timestamp() WHERE id=$1 AND owner_account_id=$2 AND public_key=$3 AND claim_generation=$4 AND credential_generation=$5 AND revoked_at IS NULL`, server, account, w.PublicKey, generation.Int64, credential.Int64)
		if e != nil {
			return empty, e
		}
		count, e := result.RowsAffected()
		if e != nil {
			return empty, e
		}
		if count == 1 {
			if e = withdrawClaimAddressesTx(ctx, tx, server, w.OperationID); e != nil {
				return empty, e
			}
			if e = dropServerIndexTx(ctx, tx, server, account); e != nil {
				return empty, e
			}
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE claim_operations SET state='cancelled',cancelled_at=clock_timestamp() WHERE id=$1`, w.OperationID); e != nil {
		return empty, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO claim_cancellations(operation_id,request_id,request_digest) VALUES($1,$2,$3)`, w.OperationID, w.RequestID, digest[:]); e != nil {
		return empty, e
	}
	// Nonce rows retain bounded replay/quota evidence; absence of the approval
	// disables all positive delivery and releases the captured-family FK.
	if _, e = tx.ExecContext(ctx, `DELETE FROM claim_approvals WHERE operation_id=$1`, w.OperationID); e != nil {
		return empty, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE setup_claim_starts SET phase='cancelled' WHERE operation_id=$1 AND phase<>'consumed'`, w.OperationID); e != nil {
		return empty, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE owner_claim_starts SET phase='cancelled' WHERE operation_id=$1 AND phase<>'consumed'`, w.OperationID); e != nil {
		return empty, e
	}
	if e = audit(ctx, tx, account, "claim.cancel", w.OperationID, 0); e != nil {
		return empty, e
	}
	if e = tx.Commit(); e != nil {
		return empty, e
	}
	return claims.CancellationResult{OperationID: w.OperationID, RequestID: w.RequestID}, nil
}

func (s *Store) AcknowledgeClaim(ctx context.Context, hash []byte, in claims.Commit) (claims.Commit, error) {
	var empty claims.Commit
	if len(hash) != 32 {
		return empty, claims.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return empty, e
	}
	defer tx.Rollback()
	var account string
	if e = tx.QueryRowContext(ctx, `SELECT account_id FROM claim_operations WHERE id=$1`, in.OperationID).Scan(&account); e != nil {
		return empty, claimAuthError(e)
	}
	var locked string
	if e = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&locked); e != nil {
		return empty, claimAuthError(e)
	}
	if _, e = readClaimApproval(ctx, tx, in.OperationID, true); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return empty, e
	}
	var server, state string
	var generation, credential int64
	var committed []byte
	if e = tx.QueryRowContext(ctx, `SELECT server_id,state,claim_generation,credential_generation,committed_token_hash FROM claim_operations WHERE id=$1 FOR UPDATE`, in.OperationID).Scan(&server, &state, &generation, &credential, &committed); e != nil {
		return empty, e
	}
	if server != in.ServerID || (state != "committed_pending_install" && state != "installed") || strconv.FormatInt(generation, 10) != in.ClaimGeneration || strconv.FormatInt(credential, 10) != in.CredentialGeneration || !bytes.Equal(hash, committed) {
		return empty, claims.ErrStale
	}
	if e = lockClaimServer(ctx, tx, server); e != nil {
		return empty, e
	}
	var valid bool
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(owner_account_id=$2 AND claim_generation=$3 AND credential_generation=$4 AND token_hash=$5 AND revoked_at IS NULL,false) FROM servers WHERE id=$1 FOR UPDATE`, server, account, generation, credential, hash).Scan(&valid); e != nil {
		return empty, e
	}
	if !valid {
		return empty, claims.ErrStale
	}
	if state != "installed" {
		if _, e = tx.ExecContext(ctx, `UPDATE claim_operations SET state='installed',installed_at=clock_timestamp() WHERE id=$1`, in.OperationID); e != nil {
			return empty, e
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM claim_approvals WHERE operation_id=$1`, in.OperationID); e != nil {
			return empty, e
		}
		// The server list shows installed servers only, so installing is a
		// change to it: a reader that cached the list at commit must see it.
		if e = touchServerListTx(ctx, tx, server); e != nil {
			return empty, e
		}
	}
	if e = tx.Commit(); e != nil {
		return empty, e
	}
	return in, nil
}
