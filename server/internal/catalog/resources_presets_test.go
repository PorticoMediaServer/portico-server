package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
)

func createPreset(t *testing.T, s *Service, a ResourceActor, op, name string, d SavedViewDefinition) SavedResourceReceipt {
	t.Helper()
	r, err := s.MutateSavedResource(a, "", "create", SavedResourceMutation{OperationID: op, Kind: "view", Name: &name, Definition: &d}, func(*sql.Tx) error { return nil })
	if err != nil {
		t.Fatalf("%v; validation=%v, definition=%+v", err, validateSavedView(s.read(), d), d)
	}
	return r
}

func TestSavedViewPresetsScopePagingAndCompleteDefinition(t *testing.T) {
	s, db, v := savedFixture(t)
	fixtures := catalogtest.New(t, db)
	fixtures.Library("tv", "TV", "tv", "/tv")
	fixtures.Drain()
	a := ResourceActor{Authority: v.Authority, AccountID: v.AccountID, ProfileID: v.ProfileID}
	d := SavedViewDefinition{LibraryID: "library", Pivot: "movies", Presentation: "grid", Sort: []BrowseSortSelection{{Field: "random", Direction: "desc", Seed: 731}}, Query: &BrowseNode{All: []BrowseNode{
		{Any: []BrowseNode{{Field: "year", Operator: "equals", Value: float64(2020)}, {Field: "year", Operator: "is-missing"}}},
		{Not: &BrowseNode{Field: "title", Operator: "equals", Value: "Excluded"}},
	}}}
	var ids []string
	for _, op := range []string{"preset-a", "preset-b", "preset-c"} {
		ids = append(ids, createPreset(t, s, a, op, "Same", d).ResourceID)
	}
	tv := d
	tv.LibraryID = "tv"
	tv.Pivot = "shows"
	createPreset(t, s, a, "tv-shows", "TV", tv)
	tv.Pivot = "episodes"
	createPreset(t, s, a, "tv-episodes", "TV episodes", tv)
	for i, actor := range []ResourceActor{{Authority: "hosted", AccountID: a.AccountID, ProfileID: a.ProfileID}, {Authority: a.Authority, AccountID: "other", ProfileID: a.ProfileID}, {Authority: a.Authority, AccountID: a.AccountID, ProfileID: "child"}} {
		createPreset(t, s, actor, []string{"other-authority", "other-account", "other-profile"}[i], "Same", d)
	}
	viewer := Viewer{Libraries: []string{"library", "tv"}}
	var seen []string
	cursor := ""
	for {
		page, err := s.SavedViewPresets(viewer, "server", "fence", a, "library", "movies", cursor, false, 1)
		if err != nil || len(page.Resources) != 1 {
			t.Fatal(page, err)
		}
		r := page.Resources[0]
		seen = append(seen, r.ID)
		raw, _ := json.Marshal(r.Definition)
		want, _ := json.Marshal(d)
		if string(raw) != string(want) || r.Status != "ready" || r.Role != "owner" || !reflect.DeepEqual(r.Actions, []string{"pin", "update", "delete"}) {
			t.Fatalf("lost preset facts: %s (%+v)", raw, r)
		}
		if page.NextCursor == "" {
			break
		}
		if cursor == "" {
			for _, change := range []struct {
				library, pivot, fence string
				actor                 ResourceActor
			}{{"tv", "shows", "fence", a}, {"library", "", "fence", a}, {"library", "movies", "other-fence", a}, {"library", "movies", "fence", ResourceActor{Authority: a.Authority, AccountID: a.AccountID, ProfileID: "child"}}} {
				if _, err := s.SavedViewPresets(viewer, "server", change.fence, change.actor, change.library, change.pivot, page.NextCursor, false, 1); err == nil {
					t.Fatal("cursor crossed scope", change)
				}
			}
		}
		cursor = page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatal("missing or leaked profiles", seen)
	}
	for _, id := range ids {
		if !containsString(seen, id) {
			t.Fatal("missing", id)
		}
	}
	page, err := s.SavedViewPresets(viewer, "server", "fence", a, "tv", "", "", false, 40)
	if err != nil || len(page.Resources) != 2 {
		t.Fatal(page, err)
	}
	if _, err = s.SavedViewPresets(Viewer{}, "server", "fence", a, "library", "movies", "", false, 40); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("revoked library allowed", err)
	}
	if _, err = s.SavedViewPresets(viewer, "server", "fence", a, "tv", "movies", "", false, 40); err == nil {
		t.Fatal("wrong pivot allowed")
	}
	// Updating a resource invalidates the picker continuation; continuing after
	// a rename could otherwise silently skip or repeat rows.
	page, err = s.SavedViewPresets(viewer, "server", "fence", a, "library", "movies", "", false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE saved_resources SET revision=revision+1 WHERE id=?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavedViewPresets(viewer, "server", "fence", a, "library", "movies", page.NextCursor, false, 1); !errors.Is(err, ErrStaleContinuation) {
		t.Fatal("accepted stale page", err)
	}
}

func TestSavedViewPresetsBatchMetadataLegacySharesAndInvalidDefinitions(t *testing.T) {
	s, db, v := savedFixture(t)
	a := ResourceActor{Authority: v.Authority, AccountID: v.AccountID, ProfileID: v.ProfileID}
	other := ResourceActor{Authority: "local", AccountID: "other", ProfileID: "other"}
	d := SavedViewDefinition{LibraryID: "library", Pivot: "movies", Presentation: "grid", Sort: []BrowseSortSelection{{Field: "title", Direction: "asc"}}}
	_ = createPreset(t, s, a, "owner-preset", "Owner", d)
	shared := createPreset(t, s, other, "shared-preset", "Shared", d)
	stale := createPreset(t, s, a, "stale-preset", "Stale", d)
	malformed := createPreset(t, s, a, "malformed-preset", "Malformed", d)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO saved_resource_shares(resource_id,authority,account_id,profile_id,role) VALUES(?,?,?,?,'viewer')`, []any{shared.ResourceID, a.Authority, a.AccountID, a.ProfileID}},
		{`INSERT INTO saved_pins(owner_key,kind,resource_id,pinned,revision) VALUES(?,'view',?,1,7)`, []any{actorKey(a), shared.ResourceID}},
		{`INSERT INTO saved_pin_order(owner_key,kind,resource_id,position) VALUES(?,'view',?,0)`, []any{actorKey(a), shared.ResourceID}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	d.Sort = []BrowseSortSelection{{Field: "groupOrder", Direction: "asc"}}
	raw, _ := json.Marshal(d)
	if _, err := db.Exec(`UPDATE saved_resources SET definition=? WHERE id=?`, string(raw), stale.ResourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE saved_resources SET definition='broken historical JSON' WHERE id=?`, malformed.ResourceID); err != nil {
		t.Fatal(err)
	}
	page, err := s.SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", "", false, 100)
	if err != nil || len(page.Resources) != 3 {
		t.Fatal(page, err)
	}
	for _, r := range page.Resources {
		single, err := s.SavedResource("server", "fence", r.ID, a)
		if err != nil || !reflect.DeepEqual(r, single) {
			t.Fatalf("batch differs from resource read: %#v / %#v (%v)", r, single, err)
		}
		if r.ID == stale.ResourceID && (r.Status != "needs-review" || !containsString(r.InvalidComponents, "sort[0].field")) {
			t.Fatal("stale preset executable", r)
		}
	}
	pinned, err := s.SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", "", true, 40)
	if err != nil || len(pinned.Resources) != 1 || pinned.Resources[0].ID != shared.ResourceID || pinned.Resources[0].PinRevision != 7 {
		t.Fatal(pinned, err)
	}
	// Malformed records still have an owner management path.
	generic, err := s.SavedResource("server", "fence", malformed.ResourceID, a)
	if err != nil || generic.Status != "needs-review" {
		t.Fatal(generic, err)
	}
	if _, err = db.Exec(`DELETE FROM saved_resource_shares WHERE resource_id=?`, shared.ResourceID); err != nil {
		t.Fatal(err)
	}
	page, err = s.SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", "", false, 100)
	if err != nil || len(page.Resources) != 2 {
		t.Fatal("share revocation not enforced", page, err)
	}
	if _, err = s.MutateSavedResource(a, "", "create", SavedResourceMutation{OperationID: "invalid-order", Kind: "view", Name: ptrPresetName("Invalid"), Definition: &d}, func(*sql.Tx) error { return nil }); err == nil {
		t.Fatal("new invalid group order saved")
	}
}

func ptrPresetName(v string) *string { return &v }

func TestSavedViewPresetsQueryUsesScopedIndexes(t *testing.T) {
	s, _, v := savedFixture(t)
	a := ResourceActor{Authority: v.Authority, AccountID: v.AccountID, ProfileID: v.ProfileID}
	for _, pivot := range []string{"", "movies"} {
		query, args := savedViewPresetSQL(a, "library", pivot, false, 40, nil)
		rows, err := s.read().Query("EXPLAIN QUERY PLAN "+query, args...)
		if err != nil {
			t.Fatal(err)
		}
		plan := ""
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan += detail + "\n"
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		index := "saved_view_library_page"
		if pivot != "" {
			index = "saved_view_pivot_page"
		}
		if !strings.Contains(plan, index) || !strings.Contains(plan, "saved_resource_shared") {
			t.Fatalf("unbounded preset scope (%s):\n%s", pivot, plan)
		}
	}
}

func TestSavedViewPresetsHydrationHasFixedDatabaseCost(t *testing.T) {
	s, _, v := savedFixture(t)
	a := ResourceActor{Authority: v.Authority, AccountID: v.AccountID, ProfileID: v.ProfileID}
	d := SavedViewDefinition{LibraryID: "library", Pivot: "movies", Presentation: "grid", Sort: []BrowseSortSelection{{Field: "title", Direction: "asc"}}}
	for _, name := range []string{"One", "Two", "Three", "Four"} {
		createPreset(t, s, a, "fixed-cost-"+name, name, d)
	}
	if _, err := s.cursorKey(); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{1, 100} {
		ctx, cost := dbwork.Measure(context.Background())
		page, err := s.WithContext(ctx).SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", "", false, limit)
		spent := cost().Statements
		if err != nil || len(page.Resources) == 0 {
			t.Fatal(page, err)
		}
		// Library kind, generation, and one metadata query, measured in the
		// driver so statements inside the snapshot are counted too.
		if spent != 3 {
			t.Fatalf("limit %d used %d database calls, want 3", limit, spent)
		}
	}
}
