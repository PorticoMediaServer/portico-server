package postgres_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/trust"
	"sync/atomic"
	"testing"
	"time"
)

func rolloutKey(t *testing.T, root ed25519.PrivateKey, revision uint64, revoked []string) *security.Keys {
	t.Helper()
	pub, private, _ := ed25519.GenerateKey(rand.Reader)
	cert, e := trust.Certify(root, pub, trust.KeyID(pub), "documents", time.Now().Add(-time.Hour), time.Now().Add(time.Hour), revision, revoked)
	if e != nil {
		t.Fatal(e)
	}
	return &security.Keys{RootPublicKey: root.Public().(ed25519.PublicKey), PrivateKey: private, KeyID: cert.KeyID, Certificate: cert, SignerRevoked: &atomic.Bool{}}
}
func TestSigningRolloutCheckpointsOnceAndRejectsRollback(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, root, _ := ed25519.GenerateKey(rand.Reader)
	first := rolloutKey(t, root, 1, nil)
	if e := f.store.RecordSigningCertificate(ctx, first); e != nil {
		t.Fatal(e)
	}
	// Synthetic active server rows exercise only the bounded cursor/queue, not
	// claim installation. No network worker runs in this fixture.
	if _, e := f.store.DB.Exec(`INSERT INTO servers(id,owner_account_id,name,public_key,token_hash,token_ciphertext,claim_generation,credential_generation) SELECT 'srv_'||rpad(n::text,43,'a'),$1,'Fixture',repeat('a',43),sha256(n::text::bytea),repeat('x',32)::bytea,1,1 FROM generate_series(1,260) n`, f.owner.Account.ID); e != nil {
		t.Fatal(e)
	}
	var initial int64
	if e := f.store.DB.QueryRow(`SELECT sum(policy_revision) FROM servers`).Scan(&initial); e != nil {
		t.Fatal(e)
	}
	more, e := f.store.SigningRolloutBatch(ctx, 1)
	if e != nil || !more {
		t.Fatal(more, e)
	}
	var queued int64
	if e = f.store.DB.QueryRow(`SELECT sum(policy_revision) FROM servers`).Scan(&queued); e != nil || queued-initial != 128 {
		t.Fatal("unbounded batch", queued, e)
	}
	for more {
		more, e = f.store.SigningRolloutBatch(ctx, 1)
		if e != nil {
			t.Fatal(e)
		}
	}
	if e = f.store.RecordSigningCertificate(ctx, first); e != nil {
		t.Fatal(e)
	}
	if more, e = f.store.SigningRolloutBatch(ctx, 1); e != nil || more {
		t.Fatal("same certificate requeued", more, e)
	}
	second := rolloutKey(t, root, 2, []string{first.KeyID})
	if e = f.store.RecordSigningCertificate(ctx, second); e != nil {
		t.Fatal(e)
	}
	if e = f.store.RecordSigningCertificate(ctx, first); !errors.Is(e, trust.ErrTrust) {
		t.Fatal("rollback", e)
	}
	bad := rolloutKey(t, root, 3, nil)
	if e = f.store.RecordSigningCertificate(ctx, bad); !errors.Is(e, trust.ErrTrust) {
		t.Fatal("forgot root revocation", e)
	}
	if _, e = f.store.SigningRolloutBatch(ctx, 1); !errors.Is(e, trust.ErrTrust) {
		t.Fatal("retired sweep wrote", e)
	}
	fenceCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	f.store.RunSignerRevocationFence(fenceCtx, first)
	if e = first.ValidateSigner(time.Now()); !errors.Is(e, trust.ErrTrust) {
		t.Fatal("old process retained signer", e)
	}
}
