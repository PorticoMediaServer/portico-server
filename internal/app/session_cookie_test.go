package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func requestTransportContextForCookieTest(t *testing.T, server *Server, rawURL string) context.Context {
	t.Helper()
	var captured context.Context
	handler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		captured = request.Context()
	})
	server.requestTransport(handler).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, rawURL, nil))
	if captured == nil {
		t.Fatal("request transport did not capture a context")
	}
	return captured
}

func TestPrimarySessionCookieSeparatesLANAndRemoteWithoutLegacyFallback(t *testing.T) {
	server := &Server{
		sessionCookieNamesCache: []string{"portico_session_test"},
		sessionCookieCacheUntil: time.Now().Add(time.Hour),
	}
	lanContext := requestTransportContextForCookieTest(t, server, "http://192.168.1.20/api/auth/login")
	remoteContext := requestTransportContextForCookieTest(t, server, "https://remote.example.test/api/auth/login")

	lanNames := server.sessionCookieNamesContext(lanContext)
	remoteNames := server.sessionCookieNamesContext(remoteContext)
	if len(lanNames) != 1 || len(remoteNames) != 1 {
		t.Fatalf("expected one transport-scoped cookie name, got LAN=%v remote=%v", lanNames, remoteNames)
	}
	if lanNames[0] != "portico_session_test_lan" || remoteNames[0] != "portico_session_test_remote" {
		t.Fatalf("unexpected transport-scoped cookie names: LAN=%q remote=%q", lanNames[0], remoteNames[0])
	}
	if lanNames[0] == remoteNames[0] || strings.Contains(lanNames[0], "_remote") || strings.Contains(remoteNames[0], "_lan") {
		t.Fatalf("LAN and remote cookie namespaces overlap: LAN=%q remote=%q", lanNames[0], remoteNames[0])
	}

	lanRequest := httptest.NewRequest(http.MethodGet, "http://192.168.1.20/api/auth/me", nil).WithContext(lanContext)
	lanRequest.AddCookie(&http.Cookie{Name: remoteNames[0], Value: "remote-token"})
	lanRequest.AddCookie(&http.Cookie{Name: lanNames[0], Value: "lan-token"})
	if cookies := server.requestSessionCookies(lanRequest); len(cookies) != 1 || cookies[0].Name != lanNames[0] {
		t.Fatalf("LAN request accepted the wrong cookie namespace: %#v", cookies)
	}

	remoteRequest := httptest.NewRequest(http.MethodGet, "https://remote.example.test/api/auth/me", nil).WithContext(remoteContext)
	remoteRequest.AddCookie(&http.Cookie{Name: lanNames[0], Value: "lan-token"})
	remoteRequest.AddCookie(&http.Cookie{Name: remoteNames[0], Value: "remote-token"})
	if cookies := server.requestSessionCookies(remoteRequest); len(cookies) != 1 || cookies[0].Name != remoteNames[0] {
		t.Fatalf("remote request accepted the wrong cookie namespace: %#v", cookies)
	}

	for _, legacyName := range []string{sessionCookieName, "portico_session_test"} {
		legacyRequest := httptest.NewRequest(http.MethodGet, "http://192.168.1.20/api/auth/me", nil).WithContext(lanContext)
		legacyRequest.AddCookie(&http.Cookie{Name: legacyName, Value: "legacy-token"})
		if cookies := server.requestSessionCookies(legacyRequest); len(cookies) != 0 {
			t.Fatalf("legacy cookie %q was accepted as a fallback: %#v", legacyName, cookies)
		}
		if mode := server.requestAuthMode(legacyRequest); mode != "anonymous" {
			t.Fatalf("legacy cookie %q changed auth mode to %q", legacyName, mode)
		}
		if hashes := server.currentSessionTokenHashes(legacyRequest); len(hashes) != 0 {
			t.Fatalf("legacy cookie %q influenced current-session bookkeeping: %#v", legacyName, hashes)
		}
	}

	lanRecorder := httptest.NewRecorder()
	server.setSessionCookie(lanContext, lanRecorder, lanNames[0], "lan-token", time.Now().Add(time.Minute))
	lanCookie := lanRecorder.Result().Cookies()[0]
	if lanCookie.Name != lanNames[0] || lanCookie.Secure {
		t.Fatalf("LAN cookie topology is not non-Secure and isolated: %#v", lanCookie)
	}

	remoteRecorder := httptest.NewRecorder()
	server.setSessionCookie(remoteContext, remoteRecorder, remoteNames[0], "remote-token", time.Now().Add(time.Hour))
	remoteCookie := remoteRecorder.Result().Cookies()[0]
	if remoteCookie.Name != remoteNames[0] || !remoteCookie.Secure {
		t.Fatalf("remote cookie topology is not Secure and isolated: %#v", remoteCookie)
	}
}

func TestPrimarySessionCookieUsesTheSameRollingLifetimeAcrossRoutes(t *testing.T) {
	server := &Server{}
	lanContext := requestTransportContextForCookieTest(t, server, "http://192.168.1.20/api/auth/login")
	remoteContext := requestTransportContextForCookieTest(t, server, "https://remote.example.test/api/auth/login")

	if got := server.browserSessionTTLContext(lanContext); got != browserSessionTTL {
		t.Fatalf("LAN browser session TTL = %s, want %s", got, browserSessionTTL)
	}
	if !server.browserSessionMayRoll(lanContext) {
		t.Fatal("LAN browser session must retain rolling behavior")
	}
	if got := server.browserSessionTTLContext(remoteContext); got != browserSessionTTL {
		t.Fatalf("remote browser session TTL = %s, want %s", got, browserSessionTTL)
	}
	if !server.browserSessionMayRoll(remoteContext) {
		t.Fatal("remote browser session must retain rolling behavior")
	}
}
