package hosted

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"portico.local/server/internal/hostedtrust"
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
