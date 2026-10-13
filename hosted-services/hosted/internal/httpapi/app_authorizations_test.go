package httpapi

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAppAuthorizationQueryStrict(t *testing.T) {
	b64 := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	// The challenge is the size a local server issues (24 bytes).
	q := url.Values{"serverId": {"srv_" + b64}, "challenge": {base64.RawURLEncoding.EncodeToString(make([]byte, 24))}, "redirectUri": {"https://client.example/finish"}, "state": {strings.Repeat("s", 32)}, "appName": {"Unverified App"}, "codeChallenge": {b64}}
	r := httptest.NewRequest("GET", "/auth/apps/authorize?"+q.Encode(), nil)
	if _, e := appAuthorizationQuery(r); e != nil || !appAuthorizationQueryAllowed(r) {
		t.Fatal(e)
	}
	for _, extra := range []string{"&state=other", "&unknown=field", "&codeChallengeMethod=plain", "&bad=%zz"} {
		r = httptest.NewRequest("GET", "/auth/apps/authorize?"+q.Encode()+extra, nil)
		if appAuthorizationQueryAllowed(r) {
			t.Fatal(extra)
		}
	}
}
func TestAppAuthorizationCookieCanonicalOriginAndCSRF(t *testing.T) {
	h := &Handler{AccountOrigin: "https://web.getportico.tv", Origins: map[string]bool{"https://web.getportico.tv": true, "https://media.getportico.tv": true}}
	for _, tt := range []struct {
		origin, csrf, bearer string
		allow                bool
	}{{"https://web.getportico.tv", strings.Repeat("c", 43), "", true}, {"https://media.getportico.tv", strings.Repeat("c", 43), "", false}, {"", strings.Repeat("c", 43), "", false}, {"https://web.getportico.tv", "wrong", "", false}, {"https://web.getportico.tv", strings.Repeat("c", 43), "access", false}} {
		r := httptest.NewRequest("POST", "/v1/app-authorizations", nil)
		r.Header.Set("Origin", tt.origin)
		r.Header.Set("X-CSRF-Token", tt.csrf)
		if tt.bearer != "" {
			r.Header.Set("Authorization", "Bearer "+tt.bearer)
		}
		r.AddCookie(&http.Cookie{Name: "__Host-portico_refresh", Value: "hrt_" + strings.Repeat("a", 43)})
		r.AddCookie(&http.Cookie{Name: "__Host-portico_refresh_csrf", Value: strings.Repeat("c", 43)})
		_, ok := h.appAuthorizationCookie(httptest.NewRecorder(), r)
		if ok != tt.allow {
			t.Fatal(tt, ok)
		}
	}
}
func TestAppExchangeCORSIsCredentiallessAndIsolated(t *testing.T) {
	h := &Handler{Origins: map[string]bool{}}
	handler := h.Routes()
	r := httptest.NewRequest("OPTIONS", "/v1/app-authorizations/token", nil)
	r.Header.Set("Origin", "https://outside.example")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal(w.Code, w.Header())
	}
	r = httptest.NewRequest("OPTIONS", "/v1/me", nil)
	r.Header.Set("Origin", "https://outside.example")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("outside origin crossed account boundary", w.Code)
	}
}
func TestCredentialAdmissionSharesRoutesBeforeDatabase(t *testing.T) {
	h := &Handler{}
	r := httptest.NewRequest("POST", "/v1/servers/arbitrary/heartbeat", nil)
	r.RemoteAddr = "8.8.8.8:4321"
	for range 240 {
		if !h.allowCredentialLookup(httptest.NewRecorder(), r, "server") {
			t.Fatal("early")
		}
	}
	r.URL.Path = "/v1/servers/another/heartbeat"
	w := httptest.NewRecorder()
	if h.allowCredentialLookup(w, r, "server") || w.Code != 429 || w.Header().Get("Retry-After") != "60" {
		t.Fatal(w.Code, w.Header())
	}
}

func TestAppAuthorizationPageIsEscapedAndUsesExternalAssets(t *testing.T) {
	h := &Handler{AppAuthorizations: &AppAuthorizations{}, Origins: map[string]bool{}}
	b64 := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	q := url.Values{"serverId": {"srv_" + b64}, "challenge": {b64}, "redirectUri": {"https://client.example/finish"}, "state": {strings.Repeat("s", 32)}, "appName": {"</p><script>alert(1)</script>"}, "codeChallenge": {b64}}
	r := httptest.NewRequest("GET", "/auth/apps/authorize?"+q.Encode(), nil)
	w := httptest.NewRecorder()
	handler := h.Routes()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/html") || w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Header())
	}
	body := w.Body.String()
	if strings.Contains(body, "<script>alert") || strings.Contains(body, "unsafe-inline") || !strings.Contains(body, `src="/auth/apps/authorize.js"`) || !strings.Contains(body, `id="strings" hidden`) || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("page escaping/CSP", body)
	}
	for _, path := range []string{"/auth/apps/authorize.js", "/auth/apps/authorize.css"} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatal(path, w.Code)
		}
	}
}

func TestAppAuthorizationAdmissionCannotResetWithRequestID(t *testing.T) {
	h := &Handler{}
	r := httptest.NewRequest("POST", "/v1/app-authorizations/app_first/decision", nil)
	for range 60 {
		if !h.appAuthorizationRate(httptest.NewRecorder(), r) {
			t.Fatal("early refusal")
		}
	}
	r.URL.Path = "/v1/app-authorizations/app_next/decision"
	w := httptest.NewRecorder()
	if h.appAuthorizationRate(w, r) || w.Code != 429 || w.Header().Get("Retry-After") != "60" {
		t.Fatal("request ID reset rate", w.Code, w.Header())
	}
}
