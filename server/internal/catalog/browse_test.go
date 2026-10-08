package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
)

// browseFixture builds one movie library with enough descriptive, personal and
// availability facts to exercise every published operator.
func browseFixture(t *testing.T) (*sql.DB, *Service, catalogtest.Names) {
	t.Helper()
	c := catalogtest.Open(t)
	db := c.DB
	films := c.Library("a", "Movies", "movie", "/a")
	c.Library("tv", "Shows", "tv", "/tv")
	names := catalogtest.Names{}
	rows := []struct {
		id, title string
		year      int
	}{{"m1", "Alpha", 1995}, {"m2", "Beta", 2005}, {"m3", "Gamma", 2015}, {"m4", "Delta", 2015}, {"m5", "9 Lives", 1985}}
	for index, row := range rows {
		if row.id == "m4" {
			names[row.id] = c.Entity(compactcatalog.Entity{Library: films, Kind: compactcatalog.Movie, Key: compactcatalog.ItemKey("/a", "/a/m4.mkv", 0), Title: row.title, Year: row.year}, nil)
		} else {
			names[row.id] = c.Movie(films, "/a/"+row.id+".mkv", row.title, row.year)
		}
		c.Fields(names[row.id].ID, map[string]any{"added_text": fmt.Sprintf("2026-09-0%dT00:00:00.000Z", index+1)})
	}
	assets := []struct {
		item      string
		height    int
		duration  float64
		available bool
	}{{"m1", 1080, 100, true}, {"m2", 2160, 200, true}, {"m3", 480, 300, false}, {"m5", 720, 50, true}}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		for _, asset := range assets {
			if err := compactcatalog.SetAssetTx(ctx, tx, names[asset.item].Asset, map[string]any{"height": asset.height, "duration": asset.duration, "available": asset.available}); err != nil {
				return err
			}
		}
		return nil
	})
	c.Genres(names["m1"].ID, "tmdb", "Action", "Drama")
	c.Genres(names["m2"].ID, "tmdb", "Action")
	c.Genres(names["m3"].ID, "tmdb", "Comedy")
	c.Genres(names["m5"].ID, "tmdb", "Drama")
	for _, attribute := range [][3]string{{"m1", "contentRating", "PG"}, {"m2", "contentRating", "R"}, {"m1", "studio", "Example Pictures"}, {"m2", "tag", "rewatch"}} {
		c.Attributes(names[attribute[0]].ID, attribute[1], attribute[2])
	}
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,rating,revision,watched,last_played_at) VALUES('p',?,0,1,4.5,1,0,''),('p',?,0,0,NULL,1,1,'2026-09-10T00:00:00Z')`, names["m1"].ID, names["m2"].ID)
	c.Exec(`INSERT INTO personal_watched_intents(profile_id,item_id,watched,authored_at) VALUES('p',?,1,'2026-09-10T00:00:00.000000000Z')`, names["m2"].ID)
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES('p',?,?,0,0,'pb')`, names["m3"].ID, 10000)
	c.Drain()
	return db, New(db), names
}

func browseIDs(t *testing.T, s *Service, names catalogtest.Names, query string) []string {
	t.Helper()
	parsed, e := ParseBrowseQuery(json.RawMessage(query), "query")
	if e != nil {
		t.Fatalf("parse %s: %v", query, e)
	}
	out, e := s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Query: parsed, Limit: 50})
	if e != nil {
		t.Fatalf("browse %s: %v", query, e)
	}
	ids := []string{}
	for _, entry := range out.Entries {
		ids = append(ids, entry.ID)
	}
	sort.Strings(ids)
	ids = names.All(ids)
	sort.Strings(ids)
	return ids
}

func TestBrowseCompilesEveryOperator(t *testing.T) {
	_, s, names := browseFixture(t)
	cases := []struct {
		name, query string
		want        []string
	}{
		{"equals", `{"field":"title","operator":"equals","value":"Alpha"}`, []string{"m1"}},
		{"not-equals", `{"field":"title","operator":"not-equals","value":"Alpha"}`, []string{"m2", "m3", "m4", "m5"}},
		{"contains", `{"field":"title","operator":"contains","value":"amm"}`, []string{"m3"}},
		{"starts-with", `{"field":"title","operator":"starts-with","value":"D"}`, []string{"m4"}},
		{"in", `{"field":"year","operator":"in","value":[1995,2005]}`, []string{"m1", "m2"}},
		{"not-in", `{"field":"year","operator":"not-in","value":[1995,2005]}`, []string{"m3", "m4", "m5"}},
		{"less-than", `{"field":"year","operator":"less-than","value":2000}`, []string{"m1", "m5"}},
		{"at-most", `{"field":"year","operator":"at-most","value":1995}`, []string{"m1", "m5"}},
		{"greater-than", `{"field":"year","operator":"greater-than","value":2005}`, []string{"m3", "m4"}},
		{"at-least", `{"field":"year","operator":"at-least","value":2005}`, []string{"m2", "m3", "m4"}},
		{"between", `{"field":"year","operator":"between","value":[1995,2005]}`, []string{"m1", "m2"}},
		{"is-present", `{"field":"contentRating","operator":"is-present","value":null}`, []string{"m1", "m2"}},
		{"is-missing", `{"field":"contentRating","operator":"is-missing","value":null}`, []string{"m3", "m4", "m5"}},
		{"contains-any", `{"field":"genre","operator":"contains-any","value":["Action","Comedy"]}`, []string{"m1", "m2", "m3"}},
		{"contains-all", `{"field":"genre","operator":"contains-all","value":["Action","Drama"]}`, []string{"m1"}},
		{"set contains", `{"field":"genre","operator":"contains","value":"Drama"}`, []string{"m1", "m5"}},
		{"decade", `{"field":"decade","operator":"equals","value":2010}`, []string{"m3", "m4"}},
		{"availability", `{"field":"availability","operator":"equals","value":"available"}`, []string{"m1", "m2", "m5"}},
		{"unavailable", `{"field":"availability","operator":"equals","value":"unavailable"}`, []string{"m3", "m4"}},
		{"resolution", `{"field":"resolution","operator":"equals","value":"4k"}`, []string{"m2"}},
		{"duration", `{"field":"durationSeconds","operator":"at-least","value":200}`, []string{"m2", "m3"}},
		{"favorite", `{"field":"favorite","operator":"equals","value":true}`, []string{"m1"}},
		{"not favorite", `{"field":"favorite","operator":"equals","value":false}`, []string{"m2", "m3", "m4", "m5"}},
		{"played", `{"field":"playState","operator":"equals","value":"played"}`, []string{"m2"}},
		{"in-progress", `{"field":"playState","operator":"equals","value":"in-progress"}`, []string{"m3"}},
		{"unplayed", `{"field":"playState","operator":"equals","value":"unplayed"}`, []string{"m1", "m4", "m5"}},
		{"personalRating", `{"field":"personalRating","operator":"at-least","value":4}`, []string{"m1"}},
		{"lastPlayedAt present", `{"field":"lastPlayedAt","operator":"is-present","value":null}`, []string{"m2"}},
		{"studio", `{"field":"studio","operator":"contains","value":"Example"}`, []string{"m1"}},
		{"tag", `{"field":"tag","operator":"contains","value":"rewatch"}`, []string{"m2"}},
		{"all group", `{"all":[{"field":"genre","operator":"contains","value":"Action"},{"field":"year","operator":"at-least","value":2000}]}`, []string{"m2"}},
		{"any group", `{"any":[{"field":"title","operator":"equals","value":"Alpha"},{"field":"title","operator":"equals","value":"Delta"}]}`, []string{"m1", "m4"}},
		{"not group", `{"not":{"field":"genre","operator":"contains","value":"Action"}}`, []string{"m3", "m4", "m5"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := browseIDs(t, s, names, test.query)
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("want %v got %v", test.want, got)
			}
		})
	}
}

func TestBrowseRejectsQueriesOverTheLimits(t *testing.T) {
	deep := `{"field":"title","operator":"equals","value":"Alpha"}`
	for i := 0; i < 6; i++ {
		deep = `{"not":` + deep + `}`
	}
	if _, e := ParseBrowseQuery(json.RawMessage(deep), "query"); e == nil {
		t.Fatal("depth accepted")
	} else {
		var issue *BrowseValidationError
		if !errors.As(e, &issue) || !strings.HasPrefix(issue.Path, "query.not") {
			t.Fatalf("depth path %v", e)
		}
	}
	clauses := []string{}
	for i := 0; i < BrowseMaximumClauses+1; i++ {
		clauses = append(clauses, `{"field":"year","operator":"equals","value":`+fmt.Sprint(1900+i)+`}`)
	}
	if _, e := ParseBrowseQuery(json.RawMessage(`{"any":[`+strings.Join(clauses, ",")+`]}`), "query"); e == nil {
		t.Fatal("clause count accepted")
	}
	for _, bad := range []struct{ query, path string }{
		{`{"field":"nonsense","operator":"equals","value":"x"}`, "query.field"},
		{`{"field":"title","operator":"at-least","value":"x"}`, "query.operator"},
		{`{"field":"year","operator":"equals","value":"x"}`, "query.value"},
		{`{"field":"playState","operator":"equals","value":"halfway"}`, "query.value"},
		{`{"all":[{"field":"year","operator":"between","value":[1]}]}`, "query.all[0].value"},
		{`{"all":[],"any":[]}`, "query"},
	} {
		_, e := ParseBrowseQuery(json.RawMessage(bad.query), "query")
		var issue *BrowseValidationError
		if !errors.As(e, &issue) || issue.Path != bad.path {
			t.Fatalf("query %s wanted path %s got %v", bad.query, bad.path, e)
		}
	}
}

func TestBrowseQuickFiltersRunOnTheEngine(t *testing.T) {
	_, s, names := browseFixture(t)
	expected := map[string][]string{"in-progress": {"m3"}, "unwatched": {"m1", "m4", "m5"}, "watched": {"m2"}, "favorites": {"m1"}, "watchlist": {}, "available": {"m1", "m2", "m5"}, "missing": {"m3", "m4"}}
	for _, filter := range BrowseQuickFilters() {
		raw, e := json.Marshal(filter.Query)
		if e != nil {
			t.Fatal(e)
		}
		got := browseIDs(t, s, names, string(raw))
		want, ok := expected[filter.ID]
		if !ok {
			t.Fatalf("unexpected quick filter %s", filter.ID)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s want %v got %v", filter.ID, want, got)
		}
	}
}

func TestBrowsePositionIndexAndSeek(t *testing.T) {
	_, s, names := browseFixture(t)
	page, e := s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Limit: 2})
	if e != nil {
		t.Fatal(e)
	}
	want := []BrowsePositionAnchor{{"#", 0}, {"A", 1}, {"B", 2}, {"D", 3}, {"G", 4}}
	if fmt.Sprint(page.PositionIndex) != fmt.Sprint(want) {
		t.Fatalf("index %v", page.PositionIndex)
	}
	if page.PageInfo.Total != 5 || !page.PageInfo.HasMore || page.PageInfo.NextCursor == "" {
		t.Fatalf("page %+v", page.PageInfo)
	}
	sought, e := s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Limit: 2, Seek: &BrowseSeek{Prefix: "d"}})
	if e != nil {
		t.Fatal(e)
	}
	if sought.PageInfo.Start != 3 || len(sought.Entries) != 2 || sought.Entries[0].ID != names["m4"].Public {
		t.Fatalf("seek %+v %v", sought.PageInfo, sought.Entries)
	}
	if _, e = s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Sort: []BrowseSortSelection{{Field: "year", Direction: "desc"}}, Seek: &BrowseSeek{Prefix: "d"}}); e == nil {
		t.Fatal("seek accepted without a title sort")
	}
	descending, e := s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Sort: []BrowseSortSelection{{Field: "title", Direction: "desc"}}, Limit: 5})
	if e != nil {
		t.Fatal(e)
	}
	// M10: every sort publishes anchors; a descending title sort runs its
	// letters backwards, each at the index where its run begins.
	if len(descending.PositionIndex) == 0 || descending.PositionIndex[0].Index != 0 {
		t.Fatalf("descending letter index %+v", descending.PositionIndex)
	}
	for i := 1; i < len(descending.PositionIndex); i++ {
		a, b := descending.PositionIndex[i-1], descending.PositionIndex[i]
		if b.Index <= a.Index || b.Key >= a.Key {
			t.Fatalf("descending letter index out of order %+v", descending.PositionIndex)
		}
	}
}

func TestBrowseCursorPagesTheWholeLibrary(t *testing.T) {
	_, s, _ := browseFixture(t)
	request := BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Limit: 2}
	seen := map[string]bool{}
	for {
		page, e := s.BrowseEntities(Viewer{Profile: request.Profile, Fence: request.ViewerFence, Libraries: []string{request.Library}, Restrictions: request.Restrictions}, request)
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range page.Entries {
			if seen[entry.ID] {
				t.Fatal("duplicate", entry.ID)
			}
			seen[entry.ID] = true
		}
		if page.PageInfo.NextCursor == "" {
			break
		}
		request.Cursor = page.PageInfo.NextCursor
	}
	if len(seen) != 5 {
		t.Fatal("missing rows", len(seen))
	}
}

func TestBrowseRangeReanchorsAfterAPublication(t *testing.T) {
	db, s, names := browseFixture(t)
	page, e := s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Limit: 2, Range: &BrowseRange{Start: 3}})
	if e != nil {
		t.Fatal(e)
	}
	if page.PageInfo.Start != 3 || page.Entries[0].ID != names["m4"].Public {
		t.Fatalf("range %+v", page.PageInfo)
	}
	revision := page.PageInfo.Revision
	catalog := catalogtest.New(t, db)
	names["m0"] = catalog.Movie(catalog.Handle("a"), "/a/m0.mkv", "AAA", 2001)
	settleCompact(t, db)
	if _, e = s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Limit: 2, Range: &BrowseRange{Start: 3, Revision: revision}}); !errors.Is(e, ErrStaleContinuation) {
		t.Fatalf("stale range admitted %v", e)
	}
	moved, e := s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Limit: 2, Range: &BrowseRange{Start: 3, Revision: revision, AnchorID: names["m4"].Public}})
	if e != nil {
		t.Fatal(e)
	}
	if moved.PageInfo.Start != 4 || moved.Entries[0].ID != names["m4"].Public {
		t.Fatalf("anchor %+v %v", moved.PageInfo, moved.Entries)
	}
	if _, e = s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Limit: 2, Range: &BrowseRange{Start: 3, Revision: revision, AnchorID: "missing"}}); !errors.Is(e, ErrStaleContinuation) {
		t.Fatalf("dead anchor admitted %v", e)
	}
}

func TestBrowseCapabilitiesPublishOnlyExecutableVocabulary(t *testing.T) {
	_, s, _ := browseFixture(t)
	capabilities, e := s.BrowseCapabilitiesFor("a", "")
	if e != nil {
		t.Fatal(e)
	}
	ids := []string{}
	for _, pivot := range capabilities.Pivots {
		ids = append(ids, pivot.ID)
	}
	if strings.Join(ids, ",") != "discover,movies,collections,categories" {
		t.Fatalf("pivots %v", ids)
	}
	if capabilities.QueryLimits.MaximumDepth != 5 || capabilities.QueryLimits.MaximumClauses != 40 || capabilities.QueryLimits.CursorTTLSeconds != 1800 {
		t.Fatalf("limits %+v", capabilities.QueryLimits)
	}
	for _, field := range capabilities.Fields {
		if field.ControlHint == "" || field.Complexity == "" || field.Cost == "" || len(field.Operators) == 0 {
			t.Fatalf("incomplete field %+v", field)
		}
		if _, ok := browseFieldByID(field.ID); !ok {
			t.Fatalf("unknown field %s", field.ID)
		}
	}
	scoped, e := s.BrowseCapabilitiesFor("a", "collections")
	if e != nil {
		t.Fatal(e)
	}
	if scoped.ResolvedPivot == nil || scoped.ResolvedPivot.ID != "collections" {
		t.Fatalf("resolved %+v", scoped.ResolvedPivot)
	}
	for _, field := range scoped.Fields {
		if field.ID == "network" {
			t.Fatal("network published for a movie collection pivot")
		}
	}
	if _, e = s.BrowseCapabilitiesFor("a", "songs"); e == nil {
		t.Fatal("foreign pivot accepted")
	}
	television, e := s.BrowseCapabilitiesFor("tv", "")
	if e != nil {
		t.Fatal(e)
	}
	ids = ids[:0]
	for _, pivot := range television.Pivots {
		ids = append(ids, pivot.ID)
	}
	if strings.Join(ids, ",") != "discover,shows,episodes,collections,categories" {
		t.Fatalf("television pivots %v", ids)
	}
}

func TestFacetsRespectVisibilityAndCacheByRevision(t *testing.T) {
	db, s, names := browseFixture(t)
	page, e := s.Facets(Viewer{Profile: "p", Libraries: []string{"a"}}, FacetRequest{Library: "a", Profile: "p", Field: "genre"})
	if e != nil {
		t.Fatal(e)
	}
	counts := map[string]int{}
	for _, value := range page.Values {
		counts[value.Value] = value.Count
	}
	if counts["Action"] != 2 || counts["Drama"] != 2 || counts["Comedy"] != 1 {
		t.Fatalf("counts %+v", counts)
	}
	if _, e = db.Exec(`INSERT INTO dvr_catalog_retirements(item_id,requested_ms) VALUES(?,0)`, names["m1"].ID); e != nil {
		t.Fatal(e)
	}
	settleCompact(t, db)
	page, e = s.Facets(Viewer{Profile: "p", Libraries: []string{"a"}}, FacetRequest{Library: "a", Profile: "p", Field: "genre"})
	if e != nil {
		t.Fatal(e)
	}
	counts = map[string]int{}
	for _, value := range page.Values {
		counts[value.Value] = value.Count
	}
	if counts["Action"] != 1 || counts["Drama"] != 1 {
		t.Fatalf("retired row still counted %+v", counts)
	}
	decades, e := s.Facets(Viewer{Profile: "p", Libraries: []string{"a"}}, FacetRequest{Library: "a", Profile: "p", Field: "decade"})
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, value := range decades.Values {
		if value.Value == "2010" && value.Label == "2010s" && value.Count == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("decades %+v", decades.Values)
	}
	if _, e = s.Facets(Viewer{Profile: "p", Libraries: []string{"a"}}, FacetRequest{Library: "a", Profile: "p", Field: "nonsense"}); e == nil {
		t.Fatal("unknown facet accepted")
	}
	filtered, e := s.Facets(Viewer{Profile: "p", Libraries: []string{"a"}}, FacetRequest{Library: "a", Profile: "p", Field: "genre", Q: "dra"})
	if e != nil {
		t.Fatal(e)
	}
	if len(filtered.Values) != 1 || filtered.Values[0].Value != "Drama" {
		t.Fatalf("filtered %+v", filtered.Values)
	}
}

func TestSavedViewValidationNamesTheFailingPath(t *testing.T) {
	db, s, _ := browseFixture(t)
	_ = s
	good := SavedViewDefinition{LibraryID: "a", Pivot: "movies", Presentation: "grid", Sort: []BrowseSortSelection{{Field: "title", Direction: "asc"}}, Query: &BrowseNode{Field: "genre", Operator: "contains", Value: "Action"}}
	if bad := validateSavedView(db, good); len(bad) != 0 {
		t.Fatalf("valid view rejected %v", bad)
	}
	for _, test := range []struct {
		name string
		path string
		make func(SavedViewDefinition) SavedViewDefinition
	}{
		{"library", "libraryId", func(v SavedViewDefinition) SavedViewDefinition { v.LibraryID = "nope"; return v }},
		{"pivot", "pivot", func(v SavedViewDefinition) SavedViewDefinition { v.Pivot = "songs"; return v }},
		{"presentation", "presentation", func(v SavedViewDefinition) SavedViewDefinition { v.Presentation = "carousel"; return v }},
		{"sort field", "sort[0].field", func(v SavedViewDefinition) SavedViewDefinition {
			v.Sort = []BrowseSortSelection{{Field: "nonsense", Direction: "asc"}}
			return v
		}},
		{"query field", "query.field", func(v SavedViewDefinition) SavedViewDefinition {
			v.Query = &BrowseNode{Field: "nonsense", Operator: "equals", Value: "x"}
			return v
		}},
	} {
		bad := validateSavedView(db, test.make(good))
		if len(bad) == 0 || bad[0] != test.path {
			t.Fatalf("%s wanted %s got %v", test.name, test.path, bad)
		}
	}
	// A whole expression survives the store round trip unchanged.
	raw, e := json.Marshal(good)
	if e != nil {
		t.Fatal(e)
	}
	var restored SavedViewDefinition
	if e = json.Unmarshal(raw, &restored); e != nil {
		t.Fatal(e)
	}
	if restored.Query == nil || restored.Query.Field != "genre" || restored.Query.Value != "Action" {
		t.Fatalf("round trip %+v", restored.Query)
	}
	if bad := validateSavedView(db, restored); len(bad) != 0 {
		t.Fatalf("restored view rejected %v", bad)
	}
}

func TestPinOrderAndPinnedLibraries(t *testing.T) {
	db, s, _ := browseFixture(t)
	actor := ResourceActor{Authority: "local", AccountID: "account", ProfileID: "profile"}
	owner := actorKey(actor)
	for _, name := range []string{"Zulu", "Alpha", "Mike"} {
		if _, e := db.Exec(`INSERT INTO saved_resources(id,kind,owner_key,owner_authority,owner_account,owner_profile,name,created_at) VALUES(?,'view',?,?,?,?,?,'2026-09-01T00:00:00Z')`, name, owner, actor.Authority, actor.AccountID, actor.ProfileID, name); e != nil {
			t.Fatal(e)
		}
		if _, e := db.Exec(`INSERT INTO saved_pins VALUES(?,'view',?,1,1)`, owner, name); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := s.SetPinOrder(actor, 1, []PinOrderEntry{{"view", "Zulu"}}, func(*sql.Tx) error { return nil }); !errors.Is(e, ErrPinOrderConflict) {
		t.Fatalf("stale revision accepted %v", e)
	}
	order, e := s.SetPinOrder(actor, 0, []PinOrderEntry{{"view", "Zulu"}, {"view", "Mike"}}, func(*sql.Tx) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	if order.Revision != 1 {
		t.Fatalf("revision %d", order.Revision)
	}
	if _, e = s.SetPinOrder(actor, 1, []PinOrderEntry{{"view", "unknown"}}, func(*sql.Tx) error { return nil }); e == nil {
		t.Fatal("unpinned resource accepted into the order")
	}
	page, e := s.SavedResources("server", "fence", actor, "view", "", true, 10)
	if e != nil {
		t.Fatal(e)
	}
	names := []string{}
	for _, resource := range page.Resources {
		names = append(names, resource.Name)
	}
	if strings.Join(names, ",") != "Zulu,Mike,Alpha" {
		t.Fatalf("pin order not applied: %v", names)
	}
	read, e := s.PinOrder(actor)
	if e != nil {
		t.Fatal(e)
	}
	if len(read.Order) != 2 || read.Order[0].ID != "Zulu" {
		t.Fatalf("stored order %+v", read.Order)
	}
}

func TestPinnedLibrariesLeadTheListing(t *testing.T) {
	_, s, _ := browseFixture(t)
	actor := ResourceActor{Authority: "local", AccountID: "account", ProfileID: "profile"}
	navigation, e := s.LibraryNavigation(actor, nil)
	if e != nil || navigation.Revision != 0 || len(navigation.PinnedLibraryIDs) != 0 {
		t.Fatalf("empty navigation %+v %v", navigation, e)
	}
	if _, e = s.SetLibraryNavigation(actor, 1, []string{"tv"}, func(*sql.Tx, string) error { return nil }); !errors.Is(e, ErrLibraryNavigationConflict) {
		t.Fatalf("stale revision accepted %v", e)
	}
	saved, e := s.SetLibraryNavigation(actor, 0, []string{"tv"}, func(*sql.Tx, string) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	if saved.Revision != 1 || saved.PinnedLibraryIDs[0] != "tv" {
		t.Fatalf("saved %+v", saved)
	}
	if _, e = s.SetLibraryNavigation(actor, 1, []string{"tv", "tv"}, func(*sql.Tx, string) error { return nil }); e == nil {
		t.Fatal("duplicate library accepted")
	}
	libraries, e := s.LibrariesForViewer(actor)
	if e != nil {
		t.Fatal(e)
	}
	if libraries[0].ID != "tv" || !libraries[0].Pinned || libraries[1].ID != "a" || libraries[1].Pinned {
		t.Fatalf("ordering %+v", libraries)
	}
}

func TestAggregatePivotProjectsFacets(t *testing.T) {
	_, s, _ := browseFixture(t)
	page, e := s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "categories"})
	if e != nil {
		t.Fatal(e)
	}
	if page.Pivot != "categories" || len(page.Entries) == 0 {
		t.Fatalf("categories %+v", page)
	}
	for _, entry := range page.Entries {
		if entry.Kind != "category" || entry.Navigation == nil || !strings.HasPrefix(entry.Navigation.Category, "decade:") {
			t.Fatalf("entry %+v", entry)
		}
	}
	if _, e = s.BrowseEntities(Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "categories", Query: &BrowseNode{Field: "title", Operator: "equals", Value: "x"}}); e == nil {
		t.Fatal("aggregate pivot accepted an expression")
	}
}
