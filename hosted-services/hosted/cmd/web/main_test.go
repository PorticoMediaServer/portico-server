package main

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServicesHTMLReloadsAndAssetTypes(t *testing.T) {
	root := t.TempDir()
	index := `<html><script src="/browser-check.js"></script><script type="module" src="/assets/index-test.js"></script></html>`
	for name, body := range map[string]string{
		"index.html": index, "browser-check.js": "window.browserCheck = true;",
		"assets/index-test.js": "export {};", "assets/index-test.css": "body{}",
		"assets/font.woff2": "wOF2", "assets/wordmark.svg": `<svg xmlns="http://www.w3.org/2000/svg"/>`,
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	handler := webHandler(root, "")
	for _, route := range []string{"/", "/account", "/device", "/reset", "/verify-email", "/claim", "/join", "/app/account-return"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+route, func(t *testing.T) {
				request := httptest.NewRequest(method, route, nil)
				request.Header.Set("Accept", "text/html")
				out := httptest.NewRecorder()
				handler.ServeHTTP(out, request)
				if out.Code != 200 || !strings.HasPrefix(out.Header().Get("Content-Type"), "text/html") || out.Header().Get("Cache-Control") != "no-cache" || out.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatal(out.Code, out.Header())
				}
				if method == http.MethodGet && out.Body.String() != index || method == http.MethodHead && out.Body.Len() != 0 {
					t.Fatal("incorrect root HTML fallback body")
				}
			})
		}
	}
	for asset, types := range map[string]string{
		"/browser-check.js": "text/javascript application/javascript", "/assets/index-test.js": "text/javascript application/javascript",
		"/assets/index-test.css": "text/css", "/assets/font.woff2": "font/woff2", "/assets/wordmark.svg": "image/svg+xml",
	} {
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, httptest.NewRequest(http.MethodGet, asset, nil))
		kind, _, err := mime.ParseMediaType(out.Header().Get("Content-Type"))
		if err != nil || out.Code != 200 || !strings.Contains(types, kind) || out.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal(asset, out.Code, out.Header())
		}
		immutable := strings.HasPrefix(asset, "/assets/index-")
		if immutable != strings.Contains(out.Header().Get("Cache-Control"), "immutable") {
			t.Fatal("incorrect asset cache policy", asset)
		}
	}
	for _, path := range []string{"/assets/missing.js", "/browser-check-missing.js"} {
		out := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if strings.HasPrefix(path, "/assets/") {
			request.Header.Set("Accept", "text/html")
		}
		handler.ServeHTTP(out, request)
		if out.Code != 404 {
			t.Fatal("a missing asset became HTML", path, out.Code)
		}
	}
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader("{}")))
	if out.Code != 405 {
		t.Fatal("the static host accepted an API mutation", out.Code)
	}
}

func TestAppleAppSiteAssociation(t *testing.T) {
	path := "/.well-known/apple-app-site-association"
	for _, team := range []string{"", "placeholder", "ABC1234567"} {
		w := httptest.NewRecorder()
		webHandler(t.TempDir(), team).ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if team == "ABC1234567" {
			if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
				t.Fatal(w.Code, w.Header())
			}
			var got map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			details := got["applinks"].(map[string]any)["details"].([]any)
			entry := details[0].(map[string]any)
			if entry["appIDs"].([]any)[0] != "ABC1234567.tv.getportico.portico.ios" || len(entry["components"].([]any)) != 5 {
				t.Fatal(got)
			}
		} else if w.Code != 404 {
			t.Fatal(team, w.Code)
		}
	}
}
