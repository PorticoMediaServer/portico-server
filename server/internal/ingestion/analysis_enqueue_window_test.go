package ingestion

// Regression family for the analysis enqueue page's raw source window. The page
// still performs one writer transaction, the same fence/policy/STRM/sidecar
// sequencing, the unchanged needs predicate and the unchanged queue conflict
// clause. Only the window and the cursor arithmetic changed: the page consumes
// a bounded prefix of the source's raw (source_id, retired, id) index, then
// evaluates the predicate over those ids. These tests drive that real enqueue
// path, not a copy of it.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/mediaanalysis"
	"portico.local/server/internal/sonic"
)

// objectIDs returns the ids the raw window is ordered by: the index's
// (source_id, retired, id) prefix with retired rows excluded.
func objectIDs(t *testing.T, db *sql.DB, source string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM inventory_objects WHERE source_id=? AND retired=0 ORDER BY id`, source)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func (f *loopFixture) queued(job string) []string {
	f.t.Helper()
	rows, err := f.db.Query(`SELECT object_id FROM inventory_analysis_queue WHERE job_id=? ORDER BY object_id`, job)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			f.t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		f.t.Fatal(err)
	}
	return ids
}

func (f *loopFixture) runState(job string) (string, string) {
	f.t.Helper()
	var phase, cursor string
	if err := f.db.QueryRow(`SELECT phase,reconcile_cursor FROM inventory_runs WHERE job_id=?`, job).Scan(&phase, &cursor); err != nil {
		f.t.Fatal(err)
	}
	return phase, cursor
}

func (f *loopFixture) librarySource() catalog.LibrarySource {
	f.t.Helper()
	source, err := f.s.catalog.LibrarySource(context.Background(), f.source)
	if err != nil {
		f.t.Fatal(err)
	}
	return source
}

// analysisOnlyJob starts a run already at the analysis-enqueue phase. The
// producer paths that do this in production (PolicyChanged, queueSourceTx,
// phase(reset)) are unchanged; forcing the phase keeps each test focused on the
// page's window and cursor contract.
func (f *loopFixture) analysisOnlyJob() Job {
	f.t.Helper()
	job := f.queue()
	f.exec(`UPDATE inventory_runs SET phase='analysis_enqueue',reconcile_cursor='' WHERE job_id=?`, job.ID)
	f.exec(`UPDATE jobs SET status='running' WHERE id=?`, job.ID)
	state, err := f.s.Get(job.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return state
}

// runEnqueue drives the real page until it reports the queue analysed. It
// returns the phase and cursor after each page.
func (f *loopFixture) runEnqueue(t *testing.T, job Job, maxPages int) ([]string, []string) {
	t.Helper()
	source := f.librarySource()
	phases := []string{}
	cursors := []string{}
	for n := 0; n < maxPages; n++ {
		if err := f.s.enqueueAnalysisPage(context.Background(), job.ID, source); err != nil {
			t.Fatalf("enqueue page %d: %v", n+1, err)
		}
		phase, cursor := f.runState(job.ID)
		phases = append(phases, phase)
		cursors = append(cursors, cursor)
		if phase == "basic_pending" {
			return phases, cursors
		}
	}
	t.Fatalf("enqueue did not reach basic_pending in %d pages", maxPages)
	return nil, nil
}

// markObjectsSettled makes every object's analysis agree with its current
// revision, policy, generation and the named completed operations.
func (f *loopFixture) markObjectsSettled(operations []string) {
	f.t.Helper()
	done, _ := json.Marshal(operations)
	f.exec(`UPDATE inventory_objects SET analysis_revision=revision,analysis_policy=(SELECT revision FROM library_scan_policies WHERE library_id=?),analysis_operations_json=?,analysis_algorithm=?,analysis_state='complete',analysis_generation=(SELECT generation FROM library_sources WHERE id=source_id)`, f.library, string(done), mediaanalysis.Algorithm)
}

// newSonicCapableAnalysis gives the enqueue page a service whose
// SonicCanAttempt is true without loading a model or running inference. The
// objects in the sonic tests need work, so the page must select the 32-row
// fallback budget but never call SonicCurrentTx.
func newSonicCapableAnalysis(t *testing.T, db *sql.DB) *mediaanalysis.Service {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := mediaanalysis.New(mediaanalysis.Options{
		Sonic:     &sonic.Runtime{},
		DB:        db,
		Directory: root,
		Open: func(context.Context, string, string) (mediaanalysis.Input, error) {
			t.Fatal("enqueue opened media")
			return nil, nil
		},
		Decoder: func(mediaanalysis.Input) mediaanalysis.Decode {
			return func(context.Context, string, decoder.AnalysisSpec, func(io.Reader) error) error {
				t.Fatal("enqueue decoded media")
				return nil
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	if !service.SonicCanAttempt() {
		t.Fatal("sonic-capable service is not selectable")
	}
	return service
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A long prefix of rows that the predicate rejects must be consumed by bounded
// raw windows. The phase may not end early, the cursor must advance past every
// rejected row, and the same predicate must run once per window rather than
// once per row.
func TestAnalysisEnqueueRawWindowConsumesRejectedPrefix(t *testing.T) {
	files := make([]string, 200)
	for i := range files {
		files[i] = fmt.Sprintf("Film %03d (2000).mkv", i)
	}
	f := newLoopFixture(t, "movie", files...)
	f.finish(f.queue())
	f.exec(`UPDATE library_scan_policies SET tier='basic',operations_json='["probe","local_metadata"]' WHERE library_id=?`, f.library)
	ids := objectIDs(t, f.db, f.source)
	if len(ids) != len(files) {
		t.Fatalf("objects=%d, want %d", len(ids), len(files))
	}
	for i, id := range ids[:150] {
		switch i % 3 {
		case 0:
			f.exec(`UPDATE inventory_objects SET state='missing' WHERE id=?`, id)
		case 1:
			f.exec(`UPDATE inventory_objects SET root_incarnation='stale-incarnation' WHERE id=?`, id)
		default:
			f.exec(`UPDATE inventory_objects SET unsupported_reason='unsupported_disc_layout' WHERE id=?`, id)
		}
	}
	job := f.analysisOnlyJob()

	dbwork.ProfileStatements(true)
	defer dbwork.ProfileStatements(false)
	phases, cursors := f.runEnqueue(t, job, 10)
	if len(phases) != 4 {
		t.Fatalf("pages=%d phases=%v", len(phases), phases)
	}
	for page := 0; page < 3; page++ {
		if phases[page] != "analysis_enqueue" {
			t.Fatalf("page %d ended the phase early: %s", page+1, phases[page])
		}
	}
	if phases[3] != "basic_pending" {
		t.Fatalf("final phase=%s", phases[3])
	}
	wantCursors := []string{ids[63], ids[127], ids[191], ""}
	if !equalIDs(cursors, wantCursors) {
		t.Fatalf("cursors=%v, want %v", cursors, wantCursors)
	}
	want := ids[150:]
	got := f.queued(job.ID)
	if !equalIDs(got, want) {
		t.Fatalf("queued=%v, want exactly the eligible tail %v", got, want)
	}
	var raw, needs int64
	for _, entry := range dbwork.StatementProfile() {
		switch {
		case strings.HasPrefix(entry.Shape, "SELECT o.id,o.revision FROM inventory_objects o INDEXED BY inventory_objects_source"):
			raw += entry.Count
		case strings.HasPrefix(entry.Shape, "WITH batch AS MATERIALIZED ( SELECT o.* FROM inventory_objects o JOIN inventory_runs r ON"):
			needs += entry.Count
		}
	}
	if raw != 4 || needs != 4 {
		t.Fatalf("raw-window statements=%d needs statements=%d, want one of each per page (4/4)", raw, needs)
	}
}

// The raw window's boundary cases: an empty source, exactly one page, and one
// row past one page. Exactly one page is raw EOF in the same commit; a probe row
// is re-read rather than skipped.
func TestAnalysisEnqueueRawWindowBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		files int
		pages int
	}{
		{"empty", 0, 1},
		{"exactly one raw page", 64, 1},
		{"one row past one raw page", 65, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := make([]string, tc.files)
			for i := range files {
				files[i] = fmt.Sprintf("Film %03d (2000).mkv", i)
			}
			f := newLoopFixture(t, "movie", files...)
			f.finish(f.queue())
			f.exec(`UPDATE library_scan_policies SET tier='basic',operations_json='["probe","local_metadata"]' WHERE library_id=?`, f.library)
			ids := objectIDs(t, f.db, f.source)
			job := f.analysisOnlyJob()
			phases, cursors := f.runEnqueue(t, job, 5)
			if len(phases) != tc.pages {
				t.Fatalf("pages=%d phases=%v, want %d", len(phases), phases, tc.pages)
			}
			if phases[len(phases)-1] != "basic_pending" {
				t.Fatalf("did not settle: %v", phases)
			}
			if got := f.queued(job.ID); len(got) != tc.files {
				t.Fatalf("queued=%d, want %d", len(got), tc.files)
			}
			switch tc.files {
			case 0:
				if cursors[0] != "" {
					t.Fatalf("empty source cursor=%q", cursors[0])
				}
			case 64:
				// Exactly one page with no probe row is EOF in the same commit.
				if cursors[0] != "" {
					t.Fatalf("one-page EOF cursor=%q, want empty", cursors[0])
				}
			case 65:
				// The first page consumes 64 raw ids and leaves the probe for the next.
				if cursors[0] != ids[63] || phases[0] != "analysis_enqueue" {
					t.Fatalf("first page cursor=%q phase=%s", cursors[0], phases[0])
				}
			}
		})
	}
}

// Queue rows, cursor and phase are one commit. An injected failure at the
// cursor update must leave neither a queue row nor a moved cursor behind.
func TestAnalysisEnqueueRawWindowKeepsQueueCursorAndPhaseAtomic(t *testing.T) {
	f := newLoopFixture(t, "movie", "One (2000).mkv", "Two (2001).mkv", "Three (2002).mkv", "Four (2003).mkv", "Five (2004).mkv")
	f.finish(f.queue())
	f.exec(`UPDATE library_scan_policies SET tier='basic',operations_json='["probe","local_metadata"]' WHERE library_id=?`, f.library)
	job := f.analysisOnlyJob()
	source := f.librarySource()

	// ABORT rolls the statement back; the page's own rollback must group the
	// already-executed queue inserts and the cursor move together.
	f.exec(`CREATE TRIGGER analysis_enqueue_cursor_failure BEFORE UPDATE OF reconcile_cursor ON inventory_runs BEGIN SELECT RAISE(ABORT,'injected cursor failure'); END`)
	if err := f.s.enqueueAnalysisPage(context.Background(), job.ID, source); err == nil {
		t.Fatal("injected cursor failure was not surfaced")
	}
	if got := f.queued(job.ID); len(got) != 0 {
		t.Fatalf("queue rows survived rollback: %v", got)
	}
	if phase, cursor := f.runState(job.ID); phase != "analysis_enqueue" || cursor != "" {
		t.Fatalf("phase=%s cursor=%q survived rollback", phase, cursor)
	}
	f.exec(`DROP TRIGGER analysis_enqueue_cursor_failure`)

	if err := f.s.enqueueAnalysisPage(context.Background(), job.ID, source); err != nil {
		t.Fatal(err)
	}
	if got := f.queued(job.ID); len(got) != 5 {
		t.Fatalf("after rollback queued=%d, want 5", len(got))
	}
	if phase, _ := f.runState(job.ID); phase != "basic_pending" {
		t.Fatalf("after rollback phase=%s", phase)
	}
}

// The unchanged needs predicate still selects exactly the rows a settled
// object, a policy change, a generation change, a pending state, an algorithm
// change, a missing deep head or a changed STRM target requires. Rows filtered
// by state, incarnation or unsupported reason stay out, and a reading-format
// row keeps the existing algorithm exemption.
func TestAnalysisEnqueueRawWindowNeedsParity(t *testing.T) {
	t.Run("state policy generation and unsupported filters", func(t *testing.T) {
		f := newLoopFixture(t, "movie", "Zero (2000).mkv", "One (2001).mkv", "Two (2002).mkv", "Three (2003).mkv", "Four (2004).mkv", "Five (2005).mkv", "Six (2006).mkv", "Seven (2007).mkv")
		f.finish(f.queue())
		f.exec(`UPDATE library_scan_policies SET tier='basic',operations_json='["probe","local_metadata"]' WHERE library_id=?`, f.library)
		ids := objectIDs(t, f.db, f.source)
		f.markObjectsSettled([]string{"probe", "local_metadata", assets.PlaybackAnalysisRevision})
		f.exec(`UPDATE inventory_objects SET analysis_revision='stale-revision' WHERE id=?`, ids[1])
		f.exec(`UPDATE inventory_objects SET analysis_policy=(SELECT revision-1 FROM library_scan_policies WHERE library_id=?) WHERE id=?`, f.library, ids[2])
		f.exec(`UPDATE inventory_objects SET analysis_generation=(SELECT generation-1 FROM library_sources WHERE id=?) WHERE id=?`, f.source, ids[3])
		f.exec(`UPDATE inventory_objects SET analysis_state='pending' WHERE id=?`, ids[4])
		f.exec(`UPDATE inventory_objects SET analysis_algorithm='old-algorithm' WHERE id=?`, ids[5])
		f.exec(`UPDATE inventory_objects SET state='missing' WHERE id=?`, ids[6])
		f.exec(`UPDATE inventory_objects SET root_incarnation='stale-incarnation' WHERE id=?`, ids[7])
		job := f.analysisOnlyJob()
		f.runEnqueue(t, job, 3)
		want := []string{ids[1], ids[2], ids[3], ids[4]}
		sort.Strings(want)
		if got := f.queued(job.ID); !equalIDs(got, want) {
			t.Fatalf("queued=%v, want %v", got, want)
		}
	})

	t.Run("algorithm and deep head", func(t *testing.T) {
		f := newLoopFixture(t, "movie", "Zero (2000).mkv", "One (2001).mkv", "Two (2002).mkv")
		f.finish(f.queue())
		f.exec(`UPDATE library_scan_policies SET tier='custom',operations_json='["probe","checksum"]' WHERE library_id=?`, f.library)
		ids := objectIDs(t, f.db, f.source)
		f.markObjectsSettled([]string{"probe", "checksum", assets.PlaybackAnalysisRevision})
		f.exec(`INSERT INTO analysis_results(id,object_id,asset_id,source_revision,root_incarnation,configuration_generation,policy_revision,stage,algorithm,evidence,tool_digest,duration_us,summary_json,created_ms) SELECT 'head:'||id,id,asset_id,revision,root_incarnation,(SELECT generation FROM library_sources WHERE id=source_id),1,'checksum',?,'e','digest',1,'{}',1 FROM inventory_objects`, mediaanalysis.Algorithm)
		f.exec(`INSERT INTO analysis_heads(object_id,source_revision,stage,result_id) SELECT id,revision,'checksum','head:'||id FROM inventory_objects`)
		f.exec(`DELETE FROM analysis_heads WHERE object_id=?`, ids[1])
		f.exec(`UPDATE inventory_objects SET analysis_algorithm='old-algorithm' WHERE id=?`, ids[2])
		job := f.analysisOnlyJob()
		f.runEnqueue(t, job, 3)
		want := []string{ids[1], ids[2]}
		sort.Strings(want)
		if got := f.queued(job.ID); !equalIDs(got, want) {
			t.Fatalf("queued=%v, want the missing head and the old algorithm %v", got, want)
		}
	})

	t.Run("strm target", func(t *testing.T) {
		f := newLoopFixture(t, "movie", "One (2000).strm", "Two (2001).strm")
		f.finish(f.queue())
		f.exec(`UPDATE library_scan_policies SET tier='basic',operations_json='["probe","local_metadata"]' WHERE library_id=?`, f.library)
		f.exec(`INSERT INTO source_strm_analysis(library_id,enabled,revision) VALUES(?,1,7) ON CONFLICT(library_id) DO UPDATE SET enabled=1,revision=7`, f.library)
		ids := objectIDs(t, f.db, f.source)
		f.markObjectsSettled([]string{"probe", "local_metadata", assets.PlaybackAnalysisRevision, "strm_target:7"})
		f.exec(`UPDATE inventory_objects SET analysis_operations_json=(SELECT json_group_array(value) FROM json_each(analysis_operations_json) WHERE value<>'strm_target:7') WHERE id=?`, ids[1])
		job := f.analysisOnlyJob()
		f.runEnqueue(t, job, 3)
		want := []string{ids[1]}
		if got := f.queued(job.ID); !equalIDs(got, want) {
			t.Fatalf("queued=%v, want only the object whose STRM target moved %v", got, want)
		}
	})

	t.Run("reading formats stay out of media analysis", func(t *testing.T) {
		f := newLoopFixture(t, "book", "Zero.epub", "One.epub")
		f.finish(f.queue())
		f.exec(`UPDATE library_scan_policies SET tier='custom',operations_json='["probe","checksum"]' WHERE library_id=?`, f.library)
		ids := objectIDs(t, f.db, f.source)
		f.markObjectsSettled([]string{"probe", "checksum", assets.PlaybackAnalysisRevision})
		f.exec(`INSERT INTO analysis_results(id,object_id,asset_id,source_revision,root_incarnation,configuration_generation,policy_revision,stage,algorithm,evidence,tool_digest,duration_us,summary_json,created_ms) SELECT 'head:'||id,id,asset_id,revision,root_incarnation,(SELECT generation FROM library_sources WHERE id=source_id),1,'checksum',?,'e','digest',1,'{}',1 FROM inventory_objects`, mediaanalysis.Algorithm)
		f.exec(`INSERT INTO analysis_heads(object_id,source_revision,stage,result_id) SELECT id,revision,'checksum','head:'||id FROM inventory_objects`)
		// A reading format is outside this media-analysis predicate entirely: an
		// old algorithm and a missing deep head still queue nothing, and the raw
		// window still consumes both rows and settles the phase.
		f.exec(`UPDATE inventory_objects SET analysis_algorithm='old-algorithm' WHERE id=?`, ids[0])
		f.exec(`UPDATE inventory_objects SET analysis_state='pending',analysis_revision='' WHERE id=?`, ids[1])
		job := f.analysisOnlyJob()
		phases, _ := f.runEnqueue(t, job, 3)
		if phases[len(phases)-1] != "basic_pending" {
			t.Fatalf("reading rows did not settle the phase: %v", phases)
		}
		if got := f.queued(job.ID); len(got) != 0 {
			t.Fatalf("reading formats entered media analysis: %v", got)
		}
	})
}

// The raw budget is 64 rows normally and 32 when a sonic fallback can run.
// SonicCurrentTx is never called here: the objects already need analysis, so
// the page must select the smaller window from policy + SonicCanAttempt alone.
// A filtered raw row is still consumed but never currentness-checked or queued.
func TestAnalysisEnqueueSonicWindowSelection(t *testing.T) {
	t.Run("non-sonic uses the raw window", func(t *testing.T) {
		files := make([]string, 40)
		for i := range files {
			files[i] = fmt.Sprintf("Film %03d (2000).mkv", i)
		}
		f := newLoopFixture(t, "movie", files...)
		f.finish(f.queue())
		f.exec(`UPDATE library_scan_policies SET tier='basic',operations_json='["probe","local_metadata"]' WHERE library_id=?`, f.library)
		job := f.analysisOnlyJob()
		phases, _ := f.runEnqueue(t, job, 3)
		if len(phases) != 1 || phases[0] != "basic_pending" {
			t.Fatalf("phases=%v, want one settled page", phases)
		}
		if got := f.queued(job.ID); len(got) != 40 {
			t.Fatalf("queued=%d, want 40", len(got))
		}
	})

	t.Run("sonic fallback keeps 32 and skips filtered rows", func(t *testing.T) {
		files := make([]string, 41)
		for i := range files {
			files[i] = fmt.Sprintf("Film %03d (2000).mkv", i)
		}
		f := newLoopFixture(t, "movie", files...)
		f.finish(f.queue())
		f.exec(`UPDATE library_scan_policies SET tier='custom',operations_json='["probe","sonic"]' WHERE library_id=?`, f.library)
		ids := objectIDs(t, f.db, f.source)
		// Put a filtered row at the head of the raw window. A wrongly widened
		// sonic page, or a currentness check over filtered rows, would show up as
		// a cursor jump or a queue row for it.
		f.exec(`UPDATE inventory_objects SET state='missing' WHERE id=?`, ids[0])
		f.s.DeepAnalysis = newSonicCapableAnalysis(t, f.db)
		job := f.analysisOnlyJob()
		phases, cursors := f.runEnqueue(t, job, 5)
		if len(phases) != 2 {
			t.Fatalf("pages=%d phases=%v, want two 32-row pages", len(phases), phases)
		}
		if phases[0] != "analysis_enqueue" || cursors[0] != ids[31] {
			t.Fatalf("first page phase=%s cursor=%q, want 32 raw ids consumed", phases[0], cursors[0])
		}
		if phases[1] != "basic_pending" {
			t.Fatalf("second page phase=%s", phases[1])
		}
		got := f.queued(job.ID)
		if len(got) != 40 {
			t.Fatalf("queued=%d, want 40 eligible rows", len(got))
		}
		for _, id := range got {
			if id == ids[0] {
				t.Fatal("a filtered row entered the queue")
			}
		}
	})
}

// The page's existing refusal and sidecar sequencing are unchanged: a fenced
// job writes nothing, a pending sidecar change runs first and keeps the main
// cursor still, and a file-list-only policy still takes its own finish path.
func TestAnalysisEnqueueSidecarFenceAndFileListOnlyPaths(t *testing.T) {
	t.Run("fence and sidecar sequencing", func(t *testing.T) {
		f := newLoopFixture(t, "movie", "One (2000).mkv", "Two (2001).mkv", "Three (2002).mkv", "Four (2003).mkv", "Five (2004).mkv")
		f.finish(f.queue())
		f.exec(`UPDATE library_scan_policies SET tier='basic',operations_json='["probe","local_metadata"]' WHERE library_id=?`, f.library)
		job := f.analysisOnlyJob()
		source := f.librarySource()

		f.exec(`UPDATE jobs SET status='paused' WHERE id=?`, job.ID)
		if err := f.s.enqueueAnalysisPage(context.Background(), job.ID, source); !errors.Is(err, context.Canceled) {
			t.Fatalf("fenced page error=%v, want cancellation", err)
		}
		if got := f.queued(job.ID); len(got) != 0 {
			t.Fatalf("fenced page queued %v", got)
		}
		if _, cursor := f.runState(job.ID); cursor != "" {
			t.Fatalf("fenced page moved the cursor to %q", cursor)
		}
		f.exec(`UPDATE jobs SET status='running' WHERE id=?`, job.ID)

		f.exec(`INSERT INTO inventory_sidecar_changes(job_id,relative_path) VALUES(?,?)`, job.ID, ".")
		if err := f.s.enqueueAnalysisPage(context.Background(), job.ID, source); err != nil {
			t.Fatal(err)
		}
		if got := f.queued(job.ID); len(got) != 0 {
			t.Fatalf("analysis page ran ahead of sidecar changes: %v", got)
		}
		if _, cursor := f.runState(job.ID); cursor != "" {
			t.Fatalf("sidecar page moved the analysis cursor to %q", cursor)
		}
		if n := f.count(`SELECT count(*) FROM inventory_sidecar_changes WHERE job_id=?`, job.ID); n != 1 {
			t.Fatalf("sidecar change row after its first page=%d", n)
		}
		if err := f.s.enqueueAnalysisPage(context.Background(), job.ID, source); err != nil {
			t.Fatal(err)
		}
		if n := f.count(`SELECT count(*) FROM inventory_sidecar_changes WHERE job_id=?`, job.ID); n != 0 {
			t.Fatalf("sidecar change row after its empty tail=%d", n)
		}
		if err := f.s.enqueueAnalysisPage(context.Background(), job.ID, source); err != nil {
			t.Fatal(err)
		}
		if got := f.queued(job.ID); len(got) != 5 {
			t.Fatalf("analysis page after sidecars queued=%d, want 5", len(got))
		}
		if phase, _ := f.runState(job.ID); phase != "basic_pending" {
			t.Fatalf("analysis page after sidecars phase=%s", phase)
		}
	})

	t.Run("file list only still finishes without analysis work", func(t *testing.T) {
		f := newLoopFixture(t, "movie", "One (2000).mkv")
		f.finish(f.queue())
		job := f.analysisOnlyJob()
		if err := f.s.enqueueAnalysisPage(context.Background(), job.ID, f.librarySource()); err != nil {
			t.Fatal(err)
		}
		if got := f.queued(job.ID); len(got) != 0 {
			t.Fatalf("file-list-only queued %v", got)
		}
		state, err := f.s.Get(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Status != "complete" {
			t.Fatalf("file-list-only status=%s", state.Status)
		}
	})
}
