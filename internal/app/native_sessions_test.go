package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestNativeBearerPrimesSecureResourceCookieForBrowserAssets(t *testing.T) {
	serverURL := newAuthTestServer(t)
	credentials := createNativeCredentialsForTest(t, serverURL, "browser-assets-0001")
	request, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create auth request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("authenticate browser transport: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("auth status=%d body=%s", response.StatusCode, body)
	}
	setCookie := response.Header.Get("Set-Cookie")
	if !strings.Contains(setCookie, credentials.AccessToken) || !strings.Contains(setCookie, "HttpOnly") || !strings.Contains(setCookie, "Path=/") {
		t.Fatalf("native bearer did not prime a scoped HttpOnly resource cookie: %q", setCookie)
	}
}

func TestNativeRefreshRotationIsCrashSafeWithMatchingReceiptAndRevokesWrongKeyReuse(t *testing.T) {
	serverURL, db, _ := newAuthTestServerWithInstance(t)
	initial := createNativeCredentialsForTest(t, serverURL, "concurrent-refresh-0001")

	type result struct {
		status      int
		body        string
		credentials NativeSessionCredentials
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			status, body, credentials := refreshNativeCredentialsForTest(serverURL, initial.RefreshToken)
			results <- result{status: status, body: body, credentials: credentials}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	rotations := make([]result, 0, 2)
	for candidate := range results {
		rotations = append(rotations, candidate)
	}
	if len(rotations) != 2 {
		t.Fatalf("refresh result count = %d", len(rotations))
	}
	for _, candidate := range rotations {
		if candidate.status != http.StatusOK {
			t.Fatalf("concurrent refresh status=%d body=%s", candidate.status, candidate.body)
		}
	}
	first, second := rotations[0].credentials, rotations[1].credentials
	if first.AccessToken == "" || first.RefreshToken == "" || first.AccessToken != second.AccessToken || first.RefreshToken != second.RefreshToken || first.AccessExpiresAt != second.AccessExpiresAt || first.RefreshExpiresAt != second.RefreshExpiresAt {
		t.Fatalf("concurrent replay returned different replacements:\nfirst=%#v\nsecond=%#v", first, second)
	}
	initialExpiry, _ := time.Parse(time.RFC3339, initial.RefreshExpiresAt)
	rotatedExpiry, _ := time.Parse(time.RFC3339, first.RefreshExpiresAt)
	if rotatedExpiry.Before(initialExpiry) {
		t.Fatalf("rolling refresh expiry moved backward: initial=%s rotated=%s", initial.RefreshExpiresAt, first.RefreshExpiresAt)
	}

	if _, err := db.Exec(`UPDATE native_refresh_tokens SET consumed_at = ? WHERE token_hash = ?`, time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339Nano), hashToken(initial.RefreshToken)); err != nil {
		t.Fatalf("age consumed refresh: %v", err)
	}
	status, body, recovered := refreshNativeCredentialsForTest(serverURL, initial.RefreshToken)
	if status != http.StatusOK || recovered.RefreshToken != first.RefreshToken || recovered.AccessToken != first.AccessToken {
		t.Fatalf("durable receipt recovery status=%d body=%s recovered=%#v first=%#v", status, body, recovered, first)
	}
	status, body, _ = refreshNativeCredentialsWithRotationForTest(serverURL, initial.RefreshToken, strings.Repeat("B", 43))
	if status != http.StatusUnauthorized || !strings.Contains(body, "server_session_revoked") {
		t.Fatalf("wrong-key reuse status=%d body=%s", status, body)
	}
	assertBearerAuthenticated(t, http.DefaultClient, serverURL, first.AccessToken, false)
	status, body, _ = refreshNativeCredentialsForTest(serverURL, first.RefreshToken)
	if status != http.StatusUnauthorized || !strings.Contains(body, "server_session_revoked") {
		t.Fatalf("revoked replacement refresh status=%d body=%s", status, body)
	}
}

func TestNativeInstallationReusesDeviceAndDeviceRevocationCascades(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	first := createNativeCredentialsForTest(t, serverURL, "stable-installation-0001")
	second := createNativeCredentialsForTest(t, serverURL, "stable-installation-0001")
	if first.Device.ID == "" || first.Device.ID != second.Device.ID {
		t.Fatalf("same installation mapped to different devices: first=%s second=%s", first.Device.ID, second.Device.ID)
	}
	var deviceCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM devices WHERE user_id = ? AND installation_id = ?`, first.User.ID, "stable-installation-0001").Scan(&deviceCount); err != nil || deviceCount != 1 {
		t.Fatalf("installation device count=%d err=%v", deviceCount, err)
	}
	if err := server.revokeDevice(first.Device.ID); err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	assertBearerAuthenticated(t, http.DefaultClient, serverURL, first.AccessToken, false)
	assertBearerAuthenticated(t, http.DefaultClient, serverURL, second.AccessToken, false)
	status, body, _ := refreshNativeCredentialsForTest(serverURL, second.RefreshToken)
	if status != http.StatusUnauthorized || !strings.Contains(body, "server_session_revoked") {
		t.Fatalf("device-revoked refresh status=%d body=%s", status, body)
	}
	var activeFamilies int
	if err := db.QueryRow(`SELECT COUNT(*) FROM native_refresh_tokens WHERE device_id = ? AND revoked_at = ''`, first.Device.ID).Scan(&activeFamilies); err != nil || activeFamilies != 0 {
		t.Fatalf("active credential rows after device revoke=%d err=%v", activeFamilies, err)
	}
}

func TestNativeSessionRevokeEndpointIsIdempotentAndRevokesAccess(t *testing.T) {
	serverURL := newAuthTestServer(t)
	credentials := createNativeCredentialsForTest(t, serverURL, "explicit-revoke-0001")
	for attempt := 0; attempt < 2; attempt++ {
		status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/sessions/revoke", NativeSessionRefreshRequest{RefreshToken: credentials.RefreshToken}, nil)
		if status != http.StatusOK {
			t.Fatalf("revoke attempt %d status=%d body=%s", attempt, status, body)
		}
	}
	assertBearerAuthenticated(t, http.DefaultClient, serverURL, credentials.AccessToken, false)
}

func TestNativeCredentialEntropyFailureClosesSession(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	server.nativeCredentialEntropy = failingQuickConnectReader{}
	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/sessions", NativeSessionCreateRequest{
		Login: "admin", Password: "Password1234", InstallationID: "entropy-failure-0001",
		DeviceName: "Failing Phone", App: "Portico", Platform: "iOS",
	}, nil)
	if status != http.StatusInternalServerError || !strings.Contains(body, "session_failed") {
		t.Fatalf("native entropy failure status=%d body=%s", status, body)
	}
	var refreshCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM native_refresh_tokens`).Scan(&refreshCount); err != nil || refreshCount != 0 {
		t.Fatalf("refresh rows after entropy failure=%d err=%v", refreshCount, err)
	}
}

func TestNativeCredentialKeyRejectsUnsafeFilesystemShapes(t *testing.T) {
	t.Run("bad length", func(t *testing.T) {
		server, keyPath := nativeKeyTestServer(t)
		if err := os.WriteFile(keyPath, []byte("short"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := server.nativeCredentialHMACKey(); err == nil || !strings.Contains(err.Error(), "invalid length") {
			t.Fatalf("bad-length key error=%v", err)
		}
	})

	t.Run("permissive mode", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX mode check")
		}
		server, keyPath := nativeKeyTestServer(t)
		if err := os.WriteFile(keyPath, bytes.Repeat([]byte{1}, nativeCredentialKeySize), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := server.nativeCredentialHMACKey(); err == nil || !strings.Contains(err.Error(), "group or others") {
			t.Fatalf("permissive key error=%v", err)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation may require elevated rights")
		}
		server, keyPath := nativeKeyTestServer(t)
		target := filepath.Join(t.TempDir(), "target.key")
		if err := os.WriteFile(target, bytes.Repeat([]byte{2}, nativeCredentialKeySize), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, keyPath); err != nil {
			t.Fatal(err)
		}
		if _, err := server.nativeCredentialHMACKey(); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("symlink key error=%v", err)
		}
	})
}

func TestNativeCredentialKeyConcurrentCreationReturnsOneStableKey(t *testing.T) {
	server := &Server{cfg: config.Config{AppDataDir: t.TempDir()}}
	results := make(chan []byte, 2)
	errorsOut := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			key, err := server.nativeCredentialHMACKey()
			results <- key
			errorsOut <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatalf("concurrent key creation: %v", err)
		}
	}
	keys := make([][]byte, 0, 2)
	for key := range results {
		keys = append(keys, key)
	}
	if len(keys) != 2 || !bytes.Equal(keys[0], keys[1]) || len(keys[0]) != nativeCredentialKeySize {
		t.Fatalf("concurrent keys differ: %x %x", keys[0], keys[1])
	}
}

func TestQuickConnectExchangeReceiptRecoveryRejectsAtTTLBoundary(t *testing.T) {
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	insideBoundary := now.Add(-nativeExchangeReceiptTTL).Add(time.Nanosecond).Format(time.RFC3339Nano)
	atBoundary := now.Add(-nativeExchangeReceiptTTL).Format(time.RFC3339Nano)
	if !nativeExchangeReceiptRecoverable(insideBoundary, now) {
		t.Fatal("receipt was rejected immediately before the recovery boundary")
	}
	if nativeExchangeReceiptRecoverable(atBoundary, now) {
		t.Fatal("receipt remained recoverable at the recovery boundary")
	}
	quick := quickConnectConsumeResult{Status: "consumed", ReceiptID: "rft_receipt", ConsumedAt: atBoundary}
	if err := validateQuickConnectExchangeState(quick, now); !errors.Is(err, errQuickConnectAlreadyUsed) {
		t.Fatalf("Quick Connect boundary error=%v", err)
	}
}

func TestNativeRefreshRejectsDisabledUserWithoutConsumingOriginalCredential(t *testing.T) {
	serverURL, db, _ := newAuthTestServerWithInstance(t)
	initial := createNativeCredentialsForTest(t, serverURL, "disabled-refresh-0001")
	now := time.Now().UTC()
	if _, err := db.Exec(`UPDATE users SET disabled_at = ? WHERE id = ?`, now.Format(time.RFC3339), initial.User.ID); err != nil {
		t.Fatal(err)
	}
	status, body, _ := refreshNativeCredentialsForTest(serverURL, initial.RefreshToken)
	if status != http.StatusForbidden || !strings.Contains(body, "account_disabled") {
		t.Fatalf("disabled-user refresh status=%d", status)
	}
	var consumedAt, replacedByID string
	if err := db.QueryRow(`SELECT consumed_at, replaced_by_id FROM native_refresh_tokens WHERE token_hash = ?`, hashToken(initial.RefreshToken)).Scan(&consumedAt, &replacedByID); err != nil {
		t.Fatal(err)
	}
	if consumedAt != "" || replacedByID != "" {
		t.Fatalf("disabled-user refresh mutated original credential: consumed=%q replacement=%q", consumedAt, replacedByID)
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = '' WHERE id = ?`, initial.User.ID); err != nil {
		t.Fatal(err)
	}
	status, body, _ = refreshNativeCredentialsForTest(serverURL, initial.RefreshToken)
	if status != http.StatusUnauthorized || !strings.Contains(body, "server_session_revoked") {
		t.Fatalf("re-enabled revoked refresh status=%d body=%s", status, body)
	}
}

func TestNativeSessionCreationRejectsDisabledUserWithoutMutatingCredentials(t *testing.T) {
	serverURL, db, _ := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	installationID := "disabled-login-0001"
	request := NativeSessionCreateRequest{
		Login: "admin", Password: "Password1234", InstallationID: installationID,
		DeviceName: "Disabled Account Device", App: "Portico Test", Platform: "TestOS",
	}
	countState := func() (devices, refreshTokens, sessions int) {
		t.Helper()
		if err := db.QueryRow(`SELECT COUNT(*) FROM devices WHERE user_id = ? AND installation_id = ?`, userID, installationID).Scan(&devices); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM native_refresh_tokens WHERE user_id = ?`, userID).Scan(&refreshTokens); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, userID).Scan(&sessions); err != nil {
			t.Fatal(err)
		}
		return devices, refreshTokens, sessions
	}
	beforeDevices, beforeRefresh, beforeSessions := countState()
	if _, err := db.Exec(`UPDATE users SET disabled_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), userID); err != nil {
		t.Fatal(err)
	}
	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/sessions", request, nil)
	if status != http.StatusUnauthorized || !strings.Contains(body, `"code":"bad_credentials"`) || strings.Contains(body, "disabled") {
		t.Fatalf("disabled-user login disclosed account state: status=%d body=%s", status, body)
	}
	afterDevices, afterRefresh, afterSessions := countState()
	if afterDevices != beforeDevices || afterRefresh != beforeRefresh || afterSessions != beforeSessions {
		t.Fatalf("disabled-user login mutated credential state: devices %d->%d refresh %d->%d sessions %d->%d",
			beforeDevices, afterDevices, beforeRefresh, afterRefresh, beforeSessions, afterSessions)
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = '' WHERE id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	var credentials NativeSessionCredentials
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/sessions", request, &credentials)
	if status != http.StatusCreated || credentials.RefreshToken == "" || credentials.Device.InstallationID != installationID {
		t.Fatalf("re-enabled login status=%d body=%s credentials=%#v", status, body, credentials)
	}
}

func TestNativeSessionTreatsMalformedInstallationIDAsUntrustedOptionalMetadata(t *testing.T) {
	serverURL, _, _ := newAuthTestServerWithInstance(t)
	var credentials NativeSessionCredentials
	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/sessions", NativeSessionCreateRequest{
		Login: "admin", Password: "Password1234", InstallationID: "bad id!",
		DeviceName: "Metadata Rotation Test", App: "Portico Test", Platform: "TestOS",
	}, &credentials)
	if status != http.StatusCreated || credentials.RefreshToken == "" || !strings.HasPrefix(credentials.Device.InstallationID, "server:dev_") {
		t.Fatalf("malformed optional installation metadata should be replaced server-side: status=%d body=%s credentials=%#v", status, body, credentials)
	}
}

func createNativeCredentialsForTest(t *testing.T, serverURL, installationID string) NativeSessionCredentials {
	t.Helper()
	var credentials NativeSessionCredentials
	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/sessions", NativeSessionCreateRequest{
		Login: "admin", Password: "Password1234", InstallationID: installationID,
		DeviceName: "Native Test Device", App: "Portico Test", Platform: "TestOS",
	}, &credentials)
	if status != http.StatusCreated {
		t.Fatalf("create native credentials status=%d body=%s", status, body)
	}
	return credentials
}

func refreshNativeCredentialsForTest(serverURL, refreshToken string) (int, string, NativeSessionCredentials) {
	return refreshNativeCredentialsWithRotationForTest(serverURL, refreshToken, strings.Repeat("A", 43))
}

func refreshNativeCredentialsWithRotationForTest(serverURL, refreshToken, rotationKey string) (int, string, NativeSessionCredentials) {
	body, _ := json.Marshal(NativeSessionRefreshRequest{RefreshToken: refreshToken, RotationKey: rotationKey})
	req, _ := http.NewRequest(http.MethodPost, serverURL+"/api/auth/sessions/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error(), NativeSessionCredentials{}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var credentials NativeSessionCredentials
	_ = json.Unmarshal(raw, &credentials)
	return resp.StatusCode, string(raw), credentials
}

func seedNativeDeviceForTest(t *testing.T, db *sql.DB, user User, descriptor nativeDeviceDescriptor, now time.Time) Device {
	t.Helper()
	device := Device{
		ID:             "dev_test_" + strings.ReplaceAll(descriptor.InstallationID, "-", "_"),
		InstallationID: descriptor.InstallationID,
		UserID:         user.ID,
		User:           user.DisplayName,
		Name:           descriptor.Name,
		AutoName:       descriptor.Name,
		App:            descriptor.App,
		Platform:       descriptor.Platform,
		Trusted:        true,
		CreatedAt:      now.Format(time.RFC3339),
		LastSeenAt:     now.Format(time.RFC3339),
	}
	if _, err := db.Exec(`
		INSERT INTO devices (id, user_id, installation_id, name, display_name, app, platform, trusted, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, '', ?, ?, 1, ?, ?)`, device.ID, device.UserID, device.InstallationID, device.AutoName,
		device.App, device.Platform, device.CreatedAt, device.LastSeenAt); err != nil {
		t.Fatal(err)
	}
	return device
}

func nativeKeyTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	appData := t.TempDir()
	dir := filepath.Join(appData, "secrets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return &Server{cfg: config.Config{AppDataDir: appData}}, filepath.Join(dir, "native-session-hmac.key")
}
