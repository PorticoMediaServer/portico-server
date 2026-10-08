package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/personalstate"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

// ProgressEntry is one observation a client recorded while it had no server in
// reach. observedAt is the client's own clock at the moment of the observation,
// which is the only ordering authority available for work done offline.
type ProgressEntry struct {
	ItemID          string  `json:"itemId"`
	PositionSeconds float64 `json:"positionSeconds"`
	Watched         *bool   `json:"watched,omitempty"`
	ObservedAt      string  `json:"observedAt"`
}

// Progress outcome codes, reported per entry.
const (
	ProgressApplied       = "applied"
	ProgressHistoryPaused = "history_paused"
	ProgressStale         = "stale_observation"
	ProgressSuperseded    = "superseded_online"
	ProgressMissing       = "item_deleted"
	ProgressInvalid       = "invalid_entry"
	ProgressFuture        = "observation_in_future"
)

// ProgressResult reports what happened to one entry, and what the server now
// holds, so a client can reconcile without a second read.
type ProgressResult struct {
	ItemID          string  `json:"itemId"`
	Outcome         string  `json:"outcome"`
	PositionSeconds float64 `json:"positionSeconds"`
	Watched         bool    `json:"watched"`
	ObservedAt      string  `json:"observedAt,omitempty"`
}

// ProgressReceipt is the answer to one deferred batch.
type ProgressReceipt struct {
	Applied   int              `json:"applied"`
	Conflicts int              `json:"conflicts"`
	Entries   []ProgressResult `json:"entries"`
}

// clockSkew is how far into the future an offline observation may sit before
// the server refuses it. A client whose clock ran ahead would otherwise pin its
// item against every later observation from every other device.
const clockSkew = 24 * time.Hour

// SyncProgress applies a batch of offline observations with last-write-wins by
// observedAt. The comparison is against two things: the last offline
// observation this server accepted for that viewer and item, and the last
// online activity it recorded. An entry older than either loses and is reported
// as a conflict rather than silently dropped.
func (s *Service) SyncProgress(ctx context.Context, p identity.Principal, operationID string, entries []ProgressEntry) (ProgressReceipt, error) {
	out := ProgressReceipt{Entries: []ProgressResult{}}
	if !validID.MatchString(operationID) || len(entries) == 0 || len(entries) > MaxBatchTargets {
		return out, ErrInput
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	now := s.millis()
	key := operations.ViewerKey(p)
	scope := "downloads-progress:" + key
	raw, digest, e := operations.Receipt(tx, scope, operationID, entries, now)
	if e != nil {
		return out, e
	}
	if raw != "" {
		return out, json.Unmarshal([]byte(raw), &out)
	}
	preferences, _, _, e := operations.EffectivePreferences(tx, p.Viewer, "")
	if e != nil {
		return out, e
	}
	personal := identity.PersonalKey(p.Viewer)
	for _, entry := range entries {
		result := ProgressResult{ItemID: entry.ItemID, ObservedAt: entry.ObservedAt}
		observed, err := time.Parse(time.RFC3339, entry.ObservedAt)
		if !validID.MatchString(entry.ItemID) || err != nil || entry.PositionSeconds < 0 || math.IsNaN(entry.PositionSeconds) || math.IsInf(entry.PositionSeconds, 0) {
			result.Outcome = ProgressInvalid
			out.Entries, out.Conflicts = append(out.Entries, result), out.Conflicts+1
			continue
		}
		observedMS := observed.UnixMilli()
		if observedMS > now+int64(clockSkew/time.Millisecond) {
			result.Outcome = ProgressFuture
			out.Entries, out.Conflicts = append(out.Entries, result), out.Conflicts+1
			continue
		}
		var library string
		var duration float64
		var entity int64
		entity, err = entityid.Resolve(ctx, tx, entry.ItemID)
		if errors.Is(err, entityid.ErrNotFound) {
			result.Outcome = ProgressMissing
			out.Entries, out.Conflicts = append(out.Entries, result), out.Conflicts+1
			continue
		}
		if err != nil {
			return out, err
		}
		err = tx.QueryRowContext(ctx, `SELECT cl.library_id,COALESCE((SELECT max(a.duration) FROM catalog_asset_links ia JOIN catalog_assets a ON a.id=ia.asset_id WHERE ia.entity_id=e.id),0) FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.id=?`, entity).Scan(&library, &duration)
		if errors.Is(err, sql.ErrNoRows) {
			result.Outcome = ProgressMissing
			out.Entries, out.Conflicts = append(out.Entries, result), out.Conflicts+1
			continue
		}
		if err != nil {
			return out, err
		}
		if preferences.Bool("privacy.pauseWatchHistory") {
			current, watched, err := readCurrentProgress(ctx, tx, personal, entity)
			if err != nil {
				return out, err
			}
			result.Outcome, result.PositionSeconds, result.Watched = ProgressHistoryPaused, current, watched
			out.Entries = append(out.Entries, result)
			continue
		}
		var mark int64
		err = tx.QueryRowContext(ctx, `SELECT observed_ms FROM download_progress_marks WHERE profile_key=? AND item_id=?`, key, entity).Scan(&mark)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		if observedMS <= mark {
			current, watched, e := readCurrentProgress(ctx, tx, personal, entity)
			if e != nil {
				return out, e
			}
			result.Outcome, result.PositionSeconds, result.Watched = ProgressStale, current, watched
			out.Entries, out.Conflicts = append(out.Entries, result), out.Conflicts+1
			continue
		}
		// Online activity carries an RFC3339 instant of its own. A newer online
		// write wins: the viewer was demonstrably at a server after this
		// observation was recorded.
		var updated string
		err = tx.QueryRowContext(ctx, `SELECT updated_at FROM progress_activity WHERE profile_id=? AND item_id=?`, personal, entity).Scan(&updated)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		if err == nil {
			if online, e := time.Parse(time.RFC3339, updated); e == nil && online.After(observed) {
				current, watched, e := readCurrentProgress(ctx, tx, personal, entity)
				if e != nil {
					return out, e
				}
				result.Outcome, result.PositionSeconds, result.Watched = ProgressSuperseded, current, watched
				out.Entries, out.Conflicts = append(out.Entries, result), out.Conflicts+1
				continue
			}
		}
		if duration > 0 && entry.PositionSeconds > duration {
			entry.PositionSeconds = duration
		}
		if err = applyProgress(ctx, tx, personal, entity, library, entry, observed); err != nil {
			return out, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO download_progress_marks(profile_key,item_id,observed_ms) VALUES(?,?,?) ON CONFLICT(profile_key,item_id) DO UPDATE SET observed_ms=excluded.observed_ms`, key, entity, observedMS); err != nil {
			return out, err
		}
		current, watched, err := readCurrentProgress(ctx, tx, personal, entity)
		if err != nil {
			return out, err
		}
		result.Outcome, result.PositionSeconds, result.Watched = ProgressApplied, current, watched
		out.Entries, out.Applied = append(out.Entries, result), out.Applied+1
	}
	if e = operations.SaveReceipt(tx, scope, operationID, digest, out, now); e != nil {
		return out, e
	}
	return out, gated.Commit()
}

func readCurrentProgress(ctx context.Context, tx *sql.Tx, personal string, entity int64) (float64, bool, error) {
	var positionMS int64
	var watched bool
	e := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT position FROM progress WHERE profile_id=? AND item_id=?),0),`+personalstate.SQL("?", "?")+``, personal, entity, personal, entity).Scan(&positionMS, &watched)
	return float64(positionMS) / 1000, watched, e
}

// applyProgress writes the ordinary personal state an online client would have
// written. Offline work is not a second kind of progress: it lands in the same
// rows, with the same projections kept coherent.
func applyProgress(ctx context.Context, tx *sql.Tx, personal string, entity int64, library string, entry ProgressEntry, observed time.Time) error {
	if _, e := tx.ExecContext(ctx, `INSERT INTO progress(profile_id,item_id,position,unit,playback_id) VALUES(?,?,?,0,'') ON CONFLICT(profile_id,item_id) DO UPDATE SET position=excluded.position,unit=0`, personal, entity, int64(math.Round(entry.PositionSeconds*1000))); e != nil {
		return e
	}
	state := "paused"
	if entry.PositionSeconds == 0 {
		state = "ended"
	}
	if entry.Watched != nil && *entry.Watched {
		state = "ended"
	}
	stampText := observed.UTC().Format(time.RFC3339)
	if _, e := tx.ExecContext(ctx, `INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) VALUES(?,?,?,?,?) ON CONFLICT(profile_id,item_id) DO UPDATE SET updated_at=excluded.updated_at,state=excluded.state`, personal, library, entity, stampText, state); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `UPDATE book_resume SET position=?,unit=0 WHERE profile_id=? AND item_id=?`, int64(math.Round(entry.PositionSeconds*1000)), personal, entity); e != nil {
		return e
	}
	if entry.Watched == nil {
		return nil
	}
	// rating stays NULL: personal_items constrains it to a half-star value, and a
	// progress observation is not a rating.
	if e := personalstate.Write(tx, personal, entity, *entry.Watched); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,rating,revision,watched,last_played_at) VALUES(?,?,0,0,NULL,1,?,?) ON CONFLICT(profile_id,item_id) DO UPDATE SET watched=excluded.watched,revision=personal_items.revision+1,last_played_at=excluded.last_played_at`, personal, entity, *entry.Watched, stampText)
	return e
}
