package postgres_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

// These are isolated PostgreSQL fixtures, not provider or email delivery tests.
func accountSecurityFixture(t *testing.T) *postgres.AccountSecurity {
	t.Helper()
	store := database(t)
	return &postgres.AccountSecurity{Store: store, Keys: &security.Keys{SealKey: bytes.Repeat([]byte{11}, 32)}, Identity: identity.New(store), Lifetime: 15 * time.Minute, MailEnabled: true}
}
func mailPayload(t *testing.T, s *postgres.AccountSecurity, kind mailoutbox.Kind, resource string) mailoutbox.Payload {
	t.Helper()
	var id string
	var ciphertext []byte
	if err := s.Store.DB.QueryRow(`SELECT id,ciphertext FROM account_mail_outbox WHERE kind=$1 AND resource_id=$2 ORDER BY created_at DESC LIMIT 1`, kind, resource).Scan(&id, &ciphertext); err != nil {
		t.Fatal(err)
	}
	raw, err := s.Keys.Open(ciphertext, "mail:"+id)
	if err != nil {
		t.Fatal(err)
	}
	var p mailoutbox.Payload
	if err = json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}
func verifiedAccount(t *testing.T, s *postgres.AccountSecurity, username string) identity.Session {
	t.Helper()
	ctx := context.Background()
	binding := security.RandomID("")
	r, err := s.CreateRegistration(ctx, binding, postgres.RegistrationInput{Username: username, DisplayName: "Fixture", Password: "StrongPass9", ContactAddress: username + "@example.test", SessionMode: "native", RequestID: security.RandomID("")})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.VerifyRegistration(ctx, r.ID, binding, mailPayload(t, s, mailoutbox.Registration, r.ID).Code, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func securityAction(t *testing.T, s *postgres.AccountSecurity, a identity.Session, purpose, target string) identity.SecurityResult {
	t.Helper()
	ctx := context.Background()
	i := identity.SecurityIntent{Purpose: purpose, Target: target, OperationID: security.RandomID("")}
	proof, err := s.PasswordProof(ctx, a.AccessToken, i, "StrongPass9", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Mutate(ctx, a.AccessToken, postgres.SecurityAction{SecurityIntent: i, Proof: proof.Proof})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestVerifiedRegistrationAtomicRetryAndMailEligibility(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	binding := security.RandomID("")
	q := postgres.RegistrationInput{Username: "pending", DisplayName: "Pending", Password: "StrongPass9", ContactAddress: "pending@example.test", SessionMode: "native", RequestID: security.RandomID("")}
	r, err := s.CreateRegistration(ctx, binding, q)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateRegistration(ctx, binding, q)
	if err != nil || again.ID != r.ID {
		t.Fatalf("registration retry: %v", err)
	}
	var count int
	if err = s.Store.DB.QueryRow(`SELECT count(*) FROM accounts WHERE username='pending'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unverified account created: %v", err)
	}
	mail := &postgres.AccountMail{Store: s.Store, Keys: s.Keys}
	delivery, err := mail.ClaimMail(ctx)
	if err != nil || delivery == nil {
		t.Fatalf("durable mail missing: %v", err)
	}
	if ok, e := mail.MailEligible(ctx, *delivery); e != nil || !ok {
		t.Fatalf("pending verification ineligible: %v", e)
	}
	if _, err = s.VerifyRegistration(ctx, r.ID, security.RandomID(""), delivery.Payload.Code, "", ""); err == nil {
		t.Fatal("different device binding accepted")
	}
	a, err := s.VerifyRegistration(ctx, r.ID, binding, delivery.Payload.Code, "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.VerifyRegistration(ctx, r.ID, binding, delivery.Payload.Code, "", "")
	if err != nil || a.AccessToken != b.AccessToken || a.FamilyID != b.FamilyID {
		t.Fatalf("session retry changed family: %v", err)
	}
	if ok, e := mail.MailEligible(ctx, *delivery); e != nil || ok {
		t.Fatalf("consumed verification still deliverable: %v", e)
	}
	q.Password = "DifferentPass8"
	if _, err = s.CreateRegistration(ctx, binding, q); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("changed request reused: %v", err)
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(a.AccessToken)); err != nil {
		t.Fatal(err)
	}
}
func TestMFAEnrollmentRevocationAndSingleUseRecovery(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "mfaowner")
	enrollment := securityAction(t, s, a, "mfa_enroll", "")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err = s.Store.DB.QueryRow(`SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	code := identity.HOTP(key, uint64(now.Unix()/30), 6)
	op := security.RandomID("")
	enabled, err := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, code, op)
	if err != nil || len(enabled.RecoveryCodes) != 10 || !enabled.SignOut {
		t.Fatalf("enrollment: %v", err)
	}
	retry, err := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, code, op)
	if err != nil || retry.RecoveryCodes[0] != enabled.RecoveryCodes[0] {
		t.Fatalf("lost enrollment response: %v", err)
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(a.AccessToken)); err == nil {
		t.Fatal("pre-MFA session survived")
	}
	login, err := s.PasswordLoginBound(ctx, "mfaowner", "StrongPass9", "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
	if err != nil || login.Challenge == nil || login.Session != nil {
		t.Fatalf("MFA bypass: %v", err)
	}
	c := login.Challenge
	signed, err := s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, enabled.RecoveryCodes[0])
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, enabled.RecoveryCodes[0])
	if err != nil || replay.AccessToken != signed.AccessToken {
		t.Fatalf("MFA receipt retry: %v", err)
	}
	other, err := s.PasswordLoginBound(ctx, "mfaowner", "StrongPass9", "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
	if err != nil || other.Challenge == nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteMFA(ctx, other.Challenge.ChallengeID, other.Challenge.ChallengeToken, enabled.RecoveryCodes[0]); !errors.Is(err, identity.ErrChallenge) {
		t.Fatalf("recovery code reused for different challenge: %v", err)
	}
	if _, err = s.CompleteMFA(ctx, other.Challenge.ChallengeID, other.Challenge.ChallengeToken, enabled.RecoveryCodes[1]); err != nil {
		t.Fatal(err)
	}
}
func TestContactChangeAndRecoveryStayAccountBound(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "emailowner")
	other := verifiedAccount(t, s, "otherowner")
	change := securityAction(t, s, a, "email_change", "new@example.test")
	code := mailPayload(t, s, mailoutbox.EmailChange, change.ChallengeID).Code
	op := security.RandomID("")
	state, err := s.State(ctx, a.AccessToken)
	if err != nil || state.ContactAddress != "emailowner@example.test" {
		t.Fatalf("unverified address replaced contact: %v", err)
	}
	if _, err = s.ConfirmEmailChange(ctx, other.AccessToken, change.ChallengeID, code, op); err == nil {
		t.Fatal("cross-account email proof accepted")
	}
	if _, err = s.ConfirmEmailChange(ctx, a.AccessToken, change.ChallengeID, code, op); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(a.AccessToken)); err == nil {
		t.Fatal("contact change did not revoke old sessions")
	}
	login, err := s.PasswordLoginBound(ctx, "emailowner", "StrongPass9", "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
	if err != nil || login.Session == nil {
		t.Fatal(err)
	}
	if err = s.RequestRecovery(ctx, postgres.RecoveryRequest{Username: "unknown", ContactAddress: "unknown@example.test", RequestID: security.RandomID("")}); err != nil {
		t.Fatalf("unknown recovery outcome differs: %v", err)
	}
	q := postgres.RecoveryRequest{Username: "emailowner", ContactAddress: "new@example.test", RequestID: security.RandomID("")}
	if err = s.RequestRecovery(ctx, q); err != nil {
		t.Fatal(err)
	}
	if err = s.RequestRecovery(ctx, q); err != nil {
		t.Fatal(err)
	}
	var id string
	var count int
	if err = s.Store.DB.QueryRow(`SELECT min(id),count(*) FROM account_email_challenges WHERE purpose='password_reset' AND account_id=$1`, a.Account.ID).Scan(&id, &count); err != nil || count != 1 {
		t.Fatalf("duplicate recovery request: %v", err)
	}
	token := mailPayload(t, s, mailoutbox.PasswordReset, id).Code
	if err = s.ResetPassword(ctx, token, "NewStrongPass8", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.ResetPassword(ctx, token, "NewStrongPass8", ""); err != nil {
		t.Fatalf("identical reset retry: %v", err)
	}
	if err = s.ResetPassword(ctx, token, "AnotherStrong9", ""); !errors.Is(err, identity.ErrChallenge) {
		t.Fatalf("changed reset replay accepted: %v", err)
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(login.Session.AccessToken)); err == nil {
		t.Fatal("password reset left previous session usable")
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(other.AccessToken)); err != nil {
		t.Fatal("unrelated account was revoked")
	}
}
