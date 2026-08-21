package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func TestPorticoLoginFailureRedirectUsesCanonicalMessageID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://localhost:32500/api/auth/portico/callback", nil)
	recorder := httptest.NewRecorder()
	server := &Server{}

	server.redirectPorticoLoginResult(
		recorder,
		request,
		"http://localhost:32500/#/home",
		false,
		"auth.profile-selection-failed",
	)

	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if got := location.Query().Get("porticoLoginMessageId"); got != "auth.profile-selection-failed" {
		t.Fatalf("message id = %q", got)
	}
	if location.Query().Get("porticoLoginError") != "" {
		t.Fatalf("callback redirect must not expose server-authored sentences: %s", location.String())
	}
}

func TestSameUserCanHaveMultipleConcurrentSessions(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)

	jarA, _ := cookiejar.New(nil)
	jarB, _ := cookiejar.New(nil)
	clientA := &http.Client{Jar: jarA}
	clientB := &http.Client{Jar: jarB}

	loginUser(t, clientA, serverURL)
	loginUser(t, clientB, serverURL)

	cookieA := sessionCookieFromJar(t, jarA, serverURL)
	cookieB := sessionCookieFromJar(t, jarB, serverURL)
	lanContext := context.WithValue(context.Background(), requestTransportSecureKey{}, false)
	expectedName := server.sessionCookieNameContext(lanContext)
	if cookieA.Name != expectedName {
		t.Fatalf("expected LAN transport-scoped session cookie name %q, got %q", expectedName, cookieA.Name)
	}
	if cookieA.Name != cookieB.Name {
		t.Fatalf("expected same server-scoped cookie name, got %q and %q", cookieA.Name, cookieB.Name)
	}
	if cookieA.Value == cookieB.Value {
		t.Fatalf("expected separate login sessions to use different tokens")
	}

	assertAuthenticated(t, clientA, serverURL, true)
	assertAuthenticated(t, clientB, serverURL, true)

	status, body := doJSON(t, clientA, http.MethodPost, serverURL+"/api/auth/logout", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("logout status = %d, body: %s", status, body)
	}

	assertAuthenticated(t, clientA, serverURL, false)
	assertAuthenticated(t, clientB, serverURL, true)
}

func TestAccountSessionsListAndRevokeOwnNonCurrentSession(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	if _, err := db.Exec(`DELETE FROM sessions`); err != nil {
		t.Fatalf("clear setup session: %v", err)
	}

	jarA, _ := cookiejar.New(nil)
	jarB, _ := cookiejar.New(nil)
	clientA := &http.Client{Jar: jarA}
	clientB := &http.Client{Jar: jarB}

	loginUser(t, clientA, serverURL)
	loginUser(t, clientB, serverURL)

	var sessions ListResponse[AccountSession]
	status, body := doJSON(t, clientA, http.MethodGet, serverURL+"/api/account/sessions", nil, &sessions)
	if status != http.StatusOK {
		t.Fatalf("account sessions status = %d, body: %s", status, body)
	}
	if sessions.Total != 2 || len(sessions.Items) != 2 {
		t.Fatalf("expected two active sessions, got %#v", sessions)
	}

	var current AccountSession
	var other AccountSession
	for _, session := range sessions.Items {
		if session.Current {
			current = session
		} else {
			other = session
		}
		if session.ID == "" || session.DeviceName == "" || session.ExpiresAt == "" {
			t.Fatalf("session inventory omitted required fields: %#v", session)
		}
	}
	if current.ID == "" || current.CanRevoke {
		t.Fatalf("current session should be identified and not revocable: %#v", current)
	}
	if other.ID == "" || !other.CanRevoke {
		t.Fatalf("non-current session should be revocable: %#v", other)
	}

	status, body = doJSON(t, clientA, http.MethodDelete, serverURL+"/api/account/sessions/"+current.ID, nil, nil)
	if status != http.StatusConflict || !strings.Contains(body, "current_session") {
		t.Fatalf("delete current session status = %d, body: %s", status, body)
	}

	status, body = doJSON(t, clientA, http.MethodDelete, serverURL+"/api/account/sessions/"+other.ID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("delete other session status = %d, body: %s", status, body)
	}

	assertAuthenticated(t, clientA, serverURL, true)
	assertAuthenticated(t, clientB, serverURL, false)
	sessions = ListResponse[AccountSession]{}
	status, body = doJSON(t, clientA, http.MethodGet, serverURL+"/api/account/sessions", nil, &sessions)
	if status != http.StatusOK || sessions.Total != 1 || len(sessions.Items) != 1 || !sessions.Items[0].Current {
		t.Fatalf("expected only current session after revoke, status=%d body=%s sessions=%#v", status, body, sessions)
	}
}

func TestCanceledSessionLookupDoesNotClearValidCookie(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	cookie := sessionCookieFromJar(t, jar, serverURL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil).WithContext(ctx)
	req.AddCookie(cookie)

	_, _, ok, err := server.userForSessionTokenWithError(req, cookie.Value)
	if ok || !errors.Is(err, context.Canceled) {
		t.Fatalf("session lookup ok=%v err=%v, expected canceled transient failure", ok, err)
	}

	rec := httptest.NewRecorder()
	if user, ok := server.currentUser(rec, req); ok {
		t.Fatalf("currentUser unexpectedly authenticated canceled lookup as %#v", user)
	}
	for _, cleared := range rec.Result().Cookies() {
		if cleared.Name == cookie.Name && (cleared.Value == "" || cleared.MaxAge < 0) {
			t.Fatalf("canceled session lookup cleared valid session cookie: %#v", cleared)
		}
	}
}

func TestLocalLoginRateLimitsRepeatedFailures(t *testing.T) {
	serverURL := newAuthTestServer(t)
	client := http.DefaultClient
	for i := 0; i < 8; i++ {
		status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
			"login":    "admin",
			"password": "wrong-password",
		}, nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, body: %s", i, status, body)
		}
	}
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "admin",
		"password": "wrong-password",
	}, nil)
	if status != http.StatusTooManyRequests {
		t.Fatalf("rate-limited status = %d, body: %s", status, body)
	}
}

func TestLocalLoginDoesNotEnumerateAccounts(t *testing.T) {
	serverURL, _, _ := newAuthTestServerWithInstance(t)
	statusKnown, bodyKnown := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login": "admin", "password": "definitely-wrong",
	}, nil)
	statusUnknown, bodyUnknown := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login": "missing-account@example.test", "password": "definitely-wrong",
	}, nil)
	if statusKnown != http.StatusUnauthorized || statusUnknown != http.StatusUnauthorized {
		t.Fatalf("login failure statuses known=%d unknown=%d", statusKnown, statusUnknown)
	}
	var knownProblem, unknownProblem map[string]any
	if err := json.Unmarshal([]byte(bodyKnown), &knownProblem); err != nil {
		t.Fatalf("decode known-account problem: %v", err)
	}
	if err := json.Unmarshal([]byte(bodyUnknown), &unknownProblem); err != nil {
		t.Fatalf("decode unknown-account problem: %v", err)
	}
	delete(knownProblem, "requestId")
	delete(unknownProblem, "requestId")
	if !reflect.DeepEqual(knownProblem, unknownProblem) || knownProblem["code"] != "bad_credentials" {
		t.Fatalf("login failure disclosed account existence: known=%s unknown=%s", bodyKnown, bodyUnknown)
	}
}

func TestLoginLimiterIsBoundedAndSubjectAware(t *testing.T) {
	server := &Server{loginAttempts: map[string][]time.Time{}}
	now := time.Now().UTC()
	for index := 0; index < 4096; index++ {
		server.loginAttempts[fmt.Sprintf("existing-%d", index)] = []time.Time{now}
	}
	if server.allowLoginAttempt("new-rotating-subject") {
		t.Fatal("saturated login limiter admitted a rotating subject")
	}
	if len(server.loginAttempts) != 4096 {
		t.Fatalf("login limiter size = %d, want 4096", len(server.loginAttempts))
	}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	request.RemoteAddr = "192.168.1.20:42000"
	first := loginRateKey("browser-login", request, "first@example.test")
	second := loginRateKey("browser-login", request, "second@example.test")
	if first == second || strings.Contains(first, "first@example.test") || strings.Contains(second, "second@example.test") {
		t.Fatalf("login limiter keys are not subject-aware and privacy-safe: %q %q", first, second)
	}
	server.loginAttempts = map[string][]time.Time{"expired": {now.Add(-time.Hour)}}
	server.loginLimiterLastPrune = time.Time{}
	if !server.allowLoginAttempt("fresh") || len(server.loginAttempts) != 0 {
		t.Fatalf("expired limiter entries were not pruned: %#v", server.loginAttempts)
	}
}

func TestAuthCapabilitiesDoNotExposeVisibleUsers(t *testing.T) {
	serverURL := newAuthTestServer(t)
	var capabilities AuthCapabilitiesResponse
	status, body := doJSON(t, http.DefaultClient, http.MethodGet, serverURL+"/api/auth/capabilities", nil, &capabilities)
	if status != http.StatusOK {
		t.Fatalf("auth capabilities status = %d, body: %s", status, body)
	}
	if capabilities.SetupRequired || !capabilities.LocalCredentialsEnabled || capabilities.ServerFriendlyName == "" || capabilities.PublicUserPickerEnabled || len(capabilities.VisibleUsers) != 0 || capabilities.GeneratedAt == "" {
		t.Fatalf("unexpected auth capabilities: %#v", capabilities)
	}
}

func TestForegroundBootstrapRoutesTimeoutWhenSQLitePoolStalls(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	server.MarkHTTPReady(serverURL)

	jar, _ := cookiejar.New(nil)
	authenticatedClient := &http.Client{Jar: jar, Timeout: 7 * time.Second}
	loginUser(t, authenticatedClient, serverURL)

	server.db.SetMaxIdleConns(0)
	server.db.SetMaxOpenConns(1)
	conn, err := server.db.Conn(context.Background())
	if err != nil {
		t.Fatalf("hold database connection: %v", err)
	}
	defer conn.Close()
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin held transaction: %v", err)
	}
	defer tx.Rollback()

	readinessClient := &http.Client{Timeout: 3 * time.Second}
	readinessStarted := time.Now()
	var readiness ReadinessResponse
	status, body := doJSON(t, readinessClient, http.MethodGet, serverURL+"/api/readiness", nil, &readiness)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("readiness under DB stall status = %d, body: %s", status, body)
	}
	if time.Since(readinessStarted) > 2500*time.Millisecond {
		t.Fatalf("readiness took too long under DB stall: %s", time.Since(readinessStarted))
	}
	if readiness.Ready || readiness.Status != "unavailable" {
		t.Fatalf("readiness unexpectedly ready under DB stall: %#v", readiness)
	}

	anonymousClient := &http.Client{Timeout: 7 * time.Second}
	status, body = doJSON(t, anonymousClient, http.MethodGet, serverURL+"/api/auth/me", nil, nil)
	if status != http.StatusGatewayTimeout {
		t.Fatalf("anonymous auth bootstrap under DB stall status = %d, body: %s", status, body)
	}
	if !strings.Contains(body, "request_timeout") {
		t.Fatalf("anonymous auth bootstrap did not return retryable timeout body: %s", body)
	}

	for _, endpoint := range []string{"/api/home", "/api/libraries"} {
		status, body = doJSON(t, authenticatedClient, http.MethodGet, serverURL+endpoint, nil, nil)
		if status != http.StatusGatewayTimeout {
			t.Fatalf("%s under DB stall status = %d, body: %s", endpoint, status, body)
		}
		if !strings.Contains(body, "request_timeout") {
			t.Fatalf("%s did not return retryable timeout body: %s", endpoint, body)
		}
	}
}

func TestPorticoAccountLocalLoginRedirectCallbackCreatesSession(t *testing.T) {
	var exchanged bool
	var exchangeAttempts int
	var introspected bool
	const installationID = "portico-browser-installation-0001"
	now := time.Now().UTC().Truncate(time.Second)
	profile := HostedProfileSnapshot{
		ExternalProfileID: "prf_usr_local", AccountID: "usr_local", DisplayName: "Portico User",
		IsPrimary: true, IsAccountAdmin: true, SortOrder: 0, PolicyUpdatedAt: now.Add(-time.Minute),
		Restrictions: defaultProfileRestrictions(),
	}
	rawEnvelope := signedHostedProfileSelectionEnvelope(t, testHostedDocumentPrivateKey(), HostedProfileSelectionEnvelope{
		Version: hostedProfileSelectionAssertionVersion, AssertionID: "psa_local_callback", Audience: hostedDocumentAudience,
		AccountID: "usr_local", ProfileID: profile.ExternalProfileID, ServerID: "srv_local", DeviceID: "dev_portico_callback", InstallationID: installationID,
		AccountRevision: 1, PINRevision: 0, Profiles: []HostedProfileSnapshot{profile},
		IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(2 * time.Minute).Format(time.RFC3339Nano),
		SignatureAlgorithm: hostedSignatureAlgorithm, SignatureKeyID: testHostedDocumentKeyID,
	})
	var selectionEnvelope HostedProfileSelectionEnvelope
	if err := json.Unmarshal(rawEnvelope, &selectionEnvelope); err != nil {
		t.Fatalf("decode profile selection envelope: %v", err)
	}
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer ptc_srv_local" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"Server credential is required."}}`))
			return
		}
		switch r.URL.Path {
		case "/api/servers/srv_local/local-login/exchange":
			exchangeAttempts++
			if exchangeAttempts == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"code":"hosted_unavailable","message":"Try again."}}`))
				return
			}
			var req struct {
				Code string `json:"code"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Code != "portico-code" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"invalid_code","message":"Invalid code."}}`))
				return
			}
			exchanged = true
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "ptc_clt_local", "expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339), "selectionEnvelope": selectionEnvelope})
		case "/api/servers/srv_local/portico-sessions/introspect":
			var req struct {
				AccessToken       string                         `json:"accessToken"`
				SelectionEnvelope HostedProfileSelectionEnvelope `json:"selectionEnvelope"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.AccessToken != "ptc_clt_local" || req.SelectionEnvelope.AssertionID != selectionEnvelope.AssertionID {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"invalid_portico_session","message":"Invalid token."}}`))
				return
			}
			introspected = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active":            true,
				"deviceId":          "dev_portico_callback",
				"selectionEnvelope": selectionEnvelope,
				"member": map[string]any{
					"id":                  "mem_local",
					"porticoMembershipId": "mem_local",
					"porticoUserId":       "usr_local",
					"userId":              "usr_local",
					"serverId":            "srv_local",
					"email":               "portico-user@example.test",
					"displayName":         "Portico User",
					"role":                "user",
					"status":              "active",
					"createdAt":           time.Now().UTC().Format(time.RFC3339),
				},
			})
		case "/api/servers/srv_local/profile-selection-exchanges":
			var req struct {
				SelectionEnvelope HostedProfileSelectionEnvelope `json:"selectionEnvelope"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SelectionEnvelope.AssertionID != selectionEnvelope.AssertionID {
				t.Fatalf("unexpected profile exchange request: %#v err=%v", req, err)
			}
			writeJSON(w, http.StatusOK, req.SelectionEnvelope)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"Not found."}}`))
		}
	}))
	t.Cleanup(hosted.Close)

	serverURL, db, server := newAuthTestServerWithInstance(t)
	server.cfg.HostedDocumentPublicKeys = testHostedDocumentPublicKeys()
	upsertJSONSetting(t, db, remoteAccessSettingsKey, map[string]any{
		"enabled":                 true,
		"hostedBaseUrl":           hosted.URL,
		"claimStatus":             "claimed",
		"serverId":                "srv_local",
		"assignedHostname":        "ptc-local.direct.getportico.tv",
		"preferredRemoteAuthMode": "portico",
	})
	if err := server.saveSecretSetting(remoteAccessCredentialKey, "ptc_srv_local"); err != nil {
		t.Fatalf("save local credential: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	startURL := serverURL + "/api/auth/portico/start?returnUrl=" + url.QueryEscape(serverURL+"/#/home") + "&installationId=" + url.QueryEscape(installationID)
	resp, err := client.Get(startURL)
	if err != nil {
		t.Fatalf("start Portico login: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("start status = %d", resp.StatusCode)
	}
	hostedURL, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse hosted redirect: %v", err)
	}
	fragmentParts := strings.SplitN(hostedURL.Fragment, "?", 2)
	if len(fragmentParts) != 2 || fragmentParts[0] != "/local-login" {
		t.Fatalf("unexpected hosted redirect fragment: %s", hostedURL.Fragment)
	}
	hostedParams, err := url.ParseQuery(fragmentParts[1])
	if err != nil {
		t.Fatalf("parse hosted params: %v", err)
	}
	if hostedParams.Get("serverId") != "srv_local" || hostedParams.Get("callbackUrl") == "" || hostedParams.Get("state") == "" || hostedParams.Get("installationId") != installationID {
		t.Fatalf("hosted redirect missing parameters: %s", resp.Header.Get("Location"))
	}

	callback, err := url.Parse(hostedParams.Get("callbackUrl"))
	if err != nil {
		t.Fatalf("parse callback: %v", err)
	}
	values := callback.Query()
	values.Set("code", "portico-code")
	values.Set("state", hostedParams.Get("state"))
	callback.RawQuery = values.Encode()
	resp, err = client.Get(callback.String())
	if err != nil {
		t.Fatalf("callback Portico login: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d", resp.StatusCode)
	}
	if location := resp.Header.Get("Location"); !strings.Contains(location, "porticoLogin=error") {
		t.Fatalf("transient callback did not return a retryable error: %s", location)
	}
	resp, err = client.Get(callback.String())
	if err != nil {
		t.Fatalf("retry callback Portico login: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("retry callback status = %d", resp.StatusCode)
	}
	if location := resp.Header.Get("Location"); !strings.Contains(location, "porticoLogin=success") {
		t.Fatalf("callback did not complete Portico sign-in: %s", location)
	}
	if exchangeAttempts != 2 || !exchanged || !introspected {
		t.Fatalf("hosted exchange=%v introspect=%v", exchanged, introspected)
	}

	var auth AuthMeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/auth/me", nil, &auth)
	if status != http.StatusOK || !auth.Authenticated || auth.User == nil || auth.User.Email != "portico-user@example.test" || auth.AuthProvider != "portico" {
		t.Fatalf("auth after Portico callback status=%d body=%s decoded=%#v", status, body, auth)
	}
}

func TestPorticoLoginStartKeepsRequestOriginDespitePersistedPublicReachability(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	upsertJSONSetting(t, db, remoteAccessSettingsKey, map[string]any{
		"enabled":                 true,
		"hostedBaseUrl":           "https://accounts.getportico.tv",
		"claimStatus":             "claimed",
		"serverId":                "srv_public_callback",
		"assignedHostname":        "ptc-public-callback.direct.getportico.tv",
		"preferredRemoteAuthMode": "portico",
		"manualPublicPort":        32500,
		"certificateStatus":       "valid",
		"lastReachabilityResult":  "public_reachable",
	})
	if err := server.saveSecretSetting(remoteAccessCredentialKey, "ptc_srv_public_callback"); err != nil {
		t.Fatalf("save public callback credential: %v", err)
	}

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(serverURL + "/api/auth/portico/start")
	if err != nil {
		t.Fatalf("start Portico login: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("start status=%d", response.StatusCode)
	}
	hostedURL, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	fragmentParts := strings.SplitN(hostedURL.Fragment, "?", 2)
	if len(fragmentParts) != 2 {
		t.Fatalf("unexpected Hosted login URL: %s", hostedURL)
	}
	parameters, err := url.ParseQuery(fragmentParts[1])
	if err != nil {
		t.Fatal(err)
	}
	wantOrigin := serverURL
	if got := parameters.Get("localOrigin"); got != wantOrigin {
		t.Fatalf("localOrigin=%q, want transaction request origin %q", got, wantOrigin)
	}
	if got := parameters.Get("callbackUrl"); got != wantOrigin+"/api/auth/portico/callback" {
		t.Fatalf("callbackUrl=%q, want same-origin callback", got)
	}
}

func TestPorticoHostedWebBasePreservesValidatedBetaEnvironment(t *testing.T) {
	if got := porticoHostedWebBaseURL("https://beta-web.getportico.tv"); got != "https://beta-web.getportico.tv" {
		t.Fatalf("beta hosted origin was forced to production: %q", got)
	}
	if got := porticoHostedWebBaseURL("https://api.getportico.tv"); got != "https://web.getportico.tv" {
		t.Fatalf("canonical API did not map to production web: %q", got)
	}
}

func TestLocalNativeSessionUsesBoundedCredentialsWithoutChangingBrowserLogin(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	browserClient := &http.Client{Jar: jar}

	var browserAuth AuthMeResponse
	status, body := doJSON(t, browserClient, http.MethodPost, serverURL+"/api/auth/login", map[string]any{
		"login":    "admin",
		"password": "Password1234",
	}, &browserAuth)
	if status != http.StatusOK {
		t.Fatalf("browser login status = %d, body: %s", status, body)
	}
	if !browserAuth.Authenticated || browserAuth.User == nil {
		t.Fatalf("browser login response = %#v", browserAuth)
	}
	browserCookie := sessionCookieFromJar(t, jar, serverURL)
	assertSessionTTLAtLeast(t, db, browserCookie.Value, browserSessionTTL-time.Minute)
	assertSessionTTLAtMost(t, db, browserCookie.Value, browserSessionTTL+time.Minute)

	var credentials NativeSessionCredentials
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/sessions", NativeSessionCreateRequest{
		Login: "admin", Password: "Password1234", InstallationID: "test-installation-0001",
		DeviceName: "Test Phone", App: "Portico Test", Platform: "iOS",
	}, &credentials)
	if status != http.StatusCreated {
		t.Fatalf("native login status = %d, body: %s", status, body)
	}
	if credentials.AccessToken == "" || credentials.RefreshToken == "" || credentials.User.Username != "admin" {
		t.Fatalf("native credentials = %#v", credentials)
	}
	assertSessionTTLAtMost(t, db, credentials.AccessToken, nativeAccessTokenTTL+time.Minute)

	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create auth/me request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send auth/me request: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read auth/me response: %v", err)
	}
	var auth AuthMeResponse
	if err := json.Unmarshal(responseBody, &auth); err != nil {
		t.Fatalf("decode auth/me response: %v\n%s", err, responseBody)
	}
	if resp.StatusCode != http.StatusOK || !auth.Authenticated || auth.User == nil || auth.User.Username != "admin" {
		t.Fatalf("unexpected native auth response status=%d body=%#v raw=%s", resp.StatusCode, auth, responseBody)
	}

	streamReq, err := http.NewRequest(http.MethodGet, serverURL+"/api/media/missing/hls/master.m3u8?access_token="+url.QueryEscape(credentials.AccessToken), nil)
	if err != nil {
		t.Fatalf("create query-token stream request: %v", err)
	}
	streamResp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("send query-token stream request: %v", err)
	}
	_ = streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("query-token stream status = %d, expected long-lived query credential rejection", streamResp.StatusCode)
	}

	disallowedReq, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me?access_token="+url.QueryEscape(credentials.AccessToken), nil)
	if err != nil {
		t.Fatalf("create disallowed query-token request: %v", err)
	}
	disallowedResp, err := http.DefaultClient.Do(disallowedReq)
	if err != nil {
		t.Fatalf("send disallowed query-token request: %v", err)
	}
	var disallowedAuth AuthMeResponse
	if err := json.NewDecoder(disallowedResp.Body).Decode(&disallowedAuth); err != nil {
		t.Fatalf("decode disallowed query-token response: %v", err)
	}
	_ = disallowedResp.Body.Close()
	if disallowedResp.StatusCode != http.StatusOK || disallowedAuth.Authenticated {
		t.Fatalf("disallowed query-token auth status = %d body=%#v", disallowedResp.StatusCode, disallowedAuth)
	}

	var rotated NativeSessionCredentials
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/sessions/refresh", NativeSessionRefreshRequest{RefreshToken: credentials.RefreshToken, RotationKey: strings.Repeat("A", 43)}, &rotated)
	if status != http.StatusOK || rotated.AccessToken == credentials.AccessToken || rotated.RefreshToken == credentials.RefreshToken {
		t.Fatalf("native refresh status=%d body=%s rotated=%#v", status, body, rotated)
	}
}

func assertSessionTTLAtMost(t *testing.T, db *sql.DB, token string, maximum time.Duration) {
	t.Helper()
	var rawExpires string
	if err := db.QueryRow(`SELECT expires_at FROM sessions WHERE token_hash = ?`, hashToken(token)).Scan(&rawExpires); err != nil {
		t.Fatalf("query session expiry: %v", err)
	}
	expires, err := parseCredentialTime(rawExpires)
	if err != nil {
		t.Fatalf("parse session expiry %q: %v", rawExpires, err)
	}
	if remaining := time.Until(expires); remaining <= 0 || remaining > maximum {
		t.Fatalf("session TTL = %v, expected within (0,%v]", remaining, maximum)
	}
}

func assertSessionTTLAtLeast(t *testing.T, db *sql.DB, token string, minimum time.Duration) {
	t.Helper()
	var rawExpires string
	if err := db.QueryRow(`SELECT expires_at FROM sessions WHERE token_hash = ?`, hashToken(token)).Scan(&rawExpires); err != nil {
		t.Fatalf("query session expiry: %v", err)
	}
	expires, err := time.Parse(time.RFC3339, rawExpires)
	if err != nil {
		t.Fatalf("parse session expiry %q: %v", rawExpires, err)
	}
	if time.Until(expires) < minimum {
		t.Fatalf("session expires too soon: %s", expires.Format(time.RFC3339))
	}
}

func TestAPIKeysCanBeCreatedScopedAndRevoked(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var created APIKeyCreateResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/api-keys", APIKeyCreateRequest{
		Name:   "Automation",
		Scopes: []string{"read", "playMedia"},
	}, &created)
	if status != http.StatusCreated {
		t.Fatalf("create api key status = %d, body: %s", status, body)
	}
	if !strings.HasPrefix(created.Token, "ptc_api_") || created.Key.LastFour == "" || created.Key.LastFour != created.Token[len(created.Token)-4:] {
		t.Fatalf("unexpected API key create response: %#v", created)
	}
	if !apiKeyScopesContain(created.Key.Scopes, "playMedia") {
		t.Fatalf("expected playMedia scope in %#v", created.Key.Scopes)
	}

	var keys ListResponse[APIKey]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/auth/api-keys", nil, &keys)
	if status != http.StatusOK || keys.Total != 1 || keys.Items[0].ID != created.Key.ID {
		t.Fatalf("list api keys status=%d keys=%#v body=%s", status, keys, body)
	}
	if strings.Contains(body, created.Token) {
		t.Fatalf("list response exposed raw API key token")
	}

	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/libraries", nil)
	if err != nil {
		t.Fatalf("create library request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+created.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send library request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("libraries with API key status = %d", resp.StatusCode)
	}

	var readOnly APIKeyCreateResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/api-keys", APIKeyCreateRequest{Name: "Read Only", Scopes: []string{"read"}}, &readOnly)
	if status != http.StatusCreated {
		t.Fatalf("create read-only api key status = %d, body: %s", status, body)
	}
	req, err = http.NewRequest(http.MethodGet, serverURL+"/api/system/diagnostics", nil)
	if err != nil {
		t.Fatalf("create read-only diagnostics request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+readOnly.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send read-only diagnostics request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read-only diagnostics status = %d, expected forbidden", resp.StatusCode)
	}
	req, err = http.NewRequest(http.MethodGet, serverURL+"/api/home", nil)
	if err != nil {
		t.Fatalf("create read-only home request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+readOnly.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send read-only home request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read-only home status = %d, expected OK", resp.StatusCode)
	}
	req, err = http.NewRequest(http.MethodPost, serverURL+"/api/playback-sessions", strings.NewReader(`{"mediaId":"movie_meridian"}`))
	if err != nil {
		t.Fatalf("create read-only playback request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+readOnly.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send read-only playback request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read-only playback status = %d, expected forbidden", resp.StatusCode)
	}

	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/auth/api-keys/"+created.Key.ID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("revoke api key status = %d, body: %s", status, body)
	}
	req, err = http.NewRequest(http.MethodGet, serverURL+"/api/system/diagnostics", nil)
	if err != nil {
		t.Fatalf("create revoked diagnostics request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+created.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send revoked diagnostics request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked diagnostics status = %d, expected unauthorized", resp.StatusCode)
	}
}

func TestAPIKeyMutationsRequireRecentInteractiveAuthentication(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	stale := time.Now().UTC().Add(-apiKeyRecentAuthenticationWindow - time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE sessions SET created_at = ? WHERE user_id = (SELECT id FROM users WHERE role = 'owner')`, stale); err != nil {
		t.Fatalf("age owner session: %v", err)
	}
	var rejected APIKeyCreateResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/api-keys", APIKeyCreateRequest{Name: "Stale Session", Scopes: []string{"read"}}, &rejected)
	if status != http.StatusUnauthorized || !strings.Contains(body, "recent_reauthentication_required") || rejected.Token != "" {
		t.Fatalf("stale session API key create status=%d body=%s response=%#v", status, body, rejected)
	}
	var keyCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM api_keys`).Scan(&keyCount); err != nil || keyCount != 0 {
		t.Fatalf("stale session minted API key: count=%d err=%v", keyCount, err)
	}

	if _, err := db.Exec(`UPDATE sessions SET created_at = ? WHERE user_id = (SELECT id FROM users WHERE role = 'owner')`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("refresh owner session evidence: %v", err)
	}
	var created APIKeyCreateResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/api-keys", APIKeyCreateRequest{Name: "Recent Session", Scopes: []string{"read"}}, &created)
	if status != http.StatusCreated || created.Key.ID == "" {
		t.Fatalf("recent session API key create status=%d body=%s response=%#v", status, body, created)
	}

	if _, err := db.Exec(`UPDATE sessions SET created_at = ? WHERE user_id = (SELECT id FROM users WHERE role = 'owner')`, stale); err != nil {
		t.Fatalf("re-age owner session: %v", err)
	}
	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/auth/api-keys/"+created.Key.ID, nil, nil)
	if status != http.StatusUnauthorized || !strings.Contains(body, "recent_reauthentication_required") {
		t.Fatalf("stale session API key revoke status=%d body=%s", status, body)
	}
	var revokedAt string
	if err := db.QueryRow(`SELECT revoked_at FROM api_keys WHERE id = ?`, created.Key.ID).Scan(&revokedAt); err != nil || revokedAt != "" {
		t.Fatalf("stale session revoked API key: revokedAt=%q err=%v", revokedAt, err)
	}
}

func TestAPIKeyRequestsAreRateLimited(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var created APIKeyCreateResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/api-keys", APIKeyCreateRequest{
		Name:   "Heavy Integration",
		Scopes: []string{"read"},
	}, &created)
	if status != http.StatusCreated {
		t.Fatalf("create api key status = %d, body: %s", status, body)
	}

	limited := false
	for i := 0; i < 610; i++ {
		req, err := http.NewRequest(http.MethodGet, serverURL+"/api/home", nil)
		if err != nil {
			t.Fatalf("create api key request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+created.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("send api key request: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			if resp.Header.Get("Retry-After") == "" {
				t.Fatalf("rate-limited response missing Retry-After")
			}
			limited = true
			break
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("api key request %d status = %d", i, resp.StatusCode)
		}
	}
	if !limited {
		t.Fatalf("expected api key request rate limit")
	}
}

func TestLocalLoginReportsThisServerIdentityContext(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	var auth AuthMeResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/login", map[string]any{
		"login":    "admin",
		"password": "Password1234",
	}, &auth)
	if status != http.StatusOK {
		t.Fatalf("login status = %d, body: %s", status, body)
	}
	if auth.AccountMode != "this_server" || auth.AuthProvider != "local" || auth.ProfileID == "" || auth.ProfileIdentityID == "" {
		t.Fatalf("unexpected login auth context: %#v", auth)
	}
	if auth.User == nil || len(auth.User.SignInMethods) != 1 || auth.User.SignInMethods[0].Provider != "local" || auth.User.SignInMethods[0].Label != "This Server" {
		t.Fatalf("unexpected sign-in methods: %#v", auth.User)
	}
	loginServerID := auth.ServerID
	if loginServerID == "" || auth.ServerFriendlyName == "" {
		t.Fatalf("expected login auth response to include local server identity: %#v", auth)
	}

	auth = AuthMeResponse{}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/auth/me", nil, &auth)
	if status != http.StatusOK {
		t.Fatalf("auth/me status = %d, body: %s", status, body)
	}
	if auth.AccountMode != "this_server" || auth.AuthProvider != "local" || auth.ProfileID == "" || auth.ProfileIdentityID == "" {
		t.Fatalf("unexpected session auth context: %#v", auth)
	}
	if auth.ServerID != loginServerID || auth.ServerFriendlyName == "" {
		t.Fatalf("expected auth/me to preserve local server identity %q: %#v", loginServerID, auth)
	}
}

func TestThisServerOnlySetupRequiresAcknowledgementAndCreatesProfile(t *testing.T) {
	serverURL, db := newEmptyAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/setup", map[string]any{
		"serverName":  "Family Portico",
		"username":    "localowner",
		"email":       "owner@example.test",
		"displayName": "Local Owner",
		"password":    "LocalOwner123!",
		"setupMode":   "local_only",
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "local_only_acknowledgement_required") {
		t.Fatalf("local-only setup without acknowledgement status=%d body=%s", status, body)
	}

	var auth AuthMeResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/setup", map[string]any{
		"serverName":            "Family Portico",
		"username":              "localowner",
		"email":                 "",
		"displayName":           "Local Owner",
		"password":              "LocalOwner123!",
		"setupMode":             "local_only",
		"localOnlyAcknowledged": true,
	}, &auth)
	if status != http.StatusCreated || !auth.Authenticated || auth.User == nil {
		t.Fatalf("local-only setup status=%d body=%s auth=%#v", status, body, auth)
	}
	if auth.AccountMode != "this_server" || auth.AuthProvider != "local" || auth.User.AuthOrigin != "local" || !auth.User.HasLocalPassword {
		t.Fatalf("unexpected local-only auth context: %#v", auth)
	}
	if auth.User.Email != "" {
		t.Fatalf("optional owner email = %q, want empty", auth.User.Email)
	}
	if auth.ServerFriendlyName != "Family Portico" {
		t.Fatalf("server friendly name = %q, want setup name", auth.ServerFriendlyName)
	}
	assertLocalProfileCredential(t, db, auth.User.ID)
}

func TestLocalAccountCreationUsesProfileIdentityModel(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var user User
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/users", map[string]any{
		"username":    "localviewer",
		"email":       "viewer@example.test",
		"displayName": "Local Viewer",
		"password":    "LocalViewer123!",
		"permissions": map[string]bool{"playMedia": true},
		"libraryIds":  []string{},
	}, &user)
	if status != http.StatusCreated {
		t.Fatalf("create local account status=%d body=%s", status, body)
	}
	if user.AuthOrigin != "local" || !user.HasLocalPassword || len(user.SignInMethods) != 1 || user.SignInMethods[0].Provider != "local" {
		t.Fatalf("unexpected local account auth context: %#v", user)
	}
	assertLocalProfileCredential(t, db, user.ID)
}

func TestLocalUserMutationsAreBlockedInPorticoAccountMode(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	if _, err := db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'remoteAccess'`, `{"enabled":true,"claimStatus":"claimed","serverId":"srv_portico","preferredRemoteAuthMode":"portico","hostedBaseUrl":"https://api.getportico.tv"}`); err != nil {
		t.Fatalf("set remote access mode: %v", err)
	}

	var auth AuthMeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/auth/me", nil, &auth)
	if status != http.StatusOK || auth.Authenticated {
		t.Fatalf("stale local session remained active in Portico Account mode: status=%d body=%s auth=%#v", status, body, auth)
	}

	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/login", map[string]any{
		"login":    "admin",
		"password": "Password1234",
	}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("local login in Portico Account mode status=%d body=%s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/users", map[string]any{
		"username":    "blocked",
		"email":       "blocked@example.test",
		"displayName": "Blocked",
		"password":    "Blocked123!",
		"role":        "user",
	}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("Portico-account-mode local create status=%d body=%s", status, body)
	}
}

func newAuthTestServer(t *testing.T) string {
	serverURL, _ := newAuthTestServerWithDB(t)
	return serverURL
}

func newAuthTestServerWithDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	serverURL, db, _ := newEmptyAuthTestServerWithInstance(t)

	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/setup", map[string]string{
		"serverName":  "Auth Test Server",
		"username":    "admin",
		"email":       "admin@example.test",
		"displayName": "Admin",
		"password":    "Password1234",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("setup status = %d, body: %s", status, body)
	}

	return serverURL, db
}

func newAuthTestServerWithInstance(t *testing.T) (string, *sql.DB, *Server) {
	t.Helper()
	serverURL, db, server := newEmptyAuthTestServerWithInstance(t)

	status, body := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/setup", map[string]string{
		"serverName":  "Auth Test Server",
		"username":    "admin",
		"email":       "admin@example.test",
		"displayName": "Admin",
		"password":    "Password1234",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("setup status = %d, body: %s", status, body)
	}

	return serverURL, db, server
}

func newEmptyAuthTestServer(t *testing.T) (string, *sql.DB) {
	serverURL, db, _ := newEmptyAuthTestServerWithInstance(t)
	return serverURL, db
}

func newEmptyAuthTestServerWithInstance(t *testing.T) (string, *sql.DB, *Server) {
	t.Helper()
	chdirRepoRoot(t)

	appDataDir := t.TempDir()
	cfg := config.Config{
		Addr:           "127.0.0.1:0",
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
		AllowedOrigins: []string{"https://web.getportico.tv"},
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	s := &Server{
		cfg:            cfg,
		db:             db,
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		logSubscribers: map[chan LogEvent]bool{},
		scannerWatch:   map[string]string{},
		transcodes:     map[string]*transcodeSession{},
	}
	testServer := httptest.NewServer(s.Handler())
	t.Cleanup(testServer.Close)
	return testServer.URL, db, s
}

func assertLocalProfileCredential(t *testing.T, db *sql.DB, profileID string) {
	t.Helper()
	var profileCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM profiles WHERE id = ?`, profileID).Scan(&profileCount); err != nil {
		t.Fatalf("query profile: %v", err)
	}
	if profileCount != 1 {
		t.Fatalf("expected profile for %s, got %d", profileID, profileCount)
	}
	var identityID string
	if err := db.QueryRow(`SELECT id FROM profile_identities WHERE profile_id = ? AND provider = 'local'`, profileID).Scan(&identityID); err != nil {
		t.Fatalf("query local identity: %v", err)
	}
	var credentialCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM local_credentials WHERE profile_identity_id = ? AND revoked_at = ''`, identityID).Scan(&credentialCount); err != nil {
		t.Fatalf("query local credential: %v", err)
	}
	if credentialCount != 1 {
		t.Fatalf("expected active local credential for identity %s, got %d", identityID, credentialCount)
	}
}

func upsertJSONSetting(t *testing.T, db *sql.DB, key string, value any) {
	t.Helper()
	bytes, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal setting %s: %v", key, err)
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, key, string(bytes), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("upsert setting %s: %v", key, err)
	}
}

func chdirRepoRoot(t *testing.T) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	root := filepath.Clean(filepath.Join(previous, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("locate repo root from %s: %v", previous, err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change to repo root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})
}

func loginUser(t *testing.T, client *http.Client, serverURL string) {
	t.Helper()
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "admin",
		"password": "Password1234",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("login status = %d, body: %s", status, body)
	}
}

func assertAuthenticated(t *testing.T, client *http.Client, serverURL string, expected bool) {
	t.Helper()
	var auth AuthMeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/auth/me", nil, &auth)
	if status != http.StatusOK {
		t.Fatalf("auth/me status = %d, body: %s", status, body)
	}
	if auth.Authenticated != expected {
		t.Fatalf("authenticated = %v, expected %v, body: %s", auth.Authenticated, expected, body)
	}
}

func assertBearerAuthenticated(t *testing.T, client *http.Client, serverURL, token string, expected bool) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create bearer auth request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send bearer auth request: %v", err)
	}
	defer resp.Body.Close()
	var auth AuthMeResponse
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		t.Fatalf("decode bearer auth response: %v", err)
	}
	if resp.StatusCode != http.StatusOK || auth.Authenticated != expected {
		t.Fatalf("bearer authenticated=%v expected=%v status=%d", auth.Authenticated, expected, resp.StatusCode)
	}
}

func apiKeyScopesContain(scopes []string, scope string) bool {
	for _, candidate := range scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func sessionCookieFromJar(t *testing.T, jar *cookiejar.Jar, rawURL string) *http.Cookie {
	t.Helper()
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	for _, cookie := range jar.Cookies(parsedURL) {
		if strings.HasPrefix(cookie.Name, sessionCookieName+"_") {
			return cookie
		}
	}
	t.Fatalf("session cookie was not set")
	return nil
}

func doJSON(t *testing.T, client *http.Client, method string, endpoint string, payload any, out any) (int, string) {
	t.Helper()

	var body io.Reader
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		body = bytes.NewReader(payloadBytes)
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		req.Header.Set(csrfHeaderName, "1")
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if out != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, out); err != nil {
			t.Fatalf("decode response body: %v\n%s", err, responseBody)
		}
	}
	return resp.StatusCode, string(responseBody)
}
