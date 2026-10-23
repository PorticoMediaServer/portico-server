package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
)

func TestSearchStructuredPersonalMatchAndPeopleFacts(t *testing.T) {
	db, s, c := searchFixture(t)
	films := c.Library("films", "Films", "movie", "/films")
	music := c.Library("music", "Music", "music", "/music")
	tv := c.Library("tv", "TV", "tv", "/tv")
	film := c.Movie(films, "/films/harbor.mkv", "Harbor", 2020)
	compactCredits(c, film.ID, personCredit("tmdb:1", "1", "Ada Lovelace", "Lead", "Acting", 0))
	hidden := c.Movie(films, "/films/private.mkv", "Private identity", 2021)
	compactCredits(c, hidden.ID, personCredit("tmdb:1", "1", "Ada Lovelace", "Lead", "Acting", 0))
	c.Attributes(hidden.ID, "label", "Adult")
	artist := c.Artist(music, "Ada Artist")
	album := c.Album(artist, "Ada Album", 2025)
	song := c.Song(album, 1, "/music/song.flac", "Quiet song")
	show := c.Show(tv, "Ada Show", 2024)
	season := c.Season(show, 2)
	episode := c.Episode(show, season, 3, "/tv/ep.mkv")
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,revision) VALUES('mine',?,1,0,1)`, film.ID)
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,completed,playback_id) VALUES('mine',?,12500,0,'search-progress')`, episode.ID)
	c.Exec(`INSERT INTO personal_play_counts(profile_id,item_id,plays) VALUES('mine',?,7)`, song.ID)
	c.Drain()
	viewer := Viewer{Profile: "mine", Fence: "viewer", Libraries: []string{"films", "music", "tv"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}}
	out, err := s.Search(context.Background(), SearchRequest{Viewer: viewer, Q: "Ada", Groups: []string{"movies", "episodes", "artists", "albums", "songs", "people"}, Limit: 40})
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]ContentEntry{}
	for _, g := range out.Groups {
		if g.Status != "success" {
			t.Fatalf("group failure: %+v", g)
		}
		for _, e := range g.Items {
			entries[e.ID] = e
		}
	}
	if e := entries[film.Public]; e.MatchedOn == nil || *e.MatchedOn != (SearchMatch{"credit", "Ada Lovelace"}) || e.Watchlisted == nil || !*e.Watchlisted {
		t.Fatalf("credit/personal state: %+v", e)
	}
	if e := entries[song.Public]; e.Artist == nil || e.Artist.ID != artist.Public || e.Album == nil || e.Album.ID != album.Public || e.Plays == nil || *e.Plays != 7 || e.MatchedOn == nil {
		t.Fatalf("song facts %+v", e)
	}
	if e := entries[episode.Public]; e.Show == nil || e.Show.ID != show.Public || e.SeasonNumber == nil || *e.SeasonNumber != 2 || e.EpisodeNumber == nil || *e.EpisodeNumber != 3 || e.ProgressSeconds == nil || *e.ProgressSeconds != 12.5 || e.MatchedOn == nil || e.MatchedOn.Field != "show" {
		t.Fatalf("episode facts %+v", e)
	}
	if e := entries[album.Public]; e.Year == nil || *e.Year != 2025 || e.Artist == nil || e.Artist.ID != artist.Public {
		t.Fatalf("album facts %+v", e)
	}
	if e := entries[artist.Public]; e.AlbumCount == nil || *e.AlbumCount != 1 {
		t.Fatalf("artist count %+v", e)
	}
	token := personToken(t, db, "tmdb:1")
	if e := entries[token]; e.KnownFor == nil || e.KnownFor.ID != film.Public || len(e.Roles) != 1 || e.Roles[0] != "Acting" {
		t.Fatalf("person known-for/roles %+v", e)
	}
	if _, ok := entries[hidden.Public]; ok {
		t.Fatal("restricted identity returned")
	}
	viewer.Profile = "other"
	other, err := s.Search(context.Background(), SearchRequest{Viewer: viewer, Q: "Ada", Groups: []string{"movies", "episodes", "songs"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range other.Groups {
		for _, e := range g.Items {
			if e.Watchlisted != nil && *e.Watchlisted || e.ProgressSeconds != nil && *e.ProgressSeconds > 0 || e.Plays != nil {
				t.Fatal("cross-profile personal state", e)
			}
		}
	}
}
func TestSearchTopUsesExistingUniqueExactAndWholeWordPrefix(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("films", "Films", "movie", "/films")
	first := c.Movie(lib, "/films/harbor.mkv", "Harbor Lights", 2020)
	c.Movie(lib, "/films/prefix.mkv", "Harbormaster", 2020)
	c.Fields(first.ID, map[string]any{"overview": "Real banner synopsis"})
	c.Drain()
	s := New(c.DB)
	r := SearchRequest{Viewer: Viewer{Profile: "mine", Fence: "viewer", Libraries: []string{"films"}}, Q: "Harbor", Groups: []string{"movies"}}
	out, err := s.Search(context.Background(), r)
	if err != nil || out.Top == nil || out.Top.Entry.ID != first.Public || out.Top.Entry.Overview != "Real banner synopsis" {
		t.Fatalf("unique whole-word Top: %+v %v", out.Top, err)
	}
	c.Movie(lib, "/films/second.mkv", "Harbor Rain", 2022)
	c.Drain()
	out, err = s.Search(context.Background(), r)
	if err != nil || out.Top != nil {
		t.Fatalf("ambiguous prefix picked a Top: %+v %v", out.Top, err)
	}
	r.Q = "Harbor Lights"
	out, err = s.Search(context.Background(), r)
	if err != nil || out.Top == nil || out.Top.Entry.ID != first.Public {
		t.Fatal("exact Top", out.Top, err)
	}
}
func TestSearchVideoGenreDiscoveryFiltersCountsAndSamples(t *testing.T) {
	c := catalogtest.Open(t)
	films := c.Library("films", "Films", "movie", "/films")
	music := c.Library("music", "Music", "music", "/music")
	books := c.Library("books", "Books", "audiobook", "/books")
	film := c.Movie(films, "/films/visible.mkv", "Visible", 2020)
	c.Genres(film.ID, "local", "Drama")
	c.Fields(film.ID, map[string]any{"backdrop_url": "local:visible-backdrop"})
	searchReadyArtworkFact(c, film.ID, "item", "backdrop", strings.Repeat("d", 64))
	searchReadyArtworkFact(c, film.ID, "item", "poster", strings.Repeat("c", 64))
	second := c.Movie(films, "/films/second.mkv", "Second", 2021)
	c.Genres(second.ID, "local", "Drama")
	searchReadyArtworkFact(c, second.ID, "item", "poster", strings.Repeat("e", 64))

	hidden := c.Movie(films, "/films/hidden.mkv", "Private", 2020)
	c.Genres(hidden.ID, "local", "Secret", "Drama")
	searchReadyArtworkFact(c, hidden.ID, "item", "backdrop", strings.Repeat("f", 64))
	c.Attributes(hidden.ID, "label", "Adult")
	album := c.Album(c.Artist(music, "Singer"), "Record", 2020)
	song := c.Song(album, 1, "/music/song.flac", "Song")
	c.Genres(song.ID, "local", "Audio only")
	book := c.Book(books, "Book", "Author")
	c.Genres(book.ID, "local", "Book only")
	c.Drain()
	s := New(c.DB)
	out, err := s.SearchDiscovery(Viewer{Profile: "p", Fence: "f", Libraries: []string{"films", "music", "books"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}}, "server", "", 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Genres) != 1 || out.Genres[0].Label != "Drama" || out.Genres[0].Count != 2 || out.GenresNextCursor != "" || out.Genres[0].BackdropURL != "/v1/metadata/item/"+film.Public+"/art/backdrop?v="+strings.Repeat("d", 64)+"&w=1920" {
		t.Fatalf("video-only genres: %+v", out)
	}

	if len(out.Genres[0].PosterURLs) != 2 || !strings.Contains(out.Genres[0].PosterURLs[0], film.Public) || !strings.Contains(out.Genres[0].PosterURLs[1], second.Public) {
		t.Fatal("bounded visible genre posters", out.Genres[0])
	}
}

func TestSearchTopShowUsesActualNextUpProgress(t *testing.T) {
	c := catalogtest.Open(t)
	tv := c.Library("tv", "TV", "tv", "/tv")
	show := c.Show(tv, "Harbor", 2024)
	season := c.Season(show, 1)
	episode := c.Episode(show, season, 1, "/tv/one.mkv")
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,completed,playback_id) VALUES('mine',?,12500,0,'search-top-progress')`, episode.ID)
	c.Drain()
	s := New(c.DB)
	out, err := s.Search(context.Background(), SearchRequest{Viewer: Viewer{Profile: "mine", Fence: "current", Libraries: []string{"tv"}}, Q: "Harbor", Groups: []string{"shows"}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Top == nil || out.Top.Group != "shows" || out.Top.Entry.ID != show.Public || out.Top.Primary == nil || out.Top.Primary.ID != episode.Public || out.Top.Primary.Playback == nil || out.Top.Primary.Playback.ItemID != episode.Public || out.Top.Primary.SeasonNumber == nil || *out.Top.Primary.SeasonNumber != 1 || out.Top.Primary.EpisodeNumber == nil || *out.Top.Primary.EpisodeNumber != 1 || out.Top.Primary.ProgressSeconds == nil || *out.Top.Primary.ProgressSeconds != 12.5 {
		t.Fatalf("actual next-up action missing %+v", out.Top)
	}
}
func TestSearchTopPunctuationUsesPresentationNormalization(t *testing.T) {
	for _, tt := range []struct{ in, want string }{{"Spider-Man", "spider man"}, {"  CAFÉ   Rain ", "cafe rain"}, {"WALL·E", "wall e"}} {
		if got := searchTopPlain(tt.in); got != tt.want {
			t.Fatalf("Top normalization %q = %q", tt.in, got)
		}
	}
}

func searchReadyArtworkFact(c *catalogtest.Catalog, entity int64, kind, role, digest string) {
	c.Exec(`INSERT INTO artwork_objects VALUES(?,'image/png',1920,1080,100,'now','ready')`, digest)
	c.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,subject,provider,image_id,origin,attribution,source_fence,observed_at) VALUES(?,?,?,?,'','fixture',?,'local','fixture','fence','now')`, digest, kind, entity, role, digest)
	c.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,thumbnail_digest,locked,revision,actor,observed_at) VALUES(?,?,?,'',?,?,?,0,1,'fixture','now')`, kind, entity, role, digest, digest, digest)
}

func TestSearchArtistArtIsBoundedAndUsesVisibleAlbums(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("music", "Music", "music", "/music")
	artist := c.Artist(lib, "Atlas")
	var hidden catalogtest.Item
	for i := 0; i < 6; i++ {
		album := c.Album(artist, fmt.Sprintf("Album %d", i), 2020+i)
		song := c.Song(album, 1, fmt.Sprintf("/music/%d.flac", i), "Song")
		searchReadyArtworkFact(c, album.ID, "album", "cover", strings.Repeat(fmt.Sprintf("%x", i+1), 64))
		if i == 5 {
			hidden = album
			c.Attributes(song.ID, "label", "Adult")
		}
	}
	c.Drain()
	s := New(c.DB)
	out, err := s.Search(context.Background(), SearchRequest{Viewer: Viewer{Profile: "mine", Fence: "current", Libraries: []string{"music"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}}, Q: "Atlas", Group: "artists"})
	if err != nil || len(out.Groups) != 1 || len(out.Groups[0].Items) != 1 {
		t.Fatal(out, err)
	}
	e := out.Groups[0].Items[0]
	if e.AlbumCount == nil || *e.AlbumCount != 5 || len(e.ArtworkPaths) != 4 {
		t.Fatal("actual bounded album art", e)
	}
	for _, path := range e.ArtworkPaths {
		if strings.Contains(path, hidden.Public) {
			t.Fatal("hidden album identity", path)
		}
	}
}

func TestSearchBookProgressUsesOnlyOwnVisibleResumePart(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("books", "Books", "audiobook", "/books")
	book := c.Book(lib, "Atlas Book", "Author")
	part := c.BookFile(book, 1, "/books/one.m4b")
	c.BookFile(book, 2, "/books/two.m4b")
	c.Exec(`INSERT INTO book_resume(profile_id,book_id,item_id,playback_id,position,unit,completed) VALUES('mine',?,?,'actual-resume',12345,0,0)`, book.ID, part.ID)
	c.Drain()
	s := New(c.DB)
	r := SearchRequest{Viewer: Viewer{Profile: "mine", Fence: "current", Libraries: []string{"books"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}}, Q: "Atlas", Group: "books"}
	read := func() ContentEntry {
		t.Helper()
		out, err := s.Search(context.Background(), r)
		if err != nil || len(out.Groups) != 1 || len(out.Groups[0].Items) != 1 {
			t.Fatal(out, err)
		}
		return out.Groups[0].Items[0]
	}
	if p := read().BookProgress; p == nil || p.BookID != book.Public || p.ItemID != part.Public || p.PartPositionMs != 12345 || p.PositionMs != nil || p.DurationMs != nil || p.RemainingMs != nil || p.Chapter != nil {
		t.Fatal("actual partial resume", p)
	}
	r.Viewer.Profile = "other"
	if read().BookProgress != nil {
		t.Fatal("another profile's resume")
	}
	r.Viewer.Profile = "mine"
	c.Attributes(part.ID, "label", "Adult")
	c.Drain()
	if read().BookProgress != nil {
		t.Fatal("hidden resume part identity")
	}
}

func TestSearchGenreContinuationKeepsRevisionAndTrueExhaustion(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("films", "Films", "movie", "/films")
	film := c.Movie(lib, "/films/one.mkv", "One", 2020)
	var names []string
	for i := 0; i < 37; i++ {
		names = append(names, fmt.Sprintf("Genre %02d", i))
	}
	c.Genres(film.ID, "local", names...)
	c.Drain()
	s := New(c.DB)
	v := Viewer{Profile: "mine", Fence: "current", Libraries: []string{"films"}}
	cursor, firstCursor := "", ""
	seen := map[string]bool{}
	var revision ContentRevision
	for {
		page, err := s.SearchDiscovery(v, "server", cursor, 7)
		if err != nil {
			t.Fatal(err)
		}
		if len(seen) == 0 {
			revision = page.Revision
			firstCursor = page.GenresNextCursor
		}
		if page.Revision != revision {
			t.Fatal("mixed genre revision")
		}
		replay, err := s.SearchDiscovery(v, "server", cursor, 7)
		if err != nil || len(replay.Genres) != len(page.Genres) {
			t.Fatal("genre replay", err)
		}
		for i, g := range page.Genres {
			if seen[g.Value] || g.Value != replay.Genres[i].Value || g.Count != 1 || g.CountIsLowerBound {
				t.Fatal("genre membership/count/replay", g)
			}
			seen[g.Value] = true
		}
		if page.GenresNextCursor == "" {
			break
		}
		if page.GenresNextCursor == cursor {
			t.Fatal("repeated genre cursor")
		}
		cursor = page.GenresNextCursor
	}
	if len(seen) != 37 {
		t.Fatal("genre display cap mistaken for exhaustion", len(seen))
	}
	c.Exec(`UPDATE library_revisions SET revision=revision+1 WHERE library_id='films'`)
	if _, err := s.SearchDiscovery(v, "server", firstCursor, 7); !errors.Is(err, ErrCursor) && !errors.Is(err, ErrStaleContinuation) {
		t.Fatal("old genre cursor did not require restart", err)
	}
}

func TestSearchTopContextualUnicodeMatchesCanonicalJS(t *testing.T) {
	for _, v := range []struct{ in, want string }{{"ΟΣ", "ος"}, {"ΟΣ Α", "ος α"}, {"ΟΣ'Α", "οσ α"}, {"Ο\u0301Σ", "ος"}, {"ΟΣ.Α", "οσ α"}, {"İ", "i"}} {
		if got := searchTopPlain(v.in); got != v.want {
			t.Fatalf("%q => %q want %q", v.in, got, v.want)
		}
	}
	c := catalogtest.Open(t)
	lib := c.Library("films", "Films", "movie", "/films")
	film := c.Movie(lib, "/films/sigma.mkv", "ΟΣ", 2026)
	c.Drain()
	s := New(c.DB)
	for _, q := range []string{"οσ", "ος", "ΟΣ"} {
		out, err := s.Search(context.Background(), SearchRequest{Viewer: Viewer{Profile: "p", Fence: "f", Libraries: []string{"films"}}, Q: q, Groups: []string{"movies"}})
		if err != nil {
			t.Fatal(err)
		}
		want := q != "οσ"
		if (out.Top != nil) != want {
			t.Fatalf("q=%s top=%+v", q, out.Top)
		}
		if out.Top != nil && out.Top.Entry.ID != film.Public {
			t.Fatal("wrong Top identity")
		}
	}
}
