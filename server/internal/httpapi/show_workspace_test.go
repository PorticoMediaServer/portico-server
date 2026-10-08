package httpapi

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
	"testing"
	"time"
)

func TestShowWorkspaceHTTPPermissionAndParentResolution(t *testing.T) {
	root := t.TempDir()
	db, e := persistence.Open(filepath.Join(root, "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ident, e := identity.New(db, root)
	if e != nil {
		t.Fatal(e)
	}
	c := catalogtest.New(t, db)
	a := c.Library("a", "TV", "tv", "/a")
	b := c.Library("b", "Hidden", "tv", "/b")
	showA := c.Show(a, "Visible", 0)
	showB := c.Show(b, "Hidden", 0)
	seasonA := c.Season(showA, 1)
	seasonB := c.Season(showB, 1)
	c.Drain()
	_, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	control, e := hosted.New(db, ident, "http://127.0.0.1:19410", tl6HostedRootPin(), tl6HostedRootID())
	if e != nil {
		t.Fatal(e)
	}
	policy := hosted.Policy{ServerID: ident.ID(), Revision: 1, IssuedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), Members: []hosted.Member{{AccountID: "account", ProfileID: "profile", Role: "member", AllowedLibraries: []string{"a"}}}}
	apply := func() {
		raw, _ := json.Marshal(policy)
		if e = control.Apply(tl6CertifiedHostedPolicy(t, priv, "fixture", raw)); e != nil {
			t.Fatal(e)
		}
	}
	apply()
	session, e := tl6IssueHostedFixture(t, db, ident, "account", "profile", "member")
	if e != nil {
		t.Fatal(e)
	}
	h := New(Dependencies{DB: db, Identity: ident, Catalog: catalog.New(db), Hosted: control})
	read := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	path := "/v1/libraries/a/show-workspace?seasonId=" + seasonA.Public
	if w := read(path, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := read(path, session.AccessToken)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out catalog.ShowWorkspace
	if e = json.Unmarshal(w.Body.Bytes(), &out); e != nil || out.Show.ID != showA.Public || out.Scope.LibraryID != "a" || out.Selected.SeasonID == nil || *out.Selected.SeasonID != seasonA.Public || out.Scope.ViewerFence == "" {
		t.Fatal(out, e)
	}
	for _, bad := range []string{"/v1/libraries/b/show-workspace?showId=" + showB.Public, "/v1/libraries/a/show-workspace?showId=" + showA.Public + "&seasonId=" + seasonB.Public, path + "&seasonId=" + seasonB.Public, path + "&limit=1000", path + "&unknown=1"} {
		if w = read(bad, session.AccessToken); w.Code == 200 {
			t.Fatal("unauthorized/invalid context accepted", bad)
		}
	}
	policy.Revision++
	policy.Members = nil
	apply()
	if w = read(path, session.AccessToken); w.Code != 401 {
		t.Fatal("revoked member retained show workspace", w.Code)
	}
}
