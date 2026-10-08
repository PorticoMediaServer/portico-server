package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
	"time"
)

const notificationSchema = `CREATE TABLE account_notification_preferences(account_id text PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,server_offline boolean NOT NULL DEFAULT true,storage_nearly_full boolean NOT NULL DEFAULT true,invitation_accepted boolean NOT NULL DEFAULT true);
CREATE TABLE account_notification_events(id text PRIMARY KEY,account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,server_id text REFERENCES servers(id),kind text NOT NULL CHECK(kind IN('server_offline','storage_nearly_full','invitation_accepted','new_device')),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),processed_at timestamptz);
CREATE INDEX notification_pending ON account_notification_events(created_at) WHERE processed_at IS NULL;`

// In-app notifications (Justin, 23 Sep: no push notifications, and email only
// for account setup, invitations and password reset). Owner alerts, security
// events, new sign-ins, export completion and the operator's key-expiry
// warning are rows in account_notification_events, which the account reads as
// a feed in the web Account page and the apps' Notifications screen.
//
// Kinds (clients own the words; detail carries only codes and identifiers):
//   server_offline        server{id,name}
//   storage_nearly_full   server{id,name}
//   invitation_accepted   server{id,name}
//   new_device            detail{deviceId,name,platform,app}
//   security_event        detail{event}: password_changed, password_reset,
//                         recovery_codes_replaced, sign_in_method_removed,
//                         provider_linked, two_step_enabled, two_step_disabled,
//                         contact_email_changed, contact_email_verified
//   account_export_ready  detail{exportId}
//   signing_key_expiring  detail{keyId,expiresAt} (operator account only)

type NotificationPreferences struct {
	ServerOffline      bool `json:"serverOffline"`
	StorageNearlyFull  bool `json:"storageNearlyFull"`
	InvitationAccepted bool `json:"invitationAccepted"`
}

// NotificationPreferences hides owner-alert kinds the account turned off.
// Security events and sign-ins are always shown.
func (s *AccountSecurity) NotificationPreferences(ctx context.Context, token string, update *NotificationPreferences) (NotificationPreferences, error) {
	out := NotificationPreferences{ServerOffline: true, StorageNearlyFull: true, InvitationAccepted: true}
	tx, a, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	if update != nil {
		_, e = tx.ExecContext(ctx, `INSERT INTO account_notification_preferences(account_id,server_offline,storage_nearly_full,invitation_accepted) VALUES($1,$2,$3,$4) ON CONFLICT(account_id) DO UPDATE SET server_offline=$2,storage_nearly_full=$3,invitation_accepted=$4`, a.AccountID, update.ServerOffline, update.StorageNearlyFull, update.InvitationAccepted)
		if e != nil {
			return out, e
		}
	}
	e = tx.QueryRowContext(ctx, `SELECT server_offline,storage_nearly_full,invitation_accepted FROM account_notification_preferences WHERE account_id=$1`, a.AccountID).Scan(&out.ServerOffline, &out.StorageNearlyFull, &out.InvitationAccepted)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	return out, tx.Commit()
}

type NotificationServer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Notification struct {
	ID         string              `json:"id"`
	Kind       string              `json:"kind"`
	OccurredAt time.Time           `json:"occurredAt"`
	Read       bool                `json:"read"`
	Server     *NotificationServer `json:"server,omitempty"`
	Detail     json.RawMessage     `json:"detail"`
}
type NotificationFeed struct {
	Items      []Notification `json:"items"`
	Unread     int            `json:"unread"`
	NextCursor string         `json:"nextCursor,omitempty"`
}

const notificationPage = 50

// visibleNotifications is the account's feed filter: its own rows, minus the
// owner-alert kinds it turned off.
const visibleNotifications = `n.account_id=$1 AND NOT EXISTS(SELECT 1 FROM account_notification_preferences p WHERE p.account_id=n.account_id AND (n.kind='server_offline' AND NOT p.server_offline OR n.kind='storage_nearly_full' AND NOT p.storage_nearly_full OR n.kind='invitation_accepted' AND NOT p.invitation_accepted))`

func scanNotification(rows interface{ Scan(...any) error }) (Notification, error) {
	var n Notification
	var server, name sql.NullString
	var read sql.NullTime
	if e := rows.Scan(&n.ID, &n.Kind, &n.OccurredAt, &read, &server, &name, &n.Detail); e != nil {
		return n, e
	}
	n.Read = read.Valid
	if server.Valid {
		n.Server = &NotificationServer{ID: server.String, Name: name.String}
	}
	return n, nil
}

// Notifications is one page of the feed, newest first. The cursor is the last
// item's id; ids are unique, so (created_at,id) orders the keyset.
func (s *AccountSecurity) Notifications(ctx context.Context, token, cursor string) (NotificationFeed, error) {
	out := NotificationFeed{Items: []Notification{}}
	tx, a, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT n.id,n.kind,n.created_at,n.read_at,n.server_id,s.name,n.detail FROM account_notification_events n LEFT JOIN servers s ON s.id=n.server_id WHERE `+visibleNotifications+` AND ($2='' OR (n.created_at,n.id)<(SELECT created_at,id FROM account_notification_events WHERE id=$2 AND account_id=$1)) ORDER BY n.created_at DESC,n.id DESC LIMIT $3`, a.AccountID, cursor, notificationPage+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		n, e := scanNotification(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, n)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > notificationPage {
		out.Items = out.Items[:notificationPage]
		out.NextCursor = out.Items[notificationPage-1].ID
	}
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM account_notification_events n WHERE `+visibleNotifications+` AND n.read_at IS NULL`, a.AccountID).Scan(&out.Unread); e != nil {
		return out, e
	}
	return out, tx.Commit()
}

// NotificationByID is one item, for a page opened from a notification.
func (s *AccountSecurity) NotificationByID(ctx context.Context, token, id string) (Notification, error) {
	tx, a, e := s.authorize(ctx, token)
	if e != nil {
		return Notification{}, e
	}
	defer tx.Rollback()
	n, e := scanNotification(tx.QueryRowContext(ctx, `SELECT n.id,n.kind,n.created_at,n.read_at,n.server_id,s.name,n.detail FROM account_notification_events n LEFT JOIN servers s ON s.id=n.server_id WHERE `+visibleNotifications+` AND n.id=$2`, a.AccountID, id))
	if errors.Is(e, sql.ErrNoRows) {
		return n, identity.ErrInvalid
	}
	if e != nil {
		return n, e
	}
	return n, tx.Commit()
}

// MarkNotificationsRead marks the given items (or all, when ids is empty and
// all is set) read for this account.
func (s *AccountSecurity) MarkNotificationsRead(ctx context.Context, token string, ids []string, all bool) error {
	if len(ids) > notificationPage || len(ids) == 0 && !all {
		return identity.ErrInvalid
	}
	tx, a, e := s.authorize(ctx, token)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if all {
		_, e = tx.ExecContext(ctx, `UPDATE account_notification_events SET read_at=clock_timestamp() WHERE account_id=$1 AND read_at IS NULL`, a.AccountID)
	} else {
		_, e = tx.ExecContext(ctx, `UPDATE account_notification_events SET read_at=clock_timestamp() WHERE account_id=$1 AND read_at IS NULL AND id=ANY($2)`, a.AccountID, ids)
	}
	if e != nil {
		return e
	}
	return tx.Commit()
}

// notifyAccountTx adds one feed item inside the caller's transaction.
func notifyAccountTx(ctx context.Context, tx *sql.Tx, id, account, server, kind string, detail any) error {
	raw, e := json.Marshal(detail)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO account_notification_events(id,account_id,server_id,kind,detail) VALUES($1,$2,NULLIF($3,''),$4,$5) ON CONFLICT DO NOTHING`, id, account, server, kind, raw)
	return e
}

func (s *Store) ReportStorageWarning(ctx context.Context, a directory.ServerAuthority) error {
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO account_notification_events(id,account_id,server_id,kind) VALUES('storage_'||md5($1||current_date::text),$2,$1,'storage_nearly_full') ON CONFLICT DO NOTHING`, a.ServerID, a.AccountID)
	if e != nil {
		return e
	}
	return tx.Commit()
}

// NextOfflineDue is when the next unreported server crosses the 24-hour
// offline threshold (zero when none will). A75: the offline worker sleeps until
// exactly then instead of scanning on a clock.
func (s *AccountSecurity) NextOfflineDue(ctx context.Context) (time.Time, error) {
	var next sql.NullTime
	err := s.Store.DB.QueryRowContext(ctx, `SELECT min(presence_expires_at)+interval '24 hours' FROM servers WHERE revoked_at IS NULL AND offline_notified_for IS DISTINCT FROM last_seen_at`).Scan(&next)
	if err != nil || !next.Valid {
		return time.Time{}, err
	}
	return next.Time, nil
}

// ScanOfflineServers reports every server now past the offline threshold, in
// batches, until none is left (A75), marking each with the silence it reported
// so a server is reported once per silence (A54).
func (s *AccountSecurity) ScanOfflineServers(ctx context.Context) error {
	for {
		tx, e := s.Store.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		// A coarse lease cannot identify an unannounced power failure early.
		result, e := tx.ExecContext(ctx, `WITH due AS (SELECT id,owner_account_id,last_seen_at FROM servers WHERE revoked_at IS NULL AND presence_expires_at<clock_timestamp()-interval '24 hours' AND offline_notified_for IS DISTINCT FROM last_seen_at ORDER BY presence_expires_at LIMIT 100 FOR UPDATE SKIP LOCKED),
 marked AS (UPDATE servers s SET offline_notified_for=due.last_seen_at FROM due WHERE s.id=due.id RETURNING s.id,due.owner_account_id,due.last_seen_at),
 feed AS (INSERT INTO account_notification_events(id,account_id,server_id,kind) SELECT 'offline_'||md5(id||last_seen_at::text),owner_account_id,id,'server_offline' FROM marked WHERE owner_account_id IS NOT NULL AND last_seen_at>clock_timestamp()-interval '90 days' ON CONFLICT DO NOTHING)
 SELECT 1 FROM marked`)
		if e != nil {
			tx.Rollback()
			return e
		}
		n, _ := result.RowsAffected()
		if e = tx.Commit(); e != nil {
			return e
		}
		if n < 100 {
			return nil
		}
	}
}

// SigningKeyAlertWindow is how long before expiry the operator is alerted.
const SigningKeyAlertWindow = 30 * 24 * time.Hour

// QueueSigningKeyAlert puts one key-expiry item in the operator's feed per key.
func (s *AccountSecurity) QueueSigningKeyAlert(ctx context.Context, account, keyID string, expires time.Time) error {
	if account == "" || keyID == "" {
		return nil
	}
	raw, _ := json.Marshal(map[string]string{"keyId": keyID, "expiresAt": expires.UTC().Format(time.RFC3339)})
	_, e := s.Store.DB.ExecContext(ctx, `INSERT INTO account_notification_events(id,account_id,kind,detail) SELECT 'keyexp_'||md5($2),$1,'signing_key_expiring',$3 WHERE EXISTS(SELECT 1 FROM accounts WHERE id=$1) ON CONFLICT DO NOTHING`, account, keyID, raw)
	return e
}
