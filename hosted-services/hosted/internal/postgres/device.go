package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"portico.local/hosted/internal/device"
	"portico.local/hosted/internal/identity"
	"time"
)

func (s *Store) DeviceLimit(ctx context.Context, subject []byte, kind string, limit int, window time.Duration) error {
	var count int
	seconds := int(window.Seconds())
	e := s.DB.QueryRowContext(ctx, `INSERT INTO device_rate_limits(subject_hash,kind,window_start,attempts) VALUES($1,$2,now(),1) ON CONFLICT(subject_hash,kind) DO UPDATE SET attempts=CASE WHEN device_rate_limits.window_start<now()-make_interval(secs=>$3) THEN 1 ELSE device_rate_limits.attempts+1 END,window_start=CASE WHEN device_rate_limits.window_start<now()-make_interval(secs=>$3) THEN now() ELSE device_rate_limits.window_start END RETURNING attempts`, subject, kind, seconds).Scan(&count)
	if e != nil {
		return e
	}
	if count > limit {
		return &device.ProtocolError{Code: "rate_limited", Interval: seconds}
	}
	return nil
}
func (s *Store) StartDevice(ctx context.Context, c device.Creation) (device.Created, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return device.Created{}, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(encode($1::bytea,'hex'),19500))`, c.CreationHash); e != nil {
		return device.Created{}, e
	}
	var out device.Created
	var hash []byte
	var status string
	e = tx.QueryRowContext(ctx, `SELECT id,request_hash,creation_cipher,expires_at,interval_seconds,status FROM device_authorizations WHERE creation_hash=$1`, c.CreationHash).Scan(&out.ID, &hash, &out.Cipher, &out.ExpiresAt, &out.Interval, &status)
	if e == nil {
		if !bytes.Equal(c.RequestHash, hash) {
			return out, device.ErrConflict
		}
		if !out.ExpiresAt.After(time.Now()) || status != "pending" {
			return out, &device.ProtocolError{Code: "invalid_grant"}
		}
		return out, tx.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	inserted, e := tx.ExecContext(ctx, `INSERT INTO device_authorizations(id,creation_hash,request_hash,creation_cipher,device_hash,user_hash,device_name,platform,app_version,expires_at,session_mode,protocol,tv_public_key,client_request_id,next_poll_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,now()+interval '5 seconds') ON CONFLICT(user_hash) DO NOTHING`, c.ID, c.CreationHash, c.RequestHash, c.Cipher, c.DeviceHash, c.UserHash, c.DeviceName, c.Platform, c.AppVersion, c.ExpiresAt, c.SessionMode, c.Protocol, c.PublicKey, c.RequestID)
	if e != nil {
		return out, e
	}
	count, e := inserted.RowsAffected()
	if e != nil {
		return out, e
	}
	if count != 1 {
		return out, device.ErrCodeCollision
	}
	return device.Created{ID: c.ID, Cipher: c.Cipher, ExpiresAt: c.ExpiresAt, Interval: 5}, tx.Commit()
}
func (s *Store) ReviewDevice(ctx context.Context, account string, hash []byte) (device.Preview, error) {
	var p device.Preview
	e := s.DB.QueryRowContext(ctx, `UPDATE device_authorizations SET reviewed_account_id=$2 WHERE user_hash=$1 AND status='pending' AND expires_at>clock_timestamp() AND (reviewed_account_id IS NULL OR reviewed_account_id=$2) AND EXISTS(SELECT 1 FROM accounts WHERE id=$2) RETURNING id,device_name,platform,app_version,expires_at,protocol,tv_public_key,client_request_id`, hash, account).Scan(&p.RequestID, &p.DeviceName, &p.Platform, &p.AppVersion, &p.ExpiresAt, &p.Protocol, &p.PublicKey, &p.ClientRequestID)
	if errors.Is(e, sql.ErrNoRows) {
		return p, &device.ProtocolError{Code: "invalid_grant"}
	}
	return p, e
}
func (s *Store) DecideDevice(ctx context.Context, expected identity.CurrentFamilyExpectation, id string, hash []byte, decision string) error {
	if decision != "approve" && decision != "deny" {
		return device.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, expected)
	if e != nil {
		return e
	}
	account := facts.AccountID
	var status string
	var owner, reviewer sql.NullString
	var expiry time.Time
	e = tx.QueryRowContext(ctx, `SELECT status,approved_account_id,expires_at,reviewed_account_id FROM device_authorizations WHERE id=$1 AND user_hash=$2 FOR UPDATE`, id, hash).Scan(&status, &owner, &expiry, &reviewer)
	if errors.Is(e, sql.ErrNoRows) {
		return &device.ProtocolError{Code: "invalid_grant"}
	}
	if e != nil {
		return e
	}
	if reviewer.String != account {
		return device.ErrConflict
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return e
	}
	if !facts.UnexpiredAt(now) {
		return identity.ErrCredentials
	}
	if !expiry.After(now) {
		return &device.ProtocolError{Code: "expired_token"}
	}
	next := "approved"
	if decision == "deny" {
		next = "denied"
	}
	if status == next && owner.String == account {
		return tx.Commit()
	}
	if status != "pending" {
		return device.ErrConflict
	}
	epoch := facts.AuthEpoch
	if _, e = tx.ExecContext(ctx, `UPDATE device_authorizations SET status=$2,approved_account_id=$3,approved_epoch=$4,decided_at=now() WHERE id=$1`, id, next, account, epoch); e != nil {
		return e
	}
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return e
	}
	if !facts.UnexpiredAt(now) || !expiry.After(now) {
		return identity.ErrCredentials
	}
	if e = audit(ctx, tx, account, "device."+decision, id, 0); e != nil {
		return e
	}
	return tx.Commit()
}

var errDeviceAccountChanged = errors.New("device approval changed while locking")

func (s *Store) PollDevice(ctx context.Context, hash []byte, issue device.Issue) (device.Grant, error) {
	for range 2 {
		out, e := s.pollDevice(ctx, hash, issue)
		if !errors.Is(e, errDeviceAccountChanged) {
			return out, e
		}
	}
	return device.Grant{}, device.ErrConflict
}
func (s *Store) pollDevice(ctx context.Context, hash []byte, issue device.Issue) (device.Grant, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return device.Grant{}, e
	}
	defer tx.Rollback()
	preliminary, e := lockDeviceAccountTx(ctx, tx, hash)
	if errors.Is(e, sql.ErrNoRows) {
		return device.Grant{}, &device.ProtocolError{Code: "invalid_grant"}
	}
	if e != nil {
		return device.Grant{}, e
	}
	var id, status, sessionMode, protocol, publicKey, requestID string
	var protected []byte
	var account sql.NullString
	var expires, next time.Time
	var interval int
	var epoch sql.NullInt64
	var cipher, sessionHash []byte
	var sessionExpiry, recovery sql.NullTime
	e = tx.QueryRowContext(ctx, `SELECT id,status,approved_account_id,expires_at,next_poll_at,interval_seconds,approved_epoch,session_cipher,session_hash,session_expires_at,recovery_until,session_mode,protocol,tv_public_key,client_request_id,tv_grant FROM device_authorizations WHERE device_hash=$1 FOR UPDATE`, hash).Scan(&id, &status, &account, &expires, &next, &interval, &epoch, &cipher, &sessionHash, &sessionExpiry, &recovery, &sessionMode, &protocol, &publicKey, &requestID, &protected)
	if errors.Is(e, sql.ErrNoRows) {
		return device.Grant{}, &device.ProtocolError{Code: "invalid_grant"}
	}
	if e != nil {
		return device.Grant{}, e
	}
	if account != preliminary {
		return device.Grant{}, errDeviceAccountChanged
	}
	if sessionMode != issue.Family.Mode || protocol != issue.Protocol || (protocol == device.TVProtocol && issue.SealGrant == nil) {
		return device.Grant{}, &device.ProtocolError{Code: "invalid_grant"}
	}
	if status == "denied" || status == "cancelled" {
		return device.Grant{}, &device.ProtocolError{Code: "access_denied"}
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return device.Grant{}, e
	}
	if status == "consumed" {
		if !recovery.Valid || !recovery.Time.After(now) {
			return device.Grant{}, &device.ProtocolError{Code: "invalid_grant"}
		}
	} else if !expires.After(now) {
		return device.Grant{}, &device.ProtocolError{Code: "expired_token"}
	}
	if next.After(now) {
		interval = min(30, interval+5)
		if _, e = tx.ExecContext(ctx, `UPDATE device_authorizations SET interval_seconds=$2,next_poll_at=now()+make_interval(secs=>$2::integer) WHERE id=$1`, id, interval); e != nil {
			return device.Grant{}, e
		}
		if e = tx.Commit(); e != nil {
			return device.Grant{}, e
		}
		return device.Grant{}, &device.ProtocolError{Code: "slow_down", Interval: interval}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE device_authorizations SET next_poll_at=now()+make_interval(secs=>$2::integer) WHERE id=$1`, id, interval); e != nil {
		return device.Grant{}, e
	}
	if status == "pending" {
		if e = tx.Commit(); e != nil {
			return device.Grant{}, e
		}
		return device.Grant{}, &device.ProtocolError{Code: "authorization_pending", Interval: interval}
	}
	var v identity.Viewer
	var currentEpoch int64
	if e = tx.QueryRowContext(ctx, `SELECT id,username,display_name,auth_epoch FROM accounts WHERE id=$1 FOR SHARE`, account.String).Scan(&v.Account.ID, &v.Account.Username, &v.Account.DisplayName, &currentEpoch); e != nil {
		return device.Grant{}, e
	}
	if !epoch.Valid || epoch.Int64 != currentEpoch {
		return device.Grant{}, &device.ProtocolError{Code: "access_denied"}
	}
	if status == "consumed" {
		var valid bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_sessions a JOIN session_families f ON f.id=a.family_id WHERE a.token_hash=$1 AND a.account_id=$2 AND a.expires_at>now() AND f.generation=0 AND f.revoked_at IS NULL)`, sessionHash, account.String).Scan(&valid); e != nil {
			return device.Grant{}, e
		}
		if !valid {
			return device.Grant{}, &device.ProtocolError{Code: "access_denied"}
		}
	} else {
		if e = insertFamily(ctx, tx, account.String, issue.Hash, issue.ExpiresAt, issue.Family); e != nil {
			return device.Grant{}, e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE hosted_devices d SET id=a.id,name=a.device_name,platform=a.platform,app_version=a.app_version,session_hash=$3 FROM device_authorizations a WHERE a.id=$1 AND d.account_id=$2 AND d.family_id=$4`, id, account.String, issue.Hash, issue.Family.ID); e != nil {
			return device.Grant{}, e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE device_authorizations SET status='consumed',session_hash=$2,session_cipher=$3,session_expires_at=$4,recovery_until=now()+interval '5 minutes',consumed_at=now() WHERE id=$1`, id, issue.Hash, issue.Cipher, issue.ExpiresAt); e != nil {
			return device.Grant{}, e
		}
		cipher = issue.Cipher
		sessionExpiry = sql.NullTime{Time: issue.ExpiresAt, Valid: true}
		if e = audit(ctx, tx, account.String, "device.consume", id, 0); e != nil {
			return device.Grant{}, e
		}
	}
	// Encryption and the exact replay receipt precede the SAME commit as Device
	// and credential insertion. A failed seal cannot leave an issued family.
	if protocol == device.TVProtocol {
		if status != "consumed" {
			protected, e = issue.SealGrant(requestID, publicKey, v)
			if e != nil {
				return device.Grant{}, e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE device_authorizations SET tv_grant=$2 WHERE id=$1`, id, protected); e != nil {
				return device.Grant{}, e
			}
		}
		if len(protected) == 0 {
			return device.Grant{}, device.ErrInvalid
		}
	}
	return device.Grant{Cipher: cipher, Protected: protected, ExpiresAt: sessionExpiry.Time, Viewer: v}, tx.Commit()
}
func (s *Store) CancelDevice(ctx context.Context, hash []byte) error {
	for range 2 {
		e := s.cancelDevice(ctx, hash)
		if !errors.Is(e, errDeviceAccountChanged) {
			return e
		}
	}
	return device.ErrConflict
}
func (s *Store) cancelDevice(ctx context.Context, hash []byte) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	preliminary, e := lockDeviceAccountTx(ctx, tx, hash)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	var id string
	var session []byte
	var account sql.NullString
	e = tx.QueryRowContext(ctx, `SELECT id,session_hash,approved_account_id FROM device_authorizations WHERE device_hash=$1 FOR UPDATE`, hash).Scan(&id, &session, &account)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if account != preliminary {
		return errDeviceAccountChanged
	}
	var family sql.NullString
	if e = tx.QueryRowContext(ctx, `SELECT family_id FROM hosted_devices WHERE id=$1`, id).Scan(&family); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if family.Valid {
		if _, e = tx.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR SHARE`, account.String); e != nil {
			return e
		}
		if e = revokeFamily(ctx, tx, family.String, "device_cancel"); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM account_sessions WHERE token_hash=$1`, session); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE hosted_devices SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1`, id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE device_authorizations SET status='cancelled',session_cipher=NULL,tv_grant=NULL WHERE id=$1`, id); e != nil {
		return e
	}
	if e = audit(ctx, tx, account.String, "device.cancel", id, 0); e != nil {
		return e
	}
	return tx.Commit()
}

// No operation holding a device row may wait for an account lock: deletion
// takes account first and later deletes that same device row.
func lockDeviceAccountTx(ctx context.Context, tx *sql.Tx, hash []byte) (sql.NullString, error) {
	var account sql.NullString
	e := tx.QueryRowContext(ctx, `SELECT approved_account_id FROM device_authorizations WHERE device_hash=$1`, hash).Scan(&account)
	if e != nil {
		return account, e
	}
	if account.Valid {
		var id string
		e = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR SHARE`, account.String).Scan(&id)
	}
	return account, e
}
