package operations

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestPreferenceRegistryIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range PreferenceRegistry() {
		if seen[f.Key] {
			t.Fatalf("duplicate registry key %q", f.Key)
		}
		seen[f.Key] = true
		if len(f.Scopes) == 0 || f.Group == "" || f.LabelKey != "preferences."+f.Key {
			t.Fatalf("incomplete field %+v", f)
		}
		for _, scope := range f.Scopes {
			if scope != ScopeProfileServer && scope != ScopeDeviceClass {
				t.Fatalf("unpublished scope %q on %q", scope, f.Key)
			}
		}
		// Every default must itself survive the field's own validation, or the
		// server would publish a value it would refuse on the way back in.
		if _, clamped, e := canonicalPreference(f, f.Default); e != nil || clamped {
			t.Fatalf("default is not acceptable for %q: %v clamped=%t", f.Key, e, clamped)
		}
	}
	for _, key := range []string{
		"playback.startedThresholdPercent", "playback.playedThresholdPercent", "delivery.transcode",
		"quality.cellular.maxVideoHeight", "music.crossfadeSeconds", "privacy.pauseWatchHistory",
		"search.rememberHistory", "region.hourCycle", "home.rowOrder",
		"appearance.cardSizePercent", "notifications.badges",
	} {
		if !seen[key] {
			t.Fatalf("plan field %q is missing from the registry", key)
		}
	}
	// The published registry must round-trip as JSON for the client contract.
	b, e := json.Marshal(PreferenceRegistryDocument{Revision: PreferenceRegistryRevision, Fields: PreferenceRegistry()})
	if e != nil || len(b) < 1000 {
		t.Fatalf("registry does not publish: %v", e)
	}
}

func TestPreferencePatchValidationAndClamping(t *testing.T) {
	s, p, a := consoleFixture(t)
	ctx := context.Background()
	// A number outside its range clamps and says so; the enum beside it is kept.
	out, e := s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeProfileServer, DeviceClass: "web", ExpectedRevision: 1, IdempotencyKey: "clamp", Values: PreferencePatch{
		"playback.startedThresholdPercent": 90,
		"playback.playedThresholdPercent":  10,
		"music.audioNormalization":         "album",
	}})
	if e != nil {
		t.Fatal(e)
	}
	if out.Effective.Int("playback.startedThresholdPercent") != 25 || out.Effective.Int("playback.playedThresholdPercent") != 75 {
		t.Fatalf("not clamped: %+v", out.Effective)
	}
	if out.Effective.Text("music.audioNormalization") != "album" {
		t.Fatal("enum lost")
	}
	if len(out.ClampedFields) != 2 || out.ClampedFields[0] != "playback.playedThresholdPercent" || out.ClampedFields[1] != "playback.startedThresholdPercent" {
		t.Fatalf("clampedFields not reported: %+v", out.ClampedFields)
	}
	var fields *ValidationError
	// Unknown key.
	_, e = s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeProfileServer, DeviceClass: "web", ExpectedRevision: 2, IdempotencyKey: "unknown", Values: PreferencePatch{"playback.warpDrive": true}})
	if !errors.As(e, &fields) || len(fields.Fields) != 1 || fields.Fields[0] != "values.playback.warpDrive" {
		t.Fatalf("unknown key was not named: %v", e)
	}
	// Known key in the wrong scope.
	_, e = s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeProfileServer, DeviceClass: "web", ExpectedRevision: 2, IdempotencyKey: "wrong-scope", Values: PreferencePatch{"appearance.reduceMotion": true}})
	if !errors.As(e, &fields) || fields.Fields[0] != "values.appearance.reduceMotion" {
		t.Fatalf("scope violation was not named: %v", e)
	}
	// Value outside an enumerated domain is refused rather than quietly rewritten.
	_, e = s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeProfileServer, DeviceClass: "web", ExpectedRevision: 2, IdempotencyKey: "bad-enum", Values: PreferencePatch{"playback.defaultSpeed": 3.5}})
	if !errors.As(e, &fields) || fields.Fields[0] != "values.playback.defaultSpeed" {
		t.Fatalf("enum violation was not named: %v", e)
	}
	// Wrong type.
	_, e = s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeProfileServer, DeviceClass: "web", ExpectedRevision: 2, IdempotencyKey: "bad-type", Values: PreferencePatch{"search.rememberHistory": "yes"}})
	if !errors.As(e, &fields) || fields.Fields[0] != "values.search.rememberHistory" {
		t.Fatalf("type violation was not named: %v", e)
	}
	// A duplicate entry in a list is a client bug, not a value to deduplicate.
	_, e = s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeProfileServer, DeviceClass: "web", ExpectedRevision: 2, IdempotencyKey: "bad-list", Values: PreferencePatch{"playback.preferredAudioLanguages": []any{"en", "en"}}})
	if !errors.As(e, &fields) || fields.Fields[0] != "values.playback.preferredAudioLanguages" {
		t.Fatalf("list violation was not named: %v", e)
	}
	// None of the rejections advanced the document.
	current, e := s.Preferences(ctx, p, "web", a)
	if e != nil || current.Documents[0].Revision != 2 {
		t.Fatalf("rejected patches moved the revision: %v %+v", e, current.Documents[0])
	}
}

func TestPreferenceDeviceClassPolicyClampsCellular(t *testing.T) {
	s, p, a := consoleFixture(t)
	ctx := context.Background()
	out, e := s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeDeviceClass, DeviceClass: "television", ExpectedRevision: 1, IdempotencyKey: "tv", Values: PreferencePatch{"quality.cellular.mode": "original"}})
	if e != nil {
		t.Fatal(e)
	}
	if out.Effective.Text("quality.cellular.mode") != "off" {
		t.Fatalf("a television kept a cellular ladder: %+v", out.Effective)
	}
	found := false
	for _, key := range out.ClampedFields {
		found = found || key == "quality.cellular.mode"
	}
	if !found {
		t.Fatalf("policy clamp not published: %+v", out.ClampedFields)
	}
	mobile, e := s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeDeviceClass, DeviceClass: "mobile", ExpectedRevision: 1, IdempotencyKey: "mobile", Values: PreferencePatch{"quality.cellular.mode": "original"}})
	if e != nil || mobile.Effective.Text("quality.cellular.mode") != "original" {
		t.Fatalf("a phone lost its cellular ladder: %v %+v", e, mobile.Effective)
	}
}

func TestPreferenceSnapshotPublishesRegistryAndDefaults(t *testing.T) {
	s, p, a := consoleFixture(t)
	out, e := s.Preferences(context.Background(), p, "web", a)
	if e != nil {
		t.Fatal(e)
	}
	if out.Registry.Revision != PreferenceRegistryRevision || len(out.Registry.Fields) != len(PreferenceRegistry()) {
		t.Fatalf("registry not published: %+v", out.Registry)
	}
	if len(out.Documents) != len(PreferenceScopes) {
		t.Fatalf("scope documents missing: %+v", out.Documents)
	}
	if len(out.Effective) != len(PreferenceRegistry()) {
		t.Fatal("effective values must cover every registry field")
	}
	if !out.Effective.Bool("search.rememberHistory") || out.Effective.Int("playback.playedThresholdPercent") != 95 || out.EffectiveSource["playback.playedThresholdPercent"] != "default" {
		t.Fatalf("defaults are wrong: %+v", out.Effective)
	}
	if list := out.Effective.List("home.rowOrder"); len(list) != 0 {
		t.Fatal("list default must be empty, not null")
	}
}

func TestEffectivePreferencesAccessorReadsWithoutHTTP(t *testing.T) {
	s, p, a := consoleFixture(t)
	ctx := context.Background()
	if _, e := s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeProfileServer, DeviceClass: "web", ExpectedRevision: 1, IdempotencyKey: "accessor", Values: PreferencePatch{"playback.startedThresholdPercent": 12}}); e != nil {
		t.Fatal(e)
	}
	values, source, _, e := EffectivePreferences(s.DB, p.Viewer, "")
	if e != nil || values.Int("playback.startedThresholdPercent") != 12 || source["playback.startedThresholdPercent"] != ScopeProfileServer {
		t.Fatalf("accessor did not read the stored value: %v %+v", e, values)
	}
}

// A profile document saved while region.timeZone was still a preference keeps
// reading: the retired key is left out, the keys beside it are kept, and the
// next save writes the document without it.
func TestStoredPreferenceForARetiredKeyIsIgnored(t *testing.T) {
	s, p, a := consoleFixture(t)
	ctx := context.Background()
	key, e := preferenceScopeKey(p.Viewer, ScopeProfileServer, "web")
	if e != nil {
		t.Fatal(e)
	}
	body := `{"region.timeZone":"America/Halifax","region.hourCycle":"h23","playback.startedThresholdPercent":12}`
	if _, e = s.DB.Exec(`INSERT INTO console_documents VALUES(?,?,?,?)`, key, 4, body, s.now()); e != nil {
		t.Fatal(e)
	}
	out, e := s.Preferences(ctx, p, "web", a)
	if e != nil {
		t.Fatalf("a document with a retired key did not read: %v", e)
	}
	if _, ok := out.Effective["region.timeZone"]; ok {
		t.Fatalf("retired key is effective: %+v", out.Effective)
	}
	if _, ok := out.EffectiveSource["region.timeZone"]; ok {
		t.Fatalf("retired key has a source: %+v", out.EffectiveSource)
	}
	if _, ok := out.Documents[0].Values["region.timeZone"]; ok || out.Documents[0].Revision != 4 {
		t.Fatalf("retired key is published in the document: %+v", out.Documents[0])
	}
	if out.Effective.Text("region.hourCycle") != "h23" || out.Effective.Int("playback.startedThresholdPercent") != 12 {
		t.Fatalf("the keys beside the retired one were lost: %+v", out.Effective)
	}
	values, source, _, e := EffectivePreferences(s.DB, p.Viewer, "")
	if e != nil || values.Text("region.hourCycle") != "h23" || source["region.timeZone"] != "" {
		t.Fatalf("accessor did not tolerate the retired key: %v %+v", e, values)
	}
	// A client that still sends the key is told which one, as for any unknown key.
	var fields *ValidationError
	_, e = s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeProfileServer, DeviceClass: "web", ExpectedRevision: 4, IdempotencyKey: "retired", Values: PreferencePatch{"region.timeZone": "UTC"}})
	if !errors.As(e, &fields) || len(fields.Fields) != 1 || fields.Fields[0] != "values.region.timeZone" {
		t.Fatalf("retired key was not named: %v", e)
	}
	// The next save drops it from storage.
	if _, e = s.ApplyPreferences(ctx, p, a, PreferenceChange{Scope: ScopeProfileServer, DeviceClass: "web", ExpectedRevision: 4, IdempotencyKey: "after", Values: PreferencePatch{"region.hourCycle": "h12"}}); e != nil {
		t.Fatal(e)
	}
	var stored string
	if e = s.DB.QueryRow(`SELECT body FROM console_documents WHERE scope=?`, key).Scan(&stored); e != nil {
		t.Fatal(e)
	}
	var saved map[string]any
	if e = json.Unmarshal([]byte(stored), &saved); e != nil {
		t.Fatal(e)
	}
	if _, ok := saved["region.timeZone"]; ok || saved["region.hourCycle"] != "h12" || len(saved) != 2 {
		t.Fatalf("the saved document kept the retired key: %s", stored)
	}
}

func TestPreferenceConsumersAndSemanticDomains(t *testing.T) {
	for _, f := range PreferenceRegistry() {
		if f.Consumer == "" {
			t.Fatalf("unconsumed preference %s", f.Key)
		}
		if f.Type == "integer" && (f.Step == nil || *f.Step != 1) {
			t.Fatalf("integer step missing: %s", f.Key)
		}
	}
	for _, key := range []string{"music.shuffleDefault", "music.repeatDefault", "navigation.sidebarCollapsed", "region.timeZone"} {
		if _, ok := PreferenceFieldFor(key); ok {
			t.Fatal("removed key survived", key)
		}
	}
	cases := []struct {
		key       string
		good, bad any
	}{
		{"region.locale", "fr-CA", "not_a_real_locale"},
		{"playback.preferredAudioLanguages", []string{"en", "fr-CA"}, []string{"en", "EN"}},
		{"quality.wifi.maxVideoBitrateMbps", 12, 13},
		{"quality.wifi.maxAudioBitrateKbps", 320, 321},
	}
	for _, c := range cases {
		f, _ := PreferenceFieldFor(c.key)
		if _, _, err := canonicalPreference(f, c.good); err != nil {
			t.Fatal(c.key, err)
		}
		if _, _, err := canonicalPreference(f, c.bad); err == nil {
			t.Fatal("bad value accepted", c.key)
		}
	}
	if f, _ := PreferenceFieldFor("region.locale"); f.Default != "auto" {
		t.Fatal("region.locale")
	}
	for _, f := range PreferenceRegistry() {
		if f.Type == "timeZone" {
			t.Fatal("a viewer time zone preference is published", f.Key)
		}
	}
	for _, key := range []string{"quality.wifi.maxVideoBitrateMbps", "quality.wifi.maxAudioBitrateKbps"} {
		f, _ := PreferenceFieldFor(key)
		for _, raw := range f.AllowedValues {
			option, ok := raw.(PreferenceOption)
			if !ok || option.Label == "" {
				t.Fatal("unlabelled quality choice", raw)
			}
			if option.Value > 0 && key == "quality.wifi.maxVideoBitrateMbps" && option.MaxVideoHeight == 0 {
				t.Fatal("missing implied resolution", option)
			}
		}
	}
	values := PreferenceValues{"music.gapless": false, "music.crossfadeSeconds": int64(6), "music.audioNormalization": "album"}
	effects := values.AudioEffects()
	if effects.Gapless || effects.CrossfadeSeconds != 6 || effects.Normalization != "album" {
		t.Fatal(effects)
	}
}
