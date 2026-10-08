// Package compactcatalog is the catalogue: its write API (write.go) and the
// derived data kept from its facts (search, browse, availability, counts).
//
// Facts are written synchronously by the write API in the writer's own
// transaction. Derived data is queued on catalog_dirty(domain, entity_id,
// revision) by that API and by small triggers on the fact tables, and one
// supervised Worker drains it in bounded batches. A derivation can always be
// rebuilt from facts: raising its Version rebuilds it in the background after
// startup, and readers of that domain answer ErrBuilding meanwhile.
package compactcatalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/worker"
)

// Derived domains. Numbers are stored (catalog_dirty, catalog_derivations) and
// permanent: never reuse one.
const (
	DomainSearch       = 17 // catalog_search_documents and the title FTS
	DomainBrowseRows   = 18 // catalog_browse_rows, buckets, counted rows, Home buckets
	DomainBrowseEdges  = 19 // catalog_browse_memberships
	DomainAvailability = 20 // catalog_item_availability, catalog_asset_links.available
	DomainRelated      = 21 // catalog_related_facets
	DomainItemMetrics  = 25 // per-item duration/rating fanned out to containers
	DomainAssetMetrics = 26 // an asset's change fanned out to its items
	DomainCategories   = 27 // movie genre/decade/rating categories
	// DomainInheritedAttributes republishes an item's browse attributes that it
	// inherits from its show, album or book (registered by package metadata).
	DomainInheritedAttributes = 31
)

// BatchLimit is the Worker's units per transaction.
const BatchLimit = 64

// ErrBuilding is what a reader of a derived domain answers while it rebuilds.
var ErrBuilding = errors.New("catalogue is being prepared")

// Key is one queued unit: an entity (or asset, as the domain defines) and the
// revision it was queued at.
type Key struct {
	ID       int64
	Revision int64
}

// Derivation is how one domain's derived data is kept. Drain processes a
// bounded batch inside the Worker's transaction and returns the ids it
// finished; an unfinished id keeps its queue row (a domain with long fanouts
// keeps its own cursor, restarting it when the id's revision changes).
// Backfill enumerates the domain's ids after a cursor for a rebuild.
type Derivation struct {
	Version  int
	Drain    func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error)
	Backfill func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error)
}

var registry = struct {
	sync.RWMutex
	domains map[int]Derivation
}{domains: map[int]Derivation{}}

// Register installs a domain's derivation; called from init.
func Register(domain int, d Derivation) {
	if domain < 1 || domain > 99 || d.Version < 1 || d.Drain == nil || d.Backfill == nil {
		panic(fmt.Sprintf("invalid catalogue derivation %d", domain))
	}
	registry.Lock()
	defer registry.Unlock()
	registry.domains[domain] = d
}

func derivation(domain int) (Derivation, bool) {
	registry.RLock()
	defer registry.RUnlock()
	d, ok := registry.domains[domain]
	return d, ok
}

func registeredDomains() []int {
	registry.RLock()
	defer registry.RUnlock()
	out := make([]int, 0, len(registry.domains))
	for d := range registry.domains {
		out = append(out, d)
	}
	sort.Ints(out)
	return out
}

// TouchTx queues ids of a domain in the caller's transaction, bumping the
// revision of an id already queued so a drain in progress runs it again.
func TouchTx(ctx context.Context, tx *sql.Tx, domain int, ids ...int64) error {
	switch {
	case len(ids) == 0:
		return nil
	case len(ids) == 1:
		_, err := tx.ExecContext(ctx, touchOne, domain, ids[0])
		return err
	}
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		if _, err := tx.ExecContext(ctx, touchMany, domain, mustJSON(ids[start:end])); err != nil {
			return err
		}
	}
	return nil
}

const (
	touchOne  = `INSERT INTO catalog_dirty(domain,entity_id,revision) VALUES(?,?,1) ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1`
	touchMany = `INSERT INTO catalog_dirty(domain,entity_id,revision) SELECT ?,value,1 FROM json_each(?) WHERE 1 ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1`
)

// CheckReadiness answers ErrBuilding while one of the derived domains a reader
// needs is rebuilding. Facts are always current (they are written
// synchronously); only derived data can be behind. One indexed probe.
func CheckReadiness(ctx context.Context, q ReadQuery, domains ...int) error {
	if len(domains) == 0 {
		return nil
	}
	var building bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_derivations WHERE rebuilding=1 AND domain IN(SELECT value FROM json_each(?)))`, mustJSON(domains)).Scan(&building); err != nil {
		return err
	}
	if building {
		return ErrBuilding
	}
	return nil
}

// ReadQuery is a transaction or handle a reader passes in.
type ReadQuery interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Worker drains catalog_dirty and runs the derived step workers.
type Worker struct {
	db *sql.DB
	mu sync.Mutex
	// isolate names domains whose last batch failed: their batches are one id
	// until one succeeds, so a bad id can't hold a whole batch back. poison is
	// the ids that then still failed, one by one; they are skipped until
	// nothing else is queued (a set: a second bad id must not stall the rest).
	isolate map[int]bool
	poison  map[int]map[int64]bool
	// backfillTurn alternates queued ids and rebuild enumeration, so edits
	// aren't queued behind a rebuild.
	backfillTurn map[int]bool
	stepTurn     int
	ready        bool
}

func NewWorker(db *sql.DB) *Worker {
	return &Worker{db: db, isolate: map[int]bool{}, poison: map[int]map[int64]bool{}, backfillTurn: map[int]bool{}}
}

// Run is supervised by the server. It is change-driven: every catalogue write
// reaches catalog_dirty or a job table, and that commit wakes it.
func (w *Worker) Run(ctx context.Context) {
	signal := worker.NewSignal()
	unregister := dbwork.WakeOnTables(signal, "catalog_*", "compact_*")
	defer unregister()
	worker.Run(ctx, "catalogue derived data", signal, func(ctx context.Context) time.Duration {
		if !dbwork.Yield(ctx) {
			return 0
		}
		n, err := w.Step(ctx, BatchLimit)
		if ctx.Err() != nil {
			return 0
		}
		if err != nil {
			log.Printf("Catalogue derived data will retry: %v", err)
			return 5 * time.Second
		}
		if n > 0 {
			return time.Millisecond
		}
		return 0
	})
}

// prepare records every registered domain and starts a rebuild of any whose
// code version is newer than the one its data was built with. Rows only; the
// rebuild itself runs in later Steps.
func (w *Worker) prepare(ctx context.Context) error {
	w.mu.Lock()
	ready := w.ready
	w.mu.Unlock()
	if ready {
		return nil
	}
	err := dbwork.WithWriteTx(ctx, w.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		for _, domain := range registeredDomains() {
			d, _ := derivation(domain)
			if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_derivations(domain,version) VALUES(?,?) ON CONFLICT(domain) DO UPDATE SET version=excluded.version,rebuilding=1,rebuild_cursor=0 WHERE excluded.version>catalog_derivations.version`, domain, d.Version); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		w.mu.Lock()
		w.ready = true
		w.mu.Unlock()
	}
	return err
}

// Rebuild starts rebuilding one domain from facts (tests and diagnostics).
func (w *Worker) Rebuild(ctx context.Context, domain int) error {
	return dbwork.WithWriteTx(ctx, w.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE catalog_derivations SET rebuilding=1,rebuild_cursor=0 WHERE domain=?`, domain)
		return err
	})
}

// Step commits at most about limit units of derived work: one domain's queued
// ids (or rebuild enumeration), else the step workers round-robin.
func (w *Worker) Step(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 500 {
		return 0, fmt.Errorf("catalogue batch must contain 1..500 units")
	}
	if err := w.prepare(ctx); err != nil {
		return 0, err
	}
	n := 0
	domain, failed := -1, int64(0)
	step := -1
	err := dbwork.WithWriteTx(ctx, w.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		n, domain, failed, step = 0, -1, 0, -1
		var rebuilding int
		err := tx.QueryRowContext(ctx, `SELECT domain,rebuilding FROM catalog_derivations st WHERE rebuilding=1
 OR EXISTS(SELECT 1 FROM catalog_dirty d WHERE d.domain=st.domain) ORDER BY served_ms,domain LIMIT 1`).Scan(&domain, &rebuilding)
		if errors.Is(err, sql.ErrNoRows) {
			domain = -1
			var s int
			n, s, err = w.runSteps(ctx, tx, limit)
			step = s
			return err
		}
		if err != nil {
			return err
		}
		n, failed, err = w.drainDomain(ctx, tx, domain, rebuilding, limit)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE catalog_derivations SET served_ms=? WHERE domain=?`, time.Now().UnixMilli(), domain)
		return err
	})
	if err != nil {
		w.setAside(ctx, domain, failed, step, err)
		return 0, err
	}
	if domain >= 0 {
		w.mu.Lock()
		delete(w.isolate, domain)
		delete(w.poison[domain], failed)
		w.mu.Unlock()
	}
	return n, nil
}

// Drain runs the worker to completion: every queued id and every pending step.
// It is for fixtures, tools and tests that build a catalogue and then read it;
// the server runs the worker in the background instead.
func Drain(ctx context.Context, db *sql.DB) error {
	w := NewWorker(db)
	for {
		n, err := w.Step(ctx, 500)
		if err != nil || n == 0 {
			return err
		}
	}
}

func (w *Worker) drainDomain(ctx context.Context, tx *sql.Tx, domain, rebuilding, limit int) (int, int64, error) {
	d, ok := derivation(domain)
	if !ok {
		// A domain queued by an older build: nothing derives it any more.
		_, err := tx.ExecContext(ctx, `DELETE FROM catalog_dirty WHERE domain=?`, domain)
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE catalog_derivations SET rebuilding=0 WHERE domain=?`, domain)
		}
		return 1, 0, err
	}
	w.mu.Lock()
	poisoned := len(w.poison[domain]) > 0
	skip := make([]int64, 0, len(w.poison[domain]))
	for id := range w.poison[domain] {
		skip = append(skip, id)
	}
	batch := limit
	if w.isolate[domain] {
		batch = 1
	}
	queueTurn := rebuilding == 0 || !w.backfillTurn[domain]
	if rebuilding == 1 {
		w.backfillTurn[domain] = !w.backfillTurn[domain]
	}
	w.mu.Unlock()
	var keys []Key
	var err error
	if queueTurn {
		if keys, err = readKeys(tx.QueryContext(ctx, `SELECT entity_id,revision FROM catalog_dirty WHERE domain=? AND entity_id NOT IN(SELECT value FROM json_each(?)) ORDER BY entity_id LIMIT ?`, domain, mustJSON(skip), batch)); err != nil {
			return 0, 0, err
		}
		if len(keys) == 0 && poisoned {
			if keys, err = readKeys(tx.QueryContext(ctx, `SELECT entity_id,revision FROM catalog_dirty WHERE domain=? ORDER BY entity_id LIMIT ?`, domain, batch)); err != nil {
				return 0, 0, err
			}
		}
	}
	if len(keys) == 0 && rebuilding == 1 {
		var after int64
		if err = tx.QueryRowContext(ctx, `SELECT rebuild_cursor FROM catalog_derivations WHERE domain=?`, domain).Scan(&after); err != nil {
			return 0, 0, err
		}
		ids, err := d.Backfill(ctx, tx, after, batch)
		if err != nil {
			return 0, 0, err
		}
		if len(ids) == 0 {
			_, err = tx.ExecContext(ctx, `UPDATE catalog_derivations SET rebuilding=0,rebuild_cursor=0 WHERE domain=?`, domain)
			return 1, 0, err
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		if _, err = tx.ExecContext(ctx, `UPDATE catalog_derivations SET rebuild_cursor=? WHERE domain=?`, ids[len(ids)-1], domain); err != nil {
			return 0, 0, err
		}
		// Enumerated ids go through the queue, so an unfinished one keeps its
		// continuation and a failing one can be isolated like any other.
		if err = TouchTx(ctx, tx, domain, ids...); err != nil {
			return 0, 0, err
		}
		if keys, err = readKeys(tx.QueryContext(ctx, `SELECT entity_id,revision FROM catalog_dirty WHERE domain=? AND entity_id IN(SELECT value FROM json_each(?)) ORDER BY entity_id`, domain, mustJSON(ids))); err != nil {
			return 0, 0, err
		}
	}
	if len(keys) == 0 {
		return 0, 0, nil
	}
	var single int64
	if len(keys) == 1 {
		single = keys[0].ID
	}
	done, err := d.Drain(ctx, tx, keys, limit)
	if err != nil {
		return 0, single, fmt.Errorf("catalogue domain %d: %w", domain, err)
	}
	// Acknowledge exactly the revision this batch saw; an id touched again in
	// the meantime stays queued.
	seen := make(map[int64]int64, len(keys))
	for _, k := range keys {
		seen[k.ID] = k.Revision
	}
	pairs := make([][2]int64, 0, len(done))
	for _, id := range done {
		if revision, ok := seen[id]; ok {
			pairs = append(pairs, [2]int64{id, revision})
		}
	}
	if len(pairs) > 0 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_dirty WHERE domain=? AND (entity_id,revision) IN(SELECT json_extract(value,'$[0]'),json_extract(value,'$[1]') FROM json_each(?))`, domain, mustJSON(pairs)); err != nil {
			return 0, single, err
		}
	}
	return len(keys), single, nil
}

// steps are the derived workers that keep their own job tables (fed by
// triggers on the derived tables above them). They run round-robin when no
// domain has queued work.
var steps = [...]func(context.Context, *sql.Tx, int) (int, error){StepCollectionVisibleCounts, StepCategoryVisibility, StepVisibilitySortBuckets, StepPersonalSortBuckets, StepBookContextEvidence, StepBookGroupVisibleCounts, StepContentEligible, StepRecProfiles}

func (w *Worker) runSteps(ctx context.Context, tx *sql.Tx, limit int) (int, int, error) {
	w.mu.Lock()
	kind := w.stepTurn % len(steps)
	w.mu.Unlock()
	total, idle := 0, 0
	for total < limit && idle < len(steps) {
		n, err := steps[kind](ctx, tx, limit-total)
		if err != nil {
			return 0, kind, err
		}
		if n > 0 {
			total += n
			continue
		}
		idle++
		kind = (kind + 1) % len(steps)
	}
	w.mu.Lock()
	w.stepTurn = (kind + 1) % len(steps)
	w.mu.Unlock()
	return total, -1, nil
}

// setAside keeps one failing unit from stalling the Worker: the transaction
// rolled back, so the work is retried, but a failing domain moves behind the
// others and retries one id at a time; once isolated, the one id that still
// fails is skipped until the rest of the queue drains. A failing step worker
// yields its turn.
func (w *Worker) setAside(ctx context.Context, domain int, id int64, step int, cause error) {
	if domain < 0 {
		if step >= 0 {
			log.Printf("Catalogue derived worker %d failed and yields its turn: %v", step, cause)
			w.mu.Lock()
			w.stepTurn = (step + 1) % len(steps)
			w.mu.Unlock()
		}
		return
	}
	w.mu.Lock()
	single := id != 0 && (w.isolate[domain] || w.poison[domain][id])
	if single {
		if w.poison[domain] == nil {
			w.poison[domain] = map[int64]bool{}
		}
		w.poison[domain][id] = true
	}
	w.isolate[domain] = true
	w.mu.Unlock()
	if single {
		log.Printf("Catalogue id %d in domain %d failed; it stays queued and the rest continues: %v", id, domain, cause)
	} else {
		log.Printf("Catalogue domain %d failed and retries one id at a time: %v", domain, cause)
	}
	err := dbwork.WithWriteTx(ctx, w.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE catalog_derivations SET served_ms=? WHERE domain=?`, time.Now().UnixMilli(), domain)
		return err
	})
	if err != nil && ctx.Err() == nil {
		log.Printf("Catalogue could not set the failed work aside: %v", err)
	}
}

func readKeys(rows *sql.Rows, err error) ([]Key, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []Key
	for rows.Next() {
		var k Key
		if err = rows.Scan(&k.ID, &k.Revision); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func keyIDs(keys []Key) []int64 {
	ids := make([]int64, len(keys))
	for i, k := range keys {
		ids[i] = k.ID
	}
	return ids
}

// scanIDs reads one column of integer ids.
func scanIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}
