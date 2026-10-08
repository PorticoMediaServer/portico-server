package httpapi

import (
	"net/http"
	"net/http/httptest"
	"portico.local/hosted/internal/postgres"
	"strings"
	"testing"
)

func TestAppleFormPostHasDedicatedCrossSiteCookie(t *testing.T) {
	binding := strings.Repeat("A", 43)
	w := httptest.NewRecorder()
	setAppleCallbackCookie(w, binding)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal(cookies)
	}
	c := cookies[0]
	if c.Name != appleCallbackCookie || c.Path != "/auth/sso/apple/callback" || c.SameSite != http.SameSiteNoneMode || !c.Secure || !c.HttpOnly || c.MaxAge != 600 {
		t.Fatal(c)
	}
	h := &Handler{}
	r := httptest.NewRequest(http.MethodPost, "/auth/sso/apple/callback", nil)
	r.AddCookie(c)
	if h.oidcBinding(r) != binding {
		t.Fatal("Apple callback binding lost")
	}
	r2 := httptest.NewRequest(http.MethodPost, "/auth/sso/apple/callback", nil)
	r2.AddCookie(&http.Cookie{Name: "__Host-portico_identity", Value: binding})
	if h.oidcBinding(r2) != "" {
		t.Fatal("Lax browser cookie accepted on cross-site callback")
	}
	clear := httptest.NewRecorder()
	setAppleCallbackCookie(clear, "")
	if clear.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("callback cookie not cleared")
	}
}

// A57: every Apple callback answer, including an early rejection, clears the
// callback cookie in the response it sends.
func TestAppleCallbackClearsCookieOnEveryAnswer(t *testing.T) {
	// The early rejection happens before the provider service is consulted.
	h := &Handler{AccountOrigin: "https://web.getportico.tv", OIDC: &postgres.OIDCLogin{}}
	mux := http.NewServeMux()
	h.oidcRoutes(mux)
	r := httptest.NewRequest(http.MethodPost, "/auth/sso/apple/callback", strings.NewReader("state=s&code=c"))
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status %d", w.Code)
	}
	cleared := false
	for _, c := range w.Result().Cookies() {
		cleared = cleared || c.Name == appleCallbackCookie && c.MaxAge == -1
	}
	if !cleared {
		t.Fatal("callback cookie not cleared on the redirect")
	}
}
