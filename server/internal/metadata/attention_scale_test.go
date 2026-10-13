package metadata

import (
	"context"
	"fmt"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"testing"
)

func TestAttentionCountsFollowDismissalRetirementAndRemoval(t *testing.T) {
	s, db, _ := screenFixture(t)
	item := publicationFixtureMovie(t, db)
	ctx := context.Background()
	check := func(want int64) {
		t.Helper()
		counts, err := s.AttentionCounts(ctx)
		if err != nil || counts.Total != want {
			t.Fatalf("counts=%+v, want %d: %v", counts, want, err)
		}
		for _, library := range []string{"", "lib"} {
			page, err := s.Attention(ctx, library, "", 25)
			if err != nil || page.Total != want {
				t.Fatalf("%q page=%+v: %v", library, page, err)
			}
		}
	}
	publicationExec(t, db, `UPDATE screen_metadata_work SET status='low_confidence' WHERE target_id=?`, item.ID)
	check(1)
	publicationExec(t, db, `UPDATE screen_metadata_attention SET dismissed_generation=generation WHERE entity_id=?`, item.ID)
	check(0)
	publicationExec(t, db, `UPDATE screen_metadata_work SET generation=generation+1 WHERE target_id=?`, item.ID)
	check(1)
	publicationExec(t, db, `UPDATE catalog_entities SET retired=1 WHERE id=?`, item.ID)
	check(0)
	// Recreating an attention entry for an already retired title must stay hidden.
	publicationExec(t, db, `DELETE FROM screen_metadata_attention WHERE entity_id=?`, item.ID)
	publicationExec(t, db, `UPDATE screen_metadata_work SET status='unmatched' WHERE target_id=?`, item.ID)
	check(0)
	publicationExec(t, db, `UPDATE catalog_entities SET retired=0 WHERE id=?`, item.ID)
	check(1)
	publicationExec(t, db, `UPDATE screen_metadata_work SET status='pending' WHERE target_id=?`, item.ID)
	check(0)
	publicationExec(t, db, `UPDATE screen_metadata_work SET status='unmatched' WHERE target_id=?`, item.ID)
	check(1)
	publicationExec(t, db, `DELETE FROM catalog_entities WHERE id=?`, item.ID)
	check(0)
}

func TestAttentionPageHasConstantStatementCount(t *testing.T) {
	s, db, _ := screenFixture(t)
	c := catalogtest.New(t, db)
	lib := c.Handle("lib")
	for i := 0; i < 24; i++ {
		c.Movie(lib, fmt.Sprintf("/films/scale%d.mkv", i), fmt.Sprintf("Film %d", i), 2000)
	}
	ctx, cost := dbwork.Measure(context.Background())
	page, err := s.Attention(ctx, "lib", "", 25)
	if err != nil || len(page.Entries) != 25 {
		t.Fatalf("page entries=%d: %v", len(page.Entries), err)
	}
	// Count, existence, page and page candidates; independent of page length.
	if got := cost().Statements; got > 6 {
		t.Fatalf("page issued %d statements", got)
	}
	for _, entry := range page.Entries {
		if entry.EntityID == "" {
			t.Fatal("missing public identity")
		}
	}
}
