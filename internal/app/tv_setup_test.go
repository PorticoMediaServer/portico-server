package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNearbyTVSetupAuthorizesTVDeviceSession(t *testing.T) {
	serverURL := newAuthTestServer(t)
	curve := ecdh.X25519()
	tvPrivateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate tv key: %v", err)
	}
	tvPublicKey := base64.RawURLEncoding.EncodeToString(tvPrivateKey.PublicKey().Bytes())

	var setupSession TVSetupSessionResponse
	status, body := doJSONWithUserAgent(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/tv-setup/sessions", TVSetupSessionRequest{
		InstallationID:  "apple-tv-install-0001",
		DevicePublicKey: tvPublicKey,
		DeviceName:      "Living Room Apple TV",
		Platform:        "tvOS",
		AppVersion:      "1.0",
		AuthModeHint:    "local",
	}, &setupSession, "Portico tvOS/1.0")
	if status != http.StatusCreated {
		t.Fatalf("tv setup session status=%d body=%s", status, body)
	}
	if setupSession.SetupSessionID == "" || setupSession.Code == "" || setupSession.DevicePublicKey != tvPublicKey || setupSession.EncryptedGrant != nil {
		t.Fatalf("unexpected setup session = %#v", setupSession)
	}
	if setupSession.ProtocolVersion != 1 || len(setupSession.Code) != 9 || setupSession.Code[4] != '-' || normalizeTVSetupCode(setupSession.Code) == "" {
		t.Fatalf("setup session code/protocol = %q/v%d, want canonical protocol-v1 code", setupSession.Code, setupSession.ProtocolVersion)
	}

	jar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: jar}
	loginUser(t, adminClient, serverURL)

	status, body = doJSON(t, adminClient, http.MethodPost, serverURL+"/api/auth/tv-setup/grants", TVSetupGrantRequest{
		SetupSessionID:  setupSession.SetupSessionID,
		Code:            "WRONG",
		DevicePublicKey: tvPublicKey,
	}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("wrong code grant status=%d body=%s", status, body)
	}

	var grant TVSetupGrantResponse
	status, body = doJSON(t, adminClient, http.MethodPost, serverURL+"/api/auth/tv-setup/grants", TVSetupGrantRequest{
		SetupSessionID:  setupSession.SetupSessionID,
		Code:            strings.ToLower(strings.ReplaceAll(setupSession.Code, "-", " - ")),
		DevicePublicKey: tvPublicKey,
	}, &grant)
	if status != http.StatusCreated {
		t.Fatalf("grant status=%d body=%s", status, body)
	}
	if grant.Status != "grant_ready" || grant.EncryptedGrant.Ciphertext == "" || grant.EncryptedGrant.ServerPublicKey == "" {
		t.Fatalf("unexpected grant = %#v", grant)
	}

	var polled TVSetupSessionResponse
	status, body = doJSON(t, http.DefaultClient, http.MethodGet, serverURL+"/api/auth/tv-setup/sessions/"+setupSession.SetupSessionID, nil, &polled)
	if status != http.StatusOK || polled.Status != "grant_ready" || polled.EncryptedGrant == nil {
		t.Fatalf("polled session status=%d body=%s session=%#v", status, body, polled)
	}

	payload := decryptTVSetupGrantForTest(t, tvPrivateKey, setupSession.SetupSessionID, grant.EncryptedGrant)
	if payload.SetupSessionID != setupSession.SetupSessionID || payload.GrantSecret == "" || payload.UserID == "" {
		t.Fatalf("unexpected grant payload = %#v", payload)
	}

	deviceClient := http.DefaultClient
	var auth NativeSessionCredentials
	status, body = doJSONWithUserAgent(t, deviceClient, http.MethodPost, serverURL+"/api/auth/tv-setup/redeem", TVSetupRedeemRequest{
		SetupSessionID: setupSession.SetupSessionID,
		GrantSecret:    payload.GrantSecret,
		DeviceName:     "Living Room Apple TV",
		Platform:       "tvOS",
		AppVersion:     "1.0",
	}, &auth, "Portico tvOS/1.0")
	if status != http.StatusOK || auth.AccessToken == "" || auth.RefreshToken == "" || auth.User.Username != "admin" || auth.Device.InstallationID != "apple-tv-install-0001" {
		t.Fatalf("redeem status=%d body=%s auth=%#v", status, body, auth)
	}

	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create auth/me: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	resp, err := deviceClient.Do(req)
	if err != nil {
		t.Fatalf("auth/me with tv token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tv bearer auth status=%d", resp.StatusCode)
	}

	var recovered NativeSessionCredentials
	status, body = doJSONWithUserAgent(t, deviceClient, http.MethodPost, serverURL+"/api/auth/tv-setup/redeem", TVSetupRedeemRequest{
		SetupSessionID: setupSession.SetupSessionID,
		GrantSecret:    payload.GrantSecret,
	}, &recovered, "Portico tvOS/1.0")
	if status != http.StatusOK || recovered.AccessToken != auth.AccessToken || recovered.RefreshToken != auth.RefreshToken {
		t.Fatalf("receipt recovery status=%d body=%s credentials=%#v", status, body, recovered)
	}

	var audit ListResponse[AuditEvent]
	status, body = doJSON(t, adminClient, http.MethodGet, serverURL+"/api/audit-events?limit=50", nil, &audit)
	if status != http.StatusOK {
		t.Fatalf("audit status=%d body=%s", status, body)
	}
	for _, action := range []string{"tv_setup.grant_failed", "tv_setup.grant_created", "tv_setup.redeemed"} {
		if !hasAuditAction(audit.Items, action) {
			t.Fatalf("expected audit action %s, got %#v", action, audit.Items)
		}
	}
}

func TestTVSetupExchangeReceiptSurvivesResponseFailureAndConvergesConcurrentRetries(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	postCommit := seedAuthorizedTVSetupReceiptTest(t, db, server, "tv-post-commit", "ABCD-EFGH", "tv-setup-receipt-0001")
	server.nativeExchangeAfterCommit = func() error { return errors.New("injected response failure") }
	request := httptest.NewRequest(http.MethodPost, "/api/auth/tv-setup/redeem", nil)
	if _, err := server.redeemTVSetupCredentials(request, postCommit, "tv-post-commit"); err == nil || !strings.Contains(err.Error(), "injected response failure") {
		t.Fatalf("post-commit fault error=%v", err)
	}
	server.nativeExchangeAfterCommit = nil
	recovered, err := server.redeemTVSetupCredentials(request, postCommit, "tv-post-commit")
	if err != nil || recovered.RefreshToken == "" {
		t.Fatalf("recover committed TV setup credentials=%#v err=%v", recovered, err)
	}
	assertOneTVSetupCredentialReceipt(t, db, postCommit.ID)

	concurrent := seedAuthorizedTVSetupReceiptTest(t, db, server, "tv-concurrent", "JKLM-NPQR", "tv-setup-receipt-0002")
	const callers = 8
	credentials := make([]NativeSessionCredentials, callers)
	errorsByCaller := make([]error, callers)
	var wait sync.WaitGroup
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			request := httptest.NewRequest(http.MethodPost, "/api/auth/tv-setup/redeem", nil)
			credentials[index], errorsByCaller[index] = server.redeemTVSetupCredentials(request, concurrent, "tv-concurrent")
		}(index)
	}
	wait.Wait()
	for index := range callers {
		if errorsByCaller[index] != nil {
			t.Fatalf("concurrent caller %d: %v", index, errorsByCaller[index])
		}
		if credentials[index].AccessToken != credentials[0].AccessToken || credentials[index].RefreshToken != credentials[0].RefreshToken {
			t.Fatalf("concurrent caller %d received a different credential family", index)
		}
	}
	assertOneTVSetupCredentialReceipt(t, db, concurrent.ID)
}

func TestTVSetupExchangePreCommitFailureLeavesGrantRetryable(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	session := seedAuthorizedTVSetupReceiptTest(t, db, server, "tv-pre-commit", "RSTU-VWXY", "tv-setup-receipt-0003")
	server.nativeCredentialEntropy = failingQuickConnectReader{}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/tv-setup/redeem", nil)
	if _, err := server.redeemTVSetupCredentials(request, session, "tv-pre-commit"); err == nil {
		t.Fatal("pre-commit entropy fault unexpectedly succeeded")
	}
	server.nativeCredentialEntropy = nil
	var status, receiptID string
	if err := db.QueryRow(`SELECT status, native_refresh_token_id FROM tv_setup_sessions WHERE id = ?`, session.ID).Scan(&status, &receiptID); err != nil {
		t.Fatal(err)
	}
	if status != "grant_ready" || receiptID != "" {
		t.Fatalf("pre-commit failure status=%q receipt=%q", status, receiptID)
	}
	if credentials, err := server.redeemTVSetupCredentials(request, session, "tv-pre-commit"); err != nil || credentials.RefreshToken == "" {
		t.Fatalf("retry credentials=%#v err=%v", credentials, err)
	}
	assertOneTVSetupCredentialReceipt(t, db, session.ID)
}

func TestTVSetupExchangeRejectsDisabledUserBeforeCommitAndDuringRecovery(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	session := seedAuthorizedTVSetupReceiptTest(t, db, server, "tv-disabled-user", "CDEF-GHJK", "tv-setup-disabled-0001")
	var userID string
	if err := db.QueryRow(`SELECT user_id FROM tv_setup_sessions WHERE id = ?`, session.ID).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`UPDATE users SET disabled_at = ? WHERE id = ?`, now.Format(time.RFC3339), userID); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/tv-setup/redeem", nil)
	if _, err := server.redeemTVSetupCredentials(request, session, "tv-disabled-user"); !errors.Is(err, errNativeAccountDisabled) {
		t.Fatalf("disabled initial exchange error=%v", err)
	}
	var status, receiptID string
	if err := db.QueryRow(`SELECT status, native_refresh_token_id FROM tv_setup_sessions WHERE id = ?`, session.ID).Scan(&status, &receiptID); err != nil {
		t.Fatal(err)
	}
	if status != "grant_ready" || receiptID != "" {
		t.Fatalf("disabled initial exchange status=%q receipt=%q", status, receiptID)
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = '' WHERE id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	if credentials, err := server.redeemTVSetupCredentials(request, session, "tv-disabled-user"); err != nil || credentials.RefreshToken == "" {
		t.Fatalf("enabled exchange credentials=%#v err=%v", credentials, err)
	}
	if err := db.QueryRow(`SELECT native_refresh_token_id FROM tv_setup_sessions WHERE id = ?`, session.ID).Scan(&receiptID); err != nil || receiptID == "" {
		t.Fatalf("load committed receipt id=%q err=%v", receiptID, err)
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = ? WHERE id = ?`, now.Format(time.RFC3339), userID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.redeemTVSetupCredentials(request, session, "tv-disabled-user"); !errors.Is(err, errNativeAccountDisabled) {
		t.Fatalf("disabled receipt recovery error=%v", err)
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = '' WHERE id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE native_refresh_tokens SET expires_at = ? WHERE id = ?`, now.Add(-time.Second).Format(time.RFC3339Nano), receiptID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.redeemTVSetupCredentials(request, session, "tv-disabled-user"); !errors.Is(err, errInvalidNativeRefreshToken) {
		t.Fatalf("expired credential receipt recovery error=%v", err)
	}
}

func TestTVSetupExchangeRollsBackDeviceReactivationBeforeReceiptCommit(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	session := seedAuthorizedTVSetupReceiptTest(t, db, server, "tv-device-rollback", "DEFG-HJKM", "tv-setup-rollback-0001")
	var userID string
	if err := db.QueryRow(`SELECT user_id FROM tv_setup_sessions WHERE id = ?`, session.ID).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	user, err := server.getUser(userID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/tv-setup/redeem", nil)
	device := seedNativeDeviceForTest(t, db, user, nativeDeviceDescriptor{
		InstallationID: session.InstallationID, Name: "Old TV", App: "Portico TV", Platform: "tvOS",
	}, now.Add(-time.Hour))
	if _, err := db.Exec(`UPDATE devices SET trusted = 0, revoked_at = ? WHERE id = ?`, now.Format(time.RFC3339), device.ID); err != nil {
		t.Fatal(err)
	}
	server.nativeExchangeAfterDeviceUpsert = func() error { return errors.New("injected after-device-upsert fault") }
	if _, err := server.redeemTVSetupCredentials(request, session, "tv-device-rollback"); err == nil || !strings.Contains(err.Error(), "after-device-upsert") {
		t.Fatalf("after-device-upsert fault error=%v", err)
	}
	server.nativeExchangeAfterDeviceUpsert = nil
	var trusted int
	var revokedAt, grantStatus, receiptID string
	if err := db.QueryRow(`SELECT trusted, revoked_at FROM devices WHERE id = ?`, device.ID).Scan(&trusted, &revokedAt); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT status, native_refresh_token_id FROM tv_setup_sessions WHERE id = ?`, session.ID).Scan(&grantStatus, &receiptID); err != nil {
		t.Fatal(err)
	}
	if trusted != 0 || revokedAt == "" || grantStatus != "grant_ready" || receiptID != "" {
		t.Fatalf("rolled-back state trusted=%d revoked=%q grant=%q receipt=%q", trusted, revokedAt, grantStatus, receiptID)
	}
	if credentials, err := server.redeemTVSetupCredentials(request, session, "tv-device-rollback"); !errors.Is(err, errDeviceNotAllowed) || credentials.AccessToken != "" || credentials.RefreshToken != "" {
		t.Fatalf("revoked device retry was not terminal: credentials=%#v err=%v", credentials, err)
	}
}

func seedAuthorizedTVSetupReceiptTest(t *testing.T, db *sql.DB, server *Server, secret, code, installationID string) tvSetupSessionRecord {
	t.Helper()
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	session := tvSetupSessionRecord{
		ID: "setup-" + installationID, Code: code, Status: "pending", InstallationID: installationID,
		DevicePublicKey: "test-public-key", DeviceName: "Test TV", Platform: "tvOS",
		ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
	}
	if err := server.createTVSetupSession(session); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		UPDATE tv_setup_sessions SET status = 'grant_ready', user_id = ?, grant_secret_hash = ?, updated_at = ? WHERE id = ?`,
		userID, hashToken(secret), now.Format(time.RFC3339), session.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := server.tvSetupSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func assertOneTVSetupCredentialReceipt(t *testing.T, db *sql.DB, sessionID string) {
	t.Helper()
	var receiptID string
	if err := db.QueryRow(`SELECT native_refresh_token_id FROM tv_setup_sessions WHERE id = ?`, sessionID).Scan(&receiptID); err != nil || receiptID == "" {
		t.Fatalf("receipt id=%q err=%v", receiptID, err)
	}
	var refreshCount, sessionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM native_refresh_tokens WHERE id = ?`, receiptID).Scan(&refreshCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, nativeAccessSessionID(receiptID)).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if refreshCount != 1 || sessionCount != 1 {
		t.Fatalf("receipt %q refresh rows=%d access sessions=%d", receiptID, refreshCount, sessionCount)
	}
}

func TestTVSetupCodeProtocolV1NormalizationAndLegacyRejection(t *testing.T) {
	valid := map[string]string{
		"ABCD-EFGH":   "ABCDEFGH",
		"abcd-efgh":   "ABCDEFGH",
		" ABCD EFGH ": "ABCDEFGH",
		"ABCD - EFGH": "ABCDEFGH",
		"ABCDEFGH":    "ABCDEFGH",
	}
	for input, expected := range valid {
		if actual := normalizeTVSetupCode(input); actual != expected {
			t.Errorf("normalizeTVSetupCode(%q)=%q, want %q", input, actual, expected)
		}
	}
	for _, input := range []string{
		"ABCD--EFGH", "ABCD_EFGH", "ABCD\tEFGH", "ABCDIEFG", "ABCDLEFG", "ABCDOEFG", "ABCD0EFG", "ABCD1EFG", "ABC-EFG", "ABCDE-FGHI",
	} {
		if actual := normalizeTVSetupCode(input); actual != "" {
			t.Errorf("normalizeTVSetupCode(%q)=%q, want rejection", input, actual)
		}
	}
	if !tvSetupCodeMatchesSession(" abcd - efgh ", "ABCD-EFGH") {
		t.Fatal("protocol-v1 equivalent code did not match")
	}
	if tvSetupCodeMatchesSession("123456", "123456") || tvSetupCodeForLookup("123456") != "" {
		t.Fatal("six-digit setup code was accepted")
	}
	if tvSetupProtocolForSession("ABCD-EFGH") != 1 {
		t.Fatal("setup session did not report protocol version 1")
	}
	for attempt := 0; attempt < 100; attempt++ {
		code, err := randomTVSetupCode(rand.Reader)
		if err != nil {
			t.Fatalf("randomTVSetupCode: %v", err)
		}
		if len(code) != 9 || code[4] != '-' || normalizeTVSetupCode(code) == "" {
			t.Fatalf("generated invalid protocol-v1 code %q", code)
		}
	}
}

func TestTVSetupCodeActiveUniquenessAndStaleReuse(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	now := time.Now().UTC()
	makeSession := func(id string, code string, expiresAt time.Time) tvSetupSessionRecord {
		return tvSetupSessionRecord{
			ID: id, Code: code, Status: "pending", InstallationID: "installation-" + id,
			DevicePublicKey: "public-key", DeviceName: "TV", Platform: "tvOS",
			ExpiresAt: expiresAt.Format(time.RFC3339), CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
		}
	}
	if err := server.createTVSetupSession(makeSession("active-1", "ABCD-EFGH", now.Add(time.Minute))); err != nil {
		t.Fatalf("create active session: %v", err)
	}
	if err := server.createTVSetupSession(makeSession("active-2", "ABCD-EFGH", now.Add(time.Minute))); !isTVSetupCodeConflict(err) {
		t.Fatalf("duplicate active code error=%v, want unique conflict", err)
	}
	if err := server.createTVSetupSession(makeSession("stale-1", "JKMN-PQRS", now.Add(-time.Second))); err != nil {
		t.Fatalf("create stale session: %v", err)
	}
	if err := server.createTVSetupSession(makeSession("stale-2", "JKMN-PQRS", now.Add(time.Minute))); err != nil {
		t.Fatalf("reuse stale code: %v", err)
	}
	var staleStatus string
	if err := db.QueryRow(`SELECT status FROM tv_setup_sessions WHERE id = 'stale-1'`).Scan(&staleStatus); err != nil || staleStatus != "expired" {
		t.Fatalf("stale session status=%q err=%v", staleStatus, err)
	}
}

func TestTVSetupAdmissionCapsAndPrunesExpiredRowsIndependently(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO tv_setup_sessions (id, code, status, installation_id, device_public_key, expires_at, created_at, updated_at)
		VALUES ('ancient-tv-setup', 'WXYZ-2345', 'expired', 'ancient-installation', 'public-key', ?, ?, ?)`,
		now.Add(-25*time.Hour).Format(time.RFC3339), now.Add(-26*time.Hour).Format(time.RFC3339), now.Add(-25*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	for index, code := range []string{"ABCD-2345", "ABCD-2346", "ABCD-2347"} {
		session := tvSetupSessionRecord{
			ID: fmt.Sprintf("bounded-%d", index), Code: code, Status: "pending", InstallationID: "bounded-installation",
			DevicePublicKey: "public-key", DeviceName: "TV", Platform: "tvOS",
			ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
		}
		if err := server.createTVSetupSession(session); err != nil {
			t.Fatalf("create bounded session %d: %v", index, err)
		}
	}
	if err := server.createTVSetupSession(tvSetupSessionRecord{
		ID: "bounded-overflow", Code: "ABCD-2348", Status: "pending", InstallationID: "bounded-installation",
		DevicePublicKey: "public-key", DeviceName: "TV", Platform: "tvOS",
		ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
	}); !errors.Is(err, errTVSetupDeviceBusy) {
		t.Fatalf("overflow error=%v", err)
	}
	var ancientRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tv_setup_sessions WHERE id = 'ancient-tv-setup'`).Scan(&ancientRows); err != nil || ancientRows != 0 {
		t.Fatalf("independent TV setup prune rows=%d err=%v", ancientRows, err)
	}
}

type zeroTVSetupEntropy struct{}

func (zeroTVSetupEntropy) Read(value []byte) (int, error) {
	clear(value)
	return len(value), nil
}

func TestTVSetupCodeCollisionBudgetFailsClosed(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	now := time.Now().UTC()
	if err := server.createTVSetupSession(tvSetupSessionRecord{
		ID: "existing-collision", Code: "AAAA-AAAA", Status: "pending", InstallationID: "existing-installation",
		DevicePublicKey: "public-key", DeviceName: "Existing TV", Platform: "tvOS",
		ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("seed active collision: %v", err)
	}
	server.nativeCredentialEntropy = zeroTVSetupEntropy{}
	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/tv-setup/sessions", TVSetupSessionRequest{
		InstallationID: "collision-installation-0001", DevicePublicKey: validTVSetupPublicKeyForTest(t),
	}, nil)
	if status != http.StatusServiceUnavailable || !strings.Contains(body, "tv_setup_unavailable") {
		t.Fatalf("collision budget status=%d body=%s", status, body)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tv_setup_sessions`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("TV setup rows after collision exhaustion=%d err=%v", count, err)
	}
}

func decryptTVSetupGrantForTest(t *testing.T, tvPrivateKey *ecdh.PrivateKey, setupSessionID string, encryptedGrant TVSetupEncryptedGrant) tvSetupGrantPayload {
	t.Helper()
	serverPublicKeyBytes, err := decodeBase64URLFlexible(encryptedGrant.ServerPublicKey)
	if err != nil {
		t.Fatalf("decode server public key: %v", err)
	}
	serverPublicKey, err := ecdh.X25519().NewPublicKey(serverPublicKeyBytes)
	if err != nil {
		t.Fatalf("server public key: %v", err)
	}
	sharedSecret, err := tvPrivateKey.ECDH(serverPublicKey)
	if err != nil {
		t.Fatalf("derive shared secret: %v", err)
	}
	key, err := tvSetupGrantKey(sharedSecret, setupSessionID)
	if err != nil {
		t.Fatalf("derive grant key: %v", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("create gcm: %v", err)
	}
	nonce, err := decodeBase64URLFlexible(encryptedGrant.Nonce)
	if err != nil {
		t.Fatalf("decode nonce: %v", err)
	}
	ciphertext, err := decodeBase64URLFlexible(encryptedGrant.Ciphertext)
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(setupSessionID))
	if err != nil {
		t.Fatalf("decrypt grant: %v", err)
	}
	var payload tvSetupGrantPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		t.Fatalf("decode grant payload: %v", err)
	}
	return payload
}
