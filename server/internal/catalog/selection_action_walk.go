package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/selectionactions"
)

// SelectionActionWalk retains only the existing incremental predicate reducer.
// A selected identity is emitted in its qualifying snapshot, never recovered
// from a page or from a second independently compiled selection query.
type SelectionActionWalk struct {
	state  *BrowseSelectionBatchState
	viewer Viewer
	query  JobQuery
}

func (w *SelectionActionWalk) Bytes() int64 {
	if w == nil {
		return 0
	}
	return int64(w.state.Bytes() + len(w.query.Filter) + len(w.query.LibraryID) + len(w.query.Pivot) + len(w.viewer.Profile) + 4096)
}

func (s *Service) NewSelectionActionWalk(query JobQuery, expected JobExpected, viewer Viewer) (*SelectionActionWalk, error) {
	if dbwork.Snapshot(s.Context()) == nil {
		return nil, sql.ErrTxDone
	}
	if query.SelectionBasis == "" || !selectionBasisPattern.MatchString(query.SelectionBasis) || expected.CatalogRevision == "" {
		return nil, selectionactions.ErrInput
	}
	if err := s.checkBrowseSelectionBasis(viewer, query.LibraryID, query.SelectionBasis); err != nil {
		return nil, err
	}
	lib, err := s.library(query.LibraryID)
	if err != nil {
		return nil, err
	}
	pivot, ok := pivotForKind(lib.Kind, query.Pivot)
	if !ok || !selectionLeafPivot(pivot) {
		return nil, ErrSelectionBatchUnsupported
	}
	var node *BrowseNode
	if len(query.Filter) > 0 {
		node, err = ParseBrowseQuery(query.Filter, "selector.query.filter")
		if err != nil {
			return nil, err
		}
	}
	sorts, err := resolveBrowseSorts(pivot, query.Sort)
	if err != nil {
		return nil, err
	}
	compiler := &browseCompiler{now: s.recommendationNow(s.browseInstant), profile: viewer.Profile, restrictions: viewer.EffectiveRestrictions()}
	if _, err = compiler.compile(node); err != nil {
		return nil, err
	}
	revision, err := s.browseRevision(BrowseRequest{Library: query.LibraryID, Profile: viewer.Profile, ViewerFence: viewer.Fence, Restrictions: viewer.EffectiveRestrictions(), Query: node}, pivot, sorts)
	if err != nil {
		return nil, err
	}
	if revision != expected.CatalogRevision {
		return nil, ErrStaleContinuation
	}
	bound := BoundBrowseQuery{Library: query.LibraryID, Pivot: query.Pivot, Query: node, Sorts: sorts, Revision: revision, snapshot: dbwork.Snapshot(s.Context())}
	state, err := s.NewBrowseSelectionBatch(BrowseRequest{}, viewer, bound, query.SelectionBasis)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &query); err != nil {
		return nil, err
	}
	return &SelectionActionWalk{state: state, viewer: viewer, query: query}, nil
}

// StepSelectionActionWalk charges every ordinary raw/nested predicate unit and
// stops immediately after one accepted identity. It does not fork predicates.
func (s *Service) StepSelectionActionWalk(ctx context.Context, w *SelectionActionWalk, check func(*Service) error) (*SelectionActionWalk, *selectionactions.Target, bool, error) {
	if w == nil || w.state == nil || check == nil {
		return nil, nil, false, selectionactions.ErrInput
	}
	next := *w
	next.state = w.state.Clone()
	var target *selectionactions.Target
	err := dbwork.WithReadSnapshot(dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia), s.db, func(inner context.Context) error {
		read := s.WithContext(inner)
		if err := check(read); err != nil {
			return err
		}
		if err := read.checkBrowseSelectionBasis(w.viewer, w.query.LibraryID, w.query.SelectionBasis); err != nil {
			return err
		}
		deadline := time.Now().Add(selectionBatchTime)
		bytes := 0
		for units := 0; units < SelectionBatchMaximumRows && !next.state.done && time.Now().Before(deadline); units++ {
			if err := inner.Err(); err != nil {
				return err
			}
			// Stage8 is reached only after the exact existing predicate/restriction
			// evaluator accepted this leaf. Capture before that unit clears candidate.
			id := int64(0)
			if next.state.candidate.stage == 8 {
				id = next.state.candidate.id
			}
			next.state.scalarBytes = 0
			if err := read.selectionBatchUnit(next.state); err != nil {
				return err
			}
			bytes += selectionBatchProbeBytes + next.state.scalarBytes
			if id != 0 {
				value := selectionactions.Target{EntityID: id, LibraryID: w.query.LibraryID}
				var parent, added sql.NullInt64
				var titleBytes int
				err := read.read().QueryRow(`SELECT pid(e.public_id),CASE WHEN length(CAST(e.title AS BLOB))<=? THEN e.title ELSE '' END,k.name,e.parent_id,e.added_ms,length(CAST(e.title AS BLOB)) FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind JOIN catalog_libraries l ON l.id=e.library_id WHERE e.id=? AND l.library_id=? AND e.retired=0`, SelectionBatchMaximumBytes, id, w.query.LibraryID).Scan(&value.ItemID, &value.Title, &value.Kind, &parent, &added, &titleBytes)
				if errors.Is(err, sql.ErrNoRows) {
					return ErrSelectionBatchStale
				}
				if err != nil {
					return err
				}
				var parentValue any
				if parent.Valid {
					parentValue = parent.Int64
				}
				value.Identity = selectionactions.TargetIdentity(value.ItemID, value.LibraryID, value.Kind, parentValue)
				if added.Valid {
					value.Lifetime = strconv.FormatInt(added.Int64, 10)
				}
				// Scalar materialization is bounded by the source/store lease, not an
				// uncharged sample/result cache. A single exceptional title is not truncated.
				if titleBytes > SelectionBatchMaximumBytes || len(value.Title)+len(value.ItemID)+next.state.Bytes() > SelectionBatchMaximumBytes {
					return ErrSelectionBatchCapacity
				}
				target = &value
				break
			}
			if bytes >= SelectionBatchMaximumBytes {
				break
			}
		}
		if err := check(read); err != nil {
			return err
		}
		return read.checkBrowseSelectionBasis(w.viewer, w.query.LibraryID, w.query.SelectionBasis)
	})
	if err != nil {
		return nil, nil, false, err
	}
	return &next, target, next.state.done, nil
}
