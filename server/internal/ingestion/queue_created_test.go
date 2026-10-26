package ingestion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/persistence"
)

// queueCreatedFixture builds a movie library with one source per folder, so a
// library-wide enqueue has several children to create or coalesce.
func queueCreatedFixture(t *testing.T, folders int) (*Service, *sql.DB, string, []string) {
	t.Helper()
	root := t.TempDir()
	paths := make([]string, folders)
	for i := range paths {
		paths[i] = filepath.Join(root, fmt.Sprintf("media-%d", i))
		if err := os.Mkdir(paths[i], 0700); err != nil {
			t.Fatal(err)
		}
	}
	db, err := persistence.Open(filepath.Join(root, "catalog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cat := catalog.New(db)
	var library catalog.Library
	if folders == 1 {
		library, err = cat.Create("Movies", "movie", paths[0])
	} else {
		library, err = cat.CreateFoldersAuthorizedThen(context.Background(), "Movies", "movie", paths, nil, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT id FROM library_sources WHERE library_id=? AND enabled=1 ORDER BY CASE WHEN id=library_id THEN 0 ELSE 1 END,id`, library.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != folders {
		t.Fatalf("library has %d sources, want %d", len(ids), folders)
	}
	return New(db, cat, assets.Probe{}), db, library.ID, ids
}

func jobIDs(j Job) []string { return append([]string{}, j.Jobs...) }

// The callback runs in the accepting enqueue transaction, after the job, its run
// and its active-source row exist and before the commit that makes them durable.
func TestQueueModeCreatedReportsTheNewJobInsideTheTransaction(t *testing.T) {
	s, db, library, sources := queueCreatedFixture(t, 1)
	seen := []CreatedScanJob{}
	job, err := s.QueueModeCreated(context.Background(), library, "", "", nil, func(ctx context.Context, tx *sql.Tx, created CreatedScanJob) error {
		seen = append(seen, created)
		var status, source string
		if err := tx.QueryRowContext(ctx, `SELECT j.status,r.source_id FROM jobs j JOIN inventory_runs r ON r.job_id=j.id WHERE j.id=?`, created.JobID).Scan(&status, &source); err != nil {
			return fmt.Errorf("created job not readable in the callback transaction: %w", err)
		}
		if status != "queued" || source != created.SourceID {
			return fmt.Errorf("callback saw job %q of %q", status, source)
		}
		var active string
		if err := tx.QueryRowContext(ctx, `SELECT job_id FROM inventory_source_active WHERE source_id=?`, created.SourceID).Scan(&active); err != nil {
			return fmt.Errorf("created job has no active-source row yet: %w", err)
		}
		if active != created.JobID {
			return fmt.Errorf("active row points at %q, want %q", active, created.JobID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 {
		t.Fatalf("callback ran %d times, want once", len(seen))
	}
	if want := (CreatedScanJob{JobID: job.ID, LibraryID: library, SourceID: sources[0]}); seen[0] != want {
		t.Fatalf("reported %+v, want %+v", seen[0], want)
	}
	if job.Status != "queued" || len(job.Jobs) != 1 || job.Jobs[0] != job.ID {
		t.Fatalf("enqueue answered %+v", job)
	}
	var durable int
	if err := db.QueryRow(`SELECT count(*) FROM jobs WHERE id=?`, job.ID).Scan(&durable); err != nil || durable != 1 {
		t.Fatalf("created job did not commit: %d %v", durable, err)
	}
}

// A job that already exists is returned unchanged and is never reported as new.
func TestQueueModeCreatedNeverReportsACoalescedJob(t *testing.T) {
	s, _, library, sources := queueCreatedFixture(t, 1)
	count := 0
	report := func(context.Context, *sql.Tx, CreatedScanJob) error { count++; return nil }
	first, err := s.QueueModeCreated(context.Background(), library, "", "", nil, report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.QueueModeCreated(context.Background(), library, sources[0], "full", nil, report)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("a second enqueue made a new job %q, want the active %q", second.ID, first.ID)
	}
	if count != 1 {
		t.Fatalf("the coalesced job was reported again: %d reports", count)
	}
	// The legacy wrapper coalesces onto the same job and reports nothing.
	legacy, err := s.QueueMode(context.Background(), library, "", "", nil)
	if err != nil || legacy.ID != first.ID || legacy.Status != first.Status {
		t.Fatalf("legacy enqueue answered %+v %v, want %q", legacy, err, first.ID)
	}
	if count != 1 {
		t.Fatalf("legacy enqueue reached the callback: %d reports", count)
	}
}

// A library fan-out reports one created job per source it actually creates.
func TestQueueModeCreatedReportsEachNewSourceInALibraryFanout(t *testing.T) {
	s, _, library, sources := queueCreatedFixture(t, 2)
	seen := []CreatedScanJob{}
	job, err := s.QueueModeCreated(context.Background(), library, "", "", nil, func(_ context.Context, _ *sql.Tx, created CreatedScanJob) error {
		seen = append(seen, created)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(sources) {
		t.Fatalf("reported %d created jobs, want %d: %+v", len(seen), len(sources), seen)
	}
	bySource := map[string]string{}
	for _, created := range seen {
		if created.LibraryID != library {
			t.Fatalf("reported library %q, want %q", created.LibraryID, library)
		}
		bySource[created.SourceID] = created.JobID
	}
	reported := map[string]bool{}
	for _, id := range jobIDs(job) {
		reported[id] = true
	}
	for _, source := range sources {
		id := bySource[source]
		if id == "" {
			t.Fatalf("source %q was created but not reported: %+v", source, seen)
		}
		if !reported[id] {
			t.Fatalf("reported job %q is missing from the enqueue answer %v", id, jobIDs(job))
		}
	}
}

// Mixed fan-out: sources that already have an active job are reused and stay
// unreported; only the genuinely new child is reported.
func TestQueueModeCreatedReportsOnlyNewSourcesWhenMixed(t *testing.T) {
	s, _, library, sources := queueCreatedFixture(t, 2)
	active, err := s.QueueContext(context.Background(), library, sources[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := []CreatedScanJob{}
	job, err := s.QueueModeCreated(context.Background(), library, "", "", nil, func(_ context.Context, _ *sql.Tx, created CreatedScanJob) error {
		seen = append(seen, created)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0].SourceID != sources[1] {
		t.Fatalf("reported %+v, want only the new job on %q", seen, sources[1])
	}
	ids := jobIDs(job)
	if len(ids) != 2 {
		t.Fatalf("fan-out answered %v, want the reused and the new job", ids)
	}
	found := map[string]bool{ids[0]: true, ids[1]: true}
	if !found[active.ID] || !found[seen[0].JobID] {
		t.Fatalf("fan-out answered %v, want %q and %q", ids, active.ID, seen[0].JobID)
	}
}

// A callback refusal aborts the whole enqueue transaction: every child created by
// the fan-out is rolled back, not only the one that refused.
func TestQueueModeCreatedRollsBackEveryChildWhenTheCallbackRefuses(t *testing.T) {
	s, db, library, _ := queueCreatedFixture(t, 2)
	refusal := errors.New("private admission refused")
	calls := 0
	_, err := s.QueueModeCreated(context.Background(), library, "", "", nil, func(context.Context, *sql.Tx, CreatedScanJob) error {
		calls++
		if calls == 2 {
			return refusal
		}
		return nil
	})
	if !errors.Is(err, refusal) {
		t.Fatalf("enqueue answered %v, want the callback's refusal", err)
	}
	if calls != 2 {
		t.Fatalf("callback ran %d times, want the refusing second child", calls)
	}
	for _, query := range []string{
		`SELECT count(*) FROM jobs WHERE library_id=?`,
		`SELECT count(*) FROM inventory_runs r JOIN library_sources s ON s.id=r.source_id WHERE s.library_id=?`,
		`SELECT count(*) FROM inventory_source_active a JOIN library_sources s ON s.id=a.source_id WHERE s.library_id=?`,
	} {
		var n int
		if err = db.QueryRow(query, library).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%q left %d rows after a refused enqueue", query, n)
		}
	}
}

// The seam adds no field to the answered job, and a nil callback is exactly the
// legacy enqueue.
func TestQueueModeCreatedKeepsTheLegacyJobShape(t *testing.T) {
	typ := reflect.TypeOf(Job{})
	for i := 0; i < typ.NumField(); i++ {
		if name := typ.Field(i).Name; name == "Created" || name == "CreatedJobs" || name == "CreatedScanJobs" {
			t.Fatalf("Job gained a created field: %s", name)
		}
	}
	s, _, library, _ := queueCreatedFixture(t, 1)
	created, err := s.QueueModeCreated(context.Background(), library, "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "queued" || created.ScanMode != "first-import" || len(created.Jobs) != 1 || created.Jobs[0] != created.ID {
		t.Fatalf("nil-callback enqueue answered %+v", created)
	}
	raw, err := json.Marshal(created)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "created") {
		t.Fatalf("enqueue answer gained a wire field: %s", raw)
	}
	legacy, err := s.QueueMode(context.Background(), library, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.ID != created.ID || legacy.Status != created.Status || legacy.ScanMode != created.ScanMode || strings.Join(jobIDs(legacy), ",") != strings.Join(jobIDs(created), ",") {
		t.Fatalf("legacy enqueue answered %+v, want the same job as %+v", legacy, created)
	}
}
