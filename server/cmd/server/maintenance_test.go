package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"portico.local/apikit/idempotency"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

func TestCheckpointHousekeepingRetriesDeferredHardResetWithoutNewCommits(t *testing.T) {
	// Both outcomes retain genuine successful PASSIVE copy numbers. The old
	// counts-only settled predicate would suppress the next checkpoint forever
	// once writes stopped, leaving a failed hard reset unattempted.
	for _, outcome := range []string{"reader-timeout", "reader-backoff", "future-deferred-reset"} {
		result := dbwork.CheckpointResult{Mode: "passive", Outcome: outcome, LogFrames: dbwork.CheckpointHardFrames + 1, Checkpoint: dbwork.CheckpointHardFrames + 1}
		if checkpointSettled(result) {
			t.Fatalf("%s suppressed retry of fully copied but unreset WAL", outcome)
		}
		if result.Busy != 0 || result.Err != nil || result.Checkpoint != result.LogFrames {
			t.Fatal("settlement changed SQLite's truthful result")
		}
	}
}

func TestCheckpointHousekeepingSettlesOnlyCompletedCheckpoint(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result dbwork.CheckpointResult
		want   bool
	}{
		{"copied", dbwork.CheckpointResult{Mode: "passive", Outcome: "copied", LogFrames: 10, Checkpoint: 10}, true},
		{"truncated", dbwork.CheckpointResult{Mode: "truncate", Outcome: "truncated"}, true},
		{"partial", dbwork.CheckpointResult{Mode: "passive", Outcome: "partial", LogFrames: 10, Checkpoint: 9}, false},
		{"busy truncate", dbwork.CheckpointResult{Mode: "truncate", Outcome: "busy", Busy: 1, LogFrames: 10, Checkpoint: 10}, false},
		{"error", dbwork.CheckpointResult{Mode: "truncate", Outcome: "error", Err: errors.New("interrupted")}, false},
		{"unclassified", dbwork.CheckpointResult{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkpointSettled(tc.result); got != tc.want {
				t.Fatalf("settled=%t want%t: %+v", got, tc.want, tc.result)
			}
		})
	}
}

func TestReceiptSweepSkipsEmptyGateAndDrainsBacklogInBatches(t *testing.T) {
	ctx := context.Background()
	db, err := persistence.Open(filepath.Join(t.TempDir(), "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	starts := 0
	receipts := idempotency.Store{SweepBegin: func(ctx context.Context) (idempotency.Transaction, error) {
		starts++
		return dbwork.Begin(ctx, db, dbwork.ClassMaintenance)
	}}
	if backlog, err := sweepDueReceipts(ctx, db, receipts); err != nil || backlog || starts != 0 {
		t.Fatalf("empty sweep: backlog=%v starts=%d err=%v", backlog, starts, err)
	}
	_, err = db.ExecContext(ctx, `WITH RECURSIVE seq(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM seq WHERE x<1050)
		INSERT INTO api_idempotency(scope,key,digest,status,body,expires_at)
		SELECT 'test',printf('key-%04d',x),'digest',200,x'',? FROM seq`, time.Now().Add(-time.Minute).Unix())
	if err != nil {
		t.Fatal(err)
	}
	if backlog, err := sweepDueReceipts(ctx, db, receipts); err != nil || !backlog || starts != 10 {
		t.Fatalf("first sweep: backlog=%v starts=%d err=%v", backlog, starts, err)
	}
	if backlog, err := sweepDueReceipts(ctx, db, receipts); err != nil || backlog || starts != 11 {
		t.Fatalf("remaining sweep: backlog=%v starts=%d err=%v", backlog, starts, err)
	}
	var remaining int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM api_idempotency`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("remaining receipts=%d err=%v", remaining, err)
	}
}
