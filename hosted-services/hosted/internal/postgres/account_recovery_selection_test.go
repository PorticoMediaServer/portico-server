package postgres_test

import (
	"context"
	"testing"

	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

func recoveryPrimaryContact(t *testing.T, s *postgres.AccountSecurity, account, address string) {
	t.Helper()
	result, err := s.Store.DB.Exec(`UPDATE account_contact_emails SET address=$2 WHERE account_id=$1 AND is_primary`, account, address)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("primary contact update: %d %v", n, err)
	}
}

func TestRecoverySharedContactRequiresExactUsername(t *testing.T) {
	s, ctx := accountSecurityFixture(t), context.Background()
	a, b, passwordless := verifiedAccount(t, s, "recoverfirst"), verifiedAccount(t, s, "recoversecond"), verifiedAccount(t, s, "recoverpasswordless")
	const address = "shared-recovery@example.test"
	for _, account := range []string{a.Account.ID, b.Account.ID, passwordless.Account.ID} {
		recoveryPrimaryContact(t, s, account, address)
	}
	if _, err := s.Store.DB.Exec(`UPDATE accounts SET password_hash=NULL WHERE id=$1`, passwordless.Account.ID); err != nil {
		t.Fatal(err)
	}
	for _, request := range []postgres.RecoveryRequest{
		{Username: "  RECOVERFIRST  ", ContactAddress: address},
		{Username: "notthataccount", ContactAddress: address},
		{Username: "recoversecond", ContactAddress: "wrong-contact@example.test"},
		{Username: "recoverpasswordless", ContactAddress: address},
		{Username: "recoversecond", ContactAddress: address},
	} {
		request.RequestID = security.RandomID("")
		if err := s.RequestRecovery(ctx, request); err != nil {
			t.Fatalf("generic recovery outcome differed: %v", err)
		}
	}
	for account, expected := range map[string]int{a.Account.ID: 1, b.Account.ID: 1, passwordless.Account.ID: 0} {
		var count int
		if err := s.Store.DB.QueryRow(`SELECT count(*) FROM account_email_challenges WHERE account_id=$1 AND purpose='password_reset'`, account).Scan(&count); err != nil || count != expected {
			t.Fatalf("wrong recovery account selected: got %d want %d (%v)", count, expected, err)
		}
	}
}

func TestLegacyAmbiguousRecoveryPreservesOriginalProof(t *testing.T) {
	s, ctx := accountSecurityFixture(t), context.Background()
	a := verifiedAccount(t, s, "legacyfirst")
	const address = "legacy-recovery@example.test"
	recoveryPrimaryContact(t, s, a.Account.ID, address)
	original := postgres.RecoveryRequest{ContactAddress: address, RequestID: security.RandomID("")}
	if err := s.RequestRecovery(ctx, original); err != nil {
		t.Fatal(err)
	}
	var challenge string
	if err := s.Store.DB.QueryRow(`SELECT id FROM account_email_challenges WHERE account_id=$1 AND purpose='password_reset'`, a.Account.ID).Scan(&challenge); err != nil {
		t.Fatal(err)
	}
	token := mailPayload(t, s, mailoutbox.PasswordReset, challenge).Code
	b := verifiedAccount(t, s, "legacysecond")
	recoveryPrimaryContact(t, s, b.Account.ID, address)
	for _, request := range []postgres.RecoveryRequest{original, {ContactAddress: address, RequestID: security.RandomID("")}} {
		if err := s.RequestRecovery(ctx, request); err != nil {
			t.Fatalf("ambiguous recovery outcome differed: %v", err)
		}
	}
	var challenges, superseded, mails int
	if err := s.Store.DB.QueryRow(`SELECT count(*),count(*) FILTER(WHERE superseded_at IS NOT NULL) FROM account_email_challenges WHERE purpose='password_reset'`).Scan(&challenges, &superseded); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.DB.QueryRow(`SELECT count(*) FROM account_mail_outbox WHERE kind='password_reset'`).Scan(&mails); err != nil {
		t.Fatal(err)
	}
	if challenges != 1 || superseded != 0 || mails != 1 {
		t.Fatalf("ambiguous request mutated original recovery: challenges=%d superseded=%d mails=%d", challenges, superseded, mails)
	}
	if err := s.ResetPassword(ctx, token, "NewStrongPass8", ""); err != nil {
		t.Fatalf("ambiguity invalidated the original account-bound proof: %v", err)
	}
}

func TestLegacyRecoveryCountsOnlyEligiblePasswordAccounts(t *testing.T) {
	s, ctx := accountSecurityFixture(t), context.Background()
	passwordless := verifiedAccount(t, s, "legacywithoutpassword")
	const address = "eligible-recovery@example.test"
	recoveryPrimaryContact(t, s, passwordless.Account.ID, address)
	if _, err := s.Store.DB.Exec(`UPDATE accounts SET password_hash=NULL WHERE id=$1`, passwordless.Account.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestRecovery(ctx, postgres.RecoveryRequest{ContactAddress: address, RequestID: security.RandomID("")}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.Store.DB.QueryRow(`SELECT count(*) FROM account_email_challenges WHERE purpose='password_reset'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("password-less account received a reset: %d %v", count, err)
	}
	eligible := verifiedAccount(t, s, "legacywithpassword")
	recoveryPrimaryContact(t, s, eligible.Account.ID, address)
	if err := s.RequestRecovery(ctx, postgres.RecoveryRequest{ContactAddress: address, RequestID: security.RandomID("")}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.DB.QueryRow(`SELECT count(*) FROM account_email_challenges WHERE account_id=$1 AND purpose='password_reset'`, eligible.Account.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unambiguous legacy password account refused: %d %v", count, err)
	}
}
