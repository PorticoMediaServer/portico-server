package playbackv1

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/subtitles"
)

// Spec defaults (§5.8, §6).
const (
	DefaultLease       = 120 * time.Second
	ReportPlaying      = 10 * time.Second
	ReportPaused       = 30 * time.Second
	DefaultReadyWait   = 1500 * time.Millisecond
	PreparingRetryMs   = 500
	maxStartPositionMs = 1 << 40
)

// TrackRef is a track id in a request where JSON null means "off"
// (`subtitleTrackId: null`). Off is stored as TrackOff.
type TrackRef string

const TrackOff TrackRef = "-"

func (t *TrackRef) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*t = TrackOff
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*t = TrackRef(s)
	return nil
}

// QueueRef ties a session to its queue entry.
type QueueRef struct {
	QueueID string `json:"queueId"`
	EntryID string `json:"entryId"`
}

// AudioRequest chooses an audio track (spec §5.4).
type AudioRequest struct {
	TrackID  string `json:"trackId"`
	Channels int    `json:"channels,omitempty"`
}

// SubtitleRequest chooses subtitles (spec §5.5); a null trackId turns them off.
type SubtitleRequest struct {
	TrackID  TrackRef `json:"trackId"`
	Delivery string   `json:"delivery,omitempty"`
}

// StartRequest is POST /v1/playback/sessions (spec §5.1).
type StartRequest struct {
	ItemID    string `json:"itemId,omitempty"`
	ChannelID string `json:"channelId,omitempty"`
	// ChannelGeneration is the guide generation the viewer tuned from (a fence
	// against a lineup that changed since); absent: the current one.
	ChannelGeneration string           `json:"channelGeneration,omitempty"`
	Queue             *QueueRef        `json:"queue,omitempty"`
	TransferID        string           `json:"transferId,omitempty"`
	VersionID         string           `json:"versionId,omitempty"`
	PartIndex         int              `json:"partIndex,omitempty"`
	StartPositionMs   *int64           `json:"startPositionMs,omitempty"`
	StartFrom         string           `json:"startFrom,omitempty"`
	State             string           `json:"state,omitempty"`
	Quality           *Quality         `json:"quality,omitempty"`
	Audio             *AudioRequest    `json:"audio,omitempty"`
	Subtitles         *SubtitleRequest `json:"subtitles,omitempty"`
	Network           string           `json:"network,omitempty"`
	ReplacesSessionID string           `json:"replacesSessionId,omitempty"`
}

// Change is PATCH /v1/playback/sessions/{id} (spec §5.7).
type Change struct {
	State            string           `json:"state,omitempty"`
	VersionID        string           `json:"versionId,omitempty"`
	PartIndex        *int             `json:"partIndex,omitempty"`
	Audio            *AudioRequest    `json:"audio,omitempty"`
	Subtitles        *SubtitleRequest `json:"subtitles,omitempty"`
	Quality          *Quality         `json:"quality,omitempty"`
	Seek             *SeekRequest     `json:"seek,omitempty"`
	SubtitleOffsetMs *int64           `json:"subtitleOffsetMs,omitempty"`
}
type SeekRequest struct {
	PositionMs int64  `json:"positionMs"`
	ID         string `json:"id"`
	// Live (channels, §18.6): to the live edge; positionMs is ignored.
	Live bool `json:"live,omitempty"`
}

// StopRequest is the optional DELETE body (spec §5.8).
type StopRequest struct {
	PositionMs *int64 `json:"positionMs,omitempty"`
}

// SessionView is the session resource (spec §5.1).
type SessionView struct {
	ID           string       `json:"id"`
	Revision     string       `json:"revision"`
	Kind         string       `json:"kind"`
	Role         string       `json:"role"`
	State        string       `json:"state"`
	ItemID       string       `json:"itemId,omitempty"`
	VersionID    string       `json:"versionId,omitempty"`
	ChannelID    string       `json:"channelId,omitempty"`
	Queue        *QueueRef    `json:"queue,omitempty"`
	Lease        Lease        `json:"lease"`
	Presentation Presentation `json:"presentation"`
	// RetryAfterMs is set only on a 202 while the presentation is prepared; the
	// client replays the same start (spec §17.2).
	RetryAfterMs int `json:"retryAfterMs,omitempty"`
	// End says why an ended session ended, so a device that missed the
	// session.updated event learns it from a re-read: "terminated" (an
	// administrator, with their message), "paused_timeout" (paused longer than
	// the paused-session limit, with its message), "stopped", "transferred",
	// "lease_expired", "replaced" and so on.
	End *SessionEnd `json:"end,omitempty"`
}

// SessionEnd is why a session ended and what the viewer is told.
type SessionEnd struct {
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}
type Lease struct {
	ExpiresAt     string `json:"expiresAt"`
	ReportEveryMs int    `json:"reportEveryMs"`
}
type Presentation struct {
	Generation      int            `json:"generation"`
	Mode            string         `json:"mode"`
	URL             string         `json:"url"`
	StartPositionMs int64          `json:"startPositionMs"`
	Subtitles       []SubtitleFile `json:"subtitles"`
	Decision        Decisions      `json:"decision"`
	BitrateKbps     int            `json:"bitrateKbps,omitempty"`
	// Container is what the URL delivers: the file's own container for direct
	// play ("mp4", "mkv", "flac"…), or "fmp4_hls" / "mpegts_hls" for a stream. A
	// player that must be told the segment format up front (Cast) reads it here.
	Container string `json:"container,omitempty"`
	// AudioRender is an audio presentation's plan (spec §18.1, version 2): how the
	// client fetches and decodes it, with the exact gapless trim and the gains.
	AudioRender *AudioRender `json:"audioRender,omitempty"`
	// Linear is a channel presentation's linear clock (spec §11, §18.6).
	Linear *LinearView `json:"linear,omitempty"`
}
type SubtitleFile struct {
	TrackID string `json:"trackId"`
	Format  string `json:"format"`
	URL     string `json:"url"`
}

// Caller is who is asking: the admitted principal, its device, and admission's
// clamps (spec §5.3).
type Caller struct {
	Principal identity.Principal
	DeviceID  string
	// AdminMaxVideoBitrateBPS is the member's remote cap when this request is remote.
	AdminMaxVideoBitrateBPS int
	Remote                  bool
	// Admit applies media admission (library access, restrictions, member
	// limits, schedule; ARCH-MEDIA-21) to an item before a new presentation
	// takes a slot, returning the remote bitrate cap. Replays never re-admit.
	Admit func(ctx context.Context, item string) (int, error)
	// AdmitChannel applies the live channel policy (allow/deny list and
	// schedule) to a channel start. Replays never re-admit.
	AdmitChannel func(ctx context.Context, channel string) error
}

type replanKey struct{}

// Replan names what an admission continues: the v1 session it re-plans or
// replaces, and a prepared entry's media session that already exists. Neither
// is a new stream, so the stream limit doesn't count them.
type Replan struct{ Session, Media string }

// WithReplan marks an admission as continuing an existing stream.
func WithReplan(ctx context.Context, session, media string) context.Context {
	return context.WithValue(ctx, replanKey{}, Replan{Session: session, Media: media})
}

// ReplanOf is what an admission continues (zero for a new stream).
func ReplanOf(ctx context.Context) Replan {
	r, _ := ctx.Value(replanKey{}).(Replan)
	return r
}

func (c *Caller) admit(ctx context.Context, item string) error {
	if c.Admit == nil {
		return nil
	}
	limit, err := c.Admit(ctx, item)
	if err != nil {
		return err
	}
	if c.Remote {
		c.AdminMaxVideoBitrateBPS = limit
	}
	return nil
}

func (c *Caller) admitChannel(ctx context.Context, channel string) error {
	if c.AdmitChannel == nil {
		return nil
	}
	return c.AdmitChannel(ctx, channel)
}

// storedRequest is what a session keeps of its request for re-planning.
type storedRequest struct {
	VersionID        string           `json:"versionId,omitempty"`
	Quality          Quality          `json:"quality"`
	Audio            *AudioRequest    `json:"audio,omitempty"`
	Subtitles        *SubtitleRequest `json:"subtitles,omitempty"`
	SubtitleOffsetMs int64            `json:"subtitleOffsetMs,omitempty"`
	Network          string           `json:"network,omitempty"`
}

// row is one playback_v1_sessions row.
type row struct {
	id, device, account, profile, authority, startKey, startDigest string
	kind, role, state, item, version, channel, queueID, entryID    string
	transfer, group, request, media, presentation, audioTrack      string
	subtitleTrack, location, endReason, message                    string
	partIndex, revision, generation, mediaGeneration               int
	startPosition, position, lastSeq, bandwidth, lease, created    int64
	updated, ended                                                 int64
	// pausedSince is when the session entered paused, in epoch ms (0 when it
	// isn't paused). Every state write maintains it in SQL (pausedSinceUpdate).
	pausedSince int64
}

const rowColumns = `id,device_id,account_id,profile_id,authority,start_key,start_digest,kind,role,state,item_id,version_id,channel_id,queue_id,entry_id,transfer_id,group_id,request,media_session_id,presentation,audio_track_id,subtitle_track_id,location,end_reason,message,part_index,revision,generation,media_generation,start_position_ms,position_ms,last_seq,bandwidth_kbps,lease_expires_ms,created_ms,updated_ms,ended_ms,paused_since_ms`

// item_id is the catalogue's INTEGER entity id; the row carries the public id.
// rowSelect reads it back as one (empty once the item is gone), and rowValues
// resolves it on insert (0 for a channel session, which has none).
const rowSelect = `id,device_id,account_id,profile_id,authority,start_key,start_digest,kind,role,state,COALESCE((SELECT pid(item.public_id) FROM catalog_entities item WHERE item.id=playback_v1_sessions.item_id),''),version_id,channel_id,queue_id,entry_id,transfer_id,group_id,request,media_session_id,presentation,audio_track_id,subtitle_track_id,location,end_reason,message,part_index,revision,generation,media_generation,start_position_ms,position_ms,last_seq,bandwidth_kbps,lease_expires_ms,created_ms,updated_ms,ended_ms,paused_since_ms`

const rowValues = `VALUES(?,?,?,?,?,?,?,?,?,?,COALESCE((SELECT id FROM catalog_entities WHERE public_id=pid_blob(NULLIF(?,''))),0),?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

// pausedSinceUpdate keeps paused_since_ms with the state in the same UPDATE:
// entering paused stamps now, staying paused keeps the old stamp, any other
// state clears it to 0. Its two arguments are the new state and now in epoch
// ms (the unqualified columns read the row's old values).
const pausedSinceUpdate = `paused_since_ms=CASE WHEN ?='paused' THEN CASE WHEN state='paused' AND paused_since_ms>0 THEN paused_since_ms ELSE ? END ELSE 0 END`

type scanner interface{ Scan(...any) error }

func scanRow(s scanner) (row, error) {
	var r row
	err := s.Scan(&r.id, &r.device, &r.account, &r.profile, &r.authority, &r.startKey, &r.startDigest, &r.kind, &r.role, &r.state, &r.item, &r.version, &r.channel, &r.queueID, &r.entryID, &r.transfer, &r.group, &r.request, &r.media, &r.presentation, &r.audioTrack, &r.subtitleTrack, &r.location, &r.endReason, &r.message, &r.partIndex, &r.revision, &r.generation, &r.mediaGeneration, &r.startPosition, &r.position, &r.lastSeq, &r.bandwidth, &r.lease, &r.created, &r.updated, &r.ended, &r.pausedSince)
	return r, err
}

type querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadRow(ctx context.Context, q querier, id string) (row, error) {
	r, err := scanRow(q.QueryRowContext(ctx, `SELECT `+rowSelect+` FROM playback_v1_sessions WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Service) lease() time.Duration {
	if s.LeaseDuration > 0 {
		return s.LeaseDuration
	}
	return DefaultLease
}

func (s *Service) reportEvery(state string) time.Duration {
	if s.ReportEvery > 0 {
		return s.ReportEvery
	}
	if state == "paused" {
		return ReportPaused
	}
	return ReportPlaying
}

func (s *Service) view(r row) SessionView {
	v := SessionView{ID: r.id, Revision: strconv.Itoa(r.revision), Kind: r.kind, Role: r.role, State: r.state, ItemID: r.item, VersionID: r.version, ChannelID: r.channel}
	if r.ended > 0 {
		v.State = "ended"
		v.End = &SessionEnd{Reason: r.endReason, Message: r.message}
		if v.End.Reason == "" {
			v.End.Reason = "stopped"
		}
	}
	if v.State == "preparing" {
		v.State = "playing"
		var req struct {
			State string `json:"state"`
		}
		_ = json.Unmarshal([]byte(r.request), &req)
		if req.State == "paused" {
			v.State = "paused"
		}
	}
	if r.queueID != "" {
		v.Queue = &QueueRef{QueueID: r.queueID, EntryID: r.entryID}
	}
	v.Lease = Lease{ExpiresAt: time.UnixMilli(r.lease).UTC().Format(time.RFC3339Nano), ReportEveryMs: int(s.reportEvery(v.State) / time.Millisecond)}
	_ = json.Unmarshal([]byte(r.presentation), &v.Presentation)
	if v.Presentation.Subtitles == nil {
		v.Presentation.Subtitles = []SubtitleFile{}
	}
	v.Presentation.Generation = r.generation
	if isChannel(r) {
		v.Presentation = s.channelPresentation(r)
	}
	return v
}

// owns reports whether the caller may see and control a session: its own
// device, or another device signed in to the same profile (spec §9.1).
func owns(c Caller, r row) bool {
	return r.device == c.DeviceID || r.account == c.Principal.AccountID && r.profile == c.Principal.ProfileID && r.authority == c.Principal.Authority
}

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

func newID(prefix string) string {
	var raw [15]byte
	_, _ = rand.Read(raw[:])
	return prefix + base64.RawURLEncoding.EncodeToString(raw[:])
}

func digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// validate checks a start request's shape.
func (r StartRequest) validate() error {
	targets := 0
	for _, set := range []bool{r.ItemID != "", r.ChannelID != "", r.Queue != nil, r.TransferID != ""} {
		if set {
			targets++
		}
	}
	if targets != 1 {
		return &FieldError{Path: "itemId"}
	}
	if r.StartPositionMs != nil && (*r.StartPositionMs < 0 || *r.StartPositionMs > maxStartPositionMs) || r.StartPositionMs != nil && r.StartFrom != "" {
		return &FieldError{Path: "startPositionMs"}
	}
	switch r.StartFrom {
	case "", "resume", "beginning", "live", "startOver":
	default:
		return &FieldError{Path: "startFrom"}
	}
	switch r.State {
	case "", "playing", "paused":
	default:
		return &FieldError{Path: "state"}
	}
	switch r.Network {
	case "", "local", "remote", "cellular":
	default:
		return &FieldError{Path: "network"}
	}
	if r.PartIndex < 0 || r.PartIndex > 1000 {
		return &FieldError{Path: "partIndex"}
	}
	if r.Quality != nil {
		if err := r.Quality.Validate("quality"); err != nil {
			return err
		}
	}
	return validateTracks(r.Audio, r.Subtitles)
}

func validateTracks(a *AudioRequest, sub *SubtitleRequest) error {
	if a != nil && (a.TrackID == "" || len(a.TrackID) > 128 || a.Channels < 0 || a.Channels > 16) {
		return &FieldError{Path: "audio"}
	}
	if sub != nil {
		switch sub.Delivery {
		case "", "auto", "sidecar", "embeddedClient", "burn":
		default:
			return &FieldError{Path: "subtitles.delivery"}
		}
		if len(sub.TrackID) > 128 {
			return &FieldError{Path: "subtitles.trackId"}
		}
	}
	return nil
}

// presentationTarget is one media start: what to play and how.
type presentationTarget struct {
	item, version string
	// part is the version's part to play (its part index; 0 is the first).
	part      int
	request   storedRequest
	startMs   int64
	replacing *playback.SessionReplacement
	requestID string
	// subtitleOp names the subtitle selection (idempotent per presentation).
	subtitleOp string
	// privateFor makes a private presentation (§18.2) of the given media
	// session's prepared next item, until privateUntilMs.
	privateFor     string
	privateUntilMs int64
}

// present creates a presentation (a playback_sessions row with its own grant)
// and returns it with its view. It does not wait for production.
func (s *Service) present(ctx context.Context, c Caller, t presentationTarget) (playback.Session, Presentation, error) {
	if s.Playback == nil {
		return playback.Session{}, Presentation{}, playback.ErrIncompatible
	}
	asset, err := s.partAsset(ctx, t.item, t.version, t.part)
	if err != nil {
		return playback.Session{}, Presentation{}, err
	}
	choice := playback.V1Choice{AssetID: asset, AdminMaxVideoBitrateBPS: c.AdminMaxVideoBitrateBPS, Remote: c.Remote, PrivateFor: t.privateFor, PrivateUntilMs: t.privateUntilMs}
	if err = s.audioChoice(ctx, c, t.item, &choice); err != nil {
		return playback.Session{}, Presentation{}, err
	}
	if t.request.Quality.Mode == "limit" {
		choice.Limit = true
		choice.MaxVideoBitrateBPS = t.request.Quality.MaxVideoBitrateKbps * 1000
		choice.MaxAudioBitrateBPS = t.request.Quality.MaxAudioBitrateKbps * 1000
		choice.MaxVideoHeight = t.request.Quality.MaxHeight
	}
	if a := t.request.Audio; a != nil {
		index, ok := ParseStreamID("a", a.TrackID)
		if !ok {
			return playback.Session{}, Presentation{}, &FieldError{Path: "audio.trackId"}
		}
		choice.AudioStream = &index
	}
	media, err := s.Playback.CreateV1(ctx, c.Principal, t.item, t.requestID, choice, t.replacing)
	if errors.Is(err, sql.ErrNoRows) && t.version != "" {
		return media, Presentation{}, &FieldError{Path: "versionId"}
	}
	if err != nil {
		return media, Presentation{}, err
	}
	view := Presentation{Mode: "stream", URL: media.StreamURL, StartPositionMs: t.startMs, Subtitles: []SubtitleFile{}}
	if media.Mode == "direct" {
		view.Mode = "direct"
	}
	// §18.1: an audio presentation's render plan, pinned to this presentation.
	view.AudioRender = audioRenderOf(media.AudioPlan)
	plan, err := s.Playback.SessionPlan(ctx, media.ID)
	if err != nil {
		return media, Presentation{}, err
	}
	if plan != nil {
		view.Decision = decisions(*plan, choice, plan.SourceVideoCodec != "")
		view.BitrateKbps = (plan.VideoBitrateBPS + plan.AudioBitrateBPS) / 1000
		view.Container = plan.OutputContainer
	}
	if a := t.request.Audio; a != nil && plan != nil && plan.AudioStream >= 0 && streamID("a", plan.AudioStream) != a.TrackID {
		// The chosen track doesn't exist in this version.
		_ = s.Playback.StopV1(ctx, media.ID)
		return media, Presentation{}, &FieldError{Path: "audio.trackId"}
	}
	result, err := s.applySubtitles(ctx, c.Principal, t.item, media.ID, t.request.Subtitles, t.request.SubtitleOffsetMs, t.startMs, t.subtitleOp)
	if err != nil {
		_ = s.Playback.StopV1(ctx, media.ID)
		return media, Presentation{}, err
	}
	view.Subtitles = result.files
	if result.replaced != nil {
		// Burn-in: the same playback session, re-planned with the subtitle in the
		// video, under a new grant and generation.
		media.StreamURL, media.Generation = result.replaced.StreamURL, result.replaced.Generation
		s.adoptReplaced(ctx, &view, media.ID, result.replaced, choice)
	}
	return media, view, nil
}

// adoptReplaced points a presentation view at a stream a subtitle selection
// replaced (burn-in on or off): its URL, mode and decision.
func (s *Service) adoptReplaced(ctx context.Context, view *Presentation, media string, replaced *subtitles.Presentation, choice playback.V1Choice) {
	view.URL = replaced.StreamURL
	view.Mode = "stream"
	if replaced.Mode == "direct" {
		view.Mode = "direct"
	}
	if plan, err := s.Playback.SessionPlan(ctx, media); err == nil && plan != nil {
		view.Decision = decisions(*plan, choice, plan.SourceVideoCodec != "")
		view.BitrateKbps = (plan.VideoBitrateBPS + plan.AudioBitrateBPS) / 1000
		view.Container = plan.OutputContainer
	}
}

func (s *Service) readyWait() time.Duration {
	if s.ReadyWait > 0 {
		return s.ReadyWait
	}
	return DefaultReadyWait
}

// Start is POST /v1/playback/sessions. It returns the session and whether it is
// still being prepared (202). A replay with the same key returns the same
// session; the same key with another body is ErrIdempotencyMismatch (422
// idempotency_key_reused, the one API-wide code; spec §17.1).
func (s *Service) Start(ctx context.Context, c Caller, key string, req StartRequest) (SessionView, bool, error) {
	if key == "" {
		return SessionView{}, false, ErrKeyRequired
	}
	if !keyPattern.MatchString(key) {
		return SessionView{}, false, &FieldError{Path: "Idempotency-Key"}
	}
	if err := req.validate(); err != nil {
		return SessionView{}, false, err
	}
	want := digest(req)
	existing, err := scanRow(s.DB.QueryRowContext(ctx, `SELECT `+rowSelect+` FROM playback_v1_sessions WHERE device_id=? AND start_key=?`, c.DeviceID, key))
	if err == nil {
		if existing.startDigest != want {
			return SessionView{}, false, ErrIdempotencyMismatch
		}
		return s.replayStart(ctx, c, existing)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return SessionView{}, false, err
	}
	switch {
	case req.ItemID != "":
		return s.startItem(ctx, c, key, want, req)
	default:
		return s.startOther(ctx, c, key, want, req)
	}
}

// startOther is filled in by later slices (channels, queues, transfers).
var startTargets = map[string]func(*Service, context.Context, Caller, string, string, StartRequest) (SessionView, bool, error){}

func (s *Service) startOther(ctx context.Context, c Caller, key, digest string, req StartRequest) (SessionView, bool, error) {
	kind := "channelId"
	switch {
	case req.Queue != nil:
		kind = "queue"
	case req.TransferID != "":
		kind = "transferId"
	}
	if start, ok := startTargets[kind]; ok {
		return start(s, ctx, c, key, digest, req)
	}
	return SessionView{}, false, &FieldError{Path: kind}
}

// replayStart answers a repeated start: 202 until the presentation is ready.
func (s *Service) replayStart(ctx context.Context, c Caller, r row) (SessionView, bool, error) {
	r = s.expireIfLapsed(ctx, r)
	if r.state != "preparing" || r.ended > 0 {
		return s.view(r), false, nil
	}
	ready, err := s.mediaReady(ctx, r)
	if err != nil {
		return SessionView{}, false, err
	}
	if !ready {
		v := s.view(r)
		v.RetryAfterMs = PreparingRetryMs
		return v, true, nil
	}
	return s.markReady(ctx, r)
}

func (s *Service) mediaReady(ctx context.Context, r row) (bool, error) {
	p := r.presentationView()
	mode := "hls"
	if p.Mode == "direct" {
		mode = "direct"
	}
	return s.ready(ctx, playback.Session{ID: r.media, Mode: mode, StreamURL: p.URL})
}

func (s *Service) ready(ctx context.Context, media playback.Session) (bool, error) {
	if s.ReadyCheck != nil {
		return s.ReadyCheck(ctx, media)
	}
	return s.Playback.ReadyWithin(ctx, media, s.readyWait())
}

// markReady moves a prepared session to its requested state.
func (s *Service) markReady(ctx context.Context, r row) (SessionView, bool, error) {
	var req struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal([]byte(r.request), &req)
	state := "playing"
	if req.State == "paused" {
		state = "paused"
	}
	now := s.now()
	err := dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassPlaybackStart, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE playback_v1_sessions SET state=?,revision=revision+1,updated_ms=?,`+pausedSinceUpdate+` WHERE id=? AND state='preparing' AND ended_ms=0`, state, now.UnixMilli(), state, now.UnixMilli(), r.id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			r.state, r.revision = state, r.revision+1
			return s.sessionEventsTx(ctx, tx, r, now, false)
		}
		return nil
	})
	if err != nil {
		return SessionView{}, false, err
	}
	s.hub.wake()
	current, err := loadRow(ctx, s.DB, r.id)
	return s.view(current), false, err
}

func (s *Service) startItem(ctx context.Context, c Caller, key, startDigest string, req StartRequest) (SessionView, bool, error) {
	kind, err := readItemKind(ctx, s.DB, req.ItemID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionView{}, false, ErrNotFound
		}
		return SessionView{}, false, err
	}
	sessionKind := "vod"
	if kind == "song" || kind == "audiobook_file" || kind == "track" {
		sessionKind = "audio"
	}
	stored := storedRequest{VersionID: req.VersionID, Quality: Quality{Mode: "original"}, Audio: req.Audio, Subtitles: req.Subtitles, Network: req.Network}
	if req.Quality != nil {
		stored.Quality = *req.Quality
	}
	return s.startPresentation(ctx, c, key, startDigest, req, row{kind: sessionKind, role: "local", item: req.ItemID, version: req.VersionID, partIndex: req.PartIndex}, stored)
}

// startPresentation creates the first presentation of a new session and
// records it. base carries the kind, role, target and links.
func (s *Service) startPresentation(ctx context.Context, c Caller, key, startDigest string, req StartRequest, base row, stored storedRequest) (SessionView, bool, error) {
	id := newID("ps_")
	var replacing *playback.SessionReplacement
	var old row
	if req.ReplacesSessionID != "" {
		var err error
		old, err = loadRow(ctx, s.DB, req.ReplacesSessionID)
		if err != nil || old.device != c.DeviceID {
			return SessionView{}, false, &FieldError{Path: "replacesSessionId"}
		}
		if old.ended == 0 && old.media != "" {
			replacing = &playback.SessionReplacement{ID: old.media, Generation: old.mediaGeneration}
		}
	}
	admitCtx := ctx
	if req.ReplacesSessionID != "" && old.ended == 0 {
		admitCtx = WithReplan(ctx, old.id, "")
	}
	// Admitted and made countable under one gate per account (admission_gate.go).
	release := s.admission.hold(c.Principal.AccountID)
	defer release()
	if err := c.admit(admitCtx, base.item); err != nil {
		return SessionView{}, false, err
	}
	startMs := int64(0)
	if req.StartPositionMs != nil {
		startMs = *req.StartPositionMs
	}
	target := presentationTarget{item: base.item, version: base.version, part: base.partIndex, request: stored, startMs: startMs, replacing: replacing, requestID: "v1." + id + ".1", subtitleOp: "v1-" + id + "-1"}
	media, view, err := s.present(ctx, c, target)
	if errors.Is(err, identity.ErrUnauthorized) && replacing != nil {
		// The session being replaced belongs to an earlier sign-in of this device:
		// end it, then start without taking its slot.
		_ = s.Playback.StopV1(ctx, old.media)
		target.replacing = nil
		media, view, err = s.present(ctx, c, target)
	}
	if err != nil {
		return SessionView{}, false, err
	}
	if req.StartPositionMs == nil && (req.StartFrom == "" || req.StartFrom == "resume") {
		view.StartPositionMs = int64(media.ResumeSeconds * 1000)
	}
	ready, err := s.ready(ctx, media)
	if err != nil {
		_ = s.Playback.StopV1(ctx, media.ID)
		return SessionView{}, false, err
	}
	state := "playing"
	if req.State == "paused" {
		state = "paused"
	}
	if !ready {
		state = "preparing"
	}
	requestJSON, _ := json.Marshal(struct {
		storedRequest
		State string `json:"state,omitempty"`
	}{stored, req.State})
	presentationJSON, _ := json.Marshal(view)
	now := s.now()
	r := base
	r.id, r.device, r.account, r.profile, r.authority = id, c.DeviceID, c.Principal.AccountID, c.Principal.ProfileID, c.Principal.Authority
	r.startKey, r.startDigest, r.state, r.request, r.revision, r.generation = key, startDigest, state, string(requestJSON), 1, 1
	r.media, r.mediaGeneration, r.presentation, r.startPosition, r.position = media.ID, media.Generation, string(presentationJSON), view.StartPositionMs, view.StartPositionMs
	r.lease, r.created, r.updated = now.Add(s.lease()).UnixMilli(), now.UnixMilli(), now.UnixMilli()
	if state == "paused" {
		r.pausedSince = now.UnixMilli()
	}
	r.location = "local"
	if c.Remote {
		r.location = "remote"
	}
	if r.version == "" {
		if plan, _ := s.Playback.SessionPlan(ctx, media.ID); plan != nil {
			r.version = plan.SourceID
		}
	}
	err = dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassPlaybackStart, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO playback_v1_sessions(`+rowColumns+`) `+rowValues,
			r.id, r.device, r.account, r.profile, r.authority, r.startKey, r.startDigest, r.kind, r.role, r.state, r.item, r.version, r.channel, r.queueID, r.entryID, r.transfer, r.group, r.request, r.media, r.presentation, r.audioTrack, r.subtitleTrack, r.location, r.endReason, r.message, r.partIndex, r.revision, r.generation, r.mediaGeneration, r.startPosition, r.position, r.lastSeq, r.bandwidth, r.lease, r.created, r.updated, r.ended, r.pausedSince); err != nil {
			return err
		}
		if old.id != "" && old.ended == 0 {
			if err := s.endTx(ctx, tx, old, now, "replaced", ""); err != nil {
				return err
			}
		}
		return s.sessionEventsTx(ctx, tx, r, now, true)
	})
	if err != nil {
		_ = s.Playback.StopV1(ctx, media.ID)
		// A concurrent start with the same key won: replay it.
		if existing, e := scanRow(s.DB.QueryRowContext(ctx, `SELECT `+rowSelect+` FROM playback_v1_sessions WHERE device_id=? AND start_key=?`, c.DeviceID, key)); e == nil {
			if existing.startDigest != startDigest {
				return SessionView{}, false, ErrIdempotencyMismatch
			}
			return s.replayStart(ctx, c, existing)
		}
		return SessionView{}, false, err
	}
	s.hub.wake()
	s.sweep.kick(s)
	v := s.view(r)
	if state == "preparing" {
		v.RetryAfterMs = PreparingRetryMs
	}
	return v, state == "preparing", nil
}

// sessionEventsTx publishes a session change: session.updated to its device and
// to the profile's other devices, admin.sessions to administrators ("the list
// changed; refetch", §17.14).
func (s *Service) sessionEventsTx(ctx context.Context, tx *sql.Tx, r row, now time.Time, listChanged bool) error {
	resource := &EventResource{Kind: "session", ID: r.id}
	state := r.state
	if r.ended > 0 {
		state = "ended"
	}
	data := map[string]any{"state": state, "deviceId": r.device, "generation": r.generation}
	if r.message != "" {
		data["message"] = r.message
	}
	if r.endReason != "" {
		data["reason"] = r.endReason
	}
	rev := strconv.Itoa(r.revision)
	if err := publishTx(ctx, tx, now, DeviceAudience(r.device), "session.updated", resource, rev, data); err != nil {
		return err
	}
	if err := publishTx(ctx, tx, now, ProfileAudience(r.authority, r.account, r.profile), "session.updated", resource, rev, data); err != nil {
		return err
	}
	if listChanged {
		return publishTx(ctx, tx, now, AdminAudience, "admin.sessions", nil, "", nil)
	}
	return nil
}

// endTx ends a session: its presentation's grant is fenced once the caller
// stops it (StopV1 after commit), and the row records why.
func (s *Service) endTx(ctx context.Context, tx *sql.Tx, r row, now time.Time, reason, message string) error {
	res, err := tx.ExecContext(ctx, `UPDATE playback_v1_sessions SET ended_ms=?,end_reason=?,message=?,revision=revision+1,updated_ms=? WHERE id=? AND ended_ms=0`, now.UnixMilli(), reason, message, now.UnixMilli(), r.id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	r.ended, r.endReason, r.message, r.revision = now.UnixMilli(), reason, message, r.revision+1
	if err = s.sessionEventsTx(ctx, tx, r, now, true); err != nil {
		return err
	}
	if isChannel(r) && s.Channels != nil {
		if _, err = s.Channels.EndTx(ctx, tx, r.id); err != nil {
			return err
		}
	}
	if s.onEnded != nil {
		return s.onEnded(ctx, tx, r, now)
	}
	return nil
}

// End stops a session for a reason other than its own DELETE (lease expiry,
// admin terminate, transfer commit). The grant is fenced before returning.
func (s *Service) End(ctx context.Context, id, reason, message string) error {
	s.cancelPreparedFor(ctx, id)
	r, err := loadRow(ctx, s.DB, id)
	if err != nil {
		return err
	}
	if r.ended > 0 {
		return nil
	}
	now := s.now()
	if err = dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassEstablishedPlayback, func(ctx context.Context, tx *sql.Tx) error {
		if err := s.endTx(ctx, tx, r, now, reason, message); err != nil || r.media == "" {
			return err
		}
		// The grant is fenced in the same transaction that ends the session,
		// so there is no moment where the session is over and its media still
		// plays. StopV1 below then stops the stream itself.
		_, err := tx.ExecContext(ctx, `UPDATE playback_sessions SET state='stopped' WHERE id=?`, r.media)
		return err
	}); err != nil {
		return err
	}
	s.hub.wake()
	if isChannel(r) {
		s.wakeChannels()
	}
	s.sweepQueues() // an end is when old sessions may be due for pruning
	if r.media != "" {
		return s.Playback.StopV1(ctx, r.media)
	}
	return nil
}

// expireIfLapsed ends a session whose lease lapsed (spec §5.8) when it is read
// before the sweeper gets to it.
func (s *Service) expireIfLapsed(ctx context.Context, r row) row {
	if r.ended == 0 && r.lease <= s.now().UnixMilli() {
		if err := s.End(ctx, r.id, "lease_expired", ""); err == nil {
			if fresh, err := loadRow(ctx, s.DB, r.id); err == nil {
				return fresh
			}
		}
	}
	return r
}

// session loads a session the caller may see.
func (s *Service) session(ctx context.Context, c Caller, id string) (row, error) {
	r, err := loadRow(ctx, s.DB, id)
	if err != nil {
		return r, err
	}
	if !owns(c, r) {
		return r, ErrNotFound
	}
	return s.expireIfLapsed(ctx, r), nil
}

// Get is GET /v1/playback/sessions/{id}. Reading a session renews its lease
// only through its own device's timeline, never here.
func (s *Service) Get(ctx context.Context, c Caller, id string) (SessionView, error) {
	r, err := s.session(ctx, c, id)
	if err != nil {
		return SessionView{}, err
	}
	return s.view(s.endIfChannelRefused(ctx, r)), nil
}

// endIfChannelRefused ends a live channel session the viewer is no longer
// permitted to watch (the profile's Live TV switch was turned off mid-play:
// the channel resolver refuses it with feature_restricted), with that reason,
// so the player shows why instead of a channel stuck preparing. Like
// expireIfLapsed, a read of such a session is where it ends.
func (s *Service) endIfChannelRefused(ctx context.Context, r row) row {
	if !isChannel(r) || r.ended != 0 || s.Channels == nil {
		return r
	}
	tx, done, err := dbwork.BeginRead(ctx, s.DB)
	if err != nil {
		return r
	}
	_, err = s.Channels.ReadTx(ctx, tx, rowPrincipal(r), r.id)
	done()
	var fault *playback.ControlFault
	if !errors.As(err, &fault) || fault.Code != "feature_restricted" {
		return r
	}
	if err = s.End(ctx, r.id, "feature_restricted", "Live TV is turned off for this profile."); err != nil {
		return r
	}
	if fresh, err := loadRow(ctx, s.DB, r.id); err == nil {
		return fresh
	}
	return r
}

// IsV1Session reports whether an id names a v1 session (legacy routes share
// their paths with v1 and dispatch on it).
func (s *Service) IsV1Session(ctx context.Context, id string) bool {
	var one int
	return s.DB.QueryRowContext(ctx, `SELECT 1 FROM playback_v1_sessions WHERE id=?`, id).Scan(&one) == nil
}
