package metadata

// The approved P2-59 construction gate. It is test policy only: production
// WritePlan budgets, admission and accounting are unchanged. One construction
// run is every stage, proof and seal transaction mbStageRelease performs; each
// opted-in turn is classified against its own declared plan.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
)

const (
	// mbConstructionHold bounds one construction turn's gate hold.
	mbConstructionHold = 250 * time.Millisecond
	// mbConstructionCPU bounds whole-process user+system CPU (every goroutine,
	// not this transaction alone) consumed during that hold. Reaching it is
	// CPU-heavy work, not a busy machine.
	mbConstructionCPU = 50 * time.Millisecond
	// mbConstructionVMShare bounds a turn's measured VM operations as a share of
	// the VM operations its own plan allows.
	mbConstructionVMShare = 0.10
	// mbConstructionAttempts is the most construction runs one gate may start.
	mbConstructionAttempts = 3
	// mbConstructionEnvironmentalFails is the number of environmental runs that
	// fails the gate; two of three cannot be outvoted by one clean run.
	mbConstructionEnvironmentalFails = 2
)

var mbConstructionGatePolicy = dbwork.TestHoldPolicy{
	Wall:    mbConstructionHold,
	CPU:     mbConstructionCPU,
	VMShare: mbConstructionVMShare,
}

// mbConstructionDecision is what one measured construction run permits next.
type mbConstructionDecision int

const (
	// mbConstructionPass accepts the construction result already measured.
	mbConstructionPass mbConstructionDecision = iota
	// mbConstructionRetry starts one more full, fresh construction attempt.
	mbConstructionRetry
	// mbConstructionFail stops the gate without another attempt.
	mbConstructionFail
)

func (d mbConstructionDecision) String() string {
	switch d {
	case mbConstructionPass:
		return "pass"
	case mbConstructionFail:
		return "fail"
	default:
		return "retry"
	}
}

// mbConstructionNext is the pure retry policy over every verdict measured so
// far. Any hard run fails immediately. A clean first run passes with one
// attempt. A first environmental run is remeasured: environmental+clean+clean
// passes, environmental+environmental fails as two of three, and
// environmental+clean+environmental also fails. No other run may start once an
// attempt has been judged.
func mbConstructionNext(verdicts []dbwork.TestHoldVerdict) mbConstructionDecision {
	if len(verdicts) == 0 {
		return mbConstructionRetry
	}
	clean, environmental := 0, 0
	for _, verdict := range verdicts {
		switch verdict {
		case dbwork.TestHoldHard:
			return mbConstructionFail
		case dbwork.TestHoldEnvironmental:
			environmental++
		default:
			clean++
		}
	}
	if environmental >= mbConstructionEnvironmentalFails {
		return mbConstructionFail
	}
	if environmental == 0 || clean >= 2 {
		return mbConstructionPass
	}
	if len(verdicts) >= mbConstructionAttempts {
		return mbConstructionFail
	}
	return mbConstructionRetry
}

// mbConstructionVerdict folds one attempt's bounded aggregate and gate peak into
// the run verdict. A peak over the bound with no opted-in sample over it is an
// over-budget hold this window cannot explain, so it is conservatively hard.
func mbConstructionVerdict(aggregate dbwork.TestHoldAggregate, peak time.Duration) (dbwork.TestHoldVerdict, string) {
	verdict := aggregate.Verdict()
	switch {
	case verdict == dbwork.TestHoldHard:
		sample, ok := aggregate.WorstOverrun, aggregate.WorstOverrunOK
		if !ok {
			sample, ok = aggregate.WorstHeavy, aggregate.WorstHeavyOK
		}
		if !ok {
			return dbwork.TestHoldHard, "hard aggregate without a retained hard sample"
		}
		_, reason := dbwork.TestHoldVerdictFor(sample, mbConstructionGatePolicy)
		return dbwork.TestHoldHard, reason
	case peak > mbConstructionHold && (!aggregate.LargestOK || aggregate.Largest.Wall <= mbConstructionHold):
		return dbwork.TestHoldHard, fmt.Sprintf("gate peak %s>%s with no opted-in turn over the bound", peak, mbConstructionHold)
	case verdict == dbwork.TestHoldEnvironmental:
		_, reason := dbwork.TestHoldVerdictFor(aggregate.WorstEnvironmental, mbConstructionGatePolicy)
		return dbwork.TestHoldEnvironmental, reason
	default:
		return dbwork.TestHoldWithin, ""
	}
}

// mbConstructionDiagnostic is the full bounded self-diagnosis: every extreme the
// window kept, never a per-turn list.
func mbConstructionDiagnostic(aggregate dbwork.TestHoldAggregate, peak time.Duration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "samples=%d peak-held=%s verdict=%s", aggregate.Samples, peak, aggregate.Verdict())
	for _, slot := range []struct {
		name   string
		sample dbwork.TestHoldSample
		ok     bool
	}{
		{"largest-wall", aggregate.Largest, aggregate.LargestOK},
		{"max-cpu", aggregate.MaxCPU, aggregate.MaxCPUOK},
		{"worst-overrun", aggregate.WorstOverrun, aggregate.WorstOverrunOK},
		{"worst-heavy", aggregate.WorstHeavy, aggregate.WorstHeavyOK},
		{"worst-environmental", aggregate.WorstEnvironmental, aggregate.WorstEnvironmentalOK},
	} {
		if slot.ok {
			fmt.Fprintf(&b, "\n  %s: %s", slot.name, slot.sample)
		}
	}
	return b.String()
}

// mbConstructionCPULine is the one-line worst CPU-heavy turn of one construction
// run, so each run names the turn that came closest to the CPU bound.
func mbConstructionCPULine(aggregate dbwork.TestHoldAggregate) string {
	if !aggregate.MaxCPUOK {
		return "worst-cpu-turn unavailable (no turn sampled)"
	}
	sample := aggregate.MaxCPU
	state := "uncommitted"
	if sample.Committed {
		state = "committed"
	}
	share := 0.0
	if sample.BudgetKnown && sample.Budget.VMOps > 0 {
		share = float64(sample.VMOps) / float64(sample.Budget.VMOps)
	}
	return fmt.Sprintf("worst-cpu-turn wall=%s cpu=%s vm=%d/%d(%.1f%%) state=%s plan=%s site=%s",
		sample.Wall, sample.ProcessCPU, sample.VMOps, sample.Budget.VMOps, share*100, state, sample.Plan, sample.Site)
}

// TestMBConstructionRetryPolicy fixes the approved retry decision without
// starting any construction run.
func TestMBConstructionRetryPolicy(t *testing.T) {
	clean := dbwork.TestHoldWithin
	environmental := dbwork.TestHoldEnvironmental
	hard := dbwork.TestHoldHard
	for _, tc := range []struct {
		name     string
		verdicts []dbwork.TestHoldVerdict
		want     mbConstructionDecision
	}{
		{"no attempt yet starts the first", nil, mbConstructionRetry},
		{"one clean run passes with one attempt", []dbwork.TestHoldVerdict{clean}, mbConstructionPass},
		{"first environmental run is remeasured", []dbwork.TestHoldVerdict{environmental}, mbConstructionRetry},
		{"environmental then clean needs a third", []dbwork.TestHoldVerdict{environmental, clean}, mbConstructionRetry},
		{"environmental clean clean passes", []dbwork.TestHoldVerdict{environmental, clean, clean}, mbConstructionPass},
		{"environmental environmental fails two of three", []dbwork.TestHoldVerdict{environmental, environmental}, mbConstructionFail},
		{"environmental clean environmental fails", []dbwork.TestHoldVerdict{environmental, clean, environmental}, mbConstructionFail},
		{"hard fails immediately", []dbwork.TestHoldVerdict{hard}, mbConstructionFail},
		{"hard after environmental fails immediately", []dbwork.TestHoldVerdict{environmental, hard}, mbConstructionFail},
		{"clean then hard fails immediately", []dbwork.TestHoldVerdict{clean, hard}, mbConstructionFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mbConstructionNext(tc.verdicts); got != tc.want {
				t.Fatalf("decision=%s want=%s for %v", got, tc.want, tc.verdicts)
			}
		})
	}
}

// TestMBConstructionRunVerdict proves the assembly rules: a hard sample wins
// over a longer environmental one, an unexplained gate peak over the bound is
// conservatively hard, and an environmental run is only environmental.
func TestMBConstructionRunVerdict(t *testing.T) {
	within := dbwork.TestHoldSample{
		Wall: 10 * time.Millisecond, Rows: 10, Pages: 4, VMOps: 100, CostKnown: true,
		Budget: dbwork.WriteCost{Rows: 160, Pages: 256, VMOps: 250_000}, BudgetKnown: true,
		ProcessCPU: time.Millisecond, ProcessCPUKnown: true,
	}
	environmental := within
	environmental.Wall = 900 * time.Millisecond
	environmental.ProcessCPU = 5 * time.Millisecond
	heavy := within
	heavy.Wall = 260 * time.Millisecond
	heavy.ProcessCPU = 60 * time.Millisecond
	// overrun exceeds its own plan on rows at factor one, so the recorded sample
	// is hard work even though its wall is inside the bound.
	overrun := within
	overrun.Rows = 161
	for _, tc := range []struct {
		name      string
		aggregate dbwork.TestHoldAggregate
		peak      time.Duration
		want      dbwork.TestHoldVerdict
	}{
		{"within", dbwork.TestHoldAggregate{Samples: 1, Largest: within, LargestOK: true}, 10 * time.Millisecond, dbwork.TestHoldWithin},
		{"environmental", dbwork.TestHoldAggregate{Samples: 1, Largest: environmental, LargestOK: true, WorstEnvironmental: environmental, WorstEnvironmentalOK: true}, 900 * time.Millisecond, dbwork.TestHoldEnvironmental},
		{"concealed heavy", dbwork.TestHoldAggregate{Samples: 2, Largest: environmental, LargestOK: true, WorstHeavy: heavy, WorstHeavyOK: true, WorstEnvironmental: environmental, WorstEnvironmentalOK: true}, 900 * time.Millisecond, dbwork.TestHoldHard},
		{"work overrun", dbwork.TestHoldAggregate{Samples: 1, Largest: within, LargestOK: true, WorstOverrun: overrun, WorstOverrunOK: true}, 10 * time.Millisecond, dbwork.TestHoldHard},
		{"unexplained peak over bound", dbwork.TestHoldAggregate{Samples: 1, Largest: within, LargestOK: true}, 400 * time.Millisecond, dbwork.TestHoldHard},
		{"no sample but peak within", dbwork.TestHoldAggregate{}, 10 * time.Millisecond, dbwork.TestHoldWithin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := mbConstructionVerdict(tc.aggregate, tc.peak)
			if got != tc.want {
				t.Fatalf("verdict=%s want=%s reason=%q", got, tc.want, reason)
			}
			if got != dbwork.TestHoldWithin && reason == "" {
				t.Fatal("verdict gave no reason")
			}
		})
	}
}
