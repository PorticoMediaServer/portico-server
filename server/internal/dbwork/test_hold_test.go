package dbwork

import (
	"context"
	"database/sql"
	"testing"
)

func insertOneRow(ctx context.Context, db *sql.DB, value string) error {
	return WithWriteTx(ctx, db, ClassInteractive, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO rows(value) VALUES (?)`, value)
		return err
	})
}

// TestWriterHoldCollectorSamplesOnlyOptedInWrites proves the diagnostic is
// opt-in and that a sampled hold carries the facts a failed latency assertion
// needs: the actual gate wall, the source site, the class and plan, the measured
// rows/pages/VM, and an honest committed or rollback state.
func TestWriterHoldCollectorSamplesOnlyOptedInWrites(t *testing.T) {
	db := testDB(t)
	ctx, holds := WithTestWriterHolds(context.Background())
	if holds == nil {
		t.Fatal("no collector in a test binary")
	}
	// The same write without the collector context must not be observed.
	if err := insertOneRow(context.Background(), db, "plain"); err != nil {
		t.Fatal(err)
	}
	if sample, ok := holds.Snapshot(); ok {
		t.Fatalf("unopted write was sampled: %+v", sample)
	}
	if err := insertOneRow(ctx, db, "opted in"); err != nil {
		t.Fatal(err)
	}
	sample, ok := holds.Snapshot()
	if !ok {
		t.Fatal("opted-in write was not sampled")
	}
	if sample.Wall <= 0 || sample.TxPhase <= 0 {
		t.Fatalf("sampled hold has no wall or terminal tx phase: %+v", sample)
	}
	if sample.Site == "" || sample.Class != ClassInteractive.String() || sample.Plan != DefaultPlan(ClassInteractive).Name {
		t.Fatalf("sample does not name its source: %+v", sample)
	}
	if !sample.Committed || sample.Rows != 1 {
		t.Fatalf("committed one-row write reported as %+v", sample)
	}
	if !sample.CostKnown || sample.Pages < 1 || sample.VMOps < 1 {
		t.Fatalf("committed write cost was not measured: %+v", sample)
	}
	if sample.Wall < sample.TxPhase {
		t.Fatalf("wall %s is shorter than its tx phase %s", sample.Wall, sample.TxPhase)
	}
	// A rollback is sampled too, because the gate peak counts rollbacks: it must
	// say uncommitted and charge no accepted rows.
	ctx, holds = WithTestWriterHolds(context.Background())
	w, err := Begin(ctx, db, ClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Tx().Exec(`INSERT INTO rows(value) VALUES ('rolled back')`); err != nil {
		t.Fatal(err)
	}
	if err = w.Rollback(); err != nil {
		t.Fatal(err)
	}
	rolled, ok := holds.Snapshot()
	if !ok {
		t.Fatal("rolled-back write was not sampled")
	}
	if rolled.Committed || rolled.Rows != 0 || rolled.Wall <= 0 || rolled.TxPhase <= 0 {
		t.Fatalf("uncommitted sample is not honest about it: %+v", rolled)
	}
}
