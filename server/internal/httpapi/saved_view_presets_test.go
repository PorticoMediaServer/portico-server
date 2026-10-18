package httpapi

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"slices"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
)

type presetHTTPReceipt struct {
	Receipt catalog.SavedResourceReceipt `json:"receipt"`
}

func presetHTTPDefinition(library, pivot string) catalog.SavedViewDefinition {
	return catalog.SavedViewDefinition{
		LibraryID: library, Pivot: pivot, Presentation: "grid",
		Sort: []catalog.BrowseSortSelection{{Field: "title", Direction: "asc"}},
	}
}

func presetHTTPCreate(t *testing.T, f *v1Fixture, token, operation, name string, definition catalog.SavedViewDefinition) catalog.SavedResourceReceipt {
	t.Helper()
	var out presetHTTPReceipt
	f.callAs(token, "POST", "/v1/saved-resources", nil, map[string]any{
		"operationId": operation, "expectedRevision": 0, "kind": "view", "name": name,
		"summary": "An explicitly saved filter", "definition": definition,
	}, 200, &out)
	if out.Receipt.ResourceID == "" || out.Receipt.Revision < 1 || out.Receipt.Deleted {
		t.Fatalf("invalid create receipt: %+v", out.Receipt)
	}
	return out.Receipt
}

func presetHTTPPage(f *v1Fixture, token, query string) catalog.SavedResourcePage {
	f.t.Helper()
	var out catalog.SavedResourcePage
	w := f.callAs(token, "GET", "/v1/saved-resources?"+query, nil, nil, 200, &out)
	if out.Resources == nil || out.ServerID != f.id.ID() || out.ViewerFence == "" {
		f.t.Fatalf("incomplete saved resource page: %+v", out)
	}
	var wire struct {
		Resources []struct {
			Definition json.RawMessage `json:"definition"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &wire); err != nil {
		f.t.Fatal(err)
	}
	for i, resource := range out.Resources {
		if resource.Definition != nil {
			// BrowseNode.MarshalJSON restores a missing presence value as null.
			// Check the actual wire object too, so decoding cannot hide that loss.
			presetHTTPDefinitionEqual(f.t, wire.Resources[i].Definition, *resource.Definition)
		}
	}
	return out
}

func presetHTTPIDs(t *testing.T, page catalog.SavedResourcePage, want ...string) {
	t.Helper()
	got := make([]string, len(page.Resources))
	for i, resource := range page.Resources {
		got[i] = resource.ID
	}
	if !slices.Equal(got, want) {
		t.Fatalf("resource IDs = %v, want %v", got, want)
	}
}

func presetHTTPDefinitionEqual(t *testing.T, got any, want catalog.SavedViewDefinition) {
	t.Helper()
	// Compare JSON values, preserving every AST node, null and sort seed while
	// treating numeric Go input values the same as their decoded JSON values.
	canonical := func(v any) any {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var out any
		if err = json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if !reflect.DeepEqual(canonical(got), canonical(want)) {
		t.Fatalf("definition = %s, want %s", mustPresetHTTPJSON(t, got), mustPresetHTTPJSON(t, want))
	}
}

func mustPresetHTTPJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func presetHTTPOtherProfile(t *testing.T, f *v1Fixture) identity.Envelope {
	t.Helper()
	snapshot, err := f.id.CreateDirectProfile(context.Background(), f.owner.AccessToken, "Other profile", "mint")
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range snapshot.Profiles {
		if !profile.Primary {
			picker := f.device("preset-profile-picker")
			selected, err := f.id.SelectDirectProfile(context.Background(), picker.AccessToken, profile.ID, identity.DirectSelection{})
			if err != nil {
				t.Fatalf("select secondary profile: %v", err)
			}
			if selected.Session.AccessToken == "" {
				t.Fatal("secondary profile selection returned no session")
			}
			return selected.Session
		}
	}
	t.Fatal("new secondary profile missing")
	return identity.Envelope{}
}

func presetHTTPMember(t *testing.T, f *v1Fixture, libraries ...string) (identity.DirectAccount, identity.Envelope) {
	t.Helper()
	member, err := f.id.CreateDirectMember(context.Background(), f.owner.AccessToken, identity.DirectMemberInput{
		Username: "preset-member", Name: "Preset member", Password: "Preset-test-password-1!", AllowedLibraries: libraries,
	})
	if err != nil {
		t.Fatal(err)
	}
	return member, f.deviceFor(member.ID, member.PrimaryProfileID, member.Role, "preset-member-device")
}

func TestSavedViewPresetsRoundTripAcrossDevices(t *testing.T) {
	f := newV1Fixture(t, 0)
	first, second := f.device("preset-device-a"), f.device("preset-device-b")
	if first.SessionFamilyID == second.SessionFamilyID || first.AccessToken == second.AccessToken || first.Viewer != second.Viewer {
		t.Fatal("fixture must have two independent sessions for the same viewer")
	}
	definition := presetHTTPDefinition(f.library, "movies")
	definition.Presentation = "list"
	definition.Query = &catalog.BrowseNode{All: []catalog.BrowseNode{
		{Any: []catalog.BrowseNode{
			{Field: "genre", Operator: "contains-any", Value: []string{"Drama", "Science Fiction"}},
			{All: []catalog.BrowseNode{
				{Field: "year", Operator: "between", Value: []int{1995, 2026}},
				{Not: &catalog.BrowseNode{Field: "title", Operator: "contains", Value: "spoiler"}},
			}},
		}},
		{Field: "contentRating", Operator: "is-present", Value: nil},
	}}
	definition.Sort = []catalog.BrowseSortSelection{{Field: "random", Direction: "desc", Seed: 4000000001}}
	created := presetHTTPCreate(t, f, first.AccessToken, "preset-roundtrip-create", "Evening picks", definition)
	query := "kind=view&libraryId=" + f.library + "&pivot=movies"
	page := presetHTTPPage(f, second.AccessToken, query)
	presetHTTPIDs(t, page, created.ResourceID)
	resource := page.Resources[0]
	presetHTTPDefinitionEqual(t, resource.Definition, definition)
	if resource.Name != "Evening picks" || resource.Summary != "An explicitly saved filter" || resource.Revision != created.Revision || resource.Kind != "view" || resource.Role != "owner" || resource.Visibility != "private" || resource.Status != "ready" || len(resource.InvalidComponents) != 0 {
		t.Fatalf("incomplete or changed resource metadata: %+v", resource)
	}
	if resource.ServerID != page.ServerID || resource.ViewerFence != page.ViewerFence || !slices.Contains(resource.Actions, "update") || !slices.Contains(resource.Actions, "delete") {
		t.Fatalf("missing resource scope or management actions: %+v", resource)
	}
	// A rename on device B preserves the complete definition on device A.
	var renamed presetHTTPReceipt
	f.callAs(second.AccessToken, "PATCH", "/v1/saved-resources/"+created.ResourceID, nil, map[string]any{
		"operationId": "preset-roundtrip-rename", "expectedRevision": created.Revision, "name": "Weekend picks",
	}, 200, &renamed)
	page = presetHTTPPage(f, first.AccessToken, query)
	presetHTTPIDs(t, page, created.ResourceID)
	presetHTTPDefinitionEqual(t, page.Resources[0].Definition, definition)
	if page.Resources[0].Name != "Weekend picks" || page.Resources[0].Revision != renamed.Receipt.Revision || renamed.Receipt.Revision <= created.Revision {
		t.Fatalf("rename did not propagate: %+v", page.Resources[0])
	}
	// An explicit definition update is visible on the other device too.
	definition.Sort[0].Seed = 123456789
	definition.Presentation = "grid"
	var updated presetHTTPReceipt
	f.callAs(first.AccessToken, "PATCH", "/v1/saved-resources/"+created.ResourceID, nil, map[string]any{
		"operationId": "preset-roundtrip-update", "expectedRevision": renamed.Receipt.Revision, "definition": definition,
	}, 200, &updated)
	page = presetHTTPPage(f, second.AccessToken, query)
	presetHTTPIDs(t, page, created.ResourceID)
	presetHTTPDefinitionEqual(t, page.Resources[0].Definition, definition)
	if page.Resources[0].Revision != updated.Receipt.Revision || updated.Receipt.Revision <= renamed.Receipt.Revision {
		t.Fatalf("definition revision did not propagate: %+v", page.Resources[0])
	}
	f.callAs(second.AccessToken, "DELETE", "/v1/saved-resources/"+created.ResourceID, nil, map[string]any{
		"operationId": "preset-roundtrip-delete", "expectedRevision": updated.Receipt.Revision,
	}, 200, nil)
	presetHTTPIDs(t, presetHTTPPage(f, first.AccessToken, query))
}

func TestSavedViewPresetsScopeAndExclusions(t *testing.T) {
	f := newV1Fixture(t, 0)
	f.catalogTest.Library("other-movies", "Other movies", "movie", t.TempDir())
	f.catalogTest.Drain()
	otherProfile := presetHTTPOtherProfile(t, f)
	_, otherAccount := presetHTTPMember(t, f, f.library, "other-movies")
	def := presetHTTPDefinition(f.library, "movies")
	// Unrelated names precede the matches: filtering must happen before LIMIT.
	otherLibrary := presetHTTPCreate(t, f, f.owner.AccessToken, "scope-library", "00 Other library", presetHTTPDefinition("other-movies", "movies"))
	otherPivot := presetHTTPCreate(t, f, f.owner.AccessToken, "scope-pivot", "01 Other pivot", presetHTTPDefinition(f.library, "collections"))
	profileOnly := presetHTTPCreate(t, f, otherProfile.AccessToken, "scope-profile", "02 Other profile", def)
	accountOnly := presetHTTPCreate(t, f, otherAccount.AccessToken, "scope-account", "03 Other account", def)
	deleted := presetHTTPCreate(t, f, f.owner.AccessToken, "scope-deleted", "04 Deleted", def)
	f.call("DELETE", "/v1/saved-resources/"+deleted.ResourceID, nil, map[string]any{"operationId": "scope-delete", "expectedRevision": deleted.Revision}, 200, nil)
	var collection presetHTTPReceipt
	f.call("POST", "/v1/saved-resources", nil, map[string]any{"operationId": "scope-collection", "expectedRevision": 0, "kind": "collection", "name": "05 Collection"}, 200, &collection)
	first := presetHTTPCreate(t, f, f.owner.AccessToken, "scope-first", "10 Same name", def)
	second := presetHTTPCreate(t, f, f.owner.AccessToken, "scope-second", "10 Same name", def)
	third := presetHTTPCreate(t, f, f.owner.AccessToken, "scope-third", "20 Last", def)
	want := []string{first.ResourceID, second.ResourceID}
	slices.Sort(want) // Equal names use ID as a stable tie-breaker.
	want = append(want, third.ResourceID)
	query := "kind=view&libraryId=" + f.library + "&pivot=movies&limit=1"
	var got []string
	cursor := ""
	for range 4 {
		page := presetHTTPPage(f, f.owner.AccessToken, query+"&cursor="+url.QueryEscape(cursor))
		if len(page.Resources) != 1 {
			t.Fatalf("scoped page should contain exactly one matching view: %+v", page)
		}
		presetHTTPDefinitionEqual(t, page.Resources[0].Definition, def)
		got = append(got, page.Resources[0].ID)
		if page.NextCursor == "" {
			break
		}
		if page.NextCursor == cursor {
			t.Fatal("pagination cursor did not advance")
		}
		cursor = page.NextCursor
	}
	if !slices.Equal(got, want) {
		t.Fatalf("scoped pagination IDs = %v, want %v", got, want)
	}
	presetHTTPIDs(t, presetHTTPPage(f, otherProfile.AccessToken, "kind=view&libraryId="+f.library+"&pivot=movies"), profileOnly.ResourceID)
	presetHTTPIDs(t, presetHTTPPage(f, otherAccount.AccessToken, "kind=view&libraryId="+f.library+"&pivot=movies"), accountOnly.ResourceID)
	for _, token := range []string{otherProfile.AccessToken, otherAccount.AccessToken} {
		f.callAs(token, "GET", "/v1/saved-resources/"+first.ResourceID, nil, nil, 404, nil)
	}
	// Optional pivot includes both pivots, while another library stays excluded.
	presetHTTPIDs(t, presetHTTPPage(f, f.owner.AccessToken, "kind=view&libraryId="+f.library), append([]string{otherPivot.ResourceID}, want...)...)
	presetHTTPIDs(t, presetHTTPPage(f, f.owner.AccessToken, "kind=view&libraryId=other-movies&pivot=movies"), otherLibrary.ResourceID)
	presetHTTPIDs(t, presetHTTPPage(f, f.owner.AccessToken, "kind=view&libraryId=other-movies&pivot=collections"))
	// The existing generic directory still includes all this viewer's libraries/pivots.
	presetHTTPIDs(t, presetHTTPPage(f, f.owner.AccessToken, "kind=view"), append([]string{otherLibrary.ResourceID, otherPivot.ResourceID}, want...)...)
	presetHTTPIDs(t, presetHTTPPage(f, f.owner.AccessToken, "kind=collection"), collection.Receipt.ResourceID)
	f.call("PUT", "/v1/saved-pins/view/"+second.ResourceID, nil, map[string]any{"operationId": "scope-pin", "expectedRevision": 0, "pinned": true}, 200, nil)
	for _, pinned := range []string{"true", "1"} {
		page := presetHTTPPage(f, f.owner.AccessToken, "kind=view&libraryId="+f.library+"&pivot=movies&pinned="+pinned)
		presetHTTPIDs(t, page, second.ResourceID)
		if !page.Resources[0].Pinned || page.Resources[0].PinRevision < 1 {
			t.Fatalf("pinned preset lost its pin metadata: %+v", page.Resources[0])
		}
	}
	for _, pinned := range []string{"false", "0"} {
		presetHTTPIDs(t, presetHTTPPage(f, f.owner.AccessToken, "kind=view&libraryId="+f.library+"&pivot=movies&pinned="+pinned), want...)
	}
}

func TestSavedViewPresetsRejectMalformedQueries(t *testing.T) {
	f := newV1Fixture(t, 0)
	base := "kind=view&libraryId=" + f.library + "&pivot=movies"
	cases := []struct{ query, code string }{
		{"libraryId=movies", "invalid_request"},
		{"kind=collection&libraryId=movies", "invalid_request"},
		{"kind=unknown&libraryId=movies", "invalid_request"},
		{"kind=view&libraryId=", "invalid_request"},
		{"kind=view&pivot=movies", "invalid_request"},
		{"kind=collection&pivot=movies", "invalid_request"},
		{"kind=view&libraryId=movies&pivot=", "invalid_request"},
		{"kind=view&libraryId=movies&pivot=unknown", "invalid_request"},
		{"kind=view&libraryId=movies&pivot=songs", "invalid_request"},
		{"kind=view&libraryId=movies&pivot=categories", "invalid_request"},
		{"kind=view&libraryId=movies&pivot=discover", "invalid_request"},
		{base + "&owner=someone-else", "invalid_cursor"},
		{base + "&profileId=someone-else", "invalid_cursor"},
		{base + "&pinned=yes", "invalid_cursor"},
		{base + "&pinned=", "invalid_cursor"},
	}
	for _, limit := range []string{"", "0", "-1", "101", "abc", "1.5", "99999999999999999999999"} {
		cases = append(cases, struct{ query, code string }{base + "&limit=" + limit, "invalid_cursor"})
	}
	for _, duplicate := range []string{"kind=view", "libraryId=movies", "pivot=movies", "limit=1&limit=1", "cursor=&cursor=", "pinned=true&pinned=true"} {
		cases = append(cases, struct{ query, code string }{base + "&" + duplicate, "invalid_cursor"})
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			w := f.raw("GET", "/v1/saved-resources?"+tc.query, f.owner.AccessToken, nil, nil)
			if w.Code != 400 || v1Code(w) != tc.code {
				t.Fatalf("query rejection = %d %s, want 400 %s", w.Code, w.Body.String(), tc.code)
			}
		})
	}
	for _, suffix := range []string{"", "&limit=1", "&limit=100"} {
		presetHTTPIDs(t, presetHTTPPage(f, f.owner.AccessToken, base+suffix))
	}
}

func TestSavedViewPresetsLibraryRevocationAndUnknownLibrary(t *testing.T) {
	f := newV1Fixture(t, 0)
	f.catalogTest.Library("still-allowed", "Still allowed", "movie", t.TempDir())
	f.catalogTest.Drain()
	member, device := presetHTTPMember(t, f, f.library, "still-allowed")
	created := presetHTTPCreate(t, f, device.AccessToken, "revocation-create", "Member's preset", presetHTTPDefinition(f.library, "movies"))
	query := "kind=view&libraryId=" + f.library + "&pivot=movies"
	presetHTTPIDs(t, presetHTTPPage(f, device.AccessToken, query), created.ResourceID)
	if err := f.id.UpdateDirectMember(context.Background(), f.owner.AccessToken, member.ID, identity.DirectMemberInput{
		ExpectedRevision: member.Revision, AllowedLibraries: []string{"still-allowed"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.id.Authenticate(device.AccessToken); err != nil {
		t.Fatalf("library revocation should leave the member's session authenticated: %v", err)
	}
	for _, library := range []string{f.library, "unknown-library"} {
		for _, pivot := range []string{"", "&pivot=movies"} {
			w := f.raw("GET", "/v1/saved-resources?kind=view&libraryId="+library+pivot, device.AccessToken, nil, nil)
			if w.Code != 404 || v1Code(w) != "not_found" {
				t.Fatalf("revoked/unknown library %s: %d %s", library, w.Code, w.Body.String())
			}
		}
	}
	f.call("GET", "/v1/saved-resources?kind=view&libraryId=unknown-library&pivot=movies", nil, nil, 404, nil)
	presetHTTPIDs(t, presetHTTPPage(f, device.AccessToken, "kind=view&libraryId=still-allowed&pivot=movies"))
	// Personal management remains available after access loss, as before.
	presetHTTPIDs(t, presetHTTPPage(f, device.AccessToken, "kind=view"), created.ResourceID)
}

func TestSavedViewPresetsCursorScopeAndRevision(t *testing.T) {
	f := newV1Fixture(t, 0)
	f.catalogTest.Library("other-movies", "Other movies", "movie", t.TempDir())
	f.catalogTest.Drain()
	otherDevice := f.device("cursor-second-device")
	otherProfile := presetHTTPOtherProfile(t, f)
	_, otherAccount := presetHTTPMember(t, f, f.library, "other-movies")
	def := presetHTTPDefinition(f.library, "movies")
	first := presetHTTPCreate(t, f, f.owner.AccessToken, "cursor-first", "A first", def)
	second := presetHTTPCreate(t, f, f.owner.AccessToken, "cursor-second", "B second", def)
	query := "kind=view&libraryId=" + f.library + "&pivot=movies&limit=1"
	page := presetHTTPPage(f, f.owner.AccessToken, query)
	presetHTTPIDs(t, page, first.ResourceID)
	if page.NextCursor == "" {
		t.Fatal("expected a continuation for the second matching preset")
	}
	cursor := url.QueryEscape(page.NextCursor)
	continued := presetHTTPPage(f, otherDevice.AccessToken, query+"&cursor="+cursor)
	presetHTTPIDs(t, continued, second.ResourceID)
	if continued.NextCursor != "" || continued.ViewerFence != page.ViewerFence {
		t.Fatalf("same-profile continuation changed scope or did not finish: %+v", continued)
	}
	for _, mismatch := range []struct{ name, token, query string }{
		{"library", f.owner.AccessToken, "kind=view&libraryId=other-movies&pivot=movies&limit=1"},
		{"pivot", f.owner.AccessToken, "kind=view&libraryId=movies&pivot=collections&limit=1"},
		{"omitted pivot", f.owner.AccessToken, "kind=view&libraryId=movies&limit=1"},
		{"generic directory", f.owner.AccessToken, "kind=view&limit=1"},
		{"kind", f.owner.AccessToken, "kind=collection&limit=1"},
		{"limit", f.owner.AccessToken, "kind=view&libraryId=movies&pivot=movies&limit=2"},
		{"pinned", f.owner.AccessToken, query + "&pinned=true"},
		{"profile", otherProfile.AccessToken, query},
		{"account", otherAccount.AccessToken, query},
	} {
		t.Run(mismatch.name, func(t *testing.T) {
			w := f.raw("GET", "/v1/saved-resources?"+mismatch.query+"&cursor="+cursor, mismatch.token, nil, nil)
			if w.Code != 400 || v1Code(w) != "invalid_cursor" {
				t.Fatalf("scope mismatch = %d %s, want 400 invalid_cursor", w.Code, w.Body.String())
			}
		})
	}
	// A generic directory cursor cannot be repurposed for a scoped picker.
	generic := presetHTTPPage(f, f.owner.AccessToken, "kind=view&limit=1")
	if generic.NextCursor == "" {
		t.Fatal("generic directory must provide a continuation")
	}
	w := f.raw("GET", "/v1/saved-resources?"+query+"&cursor="+url.QueryEscape(generic.NextCursor), f.owner.AccessToken, nil, nil)
	if w.Code != 400 || v1Code(w) != "invalid_cursor" {
		t.Fatalf("generic cursor in scoped listing = %d %s", w.Code, w.Body.String())
	}
	for _, bad := range []string{"not-a-cursor", page.NextCursor + "tampered"} {
		w = f.raw("GET", "/v1/saved-resources?"+query+"&cursor="+url.QueryEscape(bad), f.owner.AccessToken, nil, nil)
		if w.Code != 400 || v1Code(w) != "invalid_cursor" {
			t.Fatalf("malformed cursor = %d %s", w.Code, w.Body.String())
		}
	}
	var renamed presetHTTPReceipt
	f.call("PATCH", "/v1/saved-resources/"+second.ResourceID, nil, map[string]any{
		"operationId": "cursor-rename", "expectedRevision": second.Revision, "name": "C renamed",
	}, 200, &renamed)
	w = f.raw("GET", "/v1/saved-resources?"+query+"&cursor="+cursor, otherDevice.AccessToken, nil, nil)
	if w.Code != 409 || v1Code(w) != "stale_continuation" {
		t.Fatalf("stale cursor = %d %s, want 409 stale_continuation", w.Code, w.Body.String())
	}
	fresh := presetHTTPPage(f, otherDevice.AccessToken, query)
	presetHTTPIDs(t, fresh, first.ResourceID)
	if fresh.Revision <= page.Revision || fresh.NextCursor == "" {
		t.Fatalf("fresh page did not capture the changed revision: %+v", fresh)
	}
}
