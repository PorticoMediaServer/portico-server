package access

import (
	"reflect"
	"testing"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

func TestCredentialCeilingsOnlyNarrowRequestedScopesAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	policy := operations.DefaultSettings().Integrations()
	policy.CredentialMaxDays = 30
	policy.MemberTokenScope = "read"
	requested := []string{"read", "playback", "library", "admin"}
	scopes, expiry, err := credentialCeilings(policy, identity.CredentialKindToken, requested, time.Time{}, now)
	if err != nil || !reflect.DeepEqual(scopes, []string{"read"}) || !expiry.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("restricted token: %v %v %v", scopes, expiry, err)
	}
	policy.MemberTokenScope = "play"
	scopes, expiry, err = credentialCeilings(policy, identity.CredentialKindToken, []string{"playback", "admin"}, now.Add(24*time.Hour), now)
	if err != nil || !reflect.DeepEqual(scopes, []string{"playback"}) || !expiry.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("shorter request or absent read scope raised: %v %v %v", scopes, expiry, err)
	}
	if _, _, err = credentialCeilings(policy, identity.CredentialKindToken, []string{"admin"}, time.Time{}, now); err == nil {
		t.Fatal("empty scope intersection was issued")
	}
	// The member ceiling is for personal tokens. API keys retain their requested
	// scopes and remain limited by their actual owner in the ordinary gate.
	scopes, expiry, err = credentialCeilings(policy, identity.CredentialKindKey, requested, now.Add(365*24*time.Hour), now)
	if err != nil || !reflect.DeepEqual(scopes, requested) || !expiry.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("key: %v %v %v", scopes, expiry, err)
	}
	policy.CredentialMaxDays = 0
	policy.MemberTokenScope = "same"
	scopes, expiry, err = credentialCeilings(policy, identity.CredentialKindToken, requested, time.Time{}, now)
	if err != nil || !reflect.DeepEqual(scopes, requested) || !expiry.IsZero() {
		t.Fatalf("unlimited/same changed request: %v %v %v", scopes, expiry, err)
	}
}
