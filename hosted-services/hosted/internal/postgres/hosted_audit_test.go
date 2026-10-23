package postgres_test

import (
	"context"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"portico.local/hosted/internal/claims"
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
	// Cancellation removes the discovery index and leaves no affected server.
	p, e := f.store.PrepareDeletion(context.Background(), "strict-owner", passwordHash, security.Hash("preview"), time.Now().Add(time.Minute))
	if e != nil || !p.CanDelete || len(p.OwnedServers) != 0 || len(p.AffectedServers) != 0 {
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
