package app

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type hostedFreshnessFixture struct {
	db       *sql.DB
	server   *Server
	account  User
	primary  HostedProfileSnapshot
	child    HostedProfileSnapshot
	childID  string
	mode     string
	revision int64
	profiles []HostedProfileSnapshot
	delay    time.Duration
	requests atomic.Int64
}

func signedHostedDirectorySnapshot(t *testing.T, snapshot HostedProfileDirectorySnapshot) json.RawMessage {
	t.Helper()
	snapshot.Signature = ""
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal unsigned directory snapshot: %v", err)
	}
	payload, err := canonicalHostedDocument("profile-directory-snapshot", raw)
	if err != nil {
		t.Fatalf("canonicalize directory snapshot: %v", err)
	}
	snapshot.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(testHostedDocumentPrivateKey(), payload))
	raw, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal signed directory snapshot: %v", err)
	}
	return raw
}

func newHostedFreshnessFixture(t *testing.T) *hostedFreshnessFixture {
	t.Helper()
	_, db, server := newAuthTestServerWithInstance(t)
	account, err := server.createUser(UserRequest{
		Username: "freshness-account", Email: "freshness@example.test", DisplayName: "Freshness Account",
		Password: "Freshness-password1", Role: "user", Permissions: map[string]bool{"playMedia": true, "downloadMedia": true},
	})
	if err != nil {
		t.Fatalf("create freshness account: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET auth_origin = 'portico', portico_user_id = 'cloud-freshness-account', portico_membership_id = 'freshness-membership', password_hash = NULL WHERE id = ?`, account.ID); err != nil {
		t.Fatalf("convert freshness account: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	primary := HostedProfileSnapshot{
		ExternalProfileID: "cloud-freshness-primary", AccountID: "cloud-freshness-account", DisplayName: "Primary",
		IsPrimary: true, IsAccountAdmin: true, SortOrder: 0, PolicyUpdatedAt: now.Add(-time.Minute), Restrictions: defaultProfileRestrictions(),
	}
	childRestrictions := defaultProfileRestrictions()
	childRestrictions.AllowDownloads = false
	child := HostedProfileSnapshot{
		ExternalProfileID: "cloud-freshness-child", AccountID: "cloud-freshness-account", DisplayName: "Child",
		SortOrder: 1, PINRequired: true, PINRevision: 2, PolicyUpdatedAt: now.Add(-time.Minute), Restrictions: childRestrictions,
	}
	if err := server.reconcileHostedProfileSelectionEnvelopeContext(context.Background(), account.ID, HostedProfileSelectionEnvelope{
		AssertionID: "initial-freshness-snapshot", AccountID: "cloud-freshness-account", AccountRevision: 1,
		Profiles: []HostedProfileSnapshot{primary, child}, IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(5 * time.Minute).Format(time.RFC3339Nano),
	}, now); err != nil {
		t.Fatalf("apply initial freshness directory: %v", err)
	}
	fixture := &hostedFreshnessFixture{db: db, server: server, account: account, primary: primary, child: child, mode: "unchanged", revision: 1, profiles: []HostedProfileSnapshot{primary, child}}
	if err := db.QueryRow(`SELECT id FROM profiles WHERE account_id = ? AND external_profile_id = ?`, account.ID, child.ExternalProfileID).Scan(&fixture.childID); err != nil {
		t.Fatalf("find hosted child: %v", err)
	}
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/servers/freshness-server/profile-directory-snapshots" || r.Header.Get("Authorization") != "Bearer freshness-credential" {
			http.NotFound(w, r)
			return
		}
		fixture.requests.Add(1)
		if fixture.delay > 0 {
			time.Sleep(fixture.delay)
		}
		if fixture.mode == "unavailable" {
			w.Header().Set("Retry-After", "5")
			writeProductError(w, http.StatusServiceUnavailable, "server_unavailable", "Portico could not refresh profile access.")
			return
		}
		if fixture.mode == "terminal" {
			writeProductError(w, http.StatusForbidden, "profile_not_available_on_server", "This profile can no longer use this server.")
			return
		}
		checkedAt := time.Now().UTC()
		snapshot := HostedProfileDirectorySnapshot{
			Version: "v1", SnapshotID: "freshness-snapshot", Audience: hostedDocumentAudience,
			ServerID: "freshness-server", AccountID: "cloud-freshness-account", Status: fixture.mode,
			Revision: fixture.revision, CheckedAt: checkedAt.Format(time.RFC3339Nano), MaxAgeSeconds: int(hostedProfileFreshnessLease / time.Second), StaleIfErrorSeconds: int(hostedProfileStaleIfError / time.Second),
			SignatureAlgorithm: hostedSignatureAlgorithm, SignatureKeyID: testHostedDocumentKeyID,
		}
		if fixture.mode == "changed" {
			snapshot.Profiles = fixture.profiles
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(signedHostedDirectorySnapshot(t, snapshot))
	}))
	t.Cleanup(hosted.Close)
	server.cfg.HostedDocumentPublicKeys = testHostedDocumentPublicKeys()
	if err := server.saveRemoteAccessSettings(RemoteAccessSettings{Enabled: true, HostedBaseURL: hosted.URL, ClaimStatus: "claimed", ServerID: "freshness-server", PublicPortMode: "disabled", PreferredRemoteAuthMode: "portico"}); err != nil {
		t.Fatalf("save freshness remote settings: %v", err)
	}
	if err := server.saveSecretSetting(remoteAccessCredentialKey, "freshness-credential"); err != nil {
		t.Fatalf("save freshness server credential: %v", err)
	}
	loaded, err := server.remoteAccessSettings()
	if err != nil || loaded.HostedBaseURL != hosted.URL || loaded.ServerID != "freshness-server" || loaded.ClaimStatus != "claimed" || server.secretSetting(remoteAccessCredentialKey) != "freshness-credential" {
		t.Fatalf("freshness settings=%#v credential=%q err=%v", loaded, server.secretSetting(remoteAccessCredentialKey), err)
	}
	return fixture
}

func (fixture *hostedFreshnessFixture) expireLease(t *testing.T, age time.Duration) {
	t.Helper()
	if _, err := fixture.db.Exec(`UPDATE hosted_profile_snapshot_state SET checked_at = ?, refresh_retry_at = '' WHERE account_id = ?`, time.Now().UTC().Add(-age).Format(time.RFC3339Nano), fixture.account.ID); err != nil {
		t.Fatalf("expire hosted profile lease: %v", err)
	}
}

func (fixture *hostedFreshnessFixture) insertBrowserSession(t *testing.T, profileID string) string {
	t.Helper()
	token := "hosted-browser-session-" + profileID
	now := time.Now().UTC()
	deviceID := bindProfileTestDevice(t, fixture.db, fixture.server, fixture.account.ID, "freshness-browser-install-0001")
	var externalProfileID string
	if err := fixture.db.QueryRow(`SELECT external_profile_id FROM profiles WHERE id = ?`, profileID).Scan(&externalProfileID); err != nil {
		t.Fatalf("load hosted profile subject: %v", err)
	}
	identityNow := now.Format(time.RFC3339)
	if _, err := fixture.db.Exec(`
		INSERT OR IGNORE INTO profile_identities (id, profile_id, provider, subject, email, display_name, verified_at, last_seen_at, created_at, updated_at)
		VALUES (?, ?, 'portico', ?, ?, ?, ?, ?, ?, ?)`,
		"pident_freshness_"+profileID, profileID, externalProfileID, fixture.account.Email, externalProfileID, identityNow, identityNow, identityNow, identityNow); err != nil {
		t.Fatalf("insert hosted profile identity: %v", err)
	}
	var profileIdentityID string
	if err := fixture.db.QueryRow(`SELECT id FROM profile_identities WHERE profile_id = ? AND provider = 'portico'`, profileID).Scan(&profileIdentityID); err != nil {
		t.Fatalf("load hosted profile identity: %v", err)
	}
	if _, err := fixture.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, profile_identity_id, auth_provider, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, 'portico', ?, ?, ?, ?, ?)`, randomID("sess"), fixture.account.ID, profileID, profileIdentityID, deviceID, hashToken(token),
		now.Add(time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert hosted browser session: %v", err)
	}
	return token
}

func TestHostedProfileFreshnessRevokesRemovedProfileAtRequestBoundary(t *testing.T) {
	fixture := newHostedFreshnessFixture(t)
	token := fixture.insertBrowserSession(t, fixture.childID)
	fixture.mode = "changed"
	fixture.revision = 2
	fixture.profiles = []HostedProfileSnapshot{fixture.primary}
	fixture.expireLease(t, hostedProfileFreshnessLease+time.Minute)

	request := httptest.NewRequest(http.MethodGet, "http://portico.test/api/home", nil)
	_, _, ok, err := fixture.server.userForSessionTokenWithError(request, token)
	if ok || err != nil {
		t.Fatalf("removed profile session ok=%v err=%v", ok, err)
	}
	var sessions int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ? AND profile_id = ?`, fixture.account.ID, fixture.childID).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("removed profile sessions=%d err=%v", sessions, err)
	}
	var profileCount int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM profiles WHERE id = ?`, fixture.childID).Scan(&profileCount); err != nil || profileCount != 0 {
		t.Fatalf("removed hosted profile survived permanent erasure: count=%d err=%v", profileCount, err)
	}
}

func TestHostedProfileFreshnessAppliesPolicyRestrictionWithoutEndingSession(t *testing.T) {
	fixture := newHostedFreshnessFixture(t)
	token := fixture.insertBrowserSession(t, fixture.account.ID)
	beforeRevision := fixture.server.authorizationRevisionForUserContext(context.Background(), fixture.account)
	restricted := fixture.primary
	restricted.Restrictions.AllowDownloads = false
	restricted.PINRequired = true
	restricted.PINRevision = 1
	restricted.PolicyUpdatedAt = time.Now().UTC()
	fixture.mode = "changed"
	fixture.revision = 2
	fixture.profiles = []HostedProfileSnapshot{restricted, fixture.child}
	fixture.expireLease(t, hostedProfileFreshnessLease+time.Minute)

	request := httptest.NewRequest(http.MethodGet, "http://portico.test/api/home", nil)
	user, _, ok, err := fixture.server.userForSessionTokenWithError(request, token)
	if !ok || err != nil {
		t.Fatalf("policy refresh ended active session: ok=%v err=%v", ok, err)
	}
	if user.Permissions["downloadMedia"] {
		t.Fatal("policy refresh did not remove download permission at request boundary")
	}
	if afterRevision := fixture.server.authorizationRevisionForUserContext(context.Background(), user); afterRevision == beforeRevision {
		t.Fatalf("policy refresh did not advance authorization revision %q", afterRevision)
	}
	var sessions, pinRequired int
	var pinRevision int64
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE token_hash = ?`, hashToken(token)).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("policy refresh sessions=%d err=%v", sessions, err)
	}
	if err := fixture.db.QueryRow(`SELECT pin_required, pin_revision FROM profiles WHERE id = ?`, fixture.account.ID).Scan(&pinRequired, &pinRevision); err != nil || pinRequired != 1 || pinRevision != 1 {
		t.Fatalf("policy refresh PIN state required=%d revision=%d err=%v", pinRequired, pinRevision, err)
	}
}

func TestHostedProfileRefreshReplacesQuarantinedRestoredProjectionEvenAtLowerRevision(t *testing.T) {
	fixture := newHostedFreshnessFixture(t)
	maliciousFuture := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	if _, err := fixture.db.Exec(`
		UPDATE hosted_profile_snapshot_state
		SET revision = 99, payload_digest = 'restored-untrusted', quarantined_at = ?, checked_at = ''
		WHERE account_id = ?`, time.Now().UTC().Format(time.RFC3339Nano), fixture.account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec(`UPDATE profiles SET pin_revision = 99, policy_updated_at = ? WHERE id = ?`, maliciousFuture, fixture.account.ID); err != nil {
		t.Fatal(err)
	}
	fixture.mode = "changed"
	fixture.revision = 1
	fixture.profiles = []HostedProfileSnapshot{fixture.primary, fixture.child}
	if err := fixture.server.refreshHostedProfileDirectoryContext(context.Background(), fixture.account.ID, hostedProfileSnapshotState{}, sql.ErrNoRows, time.Now().UTC()); err != nil {
		t.Fatalf("current signed Hosted projection could not replace quarantine: %v", err)
	}
	var revision, pinRevision int64
	var quarantinedAt, externalPrimary string
	if err := fixture.db.QueryRow(`SELECT revision, quarantined_at FROM hosted_profile_snapshot_state WHERE account_id = ?`, fixture.account.ID).Scan(&revision, &quarantinedAt); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.QueryRow(`SELECT external_profile_id, pin_revision FROM profiles WHERE id = ?`, fixture.account.ID).Scan(&externalPrimary, &pinRevision); err != nil {
		t.Fatal(err)
	}
	if revision != 1 || quarantinedAt != "" || externalPrimary != fixture.primary.ExternalProfileID || pinRevision != fixture.primary.PINRevision {
		t.Fatalf("reconciled projection revision=%d quarantine=%q primary=%q pinRevision=%d", revision, quarantinedAt, externalPrimary, pinRevision)
	}
}

func TestHostedProfileFreshnessKeepsPrimaryWhenMembershipDisallowsSubprofiles(t *testing.T) {
	fixture := newHostedFreshnessFixture(t)
	primaryToken := fixture.insertBrowserSession(t, fixture.account.ID)
	childToken := fixture.insertBrowserSession(t, fixture.childID)
	if _, err := fixture.db.Exec(`UPDATE users SET allow_account_profiles = 0 WHERE id = ?`, fixture.account.ID); err != nil {
		t.Fatalf("disable hosted subprofiles: %v", err)
	}
	fixture.mode = "changed"
	fixture.revision = 2
	fixture.profiles = []HostedProfileSnapshot{fixture.primary, fixture.child}
	fixture.expireLease(t, hostedProfileFreshnessLease+time.Minute)

	request := httptest.NewRequest(http.MethodGet, "http://portico.test/api/home", nil)
	primary, _, ok, err := fixture.server.userForSessionTokenWithError(request, primaryToken)
	if !ok || err != nil || primary.ProfileID != fixture.account.ID {
		t.Fatalf("primary session rejected by subprofile policy: ok=%v err=%v user=%#v", ok, err, primary)
	}
	var activeProfiles int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM profiles WHERE account_id = ? AND disabled_at = ''`, fixture.account.ID).Scan(&activeProfiles); err != nil || activeProfiles != 2 {
		t.Fatalf("complete hosted directory was not reconciled: profiles=%d err=%v", activeProfiles, err)
	}
	if _, _, ok, err := fixture.server.userForSessionTokenWithError(request, childToken); ok || err != nil {
		t.Fatalf("disallowed subordinate session ok=%v err=%v", ok, err)
	}
	if _, err := fixture.server.resolveRequestPrincipalContext(context.Background(), fixture.account.ID, fixture.childID); !errors.Is(err, errProfileNotAllowed) {
		t.Fatalf("subordinate selection policy err=%v", err)
	}
}

func TestHostedProfileFreshnessCoalescesConcurrentExpiredRequests(t *testing.T) {
	fixture := newHostedFreshnessFixture(t)
	fixture.delay = 100 * time.Millisecond
	fixture.expireLease(t, hostedProfileFreshnessLease+time.Minute)

	const callers = 20
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errs <- fixture.server.ensureHostedProfileDirectoryFreshness(context.Background(), fixture.account.ID, time.Now().UTC())
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent freshness request failed: %v", err)
		}
	}
	if requests := fixture.requests.Load(); requests != 1 {
		t.Fatalf("concurrent requests reached Hosted Services %d times, want 1", requests)
	}
}

func TestHostedProfileFreshnessRefreshAheadDoesNotBlockRequest(t *testing.T) {
	fixture := newHostedFreshnessFixture(t)
	fixture.delay = 250 * time.Millisecond
	age := hostedProfileFreshnessLease - hostedProfileRefreshAhead(fixture.account.ID) + time.Second
	fixture.expireLease(t, age)

	started := time.Now()
	if err := fixture.server.ensureHostedProfileDirectoryFreshness(context.Background(), fixture.account.ID, time.Now().UTC()); err != nil {
		t.Fatalf("refresh-ahead request failed: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("refresh-ahead blocked ordinary request for %s", elapsed)
	}
	deadline := time.Now().Add(2 * time.Second)
	for fixture.requests.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if fixture.requests.Load() != 1 {
		t.Fatalf("refresh-ahead Hosted request count=%d", fixture.requests.Load())
	}
}

func TestHostedProfileFreshnessBoundsTransientOutageWithoutDeletingCredentials(t *testing.T) {
	fixture := newHostedFreshnessFixture(t)
	token := fixture.insertBrowserSession(t, fixture.account.ID)
	fixture.mode = "unavailable"
	fixture.expireLease(t, hostedProfileFreshnessLease+time.Minute)
	request := httptest.NewRequest(http.MethodGet, "http://portico.test/api/home", nil)
	if _, _, ok, err := fixture.server.userForSessionTokenWithError(request, token); !ok || err != nil {
		var sessionCount int
		_ = fixture.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE token_hash = ?`, hashToken(token)).Scan(&sessionCount)
		var profileDisabled string
		_ = fixture.db.QueryRow(`SELECT disabled_at FROM profiles WHERE id = ?`, fixture.account.ID).Scan(&profileDisabled)
		t.Fatalf("brief outage should use bounded stale policy: ok=%v err=%v sessions=%d profileDisabled=%q", ok, err, sessionCount, profileDisabled)
	}

	fixture.expireLease(t, hostedProfileFreshnessLease+hostedProfileStaleIfError+time.Hour)
	if _, _, ok, err := fixture.server.userForSessionTokenWithError(request, token); ok || !errors.Is(err, errHostedProfileDirectoryUnavailable) {
		t.Fatalf("outage beyond stale bound ok=%v err=%v", ok, err)
	}
	var sessions int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE token_hash = ?`, hashToken(token)).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("transient outage deleted durable credential: sessions=%d err=%v", sessions, err)
	}
}

func TestHostedProfileFreshnessTerminalResponseRevokesAccountImmediately(t *testing.T) {
	fixture := newHostedFreshnessFixture(t)
	token := fixture.insertBrowserSession(t, fixture.account.ID)
	fixture.mode = "terminal"
	fixture.expireLease(t, hostedProfileFreshnessLease+time.Minute)
	request := httptest.NewRequest(http.MethodGet, "http://portico.test/api/home", nil)
	if _, _, ok, err := fixture.server.userForSessionTokenWithError(request, token); ok || !errors.Is(err, errHostedProfileAccessRevoked) {
		t.Fatalf("terminal freshness response ok=%v err=%v", ok, err)
	}
	var sessions int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, fixture.account.ID).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("terminal response sessions=%d err=%v", sessions, err)
	}
}

func TestHostedProfileFreshnessRevokesRemovedProfileOnNativeRefresh(t *testing.T) {
	fixture := newHostedFreshnessFixture(t)
	installationID := "freshness-native-install-0001"
	deviceID := bindProfileTestDevice(t, fixture.db, fixture.server, fixture.account.ID, installationID)
	tx, err := fixture.db.Begin()
	if err != nil {
		t.Fatalf("begin native freshness grant: %v", err)
	}
	grant, err := fixture.server.mintProfileSelectionGrantBoundTx(tx, fixture.account.ID, fixture.childID, "portico", "native", "freshness-selection-assertion", deviceID, installationID, time.Now().UTC())
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("mint native freshness grant: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit native freshness grant: %v", err)
	}
	user, err := fixture.server.getUser(fixture.account.ID)
	if err != nil {
		t.Fatalf("load freshness user: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://portico.test/api/auth/profile-sessions/native", nil)
	credentials, err := fixture.server.issueNativeSessionCredentials(request, user, "portico", nativeDeviceDescriptor{
		InstallationID: installationID, Name: "Freshness Native", App: "Portico Test", Platform: "TestOS",
		ProfileSelectionGrant: grant.Token, ProfileSelectionPurpose: "native",
	})
	if err != nil {
		t.Fatalf("issue native profile credentials: %v", err)
	}
	fixture.mode = "changed"
	fixture.revision = 2
	fixture.profiles = []HostedProfileSnapshot{fixture.primary}
	fixture.expireLease(t, hostedProfileFreshnessLease+time.Minute)
	if _, err := fixture.server.rotateNativeSessionCredentials(request, credentials.RefreshToken, strings.Repeat("A", 43)); err == nil {
		t.Fatal("native refresh succeeded after Cloud removed the selected profile")
	}
	var active int
	if err := fixture.db.QueryRow(`SELECT COUNT(*) FROM native_refresh_tokens WHERE user_id = ? AND profile_id = ? AND revoked_at = ''`, fixture.account.ID, fixture.childID).Scan(&active); err != nil || active != 0 {
		t.Fatalf("removed profile retained %d active refresh credentials: %v", active, err)
	}
}
