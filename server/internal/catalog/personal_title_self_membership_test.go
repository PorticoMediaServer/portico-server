package catalog

import (
	"fmt"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
	"reflect"
	"sort"
	"testing"
)

// Direct items and containers read the same viewer's saved facts. A container
// can additionally match its own state or a permitted member; a hidden member
// and another viewer's state must not supply a match.
func TestPersonalTitleSelfMembershipPreservesPolarityAndBindings(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("music", "Music", "music", "/music")
	artist := c.Artist(lib, "Artist")
	a := c.Album(artist, "A", 2000)
	b := c.Album(artist, "B", 2000)
	other := c.Album(artist, "C", 2000)
	s1 := c.Song(a, 1, "/music/a.flac", "One")
	s3 := c.Song(b, 1, "/music/b.flac", "Three")
	s4 := c.Song(other, 1, "/music/c.flac", "Four")
	hidden := c.Song(other, 2, "/music/hidden.flac", "Hidden")
	c.Attributes(hidden.ID, "label", "private")
	for _, row := range []struct {
		profile          string
		id               int64
		favorite, rating int
	}{
		{"p", a.ID, 0, 4}, {"p", b.ID, 1, 9}, {"p", s1.ID, 1, 8}, {"p", hidden.ID, 1, 10}, {"q", s3.ID, 1, 8},
	} {
		c.Exec(`INSERT INTO personal_items(profile_id,item_id,favorite,watchlisted,rating,revision) VALUES(?,?,?,?,?,1)`, row.profile, row.id, row.favorite, row.favorite, row.rating)
	}
	c.Drain()
	service := New(c.DB)
	viewer := Viewer{Profile: "p", Fence: "p-authority", Libraries: []string{"music"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"private"}}}
	names := map[string]string{a.Public: "A", b.Public: "B", other.Public: "C", s1.Public: "One", s3.Public: "Three", s4.Public: "Four", hidden.Public: "Hidden"}
	cases := []struct {
		name, field, operator string
		value                 any
		songs, albums         []string
	}{
		{"favorite", "favorite", "equals", true, []string{"One"}, []string{"A", "B"}},
		{"not favorite", "favorite", "equals", false, []string{"Four", "Three"}, []string{"C"}},
		{"favorite not true", "favorite", "not-equals", true, []string{"Four", "Three"}, []string{"C"}},
		{"favorite not false", "favorite", "not-equals", false, []string{"One"}, []string{"A", "B"}},
		{"watchlisted", "watchlisted", "equals", true, []string{"One"}, []string{"A", "B"}},
		{"rating threshold", "personalRating", "at-least", float64(7), []string{"One"}, []string{"A", "B"}},
		{"rating low", "personalRating", "at-most", float64(4), []string{}, []string{"A"}},
		{"rating negative", "personalRating", "not-equals", float64(8), []string{"Four", "Three"}, []string{"B", "C"}},
		{"rating in", "personalRating", "in", []any{float64(4), float64(8)}, []string{"One"}, []string{"A"}},
		{"rating not in", "personalRating", "not-in", []any{float64(4), float64(8)}, []string{"Four", "Three"}, []string{"B", "C"}},
		{"rating range", "personalRating", "between", []any{float64(7), float64(9)}, []string{"One"}, []string{"A", "B"}},
		{"rating present", "personalRating", "is-present", nil, []string{"One"}, []string{"A", "B"}},
		{"rating absent", "personalRating", "is-missing", nil, []string{"Four", "Three"}, []string{"C"}},
	}
	for _, tc := range cases {
		for _, pivot := range []string{"songs", "albums"} {
			t.Run(tc.name+"/"+pivot, func(t *testing.T) {
				page, e := service.BrowseEntities(viewer, BrowseRequest{Library: "music", Pivot: pivot, Limit: 40, Query: &BrowseNode{Field: tc.field, Operator: tc.operator, Value: tc.value}})
				if e != nil {
					t.Fatal(e)
				}
				got := []string{}
				for _, entry := range page.Entries {
					got = append(got, names[entry.ID])
				}
				sort.Strings(got)
				want := tc.songs
				if pivot == "albums" {
					want = tc.albums
				}
				if !reflect.DeepEqual(got, want) || page.PageInfo.Total != len(want) {
					t.Fatalf("viewer-bound %s: got %v total%d want%v", pivot, got, page.PageInfo.Total, want)
				}
			})
		}
	}
	viewer.Profile = "q"
	viewer.Fence = "q-authority"
	for _, pivot := range []string{"songs", "albums"} {
		page, e := service.BrowseEntities(viewer, BrowseRequest{Library: "music", Pivot: pivot, Query: &BrowseNode{Field: "favorite", Operator: "equals", Value: true}, Limit: 40})
		want := "Three"
		if pivot == "albums" {
			want = "B"
		}
		if e != nil || len(page.Entries) != 1 || names[page.Entries[0].ID] != want {
			t.Fatal(fmt.Sprintf("other viewer %s: %+v %v", pivot, page, e))
		}
	}
}
