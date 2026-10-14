package catalog

import (
	"context"
	"fmt"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

// readSteps runs one read in its own snapshot and returns the SQLite
// virtual-machine steps it took. Steps measure work independently of the
// machine, so these tests assert the shape of a read's cost, not a time.
func readSteps(t *testing.T, c *catalogtest.Catalog, read func(*Service) error) int64 {
	t.Helper()
	ctx, cost := dbwork.Measure(context.Background())
	if err := dbwork.WithReadSnapshot(ctx, c.DB, func(ctx context.Context) error { return read(New(c.DB).WithContext(ctx)) }); err != nil {
		t.Fatal(err)
	}
	return cost().VMSteps
}

// A filtered grouping counts the entities that hold the grouped field. Songs
// hold genres; their album and artist do not, and evaluating a song predicate
// on those containers walks every song a second and third time.
func TestFilteredGroupingEvaluatesOnlyFieldHolders(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("mu", "Music", "music", "/mu")
	artist := c.Artist(lib, "Someone")
	for a := 0; a < 3; a++ {
		album := c.Album(artist, fmt.Sprintf("Album %d", a), 2000+a)
		for i := 0; i < 60; i++ {
			song := c.Song(album, i+1, fmt.Sprintf("/mu/%d/%d.flac", a, i), fmt.Sprintf("Song %d %d", a, i))
			c.Genres(song.ID, "fixture", fmt.Sprintf("Genre %d", i%4))
		}
	}
	c.Drain()
	c.Exec(`ANALYZE`)
	v := Viewer{Profile: "p", Fence: "shape", Libraries: []string{"mu"}}
	for _, query := range []*BrowseNode{
		// Version counts have no value-side index, so each title is tested.
		{Field: "versions", Operator: "equals", Value: float64(9)},
		{Field: "versions", Operator: "equals", Value: float64(1)},
	} {
		var groups BrowseResult
		grouped := readSteps(t, c, func(s *Service) error {
			var err error
			groups, err = s.BrowseEntities(v, BrowseRequest{Library: "mu", Pivot: "genres", Query: query, Limit: 40})
			return err
		})
		// The same predicate counted over the songs alone.
		var songs int
		counted := readSteps(t, c, func(s *Service) error {
			compiler := browseCompiler{profile: v.Profile}
			clause, err := compiler.compile(query)
			if err != nil {
				return err
			}
			return s.read().QueryRow(`SELECT count(*) FROM catalog_browse_rows e WHERE e.library_id=(SELECT id FROM catalog_libraries WHERE library_id='mu') AND e.kind=? AND `+clause, append([]any{int(compactcatalog.Track)}, compiler.args...)...).Scan(&songs)
		})
		total := 0
		for _, entry := range groups.Entries {
			total += *entry.Count
		}
		if total != songs {
			t.Fatalf("%v: grouped counts %d, songs %d", query.Value, total, songs)
		}
		t.Logf("%v: grouping %d steps, songs count %d steps", query.Value, grouped, counted)
		// With no match the grouping's work is the predicate's evaluation; the
		// former shape took about 2.5 times the songs' count here.
		if songs == 0 && grouped > counted*2 {
			t.Fatalf("%v: grouping took %d steps, more than twice the %d of counting the songs", query.Value, grouped, counted)
		}
	}
}

// A selective filter is answered from its values: artists, attribute terms and
// the ordered ratings index name the few matching titles, so the first page
// and its exact count cost the same in a four times larger library.
func TestSelectiveFiltersStayFlatAsTheLibraryGrows(t *testing.T) {
	queries := map[string]*BrowseNode{
		"artist":          {Field: "artist", Operator: "equals", Value: "Nobody"},
		"album":           {Field: "album", Operator: "starts-with", Value: "Rare"},
		"tag":             {Field: "tag", Operator: "equals", Value: "Rare"},
		"communityRating": {Field: "communityRating", Operator: "greater-than", Value: float64(9)},
	}
	steps := map[string][2]int64{}
	for size, songs := range []int{80, 320} {
		c := catalogtest.Open(t)
		lib := c.Library("mu", "Music", "music", "/mu")
		// The two matching songs belong to their own small artist and album,
		// so a container page reads the same small container at both sizes.
		rarity := c.Album(c.Artist(lib, "Rarity"), "Rare Album", 2000)
		someone := c.Artist(lib, "Someone")
		var album catalogtest.Item
		for i := 0; i < songs; i++ {
			on := rarity
			if i >= 2 {
				if (i-2)%20 == 0 {
					album = c.Album(someone, fmt.Sprintf("Album %d", i/20), 2000)
				}
				on = album
			}
			song := c.Song(on, i%20+1, fmt.Sprintf("/mu/%d.flac", i), fmt.Sprintf("Song %d", i))
			tag := "Common"
			if i < 2 {
				tag = "Rare"
			}
			c.Attributes(song.ID, "tag", tag)
			rating := 5.0
			if i < 2 {
				rating = 9.5
			}
			c.Exec(`INSERT INTO metadata_ratings(item_id,provider,value,scale,votes,source_url,observed_at) VALUES(?,'tmdb',?,10,1,'','2026-01-01')`, song.ID, rating)
		}
		c.Drain()
		c.Exec(`ANALYZE`)
		v := Viewer{Profile: "p", Fence: "shape", Libraries: []string{"mu"}}
		for name, query := range queries {
			for _, pivot := range []string{"songs", "albums", "artists"} {
				var page BrowseResult
				n := readSteps(t, c, func(s *Service) error {
					var err error
					page, err = s.BrowseEntities(v, BrowseRequest{Library: "mu", Pivot: pivot, Query: query, Limit: 40})
					return err
				})
				if pivot == "songs" {
					want := map[string]int{"artist": 0, "album": 2, "tag": 2, "communityRating": 2}[name]
					if page.PageInfo.Total != want {
						t.Fatalf("%s: %d songs, want %d", name, page.PageInfo.Total, want)
					}
				}
				pair := steps[name+"/"+pivot]
				pair[size] = n
				steps[name+"/"+pivot] = pair
			}
		}
	}
	for key, pair := range steps {
		t.Logf("%s: %d steps at 80 songs, %d at 320", key, pair[0], pair[1])
		if pair[1] > pair[0]*3/2+500 {
			t.Errorf("%s grew from %d to %d steps with the library", key, pair[0], pair[1])
		}
	}
}

// Attribute filters test a term's key before reading its titles. The key is
// the value's lower-case trimmed text and must fold exactly as the value does.
func TestAttributeTermKeysFoldLikeTheirValues(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("m", "Movies", "movie", "/m")
	tags := [][]string{{"  Rock "}, {"RÖCK"}, {"Rock and Roll"}, {"Jazz"}, nil, {"ǅungla"}, {"STRASSE"}, {"Straße"}}
	ids := make([]string, len(tags))
	for i, values := range tags {
		item := c.Movie(lib, fmt.Sprintf("/m/%d.mkv", i), fmt.Sprintf("Film %d", i), 2000)
		ids[i] = item.Public
		if values != nil {
			c.Attributes(item.ID, "tag", values...)
		}
	}
	c.Drain()
	v := Viewer{Profile: "p", Fence: "fold", Libraries: []string{"m"}}
	for _, check := range []struct {
		op    string
		value any
		want  []int
	}{
		{"equals", "rock", []int{0, 1}},
		{"contains", "rock", []int{0, 1}},
		{"starts-with", "ro", []int{0, 1, 2}},
		{"not-equals", "rock", []int{2, 3, 4, 5, 6, 7}},
		{"in", []any{"jazz", "ǄUNGLA"}, []int{3, 5}},
		{"equals", "strasse", []int{6, 7}},
		{"is-missing", nil, []int{4}},
	} {
		page, err := New(c.DB).BrowseEntities(v, BrowseRequest{Library: "m", Pivot: "movies", Query: &BrowseNode{Field: "tag", Operator: check.op, Value: check.value}, Limit: 40})
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, entry := range page.Entries {
			got[entry.ID] = true
		}
		if len(got) != len(check.want) {
			t.Fatalf("%s %v: %d titles, want %d", check.op, check.value, len(got), len(check.want))
		}
		for _, index := range check.want {
			if !got[ids[index]] {
				t.Fatalf("%s %v: missing %q", check.op, check.value, tags[index])
			}
		}
	}
}
