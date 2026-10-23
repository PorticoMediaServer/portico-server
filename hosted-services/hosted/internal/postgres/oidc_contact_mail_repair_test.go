package postgres_test

import (
	"bytes"
	"context"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"sync"
	"testing"
	"time"
)

// The assertion is an explicitly disposable verified-provider repository fixture,
// not evidence of a real SSO login. All sending/eligibility/completion consumers run.
func TestOIDCSignupMailRepairAuthorityRetriesAndCompletion(t *testing.T) {
	for _, scenario := range []string{"signup-retry", "linked", "revoked", "deleted", "stale-binding", "stale-policy", "security-flow", "concurrent-completion"} {
		t.Run(scenario, func(t *testing.T) {
			f := currentFamilySetup(t)
			ctx := context.Background()
			keys := &security.Keys{SealKey: bytes.Repeat([]byte{3}, 32)}
			o := &postgres.OIDCLogin{Store: f.store, Keys: keys, Security: &postgres.AccountSecurity{Store: f.store, Keys: keys, Identity: identity.New(f.store), Lifetime: 15 * time.Minute, MailEnabled: true}, Lifetime: 15 * time.Minute, Providers: map[string]*oidclogin.Provider{"google": {ID: "google", Issuer: "https://accounts.google.com", Audience: "fixture-client", Callback: "https://web.getportico.tv/auth/sso/google/callback"}}}
			if e := o.Configure(ctx); e != nil {
				t.Fatal(e)
			}
			binding := security.RandomID("")
			var v postgres.OIDCView
			var e error
			if scenario == "security-flow" {
				account := verifiedAccount(t, o.Security, "contact-linker")
				v, _, _, e = o.StartFlow(ctx, "google", binding, account.AccessToken, postgres.OIDCStartInput{Mode: "link", SessionMode: "native", Completion: postgres.NativeCompletionCode, RequestID: security.RandomID(""), Intent: &identity.SecurityIntent{Purpose: "provider_link", Target: "google", OperationID: security.RandomID("")}})
			} else {
				v, _, e = o.Start(ctx, "google", binding)
			}
			if e != nil {
				t.Fatal(e)
			}
			_, e = f.store.DB.Exec(`INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,email_verified,transaction_revision,lease_id) SELECT id,'https://accounts.google.com','mail-subject','fixture-client',clock_timestamp(),clock_timestamp(),nonce_hash,false,revision,'fixture-lease' FROM identity_transactions WHERE id=$1`, v.ID)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.store.DB.Exec(`UPDATE identity_transactions SET status='verified' WHERE id=$1`, v.ID); e != nil {
				t.Fatal(e)
			}
			switch scenario {
			case "linked", "revoked":
				if _, e = f.store.DB.Exec(`INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES('linked-mail-identity','strict-owner','google','https://accounts.google.com','mail-subject',clock_timestamp())`); e != nil {
					t.Fatal(e)
				}
				if scenario == "revoked" {
					if _, e = f.store.DB.Exec(`UPDATE account_external_identities SET revoked_at=clock_timestamp(),revision=revision+1 WHERE id='linked-mail-identity'`); e != nil {
						t.Fatal(e)
					}
				}
			case "deleted":
				_, e = f.store.DB.Exec(`INSERT INTO deleted_identity_markers(marker_key_id,subject_digest,deleted_at,expires_at) SELECT marker_key_id,$2,clock_timestamp(),clock_timestamp()+interval '10 minutes' FROM identity_transactions WHERE id=$1`, v.ID, identity.SecretDigest(keys.SealKey, "deleted-identity", "https://accounts.google.com", "mail-subject"))
				if e != nil {
					t.Fatal(e)
				}
			case "stale-binding":
				if _, e = f.store.DB.Exec(`UPDATE identity_browser_bindings SET generation=generation+1 WHERE binding_hash=$1`, security.Hash(binding)); e != nil {
					t.Fatal(e)
				}
			case "stale-policy":
				if _, e = f.store.DB.Exec(`UPDATE identity_protocol_policy SET revision=revision+1`); e != nil {
					t.Fatal(e)
				}
			}
			challenge, e := o.SendContact(ctx, v.ID, binding, "signup@example.test", "fixture-contact-request-123")
			allowed := scenario == "signup-retry" || scenario == "concurrent-completion"
			if (e == nil) != allowed {
				t.Fatalf("signup sending predicate %s: %v", scenario, e)
			}
			var count int
			if e = f.store.DB.QueryRow(`SELECT count(*) FROM account_mail_outbox WHERE kind='oidc_contact'`).Scan(&count); e != nil {
				t.Fatal(e)
			}
			if !allowed {
				if count != 0 {
					t.Fatal("refused flow enqueued")
				}
				return
			}
			again, e := o.SendContact(ctx, v.ID, binding, "signup@example.test", "fixture-contact-request-123")
			if e != nil || again.ChallengeID != challenge.ChallengeID {
				t.Fatal("legitimate retry changed challenge", e)
			}
			m := &postgres.AccountMail{Store: f.store, Keys: keys}
			delivery, e := m.ClaimMail(ctx)
			if e != nil || delivery == nil {
				t.Fatal("actual mail claim", e)
			}
			if ok, e := m.MailEligible(ctx, *delivery); e != nil || !ok {
				t.Fatal("genuine signup ineligible", e)
			}
			if _, e = o.VerifyContact(ctx, v.ID, binding, challenge.ChallengeID, delivery.Payload.Code); e != nil {
				t.Fatal("actual contact verification", e)
			}
			if scenario == "concurrent-completion" {
				start := make(chan struct{})
				var wg sync.WaitGroup
				wg.Add(2)
				var completedErr error
				go func() {
					defer wg.Done()
					<-start
					_, completedErr = o.Complete(ctx, v.ID, binding, "signup-fixture", "Fixture")
				}()
				go func() {
					defer wg.Done()
					<-start
					_, _ = o.SendContact(ctx, v.ID, binding, "signup@example.test", "fixture-contact-request-123")
				}()
				close(start)
				wg.Wait()
				if completedErr != nil {
					t.Fatal("actual completion", completedErr)
				}
			} else {
				if _, e = o.Complete(ctx, v.ID, binding, "signup-fixture", "Fixture"); e != nil {
					t.Fatal(e)
				}
			}
			if ok, e := m.MailEligible(ctx, *delivery); e != nil || ok {
				t.Fatal("existing account contact mail remained eligible", e)
			}
			if _, e = o.SendContact(ctx, v.ID, binding, "signup@example.test", "fixture-contact-request-123"); e == nil {
				t.Fatal("existing-account retry readback accepted")
			}
		})
	}
}
