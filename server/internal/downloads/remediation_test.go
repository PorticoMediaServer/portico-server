package downloads

import (
	"context"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/preparedmedia"
	"testing"
	"time"
)

func TestOfflineProgressHonorsPausedHistory(t *testing.T) {
	h := newHarness(t)
	if _, err := h.db.Exec(`INSERT INTO console_documents VALUES(?,1,'{"privacy.pauseWatchHistory":true}',0)`, "profile:"+operations.ViewerScopeKey(h.viewer.Viewer)); err != nil {
		t.Fatal(err)
	}
	watched := true
	got, err := h.service.SyncProgress(context.Background(), h.viewer, "paused-history", []ProgressEntry{{ItemID: h.item, PositionSeconds: 42, Watched: &watched, ObservedAt: h.now.Add(-time.Minute).Format(time.RFC3339)}})
	if err != nil || len(got.Entries) != 1 || got.Entries[0].Outcome != ProgressHistoryPaused || got.Applied != 0 {
		t.Fatal(got, err)
	}
	for _, table := range []string{"progress", "progress_activity", "personal_items", "personal_history", "download_progress_marks"} {
		var n int
		if err = h.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
}

func TestEveryDownloadRungHasMatchingRecipeAndEstimate(t *testing.T) {
	for _, rung := range playback.QualityLadder {
		recipe, err := preparedmedia.ProfileByID(preparedProfile("movie", rung))
		if err != nil || recipe.Height != rung.Height {
			t.Fatal(rung, recipe, err)
		}
		got, _ := estimateBytes(rung.ID, source{Kind: "movie", Duration: 100})
		want := int64(float64((recipe.VideoKbps+recipe.AudioKbps)*1000) / 8 * 100 * 1.03)
		if got != want {
			t.Fatal(rung.ID, got, want)
		}
	}
}
