package hosted

import (
	"context"
	"database/sql"
	"errors"
	"math/rand/v2"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/persistence"
)

// These are control-plane deadlines, not authorization: nothing a member can do here depends on
// reaching Hosted. NextRenewal is unused since the membership policy went away (it stays in
// networking_control_schedule's shape).
type controlSchedule struct {
	NextHeartbeat, NextRenewal, NextTry time.Time
	Attempts                            int
}

// NetworkChanged coalesces startup, interface and resume observations. It never does network
// I/O on the listener, certificate worker or UI request goroutine.
//
// A network change is a reason to look again locally; it is not a reason to call Hosted. It
// used to force a check-in, so a LAN address moving, a VPN adapter appearing or a gateway
// being renumbered each cost a Hosted request that carried nothing Hosted did not already
// know. What Hosted cares about — the published routes — is the remote manager's business, and
// that publishes only when its route digest actually moves. So this wakes the loop to
// re-evaluate what is due, and the loop calls Hosted only if something is.
//
// The signal coalesces: the channel holds one, so a burst of changes is one look.
func (s *Service) NetworkChanged() { s.wakeControl() }

func (s *Service) currentSchedule(ctx context.Context, v networking.Intent) (controlSchedule, error) {
	var q controlSchedule
	var heartbeat, renewal, retry int64
	e := s.db.QueryRowContext(ctx, `SELECT heartbeat_at,renewal_at,retry_at,attempts
 FROM networking_control_schedule WHERE operation_id=? AND claim_generation=? AND credential_generation=?`, v.OperationID, v.ClaimGeneration, v.CredentialGeneration).Scan(&heartbeat, &renewal, &retry, &q.Attempts)
	if errors.Is(e, sql.ErrNoRows) {
		return q, nil
	}
	if e != nil {
		return q, e
	}
	if heartbeat > 0 {
		q.NextHeartbeat = time.UnixMilli(heartbeat)
	}
	if renewal > 0 {
		q.NextRenewal = time.UnixMilli(renewal)
	}
	if retry > 0 {
		q.NextTry = time.UnixMilli(retry)
	}
	return q, nil
}
func (s *Service) saveCurrentSchedule(ctx context.Context, v networking.Intent, q controlSchedule) error {
	return s.current.store.WithInstalledTransaction(ctx, v, func(ctx context.Context, tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `DELETE FROM networking_control_schedule WHERE operation_id<>?`, v.OperationID)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO networking_control_schedule(operation_id,claim_generation,credential_generation,heartbeat_at,renewal_at,retry_at,attempts) VALUES(?,?,?,?,?,?,?)
   ON CONFLICT(operation_id) DO UPDATE SET claim_generation=excluded.claim_generation,credential_generation=excluded.credential_generation,heartbeat_at=excluded.heartbeat_at,renewal_at=excluded.renewal_at,retry_at=excluded.retry_at,attempts=excluded.attempts`, v.OperationID, v.ClaimGeneration, v.CredentialGeneration, q.NextHeartbeat.UnixMilli(), q.NextRenewal.UnixMilli(), q.NextTry.UnixMilli(), q.Attempts)
		return e
	})
}

// Hosted is contacted when something changed, not on a clock. A check-in is a backstop for a
// missed wake, so its bound is days. There is no membership policy to renew: membership is
// this server's own, and changes to it are pushed (membership_push.go).
const maxCheckInSeconds = 8 * 24 * 60 * 60

// controlSafetyInterval is the longest this loop will sleep when it believes
// nothing is due. It is a backstop against a schedule this process computed
// wrongly or a wake that was never delivered, not a polling interval — which is
// why it is hours. Between two deadlines an idle server makes no query here at
// all, which was the entire cost of the ten-second ticker this replaced.
const controlSafetyInterval = 6 * time.Hour

// controlFloor keeps a step that finds more work from becoming a spin.
const controlFloor = 2 * time.Second

func (s *Service) runCurrent(ctx context.Context) {
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	// Starting is not a reason to call Hosted. The first step runs what is due and nothing
	// else, so a fleet that restarts together (an update, a power cut ending) stays quiet.
	// A network that changed while this server was off is caught by the remote manager.
	next := time.Now()
	for {
		if wait := time.Until(next); wait > 0 {
			timer.Reset(wait)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			case <-s.wake:
				// Something local changed — a restriction was written, a policy revision
				// was pushed, the network moved. Look, but do not make this a heartbeat:
				// the step calls Hosted only for work that is actually due.
				stopTimer(timer)
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return
		}
		step, cancel := context.WithTimeout(ctx, 35*time.Second)
		due := time.Now().Add(controlSafetyInterval)
		// Lifecycle acquisition fences reset/restore and exact installed credentials.
		_ = s.current.runner.Do(step, func(ctx context.Context) error {
			at, e := s.currentControlStep(ctx)
			if !at.IsZero() {
				due = at
			}
			return e
		})
		cancel()
		if floor := time.Now().Add(controlFloor); due.Before(floor) {
			due = floor
		}
		if ceiling := time.Now().Add(controlSafetyInterval); due.After(ceiling) {
			due = ceiling
		}
		next = due
	}
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

// nextControlDeadline is the earliest instant at which this schedule has
// something to do. It is what lets the loop sleep instead of poll.
// OnRouteLabel sets who is told the server's members-only label (the remote
// manager). Hosted returns it with each check-in (A86); a member removal
// changes it, and that removal also wakes the server for its new policy,
// which runs a check-in, so the label is picked up without periodic contact.
func (s *Service) OnRouteLabel(f func(context.Context, string) error) {
	if s != nil && f != nil {
		s.routeLabel.Store(&f)
	}
}

func nextControlDeadline(q controlSchedule, now time.Time) time.Time {
	next := time.Time{}
	consider := func(at time.Time) {
		if at.IsZero() {
			return
		}
		if next.IsZero() || at.Before(next) {
			next = at
		}
	}
	// A persisted retry deadline supersedes the rest: nothing may be attempted
	// before it, so it is the only thing worth waking for.
	if now.Before(q.NextTry) {
		return q.NextTry
	}
	consider(q.NextHeartbeat)
	if next.IsZero() {
		return now
	}
	return next
}

// currentControlStep does one pass and reports when the next one is due, so the
// loop can sleep until then rather than ask the database every ten seconds
// whether anything has changed.
func (s *Service) currentControlStep(ctx context.Context) (time.Time, error) {
	eventDue, eventErr := s.sendServerEvents(ctx)
	due, err := s.ordinaryControlStep(ctx)
	if v, e := s.current.store.InstalledIntent(ctx); e == nil {
		memberDue, memberErr := s.memberStep(ctx, v)
		if !memberDue.IsZero() && (due.IsZero() || memberDue.Before(due)) {
			due = memberDue
		}
		if err == nil {
			err = memberErr
		}
	}
	if networking.NeedsClaimReconciliation(err) || networking.NeedsClaimReconciliation(eventErr) {
		_ = s.requestClaimCleanup(ctx)
	}
	var cleanupDue time.Time
	needed, cleanupErr := s.cleanupNeeded(ctx)
	if cleanupErr == nil && needed {
		cleanupDue, cleanupErr = s.cleanupControlStep(ctx)
	}
	if !cleanupDue.IsZero() && (due.IsZero() || cleanupDue.Before(due)) {
		due = cleanupDue
	}
	if cleanupErr != nil {
		return due, cleanupErr
	}
	// Each queued event carries its own backoff; the loop only needs the earliest.
	if !eventDue.IsZero() && (due.IsZero() || eventDue.Before(due)) {
		due = eventDue
	}
	if eventErr != nil {
		return due, eventErr
	}
	return due, err
}

func (s *Service) ordinaryControlStep(ctx context.Context) (time.Time, error) {
	v, e := s.current.store.InstalledIntent(ctx)
	if e != nil {
		return time.Time{}, e
	}
	schedule, e := s.currentSchedule(ctx, v)
	if e != nil {
		return time.Time{}, e
	}
	now := time.Now().UTC()
	// Wake and topology change cannot bypass a persisted Retry-After deadline.
	if now.Before(schedule.NextTry) {
		return schedule.NextTry, nil
	}
	desired, _ := strconv.ParseInt(persistence.Get(s.db, wakeRevisionKey), 10, 64)
	checked, _ := strconv.ParseInt(persistence.Get(s.db, checkedRevisionKey), 10, 64)
	// A wake (an erasure job, a rotated members-only label) runs the check-in now.
	// There is no other periodic contact with Hosted.
	woken := desired > checked
	if now.Before(schedule.NextHeartbeat) && !woken {
		return nextControlDeadline(schedule, now), nil
	}
	work := func() error {
		digest, e := s.currentMemberDigest(ctx)
		if e != nil {
			return e
		}
		sync, e := s.memberSyncState(ctx)
		if e != nil {
			return e
		}
		var result struct {
			HeartbeatAfterSeconds int         `json:"heartbeatAfterSeconds"`
			PresenceExpiresAt     time.Time   `json:"presenceExpiresAt"`
			RouteLabel            string      `json:"routeLabel"`
			MemberSequence        int64       `json:"memberSequence"`
			MemberDigestMatches   bool        `json:"memberDigestMatches"`
			CleanupPending        bool        `json:"cleanupPending"`
			Departures            []Departure `json:"departures"`
		}
		in := map[string]any{"wakeRevision": desired, "memberSequence": sync.acked, "memberDigest": digest}
		if e := s.current.transport.CallServer(ctx, v, networking.SendHeartbeat, in, &result); e != nil {
			return e
		}
		if result.HeartbeatAfterSeconds < 60 || result.HeartbeatAfterSeconds > maxCheckInSeconds || !result.PresenceExpiresAt.After(now) {
			return networking.ErrInvalid
		}
		// Hosted sets the cadence (weekly when it can wake this server, daily when it cannot);
		// per-server jitter keeps a fleet from checking in together.
		delay := time.Duration(result.HeartbeatAfterSeconds) * time.Second
		delay = delay*9/10 + time.Duration(rand.Int64N(int64(delay/5)))
		schedule.NextHeartbeat = now.Add(delay)
		if f := s.routeLabel.Load(); result.RouteLabel != "" && f != nil {
			_ = (*f)(ctx, result.RouteLabel)
		}
		// Hosted's index disagrees with this server's members (a restore on
		// either side, a lost push): replace it with the full list.
		if !sync.journalPending && (!result.MemberDigestMatches || result.MemberSequence != sync.acked) {
			if e := s.requestFullMemberPush(ctx, v); e != nil {
				return e
			}
		}
		if e := s.applyDepartures(ctx, v, result.Departures); e != nil {
			return e
		}
		if result.CleanupPending {
			if e := s.requestClaimCleanup(ctx); e != nil {
				return e
			}
		}
		if woken {
			if _, e := dbwork.ExecWrite(ctx, s.db, dbwork.ClassInteractive, `INSERT INTO configuration VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=CAST(max(CAST(value AS INTEGER),CAST(excluded.value AS INTEGER)) AS TEXT)`, checkedRevisionKey, strconv.FormatInt(desired, 10)); e != nil {
				return e
			}
		}
		return nil
	}
	e = work()
	if e != nil {
		schedule.Attempts = min(schedule.Attempts+1, 8)
		schedule.NextTry = networking.RetryAt(now, 5*time.Second, schedule.Attempts, 8, networking.ControlRetryAt(e))
	} else {
		schedule.Attempts = 0
		schedule.NextTry = time.Time{}
	}
	saveErr := s.saveCurrentSchedule(ctx, v, schedule)
	// An outbox that still has entries is work this loop knows about, so it says
	// so rather than waiting for a deadline that has nothing to do with it.
	due := nextControlDeadline(schedule, time.Now().UTC())
	if e != nil {
		return due, e
	}
	return due, saveErr
}
