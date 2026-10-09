package dbwork

import (
	"context"
	"strings"
	"sync"
	"time"
)

// ReaderLifetimes measures ownership, not SQLite's private read marks. BEGIN
// may precede the first read, and a scalar query may never read the WAL. These
// are conservative lifetimes: rows remain counted through native finalization,
// and snapshots through Commit/Rollback. No SQL, bindings or identity is kept.
type ReaderLifetimeStats struct {
	Snapshots           int    `json:"snapshots"`
	ImplicitRows        int    `json:"implicitRows"`
	OldestSnapshotMs    int64  `json:"oldestSnapshotMs"`
	OldestImplicitMs    int64  `json:"oldestImplicitRowsMs"`
	MaxSnapshotMs       int64  `json:"maxSnapshotMs"`
	MaxImplicitMs       int64  `json:"maxImplicitRowsMs"`
	OldestSnapshotClass string `json:"oldestSnapshotClass,omitempty"`
	OldestImplicitClass string `json:"oldestImplicitRowsClass,omitempty"`
	AdmissionWaiting    int    `json:"admissionWaiting"`
	AdmissionPaused     bool   `json:"admissionPaused"`
	AdmissionWaits      uint64 `json:"admissionWaits"`
	AdmissionWaitMs     int64  `json:"admissionWaitMs"`
}

type readerScope struct {
	mu                       sync.Mutex
	key                      string
	connections              int // guarded by readerScopes.mu
	active                   map[*readerLease]struct{}
	resume                   chan struct{}
	drained                  chan struct{}
	waiting                  int
	waits                    uint64
	waitNanos                int64
	maxSnapshot, maxImplicit int64
	nextPressure             time.Time
	pressureFailures         int
}

type readerLease struct {
	scope    *readerScope
	start    time.Time
	class    Class
	snapshot bool
	once     sync.Once
}

var readerScopes = struct {
	sync.Mutex
	byName map[string]*readerScope
}{byName: make(map[string]*readerScope)}

// Pools with different connection-local policies still share one WAL. The
// filename part of the DSN joins them; it is never exported in diagnostics.
func acquireReaderScope(name string) *readerScope {
	key, _, _ := strings.Cut(name, "?")
	readerScopes.Lock()
	defer readerScopes.Unlock()
	s := readerScopes.byName[key]
	if s == nil {
		s = &readerScope{key: key, active: make(map[*readerLease]struct{})}
		readerScopes.byName[key] = s
	}
	s.connections++
	return s
}

func releaseReaderScope(s *readerScope) {
	if s == nil {
		return
	}
	readerScopes.Lock()
	defer readerScopes.Unlock()
	s.connections--
	if s.connections == 0 {
		delete(readerScopes.byName, s.key)
	}
}

type checkpointReaderBypassKey struct{}

func (c *observedConn) admitReader(ctx context.Context, snapshot bool) (*readerLease, error) {
	if c.readers == nil || (!snapshot && c.inTx) || ctx.Value(changeKey{}) != nil || ctx.Value(checkpointReaderBypassKey{}) != nil {
		return nil, nil
	}
	return c.readers.admit(ctx, snapshot)
}

func (s *readerScope) admit(ctx context.Context, snapshot bool) (*readerLease, error) {
	s.mu.Lock()
	var waited time.Time
	class := ClassFrom(ctx, ClassInteractive)
	// Fresh authority reads share the security class's priority. Keep tracking
	// their lifetime, but let them pass: a security flood may defer the reset;
	// it must not wait behind maintenance. Other foreground classes still wait.
	for s.resume != nil && class != ClassSecurityFence {
		if waited.IsZero() {
			waited = time.Now()
			s.waits++
			s.waiting++
		}
		resume := s.resume
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.waiting--
			s.waitNanos += int64(time.Since(waited))
			s.mu.Unlock()
			return nil, ctx.Err()
		case <-resume:
		}
		s.mu.Lock()
	}
	if !waited.IsZero() {
		s.waiting--
		s.waitNanos += int64(time.Since(waited))
	}
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	lease := &readerLease{scope: s, start: time.Now(), class: class, snapshot: snapshot}
	s.active[lease] = struct{}{}
	s.mu.Unlock()
	return lease, nil
}

func (r *readerLease) close() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		s := r.scope
		s.mu.Lock()
		delete(s.active, r)
		elapsed := int64(time.Since(r.start))
		if r.snapshot {
			s.maxSnapshot = max(s.maxSnapshot, elapsed)
		} else {
			s.maxImplicit = max(s.maxImplicit, elapsed)
		}
		if len(s.active) == 0 && s.drained != nil {
			close(s.drained)
			s.drained = nil
		}
		s.mu.Unlock()
	})
}

func (c *observedConn) finishReadTransaction() {
	c.readerTx.close()
	c.readerTx = nil
}

// ReaderLifetimes includes implicit rows from direct database/sql calls as well
// as every explicit transaction not declared a gated writer. The live map grows
// only with active readers, and is removed when the last connection closes.
func ReaderLifetimes() ReaderLifetimeStats {
	readerScopes.Lock()
	defer readerScopes.Unlock()
	out := ReaderLifetimeStats{}
	for _, s := range readerScopes.byName {
		mergeReaderStats(&out, s.stats())
	}
	return out
}

func (s *readerScope) stats() ReaderLifetimeStats {
	if s == nil {
		return ReaderLifetimeStats{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ReaderLifetimeStats{AdmissionPaused: s.resume != nil, AdmissionWaiting: s.waiting, AdmissionWaits: s.waits, AdmissionWaitMs: s.waitNanos / int64(time.Millisecond), MaxSnapshotMs: s.maxSnapshot / int64(time.Millisecond), MaxImplicitMs: s.maxImplicit / int64(time.Millisecond)}
	now := time.Now()
	for lease := range s.active {
		age := now.Sub(lease.start).Milliseconds()
		if lease.snapshot {
			out.Snapshots++
			if out.OldestSnapshotClass == "" || age > out.OldestSnapshotMs {
				out.OldestSnapshotMs, out.OldestSnapshotClass = age, lease.class.String()
			}
		} else {
			out.ImplicitRows++
			if out.OldestImplicitClass == "" || age > out.OldestImplicitMs {
				out.OldestImplicitMs, out.OldestImplicitClass = age, lease.class.String()
			}
		}
	}
	out.MaxSnapshotMs = max(out.MaxSnapshotMs, out.OldestSnapshotMs)
	out.MaxImplicitMs = max(out.MaxImplicitMs, out.OldestImplicitMs)
	return out
}

func mergeReaderStats(out *ReaderLifetimeStats, next ReaderLifetimeStats) {
	out.Snapshots += next.Snapshots
	out.ImplicitRows += next.ImplicitRows
	if out.OldestSnapshotClass == "" || next.OldestSnapshotMs > out.OldestSnapshotMs {
		out.OldestSnapshotMs, out.OldestSnapshotClass = next.OldestSnapshotMs, next.OldestSnapshotClass
	}
	if out.OldestImplicitClass == "" || next.OldestImplicitMs > out.OldestImplicitMs {
		out.OldestImplicitMs, out.OldestImplicitClass = next.OldestImplicitMs, next.OldestImplicitClass
	}
	out.MaxSnapshotMs = max(out.MaxSnapshotMs, next.MaxSnapshotMs)
	out.MaxImplicitMs = max(out.MaxImplicitMs, next.MaxImplicitMs)
	out.AdmissionWaiting += next.AdmissionWaiting
	out.AdmissionPaused = out.AdmissionPaused || next.AdmissionPaused
	out.AdmissionWaits += next.AdmissionWaits
	out.AdmissionWaitMs += next.AdmissionWaitMs
}
