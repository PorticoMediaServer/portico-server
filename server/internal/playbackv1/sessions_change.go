package playbackv1

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/supervise"
)

func (r row) stored() storedRequest {
	var out storedRequest
	_ = json.Unmarshal([]byte(r.request), &out)
	if out.Quality.Mode == "" {
		out.Quality.Mode = "original"
	}
	return out
}

func (r row) presentationView() Presentation {
	var p Presentation
	_ = json.Unmarshal([]byte(r.presentation), &p)
	if p.Subtitles == nil {
		p.Subtitles = []SubtitleFile{}
	}
	return p
}

// converting reports whether a presentation re-produces bytes (so a seek must
// restart production on the server, spec §17.4).
func (p Presentation) converting() bool {
	for _, d := range []*Decision{p.Decision.Video, p.Decision.Audio} {
		if d != nil && (d.Action == "transcode" || d.Action == "burn") {
			return true
		}
	}
	return false
}

// Patch is PATCH /v1/playback/sessions/{id} with If-Match (spec §5.7). A change
// that alters bytes starts a new presentation (a new generation, a new URL, at
// the current position); other changes keep the generation.
func (s *Service) Patch(ctx context.Context, c Caller, id, ifMatch string, change Change) (SessionView, error) {
	if err := validateTracks(change.Audio, change.Subtitles); err != nil {
		return SessionView{}, err
	}
	s.cancelPreparedFor(ctx, id) // §18.2: a change to the session cancels its prepared next
	switch change.State {
	case "", "playing", "paused":
	default:
		return SessionView{}, &FieldError{Path: "state"}
	}
	if change.Quality != nil {
		if err := change.Quality.Validate("quality"); err != nil {
			return SessionView{}, err
		}
	}
	if change.Seek != nil && (change.Seek.PositionMs < 0 || change.Seek.PositionMs > maxStartPositionMs) {
		return SessionView{}, &FieldError{Path: "seek.positionMs"}
	}
	r, err := s.session(ctx, c, id)
	if err != nil {
		return SessionView{}, err
	}
	if r.ended > 0 {
		return SessionView{}, ErrEnded
	}
	if ifMatch == "" {
		return SessionView{}, ErrRevisionRequired
	}
	if want, ok := ParseRevision(ifMatch); !ok || want != int64(r.revision) {
		return SessionView{}, &RevisionError{Current: s.view(r), Revision: int64(r.revision)}
	}
	if isChannel(r) {
		return s.patchChannel(ctx, c, r, change)
	}
	stored := r.stored()
	next := stored
	bytes := false
	if change.VersionID != "" && change.VersionID != r.version {
		next.VersionID, bytes = change.VersionID, true
	}
	if change.Audio != nil && (stored.Audio == nil || !reflect.DeepEqual(*change.Audio, *stored.Audio)) {
		next.Audio, bytes = change.Audio, true
	}
	if change.Quality != nil && *change.Quality != stored.Quality {
		next.Quality, bytes = *change.Quality, true
	}
	if change.PartIndex != nil && *change.PartIndex != r.partIndex {
		bytes = true
	}
	if change.Subtitles != nil {
		// Burn-in changes the bytes, but on the same playback session: the
		// subtitle selection re-plans it (below), rather than a new presentation.
		next.Subtitles = change.Subtitles
	}
	if change.SubtitleOffsetMs != nil {
		next.SubtitleOffsetMs = *change.SubtitleOffsetMs
	}
	presentation := r.presentationView()
	generation, media, mediaGeneration := r.generation, r.media, r.mediaGeneration
	position := r.position
	if change.Seek != nil {
		position = change.Seek.PositionMs
	}
	// A re-plan (quality, version, audio track or part change) re-admits like a
	// start, so the remote bitrate cap and the schedule and item policy apply
	// mid-session. It already holds its stream, so its own slot is not counted
	// again: a member at their stream limit can still change quality.
	if bytes {
		if err := c.admit(WithReplan(ctx, id, ""), r.item); err != nil {
			return SessionView{}, err
		}
	}
	switch {
	case bytes:
		version := next.VersionID
		if version == "" {
			version = r.version
		}
		op := "v1-" + r.id + "-" + strconv.Itoa(r.generation+1)
		part := changedPart(r, change, version)
		newMedia, view, err := s.present(ctx, c, presentationTarget{item: r.item, version: version, part: part, request: next, startMs: position,
			replacing: &playback.SessionReplacement{ID: r.media, Generation: r.mediaGeneration}, requestID: "v1." + r.id + "." + strconv.Itoa(r.generation+1), subtitleOp: op})
		if errors.Is(err, identity.ErrUnauthorized) {
			_ = s.Playback.StopV1(ctx, r.media)
			newMedia, view, err = s.present(ctx, c, presentationTarget{item: r.item, version: version, part: part, request: next, startMs: position, requestID: "v1." + r.id + "." + strconv.Itoa(r.generation+1) + ".r", subtitleOp: op + "-r"})
		}
		if err != nil {
			return SessionView{}, err
		}
		presentation, media, mediaGeneration, generation = view, newMedia.ID, newMedia.Generation, r.generation+1
		if plan, _ := s.Playback.SessionPlan(ctx, newMedia.ID); plan != nil {
			next.VersionID = plan.SourceID
		}
	case change.Subtitles != nil || change.SubtitleOffsetMs != nil:
		// A sidecar switch keeps the bytes and the generation.
		result, err := s.applySubtitles(ctx, c.Principal, r.item, r.media, next.Subtitles, next.SubtitleOffsetMs, position, "v1-"+r.id+"-r"+strconv.Itoa(r.revision+1))
		if err != nil {
			return SessionView{}, err
		}
		presentation.Subtitles = result.files
		if result.replaced != nil {
			// Burn-in on or off: new bytes from the position, a new generation.
			s.adoptReplaced(ctx, &presentation, r.media, result.replaced, playback.V1Choice{AdminMaxVideoBitrateBPS: c.AdminMaxVideoBitrateBPS, Remote: c.Remote})
			presentation.StartPositionMs = position
			mediaGeneration, generation = result.replaced.Generation, r.generation+1
		} else if change.Seek != nil && presentation.converting() {
			s.Playback.RestartConversion(ctx, r.media, float64(position)/1000)
			presentation.StartPositionMs = position
			generation = r.generation + 1
		}
	case change.Seek != nil && presentation.converting():
		// Production restarts at the position; the client reloads the same URL
		// there because the generation moved.
		s.Playback.RestartConversion(ctx, r.media, float64(position)/1000)
		presentation.StartPositionMs = position
		generation = r.generation + 1
	}
	state := r.state
	if change.State != "" && state != "preparing" {
		state = change.State
	}
	requestJSON, _ := json.Marshal(next)
	presentationJSON, _ := json.Marshal(presentation)
	now := s.now()
	version := next.VersionID
	if version == "" {
		version = r.version
	}
	part := changedPart(r, change, version)
	err = dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassEstablishedPlayback, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE playback_v1_sessions SET state=?,version_id=?,part_index=?,request=?,media_session_id=?,media_generation=?,presentation=?,generation=?,position_ms=?,start_position_ms=?,revision=revision+1,updated_ms=?,`+pausedSinceUpdate+` WHERE id=? AND revision=? AND ended_ms=0`,
			state, version, part, string(requestJSON), media, mediaGeneration, string(presentationJSON), generation, position, presentation.StartPositionMs, now.UnixMilli(), state, now.UnixMilli(), r.id, r.revision)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrRevision
		}
		r.state, r.version, r.partIndex, r.request, r.media, r.mediaGeneration = state, version, part, string(requestJSON), media, mediaGeneration
		r.presentation, r.generation, r.position, r.startPosition, r.revision, r.updated = string(presentationJSON), generation, position, presentation.StartPositionMs, r.revision+1, now.UnixMilli()
		return s.sessionEventsTx(ctx, tx, r, now, false)
	})
	if errors.Is(err, ErrRevision) {
		current, _ := loadRow(ctx, s.DB, id)
		return SessionView{}, &RevisionError{Current: s.view(current), Revision: int64(current.revision)}
	}
	if err != nil {
		return SessionView{}, err
	}
	s.hub.wake()
	return s.view(r), nil
}

// Stop is DELETE /v1/playback/sessions/{id} (spec §5.8): idempotent; a final
// position is recorded for resume before the grant is fenced.
func (s *Service) Stop(ctx context.Context, c Caller, id string, positionMs *int64) error {
	s.cancelPreparedFor(ctx, id)
	r, err := s.session(ctx, c, id)
	if err != nil {
		return err
	}
	if r.ended > 0 {
		return nil
	}
	if positionMs != nil && *positionMs >= 0 && r.media != "" {
		_ = s.Playback.ProgressContext(ctx, c.Principal, r.media, r.mediaGeneration, int64(r.lastSeq)+1, float64(*positionMs)/1000, "paused")
		_, _ = s.DB.ExecContext(ctx, `UPDATE playback_v1_sessions SET position_ms=?,last_seq=last_seq+1 WHERE id=? AND ended_ms=0`, *positionMs, r.id)
	}
	return s.End(ctx, id, "stopped", "")
}

// Report is one timeline report (spec §6, §17.6).
type Report struct {
	Seq             int64          `json:"seq"`
	Generation      int            `json:"generation"`
	State           string         `json:"state"`
	PositionMs      int64          `json:"positionMs"`
	PartIndex       *int           `json:"partIndex,omitempty"`
	Rate            float64        `json:"rate,omitempty"`
	BufferedMs      int64          `json:"bufferedMs,omitempty"`
	BandwidthKbps   int64          `json:"bandwidthKbps,omitempty"`
	DroppedFrames   int64          `json:"droppedFrames,omitempty"`
	Volume          *float64       `json:"volume,omitempty"`
	Muted           *bool          `json:"muted,omitempty"`
	AudioTrackID    string         `json:"audioTrackId,omitempty"`
	SubtitleTrackID *TrackRef      `json:"subtitleTrackId,omitempty"`
	Skipped         *SkipReport    `json:"skipped,omitempty"`
	Error           *ReportedError `json:"error,omitempty"`
}

// SkipReport is a marker the viewer skipped (spec §4.2): the segment marker's
// id, whether the skip was automatic (the viewer's preference) or manual, and
// the position the skip left from. It is evidence, never transport: the client
// has already seeked.
type SkipReport struct {
	MarkerID   string `json:"markerId"`
	Mode       string `json:"mode"`
	PositionMs int64  `json:"positionMs"`
}
type ReportedError struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// TimelineHooks run inside the report's transaction when an accepted report
// changes state (transfer commit, group readiness).
type timelineHook func(ctx context.Context, tx *sql.Tx, r row, report Report, now time.Time) error

// Timeline is POST /v1/playback/sessions/{id}/timeline (spec §6): progress,
// lease renewal and observation in one call. Reports for another generation or
// a lower seq renew the lease but change nothing (invariant 3). It returns the
// cadence for Report-Every-Ms.
func (s *Service) Timeline(ctx context.Context, c Caller, id string, report Report) (time.Duration, error) {
	switch report.State {
	case "playing", "paused", "buffering", "ended", "error":
	default:
		return 0, &FieldError{Path: "state"}
	}
	if report.Seq < 0 || report.PositionMs < 0 || report.PositionMs > maxStartPositionMs {
		return 0, &FieldError{Path: "seq"}
	}
	if k := report.Skipped; k != nil && (!markerIDPattern.MatchString(k.MarkerID) || k.Mode != "automatic" && k.Mode != "manual" || k.PositionMs < 0 || k.PositionMs > maxStartPositionMs) {
		return 0, &FieldError{Path: "skipped"}
	}
	r, err := s.session(ctx, c, id)
	if err != nil {
		return 0, err
	}
	if r.ended > 0 {
		return 0, ErrEnded
	}
	if r.device != c.DeviceID {
		// Only the playing device reports; a controller changes state with PATCH.
		return 0, ErrNotFound
	}
	if isChannel(r) {
		return s.timelineChannel(ctx, r, report)
	}
	now := s.now()
	lease := now.Add(s.lease()).UnixMilli()
	fresh := report.Generation == r.generation && report.Seq > r.lastSeq && r.state != "preparing"
	if !fresh {
		// Replays still renew the lease, but a repeated sequence must not turn
		// into a writer storm. Keep ample lease headroom and extend at most once
		// per half report interval (never more than a quarter of the lease).
		interval := max(time.Millisecond, min(s.reportEvery(r.state)/2, s.lease()/4))
		threshold := lease - interval.Milliseconds()
		if r.lease >= threshold {
			return s.reportEvery(r.state), nil
		}
		_, err = dbwork.ExecWrite(ctx, s.DB, dbwork.ClassEstablishedPlayback, `UPDATE playback_v1_sessions SET lease_expires_ms=? WHERE id=? AND ended_ms=0 AND lease_expires_ms<?`, lease, r.id, threshold)
		return s.reportEvery(r.state), err
	}
	state := r.state
	switch report.State {
	case "playing", "paused":
		state = report.State
	}
	audio, subtitle := r.audioTrack, r.subtitleTrack
	if report.AudioTrackID != "" {
		audio = report.AudioTrackID
	}
	if report.SubtitleTrackID != nil {
		subtitle = string(*report.SubtitleTrackID)
	}
	part := r.partIndex
	if report.PartIndex != nil && *report.PartIndex >= 0 {
		part = *report.PartIndex
	}
	legacy := "playing"
	switch report.State {
	case "paused":
		legacy = "paused"
	case "ended":
		legacy = "ended"
	}
	if r.media != "" {
		// Resume, played threshold, watch history (respecting pause-history) and
		// Continue Watching are written by the existing evidence path.
		if err = s.Playback.ProgressContext(ctx, c.Principal, r.media, r.mediaGeneration, report.Seq, float64(report.PositionMs)/1000, legacy); err != nil && !errors.Is(err, identity.ErrUnauthorized) {
			return 0, err
		}
	}
	changed := state != r.state
	err = dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassEstablishedPlayback, func(ctx context.Context, tx *sql.Tx) error {
		revision := r.revision
		if changed {
			revision++
		}
		res, err := tx.ExecContext(ctx, `UPDATE playback_v1_sessions SET state=?,position_ms=?,last_seq=?,part_index=?,audio_track_id=?,subtitle_track_id=?,bandwidth_kbps=?,lease_expires_ms=?,revision=?,updated_ms=?,`+pausedSinceUpdate+` WHERE id=? AND ended_ms=0 AND last_seq<? AND generation=?`,
			state, report.PositionMs, report.Seq, part, audio, subtitle, max(report.BandwidthKbps, 0), lease, revision, now.UnixMilli(), state, now.UnixMilli(), r.id, report.Seq, report.Generation)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		r.state, r.position, r.lastSeq, r.partIndex, r.audioTrack, r.subtitleTrack, r.revision, r.lease = state, report.PositionMs, report.Seq, part, audio, subtitle, revision, lease
		if changed {
			if err = s.sessionEventsTx(ctx, tx, r, now, false); err != nil {
				return err
			}
		}
		for _, hook := range s.timelineHooks {
			if err = hook(ctx, tx, r, report, now); err != nil {
				return err
			}
		}
		if report.State == "ended" {
			return s.endTx(ctx, tx, r, now, "ended", "")
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	s.hub.wake()
	if report.State == "ended" && r.media != "" {
		_ = s.Playback.StopV1(ctx, r.media)
	}
	if report.Skipped != nil && !isChannel(r) {
		s.recordSkip(ctx, r, *report.Skipped)
	}
	for _, after := range s.afterTimeline {
		after(ctx, r, report)
	}
	return s.reportEvery(state), nil
}

// EndReasonPausedTimeout is why a video session paused longer than the
// paused-session limit ended; PausedTimeoutMessage is what its viewer is told.
const (
	EndReasonPausedTimeout = "paused_timeout"
	PausedTimeoutMessage   = "Playback stopped after a long pause."
)

// sweeper ends sessions whose lease lapsed (spec §5.8, invariant 4). It runs
// only while sessions exist: it sleeps until the earliest lease, is kicked
// when a session starts, and exits when none remain. Nothing polls idly.
type sweeper struct {
	mu      sync.Mutex
	running bool
	kickCh  chan struct{}
	// pausedAt is when the paused-too-long sweep last ran (at most once a
	// minute, so the common case costs one indexed read per minute).
	pausedAt time.Time
}

func (w *sweeper) kick(s *Service) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.kickCh == nil {
		w.kickCh = make(chan struct{}, 1)
	}
	if w.running {
		select {
		case w.kickCh <- struct{}{}:
		default:
		}
		return
	}
	w.running = true
	supervise.Supervise(context.Background(), "playbackv1.lease-sweeper", func(context.Context) { w.run(s) })
}

func (w *sweeper) run(s *Service) {
	ctx := context.Background()
	for {
		var next sql.NullInt64
		if err := s.DB.QueryRowContext(ctx, `SELECT MIN(lease_expires_ms) FROM playback_v1_sessions WHERE ended_ms=0`).Scan(&next); err != nil || !next.Valid {
			w.mu.Lock()
			// Re-check under the lock so a start that raced the exit is swept.
			var again sql.NullInt64
			if err == nil {
				_ = s.DB.QueryRowContext(ctx, `SELECT MIN(lease_expires_ms) FROM playback_v1_sessions WHERE ended_ms=0`).Scan(&again)
			}
			if err != nil || !again.Valid {
				w.running = false
				w.mu.Unlock()
				return
			}
			w.mu.Unlock()
			continue
		}
		wait := time.Until(time.UnixMilli(next.Int64))
		if s.Now != nil {
			wait = time.UnixMilli(next.Int64).Sub(s.now())
		}
		if wait > 0 {
			timer := time.NewTimer(min(wait, time.Hour))
			select {
			case <-timer.C:
			case <-w.kickCh:
				timer.Stop()
				continue
			}
		}
		s.ExpireLapsed(ctx)
		s.sweepPausedIfDue(ctx)
	}
}

// sweepPausedIfDue ends sessions paused past the limit, at most once a minute.
func (s *Service) sweepPausedIfDue(ctx context.Context) {
	s.sweep.mu.Lock()
	if s.now().Sub(s.sweep.pausedAt) < time.Minute {
		s.sweep.mu.Unlock()
		return
	}
	s.sweep.pausedAt = s.now()
	s.sweep.mu.Unlock()
	s.EndPausedTooLong(ctx)
}

// ExpireLapsed ends every session whose lease has lapsed.
func (s *Service) ExpireLapsed(ctx context.Context) int {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM playback_v1_sessions WHERE ended_ms=0 AND lease_expires_ms<=? ORDER BY lease_expires_ms LIMIT 256`, s.now().UnixMilli())
	if err != nil {
		return 0
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		_ = s.End(ctx, id, "lease_expired", "")
	}
	return len(ids)
}

// EndPausedTooLong ends video sessions paused longer than PausedLimit (Plex
// "Terminate Sessions Paused for Longer Than"). Listening sessions (kind
// "audio": songs, tracks, audiobooks) and Live/Library channels (kinds "live"
// and "channel", see isChannel) are never ended. Each ends through End like an
// administrator terminate, so its slot and transcode free through the normal
// end path. At most 64 sessions per call; the sweep loop calls it at most once
// a minute, so the idle cost is one indexed read per minute.
func (s *Service) EndPausedTooLong(ctx context.Context) int {
	var limit time.Duration
	if s.PausedLimit != nil {
		limit = s.PausedLimit(ctx)
	}
	if limit <= 0 {
		return 0
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM playback_v1_sessions WHERE ended_ms=0 AND state='paused' AND paused_since_ms>0 AND paused_since_ms<=? AND kind NOT IN ('audio','live','channel') ORDER BY paused_since_ms LIMIT 64`, s.now().Add(-limit).UnixMilli())
	if err != nil {
		return 0
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		_ = s.End(ctx, id, EndReasonPausedTimeout, PausedTimeoutMessage)
	}
	return len(ids)
}

// changedPart is the part a change plays: the one it names, else the current
// one, except that another version starts at its first part.
func changedPart(r row, change Change, version string) int {
	switch {
	case change.PartIndex != nil:
		return *change.PartIndex
	case version != r.version:
		return 0
	}
	return r.partIndex
}
