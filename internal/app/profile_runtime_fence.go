package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

var errProfileErasureInProgress = errors.New("profile erasure is in progress")
var errProfileErasureDrainTimeout = errors.New("profile erasure could not safely drain active work")

const profileErasureDrainTimeout = 2 * time.Second

// profileRuntimeFence turns permanent profile erasure into a runtime authority
// boundary. Authenticated requests hold cancellable leases; erasure closes
// admission, cancels existing leases, waits for a bounded acknowledgement, and
// only then removes durable state. The map is deliberately process-local and
// releases the raw profile identifier after completion; the database remains
// the restart-safe authority and retains only a deidentified receipt digest.
type profileRuntimeFence struct {
	mu             sync.Mutex
	nextID         uint64
	entries        map[string]*profileRuntimeFenceEntry
	accountEntries map[string]*profileRuntimeFenceEntry
}

type profileRuntimeFenceEntry struct {
	deleting   bool
	generation uint64
	leases     map[uint64]context.CancelFunc
	changed    chan struct{}
	done       chan struct{}
}

type profileErasureFenceHandle struct {
	fence      *profileRuntimeFence
	key        string
	generation uint64
	owner      bool
	account    bool
	done       <-chan struct{}
}

func profileRuntimeFenceKey(accountID, profileID string) string {
	return strings.TrimSpace(accountID) + "\x00" + strings.TrimSpace(profileID)
}

func (f *profileRuntimeFence) acquire(parent context.Context, accountID, profileID string) (context.Context, func(), error) {
	if parent == nil {
		parent = context.Background()
	}
	key := profileRuntimeFenceKey(accountID, profileID)
	if key == "\x00" {
		return parent, func() {}, nil
	}
	f.mu.Lock()
	if f.entries == nil {
		f.entries = map[string]*profileRuntimeFenceEntry{}
	}
	if f.accountEntries == nil {
		f.accountEntries = map[string]*profileRuntimeFenceEntry{}
	}
	accountKey := strings.TrimSpace(accountID)
	accountEntry := f.accountEntries[accountKey]
	if accountEntry != nil && accountEntry.deleting {
		f.mu.Unlock()
		return parent, func() {}, errProfileErasureInProgress
	}
	entry := f.entries[key]
	if entry == nil {
		entry = &profileRuntimeFenceEntry{leases: map[uint64]context.CancelFunc{}, changed: make(chan struct{})}
		f.entries[key] = entry
	}
	if entry.deleting {
		f.mu.Unlock()
		return parent, func() {}, errProfileErasureInProgress
	}
	f.nextID++
	leaseID := f.nextID
	ctx, cancel := context.WithCancel(parent)
	entry.leases[leaseID] = cancel
	if accountEntry == nil {
		accountEntry = &profileRuntimeFenceEntry{leases: map[uint64]context.CancelFunc{}, changed: make(chan struct{})}
		f.accountEntries[accountKey] = accountEntry
	}
	accountEntry.leases[leaseID] = cancel
	f.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel()
			f.mu.Lock()
			if current := f.entries[key]; current == entry {
				delete(current.leases, leaseID)
				close(current.changed)
				current.changed = make(chan struct{})
				if !current.deleting && len(current.leases) == 0 {
					delete(f.entries, key)
				}
			}
			if current := f.accountEntries[accountKey]; current == accountEntry {
				delete(current.leases, leaseID)
				close(current.changed)
				current.changed = make(chan struct{})
				if !current.deleting && len(current.leases) == 0 {
					delete(f.accountEntries, accountKey)
				}
			}
			f.mu.Unlock()
		})
	}
	return ctx, release, nil
}

func (f *profileRuntimeFence) beginErasure(accountID, profileID string) profileErasureFenceHandle {
	key := profileRuntimeFenceKey(accountID, profileID)
	f.mu.Lock()
	if f.entries == nil {
		f.entries = map[string]*profileRuntimeFenceEntry{}
	}
	if f.accountEntries == nil {
		f.accountEntries = map[string]*profileRuntimeFenceEntry{}
	}
	if accountEntry := f.accountEntries[strings.TrimSpace(accountID)]; accountEntry != nil && accountEntry.deleting {
		handle := profileErasureFenceHandle{fence: f, key: strings.TrimSpace(accountID), generation: accountEntry.generation, account: true, done: accountEntry.done}
		f.mu.Unlock()
		return handle
	}
	entry := f.entries[key]
	if entry == nil {
		entry = &profileRuntimeFenceEntry{leases: map[uint64]context.CancelFunc{}, changed: make(chan struct{})}
		f.entries[key] = entry
	}
	if entry.deleting {
		handle := profileErasureFenceHandle{fence: f, key: key, generation: entry.generation, done: entry.done}
		f.mu.Unlock()
		return handle
	}
	entry.deleting = true
	entry.generation++
	entry.done = make(chan struct{})
	cancellations := make([]context.CancelFunc, 0, len(entry.leases))
	for _, cancel := range entry.leases {
		cancellations = append(cancellations, cancel)
	}
	handle := profileErasureFenceHandle{fence: f, key: key, generation: entry.generation, owner: true, done: entry.done}
	f.mu.Unlock()
	for _, cancel := range cancellations {
		cancel()
	}
	return handle
}

func (f *profileRuntimeFence) beginAccountErasure(accountID string) profileErasureFenceHandle {
	key := strings.TrimSpace(accountID)
	f.mu.Lock()
	if f.accountEntries == nil {
		f.accountEntries = map[string]*profileRuntimeFenceEntry{}
	}
	entry := f.accountEntries[key]
	if entry == nil {
		entry = &profileRuntimeFenceEntry{leases: map[uint64]context.CancelFunc{}, changed: make(chan struct{})}
		f.accountEntries[key] = entry
	}
	if entry.deleting {
		handle := profileErasureFenceHandle{fence: f, key: key, generation: entry.generation, account: true, done: entry.done}
		f.mu.Unlock()
		return handle
	}
	entry.deleting = true
	entry.generation++
	entry.done = make(chan struct{})
	cancellations := make([]context.CancelFunc, 0, len(entry.leases))
	for _, cancel := range entry.leases {
		cancellations = append(cancellations, cancel)
	}
	handle := profileErasureFenceHandle{fence: f, key: key, generation: entry.generation, owner: true, account: true, done: entry.done}
	f.mu.Unlock()
	for _, cancel := range cancellations {
		cancel()
	}
	return handle
}

func (h profileErasureFenceHandle) entryLocked() *profileRuntimeFenceEntry {
	if h.account {
		return h.fence.accountEntries[h.key]
	}
	return h.fence.entries[h.key]
}

func (h profileErasureFenceHandle) wait(ctx context.Context, maximum time.Duration) bool {
	if h.fence == nil {
		return true
	}
	if !h.owner {
		select {
		case <-h.done:
			return true
		case <-ctx.Done():
			return false
		case <-time.After(maximum):
			return false
		}
	}
	deadline := time.NewTimer(maximum)
	defer deadline.Stop()
	for {
		h.fence.mu.Lock()
		entry := h.entryLocked()
		if entry == nil || entry.generation != h.generation || len(entry.leases) == 0 {
			h.fence.mu.Unlock()
			return true
		}
		changed := entry.changed
		h.fence.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		}
	}
}

func (h profileErasureFenceHandle) finish() {
	if h.fence == nil || !h.owner {
		return
	}
	h.fence.mu.Lock()
	if entry := h.entryLocked(); entry != nil && entry.generation == h.generation {
		if entry.done != nil {
			close(entry.done)
		}
		if h.account {
			delete(h.fence.accountEntries, h.key)
		} else {
			delete(h.fence.entries, h.key)
		}
	}
	h.fence.mu.Unlock()
}

// beginAccountRuntimeErasureContext closes admission for every profile that
// can currently authenticate as an account. Holding all returned handles until
// the durable account mutation commits prevents an already-authenticated
// request from restoring authority after account deletion or hosted-access
// revocation.
func (s *Server) beginAccountRuntimeErasureContext(ctx context.Context, accountID string) ([]profileErasureFenceHandle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for attempt := 0; attempt < 3; attempt++ {
		handle := s.profileRuntime.beginAccountErasure(accountID)
		if !handle.wait(ctx, profileErasureDrainTimeout) {
			if handle.owner {
				handle.finish()
				return nil, errProfileErasureDrainTimeout
			}
			return nil, errProfileErasureInProgress
		}
		if !handle.owner {
			continue
		}
		return []profileErasureFenceHandle{handle}, nil
	}
	return nil, errors.New("account profile authority changed repeatedly during revocation")
}

func finishProfileErasureFences(handles []profileErasureFenceHandle) {
	for _, handle := range handles {
		handle.finish()
	}
}

func (s *Server) beginAccountRuntimeErasuresContext(ctx context.Context, accountIDs []string) ([]profileErasureFenceHandle, error) {
	unique := map[string]bool{}
	for _, accountID := range accountIDs {
		if accountID = strings.TrimSpace(accountID); accountID != "" {
			unique[accountID] = true
		}
	}
	ordered := make([]string, 0, len(unique))
	for accountID := range unique {
		ordered = append(ordered, accountID)
	}
	sort.Strings(ordered)
	handles := make([]profileErasureFenceHandle, 0, len(ordered))
	for _, accountID := range ordered {
		acquired, err := s.beginAccountRuntimeErasureContext(ctx, accountID)
		if err != nil {
			finishProfileErasureFences(handles)
			return nil, err
		}
		handles = append(handles, acquired...)
	}
	return handles, nil
}
