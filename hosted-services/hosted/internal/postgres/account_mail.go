package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/security"
	"time"
)

type AccountMail struct {
	Store *Store
	Keys  *security.Keys
}

// EnqueueAccountMailTx is the single typed, transactional enqueue boundary.
// Same ID + same payload is a safe retry; different payload is a conflict.
func EnqueueAccountMailTx(ctx context.Context, tx *sql.Tx, keys *security.Keys, i mailoutbox.Intent) error {
	if err := i.Validate(); err != nil {
		return err
	}
	raw, err := mailoutbox.MarshalPayload(i.Payload)
	if err != nil {
		return err
	}
	cipher, err := keys.Seal(raw, "mail:"+i.ID)
	if err != nil {
		return err
	}
	digest := identity.SecretDigest(keys.SealKey, "mail-payload", i.ID, fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%d\x00%s", i.Kind, i.ResourceID, i.Revision, i.AccountID, i.AccountEpoch, raw))
	var stored []byte
	err = tx.QueryRowContext(ctx, `INSERT INTO account_mail_outbox(id,kind,resource_id,revision,account_id,account_epoch,ciphertext,payload_hash,expires_at) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9) ON CONFLICT(id) DO UPDATE SET id=EXCLUDED.id RETURNING payload_hash`, i.ID, i.Kind, i.ResourceID, i.Revision, i.AccountID, i.AccountEpoch, cipher, digest, i.ExpiresAt).Scan(&stored)
	if err != nil {
		return err
	}
	if !equalSecret(stored, digest) {
		return identity.ErrConflict
	}
	// The delivery lane sleeps until this commit rather than asking twice a second whether
	// anybody has sent an email.
	return notifyTx(ctx, tx, MailChannel)
}
func (m *AccountMail) ClaimMail(ctx context.Context) (*mailoutbox.Delivery, error) {
	tx, e := m.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var d mailoutbox.Delivery
	var cipher []byte
	var now time.Time
	var kind string
	e = tx.QueryRowContext(ctx, `SELECT id,kind,resource_id,revision,COALESCE(account_id,''),account_epoch,ciphertext,expires_at,attempts,created_at,clock_timestamp() FROM account_mail_outbox WHERE (status='pending' AND next_attempt_at<=clock_timestamp()) OR (status='sending' AND lease_until<=clock_timestamp()) ORDER BY next_attempt_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&d.ID, &kind, &d.ResourceID, &d.Revision, &d.AccountID, &d.AccountEpoch, &cipher, &d.ExpiresAt, &d.Attempts, &d.CreatedAt, &now)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if d.Attempts >= 8 || !d.ExpiresAt.After(now) {
		_, e = tx.ExecContext(ctx, `UPDATE account_mail_outbox SET status='failed',ciphertext=NULL,lease_id=NULL,lease_until=NULL,last_failure='expired' WHERE id=$1`, d.ID)
		if e != nil {
			return nil, e
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		return nil, mailoutbox.ErrQueueProgress
	}
	d.Kind = mailoutbox.Kind(kind)
	d.Lease = security.RandomID("mail_lease_")
	d.Attempts++
	if _, e = tx.ExecContext(ctx, `UPDATE account_mail_outbox SET status='sending',attempts=attempts+1,lease_id=$2,lease_until=statement_timestamp()+interval '45 seconds' WHERE id=$1`, d.ID, d.Lease); e != nil {
		return nil, e
	}
	plain, e := m.Keys.Open(cipher, "mail:"+d.ID)
	if e != nil || json.Unmarshal([]byte(plain), &d.Payload) != nil {
		_, e = tx.ExecContext(ctx, `UPDATE account_mail_outbox SET status='failed',ciphertext=NULL,lease_id=NULL,lease_until=NULL,last_failure='invalid' WHERE id=$1`, d.ID)
		if e != nil {
			return nil, e
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		return nil, mailoutbox.ErrQueueProgress
	}
	return &d, tx.Commit()
}
func (m *AccountMail) MailEligible(ctx context.Context, d mailoutbox.Delivery) (bool, error) {
	var live bool
	if allowed, err := m.appleMailAllowed(ctx, d.Payload.To); err != nil || !allowed {
		return false, err
	}
	if e := m.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_mail_outbox WHERE id=$1 AND lease_id=$2 AND status='sending' AND lease_until>clock_timestamp() AND expires_at>clock_timestamp())`, d.ID, d.Lease).Scan(&live); e != nil || !live {
		return false, e
	}
	switch d.Kind {
	case mailoutbox.Registration:
		e := m.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_registrations WHERE id=$1 AND revision=$2 AND address=$3 AND verified_at IS NULL AND completed_at IS NULL AND cancelled_at IS NULL AND code_expires_at>clock_timestamp() AND expires_at>clock_timestamp() AND attempts<5)`, d.ResourceID, d.Revision, d.Payload.To).Scan(&live)
		return live, e
	case mailoutbox.OIDCContact:
		e := m.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM identity_contact_challenges c JOIN identity_transactions t ON t.id=c.transaction_id JOIN identity_browser_bindings b ON b.binding_hash=t.binding_hash JOIN identity_protocol_policy p ON p.singleton JOIN identity_provider_descriptors pd ON pd.id=t.descriptor_id WHERE c.id=$1 AND c.revision=$2 AND c.address=$3 AND c.is_current AND c.verified_at IS NULL AND c.attempt_count<5 AND c.expires_at>clock_timestamp() AND t.terminal_at IS NULL AND t.expires_at>clock_timestamp() AND t.binding_generation=b.generation AND b.expires_at>clock_timestamp() AND p.enabled AND p.revision=t.policy_revision AND pd.enabled AND pd.revision=t.descriptor_revision)`, d.ResourceID, d.Revision, d.Payload.To).Scan(&live)
		return live, e
	case mailoutbox.PasswordReset, mailoutbox.EmailChange:
		e := m.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_email_challenges c JOIN accounts a ON a.id=c.account_id WHERE c.id=$1 AND c.purpose=$2 AND c.revision=$3 AND c.address=$4 AND a.auth_epoch=c.account_epoch AND c.consumed_at IS NULL AND c.superseded_at IS NULL AND c.attempts<5 AND c.expires_at>clock_timestamp() AND (c.purpose<>'password_reset' OR (a.password_hash IS NOT NULL AND EXISTS(SELECT 1 FROM account_contact_emails e WHERE e.account_id=a.id AND e.id=c.contact_id AND e.address=c.address AND e.is_primary))) AND (c.purpose<>'email_change' OR EXISTS(SELECT 1 FROM session_families f JOIN account_sessions s ON s.family_id=f.id WHERE f.id=c.family_id AND f.account_id=a.id AND f.account_epoch=a.auth_epoch AND f.generation=c.family_generation AND f.revoked_at IS NULL AND LEAST(f.idle_expires_at,f.absolute_expires_at)>clock_timestamp() AND s.token_hash=c.access_hash AND s.expires_at>clock_timestamp())))`, d.ResourceID, string(d.Kind), d.Revision, d.Payload.To).Scan(&live)
		return live, e
	case mailoutbox.Invitation:
		e := m.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM invitations i WHERE i.id=$1 AND i.status='pending' AND i.expires_at>clock_timestamp() AND i.delivery_revision=$5 AND ((i.recipient_kind='email' AND i.recipient_email=$2 AND $3='') OR (i.recipient_kind='username' AND EXISTS(SELECT 1 FROM accounts a JOIN account_contact_emails c ON c.account_id=a.id AND c.is_primary WHERE a.id=i.recipient_id AND c.address=$2 AND a.id=$3 AND a.auth_epoch=$4))))`, d.ResourceID, d.Payload.To, d.AccountID, d.AccountEpoch, d.Revision).Scan(&live)
		return live, e
	}
	return false, nil
}
func (m *AccountMail) FinishMail(ctx context.Context, d mailoutbox.Delivery, result string, terminal bool) error {
	if result == "sent" {
		_, e := m.Store.DB.ExecContext(ctx, `UPDATE account_mail_outbox SET status='sent',ciphertext=NULL,lease_id=NULL,lease_until=NULL,last_failure=NULL,sent_at=clock_timestamp() WHERE id=$1 AND lease_id=$2 AND status='sending'`, d.ID, d.Lease)
		return e
	}
	if terminal || d.Attempts >= 8 {
		status := "failed"
		if result == "ineligible" {
			status = "cancelled"
		}
		_, e := m.Store.DB.ExecContext(ctx, `UPDATE account_mail_outbox SET status=$3,ciphertext=NULL,lease_id=NULL,lease_until=NULL,last_failure=$4 WHERE id=$1 AND lease_id=$2 AND status='sending'`, d.ID, d.Lease, status, result)
		return e
	}
	// Stable jitter avoids a fleet retry burst while remaining restart-safe.
	jitter := int(security.Hash(d.ID)[0]) % 16
	delay := min(3600, 15*(1<<min(d.Attempts, 8))) + jitter
	_, e := m.Store.DB.ExecContext(ctx, `UPDATE account_mail_outbox SET status='pending',lease_id=NULL,lease_until=NULL,last_failure=$3,next_attempt_at=clock_timestamp()+($4 * interval '1 second') WHERE id=$1 AND lease_id=$2 AND status='sending'`, d.ID, d.Lease, result, delay)
	return e
}
