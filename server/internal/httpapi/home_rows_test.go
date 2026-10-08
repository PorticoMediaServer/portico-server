package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"portico.local/server/internal/apispec"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/persistence"
	"strings"
	"testing"
)

func homeAPIFixture(t *testing.T) (http.Handler, identity.Envelope, catalogtest.Names) {
	t.Helper()
	root := t.TempDir()
	db, e := persistence.Open(filepath.Join(root, "db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	id, e := identity.New(db, root)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO accounts VALUES('owner','owner',x'00','profile',1)`); e != nil {
		t.Fatal(e)
	}
	c := catalogtest.New(t, db)
	movies := c.Library("movies", "Movies", "movie", "/movies")
	names := catalogtest.Names{
		"m1": c.Movie(movies, "/movies/m1.mp4", "First", 2001),
		"m2": c.Movie(movies, "/movies/m2.mp4", "Second", 2002),
		"m3": c.Movie(movies, "/movies/m3.mp4", "Third", 2003),
	}
	for i, name := range []string{"m1", "m2", "m3"} {
		added := []string{"2026-01-01T00:00:00.000Z", "2026-01-02T00:00:00.000Z", "2026-01-03T00:00:00.000Z"}[i]
		c.Fields(names[name].ID, map[string]any{"backdrop_url": "/b", "added_text": added})
		c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,revision) VALUES(?,?,1,0,1)`, identity.PersonalKey(identity.Viewer{Authority: "local", AccountID: "owner", ProfileID: "profile"}), names[name].ID)
	}
	c.Attributes(names["m1"].ID, "contentRating", "PG-13")
	c.Genres(names["m1"].ID, "fixture", "Drama", "Comedy", "Thriller")
	c.Drain()
	owner, e := id.Issue("owner", "profile", "local", "owner", 1)
	if e != nil {
		t.Fatal(e)
	}
	return New(Dependencies{DB: db, Identity: id, Catalog: catalog.New(db), Console: operations.New(db)}), owner, names
}

func homeAPIRequest(t *testing.T, h http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, path, reader)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// The home surface, its per-row paging and its customisation are all reachable
// over HTTP, and the layout endpoint refuses required rows by name.
func TestHomeRoutesComposePageAndCustomise(t *testing.T) {
	handler, owner, names := homeAPIFixture(t)
	w := homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/home?limit=2", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var document catalog.HomeDocument
	if e := json.Unmarshal(w.Body.Bytes(), &document); e != nil {
		t.Fatal(e)
	}
	if len(document.Rows) == 0 || document.GeneratedAt == "" {
		t.Fatal("empty home", w.Body.String())
	}
	var recent *catalog.HomeRow
	for index := range document.Rows {
		if document.Rows[index].ID == "recent_movies" {
			recent = &document.Rows[index]
		}
	}
	if recent == nil || len(recent.Entries) != 2 || recent.Total != 3 || recent.Endpoint != "/v1/home/rows/recent_movies" {
		t.Fatalf("recent row: %+v", recent)
	}
	w = homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/home/rows/recent_movies?limit=2&cursor="+recent.NextCursor, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var page catalog.HomeRow
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || len(page.Entries) != 1 || page.Start != 2 {
		t.Fatal(e, w.Body.String())
	}
	if w = homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/home/rows/nonexistent", ""); w.Code != 404 {
		t.Fatal("unknown row", w.Code, w.Body.String())
	}
	if w = homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/home/rows/trending", ""); w.Code != 404 {
		t.Fatal("gated community row reachable", w.Code)
	}
	w = homeAPIRequest(t, handler, owner.AccessToken, "PUT", "/v1/home/layout", `{"expectedRevision":1,"idempotencyKey":"k1","rowOrder":["recent_movies"],"hiddenRowIds":["continue"]}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_home_layout") {
		t.Fatal("required row hidden", w.Code, w.Body.String())
	}
	w = homeAPIRequest(t, handler, owner.AccessToken, "PUT", "/v1/home/layout", `{"expectedRevision":1,"idempotencyKey":"k2","rowOrder":["trending_now","watchlist"],"hiddenRowIds":["recent_movies"]}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var layout operations.HomeLayoutDocument
	if e := json.Unmarshal(w.Body.Bytes(), &layout); e != nil || layout.Revision != 2 || len(layout.HiddenRowIDs) != 1 {
		t.Fatal(e, w.Body.String())
	}
	w = homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/home", "")
	if e := json.Unmarshal(w.Body.Bytes(), &document); e != nil {
		t.Fatal(e)
	}
	// trending_now is empty for this viewer and "watchlist" is a row Home no longer
	// has (My List is a Saved tab): the layout saves and the server's order leads.
	if document.Rows[0].ID != "continue" || document.Layout.Revision != 2 {
		t.Fatalf("saved layout not applied: %+v", document.Layout)
	}
	for _, row := range document.Rows {
		if row.ID == "recent_movies" {
			t.Fatal("hidden row published over HTTP")
		}
	}
	w = homeAPIRequest(t, handler, owner.AccessToken, "PUT", "/v1/home/layout", `{"expectedRevision":1,"idempotencyKey":"k3","rowOrder":[],"hiddenRowIds":[]}`)
	if w.Code != 409 {
		t.Fatal("stale layout revision accepted", w.Code, w.Body.String())
	}
	w = homeAPIRequest(t, handler, owner.AccessToken, "POST", "/v1/home/layout/reset", `{"idempotencyKey":"k4"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := json.Unmarshal(w.Body.Bytes(), &layout); e != nil || len(layout.RowOrder) != 0 || len(layout.HiddenRowIDs) != 0 {
		t.Fatal("reset did not clear the layout", e, w.Body.String())
	}
	w = homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/suggestions?limit=5", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "generatedAt") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/items/"+names["m1"].Public+"/recommendations", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "\"rows\"") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = homeAPIRequest(t, handler, "", "GET", "/v1/home", ""); w.Code != 401 {
		t.Fatal("home served without a session", w.Code)
	}
	if w = homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/home?limit=999", ""); w.Code != 200 {
		t.Fatal("oversized limit was not clamped", w.Code)
	}
}

// CON-23: the layout lists every row the viewer can arrange, including ones with no entries
// (trending_now here), in the saved order, and marks the hidden ones.
func TestHomeLayoutListsEveryArrangeableRow(t *testing.T) {
	handler, owner, _ := homeAPIFixture(t)
	w := homeAPIRequest(t, handler, owner.AccessToken, "PUT", "/v1/home/layout", `{"expectedRevision":1,"idempotencyKey":"k1","rowOrder":["trending_now","recent_movies"],"hiddenRowIds":["recommended"]}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/home/layout", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	tl11AssertHomeSpecResponse(t, "GET", "/v1/home/layout", w)
	var view catalog.HomeLayoutView
	if e := json.Unmarshal(w.Body.Bytes(), &view); e != nil {
		t.Fatal(e)
	}
	if view.Revision != 2 || len(view.RowOrder) != 2 || len(view.HiddenRowIDs) != 1 {
		t.Fatalf("layout: %+v", view)
	}
	ids := map[string]catalog.HomeLayoutRow{}
	for _, row := range view.Rows {
		ids[row.ID] = row
	}
	if len(view.Rows) < 2 || view.Rows[0].ID != "trending_now" || view.Rows[1].ID != "recent_movies" {
		t.Fatalf("saved order first: %+v", view.Rows)
	}
	if _, ok := ids["trending_now"]; !ok {
		t.Fatal("an empty row is missing from the layout")
	}
	if !ids["recommended"].Hidden || ids["continue"].Hidden || !ids["continue"].Required || ids["recent_movies"].LibraryID != "movies" || ids["recent_movies"].Hidden {
		t.Fatalf("row flags: %+v", ids)
	}
	// Home itself still omits the empty row.
	w = homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/home", "")
	if strings.Contains(w.Body.String(), `"id":"trending_now"`) {
		t.Fatal("home published an empty row")
	}
	if w = homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/home/layout?x=1", ""); w.Code != 400 {
		t.Fatal("unknown query accepted", w.Code)
	}
}

// M5: Home entries carry the year, the content rating, the first two genres and the viewer's
// My List state, so the hero can show its meta line and a My List toggle.
func TestHomeEntriesCarryHeroFields(t *testing.T) {
	handler, owner, names := homeAPIFixture(t)
	w := homeAPIRequest(t, handler, owner.AccessToken, "GET", "/v1/home/rows/recent_movies", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	tl11AssertHomeSpecResponse(t, "GET", "/v1/home/rows/{id}", w)
	var row catalog.HomeRow
	if e := json.Unmarshal(w.Body.Bytes(), &row); e != nil || len(row.Entries) == 0 {
		t.Fatal(e, w.Body.String())
	}
	sawRated := false
	for _, entry := range row.Entries {
		sawRated = sawRated || entry.ID == names["m1"].Public
		if entry.Year == nil || *entry.Year < 2001 || entry.Watchlisted == nil || !*entry.Watchlisted {
			t.Fatalf("entry without its hero fields: %+v", entry)
		}
		if entry.ID == names["m1"].Public && (entry.ContentRating != "PG-13" || len(entry.Genres) != 2 || entry.Genres[0] != "Drama" || entry.Genres[1] != "Comedy") {
			t.Fatalf("m1 rating and genres: %+v", entry)
		}
	}
	if !sawRated {
		t.Fatal("the row lost m1")
	}
}

func tl11AssertHomeSpecResponse(t *testing.T, method, path string, w *httptest.ResponseRecorder) {
	t.Helper()
	if err := apispec.ValidateErrorEnvelope(w.Code, w.Body.Bytes()); method != "HEAD" && err != nil {
		t.Fatalf("%s %s %d: %v", method, path, w.Code, err)
	}
	doc, schema, err := apispec.Response(method, path, w.Code)
	if err != nil {
		t.Fatal(err)
	}
	if problems := doc.ValidateJSON(schema, w.Body.Bytes()); len(problems) > 0 {
		t.Fatalf("%s %s %d does not match %s: %v\n%s", method, path, w.Code, doc.File, problems, w.Body.String())
	}
}
