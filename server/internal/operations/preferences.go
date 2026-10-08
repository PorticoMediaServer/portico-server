package operations

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/text/language"
	"sort"
	"strings"
	"unicode/utf8"

	"portico.local/server/internal/identity"
)

// PreferenceRegistryRevision changes whenever a field is added, removed, or its
// domain moves. Clients render settings from the published registry, so the
// revision is the only thing they have to compare between releases.
const PreferenceRegistryRevision = "p39.CD32.B4"

const (
	HomeLayoutMaxRows        = 64
	HomeLayoutRowIDMaxLength = 128
	ScopeProfileServer       = "profile-server"
	ScopeDeviceClass         = "profile-device-class"
)

// PreferenceScopes is the precedence order: later scopes win over earlier ones.
var PreferenceScopes = []string{ScopeProfileServer, ScopeDeviceClass}

// PreferenceField is the published description of one viewer preference. The
// server is the only authority on defaults, domains and clamps; a client that
// cannot find a field in this registry must not invent one.
type PreferenceField struct {
	Consumer      string   `json:"consumer"`
	Step          *float64 `json:"step,omitempty"`
	Key           string   `json:"key"`
	Type          string   `json:"type"`
	Default       any      `json:"default"`
	AllowedValues []any    `json:"allowedValues,omitempty"`
	Min           *float64 `json:"min,omitempty"`
	Max           *float64 `json:"max,omitempty"`
	Scopes        []string `json:"scopes"`
	Group         string   `json:"group"`
	LabelKey      string   `json:"labelKey"`

	maxLength int
	maxItems  int
}

func (f PreferenceField) allows(scope string) bool {
	for _, s := range f.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func group(key string) string {
	if i := strings.IndexByte(key, '.'); i > 0 {
		return key[:i]
	}
	return key
}
func label(key string) string { return "preferences." + key }
func numeric(v float64) *float64 {
	out := v
	return &out
}
func field(key, kind string, def any, scope string) PreferenceField {
	return PreferenceField{Key: key, Type: kind, Default: def, Scopes: []string{scope}, Group: group(key), LabelKey: label(key)}
}
func prefBool(key string, def bool, scope string) PreferenceField {
	return field(key, "boolean", def, scope)
}
func prefEnum(key, def string, allowed []string, scope string) PreferenceField {
	f := field(key, "string", def, scope)
	for _, v := range allowed {
		f.AllowedValues = append(f.AllowedValues, v)
	}
	return f
}
func prefInt(key string, def, low, high int, scope string) PreferenceField {
	f := field(key, "integer", def, scope)
	f.Min, f.Max, f.Step = numeric(float64(low)), numeric(float64(high)), numeric(1)
	return f
}
func prefIntEnum(key string, def int, allowed []int, scope string) PreferenceField {
	f := field(key, "integer", def, scope)
	f.Step = numeric(1)
	for _, v := range allowed {
		f.AllowedValues = append(f.AllowedValues, v)
	}
	return f
}
func prefNumberEnum(key string, def float64, allowed []float64, scope string) PreferenceField {
	f := field(key, "number", def, scope)
	for _, v := range allowed {
		f.AllowedValues = append(f.AllowedValues, v)
	}
	return f
}
func prefText(key, def string, maxLength int, scope string) PreferenceField {
	f := field(key, "string", def, scope)
	f.maxLength = maxLength
	return f
}
func prefList(key string, maxItems, maxLength int, scope string) PreferenceField {
	f := field(key, "string-list", []string{}, scope)
	f.maxItems, f.maxLength = maxItems, maxLength
	return f
}

var (
	skipModes      = []string{"ask", "auto", "off"}
	deliveryModes  = []string{"allow", "prefer", "never", "require"}
	qualityModes   = []string{"off", "automatic", "original", "high", "standard", "data-saver"}
	videoHeights   = []int{360, 480, 720, 1080, 1440, 2160, 4320}
	playbackSpeeds = []float64{0.5, 0.75, 1, 1.25, 1.5, 1.75, 2}
)

// qualityDefaults keeps each network lane honest about its own cost. Cellular
// starts conservative; the local lane starts unrestricted.
var qualityDefaults = map[string]struct {
	Mode         string
	VideoMbps    int
	AudioKbps    int
	VideoHeight  int
	AllowHDR     bool
	DefaultOrder int
}{
	"local":    {"original", 0, 0, 4320, true, 0},
	"wifi":     {"automatic", 0, 0, 2160, true, 1},
	"cellular": {"standard", 8, 256, 1080, false, 2},
	"unknown":  {"automatic", 20, 320, 1080, true, 3},
}

func qualityFields() []PreferenceField {
	networks := make([]string, 0, len(qualityDefaults))
	for network := range qualityDefaults {
		networks = append(networks, network)
	}
	sort.Slice(networks, func(i, j int) bool {
		return qualityDefaults[networks[i]].DefaultOrder < qualityDefaults[networks[j]].DefaultOrder
	})
	out := []PreferenceField{}
	for _, network := range networks {
		d := qualityDefaults[network]
		prefix := "quality." + network + "."
		out = append(out,
			prefEnum(prefix+"mode", d.Mode, qualityModes, ScopeDeviceClass),
			qualityPreset(prefix+"maxVideoBitrateMbps", d.VideoMbps, true),
			qualityPreset(prefix+"maxAudioBitrateKbps", d.AudioKbps, false),
			prefIntEnum(prefix+"maxVideoHeight", d.VideoHeight, videoHeights, ScopeDeviceClass),
			prefBool(prefix+"allowHDR", d.AllowHDR, ScopeDeviceClass),
		)
	}
	return out
}

var preferenceRegistry = func() []PreferenceField {
	out := []PreferenceField{
		prefIntEnum("playback.skipBackSeconds", 10, []int{5, 10, 15, 30}, ScopeProfileServer),
		prefIntEnum("playback.skipForwardSeconds", 30, []int{10, 15, 30, 60}, ScopeProfileServer),
		prefBool("playback.autoplayNext", true, ScopeProfileServer),
		prefIntEnum("playback.upNextCountdownSeconds", 10, []int{0, 5, 10, 15}, ScopeProfileServer),
		prefBool("playback.passoutProtection", true, ScopeProfileServer),
		prefInt("playback.passoutAfterEpisodes", 3, 2, 5, ScopeProfileServer),
		prefEnum("playback.introSkip", "ask", skipModes, ScopeProfileServer),
		prefEnum("playback.creditsSkip", "ask", skipModes, ScopeProfileServer),
		prefEnum("playback.recapSkip", "ask", skipModes, ScopeProfileServer),
		prefNumberEnum("playback.defaultSpeed", 1, playbackSpeeds, ScopeProfileServer),
		prefNumberEnum("music.defaultSpeed", 1, playbackSpeeds, ScopeProfileServer),
		prefNumberEnum("audiobooks.defaultSpeed", 1, playbackSpeeds, ScopeProfileServer),
		prefIntEnum("playback.sleepTimerMinutes", 0, []int{0, 30, 60, 90, 120}, ScopeProfileServer),
		prefInt("playback.startedThresholdPercent", 5, 1, 25, ScopeProfileServer),
		prefInt("playback.playedThresholdPercent", 95, 75, 100, ScopeProfileServer),
		languagePreference("playback.preferredAudioLanguages"),
		languagePreference("playback.preferredSubtitleLanguages"),
		// Subtitles without being asked: never; only the forced track beside audio in its own
		// language (the part of a film the audio does not carry); or always, in a preferred language.
		prefEnum("playback.subtitleMode", "forced", []string{"off", "forced", "always"}, ScopeProfileServer),
		prefEnum("playback.subtitleSize", "medium", []string{"small", "medium", "large", "extra-large"}, ScopeProfileServer),
		prefEnum("playback.subtitleBackground", "translucent", []string{"none", "translucent", "opaque"}, ScopeProfileServer),
		prefBool("playback.showSyncedLyrics", true, ScopeProfileServer),

		prefEnum("delivery.directPlay", "prefer", deliveryModes, ScopeDeviceClass),
		prefEnum("delivery.directStream", "allow", deliveryModes, ScopeDeviceClass),
		prefEnum("delivery.transcode", "allow", deliveryModes, ScopeDeviceClass),
	}
	out = append(out, qualityFields()...)
	out = append(out,
		prefEnum("music.audioNormalization", "off", []string{"off", "track", "album"}, ScopeProfileServer),
		prefInt("music.crossfadeSeconds", 0, 0, 12, ScopeProfileServer),
		prefBool("music.gapless", true, ScopeProfileServer),

		prefBool("privacy.pauseWatchHistory", false, ScopeProfileServer),
		prefBool("privacy.showActivityToMembers", true, ScopeProfileServer),
		prefBool("privacy.includeInWatchTogether", true, ScopeProfileServer),

		prefBool("search.rememberHistory", true, ScopeProfileServer),

		// No region.timeZone: every client shows times in the device's own zone.
		semanticPreference("region.locale", "locale", 35),
		prefEnum("region.hourCycle", "auto", []string{"auto", "h12", "h23"}, ScopeProfileServer),

		prefList("home.rowOrder", HomeLayoutMaxRows, HomeLayoutRowIDMaxLength, ScopeProfileServer),
		prefList("home.hiddenRowIds", HomeLayoutMaxRows, HomeLayoutRowIDMaxLength, ScopeProfileServer),

		prefBool("appearance.showBackdrops", true, ScopeDeviceClass),
		prefInt("appearance.cardSizePercent", 100, 75, 150, ScopeDeviceClass),
		prefBool("appearance.reduceMotion", false, ScopeDeviceClass),

		prefBool("notifications.badges", true, ScopeProfileServer),
	)
	for i := range out {
		out[i].Consumer = preferenceConsumers[out[i].Key]
	}
	return out
}()

var preferenceIndex = func() map[string]PreferenceField {
	out := make(map[string]PreferenceField, len(preferenceRegistry))
	for _, f := range preferenceRegistry {
		out[f.Key] = f
	}
	return out
}()

// PreferenceRegistry is the published field set. The slice is copied so a caller
// cannot reshape the server's own authority.
func PreferenceRegistry() []PreferenceField {
	out := make([]PreferenceField, len(preferenceRegistry))
	copy(out, preferenceRegistry)
	return out
}

// PreferenceFieldFor reports the registry entry for a dotted key.
func PreferenceFieldFor(key string) (PreferenceField, bool) {
	f, ok := preferenceIndex[key]
	return f, ok
}

// PreferenceValues is a flat map of dotted key to canonical value: bool, int64,
// float64, string or []string. Never a raw JSON number.
type PreferenceValues map[string]any

func (v PreferenceValues) value(key string) any {
	if raw, ok := v[key]; ok && raw != nil {
		return raw
	}
	f, ok := preferenceIndex[key]
	if !ok {
		return nil
	}
	canonical, _, e := canonicalPreference(f, f.Default)
	if e != nil {
		return nil
	}
	return canonical
}

func (v PreferenceValues) Bool(key string) bool {
	out, _ := v.value(key).(bool)
	return out
}
func (v PreferenceValues) Int(key string) int {
	switch n := v.value(key).(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
func (v PreferenceValues) Float(key string) float64 {
	switch n := v.value(key).(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	}
	return 0
}
func (v PreferenceValues) Text(key string) string {
	out, _ := v.value(key).(string)
	return out
}
func (v PreferenceValues) List(key string) []string {
	out, _ := v.value(key).([]string)
	if out == nil {
		return []string{}
	}
	return out
}

func numberOf(raw any) (float64, bool) {
	switch n := raw.(type) {
	case json.Number:
		f, e := n.Float64()
		return f, e == nil
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func allowedNumber(f PreferenceField, n float64) bool {
	for _, raw := range f.AllowedValues {
		if option, ok := raw.(PreferenceOption); ok {
			raw = option.Value
		}
		if v, ok := numberOf(raw); ok && v == n {
			return true
		}
	}
	return false
}

func safeValue(v string, max int) bool {
	return utf8.ValidString(v) && strings.TrimSpace(v) != "" && utf8.RuneCountInString(v) <= max && !strings.ContainsAny(v, "\n\r\t")
}

// canonicalPreference validates one submitted value against its registry entry.
// Numeric ranges clamp (and say so); every other violation is rejected, because
// silently rewriting an enum would hide a client bug behind working behaviour.
func canonicalPreference(f PreferenceField, raw any) (any, bool, error) {
	clamped := false
	switch f.Type {
	case "boolean":
		out, ok := raw.(bool)
		if !ok {
			return nil, false, ErrInvalid
		}
		return out, false, nil
	case "integer", "number":
		n, ok := numberOf(raw)
		if !ok {
			return nil, false, ErrInvalid
		}
		if f.Type == "integer" && n != float64(int64(n)) {
			return nil, false, ErrInvalid
		}
		if len(f.AllowedValues) > 0 {
			if !allowedNumber(f, n) {
				return nil, false, ErrInvalid
			}
		} else {
			if f.Min != nil && n < *f.Min {
				n, clamped = *f.Min, true
			}
			if f.Max != nil && n > *f.Max {
				n, clamped = *f.Max, true
			}
		}
		if f.Type == "integer" {
			return int64(n), clamped, nil
		}
		return n, clamped, nil
	case "string", "locale":
		out, ok := raw.(string)
		if !ok {
			return nil, false, ErrInvalid
		}
		if len(f.AllowedValues) > 0 {
			for _, allowed := range f.AllowedValues {
				if allowed == out {
					return out, false, nil
				}
			}
			return nil, false, ErrInvalid
		}
		if !safeValue(out, f.maxLength) {
			return nil, false, ErrInvalid
		}
		if f.Type == "locale" && out != "auto" {
			if strings.Contains(out, "_") {
				return nil, false, ErrInvalid
			}
			tag, err := language.Parse(out)
			if err != nil || tag == language.Und {
				return nil, false, ErrInvalid
			}
			out = tag.String()
		}
		return out, false, nil
	case "string-list", "languageList":
		items, ok := raw.([]any)
		if !ok {
			if typed, typedOK := raw.([]string); typedOK {
				items = make([]any, len(typed))
				for i, v := range typed {
					items[i] = v
				}
			} else {
				return nil, false, ErrInvalid
			}
		}
		if len(items) > f.maxItems {
			return nil, false, ErrInvalid
		}
		out, seen := make([]string, 0, len(items)), map[string]bool{}
		for _, item := range items {
			text, ok := item.(string)
			if f.Type == "languageList" {
				if strings.Contains(text, "_") {
					return nil, false, ErrInvalid
				}
				tag, err := language.Parse(text)
				if err != nil || tag == language.Und {
					return nil, false, ErrInvalid
				}
				text = tag.String()
			}
			if !ok || !safeValue(text, f.maxLength) || seen[text] {
				return nil, false, ErrInvalid
			}
			seen[text] = true
			out = append(out, text)
		}
		return out, false, nil
	}
	return nil, false, ErrInvalid
}

// decodePreferenceDocument reads a stored scope document, dropping keys that the
// current registry no longer publishes rather than failing the whole read.
func decodePreferenceDocument(raw string, scope string) (PreferenceValues, error) {
	out := PreferenceValues{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	var stored map[string]any
	if e := json.Unmarshal([]byte(raw), &stored); e != nil {
		return nil, errors.New("invalid saved preference document")
	}
	for key, value := range stored {
		f, ok := preferenceIndex[key]
		if !ok || !f.allows(scope) || value == nil {
			continue
		}
		canonical, _, e := canonicalPreference(f, value)
		if e != nil {
			continue
		}
		out[key] = canonical
	}
	return out, nil
}

// ViewerScopeKey is the durable owner tuple for preference documents. It matches
// ViewerKey but takes the viewer alone, so packages without a Principal (the
// personal-activity writer, for one) can read the same rows.
func ViewerScopeKey(v identity.Viewer) string {
	b, _ := json.Marshal([]string{v.Authority, v.AccountID, v.ProfileID})
	return string(b)
}

func preferenceScopeKey(v identity.Viewer, scope, device string) (string, error) {
	switch scope {
	case ScopeProfileServer:
		return "profile:" + ViewerScopeKey(v), nil
	case ScopeDeviceClass:
		if !validDeviceClass(device) {
			return "", ErrInvalid
		}
		return "device:" + ViewerScopeKey(v) + ":" + device, nil
	}
	return "", ErrInvalid
}

func validDeviceClass(device string) bool {
	return device == "web" || device == "mobile" || device == "television"
}

// PreferenceQuery is the read surface shared by *sql.DB and *sql.Tx, so the
// effective-value accessor works inside an existing write transaction.
type PreferenceQuery interface {
	QueryRow(query string, args ...any) *sql.Row
}

// EffectivePreferences merges the stored scope documents over the registry
// defaults for one viewer. deviceClass may be empty when the caller only needs
// profile-server fields; device-class documents are then skipped.
func EffectivePreferences(q PreferenceQuery, v identity.Viewer, deviceClass string) (PreferenceValues, map[string]string, []string, error) {
	values, source := PreferenceValues{}, map[string]string{}
	for _, f := range preferenceRegistry {
		canonical, _, e := canonicalPreference(f, f.Default)
		if e != nil {
			return nil, nil, nil, e
		}
		values[f.Key], source[f.Key] = canonical, "default"
	}
	for _, scope := range PreferenceScopes {
		if scope == ScopeDeviceClass && !validDeviceClass(deviceClass) {
			continue
		}
		key, e := preferenceScopeKey(v, scope, deviceClass)
		if e != nil {
			return nil, nil, nil, e
		}
		var raw string
		var revision int64
		e = q.QueryRow(`SELECT revision,body FROM console_documents WHERE scope=?`, key).Scan(&revision, &raw)
		if errors.Is(e, sql.ErrNoRows) {
			continue
		}
		if e != nil {
			return nil, nil, nil, e
		}
		stored, e := decodePreferenceDocument(raw, scope)
		if e != nil {
			return nil, nil, nil, e
		}
		for field, value := range stored {
			values[field], source[field] = value, scope
		}
	}
	clamped := preferencePolicy(values, source, deviceClass)
	return values, source, clamped, nil
}

// preferencePolicy applies the server's own limits after the viewer's choices.
// A cellular ladder only means something on a device class that has cellular.
// The clamp is reported only when it actually overrode a stored choice; a
// default the viewer never touched is not something to warn a client about.
func preferencePolicy(values PreferenceValues, source map[string]string, deviceClass string) []string {
	clamped := []string{}
	if deviceClass != "mobile" && values["quality.cellular.mode"] != "off" {
		if source["quality.cellular.mode"] != "default" {
			clamped = append(clamped, "quality.cellular.mode")
		}
		values["quality.cellular.mode"] = "off"
		source["quality.cellular.mode"] = "policy"
	}
	return clamped
}

// PreferenceOption carries the presentation label and the policy implied by a
// selected bitrate. Zero means Original (no bitrate limit), never a zero-rate stream.
type PreferenceOption struct {
	Value          int    `json:"value"`
	Label          string `json:"label"`
	MaxVideoHeight int    `json:"maxVideoHeight,omitempty"`
}

func VideoPresetHeight(mbps int) int {
	switch mbps {
	case 40, 20:
		return 2160
	case 12, 8:
		return 1080
	case 4, 2:
		return 720
	case 1:
		return 480
	}
	return 0
}
func qualityPreset(key string, def int, video bool) PreferenceField {
	f := field(key, "integer", def, ScopeDeviceClass)
	f.Step = numeric(1)
	choices := []int{0, 320, 256, 192, 128, 96}
	unit := "kbps"
	if video {
		choices = []int{0, 40, 20, 12, 8, 4, 2, 1}
		unit = "Mbps"
	}
	for _, value := range choices {
		option := PreferenceOption{Value: value, Label: fmt.Sprintf("%d %s", value, unit)}
		if value == 0 {
			option.Label = "Original"
		}
		if video {
			option.MaxVideoHeight = VideoPresetHeight(value)
		}
		f.AllowedValues = append(f.AllowedValues, option)
	}
	return f
}
func semanticPreference(key, kind string, maxLength int) PreferenceField {
	f := field(key, kind, "auto", ScopeProfileServer)
	f.maxLength = maxLength
	return f
}
func languagePreference(key string) PreferenceField {
	f := prefList(key, 8, 35, ScopeProfileServer)
	f.Type = "languageList"
	return f
}
