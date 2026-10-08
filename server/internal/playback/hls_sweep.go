package playback

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/diskspace"
)

// The per-second sweep used to hold h.mu across a directory walk and a pooled
// database query, per session. A three-hour title at six-second segments is
// 1,800 files; at two hundred sessions that was 360,000 stat calls and two
// hundred pooled queries under the lock, every second — and h.mu is the same
// lock start, noteServed and produce's cleanup take, which puts it on the path
// of every segment request. If the pool was saturated the lock was held for the
// full pool wait, two hundred times over, and playback latency collapsed
// server-wide for a reason no diagnostic could show: a healthy write gate, a
// busy pool, and a mutex nobody was measuring.
//
// The rules here are the same ones the concurrency architecture states for the
// database, applied to a mutex: take it to read a snapshot, release it, do the
// filesystem and the database work outside, take it again only to act.

// These are variables rather than constants so a test can move them; nothing in
// the server writes to them.
var (
	// hlsGlobalByteBudget bounds every generated session together. Two hundred
	// sessions at even five hundred megabytes would fill a home server's volume —
	// the same volume as server.sqlite and its log — and the per-session cap said
	// nothing about that.
	hlsGlobalByteBudget int64 = 24 << 30
	// hlsFreeSpaceFloor is how much room a producer needs before it may start.
	// Beyond-target behaviour only: a volume with less than this free is already
	// in trouble, and the honest refusal is better than the database discovering
	// it first.
	hlsFreeSpaceFloor int64 = 2 << 30
)

// hlsGlobalByteBudgetForTest and hlsFreeSpaceFloorForTest move the ceilings a
// test needs to cross without writing gigabytes.
func hlsGlobalByteBudgetForTest(v int64) int64 {
	previous := hlsGlobalByteBudget
	hlsGlobalByteBudget = v
	return previous
}

func hlsFreeSpaceFloorForTest(v int64) int64 {
	previous := hlsFreeSpaceFloor
	hlsFreeSpaceFloor = v
	return previous
}

const (
	// hlsUsageInterval is how often a session's directory is measured. The
	// per-session ceiling exists to stop a runaway converter, and a converter
	// cannot run away in less than this.
	hlsUsageInterval = 30 * time.Second
	// hlsSessionByteCeiling is the per-session cap that has always been here.
	hlsSessionByteCeiling = 4 << 30
)

// hlsUsage is one generated session's measured footprint.
type hlsUsage struct {
	bytes    int64
	files    int
	measured time.Time
	// served is when a segment was last handed to a viewer. It is the eviction
	// order: the least recently watched session is the one whose bytes are least
	// likely to be wanted again.
	served time.Time
}

// activeSnapshot copies the live session set, so everything after it runs with
// the lock released.
func (h *HLS) activeSnapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	ids := make([]string, 0, len(h.active))
	seen := map[string]bool{}
	for key := range h.active {
		id := producerSession(key)
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// cancelSession stops one producer. It is idempotent: produce's own cleanup
// already tolerates a session that ended between the snapshot and here.
func (h *HLS) cancelSession(id string) {
	h.mu.Lock()
	cancels := []context.CancelFunc{}
	for key, cancel := range h.active {
		if producerSession(key) == id {
			cancels = append(cancels, cancel)
		}
	}
	h.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}

}

func (h *HLS) cancelAll() {
	h.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(h.active))
	for _, cancel := range h.active {
		cancels = append(cancels, cancel)
	}
	h.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// sessionStates reads the state of every live session in one query instead of
// one per session, with the sweep's own context so a slow pool cannot pin the
// sweep either.
func (h *HLS) sessionStates(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, time.Now().UTC().Format(time.RFC3339))
	query := `SELECT id,state FROM playback_sessions WHERE id IN (` + placeholders(len(ids)) + `) AND expires_at>?`
	rows, err := dbwork.Query(ctx, h.db, query, args...)
	if err != nil {
		// Unreadable is not terminal: a busy pool must never be the reason two
		// hundred viewers' conversions are cancelled.
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var id, state string
		if rows.Scan(&id, &state) != nil {
			return nil
		}
		out[id] = state
	}
	if rows.Err() != nil {
		return nil
	}
	return out
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// sweep is one tick: cancel what should no longer run, and keep the generated
// directory inside its budget. Nothing here holds h.mu across work the mutex
// does not need.
func (h *HLS) sweep(ctx context.Context) {
	ids := h.activeSnapshot()
	if len(ids) > 0 {
		if states := h.sessionStates(ctx, ids); states != nil {
			for _, id := range ids {
				state, live := states[id]
				if !live || state == "stopped" || state == "ended" || state == "failed" {
					h.cancelSession(id)
				}
			}
		}
		h.enforceSessionCeilings(ids)
		// A conversion nobody is fetching from is a processor this server is
		// spending on somebody who is not there. Reclaimed without ending the
		// session: the segments stay, and asking for one restarts the producer.
		h.reclaimAbandoned()
	}
	h.enforceGlobalBudget(ctx)
}

// enforceSessionCeilings measures each live session's directory, amortised, and
// stops a producer that has run away.
func (h *HLS) enforceSessionCeilings(ids []string) {
	now := time.Now()
	for _, id := range ids {
		h.mu.Lock()
		usage := h.usage[id]
		h.mu.Unlock()
		if usage != nil && now.Sub(usage.measured) < hlsUsageInterval {
			continue
		}
		bytes, files, err := measureDirectory(filepath.Join(h.root, id))
		if err != nil && !os.IsNotExist(err) {
			h.cancelSession(id)
			continue
		}
		h.recordUsage(id, bytes, files, now)
		if bytes > hlsSessionByteCeiling || files > audioArtifactFiles {
			h.cancelSession(id)
		}
	}
}

func (h *HLS) recordUsage(id string, bytes int64, files int, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.usage == nil {
		h.usage = map[string]*hlsUsage{}
	}
	entry := h.usage[id]
	if entry == nil {
		entry = &hlsUsage{}
		h.usage[id] = entry
	}
	entry.bytes, entry.files, entry.measured = bytes, files, at
}

// noteSessionServed records that a viewer is watching this session now. It is
// the eviction order and the fence that keeps a live viewer's bytes: a session
// served in the last retention window is never evicted.
func (h *HLS) noteSessionServed(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.noteSessionServedLocked(id)
}

// noteSessionServedLocked is noteSessionServed for a caller already holding the
// mutex.
func (h *HLS) noteSessionServedLocked(id string) {
	if h.usage == nil {
		h.usage = map[string]*hlsUsage{}
	}
	entry := h.usage[id]
	if entry == nil {
		entry = &hlsUsage{}
		h.usage[id] = entry
	}
	entry.served = time.Now()
}

// enforceGlobalBudget keeps <state>/hls inside one ceiling across every session,
// evicting the least recently served completed sessions first.
func (h *HLS) enforceGlobalBudget(ctx context.Context) {
	now := time.Now()
	h.mu.Lock()
	due := now.Sub(h.budgetSwept) >= hlsUsageInterval
	if due {
		h.budgetSwept = now
	}
	h.mu.Unlock()
	if !due {
		return
	}
	entries, err := os.ReadDir(h.root)
	if err != nil {
		return
	}
	present := map[string]bool{}
	type candidate struct {
		id     string
		bytes  int64
		served time.Time
	}
	var total int64
	var candidates []candidate
	for _, entry := range entries {
		// A crash after retirement must not strand evicted cache bytes.
		if strings.HasPrefix(entry.Name(), ".evicted-") {
			if err := os.RemoveAll(filepath.Join(h.root, entry.Name())); err != nil {
				continue
			}
			continue
		}
		if !entry.IsDir() || !generatedDirectoryID.MatchString(entry.Name()) {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		id := entry.Name()
		present[id] = true
		bytes, files, measureErr := measureDirectory(filepath.Join(h.root, id))
		if measureErr != nil {
			continue
		}
		total += bytes
		h.recordUsage(id, bytes, files, now)
		h.mu.Lock()
		live := h.sessionActiveLocked(id)
		served := time.Time{}
		if usage := h.usage[id]; usage != nil {
			served = usage.served
		}
		h.mu.Unlock()
		if live {
			// A producer still running owns its output; cancelling it is the
			// per-session ceiling's job, not the budget's.
			continue
		}
		candidates = append(candidates, candidate{id: id, bytes: bytes, served: served})
	}
	// Sessions whose directories have gone leave nothing behind: this map would
	// otherwise grow with every session the server has ever converted.
	h.mu.Lock()
	for id := range h.usage {
		if !present[id] {
			delete(h.usage, id)
		}
	}
	// The same goes for every other per-session map: cut lists, retention floors
	// and restart budgets all outlived their sessions before this.
	for id := range h.timelines {
		if !present[id] {
			delete(h.timelines, id)
		}
	}
	for id := range h.reclaimed {
		if !present[id] {
			delete(h.reclaimed, id)
		}
	}
	for id := range h.restarts {
		if !present[id] {
			delete(h.restarts, id)
		}
	}
	h.mu.Unlock()
	if total <= hlsGlobalByteBudget {
		return
	}
	// Least recently served first. A session nobody has watched in this process's
	// life sorts to the front, which is exactly right: its bytes are the ones
	// least likely to be wanted again.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].served.Before(candidates[j].served) })
	retention := time.Duration(h.configuration().PlayedRetentionSeconds) * time.Second
	if retention <= 0 {
		retention = time.Duration(DefaultPlayedRetention) * time.Second
	}
	for _, c := range candidates {
		if total <= hlsGlobalByteBudget || ctx.Err() != nil {
			return
		}
		// Never take bytes a viewer may seek back to. Retention is the setting
		// that says how far back that is, and prunePlayedSegments already honours
		// it for the played region of a live session.
		if !c.served.IsZero() && now.Sub(c.served) < retention {
			continue
		}
		// Fence admission against eviction. Retire the directory atomically;
		// recursive deletion needs no HLS lock.
		h.mu.Lock()
		if h.sessionActiveLocked(c.id) || (h.usage[c.id] != nil && !h.usage[c.id].served.IsZero() && now.Sub(h.usage[c.id].served) < retention) {
			h.mu.Unlock()
			continue
		}
		retired := filepath.Join(h.root, ".evicted-"+c.id+"-"+strconv.FormatInt(time.Now().UnixNano(), 10))
		err = os.Rename(filepath.Join(h.root, c.id), retired)
		if err == nil {
			delete(h.usage, c.id)
			delete(h.reclaimed, c.id)
			delete(h.timelines, c.id)
		}
		h.mu.Unlock()
		if err != nil {
			continue
		}
		if err = os.RemoveAll(retired); err == nil {
			total -= c.bytes
		}
	}
}

// measureDirectory sums one session's output. It is one ReadDir and one stat per
// entry, which is why it is amortised rather than run every second.
func measureDirectory(dir string) (int64, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	var total int64
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return total, len(entries), infoErr
		}
		total += info.Size()
	}
	return total, len(entries), nil
}

// roomToProduce reports whether the volume has room for another conversion. A
// volume this full is already in trouble; refusing honestly is better than
// letting the database be the one to discover it.
func (h *HLS) roomToProduce() bool {
	free, err := diskspace.Free(h.root)
	if err != nil {
		// Unmeasurable is not full. A filesystem that cannot answer must not be
		// the reason playback stops.
		return true
	}
	return free >= hlsFreeSpaceFloor
}
