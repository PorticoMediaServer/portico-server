package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/playbackv1"
)

func startBody(item string, extra map[string]any) map[string]any {
	body := map[string]any{"itemId": item}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

// Slice 2: idempotent start, If-Match changes with generations, timeline
// fencing, stop fencing the grant, replacement and lease expiry (spec §16.1–4).
func TestPlaybackV1SessionLifecycle(t *testing.T) {
	f := newV1Fixture(t, 3)
	f.call("PUT", "/v1/me/devices/current/capabilities", nil, webCapabilities(), 204, nil)
	key := map[string]string{"Idempotency-Key": "start-key-0000000001"}

	var s playbackv1.SessionView
	w := f.call("POST", "/v1/playback/sessions", key, startBody(f.items[0], map[string]any{"startFrom": "beginning"}), 201, &s)
	if s.ID == "" || s.Revision != "1" || s.Kind != "vod" || s.Role != "local" || s.State != "playing" || s.ItemID != f.items[0] || s.VersionID == "" {
		t.Fatalf("session %+v", s)
	}
	if w.Header().Get("ETag") != `"1"` || w.Header().Get("Deprecation") != "" {
		t.Fatalf("headers %v", w.Header())
	}
	p := s.Presentation
	if p.Generation != 1 || p.Mode != "direct" || !strings.HasPrefix(p.URL, "/v1/media/") || p.Decision.Video == nil || p.Decision.Video.Action != "direct" {
		t.Fatalf("presentation %+v", p)
	}
	if s.Lease.ReportEveryMs != 10_000 || s.Lease.ExpiresAt == "" {
		t.Fatalf("lease %+v", s.Lease)
	}
	// The grant plays.
	if w = f.raw("GET", p.URL, "", map[string]string{"Range": "bytes=0-3"}, nil); w.Code != 206 && w.Code != 200 {
		t.Fatalf("media %d %s", w.Code, w.Body.String())
	}

	// Invariant 1: a replay returns the same session; another body with the key is 422.
	var replay playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", key, startBody(f.items[0], map[string]any{"startFrom": "beginning"}), 201, &replay)
	if replay.ID != s.ID {
		t.Fatalf("replay made %s, want %s", replay.ID, s.ID)
	}
	if w = f.raw("POST", "/v1/playback/sessions", f.owner.AccessToken, key, startBody(f.items[1], nil)); w.Code != 422 || v1Code(w) != "idempotency_key_reused" {
		t.Fatalf("reused key: %d %s", w.Code, w.Body.String())
	}
	// A short key is refused; no key at all is the deprecated legacy route.
	if w = f.raw("POST", "/v1/playback/sessions", f.owner.AccessToken, map[string]string{"Idempotency-Key": "short"}, startBody(f.items[1], nil)); w.Code != 400 {
		t.Fatalf("short key: %d", w.Code)
	}
	if w = f.raw("POST", "/v1/playback/sessions", f.owner.AccessToken, nil, map[string]any{"itemId": f.items[2], "quality": "auto", "requestId": "legacy"}); w.Code != 201 || w.Header().Get("Deprecation") != "true" {
		t.Fatalf("legacy start: %d %v", w.Code, w.Header())
	}

	// GET carries the revision ETag; another profile's device can't see it.
	w = f.call("GET", "/v1/playback/sessions/"+s.ID, nil, nil, 200, nil)
	if w.Header().Get("ETag") != `"1"` {
		t.Fatalf("get etag %q", w.Header().Get("ETag"))
	}

	// Invariant 2: PATCH needs If-Match; a stale one is 412 with the current session.
	path := "/v1/playback/sessions/" + s.ID
	if w = f.raw("PATCH", path, f.owner.AccessToken, nil, map[string]any{"state": "paused"}); w.Code != 428 {
		t.Fatalf("no If-Match: %d", w.Code)
	}
	w = f.raw("PATCH", path, f.owner.AccessToken, map[string]string{"If-Match": `"9"`}, map[string]any{"state": "paused"})
	var stale struct {
		Error struct {
			Code    string                 `json:"code"`
			Current playbackv1.SessionView `json:"current"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &stale)
	if w.Code != 412 || stale.Error.Code != "revision_mismatch" || stale.Error.Current.ID != s.ID || stale.Error.Current.Revision != "1" {
		t.Fatalf("stale PATCH: %d %s", w.Code, w.Body.String())
	}
	var paused playbackv1.SessionView
	f.call("PATCH", path, map[string]string{"If-Match": `"1"`}, map[string]any{"state": "paused"}, 200, &paused)
	if paused.State != "paused" || paused.Revision != "2" || paused.Presentation.Generation != 1 || paused.Presentation.URL != p.URL {
		t.Fatalf("paused %+v", paused)
	}
	// A quality change alters the bytes: a new generation with a new URL; the old grant is fenced.
	var changed playbackv1.SessionView
	f.call("PATCH", path, map[string]string{"If-Match": "2"}, map[string]any{"quality": map[string]any{"mode": "limit", "maxVideoBitrateKbps": 100000}}, 200, &changed)
	if changed.Presentation.Generation != 2 || changed.Presentation.URL == p.URL || changed.Revision != "3" {
		t.Fatalf("changed %+v", changed)
	}
	// NEW-35: a grant is a capability; an old one is a hidden 404, never a 401.
	if w = f.raw("GET", p.URL, "", nil, nil); w.Code != 404 {
		t.Fatalf("old generation's grant: %d %s", w.Code, w.Body.String())
	}

	// Invariant 3: stale generation or seq never changes state.
	timeline := path + "/timeline"
	report := func(seq int64, generation int, state string, position int64) *httptest.ResponseRecorder {
		return f.raw("POST", timeline, f.owner.AccessToken, nil, map[string]any{"seq": seq, "generation": generation, "state": state, "positionMs": position, "rate": 1})
	}
	if w = report(1, 2, "playing", 5_000); w.Code != 204 || w.Header().Get("Report-Every-Ms") != "10000" {
		t.Fatalf("timeline %d %v %s", w.Code, w.Header(), w.Body.String())
	}
	report(1, 2, "paused", 9_000) // same seq: ignored
	report(2, 1, "paused", 9_000) // old generation: ignored
	var got playbackv1.SessionView
	f.call("GET", path, nil, nil, 200, &got)
	if got.State != "playing" {
		t.Fatalf("stale report changed state: %+v", got)
	}
	if w = report(3, 2, "paused", 12_000); w.Header().Get("Report-Every-Ms") != "30000" {
		t.Fatalf("paused cadence %v", w.Header())
	}
	f.call("GET", path, nil, nil, 200, &got)
	if got.State != "paused" {
		t.Fatalf("state %s", got.State)
	}
	var position int64
	_ = f.db.QueryRow(`SELECT position_ms FROM playback_v1_sessions WHERE id=?`, s.ID).Scan(&position)
	if position != 12_000 {
		t.Fatalf("position %d", position)
	}

	// replacesSessionId ends the old session and takes its slot.
	var next playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "start-key-0000000002"}, startBody(f.items[1], map[string]any{"replacesSessionId": s.ID}), 201, &next)
	f.call("GET", path, nil, nil, 200, &got)
	if got.State != "ended" || next.ID == s.ID {
		t.Fatalf("replaced %+v next %+v", got, next)
	}
	if w = report(4, 2, "playing", 1); w.Code != 410 || v1Code(w) != "session_ended" {
		t.Fatalf("report after end: %d %s", w.Code, w.Body.String())
	}

	// Invariant 4: DELETE fences the grant at once and is idempotent.
	f.call("DELETE", "/v1/playback/sessions/"+next.ID, nil, map[string]any{"positionMs": 42_000}, 204, nil)
	if w = f.raw("GET", next.Presentation.URL, "", nil, nil); w.Code < 400 {
		t.Fatalf("stopped grant still plays: %d", w.Code)
	}
	f.call("DELETE", "/v1/playback/sessions/"+next.ID, nil, nil, 204, nil)
	if w = f.raw("DELETE", "/v1/playback/sessions/nope", f.owner.AccessToken, nil, nil); w.Code != 204 && w.Code != 404 {
		t.Fatalf("unknown delete: %d", w.Code)
	}

	// Lease expiry ends a session and fences its grant: the sweeper wakes at the
	// earliest lease (it sleeps, it doesn't poll).
	f.v1.LeaseDuration = 150 * time.Millisecond
	var lapsing playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "start-key-0000000003"}, startBody(f.items[2], nil), 201, &lapsing)
	f.v1.LeaseDuration = 0
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ended int64
		_ = f.db.QueryRow(`SELECT ended_ms FROM playback_v1_sessions WHERE id=?`, lapsing.ID).Scan(&ended)
		if ended > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lapsed session was not swept")
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.call("GET", "/v1/playback/sessions/"+lapsing.ID, nil, nil, 200, &got)
	if got.State != "ended" {
		t.Fatalf("lapsed session %+v", got)
	}
	if w = f.raw("GET", lapsing.Presentation.URL, "", nil, nil); w.Code < 400 {
		t.Fatalf("lapsed grant still plays: %d", w.Code)
	}

	// Events: the device heard about each change.
	var events int
	_ = f.db.QueryRow(`SELECT count(*) FROM api_events WHERE type='session.updated' AND audience LIKE 'device:%'`).Scan(&events)
	if events < 6 {
		t.Fatalf("%d session events", events)
	}
}

// B8b: an ended session is kept for the longest playback history period, then
// pruned by the v1 sweep; a live session and a recent one stay.
func TestPlaybackV1EndedSessionsArePrunedPastTheHistory(t *testing.T) {
	f := newV1Fixture(t, 1)
	f.call("PUT", "/v1/me/devices/current/capabilities", nil, webCapabilities(), 204, nil)
	var live, recent, old playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "prune-live-000000001"}, startBody(f.items[0], nil), 201, &live)
	other := f.device("second-device")
	f.callAs(other.AccessToken, "PUT", "/v1/me/devices/current/capabilities", nil, webCapabilities(), 204, nil)
	f.callAs(other.AccessToken, "POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "prune-recent-0000001"}, startBody(f.items[0], nil), 201, &recent)
	f.callAs(other.AccessToken, "DELETE", "/v1/playback/sessions/"+recent.ID, nil, nil, 204, nil)
	third := f.device("third-device")
	f.callAs(third.AccessToken, "PUT", "/v1/me/devices/current/capabilities", nil, webCapabilities(), 204, nil)
	f.callAs(third.AccessToken, "POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "prune-old-000000001"}, startBody(f.items[0], nil), 201, &old)
	f.callAs(third.AccessToken, "DELETE", "/v1/playback/sessions/"+old.ID, nil, nil, 204, nil)
	past := time.Now().Add(-playbackv1.SessionRetention - time.Hour).UnixMilli()
	if _, err := f.db.Exec(`UPDATE playback_v1_sessions SET created_ms=?,ended_ms=? WHERE id=?`, past-1000, past, old.ID); err != nil {
		t.Fatal(err)
	}
	f.v1.SweepQueues(context.Background())
	for id, want := range map[string]int{live.ID: 1, recent.ID: 1, old.ID: 0} {
		var n int
		if err := f.db.QueryRow(`SELECT count(*) FROM playback_v1_sessions WHERE id=?`, id).Scan(&n); err != nil || n != want {
			t.Fatalf("session %s: %d rows, want %d (%v)", id, n, want, err)
		}
	}
}

// B8b: a timeline's skip report is marker evidence (spec §4.2): recorded once
// per session and marker, never for an automatic skip of a marker the server did
// not authorize for unattended skipping, and never for another item's marker.
func TestPlaybackV1SkipReportsAreMarkerEvidence(t *testing.T) {
	f := newV1Fixture(t, 2)
	asset, other := f.records[0].Token, f.records[1].Token
	for _, q := range []string{
		`INSERT OR IGNORE INTO library_sources(id,library_id,configured_root,root,incarnation,generation) VALUES('skip-source','` + f.library + `','/isolated/skips','/isolated/skips','incarnation',1)`,
		`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns) VALUES('object','skip-source','` + asset + `','incarnation','a.mp4','revision','{}',1,1),('other-object','skip-source','` + other + `','incarnation','b.mp4','revision','{}',1,1)`,
		`INSERT INTO analysis_markers(id,object_id,source_revision,source_binding,kind,start_us,end_us,confidence,provenance,result_id,approved) VALUES
 ('safe-intro','object','revision','binding','intro',5000000,35000000,.9,'luminance_opening_boundary:v1','result',0),
 ('unsafe-recap','object','revision','binding','recap',0,4000000,.85,'embedded_chapter_label:v1','result',0),
 ('elsewhere','other-object','revision','binding','intro',0,4000000,.9,'luminance_opening_boundary:v1','result',0)`,
	} {
		if _, err := f.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	drainPlaybackCatalogue(t, f.db)
	var evidence []string
	f.v1.SkipEvidence = func(_ context.Context, code string, _ map[string]int64) { evidence = append(evidence, code) }
	f.call("PUT", "/v1/me/devices/current/capabilities", nil, webCapabilities(), 204, nil)
	var s playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "skip-start-000000001"}, startBody(f.items[0], nil), 201, &s)
	seq := int64(0)
	report := func(skipped map[string]any) int {
		seq++
		return f.raw("POST", "/v1/playback/sessions/"+s.ID+"/timeline", f.owner.AccessToken, nil, map[string]any{"seq": seq, "generation": s.Presentation.Generation, "state": "playing", "positionMs": 35_000, "skipped": skipped}).Code
	}
	for _, k := range []map[string]any{
		{"markerId": "safe-intro", "mode": "automatic", "positionMs": 5_000},
		{"markerId": "safe-intro", "mode": "manual", "positionMs": 9_000}, // a retry: the first record stands
		{"markerId": "unsafe-recap", "mode": "automatic", "positionMs": 0},
		{"markerId": "elsewhere", "mode": "manual", "positionMs": 0},
	} {
		if code := report(k); code != 204 {
			t.Fatalf("report %v: %d", k, code)
		}
	}
	if code := report(map[string]any{"markerId": "safe-intro", "mode": "sometimes", "positionMs": 0}); code != 400 {
		t.Fatalf("an invalid skip report was accepted: %d", code)
	}
	if code := report(map[string]any{"markerId": "unsafe-recap", "mode": "manual", "positionMs": 1_000}); code != 204 {
		t.Fatalf("manual skip: %d", code)
	}
	rows, err := f.db.Query(`SELECT marker_id,mode,position_us FROM playback_marker_skips WHERE playback_id=? ORDER BY marker_id`, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var id, mode string
		var at int64
		_ = rows.Scan(&id, &mode, &at)
		got = append(got, id+"/"+mode+"/"+strconv.FormatInt(at, 10))
	}
	rows.Close()
	if strings.Join(got, ",") != "safe-intro/automatic/5000000,unsafe-recap/manual/1000000" {
		t.Fatalf("recorded skips %v", got)
	}
	if strings.Join(evidence, ",") != "marker_skip_automatic_intro,marker_skip_manual_recap" {
		t.Fatalf("diagnostics evidence %v", evidence)
	}
}

// The first start on a restored database ends every session of the restored
// state with reason "restored" and revokes its media: nothing minted before the
// backup plays on.
func TestPlaybackV1RestoreEndsSessionsAndRevokesMedia(t *testing.T) {
	f := newV1Fixture(t, 1)
	f.call("PUT", "/v1/me/devices/current/capabilities", nil, webCapabilities(), 204, nil)
	var s playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "restore-start-00000001"}, startBody(f.items[0], nil), 201, &s)
	if w := f.raw("GET", s.Presentation.URL, "", nil, nil); w.Code >= 400 {
		t.Fatalf("media before the restore: %d", w.Code)
	}
	tx, err := f.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = playbackv1.RetireRestored(context.Background(), tx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if w := f.raw("GET", s.Presentation.URL, "", nil, nil); w.Code < 400 {
		t.Fatalf("the restored grant still serves media: %d", w.Code)
	}
	var got playbackv1.SessionView
	f.call("GET", "/v1/playback/sessions/"+s.ID, nil, nil, 200, &got)
	if got.State != "ended" || got.End == nil || got.End.Reason != playbackv1.EndReasonRestored {
		t.Fatalf("the session did not end as restored: %+v", got)
	}
	w := f.raw("POST", "/v1/playback/sessions/"+s.ID+"/timeline", f.owner.AccessToken, nil, map[string]any{"seq": 1, "generation": s.Presentation.Generation, "state": "playing", "positionMs": 1000, "rate": 1})
	if w.Code < 400 {
		t.Fatalf("a restored session accepted a timeline report: %d", w.Code)
	}
}
