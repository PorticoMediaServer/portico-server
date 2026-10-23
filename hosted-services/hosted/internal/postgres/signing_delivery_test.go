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
	oversized, err := trust.Certify(root, second.PrivateKey.Public().(ed25519.PublicKey), second.KeyID, "documents", time.Now().Add(-time.Minute), time.Now().Add(time.Hour), security.MaxSigningRevision+1, []string{first.KeyID})
	if err != nil {
		t.Fatal(err)
	}
	oversizedRaw, _ := json.Marshal(oversized)
	for _, artifact := range [][]byte{raw, oversizedRaw, []byte(`{"keyId":"untrusted"}`), []byte(`invalid`)} {
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
			if _, e := f.store.DB.Exec(`DELETE FROM server_members WHERE account_id=$1`, f.member.Account.ID); e != nil {
				t.Fatal(e)
			}
			ctx := context.Background()
			_, root, _ := ed25519.GenerateKey(rand.Reader)
			first := rolloutKey(t, root, 1, nil)
			old := directory.New(f.store, first)
			consent, err := old.IssueIdentity(f.member.Account.ID, "member", "Member", "", f.server, testChallenge, kind)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(consent)
			if err = f.store.RecordSigningCertificate(ctx, first); err != nil {
				t.Fatal(err)
			}
			authority, err := old.CaptureServerAuthority(ctx, f.token)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "custody" {
				// Existing discovery standing needs no new consent for a role
				// change, but moving custody still needs an unrevoked signer.
				out, err := old.PushMembers(ctx, authority, directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: f.owner.Account.ID, Role: "owner", Active: true}, {AccountID: f.member.Account.ID, Role: "member", Active: true, Consent: raw}}})
				if err != nil || len(out.Skipped) != 0 {
					t.Fatal(out, err)
				}
			}
			second := rolloutKey(t, root, 2, []string{first.KeyID})
			if err = f.store.RecordSigningCertificate(ctx, second); err != nil {
				t.Fatal(err)
			}
			role := "member"
			if kind == "custody" {
				role = "owner"
			}
			push := directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: f.owner.Account.ID, Role: "owner", Active: true}, {AccountID: f.member.Account.ID, Role: role, Active: true, Consent: raw}}}
			if kind == "custody" {
				push.Through = 2
				push.Members[0].Role = "member"
			}
			out, err := old.PushMembers(ctx, authority, push)
			if err != nil || !out.Applied || (kind == "" && (len(out.Skipped) != 1 || f.listed(t, f.member.Account.ID)[f.server])) {
				t.Fatal("revoked consent used by older replica", out, err)
			}
			if (kind == "" && out.Custodian != f.owner.Account.ID) || (kind == "custody" && out.Custodian != "") {
				t.Fatal("revoked custody accepted", out.Custodian)
			}
			current := directory.New(f.store, second)
			consent, err = current.IssueIdentity(f.member.Account.ID, "member", "Member", "", f.server, testChallenge, kind)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ = json.Marshal(consent)
			push.Through++
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

func TestHostedRejectsThePreviousSigningDeliverySchema(t *testing.T) {
	f := setup(t)
	if _, err := f.store.DB.Exec(`UPDATE hosted_schema_contract SET version=20 WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	before := catalogue(t, f.store)
	if err := f.store.Migrate(context.Background()); err == nil {
		t.Fatal("adopted previous pre-release format")
	}
	if after := catalogue(t, f.store); after != before {
		t.Fatal("modified previous format during refusal")
	}
}

// A consent verified by an old process before it waits for a rollout must
// still meet the terminal revocation set that commits ahead of its mutation.
func TestMemberConsentRechecksRevocationAfterWaitingForRollout(t *testing.T) {
	f := setup(t)
	if _, e := f.store.DB.Exec(`DELETE FROM server_members WHERE account_id=$1`, f.member.Account.ID); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, root, _ := ed25519.GenerateKey(rand.Reader)
	first := rolloutKey(t, root, 1, nil)
	old := directory.New(f.store, first)
	if err := f.store.RecordSigningCertificate(ctx, first); err != nil {
		t.Fatal(err)
	}
	consent, err := old.IssueIdentity(f.member.Account.ID, "member", "Member", "", f.server, testChallenge, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(consent)
	authority, err := old.CaptureServerAuthority(ctx, f.token)
	if err != nil {
		t.Fatal(err)
	}
	rollout, err := f.store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rollout.Rollback()
	var revision string
	if err = rollout.QueryRowContext(ctx, `SELECT revision::text FROM hosted_signing_state WHERE singleton FOR UPDATE`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	second := rolloutKey(t, root, 2, []string{first.KeyID})
	cert, _ := json.Marshal(second.Certificate)
	if _, err = rollout.ExecContext(ctx, `UPDATE hosted_signing_state SET revision=2,certificate_hash=$1,certificate=$2 WHERE singleton`, security.Hash(string(cert)), cert); err != nil {
		t.Fatal(err)
	}
	if _, err = rollout.ExecContext(ctx, `INSERT INTO hosted_signing_revocations(key_id) VALUES($1)`, first.KeyID); err != nil {
		t.Fatal(err)
	}
	type answer struct {
		result directory.MemberPushResult
		err    error
	}
	done := make(chan answer, 1)
	go func() {
		out, err := old.PushMembers(ctx, authority, directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: f.member.Account.ID, Role: "member", Active: true, Consent: raw}}})
		done <- answer{out, err}
	}()
	// Observe the actual PostgreSQL lock wait, rather than guessing a delay.
	waiting := false
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if err = f.store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT revision::text FROM hosted_signing_state%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("member consent did not serialize with signing rollout")
	}
	if err = rollout.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case out := <-done:
		if out.err != nil || !out.result.Applied || len(out.result.Skipped) != 1 || f.listed(t, f.member.Account.ID)[f.server] {
			t.Fatal("consent survived an earlier committed revocation", out)
		}
	case <-ctx.Done():
		t.Fatal("consent mutation did not finish after rollout", ctx.Err())
	}
}

func TestRevocationFilteringDoesNotSelectAnUnlockedCustodian(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, root, _ := ed25519.GenerateKey(rand.Reader)
	first := rolloutKey(t, root, 1, nil)
	second := rolloutKey(t, root, 2, []string{first.KeyID})
	old := directory.New(f.store, first)
	current := directory.New(f.store, second)
	third, err := f.ids.Register(ctx, "otherowner", "Other owner", "StrongPass9")
	if err != nil {
		t.Fatal(err)
	}
	consentA, err := old.IssueIdentity(f.member.Account.ID, "member", "Member", "", f.server, testChallenge, "custody")
	if err != nil {
		t.Fatal(err)
	}
	consentB, err := current.IssueIdentity(third.Account.ID, "otherowner", "Other owner", "", f.server, testChallenge, "custody")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(consentA)
	b, _ := json.Marshal(consentB)
	if err = f.store.RecordSigningCertificate(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err = f.store.RecordSigningCertificate(ctx, second); err != nil {
		t.Fatal(err)
	}
	authority, err := old.CaptureServerAuthority(ctx, f.token)
	if err != nil {
		t.Fatal(err)
	}
	out, err := old.PushMembers(ctx, authority, directory.MemberPush{Through: 1, Full: true, Members: []directory.MemberStanding{{AccountID: f.owner.Account.ID, Role: "member", Active: true}, {AccountID: f.member.Account.ID, Role: "owner", Active: true, Consent: a}, {AccountID: third.Account.ID, Role: "owner", Active: true, Consent: b}}})
	if err != nil || !out.Applied || out.Custodian != "" {
		t.Fatal("ambiguous custody changed after account locks", out, err)
	}
	var holder string
	if err = f.store.DB.QueryRow(`SELECT owner_account_id FROM servers WHERE id=$1`, f.server).Scan(&holder); err != nil || holder != f.owner.Account.ID {
		t.Fatal("unlocked custody candidate was selected", holder, err)
	}
}
