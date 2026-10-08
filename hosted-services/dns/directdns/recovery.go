package directdns

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Sync drains the authenticated feed before reporting readiness. A full page
// never establishes synchronization, and cancellation bounds a changing feed.
func (f *Feed) Sync(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		more, err := f.once(ctx)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
}

// Rebuild is an explicit offline restore operation. It fetches from zero into a
// separate empty table and replaces the old rows AND cursor only after success.
// The caller must stop the serving process and freeze Hosted address writes.
func (f *Feed) Rebuild(ctx context.Context) error {
	fresh, err := OpenTable("", f.Table.AllowPrivate)
	if err != nil {
		return err
	}
	copyFeed := *f
	copyFeed.Table = fresh
	copyFeed.Readiness = nil
	if err = copyFeed.Sync(ctx); err != nil {
		return err
	}
	f.Table.compactMu.Lock()
	defer f.Table.compactMu.Unlock()
	f.Table.mu.Lock()
	defer f.Table.mu.Unlock()
	state := fresh.current.Load()
	rows := state.rows()
	// Switch snapshot and journal generation together. An interrupted rebuild
	// must never replay a previous generation above the new (lower) cursor.
	oldJournal := f.Table.journal
	if f.Table.path != "" {
		j, e := os.CreateTemp(filepath.Dir(f.Table.path), ".dns-journal-*")
		if e != nil {
			return e
		}
		if e = j.Sync(); e != nil {
			j.Close()
			return e
		}
		if e = j.Close(); e != nil {
			return e
		}
		f.Table.journal = j.Name()
	}
	if err = f.Table.persist(rows, fresh.sequence); err != nil {
		f.Table.journal = oldJournal
		return err
	}
	if oldJournal != "" && oldJournal != f.Table.journal {
		_ = os.Remove(oldJournal)
	}
	f.Table.current.Store(state)
	f.Table.sequence = fresh.sequence
	return nil
}

// Summary identifies a restored dataset without logging addresses or secrets.
func (t *Table) Summary() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	raw, _ := json.Marshal(struct {
		Sequence int64
		Rows     map[string]Addresses
	}{t.sequence, t.current.Load().rows()})
	sum := sha256.Sum256(raw)
	return map[string]any{"sequence": t.sequence, "rows": t.current.Load().count, "digest": hex.EncodeToString(sum[:])}
}

// Readiness belongs to this process lifetime, never the persisted table. DNS
// liveness stays independent so Hosted outages do not remove working authority.
// readinessWindow is how long after a completed sync the node still reports ready. The feed
// polls every 30–60 s, so the window must cover one full interval plus margin for jitter.
const readinessWindow = 90 * time.Second

type Readiness struct {
	mu       sync.Mutex
	lastSync time.Time
	failed   bool
}

func (s *Readiness) Record(more bool, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.failed = true
		return
	}
	if !more {
		s.lastSync = time.Now().UTC()
		s.failed = false
	}
}
func (s *Readiness) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	last, failed := s.lastSync, s.failed
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	ready := !failed && !last.IsZero() && time.Since(last) < readinessWindow
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ready": ready, "lastAuthenticatedSync": last})
}

func (s *Readiness) Metrics(w http.ResponseWriter, r *http.Request, t *Table) {
	s.mu.Lock()
	last, failed := s.lastSync, s.failed
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	lag := time.Since(last).Seconds()
	if last.IsZero() {
		lag = -1
	}
	ok := 0
	if !failed && !last.IsZero() {
		ok = 1
	}
	fmt.Fprintf(w, "portico_dns_rows %d\nportico_dns_applied_sequence %d\nportico_dns_last_sync_timestamp_seconds %d\nportico_dns_sync_age_seconds %g\nportico_dns_sync_success %d\n", t.Len(), t.Sequence(), last.Unix(), lag, ok)
}
