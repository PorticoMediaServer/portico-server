package device

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/crypto/hkdf"
	"io"
	"net"
	"net/url"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"strings"
	"time"
)

const TVProtocol = "tv-x25519-v1"

type TVGrant struct {
	Kind         string `json:"kind"`
	Version      int    `json:"version"`
	Issuer       string `json:"issuer"`
	RequestID    string `json:"requestId"`
	RecipientKey string `json:"recipientKey"`
	PublicKey    string `json:"publicKey"`
	Salt         string `json:"salt"`
	Nonce        string `json:"nonce"`
	Ciphertext   string `json:"ciphertext"`
}

func validTVKey(value string) bool {
	b, e := base64.RawURLEncoding.Strict().DecodeString(value)
	if e != nil || len(b) != 32 || base64.RawURLEncoding.EncodeToString(b) != value {
		return false
	}
	pub, e := ecdh.X25519().NewPublicKey(b)
	if e != nil {
		return false
	}
	// ECDH, not just the 32-byte parser, rejects small-order/all-zero points.
	private, e := ecdh.X25519().GenerateKey(rand.Reader)
	if e != nil {
		return false
	}
	_, e = private.ECDH(pub)
	return e == nil
}
func (s *Service) ConfigureTV(origin string, local bool) error {
	u, e := url.Parse(origin)
	if e != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return ErrInvalid
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(local && u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return ErrInvalid
	}
	s.tvIssuer = u.String()
	return nil
}
func sealTVGrant(issuer, requestID, recipient string, plain []byte) (TVGrant, error) {
	out := TVGrant{Kind: "portico.tv-grant", Version: 1, Issuer: issuer, RequestID: requestID, RecipientKey: recipient}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(recipient)
	if e != nil || len(raw) != 32 {
		return out, ErrInvalid
	}
	public, e := ecdh.X25519().NewPublicKey(raw)
	if e != nil {
		return out, ErrInvalid
	}
	ephemeral, e := ecdh.X25519().GenerateKey(rand.Reader)
	if e != nil {
		return out, e
	}
	shared, e := ephemeral.ECDH(public)
	if e != nil {
		return out, ErrInvalid
	}
	defer clear(shared)
	out.PublicKey = base64.RawURLEncoding.EncodeToString(ephemeral.PublicKey().Bytes())
	// This context is byte-for-byte shared with PorticoNetwork.openTVGrant. A
	// different authority, requester, recipient, or ephemeral key cannot decrypt.
	info := []byte("portico.tv-grant.v1\x00" + issuer + "\x00" + requestID + "\x00" + recipient + "\x00" + out.PublicKey)
	salt := make([]byte, 32)
	if _, e = io.ReadFull(rand.Reader, salt); e != nil {
		return out, e
	}
	key := make([]byte, 32)
	defer clear(key)
	if _, e = io.ReadFull(hkdf.New(sha256.New, shared, salt, info), key); e != nil {
		return out, e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return out, e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return out, e
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = io.ReadFull(rand.Reader, nonce); e != nil {
		return out, e
	}
	out.Salt = base64.RawURLEncoding.EncodeToString(salt)
	out.Nonce = base64.RawURLEncoding.EncodeToString(nonce)
	out.Ciphertext = base64.RawURLEncoding.EncodeToString(g.Seal(nil, nonce, plain, info))
	return out, nil
}
func (s *Service) PollTV(ctx context.Context, secret string) (json.RawMessage, error) {
	if s.tvIssuer == "" || !strings.HasPrefix(secret, "device_") || len(secret) != 50 {
		return nil, &ProtocolError{Code: "invalid_grant"}
	}
	token := security.RandomID("hat_")
	family, refresh := identity.NewFamily("native")
	expiry := time.Now().UTC().Add(s.accessLifetime)
	pair := identity.Session{AccessToken: token, RefreshToken: refresh, FamilyID: family.ID, RefreshExpiresAt: &family.RefreshExpiresAt, ExpiresAt: expiry, Mode: "native"}
	raw, e := json.Marshal(pair)
	if e != nil {
		return nil, e
	}
	defer clear(raw)
	sealed, e := s.keys.Seal(string(raw), "device-grant:"+secret)
	if e != nil {
		return nil, e
	}
	grant, e := s.repo.PollDevice(ctx, s.hash("secret", secret), Issue{Protocol: TVProtocol, Family: family, Hash: security.Hash(token), Cipher: sealed, ExpiresAt: expiry,
		SealGrant: func(id, key string, viewer identity.Viewer) ([]byte, error) {
			pair.Account = viewer.Account
			raw, e := json.Marshal(pair)
			if e != nil {
				return nil, e
			}
			defer clear(raw)
			out, e := sealTVGrant(s.tvIssuer, id, key, raw)
			if e != nil {
				return nil, e
			}
			return json.Marshal(out)
		}})
	if e != nil {
		return nil, e
	}
	return grant.Protected, nil
}
