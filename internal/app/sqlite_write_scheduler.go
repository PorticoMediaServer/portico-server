package app

import (
	"context"
	"sync"
)

// sqliteWritePriority is ordered from the work that must be protected most to
// the work that must yield first. SQLite still has one writer; this scheduler
// decides which Portico writer is allowed to compete for it next.
type sqliteWritePriority uint8

const (
	sqliteWritePlayback sqliteWritePriority = iota
	sqliteWriteInteractive
	sqliteWriteBackground
	sqliteWritePriorityCount
)

type sqliteWriteScheduler struct {
	mu        sync.Mutex
	active    bool
	nextID    uint64
	waitQueue [sqliteWritePriorityCount][]uint64
	notify    chan struct{}
}

func (q *sqliteWriteScheduler) acquire(ctx context.Context, priority sqliteWritePriority) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if priority >= sqliteWritePriorityCount {
		priority = sqliteWriteBackground
	}

	q.mu.Lock()
	if q.notify == nil {
		q.notify = make(chan struct{})
	}
	q.nextID++
	waiterID := q.nextID
	q.waitQueue[priority] = append(q.waitQueue[priority], waiterID)
	for {
		if !q.active && q.firstWaiter(priority) == waiterID && !q.higherPriorityWaiting(priority) {
			q.waitQueue[priority] = q.waitQueue[priority][1:]
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
			q.removeWaiter(priority, waiterID)
			q.signalLocked()
			q.mu.Unlock()
			return nil, ctx.Err()
		case <-notify:
			q.mu.Lock()
		}
	}
}

func (q *sqliteWriteScheduler) higherPriorityWaiting(priority sqliteWritePriority) bool {
	for candidate := sqliteWritePlayback; candidate < priority; candidate++ {
		if len(q.waitQueue[candidate]) > 0 {
			return true
		}
	}
	return false
}

func (q *sqliteWriteScheduler) firstWaiter(priority sqliteWritePriority) uint64 {
	if len(q.waitQueue[priority]) == 0 {
		return 0
	}
	return q.waitQueue[priority][0]
}

func (q *sqliteWriteScheduler) removeWaiter(priority sqliteWritePriority, waiterID uint64) {
	for index, candidate := range q.waitQueue[priority] {
		if candidate == waiterID {
			q.waitQueue[priority] = append(q.waitQueue[priority][:index], q.waitQueue[priority][index+1:]...)
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

func (q *sqliteWriteScheduler) pressure() (playback, interactive int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.waitQueue[sqliteWritePlayback]), len(q.waitQueue[sqliteWriteInteractive])
}

func (q *sqliteWriteScheduler) activeOrWaiting() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active {
		return true
	}
	for priority := sqliteWritePlayback; priority < sqliteWritePriorityCount; priority++ {
		if len(q.waitQueue[priority]) > 0 {
			return true
		}
	}
	return false
}
