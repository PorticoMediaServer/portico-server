package metadata

import (
	"context"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/readprep"
)

// stepWalk advances the existing exact predicate walker, then reads one matched
// entity in a separately fenced short snapshot. The fixed match window is not
// a query membership cache, and is cleared as each entity is reduced.
func (w *bulkPreviewComputation) stepWalk(ctx context.Context, budget *readprep.Budget) error {
	r := w.state
	if r.exhausted {
		return r.step(dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia))
	}
	if w.matchNext < w.matchCount {
		id := w.matches[w.matchNext]
		var page bulkPreviewPage
		err := dbwork.WithReadSnapshot(dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia), r.s.db, func(inner context.Context) error {
			if _, _, err := r.read(inner, true); err != nil {
				return err
			}
			var err error
			page, err = r.s.bulkPreviewPageTx(inner, dbwork.Snapshot(inner), catalog.BulkMetadataSelection{SQL: "SELECT ? AS id", Args: []any{id}, Revision: r.revision})
			if err != nil {
				return err
			}
			if page.preview.Total != 1 {
				return catalog.ErrStaleContinuation
			}
			_, _, err = r.read(inner, true)
			return err
		})
		if err != nil {
			return err
		}
		if dbwork.AuthorityGeneration() != r.authority {
			return catalog.ErrStaleContinuation
		}
		if err = r.addPage(ctx, page); err != nil {
			return err
		}
		w.matches[w.matchNext] = 0
		w.matchNext++
		return nil
	}
	if w.walk.Complete() {
		r.exhausted = true
		return r.step(dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia))
	}
	// The previous state remains live while the shared walker clones it. The
	// native scalar evaluator reserves its complete peak before touching text.
	clone := w.walk.Bytes()
	w.state.transient = clone + catalog.SelectionBatchMaximumBytes + 65536
	if err := budget.Resize(w.AccountedBytes()); err != nil {
		return err
	}
	witness := func(read *catalog.Service) (string, error) {
		_, _, err := r.read(read.Context(), true)
		if err != nil {
			return "", err
		}
		// Source already checked the complete held basis, authority and metadata
		// revisions. The walk additionally compares its original selection proof.
		return w.walk.Basis(), nil
	}
	next, work, err := w.walk.Step(ctx, witness, func(n int64) error {
		w.state.transient = clone + max(int64(catalog.SelectionBatchMaximumBytes), n) + 65536
		return budget.Resize(w.AccountedBytes())
	})
	if err != nil {
		return err
	}
	w.walk, w.matches, w.matchCount, w.matchNext = next, work.Matches, work.MatchCount, 0
	return nil
}
