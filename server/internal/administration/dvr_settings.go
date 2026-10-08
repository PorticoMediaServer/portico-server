package administration

import (
	"context"
	"database/sql"
	"encoding/json"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/livechannels/dvr"
	"regexp"
	"sort"
	"strings"
)

// KeepPolicy decides what a recording group keeps once it has recorded.
type KeepPolicy struct {
	Mode             string `json:"mode"`
	KeepCount        int    `json:"keepCount"`
	KeepDays         int    `json:"keepDays"`
	KeepUntilWatched bool   `json:"keepUntilWatched"`
}

// KeepModes is the published choice list.
var KeepModes = []string{"keep-all", "keep-count", "keep-days"}

// RecordingConversion says what happens to the captured stream afterwards.
// "none" keeps the transport stream exactly as captured; "remux" repackages it
// without re-encoding; "ladder" hands it to the delivery conversion ladder.
type RecordingConversion struct {
	Mode           string `json:"mode"`
	LadderID       string `json:"ladderId"`
	DeleteOriginal bool   `json:"deleteOriginal"`
}

// ConversionModes is the published choice list.
var ConversionModes = []string{"none"}

// Sidecars are the files written next to a finished recording.
type Sidecars struct {
	NFO       bool `json:"nfo"`
	Poster    bool `json:"poster"`
	Fanart    bool `json:"fanart"`
	Thumbnail bool `json:"thumbnail"`
}

// DVRDefaults is the server-wide recording document.
type DVRDefaults struct {
	PrePaddingSeconds  int                 `json:"prePaddingSeconds"`
	PostPaddingSeconds int                 `json:"postPaddingSeconds"`
	Keep               KeepPolicy          `json:"keepPolicy"`
	Conversion         RecordingConversion `json:"conversion"`
	FolderTemplate     string              `json:"folderTemplate"`
	MovieTemplate      string              `json:"movieFolderTemplate"`
	RecordingProfile   string              `json:"recordingProfile"`
	Sidecars           Sidecars            `json:"sidecars"`
	// TunerPreference decides which tuner a new recording takes when several
	// could serve it.
	TunerPreference string `json:"tunerPreference"`
}

// RecordingProfiles and TunerPreferences are published choice lists.
var (
	RecordingProfiles = []string{"original"}
	TunerPreferences  = []string{"first-available"}
)

// DefaultDVRDefaults is the answer before any owner write.
func DefaultDVRDefaults() DVRDefaults {
	return DVRDefaults{
		PrePaddingSeconds:  60,
		PostPaddingSeconds: 180,
		Keep:               KeepPolicy{Mode: "keep-all"},
		Conversion:         RecordingConversion{Mode: "none"},
		FolderTemplate:     "",
		MovieTemplate:      "",
		RecordingProfile:   "original",
		Sidecars:           Sidecars{},
		TunerPreference:    "first-available",
	}
}

// FolderToken is one substitution the recording folder template accepts.
type FolderToken struct {
	Token       string `json:"token"`
	Description string `json:"description"`
	Example     string `json:"example"`
	// Padded says the token accepts a two-digit form, written {season:02}.
	Padded bool `json:"supportsPadding"`
}

// FolderTokens is the template vocabulary. A template may use nothing else.
func FolderTokens() []FolderToken {
	return []FolderToken{
		{"{series}", "Series title, or the programme title when there is no series.", "Nova", false},
		{"{title}", "Programme or episode title.", "The Planets", false},
		{"{season}", "Season number.", "3", true},
		{"{episode}", "Episode number within the season.", "7", true},
		{"{year}", "Year the programme was first shown.", "2024", false},
		{"{channel}", "Channel name the recording came from.", "BBC Two", false},
		{"{channelNumber}", "Channel number as served.", "102", true},
		{"{date}", "Recording start date, YYYY-MM-DD.", "2026-09-16", false},
		{"{time}", "Recording start time, HH-MM.", "20-30", false},
		{"{source}", "Live source name.", "Rooftop aerial", false},
		{"{group}", "Recording group that scheduled it.", "Nova", false},
	}
}

var templateToken = regexp.MustCompile(`\{([a-zA-Z]+)(?::0?2)?\}`)

// validTemplate refuses anything but the published tokens and refuses any path
// escape, so a template can never write outside the recordings root.
func validTemplate(v string) bool {
	if v == "" || !safeText(v, 400) {
		return false
	}
	if strings.ContainsAny(v, `\:*?"<>|`) && !strings.Contains(v, ":0") {
		return false
	}
	if strings.HasPrefix(v, "/") || strings.Contains(v, "..") || strings.Contains(v, "//") {
		return false
	}
	known := map[string]bool{}
	for _, token := range FolderTokens() {
		known[strings.Trim(token.Token, "{}")] = token.Padded
	}
	remainder := templateToken.ReplaceAllStringFunc(v, func(match string) string {
		name := templateToken.FindStringSubmatch(match)[1]
		padded, ok := known[name]
		if !ok || strings.Contains(match, ":0") && !padded {
			return "\x00"
		}
		return ""
	})
	if strings.Contains(remainder, "\x00") || strings.ContainsAny(remainder, "{}") {
		return false
	}
	// At least one token must vary per recording or every recording collides.
	return strings.Contains(v, "{title}") || strings.Contains(v, "{episode}") || strings.Contains(v, "{date}")
}

func validateDVRDefaults(v *DVRDefaults) error {
	fields := []string{}
	if v.Keep.KeepUntilWatched || v.Sidecars != (Sidecars{}) || v.Conversion.DeleteOriginal {
		fields = append(fields, "settings.unsupported")
	}
	if (v.Keep.Mode == "keep-count" && v.Keep.KeepCount < 1) || (v.Keep.Mode == "keep-days" && v.Keep.KeepDays < 1) {
		fields = append(fields, "settings.keepPolicy")
	}

	if !oneOf(v.Keep.Mode, KeepModes...) {
		fields = append(fields, "settings.keepPolicy.mode")
	}
	if !oneOf(v.Conversion.Mode, ConversionModes...) {
		fields = append(fields, "settings.conversion.mode")
	}
	if !safeText(v.Conversion.LadderID, 128) || v.Conversion.Mode != "ladder" && v.Conversion.LadderID != "" {
		fields = append(fields, "settings.conversion.ladderId")
	}
	if v.Conversion.Mode == "none" && v.Conversion.DeleteOriginal {
		// There is no converted copy to keep instead, so this would delete the
		// only recording that exists.
		fields = append(fields, "settings.conversion.deleteOriginal")
	}
	if v.FolderTemplate != "" {
		fields = append(fields, "settings.folderTemplate")
	}
	if v.MovieTemplate != "" {
		fields = append(fields, "settings.movieFolderTemplate")
	}
	if !oneOf(v.RecordingProfile, RecordingProfiles...) {
		fields = append(fields, "settings.recordingProfile")
	}
	if !oneOf(v.TunerPreference, TunerPreferences...) {
		fields = append(fields, "settings.tunerPreference")
	}
	if len(fields) > 0 {
		return invalid(fields...)
	}
	clampInt(&v.PrePaddingSeconds, 0, 1800)
	clampInt(&v.PostPaddingSeconds, 0, 7200)
	clampInt(&v.Keep.KeepCount, 0, 1000)
	clampInt(&v.Keep.KeepDays, 0, 3650)
	return nil
}

const dvrScope = "dvr"

// DVRDocument is the DVR settings page: the document plus every vocabulary the
// page renders from.
type DVRDocument struct {
	Revision     int64               `json:"revision"`
	Digest       string              `json:"digest"`
	Settings     DVRDefaults         `json:"settings"`
	FolderTokens []FolderToken       `json:"folderTokens"`
	Enumerations map[string][]string `json:"enumerations"`
}

// DVRSettings reads the DVR defaults page.
func (s *Service) DVRSettings(ctx context.Context, auth Authorize) (DVRDocument, error) {
	return s.runtimeDVRSettings(ctx, auth)
}
func (s *Service) SaveDVRSettings(ctx context.Context, auth Authorize, change Change[DVRDefaults]) (DVRDocument, error) {
	return s.saveRuntimeDVRSettings(ctx, auth, change)
}

func dvrEnumerations() map[string][]string {
	return map[string][]string{"keepMode": KeepModes, "conversionMode": ConversionModes, "recordingProfile": RecordingProfiles, "tunerPreference": TunerPreferences}
}

// TunerAllocation is one live source's tuners: how many there are, what is
// using them now, and what is queued next.
type TunerAllocation struct {
	SourceID   string            `json:"sourceId"`
	SourceName string            `json:"sourceName"`
	TunerCount int               `json:"tunerCount"`
	InUse      int               `json:"inUse"`
	Active     []TunerAssignment `json:"active"`
	Upcoming   []TunerAssignment `json:"upcoming"`
	// Conflicts counts upcoming recordings that cannot all fit at once.
	Conflicts int `json:"conflicts"`
}

// TunerAssignment is one recording's claim on a tuner.
type TunerAssignment struct {
	RecordingID  string `json:"recordingId"`
	ChannelID    string `json:"channelId"`
	Title        string `json:"title"`
	State        string `json:"state"`
	StartsAt     string `json:"startsAt"`
	EndsAt       string `json:"endsAt"`
	AllocationID string `json:"allocationId,omitempty"`
}

// TunerView is the whole allocation page.
type TunerView struct {
	Sources    []TunerAllocation `json:"sources"`
	ObservedAt string            `json:"observedAt"`
	// UpcomingWindowHours is how far ahead the upcoming lists look.
	UpcomingWindowHours int `json:"upcomingWindowHours"`
}

// upcomingWindow is how far ahead the allocation page looks.
const upcomingWindow = 48

// Tuners reports which tuner each source is using and what is queued. It reads
// the DVR store's own tables; nothing here schedules or claims anything.
func (s *Service) Tuners(ctx context.Context, auth Authorize) (TunerView, error) {
	out := TunerView{Sources: []TunerAllocation{}, UpcomingWindowHours: upcomingWindow}
	now := s.milliseconds()
	out.ObservedAt = timeFromMilliseconds(now)
	horizon := now + int64(upcomingWindow)*3600000
	err := s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		// Without a live channel store there are no tuners to allocate, which is
		// an empty view rather than a failure.
		if !tableExists(ctx, tx, "live_sources") {
			return nil
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,name,tuner_count FROM live_sources ORDER BY name,id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a TunerAllocation
			if err = rows.Scan(&a.SourceID, &a.SourceName, &a.TunerCount); err != nil {
				return err
			}
			a.Active, a.Upcoming = []TunerAssignment{}, []TunerAssignment{}
			out.Sources = append(out.Sources, a)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		if !tableExists(ctx, tx, "dvr_recordings") {
			return nil
		}
		for i := range out.Sources {
			source := &out.Sources[i]
			if source.Active, err = assignments(ctx, tx, source.SourceID, `state IN('recording','starting') AND end_ms>?`, now); err != nil {
				return err
			}
			source.InUse = len(source.Active)
			if source.Upcoming, err = assignments(ctx, tx, source.SourceID, `state IN('scheduled','pending') AND start_ms>=? AND start_ms<`+itoa(horizon), now); err != nil {
				return err
			}
			source.Conflicts = overlapExcess(source.Upcoming, source.TunerCount)
		}
		return nil
	})
	return out, err
}

func itoa(v int64) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func assignments(ctx context.Context, tx *sql.Tx, source, condition string, bound int64) ([]TunerAssignment, error) {
	out := []TunerAssignment{}
	rows, err := tx.QueryContext(ctx, `SELECT id,channel_id,programme_json,state,start_ms,end_ms,allocation_id FROM dvr_recordings WHERE source_id=? AND `+condition+` ORDER BY start_ms,id`, source, bound)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var a TunerAssignment
		var programme string
		var start, end int64
		if err = rows.Scan(&a.RecordingID, &a.ChannelID, &programme, &a.State, &start, &end, &a.AllocationID); err != nil {
			return out, err
		}
		var decoded struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal([]byte(programme), &decoded)
		a.Title, a.StartsAt, a.EndsAt = decoded.Title, timeFromMilliseconds(start), timeFromMilliseconds(end)
		out = append(out, a)
	}
	return out, rows.Err()
}

// overlapExcess counts how many upcoming recordings cannot fit given the tuner
// count, by sweeping the start and end points.
func overlapExcess(list []TunerAssignment, tuners int) int {
	if tuners <= 0 || len(list) == 0 {
		return 0
	}
	type point struct {
		at    string
		delta int
	}
	points := make([]point, 0, len(list)*2)
	for _, a := range list {
		points = append(points, point{a.StartsAt, 1}, point{a.EndsAt, -1})
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].at == points[j].at {
			return points[i].delta < points[j].delta
		}
		return points[i].at < points[j].at
	})
	current, worst := 0, 0
	for _, p := range points {
		current += p.delta
		if current > worst {
			worst = current
		}
	}
	if worst > tuners {
		return worst - tuners
	}
	return 0
}

// RecordingGroup is a standing instruction to record a series or anything
// matching a keyword.
type RecordingGroup struct {
	Owner     livechannels.Owner `json:"owner"`
	Status    string             `json:"status"`
	ID        string             `json:"id"`
	Kind      string             `json:"kind"`
	Name      string             `json:"name"`
	Match     string             `json:"match"`
	SourceID  string             `json:"sourceId"`
	Enabled   bool               `json:"enabled"`
	Revision  int64              `json:"revision"`
	Options   GroupOptions       `json:"options"`
	CreatedAt string             `json:"createdAt"`
	UpdatedAt string             `json:"updatedAt"`
}

// GroupOptions overrides the server defaults for one group. A nil field
// inherits.
type GroupOptions struct {
	PrePaddingSeconds  *int        `json:"prePaddingSeconds"`
	PostPaddingSeconds *int        `json:"postPaddingSeconds"`
	Keep               *KeepPolicy `json:"keepPolicy"`
	NewEpisodesOnly    bool        `json:"newEpisodesOnly"`
	Priority           int         `json:"priority"`
	FolderTemplate     string      `json:"folderTemplate"`
}

// RecordingGroupPage is one page of the groups listing.
type RecordingGroupPage struct {
	Items      []RecordingGroup `json:"items"`
	NextCursor string           `json:"nextCursor"`
}

// RecordingGroupChange creates or updates one group.
type RecordingGroupChange struct {
	Owner            livechannels.Owner `json:"owner"`
	Anchor           dvr.Occurrence     `json:"anchor"`
	ExpectedRevision int64              `json:"expectedRevision"`
	OperationID      string             `json:"operationId"`
	Kind             string             `json:"kind"`
	Name             string             `json:"name"`
	Match            string             `json:"match"`
	SourceID         string             `json:"sourceId"`
	Enabled          bool               `json:"enabled"`
	Options          GroupOptions       `json:"options"`
}

func validateGroup(c *RecordingGroupChange) error {
	fields := []string{}
	if !oneOf(c.Kind, "series", "keyword") {
		fields = append(fields, "kind")
	}
	if c.Name == "" || !safeText(c.Name, 160) {
		fields = append(fields, "name")
	}
	if c.Match == "" || !safeText(c.Match, 200) {
		fields = append(fields, "match")
	}
	if !safeText(c.SourceID, 128) {
		fields = append(fields, "sourceId")
	}
	if c.Options.Keep != nil && !oneOf(c.Options.Keep.Mode, KeepModes...) {
		fields = append(fields, "options.keepPolicy.mode")
	}
	if c.Options.FolderTemplate != "" && !validTemplate(c.Options.FolderTemplate) {
		fields = append(fields, "options.folderTemplate")
	}
	if c.Options.PrePaddingSeconds != nil && (*c.Options.PrePaddingSeconds < 0 || *c.Options.PrePaddingSeconds > 1800) {
		fields = append(fields, "options.prePaddingSeconds")
	}
	if c.Options.PostPaddingSeconds != nil && (*c.Options.PostPaddingSeconds < 0 || *c.Options.PostPaddingSeconds > 7200) {
		fields = append(fields, "options.postPaddingSeconds")
	}
	if len(fields) > 0 {
		return invalid(fields...)
	}
	clampInt(&c.Options.Priority, -100, 100)
	if c.Options.Keep != nil {
		clampInt(&c.Options.Keep.KeepCount, 0, 1000)
		clampInt(&c.Options.Keep.KeepDays, 0, 3650)
	}
	return nil
}

// RecordingGroups pages the groups newest first.
func (s *Service) RecordingGroups(ctx context.Context, auth Authorize, token string, limit int) (RecordingGroupPage, error) {
	return s.runtimeRecordingGroups(ctx, auth, token, limit)
}
func (s *Service) SaveRecordingGroup(ctx context.Context, auth Authorize, id string, change RecordingGroupChange) (RecordingGroup, error) {
	return s.saveRuntimeRecordingGroup(ctx, auth, id, change)
}
func (s *Service) DeleteRecordingGroup(ctx context.Context, auth Authorize, id string, revision int64, operation string) error {
	return s.deleteRuntimeRecordingGroup(ctx, auth, id, revision, operation)
}
