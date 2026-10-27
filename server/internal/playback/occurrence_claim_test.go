package playback

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestOccurrenceClaimFencesLateIntentAndAbortReopens(t *testing.T) {
	var r occurrenceRegistry
	old, err := r.intent("media-1")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := r.claim("media-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = old.install(&occurrenceRun{generation: 1}, func() { t.Fatal("old intent published") }); !errors.Is(err, errOccurrenceClaimed) {
		t.Fatalf("late publication: %v", err)
	}
	claim.abort()
	if old.current() {
		t.Fatal("aborted claim revived the old epoch")
	}
	old.release()
	fresh, err := r.intent("media-1")
	if err != nil {
		t.Fatal(err)
	}
	fresh.release()
}

func TestOccurrenceClaimCommitOwnsExactRunsAndDoesNotReopen(t *testing.T) {
	var r occurrenceRegistry
	var oldCancelled, successorCancelled atomic.Int32
	old, err := r.intent("media-1")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	oldRun := &occurrenceRun{generation: 1, cancel: func() { oldCancelled.Add(1); close(done) }, done: done}
	if err = old.install(oldRun, func() {}); err != nil {
		t.Fatal(err)
	}
	other := &occurrenceRun{generation: 2, cancel: func() { successorCancelled.Add(1) }}
	if err = old.install(other, func() {}); err != nil {
		t.Fatal(err)
	}
	claim, err := r.claim("media-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := r.claim("media-1", 1)
	if err != nil || claim != duplicate {
		t.Fatalf("claim copies: %v", err)
	}
	if err = claim.commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	claim.abort()
	if err = duplicate.commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if oldCancelled.Load() != 1 || successorCancelled.Load() != 0 {
		t.Fatalf("wrong cancellations %d %d", oldCancelled.Load(), successorCancelled.Load())
	}
	if _, err = r.intent("media-1"); !errors.Is(err, errOccurrenceClaimed) {
		t.Fatalf("committed claim reopened: %v", err)
	}
	old.release()
	r.finished("media-1", old.entry, oldRun)
	r.finished("media-1", old.entry, other)
	fresh, err := r.intent("media-1")
	if err != nil {
		t.Fatal(err)
	}
	fresh.release()
}

func TestOccurrenceClaimTimeoutDoesNotAbortCommittedFence(t *testing.T) {
	var r occurrenceRegistry
	i, err := r.intent("media-1")
	if err != nil {
		t.Fatal(err)
	}
	run := &occurrenceRun{generation: 1, cancel: func() {}, done: make(chan struct{})}
	if err = i.install(run, func() {}); err != nil {
		t.Fatal(err)
	}
	claim, err := r.claim("media-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	if err = claim.commit(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("join: %v", err)
	}
	claim.abort()
	if _, err = r.intent("media-1"); !errors.Is(err, errOccurrenceClaimed) {
		t.Fatalf("timed-out committed fence reopened: %v", err)
	}
	i.release()
	r.finished("media-1", i.entry, run)
}
