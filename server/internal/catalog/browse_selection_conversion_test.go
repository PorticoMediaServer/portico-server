package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/readprep"
)

// This factory exercises the real background count computation independently of
// the public admission switch, which stays off until the full cohort is checked.
func prepareLocalConversion(t *testing.T, s *Service, v Viewer, q *BrowseNode) (BrowseSelectionPreparing, string) {
	t.Helper()
	var pending BrowseSelectionPreparing
	var revision string
	err := dbwork.WithReadSnapshot(context.Background(), s.db, func(ctx context.Context) error {
		read := s.WithContext(ctx)
		page, err := read.BrowseEntities(v, BrowseRequest{Library: "a", Pivot: "movies", Query: q, Limit: 1})
		if err != nil {
			return err
		}
		revision = page.PageInfo.Revision
		basis, err := read.browseSelectionWitness("a", v.Profile, v.EffectiveRestrictions())
		if err != nil {
			return err
		}
		state, err := read.NewBrowseSelectionBatch(BrowseRequest{}, v, *page.Bound, basis)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(q)
		if err != nil {
			return err
		}
		spec, err := json.Marshal(selectionPreparationSpec{Library: "a", Pivot: "movies", Revision: revision, Query: raw, Sorts: page.Applied.Sort, Viewer: selectionPreparationViewer(v, "a")})
		if err != nil {
			return err
		}
		comp := &selectionPreparationComputation{service: s, state: state, check: func(*Service) error { return nil }}
		key := readprep.Key{Namespace: browseSelectionPreparationDomain, Basis: basis, Canonical: string(spec)}
		h, err := s.ReadPreparations().Submit(ctx, "conversion-test", key, readprep.Plan{ColdSearch: true, InitialBytes: comp.AccountedBytes() + int64(unsafe.Sizeof(comp)), New: func(context.Context, *readprep.Budget) (readprep.Computation, error) { return comp, nil }})
		if err != nil {
			return err
		}
		pending = selectionPreparationPending(string(h))
		pending.basis = basis
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pending, revision
}

func TestPreparedLocalConversionUsesCompleteIDsAndLiveBasis(t *testing.T) {
	_, s, _ := browseFixture(t)
	store, err := readprep.New(readprep.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s.UseReadPreparations(store)
	v := Viewer{Profile: "p", Fence: "browse-route", Libraries: []string{"a"}}
	q := &BrowseNode{Field: "search", Operator: "contains", Value: "a"}
	pending, revision := prepareLocalConversion(t, s, v, q)
	access := SelectionPreparationAccess{Caller: "conversion-test", Check: func(*Service) error { return nil }}
	ready := awaitSelectionPreparation(t, s, v, pending.QueryID, access, HomeActionAuthority{})
	defer ready.Release()
	if ready.Ready.Total != 1 {
		t.Fatalf("actual prefix result %+v", ready.Ready)
	}
	original := *ready.Ready
	// Another route may carry a different private fence and a wider authorized
	// library list. This does not change the held single-library content basis.
	metadataViewer := v
	metadataViewer.Fence = "metadata-route"
	metadataViewer.Libraries = []string{"tv", "a"}
	err = dbwork.WithReadSnapshot(context.Background(), s.db, func(ctx context.Context) error {
		read := s.WithContext(ctx)
		converted, err := read.ResolvePreparedBrowseSelection(metadataViewer, original.Selector.Query, revision)
		if err != nil {
			return err
		}
		defer converted.Release()
		if len(converted.IDs) != 1 {
			t.Fatal(converted.IDs)
		}
		request := BrowseRequest{Library: "a", Pivot: "movies", Query: q, Sort: original.Selector.Query.Sort, SelectionBasis: pending.basis, Limit: 200, Range: &BrowseRange{Start: 0, Revision: revision}}
		page, err := read.BrowseEntities(v, request)
		if err != nil {
			return err
		}
		defer page.Release()
		if len(page.Entries) != 1 || page.PageInfo.Total != 1 || page.PageInfo.HasMore || page.PageInfo.TotalIsLowerBound || page.PageInfo.Revision != revision {
			t.Fatalf("incomplete explicit conversion %+v", page.PageInfo)
		}
		changed := original.Selector.Query
		changed.Filter = json.RawMessage(`{"field":"search","operator":"contains","value":"b"}`)
		if bad, e := read.ResolvePreparedBrowseSelection(metadataViewer, changed, revision); e == nil {
			bad.Release()
			t.Fatal("changed query borrowed the count proof")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, *ready.Ready) {
		t.Fatal("consumer mutated shared descriptor")
	}
	if _, err = dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassInteractive, `UPDATE catalog_browse_predicate_epoch SET revision=revision+1 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	err = dbwork.WithReadSnapshot(context.Background(), s.db, func(ctx context.Context) error {
		r, e := s.WithContext(ctx).ResolvePreparedBrowseSelection(v, original.Selector.Query, revision)
		r.Release()
		return e
	})
	if !errors.Is(err, ErrStaleContinuation) {
		t.Fatalf("late drift %v", err)
	}
}
