package postgres

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/security"
	"strings"
	"time"
)

type AccountSecurity struct {
	Store       *Store
	Keys        *security.Keys
	Identity    *identity.Service
	Lifetime    time.Duration
	MailEnabled bool
}
type SecurityAction struct {
	identity.SecurityIntent
	// Proof is required only when the purpose NeedsProof (A80).
	Proof       string `json:"proof,omitempty"`
	NewPassword string `json:"newPassword,omitempty"`
}

func equalSecret(a, b []byte) bool { return len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1 }
func (s *AccountSecurity) digest(kind, owner, value string) []byte {
	return identity.SecretDigest(s.Keys.SealKey, kind, owner, value)
}
func (s *AccountSecurity) requestHash(value any) []byte {
	raw, _ := json.Marshal(value)
	return s.digest("security-request", "", string(raw))
}
func intentDigest(i identity.SecurityIntent) []byte {
	b, _ := json.Marshal(i)
	return security.Hash(string(b))
}
func NormalizeContact(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !mailoutbox.ValidAddress(value) {
		return "", identity.ErrInvalid
	}
	parts := strings.Split(value, "@")
	if len(parts) != 2 {
		return "", identity.ErrInvalid
	}
	return parts[0] + "@" + strings.ToLower(parts[1]), nil
}
func maskedContact(value string) string {
	p := strings.LastIndex(value, "@")
	if p < 1 {
		return "your contact email"
	}
	return string([]rune(value[:p])[0]) + "•••" + value[p:]
}
func (s *AccountSecurity) Limit(ctx context.Context, kind, subject string, limit int, window time.Duration) error {
	return s.limitWith(ctx, s.Store.DB, kind, subject, limit, window)
}
func (s *AccountSecurity) limitWith(ctx context.Context, db receiptReader, kind, subject string, limit int, window time.Duration) error {
	var attempts int
	e := db.QueryRowContext(ctx, `INSERT INTO account_security_limits(subject_hash,kind,window_start,attempts) VALUES($1,$2,clock_timestamp(),1) ON CONFLICT(subject_hash,kind) DO UPDATE SET window_start=CASE WHEN account_security_limits.window_start<clock_timestamp()-($3 * interval '1 second') THEN clock_timestamp() ELSE account_security_limits.window_start END,attempts=CASE WHEN account_security_limits.window_start<clock_timestamp()-($3 * interval '1 second') THEN 1 ELSE LEAST(account_security_limits.attempts+1,1000000) END RETURNING attempts`, s.digest("rate", kind, subject), kind, int(window.Seconds())).Scan(&attempts)
	if e != nil {
		return e
	}
	if attempts > limit {
		return identity.ErrRateLimited
	}
	return nil
}
func (s *AccountSecurity) authorize(ctx context.Context, token string) (*sql.Tx, identity.CurrentFamilyExpectation, error) {
	var empty identity.CurrentFamilyExpectation
	if len(token) < 20 || len(token) > 128 {
		return nil, empty, identity.ErrCredentials
	}
	expected, e := s.Store.CaptureClaimAuthority(ctx, security.Hash(token))
	if e != nil {
		return nil, empty, e
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, empty, e
	}
	if _, e = AuthorizeCurrentFamilyTx(ctx, tx, expected); e != nil {
		tx.Rollback()
		return nil, empty, e
	}
	return tx, expected, nil
}

type receiptReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Only these operations intentionally revoke their own authority while returning
// a terminal acknowledgement. In particular enrollment and code replacement
// are NOT terminal exceptions.
func receiptRevocationReason(purpose string) string {
	switch purpose {
	case "mfa_confirm":
		return "mfa_enabled"
	case "email_confirm":
		return "email_changed"
	case "password_set_or_change":
		return "password_change"
	case "mfa_disable":
		return "mfa_disabled"
	case "provider_unlink":
		return "provider_unlinked"
	}
	return ""
}
func sensitiveMFAReceipt(purpose string) bool {
	return purpose == "mfa_enroll" || purpose == "mfa_confirm" || purpose == "recovery_codes"
}
func (s *AccountSecurity) readReceipt(ctx context.Context, token, operation, purpose string, request []byte, out any) (bool, error) {
	if len(token) < 20 || len(token) > 128 {
		return false, identity.ErrCredentials
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	found, err := s.readReceiptFrom(ctx, tx, security.Hash(token), operation, purpose, request, out)
	if err != nil {
		return false, err
	}
	return found, tx.Commit()
}
func (s *AccountSecurity) readReceiptFrom(ctx context.Context, tx *sql.Tx, access []byte, operation, purpose string, request []byte, out any) (bool, error) {
	var encrypted, stored []byte
	var account, family, terminal string
	var epoch, generation, resultEpoch, mfaRevision sql.NullInt64
	var expires time.Time
	var terminalAt sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT account_id,family_id,request_hash,ciphertext,expires_at,account_epoch,family_generation,result_epoch,terminal_reason,terminal_revoked_at,mfa_revision FROM account_security_receipts WHERE access_hash=$1 AND operation_id=$2 AND purpose=$3`, access, operation, purpose).Scan(&account, &family, &stored, &encrypted, &expires, &epoch, &generation, &resultEpoch, &terminal, &terminalAt, &mfaRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !equalSecret(stored, request) {
		return false, identity.ErrConflict
	}
	if !epoch.Valid || !generation.Valid || !resultEpoch.Valid {
		return false, identity.ErrCredentials
	}
	// Serialize replay with logout-all/security mutations, in the same lock order
	// as new operations. Never decrypt a usable secret before these checks.
	var currentEpoch int64
	if err = tx.QueryRowContext(ctx, `SELECT auth_epoch FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&currentEpoch); err != nil {
		return false, currentFamilyError(ctx, err)
	}
	if currentEpoch != resultEpoch.Int64 {
		return false, identity.ErrCredentials
	}
	var familyAccount, mode string
	var familyEpoch, familyGeneration int64
	var revoked sql.NullTime
	var reason sql.NullString
	var absolute, idle time.Time
	err = tx.QueryRowContext(ctx, `SELECT account_id,account_epoch,generation,mode,revoked_at,revoke_reason,absolute_expires_at,idle_expires_at FROM session_families WHERE id=$1 FOR SHARE`, family).Scan(&familyAccount, &familyEpoch, &familyGeneration, &mode, &revoked, &reason, &absolute, &idle)
	if err != nil {
		return false, currentFamilyError(ctx, err)
	}
	if familyAccount != account || familyEpoch != epoch.Int64 || familyGeneration != generation.Int64 {
		return false, identity.ErrCredentials
	}
	if terminal == "" {
		if resultEpoch.Int64 != epoch.Int64 {
			return false, identity.ErrCredentials
		}
		_, err = AuthorizeCurrentFamilyTx(ctx, tx, identity.CurrentFamilyExpectation{AccountID: account, FamilyID: family, AuthEpoch: epoch.Int64, FamilyGeneration: generation.Int64, Mode: mode, TokenHash: access})
		if err != nil {
			return false, err
		}
	} else if terminal != receiptRevocationReason(purpose) || resultEpoch.Int64 != epoch.Int64+1 || !revoked.Valid || !terminalAt.Valid || !revoked.Time.Equal(terminalAt.Time) || reason.String != terminal {
		return false, identity.ErrCredentials
	}
	if sensitiveMFAReceipt(purpose) {
		var revision int64
		if !mfaRevision.Valid {
			return false, identity.ErrCredentials
		}
		if err = tx.QueryRowContext(ctx, `SELECT revision FROM account_mfa WHERE account_id=$1`, account).Scan(&revision); err != nil {
			return false, currentFamilyError(ctx, err)
		}
		if revision != mfaRevision.Int64 {
			return false, identity.ErrCredentials
		}
	}
	var now time.Time
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return false, err
	}
	if !expires.After(now) {
		return false, identity.ErrConflict
	}
	if !absolute.After(now) || !idle.After(now) {
		return false, identity.ErrCredentials
	}
	plain, err := s.Keys.Open(encrypted, "security-receipt:"+account+":"+operation+":"+purpose)
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(plain), out)
}
func (s *AccountSecurity) saveReceipt(ctx context.Context, tx *sql.Tx, expected identity.CurrentFamilyExpectation, i identity.SecurityIntent, request []byte, out any) error {
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	cipher, err := s.Keys.Seal(string(raw), "security-receipt:"+expected.AccountID+":"+i.OperationID+":"+i.Purpose)
	if err != nil {
		return err
	}
	var resultEpoch int64
	var mfaRevision sql.NullInt64
	var revoked sql.NullTime
	var reason sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT a.auth_epoch,m.revision,f.revoked_at,f.revoke_reason FROM accounts a JOIN session_families f ON f.id=$2 AND f.account_id=a.id LEFT JOIN account_mfa m ON m.account_id=a.id WHERE a.id=$1`, expected.AccountID, expected.FamilyID).Scan(&resultEpoch, &mfaRevision, &revoked, &reason)
	if err != nil {
		return err
	}
	terminal := ""
	if result, ok := out.(identity.SecurityResult); ok && result.SignOut {
		terminal = receiptRevocationReason(i.Purpose)
		if terminal == "" || resultEpoch != expected.AuthEpoch+1 || !revoked.Valid || reason.String != terminal {
			return identity.ErrCredentials
		}
	} else if resultEpoch != expected.AuthEpoch {
		return identity.ErrCredentials
	}
	var existing []byte
	err = tx.QueryRowContext(ctx, `INSERT INTO account_security_receipts(account_id,operation_id,purpose,family_id,access_hash,request_hash,ciphertext,expires_at,account_epoch,family_generation,result_epoch,terminal_reason,terminal_revoked_at,mfa_revision) VALUES($1,$2,$3,$4,$5,$6,$7,statement_timestamp()+interval '5 minutes',$8,$9,$10,$11,$12,$13) ON CONFLICT(account_id,operation_id,purpose) DO UPDATE SET operation_id=EXCLUDED.operation_id RETURNING request_hash`, expected.AccountID, i.OperationID, i.Purpose, expected.FamilyID, expected.TokenHash, request, cipher, expected.AuthEpoch, expected.FamilyGeneration, resultEpoch, terminal, revoked, mfaRevision).Scan(&existing)
	if err == nil && !equalSecret(existing, request) {
		err = identity.ErrConflict
	}
	return err
}
func (s *AccountSecurity) proofTx(ctx context.Context, tx *sql.Tx, expected identity.CurrentFamilyExpectation, i identity.SecurityIntent, proof string) error {
	if !i.Valid() || len(proof) < 32 || len(proof) > 128 {
		return identity.ErrRecentAuth
	}
	var kind string
	var identityID sql.NullString
	var revision sql.NullInt64
	e := tx.QueryRowContext(ctx, `UPDATE account_recent_proofs SET consumed_at=clock_timestamp() WHERE proof_hash=$1 AND account_id=$2 AND family_id=$3 AND account_epoch=$4 AND family_generation=$5 AND session_mode=$6 AND captured_access_hash=$7 AND purpose=$8 AND intent_digest=$9 AND consumed_at IS NULL AND expires_at>clock_timestamp() RETURNING credential_kind,identity_id,identity_revision`, security.Hash(proof), expected.AccountID, expected.FamilyID, expected.AuthEpoch, expected.FamilyGeneration, expected.Mode, expected.TokenHash, i.Purpose, intentDigest(i)).Scan(&kind, &identityID, &revision)
	if errors.Is(e, sql.ErrNoRows) {
		return identity.ErrRecentAuth
	}
	if e != nil {
		return e
	}
	if kind == "provider" || kind == "provider_security_return" {
		if kind == "provider_security_return" {
			if e = s.securityReturnActionTx(ctx, tx, expected, i); e != nil {
				return e
			}
		}
		var valid bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_external_identities WHERE id=$1 AND account_id=$2 AND revision=$3 AND revoked_at IS NULL)`, identityID.String, expected.AccountID, revision.Int64).Scan(&valid)
		if e != nil {
			return e
		}
		if !valid {
			return identity.ErrRecentAuth
		}
	}
	return nil
}
func (s *AccountSecurity) issueProofTx(ctx context.Context, tx *sql.Tx, expected identity.CurrentFamilyExpectation, i identity.SecurityIntent, kind, link string, revision int64) (identity.RecentProof, error) {
	out := identity.RecentProof{Proof: security.RandomID("proof_"), AccountID: expected.AccountID, FamilyID: expected.FamilyID}
	var identityID, identityRevision any
	if kind == "provider" || kind == "provider_security_return" {
		identityID = link
		identityRevision = revision
	}
	e := tx.QueryRowContext(ctx, `INSERT INTO account_recent_proofs(proof_hash,account_id,family_id,account_epoch,family_generation,session_mode,captured_access_hash,credential_kind,identity_id,identity_revision,purpose,intent_digest,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,statement_timestamp()+interval '5 minutes') RETURNING expires_at`, security.Hash(out.Proof), expected.AccountID, expected.FamilyID, expected.AuthEpoch, expected.FamilyGeneration, expected.Mode, expected.TokenHash, kind, identityID, identityRevision, i.Purpose, intentDigest(i)).Scan(&out.ExpiresAt)
	return out, e
}
func (s *AccountSecurity) PasswordProof(ctx context.Context, token string, i identity.SecurityIntent, password, factor string) (identity.RecentProof, error) {
	var out identity.RecentProof
	if !i.Valid() {
		return out, identity.ErrInvalid
	}
	expected, e := s.Store.CaptureClaimAuthority(ctx, security.Hash(token))
	if e != nil {
		return out, e
	}
	if e = s.Limit(ctx, "recent-auth", expected.AccountID, 10, 15*time.Minute); e != nil {
		return out, e
	}
	request := s.requestHash(struct {
		Intent           identity.SecurityIntent
		Password, Factor string
	}{i, password, factor})
	var username string
	if e = s.Store.DB.QueryRowContext(ctx, `SELECT username FROM accounts WHERE id=$1`, expected.AccountID).Scan(&username); e != nil {
		return out, e
	}
	// Receipt recovery is checked under current-family authority. It does not
	// permit a revoked family to recover a still-usable recent-auth proof.
	tx, expected, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	found, e := s.readReceiptFrom(ctx, tx, expected.TokenHash, i.OperationID, "proof:"+i.Purpose, request, &out)
	tx.Rollback()
	if e != nil || found {
		return out, e
	}
	_, hash, e := s.Identity.PasswordCheck(ctx, username, password)
	if e != nil {
		return out, e
	}
	tx, expected, e = s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	found, e = s.readReceiptFrom(ctx, tx, expected.TokenHash, i.OperationID, "proof:"+i.Purpose, request, &out)
	if e != nil || found {
		return out, e
	}
	var stored []byte
	if e = tx.QueryRowContext(ctx, `SELECT password_hash FROM accounts WHERE id=$1`, expected.AccountID).Scan(&stored); e != nil {
		return out, e
	}
	if !equalSecret(hash, stored) {
		return out, identity.ErrRecentAuth
	}
	ok, e := s.factorTx(ctx, tx, expected.AccountID, factor)
	if e != nil {
		return out, e
	}
	if !ok {
		if e = audit(ctx, tx, expected.AccountID, "account.mfa_rejected", expected.AccountID, 0); e != nil {
			return out, e
		}
		if e = tx.Commit(); e != nil {
			return out, e
		}
		return out, identity.ErrChallenge
	}
	out, e = s.issueProofTx(ctx, tx, expected, i, "password", "", 0)
	if e != nil {
		return out, e
	}
	receiptIntent := i
	receiptIntent.Purpose = "proof:" + i.Purpose
	if e = s.saveReceipt(ctx, tx, expected, receiptIntent, request, out); e != nil {
		return out, e
	}
	return out, tx.Commit()
}

// Second-factor backoff (HOST02). The replenishing per-window limits alone let
// someone holding the password keep fetching fresh challenges and guess about
// 1,900 codes a day. Failures are therefore also remembered per account until
// a day passes without one: after factorFreeFailures, each further code waits
// a cool-off doubling from factorCoolOffBase to factorCoolOffCap, so sustained
// guessing falls to a few dozen codes a day.
//
// It can never become a permanent lockout. Only someone past the first factor
// (password, provider or signed-in session) reaches it; the wait is capped; a
// correct code clears it; and a recovery code (128 random bits, not guessable)
// is still checked during a cool-off, so the owner can always get past an
// attacker who holds the password. Password reset, which also needs the
// emailed link, keeps its own count, so the owner can always reset the
// password and end the attacker's first factor.
const (
	factorFreeFailures  = 10
	factorCoolOffBase   = 30 * time.Second
	factorCoolOffCap    = 30 * time.Minute
	factorFailureMemory = 24 * time.Hour

	factorBackoffSignIn = "factor-backoff"
	factorBackoffReset  = "factor-backoff-reset"
)

func factorCoolOff(failures int) time.Duration {
	if failures < factorFreeFailures {
		return 0
	}
	return min(factorCoolOffCap, factorCoolOffBase<<min(failures-factorFreeFailures, 16))
}

func (s *AccountSecurity) factorTx(ctx context.Context, tx *sql.Tx, account, factor string) (bool, error) {
	return s.factorScopedTx(ctx, tx, account, factorBackoffSignIn, factor)
}
func (s *AccountSecurity) factorScopedTx(ctx context.Context, tx *sql.Tx, account, backoff, factor string) (bool, error) {
	var cipher []byte
	var last int64
	e := tx.QueryRowContext(ctx, `SELECT secret,last_step FROM account_mfa WHERE account_id=$1 FOR UPDATE`, account).Scan(&cipher, &last)
	if errors.Is(e, sql.ErrNoRows) || e == nil && len(cipher) == 0 {
		return true, nil
	}
	if e != nil {
		return false, e
	}
	// The account_mfa row lock above serializes this read-check-record for the
	// account, so parallel guesses earn no more tries than a sequence.
	backoffKey := s.digest("rate", backoff, account)
	var failures int
	var lastFailure, now time.Time
	e = tx.QueryRowContext(ctx, `SELECT attempts,window_start,clock_timestamp() FROM account_security_limits WHERE subject_hash=$1 AND kind=$2`, backoffKey, backoff).Scan(&failures, &lastFailure, &now)
	if errors.Is(e, sql.ErrNoRows) {
		failures = 0
	} else if e != nil {
		return false, e
	} else if lastFailure.Before(now.Add(-factorFailureMemory)) {
		failures = 0
	}
	if until := lastFailure.Add(factorCoolOff(failures)); failures >= factorFreeFailures && until.After(now) && len(identity.NormalizeRecoveryCode(factor)) != 32 {
		// Refused without checking or counting the code, and without spending
		// one of the challenge's attempts. The challenge still expires on its
		// own (five minutes), usually before a long cool-off ends; the caller
		// then signs in with the password again for a fresh one.
		return false, identity.CoolOff{Until: until}
	}
	if e = s.limitWith(ctx, tx, "factor-account", account, 20, 15*time.Minute); e != nil {
		return false, e
	}
	valid, e := s.checkFactorTx(ctx, tx, account, factor, cipher, last)
	if e != nil {
		return false, e
	}
	if valid {
		_, e = tx.ExecContext(ctx, `DELETE FROM account_security_limits WHERE subject_hash=$1 AND kind=$2`, backoffKey, backoff)
		return e == nil, e
	}
	// window_start records the latest failure; retention keeps rows for two
	// days, longer than factorFailureMemory.
	_, e = tx.ExecContext(ctx, `INSERT INTO account_security_limits(subject_hash,kind,window_start,attempts) VALUES($1,$2,clock_timestamp(),1) ON CONFLICT(subject_hash,kind) DO UPDATE SET attempts=CASE WHEN account_security_limits.window_start<clock_timestamp()-($3 * interval '1 second') THEN 1 ELSE LEAST(account_security_limits.attempts+1,1000000) END,window_start=clock_timestamp()`, backoffKey, backoff, int(factorFailureMemory.Seconds()))
	return false, e
}
func (s *AccountSecurity) checkFactorTx(ctx context.Context, tx *sql.Tx, account, factor string, cipher []byte, last int64) (bool, error) {
	factor = strings.TrimSpace(factor)
	if len(factor) == 6 {
		secret, e := s.Keys.Open(cipher, "totp:"+account)
		if e != nil {
			return false, e
		}
		var now time.Time
		if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
			return false, e
		}
		step, valid := identity.TOTPStep(secret, factor, now, last)
		if !valid {
			return false, nil
		}
		_, e = tx.ExecContext(ctx, `UPDATE account_mfa SET last_step=$2 WHERE account_id=$1`, account, step)
		return e == nil, e
	}
	normalized := identity.NormalizeRecoveryCode(factor)
	if len(normalized) != 32 {
		return false, nil
	}
	var ignored []byte
	e := tx.QueryRowContext(ctx, `UPDATE account_recovery_codes SET consumed_at=clock_timestamp() WHERE account_id=$1 AND code_hash=$2 AND consumed_at IS NULL RETURNING code_hash`, account, s.digest("mfa-recovery", account, normalized)).Scan(&ignored)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	e = audit(ctx, tx, account, "account.recovery_code_used", account, 0)
	return e == nil, e
}
func (s *AccountSecurity) replaceRecoveryTx(ctx context.Context, tx *sql.Tx, account string) ([]string, error) {
	codes, e := identity.NewRecoveryCodes()
	if e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_mfa SET revision=revision+1 WHERE account_id=$1`, account); e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM account_recovery_codes WHERE account_id=$1`, account); e != nil {
		return nil, e
	}
	for _, code := range codes {
		if _, e = tx.ExecContext(ctx, `INSERT INTO account_recovery_codes(account_id,code_hash) VALUES($1,$2)`, account, s.digest("mfa-recovery", account, identity.NormalizeRecoveryCode(code))); e != nil {
			return nil, e
		}
	}
	return codes, nil
}
func revokeAccountTx(ctx context.Context, tx *sql.Tx, account, reason string) error {
	if _, e := tx.ExecContext(ctx, `UPDATE accounts SET auth_epoch=auth_epoch+1 WHERE id=$1`, account); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `UPDATE session_families SET revoked_at=COALESCE(revoked_at,clock_timestamp()),revoke_reason=COALESCE(revoke_reason,$2) WHERE account_id=$1`, account, reason); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `DELETE FROM account_sessions WHERE account_id=$1`, account); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `DELETE FROM account_recent_proofs WHERE account_id=$1`, account)
	return e
}

// revokeOtherFamiliesTx signs out every session family of the account except
// keep. Unlike revokeAccountTx it leaves auth_epoch alone, so the kept family
// stays valid.
func revokeOtherFamiliesTx(ctx context.Context, tx *sql.Tx, account, keep, reason string) error {
	if _, e := tx.ExecContext(ctx, `UPDATE session_families SET revoked_at=COALESCE(revoked_at,clock_timestamp()),revoke_reason=COALESCE(revoke_reason,$3) WHERE account_id=$1 AND id<>$2`, account, keep, reason); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `DELETE FROM account_sessions WHERE account_id=$1 AND family_id<>$2`, account, keep); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `UPDATE hosted_devices SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE account_id=$1 AND family_id<>$2`, account, keep)
	return e
}

// notifyTx records a security event in the account's in-app feed. event is a
// code (password_changed, two_step_enabled, ...); clients own the words. One
// item per operation and event, so a retried request adds nothing.
func (s *AccountSecurity) notifyTx(ctx context.Context, tx *sql.Tx, account, event, operation string) error {
	id := "security_" + hex.EncodeToString(s.digest("security-event", account, operation+":"+event))[:40]
	return notifyAccountTx(ctx, tx, id, account, "", "security_event", map[string]string{"event": event})
}
func (s *AccountSecurity) State(ctx context.Context, token string) (identity.SecurityState, error) {
	out := identity.SecurityState{Providers: []identity.SecurityProvider{}}
	tx, expected, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	out.AccountID = expected.AccountID
	out.FamilyID = expected.FamilyID
	e = tx.QueryRowContext(ctx, `SELECT a.password_hash IS NOT NULL,COALESCE(c.address,''),COALESCE(c.private_relay,false),EXISTS(SELECT 1 FROM account_mfa m WHERE m.account_id=a.id AND m.secret IS NOT NULL),(SELECT count(*) FROM account_recovery_codes r WHERE r.account_id=a.id AND r.consumed_at IS NULL) FROM accounts a LEFT JOIN account_contact_emails c ON c.account_id=a.id AND c.is_primary WHERE a.id=$1`, expected.AccountID).Scan(&out.HasPassword, &out.ContactAddress, &out.PrivateEmail, &out.MFAEnabled, &out.RecoveryCodesRemaining)
	if e != nil {
		return out, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT id,provider,revision FROM account_external_identities WHERE account_id=$1 AND revoked_at IS NULL ORDER BY provider,id`, expected.AccountID)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var p identity.SecurityProvider
		if e = rows.Scan(&p.ID, &p.Provider, &p.Revision); e != nil {
			rows.Close()
			return out, e
		}
		out.Providers = append(out.Providers, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}

func (s *AccountSecurity) sessionTx(ctx context.Context, tx *sql.Tx, account identity.Account, mode string) (identity.Session, error) {
	family, refresh := identity.NewFamily(mode)
	token := security.RandomID("hat_")
	expires := time.Now().UTC().Add(s.Lifetime)
	if e := insertFamily(ctx, tx, account.ID, security.Hash(token), expires, family); e != nil {
		return identity.Session{}, e
	}
	return identity.Session{Account: account, AccessToken: token, ExpiresAt: expires, RefreshToken: refresh, RefreshExpiresAt: &family.RefreshExpiresAt, FamilyID: family.ID, Mode: mode}, nil
}
func (s *AccountSecurity) sessionReceipt(ctx context.Context, tx *sql.Tx, cipher []byte, aad, account, family string, epoch, generation int64, mode string) (identity.Session, error) {
	var out identity.Session
	var live bool
	e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts a JOIN session_families f ON f.account_id=a.id WHERE a.id=$1 AND a.auth_epoch=$3 AND f.id=$2 AND f.account_epoch=a.auth_epoch AND f.generation=$4 AND f.revoked_at IS NULL AND LEAST(f.idle_expires_at,f.absolute_expires_at)>clock_timestamp())`, account, family, epoch, generation).Scan(&live)
	if e != nil {
		return out, e
	}
	if !live {
		return out, identity.ErrCredentials
	}
	raw, e := s.Keys.Open(cipher, aad)
	if e != nil {
		return out, e
	}
	if e = json.Unmarshal([]byte(raw), &out); e != nil {
		return out, e
	}
	out.Mode = mode
	var accessLive bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_sessions WHERE token_hash=$1 AND family_id=$2 AND expires_at>clock_timestamp())`, security.Hash(out.AccessToken), family).Scan(&accessLive)
	if e != nil {
		return out, e
	}
	if !accessLive {
		return out, identity.ErrCredentials
	}
	return out, nil
}
func (s *AccountSecurity) sealResult(value any, aad string) ([]byte, error) {
	b, e := json.Marshal(value)
	if e != nil {
		return nil, e
	}
	return s.Keys.Seal(string(b), aad)
}
