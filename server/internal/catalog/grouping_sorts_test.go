package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
)

func walkBrowse(t *testing.T, s *Service, v Viewer, r BrowseRequest) []ContentEntry {
	t.Helper()
	out := []ContentEntry{}
	seen := map[string]bool{}
	for pages := 0; pages < 1000; pages++ {
		p, err := s.BrowseEntities(v, r)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range p.Entries {
			if seen[entry.ID] {
				t.Fatalf("duplicate %s", entry.ID)
			}
			seen[entry.ID] = true
			out = append(out, entry)
		}
		if p.PageInfo.NextCursor == "" {
			return out
		}
		if len(p.Entries) == 0 {
			t.Fatal("empty continuation")
		}
		r.Cursor = p.PageInfo.NextCursor
	}
	t.Fatal("unbounded cursor walk")
	return nil
}

func TestGroupingTitleSortsUseExactKeysAndRestrictedKeysets(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("m", "Movies", "movie", "/m")
	for i := 0; i < 23; i++ {
		item := c.Movie(library, fmt.Sprintf("/m/%d.mkv", i), fmt.Sprintf("Harbor %d", i), 2000+i%4)
		c.Fields(item.ID, map[string]any{"release_date": fmt.Sprintf("2020-01-%02d", 1+i%6), "added_text": fmt.Sprintf("2026-01-%02dT00:00:00.000Z", 1+i%5)})
		c.Genres(item.ID, "fixture", "Drama")
		if i%4 == 0 {
			c.Attributes(item.ID, "contentRating", "R")
		}
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			return compactcatalog.SetAssetTx(ctx, tx, item.Asset, map[string]any{"size": int64(100 + i%3), "height": 480 + 240*(i%4)})
		})
		c.Exec(`INSERT INTO metadata_ratings(item_id,provider,value,scale,votes,source_url,observed_at) VALUES(?,'metacritic',?,10,0,'','now')`, item.ID, float64(i%5))
		c.Exec(`INSERT INTO personal_play_counts(profile_id,item_id,plays) VALUES('p',?,?)`, item.ID, i%7)
	}
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('r',17)`)
	c.Drain()
	s := New(c.DB)
	restricted := classesRestrictionOf(classesCeiling(13), false)
	for _, v := range []Viewer{{Profile: "p", Fence: "open", Libraries: []string{"m"}}, {Profile: "p", Fence: "restricted", Libraries: []string{"m"}, Restrictions: restricted}} {
		for _, field := range []string{"releaseDate", "criticRating", "fileSize", "resolution", "playCount", "random"} {
			for _, direction := range []string{"asc", "desc"} {
				r := BrowseRequest{Library: "m", Pivot: "movies", Query: &BrowseNode{Field: "genre", Operator: "equals", Value: "Drama"}, Sort: []BrowseSortSelection{{Field: field, Direction: direction}}, Limit: 3}
				got := entryIDs(walkBrowse(t, s, v, r))
				r.Limit = 100
				want := entryIDs(walkBrowse(t, s, v, r))
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s %s %s: %v != %v", v.Fence, field, direction, got, want)
				}
				if len(got) != 23 && !v.Restrictions.Active() || len(got) != 17 && v.Restrictions.Active() {
					t.Fatalf("visible count %s %d", v.Fence, len(got))
				}
				// Independently verify the indexed scalar order, including duplicate keys.
				if field != "playCount" && field != "random" {
					def, _ := browseSortByID(field)
					restriction, args := EntityRestrictionSQL("e.entity_id", v.Restrictions)
					rows, err := c.DB.Query(`SELECT pid(ce.public_id) FROM catalog_browse_rows e JOIN catalog_entities ce ON ce.id=e.entity_id WHERE e.kind=1 AND `+restriction+` ORDER BY `+def.Expression+` `+direction+`,e.entity_id `+direction, args...)
					if err != nil {
						t.Fatal(err)
					}
					expected := []string{}
					for rows.Next() {
						var id string
						if err = rows.Scan(&id); err != nil {
							t.Fatal(err)
						}
						expected = append(expected, id)
					}
					rows.Close()
					if !reflect.DeepEqual(got, expected) {
						t.Fatalf("%s %s wrong order", field, direction)
					}
				}
			}
		}
	}
	v := Viewer{Profile: "p", Fence: "open", Libraries: []string{"m"}}
	r := BrowseRequest{Library: "m", Pivot: "movies", Sort: []BrowseSortSelection{{Field: "random", Direction: "asc", Seed: 2000000000}}, Limit: 4}
	a := entryIDs(walkBrowse(t, s, v, r))
	r.Limit = 100
	b := entryIDs(walkBrowse(t, s, v, r))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("seed changes with page size")
	}
	r.Sort[0].Seed = 0
	if reflect.DeepEqual(a, entryIDs(walkBrowse(t, s, v, r))) {
		t.Fatal("seed did not rotate order")
	}
}

func TestGroupingCountActivityAndFilters(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("m", "Movies", "movie", "/m")
	for i := 0; i < 8; i++ {
		item := c.Movie(library, fmt.Sprintf("/m/%d.mkv", i), fmt.Sprintf("Film %d", i), 1990+i)
		c.Fields(item.ID, map[string]any{"added_text": fmt.Sprintf("2026-01-%02dT00:00:00.000Z", 1+i)})
		if i < 3 {
			c.Attributes(item.ID, "contentRating", "R")
		}
	}
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('r',17)`)
	c.Drain()
	s := New(c.DB)
	for _, v := range []Viewer{{Profile: "p", Fence: "o", Libraries: []string{"m"}}, {Profile: "p", Fence: "r", Libraries: []string{"m"}, Restrictions: classesRestrictionOf(classesCeiling(13), false)}} {
		caps, err := s.BrowseCapabilitiesFor("m", "categories")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(caps.ResolvedPivot.Sorts, []string{"title", "count", "recentActivity"}) || len(caps.Fields) == 0 {
			t.Fatalf("capabilities %+v", caps)
		}
		for _, field := range []string{"title", "count", "recentActivity"} {
			for _, direction := range []string{"asc", "desc"} {
				entries := walkBrowse(t, s, v, BrowseRequest{Library: "m", Pivot: "categories", Sort: []BrowseSortSelection{{Field: field, Direction: direction}}, Limit: 1})
				if len(entries) != 1 || entries[0].Count == nil {
					t.Fatalf("groups %+v", entries)
				}
				count := 8
				if v.Restrictions.Active() {
					count = 5
				}
				if *entries[0].Count != count {
					t.Fatalf("restricted count %d", *entries[0].Count)
				}
			}
		}
		entries := walkBrowse(t, s, v, BrowseRequest{Library: "m", Pivot: "categories", Query: &BrowseNode{Field: "year", Operator: "at-least", Value: float64(1995)}, Sort: []BrowseSortSelection{{Field: "count", Direction: "desc"}}, Limit: 1})
		if len(entries) != 1 || *entries[0].Count != 3 {
			t.Fatalf("filtered groups %+v", entries)
		}
	}
}

func TestGroupingScopesRespectListsAndHiddenMembers(t *testing.T) {
	c := catalogtest.Open(t)
	movies := c.Library("m", "Movies", "movie", "/m")
	visible := c.Movie(movies, "/m/a.mkv", "Harbor", 2000)
	hidden := c.Movie(movies, "/m/b.mkv", "Harbor Adult", 2000)
	c.Attributes(hidden.ID, "contentRating", "R")
	c.Attributes(visible.ID, "subtitleLanguage", "fr")
	collection := c.Collection(movies, "Harbor collection", visible, hidden)
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('r',17)`)
	c.Exec(`INSERT INTO catalog_playlists(token,owner_authority,owner_account,owner_profile,name,creation_operation,creation_hash,created_at) VALUES('list','local','account','profile','Private','op','hash','now')`)
	c.Exec(`INSERT INTO catalog_playlist_entries(token,playlist_id,item_id,position) SELECT 'entry',id,?,1 FROM catalog_playlists WHERE token='list'`, visible.ID)
	c.Drain()
	s := New(c.DB)
	owner := identity.PersonalKey(identity.Viewer{Authority: "local", AccountID: "account", ProfileID: "profile"})
	for _, tc := range []struct {
		profile, field, op string
		value              any
		want               int
	}{{owner, "list", "equals", "list", 1}, {"unrelated", "list", "equals", "list", 0}, {owner, "collection", "equals", collection.Public, 1}, {owner, "memberOf", "equals", collection.Public, 1}, {owner, "subtitleLanguage", "equals", "fr", 1}, {owner, "versions", "equals", float64(1), 1}, {owner, "search", "contains", "Harbor", 1}} {
		v := Viewer{Profile: tc.profile, Fence: tc.profile, Libraries: []string{"m"}, Restrictions: classesRestrictionOf(classesCeiling(13), false)}
		got := walkBrowse(t, s, v, BrowseRequest{Library: "m", Pivot: "movies", Query: &BrowseNode{Field: tc.field, Operator: tc.op, Value: tc.value}, Sort: []BrowseSortSelection{{Field: "fileSize", Direction: "desc"}}, Limit: 1})
		if len(got) != tc.want {
			t.Fatalf("%s %s: %+v", tc.profile, tc.field, got)
		}
	}
}
