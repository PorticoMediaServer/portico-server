package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestViewerPreferenceScopeErrorsAreNormalized(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	const token = "viewer-preference-error-token"
	addViewerPreferenceSession(t, server, account.ID, account.ID, token, "browser-installation-0001", "portico-web", "web")

	invalid := httptest.NewRecorder()
	server.handleViewerPreferences(invalid, httptest.NewRequest(http.MethodGet, "/api/preferences?deviceClass=console&installationId=browser-0001", nil), user)
	if invalid.Code != http.StatusBadRequest || strings.Contains(invalid.Body.String(), "deviceClass must") {
		t.Fatalf("invalid scope was not normalized: status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	if err := server.db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	failed := httptest.NewRecorder()
	server.handleViewerPreferences(failed, authorizedPreferenceRequest(http.MethodGet, "/api/preferences?deviceClass=web&installationId=browser-installation-0001", token, nil), user)
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("storage failure status=%d body=%s", failed.Code, failed.Body.String())
	}
	var problem struct {
		Code      string `json:"code"`
		Detail    string `json:"detail"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(failed.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != "preferences_failed" || problem.RequestID == "" || strings.Contains(strings.ToLower(problem.Detail), "database") {
		t.Fatalf("storage detail leaked: %#v", problem)
	}
}
