package catalog

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
)

func selectSearchGenreArt(t *testing.T, c *catalogtest.Catalog, item catalogtest.Item, kind, role string) string {
	t.Helper()
	key := fmt.Sprintf("genre-%d-%s", item.ID, role)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
	c.Exec(`INSERT INTO artwork_objects VALUES(?,'image/jpeg',1920,1080,100,'now','ready')`, digest)
	c.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,subject,provider,image_id,origin,attribution,source_fence,observed_at)
	 VALUES(?,?,?,?,'','fixture',?,'local','fixture','fence','now')`, key, kind, item.ID, role, key)
	c.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,thumbnail_digest,locked,revision,actor,observed_at)
	 VALUES(?,?,?,'',?,?,?,0,1,'fixture','now')`, kind, item.ID, role, key, digest, digest)
	return digest
}

func TestSearchGenresRuling36CountsOrderAndRepresentativeArt(t *testing.T) {
	c := catalogtest.Open(t)
	movies := c.Library("movies", "Movies", "movie", "/movies")
	tv := c.Library("tv", "TV", "tv", "/tv")
	music := c.Library("music", "Music", "music", "/music")
	private := c.Library("private", "Private", "movie", "/private")
	items := map[string]catalogtest.Item{}
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "no-art"} {
		items[name] = c.Movie(movies, "/movies/"+name+".mkv", name, 2026)
	}
	c.Genres(items["a"].ID, "fixture", "Crime", "Drama", "Sci-Fi & Fantasy", "Repeat")
	c.Genres(items["b"].ID, "fixture", "Crime", "Drama")
	c.Genres(items["c"].ID, "fixture", "Crime")
	c.Genres(items["d"].ID, "fixture", "Drama")
	c.Genres(items["e"].ID, "fixture", "Sci-Fi & Fantasy")
	c.Genres(items["f"].ID, "fixture", "No backdrop")
	c.Genres(items["no-art"].ID, "fixture", "No art")
	hidden := c.Movie(private, "/private/hidden.mkv", "Hidden", 2026)
	c.Genres(hidden.ID, "fixture", "Hidden only", "Crime")
	blocked := c.Movie(movies, "/movies/blocked.mkv", "Blocked", 2026)
	c.Genres(blocked.ID, "fixture", "Blocked only", "Crime")
	c.Attributes(blocked.ID, "label", "Adult")
	artist := c.Artist(music, "Artist")
	album := c.Album(artist, "Album", 2026)
	c.Genres(album.ID, "fixture", "Music only", "Crime")
	c.Song(album, 1, "/music/song.flac", "Song")
	show := c.Show(tv, "Show", 2026)
	season := c.Season(show, 1)
	c.Episode(show, season, 1, "/tv/one.mkv")
	c.Episode(show, season, 2, "/tv/two.mkv")
	c.Genres(show.ID, "fixture", "Television")
	emptyShow := c.Show(tv, "Empty show", 2026)
	c.Genres(emptyShow.ID, "fixture", "Quiet")
	c.Collection(movies, "Not a title", items["a"], items["b"])
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		selectSearchGenreArt(t, c, items[name], "item", "poster")
		selectSearchGenreArt(t, c, items[name], "item", "backdrop")
	}
	selectSearchGenreArt(t, c, items["f"], "item", "poster")
	selectSearchGenreArt(t, c, show, "show", "backdrop")
	c.Drain()
	for index, name := range []string{"a", "b", "c", "d", "e", "f"} {
		c.Exec(`UPDATE catalog_browse_rows SET rating_max=? WHERE entity_id=?`, 10-index, items[name].ID)
	}
	s := New(c.DB)
	viewer := Viewer{Profile: "viewer", Fence: "authority", Libraries: []string{"movies", "tv", "music"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}}
	page, err := s.SearchGenres(viewer, SearchGenreRequest{})
	if err != nil {
		t.Fatal(err)
	}
	values := []string{}
	byValue := map[string]SearchGenre{}
	for _, genre := range page.Genres {
		values = append(values, genre.Value)
		byValue[genre.Value] = genre
		if genre.Label != genre.Value || genre.Covers == nil || len(genre.Covers) > 2 {
			t.Fatal("genre lost its label or bounded cover list", genre)
		}
	}
	if want := []string{"Crime", "Drama", "Sci-Fi & Fantasy", "No art", "No backdrop", "Quiet", "Repeat", "Television"}; !reflect.DeepEqual(values, want) {
		t.Fatalf("video title counts/sort/restrictions: %v want %v", values, want)
	}
	for value, count := range map[string]int{"Crime": 3, "Drama": 3, "Sci-Fi & Fantasy": 2, "Television": 1} {
		if byValue[value].Count != count {
			t.Fatalf("%s counted episodes, collections, hidden titles or music: %+v", value, byValue[value])
		}
	}
	for value, name := range map[string]string{"Crime": "a", "Drama": "b", "Sci-Fi & Fantasy": "e", "Repeat": "a"} {
		if art := byValue[value].Backdrop; art == nil || art.ID != items[name].Public || art.Kind != "item" || !strings.HasPrefix(art.Path, "/v1/metadata/item/"+art.ID+"/art/backdrop?v=") {
			t.Fatalf("%s representative does not follow rating/no-repeat/fallback: %+v", value, art)
		}
	}
	if art := byValue["Television"].Backdrop; art == nil || art.ID != show.Public || art.Kind != "show" {
		t.Fatal("show must use its own selected backdrop", art)
	}
	if byValue["No backdrop"].Backdrop != nil || len(byValue["No backdrop"].Covers) != 1 || byValue["No art"].Backdrop != nil || len(byValue["No art"].Covers) != 0 {
		t.Fatal("missing art was invented or the counted genre was removed", byValue)
	}
	if got := byValue["Crime"].Covers; len(got) != 2 || got[0].ID != items["a"].Public || got[1].ID != items["b"].Public {
		t.Fatal("first two best posters", got)
	}
	if got := byValue["Drama"].Covers; len(got) != 2 || got[0].ID != items["d"].Public || got[1].ID != items["a"].Public {
		t.Fatal("unused poster then ranked fallback", got)
	}
	short, err := s.SearchGenres(viewer, SearchGenreRequest{Limit: 2})
	if err != nil || !reflect.DeepEqual(short.Genres, page.Genres[:2]) || short.NextCursor == "" {
		t.Fatal("the initial prefix changed with surface limit", short, err)
	}
	continued, err := s.SearchGenres(viewer, SearchGenreRequest{Limit: 2, Cursor: short.NextCursor})
	if err != nil || len(continued.Genres) != 2 || continued.Genres[0].Value != "Sci-Fi & Fantasy" || continued.Genres[1].Value != "No art" {
		t.Fatal("All genres cannot continue through missing art", continued, err)
	}
	otherViewer := viewer
	otherViewer.Fence = "revoked-and-regranted"
	if _, err = s.SearchGenres(otherViewer, SearchGenreRequest{Limit: 2, Cursor: short.NextCursor}); !errors.Is(err, ErrCursor) {
		t.Fatal("cursor crossed an authority fence", err)
	}
	// A cached counted list must still check actual ready bytes in this snapshot.
	c.Exec(`UPDATE artwork_objects SET status='deleting' WHERE digest=(SELECT digest FROM artwork_selections WHERE entity_id=? AND role='backdrop')`, items["a"].ID)
	withoutReady, err := s.SearchGenres(viewer, SearchGenreRequest{})
	if err != nil || withoutReady.Genres[0].Backdrop == nil || withoutReady.Genres[0].Backdrop.ID != items["b"].Public {
		t.Fatal("cached art outlived readiness", withoutReady, err)
	}
	c.Exec(`DELETE FROM artwork_selections WHERE entity_id=? AND role='poster'`, items["a"].ID)
	if _, err = s.SearchGenres(viewer, SearchGenreRequest{Limit: 2, Cursor: short.NextCursor}); !errors.Is(err, ErrStaleContinuation) {
		t.Fatal("selected-art revision did not invalidate the old cursor", err)
	}
	if empty, err := s.SearchGenres(Viewer{Profile: "none"}, SearchGenreRequest{}); err != nil || len(empty.Genres) != 0 {
		t.Fatal("empty library authority disclosed genres", empty, err)
	}
}

func TestSearchGenresRejectInvalidWindows(t *testing.T) {
	s := New(nil)
	for _, limit := range []int{-1, 51} {
		if _, err := s.SearchGenres(Viewer{}, SearchGenreRequest{Limit: limit}); err == nil {
			t.Fatal("invalid window accepted", limit)
		}
	}
}
