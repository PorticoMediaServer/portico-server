package livechannels

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"
)

// Owner excludes session/policy revisions: those fence permission, not ownership.
type Owner struct {
	Authority string `json:"authority"`
	AccountID string `json:"accountId"`
	ProfileID string `json:"profileId"`
}

func (o Owner) Valid() bool {
	return (o.Authority == "local" || o.Authority == "hosted") && validText(o.AccountID, 256) && validText(o.ProfileID, 256)
}
func (o Owner) Key() string { return id(o.Authority, o.AccountID, o.ProfileID) }

const AllocationTTL = 20 * time.Second

var ErrCapacity = errors.New("The source has reached its confirmed tuner limit.")
var ErrPreemptPending = errors.New("A live tuner is being released for a protected recording.")
var ErrReservation = errors.New("A protected recording requires this tuner now.")
var ErrLease = errors.New("This source allocation is no longer valid.")

func CapacityTx(ctx context.Context, tx *sql.Tx, sid string) (Capacity, error) {
	c := Capacity{PlanningEstimate: 1, Mode: "defaulted"}
	var count int
	e := tx.QueryRowContext(ctx, `SELECT tuner_count FROM live_sources WHERE id=? AND state='active'`, sid).Scan(&count)
	if e != nil {
		return c, ErrConflict
	}
	if count > 0 {
		c.Known = true
		c.Effective = count
		c.PlanningEstimate = count
		c.Mode = "configured"
	}
	var physical, limit int
	e = tx.QueryRowContext(ctx, `SELECT physical_count,owner_limit FROM live_source_settings WHERE source_id=?`, sid).Scan(&physical, &limit)
	if e == nil {
		count = effectiveCapacity(physical, limit, true)
		c.Known = count > 0
		c.Effective = count
		if count > 0 {
			c.PlanningEstimate = count
		}
		if physical > 0 && limit == 0 {
			c.Mode = "discovered"
		}
	} else if !errors.Is(e, sql.ErrNoRows) {
		return c, ErrUnavailable
	}
	return c, nil
}

type Allocation struct {
	ID         string
	SourceID   string
	ChannelID  string
	ResourceID string
	Kind       string
	Owner      Owner
	Token      string
	Generation int64
	Expires    time.Time
	PreemptAt  time.Time
}

func reservationsTx(ctx context.Context, tx *sql.Tx, sid string, start, end time.Time) ([]Reservation, error) {
	rows, e := tx.QueryContext(ctx, `SELECT id,start_ms,end_ms,priority FROM live_reservations WHERE source_id=? AND enabled=1 AND start_ms<? AND end_ms>? ORDER BY start_ms,id`, sid, end.UnixMilli(), start.UnixMilli())
	if e != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Reservation{}
	for rows.Next() {
		var r Reservation
		var a, b int64
		if rows.Scan(&r.ID, &a, &b, &r.Priority) != nil {
			return nil, ErrUnavailable
		}
		r.Start = time.UnixMilli(a)
		r.End = time.UnixMilli(b)
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

// AcquireTx must be called inside the caller's authorized short transaction.
// The caller commits preemption even when ErrPreemptPending is returned, then
// retries only after old source leases have actually released/expired.
func AcquireTx(ctx context.Context, tx *sql.Tx, want Allocation, now time.Time) (Allocation, error) {
	if !want.Owner.Valid() || (want.Kind != "live" && want.Kind != "recording") || !validText(want.ResourceID, 256) {
		return want, ErrInvalid
	}
	cap, e := CapacityTx(ctx, tx, want.SourceID)
	if e != nil {
		return want, e
	}
	// Expired capacity stays quarantined until PhysicalLocks.ReconcileExpired
	// proves all inherited decoder/input ownership retired, plus bounded grace.
	var existing Allocation
	var expiry int64
	var state string
	e = tx.QueryRowContext(ctx, `SELECT id,token,generation,state,lease_until_ms FROM live_allocations WHERE kind=? AND resource_id=?`, want.Kind, want.ResourceID).Scan(&existing.ID, &existing.Token, &existing.Generation, &state, &expiry)
	if e == nil && state != "released" {
		return want, ErrLease
	}
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return want, ErrUnavailable
	}
	var active int
	if tx.QueryRowContext(ctx, `SELECT count(*) FROM live_allocations WHERE source_id=? AND state!='released'`, want.SourceID).Scan(&active) != nil {
		return want, ErrUnavailable
	}
	reservations, e := reservationsTx(ctx, tx, want.SourceID, now, now.Add(7*24*time.Hour))
	if e != nil {
		return want, e
	}
	if cap.Known {
		if want.Kind == "recording" {
			activeReservations := []Reservation{}
			for _, r := range reservations {
				if !now.Before(r.Start) && now.Before(r.End) {
					activeReservations = append(activeReservations, r)
				}
			}
			sort.Slice(activeReservations, func(i, j int) bool { return reservationBefore(activeReservations[i], activeReservations[j]) })
			for n, r := range activeReservations {
				if r.ID == want.ResourceID && n >= cap.Effective {
					return want, ErrCapacity
				}
			}
		}
		if active >= cap.Effective {
			if want.Kind == "recording" {
				// A newly submitted recording cannot silently kill an active viewer.
				// Future reservations establish their boundary earlier; otherwise a
				// typed countdown gets ten seconds and the capture reports lateness.
				boundary := now.Add(RetirementGrace).UnixMilli()
				var victim string
				err := tx.QueryRowContext(ctx, `SELECT id FROM live_allocations WHERE source_id=? AND kind='live' AND state='active' ORDER BY created_ms DESC,id DESC LIMIT 1`, want.SourceID).Scan(&victim)
				if err == nil {
					_, err = tx.ExecContext(ctx, `UPDATE live_allocations SET preempt_at_ms=CASE WHEN preempt_at_ms=0 OR preempt_at_ms>? THEN ? ELSE preempt_at_ms END WHERE id=?`, boundary, boundary, victim)
					if err != nil {
						return want, ErrUnavailable
					}
					return want, ErrPreemptPending
				}
				if !errors.Is(err, sql.ErrNoRows) {
					return want, ErrUnavailable
				}
			}
			return want, ErrCapacity
		}
		if want.Kind == "live" {
			// Active recording leases already consume capacity; count only live leases
			// plus protected reservations, so a recording is never counted twice.
			var liveCount int
			if tx.QueryRowContext(ctx, `SELECT count(*) FROM live_allocations WHERE source_id=? AND kind='live' AND state!='released'`, want.SourceID).Scan(&liveCount) != nil {
				return want, ErrUnavailable
			}
			at, blocked := protectedBoundary(reservations, now, cap.Effective-liveCount-1)
			if blocked {
				want.PreemptAt = at
				if !at.After(now.Add(AllocationTTL)) {
					return want, ErrReservation
				}
			}
		}
	}
	if existing.ID == "" {
		want.ID, e = nonceID()
		want.Generation = 1
	} else {
		want.ID = existing.ID
		want.Generation = existing.Generation + 1
	}
	if e != nil {
		return want, e
	}
	want.Token, e = nonceID()
	if e != nil {
		return want, e
	}
	want.Expires = now.Add(AllocationTTL)
	if !want.PreemptAt.IsZero() && want.PreemptAt.Before(want.Expires) {
		want.Expires = want.PreemptAt
	}
	preempt := int64(0)
	if !want.PreemptAt.IsZero() {
		preempt = want.PreemptAt.UnixMilli()
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO live_allocations VALUES(?,?,?,?,?,?,?,?,?,?,'active',?,?,?) ON CONFLICT(kind,resource_id) DO UPDATE SET source_id=excluded.source_id,channel_id=excluded.channel_id,authority=excluded.authority,account_id=excluded.account_id,profile_id=excluded.profile_id,token=excluded.token,generation=excluded.generation,state='active',lease_until_ms=excluded.lease_until_ms,created_ms=excluded.created_ms,preempt_at_ms=excluded.preempt_at_ms`, want.ID, want.SourceID, want.ChannelID, want.ResourceID, want.Kind, want.Owner.Authority, want.Owner.AccountID, want.Owner.ProfileID, want.Token, want.Generation, want.Expires.UnixMilli(), now.UnixMilli(), preempt)
	if e != nil {
		return want, ErrUnavailable
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO live_source_dependencies VALUES(?,'allocation',?) ON CONFLICT DO NOTHING`, want.SourceID, want.ID)
	if e != nil {
		return want, ErrUnavailable
	}
	return want, nil
}
func RenewAllocationTx(ctx context.Context, tx *sql.Tx, a Allocation, now time.Time) (time.Time, error) {
	var deadline int64
	if tx.QueryRowContext(ctx, `SELECT preempt_at_ms FROM live_allocations WHERE id=? AND token=? AND generation=? AND state='active' AND lease_until_ms>?`, a.ID, a.Token, a.Generation, now.UnixMilli()).Scan(&deadline) != nil {
		return time.Time{}, ErrLease
	}
	expiry := now.Add(AllocationTTL)
	if deadline > 0 {
		if deadline <= now.UnixMilli() {
			return time.Time{}, ErrLease
		}
		if t := time.UnixMilli(deadline); t.Before(expiry) {
			expiry = t
		}
	}
	res, e := tx.ExecContext(ctx, `UPDATE live_allocations SET lease_until_ms=? WHERE id=? AND token=? AND generation=? AND state='active' AND lease_until_ms>?`, expiry.UnixMilli(), a.ID, a.Token, a.Generation, now.UnixMilli())
	if e != nil {
		return time.Time{}, ErrUnavailable
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return time.Time{}, ErrLease
	}
	return expiry, nil
}

// protectedBoundary sweeps starts/ends in O(n log n), with half-open intervals.
// available is the remaining capacity after existing live viewers and this one.
func protectedBoundary(all []Reservation, now time.Time, available int) (time.Time, bool) {
	type edge struct {
		at    time.Time
		delta int
	}
	edges := []edge{}
	demand := 0
	for _, r := range all {
		if !r.End.After(now) {
			continue
		}
		if !r.Start.After(now) {
			demand++
		} else {
			edges = append(edges, edge{r.Start, 1})
		}
		edges = append(edges, edge{r.End, -1})
	}
	if demand > available {
		return now, true
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].at.Before(edges[j].at) })
	for i := 0; i < len(edges); {
		at := edges[i].at
		for i < len(edges) && edges[i].at.Equal(at) {
			demand += edges[i].delta
			i++
		}
		if demand > available {
			return at, true
		}
	}
	return time.Time{}, false
}
func ReleaseAllocationTx(ctx context.Context, tx *sql.Tx, a Allocation) error {
	res, e := tx.ExecContext(ctx, `UPDATE live_allocations SET state='released',lease_until_ms=0 WHERE id=? AND token=? AND generation=?`, a.ID, a.Token, a.Generation)
	if e != nil {
		return ErrUnavailable
	}
	n, _ := res.RowsAffected()
	if n == 1 {
		_, e = tx.ExecContext(ctx, `DELETE FROM live_source_dependencies WHERE source_id=? AND kind='allocation' AND id=?`, a.SourceID, a.ID)
	}
	if e != nil {
		return ErrUnavailable
	}
	return nil
}
