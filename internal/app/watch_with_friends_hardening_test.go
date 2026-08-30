package app

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"
)

func TestWatchWithFriendsAccountCannotBypassChildProfilePolicy(t *testing.T) {
	_, _, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{
		Username: "wwf-admin-parent", Email: "wwf-admin-parent@example.test", DisplayName: "Admin Parent",
		Password: "Password1234", Role: "user", Permissions: map[string]bool{
			"playMedia": true, "watchWithFriends": true,
		},
	})
	if err != nil {
		t.Fatalf("create admin parent: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, account.ID, "2468"); err != nil {
		t.Fatalf("set primary PIN: %v", err)
	}
	restrictions := defaultProfileRestrictions()
	restrictions.AllowWatchWithFriends = false
	child, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{
		DisplayName: "Restricted Child", PIN: "1357", Restrictions: restrictions,
	})
	if err != nil {
		t.Fatalf("create restricted child: %v", err)
	}
	if !server.watchWithFriendsProfilePermissionAllowedContext(context.Background(), account.ID) {
		t.Fatal("primary profile unexpectedly lost its explicit Watch With Friends permission")
	}
	if server.watchWithFriendsProfilePermissionAllowedContext(context.Background(), child.ID) {
		t.Fatal("administrative account authority bypassed the child profile Watch With Friends restriction")
	}
	principal, err := server.resolveRequestPrincipalContext(context.Background(), account.ID, child.ID)
	if err != nil {
		t.Fatalf("resolve child principal: %v", err)
	}
	if principal.Permissions["watchWithFriends"] || principal.Permissions["manageServer"] {
		t.Fatalf("restricted child received an expanded effective permission envelope: %#v", principal.Permissions)
	}
}

func TestWatchWithFriendsPrimaryAdminRequiresExplicitViewerPermissionAndCreatesNoOrphans(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	permissions := map[string]bool{
		"manageServer": true,
		"playMedia":    true,
		// Deliberately false: administrative authority is not viewer authority.
		"watchWithFriends": false,
	}
	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		t.Fatalf("marshal permissions: %v", err)
	}
	var accountID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&accountID); err != nil {
		t.Fatalf("load admin account: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET permissions_json = ? WHERE id = ?`, string(permissionsJSON), accountID); err != nil {
		t.Fatalf("restrict primary admin: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/watch-with-friends/groups", WatchWithFriendsCreateRequest{MediaID: "movie_meridian"}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("primary admin create status=%d body=%s want=%d", status, body, http.StatusForbidden)
	}

	// Exercise the transaction boundary with a deliberately stale caller
	// envelope. The database's current effective profile policy must win before
	// the first group, member, or queue row is inserted.
	stale := User{
		ID: accountID, AccountID: accountID, ProfileID: accountID,
		Username: "admin", Role: "owner", AuthProvider: "local",
		Permissions: map[string]bool{"manageServer": true, "playMedia": true, "watchWithFriends": true},
	}
	if _, err := server.createWatchWithFriendsGroupContext(context.Background(), stale, WatchWithFriendsCreateRequest{MediaID: "movie_meridian"}); !errors.Is(err, errWatchWithFriendsPermissionRequired) {
		t.Fatalf("stale administrative caller error=%v want explicit viewer permission denial", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin authority transaction: %v", err)
	}
	if watchWithFriendsPermissionAllowedTx(tx, stale) {
		_ = tx.Rollback()
		t.Fatal("transaction authority expanded manageServer into watchWithFriends")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback authority transaction: %v", err)
	}

	for _, table := range []string{"watch_with_friends_groups", "watch_with_friends_members", "watch_with_friends_queue"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("unauthorized create left %d orphan rows in %s", count, table)
		}
	}
}

func TestWatchWithFriendsHistoricalIdempotencyAndEndedSnapshot(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var group WatchWithFriendsGroup
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/watch-with-friends/groups", WatchWithFriendsCreateRequest{MediaID: "movie_meridian"}, &group)
	if status != http.StatusCreated {
		t.Fatalf("create group status=%d body=%s", status, body)
	}

	firstRevision := group.Revision
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", map[string]any{
		"action": "seek", "positionSeconds": 12, "expectedRevision": firstRevision, "idempotencyKey": "historical-000",
	}, &group)
	if status != http.StatusOK || group.Revision != firstRevision+1 {
		t.Fatalf("first command status=%d body=%s group=%#v", status, body, group)
	}
	var mismatch map[string]any
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", map[string]any{
		"action": "pause", "positionSeconds": 12, "expectedRevision": firstRevision, "idempotencyKey": "historical-000",
	}, &mismatch)
	if status != http.StatusBadRequest {
		t.Fatalf("idempotency key reuse with different request status=%d body=%s", status, body)
	}

	var recentExpectedRevision int64
	for index := 1; index <= maxWatchWithFriendsCommandReceipts+2; index++ {
		expected := group.Revision
		if index == maxWatchWithFriendsCommandReceipts+2 {
			recentExpectedRevision = expected
		}
		status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", map[string]any{
			"action": "seek", "positionSeconds": index + 12, "expectedRevision": expected, "idempotencyKey": fmt.Sprintf("historical-%03d", index),
		}, &group)
		if status != http.StatusOK || group.Revision != expected+1 {
			t.Fatalf("command %d status=%d body=%s group=%#v", index, status, body, group)
		}
	}

	var receiptCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM watch_with_friends_command_receipts WHERE group_id = ?`, group.ID).Scan(&receiptCount); err != nil {
		t.Fatalf("count receipts: %v", err)
	}
	if receiptCount != maxWatchWithFriendsCommandReceipts {
		t.Fatalf("receipt count=%d want=%d", receiptCount, maxWatchWithFriendsCommandReceipts)
	}

	beforeRetry := group.Revision
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", map[string]any{
		"action": "seek", "positionSeconds": maxWatchWithFriendsCommandReceipts + 14, "expectedRevision": recentExpectedRevision, "idempotencyKey": fmt.Sprintf("historical-%03d", maxWatchWithFriendsCommandReceipts+2),
	}, &group)
	if status != http.StatusOK || group.Revision != beforeRetry {
		t.Fatalf("historical retry status=%d body=%s revision=%d want=%d", status, body, group.Revision, beforeRetry)
	}

	beforeStopRevision := group.Revision
	beforeStopPlaybackRevision := group.PlaybackRevision
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", map[string]any{
		"action": "stop", "positionSeconds": group.PositionSeconds, "expectedRevision": beforeStopRevision, "idempotencyKey": "stop-once",
	}, &group)
	if status != http.StatusOK || group.State != "stopped" || group.Revision != beforeStopRevision+1 || group.PlaybackRevision != beforeStopPlaybackRevision+1 {
		t.Fatalf("stop snapshot status=%d body=%s group=%#v", status, body, group)
	}
	endedRevision := group.Revision

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", map[string]any{
		"action": "stop", "positionSeconds": group.PositionSeconds, "expectedRevision": beforeStopRevision, "idempotencyKey": "stop-once",
	}, &group)
	if status != http.StatusOK || group.State != "stopped" || group.Revision != endedRevision {
		t.Fatalf("duplicate stop status=%d body=%s group=%#v", status, body, group)
	}
}

func TestWatchWithFriendsHostMutationsRequireIdempotencyAndReplayCurrentSnapshot(t *testing.T) {
	serverURL, _ := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var group WatchWithFriendsGroup
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/watch-with-friends/groups", WatchWithFriendsCreateRequest{MediaID: "movie_meridian"}, &group)
	if status != http.StatusCreated {
		t.Fatalf("create group status=%d body=%s", status, body)
	}

	missingKeyCases := []struct {
		name    string
		method  string
		path    string
		request any
	}{
		{name: "state", method: http.MethodPatch, path: "/state", request: map[string]any{"action": "play", "expectedRevision": group.Revision}},
		{name: "settings", method: http.MethodPatch, path: "/settings", request: map[string]any{"repeatMode": "all", "expectedRevision": group.Revision}},
		{name: "queue add", method: http.MethodPost, path: "/queue", request: map[string]any{"mediaId": "movie_saffron", "expectedRevision": group.Revision}},
		{name: "queue reorder", method: http.MethodPatch, path: "/queue", request: map[string]any{"mediaIds": []string{"movie_meridian"}, "expectedRevision": group.Revision}},
		{name: "queue remove", method: http.MethodDelete, path: fmt.Sprintf("/queue/movie_saffron?expectedRevision=%d", group.Revision)},
		{name: "end", method: http.MethodDelete, path: fmt.Sprintf("?expectedRevision=%d", group.Revision)},
	}
	for _, testCase := range missingKeyCases {
		t.Run("missing key "+testCase.name, func(t *testing.T) {
			status, responseBody := doJSON(t, client, testCase.method, serverURL+"/api/watch-with-friends/groups/"+group.ID+testCase.path, testCase.request, nil)
			if status != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", status, responseBody)
			}
		})
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", map[string]any{
		"action": "play", "expectedRevision": group.Revision, "idempotencyKey": strings.Repeat("x", 121),
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("overlong key status=%d body=%s", status, body)
	}

	settingsExpected := group.Revision
	settingsRequest := map[string]any{"repeatMode": "all", "expectedRevision": settingsExpected, "idempotencyKey": "settings-current-snapshot"}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/settings", settingsRequest, &group)
	if status != http.StatusOK || group.RepeatMode != "all" || group.Revision != settingsExpected+1 {
		t.Fatalf("settings status=%d body=%s group=%#v", status, body, group)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", map[string]any{
		"action": "seek", "positionSeconds": 24, "expectedRevision": group.Revision, "idempotencyKey": "after-settings",
	}, &group)
	if status != http.StatusOK {
		t.Fatalf("intervening state status=%d body=%s", status, body)
	}
	currentRevision := group.Revision
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/settings", settingsRequest, &group)
	if status != http.StatusOK || group.Revision != currentRevision || group.PositionSeconds != 24 {
		t.Fatalf("settings replay status=%d body=%s group=%#v wantRevision=%d", status, body, group, currentRevision)
	}
	status, _ = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/settings", map[string]any{
		"repeatMode": "one", "expectedRevision": settingsExpected, "idempotencyKey": "settings-current-snapshot",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("settings fingerprint mismatch status=%d", status)
	}

	addExpected := group.Revision
	addRequest := map[string]any{"mediaId": "movie_saffron", "expectedRevision": addExpected, "idempotencyKey": "queue-add-current-snapshot"}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", addRequest, &group)
	if status != http.StatusOK || len(group.Queue) != 2 {
		t.Fatalf("queue add status=%d body=%s queue=%#v", status, body, group.Queue)
	}
	interveningExpected := group.Revision
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/settings", map[string]any{
		"shuffleEnabled": true, "expectedRevision": interveningExpected, "idempotencyKey": "after-queue-add",
	}, &group)
	if status != http.StatusOK {
		t.Fatalf("intervening settings status=%d body=%s", status, body)
	}
	currentRevision = group.Revision
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", addRequest, &group)
	if status != http.StatusOK || group.Revision != currentRevision || len(group.Queue) != 2 || !group.ShuffleEnabled {
		t.Fatalf("queue add replay status=%d body=%s group=%#v", status, body, group)
	}
	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", map[string]any{
		"mediaId": "movie_neon", "expectedRevision": addExpected, "idempotencyKey": "queue-add-current-snapshot",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("queue add fingerprint mismatch status=%d", status)
	}

	reorderExpected := group.Revision
	reorderRequest := map[string]any{
		"mediaIds": []string{"movie_saffron", "movie_meridian"}, "expectedRevision": reorderExpected, "idempotencyKey": "queue-reorder-current-snapshot",
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", reorderRequest, &group)
	if status != http.StatusOK || group.Queue[0].MediaID != "movie_saffron" {
		t.Fatalf("queue reorder status=%d body=%s queue=%#v", status, body, group.Queue)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", map[string]any{
		"action": "seek", "positionSeconds": 27, "expectedRevision": group.Revision, "idempotencyKey": "after-queue-reorder",
	}, &group)
	if status != http.StatusOK {
		t.Fatalf("intervening reorder state status=%d body=%s", status, body)
	}
	currentRevision = group.Revision
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", reorderRequest, &group)
	if status != http.StatusOK || group.Revision != currentRevision || group.Queue[0].MediaID != "movie_saffron" || group.PositionSeconds != 27 {
		t.Fatalf("queue reorder replay status=%d body=%s group=%#v", status, body, group)
	}

	removeExpected := group.Revision
	removeURL := fmt.Sprintf("%s/api/watch-with-friends/groups/%s/queue/movie_saffron?expectedRevision=%d&idempotencyKey=queue-remove-current-snapshot", serverURL, group.ID, removeExpected)
	status, body = doJSON(t, client, http.MethodDelete, removeURL, nil, &group)
	if status != http.StatusOK || len(group.Queue) != 1 {
		t.Fatalf("queue remove status=%d body=%s queue=%#v", status, body, group.Queue)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", map[string]any{
		"action": "pause", "positionSeconds": 31, "expectedRevision": group.Revision, "idempotencyKey": "after-queue-remove",
	}, &group)
	if status != http.StatusOK {
		t.Fatalf("intervening pause status=%d body=%s", status, body)
	}
	currentRevision = group.Revision
	status, body = doJSON(t, client, http.MethodDelete, removeURL, nil, &group)
	if status != http.StatusOK || group.Revision != currentRevision || len(group.Queue) != 1 || group.PositionSeconds != 31 {
		t.Fatalf("queue remove replay status=%d body=%s group=%#v", status, body, group)
	}

	endExpected := group.Revision
	endURL := fmt.Sprintf("%s/api/watch-with-friends/groups/%s?expectedRevision=%d&idempotencyKey=end-current-snapshot", serverURL, group.ID, endExpected)
	status, body = doJSON(t, client, http.MethodDelete, endURL, nil, &group)
	if status != http.StatusOK || group.State != "stopped" || group.Revision != endExpected+1 {
		t.Fatalf("end status=%d body=%s group=%#v", status, body, group)
	}
	endedRevision := group.Revision
	status, body = doJSON(t, client, http.MethodDelete, endURL, nil, &group)
	if status != http.StatusOK || group.State != "stopped" || group.Revision != endedRevision {
		t.Fatalf("end replay status=%d body=%s group=%#v", status, body, group)
	}
	status, _ = doJSON(t, client, http.MethodDelete, fmt.Sprintf("%s/api/watch-with-friends/groups/%s?expectedRevision=%d&idempotencyKey=end-current-snapshot", serverURL, group.ID, endedRevision), nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("end fingerprint mismatch status=%d", status)
	}
}

func TestWatchWithFriendsRevokedAndStaleMembersCannotDeadlockGroup(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	hostJar, _ := cookiejar.New(nil)
	hostClient := &http.Client{Jar: hostJar}
	loginUser(t, hostClient, serverURL)

	permissions := permissionsForRole("user")
	permissions["watchWithFriends"] = true
	var guest User
	status, body := doJSON(t, hostClient, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username: "wwf-revoked", Email: "wwf-revoked@example.test", DisplayName: "Revoked Guest", Password: "Password1234",
		Role: "user", Permissions: permissions, LibraryIDs: []string{"lib_movies"},
	}, &guest)
	if status != http.StatusCreated {
		t.Fatalf("create guest status=%d body=%s", status, body)
	}
	guestJar, _ := cookiejar.New(nil)
	guestClient := &http.Client{Jar: guestJar}
	status, body = doJSON(t, guestClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": guest.Username, "password": "Password1234"}, nil)
	if status != http.StatusOK {
		t.Fatalf("login guest status=%d body=%s", status, body)
	}

	var inaccessibleQueueGroup WatchWithFriendsGroup
	status, body = doJSON(t, hostClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups", WatchWithFriendsCreateRequest{MediaID: "movie_meridian"}, &inaccessibleQueueGroup)
	if status != http.StatusCreated {
		t.Fatalf("create group status=%d body=%s", status, body)
	}
	status, body = doJSON(t, hostClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+inaccessibleQueueGroup.ID+"/queue", map[string]any{
		"mediaId": "track_mara_01", "expectedRevision": inaccessibleQueueGroup.Revision, "idempotencyKey": "inaccessible-add",
	}, &inaccessibleQueueGroup)
	if status != http.StatusOK {
		t.Fatalf("queue inaccessible item status=%d body=%s", status, body)
	}
	status, _ = doJSON(t, guestClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+inaccessibleQueueGroup.ID+"/join", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("join with inaccessible retained queue status=%d want=%d", status, http.StatusNotFound)
	}
	status, body = doJSON(t, hostClient, http.MethodDelete, fmt.Sprintf("%s/api/watch-with-friends/groups/%s?expectedRevision=%d&idempotencyKey=inaccessible-end", serverURL, inaccessibleQueueGroup.ID, inaccessibleQueueGroup.Revision), nil, &inaccessibleQueueGroup)
	if status != http.StatusOK || inaccessibleQueueGroup.State != "stopped" {
		t.Fatalf("end inaccessible group status=%d body=%s group=%#v", status, body, inaccessibleQueueGroup)
	}

	var group WatchWithFriendsGroup
	status, body = doJSON(t, hostClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups", WatchWithFriendsCreateRequest{MediaID: "movie_meridian"}, &group)
	if status != http.StatusCreated {
		t.Fatalf("create active group status=%d body=%s", status, body)
	}
	status, body = doJSON(t, guestClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/join", nil, &group)
	if status != http.StatusOK || len(group.Members) != 2 || group.ReconnectGeneration < 1 {
		t.Fatalf("join status=%d body=%s group=%#v", status, body, group)
	}
	streamRequest, err := http.NewRequest(http.MethodGet, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/events", nil)
	if err != nil {
		t.Fatalf("create guest group stream request: %v", err)
	}
	streamResponse, err := guestClient.Do(streamRequest)
	if err != nil {
		t.Fatalf("open guest group stream: %v", err)
	}
	defer streamResponse.Body.Close()
	if streamResponse.StatusCode != http.StatusOK {
		t.Fatalf("guest group stream status=%d", streamResponse.StatusCode)
	}
	streamReader := bufio.NewReader(streamResponse.Body)
	if eventName, readErr := readSSEEventName(streamReader); readErr != nil || eventName != "group" {
		t.Fatalf("initial guest stream event=%q err=%v", eventName, readErr)
	}
	reconnectBeforeRevocation := group.ReconnectGeneration
	staleRevision := group.Revision - 1
	status, _ = doJSON(t, hostClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/settings", map[string]any{
		"repeatMode": "all", "expectedRevision": staleRevision, "idempotencyKey": "stale-settings",
	}, nil)
	if status != http.StatusConflict {
		t.Fatalf("stale settings status=%d want=%d", status, http.StatusConflict)
	}
	status, _ = doJSON(t, hostClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", map[string]any{
		"mediaId": "movie_saffron", "expectedRevision": staleRevision, "idempotencyKey": "stale-add",
	}, nil)
	if status != http.StatusConflict {
		t.Fatalf("stale queue add status=%d want=%d", status, http.StatusConflict)
	}

	if _, err := server.execUserWrite(context.Background(), `UPDATE users SET permissions_json = '{}' WHERE id = ?`, guest.ID); err != nil {
		t.Fatalf("revoke guest permission: %v", err)
	}
	if server.watchWithFriendsProfilePermissionAllowedContext(context.Background(), viewerProfileID(guest)) {
		t.Fatal("revoked guest retained Watch With Friends permission")
	}
	streamClosed := make(chan error, 1)
	go func() {
		_, readErr := streamReader.ReadString('\n')
		for readErr == nil {
			_, readErr = streamReader.ReadString('\n')
		}
		streamClosed <- readErr
	}()
	select {
	case <-streamClosed:
	case <-time.After(2*watchWithFriendsAuthorizationRecheck + time.Second):
		t.Fatal("revoked Watch With Friends stream remained open")
	}
	status, body = doJSON(t, hostClient, http.MethodGet, serverURL+"/api/watch-with-friends/groups/"+group.ID, nil, &group)
	if status != http.StatusOK || len(group.Members) != 1 || group.ReconnectGeneration <= reconnectBeforeRevocation {
		t.Fatalf("reconcile revoked member status=%d body=%s group=%#v", status, body, group)
	}

	expected := group.Revision
	status, body = doJSON(t, hostClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", map[string]any{
		"mediaId": "movie_saffron", "expectedRevision": expected, "idempotencyKey": "post-eviction-add",
	}, &group)
	if status != http.StatusOK || group.Revision != expected+1 {
		t.Fatalf("host mutation after eviction status=%d body=%s group=%#v", status, body, group)
	}
	status, _ = doJSON(t, hostClient, http.MethodDelete, fmt.Sprintf("%s/api/watch-with-friends/groups/%s/queue/movie_saffron?expectedRevision=%d&idempotencyKey=stale-remove", serverURL, group.ID, expected), nil, nil)
	if status != http.StatusConflict {
		t.Fatalf("stale queue remove status=%d want=%d", status, http.StatusConflict)
	}

	if _, err := server.execUserWrite(context.Background(), `UPDATE users SET permissions_json = ? WHERE id = ?`, mustWatchWithFriendsJSONForTest(t, permissions), guest.ID); err != nil {
		t.Fatalf("restore guest permission: %v", err)
	}
	status, body = doJSON(t, guestClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/join", nil, &group)
	if status != http.StatusOK {
		t.Fatalf("guest rejoin status=%d body=%s", status, body)
	}
	stale := time.Now().UTC().Add(-watchWithFriendsMemberStaleAfter - time.Minute).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE watch_with_friends_members SET last_seen_at = ? WHERE group_id = ? AND profile_id = ?`, stale, group.ID, group.OwnerProfileID); err != nil {
		t.Fatalf("stale host: %v", err)
	}
	status, _ = doJSON(t, guestClient, http.MethodGet, serverURL+"/api/watch-with-friends/groups/"+group.ID, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("stale host group status=%d want=%d", status, http.StatusNotFound)
	}
	var state, endedAt string
	var revision, playbackRevision, reconnectGeneration int64
	if err := db.QueryRow(`SELECT state, ended_at, revision, playback_revision, reconnect_generation FROM watch_with_friends_groups WHERE id = ?`, group.ID).Scan(&state, &endedAt, &revision, &playbackRevision, &reconnectGeneration); err != nil && err != sql.ErrNoRows {
		t.Fatalf("load ended group: %v", err)
	}
	if state != "stopped" || endedAt == "" || revision <= group.Revision || playbackRevision <= group.PlaybackRevision || reconnectGeneration <= group.ReconnectGeneration {
		t.Fatalf("stale host did not end with committed generations state=%s ended=%q rev=%d playback=%d reconnect=%d", state, endedAt, revision, playbackRevision, reconnectGeneration)
	}
}

func mustWatchWithFriendsJSONForTest(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal test value: %v", err)
	}
	return string(encoded)
}
