package postgres_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"testing"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

func TestI01FreshAndRepeatedSchemaIncludesBothGroups(t *testing.T) {
	store := database(t) // fresh current-contract schema, including both additions
	for pass := 0; pass < 2; pass++ {
		if e := store.Migrate(context.Background()); e != nil {
			t.Fatal(e)
		}
		for _, name := range []string{"certificate_namespaces", "certificate_orders", "certificate_challenges", "account_mail_outbox", "account_mfa_challenges", "identity_completion_receipts"} {
			var exists bool
			if e := store.DB.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); e != nil || !exists {
				t.Fatalf("missing composed schema %s: %v", name, e)
			}
		}
		var version int
		if e := store.DB.QueryRow(`SELECT version FROM account_security_schema WHERE singleton`).Scan(&version); e != nil || version != 1 {
			t.Fatal(version, e)
		}
		// The deletion worker relies on P03's exact installed-claim tuple columns.
		if _, e := store.DB.Exec(`SELECT notice_digest FROM server_leave_receipts LIMIT 0`); e != nil {
			t.Fatal(e)
		}
	}
}

func TestI01FailedOrderWithoutTXTStillRetainsIssuerAndCleanupLease(t *testing.T) {
	store := database(t)
	ctx := context.Background()
	if _, e := store.DB.Exec(`INSERT INTO certificate_namespaces(operation_id,namespace) VALUES('operation','ptc-aaaaaaaaaaaaaaaaaaaa')`); e != nil {
		t.Fatal(e)
	}
	// Legacy failed receipt, no cancel bit and no TXT (CA reused authorization).
	if _, e := store.DB.Exec(`INSERT INTO certificate_orders(id,request_id,operation_id,server_id,account_id,claim_generation,credential_generation,token_hash,public_key,namespace,issuer,csr,csr_hash,state) VALUES('failed-order','request','operation','server','deleted-account',1,1,$1,$1,'ptc-aaaaaaaaaaaaaaaaaaaa','fixture-issuer',$2,$1,'failed')`, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 100)); e != nil {
		t.Fatal(e)
	}
	issuers, e := store.OutstandingCertificateIssuers(ctx)
	if e != nil || len(issuers) != 1 || issuers[0] != "fixture-issuer" {
		t.Fatal(issuers, e)
	}
	o, e := store.LeaseCertificate(ctx, "fixture-issuer", "cleanup-lease", true)
	if e != nil || o.ID != "failed-order" {
		t.Fatal(o, e)
	}
	if e = store.SetCertificateState(ctx, o, "failed", "provider_response_invalid", time.Now()); e != nil {
		t.Fatal(e)
	}
	var cancel bool
	if e = store.DB.QueryRow(`SELECT cancel_requested FROM certificate_orders WHERE id=$1`, o.ID).Scan(&cancel); e != nil || !cancel {
		t.Fatal("failed receipt did not require revocation", e)
	}
	if e = store.FinishCertificateRevocation(ctx, o); e != nil {
		t.Fatal(e)
	}
	issuers, e = store.OutstandingCertificateIssuers(ctx)
	if e != nil || len(issuers) != 0 {
		t.Fatal("completed cleanup retained custody requirement", issuers, e)
	}
}

// No provider access: assertions are synthetic rows bound to a configured fixture.
func TestI01ProviderMFACompletesOIDCAndSharesExactReplayReceipt(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "providermfa")
	if _, e := s.Store.DB.Exec(`INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES('fixture-link',$1,'google','https://accounts.google.com','fixture-mfa-subject',clock_timestamp())`, a.Account.ID); e != nil {
		t.Fatal(e)
	}
	enrollment := securityAction(t, s, a, "mfa_enroll", "")
	key, e := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if e != nil {
		t.Fatal(e)
	}
	var now time.Time
	if e = s.Store.DB.QueryRow(`SELECT clock_timestamp()`).Scan(&now); e != nil {
		t.Fatal(e)
	}
	enabled, e := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, identity.HOTP(key, uint64(now.Unix()/30), 6), security.RandomID(""))
	if e != nil || len(enabled.RecoveryCodes) < 2 {
		t.Fatal("enable MFA", e)
	}
	o := &postgres.OIDCLogin{Store: s.Store, Keys: s.Keys, Security: s, Lifetime: 15 * time.Minute, Providers: map[string]*oidclogin.Provider{"google": {ID: "google", Issuer: "https://accounts.google.com", Audience: "fixture-client", Callback: "https://web.getportico.tv/auth/sso/google/callback"}}}
	if e = o.Configure(ctx); e != nil {
		t.Fatal(e)
	}
	binding := security.RandomID("")
	start := func() (string, *identity.MFAChallenge, string) {
		t.Helper()
		v, _, _, err := o.StartFlow(ctx, "google", binding, "", postgres.OIDCStartInput{Mode: "login", SessionMode: "native", RequestID: security.RandomID(""), Completion: postgres.NativeCompletionCode})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Store.DB.Exec(`INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,transaction_revision,lease_id) SELECT id,'https://accounts.google.com','fixture-mfa-subject','fixture-client',clock_timestamp(),clock_timestamp(),nonce_hash,'fixture@example.test',true,revision,'fixture-lease' FROM identity_transactions WHERE id=$1`, v.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Store.DB.Exec(`UPDATE identity_transactions SET status='verified' WHERE id=$1`, v.ID); err != nil {
			t.Fatal(err)
		}
		proof := security.RandomID("")
		if _, err = s.Store.DB.Exec(`INSERT INTO identity_native_completions(transaction_id,code_hash,expires_at) VALUES($1,$2,clock_timestamp()+interval '2 minutes')`, v.ID, security.Hash(proof)); err != nil {
			t.Fatal(err)
		}
		out, err := o.CompleteFlow(ctx, v.ID, binding, "", postgres.OIDCCompleteInput{CompletionCode: proof}, true)
		if err != nil || out.Session != nil || out.Challenge == nil {
			t.Fatalf("provider MFA bypass: %#v %v", out, err)
		}
		return v.ID, out.Challenge, proof
	}
	id, c, code := start()
	// A lost 202 is retried with the same, now consumed, completion code: the
	// retry reads back the same challenge instead of failing.
	again, e := o.CompleteFlow(ctx, id, binding, "", postgres.OIDCCompleteInput{CompletionCode: code}, true)
	if e != nil || again.Session != nil || again.Challenge == nil || again.Challenge.ChallengeID != c.ChallengeID {
		t.Fatalf("retry after a lost MFA response: %#v %v", again, e)
	}
	session, e := s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, enabled.RecoveryCodes[0])
	if e != nil {
		t.Fatal(e)
	}
	var status string
	if e = s.Store.DB.QueryRow(`SELECT status FROM identity_transactions WHERE id=$1`, id).Scan(&status); e != nil || status != "completed" {
		t.Fatal("provider transaction not terminal", status, e)
	}
	// A92: with the completion code the view shows the full details, so the
	// MFA flag is really checked (without it the view is status-only).
	view, e := o.View(ctx, id, binding, code)
	if e != nil || view == nil || view.MFARequired || view.Status != "completed" || !view.ContactVerified {
		t.Fatal("completed provider view still requires MFA", view, e)
	}
	retry, e := o.CompleteFlow(ctx, id, binding, "", postgres.OIDCCompleteInput{}, true)
	if e == nil || retry.Session != nil {
		t.Fatal("consumed callback proof returned an OIDC session")
	}
	factorRetry, e := s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, enabled.RecoveryCodes[0])
	if e != nil || factorRetry.AccessToken != session.AccessToken {
		t.Fatal("factor replay differs", e)
	}
	_, cancelled, _ := start()
	if e = o.Cancel(ctx, binding); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteMFA(ctx, cancelled.ChallengeID, cancelled.ChallengeToken, enabled.RecoveryCodes[1]); e == nil {
		t.Fatal("cancelled provider continuation issued a session")
	}
	var live int
	if e = s.Store.DB.QueryRow(`SELECT count(*) FROM session_families WHERE account_id=$1 AND revoked_at IS NULL`, a.Account.ID).Scan(&live); e != nil || live != 1 {
		t.Fatal("duplicate family from completed/cancelled provider flow", live, e)
	}
}
