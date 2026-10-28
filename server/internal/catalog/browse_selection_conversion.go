package catalog

import (
	"context"
	"encoding/json"
	"fmt"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/readprep"
)

// BrowseSelectionConversion owns a delivery lease for a COMPLETE, small result
// retained by the original off-request count build. It never runs a census.
// Hold Release through hydration and response serialization.
type BrowseSelectionConversion struct {
	IDs      []int64
	Revision string
	Basis    string
	lease    readprep.Result
}

func (c BrowseSelectionConversion) Release()                  { c.lease.Release() }
func (c BrowseSelectionConversion) Reserve(bytes int64) error { return c.lease.ResizeDelivery(bytes) }

func (s *Service) ResolvePreparedBrowseSelection(viewer Viewer, query JobQuery, revision string) (out BrowseSelectionConversion, err error) {
	if dbwork.Snapshot(s.Context()) == nil || query.SelectionBasis == "" || revision == "" {
		return out, ErrStaleContinuation
	}
	if err = s.checkBrowseSelectionBasis(viewer, query.LibraryID, query.SelectionBasis); err != nil {
		return out, err
	}
	var node *BrowseNode
	if len(query.Filter) > 0 {
		if err = json.Unmarshal(query.Filter, &node); err != nil {
			return out, err
		}
	}
	if !selectionBatchHasLocalSearch(node) {
		return out, ErrSelectionBatchUnsupported
	}
	library, err := s.library(query.LibraryID)
	if err != nil {
		return out, err
	}
	pivot, ok := pivotForKind(library.Kind, query.Pivot)
	if !ok || !selectionLeafPivot(pivot) {
		return out, ErrSelectionBatchUnsupported
	}
	sorts, err := resolveBrowseSorts(pivot, query.Sort)
	if err != nil {
		return out, err
	}
	request := BrowseRequest{Library: query.LibraryID, Pivot: query.Pivot, Profile: viewer.Profile, ViewerFence: viewer.Fence, Restrictions: viewer.EffectiveRestrictions(), Query: node}
	current, err := s.browseRevision(request, pivot, sorts)
	if err != nil {
		return out, err
	}
	if current != revision {
		return out, ErrStaleContinuation
	}
	raw, err := json.Marshal(node)
	if err != nil {
		return out, err
	}
	spec, err := json.Marshal(selectionPreparationSpec{Library: query.LibraryID, Pivot: query.Pivot, Revision: revision, Query: raw, Sorts: sorts, Viewer: viewer})
	if err != nil {
		return out, err
	}
	store := s.ReadPreparations()
	if store == nil {
		return out, readprep.ErrStale
	}
	key := readprep.Key{Namespace: browseSelectionPreparationDomain, Basis: query.SelectionBasis, Canonical: string(spec)}
	// Opening a private lease on existing exact content does not submit work.
	// It grants no authority: scope, query, current witness and viewer were all
	// checked above in this snapshot, and HTTP checks live authority again.
	caller := readprep.Caller("browse-conversion/" + operationHash(viewer))
	h, err := store.Open(s.Context(), caller, key)
	if err != nil {
		return out, err
	}
	defer func() { _ = store.Cancel(context.WithoutCancel(s.Context()), caller, h) }()
	result, err := store.Poll(s.Context(), caller, h)
	if err != nil {
		return out, err
	}
	keep := false
	defer func() {
		if !keep {
			result.Release()
		}
	}()
	if result.State != readprep.Ready {
		return out, readprep.ErrStale
	}
	if err = result.ResizeDelivery(int64(8*len(result.Payload) + 65536)); err != nil {
		return out, err
	}
	var content selectionPreparationContent
	if err = json.Unmarshal(result.Payload, &content); err != nil {
		return out, err
	}
	total := 0
	for _, counts := range content.Counts {
		if counts[0] < 0 || counts[1] < 0 || counts[1] > counts[0] {
			return out, readprep.ErrInvalid
		}
		total += counts[0]
	}
	if total > BrowseSelectionConversionLimit {
		return out, fmt.Errorf("%w: explicit conversion exceeds %d", ErrSelectionBatchUnsupported, BrowseSelectionConversionLimit)
	}
	if content.Library != query.LibraryID || content.Pivot != query.Pivot || content.Revision != revision || total != len(content.IDs) {
		return out, ErrStaleContinuation
	}
	seen := map[int64]bool{}
	for _, id := range content.IDs {
		if id <= 0 || seen[id] {
			return out, readprep.ErrInvalid
		}
		seen[id] = true
	}
	keep = true
	return BrowseSelectionConversion{IDs: content.IDs, Revision: revision, Basis: query.SelectionBasis, lease: result}, nil
}

func (s *Service) browsePreparedSelection(viewer Viewer, request BrowseRequest) (BrowseResult, error) {
	out := BrowseResult{Entries: []ContentEntry{}, PositionIndex: []BrowsePositionAnchor{}}
	if request.Cursor != "" || request.Seek != nil || request.HomeRowDisplay != "" || request.Range == nil || request.Range.Start != 0 || request.Range.Revision == "" {
		return out, ErrSelectionBatchUnsupported
	}
	filter, err := json.Marshal(request.Query)
	if err != nil {
		return out, err
	}
	converted, err := s.ResolvePreparedBrowseSelection(viewer, JobQuery{LibraryID: request.Library, Pivot: request.Pivot, Filter: filter, Sort: request.Sort, SelectionBasis: request.SelectionBasis}, request.Range.Revision)
	if err != nil {
		return out, err
	}
	keep := false
	defer func() {
		if !keep {
			converted.Release()
		}
	}()
	if request.Limit < len(converted.IDs) {
		return out, ErrSelectionBatchUnsupported
	}
	// Ordinary hydrators remain <=100. The bounded <=200 source and its final
	// ordered SQL never recompile the query, recount matches or build anchors.
	if err = converted.Reserve(8 << 20); err != nil {
		return out, err
	}
	ids, err := json.Marshal(converted.IDs)
	if err != nil {
		return out, err
	}
	library, err := s.library(request.Library)
	if err != nil {
		return out, err
	}
	pivot, _ := pivotForKind(library.Kind, request.Pivot)
	sorts, err := resolveBrowseSorts(pivot, request.Sort)
	if err != nil {
		return out, err
	}
	order, args := browseOrderSQL(viewer.Profile, sorts)
	args = append([]any{string(ids)}, args...)
	rows, err := s.queryBrowseRows(`SELECT `+browseRowColumns+` FROM json_each(?) picked CROSS JOIN catalog_browse_rows e ON e.entity_id=picked.value CROSS JOIN catalog_entities ce ON ce.id=e.entity_id ORDER BY `+order, args...)
	if err != nil {
		return out, err
	}
	if len(rows) != len(converted.IDs) {
		return out, ErrStaleContinuation
	}
	for start := 0; start < len(rows); start += BrowseMaximumLimit {
		entries, e := s.browseProjection(viewer.Profile, rows[start:min(start+BrowseMaximumLimit, len(rows))])
		if e != nil {
			return out, e
		}
		out.Entries = append(out.Entries, entries...)
	}
	if err = s.attachViewerRatings(viewer.Profile, out.Entries); err != nil {
		return out, err
	}
	if err = s.EnrichListeningEntries(viewer, out.Entries); err != nil {
		return out, err
	}
	out.Pivot = request.Pivot
	out.Applied = BrowseApplied{Query: request.Query, Sort: sorts}
	out.PageInfo = BrowsePageInfo{Total: len(rows), Revision: converted.Revision}
	// This is a selection conversion, not a new browse window. Its duration is
	// unused by the explicit action paths and no whole-query duration runs.
	out.selectionRelease = converted.Release
	keep = true
	return out, nil
}
