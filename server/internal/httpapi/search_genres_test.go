package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

func TestSearchGenresHandlerUsesCurrentLibraryAuthority(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ident, err := identity.New(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := catalogtest.New(t, db)
	allowed := c.Library("allowed", "Allowed", "movie", "/allowed")
	private := c.Library("private", "Private", "movie", "/private")
	one := c.Movie(allowed, "/allowed/one.mkv", "One", 2026)
	secret := c.Movie(private, "/private/secret.mkv", "Secret", 2026)
	c.Genres(one.ID, "fixture", "Drama")
	c.Genres(secret.ID, "fixture", "Secret genre")
	c.Drain()
	control, err := hosted.New(db, ident, "http://127.0.0.1:19410", tl6HostedRootPin(), tl6HostedRootID())
	if err != nil {
		t.Fatal(err)
	}
	apply := func(revision int64, libraries []string) {
		policy := hosted.Policy{ServerID: ident.ID(), Revision: revision, IssuedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), Members: []hosted.Member{{AccountID: "member", ProfileID: "p", Role: "member", AllowedLibraries: libraries}}}
		raw, _ := json.Marshal(policy)
		if err := storeHTTPFixturePolicy(db, raw); err != nil {
			t.Fatal(err)
		}
	}
	apply(1, []string{"allowed"})
	session, err := tl6IssueHostedFixture(t, db, ident, "member", "p", "member")
	if err != nil {
		t.Fatal(err)
	}
	d := Dependencies{DB: db, Identity: ident, Catalog: catalog.New(db), Hosted: control}
	request := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		d.searchGenresHandler(w, r)
		return w
	}
	path := "/v1/search/genres?limit=8"
	if w := request(path, ""); w.Code != 401 {
		t.Fatal("anonymous genre disclosure", w.Code, w.Body.String())
	}
	read := func() catalog.SearchGenrePage {
		w := request(path, session.AccessToken)
		var page catalog.SearchGenrePage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		return page
	}
	if page := read(); len(page.Genres) != 1 || page.Genres[0].Value != "Drama" || page.Genres[0].Count != 1 {
		t.Fatal("genre authority diverged from browse", page)
	}
	apply(2, []string{"private"})
	if page := read(); len(page.Genres) != 1 || page.Genres[0].Value != "Secret genre" {
		t.Fatal("cached genres survived a library revocation", page)
	}
	for _, query := range []string{"?limit=0", "?limit=51", "?limit=8&limit=4", "?source=/private", "?cursor=forged"} {
		if w := request("/v1/search/genres"+query, session.AccessToken); w.Code != 400 {
			t.Fatal("unbounded or invalid request accepted", query, w.Code, w.Body.String())
		}
	}
}
