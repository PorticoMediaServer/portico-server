package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"portico.local/server/internal/requestinput"
	"strconv"

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
	return acceptPlaybackProgress(tx, v, playbackID, item, sequence, position, duration, state, canonical, 0, nil, time.Now().UTC())
}

// AcceptPlaybackProgressEvidence is AcceptPlaybackProgress with the player's
// own reported rate and whether this report follows a seek. Both are needed to
// prove a played interval; callers that cannot report them keep the plain form
// and record no chapter mark.
func (s *Service) AcceptPlaybackProgressEvidence(tx *sql.Tx, v identity.Viewer, playbackID, item string, sequence int64, position, duration float64, state string, canonical bool, rate float64, seeked *bool) error {
	return acceptPlaybackProgress(tx, v, playbackID, item, sequence, position, duration, state, canonical, rate, seeked, time.Now().UTC())
}

func acceptPlaybackProgress(tx *sql.Tx, v identity.Viewer, playbackID, item string, sequence int64, position, duration float64, state string, canonical bool, rate float64, seeked *bool, clock time.Time) error {
	// Match the timeline's millisecond bound even when no runtime is known.
	const maximumSeconds = float64(int64(1)<<40) / 1000
	if math.IsNaN(position) || math.IsInf(position, 0) || position < 0 || position > maximumSeconds || math.IsNaN(duration) || math.IsInf(duration, 0) || duration < 0 || duration > maximumSeconds || sequence < 0 || sequence > 9007199254740991 || (state != "playing" && state != "paused" && state != "ended") {
		return jobInvalid("invalid playback evidence")
	}
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
	now := clock.Format("2006-01-02T15:04:05.000Z")
	position = math.Max(0, position)
	if duration > 0 {
		position = math.Min(position, duration)
	}
	video := kind == int64(compactcatalog.Movie) || kind == int64(compactcatalog.Episode)
	thresholdHit := duration > 0 && position >= duration*played/100
	if kind == int64(compactcatalog.Part) {
		thresholdHit = duration > 0 && position > 0 && duration-position <= math.Min(30, duration/20)
	}
	creditsHit := false
	creditsAvailable := false
	if video && settings.VideoCompletion != "threshold" && (settings.VideoCompletion == "credits" || !thresholdHit) {
		creditsStart, creditsErr := firstApprovedCreditsTx(tx, item)
		if creditsErr != nil {
			return creditsErr
		}
		creditsAvailable = creditsStart.Valid
		creditsHit = creditsAvailable && position >= creditsStart.Float64
	}
	// An early end-of-stream is not completion of a known longer title.
	// Approved credits and the configured percentage can still finish it early.
	terminal := state == "ended" && (duration <= 0 || position >= duration-math.Min(1, duration/100))
	completed := terminal || ((!video || settings.VideoCompletion != "credits" || !creditsAvailable) && thresholdHit) || creditsHit
	// Known durations honor the viewer percentage; unknown durations start at
	// thirty seconds. Audiobook parts keep the listening rule of one second.
	threshold := 30.
	if duration > 0 {
		threshold = duration * started / 100
	}
	if kind == int64(compactcatalog.Part) {
		threshold = 1
	}
	counted, e := recordPlay(tx, v, playbackID, entityID, kind, clock, positionMillis(position), duration, position >= threshold, completed, !preferences.Bool("privacy.pauseWatchHistory") && preferences.Bool("privacy.showActivityToMembers"), state)
	if e != nil {
		return e
	}
	// A paused history records nothing new of the viewer's own. Existing progress
	// still resumes, and watched/watchlist mutations keep their own path.
	if preferences.Bool("privacy.pauseWatchHistory") {
		return nil
	}
	personalCounted := false
	if completed {
		if personalCounted, e = countPersonalPlay(tx, profile, playbackID, entityID); e != nil {
			return e
		}
	}
	// The last accepted report of this playback is the only interval evidence a
	// chapter mark may extend. It is read before this report replaces it.
	previousRun := chapterRunEvidence{}
	source := chapterSource{}
	var preferenceRevision int64
	if kind == int64(compactcatalog.Part) {
		if previousRun, e = readChapterRun(tx, profile, playbackID); e != nil {
			return e
		}
		if source, e = chapterSourceFor(tx, entityID, playbackID); e != nil {
			return e
		}
		if preferenceRevision, e = profileServerPreferenceRevision(tx, v); e != nil {
			return e
		}
	}
	positionMs := positionMillis(position)
	if position >= threshold || completed {
		_, e = tx.Exec(`INSERT INTO personal_history(id,profile_id,item_id,started_at,updated_at,sequence,position,unit,completed) VALUES(?,?,?,?,?,?,?,0,?) ON CONFLICT(profile_id,id) DO UPDATE SET updated_at=excluded.updated_at,sequence=excluded.sequence,position=excluded.position,completed=max(personal_history.completed,excluded.completed) WHERE excluded.sequence>personal_history.sequence`, playbackID, profile, entityID, now, now, sequence, positionMs, completed)
		if e != nil {
			return e
		}
	}
	out, e := readPersonalByID(tx, profile, entityID, personalstate.SQL("?", "?"))
	if e != nil {
		return e
	}
	previous := out
	previousPosition := out.ProgressSeconds
	previousLastPlayed := out.LastPlayedAt
	meaningful := personalCounted || counted || completed && (!out.Watched || kind == int64(compactcatalog.Part) && positionMillis(position) != positionMillis(previousPosition)) || !completed && position >= threshold && (positionMillis(position) != positionMillis(previousPosition) || out.LastPlayedAt == "")
	previousPlayedAt, previousErr := time.Parse(time.RFC3339Nano, previousLastPlayed)
	if meaningful && (previousErr != nil || clock.After(previousPlayedAt)) {
		out.LastPlayedAt = now
	}
	var blocked int
	if e = tx.QueryRow(`SELECT (SELECT count(*) FROM personal_evidence_fences WHERE profile_id=? AND item_id=? AND playback_id=?)+(SELECT count(*) FROM playback_personal_fences f WHERE f.profile_key=? AND f.item_id=? AND f.through_ordinal>=COALESCE((SELECT accepted_ordinal FROM playback_personal_claims WHERE playback_id=? AND profile_key=f.profile_key AND item_id=f.item_id),0))+(SELECT count(*) FROM playback_personal_profile_fences f WHERE f.profile_key=? AND f.through_ordinal>=COALESCE((SELECT accepted_ordinal FROM playback_personal_claims WHERE playback_id=? AND profile_key=f.profile_key),0))`, profile, entityID, playbackID, profile, entityID, playbackID, profile, playbackID).Scan(&blocked); e != nil {
		return e
	}
	// Historical observations retain history but never overwrite newer personal
	// evidence or an explicit container watermark.
	var latest string
	if e = tx.QueryRow(`SELECT COALESCE(max(stamp),'') FROM (
 SELECT authored_at AS stamp FROM personal_watched_intents WHERE profile_id=? AND item_id=?
 UNION ALL SELECT watermark FROM container_personal_state c WHERE c.profile_id=? AND ((c.kind='show' AND c.container_id=(SELECT show_id FROM catalog_episodes WHERE entity_id=?)) OR (c.kind='season' AND c.container_id=(SELECT season_id FROM catalog_episodes WHERE entity_id=?)) OR (c.kind='album' AND c.container_id=(SELECT album_id FROM catalog_songs WHERE entity_id=?)) OR (c.kind='book' AND c.container_id=(SELECT book_id FROM catalog_book_files WHERE entity_id=?))))`, profile, entityID, profile, entityID, entityID, entityID, entityID).Scan(&latest); e != nil {
		return e
	}
	for _, stamp := range []string{latest, previousLastPlayed} {
		if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil && at.After(clock) {
			blocked++
		}
	}
	for _, c := range out.Conflicts {
		if c.Field == "watched" {
			blocked++
		}
	}
	// Personal chapter completion joins the same accepted transaction, after the
	// paused-history return and the personal, playback and container fences
	// above. A mark needs two consecutive accepted reports of this session that
	// the player advanced by its own reported rate over the time that passed,
	// under the same source, chapter definition and privacy generation. The
	// owner's recordPlay history is never consulted.
	if kind == int64(compactcatalog.Part) {
		switch {
		case blocked != 0:
			// A manual or historical fence ends the run: the next accepted
			// report starts a fresh interval instead of bridging the fence.
			if _, e = tx.Exec(`DELETE FROM personal_playback_runs WHERE profile_id=? AND playback_id=?`, profile, playbackID); e != nil {
				return e
			}
		case canonical || completed:
			if seeked == nil {
				// This client cannot say whether the report follows a jump, so
				// no played interval may be counted from it. Withhold and make
				// the next report start from scratch.
				if _, e = tx.Exec(`DELETE FROM personal_playback_runs WHERE profile_id=? AND playback_id=?`, profile, playbackID); e != nil {
					return e
				}
				break
			}
			currentRun := chapterRunEvidence{sequence: sequence, position: positionMs, state: state, rateMilli: rateMilli(rate), seeked: *seeked, preferenceRevision: preferenceRevision, assetRevision: source.assetRevision, assetID: source.assetID, assetSize: source.assetSize, assetModified: source.assetModified, item: entityID, at: clock, valid: true}
			if source.assetID != 0 && source.assetRevision != 0 {
				if e = recordChapterCompletions(tx, profile, entityID, previousRun, currentRun, clock); e != nil {
					return e
				}
			}
			if e = storeChapterRun(tx, profile, playbackID, currentRun, now); e != nil {
				return e
			}
		}
	}
	if (canonical || completed) && blocked == 0 {
		switch {
		case completed:
			out.Watched = true
			out.ProgressSeconds = 0
			// A book file is played at the threshold like anything else, but
			// its listener is still in it: they keep their place, in the file
			// and in the book, until the file really ends.
			if kind == int64(compactcatalog.Part) {
				out.ProgressSeconds = position
			}
		case position >= threshold:
			out.Watched = false
			out.ProgressSeconds = position
		default:
			out.ProgressSeconds = 0 // a brief replay must not clear prior completion
		}
		if completed || position >= threshold {
			if meaningful {
				if e = personalstate.WriteAt(tx, profile, entityID, out.Watched, clock); e != nil {
					return e
				}
			}
		}
		if kind == int64(compactcatalog.Track) {
			out.ProgressSeconds = 0
		}
		if _, e = tx.Exec(`UPDATE progress SET position=? WHERE profile_id=? AND item_id=? AND (playback_id=? OR ?=1)`, positionMillis(out.ProgressSeconds), profile, entityID, playbackID, completed); e != nil {
			return e
		}
		activityState := state
		if completed {
			activityState = "ended"
		}
		if meaningful {
			if _, e = tx.Exec(`INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) SELECT ?,cl.library_id,e.id,?,? FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.id=? ON CONFLICT(profile_id,item_id) DO UPDATE SET updated_at=excluded.updated_at,state=excluded.state`, profile, now, activityState, entityID); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(`UPDATE book_resume SET position=? WHERE profile_id=? AND item_id=? AND (playback_id=? OR ?=1)`, positionMillis(out.ProgressSeconds), profile, entityID, playbackID, completed); e != nil {
			return e
		}
	}
	if !meaningful || blocked != 0 || (!canonical && !completed) {
		return nil
	}
	// Observations retain current resume/recency without changing the command
	// revision or adding snapshots. New plays and completion remain semantic.
	changed := out.Watched != previous.Watched || previous.LastPlayedAt == "" || personalCounted || counted
	if changed {
		if e = snapshotPersonalByID(tx, profile, entityID, previous); e != nil {
			return e
		}
		out.Revision++
	}
	if e = storePersonalByID(tx, profile, entityID, out); e != nil {
		return e
	}
	if changed {
		return snapshotPersonalByID(tx, profile, entityID, out)
	}
	return nil
}

// chapterRunEvidence is the last accepted report of one playback: the accepted
// sequence, the session's own state and rate, the source and chapter definition
// it was played against, and when it was accepted. It is control evidence, not
// the viewer's resume point or a manual watched choice.
type chapterRunEvidence struct {
	sequence           int64
	position           int64
	state              string
	rateMilli          int64
	seeked             bool
	preferenceRevision int64
	assetRevision      int64
	assetID            int64
	assetSize          int64
	assetModified      int64
	item               int64
	at                 time.Time
	valid              bool
}

// chapterSource is the file's current chapter definition generation and the
// asset the playback session actually selected, which must still be linked.
type chapterSource struct {
	assetRevision int64
	assetID       int64
	assetSize     int64
	assetModified int64
}

func readChapterRun(tx *sql.Tx, profile, playbackID string) (chapterRunEvidence, error) {
	var out chapterRunEvidence
	var stamp string
	err := tx.QueryRow(`SELECT sequence,position_ms,state,rate_milli,seeked,preference_revision,asset_revision,asset_id,asset_size,asset_modified_ns,item_id,reported_at FROM personal_playback_runs WHERE profile_id=? AND playback_id=?`, profile, playbackID).Scan(&out.sequence, &out.position, &out.state, &out.rateMilli, &out.seeked, &out.preferenceRevision, &out.assetRevision, &out.assetID, &out.assetSize, &out.assetModified, &out.item, &stamp)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return out, nil
	}
	out.at, out.valid = at, true
	return out, nil
}

func storeChapterRun(tx *sql.Tx, profile, playbackID string, run chapterRunEvidence, now string) error {
	_, err := tx.Exec(`INSERT INTO personal_playback_runs(profile_id,playback_id,item_id,sequence,position_ms,state,rate_milli,seeked,preference_revision,asset_revision,asset_id,asset_size,asset_modified_ns,reported_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(profile_id,playback_id) DO UPDATE SET item_id=excluded.item_id,sequence=excluded.sequence,position_ms=excluded.position_ms,state=excluded.state,rate_milli=excluded.rate_milli,seeked=excluded.seeked,preference_revision=excluded.preference_revision,asset_revision=excluded.asset_revision,asset_id=excluded.asset_id,asset_size=excluded.asset_size,asset_modified_ns=excluded.asset_modified_ns,reported_at=excluded.reported_at`, profile, playbackID, run.item, run.sequence, run.position, run.state, run.rateMilli, run.seeked, run.preferenceRevision, run.assetRevision, run.assetID, run.assetSize, run.assetModified, now)
	return err
}

// chapterSourceFor resolves the file's definition generation and the asset its
// playback session selected. A file whose session or linked asset is unknown
// yields no source, so nothing is recorded for it.
func chapterSourceFor(tx *sql.Tx, item int64, playbackID string) (chapterSource, error) {
	var out chapterSource
	// The revision comes from asset_chapter_facts, which moves only when the
	// source's chapter boundaries or its size/mtime really change. It is the
	// generation a completion binds to and cannot recur once it moves on.
	err := tx.QueryRow(`SELECT COALESCE(f.revision,0),a.id,a.size,a.modified_ns
 FROM catalog_entities file
 JOIN playback_sessions s ON s.id=?2 AND s.item_id=file.id
 JOIN catalog_assets a ON a.token=s.asset_id
 JOIN catalog_asset_links l ON l.entity_id=file.id AND l.asset_id=a.id
 LEFT JOIN asset_chapter_facts f ON f.asset_id=a.token
 WHERE file.id=?1`, item, playbackID).Scan(&out.assetRevision, &out.assetID, &out.assetSize, &out.assetModified)
	if errors.Is(err, sql.ErrNoRows) {
		// No session or no linked source (a downloaded play, a file being
		// rescanned): there is nothing to bind a completion to.
		return chapterSource{}, nil
	}
	return out, err
}

// profileServerPreferenceRevision is the generation of the profile's server
// preferences. Toggling watch-history privacy moves it, so an interval that
// spans the toggle is never treated as one run.
func profileServerPreferenceRevision(tx *sql.Tx, v identity.Viewer) (int64, error) {
	var out int64
	err := tx.QueryRow(`SELECT COALESCE((SELECT revision FROM preference_scopes WHERE authority=? AND account_id=? AND profile_id=? AND scope='profile-server' AND device_key=''),0)`, v.Authority, v.AccountID, v.ProfileID).Scan(&out)
	return out, err
}

// rateMilli keeps the player's reported rate at thousandths. A session that
// does not report one leaves 0 and cannot prove a played interval.
func rateMilli(rate float64) int64 {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0.25 || rate > 4 {
		return 0
	}
	return int64(math.Round(rate * 1000))
}

// The client reports every ten seconds while playing; an interval longer than
// this is not one uninterrupted report pair. The tolerance is under a second of
// position rounding, never enough to carry a seek across a chapter end.
const (
	chapterReportGap        = 12 * time.Second
	chapterAdvanceTolerance = 500.0
)

// chapterRunContinues proves the interval between two accepted reports of one
// playback is playback, not a jump. Both must be playing reports of the same
// item and session with consecutive accepted sequence, the same player rate,
// the same privacy generation, chapter definition and source asset; the
// position must advance by exactly rate x elapsed time within rounding. A
// pause, resume, seek, skipped report, source change or privacy toggle changes
// one of those and starts a new interval instead.
func chapterRunContinues(previous, current chapterRunEvidence) bool {
	if !previous.valid || !current.valid || previous.item != current.item || current.sequence != previous.sequence+1 {
		return false
	}
	if previous.state != "playing" || current.state != "playing" || current.seeked || current.rateMilli <= 0 || previous.rateMilli != current.rateMilli {
		return false
	}
	if previous.preferenceRevision != current.preferenceRevision || previous.assetRevision != current.assetRevision {
		return false
	}
	if previous.assetID != current.assetID || previous.assetSize != current.assetSize || previous.assetModified != current.assetModified {
		return false
	}
	elapsed := current.at.Sub(previous.at)
	if elapsed <= 0 || elapsed > chapterReportGap {
		return false
	}
	advance := current.position - previous.position
	if advance <= 0 {
		return false
	}
	expected := float64(current.rateMilli) / 1000 * float64(elapsed.Milliseconds())
	return math.Abs(float64(advance)-expected) <= chapterAdvanceTolerance
}

// recordChapterCompletions marks the chapters whose end lies inside an interval
// the session proved was played. One indexed statement finds the candidates by
// their canonical end milliseconds; the exact end counts and one millisecond
// before it does not. Rounded boundaries must still be a real span.
func recordChapterCompletions(tx *sql.Tx, profile string, item int64, previous, current chapterRunEvidence, clock time.Time) error {
	if !chapterRunContinues(previous, current) {
		return nil
	}
	_, err := tx.Exec(`INSERT INTO personal_chapter_completions(profile_id,file_entity_id,chapter_index,start_ms,end_ms,asset_revision,asset_id,asset_size,asset_modified_ns,completed_at)
 SELECT ?,c.file_id,c.chapter_index,CAST(round(c.start_seconds*1000) AS INTEGER),CAST(round(c.end_seconds*1000) AS INTEGER),?,?,?,?,?
 FROM catalog_book_chapters c INDEXED BY catalog_book_chapters_end_ms
 WHERE c.file_id=? AND c.start_seconds>=0 AND c.start_seconds<c.end_seconds AND c.end_seconds>0 AND c.end_seconds<1e15
  AND CAST(round(c.start_seconds*1000) AS INTEGER)<CAST(round(c.end_seconds*1000) AS INTEGER)
  AND CAST(round(c.end_seconds*1000) AS INTEGER)>? AND CAST(round(c.end_seconds*1000) AS INTEGER)<=?
 ON CONFLICT(profile_id,file_entity_id,chapter_index) DO NOTHING`, profile, current.assetRevision, current.assetID, current.assetSize, current.assetModified, clock.Format("2006-01-02T15:04:05.000Z"), item, previous.position, current.position)
	return err
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
var HistoryPeriods = map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour, "90d": 90 * 24 * time.Hour, "1y": 365 * 24 * time.Hour, "all": 0}

func (s *Service) History(server, fence, profile, cursor, period, libraryID string, libraries []string, limit int, viewer Viewer) (HistoryPage, error) {
	if dbwork.Snapshot(s.Context()) == nil {
		var out HistoryPage
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var err error
			out, err = s.WithContext(ctx).History(server, fence, profile, cursor, period, libraryID, libraries, limit, viewer)
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
	visibility, visibleArgs := viewer.itemVisibilitySQL("i.id")
	where += ` AND ` + visibility + recordingsClause("i.id", viewer.EffectiveRestrictions())
	args := append([]any{profile, string(raw)}, visibleArgs...)
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
		return out, requestinput.New("invalid activity operation")
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
		for _, table := range []string{"container_personal_state", "personal_watched_intents", "container_personal_undo", "personal_play_counts", "personal_chapter_completions", "personal_playback_runs"} {
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
	// Explicit History commands must reach other devices even when no title's
	// watched/resume state changed. History observations have their own hints.
	if _, e = tx.Exec(`INSERT INTO api_change_pending(audience,type,resource_kind,resource_id,revision)
 VALUES(?,'personal.changed','personal','history',?)
 ON CONFLICT(audience,type,resource_kind,resource_id) DO UPDATE SET revision=excluded.revision`, "personal:"+profile, strconv.FormatInt(out.Revision, 10)); e != nil {
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
