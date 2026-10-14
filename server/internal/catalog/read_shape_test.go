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
		{Field: "artist", Operator: "equals", Value: "Nobody"},
		{Field: "artist", Operator: "equals", Value: "Someone"},
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
