package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIdentityCookieFollowsExplicitDevelopmentPolicy(t *testing.T) {
	h := &Handler{AccountOrigin: "https://web.getportico.tv", Origins: map[string]bool{
		"https://web.getportico.tv": true, "http://127.0.0.1:19412": true, "http://remote.example": true,
	}}
	for _, test := range []struct {
		origin      string
		development bool
		name        string
		secure      bool
	}{
		{"http://127.0.0.1:19412", false, "__Host-portico_identity", true},
		{"http://127.0.0.1:19412", true, "portico_identity_dev", false},
		{"http://remote.example", true, "__Host-portico_identity", true},
		{"https://web.getportico.tv", true, "__Host-portico_identity", true},
	} {
		r := httptest.NewRequest(http.MethodPost, "/v1/registrations", nil)
		r.Header.Set("Origin", test.origin)
		h.DevelopmentCookies = test.development
		w := httptest.NewRecorder()
		h.setIdentityCookie(w, r, strings.Repeat("a", 43))
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != test.name || cookies[0].Secure != test.secure || !cookies[0].HttpOnly || cookies[0].Path != "/" {
			t.Fatalf("identity cookie for %s (development=%v): %#v", test.origin, test.development, cookies)
		}
	}
}

func TestIdentityContinuationReadIsNotMediaOriginAuthority(t *testing.T) {
	const account = "https://web.getportico.tv"
	const media = "https://media.example"
	h := &Handler{AccountOrigin: account, Origins: map[string]bool{account: true, media: true}}
	binding := strings.Repeat("a", 43)
	for _, origin := range []string{"", account, media, "https://untrusted.example"} {
		r := httptest.NewRequest(http.MethodGet, "/v1/identity/current", nil)
		r.Header.Set("Origin", origin)
		r.AddCookie(&http.Cookie{Name: "__Host-portico_identity", Value: binding})
		w := httptest.NewRecorder()
		got, ok := h.continuation(w, r, false)
		allowed := origin == "" || origin == account
		if ok != allowed || (ok && got != binding) || (!ok && w.Code != http.StatusForbidden) {
			t.Fatalf("origin %q: binding=%q allowed=%v status=%d", origin, got, ok, w.Code)
		}
	}
	// Native continuation is private authority, never an override of a browser cookie.
	r := httptest.NewRequest(http.MethodPost, "/v1/identity/google/start", nil)
	r.Header.Set("X-Portico-Continuation", binding)
	if got, ok := h.continuation(httptest.NewRecorder(), r, false); !ok || got != binding {
		t.Fatal("native continuation denied")
	}
	r.Header.Set("Origin", account)
	if _, ok := h.continuation(httptest.NewRecorder(), r, false); ok {
		t.Fatal("browser supplied native continuation")
	}
	r.Header.Del("Origin")
	r.AddCookie(&http.Cookie{Name: "__Host-portico_identity", Value: binding})
	if _, ok := h.continuation(httptest.NewRecorder(), r, false); ok {
		t.Fatal("mixed continuation authorities accepted")
	}
}
