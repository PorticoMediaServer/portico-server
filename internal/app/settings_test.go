package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func patchSettingsGroups(t *testing.T, client *http.Client, serverURL string, groups map[string]any, out any) (int, string) {
	t.Helper()
	var current SettingsDocument
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/settings", nil, &current)
	if status != http.StatusOK {
		return status, body
	}
	var updated SettingsDocument
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/settings", map[string]any{
		"expectedRevision": current.Revision,
		"idempotencyKey":   randomID("settings-test"),
		"groups":           groups,
	}, &updated)
	if status == http.StatusOK && out != nil {
		switch target := out.(type) {
		case *map[string]any:
			*target = updated.Groups
		case *SettingsDocument:
			*target = updated
		}
	}
	return status, body
}

func TestWritableSettingGroups(t *testing.T) {
	writable := []string{"server", "devices", "dlna", "dvr", "library", "languages", "metadataAgents", "network", "notifications", "viewerFeedback", "optimizedVersions", "retention", "scheduledTasks", "transcoder", "troubleshooting"}
	for _, group := range writable {
		if !isWritableSettingGroup(group) {
			t.Fatalf("expected %s to be writable", group)
		}
	}

	readOnly := []string{
		"branding",
		"extras",
		"remoteAccess",
		"updates",
	}
	for _, group := range readOnly {
		if isWritableSettingGroup(group) {
			t.Fatalf("expected %s to stay read-only until its backend exists", group)
		}
	}
}

func TestCanonicalSettingRegistryCoversEveryExposedField(t *testing.T) {
	for group, schema := range writableSettingSchemas {
		definition, ok := canonicalSettingRegistry[group]
		if !ok || !definition.Revisioned || definition.Scope != "server" || definition.RuntimeConsumer == "" {
			t.Fatalf("setting group %s has incomplete canonical registry metadata: %#v", group, definition)
		}
		for field := range schema {
			if _, ok := definition.Defaults[field]; !ok {
				t.Fatalf("setting %s.%s has no canonical default", group, field)
			}
			metadata, ok := definition.Fields[field]
			if !ok || metadata.Type == "" || metadata.Validation == "" || metadata.EffectiveValue == "" ||
				metadata.RuntimeConsumer == "" || metadata.OperationalStatus == "" || metadata.OutcomeTest == "" ||
				metadata.SaveMode == "" || metadata.ApplicationMode == "" || metadata.Permission != "manageServer" ||
				metadata.Capability == "" || metadata.AuditClass == "" || metadata.RetentionClass == "" || metadata.Revision < 1 || metadata.Secret {
				t.Fatalf("setting %s.%s has incomplete field registry metadata: %#v", group, field, metadata)
			}
		}
		for field := range definition.Defaults {
			if _, ok := schema[field]; !ok {
				t.Fatalf("canonical default %s.%s is not exposed by the writable schema", group, field)
			}
		}
	}
	for _, field := range []string{"tmdbReadAccessToken", "tmdbAPIKey", "tvdbAPIKey"} {
		metadata := canonicalSettingRegistry["metadataAgents"].Fields[field]
		if !metadata.Secret || metadata.Validation == "" || metadata.EffectiveValue == "" || metadata.OperationalStatus != "supported" {
			t.Fatalf("secret setting metadataAgents.%s has incomplete field registry metadata: %#v", field, metadata)
		}
	}
}

func TestMalformedAndInternalSettingsDoNotPoisonPublicRevision(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	_, originalRevision, _, err := server.loadSettingsSnapshotContext(t.Context())
	if err != nil {
		t.Fatalf("load original settings revision: %v", err)
	}
	if _, err := db.Exec(`UPDATE settings SET value_json = '{broken', updated_at = '2099-01-01T00:00:00Z' WHERE key = 'dlna'`); err != nil {
		t.Fatalf("corrupt isolated setting fixture: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('internal-revision-probe', '{"value":2}', '2099-01-02T00:00:00Z')`); err != nil {
		t.Fatalf("write internal setting fixture: %v", err)
	}
	_, nextRevision, _, err := server.loadSettingsSnapshotContext(t.Context())
	if err != nil {
		t.Fatalf("malformed group poisoned settings snapshot: %v", err)
	}
	if nextRevision == originalRevision {
		t.Fatal("removing a malformed public group from revision authority did not change the public revision")
	}
	if _, err := db.Exec(`UPDATE settings SET value_json = '{"value":3}', updated_at = '2099-01-03T00:00:00Z' WHERE key = 'internal-revision-probe'`); err != nil {
		t.Fatalf("update internal setting fixture: %v", err)
	}
	_, afterInternalRevision, _, err := server.loadSettingsSnapshotContext(t.Context())
	if err != nil || afterInternalRevision != nextRevision {
		t.Fatalf("internal setting changed public revision: before=%q after=%q err=%v", nextRevision, afterInternalRevision, err)
	}
	var document SettingsDocument
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/settings", nil, &document)
	if status != http.StatusOK {
		t.Fatalf("settings GET with one malformed group status=%d body=%s", status, body)
	}
	dlna, _ := document.Groups["dlna"].(map[string]any)
	if dlna["enabled"] != false || dlna["reportTimeline"] != true {
		t.Fatalf("malformed DLNA group did not receive canonical defaults: %#v", dlna)
	}
	var quarantined, activeValid int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings_quarantine WHERE key = 'dlna' AND value_json = '{broken'`).Scan(&quarantined); err != nil || quarantined != 1 {
		t.Fatalf("malformed DLNA quarantine count=%d err=%v", quarantined, err)
	}
	if err := db.QueryRow(`SELECT json_valid(value_json) FROM settings WHERE key = 'dlna'`).Scan(&activeValid); err != nil || activeValid != 1 {
		t.Fatalf("repaired DLNA active row valid=%d err=%v", activeValid, err)
	}
}

func TestSemanticallyInvalidSettingGroupPreservesValidFieldsAndQuarantinesRawDocument(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	raw := `{"enabled":true,"friendlyName":"Living Room","reportTimeline":"sometimes","removedLegacyField":42}`
	if _, err := db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'dlna'`, raw); err != nil {
		t.Fatalf("seed invalid DLNA settings: %v", err)
	}
	settings, _, _, err := server.loadSettingsSnapshotContext(t.Context())
	if err != nil {
		t.Fatalf("repair invalid DLNA settings: %v", err)
	}
	dlna, _ := settings["dlna"].(map[string]any)
	if dlna["enabled"] != true || dlna["friendlyName"] != "Living Room" || dlna["reportTimeline"] != true {
		t.Fatalf("semantic repair did not preserve valid fields and restore defaults: %#v", dlna)
	}
	if _, exists := dlna["removedLegacyField"]; exists {
		t.Fatalf("unsupported legacy field survived repair: %#v", dlna)
	}
	var quarantined int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings_quarantine WHERE key = 'dlna' AND value_json = ?`, raw).Scan(&quarantined); err != nil || quarantined != 1 {
		t.Fatalf("semantic quarantine count=%d err=%v", quarantined, err)
	}
}

func TestExtrasSettingsStayProductOwnedAndHiddenFromSettings(t *testing.T) {
	server := newScannerTestServer(t)
	if isWritableSettingGroup("extras") {
		t.Fatal("extras must not be a public writable Settings group")
	}
	if _, err := normalizeWritableSettingGroup("extras", json.RawMessage(`{"cinemaTrailers":1}`)); err == nil {
		t.Fatal("extras settings unexpectedly accepted through the public Settings schema")
	}

	client := server.clientSettings(map[string]any{
		"extras": map[string]any{
			"cinemaTrailers":       1,
			"includeBehindScenes":  true,
			"includeDeletedScenes": true,
			"includeFeaturettes":   true,
		},
		"server": map[string]any{"friendlyName": "Portico"},
	})
	if _, ok := client["extras"]; ok {
		t.Fatalf("extras leaked to client settings: %#v", client["extras"])
	}
	if _, ok := client["server"]; !ok {
		t.Fatalf("server settings missing from client settings: %#v", client)
	}

	summary := server.settingsSummary(map[string]any{
		"extras": map[string]any{"cinemaTrailers": 1},
	})
	for _, group := range summary.Groups {
		if group.ID == "extras" {
			t.Fatalf("extras leaked to settings summary: %#v", group)
		}
	}
}

func TestNetworkSettingsParsesTrustedRangesAndAccessURLs(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'network'`, `{"secureConnections":"required","lanNetworks":"192.168.1.0/24,not-a-cidr\n10.0.0.0/8","customAccessUrls":"https://media.example.test/,ftp://bad.example,http://localhost:32400"}`); err != nil {
		t.Fatalf("save network settings: %v", err)
	}
	settings := server.networkSettings()
	if settings.SecureConnections != "required" {
		t.Fatalf("secure policy = %s", settings.SecureConnections)
	}
	if len(settings.LANNetworks) != 2 || settings.LANNetworks[0] != "192.168.1.0/24" || settings.LANNetworks[1] != "10.0.0.0/8" {
		t.Fatalf("lan networks = %#v", settings.LANNetworks)
	}
	if len(settings.CustomAccessURLs) != 2 || settings.CustomAccessURLs[0] != "https://media.example.test" {
		t.Fatalf("custom urls = %#v", settings.CustomAccessURLs)
	}
}

func TestServerIdentitySettingsStoreOperatorNoteAndResetIdentity(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var savedSettings map[string]any
	status, body := patchSettingsGroups(t, client, serverURL, map[string]any{
		"server": map[string]any{
			"friendlyName": "Studio Portico",
			"operatorNote": "Primary rack",
		},
	}, &savedSettings)
	if status != http.StatusOK {
		t.Fatalf("server settings status = %d, body: %s", status, body)
	}
	serverGroup, _ := savedSettings["server"].(map[string]any)
	if serverGroup["operatorNote"] != "Primary rack" {
		t.Fatalf("operator note was not returned in server settings: %#v", serverGroup)
	}

	var before SystemIdentityResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/system/identity", nil, &before)
	if status != http.StatusOK {
		t.Fatalf("identity status = %d, body: %s", status, body)
	}
	upsertJSONSetting(t, db, remoteAccessSettingsKey, map[string]any{
		"enabled":                 true,
		"claimStatus":             "claimed",
		"serverId":                "srv_claimed",
		"assignedHostname":        "claimed.direct.getportico.tv",
		"certificateStatus":       "valid",
		"preferredRemoteAuthMode": "local",
	})
	if err := server.saveSecretSetting(remoteAccessCredentialKey, "server-credential"); err != nil {
		t.Fatalf("save remote access credential: %v", err)
	}
	loginUser(t, client, serverURL)

	var after SystemIdentityResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/system/identity/reset", nil, &after)
	if status != http.StatusOK {
		t.Fatalf("identity reset status = %d, body: %s", status, body)
	}
	if after.ServerID == "" || after.ServerID == before.ServerID {
		t.Fatalf("server id was not reset: before=%q after=%q", before.ServerID, after.ServerID)
	}
	if after.PublicKeyFingerprint == "" || after.PublicKeyFingerprint == before.PublicKeyFingerprint {
		t.Fatalf("public key fingerprint was not reset: before=%q after=%q", before.PublicKeyFingerprint, after.PublicKeyFingerprint)
	}
	remoteSettings, err := server.remoteAccessSettings()
	if err != nil {
		t.Fatalf("remote access settings: %v", err)
	}
	if remoteSettings.Enabled || remoteSettings.ServerID != "" || remoteSettings.ClaimStatus != "not_claimed" || remoteSettings.AssignedHostname != "" || remoteSettings.CertificateStatus != "not_requested" {
		t.Fatalf("remote access state was not reset: %#v", remoteSettings)
	}
	if credential := server.secretSetting(remoteAccessCredentialKey); credential != "" {
		t.Fatalf("remote access credential was not cleared: %q", credential)
	}
}

func TestDLNASettingsUseFixedDiscoveryCompatibilityPolicy(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'dlna'`, `{"enabled":true,"friendlyName":"Portico DLNA"}`); err != nil {
		t.Fatalf("save dlna settings: %v", err)
	}
	cfg, err := server.dlnaConfig()
	if err != nil {
		t.Fatalf("dlna config: %v", err)
	}
	if !cfg.Enabled || cfg.FriendlyName != "Portico DLNA" {
		t.Fatalf("unexpected dlna identity: %#v", cfg)
	}
	if got := int(cfg.DiscoveryInterval.Seconds()); got != dlnaDefaultDiscoveryEvery {
		t.Fatalf("discovery seconds = %d", got)
	}
	if cfg.LeaseSeconds != dlnaDefaultLeaseSeconds {
		t.Fatalf("lease seconds = %d", cfg.LeaseSeconds)
	}
	if cfg.ProtocolInfo != dlnaDefaultProtocolInfo {
		t.Fatalf("protocol info = %q", cfg.ProtocolInfo)
	}
}

func TestDLNASettingsRejectRemovedCompatibilityFields(t *testing.T) {
	for _, raw := range []string{
		`{"clientDiscoverySec":60}`,
		`{"announcementLeaseSec":1800}`,
		`{"protocolInfo":"http-get:*:video/mp4:*"}`,
	} {
		if _, err := normalizeWritableSettingGroup("dlna", json.RawMessage(raw)); err == nil {
			t.Fatalf("expected %s to be rejected", raw)
		}
	}
}

func TestClientSettingsOmitUnsupportedDLNAFields(t *testing.T) {
	server := newScannerTestServer(t)
	client := server.clientSettings(map[string]any{
		"dlna": map[string]any{
			"enabled":              true,
			"friendlyName":         "Portico DLNA",
			"clientDiscoverySec":   60,
			"announcementLeaseSec": 1800,
			"protocolInfo":         "http-get:*:video/mp4:*",
		},
	})
	group, _ := client["dlna"].(map[string]any)
	if group == nil {
		t.Fatal("dlna settings missing")
	}
	if group["enabled"] != true || group["friendlyName"] != "Portico DLNA" {
		t.Fatalf("supported dlna settings missing: %#v", group)
	}
	for _, field := range []string{"clientDiscoverySec", "announcementLeaseSec", "protocolInfo"} {
		if _, ok := group[field]; ok {
			t.Fatalf("unsupported dlna field %s leaked to client settings: %#v", field, group)
		}
	}
}

func TestUpdatesSettingsAreEntirelyUnavailable(t *testing.T) {
	for _, raw := range []string{
		`{"checkAutomatically":true}`,
		`{"includePrereleases":true}`,
		`{"maintenanceWindowUTC":"overnight"}`,
		`{"maintenanceWindowUTC":"3:00-4:00"}`,
	} {
		if _, err := normalizeWritableSettingGroup("updates", json.RawMessage(raw)); err == nil {
			t.Fatalf("expected %s to be rejected", raw)
		}
	}
	if _, err := normalizeWritableSettingGroup("updates", json.RawMessage(`{"channel":"beta","maintenanceWindowUTC":"03:00-04:00"}`)); err == nil {
		t.Fatal("fictional updater settings were accepted")
	}
}

func TestServerSettingsEndpointsDenyUsersWithoutManageServer(t *testing.T) {
	serverURL, _ := newAuthTestServerWithDB(t)
	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}
	loginUser(t, adminClient, serverURL)

	status, body := doJSON(t, adminClient, http.MethodPost, serverURL+"/api/users", map[string]any{
		"username":    "viewer",
		"email":       "viewer@example.test",
		"displayName": "Viewer",
		"password":    "Viewer123456!",
		"permissions": map[string]bool{"playMedia": true},
		"libraryIds":  []string{},
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("create viewer status=%d body=%s", status, body)
	}

	viewerJar, _ := cookiejar.New(nil)
	viewerClient := &http.Client{Jar: viewerJar}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "viewer",
		"password": "Viewer123456!",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("viewer login status=%d body=%s", status, body)
	}

	for _, endpoint := range []string{
		"/api/settings",
		"/api/settings/summary",
		"/api/dlna/status",
		"/api/dashboard",
		"/api/system/storage",
		"/api/backups",
		"/api/logs",
		"/api/audit-events",
		"/api/transcode/capacity",
	} {
		status, body = doJSON(t, viewerClient, http.MethodGet, serverURL+endpoint, nil, nil)
		if status != http.StatusForbidden {
			t.Fatalf("%s leaked to viewer: status=%d body=%s", endpoint, status, body)
		}
	}

	status, body = patchSettingsGroups(t, viewerClient, serverURL, map[string]any{"dlna": map[string]any{"enabled": true}}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("settings patch was not forbidden: status=%d body=%s", status, body)
	}
	status, body = doJSON(t, viewerClient, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{"enabled": true}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("remote settings patch was not forbidden: status=%d body=%s", status, body)
	}
}

func TestMetadataSettingsStoreAndRedactTMDBCredentials(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	payload := map[string]any{
		"metadataAgents": map[string]any{
			"movies":              "TMDB",
			"tv":                  "TMDB",
			"localNFO":            true,
			"embeddedTags":        true,
			"refreshDays":         7,
			"metadataLanguage":    "en-US",
			"tmdbReadAccessToken": map[string]any{"replacement": "tmdb-read-token"},
			"tmdbAPIKey":          map[string]any{"replacement": "tmdb-api-key"},
		},
	}
	status, body := patchSettingsGroups(t, client, serverURL, payload, nil)
	if status != http.StatusOK {
		t.Fatalf("settings status = %d, body: %s", status, body)
	}
	for _, secret := range []string{"tmdb-read-token", "tmdb-api-key"} {
		if strings.Contains(body, secret) {
			t.Fatalf("settings response exposed %q: %s", secret, body)
		}
	}
	if !strings.Contains(body, `"tmdbReadAccessToken":{"present":true}`) || !strings.Contains(body, `"tmdbAPIKey":{"present":true}`) {
		t.Fatalf("settings response did not report configured TMDB credentials: %s", body)
	}
	if got := server.tmdbReadAccessToken(); got != "tmdb-read-token" {
		t.Fatalf("read access token = %q", got)
	}
	if got := server.tmdbAPIKey(); got != "tmdb-api-key" {
		t.Fatalf("api key = %q", got)
	}
	var stored string
	if err := db.QueryRow(`SELECT value_json FROM settings WHERE key = 'metadataAgents'`).Scan(&stored); err != nil {
		t.Fatalf("load metadata settings: %v", err)
	}
	if strings.Contains(stored, "tmdb-read-token") || strings.Contains(stored, "tmdb-api-key") {
		t.Fatalf("metadata settings stored secret material: %s", stored)
	}
}

func TestSettingsDocumentUsesRevisionETagAndRejectsStaleWrites(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	request, err := http.NewRequest(http.MethodGet, serverURL+"/api/settings", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var original SettingsDocument
	if err := json.NewDecoder(response.Body).Decode(&original); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if response.StatusCode != http.StatusOK || original.Revision == "" || response.Header.Get("ETag") == "" || response.Header.Get("X-Portico-Settings-Revision") != original.Revision {
		t.Fatalf("settings revision headers/status = %d %#v revision=%q", response.StatusCode, response.Header, original.Revision)
	}

	var updated SettingsDocument
	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/settings", map[string]any{
		"expectedRevision": original.Revision,
		"idempotencyKey":   randomID("settings-revision"),
		"groups": map[string]any{
			"server": map[string]any{"operatorNote": "Revision-safe"},
		},
	}, &updated)
	if status != http.StatusOK || updated.Revision == original.Revision {
		t.Fatalf("settings update status=%d body=%s revisions=%q/%q", status, body, original.Revision, updated.Revision)
	}
	serverGroup, _ := updated.Groups["server"].(map[string]any)
	if serverGroup["operatorNote"] != "Revision-safe" || serverGroup["friendlyName"] == nil {
		t.Fatalf("partial group update did not preserve fields: %#v", serverGroup)
	}
	if len(updated.ApplyImpact.ChangedFields) != 1 || updated.ApplyImpact.ChangedFields[0] != "server.operatorNote" {
		t.Fatalf("apply impact = %#v", updated.ApplyImpact)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/settings", map[string]any{
		"expectedRevision": original.Revision,
		"idempotencyKey":   randomID("settings-stale"),
		"groups":           map[string]any{"server": map[string]any{"operatorNote": "Stale"}},
	}, nil)
	if status != http.StatusConflict || !strings.Contains(body, "settings_revision_conflict") {
		t.Fatalf("stale settings write status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPut, serverURL+"/api/settings", map[string]any{}, nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("retired settings PUT status=%d body=%s", status, body)
	}
}

func TestConcurrentSettingsWritesFromOneRevisionHaveOneWinnerAndNoLostUpdate(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var original SettingsDocument
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/settings", nil, &original)
	if status != http.StatusOK || original.Revision == "" {
		t.Fatalf("load settings status=%d body=%s document=%#v", status, body, original)
	}
	type result struct {
		status int
		body   string
		err    error
	}
	write := func(note string, start <-chan struct{}, output chan<- result) {
		payload, err := json.Marshal(map[string]any{
			"expectedRevision": original.Revision,
			"idempotencyKey":   randomID("settings-concurrent"),
			"groups":           map[string]any{"server": map[string]any{"operatorNote": note}},
		})
		if err != nil {
			output <- result{err: err}
			return
		}
		<-start
		request, err := http.NewRequest(http.MethodPatch, serverURL+"/api/settings", bytes.NewReader(payload))
		if err != nil {
			output <- result{err: err}
			return
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(csrfHeaderName, "1")
		response, err := client.Do(request)
		if err != nil {
			output <- result{err: err}
			return
		}
		responseBody, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		output <- result{status: response.StatusCode, body: string(responseBody), err: readErr}
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var workers sync.WaitGroup
	for _, note := range []string{"Concurrent Alpha", "Concurrent Beta"} {
		workers.Add(1)
		go func(value string) {
			defer workers.Done()
			write(value, start, results)
		}(note)
	}
	close(start)
	workers.Wait()
	close(results)
	winners, conflicts := 0, 0
	for candidate := range results {
		if candidate.err != nil {
			t.Fatalf("concurrent settings write: %v", candidate.err)
		}
		switch candidate.status {
		case http.StatusOK:
			winners++
		case http.StatusConflict:
			conflicts++
			if !strings.Contains(candidate.body, "settings_revision_conflict") {
				t.Fatalf("settings conflict missing stable code: %s", candidate.body)
			}
		default:
			t.Fatalf("concurrent settings status=%d body=%s", candidate.status, candidate.body)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("concurrent settings results winners=%d conflicts=%d", winners, conflicts)
	}
	var current SettingsDocument
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/settings", nil, &current)
	serverGroup, _ := current.Groups["server"].(map[string]any)
	note, _ := serverGroup["operatorNote"].(string)
	if status != http.StatusOK || (note != "Concurrent Alpha" && note != "Concurrent Beta") || current.Revision == original.Revision {
		t.Fatalf("concurrent settings final status=%d note=%q revision=%q body=%s", status, note, current.Revision, body)
	}
}

func TestSettingsMutationIdempotencyReceiptReplaysExactOutcome(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var original SettingsDocument
	if status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/settings", nil, &original); status != http.StatusOK {
		t.Fatalf("load settings status=%d body=%s", status, body)
	}
	payload := map[string]any{
		"expectedRevision": original.Revision,
		"idempotencyKey":   "settings-receipt-replay-0001",
		"groups":           map[string]any{"server": map[string]any{"operatorNote": "receipt-safe"}},
	}
	var first SettingsDocument
	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/settings", payload, &first)
	if status != http.StatusOK {
		t.Fatalf("first settings mutation status=%d body=%s", status, body)
	}
	var replay SettingsDocument
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/settings", payload, &replay)
	if status != http.StatusOK || replay.Revision != first.Revision || replay.ApplyImpact.ChangedFields[0] != first.ApplyImpact.ChangedFields[0] {
		t.Fatalf("idempotent replay status=%d body=%s first=%#v replay=%#v", status, body, first, replay)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/settings", map[string]any{
		"expectedRevision": original.Revision,
		"idempotencyKey":   payload["idempotencyKey"],
		"groups":           map[string]any{"server": map[string]any{"operatorNote": "different-intent"}},
	}, nil)
	if status != http.StatusConflict || !strings.Contains(body, "settings_idempotency_conflict") {
		t.Fatalf("idempotency key reuse status=%d body=%s", status, body)
	}
}

func TestSettingsRejectUnknownAndRemovedCompatibilityFields(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := patchSettingsGroups(t, client, serverURL, map[string]any{
		"metadataAgents": map[string]any{
			"preferredMovieProvider": "local",
			"preferredTvProvider":    "tmdb",
			"preferLocalMetadata":    false,
			"preferLocal":            false,
		},
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "metadataAgents.") {
		t.Fatalf("removed metadata field status = %d, body: %s", status, body)
	}

	status, body = patchSettingsGroups(t, client, serverURL, map[string]any{
		"scheduledTasks": map[string]any{
			"enabled":              true,
			"upgradeMediaAnalysis": false,
			"trashRetentionDays":   21,
		},
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "scheduledTasks.upgradeMediaAnalysis") {
		t.Fatalf("removed scheduled field status = %d, body: %s", status, body)
	}

	status, body = patchSettingsGroups(t, client, serverURL, map[string]any{
		"metadataAgents": map[string]any{
			"movies":     "TMDB",
			"unexpected": true,
		},
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "metadataAgents.unexpected") {
		t.Fatalf("unknown metadata field status = %d, body: %s", status, body)
	}

	status, body = patchSettingsGroups(t, client, serverURL, map[string]any{
		"scheduledTasks": map[string]any{
			"enabled":   true,
			"startHour": 24,
		},
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "scheduledTasks.startHour") {
		t.Fatalf("invalid scheduler field status = %d, body: %s", status, body)
	}

	status, body = patchSettingsGroups(t, client, serverURL, map[string]any{
		"optimizedVersions": map[string]any{
			"maxConcurrentJobs": 5,
		},
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "optimizedVersions.maxConcurrentJobs") {
		t.Fatalf("invalid optimized concurrency status = %d, body: %s", status, body)
	}

	status, body = patchSettingsGroups(t, client, serverURL, map[string]any{
		"optimizedVersions": map[string]any{
			"storageDirectory": "relative/optimized",
		},
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "optimizedVersions.storageDirectory") {
		t.Fatalf("invalid optimized storage directory status = %d, body: %s", status, body)
	}
}

func TestCanonicalSettingDefaultsPassFieldAndGroupValidation(t *testing.T) {
	for group, definition := range canonicalSettingRegistry {
		t.Run(group, func(t *testing.T) {
			encoded, err := json.Marshal(definition.Defaults)
			if err != nil {
				t.Fatalf("encode defaults: %v", err)
			}
			var defaults map[string]any
			if err := json.Unmarshal(encoded, &defaults); err != nil {
				t.Fatalf("decode defaults: %v", err)
			}
			schema := writableSettingSchemas[group]
			if len(schema) == 0 {
				t.Fatal("canonical group has no writable schema")
			}
			for field, value := range defaults {
				kind, ok := schema[field]
				if !ok {
					t.Fatalf("default field %s is not in schema", field)
				}
				if err := validateSettingFieldValue(group, field, kind, value); err != nil {
					t.Fatalf("invalid default %s: %v", field, err)
				}
			}
			if err := validateSettingGroupPolicy(group, defaults); err != nil {
				t.Fatalf("invalid default group: %v", err)
			}
		})
	}
}

func TestSettingsFieldRuntimeContract(t *testing.T) {
	for group, definition := range canonicalSettingRegistry {
		for field, metadata := range definition.Fields {
			if metadata.OutcomeTest == "" || metadata.OutcomeTest != settingGroupOutcomeTest(group) {
				t.Fatalf("%s.%s does not identify its field outcome evidence: %q", group, field, metadata.OutcomeTest)
			}
			if metadata.Permission != "manageServer" || metadata.SaveMode == "" || metadata.ApplicationMode == "" {
				t.Fatalf("%s.%s has incomplete application contract metadata: %#v", group, field, metadata)
			}
		}
	}
}

func TestSettingsRejectCrossFieldAndUnboundedValues(t *testing.T) {
	tests := []struct {
		name  string
		group string
		patch map[string]any
	}{
		{name: "disabled optimized default", group: "optimizedVersions", patch: map[string]any{"defaultProfile": "720p-medium", "templates": []any{map[string]any{"id": "only", "name": "Only", "profile": "720p-medium", "enabled": false}}}},
		{name: "unbounded automatic deletion", group: "optimizedVersions", patch: map[string]any{"autoDelete": true, "retentionDays": 0, "maxPerItem": 0, "maxStorageMB": 0}},
		{name: "empty custom window", group: "scheduledTasks", patch: map[string]any{"maintenanceWindow": "custom", "startHour": 4, "endHour": 4}},
		{name: "invalid preview mode", group: "library", patch: map[string]any{"generateVideoPreview": "whenever"}},
		{name: "custom embedded tags without probe", group: "library", patch: map[string]any{"analysisTier": analysisTierCustom, "probeStreams": false, "readEmbeddedTags": true}},
		{name: "custom attachments without indexes", group: "library", patch: map[string]any{"analysisTier": analysisTierCustom, "probeStreams": true, "readEmbeddedIndexes": false, "extractAllEmbeddedAttachments": true}},
		{name: "custom selected embedded asset without indexes", group: "library", patch: map[string]any{"analysisTier": analysisTierCustom, "probeStreams": true, "readEmbeddedIndexes": false, "extractSelectedEmbeddedAssets": true}},
		{name: "custom seek validation without probe", group: "library", patch: map[string]any{"analysisTier": analysisTierCustom, "probeStreams": false, "validateSeekBehavior": true}},
		{name: "custom chapter thumbnails without indexes", group: "library", patch: map[string]any{"analysisTier": analysisTierCustom, "probeStreams": true, "readEmbeddedIndexes": false, "generateChapterThumbnails": true}},
		{name: "custom segment detection without indexes", group: "library", patch: map[string]any{"analysisTier": analysisTierCustom, "probeStreams": true, "readEmbeddedIndexes": false, "detectSegments": true}},
		{name: "invalid metadata language", group: "metadataAgents", patch: map[string]any{"metadataLanguage": "not a tag"}},
		{name: "unsafe DVR template", group: "dvr", patch: map[string]any{"recordingPathTemplate": "../{title}"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := cloneSettingMap(canonicalSettingRegistry[test.group].Defaults)
			for field, value := range test.patch {
				current[field] = value
			}
			encoded, err := json.Marshal(current)
			if err != nil {
				t.Fatalf("encode group: %v", err)
			}
			if _, err := normalizeWritableSettingGroup(test.group, encoded); err == nil {
				t.Fatal("invalid settings group was accepted")
			}
		})
	}
}
