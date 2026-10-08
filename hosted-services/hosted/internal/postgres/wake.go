package postgres

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/hosted/internal/wake"
	"time"
)

func (s *Store) Claim(ctx context.Context, lease string, now time.Time) (wake.Work, bool, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return wake.Work{}, false, e
	}
	defer tx.Rollback()
	var w wake.Work
	e = tx.QueryRowContext(ctx, `SELECT w.server_id,w.kind,w.desired_revision,w.attempts,s.base_url FROM wake_state w JOIN servers s ON s.id=w.server_id WHERE s.revoked_at IS NULL AND s.endpoint_verified AND s.base_url IS NOT NULL AND NOT w.wake_unreachable AND w.desired_revision>w.acked_revision AND w.next_attempt_at<=$1 AND (w.lease_until IS NULL OR w.lease_until<$1) ORDER BY w.next_attempt_at,w.server_id LIMIT 1 FOR UPDATE OF w SKIP LOCKED`, now).Scan(&w.ServerID, &w.Kind, &w.Revision, &w.Attempts, &w.BaseURL)
	if errors.Is(e, sql.ErrNoRows) {
		return w, false, nil
	}
	if e != nil {
		return w, false, e
	}
	w.LeaseToken = lease
	if _, e = tx.ExecContext(ctx, `UPDATE wake_state SET first_attempt_at=COALESCE(first_attempt_at,$5),lease_token=$3,lease_until=$4,attempts=attempts+1 WHERE server_id=$1 AND kind=$2`, w.ServerID, w.Kind, lease, now.Add(15*time.Second), now); e != nil {
		return w, false, e
	}
	return w, true, tx.Commit()
}

// NextDue is the earliest instant at which any deliverable wake may be attempted, so the
// worker can sleep until then rather than ask every second. It reads the wake_due index.
func (s *Store) NextDue(ctx context.Context) (time.Time, bool, error) {
	var at sql.NullTime
	e := s.DB.QueryRowContext(ctx, `SELECT min(w.next_attempt_at) FROM wake_state w JOIN servers s ON s.id=w.server_id
 WHERE s.revoked_at IS NULL AND s.endpoint_verified AND s.base_url IS NOT NULL AND NOT w.wake_unreachable AND w.desired_revision>w.acked_revision`).Scan(&at)
	if e != nil {
		return time.Time{}, false, e
	}
	return at.Time, at.Valid, nil
}

func (s *Store) Finish(ctx context.Context, w wake.Work, status int, next time.Time) error {
	// Never rewrite desired/acked: a newer mutation or an early ack can arrive while HTTP is in flight.
	_, e := s.DB.ExecContext(ctx, `UPDATE wake_state SET wake_unreachable=(desired_revision=$3 AND first_attempt_at<=clock_timestamp()-interval '24 hours'),lease_token=NULL,lease_until=NULL,last_status=$4,next_attempt_at=CASE WHEN desired_revision>$3 THEN LEAST(next_attempt_at,now()) ELSE $5 END WHERE server_id=$1 AND kind=$2 AND lease_token=$6`, w.ServerID, w.Kind, w.Revision, status, next, w.LeaseToken)
	return e
}
