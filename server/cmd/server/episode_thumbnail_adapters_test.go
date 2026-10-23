package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadata"
	"portico.local/server/internal/remotesources"
	"portico.local/server/internal/workpolicy"
)

func TestEpisodeSourceStateUsesCurrentMetadataAndSinglePolicyWriter(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("tv", "TV", "tv", "/tv")
	show := c.Show(library, "Show", 2026)
	season := c.Season(show, 1)
	episode := c.Episode(show, season, 1, "/tv/one.mkv")
	c.Drain()
	ctx := context.Background()
	reader := episodeMetadataState{service: metadata.New(c.DB, "")}
	target := remotesources.EpisodeTarget{LibraryID: "tv", EpisodeID: episode.Public}
	var before remotesources.EpisodeState
	if err := dbwork.WithWriteTx(ctx, c.DB, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		var err error
		before, err = reader.ReadEpisodeStateTx(ctx, tx, target)
		if err != nil {
			return err
		}
		if before.Mode != "provider_then_made" || before.PolicyRevision < 1 || len(before.ArtworkRevision) != 64 || before.ProviderResolved || before.ProviderStill || !before.GenerationNeeded {
			t.Fatalf("unknown provider facts were replaced: %+v", before)
		}
		if err = catalog.ApplyEpisodeThumbnailModeTx(ctx, tx, "tv", "off", before.PolicyRevision); err != nil {
			return err
		}
		// Reading through this same uncommitted transaction sees the sole
		// writer's new revision, rather than reopening an older snapshot.
		after, err := reader.ReadEpisodeStateTx(ctx, tx, target)
		if err == nil && (after.Mode != "off" || after.PolicyRevision != before.PolicyRevision+1 || after.ProviderResolved || after.ProviderStill) {
			t.Fatalf("mode/revision or unknown metadata lost: %+v", after)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadEpisodeStateTx(ctx, nil, target); err == nil {
		t.Fatal("nil transaction admitted")
	}
	if err := dbwork.WithReadSnapshot(ctx, c.DB, func(ctx context.Context) error {
		tx := dbwork.Snapshot(ctx)
		wrong := target
		wrong.LibraryID = "other"
		if _, err := reader.ReadEpisodeStateTx(ctx, tx, wrong); err == nil {
			t.Fatal("wrong library read as target metadata")
		}
		if _, err := (episodeMetadataState{}).ReadEpisodeStateTx(ctx, tx, target); err == nil {
			t.Fatal("absent metadata producer supplied a fact")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEpisodeSourceAnalysisUsesSupportedWindowAndPause(t *testing.T) {
	c := catalogtest.Open(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	reader := episodeAnalysisSchedule{now: func() time.Time { return at }}
	if err := dbwork.WithWriteTx(ctx, c.DB, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		if err := workpolicy.SaveTx(ctx, tx, workpolicy.Policy{Windows: []workpolicy.Window{
			{ID: "heavy", Enabled: true, Days: []string{"monday"}, StartMinute: 60, DurationMinutes: 180, Timezone: "UTC", Tasks: []string{"analysis"}},
		}}); err != nil {
			return err
		}
		first, err := reader.ReadEpisodeAnalysisTx(ctx, tx, remotesources.EpisodeTarget{})
		if err != nil {
			return err
		}
		if first.Revision < 1 || !first.Open {
			t.Fatalf("actual open analysis window lost: %+v", first)
		}
		at = at.Add(10 * time.Hour)
		outside, err := reader.ReadEpisodeAnalysisTx(ctx, tx, remotesources.EpisodeTarget{})
		if err != nil {
			return err
		}
		if outside.Open || outside.Revision != first.Revision {
			t.Fatalf("outside window admitted or revision changed: %+v", outside)
		}
		at = at.Add(-10 * time.Hour)
		if _, err = tx.ExecContext(ctx, `UPDATE maintenance_policy SET paused_until_ms=?,revision=revision+1 WHERE singleton=1`, at.Add(time.Hour).UnixMilli()); err != nil {
			return err
		}
		paused, err := reader.ReadEpisodeAnalysisTx(ctx, tx, remotesources.EpisodeTarget{})
		if err == nil && (paused.Open || paused.Revision != first.Revision+1) {
			t.Fatalf("current pause or policy revision ignored: %+v", paused)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadEpisodeAnalysisTx(ctx, nil, remotesources.EpisodeTarget{}); err == nil {
		t.Fatal("nil transaction supplied scheduling facts")
	}
}
