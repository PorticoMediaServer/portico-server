package httpapi

import (
	"encoding/json"
	"testing"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

// The owner's "require approval for new devices" switch, end to end: saved in
// settings, it holds a member's next new device; the device shows up in the
// members' device list; approving it there lets it sign in.
func TestDeviceApprovalSwitchHoldsNewDevicesAndTheListApprovesThem(t *testing.T) {
	f := memberLimitsFixture(t, 0)
	f.deps.Console = operations.New(f.db)
	f.handler = New(f.deps)
	f.deps.v1 = f.v1
	passwordAccount(t, f, "newcomer", "Testing1!")
	phone := map[string]string{"X-Portico-Installation-Id": "installation-newcomer-phone-000000000001", "X-Portico-Device-Name": "Newcomer's phone", "X-Portico-Device-Platform": "ios", "X-Portico-App": "portico"}
	signIn := func() (int, identity.DirectSignIn) {
		w := f.raw("POST", "/v1/direct/sign-in", "", phone, map[string]string{"username": "newcomer", "password": "Testing1!"})
		var out identity.DirectSignIn
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	// Off by default: a new device signs straight in.
	if code, out := signIn(); code != 200 || out.Session == nil {
		t.Fatalf("sign-in with approval off: %d %+v", code, out)
	}
	// The owner turns it on through the settings document.
	settings, err := f.deps.Console.Settings(t.Context(), operations.AllowServerScope)
	if err != nil {
		t.Fatal(err)
	}
	values := settings.Requested
	values.DeviceApprovalRequired = true
	owner := identity.Principal{Viewer: f.owner.Viewer}
	if _, err = f.deps.Console.ApplySettings(t.Context(), owner, operations.AllowServerScope, operations.SettingsChange{ExpectedRevision: settings.Revision, IdempotencyKey: "approval-on-0001", Values: values}); err != nil {
		t.Fatal(err)
	}
	// The phone already in use stays approved; a second device is held.
	if code, out := signIn(); code != 200 || out.Session == nil {
		t.Fatalf("a device already in use was held when approval was turned on: %d %+v", code, out)
	}
	phone["X-Portico-Installation-Id"] = "installation-newcomer-tablet-00000000001"
	phone["X-Portico-Device-Name"] = "Newcomer's tablet"
	code, held := signIn()
	if code != 200 || !held.DeviceApprovalPending || held.Session != nil {
		t.Fatalf("a new device with approval on: %d %+v", code, held)
	}

	w := f.raw("GET", "/v1/admin/access/devices?trust=pending", f.owner.AccessToken, nil, nil)
	var page identity.MemberDevicePage
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != 200 || !page.ApprovalRequired || len(page.Items) != 1 || page.Items[0].Name != "Newcomer's tablet" || page.Items[0].Username != "newcomer" || page.Items[0].Trust != "pending" {
		t.Fatalf("the owner's list of waiting devices: %d %s", w.Code, w.Body.String())
	}
	device := page.Items[0]
	if w = f.raw("GET", "/v1/admin/access/devices?trust=nonsense", f.owner.AccessToken, nil, nil); w.Code != 400 {
		t.Fatalf("an unknown filter: %d %s", w.Code, w.Body.String())
	}
	if w = f.raw("GET", "/v1/admin/access/devices", "", nil, nil); w.Code != 401 {
		t.Fatalf("no session: %d %s", w.Code, w.Body.String())
	}
	path := "/v1/admin/access/devices/" + device.ID + "/trust"
	if w = f.raw("PUT", path, f.owner.AccessToken, nil, map[string]any{"expectedRevision": device.Revision + 3, "operationId": "trust-stale-0001", "trust": "approved"}); w.Code != 412 || v1Code(w) != "revision_mismatch" {
		t.Fatalf("a decision against an older state: %d %s", w.Code, w.Body.String())
	}
	if w = f.raw("PUT", path, f.owner.AccessToken, nil, map[string]any{"expectedRevision": device.Revision, "operationId": "trust-bad-0001", "trust": "trusted"}); w.Code != 400 {
		t.Fatalf("an unknown decision: %d %s", w.Code, w.Body.String())
	}
	if w = f.raw("PUT", "/v1/admin/access/devices/no-such-device/trust", f.owner.AccessToken, nil, map[string]any{"expectedRevision": 1, "operationId": "trust-miss-0001", "trust": "approved"}); w.Code != 404 {
		t.Fatalf("an unknown device: %d %s", w.Code, w.Body.String())
	}
	w = f.raw("PUT", path, f.owner.AccessToken, nil, map[string]any{"expectedRevision": device.Revision, "operationId": "trust-approve-0001", "trust": "approved"})
	var approved identity.MemberDevice
	if err = json.Unmarshal(w.Body.Bytes(), &approved); err != nil || w.Code != 200 || approved.Trust != "approved" || approved.Revision != device.Revision+1 {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}
	code, in := signIn()
	if code != 200 || in.Session == nil {
		t.Fatalf("the approved device: %d %+v", code, in)
	}
	// Blocking it signs it out.
	if w = f.raw("PUT", path, f.owner.AccessToken, nil, map[string]any{"expectedRevision": approved.Revision, "operationId": "trust-block-0001", "trust": "blocked"}); w.Code != 200 {
		t.Fatalf("block: %d %s", w.Code, w.Body.String())
	}
	tokenWorks(t, f, in.Session.AccessToken, 401)
	if code, _ = signIn(); code != 403 {
		t.Fatalf("a blocked device signing in: %d", code)
	}
}

// A member is refused both routes without being signed out.
func TestMembersDeviceRoutesAreForAdministrators(t *testing.T) {
	f := memberLimitsFixture(t, 0)
	member, _ := f.member()
	if w := f.raw("GET", "/v1/admin/access/devices", member, nil, nil); w.Code != 403 {
		t.Fatalf("a member listing members' devices: %d %s", w.Code, w.Body.String())
	}
	if w := f.raw("PUT", "/v1/admin/access/devices/anything/trust", member, nil, map[string]any{"expectedRevision": 1, "operationId": "trust-member-0001", "trust": "approved"}); w.Code != 403 {
		t.Fatalf("a member deciding about a device: %d %s", w.Code, w.Body.String())
	}
	tokenWorks(t, f, member, 200)
}
