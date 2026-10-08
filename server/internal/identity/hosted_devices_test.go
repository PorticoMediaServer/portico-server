package identity

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// hostedViewer signs a Portico Account viewer in on this server: a cached policy with a live
// horizon, and one viewing family issued under it. Nothing here contacts Hosted Services, and
// nothing in the device feature that follows does either.
func hostedViewer(t *testing.T, s *Service, db *sql.DB, account, profile, role, installation string) Envelope {
	t.Helper()
	horizon := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	if _, e := db.Exec(`INSERT INTO policy(server_id,revision,payload,expires_at) VALUES(?,1,'{}',?) ON CONFLICT(server_id) DO UPDATE SET expires_at=excluded.expires_at`, s.ID(), horizon.Format(time.RFC3339)); e != nil {
		t.Fatal(e)
	}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	ctx, e := WithIssuingDevice(context.Background(), registration(installation), "203.0.113.9:4001")
	if e != nil {
		t.Fatal(e)
	}
	envelope, e := s.IssueTx(ctx, tx, account, profile, "hosted", role, 1, horizon)
	if e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	return envelope
}

// The whole of the gap this closes: a Portico Account viewer could not register a device, so
// there was no Devices entry, no per-device sign-out and no Top Shelf. It now gets the same
// record, on the same server, with no Hosted Services call anywhere in the path.
func TestAPorticoAccountViewerRegistersAndBindsADevice(t *testing.T) {
	ctx := context.Background()
	s, db, _ := familyFixture(t)
	installation := Token()
	viewer := hostedViewer(t, s, db, "hosted-account", "hosted-profile", "owner", installation)
	device, e := s.RegisterDevice(ctx, viewer.AccessToken, registration(installation), "203.0.113.9:4001")
	if e != nil {
		t.Fatal("a Portico Account viewer could not register a device", e)
	}
	if device.Authority != "hosted" || device.IP != "203.0.113.9" || device.ApprovalState != "approved" {
		t.Fatalf("hosted device record: %+v", device)
	}
	if e = s.BindDeviceFamily(ctx, viewer.AccessToken, installation, viewer.SessionFamilyID); e != nil {
		t.Fatal("the viewing family was not bound to the device", e)
	}
	list, e := s.Devices(ctx, viewer.AccessToken, installation)
	if e != nil || len(list) != 1 {
		t.Fatal("the device is missing from the account's own list", list, e)
	}
	if !list[0].Current || list[0].Sessions != 1 || list[0].Authority != "hosted" {
		t.Fatalf("listed device: %+v", list[0])
	}
	// Re-registering the same installation refreshes rather than duplicates, exactly as it
	// does for a local account.
	again, e := s.RegisterDevice(ctx, viewer.AccessToken, registration(installation), "203.0.113.9:4002")
	if e != nil || again.ID != device.ID {
		t.Fatal("re-registration created a second record", again, e)
	}
}

func TestHostedSessionReturnsInstallationBindingForRefresh(t *testing.T) {
	s, db, _ := familyFixture(t)
	installation := Token()
	issued := hostedViewer(t, s, db, "hosted-account", "hosted-profile", "member", installation)
	if issued.InstallationID != installation {
		t.Fatalf("hosted session omitted its installation binding: %q", issued.InstallationID)
	}
	verified := func(context.Context, *sql.Tx, Principal) (time.Time, error) {
		return time.Now().UTC().Add(time.Hour), nil
	}
	refreshed, err := s.RefreshSession(context.Background(), issued.RefreshToken, issued.InstallationID, Token(), verified)
	if err != nil || refreshed.InstallationID != installation || refreshed.RefreshToken == issued.RefreshToken {
		t.Fatalf("hosted session could not refresh from its response: %v", err)
	}
}

// The two authorities do not see each other's devices, even on the same installation, because
// they are two different sign-ins on one piece of hardware.
func TestDeviceRecordsAreSeparatePerAuthority(t *testing.T) {
	ctx := context.Background()
	s, db, login := directFixture(t)
	installation := Token()
	viewer := hostedViewer(t, s, db, "hosted-account", "hosted-profile", "owner", installation)
	local, e := s.RegisterDevice(ctx, login.AccountToken, registration(installation), "198.51.100.2:1")
	if e != nil {
		t.Fatal(e)
	}
	hosted, e := s.RegisterDevice(ctx, viewer.AccessToken, registration(installation), "198.51.100.2:2")
	if e != nil {
		t.Fatal(e)
	}
	if local.ID == hosted.ID {
		t.Fatal("two sign-ins on one installation shared a device record")
	}
	localList, e := s.Devices(ctx, login.AccountToken, installation)
	if e != nil || len(localList) != 2 || localList[0].ID != local.ID && localList[1].ID != local.ID {
		t.Fatal("the local account saw a Portico Account's device", localList, e)
	}
	hostedList, e := s.Devices(ctx, viewer.AccessToken, installation)
	if e != nil || len(hostedList) != 1 || hostedList[0].ID != hosted.ID {
		t.Fatal("the Portico Account saw a local device", hostedList, e)
	}
	// Neither can act on the other's record.
	if e = s.SignOutDevice(ctx, login.AccountToken, hosted.ID); !errors.Is(e, ErrDeviceUnknown) {
		t.Fatal("a local account signed out a Portico Account's device", e)
	}
	if e = s.SignOutDevice(ctx, viewer.AccessToken, local.ID); !errors.Is(e, ErrDeviceUnknown) {
		t.Fatal("a Portico Account signed out a local device", e)
	}
}

// Revocation has to be real: signing the device out ends exactly the hosted viewing session
// bound to it, and the Top Shelf token issued for it dies with it.
func TestSigningOutAHostedDeviceEndsThatViewingSession(t *testing.T) {
	ctx := context.Background()
	s, db, _ := familyFixture(t)
	installation := Token()
	viewer := hostedViewer(t, s, db, "hosted-account", "hosted-profile", "owner", installation)
	device, e := s.RegisterDevice(ctx, viewer.AccessToken, registration(installation), "203.0.113.4:1")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.BindDeviceFamily(ctx, viewer.AccessToken, installation, viewer.SessionFamilyID); e != nil {
		t.Fatal(e)
	}
	principal, e := s.Authenticate(viewer.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	grant, e := s.IssueTopShelfToken(ctx, principal, installation, 0)
	if e != nil {
		t.Fatal("a Portico Account device got no Top Shelf token", e)
	}
	if resolved, err := s.TopShelfGrant(ctx, grant.Token); err != nil || resolved.Viewer.Authority != "hosted" || resolved.Viewer.AccountID != "hosted-account" || resolved.Viewer.Role != "owner" {
		t.Fatalf("the feed token did not resolve to its viewer: %+v %v", resolved, err)
	}
	if e = s.SignOutDevice(ctx, viewer.AccessToken, device.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(viewer.AccessToken); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("the hosted viewing session survived its device being signed out", e)
	}
	if _, e = s.TopShelfGrant(ctx, grant.Token); !errors.Is(e, ErrTopShelfToken) {
		t.Fatal("the Top Shelf feed outlived the device", e)
	}
	// The record itself stays, so the person keeps seeing it and signing back in on it does
	// not look like a new device.
	var remaining int
	if e = db.QueryRow(`SELECT count(*) FROM identity_devices WHERE id=?`, device.ID).Scan(&remaining); e != nil || remaining != 1 {
		t.Fatal("signing out removed the record", remaining, e)
	}
}

// Owner approval is the owner's policy, not the local accounts' policy: a Portico Account
// device is held pending in exactly the same way, and denying it ends its session.
func TestOwnerApprovalAppliesToHostedDevicesToo(t *testing.T) {
	ctx := context.Background()
	s, db, _ := familyFixture(t)
	currentInstallation := Token()
	viewer := hostedViewer(t, s, db, "hosted-account", "hosted-profile", "owner", currentInstallation)
	if e := s.SetDeviceApprovalPolicy(DeviceApprovalOwner); e != nil {
		t.Fatal(e)
	}
	installation := Token()
	device, e := s.RegisterDevice(ctx, viewer.AccessToken, registration(installation), "203.0.113.5:1")
	if !errors.Is(e, ErrDevicePending) {
		t.Fatal("a new Portico Account device was admitted under owner approval", e)
	}
	if device.ApprovalState != "pending" {
		t.Fatalf("%+v", device)
	}
	// A pending device cannot bind a session or take a feed token.
	if e = s.BindDeviceFamily(ctx, viewer.AccessToken, installation, viewer.SessionFamilyID); !errors.Is(e, ErrDevicePending) {
		t.Fatal("a pending device bound a session", e)
	}
	principal, e := s.Authenticate(viewer.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.IssueTopShelfToken(ctx, principal, installation, 0); !errors.Is(e, ErrDevicePending) {
		t.Fatal("a pending device got a Top Shelf token", e)
	}
	approved, e := s.ApproveDevice(ctx, viewer.AccessToken, device.ID, true)
	if e != nil || approved.ApprovalState != "approved" {
		t.Fatal(approved, e)
	}
	newViewer := hostedViewer(t, s, db, "hosted-account", "hosted-profile", "owner", installation)
	if _, e = s.ApproveDevice(ctx, viewer.AccessToken, device.ID, false); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(newViewer.AccessToken); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("denying the device left its session alive", e)
	}
	if _, e = s.Authenticate(viewer.AccessToken); e != nil {
		t.Fatal("denying another device ended the current one", e)
	}
}

// Managing a device — approving, signing out, forgetting — is the account's managing
// principal's job. For a local account that is the primary profile; for a Portico Account the
// policy's own role is the same distinction, so a member can register and list but not decide.
func TestAHostedMemberRegistersButDoesNotManage(t *testing.T) {
	ctx := context.Background()
	s, db, _ := familyFixture(t)
	installation := Token()
	member := hostedViewer(t, s, db, "hosted-account", "member-profile", "member", installation)
	device, e := s.RegisterDevice(ctx, member.AccessToken, registration(installation), "203.0.113.6:1")
	if e != nil {
		t.Fatal("a member could not register its own device", e)
	}
	if list, err := s.Devices(ctx, member.AccessToken, installation); err != nil || len(list) != 1 {
		t.Fatal("a member could not list its devices", list, err)
	}
	if _, e = s.EditDevice(ctx, member.AccessToken, device.ID, DeviceEdit{Name: stringPointer("Bedroom")}); e != nil {
		t.Fatal("a member could not rename its own device", e)
	}
	trusted := true
	if _, e = s.EditDevice(ctx, member.AccessToken, device.ID, DeviceEdit{Trusted: &trusted}); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("a member promoted its own device past the owner's policy", e)
	}
	for name, act := range map[string]func() error{
		"approve": func() error { _, err := s.ApproveDevice(ctx, member.AccessToken, device.ID, true); return err },
		"signout": func() error { return s.SignOutDevice(ctx, member.AccessToken, device.ID) },
		"forget":  func() error { return s.ForgetDevice(ctx, member.AccessToken, device.ID) },
	} {
		if err := act(); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("%s: a member acted as the managing principal: %v", name, err)
		}
	}
}

func stringPointer(v string) *string { return &v }
