package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"portico.local/server/internal/catalogtest"
)

// Focused tests for the phase-5 Reading terminal drain. They drive the real
// `removeLibraryBatchTx` against real catalogue/reading_path rows; the shared
// `RetireReadingScopePage` tests remain the source-only proof of the scope
// unit itself.

// readingTerminalBuildChildren counts the build children the scope page deletes
// one raw row at a time. `reading_path_stage` is excluded because the installed
// publication cascade already removes it during the container phase.
func readingTerminalBuildChildren(t *testing.T, c *catalogtest.Catalog, build int64) int {
	t.Helper()
	return rows(t, c, `SELECT (SELECT count(*) FROM reading_path_capture WHERE build_id=?1)
 +(SELECT count(*) FROM reading_path_representative WHERE build_id=?1)`, build)
}

func readingTerminalHandleRows(t *testing.T, c *catalogtest.Catalog, handle int64) map[string]int {
	t.Helper()
	left := map[string]int{}
	for _, table := range []string{
		"reading_path_scope",
		"reading_path_inputs",
		"reading_path_dirty_work",
		"reading_path_semantic_pending",
		"reading_path_availability_pending",
	} {
		if n := rows(t, c, `SELECT count(*) FROM `+table+` WHERE library_id=?`, handle); n > 0 {
			left[table] = n
		}
	}
	return left
}

func readingTerminalBatch(t *testing.T, c *catalogtest.Catalog, library string) bool {
	t.Helper()
	done := false
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		done, err = removeLibraryBatchTx(ctx, tx, library)
		return err
	})
	return done
}

// The removal must reclaim an activated scope (slot + overlay + children), a
// parked scope behind domain28 work, and a settled scope with no builds, while
// preserving another reading library's scope and the global Reading clock. Each
// batch still deletes at most one raw build child, block or barrier row.
func TestLibraryRemovalReadingTerminalDrainsPrivateScopes(t *testing.T) {
	c := catalogtest.Open(t)
	keepRoot, root := t.TempDir(), t.TempDir()
	keep := c.Library("keep", "Keep", "book", keepRoot)
	lib := c.Library("r", "Reading", "book", root)

	keepBooks := readingPurgeBooks(t, c, keep, keepRoot, "Keep", 1)
	keepScope := readingPurgeScope(t, c, keep, "Keep")
	keepBuild := readingPurgeBuild(t, c, keepScope, "flatten", keepBooks)
	keepState := readingPurgeScopeState(t, c, keepScope)
	keepCounts := readingPurgeScopeCounts(t, c, keepScope)
	keepChildren := readingPurgeChildRows(t, c, keepBuild)

	shelfBooks := readingPurgeBooks(t, c, lib, root, "Shelf", 2)
	shelfScope := readingPurgeScope(t, c, lib, "Shelf")
	shelfBuild := readingPurgeBuild(t, c, shelfScope, "flatten", shelfBooks)

	boxBooks := readingPurgeBooks(t, c, lib, root, "Box", 1)
	boxScope := readingPurgeScope(t, c, lib, "Box")
	boxBuild := readingPurgeBuild(t, c, boxScope, "capture", boxBooks)

	readingPurgeBooks(t, c, lib, root, "Settled", 1)
	settledScope := readingPurgeScope(t, c, lib, "Settled")

	// Activated: the singleton slot names this scope's published build.
	c.Exec(`UPDATE reading_path_slot SET build_id=?1,overlay_id=?1 WHERE singleton=1`, shelfBuild)
	// Parked: no dispatcher will ever select it, and a domain28 failure says so.
	c.Exec(`UPDATE reading_path_scope SET queued=0,parked=1 WHERE id=?`, boxScope)
	c.Exec(`INSERT INTO catalog_dirty(domain,entity_id,revision) VALUES(28,?1,1) ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1`, lib)
	c.Exec(`INSERT INTO catalog_derivation_failures(domain,entity_id,reason,attempts,failed_ms) VALUES(28,?1,'terminal-test',1,1) ON CONFLICT(domain,entity_id) DO UPDATE SET attempts=attempts+1`, lib)
	// Settled: no build and nothing runnable.
	c.Exec(`UPDATE reading_path_scope SET queued=0,parked=0,completed=desired WHERE id=?`, settledScope)

	var source string
	if err := c.DB.QueryRow(`SELECT id FROM library_sources WHERE library_id='r'`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	c.Exec(`INSERT INTO reading_path_source_barrier(source_id,job_id,incarnation,source_generation,root_identity,root,phase) VALUES(?,?,?,?,?,?,'cancelled')`, source, "terminal-job", "inc", 1, "root-id", root)
	for i := 0; i < 5; i++ {
		c.Exec(`INSERT INTO reading_path_block(job_id,source_id,directory_key,mode,reason) VALUES(?,?,?,?,?)`, "terminal-job", source, fmt.Sprintf("block-%d", i), "scope", "invalid_reading_path")
	}

	var clock int64
	if err := c.DB.QueryRow(`SELECT sequence FROM reading_path_clock WHERE singleton=1`).Scan(&clock); err != nil {
		t.Fatal(err)
	}
	c.Exec(`UPDATE catalog_libraries SET retired=1 WHERE id=?`, lib)

	for i := 0; ; i++ {
		childrenBefore := readingTerminalBuildChildren(t, c, shelfBuild) + readingTerminalBuildChildren(t, c, boxBuild)
		blocksBefore := rows(t, c, `SELECT count(*) FROM reading_path_block WHERE source_id=?`, source)
		barriersBefore := rows(t, c, `SELECT count(*) FROM reading_path_source_barrier WHERE source_id=?`, source)
		done := readingTerminalBatch(t, c, "r")
		if drop := childrenBefore - (readingTerminalBuildChildren(t, c, shelfBuild) + readingTerminalBuildChildren(t, c, boxBuild)); drop > 1 {
			t.Fatalf("one batch deleted %d raw build children", drop)
		}
		if drop := blocksBefore - rows(t, c, `SELECT count(*) FROM reading_path_block WHERE source_id=?`, source); drop > 1 {
			t.Fatalf("one batch deleted %d raw blocks", drop)
		}
		if drop := barriersBefore - rows(t, c, `SELECT count(*) FROM reading_path_source_barrier WHERE source_id=?`, source); drop > 1 {
			t.Fatalf("one batch deleted %d raw barriers", drop)
		}
		if done {
			break
		}
		if i > 400 {
			t.Fatal("the removal did not finish")
		}
	}

	for _, scope := range []int64{shelfScope, boxScope, settledScope} {
		if got := readingPurgeScopeCounts(t, c, scope); got != (readingPurgeCounts{}) {
			t.Fatalf("scope %d left builds=%d keys=%d entities=%d scopes=%d", scope, got.builds, got.keys, got.entities, got.scopes)
		}
	}
	if readingPurgeChildRows(t, c, shelfBuild) != 0 || readingPurgeChildRows(t, c, boxBuild) != 0 {
		t.Fatal("a retired build kept capture/representative/stage rows")
	}
	if left := readingTerminalHandleRows(t, c, lib); len(left) != 0 {
		t.Fatalf("the removed handle kept %v", left)
	}
	if n := rows(t, c, `SELECT count(*) FROM reading_path_source_barrier WHERE source_id=?`, source); n != 0 {
		t.Fatalf("the removed source kept %d barriers", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM reading_path_block WHERE source_id=?`, source); n != 0 {
		t.Fatalf("the removed source kept %d blocks", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_removal_progress`); n != 0 {
		t.Fatalf("the finished removal left %d progress rows", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM libraries WHERE id='r'`); n != 0 {
		t.Fatal("the removed library row remains")
	}
	var slotBuild, slotOverlay sql.NullInt64
	if err := c.DB.QueryRow(`SELECT build_id,overlay_id FROM reading_path_slot WHERE singleton=1`).Scan(&slotBuild, &slotOverlay); err != nil {
		t.Fatal(err)
	}
	if slotBuild.Valid || slotOverlay.Valid {
		t.Fatalf("the retired scope's slot was not detached: build=%v overlay=%v", slotBuild, slotOverlay)
	}

	// Another library's private lifetime, its counters and the shared clock are
	// not this removal's to touch.
	if got := readingPurgeScopeState(t, c, keepScope); got != keepState {
		t.Fatalf("the sibling scope changed: %s, was %s", got, keepState)
	}
	if got := readingPurgeScopeCounts(t, c, keepScope); got != keepCounts {
		t.Fatalf("the sibling scope lost rows: %+v, was %+v", got, keepCounts)
	}
	if readingPurgeChildRows(t, c, keepBuild) != keepChildren {
		t.Fatal("the sibling build lost a child")
	}
	var after int64
	if err := c.DB.QueryRow(`SELECT sequence FROM reading_path_clock WHERE singleton=1`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != clock {
		t.Fatalf("the Reading clock moved from %d to %d", clock, after)
	}
}

// The singleton slot is global. A retired library's drain must leave another
// live library's slot, barrier and blocks in place.
func TestLibraryRemovalReadingTerminalKeepsAnotherLibrariesSlot(t *testing.T) {
	c := catalogtest.Open(t)
	keepRoot, root := t.TempDir(), t.TempDir()
	keep := c.Library("keep", "Keep", "book", keepRoot)
	lib := c.Library("r", "Reading", "book", root)

	keepBooks := readingPurgeBooks(t, c, keep, keepRoot, "Keep", 1)
	keepScope := readingPurgeScope(t, c, keep, "Keep")
	keepBuild := readingPurgeBuild(t, c, keepScope, "flatten", keepBooks)
	keepState := readingPurgeScopeState(t, c, keepScope)
	keepCounts := readingPurgeScopeCounts(t, c, keepScope)
	keepChildren := readingPurgeChildRows(t, c, keepBuild)

	var keepSource string
	if err := c.DB.QueryRow(`SELECT id FROM library_sources WHERE library_id='keep'`).Scan(&keepSource); err != nil {
		t.Fatal(err)
	}
	c.Exec(`INSERT INTO reading_path_source_barrier(source_id,job_id,incarnation,source_generation,root_identity,root,phase) VALUES(?,?,?,?,?,?,'cancelled')`, keepSource, "keep-job", "inc", 1, "root-id", keepRoot)
	for i := 0; i < 3; i++ {
		c.Exec(`INSERT INTO reading_path_block(job_id,source_id,directory_key,mode,reason) VALUES(?,?,?,?,?)`, "keep-job", keepSource, fmt.Sprintf("keep-block-%d", i), "scope", "invalid_reading_path")
	}

	books := readingPurgeBooks(t, c, lib, root, "Shelf", 1)
	scope := readingPurgeScope(t, c, lib, "Shelf")
	readingPurgeBuild(t, c, scope, "cleanup", books)

	c.Exec(`UPDATE reading_path_slot SET build_id=?1,overlay_id=?1 WHERE singleton=1`, keepBuild)
	c.Exec(`UPDATE catalog_libraries SET retired=1 WHERE id=?`, lib)

	for i := 0; ; i++ {
		if readingTerminalBatch(t, c, "r") {
			break
		}
		if i > 400 {
			t.Fatal("the removal did not finish")
		}
	}

	var slotBuild, slotOverlay sql.NullInt64
	if err := c.DB.QueryRow(`SELECT build_id,overlay_id FROM reading_path_slot WHERE singleton=1`).Scan(&slotBuild, &slotOverlay); err != nil {
		t.Fatal(err)
	}
	if !slotBuild.Valid || slotBuild.Int64 != keepBuild || !slotOverlay.Valid || slotOverlay.Int64 != keepBuild {
		t.Fatalf("another library's slot changed: build=%v overlay=%v, want %d", slotBuild, slotOverlay, keepBuild)
	}
	if got := readingPurgeScopeState(t, c, keepScope); got != keepState {
		t.Fatalf("the sibling scope changed: %s, was %s", got, keepState)
	}
	if got := readingPurgeScopeCounts(t, c, keepScope); got != keepCounts {
		t.Fatalf("the sibling scope lost rows: %+v, was %+v", got, keepCounts)
	}
	if readingPurgeChildRows(t, c, keepBuild) != keepChildren {
		t.Fatal("the sibling build lost a child")
	}
	if n := rows(t, c, `SELECT count(*) FROM reading_path_source_barrier WHERE source_id=?`, keepSource); n != 1 {
		t.Fatalf("the sibling barrier was touched: %d", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM reading_path_block WHERE source_id=?`, keepSource); n != 3 {
		t.Fatalf("the sibling blocks were touched: %d", n)
	}
	if left := readingTerminalHandleRows(t, c, lib); len(left) != 0 {
		t.Fatalf("the removed handle kept %v", left)
	}
}

// A phase-5 cursor that this build cannot resume must restart safely instead of
// blocking the removal: a sub-cursor from an older compiled plan, an unknown
// stage, a missing scope id (which must not be read as terminal) and a cursor
// naming another lifetime all still end in a proved-empty terminal state.
func TestLibraryRemovalReadingTerminalRestartsInterruptedPhaseFive(t *testing.T) {
	type corrupt struct {
		name   string
		cursor func(handle, scope int64) string
	}
	cases := []corrupt{
		{"foreign-plan", func(handle, scope int64) string {
			return fmt.Sprintf(`{"h":%d,"s":"scope","q":%d,"c":"{\"d\":\"old-plan\",\"c\":\"\"}"}`, handle, scope)
		}},
		{"unknown-stage", func(handle, _ int64) string {
			return fmt.Sprintf(`{"h":%d,"s":"phase6","q":0}`, handle)
		}},
		{"missing-scope", func(handle, scope int64) string {
			return fmt.Sprintf(`{"h":%d,"s":"scope","q":%d}`, handle, scope+1000000)
		}},
		{"another-lifetime", func(handle, scope int64) string {
			return fmt.Sprintf(`{"h":%d,"s":"scope","q":%d}`, handle+1000000, scope)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := catalogtest.Open(t)
			root := t.TempDir()
			lib := c.Library("r", "Reading", "book", root)
			books := readingPurgeBooks(t, c, lib, root, "Shelf", 2)
			scope := readingPurgeScope(t, c, lib, "Shelf")
			build := readingPurgeBuild(t, c, scope, "flatten", books)
			c.Exec(`UPDATE reading_path_slot SET build_id=?1,overlay_id=?1 WHERE singleton=1`, build)
			c.Exec(`UPDATE catalog_libraries SET retired=1 WHERE id=?`, lib)

			sawPhaseFive := false
			for i := 0; i < 400; i++ {
				var phase int
				var cursor string
				err := c.DB.QueryRow(`SELECT phase,object_cursor FROM catalog_removal_progress WHERE kind='library' AND target='r'`).Scan(&phase, &cursor)
				if err == nil && phase == 5 && cursor != "" {
					sawPhaseFive = true
					break
				}
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					t.Fatal(err)
				}
				if readingTerminalBatch(t, c, "r") {
					t.Fatal("the removal finished before its phase-5 cursor existed")
				}
			}
			if !sawPhaseFive {
				t.Fatal("phase 5 never stored a resumable cursor")
			}
			c.Exec(`UPDATE catalog_removal_progress SET object_cursor=? WHERE kind='library' AND target='r'`, tc.cursor(lib, scope))

			for i := 0; ; i++ {
				if readingTerminalBatch(t, c, "r") {
					break
				}
				if i > 400 {
					t.Fatal("the restarted removal did not finish")
				}
			}
			if left := readingTerminalHandleRows(t, c, lib); len(left) != 0 {
				t.Fatalf("the restarted removal kept %v", left)
			}
			if n := rows(t, c, `SELECT count(*) FROM reading_path_build WHERE scope_id=?`, scope); n != 0 {
				t.Fatalf("the restarted removal kept %d builds", n)
			}
			if n := rows(t, c, `SELECT count(*) FROM reading_path_slot WHERE singleton=1 AND build_id IS NOT NULL`); n != 0 {
				t.Fatalf("the restarted removal kept %d slot builds", n)
			}
			if n := rows(t, c, `SELECT count(*) FROM catalog_removal_progress`); n != 0 {
				t.Fatalf("the restarted removal left %d progress rows", n)
			}
			if n := rows(t, c, `SELECT count(*) FROM libraries WHERE id='r'`); n != 0 {
				t.Fatal("the restarted removal left the library row")
			}
		})
	}
}
