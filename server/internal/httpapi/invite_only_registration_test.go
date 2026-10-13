package httpapi

import (
	"context"
	"encoding/json"
	"testing"

	"portico.local/server/internal/access"
	"portico.local/server/internal/identity"
)

// Sign-up set to "invitation only", wired as the server wires it: an account
// is created only against an unspent member invitation, gets that
// invitation's libraries, and the invitation is spent exactly once.
func TestInviteOnlySignUpCreatesAnAccountAgainstAnInvitation(t *testing.T) {
	f := memberLimitsFixture(t, 0)
	f.id.RedeemInvitation = f.deps.Access.Access.RegistrationRedeemer()
	if err := f.id.SetSelfRegistration(identity.SelfRegistrationInvite); err != nil {
		t.Fatal(err)
	}
	owner := identity.Principal{Viewer: f.owner.Viewer}
	invite, err := f.deps.Access.Access.Invite(context.Background(), nil, owner, access.InvitationRequest{OperationID: "sign-up-invite-1", Role: identity.TierMember, AllowedLibraries: []string{f.library}})
	if err != nil {
		t.Fatal(err)
	}
	register := func(username, code string) (int, string, identity.DirectSignIn) {
		w := f.raw("POST", "/v1/auth/register", "", nil, map[string]string{"username": username, "password": "Testing1!", "invitationCode": code})
		var out identity.DirectSignIn
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, v1Code(w), out
	}
	if code, name, _ := register("nocode", ""); code != 403 || name != "registration_invitation_required" {
		t.Fatalf("sign-up with no invitation: %d %s", code, name)
	}
	if code, name, _ := register("badcode", "this-is-not-an-invitation-code-at-all-0000"); code != 403 || name != "registration_invitation_required" {
		t.Fatalf("sign-up with a made-up invitation: %d %s", code, name)
	}
	// A taken username does not cost the invitee their invitation.
	if code, name, _ := register("owner", invite.Code); code != 409 || name != "username_taken" {
		t.Fatalf("sign-up with a taken username: %d %s", code, name)
	}
	code, _, out := register("invited", invite.Code)
	if code != 201 || out.Session == nil || out.Account.Role != identity.TierMember || len(out.Account.AllowedLibraries) != 1 || out.Account.AllowedLibraries[0] != f.library {
		t.Fatalf("sign-up with the invitation: %d %+v", code, out.Account)
	}
	tokenWorks(t, f, out.Session.AccessToken, 200)
	if code, name, _ := register("second", invite.Code); code != 403 || name != "registration_invitation_required" {
		t.Fatalf("the same invitation used twice: %d %s", code, name)
	}
	var accounts int
	if err = f.db.QueryRow(`SELECT count(*) FROM accounts WHERE username IN('nocode','badcode','second')`).Scan(&accounts); err != nil || accounts != 0 {
		t.Fatalf("a refused sign-up left %d accounts: %v", accounts, err)
	}
}
