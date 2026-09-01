package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiagnosticsDeltaForSummarizesPressureChanges(t *testing.T) {
	before := json.RawMessage(`{
		"runtime": {"goroutines": 20, "heapAllocBytes": 1000, "numGc": 2},
		"sqlite": {"waitCount": 3, "waitDurationMillis": 10, "writeOperations": 7, "writeAttempts": 9, "lockRetries": 1, "lockRetryWaitMillis": 5, "walBytes": 100},
		"resources": {"status": "normal", "backgroundJobsDeferred": false, "activePlaybackSessions": 1, "activeTranscodeSessions": 0, "availableTranscodeSlots": 2, "queuedBackgroundJobs": 1, "runningBackgroundJobs": 0, "saturatedWorkloadLanes": [], "saturatedJobLanes": [], "signals": ["resource pressure is within configured limits"], "degradationActions": ["No graceful degradation action is active."]},
		"admission": {"searchRejected": 1, "downloadRejected": 2, "streamRejected": 3, "transcodeRejected": 0, "transcodeUserRejected": 1},
		"workloadLanes": [{"id": "browsing", "active": 1, "capacity": 96, "rejected": 2}],
		"jobLanes": [{"id": "write-heavy", "queued": 1, "running": 0}]
	}`)
	after := json.RawMessage(`{
		"runtime": {"goroutines": 24, "heapAllocBytes": 1300, "numGc": 4},
		"sqlite": {"waitCount": 8, "waitDurationMillis": 30, "writeOperations": 11, "writeAttempts": 15, "lockRetries": 3, "lockRetryWaitMillis": 25, "walBytes": 250},
		"resources": {"status": "overloaded", "backgroundJobsDeferred": true, "activePlaybackSessions": 3, "activeTranscodeSessions": 2, "availableTranscodeSlots": 0, "queuedBackgroundJobs": 4, "runningBackgroundJobs": 1, "saturatedWorkloadLanes": ["browsing"], "saturatedJobLanes": ["write-heavy"], "signals": ["browsing workload lane is saturated"], "degradationActions": ["Defer starting queued background jobs until critical lanes recover."]},
		"admission": {"searchRejected": 4, "downloadRejected": 3, "streamRejected": 5, "transcodeRejected": 2, "transcodeUserRejected": 4},
		"workloadLanes": [{"id": "browsing", "active": 96, "capacity": 96, "rejected": 5}],
		"jobLanes": [{"id": "write-heavy", "queued": 4, "running": 1}]
	}`)

	delta := diagnosticsDeltaFor(before, after)
	if delta == nil {
		t.Fatalf("expected diagnostics delta")
	}
	if delta.Runtime.Goroutines != 4 || delta.Runtime.HeapAllocBytes != 300 || delta.Runtime.NumGC != 2 {
		t.Fatalf("unexpected runtime delta: %#v", delta.Runtime)
	}
	if delta.SQLite.LockRetries != 2 || delta.SQLite.WaitDurationMillis != 20 || delta.SQLite.WALBytes != 150 {
		t.Fatalf("unexpected sqlite delta: %#v", delta.SQLite)
	}
	if delta.ResourceStatusBefore != "normal" || delta.ResourceStatusAfter != "overloaded" || !delta.BackgroundDeferredAfter {
		t.Fatalf("unexpected resource status delta: %#v", delta)
	}
	if delta.ActivePlaybackDelta != 2 || delta.ActiveTranscodeDelta != 2 || delta.AvailableTranscodeSlotsAfter != 0 {
		t.Fatalf("unexpected playback/transcode delta: %#v", delta)
	}
	if delta.WorkloadRejectedDelta["browsing"] != 3 {
		t.Fatalf("unexpected workload rejection delta: %#v", delta.WorkloadRejectedDelta)
	}
	if delta.AdmissionRejectedDelta.Search != 3 || delta.AdmissionRejectedDelta.Download != 1 || delta.AdmissionRejectedDelta.Stream != 2 || delta.AdmissionRejectedDelta.Transcode != 2 || delta.AdmissionRejectedDelta.TranscodeUser != 3 {
		t.Fatalf("unexpected admission rejection delta: %#v", delta.AdmissionRejectedDelta)
	}
	if delta.JobQueuedDelta["write-heavy"] != 3 || delta.JobRunningDelta["write-heavy"] != 1 {
		t.Fatalf("unexpected job backlog delta: queued=%#v running=%#v", delta.JobQueuedDelta, delta.JobRunningDelta)
	}
}

func TestDiagnosticsTimelinePointForCompactsSnapshots(t *testing.T) {
	raw := json.RawMessage(`{
		"runtime": {"goroutines": 12, "heapAllocBytes": 2048, "numGc": 3},
		"sqlite": {"waitCount": 4, "waitDurationMillis": 15, "lockRetries": 2, "lockRetryWaitMillis": 8, "walBytes": 512},
		"resources": {"status": "normal", "backgroundJobsDeferred": false, "activePlaybackSessions": 2, "activeTranscodeSessions": 1, "queuedBackgroundJobs": 3, "runningBackgroundJobs": 1},
		"admission": {"searchRejected": 1, "downloadRejected": 2, "streamRejected": 5, "transcodeRejected": 3, "transcodeUserRejected": 4},
		"workloadLanes": [{"id": "browsing", "active": 0, "capacity": 96, "rejected": 2}, {"id": "media", "active": 0, "capacity": 96, "rejected": 4}]
	}`)
	capturedAt := time.Date(2026, 5, 5, 0, 30, 0, 0, time.UTC)
	point, ok := diagnosticsTimelinePointFor(raw, capturedAt)
	if !ok {
		t.Fatalf("expected timeline point")
	}
	if point.CapturedAt != "2026-05-05T00:30:00Z" || point.Goroutines != 12 || point.HeapAllocBytes != 2048 || point.NumGC != 3 {
		t.Fatalf("unexpected runtime timeline point: %#v", point)
	}
	if point.SQLiteLockRetries != 2 || point.SQLiteWALBytes != 512 || point.WorkloadRejectionsTotal != 6 {
		t.Fatalf("unexpected pressure timeline point: %#v", point)
	}
	if point.AdmissionRejectionsTotal != 15 {
		t.Fatalf("unexpected admission timeline total: %#v", point)
	}
	if point.ActivePlaybackSessions != 2 || point.ActiveTranscodeSessions != 1 || point.QueuedBackgroundJobs != 3 || point.RunningBackgroundJobs != 1 {
		t.Fatalf("unexpected resource timeline point: %#v", point)
	}
}

func TestMinDuration(t *testing.T) {
	if got := minDuration(2*time.Second, 500*time.Millisecond); got != 500*time.Millisecond {
		t.Fatalf("minDuration shorter second = %s", got)
	}
	if got := minDuration(250*time.Millisecond, time.Second); got != 250*time.Millisecond {
		t.Fatalf("minDuration shorter first = %s", got)
	}
}

func TestScanDuringRunUsesExplicitReconcileMode(t *testing.T) {
	payload, err := json.Marshal(reconcileScanRequest())
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"mode":"reconcile"}` {
		t.Fatalf("scan request payload = %s", payload)
	}
}

func TestAcceptanceCredentialEnvironmentFallback(t *testing.T) {
	t.Setenv("PORTICO_ACCEPTANCE_LOGIN", "  benchmark-owner@example.test  ")
	if got := firstNonEmptyEnvironment("PORTICO_ACCEPTANCE_LOGIN", "admin"); got != "benchmark-owner@example.test" {
		t.Fatalf("credential environment login = %q", got)
	}
	t.Setenv("PORTICO_ACCEPTANCE_LOGIN", " ")
	if got := firstNonEmptyEnvironment("PORTICO_ACCEPTANCE_LOGIN", "admin"); got != "admin" {
		t.Fatalf("empty credential environment fallback = %q", got)
	}
}

func TestAcceptanceBearerTokenRequiresPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acceptance-token")
	if err := os.WriteFile(path, []byte("  test-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := acceptanceBearerToken(path, "")
	if err != nil {
		t.Fatalf("load private bearer token: %v", err)
	}
	if token != "test-secret" {
		t.Fatalf("loaded bearer token mismatch")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := acceptanceBearerToken(path, ""); err == nil {
		t.Fatal("world-readable bearer token file should be rejected")
	}
	if _, err := acceptanceBearerToken(path, "environment-secret"); err == nil {
		t.Fatal("simultaneous file and environment bearer token should be rejected")
	}
}

func TestVirtualUserUsesBearerWithoutLoginOrCSRF(t *testing.T) {
	loginRequests := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			loginRequests++
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("authorization header = %q", got)
		}
		if got := r.Header.Get(csrfHeaderName); got != "" {
			t.Errorf("bearer request unexpectedly included CSRF header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer testServer.Close()

	vu, err := newVirtualUser(testServer.URL, "", "", "test-secret", time.Second)
	if err != nil {
		t.Fatalf("new bearer virtual user: %v", err)
	}
	if sample := vu.doJSON("search", http.MethodPost, "/api/search", map[string]string{"query": "movie"}, nil); sample.Err != "" || sample.Status != http.StatusOK {
		t.Fatalf("bearer request failed: %#v", sample)
	}
	if body := vu.rawJSON("/api/read"); len(body) == 0 {
		t.Fatal("raw bearer request returned no body")
	}
	if loginRequests != 0 {
		t.Fatalf("bearer virtual user sent %d password login requests", loginRequests)
	}
}

func TestRunPlaybackUsesCanonicalSessionAndOperationScopedMediaGrant(t *testing.T) {
	var requests []string
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/playback-sessions":
			if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
				t.Errorf("playback create authorization = %q", got)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode playback create: %v", err)
			}
			profile, _ := body["clientProfile"].(map[string]any)
			if profile["capabilitySchemaVersion"] != "playback-capability-v2" {
				t.Errorf("playback capability profile = %#v", profile)
			}
			_, _ = w.Write([]byte(`{"sessionId":"session-1","sourceUrl":"/api/media/movie-1/stream","directPlay":true,"nextEventSequence":1,"generation":2,"mediaGrant":{"token":"grant-secret","expiresAt":"2099-01-01T00:00:00Z"},"timeline":{"durationSeconds":120}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/media/movie-1/stream":
			if got := r.Header.Get("Authorization"); got != "PorticoMedia grant-secret" {
				t.Errorf("media authorization = %q", got)
			}
			if got := r.Header.Get("Range"); got != "bytes=0-65535" {
				t.Errorf("media range = %q", got)
			}
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("media"))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/playback-sessions/session-1":
			if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
				t.Errorf("playback progress authorization = %q", got)
			}
			var body struct {
				EventSequence int64 `json:"eventSequence"`
				Generation    int64 `json:"generation"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode playback progress: %v", err)
			}
			if body.Generation != 2 || body.EventSequence < 1 || body.EventSequence > 2 {
				t.Errorf("playback progress authority = %#v", body)
			}
			_, _ = w.Write([]byte(`{"accepted":true,"highestEventSequence":` + string(rune('0'+body.EventSequence)) + `,"generation":2}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/playback-sessions/session-1":
			var body struct {
				RequestID string `json:"requestId"`
				Terminal  struct {
					Disposition   string `json:"disposition"`
					EventSequence int64  `json:"eventSequence"`
					Generation    int64  `json:"generation"`
				} `json:"terminal"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode playback stop: %v", err)
			}
			if body.RequestID == "" || body.Terminal.Disposition != "stopped" || body.Terminal.EventSequence != 3 || body.Terminal.Generation != 2 {
				t.Errorf("playback terminal authority = %#v", body)
			}
			_, _ = w.Write([]byte(`{"accepted":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer testServer.Close()

	vu, err := newVirtualUser(testServer.URL, "", "", "test-api-key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	samples := make(chan sample, 5)
	runPlayback(vu, "movie-1", samples, &playbackTracker{})
	close(samples)
	for got := range samples {
		if got.Err != "" || got.Status < 200 || got.Status >= 300 {
			t.Fatalf("playback sample failed: %#v", got)
		}
	}
	want := []string{
		"POST /api/playback-sessions",
		"GET /api/media/movie-1/stream",
		"PATCH /api/playback-sessions/session-1",
		"PATCH /api/playback-sessions/session-1",
		"DELETE /api/playback-sessions/session-1",
	}
	if len(requests) != len(want) {
		t.Fatalf("playback requests = %#v", requests)
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("playback request[%d] = %q, want %q", i, requests[i], want[i])
		}
	}
}

func TestParseServerTimingSeparatesServerQueueAndHandler(t *testing.T) {
	server, queue, handler, ok := parseServerTiming(`portico;dur=18.250, queue;dur=1.125, handler;dur=17.125`)
	if !ok || server != 18250*time.Microsecond || queue != 1125*time.Microsecond || handler != 17125*time.Microsecond {
		t.Fatalf("unexpected parsed timing: server=%s queue=%s handler=%s ok=%t", server, queue, handler, ok)
	}
	if _, _, _, ok := parseServerTiming(`portico;dur=invalid`); ok {
		t.Fatal("invalid Server-Timing duration should not be accepted")
	}
}

func TestParseLoadProfile(t *testing.T) {
	profile, scenarios, err := parseLoadProfile("browsing,search,stream,transcode")
	if err != nil {
		t.Fatalf("parse profile: %v", err)
	}
	if !profile.Browsing || !profile.Artwork || !profile.Search || !profile.Stream || !profile.Transcode {
		t.Fatalf("profile did not enable expected lanes: %#v", profile)
	}
	if profile.Playback || profile.Dashboard || profile.Download {
		t.Fatalf("profile enabled unexpected lanes: %#v", profile)
	}
	if len(scenarios) != 4 || scenarios[0] != "profile_browsing" || scenarios[1] != "profile_search" || scenarios[2] != "profile_stream" || scenarios[3] != "profile_transcode" {
		t.Fatalf("unexpected profile scenarios: %#v", scenarios)
	}

	mixed, _, err := parseLoadProfile("mixed")
	if err != nil {
		t.Fatalf("parse mixed profile: %v", err)
	}
	if !mixed.Browsing || !mixed.Search || !mixed.Dashboard || !mixed.Download || !mixed.Artwork || !mixed.Playback || mixed.Stream || mixed.Transcode {
		t.Fatalf("mixed profile mismatch: %#v", mixed)
	}

	if _, _, err := parseLoadProfile("unknown"); err == nil {
		t.Fatalf("unknown profile should fail")
	}
}

func TestBudgetFailuresSummarizesExceededBudgets(t *testing.T) {
	result := report{
		Errors:    2,
		P95Millis: 900,
		DiagnosticsDelta: &diagnosticsDelta{
			SQLite:                 sqliteDiagnosticsDelta{LockRetries: 3},
			WorkloadRejectedDelta:  map[string]uint64{"browsing": 2, "media": 1},
			AdmissionRejectedDelta: admissionRejectedDelta{Search: 1, Download: 1, Stream: 1, TranscodeUser: 1},
		},
	}
	failures := budgetFailures(result, loadBudgets{
		MaxErrors:              1,
		MaxP95Millis:           750,
		MaxSQLiteLockRetries:   1,
		MaxWorkloadRejections:  2,
		MaxAdmissionRejections: 2,
	})
	if len(failures) != 5 {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestBudgetFailuresTreatsZeroOptionalBudgetsAsDisabled(t *testing.T) {
	result := report{
		Errors:    0,
		P95Millis: 900,
		DiagnosticsDelta: &diagnosticsDelta{
			SQLite:                 sqliteDiagnosticsDelta{LockRetries: 3},
			WorkloadRejectedDelta:  map[string]uint64{"browsing": 2},
			AdmissionRejectedDelta: admissionRejectedDelta{Search: 3},
		},
	}
	if failures := budgetFailures(result, loadBudgets{}); len(failures) != 0 {
		t.Fatalf("unexpected disabled-budget failures: %#v", failures)
	}
}

func TestSummarizeReportsThroughput(t *testing.T) {
	samples := make(chan sample, 2)
	samples <- sample{Name: "home", Status: 200, Bytes: 300, Took: 10 * time.Millisecond, Server: 2 * time.Millisecond, Queue: 100 * time.Microsecond, Handler: 1900 * time.Microsecond, HasServerTiming: true}
	samples <- sample{Name: "search", Status: 200, Bytes: 700, Took: 20 * time.Millisecond, Server: 8 * time.Millisecond, Queue: 200 * time.Microsecond, Handler: 7800 * time.Microsecond, HasServerTiming: true}
	close(samples)

	started := time.Date(2026, 5, 5, 1, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Second)
	result := summarize("http://127.0.0.1:32500", 2, 2*time.Second, started, finished, samples)
	if result.Requests != 2 || result.Bytes != 1000 {
		t.Fatalf("unexpected summary totals: %#v", result)
	}
	if result.RequestsPerSecond != 1 || result.BytesPerSecond != 500 {
		t.Fatalf("unexpected throughput: requests/s=%f bytes/s=%f", result.RequestsPerSecond, result.BytesPerSecond)
	}
	if result.ServerP95Millis != 8 || result.QueueP95Millis != 0.2 || result.HandlerP95Millis != 7.8 {
		t.Fatalf("unexpected Server-Timing summary: server=%f queue=%f handler=%f", result.ServerP95Millis, result.QueueP95Millis, result.HandlerP95Millis)
	}
	if result.ByName["search"].AverageBytes != 700 || result.ByName["search"].P95Bytes != 700 || result.ByName["search"].P99Millis != 20 {
		t.Fatalf("unexpected per-route payload/latency summary: %#v", result.ByName["search"])
	}
}
