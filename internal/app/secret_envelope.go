package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SecretEnvelope is the only representation of long-lived authority material
// allowed in SQLite/settings. The wrapping key is deliberately not part of the
// envelope, so a database backup cannot authenticate by itself.
type SecretEnvelope struct {
	Version    int    `json:"version"`
	Algorithm  string `json:"algorithm"`
	KeyID      string `json:"keyId"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

const (
	secretEnvelopeVersion   = 1
	secretEnvelopeAlgorithm = "AES-256-GCM"
	secretEnvelopeKeySize   = 32
)

var (
	ErrSecretKeyMissing             = errors.New("secret key is missing; re-claim or restore the key")
	ErrSecretKeyCorrupt             = errors.New("secret key is corrupt; recovery is required")
	ErrSecretKeyMismatch            = errors.New("secret envelope belongs to a different secret key")
	ErrSecretEnvelope               = errors.New("secret envelope is invalid")
	ErrSecretEnvelopeAuthentication = errors.New("secret envelope authentication failed")
	ErrSecretKeyUnavailable         = errors.New("secret key provider is unavailable")
)

// SecretProviderKind identifies where the envelope wrapping key is kept. The
// key itself is never exposed through this metadata or through SecretEnvelope.
type SecretProviderKind string

const (
	SecretProviderKindLocal       SecretProviderKind = "local-file"
	SecretProviderKindKeychain    SecretProviderKind = "macos-keychain"
	SecretProviderKindDPAPI       SecretProviderKind = "windows-dpapi"
	SecretProviderKindUnavailable SecretProviderKind = "unavailable"
)

// SecretProviderState is a machine-readable failure state for callers that
// need to distinguish a missing/corrupt provider from an empty credential.
type SecretProviderState string

const (
	SecretProviderStateMissing            SecretProviderState = "missing"
	SecretProviderStateCorrupt            SecretProviderState = "corrupt"
	SecretProviderStateKeyMismatch        SecretProviderState = "key-mismatch"
	SecretProviderStateEnvelopeInvalid    SecretProviderState = "envelope-invalid"
	SecretProviderStateAuthenticationFail SecretProviderState = "authentication-failed"
	SecretProviderStateUnavailable        SecretProviderState = "unavailable"
	SecretProviderStateIO                 SecretProviderState = "io-error"
)

// SecretProviderError preserves the underlying sentinel while making provider
// failures explicit to status/reporting code. In particular, a provider error
// must not be translated into an empty authority string.
type SecretProviderError struct {
	Provider  SecretProviderKind
	Operation string
	State     SecretProviderState
	Err       error
}

func (e *SecretProviderError) Error() string {
	if e == nil {
		return "secret key provider error"
	}
	provider := strings.TrimSpace(string(e.Provider))
	if provider == "" {
		provider = string(SecretProviderKindUnavailable)
	}
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "access"
	}
	state := strings.TrimSpace(string(e.State))
	if state == "" {
		state = string(SecretProviderStateUnavailable)
	}
	if e.Err == nil {
		return fmt.Sprintf("secret key provider %s %s failed (%s)", provider, operation, state)
	}
	return fmt.Sprintf("secret key provider %s %s failed (%s): %v", provider, operation, state, e.Err)
}

func (e *SecretProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newSecretProviderError(provider SecretProviderKind, operation string, state SecretProviderState, err error) error {
	if err == nil {
		switch state {
		case SecretProviderStateMissing:
			err = ErrSecretKeyMissing
		case SecretProviderStateCorrupt, SecretProviderStateKeyMismatch:
			err = ErrSecretKeyCorrupt
		case SecretProviderStateEnvelopeInvalid:
			err = ErrSecretEnvelope
		case SecretProviderStateAuthenticationFail:
			err = ErrSecretEnvelopeAuthentication
		default:
			err = ErrSecretKeyUnavailable
		}
	}
	return &SecretProviderError{Provider: provider, Operation: operation, State: state, Err: err}
}

func secretProviderErrorFor(provider SecretProviderKind, operation string, err error) error {
	if err == nil {
		return nil
	}
	var providerErr *SecretProviderError
	if errors.As(err, &providerErr) {
		return err
	}
	state := SecretProviderStateIO
	switch {
	case errors.Is(err, ErrSecretKeyMissing):
		state = SecretProviderStateMissing
	case errors.Is(err, ErrSecretKeyCorrupt), errors.Is(err, ErrSecretKeyMismatch):
		state = SecretProviderStateCorrupt
	case errors.Is(err, ErrSecretEnvelopeAuthentication):
		state = SecretProviderStateAuthenticationFail
	case errors.Is(err, ErrSecretEnvelope):
		state = SecretProviderStateEnvelopeInvalid
	case errors.Is(err, ErrSecretKeyUnavailable):
		state = SecretProviderStateUnavailable
	}
	return newSecretProviderError(provider, operation, state, err)
}

// SecretProviderKindOf reports the selected provider without requiring the
// interface to grow a method that would break small test/fallback providers.
func SecretProviderKindOf(provider SecretKeyProvider) SecretProviderKind {
	if provider == nil {
		return SecretProviderKindUnavailable
	}
	identified, ok := provider.(interface{ ProviderKind() SecretProviderKind })
	if !ok {
		return SecretProviderKindUnavailable
	}
	return identified.ProviderKind()
}

// SecretKeyProvider is intentionally small so native platform key stores can
// be used without changing settings or Hosted authority call sites.
// Implementations must never serialize the key through this interface.
type SecretKeyProvider interface {
	Seal(plaintext []byte) (SecretEnvelope, error)
	Open(envelope SecretEnvelope) ([]byte, error)
}

// UnavailableSecretKeyProvider is an explicit platform-provider state. It is
// preferable to silently falling back to plaintext or inventing a second key.
type UnavailableSecretKeyProvider struct{ Reason string }

func (p UnavailableSecretKeyProvider) ProviderKind() SecretProviderKind {
	return SecretProviderKindUnavailable
}

func (p UnavailableSecretKeyProvider) failure(operation string) error {
	reason := strings.TrimSpace(p.Reason)
	if reason == "" {
		return newSecretProviderError(SecretProviderKindUnavailable, operation, SecretProviderStateUnavailable, ErrSecretKeyUnavailable)
	}
	return newSecretProviderError(SecretProviderKindUnavailable, operation, SecretProviderStateUnavailable, fmt.Errorf("%w: %s", ErrSecretKeyUnavailable, reason))
}

func (p UnavailableSecretKeyProvider) Seal([]byte) (SecretEnvelope, error) {
	return SecretEnvelope{}, p.failure("seal")
}

func (p UnavailableSecretKeyProvider) Open(SecretEnvelope) ([]byte, error) {
	return nil, p.failure("open")
}

// LocalSecretKeyProvider is the portable/headless provider. The file is
// created only when a secret is first written, is mode-restricted, and lives
// outside the database/backup payload. A missing key never gets silently
// regenerated: existing envelopes become an explicit recovery state.
type LocalSecretKeyProvider struct {
	path string
	mu   sync.Mutex
}

func NewLocalSecretKeyProvider(path string) *LocalSecretKeyProvider {
	return &LocalSecretKeyProvider{path: path}
}

func (p *LocalSecretKeyProvider) ProviderKind() SecretProviderKind {
	return SecretProviderKindLocal
}

func (p *LocalSecretKeyProvider) Seal(plaintext []byte) (SecretEnvelope, error) {
	if p == nil {
		return SecretEnvelope{}, newSecretProviderError(SecretProviderKindLocal, "seal", SecretProviderStateUnavailable, ErrSecretKeyUnavailable)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	key, err := p.loadOrCreateKey()
	if err != nil {
		return SecretEnvelope{}, secretProviderErrorFor(SecretProviderKindLocal, "seal", err)
	}
	return sealSecretEnvelope(SecretProviderKindLocal, key, plaintext)
}

func (p *LocalSecretKeyProvider) Open(envelope SecretEnvelope) ([]byte, error) {
	if p == nil {
		return nil, newSecretProviderError(SecretProviderKindLocal, "open", SecretProviderStateUnavailable, ErrSecretKeyUnavailable)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := validateSecretEnvelope(envelope); err != nil {
		return nil, newSecretProviderError(SecretProviderKindLocal, "open", SecretProviderStateEnvelopeInvalid, err)
	}
	key, err := p.loadKey()
	if err != nil {
		return nil, secretProviderErrorFor(SecretProviderKindLocal, "open", err)
	}
	return openSecretEnvelope(SecretProviderKindLocal, key, envelope)
}

func sealSecretEnvelope(provider SecretProviderKind, key, plaintext []byte) (SecretEnvelope, error) {
	if len(key) != secretEnvelopeKeySize {
		return SecretEnvelope{}, newSecretProviderError(provider, "seal", SecretProviderStateCorrupt, ErrSecretKeyCorrupt)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return SecretEnvelope{}, newSecretProviderError(provider, "seal", SecretProviderStateCorrupt, fmt.Errorf("%w: %v", ErrSecretKeyCorrupt, err))
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return SecretEnvelope{}, newSecretProviderError(provider, "seal", SecretProviderStateUnavailable, err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return SecretEnvelope{}, newSecretProviderError(provider, "seal", SecretProviderStateUnavailable, err)
	}
	keyID := secretKeyID(key)
	ciphertext := gcm.Seal(nil, nonce, plaintext, []byte(secretEnvelopeAAD(keyID)))
	return SecretEnvelope{
		Version:    secretEnvelopeVersion,
		Algorithm:  secretEnvelopeAlgorithm,
		KeyID:      keyID,
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
	}, nil
}

func openSecretEnvelope(provider SecretProviderKind, key []byte, envelope SecretEnvelope) ([]byte, error) {
	if err := validateSecretEnvelope(envelope); err != nil {
		return nil, newSecretProviderError(provider, "open", SecretProviderStateEnvelopeInvalid, err)
	}
	if len(key) != secretEnvelopeKeySize {
		return nil, newSecretProviderError(provider, "open", SecretProviderStateCorrupt, ErrSecretKeyCorrupt)
	}
	if secretKeyID(key) != envelope.KeyID {
		return nil, newSecretProviderError(provider, "open", SecretProviderStateKeyMismatch, fmt.Errorf("%w: %w", ErrSecretKeyCorrupt, ErrSecretKeyMismatch))
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, newSecretProviderError(provider, "open", SecretProviderStateEnvelopeInvalid, fmt.Errorf("%w: invalid nonce", ErrSecretEnvelope))
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, newSecretProviderError(provider, "open", SecretProviderStateEnvelopeInvalid, fmt.Errorf("%w: invalid ciphertext", ErrSecretEnvelope))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, newSecretProviderError(provider, "open", SecretProviderStateCorrupt, fmt.Errorf("%w: %v", ErrSecretKeyCorrupt, err))
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, newSecretProviderError(provider, "open", SecretProviderStateUnavailable, err)
	}
	if len(nonce) != gcm.NonceSize() || len(ciphertext) < gcm.Overhead() {
		return nil, newSecretProviderError(provider, "open", SecretProviderStateEnvelopeInvalid, ErrSecretEnvelope)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(secretEnvelopeAAD(envelope.KeyID)))
	if err != nil {
		authenticationErr := fmt.Errorf("%w: %w", ErrSecretEnvelope, ErrSecretEnvelopeAuthentication)
		return nil, newSecretProviderError(provider, "open", SecretProviderStateAuthenticationFail, authenticationErr)
	}
	return plaintext, nil
}

func validateSecretEnvelope(envelope SecretEnvelope) error {
	if envelope.Version != secretEnvelopeVersion || envelope.Algorithm != secretEnvelopeAlgorithm || strings.TrimSpace(envelope.KeyID) == "" || strings.TrimSpace(envelope.Nonce) == "" || strings.TrimSpace(envelope.Ciphertext) == "" {
		return ErrSecretEnvelope
	}
	return nil
}

func (p *LocalSecretKeyProvider) loadOrCreateKey() ([]byte, error) {
	key, err := p.loadKey()
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, ErrSecretKeyMissing) {
		return nil, err
	}
	if strings.TrimSpace(p.path) == "" {
		return nil, ErrSecretKeyUnavailable
	}
	if err := ensurePrivateKeyDirectory(filepath.Dir(p.path)); err != nil {
		return nil, err
	}
	key = make([]byte, secretEnvelopeKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(p.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return p.loadKey()
		}
		return nil, err
	}
	removeOnFailure := true
	defer func() {
		if removeOnFailure {
			_ = os.Remove(p.path)
		}
	}()
	if _, err := file.Write(key); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(p.path)
	if err != nil {
		return nil, err
	}
	if err := validatePrivateKeyFile(info); err != nil {
		return nil, err
	}
	removeOnFailure = false
	return key, nil
}

func (p *LocalSecretKeyProvider) loadKey() ([]byte, error) {
	if p == nil || strings.TrimSpace(p.path) == "" {
		return nil, ErrSecretKeyUnavailable
	}
	if err := validateExistingPrivateKeyDirectory(filepath.Dir(p.path)); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	info, err := os.Lstat(p.path)
	if os.IsNotExist(err) {
		return nil, ErrSecretKeyMissing
	}
	if err != nil {
		return nil, err
	}
	if err := validatePrivateKeyFile(info); err != nil {
		return nil, err
	}
	file, err := os.Open(p.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return nil, fmt.Errorf("%w: key changed while opening", ErrSecretKeyCorrupt)
	}
	if err := validatePrivateKeyFile(openedInfo); err != nil {
		return nil, err
	}
	key, err := io.ReadAll(io.LimitReader(file, secretEnvelopeKeySize+1))
	if err != nil {
		return nil, err
	}
	if len(key) != secretEnvelopeKeySize {
		return nil, ErrSecretKeyCorrupt
	}
	return key, nil
}

func ensurePrivateKeyDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return validateExistingPrivateKeyDirectory(path)
}

func validateExistingPrivateKeyDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: key directory is not a real private directory", ErrSecretKeyCorrupt)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: key directory is accessible by group or others", ErrSecretKeyCorrupt)
	}
	if err := validateNativeCredentialOwner(info); err != nil {
		return fmt.Errorf("%w: %v", ErrSecretKeyCorrupt, err)
	}
	return nil
}

func validatePrivateKeyFile(info os.FileInfo) error {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: key must be a regular non-symlink file", ErrSecretKeyCorrupt)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: key file is accessible by group or others", ErrSecretKeyCorrupt)
	}
	if err := validateNativeCredentialOwner(info); err != nil {
		return fmt.Errorf("%w: %v", ErrSecretKeyCorrupt, err)
	}
	return nil
}

func secretKeyID(key []byte) string {
	digest := sha256.Sum256(key)
	return base64.RawURLEncoding.EncodeToString(digest[:12])
}

// Retain the old helper name for package-local callers/tests while keeping the
// key identifier calculation shared by every provider.
func localSecretKeyID(key []byte) string { return secretKeyID(key) }

func secretEnvelopeAAD(keyID string) string { return "portico-secret-envelope-v1\x00" + keyID }

func (s *Server) currentSecretProvider() SecretKeyProvider {
	if s != nil && s.secretProvider != nil {
		return s.secretProvider
	}
	if s == nil {
		return UnavailableSecretKeyProvider{Reason: "server is unavailable"}
	}
	return NewSecretKeyProvider(s.cfg.AppDataDir)
}

func (s *Server) encryptRemoteSecret(value string) ([]byte, error) {
	envelope, err := s.currentSecretProvider().Seal([]byte(value))
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func (s *Server) decryptRemoteSecret(raw []byte) (string, error) {
	var envelope SecretEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", fmt.Errorf("%w: %v", ErrSecretEnvelope, err)
	}
	plaintext, err := s.currentSecretProvider().Open(envelope)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
