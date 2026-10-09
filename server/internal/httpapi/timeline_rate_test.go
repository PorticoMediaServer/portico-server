package httpapi

import (
	"fmt"
	"testing"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/playbackv1"
)

func rateCaller(device string) v1Caller {
	return v1Caller{Principal: identity.Principal{Viewer: identity.Viewer{ServerID: "server", Authority: "local", AccountID: "account", ProfileID: "profile"}}, DeviceID: device}
}

func TestTimelineRateBurstRefillAndDeviceIsolation(t *testing.T) {
	l := newTimelineLimiter()
	now := time.Now()
	caller := rateCaller("one")
	for i := 0; i < timelineReportBurst; i++ {
		if !l.allow(caller, now) {
			t.Fatal("manual burst was refused before its limit")
		}
	}
	for i := 0; i < 100; i++ {
		if l.allow(caller, now) {
			t.Fatal("device exceeded its burst")
		}
	}
	if !l.allow(rateCaller("two"), now) {
		t.Fatal("one device's flood refused another device")
	}
	caller.Hash = "rotated-token"
	caller.ProfileID = "another-profile"
	if l.allow(caller, now) {
		t.Fatal("token or profile rotation minted a new bucket")
	}
	caller.AccountID = "different-account"
	if !l.allow(caller, now) {
		t.Fatal("device ID collision crossed account boundaries")
	}
	caller = rateCaller("one")
	now = now.Add(time.Second)
	for i := 0; i < timelineReportsPerSecond; i++ {
		if !l.allow(caller, now) {
			t.Fatal("tokens did not refill at the declared cadence")
		}
	}
	if l.allow(caller, now) {
		t.Fatal("refill exceeded the declared cadence")
	}
	if !l.allow(caller, now.Add(timelineRateTTL)) {
		t.Fatal("idle bucket did not expire and recover")
	}
}

func TestTimelineRateLedgerIsBounded(t *testing.T) {
	l := newTimelineLimiter()
	now := time.Now()
	for i := 0; i < timelineRateEntries+100; i++ {
		if !l.allow(rateCaller(fmt.Sprintf("device-%d", i)), now) {
			t.Fatal("full accounting table refused a new legitimate device")
		}
	}
	if len(l.entries) != timelineRateEntries || l.order.Len() != timelineRateEntries {
		t.Fatal("rate ledger exceeded its memory bound")
	}
}

func TestTimelineHTTPRateRefusalDoesNotWriteAndOtherDeviceContinues(t *testing.T) {
	f := newV1Fixture(t, 2)
	var session playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "timeline-rate-session-0001"}, startBody(f.items[0], nil), 201, &session)
	path := "/v1/playback/sessions/" + session.ID + "/timeline"
	denied := 0
	lastAccepted := int64(0)
	for seq := int64(1); seq <= 100; seq++ {
		w := f.raw("POST", path, f.owner.AccessToken, nil, map[string]any{"seq": seq, "generation": session.Presentation.Generation, "state": "playing", "positionMs": seq * 100, "rate": 1})
		switch w.Code {
		case 204:
			lastAccepted = seq
		case 429:
			denied++
			if v1Code(w) != "rate_limited" || w.Header().Get("Retry-After") != "1" {
				t.Fatalf("rate refusal contract: %d %s %s", w.Code, w.Header(), w.Body.String())
			}
		default:
			t.Fatalf("timeline returned %d %s", w.Code, w.Body.String())
		}
		if w.Code == 429 {
			var recorded int64
			if err := f.db.QueryRow(`SELECT last_seq FROM playback_v1_sessions WHERE id=?`, session.ID).Scan(&recorded); err != nil {
				t.Fatal(err)
			}
			if recorded != lastAccepted {
				t.Fatal("rejected report wrote timeline evidence")
			}
		}
	}
	if denied == 0 {
		t.Fatal("rapid fresh reports were never throttled")
	}
	other := f.device("independent-timeline-device")
	var otherSession playbackv1.SessionView
	f.callAs(other.AccessToken, "POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "timeline-rate-session-0002"}, startBody(f.items[1], nil), 201, &otherSession)
	if w := f.raw("POST", "/v1/playback/sessions/"+otherSession.ID+"/timeline", other.AccessToken, nil, map[string]any{"seq": 1, "generation": otherSession.Presentation.Generation, "state": "playing", "positionMs": 100, "rate": 1}); w.Code != 204 {
		t.Fatalf("other device was affected: %d %s", w.Code, w.Body.String())
	}
	if _, err := f.db.Exec(`UPDATE authorization_session_families SET revoked=1 WHERE id=(SELECT family_id FROM authorization_family_tokens WHERE token_hash=?)`, identity.Digest(f.owner.AccessToken)); err != nil {
		t.Fatal(err)
	}
	if w := f.raw("POST", path, f.owner.AccessToken, nil, map[string]any{"seq": 101, "generation": session.Presentation.Generation, "state": "playing", "positionMs": 10100, "rate": 1}); w.Code != 401 {
		t.Fatalf("rate bucket bypassed or preceded authentication: %d", w.Code)
	}
}
