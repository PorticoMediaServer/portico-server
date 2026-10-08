package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/identity"
	"time"
)

type lockedClaim struct {
	approval claimApprovalRow
	family   identity.CurrentFamilyFacts
	state    string
}

func lockClaim(ctx context.Context, tx *sql.Tx, v claims.Verified) (lockedClaim, error) {
	var empty lockedClaim
	preliminary, e := readClaimApproval(ctx, tx, v.Wire.OperationID, false)
	if e != nil {
		return empty, claimAuthError(e)
	}
	if preliminary.expected.AccountID != v.Wire.AccountID {
		return empty, claims.ErrStale
	}
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, preliminary.expected)
	if e != nil {
		return empty, claimAuthError(e)
	}
	current, e := readClaimApproval(ctx, tx, v.Wire.OperationID, true)
	if e != nil {
		return empty, claimAuthError(e)
	}
	if !sameClaimApproval(preliminary, current) || current.revision != v.Wire.ApprovalRevision {
		return empty, claims.ErrApprovalRequired
	}
	var server, account, state string
	var key []byte
	var generation int64
	if e = tx.QueryRowContext(ctx, `SELECT server_id,account_id,public_key,local_generation,state FROM claim_operations WHERE id=$1 FOR UPDATE`, v.Wire.OperationID).Scan(&server, &account, &key, &generation, &state); e != nil {
		return empty, claimAuthError(e)
	}
	if server != v.Wire.ServerID || account != v.Wire.AccountID || base64.RawURLEncoding.EncodeToString(key) != v.Wire.PublicKey || generation != v.Wire.LocalGeneration {
		return empty, claims.ErrStale
	}
	if state == "cancelled" {
		return empty, claims.ErrCancelled
	}
	if state == "installed" {
		return empty, claims.ErrStale
	}
	out := lockedClaim{current, facts, state}
	if _, e = claimDecisionTime(ctx, tx, out, v.ExpiresAt); e != nil {
		return empty, e
	}
	return out, nil
}
func claimDecisionTime(ctx context.Context, tx *sql.Tx, c lockedClaim, proofExpiry time.Time) (time.Time, error) {
	var now time.Time
	if e := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return now, e
	}
	if !c.family.UnexpiredAt(now) || !c.approval.expiry.After(now) {
		return now, claims.ErrApprovalRequired
	}
	if !proofExpiry.After(now) {
		return now, claims.ErrStale
	}
	return now, nil
}
func claimAuthError(e error) error {
	if errors.Is(e, identity.ErrCredentials) || errors.Is(e, sql.ErrNoRows) {
		return claims.ErrApprovalRequired
	}
	return e
}

// ClaimNonce receives freshly generated random material from the service after
// signature verification. Quotas/replay are serialized by the exact account,
// approval and operation locks; PostgreSQL clock owns expiry.
func (s *Store) ClaimNonce(ctx context.Context, v claims.Verified, id string, nonce []byte) (claims.Challenge, error) {
	var empty claims.Challenge
	if v.Wire.Kind != claims.NonceRequest || (v.Wire.Purpose != claims.Finalize && v.Wire.Purpose != claims.Result) || len(nonce) != 32 || len(id) == 0 || len(id) > 128 {
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
	if v.Wire.Purpose == claims.Result && c.state != "committed_pending_install" {
		return empty, claims.ErrStale
	}
	var existing claims.Challenge
	var digest, savedNonce []byte
	var revision int64
	var consumed sql.NullTime
	e = tx.QueryRowContext(ctx, `SELECT id,request_digest,nonce,approval_revision,issued_at,expires_at,consumed_at FROM claim_nonces WHERE operation_id=$1 AND purpose=$2 AND request_id=$3 FOR UPDATE`, v.Wire.OperationID, v.Wire.Purpose, v.Wire.RequestID).Scan(&existing.NonceID, &digest, &savedNonce, &revision, &existing.IssuedAt, &existing.ExpiresAt, &consumed)
	if e == nil {
		if !bytes.Equal(digest, v.Digest[:]) || revision != v.Wire.ApprovalRevision || consumed.Valid {
			return empty, claims.ErrStale
		}
		if _, e = claimDecisionTime(ctx, tx, c, existing.ExpiresAt); e != nil {
			return empty, e
		}
		existing.RequestDigest = base64.RawURLEncoding.EncodeToString(digest)
		existing.Nonce = base64.RawURLEncoding.EncodeToString(savedNonce)
		if e = tx.Commit(); e != nil {
			return empty, e
		}
		return existing, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return empty, e
	}
	var active, recent int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE consumed_at IS NULL AND expires_at>clock_timestamp() AND approval_revision=$2),count(*) FILTER(WHERE issued_at>clock_timestamp()-interval '1 hour') FROM claim_nonces WHERE operation_id=$1`, v.Wire.OperationID, v.Wire.ApprovalRevision).Scan(&active, &recent); e != nil {
		return empty, e
	}
	if active >= 4 || recent >= 64 {
		return empty, claims.ErrCapacity
	}
	now, e := claimDecisionTime(ctx, tx, c, v.ExpiresAt)
	if e != nil {
		return empty, e
	}
	expiry := now.Add(30 * time.Second)
	if v.ExpiresAt.Before(expiry) {
		expiry = v.ExpiresAt
	}
	if c.approval.expiry.Before(expiry) {
		expiry = c.approval.expiry
	}
	// PostgreSQL stores microseconds. Conservatively truncate the signed bound,
	// then return the stored values so first delivery and exact retry agree.
	expiry = expiry.UTC().Truncate(time.Microsecond)
	if !expiry.After(now) {
		return empty, claims.ErrStale
	}
	if e = tx.QueryRowContext(ctx, `INSERT INTO claim_nonces(id,operation_id,approval_revision,purpose,request_id,request_digest,nonce,issued_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING issued_at,expires_at`, id, v.Wire.OperationID, v.Wire.ApprovalRevision, v.Wire.Purpose, v.Wire.RequestID, v.Digest[:], nonce, now, expiry).Scan(&now, &expiry); e != nil {
		return empty, e
	}
	if _, e = claimDecisionTime(ctx, tx, c, expiry); e != nil {
		return empty, e
	}
	if e = tx.Commit(); e != nil {
		return empty, e
	}
	return claims.Challenge{RequestDigest: base64.RawURLEncoding.EncodeToString(v.Digest[:]), NonceID: id, Nonce: base64.RawURLEncoding.EncodeToString(nonce), IssuedAt: now, ExpiresAt: expiry}, nil
}
