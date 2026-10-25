package postgres_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

type emailViewFixture struct {
	store   *postgres.Store
	login   *postgres.OIDCLogin
	handler http.Handler
}

func emailViewSetup(t *testing.T) emailViewFixture {
	t.Helper()
	f := currentFamilySetup(t)
	keys := &security.Keys{SealKey: bytes.Repeat([]byte{3}, 32)}
	s := &postgres.AccountSecurity{Store: f.store, Keys: keys, Identity: identity.New(f.store), Lifetime: 15 * time.Minute, MailEnabled: true}
	o := &postgres.OIDCLogin{Store: f.store, Keys: keys, Security: s, Lifetime: 15 * time.Minute, Providers: map[string]*oidclogin.Provider{"google": {ID: "google", Issuer: "https://accounts.google.com", Audience: "fixture-client", Callback: "https://web.getportico.tv/auth/sso/google/callback"}}}
	if e := o.Configure(context.Background()); e != nil {
		t.Fatal(e)
	}
	h := (&httpapi.Handler{Identity: s.Identity, Security: s, OIDC: o, AccountOrigin: "https://web.getportico.tv", Origins: map[string]bool{"https://web.getportico.tv": true}}).Routes()
	return emailViewFixture{f.store, o, h}
}

func (f emailViewFixture) start(t *testing.T, native bool) (postgres.OIDCView, string) {
	t.Helper()
	q := postgres.OIDCStartInput{Mode: "login", SessionMode: "browser", RequestID: security.RandomID("")}
	if native {
		q.SessionMode, q.Completion = "native", postgres.NativeCompletionCode
	}
	binding := security.RandomID("")
	v, _, _, e := f.login.StartFlow(context.Background(), "google", binding, "", q)
	if e != nil {
		t.Fatal(e)
	}
	return v, binding
}

// This is the established verified-provider repository fixture, not a real
// Google login. HTTP view, collision, contact and authority consumers run.
func (f emailViewFixture) assertion(t *testing.T, v postgres.OIDCView, email string, verified bool) {
	t.Helper()
	_, e := f.store.DB.Exec(`INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,transaction_revision,lease_id) SELECT id,'https://accounts.google.com','email_'||id,'fixture-client',clock_timestamp(),clock_timestamp(),nonce_hash,NULLIF($2,''),$3,revision,'email-fixture-lease' FROM identity_transactions WHERE id=$1`, v.ID, email, verified)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`UPDATE identity_transactions SET status='verified' WHERE id=$1`, v.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`DELETE FROM identity_exchange_secrets WHERE transaction_id=$1`, v.ID); e != nil {
		t.Fatal(e)
	}
}

func (f emailViewFixture) call(t *testing.T, v postgres.OIDCView, binding string, native bool, completion, method, body string) (*httptest.ResponseRecorder, map[string]json.RawMessage) {
	t.Helper()
	path := "/v1/identity/transactions/" + v.ID
	if method == "POST" {
		path += "/complete"
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if native {
		r.Header.Set("X-Portico-Continuation", binding)
	} else {
		r.Header.Set("Origin", "https://web.getportico.tv")
		r.AddCookie(&http.Cookie{Name: "__Host-portico_identity", Value: binding})
	}
	if completion != "" {
		r.Header.Set("X-Portico-Completion", completion)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	out := httptest.NewRecorder()
	f.handler.ServeHTTP(out, r)
	var fields map[string]json.RawMessage
	if json.Unmarshal(out.Body.Bytes(), &fields) != nil {
		t.Fatalf("identity response is not an object, status %d", out.Code)
	}
	return out, fields
}

func emailField(t *testing.T, fields map[string]json.RawMessage, key string) string {
	t.Helper()
	var value string
	if raw := fields[key]; raw != nil && json.Unmarshal(raw, &value) != nil {
		t.Fatalf("identity field %s is not a string", key)
	}
	return value
}

func captureIdentityFacts(t *testing.T, name string, out *httptest.ResponseRecorder, fields map[string]json.RawMessage) {
	t.Helper()
	dir := os.Getenv("PORTICO_IDENTITY_FACT_FIXTURES_DIR")
	if dir == "" {
		return
	}
	selected := map[string]json.RawMessage{}
	for _, key := range []string{"status", "provider", "contactVerified", "needsUsername", "suggestedName", "verifiedEmail"} {
		if raw := fields[key]; raw != nil {
			selected[key] = raw
		}
	}
	raw, e := json.MarshalIndent(struct {
		Status        int                        `json:"status"`
		ContentType   string                     `json:"contentType"`
		Qualification string                     `json:"qualification"`
		Fields        map[string]json.RawMessage `json:"selectedActualResponseFields"`
	}{out.Code, out.Header().Get("Content-Type"), "Selected actual HTTP transaction fields only; private onboarding token, request/callback credentials and raw provider response excluded. Repository assertion fixture, not external SSO or UI seed data.", selected}, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(dir, 0o700); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(dir, name+".json")
	if e = os.WriteFile(p+".tmp", append(raw, '\n'), 0o600); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(p+".tmp", p); e != nil {
		t.Fatal(e)
	}
}

func TestOIDCVerifiedEmailHTTPFactsAndCollision(t *testing.T) {
	f := emailViewSetup(t)
	v, binding := f.start(t, false)
	out, fields := f.call(t, v, binding, false, "", "GET", "")
	if out.Code != 200 || fields["verifiedEmail"] != nil {
		t.Fatal("started transaction exposed an email")
	}
	f.assertion(t, v, "provider-fixture@example.test", true)
	out, fields = f.call(t, v, binding, false, "", "GET", "")
	if out.Code != 200 || emailField(t, fields, "verifiedEmail") != "provider-fixture@example.test" || emailField(t, fields, "provider") != "google" || string(fields["contactVerified"]) != "true" || fields["suggestedName"] != nil {
		t.Fatal("verified transaction facts do not match their producer")
	}
	captureIdentityFacts(t, "provider-onboarding", out, fields)
	token := emailField(t, fields, "onboardingToken")
	body, _ := json.Marshal(postgres.OIDCCompleteInput{Username: "strict-owner", DisplayName: "User-entered fixture name", OnboardingToken: token})
	out, fields = f.call(t, v, binding, false, "", "POST", string(body))
	if out.Code != 409 {
		t.Fatalf("real username collision status %d", out.Code)
	}
	var collision struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(fields["error"], &collision) != nil || collision.Code != "username_unavailable" {
		t.Fatal("confirmed username refusal lost its closed code")
	}
	out, fields = f.call(t, v, binding, false, "", "GET", "")
	if out.Code != 200 || emailField(t, fields, "verifiedEmail") != "provider-fixture@example.test" || emailField(t, fields, "status") != "onboarding" {
		t.Fatal("collision lost the verified current transaction fact")
	}
	captureIdentityFacts(t, "provider-username-collision", out, fields)
	currentToken := emailField(t, fields, "onboardingToken")
	if currentToken == "" || currentToken == token {
		t.Fatal("collision did not rotate the original onboarding token")
	}
	retry, _ := json.Marshal(postgres.OIDCCompleteInput{Username: "provider_retry", DisplayName: "User-entered fixture name", OnboardingToken: currentToken})
	out, _ = f.call(t, v, binding, false, "", "POST", string(retry))
	if out.Code != 200 {
		t.Fatalf("same proved transaction could not finish after choosing another username: %d", out.Code)
	}
}

func TestOIDCVerifiedEmailUsesOnlyCurrentVerifiedContact(t *testing.T) {
	f := emailViewSetup(t)
	v, binding := f.start(t, false)
	f.assertion(t, v, "unverified-provider@example.test", false)
	out, fields := f.call(t, v, binding, false, "", "GET", "")
	if out.Code != 200 || fields["verifiedEmail"] != nil {
		t.Fatal("unverified provider address exposed")
	}
	challenge, e := f.login.SendContact(context.Background(), v.ID, binding, "current-contact@example.test", security.RandomID(""))
	if e != nil {
		t.Fatal(e)
	}
	out, fields = f.call(t, v, binding, false, "", "GET", "")
	if out.Code != 200 || fields["verifiedEmail"] != nil {
		t.Fatal("unverified contact challenge exposed")
	}
	delivery, e := (&postgres.AccountMail{Store: f.store, Keys: f.login.Keys}).ClaimMail(context.Background())
	if e != nil || delivery == nil {
		t.Fatal("actual contact mail consumer", e)
	}
	if _, e = f.login.VerifyContact(context.Background(), v.ID, binding, challenge.ChallengeID, delivery.Payload.Code); e != nil {
		t.Fatal(e)
	}
	// The completion consumer also prefers this current verified challenge over
	// a verified assertion. Both facts here are explicit repository fixtures.
	if _, e = f.store.DB.Exec(`UPDATE identity_verified_assertions SET email_verified=true WHERE transaction_id=$1`, v.ID); e != nil {
		t.Fatal(e)
	}
	out, fields = f.call(t, v, binding, false, "", "GET", "")
	if out.Code != 200 || emailField(t, fields, "verifiedEmail") != "current-contact@example.test" {
		t.Fatal("current verified contact did not take precedence")
	}
	captureIdentityFacts(t, "verified-current-contact", out, fields)
	if _, e = f.store.DB.Exec(`UPDATE identity_contact_challenges SET is_current=false WHERE id=$1`, challenge.ChallengeID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`UPDATE identity_verified_assertions SET email_verified=false WHERE transaction_id=$1`, v.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES('email-known','strict-owner','google','https://accounts.google.com',$1,clock_timestamp())`, "email_"+v.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`INSERT INTO account_contact_emails(id,account_id,address,verified_at,source,is_primary) VALUES('email-primary','strict-owner','account-primary@example.test',clock_timestamp(),'challenge',true)`); e != nil {
		t.Fatal(e)
	}
	other, otherBinding := f.start(t, false)
	f.assertion(t, other, "", false)
	foreign, e := f.login.SendContact(context.Background(), other.ID, otherBinding, "other-transaction@example.test", security.RandomID(""))
	if e != nil {
		t.Fatal(e)
	}
	otherDelivery, e := (&postgres.AccountMail{Store: f.store, Keys: f.login.Keys}).ClaimMail(context.Background())
	if e != nil || otherDelivery == nil {
		t.Fatal("other actual contact mail consumer", e)
	}
	if _, e = f.login.VerifyContact(context.Background(), other.ID, otherBinding, foreign.ChallengeID, otherDelivery.Payload.Code); e != nil {
		t.Fatal(e)
	}
	out, fields = f.call(t, v, binding, false, "", "GET", "")
	if out.Code != 200 || fields["verifiedEmail"] != nil || string(fields["contactVerified"]) != "true" {
		t.Fatal("old contact or linked account primary became a transaction email")
	}
	captureIdentityFacts(t, "linked-primary-is-not-transaction-contact", out, fields)
}

func TestOIDCVerifiedEmailNativeCompletionPrivacy(t *testing.T) {
	f := emailViewSetup(t)
	v, binding := f.start(t, true)
	f.assertion(t, v, "native-provider@example.test", true)
	code := security.RandomID("")
	if _, e := f.store.DB.Exec(`INSERT INTO identity_native_completions(transaction_id,code_hash,expires_at) VALUES($1,$2,clock_timestamp()+interval '2 minutes')`, v.ID, security.Hash(code)); e != nil {
		t.Fatal(e)
	}
	for _, proof := range []string{"", security.RandomID("")} {
		out, fields := f.call(t, v, binding, true, proof, "GET", "")
		if out.Code != 200 || fields["verifiedEmail"] != nil || fields["onboardingToken"] != nil || string(fields["contactVerified"]) != "false" {
			t.Fatal("native initiator without callback proof saw private facts")
		}
	}
	out, fields := f.call(t, v, security.RandomID(""), true, code, "GET", "")
	if out.Code == 200 || fields["verifiedEmail"] != nil {
		t.Fatal("callback proof bypassed original binding")
	}
	out, fields = f.call(t, v, binding, true, code, "GET", "")
	if out.Code != 200 || emailField(t, fields, "verifiedEmail") != "native-provider@example.test" {
		t.Fatal("actual callback holder denied verified transaction fact")
	}
	captureIdentityFacts(t, "native-callback-proof-holder", out, fields)
	if _, e := f.store.DB.Exec(`UPDATE identity_native_completions SET expires_at=clock_timestamp()-interval '1 second' WHERE transaction_id=$1`, v.ID); e != nil {
		t.Fatal(e)
	}
	out, fields = f.call(t, v, binding, true, code, "GET", "")
	if out.Code != 200 || fields["verifiedEmail"] != nil || fields["onboardingToken"] != nil || string(fields["contactVerified"]) != "false" {
		t.Fatal("expired callback proof exposed private transaction facts")
	}
}

func TestOIDCVerifiedEmailRechecksTransactionAuthority(t *testing.T) {
	for _, scenario := range []string{"wrong-binding", "binding-generation", "policy", "provider", "expiry", "failed-status"} {
		t.Run(scenario, func(t *testing.T) {
			f := emailViewSetup(t)
			v, binding := f.start(t, false)
			f.assertion(t, v, "private-authority@example.test", true)
			var e error
			switch scenario {
			case "wrong-binding":
				binding = security.RandomID("")
			case "binding-generation":
				_, e = f.store.DB.Exec(`UPDATE identity_browser_bindings SET generation=generation+1 WHERE binding_hash=$1`, security.Hash(binding))
			case "policy":
				_, e = f.store.DB.Exec(`UPDATE identity_protocol_policy SET revision=revision+1`)
			case "provider":
				_, e = f.store.DB.Exec(`UPDATE identity_provider_descriptors SET enabled=false`)
			case "expiry":
				_, e = f.store.DB.Exec(`UPDATE identity_transactions SET created_at=statement_timestamp()-interval '11 minutes',expires_at=statement_timestamp()-interval '1 minute' WHERE id=$1`, v.ID)
			case "failed-status":
				_, e = f.store.DB.Exec(`UPDATE identity_transactions SET status='failed',terminal_at=clock_timestamp() WHERE id=$1`, v.ID)
			}
			if e != nil {
				t.Fatal(e)
			}
			out, fields := f.call(t, v, binding, false, "", "GET", "")
			if fields["verifiedEmail"] != nil || strings.Contains(out.Body.String(), "private-authority@example.test") || (scenario != "failed-status" && out.Code == 200) {
				t.Fatal("stale transaction authority exposed an email")
			}
		})
	}
}
