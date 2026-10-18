package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	// A legacy grant role changes only canonical metadata, not index membership.
	if _, err := db.Exec(`UPDATE saved_resource_shares SET role='editor' WHERE resource_id=?`, shared.ResourceID); err != nil {
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
		if r.ID == shared.ResourceID && r.Role != "editor" {
			t.Fatal("hydration retained an obsolete grant role", r)
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
		index := "saved_view_picker_library"
		if pivot != "" {
			index = "saved_view_picker_pivot"
		}
		if !strings.Contains(plan, index) || !strings.Contains(plan, "MATERIALIZE selected") {
			t.Fatalf("unbounded preset scope (%s):\n%s", pivot, plan)
		}
	}
}

func TestSavedViewPresetsHydrationHasFixedDatabaseCost(t *testing.T) {
	s, _, v := savedFixture(t)
	a := ResourceActor{Authority: v.Authority, AccountID: v.AccountID, ProfileID: v.ProfileID}
	d := SavedViewDefinition{LibraryID: "library", Pivot: "movies", Presentation: "grid", Sort: []BrowseSortSelection{{Field: "title", Direction: "asc"}}}
	for _, name := range []string{"One", "Two", "Three", "Four"} {
		r := createPreset(t, s, a, "fixed-cost-"+name, name, d)
		if _, err := s.MutateSavedPin(a, "view", r.ResourceID, PinMutation{OperationID: "fixed-cost-pin-" + name, Pinned: true}, func(*sql.Tx) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.cursorKey(); err != nil {
		t.Fatal(err)
	}
	for _, pinned := range []bool{false, true} {
		for _, limit := range []int{1, 100} {
			ctx, cost := dbwork.Measure(context.Background())
			page, err := s.WithContext(ctx).SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", "", pinned, limit)
			if err != nil || len(page.Resources) == 0 {
				t.Fatal(page, err)
			}
			// Library kind, combined generation/arrangement revision, and bounded
			// selection/hydration; includes all statements inside the snapshot.
			if spent := cost().Statements; spent != 3 {
				t.Fatalf("pinned=%t limit=%d used %d statements, want 3", pinned, limit, spent)
			}
		}
	}
}

func TestSavedViewPresetsPinOrderContinuation(t *testing.T) {
	s, _, v := savedFixture(t)
	a := ResourceActor{Authority: v.Authority, AccountID: v.AccountID, ProfileID: v.ProfileID}
	authorize := func(*sql.Tx) error { return nil }
	d := SavedViewDefinition{LibraryID: "library", Pivot: "movies", Presentation: "grid", Sort: []BrowseSortSelection{{Field: "title", Direction: "asc"}}}
	var order []PinOrderEntry
	for _, name := range []string{"Zulu", "Alpha", "Charlie"} {
		r := createPreset(t, s, a, "order-preset-"+name, name, d)
		if _, err := s.MutateSavedPin(a, "view", r.ResourceID, PinMutation{OperationID: "order-pin-" + name, Pinned: true}, authorize); err != nil {
			t.Fatal(err)
		}
		order = append(order, PinOrderEntry{Kind: "view", ID: r.ResourceID})
	}
	arranged, err := s.SetPinOrder(a, 0, order, authorize)
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", "", true, 1)
	if err != nil || len(page.Resources) != 1 || page.Resources[0].ID != order[0].ID || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	// Capture an unpinned cursor too: arrangement changes do not affect name order.
	unpinned, err := s.SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", "", false, 1)
	if err != nil {
		t.Fatal(err)
	}
	order[0], order[1] = order[1], order[0]
	if _, err = s.SetPinOrder(a, arranged.Revision, order, authorize); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", page.NextCursor, true, 1); !errors.Is(err, ErrStaleContinuation) {
		t.Fatal("reordered continuation accepted", err)
	}
	fresh, err := s.SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", "", true, 1)
	if err != nil || fresh.Resources[0].ID != order[0].ID {
		t.Fatal(fresh, err)
	}
	if _, err = s.SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", unpinned.NextCursor, false, 1); err != nil {
		t.Fatal("unrelated arrangement invalidated name order", err)
	}
}

// VM work, rather than elapsed time or statement count, catches a scope scan
// hidden behind an outer LIMIT. Both audiences have the entire growing scope;
// tied names exercise the second seek component on deep continuations.
func TestSavedViewPresetsSeekWorkDoesNotGrowWithScope(t *testing.T) {
	_, db, v := savedFixture(t)
	owner := ResourceActor{Authority: v.Authority, AccountID: v.AccountID, ProfileID: v.ProfileID}
	shared := ResourceActor{Authority: "hosted", AccountID: "shared-account", ProfileID: "shared-profile"}
	insert := func(start, end int) {
		t.Helper()
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		resource, err := tx.Prepare(`INSERT INTO saved_resources(id,kind,owner_key,owner_authority,owner_account,owner_profile,name,definition,created_at) VALUES(?,'view',?,?,?,?,?, '{"libraryId":"library","pivot":"movies","presentation":"grid"}','2026-10-18')`)
		if err != nil {
			t.Fatal(err)
		}
		defer resource.Close()
		share, err := tx.Prepare(`INSERT INTO saved_resource_shares VALUES(?,?,?,?,'viewer')`)
		if err != nil {
			t.Fatal(err)
		}
		defer share.Close()
		pin, err := tx.Prepare(`INSERT INTO saved_pins VALUES(?,'view',?,1,1)`)
		if err != nil {
			t.Fatal(err)
		}
		defer pin.Close()
		for i := start; i < end; i++ {
			id := fmt.Sprintf("seek-%05d", i)
			name := "Same"
			if i%2 == 1 {
				name = fmt.Sprintf("Name %05d", i)
			}
			if _, err = resource.Exec(id, actorKey(owner), owner.Authority, owner.AccountID, owner.ProfileID, name); err != nil {
				t.Fatal(err)
			}
			if _, err = share.Exec(id, shared.Authority, shared.AccountID, shared.ProfileID); err != nil {
				t.Fatal(err)
			}
			for _, a := range []ResourceActor{owner, shared} {
				if _, err = pin.Exec(actorKey(a), id); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	measure := func(n int) map[string]int64 {
		t.Helper()
		result := map[string]int64{}
		for audience, a := range map[string]ResourceActor{"owner": owner, "shared": shared} {
			for _, pivot := range []string{"", "movies"} {
				for _, pinned := range []bool{false, true} {
					for _, continuation := range []string{"first", "name", "tie"} {
						var after *cursorValue
						prefix := ""
						if pinned {
							prefix = "99999999\x1f"
						}
						switch continuation {
						case "name":
							after = &cursorValue{Value: prefix + fmt.Sprintf("name %05d", n-5), ID: fmt.Sprintf("seek-%05d", n-5)}
						case "tie":
							after = &cursorValue{Value: prefix + "same", ID: fmt.Sprintf("seek-%05d", n-6)}
						}
						query, args := savedViewPresetSQL(a, "library", pivot, pinned, 1, after)
						ctx, cost := dbwork.Measure(context.Background())
						rows, err := db.QueryContext(ctx, query, args...)
						if err != nil {
							t.Fatal(err)
						}
						count := 0
						var ids []string
						for rows.Next() {
							var values [10]any
							targets := make([]any, len(values))
							for i := range values {
								targets[i] = &values[i]
							}
							if err = rows.Scan(targets...); err != nil {
								t.Fatal(err)
							}
							count++
							ids = append(ids, values[0].(string))
						}
						rows.Close()
						if err = rows.Err(); err != nil {
							t.Fatal(err)
						}
						if count != 2 || cost().Statements != 1 || cost().VMSteps <= 0 {
							t.Fatal("invalid work sample", count, cost())
						}
						want := []string{"seek-00001", "seek-00003"}
						switch continuation {
						case "name":
							want = []string{fmt.Sprintf("seek-%05d", n-3), fmt.Sprintf("seek-%05d", n-1)}
						case "tie":
							want = []string{fmt.Sprintf("seek-%05d", n-4), fmt.Sprintf("seek-%05d", n-2)}
						case "first":
							if pinned {
								want = []string{"seek-00000", "seek-00001"}
							}
						}
						if !reflect.DeepEqual(ids, want) {
							t.Fatalf("%s pinned=%t: IDs %v, want %v", continuation, pinned, ids, want)
						}
						label := fmt.Sprintf("%s/pivot=%s/pinned=%t/%s", audience, pivot, pinned, continuation)
						result[label] = cost().VMSteps
						planRows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
						if err != nil {
							t.Fatal(err)
						}
						plan := ""
						selection := true
						for planRows.Next() {
							var id, parent, unused int
							var detail string
							if err = planRows.Scan(&id, &parent, &unused, &detail); err != nil {
								t.Fatal(err)
							}
							if strings.Contains(detail, "SCAN selected") {
								selection = false
							}
							if selection && strings.Contains(detail, "TEMP B-TREE") {
								t.Fatalf("unbounded selection sort: %s\n%s", label, plan+detail)
							}
							plan += detail + "\n"
						}
						planRows.Close()
						if err = planRows.Err(); err != nil {
							t.Fatal(err)
						}
						index := "saved_view_picker_"
						if pinned {
							index += "pinned_"
						}
						if pivot == "" {
							index += "library"
						} else {
							index += "pivot"
						}
						if !strings.Contains(plan, index) || !strings.Contains(plan, "MATERIALIZE selected") {
							t.Fatalf("missing bounded index selection %s:\n%s", label, plan)
						}
						if after != nil && !strings.Contains(plan, "(name_key,resource_id)>(?,?)") && !strings.Contains(plan, "(pin_key,resource_id)>(?,?)") {
							t.Fatalf("cursor is not a tuple index seek %s:\n%s", label, plan)
						}
					}
				}
			}
		}
		return result
	}
	insert(0, 100)
	for _, a := range []ResourceActor{owner, shared} {
		if _, err := db.Exec(`INSERT INTO saved_pin_order VALUES(?,'view','seek-00000',0)`, actorKey(a)); err != nil {
			t.Fatal(err)
		}
	}
	small := measure(100)
	insert(100, 10000)
	large := measure(10000)
	for label, steps := range large {
		t.Logf("%s: 100 rows=%d VM; 10000 rows=%d VM", label, small[label], steps)
		if steps > small[label]+100 {
			t.Fatalf("scope growth increased VM work %s: %d -> %d", label, small[label], steps)
		}
	}
}

func TestSavedViewPresetsHydratesCanonicalAuthorization(t *testing.T) {
	s, db, v := savedFixture(t)
	a := ResourceActor{Authority: v.Authority, AccountID: v.AccountID, ProfileID: v.ProfileID}
	other := ResourceActor{Authority: "local", AccountID: "foreign", ProfileID: "foreign"}
	r := createPreset(t, s, other, "auth-index-canonical", "Foreign", SavedViewDefinition{LibraryID: "library", Pivot: "movies", Presentation: "grid", Sort: []BrowseSortSelection{{Field: "title", Direction: "asc"}}})
	// Even a corrupt/stale derived audience entry cannot grant access.
	if _, err := db.Exec(`INSERT INTO saved_view_picker SELECT ?,resource_id,library_id,pivot,name_key,pinned,pin_key FROM saved_view_picker WHERE resource_id=?`, actorKey(a), r.ResourceID); err != nil {
		t.Fatal(err)
	}
	page, err := s.SavedViewPresets(Viewer{Libraries: []string{"library"}}, "server", "fence", a, "library", "movies", "", false, 40)
	if err != nil || len(page.Resources) != 0 {
		t.Fatal("index granted authorization", page, err)
	}
}
