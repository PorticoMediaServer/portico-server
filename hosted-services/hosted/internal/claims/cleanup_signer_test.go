package claims

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/trust"
)

type cleanupRepo struct {
	ConsumerRepository
	calls int
}

func (r *cleanupRepo) ClaimCleanup(context.Context, CleanupWire) (bool, []directory.ErasureJob, error) {
	r.calls++
	return true, nil, nil
}

// A clean-up answer retires a server and hands it erasure work. Like every
// other document Hosted signs, it is not issued by an expired or revoked
// signer, and such a process does not hand the work out either.
func TestCleanupIsNotSignedByAnUnusableSigner(t *testing.T) {
	rootPub, root, _ := ed25519.GenerateKey(rand.Reader)
	pub, private, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC()
	valid, e := trust.Certify(root, pub, trust.KeyID(pub), "documents", now.Add(-time.Hour), now.Add(time.Hour), 1, nil)
	if e != nil {
		t.Fatal(e)
	}
	expired, e := trust.Certify(root, pub, trust.KeyID(pub), "documents", now.Add(-2*time.Hour), now.Add(-time.Hour), 1, nil)
	if e != nil {
		t.Fatal(e)
	}
	keys := &security.Keys{SignerRevoked: &atomic.Bool{}, RootPublicKey: rootPub, PrivateKey: private, KeyID: valid.KeyID, Certificate: valid}
	repo := &cleanupRepo{}
	s := &Service{repo: repo, keys: keys, audience: "https://web.getportico.tv", now: time.Now}

	serverPub, serverKey, _ := ed25519.GenerateKey(rand.Reader)
	server, e := ServerID(serverPub)
	if e != nil {
		t.Fatal(e)
	}
	payload, _ := json.Marshal(CleanupWire{Kind: "portico.claim.cleanup", Version: "1", Audience: s.audience, OperationID: "operation", ServerID: server, PublicKey: base64.RawURLEncoding.EncodeToString(serverPub), ClaimGeneration: "1", CredentialGeneration: "1", RequestID: "request", IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(20 * time.Second).Format(time.RFC3339Nano)})
	signature := ed25519.Sign(serverKey, payload)

	out, e := s.Cleanup(context.Background(), payload, signature)
	if e != nil || out.Certificate != valid || repo.calls != 1 {
		t.Fatal("valid signer", e, repo.calls)
	}
	keys.Certificate = expired
	if _, e = s.Cleanup(context.Background(), payload, signature); !errors.Is(e, trust.ErrTrust) || repo.calls != 1 {
		t.Fatal("expired signer answered", e, repo.calls)
	}
	keys.Certificate = valid
	keys.SignerRevoked.Store(true)
	if _, e = s.Cleanup(context.Background(), payload, signature); !errors.Is(e, trust.ErrTrust) || repo.calls != 1 {
		t.Fatal("revoked signer answered", e, repo.calls)
	}
}
