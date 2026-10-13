package directory

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/trust"
	"testing"
	"time"
)

func TestRevokedSignerCannotSupplyNewDirectoryConsent(t *testing.T) {
	rootPub, root, _ := ed25519.GenerateKey(rand.Reader)
	oldPub, old, _ := ed25519.GenerateKey(rand.Reader)
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC()
	before, after := now.Add(-time.Hour), now.Add(time.Hour)
	oldCert, e := trust.Certify(root, oldPub, "old", "documents", before, after, 1, nil)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := trust.Certify(root, pub, "current", "documents", before, after, 2, []string{"old"})
	if e != nil {
		t.Fatal(e)
	}
	payload, _ := json.Marshal(IdentityClaims{Kind: IdentityKind, AccountID: "member", ServerID: "server", IssuedAt: now, ExpiresAt: now.Add(IdentityLifetime)})
	envelope, e := trust.Sign(old, oldCert, payload)
	if e != nil {
		t.Fatal(e)
	}
	// The old certificate still verifies historically. Current revocation must
	// prevent using it to add a new member or accept custody.
	if _, e = envelope.Verify(rootPub, "documents", now); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(envelope)
	s := &Service{keys: &security.Keys{RootPublicKey: rootPub, PrivateKey: key, KeyID: "current", Certificate: cert}}
	if at, _, _ := s.verifyConsent(MemberStanding{AccountID: "member", Consent: raw}, "server"); !at.IsZero() {
		t.Fatal("revoked signer accepted")
	}
	envelope, e = trust.Sign(key, cert, payload)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(envelope)
	if at, kind, _ := s.verifyConsent(MemberStanding{AccountID: "member", Consent: raw}, "server"); at.IsZero() || kind != IdentityKind {
		t.Fatal(at, kind)
	}
}
