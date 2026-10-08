package playbackv1

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/audiofacts"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/subtitles"
)

// Options is `GET /v1/items/{itemId}/playback-options` (spec §4.1).
type Options struct {
	ItemID    string         `json:"itemId"`
	Kind      string         `json:"kind"`
	Resume    *OptionsResume `json:"resume,omitempty"`
	Versions  []Version      `json:"versions"`
	Chapters  []Chapter      `json:"chapters"`
	Markers   []Marker       `json:"markers"`
	Preferred *Preferred     `json:"preferred,omitempty"`
	Plan      *Plan          `json:"plan,omitempty"`
}
type OptionsResume struct {
	PositionMs int64  `json:"positionMs"`
	PartIndex  int    `json:"partIndex"`
	UpdatedAt  string `json:"updatedAt,omitempty"`
}
type Version struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Edition     string        `json:"edition,omitempty"`
	Container   string        `json:"container,omitempty"`
	SizeBytes   int64         `json:"sizeBytes"`
	DurationMs  int64         `json:"durationMs"`
	BitrateKbps int           `json:"bitrateKbps,omitempty"`
	Parts       []Part        `json:"parts"`
	Video       []VideoStream `json:"video"`
	Audio       []AudioStream `json:"audio"`
	Subtitles   []SubStream   `json:"subtitles"`
}
type Part struct {
	ID         string `json:"id"`
	Index      int    `json:"index"`
	DurationMs int64  `json:"durationMs"`
	SizeBytes  int64  `json:"sizeBytes"`
}
type VideoStream struct {
	ID          string  `json:"id"`
	Codec       string  `json:"codec"`
	Profile     string  `json:"profile,omitempty"`
	Level       int     `json:"level,omitempty"`
	BitDepth    int     `json:"bitDepth,omitempty"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	Fps         float64 `json:"fps,omitempty"`
	HDR         string  `json:"hdr,omitempty"`
	BitrateKbps int     `json:"bitrateKbps,omitempty"`
}
type AudioStream struct {
	ID             string `json:"id"`
	Codec          string `json:"codec"`
	Channels       int    `json:"channels,omitempty"`
	Layout         string `json:"layout,omitempty"`
	Atmos          bool   `json:"atmos"`
	Language       string `json:"language,omitempty"`
	Title          string `json:"title,omitempty"`
	Default        bool   `json:"default"`
	Commentary     bool   `json:"commentary"`
	VisualImpaired bool   `json:"visualImpaired"`
}
type SubStream struct {
	ID       string `json:"id"`
	Format   string `json:"format"`
	Source   string `json:"source"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Forced   bool   `json:"forced"`
	SDH      bool   `json:"sdh"`
	Default  bool   `json:"default"`
}
type Chapter struct {
	StartMs int64  `json:"startMs"`
	Title   string `json:"title"`
}

// Marker is one skippable segment (spec §4.2). ID is what a skip report names;
// AutomaticSafe says the server authorizes skipping it unattended (a viewer's
// `auto` preference applies only then).
type Marker struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	StartMs       int64  `json:"startMs"`
	EndMs         int64  `json:"endMs"`
	Confidence    string `json:"confidence"`
	AutomaticSafe bool   `json:"automaticSafe"`
}
type Preferred struct {
	VersionID  string `json:"versionId"`
	AudioID    string `json:"audioId,omitempty"`
	SubtitleID string `json:"subtitleId,omitempty"`
	Reason     string `json:"reason"`
}
type Plan struct {
	VersionID            string       `json:"versionId"`
	Mode                 string       `json:"mode"`
	Streams              []PlanStream `json:"streams"`
	EstimatedBitrateKbps int          `json:"estimatedBitrateKbps,omitempty"`
	Quality              Quality      `json:"quality"`
}
type PlanStream struct {
	ID      string   `json:"id"`
	Action  string   `json:"action"`
	To      string   `json:"to,omitempty"`
	Reasons []string `json:"reasons"`
}

// Quality is the client's quality request (spec §5.1).
type Quality struct {
	Mode                string `json:"mode"`
	MaxVideoBitrateKbps int    `json:"maxVideoBitrateKbps,omitempty"`
	MaxHeight           int    `json:"maxHeight,omitempty"`
	MaxAudioBitrateKbps int    `json:"maxAudioBitrateKbps,omitempty"`
}

// Validate bounds a quality request.
func (q Quality) Validate(path string) error {
	switch q.Mode {
	case "original":
		if q.MaxVideoBitrateKbps != 0 || q.MaxHeight != 0 || q.MaxAudioBitrateKbps != 0 {
			return &FieldError{Path: path}
		}
	case "limit":
		if q.MaxVideoBitrateKbps < 0 || q.MaxVideoBitrateKbps > 2_000_000 || q.MaxHeight < 0 || q.MaxHeight > 8640 || q.MaxAudioBitrateKbps < 0 || q.MaxAudioBitrateKbps > 10_000 {
			return &FieldError{Path: path}
		}
	default:
		return &FieldError{Path: path + ".mode"}
	}
	return nil
}

// Preview is what the options query asks "what if" about (plan §7 item 15).
type Preview struct {
	VersionID  string
	PartID     string // one part of a multi-part version (plans that file)
	AudioID    string
	SubtitleID string // "none" for off
	Quality    Quality
}

// MarkerFunc projects the viewer's segment markers for one source.
type MarkerFunc func(ctx context.Context, tx *sql.Tx, p identity.Principal, item, source string) ([]Marker, error)

// Service is the v1 resource layer over the existing delivery machinery. One
// per router: it owns the event hub and the lease sweeper.
type Service struct {
	DB        *sql.DB
	Playback  *playback.Service
	Subtitles *subtitles.Service
	Now       func() time.Time
	// LeaseDuration and ReportEvery override the spec defaults (tests).
	LeaseDuration time.Duration
	ReportEvery   time.Duration
	// PausedLimit is how long a video session may stay paused before the
	// sweeper ends it (operations pausedSessionTimeoutMinutes: Plex "Terminate
	// Sessions Paused for Longer Than"). Nil or non-positive: the sweeper is
	// off. Wired from console settings; a read error ends nothing on that tick.
	PausedLimit func(context.Context) time.Duration
	// ReadyWait bounds how long a start waits for its presentation before 202.
	ReadyWait time.Duration
	// ReadyCheck replaces the presentation readiness probe (tests).
	ReadyCheck func(context.Context, playback.Session) (bool, error)
	// QueueKeyLimit overrides MaxQueueKeys (tests).
	QueueKeyLimit int64
	// QueueScanBudget and QueueCountBudget override how many keys a request
	// snapshots itself and how long a large shuffle's count may take (tests);
	// PauseQueueBuilds holds background builds until ResumeQueueBuilds (tests).
	QueueScanBudget  int64
	QueueCountBudget time.Duration
	PauseQueueBuilds bool
	// LibraryCheck and Visibility authorize queue sources and windows (library
	// access, restrictions and member limits); HTTP composition installs them.
	LibraryCheck LibraryCheck
	Visibility   Visibility
	// Unrestricted says the caller's item fence admits every item (every
	// library, no restrictions): such a caller sees a container as it is, so a
	// large shuffle can use the container's cached size (queue_counts.go).
	// Nil: never.
	Unrestricted func(ctx context.Context, tx *sql.Tx, p identity.Principal) (bool, error)
	// CreatePlaylist is the catalog's playlist create (review P32): a new
	// playlist of the caller's holding items in order, under the operation key.
	CreatePlaylist func(ctx context.Context, p identity.Principal, key, name, summary string, items []string) (id string, revision int64, err error)
	// PostPlay reads the viewer's post-play policy (autoplay, countdown) for
	// QueueView.postPlay (spec §18.5).
	PostPlay func(ctx context.Context, tx *sql.Tx, p identity.Principal) (autoplay bool, countdownSeconds int, err error)
	// Channels plays Live TV and Library Channels as v1 sessions (§18.6);
	// ChannelWake starts the linear runtime's work after a change; TunerAvailable
	// says whether a live source has a tuner for one more viewer (excluding the
	// session being replaced). Nil Channels: channel starts are unavailable.
	Channels       ChannelEngine
	ChannelWake    func()
	TunerAvailable func(ctx context.Context, tx *sql.Tx, sourceID, replacing string) (bool, error)
	// MeasureAudio measures an audio asset's facts (spec §18.7) when a play finds
	// them missing; nil: plays without facts are unavailable for client decoding.
	MeasureAudio audiofacts.Measure
	// AudioMeasureBudget bounds that measurement (default 5 s).
	AudioMeasureBudget time.Duration
	hub                eventHub
	sweep              sweeper
	build              builder
	admission          admissionGates
	queueSweep         queueSweeper
	// Hooks other resources (transfers, queues, groups) attach to sessions.
	// SkipEvidence records a marker skip in the console's diagnostics lane
	// (wired by HTTP composition); nil records the skip without diagnostics.
	SkipEvidence  func(ctx context.Context, code string, fields map[string]int64)
	timelineHooks []timelineHook
	afterTimeline []func(context.Context, row, Report)
	onEnded       func(context.Context, *sql.Tx, row, time.Time) error
}

// New returns the service for one router.
func New(db *sql.DB, player *playback.Service, subs *subtitles.Service) *Service {
	return &Service{DB: db, Playback: player, Subtitles: subs}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var kindNames = map[string]string{"movie": "movie", "episode": "episode", "song": "track", "track": "track", "audiobook_file": "audiobook", "audiobook": "audiobook", "book": "audiobook", "music_video": "musicVideo", "extra": "extra", "trailer": "extra", "recording": "recording"}

// v1Kind is the item's kind in v1 vocabulary; unknown kinds pass through.
func v1Kind(kind string) string {
	if k, ok := kindNames[kind]; ok {
		return k
	}
	return kind
}

type assetRow struct {
	id, container, videoCodec string
	size                      int64
	duration                  float64
	width, height, part       int
}

func streamID(prefix string, index int) string { return prefix + strconv.Itoa(index) }

// ParseStreamID reads "a3" → 3 for the given prefix.
func ParseStreamID(prefix, id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, prefix)
	if !ok || rest == "" || len(rest) > 4 {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil && n >= 0
}

func resolutionLabel(height int) string {
	switch {
	case height >= 2000:
		return "4K"
	case height >= 1000:
		return "1080p"
	case height >= 700:
		return "720p"
	case height > 0:
		return "SD"
	}
	return ""
}

func hdrName(transfer, detail string) string {
	if strings.Contains(detail, "dolby_vision") || strings.Contains(detail, "\"dovi") {
		return "dolbyvision"
	}
	switch transfer {
	case "smpte2084":
		if strings.Contains(detail, "hdr10plus") || strings.Contains(detail, "hdr10_plus") {
			return "hdr10plus"
		}
		return "hdr10"
	case "arib-std-b67":
		return "hlg"
	}
	return ""
}

// readAssets lists the item's available files in part order.
func readAssets(ctx context.Context, tx *sql.Tx, item string) ([]assetRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT a.token,a.container,a.video_codec,a.size,a.duration,a.width,a.height,link.part_index
 FROM catalog_entities item JOIN catalog_asset_links link ON link.entity_id=item.id JOIN catalog_assets a ON a.id=link.asset_id
 WHERE item.public_id=pid_blob(?) AND a.available=1
 ORDER BY link.part_index,a.token`, item)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []assetRow
	for rows.Next() {
		var a assetRow
		if err = rows.Scan(&a.id, &a.container, &a.videoCodec, &a.size, &a.duration, &a.width, &a.height, &a.part); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// versionsOf groups files: distinct part indexes are one multi-part version;
// otherwise files of the same shape (container, video codec, height) whose
// part indexes are distinct are one version's parts (two copies of a two-part
// title are two versions of two parts, not four), and every other file is its
// own version (a 4K remux next to a 1080p encode). Parts keep their order.
func versionsOf(assets []assetRow) [][]assetRow {
	distinct := func(g []assetRow) bool {
		seen := map[int]bool{}
		for _, a := range g {
			if seen[a.part] {
				return false
			}
			seen[a.part] = true
		}
		return true
	}
	if len(assets) > 1 && distinct(assets) {
		return [][]assetRow{assets}
	}
	type shape struct {
		container, codec string
		height           int
	}
	order := []shape{}
	groups := map[shape][]assetRow{}
	for _, a := range assets {
		k := shape{a.container, a.videoCodec, a.height}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], a)
	}
	out := make([][]assetRow, 0, len(assets))
	for _, k := range order {
		if g := groups[k]; len(g) > 1 && distinct(g) {
			out = append(out, g)
			continue
		}
		for _, a := range groups[k] {
			out = append(out, []assetRow{a})
		}
	}
	return out
}

// partAsset resolves a version's part (its part index, as Options lists it)
// to that part's file. Part 0, or the version's first part, is the version
// itself; a part the version doesn't have is a field error. A version that
// isn't a group's first file is left as it is (the planner checks it).
func (s *Service) partAsset(ctx context.Context, item, version string, part int) (string, error) {
	if part == 0 {
		return version, nil
	}
	tx, done, err := dbwork.BeginRead(ctx, s.DB)
	if err != nil {
		return "", err
	}
	defer done()
	assets, err := readAssets(ctx, tx, item)
	if err != nil {
		return "", err
	}
	for i, group := range versionsOf(assets) {
		if version != "" && group[0].id != version || version == "" && i > 0 {
			continue
		}
		for _, a := range group {
			if a.part == part {
				return a.id, nil
			}
		}
		if group[0].part == part {
			return group[0].id, nil
		}
		return "", &FieldError{Path: "partIndex"}
	}
	return "", &FieldError{Path: "partIndex"}
}

func (s *Service) streams(ctx context.Context, tx *sql.Tx, asset string, v *Version) error {
	rows, err := tx.QueryContext(ctx, `SELECT stream_index,type,codec,language,title,channels,channel_layout,is_default,is_forced,color_transfer,detail_json FROM asset_streams WHERE asset_id=? ORDER BY stream_index`, asset)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var index, channels int
		var kind, codec, language, title, layout, transfer, detail string
		var isDefault, forced bool
		if err = rows.Scan(&index, &kind, &codec, &language, &title, &channels, &layout, &isDefault, &forced, &transfer, &detail); err != nil {
			return err
		}
		var facts struct {
			Profile     string  `json:"profile"`
			Level       int     `json:"level"`
			BitDepth    int     `json:"bitDepth"`
			Width       int     `json:"width"`
			Height      int     `json:"height"`
			FrameRate   float64 `json:"frameRate"`
			BitRate     int     `json:"bitRate"`
			ObjectAudio string  `json:"objectAudio"`
		}
		_ = json.Unmarshal([]byte(detail), &facts)
		lower := strings.ToLower(title)
		switch kind {
		case "video":
			v.Video = append(v.Video, VideoStream{ID: streamID("v", index), Codec: codec, Profile: strings.ToLower(facts.Profile), Level: facts.Level, BitDepth: facts.BitDepth, Width: facts.Width, Height: facts.Height, Fps: facts.FrameRate, HDR: hdrName(transfer, detail), BitrateKbps: facts.BitRate / 1000})
		case "audio":
			v.Audio = append(v.Audio, AudioStream{ID: streamID("a", index), Codec: codec, Channels: channels, Layout: layout, Atmos: facts.ObjectAudio == "atmos", Language: language, Title: title, Default: isDefault, Commentary: strings.Contains(lower, "commentary"), VisualImpaired: strings.Contains(lower, "descriptive") || strings.Contains(lower, "audio description")})
		case "subtitle":
			if s.Subtitles == nil {
				v.Subtitles = append(v.Subtitles, SubStream{ID: streamID("s", index), Format: codec, Source: "embedded", Language: language, Title: title, Forced: forced, SDH: strings.Contains(lower, "sdh") || strings.Contains(lower, "hearing"), Default: isDefault})
			}
		}
	}
	return rows.Err()
}

func subtitleSource(origin string) string {
	switch origin {
	case "embedded":
		return "embedded"
	case "sidecar", "local":
		return "sidecar"
	}
	return "downloaded"
}

// Options reads what an item offers and what this device would get (§4.1). No
// side effects. The caller has already admitted the item for this viewer.
// Reach is where a request comes from, for remote caps: a preview is planned
// exactly as a start from the same place would be (review P4).
type Reach struct {
	Remote                  bool
	AdminMaxVideoBitrateBPS int
}

func (s *Service) Options(ctx context.Context, p identity.Principal, item string, preview Preview, markersFor MarkerFunc, reach Reach) (Options, error) {
	tx, done, err := dbwork.BeginRead(ctx, s.DB)
	if err != nil {
		return Options{}, err
	}
	defer done()
	// Facts are synchronous; the deleted CheckItemReadiness only guarded the
	// old projection.
	var kind string
	var edition string
	if err = tx.QueryRowContext(ctx, `SELECT k.name,COALESCE(d.edition,'') FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind AND k.playable=1 LEFT JOIN catalog_item_details d ON d.entity_id=e.id WHERE e.public_id=pid_blob(?)`, item).Scan(&kind, &edition); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Options{}, ErrNotFound
		}
		return Options{}, err
	}
	out := Options{ItemID: item, Kind: v1Kind(kind), Versions: []Version{}, Chapters: []Chapter{}, Markers: []Marker{}}
	assets, err := readAssets(ctx, tx, item)
	if err != nil {
		return Options{}, err
	}
	var catalog subtitles.Catalog
	if s.Subtitles != nil && len(assets) > 0 {
		if catalog, err = s.Subtitles.ListTx(ctx, tx, p, item, ""); err != nil {
			return Options{}, err
		}
	}
	renderers := map[string]string{}
	for _, group := range versionsOf(assets) {
		first := group[0]
		v := Version{ID: first.id, Edition: edition, Container: first.container, Parts: []Part{}, Video: []VideoStream{}, Audio: []AudioStream{}, Subtitles: []SubStream{}}
		for _, a := range group {
			v.Parts = append(v.Parts, Part{ID: a.id, Index: a.part, DurationMs: int64(a.duration * 1000), SizeBytes: a.size})
			v.SizeBytes += a.size
			v.DurationMs += int64(a.duration * 1000)
		}
		if v.DurationMs > 0 {
			v.BitrateKbps = int(v.SizeBytes * 8 / v.DurationMs)
		}
		if err = s.streams(ctx, tx, first.id, &v); err != nil {
			return Options{}, err
		}
		for _, r := range catalog.Resources {
			renderers[r.ID] = r.Renderer
			if r.SourceID == first.id && r.Enabled {
				lower := strings.ToLower(r.Title)
				v.Subtitles = append(v.Subtitles, SubStream{ID: r.ID, Format: r.Format, Source: subtitleSource(r.Origin), Language: r.Language, Title: r.Title, Forced: r.Forced, SDH: strings.Contains(lower, "sdh") || strings.Contains(lower, "hearing"), Default: r.Default})
			}
		}
		label := []string{}
		if l := resolutionLabel(first.height); l != "" {
			label = append(label, l)
		}
		if len(v.Video) > 0 && v.Video[0].HDR != "" {
			if v.Video[0].HDR == "dolbyvision" {
				label = append(label, "Dolby Vision")
			} else {
				label = append(label, "HDR")
			}
		}
		if first.container != "" && len(label) > 0 {
			label = append(label, strings.ToUpper(first.container))
		}
		v.Label = strings.Join(label, " · ")
		out.Versions = append(out.Versions, v)
	}
	var positionMS int64
	if err = tx.QueryRowContext(ctx, `SELECT position FROM progress WHERE profile_id=? AND item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, identity.PersonalKey(p.Viewer), item).Scan(&positionMS); err == nil && positionMS > 0 {
		out.Resume = &OptionsResume{PositionMs: positionMS}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Options{}, err
	}
	if len(out.Versions) == 0 {
		return out, nil
	}
	choice, version, err := s.choice(out, preview, reach)
	if err != nil {
		return Options{}, err
	}
	// The preview's subtitle, when there is one: a known track of this version,
	// drawn by the device (sidecar) or burned into the video.
	burn := false
	if preview.SubtitleID != "" && preview.SubtitleID != "none" {
		known := false
		for _, sub := range version.Subtitles {
			known = known || sub.ID == preview.SubtitleID
		}
		if !known {
			return Options{}, &FieldError{Path: "subtitleId"}
		}
		burn = renderers[preview.SubtitleID] == "burn_in"
	}
	// Chapters and markers are the chosen version's, part after part on one
	// timeline (each part's starts offset by the parts before it).
	offset := int64(0)
	for _, part := range version.Parts {
		rows, err := tx.QueryContext(ctx, `SELECT title,start_seconds FROM asset_chapters WHERE asset_id=? ORDER BY chapter_index`, part.ID)
		if err != nil {
			return Options{}, err
		}
		for rows.Next() {
			var c Chapter
			var start float64
			if err = rows.Scan(&c.Title, &start); err != nil {
				rows.Close()
				return Options{}, err
			}
			c.StartMs = offset + int64(start*1000)
			out.Chapters = append(out.Chapters, c)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return Options{}, err
		}
		rows.Close()
		if markersFor != nil {
			markers, err := markersFor(ctx, tx, p, item, part.ID)
			if err != nil {
				return Options{}, err
			}
			for _, m := range markers {
				m.StartMs, m.EndMs = m.StartMs+offset, m.EndMs+offset
				out.Markers = append(out.Markers, m)
			}
		}
		offset += part.DurationMs
	}
	done()
	out.Preferred = &Preferred{VersionID: version.ID, Reason: "default"}
	if s.Playback != nil {
		plan, err := s.Playback.PlanV1(ctx, p, item, choice)
		switch {
		case err == nil:
			view := planView(plan, version, choice)
			if preview.SubtitleID != "" && preview.SubtitleID != "none" {
				view.withSubtitle(preview.SubtitleID, burn)
			}
			view.Quality = preview.Quality
			if view.Quality.Mode == "" {
				view.Quality.Mode = "original"
			}
			out.Plan = &view
			if plan.AudioStream >= 0 {
				out.Preferred.AudioID = streamID("a", plan.AudioStream)
			}
		case errors.Is(err, playback.ErrTranscodingDisabled):
			// Options still describe the title; starting it will be refused.
		default:
			// A refused choice (no permitted route for this preview) has no plan.
			var refused playback.ErrDeliveryRefused
			if !errors.As(err, &refused) {
				return Options{}, err
			}
		}
	}
	if out.Preferred.AudioID == "" {
		for _, a := range version.Audio {
			if a.Default {
				out.Preferred.AudioID = a.ID
				break
			}
		}
	}
	return out, nil
}

// choice turns a preview (or a start request) into the planner's choice.
func (s *Service) choice(o Options, preview Preview, reach Reach) (playback.V1Choice, Version, error) {
	version := o.Versions[0]
	if preview.VersionID != "" {
		found := false
		for _, v := range o.Versions {
			if v.ID == preview.VersionID {
				version, found = v, true
			}
		}
		if !found {
			return playback.V1Choice{}, Version{}, &FieldError{Path: "versionId"}
		}
	}
	c := playback.V1Choice{AssetID: version.ID, Remote: reach.Remote}
	if preview.PartID != "" {
		found := false
		for _, part := range version.Parts {
			found = found || part.ID == preview.PartID
		}
		if !found {
			return playback.V1Choice{}, Version{}, &FieldError{Path: "partId"}
		}
		c.AssetID = preview.PartID
	}
	if reach.Remote {
		c.AdminMaxVideoBitrateBPS = reach.AdminMaxVideoBitrateBPS
	}
	if preview.AudioID != "" {
		index, ok := ParseStreamID("a", preview.AudioID)
		known := false
		for _, a := range version.Audio {
			known = known || a.ID == preview.AudioID
		}
		if !ok || !known {
			return playback.V1Choice{}, Version{}, &FieldError{Path: "audioId"}
		}
		c.AudioStream = &index
	}
	if preview.Quality.Mode == "limit" {
		c.Limit = true
		c.MaxVideoBitrateBPS = preview.Quality.MaxVideoBitrateKbps * 1000
		c.MaxAudioBitrateBPS = preview.Quality.MaxAudioBitrateKbps * 1000
		c.MaxVideoHeight = preview.Quality.MaxHeight
	}
	return c, version, nil
}

// reasonNames maps the planner's reason codes to the v1 vocabulary clients
// localise (spec §2 "delivery decision"); unknown codes pass through.
var reasonNames = map[string]string{
	playback.ReasonContainerRequiresRemux:    "container_unsupported",
	playback.ReasonAudioCodecRequiresConvert: "audio_codec_unsupported",
	playback.ReasonVideoCodecRequiresConvert: "video_codec_unsupported",
	playback.ReasonHeightExceedsPolicy:       "height_above_request",
	playback.ReasonBitrateExceedsPolicy:      "bitrate_above_request",
	playback.ReasonHDRNotAllowed:             "hdr_tone_mapping_required",
	playback.ReasonSubtitleBurnIn:            "subtitle_image_needs_burn",
	playback.ReasonFiniteNormalization:       "stream_normalized",
}

// chosen-route codes that explain nothing to a viewer.
var quietReasons = map[string]bool{
	playback.ReasonDirectPlayCompatible: true, playback.ReasonDirectAudioCompatible: true, playback.ReasonRemotePreflightCompatible: true,
	playback.ReasonPreparedVersionCompatible: true, playback.ReasonDirectPlayPreferred: true, playback.ReasonDirectStreamPreferred: true,
	playback.ReasonTranscodePreferred: true, playback.ReasonPlanningMaximumFidelity: true, playback.ReasonPlanningCompatibility: true,
	playback.ReasonPlanningMinimizeWork: true, playback.ReasonHardwareBackend: true, playback.ReasonSoftwareFallback: true,
	playback.ReasonQualityRungSelected: true,
}

// Decision is one stream family's delivery in a presentation (spec §5.1).
type Decision struct {
	Action   string   `json:"action"`
	To       string   `json:"to,omitempty"`
	Channels int      `json:"channels,omitempty"`
	Reasons  []string `json:"reasons"`
}
type Decisions struct {
	Video     *Decision `json:"video,omitempty"`
	Audio     *Decision `json:"audio,omitempty"`
	Subtitles *Decision `json:"subtitles,omitempty"`
}

func reasons(plan playback.DeliveryPlan, family string, choice playback.V1Choice) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(r string) {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, code := range plan.ReasonCodes {
		if quietReasons[code] {
			continue
		}
		if family == "audio" && code != playback.ReasonAudioCodecRequiresConvert && code != playback.ReasonContainerRequiresRemux && code != playback.ReasonAlternateAudioPolicy {
			continue
		}
		if family == "video" && code == playback.ReasonAudioCodecRequiresConvert {
			continue
		}
		if name, ok := reasonNames[code]; ok {
			code = name
		}
		if code == "bitrate_above_request" && choice.AdminCapped() {
			code = "admin_cap_remote_bitrate"
		}
		add(code)
	}
	return out
}

func action(mode, a string) string {
	switch {
	case mode == "direct" || mode == "remote":
		return "direct"
	case a == "convert":
		return "transcode"
	}
	return "copy"
}

// decisions is the per-family delivery of a plan.
func decisions(plan playback.DeliveryPlan, choice playback.V1Choice, hasVideo bool) Decisions {
	var d Decisions
	if hasVideo {
		video := &Decision{Action: action(plan.Mode, plan.VideoAction), Reasons: reasons(plan, "video", choice)}
		if plan.BurnIn != nil {
			video.Action = "burn"
		}
		if video.Action == "transcode" {
			video.To = plan.OutputVideoCodec
		}
		d.Video = video
	}
	if plan.AudioStream >= 0 || plan.SourceAudioCodec != "" {
		audio := &Decision{Action: action(plan.Mode, plan.AudioAction), Reasons: reasons(plan, "audio", choice)}
		if audio.Action == "transcode" {
			audio.To, audio.Channels = plan.OutputAudioCodec, plan.AudioChannels
		}
		d.Audio = audio
	}
	return d
}

func planView(plan playback.DeliveryPlan, version Version, choice playback.V1Choice) Plan {
	mode := "stream"
	if plan.Mode == "direct" || plan.Mode == "remote" {
		mode = "direct"
	}
	view := Plan{VersionID: version.ID, Mode: mode, Streams: []PlanStream{}, EstimatedBitrateKbps: (plan.VideoBitrateBPS + plan.AudioBitrateBPS) / 1000}
	d := decisions(plan, choice, len(version.Video) > 0)
	if d.Video != nil && len(version.Video) > 0 {
		view.Streams = append(view.Streams, PlanStream{ID: version.Video[0].ID, Action: d.Video.Action, To: d.Video.To, Reasons: d.Video.Reasons})
	}
	if d.Audio != nil {
		id := ""
		if plan.AudioStream >= 0 {
			id = streamID("a", plan.AudioStream)
		} else if len(version.Audio) > 0 {
			id = version.Audio[0].ID
		}
		if id != "" {
			view.Streams = append(view.Streams, PlanStream{ID: id, Action: d.Audio.Action, To: d.Audio.To, Reasons: d.Audio.Reasons})
		}
	}
	return view
}

// withSubtitle adds the preview's subtitle to a plan: drawn by the device
// (sidecar), or burned in, which converts the video (as a start with it
// would: decision video "burn").
func (v *Plan) withSubtitle(id string, burn bool) {
	if !burn {
		v.Streams = append(v.Streams, PlanStream{ID: id, Action: "sidecar", Reasons: []string{}})
		return
	}
	v.Mode = "stream"
	for i := range v.Streams {
		if strings.HasPrefix(v.Streams[i].ID, "v") {
			v.Streams[i].Action = "burn"
			v.Streams[i].Reasons = append(v.Streams[i].Reasons, "subtitle_burn_in")
		}
	}
	v.Streams = append(v.Streams, PlanStream{ID: id, Action: "burn", Reasons: []string{}})
}

func fmtMs(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
