package postgres_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"testing"
	"time"
)

// Provider assertion insertion is deliberately a repository fixture, not real SSO evidence.
func TestOIDCBindingCompletionAndRevocation(t *testing.T) {
	f := currentFamilySetup(t)
	ctx := context.Background()
	keys := &security.Keys{SealKey: bytes.Repeat([]byte{3}, 32)}
	o := &postgres.OIDCLogin{Store: f.store, Keys: keys, Security: &postgres.AccountSecurity{Store: f.store, Keys: keys, Identity: identity.New(f.store), Lifetime: 15 * time.Minute, MailEnabled: true}, Lifetime: 15 * time.Minute, Providers: map[string]*oidclogin.Provider{"google": {ID: "google", Issuer: "https://accounts.google.com", Audience: "fixture-client", Callback: "https://web.getportico.tv/auth/sso/google/callback"}}}
	if e := o.Configure(ctx); e != nil {
		t.Fatal(e)
	}
	binding := security.RandomID("")
	v, _, e := o.Start(ctx, "google", binding)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.store.DB.ExecContext(ctx, `INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,transaction_revision,lease_id) SELECT id,'https://accounts.google.com','fixture-subject','fixture-client',clock_timestamp(),clock_timestamp(),nonce_hash,'fixture@example.test',true,revision,'fixture-lease' FROM identity_transactions WHERE id=$1;`, v.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.ExecContext(ctx, `UPDATE identity_transactions SET status='verified' WHERE id=$1`, v.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = o.Complete(ctx, v.ID, "wrong-binding", "fixture-oidc", "Fixture"); e == nil {
		t.Fatal("wrong binding accepted")
	}
	session, e := o.Complete(ctx, v.ID, binding, "fixture-oidc", "Fixture")
	if e != nil {
		t.Fatal(e)
	}
	again, e := o.Complete(ctx, v.ID, binding, "ignored-name", "Ignored")
	if e != nil || again.AccessToken != session.AccessToken {
		t.Fatalf("receipt replay %v", e)
	}
	if _, e = f.store.Authenticate(ctx, security.Hash(session.AccessToken)); e != nil {
		t.Fatal(e)
	}
	if e = f.store.LogoutAll(ctx, session.Account.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = o.Complete(ctx, v.ID, binding, "fixture-oidc", "Fixture"); e == nil {
		t.Fatal("revoked receipt accepted")
	}
	next, _, e := o.Start(ctx, "google", binding)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = o.View(ctx, v.ID, binding, ""); e == nil {
		t.Fatal("superseded transaction accepted")
	}
	if e = o.Cancel(ctx, binding); e != nil {
		t.Fatal(e)
	}
	if _, e = o.View(ctx, next.ID, binding, ""); e == nil {
		t.Fatal("cancelled binding accepted")
	}
}

// Reproduces SEC-01's completion leg after the victim's provider assertion has
// been verified. No Google/Apple service is contacted by this fixture.
func TestNativeSSOInitiatorCannotTakeVictimSession(t *testing.T) {
	f := currentFamilySetup(t)
	ctx := context.Background()
	keys := &security.Keys{SealKey: bytes.Repeat([]byte{3}, 32)}
	o := &postgres.OIDCLogin{Store: f.store, Keys: keys, Security: &postgres.AccountSecurity{Store: f.store, Keys: keys, Identity: identity.New(f.store), Lifetime: 15 * time.Minute, MailEnabled: true}, Lifetime: 15 * time.Minute, Providers: map[string]*oidclogin.Provider{"google": {ID: "google", Issuer: "https://accounts.google.com", Audience: "fixture-client", Callback: "https://web.getportico.tv/auth/sso/google/callback"}}}
	if err := o.Configure(ctx); err != nil {
		t.Fatal(err)
	}
	// The victim already has a Portico account. Successful exploitation would
	// issue credentials for that existing account, not merely create a signup.
	if _, err := f.store.DB.ExecContext(ctx, `INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES('victim-identity','strict-owner','google','https://accounts.google.com','victim-subject',clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	attackerBinding := security.RandomID("")
	v, _, _, err := o.StartFlow(ctx, "google", attackerBinding, "", postgres.OIDCStartInput{Mode: "login", SessionMode: "native", RequestID: security.RandomID(""), Completion: postgres.NativeCompletionCode})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.DB.ExecContext(ctx, `INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,transaction_revision,lease_id) SELECT id,'https://accounts.google.com','victim-subject','fixture-client',clock_timestamp(),clock_timestamp(),nonce_hash,'victim@example.test',true,revision,'fixture-lease' FROM identity_transactions WHERE id=$1`, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.DB.ExecContext(ctx, `UPDATE identity_transactions SET status='verified' WHERE id=$1`, v.ID); err != nil {
		t.Fatal(err)
	}
	// A12: the initiator polling View sees the status only, not the victim's
	// name, account existence, contact state or an onboarding token.
	if view, err := o.View(ctx, v.ID, attackerBinding, ""); err != nil || view.SuggestedName != "" || view.NeedsUsername || view.ContactVerified || view.OnboardingToken != "" || view.Status != "verified" {
		t.Fatalf("initiator view leaked details: %+v %v", view, err)
	}
	q := postgres.OIDCCompleteInput{Username: "victim", DisplayName: "Victim"}
	if result, err := o.CompleteFlow(ctx, v.ID, attackerBinding, "", q, false); err == nil || result.Session != nil {
		t.Fatal("initiator acquired victim session without callback proof")
	}
	code := security.RandomID("")
	if _, err = f.store.DB.ExecContext(ctx, `INSERT INTO identity_native_completions(transaction_id,code_hash,expires_at) VALUES($1,$2,clock_timestamp()+interval '2 minutes')`, v.ID, security.Hash(code)); err != nil {
		t.Fatal(err)
	}
	q.CompletionCode = security.RandomID("")
	if _, err = o.CompleteFlow(ctx, v.ID, attackerBinding, "", q, false); err == nil {
		t.Fatal("wrong completion code accepted")
	}
	if view, err := o.View(ctx, v.ID, attackerBinding, security.RandomID("")); err != nil || view.ContactVerified || view.Status != "verified" {
		t.Fatalf("wrong completion code revealed details: %+v %v", view, err)
	}
	if view, err := o.View(ctx, v.ID, attackerBinding, code); err != nil || !view.ContactVerified || view.Status != "onboarding" {
		t.Fatalf("completion code holder denied details: %+v %v", view, err)
	}
	q.CompletionCode = code
	result, err := o.CompleteFlow(ctx, v.ID, attackerBinding, "", q, false)
	if err != nil || result.Session == nil {
		t.Fatalf("callback holder denied: %v", err)
	}
	if result.Session.Account.ID != "strict-owner" {
		t.Fatal("callback did not sign in to the victim account")
	}
	// A13: a retry after a lost response gets the stored receipt, not a new
	// session; any other code, or none, still fails.
	retry, err := o.CompleteFlow(ctx, v.ID, attackerBinding, "", q, false)
	if err != nil || retry.Session == nil || retry.Session.AccessToken != result.Session.AccessToken || retry.Session.FamilyID != result.Session.FamilyID {
		t.Fatalf("lost-response retry did not return the stored receipt: %+v %v", retry.Session, err)
	}
	var families int
	if err = f.store.DB.QueryRow(`SELECT count(*) FROM session_families WHERE account_id='strict-owner'`).Scan(&families); err != nil {
		t.Fatal(err)
	}
	q.CompletionCode = security.RandomID("")
	if _, err = o.CompleteFlow(ctx, v.ID, attackerBinding, "", q, false); err == nil {
		t.Fatal("completed transaction accepted another code")
	}
	q.CompletionCode = code
	if _, err = o.CompleteFlow(ctx, v.ID, attackerBinding, "", q, false); err != nil {
		t.Fatal(err)
	}
	var after int
	if err = f.store.DB.QueryRow(`SELECT count(*) FROM session_families WHERE account_id='strict-owner'`).Scan(&after); err != nil || after != families {
		t.Fatalf("replay minted a session family: %d -> %d %v", families, after, err)
	}
}

func TestCurrentHostedRestartNeedsNoDDL(t *testing.T) {
	f := currentFamilySetup(t)
	ctx := context.Background()
	tx, err := f.store.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = postgres.InitializeCurrentHostedSchemaTx(ctx, tx); err != nil {
		t.Fatalf("restart attempted a write or DDL: %v", err)
	}
}

// Every native sign-in through the browser must ask for the completion code.
// Without it the party that started the transaction could collect the session
// of whoever signed in at the provider (a phishing link), so a start without
// the code is refused, as is an unknown completion value or one on a browser or
// native-Apple start.
func TestNativeSignInRequiresTheCompletionCode(t *testing.T) {
	f := currentFamilySetup(t)
	ctx := context.Background()
	keys := &security.Keys{SealKey: bytes.Repeat([]byte{3}, 32)}
	o := &postgres.OIDCLogin{Store: f.store, Keys: keys, Security: &postgres.AccountSecurity{Store: f.store, Keys: keys, Identity: identity.New(f.store), Lifetime: 15 * time.Minute, MailEnabled: true}, Lifetime: 15 * time.Minute, Providers: map[string]*oidclogin.Provider{"google": {ID: "google", Issuer: "https://accounts.google.com", Audience: "fixture-client", Callback: "https://web.getportico.tv/auth/sso/google/callback"}}}
	if err := o.Configure(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB.ExecContext(ctx, `INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES('owner-identity','strict-owner','google','https://accounts.google.com','owner-subject',clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []postgres.OIDCStartInput{
		{Mode: "login", SessionMode: "native", RequestID: security.RandomID("")},
		{Mode: "link", SessionMode: "native", RequestID: security.RandomID("")},
		{Mode: "login", SessionMode: "native", RequestID: security.RandomID(""), Completion: "token"},
		{Mode: "login", SessionMode: "browser", RequestID: security.RandomID(""), Completion: postgres.NativeCompletionCode},
	} {
		if _, _, _, err := o.StartFlow(ctx, "google", security.RandomID(""), "", bad); !errors.Is(err, identity.ErrInvalid) {
			t.Fatalf("start %+v: %v", bad, err)
		}
	}
	opted, _, _, err := o.StartFlow(ctx, "google", security.RandomID(""), "", postgres.OIDCStartInput{Mode: "login", SessionMode: "native", RequestID: security.RandomID(""), Completion: postgres.NativeCompletionCode})
	if err != nil {
		t.Fatal(err)
	}
	var got bool
	if err = f.store.DB.QueryRow(`SELECT native_completion FROM identity_transactions WHERE id=$1`, opted.ID).Scan(&got); err != nil || !got {
		t.Fatalf("native_completion = %v (%v), want true", got, err)
	}
}
