package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"
	"time"
)

func createProfileProtocolAccount(t *testing.T, server *Server) (User, AccountProfile) {
	return createProfileProtocolAccountNamed(t, server, "profile-protocol")
}

func createProfileProtocolAccountNamed(t *testing.T, server *Server, username string) (User, AccountProfile) {
	t.Helper()
	account, err := server.createUser(UserRequest{
		Username: username, Email: username + "@example.test", DisplayName: "Profile Household",
		Password: "Profile-protocol-password1", Role: "user", Permissions: map[string]bool{"playMedia": true, "downloadMedia": true},
	})
	if err != nil {
		t.Fatalf("create profile protocol account: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, account.ID, "2468"); err != nil {
		t.Fatalf("set primary PIN: %v", err)
	}
	restrictions := defaultProfileRestrictions()
	restrictions.AllowDownloads = false
	child, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{
		DisplayName: "Child", PIN: "1357", Restrictions: restrictions,
	})
	if err != nil {
		t.Fatalf("create child profile: %v", err)
	}
	return account, child
}

func TestProfileAuthenticationErrorsUseCanonicalProductLanguage(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name      string
		err       error
		status    int
		code      string
		messageID string
		detail    string
		retry     bool
	}{
		{name: "pin required", err: errInvalidProfilePIN, status: 400, code: "profile_pin_required", messageID: "auth.profile-pin-required", detail: "Enter the four-digit PIN for this profile."},
		{name: "pin delay", err: profilePINRetryError(errProfilePINBackoff, now.Add(2*time.Second), now), status: 429, code: "profile_pin_retry_later", messageID: "auth.profile-pin-retry-later", detail: "For your security, wait a moment before trying the profile PIN again.", retry: true},
		{name: "pin lock", err: profilePINRetryError(errProfilePINLocked, now.Add(time.Minute), now), status: 429, code: "profile_temporarily_locked", messageID: "auth.profile-temporarily-locked", detail: "Too many incorrect PIN attempts were made. Wait until the lock expires, then try again.", retry: true},
		{name: "profile missing", err: errProfileNotFound, status: 404, code: "profile_not_found", messageID: "auth.profile-not-found"},
		{name: "profile forbidden", err: errProfileNotAllowed, status: 403, code: "profile_not_available_on_server", messageID: "auth.profile-not-available"},
		{name: "selection invalid", err: errInvalidProfileSelectionGrant, status: 401, code: "profile_selection_failed", messageID: "auth.profile-selection-failed", detail: "Portico couldn't open this profile. Choose it again or try another profile."},
		{name: "selection replayed", err: errProfileSelectionGrantConsumed, status: 409, code: "profile_selection_failed", messageID: "auth.profile-selection-failed", detail: "Portico couldn't open this profile. Choose it again or try another profile."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			(&Server{}).writeProfileAuthenticationError(recorder, test.err)
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
			var problem map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if problem["code"] != test.code || problem["messageId"] != test.messageID {
				t.Fatalf("problem=%#v", problem)
			}
			if test.detail != "" && problem["detail"] != test.detail {
				t.Fatalf("detail=%q want=%q", problem["detail"], test.detail)
			}
			if (recorder.Header().Get("Retry-After") != "") != test.retry {
				t.Fatalf("Retry-After=%q retry=%v", recorder.Header().Get("Retry-After"), test.retry)
			}
		})
	}
}

func TestProfileWireShapesMatchCanonicalClientContract(t *testing.T) {
	directory := ProfileDirectory{
		Authority: "local", AccountID: "account-1", ServerID: "server-1", ProfilesAllowed: true,
		Profiles: []SelectableProfile{{
			ID: "profile-1", Name: "Primary", IsPrimary: true, IsAccountAdmin: true,
			HasPIN: true, PINRevision: 3, SortOrder: 0, Policy: defaultProfileRestrictions(),
		}},
	}
	selection := ProfileSelectionResponse{
		SelectionGrant: "grant", Authority: "local", AccountID: "account-1", ServerID: "server-1",
		ProfileID: "profile-1", PINRevision: 3, InstallationID: "installation-0001", ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),
	}
	for name, value := range map[string]any{"directory": directory, "selection": selection} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		for _, retired := range []string{`"avatarUrl"`, `"origin"`, `"hasPin"`, `"restrictions"`, `"selectionGrant"`} {
			if bytes.Contains(encoded, []byte(retired)) {
				t.Fatalf("%s leaked retired field %s: %s", name, retired, encoded)
			}
		}
	}
	encoded, _ := json.Marshal(directory.Profiles[0])
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode canonical profile: %v", err)
	}
	for _, required := range []string{"id", "name", "isPrimary", "isAccountAdmin", "hasPIN", "pinRevision", "sortOrder", "policy"} {
		if _, ok := fields[required]; !ok {
			t.Fatalf("canonical profile missing %q: %s", required, encoded)
		}
	}
}

func TestBrowserProfileGrantConcurrentRedemptionIsExactlyOnce(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	_, child := createProfileProtocolAccount(t, server)
	jar, _ := cookiejar.New(nil)
	original := &http.Client{Jar: jar}
	authentication := authenticateProfileAccountWithClientHTTP(t, original, serverURL, "browser", "concurrent-browser-install-0001")
	status, body, selection := selectProfileWithClientHTTP(t, original, serverURL, authentication.AccountAuthenticationToken, child.ID, "1357")
	if status != http.StatusCreated {
		t.Fatalf("create concurrent browser grant status=%d body=%s", status, body)
	}
	baseURL, err := url.Parse(serverURL + "/api/auth/profile-sessions/browser")
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	cookies := original.Jar.Cookies(baseURL)
	clients := make([]*http.Client, 2)
	for index := range clients {
		clientJar, _ := cookiejar.New(nil)
		clientJar.SetCookies(baseURL, cookies)
		clients[index] = &http.Client{Jar: clientJar}
	}
	payload, _ := json.Marshal(BrowserProfileSessionRequest{SelectionGrant: selection.SelectionGrant})
	statuses := make(chan int, len(clients))
	start := make(chan struct{})
	for _, client := range clients {
		go func(client *http.Client) {
			<-start
			request, _ := http.NewRequest(http.MethodPost, serverURL+"/api/auth/profile-sessions/browser", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			response, requestErr := client.Do(request)
			if requestErr != nil {
				statuses <- 0
				return
			}
			_ = response.Body.Close()
			statuses <- response.StatusCode
		}(client)
	}
	close(start)
	results := []int{<-statuses, <-statuses}
	sort.Ints(results)
	if results[0] != http.StatusCreated || results[1] != http.StatusConflict {
		t.Fatalf("concurrent browser redemption statuses=%v, want [201 409]", results)
	}
}

func authenticateProfileAccountHTTP(t *testing.T, serverURL string, purpose, installationID string) ProfileAccountAuthenticationResponse {
	return authenticateProfileAccountWithClientHTTP(t, http.DefaultClient, serverURL, purpose, installationID)
}

func authenticateProfileAccountWithClientHTTP(t *testing.T, client *http.Client, serverURL string, purpose, installationID string) ProfileAccountAuthenticationResponse {
	t.Helper()
	var response ProfileAccountAuthenticationResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/profile-authentications/local", LocalProfileAccountAuthenticationRequest{
		Login: "profile-protocol", Password: "Profile-protocol-password1", Purpose: purpose,
		ProfileDeviceDescriptor: ProfileDeviceDescriptor{
			InstallationID: installationID, DeviceName: "Profile Test", App: "Portico Test", Platform: "TestOS",
		},
	}, &response)
	if status != http.StatusCreated {
		t.Fatalf("profile account authentication status=%d body=%s", status, body)
	}
	if response.AccountAuthenticationToken == "" || response.Directory.Authority != "local" || response.Directory.AccountID == "" || response.Directory.ServerID == "" || len(response.Directory.Profiles) == 0 || response.Directory.Profiles[0].SortOrder != 0 {
		t.Fatalf("profile account authentication response=%#v", response)
	}
	for index, profile := range response.Directory.Profiles {
		if profile.SortOrder != index {
			t.Fatalf("profile directory is not contiguous: %#v", response.Directory.Profiles)
		}
	}
	return response
}

func selectProfileHTTP(t *testing.T, serverURL, accountToken, profileID, pin string) (int, string, ProfileSelectionResponse) {
	return selectProfileWithClientHTTP(t, http.DefaultClient, serverURL, accountToken, profileID, pin)
}

func selectProfileWithClientHTTP(t *testing.T, client *http.Client, serverURL, accountToken, profileID, pin string) (int, string, ProfileSelectionResponse) {
	t.Helper()
	var response ProfileSelectionResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/profile-selections/local", LocalProfileSelectionRequest{
		AccountAuthenticationToken: accountToken, ProfileID: profileID, PIN: pin,
	}, &response)
	return status, body, response
}

func TestLocalNativeProfileAuthenticationSelectionAndSessionProtocol(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	account, child := createProfileProtocolAccount(t, server)
	installationID := "profile-native-installation-0001"
	authentication := authenticateProfileAccountHTTP(t, serverURL, "native", installationID)

	status, _, _ := selectProfileHTTP(t, serverURL, authentication.AccountAuthenticationToken, child.ID, "9999")
	if status != http.StatusTooManyRequests {
		t.Fatalf("wrong PIN status=%d, want 429 with inter-attempt delay", status)
	}
	if _, err := db.Exec(`UPDATE local_profile_pin_credentials SET next_attempt_at = '' WHERE profile_id = ?`, child.ID); err != nil {
		t.Fatalf("advance profile PIN retry window: %v", err)
	}
	status, body, selection := selectProfileHTTP(t, serverURL, authentication.AccountAuthenticationToken, child.ID, "1357")
	if status != http.StatusCreated || selection.SelectionGrant == "" || selection.ProfileID != child.ID {
		t.Fatalf("profile selection status=%d body=%s response=%#v", status, body, selection)
	}
	if selection.Authority != "local" || selection.AccountID != account.ID || selection.ServerID != authentication.Directory.ServerID ||
		selection.PINRevision != child.PINRevision || selection.InstallationID != installationID {
		t.Fatalf("profile selection grant scope=%#v directory=%#v", selection, authentication.Directory)
	}
	var storedAuthenticationToken string
	if err := db.QueryRow(`SELECT token_hash FROM profile_account_authentications WHERE account_id = ?`, child.AccountID).Scan(&storedAuthenticationToken); err != nil {
		t.Fatalf("read stored profile account authentication: %v", err)
	}
	if storedAuthenticationToken == authentication.AccountAuthenticationToken {
		t.Fatal("profile account authentication token was stored in plaintext")
	}
	var failedAttempts int
	if err := db.QueryRow(`SELECT failed_attempts FROM local_profile_pin_credentials WHERE profile_id = ?`, child.ID).Scan(&failedAttempts); err != nil || failedAttempts != 0 {
		t.Fatalf("successful PIN did not reset persisted failures for account %s: attempts=%d err=%v", account.ID, failedAttempts, err)
	}

	alternateInstallationMetadata := NativeProfileSessionRequest{
		SelectionGrant: selection.SelectionGrant,
		ProfileDeviceDescriptor: ProfileDeviceDescriptor{
			InstallationID: "different-installation-0002", DeviceName: "Profile Test", App: "Portico Test", Platform: "TestOS",
		},
	}
	var credentials NativeSessionCredentials
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/profile-sessions/native", alternateInstallationMetadata, &credentials)
	if status != http.StatusCreated || credentials.AccessToken == "" || credentials.User.ProfileID != child.ID || credentials.User.DisplayName != "Child" {
		t.Fatalf("optional installation metadata changed native profile finalization: status=%d body=%s credentials=%#v", status, body, credentials)
	}
	if credentials.Authority != "local" || credentials.AccountID != account.ID || credentials.ProfileID != child.ID || credentials.ServerID != selection.ServerID || credentials.AuthorizationRevision == "" {
		t.Fatalf("native final viewer scope=%#v selection=%#v", credentials, selection)
	}
	if credentials.User.Permissions["downloadMedia"] {
		t.Fatal("selected profile restrictions were not applied to native session")
	}

	var recoveredCredentials NativeSessionCredentials
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/profile-sessions/native", NativeProfileSessionRequest{
		SelectionGrant: selection.SelectionGrant,
		ProfileDeviceDescriptor: ProfileDeviceDescriptor{
			InstallationID: installationID, DeviceName: "Profile Test", App: "Portico Test", Platform: "TestOS",
		},
	}, &recoveredCredentials)
	if status != http.StatusCreated || recoveredCredentials.AccessToken != credentials.AccessToken || recoveredCredentials.RefreshToken != credentials.RefreshToken {
		t.Fatalf("profile exchange receipt did not recover exact credentials: status=%d body=%s recovered=%#v original=%#v", status, body, recoveredCredentials, credentials)
	}
}

func TestLocalBrowserProfileProtocolSupportsLockedPrimaryAndChildSelection(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	account, child := createProfileProtocolAccount(t, server)

	status, _ := doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/login", BrowserLoginRequest{
		Login: "profile-protocol", Password: "Profile-protocol-password1",
	}, nil)
	if status != http.StatusConflict {
		t.Fatalf("legacy browser login bypass status=%d, want profile-selection conflict", status)
	}
	status, _ = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/sessions", NativeSessionCreateRequest{
		Login: "profile-protocol", Password: "Profile-protocol-password1", InstallationID: "legacy-native-install-0001",
		DeviceName: "Legacy", App: "Portico", Platform: "TestOS",
	}, nil)
	if status != http.StatusConflict {
		t.Fatalf("legacy native login bypass status=%d, want profile-selection conflict", status)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	authentication := authenticateProfileAccountWithClientHTTP(t, client, serverURL, "browser", "profile-browser-install-0001")
	status, body, selection := selectProfileWithClientHTTP(t, client, serverURL, authentication.AccountAuthenticationToken, child.ID, "1357")
	if status != http.StatusCreated {
		t.Fatalf("select child status=%d body=%s", status, body)
	}
	stolenJar, _ := cookiejar.New(nil)
	stolenClient := &http.Client{Jar: stolenJar}
	status, _ = doJSON(t, stolenClient, http.MethodPost, serverURL+"/api/auth/profile-sessions/browser", BrowserProfileSessionRequest{SelectionGrant: selection.SelectionGrant}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("copied browser grant without binding cookie status=%d, want 401", status)
	}
	var auth AuthMeResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/profile-sessions/browser", BrowserProfileSessionRequest{
		SelectionGrant: selection.SelectionGrant,
	}, &auth)
	if status != http.StatusCreated || !auth.Authenticated || auth.Authority != "local" || auth.User == nil || auth.User.ID != account.ID || auth.AccountID != account.ID || auth.ProfileID != child.ID || auth.User.ProfileID != child.ID || auth.ServerID == "" || auth.AuthorizationRevision == "" {
		t.Fatalf("browser child session status=%d body=%s auth=%#v", status, body, auth)
	}
	bindingURL, _ := url.Parse(serverURL + "/api/auth/profile-sessions/browser")
	for _, cookie := range client.Jar.Cookies(bindingURL) {
		if cookie.Name == profileBrowserBindingCookieName {
			t.Fatal("successful browser finalization left the transient profile binding cookie behind")
		}
	}

	request, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create auth request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("authenticate browser profile session: %v", err)
	}
	defer response.Body.Close()
	var persisted AuthMeResponse
	if err := json.NewDecoder(response.Body).Decode(&persisted); err != nil {
		t.Fatalf("decode browser auth: %v", err)
	}
	if !persisted.Authenticated || persisted.Authority != "local" || persisted.AccountID != account.ID || persisted.ProfileID != child.ID || persisted.ServerID != auth.ServerID || persisted.AuthorizationRevision != auth.AuthorizationRevision {
		t.Fatalf("persisted browser profile session=%#v", persisted)
	}
}

func TestAccountProfilePolicyDisablesChildSelectionForLocalAuth(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	account, child := createProfileProtocolAccount(t, server)
	if _, err := db.Exec(`UPDATE users SET allow_account_profiles = 0 WHERE id = ?`, account.ID); err != nil {
		t.Fatalf("disable account profiles: %v", err)
	}
	authentication := authenticateProfileAccountHTTP(t, serverURL, "native", "profile-policy-install-0001")
	if authentication.Directory.ProfilesAllowed || len(authentication.Directory.Profiles) != 2 || !authentication.Directory.Profiles[0].IsPrimary {
		t.Fatalf("disabled profile directory=%#v", authentication.Directory)
	}
	status, _, _ := selectProfileHTTP(t, serverURL, authentication.AccountAuthenticationToken, child.ID, "1357")
	if status != http.StatusForbidden {
		t.Fatalf("disabled child selection status=%d, want 403", status)
	}
}

func TestProfileSelectionFailuresPersistBackoffAndTerminalLock(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	_, child := createProfileProtocolAccount(t, server)
	authentication := authenticateProfileAccountHTTP(t, serverURL, "native", "profile-lock-install-0001")
	for attempt := 1; attempt <= localProfilePINFailureLimit; attempt++ {
		status, _, _ := selectProfileHTTP(t, serverURL, authentication.AccountAuthenticationToken, child.ID, "9999")
		if status != http.StatusTooManyRequests {
			t.Fatalf("wrong PIN attempt %d status=%d, want 429", attempt, status)
		}
		if attempt < localProfilePINFailureLimit {
			if _, err := db.Exec(`UPDATE local_profile_pin_credentials SET next_attempt_at = '' WHERE profile_id = ?`, child.ID); err != nil {
				t.Fatalf("advance retry window %d: %v", attempt, err)
			}
		}
	}
	var failed int
	var lockedUntil string
	if err := db.QueryRow(`SELECT failed_attempts, locked_until FROM local_profile_pin_credentials WHERE profile_id = ?`, child.ID).Scan(&failed, &lockedUntil); err != nil {
		t.Fatalf("read persisted PIN lock: %v", err)
	}
	if failed != localProfilePINFailureLimit || lockedUntil == "" {
		t.Fatalf("persisted PIN state failed=%d lockedUntil=%q", failed, lockedUntil)
	}

	// A fresh account proof cannot bypass the profile-level lock, and the
	// response communicates the bounded retry window without exposing the PIN.
	fresh := authenticateProfileAccountHTTP(t, serverURL, "native", "profile-lock-install-0001")
	payload, _ := json.Marshal(LocalProfileSelectionRequest{
		AccountAuthenticationToken: fresh.AccountAuthenticationToken, ProfileID: child.ID, PIN: "1357",
	})
	request, err := http.NewRequest(http.MethodPost, serverURL+"/api/auth/profile-selections/local", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create locked PIN request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send locked PIN request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusTooManyRequests || response.Header.Get("Retry-After") == "" {
		t.Fatalf("locked PIN response status=%d retry-after=%q", response.StatusCode, response.Header.Get("Retry-After"))
	}
}

func TestProfileGrantPurposeCannotCrossBrowserAndNativeFinalization(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	_, child := createProfileProtocolAccount(t, server)
	installationID := "cross-purpose-install-0001"

	browserJar, _ := cookiejar.New(nil)
	browserClient := &http.Client{Jar: browserJar}
	browserAuth := authenticateProfileAccountWithClientHTTP(t, browserClient, serverURL, "browser", installationID)
	status, body, browserGrant := selectProfileWithClientHTTP(t, browserClient, serverURL, browserAuth.AccountAuthenticationToken, child.ID, "1357")
	if status != http.StatusCreated {
		t.Fatalf("create prior browser grant status=%d body=%s", status, body)
	}
	falseValue := false
	status, body = doJSON(t, browserClient, http.MethodPost, serverURL+"/api/auth/profile-sessions/browser", BrowserProfileSessionRequest{
		SelectionGrant: browserGrant.SelectionGrant, RememberBrowser: &falseValue,
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("consume prior browser grant status=%d body=%s", status, body)
	}

	nativeAuth := authenticateProfileAccountHTTP(t, serverURL, "native", installationID)
	status, body, nativeGrant := selectProfileHTTP(t, serverURL, nativeAuth.AccountAuthenticationToken, child.ID, "1357")
	if status != http.StatusCreated {
		t.Fatalf("create native grant status=%d body=%s", status, body)
	}
	status, _ = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/profile-sessions/browser", BrowserProfileSessionRequest{
		SelectionGrant: nativeGrant.SelectionGrant, RememberBrowser: &falseValue,
	}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("native grant used for browser finalization status=%d, want 401", status)
	}

	var credentials NativeSessionCredentials
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/profile-sessions/native", NativeProfileSessionRequest{
		SelectionGrant: nativeGrant.SelectionGrant,
		ProfileDeviceDescriptor: ProfileDeviceDescriptor{
			InstallationID: installationID, DeviceName: "Cross Purpose", App: "Portico Test", Platform: "TestOS",
		},
	}, &credentials)
	if status != http.StatusCreated || credentials.User.ProfileID != child.ID {
		t.Fatalf("native grant after rejected browser use status=%d body=%s credentials=%#v", status, body, credentials)
	}
}

func TestProfileAccountAuthenticationMigrationHasRequiredBindings(t *testing.T) {
	_, db, _ := newAuthTestServerWithInstance(t)
	columns := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(profile_account_authentications)`)
	if err != nil {
		t.Fatalf("inspect account authentication table: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan account authentication column: %v", err)
		}
		columns[name] = true
	}
	for _, required := range []string{"token_hash", "account_id", "auth_provider", "purpose", "device_id", "installation_id", "browser_binding_hash", "expires_at", "consumed_at"} {
		if !columns[required] {
			t.Fatalf("profile account authentication missing %q", required)
		}
	}
}
