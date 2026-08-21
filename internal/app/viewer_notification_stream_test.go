package app

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type notificationStreamFixture struct {
	server    *Server
	reader    *bufio.Reader
	cancel    context.CancelFunc
	accountID string
	profileID string
	childID   string
	deviceID  string
	sessionID string
	recipient notificationRecipient
}

type notificationStreamEvent map[string]any

type notificationStreamReadResult struct {
	event notificationStreamEvent
	err   error
}

func newNotificationStreamFixture(t *testing.T) notificationStreamFixture {
	t.Helper()
	server := newScannerTestServer(t)
	account, child := createProfileProtocolAccount(t, server)
	profile := explicitPrimaryUser(account)
	const installationID = "notification-stream-installation-0001"
	deviceID := bindProfileTestDevice(t, server.db, server, account.ID, installationID)
	if _, err := server.db.Exec(`UPDATE devices SET app = 'portico-web', platform = 'web' WHERE id = ?`, deviceID); err != nil {
		t.Fatalf("mark notification stream device interactive: %v", err)
	}
	sessionID := "session_notification_stream"
	sessionToken := "notification-stream-session-token"
	now := time.Now().UTC()
	if _, err := server.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, auth_provider, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, 'local', ?, ?, ?, ?, ?)`, sessionID, account.ID, account.ID, deviceID, hashToken(sessionToken),
		now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed notification stream session: %v", err)
	}
	serverID, err := server.profileDirectoryServerIDContext(context.Background())
	if err != nil {
		t.Fatalf("notification stream server id: %v", err)
	}

	httpServer := httptest.NewServer(server.Handler())
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/api/notifications/events?audience=profile", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+sessionToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		httpServer.Close()
		t.Fatalf("open notification stream: %v", err)
	}
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		cancel()
		_ = response.Body.Close()
		httpServer.Close()
		t.Fatalf("notification stream status=%d content-type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	fixture := notificationStreamFixture{
		server: server, reader: bufio.NewReader(response.Body), cancel: cancel,
		accountID: account.ID, profileID: profile.ProfileID, childID: child.ID, deviceID: deviceID, sessionID: sessionID,
		recipient: notificationRecipient{Authority: "local", AccountID: account.ID, ServerID: serverID, Audience: "profile", ProfileID: profile.ProfileID},
	}
	t.Cleanup(func() {
		cancel()
		_ = response.Body.Close()
		httpServer.Close()
	})
	return fixture
}

func readNotificationStreamEvent(reader *bufio.Reader) (notificationStreamEvent, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return notificationStreamEvent{}, err
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event notificationStreamEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &event); err != nil {
			return notificationStreamEvent{}, err
		}
		return event, nil
	}
}

func awaitNotificationStreamRead(reader *bufio.Reader, timeout time.Duration) <-chan notificationStreamReadResult {
	result := make(chan notificationStreamReadResult, 1)
	go func() {
		event, err := readNotificationStreamEvent(reader)
		result <- notificationStreamReadResult{event: event, err: err}
	}()
	return result
}

func requireNotificationStreamEvent(t *testing.T, reader *bufio.Reader, timeout time.Duration) notificationStreamEvent {
	t.Helper()
	select {
	case result := <-awaitNotificationStreamRead(reader, timeout):
		if result.err != nil {
			t.Fatalf("read notification stream event: %v", result.err)
		}
		return result.event
	case <-time.After(timeout):
		t.Fatal("timed out waiting for notification stream event")
		return notificationStreamEvent{}
	}
}

func requireContentFreeNotificationStreamEvent(t *testing.T, event notificationStreamEvent) {
	t.Helper()
	if event["version"] != "v1" || event["kind"] != "notifications.invalidated" {
		t.Fatalf("invalid notification stream event: %#v", event)
	}
	if _, ok := event["occurredAt"].(string); !ok {
		t.Fatalf("notification stream event has no timestamp: %#v", event)
	}
	if len(event) != 3 {
		t.Fatalf("notification stream event exposed private or unknown fields: %#v", event)
	}
}

func requireNotificationStreamClosedWithoutEvent(t *testing.T, fixture notificationStreamFixture, timeout time.Duration) {
	t.Helper()
	select {
	case result := <-awaitNotificationStreamRead(fixture.reader, timeout):
		if result.err == nil {
			t.Fatalf("notification stream emitted private state after authorization loss: %#v", result.event)
		}
		if result.err != io.EOF && !strings.Contains(strings.ToLower(result.err.Error()), "closed") && !strings.Contains(strings.ToLower(result.err.Error()), "canceled") {
			t.Fatalf("notification stream closed with unexpected error: %v", result.err)
		}
	case <-time.After(timeout):
		fixture.cancel()
		t.Fatal("notification stream remained open after authorization loss")
	}
}

func createNotificationStreamInvalidation(t *testing.T, fixture notificationStreamFixture) {
	t.Helper()
	err := fixture.server.withUserTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
		return createViewerNotificationTx(tx, fixture.recipient, "server.message", "informational", "notification.server-message", "status.notification",
			map[string]string{}, &viewerNotificationContent{Body: "Private stream update"}, []viewerNotificationAction{}, "", time.Now().UTC().Add(time.Hour))
	})
	if err != nil {
		t.Fatalf("create notification invalidation: %v", err)
	}
}

func TestViewerNotificationStreamClosesWhenAuthorizationChanges(t *testing.T) {
	previousInterval := viewerNotificationStreamAuthorizationInterval
	viewerNotificationStreamAuthorizationInterval = 25 * time.Millisecond
	t.Cleanup(func() { viewerNotificationStreamAuthorizationInterval = previousInterval })

	tests := []struct {
		name   string
		mutate func(*testing.T, notificationStreamFixture)
	}{
		{name: "session expires", mutate: func(t *testing.T, fixture notificationStreamFixture) {
			_, err := fixture.server.db.Exec(`UPDATE sessions SET expires_at = ? WHERE id = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), fixture.sessionID)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "session is logged out or revoked", mutate: func(t *testing.T, fixture notificationStreamFixture) {
			if _, err := fixture.server.db.Exec(`DELETE FROM sessions WHERE id = ?`, fixture.sessionID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "device is revoked", mutate: func(t *testing.T, fixture notificationStreamFixture) {
			_, err := fixture.server.db.Exec(`UPDATE devices SET trusted = 0, revoked_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), fixture.deviceID)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "profile policy revision changes", mutate: func(t *testing.T, fixture notificationStreamFixture) {
			now := time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano)
			_, err := fixture.server.db.Exec(`UPDATE profiles SET policy_updated_at = ?, restrictions_json = ? WHERE id = ?`, now, `{"allowDownloads":false}`, fixture.profileID)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "account authorization revision changes", mutate: func(t *testing.T, fixture notificationStreamFixture) {
			_, err := fixture.server.db.Exec(`UPDATE users SET permissions_json = ?, updated_at = ? WHERE id = ?`, `{"playMedia":false}`, time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano), fixture.accountID)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "session loses interactive status", mutate: func(t *testing.T, fixture notificationStreamFixture) {
			if _, err := fixture.server.db.Exec(`UPDATE sessions SET auth_provider = 'api_key' WHERE id = ?`, fixture.sessionID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "session profile binding changes", mutate: func(t *testing.T, fixture notificationStreamFixture) {
			if _, err := fixture.server.db.Exec(`UPDATE sessions SET profile_id = ? WHERE id = ?`, fixture.childID, fixture.sessionID); err != nil {
				t.Fatal(err)
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newNotificationStreamFixture(t)
			initial := requireNotificationStreamEvent(t, fixture.reader, time.Second)
			requireContentFreeNotificationStreamEvent(t, initial)
			test.mutate(t, fixture)
			createNotificationStreamInvalidation(t, fixture)
			requireNotificationStreamClosedWithoutEvent(t, fixture, time.Second)
		})
	}
}

func TestViewerNotificationStreamClosesWhenTrustedDeviceRequirementRejectsSession(t *testing.T) {
	previousInterval := viewerNotificationStreamAuthorizationInterval
	viewerNotificationStreamAuthorizationInterval = 25 * time.Millisecond
	t.Cleanup(func() { viewerNotificationStreamAuthorizationInterval = previousInterval })

	fixture := newNotificationStreamFixture(t)
	requireContentFreeNotificationStreamEvent(t, requireNotificationStreamEvent(t, fixture.reader, time.Second))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := fixture.server.db.Exec(`
		INSERT INTO settings (key, value_json, updated_at) VALUES ('devices', '{"requireTrustedDevices":true}', ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.server.db.Exec(`UPDATE devices SET trusted = 0, revoked_at = '' WHERE id = ?`, fixture.deviceID); err != nil {
		t.Fatal(err)
	}
	createNotificationStreamInvalidation(t, fixture)
	requireNotificationStreamClosedWithoutEvent(t, fixture, time.Second)
}

func TestViewerNotificationStreamClosesWhenAccountDevicePolicyRejectsSession(t *testing.T) {
	previousInterval := viewerNotificationStreamAuthorizationInterval
	viewerNotificationStreamAuthorizationInterval = 25 * time.Millisecond
	t.Cleanup(func() { viewerNotificationStreamAuthorizationInterval = previousInterval })

	fixture := newNotificationStreamFixture(t)
	requireContentFreeNotificationStreamEvent(t, requireNotificationStreamEvent(t, fixture.reader, time.Second))
	if _, err := fixture.server.db.Exec(`UPDATE users SET preferences_json = ? WHERE id = ?`,
		`{"devicePolicy":{"mode":"allowlist","allowedDeviceIds":["another-device"]}}`, fixture.accountID); err != nil {
		t.Fatal(err)
	}
	createNotificationStreamInvalidation(t, fixture)
	requireNotificationStreamClosedWithoutEvent(t, fixture, time.Second)
}

func TestViewerNotificationStreamInterleavingNeverExposesPrivatePayload(t *testing.T) {
	previousInterval := viewerNotificationStreamAuthorizationInterval
	viewerNotificationStreamAuthorizationInterval = 25 * time.Millisecond
	t.Cleanup(func() { viewerNotificationStreamAuthorizationInterval = previousInterval })

	tests := []struct {
		name   string
		revoke func(*testing.T, notificationStreamFixture)
	}{
		{name: "logout", revoke: func(t *testing.T, fixture notificationStreamFixture) {
			if _, err := fixture.server.db.Exec(`DELETE FROM sessions WHERE id = ?`, fixture.sessionID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "device revocation", revoke: func(t *testing.T, fixture notificationStreamFixture) {
			if _, err := fixture.server.db.Exec(`UPDATE devices SET revoked_at = ?, trusted = 0 WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), fixture.deviceID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "profile revocation", revoke: func(t *testing.T, fixture notificationStreamFixture) {
			if _, err := fixture.server.db.Exec(`UPDATE profiles SET disabled_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), fixture.profileID); err != nil {
				t.Fatal(err)
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateSelected := make(chan struct{})
			resumeWrite := make(chan struct{})
			var resumeOnce sync.Once
			var candidateCount atomic.Int32
			viewerNotificationStreamCandidateSelectedHook = func() {
				if candidateCount.Add(1) != 2 {
					return
				}
				close(candidateSelected)
				<-resumeWrite
			}
			fixture := newNotificationStreamFixture(t)
			t.Cleanup(func() {
				viewerNotificationStreamCandidateSelectedHook = nil
				resumeOnce.Do(func() { close(resumeWrite) })
			})
			requireContentFreeNotificationStreamEvent(t, requireNotificationStreamEvent(t, fixture.reader, time.Second))

			createNotificationStreamInvalidation(t, fixture)
			select {
			case <-candidateSelected:
			case <-time.After(time.Second):
				t.Fatal("stream did not pause after candidate invalidation selection")
			}
			test.revoke(t, fixture)
			resumeOnce.Do(func() { close(resumeWrite) })
			event := requireNotificationStreamEvent(t, fixture.reader, time.Second)
			requireContentFreeNotificationStreamEvent(t, event)
			requireNotificationStreamClosedWithoutEvent(t, fixture, time.Second)
		})
	}
}

func TestViewerNotificationStreamRemainsOpenForCurrentInteractiveSession(t *testing.T) {
	previousInterval := viewerNotificationStreamAuthorizationInterval
	viewerNotificationStreamAuthorizationInterval = 25 * time.Millisecond
	t.Cleanup(func() { viewerNotificationStreamAuthorizationInterval = previousInterval })

	fixture := newNotificationStreamFixture(t)
	initial := requireNotificationStreamEvent(t, fixture.reader, time.Second)
	requireContentFreeNotificationStreamEvent(t, initial)
	time.Sleep(4 * viewerNotificationStreamAuthorizationInterval)
	createNotificationStreamInvalidation(t, fixture)
	updated := requireNotificationStreamEvent(t, fixture.reader, time.Second)
	requireContentFreeNotificationStreamEvent(t, updated)
	fixture.cancel()
}
