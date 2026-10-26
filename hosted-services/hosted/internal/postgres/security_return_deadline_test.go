package postgres

import (
	"context"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/serverreturn"
	"testing"
	"time"
)

// Held-clock policy vectors for the predicate used by both proof issuance and
// accepting action. These are source-level unit vectors, not provider/runtime
// qualification. A retained oversized proof cannot widen any original bound.
func TestSecurityReturnAcceptingDeadlineUsesOriginalServerAndConfirmation(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	for _, scenario := range []struct {
		name                    string
		authority, confirmation time.Duration
	}{
		{"server authority ends first", 50 * time.Millisecond, time.Minute},
		{"finite confirmation ends first", time.Minute, 50 * time.Millisecond},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			original := serverreturn.SecurityContext{ExpiresAt: now.Add(5 * time.Minute), AuthorityExpiresAt: now.Add(scenario.authority)}
			deadline := securityReturnDeadline(original, now.Add(scenario.confirmation))
			before := now.Add(49 * time.Millisecond)
			if !deadline.After(before) {
				t.Fatal("live original command refused before its earliest bound")
			}
			for _, after := range []time.Time{now.Add(50 * time.Millisecond), now.Add(51 * time.Millisecond), now.Add(2 * time.Minute)} {
				if deadline.After(after) {
					t.Fatal("accepting action trusted an oversized proof after original authority/confirmation expiry")
				}
			}
			if deadline.After(original.ExpiresAt) || deadline.After(original.AuthorityExpiresAt) || deadline.After(now.Add(scenario.confirmation)) {
				t.Fatal("proof issuance renewed original authority")
			}
		})
	}
	if !securityReturnDeadline(serverreturn.SecurityContext{ExpiresAt: now.Add(time.Minute)}, now.Add(time.Minute)).IsZero() {
		t.Fatal("missing authoritative server expiry gained a fallback")
	}
}

func TestOrdinaryProviderProofDoesNotAcquireSecurityReturnDeadline(t *testing.T) {
	proof := identity.RecentProof{Proof: "ordinary-proof-fixture", ExpiresAt: time.Date(2026, 10, 26, 12, 5, 0, 0, time.UTC), AccountID: "account", FamilyID: "family"}
	before := proof
	// No transaction/context is consulted for an ordinary provider proof.
	if e := new(OIDCLogin).boundSecurityReturnProofTx(context.Background(), nil, "ordinary", &proof, false); e != nil {
		t.Fatal(e)
	}
	if proof != before {
		t.Fatal("ordinary provider proof was changed")
	}
}
