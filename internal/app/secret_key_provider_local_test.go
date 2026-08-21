//go:build !darwin && !windows

package app

import (
	"path/filepath"
	"testing"
)

func TestNewSecretKeyProviderUsesPrivateLocalFallback(t *testing.T) {
	root := t.TempDir()
	provider := NewSecretKeyProvider(root)
	if got := SecretProviderKindOf(provider); got != SecretProviderKindLocal {
		t.Fatalf("provider kind = %q, want %q", got, SecretProviderKindLocal)
	}
	local, ok := provider.(*LocalSecretKeyProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *LocalSecretKeyProvider", provider)
	}
	if local.path != filepath.Join(root, "keys", "hosted-authority.key") {
		t.Fatalf("local key path = %q", local.path)
	}
}
