package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/personalstate"
	"time"
)

// AcceptPlaybackProgress consumes already-authorized, sequence-accepted evidence.
// Playback alone selects the canonical writer. All occurrences retain history;
// an explicit personal fence prevents an old writer undoing a manual choice.
// Called in the playback transaction, with no network or source IO.
//
// Two histories are kept. The server's play history (play_history.go) is the
// owner's and records every play; the viewer's own (personal_history, and the
// watched state and resume point that go with it) stops while they have paused
// it.
func (s *Service) AcceptPlaybackProgress(tx *sql.Tx, v identity.Viewer, playbackID, item string, sequence int64, position, duration float64, state string, canonical bool) error {
	preferences, preferenceSources, _, e := operations.EffectivePreferences(tx, v, "")
	if e != nil {
		return e
	}
	started := float64(preferences.Int("playback.startedThresholdPercent"))
	settings, e := continueWatchingSettingsTx(tx, item)
	if e != nil {
		return e
	}
	entityID, e := personalID(tx, item)
	if e != nil {
		return e
	}
	var kind int64
	if e = tx.QueryRow(`SELECT kind FROM catalog_entities WHERE id=?`, entityID).Scan(&kind); e != nil {
		return e
	}
	played := float64(settings.VideoPlayedThreshold)
	if (kind != int64(compactcatalog.Movie) && kind != int64(compactcatalog.Episode)) || preferenceSources["playback.playedThresholdPercent"] != "default" {
		played = float64(preferences.Int("playback.playedThresholdPercent"))
	}
	profile := identity.PersonalKey(v)
	clock := time.Now().UTC()
	now := clock.Format("2006-01-02T15:04:05.000Z")
	position = math.Max(0, position)
	if duration > 0 {
		position = math.Min(position, duration)
	}
	video := kind == int64(compactcatalog.Movie) || kind == int64(compactcatalog.Episode)
	thresholdHit := duration > 0 && position >= duration*played/100
	creditsHit := false
	if video && settings.VideoCompletion != "threshold" && (settings.VideoCompletion == "credits" || !thresholdHit) {
		creditsStart, creditsErr := firstApprovedCreditsTx(tx, item)
		if creditsErr != nil {
			return creditsErr
		}
		creditsHit = creditsStart.Valid && position >= creditsStart.Float64
	}
	completed := state == "ended" || ((!video || settings.VideoCompletion != "credits") && thresholdHit) || creditsHit
	// A play has started once it is past the point where it would resume: thirty
	// seconds at most, sooner for a short title, at once for an audiobook file.
	threshold := 30.
	if duration > 0 {
		threshold = math.Min(threshold, duration*started/100)
	}
	if kind == int64(compactcatalog.Part) {
		threshold = 1
	}
	counted, e := recordPlay(tx, v, playbackID, entityID, kind, clock, positionMillis(position), duration, position >= threshold, completed)
	if e != nil {
		return e
	}
	// A paused history records nothing new of the viewer's own. Existing progress
	// still resumes, and watched/watchlist mutations keep their own path.
	if preferences.Bool("privacy.pauseWatchHistory") {
		return nil
	}
	if counted {
		if e = countPersonalPlay(tx, profile, entityID); e != nil {
			return e
		}
	}
	positionMs := positionMillis(position)
	_, e = tx.Exec(`INSERT INTO personal_history(id,profile_id,item_id,started_at,updated_at,sequence,position,unit,completed) VALUES(?,?,?,?,?,?,?,0,?) ON CONFLICT(profile_id,id) DO UPDATE SET updated_at=excluded.updated_at,sequence=excluded.sequence,position=excluded.position,completed=excluded.completed WHERE excluded.sequence>personal_history.sequence`, playbackID, profile, entityID, now, now, sequence, positionMs, completed)
	if e != nil {
		return e
	}
	out, e := readPersonalByID(tx, profile, entityID, personalstate.SQL("?", "?"))
	if e != nil {
		return e
	}
	if e = snapshotPersonalByID(tx, profile, entityID, out); e != nil {
		return e
	}
	out.LastPlayedAt = now
	var blocked int
	if e = tx.QueryRow(`SELECT (SELECT count(*) FROM personal_evidence_fences WHERE profile_id=? AND item_id=? AND playback_id=?)+(SELECT count(*) FROM playback_personal_fences f WHERE f.profile_key=? AND f.item_id=? AND f.through_ordinal>=COALESCE((SELECT accepted_ordinal FROM playback_personal_claims WHERE playback_id=? AND profile_key=f.profile_key AND item_id=f.item_id),0))+(SELECT count(*) FROM playback_personal_profile_fences f WHERE f.profile_key=? AND f.through_ordinal>=COALESCE((SELECT accepted_ordinal FROM playback_personal_claims WHERE playback_id=? AND profile_key=f.profile_key),0))`, profile, entityID, playbackID, profile, entityID, playbackID, profile, playbackID).Scan(&blocked); e != nil {
		return e
	}
	for _, c := range out.Conflicts {
		if c.Field == "watched" {
			blocked++
		}
	}
	if canonical && blocked == 0 {
		switch {
		case completed:
			out.Watched = true
			out.ProgressSeconds = 0
		case position >= threshold:
			out.Watched = false
			out.ProgressSeconds = position
		default:
			out.ProgressSeconds = 0 // a brief replay must not clear prior completion
		}
		if completed || position >= threshold {
			if e = personalstate.Write(tx, profile, entityID, out.Watched); e != nil {
				return e
			}
		}
		if kind == int64(compactcatalog.Track) {
			out.ProgressSeconds = 0
		}
		if _, e = tx.Exec(`UPDATE progress SET position=? WHERE profile_id=? AND item_id=? AND playback_id=?`, positionMillis(out.ProgressSeconds), profile, entityID, playbackID); e != nil {
			return e
		}
		activityState := state
		if completed {
			activityState = "ended"
		}
		if _, e = tx.Exec(`INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) SELECT ?,cl.library_id,e.id,?,? FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.id=? ON CONFLICT(profile_id,item_id) DO UPDATE SET updated_at=excluded.updated_at,state=excluded.state`, profile, now, activityState, entityID); e != nil {
			return e
		}
		if _, e = tx.Exec(`UPDATE book_resume SET position=? WHERE profile_id=? AND item_id=? AND playback_id=?`, positionMillis(out.ProgressSeconds), profile, entityID, playbackID); e != nil {
			return e
		}
	}
	out.Revision++
	if e = storePersonalByID(tx, profile, entityID, out); e != nil {
		return e
	}
	return snapshotPersonalByID(tx, profile, entityID, out)
}

type HistoryEntry struct {
	ID        string       `json:"id"`
	StartedAt string       `json:"startedAt"`
	UpdatedAt string       `json:"updatedAt"`
	Position  float64      `json:"positionSeconds"`
	Completed bool         `json:"completed"`
	Media     ContentEntry `json:"media"`
}
type HistoryPage struct {
	ServerID    string         `json:"serverId"`
	ViewerFence string         `json:"viewerFence"`
	Revision    int64          `json:"revision"`
	Period      string         `json:"period"`
	LibraryID   string         `json:"libraryId"`
	Entries     []HistoryEntry `json:"entries"`
	NextCursor  string         `json:"nextCursor"`
}

// HistoryPeriods are the windows a client may ask for. The server computes the
// boundary so every device agrees on what "last 7 days" means.
var HistoryPeriods = map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour, "90d": 90 * 24 * time.Hour, "all": 0}

func (s *Service) History(server, fence, profile, cursor, period, libraryID string, libraries []string, limit int) (HistoryPage, error) {
	if dbwork.Snapshot(s.Context()) == nil {
		var out HistoryPage
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var err error
			out, err = s.WithContext(ctx).History(server, fence, profile, cursor, period, libraryID, libraries, limit)
			return err
		})
		return out, err
	}
	out := HistoryPage{ServerID: server, ViewerFence: fence, Entries: []HistoryEntry{}, Period: period, LibraryID: libraryID}
	if limit < 1 || limit > 100 {
		return out, ErrCursor
	}
	if period == "" {
		period = "all"
		out.Period = period
	}
	window, ok := HistoryPeriods[period]
	if !ok {
		return out, ErrCursor
	}
	if libraryID != "" {
		allowed := false
		for _, id := range libraries {
			if id == libraryID {
				allowed = true
			}
		}
		if !allowed {
			return out, identity.ErrUnauthorized
		}
		libraries = []string{libraryID}
	}
	if e := s.read().QueryRow(`SELECT COALESCE((SELECT revision FROM personal_activity_revisions WHERE profile_id=?),0)`, profile).Scan(&out.Revision); e != nil {
		return out, e
	}
	rev, e := s.homeRevision(libraries, profile)
	if e != nil {
		return out, e
	}
	rev.Viewer += out.Revision
	scope := cursorScope{Profile: profile, View: "history", Viewer: fence, Limit: limit, Category: period, Library: libraryID}
	raw, _ := json.Marshal(libraries)
	where := ` FROM personal_history h CROSS JOIN catalog_entities i ON i.id=h.item_id CROSS JOIN catalog_item_details detail ON detail.entity_id=i.id WHERE h.profile_id=? AND i.retired=0 AND i.library_id IN(SELECT id FROM catalog_libraries WHERE library_id IN(SELECT value FROM json_each(?)) AND retired=0)`
	args := []any{profile, string(raw)}
	if window > 0 {
		where += ` AND h.updated_at>=?`
		args = append(args, time.Now().UTC().Add(-window).Format("2006-01-02T15:04:05.000Z"))
	}
	if cursor != "" {
		c, e := s.decodeRevisionCursor(cursor, scope, rev)
		if e != nil {
			return out, e
		}
		where += ` AND (h.updated_at,h.id)<(?,?)`
		args = append(args, c.Value, c.ID)
	}
	args = append(args, limit+1)
	rows, e := s.read().Query(`SELECT h.id,pid(i.public_id),h.started_at,h.updated_at,h.position/1000.0,h.completed`+where+` ORDER BY h.updated_at DESC,h.id DESC LIMIT ?`, args...)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var row HistoryEntry
		var item string
		if e = rows.Scan(&row.ID, &item, &row.StartedAt, &row.UpdatedAt, &row.Position, &row.Completed); e != nil {
			rows.Close()
			return out, e
		}
		out.Entries = append(out.Entries, row)
		ids = append(ids, item)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Entries) > limit {
		out.Entries = out.Entries[:limit]
		ids = ids[:limit]
		last := out.Entries[limit-1]
		out.NextCursor, e = s.encodeRevisionCursor(cursorValue{Scope: scope, Value: last.UpdatedAt, ID: last.ID, Expires: time.Now().Add(30 * time.Minute).Unix()}, rev)
		if e != nil {
			return out, e
		}
	}
	items, e := s.mediaPage(profile, ids, false)
	if e != nil {
		return out, e
	}
	byID := map[string]Item{}
	for _, i := range items {
		byID[i.ID] = i
	}
	for i, id := range ids {
		item, ok := byID[id]
		if !ok {
			return out, ErrStaleContinuation
		}
		out.Entries[i].Media = contentItem(item)
		if !item.Available {
			out.Entries[i].Media.Playback = nil
		}
	}
	var after int64
	if e = s.read().QueryRow(`SELECT COALESCE((SELECT revision FROM personal_activity_revisions WHERE profile_id=?),0)`, profile).Scan(&after); e != nil {
		return out, e
	}
	afterCatalog, e := s.homeRevision(libraries, profile)
	if e != nil {
		return out, e
	}
	afterCatalog.Viewer += after
	if afterCatalog != rev {
		return out, ErrStaleContinuation
	}
	return out, nil
}

type ActivityMutation struct {
	OperationID      string `json:"operationId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Action           string `json:"action"`
}
type ActivityReceipt struct {
	OperationID string `json:"operationId"`
	Revision    int64  `json:"revision"`
	Action      string `json:"action"`
}

func (s *Service) MutateActivity(profile string, m ActivityMutation, authorize func(*sql.Tx) error) (ActivityReceipt, error) {
	out := ActivityReceipt{OperationID: m.OperationID, Action: m.Action}
	if !personalOperation.MatchString(m.OperationID) || m.ExpectedRevision < 0 || (m.Action != "clear-history" && m.Action != "reset-viewing-activity") {
		return out, errors.New("invalid activity operation")
	}
	gated, e := dbwork.Begin(context.Background(), s.db, dbwork.ClassFrom(context.Background(), dbwork.ClassInteractive))
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if e = authorize(tx); e != nil {
		return out, e
	}
	hash := operationHash(m)
	var prior, raw string
	e = tx.QueryRow(`SELECT request_hash,response FROM personal_activity_receipts WHERE profile_id=? AND operation_id=? AND created_at>=?`, profile, m.OperationID, time.Now().Add(-30*24*time.Hour).UTC().Format(time.RFC3339)).Scan(&prior, &raw)
	if e == nil {
		if prior != hash {
			return out, ErrOperationConflict
		}
		e = json.Unmarshal([]byte(raw), &out)
		return out, e
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	if e = checkRetired(tx, profile, "activity", m.OperationID, hash); e != nil {
		return out, e
	}
	if e = tx.QueryRow(`SELECT COALESCE((SELECT revision FROM personal_activity_revisions WHERE profile_id=?),0)`, profile).Scan(&out.Revision); e != nil {
		return out, e
	}
	if out.Revision != m.ExpectedRevision {
		return out, ErrPersonalConflict
	}
	if m.Action == "reset-viewing-activity" {
		if _, e = tx.Exec(`INSERT INTO playback_personal_profile_fences(profile_key,through_ordinal) VALUES(?,(SELECT COALESCE(max(ordinal),0) FROM playback_personal_intents)) ON CONFLICT(profile_key) DO UPDATE SET through_ordinal=excluded.through_ordinal`, profile); e != nil {
			return out, e
		}
		if _, e = tx.Exec(`INSERT INTO personal_evidence_fences SELECT profile_id,item_id,playback_id FROM progress WHERE profile_id=? ON CONFLICT(profile_id,item_id) DO UPDATE SET playback_id=excluded.playback_id`, profile); e != nil {
			return out, e
		}
		for _, table := range []string{"container_personal_state", "personal_watched_intents", "container_personal_resets", "personal_play_counts"} {
			if _, e = tx.Exec(`DELETE FROM `+table+` WHERE profile_id=?`, profile); e != nil {
				return out, e
			}
		}
		if _, e = tx.Exec(`UPDATE personal_items SET watched=0,last_played_at='',revision=revision+1 WHERE profile_id=?`, profile); e != nil {
			return out, e
		}
		if _, e = tx.Exec(`UPDATE progress SET position=0 WHERE profile_id=?`, profile); e != nil {
			return out, e
		}
		if _, e = tx.Exec(`DELETE FROM progress_activity WHERE profile_id=?`, profile); e != nil {
			return out, e
		}
		if _, e = tx.Exec(`DELETE FROM profile_show_activity WHERE profile_id=?`, profile); e != nil {
			return out, e
		}
		if _, e = tx.Exec(`DELETE FROM continue_show_dismissals WHERE profile_id=?`, profile); e != nil {
			return out, e
		}
		if _, e = tx.Exec(`UPDATE book_resume SET position=0 WHERE profile_id=?`, profile); e != nil {
			return out, e
		}
		if _, e = tx.Exec(`UPDATE personal_conflicts SET resolved=1 WHERE profile_id=? AND field='watched'`, profile); e != nil {
			return out, e
		}
	}
	if _, e = tx.Exec(`DELETE FROM personal_history WHERE profile_id=?`, profile); e != nil {
		return out, e
	}
	if _, e = tx.Exec(`INSERT INTO personal_activity_revisions VALUES(?,1) ON CONFLICT(profile_id) DO UPDATE SET revision=revision+1`, profile); e != nil {
		return out, e
	}
	if e = tx.QueryRow(`SELECT revision FROM personal_activity_revisions WHERE profile_id=?`, profile).Scan(&out.Revision); e != nil {
		return out, e
	}
	if e = fenceOperation(tx, profile, "activity", m.OperationID, hash); e != nil {
		return out, e
	}
	b, _ := json.Marshal(out)
	if _, e = tx.Exec(`INSERT INTO personal_activity_receipts VALUES(?,?,?,?,?)`, profile, m.OperationID, hash, string(b), time.Now().UTC().Format(time.RFC3339)); e != nil {
		return out, e
	}
	return out, gated.Commit()
}
