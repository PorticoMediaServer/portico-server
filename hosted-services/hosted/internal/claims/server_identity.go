package claims

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
)

// ServerID is the one current key-bound identity. Key rotation creates a new ID;
// no previous random ID, alias or different-key same-ID adoption is accepted.
func ServerID(publicKey []byte) (string, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return "", ErrInvalid
	}
	h := sha256.New()
	_, _ = h.Write([]byte("portico.server.identity.v1\x00"))
	_, _ = h.Write(publicKey)
	return "srv_" + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
}
