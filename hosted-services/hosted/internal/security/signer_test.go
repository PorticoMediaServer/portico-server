package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"portico.local/hosted/trust"
	"testing"
	"time"
)

func TestSignerRechecksCertificateAfterStartup(t *testing.T) {
	rootPub, root, _ := ed25519.GenerateKey(rand.Reader)
	pub, private, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC()
	cert, e := trust.Certify(root, pub, trust.KeyID(pub), "documents", now.Add(-time.Hour), now.Add(time.Hour), 1, nil)
	if e != nil {
		t.Fatal(e)
	}
	keys := &Keys{RootPublicKey: rootPub, PrivateKey: private, Certificate: cert, KeyID: cert.KeyID}
	signed, e := keys.Sign([]byte("document"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = signed.Verify(rootPub, "documents", now); e != nil {
		t.Fatal(e)
	}
	if e = keys.ValidateSigner(cert.NotAfter); e == nil {
		t.Fatal("expired process signer")
	}
	cert, e = trust.Certify(root, pub, cert.KeyID, "documents", now.Add(-time.Hour), now.Add(-time.Minute), 2, nil)
	if e != nil {
		t.Fatal(e)
	}
	keys.Certificate = cert
	if _, e = keys.Sign([]byte("document")); e == nil {
		t.Fatal("issued after expiry")
	}
	cert, e = trust.Certify(root, pub, cert.KeyID, "documents", now.Add(-time.Hour), now.Add(time.Hour), 3, []string{cert.KeyID})
	if e != nil {
		t.Fatal(e)
	}
	keys.Certificate = cert
	if _, e = keys.Sign([]byte("document")); e == nil {
		t.Fatal("issued with revoked signer")
	}
	keys.Certificate = nil
	if _, e = keys.Sign(nil); e == nil {
		t.Fatal("unsigned certificate")
	}
	keys = nil
	if _, e = keys.Sign(nil); e == nil {
		t.Fatal("nil signer")
	}
}

func TestSigningRevisionFitsTheLocalServersInteger(t *testing.T) {
	rootPub, root, _ := ed25519.GenerateKey(rand.Reader)
	pub, private, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC()
	for _, revision := range []uint64{MaxSigningRevision, MaxSigningRevision + 1} {
		cert, err := trust.Certify(root, pub, trust.KeyID(pub), "documents", now.Add(-time.Minute), now.Add(time.Hour), revision, nil)
		if err != nil {
			t.Fatal(err)
		}
		keys := &Keys{RootPublicKey: rootPub, PrivateKey: private, Certificate: cert, KeyID: cert.KeyID}
		err = keys.ValidateSigner(now)
		if (err == nil) != (revision == MaxSigningRevision) {
			t.Fatal("revision outside the shared persistence range", revision, err)
		}
	}
	if _, err := writeSigningSet(t.TempDir(), root, nil, 365, MaxSigningRevision+1, nil); err == nil {
		t.Fatal("generated a certificate user servers cannot persist")
	}
}
