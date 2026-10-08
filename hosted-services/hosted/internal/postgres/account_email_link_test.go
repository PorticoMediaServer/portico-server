package postgres_test

import (
	"context"
	"testing"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

// A33: the registration link opened on another device (no browser binding)
// proves the address; the device that started the flow then finishes without
// the code.
func TestRegistrationLinkConfirmsFromAnotherDevice(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	binding := security.RandomID("")
	r, err := s.CreateRegistration(ctx, binding, postgres.RegistrationInput{Username: "linkowner", DisplayName: "Link", Password: "StrongPass9", ContactAddress: "linkowner@example.test", SessionMode: "native", RequestID: security.RandomID("")})
	if err != nil {
		t.Fatal(err)
	}
	code := mailPayload(t, s, mailoutbox.Registration, r.ID).Code
	if _, err = s.ConfirmEmailLink(ctx, "registration", r.ID, "000000"); err != identity.ErrChallenge {
		t.Fatal("wrong code accepted", err)
	}
	got, err := s.ConfirmEmailLink(ctx, "registration", r.ID, code)
	if err != nil || got.Status != "confirmed" {
		t.Fatal(got, err)
	}
	if again, err := s.ConfirmEmailLink(ctx, "registration", r.ID, code); err != nil || again.Status != "already_confirmed" {
		t.Fatal("second click", again, err)
	}
	view, err := s.RegistrationView(ctx, r.ID, binding)
	if err != nil || !view.ContactVerified {
		t.Fatal("original device can't see the confirmation", view, err)
	}
	if _, err = s.VerifyRegistration(ctx, r.ID, binding, "", "", ""); err != nil {
		t.Fatal("original device could not finish without the code", err)
	}
}

// Every verification link round-trips through the token the mail carries.
func TestVerificationTokenRoundTrip(t *testing.T) {
	token := mailoutbox.VerificationToken(mailoutbox.EmailChange, "chal_1", "123456")
	kind, resource, code, err := mailoutbox.ParseVerificationToken(token)
	if err != nil || kind != mailoutbox.EmailChange || resource != "chal_1" || code != "123456" {
		t.Fatal(kind, resource, code, err)
	}
	if _, _, _, err = mailoutbox.ParseVerificationToken(mailoutbox.VerificationToken(mailoutbox.PasswordReset, "x", "y")); err == nil {
		t.Fatal("reset token accepted as a verification link")
	}
}

// A33: the email-change link completes the change from any device, with the
// same effects as the in-session confirmation (new primary, sessions revoked).
func TestEmailChangeLinkCompletesFromAnotherDevice(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "changeowner")
	change := securityAction(t, s, a, "email_change", "moved@example.test")
	code := mailPayload(t, s, mailoutbox.EmailChange, change.ChallengeID).Code
	got, err := s.ConfirmEmailLink(ctx, "email_change", change.ChallengeID, code)
	if err != nil || got.Status != "email_changed" {
		t.Fatal(got, err)
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(a.AccessToken)); err == nil {
		t.Fatal("the change did not revoke existing sessions")
	}
	login, err := s.PasswordLoginBound(ctx, "changeowner", "StrongPass9", "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
	if err != nil || login.Session == nil {
		t.Fatal(err)
	}
	state, err := s.State(ctx, login.Session.AccessToken)
	if err != nil || state.ContactAddress != "moved@example.test" {
		t.Fatal("new address not primary", state.ContactAddress, err)
	}
}
