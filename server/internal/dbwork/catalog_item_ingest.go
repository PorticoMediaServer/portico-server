package dbwork

import (
	"database/sql"
	"sort"
	"sync"
	"time"
)

// One committed ingest turn buys one item-operation admission while work is
// owed. Durable intent/custody stays in the existing tables; these booleans
// only alternate their scheduler opportunities, never authorize a mutation.
type catalogItemIngestKey struct {
	db      *sql.DB
	library string
}
type catalogItemIngestTurn struct {
	due, credit   bool
	began, last   time.Time
	lastMS, maxMS int64
}

var catalogItemIngestTurns = struct {
	sync.Mutex
	entries map[catalogItemIngestKey]*catalogItemIngestTurn
}{entries: map[catalogItemIngestKey]*catalogItemIngestTurn{}}

type CatalogItemIngestWait struct {
	Library    string `json:"library"`
	Deferred   bool   `json:"deferred"`
	LastMillis int64  `json:"lastMillis"`
	MaxMillis  int64  `json:"maxMillis"`
}

func itemIngestKey(tx *sql.Tx, library string) (catalogItemIngestKey, bool) {
	value, ok := liveWrites.Load(tx)
	if !ok {
		return catalogItemIngestKey{}, false
	}
	w := value.(*Write)
	return catalogItemIngestKey{w.database, library}, w.database != nil && library != ""
}

// CatalogItemIngestDeferred records only after the rerun mark commits. A
// rolled-back scan must not create a scheduler debt or an observed wait.
func CatalogItemIngestDeferred(tx *sql.Tx, library string) error {
	key, ok := itemIngestKey(tx, library)
	if !ok {
		return ErrTransactionContext
	}
	return AfterCommit(tx, func() {
		now := time.Now()
		catalogItemIngestTurns.Lock()
		defer catalogItemIngestTurns.Unlock()
		e := catalogItemIngestTurns.entries[key]
		if e == nil {
			e = &catalogItemIngestTurn{}
			catalogItemIngestTurns.entries[key] = e
		}
		if !e.due {
			e.began = now
		}
		e.due = true
		e.last = now
	})
}

// CatalogItemAdmission allows existing admitted descendants independently of
// this function. The caller invokes it only for a NEW root, with an indexed
// durable-intent probe. One successful committed admission consumes the credit.
func CatalogItemAdmission(tx *sql.Tx, library string, pending bool) (bool, error) {
	key, ok := itemIngestKey(tx, library)
	if !ok {
		return false, ErrTransactionContext
	}
	catalogItemIngestTurns.Lock()
	e := catalogItemIngestTurns.entries[key]
	if e == nil && pending {
		e = &catalogItemIngestTurn{due: true, began: time.Now(), last: time.Now()}
		catalogItemIngestTurns.entries[key] = e
	}
	allowed := e == nil || (!(pending || e.due) || e.credit)
	catalogItemIngestTurns.Unlock()
	if !allowed {
		return false, nil
	}
	return true, AfterCommit(tx, func() {
		catalogItemIngestTurns.Lock()
		defer catalogItemIngestTurns.Unlock()
		if e := catalogItemIngestTurns.entries[key]; e != nil {
			e.credit = false
			e.last = time.Now()
		}
	})
}

// CatalogItemIngestProgress is called only by an actual inventory/analysis
// publication turn, not by a status read, scan admission or failed observation.
func CatalogItemIngestProgress(tx *sql.Tx, library string) error {
	key, ok := itemIngestKey(tx, library)
	if !ok {
		return ErrTransactionContext
	}
	return AfterCommit(tx, func() {
		now := time.Now()
		catalogItemIngestTurns.Lock()
		defer catalogItemIngestTurns.Unlock()
		e := catalogItemIngestTurns.entries[key]
		if e == nil {
			return
		}
		if e.due {
			e.lastMS = max(int64(0), now.Sub(e.began).Milliseconds())
			e.maxMS = max(e.maxMS, e.lastMS)
		}
		e.due = false
		e.credit = true
		e.last = now
		// Retain at most256 completed diagnostic rows; active due entries are
		// never evicted. Losing an old credit asks for another real ingest turn.
		if len(catalogItemIngestTurns.entries) > 256 {
			var oldest catalogItemIngestKey
			var at time.Time
			for k, v := range catalogItemIngestTurns.entries {
				if !v.due && (at.IsZero() || v.last.Before(at)) {
					oldest, at = k, v.last
				}
			}
			if !at.IsZero() {
				delete(catalogItemIngestTurns.entries, oldest)
			}
		}
	})
}

func CatalogItemIngestWaits() []CatalogItemIngestWait {
	catalogItemIngestTurns.Lock()
	defer catalogItemIngestTurns.Unlock()
	out := make([]CatalogItemIngestWait, 0, len(catalogItemIngestTurns.entries))
	for key, e := range catalogItemIngestTurns.entries {
		out = append(out, CatalogItemIngestWait{key.library, e.due, e.lastMS, e.maxMS})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Library < out[j].Library })
	return out
}
