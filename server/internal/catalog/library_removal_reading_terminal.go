package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"

	"portico.local/server/internal/phasedpurge"
)

// Phase 5 owns private Reading lifetime cleanup, independently of dispatcher
// admission. The captured retired handle remains alive throughout this drain.
// The generic library/source delete is NOT an admitted physical unit: after
// draining, return a visible blocker without invoking its savepoint suppression.
// See P2-59-O022-READING-TERMINAL-r2.md for the literal cost and final-effect proof.
const (
	readingTerminalScopeStage        = "scope"
	readingTerminalResidueStage      = "residue"
	// Includes caller handle/progress reads, our lifetime/scope/source points,
	// both engine meter reads and conservative spare point probes. Save costs 1.
	readingTerminalOwnerReads = 16
	// FK/trigger-inclusive source envelope for the unchanged scope helper:
	// engine runs at most four units, or eight empty proofs (at most 4 each).
	// 16+4*12=64 and 16+8*4=48 stay below the shared 128 reads.
	readingTerminalScopeUnitReads    = 12
	readingTerminalLifetimeSQL       = `SELECT library_id,retired FROM catalog_libraries WHERE id=?1`
	readingTerminalScopePickSQL      = `SELECT id FROM reading_path_scope INDEXED BY reading_path_scope_library WHERE library_id=?1 ORDER BY id LIMIT 1`
	readingTerminalSourceFirstSQL    = `SELECT id FROM library_sources INDEXED BY library_sources_library WHERE library_id=?1 ORDER BY id LIMIT 1`
	readingTerminalSourcePickSQL     = `SELECT id FROM library_sources INDEXED BY library_sources_library WHERE library_id=?1 AND id>?2 ORDER BY id LIMIT 1`
)

var (
	errReadingTerminalPlan       = errors.New("Reading terminal cursor does not belong to this compiled plan")
	errReadingTerminalCursor     = errors.New("malformed Reading terminal cursor")
	errReadingTerminalFinalUnit  = errors.New("Reading private drain complete; library/source final physical unit is not admitted: removeLibraryRowTx has unbounded source/root and generic reference deletes and suppresses DELETE failure with SAVEPOINT library_row; keep retired handle and phase-5 cursor")
)

// Each descriptor selects ONE raw child, deletes by its full PK, and probes
// emptiness through that same leading index. effects includes trigger writes.
// Source selection is a separate raw LIMIT 1, never a bound on its children.
type readingTerminalUnit struct {
	name             string
	reads, effects   int
	pick, del, empty string
	source           bool
}

var readingTerminalUnits = []readingTerminalUnit{
	{"availability", 5, 3,
		`SELECT entity_id FROM reading_path_availability_pending INDEXED BY reading_path_availability_library WHERE library_id=?1 ORDER BY entity_id LIMIT 1`,
		`DELETE FROM reading_path_availability_pending WHERE entity_id=?1 AND library_id=?2`,
		`SELECT NOT EXISTS(SELECT 1 FROM reading_path_availability_pending INDEXED BY reading_path_availability_library WHERE library_id=?1)`, false},
	{"semantic", 2, 1,
		`SELECT library_id FROM reading_path_semantic_pending WHERE library_id=?1`,
		`DELETE FROM reading_path_semantic_pending WHERE library_id=?1 AND library_id=?2`,
		`SELECT NOT EXISTS(SELECT 1 FROM reading_path_semantic_pending WHERE library_id=?1)`, false},
	{"inputs", 2, 1,
		`SELECT library_id FROM reading_path_inputs WHERE library_id=?1`,
		`DELETE FROM reading_path_inputs WHERE library_id=?1 AND library_id=?2`,
		`SELECT NOT EXISTS(SELECT 1 FROM reading_path_inputs WHERE library_id=?1)`, false},
	{"work", 2, 1,
		`SELECT kind,work_key FROM reading_path_dirty_work WHERE library_id=?1 ORDER BY kind,work_key LIMIT 1`,
		`DELETE FROM reading_path_dirty_work WHERE library_id=?1 AND kind=?2 AND work_key=?3`,
		`SELECT NOT EXISTS(SELECT 1 FROM reading_path_dirty_work WHERE library_id=?1)`, false},
	{"blocks", 2, 1,
		`SELECT id FROM reading_path_block INDEXED BY reading_path_block_source WHERE source_id=?1 ORDER BY id LIMIT 1`,
		`DELETE FROM reading_path_block WHERE id=?1 AND source_id=?2`,
		`SELECT NOT EXISTS(SELECT 1 FROM reading_path_block INDEXED BY reading_path_block_source WHERE source_id=?1)`, true},
	{"barrier", 2, 1,
		`SELECT source_id FROM reading_path_source_barrier WHERE source_id=?1`,
		`DELETE FROM reading_path_source_barrier WHERE source_id=?1 AND source_id=?2`,
		`SELECT NOT EXISTS(SELECT 1 FROM reading_path_source_barrier WHERE source_id=?1)`, true},
}

// Digest binds the parent state, scope plan, residue plan, literals, declared
// costs, ordering, control-save semantics and unchanged shared engine limits.
var readingTerminalDigest = func() string {
	h := sha256.New()
	write := func(s string) {
		fmt.Fprintf(h, "%d:%s|", len(s), s)
	}
	for _, s := range []string{"p2-59-reading-terminal-r2", ReadingScopePurgePlanDigest(),
		readingTerminalLifetimeSQL, readingTerminalScopePickSQL, readingTerminalSourceFirstSQL, readingTerminalSourcePickSQL,
		"control:empty-unit/verify/finalize-save", "source:raw-successor/verify/advance-save",
		"canonical-parent/scope/residue-cursors;source-binding-before-scope-rewind;missing-scope-requires-terminal-unit",
		strconv.Itoa(readingTerminalOwnerReads), strconv.Itoa(readingTerminalScopeUnitReads), "save-effects:1",
		strconv.Itoa(phasedpurge.EffectLimit), strconv.Itoa(phasedpurge.ReadLimit)} {
		write(s)
	}
	for _, u := range readingTerminalUnits {
		for _, s := range []string{u.name, "phase:0", strconv.Itoa(u.reads), strconv.Itoa(u.effects),
			u.pick, u.del, u.empty, strconv.FormatBool(u.source)} {
			write(s)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}()

type readingTerminalState struct {
	Digest      string `json:"d"`
	Handle      int64  `json:"h"`
	Stage       string `json:"s"`
	Scope       int64  `json:"q,omitempty"`
	ScopeCursor string `json:"p,omitempty"`
	// After is advanced only after all units for the raw source were verified.
	After  *string `json:"a,omitempty"`
	Source *string `json:"r,omitempty"`
	Cursor string  `json:"c,omitempty"`
}

func readingTerminalLoad(raw string, handle int64) (readingTerminalState, error) {
	fresh := readingTerminalState{Digest: readingTerminalDigest, Handle: handle, Stage: readingTerminalScopeStage}
	if raw == "" {
		return fresh, nil
	}
	var st readingTerminalState
	if !utf8.ValidString(raw) {
		return st, errReadingTerminalCursor
	}
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&st); err != nil {
		return st, fmt.Errorf("%w: %v", errReadingTerminalCursor, err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return st, errReadingTerminalCursor
	}
	if st.Digest != readingTerminalDigest {
		return st, errReadingTerminalPlan
	}
	if st.Handle != handle || handle <= 0 || st.Scope < 0 {
		return st, errReadingTerminalCursor
	}
	if st.Stage != readingTerminalScopeStage && st.Stage != readingTerminalResidueStage {
		return st, errReadingTerminalCursor
	}
	if st.Stage == readingTerminalScopeStage && (st.After != nil || st.Source != nil || st.Cursor != "" || (st.Scope == 0 && st.ScopeCursor != "")) {
		return st, errReadingTerminalCursor
	}
	if st.Stage == readingTerminalResidueStage && (st.Scope != 0 || st.ScopeCursor != "") {
		return st, errReadingTerminalCursor
	}
	if st.Scope > 0 {
		if st.ScopeCursor == "" {
			return st, errReadingTerminalCursor
		}
		inner, err := readingScopePurgeDecodeStored(st.ScopeCursor)
		if err != nil {
			return st, err
		}
		var envelope readingScopePurgeStoredCursor
		if err := json.Unmarshal([]byte(st.ScopeCursor), &envelope); err != nil {
			return st, ErrReadingScopePurgeCursor
		}
		canonical, err := json.Marshal(envelope)
		if err != nil || string(canonical) != st.ScopeCursor {
			return st, ErrReadingScopePurgeCursor
		}
		cursor, err := phasedpurge.DecodeCursor(inner, readingScopePurgePlan(&readingScopePurgeState{}))
		if err != nil {
			return st, fmt.Errorf("%w: %w", ErrReadingScopePurgeCursor, err)
		}
		canonicalInner, err := phasedpurge.EncodeCursor(cursor)
		if err != nil || canonicalInner != inner {
			return st, ErrReadingScopePurgeCursor
		}
	}
	// Only our canonical persisted encoding is accepted (also rejects duplicate
	// fields, null and omitted/extra state). Never repair or reset an invalid plan.
	encoded, err := json.Marshal(st)
	if err != nil || string(encoded) != raw {
		return st, errReadingTerminalCursor
	}
	return st, nil
}

func readingTerminalSave(ctx context.Context, tx *sql.Tx, p *removalProgress, st readingTerminalState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	p.objectCursor = string(raw)
	return p.saveTx(ctx, tx)
}

// This function saves inside the engine meter. The phase-5 hook MUST yield
// directly, without a second save or entering the generic final delete.
func removeLibraryReadingTerminalTx(ctx context.Context, tx *sql.Tx, library string, handle int64, p *removalProgress) (bool, error) {
	st, err := readingTerminalLoad(p.objectCursor, handle)
	if err != nil {
		return false, err
	}
	// Validate a residue cursor before considering a legitimate scope rewind.
	if st.Stage == readingTerminalResidueStage {
		cursor, err := phasedpurge.DecodeCursor(st.Cursor, readingTerminalPlan(handle, sql.NullString{}))
		if err != nil {
			return false, fmt.Errorf("%w: %w", errReadingTerminalCursor, err)
		}
		if st.Cursor != "" {
			canonical, err := phasedpurge.EncodeCursor(cursor)
			if err != nil || canonical != st.Cursor {
				return false, errReadingTerminalCursor
			}
		}
	}
	var actual string
	var retired int
	if err := tx.QueryRowContext(ctx, readingTerminalLifetimeSQL, handle).Scan(&actual, &retired); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrReadingScopePurgeLifetime
		}
		return false, err
	}
	if actual != library || retired != 1 {
		return false, ErrReadingScopePurgeLifetime
	}

	// One raw source; full child PKs, not this parent limit, bound each delete.
	var source sql.NullString
	if st.Stage == readingTerminalResidueStage {
		if st.After == nil {
			err = tx.QueryRowContext(ctx, readingTerminalSourceFirstSQL, library).Scan(&source)
		} else {
			err = tx.QueryRowContext(ctx, readingTerminalSourcePickSQL, library, *st.After).Scan(&source)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if st.Source != nil && (!source.Valid || source.String != *st.Source) {
			return false, fmt.Errorf("%w: captured source changed during residue proof", errReadingTerminalCursor)
		}
		if source.Valid {
			st.Source = &source.String
		}
	}

	// The non-runnable index is deliberate: parked/settled/domain28 state has
	// no authority over private lifetime reclamation.
	if st.Scope == 0 {
		var scope int64
		err := tx.QueryRowContext(ctx, readingTerminalScopePickSQL, handle).Scan(&scope)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if err == nil {
			// A real reappearing scope requires a fresh residue proof afterward.
			// This is state-driven invalidation, never recovery from a bad cursor.
			st.Stage, st.Scope, st.After, st.Source, st.Cursor = readingTerminalScopeStage, scope, nil, nil, ""
		}
	}
	if st.Scope != 0 {
		out, err := RetireReadingScopePage(ctx, tx, ReadingScopePurgeRequest{
			ScopeID: st.Scope, LibraryHandle: handle, StoredCursor: st.ScopeCursor,
			Save: func(ctx context.Context, tx *sql.Tx, stored string) error {
				st.ScopeCursor = stored
				return readingTerminalSave(ctx, tx, p, st)
			},
		})
		if err != nil {
			return false, err // including strict scope-plan/cursor refusal
		}
		if !out.Done {
			if !out.Worked {
				return false, phasedpurge.ErrNoProgress
			}
			return true, nil
		}
		// An absent scope is expected only after this plan's scope DELETE
		// persisted Unit==len(plan). An arbitrary missing target is a visible
		// mismatch, not a reason to skip to another scope.
		inner, err := readingScopePurgeDecodeStored(st.ScopeCursor)
		if err != nil {
			return false, err
		}
		cursor, err := phasedpurge.DecodeCursor(inner, readingScopePurgePlan(&readingScopePurgeState{}))
		if err != nil || cursor.Unit != len(readingScopePurgeSteps) || cursor.Verify != 0 {
			return false, fmt.Errorf("%w: scope disappeared before its terminal cursor", errReadingTerminalCursor)
		}
		st.Scope, st.ScopeCursor = 0, ""
		return readingTerminalSaveTurn(ctx, tx, p, st)
	}
	if st.Stage == readingTerminalScopeStage {
		st.Stage = readingTerminalResidueStage
		return readingTerminalSaveTurn(ctx, tx, p, st)
	}

	plan := readingTerminalPlan(handle, source)
	cursor, err := phasedpurge.DecodeCursor(st.Cursor, plan)
	if err != nil {
		return false, fmt.Errorf("%w: %w", errReadingTerminalCursor, err)
	}
	_, err = phasedpurge.Turn(ctx, tx, plan, cursor, phasedpurge.Owner{
		ReadBound: readingTerminalOwnerReads,
		Save: func(ctx context.Context, tx *sql.Tx, c phasedpurge.Cursor) error {
			var err error
			st.Cursor, err = phasedpurge.EncodeCursor(c)
			if err != nil {
				return err
			}
			return readingTerminalSave(ctx, tx, p, st)
		},
		Finalize: func(ctx context.Context, tx *sql.Tx) error {
			if !source.Valid {
				// No generic DELETE, no hidden savepoint failure, no final drain
				// loop. Caller rolls back this proof-only turn on the blocker.
				return errReadingTerminalFinalUnit
			}
			st.After, st.Source, st.Cursor = &source.String, nil, ""
			return readingTerminalSave(ctx, tx, p, st)
		},
	})
	return err == nil, err
}

// State-only transitions still save once INSIDE the existing engine's meter.
func readingTerminalSaveTurn(ctx context.Context, tx *sql.Tx, p *removalProgress, st readingTerminalState) (bool, error) {
	plan := []phasedpurge.Unit{{Phase: 0, Name: "terminal_state", ReadBound: 1,
		Run: func(context.Context, *sql.Tx, []phasedpurge.Key) (phasedpurge.Outcome, error) {
			return phasedpurge.Outcome{Drained: true}, nil
		},
		Empty: func(context.Context, *sql.Tx) (bool, error) {
			return true, nil
		},
	}}
	_, err := phasedpurge.Turn(ctx, tx, plan, phasedpurge.Cursor{Unit: len(plan)}, phasedpurge.Owner{
		ReadBound: readingTerminalOwnerReads,
		Save: func(context.Context, *sql.Tx, phasedpurge.Cursor) error {
			return phasedpurge.ErrPlan
		},
		Finalize: func(ctx context.Context, tx *sql.Tx) error {
			return readingTerminalSave(ctx, tx, p, st)
		},
	})
	return err == nil, err
}

func readingTerminalPlan(handle int64, source sql.NullString) []phasedpurge.Unit {
	plan := make([]phasedpurge.Unit, 0, len(readingTerminalUnits))
	for _, step := range readingTerminalUnits {
		u := step
		var owner any = handle
		if u.source {
			owner = source.String
		}
		plan = append(plan, phasedpurge.Unit{Phase: 0, Name: u.name, ReadBound: u.reads,
			Run: func(ctx context.Context, tx *sql.Tx, _ []phasedpurge.Key) (phasedpurge.Outcome, error) {
				if u.source && !source.Valid {
					return phasedpurge.Outcome{Drained: true}, nil
				}
				var args []any
				if u.name == "work" {
					var kind, key string
					err := tx.QueryRowContext(ctx, u.pick, owner).Scan(&kind, &key)
					if errors.Is(err, sql.ErrNoRows) {
						return phasedpurge.Outcome{Drained: true}, nil
					}
					if err != nil {
						return phasedpurge.Outcome{}, err
					}
					args = []any{owner, kind, key}
				} else {
					var key any
					err := tx.QueryRowContext(ctx, u.pick, owner).Scan(&key)
					if errors.Is(err, sql.ErrNoRows) {
						return phasedpurge.Outcome{Drained: true}, nil
					}
					if err != nil {
						return phasedpurge.Outcome{}, err
					}
					args = []any{key, owner}
				}
				result, err := tx.ExecContext(ctx, u.del, args...)
				if err != nil {
					return phasedpurge.Outcome{}, err
				}
				n, err := result.RowsAffected()
				if err != nil {
					return phasedpurge.Outcome{}, err
				}
				if n != 1 {
					return phasedpurge.Outcome{}, fmt.Errorf("Reading terminal %s selected one row, deleted %d", u.name, n)
				}
				return phasedpurge.Outcome{Changed: true}, nil
			},
			Empty: func(ctx context.Context, tx *sql.Tx) (bool, error) {
				if u.source && !source.Valid {
					return true, nil
				}
				var empty bool
				err := tx.QueryRowContext(ctx, u.empty, owner).Scan(&empty)
				return empty, err
			},
		})
	}
	return plan
}
