package ingestion

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/server/internal/persistence"
	"portico.local/server/internal/supervise"
)

// A Run must own the monitor, its worker loop and every quantum it starts. An
// iteration panic then unwinds only after those children stop, so the outer
// supervise.Loop can restart the scanner without reusing a stale generation.
func TestRunOwnsChildrenSoPanicReachesSupervisor(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`INSERT INTO libraries(id,name,kind,root) VALUES('lib','Movies','movie','/private-root')`,
		`INSERT INTO jobs(id,library_id,status,created_at) VALUES('job','lib','queued','2026-10-22')`,
		`INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision,phase,discovered,analyzed) VALUES('job','src',1,'inc','root',1,'basic_running',10,2)`,
	} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	service := New(db, nil, nil)

	previousMonitor, previousQuantum, previousHook := monitorRun, quantumRun, iterationHook
	t.Cleanup(func() { monitorRun, quantumRun, iterationHook = previousMonitor, previousQuantum, previousHook })

	monitorStarts := make(chan struct{}, 8)
	monitorStops := make(chan struct{}, 8)
	quantumStarts := make(chan struct{}, 8)
	quantumStops := make(chan struct{}, 8)
	monitorRun = func(s *Service, ctx context.Context) {
		monitorStarts <- struct{}{}
		<-ctx.Done()
		monitorStops <- struct{}{}
	}
	quantumRun = func(s *Service, ctx context.Context, id, library string) bool {
		quantumStarts <- struct{}{}
		<-ctx.Done()
		quantumStops <- struct{}{}
		return false
	}
	var iterations atomic.Int32
	iterationHook = func(context.Context) {
		// The first two iterations start the monitor and the queued job's
		// quantum; the third panics while both are still running.
		if iterations.Add(1) == 3 {
			panic("injected iteration panic")
		}
	}

	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	done := make(chan struct{})
	go func() {
		defer close(done)
		supervise.Loop(parent, "test.ingestion", service.Run)
	}()

	waitRunSignal(t, monitorStarts, "first monitor start")
	waitRunSignal(t, quantumStarts, "first quantum start")
	// The panic unwinds Run: its own generation stops, so the first quantum and
	// monitor join without the parent being cancelled.
	waitRunSignal(t, quantumStops, "first quantum join")
	waitRunSignal(t, monitorStops, "first monitor join")
	// The supervisor restarts the scanner; the durable queued job is picked up
	// again by the new generation.
	waitRunSignal(t, quantumStarts, "restarted quantum start")

	cancelParent()
	waitRunSignal(t, done, "supervisor exit")
	waitRunSignal(t, quantumStops, "restarted quantum join")
	waitRunSignal(t, monitorStops, "restarted monitor join")
}

func waitRunSignal(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}
