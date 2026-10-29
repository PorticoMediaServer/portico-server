package dbwork

import (
	"context"
	"database/sql"
	"testing"
	"time"
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
	if !sample.BudgetKnown {
		t.Fatalf("sample did not carry its own turn's plan: %+v", sample)
	}
	if want := DefaultPlan(ClassInteractive); sample.Budget != (WriteCost{Rows: want.Rows, Pages: want.Pages, VMOps: want.VMOps}) {
		t.Fatalf("sample budget %+v, want its declared plan %+v", sample.Budget, want)
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

// holdSample is one synthetic turn against the ordinary background plan
// (160 rows, 256 pages, 250,000 VM operations), within every work dimension.
func holdSample(wall, cpu time.Duration) TestHoldSample {
	return TestHoldSample{
		Wall: wall, TxPhase: wall / 2, Committed: true,
		Rows: 60, Pages: 100, VMOps: 20_000, CostKnown: true,
		Budget: WriteCost{Rows: 160, Pages: 256, VMOps: 250_000}, BudgetKnown: true,
		ProcessCPU: cpu, ProcessCPUKnown: true,
	}
}

// TestTestHoldVerdictBoundaries fixes the approved construction rule: 250 ms
// wall, 50 ms whole-process CPU, 10% of that turn's plan VM, and factor-one
// work budgets. Unknown CPU, cost or budget over the wall is conservatively
// hard, never environmental.
func TestTestHoldVerdictBoundaries(t *testing.T) {
	policy := TestHoldPolicy{Wall: 250 * time.Millisecond, CPU: 50 * time.Millisecond, VMShare: 0.10}
	over := 250*time.Millisecond + time.Nanosecond
	for _, tc := range []struct {
		name   string
		sample func() TestHoldSample
		want   TestHoldVerdict
	}{
		{"wall exactly at bound", func() TestHoldSample { return holdSample(250*time.Millisecond, 49*time.Millisecond) }, TestHoldWithin},
		{"work overrun with tiny wall", func() TestHoldSample {
			s := holdSample(time.Millisecond, time.Millisecond)
			s.Rows = 161
			return s
		}, TestHoldHard},
		{"pages one over plan", func() TestHoldSample {
			s := holdSample(time.Millisecond, time.Millisecond)
			s.Pages = 257
			return s
		}, TestHoldHard},
		{"vm one over plan", func() TestHoldSample {
			s := holdSample(time.Millisecond, time.Millisecond)
			s.VMOps = 250_001
			return s
		}, TestHoldHard},
		{"wall over, cpu under, vm at ten percent", func() TestHoldSample { return holdSample(over, 49*time.Millisecond) }, TestHoldEnvironmental},
		{"cpu exactly at bound", func() TestHoldSample { return holdSample(over, 50*time.Millisecond) }, TestHoldHard},
		{"vm one over ten percent", func() TestHoldSample {
			s := holdSample(over, 10*time.Millisecond)
			s.VMOps = 25_001
			return s
		}, TestHoldHard},
		{"unknown process cpu over wall", func() TestHoldSample {
			s := holdSample(over, 0)
			s.ProcessCPUKnown = false
			return s
		}, TestHoldHard},
		{"unknown cost over wall", func() TestHoldSample {
			s := holdSample(over, 10*time.Millisecond)
			s.CostKnown = false
			return s
		}, TestHoldHard},
		{"unknown budget over wall", func() TestHoldSample {
			s := holdSample(over, 10*time.Millisecond)
			s.BudgetKnown = false
			return s
		}, TestHoldHard},
		{"unbounded vm budget over wall", func() TestHoldSample {
			s := holdSample(over, 10*time.Millisecond)
			s.Budget.VMOps = 0
			return s
		}, TestHoldHard},
		{"unknown cpu within wall", func() TestHoldSample {
			s := holdSample(250*time.Millisecond, 0)
			s.ProcessCPUKnown = false
			return s
		}, TestHoldWithin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := TestHoldVerdictFor(tc.sample(), policy)
			if got != tc.want {
				t.Fatalf("verdict=%s want=%s reason=%s", got, tc.want, reason)
			}
			if got != TestHoldWithin && reason == "" {
				t.Fatal("classification gave no reason")
			}
		})
	}
}

// TestTestHoldAggregateKeepsConcealedHardTurn proves bounded aggregates evaluate
// every sample: a short CPU-heavy turn stays a hard failure even when a longer,
// cheap environmental stall is the largest wall, and a work overrun stays
// visible behind a still larger environmental turn.
func TestTestHoldAggregateKeepsConcealedHardTurn(t *testing.T) {
	_, holds := WithTestWriterHoldsPolicy(context.Background(), TestHoldPolicy{Wall: 250 * time.Millisecond, CPU: 50 * time.Millisecond, VMShare: 0.10})
	holds.record(holdSample(900*time.Millisecond, 10*time.Millisecond), callSite{})
	holds.record(holdSample(260*time.Millisecond, 60*time.Millisecond), callSite{})
	aggregate := holds.Aggregate()
	if aggregate.Samples != 2 {
		t.Fatalf("samples=%d want 2", aggregate.Samples)
	}
	if aggregate.Verdict() != TestHoldHard {
		t.Fatalf("verdict=%s, long environmental stall masked the heavy turn", aggregate.Verdict())
	}
	if !aggregate.LargestOK || aggregate.Largest.Wall != 900*time.Millisecond {
		t.Fatalf("largest wall not preserved: %+v", aggregate.Largest)
	}
	if !aggregate.WorstHeavyOK || aggregate.WorstHeavy.Wall != 260*time.Millisecond || aggregate.WorstHeavy.ProcessCPU != 60*time.Millisecond {
		t.Fatalf("concealed heavy turn lost: %+v", aggregate.WorstHeavy)
	}
	if !aggregate.MaxCPUOK || aggregate.MaxCPU.ProcessCPU != 60*time.Millisecond {
		t.Fatalf("max cpu sample wrong: %+v", aggregate.MaxCPU)
	}
	if !aggregate.WorstEnvironmentalOK || aggregate.WorstEnvironmental.Wall != 900*time.Millisecond {
		t.Fatalf("environmental sample wrong: %+v", aggregate.WorstEnvironmental)
	}
	// The original diagnostic API still reports the largest wall.
	if sample, ok := holds.Snapshot(); !ok || sample.Wall != 900*time.Millisecond {
		t.Fatalf("Snapshot changed: %+v %t", sample, ok)
	}
	// A work overrun at a small wall survives later, longer turns.
	overrun := holdSample(100*time.Millisecond, time.Millisecond)
	overrun.Rows = 161
	holds.record(overrun, callSite{})
	holds.record(holdSample(2*time.Second, time.Millisecond), callSite{})
	aggregate = holds.Aggregate()
	if aggregate.Verdict() != TestHoldHard || !aggregate.WorstOverrunOK || aggregate.WorstOverrun.Rows != 161 || aggregate.WorstOverrun.Wall != 100*time.Millisecond {
		t.Fatalf("work overrun lost behind a longer stall: %+v", aggregate)
	}
	if aggregate.Largest.Wall != 2*time.Second || aggregate.WorstHeavy.Wall != 260*time.Millisecond {
		t.Fatalf("aggregates not updated independently: %+v", aggregate)
	}
	// An unknown-CPU turn never displaces a known max-CPU sample.
	unknown := holdSample(time.Second, 0)
	unknown.ProcessCPUKnown = false
	holds.record(unknown, callSite{})
	if aggregate = holds.Aggregate(); aggregate.MaxCPU.ProcessCPU != 60*time.Millisecond || aggregate.Samples != 5 {
		t.Fatalf("unknown cpu displaced the max cpu sample: %+v", aggregate)
	}
}
