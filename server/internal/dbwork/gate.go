package dbwork

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// StuckWait is how long a waiter tolerates a holder before it treats the hold as
// a leak rather than contention. A gated transaction should never run for
// anything near this long: the batch sizes and the rule against doing
// filesystem, process or network work inside a transaction both exist to keep
// holds in the millisecond range.
const StuckWait = 20 * time.Second

// A waiting background batch earns one rung each second. Once it reaches the
// request rung it wins ties by arrival time, so a stream of new requests cannot
// starve it. Security, capture and playback writes remain ahead at every age.
const GateAgePromotionInterval = time.Second

// Gate preserves SQLite's one-writer physical governor while using the class
// ladder for semantic ordering. A waiting security fence is therefore selected
// before every ordinary queued mutation; a transaction that has already started
// is allowed to reach its atomic commit.
//
// The gate is an application-level lock, not a database one. It exists so
// contention is resolved in Go, where priority is expressible, instead of inside
// SQLite, where the loser only learns it lost by receiving SQLITE_BUSY.
type Gate struct {
	mu        sync.Mutex
	active    bool
	grantID   uint64
	nextID    uint64
	waitQueue map[Class][]waiter
	notify    chan struct{}
	// maxQueueAge is the longest any waiter of a class has ever queued.
	maxQueueAge map[Class]uint64

	holder    string
	acquired  atomic.Uint64
	waited    atomic.Uint64
	waitNanos atomic.Uint64
	cancelled atomic.Uint64
	heldNanos atomic.Uint64
	maxHeld   atomic.Uint64
}

// waiter is one queued acquisition: its ticket, and when it started waiting.
type waiter struct {
	id    uint64
	since time.Time
}

// NewGate returns a gate with no holder.
func NewGate() *Gate {
	return &Gate{waitQueue: map[Class][]waiter{}, maxQueueAge: map[Class]uint64{}, notify: make(chan struct{})}
}

// Acquire blocks until this caller owns the single write slot, or ctx ends. The
// returned release is idempotent: calling it twice is harmless, which is what
// lets a deferred release sit alongside an explicit one on the commit path.
//
// A cancelled waiter removes itself from its queue and wakes the others, so an
// abandoned request can never hold a place in the ladder.
func (g *Gate) Acquire(ctx context.Context, class Class) (func(), error) {
	if !class.Valid() {
		class = ClassInteractive
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	// The stuck-hold timer is created once, not once per wake. It used to be
	// created inside the loop with a deferred Stop, so every spurious wake left
	// another timer and another deferred call alive until Acquire returned — a
	// leak that only tests could see, and precisely the kind of thing that makes
	// a long soak run report the wrong numbers.
	var stuck <-chan time.Time
	if traceHolders() {
		timer := time.NewTimer(StuckWait)
		defer timer.Stop()
		stuck = timer.C
	}
	g.mu.Lock()
	g.nextID++
	id := g.nextID
	g.waitQueue[class] = append(g.waitQueue[class], waiter{id: id, since: start})
	if !g.active {
		g.grantID = g.pickWinnerLocked()
		g.signalLocked()
	}
	queued := false
	for {
		if err := ctx.Err(); err != nil {
			g.removeWaiterLocked(class, id)
			if g.grantID == id {
				g.grantID = g.pickWinnerLocked()
			}
			g.signalLocked()
			g.mu.Unlock()
			g.cancelled.Add(1)
			return nil, err
		}
		if !g.active && g.grantID == id {
			g.removeWaiterLocked(class, id)
			g.grantID = 0
			g.active = true
			if traceHolders() {
				g.holder = callerStack()
			}
			held := time.Now()
			g.mu.Unlock()
			g.acquired.Add(1)
			if queued {
				g.waited.Add(1)
				g.waitNanos.Add(uint64(time.Since(start)))
			}
			var once sync.Once
			return func() {
				once.Do(func() {
					elapsed := uint64(time.Since(held))
					g.heldNanos.Add(elapsed)
					for {
						current := g.maxHeld.Load()
						if elapsed <= current || g.maxHeld.CompareAndSwap(current, elapsed) {
							break
						}
					}
					g.mu.Lock()
					g.active = false
					g.grantID = g.pickWinnerLocked()
					g.holder = ""
					g.signalLocked()
					g.mu.Unlock()
				})
			}, nil
		}
		queued = true
		notify := g.notify
		g.mu.Unlock()
		// A hold that outlasts StuckWait is a bug, not load: something is holding
		// the writer across work that does not belong inside a transaction, or has
		// returned without committing or rolling back. Say so with the holder's own
		// stack rather than letting the server look merely slow.
		select {
		case <-stuck:
			g.mu.Lock()
			holder := g.holder
			g.mu.Unlock()
			panic(fmt.Sprintf("dbwork: the write gate has been held for more than %s while a %s writer waits.\nHolder:\n%s", StuckWait, class, holder))
		case <-ctx.Done():
			g.mu.Lock()
			g.removeWaiterLocked(class, id)
			if g.grantID == id {
				g.grantID = g.pickWinnerLocked()
			}
			g.signalLocked()
			g.mu.Unlock()
			g.cancelled.Add(1)
			return nil, ctx.Err()
		case <-notify:
			g.mu.Lock()
		}
	}
}

// signalLocked wakes every waiter so each can re-evaluate its own position. A
// broadcast is correct rather than wasteful here: the queues are short and only
// one waiter can win, so a targeted handoff would only move the priority
// decision into the releasing goroutine, which does not know the ladder state
// any better than the waiters do.
func (g *Gate) signalLocked() {
	close(g.notify)
	g.notify = make(chan struct{})
}

func (g *Gate) removeWaiterLocked(class Class, id uint64) {
	queue := g.waitQueue[class]
	for index, candidate := range queue {
		if candidate.id == id {
			if age := uint64(time.Since(candidate.since)); age > g.maxQueueAge[class] {
				g.maxQueueAge[class] = age
			}
			g.waitQueue[class] = append(queue[:index:index], queue[index+1:]...)
			if len(g.waitQueue[class]) == 0 {
				delete(g.waitQueue, class)
			}
			return
		}
	}
}

func promotedPriority(class Class, since, now time.Time) int {
	priority := class.Priority()
	if class.Background() {
		priority -= int(now.Sub(since) / GateAgePromotionInterval)
		if priority < ClassInteractive.Priority() {
			priority = ClassInteractive.Priority()
		}
	}
	return priority
}

// pickWinnerLocked freezes one decision at one instant. If each waiter compares
// priorities separately, an age-promotion boundary can make A see B ahead and
// B see A ahead, leaving a free gate with nobody able to acquire it.
func (g *Gate) pickWinnerLocked() uint64 {
	now := time.Now()
	selected := waiter{}
	best := int(ClassMaintenance) + 1
	for class, queue := range g.waitQueue {
		if len(queue) == 0 {
			continue
		}
		priority := promotedPriority(class, queue[0].since, now)
		if selected.id == 0 || priority < best || priority == best && queue[0].since.Before(selected.since) {
			selected, best = queue[0], priority
		}
	}
	return selected.id
}

// QueueAges reports, per class, the longest a waiter of that class has ever
// queued and how long the oldest one currently waiting has been there. The
// second is the live signal; the first is the one that survives the moment.
func (g *Gate) QueueAges() (peak map[string]uint64, current map[string]uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	peak, current = map[string]uint64{}, map[string]uint64{}
	for class, age := range g.maxQueueAge {
		if age > 0 {
			peak[class.String()] = age / uint64(time.Millisecond)
		}
	}
	now := time.Now()
	for class, queue := range g.waitQueue {
		oldest := time.Duration(0)
		for _, w := range queue {
			if age := now.Sub(w.since); age > oldest {
				oldest = age
			}
		}
		if oldest > 0 {
			current[class.String()] = uint64(oldest / time.Millisecond)
		}
	}
	return peak, current
}

// ActiveOrWaiting reports whether any writer holds or wants the slot. It is the
// "is anyone writing?" signal exclusive maintenance consults before taking the
// database for itself.
func (g *Gate) ActiveOrWaiting() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active {
		return true
	}
	for _, queue := range g.waitQueue {
		if len(queue) > 0 {
			return true
		}
	}
	return false
}

// Waiting returns the number of queued waiters per class, for diagnostics.
func (g *Gate) Waiting() map[string]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[string]int{}
	for class, queue := range g.waitQueue {
		if len(queue) > 0 {
			out[class.String()] = len(queue)
		}
	}
	return out
}

// GateStats is the observable state of the write gate.
type GateStats struct {
	Active         bool           `json:"active"`
	Acquired       uint64         `json:"acquired"`
	Queued         uint64         `json:"queued"`
	Cancelled      uint64         `json:"cancelled"`
	QueueWaitMilli uint64         `json:"queueWaitMillis"`
	HeldMilli      uint64         `json:"heldMillis"`
	MaxHeldMilli   uint64         `json:"maxHeldMillis"`
	Waiting        map[string]int `json:"waiting"`
	// MaxQueueAgeMilli is the longest wait ever seen per class, and
	// QueueAgeMilli the age of the oldest waiter in each class right now. A
	// ladder without aging can starve its bottom rung; these are how that would
	// be noticed rather than guessed at.
	MaxQueueAgeMilli map[string]uint64 `json:"maxQueueAgeMillis"`
	QueueAgeMilli    map[string]uint64 `json:"queueAgeMillis"`
}

// Stats snapshots the gate counters.
func (g *Gate) Stats() GateStats {
	g.mu.Lock()
	active := g.active
	g.mu.Unlock()
	peak, current := g.QueueAges()
	return GateStats{
		Active:           active,
		Acquired:         g.acquired.Load(),
		Queued:           g.waited.Load(),
		Cancelled:        g.cancelled.Load(),
		QueueWaitMilli:   g.waitNanos.Load() / uint64(time.Millisecond),
		HeldMilli:        g.heldNanos.Load() / uint64(time.Millisecond),
		MaxHeldMilli:     g.maxHeld.Load() / uint64(time.Millisecond),
		Waiting:          g.Waiting(),
		MaxQueueAgeMilli: peak,
		QueueAgeMilli:    current,
	}
}

// ResetPeak forgets the longest hold seen so far. A measurement window that
// begins after a fixture has been built must not inherit that fixture's own
// transactions: a load test reports the longest hold *during the run*, and a
// peak carried over from setup is a number about nothing.
func (g *Gate) ResetPeak() {
	g.maxHeld.Store(0)
	g.mu.Lock()
	clear(g.maxQueueAge)
	g.mu.Unlock()
}

// traceHolders reports whether the gate records who holds it. Capturing a stack
// on every acquisition would be a measurable cost on the write path, so it is
// enabled only where the answer is worth it: tests.
func traceHolders() bool { return strict() }

func callerStack() string {
	buffer := make([]byte, 4096)
	return string(buffer[:runtime.Stack(buffer, false)])
}
