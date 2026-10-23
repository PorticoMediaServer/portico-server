package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/identity"
	"time"
)

// Central notifications cover account security and server reachability only.
// Media, storage and invitations are private to each server.
type NotificationPreferences struct {
	ServerOffline bool `json:"serverOffline"`
}

// NotificationPreferences hides owner-alert kinds the account turned off.
// Security events and sign-ins are always shown.
func (s *AccountSecurity) NotificationPreferences(ctx context.Context, token string, update *NotificationPreferences) (NotificationPreferences, error) {
	out := NotificationPreferences{ServerOffline: true}
	tx, a, e := s.authorize(ctx, token)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	if update != nil {
		_, e = tx.ExecContext(ctx, `INSERT INTO account_notification_preferences(account_id,server_offline) VALUES($1,$2) ON CONFLICT(account_id) DO UPDATE SET server_offline=$2`, a.AccountID, update.ServerOffline)
		if e != nil {
			return out, e
		}
	}
	e = tx.QueryRowContext(ctx, `SELECT server_offline FROM account_notification_preferences WHERE account_id=$1`, a.AccountID).Scan(&out.ServerOffline)
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
const visibleNotifications = `n.account_id=$1 AND NOT EXISTS(SELECT 1 FROM account_notification_preferences p WHERE p.account_id=n.account_id AND (n.kind='server_offline' AND NOT p.server_offline))`

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
