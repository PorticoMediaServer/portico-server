package postgres_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

func appleFixture(t *testing.T) (currentFamilyFixture, *postgres.OIDCLogin) {
	t.Helper()
	f := currentFamilySetup(t)
	keys := &security.Keys{SealKey: bytes.Repeat([]byte{3}, 32)}
	o := &postgres.OIDCLogin{Store: f.store, Keys: keys, Lifetime: 15 * time.Minute, Security: &postgres.AccountSecurity{Store: f.store, Keys: keys, Identity: identity.New(f.store), Lifetime: 15 * time.Minute, MailEnabled: true}, Providers: map[string]*oidclogin.Provider{"apple": {ID: "apple", Issuer: oidclogin.AppleIssuer, Audience: "tv.getportico.account", Callback: "https://web.getportico.tv/auth/sso/apple/callback"}}}
	if err := o.Configure(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := f.store.DB.Exec(`INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES('apple-link','strict-owner','apple','https://appleid.apple.com','apple-subject',clock_timestamp()); INSERT INTO account_contact_emails(id,account_id,address,verified_at,source,source_identity_id,private_relay,is_primary) VALUES('relay-contact','strict-owner','relay@privaterelay.appleid.com',clock_timestamp(),'provider','apple-link',true,true)`)
	if err != nil {
		t.Fatal(err)
	}
	return f, o
}
func appleEvent(id, kind string, at time.Time) oidclogin.AppleNotification {
	return oidclogin.AppleNotification{ID: id, Audience: "tv.getportico.ios", Type: kind, Subject: "apple-subject", Email: "relay@privaterelay.appleid.com", EventTime: at}
}
func applyApple(t *testing.T, o *postgres.OIDCLogin, n oidclogin.AppleNotification) {
	t.Helper()
	if err := o.ApplyAppleNotification(context.Background(), n); err != nil {
		t.Fatal(err)
	}
}

func TestAppleNotificationRevocationAndDuplicates(t *testing.T) {
	f, o := appleFixture(t)
	ctx := context.Background()
	n := appleEvent("revoke-1", "consent-revoked", time.Now().Add(-time.Second))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := o.ApplyAppleNotification(ctx, n); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if _, err := f.store.Authenticate(ctx, security.Hash("strict-owner-access")); err == nil {
		t.Fatal("revoked account session survived")
	}
	if _, err := f.store.Authenticate(ctx, security.Hash("strict-other-access")); err != nil {
		t.Fatal("unrelated session revoked", err)
	}
	var epoch, receipts int
	if err := f.store.DB.QueryRow(`SELECT auth_epoch,(SELECT count(*) FROM apple_account_notifications) FROM accounts WHERE id='strict-owner'`).Scan(&epoch, &receipts); err != nil || epoch != 1 || receipts != 1 {
		t.Fatalf("dedupe epoch=%d receipts=%d error=%v", epoch, receipts, err)
	}
	n.Type = "account-deleted"
	if err := o.ApplyAppleNotification(ctx, n); err == nil {
		t.Fatal("same ID with different event accepted")
	}
	if err := f.store.Migrate(ctx); err != nil {
		t.Fatal("restart migration", err)
	}
	applyApple(t, o, appleEvent("older", "consent-revoked", n.EventTime.Add(-time.Hour)))
	if err := f.store.DB.QueryRow(`SELECT auth_epoch FROM accounts WHERE id='strict-owner'`).Scan(&epoch); err != nil || epoch != 1 {
		t.Fatal("older event repeated revocation", err)
	}
}

func queueAppleMail(t *testing.T, f currentFamilyFixture, o *postgres.OIDCLogin) *mailoutbox.Delivery {
	t.Helper()
	ctx := context.Background()
	tx, err := f.store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// A live password-reset challenge to the relay address: one of the mails
	// Hosted still sends, so the relay filter decides.
	if _, err = tx.ExecContext(ctx, `UPDATE accounts SET password_hash='\x01' WHERE id='strict-owner'; INSERT INTO account_email_challenges(id,account_id,purpose,address,contact_id,account_epoch,code_hash,expires_at,operation_id,request_hash) SELECT 'strict-reset','strict-owner','password_reset','relay@privaterelay.appleid.com','relay-contact',auth_epoch,'\x02',clock_timestamp()+interval '1 hour','strict-op','\x03' FROM accounts WHERE id='strict-owner'`); err != nil {
		t.Fatal(err)
	}
	err = postgres.EnqueueAccountMailTx(ctx, tx, o.Keys, mailoutbox.Intent{ID: security.RandomID("mail_"), Kind: mailoutbox.PasswordReset, ResourceID: "strict-reset", Revision: 1, AccountID: "strict-owner", ExpiresAt: time.Now().Add(time.Hour), Payload: mailoutbox.Payload{To: "relay@privaterelay.appleid.com", Code: "reset-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	m := &postgres.AccountMail{Store: f.store, Keys: o.Keys}
	d, err := m.ClaimMail(ctx)
	if err != nil || d == nil {
		t.Fatal("claim", err)
	}
	return d
}
func TestAppleRelayOrderingAndDeletion(t *testing.T) {
	f, o := appleFixture(t)
	ctx := context.Background()
	d := queueAppleMail(t, f, o)
	m := &postgres.AccountMail{Store: f.store, Keys: o.Keys}
	now := time.Now().Add(-time.Minute)
	check := func(want bool) {
		t.Helper()
		got, err := m.MailEligible(ctx, *d)
		if err != nil || got != want {
			t.Fatalf("mail eligible=%v want=%v err=%v", got, want, err)
		}
	}
	check(true)
	applyApple(t, o, appleEvent("disabled", "email-disabled", now))
	check(false)
	applyApple(t, o, appleEvent("old-enabled", "email-enabled", now.Add(-time.Second)))
	check(false)
	applyApple(t, o, appleEvent("enabled", "email-enabled", now.Add(time.Second)))
	check(true)
	applyApple(t, o, appleEvent("same-disabled", "email-disabled", now.Add(time.Second)))
	check(false)
	applyApple(t, o, appleEvent("same-enabled", "email-enabled", now.Add(time.Second)))
	check(false)
	applyApple(t, o, appleEvent("deleted", "account-deleted", now.Add(2*time.Second)))
	check(false)
	applyApple(t, o, appleEvent("after-deletion", "email-enabled", now.Add(3*time.Second)))
	check(false)
	var count int
	if err := f.store.DB.QueryRow(`SELECT count(*) FROM accounts WHERE id='strict-owner'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("Portico account erased", err)
	}
}

func seedAppleFlow(t *testing.T, o *postgres.OIDCLogin, subject string, issued time.Time) (string, string) {
	t.Helper()
	ctx := context.Background()
	binding := security.RandomID("")
	v, _, err := o.Start(ctx, "apple", binding)
	if err != nil {
		t.Fatal(err)
	}
	_, err = o.Store.DB.Exec(`INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,transaction_revision,lease_id) SELECT id,'https://appleid.apple.com',$2,'tv.getportico.account',$3,clock_timestamp(),nonce_hash,'fixture@example.test',true,revision,'fixture-lease' FROM identity_transactions WHERE id=$1`, v.ID, subject, issued)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = o.Store.DB.Exec(`UPDATE identity_transactions SET status='verified' WHERE id=$1`, v.ID); err != nil {
		t.Fatal(err)
	}
	return v.ID, binding
}
func TestAppleRevocationCompletionFenceAndFreshConsent(t *testing.T) {
	f, o := appleFixture(t)
	ctx := context.Background()
	now := time.Now().Add(-time.Minute)
	stale, binding := seedAppleFlow(t, o, "apple-subject", now.Add(-time.Second))
	applyApple(t, o, appleEvent("revoked", "consent-revoked", now))
	if _, err := o.Complete(ctx, stale, binding, "unused", "Unused"); err == nil {
		t.Fatal("stale assertion completed")
	}
	fresh, binding := seedAppleFlow(t, o, "apple-subject", time.Now())
	session, err := o.Complete(ctx, fresh, binding, "unused", "Unused")
	if err != nil {
		t.Fatal("fresh consent could not restore login", err)
	}
	if _, err = f.store.Authenticate(ctx, security.Hash(session.AccessToken)); err != nil {
		t.Fatal(err)
	}
	// The ordering guarantee must survive removal of transient auth receipts.
	if _, err = f.store.DB.Exec(`DELETE FROM identity_completion_receipts WHERE transaction_id=$1`, fresh); err != nil {
		t.Fatal(err)
	}
	applyApple(t, o, appleEvent("late-revocation", "consent-revoked", now.Add(time.Second)))
	if _, err = f.store.Authenticate(ctx, security.Hash(session.AccessToken)); err != nil {
		t.Fatal("late event revoked newer login", err)
	}
	applyApple(t, o, appleEvent("deleted", "account-deleted", now.Add(2*time.Second)))
	if _, err = o.Complete(ctx, fresh, binding, "unused", "Unused"); err == nil {
		t.Fatal("deleted-account receipt replayed")
	}
	fresh, binding = seedAppleFlow(t, o, "apple-subject", time.Now())
	if _, err = o.Complete(ctx, fresh, binding, "unused", "Unused"); err == nil {
		t.Fatal("deleted Apple identity restored")
	}
}
func TestAppleUnknownSubjectAndManualUnlink(t *testing.T) {
	f, o := appleFixture(t)
	ctx := context.Background()
	now := time.Now().Add(-time.Minute)
	n := appleEvent("unknown", "consent-revoked", now)
	n.Subject = "new-subject"
	applyApple(t, o, n)
	id, binding := seedAppleFlow(t, o, n.Subject, now.Add(-time.Second))
	if _, err := o.Complete(ctx, id, binding, "new-user", "New User"); err == nil {
		t.Fatal("pre-account revocation bypassed")
	}
	if _, err := f.store.DB.Exec(`UPDATE account_external_identities SET revoked_at=clock_timestamp(),revision=revision+1 WHERE id='apple-link'`); err != nil {
		t.Fatal(err)
	}
	applyApple(t, o, appleEvent("manual", "consent-revoked", now))
	id, binding = seedAppleFlow(t, o, "apple-subject", time.Now())
	if _, err := o.Complete(ctx, id, binding, "unused", "Unused"); err == nil {
		t.Fatal("notification undid manual unlink")
	}
}

func TestAppleNotificationHTTPBoundary(t *testing.T) {
	f, o := appleFixture(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	o.Providers["apple"].AppleNotifications = oidclogin.NewAppleNotificationVerifier(&oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, []string{"tv.getportico.ios"})
	handler := (&httpapi.Handler{OIDC: o}).Routes()
	enc := base64.RawURLEncoding.EncodeToString
	claims, _ := json.Marshal(map[string]any{"iss": oidclogin.AppleIssuer, "aud": "tv.getportico.ios", "iat": time.Now().Unix(), "jti": "http-event", "events": map[string]any{"type": "consent-revoked", "sub": "apple-subject", "event_time": time.Now().Add(-time.Second).Unix()}})
	input := enc([]byte(`{"alg":"RS256","kid":"fixture"}`)) + "." + enc(claims)
	sum := sha256.Sum256([]byte(input))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	body, _ := json.Marshal(map[string]string{"payload": input + "." + enc(sig)})
	cases := []struct {
		body, kind string
		want       int
	}{{string(body), "application/json", 200}, {string(body), "application/json; charset=utf-8", 200}, {`{"payload":"forged"}`, "application/json", 401}, {`{}`, "application/json", 401}, {string(body) + `{}`, "application/json", 400}, {string(body), "text/plain", 415}, {`{"payload":"` + strings.Repeat("a", 25<<10) + `"}`, "application/json", 400}}
	for _, tc := range cases {
		r := httptest.NewRequest("POST", "/v1/identity/apple/notifications", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.kind)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("status %d want %d: %s", w.Code, tc.want, w.Body.String())
		}
	}
	var count int
	if err = f.store.DB.QueryRow(`SELECT count(*) FROM apple_account_notifications`).Scan(&count); err != nil || count != 1 {
		t.Fatal("unsigned request mutated store", err, count)
	}
	o.Providers["apple"].AppleNotifications = nil
	r := httptest.NewRequest("POST", "/v1/identity/apple/notifications", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	(&httpapi.Handler{OIDC: o}).Routes().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("unconfigured endpoint registered", w.Code)
	}
}
