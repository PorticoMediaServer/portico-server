package dbwork

// Bounded, opt-in diagnostics for a test that asserts how long the write gate
// was held. A peak-hold failure says how long; it does not say which write held
// it, or whether that wall was CPU or waiting. This records one sample per
// opted-in transaction and keeps a fixed set of extremes — the largest wall, the
// largest process CPU, the worst work-budget overrun, the worst CPU-heavy turn
// over the wall bound, and the worst environmental turn — all after the gate has
// been released. Every sample is classified against its own turn's plan as it is
// recorded, so a cheap but much longer stall cannot hide a shorter turn that is
// over its budget or its CPU bound. There is no sample list, no process-wide
// ring, no reset and no logging: a context created by WithTestWriterHolds is the
// only way in, and a write that does not carry it is not observed.
//
// Deliberately out of scope: fsync and checkpoint durations are not exposed by
// the driver, so a sample says unavailable rather than guessing. Autocommit
// ExecWrite statements and reserved-connection BeginConn transactions create no
// Write this file can attach a sample to, so they are not sampled.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestHoldSample is one opted-in writer turn: its gate hold, the plan it
// declared and the work it measured.
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
	// Budget is the plan this turn declared, attached from the live write at
	// publication, so a sample is judged against its own turn rather than the
	// largest budget the process happened to use. BudgetKnown is false when no
	// plan was attached; zero budget dimensions are unbounded, as in WritePlan.
	Budget      WriteCost
	BudgetKnown bool
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
	budget := "unavailable"
	if s.BudgetKnown {
		budget = fmt.Sprintf("%d/%d/%d", s.Budget.Rows, s.Budget.Pages, s.Budget.VMOps)
	}
	return fmt.Sprintf("site=%s class=%s plan=%s state=%s wall=%s tx-phase=%s after-tx=%s rows=%d pages=%d vm=%d cost-known=%t budget=%s process-cpu=%s(all-goroutines,user+system) fsync/checkpoint=unavailable",
		s.Site, s.Class, s.Plan, state, s.Wall, s.TxPhase, s.AfterTx,
		s.Rows, s.Pages, s.VMOps, s.CostKnown, budget, cpu)
}

// TestHoldPolicy is the approved bound for one writer turn. It is test policy
// only: it never admits, refuses or delays a production write.
type TestHoldPolicy struct {
	// Wall bounds a turn's gate hold.
	Wall time.Duration
	// CPU bounds whole-process user+system CPU — every goroutine, not this
	// transaction alone — during that hold. Reaching the bound is heavy.
	CPU time.Duration
	// VMShare bounds this turn's measured VM operations as a fraction of the VM
	// operations its own plan allows. Exceeding the fraction is heavy.
	VMShare float64
}

// DefaultTestHoldPolicy is the approved ordinary writer bound the diagnostics
// use when no policy is supplied.
func DefaultTestHoldPolicy() TestHoldPolicy {
	return TestHoldPolicy{Wall: 250 * time.Millisecond, CPU: 50 * time.Millisecond, VMShare: 0.10}
}

func (p TestHoldPolicy) normalized() TestHoldPolicy {
	if p.Wall <= 0 || p.CPU <= 0 || p.VMShare <= 0 {
		return DefaultTestHoldPolicy()
	}
	return p
}

// TestHoldVerdict is how one opted-in writer turn compares with its policy.
type TestHoldVerdict int

const (
	// TestHoldWithin: the turn stayed inside its work plan and wall bound.
	TestHoldWithin TestHoldVerdict = iota
	// TestHoldEnvironmental: the turn was over the wall bound, but process CPU
	// stayed under its bound and measured VM operations stayed within the share
	// of this turn's own plan. A busy machine is the explanation, not the work.
	TestHoldEnvironmental
	// TestHoldHard: the turn exceeded its plan on rows, pages or VM operations,
	// or was over the wall bound with CPU, VM or measurement evidence that the
	// work itself is heavy.
	TestHoldHard
)

func (v TestHoldVerdict) String() string {
	switch v {
	case TestHoldEnvironmental:
		return "environmental"
	case TestHoldHard:
		return "hard"
	default:
		return "within"
	}
}

// overBudget names the plan dimensions a sample actually exceeded, at factor
// one, using the plan attached to that turn. Unknown cost can only be judged on
// rows; unknown budget claims nothing here and is judged by the wall rule.
func (s TestHoldSample) overBudget() []string {
	if !s.BudgetKnown {
		return nil
	}
	var out []string
	if s.Budget.Rows > 0 && s.Rows > s.Budget.Rows {
		out = append(out, fmt.Sprintf("rows=%d>%d", s.Rows, s.Budget.Rows))
	}
	if s.CostKnown {
		if s.Budget.Pages > 0 && s.Pages > s.Budget.Pages {
			out = append(out, fmt.Sprintf("pages=%d>%d", s.Pages, s.Budget.Pages))
		}
		if s.Budget.VMOps > 0 && s.VMOps > s.Budget.VMOps {
			out = append(out, fmt.Sprintf("vm=%d>%d", s.VMOps, s.Budget.VMOps))
		}
	}
	return out
}

// TestHoldVerdictFor classifies one sample. The exact rule is:
//
//   - hard if any plan dimension is exceeded at factor one (rows, pages, VM);
//   - otherwise within if Wall is not over the policy wall;
//   - over the wall, hard if process CPU is unknown, cost is unknown, the turn's
//     VM budget is unknown or unbounded, CPU has reached its bound, or VMOps
//     exceed VMShare of that turn's own plan;
//   - over the wall, environmental only when CPU is known to be under its bound
//     and measured VM operations are within VMShare of this turn's own plan.
//
// Process CPU is RUSAGE_SELF user+system for the whole process, so every
// goroutine is included; a wall overrun with low process CPU is attributed to
// the environment rather than to this turn's work. No production path classifies
// or logs anything: this is used by a test that already opted in.
func TestHoldVerdictFor(s TestHoldSample, p TestHoldPolicy) (TestHoldVerdict, string) {
	p = p.normalized()
	if over := s.overBudget(); len(over) > 0 {
		return TestHoldHard, "work overrun " + strings.Join(over, " ")
	}
	if s.Wall <= p.Wall {
		return TestHoldWithin, ""
	}
	if !s.ProcessCPUKnown {
		return TestHoldHard, fmt.Sprintf("wall=%s>%s process-cpu-unknown", s.Wall, p.Wall)
	}
	if !s.CostKnown {
		return TestHoldHard, fmt.Sprintf("wall=%s>%s cost-unknown", s.Wall, p.Wall)
	}
	if !s.BudgetKnown || s.Budget.VMOps <= 0 {
		return TestHoldHard, fmt.Sprintf("wall=%s>%s plan-vm-unknown", s.Wall, p.Wall)
	}
	vmLimit := int64(float64(s.Budget.VMOps) * p.VMShare)
	if s.ProcessCPU >= p.CPU {
		return TestHoldHard, fmt.Sprintf("wall=%s>%s cpu=%s>=%s", s.Wall, p.Wall, s.ProcessCPU, p.CPU)
	}
	if s.VMOps > vmLimit {
		return TestHoldHard, fmt.Sprintf("wall=%s>%s vm=%d>%d(%g%% of %d)", s.Wall, p.Wall, s.VMOps, vmLimit, p.VMShare*100, s.Budget.VMOps)
	}
	return TestHoldEnvironmental, fmt.Sprintf("wall=%s>%s cpu=%s<%s vm=%d<=%d(%g%% of %d)", s.Wall, p.Wall, s.ProcessCPU, p.CPU, s.VMOps, vmLimit, p.VMShare*100, s.Budget.VMOps)
}

// TestHoldAggregate is the fixed set of extremes one measurement window keeps.
// It is bounded: no per-turn samples are retained.
type TestHoldAggregate struct {
	// Samples counts opted-in turns recorded in the window.
	Samples int
	// Largest is the turn with the largest wall.
	Largest TestHoldSample
	// LargestOK reports whether Largest was recorded.
	LargestOK bool
	// MaxCPU is the turn with the largest known process CPU; unknown-CPU turns
	// never become MaxCPU.
	MaxCPU   TestHoldSample
	MaxCPUOK bool
	// WorstOverrun is the largest-wall turn that exceeded its own work plan.
	WorstOverrun   TestHoldSample
	WorstOverrunOK bool
	// WorstHeavy is the largest-wall turn that was over the wall bound and hard
	// on CPU, VM, or unknown measurement.
	WorstHeavy   TestHoldSample
	WorstHeavyOK bool
	// WorstEnvironmental is the largest-wall turn that was over the wall bound
	// and environmental.
	WorstEnvironmental   TestHoldSample
	WorstEnvironmentalOK bool
}

// Verdict is the window's verdict: any hard turn fails the window even if a
// longer environmental turn was also recorded.
func (a TestHoldAggregate) Verdict() TestHoldVerdict {
	if a.WorstOverrunOK || a.WorstHeavyOK {
		return TestHoldHard
	}
	if a.WorstEnvironmentalOK {
		return TestHoldEnvironmental
	}
	return TestHoldWithin
}

// WriterHoldCollector keeps the bounded extremes of one measurement window.
// It is created by WithTestWriterHolds; a window boundary gets a fresh one.
type WriterHoldCollector struct {
	mu        sync.Mutex
	policy    TestHoldPolicy
	largest   TestHoldSample
	seen      bool
	maxCPU    TestHoldSample
	cpuSeen   bool
	overrun   TestHoldSample
	overSeen  bool
	heavy     TestHoldSample
	heavySeen bool
	env       TestHoldSample
	envSeen   bool
	samples   int
}

// Snapshot returns the largest wall sample seen, and whether any opted-in write
// finished in this window. It is the original diagnostic API and is unchanged.
func (c *WriterHoldCollector) Snapshot() (TestHoldSample, bool) {
	if c == nil {
		return TestHoldSample{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.largest, c.seen
}

// Aggregate returns the bounded extremes of this window together with its
// verdict under the collector's policy.
func (c *WriterHoldCollector) Aggregate() TestHoldAggregate {
	if c == nil {
		return TestHoldAggregate{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return TestHoldAggregate{
		Samples:              c.samples,
		Largest:              c.largest,
		LargestOK:            c.seen,
		MaxCPU:               c.maxCPU,
		MaxCPUOK:             c.cpuSeen,
		WorstOverrun:         c.overrun,
		WorstOverrunOK:       c.overSeen,
		WorstHeavy:           c.heavy,
		WorstHeavyOK:         c.heavySeen,
		WorstEnvironmental:   c.env,
		WorstEnvironmentalOK: c.envSeen,
	}
}

type testHoldKey struct{}

// WithTestWriterHolds opts the writes carrying ctx into hold diagnostics. It is
// test-only: outside a test binary it returns ctx and a nil collector, so no
// production path can collect, time or log by accident.
func WithTestWriterHolds(ctx context.Context) (context.Context, *WriterHoldCollector) {
	return WithTestWriterHoldsPolicy(ctx, DefaultTestHoldPolicy())
}

// WithTestWriterHoldsPolicy is WithTestWriterHolds with an explicit verdict
// bound, for a gate whose approved wall, CPU or VM share differs. A policy with
// a non-positive field is normalized to DefaultTestHoldPolicy. It is test-only
// in the same way: outside a test binary it returns ctx and a nil collector.
func WithTestWriterHoldsPolicy(ctx context.Context, policy TestHoldPolicy) (context.Context, *WriterHoldCollector) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !testing.Testing() {
		return ctx, nil
	}
	collector := &WriterHoldCollector{policy: policy.normalized()}
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
// one of the window's bounded extremes, so a run of thousands of transactions
// does not pay to symbolise frames it will never report.
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
		Budget:          WriteCost{Rows: plan.Rows, Pages: plan.Pages, VMOps: plan.VMOps},
		BudgetKnown:     plan.Name != "",
		ProcessCPU:      h.cpu,
		ProcessCPUKnown: h.cpuOK,
	}, w.site)
}

// record classifies every sample as it arrives, so no policy evaluation depends
// on a later best sample, and keeps only the bounded extremes. Site resolution
// runs once, and only for a sample that becomes one of those extremes.
func (c *WriterHoldCollector) record(sample TestHoldSample, site callSite) {
	verdict, _ := TestHoldVerdictFor(sample, c.policy)
	over := sample.overBudget()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.samples++
	largest := !c.seen || sample.Wall > c.largest.Wall
	cpu := sample.ProcessCPUKnown && (!c.cpuSeen || sample.ProcessCPU > c.maxCPU.ProcessCPU ||
		sample.ProcessCPU == c.maxCPU.ProcessCPU && sample.Wall > c.maxCPU.Wall)
	overrun := len(over) > 0 && (!c.overSeen || sample.Wall > c.overrun.Wall)
	heavy := verdict == TestHoldHard && len(over) == 0 && (!c.heavySeen || sample.Wall > c.heavy.Wall)
	environmental := verdict == TestHoldEnvironmental && (!c.envSeen || sample.Wall > c.env.Wall)
	if !largest && !cpu && !overrun && !heavy && !environmental {
		return
	}
	function, file, line, _ := site.resolve()
	sample.Site = fmt.Sprintf("%s (%s:%d)", function, file, line)
	if largest {
		c.largest, c.seen = sample, true
	}
	if cpu {
		c.maxCPU, c.cpuSeen = sample, true
	}
	if overrun {
		c.overrun, c.overSeen = sample, true
	}
	if heavy {
		c.heavy, c.heavySeen = sample, true
	}
	if environmental {
		c.env, c.envSeen = sample, true
	}
}

// heldSinceNow reports when the current holder acquired the gate, so a sample
// measures the actual hold rather than the time this caller noticed it.
func (g *Gate) heldSinceNow() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.heldSince
}
