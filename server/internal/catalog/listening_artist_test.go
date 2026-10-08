package catalog

import (
	"context"
	"database/sql"
	"testing"

	"portico.local/server/internal/compactcatalog"
)

// Spec — Page Content §2 item 4: the artist entity carries the agent's
// biography (attributed), country and active years. An owner's editor text
// wins over the Wikipedia biography.
func TestListeningArtistEntityBiographyCountryYears(t *testing.T) {
	s, c, names := phase34ListeningFixture(t)
	if _, e := c.DB.Exec(`INSERT INTO music_artist_evidence(artist_id,mbid,observed_at,name,country,begin_year,end_year,wikidata_id,bio,bio_source,bio_source_url,image_url,image_licence,complete) VALUES(?,?,?,?,? ,1975,NULL,'Q123','She sang.','Wikipedia','https://en.wikipedia.org/wiki/Artist','https://upload.wikimedia.org/wikipedia/commons/a/ab/Artist.jpg','CC BY-SA 4.0',1)`, names["artist"].ID, "55555555-5555-5555-5555-555555555555", "now", "Artist", "US"); e != nil {
		t.Fatal(e)
	}
	page, e := s.Content(phase34ListeningRequest("music", "artist", names["artist"].Public))
	if e != nil || page.Entity == nil {
		t.Fatal(page, e)
	}
	entity := page.Entity
	if entity.Overview != "She sang." || entity.OverviewSource != "Wikipedia" || entity.OverviewSourceURL != "https://en.wikipedia.org/wiki/Artist" || entity.Country != "US" || entity.ActiveBeginYear == nil || *entity.ActiveBeginYear != 1975 || entity.ActiveEndYear != nil {
		t.Fatalf("artist facts: %+v", entity)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetFieldsTx(ctx, tx, names["artist"].ID, compactcatalog.Owner, map[string]any{"local_key": "artist", "overview": "Owner text"})
	})
	c.Drain() // the owner's text is read from the published artist
	page, e = s.Content(phase34ListeningRequest("music", "artist", names["artist"].Public))
	if e != nil || page.Entity == nil {
		t.Fatal(page, e)
	}
	if page.Entity.Overview != "Owner text" || page.Entity.OverviewSource != "" || page.Entity.OverviewSourceURL != "" {
		t.Fatalf("owner text did not win: %+v", page.Entity)
	}
}
