package ingestion

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestScanHistoryPruningIsBoundedAndPreservesActiveRuns(t *testing.T) {
	f := newLoopFixture(t, "movie", "Film (2000).mkv")
	f.finish(f.queue())
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	for n := 0; n < 30; n++ {
		id := fmt.Sprintf("old-%03d", n)
		f.exec(`INSERT INTO jobs(id,library_id,status,created_at) VALUES(?,?,'complete',?)`, id, f.library, old)
		f.exec(`INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision,phase,finished_at) SELECT ?,id,generation,incarnation,root_identity,1,'complete',? FROM library_sources WHERE id=?`, id, old, f.source)
		f.exec(`INSERT INTO inventory_directories(job_id,relative_path,state) VALUES(?,'old','done')`, id)
		f.exec(`INSERT INTO inventory_analysis_queue(job_id,object_id,revision,policy_revision,state) SELECT ?,id,revision,1,'done' FROM inventory_objects LIMIT 1`, id)
	}
	f.exec(`WITH RECURSIVE n(x) AS(SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<600) INSERT INTO inventory_directories(job_id,relative_path,state) SELECT 'old-000','many-'||x,'done' FROM n`)
	active := f.queue()
	before := f.count(`SELECT count(*) FROM inventory_directories`)
	more, err := f.s.PruneSource(context.Background(), f.source)
	if err != nil || !more {
		t.Fatalf("prune: %v %v", more, err)
	}
	after := f.count(`SELECT count(*) FROM inventory_directories`)
	if before-after > scanRetentionBatch {
		t.Fatal("one prune held the writer for too many rows")
	}
	for n := 0; n < 100; n++ {
		more, err = f.s.PruneSource(context.Background(), f.source)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			break
		}
	}
	if n := f.count(`SELECT count(*) FROM jobs WHERE status='complete' AND id LIKE 'old-%'`); n > retainedScansPerSource {
		t.Fatalf("retained old scans: %d", n)
	}
	if n := f.count(`SELECT count(*) FROM inventory_directories WHERE job_id=?`, active.ID); n != 1 {
		t.Fatal("active checkpoints pruned")
	}
	if n := f.count(`SELECT count(*) FROM jobs WHERE id=?`, active.ID); n != 1 {
		t.Fatal("active scan removed")
	}
}
func TestChangeFeedPruningExposesExpiredCursor(t *testing.T) {
	f := newLoopFixture(t, "movie", "Film (2000).mkv")
	job := f.queue()
	f.finish(job)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	f.exec(`WITH RECURSIVE n(x) AS(SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<10500) INSERT INTO inventory_changes(source_id,job_id,object_id,kind,created_at) SELECT ?,?,'object-'||x,'observed',? FROM n`, f.source, job.ID, now)
	for n := 0; n < 10; n++ {
		more, err := f.s.PruneSource(context.Background(), f.source)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			break
		}
	}
	if n := f.count(`SELECT count(*) FROM inventory_changes WHERE source_id=?`, f.source); n != retainedChangesPerSource {
		t.Fatalf("change count: %d", n)
	}
	page, err := f.s.catalog.InventoryChangesPage(context.Background(), f.library, f.source, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !page.ResetRequired || page.OldestSequence <= 1 || len(page.Items) != 128 || !page.HasMore {
		t.Fatalf("expired page: %+v", page)
	}
}
func TestMonitorSettingDisablesOnlyAutomaticWatching(t *testing.T) {
	f := newLoopFixture(t, "movie", "Film (2000).mkv")
	sources, _, err := f.s.monitorConfiguration(context.Background())
	if err != nil || len(sources) != 1 {
		t.Fatalf("default monitor: %v %v", sources, err)
	}
	f.exec(`INSERT INTO admin_documents(scope,revision,body,updated_ms) VALUES(?,1,'{"monitorChanges":false}',1)`, "library:"+f.library)
	sources, _, err = f.s.monitorConfiguration(context.Background())
	if err != nil || len(sources) != 0 {
		t.Fatalf("disabled monitor: %v %v", sources, err)
	}
	f.finish(f.queue())
}

func TestManagedListingRetentionKeepsOneCurrentSnapshotAndLiveWork(t *testing.T) {
	f := newLoopFixture(t, "movie", "Film (2000).mkv")
	f.finish(f.queue())
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	stamp := time.Now().Add(-48 * time.Hour).Unix()
	f.exec(`INSERT INTO jobs(id,library_id,status,created_at) VALUES('remote-old',?,'complete',?)`, f.library, old)
	f.exec(`INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision,phase,finished_at) SELECT 'remote-old',id,generation,incarnation,root_identity,1,'complete',? FROM library_sources WHERE id=?`, old, f.source)
	for _, v := range []struct {
		id, job, generation string
		stamp               int64
	}{{"old-list", "remote-old", "old-generation", stamp}, {"latest-list", "retained-summary", "new-generation", stamp + 1}, {"live-list", "paused-job", "new-generation", stamp + 2}} {
		f.exec(`INSERT INTO remote_listing_sessions(id,job_id,source_id,generation,directory,ready,created_at) VALUES(?,?, 'adapter',?,?,1,?)`, v.id, v.job, v.generation, f.root, v.stamp)
	}
	f.exec(`INSERT INTO jobs(id,library_id,status,created_at) VALUES('paused-job',?,'paused',?)`, f.library, old)
	f.exec(`WITH RECURSIVE n(x) AS(SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<600) INSERT INTO remote_listing_entries(listing_id,name,snapshot) SELECT 'old-list','entry-'||x,'{}' FROM n`)
	// A paused job's latest snapshot must survive even with no run summary.
	f.exec(`INSERT INTO remote_listing_entries(listing_id,name,snapshot) VALUES('live-list','live','{}')`)
	before := f.count(`SELECT count(*) FROM remote_listing_entries`)
	for n := 0; n < 30; n++ {
		more, err := f.s.PruneSource(context.Background(), f.source)
		if err != nil {
			t.Fatal(err)
		}
		after := f.count(`SELECT count(*) FROM remote_listing_entries`)
		if before-after > scanRetentionBatch {
			t.Fatal("unbounded remote listing prune")
		}
		before = after
		if !more {
			break
		}
	}
	if f.count(`SELECT count(*) FROM remote_listing_sessions WHERE id IN('old-list','latest-list')`) != 0 {
		t.Fatal("superseded generations/listings retained")
	}
	if f.count(`SELECT count(*) FROM remote_listing_sessions WHERE id='live-list'`) != 1 || f.count(`SELECT count(*) FROM remote_listing_entries WHERE listing_id='live-list'`) != 1 {
		t.Fatal("paused listing pruned")
	}
	// Once its job is gone, the latest ready snapshot remains the current parent
	// ignore configuration until the directory cache retires it.
	f.exec(`DELETE FROM jobs WHERE id='paused-job'`)
	_, err := f.s.PruneSource(context.Background(), f.source)
	if err != nil {
		t.Fatal(err)
	}
	if f.count(`SELECT count(*) FROM remote_listing_sessions WHERE id='live-list'`) != 1 {
		t.Fatal("current snapshot lost")
	}
	f.exec(`DELETE FROM inventory_directory_cache WHERE source_id=?`, f.source)
	for n := 0; n < 4; n++ {
		if _, err = f.s.PruneSource(context.Background(), f.source); err != nil {
			t.Fatal(err)
		}
	}
	if f.count(`SELECT count(*) FROM remote_listing_sessions WHERE id='live-list'`) != 0 {
		t.Fatal("retired directory listing kept")
	}
}
