package catalog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
)

// within-last-days is the relative recency form: its window is measured from
// the catalogue clock, so a saved view keeps meaning "the last N days".
func TestBrowseWithinLastDaysWindowBoundaries(t *testing.T) {
	db, s, names := browseFixture(t)
	c := catalogtest.New(t, db)
	added := map[string]string{
		"m1": "2026-10-16T12:00:00.000Z", // exactly now - 7 days: included
		"m2": "2026-10-16T11:59:59.999Z", // one millisecond earlier: excluded
		"m3": "2026-10-23T12:00:00.000Z", // exactly now: included
		"m4": "2026-10-23T12:00:00.001Z", // one millisecond in the future: excluded
		"m5": "2026-10-01T00:00:00.000Z", // outside seven days
	}
	for name, value := range added {
		c.Fields(names[name].ID, map[string]any{"added_text": value})
	}
	c.Drain()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s.Clock = func() time.Time { return now }

	got := browseIDs(t, s, names, `{"field":"dateAdded","operator":"within-last-days","value":7}`)
	if strings.Join(got, ",") != "m1,m3" {
		t.Fatalf("seven-day window: want m1,m3 got %v", got)
	}
	// The same titles at a wider window: the 1 October title is within 30 days.
	got = browseIDs(t, s, names, `{"field":"dateAdded","operator":"within-last-days","value":30}`)
	if strings.Join(got, ",") != "m1,m3,m5" {
		t.Fatalf("thirty-day window: want m1,m3,m5 got %v", got)
	}
}

func TestBrowseWithinLastDaysEveryDateField(t *testing.T) {
	db, s, names := browseFixture(t)
	c := catalogtest.New(t, db)
	c.Fields(names["m1"].ID, map[string]any{"added_text": "2026-10-20T00:00:00.000Z"})
	c.Attributes(names["m2"].ID, "releaseDate", "2026-10-20")
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,revision,last_played_at) VALUES('p',?,1,'2026-10-20T00:00:00Z')
 ON CONFLICT(profile_id,item_id) DO UPDATE SET last_played_at=excluded.last_played_at`, names["m3"].ID)
	c.Drain()
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	s.Clock = func() time.Time { return now }
	for _, test := range []struct {
		field, want string
	}{
		{"dateAdded", "m1"},
		{"releaseDate", "m2"},
		{"lastPlayedAt", "m3"},
	} {
		t.Run(test.field, func(t *testing.T) {
			got := browseIDs(t, s, names, fmt.Sprintf(`{"field":%q,"operator":"within-last-days","value":7}`, test.field))
			if strings.Join(got, ",") != test.want {
				t.Fatalf("want %s got %v", test.want, got)
			}
		})
	}
}

func TestBrowseWithinLastDaysValidationAndCapabilities(t *testing.T) {
	_, s, _ := browseFixture(t)
	less := []string{
		`{"field":"dateAdded","operator":"within-last-days","value":0}`,
		`{"field":"dateAdded","operator":"within-last-days","value":36501}`,
		`{"field":"dateAdded","operator":"within-last-days","value":1.5}`,
		`{"field":"dateAdded","operator":"within-last-days","value":"30"}`,
		`{"field":"dateAdded","operator":"within-last-days","value":null}`,
		`{"field":"dateAdded","operator":"within-last-days","value":true}`,
		`{"field":"dateAdded","operator":"within-last-days"}`,
		`{"field":"year","operator":"within-last-days","value":7}`,
	}
	for _, raw := range less {
		if _, err := ParseBrowseQuery(json.RawMessage(raw), "query"); err == nil {
			t.Errorf("accepted invalid query %s", raw)
		}
	}
	for _, raw := range []string{
		`{"field":"dateAdded","operator":"within-last-days","value":1}`,
		`{"field":"dateAdded","operator":"within-last-days","value":36500}`,
	} {
		if _, err := ParseBrowseQuery(json.RawMessage(raw), "query"); err != nil {
			t.Errorf("refused valid query %s: %v", raw, err)
		}
	}
	capabilities, err := s.BrowseCapabilitiesFor("a", "movies")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"dateAdded", "releaseDate", "lastPlayedAt"} {
		found := false
		for _, field := range capabilities.Fields {
			if field.ID != id {
				continue
			}
			found = containsString(field.Operators, "within-last-days")
		}
		if !found {
			t.Errorf("%s does not publish within-last-days", id)
		}
	}
	for _, field := range capabilities.Fields {
		if field.Type != "date" && containsString(field.Operators, "within-last-days") {
			t.Errorf("%s publishes a date-only operator", field.ID)
		}
	}
}

func TestSavedViewWithinLastDaysStaysCurrent(t *testing.T) {
	db, s, names := browseFixture(t)
	c := catalogtest.New(t, db)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	added := map[string]time.Time{
		"m1": now.AddDate(0, 0, -5),
		"m2": now.AddDate(0, 0, -15),
		"m3": now.AddDate(0, 0, -25),
		"m4": now.AddDate(0, 0, -45),
		"m5": now.AddDate(0, 0, -50),
	}
	for name, at := range added {
		c.Fields(names[name].ID, map[string]any{"added_text": at.UTC().Format("2006-01-02T15:04:05.000Z")})
	}
	c.Drain()
	s.Clock = func() time.Time { return now }
	actor := ResourceActor{Authority: "local", AccountID: "account", ProfileID: "profile"}
	name := "New albums"
	definition := SavedViewDefinition{LibraryID: "a", Pivot: "movies", Query: &BrowseNode{Field: "dateAdded", Operator: "within-last-days", Value: 30}, Sort: titleAscending(), Presentation: "grid"}
	made, err := s.MutateSavedResource(actor, "", "create", SavedResourceMutation{OperationID: "relative-view", Kind: "view", Name: &name, Definition: &definition}, func(*sql.Tx) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	viewer := Viewer{Profile: actorKey(actor), Fence: "fence", Libraries: []string{"a"}}
	titles := func() string {
		content, err := s.SavedResourceContent(viewer, "server", "fence", made.ResourceID, actor, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, entry := range content.Browse.Entries {
			out = append(out, names.Of(entry.ID))
		}
		return strings.Join(out, ",")
	}
	if got := titles(); got != "m1,m2,m3" {
		t.Fatalf("at the first instant: want m1,m2,m3 got %q", got)
	}
	// Twenty days later the 30-day window no longer holds m2 or m3: the saved
	// view stored the rule, not a frozen date.
	now = now.AddDate(0, 0, 20)
	if got := titles(); got != "m1" {
		t.Fatalf("after moving the clock: want m1 got %q", got)
	}
}

func TestRelativeFacetAndCountsDoNotGoStale(t *testing.T) {
	db, s, names := browseFixture(t)
	c := catalogtest.New(t, db)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	added := map[string]time.Time{
		"m1": now.AddDate(0, 0, -6), // Action
		"m2": now.AddDate(0, 0, -2), // Action
		"m3": now.AddDate(0, 0, -1), // Comedy
		"m4": now.AddDate(0, 0, -100),
		"m5": now.AddDate(0, 0, -100), // Drama
	}
	for name, at := range added {
		c.Fields(names[name].ID, map[string]any{"added_text": at.UTC().Format("2006-01-02T15:04:05.000Z")})
	}
	c.Drain()
	s.Clock = func() time.Time { return now }
	query, err := ParseBrowseQuery(json.RawMessage(`{"field":"dateAdded","operator":"within-last-days","value":7}`), "query")
	if err != nil {
		t.Fatal(err)
	}
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}
	actionCount := func() int {
		page, err := s.Facets(viewer, FacetRequest{Library: "a", Field: "genre", Query: query})
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range page.Values {
			if value.Value == "Action" {
				return value.Count
			}
		}
		return 0
	}
	total := func() int {
		batch, err := s.BrowseCounts(viewer, "a", BrowseCountRequest{Pivot: "movies", Queries: []BrowseCountQuery{{ID: "relative", Query: query}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return batch.Counts[0].Total
	}
	if got := actionCount(); got != 2 {
		t.Fatalf("Action at the first instant: want 2 got %d", got)
	}
	if got := total(); got != 3 {
		t.Fatalf("count at the first instant: want 3 got %d", got)
	}
	// More than a minute, and past a membership boundary: the count cache is
	// keyed by the revision, which now carries the evaluation instant.
	now = now.AddDate(0, 0, 2)
	if got := actionCount(); got != 1 {
		t.Fatalf("Action after the clock moved: want 1 got %d", got)
	}
	if got := total(); got != 2 {
		t.Fatalf("count after the clock moved: want 2 got %d", got)
	}
}

func TestRelativeCursorPagesKeepTheFirstInstant(t *testing.T) {
	db, s, names := browseFixture(t)
	c := catalogtest.New(t, db)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for index, name := range []string{"m1", "m2", "m3", "m4", "m5"} {
		at := now.AddDate(0, 0, -6+index)
		c.Fields(names[name].ID, map[string]any{"added_text": at.UTC().Format("2006-01-02T15:04:05.000Z")})
	}
	c.Drain()
	s.Clock = func() time.Time { return now }
	query, err := ParseBrowseQuery(json.RawMessage(`{"field":"dateAdded","operator":"within-last-days","value":7}`), "query")
	if err != nil {
		t.Fatal(err)
	}
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}
	request := BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Query: query, Sort: titleAscending(), Limit: 2}
	first, err := s.BrowseEntities(viewer, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 2 || !first.PageInfo.HasMore || first.PageInfo.Total != 5 {
		t.Fatalf("first page: %d entries, more=%v total=%d", len(first.Entries), first.PageInfo.HasMore, first.PageInfo.Total)
	}
	seen := []string{}
	for _, entry := range first.Entries {
		seen = append(seen, names.Of(entry.ID))
	}
	// Three days later every title is outside the seven-day window. The
	// cursor's later pages still read the first page's snapshot.
	now = now.AddDate(0, 0, 3)
	for pages := 0; pages < 2; pages++ {
		request.Cursor = first.PageInfo.NextCursor
		page, err := s.BrowseEntities(viewer, request)
		if err != nil {
			t.Fatal(err)
		}
		if page.PageInfo.Revision != first.PageInfo.Revision || page.PageInfo.Total != 5 {
			t.Fatalf("continuation changed its snapshot: revision %q total %d", page.PageInfo.Revision, page.PageInfo.Total)
		}
		for _, entry := range page.Entries {
			seen = append(seen, names.Of(entry.ID))
		}
		first = page
	}
	if len(seen) != 5 {
		t.Fatalf("cursor pages skipped or repeated titles: %v", seen)
	}
	for _, name := range []string{"m1", "m2", "m3", "m4", "m5"} {
		if !containsString(seen, name) {
			t.Fatalf("cursor pages lost %s: %v", name, seen)
		}
	}
	// A new read without a cursor has no snapshot: at this instant nothing is
	// within seven days any more.
	request.Cursor = ""
	fresh, err := s.BrowseEntities(viewer, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.Entries) != 0 || fresh.PageInfo.Total != 0 {
		t.Fatalf("a fresh read reused an old instant: %d entries total %d", len(fresh.Entries), fresh.PageInfo.Total)
	}
}

func TestAbsoluteDateCursorsKeepTheirOldBehaviour(t *testing.T) {
	db, s, names := browseFixture(t)
	c := catalogtest.New(t, db)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for index, name := range []string{"m1", "m2", "m3", "m4", "m5"} {
		at := time.Date(2026, 9, 1+index, 0, 0, 0, 0, time.UTC)
		c.Fields(names[name].ID, map[string]any{"added_text": at.Format("2006-01-02T15:04:05.000Z")})
	}
	c.Drain()
	s.Clock = func() time.Time { return now }
	query, err := ParseBrowseQuery(json.RawMessage(`{"field":"dateAdded","operator":"at-least","value":"2026-09-01"}`), "query")
	if err != nil {
		t.Fatal(err)
	}
	if browseUsesRelative(query) {
		t.Fatal("an absolute query was classified as relative")
	}
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}
	request := BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Query: query, Sort: titleAscending(), Limit: 2}
	first, err := s.BrowseEntities(viewer, request)
	if err != nil {
		t.Fatal(err)
	}
	// A clock move cannot affect an absolute query, and its cursor carries no
	// evaluation instant: the revision and the later pages are unchanged.
	now = now.AddDate(0, 0, 100)
	request.Cursor = first.PageInfo.NextCursor
	second, err := s.BrowseEntities(viewer, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.PageInfo.Revision != first.PageInfo.Revision || second.PageInfo.Total != 5 || len(second.Entries) == 0 {
		t.Fatalf("absolute continuation changed: revision %q total %d entries %d", second.PageInfo.Revision, second.PageInfo.Total, len(second.Entries))
	}
}

func TestHomeSavedViewRelativeCursorIsOneSnapshot(t *testing.T) {
	db, s, names := browseFixture(t)
	c := catalogtest.New(t, db)
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for index, name := range []string{"m1", "m2", "m3", "m4", "m5"} {
		at := now.AddDate(0, 0, -6+index)
		c.Fields(names[name].ID, map[string]any{"added_text": at.UTC().Format("2006-01-02T15:04:05.000Z")})
	}
	c.Drain()
	s.Clock = func() time.Time { return now }
	actor := ResourceActor{Authority: "local", AccountID: "account", ProfileID: "profile"}
	name := "New albums"
	definition := SavedViewDefinition{LibraryID: "a", Pivot: "movies", Query: &BrowseNode{Field: "dateAdded", Operator: "within-last-days", Value: 7}, Sort: titleAscending(), Presentation: "grid"}
	made, err := s.MutateSavedResource(actor, "", "create", SavedResourceMutation{OperationID: "home-relative-view", Kind: "view", Name: &name, Definition: &definition}, func(*sql.Tx) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	rowID := homeViewPrefix + made.ResourceID
	request := HomeRequest{ServerID: "server", Viewer: Viewer{Profile: actorKey(actor), Fence: "fence", Libraries: []string{"a"}}, RowOrder: []string{rowID}}
	first, err := s.HomeSingleRow(request, rowID, HomeRowPage{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 2 || first.NextCursor == "" || first.Total != 5 {
		t.Fatalf("first Home row page: %d entries cursor=%v total=%d", len(first.Entries), first.NextCursor != "", first.Total)
	}
	now = now.AddDate(0, 0, 3)
	second, err := s.HomeSingleRow(request, rowID, HomeRowPage{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 2 || second.Total != 5 {
		t.Fatalf("Home row continuation changed its snapshot: %d entries total %d", len(second.Entries), second.Total)
	}
	fresh, err := s.HomeSingleRow(request, rowID, HomeRowPage{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Total != 0 || len(fresh.Entries) != 0 {
		t.Fatalf("a fresh Home read reused an old instant: total %d entries %d", fresh.Total, len(fresh.Entries))
	}
}
