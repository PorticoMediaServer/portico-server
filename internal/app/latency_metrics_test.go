package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func TestBoundedLatencyHistogramPublishesStablePercentiles(t *testing.T) {
	var histogram boundedLatencyHistogram
	for _, duration := range []time.Duration{
		500 * time.Microsecond,
		2 * time.Millisecond,
		8 * time.Millisecond,
		20 * time.Millisecond,
		90 * time.Millisecond,
		400 * time.Millisecond,
		2 * time.Second,
		70 * time.Second,
	} {
		histogram.observe(duration)
	}
	snapshot := histogram.snapshot()
	if snapshot.Count != 8 {
		t.Fatalf("count = %d, want 8", snapshot.Count)
	}
	if snapshot.P50Millis != 25 || snapshot.P95Millis != 60001 || snapshot.P99Millis != 60001 {
		t.Fatalf("unexpected percentile snapshot: %+v", snapshot)
	}
	if snapshot.MaximumMillis != 70000 {
		t.Fatalf("maximum = %d, want 70000", snapshot.MaximumMillis)
	}
}

func TestTrackedSQLRowsObserveCompletedConsumptionSpan(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	if _, err := db.Exec(`INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at) VALUES ('latency-read', 'Latency Read', 'movies', 999, '/tmp/latency-read', '{}', '2026-08-31T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	server.sqliteMetrics = SQLiteDiagnostics{}
	server.latencyMetrics = latencyMetricsRegistry{}
	rows, err := server.queryUserRead(context.Background(), `SELECT id FROM libraries WHERE id = 'latency-read'`)
	if err != nil {
		t.Fatal(err)
	}
	if reads := server.sqliteDiagnostics().ReadOperations; reads != 0 {
		t.Fatalf("read recorded before result consumption: %d", reads)
	}
	time.Sleep(12 * time.Millisecond)
	if !rows.Next() {
		t.Fatal("expected one row")
	}
	var id string
	if err := rows.Scan(&id); err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Fatal("expected exactly one row")
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	diagnostics := server.sqliteDiagnostics()
	if diagnostics.ReadOperations != 1 {
		t.Fatalf("completed read operations = %d, want 1", diagnostics.ReadOperations)
	}
	if diagnostics.SlowestReadMillis < 10 {
		t.Fatalf("completed read span = %dms, want row-consumption delay included", diagnostics.SlowestReadMillis)
	}
	if diagnostics.ReadLatency[0].Count != 1 {
		t.Fatalf("user read histogram = %+v", diagnostics.ReadLatency[0])
	}
}

func TestRequestTimingPublishesServerTimingBeforeEmptyResponseCommit(t *testing.T) {
	server := &Server{workloadLanes: newWorkloadLanes()}
	handler := server.requestTiming(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		timing, ok := request.Context().Value(requestTimingContextKey{}).(*requestTimingState)
		if !ok || timing == nil {
			t.Fatal("request timing state was not attached")
		}
		timing.queueNanos.Store((2 * time.Millisecond).Nanoseconds())
		time.Sleep(4 * time.Millisecond)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	timing := recorder.Header().Get("Server-Timing")
	for _, field := range []string{"portico;dur=", "queue;dur=2.000", "handler;dur="} {
		if !strings.Contains(timing, field) {
			t.Fatalf("Server-Timing %q missing %q", timing, field)
		}
	}
}

func TestResponseCompressionNegotiatesBoundedJSONAndPreservesTiming(t *testing.T) {
	server := &Server{workloadLanes: newWorkloadLanes()}
	payload := strings.Repeat(`{"title":"Portico dashboard telemetry","value":42}`, 250)
	handler := server.requestTiming(server.responseCompression(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", "99999")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, payload)
	})))
	request := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	request.Header.Set("Accept-Encoding", "br, gzip;q=1.0")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := recorder.Header().Get("Content-Length"); got != "" {
		t.Fatalf("compressed Content-Length = %q, want empty", got)
	}
	if got := recorder.Header().Values("Vary"); len(got) != 1 || got[0] != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	if !strings.Contains(recorder.Header().Get("Server-Timing"), "handler;dur=") {
		t.Fatalf("Server-Timing = %q", recorder.Header().Get("Server-Timing"))
	}
	reader, err := gzip.NewReader(bytes.NewReader(recorder.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if string(decoded) != payload {
		t.Fatalf("decoded payload length = %d, want %d", len(decoded), len(payload))
	}
	if recorder.Body.Len() >= len(payload)/3 {
		t.Fatalf("compressed bytes = %d, want materially below %d", recorder.Body.Len(), len(payload))
	}
}

func TestResponseCompressionGuardrails(t *testing.T) {
	largeJSON := []byte(strings.Repeat(`{"reflected":"attacker controlled","secret":"server value"}`, 80))
	tests := []struct {
		name            string
		method          string
		path            string
		acceptEncoding  string
		requestRange    string
		status          int
		contentType     string
		contentEncoding string
		contentLength   string
		body            []byte
		flush           bool
		wantCompressed  bool
		wantVary        bool
		wantLength      string
	}{
		{name: "small response", method: http.MethodGet, path: "/api/system", acceptEncoding: "gzip", status: http.StatusOK, contentType: "application/json", contentLength: "11", body: []byte(`{"ok":true}`), wantVary: true, wantLength: "11"},
		{name: "identity negotiation", method: http.MethodGet, path: "/api/dashboard", acceptEncoding: "gzip;q=0", status: http.StatusOK, contentType: "application/json", body: largeJSON, wantVary: true},
		{name: "zero decimal negotiation", method: http.MethodGet, path: "/api/dashboard", acceptEncoding: "gzip;q=0.000", status: http.StatusOK, contentType: "application/json", body: largeJSON, wantVary: true},
		{name: "malformed negotiation", method: http.MethodGet, path: "/api/dashboard", acceptEncoding: "gzip;q=not-a-number", status: http.StatusOK, contentType: "application/json", body: largeJSON, wantVary: true},
		{name: "auth secret", method: http.MethodPost, path: "/api/auth/sessions", acceptEncoding: "gzip", status: http.StatusOK, contentType: "application/json", body: largeJSON},
		{name: "playback credential", method: http.MethodGet, path: "/api/playback/receivers", acceptEncoding: "gzip", status: http.StatusOK, contentType: "application/json", body: largeJSON},
		{name: "profile administration proof", method: http.MethodPost, path: "/api/account/profile-admin-proofs", acceptEncoding: "gzip", status: http.StatusOK, contentType: "application/json", body: largeJSON},
		{name: "remote coordination", method: http.MethodGet, path: "/api/remote-access/status", acceptEncoding: "gzip", status: http.StatusOK, contentType: "application/json", body: largeJSON},
		{name: "media authorization", method: http.MethodGet, path: "/api/media/item/playback", acceptEncoding: "gzip", status: http.StatusOK, contentType: "application/json", body: largeJSON},
		{name: "watch together credential", method: http.MethodGet, path: "/api/watch-with-friends/groups", acceptEncoding: "gzip", status: http.StatusOK, contentType: "application/json", body: largeJSON},
		{name: "error reflection", method: http.MethodGet, path: "/api/search", acceptEncoding: "gzip", status: http.StatusBadRequest, contentType: "application/json", body: largeJSON},
		{name: "range request", method: http.MethodGet, path: "/api/dashboard", acceptEncoding: "gzip", requestRange: "bytes=0-1023", status: http.StatusOK, contentType: "application/json", body: largeJSON, wantVary: true},
		{name: "partial response", method: http.MethodGet, path: "/api/dashboard", acceptEncoding: "gzip", status: http.StatusPartialContent, contentType: "application/json", body: largeJSON, wantVary: true},
		{name: "head response", method: http.MethodHead, path: "/api/dashboard", acceptEncoding: "gzip", status: http.StatusOK, contentType: "application/json", body: largeJSON, wantVary: true},
		{name: "preencoded", method: http.MethodGet, path: "/api/dashboard", acceptEncoding: "gzip", status: http.StatusOK, contentType: "application/json", contentEncoding: "br", body: largeJSON, wantVary: true},
		{name: "binary", method: http.MethodGet, path: "/api/media/item/stream", acceptEncoding: "gzip", status: http.StatusOK, contentType: "video/mp4", body: largeJSON},
		{name: "stream", method: http.MethodGet, path: "/api/events", acceptEncoding: "gzip", status: http.StatusOK, contentType: "text/event-stream", body: largeJSON, flush: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := (&Server{}).responseCompression(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				if test.contentEncoding != "" {
					w.Header().Set("Content-Encoding", test.contentEncoding)
				}
				if test.contentLength != "" {
					w.Header().Set("Content-Length", test.contentLength)
				}
				w.WriteHeader(test.status)
				_, _ = w.Write(test.body)
				if test.flush {
					flusher, ok := w.(http.Flusher)
					if !ok {
						t.Fatal("compression writer did not preserve http.Flusher")
					}
					flusher.Flush()
				}
			}))
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Header.Set("Accept-Encoding", test.acceptEncoding)
			if test.requestRange != "" {
				request.Header.Set("Range", test.requestRange)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if got := recorder.Header().Get("Content-Encoding"); (got == "gzip") != test.wantCompressed {
				t.Fatalf("Content-Encoding = %q, want compressed %t", got, test.wantCompressed)
			}
			if got := strings.Join(recorder.Header().Values("Vary"), ","); strings.Contains(got, "Accept-Encoding") != test.wantVary {
				t.Fatalf("Vary = %q, want Accept-Encoding %t", got, test.wantVary)
			}
			if got := recorder.Header().Get("Content-Length"); got != test.wantLength {
				t.Fatalf("Content-Length = %q, want %q", got, test.wantLength)
			}
			if !test.wantCompressed && recorder.Body.Len() != len(test.body) {
				t.Fatalf("identity body bytes = %d, want %d", recorder.Body.Len(), len(test.body))
			}
		})
	}
}

func TestLatencyRegistryUsesOnlyFixedRouteAndSQLiteLabels(t *testing.T) {
	var registry latencyMetricsRegistry
	registry.observeRouteService("/api/media/private-user-id", 12*time.Millisecond)
	registry.observeRouteQueue(workloadLaneBrowsing, 4*time.Millisecond)
	registry.observeSQLiteRead("query containing private media path", 9*time.Millisecond)
	registry.observeSQLiteWrite("background", 18*time.Millisecond)

	queue, service := registry.routeSnapshot(workloadLaneDefault)
	if queue.Count != 0 || service.Count != 1 {
		t.Fatalf("default route metrics = queue %+v service %+v", queue, service)
	}
	for _, metric := range append(registry.sqliteReadSnapshot(), registry.sqliteWriteSnapshot()...) {
		if strings.Contains(metric.Lane, "private") || strings.Contains(metric.Lane, "/") {
			t.Fatalf("unbounded or private SQLite label escaped: %q", metric.Lane)
		}
	}
	if got := len(registry.sqliteReadSnapshot()); got != latencySQLiteLaneCount {
		t.Fatalf("read lane count = %d, want %d", got, latencySQLiteLaneCount)
	}
}

func TestLatencyMetricsAppearInExistingDiagnostics(t *testing.T) {
	server := &Server{workloadLanes: newWorkloadLanes()}
	server.latencyMetrics.observeRouteQueue(workloadLaneBrowsing, 7*time.Millisecond)
	server.latencyMetrics.observeRouteService(workloadLaneBrowsing, 31*time.Millisecond)
	server.recordSQLiteReadMetrics("user", 14*time.Millisecond, nil)
	server.recordSQLiteWriteMetrics("background", database.RetryStats{Attempts: 1}, 22*time.Millisecond, nil)

	var browsing WorkloadLaneDiagnostic
	for _, diagnostic := range server.workloadLaneDiagnostics() {
		if diagnostic.ID == workloadLaneBrowsing {
			browsing = diagnostic
			break
		}
	}
	if browsing.QueueWaitLatency.Count != 1 || browsing.ServiceLatency.Count != 1 {
		t.Fatalf("workload diagnostics missing latency: %+v", browsing)
	}
	sqlite := server.sqliteDiagnostics()
	if sqlite.ReadLatency[0].Lane != "user" || sqlite.ReadLatency[0].Count != 1 {
		t.Fatalf("SQLite read latency missing: %+v", sqlite.ReadLatency)
	}
	if sqlite.WriteLatency[1].Lane != "background" || sqlite.WriteLatency[1].Count != 1 {
		t.Fatalf("SQLite write latency missing: %+v", sqlite.WriteLatency)
	}
}
