package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func issueCastTarget(t *testing.T, s *Service, ctx context.Context, account, profile, derivedFrom string) Envelope {
	t.Helper()
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	out, err := s.IssueCastTargetTx(ctx, tx, "local", account, profile, 1, time.Time{}, derivedFrom)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return out
}

// A Cast target's session, whoever paired it: a member's, playback only, not
// held for the owner's approval, and never widened afterwards.
func TestACastTargetSessionIsAMembersAndOnlyForPlayback(t *testing.T) {
	ctx := context.Background()
	s, _, owner := directFixture(t)
	linkedMember(t, s, "linked", "acc_linked")
	pairing, err := s.PorticoSignIn(ctx, PorticoIdentity{AccountID: "acc_linked", Username: "linked", SignerKeyID: "key-cast"})
	if err != nil || pairing.Session == nil {
		t.Fatalf("pairing: %+v %v", pairing, err)
	}
	// A member's screen is not held by the approval rule: the paired device vouches for it.
	screenCtx := onDevice(t, "cast-0000000000000000000000000000000000000001", "Living room TV")
	member := issueCastTarget(t, s, screenCtx, "linked", "linked-profile", pairing.Session.SessionFamilyID)
	// The owner's screen is a member's too.
	ownerScreen := issueCastTarget(t, s, onDevice(t, "cast-0000000000000000000000000000000000000002", "Den TV"), owner.Account.ID, owner.Account.PrimaryProfileID, "")
	if ownerScreen.Viewer.Role != TierMember || member.Viewer.Role != TierMember || ownerScreen.RefreshToken != "" {
		t.Fatalf("issued: owner's screen role=%q refresh=%q; member's screen role=%q", ownerScreen.Viewer.Role, ownerScreen.RefreshToken, member.Viewer.Role)
	}

	// Outside playback it is refused, as a refusal and not as a failed sign-in.
	if _, err = s.AuthenticateContext(ctx, ownerScreen.AccessToken); !errors.Is(err, ErrForbidden) {
		t.Fatalf("the owner's screen on a request that is not playback: %v", err)
	}
	if status, _, _ := Refusal(err); status != 403 {
		t.Fatalf("the refusal answers %d", status)
	}
	p, err := s.AuthenticateContext(WithPlaybackRequest(ctx), ownerScreen.AccessToken)
	if err != nil || p.Role != TierMember || p.Grant != GrantCastTarget || p.AccountID != owner.Account.ID {
		t.Fatalf("the owner's screen on a playback request: %+v %v", p, err)
	}
	// No account route is open to it, playback request or not.
	if _, err = s.DirectMe(WithPlaybackRequest(ctx), ownerScreen.AccessToken); !errors.Is(err, ErrForbidden) {
		t.Fatalf("the screen read the owner's account: %v", err)
	}
	if _, err = s.SelectDirectProfile(WithPlaybackRequest(ctx), ownerScreen.AccessToken, owner.Account.PrimaryProfileID, DirectSelection{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("the screen chose a profile and got an ordinary session: %v", err)
	}
	if _, err = s.DirectMembers(WithPlaybackRequest(ctx), ownerScreen.AccessToken); !errors.Is(err, ErrForbidden) {
		t.Fatalf("the screen listed members: %v", err)
	}
	if _, err = s.Devices(WithPlaybackRequest(ctx), ownerScreen.AccessToken, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("the screen listed devices: %v", err)
	}
	// A change of the account's tier does not reach it.
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = RetierFamiliesTx(ctx, tx, owner.Account.ID, TierOwner); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if p, err = s.AuthenticateContext(WithPlaybackRequest(ctx), ownerScreen.AccessToken); err != nil || p.Role != TierMember {
		t.Fatalf("the screen after its account's sessions were re-tiered: %+v %v", p.Role, err)
	}
	// It ends with the signing key behind the session that paired it.
	if n := revokeKey(t, s, "key-cast"); n != 2 {
		t.Fatalf("revoking the pairing session's signing key ended %d sessions, want the phone's and its screen's", n)
	}
	if _, err = s.AuthenticateContext(WithPlaybackRequest(ctx), member.AccessToken); !errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden) {
		t.Fatalf("the member's screen after the key was revoked: %v", err)
	}
	if _, err = s.AuthenticateContext(WithPlaybackRequest(ctx), ownerScreen.AccessToken); err != nil {
		t.Fatalf("the owner's screen, which no key admitted, was ended: %v", err)
	}
	// An ordinary session is not affected by any of this.
	if _, err = s.AuthenticateContext(ctx, owner.Session.AccessToken); err != nil {
		t.Fatalf("an ordinary session outside a playback request: %v", err)
	}
}

// A device-token-authorized reconnect can validate the exact target family
// after its access token expires, without authenticating that expired bearer.
func TestCastTargetParentAllowsOnlyAccessExpiry(t *testing.T) {
	ctx := context.Background()
	s, db, owner := directFixture(t)
	screen := issueCastTarget(t, s, onDevice(t, "cast-parent-test-1", "TV"), owner.Account.ID, owner.Account.PrimaryProfileID, owner.Session.SessionFamilyID)
	expired := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE authorization_family_tokens SET expires_at=? WHERE family_id=?; UPDATE authorization_access SET expires_at=? WHERE hash=?`, expired, screen.SessionFamilyID, expired, Digest(screen.AccessToken)); err != nil {
		t.Fatal(err)
	}
	expected := Principal{Viewer: screen.Viewer, Epoch: 1, Grant: GrantCastTarget}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	facts, err := s.CastTargetParentTx(ctx, tx, screen.SessionFamilyID, expected)
	if err != nil || facts.ID != screen.SessionFamilyID || facts.DeviceID != screen.DeviceID {
		t.Fatalf("valid parent: %+v %v", facts, err)
	}
	if _, err = s.FamilyAuthorityTx(ctx, tx, screen.SessionFamilyID, expected); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("ordinary family accepted expired token: %v", err)
	}
	if _, err = tx.Exec(`UPDATE authorization_session_families SET revoked=1 WHERE id=?`, screen.SessionFamilyID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CastTargetParentTx(ctx, tx, screen.SessionFamilyID, expected); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked parent: %v", err)
	}
}

func TestCastTargetParentRefusesChangedFactsAndRetiredToken(t *testing.T) {
	ctx := context.Background()
	s, db, owner := directFixture(t)
	screen := issueCastTarget(t, s, onDevice(t, "cast-parent-fence-1", "TV"), owner.Account.ID, owner.Account.PrimaryProfileID, owner.Session.SessionFamilyID)
	expected := Principal{Viewer: screen.Viewer, Epoch: 1, Grant: GrantCastTarget}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	changed := expected
	changed.ProfileID = "other-profile"
	if _, err = s.CastTargetParentTx(ctx, tx, screen.SessionFamilyID, changed); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("changed profile: %v", err)
	}
	changed = expected
	changed.Role = TierOwner
	if _, err = s.CastTargetParentTx(ctx, tx, screen.SessionFamilyID, changed); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("changed role: %v", err)
	}
	changed = expected
	changed.Epoch++
	if _, err = s.CastTargetParentTx(ctx, tx, screen.SessionFamilyID, changed); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("changed epoch: %v", err)
	}
	changed = expected
	changed.Grant = ""
	if _, err = s.CastTargetParentTx(ctx, tx, screen.SessionFamilyID, changed); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("changed grant: %v", err)
	}
	if _, err = tx.Exec(`UPDATE authorization_family_tokens SET retired=1 WHERE family_id=?`, screen.SessionFamilyID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CastTargetParentTx(ctx, tx, screen.SessionFamilyID, expected); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("retired token: %v", err)
	}
}

func TestCastTargetLocalParentHorizonDoesNotExtend(t *testing.T) {
	ctx := context.Background()
	s, db, owner := directFixture(t)
	horizon := time.Now().Add(6 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE authorization_session_families SET authorization_horizon=? WHERE id=?`, horizon, owner.Session.SessionFamilyID); err != nil {
		t.Fatal(err)
	}
	child := issueCastTarget(t, s, onDevice(t, "cast-local-horizon-1", "TV"), owner.Account.ID, owner.Account.PrimaryProfileID, owner.Session.SessionFamilyID)
	var childHorizon string
	if err := db.QueryRow(`SELECT authorization_horizon FROM authorization_session_families WHERE id=?`, child.SessionFamilyID).Scan(&childHorizon); err != nil {
		t.Fatal(err)
	}
	if childHorizon > horizon {
		t.Fatalf("child horizon %q exceeds original %q", childHorizon, horizon)
	}
}
