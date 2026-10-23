package administration

import (
	"context"
	"portico.local/server/internal/dbwork"
	"testing"
	"time"
)

func TestBackgroundPauseDurableAndIdempotent(t *testing.T) {
	defer dbwork.SetBackgroundPolicy(dbwork.SpeedNormal, 0)
	service, _, _ := newService(t)
	ctx := context.Background()
	first, err := service.PauseBackground(ctx, allow, BackgroundPauseChange{true, "pause-work-hour"})
	if err != nil || first.PausedUntil < time.Now().Add(59*time.Minute).UnixMilli() {
		t.Fatal(first, err)
	}
	again, err := service.PauseBackground(ctx, allow, BackgroundPauseChange{true, "pause-work-hour"})
	if err != nil || first.PausedUntil != again.PausedUntil || first.Revision != again.Revision {
		t.Fatal("retry extended pause", again, err)
	}
	saved, err := service.MaintenanceSettings(ctx, allow)
	if err != nil || saved.PausedUntil != first.PausedUntil {
		t.Fatal("pause not stored", saved, err)
	}
	settings := saved.Settings
	settings.BackgroundSpeed = "fast"
	saved, err = service.SaveMaintenanceSettings(ctx, allow, Change[MaintenanceSettingsDocument]{ExpectedRevision: saved.Revision, OperationID: "speed-while-paused", Settings: settings})
	if err != nil || saved.PausedUntil != first.PausedUntil {
		t.Fatal("settings save erased pause", saved, err)
	}
	resumed, err := service.PauseBackground(ctx, allow, BackgroundPauseChange{false, "resume-work-now"})
	if err != nil || resumed.PausedUntil != 0 {
		t.Fatal(resumed, err)
	}
	if dbwork.BackgroundSpeed(ctx) != "fast" || !dbwork.Yield(ctx) {
		t.Fatal("running policy not applied")
	}
}

func TestMaintenanceKeepsBackupAndHeavyWindowsSeparate(t *testing.T) {
	service, _, _ := newService(t)
	current, err := service.MaintenanceSettings(context.Background(), allow)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Tasks) != 3 || len(current.Settings.Windows) != 2 {
		t.Fatal("inert scheduled jobs offered", current.Tasks)
	}
	settings := current.Settings
	settings.Windows[0].Tasks = []string{"analysis"}
	if _, err = service.SaveMaintenanceSettings(context.Background(), allow, Change[MaintenanceSettingsDocument]{ExpectedRevision: current.Revision, OperationID: "unsupported-schedule", Settings: settings}); err == nil {
		t.Fatal("inert schedule saved")
	}
}

func TestCommittedBackgroundPolicySurvivesCancelledRequest(t *testing.T) {
	defer dbwork.SetBackgroundPolicy(dbwork.SpeedNormal, 0)
	service, _, _ := newService(t)
	saved, e := service.MaintenanceSettings(context.Background(), allow)
	if e != nil {
		t.Fatal(e)
	}
	settings := saved.Settings
	settings.BackgroundSpeed = "fast"
	if _, e = service.SaveMaintenanceSettings(context.Background(), allow, Change[MaintenanceSettingsDocument]{ExpectedRevision: saved.Revision, OperationID: "committed-speed", Settings: settings}); e != nil {
		t.Fatal(e)
	}
	dbwork.SetBackgroundPolicy(dbwork.SpeedNormal, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service.ApplyBackgroundPolicy(ctx)
	if got := dbwork.BackgroundSpeed(context.Background()); got != "fast" {
		t.Fatalf("committed speed left unapplied after request cancelled: %s", got)
	}
}

func TestBoundedWorkerReadsEachCommittedMaintenanceSpeed(t *testing.T) {
	defer dbwork.SetBackgroundPolicy(dbwork.SpeedNormal, 0)
	service, db, _ := newService(t)
	ctx := context.Background()
	document, err := service.MaintenanceSettings(ctx, allow)
	if err != nil {
		t.Fatal(err)
	}
	for _, speed := range []string{dbwork.SpeedGentle, dbwork.SpeedFast, dbwork.SpeedNormal} {
		settings := document.Settings
		settings.BackgroundSpeed = speed
		document, err = service.SaveMaintenanceSettings(ctx, allow, Change[MaintenanceSettingsDocument]{ExpectedRevision: document.Revision, OperationID: "batch-speed-" + speed, Settings: settings})
		if err != nil {
			t.Fatal(err)
		}
		// The real workers reload at every bounded batch. A stale process
		// projection must not replace the durable owner choice with Normal.
		dbwork.SetBackgroundPolicy(dbwork.SpeedNormal, 0)
		batch := dbwork.WithBackgroundSpeed(ctx, dbwork.ReadBackgroundSpeed(ctx, db))
		if got := dbwork.BackgroundSpeed(batch); got != speed {
			t.Fatalf("next batch: got %q, want committed %q", got, speed)
		}
		want := map[string]int{dbwork.SpeedGentle: 2, dbwork.SpeedNormal: 4, dbwork.SpeedFast: 8}[speed]
		if got := dbwork.BackgroundConcurrency(batch); got != want {
			t.Fatalf("next batch concurrency: got %d, want %d", got, want)
		}
	}
}
