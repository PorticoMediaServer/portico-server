//go:build darwin

package app

import (
	"errors"
	"strings"
	"testing"
)

func TestNewSecretKeyProviderUsesMacOSKeychain(t *testing.T) {
	provider := NewSecretKeyProvider(t.TempDir())
	if got := SecretProviderKindOf(provider); got != SecretProviderKindKeychain {
		t.Fatalf("provider kind = %q, want %q", got, SecretProviderKindKeychain)
	}
	if _, ok := provider.(*keychainSecretKeyProvider); !ok {
		t.Fatalf("provider type = %T, want *keychainSecretKeyProvider", provider)
	}
}

func TestKeychainProviderUsesStdinAndClassifiesProviderFailures(t *testing.T) {
	provider := newKeychainSecretKeyProvider(t.TempDir())
	var stored string
	var addArgs []string
	provider.run = func(stdin []byte, args ...string) ([]byte, error) {
		if len(args) == 0 {
			t.Fatal("keychain command received no arguments")
		}
		switch args[0] {
		case "find-generic-password":
			if stored == "" {
				return nil, &keychainCommandFailure{missing: true}
			}
			return []byte(stored + "\n"), nil
		case "add-generic-password":
			addArgs = append([]string(nil), args...)
			stored = strings.Split(strings.TrimSpace(string(stdin)), "\n")[0]
			return nil, nil
		default:
			t.Fatalf("unexpected keychain command %q", args[0])
			return nil, nil
		}
	}
	envelope, err := provider.Seal([]byte("authority"))
	if err != nil {
		t.Fatalf("seal through fake Keychain: %v", err)
	}
	if stored == "" || len(addArgs) == 0 || strings.Contains(strings.Join(addArgs, " "), stored) {
		t.Fatalf("wrapping key was not confined to stdin: args=%q stored=%q", addArgs, stored)
	}
	if _, err := provider.Open(envelope); err != nil {
		t.Fatalf("open through fake Keychain: %v", err)
	}

	provider.run = func([]byte, ...string) ([]byte, error) {
		return nil, &keychainCommandFailure{missing: true}
	}
	plaintext, err := provider.Open(envelope)
	if plaintext != nil || !errors.Is(err, ErrSecretKeyMissing) {
		t.Fatalf("missing Keychain plaintext=%q error=%v", plaintext, err)
	}
	var providerErr *SecretProviderError
	if !errors.As(err, &providerErr) || providerErr.State != SecretProviderStateMissing {
		t.Fatalf("missing Keychain provider error = %#v", providerErr)
	}

	provider.run = func([]byte, ...string) ([]byte, error) {
		return []byte("not-a-key"), nil
	}
	plaintext, err = provider.Open(envelope)
	if plaintext != nil || !errors.Is(err, ErrSecretKeyCorrupt) {
		t.Fatalf("corrupt Keychain plaintext=%q error=%v", plaintext, err)
	}
	if !errors.As(err, &providerErr) || providerErr.State != SecretProviderStateCorrupt {
		t.Fatalf("corrupt Keychain provider error = %#v", providerErr)
	}
}
