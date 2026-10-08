package catalog

import (
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
