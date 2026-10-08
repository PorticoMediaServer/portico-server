package dvr

import (
	"context"
	"database/sql"
	"encoding/json"
	"portico.local/server/internal/livechannels"
	"strings"
	"time"
)

// Detail explains competition without disclosing another profile's recording
// identity, title, or rule. Demand/capacity still include all reservations.
type Overlap struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Priority int    `json:"priority"`
	State    string `json:"state"`
}
type Detail struct {
	Recording   Recording             `json:"recording"`
	Capacity    livechannels.Capacity `json:"capacity"`
	Overlaps    []Overlap             `json:"overlaps"`
	NextOverlap string                `json:"nextOverlap"`
}

func (s *Store) Detail(ctx context.Context, a livechannels.Authority, o livechannels.Owner, id, after string) (Detail, error) {
	out := Detail{Overlaps: []Overlap{}}
	if !canonicalID.MatchString(id) || after != "" && !canonicalID.MatchString(after) {
		return out, ErrInvalid
	}
	e := s.snapshot(ctx, a, o, func(tx *sql.Tx, _ func(string, string) bool) error {
		r, e := loadTx(ctx, tx, o, id)
		if e != nil {
			return e
		}
		out.Recording = r
		if r.State == "pending-delete" {
			var reason string
			if err := tx.QueryRowContext(ctx, `SELECT reason FROM dvr_delete_work WHERE recording_id=?`, r.ID).Scan(&reason); err != nil && err != sql.ErrNoRows {
				return err
			}
			switch reason {
			case "physical-reader-active", "artifact-remove-unavailable", "active-reader-grace":
				out.Recording.Reason = reason
			}
		}
		cap, e := livechannels.CapacityTx(ctx, tx, r.Occurrence.SourceID)
		if e == nil {
			out.Capacity = cap
		}
		if !mutable(r.State) && !capturing(r.State) {
			return nil
		}
		if e != nil {
			return e
		}
		out.Recording.Conflicts, e = conflictsTx(ctx, tx, r)
		if e != nil {
			return e
		}
		if len(out.Recording.Conflicts) > 0 && r.State == "scheduled" {
			out.Recording.State = "conflicted"
			out.Recording.Reason = "tuner-conflict"
		}
		start, _ := time.Parse(time.RFC3339Nano, r.Start)
		end, _ := time.Parse(time.RFC3339Nano, r.End)
		rows, e := tx.QueryContext(ctx, `SELECT id,programme_json,start_ms,end_ms,priority,state FROM dvr_recordings WHERE owner_key=? AND source_id=? AND id<>? AND id>? AND start_ms<? AND end_ms>? AND state IN('scheduled','conflicted','preparing','recording','finalizing') ORDER BY id LIMIT 21`, o.Key(), r.Occurrence.SourceID, id, after, end.UnixMilli(), start.UnixMilli())
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v Overlap
			var programme string
			var start, end int64
			if e = rows.Scan(&v.ID, &programme, &start, &end, &v.Priority, &v.State); e != nil {
				return e
			}
			p, e := decodeProgramme(programme)
			if e != nil {
				return e
			}
			v.Title = p.Title
			v.Start = time.UnixMilli(start).UTC().Format(time.RFC3339Nano)
			v.End = time.UnixMilli(end).UTC().Format(time.RFC3339Nano)
			out.Overlaps = append(out.Overlaps, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Overlaps) > 20 {
			out.Overlaps = out.Overlaps[:20]
			out.NextOverlap = out.Overlaps[19].ID
		}
		return nil
	})
	return out, e
}

type ChannelChoice struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Number string `json:"number"`
}
type ChannelChoices struct {
	Channels   []ChannelChoice `json:"channels"`
	NextCursor string          `json:"nextCursor"`
	Generation string          `json:"generation"`
}

// The picker reads the current authoritative source generation. No provider
// locators, credentials, or inaccessible sources are part of this projection.
func (s *Store) ChannelChoices(ctx context.Context, a livechannels.Authority, o livechannels.Owner, source, search, cursor string) (ChannelChoices, error) {
	out := ChannelChoices{Channels: []ChannelChoice{}}
	if !hexID.MatchString(source) || len(search) > 120 || strings.ContainsAny(search, "\x00\r\n") {
		return out, ErrInvalid
	}
	e := s.snapshot(ctx, a, o, func(tx *sql.Tx, allowed func(string, string) bool) error {
		if !allowed(source, "") {
			return ErrDenied
		}
		var state string
		if e := tx.QueryRowContext(ctx, `SELECT active_generation,state FROM live_sources WHERE id=?`, source).Scan(&out.Generation, &state); e != nil {
			return ErrDenied
		}
		after := ""
		if cursor != "" {
			parts := strings.Split(cursor, ".")
			if len(parts) != 2 || !hexID.MatchString(parts[0]) || !canonicalID.MatchString(parts[1]) {
				return ErrInvalid
			}
			if parts[0] != out.Generation {
				return ErrConflict
			}
			after = parts[1]
		}
		rows, e := tx.QueryContext(ctx, `SELECT channel_id,name,number FROM live_channel_versions WHERE generation_id=? AND channel_id>? AND (?='' OR instr(lower(name),lower(?))>0 OR instr(lower(number),lower(?))>0) ORDER BY channel_id LIMIT 31`, out.Generation, after, search, search, search)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v ChannelChoice
			if e = rows.Scan(&v.ID, &v.Name, &v.Number); e != nil {
				return e
			}
			if allowed(source, v.ID) {
				out.Channels = append(out.Channels, v)
			}
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Channels) > 30 {
			out.Channels = out.Channels[:30]
			out.NextCursor = out.Generation + "." + out.Channels[29].ID
		}
		return nil
	})
	return out, e
}

// Annotate only programmes already authorized by the guide, owned by this exact
// durable viewer. The guide's generation remains the occurrence authority.
func (s *Store) AnnotateGuide(ctx context.Context, a livechannels.Authority, o livechannels.Owner, g *livechannels.Guide) error {
	if g == nil {
		return ErrInvalid
	}
	return s.snapshot(ctx, a, o, func(tx *sql.Tx, allowed func(string, string) bool) error {
		for i := range g.Channels {
			ch := &g.Channels[i]
			if ch.Provenance != livechannels.LiveSource || !allowed(ch.SourceID, ch.ID) {
				continue
			}
			rows, e := tx.QueryContext(ctx, `SELECT programme_id,id,state FROM dvr_recordings WHERE owner_key=? AND source_id=? AND channel_id=? AND end_ms>? AND start_ms<? AND state NOT IN('cancelled','deleted')`, o.Key(), ch.SourceID, ch.ID, mustTime(g.Start).UnixMilli(), mustTime(g.End).UnixMilli())
			if e != nil {
				return e
			}
			type status struct{ id, state string }
			values := map[string]status{}
			for rows.Next() {
				var p string
				var v status
				if e = rows.Scan(&p, &v.id, &v.state); e != nil {
					rows.Close()
					return e
				}
				values[p] = v
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			for j := range ch.Programmes {
				p := &ch.Programmes[j]
				if v, ok := values[p.ID]; ok {
					p.RecordingID = v.id
					p.RecordingState = v.state
				}
			}
		}
		return nil
	})
}
func mustTime(s string) time.Time { v, _ := time.Parse(time.RFC3339Nano, s); return v }

func decodeProgramme(raw string) (livechannels.Programme, error) {
	var p livechannels.Programme
	if json.Unmarshal([]byte(raw), &p) != nil {
		return p, ErrUnavailable
	}
	return p, nil
}
