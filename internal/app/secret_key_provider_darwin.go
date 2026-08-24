//go:build darwin

package app

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const (
	darwinKeychainAccount = "authority-wrapping-key"
	darwinKeychainPrefix  = "tv.getportico.server.secret-envelope"
)

type keychainCommandRunner func(stdin []byte, args ...string) ([]byte, error)

// keychainCommandFailure intentionally does not retain command output. The
// security tool is given the key through stdin, and neither stdout nor stderr
// is allowed to become a provider error/log payload.
type keychainCommandFailure struct {
	missing bool
	err     error
}

func (e *keychainCommandFailure) Error() string {
	if e != nil && e.missing {
		return "keychain item is missing"
	}
	return "keychain command failed"
}

func (e *keychainCommandFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

type keychainSecretKeyProvider struct {
	account string
	service string
	run     keychainCommandRunner
	mu      sync.Mutex
}

// NewSecretKeyProvider uses a private, mode-restricted key file. Packaged
// servers can run without an interactive login session, where the `security`
// command may otherwise wait for Keychain UI that cannot be presented.
func NewSecretKeyProvider(appDataDir string) SecretKeyProvider {
	return NewLocalSecretKeyProvider(filepath.Join(appDataDir, "keys", "hosted-authority.key"))
}

func newKeychainSecretKeyProvider(appDataDir string) *keychainSecretKeyProvider {
	return &keychainSecretKeyProvider{
		account: darwinKeychainAccount,
		service: darwinKeychainPrefix + "." + keychainNamespace(appDataDir),
		run:     runSecurityCommand,
	}
}

func (p *keychainSecretKeyProvider) ProviderKind() SecretProviderKind {
	return SecretProviderKindKeychain
}

func (p *keychainSecretKeyProvider) Seal(plaintext []byte) (SecretEnvelope, error) {
	if p == nil {
		return SecretEnvelope{}, newSecretProviderError(SecretProviderKindKeychain, "seal", SecretProviderStateUnavailable, ErrSecretKeyUnavailable)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	key, err := p.loadOrCreateKey()
	if err != nil {
		return SecretEnvelope{}, secretProviderErrorFor(SecretProviderKindKeychain, "seal", err)
	}
	return sealSecretEnvelope(SecretProviderKindKeychain, key, plaintext)
}

func (p *keychainSecretKeyProvider) Open(envelope SecretEnvelope) ([]byte, error) {
	if p == nil {
		return nil, newSecretProviderError(SecretProviderKindKeychain, "open", SecretProviderStateUnavailable, ErrSecretKeyUnavailable)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := validateSecretEnvelope(envelope); err != nil {
		return nil, newSecretProviderError(SecretProviderKindKeychain, "open", SecretProviderStateEnvelopeInvalid, err)
	}
	key, err := p.loadKey()
	if err != nil {
		return nil, secretProviderErrorFor(SecretProviderKindKeychain, "open", err)
	}
	return openSecretEnvelope(SecretProviderKindKeychain, key, envelope)
}

func (p *keychainSecretKeyProvider) loadOrCreateKey() ([]byte, error) {
	key, err := p.loadKey()
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, ErrSecretKeyMissing) {
		return nil, err
	}
	key = make([]byte, secretEnvelopeKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(key)
	// `security add-generic-password -w` prompts twice when the password is
	// supplied non-interactively. Keep both copies on stdin; the key never
	// appears in process arguments or diagnostic output.
	if _, err := p.run([]byte(encoded+"\n"+encoded+"\n"), "add-generic-password", "-a", p.account, "-s", p.service, "-w"); err != nil {
		// A second process may have created the item between find and add. Use
		// the already-published item rather than replacing it and invalidating
		// envelopes that raced this creation.
		if existing, lookupErr := p.loadKey(); lookupErr == nil {
			return existing, nil
		}
		return nil, fmt.Errorf("%w: keychain item could not be created", ErrSecretKeyUnavailable)
	}
	return p.loadKey()
}

func (p *keychainSecretKeyProvider) loadKey() ([]byte, error) {
	if p == nil || p.run == nil || strings.TrimSpace(p.account) == "" || strings.TrimSpace(p.service) == "" {
		return nil, ErrSecretKeyUnavailable
	}
	output, err := p.run(nil, "find-generic-password", "-a", p.account, "-s", p.service, "-w")
	if err != nil {
		var commandErr *keychainCommandFailure
		if errors.As(err, &commandErr) && commandErr.missing {
			return nil, ErrSecretKeyMissing
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrSecretKeyUnavailable
		}
		return nil, fmt.Errorf("%w: keychain lookup failed", ErrSecretKeyUnavailable)
	}
	encoded := strings.TrimSpace(string(output))
	key, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(key) != secretEnvelopeKeySize {
		return nil, ErrSecretKeyCorrupt
	}
	return key, nil
}

func runSecurityCommand(stdin []byte, args ...string) ([]byte, error) {
	command := exec.Command("security", args...)
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		missing := keychainMissingDiagnostic(stderr.String())
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 44 {
			missing = true
		}
		return nil, &keychainCommandFailure{missing: missing, err: err}
	}
	return stdout.Bytes(), nil
}

func keychainMissingDiagnostic(diagnostic string) bool {
	diagnostic = strings.ToLower(strings.TrimSpace(diagnostic))
	for _, marker := range []string{"could not be found", "no matching", "item not found", "does not exist"} {
		if strings.Contains(diagnostic, marker) {
			return true
		}
	}
	return false
}

func keychainNamespace(appDataDir string) string {
	path := strings.TrimSpace(appDataDir)
	if path == "" {
		path = "default"
	} else if absolute, err := filepath.Abs(path); err == nil {
		path = filepath.Clean(absolute)
	}
	digest := sha256.Sum256([]byte(path))
	return hex.EncodeToString(digest[:8])
}
