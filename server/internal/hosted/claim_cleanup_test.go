package hosted

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/networking"
)

type cleanupExchange func(context.Context, networking.Intent, networking.SignedProof) (networking.PolicyEnvelope, error)

func (f cleanupExchange) ClaimCleanup(c context.Context, v networking.Intent, p networking.SignedProof) (networking.PolicyEnvelope, error) {
	return f(c, v, p)
}
func cleanupFixture(t *testing.T) *claimFixture {
	t.Helper()
	f := newClaimFixture(t)
	if e := f.service.ConfigureClaimCleanup(f.keys); e != nil {
		t.Fatal(e)
	}
	return f
}
func cleanupStep(t *testing.T, f *claimFixture) error {
	t.Helper()
	return f.runner.Do(context.Background(), func(ctx context.Context) error { _, e := f.service.cleanupControlStep(ctx); return e })
}
func cleanupSigned(f *claimFixture, w networking.CleanupWire, retired bool, jobs []cleanupJob) networking.PolicyEnvelope {
	now := time.Now().UTC()
	expires, _ := time.Parse(time.RFC3339Nano, w.ExpiresAt)
	for i := range jobs {
		jobs[i].Kind = "portico.account-erasure.v1"
		jobs[i].IssuedAt = now
		jobs[i].ExpiresAt = expires
	}
	r := cleanupResult{Kind: "portico.claim.cleanup.result", RequestID: w.RequestID, OperationID: w.OperationID, ServerID: w.ServerID, ClaimGeneration: w.ClaimGeneration, CredentialGeneration: w.CredentialGeneration, Retired: retired, Jobs: jobs, IssuedAt: now, ExpiresAt: expires}
	raw, _ := json.Marshal(r)
	return networking.PolicyEnvelope{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(f.hostedKey, raw)), KeyID: "pin1", Certificate: testSigningCertificate(f.hostedKey, "pin1")}
}
func cleanupRequest(t *testing.T, f *claimFixture, v networking.Intent, p networking.SignedProof) networking.CleanupWire {
	t.Helper()
	var w networking.CleanupWire
	if json.Unmarshal(p.Payload, &w) != nil || !ed25519.Verify(f.installed.PublicKey, p.Payload, p.Signature) || w.OperationID != f.installed.OperationID || w.ClaimGeneration != f.installed.ClaimGeneration || w.CredentialGeneration != f.installed.CredentialGeneration {
		t.Fatal("not original-key proof")
	}
	return w
}
func cleanupTestJob(f *claimFixture) cleanupJob {
	return cleanupJob{ID: "erasure_one", DeletionID: "deleted_account", ServerID: f.installed.ServerID, AccountID: "account", ProfileIDs: []string{"viewer"}, Revision: 1, Disposition: "delete_owned_personal_resources"}
}
func seedCleanupResources(t *testing.T, f *claimFixture) {
	t.Helper()
	for _, authority := range []string{"local", "hosted"} {
		for _, profile := range []string{"", "viewer"} {
			if _, e := f.db.Exec(`INSERT INTO dvr_recording_grants VALUES(?,?,?,1,1,1)`, authority, "account", profile); e != nil {
				t.Fatal(e)
			}
		}
	}
	for _, authority := range []string{"hosted", "local"} {
		key := identity.PersonalKey(identity.Viewer{Authority: authority, AccountID: "account", ProfileID: "viewer"})
		if _, e := f.db.Exec(`INSERT INTO saved_resources(id,kind,owner_key,owner_authority,owner_account,owner_profile,name,created_at) VALUES(?,'collection',?,?,'account','viewer','Private collection','now')`, authority, key, authority); e != nil {
			t.Fatal(e)
		}
		if _, e := f.db.Exec(`INSERT INTO download_preparations(id,profile_key,authority,account_id,profile_id,item_id,library_id,quality,origin,state,created_ms,updated_ms) VALUES(?,?,?,'account','viewer',1,'library','original','item','ready',1,1)`, authority, key, authority); e != nil {
			t.Fatal(e)
		}
	}
}
func countCleanup(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if e := db.QueryRow(query, args...).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

func TestClaimCleanupErasesBeforeRetireAndAcknowledgesAfterRestart(t *testing.T) {
	f := cleanupFixture(t)
	seedCleanupResources(t, f)
	calls := 0
	exchange := cleanupExchange(func(ctx context.Context, v networking.Intent, p networking.SignedProof) (networking.PolicyEnvelope, error) {
		w := cleanupRequest(t, f, v, p)
		calls++
		if calls == 1 {
			if w.AcknowledgeJobID != "" {
				t.Fatal("premature ack")
			}
			return cleanupSigned(f, w, true, []cleanupJob{cleanupTestJob(f)}), nil
		}
		if w.AcknowledgeJobID != "erasure_one" {
			t.Fatal("lost durable acknowledgement")
		}
		if countCleanup(t, f.db, `SELECT count(*) FROM saved_resources WHERE owner_authority='hosted'`) != 0 {
			t.Fatal("ack before erasure")
		}
		return cleanupSigned(f, w, true, nil), nil
	})
	f.service.current.cleanup.transport = exchange
	if e := cleanupStep(t, f); e != nil {
		t.Fatal(e)
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM saved_resources WHERE owner_authority='hosted'`)+countCleanup(t, f.db, `SELECT count(*) FROM download_preparations WHERE authority='hosted'`) != 0 {
		t.Fatal("personal data remains")
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM saved_resources WHERE owner_authority='local'`)+countCleanup(t, f.db, `SELECT count(*) FROM download_preparations WHERE authority='local'`) != 2 {
		t.Fatal("local namesake erased")
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM dvr_recording_grants WHERE authority='hosted'`) != 0 || countCleanup(t, f.db, `SELECT count(*) FROM dvr_recording_grants WHERE authority='local'`) != 2 {
		t.Fatal("recording grant authority erasure failed")
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM accounts WHERE id=?`, f.owner.Viewer.AccountID) != 1 {
		t.Fatal("owner erased")
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM networking_claim_credentials`) != 0 {
		t.Fatal("ordinary credential retained")
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM hosted_claim_cleanup_receipts WHERE acknowledged=0`) != 1 {
		t.Fatal("receipt not pending")
	}
	keys, e := networking.OpenCurrentKeys(context.Background(), f.db, f.state, f.runner)
	if e != nil {
		t.Fatal(e)
	}
	defer keys.Close()
	if e = f.service.ConfigureClaimCleanup(keys); e != nil {
		t.Fatal(e)
	}
	f.service.current.cleanup.transport = exchange
	if e = cleanupStep(t, f); e != nil {
		t.Fatal(e)
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM hosted_claim_cleanup_schedule WHERE complete=1`) != 1 || countCleanup(t, f.db, `SELECT count(*) FROM hosted_claim_cleanup_receipts WHERE acknowledged=1`) != 1 {
		t.Fatal("cleanup not completed")
	}
	if e = cleanupStep(t, f); e != nil || calls != 2 {
		t.Fatalf("completed cleanup polled: %v calls %d", e, calls)
	}
}
func TestClaimCleanupFailureRollsBackRetirementAndNeverAcknowledges(t *testing.T) {
	f := cleanupFixture(t)
	seedCleanupResources(t, f)
	if _, e := f.db.Exec(`CREATE TRIGGER fail_cleanup BEFORE INSERT ON hosted_claim_cleanup_receipts BEGIN SELECT RAISE(ABORT,'receipt failure'); END`); e != nil {
		t.Fatal(e)
	}
	f.service.current.cleanup.transport = cleanupExchange(func(ctx context.Context, v networking.Intent, p networking.SignedProof) (networking.PolicyEnvelope, error) {
		w := cleanupRequest(t, f, v, p)
		if w.AcknowledgeJobID != "" {
			t.Fatal("failed erasure acknowledged")
		}
		return cleanupSigned(f, w, true, []cleanupJob{cleanupTestJob(f)}), nil
	})
	if e := cleanupStep(t, f); e == nil {
		t.Fatal("expected write failure")
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM saved_resources WHERE owner_authority='hosted'`) != 1 || countCleanup(t, f.db, `SELECT count(*) FROM networking_claim_credentials`) != 1 || countCleanup(t, f.db, `SELECT count(*) FROM hosted_claim_cleanup_receipts`) != 0 {
		t.Fatal("partial erasure committed")
	}
	f.db.Exec(`DROP TRIGGER fail_cleanup`)
	if e := cleanupStep(t, f); e != nil {
		t.Fatal(e)
	}
}
func TestClaimCleanupRejectsForgedStaleOrCrossClaimResults(t *testing.T) {
	for _, attack := range []string{"signature", "nonce", "generation", "operation", "expiry", "disposition", "profile"} {
		t.Run(attack, func(t *testing.T) {
			f := cleanupFixture(t)
			seedCleanupResources(t, f)
			f.service.current.cleanup.transport = cleanupExchange(func(ctx context.Context, v networking.Intent, p networking.SignedProof) (networking.PolicyEnvelope, error) {
				w := cleanupRequest(t, f, v, p)
				j := cleanupTestJob(f)
				switch attack {
				case "nonce":
					w.RequestID = "replayed"
				case "generation":
					w.ClaimGeneration = "2"
				case "operation":
					w.OperationID = "newclaim"
				case "expiry":
					w.ExpiresAt = time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
				case "disposition":
					j.Disposition = "delete_media"
				case "profile":
					j.ProfileIDs = []string{"../owner"}
				}
				envelope := cleanupSigned(f, w, true, []cleanupJob{j})
				if attack == "signature" {
					envelope.Signature = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
				}
				return envelope, nil
			})
			if e := cleanupStep(t, f); e == nil {
				t.Fatal("invalid result accepted")
			}
			if countCleanup(t, f.db, `SELECT count(*) FROM networking_claim_credentials`) != 1 || countCleanup(t, f.db, `SELECT count(*) FROM saved_resources WHERE owner_authority='hosted'`) != 1 {
				t.Fatal("invalid response changed authority/data")
			}
		})
	}
}
func TestClaimCleanupAcknowledgementOutageAndSuccessorIsolation(t *testing.T) {
	f := cleanupFixture(t)
	seedCleanupResources(t, f)
	calls := 0
	f.service.current.cleanup.transport = cleanupExchange(func(ctx context.Context, v networking.Intent, p networking.SignedProof) (networking.PolicyEnvelope, error) {
		w := cleanupRequest(t, f, v, p)
		calls++
		if calls == 1 {
			return cleanupSigned(f, w, true, []cleanupJob{cleanupTestJob(f)}), nil
		}
		if calls == 2 {
			return networking.PolicyEnvelope{}, errors.New("network offline after retirement")
		}
		return cleanupSigned(f, w, true, nil), nil
	})
	if e := cleanupStep(t, f); e != nil {
		t.Fatal(e)
	}
	var successor networking.Intent
	if e := f.runner.Do(context.Background(), func(ctx context.Context) error {
		var e error
		b := f.installed.Binding
		b.OperationID = "successor"
		b.AccountID = "new_owner"
		if e = f.db.QueryRow(`SELECT reset_generation FROM networking_claim_identity`).Scan(&b.LocalGeneration); e != nil {
			return e
		}
		successor, e = f.store.Prepare(ctx, b, networking.LocalOwner{AccountID: f.owner.Viewer.AccountID, ProfileID: f.owner.Viewer.ProfileID, Epoch: 1})
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if e := cleanupStep(t, f); e == nil {
		t.Fatal("expected outage")
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM hosted_claim_cleanup_receipts WHERE acknowledged=0`) != 1 {
		t.Fatal("outage lost obligation")
	}
	f.db.Exec(`UPDATE hosted_claim_cleanup_schedule SET next_at=0`)
	if e := cleanupStep(t, f); e != nil {
		t.Fatal(e)
	}
	var active string
	if e := f.db.QueryRow(`SELECT active_operation_id FROM networking_claim_identity`).Scan(&active); e != nil || active != successor.OperationID {
		t.Fatalf("successor cleared: %s %v", active, e)
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM networking_claim_intents WHERE operation_id='successor' AND stage='prepared'`) != 1 {
		t.Fatal("successor mutated")
	}
}
func TestClaimCleanupMemberErasureDoesNotRetireOwnerClaim(t *testing.T) {
	f := cleanupFixture(t)
	seedCleanupResources(t, f)
	f.service.current.cleanup.transport = cleanupExchange(func(ctx context.Context, v networking.Intent, p networking.SignedProof) (networking.PolicyEnvelope, error) {
		w := cleanupRequest(t, f, v, p)
		return cleanupSigned(f, w, false, []cleanupJob{cleanupTestJob(f)}), nil
	})
	if e := cleanupStep(t, f); e != nil {
		t.Fatal(e)
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM networking_claim_credentials`) != 1 || countCleanup(t, f.db, `SELECT count(*) FROM networking_claim_identity WHERE installed_operation_id='operation'`) != 1 {
		t.Fatal("member erasure retired claim")
	}
}

func TestClaimCleanupCancelledLeaseCannotEraseOrRetire(t *testing.T) {
	f := cleanupFixture(t)
	seedCleanupResources(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.service.current.cleanup.transport = cleanupExchange(func(ctx context.Context, v networking.Intent, p networking.SignedProof) (networking.PolicyEnvelope, error) {
		w := cleanupRequest(t, f, v, p)
		reply := cleanupSigned(f, w, true, []cleanupJob{cleanupTestJob(f)})
		cancel()
		return reply, nil
	})
	e := f.runner.Do(ctx, func(ctx context.Context) error { _, e := f.service.cleanupControlStep(ctx); return e })
	if e == nil {
		t.Fatal("cancelled lease committed")
	}
	if countCleanup(t, f.db, `SELECT count(*) FROM networking_claim_credentials`) != 1 || countCleanup(t, f.db, `SELECT count(*) FROM saved_resources WHERE owner_authority='hosted'`) != 1 {
		t.Fatal("cancelled work changed state")
	}
}

func TestClaimCleanupRejectsAckWithoutCommittedReceipt(t *testing.T) {
	f := cleanupFixture(t)
	e := f.runner.Do(context.Background(), func(ctx context.Context) error {
		_, _, e := f.keys.CleanupProof(ctx, f.installed, f.service.origin, "not_applied")
		return e
	})
	if e == nil {
		t.Fatal("unapplied erasure signed for acknowledgement")
	}
}
