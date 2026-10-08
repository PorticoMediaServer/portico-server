package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/sessions"
	"time"
)

func insertFamily(ctx context.Context, tx *sql.Tx, account string, access []byte, expiry time.Time, f identity.FamilySeed) error {
	if f.ID == "" || len(f.RefreshHash) != 32 || !identity.ValidMode(f.Mode) {
		return identity.ErrInvalid
	}
	if _, e := tx.ExecContext(ctx, `INSERT INTO session_families(id,account_id,mode,current_refresh_hash,absolute_expires_at,idle_expires_at,account_epoch) SELECT $1,id,$3,$4,now()+interval '90 days',LEAST($5,now()+interval '90 days'),auth_epoch FROM accounts WHERE id=$2`, f.ID, account, f.Mode, f.RefreshHash, f.RefreshExpiresAt); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `INSERT INTO refresh_tokens(token_hash,family_id,generation) VALUES($1,$2,0)`, f.RefreshHash, f.ID); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `INSERT INTO account_sessions(token_hash,account_id,expires_at,family_id) VALUES($1,$2,$3,$4)`, access, account, expiry, f.ID)
	if e != nil {
		return e
	}
	d := identity.DeviceFromContext(ctx)
	var known bool
	if d.InstallationID != "" {
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM hosted_devices WHERE account_id=$1 AND installation_id=$2 AND revoked_at IS NULL)`, account, d.InstallationID).Scan(&known); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO hosted_devices(id,account_id,name,platform,app_version,session_hash,family_id,app,installation_id,last_seen_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,clock_timestamp())`, "device_"+f.ID, account, d.Name, d.Platform, d.Version, access, f.ID, d.App, d.InstallationID); e != nil {
		return e
	}
	if !known {
		e = notifyAccountTx(ctx, tx, "signin_"+f.ID, account, "", "new_device", map[string]string{"deviceId": "device_" + f.ID, "name": d.Name, "platform": d.Platform, "app": d.App})
	}
	return e
}
func (s *Store) LookupRefresh(ctx context.Context, hash []byte) (sessions.Lookup, error) {
	var out sessions.Lookup
	e := s.DB.QueryRowContext(ctx, `SELECT f.id,f.mode,f.absolute_expires_at FROM refresh_tokens t JOIN session_families f ON f.id=t.family_id WHERE t.token_hash=$1`, hash).Scan(&out.FamilyID, &out.Mode, &out.AbsoluteExpiresAt)
	if errors.Is(e, sql.ErrNoRows) {
		return out, sessions.ErrGrant
	}
	return out, e
}
func revokeFamily(ctx context.Context, tx *sql.Tx, id, reason string) error {
	if _, e := tx.ExecContext(ctx, `UPDATE session_families SET revoked_at=COALESCE(revoked_at,now()),revoke_reason=COALESCE(revoke_reason,$2) WHERE id=$1`, id, reason); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `DELETE FROM account_sessions WHERE family_id=$1`, id); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `UPDATE hosted_devices SET revoked_at=COALESCE(revoked_at,now()) WHERE family_id=$1`, id)
	return e
}
func (s *Store) RotateRefresh(ctx context.Context, r sessions.Rotation) (sessions.Rotated, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return sessions.Rotated{}, e
	}
	defer tx.Rollback()
	var account string
	e = tx.QueryRowContext(ctx, `SELECT account_id FROM session_families WHERE id=$1`, r.FamilyID).Scan(&account)
	if errors.Is(e, sql.ErrNoRows) {
		return sessions.Rotated{}, sessions.ErrGrant
	}
	if e != nil {
		return sessions.Rotated{}, e
	}
	var out sessions.Rotated
	var accountEpoch int64
	if e = tx.QueryRowContext(ctx, `SELECT id,username,display_name,auth_epoch FROM accounts WHERE id=$1 FOR SHARE`, account).Scan(&out.Viewer.Account.ID, &out.Viewer.Account.Username, &out.Viewer.Account.DisplayName, &accountEpoch); e != nil {
		return out, e
	}
	var current []byte
	var generation, epoch int64
	var mode string
	var absolute, idle time.Time
	var revoked sql.NullTime
	if e = tx.QueryRowContext(ctx, `SELECT current_refresh_hash,generation,mode,absolute_expires_at,idle_expires_at,revoked_at,account_epoch FROM session_families WHERE id=$1 FOR UPDATE`, r.FamilyID).Scan(&current, &generation, &mode, &absolute, &idle, &revoked, &epoch); e != nil {
		return out, e
	}
	now := time.Now()
	if revoked.Valid || !absolute.After(now) || !idle.After(now) || epoch != accountEpoch || mode != r.Mode {
		return out, sessions.ErrGrant
	}
	var input, output, cipher []byte
	var receiptExpiry time.Time
	e = tx.QueryRowContext(ctx, `SELECT input_hash,output_hash,ciphertext,expires_at FROM refresh_receipts WHERE family_id=$1 AND request_id=$2`, r.FamilyID, r.RequestID).Scan(&input, &output, &cipher, &receiptExpiry)
	if e == nil {
		if !bytes.Equal(r.InputHash, input) && !bytes.Equal(r.InputHash, output) {
			if !bytes.Equal(current, r.InputHash) {
				if e = revokeFamily(ctx, tx, r.FamilyID, "refresh_reuse"); e != nil {
					return out, e
				}
				if e = tx.Commit(); e != nil {
					return out, e
				}
				return out, sessions.ErrReuse
			}
			return out, sessions.ErrConflict
		}
		// The exact answer is replayable until its successor refresh token is
		// first used (that rotation deletes this receipt, so a later replay is
		// reuse) or the family horizon passes. Every explicit revocation revokes
		// the family or bumps the account epoch, both checked above, so the
		// access token's own row is not consulted: after an outage longer than
		// its lifetime the client gets it back expired and refreshes with the
		// successor it now holds.
		if !bytes.Equal(current, output) || !receiptExpiry.After(now) || cipher == nil {
			return out, sessions.ErrGrant
		}
		out.Cipher = cipher
		out.RefreshExpiresAt = idle
	} else {
		if !errors.Is(e, sql.ErrNoRows) {
			return out, e
		}
		var used sql.NullTime
		var belongs string
		e = tx.QueryRowContext(ctx, `SELECT family_id,consumed_at FROM refresh_tokens WHERE token_hash=$1`, r.InputHash).Scan(&belongs, &used)
		if errors.Is(e, sql.ErrNoRows) {
			return out, sessions.ErrGrant
		}
		if e != nil {
			return out, e
		}
		if belongs != r.FamilyID {
			return out, sessions.ErrGrant
		}
		if used.Valid || !bytes.Equal(current, r.InputHash) {
			if e = revokeFamily(ctx, tx, r.FamilyID, "refresh_reuse"); e != nil {
				return out, e
			}
			if e = audit(ctx, tx, account, "session.refresh_reuse", r.FamilyID, 0); e != nil {
				return out, e
			}
			if e = tx.Commit(); e != nil {
				return out, e
			}
			return out, sessions.ErrReuse
		}
		if !r.ExpiresAt.After(now) || r.ExpiresAt.After(absolute) {
			return out, sessions.ErrGrant
		}
		if _, e = tx.ExecContext(ctx, `UPDATE refresh_tokens SET consumed_at=now() WHERE token_hash=$1`, r.InputHash); e != nil {
			return out, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO refresh_tokens(token_hash,family_id,generation) VALUES($1,$2,$3)`, r.OutputHash, r.FamilyID, generation+1); e != nil {
			return out, e
		}
		if e = tx.QueryRowContext(ctx, `UPDATE session_families SET current_refresh_hash=$2,generation=generation+1,absolute_expires_at=now()+interval '90 days',idle_expires_at=now()+interval '90 days' WHERE id=$1 RETURNING idle_expires_at`, r.FamilyID, r.OutputHash).Scan(&out.RefreshExpiresAt); e != nil {
			return out, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO account_sessions(token_hash,account_id,expires_at,family_id) VALUES($1,$2,$3,$4)`, r.AccessHash, account, r.ExpiresAt, r.FamilyID); e != nil {
			return out, e
		}
		// One receipt per family: this rotation is the successor's first use, so
		// the older receipts stop being replayable (a replay of them is reuse).
		// The new one lives until its own successor is used, capped at the
		// family horizon, so an answer lost in an outage of any length replays.
		if _, e = tx.ExecContext(ctx, `DELETE FROM refresh_receipts WHERE family_id=$1`, r.FamilyID); e != nil {
			return out, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO refresh_receipts(family_id,request_id,input_hash,output_hash,access_hash,ciphertext,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, r.FamilyID, r.RequestID, r.InputHash, r.OutputHash, r.AccessHash, r.Cipher, out.RefreshExpiresAt); e != nil {
			return out, e
		}
		if e = audit(ctx, tx, account, "session.refresh", r.FamilyID, generation+1); e != nil {
			return out, e
		}
		out.Cipher = r.Cipher
	}
	if _, e = tx.ExecContext(ctx, `UPDATE hosted_devices SET last_seen_at=clock_timestamp() WHERE family_id=$1 AND last_seen_at<clock_timestamp()-interval '6 hours'`, r.FamilyID); e != nil {
		return out, e
	}
	rows, e := tx.QueryContext(ctx, sessionProfilesQuery, account)
	if e != nil {
		return out, e
	}
	out.Viewer.Profiles = []identity.Profile{}
	for rows.Next() {
		var p identity.Profile
		if e = rows.Scan(&p.ID, &p.Name); e != nil {
			rows.Close()
			return out, e
		}
		out.Viewer.Profiles = append(out.Viewer.Profiles, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func (s *Store) RevokeRefresh(ctx context.Context, hash []byte, mode string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var id, account, actualMode string
	e = tx.QueryRowContext(ctx, `SELECT f.id,f.account_id,f.mode FROM refresh_tokens t JOIN session_families f ON f.id=t.family_id WHERE t.token_hash=$1`, hash).Scan(&id, &account, &actualMode)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if actualMode != mode {
		return sessions.ErrGrant
	}
	if _, e = tx.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR SHARE`, account); e != nil {
		return e
	}
	if e = revokeFamily(ctx, tx, id, "logout"); e != nil {
		return e
	}
	if e = audit(ctx, tx, account, "session.family_logout", id, 0); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) LogoutAll(ctx context.Context, account string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `UPDATE accounts SET auth_epoch=auth_epoch+1 WHERE id=$1`, account); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE session_families SET revoked_at=COALESCE(revoked_at,now()),revoke_reason=COALESCE(revoke_reason,'logout_all') WHERE account_id=$1`, account); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM account_sessions WHERE account_id=$1`, account); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE hosted_devices SET revoked_at=COALESCE(revoked_at,now()) WHERE account_id=$1`, account); e != nil {
		return e
	}
	if e = audit(ctx, tx, account, "session.logout_all", account, 0); e != nil {
		return e
	}
	return tx.Commit()
}
