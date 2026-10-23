package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadataprovider"
)

type MetadataRefreshRequest struct {
	ExpectedRevision int64 `json:"expectedRevision"`
}
type MetadataRefreshStatus struct {
	Queueing    bool   `json:"queueing"`
	Queued      int64  `json:"queued"`
	RequestedAt string `json:"requestedAt"`
	EnqueuedAt  string `json:"enqueuedAt"`
}

func readRefreshStatus(ctx context.Context, tx *sql.Tx) (MetadataRefreshStatus, error) {
	var out MetadataRefreshStatus
	err := tx.QueryRowContext(ctx, `SELECT running AND forced,queued,requested_at,completed_at FROM metadata_refresh_sweep WHERE singleton=1`).Scan(&out.Queueing, &out.Queued, &out.RequestedAt, &out.EnqueuedAt)
	return out, err
}

// RefreshAllMetadata starts one durable bounded sweep. Repeating the action
// while it runs returns that sweep; it never restarts or duplicates its work.
func (s *Service) RefreshAllMetadata(ctx context.Context, in MetadataRefreshRequest, authorize func(*sql.Tx) error) (MetadataRefreshStatus, error) {
	if in.ExpectedRevision < 1 {
		return MetadataRefreshStatus{}, ErrRepairInput
	}
	w, err := dbwork.Begin(ctx, s.db, dbwork.ClassInteractive)
	if err != nil {
		return MetadataRefreshStatus{}, err
	}
	defer w.Rollback()
	tx := w.Tx()
	if authorize != nil {
		if err = authorize(tx); err != nil {
			return MetadataRefreshStatus{}, err
		}
	}
	settings, _, _, err := readMetadataSettings(ctx, tx)
	if err != nil {
		return MetadataRefreshStatus{}, err
	}
	if settings.Revision != in.ExpectedRevision {
		return MetadataRefreshStatus{}, ErrScreenConflict
	}
	if !settings.OnlineLookups {
		return MetadataRefreshStatus{}, ErrLocalMetadataOnly
	}
	out, err := readRefreshStatus(ctx, tx)
	if err != nil {
		return out, err
	}
	if !out.Queueing {
		_, err = tx.ExecContext(ctx, `UPDATE metadata_refresh_sweep SET running=1,forced=1,phase='item',cursor=0,bound=(SELECT COALESCE(max(id),0) FROM catalog_entities),queued=0,requested_at=?,completed_at='',next_check=0 WHERE singleton=1`, tvdbStamp(s.publicationTime()))
		if err != nil {
			return out, err
		}
	}
	out, err = readRefreshStatus(ctx, tx)
	if err != nil {
		return out, err
	}
	return out, w.Commit()
}

// GlobalRefreshStep walks primary-key pages of 32 titles. It neither scans a
// whole library inside a writer nor performs network I/O. Normal refreshes use
// document freshness, while explicit refreshes retain identity, edits and locks.
func (s *Service) GlobalRefreshStep(ctx context.Context) error {
	w, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer w.Rollback()
	tx := w.Tx()
	var phase, policy string
	var cursor, bound, next int64
	var running, forced, online bool
	if err = tx.QueryRowContext(ctx, `SELECT s.phase,s.cursor,s.bound,s.running,s.forced,s.next_check,r.refresh_policy,c.confirmed FROM metadata_refresh_sweep s CROSS JOIN metadata_runtime_settings r CROSS JOIN screen_metadata_consent c WHERE s.singleton=1 AND r.singleton=1 AND c.singleton=1`).Scan(&phase, &cursor, &bound, &running, &forced, &next, &policy, &online); err != nil {
		return err
	}
	now := s.publicationTime()
	if !online || !running && (policy == "never" || next > now.Unix()) {
		return nil
	}
	if !running {
		running = true
		forced = false
		phase = "item"
		cursor = 0
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(id),0) FROM catalog_entities`).Scan(&bound); err != nil {
			return err
		}
	}
	type target struct {
		id                                                     int64
		library, status, provider, identity, payload, observed string
	}
	query := `SELECT w.target_id,w.library_id,w.status,w.provider,w.provider_id,COALESCE(p.payload,''),COALESCE(p.observed_at,'') FROM screen_metadata_work w LEFT JOIN screen_metadata_publications p ON p.id=w.accepted_publication WHERE w.target_kind=? AND w.target_id>? AND w.target_id<=? ORDER BY w.target_id LIMIT 32`
	if phase == "album" || phase == "song" {
		column := "album_id"
		if phase == "song" {
			column = "item_id"
		}
		query = `SELECT j.entity_id,cl.library_id,j.status,'musicbrainz',j.selected_id,'',COALESCE(l.observed_at,'') FROM mb_jobs j JOIN catalog_entities e ON e.id=j.entity_id JOIN catalog_libraries cl ON cl.id=e.library_id LEFT JOIN mb_` + phase + `_links l ON l.` + column + `=j.entity_id WHERE j.kind=? AND j.entity_id>? AND j.entity_id<=? ORDER BY j.entity_id LIMIT 32`
	}
	rows, err := tx.QueryContext(ctx, query, phase, cursor, bound)
	if err != nil {
		return err
	}
	targets := []target{}
	for rows.Next() {
		var t target
		if err = rows.Scan(&t.id, &t.library, &t.status, &t.provider, &t.identity, &t.payload, &t.observed); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	queued := 0
	for _, t := range targets {
		cursor = t.id
		local, e := LibraryLocalOnlyTx(ctx, tx, t.library)
		if e != nil {
			return e
		}
		if local {
			continue
		}
		var enabled bool
		if phase == "album" || phase == "song" {
			e = tx.QueryRowContext(ctx, `SELECT enabled FROM mb_provider_policies WHERE library_id=?`, t.library).Scan(&enabled)
		} else {
			e = tx.QueryRowContext(ctx, `SELECT enabled AND EXISTS(SELECT 1 FROM json_each(providers) WHERE value=?) FROM screen_metadata_policies WHERE library_id=?`, t.provider, t.library).Scan(&enabled)
		}
		if errors.Is(e, sql.ErrNoRows) {
			continue
		}
		if e != nil {
			return e
		}
		if !enabled {
			continue
		}
		if (t.status != "matched" && t.status != "delegated_tvdb") || t.identity == "" {
			continue
		}
		due := forced
		if !due {
			at, e := time.Parse(time.RFC3339, t.observed)
			if e != nil {
				continue
			}
			priority := RefreshOrdinary
			kind := phase
			if phase == "item" || phase == "show" {
				var record metadataprovider.ScreenRecord
				if json.Unmarshal([]byte(t.payload), &record) != nil {
					continue
				}
				kind = record.Identity.Type
				if airingStatuses[record.Status] {
					priority = RefreshAiring
				}
			}
			var added sql.NullString
			q := `SELECT added_text FROM catalog_item_details WHERE entity_id=?`
			if phase == "show" {
				q = `SELECT min(d.added_text) FROM catalog_episodes e JOIN catalog_item_details d ON d.entity_id=e.entity_id WHERE e.show_id=?`
			} else if phase == "album" {
				q = `SELECT min(d.added_text) FROM catalog_songs s JOIN catalog_item_details d ON d.entity_id=s.entity_id WHERE s.album_id=?`
			}
			if e = tx.QueryRowContext(ctx, q, t.id).Scan(&added); e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
			if stamp, e := time.Parse(time.RFC3339, added.String); e == nil && now.Sub(stamp) < recentlyAdded {
				priority = RefreshRecentlyAdded
			}
			if policy == "never" || policy == "recent" && priority == RefreshOrdinary {
				continue
			}
			due = now.Sub(at) >= documentFreshness(t.provider, kind, priority)
		}
		if !due {
			continue
		}
		if phase == "item" || phase == "show" {
			_, err = tx.ExecContext(ctx, `UPDATE screen_metadata_work SET status='pending',force_refresh=?,generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error='' WHERE target_kind=? AND target_id=?`, forced, phase, t.id)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE mb_jobs SET force_refresh=?,status='pending',generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error='' WHERE kind=? AND entity_id=?`, forced, phase, t.id)
		}
		if err != nil {
			return err
		}
		queued++
	}
	if len(targets) < 32 {
		cursor = 0
		switch phase {
		case "item":
			phase = "show"
		case "show":
			phase = "album"
		case "album":
			phase = "song"
		case "song":
			running = false
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE metadata_refresh_sweep SET phase=?,cursor=?,bound=?,running=?,forced=?,queued=CASE WHEN ? THEN queued+? ELSE ? END,next_check=?,completed_at=CASE WHEN ? THEN completed_at ELSE ? END WHERE singleton=1`, phase, cursor, bound, running, forced, forced, queued, queued, now.Add(time.Hour).Unix(), running, tvdbStamp(now))
	if err != nil {
		return err
	}
	return w.Commit()
}

func (s *Service) MetadataRefreshStatus(ctx context.Context) (MetadataRefreshStatus, error) {
	r, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return MetadataRefreshStatus{}, err
	}
	defer r.Rollback()
	return readRefreshStatus(ctx, r.Tx())
}

type musicRefreshKey struct{}

func refreshBudgetWait(provider string) error {
	return &metadataprovider.Error{Provider: provider, Code: "refresh_budget", Status: 429, RetryAfter: time.Hour}
}
func (s *Service) refreshMusicDocument(ctx context.Context, kind, observed string) (bool, error) {
	priority := RefreshOrdinary
	if v, ok := ctx.Value(musicRefreshPriorityKey{}).(RefreshPriority); ok {
		priority = v
	}
	forced, _ := ctx.Value(musicRefreshKey{}).(bool)
	var policy string
	if e := dbwork.QueryRow(ctx, s.db, `SELECT refresh_policy FROM metadata_runtime_settings WHERE singleton=1`).Scan(&policy); e != nil {
		return false, e
	}
	if !forced {
		if policy == "never" || policy == "recent" && priority == RefreshOrdinary {
			return false, nil
		}
		at, e := time.Parse(time.RFC3339, observed)
		if e != nil || s.publicationTime().Sub(at) < documentFreshness("musicbrainz", kind, priority) {
			return false, nil
		}
	}
	allowed, err := s.admitRefresh(ctx, "musicbrainz", s.publicationTime())
	if err != nil {
		return false, err
	}
	if !allowed && forced {
		return false, refreshBudgetWait("musicbrainz")
	}
	return allowed, nil
}

type musicRefreshPriorityKey struct{}

func (s *Service) musicRefreshContext(ctx context.Context, j mbJob) context.Context {
	var added sql.NullString
	q := `SELECT added_text FROM catalog_item_details WHERE entity_id=?`
	if j.kind == "album" {
		q = `SELECT min(d.added_text) FROM catalog_songs s JOIN catalog_item_details d ON d.entity_id=s.entity_id WHERE s.album_id=?`
	}
	priority := RefreshOrdinary
	if e := dbwork.QueryRow(ctx, s.db, q, j.base.Entity).Scan(&added); e == nil {
		if at, e := time.Parse(time.RFC3339, added.String); e == nil && s.publicationTime().Sub(at) < recentlyAdded {
			priority = RefreshRecentlyAdded
		}
	}
	return context.WithValue(context.WithValue(ctx, musicRefreshKey{}, j.forceRefresh), musicRefreshPriorityKey{}, priority)
}
