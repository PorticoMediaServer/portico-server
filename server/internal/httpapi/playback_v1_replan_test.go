package httpapi

import (
	"context"
	"encoding/json"
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/playbackv1"
)

// replanFixtureV1 is a v1 fixture whose server can repackage and convert (so a
// plan other than the original exists) without producing any media.
func replanFixtureV1(t *testing.T, films int) *v1Fixture {
	t.Helper()
	f := newV1Fixture(t, films)
	f.deps.Playback.ConfigureHLS(&playback.HLS{})
	f.v1.ReadyCheck = func(context.Context, playback.Session) (bool, error) { return true, nil }
	f.call("PUT", "/v1/me/devices/current/capabilities", nil, webCapabilities(), 204, nil)
	return f
}

// filmStreams gives a fixture film stream facts: a 1080p picture, English
// audio first and Japanese second, at the given whole bitrate.
func filmStreams(t *testing.T, f *v1Fixture, index int, bitsPerSecond int64) {
	t.Helper()
	token := f.records[index].Token
	if _, err := f.db.Exec(`UPDATE catalog_assets SET size=? WHERE token=?`, bitsPerSecond*600/8, token); err != nil {
		t.Fatal(err)
	}
	var size, modified int64
	if err := f.db.QueryRow(`SELECT size,modified_ns FROM catalog_assets WHERE token=?`, token).Scan(&size, &modified); err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = assets.PersistStreams(tx, token, size, modified, []assets.Stream{
		{Index: 0, Type: "video", Codec: "h264", Detail: assets.StreamDetail{Profile: "High", Level: 41, BitDepth: 8, Width: 1920, Height: 1080, FrameRate: 24, DynamicRange: assets.RangeSDR}},
		{Index: 1, Type: "audio", Codec: "aac", Channels: 2, Language: "eng", Default: true},
		{Index: 2, Type: "audio", Codec: "aac", Channels: 2, Language: "jpn"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// The presentation says which engine plays it and which quality is in force;
// the quality of a start that names none is the viewer's saved default for
// where the request comes from; automatic steps along the ladder in place.
func TestPlaybackV1QualityDefaultsAndAutomaticSteps(t *testing.T) {
	f := replanFixtureV1(t, 1)
	filmStreams(t, f, 0, 12_000_000)
	var s playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "quality-default-0001"}, startBody(f.items[0], nil), 201, &s)
	p := s.Presentation
	if p.Engine != playback.EngineNative || p.Quality == nil || p.Quality.Mode != "original" || p.Mode != "direct" || p.AudioTrackID != "a1" {
		t.Fatalf("default start: %+v quality %+v", p, p.Quality)
	}
	f.call("DELETE", "/v1/playback/sessions/"+s.ID, nil, nil, 204, nil)

	// The viewer saves automatic for home and 720p for everywhere else. A start
	// that names no quality follows the one for where it comes from.
	var saved map[string]any
	f.call("PATCH", "/v1/preferences", nil, map[string]any{"expectedRevision": 1, "idempotencyKey": "quality-home", "scope": "profile-device-class", "deviceClass": "web", "values": map[string]any{"quality.home": "automatic", "quality.remote": "720p"}}, 200, &saved)
	const home, away = "192.168.1.20:51000", "93.184.216.34:51000"
	start := func(peer, key string) playbackv1.SessionView {
		t.Helper()
		w := f.rawFrom(peer, "POST", "/v1/playback/sessions", f.owner.AccessToken, map[string]string{"Idempotency-Key": key, "X-Portico-Device-Class": "web"}, startBody(f.items[0], nil))
		var out playbackv1.SessionView
		if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("start from %s: %d %s", peer, w.Code, w.Body.String())
		}
		return out
	}
	remote := start(away, "quality-default-0002")
	if q := remote.Presentation.Quality; q == nil || q.Mode != "fixed" || q.ID != "720p" || remote.Presentation.Mode != "stream" || remote.Presentation.BitrateKbps > 4128 {
		t.Fatalf("away from home: %+v quality %+v", remote.Presentation, q)
	}
	f.call("DELETE", "/v1/playback/sessions/"+remote.ID, nil, nil, 204, nil)
	s = start(home, "quality-default-0003")
	if q := s.Presentation.Quality; q == nil || q.Mode != "automatic" || q.ID != "" || s.Presentation.Mode != "direct" {
		t.Fatalf("automatic starts at the original: %+v quality %+v", s.Presentation, q)
	}
	var options playbackv1.Options
	if w := f.rawFrom(home, "GET", "/v1/items/"+f.items[0]+"/playback-options", f.owner.AccessToken, map[string]string{"X-Portico-Device-Class": "web"}, nil); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &options) != nil {
		t.Fatalf("options: %d %s", w.Code, w.Body.String())
	}
	if options.Preferred == nil || options.Preferred.Quality.Mode != "automatic" || options.Plan == nil || options.Plan.Engine != playback.EngineNative || options.Plan.Quality.Mode != "automatic" {
		t.Fatalf("options preferred %+v plan %+v", options.Preferred, options.Plan)
	}
	if len(options.Qualities) != 6 || options.Qualities[0].Mode != "original" || options.Qualities[1].Mode != "automatic" || options.Qualities[2].ID != "1080p" || options.Qualities[5].ID != "360p" {
		t.Fatalf("quality menu %+v", options.Qualities)
	}

	// One step down: the same session, a new generation, converted to 1080p at
	// the ladder's rate from where the player is.
	path := "/v1/playback/sessions/" + s.ID
	var down playbackv1.SessionView
	f.call("PATCH", path, map[string]string{"If-Match": s.Revision}, map[string]any{"quality": map[string]any{"mode": "automatic", "step": "down"}, "positionMs": 61_000}, 200, &down)
	if down.ID != s.ID || down.Presentation.Generation != 2 || down.Presentation.Quality.ID != "1080p" || down.Presentation.Quality.Step != "" || down.Presentation.Mode != "stream" || down.Presentation.StartPositionMs != 61_000 {
		t.Fatalf("step down: %+v quality %+v", down.Presentation, down.Presentation.Quality)
	}
	if d := down.Presentation.Decision.Video; d == nil || d.Action != "transcode" || down.Presentation.BitrateKbps > 8192 {
		t.Fatalf("the step was not planned under its ceiling: %+v at %d kbps", d, down.Presentation.BitrateKbps)
	}
	// Naming automatic again is not a change; a second step goes on down.
	var same playbackv1.SessionView
	f.call("PATCH", path, map[string]string{"If-Match": down.Revision}, map[string]any{"quality": map[string]any{"mode": "automatic"}}, 200, &same)
	if same.Presentation.Generation != 2 || same.Presentation.Quality.ID != "1080p" {
		t.Fatalf("automatic named again moved the session: %+v", same.Presentation.Quality)
	}
	var lower, higher, up playbackv1.SessionView
	f.call("PATCH", path, map[string]string{"If-Match": same.Revision}, map[string]any{"quality": map[string]any{"mode": "automatic", "step": "down"}}, 200, &lower)
	if lower.Presentation.Quality.ID != "720p" || lower.Presentation.Generation != 3 {
		t.Fatalf("second step: %+v", lower.Presentation.Quality)
	}
	f.call("PATCH", path, map[string]string{"If-Match": lower.Revision}, map[string]any{"quality": map[string]any{"mode": "automatic", "step": "up"}}, 200, &higher)
	if higher.Presentation.Quality.ID != "1080p" {
		t.Fatalf("one step up: %+v", higher.Presentation.Quality)
	}
	f.call("PATCH", path, map[string]string{"If-Match": higher.Revision}, map[string]any{"quality": map[string]any{"mode": "automatic", "step": "up"}}, 200, &up)
	if up.Presentation.Quality.ID != "" || up.Presentation.Mode != "direct" || up.Presentation.Generation != 5 {
		t.Fatalf("back up to the original: %+v quality %+v", up.Presentation, up.Presentation.Quality)
	}
	// A client that chooses the step itself names it, and names the original
	// to come back from one.
	var named, back playbackv1.SessionView
	f.call("PATCH", path, map[string]string{"If-Match": up.Revision}, map[string]any{"quality": map[string]any{"mode": "automatic", "id": "480p"}}, 200, &named)
	if q := named.Presentation.Quality; q.Mode != "automatic" || q.ID != "480p" || named.Presentation.Mode != "stream" {
		t.Fatalf("a named step: %+v", q)
	}
	f.call("PATCH", path, map[string]string{"If-Match": named.Revision}, map[string]any{"quality": map[string]any{"mode": "automatic", "id": "original"}}, 200, &back)
	if q := back.Presentation.Quality; q.Mode != "automatic" || q.ID != "" || back.Presentation.Mode != "direct" {
		t.Fatalf("back to the original by name: %+v", q)
	}
	up = back
	// A fixed quality is a ceiling: this file is over 720p's, so it converts.
	var fixed playbackv1.SessionView
	f.call("PATCH", path, map[string]string{"If-Match": up.Revision}, map[string]any{"quality": map[string]any{"mode": "fixed", "id": "720p"}}, 200, &fixed)
	if q := fixed.Presentation.Quality; q.Mode != "fixed" || q.ID != "720p" || fixed.Presentation.Mode != "stream" || fixed.Presentation.BitrateKbps > 4128 {
		t.Fatalf("fixed quality: %+v at %d kbps", q, fixed.Presentation.BitrateKbps)
	}
	if w := f.raw("PATCH", path, f.owner.AccessToken, map[string]string{"If-Match": fixed.Revision}, map[string]any{"quality": map[string]any{"mode": "fixed", "id": "900p"}}); w.Code != 400 {
		t.Fatalf("an unpublished quality: %d %s", w.Code, w.Body.String())
	}
	if w := f.raw("POST", "/v1/playback/sessions", f.owner.AccessToken, map[string]string{"Idempotency-Key": "quality-step-start-01"}, startBody(f.items[0], map[string]any{"quality": map[string]any{"mode": "automatic", "step": "down"}})); w.Code != 400 {
		t.Fatalf("a start cannot step: %d %s", w.Code, w.Body.String())
	}
}

// A start the measured connection plainly cannot carry is refused with the
// quality that would fit; automatic lands on that quality instead.
func TestPlaybackV1ThroughputEstimate(t *testing.T) {
	f := replanFixtureV1(t, 1)
	filmStreams(t, f, 0, 20_000_000)
	w := f.raw("POST", "/v1/playback/sessions", f.owner.AccessToken, map[string]string{"Idempotency-Key": "throughput-start-001"}, startBody(f.items[0], map[string]any{"throughputKbps": 5000}))
	var refused struct {
		Error struct {
			Code   string `json:"code"`
			Reason string `json:"reason"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &refused)
	if w.Code != 422 || refused.Error.Code != "connection_too_slow" || refused.Error.Reason != "480p" {
		t.Fatalf("a 20 Mb/s original over a 5 Mb/s connection: %d %s", w.Code, w.Body.String())
	}
	var active int
	if err := f.db.QueryRow(`SELECT count(*) FROM playback_v1_sessions`).Scan(&active); err != nil || active != 0 {
		t.Fatalf("a refused start left a session: %d %v", active, err)
	}
	// Close enough is allowed: the estimate is not exact.
	var s playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "throughput-start-002"}, startBody(f.items[0], map[string]any{"throughputKbps": 17_000}), 201, &s)
	f.call("DELETE", "/v1/playback/sessions/"+s.ID, nil, nil, 204, nil)
	// Automatic with the same estimate starts on the step that fits.
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "throughput-start-003"}, startBody(f.items[0], map[string]any{"throughputKbps": 5000, "quality": map[string]any{"mode": "automatic"}}), 201, &s)
	if q := s.Presentation.Quality; q.Mode != "automatic" || q.ID != "480p" || s.Presentation.Mode != "stream" {
		t.Fatalf("automatic on a 5 Mb/s connection: %+v", q)
	}
	// Asking for the original on that connection mid-play is refused, and the
	// session plays on as it was.
	path := "/v1/playback/sessions/" + s.ID
	w = f.raw("PATCH", path, f.owner.AccessToken, map[string]string{"If-Match": s.Revision}, map[string]any{"quality": map[string]any{"mode": "original"}, "throughputKbps": 5000})
	if w.Code != 422 || v1Code(w) != "connection_too_slow" {
		t.Fatalf("original over a slow connection mid-play: %d %s", w.Code, w.Body.String())
	}
	var still playbackv1.SessionView
	f.call("GET", path, nil, nil, 200, &still)
	if still.Presentation.Generation != s.Presentation.Generation || still.Presentation.URL != s.Presentation.URL || still.State != "playing" {
		t.Fatalf("a refused change disturbed the session: %+v", still)
	}
}

// A player that could not play what it was given tells the session; the server
// remembers it for the file and re-plans the same session another way. The
// session keeps its queue and its place in it.
func TestPlaybackV1PlayerFailureReplansInPlace(t *testing.T) {
	f := replanFixtureV1(t, 2)
	body := map[string]any{"owner": "device", "segments": []any{map[string]any{"source": queueItems(f.items...)}}, "startPlayback": map[string]any{"state": "playing"}}
	var created playbackv1.QueueReply
	f.call("POST", "/v1/queues", map[string]string{"Idempotency-Key": "failure-queue-000001"}, body, 201, &created)
	s := created.Session
	if s == nil || s.Queue == nil || s.Presentation.Mode != "direct" {
		t.Fatalf("queue start %+v", s)
	}
	path := "/v1/playback/sessions/" + s.ID
	var replanned playbackv1.SessionView
	f.call("PATCH", path, map[string]string{"If-Match": s.Revision}, map[string]any{"failure": map[string]any{"code": "decode_error", "detail": "MEDIA_ERR_DECODE"}, "positionMs": 42_000}, 200, &replanned)
	if replanned.ID != s.ID || replanned.Queue == nil || replanned.Queue.QueueID != s.Queue.QueueID || replanned.Queue.EntryID != s.Queue.EntryID {
		t.Fatalf("the re-plan lost the queue: %+v", replanned.Queue)
	}
	if replanned.Presentation.Generation != 2 || replanned.Presentation.Mode != "stream" || replanned.Presentation.URL == s.Presentation.URL || replanned.Presentation.StartPositionMs != 42_000 || replanned.Presentation.Engine != playback.EngineNative {
		t.Fatalf("the failed original was not replaced: %+v", replanned.Presentation)
	}
	var queue playbackv1.QueueView
	f.call("GET", "/v1/queues/"+s.Queue.QueueID, nil, nil, 200, &queue)
	if queue.Current == nil || queue.Current.EntryID != s.Queue.EntryID || queue.Total != 2 {
		t.Fatalf("the queue moved: %+v", queue)
	}
	// The next start of the same file skips the way that failed.
	var again playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "failure-next-start-1"}, startBody(f.items[0], nil), 201, &again)
	if again.Presentation.Mode != "stream" {
		t.Fatalf("the failed way was offered again: %+v", again.Presentation)
	}
	// Each report climbs one way; when nothing is left the answer says so and
	// the session is left as it is.
	current := replanned
	for i := 0; i < 2; i++ {
		f.call("PATCH", path, map[string]string{"If-Match": current.Revision}, map[string]any{"failure": map[string]any{"code": "source_not_supported"}}, 200, &current)
	}
	if d := current.Presentation.Decision.Video; d == nil || d.Action != "transcode" {
		t.Fatalf("the last way is a conversion: %+v", current.Presentation.Decision)
	}
	if w := f.raw("PATCH", path, f.owner.AccessToken, map[string]string{"If-Match": current.Revision}, map[string]any{"failure": map[string]any{"code": "decode_error"}}); w.Code != 422 || v1Code(w) != "unsupported_media" {
		t.Fatalf("a failure with nothing left to try: %d %s", w.Code, w.Body.String())
	}
	if w := f.raw("PATCH", path, f.owner.AccessToken, map[string]string{"If-Match": current.Revision}, map[string]any{"failure": map[string]any{"code": "Not A Code"}}); w.Code != 400 {
		t.Fatalf("a malformed failure: %d %s", w.Code, w.Body.String())
	}
}

// The audio track a profile picks is remembered for the film by language and
// applied at the next start that names none.
func TestPlaybackV1RemembersTrackChoices(t *testing.T) {
	f := replanFixtureV1(t, 1)
	filmStreams(t, f, 0, 6_000_000)
	var s playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "remember-tracks-0001"}, startBody(f.items[0], nil), 201, &s)
	if s.Presentation.AudioTrackID != "a1" {
		t.Fatalf("the default track: %+v", s.Presentation)
	}
	var chosen playbackv1.SessionView
	f.call("PATCH", "/v1/playback/sessions/"+s.ID, map[string]string{"If-Match": s.Revision}, map[string]any{"audio": map[string]any{"trackId": "a2"}}, 200, &chosen)
	if chosen.Presentation.AudioTrackID != "a2" || chosen.Presentation.Generation != 2 {
		t.Fatalf("audio change: %+v", chosen.Presentation)
	}
	f.call("DELETE", "/v1/playback/sessions/"+s.ID, nil, nil, 204, nil)
	var language string
	if err := f.db.QueryRow(`SELECT audio_language FROM playback_track_memory`).Scan(&language); err != nil || language != "ja" {
		t.Fatalf("remembered %q %v", language, err)
	}
	var next playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "remember-tracks-0002"}, startBody(f.items[0], nil), 201, &next)
	if next.Presentation.AudioTrackID != "a2" {
		t.Fatalf("the remembered language was not applied: %+v", next.Presentation)
	}
	var options playbackv1.Options
	f.call("GET", "/v1/items/"+f.items[0]+"/playback-options", nil, nil, 200, &options)
	if options.Preferred == nil || options.Preferred.AudioID != "a2" || options.Preferred.Reason != "last_used" {
		t.Fatalf("options preferred %+v", options.Preferred)
	}
	// Another profile is not affected.
	member, _ := f.member()
	f.callAs(member, "PUT", "/v1/me/devices/current/capabilities", nil, webCapabilities(), 204, nil)
	if err := f.db.QueryRow(`SELECT count(*) FROM playback_track_memory`).Scan(new(int)); err != nil {
		t.Fatal(err)
	}
	var count int
	_ = f.db.QueryRow(`SELECT count(DISTINCT profile_id) FROM playback_track_memory`).Scan(&count)
	if count != 1 {
		t.Fatalf("%d profiles remembered a choice only one made", count)
	}
}

// The viewer's overrides travel in the capability document and change the plan.
func TestPlaybackV1CapabilityOverridesAndEngines(t *testing.T) {
	f := replanFixtureV1(t, 1)
	filmStreams(t, f, 0, 6_000_000)
	// A device that carries mpv beside its own player, set to mpv.
	caps := webCapabilities()
	native := nativeEngine(caps)
	mpv := map[string]any{}
	for k, v := range native {
		mpv[k] = v
	}
	mpv["name"] = "mpv"
	mpv["subtitleRendering"] = []string{"text", "ass", "pgs", "vobsub", "dvb"}
	caps["engines"].(map[string]any)["mpv"] = mpv
	caps["engineSetting"] = "mpv"
	caps["overrides"] = map[string]any{"forceStereo": true, "disableHdr": true}
	f.call("PUT", "/v1/me/devices/current/capabilities", nil, caps, 204, nil)
	var doc playbackv1.CapabilitiesDocument
	f.call("GET", "/v1/me/devices/current/capabilities", nil, nil, 200, &doc)
	if doc.Document.EngineSetting != "mpv" || !doc.Document.Overrides.ForceStereo || !doc.Document.Overrides.DisableHDR || doc.Document.Overrides.DisablePassthrough || doc.Document.Engines.MPV == nil {
		t.Fatalf("stored document %+v", doc.Document)
	}
	var s playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "engine-setting-00001"}, startBody(f.items[0], nil), 201, &s)
	if s.Presentation.Engine != playback.EngineMPV {
		t.Fatalf("the viewer's player setting was ignored: %+v", s.Presentation)
	}
	var options playbackv1.Options
	f.call("GET", "/v1/items/"+f.items[0]+"/playback-options", nil, nil, 200, &options)
	if options.Plan == nil || options.Plan.Engine != playback.EngineMPV {
		t.Fatalf("the preview names another engine than the start: %+v", options.Plan)
	}
	// A document that names an engine setting the server does not know is refused whole.
	caps["engineSetting"] = "vlc"
	if w := f.raw("PUT", "/v1/me/devices/current/capabilities", f.owner.AccessToken, nil, caps); w.Code != 400 {
		t.Fatalf("an unknown player setting: %d %s", w.Code, w.Body.String())
	}
	caps["engineSetting"] = "automatic"
	caps["version"] = 1
	if w := f.raw("PUT", "/v1/me/devices/current/capabilities", f.owner.AccessToken, nil, caps); w.Code != 400 {
		t.Fatalf("an old document version: %d %s", w.Code, w.Body.String())
	}
}
