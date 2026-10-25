package postgres

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/security"
	"strings"
	"time"
)

type RecoveryRequest struct {
	Username       string `json:"username"`
	ContactAddress string `json:"contactAddress"`
	RequestID      string `json:"requestId"`
}

// RecoveryRequest has the same public outcome for unknown, provider-only and
// password accounts. Rate limits and mail readiness are checked independently
// of whether the supplied account exists. No reset proof leaves this boundary.
func (s *AccountSecurity) RequestRecovery(ctx context.Context, q RecoveryRequest) error {
	if !s.MailEnabled {
		return identity.ErrMailUnavailable
	}
	address, e := NormalizeContact(q.ContactAddress)
	q.Username = strings.ToLower(strings.TrimSpace(q.Username))
	if e != nil || len(q.Username) > 64 || len(q.RequestID) < 16 || len(q.RequestID) > 128 {
		return identity.ErrInvalid
	}
	if e = s.Limit(ctx, "recovery-address", strings.ToLower(address), 5, time.Hour); e != nil {
		if errors.Is(e, identity.ErrRateLimited) {
			return nil
		}
		return e
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var account, contact string
	var epoch int64
	var password []byte
	e = tx.QueryRowContext(ctx, `SELECT a.id,a.auth_epoch,a.password_hash,c.id,c.address FROM accounts a JOIN account_contact_emails c ON c.account_id=a.id AND c.is_primary WHERE ($1='' OR a.username=$1) AND lower(c.address)=lower($2) AND a.password_hash IS NOT NULL FOR UPDATE OF a`, q.Username, address).Scan(&account, &epoch, &password, &contact, &address)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	var prior bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_email_challenges WHERE account_id=$1 AND purpose='password_reset' AND operation_id=$2)`, account, q.RequestID).Scan(&prior); e != nil {
		return e
	}
	if prior {
		return nil
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_email_challenges SET superseded_at=clock_timestamp() WHERE account_id=$1 AND purpose='password_reset' AND consumed_at IS NULL AND superseded_at IS NULL`, account); e != nil {
		return e
	}
	id, token := security.RandomID("reset_"), security.RandomID("recover_")
	var expires time.Time
	e = tx.QueryRowContext(ctx, `INSERT INTO account_email_challenges(id,account_id,purpose,address,contact_id,account_epoch,password_fingerprint,code_hash,expires_at,operation_id,request_hash) VALUES($1,$2,'password_reset',$3,$4,$5,$6,$7,statement_timestamp()+interval '15 minutes',$8,$9) RETURNING expires_at`, id, account, address, contact, epoch, s.digest("password-fingerprint", account, string(password)), security.Hash("password-reset:"+token), q.RequestID, s.requestHash(q)).Scan(&expires)
	if e != nil {
		return e
	}
	if e = EnqueueAccountMailTx(ctx, tx, s.Keys, mailoutbox.Intent{ID: security.RandomID("mail_"), Kind: mailoutbox.PasswordReset, ResourceID: id, Revision: 1, AccountID: account, AccountEpoch: epoch, ExpiresAt: expires, Payload: mailoutbox.Payload{To: address, Code: token}}); e != nil {
		return e
	}
	if e = audit(ctx, tx, account, "account.recovery_requested", account, 0); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *AccountSecurity) ResetPassword(ctx context.Context, token, password, factor string) error {
	if len(token) < 32 || len(token) > 128 {
		return identity.ErrResetLinkMalformed
	}
	if len(factor) > 128 {
		return identity.ErrChallenge
	}
	if e := s.Limit(ctx, "reset-token", token, 10, 15*time.Minute); e != nil {
		return e
	}
	tokenHash := security.Hash("password-reset:" + token)
	request := s.requestHash(struct{ Password, Factor string }{password, factor})
	var account string
	e := s.Store.DB.QueryRowContext(ctx, `SELECT account_id FROM account_email_challenges WHERE code_hash=$1 AND purpose='password_reset' AND expires_at>clock_timestamp() AND superseded_at IS NULL`, tokenHash).Scan(&account)
	if e != nil {
		return currentChallengeError(e)
	}
	if e = s.Limit(ctx, "reset-account", account, 10, 15*time.Minute); e != nil {
		return e
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var epoch int64
	var old []byte
	e = tx.QueryRowContext(ctx, `SELECT auth_epoch,password_hash FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&epoch, &old)
	if e != nil {
		return currentChallengeError(e)
	}
	var id, address, contact string
	var expected int64
	var fingerprint, completion []byte
	var consumed, superseded sql.NullTime
	var expiry time.Time
	var attempts int
	e = tx.QueryRowContext(ctx, `SELECT id,address,contact_id,account_epoch,password_fingerprint,consumed_at,superseded_at,expires_at,attempts,completion_hash FROM account_email_challenges WHERE code_hash=$1 AND account_id=$2 AND purpose='password_reset' FOR UPDATE`, tokenHash, account).Scan(&id, &address, &contact, &expected, &fingerprint, &consumed, &superseded, &expiry, &attempts, &completion)
	if e != nil {
		return currentChallengeError(e)
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return e
	}
	if !expiry.After(now) {
		return identity.ErrChallenge
	}
	if consumed.Valid {
		if equalSecret(completion, request) {
			return nil
		}
		return identity.ErrChallenge
	}
	if superseded.Valid || attempts >= 5 || epoch != expected || len(old) == 0 || !equalSecret(fingerprint, s.digest("password-fingerprint", account, string(old))) {
		return identity.ErrChallenge
	}
	var validContact bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_contact_emails WHERE id=$1 AND account_id=$2 AND is_primary AND lower(address)=lower($3))`, contact, account, address).Scan(&validContact)
	if e != nil {
		return e
	}
	if !validContact {
		return identity.ErrChallenge
	}
	valid, e := s.factorScopedTx(ctx, tx, account, factorBackoffReset, factor)
	if e != nil {
		return e
	}
	if !valid {
		if _, e = tx.ExecContext(ctx, `UPDATE account_email_challenges SET attempts=attempts+1 WHERE id=$1`, id); e != nil {
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
		return identity.ErrResetFactor
	}
	hash, e := s.Identity.HashPassword(ctx, password)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE accounts SET password_hash=$2 WHERE id=$1`, account, hash); e != nil {
		return e
	}
	// The emailed link and a factor proved the owner, and the old password no
	// longer works, so second-factor failures earned with it stop slowing the
	// owner's sign-in. With MFA, factorScopedTx already holds the account_mfa
	// lock that serializes that count.
	if _, e = tx.ExecContext(ctx, `DELETE FROM account_security_limits WHERE subject_hash=$1 AND kind=$2`, s.digest("rate", factorBackoffSignIn, account), factorBackoffSignIn); e != nil {
		return e
	}
	if e = revokeAccountTx(ctx, tx, account, "password_reset"); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_email_challenges SET consumed_at=clock_timestamp(),completion_hash=$2 WHERE id=$1`, id, request); e != nil {
		return e
	}
	if e = s.notifyTx(ctx, tx, account, "password_reset", id); e != nil {
		return e
	}
	if e = audit(ctx, tx, account, "account.password_reset", account, 0); e != nil {
		return e
	}
	return tx.Commit()
}
