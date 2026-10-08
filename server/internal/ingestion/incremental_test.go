package ingestion

import (
	"context"
	"os"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/persistence"
	"testing"
	"time"
)

func TestIncrementalScanSkipsUnchangedDirectoriesAndIntegrityFindsReplacements(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(root, "media")
	if err = os.MkdirAll(filepath.Join(media, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Root (2000).mp4", "nested/Film (2001).mp4"} {
		if err = os.WriteFile(filepath.Join(media, name), []byte("first"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	db, err := persistence.Open(filepath.Join(root, "catalog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := catalog.New(db)
	lib, err := c.Create("Movies", "movie", media)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE library_scan_policies SET tier='file_list_only',operations_json='[]' WHERE library_id=?`, lib.ID); err != nil {
		t.Fatal(err)
	}
	s := New(db, c, assets.Probe{})
	ctx := context.Background()
	run := func(mode, want string) string {
		t.Helper()
		job, e := s.QueueMode(ctx, lib.ID, "", mode, nil)
		if e != nil {
			t.Fatal(e)
		}
		if job.ScanMode != want {
			t.Fatal("wrong scan mode", job.ScanMode, want)
		}
		for i := 0; i < 100; i++ {
			s.process(ctx, job.ID, lib.ID)
			state, e := s.Get(job.ID)
			if e != nil {
				t.Fatal(e)
			}
			if state.Status == "complete" {
				return job.ID
			}
			if state.Status == "failed" {
				t.Fatal(state)
			}
		}
		t.Fatal("scan did not finish")
		return ""
	}
	run("", "first-import")
	second := run("", "incremental")
	var pages, skipped int
	if err = db.QueryRow(`SELECT sum(pages),sum(skipped) FROM inventory_directories WHERE job_id=?`, second).Scan(&pages, &skipped); err != nil || pages != 0 || skipped != 2 {
		t.Fatal("unchanged folder enumerated", pages, skipped, err)
	}
	if err = os.WriteFile(filepath.Join(media, "nested/New (2002).mp4"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	third := run("", "incremental")
	var rootSkipped int
	if err = db.QueryRow(`SELECT skipped FROM inventory_directories WHERE job_id=? AND relative_path='.'`, third).Scan(&rootSkipped); err != nil || rootSkipped != 1 {
		t.Fatal("unchanged root was walked", err)
	}
	var objects int
	if err = db.QueryRow(`SELECT count(*) FROM inventory_objects WHERE retired=0 AND state='available'`).Scan(&objects); err != nil || objects != 3 {
		t.Fatal("nested addition missed or unchanged files lost", objects, err)
	}
	file := filepath.Join(media, "nested/Film (2001).mp4")
	if err = os.WriteFile(file, []byte("replacement with different bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(time.Second)
	if err = os.Chtimes(file, now, now); err != nil {
		t.Fatal(err)
	}
	run("", "incremental")
	var size int
	if err = db.QueryRow(`SELECT size FROM inventory_objects WHERE relative_path='nested/Film (2001).mp4' AND retired=0`).Scan(&size); err != nil || size != 5 {
		t.Fatal("incremental unexpectedly re-read unchanged folder", size, err)
	}
	full := run("full", "full")
	if err = db.QueryRow(`SELECT size FROM inventory_objects WHERE relative_path='nested/Film (2001).mp4' AND retired=0`).Scan(&size); err != nil || size <= 5 {
		t.Fatal("full rescan missed replacement", size, err)
	}
	if err = db.QueryRow(`SELECT sum(skipped) FROM inventory_directories WHERE job_id=?`, full).Scan(&skipped); err != nil || skipped != 0 {
		t.Fatal("full scan skipped a directory", err)
	}
	if _, err = db.Exec(`UPDATE inventory_runs SET inventory_completed_at=? WHERE scan_mode IN('first-import','full','integrity')`, time.Now().Add(-8*24*time.Hour).UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	run("", "integrity")
}
