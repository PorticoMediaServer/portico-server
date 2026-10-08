package postgres_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"testing"
	"time"
)

func TestCancelledClaimWithdrawsBothFamiliesAndDeletionPreview(t *testing.T) {
	f := currentClaimJourney(t)
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("StrongPass9"), bcrypt.DefaultCost)
	if _, e := f.store.DB.Exec(`UPDATE accounts SET password_hash=$1 WHERE id='strict-owner'`, passwordHash); e != nil {
		t.Fatal(e)
	}
	a := journeyApproval(t, f)
	raw, sig := journeyProof(t, f, a, claims.Finalize)
	var commit claims.Commit
	claimHTTP(t, f.handler, "/v1/server-claims/finalize", "", proofEnvelope(raw, sig), &commit, 200)
	ns := "ptc-aaaaaaaaaaaaaaaaaaaa"
	if _, e := f.store.DB.Exec(`INSERT INTO certificate_namespaces(operation_id,namespace) VALUES($1,$2);`, commit.OperationID, ns); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.DB.Exec(`INSERT INTO direct_addresses(namespace,server_id,ipv4,ipv6,sequence) VALUES($1,$2,'8.8.8.8','2606:4700:4700::1111',nextval('direct_address_sequence'))`, ns, commit.ServerID); e != nil {
		t.Fatal(e)
	}
	raw, sig = cancelAssertion(t, f, commit.ClaimGeneration)
	claimHTTP(t, f.handler, "/v1/server-claims/cancel", "", proofEnvelope(raw, sig), nil, 200)
	var ipv4, ipv6 string
	var sequence int64
	if e := f.store.DB.QueryRow(`SELECT ipv4,ipv6,sequence FROM direct_addresses WHERE namespace=$1`, ns).Scan(&ipv4, &ipv6, &sequence); e != nil || ipv4 != "" || ipv6 != "" || sequence <= 1 {
		t.Fatal(ipv4, ipv6, sequence, e)
	}
	claimHTTP(t, f.handler, "/v1/server-claims/cancel", "", proofEnvelope(raw, sig), nil, 200)
	var retry int64
	f.store.DB.QueryRow(`SELECT sequence FROM direct_addresses WHERE namespace=$1`, ns).Scan(&retry)
	if retry != sequence {
		t.Fatal("duplicate cancellation advanced feed")
	}
	// The actual cancellation leaves a revoked owner membership with a NULL owner.
	p, e := f.store.PrepareDeletion(context.Background(), "strict-owner", passwordHash, security.Hash("preview"), time.Now().Add(time.Minute))
	if e != nil || !p.CanDelete || len(p.OwnedServers) != 0 || len(p.AffectedServers) != 1 {
		t.Fatal(p, e)
	}
	// A lost cancellation response must not withdraw a newly installed claim.
	f.input.OperationID = "operation_b"
	a = journeyApproval(t, f)
	newRaw, newSig := journeyProof(t, f, a, claims.Finalize)
	var successor claims.Commit
	claimHTTP(t, f.handler, "/v1/server-claims/finalize", "", proofEnvelope(newRaw, newSig), &successor, 200)
	nextNS := "ptc-bbbbbbbbbbbbbbbbbbbb"
	if _, e = f.store.DB.Exec(`INSERT INTO certificate_namespaces(operation_id,namespace) VALUES($1,$2)`, successor.OperationID, nextNS); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`INSERT INTO direct_addresses(namespace,server_id,ipv4,sequence) VALUES($1,$2,'9.9.9.9',nextval('direct_address_sequence'))`, nextNS, successor.ServerID); e != nil {
		t.Fatal(e)
	}
	claimHTTP(t, f.handler, "/v1/server-claims/cancel", "", proofEnvelope(raw, sig), nil, 200)
	if e = f.store.DB.QueryRow(`SELECT ipv4 FROM direct_addresses WHERE namespace=$1`, nextNS).Scan(&ipv4); e != nil || ipv4 != "9.9.9.9" {
		t.Fatal("successor withdrawn", ipv4, e)
	}
}
func TestDeletionCompletesWithOwnedCancelledAndMissingClaim(t *testing.T) {
	for _, mode := range []string{"owned", "cancelled", "missing-install", "missing-operation"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			ctx := context.Background()
			account := f.member.Account
			switch mode {
			case "owned":
				account = f.owner.Account
			case "cancelled":
				if _, e := f.store.DB.Exec(`UPDATE claim_operations SET state='cancelled',cancelled_at=clock_timestamp(); UPDATE servers SET owner_account_id=NULL,token_hash=NULL,token_ciphertext=NULL,revoked_at=clock_timestamp(),base_url=NULL,endpoint_verified=false; UPDATE memberships SET revoked=true,role='member'`); e != nil {
					t.Fatal(e)
				}
			case "missing-install":
				if _, e := f.store.DB.Exec(`UPDATE claim_operations SET state='committed_pending_install',installed_at=NULL`); e != nil {
					t.Fatal(e)
				}
			case "missing-operation":
				if _, e := f.store.DB.Exec(`DELETE FROM claim_operations`); e != nil {
					t.Fatal(e)
				}
			}
			p, e := f.ids.PrepareDeletion(ctx, account, "StrongPass9")
			if e != nil || !p.CanDelete {
				t.Fatal(p, e)
			}
			r, e := f.ids.AcceptDeletion(ctx, p.Token, identity.DeletePersonalResources)
			if e != nil || !r.AccountDeleted || len(r.PendingServers) != 1 {
				t.Fatal(r, e)
			}
			r2, e := f.ids.AcceptDeletion(ctx, p.Token, identity.DeletePersonalResources)
			if e != nil || r2.ID != r.ID {
				t.Fatal(r2, e)
			}
			var n int
			f.store.DB.QueryRow(`SELECT count(*) FROM accounts WHERE id=$1`, account.ID).Scan(&n)
			if n != 0 {
				t.Fatal("account retained")
			}
			if _, e = f.store.CaptureServerAuthority(ctx, security.Hash(f.token)); e == nil {
				t.Fatal("invalid bearer retained authority")
			}
			if mode != "owned" {
				var owner bool
				f.store.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)`, f.owner.Account.ID).Scan(&owner)
				if !owner {
					t.Fatal("member deletion removed owner")
				}
			}
		})
	}
}
func TestOriginalKeyCleanupAfterOwnerDeletion(t *testing.T) {
	f := currentClaimJourney(t)
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("StrongPass9"), bcrypt.DefaultCost)
	if _, e := f.store.DB.Exec(`UPDATE accounts SET password_hash=$1 WHERE id='strict-owner'`, passwordHash); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	a := journeyApproval(t, f)
	raw, sig := journeyProof(t, f, a, claims.Finalize)
	var commit claims.Commit
	claimHTTP(t, f.handler, "/v1/server-claims/finalize", "", proofEnvelope(raw, sig), &commit, 200)
	token := security.Hash("deletion-fixture")
	var password []byte
	f.store.DB.QueryRow(`SELECT password_hash FROM accounts WHERE id='strict-owner'`).Scan(&password)
	if _, e := f.store.PrepareDeletion(ctx, "strict-owner", password, token, time.Now().Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.AcceptDeletion(ctx, token, identity.DeletePersonalResources); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	w := claims.CleanupWire{Kind: "portico.claim.cleanup", Version: "1", Audience: "https://hosted.example", OperationID: commit.OperationID, ServerID: commit.ServerID, PublicKey: f.input.PublicKey, ClaimGeneration: commit.ClaimGeneration, CredentialGeneration: commit.CredentialGeneration, RequestID: "cleanup_request", IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(25 * time.Second).Format(time.RFC3339Nano)}
	fetch := func(w claims.CleanupWire) claims.CleanupResult {
		t.Helper()
		raw, _ := json.Marshal(w)
		var envelope claims.Envelope
		claimHTTP(t, f.handler, "/v1/server-claims/cleanup", "", proofEnvelope(raw, ed25519.Sign(f.key, raw)), &envelope, 200)
		payload, e := base64.RawURLEncoding.DecodeString(envelope.Payload)
		if e != nil {
			t.Fatal(e)
		}
		var result claims.CleanupResult
		if e = json.Unmarshal(payload, &result); e != nil {
			t.Fatal(e)
		}
		return result
	}
	result := fetch(w)
	if !result.Retired || len(result.Jobs) != 1 || result.Jobs[0].AccountID != "strict-owner" {
		t.Fatal(result)
	}
	bad := w
	bad.ClaimGeneration = "999"
	raw, _ = json.Marshal(bad)
	claimHTTP(t, f.handler, "/v1/server-claims/cleanup", "", proofEnvelope(raw, ed25519.Sign(f.key, raw)), nil, 409)
	w.AcknowledgeJobID = result.Jobs[0].ID
	w.AcknowledgeRevision = 1
	if result = fetch(w); len(result.Jobs) != 0 {
		t.Fatal(result)
	}
	if result = fetch(w); len(result.Jobs) != 0 {
		t.Fatal("ack retry", result)
	}
	var targets string
	f.store.DB.QueryRow(`SELECT account_id FROM account_erasure_jobs WHERE id=$1`, w.AcknowledgeJobID).Scan(&targets)
	if targets != "" {
		t.Fatal("acknowledged target retained")
	}
}

func TestMailDiscardProgress(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	repo := &postgres.AccountMail{Store: s.Store, Keys: s.Keys}
	for _, mode := range []string{"expired", "attempts", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			id := security.RandomID("mail_")
			tx, e := s.Store.DB.BeginTx(ctx, nil)
			if e != nil {
				t.Fatal(e)
			}
			e = postgres.EnqueueAccountMailTx(ctx, tx, s.Keys, mailoutbox.Intent{ID: id, Kind: mailoutbox.Registration, ResourceID: "registration", ExpiresAt: time.Now().Add(time.Hour), Payload: mailoutbox.Payload{To: "test@example.test", Code: "123456"}})
			if e != nil {
				t.Fatal(e)
			}
			if e = tx.Commit(); e != nil {
				t.Fatal(e)
			}
			q := `UPDATE account_mail_outbox SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`
			if mode == "attempts" {
				q = `UPDATE account_mail_outbox SET attempts=8 WHERE id=$1`
			}
			if mode == "corrupt" {
				q = `UPDATE account_mail_outbox SET ciphertext='broken' WHERE id=$1`
			}
			if _, e = s.Store.DB.Exec(q, id); e != nil {
				t.Fatal(e)
			}
			d, e := repo.ClaimMail(ctx)
			if d != nil || !errors.Is(e, mailoutbox.ErrQueueProgress) {
				t.Fatal(d, e)
			}
		})
	}
	d, e := repo.ClaimMail(ctx)
	if d != nil || e != nil {
		t.Fatal("queue not idle", d, e)
	}
}
