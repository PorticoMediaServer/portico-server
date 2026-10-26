package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"portico.local/server/internal/persistence"
	"portico.local/server/internal/storagepaths"
)

// restoreMaintenanceSeams resets the focused lifecycle knobs after a test.
func restoreMaintenanceSeams(t *testing.T) {
	t.Helper()
	retention, planner := retentionRun, plannerRefresh
	interval := plannerRefreshInterval
	poll, ancillary := housekeepingPollInterval, housekeepingAncillaryInterval
	tickHook, plannerHook := housekeepingTickHook, plannerRefreshHook
	t.Cleanup(func() {
		retentionRun, plannerRefresh = retention, planner
		plannerRefreshInterval = interval
		housekeepingPollInterval, housekeepingAncillaryInterval = poll, ancillary
		housekeepingTickHook, plannerRefreshHook = tickHook, plannerHook
	})
}

func openMaintenanceDB(t *testing.T) *sql.DB {
	t.Helper()
	state := t.TempDir()
	db, err := persistence.Open(storagepaths.Path(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Retention owns a never-ending timer loop. Before the fix it ran inline, so the
// goroutine below never reached its ticker, planner refresh or checkpoints. This
// test blocks retention and proves the housekeeping body still advances first.
func TestHousekeepingAdvancesWhileRetentionIsBlocked(t *testing.T) {
	db := openMaintenanceDB(t)
	restoreMaintenanceSeams(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	retentionStarted := make(chan struct{})
	retentionStopped := make(chan struct{})
	retentionRun = func(ctx context.Context, db *sql.DB) {
		close(retentionStarted)
		<-ctx.Done()
		close(retentionStopped)
	}
	plannerRefreshInterval = 0
	housekeepingPollInterval = 5 * time.Millisecond
	housekeepingAncillaryInterval = time.Millisecond
	ticked := make(chan struct{}, 1)
	housekeepingTickHook = func() {
		select {
		case ticked <- struct{}{}:
		default:
		}
	}
	refreshed := make(chan struct{}, 1)
	plannerRefresh = func(ctx context.Context, db *sql.DB) error {
		select {
		case refreshed <- struct{}{}:
		default:
		}
		return nil
	}
	plannerRefreshHook = func() {}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runDatabaseHousekeeping(ctx, db)
	}()

	waitFor(t, retentionStarted, "retention child start")
	waitFor(t, ticked, "housekeeping checkpoint tick")
	waitFor(t, refreshed, "planner refresh tick")

	cancel()
	waitFor(t, done, "housekeeping exit")
	waitFor(t, retentionStopped, "retention child exit")
}

// Cancellation must join both tracked loops through the existing shutdown path.
func TestHousekeepingCancellationStopsTrackedRetention(t *testing.T) {
	db := openMaintenanceDB(t)
	restoreMaintenanceSeams(t)

	ctx, cancel := context.WithCancel(context.Background())
	retentionStarted := make(chan struct{})
	retentionStopped := make(chan struct{})
	retentionRun = func(ctx context.Context, db *sql.DB) {
		close(retentionStarted)
		<-ctx.Done()
		close(retentionStopped)
	}
	housekeepingPollInterval = 5 * time.Millisecond
	housekeepingAncillaryInterval = time.Hour
	plannerRefresh = func(ctx context.Context, db *sql.DB) error { return nil }

	done := make(chan struct{})
	go func() {
		defer close(done)
		runDatabaseHousekeeping(ctx, db)
	}()
	waitFor(t, retentionStarted, "retention child start")

	cancel()
	waitFor(t, done, "housekeeping exit")
	waitFor(t, retentionStopped, "retention child exit")
}

func waitFor(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}
