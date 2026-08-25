package app

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"
)

// kdfCallsite is deliberately not a free-form string at callers.  Every
// password/PIN operation must declare which product boundary is spending the
// process-global bcrypt budget, so a newly added path cannot silently create a
// second, unbounded CPU pool.
type kdfCallsite string

const (
	kdfAccountSetupHash        kdfCallsite = "account.setup.hash"
	kdfBrowserLoginCompare     kdfCallsite = "auth.browser.compare"
	kdfNativeLoginCompare      kdfCallsite = "auth.native.compare"
	kdfPasswordChangeCompare   kdfCallsite = "account.password.compare"
	kdfPasswordChangeHash      kdfCallsite = "account.password.hash"
	kdfProfileReauthCompare    kdfCallsite = "profile.admin.compare"
	kdfRestoreReauthCompare    kdfCallsite = "restore.compare"
	kdfUserCreateHash          kdfCallsite = "user.create.hash"
	kdfUserUpdateHash          kdfCallsite = "user.update.hash"
	kdfProfilePINSetHash       kdfCallsite = "profile.pin.set.hash"
	kdfProfilePINSelectCompare kdfCallsite = "profile.pin.select.compare"
	kdfProfilePINAdminCompare  kdfCallsite = "profile.pin.admin.compare"
	kdfPasswordUpgradeHash     kdfCallsite = "account.password.upgrade.hash"
)

var (
	errKDFUnavailable = errors.New("credential verification capacity is temporarily unavailable")
	errKDFCancelled   = errors.New("credential verification was cancelled")
	errKDFCallsite    = errors.New("credential verification callsite is not declared")
)

const kdfRetryAfter = time.Second

type kdfLane uint8

const (
	kdfLaneCompare kdfLane = iota
	kdfLaneMutation
)

type kdfWaiter struct {
	lane    kdfLane
	ctx     context.Context
	ready   chan struct{}
	granted bool
}

// kdfAdmission is a bounded, process-global scheduler.  It has one total CPU
// ceiling; lane reservations are carved out of that ceiling rather than added
// to it.  Waiters block on their own channel and are dispatched by release --
// there is no goroutine per queued request.
type kdfAdmission struct {
	mu sync.Mutex

	capacity      int
	compareLimit  int
	queueCapacity int
	compareQueue  int
	mutationQueue int

	active         int
	activeCompare  int
	activeMutation int
	nextLane       kdfLane
	waiters        []*kdfWaiter
}

func newKDFAdmission(capacity, queueCapacity int) *kdfAdmission {
	if capacity < 2 {
		capacity = 2
	}
	if queueCapacity < 2 {
		queueCapacity = 2
	}
	return &kdfAdmission{
		capacity: capacity, compareLimit: capacity - 1, queueCapacity: queueCapacity,
		compareQueue:  queueCapacity - max(1, queueCapacity/4),
		mutationQueue: max(1, queueCapacity/4), nextLane: kdfLaneCompare,
	}
}

func defaultKDFCapacity() int {
	capacity := runtime.GOMAXPROCS(0) / 2
	if capacity < 2 {
		return 2
	}
	if capacity > 4 {
		return 4
	}
	return capacity
}

var processKDFAdmission = newKDFAdmission(defaultKDFCapacity(), 64)

func (a *kdfAdmission) acquire(ctx context.Context, lane kdfLane) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(errKDFCancelled, err)
	}
	a.mu.Lock()
	// Context cancellation is checked while holding the admission lock.  This
	// closes the immediate-grant window: a request that is already cancelled
	// must not spend a bcrypt slot even when capacity is available.
	if err := ctx.Err(); err != nil {
		a.mu.Unlock()
		return nil, errors.Join(errKDFCancelled, err)
	}
	if a.canGrantLocked(lane) && len(a.waiters) == 0 {
		a.grantLocked(lane)
		a.mu.Unlock()
		return a.releaseFunc(lane), nil
	}
	if len(a.waiters) >= a.queueCapacity || (lane == kdfLaneCompare && a.queuedLocked(lane) >= a.compareQueue) || (lane == kdfLaneMutation && a.queuedLocked(lane) >= a.mutationQueue) {
		a.mu.Unlock()
		return nil, errKDFUnavailable
	}
	waiter := &kdfWaiter{lane: lane, ctx: ctx, ready: make(chan struct{})}
	a.waiters = append(a.waiters, waiter)
	a.dispatchLocked()
	a.mu.Unlock()

	select {
	case <-waiter.ready:
		// select may choose ready after cancellation won the race.  Return the
		// cancellation and release the grant before the caller can run bcrypt.
		if err := ctx.Err(); err != nil {
			a.mu.Lock()
			if waiter.granted {
				a.releaseLocked(lane)
				waiter.granted = false
				a.dispatchLocked()
			}
			a.mu.Unlock()
			return nil, errors.Join(errKDFCancelled, err)
		}
		return a.releaseFunc(lane), nil
	case <-ctx.Done():
		a.mu.Lock()
		if waiter.granted {
			a.releaseLocked(lane)
			waiter.granted = false
			a.dispatchLocked()
			a.mu.Unlock()
		} else {
			for i, queued := range a.waiters {
				if queued == waiter {
					a.waiters = append(a.waiters[:i], a.waiters[i+1:]...)
					break
				}
			}
			a.mu.Unlock()
		}
		return nil, errors.Join(errKDFCancelled, ctx.Err())
	}
}

func (a *kdfAdmission) queuedLocked(lane kdfLane) int {
	count := 0
	for _, waiter := range a.waiters {
		if waiter.lane == lane {
			count++
		}
	}
	return count
}

func (a *kdfAdmission) canGrantLocked(lane kdfLane) bool {
	if a.active >= a.capacity {
		return false
	}
	if lane == kdfLaneCompare {
		// One slot always remains available to authenticated credential/PIN
		// mutation work, so an unauthenticated login flood cannot consume it.
		return a.activeCompare < a.compareLimit
	}
	if a.hasQueuedLaneLocked(kdfLaneCompare) {
		return a.activeMutation < a.capacity-1
	}
	return true
}

func (a *kdfAdmission) hasQueuedLaneLocked(lane kdfLane) bool {
	for _, waiter := range a.waiters {
		if waiter.lane == lane {
			return true
		}
	}
	return false
}

func (a *kdfAdmission) dispatchLocked() {
	for a.active < a.capacity && len(a.waiters) > 0 {
		index := a.nextGrantIndexLocked()
		if index < 0 {
			return
		}
		waiter := a.waiters[index]
		a.waiters = append(a.waiters[:index], a.waiters[index+1:]...)
		// A cancelled waiter can be removed without ever being granted.  Its
		// acquire call is already observing ctx.Done and will account for the
		// cancellation; importantly, no bcrypt operation can follow this path.
		if err := waiter.ctx.Err(); err != nil {
			continue
		}
		a.grantLocked(waiter.lane)
		waiter.granted = true
		a.nextLane = kdfLaneCompare
		if waiter.lane == kdfLaneCompare {
			a.nextLane = kdfLaneMutation
		}
		close(waiter.ready)
	}
}

func (a *kdfAdmission) nextGrantIndexLocked() int {
	for _, lane := range []kdfLane{a.nextLane, 1 - a.nextLane} {
		if !a.canGrantLocked(lane) {
			continue
		}
		for i, waiter := range a.waiters {
			if waiter.lane == lane {
				return i
			}
		}
	}
	return -1
}

func (a *kdfAdmission) grantLocked(lane kdfLane) {
	a.active++
	if lane == kdfLaneCompare {
		a.activeCompare++
	} else {
		a.activeMutation++
	}
}

func (a *kdfAdmission) releaseLocked(lane kdfLane) {
	a.active--
	if lane == kdfLaneCompare {
		a.activeCompare--
	} else {
		a.activeMutation--
	}
}

func (a *kdfAdmission) releaseFunc(lane kdfLane) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			a.releaseLocked(lane)
			a.dispatchLocked()
			a.mu.Unlock()
		})
	}
}

func runKDF[T any](ctx context.Context, callsite kdfCallsite, lane kdfLane, operation func() (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	if !validKDFCallsite(callsite, lane) {
		return zero, errKDFCallsite
	}
	if err := ctx.Err(); err != nil {
		return zero, errors.Join(errKDFCancelled, err)
	}
	release, err := processKDFAdmission.acquire(ctx, lane)
	if err != nil {
		return zero, err
	}
	defer release()
	// This is deliberately immediately before the expensive callback.  The
	// admission grant is not a credential authorization and cancellation after
	// wake must still execute zero bcrypt work.
	if err := ctx.Err(); err != nil {
		return zero, errors.Join(errKDFCancelled, err)
	}
	return operation()
}

func validKDFCallsite(callsite kdfCallsite, lane kdfLane) bool {
	switch callsite {
	case kdfAccountSetupHash, kdfBrowserLoginCompare, kdfNativeLoginCompare, kdfPasswordChangeCompare,
		kdfProfileReauthCompare, kdfRestoreReauthCompare, kdfProfilePINSelectCompare,
		kdfProfilePINAdminCompare:
		return lane == kdfLaneCompare
	case kdfPasswordChangeHash, kdfUserCreateHash, kdfUserUpdateHash,
		kdfProfilePINSetHash, kdfPasswordUpgradeHash:
		return lane == kdfLaneMutation
	default:
		return false
	}
}

func kdfLaneForCallsite(callsite kdfCallsite) kdfLane {
	if validKDFCallsite(callsite, kdfLaneCompare) {
		return kdfLaneCompare
	}
	return kdfLaneMutation
}
