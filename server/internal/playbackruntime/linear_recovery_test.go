package playbackruntime

import (
	"testing"
	"time"

	"portico.local/server/internal/linearbuffer"
)

// A channel that plays for a long stretch and then hiccups is a new fault, not
// one more step toward giving up: only failures with no sustained output in
// between count against the recovery budget.
func TestLinearRecoveryBudgetResetsAfterSustainedOutput(t *testing.T) {
	second := time.Second.Microseconds()
	window := func(start, end int64, gaps ...linearbuffer.Interval) linearbuffer.Window {
		return linearbuffer.Window{StartUS: start * second, EndUS: end * second, LiveUS: end * second, Ready: end > start, Gaps: gaps}
	}
	healthy := int64(linearHealthyRun / time.Second)
	// Five long healthy runs, each ending in a failure, never exhaust recovery.
	attempts, edge := 0, int64(0)
	for run := 0; run < linearRecoveryBudget*3; run++ {
		before := window(0, edge)
		edge += healthy
		attempts = linearFailureCount(attempts, before, window(0, edge))
		if attempts != 1 {
			t.Fatalf("run %d: a failure after sustained output counted as failure %d", run, attempts)
		}
	}
	// A source that never plays for long still runs out of retries.
	attempts, edge = 0, 0
	for run := 1; run <= linearRecoveryBudget; run++ {
		before := window(0, edge)
		edge += 5
		attempts = linearFailureCount(attempts, before, window(0, edge))
		if attempts != run {
			t.Fatalf("a short run reset the count: %d after %d failures", attempts, run)
		}
	}
	if attempts < linearRecoveryBudget {
		t.Fatal("continuous failure no longer exhausts recovery")
	}
	// A producer that added nothing leaves the count growing, however much
	// earlier media the buffer still holds.
	if got := linearFailureCount(3, window(0, 600), window(0, 600)); got != 4 {
		t.Fatalf("retained media from an earlier producer was counted as healthy output: %d", got)
	}
	// Time spent waiting and reconnecting is a gap, not output.
	gap := linearbuffer.Interval{StartUS: 100 * second, EndUS: (100 + healthy) * second}
	if got := linearFailureCount(3, window(0, 100), window(0, 100+healthy+4, gap)); got != 4 {
		t.Fatalf("a reconnect gap was counted as healthy output: %d", got)
	}
	if got := linearFailureCount(3, window(0, 100), window(0, 100+2*healthy+4, gap)); got != 1 {
		t.Fatalf("sustained output after a gap did not reset the count: %d", got)
	}
	// The first producer of an empty buffer is measured from its own first media.
	if got := linearFailureCount(0, linearbuffer.Window{}, window(90, 94)); got != 1 {
		t.Fatalf("first failure: %d", got)
	}
	if got := linearFailureCount(2, linearbuffer.Window{}, window(90, 94)); got != 3 {
		t.Fatalf("start-up delay before the first media was counted as output: %d", got)
	}
	if got := linearFailureCount(2, linearbuffer.Window{}, window(90, 90+healthy)); got != 1 {
		t.Fatalf("a long first run did not reset the count: %d", got)
	}
}
