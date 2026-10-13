package postgres_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/trust"
)

func TestCheckInDeliversLatestCertificateFromAnOlderReplica(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, root, _ := ed25519.GenerateKey(rand.Reader)
	first := rolloutKey(t, root, 1, nil)
	second := rolloutKey(t, root, 2, []string{first.KeyID})
	old := directory.New(f.store, first)
	for _, keys := range []*security.Keys{first, second} {
		if err := f.store.RecordSigningCertificate(ctx, keys); err != nil {
			t.Fatal(err)
		}
	}
	first.SignerRevoked.Store(true)
	authority, err := old.CaptureServerAuthority(ctx, f.token)
	if err != nil {
		t.Fatal(err)
	}
	out, err := old.ServerHeartbeat(ctx, authority, directory.CheckInReport{WakeRevision: 2})
	if err != nil || out.SigningCertificate == nil || out.SigningCertificate.Revision != 2 {
		t.Fatalf("old replica did not deliver current certificate: %+v %v", out, err)
	}
	if _, err = out.SigningCertificate.Verify(first.RootPublicKey, "documents", time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(out.SigningCertificate.Revoked) != 1 || out.SigningCertificate.Revoked[0] != first.KeyID {
		t.Fatal("lost revocation")
	}
	var ack int64
	if err = f.store.DB.QueryRow(`SELECT acked_revision FROM wake_state WHERE server_id=$1`, f.server).Scan(&ack); err != nil || ack != 2 {
		t.Fatal("wake acknowledgment lost", ack, err)
	}
	// Neither corrupt stored bytes nor a lapsed public certificate may make
	// presence recording dependent on a signer being available.
	expired, err := trust.Certify(root, second.PrivateKey.Public().(ed25519.PublicKey), second.KeyID, "documents", time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour), 2, []string{first.KeyID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(expired)
	for _, artifact := range [][]byte{raw, []byte(`{"keyId":"untrusted"}`), []byte(`invalid`)} {
		if _, err = f.store.DB.Exec(`UPDATE hosted_signing_state SET certificate=$1 WHERE singleton`, artifact); err != nil {
			t.Fatal(err)
		}
		out, err = old.ServerHeartbeat(ctx, authority, directory.CheckInReport{})
		if err != nil || out.SigningCertificate != nil || out.PresenceExpiresAt.IsZero() {
			t.Fatal("unusable artifact broke check-in", out, err)
		}
	}
}

func TestOlderReplicaRefusesRevokedConsentForDiscoveryAndCustody(t *testing.T) {
	for _, kind := range []string{"", "custody"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			ctx := context.Background()
			_, root, _ := ed25519.GenerateKey(rand.Reader)
			first := rolloutKey(t, root, 1, nil)
			old := directory.New(f.store, first)
			consent, err := old.IssueIdentity(f.member.Account.ID, "member", "Member", f.server, testChallenge, kind)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(consent)
			if err = f.store.RecordSigningCertificate(ctx, first); err != nil {
				t.Fatal(err)
			}
			second := rolloutKey(t, root, 2, []string{first.KeyID})
			if err = f.store.RecordSigningCertificate(ctx, second); err != nil {
				t.Fatal(err)
			}
			authority, err := old.CaptureServerAuthority(ctx, f.token)
			if err != nil {
				t.Fatal(err)
			}
			role := "member"
			if kind == "custody" {
				role = "owner"
			}
			push := directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: f.owner.Account.ID, Role: "owner", Active: true}, {AccountID: f.member.Account.ID, Role: role, Active: true, Consent: raw}}}
			out, err := old.PushMembers(ctx, authority, push)
			if err != nil || !out.Applied || len(out.Skipped) != 1 || f.listed(t, f.member.Account.ID)[f.server] {
				t.Fatal("revoked consent used by older replica", out, err)
			}
			if out.Custodian != f.owner.Account.ID {
				t.Fatal("revoked custody accepted", out.Custodian)
			}
			current := directory.New(f.store, second)
			consent, err = current.IssueIdentity(f.member.Account.ID, "member", "Member", f.server, testChallenge, kind)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ = json.Marshal(consent)
			push.Through = 2
			push.Members[1].Consent = raw
			out, err = current.PushMembers(ctx, authority, push)
			if err != nil || len(out.Skipped) != 0 || !f.listed(t, f.member.Account.ID)[f.server] {
				t.Fatal("current consent refused", out, err)
			}
			if kind == "custody" && out.Custodian != f.member.Account.ID {
				t.Fatal("current custody refused", out.Custodian)
			}
		})
	}
}
