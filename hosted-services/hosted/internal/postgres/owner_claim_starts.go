package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
)

// Owner starts retain immutable signed proofs, while every decision uses typed
// columns and live account, family, installed credential and route authority.
type ownerStartRow struct {
	id, operation, server, address, phase, username, registration string
	key, claimHash, startHash, stateHash, challengeHash           []byte
	local                                                         int64
	codeChallenge, installation                                   string
	issued, expiry                                                time.Time
	a                                                             identity.CurrentFamilyExpectation
	t                                                             returnTuple
	completed, consumed                                           sql.NullTime
}

func (ownerStartRow) String() string   { return "[owner start redacted]" }
func (ownerStartRow) GoString() string { return "[owner start redacted]" }
func (r ownerStartRow) liveAt(now time.Time) bool {
	return r.expiry.After(now) && (r.phase == "prepared" || r.phase == "approved" || r.phase == "installed" || r.phase == "completed")
}
func sameOwnerFamily(a, b identity.CurrentFamilyExpectation) bool {
	return a.AccountID == b.AccountID && a.FamilyID == b.FamilyID && a.AuthEpoch == b.AuthEpoch && a.FamilyGeneration == b.FamilyGeneration && a.Mode == b.Mode && equalSecret(a.TokenHash, b.TokenHash)
}
func readOwnerStart(ctx context.Context, tx *sql.Tx, id string, operation, lock bool) (ownerStartRow, error) {
	var r ownerStartRow
	column := "transaction_id"
	if operation {
		column = "operation_id"
	}
	query := `SELECT transaction_id,operation_id,server_id,public_key,local_generation,claim_hash,start_hash,original_address,phase,state_hash,code_challenge,installation_id,server_challenge_hash,issued_at,expires_at,COALESCE(approved_account_id,''),COALESCE(approved_username,''),COALESCE(family_id,''),access_hash,COALESCE(account_epoch,0),COALESCE(family_generation,0),COALESCE(captured_session_mode,''),COALESCE(claim_generation,0),COALESCE(credential_generation,0),COALESCE(route_generation,0),COALESCE(registration_id,''),COALESCE(registered_https_origin,''),current_owner_account_id,previous_owner_role,completed_at,consumed_at FROM owner_claim_starts WHERE ` + column + `=$1`
	if lock {
		query += " FOR UPDATE"
	}
	e := tx.QueryRowContext(ctx, query, id).Scan(&r.id, &r.operation, &r.server, &r.key, &r.local, &r.claimHash, &r.startHash, &r.address, &r.phase, &r.stateHash, &r.codeChallenge, &r.installation, &r.challengeHash, &r.issued, &r.expiry, &r.a.AccountID, &r.username, &r.a.FamilyID, &r.a.TokenHash, &r.a.AuthEpoch, &r.a.FamilyGeneration, &r.a.Mode, &r.t.claim, &r.t.credential, &r.t.route, &r.registration, &r.t.origin, &r.t.current, &r.t.previous, &r.completed, &r.consumed)
	r.t.operation, r.t.purpose = r.operation, "switch"
	return r, e
}

// RegisterOwnerStart verifies both complete signatures and freezes the original
// canonical claim hash in the same transaction that creates its approval code.
func (s *Store) RegisterOwnerStart(ctx context.Context, original, signed claimweb.Signed, audience string, seal []byte) (claimweb.Pending, error) {
	return s.RegisterOwnerStartWithCount(ctx, original, signed, nil, audience, seal)
}

// RegisterOwnerStartWithCount freezes an optional preview proof only with the
// first original/start insert. Count failures never replace parent authority.
// The nullable proof columns are supplied by the canonical baseline cohort.
func (s *Store) RegisterOwnerStartWithCount(ctx context.Context, original, signed claimweb.Signed, count *claimweb.Signed, audience string, seal []byte) (claimweb.Pending, error) {
	var empty claimweb.Pending
	q, start, e := claimweb.VerifyOwnerStart(original, signed, audience, time.Now())
	if e != nil || q.ReturnRegistrationID != "" {
		return empty, claimweb.ErrInvalid
	}
	claimRaw, _ := base64.RawURLEncoding.DecodeString(original.Payload)
	startRaw, _ := base64.RawURLEncoding.DecodeString(signed.Payload)
	claimSig, _ := base64.RawURLEncoding.DecodeString(original.Signature)
	startSig, _ := base64.RawURLEncoding.DecodeString(signed.Signature)
	var countPayload, countSignature []byte
	if count != nil {
		// Copy the optional strings before transaction waits; an invalid count
		// is absent, including signed overflow. Zero remains a verified fact.
		captured := *count
		if _, err := claimweb.VerifyOwnerSwitchCount(original, signed, captured, audience, time.Now()); err == nil {
			countPayload, _ = base64.RawURLEncoding.DecodeString(captured.Payload)
			countSignature, _ = base64.RawURLEncoding.DecodeString(captured.Signature)
		}
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return empty, e
	}
	defer tx.Rollback()
	p, e := registerWebClaimTx(ctx, tx, q, seal)
	if e != nil {
		return empty, e
	}
	var existingOperation, existingStart bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM claim_operations WHERE id=$1),EXISTS(SELECT 1 FROM owner_claim_starts WHERE operation_id=$1)`, q.OperationID).Scan(&existingOperation, &existingStart); e != nil {
		return empty, e
	}
	if existingOperation && !existingStart {
		return empty, claims.ErrStale
	}
	// PostgreSQL timestamps have microsecond precision. Round the deadline down,
	// never past either original signed deadline.
	expiry := start.ExpiresAt.Truncate(time.Microsecond)
	issued := start.IssuedAt.Truncate(time.Microsecond)
	query := `INSERT INTO owner_claim_starts(transaction_id,operation_id,server_id,public_key,local_generation,claim_hash,start_hash,original_address,purpose,current_owner_account_id,previous_owner_role,state_hash,code_challenge,installation_id,server_challenge_hash,issued_at,expires_at,original_claim_payload,original_claim_signature,start_payload,start_signature) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'switch',$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) ON CONFLICT(operation_id) DO NOTHING`
	args := []any{security.RandomID("ocs_"), q.OperationID, q.ServerID, mustWebPublicKey(q.PublicKey), q.LocalGeneration, security.Hash(string(claimRaw)), security.Hash(string(startRaw)), q.Address, q.CurrentOwnerAccountID, q.PreviousOwnerRole, security.Hash(start.State), start.CodeChallenge, start.InstallationID, security.Hash(start.ServerChallenge), issued, expiry, claimRaw, claimSig, startRaw, startSig}
	if count != nil {
		query = `INSERT INTO owner_claim_starts(transaction_id,operation_id,server_id,public_key,local_generation,claim_hash,start_hash,original_address,purpose,current_owner_account_id,previous_owner_role,state_hash,code_challenge,installation_id,server_challenge_hash,issued_at,expires_at,original_claim_payload,original_claim_signature,start_payload,start_signature,switch_count_payload,switch_count_signature) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'switch',$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22) ON CONFLICT(operation_id) DO NOTHING`
		args = append(args, countPayload, countSignature)
	}
	_, e = tx.ExecContext(ctx, query, args...)
	if e != nil {
		return empty, e
	}
	r, e := readOwnerStart(ctx, tx, q.OperationID, true, true)
	if e != nil || !r.liveAt(time.Now()) || !equalSecret(r.claimHash, security.Hash(string(claimRaw))) || !equalSecret(r.startHash, security.Hash(string(startRaw))) {
		return empty, claims.ErrStale
	}
	var live bool
	if e = tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() FROM owner_claim_starts WHERE transaction_id=$1`, r.id).Scan(&live); e != nil {
		return empty, e
	}
	if !live {
		return empty, claims.ErrStale
	}
	p.OwnerTransactionID, p.ExpiresAt = r.id, r.expiry
	return p, tx.Commit()
}

func bindOwnerApprovalTx(ctx context.Context, tx *sql.Tx, a identity.CurrentFamilyExpectation, q claimweb.Request, approval *claims.Approval) error {
	r, e := readOwnerStart(ctx, tx, q.OperationID, true, true)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	raw, _ := json.Marshal(q)
	if r.phase != "prepared" || !equalSecret(r.claimHash, security.Hash(string(raw))) || r.server != q.ServerID || r.local != q.LocalGeneration || !bytes.Equal(r.key, mustWebPublicKey(q.PublicKey)) || r.t.current != q.CurrentOwnerAccountID || r.t.previous != q.PreviousOwnerRole {
		return claims.ErrStale
	}
	expires, e := time.Parse(time.RFC3339Nano, approval.ExpiresAt)
	if e != nil {
		return e
	}
	if r.expiry.Before(expires) {
		expires = r.expiry
	}
	result, e := tx.ExecContext(ctx, `UPDATE claim_approvals SET expires_at=LEAST(expires_at,$2) WHERE operation_id=$1 AND $2>clock_timestamp()`, q.OperationID, expires)
	if e != nil {
		return e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return claims.ErrStale
	}
	result, e = tx.ExecContext(ctx, `UPDATE owner_claim_starts SET phase='approved',approved_account_id=$2,approved_username=$3,family_id=$4,access_hash=$5,account_epoch=$6,family_generation=$7,captured_session_mode=$8 WHERE transaction_id=$1 AND phase='prepared' AND expires_at>clock_timestamp()`, r.id, a.AccountID, approval.TargetUsername, a.FamilyID, a.TokenHash, a.AuthEpoch, a.FamilyGeneration, a.Mode)
	if e != nil {
		return e
	}
	n, _ = result.RowsAffected()
	if n != 1 {
		return claims.ErrStale
	}
	approval.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	return nil
}

// ownerTupleTx also checks the original switch intent and approved username;
// a current registration alone cannot authorize an owner continuation.
func ownerTupleTx(ctx context.Context, tx *sql.Tx, r ownerStartRow) (returnTuple, error) {
	t, e := returnTupleTx(ctx, tx, r.a.AccountID, r.server, r.operation, "switch", r.registration)
	if e != nil {
		return t, e
	}
	var exact bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM claim_operations o JOIN accounts a ON a.id=o.account_id WHERE o.id=$1 AND o.server_id=$2 AND o.account_id=$3 AND o.public_key=$4 AND o.local_generation=$5 AND o.purpose='switch' AND o.current_owner_account_id=$6 AND o.previous_owner_role=$7 AND o.return_registration_id='' AND o.target_username=$8 AND a.username=$8)`, r.operation, r.server, r.a.AccountID, r.key, r.local, r.t.current, r.t.previous, r.username).Scan(&exact)
	if e != nil {
		return t, e
	}
	if !exact || t.current != r.t.current || t.previous != r.t.previous {
		return t, serverreturn.ErrInvalid
	}
	return t, nil
}

// bindOwnerRegistrationTx accepts only the resolver's current verified HTTPS
// fact. A later generation invalidates the attempt instead of retargeting it.
func bindOwnerRegistrationTx(ctx context.Context, tx *sql.Tx, a directory.ServerAuthority, registration serverreturn.Registration, t returnTuple) error {
	r, e := readOwnerStart(ctx, tx, a.OperationID, true, true)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if !r.liveAt(time.Now()) {
		return nil
	}
	if r.server != a.ServerID || r.a.AccountID != a.AccountID || !bytes.Equal(r.key, a.PublicKey) {
		return serverreturn.ErrInvalid
	}
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, r.a)
	if e != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE owner_claim_starts SET phase='cancelled' WHERE transaction_id=$1 AND phase<>'consumed'`, r.id); err != nil {
			return err
		}
		return nil
	}
	if r.registration != "" {
		if r.registration != registration.ID || r.t.claim != t.claim || r.t.credential != t.credential || r.t.route != t.route || r.t.origin != registration.Origin {
			_, e = tx.ExecContext(ctx, `UPDATE owner_claim_starts SET phase='superseded' WHERE transaction_id=$1 AND phase<>'consumed'`, r.id)
			return e
		}
		if _, e = ownerTupleTx(ctx, tx, r); e != nil {
			return e
		}
		return returnFresh(ctx, tx, facts)
	}
	if r.phase != "approved" {
		return serverreturn.ErrInvalid
	}
	r.registration, r.t.claim, r.t.credential, r.t.route, r.t.origin = registration.ID, t.claim, t.credential, t.route, registration.Origin
	if _, e = ownerTupleTx(ctx, tx, r); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE owner_claim_starts SET phase='installed',claim_generation=$2,credential_generation=$3,route_generation=$4,registration_id=$5,registered_https_origin=$6 WHERE transaction_id=$1 AND phase='approved' AND expires_at>clock_timestamp()`, r.id, t.claim, t.credential, t.route, registration.ID, registration.Origin)
	if e != nil {
		return e
	}
	return returnFresh(ctx, tx, facts)
}
