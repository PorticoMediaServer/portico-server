package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretEnvelopeRoundTripKeepsWrappingKeyOutsideEnvelope(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "keys", "hosted-authority.key")
	provider := NewLocalSecretKeyProvider(keyPath)
	plaintext := []byte("hosted-authority-value")

	envelope, err := provider.Seal(plaintext)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if envelope.Version != secretEnvelopeVersion || envelope.Algorithm != secretEnvelopeAlgorithm || envelope.KeyID == "" || envelope.Nonce == "" || envelope.Ciphertext == "" {
		t.Fatalf("invalid versioned envelope: %#v", envelope)
	}
	serialized, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read generated key for assertion: %v", err)
	}
	if bytes.Contains(serialized, plaintext) || bytes.Contains(serialized, key) || strings.Contains(string(serialized), base64.RawURLEncoding.EncodeToString(key)) {
		t.Fatalf("envelope serialized usable authority or wrapping key: %s", serialized)
	}

	opened, err := provider.Open(envelope)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Fatalf("opened plaintext = %q, want %q", opened, plaintext)
	}
}

func TestSecretEnvelopeRejectsUnsupportedVersionAndUnauthenticatedData(t *testing.T) {
	provider := NewLocalSecretKeyProvider(filepath.Join(t.TempDir(), "keys", "hosted-authority.key"))
	envelope, err := provider.Seal([]byte("authority"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	invalid := []SecretEnvelope{
		func() SecretEnvelope { copy := envelope; copy.Version++; return copy }(),
		func() SecretEnvelope { copy := envelope; copy.Algorithm = "AES-128-GCM"; return copy }(),
		func() SecretEnvelope { copy := envelope; copy.KeyID = ""; return copy }(),
		func() SecretEnvelope { copy := envelope; copy.Nonce = ""; return copy }(),
		func() SecretEnvelope { copy := envelope; copy.Ciphertext = ""; return copy }(),
	}
	for index, candidate := range invalid {
		_, candidateErr := provider.Open(candidate)
		if !errors.Is(candidateErr, ErrSecretEnvelope) {
			t.Fatalf("invalid envelope %d error = %v, want ErrSecretEnvelope", index, candidateErr)
		}
		var providerErr *SecretProviderError
		if !errors.As(candidateErr, &providerErr) || providerErr.State != SecretProviderStateEnvelopeInvalid {
			t.Fatalf("invalid envelope %d provider error = %#v", index, providerErr)
		}
	}

	tampered := envelope
	ciphertext, err := base64.RawURLEncoding.DecodeString(tampered.Ciphertext)
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}
	ciphertext[len(ciphertext)-1] ^= 0x01
	tampered.Ciphertext = base64.RawURLEncoding.EncodeToString(ciphertext)
	_, err = provider.Open(tampered)
	if !errors.Is(err, ErrSecretEnvelope) || !errors.Is(err, ErrSecretEnvelopeAuthentication) {
		t.Fatalf("tampered envelope error = %v, want authentication error", err)
	}
	var providerErr *SecretProviderError
	if !errors.As(err, &providerErr) || providerErr.State != SecretProviderStateAuthenticationFail {
		t.Fatalf("tampered provider error = %#v", providerErr)
	}
}

func TestSecretEnvelopeRejectsKeyMismatch(t *testing.T) {
	provider := NewLocalSecretKeyProvider(filepath.Join(t.TempDir(), "keys", "hosted-authority.key"))
	envelope, err := provider.Seal([]byte("authority"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	envelope.KeyID = "different-key-id"
	_, err = provider.Open(envelope)
	if !errors.Is(err, ErrSecretKeyMismatch) || !errors.Is(err, ErrSecretKeyCorrupt) {
		t.Fatalf("mismatched key id error = %v", err)
	}
	var providerErr *SecretProviderError
	if !errors.As(err, &providerErr) || providerErr.State != SecretProviderStateKeyMismatch {
		t.Fatalf("mismatched key id provider error = %#v", providerErr)
	}
}

func TestLocalSecretKeyProviderRejectsMissingAndUnsafeProvider(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		keyPath := filepath.Join(t.TempDir(), "keys", "hosted-authority.key")
		provider := NewLocalSecretKeyProvider(keyPath)
		envelope, err := provider.Seal([]byte("authority"))
		if err != nil {
			t.Fatalf("seal: %v", err)
		}
		if err := os.Remove(keyPath); err != nil {
			t.Fatalf("remove key: %v", err)
		}
		plaintext, err := provider.Open(envelope)
		if plaintext != nil || !errors.Is(err, ErrSecretKeyMissing) {
			t.Fatalf("open missing key plaintext=%q error=%v", plaintext, err)
		}
		var providerErr *SecretProviderError
		if !errors.As(err, &providerErr) || providerErr.State != SecretProviderStateMissing {
			t.Fatalf("missing provider error = %#v", providerErr)
		}
	})

	t.Run("unsafe-file-permissions", func(t *testing.T) {
		keyPath := filepath.Join(t.TempDir(), "keys", "hosted-authority.key")
		provider := NewLocalSecretKeyProvider(keyPath)
		envelope, err := provider.Seal([]byte("authority"))
		if err != nil {
			t.Fatalf("seal: %v", err)
		}
		if err := os.Chmod(keyPath, 0o640); err != nil {
			t.Fatalf("chmod key: %v", err)
		}
		plaintext, err := provider.Open(envelope)
		if plaintext != nil || !errors.Is(err, ErrSecretKeyCorrupt) {
			t.Fatalf("open unsafe key plaintext=%q error=%v", plaintext, err)
		}
		var providerErr *SecretProviderError
		if !errors.As(err, &providerErr) || providerErr.State != SecretProviderStateCorrupt {
			t.Fatalf("unsafe provider error = %#v", providerErr)
		}
	})

	t.Run("corrupt-key-bytes", func(t *testing.T) {
		keyPath := filepath.Join(t.TempDir(), "keys", "hosted-authority.key")
		provider := NewLocalSecretKeyProvider(keyPath)
		envelope, err := provider.Seal([]byte("authority"))
		if err != nil {
			t.Fatalf("seal: %v", err)
		}
		if err := os.WriteFile(keyPath, []byte("short"), 0o600); err != nil {
			t.Fatalf("corrupt key: %v", err)
		}
		plaintext, err := provider.Open(envelope)
		if plaintext != nil || !errors.Is(err, ErrSecretKeyCorrupt) {
			t.Fatalf("open corrupt key plaintext=%q error=%v", plaintext, err)
		}
	})
}

func TestUnavailableSecretKeyProviderIsTypedAndNeverAnEmptyCredential(t *testing.T) {
	provider := UnavailableSecretKeyProvider{Reason: "Keychain is unavailable"}
	if SecretProviderKindOf(provider) != SecretProviderKindUnavailable {
		t.Fatalf("provider kind = %q", SecretProviderKindOf(provider))
	}
	plaintext, err := provider.Open(SecretEnvelope{Version: secretEnvelopeVersion})
	if plaintext != nil || !errors.Is(err, ErrSecretKeyUnavailable) {
		t.Fatalf("unavailable provider plaintext=%q error=%v", plaintext, err)
	}
	var providerErr *SecretProviderError
	if !errors.As(err, &providerErr) || providerErr.State != SecretProviderStateUnavailable || !strings.Contains(err.Error(), "Keychain is unavailable") {
		t.Fatalf("unavailable provider error = %#v (%v)", providerErr, err)
	}
}
