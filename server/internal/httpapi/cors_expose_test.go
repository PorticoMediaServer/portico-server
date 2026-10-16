package httpapi

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

// A web client on another origin reads the timeline's report cadence from a
// response header, which a browser hides unless the server exposes it.
func TestCrossOriginClientsCanReadTheReportCadence(t *testing.T) {
	root := t.TempDir()
	db, err := persistence.Open(filepath.Join(root, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := identity.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	host, err := hosted.New(db, id, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	h := New(Dependencies{DB: db, Identity: id, Catalog: catalog.New(db), Hosted: host, Origins: []string{"https://web.getportico.tv"}})
	exposed := func(origin string) []string {
		r := httptest.NewRequest("OPTIONS", "http://127.0.0.1:32500/v1/playback/sessions/s/timeline", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var names []string
		for _, name := range strings.Split(w.Header().Get("Access-Control-Expose-Headers"), ",") {
			if name = strings.TrimSpace(name); name != "" {
				names = append(names, name)
			}
		}
		return names
	}
	has := func(names []string, want string) bool {
		for _, name := range names {
			if strings.EqualFold(name, want) {
				return true
			}
		}
		return false
	}
	names := exposed("https://web.getportico.tv")
	for _, want := range []string{"Report-Every-Ms", "Retry-After", "ETag", "Idempotent-Replayed"} {
		if !has(names, want) {
			t.Fatalf("%s is not exposed to an allowed origin: %v", want, names)
		}
	}
	// A tool on any other origin may call with a token, so its preflight is
	// answered too; the real request is held to the token rule.
	if names = exposed("https://tool.example"); !has(names, "Report-Every-Ms") {
		t.Fatalf("a token-carrying tool cannot read the cadence: %v", names)
	}
}
