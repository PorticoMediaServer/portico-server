package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func bindProfileTestDevice(t *testing.T, db *sql.DB, server *Server, accountID, installationID string) string {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin profile test device transaction: %v", err)
	}
	request := httptest.NewRequest("POST", "http://portico.test/api/auth/profile-sessions/browser", nil)
	request.Header.Set("User-Agent", "Portico profile protocol test")
	deviceID, err := server.upsertProfileAuthenticationDeviceTx(tx, request, accountID, ProfileDeviceDescriptor{
		InstallationID: installationID,
		DeviceName:     "Profile protocol test device",
		Platform:       "test",
		App:            "portico-test",
	}, time.Now().UTC())
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("bind profile test device: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit profile test device: %v", err)
	}
	return deviceID
}

func signedHostedProfileSelectionEnvelope(t *testing.T, privateKey ed25519.PrivateKey, envelope HostedProfileSelectionEnvelope) json.RawMessage {
	t.Helper()
	envelope.Signature = ""
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal unsigned hosted profile selection envelope: %v", err)
	}
	payload, err := canonicalHostedDocument("profile-selection-envelope", raw)
	if err != nil {
		t.Fatalf("canonicalize hosted profile selection envelope: %v", err)
	}
	envelope.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	raw, err = json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal signed hosted profile selection envelope: %v", err)
	}
	return raw
}

func TestLocalProfilePINHashUsesConfiguredBcryptCost(t *testing.T) {
	hash, err := hashLocalProfilePIN(t.Context(), kdfProfilePINSetHash, "1234")
	if err != nil {
		t.Fatalf("hash local profile PIN: %v", err)
	}
	if cost, err := bcrypt.Cost([]byte(hash)); err != nil || cost != localProfilePINBcryptCost {
		t.Fatalf("local profile PIN bcrypt cost=%d err=%v", cost, err)
	}
	valid, err := verifyLocalProfilePINHash(t.Context(), kdfProfilePINSelectCompare, hash, "1234")
	wrong, wrongErr := verifyLocalProfilePINHash(t.Context(), kdfProfilePINSelectCompare, hash, "9999")
	if !valid || err != nil || wrong || wrongErr != nil {
		t.Fatal("local profile PIN hash verification did not distinguish valid and invalid PINs")
	}
	amplified := strings.Replace(hash, "$08$", "$10$", 1)
	if valid, err := verifyLocalProfilePINHash(t.Context(), kdfProfilePINSelectCompare, amplified, "1234"); valid || err != nil {
		t.Fatal("profile PIN verifier accepted an unexpected bcrypt cost")
	}
	if _, err := hashLocalProfilePIN(t.Context(), kdfProfilePINSetHash, "12ab"); !errors.Is(err, errInvalidProfilePIN) {
		t.Fatalf("invalid PIN hash error = %v", err)
	}
}

func TestProfileSelectionCASCannotGrantAfterConcurrentPINReplacement(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{
		Username: "pin-cas", Email: "pin-cas@example.test", DisplayName: "PIN CAS",
		Password: "Profile-pin-cas-password1", Role: "user",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := server.setLocalProfilePINContext(t.Context(), account.ID, account.ID, "1234"); err != nil {
		t.Fatalf("set original PIN: %v", err)
	}

	previousAdmission := processKDFAdmission
	admission := newKDFAdmission(2, 8)
	processKDFAdmission = admission
	t.Cleanup(func() { processKDFAdmission = previousAdmission })
	blockerRelease, err := admission.acquire(t.Context(), kdfLaneCompare)
	if err != nil {
		t.Fatalf("occupy compare lane: %v", err)
	}

	type selectionResult struct {
		grant ProfileSelectionGrant
		err   error
	}
	result := make(chan selectionResult, 1)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	go func() {
		grant, issueErr := server.issueLocalProfileSelectionGrantForPurposeContext(
			t.Context(), account.ID, account.ID, "1234", "device-cas", "installation-cas", "native", "proof-cas", now,
		)
		result <- selectionResult{grant: grant, err: issueErr}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		admission.mu.Lock()
		queued := admission.queuedLocked(kdfLaneCompare)
		admission.mu.Unlock()
		if queued == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("selection did not reach admitted bcrypt wait")
		}
		runtime.Gosched()
	}

	// Hashing the authenticated replacement uses the reserved lane and commits
	// while selection still owns only the old read snapshot.
	if err := server.setLocalProfilePINContext(t.Context(), account.ID, account.ID, "5678"); err != nil {
		t.Fatalf("replace PIN while selection waits: %v", err)
	}
	blockerRelease()
	selection := <-result
	if selection.grant.Token != "" || (!errors.Is(selection.err, errProfilePINBackoff) && !errors.Is(selection.err, errProfilePINLocked)) {
		t.Fatalf("old PIN crossed replacement CAS: grant=%#v err=%v", selection.grant, selection.err)
	}
	var grants int
	if err := db.QueryRow(`SELECT COUNT(*) FROM profile_selection_grants WHERE account_id = ?`, account.ID).Scan(&grants); err != nil || grants != 0 {
		t.Fatalf("stale PIN minted grants=%d err=%v", grants, err)
	}
	if valid, err := server.verifyLocalProfilePINContext(t.Context(), account.ID, account.ID, "5678", now.Add(2*time.Second)); err != nil || !valid {
		t.Fatalf("replacement PIN did not recover normally: valid=%v err=%v", valid, err)
	}
}

func TestLocalProfilesUseAccountPermissionEnvelopeAndHashedPINs(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{
		Username:    "profile-account",
		Email:       "profile-account@example.test",
		DisplayName: "Profile Account",
		Password:    "Profile-account-password",
		Role:        "user",
		Permissions: map[string]bool{
			"playMedia": true, "downloadMedia": true, "editMetadata": true,
			"manageLyrics": true, "manageSubtitles": true, "manageDVR": true, "deleteMedia": true,
		},
		MaxContentRating: "PG-13",
	})
	if err != nil {
		t.Fatalf("create local account: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, account.ID, "2468"); err != nil {
		t.Fatalf("set primary profile PIN: %v", err)
	}
	restrictions := defaultProfileRestrictions()
	restrictions.AllowDownloads = false
	maximumAge := 17
	restrictions.MaximumAgeRating = &maximumAge

	profile, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{
		DisplayName:  "Child",
		PIN:          "1234",
		Restrictions: restrictions,
	})
	if err != nil {
		t.Fatalf("create local profile: %v", err)
	}
	if profile.AccountID != account.ID || profile.ID == account.ID || !profile.HasPIN {
		t.Fatalf("unexpected local profile: %#v", profile)
	}

	var pinHash string
	if err := db.QueryRow(`SELECT pin_hash FROM local_profile_pin_credentials WHERE profile_id = ?`, profile.ID).Scan(&pinHash); err != nil {
		t.Fatalf("read local profile pin credential: %v", err)
	}
	if pinHash == "1234" || !validProfilePINBcryptHash(pinHash, localProfilePINBcryptCost) {
		t.Fatalf("profile PIN was not stored at configured bcrypt cost %d: %q", localProfilePINBcryptCost, pinHash)
	}
	profiles, err := server.listAccountProfilesContext(context.Background(), account.ID)
	if err != nil || len(profiles) != 2 || !profiles[1].HasPIN || !profiles[1].PINRequired {
		t.Fatalf("list local account profiles = %#v err=%v", profiles, err)
	}

	principal, err := server.resolveRequestPrincipalContext(context.Background(), account.ID, profile.ID)
	if err != nil {
		t.Fatalf("resolve local profile principal: %v", err)
	}
	if !principal.Permissions["playMedia"] {
		t.Fatal("profile restriction unexpectedly removed an unrelated account permission")
	}
	if principal.AuthenticationAuthority != AuthenticationAuthorityLocal {
		t.Fatalf("local principal authority = %q", principal.AuthenticationAuthority)
	}
	if principal.MembershipIdentity.ServerAccountID != account.ID || principal.MembershipIdentity.HostedAccountID != "" || principal.MembershipIdentity.HostedMembershipID != "" {
		t.Fatalf("local membership identity = %#v", principal.MembershipIdentity)
	}
	if principal.MembershipEnvelope.Role != "user" || !principal.MembershipEnvelope.Permissions["playMedia"] || !principal.MembershipEnvelope.Permissions["downloadMedia"] {
		t.Fatalf("local membership envelope = %#v", principal.MembershipEnvelope)
	}
	if principal.Permissions["downloadMedia"] {
		t.Fatal("profile restriction expanded a permission denied to the account")
	}
	for _, permission := range []string{"manageServer", "manageLibraries", "manageUsers"} {
		if principal.Permissions[permission] {
			t.Fatalf("subordinate profile inherited administrative permission %q", permission)
		}
	}
	for _, permission := range []string{"editMetadata", "manageLyrics", "manageSubtitles", "manageDVR", "deleteMedia"} {
		if !principal.Permissions[permission] {
			t.Fatalf("subordinate profile lost granted media permission %q", permission)
		}
	}
	if principal.ProfileIsPrimary {
		t.Fatal("subordinate profile resolved as primary")
	}
	if principal.MaxContentRating != "PG-13" {
		t.Fatalf("profile expanded account content rating to %q", principal.MaxContentRating)
	}
	if principal.MaximumAgeRating == nil || *principal.MaximumAgeRating != maximumAge {
		t.Fatalf("canonical profile age restriction was not applied: %#v", principal.MaximumAgeRating)
	}
	if _, err := server.resolveRequestPrincipalContext(context.Background(), adminUserID(t, db), profile.ID); !errors.Is(err, errProfileNotFound) {
		t.Fatalf("cross-account profile resolution error = %v", err)
	}

	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	attemptAt := now
	for attempt := 0; attempt < localProfilePINFailureLimit; attempt++ {
		valid, err := server.verifyLocalProfilePINContext(context.Background(), account.ID, profile.ID, "9999", attemptAt)
		if err != nil || valid {
			t.Fatalf("wrong PIN attempt %d valid=%v err=%v", attempt+1, valid, err)
		}
		attemptAt = attemptAt.Add(time.Duration(1<<attempt)*time.Second + time.Millisecond)
	}
	if valid, err := server.verifyLocalProfilePINContext(context.Background(), account.ID, profile.ID, "1234", attemptAt); valid || !errors.Is(err, errProfilePINLocked) {
		t.Fatalf("locked PIN verification valid=%v err=%v", valid, err)
	}
	if valid, err := server.verifyLocalProfilePINContext(context.Background(), account.ID, profile.ID, "1234", attemptAt.Add(localProfilePINLockDuration+time.Second)); err != nil || !valid {
		t.Fatalf("PIN did not recover after lock interval: valid=%v err=%v", valid, err)
	}
	if err := server.clearLocalProfilePINContext(context.Background(), account.ID, profile.ID); err != nil {
		t.Fatalf("clear local profile PIN: %v", err)
	}
	if valid, err := server.verifyLocalProfilePINContext(context.Background(), account.ID, profile.ID, "1234", now.Add(time.Hour)); valid || !errors.Is(err, errProfilePINNotSet) {
		t.Fatalf("cleared local profile PIN valid=%v err=%v", valid, err)
	}
}

func TestProfileBoundSessionReturnsEffectivePrincipal(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{
		Username:    "profile-session",
		Email:       "profile-session@example.test",
		DisplayName: "Profile Session",
		Password:    "Profile-session-password1",
		Role:        "user",
		Permissions: map[string]bool{"playMedia": true, "downloadMedia": true},
	})
	if err != nil {
		t.Fatalf("create session account: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, account.ID, "2468"); err != nil {
		t.Fatalf("set session account primary PIN: %v", err)
	}
	restrictions := defaultProfileRestrictions()
	restrictions.AllowDownloads = false
	profile, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{
		DisplayName:  "Restricted Viewer",
		Restrictions: restrictions,
	})
	if err != nil {
		t.Fatalf("create session profile: %v", err)
	}

	request := httptest.NewRequest("POST", "http://portico.test/api/auth/login", nil)
	request.Header.Set("User-Agent", "Portico profile test")
	recorder := httptest.NewRecorder()
	installationID := "profile-session-installation"
	deviceID := bindProfileTestDevice(t, db, server, account.ID, installationID)
	grant, err := server.issueLocalProfileSelectionGrantForPurposeContext(context.Background(), account.ID, profile.ID, "", deviceID, installationID, "browser", "profile-session-proof", time.Now().UTC())
	if err != nil {
		t.Fatalf("issue profile-selection grant: %v", err)
	}
	token, err := server.createSessionForProviderWithSessionOptions(recorder, request, account.ID, "local", sessionCreateOptions{ProfileID: profile.ID, ProfileSelectionGrant: grant.Token, ProfileSelectionPurpose: "browser", BoundDeviceID: deviceID, BoundInstallationID: installationID})
	if err != nil {
		t.Fatalf("create profile-bound session: %v", err)
	}
	var storedProfileID string
	if err := db.QueryRow(`SELECT profile_id FROM sessions WHERE token_hash = ?`, hashToken(token)).Scan(&storedProfileID); err != nil {
		t.Fatalf("read profile-bound session: %v", err)
	}
	if storedProfileID != profile.ID {
		t.Fatalf("session profile = %q, want %q", storedProfileID, profile.ID)
	}

	authenticated, _, ok := server.userForSessionToken(httptest.NewRequest("GET", "http://portico.test/api/auth/me", nil), token)
	if !ok {
		t.Fatal("profile-bound session was not authenticated")
	}
	if authenticated.ID != account.ID || authenticated.AccountID != account.ID || authenticated.ProfileID != profile.ID {
		t.Fatalf("authenticated principal identity = %#v", authenticated)
	}
	if authenticated.DisplayName != "Restricted Viewer" || authenticated.Permissions["downloadMedia"] {
		t.Fatalf("authenticated profile policy was not applied: %#v", authenticated)
	}
}

func TestWatchHistoryClearIsIsolatedToSelectedProfile(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{
		Username: "history-household", Email: "history-household@example.test", DisplayName: "History Household",
		Password: "History-household-password", Role: "user", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create history account: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, account.ID, "2468"); err != nil {
		t.Fatalf("set primary profile PIN: %v", err)
	}
	child, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{
		DisplayName: "History Child", Restrictions: defaultProfileRestrictions(),
	})
	if err != nil {
		t.Fatalf("create history child: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, profileID := range []string{account.ID, child.ID} {
		if _, err := db.Exec(`
			INSERT INTO user_media_state (user_id, profile_id, media_id, watched, progress_seconds, last_played_at, updated_at)
			VALUES (?, ?, 'movie_meridian', 1, 600, ?, ?)
			ON CONFLICT(profile_id, media_id) DO UPDATE SET watched=1, progress_seconds=600, last_played_at=excluded.last_played_at, updated_at=excluded.updated_at`,
			account.ID, profileID, now, now); err != nil {
			t.Fatalf("seed %s media state: %v", profileID, err)
		}
		if _, err := db.Exec(`
			INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, state)
			VALUES (?, ?, ?, 'movie_meridian', 'movie', 'Meridian', ?, ?, ?, 'stopped')`,
			"history_"+profileID, account.ID, profileID, now, now, now); err != nil {
			t.Fatalf("seed %s playback session: %v", profileID, err)
		}
	}

	viewer := account
	viewer.AccountID = account.ID
	viewer.ProfileID = child.ID
	viewer.ProfileIsPrimary = false
	if _, err := server.clearAccountWatchHistoryContext(context.Background(), viewer); err != nil {
		t.Fatalf("clear child watch history: %v", err)
	}

	var primaryWatched, primaryProgress, childWatched, childProgress int
	if err := db.QueryRow(`SELECT watched, progress_seconds FROM user_media_state WHERE profile_id = ? AND media_id = 'movie_meridian'`, account.ID).Scan(&primaryWatched, &primaryProgress); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT watched, progress_seconds FROM user_media_state WHERE profile_id = ? AND media_id = 'movie_meridian'`, child.ID).Scan(&childWatched, &childProgress); err != nil {
		t.Fatal(err)
	}
	if primaryWatched != 1 || primaryProgress != 600 {
		t.Fatalf("primary history changed while clearing child: watched=%d progress=%d", primaryWatched, primaryProgress)
	}
	if childWatched != 0 || childProgress != 0 {
		t.Fatalf("child history was not cleared: watched=%d progress=%d", childWatched, childProgress)
	}
	var primarySessions, childSessions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, "history_"+account.ID).Scan(&primarySessions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, "history_"+child.ID).Scan(&childSessions); err != nil {
		t.Fatal(err)
	}
	if primarySessions != 1 || childSessions != 0 {
		t.Fatalf("profile session isolation failed: primary=%d child=%d", primarySessions, childSessions)
	}
}

func TestSubordinateProfileCannotReachAccountOrAdministrativeSurfaces(t *testing.T) {
	_, _, server := newAuthTestServerWithInstance(t)
	viewer := User{
		ID: "account_owner", AccountID: "account_owner", ProfileID: "profile_child",
		ProfileIsPrimary: false, Role: "owner",
		Permissions: map[string]bool{"playMedia": true},
	}
	for _, route := range []struct {
		path    string
		handler authedHandler
	}{
		{path: "/api/account/profile", handler: server.handleAccountProfile},
		{path: "/api/account/preferences", handler: server.handleAccountPreferences},
	} {
		request := httptest.NewRequest(http.MethodGet, "http://portico.test"+route.path, nil)
		response := httptest.NewRecorder()
		route.handler(response, request, viewer)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "primary_profile_required") {
			t.Fatalf("subordinate access to %s status=%d body=%s", route.path, response.Code, response.Body.String())
		}
	}
	if _, err := server.db.Exec(`
		INSERT INTO settings (key, value_json, updated_at) VALUES ('devices', '{"quickConnectApprovalMode":"ownerOnly"}', ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if server.userCanApproveQuickConnect(viewer) {
		t.Fatal("subordinate owner profile retained owner-only Quick Connect approval")
	}
}

func TestProfileRestrictionsRejectUnknownVersionsFieldsAndUnsafeBounds(t *testing.T) {
	valid, err := encodeProfileRestrictions(defaultProfileRestrictions())
	if err != nil {
		t.Fatalf("encode default restrictions: %v", err)
	}
	if decoded, err := decodeProfileRestrictions(valid); err != nil || decoded.Version != "v1" || !decoded.AllowDownloads {
		t.Fatalf("decode default restrictions = %#v err=%v", decoded, err)
	}
	for _, raw := range []string{
		`{"version":"portico.profile-restrictions.v1","maximumAgeRating":null,"allowUnrated":true,"blockedLabels":[],"allowDownloads":true,"allowLiveTV":true,"allowDvr":true,"allowWatchWithFriends":true,"allowFeedback":true}`,
		`{"version":"v1","maximumAgeRating":22,"allowUnrated":true,"blockedLabels":[],"allowDownloads":true,"allowLiveTV":true,"allowDvr":true,"allowWatchWithFriends":true,"allowFeedback":true}`,
		`{"version":"v1","maximumAgeRating":null,"allowUnrated":true,"blockedLabels":[],"allowDownloads":true,"allowLiveTV":true,"allowDvr":true,"allowWatchWithFriends":true,"allowFeedback":true,"permissions":{}}`,
		`{"version":"v1","maximumAgeRating":null,"allowUnrated":true,"blockedLabels":["Violence","violence"],"allowDownloads":true,"allowLiveTV":true,"allowDvr":true,"allowWatchWithFriends":true,"allowFeedback":true}`,
	} {
		if _, err := decodeProfileRestrictions(raw); !errors.Is(err, errInvalidProfileRestriction) {
			t.Fatalf("unsafe restriction document accepted: %s err=%v", raw, err)
		}
	}
}

func TestPrimaryPINIsRequiredForChildrenAndCannotBeCleared(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{Username: "pin-owner", Email: "pin-owner@example.test", DisplayName: "PIN Owner", Password: "Pin-owner-password1", Role: "user", Permissions: map[string]bool{"playMedia": true}})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{DisplayName: "Blocked Child"}); !errors.Is(err, errPrimaryProfilePINRequired) {
		t.Fatalf("child without primary PIN error = %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, account.ID, "2468"); err != nil {
		t.Fatalf("set primary PIN: %v", err)
	}
	request := httptest.NewRequest("POST", "http://portico.test/api/auth/login", nil)
	if _, err := server.createSessionForProviderWithSessionOptions(httptest.NewRecorder(), request, account.ID, "local", sessionCreateOptions{}); !errors.Is(err, errInvalidProfileSelectionGrant) {
		t.Fatalf("PIN-locked primary profile session without selection proof error = %v", err)
	}
	installationID := "primary-pin-installation"
	deviceID := bindProfileTestDevice(t, db, server, account.ID, installationID)
	primaryGrant, err := server.issueLocalProfileSelectionGrantForPurposeContext(context.Background(), account.ID, account.ID, "2468", deviceID, installationID, "browser", "primary-pin-proof", time.Now().UTC())
	if err != nil {
		t.Fatalf("issue primary profile selection grant: %v", err)
	}
	if _, err := server.createSessionForProviderWithSessionOptions(httptest.NewRecorder(), request, account.ID, "local", sessionCreateOptions{ProfileSelectionGrant: primaryGrant.Token, ProfileSelectionPurpose: "browser", BoundDeviceID: deviceID, BoundInstallationID: installationID}); err != nil {
		t.Fatalf("PIN-locked primary profile session with selection proof: %v", err)
	}
	child, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{DisplayName: "Allowed Child"})
	if err != nil {
		t.Fatalf("create child after primary PIN: %v", err)
	}
	if err := server.clearLocalProfilePINContext(context.Background(), account.ID, account.ID); !errors.Is(err, errPrimaryProfilePINInUse) {
		t.Fatalf("clear primary PIN with active child error = %v", err)
	}
	if _, err := db.Exec(`DELETE FROM local_profile_pin_credentials WHERE profile_id = ?`, account.ID); err == nil {
		t.Fatal("database trigger allowed primary PIN deletion while a child exists")
	}
	if _, err := db.Exec(`UPDATE profiles SET disabled_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), child.ID); err != nil {
		t.Fatalf("disable child: %v", err)
	}
	if err := server.clearLocalProfilePINContext(context.Background(), account.ID, account.ID); err != nil {
		t.Fatalf("clear primary PIN after child disabled: %v", err)
	}
}

func TestProfileSelectionGrantIsBoundOneTimeShortLivedAndPINRevisionAware(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{Username: "grant-owner", Email: "grant-owner@example.test", DisplayName: "Grant Owner", Password: "Grant-owner-password1", Role: "user", Permissions: map[string]bool{"playMedia": true}})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, account.ID, "2468"); err != nil {
		t.Fatalf("set primary PIN: %v", err)
	}
	child, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{DisplayName: "Grant Child", PIN: "1234"})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	now := time.Now().UTC()
	installationID := "selection-grant-installation"
	deviceID := bindProfileTestDevice(t, db, server, account.ID, installationID)
	grant, err := server.issueLocalProfileSelectionGrantForPurposeContext(context.Background(), account.ID, child.ID, "1234", deviceID, installationID, "browser", "selection-grant-proof-1", now)
	if err != nil {
		t.Fatalf("issue grant: %v", err)
	}
	request := httptest.NewRequest("POST", "http://portico.test/api/auth/login", nil)
	sessionOptions := func(token string) sessionCreateOptions {
		return sessionCreateOptions{ProfileID: child.ID, ProfileSelectionGrant: token, ProfileSelectionPurpose: "browser", BoundDeviceID: deviceID, BoundInstallationID: installationID}
	}
	if _, err := server.createSessionForProviderWithSessionOptions(httptest.NewRecorder(), request, account.ID, "local", sessionOptions(grant.Token)); err != nil {
		t.Fatalf("consume grant: %v", err)
	}
	if _, err := server.createSessionForProviderWithSessionOptions(httptest.NewRecorder(), request, account.ID, "local", sessionOptions(grant.Token)); !errors.Is(err, errProfileSelectionGrantConsumed) {
		t.Fatalf("reused grant error = %v", err)
	}
	expired, err := server.issueLocalProfileSelectionGrantForPurposeContext(context.Background(), account.ID, child.ID, "1234", deviceID, installationID, "browser", "selection-grant-proof-expired", now.Add(-profileSelectionGrantTTL-time.Second))
	if err != nil {
		t.Fatalf("issue expired grant fixture: %v", err)
	}
	if _, err := server.createSessionForProviderWithSessionOptions(httptest.NewRecorder(), request, account.ID, "local", sessionOptions(expired.Token)); !errors.Is(err, errInvalidProfileSelectionGrant) {
		t.Fatalf("expired grant error = %v", err)
	}
	revisionGrant, err := server.issueLocalProfileSelectionGrantForPurposeContext(context.Background(), account.ID, child.ID, "1234", deviceID, installationID, "browser", "selection-grant-proof-revision", now)
	if err != nil {
		t.Fatalf("issue revision grant: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, child.ID, "5678"); err != nil {
		t.Fatalf("rotate child PIN: %v", err)
	}
	if _, err := server.createSessionForProviderWithSessionOptions(httptest.NewRecorder(), request, account.ID, "local", sessionOptions(revisionGrant.Token)); !errors.Is(err, errInvalidProfileSelectionGrant) {
		t.Fatalf("stale PIN-revision grant error = %v", err)
	}
}

func TestNativeCredentialTokensAreCryptographicallyProfileBound(t *testing.T) {
	_, _, server := newAuthTestServerWithInstance(t)
	base := nativeRefreshTokenRecord{ID: "rft_profile_binding", FamilyID: "family_profile_binding", UserID: "account_1", ProfileID: "profile_1", DeviceID: "device_1", AuthProvider: "local", CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour)}
	refreshOne, err := server.nativeRefreshTokenValue(base)
	if err != nil {
		t.Fatalf("derive refresh token: %v", err)
	}
	accessOne, _, err := server.nativeAccessTokenValue(base)
	if err != nil {
		t.Fatalf("derive access token: %v", err)
	}
	base.ProfileID = "profile_2"
	refreshTwo, err := server.nativeRefreshTokenValue(base)
	if err != nil {
		t.Fatalf("derive second refresh token: %v", err)
	}
	accessTwo, _, err := server.nativeAccessTokenValue(base)
	if err != nil {
		t.Fatalf("derive second access token: %v", err)
	}
	if refreshOne == refreshTwo || accessOne == accessTwo {
		t.Fatal("native credential token did not change when only profile identity changed")
	}
}

func TestHostedProfileSelectionEnvelopeIsVerifiedBoundAndSingleUse(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{Username: "hosted-selection", Email: "hosted-selection@example.test", DisplayName: "Hosted Selection", Password: "Hosted-selection-password1", Role: "user", Permissions: map[string]bool{"playMedia": true}})
	if err != nil {
		t.Fatalf("create account shell: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET auth_origin = 'portico', portico_user_id = 'cloud_account_selection', portico_membership_id = 'membership_selection', password_hash = NULL WHERE id = ?`, account.ID); err != nil {
		t.Fatalf("convert account shell: %v", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	server.cfg.HostedDocumentPublicKeys = map[string]string{"profiles-key": base64.StdEncoding.EncodeToString(publicKey)}
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/servers/server-selection/profile-selection-exchanges" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer server-selection-credential" {
			writeError(w, http.StatusUnauthorized, "invalid_server_credential", "Server credential is invalid.")
			return
		}
		var request struct {
			SelectionEnvelope json.RawMessage `json:"selectionEnvelope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.SelectionEnvelope) == 0 {
			writeError(w, http.StatusBadRequest, "invalid_selection_envelope", "Selection envelope is required.")
			return
		}
		writeJSON(w, http.StatusOK, request.SelectionEnvelope)
	}))
	t.Cleanup(hosted.Close)
	if err := server.saveRemoteAccessSettings(RemoteAccessSettings{
		Enabled: true, HostedBaseURL: hosted.URL, ClaimStatus: "claimed", ServerID: "server-selection",
		PublicPortMode: "disabled", PreferredRemoteAuthMode: "portico",
	}); err != nil {
		t.Fatalf("save claimed server identity: %v", err)
	}
	if err := server.saveSecretSetting(remoteAccessCredentialKey, "server-selection-credential"); err != nil {
		t.Fatalf("save server credential: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	primary := HostedProfileSnapshot{ExternalProfileID: "cloud_primary_selection", AccountID: "cloud_account_selection", DisplayName: "Primary", IsPrimary: true, IsAccountAdmin: true, SortOrder: 0, PolicyUpdatedAt: now.Add(-time.Minute), Restrictions: defaultProfileRestrictions()}
	child := HostedProfileSnapshot{ExternalProfileID: "cloud_child_selection", AccountID: "cloud_account_selection", DisplayName: "Child", SortOrder: 1, PINRequired: true, PINRevision: 4, PolicyUpdatedAt: now.Add(-time.Minute), Restrictions: defaultProfileRestrictions()}
	envelopeFixture := func(assertionID, serverID, deviceID, installationID string, accountRevision, pinRevision int64, profiles []HostedProfileSnapshot, issuedAt, expiresAt time.Time) json.RawMessage {
		return signedHostedProfileSelectionEnvelope(t, privateKey, HostedProfileSelectionEnvelope{
			Version: hostedProfileSelectionAssertionVersion, AssertionID: assertionID, Audience: hostedDocumentAudience,
			AccountID: "cloud_account_selection", ProfileID: child.ExternalProfileID, ServerID: serverID, DeviceID: deviceID, InstallationID: installationID,
			AccountRevision: accountRevision, PINRevision: pinRevision, Profiles: profiles,
			IssuedAt: issuedAt.Format(time.RFC3339Nano), ExpiresAt: expiresAt.Format(time.RFC3339Nano),
			SignatureAlgorithm: hostedSignatureAlgorithm, SignatureKeyID: "profiles-key",
		})
	}
	installationID := "installation-selection-0001"
	valid := envelopeFixture("assertion-selection-1", "server-selection", "hosted-device-selection", installationID, 1, child.PINRevision, []HostedProfileSnapshot{primary, child}, now, now.Add(2*time.Minute))
	grant, err := server.issueHostedProfileSelectionGrantContext(context.Background(), account.ID, valid, "hosted-device-selection", "local-device-selection", installationID, now)
	if err != nil {
		t.Fatalf("issue hosted profile selection grant: %v", err)
	}
	if grant.AuthProvider != "portico" || grant.DeviceID != "local-device-selection" || grant.InstallationID != installationID {
		t.Fatalf("hosted grant binding = %#v", grant)
	}
	wrongDeviceTx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin wrong-device consumption: %v", err)
	}
	if _, err := server.consumeProfileSelectionGrantTx(wrongDeviceTx, grant.Token, account.ID, "portico", "other-local-device", installationID, now); !errors.Is(err, errInvalidProfileSelectionGrant) {
		wrongDeviceTx.Rollback()
		t.Fatalf("wrong local device consumed hosted grant: %v", err)
	}
	if err := wrongDeviceTx.Rollback(); err != nil {
		t.Fatalf("rollback wrong-device consumption: %v", err)
	}
	consumeTx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin correct-device consumption: %v", err)
	}
	principal, err := server.consumeProfileSelectionGrantTx(consumeTx, grant.Token, account.ID, "portico", "local-device-selection", installationID, now)
	if err != nil {
		consumeTx.Rollback()
		t.Fatalf("correct local device could not consume hosted grant: %v", err)
	}
	if principal.ProfileID == account.ID || principal.ProfileID == "" {
		consumeTx.Rollback()
		t.Fatalf("hosted grant selected wrong profile: %#v", principal)
	}
	if err := consumeTx.Commit(); err != nil {
		t.Fatalf("commit correct-device consumption: %v", err)
	}
	if _, err := server.issueHostedProfileSelectionGrantContext(context.Background(), account.ID, valid, "hosted-device-selection", "local-device-selection", installationID, now); !errors.Is(err, errHostedProfileSelectionAssertionReplayed) {
		t.Fatalf("replayed assertion error = %v", err)
	}
	mismatched := envelopeFixture("assertion-selection-2", "server-selection", "hosted-device-selection", installationID, 2, child.PINRevision, []HostedProfileSnapshot{primary, child}, now, now.Add(2*time.Minute))
	if _, err := server.issueHostedProfileSelectionGrantContext(context.Background(), account.ID, mismatched, "other-hosted-device", "local-device-selection", installationID, now); !errors.Is(err, errInvalidHostedProfileSelectionAssertion) {
		t.Fatalf("device-mismatched assertion error = %v", err)
	}
	if _, err := server.issueHostedProfileSelectionGrantContext(context.Background(), account.ID, mismatched, "hosted-device-selection", "local-device-selection", "other-installation-0001", now); err != nil {
		t.Fatalf("optional installation metadata changed assertion validity: %v", err)
	}
	wrongServer := envelopeFixture("assertion-selection-server", "other-server", "hosted-device-selection", installationID, 2, child.PINRevision, []HostedProfileSnapshot{primary, child}, now, now.Add(2*time.Minute))
	if _, err := server.issueHostedProfileSelectionGrantContext(context.Background(), account.ID, wrongServer, "hosted-device-selection", "local-device-selection", installationID, now); !errors.Is(err, errInvalidHostedProfileSelectionAssertion) {
		t.Fatalf("server-mismatched assertion error = %v", err)
	}
	staleRevision := envelopeFixture("assertion-selection-3", "server-selection", "hosted-device-selection", installationID, 2, child.PINRevision-1, []HostedProfileSnapshot{primary, child}, now, now.Add(2*time.Minute))
	if _, err := server.issueHostedProfileSelectionGrantContext(context.Background(), account.ID, staleRevision, "hosted-device-selection", "local-device-selection", installationID, now); !errors.Is(err, errInvalidHostedProfileSelectionAssertion) {
		t.Fatalf("stale PIN revision assertion error = %v", err)
	}
	expired := envelopeFixture("assertion-selection-expired", "server-selection", "hosted-device-selection", installationID, 2, child.PINRevision, []HostedProfileSnapshot{primary, child}, now.Add(-3*time.Minute), now.Add(-time.Second))
	if _, err := server.issueHostedProfileSelectionGrantContext(context.Background(), account.ID, expired, "hosted-device-selection", "local-device-selection", installationID, now); !errors.Is(err, errInvalidHostedProfileSelectionAssertion) {
		t.Fatalf("expired selection envelope error = %v", err)
	}
	var receiptCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM hosted_profile_selection_assertion_receipts WHERE account_id = ?`, account.ID).Scan(&receiptCount); err != nil || receiptCount != 2 {
		t.Fatalf("hosted assertion receipts=%d err=%v", receiptCount, err)
	}
}

func TestViewerMediaStateIsIsolatedByProfile(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{Username: "viewer-state", Email: "viewer-state@example.test", DisplayName: "Viewer State", Password: "Viewer-state-password", Role: "user", Permissions: map[string]bool{"playMedia": true}})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) SELECT ?, library_id, ? FROM media_items WHERE id = ? ON CONFLICT(user_id, library_id) DO NOTHING`, account.ID, time.Now().UTC().Format(time.RFC3339Nano), "movie_meridian"); err != nil {
		t.Fatalf("grant test library access: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, account.ID, "2468"); err != nil {
		t.Fatalf("set primary PIN: %v", err)
	}
	child, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{DisplayName: "Viewer Child"})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := server.setProgress(account.ID, "movie_meridian", 480, false); err != nil {
		t.Fatalf("set primary progress: %v", err)
	}
	if err := server.setProgress(child.ID, "movie_meridian", 900, false); err != nil {
		t.Fatalf("set child progress: %v", err)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_media_state WHERE user_id = ? AND media_id = ?`, account.ID, "movie_meridian").Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("profile state rows=%d err=%v", rows, err)
	}
	primaryItem, err := server.getMediaContext(context.Background(), account.ID, "movie_meridian")
	if err != nil {
		t.Fatalf("read primary media state: %v", err)
	}
	childItem, err := server.getMediaContext(context.Background(), child.ID, "movie_meridian")
	if err != nil {
		t.Fatalf("read child media state: %v", err)
	}
	if primaryItem.State.ProgressSeconds != 480 || childItem.State.ProgressSeconds != 900 {
		t.Fatalf("profile progress leaked: primary=%d child=%d", primaryItem.State.ProgressSeconds, childItem.State.ProgressSeconds)
	}
}

func TestSecondaryProfileDeletionPermanentlyErasesViewerState(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{
		Username: "erase-profile", Email: "erase-profile@example.test", DisplayName: "Erase Profile",
		Password: "Erase-profile-password1", Role: "user", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := server.setLocalProfilePINContext(context.Background(), account.ID, account.ID, "2468"); err != nil {
		t.Fatalf("set primary PIN: %v", err)
	}
	child, err := server.createLocalProfileContext(context.Background(), account.ID, CreateLocalProfileInput{DisplayName: "Erase Child"})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	serverID, err := server.profileDirectoryServerIDContext(context.Background())
	if err != nil {
		t.Fatalf("load profile directory server ID: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		INSERT INTO playlists (id, user_id, profile_id, kind, title, summary, visibility, smart_filter_json, created_at, updated_at)
		VALUES ('playlist-erasure', ?, ?, 'playlist', 'Private child list', '', 'private', '{}', ?, ?)`,
		account.ID, child.ID, now, now); err != nil {
		t.Fatalf("seed profile erasure playlist: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at)
		VALUES ('session-erasure', ?, ?, 'movie_meridian', 'movie', 'Meridian', ?, ?, ?)`,
		account.ID, child.ID, now, now, now); err != nil {
		t.Fatalf("seed profile erasure session: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dashboard_playback_rollups (session_id, user_id, user_name, user_role, media_id, media_type, media_group, title, started_at, last_seen_at, ended_at, updated_at)
		VALUES ('session-erasure', ?, 'Erase Child', 'user', 'movie_meridian', 'movie', 'Movies', 'Meridian', ?, ?, ?, ?)`,
		account.ID, now, now, now, now); err != nil {
		t.Fatalf("seed profile erasure rollup: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO viewer_preference_documents (
			id, scope_type, authority, account_id, profile_id, server_id, device_class, installation_id,
			version, revision, values_json, created_at, updated_at
		) VALUES ('preference-erasure', 'account-server-installation', 'local', ?, '', ?, '', 'erase-installation', 'v1', 4, ?, ?, ?)
	`, account.ID, serverID, `{"rememberAccount":true,"profileSelection":"last-used","lastProfileId":"`+child.ID+`"}`, now, now); err != nil {
		t.Fatalf("seed profile erasure state: %v", err)
	}
	leasedContext, releaseLease, err := server.profileRuntime.acquire(context.Background(), account.ID, child.ID)
	if err != nil {
		t.Fatalf("acquire child profile runtime lease: %v", err)
	}
	type erasureResult struct {
		operationID string
		err         error
	}
	erasureDone := make(chan erasureResult, 1)
	go func() {
		operationID, eraseErr := server.deleteManagedProfileContext(context.Background(), account.ID, child.ID)
		erasureDone <- erasureResult{operationID: operationID, err: eraseErr}
	}()
	select {
	case <-leasedContext.Done():
	case <-time.After(time.Second):
		t.Fatal("profile erasure did not cancel active profile work")
	}
	if _, _, err := server.profileRuntime.acquire(context.Background(), account.ID, child.ID); !errors.Is(err, errProfileErasureInProgress) {
		t.Fatalf("profile erasure admitted new work: %v", err)
	}
	releaseLease()
	result := <-erasureDone
	if result.err != nil {
		t.Fatalf("erase child profile: %v", result.err)
	}
	operationID := result.operationID
	if operationID == "" {
		t.Fatal("erase child profile returned an empty operation receipt")
	}
	retryOperationID, err := server.deleteManagedProfileContext(context.Background(), account.ID, child.ID)
	if err != nil {
		t.Fatalf("retry child profile erasure: %v", err)
	}
	if retryOperationID != operationID {
		t.Fatalf("profile erasure retry returned receipt %q, want %q", retryOperationID, operationID)
	}
	for label, query := range map[string]string{
		"profile":  `SELECT COUNT(*) FROM profiles WHERE id = '` + child.ID + `'`,
		"playlist": `SELECT COUNT(*) FROM playlists WHERE id = 'playlist-erasure'`,
		"session":  `SELECT COUNT(*) FROM playback_sessions WHERE id = 'session-erasure'`,
		"rollup":   `SELECT COUNT(*) FROM dashboard_playback_rollups WHERE session_id = 'session-erasure'`,
	} {
		var count int
		if err := db.QueryRow(query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s survived permanent profile erasure: count=%d err=%v", label, count, err)
		}
	}
	var primaryCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM profiles WHERE id = ? AND account_id = ? AND is_primary = 1`, account.ID, account.ID).Scan(&primaryCount); err != nil || primaryCount != 1 {
		t.Fatalf("primary profile changed during child erasure: count=%d err=%v", primaryCount, err)
	}
	var lastProfileID string
	var preferenceRevision int
	if err := db.QueryRow(`SELECT json_extract(values_json, '$.lastProfileId'), revision FROM viewer_preference_documents WHERE id = 'preference-erasure'`).Scan(&lastProfileID, &preferenceRevision); err != nil || lastProfileID != account.ID || preferenceRevision != 5 {
		t.Fatalf("account installation preference after erasure lastProfileId=%q revision=%d err=%v", lastProfileID, preferenceRevision, err)
	}
	if _, err := server.deleteManagedProfileContext(context.Background(), account.ID, account.ID); err == nil {
		t.Fatal("primary profile deletion unexpectedly succeeded")
	}
	var receiptCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM profile_erasure_receipts WHERE account_id = ?`, account.ID).Scan(&receiptCount); err != nil || receiptCount != 1 {
		t.Fatalf("profile erasure receipt count=%d err=%v", receiptCount, err)
	}
}

func TestProfileErasureInventoryFailsClosedOnUndispositionedProfileColumn(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	if _, err := db.Exec(`CREATE TABLE future_profile_private_state (id TEXT PRIMARY KEY, profile_id TEXT NOT NULL, payload TEXT NOT NULL)`); err != nil {
		t.Fatalf("create synthetic profile-owned table: %v", err)
	}
	err := server.withUserTx(context.Background(), func(tx *sql.Tx) error {
		return verifyProfileErasureInventoryTx(context.Background(), tx)
	})
	if err == nil || !strings.Contains(err.Error(), "future_profile_private_state.profile_id") {
		t.Fatalf("profile erasure inventory accepted undispositioned schema drift: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE future_profile_private_state`); err != nil {
		t.Fatalf("drop synthetic profile-owned table: %v", err)
	}
	if err := server.withUserTx(context.Background(), func(tx *sql.Tx) error {
		return verifyProfileErasureInventoryTx(context.Background(), tx)
	}); err != nil {
		t.Fatalf("canonical profile erasure inventory failed: %v", err)
	}
}
