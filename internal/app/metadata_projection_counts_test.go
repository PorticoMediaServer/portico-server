package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestReplaceMediaCategoryFacetsRebuildsWholeLibraryCountsAtomically(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	if _, err := db.Exec(`INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, added_at) VALUES ('projection_count_peer', 'lib_movies', 'movie', 'Peer', 'Peer', '["Comedy"]', '2026-08-05T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE media_items SET genres_json = '["Drama"]' WHERE id = 'movie_meridian'`); err != nil {
		t.Fatal(err)
	}
	project := func(mediaID string) {
		t.Helper()
		if err := server.withBackgroundTxTagged(context.Background(), []string{"metadata"}, func(tx *sql.Tx) error {
			return replaceMediaCategoryFacetsTx(context.Background(), tx, mediaID, 0, "test")
		}); err != nil {
			t.Fatal(err)
		}
	}
	project("movie_meridian")
	project("projection_count_peer")

	for filter, want := range map[string]int{"genre:Drama": 1, "genre:Comedy": 1} {
		var got int
		if err := db.QueryRow(`SELECT count FROM library_category_counts WHERE library_id = 'lib_movies' AND filter = ?`, filter).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", filter, err)
		}
		if got != want {
			t.Fatalf("%s count = %d, want %d", filter, got, want)
		}
	}

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE media_items SET genres_json = '["Thriller"]' WHERE id = 'movie_meridian'`); err != nil {
		t.Fatal(err)
	}
	if err := replaceMediaCategoryFacetsTx(context.Background(), tx, "movie_meridian", 0, "test-rollback"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	var drama, thriller int
	if err := db.QueryRow(`SELECT count FROM library_category_counts WHERE library_id = 'lib_movies' AND filter = 'genre:Drama'`).Scan(&drama); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM library_category_counts WHERE library_id = 'lib_movies' AND filter = 'genre:Thriller'`).Scan(&thriller); err != nil {
		t.Fatal(err)
	}
	if drama != 1 || thriller != 0 {
		t.Fatalf("rolled-back category counts drama=%d thriller=%d", drama, thriller)
	}
}
