package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"

	"portico.local/server/internal/compactcatalog"
)

// BulkMetadataTarget names the displayed entity and its real editor namespace.
// ContentKind prevents two different playable kinds from masquerading as one item schema.
type BulkMetadataTarget struct {
	Kind        string `json:"kind"`
	ContentKind string `json:"contentKind"`
	ID          string `json:"id"`
	LibraryID   string `json:"libraryId"`
}
type BulkMetadataPreviewRequest struct {
	Targets  []BulkMetadataTarget `json:"targets,omitempty"`
	Selector *JobSelector         `json:"selector,omitempty"`
	Expected *JobExpected         `json:"expected,omitempty"`
}

// BulkMetadataSelection is transaction-local SQL, never a client-provided fragment.
// Query selections name displayed entities, not their playable descendants.
type BulkMetadataSelection struct {
	SQL      string
	Args     []any
	Targets  []BulkMetadataTarget
	Revision string
}

const BulkMetadataPreviewTargetLimit = 2048

// ValidateSelection checks only request shape, before a service or database is
// required. Authority and live target/revision checks remain in the transaction.
func (in BulkMetadataPreviewRequest) ValidateSelection() error {
	if len(in.Targets) > 0 {
		if in.Selector != nil || in.Expected != nil || len(in.Targets) > BulkMetadataPreviewTargetLimit {
			return jobInvalid("preview needs bounded targets or one query selector")
		}
		seen := map[string]bool{}
		for _, target := range in.Targets {
			kind, err := compactcatalog.ParseKind(target.ContentKind)
			if err != nil || compactcatalog.OwnerNamespace(kind) != target.Kind || target.ID == "" || len(target.ID) > 256 || target.LibraryID == "" || len(target.LibraryID) > 256 || seen[target.ID] {
				return jobInvalid("invalid metadata target")
			}
			seen[target.ID] = true
		}
		return nil
	}
	if in.Selector == nil || in.Selector.Query == nil || in.Selector.Items != nil || in.Selector.Container != nil || in.Expected == nil || in.Expected.CatalogRevision == "" {
		return jobInvalid("preview query requires its exact revision")
	}
	return nil
}

func (s *Service) BulkMetadataSelectionTx(ctx context.Context, tx *sql.Tx, v Viewer, in BulkMetadataPreviewRequest) (BulkMetadataSelection, error) {
	clone := *s.WithContext(ctx)
	clone.readTx = tx
	s = &clone
	out := BulkMetadataSelection{Targets: in.Targets}
	if err := in.ValidateSelection(); err != nil {
		return out, err
	}
	if len(in.Targets) > 0 {
		raw, err := json.Marshal(in.Targets)
		if err != nil {
			return out, err
		}
		visible, args := v.entityVisibilitySQL("i.id", "l.library_id")
		// Playable entities can be visible even before a browse row is published.
		leaf, leafArgs := v.itemVisibilitySQL("i.id")
		out.SQL = `SELECT i.id FROM json_each(?) target JOIN catalog_entities i ON i.public_id=pid_blob(json_extract(target.value,'$.id')) JOIN catalog_libraries l ON l.id=i.library_id JOIN catalog_kinds k ON k.id=i.kind WHERE i.retired=0 AND l.retired=0 AND l.library_id=json_extract(target.value,'$.libraryId') AND k.name=json_extract(target.value,'$.contentKind') AND ((k.playable=1 AND ` + leaf + `) OR (k.playable=0 AND ` + visible + `))` + recordingsClause("i.id", v.EffectiveRestrictions())
		out.Args = append([]any{string(raw)}, leafArgs...)
		out.Args = append(out.Args, args...)
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM (`+out.SQL+`)`, out.Args...).Scan(&count); err != nil {
			return out, err
		}
		if count != len(in.Targets) {
			return out, sql.ErrNoRows
		}
		return out, nil
	}
	q := in.Selector.Query
	if err := s.checkBrowseSelectionBasis(v, q.LibraryID, q.SelectionBasis); err != nil {
		return out, err
	}
	if !v.AllowsLibrary(q.LibraryID) {
		return out, sql.ErrNoRows
	}
	lib, err := s.library(q.LibraryID)
	if err != nil {
		return out, err
	}
	pivot, ok := pivotForKind(lib.Kind, q.Pivot)
	if !ok || !pivot.Browsable {
		return out, ErrBrowsePivot
	}
	if err = s.browseReady(q.LibraryID); err != nil {
		return out, err
	}
	sorts, err := resolveBrowseSorts(pivot, q.Sort)
	if err != nil {
		return out, err
	}
	var node *BrowseNode
	if len(q.Filter) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(q.Filter))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&node); err != nil {
			return out, jobInvalid("invalid preview filter")
		}
	}
	compiler := &browseCompiler{now: s.recommendationNow(s.browseInstant), profile: v.Profile, restrictions: v.EffectiveRestrictions(), count: s.countRows, ids: s.entityIDs, itemPivot: playablePivot(pivot)}
	for _, name := range entityKindMembers(pivot) {
		if kind, e := compactcatalog.ParseKind(name); e == nil {
			compiler.kinds = append(compiler.kinds, int(kind))
		}
	}
	readingEligible, err := s.bindReadingBrowse(q.LibraryID, compiler)
	if err != nil {
		return out, err
	}
	filter, err := compiler.compile(node)
	if err != nil {
		return out, err
	}
	where, args := browseScopeSQL(q.LibraryID, pivot)
	where += " AND " + readingEligible
	visible, bound := v.entityVisibilitySQL("e.entity_id", "l.library_id")
	out.SQL = `SELECT e.entity_id AS id FROM catalog_browse_rows e JOIN catalog_libraries l ON l.id=e.library_id WHERE ` + where + ` AND ` + filter + ` AND ` + visible
	out.Args = append(args, compiler.args...)
	out.Args = append(out.Args, bound...)
	revision, err := s.browseRevision(BrowseRequest{Library: q.LibraryID, Profile: v.Profile, ViewerFence: v.Fence, Restrictions: v.EffectiveRestrictions(), Query: node}, pivot, sorts)
	if err != nil {
		return out, err
	}
	if revision != in.Expected.CatalogRevision {
		return out, ErrStaleContinuation
	}
	out.Revision = revision
	return out, nil
}
