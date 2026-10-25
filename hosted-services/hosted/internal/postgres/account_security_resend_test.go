package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/security"
)

func TestEmailChangeResendPreservesOriginalAuthorityBudgetAndDeadline(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "resendowner")
	other := verifiedAccount(t, s, "resendother")
	change := securityAction(t, s, a, "email_change", "new@example.test")
	old := mailPayload(t, s, mailoutbox.EmailChange, change.ChallengeID).Code
	if change.MaskedAddress != "n***@example.test" || change.ResendsRemaining == nil || *change.ResendsRemaining != 5 {
		t.Fatal("initial challenge lacks its checked masked destination and resend budget")
	}
	// A new family of this same account cannot rotate the original family's code.
	second, err := s.PasswordLoginBound(ctx, "resendowner", "StrongPass9", "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
	if err != nil || second.Session == nil {
		t.Fatal("could not create second family", err)
	}
	for _, token := range []string{other.AccessToken, second.Session.AccessToken} {
		if _, err = s.ResendEmailChange(ctx, token, change.ChallengeID, security.RandomID("")); err == nil {
			t.Fatal("foreign account or family rotated the original challenge")
		}
	}
	if _, err = s.Store.DB.Exec(`UPDATE account_email_challenges SET attempts=2 WHERE id=$1`, change.ChallengeID); err != nil {
		t.Fatal(err)
	}
	op := security.RandomID("")
	first, err := s.ResendEmailChange(ctx, a.AccessToken, change.ChallengeID, op)
	if err != nil || first.ChallengeID != change.ChallengeID || first.ExpiresAt == nil || !first.ExpiresAt.Equal(*change.ExpiresAt) || first.MaskedAddress != change.MaskedAddress || first.ResendsRemaining == nil || *first.ResendsRemaining != 4 {
		t.Fatal("resend changed the original challenge or deadline", err)
	}
	fresh := mailPayload(t, s, mailoutbox.EmailChange, change.ChallengeID).Code
	if fresh == old {
		t.Fatal("resend kept the previous code")
	}
	retry, err := s.ResendEmailChange(ctx, a.AccessToken, change.ChallengeID, op)
	if err != nil || retry.ResendsRemaining == nil || *retry.ResendsRemaining != 4 {
		t.Fatal("exact retry did not recover the original receipt", err)
	}
	var revision, attempts, mails int
	var expires time.Time
	if err = s.Store.DB.QueryRow(`SELECT revision,attempts,expires_at,(SELECT count(*) FROM account_mail_outbox WHERE resource_id=$1) FROM account_email_challenges WHERE id=$1`, change.ChallengeID).Scan(&revision, &attempts, &expires, &mails); err != nil || revision != 2 || attempts != 2 || mails != 2 || !expires.Equal(*change.ExpiresAt) {
		t.Fatal("retry duplicated mail or resend reset the guessing budget", err)
	}
	if _, err = s.ConfirmEmailChange(ctx, a.AccessToken, change.ChallengeID, old, security.RandomID("")); !errors.Is(err, identity.ErrChallenge) {
		t.Fatal("old code remained usable", err)
	}
	for n := 0; n < 4; n++ {
		if _, err = s.ResendEmailChange(ctx, a.AccessToken, change.ChallengeID, security.RandomID("")); err != nil {
			t.Fatal("bounded resend refused early", err)
		}
	}
	if _, err = s.ResendEmailChange(ctx, a.AccessToken, change.ChallengeID, security.RandomID("")); !errors.Is(err, identity.ErrRateLimited) {
		t.Fatal("sixth resend was admitted", err)
	}
	// Receipt replay is still exact and consumes no extra budget after exhaustion.
	if _, err = s.ResendEmailChange(ctx, a.AccessToken, change.ChallengeID, op); err != nil {
		t.Fatal("exact receipt was hidden by exhausted resend budget", err)
	}
	latest := mailPayload(t, s, mailoutbox.EmailChange, change.ChallengeID).Code
	if _, err = s.ConfirmEmailChange(ctx, a.AccessToken, change.ChallengeID, latest, security.RandomID("")); err != nil {
		t.Fatal("last sent code could not complete the original change", err)
	}
	if _, err = s.ResendEmailChange(ctx, a.AccessToken, change.ChallengeID, op); err == nil {
		t.Fatal("resend receipt survived the terminal authority change")
	}
}

func TestEmailChangeResendRefusesExpiredExhaustedAndSupersededChallenges(t *testing.T) {
	for _, update := range []string{"expires_at=clock_timestamp()-interval '1 second'", "attempts=5", "superseded_at=clock_timestamp()"} {
		t.Run(update, func(t *testing.T) {
			s := accountSecurityFixture(t)
			a := verifiedAccount(t, s, "resendclosed")
			change := securityAction(t, s, a, "email_change", "new@example.test")
			if _, err := s.Store.DB.Exec(`UPDATE account_email_challenges SET `+update+` WHERE id=$1`, change.ChallengeID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ResendEmailChange(context.Background(), a.AccessToken, change.ChallengeID, security.RandomID("")); !errors.Is(err, identity.ErrChallenge) {
				t.Fatal("closed challenge was rotated", err)
			}
		})
	}
}
