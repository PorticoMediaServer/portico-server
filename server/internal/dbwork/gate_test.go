package dbwork

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

// waitForWaiters blocks until the gate reports n queued waiters, so a test never
// depends on a sleep to establish ordering.
func waitForWaiters(t *testing.T, g *Gate, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		total := 0
		for _, count := range g.Waiting() {
			total += count
		}
		if total >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("gate never reported %d waiters: %#v", n, g.Waiting())
}

func TestGateStrictPriorityAcrossClasses(t *testing.T) {
	g := NewGate()
	hold, err := g.Acquire(context.Background(), ClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	order := []Class{}
	launched := 1
	var wg sync.WaitGroup
	// Queue the lowest priorities first: the gate must still serve the fence and
	// playback ahead of them when the holder releases.
	for _, class := range []Class{ClassMaintenance, ClassBackgroundMedia, ClassInteractive, ClassEstablishedPlayback, ClassSecurityFence} {
		wg.Add(1)
		go func(class Class) {
			defer wg.Done()
			release, err := g.Acquire(context.Background(), class)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			order = append(order, class)
			mu.Unlock()
			release()
		}(class)
		waitForWaiters(t, g, launched)
		launched++
	}
	hold()
	wg.Wait()
	want := []Class{ClassSecurityFence, ClassEstablishedPlayback, ClassInteractive, ClassBackgroundMedia, ClassMaintenance}
	if len(order) != len(want) {
		t.Fatalf("expected %d acquisitions, got %v", len(want), order)
	}
	for i, class := range want {
		if order[i] != class {
			t.Fatalf("priority order was %v, want %v", order, want)
		}
	}
}

func TestGatePromotesOldMaintenanceBehindPlaybackAndSecurity(t *testing.T) {
	g := NewGate()
	hold, err := g.Acquire(context.Background(), ClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	order := make(chan Class, 3)
	var wg sync.WaitGroup
	for i, class := range []Class{ClassMaintenance, ClassInteractive, ClassSecurityFence} {
		wg.Add(1)
		go func(class Class) {
			defer wg.Done()
			release, e := g.Acquire(context.Background(), class)
			if e != nil {
				t.Error(e)
				return
			}
			order <- class
			release()
		}(class)
		waitForWaiters(t, g, i+1)
	}
	g.mu.Lock()
	g.waitQueue[ClassMaintenance][0].since = time.Now().Add(-10 * GateAgePromotionInterval)
	g.mu.Unlock()
	hold()
	wg.Wait()
	close(order)
	got := []Class{}
	for class := range order {
		got = append(got, class)
	}
	want := []Class{ClassSecurityFence, ClassMaintenance, ClassInteractive}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("aged priority %v, want %v", got, want)
		}
	}
}

func TestAgedBackgroundWriterCompletesUnderSustainedRequests(t *testing.T) {
	g := NewGate()
	hold, err := g.Acquire(context.Background(), ClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	first := make(chan Class, 51)
	go func() {
		release, err := g.Acquire(context.Background(), ClassBackgroundMedia)
		if err == nil {
			first <- ClassBackgroundMedia
			release()
		}
		close(done)
	}()
	waitForWaiters(t, g, 1)
	g.mu.Lock()
	g.waitQueue[ClassBackgroundMedia][0].since = time.Now().Add(-3 * GateAgePromotionInterval)
	g.mu.Unlock()
	requestDone := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		go func() {
			release, err := g.Acquire(context.Background(), ClassInteractive)
			if err == nil {
				first <- ClassInteractive
				release()
			}
			requestDone <- struct{}{}
		}()
	}
	waitForWaiters(t, g, 51)
	hold()
	select {
	case class := <-first:
		if class != ClassBackgroundMedia {
			t.Fatal("a newly arrived request beat the aged background writer")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("aged background writer stalled under queued requests")
	}
	for i := 0; i < 50; i++ {
		<-requestDone
	}
}

func TestGateHandoffSurvivesPromotionBoundary(t *testing.T) {
	for n := 0; n < 100; n++ {
		g := NewGate()
		hold, err := g.Acquire(context.Background(), ClassInteractive)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		done := make(chan error, 2)
		for _, class := range []Class{ClassMaintenance, ClassBackgroundMedia} {
			go func(class Class) {
				release, err := g.Acquire(ctx, class)
				if err == nil {
					release()
				}
				done <- err
			}(class)
		}
		waitForWaiters(t, g, 2)
		g.mu.Lock()
		// An old maintenance waiter is exactly at the promotion edge when
		// the holder releases. Both waiters must still acquire in turn.
		g.waitQueue[ClassMaintenance][0].since = time.Now().Add(-GateAgePromotionInterval + time.Millisecond)
		g.mu.Unlock()
		hold()
		for i := 0; i < 2; i++ {
			if err := <-done; err != nil {
				t.Fatalf("free gate stalled on iteration %d: %v", n, err)
			}
		}
		cancel()
	}
}

func TestGateIsFIFOWithinAClass(t *testing.T) {
	g := NewGate()
	hold, err := g.Acquire(context.Background(), ClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	order := []int{}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		index := i
		go func() {
			defer wg.Done()
			release, err := g.Acquire(context.Background(), ClassBackgroundMedia)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			order = append(order, index)
			mu.Unlock()
			release()
		}()
		// Enqueue deterministically so the expected order is the arrival order.
		waitForWaiters(t, g, index+1)
	}
	hold()
	wg.Wait()
	for i, index := range order {
		if index != i {
			t.Fatalf("arrival order not preserved inside a class: %v", order)
		}
	}
}

func TestCancelledWaiterLeavesTheQueue(t *testing.T) {
	g := NewGate()
	hold, err := g.Acquire(context.Background(), ClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	defer hold()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := g.Acquire(ctx, ClassSecurityFence)
		done <- err
	}()
	waitForWaiters(t, g, 1)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter returned %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(g.Waiting()) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("cancelled waiter still holds a place: %#v", g.Waiting())
}

func TestCancelledWaiterDoesNotBlockLowerPriority(t *testing.T) {
	// A fence that gives up must not keep an interactive writer waiting for it.
	g := NewGate()
	hold, err := g.Acquire(context.Background(), ClassMaintenance)
	if err != nil {
		t.Fatal(err)
	}
	fenceCtx, cancelFence := context.WithCancel(context.Background())
	fenceDone := make(chan error, 1)
	go func() {
		_, err := g.Acquire(fenceCtx, ClassSecurityFence)
		fenceDone <- err
	}()
	waitForWaiters(t, g, 1)
	interactive := make(chan struct{})
	go func() {
		release, err := g.Acquire(context.Background(), ClassInteractive)
		if err != nil {
			t.Error(err)
			return
		}
		release()
		close(interactive)
	}()
	waitForWaiters(t, g, 2)
	cancelFence()
	<-fenceDone
	hold()
	select {
	case <-interactive:
	case <-time.After(5 * time.Second):
		t.Fatal("an abandoned fence blocked an interactive writer")
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	g := NewGate()
	release, err := g.Acquire(context.Background(), ClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	release()
	release()
	release()
	if g.ActiveOrWaiting() {
		t.Fatal("a repeated release corrupted the gate state")
	}
	second, err := g.Acquire(context.Background(), ClassInteractive)
	if err != nil {
		t.Fatalf("gate unusable after repeated release: %v", err)
	}
	second()
}

func TestActiveOrWaitingReportsQueuedWriters(t *testing.T) {
	g := NewGate()
	if g.ActiveOrWaiting() {
		t.Fatal("a fresh gate reported a writer")
	}
	release, err := g.Acquire(context.Background(), ClassMaintenance)
	if err != nil {
		t.Fatal(err)
	}
	if !g.ActiveOrWaiting() {
		t.Fatal("an active writer was not reported")
	}
	release()
	if g.ActiveOrWaiting() {
		t.Fatal("a released writer was still reported")
	}
}

func TestAcquireHonoursAnAlreadyCancelledContext(t *testing.T) {
	g := NewGate()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Acquire(ctx, ClassInteractive); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if g.ActiveOrWaiting() {
		t.Fatal("a cancelled acquire took the slot")
	}
}

func TestClassLadderIsOrderedAndNamed(t *testing.T) {
	previous := 0
	for _, class := range Classes() {
		if class.Priority() <= previous {
			t.Fatalf("class %s is out of order", class)
		}
		previous = class.Priority()
		parsed, ok := ParseClass(class.String())
		if !ok || parsed != class {
			t.Fatalf("class %s did not round-trip through its name", class)
		}
	}
	if !ClassBackgroundMedia.Background() || !ClassMaintenance.Background() {
		t.Fatal("bulk classes must report as background")
	}
	if ClassInteractive.Background() || ClassPlaybackStart.Background() {
		t.Fatal("foreground classes must not report as background")
	}
}

// A watchdog that cannot hand the process a replacement handle must never
// recycle: every service would keep the old *sql.DB and it would be closed
// under them.
func TestWatchdogWithoutSwapNeverRecycles(t *testing.T) {
	w := NewWatchdog(nil)
	w.Reopen = func() (*sql.DB, error) { return nil, errors.New("unused") }
	w.state = DegradedState
	if w.mayRecycleLocked(KindBusy) {
		t.Fatal("recycle allowed without a Swap")
	}
	w.Swap = func(*sql.DB) {}
	if !w.mayRecycleLocked(KindBusy) {
		t.Fatal("recycle refused although the process can adopt the new handle")
	}
	if w.mayRecycleLocked(KindCorrupt) {
		t.Fatal("corruption must never recycle")
	}
}
