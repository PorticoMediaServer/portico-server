package postgres_test

import (
	"context"
	"encoding/base32"
	"errors"
	"reflect"
	"testing"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/sessions"
)

// Compare instants rather than driver-specific time.Location pointers.
func sameSecurityResult(a, b identity.SecurityResult) bool {
	if (a.ExpiresAt == nil) != (b.ExpiresAt == nil) {
		return false
	}
	if a.ExpiresAt != nil && !a.ExpiresAt.Equal(*b.ExpiresAt) {
		return false
	}
	a.ExpiresAt, b.ExpiresAt = nil, nil
	return reflect.DeepEqual(a, b)
}

// Reproduce both leaked payloads, without any mail/provider infrastructure.
func sensitiveReceipt(t *testing.T, s *postgres.AccountSecurity, purpose string) (identity.Session, postgres.SecurityAction, identity.SecurityResult) {
	t.Helper()
	ctx := context.Background()
	a := verifiedAccount(t, s, "receiptowner")
	factor := ""
	if purpose == "recovery_codes" {
		secret, err := identity.NewTOTPSecret()
		if err != nil {
			t.Fatal(err)
		}
		sealed, err := s.Keys.Seal(secret, "totp:"+a.Account.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Store.DB.Exec(`INSERT INTO account_mfa(account_id,secret) VALUES($1,$2)`, a.Account.ID, sealed); err != nil {
			t.Fatal(err)
		}
		key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
		if err != nil {
			t.Fatal(err)
		}
		var now time.Time
		if err = s.Store.DB.QueryRow(`SELECT clock_timestamp()`).Scan(&now); err != nil {
			t.Fatal(err)
		}
		factor = identity.HOTP(key, uint64(now.Unix()/30), 6)
	}
	intent := identity.SecurityIntent{Purpose: purpose, OperationID: security.RandomID("")}
	proof, err := s.PasswordProof(ctx, a.AccessToken, intent, "StrongPass9", factor)
	if err != nil {
		t.Fatal(err)
	}
	q := postgres.SecurityAction{SecurityIntent: intent, Proof: proof.Proof}
	first, err := s.Mutate(ctx, a.AccessToken, q)
	if err != nil {
		t.Fatal(err)
	}
	if purpose == "mfa_enroll" && first.Secret == "" || purpose == "recovery_codes" && len(first.RecoveryCodes) != 10 {
		t.Fatal("fixture has no sensitive response")
	}
	return a, q, first
}
func TestSensitiveReceiptsRequireCurrentAuthority(t *testing.T) {
	for _, purpose := range []string{"mfa_enroll", "recovery_codes"} {
		for _, invalidation := range []string{"logout_all", "family_logout", "refresh_rotation", "expired", "superseded"} {
			t.Run(purpose+"/"+invalidation, func(t *testing.T) {
				s := accountSecurityFixture(t)
				ctx := context.Background()
				a, q, first := sensitiveReceipt(t, s, purpose)
				again, err := s.Mutate(ctx, a.AccessToken, q)
				if err != nil || !sameSecurityResult(first, again) {
					t.Fatalf("exact active retry: %v", err)
				}
				switch invalidation {
				case "logout_all":
					err = s.LogoutAll(ctx, a.AccessToken)
				case "family_logout":
					err = s.Store.RevokeRefresh(ctx, security.Hash(a.RefreshToken), "native")
				case "refresh_rotation":
					_, err = sessions.New(s.Store, s.Keys, 15*time.Minute).Refresh(ctx, a.RefreshToken, refreshOne, "native")
				case "expired":
					_, err = s.Store.DB.Exec(`UPDATE account_security_receipts SET expires_at=clock_timestamp()-interval '1 second' WHERE account_id=$1`, a.Account.ID)
				case "superseded":
					i := identity.SecurityIntent{Purpose: purpose, OperationID: security.RandomID("")}
					factor := ""
					if purpose == "recovery_codes" {
						factor = first.RecoveryCodes[0]
					}
					var proof identity.RecentProof
					proof, err = s.PasswordProof(ctx, a.AccessToken, i, "StrongPass9", factor)
					if err == nil {
						_, err = s.Mutate(ctx, a.AccessToken, postgres.SecurityAction{SecurityIntent: i, Proof: proof.Proof})
					}
				}
				if err != nil {
					t.Fatal("invalidation failed", err)
				}
				leaked, err := s.Mutate(ctx, a.AccessToken, q)
				if err == nil || leaked.Secret != "" || len(leaked.RecoveryCodes) != 0 {
					t.Fatalf("stale receipt disclosed material: err=%v", err)
				}
				if !errors.Is(err, identity.ErrCredentials) && !errors.Is(err, identity.ErrConflict) {
					t.Fatalf("unexpected denial: %v", err)
				}
			})
		}
	}
}
func TestTerminalMFAReceiptOnlySurvivesItsOwnRevocation(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a, _, enrollment := sensitiveReceipt(t, s, "mfa_enroll")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err = s.Store.DB.QueryRow(`SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	code, op := identity.HOTP(key, uint64(now.Unix()/30), 6), security.RandomID("")
	first, err := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, code, op)
	if err != nil || !first.SignOut || len(first.RecoveryCodes) != 10 {
		t.Fatal("confirmation failed", err)
	}
	retry, err := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, code, op)
	if err != nil || !sameSecurityResult(first, retry) {
		t.Fatal("terminal exact retry lost", err)
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(a.AccessToken)); err == nil {
		t.Fatal("self-revoked access still active")
	}
	// A genuinely later logout-all must invalidate the terminal exception as well.
	if err = s.Store.LogoutAll(ctx, a.Account.ID); err != nil {
		t.Fatal(err)
	}
	leaked, err := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, code, op)
	if !errors.Is(err, identity.ErrCredentials) || len(leaked.RecoveryCodes) != 0 {
		t.Fatal("terminal receipt survived later authority change", err)
	}
}
func TestLegacyUnboundReceiptFailsClosed(t *testing.T) {
	s := accountSecurityFixture(t)
	a, q, _ := sensitiveReceipt(t, s, "mfa_enroll")
	if _, err := s.Store.DB.Exec(`UPDATE account_security_receipts SET account_epoch=NULL,family_generation=NULL,result_epoch=NULL WHERE account_id=$1`, a.Account.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	leaked, err := s.Mutate(context.Background(), a.AccessToken, q)
	if !errors.Is(err, identity.ErrCredentials) || leaked.Secret != "" {
		t.Fatal("migration invented authority for old receipt", err)
	}
}
