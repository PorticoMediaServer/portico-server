package dvr

import (
	"context"
	"database/sql"
	"portico.local/server/internal/livechannels"
	"time"
)

// The target loses exactly where at least capacity higher-ranked reservations
// overlap it. SQL streams grouped interval edges; memory is independent of the
// number of competing programmes. Explanations expose counts, never foreign IDs.
func conflictsTx(ctx context.Context, tx *sql.Tx, r Recording) ([]livechannels.LosingInterval, error) {
	out := []livechannels.LosingInterval{}
	cap, e := livechannels.CapacityTx(ctx, tx, r.Occurrence.SourceID)
	if e != nil {
		return out, e
	}
	if !cap.Known {
		return out, nil
	}
	start, e := time.Parse(time.RFC3339Nano, r.Start)
	if e != nil {
		return out, ErrUnavailable
	}
	end, e := time.Parse(time.RFC3339Nano, r.End)
	if e != nil {
		return out, ErrUnavailable
	}
	rows, e := tx.QueryContext(ctx, `WITH competitors AS (
 SELECT max(start_ms,?) a,min(end_ms,?) b,
 CASE WHEN priority>? OR (priority=? AND (start_ms<? OR (start_ms=? AND id<?))) THEN 1 ELSE 0 END higher
 FROM live_reservations WHERE source_id=? AND enabled=1 AND id<>? AND start_ms<? AND end_ms>?
 ), edges AS (SELECT a at,1 demand,higher FROM competitors UNION ALL SELECT b at,-1 demand,-higher FROM competitors)
 SELECT at,SUM(demand),SUM(higher) FROM edges GROUP BY at ORDER BY at`, start.UnixMilli(), end.UnixMilli(), r.Options.Priority, r.Options.Priority, start.UnixMilli(), start.UnixMilli(), r.ID, r.Occurrence.SourceID, r.ID, end.UnixMilli(), start.UnixMilli())
	if e != nil {
		return out, e
	}
	defer rows.Close()
	demand, higher := 1, 0
	last := start.UnixMilli()
	for rows.Next() {
		var at int64
		var d, h int
		if e = rows.Scan(&at, &d, &h); e != nil {
			return out, e
		}
		if at > last && higher >= cap.Effective {
			a, b := time.UnixMilli(last).UTC().Format(time.RFC3339Nano), time.UnixMilli(at).UTC().Format(time.RFC3339Nano)
			if len(out) > 0 && out[len(out)-1].End == a && out[len(out)-1].Demand == demand {
				out[len(out)-1].End = b
			} else {
				out = append(out, livechannels.LosingInterval{Start: a, End: b, Demand: demand, Capacity: cap.Effective})
			}
		}
		demand += d
		higher += h
		last = at
	}
	return out, rows.Err()
}
