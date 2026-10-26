package postgres_test

import (
	"context"
	"testing"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

// Repository protocol regression source, UNRUN. Provider assertions use the
// existing verified-assertion fixture seam; no provider or mail is contacted.
func TestCancelSecurityTransactionKeepsOtherProofsForSameIntent(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	binding := security.RandomID("")
	registration, err := s.CreateRegistration(ctx, binding, postgres.RegistrationInput{Username: "cancel-original", DisplayName: "Fixture", Password: "StrongPass9", ContactAddress: "cancel-original@example.test", SessionMode: "browser", RequestID: security.RandomID("")})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.VerifyRegistration(ctx, registration.ID, binding, mailPayload(t, s, mailoutbox.Registration, registration.ID).Code, "", "")
	if err != nil {
		t.Fatal(err)
	}
	o := &postgres.OIDCLogin{Store: s.Store, Keys: s.Keys, Security: s, Lifetime: 15 * time.Minute, Providers: map[string]*oidclogin.Provider{"google": {ID: "google", Issuer: "https://accounts.google.com", Audience: "fixture-client", Callback: "https://web.getportico.tv/auth/sso/google/callback"}}}
	if err = o.Configure(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.ExecContext(ctx, `INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES('cancel-provider',$1,'google','https://accounts.google.com','cancel-subject',clock_timestamp())`, a.Account.ID); err != nil {
		t.Fatal(err)
	}
	intent := identity.SecurityIntent{Purpose: "password_set_or_change", Target: "", OperationID: security.RandomID("")}
	start := func() string {
		t.Helper()
		view, _, _, e := o.StartFlow(ctx, "google", binding, a.AccessToken, postgres.OIDCStartInput{Mode: "reauthenticate", SessionMode: "browser", RequestID: security.RandomID(""), Intent: &intent, ExpectedIdentity: &postgres.ExpectedProviderIdentity{ID: "cancel-provider", Revision: 1}})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.Store.DB.ExecContext(ctx, `INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,transaction_revision,lease_id) SELECT id,'https://accounts.google.com','cancel-subject','fixture-client',clock_timestamp(),clock_timestamp(),nonce_hash,'cancel-original@example.test',true,revision,'fixture-lease' FROM identity_transactions WHERE id=$1`, view.ID); e != nil {
			t.Fatal(e)
		}
		if _, e = s.Store.DB.ExecContext(ctx, `UPDATE identity_transactions SET status='verified' WHERE id=$1`, view.ID); e != nil {
			t.Fatal(e)
		}
		return view.ID
	}
	complete := func(id string) identity.RecentProof {
		t.Helper()
		out, e := o.CompleteFlow(ctx, id, binding, a.AccessToken, postgres.OIDCCompleteInput{}, false)
		if e != nil || out.Proof == nil || out.Session != nil || out.Result != nil {
			t.Fatalf("credential completion did not issue only its proof: %v", e)
		}
		return *out.Proof
	}
	first, second := start(), start()
	if first == second {
		t.Fatal("distinct request IDs reused one transaction")
	}
	p1, p2 := complete(first), complete(second)
	password, err := s.PasswordProof(ctx, a.AccessToken, intent, "StrongPass9", "")
	if err != nil {
		t.Fatal(err)
	}
	var generation int64
	if err = s.Store.DB.QueryRowContext(ctx, `SELECT generation FROM identity_browser_bindings WHERE binding_hash=$1`, security.Hash(binding)).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if err = o.CancelSecurityTransaction(ctx, first, binding); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, proof string
		want        bool
	}{{"original", p1.Proof, false}, {"successor", p2.Proof, true}, {"password", password.Proof, true}} {
		var present bool
		if err = s.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_recent_proofs WHERE proof_hash=$1 AND consumed_at IS NULL)`, security.Hash(tc.proof)).Scan(&present); err != nil || present != tc.want {
			t.Fatalf("%s proof present=%v want=%v: %v", tc.name, present, tc.want, err)
		}
	}
	var after int64
	if err = s.Store.DB.QueryRowContext(ctx, `SELECT generation FROM identity_browser_bindings WHERE binding_hash=$1 AND expires_at>clock_timestamp()`, security.Hash(binding)).Scan(&after); err != nil || after != generation {
		t.Fatalf("shared binding changed: %v", err)
	}
	if _, err = o.CompleteFlow(ctx, first, binding, a.AccessToken, postgres.OIDCCompleteInput{}, false); err == nil {
		t.Fatal("cancelled original replayed its completion receipt")
	}
	if replay := complete(second); replay.Proof != p2.Proof {
		t.Fatal("successor receipt was changed")
	}
	if _, err = s.Mutate(ctx, a.AccessToken, postgres.SecurityAction{SecurityIntent: intent, Proof: p1.Proof, NewPassword: "ChangedPass9"}); err == nil {
		t.Fatal("cancelled original proof remained usable")
	}
	if _, err = s.Mutate(ctx, a.AccessToken, postgres.SecurityAction{SecurityIntent: intent, Proof: p2.Proof, NewPassword: "ChangedPass9"}); err != nil {
		t.Fatalf("successor proof could not perform its original action: %v", err)
	}
}
