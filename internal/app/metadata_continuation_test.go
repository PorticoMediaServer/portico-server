package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newMetadataContinuationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	migrationPath := filepath.Join("..", "..", "migrations", "001_initial.sql")
	contents, err := os.ReadFile(migrationPath)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	const marker = "-- Durable traversal state for restart-safe metadata descendant cascades."
	index := strings.Index(string(contents), marker)
	if index < 0 {
		db.Close()
		t.Fatal("metadata continuation schema marker missing")
	}
	if _, err = db.Exec(string(contents[index:])); err != nil {
		db.Close()
		t.Fatalf("apply continuation schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func startMetadataContinuationTestOperation(t *testing.T, store *MetadataContinuationStore, id string) {
	t.Helper()
	_, err := store.Start(context.Background(), MetadataContinuationStart{
		ID: id, RootKind: "show", RootID: "show-1", Provider: "tmdb",
		PolicyRevision: "policy-4", ProviderRevision: "tmdb-v3", InitialPhase: "seasons", InitialCursor: "page-1",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMetadataContinuationMoreThanFiveHundredDescendantsIsIdempotent(t *testing.T) {
	db := newMetadataContinuationTestDB(t)
	store := NewMetadataContinuationStore(db)
	ctx := context.Background()
	startMetadataContinuationTestOperation(t, store, "large-show")

	all := make([]MetadataContinuationItemInput, 0, 701)
	for i := 0; i < 701; i++ {
		all = append(all, MetadataContinuationItemInput{Key: fmt.Sprintf("episode-%04d", i), ParentKey: fmt.Sprintf("season-%02d", i/50), Kind: "episode", ProviderID: fmt.Sprint(10000 + i)})
	}
	for page := 0; page < 8; page++ {
		start, end := page*100, (page+1)*100
		if end > len(all) {
			end = len(all)
		}
		cursor := fmt.Sprintf("page-%d", page+1)
		next := fmt.Sprintf("page-%d", page+2)
		exhausted := end == len(all)
		if err := store.RecordPage(ctx, "large-show", "seasons", "", cursor, next, exhausted, all[start:end]); err != nil {
			t.Fatal(err)
		}
		// Provider delivery can be replayed after a timeout without duplicating descendants.
		if err := store.RecordPage(ctx, "large-show", "seasons", "", cursor, next, exhausted, all[start:end]); err != nil {
			t.Fatal(err)
		}
		if exhausted {
			break
		}
	}
	op, err := store.Get(ctx, "large-show")
	if err != nil {
		t.Fatal(err)
	}
	if op.Remaining != 701 {
		t.Fatalf("remaining=%d, want 701", op.Remaining)
	}

	processed := map[string]bool{}
	for len(processed) < len(all) {
		claimed, err := store.ClaimReadyItems(ctx, "large-show", 37, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if len(claimed) == 0 {
			t.Fatal("items skipped before all descendants were claimed")
		}
		for _, item := range claimed {
			if processed[item.Key] {
				t.Fatalf("duplicate claim for %s", item.Key)
			}
			processed[item.Key] = true
			if err := store.SucceedItem(ctx, "large-show", item.Key); err != nil {
				t.Fatal(err)
			}
		}
	}
	complete, err := store.TryComplete(ctx, "large-show")
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Fatal("exhausted cascade did not complete")
	}
	op, _ = store.Get(ctx, "large-show")
	if op.Status != "completed" || op.Processed != 701 || op.Remaining != 0 {
		t.Fatalf("unexpected terminal operation: %+v", op)
	}
}

func TestMetadataContinuationRestartReclaimsLeaseAndWaitsForCursorAndRetry(t *testing.T) {
	db := newMetadataContinuationTestDB(t)
	base := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	store := NewMetadataContinuationStore(db)
	store.now = func() time.Time { return base }
	ctx := context.Background()
	startMetadataContinuationTestOperation(t, store, "restart")
	if done, err := store.TryComplete(ctx, "restart"); err != nil || done {
		t.Fatalf("unconsumed initial cursor completed: done=%v err=%v", done, err)
	}
	items := []MetadataContinuationItemInput{{Key: "season-1", Kind: "season"}, {Key: "season-2", Kind: "season"}}
	if err := store.RecordPage(ctx, "restart", "seasons", "", "page-1", "", true, items); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimReadyItems(ctx, "restart", 2, time.Minute)
	if err != nil || len(claimed) != 2 {
		t.Fatalf("initial claim len=%d err=%v", len(claimed), err)
	}

	// Simulate process loss by constructing a fresh store over the durable DB.
	restarted := NewMetadataContinuationStore(db)
	restarted.now = func() time.Time { return base.Add(2 * time.Minute) }
	reclaimed, err := restarted.ClaimReadyItems(ctx, "restart", 2, time.Minute)
	if err != nil || len(reclaimed) != 2 {
		t.Fatalf("restart claim len=%d err=%v", len(reclaimed), err)
	}
	if err = restarted.SucceedItem(ctx, "restart", "season-1"); err != nil {
		t.Fatal(err)
	}
	if err = restarted.RetryItem(ctx, "restart", "season-2", "provider throttled", base.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if done, err := restarted.TryComplete(ctx, "restart"); err != nil || done {
		t.Fatalf("retrying item completed: done=%v err=%v", done, err)
	}
	restarted.now = func() time.Time { return base.Add(6 * time.Minute) }
	retry, err := restarted.ClaimReadyItems(ctx, "restart", 1, time.Minute)
	if err != nil || len(retry) != 1 || retry[0].Key != "season-2" {
		t.Fatalf("retry claim=%+v err=%v", retry, err)
	}
	if err = restarted.FailItem(ctx, "restart", "season-2", "metadata unavailable"); err != nil {
		t.Fatal(err)
	}
	if done, err := restarted.TryComplete(ctx, "restart"); err != nil || !done {
		t.Fatalf("terminal failures should finish visibly: done=%v err=%v", done, err)
	}
	failures, err := restarted.Failures(ctx, "restart")
	if err != nil || len(failures) != 1 || failures[0].Key != "season-2" {
		t.Fatalf("failures=%+v err=%v", failures, err)
	}
	op, _ := restarted.Get(ctx, "restart")
	if op.Status != "completed_with_failures" || op.Processed != 1 || op.Failed != 1 || op.Remaining != 0 {
		t.Fatalf("unexpected result: %+v", op)
	}
}

func TestMetadataContinuationNestedArtistAlbumTrackCursorPreventsFalseCompletion(t *testing.T) {
	db := newMetadataContinuationTestDB(t)
	store := NewMetadataContinuationStore(db)
	ctx := context.Background()
	_, err := store.Start(ctx, MetadataContinuationStart{ID: "artist", RootKind: "artist", RootID: "artist-1", Provider: "musicbrainz", PolicyRevision: "p1", ProviderRevision: "ws2", InitialPhase: "albums", InitialCursor: "offset:0"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RecordPage(ctx, "artist", "albums", "", "offset:0", "", true, []MetadataContinuationItemInput{{Key: "album-1", Kind: "album"}}); err != nil {
		t.Fatal(err)
	}
	album, _ := store.ClaimReadyItems(ctx, "artist", 1, time.Minute)
	if err = store.RecordPage(ctx, "artist", "tracks", "album-1", "offset:0", "offset:100", false, []MetadataContinuationItemInput{{Key: "track-1", ParentKey: "album-1", Kind: "track"}}); err != nil {
		t.Fatal(err)
	}
	if err = store.SucceedItem(ctx, "artist", album[0].Key); err != nil {
		t.Fatal(err)
	}
	track, _ := store.ClaimReadyItems(ctx, "artist", 1, time.Minute)
	if err = store.SucceedItem(ctx, "artist", track[0].Key); err != nil {
		t.Fatal(err)
	}
	if done, err := store.TryComplete(ctx, "artist"); err != nil || done {
		t.Fatalf("open track cursor completed: done=%v err=%v", done, err)
	}
	if err = store.RecordPage(ctx, "artist", "tracks", "album-1", "offset:100", "", true, nil); err != nil {
		t.Fatal(err)
	}
	if done, err := store.TryComplete(ctx, "artist"); err != nil || !done {
		t.Fatalf("nested cascade did not complete: done=%v err=%v", done, err)
	}
}

func TestMetadataContinuationDescendantPageCreatesDurableParentCursor(t *testing.T) {
	db := newMetadataContinuationTestDB(t)
	store := NewMetadataContinuationStore(db)
	ctx := context.Background()
	_, err := store.Start(ctx, MetadataContinuationStart{ID: "tree", RootKind: "show", RootID: "show-1", Provider: "tmdb", PolicyRevision: "p1", ProviderRevision: "v1", InitialPhase: "descendants"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPage(ctx, "tree", "descendants", "", "", "", true, []MetadataContinuationItemInput{{Key: "show-1", Kind: "show"}}); err != nil {
		t.Fatal(err)
	}
	var open int
	if err := db.QueryRow(`SELECT COUNT(*) FROM metadata_continuation_cursors WHERE operation_id = 'tree' AND phase = 'descendants' AND parent_key = 'show-1' AND exhausted = 0`).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 1 {
		t.Fatalf("root child cursor count=%d, want 1", open)
	}
	if done, err := store.TryComplete(ctx, "tree"); err != nil || done {
		t.Fatalf("operation completed before root hierarchy discovery: done=%v err=%v", done, err)
	}
}
