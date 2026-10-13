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
	all, e := livechannels.ReservationsTx(ctx, tx, r.Occurrence.SourceID, start, end)
	if e != nil {
		return out, e
	}
	selected := false
	for i := range all {
		if all[i].ID == r.ID {
			all[i].ChannelID = r.Occurrence.ChannelID
			selected = true
		}
	}
	if !selected {
		all = append(all, livechannels.Reservation{ID: r.ID, ChannelID: r.Occurrence.ChannelID, Start: start, End: end, Priority: r.Options.Priority})
	}
	return livechannels.Conflicts(all, r.ID, cap), nil
}
