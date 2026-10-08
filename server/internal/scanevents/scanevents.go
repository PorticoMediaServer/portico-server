package scanevents

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"sort"
	"strconv"
	"time"

	"portico.local/server/internal/apievents"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/worker"
)

const progressInterval = 10 * time.Second
const debounceInterval = time.Second

type entry struct {
	found    int64
	sentAt   time.Time
	revision int64
}

type Publisher struct {
	db      *sql.DB
	status  func(ctx context.Context, library string) (catalog.InventoryStatus, error)
	now     func() time.Time
	tracked map[string]entry
}

func New(db *sql.DB, status func(ctx context.Context, library string) (catalog.InventoryStatus, error), now func() time.Time) *Publisher {
	if now == nil {
		now = time.Now
	}
	return &Publisher{db: db, status: status, now: now, tracked: map[string]entry{}}
}

func Run(ctx context.Context, db *sql.DB, status func(ctx context.Context, library string) (catalog.InventoryStatus, error)) {
	New(db, status, nil).Run(ctx)
}

func (p *Publisher) Run(ctx context.Context) {
	// Only a write that names a scan table wakes the publisher: an idle server
	// (no scan) runs no timer and no query, whatever else it commits. While a
	// scan is tracked, a bounded poll at the progress interval backs the wakes
	// up, so progress and the end are never missed.
	signal := worker.NewSignal()
	unregister := dbwork.WakeOnTableWrites(signal, "inventory_source_active", "inventory_runs")
	defer unregister()
	var lastTick time.Time
	nextDue, _ := p.tick(ctx, p.now())
	lastTick = p.now()
	for {
		if nextDue == nil && len(p.tracked) > 0 {
			poll := p.now().Add(progressInterval)
			nextDue = &poll
		}
		var deadlineCtx context.Context
		var cancel context.CancelFunc
		if nextDue != nil {
			deadlineCtx, cancel = context.WithDeadline(ctx, *nextDue)
		} else {
			deadlineCtx, cancel = context.WithCancel(ctx)
		}
		signaled := signal.Wait(deadlineCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if !signaled {
			if remaining := time.Until(lastTick.Add(debounceInterval)); remaining > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(remaining):
				}
			}
			due, _ := p.tick(ctx, p.now())
			lastTick = p.now()
			nextDue = due
			continue
		}
		if remaining := time.Until(lastTick.Add(debounceInterval)); remaining > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(remaining):
			}
		}
		if ctx.Err() != nil {
			return
		}
		due, _ := p.tick(ctx, p.now())
		lastTick = p.now()
		nextDue = due
	}
}

type pendingEvent struct {
	library  string
	state    string
	found    int64
	revision string
	revNum   int64
}

// tick evaluates every active or tracked library once and, when events are
// due, writes them in a single short transaction. Tracked state changes only
// after the commit succeeds; on a write error nothing is updated and the next
// tick retries.
func (p *Publisher) tick(ctx context.Context, now time.Time) (*time.Time, error) {
	active, err := p.activeLibraries(ctx)
	if err != nil {
		return nil, err
	}
	union := map[string]bool{}
	for _, l := range active {
		union[l] = true
	}
	for l := range p.tracked {
		union[l] = true
	}
	libraries := make([]string, 0, len(union))
	for l := range union {
		libraries = append(libraries, l)
	}
	sort.Strings(libraries)
	var events []pendingEvent
	var earliestDue *time.Time
	apply := map[string]*entry{}
	for _, library := range libraries {
		st, err := p.status(ctx, library)
		scanning := false
		var found int64
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				if prev, ok := p.tracked[library]; ok {
					found = prev.found
				}
				scanning = false
			} else {
				continue
			}
		} else {
			for _, s := range st.Sources {
				if s.Status == "queued" || s.Status == "running" {
					scanning = true
				}
				found += s.Discovered
			}
		}
		prev, tracked := p.tracked[library]
		switch {
		case scanning && !tracked:
			rev := now.UnixMilli()
			if rev < 1 {
				rev = 1
			}
			events = append(events, pendingEvent{library: library, state: "started", found: found, revision: strconv.FormatInt(rev, 10), revNum: rev})
			next := entry{found: found, sentAt: now, revision: rev}
			apply[library] = &next
		case scanning && tracked && found != prev.found && !now.Before(prev.sentAt.Add(progressInterval)):
			base := prev.revision + 1
			if now.UnixMilli() > base {
				base = now.UnixMilli()
			}
			events = append(events, pendingEvent{library: library, state: "progress", found: found, revision: strconv.FormatInt(base, 10), revNum: base})
			next := entry{found: found, sentAt: now, revision: base}
			apply[library] = &next
		case scanning && tracked:
			if found != prev.found {
				due := prev.sentAt.Add(progressInterval)
				if earliestDue == nil || due.Before(*earliestDue) {
					dueCopy := due
					earliestDue = &dueCopy
				}
			}
		case !scanning && tracked:
			base := prev.revision + 1
			if now.UnixMilli() > base {
				base = now.UnixMilli()
			}
			events = append(events, pendingEvent{library: library, state: "finished", found: found, revision: strconv.FormatInt(base, 10)})
			apply[library] = nil
		}
	}
	if len(events) == 0 {
		return earliestDue, nil
	}
	if err := p.writeEvents(ctx, events); err != nil {
		log.Printf("scan events write failed: %v", err)
		return earliestDue, err
	}
	for _, e := range events {
		if next, ok := apply[e.library]; ok {
			if next == nil {
				delete(p.tracked, e.library)
			} else {
				p.tracked[e.library] = *next
			}
		}
	}
	// Another library's throttled progress may still be due: keep its timer.
	return earliestDue, nil
}

func (p *Publisher) activeLibraries(ctx context.Context) ([]string, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT DISTINCT s.library_id FROM inventory_source_active a JOIN library_sources s ON s.id=a.source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (p *Publisher) writeEvents(ctx context.Context, events []pendingEvent) error {
	return dbwork.WithWriteTx(ctx, p.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		for _, e := range events {
			if err := apievents.Append(tx, apievents.LibraryAudience(e.library), "library.scan.updated", "library", e.library, e.revision, map[string]any{"state": e.state, "found": e.found}); err != nil {
				return err
			}
		}
		return nil
	})
}
