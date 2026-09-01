package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTelemetryFileReadIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry-output")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 128)), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := readTelemetryFile(path, 32); err == nil {
		t.Fatalf("expected oversized telemetry file to fail closed")
	}
}

func TestTelemetryCommandOutputIsBounded(t *testing.T) {
	result := readSingleTelemetryJSON(strings.NewReader(strings.Repeat("{", 64)), 16)
	if !errors.Is(result.err, errManagedCommandOutputLimit) {
		t.Fatalf("read result error = %v, want output-limit error", result.err)
	}
	if len(result.output) != 0 {
		t.Fatalf("output retained after limit failure: %d bytes", len(result.output))
	}
}

func TestTelemetryCommandTimeoutKillsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell process groups are platform-specific")
	}
	directory := t.TempDir()
	sentinel := filepath.Join(directory, "orphan-survived")
	script := filepath.Join(directory, "telemetry-child.sh")
	contents := "#!/bin/sh\nsleep 1\ntouch " + sentinel + "\n"
	if err := os.WriteFile(script, []byte(contents), 0o700); err != nil {
		t.Fatalf("write command fixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runTelemetryCommand(ctx, script)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("command error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timed-out telemetry command took %s", elapsed)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, statErr := os.Stat(sentinel); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("child process survived group cancellation: stat error=%v", statErr)
	}
}

func TestIntelGPUProbeUsesOneBoundedSampleAndCleansChildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Intel probe process-group behavior is covered by the Windows build and Unix runtime tests")
	}
	directory := t.TempDir()
	sentinel := filepath.Join(directory, "intel-child-survived")
	fake := filepath.Join(directory, "intel_gpu_top")
	contents := "#!/bin/sh\nprintf '%s\\n' '{\"Render/3D/0\":42,\"Device memory\":17,\"Video/0\":8}'\n(sleep 1; touch " + sentinel + ") &\nwait\n"
	if err := os.WriteFile(fake, []byte(contents), 0o700); err != nil {
		t.Fatalf("write Intel fixture: %v", err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := runIntelGPUSample(context.Background())
	if err != nil {
		t.Fatalf("run Intel sample: %v", err)
	}
	sample, err := parseIntelGPUSample(output)
	if err != nil {
		t.Fatalf("parse Intel sample: %v", err)
	}
	if sample.Usage.Status != telemetryStatusOK || sample.Usage.Value != 42 {
		t.Fatalf("unexpected Intel usage sample: %#v", sample.Usage)
	}
	if sample.Memory.Status != telemetryStatusOK || sample.Memory.Value != 17 || sample.Encoder.Value != 8 {
		t.Fatalf("unexpected Intel GPU sample: %#v", sample)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, statErr := os.Stat(sentinel); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Intel child survived bounded probe: stat error=%v", statErr)
	}
}

func TestTelemetryUnavailableMetricsNeverBecomeSuccessfulZeros(t *testing.T) {
	memory := boundedMemoryUsage(0, 0)
	if memory.Status == telemetryStatusOK || memory.UsedBytes != 0 || memory.FreeBytes != 0 || memory.TotalBytes != 0 {
		t.Fatalf("invalid memory snapshot was presented as success: %#v", memory)
	}
	sample := sampleGPUContext(context.Background(), &telemetrySamplerState{}, DashboardGPUInfo{Provider: "Apple Silicon", Note: "collector not configured"})
	if sample.Usage.Status == telemetryStatusOK || sample.Memory.Status == telemetryStatusOK || sample.Encoder.Status == telemetryStatusOK {
		t.Fatalf("unsupported GPU metrics were presented as success: %#v", sample)
	}
	point := sampleSystemTelemetryContext(context.Background(), &telemetrySamplerState{}, DashboardGPUInfo{Provider: "Unknown", Note: "no collector"})
	if point.GPUUsageState.Status == telemetryStatusOK {
		t.Fatalf("GPU point was not fail-closed: %#v", point.GPUUsageState)
	}
}

func TestTelemetryStatusFieldsRemainExplicitOnWire(t *testing.T) {
	status := telemetryMetricStatus(unavailableTelemetryMetric(telemetryStatusUnsupported, "collector not configured"))
	activity, err := json.Marshal(ServerActivityResponse{CPUPercent: 0, CPUStatus: status, MemoryStatus: status})
	if err != nil {
		t.Fatalf("marshal activity response: %v", err)
	}
	var activityPayload map[string]any
	if err := json.Unmarshal(activity, &activityPayload); err != nil {
		t.Fatalf("decode activity response: %v", err)
	}
	if activityPayload["cpuPercent"] != float64(0) {
		t.Fatalf("unexpected numeric CPU fallback: %#v", activityPayload["cpuPercent"])
	}
	cpuStatus, ok := activityPayload["cpuStatus"].(map[string]any)
	if !ok || cpuStatus["status"] != telemetryStatusUnsupported || cpuStatus["reason"] != "collector not configured" {
		t.Fatalf("CPU status was not explicit on the wire: %#v", activityPayload["cpuStatus"])
	}

	dashboard, err := json.Marshal(DashboardGPUSample{Usage: 0, UsageStatus: status, HeadroomStatus: status})
	if err != nil {
		t.Fatalf("marshal dashboard GPU sample: %v", err)
	}
	var dashboardPayload map[string]any
	if err := json.Unmarshal(dashboard, &dashboardPayload); err != nil {
		t.Fatalf("decode dashboard GPU sample: %v", err)
	}
	usageStatus, ok := dashboardPayload["usageStatus"].(map[string]any)
	if !ok || usageStatus["status"] != telemetryStatusUnsupported || usageStatus["reason"] != "collector not configured" {
		t.Fatalf("GPU status was not explicit on the wire: %#v", dashboardPayload["usageStatus"])
	}
	headroomStatus, ok := dashboardPayload["headroomStatus"].(map[string]any)
	if !ok || headroomStatus["status"] != telemetryStatusUnsupported || headroomStatus["reason"] != "collector not configured" {
		t.Fatalf("GPU headroom status was not explicit on the wire: %#v", dashboardPayload["headroomStatus"])
	}
}

func TestTelemetryHistoryIsHardCapped(t *testing.T) {
	point := systemTelemetryPoint{At: time.Now().UTC()}
	points := make([]systemTelemetryPoint, 0, telemetryHistoryLimit+10)
	for index := 0; index < telemetryHistoryLimit+10; index++ {
		point.At = point.At.Add(time.Millisecond)
		points = retainTelemetryPoint(points, point)
	}
	if len(points) != telemetryHistoryLimit {
		t.Fatalf("telemetry history length = %d, want %d", len(points), telemetryHistoryLimit)
	}
	old := systemTelemetryPoint{At: time.Now().UTC().Add(-telemetryHistoryWindow - time.Second)}
	points = retainTelemetryPoint(append(points, old), point)
	for _, existing := range points {
		if existing.At.Before(point.At.Add(-telemetryHistoryWindow)) {
			t.Fatalf("expired telemetry point was retained: %s", existing.At)
		}
	}
}

func TestTelemetrySamplerBacksOffWithoutDashboardObserver(t *testing.T) {
	now := time.Now()
	server := &Server{}
	if got := server.telemetrySamplingInterval(now); got != telemetryIdleSampleInterval {
		t.Fatalf("idle telemetry interval = %s, want %s", got, telemetryIdleSampleInterval)
	}
	server.serverActivityCache.expiresAt = now.Add(time.Second)
	if got := server.telemetrySamplingInterval(now); got != telemetryActiveSampleInterval {
		t.Fatalf("observed telemetry interval = %s, want %s", got, telemetryActiveSampleInterval)
	}
}

func TestParseLinuxIOPressure(t *testing.T) {
	raw := `
some avg10=12.34 avg60=5.67 avg300=1.23 total=456789
full avg10=0.45 avg60=0.12 avg300=0.01 total=12345
`
	sample, err := parseLinuxIOPressure(raw)
	if err != nil {
		t.Fatalf("parse io pressure: %v", err)
	}
	if sample.Some.Avg10 != 12.34 || sample.Some.Avg60 != 5.67 || sample.Some.Avg300 != 1.23 {
		t.Fatalf("unexpected some pressure values: %#v", sample.Some)
	}
	if sample.Full.Avg10 != 0.45 || sample.Full.Avg60 != 0.12 || sample.Full.Avg300 != 0.01 {
		t.Fatalf("unexpected full pressure values: %#v", sample.Full)
	}
}

func TestParseLinuxIOPressureRequiresSomeAndFull(t *testing.T) {
	if _, err := parseLinuxIOPressure(`some avg10=0.00 avg60=0.00 avg300=0.00 total=1`); err == nil {
		t.Fatalf("expected missing full line to fail")
	}
	if _, err := parseLinuxIOPressure(`full avg10=0.00 avg60=0.00 avg300=0.00 total=1`); err == nil {
		t.Fatalf("expected missing some line to fail")
	}
}

func TestParseLinuxDiskStatsAggregatesPhysicalDevices(t *testing.T) {
	raw := `
   7       0 loop0 10 0 20 0 30 0 40 0 0 0 0 0 0 0 0 0 0
   8       0 sda 100 1 2000 3 400 5 6000 7 2 9 10 0 0 0 0 0
 259       0 nvme0n1 300 2 4000 5 600 7 8000 9 1 11 12 0 0 0 0 0
`
	stats, err := parseLinuxDiskStats(raw)
	if err != nil {
		t.Fatalf("parse diskstats: %v", err)
	}
	if !stats.Supported {
		t.Fatalf("diskstats should be supported: %#v", stats)
	}
	if stats.ReadOperations != 400 || stats.WriteOperations != 1000 {
		t.Fatalf("unexpected disk operations: %#v", stats)
	}
	if stats.ReadSectors != 6000 || stats.WriteSectors != 14000 || stats.IOInProgress != 3 {
		t.Fatalf("unexpected disk sector/in-flight counts: %#v", stats)
	}
}

func TestIOPressureDiagnosticsTracksHighPressureHistory(t *testing.T) {
	first := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)
	points := []systemTelemetryPoint{
		{At: first, IOPressure: ioPressureSample{
			At:        first,
			Supported: true,
			Status:    ioPressureStatusOK,
			Source:    linuxIOPressurePath,
			Some:      ioPressureLine{Avg10: 2, Avg60: 3, Avg300: 4},
			Full:      ioPressureLine{Avg10: 0.2, Avg60: 0.3, Avg300: 0.4},
		}},
		{At: second, IOPressure: ioPressureSample{
			At:        second,
			Supported: true,
			Status:    ioPressureStatusOK,
			Source:    linuxIOPressurePath,
			Some:      ioPressureLine{Avg10: 12, Avg60: 8, Avg300: 6},
			Full:      ioPressureLine{Avg10: 1.5, Avg60: 0.7, Avg300: 0.5},
			Disk:      diskStatsSample{Supported: true, ReadOperations: 10, WriteOperations: 20, ReadSectors: 30, WriteSectors: 40, IOInProgress: 1},
		}},
	}
	diagnostics := ioPressureDiagnosticsFromPoints(points, ioPressureSample{})
	if diagnostics.Status != ioPressureStatusOK || !diagnostics.Supported || diagnostics.Samples != 2 {
		t.Fatalf("unexpected diagnostics status: %#v", diagnostics)
	}
	if diagnostics.CurrentSomeAvg10 != 12 || diagnostics.HighestRecentSomeAvg10 != 12 || diagnostics.HighestRecentFullAvg10 != 1.5 {
		t.Fatalf("unexpected pressure summary: %#v", diagnostics)
	}
	if diagnostics.LastHighPressureAt != second.Format(time.RFC3339) {
		t.Fatalf("last high pressure at = %q, expected %q", diagnostics.LastHighPressureAt, second.Format(time.RFC3339))
	}
	if !diagnostics.DiskStatsSupported || diagnostics.DiskReadOperations != 10 || diagnostics.DiskWriteOperations != 20 {
		t.Fatalf("unexpected disk stats in diagnostics: %#v", diagnostics)
	}
}

func TestDashboardDiskIOSampleUsesDiskStatDeltas(t *testing.T) {
	start := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	points := []systemTelemetryPoint{
		{At: start, IOPressure: ioPressureSample{Disk: diskStatsSample{
			Supported:       true,
			ReadOperations:  100,
			WriteOperations: 40,
			ReadSectors:     2048,
			WriteSectors:    1024,
			IOInProgress:    1,
		}}},
		{At: start.Add(10 * time.Second), IOPressure: ioPressureSample{Disk: diskStatsSample{
			Supported:       true,
			ReadOperations:  130,
			WriteOperations: 60,
			ReadSectors:     22528,
			WriteSectors:    11264,
			IOInProgress:    2,
		}}},
	}

	sample := dashboardDiskIOSample(points, start, start.Add(10*time.Second), "NOW")
	if !sample.Supported {
		t.Fatalf("disk sample should be supported: %#v", sample)
	}
	if sample.ReadMegabytesPerSecond != 1 || sample.WriteMegabytesPerSecond != 0.5 {
		t.Fatalf("unexpected disk throughput: %#v", sample)
	}
	if sample.OperationsPerSecond != 5 || sample.IOInProgress != 2 {
		t.Fatalf("unexpected disk operations: %#v", sample)
	}
}

func TestDashboardTodayStartUsesLocalCalendarDay(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("AST", -4*60*60)
	defer func() { time.Local = previous }()

	now := time.Date(2026, 6, 19, 3, 30, 0, 0, time.UTC)
	start := dashboardTodayStart(now)
	expected := time.Date(2026, 6, 18, 4, 0, 0, 0, time.UTC)
	if !start.Equal(expected) {
		t.Fatalf("dashboardTodayStart = %s, expected %s", start, expected)
	}
}
