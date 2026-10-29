package compactcatalog_test

import (
	"fmt"
	"strings"
	"testing"

	"portico.local/server/internal/compactcatalog"
)

// Keep writing while the worker walks. The first captured image must publish;
// subsequent bumps owe another finite pass and never revoke that image.
func TestReadingProjectionCapturedPassPublishesUnderContinuousAppend(t *testing.T) {
	c, library, items := readingProjectionFixture(t, 12)
	readingProjectionStep(t, c, 3) // rule
	readingProjectionStep(t, c, 3) // finite frontier
	var fence int64
	if err := c.DB.QueryRow(`SELECT fence_item FROM reading_attribution_state WHERE library_id=?`, library).Scan(&fence); err != nil {
		t.Fatal(err)
	}
	if fence != items[len(items)-1].ID {
		t.Fatalf("frontier %d", fence)
	}
	published := false
	var first int64
	for turn := 0; turn < 40; turn++ {
		c.Entity(compactcatalog.Entity{Library: library, Kind: compactcatalog.Ebook, Key: fmt.Sprintf("append-%d", turn), Title: "Appended"}, nil)
		readingProjectionStep(t, c, 3)
		var ready, after, held int64
		var phase string
		if err := c.DB.QueryRow(`SELECT ready_generation,after_item,fence_item,phase FROM reading_attribution_state WHERE library_id=?`, library).Scan(&ready, &after, &held, &phase); err != nil {
			t.Fatal(err)
		}
		if !published && ready == 0 && held != fence {
			t.Fatalf("input restarted captured frontier: %d != %d", held, fence)
		}
		if ready > 0 && !published {
			published, first = true, ready
			if turn > 5 {
				t.Fatalf("first pass did not publish within captured window: %d", turn)
			}
			var count int
			if err := c.DB.QueryRow(`SELECT count(*) FROM reading_attribution_members WHERE library_id=? AND generation=?`, library, ready).Scan(&count); err != nil || count != 12 {
				t.Fatalf("first image %d %v", count, err)
			}
		}
		if published && ready < first {
			t.Fatalf("published image retired during import: %d", ready)
		}
	}
	if !published {
		t.Fatal("continuous input withheld first publication")
	}
	readingProjectionDrain(t, c)
	readingProjectionGeneration(t, c, library, true)
}

func TestReadingProjectionPendingIndexCoversInputFlipAndCleanup(t *testing.T) {
	c, library, items := readingProjectionFixture(t, 3)
	readingProjectionDrain(t, c)
	pending := func(want int) {
		t.Helper()
		var got int
		if err := c.DB.QueryRow(`SELECT count(*) FROM reading_attribution_state INDEXED BY reading_attribution_pending WHERE (phase<>'ready')=1 AND library_id=?`, library).Scan(&got); err != nil || got != want {
			t.Fatalf("pending=%d want=%d err=%v", got, want, err)
		}
	}
	pending(0)
	c.Exec(`UPDATE catalog_entities SET sort_key='input-bump' WHERE id=?`, items[0].ID)
	pending(1) // the same input trigger must enqueue the ready scope
	rows, err := c.DB.Query(`EXPLAIN QUERY PLAN SELECT library_id FROM reading_attribution_state WHERE (phase<>'ready')=1 ORDER BY served_ms,namespace LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	for rows.Next() {
		var a, b, d int
		var detail string
		if err = rows.Scan(&a, &b, &d, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if !strings.Contains(plan.String(), "SEARCH reading_attribution_state USING INDEX reading_attribution_pending") || strings.Contains(plan.String(), "TEMP B-TREE") {
		t.Fatal(plan.String())
	}
	old := readingProjectionGeneration(t, c, library, false).Ready
	for turn := 0; turn < 20; turn++ {
		readingProjectionStep(t, c, 1)
		var phase string
		var ready int64
		if err = c.DB.QueryRow(`SELECT phase,ready_generation FROM reading_attribution_state WHERE library_id=?`, library).Scan(&phase, &ready); err != nil {
			t.Fatal(err)
		}
		pending(1)
		if phase == "cleanup" {
			if ready <= old {
				t.Fatal("cleanup began before published flip")
			}
			readingProjectionGeneration(t, c, library, true)
			break
		}
		if turn == 19 {
			t.Fatal("never flipped")
		}
	}
	readingProjectionStep(t, c, 1)
	pending(1) // old members remain
	readingProjectionDrain(t, c)
	pending(0)
}
