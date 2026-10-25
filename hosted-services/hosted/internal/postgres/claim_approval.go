package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/identity"
	"time"
)

type claimApprovalRow struct {
	expected       identity.CurrentFamilyExpectation
	revision       int64
	issued, expiry time.Time
}

func readClaimApproval(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, op string, locked bool) (claimApprovalRow, error) {
	var a claimApprovalRow
	query := `SELECT account_id,family_id,access_hash,account_epoch,family_generation,session_mode,revision,issued_at,expires_at FROM claim_approvals WHERE operation_id=$1`
	if locked {
		query += ` FOR UPDATE`
	}
	err := q.QueryRowContext(ctx, query, op).Scan(&a.expected.AccountID, &a.expected.FamilyID, &a.expected.TokenHash, &a.expected.AuthEpoch, &a.expected.FamilyGeneration, &a.expected.Mode, &a.revision, &a.issued, &a.expiry)
	return a, err
}
func sameClaimApproval(a, b claimApprovalRow) bool {
	return a.expected.AccountID == b.expected.AccountID && a.expected.FamilyID == b.expected.FamilyID && bytes.Equal(a.expected.TokenHash, b.expected.TokenHash) && a.expected.AuthEpoch == b.expected.AuthEpoch && a.expected.FamilyGeneration == b.expected.FamilyGeneration && a.expected.Mode == b.expected.Mode && a.revision == b.revision && a.issued.Equal(b.issued) && a.expiry.Equal(b.expiry)
}

// ApproveClaim is the sole positive account approval consumer. Expected authority
// was captured by the server, never decoded from an account request body.
func (s *Store) ApproveClaim(ctx context.Context, expected identity.CurrentFamilyExpectation, in claims.ApprovalInput, audience string) (claims.Approval, error) {
	var empty claims.Approval
	if !in.Valid() {
		return empty, claims.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return empty, e
	}
	defer tx.Rollback()
	// Serialize with pending registration: an owner operation is approved only
	// by its code consumer, which binds the actual family and frozen start.
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('claim-web:'||$1,0))`, in.ServerID); e != nil {
		return empty, e
	}
	var ownerStart bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM owner_claim_starts WHERE operation_id=$1) OR EXISTS(SELECT 1 FROM setup_claim_starts WHERE operation_id=$1)`, in.OperationID).Scan(&ownerStart); e != nil {
		return empty, e
	}
	if ownerStart {
		return empty, claims.ErrStale
	}
	out, e := s.approveClaimTx(ctx, tx, expected, in, audience)
	if e != nil {
		return empty, e
	}
	return out, tx.Commit()
}
func (s *Store) approveClaimTx(ctx context.Context, tx *sql.Tx, expected identity.CurrentFamilyExpectation, in claims.ApprovalInput, audience string) (claims.Approval, error) {
	var empty claims.Approval
	if !in.Valid() {
		return empty, claims.ErrInvalid
	}
	if e := lockClaimAccounts(ctx, tx, expected.AccountID, in.ServerID); e != nil {
		return empty, e
	}
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, expected)
	if e != nil {
		return empty, e
	}
	// Serialize operation insertion on the already locked account. A second
	// account cannot take over the same operation due to its immutable PK/binding.
	old, e := readClaimApproval(ctx, tx, in.OperationID, true)
	exists := e == nil
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return empty, e
	}
	var targetUsername string
	if in.Purpose == "change-owner" || in.Purpose == "switch" {
		if e = tx.QueryRowContext(ctx, `SELECT username FROM accounts WHERE id=$1`, expected.AccountID).Scan(&targetUsername); e != nil {
			return empty, e
		}
		if !identity.ValidUsername(targetUsername) {
			return empty, claims.ErrInvalid
		}
	}
	digest := in.IntentDigest(expected.AccountID, audience)
	if exists {
		if old.expected.AccountID != expected.AccountID || (old.revision != in.ExpectedRevision && old.revision != in.ExpectedRevision+1) {
			return empty, claims.ErrStale
		}
		var saved []byte
		var state, savedUsername string
		if e = tx.QueryRowContext(ctx, `SELECT intent_digest,state,target_username FROM claim_operations WHERE id=$1 FOR UPDATE`, in.OperationID).Scan(&saved, &state, &savedUsername); e != nil {
			return empty, e
		}
		if !bytes.Equal(saved, digest[:]) || savedUsername != targetUsername {
			return empty, claims.ErrStale
		}
		if state == "cancelled" {
			return empty, claims.ErrCancelled
		}
		// Installed delivery is not reopened by a routine account approval. An
		// explicit local-owner recovery journey is a separate required consumer.
		if state == "installed" {
			return empty, claims.ErrStale
		}
	} else {
		if in.ExpectedRevision != 0 {
			return empty, claims.ErrStale
		}
		var active, recent int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE o.state='approved' AND a.expires_at>clock_timestamp()),count(*) FILTER(WHERE o.created_at>clock_timestamp()-interval '1 hour') FROM claim_operations o LEFT JOIN claim_approvals a ON a.operation_id=o.id WHERE o.account_id=$1`, expected.AccountID).Scan(&active, &recent); e != nil {
			return empty, e
		}
		if active >= 5 || recent >= 20 {
			return empty, claims.ErrCapacity
		}
		key, _ := base64.RawURLEncoding.DecodeString(in.PublicKey)
		if _, e = tx.ExecContext(ctx, `INSERT INTO claim_operations(id,server_id,account_id,public_key,local_generation,server_name,intent_digest,state,purpose,current_owner_account_id,previous_owner_role,return_registration_id,target_username) VALUES($1,$2,$3,$4,$5,$6,$7,'approved',$8,$9,$10,$11,$12)`, in.OperationID, in.ServerID, expected.AccountID, key, in.LocalGeneration, in.Name, digest[:], in.Purpose, in.CurrentOwnerAccountID, in.PreviousOwnerRole, in.ReturnRegistrationID, targetUsername); e != nil {
			return empty, e
		}
	}
	if in.ReplacesServerID != "" {
		var claim, credential int64
		if e = tx.QueryRowContext(ctx, `SELECT claim_generation,credential_generation FROM servers WHERE id=$1 AND owner_account_id=$2 AND revoked_at IS NULL FOR SHARE`, in.ReplacesServerID, expected.AccountID).Scan(&claim, &credential); e != nil {
			return empty, claims.ErrStale
		}
		if _, e = tx.ExecContext(ctx, `UPDATE claim_operations SET replaces_server_id=$2,replaces_claim_generation=$3,replaces_credential_generation=$4 WHERE id=$1 AND (replaces_server_id IS NULL OR (replaces_server_id=$2 AND replaces_claim_generation=$3 AND replaces_credential_generation=$4))`, in.OperationID, in.ReplacesServerID, claim, credential); e != nil {
			return empty, e
		}
	}
	var existingOwner sql.NullString
	var existingKey string
	e = tx.QueryRowContext(ctx, `SELECT owner_account_id,public_key FROM servers WHERE id=$1 FOR SHARE`, in.ServerID).Scan(&existingOwner, &existingKey)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return empty, e
	}
	if e == nil && existingKey != in.PublicKey {
		return empty, claims.ErrStale
	}
	if in.Purpose == "setup" && e == nil && existingOwner.Valid {
		return empty, claims.ErrStale
	}
	if in.Purpose != "setup" && !(in.Purpose == "switch" && in.CurrentOwnerAccountID == "" && (e != nil || !existingOwner.Valid)) && (e != nil || !existingOwner.Valid || existingOwner.String != in.CurrentOwnerAccountID) {
		return empty, claims.ErrStale
	}
	if in.Purpose == "change-owner" && expected.AccountID == in.CurrentOwnerAccountID {
		return empty, claims.ErrStale
	}
	if in.Purpose == "switch" && in.CurrentOwnerAccountID != "" && expected.AccountID != in.CurrentOwnerAccountID {
		return empty, claims.ErrStale
	}

	// The approval echoes the mode frozen in the stored signed start. A v1 start
	// has none and stays a version-2 ordinary switch approval.
	transition := ""
	if in.Purpose == "switch" {
		var startPayload []byte
		if e = tx.QueryRowContext(ctx, `SELECT start_payload FROM owner_claim_starts WHERE operation_id=$1`, in.OperationID).Scan(&startPayload); e != nil {
			return empty, claims.ErrStale
		}
		var start claimweb.OwnerStart
		if json.Unmarshal(startPayload, &start) != nil {
			return empty, claims.ErrInvalid
		}
		transition = start.TransitionMode
	}
	version := "2"
	if transition != "" {
		version = "3"
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return empty, e
	}
	if !facts.UnexpiredAt(now) {
		return empty, claims.ErrApprovalRequired
	}
	if exists && old.revision == in.ExpectedRevision+1 {
		captured := claimApprovalRow{expected: expected, revision: old.revision, issued: old.issued, expiry: old.expiry}
		if !sameClaimApproval(old, captured) || !old.expiry.After(now) {
			return empty, claims.ErrStale
		}
		return claims.Approval{TargetUsername: targetUsername, Kind: "portico.claim.approval", Version: version, Audience: audience, OperationID: in.OperationID, ServerID: in.ServerID, AccountID: expected.AccountID, PublicKey: in.PublicKey, LocalGeneration: in.LocalGeneration, ApprovalRevision: old.revision, Purpose: in.Purpose, CurrentOwnerAccountID: in.CurrentOwnerAccountID, PreviousOwnerRole: in.PreviousOwnerRole, ReturnRegistrationID: in.ReturnRegistrationID, TransitionMode: transition, IssuedAt: old.issued.UTC().Format(time.RFC3339Nano), ExpiresAt: old.expiry.UTC().Format(time.RFC3339Nano)}, nil
	}
	expiry := now.Add(5 * time.Minute)
	for _, bound := range []time.Time{facts.AccessExpiresAt, facts.AbsoluteExpiresAt, facts.IdleExpiresAt} {
		if bound.Before(expiry) {
			expiry = bound
		}
	}
	revision := old.revision + 1
	if revision < 1 {
		return empty, claims.ErrStale
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO claim_approvals(operation_id,account_id,family_id,access_hash,account_epoch,family_generation,session_mode,revision,issued_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(operation_id) DO UPDATE SET family_id=EXCLUDED.family_id,access_hash=EXCLUDED.access_hash,account_epoch=EXCLUDED.account_epoch,family_generation=EXCLUDED.family_generation,session_mode=EXCLUDED.session_mode,revision=EXCLUDED.revision,issued_at=EXCLUDED.issued_at,expires_at=EXCLUDED.expires_at`, in.OperationID, expected.AccountID, expected.FamilyID, expected.TokenHash, expected.AuthEpoch, expected.FamilyGeneration, expected.Mode, revision, now, expiry)
	if e != nil {
		return empty, e
	}
	// Old nonce rows remain replay/quota guards but their revision cannot authorize.
	if e = audit(ctx, tx, expected.AccountID, "claim.approve", in.OperationID, revision); e != nil {
		return empty, e
	}
	var decisionTime time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&decisionTime); e != nil {
		return empty, e
	}
	if !facts.UnexpiredAt(decisionTime) || !expiry.After(decisionTime) {
		return empty, claims.ErrApprovalRequired
	}
	return claims.Approval{TargetUsername: targetUsername, Kind: "portico.claim.approval", Version: version, Audience: audience, OperationID: in.OperationID, ServerID: in.ServerID, AccountID: expected.AccountID, PublicKey: in.PublicKey, LocalGeneration: in.LocalGeneration, ApprovalRevision: revision, Purpose: in.Purpose, CurrentOwnerAccountID: in.CurrentOwnerAccountID, PreviousOwnerRole: in.PreviousOwnerRole, ReturnRegistrationID: in.ReturnRegistrationID, TransitionMode: transition, IssuedAt: now.UTC().Format(time.RFC3339Nano), ExpiresAt: expiry.UTC().Format(time.RFC3339Nano)}, nil
}

// Match the server-operation account lock order before any family/claim locks.
func lockClaimAccounts(ctx context.Context, tx *sql.Tx, account, server string) error {
	rows, e := tx.QueryContext(ctx, `SELECT id FROM accounts WHERE id=$1 OR id=(SELECT owner_account_id FROM servers WHERE id=$2) ORDER BY id FOR UPDATE`, account, server)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return e
		}
	}
	return rows.Err()
}
