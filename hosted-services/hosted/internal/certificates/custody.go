package certificates

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/crypto/acme"
	"portico.local/hosted/internal/egress"
	"portico.local/hosted/internal/security"
)

// CustodyScope is deliberately restricted to the configured production issuer.
// Recovery is an operator action, never an alternate certificate initializer.
func CustodyScope(cfg *Config) (string, string, error) {
	if cfg == nil || cfg.ID() != "google-production" {
		return "", "", ErrConfiguration
	}
	directory, err := cfg.directory()
	return cfg.ID(), directory, err
}

func custodyKey(seal *security.Keys, issuer string, cipher []byte) (*ecdsa.PrivateKey, error) {
	encoded, err := seal.Open(cipher, "portico/acme-account/v1/"+issuer)
	if err != nil {
		return nil, ErrConfiguration
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrConfiguration
	}
	defer clear(raw)
	parsed, err := x509.ParsePKCS8PrivateKey(raw)
	if err != nil {
		return nil, ErrConfiguration
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, ErrConfiguration
	}
	return key, nil
}

// VerifyCustodyRecovery authenticates two read-only provider lookups. The old
// key must name the exact preserved valid account; the candidate must definitely
// have no provider account. No Register, EAB, newOrder or key rotation is used.
func VerifyCustodyRecovery(ctx context.Context, cfg *Config, original, candidate Account) error {
	return verifyCustodyRecovery(ctx, cfg, original, candidate, func(key *ecdsa.PrivateKey, directory string) *acme.Client {
		origin, _ := url.Parse(directory)
		client := egress.NewClient(false)
		client.Timeout = 15 * time.Second
		client.Transport = &providerTransport{base: client.Transport, origin: origin.Scheme + "://" + origin.Host}
		return &acme.Client{Key: key, DirectoryURL: directory, HTTPClient: client, UserAgent: "Portico-ACME-Custody/1", RetryBackoff: noACMERetry}
	})
}

func verifyCustodyRecovery(ctx context.Context, cfg *Config, original, candidate Account, client func(*ecdsa.PrivateKey, string) *acme.Client) error {
	issuer, directory, err := CustodyScope(cfg)
	if err != nil {
		return err
	}
	origin, _ := url.Parse(directory)
	if original.URL == "" || !sameProviderURL(original.URL, origin.Scheme+"://"+origin.Host) || candidate.URL != "" || len(original.Cipher) == 0 || len(candidate.Cipher) == 0 || bytes.Equal(original.Cipher, candidate.Cipher) {
		return ErrConflict
	}
	seal, err := cfg.sealing()
	if err != nil {
		return ErrConfiguration
	}
	defer clear(seal.SealKey)
	oldKey, err := custodyKey(seal, issuer, original.Cipher)
	if err != nil {
		return err
	}
	newKey, err := custodyKey(seal, issuer, candidate.Cipher)
	if err != nil {
		return err
	}
	if oldKey.Equal(newKey) {
		return ErrConflict
	}
	old, err := client(oldKey, directory).GetReg(ctx, "")
	if err != nil || old == nil || old.Status != acme.StatusValid || old.URI != original.URL {
		return ErrUncertain
	}
	_, err = client(newKey, directory).GetReg(ctx, "")
	if !accountDoesNotExist(err) {
		return ErrUncertain
	}
	return ctx.Err()
}

// Compile-time pin: recovery uses the ordinary verified public HTTP client.
var _ http.RoundTripper = (*providerTransport)(nil)
