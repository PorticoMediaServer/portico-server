package ingestion

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/storage"
)

func TestDirectoryHintDoesNotAuthorizeEmptyRoot(t *testing.T) {
	f := newLoopFixture(t, "movie", "Film (2000).mkv")
	f.finish(f.queue())
	if err := os.Remove(filepath.Join(f.root, "Film (2000).mkv")); err != nil {
		t.Fatal(err)
	}
	result, err := f.s.ReportPaths(context.Background(), []string{f.root}, nil)
	if err != nil || len(result.Jobs) != 1 {
		t.Fatalf("report: %+v %v", result, err)
	}
	job := result.Jobs[0]
	for n := 0; n < 200; n++ {
		f.s.process(context.Background(), job.ID, f.library)
		state, err := f.s.Get(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Status == "failed" || state.Status == "complete" || state.Status == "complete_with_warnings" {
			if state.Status != "failed" || state.Error != "scan_source_unavailable" || f.count(`SELECT count(*) FROM inventory_objects WHERE state='available'`) != 1 {
				t.Fatalf("ambiguous empty-root report published absence: %+v; missing=%d", state, f.count(`SELECT count(*) FROM inventory_objects WHERE state='missing'`))
			}
			return
		}
	}
	t.Fatal("scan did not stop")
}

func TestReportedRemovalRespectsStorageGuard(t *testing.T) {
	f := newLoopFixture(t, "movie", "Film (2000).mkv")
	f.finish(f.queue())
	path := filepath.Join(f.root, "Film (2000).mkv")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	store := storage.New("unused")
	store.Guard = func(string) error { return os.ErrPermission }
	f.s.SetStorage(store)
	evidence, err := f.s.removedPathEvidence(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 0 {
		t.Fatal("removal inspected directly in server despite denied storage operation")
	}
}

func TestScanStopsAtNameReaderDepthBound(t *testing.T) {
	relative := strings.Repeat("d/", 129) + "Film (2000).mkv"
	f := newLoopFixture(t, "movie", relative, "Healthy (2001).mkv")
	f.finish(f.queue())
	if n := f.count(`SELECT count(*) FROM inventory_directories WHERE relative_path=?`, filepath.Dir(relative)); n != 0 {
		t.Fatalf("scan descended beyond name-reader bound: %d deepest directories", n)
	}
	if n := f.count(`SELECT count(*) FROM inventory_scan_issues WHERE reason='path_depth_exceeded' AND protect=1`); n == 0 {
		t.Fatal("depth skip not diagnosed/protected")
	}
	if n := f.count(`SELECT count(*) FROM catalog_entities WHERE kind=1 AND title='Healthy'`); n != 1 {
		t.Fatal("healthy file dropped")
	}
}
