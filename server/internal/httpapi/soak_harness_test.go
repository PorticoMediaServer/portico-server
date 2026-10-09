package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/server/internal/playbackv1"
)

func TestSoakConfigurationExposesStreamingLoad(t *testing.T) {
	t.Setenv("PORTICO_SOAK_SECONDS", "45")
	t.Setenv("PORTICO_SOAK_VIEWERS", "200")
	t.Setenv("PORTICO_SOAK_STREAMS", "110")
	t.Setenv("PORTICO_SOAK_EVENT_HOLDS", "0")
	t.Setenv("PORTICO_SOAK_STREAM_BYTES_PER_SECOND", "2097152")
	s := soakConfiguration()
	if s.seconds != 45 || s.viewers != 200 || s.streams != 110 || s.eventHolds != 0 || s.streamBytesPerSecond != 2097152 {
		t.Fatalf("load configuration: %+v", s)
	}
}

func TestSoakReportSeparatesCleanCapacityFromFaultTraffic(t *testing.T) {
	o := &soakObservations{codes: map[string]int{}, byLabel: map[string]int{}, caps: map[string]int{}}
	o.record("home", 200, 20*time.Millisecond, true, false)
	o.record("home", 503, 40*time.Millisecond, true, false)
	for n := 0; n < 100; n++ {
		o.record("home", 503, 5*time.Second, false, false)
	}
	o.record("home", 0, 5*time.Second, true, true)
	o.sampleStreams(40, 100, true, false)
	o.sampleStreams(100, 100, true, false)
	o.sampleStreams(110, 100, false, false)
	o.sampleStreams(100, 100, true, true)
	o.streamRead(0, 1<<20)
	o.streamRead(1, 512<<10)
	o.streamRead(1, 512<<10)
	o.streamRead(2, 100)
	o.record("recovery-home", 200, 10*time.Millisecond, true, false)
	r := o.snapshot()
	if r.cleanRequests != 3 || r.quietRefusals != 1 || r.faultedRefusals != 100 || r.cleanP95 != 40*time.Millisecond {
		t.Fatalf("fault traffic hid clean-capacity results: %+v", r)
	}
	if r.streamPeak != 110 || r.cleanStreamSamples != 3 || r.cleanStreamsAtTarget != 2 || r.recoveryStreamsAtTarget != 1 {
		t.Fatalf("configured workers mistaken for active bodies: %+v", r)
	}
	if r.cleanTargetStreakPeak != 1 || r.recoveryTargetStreakPeak != 1 || r.streamReaders != 2 || r.recoverySuccesses != 1 {
		t.Fatalf("stream progress or recovery hidden: %+v", r)
	}
}

func TestFixtureMediaCopiesCompleteBytes(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "source"), filepath.Join(root, "destination")
	want := bytes.Repeat([]byte("media-body\x00"), 100000)
	if err := os.WriteFile(source, want, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeFixtureMedia(destination, source); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("media copy incomplete: read=%v size=%d", err, len(got))
	}
}

func TestSoakNormalClientHonorsRetryAfterWithoutHidingRefusal(t *testing.T) {
	var attempts atomic.Int32
	var firstAt atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			firstAt.Store(time.Now().UnixNano())
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if elapsed := time.Since(time.Unix(0, firstAt.Load())); elapsed < time.Second {
			t.Errorf("retried before Retry-After: %s", elapsed)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	o := &soakObservations{codes: map[string]int{}, byLabel: map[string]int{}, caps: map[string]int{}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, _ := soakCall(ctx, server.Client(), server.URL, "fixture-token", "home", "GET", "/v1/home", nil, o, true)
	r := o.snapshot()
	if code != 200 || attempts.Load() != 2 || r.quietRefusals != 1 || r.clientRetries != 1 || r.logicalCleanRequests != 1 || r.logicalCleanFailures != 0 || r.logicalCleanP95 < time.Second {
		t.Fatalf("retry erased admission or wait evidence: status=%d attempts=%d report=%+v", code, attempts.Load(), r)
	}
}

func TestSoakCurrentPlaybackStopWithoutBody(t *testing.T) {
	f := newV1Fixture(t, 1)
	f.call("PUT", "/v1/me/devices/current/capabilities", nil, webCapabilities(), 204, nil)
	var session playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "soak-stop-regression-0001"}, startBody(f.items[0], nil), 201, &session)
	path := "/v1/playback/sessions/" + session.ID
	// The old harness advertised a JSON document but sent no document. The
	// optional-body contract correctly rejects that shape before stopping.
	if old := f.raw("DELETE", path, f.owner.AccessToken, map[string]string{"Content-Type": "application/json"}, nil); old.Code != 400 {
		t.Fatalf("old harness shape unexpectedly accepted: %d %s", old.Code, old.Body.String())
	}
	server := httptest.NewServer(f.handler)
	defer server.Close()
	o := &soakObservations{codes: map[string]int{}, byLabel: map[string]int{}, caps: map[string]int{}}
	for repeat := 0; repeat < 2; repeat++ {
		code, body := soakCall(context.Background(), server.Client(), server.URL, f.owner.AccessToken, "playback-stop", "DELETE", path, nil, o, true)
		if code != 204 {
			t.Fatalf("bodyless current-protocol stop: %d %s", code, body)
		}
	}
	if report := o.snapshot(); report.failures != 0 || report.logicalCleanFailures != 0 {
		t.Fatalf("valid optional-body stop was counted as a failure: %+v", report)
	}
}

func TestSoakCompletionPhasesRemainSeparateAcrossRetries(t *testing.T) {
	phase := "quiet"
	o := &soakObservations{codes: map[string]int{}, byLabel: map[string]int{}, caps: map[string]int{}, phaseAt: func() string { return phase }}
	o.record("home", 503, 20*time.Millisecond, true, false)
	phase = "fault"
	o.record("home", 503, 40*time.Millisecond, true, false)
	o.logical(503, time.Second, true, false)
	phase = "recovery"
	o.record("home", 200, 10*time.Millisecond, true, false)
	o.logical(200, 10*time.Millisecond, true, false)
	r := o.snapshot()
	if r.phases["quiet"].refusals != 1 || r.phases["quiet"].logicalRequests != 0 || r.phases["fault"].refusals != 1 || r.phases["fault"].logicalFailures != 1 || r.phases["recovery"].refusals != 0 || r.phases["recovery"].logicalFailures != 0 || r.phases["recovery"].logicalRequests != 1 {
		t.Fatalf("phase crossing hid overload or recovery: %+v", r.phases)
	}
}
