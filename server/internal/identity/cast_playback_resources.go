package identity

import (
	"context"
	"database/sql"
	"time"
)

// CastPlaybackResourceTx permits only the paired screen's adopted current
// occurrence. Same-profile ownership of some other device's queue is not enough.
// It grants no browsing right and must run beside the caller's content checks.
// Queue completion may follow a genuine end report, while its original lease
// remains live. Stop, replacement, error and an expired lease do not qualify.
func (s *Service) CastPlaybackResourceTx(ctx context.Context, tx *sql.Tx, p Principal, item, queue string) (string, error) {
	if tx == nil || p.Grant != GrantCastTarget || (item == "") == (queue == "") {
		return "", ErrForbidden
	}
	family, err := s.SessionFamilyTx(ctx, tx, p)
	if err != nil {
		return "", err
	}
	if p.DeviceID != "" && p.DeviceID != family.DeviceID {
		return "", ErrUnauthorized
	}
	var current string
	err = tx.QueryRowContext(ctx, `SELECT pid(e.public_id) FROM social_cast_devices c
 JOIN playback_v1_sessions s ON s.device_id=? AND s.authority=c.authority AND s.account_id=c.account_id AND s.profile_id=c.profile_id
 JOIN catalog_entities e ON e.id=s.item_id
 WHERE c.session_family_id=? AND c.authority=? AND c.account_id=? AND c.profile_id=? AND c.account_epoch=? AND c.state='active' AND c.expires_at_ms>?
 AND s.lease_expires_ms>?
 AND ((?<>'' AND e.public_id=pid_blob(?) AND s.ended_ms=0 AND s.state IN('preparing','playing','paused'))
 OR (?<>'' AND s.queue_id=? AND (s.ended_ms=0 AND s.state IN('preparing','playing','paused') OR s.ended_ms>0 AND s.end_reason IN('ended','completed'))
 AND EXISTS(SELECT 1 FROM queues_v1 q WHERE q.id=s.queue_id AND q.session_id=s.id AND q.owner_kind='device' AND substr(q.owner_id,1,1) NOT IN('!','~') AND q.authority=s.authority AND q.account_id=s.account_id AND q.profile_id=s.profile_id)))
 LIMIT 1`, family.DeviceID, family.ID, p.Authority, p.AccountID, p.ProfileID, p.Epoch, time.Now().UnixMilli(), time.Now().UnixMilli(), item, item, queue, queue).Scan(&current)
	if err == sql.ErrNoRows {
		return "", ErrForbidden
	}
	if err != nil {
		return "", err
	}
	return current, ctx.Err()
}
