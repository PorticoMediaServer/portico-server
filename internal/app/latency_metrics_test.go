package app

import (
	"strings"
	"testing"
	"time"

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
