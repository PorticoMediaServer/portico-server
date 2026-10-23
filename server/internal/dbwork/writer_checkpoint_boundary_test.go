package dbwork

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestWriterLeavesThresholdCheckpointToMaintenance(t *testing.T) {
	ctx := context.Background()
	db, err := OpenHandle(filepath.Join(t.TempDir(), "checkpoint-control.sqlite"), DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = ExecWrite(ctx, db, ClassInteractive, `CREATE TABLE checkpoint_control(id INTEGER PRIMARY KEY, revision INTEGER NOT NULL); INSERT INTO checkpoint_control VALUES(1,0)`); err != nil {
		t.Fatal(err)
	}
	if initial := WAL(ctx, db); initial.LastError != "" || initial.BacklogFrames != 0 {
		t.Fatalf("initial checkpoint: %+v", initial)
	}
	schedule, err := NewCheckpointSchedule(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	// Freeze the former production threshold independently of the new policy.
	// No Backup or maintenance worker runs while these real commits accumulate.
	// A writer-side checkpoint resets the logical log before the final probe.
	const commits = 16128
	gate.ResetPeak()
	defer gate.ResetPeak()
	var slowest time.Duration
	for range commits {
		start := time.Now()
		if _, err = ExecWrite(ctx, db, ClassInteractive, `UPDATE checkpoint_control SET revision=revision+1 WHERE id=1`); err != nil {
			t.Fatal(err)
		}
		slowest = max(slowest, time.Since(start))
	}
	held := gate.Stats().MaxHeldMilli
	// This PASSIVE probe is deliberately on the public read connection, after
	// every writer has released the gate. It also supplies a real copy control.
	before := WAL(ctx, db)
	t.Logf("commits=%d logFrames=%d fileBytes=%d slowestWrite=%s maxHeldMs=%d", commits, before.LogFrames, before.FileBytes, slowest, held)
	if before.LastError != "" || before.LogFrames < commits || before.CheckpointedFrames != before.LogFrames {
		t.Errorf("writer copied/reset the WAL before maintenance: %+v", before)
	}
	if slowest > 250*time.Millisecond || held > 250 {
		t.Errorf("writer exceeded unchanged 250ms budget: slowest=%s held=%dms", slowest, held)
	}

	// Reuse the production schedule rather than creating a second checkpoint
	// loop. New commits must still converge outside the writer's critical path.
	schedule.lastAttempt = time.Now().Add(-CheckpointCopyInterval)
	result, ran := schedule.Tick(ctx)
	if !ran || !CheckpointSettled(result) {
		t.Fatalf("maintenance did not settle the accumulated WAL: ran=%v result=%+v", ran, result)
	}
	var revision int
	if err = db.QueryRowContext(ctx, `SELECT revision FROM checkpoint_control WHERE id=1`).Scan(&revision); err != nil || revision != commits {
		t.Fatalf("committed data changed across checkpoint: revision=%d err=%v", revision, err)
	}
}
