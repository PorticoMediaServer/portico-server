package httpapi

import (
	"net/http/httptest"
	"testing"
)

// A16: browser sessions only for origins on Hosted's site; clearing the
// refresh cookie clears its CSRF cookie too.
func TestBrowserModeOnlyForSameSiteOrigins(t *testing.T) {
	h := &Handler{AccountOrigin: "https://web.getportico.tv", Origins: map[string]bool{
		"https://web.getportico.tv":              true,
		"https://demo.getportico.tv":             true,
		"https://c-abc.ns1.direct.getportico.tv": true,
		"https://elsewhere.example":              true,
	}}
	for origin, want := range map[string]bool{
		"https://web.getportico.tv":              true,
		"https://demo.getportico.tv":             true,
		"https://c-abc.ns1.direct.getportico.tv": false,
		"https://elsewhere.example":              false,
		"https://unlisted.getportico.tv":         false,
	} {
		r := httptest.NewRequest("POST", "https://web.getportico.tv/v1/sessions", nil)
		r.Header.Set("Origin", origin)
		if _, _, ok := h.browserCookiePolicy(r); ok != want {
			t.Errorf("%s: browser mode %v, want %v", origin, ok, want)
		}
	}
	r := httptest.NewRequest("DELETE", "https://web.getportico.tv/v1/sessions/current", nil)
	r.Header.Set("Origin", "https://web.getportico.tv")
	w := httptest.NewRecorder()
	h.clearRefreshCookie(w, r)
	cleared := map[string]bool{}
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 {
			cleared[c.Name] = true
		}
	}
	if !cleared["__Host-portico_refresh"] || !cleared["__Host-portico_refresh_csrf"] {
		t.Fatalf("cleared %v", cleared)
	}
}
