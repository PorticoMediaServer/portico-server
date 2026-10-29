package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/compactcatalog"
	"strings"
	"testing"

	"portico.local/server/internal/identity"
)

func TestReadingBrowseHTTPUsesLiveReadyAuthorityForAllFourSurfaces(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	readingSeriesOwnerField(t, r, r.item, "author", "Protected Reading author")
	readingSeriesOwnerField(t, r, r.item, "series", "Protected Reading series")
	readingSeriesDrain(t, r)
	h := New(r.d)
	type target struct {
		method, path string
		body         any
	}
	base := "/v1/libraries/" + r.library
	targets := []target{{"GET", base + "/browse-capabilities", nil}, {"POST", base + "/browse", map[string]any{"pivot": "books", "limit": 1}}, {"GET", base + "/facets?field=author", nil}, {"POST", base + "/browse/counts", map[string]any{"pivot": "books", "queries": []any{map[string]any{"id": "base"}}}}}
	check := func(token string, status int, hidden bool) {
		t.Helper()
		for _, target := range targets {
			w := readingHTTP(t, h, target.method, target.path, token, "", target.body)
			if w.Code != status {
				t.Fatalf("%s %s: %d %s", target.method, target.path, w.Code, w.Body.String())
			}
			if target.method == "POST" && target.path == base+"/browse" && strings.Contains(w.Body.String(), `"wholeSelection"`) {
				t.Fatal("Reading advertised an unsupported whole-view selector")
			}
			if hidden {
				for _, secret := range []string{r.item, r.source, "Protected Reading author", "Protected Reading series", compactcatalog.ReadingSeriesID(r.library, "Protected Reading series")} {
					if strings.Contains(w.Body.String(), secret) {
						t.Fatalf("hidden Reading fact escaped through %s", target.path)
					}
				}
			}
		}
	}
	check(r.f.owner.AccessToken, 200, false)
	initial := readingHTTP(t, h, "POST", base+"/browse", r.f.owner.AccessToken, "", map[string]any{"pivot": "books", "limit": 1})
	var held catalog.BrowseResult
	if initial.Code != 200 || json.Unmarshal(initial.Body.Bytes(), &held) != nil || held.PageInfo.Revision == "" {
		t.Fatalf("initial revision: %d %s", initial.Code, initial.Body.String())
	}
	check("", 401, true)
	readingSeriesOwnerField(t, r, r.item, "author", "Changed protected author")
	check(r.f.owner.AccessToken, http.StatusOK, false)
	// Unconditional reads keep the last published attribution. The pending
	// owner edit is not falsely served as part of that immutable image.
	w := readingHTTP(t, h, "POST", base+"/browse", r.f.owner.AccessToken, "", map[string]any{"pivot": "books", "limit": 1})
	var updating catalog.BrowseResult
	if err := json.Unmarshal(w.Body.Bytes(), &updating); err != nil || !updating.Importing || len(updating.Entries) != 1 || updating.Entries[0].Author != "Protected Reading author" {
		t.Fatalf("published image while importing: %d %+v %v", w.Code, updating, err)
	}
	conditional := readingHTTP(t, h, "POST", base+"/browse", r.f.owner.AccessToken, "", map[string]any{"pivot": "books", "limit": 1, "range": map[string]any{"start": 0, "revision": held.PageInfo.Revision}})
	if conditional.Code != 412 || conditional.Header().Get("ETag") == "" || conditional.Header().Get("ETag") == "\""+held.PageInfo.Revision+"\"" {
		t.Fatalf("conditional mismatch lacks current revision: %d %s %s", conditional.Code, conditional.Header().Get("ETag"), conditional.Body.String())
	}
	readingSeriesDrain(t, r)
	owner := readingOwnerLogin(t, r)
	profiles, err := r.f.id.CreateDirectProfile(ctx, owner.AccountToken, "Browse reader", "blue")
	if err != nil {
		t.Fatal(err)
	}
	var child identity.DirectProfile
	for _, profile := range profiles.Profiles {
		if !profile.Primary && profile.Name == "Browse reader" {
			child = profile
		}
	}
	selected, err := r.f.id.SelectDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectSelection{})
	if err != nil {
		t.Fatal(err)
	}
	check(selected.Session.AccessToken, 200, false)
	if _, err = r.f.id.SetProfileRestrictions(ctx, owner.AccountToken, child.ID, identity.ProfileRestrictionEdit{ExpectedRevision: 1, AllowUnrated: false}); err != nil {
		t.Fatal(err)
	}
	check(selected.Session.AccessToken, 200, true)
	if _, err = r.f.id.EditDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectProfileEdit{ExpectedRevision: child.Revision, Policy: &identity.ProfilePolicy{AllowedLibraries: []string{}}}); err != nil {
		t.Fatal(err)
	}
	check(selected.Session.AccessToken, http.StatusNotFound, true)
	if err = r.f.id.LogoutToken(ctx, r.f.owner.AccessToken); err != nil {
		t.Fatal(err)
	}
	check(r.f.owner.AccessToken, 401, true)
}

func TestReadingBrowseFacetWindowHTTP(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	readingSeriesOwnerField(t, r, r.item, "author", "One author")
	readingSeriesDrain(t, r)
	h := New(r.d)
	path := "/v1/libraries/" + r.library + "/facets?field=author&pivot=authors&limit=1"
	var first catalog.FacetPage
	w := readingHTTP(t, h, "GET", path, r.f.owner.AccessToken, "", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Total != 1 || len(first.Values) != 1 {
		t.Fatalf("first window %+v", first)
	}
	w = readingHTTP(t, h, "GET", path+"&offset=1", r.f.owner.AccessToken, "", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var next catalog.FacetPage
	if err := json.Unmarshal(w.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if next.Total != 1 || len(next.Values) != 0 || next.Revision != first.Revision {
		t.Fatalf("next window %+v", next)
	}
	for _, offset := range []string{"-1", "no", "1&offset=2"} {
		w = readingHTTP(t, h, "GET", path+"&offset="+offset, r.f.owner.AccessToken, "", nil)
		if w.Code != 400 {
			t.Fatalf("invalid offset accepted %s: %d", offset, w.Code)
		}
	}
}

func TestReadingBrowseColdImageIsHonestBuildingOverHTTP(t *testing.T) {
	f := newV1Fixture(t, 0)
	library, err := f.cat.Create("Building Reading", "book", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, prepared := range []bool{false, true} {
		d := f.deps
		d.PrepareReadingBrowse = prepared
		h := New(d)
		base := "/v1/libraries/" + library.ID
		w := readingHTTP(t, h, "GET", base+"/browse-capabilities", f.owner.AccessToken, "", nil)
		var caps catalog.BrowseCapabilities
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &caps) != nil || !caps.Importing {
			t.Fatalf("prepared=%v cold capabilities: %d %s", prepared, w.Code, w.Body.String())
		}
		w = readingHTTP(t, h, "POST", base+"/browse", f.owner.AccessToken, "", map[string]any{"pivot": "books", "limit": 1})
		var page catalog.BrowseResult
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || !page.Importing || len(page.Entries) != 0 || !page.PageInfo.TotalIsLowerBound || page.WholeSelection != nil {
			t.Fatalf("prepared=%v cold page: %d %s", prepared, w.Code, w.Body.String())
		}
	}
}
