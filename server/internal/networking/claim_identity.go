package networking

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
)

// ServerIdentity derives the sole current server ID from its Ed25519 public key.
// It is a namespace binding, not proof of private-key possession. Claim and
// reset still require their signed operation and durable publication fences.
func ServerIdentity(public ed25519.PublicKey) (string, error) {
	if len(public) != ed25519.PublicKeySize {
		return "", ErrInvalid
	}
	h := sha256.New()
	_, _ = h.Write([]byte("portico.server.identity.v1\x00"))
	_, _ = h.Write(public)
	return "srv_" + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
}
