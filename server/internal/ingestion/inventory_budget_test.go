//go:build linux

package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/storage"
)

// TestFolderBatchPublishesOneBudgetedPrefix is SRV029 at the scan: one
// quantum over 30 pending folders of 12 names, at the 64-name writer page,
// publishes exactly five complete folders and four names of the sixth in one
// commit, discards no listing, and leaves the rest untouched. It runs through
// the in-process path and through the real helper process; the second quantum
// starts at the cut folder and finishes it.
func TestFolderBatchPublishesOneBudgetedPrefix(t *testing.T) {
	for _, helper := range []bool{false, true} {
		t.Run(fmt.Sprintf("helper=%v", helper), func(t *testing.T) {
			files := []string{}
			for i := 0; i < 30; i++ {
				for j := 0; j < 12; j++ {
					files = append(files, fmt.Sprintf("F%02d/Film %02d (2000) - pt%d.mkv", i, i, j+1))
				}
			}
			f := newLoopFixture(t, "movie", files...)
			if helper {
				binary, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				f.s.SetStorage(storage.New(binary))
			}
			job := f.queue()
			// The root is the only pending folder, so it takes the one-folder
			// path and seeds the 30 child folders.
			f.s.processQuantum(context.Background(), job.ID, f.library)
			if n := f.count(`SELECT count(*) FROM inventory_directories WHERE job_id=? AND state='pending'`, job.ID); n != 30 {
				t.Fatalf("pending folders after the root page: %d", n)
			}
			f.s.processQuantum(context.Background(), job.ID, f.library)
			if n := f.count(`SELECT count(*) FROM inventory_directories WHERE job_id=? AND state='done' AND relative_path<>'.' AND pages=1`, job.ID); n != 5 {
				t.Fatalf("complete folders after one batch: %d, want 5", n)
			}
			if n := f.count(`SELECT count(*) FROM inventory_directories WHERE job_id=? AND relative_path='F05' AND state='pending' AND cursor<>'' AND pages=1`, job.ID); n != 1 {
				t.Fatal("the sixth folder was not cut with a cursor")
			}
			if n := f.count(`SELECT count(*) FROM inventory_directories WHERE job_id=? AND pages=0`, job.ID); n != 24 {
				t.Fatalf("untouched folders past the cut: %d, want 24", n)
			}
			if n := f.count(`SELECT count(*) FROM inventory_objects WHERE source_id=? AND retired=0`, f.source); n != 64 {
				t.Fatalf("objects after one batch: %d, want 64", n)
			}
			f.s.processQuantum(context.Background(), job.ID, f.library)
			if n := f.count(`SELECT count(*) FROM inventory_directories WHERE job_id=? AND relative_path='F05' AND state='done' AND pages=2`, job.ID); n != 1 {
				t.Fatal("the cut folder did not finish in the next batch")
			}
			if n := f.count(`SELECT count(*) FROM inventory_objects WHERE source_id=? AND retired=0 AND relative_path LIKE 'F05/%'`, f.source); n != 12 {
				t.Fatalf("the cut folder has %d objects across the cut, want 12", n)
			}
		})
	}
}

// TestFolderBatchOf32EmptyFoldersIsOneCommit: empty folders cost no budget, so
// one quantum completes all of them, through either path.
func TestFolderBatchOf32EmptyFoldersIsOneCommit(t *testing.T) {
	for _, helper := range []bool{false, true} {
		t.Run(fmt.Sprintf("helper=%v", helper), func(t *testing.T) {
			f := newLoopFixture(t, "movie")
			for i := 0; i < 32; i++ {
				if err := os.MkdirAll(filepath.Join(f.root, fmt.Sprintf("E%02d", i)), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if helper {
				binary, _ := os.Executable()
				f.s.SetStorage(storage.New(binary))
			}
			job := f.queue()
			for n := 0; n < 2; n++ {
				f.s.processQuantum(context.Background(), job.ID, f.library)
			}
			if n := f.count(`SELECT count(*) FROM inventory_directories WHERE job_id=? AND state='done' AND relative_path<>'.'`, job.ID); n != 32 {
				t.Fatalf("empty folders done after one batch: %d, want 32", n)
			}
		})
	}
}

// TestBudgetPrefixStopsBeforeTheFolderTheOneFolderPathOwns: the published
// prefix ends before an error, an unstable page or a skip, and a lone healthy
// page is published on its own.
func TestBudgetPrefixStopsBeforeTheFolderTheOneFolderPathOwns(t *testing.T) {
	page := func(entries int, unstable bool) storage.BudgetEnvelope {
		p := storage.InventoryPage{Entries: make([]storage.Snapshot, entries), Unstable: unstable, Complete: true}
		return storage.BudgetEnvelope{Outcome: storage.BudgetPage, RawUsed: entries, Page: &p}
	}
	skip := storage.BudgetEnvelope{Outcome: storage.BudgetSkipped}
	fail := storage.BudgetEnvelope{Outcome: storage.BudgetError, ErrorCode: "changed"}
	for name, tc := range map[string]struct {
		envelopes []storage.BudgetEnvelope
		want      int
	}{
		"unstable fourth":  {[]storage.BudgetEnvelope{page(12, false), page(12, false), page(12, false), page(3, true), skip}, 3},
		"error sixth":      {[]storage.BudgetEnvelope{page(12, false), page(12, false), page(12, false), page(12, false), page(12, false), fail, skip}, 5},
		"one then skips":   {[]storage.BudgetEnvelope{page(64, false), skip, skip}, 1},
		"error first":      {[]storage.BudgetEnvelope{fail, skip}, 0},
		"over the cap":     {[]storage.BudgetEnvelope{page(40, false), page(40, false)}, 1},
		"every folder fit": {[]storage.BudgetEnvelope{page(0, false), page(0, false)}, 2},
	} {
		if got := budgetPrefix(tc.envelopes, 64); got != tc.want {
			t.Errorf("%s: prefix %d, want %d", name, got, tc.want)
		}
	}
}

// TestBudgetedFolderBatchScansToTheSameCatalogue is r3's parity check: a whole
// scan of a mixed tree (small and large folders, nesting, Unicode, ignored
// names, an empty folder) ends with the same objects, issues and completed
// folders through the budgeted batch as through today's batch.
func TestBudgetedFolderBatchScansToTheSameCatalogue(t *testing.T) {
	files := []string{"Älien (1979)/Älien (1979).mkv", "Älien (1979)/.hidden", "Älien (1979)/@eaDir/x.jpg", "日本 (2001)/日本 (2001).mkv"}
	for i := 0; i < 40; i++ {
		files = append(files, fmt.Sprintf("Big (2002)/Big (2002) - pt%d.mkv", i+1))
	}
	for i := 0; i < 20; i++ {
		files = append(files, fmt.Sprintf("Shelf/Small %02d (2003)/Small %02d (2003).mkv", i, i))
	}
	scan := func(budgeted bool) map[string]string {
		old := budgetedFolderBatches
		budgetedFolderBatches = budgeted
		defer func() { budgetedFolderBatches = old }()
		f := newLoopFixture(t, "movie", files...)
		if err := os.MkdirAll(filepath.Join(f.root, "Empty"), 0700); err != nil {
			t.Fatal(err)
		}
		f.finish(f.queue())
		out := map[string]string{}
		for _, q := range []struct{ name, query string }{
			{"objects", `SELECT group_concat(relative_path,'|') FROM (SELECT relative_path FROM inventory_objects WHERE retired=0 ORDER BY relative_path)`},
			{"issues", `SELECT COALESCE(group_concat(relative_path||':'||reason,'|'),'') FROM (SELECT relative_path,reason FROM inventory_scan_issues ORDER BY relative_path,reason)`},
			{"folders", `SELECT group_concat(relative_path,'|') FROM (SELECT relative_path FROM inventory_directory_cache ORDER BY relative_path)`},
		} {
			var v string
			if err := f.db.QueryRow(q.query).Scan(&v); err != nil {
				t.Fatal(err)
			}
			out[q.name] = v
		}
		return out
	}
	budgeted, today := scan(true), scan(false)
	for name := range today {
		if budgeted[name] != today[name] {
			t.Errorf("%s differ:\nbudgeted %s\ntoday    %s", name, budgeted[name], today[name])
		}
	}
	if budgeted["objects"] == "" {
		t.Fatal("the scan published nothing")
	}
}

// TestBudgetedFolderBatchIsFencedAtCommit: a run that stops being the live run
// between its listing and its commit (paused here; a disable, root or policy
// change takes the same fence) publishes nothing from the batch.
func TestBudgetedFolderBatchIsFencedAtCommit(t *testing.T) {
	files := []string{}
	for i := 0; i < 6; i++ {
		files = append(files, fmt.Sprintf("F%02d/Film %02d (2000).mkv", i, i))
	}
	f := newLoopFixture(t, "movie", files...)
	job := f.queue()
	f.s.processQuantum(context.Background(), job.ID, f.library)
	f.exec(`UPDATE jobs SET status='paused' WHERE id=?`, job.ID)
	handled, err := f.s.inventoryFolders(context.Background(), job.ID, f.librarySource())
	if handled || err != nil {
		t.Fatalf("a fenced batch reported handled=%v err=%v", handled, err)
	}
	if n := f.count(`SELECT count(*) FROM inventory_directories WHERE job_id=? AND relative_path<>'.' AND pages>0`, job.ID); n != 0 {
		t.Fatalf("a fenced batch advanced %d folders", n)
	}
	if n := f.count(`SELECT count(*) FROM inventory_objects WHERE source_id=?`, f.source); n != 0 {
		t.Fatalf("a fenced batch published %d objects", n)
	}
}
