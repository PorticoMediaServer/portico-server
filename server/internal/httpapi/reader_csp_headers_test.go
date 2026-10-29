package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// policyDirectives splits a policy into directive name and its source list.
func policyDirectives(policy string) map[string]string {
	directives := make(map[string]string)
	for _, directive := range strings.Split(policy, ";") {
		if name, value, ok := strings.Cut(strings.TrimSpace(directive), " "); ok {
			directives[name] = value
		}
	}
	return directives
}

// A reading book renders in a same-origin blob iframe: the publication's
// rewritten stylesheets and deobfuscated fonts reach it as blob URLs the web
// document creates, and a blob document inherits the policy of the document
// that made it. The header therefore has to name blob: for styles and fonts,
// and it must not buy that with inline script, a remote font host or a looser
// script-src.
func TestWebDocumentAllowsTheReadersBlobStylesAndFonts(t *testing.T) {
	root := t.TempDir()
	document := `<html><head><meta name="portico-style-nonce" content="__PORTICO_STYLE_NONCE__"></head><body>Portico</body></html>`
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Dependencies{WebDirectory: root}.installationRoutes(mux)
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	policy := w.Header().Get("Content-Security-Policy")
	directives := policyDirectives(policy)
	if strings.Contains(policy, "'unsafe-inline'") {
		t.Fatalf("the web document allows inline sources: %s", policy)
	}
	for _, source := range []string{"'self'", "'nonce-", "blob:"} {
		if !strings.Contains(directives["style-src"], source) {
			t.Errorf("style-src lost %s: %q", source, directives["style-src"])
		}
	}
	for _, source := range []string{"'self'", "blob:", "data:"} {
		if !strings.Contains(directives["font-src"], source) {
			t.Errorf("font-src lost %s: %q", source, directives["font-src"])
		}
	}
	if strings.Contains(directives["font-src"], "https:") {
		t.Errorf("fonts may come from a remote host: %q", directives["font-src"])
	}
	// Every directive the app already relied on keeps its exact value; only the
	// reader's two local schemes above were added.
	for _, want := range []string{
		"default-src='self'",
		"connect-src='self' https://*.direct.getportico.tv https://web.getportico.tv",
		"img-src='self' blob: data: https://*.direct.getportico.tv",
		"media-src='self' blob:",
		"worker-src='self' blob:",
		"frame-src='self' blob:",
		"frame-ancestors='none'",
		"base-uri='none'",
		"object-src='none'",
		"form-action='self'",
	} {
		name, sources, ok := strings.Cut(want, "=")
		if !ok || directives[name] != sources {
			t.Errorf("%s changed:\n got  %q\n want %q", name, directives[name], sources)
		}
	}
	// Scripts are still this origin, WebAssembly for the subtitle renderer, and
	// the Cast sender's three checked paths. The comparison is exact so an
	// inline, wildcard or new remote source cannot slip in unnoticed.
	const scripts = "'self' 'wasm-unsafe-eval' https://www.gstatic.com/cv/js/sender/ https://www.gstatic.com/eureka/clank/ https://www.gstatic.com/cast/sdk/libs/sender/"
	if directives["script-src"] != scripts {
		t.Errorf("script-src changed:\n got  %q\n want %q", directives["script-src"], scripts)
	}
}

// Responses that are not the nonce document (API answers, hashed assets) carry
// the same constant, so the reader's blob styles and fonts resolve there too
// without a second, duplicated directive.
func TestEveryWebResponseCarriesTheReaderBlobPolicy(t *testing.T) {
	w := httptest.NewRecorder()
	New(Dependencies{}).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	policy := w.Header().Get("Content-Security-Policy")
	if policy == "" {
		t.Fatal("this response carries no policy")
	}
	directives := policyDirectives(policy)
	if directives["style-src"] != "'self' blob:" {
		t.Errorf("style-src: %q", directives["style-src"])
	}
	if directives["font-src"] != "'self' blob: data:" {
		t.Errorf("font-src: %q", directives["font-src"])
	}
	if strings.Contains(policy, "'unsafe-inline'") {
		t.Fatalf("an inline source is allowed: %s", policy)
	}
}
