package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
)

func (s *Store) CaptureServerAuthority(ctx context.Context, hash []byte) (directory.ServerAuthority, error) {
	var a directory.ServerAuthority
	if len(hash) != 32 {
		return a, directory.ErrDenied
	}
	e := s.DB.QueryRowContext(ctx, `SELECT s.id,s.owner_account_id,o.id,s.claim_generation,s.credential_generation,o.public_key,o.origin_account_id FROM servers s JOIN claim_operations o ON o.server_id=s.id AND o.account_id=s.owner_account_id AND o.claim_generation=s.claim_generation AND o.credential_generation=s.credential_generation AND o.committed_token_hash=s.token_hash WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND o.state='installed'`, hash).Scan(&a.ServerID, &a.AccountID, &a.OperationID, &a.ClaimGeneration, &a.CredentialGeneration, &a.PublicKey, &a.ClaimAccountID)
	if errors.Is(e, sql.ErrNoRows) {
		return a, directory.ErrDenied
	}
	if e != nil {
		return a, e
	}
	derived, err := claims.ServerID(a.PublicKey)
	if err != nil || derived != a.ServerID {
		return directory.ServerAuthority{}, directory.ErrDenied
	}
	a.TokenHash = append([]byte(nil), hash...)
	return a, nil
}

// lockServerAuthorityOwnerTx establishes account -> immutable operation order.
// Erasure acknowledgment then locks its deletion parent before the server;
// ordinary consumers can immediately call checkServerAuthorityTx.
func lockServerAuthorityOwnerTx(ctx context.Context, tx *sql.Tx, a directory.ServerAuthority, also ...string) error {
	if tx == nil || !a.Valid() {
		return directory.ErrDenied
	}
	// Every account row this operation will lock, in id order and in one
	// statement, before any other lock: a custody move locks the custodian and
	// the new owner, and any other order could cross a path that locks the
	// same two accounts the other way round (INT N5).
	accounts := append([]string{a.AccountID}, also...)
	rows, e := tx.QueryContext(ctx, `SELECT id FROM accounts WHERE id=ANY($1) ORDER BY id FOR UPDATE`, accounts)
	if e != nil {
		return e
	}
	found := false
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		found = found || id == a.AccountID
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	if !found {
		return directory.ErrDenied
	}
	var owner sql.NullString
	var server, state, origin string
	var claim, credential int64
	var hash, key []byte
	e = tx.QueryRowContext(ctx, `SELECT account_id,server_id,state,claim_generation,credential_generation,committed_token_hash,public_key,origin_account_id FROM claim_operations WHERE id=$1 FOR SHARE`, a.OperationID).Scan(&owner, &server, &state, &claim, &credential, &hash, &key, &origin)
	if e != nil {
		return serverAuthorityError(e)
	}
	if (a.ClaimAccountID != "" && origin != a.ClaimAccountID) || !owner.Valid || owner.String != a.AccountID || server != a.ServerID || state != "installed" || claim != a.ClaimGeneration || credential != a.CredentialGeneration || !bytes.Equal(hash, a.TokenHash) || !bytes.Equal(key, a.PublicKey) {
		return directory.ErrDenied
	}
	return nil
}
func checkServerAuthorityTx(ctx context.Context, tx *sql.Tx, a directory.ServerAuthority) error {
	var owner sql.NullString
	var key string
	var claim, credential int64
	var hash []byte
	var revoked sql.NullTime
	e := tx.QueryRowContext(ctx, `SELECT owner_account_id,claim_generation,credential_generation,token_hash,revoked_at,public_key FROM servers WHERE id=$1 FOR UPDATE`, a.ServerID).Scan(&owner, &claim, &credential, &hash, &revoked, &key)
	if e != nil {
		return serverAuthorityError(e)
	}
	if !owner.Valid || owner.String != a.AccountID || claim != a.ClaimGeneration || credential != a.CredentialGeneration || !bytes.Equal(hash, a.TokenHash) || revoked.Valid || key != base64.RawURLEncoding.EncodeToString(a.PublicKey) {
		return directory.ErrDenied
	}
	return ctx.Err()
}
func (s *Store) beginServerOperation(ctx context.Context, a directory.ServerAuthority, also ...string) (*sql.Tx, error) {
	return s.beginServerOperationWithSigningState(ctx, a, false, also...)
}

func (s *Store) beginServerOperationWithSigningState(ctx context.Context, a directory.ServerAuthority, signing bool, also ...string) (*sql.Tx, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	// Take signing state before account/server rows, matching the rollout
	// worker's signing-state -> server order. Hold it through consent use.
	if signing {
		var revision string
		if e = tx.QueryRowContext(ctx, `SELECT revision::text FROM hosted_signing_state WHERE singleton FOR SHARE`).Scan(&revision); e != nil {
			tx.Rollback()
			return nil, e
		}
	}
	if e = lockServerAuthorityOwnerTx(ctx, tx, a, also...); e == nil {
		e = checkServerAuthorityTx(ctx, tx, a)
	}
	if e != nil {
		_ = tx.Rollback()
		return nil, e
	}
	// Existing authenticated traffic renews coarse presence at most four writes/day.
	if _, e = tx.ExecContext(ctx, `UPDATE servers SET last_seen_at=clock_timestamp(),presence_expires_at=clock_timestamp()+CASE WHEN endpoint_verified THEN interval '8 days' ELSE interval '2 days' END WHERE id=$1 AND last_seen_at<clock_timestamp()-interval '6 hours'`, a.ServerID); e != nil {
		tx.Rollback()
		return nil, e
	}
	return tx, nil
}
func serverAuthorityError(e error) error {
	if errors.Is(e, sql.ErrNoRows) {
		return directory.ErrDenied
	}
	return e
}
