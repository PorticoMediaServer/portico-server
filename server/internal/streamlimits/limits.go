// Package streamlimits resolves account stream settings against the owner's
// hard ceiling and counts the same durable slots for every admission surface.
// Callers own authorization and the SQLite transaction.
package streamlimits

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Mode string

const (
	Inherit   Mode = "inherit"
	Unlimited Mode = "unlimited"
	Limit     Mode = "limit"
)

func (Mode) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"inherit", "unlimited", "limit"}}
}
func Valid(mode Mode, value int) bool {
	return mode == Limit && value >= 1 && value <= 1000 || (mode == Inherit || mode == Unlimited) && value == 0
}

type Facts struct {
	Mode           Mode `json:"mode"`
	Limit          *int `json:"limit"`
	ServerCeiling  *int `json:"serverCeiling"`
	EffectiveLimit *int `json:"effectiveLimit"`
}

// Effective never lets an unlimited person setting bypass the server ceiling.
func Effective(person, ceiling *int) *int {
	if person == nil {
		if ceiling == nil {
			return nil
		}
		v := *ceiling
		return &v
	}
	v := *person
	if ceiling != nil && *ceiling < v {
		v = *ceiling
	}
	return &v
}
func ReadTx(ctx context.Context, tx *sql.Tx, account string) (out Facts, err error) {
	var value, ceiling sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(s.mode,'inherit'),s.stream_limit,p.per_account_cap FROM accounts a LEFT JOIN account_stream_limits s ON s.account_id=a.id JOIN playback_owner_policy p ON p.singleton=1 WHERE a.id=?`, account).Scan(&out.Mode, &value, &ceiling)
	if err != nil {
		return out, err
	}
	if value.Valid {
		v := int(value.Int64)
		out.Limit = &v
	}
	if ceiling.Valid {
		v := int(ceiling.Int64)
		out.ServerCeiling = &v
	}
	out.EffectiveLimit = Effective(out.Limit, out.ServerCeiling)
	return out, nil
}
func WriteTx(ctx context.Context, tx *sql.Tx, account string, mode Mode, value int) error {
	if !Valid(mode, value) {
		return errors.New("invalid account stream setting")
	}
	var v any
	if mode == Limit {
		v = value
	}
	_, e := tx.ExecContext(ctx, `INSERT INTO account_stream_limits(account_id,mode,stream_limit) VALUES(?,?,?) ON CONFLICT(account_id) DO UPDATE SET mode=excluded.mode,stream_limit=excluded.stream_limit`, account, mode, v)
	return e
}

// CountTx counts across profiles/devices. V1 leases count once; their backing
// media row and private presentations sharing another live slot do not add one.
func CountTx(ctx context.Context, tx *sql.Tx, authority, account string, scoped bool, now time.Time, replacingV1, replacingMedia string) (int64, error) {
	stamp := now.UTC().Format(time.RFC3339)
	var count int64
	e := tx.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM playback_v1_sessions v WHERE v.ended_ms=0 AND v.lease_expires_ms>?1 AND v.id<>?2 AND (?3='' OR v.media_session_id<>?3) AND (?4=0 OR (v.authority=?5 AND v.account_id=?6)))
 + (SELECT count(*) FROM playback_sessions ps JOIN authorization_access auth ON auth.hash=ps.session_hash WHERE ps.state NOT IN('stopped','ended','failed') AND ps.expires_at>?7 AND auth.expires_at>?7 AND auth.revoked=0 AND ps.id<>?3
 AND (?4=0 OR (auth.authority=?5 AND ps.account_id=?6))
 AND NOT EXISTS(SELECT 1 FROM playback_v1_sessions v WHERE v.media_session_id=ps.id AND v.ended_ms=0 AND v.lease_expires_ms>?1)
 AND NOT EXISTS(SELECT 1 FROM playback_private_presentations pp JOIN playback_sessions owner ON owner.id=pp.owner_session_id WHERE pp.session_id=ps.id AND owner.state NOT IN('stopped','ended','failed') AND owner.expires_at>?7))`, now.UnixMilli(), replacingV1, replacingMedia, scoped, authority, account, stamp).Scan(&count)
	return count, e
}
