package ffmpegsupervisor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeLauncher struct {
	mu        sync.Mutex
	processes []*fakeProcess
	err       error
}

type gatedLauncher struct {
	fakeLauncher
	firstStarted chan struct{}
	releaseFirst chan struct{}
	startCalls   atomic.Int32
	startedOnce  sync.Once
}

func (l *gatedLauncher) Start(ctx context.Context, spec LaunchSpec) (Process, error) {
	if l.startCalls.Add(1) == 1 {
		l.startedOnce.Do(func() { close(l.firstStarted) })
		<-l.releaseFirst
	}
	return l.fakeLauncher.Start(ctx, spec)
}

func (l *fakeLauncher) Start(context.Context, LaunchSpec) (Process, error) {
	if l.err != nil {
		return nil, l.err
	}
	p := &fakeProcess{done: make(chan struct{})}
	l.mu.Lock()
	l.processes = append(l.processes, p)
	l.mu.Unlock()
	return p, nil
}
func (l *fakeLauncher) at(i int) *fakeProcess {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.processes[i]
}

type fakeProcess struct {
	done            chan struct{}
	once            sync.Once
	err             error
	interrupts      atomic.Int32
	kills           atomic.Int32
	exitOnInterrupt atomic.Bool
}

func (p *fakeProcess) Wait() error { <-p.done; return p.err }
func (p *fakeProcess) Interrupt() error {
	p.interrupts.Add(1)
	if p.exitOnInterrupt.Load() {
		p.exit(nil)
	}
	return nil
}
func (p *fakeProcess) Kill() error    { p.kills.Add(1); p.exit(errors.New("killed")); return nil }
func (p *fakeProcess) exit(err error) { p.once.Do(func() { p.err = err; close(p.done) }) }

func newTestSupervisor(t *testing.T, l *fakeLauncher, cfg Config) *Supervisor {
	t.Helper()
	if cfg.InterruptGrace == 0 {
		cfg.InterruptGrace = time.Millisecond
	}
	s := New(context.Background(), l, cfg)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	})
	return s
}

func start(t *testing.T, s *Supervisor, key string, release ReleaseFunc) Snapshot {
	t.Helper()
	got, err := s.Start(StartOptions{Key: key, Mode: ModeVOD, Command: LaunchSpec{Executable: "ffmpeg", Args: []string{"-secret"}, Env: []string{"TOKEN=secret"}}, Release: release})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func eventually(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestNormalExitReleasesExactlyOnce(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{})
	var calls atomic.Int32
	g := start(t, s, "movie", func(Release) { calls.Add(1) })
	l.at(0).exit(nil)
	eventually(t, func() bool { return calls.Load() == 1 })
	if err := s.Stop(g.Key, g.Generation); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stop after exit = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("release calls=%d", calls.Load())
	}
}

func TestLaunchFailureReleasesAndDoesNotPublishGeneration(t *testing.T) {
	boom := errors.New("launch failed")
	l := &fakeLauncher{err: boom}
	s := newTestSupervisor(t, l, Config{})
	var got Release
	_, err := s.Start(StartOptions{Key: "x", Mode: ModeVOD, Release: func(r Release) { got = r }})
	if !errors.Is(err, boom) || !errors.Is(got.Err, boom) || len(s.Snapshots()) != 0 {
		t.Fatalf("err=%v release=%+v snapshots=%v", err, got, s.Snapshots())
	}
}

func TestConcurrentStartReservesKeyAndJoinsOneLaunch(t *testing.T) {
	l := &gatedLauncher{firstStarted: make(chan struct{}), releaseFirst: make(chan struct{})}
	s := newTestSupervisor(t, &l.fakeLauncher, Config{})
	// Use the gated launcher directly after constructing the supervisor so the
	// test can force the launch window while the supervisor key is reserved.
	s.launcher = l
	type result struct {
		snapshot Snapshot
		err      error
	}
	results := make(chan result, 2)
	startOne := func() {
		got, err := s.Start(StartOptions{Key: "duplicate", Mode: ModeVOD, Command: LaunchSpec{Executable: "ffmpeg"}})
		results <- result{snapshot: got, err: err}
	}
	go startOne()
	select {
	case <-l.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first launch did not reach the barrier")
	}
	go startOne()
	time.Sleep(10 * time.Millisecond)
	close(l.releaseFirst)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("start results = %+v / %+v", first, second)
	}
	if l.startCalls.Load() != 1 {
		t.Fatalf("launcher start calls = %d, expected one", l.startCalls.Load())
	}
	if first.snapshot.Generation != second.snapshot.Generation || first.snapshot.Key != second.snapshot.Key {
		t.Fatalf("joined snapshots = %+v / %+v", first.snapshot, second.snapshot)
	}
}

func TestSweepMarksStall(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{StallAfter: time.Minute})
	g := start(t, s, "x", nil)
	s.Sweep(g.LastProgressAt.Add(time.Minute))
	if got := s.Snapshots(); len(got) != 1 || !got[0].Stalled {
		t.Fatalf("snapshots=%+v", got)
	}
	if err := s.Progress("x", g.Generation, g.LastProgressAt.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if s.Snapshots()[0].Stalled {
		t.Fatal("progress did not clear stall")
	}
}

func TestInterruptIgnoredThenKill(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{InterruptGrace: time.Millisecond})
	g := start(t, s, "x", nil)
	p := l.at(0)
	if err := s.Stop("x", g.Generation); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return p.kills.Load() == 1 })
	if p.interrupts.Load() != 1 {
		t.Fatalf("interrupts=%d", p.interrupts.Load())
	}
}

func TestConcurrentStopIsIdempotent(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{InterruptGrace: time.Second})
	g := start(t, s, "x", nil)
	p := l.at(0)
	p.exitOnInterrupt.Store(true)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); _ = s.Stop("x", g.Generation) }()
	}
	wg.Wait()
	eventually(t, func() bool { return p.interrupts.Load() == 1 })
}

func TestAbandonmentUsesModeDeadline(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{VODAbandonment: time.Minute, LiveAbandonment: 2 * time.Minute, InterruptGrace: time.Second})
	v := start(t, s, "vod", nil)
	live, err := s.Start(StartOptions{Key: "live", Mode: ModeLive})
	if err != nil {
		t.Fatal(err)
	}
	l.at(0).exitOnInterrupt.Store(true)
	l.at(1).exitOnInterrupt.Store(true)
	s.Sweep(v.LastActivityAt.Add(time.Minute))
	eventually(t, func() bool { return l.at(0).interrupts.Load() == 1 })
	if l.at(1).interrupts.Load() != 0 {
		t.Fatal("live generation abandoned at VOD deadline")
	}
	s.Sweep(live.LastActivityAt.Add(2 * time.Minute))
	eventually(t, func() bool { return l.at(1).interrupts.Load() == 1 })
}

func TestVODAbandonmentIsAutonomous(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{VODAbandonment: 5 * time.Millisecond, InterruptGrace: time.Second})
	_ = start(t, s, "vod", nil)
	p := l.at(0)
	p.exitOnInterrupt.Store(true)
	eventually(t, func() bool { return p.interrupts.Load() == 1 })
}

func TestVODAbandonmentStartsWhenLastClientLeaves(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{VODAbandonment: 15 * time.Millisecond, InterruptGrace: time.Second})
	g := start(t, s, "vod", nil)
	p := l.at(0)
	p.exitOnInterrupt.Store(true)
	if err := s.ClientActivity(g.Key, g.Generation, 1, time.Time{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if p.interrupts.Load() != 0 {
		t.Fatal("attached client did not suppress abandonment")
	}
	if err := s.ClientActivity(g.Key, g.Generation, -1, time.Time{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if p.interrupts.Load() != 0 {
		t.Fatal("generation was abandoned before the final-reader deadline")
	}
	eventually(t, func() bool { return p.interrupts.Load() == 1 })
}

func TestReplacementCreatesDiscontinuityAndStopsPrior(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{InterruptGrace: time.Second})
	first := start(t, s, "x", nil)
	l.at(0).exitOnInterrupt.Store(true)
	second := start(t, s, "x", nil)
	if second.Generation != first.Generation+1 || !second.Discontinuity {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	eventually(t, func() bool { return l.at(0).interrupts.Load() == 1 })
	got := s.Snapshots()
	if len(got) != 1 || got[0].Generation != second.Generation {
		t.Fatalf("snapshots=%+v", got)
	}
}

func TestShutdownStopsAndJoins(t *testing.T) {
	l := &fakeLauncher{}
	s := New(context.Background(), l, Config{InterruptGrace: time.Second})
	_ = start(t, s, "a", nil)
	_ = start(t, s, "b", nil)
	l.at(0).exitOnInterrupt.Store(true)
	l.at(1).exitOnInterrupt.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if len(s.Snapshots()) != 0 {
		t.Fatal("shutdown returned before process waiters joined")
	}
	if _, err := s.Start(StartOptions{Key: "c", Mode: ModeVOD}); !errors.Is(err, ErrClosed) {
		t.Fatalf("start after shutdown=%v", err)
	}
}

func TestWaitersCancelAndObserveSignals(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{})
	g := start(t, s, "x", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.WaitManifest(ctx, "x", g.Generation, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait=%v", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.WaitSegment(context.Background(), "x", g.Generation, 4) }()
	if err := s.SegmentReady("x", g.Generation, 4); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter not released")
	}
}

func TestProcessExitCancelsManifestAndSegmentWaiters(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{})
	g := start(t, s, "x", nil)
	manifest := make(chan error, 1)
	segment := make(chan error, 1)
	go func() { manifest <- s.WaitManifest(context.Background(), g.Key, g.Generation, 1) }()
	go func() { segment <- s.WaitSegment(context.Background(), g.Key, g.Generation, 1) }()
	l.at(0).exit(nil)
	for name, result := range map[string]<-chan error{"manifest": manifest, "segment": segment} {
		select {
		case err := <-result:
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("%s waiter error = %v", name, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s waiter was not cancelled", name)
		}
	}
}

func TestSnapshotsAreBoundedAndContainNoLaunchMaterial(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{SnapshotLimit: 1})
	_ = start(t, s, "b", nil)
	_ = start(t, s, "a", nil)
	got := s.Snapshots()
	if len(got) != 1 || got[0].Key != "a" {
		t.Fatalf("snapshots=%+v", got)
	}
}

func TestReleaseStillExactOnceAcrossConcurrentStopAndExit(t *testing.T) {
	l := &fakeLauncher{}
	s := newTestSupervisor(t, l, Config{InterruptGrace: time.Second})
	var calls atomic.Int32
	g := start(t, s, "x", func(Release) { calls.Add(1) })
	p := l.at(0)
	p.exitOnInterrupt.Store(true)
	go func() { _ = s.Stop("x", g.Generation) }()
	go p.exit(nil)
	eventually(t, func() bool { return calls.Load() == 1 })
	time.Sleep(time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
}
