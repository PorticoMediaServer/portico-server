package app

import (
	"context"
	"errors"
	"sync"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

var errInvalidWorkClass = errors.New("work class is not declared by the Foundation contract")

// sqliteWriteScheduler preserves SQLite's one-writer physical governor while
// using the canonical Foundation class for semantic ordering. A waiting
// security fence is therefore selected before every ordinary queued mutation;
// an already-running SQLite transaction is allowed to reach its atomic commit.
type sqliteWriteScheduler struct {
	mu        sync.Mutex
	active    bool
	nextID    uint64
	waitQueue map[foundationcontract.WorkClass][]uint64
	notify    chan struct{}
}

func (q *sqliteWriteScheduler) acquire(ctx context.Context, class foundationcontract.WorkClass) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !class.Valid() {
		return nil, errInvalidWorkClass
	}

	q.mu.Lock()
	if q.notify == nil {
		q.notify = make(chan struct{})
	}
	if q.waitQueue == nil {
		q.waitQueue = map[foundationcontract.WorkClass][]uint64{}
	}
	q.nextID++
	waiterID := q.nextID
	q.waitQueue[class] = append(q.waitQueue[class], waiterID)
	for {
		if !q.active && q.firstWaiter(class) == waiterID && !q.higherPriorityWaiting(class) {
			q.waitQueue[class] = q.waitQueue[class][1:]
			q.active = true
			q.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					q.mu.Lock()
					q.active = false
					q.signalLocked()
					q.mu.Unlock()
				})
			}, nil
		}
		notify := q.notify
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			q.mu.Lock()
			q.removeWaiter(class, waiterID)
			q.signalLocked()
			q.mu.Unlock()
			return nil, ctx.Err()
		case <-notify:
			q.mu.Lock()
		}
	}
}

func (q *sqliteWriteScheduler) higherPriorityWaiting(class foundationcontract.WorkClass) bool {
	priority := class.Priority()
	for _, candidate := range foundationcontract.CanonicalWorkClasses() {
		if candidate.Priority() < priority && len(q.waitQueue[candidate]) > 0 {
			return true
		}
	}
	return false
}

func (q *sqliteWriteScheduler) firstWaiter(class foundationcontract.WorkClass) uint64 {
	if len(q.waitQueue[class]) == 0 {
		return 0
	}
	return q.waitQueue[class][0]
}

func (q *sqliteWriteScheduler) removeWaiter(class foundationcontract.WorkClass, waiterID uint64) {
	for index, candidate := range q.waitQueue[class] {
		if candidate == waiterID {
			q.waitQueue[class] = append(q.waitQueue[class][:index], q.waitQueue[class][index+1:]...)
			return
		}
	}
}

func (q *sqliteWriteScheduler) signalLocked() {
	if q.notify == nil {
		q.notify = make(chan struct{})
		return
	}
	close(q.notify)
	q.notify = make(chan struct{})
}

func (q *sqliteWriteScheduler) waiting(class foundationcontract.WorkClass) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.waitQueue[class])
}

func (q *sqliteWriteScheduler) activeOrWaiting() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active {
		return true
	}
	for _, class := range foundationcontract.CanonicalWorkClasses() {
		if len(q.waitQueue[class]) > 0 {
			return true
		}
	}
	return false
}
