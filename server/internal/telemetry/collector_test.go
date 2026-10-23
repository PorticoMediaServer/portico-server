package telemetry

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "portico.local/server/internal/thirdparty/sqlite"
)

// fakeSampler produces a predictable ramp so rollup averages and window bounds
// can be asserted without touching the host.
type fakeSampler struct {
	calls int
	gpu   bool
}

func (f *fakeSampler) Sample(_ context.Context, at int64) Sample {
	f.calls++
	s := newSample(at)
	s.Metrics["cpu"] = Available(float64(f.calls))
	s.Metrics["memory"] = Available(50)
	s.Metrics["diskRead"] = Available(1000)
	s.Metrics["diskWrite"] = Available(2000)
	s.Metrics["netIn"] = Available(10)
	s.Metrics["netOut"] = Available(20)
	s.MemoryUsedBytes, s.MemoryTotalBytes = 4<<30, 8<<30
	if f.gpu {
		s.Metrics["gpuUsage"] = Available(33)
		s.GPUDevice, s.GPUProvider = "Test GPU", "Test"
	}
	return s
}

func openTelemetryDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	raw, e := os.ReadFile("../persistence/migrations/0001_baseline.sql")
	if e != nil {
		t.Fatal(e)
	}
	schema := string(raw)
	a := strings.Index(schema, "CREATE TABLE telemetry_rollups(")
	b := strings.Index(schema[a:], ";") + a + 1
	if _, e = db.Exec(schema[a:b]); e != nil {
		t.Fatal(e)
	}
	return db
}

func TestRollupsAverageTheMinuteAndKeepUnmeasuredMetricsUnavailable(t *testing.T) {
	clock := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	sampler := &fakeSampler{}
	c := New(Options{Sampler: sampler, Now: func() time.Time { return clock }})
	// Two samples in the first minute, then one in the next, which closes the
	// first minute's rollup.
	for _, step := range []time.Duration{0, 2 * time.Second, time.Minute} {
		clock = clock.Add(step)
		c.Collect(context.Background())
	}
	rollups := c.Rollups()
	if len(rollups) != 1 {
		t.Fatal("one closed minute expected", len(rollups))
	}
	if cpu := rollups[0].Metrics["cpu"]; cpu.Status != StatusAvailable || cpu.Value != 1.5 {
		t.Fatal("the minute must average its samples", cpu)
	}
	if gpu := rollups[0].Metrics["gpuUsage"]; gpu.Known() {
		t.Fatal("a metric no sample could measure must stay unavailable, not become zero", gpu)
	}
	if rollups[0].MemoryTotalBytes != 8<<30 {
		t.Fatal("absolute memory figures must survive the rollup", rollups[0])
	}
}

func TestWindowsAreBoundedAndServeTheRightResolution(t *testing.T) {
	clock := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	c := New(Options{Sampler: &fakeSampler{}, Now: func() time.Time { return clock }})
	// Twelve minutes of two-second samples: more detail than the ten-minute
	// window retains, and enough closed minutes to chart an hour.
	for i := 0; i < 360; i++ {
		c.Collect(context.Background())
		clock = clock.Add(2 * time.Second)
	}
	if got := len(c.Rollups()); got > RollupSamples || got != 11 {
		t.Fatal("closed minutes", got)
	}

	detail, ok := c.Read("10m")
	if !ok {
		t.Fatal("the ten-minute window must be accepted")
	}
	if len(detail.Series["cpu"]) > DetailSamples {
		t.Fatal("the detail window must stay bounded", len(detail.Series["cpu"]))
	}
	if len(detail.Series["cpu"]) < 290 {
		t.Fatal("the detail window must be dense", len(detail.Series["cpu"]))
	}
	if detail.Series["gpuUsage"] == nil {
		t.Fatal("every published metric needs a series, even an empty one")
	}
	if len(detail.Series["gpuUsage"]) != 0 {
		t.Fatal("an unmeasured metric must chart as a gap, not as zeroes")
	}
	if detail.Status["cpu"].Status != StatusAvailable {
		t.Fatal("status must describe the latest sample", detail.Status)
	}

	hour, _ := c.Read("1h")
	if len(hour.Series["cpu"]) != 12 {
		t.Fatal("the hour window is served from one-minute rollups", len(hour.Series["cpu"]))
	}
	for _, point := range hour.Series["cpu"] {
		if point.T < hour.ObservedAt-time.Hour.Milliseconds() || point.T > hour.ObservedAt {
			t.Fatal("a point outside the window must not be returned", point)
		}
	}
	if _, ok := c.Read("5s"); ok {
		t.Fatal("an unknown window must be refused")
	}
}

func TestHistorySurvivesRestartAndIsPrunedToADay(t *testing.T) {
	db := openTelemetryDatabase(t)
	clock := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	first := New(Options{Sampler: &fakeSampler{gpu: true}, DB: db, Now: now})
	// A stale row from two days ago must not come back, and must be deleted.
	if _, e := db.Exec(`INSERT INTO telemetry_rollups VALUES(60000,?,'cpu',1,1,'available','')`, clock.Add(-48*time.Hour).UnixMilli()); e != nil {
		t.Fatal(e)
	}
	first.Restore(context.Background())
	for i := 0; i < 90; i++ {
		first.Collect(context.Background())
		clock = clock.Add(2 * time.Second)
	}
	first.persist(context.Background(), true)

	var rows int
	if e := db.QueryRow(`SELECT count(DISTINCT bucket_ms) FROM telemetry_rollups WHERE resolution_ms=60000`).Scan(&rows); e != nil {
		t.Fatal(e)
	}
	if rows == 0 {
		t.Fatal("closed minutes must be persisted")
	}
	var stale int
	if e := db.QueryRow(`SELECT count(*) FROM telemetry_rollups WHERE resolution_ms=60000 AND bucket_ms<?`, clock.Add(-24*time.Hour).UnixMilli()).Scan(&stale); e != nil {
		t.Fatal(e)
	}
	if stale != 0 {
		t.Fatal("rows older than a day must be pruned")
	}

	second := New(Options{Sampler: &fakeSampler{}, DB: db, Now: now})
	second.Restore(context.Background())
	restored := second.Rollups()
	if len(restored) != rows {
		t.Fatal("a restart must read its charts back", len(restored), rows)
	}
	if restored[0].Metrics["gpuUsage"].Status != StatusAvailable {
		t.Fatal("restored rollups must keep each metric's status", restored[0].Metrics["gpuUsage"])
	}
}

func TestACollectorWithoutASamplerStillAnswers(t *testing.T) {
	c := New(Options{Sampler: emptySampler{}})
	sample := c.Collect(context.Background())
	if len(sample.Metrics) != len(MetricNames) {
		t.Fatal("a sampler that returns nothing must still publish every metric as unavailable", sample)
	}
	if c.Latest().Metrics["cpu"].Known() {
		t.Fatal("an unmeasured metric must not read as known")
	}
}

type emptySampler struct{}

func (emptySampler) Sample(context.Context, int64) Sample { return Sample{} }

func TestActivityDaySeriesUsesAtMostSixtyRealPoints(t *testing.T) {
	clock := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	c := New(Options{Sampler: &fakeSampler{}, Now: func() time.Time { return clock }})
	for n := 0; n < 1440; n++ {
		c.Collect(context.Background())
		clock = clock.Add(time.Minute)
	}
	out, ok := c.Read("24h")
	if !ok || len(out.Series["cpu"]) != 60 || len(out.Series["gpuUsage"]) != 0 {
		t.Fatal(out, ok)
	}
	points := out.Series["cpu"]
	if points[0].V != 1 || points[len(points)-1].V != 1440 {
		t.Fatal("first/last measured points changed", points)
	}
	for n, p := range points {
		if p.V != float64((p.T-points[0].T)/time.Minute.Milliseconds()+1) || n > 0 && p.T <= points[n-1].T {
			t.Fatal("invented/interpolated point", p)
		}
	}
}

func TestDashboardLongRangesRestoreNormalizedRollupsWithoutInventingGaps(t *testing.T) {
	db := openTelemetryDatabase(t)
	clock := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	c := New(Options{Sampler: &fakeSampler{gpu: true}, DB: db, Now: func() time.Time { return clock }})
	for n := 0; n < 410; n++ {
		c.Collect(context.Background())
		clock = clock.Add(24 * time.Hour)
	}
	c.persist(context.Background(), true)
	restored := New(Options{Sampler: &fakeSampler{}, DB: db, Now: func() time.Time { return clock }})
	restored.Restore(context.Background())
	for _, window := range []string{"12h", "24h", "7d", "30d", "90d", "1y", "all"} {
		out, ok := restored.Read(window)
		if !ok || len(out.Series["cpu"]) > 70 {
			t.Fatal(window, out)
		}
		if window == "all" && len(out.Series["cpu"]) < 50 {
			t.Fatal("weekly history lost after restart", len(out.Series["cpu"]))
		}
		if len(out.Series["processCPU"]) != 0 {
			t.Fatal("missing process history fabricated")
		}
	}
	var count int
	if e := db.QueryRow(`SELECT count(*) FROM telemetry_rollups WHERE resolution_ms=86400000`).Scan(&count); e != nil || count > 401*len(MetricNames) {
		t.Fatal(count, e)
	}
	if e := db.QueryRow(`SELECT count(*) FROM telemetry_rollups WHERE resolution_ms=60000`).Scan(&count); e != nil || count > len(MetricNames) {
		t.Fatal("minute retention", count, e)
	}
}

func TestLiveRetainsSixtyMeasuredTwoSecondPoints(t *testing.T) {
	clock := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	c := New(Options{Sampler: &fakeSampler{}, Now: func() time.Time { return clock }})
	for n := 0; n < 120; n++ {
		c.Collect(context.Background())
		clock = clock.Add(2 * time.Second)
	}
	out, ok := c.Read("live")
	if !ok || len(out.Series["cpu"]) != 60 {
		t.Fatal(out)
	}
	for n, p := range out.Series["cpu"] {
		if n > 0 && p.T-out.Series["cpu"][n-1].T != 2000 {
			t.Fatal("live cadence", p)
		}
	}
}
