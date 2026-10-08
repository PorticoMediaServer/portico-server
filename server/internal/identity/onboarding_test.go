package identity

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSetupCodeExpirySurvivesRestart(t *testing.T) {
	_, db, dir := familyFixture(t)
	if _, err := db.Exec(`DELETE FROM accounts`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "setup-token")
	oldCode, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	// No expiry: a restart keeps the same resume secret, however old.
	restarted, err := New(db, dir)
	if err != nil {
		t.Fatal(err)
	}
	newCode, err := restarted.BrowserSetupToken(context.Background())
	if err != nil || newCode != string(oldCode) {
		t.Fatalf("restart replaced the setup resume secret: %v", err)
	}
}

func TestP09QuickConnectCommitsOneFamilyAndRecoversExactGrant(t *testing.T) {
	s, db, _ := familyFixture(t)
	ctx := context.Background()
	owner := issueFamily(t, s)
	p, e := s.Authenticate(owner.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	q := QuickStart{RequestID: "11111111-1111-4111-8111-111111111111", DeviceName: "Living Room", Platform: "tvos", AppVersion: "1.0"}
	a, e := s.StartQuick(ctx, q, true)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.StartQuick(ctx, q, true)
	if e != nil || a.DeviceCode != again.DeviceCode || a.UserCode != again.UserCode {
		t.Fatal("create was not idempotent", e)
	}
	preview, e := s.ReviewQuick(ctx, strings.ToLower(strings.ReplaceAll(a.UserCode, "-", " ")))
	if e != nil {
		t.Fatal(e)
	}
	// The phone approves in its own request; the TV collects in its own.
	phone, e := WithIssuingDevice(ctx, DeviceRegistration{InstallationID: Token(), Name: "Phone", Platform: "ios", App: "portico", AppVersion: "1.0"}, "127.0.0.1:1")
	if e != nil {
		t.Fatal(e)
	}
	tvInstallation := Token()
	tv, e := WithIssuingDevice(ctx, DeviceRegistration{InstallationID: tvInstallation, Name: "Living Room", Platform: "tvos", App: "portico", AppVersion: "1.0"}, "127.0.0.1:2")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DecideQuick(phone, p, preview.RequestID, a.UserCode, "approve"); e != nil {
		t.Fatal(e)
	}
	if e = s.DecideQuick(phone, p, preview.RequestID, a.UserCode, "approve"); e != nil {
		t.Fatal("duplicate approval", e)
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM authorization_session_families`).Scan(&count)
	if count != 1 {
		t.Fatalf("approval issued a family before the TV collected it: %d", count)
	}
	db.Exec(`UPDATE quick_connect_v1 SET next_poll=0`)
	first, e := s.PollQuick(tv, a.DeviceCode)
	if e != nil {
		t.Fatal(e)
	}
	db.QueryRow(`SELECT count(*) FROM authorization_session_families`).Scan(&count)
	if count != 2 {
		t.Fatalf("expected owner plus one issued family, got %d", count)
	}
	var bound, platform string
	if e = db.QueryRow(`SELECT d.installation_id,d.platform FROM identity_device_families f JOIN identity_devices d ON d.id=f.device_id WHERE f.family_id=?`, first.SessionFamilyID).Scan(&bound, &platform); e != nil || bound != tvInstallation || platform != "tvos" {
		t.Fatalf("TV session bound to %q (%s), not the TV: %v", bound, platform, e)
	}
	// The TV is signed in as whoever approved the code: here the owner.
	if first.Viewer.Role != p.Role || first.Viewer.AccountID != p.AccountID {
		t.Fatalf("TV signed in as %s (%s), not as the approver %s (%s)", first.Viewer.AccountID, first.Viewer.Role, p.AccountID, p.Role)
	}
	if first.InstallationID == "" {
		t.Fatal("Quick Connect grant omitted its refresh binding")
	}
	if _, err := s.Authenticate(first.AccessToken); err != nil {
		t.Fatalf("the TV's token was rejected: %v", err)
	}
	// The owner's television does what the owner can.
	if _, err := s.DirectMembers(ctx, first.AccessToken); err != nil {
		t.Fatalf("the owner's TV could not read the accounts the owner can: %v", err)
	}
	if a.UserCode[0] < '2' || a.UserCode[0] > '9' {
		t.Fatal("server code must start with a digit")
	}
	db.Exec(`UPDATE quick_connect_v1 SET next_poll=0`)
	second, e := s.PollQuick(tv, a.DeviceCode)
	if e != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("lost response recovery changed family", e)
	}
	if refreshed, refreshErr := s.RefreshSession(ctx, first.RefreshToken, first.InstallationID, Token(), nil); refreshErr != nil || refreshed.InstallationID != first.InstallationID {
		t.Fatal("Quick Connect grant could not refresh from its response", refreshErr)
	}
	if e = s.CancelQuick(ctx, a.DeviceCode); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(first.AccessToken); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("cancelled family remained active")
	}
	if _, e = s.Authenticate(owner.AccessToken); e != nil {
		t.Fatal("unrelated owner family was revoked", e)
	}
}

// A device linked with a code is signed in as whoever approved it: it can do what
// that person's own session can, and no more. A member's television manages the
// member's profiles as the member does; a session that is itself viewer-only
// links a viewer-only television.
func TestQuickConnectSignsTheDeviceInAsTheApprover(t *testing.T) {
	s, db, _ := familyFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id,epoch) VALUES('member-account','member',X'00','member-profile',1)`); err != nil {
		t.Fatal(err)
	}
	member, err := s.Issue("member-account", "member-profile", "local", TierMember, 1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Authenticate(member.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	link := func(request string) Envelope {
		t.Helper()
		a, err := s.StartQuick(ctx, QuickStart{RequestID: request, DeviceName: "TV", Platform: "tvos", AppVersion: "1"}, true)
		if err != nil {
			t.Fatal(err)
		}
		preview, err := s.ReviewQuick(ctx, a.UserCode)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.DecideQuick(ctx, p, preview.RequestID, a.UserCode, "approve"); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`UPDATE quick_connect_v1 SET next_poll=0`); err != nil {
			t.Fatal(err)
		}
		grant, err := s.PollQuick(ctx, a.DeviceCode)
		if err != nil {
			t.Fatal(err)
		}
		return grant
	}
	grant := link("33333333-3333-4333-8333-333333333333")
	if grant.Viewer.AccountID != "member-account" || grant.Viewer.Role != TierMember {
		t.Fatalf("television signed in as %s (%s), not as the member who approved it", grant.Viewer.AccountID, grant.Viewer.Role)
	}
	if _, err = s.DirectMe(ctx, grant.AccessToken); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateDirectProfile(ctx, grant.AccessToken, "Second", ""); err != nil {
		t.Fatalf("the member's television could not do what the member's own session can: %v", err)
	}
	// The approving session becomes viewer-only: what it links is viewer-only too.
	if _, err = db.Exec(`INSERT INTO authorization_family_grants(family_id,grant_kind) VALUES(?,'viewer_only')`, member.SessionFamilyID); err != nil {
		t.Fatal(err)
	}
	limited := link("44444444-4444-4444-8444-444444444444")
	if _, err = s.CreateDirectProfile(ctx, limited.AccessToken, "Escalated", ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("a viewer-only session linked a television that manages profiles: %v", err)
	}
	selected, err := s.SelectDirectProfile(ctx, limited.AccessToken, "member-profile", DirectSelection{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateDirectProfile(ctx, selected.Session.AccessToken, "Escalated", ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("a viewer-only television gained account management after profile selection: %v", err)
	}
	if _, err = s.PINReset(ctx, selected.Session.AccessToken, "member-profile", ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("a viewer-only television reset a PIN after profile selection: %v", err)
	}
}
func TestP09QuickConnectReplayCannotRestoreRotatedAuthority(t *testing.T) {
	s, db, _ := familyFixture(t)
	ctx := context.Background()
	owner := issueFamily(t, s)
	p, _ := s.Authenticate(owner.AccessToken)
	a, e := s.StartQuick(ctx, QuickStart{"22222222-2222-4222-8222-222222222222", "TV", "tvos", "1.0"}, true)
	if e != nil {
		t.Fatal(e)
	}
	v, _ := s.ReviewQuick(ctx, a.UserCode)
	if e = s.DecideQuick(ctx, p, v.RequestID, a.UserCode, "approve"); e != nil {
		t.Fatal(e)
	}
	db.Exec(`UPDATE quick_connect_v1 SET next_poll=0`)
	grant, e := s.PollQuick(ctx, a.DeviceCode)
	if e != nil {
		t.Fatal(e)
	}
	renewed := renewFamily(t, s, grant.AccessToken, "p09-rotation")
	db.Exec(`UPDATE quick_connect_v1 SET next_poll=0`)
	if _, e = s.PollQuick(ctx, a.DeviceCode); e == nil {
		t.Fatal("retired setup grant replayed")
	}
	if _, e = s.Authenticate(renewed.AccessToken); e != nil {
		t.Fatal("current family was lost", e)
	}
}
func TestP09HostedInitializationResumeAndPrivateRecovery(t *testing.T) {
	s, db, dir := familyFixture(t)
	ctx := context.Background()
	if _, e := db.Exec(`DELETE FROM accounts`); e != nil {
		t.Fatal(e)
	}
	token, e := os.ReadFile(filepath.Join(dir, "setup-token"))
	if e != nil {
		t.Fatal(e)
	}
	q := SetupOptions{RequestID: "33333333-3333-4333-8333-333333333333", SetupToken: string(token), Username: "owner", Name: "Home", Password: "Long-Private-Password9", AuthMode: "hosted", RecoverySaved: true}
	out, e := s.SetupWithOptions(ctx, q, true)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.ResumeSetup(ctx, q.RequestID, q.SetupToken, true)
	if e != nil || !reflect.DeepEqual(out, again) {
		t.Fatal("initialization result not recoverable", e)
	}
	p, e := s.Authenticate(out.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CheckRecoveryRoute(ctx, p, false); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("remote recovery default open", e)
	}
	state, e := s.Onboarding(ctx, p)
	if e != nil || state.Ready || !state.RecoveryOwner || !state.RecoveryConfirmed {
		t.Fatal(state, e)
	}
	if _, e = s.FinishSetup(ctx, p, state.Revision); e == nil {
		t.Fatal("uninstalled claim marked ready")
	}
	// No checklist asks the owner to finish: an installed claim makes it ready.
	s.SetupClaimReadyTx = func(context.Context, *sql.Tx) (bool, error) { return true, nil }
	if claimed, e := s.Onboarding(ctx, p); e != nil || !claimed.Ready || claimed.Phase != "ready" {
		t.Fatal("installed claim did not make hosted setup ready", claimed, e)
	}
	s.SetupClaimReadyTx = nil
	if _, e = s.SetRemoteRecovery(ctx, p, state.Revision, true, false); e == nil {
		t.Fatal("remote recovery enabled without warning")
	}
	enabled, e := s.SetRemoteRecovery(ctx, p, state.Revision, true, true)
	if e != nil || !enabled.RemoteRecovery {
		t.Fatal(enabled, e)
	}
}
func TestP09RecoveryPasswordAndHumanCodePolicy(t *testing.T) {
	// One rule for every local password: 8+ characters (bcrypt caps input at 72 bytes).
	for _, password := range []string{"short9!", strings.Repeat("a", 73), "abcdefg\xff"} {
		if directPassword(password) {
			t.Fatalf("password %q accepted", password)
		}
	}
	for _, password := range []string{"abcdefgh", "password", "orchard riverside candle marigold"} {
		if !directPassword(password) {
			t.Fatalf("password %q rejected", password)
		}
	}
	for i := 0; i < 100; i++ {
		code, e := quickCode()
		if e != nil || len(code) != 8 {
			t.Fatal(e)
		}
		if _, e = normalizedQuickCode(code); e != nil {
			t.Fatal(e)
		}
	}
}

// Setup from outside the LAN is allowed (no setup code), and the recovery owner
// it creates stays reachable from there so the person setting up is never locked out.
func TestRemoteHostedSetupKeepsRecoveryReachable(t *testing.T) {
	s, db, dir := familyFixture(t)
	ctx := context.Background()
	if _, e := db.Exec(`DELETE FROM accounts`); e != nil {
		t.Fatal(e)
	}
	token, e := os.ReadFile(filepath.Join(dir, "setup-token"))
	if e != nil {
		t.Fatal(e)
	}
	q := SetupOptions{RequestID: "44444444-4444-4444-8444-444444444444", SetupToken: string(token), Username: "owner", Name: "Home", Password: "simple12", AuthMode: "hosted", RecoverySaved: true}
	if _, e = s.SetupWithOptions(ctx, q, false); e != nil {
		t.Fatal("remote setup refused", e)
	}
	if _, e = s.ResumeSetup(ctx, q.RequestID, q.SetupToken, false); e != nil {
		t.Fatal("remote resume refused", e)
	}
	var remote bool
	if e = db.QueryRow(`SELECT remote_recovery FROM onboarding_state_v1 WHERE singleton=1`).Scan(&remote); e != nil || !remote {
		t.Fatal("remote recovery off after remote setup", remote, e)
	}
}
