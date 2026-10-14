package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
)

// A narrow serving-index fixture: retired facts can outpace the derived worker
// at any size. An empty preview must stop after its candidate budget instead of
// walking every stale row looking for a live title.
func TestRecentlyAddedBoundsExaminedCandidates(t *testing.T) {
	sizes := []int{256}
	switch os.Getenv("PORTICO_PERFORMANCE_TIER") {
	case "release":
		sizes = []int{10000, 200000}
	case "deep":
		sizes = []int{10000, 200000, 2000000}
	}
	for _, n := range sizes {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			db, err := dbwork.OpenHandle(filepath.Join(t.TempDir(), "recent.sqlite"), dbwork.DefaultPolicy())
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := db.Exec(q, args...); err != nil {
					t.Fatal(err)
				}
			}
			for _, q := range []string{
				`CREATE TABLE catalog_generations(singleton INTEGER PRIMARY KEY,retaining INTEGER);INSERT INTO catalog_generations VALUES(1,0)`,
				`CREATE TABLE catalog_libraries(id INTEGER PRIMARY KEY,library_id TEXT);INSERT INTO catalog_libraries VALUES(1,'m')`,
				`CREATE TABLE catalog_entities(id INTEGER PRIMARY KEY,public_id BLOB,kind INTEGER,retired INTEGER)`,
				`CREATE TABLE catalog_browse_rows(entity_id INTEGER PRIMARY KEY,library_id INTEGER,recent_text TEXT,added_text TEXT,item_id INTEGER)`,
				`CREATE INDEX catalog_browse_recent_works ON catalog_browse_rows(library_id,recent_text DESC,entity_id DESC) WHERE recent_text IS NOT NULL`,
				`CREATE INDEX catalog_browse_recent_undated ON catalog_browse_rows(library_id,entity_id DESC) WHERE item_id IS NOT NULL AND added_text IS NULL`,
				`CREATE TABLE catalog_browse_member_metrics(entity_id INTEGER,item_id INTEGER,added TEXT)`,
				`CREATE INDEX catalog_browse_metric_added ON catalog_browse_member_metrics(entity_id,added DESC) WHERE added IS NOT NULL`,
				`CREATE TABLE metadata_details(item_id INTEGER,provider TEXT,provider_id TEXT,PRIMARY KEY(item_id,provider))`,
				`CREATE TABLE catalog_episodes(entity_id INTEGER PRIMARY KEY,show_id INTEGER)`,
				`CREATE TABLE catalog_songs(entity_id INTEGER PRIMARY KEY,album_id INTEGER)`,
				`CREATE TABLE catalog_book_files(entity_id INTEGER PRIMARY KEY,book_id INTEGER)`,
				`CREATE TABLE catalog_kinds(id INTEGER PRIMARY KEY,playable INTEGER);INSERT INTO catalog_kinds VALUES(1,1)`,
				`CREATE TABLE catalog_item_availability(entity_id INTEGER PRIMARY KEY,available INTEGER,retired INTEGER)`,
			} {
				exec(q)
			}
			for after := 0; after < n; after += 10000 {
				exec(`WITH RECURSIVE ids(i) AS(SELECT ? UNION ALL SELECT i+1 FROM ids WHERE i<?) INSERT INTO catalog_entities SELECT i,randomblob(16),1,1 FROM ids`, after+1, min(n, after+10000))
				exec(`INSERT INTO catalog_browse_rows SELECT id,1,'2026-01-01','2026-01-01',id FROM catalog_entities WHERE id>? AND id<=?`, after, min(n, after+10000))
			}
			exec(`ANALYZE`)
			s := New(db)
			ctx, cost := dbwork.Measure(context.Background())
			start := time.Now()
			err = dbwork.WithReadSnapshot(ctx, db, func(ctx context.Context) error {
				found, err := s.WithContext(ctx).homeRecentWorks(HomeRequest{}, "m")
				if len(found) != 0 {
					t.Fatalf("retired titles leaked: %d", len(found))
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(start)
			statements := cost().Statements
			if statements > 20 {
				t.Fatalf("examined the library: %d statements", statements)
			}
			if elapsed > 500*time.Millisecond {
				t.Fatalf("bounded recent preview took %s", elapsed)
			}
			t.Logf("rows=%d preview=%s statements=%d", n, elapsed, statements)
		})
	}
}
