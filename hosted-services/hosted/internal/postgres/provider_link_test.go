package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

// NEW-13: linking a provider needs the signed-in session only (no proof) and
// signs nobody out; unlinking keeps the caller signed in, signs other devices
// out, refuses the last usable sign-in method and acts
// only on the caller's own identities. Provider assertions are repository
// fixtures; no Google or Apple service is contacted.
func TestProviderLinkNeedsNoProofAndUnlinkKeepsTheLastMethod(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "linker")
	other := verifiedAccount(t, s, "bystander")
	o := &postgres.OIDCLogin{Store: s.Store, Keys: s.Keys, Security: s, Lifetime: 15 * time.Minute, Providers: map[string]*oidclogin.Provider{"google": {ID: "google", Issuer: "https://accounts.google.com", Audience: "fixture-client", Callback: "https://web.getportico.tv/auth/sso/google/callback"}}}
	if e := o.Configure(ctx); e != nil {
		t.Fatal(e)
	}
	binding := security.RandomID("")
	intent := &identity.SecurityIntent{Purpose: "provider_link", Target: "google", OperationID: security.RandomID("")}
	v, _, _, e := o.StartFlow(ctx, "google", binding, a.AccessToken, postgres.OIDCStartInput{Mode: "link", SessionMode: "native", RequestID: security.RandomID(""), Intent: intent, Completion: postgres.NativeCompletionCode})
	if e != nil {
		t.Fatalf("link start without a proof: %v", e)
	}
	if _, e = s.Store.DB.Exec(`INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,transaction_revision,lease_id) SELECT id,'https://accounts.google.com','fixture-link-subject','fixture-client',clock_timestamp(),clock_timestamp(),nonce_hash,'linker@example.test',true,revision,'fixture-lease' FROM identity_transactions WHERE id=$1`, v.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.Exec(`UPDATE identity_transactions SET status='verified' WHERE id=$1`, v.ID); e != nil {
		t.Fatal(e)
	}
	linkProof := security.RandomID("")
	if _, e = s.Store.DB.Exec(`INSERT INTO identity_native_completions(transaction_id,code_hash,expires_at) VALUES($1,$2,clock_timestamp()+interval '2 minutes')`, v.ID, security.Hash(linkProof)); e != nil {
		t.Fatal(e)
	}
	out, e := o.CompleteFlow(ctx, v.ID, binding, a.AccessToken, postgres.OIDCCompleteInput{CompletionCode: linkProof}, true)
	if e != nil || out.Result == nil || out.Result.SignOut || out.Session != nil {
		t.Fatalf("link completion: %#v %v", out, e)
	}
	if _, e = s.Store.Authenticate(ctx, security.Hash(a.AccessToken)); e != nil {
		t.Fatalf("linking signed the account out: %v", e)
	}
	var link string
	if e = s.Store.DB.QueryRow(`SELECT id FROM account_external_identities WHERE account_id=$1 AND provider='google' AND revoked_at IS NULL`, a.Account.ID).Scan(&link); e != nil {
		t.Fatal("linked identity missing", e)
	}
	unlink := func(session identity.Session, target string) (identity.SecurityResult, error) {
		return s.Mutate(ctx, session.AccessToken, postgres.SecurityAction{SecurityIntent: identity.SecurityIntent{Purpose: "provider_unlink", Target: target, OperationID: security.RandomID("")}})
	}
	// Only the account itself acts: another account can't unlink this identity.
	if _, e = unlink(other, link); !errors.Is(e, identity.ErrProviderConflict) {
		t.Fatalf("another account unlinked the identity: %v", e)
	}
	// With no password the provider is the last usable method.
	var hash []byte
	if e = s.Store.DB.QueryRow(`SELECT password_hash FROM accounts WHERE id=$1`, a.Account.ID).Scan(&hash); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.Exec(`UPDATE accounts SET password_hash=NULL WHERE id=$1`, a.Account.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = unlink(a, link); !errors.Is(e, identity.ErrLastMethod) {
		t.Fatalf("last sign-in method removed: %v", e)
	}
	if _, e = s.Store.DB.Exec(`UPDATE accounts SET password_hash=$2 WHERE id=$1`, a.Account.ID, hash); e != nil {
		t.Fatal(e)
	}
	// Another device signs in with the linked Google identity.
	loginBinding := security.RandomID("")
	lv, _, e := o.Start(ctx, "google", loginBinding)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.Exec(`INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,transaction_revision,lease_id) SELECT id,'https://accounts.google.com','fixture-link-subject','fixture-client',clock_timestamp(),clock_timestamp(),nonce_hash,'linker@example.test',true,revision,'fixture-lease' FROM identity_transactions WHERE id=$1`, lv.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.Exec(`UPDATE identity_transactions SET status='verified' WHERE id=$1`, lv.ID); e != nil {
		t.Fatal(e)
	}
	googleDevice, e := o.Complete(ctx, lv.ID, loginBinding, "", "")
	if e != nil || googleDevice.Account.ID != a.Account.ID {
		t.Fatalf("google sign-in on another device: %#v %v", googleDevice.Account, e)
	}
	// With the password left, unlinking needs no proof, keeps the caller
	// signed in and signs the other device out.
	result, e := unlink(a, link)
	if e != nil || result.SignOut {
		t.Fatalf("unlink without a proof: %#v %v", result, e)
	}
	if _, e = s.Store.Authenticate(ctx, security.Hash(a.AccessToken)); e != nil {
		t.Fatalf("unlinking signed the caller out: %v", e)
	}
	if _, e = s.Store.Authenticate(ctx, security.Hash(googleDevice.AccessToken)); e == nil {
		t.Fatal("the other device's Google session survived the unlink")
	}
	if _, e = s.Store.Authenticate(ctx, security.Hash(other.AccessToken)); e != nil {
		t.Fatalf("another account was signed out: %v", e)
	}
	var live int
	if e = s.Store.DB.QueryRow(`SELECT count(*) FROM account_external_identities WHERE account_id=$1 AND revoked_at IS NULL`, a.Account.ID).Scan(&live); e != nil || live != 0 {
		t.Fatal("identity still linked", live, e)
	}
}
