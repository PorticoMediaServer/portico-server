package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/phasedpurge"
)

// Real installed baseline, real catalogue/Reading rows, and the actual phase-5
// hook. Earlier physical phases and the unadmitted generic final delete belong
// to root; these tests explicitly require a visible blocker after the drain.
func readingTerminalTestStart(t *testing.T, c *catalogtest.Catalog, library string, handle int64) {
	t.Helper()
	c.Exec(`UPDATE catalog_libraries SET retired=1 WHERE id=?`, handle)
	c.Exec(`INSERT INTO catalog_removal_progress(kind,target,phase) VALUES('library',?,5)`, library)
}

func readingTerminalTestStep(t *testing.T, c *catalogtest.Catalog, library string) (bool, int64, error) {
	t.Helper()
	var done bool
	var effects int64
	err := dbwork.WithWriteTx(context.Background(), c.DB, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		var before, after int64
		if err := tx.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
			return err
		}
		var err error
		done, err = removeLibraryBatchTx(context.Background(), tx, library)
		if e := tx.QueryRow(`SELECT total_changes()`).Scan(&after); e != nil {
			return e
		}
		effects = after - before
		return err
	})
	if effects > phasedpurge.EffectLimit || effects < 0 {
		t.Fatalf("phase-5 turn changed %d, shared limit %d", effects, phasedpurge.EffectLimit)
	}
	return done, effects, err
}

func readingTerminalTestDrain(t *testing.T, c *catalogtest.Catalog, library string, builds ...int64) {
	t.Helper()
	for i := 0; i < 400; i++ {
		var before int
		for _, build := range builds {
			before += readingPurgeChildRows(t, c, build)
		}
		done, effects, err := readingTerminalTestStep(t, c, library)
		var after int
		for _, build := range builds {
			after += readingPurgeChildRows(t, c, build)
		}
		if before-after > 1 {
			t.Fatalf("turn deleted %d raw build children", before-after)
		}
		if done {
			t.Fatal("unadmitted generic final deletion reported Done")
		}
		if errors.Is(err, errReadingTerminalFinalUnit) {
			if effects != 0 {
				t.Fatalf("final blocker changed %d rows", effects)
			}
			return
		}
		if err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
	}
	t.Fatal("private Reading drain did not reach its final physical-unit blocker")
}

func readingTerminalTestState(t *testing.T, c *catalogtest.Catalog, library string) readingTerminalState {
	t.Helper()
	var raw string
	if err := c.DB.QueryRow(`SELECT object_cursor FROM catalog_removal_progress WHERE kind='library' AND target=?`, library).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var st readingTerminalState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func readingTerminalTestBarrier(t *testing.T, c *catalogtest.Catalog, library, root string, n int) string {
	t.Helper()
	var source string
	if err := c.DB.QueryRow(`SELECT id FROM library_sources WHERE library_id=?`, library).Scan(&source); err != nil {
		t.Fatal(err)
	}
	c.Exec(`INSERT INTO reading_path_source_barrier(source_id,job_id,incarnation,source_generation,root_identity,root,phase) VALUES(?,?,?,?,?,?,'cancelled')`, source, "terminal-"+library, "inc", 1, "root-id", root)
	for i := 0; i < n; i++ {
		c.Exec(`INSERT INTO reading_path_block(job_id,source_id,directory_key,mode,reason) VALUES(?,?,?,?,?)`, "terminal-"+library, source, fmt.Sprintf("block-%d", i), "scope", "invalid_reading_path")
	}
	return source
}

func readingTerminalTestEmpty(t *testing.T, c *catalogtest.Catalog, library string, handle int64, source string) {
	t.Helper()
	for _, table := range []string{"reading_path_scope", "reading_path_inputs", "reading_path_dirty_work", "reading_path_semantic_pending", "reading_path_availability_pending"} {
		if n := rows(t, c, `SELECT count(*) FROM `+table+` WHERE library_id=?`, handle); n != 0 {
			t.Fatalf("%s kept %d rows", table, n)
		}
	}
	for _, table := range []string{"reading_path_source_barrier", "reading_path_block"} {
		if n := rows(t, c, `SELECT count(*) FROM `+table+` WHERE source_id=?`, source); n != 0 {
			t.Fatalf("%s kept %d rows", table, n)
		}
	}
	if rows(t, c, `SELECT count(*) FROM catalog_libraries WHERE id=? AND library_id=? AND retired=1`, handle, library) != 1 {
		t.Fatal("retired lifetime disappeared before root's final unit")
	}
	if rows(t, c, `SELECT count(*) FROM libraries WHERE id=?`, library) != 1 {
		t.Fatal("public library disappeared at the source blocker")
	}
	if rows(t, c, `SELECT count(*) FROM catalog_removal_progress WHERE kind='library' AND target=? AND phase=5 AND object_cursor<>''`, library) != 1 {
		t.Fatal("source blocker lost phase-5 progress")
	}
}

func TestLibraryRemovalReadingTerminalDrainsPrivateScopes(t *testing.T) {
	for _, siblingSlot := range []bool{false, true} {
		t.Run(fmt.Sprintf("sibling-slot-%v", siblingSlot), func(t *testing.T) {
			c := catalogtest.Open(t)
			keepRoot, root := t.TempDir(), t.TempDir()
			keep := c.Library("keep", "Keep", "book", keepRoot)
			lib := c.Library("r", "Reading", "book", root)
			keepBooks := readingPurgeBooks(t, c, keep, keepRoot, "Keep", 1)
			keepScope := readingPurgeScope(t, c, keep, "Keep")
			keepBuild := readingPurgeBuild(t, c, keepScope, "flatten", keepBooks)
			keepState, keepCounts := readingPurgeScopeState(t, c, keepScope), readingPurgeScopeCounts(t, c, keepScope)
			keepChildren := readingPurgeChildRows(t, c, keepBuild)
			keepSource := readingTerminalTestBarrier(t, c, "keep", keepRoot, 3)

			books := readingPurgeBooks(t, c, lib, root, "Activated", 2)
			active := readingPurgeScope(t, c, lib, "Activated")
			activeBuild := readingPurgeBuild(t, c, active, "flatten", books)
			parkBooks := readingPurgeBooks(t, c, lib, root, "Parked", 1)
			parked := readingPurgeScope(t, c, lib, "Parked")
			parkBuild := readingPurgeBuild(t, c, parked, "capture", parkBooks)
			readingPurgeBooks(t, c, lib, root, "Settled", 1)
			settled := readingPurgeScope(t, c, lib, "Settled")
			c.Exec(`UPDATE reading_path_scope SET queued=0,parked=1 WHERE id=?`, parked)
			c.Exec(`UPDATE reading_path_scope SET queued=0,parked=0,completed=desired WHERE id=?`, settled)
			c.Exec(`INSERT INTO catalog_dirty(domain,entity_id,revision) VALUES(28,?1,1) ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1`, lib)
			c.Exec(`INSERT INTO catalog_derivation_failures(domain,entity_id,reason,attempts,failed_ms) VALUES(28,?1,'terminal-test',1,1) ON CONFLICT(domain,entity_id) DO UPDATE SET attempts=attempts+1`, lib)
			slot := activeBuild
			if siblingSlot {
				slot = keepBuild
			}
			c.Exec(`UPDATE reading_path_slot SET build_id=?1,overlay_id=?1 WHERE singleton=1`, slot)
			source := readingTerminalTestBarrier(t, c, "r", root, 5)
			var sequence, turn int64
			if err := c.DB.QueryRow(`SELECT sequence,turn FROM reading_path_clock WHERE singleton=1`).Scan(&sequence, &turn); err != nil {
				t.Fatal(err)
			}
			readingTerminalTestStart(t, c, "r", lib)
			readingTerminalTestDrain(t, c, "r", activeBuild, parkBuild)
			for _, scope := range []int64{active, parked, settled} {
				if got := readingPurgeScopeCounts(t, c, scope); got != (readingPurgeCounts{}) {
					t.Fatalf("scope %d kept %+v", scope, got)
				}
			}
			if readingPurgeChildRows(t, c, activeBuild)+readingPurgeChildRows(t, c, parkBuild) != 0 {
				t.Fatal("private build children remain")
			}
			readingTerminalTestEmpty(t, c, "r", lib, source)
			if got := readingPurgeScopeState(t, c, keepScope); got != keepState {
				t.Fatal("sibling scope counters changed")
			}
			if got := readingPurgeScopeCounts(t, c, keepScope); got != keepCounts {
				t.Fatal("sibling scope rows changed")
			}
			if readingPurgeChildRows(t, c, keepBuild) != keepChildren {
				t.Fatal("sibling build changed")
			}
			if rows(t, c, `SELECT count(*) FROM reading_path_source_barrier WHERE source_id=?`, keepSource) != 1 || rows(t, c, `SELECT count(*) FROM reading_path_block WHERE source_id=?`, keepSource) != 3 {
				t.Fatal("sibling source residue changed")
			}
			if rows(t, c, `SELECT count(*) FROM catalog_libraries WHERE id=? AND retired=0`, keep) != 1 {
				t.Fatal("live sibling library changed")
			}
			var build, overlay sql.NullInt64
			if err := c.DB.QueryRow(`SELECT build_id,overlay_id FROM reading_path_slot WHERE singleton=1`).Scan(&build, &overlay); err != nil {
				t.Fatal(err)
			}
			if siblingSlot {
				if !build.Valid || !overlay.Valid || build.Int64 != keepBuild || overlay.Int64 != keepBuild {
					t.Fatal("sibling live slot changed")
				}
			} else if build.Valid || overlay.Valid {
				t.Fatal("retired slot remains attached")
			}
			var afterSequence, afterTurn int64
			if err := c.DB.QueryRow(`SELECT sequence,turn FROM reading_path_clock WHERE singleton=1`).Scan(&afterSequence, &afterTurn); err != nil {
				t.Fatal(err)
			}
			if sequence != afterSequence || turn != afterTurn {
				t.Fatal("shared Reading clock changed")
			}
			if rows(t, c, `SELECT count(*) FROM catalog_dirty WHERE domain=28 AND entity_id=?`, lib) != 1 || rows(t, c, `SELECT count(*) FROM catalog_derivation_failures WHERE domain=28 AND entity_id=?`, lib) != 1 {
				t.Fatal("private cleanup consumed domain28 admission state")
			}
		})
	}
}

func TestLibraryRemovalReadingTerminalResidueIsOneRawRow(t *testing.T) {
	c := catalogtest.Open(t)
	root := t.TempDir()
	lib := c.Library("r", "Reading", "book", root)
	source := readingTerminalTestBarrier(t, c, "r", root, 5)
	// No FK on availability pending. These are real baseline residue rows,
	// deliberately more than the rejected 128-row slice size.
	for i := 1; i <= 129; i++ {
		c.Exec(`INSERT INTO reading_path_availability_pending(entity_id,library_id) VALUES(?,?)`, i, lib)
	}
	c.Exec(`DELETE FROM reading_path_dirty_work WHERE library_id=? AND kind='pending' AND work_key='1'`, lib)
	c.Exec(`INSERT OR IGNORE INTO reading_path_semantic_pending(library_id) VALUES(?)`, lib)
	readingTerminalTestStart(t, c, "r", lib)
	residueCounts := func() []int {
		return []int{
			rows(t, c, `SELECT count(*) FROM reading_path_availability_pending WHERE library_id=?`, lib),
			rows(t, c, `SELECT count(*) FROM reading_path_semantic_pending WHERE library_id=?`, lib),
			rows(t, c, `SELECT count(*) FROM reading_path_inputs WHERE library_id=?`, lib),
			rows(t, c, `SELECT count(*) FROM reading_path_dirty_work WHERE library_id=?`, lib),
			rows(t, c, `SELECT count(*) FROM reading_path_block WHERE source_id=?`, source),
			rows(t, c, `SELECT count(*) FROM reading_path_source_barrier WHERE source_id=?`, source),
		}
	}
	blocked := false
	for i := 0; i < 400; i++ {
		before := residueCounts()
		done, _, err := readingTerminalTestStep(t, c, "r")
		if done {
			t.Fatal("unadmitted final unit reported Done")
		}
		after := residueCounts()
		deleted := 0
		for j := range before {
			if before[j] > after[j] {
				deleted += before[j] - after[j]
			}
		}
		if deleted > 1 {
			t.Fatalf("turn deleted %d raw residue rows", deleted)
		}
		if errors.Is(err, errReadingTerminalFinalUnit) {
			blocked = true
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !blocked {
		t.Fatal("residue drain did not reach its blocker within 400 turns")
	}
	readingTerminalTestEmpty(t, c, "r", lib, source)
}

func TestLibraryRemovalReadingTerminalResumesAfterDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terminal.sqlite")
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := catalogtest.New(t, db)
	t.Cleanup(func() { _ = c.DB.Close() })
	reopen := func() {
		t.Helper()
		if err := c.DB.Close(); err != nil {
			t.Fatal(err)
		}
		var err error
		c.DB, err = persistence.Open(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	lib := c.Library("r", "Reading", "book", root)
	books := readingPurgeBooks(t, c, lib, root, "Shelf", 2)
	scope := readingPurgeScope(t, c, lib, "Shelf")
	build := readingPurgeBuild(t, c, scope, "flatten", books)
	c.Exec(`UPDATE reading_path_slot SET build_id=?1,overlay_id=?1 WHERE singleton=1`, build)
	source := readingTerminalTestBarrier(t, c, "r", root, 5)
	readingTerminalTestStart(t, c, "r", lib)
	for i := 0; i < 2; i++ {
		if done, _, err := readingTerminalTestStep(t, c, "r"); err != nil || done {
			t.Fatalf("initial page: done=%v err=%v", done, err)
		}
	}
	if st := readingTerminalTestState(t, c, "r"); st.Scope != scope || st.ScopeCursor == "" {
		t.Fatal("scope page did not persist restart state")
	}
	reopen()
	residue := false
	for i := 0; i < 400; i++ {
		if done, _, err := readingTerminalTestStep(t, c, "r"); err != nil || done {
			t.Fatalf("resumed scope: done=%v err=%v", done, err)
		}
		if st := readingTerminalTestState(t, c, "r"); st.Stage == readingTerminalResidueStage && st.Source != nil && st.Cursor != "" {
			residue = true
			break
		}
	}
	if !residue {
		t.Fatal("residue page never persisted a source-bound cursor")
	}
	reopen()
	readingTerminalTestDrain(t, c, "r", build)
	readingTerminalTestEmpty(t, c, "r", lib, source)
	if got := readingPurgeScopeCounts(t, c, scope); got != (readingPurgeCounts{}) {
		t.Fatalf("reopened scope kept %+v", got)
	}
}

func TestLibraryRemovalReadingTerminalRefusesCursorMismatch(t *testing.T) {
	c := catalogtest.Open(t)
	root := t.TempDir()
	lib := c.Library("r", "Reading", "book", root)
	books := readingPurgeBooks(t, c, lib, root, "Shelf", 2)
	scope := readingPurgeScope(t, c, lib, "Shelf")
	build := readingPurgeBuild(t, c, scope, "flatten", books)
	c.Exec(`UPDATE reading_path_slot SET build_id=?1,overlay_id=?1 WHERE singleton=1`, build)
	readingTerminalTestStart(t, c, "r", lib)
	if _, _, err := readingTerminalTestStep(t, c, "r"); err != nil {
		t.Fatal(err)
	}
	valid := readingTerminalTestState(t, c, "r")
	encode := func(st readingTerminalState) string {
		b, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	cases := []struct {
		name  string
		alter func(*readingTerminalState)
		raw   string
		want  error
	}{
		{name: "outer-plan", alter: func(s *readingTerminalState) { s.Digest = "old-plan" }, want: errReadingTerminalPlan},
		{name: "lifetime", alter: func(s *readingTerminalState) { s.Handle++ }, want: errReadingTerminalCursor},
		{name: "stage", alter: func(s *readingTerminalState) { s.Stage = "phase6" }, want: errReadingTerminalCursor},
		{name: "missing-scope-row", alter: func(s *readingTerminalState) { s.Scope += 1000000 }, want: errReadingTerminalCursor},
		{name: "missing-scope-id", alter: func(s *readingTerminalState) { s.Scope = 0 }, want: errReadingTerminalCursor},
		{name: "scope-plan", alter: func(s *readingTerminalState) { s.ScopeCursor = `{"d":"old-plan","c":""}` }, want: ErrReadingScopePurgePlan},
		{name: "scope-extra-field", alter: func(s *readingTerminalState) { s.ScopeCursor = fmt.Sprintf(`{"d":%q,"c":"{}","extra":1}`, ReadingScopePurgePlanDigest()) }, want: ErrReadingScopePurgeCursor},
		{name: "scope-cursor", alter: func(s *readingTerminalState) { s.ScopeCursor = fmt.Sprintf(`{"d":%q,"c":"{bad"}`, ReadingScopePurgePlanDigest()) }, want: ErrReadingScopePurgeCursor},
		{name: "json", raw: "{bad", want: errReadingTerminalCursor},
		{name: "source-mismatch-before-scope-rewind", alter: func(s *readingTerminalState) { s.Stage, s.Scope, s.ScopeCursor = readingTerminalResidueStage, 0, ""; bad := "not-the-source"; s.Source = &bad }, want: errReadingTerminalCursor},
		{name: "residue-cursor", alter: func(s *readingTerminalState) { s.Stage, s.Scope, s.ScopeCursor, s.Cursor = readingTerminalResidueStage, 0, "", `{"u":1,"k":[{"t":"integer","v":"1"}]}` }, want: errReadingTerminalCursor},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := valid
			raw := tc.raw
			if tc.alter != nil {
				tc.alter(&st)
				raw = encode(st)
			}
			c.Exec(`UPDATE catalog_removal_progress SET object_cursor=? WHERE kind='library' AND target='r'`, raw)
			before := readingPurgeChildRows(t, c, build)
			done, effects, err := readingTerminalTestStep(t, c, "r")
			if done || !errors.Is(err, tc.want) {
				t.Fatalf("refusal: done=%v err=%v, want %v", done, err, tc.want)
			}
			if effects != 0 || readingPurgeChildRows(t, c, build) != before {
				t.Fatal("refused cursor changed source rows")
			}
			var stored string
			if err := c.DB.QueryRow(`SELECT object_cursor FROM catalog_removal_progress WHERE kind='library' AND target='r'`).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored != raw {
				t.Fatal("refused cursor was silently reset")
			}
		})
	}
}

func TestLibraryRemovalReadingTerminalCompiledCostEnvelope(t *testing.T) {
	if readingTerminalOwnerReads+4*readingTerminalScopeUnitReads > phasedpurge.ReadLimit {
		t.Fatal("scope run envelope exceeds shared reads")
	}
	if readingTerminalOwnerReads+8*4 > phasedpurge.ReadLimit {
		t.Fatal("scope verification envelope exceeds shared reads")
	}
	reads := readingTerminalOwnerReads
	for _, u := range readingTerminalUnits {
		reads += u.reads
		if u.effects+1 > phasedpurge.EffectLimit {
			t.Fatalf("%s plus Save exceeds shared effects", u.name)
		}
	}
	if reads > phasedpurge.ReadLimit {
		t.Fatalf("owner plus full plan declares %d reads", reads)
	}
}
