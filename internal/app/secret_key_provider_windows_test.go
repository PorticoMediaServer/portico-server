//go:build windows

package app

import "testing"

func TestNewSecretKeyProviderUsesCurrentUserDPAPI(t *testing.T) {
	provider := NewSecretKeyProvider(t.TempDir())
	if got := SecretProviderKindOf(provider); got != SecretProviderKindDPAPI {
		t.Fatalf("provider kind = %q, want %q", got, SecretProviderKindDPAPI)
	}
	if _, ok := provider.(*dpapiSecretKeyProvider); !ok {
		t.Fatalf("provider type = %T, want *dpapiSecretKeyProvider", provider)
	}
}
