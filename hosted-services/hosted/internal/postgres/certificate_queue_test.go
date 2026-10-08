package postgres_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/certificates"
	"portico.local/hosted/internal/postgres"
)

func TestCertificateIdleQueueDoesNotProbeTerminalHistory(t *testing.T) {
	store := database(t)
	if _, err := store.DB.Exec(`INSERT INTO certificate_namespaces(operation_id,namespace) VALUES('history','ptc-aaaaaaaaaaaaaaaaaaaa');
 INSERT INTO certificate_orders(id,request_id,operation_id,server_id,account_id,claim_generation,credential_generation,token_hash,public_key,namespace,issuer,csr,csr_hash,state,revocation_done)
 SELECT 'cert_'||g,'req_'||g,'op_'||g,'server','account',1,1,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),'ptc-aaaaaaaaaaaaaaaaaaaa','fixture',decode(repeat('00',100),'hex'),decode(repeat('00',32),'hex'),'revoked',true FROM generate_series(1,50000)g;
 ANALYZE certificate_orders; ANALYZE certificate_challenges;`); err != nil {
		t.Fatal(err)
	}
	for _, cleanup := range []bool{false, true} {
		assertBoundedCertificatePlan(t, store.DB, postgres.CertificateWorkSQLForTest, 256, "fixture", cleanup)
	}
	assertBoundedCertificatePlan(t, store.DB, postgres.CertificateAuthoritySQLForTest, 128, "fixture")
	if _, err := store.LeaseCertificate(context.Background(), "fixture", "idle", false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("terminal history entered queue", err)
	}
	// Outstanding cleanup with a future retry must not become an idle scan of
	// 50k filtered index entries either. Effective due time is the indexed key.
	if _, err := store.DB.Exec(`UPDATE certificate_orders SET cleanup_due_at=now()-interval '1 day',next_attempt_at=now()+interval '1 day'; ANALYZE certificate_orders`); err != nil {
		t.Fatal(err)
	}
	assertBoundedCertificatePlan(t, store.DB, postgres.CertificateWorkSQLForTest, 256, "fixture", false)
}
func assertBoundedCertificatePlan(t *testing.T, db *sql.DB, query string, maximum float64, args ...any) {
	t.Helper()
	var raw []byte
	if err := db.QueryRow("EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+query, args...).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var nodes []struct{ Plan map[string]any }
	if err := json.Unmarshal(raw, &nodes); err != nil || len(nodes) != 1 {
		t.Fatal("bad explain", err)
	}
	examined := float64(0)
	var walk func(map[string]any)
	walk = func(node map[string]any) {
		loops, _ := node["Actual Loops"].(float64)
		for _, key := range []string{"Actual Rows", "Rows Removed by Filter", "Rows Removed by Join Filter", "Rows Removed by Index Recheck"} {
			n, _ := node[key].(float64)
			examined += n * loops
		}
		children, _ := node["Plans"].([]any)
		for _, child := range children {
			walk(child.(map[string]any))
		}
	}
	walk(nodes[0].Plan)
	if examined > maximum {
		t.Fatalf("idle selection examined %.0f rows (bound %.0f): %s", examined, maximum, raw)
	}
	t.Logf("examined=%.0f; plan=%s", examined, raw)
}
func seedClaimCertificates(t *testing.T, f fixture, count int, state string) {
	t.Helper()
	if _, err := f.store.DB.Exec(`INSERT INTO certificate_namespaces(operation_id,namespace) VALUES('fixture_installed_claim','ptc-aaaaaaaaaaaaaaaaaaaa') ON CONFLICT DO NOTHING;
 `); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB.Exec(`INSERT INTO certificate_orders(id,request_id,operation_id,server_id,account_id,claim_generation,credential_generation,token_hash,public_key,namespace,issuer,csr,csr_hash,state,provider_url,creation_attempted)
 SELECT 'owned_'||g,'req_'||g,o.id,s.id,s.owner_account_id,s.claim_generation,s.credential_generation,s.token_hash,o.public_key,'ptc-aaaaaaaaaaaaaaaaaaaa','fixture',decode(repeat('00',100),'hex'),decode(repeat('00',32),'hex'),$2,'https://ca.example/order/'||g,true
 FROM servers s JOIN claim_operations o ON o.server_id=s.id CROSS JOIN generate_series(1,$1::integer)g WHERE s.id=$3`, count, state, f.server); err != nil {
		t.Fatal(err)
	}
}
func TestCertificateAuthorityReconciliationCommitsEvenWhenIdle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	seedClaimCertificates(t, f, 64, "issued")
	for _, want := range []int{32, 64, 64} {
		if _, err := f.store.LeaseCertificate(ctx, "fixture", "idle", false); !errors.Is(err, sql.ErrNoRows) {
			t.Fatal(err)
		}
		var visited int
		if err := f.store.DB.QueryRow(`SELECT count(*) FROM certificate_orders WHERE authority_check_at>now()+interval '4 minutes'`).Scan(&visited); err != nil || visited != want {
			t.Fatal("idle reconciliation did not retain bounded progress", visited, want, err)
		}
	}
	// Revoked authority produces deny-only work, at most one batch per call.
	if _, err := f.store.DB.Exec(`UPDATE servers SET credential_generation=2 WHERE id=$1;`, f.server); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB.Exec(`UPDATE certificate_orders SET authority_check_at=now()`); err != nil {
		t.Fatal(err)
	}
	order, err := f.store.LeaseCertificate(ctx, "fixture", "cleanup", false)
	if err != nil || !order.CancelRequested || order.State != "cancelled" || order.ProviderURL == "" || !order.CreateAttempted {
		t.Fatal("lost custody during reconciliation", order, err)
	}
	var cancelled int
	if err = f.store.DB.QueryRow(`SELECT count(*) FROM certificate_orders WHERE cancel_requested`).Scan(&cancelled); err != nil || cancelled != 32 {
		t.Fatal("unbounded revocation reconciliation", cancelled, err)
	}
	if err = f.store.CheckCertificateLease(ctx, order, true); err == nil {
		t.Fatal("revoked claim authorized positive work")
	}
	if err = f.store.CheckCertificateLease(ctx, order, false); err != nil {
		t.Fatal("deny-only cleanup lost lease", err)
	}
}
func TestCertificateCleanupDueIsReservedAndRetiredTransactionally(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	seedClaimCertificates(t, f, 1, "queued")
	order, err := f.store.LeaseCertificate(ctx, "fixture", "working", false)
	if err != nil {
		t.Fatal(err)
	}
	challenge := certificates.Challenge{AuthorizationURL: "https://ca.example/auth/1", ChallengeURL: "https://ca.example/challenge/1", Namespace: order.Namespace, Value: strings.Repeat("a", 43)}
	if err = f.store.OwnCertificateChallenge(ctx, order, challenge); err != nil {
		t.Fatal(err)
	}
	var due sql.NullTime
	if err = f.store.DB.QueryRow(`SELECT cleanup_due_at FROM certificate_orders WHERE id=$1`, order.ID).Scan(&due); err != nil || !due.Valid || time.Until(due.Time) < 5*time.Minute {
		t.Fatal("TXT custody has no cleanup schedule", due, err)
	}
	if err = f.store.FinishCertificateRevocation(ctx, order); err != nil {
		t.Fatal(err)
	}
	if err = f.store.ReleaseCertificate(ctx, order, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.LeaseCertificate(ctx, "fixture", "early", false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("cleanup ran before replay quarantine expired", err)
	}
	// Advance only the synthetic clock evidence; do not sleep through quarantine.
	if _, err = f.store.DB.Exec(`UPDATE certificate_challenges SET cleanup_after=now()-interval '1 second'; UPDATE certificate_orders SET cleanup_due_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	order, err = f.store.LeaseCertificate(ctx, "fixture", "due", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.FinishCertificateCleanup(ctx, order, challenge); err != nil {
		t.Fatal(err)
	}
	if err = f.store.ReleaseCertificate(ctx, order, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	if err = f.store.DB.QueryRow(`SELECT cleanup_due_at FROM certificate_orders WHERE id=$1`, order.ID).Scan(&due); err != nil || due.Valid {
		t.Fatal("completed TXT retained queue eligibility", due, err)
	}
	if _, err = f.store.LeaseCertificate(ctx, "fixture", "finished", false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("completed cleanup re-entered queue", err)
	}
}
func TestAmbiguousCertificateRemainsQuarantined(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	seedClaimCertificates(t, f, 1, "reconciliation_required")
	if _, err := f.store.DB.Exec(`UPDATE certificate_orders SET provider_url=''`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.LeaseCertificate(ctx, "fixture", "normal", false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("ambiguous creation retried as new work", err)
	}
	order, err := f.store.LeaseCertificate(ctx, "fixture", "custody", true)
	if err != nil || !order.CreateAttempted || order.ProviderURL != "" || order.State != "reconciliation_required" {
		t.Fatal("ambiguous custody lost", order, err)
	}
}
