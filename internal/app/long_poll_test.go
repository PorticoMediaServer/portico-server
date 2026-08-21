package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLongPollCursorIsIntegrityProtectedScopedAndRestartBound(t *testing.T) {
	runtime := newLongPollRuntime()
	scope := runtime.digest("scope", "profile-a", "resource-a")
	cursor, err := runtime.signCursor(longPollCursorClaims{Kind: "playback-command", Scope: scope, Position: 41, Marker: runtime.digest("command-secret")})
	if err != nil {
		t.Fatal(err)
	}
	claims, reset, err := runtime.parseCursor(cursor, "playback-command", scope)
	if err != nil || reset || claims.Position != 41 {
		t.Fatalf("valid cursor claims=%+v reset=%v err=%v", claims, reset, err)
	}
	if _, _, err := runtime.parseCursor(cursor, "playback-command", runtime.digest("scope", "profile-b", "resource-a")); err == nil {
		t.Fatal("cursor crossed profile scope")
	}
	if _, _, err := runtime.parseCursor(cursor, "playback-receiver", scope); err == nil {
		t.Fatal("cursor crossed stream kind")
	}
	tamperAt := strings.IndexByte(cursor, '.') + 2
	replacement := byte('A')
	if cursor[tamperAt] == replacement {
		replacement = 'B'
	}
	tampered := cursor[:tamperAt] + string(replacement) + cursor[tamperAt+1:]
	if _, _, err := runtime.parseCursor(tampered, "playback-command", scope); err == nil {
		t.Fatal("tampered cursor was accepted")
	}

	restarted := newLongPollRuntime()
	restarted.key = append([]byte(nil), runtime.key...)
	restarted.bootID = "a-different-boot"
	if _, reset, err := restarted.parseCursor(cursor, "playback-command", scope); err != nil || !reset {
		t.Fatalf("restart cursor reset=%v err=%v", reset, err)
	}

	expiredClaims := longPollCursorClaims{Version: "v1", BootID: runtime.bootID, Kind: "playback-command", Scope: scope, Position: 41, Expires: time.Now().Add(-time.Second).Unix()}
	payload, _ := json.Marshal(expiredClaims)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, runtime.key)
	_, _ = mac.Write([]byte(encoded))
	expired := encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if _, reset, err := runtime.parseCursor(expired, "playback-command", scope); err != nil || !reset {
		t.Fatalf("expired cursor reset=%v err=%v", reset, err)
	}
	if strings.Contains(cursor, "profile-a") || strings.Contains(cursor, "resource-a") || strings.Contains(cursor, "command-secret") {
		t.Fatal("opaque cursor exposed scope or command material")
	}
}

func TestLongPollScopesSurviveCredentialRotationButNotViewerOrDeviceChanges(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	server.longPoll = newLongPollRuntime()
	var accountID, profileID string
	if err := db.QueryRow(`SELECT id FROM users WHERE role = 'owner' LIMIT 1`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM profiles WHERE account_id = ? AND is_primary = 1 LIMIT 1`, accountID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	user := User{ID: accountID, AccountID: accountID, ProfileID: profileID, AuthProvider: "local", DeviceID: "device-a"}
	requestA := httptest.NewRequest(http.MethodGet, "/api/events/poll", nil)
	requestA.Header.Set("Authorization", "Bearer access-token-a")
	ownerA, scopeA, err := server.longPollPrincipalScope(requestA, user, "app", "", "")
	if err != nil {
		t.Fatal(err)
	}
	requestB := httptest.NewRequest(http.MethodGet, "/api/events/poll", nil)
	requestB.Header.Set("Authorization", "Bearer access-token-b")
	ownerB, scopeB, err := server.longPollPrincipalScope(requestB, user, "app", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if ownerA != ownerB || scopeA != scopeB {
		t.Fatal("ordinary access-token rotation changed the long-poll cursor scope")
	}
	otherDevice := user
	otherDevice.DeviceID = "device-b"
	if _, otherScope, err := server.longPollPrincipalScope(requestB, otherDevice, "app", "", ""); err != nil || otherScope == scopeA {
		t.Fatalf("cursor crossed a device boundary: scopeEqual=%v err=%v", otherScope == scopeA, err)
	}
	otherProfile := user
	otherProfile.ProfileID = "profile-b"
	if _, otherScope, err := server.longPollPrincipalScope(requestB, otherProfile, "app", "", ""); err != nil || otherScope == scopeA {
		t.Fatalf("cursor crossed a profile boundary: scopeEqual=%v err=%v", otherScope == scopeA, err)
	}

	recipient := notificationRecipient{Authority: "local", AccountID: accountID, ProfileID: profileID, ServerID: "server-a", Audience: "profile"}
	authA := notificationStreamAuthorization{SessionID: "session-a", TokenHash: "hash-a", DeviceID: "device-a", InstallationID: "installation-a", AuthorizationRevision: "revision-a"}
	authB := authA
	authB.SessionID = "session-b"
	authB.TokenHash = "hash-b"
	authB.AuthorizationRevision = "revision-b"
	notificationOwnerA, notificationScopeA := server.notificationLongPollPrincipalScope(recipient, authA)
	notificationOwnerB, notificationScopeB := server.notificationLongPollPrincipalScope(recipient, authB)
	if notificationOwnerA != notificationOwnerB || notificationScopeA != notificationScopeB {
		t.Fatal("notification access-token rotation changed the long-poll cursor scope")
	}
	authB.InstallationID = "installation-b"
	if _, otherScope := server.notificationLongPollPrincipalScope(recipient, authB); otherScope == notificationScopeA {
		t.Fatal("notification cursor crossed an installation boundary")
	}
}

func TestLongPollBrokerRetainsBoundedAppEventsAndDetectsOverflow(t *testing.T) {
	broker := &longPollBroker{waiters: map[string]map[chan struct{}]struct{}{}}
	signal, unsubscribe := broker.subscribe(longPollAppBrokerKey)
	defer unsubscribe()
	for index := 1; index <= longPollRetainedAppEvents+40; index++ {
		broker.publishApp(AppEvent{ID: uint64(index), Type: "data.changed", Tags: []string{"media"}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	}
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("subscriber missed broker wake-up")
	}
	if _, _, _, overflow := broker.appEventsAfter(0); !overflow {
		t.Fatal("cursor older than retention did not require reset")
	}
	broker.mu.Lock()
	floor := broker.floor
	retained := len(broker.app)
	retainedBytes := broker.appSize
	broker.mu.Unlock()
	if retained > longPollRetainedAppEvents || retainedBytes > longPollRetainedAppBytes {
		t.Fatalf("retention exceeded limits: events=%d bytes=%d", retained, retainedBytes)
	}
	events, next, hasMore, overflow := broker.appEventsAfter(floor)
	if overflow || len(events) != longPollMaximumAppEvents || !hasMore || next != events[len(events)-1].ID {
		t.Fatalf("bounded page events=%d next=%d hasMore=%v overflow=%v", len(events), next, hasMore, overflow)
	}
}

func TestLongPollConcurrencyGuardsLogicalAndOwnerLimits(t *testing.T) {
	runtime := newLongPollRuntime()
	release, status := runtime.acquire("owner-a", "logical-a")
	if release == nil || status != 0 {
		t.Fatalf("first acquire status=%d", status)
	}
	if duplicate, status := runtime.acquire("owner-a", "logical-a"); duplicate != nil || status != http.StatusConflict {
		t.Fatalf("duplicate logical poll status=%d", status)
	}
	releases := []func(){release}
	for index := 1; index < longPollMaximumPerOwner; index++ {
		next, status := runtime.acquire("owner-a", "logical-"+string(rune('a'+index)))
		if next == nil || status != 0 {
			t.Fatalf("owner slot %d status=%d", index, status)
		}
		releases = append(releases, next)
	}
	if extra, status := runtime.acquire("owner-a", "logical-extra"); extra != nil || status != http.StatusTooManyRequests {
		t.Fatalf("owner overflow status=%d", status)
	}
	for _, release := range releases {
		release()
	}
	if runtime.active != 0 || len(runtime.byOwner) != 0 || len(runtime.logical) != 0 {
		t.Fatalf("concurrency state leaked: active=%d owners=%d logical=%d", runtime.active, len(runtime.byOwner), len(runtime.logical))
	}
}

func TestParseLongPollRequestRejectsBodiesUnknownQueriesAndInvalidWaits(t *testing.T) {
	tests := []struct {
		name string
		url  string
		body string
	}{
		{name: "unknown query", url: "/poll?topic=private"},
		{name: "wait too high", url: "/poll?waitSeconds=26"},
		{name: "negative wait", url: "/poll?waitSeconds=-1"},
		{name: "non integer wait", url: "/poll?waitSeconds=soon"},
		{name: "body", url: "/poll", body: `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.url, strings.NewReader(test.body))
			recorder := httptest.NewRecorder()
			if _, ok := parseLongPollRequest(recorder, request); ok || recorder.Code != http.StatusBadRequest {
				t.Fatalf("ok=%v status=%d", ok, recorder.Code)
			}
		})
	}
	request := httptest.NewRequest(http.MethodGet, "/poll?waitSeconds=0&cursor=opaque", nil)
	recorder := httptest.NewRecorder()
	parsed, ok := parseLongPollRequest(recorder, request)
	if !ok || parsed.wait != 0 || parsed.cursor != "opaque" {
		t.Fatalf("valid request parsed=%+v ok=%v", parsed, ok)
	}
}

type longPollHTTPEnvelope struct {
	Version       string            `json:"version"`
	Cursor        string            `json:"cursor"`
	ResetRequired bool              `json:"resetRequired"`
	HasMore       bool              `json:"hasMore"`
	Events        []json.RawMessage `json:"events"`
}

func TestAppLongPollRouteReplaysSSEAppEventShapeWithoutLostWakeup(t *testing.T) {
	server := newScannerTestServer(t)
	server.longPoll = newLongPollRuntime()
	account, _ := createProfileProtocolAccount(t, server)
	const installationID = "long-poll-app-installation"
	deviceID := bindProfileTestDevice(t, server.db, server, account.ID, installationID)
	const token = "long-poll-app-token"
	now := time.Now().UTC()
	if _, err := server.db.Exec(`INSERT INTO sessions (id, user_id, profile_id, auth_provider, device_id, token_hash, expires_at, created_at, last_seen_at) VALUES (?, ?, ?, 'local', ?, ?, ?, ?, ?)`,
		"long_poll_app_session", account.ID, account.ID, deviceID, hashToken(token), now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	poll := func(cursor string) (*http.Response, longPollHTTPEnvelope) {
		url := httpServer.URL + "/api/events/poll?waitSeconds=0"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		request, _ := http.NewRequest(http.MethodGet, url, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var envelope longPollHTTPEnvelope
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode status=%d: %v", response.StatusCode, err)
		}
		return response, envelope
	}
	response, initial := poll("")
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" || initial.Version != "v1" || initial.Cursor == "" || len(initial.Events) != 0 {
		t.Fatalf("initial response status=%d cache=%q envelope=%+v", response.StatusCode, response.Header.Get("Cache-Control"), initial)
	}
	server.publishDataChanged("data.changed", []string{"media", "library-items"}, "database", "", nil)
	response, update := poll(initial.Cursor)
	if response.StatusCode != http.StatusOK || len(update.Events) != 1 || update.ResetRequired || update.HasMore {
		t.Fatalf("update response status=%d envelope=%+v", response.StatusCode, update)
	}
	var event AppEvent
	if err := json.Unmarshal(update.Events[0], &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "data.changed" || event.Resource != "database" || event.ResourceID != "" || len(event.Fields) != 0 {
		t.Fatalf("long-poll event diverged from AppEvent: %+v", event)
	}
	if strings.Contains(update.Cursor, "media-1") {
		t.Fatal("cursor exposed resource identifier")
	}
}

func TestResourceLongPollHandlersRejectInvalidCursorBeforeResourceLookup(t *testing.T) {
	server := newScannerTestServer(t)
	server.longPoll = newLongPollRuntime()
	account, _ := createProfileProtocolAccount(t, server)
	user := explicitPrimaryUser(account)
	tests := []struct {
		name   string
		handle func(http.ResponseWriter, *http.Request)
	}{
		{name: "playback command", handle: func(w http.ResponseWriter, r *http.Request) {
			server.handlePlaybackCommandEventsPoll(w, r, user, "secret-session-that-does-not-exist")
		}},
		{name: "playback receiver", handle: func(w http.ResponseWriter, r *http.Request) {
			server.handlePlaybackReceiverEventsPoll(w, r, user, "secret-receiver-that-does-not-exist")
		}},
		{name: "watch with friends", handle: func(w http.ResponseWriter, r *http.Request) {
			server.handleWatchWithFriendsGroupEventsPoll(w, r, user, "secret-group-that-does-not-exist")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/events/poll?cursor=not-a-valid-cursor&waitSeconds=0", nil)
			test.handle(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "secret-") {
				t.Fatalf("resource existence material leaked: %s", recorder.Body.String())
			}
		})
	}
}

func TestNotificationLongPollIsInteractiveScopedAndContentFree(t *testing.T) {
	server := newScannerTestServer(t)
	server.longPoll = newLongPollRuntime()
	account, _ := createProfileProtocolAccount(t, server)
	profile := explicitPrimaryUser(account)
	const installationID = "long-poll-notification-installation"
	deviceID := bindProfileTestDevice(t, server.db, server, account.ID, installationID)
	if _, err := server.db.Exec(`UPDATE devices SET app = 'portico-web', platform = 'web' WHERE id = ?`, deviceID); err != nil {
		t.Fatal(err)
	}
	const token = "long-poll-notification-token"
	now := time.Now().UTC()
	if _, err := server.db.Exec(`INSERT INTO sessions (id, user_id, profile_id, auth_provider, device_id, token_hash, expires_at, created_at, last_seen_at) VALUES (?, ?, ?, 'local', ?, ?, ?, ?, ?)`,
		"long_poll_notification_session", account.ID, account.ID, deviceID, hashToken(token), now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	serverID, err := server.profileDirectoryServerIDContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	recipient := notificationRecipient{Authority: "local", AccountID: account.ID, ServerID: serverID, Audience: "profile", ProfileID: profile.ProfileID}
	if err := server.withUserTxTagged(t.Context(), nil, func(tx *sql.Tx) error {
		return createViewerNotificationTx(tx, recipient, "server.message", "informational", "notification.server-message", "status.notification",
			map[string]string{}, &viewerNotificationContent{Title: "Private title", Body: "Private body"}, []viewerNotificationAction{}, "", time.Now().Add(time.Hour))
	}); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	poll := func(cursor string, authToken string) (*http.Response, longPollHTTPEnvelope) {
		url := httpServer.URL + "/api/notifications/events/poll?audience=profile&waitSeconds=0"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		request, _ := http.NewRequest(http.MethodGet, url, nil)
		request.Header.Set("Authorization", "Bearer "+authToken)
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer response.Body.Close()
		var envelope longPollHTTPEnvelope
		_ = json.NewDecoder(response.Body).Decode(&envelope)
		return response, envelope
	}
	response, initial := poll("", token)
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" || len(initial.Events) != 1 {
		t.Fatalf("initial notification poll status=%d envelope=%+v", response.StatusCode, initial)
	}
	var event map[string]any
	if err := json.Unmarshal(initial.Events[0], &event); err != nil {
		t.Fatal(err)
	}
	if len(event) != 3 || event["version"] != "v1" || event["kind"] != "notifications.invalidated" {
		t.Fatalf("notification invalidation shape=%#v", event)
	}
	raw := string(initial.Events[0])
	if strings.Contains(raw, "Private") || strings.Contains(raw, account.ID) || strings.Contains(raw, profile.ProfileID) {
		t.Fatalf("notification invalidation exposed private state: %s", raw)
	}

	response, empty := poll(initial.Cursor, token)
	if response.StatusCode != http.StatusOK || len(empty.Events) != 0 {
		t.Fatalf("unchanged notification poll status=%d envelope=%+v", response.StatusCode, empty)
	}
	if _, err := server.db.Exec(`DELETE FROM sessions WHERE id = ?`, "long_poll_notification_session"); err != nil {
		t.Fatal(err)
	}
	response, _ = poll(empty.Cursor, token)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked interactive session status=%d", response.StatusCode)
	}
}
