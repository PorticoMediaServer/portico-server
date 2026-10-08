package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledWebAssetsStayInsidePublicRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"index.html": "<main>Portico</main>", "assets/index-test.js": "export default 1"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(outside, []byte("PRIVATE DATA"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "assets", "escape.txt")); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Dependencies{WebDirectory: root}.installationRoutes(mux)
	for _, tc := range []struct {
		path, accept string
		status       int
		body, cache  string
	}{
		{"/", "text/html", 200, "<main>Portico</main>", "no-cache"},
		{"/libraries/example/items/title", "text/html", 200, "<main>Portico</main>", "no-cache"},
		{"/assets/index-test.js", "*/*", 200, "export default 1", "public, max-age=31536000, immutable"},
		{"/assets/missing.js", "text/html", 404, "", ""},
		{"/assets/escape.txt", "text/html", 404, "", ""},
		{"/assets/", "text/html", 404, "", ""},
		{"/missing", "application/json", 404, "", ""},
		{"/v2/playback/missing", "text/html", 404, "", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest("GET", tc.path, nil)
			r.Header.Set("Accept", tc.accept)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status || (tc.body != "" && w.Body.String() != tc.body) {
				t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "PRIVATE DATA") {
				t.Fatal("escaped public root")
			}
			if tc.cache != "" && w.Header().Get("Cache-Control") != tc.cache {
				t.Fatal("wrong cache policy")
			}
			if tc.status == 200 && w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing content-type protection")
			}
		})
	}
}
