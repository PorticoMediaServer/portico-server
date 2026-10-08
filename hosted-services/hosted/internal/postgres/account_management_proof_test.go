package postgres_test

import (
	"context"
	"errors"
	"testing"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

// A80: account management needs a signed-in session only; credential changes
// (password, email, second factor) still need a fresh proof.
func TestAccountManagementNeedsNoProofButCredentialChangesDo(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "manager")
	preview, err := s.Mutate(ctx, a.AccessToken, postgres.SecurityAction{SecurityIntent: identity.SecurityIntent{Purpose: "account_deletion_preview", OperationID: security.RandomID("")}})
	if err != nil || preview.DeletionPreview == nil {
		t.Fatalf("deletion preview without proof: %v", err)
	}
	for _, purpose := range []string{"email_change", "password_set_or_change", "mfa_disable"} {
		i := identity.SecurityIntent{Purpose: purpose, Target: "new@example.test", OperationID: security.RandomID("")}
		if _, err = s.Mutate(ctx, a.AccessToken, postgres.SecurityAction{SecurityIntent: i, NewPassword: "another password"}); !errors.Is(err, identity.ErrInvalid) && !errors.Is(err, identity.ErrRecentAuth) {
			t.Fatalf("%s without proof: %v", purpose, err)
		}
	}
	for _, purpose := range []string{"server_remove", "server_transfer", "provider_link", "provider_unlink", "account_deletion_preview", "account_export"} {
		if (identity.SecurityIntent{Purpose: purpose}).NeedsProof() {
			t.Fatalf("%s still needs a proof", purpose)
		}
	}
	if err = s.RemoveServer(ctx, a.AccessToken, "srv_unknown", postgres.ServerRemoval{OperationID: security.RandomID(""), ClaimGeneration: 1, CredentialGeneration: 1}); errors.Is(err, identity.ErrRecentAuth) {
		t.Fatal("server removal still asks for a proof")
	}
}
