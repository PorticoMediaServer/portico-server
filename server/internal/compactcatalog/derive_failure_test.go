package compactcatalog

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// failingDomain is a derived domain registered only for a test: it finishes
// every id but bad, and a rebuild enumerates all.
const failingDomain = 97

func registerFailingDomain(t *testing.T, bad int64, all []int64) map[int64]bool {
	t.Helper()
	finished := map[int64]bool{}
	Register(failingDomain, Derivation{
		Version: 1,
		Drain: func(_ context.Context, _ *sql.Tx, keys []Key, _ int) ([]int64, error) {
			for _, k := range keys {
				if k.ID == bad {
					return nil, errors.New("this id never derives")
				}
			}
			for _, k := range keys {
				finished[k.ID] = true
			}
			return keyIDs(keys), nil
		},
		Backfill: func(_ context.Context, _ *sql.Tx, after int64, limit int) ([]int64, error) {
			var ids []int64
			for _, id := range all {
				if id > after && len(ids) < limit {
					ids = append(ids, id)
				}
			}
			return ids, nil
		},
	})
	t.Cleanup(func() {
		registry.Lock()
		delete(registry.domains, failingDomain)
		registry.Unlock()
	})
	return finished
}

// stepPastFailures runs the worker the way its loop does, a failed Step being
// retried, and returns how many Steps failed.
func stepPastFailures(t *testing.T, w *Worker, steps int) int {
	t.Helper()
	failures := 0
	for i := 0; i < steps; i++ {
		if _, err := w.Step(context.Background(), BatchLimit); err != nil {
			failures++
		}
	}
	return failures
}

// An id whose derivation always fails stays queued and keeps being reported,
// but the step workers still get their turn behind it.
func TestStepWorkersRunBehindAnIDThatNeverDerives(t *testing.T) {
	db := cc1OpenDB(t, "failing-id.sqlite")
	tv := cc1Library(t, db, "tv", "TV", "tv", "/tv")
	show := cc1Entity(t, db, Entity{Library: tv, Kind: Show, Key: "show:show", Title: "Show", Year: 2020}, map[string]any{"local_key": "show"})
	cc1Drain(t, db)
	finished := registerFailingDomain(t, 2, nil)
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
		return TouchTx(ctx, tx, failingDomain, 1, 2, 3)
	})
	w := NewWorker(db)
	if stepPastFailures(t, w, 12) == 0 {
		t.Fatal("the failing id was never reported")
	}
	queueContentEligible(t, db, show)
	failures := stepPastFailures(t, w, 12)
	if n := cc1Scalar(t, db, `SELECT COUNT(*) FROM catalog_content_eligible_jobs`); n != 0 {
		t.Fatalf("a step worker's job waited behind the failing id: %d queued", n)
	}
	if !finished[1] || !finished[3] {
		t.Fatalf("ids beside the failing one were not derived: %v", finished)
	}
	if n := cc1Scalar(t, db, `SELECT COUNT(*) FROM catalog_dirty WHERE domain=? AND entity_id=2`, failingDomain); n != 1 {
		t.Fatalf("the failing id left the queue: %d rows", n)
	}
	if failures == 0 {
		t.Fatal("the failing id stopped being retried and reported once the rest was idle")
	}
}

// A rebuild enumerates past an id that never derives: the domain finishes
// rebuilding with that one id still queued, and the step workers run again.
func TestRebuildFinishesPastAnIDThatNeverDerives(t *testing.T) {
	db := cc1OpenDB(t, "failing-rebuild.sqlite")
	tv := cc1Library(t, db, "tv", "TV", "tv", "/tv")
	show := cc1Entity(t, db, Entity{Library: tv, Kind: Show, Key: "show:show", Title: "Show", Year: 2020}, map[string]any{"local_key": "show"})
	cc1Drain(t, db)
	finished := registerFailingDomain(t, 3, []int64{1, 2, 3, 4, 5})
	w := NewWorker(db)
	if _, err := w.Step(context.Background(), BatchLimit); err != nil {
		t.Fatal(err)
	}
	if err := w.Rebuild(context.Background(), failingDomain); err != nil {
		t.Fatal(err)
	}
	queueContentEligible(t, db, show)
	stepPastFailures(t, w, 40)
	if n := cc1Scalar(t, db, `SELECT rebuilding FROM catalog_derivations WHERE domain=?`, failingDomain); n != 0 {
		t.Fatal("the rebuild never finished behind the failing id")
	}
	for _, id := range []int64{1, 2, 4, 5} {
		if !finished[id] {
			t.Fatalf("id %d was not rebuilt: %v", id, finished)
		}
	}
	if n := cc1Scalar(t, db, `SELECT COUNT(*) FROM catalog_dirty WHERE domain=? AND entity_id=3`, failingDomain); n != 1 {
		t.Fatalf("the failing id is not queued after the rebuild: %d rows", n)
	}
	if n := cc1Scalar(t, db, `SELECT COUNT(*) FROM catalog_content_eligible_jobs`); n != 0 {
		t.Fatalf("a step worker's job waited behind the failing id: %d queued", n)
	}
}
