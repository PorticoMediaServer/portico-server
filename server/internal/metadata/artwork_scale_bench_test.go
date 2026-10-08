package metadata

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"portico.local/server/internal/persistence"
)

// TestArtworkCompactionStepScale is an opt-in measurement (BE-SRV-14): with N
// already-compacted artwork objects, one compaction step's candidate read
// against the previous whole-table query.
//
//	PORTICO_ARTWORK_SCALE=1000000 go test ./internal/metadata -run TestArtworkCompactionStepScale -count=1 -v
func TestArtworkCompactionStepScale(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("PORTICO_ARTWORK_SCALE"))
	if n <= 0 {
		t.Skip("set PORTICO_ARTWORK_SCALE to the object count")
	}
	db, err := persistence.Open(filepath.Join(t.TempDir(), "art.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Now()
	if _, err = db.Exec(`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM n WHERE i<?)
	 INSERT INTO artwork_objects SELECT printf('%064x',i),'image/jpeg',800,1200,120000+(i%300000),'2026-01-01T00:00:00Z','ready' FROM n`, n-1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`ANALYZE`); err != nil {
		t.Fatal(err)
	}
	t.Logf("seeded %d objects in %s", n, time.Since(start).Round(time.Millisecond))
	s := New(db, "")
	s.cacheRoot = t.TempDir()
	measure := func(label string, run func() error) {
		best := time.Duration(1 << 62)
		for k := 0; k < 5; k++ {
			began := time.Now()
			if err := run(); err != nil {
				t.Fatal(label, err)
			}
			best = min(best, time.Since(began))
		}
		t.Logf("%s: best of 5 %s", label, best.Round(10*time.Microsecond))
	}
	measure("compaction step (0103 partial index)", func() error { return s.CompactArtworkStep(context.Background()) })
	// The previous schema had no candidate index.
	if _, err = db.Exec(`DROP INDEX artwork_objects_compaction`); err != nil {
		t.Fatal(err)
	}
	measure("previous candidate query (whole table)", func() error {
		var d string
		err := db.QueryRow(`SELECT digest FROM artwork_objects WHERE status='ready' AND (bytes>? OR width>? OR height>?) ORDER BY bytes DESC,digest LIMIT 1`, artworkDisplayBytes, artworkLargeEdge, artworkLargeEdge).Scan(&d)
		if err != nil && err.Error() != "sql: no rows in result set" {
			return fmt.Errorf("%w", err)
		}
		return nil
	})
}
