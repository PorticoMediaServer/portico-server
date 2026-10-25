package livechannels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type LiveDefaults struct {
	ImportLimits   ImportLimits `json:"importLimits"`
	StreamBufferMs int          `json:"streamBufferMs"`
	RetryWindowMs  int          `json:"retryWindowMs"`
	UserAgent      string       `json:"userAgent"`
	GuideDays      int          `json:"guideDays"`
	// TimeshiftMinutes is the live rewind window. Nil is the historical
	// 30-minute default; a save that omits it keeps the current value.
	TimeshiftMinutes *int `json:"timeshiftMinutes,omitempty"`
	// LogoImport turns on fetching channel logos the source advertises.
	LogoImport bool `json:"logoImport"`
	// DiscoveryEnabled allows the HDHomeRun search to send packets on the LAN.
	DiscoveryEnabled bool `json:"discoveryEnabled"`
}

// Live rewind is a closed owner choice. The engine budget is about 150 MB per
// minute of window, capped at 16 GiB so the linear buffer stays bounded.
const (
	DefaultTimeshiftMinutes = 30
	timeshiftBytesPerMinute = 150000000
	timeshiftMaxBytes       = 16 << 30
)

var timeshiftMinuteChoices = [...]int{10, 30, 60, 120}

func ValidTimeshiftMinutes(v int) bool {
	for _, choice := range timeshiftMinuteChoices {
		if v == choice {
			return true
		}
	}
	return false
}

// TimeshiftLimits is the pure engine projection of the owner's rewind choice.
// Nil means the historical 30-minute default; any other value must be one of
// the closed choices. It names no document and performs no IO.
func TimeshiftLimits(minutes *int) (time.Duration, int64, error) {
	chosen := DefaultTimeshiftMinutes
	if minutes != nil {
		if !ValidTimeshiftMinutes(*minutes) {
			return 0, 0, ErrInvalid
		}
		chosen = *minutes
	}
	maxBytes := int64(chosen) * timeshiftBytesPerMinute
	if maxBytes > timeshiftMaxBytes {
		maxBytes = timeshiftMaxBytes
	}
	return time.Duration(chosen) * time.Minute, maxBytes, nil
}

// TimeshiftMinutesOrDefault is the one read fallback for stored settings. An
// unset or invalid legacy value reads as the actual default, matching what the
// owner settings getter shows. New writes still validate explicitly.
func TimeshiftMinutesOrDefault(minutes *int) *int {
	if minutes != nil && ValidTimeshiftMinutes(*minutes) {
		return minutes
	}
	value := DefaultTimeshiftMinutes
	return &value
}

type FilterRule struct {
	Mode   string   `json:"mode"`
	Values []string `json:"values"`
}
type SourceFilters struct {
	Categories FilterRule `json:"categories"`
	Countries  FilterRule `json:"countries"`
	Keywords   FilterRule `json:"keywords"`
}
type LogoImport struct {
	Enabled           bool `json:"enabled"`
	OverwriteExisting bool `json:"overwriteExisting"`
}
type ChannelNumbering struct {
	Mode    string `json:"mode"`
	StartAt int    `json:"startAt"`
	Step    int    `json:"step"`
}
type LiveSourceSettings struct {
	configured      bool
	EnabledGroups   []string `json:"enabledGroups"`
	EnabledChannels []string `json:"enabledChannels"`
	Kind            string   `json:"kind"`
	// Null-able overrides: a nil value inherits the server default.
	StreamBufferMs *int             `json:"streamBufferMs"`
	RetryWindowMs  *int             `json:"retryWindowMs"`
	UserAgent      string           `json:"userAgent"`
	Filters        SourceFilters    `json:"filters"`
	Logos          LogoImport       `json:"logoImport"`
	Numbering      ChannelNumbering `json:"channelNumbering"`
	// GuideSourceID names the XMLTV source that supplies this source's guide,
	// which is how a standalone guide is attached to a streaming playlist.
	GuideSourceID string `json:"guideSourceId"`
	// ImportLimits lowers the server's import limits for this source alone; a
	// zero field (or none at all) keeps the server's own. It can never raise
	// them: the server's limits stay the ceiling.
	ImportLimits *ImportLimits `json:"importLimits"`
}

// SourceImportLimits is what one source may bring in: the server's limits,
// lowered where the source's own settings ask for less.
func SourceImportLimits(server ImportLimits, own *ImportLimits) ImportLimits {
	out := server.effective()
	if own == nil {
		return out
	}
	if own.Bytes > 0 {
		out.Bytes = min(out.Bytes, own.Bytes)
	}
	if own.Channels > 0 {
		out.Channels = min(out.Channels, own.Channels)
	}
	if own.Programmes > 0 {
		out.Programmes = min(out.Programmes, own.Programmes)
	}
	return out
}

// RuntimeConfigurationTx reads the same typed documents the owner edits.
func RuntimeConfigurationTx(ctx context.Context, tx *sql.Tx, sid string) (LiveDefaults, LiveSourceSettings, error) {
	d := LiveDefaults{ImportLimits: ImportLimits{MaxUploadBytes, MaxChannels, MaxProgrammes}, StreamBufferMs: 8000, RetryWindowMs: 60000, UserAgent: "Portico/1.0", GuideDays: 7, LogoImport: true, DiscoveryEnabled: true}
	v := LiveSourceSettings{Logos: LogoImport{Enabled: true}, Numbering: ChannelNumbering{Mode: "source", StartAt: 100, Step: 1}}
	for _, entry := range []struct {
		scope  string
		target any
	}{{"live", &d}, {"live-source:" + sid, &v}} {
		var raw string
		e := tx.QueryRowContext(ctx, `SELECT body FROM admin_documents WHERE scope=?`, entry.scope).Scan(&raw)
		if e == nil {
			if entry.scope == "live-source:"+sid {
				v.configured = true
			}
			if json.Unmarshal([]byte(raw), entry.target) != nil {
				return d, v, ErrInvalid
			}
		} else if !errors.Is(e, sql.ErrNoRows) {
			return d, v, e
		}
	}
	if v.StreamBufferMs != nil {
		d.StreamBufferMs = *v.StreamBufferMs
	}
	if v.RetryWindowMs != nil {
		d.RetryWindowMs = *v.RetryWindowMs
	}
	if v.UserAgent != "" {
		d.UserAgent = v.UserAgent
	}
	d.TimeshiftMinutes = TimeshiftMinutesOrDefault(d.TimeshiftMinutes)
	d.ImportLimits = d.ImportLimits.effective()
	return d, v, nil
}
func filterMatch(v string, r FilterRule) bool {
	if len(r.Values) == 0 {
		return true
	}
	found := false
	v = strings.ToLower(v)
	for _, value := range r.Values {
		if strings.Contains(v, strings.ToLower(value)) {
			found = true
			break
		}
	}
	if r.Mode == "exclude" {
		return !found
	}
	return found
}
func (s *Store) configureInput(ctx context.Context, in *SourceInput) error {
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return e
	}
	defer tx.Rollback()
	d, v, e := RuntimeConfigurationTx(ctx, tx, in.ID)
	if e != nil {
		return e
	}
	requested := in.Limits
	in.Limits = SourceImportLimits(d.ImportLimits, v.ImportLimits)
	if requested.Bytes > 0 {
		in.Limits.Bytes = min(in.Limits.Bytes, requested.Bytes)
	}
	if requested.Channels > 0 {
		in.Limits.Channels = min(in.Limits.Channels, requested.Channels)
	}
	if requested.Programmes > 0 {
		in.Limits.Programmes = min(in.Limits.Programmes, requested.Programmes)
	}
	if v.configured || in.EnabledGroups == nil {
		in.EnabledGroups = v.EnabledGroups
	}
	if v.configured || in.EnabledChannels == nil {
		in.EnabledChannels = v.EnabledChannels
	}
	if in.remote {
		in.parseContext = ctx
		in.guideDays = d.GuideDays
	}
	in.configuration = &v

	rows, e := tx.QueryContext(ctx, `SELECT c.provider_key,n.guide_channel_id FROM admin_channel_numbers n JOIN live_sources s ON s.id=n.source_id JOIN live_channel_versions c ON c.generation_id=s.active_generation AND c.channel_id=n.channel_id WHERE n.source_id=? AND n.guide_channel_id!=''`, in.ID)
	if e != nil {
		return e
	}
	defer rows.Close()
	mappings := map[string]string{}
	for _, m := range in.Mappings {
		mappings[m.ChannelKey] = m.GuideKey
	}
	for rows.Next() {
		var key, guide string
		if e = rows.Scan(&key, &guide); e != nil {
			return e
		}
		mappings[key] = guide
	}
	if e = rows.Err(); e != nil {
		return e
	}
	in.Mappings = nil
	for key, guide := range mappings {
		in.Mappings = append(in.Mappings, ChannelMapping{key, guide})
	}
	sort.Slice(in.Mappings, func(i, j int) bool { return in.Mappings[i].ChannelKey < in.Mappings[j].ChannelKey })
	return nil
}

// Preview applies the same saved import policy as publication.
func (s *Store) Preview(ctx context.Context, a Authority, in SourceInput) (Preview, error) {
	if e := s.snapshot(ctx, a, true, func(*sql.Tx) error { return nil }); e != nil {
		return Preview{}, e
	}
	if e := s.configureInput(ctx, &in); e != nil {
		return Preview{}, e
	}
	in.parseContext = ctx
	value, e := PreviewSource(in)
	if e != nil {
		return Preview{}, e
	}
	e = s.snapshot(ctx, a, true, func(*sql.Tx) error { return nil })
	return value, e
}
func applyConfiguration(data *parsedSource, v *LiveSourceSettings, days int) {
	if v == nil {
		return
	}
	kept := data.channels[:0]
	enabled := map[string]bool{}
	for _, ch := range data.channels {
		if !filterMatch(ch.group, v.Filters.Categories) || !filterMatch(ch.country, v.Filters.Countries) || !filterMatch(ch.name, v.Filters.Keywords) {
			continue
		}
		if v.Numbering.Mode == "sequential" {
			ch.number = fmt.Sprint(v.Numbering.StartAt + len(kept)*v.Numbering.Step)
		}
		if !v.Logos.Enabled {
			ch.logo = ""
		}
		enabled[ch.key] = true
		kept = append(kept, ch)
	}
	data.channels = kept
	programmes := data.programmes[:0]
	now := time.Now()
	end := now.Add(time.Duration(days) * 24 * time.Hour)
	for _, p := range data.programmes {
		if enabled[p.channel] && (days <= 0 || p.end.After(now.Add(-24*time.Hour)) && p.start.Before(end)) {
			programmes = append(programmes, p)
		}
	}
	data.programmes = programmes
}

func (s *Store) fetchSource(ctx context.Context, f SourceFetcher, c remoteConfig) (SourceInput, int, error) {
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return SourceInput{}, 0, e
	}
	d, v, e := RuntimeConfigurationTx(ctx, tx, c.Draft.ID)
	tx.Rollback()
	if e != nil {
		return SourceInput{}, 0, e
	}
	var guideRevision int64
	if v.GuideSourceID != "" {
		var guideState string
		if e = s.db.QueryRowContext(ctx, `SELECT revision,state FROM live_sources WHERE id=?`, v.GuideSourceID).Scan(&guideRevision, &guideState); e != nil {
			return SourceInput{}, 0, ErrSourceUnavailable
		}
		if guideState != "active" {
			return SourceInput{}, 0, ErrSourceUnavailable
		}
		var raw []byte
		e = s.db.QueryRowContext(ctx, `SELECT sealed FROM live_remote_configs WHERE source_id=?`, v.GuideSourceID).Scan(&raw)
		if e != nil {
			return SourceInput{}, 0, e
		}
		text, e := s.unseal(raw, "remote:"+v.GuideSourceID)
		if e != nil {
			return SourceInput{}, 0, e
		}
		var guide remoteConfig
		if json.Unmarshal([]byte(text), &guide) != nil {
			return SourceInput{}, 0, ErrInvalid
		}
		c.Draft.GuideURL = guide.Draft.GuideURL
		if c.Draft.GuideURL == "" {
			c.Draft.GuideURL = guide.Draft.Locator
		}
		c.Approvals = append(c.Approvals, guide.Approvals...)
	}
	f.defaults = d
	in, count, e := f.fetch(ctx, c)
	if e != nil {
		return in, count, e
	}
	in.parseContext = ctx
	in.guideDays = d.GuideDays
	in.remote = true
	in.guideSourceID, in.guideSourceRevision = v.GuideSourceID, guideRevision
	if e = s.configureInput(ctx, &in); e != nil {
		return in, count, e
	}
	return in, count, nil
}
