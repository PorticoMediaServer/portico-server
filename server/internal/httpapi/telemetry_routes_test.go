package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"portico.local/server/internal/operations"
	"portico.local/server/internal/telemetry"
)

func telemetryFixture(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	d, owner, member := operationsFixture(t)
	d.Console = operations.New(d.DB)
	d.Measurements = operations.NewMeasurements(d.DB, t.TempDir())
	mux := http.NewServeMux()
	d.telemetryRoutes(mux)
	return mux, owner.AccessToken, member.AccessToken
}

func telemetryData(t *testing.T, mux http.Handler, token, path string, into any) {
	t.Helper()
	w := supportRequest(mux, token, path)
	if w.Code != 200 {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("owner observations must not be cached by intermediaries", path)
	}
	var envelope struct {
		Scope struct {
			ServerID string `json:"serverId"`
			Fence    string `json:"viewerFence"`
		} `json:"scope"`
		Data json.RawMessage `json:"data"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &envelope); e != nil {
		t.Fatal(path, e)
	}
	if envelope.Scope.ServerID == "" || len(envelope.Scope.Fence) != 64 {
		t.Fatal("every owner read must carry its server and viewer fence", path)
	}
	if into != nil {
		if e := json.Unmarshal(envelope.Data, into); e != nil {
			t.Fatal(path, e)
		}
	}
}

func TestTelemetrySurfacesAreOwnerOnly(t *testing.T) {
	mux, owner, member := telemetryFixture(t)
	paths := []string{"/v1/admin/transcode/capacity", "/v1/admin/telemetry", "/v1/admin/telemetry/now", "/v1/admin/attention", "/v1/admin/playback/history", "/v1/admin/playback/history.csv"}
	for _, path := range paths {
		for _, token := range []string{"", member} {
			if w := supportRequest(mux, token, path); w.Code != refusedStatus(token) {
				t.Fatalf("%s must refuse a non-owner: %d", path, w.Code)
			}
		}
		if w := supportRequest(mux, owner, path); w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestCapacityAndAttentionReadTheSettingsRegistry(t *testing.T) {
	mux, owner, _ := telemetryFixture(t)
	var report telemetry.Report
	telemetryData(t, mux, owner, "/v1/admin/transcode/capacity", &report)
	if report.X264Preset != "veryfast" || report.Hardware.Configured != "auto" || report.ThrottleBufferSeconds != 60 {
		t.Fatal("the capacity report must publish the registry's effective values", report)
	}
	if len(report.Presets) == 0 || report.Hardware.Effective == "" {
		t.Fatal("the report must name its ladder and its effective backend", report)
	}
	var attention struct {
		Items []telemetry.Item `json:"items"`
	}
	telemetryData(t, mux, owner, "/v1/admin/attention", &attention)
	for _, item := range attention.Items {
		if item.ID == "" || item.Severity == "" || item.Action.Kind == "" {
			t.Fatal("every attention item must be actionable", item)
		}
	}
}

func TestTelemetryWindowsAndHistoryQueriesAreValidated(t *testing.T) {
	mux, owner, _ := telemetryFixture(t)
	var reading telemetry.Reading
	telemetryData(t, mux, owner, "/v1/admin/telemetry?window=1h", &reading)
	if reading.Window != "1h" || len(reading.Series) != len(telemetry.MetricNames) {
		t.Fatal("every published metric needs a series", reading)
	}
	for _, name := range telemetry.MetricNames {
		if _, ok := reading.Status[name]; !ok {
			t.Fatal("every metric must publish its own status", name)
		}
	}
	for _, bad := range []string{"/v1/admin/telemetry?window=5s", "/v1/admin/playback/history?period=forever", "/v1/admin/playback/history?limit=9999", "/v1/admin/telemetry?unknown=1"} {
		if w := supportRequest(mux, owner, bad); w.Code != 400 {
			t.Fatalf("%s must be refused: %d %s", bad, w.Code, w.Body.String())
		}
	}
	var sample telemetry.Sample
	telemetryData(t, mux, owner, "/v1/admin/telemetry/now", &sample)
	if len(sample.Metrics) != len(telemetry.MetricNames) {
		t.Fatal("the latest sample must publish every metric", sample)
	}
}

func TestPlaybackHistoryExportIsADownloadableCSV(t *testing.T) {
	mux, owner, _ := telemetryFixture(t)
	w := supportRequest(mux, owner, "/v1/admin/playback/history.csv?period=7d")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") {
		t.Fatal(w.Code, w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "playback-history-7d.csv") {
		t.Fatal("the export must be named for its period", w.Header().Get("Content-Disposition"))
	}
	if !strings.HasPrefix(w.Body.String(), "id,viewer,title") {
		t.Fatal("the export must carry a header row", w.Body.String())
	}
}
