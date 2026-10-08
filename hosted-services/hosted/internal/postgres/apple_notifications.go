package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/security"
)

func appleSubjectHash(subject string) []byte {
	return security.Hash(oidclogin.AppleIssuer + "\x00" + subject)
}
func appleEmailHash(email string) []byte {
	return security.Hash("apple-relay\x00" + strings.ToLower(strings.TrimSpace(email)))
}

// ApplyAppleNotification must only receive a signature-verified event. The subject
// lock is shared with OIDC completion, including first-account creation.
func (o *OIDCLogin) ApplyAppleNotification(ctx context.Context, n oidclogin.AppleNotification) error {
	tx, err := o.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	subject := appleSubjectHash(n.Subject)
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,193))`, hex.EncodeToString(subject)); err != nil {
		return err
	}
	raw, err := json.Marshal(n)
	if err != nil {
		return err
	}
	digest := security.Hash(string(raw))
	var previous []byte
	err = tx.QueryRowContext(ctx, `INSERT INTO apple_account_notifications(id_hash,payload_hash,event_type) VALUES($1,$2,$3) ON CONFLICT(id_hash) DO NOTHING RETURNING payload_hash`, security.Hash("apple-notification\x00"+n.ID), digest, n.Type).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		if err = tx.QueryRowContext(ctx, `SELECT payload_hash FROM apple_account_notifications WHERE id_hash=$1`, security.Hash("apple-notification\x00"+n.ID)).Scan(&previous); err != nil {
			return err
		}
		if !equalSecret(previous, digest) {
			return identity.ErrConflict
		}
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	var account, link string
	err = tx.QueryRowContext(ctx, `SELECT account_id,id FROM account_external_identities WHERE issuer=$1 AND subject=$2`, oidclogin.AppleIssuer, n.Subject).Scan(&account, &link)
	if errors.Is(err, sql.ErrNoRows) {
		account = ""
		link = ""
		err = nil
	}
	if err != nil {
		return err
	}
	if account != "" {
		if err = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&account); errors.Is(err, sql.ErrNoRows) {
			account = ""
			link = ""
		} else if err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO apple_identity_events(subject_hash) VALUES($1) ON CONFLICT DO NOTHING`, subject); err != nil {
		return err
	}
	var revoked, deleted sql.NullTime
	if err = tx.QueryRowContext(ctx, `SELECT revoked_at,deleted_at FROM apple_identity_events WHERE subject_hash=$1`, subject).Scan(&revoked, &deleted); err != nil {
		return err
	}
	if n.Type == "email-enabled" || n.Type == "email-disabled" {
		if !deleted.Valid {
			_, err = tx.ExecContext(ctx, `INSERT INTO apple_relay_events(subject_hash,email_hash,event_at,disabled) VALUES($1,$2,$3,$4) ON CONFLICT(subject_hash,email_hash) DO UPDATE SET event_at=EXCLUDED.event_at,disabled=EXCLUDED.disabled WHERE EXCLUDED.event_at>apple_relay_events.event_at OR (EXCLUDED.event_at=apple_relay_events.event_at AND EXCLUDED.disabled)`, subject, appleEmailHash(n.Email), n.EventTime, n.Type == "email-disabled")
		}
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	if n.Type != "consent-revoked" && n.Type != "account-deleted" {
		return identity.ErrInvalid
	}
	if deleted.Valid || (n.Type == "consent-revoked" && revoked.Valid && !n.EventTime.After(revoked.Time)) {
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `UPDATE apple_identity_events SET revoked_at=GREATEST(revoked_at,$2),deleted_at=CASE WHEN $3 THEN $2 ELSE deleted_at END WHERE subject_hash=$1`, subject, n.EventTime, n.Type == "account-deleted"); err != nil {
		return err
	}
	if n.Type == "account-deleted" {
		if _, err = tx.ExecContext(ctx, `UPDATE apple_relay_events SET disabled=true WHERE subject_hash=$1`, subject); err != nil {
			return err
		}
	}
	if account == "" {
		return tx.Commit()
	}
	// A delayed consent event must not revoke a subsequently completed fresh login.
	var newer bool
	if n.Type == "consent-revoked" {
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(authorized_at>$2,false) FROM apple_identity_events WHERE subject_hash=$1`, subject, n.EventTime).Scan(&newer); err != nil {
			return err
		}
	}
	if !newer {
		// Preserve a manual unlink: only notification-owned revocations can be restored
		// by a later fresh Apple consent.
		if _, err = tx.ExecContext(ctx, `UPDATE account_external_identities SET revoked_at=statement_timestamp(),apple_revocation_at=statement_timestamp(),revision=revision+1 WHERE id=$1 AND (revoked_at IS NULL OR revoked_at=apple_revocation_at)`, link); err != nil {
			return err
		}
		if err = revokeAccountTx(ctx, tx, account, "apple_"+n.Type); err != nil {
			return err
		}
	}
	if n.Type == "account-deleted" {
		// Include stored relay contacts as well as addresses learned from events.
		rows, e := tx.QueryContext(ctx, `SELECT address FROM account_contact_emails WHERE source_identity_id=$1 AND private_relay`, link)
		if e != nil {
			return e
		}
		emails := []string{}
		for rows.Next() {
			var email string
			if e = rows.Scan(&email); e != nil {
				rows.Close()
				return e
			}
			emails = append(emails, email)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, email := range emails {
			if _, err = tx.ExecContext(ctx, `INSERT INTO apple_relay_events(subject_hash,email_hash,event_at,disabled) VALUES($1,$2,$3,true) ON CONFLICT(subject_hash,email_hash) DO UPDATE SET disabled=true`, subject, appleEmailHash(email), n.EventTime); err != nil {
				return err
			}
		}
	}
	if err = audit(ctx, tx, account, "identity.apple_"+n.Type, link, 0); err != nil {
		return err
	}
	return tx.Commit()
}

// Check every completion, including security reauth/link and receipt recovery.
func appleAssertionAllowedTx(ctx context.Context, tx *sql.Tx, id, issuer, subject string) error {
	if issuer != oidclogin.AppleIssuer {
		return nil
	}
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM apple_identity_events e JOIN identity_verified_assertions a ON a.transaction_id=$2 JOIN identity_transactions t ON t.id=a.transaction_id WHERE e.subject_hash=$1 AND (e.deleted_at IS NOT NULL OR (e.revoked_at IS NOT NULL AND (a.provider_issued_at<=e.revoked_at OR t.created_at<=e.revoked_at))))`, appleSubjectHash(subject), id).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return identity.ErrProviderConflict
	}
	return nil
}

func (o *OIDCLogin) restoreAppleConsentTx(ctx context.Context, tx *sql.Tx, issuer, link string, revoked sql.NullTime) (bool, error) {
	if issuer != oidclogin.AppleIssuer || !revoked.Valid {
		return false, nil
	}
	result, err := tx.ExecContext(ctx, `UPDATE account_external_identities SET revoked_at=NULL,apple_revocation_at=NULL,verified_at=clock_timestamp(),revision=revision+1 WHERE id=$1 AND revoked_at=apple_revocation_at`, link)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// Relay suppression covers every typed account email, including queued mail and
// recipient-address invitations. A message already handed to SMTP cannot be recalled.
func (m *AccountMail) appleMailAllowed(ctx context.Context, email string) (bool, error) {
	var blocked bool
	err := m.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM apple_relay_events WHERE email_hash=$1 AND disabled)`, appleEmailHash(email)).Scan(&blocked)
	return !blocked, err
}

// Persist successful authorization independently of short-lived OIDC receipts.
// Account authority serializes this with notifications, including MFA completion.
func recordAppleAuthorizationTx(ctx context.Context, tx *sql.Tx, id, provider string) error {
	if provider != "apple" {
		return nil
	}
	var subject string
	var issued time.Time
	if err := tx.QueryRowContext(ctx, `SELECT a.subject,LEAST(a.provider_issued_at,t.created_at) FROM identity_verified_assertions a JOIN identity_transactions t ON t.id=a.transaction_id WHERE t.id=$1`, id).Scan(&subject, &issued); err != nil {
		return err
	}
	if err := appleAssertionAllowedTx(ctx, tx, id, oidclogin.AppleIssuer, subject); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO apple_identity_events(subject_hash,authorized_at) VALUES($1,$2) ON CONFLICT(subject_hash) DO UPDATE SET authorized_at=GREATEST(apple_identity_events.authorized_at,EXCLUDED.authorized_at)`, appleSubjectHash(subject), issued)
	return err
}
