package telemetry

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/worker"
)

func TestTelemetryPersistsOneBoundedSampleWithoutPerMetricStatements(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "telemetry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	clock := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	c := New(Options{Sampler: &fakeSampler{gpu: true}, DB: db, Now: func() time.Time { return clock }})
	ctx := context.Background()
	for range 3 {
		c.Collect(ctx)
		clock = clock.Add(2 * time.Second)
	}
	clock = clock.Add(time.Minute)
	// Keep the real completed sample pending, then observe its actual SQL.
	c.lastPersist = clock.UnixMilli()
	c.Collect(ctx)
	listen := func(table string) <-chan struct{} {
		sig := worker.NewSignal()
		unregister := dbwork.WakeOnTables(sig, table)
		loop, cancel := context.WithCancel(ctx)
		done, passes := make(chan struct{}), make(chan struct{}, 8)
		go func() {
			defer close(done)
			worker.RunWith(loop, "telemetry-persist-test", sig, nil, func(context.Context) time.Duration { passes <- struct{}{}; return 0 })
		}()
		<-passes
		t.Cleanup(func() { unregister(); cancel(); <-done })
		return passes
	}
	metrics, unrelated := listen("telemetry_rollups"), listen("quick_connect_v1")
	dbwork.ProfileStatements(true)
	c.persist(ctx, true)
	profile := dbwork.StatementProfile()
	dbwork.ProfileStatements(false)
	var inserts int64
	for _, entry := range profile {
		if strings.HasPrefix(entry.Shape, "INSERT INTO telemetry_rollups") {
			inserts += entry.Count
		}
	}
	if inserts != 1 {
		t.Fatal("sample persisted per metric", inserts)
	}
	select {
	case <-metrics:
	case <-time.After(time.Second):
		t.Fatal("actual telemetry persist did not name its table")
	}
	select {
	case <-unrelated:
		t.Fatal("actual telemetry persist broadcast to unrelated work")
	case <-time.After(20 * time.Millisecond):
	}
	var rows, samples int
	var total float64
	if err := db.QueryRow(`SELECT count(*) FROM telemetry_rollups`).Scan(&rows); err != nil || rows < 4 {
		t.Fatal("lost known metric cells", rows, err)
	}
	if err := db.QueryRow(`SELECT samples,total FROM telemetry_rollups WHERE metric='cpu'`).Scan(&samples, &total); err != nil || samples != 3 || total != 6 {
		t.Fatal("lost actual sample weights", samples, total, err)
	}
	c.persist(ctx, true)
	var again int
	if err := db.QueryRow(`SELECT samples FROM telemetry_rollups WHERE metric='cpu'`).Scan(&again); err != nil || again != samples {
		t.Fatal("replayed completed sample", again, err)
	}
}
