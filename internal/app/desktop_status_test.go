package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDesktopRemoteAccessSummary(t *testing.T) {
	tests := []struct {
		name     string
		settings RemoteAccessSettings
		status   string
		label    string
	}{
		{name: "off", settings: RemoteAccessSettings{}, status: "off", label: "Off"},
		{name: "account required", settings: RemoteAccessSettings{Enabled: true}, status: "account_required", label: "Portico Account not connected"},
		{name: "available", settings: RemoteAccessSettings{Enabled: true, ServerID: "server-1", ClaimStatus: "claimed", LastHeartbeatAt: "2026-08-31T12:00:00Z", LastReachabilityResult: "public_reachable"}, status: "available", label: "Direct access available"},
		{name: "hosted unavailable", settings: RemoteAccessSettings{Enabled: true, ServerID: "server-1", ClaimStatus: "claimed", LastHeartbeatError: "network unavailable"}, status: "hosted_unavailable", label: "Hosted Services unavailable"},
		{name: "checking", settings: RemoteAccessSettings{Enabled: true, ServerID: "server-1", ClaimStatus: "claimed", LastReachabilityResult: "public_checking"}, status: "checking", label: "Checking direct access"},
		{name: "unavailable", settings: RemoteAccessSettings{Enabled: true, ServerID: "server-1", ClaimStatus: "claimed", LastReachabilityResult: "public_unreachable"}, status: "unavailable", label: "Direct route unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, label := desktopRemoteAccessSummary(test.settings)
			if status != test.status || label != test.label {
				t.Fatalf("desktop summary = (%q, %q), want (%q, %q)", status, label, test.status, test.label)
			}
		})
	}
}

func TestDesktopStatusRejectsNonLoopbackRequests(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://server.example/desktop/status", nil)
	request.RemoteAddr = "203.0.113.10:42100"
	response := httptest.NewRecorder()

	(&Server{}).handleDesktopStatus(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestDesktopStatusAllowsOnlyGet(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/desktop/status", nil)
	request.RemoteAddr = "127.0.0.1:42100"
	response := httptest.NewRecorder()

	(&Server{}).handleDesktopStatus(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}
