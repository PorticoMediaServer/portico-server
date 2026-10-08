package dvr

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"portico.local/server/internal/livechannels"
)

type Query struct {
	State  string
	Cursor string
	Limit  int
}
type continuation struct {
	Owner, Fence, State, AfterID string
	AfterMS, Revision, Expiry    int64
	Limit                        int
}

func (s *Store) cursor(v continuation) string {
	b, _ := json.Marshal(v)
	h := hmac.New(sha256.New, s.live.ContinuationKey("dvr-pages"))
	h.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (s *Store) decodeCursor(text string) (continuation, error) {
	var v continuation
	if len(text) > 4096 {
		return v, ErrInvalid
	}
	parts := strings.Split(text, ".")
	if len(parts) != 2 {
		return v, ErrInvalid
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return v, ErrInvalid
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return v, ErrInvalid
	}
	h := hmac.New(sha256.New, s.live.ContinuationKey("dvr-pages"))
	h.Write(b)
	if !hmac.Equal(sig, h.Sum(nil)) || json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) List(ctx context.Context, a livechannels.Authority, o livechannels.Owner, q Query) (Page, error) {
	out := Page{DeletionAvailable: s.driver != nil && s.driver.RetirementAvailable(), Recordings: []Recording{}, Rules: []Rule{}, CaptureAvailable: s.captureAvailable}
	if !s.captureAvailable {
		out.CaptureUnavailableReason = "capture-unavailable"
	}
	if q.Limit < 1 || q.Limit > 50 || (q.State != "all" && q.State != "upcoming" && q.State != "recorded" && q.State != "history" && q.State != "rules") {
		return out, ErrInvalid
	}
	e := s.snapshot(ctx, a, o, func(tx *sql.Tx, _ func(string, string) bool) error {
		var usageErr error
		out.Usage, usageErr = usageTx(ctx, tx, o)
		if usageErr != nil {
			return usageErr
		}
		fence, _, e := a(ctx, tx, false)
		if e != nil {
			return e
		}
		e = tx.QueryRowContext(ctx, `SELECT revision FROM dvr_owner_revisions WHERE owner_key=?`, o.Key()).Scan(&out.Revision)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		c := continuation{Owner: o.Key(), Fence: fence, State: q.State, Limit: q.Limit, Revision: out.Revision, Expiry: s.now().Add(15 * time.Minute).UnixMilli(), AfterMS: 1 << 62}
		if q.Cursor != "" {
			old, e := s.decodeCursor(q.Cursor)
			if e != nil {
				return e
			}
			if old.Owner != c.Owner || old.Fence != c.Fence || old.State != c.State || old.Limit != c.Limit || old.Revision != c.Revision || old.Expiry <= s.now().UnixMilli() {
				return livechannels.ErrCursor
			}
			c = old
		}
		if q.State == "rules" {
			rows, e := tx.QueryContext(ctx, `SELECT `+ruleColumns+` FROM dvr_rules r LEFT JOIN dvr_rule_work w ON w.rule_id=r.id WHERE r.owner_key=? AND r.deleted=0 AND r.id>? ORDER BY r.id LIMIT ?`, o.Key(), c.AfterID, q.Limit+1)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				r, e := scanRule(rows)
				if e != nil {
					return e
				}
				out.Rules = append(out.Rules, r)
			}
			if e = rows.Err(); e != nil {
				return e
			}
			if len(out.Rules) > q.Limit {
				out.Rules = out.Rules[:q.Limit]
				c.AfterID = out.Rules[len(out.Rules)-1].ID
				out.NextCursor = s.cursor(c)
			}
			return nil
		}
		where := `state!='deleted'`
		switch q.State {
		case "upcoming":
			where = `state IN('scheduled','conflicted','waiting-source','waiting-guide','preparing','recording','finalizing')`
		case "recorded":
			where = `state IN('completed','incomplete-playable')`
		case "history":
			where = `state IN('failed','cancelled','pending-delete')`
		}
		rows, e := tx.QueryContext(ctx, `SELECT `+recordingColumns+`,created_ms FROM dvr_recordings WHERE owner_key=? AND `+where+` AND (created_ms<? OR (created_ms=? AND id<?)) ORDER BY created_ms DESC,id DESC LIMIT ?`, o.Key(), c.AfterMS, c.AfterMS, c.AfterID, q.Limit+1)
		if e != nil {
			return e
		}
		// Include the sort key without inventing client-computed wall-time ordering.
		created := []int64{}
		for rows.Next() {
			var r Recording
			var p, options string
			var start, end, ca, cb, at int64
			e = rows.Scan(&r.ID, &r.Revision, &r.Occurrence.SourceID, &r.Occurrence.ChannelID, &r.Occurrence.Generation, &r.Occurrence.ProgrammeID, &p, &r.RuleID, &options, &start, &end, &r.State, &r.Reason, &r.Keep, &r.ItemID, &ca, &cb, &r.Bytes, &at)
			if e != nil {
				rows.Close()
				return e
			}
			if json.Unmarshal([]byte(p), &r.Programme) != nil || json.Unmarshal([]byte(options), &r.Options) != nil {
				rows.Close()
				return ErrUnavailable
			}
			setSeriesGrouping(&r)
			r.Start = time.UnixMilli(start).UTC().Format(time.RFC3339Nano)
			r.End = time.UnixMilli(end).UTC().Format(time.RFC3339Nano)
			if ca > 0 {
				r.CoverageStart = time.UnixMilli(ca).UTC().Format(time.RFC3339Nano)
			}
			if cb > 0 {
				r.CoverageEnd = time.UnixMilli(cb).UTC().Format(time.RFC3339Nano)
			}
			if r.ItemID != "" {
				r.LibraryID = digest([]string{"private-recorded-library-v1", o.Key()})
			}
			r.Conflicts = []livechannels.LosingInterval{}
			out.Recordings = append(out.Recordings, r)
			created = append(created, at)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.Recordings) > q.Limit {
			out.Recordings = out.Recordings[:q.Limit]
			c.AfterID = out.Recordings[q.Limit-1].ID
			c.AfterMS = created[q.Limit-1]
			out.NextCursor = s.cursor(c)
		}
		for i := range out.Recordings {
			if e = decorateTx(ctx, tx, o, &out.Recordings[i]); e != nil {
				return e
			}
		}
		for i, r := range out.Recordings {
			if mutable(r.State) {
				intervals, e := conflictsTx(ctx, tx, r)
				if errors.Is(e, livechannels.ErrConflict) {
					continue
				}
				if e != nil {
					return e
				}
				out.Recordings[i].Conflicts = intervals
				if len(intervals) > 0 && r.State == "scheduled" {
					out.Recordings[i].State = "conflicted"
					out.Recordings[i].Reason = "tuner-conflict"
				}
			}
		}
		return nil
	})
	return out, e
}
