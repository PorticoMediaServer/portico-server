package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Private Reading terminal cleanup for the library removal's existing phase 5.
//
// `removeLibraryBatchTx` finishes with `removeLibraryRowTx`, which deletes the
// library, its folders and the catalogue handle. Reading keeps private rows
// keyed to the catalogue handle (`reading_path_scope.library_id` and the
// library_id columns of the inputs/work/pending tables) with no foreign key to
// the public rows, so nothing there cascades when the public library goes. The
// Reading dispatcher also only reclaims scopes it can claim: a settled
// (`queued=0`) or parked scope is never selected, and domain28 work can hold a
// runnable one back forever.
//
// This file hangs one bounded terminal drain on the EXISTING phase 5, before
// and after the library row delete. It does not add phase 6; the installed
// `catalog_removal_progress` still checks `phase BETWEEN 0 AND 5`. It adds no
// table, trigger, cap or second deletion framework:
//
//  1. scope stage: ONE private scope at a time through the existing
//     `RetireReadingScopePage` (`library_removal_reading_scope.go`). That
//     helper selects one raw primary-key child per turn and already reclaims
//     activated, settled and parked scopes without dispatch admission. Its
//     compiled plan digest and `internal/phasedpurge` cursor stay the only
//     cursor format;
//  2. residue stage: bounded, library-keyed slices for the rows the scope page
//     deliberately leaves to the library/source removal owner:
//     `reading_path_availability_pending`, `reading_path_dirty_work`,
//     `reading_path_semantic_pending` and `reading_path_inputs`, then the
//     library's source barriers and blocks. A raw window of source ids is read
//     BEFORE any barrier/block filter, and each turn deletes at most ONE raw
//     barrier row or ONE raw block row: a limited parent is never used as the
//     bound of an unbounded child;
//  3. final stage: the unchanged `removeLibraryRowTx` runs in this same writer
//     transaction. Its root `UPDATE` and the folder/handle `DELETE` fire the
//     installed Reading triggers (`reading_path_catalog_libraries_update`,
//     `reading_path_catalog_libraries_delete`,
//     `reading_path_libraries_update`, `reading_path_library_sources_delete`;
//     the handle delete is unconditional), which requeue inputs/work for the
//     already deleted handle. Those final effects are drained here, bounded,
//     and the handle's terminal emptiness is then PROVED in the same
//     transaction: a missing row in `catalog_libraries` is never taken as a
//     reason to report success over private rows that still exist. A remainder
//     is a visible error, never a silent Done.
//
// Durability. The caller stores the only durable state in the existing
// `catalog_removal_progress.object_cursor` column while `phase=5`:
//
//	{"h":<catalogue handle>,"s":"scope"|"residue","q":<scope id>,"c":<phasedpurge envelope>,"r":<source id>}
//
// `c` is exactly the string `RetireReadingScopePage` returned; it is not a
// second cursor type. A stored state with another handle, an unknown stage or
// no scope is validated and restarted from the scope stage: every unit is
// state-driven, so restarting re-proves the current database and cannot skip a
// row. A sub-cursor from an older compiled plan, or one carrying a row key this
// build never persists, is likewise restarted from that scope's beginning
// instead of blocking the removal on a cursor that cannot resume. See
// `readingTerminalLoad` and `readingTerminalScopeStep`.
//
// Costs (source declarations, not a measured proof):
//
//   - one scope turn: the scope page's existing ledger (owner 5 reads, 7
//     effects at most: the unit's raw row plus the caller's single Save row),
//     plus ONE indexed `reading_path_scope_library` pick. The Save row is this
//     turn's progress-row upsert;
//   - one residue turn: at most one bounded delete per turn. Pending and work
//     are limited to `readingTerminalWorkBatch` raw rows through their own
//     library-leading index/PK; semantic/inputs are single primary-key rows;
//     the source window reads at most `readingTerminalSourceWindow` indexed
//     `library_sources` ids, probes each with at most one barrier and one block
//     point statement, and deletes at most one raw barrier or block row. The
//     turn's progress-row upsert is the only other effect;
//   - one final turn: the existing `removeLibraryRowTx` (its own statements are
//     unchanged and not re-claimed here), bounded residue rounds, and five
//     EXISTS probes keyed by the handle.
//
// The shared `internal/phasedpurge` limits (24 effects / 128 reads) are
// untouched. The scope page remains the only user of them; the terminal slices
// are bounded by the constants below and add no unbounded statement.
//
// Not claimed: rows with no library handle (`reading_path_head`, publication
// projections, public/derived rows) continue to follow the catalogue and
// container phases that already own them; `reading_path_slot` is detached only
// by the scope page and only when it names a build of that supplied scope, so
// another library's live slot, projections and `reading_path_clock` are never
// touched here.

const (
	// readingTerminalWorkBatch bounds one pending/work slice. It matches the
	// Reading worker's own raw batch, so a terminal turn never asks for more
	// rows than the existing dispatcher would consume in one step.
	readingTerminalWorkBatch = 128
	// readingTerminalSourceWindow bounds the raw `library_sources` window read
	// before any barrier/block filter. It is deliberately small: sources are
	// owner-created and few, and each source's blocks are drained one row at a
	// time rather than as an unbounded child of a limited parent.
	readingTerminalSourceWindow = 32
	// readingTerminalFinalRounds bounds the post-delete drain loop. The final
	// delete can requeue at most one input row and one work row per library and
	// per folder; any remainder after this many bounded rounds is an error.
	readingTerminalFinalRounds = 1024

	readingTerminalScopeStage   = "scope"
	readingTerminalResidueStage = "residue"
)

var errReadingTerminalResidue = errors.New("library removal left private Reading rows behind")

const (
	// readingTerminalScopePickSQL is the one scope pick per turn. The partial
	// runnable index is deliberately not used: settled and parked scopes must
	// be reclaimed too.
	readingTerminalScopePickSQL  = `SELECT id FROM reading_path_scope INDEXED BY reading_path_scope_library WHERE library_id=?1 ORDER BY id LIMIT 1`
	readingTerminalScopeCountSQL = `SELECT EXISTS(SELECT 1 FROM reading_path_scope INDEXED BY reading_path_scope_library WHERE library_id=?1)`

	// readingTerminalPendingDeleteSQL: an availability delete fires
	// reading_path_pending_delete, which requeues one 'pending' work row. Work
	// is therefore drained after the pending tables in every residue turn.
	readingTerminalPendingDeleteSQL  = `DELETE FROM reading_path_availability_pending WHERE entity_id IN(SELECT entity_id FROM reading_path_availability_pending WHERE library_id=?1 ORDER BY entity_id LIMIT ?2)`
	readingTerminalWorkDeleteSQL     = `DELETE FROM reading_path_dirty_work WHERE (library_id,kind,work_key) IN(SELECT library_id,kind,work_key FROM reading_path_dirty_work WHERE library_id=?1 ORDER BY kind,work_key LIMIT ?2)`
	readingTerminalSemanticDeleteSQL = `DELETE FROM reading_path_semantic_pending WHERE library_id=?1`
	readingTerminalInputsDeleteSQL   = `DELETE FROM reading_path_inputs WHERE library_id=?1`

	// readingTerminalSourceWindowSQL is the raw source window. `id>` keeps the
	// walk forward-only without re-reading the whole library each turn.
	readingTerminalSourceWindowSQL = `SELECT id FROM library_sources WHERE library_id=?1 AND id>?2 ORDER BY id LIMIT ?3`
	// readingTerminalBarrierDeleteSQL is one raw barrier row: its primary key is
	// the source id, so a source window never bounds an unbounded child.
	readingTerminalBarrierDeleteSQL = `DELETE FROM reading_path_source_barrier WHERE source_id=?1`
	// readingTerminalBlockDeleteSQL selects and deletes ONE block row of one
	// source. A parent source LIMIT is never reused as a child bound.
	readingTerminalBlockDeleteSQL = `DELETE FROM reading_path_block WHERE id=(SELECT id FROM reading_path_block WHERE source_id=?1 ORDER BY id LIMIT 1)`
)

// readingTerminalState is the durable phase-5 cursor stored in the existing
// `catalog_removal_progress.object_cursor`. It wraps (does not replace) the
// scope purge's own stored envelope.
type readingTerminalState struct {
	Handle int64  `json:"h"`
	Stage  string `json:"s"`
	Scope  int64  `json:"q,omitempty"`
	Cursor string `json:"c,omitempty"`
	Source string `json:"r,omitempty"`
}

func readingTerminalLoad(raw string, handle int64) readingTerminalState {
	fresh := readingTerminalState{Handle: handle, Stage: readingTerminalScopeStage}
	if raw == "" {
		return fresh
	}
	var st readingTerminalState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return fresh
	}
	if st.Handle != handle || st.Scope < 0 {
		return fresh
	}
	switch st.Stage {
	case readingTerminalScopeStage:
		if st.Scope == 0 {
			st.Cursor = ""
		}
		st.Source = ""
		return st
	case readingTerminalResidueStage:
		st.Scope, st.Cursor = 0, ""
		return st
	default:
		return fresh
	}
}

func readingTerminalEncode(st readingTerminalState) (string, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// removeLibraryReadingTerminalTx advances phase 5 by one bounded turn. It
// returns more=false only after it has run the existing final row deletion and
// proved that no private Reading row still names the handle. The caller saves
// removalProgress when more is true; when more is false the row deletion has
// already removed the progress row.
func removeLibraryReadingTerminalTx(ctx context.Context, tx *sql.Tx, library string, handle int64, progress *removalProgress) (bool, error) {
	st := readingTerminalLoad(progress.objectCursor, handle)
	for {
		switch st.Stage {
		case readingTerminalScopeStage:
			worked, err := readingTerminalScopeStep(ctx, tx, handle, &st)
			if err != nil {
				return false, err
			}
			if worked {
				return true, readingTerminalStore(progress, st)
			}
			st.Stage = readingTerminalResidueStage
		case readingTerminalResidueStage:
			worked, err := readingTerminalResidueStep(ctx, tx, handle, &st)
			if err != nil {
				return false, err
			}
			if worked {
				return true, readingTerminalStore(progress, st)
			}
			if err := readingTerminalFinalTx(ctx, tx, library, handle); err != nil {
				return false, err
			}
			return false, nil
		default:
			st = readingTerminalState{Handle: handle, Stage: readingTerminalScopeStage}
		}
	}
}

func readingTerminalStore(progress *removalProgress, st readingTerminalState) error {
	raw, err := readingTerminalEncode(st)
	if err != nil {
		return err
	}
	progress.objectCursor = raw
	return nil
}

// readingTerminalScopeStep drains one private scope through the existing page.
// It reports worked=true when a page ran or a scope row was proved gone; the
// caller then persists and yields. A page that reports no progress is an error,
// never a silent Done.
func readingTerminalScopeStep(ctx context.Context, tx *sql.Tx, handle int64, st *readingTerminalState) (bool, error) {
	for {
		if st.Scope == 0 {
			var scope int64
			err := tx.QueryRowContext(ctx, readingTerminalScopePickSQL, handle).Scan(&scope)
			if errors.Is(err, sql.ErrNoRows) {
				// No scope row remains. This is not terminal on its own: the
				// residue stage below still has to drain and prove inputs,
				// work, pending, barriers and blocks.
				return false, nil
			}
			if err != nil {
				return false, err
			}
			st.Scope = scope
		}
		scope := st.Scope
		out, err := RetireReadingScopePage(ctx, tx, ReadingScopePurgeRequest{
			ScopeID:       scope,
			LibraryHandle: handle,
			StoredCursor:  st.Cursor,
			Save: func(_ context.Context, _ *sql.Tx, stored string) error {
				st.Cursor = stored
				return nil
			},
		})
		if errors.Is(err, ErrReadingScopePurgePlan) || errors.Is(err, ErrReadingScopePurgeCursor) {
			// The durable sub-cursor was written by a different compiled plan,
			// or carries a row key this build never persists. Both are refused
			// before a unit runs. The unit plan is state-driven, so restarting
			// this scope from its beginning re-proves the rows it has now and
			// cannot skip one. A vanished lifetime is NOT restarted here.
			st.Cursor = ""
			out, err = RetireReadingScopePage(ctx, tx, ReadingScopePurgeRequest{
				ScopeID:       scope,
				LibraryHandle: handle,
				Save: func(_ context.Context, _ *sql.Tx, stored string) error {
					st.Cursor = stored
					return nil
				},
			})
		}
		if err != nil {
			return false, err
		}
		if out.Done {
			// The scope row is proved gone (including its slot detach,
			// builds/captures/representatives/stages, keys and entities). Drop
			// the sub-cursor and pick the next scope. A Done with no work is a
			// pure proof (for example a scope already removed), so it may
			// continue in this transaction without an effect.
			st.Scope, st.Cursor = 0, ""
			if out.Worked {
				return true, nil
			}
			continue
		}
		if !out.Worked {
			return false, fmt.Errorf("Reading scope %d page made no progress", scope)
		}
		st.Cursor = out.StoredCursor
		return true, nil
	}
}

// readingTerminalResidueStep drains one bounded library-keyed slice or one raw
// source barrier/block row. It reports worked=true when the caller must save
// and yield. When it reports false the residue is proved empty for the
// library's handle and current sources.
func readingTerminalResidueStep(ctx context.Context, tx *sql.Tx, handle int64, st *readingTerminalState) (bool, error) {
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{readingTerminalPendingDeleteSQL, []any{handle, readingTerminalWorkBatch}},
		{readingTerminalWorkDeleteSQL, []any{handle, readingTerminalWorkBatch}},
		{readingTerminalSemanticDeleteSQL, []any{handle}},
		{readingTerminalInputsDeleteSQL, []any{handle}},
	} {
		n, err := readingTerminalExec(ctx, tx, q.sql, q.args...)
		if err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	worked, err := readingTerminalSourceStep(ctx, tx, handle, st)
	if err != nil {
		return false, err
	}
	if worked {
		return true, nil
	}
	// A scope must not reappear behind the residue pass. The scope page's own
	// verification rewinds one scope; this is the library-level equivalent, so
	// a reappearing scope is drained before the final delete rather than
	// hidden by it.
	var scopes int64
	if err := tx.QueryRowContext(ctx, readingTerminalScopeCountSQL, handle).Scan(&scopes); err != nil {
		return false, err
	}
	if scopes != 0 {
		st.Stage = readingTerminalScopeStage
		st.Scope, st.Cursor, st.Source = 0, "", ""
		return true, nil
	}
	return false, nil
}

// readingTerminalSourceStep reads the raw source window BEFORE any filter and
// then touches at most one barrier row or one block row of one source. A source
// is only passed once both its barrier and all its blocks are gone.
func readingTerminalSourceStep(ctx context.Context, tx *sql.Tx, handle int64, st *readingTerminalState) (bool, error) {
	rows, err := tx.QueryContext(ctx, readingTerminalSourceWindowSQL, handle, st.Source, readingTerminalSourceWindow)
	if err != nil {
		return false, err
	}
	var sources []string
	for rows.Next() {
		var source string
		if err = rows.Scan(&source); err != nil {
			rows.Close()
			return false, err
		}
		sources = append(sources, source)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, source := range sources {
		n, err := readingTerminalExec(ctx, tx, readingTerminalBarrierDeleteSQL, source)
		if err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
		n, err = readingTerminalExec(ctx, tx, readingTerminalBlockDeleteSQL, source)
		if err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
		st.Source = source
	}
	if len(sources) == readingTerminalSourceWindow {
		// The window was full: persist the forward position and read the next
		// raw window on the following turn.
		return true, nil
	}
	return false, nil
}

// readingTerminalFinalTx deletes the library row with the unchanged existing
// statement set, drains the Reading rows its own triggers requeue, and then
// proves the handle's terminal emptiness in this same writer transaction.
func readingTerminalFinalTx(ctx context.Context, tx *sql.Tx, library string, handle int64) error {
	if err := removeLibraryRowTx(ctx, tx, library); err != nil {
		return err
	}
	empty := false
	for round := 0; round < readingTerminalFinalRounds && !empty; round++ {
		n, err := readingTerminalDeleteResidue(ctx, tx, handle)
		if err != nil {
			return err
		}
		empty = n == 0
	}
	if !empty {
		return fmt.Errorf("%w: reading handle %d after %d bounded rounds", errReadingTerminalResidue, handle, readingTerminalFinalRounds)
	}
	// Builds, captures, representatives, stages, object keys and scope entities
	// cannot outlive their `reading_path_scope` row: every one has a restricting
	// foreign key to it. Proving no scope row remains therefore proves the
	// private scope tree is gone. Barriers and blocks were proved empty in the
	// residue turn immediately before this one, and the final row statements
	// above cannot create either (no delete/update trigger does).
	for _, probe := range []struct {
		name, query string
	}{
		{"reading_path_scope", readingTerminalScopeCountSQL},
		{"reading_path_inputs", `SELECT EXISTS(SELECT 1 FROM reading_path_inputs WHERE library_id=?1)`},
		{"reading_path_dirty_work", `SELECT EXISTS(SELECT 1 FROM reading_path_dirty_work WHERE library_id=?1)`},
		{"reading_path_semantic_pending", `SELECT EXISTS(SELECT 1 FROM reading_path_semantic_pending WHERE library_id=?1)`},
		{"reading_path_availability_pending", `SELECT EXISTS(SELECT 1 FROM reading_path_availability_pending WHERE library_id=?1)`},
	} {
		var n int64
		if err := tx.QueryRowContext(ctx, probe.query, handle).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			return fmt.Errorf("%w: %s still names Reading handle %d", errReadingTerminalResidue, probe.name, handle)
		}
	}
	return nil
}

func readingTerminalDeleteResidue(ctx context.Context, tx *sql.Tx, handle int64) (int64, error) {
	var total int64
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{readingTerminalPendingDeleteSQL, []any{handle, readingTerminalWorkBatch}},
		{readingTerminalWorkDeleteSQL, []any{handle, readingTerminalWorkBatch}},
		{readingTerminalSemanticDeleteSQL, []any{handle}},
		{readingTerminalInputsDeleteSQL, []any{handle}},
	} {
		n, err := readingTerminalExec(ctx, tx, q.sql, q.args...)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func readingTerminalExec(ctx context.Context, tx *sql.Tx, query string, args ...any) (int64, error) {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
