package dbwork

import (
	"context"
	"testing"
	"time"
)

func TestStuckWaitPanicRemovesWaiterAndLeavesGateUsable(t *testing.T) {
	g := NewGate()
	hold, err := g.Acquire(context.Background(), ClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	defer hold()
	stuck := make(chan time.Time, 1)
	panicked := make(chan any, 1)
	go func() {
		defer func() { panicked <- recover() }()
		release, _ := g.acquire(context.Background(), ClassBackgroundMedia, stuck)
		if release != nil {
			release()
		}
	}()
	waitForWaiters(t, g, 1)
	stuck <- time.Now()
	select {
	case value := <-panicked:
		if value == nil {
			t.Fatal("stuck waiter did not panic")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stuck waiter did not unwind")
	}
	for class, count := range g.Waiting() {
		if count != 0 {
			t.Fatalf("panic left %d waiters in %s", count, class)
		}
	}
	hold()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	release, err := g.Acquire(ctx, ClassBackgroundMedia)
	if err != nil {
		t.Fatalf("abandoned waiter stranded the next writer: %v", err)
	}
	release()
}
