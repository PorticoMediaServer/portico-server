package compactcatalog

import (
	"fmt"
	"os"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"strings"
	"testing"
	"time"
)

func TestBlockSplitsSeekInsideEqualKeyRuns(t *testing.T) {
	sizes := []int{256}
	if os.Getenv("PORTICO_PERFORMANCE_TIER") == "release" {
		sizes = []int{10000, 200000}
	}
	if os.Getenv("PORTICO_PERFORMANCE_TIER") == "deep" {
		sizes = []int{10000, 200000, 1000000}
	}
	schema, err := os.ReadFile("../persistence/migrations/0001_baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	trigger := strings.Split(strings.Split(string(schema), "CREATE TRIGGER catalog_browse_blocks_year_join")[1], "END;")[0]
	start := strings.Index(trigger, "(SELECT seek_id FROM(")
	if start < 0 {
		t.Fatal("year split is not a bounded seek")
	}
	depth, end := 0, 0
	for i, c := range trigger[start:] {
		if c == '(' {
			depth++
		}
		if c == ')' {
			depth--
			if depth == 0 {
				end = start + i
				break
			}
		}
	}
	if end == 0 {
		t.Fatal("unbalanced selector")
	}
	selector := trigger[start+1 : end]
	for _, n := range sizes {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			db, err := dbwork.OpenHandle(filepath.Join(t.TempDir(), "blocks.sqlite"), dbwork.DefaultPolicy())
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, q := range []string{`CREATE TABLE catalog_browse_rows(entity_id INTEGER PRIMARY KEY,library_id INTEGER,kind INTEGER,year INTEGER)`, `CREATE INDEX catalog_browse_year ON catalog_browse_rows(library_id,kind,year,entity_id)`, fmt.Sprintf(`WITH RECURSIVE ids(i) AS(SELECT 1 UNION ALL SELECT i+1 FROM ids WHERE i<%d) INSERT INTO catalog_browse_rows SELECT i,1,1,2000 FROM ids`, n), `ANALYZE`} {
				if _, err = db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			anchor := max(1, n-2048)
			offset := min(1024, n-anchor)
			query := strings.NewReplacer("b.library_id", "1", "b.kind", "1", "b.sort_value", "2000", "b.entity_id", fmt.Sprint(anchor), "LIMIT 1025", fmt.Sprintf("LIMIT %d", offset+1), "OFFSET 1024", fmt.Sprintf("OFFSET %d", offset)).Replace(selector)
			old := fmt.Sprintf(`SELECT r.entity_id FROM catalog_browse_rows r INDEXED BY catalog_browse_year WHERE r.library_id=1 AND r.kind=1 AND (r.year>2000 OR r.year=2000 AND r.entity_id>=%d) ORDER BY r.year,r.entity_id LIMIT 1 OFFSET %d`, anchor, offset)
			for _, version := range []struct{ name, query string }{{"before", old}, {"after", query}} {
				rows, err := db.Query("EXPLAIN QUERY PLAN " + version.query)
				if err != nil {
					t.Fatal(err)
				}
				plan := ""
				for rows.Next() {
					var a, b, c int
					var d string
					if err = rows.Scan(&a, &b, &c, &d); err != nil {
						t.Fatal(err)
					}
					plan += d + "\n"
				}
				rows.Close()
				began := time.Now()
				var got int64
				if err = db.QueryRow(version.query).Scan(&got); err != nil || got != int64(anchor+offset) {
					t.Fatalf("%s id=%d err=%v", version.name, got, err)
				}
				elapsed := time.Since(began)
				t.Logf("rows=%d %s elapsed=%s plan=%s", n, version.name, elapsed, plan)
				if version.name == "after" && (!strings.Contains(plan, "entity_id>?") || elapsed > 100*time.Millisecond) {
					t.Fatalf("split walked a prefix: %s %s", elapsed, plan)
				}
			}
		})
	}
}
