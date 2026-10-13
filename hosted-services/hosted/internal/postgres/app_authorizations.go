package postgres

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"

	"portico.local/hosted/internal/appauth"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
)

const appAuthorizationSchema = `
CREATE TABLE app_authorizations(
 id text PRIMARY KEY,
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 family_id text NOT NULL REFERENCES session_families(id) ON DELETE CASCADE,
 account_epoch bigint NOT NULL,
 server_id text NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 server_name text NOT NULL,
 challenge text NOT NULL,
 redirect_uri text NOT NULL,
 state text NOT NULL,
 app_name text NOT NULL,
 code_challenge text NOT NULL,
 code_hash bytea UNIQUE CHECK(code_hash IS NULL OR octet_length(code_hash)=32),
 code_expires_at timestamptz,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX app_authorizations_account ON app_authorizations(account_id,created_at);
CREATE INDEX app_authorizations_expiry ON app_authorizations(expires_at);
`

type appFamily struct {
	account        identity.Account
	id             string
	epoch          int64
	absolute, idle time.Time
}

// appBrowserFamilyTx never rotates or exports the Hosted browser credential.
// Account then family locks serialize logout, deletion, consent and exchange.
func appBrowserFamilyTx(ctx context.Context, tx *sql.Tx, hash []byte) (appFamily, error) {
	var f appFamily
	if len(hash) != 32 {
		return f, identity.ErrCredentials
	}
	e := tx.QueryRowContext(ctx, `SELECT account_id,id FROM session_families WHERE current_refresh_hash=$1 AND mode='browser'`, hash).Scan(&f.account.ID, &f.id)
	if e != nil {
		return f, currentFamilyError(ctx, e)
	}
	e = tx.QueryRowContext(ctx, `SELECT id,username,display_name,auth_epoch FROM accounts WHERE id=$1 FOR UPDATE`, f.account.ID).Scan(&f.account.ID, &f.account.Username, &f.account.DisplayName, &f.epoch)
	if e != nil {
		return f, currentFamilyError(ctx, e)
	}
	var epoch int64
	var revoked sql.NullTime
	var current []byte
	e = tx.QueryRowContext(ctx, `SELECT account_epoch,absolute_expires_at,idle_expires_at,revoked_at,current_refresh_hash FROM session_families WHERE id=$1 AND account_id=$2 AND mode='browser' FOR SHARE`, f.id, f.account.ID).Scan(&epoch, &f.absolute, &f.idle, &revoked, &current)
	if e != nil {
		return f, currentFamilyError(ctx, e)
	}
	if epoch != f.epoch || revoked.Valid || subtle.ConstantTimeCompare(current, hash) != 1 {
		return f, identity.ErrCredentials
	}
	if e = appFamilyFresh(ctx, tx, f); e != nil {
		return f, e
	}
	return f, nil
}
func appFamilyFresh(ctx context.Context, tx *sql.Tx, f appFamily) error {
	var now time.Time
	if e := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return e
	}
	if !f.absolute.After(now) || !f.idle.After(now) {
		return identity.ErrCredentials
	}
	return ctx.Err()
}

func (s *Store) ReviewAppAuthorization(ctx context.Context, refresh []byte, q appauth.Request) (appauth.Review, error) {
	var out appauth.Review
	if e := q.Validate(); e != nil {
		return out, e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	f, e := appBrowserFamilyTx(ctx, tx, refresh)
	if e != nil {
		return out, e
	}
	// Limit every review, including already spent codes, for one minute. Expired
	// entries are swept in bounded maintenance batches rather than anonymously.
	var count int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM app_authorizations WHERE account_id=$1 AND created_at>clock_timestamp()-interval '1 minute'`, f.account.ID).Scan(&count); e != nil {
		return out, e
	}
	if count >= 10 {
		return out, identity.ErrRateLimited
	}
	e = tx.QueryRowContext(ctx, `SELECT s.name FROM servers s JOIN claim_operations o ON o.server_id=s.id AND o.account_id=s.owner_account_id AND o.claim_generation=s.claim_generation AND o.credential_generation=s.credential_generation AND o.committed_token_hash=s.token_hash AND o.state='installed' WHERE s.id=$1 AND s.revoked_at IS NULL`, q.ServerID).Scan(&out.ServerName)
	if errors.Is(e, sql.ErrNoRows) {
		return out, appauth.ErrGrant
	}
	if e != nil {
		return out, e
	}
	out.ID = security.RandomID("app_")
	out.AccountID = f.account.ID
	out.AccountName = f.account.DisplayName
	out.AppName = q.AppName
	out.RedirectURI = q.RedirectURI
	e = tx.QueryRowContext(ctx, `INSERT INTO app_authorizations(id,account_id,family_id,account_epoch,server_id,server_name,challenge,redirect_uri,state,app_name,code_challenge,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,clock_timestamp()+interval '5 minutes') RETURNING expires_at`, out.ID, f.account.ID, f.id, f.epoch, q.ServerID, out.ServerName, q.Challenge, q.RedirectURI, q.State, q.AppName, q.CodeChallenge).Scan(&out.ExpiresAt)
	if e != nil {
		return out, e
	}
	if e = appFamilyFresh(ctx, tx, f); e != nil {
		return out, e
	}
	return out, tx.Commit()
}

func (s *Store) DecideAppAuthorization(ctx context.Context, refresh []byte, id string, q appauth.Decision) (appauth.Return, error) {
	var out appauth.Return
	if len(id) > 128 || len(id) < 20 || q.AccountID == "" {
		return out, appauth.ErrGrant
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	f, e := appBrowserFamilyTx(ctx, tx, refresh)
	if e != nil {
		return out, e
	}
	if f.account.ID != q.AccountID {
		return out, appauth.ErrGrant
	}
	var request appauth.Request
	e = tx.QueryRowContext(ctx, `SELECT redirect_uri,state FROM app_authorizations WHERE id=$1 AND account_id=$2 AND family_id=$3 AND account_epoch=$4 AND expires_at>clock_timestamp() AND code_hash IS NULL FOR UPDATE`, id, f.account.ID, f.id, f.epoch).Scan(&request.RedirectURI, &request.State)
	if errors.Is(e, sql.ErrNoRows) {
		return out, appauth.ErrGrant
	}
	if e != nil {
		return out, e
	}
	// A server renamed since review requires a fresh review so consent names the
	// same server the person saw. Revoked or unclaimed servers cannot issue codes.
	var valid bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM servers s JOIN app_authorizations a ON a.server_id=s.id WHERE a.id=$1 AND s.name=a.server_name AND s.revoked_at IS NULL)`, id).Scan(&valid)
	if e != nil {
		return out, e
	}
	if !valid {
		return out, appauth.ErrGrant
	}
	code := ""
	if q.Approve {
		code = security.RandomID("pac_")
		_, e = tx.ExecContext(ctx, `UPDATE app_authorizations SET code_hash=$2,code_expires_at=LEAST(expires_at,clock_timestamp()+interval '1 minute') WHERE id=$1`, id, security.Hash(code))
	} else {
		// Keep a deny tombstone until maintenance to keep the review quota sound.
		_, e = tx.ExecContext(ctx, `UPDATE app_authorizations SET code_hash=$2,code_expires_at=clock_timestamp() WHERE id=$1`, id, security.Hash(security.RandomID("denied_")))
	}
	if e != nil {
		return out, e
	}
	if e = appFamilyFresh(ctx, tx, f); e != nil {
		return out, e
	}
	out.Location = appauth.ReturnLocation(request, code)
	return out, tx.Commit()
}

// ExchangeAppAuthorization spends the code in the same transaction that checks
// its browser family and builds the ordinary first-party identity assertion.
// The signer is CPU-only; a signing error rolls back so the code is not lost.
func (s *Store) ExchangeAppAuthorization(ctx context.Context, q appauth.Exchange, sign func(identity.Account, string, string) (directory.SignedPolicy, error)) (appauth.Grant, error) {
	var out appauth.Grant
	if e := q.Validate(); e != nil {
		return out, e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	hash := security.Hash(q.Code)
	var account, family string
	var epoch int64
	e = tx.QueryRowContext(ctx, `SELECT account_id,family_id,account_epoch FROM app_authorizations WHERE code_hash=$1`, hash).Scan(&account, &family, &epoch)
	if errors.Is(e, sql.ErrNoRows) {
		return out, appauth.ErrGrant
	}
	if e != nil {
		return out, e
	}
	var a identity.Account
	var currentEpoch int64
	e = tx.QueryRowContext(ctx, `SELECT id,username,display_name,auth_epoch FROM accounts WHERE id=$1 FOR SHARE`, account).Scan(&a.ID, &a.Username, &a.DisplayName, &currentEpoch)
	if errors.Is(e, sql.ErrNoRows) {
		return out, appauth.ErrGrant
	}
	if e != nil {
		return out, e
	}
	var absolute, idle time.Time
	var revoked sql.NullTime
	var familyEpoch int64
	e = tx.QueryRowContext(ctx, `SELECT account_epoch,absolute_expires_at,idle_expires_at,revoked_at FROM session_families WHERE id=$1 AND account_id=$2 AND mode='browser' FOR SHARE`, family, account).Scan(&familyEpoch, &absolute, &idle, &revoked)
	if errors.Is(e, sql.ErrNoRows) {
		return out, appauth.ErrGrant
	}
	if e != nil {
		return out, e
	}
	if epoch != currentEpoch || familyEpoch != epoch || revoked.Valid {
		return out, appauth.ErrGrant
	}
	var server, challenge, redirect, pkce string
	var expiry time.Time
	e = tx.QueryRowContext(ctx, `SELECT server_id,challenge,redirect_uri,code_challenge,LEAST(expires_at,code_expires_at) FROM app_authorizations WHERE code_hash=$1 FOR UPDATE`, hash).Scan(&server, &challenge, &redirect, &pkce, &expiry)
	if errors.Is(e, sql.ErrNoRows) {
		return out, appauth.ErrGrant
	}
	if e != nil {
		return out, e
	}
	if redirect != q.RedirectURI || subtle.ConstantTimeCompare([]byte(pkce), []byte(appauth.Challenge(q.Verifier))) != 1 {
		return out, appauth.ErrGrant
	}
	var valid bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM servers WHERE id=$1 AND revoked_at IS NULL)`, server).Scan(&valid); e != nil {
		return out, e
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	if !valid || !expiry.After(now) || !absolute.After(now) || !idle.After(now) {
		return out, appauth.ErrGrant
	}
	out.Assertion, e = sign(a, server, challenge)
	if e != nil {
		return appauth.Grant{}, e
	}
	// Recheck after signing, since neither transaction-start time nor a clock
	// sampled before a row lock is an expiry boundary.
	result, e := tx.ExecContext(ctx, `UPDATE app_authorizations SET code_expires_at=clock_timestamp() WHERE code_hash=$1 AND code_expires_at>clock_timestamp() AND expires_at>clock_timestamp() AND $2>clock_timestamp() AND $3>clock_timestamp()`, hash, absolute, idle)
	if e != nil {
		return appauth.Grant{}, e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return appauth.Grant{}, e
	}
	if n != 1 {
		return appauth.Grant{}, appauth.ErrGrant
	}
	return out, tx.Commit()
}
