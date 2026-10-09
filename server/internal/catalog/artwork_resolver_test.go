package catalog

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/persistence"
)

func TestResolvedThumbnailURLUsesSelectedArtworkVersion(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "art.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := catalogtest.New(t, db)
	films := c.Library("m", "Movies", "movie", "/m")
	one := c.Movie(films, "/m/One.mkv", "One", 2020)
	full, thumb := strings.Repeat("a", 64), strings.Repeat("b", 64)
	c.Exec(`INSERT INTO artwork_objects VALUES(?, 'image/png',1920,1280,100,'now','ready'),(?, 'image/png',400,267,50,'now','ready')`, full, thumb)
	c.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,subject,provider,image_id,origin,attribution,source_fence,observed_at) VALUES('candidate','item',?,'poster','','fixture','one','local','fixture','fence','now')`, one.ID)
	c.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,thumbnail_digest,locked,revision,actor,observed_at) VALUES('item',?,'poster','','candidate',?,?,0,1,'fixture','now')`, one.ID, full, thumb)
	c.Drain()
	resolved, err := New(db).resolveArtwork([]artworkTarget{{Kind: "item", ID: one.Public}})
	if err != nil {
		t.Fatal(err)
	}
	path := resolved[artworkTarget{Kind: "item", ID: one.Public}].PosterURL
	if !strings.HasPrefix(path, "/v1/metadata/item/"+one.Public+"/art/poster?") {
		t.Fatalf("artwork URL must name the item by its public id: %q", path)
	}
	if !strings.Contains(path, "v="+full+"&w=400") || strings.Contains(path, thumb) {
		t.Fatalf("thumbnail URL did not carry selected content version: %q", path)
	}
}

// A populated cache can make SQLite choose ready objects before selections.
// The page must instead seek its selections and then each selection's digest,
// independent of how many unrelated images have accumulated on the server.
func TestArtworkResolutionSeeksSelectedDigestsWithPopulatedCacheStatistics(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("movies", "Movies", "movie", "/movies")
	movie := c.Movie(lib, "/movies/One.mkv", "One", 2020)
	ready, deleting := strings.Repeat("a", 64), strings.Repeat("b", 64)
	c.Exec(`INSERT INTO artwork_objects VALUES(?, 'image/png',400,600,100,'now','ready'),(?, 'image/png',400,600,100,'now','deleting')`, ready, deleting)
	c.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,subject,provider,image_id,origin,attribution,source_fence,observed_at) VALUES('poster','item',?,'poster','','fixture','poster','local','fixture','fence','now'),('backdrop','item',?,'backdrop','','fixture','backdrop','local','fixture','fence','now')`, movie.ID, movie.ID)
	c.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,thumbnail_digest,locked,revision,actor,observed_at) VALUES('item',?,'poster','','poster',?,?,0,1,'fixture','now'),('item',?,'backdrop','','backdrop',?,?,0,1,'fixture','now')`, movie.ID, ready, ready, movie.ID, deleting, deleting)
	c.Drain()
	// Representative statistics from a cache of thousands of selected images;
	// no catalogue-wide fixture or timing threshold is required to pin the plan.
	c.Exec(`ANALYZE sqlite_schema`)
	c.Exec(`DELETE FROM sqlite_stat1 WHERE tbl IN('artwork_objects','artwork_selections')`)
	c.Exec(`INSERT INTO sqlite_stat1(tbl,idx,stat) VALUES
 ('artwork_objects','artwork_objects_created','10632 3'),
 ('artwork_objects','sqlite_autoindex_artwork_objects_1','10632 1'),
 ('artwork_selections','artwork_selections_candidate','4284 1'),
 ('artwork_selections','artwork_selections_digest','4284 1'),
 ('artwork_selections','artwork_selections_thumbnail','4284 1'),
 ('artwork_selections','sqlite_autoindex_artwork_selections_1','4284 1428 17 8 1')`)
	c.Exec(`ANALYZE sqlite_schema`)
	targets := []artworkTarget{{Kind: "item", ID: movie.Public}}
	raw, err := json.Marshal(targets)
	if err != nil {
		t.Fatal(err)
	}
	seekOnlyPlan(t, c.DB, "artwork selected digest", artworkResolutionQuery, []any{string(raw)}, "requested", "resolved", "r", "c")
	resolved, err := New(c.DB).resolveArtwork(targets)
	if err != nil {
		t.Fatal(err)
	}
	art := resolved[targets[0]]
	if !strings.Contains(art.PosterURL, "v="+ready) || art.BackdropURL != "" {
		t.Fatalf("selected ready poster or deleting-object exclusion changed: %+v", art)
	}
}
