package main

import (
	"context"
	"path/filepath"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/persistence"
)

func TestBulkReferenceFactsDeriveIntoPlayableMixedLibraries(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "bulk.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const n = 1000
	if _, _, err = seedBulkFacts(db, n); err != nil {
		t.Fatal(err)
	}
	shape := shapeOf(n)
	for _, tc := range []struct {
		q    string
		want int
	}{{`SELECT count(*) FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind WHERE k.playable=1`, n}, {`SELECT count(*) FROM catalog_credits`, n * 6}, {`SELECT count(*) FROM catalog_term_sources`, n * 3}, {`SELECT count(*) FROM progress`, 30}, {`SELECT count(*) FROM catalog_episodes`, shape.episodes}, {`SELECT count(*) FROM catalog_songs`, shape.songs}} {
		var got int
		if err = db.QueryRow(tc.q).Scan(&got); err != nil || got != tc.want {
			t.Fatalf("%s = %d want %d: %v", tc.q, got, tc.want, err)
		}
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Fatal("fixture has broken foreign keys")
	}
	rows.Close()
	if err = compactcatalog.Drain(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	service := catalog.New(db)
	v := catalog.Viewer{Profile: "viewer", Fence: "bulk", Libraries: []string{"movies", "tv", "music"}}
	for _, tc := range []struct {
		library, pivot string
		want           int
	}{{"movies", "movies", shape.movies}, {"tv", "episodes", shape.episodes}, {"music", "songs", shape.songs}} {
		page, err := service.BrowseEntities(v, catalog.BrowseRequest{Library: tc.library, Pivot: tc.pivot, Limit: 40})
		if err != nil || page.PageInfo.Total != tc.want || len(page.Entries) == 0 {
			t.Fatalf("%s page=%+v error=%v", tc.library, page.PageInfo, err)
		}
	}
	if _, err := service.HomeRows(catalog.HomeRequest{Viewer: v, Limit: 12}); err != nil {
		t.Fatal(err)
	}
}
