package dbwork

// Bounded, opt-in diagnostics for a test that asserts how long the write gate
// was held. A peak-hold failure says how long; it does not say which write held
// it, or whether that wall was CPU or waiting. This records one sample per
// opted-in transaction and keeps the largest by wall, all after the gate has
// been released. There is no process-wide ring, no reset and no logging: a
// context created by WithTestWriterHolds is the only way in, and a write that
// does not carry it is not observed.
//
// Deliberately out of scope: fsync and checkpoint durations are not exposed by
// the driver, so a sample says unavailable rather than guessing. Autocommit
// ExecWrite statements and reserved-connection BeginConn transactions create no
// Write this file can attach a sample to, so they are not sampled.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestHoldSample is the largest gate hold one WriterHoldCollector saw.
type TestHoldSample struct {
	Site  string
	Class string
	Plan  string
	// Wall is the gate hold from the actual acquisition to just before release.
	Wall time.Duration
	// TxPhase is the terminal transaction call's duration: Commit or Rollback,
	// matching Committed.
	TxPhase time.Duration
	// AfterTx is the part of Wall after that call returned and before the gate
	// was released: after-commit callbacks and connection release.
	AfterTx   time.Duration
	Committed bool

	Rows  int64
	Pages int64
	VMOps int64
	// CostKnown reports whether the driver could read pages and VM operations.
	CostKnown bool
	// ProcessCPU is whole-process user+system CPU consumed during Wall, from
	// getrusage(RUSAGE_SELF). It includes every goroutine, so it is not this
	// transaction's CPU; a larger wall interval is not explained by process CPU.
	ProcessCPU      time.Duration
	ProcessCPUKnown bool
}

// String is the one-line form a failing gate assertion appends. It reports only
// what was measured and names what was not, rather than inventing a split.
func (s TestHoldSample) String() string {
	state := "uncommitted"
	if s.Committed {
		state = "committed"
	}
	cpu := "unavailable"
	if s.ProcessCPUKnown {
		cpu = s.ProcessCPU.String()
	}
	return fmt.Sprintf("site=%s class=%s plan=%s state=%s wall=%s tx-phase=%s after-tx=%s rows=%d pages=%d vm=%d cost-known=%t process-cpu=%s(all-goroutines,user+system) fsync/checkpoint=unavailable",
		s.Site, s.Class, s.Plan, state, s.Wall, s.TxPhase, s.AfterTx,
		s.Rows, s.Pages, s.VMOps, s.CostKnown, cpu)
}

// WriterHoldCollector keeps the largest wall sample of one measurement window.
// It is created by WithTestWriterHolds; a window boundary gets a fresh one.
type WriterHoldCollector struct {
	mu      sync.Mutex
	largest TestHoldSample
	seen    bool
}

// Snapshot returns the largest wall sample seen, and whether any opted-in write
// finished in this window.
func (c *WriterHoldCollector) Snapshot() (TestHoldSample, bool) {
	if c == nil {
		return TestHoldSample{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.largest, c.seen
}

type testHoldKey struct{}

// WithTestWriterHolds opts the writes carrying ctx into hold diagnostics. It is
// test-only: outside a test binary it returns ctx and a nil collector, so no
// production path can collect, time or log by accident.
func WithTestWriterHolds(ctx context.Context) (context.Context, *WriterHoldCollector) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !testing.Testing() {
		return ctx, nil
	}
	collector := &WriterHoldCollector{}
	return context.WithValue(ctx, testHoldKey{}, collector), collector
}

func testHoldsFrom(ctx context.Context) *WriterHoldCollector {
	if ctx == nil {
		return nil
	}
	collector, _ := ctx.Value(testHoldKey{}).(*WriterHoldCollector)
	return collector
}

// testHold is one transaction's capture. It is nil unless the caller opted in.
type testHold struct {
	collector  *WriterHoldCollector
	gateStart  time.Time
	cpuStart   time.Duration
	cpuStartOK bool
	wall       time.Duration
	cpu        time.Duration
	cpuOK      bool
	txPhase    time.Duration
	afterTx    time.Duration
	endAt      time.Time
	committed  bool
	rows       int64
}

// newTestHold captures the actual acquisition time and OS CPU baseline after the
// gate is held and before BEGIN. It returns nil for every unopted caller.
func newTestHold(ctx context.Context) *testHold {
	collector := testHoldsFrom(ctx)
	if collector == nil {
		return nil
	}
	hold := &testHold{collector: collector, gateStart: gate.heldSinceNow()}
	hold.cpuStart, hold.cpuStartOK = processCPUTime()
	return hold
}

// testTxStart marks the terminal transaction call only when a collector is
// attached; the zero time means there is nothing to measure.
func (w *Write) testTxStart() time.Time {
	if w == nil || w.testHold == nil {
		return time.Time{}
	}
	return time.Now()
}

func (w *Write) noteTestTxEnd(committed bool, start time.Time) {
	if w == nil || w.testHold == nil {
		return
	}
	w.testHold.endAt = time.Now()
	w.testHold.txPhase = time.Since(start)
	w.testHold.committed = committed
}

// sampleTestHold stops the clock just before the gate is released. It is called
// from finish, and its result is published only after release.
func (w *Write) sampleTestHold() *testHold {
	hold := w.testHold
	if hold == nil {
		return nil
	}
	hold.wall = time.Since(hold.gateStart)
	if !hold.endAt.IsZero() {
		hold.afterTx = time.Since(hold.endAt)
	}
	hold.rows = w.testRows
	if cpu, ok := processCPUTime(); ok && hold.cpuStartOK && cpu >= hold.cpuStart {
		hold.cpu, hold.cpuOK = cpu-hold.cpuStart, true
	}
	return hold
}

// publish runs with the gate already released. Cost, plan and class are cheap to
// read there; the call site is resolved by record only for a sample that becomes
// the window's largest, so a run of thousands of transactions does not pay to
// symbolise frames it will never report.
func (h *testHold) publish(w *Write) {
	cost, measured := w.meter.cost(h.rows)
	w.mu.Lock()
	plan, class := w.plan, w.owned.class
	w.mu.Unlock()
	h.collector.record(TestHoldSample{
		Class:           class.String(),
		Plan:            plan.Name,
		Wall:            h.wall,
		TxPhase:         h.txPhase,
		AfterTx:         h.afterTx,
		Committed:       h.committed,
		Rows:            cost.Rows,
		Pages:           cost.Pages,
		VMOps:           cost.VMOps,
		CostKnown:       measured,
		ProcessCPU:      h.cpu,
		ProcessCPUKnown: h.cpuOK,
	}, w.site)
}

func (c *WriterHoldCollector) record(sample TestHoldSample, site callSite) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen && sample.Wall <= c.largest.Wall {
		return
	}
	function, file, line, _ := site.resolve()
	sample.Site = fmt.Sprintf("%s (%s:%d)", function, file, line)
	c.largest, c.seen = sample, true
}

// heldSinceNow reports when the current holder acquired the gate, so a sample
// measures the actual hold rather than the time this caller noticed it.
func (g *Gate) heldSinceNow() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.heldSince
}
