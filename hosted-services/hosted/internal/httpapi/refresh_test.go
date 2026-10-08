package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserCookieDevelopmentIsExplicit(t *testing.T) {
	h := &Handler{Origins: map[string]bool{"http://127.0.0.1:19412": true, "http://other.example": true, "https://web.getportico.tv": true}}
	r := httptest.NewRequest("POST", "/v1/sessions/refresh", nil)
	r.Header.Set("Origin", "http://127.0.0.1:19412")
	if _, _, ok := h.browserCookiePolicy(r); ok {
		t.Fatal("HTTP cookie permitted without explicit development")
	}
	h.DevelopmentCookies = true
	name, secure, ok := h.browserCookiePolicy(r)
	if !ok || secure || !strings.HasPrefix(name, "portico_refresh_dev_") {
		t.Fatal("development exception missing")
	}
	r.Header.Set("Origin", "http://other.example")
	if _, _, ok = h.browserCookiePolicy(r); ok {
		t.Fatal("remote HTTP exception")
	}
	r.Header.Set("Origin", "https://web.getportico.tv")
	name, secure, ok = h.browserCookiePolicy(r)
	if !ok || !secure || name != "__Host-portico_refresh" {
		t.Fatal("development flag weakened production cookie")
	}
	r.AddCookie(&http.Cookie{Name: name, Value: "one"})
	r.AddCookie(&http.Cookie{Name: name, Value: "two"})
	w := httptest.NewRecorder()
	if _, _, ok = h.refreshInput(w, r, ""); ok || w.Code != 401 {
		t.Fatal("duplicate cookie accepted")
	}
}

func TestBrowserCookieOriginsAndFamilyIntentAreIsolated(t *testing.T) {
	h := &Handler{AccountOrigin: "https://web.getportico.tv", Origins: map[string]bool{"https://web.getportico.tv": true, "https://one.getportico.tv": true, "https://two.getportico.tv": true}}
	seen := map[string]bool{}
	// Same-site origins (A16) still get their own cookie each.
	for _, origin := range []string{"https://web.getportico.tv", "https://one.getportico.tv", "https://two.getportico.tv"} {
		r := httptest.NewRequest("POST", "/v1/sessions/refresh", nil)
		r.Header.Set("Origin", origin)
		name, secure, ok := h.browserCookiePolicy(r)
		if !ok || !secure || seen[name] {
			t.Fatal("origin cookies share authority")
		}
		seen[name] = true
		if origin != "https://web.getportico.tv" && refreshSameSite(name, secure) != http.SameSiteLaxMode {
			t.Fatal("media cookie cannot be used cross-site")
		}
		if origin == "https://web.getportico.tv" && refreshSameSite(name, secure) != http.SameSiteLaxMode {
			t.Fatal("canonical cookie policy changed")
		}
	}
	w := httptest.NewRecorder()
	if requireBrowserFamily(w, "browser", "") || w.Code != 400 {
		t.Fatal("unbound browser family accepted")
	}
	if !requireBrowserFamily(httptest.NewRecorder(), "native", "") {
		t.Fatal("native credential requires a browser hint")
	}
}
