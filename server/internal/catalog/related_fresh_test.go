package catalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/persistence"
)

func recCandidateHasID(candidates []recCandidate, id string) bool {
	for _, candidate := range candidates {
		if candidate.ID == id {
			return true
		}
	}
	return false
}

func TestItemRecommendationsRecomputeFacetsOnReload(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := catalogtest.New(t, db)
	films := c.Library("movies", "Movies", "movie", "/movies")
	m1 := c.Movie(films, "/movies/one.mkv", "One", 0)
	m2 := c.Movie(films, "/movies/two.mkv", "Two", 0)
	c.Genres(m1.ID, "tmdb", "Drama")
	c.Genres(m2.ID, "tmdb", "Drama")
	c.Drain()
	s := New(db)
	item, err := s.Get("p", m1.Public)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.recTitleRelations(Viewer{Profile: "p", Fence: "p", Libraries: []string{"movies"}}, item, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || !strings.Contains(first[0].args[0].(string), `"`+m2.Public+`"`) {
		t.Fatal("missing initial related item", first)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetTermsTx(ctx, tx, m2.ID, compactcatalog.VocabGenre, "tmdb", nil)
	})
	c.Drain()
	second, err := s.recTitleRelations(Viewer{Profile: "p", Fence: "p", Libraries: []string{"movies"}}, item, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) > 0 && strings.Contains(second[0].args[0].(string), `"`+m2.Public+`"`) {
		t.Fatal("reload served old facet scores")
	}
}

func TestListeningRecommendationsRecomputeFacetsOnReload(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := catalogtest.New(t, db)
	music := c.Library("music", "Music", "music", "/music")
	artist := c.Artist(music, "Artist")
	other := c.Artist(music, "Other")
	album1 := c.Album(artist, "First release", 2020)
	// A different decade: the artist is the only facet the two albums share.
	album2 := c.Album(artist, "Second release", 1995)
	t1 := c.Song(album1, 1, "/music/First/track.flac", "Track 1")
	t3 := c.Song(album2, 1, "/music/Second/track.flac", "Track 3")
	c.Drain()
	s := New(db)
	item, err := s.Get("p", t1.Public)
	if err != nil {
		t.Fatal(err)
	}
	has := func(rows *RelatedMovies) bool {
		for _, row := range rows.Rows {
			for _, entry := range row.Entries {
				if entry.ID == album2.Public {
					return true
				}
			}
		}
		return false
	}
	first, err := s.listeningRecommendations("p", item)
	if err != nil || !has(first) {
		t.Fatal("missing initial album", first, err)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := compactcatalog.SetFactsTx(ctx, tx, album2.ID, map[string]any{"artist_id": other.ID}); err != nil {
			return err
		}
		return compactcatalog.SetSongArtistsTx(ctx, tx, t3.ID, nil)
	})
	c.Drain()
	second, err := s.listeningRecommendations("p", item)
	if err != nil {
		t.Fatal(err)
	}
	if has(second) {
		t.Fatal("reload served old artist facets")
	}
}

func TestEmptyRelatedPoolDoesNotScanLibrary(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := catalogtest.New(t, db)
	c.Library("movies", "Movies", "movie", "/movies")
	s := New(db)
	r := HomeRequest{Libraries: []string{"movies"}}
	base, args := recBase(r, []string{})
	var n int
	if err := s.read().QueryRow(base+` SELECT count(*) FROM members`, args...).Scan(&n); err != nil || n != 0 {
		t.Fatal("empty candidates widened to library", n, err)
	}
}

func TestSharedRecommendationEntryPointIsFresh(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := catalogtest.New(t, db)
	movies := c.Library("movies", "Movies", "movie", "/movies")
	m1 := c.Movie(movies, "/movies/one.mkv", "One", 0)
	m2 := c.Movie(movies, "/movies/two.mkv", "Two", 0)
	c.Genres(m1.ID, "tmdb", "Drama")
	c.Genres(m2.ID, "tmdb", "Drama")
	c.Drain()
	s := New(db)
	viewer := Viewer{Profile: "p", Fence: "fence", Libraries: []string{"movies"}}
	r := HomeRequest{Viewer: viewer, ServerID: "server", Profile: viewer.Profile, ViewerFence: viewer.Fence, Libraries: viewer.Libraries}
	first, err := s.modelledCandidates(r, "recommended")
	if err != nil || !recCandidateHasID(first, m2.Public) {
		t.Fatal(first, err)
	}
	if _, err = db.Exec(`INSERT INTO personal_items(profile_id,item_id,not_interested,revision) VALUES(?,?,1,1)`, viewer.Profile, m2.ID); err != nil {
		t.Fatal(err)
	}
	next, err := s.modelledCandidates(r, "recommended")
	if err != nil {
		t.Fatal(err)
	}
	if recCandidateHasID(next, m2.Public) {
		t.Fatal("shared entry point returned stale personal state")
	}
}
