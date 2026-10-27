package playback

import (
	"context"
	"errors"
	"sync"
)

// occurrenceRegistry is the process-side publication fence for media work. An
// intent is acquired before reading durable state; that same intent must be
// checked after the read and before a process or provider is published.
// No database or provider work is performed while mu is held.
var errOccurrenceClaimed = errors.New("playback occurrence is being retired")

type occurrenceRegistry struct {
	mu      sync.Mutex
	entries map[string]*occurrenceEntry
}
type occurrenceEntry struct {
	epoch     uint64
	blocked   bool
	committed bool
	intents   int
	runs      map[*occurrenceRun]struct{}
	claim     *occurrenceClaim
}
type occurrenceIntent struct {
	registry *occurrenceRegistry
	id       string
	entry    *occurrenceEntry
	epoch    uint64
	released bool
}
type occurrenceRun struct {
	generation int
	requestID  string
	itemID     int64
	assetID    string
	cancel     context.CancelFunc
	done       <-chan struct{}
}
type occurrenceClaim struct {
	registry   *occurrenceRegistry
	id         string
	generation int
	entry      *occurrenceEntry
	epoch      uint64
	runs       []*occurrenceRun
	phase      uint8 // 0 held, 1 committed, 2 aborted
	cancelOnce sync.Once
}

func (r *occurrenceRegistry) intent(id string) (*occurrenceIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[string]*occurrenceEntry)
	}
	e := r.entries[id]
	if e == nil {
		e = &occurrenceEntry{runs: make(map[*occurrenceRun]struct{})}
		r.entries[id] = e
	}
	if e.blocked || e.committed {
		return nil, errOccurrenceClaimed
	}
	e.intents++
	return &occurrenceIntent{registry: r, id: id, entry: e, epoch: e.epoch}, nil
}

func (i *occurrenceIntent) currentLocked() bool {
	return !i.released && i.registry.entries[i.id] == i.entry && !i.entry.blocked && !i.entry.committed && i.entry.epoch == i.epoch
}

// install atomically checks the original ticket and records its exact run. The
// caller publishes process state only in publish; it must not perform I/O.
func (i *occurrenceIntent) install(run *occurrenceRun, publish func()) error {
	r := i.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	if !i.currentLocked() {
		return errOccurrenceClaimed
	}
	i.entry.runs[run] = struct{}{}
	publish()
	return nil
}
func (i *occurrenceIntent) current() bool {
	r := i.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	return i.currentLocked()
}
func (i *occurrenceIntent) release() {
	r := i.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	if i.released {
		return
	}
	i.released = true
	i.entry.intents--
	r.pruneLocked(i.id, i.entry)
}
func (r *occurrenceRegistry) finished(id string, e *occurrenceEntry, run *occurrenceRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries[id] != e {
		return
	}
	delete(e.runs, run)
	r.pruneLocked(id, e)
}
func (r *occurrenceRegistry) pruneLocked(id string, e *occurrenceEntry) {
	if r.entries[id] == e && e.intents == 0 && len(e.runs) == 0 && e.claim == nil {
		delete(r.entries, id)
	}
}

// claim fences new publication before the accepting transaction. Repeated
// claims of the same generation share the same once-only phase latch.
func (r *occurrenceRegistry) claim(id string, generation int) (*occurrenceClaim, error) {
	if id == "" || generation < 1 {
		return nil, errOccurrenceClaimed
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[string]*occurrenceEntry)
	}
	e := r.entries[id]
	if e == nil {
		e = &occurrenceEntry{runs: make(map[*occurrenceRun]struct{})}
		r.entries[id] = e
	}
	if e.claim != nil {
		if e.claim.generation == generation {
			return e.claim, nil
		}
		return nil, errOccurrenceClaimed
	}
	if e.blocked || e.committed {
		return nil, errOccurrenceClaimed
	}
	e.blocked = true
	e.epoch++
	c := &occurrenceClaim{registry: r, id: id, generation: generation, entry: e, epoch: e.epoch}
	for run := range e.runs {
		if run.generation == generation {
			c.runs = append(c.runs, run)
		}
	}
	e.claim = c
	return c, nil
}

// abort is valid only before the caller's transaction commits.
func (c *occurrenceClaim) abort() {
	r := c.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.phase != 0 || r.entries[c.id] != c.entry || c.entry.claim != c {
		return
	}
	c.phase = 2
	c.entry.claim = nil
	c.entry.blocked = false
	r.pruneLocked(c.id, c.entry)
}

// commit irreversibly closes the entry and cancels only the captured runs.
// A caller must invoke it only after the durable fence commits. Join is bounded
// by ctx; a timeout does not undo the committed fence.
func (c *occurrenceClaim) commit(ctx context.Context) error {
	r := c.registry
	r.mu.Lock()
	if c.phase == 2 {
		r.mu.Unlock()
		return errOccurrenceClaimed
	}
	if c.phase == 0 {
		if r.entries[c.id] != c.entry || c.entry.claim != c {
			r.mu.Unlock()
			return errOccurrenceClaimed
		}
		c.phase = 1
		c.entry.committed = true
		c.entry.claim = nil
	}
	runs := append([]*occurrenceRun(nil), c.runs...)
	r.mu.Unlock()
	c.cancelOnce.Do(func() {
		for _, run := range runs {
			if run.cancel != nil {
				run.cancel()
			}
		}
	})
	for _, run := range runs {
		if run.done != nil {
			select {
			case <-run.done:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	r.mu.Lock()
	r.pruneLocked(c.id, c.entry)
	r.mu.Unlock()
	return nil
}
