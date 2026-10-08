package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
)

// EmailLinkResult says what a verification link did. Status is "confirmed"
// (the address is proven; the flow continues on the device that started it),
// "email_changed" (the change is complete) or "already_confirmed".
type EmailLinkResult struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

// ConfirmEmailLink completes the address-ownership step from a mailed link on
// any device (A33). The link carries the resource and its one-time code; the
// code alone proves the recipient controls the address, so no browser binding
// or session is needed here. Registration and provider-contact flows are only
// marked verified: the device that started them finishes them (its next
// request sees contactVerified). An email change was authorized by a session
// when it began, so the link completes it.
func (s *AccountSecurity) ConfirmEmailLink(ctx context.Context, kind, resource, code string) (EmailLinkResult, error) {
	out := EmailLinkResult{Kind: kind}
	code = strings.TrimSpace(code)
	if resource == "" || len(resource) > 128 || code == "" || len(code) > 128 {
		return out, identity.ErrInvalid
	}
	if e := s.Limit(ctx, "email-link", resource, 10, 15*time.Minute); e != nil {
		return out, e
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	switch kind {
	case "registration":
		out.Status, e = s.confirmRegistrationLinkTx(ctx, tx, resource, code)
	case "oidc_contact":
		out.Status, e = s.confirmContactLinkTx(ctx, tx, resource, code)
	case "email_change":
		out.Status, e = s.confirmEmailChangeLinkTx(ctx, tx, resource, code)
	default:
		return out, identity.ErrInvalid
	}
	if e != nil {
		if errors.Is(e, errLinkCodeMismatch) {
			// The failed attempt is counted even though the result is an error.
			if ce := tx.Commit(); ce != nil {
				return out, ce
			}
			return out, identity.ErrChallenge
		}
		return out, e
	}
	return out, tx.Commit()
}

var errLinkCodeMismatch = errors.New("link code mismatch")

func (s *AccountSecurity) confirmRegistrationLinkTx(ctx context.Context, tx *sql.Tx, id, code string) (string, error) {
	var hash []byte
	var revision int64
	var attempts int
	var verified, completed sql.NullTime
	var live bool
	e := tx.QueryRowContext(ctx, `SELECT code_hash,revision,attempts,verified_at,completed_at,code_expires_at>clock_timestamp() AND expires_at>clock_timestamp() FROM account_registrations WHERE id=$1 AND cancelled_at IS NULL FOR UPDATE`, id).Scan(&hash, &revision, &attempts, &verified, &completed, &live)
	if errors.Is(e, sql.ErrNoRows) {
		return "", identity.ErrChallenge
	}
	if e != nil {
		return "", e
	}
	if verified.Valid || completed.Valid {
		return "already_confirmed", nil
	}
	if !live || attempts >= 5 {
		return "", identity.ErrChallenge
	}
	if !equalSecret(hash, s.digest("registration-code", id+":"+strconv.FormatInt(revision, 10), code)) {
		if _, e = tx.ExecContext(ctx, `UPDATE account_registrations SET attempts=attempts+1 WHERE id=$1`, id); e != nil {
			return "", e
		}
		return "", errLinkCodeMismatch
	}
	_, e = tx.ExecContext(ctx, `UPDATE account_registrations SET verified_at=clock_timestamp(),code_hash=NULL WHERE id=$1`, id)
	return "confirmed", e
}

func (s *AccountSecurity) confirmContactLinkTx(ctx context.Context, tx *sql.Tx, cid, code string) (string, error) {
	var hash []byte
	var tries int
	var verified sql.NullTime
	var live bool
	e := tx.QueryRowContext(ctx, `SELECT challenge_hash,attempt_count,verified_at,expires_at>clock_timestamp() FROM identity_contact_challenges WHERE id=$1 AND is_current FOR UPDATE`, cid).Scan(&hash, &tries, &verified, &live)
	if errors.Is(e, sql.ErrNoRows) {
		return "", identity.ErrChallenge
	}
	if e != nil {
		return "", e
	}
	if verified.Valid {
		return "already_confirmed", nil
	}
	if !live || tries >= 5 {
		return "", identity.ErrChallenge
	}
	if !equalSecret(hash, s.digest("oidc-contact", cid, code)) {
		if _, e = tx.ExecContext(ctx, `UPDATE identity_contact_challenges SET attempt_count=attempt_count+1 WHERE id=$1`, cid); e != nil {
			return "", e
		}
		return "", errLinkCodeMismatch
	}
	_, e = tx.ExecContext(ctx, `UPDATE identity_contact_challenges SET verified_at=clock_timestamp() WHERE id=$1`, cid)
	return "confirmed", e
}

func (s *AccountSecurity) confirmEmailChangeLinkTx(ctx context.Context, tx *sql.Tx, id, code string) (string, error) {
	var account, address, operation string
	var hash []byte
	var consumed sql.NullTime
	var live bool
	e := tx.QueryRowContext(ctx, `SELECT c.account_id,c.address,COALESCE(c.operation_id,''),c.code_hash,c.consumed_at,c.superseded_at IS NULL AND c.expires_at>clock_timestamp() AND c.attempts<5 AND c.account_epoch=a.auth_epoch FROM account_email_challenges c JOIN accounts a ON a.id=c.account_id WHERE c.id=$1 AND c.purpose='email_change' FOR UPDATE OF c`, id).Scan(&account, &address, &operation, &hash, &consumed, &live)
	if errors.Is(e, sql.ErrNoRows) {
		return "", identity.ErrChallenge
	}
	if e != nil {
		return "", e
	}
	if consumed.Valid {
		return "already_confirmed", nil
	}
	if !live {
		return "", identity.ErrChallenge
	}
	if !equalSecret(hash, s.digest("email-change", id, code)) {
		if _, e = tx.ExecContext(ctx, `UPDATE account_email_challenges SET attempts=attempts+1 WHERE id=$1`, id); e != nil {
			return "", e
		}
		return "", errLinkCodeMismatch
	}
	// The same steps as ConfirmEmailChange, minus the session it can't have here.
	if e = s.notifyTx(ctx, tx, account, "contact_email_changed", operation+":old"); e != nil {
		return "", e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_contact_emails SET is_primary=false WHERE account_id=$1`, account); e != nil {
		return "", e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO account_contact_emails(id,account_id,address,verified_at,source,is_primary) VALUES($1,$2,$3,clock_timestamp(),'challenge',true) ON CONFLICT(account_id,address) DO UPDATE SET is_primary=true,verified_at=clock_timestamp(),source='challenge',source_identity_id=NULL,private_relay=false`, security.RandomID("contact_"), account, address); e != nil {
		return "", e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_email_challenges SET consumed_at=clock_timestamp() WHERE id=$1`, id); e != nil {
		return "", e
	}
	if e = s.notifyTx(ctx, tx, account, "contact_email_verified", operation+":new"); e != nil {
		return "", e
	}
	if e = revokeAccountTx(ctx, tx, account, "email_changed"); e != nil {
		return "", e
	}
	if e = audit(ctx, tx, account, "account.email_changed", account, 0); e != nil {
		return "", e
	}
	return "email_changed", nil
}
