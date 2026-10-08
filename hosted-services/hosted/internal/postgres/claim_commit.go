package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"portico.local/hosted/internal/claims"
	"strconv"
	"time"
)

// lockClaimServer serializes absent-row creation too. Lock order remains
// account/family/access -> approval -> operation -> server -> nonce.
func lockClaimServer(ctx context.Context, tx *sql.Tx, id string) error {
	_, e := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,19411))`, id)
	return e
}
func consumeClaimNonce(ctx context.Context, tx *sql.Tx, v claims.Verified, c lockedClaim) error {
	var operation, purpose, request string
	var revision int64
	var nonce []byte
	var issued, expiry time.Time
	var consumed sql.NullTime
	e := tx.QueryRowContext(ctx, `SELECT operation_id,purpose,request_id,approval_revision,nonce,issued_at,expires_at,consumed_at FROM claim_nonces WHERE id=$1 FOR UPDATE`, v.Wire.NonceID).Scan(&operation, &purpose, &request, &revision, &nonce, &issued, &expiry, &consumed)
	if e != nil {
		return claimAuthError(e)
	}
	if operation != v.Wire.OperationID || purpose != v.Wire.Purpose || request != v.Wire.RequestID || revision != v.Wire.ApprovalRevision || base64.RawURLEncoding.EncodeToString(nonce) != v.Wire.Nonce || !issued.Equal(v.IssuedAt) || !expiry.Equal(v.ExpiresAt) || consumed.Valid {
		return claims.ErrStale
	}
	now, e := claimDecisionTime(ctx, tx, c, expiry)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE claim_nonces SET consumed_at=$2 WHERE id=$1`, v.Wire.NonceID, now)
	return e
}
func currentClaimResult(ctx context.Context, tx *sql.Tx, v claims.Verified) (claims.Commit, []byte, error) {
	var out claims.Commit
	var hash, cipher, committed []byte
	var owner sql.NullString
	var key string
	var claimGen, credentialGen, opClaim, opCredential int64
	var revoked sql.NullTime
	e := tx.QueryRowContext(ctx, `SELECT owner_account_id,public_key,claim_generation,credential_generation,token_hash,token_ciphertext,revoked_at FROM servers WHERE id=$1 FOR UPDATE`, v.Wire.ServerID).Scan(&owner, &key, &claimGen, &credentialGen, &hash, &cipher, &revoked)
	if errors.Is(e, sql.ErrNoRows) {
		return out, nil, claims.ErrStale
	}
	if e != nil {
		return out, nil, e
	}
	e = tx.QueryRowContext(ctx, `SELECT claim_generation,credential_generation,committed_token_hash FROM claim_operations WHERE id=$1`, v.Wire.OperationID).Scan(&opClaim, &opCredential, &committed)
	if e != nil {
		return out, nil, e
	}
	if !owner.Valid || owner.String != v.Wire.AccountID || key != v.Wire.PublicKey || revoked.Valid || claimGen != opClaim || credentialGen != opCredential || len(hash) != 32 || !bytes.Equal(hash, committed) || len(cipher) < 28 {
		return out, nil, claims.ErrStale
	}
	return claims.Commit{OperationID: v.Wire.OperationID, ServerID: v.Wire.ServerID, ClaimGeneration: strconv.FormatInt(claimGen, 10), CredentialGeneration: strconv.FormatInt(credentialGen, 10)}, cipher, nil
}

// FinalizeClaim creates exactly one generation; ambiguous response retries may
// consume a fresh nonce but cannot remint the operation's committed credential.
func (s *Store) FinalizeClaim(ctx context.Context, v claims.Verified, material claims.Material) (claims.Commit, error) {
	var empty claims.Commit
	if v.Wire.Kind != claims.ClaimProof || v.Wire.Purpose != claims.Finalize || len(material.Hash) != 32 || len(material.Ciphertext) < 28 || len(material.Ciphertext) > 4096 {
		return empty, claims.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return empty, e
	}
	defer tx.Rollback()
	c, e := lockClaim(ctx, tx, v)
	if e != nil {
		return empty, e
	}
	if e = lockClaimServer(ctx, tx, v.Wire.ServerID); e != nil {
		return empty, e
	}
	if c.state == "approved" {
		var generation, credential, policy int64
		var owner sql.NullString
		var key string
		var revoked sql.NullTime
		e = tx.QueryRowContext(ctx, `SELECT owner_account_id,public_key,claim_generation,credential_generation,policy_revision,revoked_at FROM servers WHERE id=$1 FOR UPDATE`, v.Wire.ServerID).Scan(&owner, &key, &generation, &credential, &policy, &revoked)
		exists := e == nil
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return empty, e
		}
		if exists && (owner.Valid || !revoked.Valid || key != v.Wire.PublicKey) {
			return empty, claims.ErrStale
		}
		if generation == int64(^uint64(0)>>1) || credential == int64(^uint64(0)>>1) || policy == int64(^uint64(0)>>1) {
			return empty, claims.ErrStale
		}
		generation++
		credential++
		policy++
		var name, profile string
		if e = tx.QueryRowContext(ctx, `SELECT server_name,owner_profile_id FROM claim_operations WHERE id=$1`, v.Wire.OperationID).Scan(&name, &profile); e != nil {
			return empty, e
		}
		if !exists {
			_, e = tx.ExecContext(ctx, `INSERT INTO servers(id,owner_account_id,name,base_url,public_key,token_hash,token_ciphertext,claim_generation,credential_generation) VALUES($1,$2,$3,NULL,$4,$5,$6,$7,$8)`, v.Wire.ServerID, v.Wire.AccountID, name, v.Wire.PublicKey, material.Hash, material.Ciphertext, generation, credential)
		} else {
			// These are generation-local delivery/idempotency projections, not claim
			// receipts. Retire them under the current server lock before installing B.
			// Old wake completions are fenced by their removed lease token. Immutable
			// claim operations/cancellations and erasure jobs are deliberately retained.
			for _, table := range []string{"mutation_receipts", "wake_state"} {
				if _, e = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE server_id=$1", v.Wire.ServerID); e != nil {
					return empty, e
				}
			}
			_, e = tx.ExecContext(ctx, `UPDATE servers SET owner_account_id=$2,custody_released=false,name=$3,base_url=NULL,token_hash=$4,token_ciphertext=$5,claim_generation=$6,credential_generation=$7,revoked_at=NULL,retired_reason='',replaced_by=NULL,last_seen_at=clock_timestamp(),policy_revision=$8,policy_issued_at=clock_timestamp(),local_revision=0,endpoint_verified=false,last_heartbeat_at=NULL,presence_expires_at=NULL WHERE id=$1`, v.Wire.ServerID, v.Wire.AccountID, name, material.Hash, material.Ciphertext, generation, credential, policy)
		}
		if e != nil {
			return empty, e
		}
		var predecessor sql.NullString
		var oldClaim, oldCredential sql.NullInt64
		if e = tx.QueryRowContext(ctx, `SELECT replaces_server_id,replaces_claim_generation,replaces_credential_generation FROM claim_operations WHERE id=$1`, v.Wire.OperationID).Scan(&predecessor, &oldClaim, &oldCredential); e != nil {
			return empty, e
		}
		if predecessor.Valid {
			// Invitations are pending consent, not copied access. The successor's
			// libraries are new identities: grant no old library IDs automatically.
			if _, e = tx.ExecContext(ctx, `INSERT INTO invitations(id,server_id,owner_id,recipient_id,operation_id,request_hash,allowed_libraries,expected_policy_revision,expected_local_revision,expires_at) SELECT 'inv_'||md5($1||m.account_id),$1,$2,m.account_id,'succession_'||md5($1||m.account_id),$4,'{}',$5,0,clock_timestamp()+interval '7 days' FROM memberships m WHERE m.server_id=$3 AND m.account_id<>$2 AND NOT m.revoked GROUP BY m.account_id`, v.Wire.ServerID, v.Wire.AccountID, predecessor.String, material.Hash, policy); e != nil {
				return empty, e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE servers SET name=(SELECT name FROM servers WHERE id=$2) WHERE id=$1`, v.Wire.ServerID, predecessor.String); e != nil {
				return empty, e
			}
			if e = retireServerTx(ctx, tx, v.Wire.AccountID, predecessor.String, oldClaim.Int64, oldCredential.Int64, "replaced", v.Wire.ServerID); e != nil {
				return empty, e
			}
		}
		// A revoked prior claim cannot carry member grants/restrictions into the new
		// owner projection. Cancellation already revoked every previous grant.
		if _, e = tx.ExecContext(ctx, `DELETE FROM restrictions WHERE server_id=$1 AND account_id=$2 AND profile_id=$3`, v.Wire.ServerID, v.Wire.AccountID, profile); e != nil {
			return empty, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO memberships(server_id,account_id,profile_id,role) VALUES($1,$2,$3,'owner') ON CONFLICT(server_id,account_id,profile_id) DO UPDATE SET role='owner',revoked=false,allowed_libraries='{}'`, v.Wire.ServerID, v.Wire.AccountID, profile); e != nil {
			return empty, e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE claim_operations SET state='committed_pending_install',claim_generation=$2,credential_generation=$3,committed_token_hash=$4 WHERE id=$1`, v.Wire.OperationID, generation, credential, material.Hash); e != nil {
			return empty, e
		}
		// The owner's devices find the new server at their next list check.
		if e = bumpServerListVersionsTx(ctx, tx, []string{v.Wire.AccountID}); e != nil {
			return empty, e
		}
	}
	out, _, e := currentClaimResult(ctx, tx, v)
	if e != nil {
		return empty, e
	}
	if e = consumeClaimNonce(ctx, tx, v, c); e != nil {
		return empty, e
	}
	if e = audit(ctx, tx, v.Wire.AccountID, "claim.commit", v.Wire.OperationID, v.Wire.ApprovalRevision); e != nil {
		return empty, e
	}
	if _, e = claimDecisionTime(ctx, tx, c, v.ExpiresAt); e != nil {
		return empty, e
	}
	if e = tx.Commit(); e != nil {
		return empty, e
	}
	return out, nil
}
func (s *Store) ResultClaim(ctx context.Context, v claims.Verified) (claims.Commit, []byte, error) {
	var empty claims.Commit
	if v.Wire.Kind != claims.ClaimProof || v.Wire.Purpose != claims.Result {
		return empty, nil, claims.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return empty, nil, e
	}
	defer tx.Rollback()
	c, e := lockClaim(ctx, tx, v)
	if e != nil {
		return empty, nil, e
	}
	if c.state != "committed_pending_install" {
		return empty, nil, claims.ErrStale
	}
	if e = lockClaimServer(ctx, tx, v.Wire.ServerID); e != nil {
		return empty, nil, e
	}
	out, cipher, e := currentClaimResult(ctx, tx, v)
	if e != nil {
		return empty, nil, e
	}
	if e = consumeClaimNonce(ctx, tx, v, c); e != nil {
		return empty, nil, e
	}
	if _, e = claimDecisionTime(ctx, tx, c, v.ExpiresAt); e != nil {
		return empty, nil, e
	}
	if e = tx.Commit(); e != nil {
		return empty, nil, e
	}
	return out, cipher, nil
}
