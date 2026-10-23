package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/security"
)

func (o *OIDCLogin) NativeAssertion(ctx context.Context, id, binding, rawToken, name string) (*OIDCView, error) {
	if len(binding) != 43 || len(rawToken) > 16384 {
		return nil, identity.ErrInvalid
	}
	if e := o.Security.Limit(ctx, "native-apple", id, 10, 15*time.Minute); e != nil {
		return nil, e
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	r, e := o.lock(ctx, tx, id, binding)
	if e != nil {
		tx.Rollback()
		return nil, e
	}
	if !r.NativeApple {
		tx.Rollback()
		return nil, identity.ErrInvalid
	}
	if r.Status == "verified" {
		v, e := o.viewTx(ctx, tx, r, "")
		if e != nil {
			tx.Rollback()
			return nil, e
		}
		return v, tx.Commit()
	}
	if r.Status != "started" {
		tx.Rollback()
		return nil, identity.ErrChallenge
	}
	tx.Rollback()
	p := o.Providers["apple"]
	if p == nil {
		return nil, identity.ErrInvalid
	}
	a, e := p.VerifyNative(ctx, rawToken)
	if e != nil {
		return nil, identity.ErrCredentials
	}
	tx, e = o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	r, e = o.lock(ctx, tx, id, binding)
	if e != nil {
		return nil, e
	}
	if r.Status == "verified" {
		v, e := o.viewTx(ctx, tx, r, "")
		if e != nil {
			return nil, e
		}
		return v, tx.Commit()
	}
	if r.Status != "started" || !equalSecret(security.Hash(a.Nonce), r.Nonce) {
		return nil, identity.ErrCredentials
	}
	var email any
	if a.EmailVerified {
		email = a.Email
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,private_relay,transaction_revision,lease_id,suggested_name) VALUES($1,$2,$3,$4,$5,clock_timestamp(),$6,$7,$8,$9,$10,$11,$12)`, id, a.Issuer, a.Subject, a.Audience, a.IssuedAt, r.Nonce, email, a.EmailVerified, a.PrivateRelay && a.EmailVerified, r.Revision, security.RandomID("native_"), cleanProviderName(name)); e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET status='verified',revision=revision+1,native_nonce_cipher=NULL WHERE id=$1`, id); e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM identity_exchange_secrets WHERE transaction_id=$1`, id); e != nil {
		return nil, e
	}
	r.Status = "verified"
	v, e := o.viewTx(ctx, tx, r, "")
	if e != nil {
		return nil, e
	}
	return v, tx.Commit()
}

// ContactChallenge is a sent contact-verification code, identified without
// revealing the address.
type ContactChallenge struct {
	ChallengeID   string    `json:"challengeId"`
	ExpiresAt     time.Time `json:"expiresAt"`
	MaskedAddress string    `json:"maskedAddress"`
}

func (o *OIDCLogin) SendContact(ctx context.Context, id, binding, address, requestID string) (ContactChallenge, error) {
	if !o.Security.MailEnabled {
		return ContactChallenge{}, identity.ErrMailUnavailable
	}
	address, e := NormalizeContact(address)
	if e != nil || len(requestID) < 16 || len(requestID) > 128 {
		return ContactChallenge{}, identity.ErrInvalid
	}
	if e = o.Security.Limit(ctx, "oidc-contact", id, 10, time.Hour); e != nil {
		return ContactChallenge{}, e
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return ContactChallenge{}, e
	}
	defer tx.Rollback()
	// Match CompleteFlow's subject-before-transaction order. A verified login
	// assertion alone is not authority to send account-setup contact email.
	var issuer, subject string
	if e = tx.QueryRowContext(ctx, `SELECT a.issuer,a.subject FROM identity_transactions t JOIN identity_verified_assertions a ON a.transaction_id=t.id WHERE t.id=$1 AND t.binding_hash=$2`, id, security.Hash(binding)).Scan(&issuer, &subject); e != nil {
		return ContactChallenge{}, identity.ErrChallenge
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,193))`, hex.EncodeToString(security.Hash(issuer+"\x00"+subject))); e != nil {
		return ContactChallenge{}, e
	}
	r, e := o.lock(ctx, tx, id, binding)
	if e != nil {
		return ContactChallenge{}, e
	}
	if r.Mode != "login" || r.Status != "verified" {
		return ContactChallenge{}, identity.ErrChallenge
	}
	var currentIssuer, currentSubject string
	if e = tx.QueryRowContext(ctx, `SELECT issuer,subject FROM identity_verified_assertions WHERE transaction_id=$1`, id).Scan(&currentIssuer, &currentSubject); e != nil || issuer != currentIssuer || subject != currentSubject {
		return ContactChallenge{}, identity.ErrChallenge
	}
	signup, e := oidcSignupContactTx(ctx, tx, o.Keys, id)
	if e != nil {
		return ContactChallenge{}, e
	}
	if !signup {
		return ContactChallenge{}, identity.ErrChallenge
	}
	var cid, priorAddress string
	var expiry time.Time
	e = tx.QueryRowContext(ctx, `SELECT id,address,expires_at FROM identity_contact_challenges WHERE transaction_id=$1 AND request_id=$2 AND is_current`, id, requestID).Scan(&cid, &priorAddress, &expiry)
	if e == nil {
		if priorAddress != address {
			return ContactChallenge{}, identity.ErrConflict
		}
		return ContactChallenge{ChallengeID: cid, ExpiresAt: expiry, MaskedAddress: maskedContact(address)}, tx.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return ContactChallenge{}, e
	}
	var count int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM identity_contact_challenges WHERE transaction_id=$1`, id).Scan(&count); e != nil {
		return ContactChallenge{}, e
	}
	if count >= 5 {
		return ContactChallenge{}, identity.ErrRateLimited
	}
	if _, e = tx.ExecContext(ctx, `UPDATE identity_contact_challenges SET is_current=false WHERE transaction_id=$1 AND is_current`, id); e != nil {
		return ContactChallenge{}, e
	}
	cid = security.RandomID("contact_challenge_")
	code, e := identity.NewEmailCode()
	if e != nil {
		return ContactChallenge{}, e
	}
	e = tx.QueryRowContext(ctx, `INSERT INTO identity_contact_challenges(id,transaction_id,transaction_expires_at,address,challenge_hash,revision,expires_at,request_id) VALUES($1,$2,$3,$4,$5,1,LEAST($3,statement_timestamp()+interval '10 minutes'),$6) RETURNING expires_at`, cid, id, r.Expires, address, o.Security.digest("oidc-contact", cid, code), requestID).Scan(&expiry)
	if e != nil {
		return ContactChallenge{}, e
	}
	if e = EnqueueAccountMailTx(ctx, tx, o.Keys, mailoutbox.Intent{ID: security.RandomID("mail_"), Kind: mailoutbox.OIDCContact, ResourceID: cid, Revision: 1, ExpiresAt: expiry, Payload: mailoutbox.Payload{To: address, Code: code}}); e != nil {
		return ContactChallenge{}, e
	}
	return ContactChallenge{ChallengeID: cid, ExpiresAt: expiry, MaskedAddress: maskedContact(address)}, tx.Commit()
}
func (o *OIDCLogin) VerifyContact(ctx context.Context, id, binding, cid, code string) (*OIDCView, error) {
	if e := o.Security.Limit(ctx, "oidc-contact-code", id, 12, 15*time.Minute); e != nil {
		return nil, e
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	r, e := o.lock(ctx, tx, id, binding)
	if e != nil {
		return nil, e
	}
	if r.Status != "verified" || r.Mode != "login" {
		return nil, identity.ErrChallenge
	}
	var hash []byte
	var tries int
	var expires time.Time
	var verified sql.NullTime
	e = tx.QueryRowContext(ctx, `SELECT challenge_hash,attempt_count,expires_at,verified_at FROM identity_contact_challenges WHERE transaction_id=$1 AND id=$2 AND is_current FOR UPDATE`, id, cid).Scan(&hash, &tries, &expires, &verified)
	if e != nil {
		return nil, currentChallengeError(e)
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return nil, e
	}
	if !expires.After(now) || tries >= 5 {
		return nil, identity.ErrChallenge
	}
	if !verified.Valid {
		if !equalSecret(hash, o.Security.digest("oidc-contact", cid, code)) {
			if _, e = tx.ExecContext(ctx, `UPDATE identity_contact_challenges SET attempt_count=attempt_count+1 WHERE id=$1`, cid); e != nil {
				return nil, e
			}
			if e = tx.Commit(); e != nil {
				return nil, e
			}
			return nil, identity.ErrChallenge
		}
		if _, e = tx.ExecContext(ctx, `UPDATE identity_contact_challenges SET verified_at=clock_timestamp() WHERE id=$1`, cid); e != nil {
			return nil, e
		}
	}
	v, e := o.viewTx(ctx, tx, r, "")
	if e != nil {
		return nil, e
	}
	return v, tx.Commit()
}

// Caller serializes this immutable subject with CompleteFlow. Email identity
// similarity is deliberately irrelevant; revoked mappings are still mappings.
func oidcSignupContactTx(ctx context.Context, tx *sql.Tx, keys *security.Keys, id string) (bool, error) {
	var issuer, subject, marker, mode, status string
	var expected sql.NullString
	e := tx.QueryRowContext(ctx, `SELECT a.issuer,a.subject,t.marker_key_id,t.mode,t.status,t.expected_account_id FROM identity_transactions t JOIN identity_verified_assertions a ON a.transaction_id=t.id WHERE t.id=$1 FOR SHARE OF t`, id).Scan(&issuer, &subject, &marker, &mode, &status, &expected)
	if e != nil {
		return false, e
	}
	if mode != "login" || status != "verified" || expected.Valid || keys == nil || len(keys.SealKey) == 0 {
		return false, nil
	}
	var eligible bool
	e = tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM account_external_identities WHERE issuer=$1 AND subject=$2) AND NOT EXISTS(SELECT 1 FROM deleted_identity_markers WHERE marker_key_id=$3 AND subject_digest=$4 AND expires_at>clock_timestamp())`, issuer, subject, marker, identity.SecretDigest(keys.SealKey, "deleted-identity", issuer, subject)).Scan(&eligible)
	return eligible, e
}
