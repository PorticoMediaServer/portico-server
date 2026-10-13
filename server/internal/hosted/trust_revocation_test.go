package hosted

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"portico.local/server/internal/hostedtrust"
	"portico.local/server/internal/identity"
	"testing"
	"time"
)

func TestRootRevocationSurvivesRestartAndOldCertificateReplay(t *testing.T) {
	f := newClaimFixture(t)
	oldCert := testSigningCertificate(f.hostedKey, "pin1")
	old, _ := trust.Sign(f.hostedKey, oldCert, []byte("fixture"))
	if _, err := f.service.verifyEnvelope(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	public, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	next, err := trust.Certify(f.hostedKey, public, "next", "documents", now.Add(-time.Minute), now.Add(time.Hour), 2, []string{"pin1"})
	if err != nil {
		t.Fatal(err)
	}
	envelope, _ := trust.Sign(key, next, []byte("fixture"))
	if _, err = f.service.verifyEnvelope(t.Context(), envelope); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.verifyEnvelope(t.Context(), old); err == nil {
		t.Fatal("old certificate undid a known root revocation")
	}
	restarted, err := New(f.db, f.id, "http://127.0.0.1:19410", base64.RawURLEncoding.EncodeToString(f.hostedKey.Public().(ed25519.PublicKey)), trust.KeyID(f.hostedKey.Public().(ed25519.PublicKey)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.verifyEnvelope(t.Context(), old); err == nil {
		t.Fatal("revocation lost on restart")
	}
}

// A89: learning a root revocation revokes the Hosted-authority families of
// servers whose policy signer is revoked, never local (direct sign-in) ones.
func TestRootRevocationLeavesLocalFamilies(t *testing.T) {
	f := newClaimFixture(t)
	server := f.id.ID()
	if _, err := f.db.Exec(`INSERT INTO hosted_policy_signers(server_id,key_id) VALUES(?,'pin1')`, server); err != nil {
		t.Fatal(err)
	}
	horizon := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for _, row := range [][2]string{{"family-hosted", "hosted"}, {"family-local", "local"}} {
		if _, err := f.db.Exec(`INSERT INTO authorization_session_families(id,server_id,account_id,profile_id,authority,role,epoch,authorization_horizon,current_generation,revoked) VALUES(?,?,'account','profile',?,'member',1,?,1,0)`, row[0], server, row[1], horizon); err != nil {
			t.Fatal(err)
		}
	}
	public, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	next, err := trust.Certify(f.hostedKey, public, "next", "documents", now.Add(-time.Minute), now.Add(time.Hour), 2, []string{"pin1"})
	if err != nil {
		t.Fatal(err)
	}
	envelope, _ := trust.Sign(key, next, []byte("fixture"))
	if _, err = f.service.verifyEnvelope(t.Context(), envelope); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]int{"family-hosted": 1, "family-local": 0} {
		var revoked int
		if err = f.db.QueryRow(`SELECT revoked FROM authorization_session_families WHERE id=?`, id).Scan(&revoked); err != nil || revoked != want {
			t.Fatalf("%s revoked=%d want %d (%v)", id, revoked, want, err)
		}
	}
}

// A person signs in with a Portico Account; the assertion was signed by key
// "pin1". Later this server learns, from a newer certificate, that "pin1" was
// revoked. The session that assertion admitted ends, with every session
// derived from it, because whoever held the key may be the one signed in.
// Sessions that never depended on the key are left alone.
func TestLearningASigningKeyWasRevokedEndsTheSessionsItAdmitted(t *testing.T) {
	f := newClaimFixture(t)
	ctx := t.Context()
	addLinkedMember(t, f, "local-m", "acc_member")
	addLinkedMember(t, f, "local-n", "acc_other")
	var hooked []string
	f.id.OnFamilyRevokedTx = func(_ context.Context, _ *sql.Tx, family identity.FamilyState) error {
		hooked = append(hooked, family.ID)
		return nil
	}

	who, err := verify(f, freshClaims(f, "acc_member"))
	if err != nil || who.SignerKeyID != "pin1" {
		t.Fatalf("the verified assertion does not name its signing key: %+v %v", who.SignerKeyID, err)
	}
	admitted, err := f.id.PorticoSignIn(ctx, who)
	if err != nil || admitted.Session == nil {
		t.Fatal(err)
	}
	// A profile chosen from that sign-in is a session derived from it.
	chosen, err := f.id.SelectDirectProfile(ctx, admitted.Session.AccessToken, admitted.Account.PrimaryProfileID, identity.DirectSelection{})
	if err != nil {
		t.Fatal(err)
	}
	// Another member, admitted by a different key that stays good.
	other, err := f.id.PorticoSignIn(ctx, identity.PorticoIdentity{AccountID: "acc_other", SignerKeyID: "pin-good"})
	if err != nil || other.Session == nil {
		t.Fatal(err)
	}
	// A session that never involved Hosted at all.
	local, err := f.id.Issue("local-n", "local-n-profile", "local", identity.TierMember, 1)
	if err != nil {
		t.Fatal(err)
	}
	signer := func(family string) string {
		var key string
		if err := f.db.QueryRow(`SELECT COALESCE((SELECT signer_key_id FROM portico_admitted_families WHERE family_id=?),'<none>')`, family).Scan(&key); err != nil {
			t.Fatal(err)
		}
		return key
	}
	if signer(admitted.Session.SessionFamilyID) != "pin1" || signer(chosen.Session.SessionFamilyID) != "pin1" || signer(other.Session.SessionFamilyID) != "pin-good" || signer(local.SessionFamilyID) != "<none>" {
		t.Fatalf("recorded signing keys: %q %q %q %q", signer(admitted.Session.SessionFamilyID), signer(chosen.Session.SessionFamilyID), signer(other.Session.SessionFamilyID), signer(local.SessionFamilyID))
	}

	// A certificate that revokes nothing changes nothing.
	public, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	harmless, err := trust.Certify(f.hostedKey, public, "next", "documents", now.Add(-time.Minute), now.Add(time.Hour), 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope, _ := trust.Sign(key, harmless, []byte("fixture"))
	if _, err = f.service.verifyEnvelope(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	if _, err = f.id.Authenticate(chosen.Session.AccessToken); err != nil {
		t.Fatalf("a newer certificate that revokes nothing ended a session: %v", err)
	}

	// The revocation of pin1 arrives.
	revoking, err := trust.Certify(f.hostedKey, public, "next", "documents", now.Add(-time.Minute), now.Add(time.Hour), 3, []string{"pin1"})
	if err != nil {
		t.Fatal(err)
	}
	envelope, _ = trust.Sign(key, revoking, []byte("fixture"))
	if _, err = f.service.verifyEnvelope(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	if _, err = f.id.Authenticate(chosen.Session.AccessToken); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("a session derived from the revoked key's sign-in is still alive: %v", err)
	}
	for _, family := range []string{admitted.Session.SessionFamilyID, chosen.Session.SessionFamilyID} {
		var revoked int
		var reason string
		if err = f.db.QueryRow(`SELECT revoked,revoked_reason FROM authorization_session_families WHERE id=?`, family).Scan(&revoked, &reason); err != nil || revoked != 1 || reason != string(identity.RevokedSigningKeyRevoked) {
			t.Fatalf("family %s: revoked=%d reason=%q %v", family, revoked, reason, err)
		}
	}
	if len(hooked) != 2 {
		t.Fatalf("the playback clean-up ran for %d sessions, want the 2 that ended", len(hooked))
	}
	// The renewal credential of an ended session is dead too.
	if _, err = f.id.RefreshSession(ctx, chosen.Session.RefreshToken, chosen.Session.InstallationID, "renew-after-revocation", nil); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("an ended session renewed: %v", err)
	}
	// Untouched: the other key's session and the one that never used Hosted.
	if _, err = f.id.Authenticate(other.Session.AccessToken); err != nil {
		t.Fatalf("a session admitted by a key that is still good was ended: %v", err)
	}
	if _, err = f.id.Authenticate(local.AccessToken); err != nil {
		t.Fatalf("a session that never involved Hosted was ended: %v", err)
	}
	// And the revoked key admits nobody new.
	if _, err = verify(f, freshClaims(f, "acc_member")); err == nil {
		t.Fatal("the revoked key's assertion was accepted afterwards")
	}
}
