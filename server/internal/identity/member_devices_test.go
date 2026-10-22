package identity

import (
	"context"
	"errors"
	"testing"
)

func onDevice(t *testing.T, installation, name string) context.Context {
	t.Helper()
	ctx, err := WithIssuingDevice(context.Background(), DeviceRegistration{InstallationID: installation, Name: name, Platform: "ios", App: "portico", AppVersion: "1.0"}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

const (
	memberPhone = "installation-member-phone-0000000000000001"
	memberTV    = "installation-member-tv-000000000000000001"
	ownerPhone  = "installation-owner-phone-00000000000000001"
)

func ownerPrincipal(owner DirectSignIn) Principal {
	return Principal{Viewer: owner.Session.Viewer, Hash: Digest(owner.Session.AccessToken), Epoch: 1}
}

// With "require approval" on, a member's new device waits, the owner sees it
// and approves it, and only then can it be used. The owner's own devices are
// never held.
func TestRequiringApprovalHoldsAMembersNewDeviceUntilTheOwnerApprovesIt(t *testing.T) {
	ctx := context.Background()
	s, owner, member, already := memberWithSession(t)
	if err := s.SetDeviceApprovalPolicy(DeviceApprovalOwner); err != nil {
		t.Fatal(err)
	}
	// A device that was in use before the policy was turned on stays approved.
	if _, err := s.Authenticate(already.Session.AccessToken); err != nil {
		t.Fatalf("turning approval on signed out a device already in use: %v", err)
	}
	waiting, err := s.DirectLogin(onDevice(t, memberPhone, "Member's phone"), "member", "Testing2!")
	if err != nil || !waiting.DeviceApprovalPending || waiting.Session != nil || waiting.DeviceID == "" {
		t.Fatalf("a member's new device under the policy: %+v %v", waiting, err)
	}
	// Trying again does not get around it.
	again, err := s.DirectLogin(onDevice(t, memberPhone, "Member's phone"), "member", "Testing2!")
	if err != nil || !again.DeviceApprovalPending || again.Session != nil || again.DeviceID != waiting.DeviceID {
		t.Fatalf("a second attempt on the waiting device: %+v %v", again, err)
	}
	// The owner's own new device is not held.
	ownerLogin, err := s.DirectLogin(onDevice(t, ownerPhone, "Owner's phone"), "owner", "Testing1!")
	if err != nil || ownerLogin.DeviceApprovalPending || ownerLogin.Session == nil {
		t.Fatalf("the owner's new device was held: %+v %v", ownerLogin, err)
	}

	actor := ownerPrincipal(owner)
	page, err := s.MemberDevices(ctx, actor, MemberDeviceQuery{Trust: "pending"})
	if err != nil || !page.ApprovalRequired || len(page.Items) != 1 {
		t.Fatalf("the owner's list of waiting devices: %+v %v", page, err)
	}
	device := page.Items[0]
	if device.ID != waiting.DeviceID || device.AccountID != member.ID || device.Username != "member" || device.Name != "Member's phone" || device.Trust != "pending" || device.Revision != 1 {
		t.Fatalf("the waiting device as listed: %+v", device)
	}
	// A member may not see or decide about anyone's devices here.
	asMember := Principal{Viewer: Viewer{AccountID: member.ID, ProfileID: member.PrimaryProfileID, Authority: "local", Role: TierMember}}
	if _, err = s.MemberDevices(ctx, asMember, MemberDeviceQuery{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a member listed members' devices: %v", err)
	}
	if _, err = s.SetMemberDeviceTrust(ctx, asMember, device.ID, MemberDeviceTrust{ExpectedRevision: 1, Trust: "approved"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a member approved a device: %v", err)
	}
	// Bad input and unknown devices.
	if _, err = s.SetMemberDeviceTrust(ctx, actor, device.ID, MemberDeviceTrust{ExpectedRevision: 1, Trust: "trusted"}); !errors.Is(err, ErrDeviceInput) {
		t.Fatalf("an unknown decision: %v", err)
	}
	if _, err = s.SetMemberDeviceTrust(ctx, actor, "no-such-device", MemberDeviceTrust{ExpectedRevision: 1, Trust: "approved"}); !errors.Is(err, ErrDeviceUnknown) {
		t.Fatalf("an unknown device: %v", err)
	}
	var changed *DeviceChangedError
	if _, err = s.SetMemberDeviceTrust(ctx, actor, device.ID, MemberDeviceTrust{ExpectedRevision: 9, Trust: "approved"}); !errors.As(err, &changed) || changed.CurrentRevision != 1 {
		t.Fatalf("a decision against an older state of the device: %v", err)
	}

	approved, err := s.SetMemberDeviceTrust(ctx, actor, device.ID, MemberDeviceTrust{ExpectedRevision: 1, Trust: "approved"})
	if err != nil || approved.Trust != "approved" || approved.Revision != 2 {
		t.Fatalf("approve: %+v %v", approved, err)
	}
	// The same decision again is the same answer, not a conflict.
	if repeat, err := s.SetMemberDeviceTrust(ctx, actor, device.ID, MemberDeviceTrust{ExpectedRevision: 1, Trust: "approved"}); err != nil || repeat.Revision != 2 {
		t.Fatalf("a repeated approval: %+v %v", repeat, err)
	}
	signedIn, err := s.DirectLogin(onDevice(t, memberPhone, "Member's phone"), "member", "Testing2!")
	if err != nil || signedIn.DeviceApprovalPending || signedIn.Session == nil {
		t.Fatalf("the approved device: %+v %v", signedIn, err)
	}

	// Blocking it ends its sessions at once and keeps it out.
	blocked, err := s.SetMemberDeviceTrust(ctx, actor, device.ID, MemberDeviceTrust{ExpectedRevision: 2, Trust: "blocked"})
	if err != nil || blocked.Trust != "blocked" {
		t.Fatalf("block: %+v %v", blocked, err)
	}
	if _, err = s.Authenticate(signedIn.Session.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("a blocked device kept its session: %v", err)
	}
	var reason string
	if err = s.db.QueryRow(`SELECT revoked_reason FROM authorization_session_families WHERE id=?`, signedIn.Session.SessionFamilyID).Scan(&reason); err != nil || reason != string(RevokedDeviceDisapproved) {
		t.Fatalf("the blocked device's session ended as %q: %v", reason, err)
	}
	if _, err = s.DirectLogin(onDevice(t, memberPhone, "Member's phone"), "member", "Testing2!"); !errors.Is(err, ErrDeviceDenied) {
		t.Fatalf("a blocked device signed in: %v", err)
	}
	// The member's other device, in use all along, is untouched.
	if _, err = s.Authenticate(already.Session.AccessToken); err != nil {
		t.Fatalf("blocking one device ended another: %v", err)
	}
}

// Turning the policy off lets a waiting device in. A blocked device stays blocked.
func TestTurningApprovalOffAdmitsWaitingDevicesButNotBlockedOnes(t *testing.T) {
	ctx := context.Background()
	s, owner, _, _ := memberWithSession(t)
	if err := s.SetDeviceApprovalPolicy(DeviceApprovalOwner); err != nil {
		t.Fatal(err)
	}
	waiting, err := s.DirectLogin(onDevice(t, memberPhone, "Phone"), "member", "Testing2!")
	if err != nil || !waiting.DeviceApprovalPending {
		t.Fatalf("%+v %v", waiting, err)
	}
	other, err := s.DirectLogin(onDevice(t, memberTV, "Television"), "member", "Testing2!")
	if err != nil || !other.DeviceApprovalPending {
		t.Fatalf("%+v %v", other, err)
	}
	if _, err = s.SetMemberDeviceTrust(ctx, ownerPrincipal(owner), other.DeviceID, MemberDeviceTrust{ExpectedRevision: 1, Trust: "blocked"}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetDeviceApprovalPolicy(DeviceApprovalAuto); err != nil {
		t.Fatal(err)
	}
	if in, err := s.DirectLogin(onDevice(t, memberPhone, "Phone"), "member", "Testing2!"); err != nil || in.Session == nil {
		t.Fatalf("a waiting device after the policy was turned off: %+v %v", in, err)
	}
	if _, err = s.DirectLogin(onDevice(t, memberTV, "Television"), "member", "Testing2!"); !errors.Is(err, ErrDeviceDenied) {
		t.Fatalf("a blocked device after the policy was turned off: %v", err)
	}
}

// An administrator decides about plain members' devices only: not another
// administrator's, and not its own.
func TestAnAdministratorDecidesOnlyAboutMembersDevices(t *testing.T) {
	ctx := context.Background()
	s, owner, member, _ := memberWithSession(t)
	admin, err := s.CreateDirectMember(ctx, owner.AccountToken, DirectMemberCreate{Username: "admin", Password: "Testing6!", Name: "Admin", AllowedLibraries: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE direct_memberships SET role='admin' WHERE account_id=?`, admin.ID); err != nil {
		t.Fatal(err)
	}
	adminLogin, err := s.DirectLogin(onDevice(t, "installation-admin-phone-00000000000000001", "Admin's phone"), "admin", "Testing6!")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DirectLogin(onDevice(t, ownerPhone, "Owner's phone"), "owner", "Testing1!"); err != nil {
		t.Fatal(err)
	}
	actor, err := s.Authenticate(adminLogin.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.MemberDevices(ctx, actor, MemberDeviceQuery{})
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("an administrator's list: %+v %v", page, err)
	}
	for _, d := range page.Items {
		if d.AccountID != member.ID {
			t.Fatalf("an administrator was shown a device it may not decide about: %+v", d)
		}
	}
	all, err := s.MemberDevices(ctx, ownerPrincipal(owner), MemberDeviceQuery{})
	if err != nil {
		t.Fatal(err)
	}
	decided := 0
	for _, d := range all.Items {
		_, err = s.SetMemberDeviceTrust(ctx, actor, d.ID, MemberDeviceTrust{ExpectedRevision: d.Revision, Trust: "blocked"})
		if d.AccountID == member.ID {
			if err != nil {
				t.Fatalf("an administrator blocking a member's device: %v", err)
			}
			decided++
			continue
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("an administrator blocking %s's device: %v", d.Username, err)
		}
	}
	if decided == 0 || len(all.Items) <= decided {
		t.Fatalf("the fixture did not cover both cases: %d of %d", decided, len(all.Items))
	}
	// Paging returns every device once.
	seen := map[string]bool{}
	cursor := ""
	for {
		p, err := s.MemberDevices(ctx, ownerPrincipal(owner), MemberDeviceQuery{Limit: "1", Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range p.Items {
			if seen[d.ID] {
				t.Fatalf("device %s listed twice", d.ID)
			}
			seen[d.ID] = true
		}
		if p.NextCursor == "" {
			break
		}
		cursor = p.NextCursor
	}
	if len(seen) != len(all.Items) {
		t.Fatalf("paging saw %d devices of %d", len(seen), len(all.Items))
	}
	if _, err = s.MemberDevices(ctx, ownerPrincipal(owner), MemberDeviceQuery{Cursor: "not a cursor"}); !errors.Is(err, ErrDeviceInput) {
		t.Fatalf("a malformed cursor: %v", err)
	}
	if _, err = s.MemberDevices(ctx, ownerPrincipal(owner), MemberDeviceQuery{Limit: "5000"}); !errors.Is(err, ErrDeviceInput) {
		t.Fatalf("an oversized page: %v", err)
	}
}

// A television signed in by code under the policy: the member's approval of
// the code is not the owner's approval of the device. The television's record
// is kept so the owner can approve it, and the same code then works.
func TestATelevisionCodeWaitsForTheOwnerWhenDevicesNeedApproval(t *testing.T) {
	ctx := context.Background()
	s, owner, member, already := memberWithSession(t)
	if err := s.SetDeviceApprovalPolicy(DeviceApprovalOwner); err != nil {
		t.Fatal(err)
	}
	tv := onDevice(t, memberTV, "Living room")
	code, err := s.StartQuick(tv, QuickStart{RequestID: "00000000-0000-4000-8000-000000000001", DeviceName: "Living room", Platform: "tvos", AppVersion: "1.0"}, true)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.ReviewQuick(ctx, code.UserCode)
	if err != nil {
		t.Fatal(err)
	}
	approver, err := s.Authenticate(already.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DecideQuick(ctx, approver, preview.RequestID, code.UserCode, "approve"); err != nil {
		t.Fatal(err)
	}
	poll := func() (Envelope, error) {
		if _, err := s.db.Exec(`UPDATE quick_connect_v1 SET next_poll=0`); err != nil {
			t.Fatal(err)
		}
		return s.PollQuick(tv, code.DeviceCode)
	}
	if _, err = poll(); !errors.Is(err, ErrDevicePending) {
		t.Fatalf("a television collecting before the owner approved it: %v", err)
	}
	page, err := s.MemberDevices(ctx, ownerPrincipal(owner), MemberDeviceQuery{Trust: "pending", Account: member.ID})
	if err != nil || len(page.Items) != 1 || page.Items[0].Name != "Living room" {
		t.Fatalf("the waiting television is not there for the owner to approve: %+v %v", page, err)
	}
	// Still waiting on a second poll, and no session was created meanwhile.
	if _, err = poll(); !errors.Is(err, ErrDevicePending) {
		t.Fatalf("a second poll: %v", err)
	}
	var families int
	if err = s.db.QueryRow(`SELECT count(*) FROM authorization_session_families f JOIN identity_device_families b ON b.family_id=f.id WHERE b.device_id=?`, page.Items[0].ID).Scan(&families); err != nil || families != 0 {
		t.Fatalf("a waiting television holds %d sessions: %v", families, err)
	}
	if _, err = s.SetMemberDeviceTrust(ctx, ownerPrincipal(owner), page.Items[0].ID, MemberDeviceTrust{ExpectedRevision: page.Items[0].Revision, Trust: "approved"}); err != nil {
		t.Fatal(err)
	}
	session, err := poll()
	if err != nil || session.AccessToken == "" || session.Viewer.AccountID != member.ID {
		t.Fatalf("the television after the owner approved it: %+v %v", session.Viewer, err)
	}
	if _, err = s.Authenticate(session.AccessToken); err != nil {
		t.Fatalf("the television's session: %v", err)
	}
}
