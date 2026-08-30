package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func explicitPrimaryUser(user User) User {
	user.AccountID = user.ID
	user.ProfileID = user.ID
	user.ProfileIsPrimary = true
	user.AllowFeedback = true
	return user
}

func TestHostedViewerPreferenceBundleProjectsPublicIdentityWithoutChangingStorageScope(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	user.AuthOrigin = "portico"
	user.AuthProvider = "portico"
	user.PorticoUserID = "hosted-account-1"
	const hostedProfileID = "hosted-profile-1"
	const hostedServerID = "hosted-server-1"
	const installationID = "hosted-installation-1"

	if _, err := server.db.Exec(`
		UPDATE users SET auth_origin = 'portico', portico_user_id = ? WHERE id = ?`,
		user.PorticoUserID, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		UPDATE profiles SET origin = 'hosted', external_profile_id = ? WHERE id = ? AND account_id = ?`,
		hostedProfileID, user.ProfileID, account.ID); err != nil {
		t.Fatal(err)
	}
	upsertJSONSetting(t, server.db, remoteAccessSettingsKey, map[string]any{
		"enabled":  true,
		"serverId": hostedServerID,
	})

	scope, err := server.viewerPreferenceScope(
		context.Background(), user, "mobile", installationID, "profile-server",
	)
	if err != nil {
		t.Fatal(err)
	}
	if scope.AccountID != account.ID || scope.ProfileID != user.ProfileID || scope.ServerID == hostedServerID {
		t.Fatalf("preference storage scope stopped using private server IDs: %#v", scope)
	}
	bundle, err := server.loadViewerPreferenceBundle(context.Background(), user, scope)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Identity.Authority != "hosted" ||
		bundle.Identity.AccountID != user.PorticoUserID ||
		bundle.Identity.ProfileID != hostedProfileID ||
		bundle.Identity.ServerID != hostedServerID ||
		bundle.Identity.DeviceClass != "mobile" ||
		bundle.Identity.InstallationID != installationID {
		t.Fatalf("hosted preference response exposed the wrong viewer identity: %#v", bundle.Identity)
	}

	var privateDocuments int
	if err := server.db.QueryRow(`
		SELECT COUNT(*) FROM viewer_preference_documents
		WHERE authority = 'hosted' AND account_id = ? AND server_id = ?`,
		account.ID, scope.ServerID).Scan(&privateDocuments); err != nil {
		t.Fatal(err)
	}
	if privateDocuments != 3 {
		t.Fatalf("private preference storage documents=%d, want 3", privateDocuments)
	}
	var publicDocuments int
	if err := server.db.QueryRow(`
		SELECT COUNT(*) FROM viewer_preference_documents
		WHERE account_id = ? OR profile_id = ? OR server_id = ?`,
		user.PorticoUserID, hostedProfileID, hostedServerID).Scan(&publicDocuments); err != nil {
		t.Fatal(err)
	}
	if publicDocuments != 0 {
		t.Fatalf("public Hosted IDs leaked into private preference storage: %d documents", publicDocuments)
	}
}

func TestViewerPreferenceDefaultsLimitsDeviceIsolationAndCanonicalConsumption(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	user.Permissions = map[string]bool{"playMedia": true, "downloadMedia": true}
	const installationID = "shared-installation-0001"
	mobileScope, err := server.viewerPreferenceScope(context.Background(), user, "mobile", installationID, "profile-server")
	if err != nil {
		t.Fatal(err)
	}
	mobile, err := server.loadViewerPreferenceBundle(context.Background(), user, mobileScope)
	if err != nil {
		t.Fatal(err)
	}
	televisionScope, err := server.viewerPreferenceScope(context.Background(), user, "television", installationID, "profile-server")
	if err != nil {
		t.Fatal(err)
	}
	television, err := server.loadViewerPreferenceBundle(context.Background(), user, televisionScope)
	if err != nil {
		t.Fatal(err)
	}
	var mobileAccount, televisionAccount accountInstallationPreferenceValues
	_ = json.Unmarshal(mobile.AccountServerInstallation.Values, &mobileAccount)
	_ = json.Unmarshal(television.AccountServerInstallation.Values, &televisionAccount)
	if mobileAccount.ProfileSelection != "last-used" || televisionAccount.ProfileSelection != "last-used" {
		t.Fatalf("device defaults mobile=%#v television=%#v", mobileAccount, televisionAccount)
	}
	if !mobile.Policy.DownloadsAllowed || television.Policy.DownloadsAllowed {
		t.Fatalf("download policy mobile=%v television=%v", mobile.Policy.DownloadsAllowed, television.Policy.DownloadsAllowed)
	}
	var installationDocuments int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM viewer_preference_documents WHERE scope_type = 'account-server-installation' AND account_id = ? AND installation_id = ?`, account.ID, installationID).Scan(&installationDocuments); err != nil || installationDocuments != 1 {
		t.Fatalf("cross-device installation documents=%d err=%v", installationDocuments, err)
	}
	mobileInstallationScope := mobileScope
	mobileInstallationScope.Type = "account-server-installation"
	if _, err := server.patchViewerPreferenceDocument(context.Background(), user, mobileInstallationScope, preferencePatchRequest{
		Version: "v1", ExpectedRevision: mobile.AccountServerInstallation.Revision,
		Changes: json.RawMessage(`{"rememberAccount":false,"profileSelection":"ask"}`),
	}); err != nil {
		t.Fatalf("patch mobile installation preference: %v", err)
	}
	reloadedMobile, err := server.loadViewerPreferenceBundle(context.Background(), user, mobileScope)
	if err != nil {
		t.Fatal(err)
	}
	reloadedTelevision, err := server.loadViewerPreferenceBundle(context.Background(), user, televisionScope)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(reloadedMobile.AccountServerInstallation.Values, &mobileAccount)
	_ = json.Unmarshal(reloadedTelevision.AccountServerInstallation.Values, &televisionAccount)
	if mobileAccount.RememberAccount || televisionAccount.RememberAccount || mobileAccount.ProfileSelection != "ask" || televisionAccount.ProfileSelection != "ask" {
		t.Fatalf("installation preference did not remain shared mobile=%#v television=%#v", mobileAccount, televisionAccount)
	}

	profileDoc, err := server.patchViewerPreferenceDocument(context.Background(), user, mobileScope, preferencePatchRequest{
		Version: "v1", ExpectedRevision: 0,
		Changes: json.RawMessage(`{"playback":{"playedThresholdPercent":75},"privacy":{"pauseWatchHistory":true}}`),
	})
	if err != nil || profileDoc.Revision != 1 {
		t.Fatalf("profile limits patch doc=%#v err=%v", profileDoc, err)
	}
	if got := server.playbackProgressPreferencesForUserContext(context.Background(), viewerProfileID(user)); got.PlayedThresholdPercent != 75 {
		t.Fatalf("canonical played threshold not consumed: %#v", got)
	}
	if got := server.userPrivacyPreferencesForProfileContext(context.Background(), viewerProfileID(user)); !got.PauseWatchHistory {
		t.Fatalf("canonical privacy not consumed: %#v", got)
	}

	deviceScope := mobileScope
	deviceScope.Type = "profile-device-class"
	_, err = server.patchViewerPreferenceDocument(context.Background(), user, deviceScope, preferencePatchRequest{
		Version: "v1", ExpectedRevision: 0,
		Changes: json.RawMessage(`{"appearance":{"cardSizePercent":150},"playback":{"quality":{"local":{"mode":"high","maxAudioBitrateKbps":4096,"maxVideoHeight":4320}}}}`),
	})
	if err != nil {
		t.Fatalf("canonical client boundary values rejected: %v", err)
	}
	user.Permissions["downloadMedia"] = false
	withoutDownloads, err := server.loadViewerPreferenceBundle(context.Background(), user, mobileScope)
	if err != nil || withoutDownloads.Policy.DownloadsAllowed {
		t.Fatalf("download permission escaped preference policy: policy=%#v err=%v", withoutDownloads.Policy, err)
	}
	if _, err := server.db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES ('viewerFeedback', '{"enabled":false}', ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	feedbackDisabled, err := server.loadViewerPreferenceBundle(context.Background(), user, mobileScope)
	if err != nil || feedbackDisabled.Policy.FeedbackAllowed {
		t.Fatalf("server feedback ceiling escaped preference policy: policy=%#v err=%v", feedbackDisabled.Policy, err)
	}
}

func TestViewerPreferencesInitialPatchConflictAndPolicyClamp(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	user.RemoteBitrateLimitMbps = 2
	scope, err := server.viewerPreferenceScope(context.Background(), user, "web", "browser-installation-0001", "profile-server")
	if err != nil {
		t.Fatalf("preference scope: %v", err)
	}
	bundle, err := server.loadViewerPreferenceBundle(context.Background(), user, scope)
	if err != nil {
		t.Fatalf("load initial bundle: %v", err)
	}
	if bundle.ProfileServer.Revision != 0 || bundle.ProfileDeviceClass.Revision != 0 || bundle.AccountServerInstallation.Revision != 0 {
		t.Fatalf("initial revisions = %#v", bundle)
	}
	var effective profileDeviceClassPreferenceValues
	if err := json.Unmarshal(bundle.EffectiveProfileDeviceClass.Values, &effective); err != nil {
		t.Fatalf("decode effective device preferences: %v", err)
	}
	for network, quality := range effective.Playback.Quality {
		if quality.MaxVideoBitrateMbps == nil || *quality.MaxVideoBitrateMbps > 2 {
			t.Fatalf("network %s escaped server bitrate clamp: %#v", network, quality)
		}
	}
	if effective.Playback.Quality["cellular"].Mode != "off" {
		t.Fatalf("web cellular mode was not clamped off: %#v", effective.Playback.Quality["cellular"])
	}

	changes := json.RawMessage(`{"search":{"rememberHistory":true,"recentQueries":["quiet films"]}}`)
	doc, err := server.patchViewerPreferenceDocument(context.Background(), user, scope, preferencePatchRequest{
		Version: viewerPreferencesVersion, ExpectedRevision: 0, Changes: changes,
	})
	if err != nil || doc.Revision != 1 {
		t.Fatalf("first patch = %#v, err=%v", doc, err)
	}
	conflict, err := server.patchViewerPreferenceDocument(context.Background(), user, scope, preferencePatchRequest{
		Version: viewerPreferencesVersion, ExpectedRevision: 0, Changes: changes,
	})
	if !errors.Is(err, errPreferenceConflict) || conflict.Revision != 1 {
		t.Fatalf("stale patch = %#v, err=%v", conflict, err)
	}
	doc, err = server.patchViewerPreferenceDocument(context.Background(), user, scope, preferencePatchRequest{
		Version: viewerPreferencesVersion, ExpectedRevision: 1,
		Changes: json.RawMessage(`{"search":{"rememberHistory":false}}`),
	})
	if err != nil {
		t.Fatalf("disable search history: %v", err)
	}
	var profile profileServerPreferenceValues
	if err := json.Unmarshal(doc.Values, &profile); err != nil {
		t.Fatalf("decode patched profile preferences: %v", err)
	}
	if profile.Search.RememberHistory || len(profile.Search.RecentQueries) != 0 {
		t.Fatalf("disabling search retention did not clear queries: %#v", profile.Search)
	}
}

func TestViewerPreferenceReadRepairsAndQuarantinesInvalidCanonicalDocument(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	scope, err := server.viewerPreferenceScope(context.Background(), user, "web", "repair-installation-0001", "profile-server")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := server.loadViewerPreferenceBundle(context.Background(), user, scope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		UPDATE viewer_preference_documents SET values_json = '{"localization":{}}'
		WHERE scope_type = 'profile-server' AND account_id = ? AND profile_id = ?`, account.ID, account.ID); err != nil {
		t.Fatal(err)
	}

	repaired, err := server.loadViewerPreferenceBundle(context.Background(), user, scope)
	if err != nil {
		t.Fatalf("load repaired bundle: %v", err)
	}
	if repaired.ProfileServer.Revision != initial.ProfileServer.Revision+1 {
		t.Fatalf("repaired revision=%d want=%d", repaired.ProfileServer.Revision, initial.ProfileServer.Revision+1)
	}
	var value profileServerPreferenceValues
	if err := json.Unmarshal(repaired.ProfileServer.Values, &value); err != nil {
		t.Fatal(err)
	}
	if value.Localization.Locale == "" || value.Playback.PlayedThresholdPercent == 0 {
		t.Fatalf("repair did not restore canonical defaults: %#v", value)
	}
	var quarantinedRaw string
	if err := server.db.QueryRow(`
		SELECT values_json FROM viewer_preference_document_quarantine
		WHERE account_id = ? AND profile_id = ? ORDER BY quarantined_at DESC LIMIT 1`, account.ID, account.ID).Scan(&quarantinedRaw); err != nil {
		t.Fatalf("load quarantine evidence: %v", err)
	}
	if quarantinedRaw != `{"localization":{}}` {
		t.Fatalf("quarantined raw=%q", quarantinedRaw)
	}
}

func TestViewerPreferencesRejectUnknownAndUnsupportedValues(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	scope, err := server.viewerPreferenceScope(context.Background(), user, "mobile", "mobile-installation-0001", "profile-server")
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	for name, changes := range map[string]string{
		"unknown field":        `{"privacy":{"sendRawLogs":true}}`,
		"retired countdown":    `{"playback":{"upNextCountdownSeconds":20}}`,
		"retired segment word": `{"playback":{"introSkip":"never"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := server.patchViewerPreferenceDocument(context.Background(), user, scope, preferencePatchRequest{
				Version: viewerPreferencesVersion, ExpectedRevision: 0, Changes: json.RawMessage(changes),
			})
			if !errors.Is(err, errPreferenceValidation) {
				t.Fatalf("patch error=%v", err)
			}
		})
	}
}

func TestViewerNotificationsAreRecipientIsolatedAndNeverUseGlobalEvents(t *testing.T) {
	server := newScannerTestServer(t)
	account, childProfile := createProfileProtocolAccount(t, server)
	primary := explicitPrimaryUser(account)
	child := primary
	child.ProfileID = childProfile.ID
	child.ProfileIsPrimary = false
	child.DisplayName = childProfile.DisplayName
	serverID, err := server.profileDirectoryServerIDContext(context.Background())
	if err != nil {
		t.Fatalf("server id: %v", err)
	}
	primaryRecipient := notificationRecipient{Authority: "local", AccountID: account.ID, ServerID: serverID, Audience: "profile", ProfileID: primary.ProfileID}
	childRecipient := notificationRecipient{Authority: "local", AccountID: account.ID, ServerID: serverID, Audience: "profile", ProfileID: child.ProfileID}
	events := server.subscribeAppEvents()
	defer server.unsubscribeAppEvents(events)
	for recipient, body := range map[notificationRecipient]string{primaryRecipient: "Primary only", childRecipient: "Child only"} {
		err := server.withUserTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
			return createViewerNotificationTx(tx, recipient, "server.message", "informational", "notification.server-message", "status.notification",
				map[string]string{}, &viewerNotificationContent{Body: body}, []viewerNotificationAction{}, "", time.Now().UTC().Add(time.Hour))
		})
		if err != nil {
			t.Fatalf("create notification: %v", err)
		}
	}
	select {
	case event := <-events:
		t.Fatalf("private notification leaked to global event stream: %#v", event)
	default:
	}
	for recipient, expectedBody := range map[notificationRecipient]string{primaryRecipient: "Primary only", childRecipient: "Child only"} {
		page, err := server.listViewerNotificationsContext(context.Background(), recipient, 25, "", "", false)
		if err != nil {
			t.Fatalf("list notifications: %v", err)
		}
		if len(page.Items) != 1 || page.Items[0].Content == nil || page.Items[0].Content.Body != expectedBody || page.UnreadCount != 1 || page.Revision != 1 {
			t.Fatalf("recipient page = %#v", page)
		}
		if page.Items[0].Recipient.ProfileID != recipient.ProfileID {
			t.Fatalf("recipient identity drifted: %#v", page.Items[0].Recipient)
		}
	}
	_ = child
}

func TestViewerNotificationEndpointsRequireInteractiveSession(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	primary := explicitPrimaryUser(account)
	primary.Permissions = map[string]bool{"read": true, "manageServer": true}

	routes := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "list", method: http.MethodGet, path: "/api/notifications?audience=account-admin"},
		{name: "single receipt", method: http.MethodPatch, path: "/api/notifications/notification-id", body: `{"read":true}`},
		{name: "read all", method: http.MethodPost, path: "/api/notifications/read-all?audience=account-admin"},
		{name: "batch archive", method: http.MethodPost, path: "/api/notifications/receipts?audience=account-admin", body: `{"version":"v1","expectedRevision":0,"recipient":{"authority":"local","accountId":"ignored","serverId":"ignored","profileId":"","audience":"account-admin"},"action":"archive","notificationIds":["notification-id"]}`},
		{name: "events", method: http.MethodGet, path: "/api/notifications/events?audience=account-admin"},
	}
	for _, scopes := range [][]string{{"read"}, {"all"}} {
		t.Run("api key "+scopes[0], func(t *testing.T) {
			apiKey := primary
			apiKey.AuthProvider = "api_key"
			apiKey.APIKeyID = "api-key-" + scopes[0]
			apiKey.APIKeyScopes = scopes
			for _, route := range routes {
				t.Run(route.name, func(t *testing.T) {
					request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
					request.Header.Set("Content-Type", "application/json")
					if route.name == "events" {
						ctx, cancel := context.WithCancel(request.Context())
						cancel()
						request = request.WithContext(ctx)
					}
					recorder := httptest.NewRecorder()
					server.handleViewerNotifications(recorder, request, apiKey)
					if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "interactive_session_required") {
						t.Fatalf("%s API key reached %s: status=%d body=%s", scopes[0], route.name, recorder.Code, recorder.Body.String())
					}
				})
			}
		})
	}

	nonInteractive := httptest.NewRecorder()
	server.handleViewerNotifications(nonInteractive, httptest.NewRequest(http.MethodGet, "/api/notifications", nil), primary)
	if nonInteractive.Code != http.StatusForbidden {
		t.Fatalf("non-interactive principal listed notifications: status=%d body=%s", nonInteractive.Code, nonInteractive.Body.String())
	}

	const installationID = "notification-session-installation-0001"
	deviceID := bindProfileTestDevice(t, server.db, server, account.ID, installationID)
	if _, err := server.db.Exec(`UPDATE devices SET app = 'portico-web', platform = 'web' WHERE id = ?`, deviceID); err != nil {
		t.Fatalf("mark notification session device interactive: %v", err)
	}
	const sessionToken = "notification-interactive-session"
	now := time.Now().UTC()
	if _, err := server.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES ('session_notifications_interactive', ?, ?, ?, ?, ?, ?, ?)`, account.ID, account.ID, deviceID, hashToken(sessionToken),
		now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed interactive notification session: %v", err)
	}
	interactiveRequest := httptest.NewRequest(http.MethodGet, "/api/notifications?audience=account-admin", nil)
	interactiveRequest.Header.Set("Authorization", "Bearer "+sessionToken)
	interactive := httptest.NewRecorder()
	server.handleViewerNotifications(interactive, interactiveRequest, primary)
	if interactive.Code != http.StatusOK {
		t.Fatalf("interactive account administrator could not list notifications: status=%d body=%s", interactive.Code, interactive.Body.String())
	}
}

func TestNotificationReceiptDatabaseGuardRejectsRecipientMismatch(t *testing.T) {
	server := newScannerTestServer(t)
	account, child := createProfileProtocolAccount(t, server)
	serverID, _ := server.profileDirectoryServerIDContext(context.Background())
	recipient := notificationRecipient{Authority: "local", AccountID: account.ID, ServerID: serverID, Audience: "profile", ProfileID: child.ID}
	var notificationID string
	err := server.withUserTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
		if err := createViewerNotificationTx(tx, recipient, "server.message", "informational", "notification.server-message", "status.notification", map[string]string{}, &viewerNotificationContent{Body: "Private"}, []viewerNotificationAction{}, "", time.Now().UTC().Add(time.Hour)); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT id FROM viewer_notifications WHERE profile_id = ?`, child.ID).Scan(&notificationID)
	})
	if err != nil {
		t.Fatalf("seed notification: %v", err)
	}
	_, err = server.db.Exec(`INSERT INTO viewer_notification_receipts (notification_id, authority, account_id, server_id, profile_id, audience, read_at, archived_at, updated_at) VALUES (?, 'local', ?, ?, ?, 'profile', '', '', ?)`,
		notificationID, account.ID, serverID, account.ID, time.Now().UTC().Format(time.RFC3339Nano))
	if err == nil || !strings.Contains(err.Error(), "recipient mismatch") {
		t.Fatalf("mismatched receipt error=%v", err)
	}
}

func TestNotificationReceiptAndRevisionIdentityIncludesAuthority(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	serverID, _ := server.profileDirectoryServerIDContext(context.Background())
	local := notificationRecipient{Authority: "local", AccountID: account.ID, ServerID: serverID, Audience: "profile", ProfileID: account.ID}
	hosted := local
	hosted.Authority = "hosted"
	ids := map[string]string{}
	for _, recipient := range []notificationRecipient{local, hosted} {
		var notificationID string
		err := server.withUserTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
			if err := createViewerNotificationTx(tx, recipient, "server.message", "informational", "notification.server-message", "status.notification", map[string]string{}, &viewerNotificationContent{Body: recipient.Authority}, []viewerNotificationAction{}, "", time.Now().UTC().Add(time.Hour)); err != nil {
				return err
			}
			return tx.QueryRow(`SELECT id FROM viewer_notifications WHERE authority = ? AND account_id = ? AND profile_id = ?`, recipient.Authority, recipient.AccountID, recipient.ProfileID).Scan(&notificationID)
		})
		if err != nil {
			t.Fatalf("seed %s notification: %v", recipient.Authority, err)
		}
		ids[recipient.Authority] = notificationID
	}
	read := true
	if _, err := server.mutateViewerNotificationContext(context.Background(), local, ids["local"], notificationMutationRequest{Read: &read}); err != nil {
		t.Fatalf("read local notification: %v", err)
	}
	localUnread, localRevision, err := notificationCountsContext(server, context.Background(), local)
	if err != nil || localUnread != 0 || localRevision != 2 {
		t.Fatalf("local receipt state unread=%d revision=%d err=%v", localUnread, localRevision, err)
	}
	hostedUnread, hostedRevision, err := notificationCountsContext(server, context.Background(), hosted)
	if err != nil || hostedUnread != 1 || hostedRevision != 1 {
		t.Fatalf("hosted receipt state was overwritten unread=%d revision=%d err=%v", hostedUnread, hostedRevision, err)
	}
}

func TestFeedbackValidationAndDiagnosticsAreBoundedAndServerConstructed(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	request := viewerFeedbackRequest{
		Version: "v1", Kind: "general", Category: "other", Message: "Please add a family row",
		Context: feedbackContextRequest{DeviceClass: "web", Platform: "web", AppVersion: "1.0.0"},
	}
	if err := validateViewerFeedbackRequest(request); err != nil {
		t.Fatalf("valid feedback rejected: %v", err)
	}
	diagnostics, _, err := server.viewerFeedbackDiagnosticsContext(context.Background(), user, request)
	if err != nil {
		t.Fatalf("build diagnostics: %v", err)
	}
	encoded, _ := json.Marshal(diagnostics)
	for _, forbidden := range []string{"path", "token", "email", "ip", "url", "log"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("diagnostics leaked forbidden field %q: %s", forbidden, encoded)
		}
	}
	for _, required := range []string{"deviceClass", "platform", "appVersion", "occurredAt"} {
		if _, ok := diagnostics[required]; !ok {
			t.Fatalf("diagnostics missing %s: %#v", required, diagnostics)
		}
	}
}

func TestAutomaticProfileTrustIsDeviceAndPINRevisionBound(t *testing.T) {
	server := newScannerTestServer(t)
	account, child := createProfileProtocolAccount(t, server)
	serverID, _ := server.profileDirectoryServerIDContext(context.Background())
	deviceID := bindProfileTestDevice(t, server.db, server, account.ID, "installation-automatic-0001")
	now := time.Now().UTC()
	token := "ptc_loc_automatic_trust_test"
	if _, err := server.db.Exec(`
		INSERT INTO automatic_profile_selection_trusts (
			id, token_hash, authority, account_id, server_id, device_id, installation_id, profile_id, pin_revision,
			expires_at, revoked_at, last_used_at, created_at, updated_at
		) VALUES ('ptrust_device_mismatch', ?, 'local', ?, ?, ?, 'installation-automatic-9999', ?, ?, ?, '', '', ?, ?)`,
		hashToken("ptc_loc_mismatched_device_trust"), account.ID, serverID, deviceID, child.ID, child.PINRevision,
		now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err == nil {
		t.Fatal("database accepted automatic trust whose installation did not match its device")
	}
	_, err := server.db.Exec(`
		INSERT INTO automatic_profile_selection_trusts (
			id, token_hash, authority, account_id, server_id, device_id, installation_id, profile_id, pin_revision,
			expires_at, revoked_at, last_used_at, created_at, updated_at
		) VALUES ('ptrust_test', ?, 'local', ?, ?, ?, 'installation-automatic-0001', ?, ?, ?, '', '', ?, ?)`,
		hashToken(token), account.ID, serverID, deviceID, child.ID, child.PINRevision, now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert trust: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE automatic_profile_selection_trusts SET installation_id = 'installation-automatic-9999' WHERE id = 'ptrust_test'`); err == nil {
		t.Fatal("database allowed automatic trust to move away from its device installation")
	}
	_, otherChild := createProfileProtocolAccountNamed(t, server, "profile-protocol-other")
	if _, err := server.db.Exec(`UPDATE automatic_profile_selection_trusts SET profile_id = ? WHERE id = 'ptrust_test'`, otherChild.ID); err == nil {
		t.Fatal("database allowed automatic trust to move to a profile owned by another account")
	}
	if _, err := server.db.Exec(`UPDATE automatic_profile_selection_trusts SET pin_revision = pin_revision + 1 WHERE id = 'ptrust_test'`); err == nil {
		t.Fatal("database allowed automatic trust to claim a non-current PIN revision")
	}
	record := profileAccountAuthenticationRecord{AccountID: account.ID, AuthProvider: "local", DeviceID: deviceID, InstallationID: "different-optional-installation-metadata"}
	err = server.withUserTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
		return automaticProfileTrustTx(tx, token, record, child.ID, serverID, now)
	})
	if err != nil {
		t.Fatalf("current trust rejected: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, child.ID, "9753"); err != nil {
		t.Fatalf("rotate child pin: %v", err)
	}
	err = server.withUserTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
		return automaticProfileTrustTx(tx, token, record, child.ID, serverID, now)
	})
	if !errors.Is(err, errInvalidProfileSelectionGrant) {
		t.Fatalf("trust survived PIN revision: %v", err)
	}
}

func TestHostedProfileTrustReopensOnlyTheRememberedHostedSession(t *testing.T) {
	server := newScannerTestServer(t)
	account, child := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	user.ProfileID = child.ID
	user.ProfileIsPrimary = false
	user.DisplayName = child.DisplayName
	user.AuthOrigin, user.AuthProvider = "portico", "portico"
	installationID := "hosted-installation-0001"
	deviceID := bindProfileTestDevice(t, server.db, server, account.ID, installationID)
	sessionToken := "ptc_loc_hosted_profile_session_test"
	now := time.Now().UTC()
	if _, err := server.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, profile_identity_id, auth_provider, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES ('sess_hosted_trust_test', ?, ?, '', 'portico', ?, ?, ?, ?, ?)`,
		account.ID, child.ID, deviceID, hashToken(sessionToken), now.Add(time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert hosted session: %v", err)
	}
	user.DeviceID = deviceID
	nonInteractive := httptest.NewRecorder()
	nonInteractiveRequest := httptest.NewRequest(http.MethodPost, "/api/account/profile-trusts", bytes.NewBufferString(`{"installationId":"hosted-installation-0001"}`))
	nonInteractiveRequest.Header.Set("Content-Type", "application/json")
	server.handleAutomaticProfileTrusts(nonInteractive, nonInteractiveRequest, user)
	if nonInteractive.Code != http.StatusForbidden {
		t.Fatalf("non-interactive principal minted profile trust status=%d body=%s", nonInteractive.Code, nonInteractive.Body.String())
	}

	mismatched := httptest.NewRecorder()
	mismatchRequest := httptest.NewRequest(http.MethodPost, "/api/account/profile-trusts", bytes.NewBufferString(`{"installationId":"hosted-installation-9999"}`))
	mismatchRequest.Header.Set("Content-Type", "application/json")
	mismatchRequest.Header.Set("Authorization", "Bearer "+sessionToken)
	server.handleAutomaticProfileTrusts(mismatched, mismatchRequest, user)
	if mismatched.Code != http.StatusCreated {
		t.Fatalf("optional installation metadata changed profile trust authority: status=%d body=%s", mismatched.Code, mismatched.Body.String())
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/account/profile-trusts", bytes.NewBufferString(`{"installationId":"hosted-installation-0001"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+sessionToken)
	server.handleAutomaticProfileTrusts(recorder, request, user)
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"authority":"hosted"`) {
		t.Fatalf("hosted trust status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var count int
	_ = server.db.QueryRow(`SELECT COUNT(*) FROM automatic_profile_selection_trusts WHERE authority = 'hosted'`).Scan(&count)
	if count != 1 {
		t.Fatalf("hosted session trust count=%d", count)
	}
	var trust automaticProfileTrustResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &trust); err != nil {
		t.Fatal(err)
	}
	serverID, _ := server.profileDirectoryServerIDContext(context.Background())
	localAuthentication := profileAccountAuthenticationRecord{AccountID: account.ID, AuthProvider: "local", DeviceID: deviceID, InstallationID: installationID}
	err := server.withUserTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
		return automaticProfileTrustTx(tx, trust.Token, localAuthentication, viewerProfileID(user), serverID, time.Now().UTC())
	})
	if !errors.Is(err, errInvalidProfileSelectionGrant) {
		t.Fatalf("Hosted trust crossed into Local Auth selection: %v", err)
	}

	primarySessionToken := "ptc_loc_hosted_primary_session_test"
	if _, err := server.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, profile_identity_id, auth_provider, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES ('sess_hosted_primary_test', ?, ?, '', 'portico', ?, ?, ?, ?, ?)`,
		account.ID, account.ID, deviceID, hashToken(primarySessionToken), now.Add(time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert hosted primary session: %v", err)
	}
	primaryUser := explicitPrimaryUser(account)
	primaryUser.AuthOrigin, primaryUser.AuthProvider, primaryUser.DeviceID = "portico", "portico", deviceID
	wrongProfile := httptest.NewRecorder()
	wrongProfileRequest := httptest.NewRequest(http.MethodPost, "/api/account/profile-trusts/redeem", bytes.NewBufferString(`{"token":"`+trust.Token+`"}`))
	wrongProfileRequest.Header.Set("Content-Type", "application/json")
	wrongProfileRequest.Header.Set("Authorization", "Bearer "+primarySessionToken)
	server.handleAutomaticProfileTrusts(wrongProfile, wrongProfileRequest, primaryUser)
	if wrongProfile.Code != http.StatusUnauthorized {
		t.Fatalf("child Hosted trust opened primary profile status=%d body=%s", wrongProfile.Code, wrongProfile.Body.String())
	}

	redeem := httptest.NewRecorder()
	redeemRequest := httptest.NewRequest(http.MethodPost, "/api/account/profile-trusts/redeem", bytes.NewBufferString(`{"token":"`+trust.Token+`"}`))
	redeemRequest.Header.Set("Content-Type", "application/json")
	redeemRequest.Header.Set("Authorization", "Bearer "+sessionToken)
	server.handleAutomaticProfileTrusts(redeem, redeemRequest, user)
	if redeem.Code != http.StatusNoContent {
		t.Fatalf("authoritative Hosted trust redemption status=%d body=%s", redeem.Code, redeem.Body.String())
	}
	var lastUsed string
	if err := server.db.QueryRow(`SELECT last_used_at FROM automatic_profile_selection_trusts WHERE token_hash = ?`, hashToken(trust.Token)).Scan(&lastUsed); err != nil || lastUsed == "" {
		t.Fatalf("Hosted trust redemption was not recorded: lastUsed=%q err=%v", lastUsed, err)
	}

	if _, err := server.db.Exec(`UPDATE automatic_profile_selection_trusts SET revoked_at = ? WHERE token_hash = ?`, time.Now().UTC().Format(time.RFC3339Nano), hashToken(trust.Token)); err != nil {
		t.Fatal(err)
	}
	revoked := httptest.NewRecorder()
	revokedRequest := httptest.NewRequest(http.MethodPost, "/api/account/profile-trusts/redeem", bytes.NewBufferString(`{"token":"`+trust.Token+`"}`))
	revokedRequest.Header.Set("Content-Type", "application/json")
	revokedRequest.Header.Set("Authorization", "Bearer "+sessionToken)
	server.handleAutomaticProfileTrusts(revoked, revokedRequest, user)
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked Hosted trust status=%d body=%s", revoked.Code, revoked.Body.String())
	}

	mintTrust := func() automaticProfileTrustResponse {
		t.Helper()
		mintRecorder := httptest.NewRecorder()
		mintRequest := httptest.NewRequest(http.MethodPost, "/api/account/profile-trusts", bytes.NewBufferString(`{"installationId":"hosted-installation-0001"}`))
		mintRequest.Header.Set("Content-Type", "application/json")
		mintRequest.Header.Set("Authorization", "Bearer "+sessionToken)
		server.handleAutomaticProfileTrusts(mintRecorder, mintRequest, user)
		if mintRecorder.Code != http.StatusCreated {
			t.Fatalf("remint Hosted trust status=%d body=%s", mintRecorder.Code, mintRecorder.Body.String())
		}
		var minted automaticProfileTrustResponse
		if err := json.Unmarshal(mintRecorder.Body.Bytes(), &minted); err != nil {
			t.Fatal(err)
		}
		return minted
	}
	redeemTrust := func(token string) *httptest.ResponseRecorder {
		t.Helper()
		result := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/account/profile-trusts/redeem", bytes.NewBufferString(`{"token":"`+token+`"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+sessionToken)
		server.handleAutomaticProfileTrusts(result, request, user)
		return result
	}
	expiredTrust := mintTrust()
	if _, err := server.db.Exec(`UPDATE automatic_profile_selection_trusts SET expires_at = ? WHERE token_hash = ?`, now.Add(-time.Minute).Format(time.RFC3339Nano), hashToken(expiredTrust.Token)); err != nil {
		t.Fatal(err)
	}
	if expired := redeemTrust(expiredTrust.Token); expired.Code != http.StatusUnauthorized {
		t.Fatalf("expired Hosted trust status=%d body=%s", expired.Code, expired.Body.String())
	}
	revisionTrust := mintTrust()
	if _, err := server.db.Exec(`UPDATE profiles SET pin_revision = pin_revision + 1 WHERE id = ? AND account_id = ?`, viewerProfileID(user), account.ID); err != nil {
		t.Fatal(err)
	}
	if stale := redeemTrust(revisionTrust.Token); stale.Code != http.StatusUnauthorized {
		t.Fatalf("PIN-revision-stale Hosted trust status=%d body=%s", stale.Code, stale.Body.String())
	}

	apiKey := user
	apiKey.APIKeyID, apiKey.AuthProvider = "api-key-test", "api_key"
	apiKeyRecorder := httptest.NewRecorder()
	apiKeyRequest := httptest.NewRequest(http.MethodPost, "/api/account/profile-trusts", bytes.NewBufferString(`{"installationId":"hosted-installation-0001"}`))
	apiKeyRequest.Header.Set("Content-Type", "application/json")
	apiKeyRequest.Header.Set("Authorization", "Bearer "+sessionToken)
	server.handleAutomaticProfileTrusts(apiKeyRecorder, apiKeyRequest, apiKey)
	if apiKeyRecorder.Code != http.StatusForbidden {
		t.Fatalf("API key principal minted profile trust status=%d body=%s", apiKeyRecorder.Code, apiKeyRecorder.Body.String())
	}
}

func TestLocalProfileCeilingRevokesChildrenAndBlocksManagement(t *testing.T) {
	server := newScannerTestServer(t)
	account, child := createProfileProtocolAccount(t, server)
	if _, err := server.db.Exec(`UPDATE users SET allow_account_profiles = 0 WHERE id = ?`, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.resolveRequestPrincipalContext(context.Background(), account.ID, child.ID); !errors.Is(err, errProfileNotAllowed) {
		t.Fatalf("active child session ceiling err=%v", err)
	}
	if _, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{DisplayName: "Blocked", Restrictions: defaultProfileRestrictions()}); !errors.Is(err, errProfileNotAllowed) {
		t.Fatalf("profile creation ceiling err=%v", err)
	}
	directory, err := server.managedProfileDirectoryContext(context.Background(), explicitPrimaryUser(account))
	if err != nil || directory.ProfilesAllowed || directory.CanManage {
		t.Fatalf("managed directory ceiling=%#v err=%v", directory, err)
	}
}

func TestSoftDisabledProfileRemovesPreferenceDocuments(t *testing.T) {
	server := newScannerTestServer(t)
	account, child := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	user.ProfileID, user.ProfileIsPrimary = child.ID, false
	scope, err := server.viewerPreferenceScope(context.Background(), user, "web", "profile-cleanup-installation", "profile-server")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.loadViewerPreferenceBundle(context.Background(), user, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := server.deleteManagedProfileContext(context.Background(), account.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM viewer_preference_documents WHERE profile_id = ?`, child.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("disabled profile preference count=%d err=%v", count, err)
	}
}

func createViewerFeedbackTestOwner(t *testing.T, server *Server, username string) User {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewBufferString(`{"serverName":"Feedback Test Server","username":"`+username+`","email":"`+username+`@example.test","displayName":"Server Owner","password":"Feedback-owner-password1","setupMode":"local_only","localOnlyAcknowledged":true}`))
	request.Header.Set("Content-Type", "application/json")
	server.handleSetup(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create test owner status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response AuthMeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.User == nil {
		t.Fatalf("decode test owner response=%s err=%v", recorder.Body.String(), err)
	}
	return *response.User
}

func TestFeedbackSubmissionAtomicallyNotifiesServerOwner(t *testing.T) {
	server := newScannerTestServer(t)
	owner := createViewerFeedbackTestOwner(t, server, "feedback-owner")
	if _, err := server.db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES ('viewerFeedback', '{"enabled":true,"feedbackRetentionDays":7,"notificationRetentionDays":730}', ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/feedback", bytes.NewBufferString(`{"version":"v1","kind":"general","category":"other","message":"Please add a classics row.","context":{"deviceClass":"web","platform":"web","appVersion":"1.0.0"}}`))
	request.Header.Set("Content-Type", "application/json")
	server.handleViewerFeedback(recorder, request, user)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("feedback status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var feedbackID string
	if err := server.db.QueryRow(`SELECT id FROM viewer_feedback WHERE account_id = ?`, account.ID).Scan(&feedbackID); err != nil {
		t.Fatal(err)
	}
	var notificationFeedbackID, audience, notificationExpires, feedbackExpires string
	if err := server.db.QueryRow(`SELECT notification.source_feedback_id, notification.audience, notification.expires_at, feedback.expires_at FROM viewer_notifications notification JOIN viewer_feedback feedback ON feedback.id = notification.source_feedback_id WHERE notification.account_id = ? AND notification.kind = 'feedback.received'`, owner.ID).Scan(&notificationFeedbackID, &audience, &notificationExpires, &feedbackExpires); err != nil {
		t.Fatal(err)
	}
	if notificationFeedbackID != feedbackID || audience != "account-admin" || notificationExpires != feedbackExpires {
		t.Fatalf("owner notification feedback=%q audience=%q notificationExpires=%q feedbackExpires=%q", notificationFeedbackID, audience, notificationExpires, feedbackExpires)
	}
	updated, err := server.updateOwnerFeedbackContext(context.Background(), owner, feedbackID, 0, nil, "I added it.", server.viewerFeedbackPolicyContext(context.Background()))
	if err != nil || updated.Status != "resolved" || updated.OwnerResponse == nil {
		t.Fatalf("owner response record=%#v err=%v", updated, err)
	}
	serverID, _ := server.profileDirectoryServerIDContext(context.Background())
	viewerRecipient := notificationRecipient{Authority: "local", AccountID: account.ID, ServerID: serverID, Audience: "profile", ProfileID: account.ID}
	page, err := server.listViewerNotificationsContext(context.Background(), viewerRecipient, 25, "", "", false)
	if err != nil || len(page.Items) != 1 || page.Items[0].Kind != "feedback.updated" || page.Items[0].Content == nil || page.Items[0].Content.Body != "I added it." {
		t.Fatalf("private owner response page=%#v err=%v", page, err)
	}
}

func TestOwnerFeedbackHidesHostedSubprofileIdentity(t *testing.T) {
	server := newScannerTestServer(t)
	account, child := createProfileProtocolAccount(t, server)
	if _, err := server.db.Exec(`UPDATE users SET auth_origin = 'portico', portico_membership_id = 'membership-private-1' WHERE id = ?`, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE profiles SET origin = 'hosted', external_profile_id = 'cloud-child-private' WHERE id = ?`, child.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		INSERT INTO viewer_feedback (
			id, authority, account_id, profile_id, kind, category, message, device_class, platform, app_version,
			diagnostics_json, duplicate_hash, status, created_at, updated_at, expires_at
		) VALUES ('feedback-private-hosted', 'hosted', ?, ?, 'general', 'other', 'A private request', 'web', 'web', '1.0', ?, 'private-hash', 'new', ?, ?, ?)`,
		account.ID, child.ID, `{"deviceClass":"web","platform":"web","appVersion":"1.0","occurredAt":"`+now+`"}`, now, now, time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	record, err := server.ownerFeedbackRecordContext(context.Background(), "feedback-private-hosted")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(record)
	if record.Reporter.MembershipID != "membership-private-1" || record.Reporter.AccountID != "" || strings.Contains(string(encoded), child.ID) || strings.Contains(string(encoded), "Child") || strings.Contains(string(encoded), "cloud-child-private") {
		t.Fatalf("owner feedback leaked hosted subprofile: %s", encoded)
	}
	directory, err := server.profileDirectoryContext(context.Background(), account.ID)
	if err != nil || directory.Authority != "hosted" {
		t.Fatalf("hosted account directory authority=%q err=%v", directory.Authority, err)
	}
	managed, err := server.managedProfileDirectoryContext(context.Background(), explicitPrimaryUser(account))
	if err != nil || managed.CanManage {
		t.Fatalf("hosted account advertised local management=%v err=%v", managed.CanManage, err)
	}
}

func TestDuplicateFeedbackAttemptsAreAtomicallyRateLimited(t *testing.T) {
	server := newScannerTestServer(t)
	createViewerFeedbackTestOwner(t, server, "rate-owner")
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	body := `{"version":"v1","kind":"general","category":"other","message":"Same request","context":{"deviceClass":"web","platform":"web","appVersion":"1.0.0"}}`
	for attempt := 0; attempt < 11; attempt++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/feedback", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		server.handleViewerFeedback(recorder, request, user)
		want := http.StatusOK
		if attempt == 0 {
			want = http.StatusCreated
		} else if attempt == 10 {
			want = http.StatusTooManyRequests
		}
		if recorder.Code != want {
			t.Fatalf("attempt %d status=%d want=%d body=%s", attempt+1, recorder.Code, want, recorder.Body.String())
		}
	}
	var duplicateCount int
	if err := server.db.QueryRow(`SELECT duplicate_count FROM viewer_feedback WHERE account_id = ?`, account.ID).Scan(&duplicateCount); err != nil || duplicateCount != 9 {
		t.Fatalf("duplicate count=%d err=%v", duplicateCount, err)
	}
}

func TestDuplicateFeedbackUpdateFailureRollsBackAndReturnsError(t *testing.T) {
	server := newScannerTestServer(t)
	createViewerFeedbackTestOwner(t, server, "rollback-owner")
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	body := `{"version":"v1","kind":"general","category":"other","message":"Same rollback request","context":{"deviceClass":"web","platform":"web","appVersion":"1.0.0"}}`
	submit := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/feedback", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		server.handleViewerFeedback(recorder, request, user)
		return recorder
	}
	if recorder := submit(); recorder.Code != http.StatusCreated {
		t.Fatalf("initial feedback status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := server.db.Exec(`CREATE TRIGGER fail_duplicate_feedback_update BEFORE UPDATE OF duplicate_count ON viewer_feedback BEGIN SELECT RAISE(ABORT, 'forced duplicate update failure'); END`); err != nil {
		t.Fatal(err)
	}
	if recorder := submit(); recorder.Code != http.StatusInternalServerError {
		t.Fatalf("duplicate failure status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var duplicateCount, feedbackCount int
	if err := server.db.QueryRow(`SELECT duplicate_count FROM viewer_feedback WHERE account_id = ?`, account.ID).Scan(&duplicateCount); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM viewer_feedback WHERE account_id = ?`, account.ID).Scan(&feedbackCount); err != nil {
		t.Fatal(err)
	}
	if duplicateCount != 0 || feedbackCount != 1 {
		t.Fatalf("failed duplicate mutated state duplicateCount=%d feedbackCount=%d", duplicateCount, feedbackCount)
	}
}

func TestNotificationActionAllowlistAndRetentionRevision(t *testing.T) {
	server := newScannerTestServer(t)
	account, _ := createProfileProtocolAccount(t, server)
	serverID, _ := server.profileDirectoryServerIDContext(context.Background())
	recipient := notificationRecipient{Authority: "local", AccountID: account.ID, ServerID: serverID, Audience: "profile", ProfileID: account.ID}
	err := server.withUserTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
		return createViewerNotificationTx(tx, recipient, "server.message", "informational", "notification.server-message", "status.notification", map[string]string{}, &viewerNotificationContent{Body: "Unsafe"}, []viewerNotificationAction{{ID: "unsafe", LabelMessageID: "action.open", Kind: "navigate", Target: "https://example.test", Parameters: map[string]string{}}}, "", time.Now().Add(time.Hour))
	})
	if err == nil {
		t.Fatal("server accepted a URL notification action")
	}
	err = server.withUserTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
		return createViewerNotificationTx(tx, recipient, "server.message", "informational", "notification.server-message", "status.notification", map[string]string{}, &viewerNotificationContent{Body: "Expired"}, []viewerNotificationAction{}, "", time.Now().Add(-time.Minute))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.pruneViewerCommunicationContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	unread, revision, err := notificationCountsContext(server, context.Background(), recipient)
	if err != nil || unread != 0 || revision != 2 {
		t.Fatalf("retention invalidation unread=%d revision=%d err=%v", unread, revision, err)
	}
}

func TestOwnerNoticeRejectsMixedRecipientIdentity(t *testing.T) {
	server := newScannerTestServer(t)
	account, child := createProfileProtocolAccount(t, server)
	owner := explicitPrimaryUser(account)
	owner.Role, owner.AuthProvider = "owner", "local"
	owner.Permissions = ownerPermissions()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/viewer-notifications", bytes.NewBufferString(`{"audience":"profile","accountId":"`+account.ID+`","profileId":"`+child.ID+`","message":"Maintenance tonight."}`))
	request.Header.Set("Content-Type", "application/json")
	server.handleAdminViewerNotices(recorder, request, owner)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("mixed notice recipient status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM viewer_notifications`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("mixed notice recipient created notifications=%d err=%v", count, err)
	}
}

func TestOwnerNotificationRecipientDirectoryIncludesLocalProfilesAndHidesHostedSubprofiles(t *testing.T) {
	server := newScannerTestServer(t)
	localAccount, localChild := createProfileProtocolAccount(t, server)
	hostedAccount, err := server.createUser(UserRequest{
		Username: "hosted-recipient", Email: "hosted-recipient@example.test", DisplayName: "Hosted Household",
		Password: "Hosted-recipient-password", Role: "user", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create hosted account fixture: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), hostedAccount.ID, hostedAccount.ID, "2468"); err != nil {
		t.Fatalf("set hosted fixture primary PIN: %v", err)
	}
	hostedChild, err := server.createLocalProfileContext(context.Background(), hostedAccount.ID, CreateLocalProfileInput{
		DisplayName: "Hosted Child", Restrictions: defaultProfileRestrictions(),
	})
	if err != nil {
		t.Fatalf("create hosted child fixture: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE users SET auth_origin = 'portico', portico_membership_id = 'membership-recipient' WHERE id = ?`, hostedAccount.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE profiles SET origin = 'hosted' WHERE account_id = ?`, hostedAccount.ID); err != nil {
		t.Fatal(err)
	}

	directory, err := server.ownerNotificationRecipientDirectoryContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	profiles := map[string]ownerNotificationProfileRecipient{}
	for _, profile := range directory.Profiles {
		profiles[profile.ProfileID] = profile
	}
	if child, ok := profiles[localChild.ID]; !ok || child.Authority != "local" || child.AccountID != localAccount.ID || child.ProfileName != "Child" {
		t.Fatalf("local child recipient missing or invalid: %#v", profiles)
	}
	if _, exposed := profiles[hostedChild.ID]; exposed {
		t.Fatalf("hosted subprofile leaked into owner directory: %#v", profiles[hostedChild.ID])
	}
	admins := map[string]ownerNotificationAccountAdminRecipient{}
	for _, account := range directory.AccountAdmins {
		admins[account.AccountID] = account
	}
	if admin, ok := admins[hostedAccount.ID]; !ok || admin.Authority != "hosted" || admin.AccountName != "Hosted Household" {
		t.Fatalf("hosted main account identity missing or invalid: %#v", admins)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/viewer-notification-recipients", nil)
	server.handleAdminViewerNotificationRecipients(recorder, request, explicitPrimaryUser(localAccount))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("ordinary viewer recipient directory status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	owner := explicitPrimaryUser(localAccount)
	owner.Role, owner.AuthProvider = "owner", "local"
	owner.Permissions = ownerPermissions()
	recorder = httptest.NewRecorder()
	server.handleAdminViewerNotificationRecipients(recorder, request, owner)
	if recorder.Code != http.StatusOK {
		t.Fatalf("owner recipient directory status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/admin/viewer-notifications", bytes.NewBufferString(`{"audience":"profile","profileId":"`+hostedChild.ID+`","message":"Private maintenance notice."}`))
	request.Header.Set("Content-Type", "application/json")
	server.handleAdminViewerNotices(recorder, request, owner)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("guessed hosted subprofile notice status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var notificationCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM viewer_notifications`).Scan(&notificationCount); err != nil || notificationCount != 0 {
		t.Fatalf("guessed hosted subprofile created notifications=%d err=%v", notificationCount, err)
	}
}
