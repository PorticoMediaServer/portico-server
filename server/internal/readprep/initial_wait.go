package readprep

import (
	"context"
	"testing"
	"time"
)

// InitialReadyGrace is a short opportunity to deliver an already-finishing
// tiny preparation in its initial request. It never changes a request deadline
// or the polling interval, and never runs a producer on the request goroutine.
const InitialReadyGrace = 50 * time.Millisecond

// Covers the timer and transient notification/control allocations.
const initialWaitBytes int64 = 512

// initialReadyWaitKey marks a context as a test fixture's opt-in for Poll's
// completion wait. It is private so only WaitForReadyForTest can set it.
type initialReadyWaitKey struct{}

// WaitForReadyForTest is a test-only HTTP-fixture lever. It asks the next Poll
// for the same caller and handle to wait on the already-admitted worker's
// completion channel before returning its ordinary result, so a fixture can
// observe the real HTTP poll without sleeping on Retry-After. It adds no
// deadline, cap, worker, pool or cache. Outside a test binary, or without an
// existing request deadline, it returns ctx unchanged; the marker is also
// ignored by Poll in those cases. Cancellation, retirement, caller binding,
// expiry and delivery charging remain Poll's.
func WaitForReadyForTest(ctx context.Context) context.Context {
	if !testing.Testing() {
		return ctx
	}
	if _, ok := ctx.Deadline(); !ok {
		return ctx
	}
	return context.WithValue(ctx, initialReadyWaitKey{}, true)
}

func waitForReady(ctx context.Context) bool {
	if !testing.Testing() {
		return false
	}
	if _, ok := ctx.Deadline(); !ok {
		return false
	}
	optIn, _ := ctx.Value(initialReadyWaitKey{}).(bool)
	return optIn
}

// PollWhenReady waits only for an initial submission's worker completion. It
// holds no database snapshot, delivery or store lock while waiting. Admission,
// computation, caller binding, expiry and delivery charging remain Poll's.
// Slow work returns the ordinary Preparing result after the fixed short grace.
func (s *Store) PollWhenReady(ctx context.Context, caller Caller, h Handle, grace time.Duration) (Result, error) {
	if grace <= 0 {
		return s.Poll(ctx, caller, h)
	}
	if grace > InitialReadyGrace {
		grace = InitialReadyGrace
	}
	if err := s.waitInitialReady(ctx, caller, h, grace); err != nil {
		return Result{}, err
	}
	return s.Poll(ctx, caller, h)
}

// waitInitialReady waits only for an initial submission's worker completion. It
// holds no database snapshot, delivery or store lock while waiting. Admission,
// computation, caller binding, expiry and delivery charging remain Poll's.
// A positive grace bounds the wait; zero waits until the worker completes,
// fails, retires or ctx ends. Only the test-only Poll opt-in passes zero.
func (s *Store) waitInitialReady(ctx context.Context, caller Caller, h Handle, grace time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.prune(s.cfg.Now())
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	l := s.handles[h]
	if l == nil || caller == "" || l.caller != caller {
		s.mu.Unlock()
		return ErrStale
	}
	if l.e.complete || l.e.err != nil || l.waiting || !s.room(initialWaitBytes, l.e) {
		s.mu.Unlock()
		return nil
	}
	// One initial waiter per private handle. Its transient control reservation
	// remains charged even when cancellation retires the handle underneath it.
	l.waiting = true
	s.initialWaiters++
	s.used += initialWaitBytes
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		l.waiting = false
		s.used -= initialWaitBytes
		s.initialWaiters--
		if s.initialWaiters == 0 {
			s.retireChanged = nil
		}
		s.mu.Unlock()
	}()
	var expired <-chan time.Time
	if grace > 0 {
		timer := time.NewTimer(grace)
		defer timer.Stop()
		expired = timer.C
	}
	for {
		s.mu.Lock()
		s.prune(s.cfg.Now())
		if s.closed {
			s.mu.Unlock()
			return ErrClosed
		}
		if s.handles[h] != l {
			s.mu.Unlock()
			return ErrStale
		}
		if l.e.complete || l.e.err != nil {
			s.mu.Unlock()
			return nil
		}
		if l.e.ready == nil {
			l.e.ready = make(chan struct{})
		}
		if s.retireChanged == nil {
			s.retireChanged = make(chan struct{})
		}
		ready, retired := l.e.ready, s.retireChanged
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-expired:
			return nil
		case <-ready:
		case <-retired:
		}
	}
}

// Called with mu held. Completion channels have no payload and fit the fixed
// per-entry metadata charge. Retirement also wakes waiters on a single released
// handle when other callers still own the shared computation.
func (s *Store) notifyRetirementLocked() {
	if s.initialWaiters > 0 && s.retireChanged != nil {
		close(s.retireChanged)
		s.retireChanged = make(chan struct{})
	}
}
func signalEntryReadyLocked(e *entry) {
	if e.ready != nil {
		close(e.ready)
		e.ready = nil
	}
}
