package app

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRememberedLocalAccountReturnsBrowserBoundProfileChallenge(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginBrowserVaultUser(t, client, serverURL, "admin", "Password1234")
	ownerID := browserVaultTestUserID(t, server.db, "admin")
	if err := server.setLocalProfilePINContext(context.Background(), ownerID, ownerID, "2468"); err != nil {
		t.Fatalf("set primary PIN: %v", err)
	}
	child, err := server.createLocalProfileContext(context.Background(), ownerID, CreateLocalProfileInput{DisplayName: "Kids", PIN: "1357"})
	if err != nil {
		t.Fatalf("create child profile: %v", err)
	}
	if status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/logout", nil, nil); status != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", status, body)
	}

	var challenge ProfileAccountAuthenticationResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/browser-accounts/switch", BrowserAccountSwitchRequest{
		AccountID:               ownerID,
		ProfileDeviceDescriptor: ProfileDeviceDescriptor{InstallationID: "browser-installation-1", DeviceName: "Test browser", App: "portico-web", Platform: "web"},
	}, &challenge)
	if status != http.StatusOK || challenge.AccountAuthenticationToken == "" || len(challenge.Directory.Profiles) != 2 || challenge.Directory.Authority != "local" {
		t.Fatalf("remembered profile challenge status=%d body=%s challenge=%#v", status, body, challenge)
	}
	base, _ := url.Parse(serverURL)
	authBase, _ := url.Parse(serverURL + "/api/auth/")
	if optionalCookie(jar.Cookies(authBase), profileBrowserBindingCookieName) == nil {
		t.Fatal("remembered profile challenge omitted the browser-bound proof cookie")
	}
	lanContext := context.WithValue(context.Background(), requestTransportSecureKey{}, false)
	if cookie := optionalCookie(jar.Cookies(base), server.sessionCookieNameContext(lanContext)); cookie != nil && cookie.Value != "" {
		t.Fatalf("profile challenge created a viewer session before selection: %q", cookie.Value)
	}

	var selection ProfileSelectionResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/profile-selections/local", LocalProfileSelectionRequest{
		AccountAuthenticationToken: challenge.AccountAuthenticationToken,
		ProfileID:                  child.ID,
		PIN:                        "1357",
	}, &selection)
	if status != http.StatusCreated || selection.SelectionGrant == "" {
		t.Fatalf("select remembered profile status=%d body=%s selection=%#v", status, body, selection)
	}
	replayStatus, replayBody := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/profile-selections/local", LocalProfileSelectionRequest{
		AccountAuthenticationToken: challenge.AccountAuthenticationToken,
		ProfileID:                  child.ID,
		PIN:                        "1357",
	}, nil)
	if replayStatus != http.StatusConflict || !strings.Contains(replayBody, "profile_selection_failed") {
		t.Fatalf("remembered account proof replay status=%d body=%s", replayStatus, replayBody)
	}
	var viewer AuthMeResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/profile-sessions/browser", BrowserProfileSessionRequest{
		SelectionGrant:  selection.SelectionGrant,
		RememberBrowser: boolPointer(true),
	}, &viewer)
	if status != http.StatusCreated || !viewer.Authenticated || viewer.ProfileID != child.ID {
		t.Fatalf("open remembered profile status=%d body=%s viewer=%#v", status, body, viewer)
	}
}

func optionalCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func browserAccountSwitchRequest(accountID string) BrowserAccountSwitchRequest {
	return BrowserAccountSwitchRequest{
		AccountID: accountID,
		ProfileDeviceDescriptor: ProfileDeviceDescriptor{
			InstallationID: "browser-installation-1",
			DeviceName:     "Test browser",
			App:            "portico-web",
			Platform:       "web",
		},
	}
}

func TestBrowserAccountVaultEnrollsListsAndSwitchesWithoutExposingPrivateUserData(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	second := createBrowserVaultTestUser(t, server, "sam", "Sam", "Password5678")

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginBrowserVaultUser(t, client, serverURL, "admin", "Password1234")
	firstSession := sessionCookieFromJar(t, jar, serverURL).Value
	firstVault := browserVaultCookieFromJar(t, jar, serverURL).Value

	loginBrowserVaultUser(t, client, serverURL, second.Username, "Password5678")
	secondSession := sessionCookieFromJar(t, jar, serverURL).Value
	secondVault := browserVaultCookieFromJar(t, jar, serverURL).Value
	if secondSession == firstSession || secondVault == firstVault {
		t.Fatalf("interactive enrollment did not rotate both credentials")
	}

	var listed BrowserAccountsResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/auth/browser-accounts", nil, &listed)
	if status != http.StatusOK {
		t.Fatalf("list browser accounts status=%d body=%s", status, body)
	}
	if len(listed.Accounts) != 2 || listed.ActiveAccountID != second.ID || !listed.AutomaticSignIn || listed.SelectionRequired || !listed.CanAddAccount {
		t.Fatalf("unexpected browser account list: %#v", listed)
	}
	for _, account := range listed.Accounts {
		if account.ID == "" || account.DisplayName == "" || account.AuthProvider != "local" || account.LastUsedAt == "" {
			t.Fatalf("browser account omitted chooser-safe identity: %#v", account)
		}
	}
	if strings.Contains(body, "permissions") || strings.Contains(body, "libraryIds") || strings.Contains(body, "email") || strings.Contains(body, "hasLocalPassword") {
		t.Fatalf("browser account list leaked private user fields: %s", body)
	}

	ownerID := browserVaultTestUserID(t, server.db, "admin")
	var switched AuthMeResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/browser-accounts/switch", browserAccountSwitchRequest(ownerID), &switched)
	if status != http.StatusOK || !switched.Authenticated || switched.User == nil || switched.User.ID != ownerID {
		t.Fatalf("switch browser account status=%d body=%s auth=%#v", status, body, switched)
	}
	if sessionCookieFromJar(t, jar, serverURL).Value == secondSession || browserVaultCookieFromJar(t, jar, serverURL).Value == secondVault {
		t.Fatalf("remembered-account switch did not rotate both credentials")
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/auth/browser-accounts/preferences", BrowserAccountPreferencesRequest{AutomaticSignIn: boolPointer(false)}, nil)
	if status != http.StatusOK {
		t.Fatalf("disable automatic sign in status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/logout", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("sign out current session status=%d body=%s", status, body)
	}
	listed = BrowserAccountsResponse{}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/auth/browser-accounts", nil, &listed)
	if status != http.StatusOK || listed.ActiveAccountID != "" || !listed.SelectionRequired || listed.AutomaticSignIn {
		t.Fatalf("signed-out chooser state status=%d body=%s response=%#v", status, body, listed)
	}
}

func TestBrowserAccountVaultRequiresCSRFAndRevalidatesDisabledAndExpiredAccounts(t *testing.T) {
	serverURL, db, _ := newAuthTestServerWithInstance(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginBrowserVaultUser(t, client, serverURL, "admin", "Password1234")
	ownerID := browserVaultTestUserID(t, db, "admin")

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/auth/browser-accounts/switch", strings.NewReader(`{"accountId":"`+ownerID+`"}`))
	if err != nil {
		t.Fatalf("create CSRF test request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send CSRF test request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("vault mutation without CSRF status=%d", resp.StatusCode)
	}
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/browser-accounts/switch", BrowserAccountSwitchRequest{AccountID: ownerID}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "device_identity_required") {
		t.Fatalf("switch without device descriptor status=%d body=%s", status, body)
	}

	if _, err := db.Exec(`UPDATE users SET disabled_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), ownerID); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/browser-accounts/switch", browserAccountSwitchRequest(ownerID), nil)
	if status != http.StatusForbidden || !strings.Contains(body, "account_disabled") {
		t.Fatalf("disabled switch status=%d body=%s", status, body)
	}
	if _, err := db.Exec(`UPDATE users SET disabled_at = '' WHERE id = ?`, ownerID); err != nil {
		t.Fatalf("enable user: %v", err)
	}
	if _, err := db.Exec(`UPDATE browser_account_entries SET expires_at = ? WHERE user_id = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), ownerID); err != nil {
		t.Fatalf("expire browser entry: %v", err)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/browser-accounts/switch", browserAccountSwitchRequest(ownerID), nil)
	if status != http.StatusUnauthorized || !strings.Contains(body, "browser_account_expired") {
		t.Fatalf("expired switch status=%d body=%s", status, body)
	}
}

func TestRevokingOneBrowserAccountDeviceKeepsUnrelatedVaultEntry(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	second := createBrowserVaultTestUser(t, server, "sam", "Sam", "Password5678")
	ownerID := browserVaultTestUserID(t, db, "admin")

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginBrowserVaultUser(t, client, serverURL, "admin", "Password1234")
	loginBrowserVaultUser(t, client, serverURL, second.Username, "Password5678")

	var vaultID, ownerDeviceID, secondDeviceID string
	if err := db.QueryRow(`
		SELECT owner_entry.vault_id
		FROM browser_account_entries owner_entry
		JOIN browser_account_entries second_entry ON second_entry.vault_id = owner_entry.vault_id
		WHERE owner_entry.user_id = ? AND second_entry.user_id = ?
			AND owner_entry.revoked_at = '' AND second_entry.revoked_at = ''`, ownerID, second.ID).Scan(&vaultID); err != nil {
		t.Fatalf("load vault id: %v", err)
	}
	if err := db.QueryRow(`SELECT device_id FROM browser_account_entries WHERE vault_id = ? AND user_id = ?`, vaultID, ownerID).Scan(&ownerDeviceID); err != nil {
		t.Fatalf("load owner device: %v", err)
	}
	if err := db.QueryRow(`SELECT device_id FROM browser_account_entries WHERE vault_id = ? AND user_id = ?`, vaultID, second.ID).Scan(&secondDeviceID); err != nil {
		t.Fatalf("load second device: %v", err)
	}
	if ownerDeviceID == secondDeviceID {
		t.Fatalf("per-user browser entries unexpectedly share a device grant")
	}
	if err := server.revokeDevice(secondDeviceID); err != nil {
		t.Fatalf("revoke second account device: %v", err)
	}

	var ownerRevoked, secondRevoked, vaultRevoked string
	if err := db.QueryRow(`SELECT revoked_at FROM browser_account_entries WHERE vault_id = ? AND user_id = ?`, vaultID, ownerID).Scan(&ownerRevoked); err != nil {
		t.Fatalf("query owner entry: %v", err)
	}
	if err := db.QueryRow(`SELECT revoked_at FROM browser_account_entries WHERE vault_id = ? AND user_id = ?`, vaultID, second.ID).Scan(&secondRevoked); err != nil {
		t.Fatalf("query second entry: %v", err)
	}
	if err := db.QueryRow(`SELECT revoked_at FROM browser_account_vaults WHERE id = ?`, vaultID).Scan(&vaultRevoked); err != nil {
		t.Fatalf("query vault: %v", err)
	}
	if ownerRevoked != "" || secondRevoked == "" || vaultRevoked != "" {
		t.Fatalf("device revocation crossed account boundary owner=%q second=%q vault=%q", ownerRevoked, secondRevoked, vaultRevoked)
	}

	var switched AuthMeResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/browser-accounts/switch", browserAccountSwitchRequest(ownerID), &switched)
	if status != http.StatusOK || switched.User == nil || switched.User.ID != ownerID {
		t.Fatalf("unrelated account no longer switchable status=%d body=%s auth=%#v", status, body, switched)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/browser-accounts/switch", browserAccountSwitchRequest(second.ID), nil)
	if status != http.StatusNotFound || !strings.Contains(body, "browser_account_not_found") {
		t.Fatalf("revoked device entry remained switchable status=%d body=%s", status, body)
	}
}

func TestRemovingActiveBrowserAccountAndSigningOutAllHaveDistinctEffects(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	second := createBrowserVaultTestUser(t, server, "sam", "Sam", "Password5678")
	ownerID := browserVaultTestUserID(t, db, "admin")

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginBrowserVaultUser(t, client, serverURL, "admin", "Password1234")
	loginBrowserVaultUser(t, client, serverURL, second.Username, "Password5678")

	var removed BrowserAccountMutationResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/browser-accounts/remove", BrowserAccountRemoveRequest{AccountID: second.ID}, &removed)
	if status != http.StatusOK || !removed.OK || !removed.ActiveAccountRemoved || removed.VaultRevoked {
		t.Fatalf("remove active account status=%d body=%s response=%#v", status, body, removed)
	}
	assertAuthenticated(t, client, serverURL, false)
	var listed BrowserAccountsResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/auth/browser-accounts", nil, &listed)
	if status != http.StatusOK || len(listed.Accounts) != 1 || listed.Accounts[0].ID != ownerID {
		t.Fatalf("remaining account list status=%d body=%s response=%#v", status, body, listed)
	}
	var activeVaultID string
	if err := db.QueryRow(`SELECT id FROM browser_account_vaults WHERE token_hash = ?`, hashToken(browserVaultCookieFromJar(t, jar, serverURL).Value)).Scan(&activeVaultID); err != nil {
		t.Fatalf("load active browser vault: %v", err)
	}

	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/auth/browser-accounts/sign-out-all", nil, &removed)
	if status != http.StatusOK {
		t.Fatalf("sign out all status=%d body=%s", status, body)
	}
	listed = BrowserAccountsResponse{}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/auth/browser-accounts", nil, &listed)
	if status != http.StatusOK || len(listed.Accounts) != 0 {
		t.Fatalf("vault survived sign out all status=%d body=%s response=%#v", status, body, listed)
	}
	var revokedAt string
	if err := db.QueryRow(`SELECT revoked_at FROM browser_account_vaults WHERE id = ?`, activeVaultID).Scan(&revokedAt); err != nil {
		t.Fatalf("load browser vault revocation: %v", err)
	}
	if revokedAt == "" {
		t.Fatalf("sign out all left the active browser vault usable")
	}
}

func createBrowserVaultTestUser(t *testing.T, server *Server, username, displayName, password string) User {
	t.Helper()
	user, err := server.createUser(UserRequest{
		Username:    username,
		Email:       username + "@example.test",
		DisplayName: displayName,
		Password:    password,
		Role:        "user",
	})
	if err != nil {
		t.Fatalf("create browser vault test user: %v", err)
	}
	return user
}

func loginBrowserVaultUser(t *testing.T, client *http.Client, serverURL, login, password string) {
	t.Helper()
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/login", map[string]any{
		"login":             login,
		"password":          password,
		"rememberOnBrowser": true,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("login %s status=%d body=%s", login, status, body)
	}
}

func browserVaultCookieFromJar(t *testing.T, jar *cookiejar.Jar, rawURL string) *http.Cookie {
	t.Helper()
	parsed, err := url.Parse(rawURL + "/api/auth/browser-accounts")
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	for _, cookie := range jar.Cookies(parsed) {
		if strings.HasPrefix(cookie.Name, browserVaultCookieName) {
			return cookie
		}
	}
	t.Fatalf("browser vault cookie was not set")
	return nil
}

func browserVaultTestUserID(t *testing.T, db *sql.DB, username string) string {
	t.Helper()
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&userID); err != nil {
		t.Fatalf("load user %s: %v", username, err)
	}
	return userID
}

func boolPointer(value bool) *bool { return &value }
