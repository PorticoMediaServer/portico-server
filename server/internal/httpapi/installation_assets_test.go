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
	for name, body := range map[string]string{
		"index.html": "<main>Portico</main>", "manifest.json": "{}",
		// Named as the build names them: a content hash before the extension.
		"assets/index-Bq3xK9aZ.js": "export default 1", "assets/Player-C3d_x-9Q.js": "export default 2", "assets/Player-C3d_x-9Q.js.map": "{}",
		"assets/Auth-BIi20xdI.css": "main{}", "assets/manrope-var-latin-BxY7Qf2k.woff2": "font",
		// Copied from the public folder under their own, unversioned names.
		"assets/tmdb.svg": "<svg/>", "assets/portico-wordmark-mono-white.svg": "<svg/>", "assets/index-test.js": "export default 3",
	} {
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
		{"/assets/index-Bq3xK9aZ.js", "*/*", 200, "export default 1", "public, max-age=31536000, immutable"},
		{"/assets/Player-C3d_x-9Q.js", "*/*", 200, "export default 2", "public, max-age=31536000, immutable"},
		{"/assets/Player-C3d_x-9Q.js.map", "*/*", 200, "{}", "public, max-age=31536000, immutable"},
		{"/assets/Auth-BIi20xdI.css", "*/*", 200, "main{}", "public, max-age=31536000, immutable"},
		{"/assets/manrope-var-latin-BxY7Qf2k.woff2", "*/*", 200, "font", "public, max-age=31536000, immutable"},
		{"/assets/tmdb.svg", "*/*", 200, "<svg/>", "no-cache"},
		{"/assets/portico-wordmark-mono-white.svg", "*/*", 200, "<svg/>", "no-cache"},
		{"/assets/index-test.js", "*/*", 200, "export default 3", "no-cache"},
		{"/manifest.json", "*/*", 200, "{}", "no-cache"},
		{"/index.html", "text/html", 200, "<main>Portico</main>", "no-cache"},
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
				t.Fatalf("cache policy %q, want %q", w.Header().Get("Cache-Control"), tc.cache)
			}
			if tc.status == 200 && w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing content-type protection")
			}
		})
	}
}

// A file the web build copies from its public folder keeps its own name, so
// it can change without its URL changing. None may be named like a
// content-hashed file, or it would be cached for a year.
func TestUnversionedWebAssetsAreNotNamedLikeHashedOnes(t *testing.T) {
	public := filepath.Join("..", "..", "..", "web", "public")
	if _, err := os.Stat(public); err != nil {
		t.Skip("the web app's sources are not beside this module")
	}
	err := filepath.WalkDir(public, func(file string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		name, err := filepath.Rel(public, file)
		if err != nil {
			return err
		}
		if name = filepath.ToSlash(name); contentHashedWebAsset(name) {
			t.Errorf("%s would be served as immutable; rename it so its last hyphenated part is not eight characters", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
