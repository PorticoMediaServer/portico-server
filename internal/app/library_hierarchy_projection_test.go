package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestMediaHierarchyProjectionUsesLibraryNameAndUnboundedCatalogCounts(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	if _, err := db.Exec(`INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at) VALUES ('lib_counts', 'My Carefully Named Library', 'mixed', 900, '/tmp/counts', '{}', '2026-08-05T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	insert := func(id, parent, kind string) {
		t.Helper()
		var parentValue any
		if parent != "" {
			parentValue = parent
		}
		if _, err := db.Exec(`INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, added_at) VALUES (?, 'lib_counts', ?, ?, ?, ?, '2026-08-05T00:00:00Z')`, id, parentValue, kind, id, id); err != nil {
			t.Fatal(err)
		}
	}
	insert("show_many", "", "show")
	insert("season_many", "show_many", "season")
	for index := 0; index < maxDetailDirectChildren+37; index++ {
		insert(fmt.Sprintf("episode_many_%03d", index), "season_many", "episode")
	}
	insert("artist_many", "", "artist")
	insert("album_many", "artist_many", "album")
	for index := 0; index < maxDetailDirectChildren+19; index++ {
		insert(fmt.Sprintf("track_many_%03d", index), "album_many", "track")
	}
	insert("book_many", "", "audiobook")
	for index := 0; index < 17; index++ {
		if _, err := db.Exec(`INSERT INTO media_chapters (id, media_id, title, start_seconds, end_seconds, sort_order) VALUES (?, 'book_many', ?, ?, ?, ?)`, fmt.Sprintf("chapter_%02d", index), fmt.Sprintf("Chapter %d", index), index*60, (index+1)*60, index); err != nil {
			t.Fatal(err)
		}
	}
	items := []MediaItem{
		{ID: "show_many", LibraryID: "lib_counts", Type: "show"},
		{ID: "season_many", LibraryID: "lib_counts", Type: "season"},
		{ID: "artist_many", LibraryID: "lib_counts", Type: "artist"},
		{ID: "album_many", LibraryID: "lib_counts", Type: "album"},
		{ID: "book_many", LibraryID: "lib_counts", Type: "audiobook"},
	}
	if err := server.populateMediaHierarchyProjectionContext(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if items[0].LibraryName != "My Carefully Named Library" || items[0].Counts == nil || *items[0].Counts.SeasonCount != 1 || *items[0].Counts.EpisodeCount != maxDetailDirectChildren+37 {
		t.Fatalf("show projection = %#v", items[0])
	}
	if *items[1].Counts.EpisodeCount != maxDetailDirectChildren+37 {
		t.Fatalf("season counts = %#v", items[1].Counts)
	}
	if *items[2].Counts.ReleaseCount != 1 || *items[2].Counts.TrackCount != maxDetailDirectChildren+19 {
		t.Fatalf("artist counts = %#v", items[2].Counts)
	}
	if *items[3].Counts.TrackCount != maxDetailDirectChildren+19 {
		t.Fatalf("album counts = %#v", items[3].Counts)
	}
	if *items[4].Counts.ChapterCount != 17 {
		t.Fatalf("book counts = %#v", items[4].Counts)
	}
}

func TestMovieHierarchyProjectionSkipsUnusedRecursiveCounts(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	if _, err := db.Exec(`INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at) VALUES ('lib_movies_no_counts', 'Movies', 'movies', 903, '/tmp/movies-no-counts', '{}', '2026-08-31T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	server.sqliteMetrics = SQLiteDiagnostics{}
	server.latencyMetrics = latencyMetricsRegistry{}
	items := []MediaItem{{ID: "movie-no-counts", LibraryID: "lib_movies_no_counts", Type: "movie"}}
	if err := server.populateMediaHierarchyProjectionContext(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if items[0].LibraryName != "Movies" || items[0].Counts != nil {
		t.Fatalf("movie projection = %#v", items[0])
	}
	if reads := server.sqliteDiagnostics().ReadOperations; reads != 1 {
		t.Fatalf("movie projection used %d reads; expected library-name lookup only", reads)
	}
}

func TestMediaHierarchyProjectionTraversesBeyondSixteenLevels(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	if _, err := db.Exec(`INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at) VALUES ('lib_deep_counts', 'Deep Counts', 'mixed', 901, '/tmp/deep-counts', '{}', '2026-08-05T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	parent := ""
	for depth := 0; depth < 24; depth++ {
		id := fmt.Sprintf("deep_count_%02d", depth)
		var parentValue any
		if parent != "" {
			parentValue = parent
		}
		if _, err := db.Exec(`INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, added_at) VALUES (?, 'lib_deep_counts', ?, 'artist', ?, ?, '2026-08-05T00:00:00Z')`, id, parentValue, id, id); err != nil {
			t.Fatal(err)
		}
		parent = id
	}
	items := []MediaItem{{ID: "deep_count_00", LibraryID: "lib_deep_counts", Type: "artist"}}
	if err := server.populateMediaHierarchyProjectionContext(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if items[0].Counts == nil || items[0].Counts.ItemCount == nil || *items[0].Counts.ItemCount != 23 {
		t.Fatalf("deep hierarchy counts = %#v", items[0].Counts)
	}
}

func TestMediaHierarchyProjectionTerminatesAndCountsCycleOnce(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	if _, err := db.Exec(`INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at) VALUES ('lib_cycle_counts', 'Cycle Counts', 'mixed', 902, '/tmp/cycle-counts', '{}', '2026-08-05T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO media_items (id, library_id, type, title, sort_title, added_at) VALUES ('cycle_count_a', 'lib_cycle_counts', 'artist', 'A', 'A', '2026-08-05T00:00:00Z'), ('cycle_count_b', 'lib_cycle_counts', 'album', 'B', 'B', '2026-08-05T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE media_items SET parent_id = CASE id WHEN 'cycle_count_a' THEN 'cycle_count_b' ELSE 'cycle_count_a' END WHERE id IN ('cycle_count_a', 'cycle_count_b')`); err != nil {
		t.Fatal(err)
	}
	items := []MediaItem{{ID: "cycle_count_a", LibraryID: "lib_cycle_counts", Type: "artist"}}
	if err := server.populateMediaHierarchyProjectionContext(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if items[0].Counts == nil || items[0].Counts.ItemCount == nil || *items[0].Counts.ItemCount != 1 || items[0].Counts.ReleaseCount == nil || *items[0].Counts.ReleaseCount != 1 {
		t.Fatalf("cycle hierarchy counts = %#v", items[0].Counts)
	}
}
