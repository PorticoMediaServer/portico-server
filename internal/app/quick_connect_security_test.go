package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type failingQuickConnectReader struct{}

func (failingQuickConnectReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestQuickConnectCodeUsesUnambiguousBase31AlphabetWithoutModuloBias(t *testing.T) {
	if quickConnectAlphabet != "ABCDEFGHJKMNPQRSTUVWXYZ23456789" || len(quickConnectAlphabet) != 31 {
		t.Fatalf("alphabet = %q (size %d)", quickConnectAlphabet, len(quickConnectAlphabet))
	}
	code, err := randomQuickConnectCode(bytes.NewReader([]byte{0, 1, 30, 2, 3, 4, 5, 6}))
	if err != nil {
		t.Fatal(err)
	}
	if code != "AB9C-DEFG" {
		t.Fatalf("deterministic code = %q", code)
	}
	if strings.ContainsAny(code, "01ILO") {
		t.Fatalf("code contains ambiguous characters: %q", code)
	}
}

func TestNormalizeQuickConnectCodeAcceptsCanonicalInputAndRejectsAmbiguity(t *testing.T) {
	if got := normalizeQuickConnectCode(" abcd - 2345 "); got != "ABCD2345" {
		t.Fatalf("normalized code = %q", got)
	}
	for _, invalid := range []string{"ABCL-2345", "ABCO-2345", "ABC1-2345", "ABCD_2345", "AB-CD-2345", "ABCDEFG", "ABCDEFGHI"} {
		if got := normalizeQuickConnectCode(invalid); got != "" {
			t.Errorf("normalizeQuickConnectCode(%q) = %q, want empty", invalid, got)
		}
	}
	if got := quickConnectCodeForLookup(" ab-cd 23 "); got != "" {
		t.Fatalf("six-character code was accepted: %q", got)
	}
}

func TestQuickConnectActiveCodeUniquenessAndStaleReuse(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	now := time.Now().UTC()
	insert := func(id, status string, expires time.Time) {
		t.Helper()
		if _, err := db.Exec(`
			INSERT INTO quick_connect_requests (
				id, code, secret_hash, status, installation_id, device_name, app, platform,
				user_agent, client_ip, expires_at, created_at, updated_at
			) VALUES (?, 'AAAA-AAAA', ?, ?, 'installation-test-0001', 'TV', 'Portico', 'tv', '', '', ?, ?, ?)`,
			id, hashToken("secret-"+id), status, expires.Format(time.RFC3339), now.Add(-time.Minute).Format(time.RFC3339), now.Add(-time.Minute).Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}

	insert("active", "approved", now.Add(time.Minute))
	server.quickConnectEntropy = bytes.NewReader(make([]byte, quickConnectCodeAttempts*(quickConnectCodeLength+12)))
	if _, err := server.createQuickConnectRequest("new-secret", "installation-test-0002", "TV", "Portico", "tv", "", "127.0.0.1", now, now.Add(time.Minute)); !errors.Is(err, errQuickConnectCodeSpaceBusy) {
		t.Fatalf("active collision error = %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM quick_connect_requests`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("request count=%d err=%v", count, err)
	}

	if _, err := db.Exec(`UPDATE quick_connect_requests SET expires_at = ? WHERE id = 'active'`, now.Add(-time.Second).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	server.quickConnectEntropy = bytes.NewReader(make([]byte, quickConnectCodeLength+12))
	code, err := server.createQuickConnectRequest("replacement-secret", "installation-test-0003", "TV", "Portico", "tv", "", "127.0.0.1", now, now.Add(time.Minute))
	if err != nil || code != "AAAA-AAAA" {
		t.Fatalf("stale reuse code=%q err=%v", code, err)
	}
	var oldStatus string
	if err := db.QueryRow(`SELECT status FROM quick_connect_requests WHERE id = 'active'`).Scan(&oldStatus); err != nil || oldStatus != "expired" {
		t.Fatalf("old status=%q err=%v", oldStatus, err)
	}
}

func TestQuickConnectStartFailsClosedWhenEntropyIsUnavailable(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	server.quickConnectEntropy = failingQuickConnectReader{}

	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/start", QuickConnectStartRequest{InstallationID: "living-room-test-0001", DeviceName: "Living Room TV"}, nil)
	if status != http.StatusInternalServerError || !strings.Contains(body, "quick_connect_entropy_unavailable") {
		t.Fatalf("entropy failure status=%d body=%s", status, body)
	}
}

func TestQuickConnectStartIsNoStoreAndLinksExcludeHumanCode(t *testing.T) {
	serverURL := newAuthTestServer(t)
	request, err := http.NewRequest(http.MethodPost, serverURL+"/api/auth/quick-connect/start", strings.NewReader(`{"installationId":"living-room-test-0001","deviceName":"Living Room TV"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var started QuickConnectStartResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Pragma") != "no-cache" {
		t.Fatalf("status=%d cache=%q pragma=%q", response.StatusCode, response.Header.Get("Cache-Control"), response.Header.Get("Pragma"))
	}
	if started.ProtocolVersion != 1 || normalizeQuickConnectCode(started.Code) == "" {
		t.Fatalf("start response=%#v", started)
	}
	if strings.Contains(started.ApprovalURL, started.Code) || strings.Contains(started.DeepLinkURL, started.Code) || strings.Contains(started.ApprovalURL, "quickConnectCode=") || strings.Contains(started.DeepLinkURL, "&code=") {
		t.Fatalf("human code leaked into links: approval=%q deepLink=%q", started.ApprovalURL, started.DeepLinkURL)
	}
}

func TestQuickConnectPublicEndpointsHaveDistinctRateLimits(t *testing.T) {
	serverURL := newAuthTestServer(t)
	startPolicy := quickConnectPolicy(quickConnectRateStart)
	statusPolicy := quickConnectPolicy(quickConnectRateStatus)
	exchangePolicy := quickConnectPolicy(quickConnectRateExchange)
	if statusPolicy.limit <= startPolicy.limit || statusPolicy.limit <= exchangePolicy.limit {
		t.Fatalf("polling limit must be more permissive: start=%d status=%d exchange=%d", startPolicy.limit, statusPolicy.limit, exchangePolicy.limit)
	}

	for attempt := 0; attempt < startPolicy.limit; attempt++ {
		status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/start", QuickConnectStartRequest{InstallationID: "rate-limit-test-0001", DeviceName: "Rate Test"}, nil)
		if status != http.StatusCreated {
			t.Fatalf("start attempt %d status=%d body=%s", attempt, status, body)
		}
	}
	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/start", QuickConnectStartRequest{InstallationID: "rate-limit-test-0001", DeviceName: "Rate Test"}, nil)
	if status != http.StatusTooManyRequests || !strings.Contains(body, startPolicy.code) {
		t.Fatalf("start rate limit status=%d body=%s", status, body)
	}

	for attempt := 0; attempt < exchangePolicy.limit; attempt++ {
		status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/exchange", map[string]string{"secret": "invalid"}, nil)
		if status != http.StatusForbidden {
			t.Fatalf("exchange attempt %d status=%d body=%s", attempt, status, body)
		}
	}
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/exchange", map[string]string{"secret": "invalid"}, nil)
	if status != http.StatusTooManyRequests || !strings.Contains(body, exchangePolicy.code) {
		t.Fatalf("exchange rate limit status=%d body=%s", status, body)
	}

	for attempt := 0; attempt < statusPolicy.limit; attempt++ {
		status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/status", map[string]string{"secret": "invalid"}, nil)
		if status != http.StatusNotFound {
			t.Fatalf("status poll %d status=%d body=%s", attempt, status, body)
		}
	}
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/status", map[string]string{"secret": "invalid"}, nil)
	if status != http.StatusTooManyRequests || !strings.Contains(body, statusPolicy.code) {
		t.Fatalf("status rate limit status=%d body=%s", status, body)
	}
	if retryAfter := quickConnectRetryAfterFromBodylessRequest(t, serverURL); retryAfter == "" {
		t.Fatal("rate-limited Quick Connect response omitted Retry-After")
	}
}

func quickConnectRetryAfterFromBodylessRequest(t *testing.T, serverURL string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, serverURL+"/api/auth/quick-connect/status", strings.NewReader(`{"secret":"invalid"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("follow-up rate limit status=%d", response.StatusCode)
	}
	return response.Header.Get("Retry-After")
}

func TestQuickConnectRateLimiterBoundsAndPrunesMemory(t *testing.T) {
	limiter := boundedWindowRateLimiter{maxKeys: 3}
	policy := quickConnectPolicy(quickConnectRateStart)
	now := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
	for index, key := range []string{"one", "two", "three", "four"} {
		if allowed, _ := limiter.allow(key, policy, now.Add(time.Duration(index)*time.Second)); !allowed {
			t.Fatalf("new key %q was unexpectedly limited", key)
		}
	}
	if len(limiter.attempts) > 3 {
		t.Fatalf("limiter retained %d keys, want at most 3", len(limiter.attempts))
	}
	if allowed, _ := limiter.allow("fresh", policy, now.Add(3*time.Minute)); !allowed {
		t.Fatal("fresh key was limited after pruning window")
	}
	if len(limiter.attempts) != 1 {
		t.Fatalf("limiter retained stale keys after prune: %#v", limiter.attempts)
	}
}

func TestQuickConnectExchangeIsAtomicRecoverableAndExpiryBound(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	createApproved := func(secret string, expires time.Time) string {
		t.Helper()
		code, err := server.createQuickConnectRequest(secret, "quick-connect-test-0001", "Test TV", "Portico TV", "Test", "test", "127.0.0.1", now, expires)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE quick_connect_requests SET status = 'approved', user_id = ? WHERE code = ?`, userID, code); err != nil {
			t.Fatal(err)
		}
		return code
	}

	createApproved("one-time-secret", now.Add(time.Minute))
	request := httptest.NewRequest(http.MethodPost, "/api/auth/quick-connect/exchange", nil)
	result, first, err := server.exchangeQuickConnectCredentials(request, "one-time-secret")
	if err != nil || result.UserID != userID || first.AccessToken == "" || first.RefreshToken == "" {
		t.Fatalf("first exchange result=%#v credentials=%#v err=%v", result, first, err)
	}
	_, recovered, err := server.exchangeQuickConnectCredentials(request, "one-time-secret")
	if err != nil || recovered.AccessToken != first.AccessToken || recovered.RefreshToken != first.RefreshToken {
		t.Fatalf("receipt recovery credentials=%#v err=%v", recovered, err)
	}
	var receiptID string
	if err := db.QueryRow(`SELECT native_refresh_token_id FROM quick_connect_requests WHERE secret_hash = ?`, hashToken("one-time-secret")).Scan(&receiptID); err != nil || receiptID == "" {
		t.Fatalf("receipt id=%q err=%v", receiptID, err)
	}

	expiredCode := createApproved("expired-secret", now.Add(-time.Second))
	if _, _, err := server.exchangeQuickConnectCredentials(request, "expired-secret"); !errors.Is(err, errQuickConnectExpired) {
		t.Fatalf("expired consume error=%v", err)
	}
	var expiredStatus string
	if err := db.QueryRow(`SELECT status FROM quick_connect_requests WHERE code = ?`, expiredCode).Scan(&expiredStatus); err != nil {
		t.Fatal(err)
	}
	if expiredStatus != "approved" {
		t.Fatalf("expired request status=%q, atomic predicate should leave it approved", expiredStatus)
	}
}

func TestQuickConnectExchangeReceiptSurvivesResponseFailureAndConvergesConcurrentRetries(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	createApproved := func(secret, installationID string) {
		t.Helper()
		code, err := server.createQuickConnectRequest(secret, installationID, "Test TV", "Portico TV", "Roku", "test", "127.0.0.1", now, now.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE quick_connect_requests SET status = 'approved', user_id = ? WHERE code = ?`, userID, code); err != nil {
			t.Fatal(err)
		}
	}

	createApproved("post-commit-secret", "quick-connect-receipt-0001")
	server.nativeExchangeAfterCommit = func() error { return errors.New("injected response failure") }
	request := httptest.NewRequest(http.MethodPost, "/api/auth/quick-connect/exchange", nil)
	if _, _, err := server.exchangeQuickConnectCredentials(request, "post-commit-secret"); err == nil || !strings.Contains(err.Error(), "injected response failure") {
		t.Fatalf("post-commit fault error=%v", err)
	}
	server.nativeExchangeAfterCommit = nil
	_, recovered, err := server.exchangeQuickConnectCredentials(request, "post-commit-secret")
	if err != nil || recovered.RefreshToken == "" {
		t.Fatalf("recover committed exchange credentials=%#v err=%v", recovered, err)
	}
	assertOneQuickConnectCredentialReceipt(t, db, "post-commit-secret")

	createApproved("concurrent-secret", "quick-connect-receipt-0002")
	const callers = 8
	credentials := make([]NativeSessionCredentials, callers)
	errorsByCaller := make([]error, callers)
	var wait sync.WaitGroup
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			request := httptest.NewRequest(http.MethodPost, "/api/auth/quick-connect/exchange", nil)
			_, credentials[index], errorsByCaller[index] = server.exchangeQuickConnectCredentials(request, "concurrent-secret")
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
	assertOneQuickConnectCredentialReceipt(t, db, "concurrent-secret")
}

func TestQuickConnectExchangePreCommitFailureLeavesGrantRetryable(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	code, err := server.createQuickConnectRequest("pre-commit-secret", "quick-connect-receipt-0003", "Test TV", "Portico TV", "Roku", "test", "127.0.0.1", now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE quick_connect_requests SET status = 'approved', user_id = ? WHERE code = ?`, userID, code); err != nil {
		t.Fatal(err)
	}
	server.nativeCredentialEntropy = failingQuickConnectReader{}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/quick-connect/exchange", nil)
	if _, _, err := server.exchangeQuickConnectCredentials(request, "pre-commit-secret"); err == nil {
		t.Fatal("pre-commit entropy fault unexpectedly succeeded")
	}
	server.nativeCredentialEntropy = nil
	var status, receiptID string
	if err := db.QueryRow(`SELECT status, native_refresh_token_id FROM quick_connect_requests WHERE secret_hash = ?`, hashToken("pre-commit-secret")).Scan(&status, &receiptID); err != nil {
		t.Fatal(err)
	}
	if status != "approved" || receiptID != "" {
		t.Fatalf("pre-commit failure status=%q receipt=%q", status, receiptID)
	}
	if _, credentials, err := server.exchangeQuickConnectCredentials(request, "pre-commit-secret"); err != nil || credentials.RefreshToken == "" {
		t.Fatalf("retry credentials=%#v err=%v", credentials, err)
	}
	assertOneQuickConnectCredentialReceipt(t, db, "pre-commit-secret")
}

func TestQuickConnectExchangeRejectsDisabledUserBeforeCommitAndDuringRecovery(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	code, err := server.createQuickConnectRequest("disabled-user-secret", "quick-connect-disabled-0001", "Test TV", "Portico TV", "Roku", "test", "127.0.0.1", now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE quick_connect_requests SET status = 'approved', user_id = ? WHERE code = ?`, userID, code); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = ? WHERE id = ?`, now.Format(time.RFC3339), userID); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/quick-connect/exchange", nil)
	if _, _, err := server.exchangeQuickConnectCredentials(request, "disabled-user-secret"); !errors.Is(err, errNativeAccountDisabled) {
		t.Fatalf("disabled initial exchange error=%v", err)
	}
	var status, receiptID string
	if err := db.QueryRow(`SELECT status, native_refresh_token_id FROM quick_connect_requests WHERE secret_hash = ?`, hashToken("disabled-user-secret")).Scan(&status, &receiptID); err != nil {
		t.Fatal(err)
	}
	if status != "approved" || receiptID != "" {
		t.Fatalf("disabled initial exchange status=%q receipt=%q", status, receiptID)
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = '' WHERE id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	if _, credentials, err := server.exchangeQuickConnectCredentials(request, "disabled-user-secret"); err != nil || credentials.RefreshToken == "" {
		t.Fatalf("enabled exchange credentials=%#v err=%v", credentials, err)
	}
	if err := db.QueryRow(`SELECT native_refresh_token_id FROM quick_connect_requests WHERE secret_hash = ?`, hashToken("disabled-user-secret")).Scan(&receiptID); err != nil || receiptID == "" {
		t.Fatalf("load committed receipt id=%q err=%v", receiptID, err)
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = ? WHERE id = ?`, now.Format(time.RFC3339), userID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.exchangeQuickConnectCredentials(request, "disabled-user-secret"); !errors.Is(err, errNativeAccountDisabled) {
		t.Fatalf("disabled receipt recovery error=%v", err)
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = '' WHERE id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE devices SET revoked_at = ?, trusted = 0 WHERE id = (SELECT device_id FROM native_refresh_tokens WHERE id = ?)`, now.Format(time.RFC3339), receiptID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.exchangeQuickConnectCredentials(request, "disabled-user-secret"); !errors.Is(err, errInvalidNativeRefreshToken) {
		t.Fatalf("revoked device receipt recovery error=%v", err)
	}
}

func TestQuickConnectExchangeRollsBackDeviceReactivationBeforeReceiptCommit(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	user, err := server.getUser(userID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/quick-connect/exchange", nil)
	device := seedNativeDeviceForTest(t, db, user, nativeDeviceDescriptor{
		InstallationID: "quick-connect-rollback-0001", Name: "Old TV", App: "Portico TV", Platform: "Roku",
	}, now.Add(-time.Hour))
	if _, err := db.Exec(`UPDATE devices SET trusted = 0, revoked_at = ? WHERE id = ?`, now.Format(time.RFC3339), device.ID); err != nil {
		t.Fatal(err)
	}
	code, err := server.createQuickConnectRequest("rollback-device-secret", "quick-connect-rollback-0001", "Test TV", "Portico TV", "Roku", "test", "127.0.0.1", now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE quick_connect_requests SET status = 'approved', user_id = ? WHERE code = ?`, userID, code); err != nil {
		t.Fatal(err)
	}
	server.nativeExchangeAfterDeviceUpsert = func() error { return errors.New("injected after-device-upsert fault") }
	if _, _, err := server.exchangeQuickConnectCredentials(request, "rollback-device-secret"); err == nil || !strings.Contains(err.Error(), "after-device-upsert") {
		t.Fatalf("after-device-upsert fault error=%v", err)
	}
	server.nativeExchangeAfterDeviceUpsert = nil
	var trusted int
	var revokedAt, grantStatus, receiptID string
	if err := db.QueryRow(`SELECT trusted, revoked_at FROM devices WHERE id = ?`, device.ID).Scan(&trusted, &revokedAt); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT status, native_refresh_token_id FROM quick_connect_requests WHERE secret_hash = ?`, hashToken("rollback-device-secret")).Scan(&grantStatus, &receiptID); err != nil {
		t.Fatal(err)
	}
	if trusted != 0 || revokedAt == "" || grantStatus != "approved" || receiptID != "" {
		t.Fatalf("rolled-back state trusted=%d revoked=%q grant=%q receipt=%q", trusted, revokedAt, grantStatus, receiptID)
	}
	if _, credentials, err := server.exchangeQuickConnectCredentials(request, "rollback-device-secret"); !errors.Is(err, errDeviceNotAllowed) || credentials.AccessToken != "" || credentials.RefreshToken != "" {
		t.Fatalf("revoked device retry credentials=%#v err=%v", credentials, err)
	}
}

func assertOneQuickConnectCredentialReceipt(t *testing.T, db interface{ QueryRow(string, ...any) *sql.Row }, secret string) {
	t.Helper()
	var receiptID string
	if err := db.QueryRow(`SELECT native_refresh_token_id FROM quick_connect_requests WHERE secret_hash = ?`, hashToken(secret)).Scan(&receiptID); err != nil || receiptID == "" {
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

func TestQuickConnectSecretGeneratorPropagatesEntropyFailure(t *testing.T) {
	if _, err := randomQuickConnectSecret(failingQuickConnectReader{}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("secret entropy error=%v", err)
	}
}
