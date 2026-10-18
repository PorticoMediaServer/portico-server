package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"strings"
	"time"
)

func (s *AccountSecurity) PasswordLogin(ctx context.Context, username, password, mode string) (identity.SignInResult, error) {
	return s.PasswordLoginBound(ctx, username, password, mode, security.RandomID(""), security.RandomID(""), "")
}

// PasswordLoginBound signs in with a password. network is the client network
// (an IPv4 address or IPv6 /64) that password backoff is counted against.
func (s *AccountSecurity) PasswordLoginBound(ctx context.Context, username, password, mode, binding, requestID, network string) (identity.SignInResult, error) {
	var result identity.SignInResult
	if !identity.ValidMode(mode) || len(binding) != 43 || len(requestID) < 16 || len(requestID) > 128 {
		return result, identity.ErrInvalid
	}
	// Backoff is attached to an existing account, never to the login service as
	// a whole. Unknown usernames still perform PasswordCheck's dummy bcrypt, and
	// a well-formed one runs the same cool-off in fixed shared buckets so the
	// answers match (HOST03). A malformed name can't be an account, so it
	// reveals nothing and is given no state.
	var backoffAccount string
	normalized := strings.ToLower(strings.TrimSpace(username))
	e := s.Store.DB.QueryRowContext(ctx, `SELECT id FROM accounts WHERE username=$1`, normalized).Scan(&backoffAccount)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return result, e
	}
	clientNetwork := passwordNetwork(network)
	admitted, until := true, time.Time{}
	if backoffAccount != "" {
		admitted, until, e = s.admitPasswordAttempt(ctx, backoffAccount, clientNetwork)
		if e != nil {
			return result, e
		}
	} else if identity.ValidUsername(normalized) {
		admitted, until, e = s.admitUnknownPasswordAttempt(ctx, normalized, clientNetwork)
		if e != nil {
			return result, e
		}
	}
	if !admitted {
		// A cool-off after failures, not a verdict on this password.
		return result, identity.CoolOff{Until: until}
	}
	a, hash, e := s.Identity.PasswordCheck(ctx, username, password)
	if e != nil {
		return result, e
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return result, e
	}
	defer tx.Rollback()
	var stored []byte
	var epoch int64
	e = tx.QueryRowContext(ctx, `SELECT password_hash,auth_epoch FROM accounts WHERE id=$1 FOR UPDATE`, a.ID).Scan(&stored, &epoch)
	if errors.Is(e, sql.ErrNoRows) {
		return result, identity.ErrCredentials
	}
	if e != nil {
		return result, e
	}
	if !equalSecret(hash, stored) {
		// The password changed between the check and this lock; the attempt was
		// already counted.
		return result, identity.ErrCredentials
	}
	if a.ID == backoffAccount {
		if e = passwordSucceededTx(ctx, tx, a.ID, clientNetwork); e != nil {
			return result, e
		}
	}
	request := s.requestHash(struct{ Username, Password, Mode string }{normalized, password, mode})
	aad := "password-login:" + hex.EncodeToString(security.Hash(binding)) + ":" + requestID
	var prior, cipher []byte
	var priorAccount, priorMode string
	var priorEpoch int64
	var expiry time.Time
	e = tx.QueryRowContext(ctx, `SELECT request_hash,account_id,account_epoch,session_mode,ciphertext,expires_at FROM account_password_logins WHERE binding_hash=$1 AND request_id=$2 FOR UPDATE`, security.Hash(binding), requestID).Scan(&prior, &priorAccount, &priorEpoch, &priorMode, &cipher, &expiry)
	if e == nil {
		if !equalSecret(prior, request) || priorAccount != a.ID || priorEpoch != epoch || priorMode != mode {
			return result, identity.ErrConflict
		}
		var now time.Time
		if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
			return result, e
		}
		if !expiry.After(now) {
			return result, identity.ErrChallenge
		}
		raw, e := s.Keys.Open(cipher, aad)
		if e != nil {
			return result, e
		}
		if e = json.Unmarshal([]byte(raw), &result); e != nil {
			return result, e
		}
		if result.Session != nil {
			result.Session.Mode = mode
			_, e = AuthorizeCurrentFamilyTx(ctx, tx, identity.CurrentFamilyExpectation{AccountID: a.ID, FamilyID: result.Session.FamilyID, AuthEpoch: epoch, FamilyGeneration: 0, Mode: mode, TokenHash: security.Hash(result.Session.AccessToken)})
			if e != nil {
				return identity.SignInResult{}, e
			}
		}
		return result, tx.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return result, e
	}
	var enabled bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_mfa WHERE account_id=$1 AND secret IS NOT NULL)`, a.ID).Scan(&enabled); e != nil {
		return result, e
	}
	if enabled {
		c, e := s.createMFAChallengeTx(ctx, tx, a.ID, epoch, mode, "password", hash, "", 0, "")
		if e != nil {
			return result, e
		}
		result.Challenge = &c
	} else {
		session, e := s.sessionTx(ctx, tx, a, mode)
		if e != nil {
			return result, e
		}
		result.Session = &session
	}
	cipher, e = s.sealResult(result, aad)
	if e != nil {
		return result, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO account_password_logins(binding_hash,request_id,request_hash,account_id,account_epoch,session_mode,ciphertext,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,statement_timestamp()+interval '5 minutes')`, security.Hash(binding), requestID, request, a.ID, epoch, mode, cipher); e != nil {
		return result, e
	}
	if e = audit(ctx, tx, a.ID, "session.password_authenticated", a.ID, epoch); e != nil {
		return result, e
	}
	return result, tx.Commit()
}
func (s *AccountSecurity) createMFAChallengeTx(ctx context.Context, tx *sql.Tx, account string, epoch int64, mode, kind string, password []byte, link string, revision int64, transaction string) (identity.MFAChallenge, error) {
	c := identity.MFAChallenge{MFARequired: true, ChallengeID: security.RandomID("mfa_"), ChallengeToken: security.RandomID("factor_")}
	e := tx.QueryRowContext(ctx, `INSERT INTO account_mfa_challenges(id,proof_hash,account_id,account_epoch,purpose,session_mode,credential_kind,password_hash,identity_id,identity_revision,identity_transaction_id,expires_at) VALUES($1,$2,$3,$4,'login',$5,$6,$7,NULLIF($8,''),$9,NULLIF($10,''),statement_timestamp()+interval '5 minutes') RETURNING expires_at`, c.ChallengeID, security.Hash(c.ChallengeToken), account, epoch, mode, kind, password, link, revision, transaction).Scan(&c.ExpiresAt)
	return c, e
}
func (s *AccountSecurity) CompleteMFA(ctx context.Context, id, token, factor string) (identity.Session, error) {
	var out identity.Session
	if len(id) > 128 || len(token) < 32 || len(token) > 128 || len(factor) > 128 {
		return out, identity.ErrChallenge
	}
	if e := s.Limit(ctx, "mfa-challenge", id, 12, 15*time.Minute); e != nil {
		return out, e
	}
	var account string
	var loginTransaction sql.NullString
	e := s.Store.DB.QueryRowContext(ctx, `SELECT account_id,identity_transaction_id FROM account_mfa_challenges WHERE id=$1 AND proof_hash=$2`, id, security.Hash(token)).Scan(&account, &loginTransaction)
	if e != nil {
		return out, currentChallengeError(e)
	}
	if e = s.Limit(ctx, "mfa-account", account, 20, 15*time.Minute); e != nil {
		return out, e
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	// Cancellation/start take the binding first. Match that ordering before
	// acquiring the account, transaction and challenge completion locks.
	var providerBinding []byte
	if loginTransaction.Valid {
		if e = tx.QueryRowContext(ctx, `SELECT binding_hash FROM identity_transactions WHERE id=$1`, loginTransaction.String).Scan(&providerBinding); e != nil {
			return out, currentChallengeError(e)
		}
		var generation int64
		if e = tx.QueryRowContext(ctx, `SELECT generation FROM identity_browser_bindings WHERE binding_hash=$1 AND expires_at>clock_timestamp() FOR SHARE`, providerBinding).Scan(&generation); e != nil {
			return out, currentChallengeError(e)
		}
	}
	var a identity.Account
	var password []byte
	var epoch int64
	e = tx.QueryRowContext(ctx, `SELECT id,username,display_name,password_hash,auth_epoch FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&a.ID, &a.Username, &a.DisplayName, &password, &epoch)
	if e != nil {
		return out, currentChallengeError(e)
	}
	var providerTransaction oidcRow
	if loginTransaction.Valid {
		providerTransaction, e = lockOIDCTransactionHash(ctx, tx, loginTransaction.String, providerBinding)
		if e != nil {
			return out, e
		}
		if providerTransaction.Mode != "login" {
			return out, identity.ErrChallenge
		}
	}
	var expectedEpoch, revision int64
	var kind, mode string
	var expectedPassword, receipt, completionHash []byte
	var link, transaction, family sql.NullString
	var generation sql.NullInt64
	var consumed, receiptExpiry sql.NullTime
	var expires time.Time
	var attempts int
	e = tx.QueryRowContext(ctx, `SELECT account_epoch,session_mode,credential_kind,password_hash,identity_id,COALESCE(identity_revision,0),identity_transaction_id,expires_at,attempts,consumed_at,family_id,family_generation,receipt,receipt_expires_at,completion_hash FROM account_mfa_challenges WHERE id=$1 AND proof_hash=$2 AND account_id=$3 FOR UPDATE`, id, security.Hash(token), account).Scan(&expectedEpoch, &mode, &kind, &expectedPassword, &link, &revision, &transaction, &expires, &attempts, &consumed, &family, &generation, &receipt, &receiptExpiry, &completionHash)
	if e != nil {
		return out, currentChallengeError(e)
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	if transaction != loginTransaction || (kind == "provider") != transaction.Valid {
		return out, identity.ErrChallenge
	}
	if kind == "provider" && (providerTransaction.SessionMode != mode || (consumed.Valid && providerTransaction.Status != "completed") || (!consumed.Valid && providerTransaction.Status != "verified")) {
		return out, identity.ErrChallenge
	}
	if consumed.Valid {
		if !equalSecret(completionHash, s.requestHash(factor)) {
			return out, identity.ErrChallenge
		}
		if !receiptExpiry.Valid || !receiptExpiry.Time.After(now) {
			return out, identity.ErrChallenge
		}
		out, e = s.sessionReceipt(ctx, tx, receipt, "mfa-login:"+id, account, family.String, expectedEpoch, generation.Int64, mode)
		if e != nil {
			return out, e
		}
		return out, tx.Commit()
	}
	if !expires.After(now) || attempts >= 5 || epoch != expectedEpoch {
		return out, identity.ErrChallenge
	}
	if kind == "password" && !equalSecret(expectedPassword, password) {
		return out, identity.ErrChallenge
	}
	if kind == "provider" {
		var valid bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_external_identities i WHERE i.id=$1 AND i.account_id=$2 AND i.revision=$3 AND i.revoked_at IS NULL)`, link.String, account, revision).Scan(&valid)
		if e != nil {
			return out, e
		}
		if !valid {
			return out, identity.ErrChallenge
		}
	}
	valid, e := s.factorTx(ctx, tx, account, factor)
	if e != nil {
		return out, e
	}
	if !valid {
		if _, e = tx.ExecContext(ctx, `UPDATE account_mfa_challenges SET attempts=attempts+1 WHERE id=$1`, id); e != nil {
			return out, e
		}
		if e = audit(ctx, tx, account, "session.mfa_rejected", account, 0); e != nil {
			return out, e
		}
		if e = tx.Commit(); e != nil {
			return out, e
		}
		return out, identity.ErrChallenge
	}
	out, e = s.sessionTx(ctx, tx, a, mode)
	if e != nil {
		return out, e
	}
	receipt, e = s.sealResult(out, "mfa-login:"+id)
	if e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_mfa_challenges SET consumed_at=clock_timestamp(),completion_hash=$4,family_id=$2,family_generation=0,receipt=$3,receipt_expires_at=statement_timestamp()+interval '5 minutes' WHERE id=$1`, id, out.FamilyID, receipt, s.requestHash(factor)); e != nil {
		return out, e
	}
	if kind == "provider" {
		if e = s.completeIdentitySessionTx(ctx, tx, providerTransaction, out, epoch); e != nil {
			return out, e
		}
		if e = audit(ctx, tx, account, "session.provider_authenticated", link.String, revision); e != nil {
			return out, e
		}
	}
	if e = audit(ctx, tx, account, "session.mfa_authenticated", account, 0); e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func currentChallengeError(e error) error {
	if errors.Is(e, sql.ErrNoRows) {
		return identity.ErrChallenge
	}
	return e
}
