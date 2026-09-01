package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEarlyStartupHandlerKeepsPublicReadinessMinimal(t *testing.T) {
	handler := newEarlyStartupHandler(time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC), "http://127.0.0.1:9090")
	handler.completePhase("http_ready", "HTTP listener ready", nil)
	handler.startPhase("database_opening", "Open database")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/readiness", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("startup status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode startup response: %v", err)
	}
	if body["status"] != "starting" || body["ready"] != false {
		t.Fatalf("unexpected startup response: %#v", body)
	}
	if _, exists := body["phases"]; exists || body["httpAddr"] != nil {
		t.Fatalf("public readiness leaked startup diagnostics: %#v", body)
	}
}

func TestEarlyStartupHandlerRejectsApplicationRoutesWithRetryAfter(t *testing.T) {
	handler := newEarlyStartupHandler(time.Now(), "http://127.0.0.1:9090")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/home", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("application route status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if rec.Header().Get("Retry-After") != "2" {
		t.Fatalf("retry-after = %q, want 2", rec.Header().Get("Retry-After"))
	}
}

func TestRestoreHostHealthTimeoutAllowsFullIntegrityCheckOnSlowHosts(t *testing.T) {
	if got := restoreHostHealthTimeout(); got < 5*time.Minute {
		t.Fatalf("restore host health timeout = %s, want at least 5m", got)
	}
}

func TestSwitchableHandlerSwapsToApplicationHandler(t *testing.T) {
	switcher := newSwitchableHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	switcher.Set(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/home", nil)
	switcher.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("swapped handler status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestProtectedBuildIdentityFailsClosed(t *testing.T) {
	previous := []string{version, buildNumber, channel, commit, builtAt, releaseSafetyClass}
	t.Cleanup(func() {
		version, buildNumber, channel, commit, builtAt, releaseSafetyClass = previous[0], previous[1], previous[2], previous[3], previous[4], previous[5]
	})
	releaseSafetyClass = "protected"
	version, buildNumber, channel, commit, builtAt = "dev", "0", "development", "unknown", "unknown"
	if err := validateBuildIdentity(); err == nil {
		t.Fatal("unstamped protected build was accepted")
	}
	version, buildNumber, channel, commit, builtAt = "1.0.0", "42", "production", "0123456789abcdef0123456789abcdef01234567", "2026-08-17T00:00:00Z"
	if err := validateBuildIdentity(); err != nil {
		t.Fatalf("stamped protected build rejected: %v", err)
	}
}
