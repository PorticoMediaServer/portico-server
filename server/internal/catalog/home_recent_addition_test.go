package catalog

import (
	"fmt"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
)

// Recently Added in Plex's shape: a show stands for what was added to it. One
// new episode reads as that episode, a season delivered together as the
// season, and several seasons at once as the show with its season count.
func TestRecentlyAddedNamesTheEpisodeOrSeasonThatArrived(t *testing.T) {
	c := catalogtest.Open(t)
	tv := c.Library("tv", "TV Shows", "tv", "/tv")
	n := 0
	episode := func(show, season catalogtest.Item, number int, added string) catalogtest.Item {
		n++
		path := fmt.Sprintf("/tv/%03d.mkv", n)
		it := c.Entity(compactcatalog.Entity{Library: tv, Kind: compactcatalog.Episode, Parent: season.ID, Key: compactcatalog.ItemKey("/tv", path, 0), Title: fmt.Sprintf("Episode %d", number), Added: added},
			map[string]any{"show_id": show.ID, "season_id": season.ID, "numbering": "seasonal", "number": number})
		c.File(it.ID, path, 2700)
		return it
	}
	// Weekly: three episodes in January, the fourth a week later.
	weekly := c.Show(tv, "Weekly", 2020)
	weeklyOne := c.Season(weekly, 1)
	for i := 1; i <= 3; i++ {
		episode(weekly, weeklyOne, i, fmt.Sprintf("2026-01-01T00:00:0%d.000Z", i))
	}
	newest := episode(weekly, weeklyOne, 4, "2026-01-08T20:00:00.000Z")
	// Season pack: season 1 in January, all of season 2 together in February.
	pack := c.Show(tv, "Season Pack", 2021)
	packOne, packTwo := c.Season(pack, 1), c.Season(pack, 2)
	episode(pack, packOne, 1, "2026-01-02T00:00:00.000Z")
	episode(pack, packOne, 2, "2026-01-02T00:00:01.000Z")
	for i := 1; i <= 3; i++ {
		episode(pack, packTwo, i, fmt.Sprintf("2026-02-01T00:00:0%d.000Z", i))
	}
	// Whole show: two seasons arriving in one scan.
	whole := c.Show(tv, "Whole Show", 2022)
	wholeOne, wholeTwo := c.Season(whole, 1), c.Season(whole, 2)
	for i := 1; i <= 2; i++ {
		episode(whole, wholeOne, i, fmt.Sprintf("2026-03-01T00:00:0%d.000Z", i))
		episode(whole, wholeTwo, i, fmt.Sprintf("2026-03-01T00:00:1%d.000Z", i))
	}
	c.Drain()
	s := New(c.DB)
	viewer := Viewer{Profile: "p", Libraries: []string{"tv"}}
	row, err := s.HomeSingleRow(HomeRequest{Viewer: viewer, ServerID: "server", Profile: "p", Libraries: []string{"tv"}}, "new_on_server", HomeRowPage{Limit: 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(row.Entries) != 3 {
		t.Fatalf("recently added: %d entries, want 3", len(row.Entries))
	}
	got := row.Entries
	if got[0].Kind != "show" || got[0].ID != whole.Public || got[0].SeasonCount == nil || *got[0].SeasonCount != 2 {
		t.Fatalf("several seasons at once read as the show with its seasons: %+v", got[0])
	}
	if got[1].Kind != "season" || got[1].ID != packTwo.Public || got[1].Subtitle != "Season Pack" || got[1].SeasonNumber == nil || *got[1].SeasonNumber != 2 || got[1].Navigation == nil || got[1].Navigation.View != "season" {
		t.Fatalf("a season delivered together reads as the season under its show: %+v", got[1])
	}
	if got[2].Kind != "episode" || got[2].ID != newest.Public || got[2].Subtitle != "Weekly" || got[2].EpisodeNumber == nil || *got[2].EpisodeNumber != 4 {
		t.Fatalf("one new episode reads as that episode: %+v", got[2])
	}
}
