package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"portico.local/hosted/internal/appstore"
	"portico.local/hosted/internal/identity"
)

var ErrPreferenceInput = errors.New("invalid account preference")
var ErrPreferenceChanged = errors.New("These preferences changed. Refresh before trying again.")

// AccountDevice is a device the account holder recognises. It is the read side of
// hosted_devices, which the device-authorization flow has always written and
// nothing has ever read back.
type AccountDevice struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	App        string `json:"app"`
	AppVersion string `json:"appVersion"`
	FamilyID   string `json:"familyId"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"`
	Trusted    bool   `json:"trusted"`
	Status     string `json:"status"`
	Current    bool   `json:"current"`
}

// AccountDeviceList is the paged device document.
type AccountDeviceList struct {
	AccountID       string          `json:"accountId"`
	CurrentFamilyID string          `json:"currentFamilyId"`
	ObservedAt      string          `json:"observedAt"`
	Items           []AccountDevice `json:"items"`
}

// Devices lists the account's devices. Status is derived from the session family
// the device holds, exactly as session review derives it, so the two surfaces
// cannot disagree about whether a device is signed in.
func (s *AccountSecurity) Devices(ctx context.Context, token string) (AccountDeviceList, error) {
	out := AccountDeviceList{Items: []AccountDevice{}}
	tx, expected, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	out.AccountID = expected.AccountID
	out.CurrentFamilyID = expected.FamilyID
	out.ObservedAt = time.Now().UTC().Format(time.RFC3339)
	rows, e := tx.QueryContext(ctx, `SELECT d.id,d.name,d.platform,d.app,d.app_version,d.family_id,d.created_at,d.last_seen_at,d.trusted,
 CASE WHEN d.revoked_at IS NOT NULL THEN 'signed-out'
      WHEN f.revoked_at IS NOT NULL THEN 'signed-out'
      WHEN LEAST(f.idle_expires_at,f.absolute_expires_at)<=clock_timestamp() THEN 'expired'
      ELSE 'active' END
 FROM hosted_devices d LEFT JOIN session_families f ON f.id=d.family_id AND f.account_id=d.account_id
 WHERE d.account_id=$1 ORDER BY d.last_seen_at DESC,d.id LIMIT 200`, expected.AccountID)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var record AccountDevice
		var created, seen time.Time
		if e = rows.Scan(&record.ID, &record.Name, &record.Platform, &record.App, &record.AppVersion, &record.FamilyID, &created, &seen, &record.Trusted, &record.Status); e != nil {
			rows.Close()
			return out, e
		}
		record.CreatedAt = created.UTC().Format(time.RFC3339)
		record.LastSeenAt = seen.UTC().Format(time.RFC3339)
		record.Current = record.FamilyID == expected.FamilyID
		out.Items = append(out.Items, record)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}

// AccountDocument carries central sign-in security only. Viewer preferences,
// profiles and invitations are private to the user's chosen server.
type AccountDocument struct {
	Security identity.SecurityState `json:"security"`
}

func (s *AccountSecurity) Document(ctx context.Context, token string) (AccountDocument, error) {
	security, e := s.State(ctx, token)
	return AccountDocument{Security: security}, e
}

// RecordAppStoreNotification stores one verified notification. The uuid is the
// primary key, so Apple's at-least-once retries are idempotent rather than
// duplicated. The raw JWS is never stored.
//
// appAccountToken is how a notification is attached to an account: the app sets
// it when it starts a purchase, and it is the only link between a StoreKit
// transaction and a Portico account. An unmatched token is stored unattached
// rather than dropped, so a purchase made before the account existed can still be
// reconciled.
func (s *AccountSecurity) RecordAppStoreNotification(ctx context.Context, n appstore.Notification, claims appstore.TransactionClaims) error {
	payload, err := json.Marshal(n)
	if err != nil {
		return err
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var account sql.NullString
	if claims.AppAccountToken != "" {
		var matched string
		if err = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1`, claims.AppAccountToken).Scan(&matched); err == nil {
			account = sql.NullString{String: matched, Valid: true}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	var signed sql.NullTime
	if at := n.SignedAt(); !at.IsZero() {
		signed = sql.NullTime{Time: at, Valid: true}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO appstore_notifications(uuid,notification_type,subtype,bundle_id,environment,original_transaction_id,app_account_token,account_id,signed_at,payload)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(uuid) DO NOTHING`,
		n.UUID, n.NotificationType, n.Subtype, n.Data.BundleID, n.Data.Environment, claims.OriginalTransactionID, claims.AppAccountToken, account, signed, payload); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordAppStoreFailure notes that a notification was refused and why. The
// refused bytes are deliberately absent.
func (s *AccountSecurity) RecordAppStoreFailure(ctx context.Context, reason string) error {
	_, err := s.Store.DB.ExecContext(ctx, `INSERT INTO appstore_notification_failures(reason) VALUES($1)`, reason)
	return err
}

func (s *Store) IdentityEmail(ctx context.Context, account string) (string, error) {
	var email string
	e := s.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT address FROM account_contact_emails WHERE account_id=$1 AND is_primary AND verified_at IS NOT NULL),'')`, account).Scan(&email)
	return email, e
}
