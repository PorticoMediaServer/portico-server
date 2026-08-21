package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func addViewerPreferenceSession(t *testing.T, server *Server, accountID, profileID, token, installationID, app, platform string) {
	t.Helper()
	now := time.Now().UTC()
	deviceID := "dev_pref_" + strings.ReplaceAll(profileID, "-", "_")
	if _, err := server.db.Exec(`
		INSERT INTO devices (id, user_id, installation_id, name, app, platform, user_agent, trusted, created_at, last_seen_at)
		VALUES (?, ?, ?, 'Preference test', ?, ?, '', 1, ?, ?)`,
		deviceID, accountID, installationID, app, platform, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert preference device: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"sess_pref_"+strings.ReplaceAll(profileID, "-", "_"), accountID, profileID, deviceID, hashToken(token),
		now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert preference session: %v", err)
	}
}

func authorizedPreferenceRequest(method, target, token string, body []byte) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	if len(body) != 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func TestViewerPreferenceHTTPScopesAreBoundToActiveAppSession(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	const token = "viewer-preference-session-token"
	const installationID = "web-installation-0001"
	addViewerPreferenceSession(t, server, account.ID, account.ID, token, installationID, "portico-web", "web")

	allowed := httptest.NewRecorder()
	server.handleViewerPreferences(allowed, authorizedPreferenceRequest(http.MethodGet,
		"/api/preferences?deviceClass=web&installationId="+installationID, token, nil), user)
	if allowed.Code != http.StatusOK {
		t.Fatalf("active installation status=%d body=%s", allowed.Code, allowed.Body.String())
	}

	for name, target := range map[string]string{
		"sibling installation":   "/api/preferences?deviceClass=web&installationId=web-installation-0002",
		"different device class": "/api/preferences?deviceClass=mobile&installationId=" + installationID,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.handleViewerPreferences(response, authorizedPreferenceRequest(http.MethodGet, target, token, nil), user)
			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "interactive_session_required") {
				t.Fatalf("unbound scope status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}

	siblingWrite := httptest.NewRecorder()
	server.handleViewerPreferenceDocument(siblingWrite, authorizedPreferenceRequest(http.MethodPatch,
		"/api/preferences/account-server-installation?deviceClass=web&installationId=web-installation-0002",
		token, []byte(`{"version":"v1","expectedRevision":0,"changes":{"rememberAccount":false}}`)), user)
	if siblingWrite.Code != http.StatusForbidden || !strings.Contains(siblingWrite.Body.String(), "interactive_session_required") {
		t.Fatalf("sibling installation write status=%d body=%s", siblingWrite.Code, siblingWrite.Body.String())
	}

	nonInteractive := httptest.NewRecorder()
	server.handleViewerPreferences(nonInteractive, httptest.NewRequest(http.MethodGet,
		"/api/preferences?deviceClass=web&installationId="+installationID, nil), user)
	if nonInteractive.Code != http.StatusForbidden || !strings.Contains(nonInteractive.Body.String(), "interactive_session_required") {
		t.Fatalf("non-interactive preference status=%d body=%s", nonInteractive.Code, nonInteractive.Body.String())
	}

	apiKeyUser := user
	apiKeyUser.AuthProvider = "api_key"
	apiKeyUser.APIKeyID = "key_preferences"
	apiKey := httptest.NewRecorder()
	server.handleViewerPreferences(apiKey, authorizedPreferenceRequest(http.MethodGet,
		"/api/preferences?deviceClass=web&installationId="+installationID, token, nil), apiKeyUser)
	if apiKey.Code != http.StatusForbidden || !strings.Contains(apiKey.Body.String(), "interactive_session_required") {
		t.Fatalf("API-key preference status=%d body=%s", apiKey.Code, apiKey.Body.String())
	}
	var siblingDocuments int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM viewer_preference_documents WHERE installation_id = 'web-installation-0002'`).Scan(&siblingDocuments); err != nil || siblingDocuments != 0 {
		t.Fatalf("sibling installation documents=%d err=%v", siblingDocuments, err)
	}
}

func TestLastProfileIsRecordedOnlyByAuthoritativeProfileActivation(t *testing.T) {
	server := newScannerTestServer(t)
	account, child := createProfileProtocolAccount(t, server)
	primary := explicitPrimaryUser(account)
	const primaryToken = "viewer-preference-primary-token"
	const installationID = "web-activation-0001"
	addViewerPreferenceSession(t, server, account.ID, account.ID, primaryToken, installationID, "portico-web", "web")

	initialResponse := httptest.NewRecorder()
	server.handleViewerPreferences(initialResponse, authorizedPreferenceRequest(http.MethodGet,
		"/api/preferences?deviceClass=web&installationId="+installationID, primaryToken, nil), primary)
	if initialResponse.Code != http.StatusOK {
		t.Fatalf("initial preference status=%d body=%s", initialResponse.Code, initialResponse.Body.String())
	}
	var initial viewerPreferenceBundle
	if err := json.Unmarshal(initialResponse.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}

	generic := httptest.NewRecorder()
	server.handleViewerPreferenceDocument(generic, authorizedPreferenceRequest(http.MethodPatch,
		"/api/preferences/account-server-installation?deviceClass=web&installationId="+installationID,
		primaryToken, []byte(`{"version":"v1","expectedRevision":0,"changes":{"lastProfileId":"`+child.ID+`"}}`)), primary)
	if generic.Code != http.StatusBadRequest || !strings.Contains(generic.Body.String(), "invalid_preferences") {
		t.Fatalf("generic last-profile patch status=%d body=%s", generic.Code, generic.Body.String())
	}

	if _, err := server.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`DELETE FROM devices WHERE user_id = ?`, account.ID); err != nil {
		t.Fatal(err)
	}
	childUser := primary
	childUser.ProfileID = child.ID
	childUser.ProfileIsPrimary = false
	childUser.DisplayName = child.DisplayName
	const childToken = "viewer-preference-child-token"
	addViewerPreferenceSession(t, server, account.ID, child.ID, childToken, installationID, "portico-web", "web")

	activation := httptest.NewRecorder()
	server.handleViewerProfileActivation(activation, authorizedPreferenceRequest(http.MethodPost,
		"/api/preferences/profile-activation", childToken,
		[]byte(`{"version":"v1","expectedRevision":`+jsonInt(initial.AccountServerInstallation.Revision)+`}`)), childUser)
	if activation.Code != http.StatusOK {
		t.Fatalf("profile activation status=%d body=%s", activation.Code, activation.Body.String())
	}
	var updated preferenceDocument
	if err := json.Unmarshal(activation.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	var values accountInstallationPreferenceValues
	if err := json.Unmarshal(updated.Values, &values); err != nil {
		t.Fatal(err)
	}
	if values.LastProfileID != child.ID || updated.Revision != initial.AccountServerInstallation.Revision+1 {
		t.Fatalf("activation document=%#v values=%#v", updated, values)
	}
}

func jsonInt(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
