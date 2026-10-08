package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
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

// AccountPreferences are account-level, not per-server: a person has one home time
// zone and one answer about whether their activity may be shown to the other
// members of their household. Per-server viewer preferences stay on the server.
type AccountPreferences struct {
	Locale                 string `json:"locale"`
	TimeZone               string `json:"timeZone"`
	HourCycle              string `json:"hourCycle"`
	PauseWatchHistory      bool   `json:"pauseWatchHistory"`
	ShowActivityToMembers  bool   `json:"showActivityToMembers"`
	IncludeInWatchTogether bool   `json:"includeInWatchTogether"`
	MarketingEmail         bool   `json:"marketingEmail"`
	Revision               int64  `json:"revision"`
}

// AccountPreferenceEdit is a full replacement guarded by ExpectedRevision.
type AccountPreferenceEdit struct {
	ExpectedRevision int64 `json:"expectedRevision"`
	AccountPreferences
}

func defaultAccountPreferences() AccountPreferences {
	return AccountPreferences{HourCycle: "auto", ShowActivityToMembers: true, IncludeInWatchTogether: true, Revision: 1}
}

// A locale or time zone is stored as the client reported it, but it is bounded and
// screened: these values are echoed back into other people's documents, so a
// control character or an unbounded string must never enter the table.
func validPreferenceText(v string, max int) bool {
	if len(v) > max {
		return false
	}
	for _, r := range v {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func validHourCycle(v string) bool {
	switch v {
	case "auto", "h11", "h12", "h23", "h24":
		return true
	}
	return false
}

// Preferences reads the account's preferences. A missing row is the documented
// default, not an error.
func (s *AccountSecurity) Preferences(ctx context.Context, token string) (AccountPreferences, error) {
	out := defaultAccountPreferences()
	tx, expected, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	e = tx.QueryRowContext(ctx, `SELECT locale,time_zone,hour_cycle,pause_watch_history,show_activity_to_members,include_in_watch_together,marketing_email,revision FROM account_preferences WHERE account_id=$1`, expected.AccountID).Scan(&out.Locale, &out.TimeZone, &out.HourCycle, &out.PauseWatchHistory, &out.ShowActivityToMembers, &out.IncludeInWatchTogether, &out.MarketingEmail, &out.Revision)
	if errors.Is(e, sql.ErrNoRows) {
		return defaultAccountPreferences(), tx.Commit()
	}
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}

// SetPreferences replaces the account's preferences.
func (s *AccountSecurity) SetPreferences(ctx context.Context, token string, edit AccountPreferenceEdit) (AccountPreferences, error) {
	if edit.ExpectedRevision < 1 || !validPreferenceText(edit.Locale, 35) || !validPreferenceText(edit.TimeZone, 64) || !validHourCycle(edit.HourCycle) {
		return AccountPreferences{}, ErrPreferenceInput
	}
	edit.Locale = strings.TrimSpace(edit.Locale)
	edit.TimeZone = strings.TrimSpace(edit.TimeZone)
	tx, expected, e := s.authorize(ctx, token)
	if e != nil {
		return AccountPreferences{}, e
	}
	defer tx.Rollback()
	current := defaultAccountPreferences()
	e = tx.QueryRowContext(ctx, `SELECT revision FROM account_preferences WHERE account_id=$1`, expected.AccountID).Scan(&current.Revision)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return AccountPreferences{}, e
	}
	if current.Revision != edit.ExpectedRevision {
		return AccountPreferences{}, ErrPreferenceChanged
	}
	out := edit.AccountPreferences
	out.Revision = current.Revision + 1
	if _, e = tx.ExecContext(ctx, `INSERT INTO account_preferences(account_id,locale,time_zone,hour_cycle,pause_watch_history,show_activity_to_members,include_in_watch_together,marketing_email,revision,updated_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,clock_timestamp())
 ON CONFLICT(account_id) DO UPDATE SET locale=excluded.locale,time_zone=excluded.time_zone,hour_cycle=excluded.hour_cycle,pause_watch_history=excluded.pause_watch_history,show_activity_to_members=excluded.show_activity_to_members,include_in_watch_together=excluded.include_in_watch_together,marketing_email=excluded.marketing_email,revision=account_preferences.revision+1,updated_at=clock_timestamp()`,
		expected.AccountID, out.Locale, out.TimeZone, out.HourCycle, out.PauseWatchHistory, out.ShowActivityToMembers, out.IncludeInWatchTogether, out.MarketingEmail, out.Revision); e != nil {
		return AccountPreferences{}, e
	}
	return out, tx.Commit()
}

// InvitationSummary is the invitation state the account document carries: enough
// for a client to draw a badge and a deadline without listing every invitation.
// The effective status uses the same SQL expression the invitation history uses,
// so the two can never disagree about what "expired" means.
type InvitationSummary struct {
	Pending        int    `json:"pending"`
	Expired        int    `json:"expired"`
	Conflicting    int    `json:"conflicting"`
	NextExpiresAt  string `json:"nextExpiresAt,omitempty"`
	LatestReceived string `json:"latestReceivedAt,omitempty"`
}

// AccountDocument is the one document an account screen needs: who the account
// is, its security state, its account-level preferences and its invitation state.
type AccountDocument struct {
	Security    identity.SecurityState `json:"security"`
	Preferences AccountPreferences     `json:"preferences"`
	Invitations InvitationSummary      `json:"invitations"`
}

// Document assembles the account screen in one authorized transaction, so the
// three parts cannot be read across a revocation that lands between them.
func (s *AccountSecurity) Document(ctx context.Context, token string) (AccountDocument, error) {
	out := AccountDocument{Preferences: defaultAccountPreferences()}
	security, e := s.State(ctx, token)
	if e != nil {
		return out, e
	}
	out.Security = security
	tx, expected, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	e = tx.QueryRowContext(ctx, `SELECT locale,time_zone,hour_cycle,pause_watch_history,show_activity_to_members,include_in_watch_together,marketing_email,revision FROM account_preferences WHERE account_id=$1`, expected.AccountID).Scan(&out.Preferences.Locale, &out.Preferences.TimeZone, &out.Preferences.HourCycle, &out.Preferences.PauseWatchHistory, &out.Preferences.ShowActivityToMembers, &out.Preferences.IncludeInWatchTogether, &out.Preferences.MarketingEmail, &out.Preferences.Revision)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	var next, latest sql.NullTime
	e = tx.QueryRowContext(ctx, `WITH effective AS(
 SELECT i.expires_at,
  CASE WHEN i.status='pending' AND i.expires_at<=now() THEN 'expired'
       WHEN i.status='pending' AND (i.expected_policy_revision<>s.policy_revision OR i.expected_local_revision<>s.local_revision OR i.owner_id<>s.owner_account_id) THEN 'conflict'
       ELSE i.status END AS status
 FROM invitations i JOIN servers s ON s.id=i.server_id WHERE i.recipient_id=$1)
 SELECT count(*) FILTER(WHERE status='pending'),count(*) FILTER(WHERE status='expired'),count(*) FILTER(WHERE status='conflict'),
  min(expires_at) FILTER(WHERE status='pending'),max(expires_at)
 FROM effective`, expected.AccountID).Scan(&out.Invitations.Pending, &out.Invitations.Expired, &out.Invitations.Conflicting, &next, &latest)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	if next.Valid {
		out.Invitations.NextExpiresAt = next.Time.UTC().Format(time.RFC3339)
	}
	if latest.Valid {
		out.Invitations.LatestReceived = latest.Time.UTC().Format(time.RFC3339)
	}
	return out, tx.Commit()
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
