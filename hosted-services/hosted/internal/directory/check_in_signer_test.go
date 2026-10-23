package directory

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/hosted/internal/security"
	"portico.local/hosted/trust"
)

type checkInRepo struct{ checkIns int }

func (*checkInRepo) List(context.Context, string) ([]Server, error) { return nil, nil }
func (*checkInRepo) AuthenticateServer(context.Context, []byte) (string, error) {
	return "", ErrDenied
}
func (*checkInRepo) CaptureServerAuthority(context.Context, []byte) (ServerAuthority, error) {
	return ServerAuthority{}, ErrDenied
}
func (r *checkInRepo) ServerHeartbeat(context.Context, ServerAuthority, CheckInReport) (Heartbeat, error) {
	r.checkIns++
	return Heartbeat{HeartbeatAfterMs: 60000}, nil
}
func (*checkInRepo) PushMembers(context.Context, ServerAuthority, MemberPush) (MemberPushResult, error) {
	return MemberPushResult{}, ErrDenied
}
func TestCheckInSurvivesAnUnusableSigner(t *testing.T) {
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
	repo := &checkInRepo{}
	s := New(repo, keys)
	out, e := s.ServerHeartbeat(context.Background(), ServerAuthority{}, CheckInReport{})
	if e != nil || out.SigningCertificate != valid || repo.checkIns != 1 {
		t.Fatal("valid signer", out.SigningCertificate, e)
	}
	keys.Certificate = expired
	out, e = s.ServerHeartbeat(context.Background(), ServerAuthority{}, CheckInReport{})
	if e != nil || out.SigningCertificate != nil || out.HeartbeatAfterMs != 60000 || repo.checkIns != 2 {
		t.Fatal("expired signer refused or advertised", out.SigningCertificate, e)
	}
	keys.Certificate = valid
	keys.SignerRevoked.Store(true)
	out, e = s.ServerHeartbeat(context.Background(), ServerAuthority{}, CheckInReport{})
	if e != nil || out.SigningCertificate != nil || repo.checkIns != 3 {
		t.Fatal("revoked signer refused or advertised", out.SigningCertificate, e)
	}
}
