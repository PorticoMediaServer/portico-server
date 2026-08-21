package main

import (
	"encoding/json"
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
	samples <- sample{Name: "home", Status: 200, Bytes: 300, Took: 10 * time.Millisecond}
	samples <- sample{Name: "search", Status: 200, Bytes: 700, Took: 20 * time.Millisecond}
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
}
