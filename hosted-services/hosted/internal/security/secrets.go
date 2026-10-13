package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"portico.local/hosted/trust"
	"sync/atomic"
	"time"
)

func RandomID(prefix string) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}
func Hash(value string) []byte { h := sha256.Sum256([]byte(value)); return h[:] }

// MaxSigningRevision fits user servers' signed SQLite integers and remains
// exact when a public JSON client decodes and forwards an identity envelope.
const MaxSigningRevision uint64 = 1<<53 - 1

type Keys struct {
	// Shared across immutable service copies; only learned root revocation sets it.
	SignerRevoked *atomic.Bool
	RootPublicKey ed25519.PublicKey
	Certificate   *trust.Certificate
	PrivateKey    ed25519.PrivateKey
	KeyID         string
	SealKey       []byte
	SealVersion   uint32
	// SealLegacy also opens values written before sealing was versioned
	// (nonce||ciphertext, context as the only associated data) with the same
	// key: the production database from the 22 Sep release. New values are
	// always written in the versioned format.
	SealLegacy bool
}
type keyFile struct {
	PrivateKey string `json:"privateKey"`
}
type SealFile struct {
	Version uint32 `json:"version"`
	Key     string `json:"key"`
	// LegacyUnversioned: this key also sealed values in the pre-versioned
	// format, which Open still reads (see Keys.SealLegacy).
	LegacyUnversioned bool `json:"legacyUnversioned,omitempty"`
}

func readPrivate(path string, out any) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("key file must be private (0600)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
func LoadKeys(path string) (*Keys, error) {
	root, err := trust.DefaultRoot()
	if err != nil {
		return nil, err
	}
	return LoadKeysWithRoot(path, root)
}
func LoadKeysWithRoot(path string, root ed25519.PublicKey) (*Keys, error) {
	var file keyFile
	if err := readPrivate(path, &file); err != nil {
		return nil, err
	}
	private, err := base64.RawURLEncoding.Strict().DecodeString(file.PrivateKey)
	if err != nil || len(private) != 64 {
		return nil, errors.New("invalid signing key")
	}
	var cert trust.Certificate
	raw, err := os.ReadFile(path + ".certificate.json")
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &cert); err != nil {
		return nil, err
	}
	if cert.Revision > MaxSigningRevision {
		return nil, trust.ErrTrust
	}
	public, err := cert.Verify(root, "documents", time.Now())
	if err != nil {
		return nil, err
	}
	if !ed25519.PublicKey(public).Equal(ed25519.PrivateKey(private).Public()) {
		return nil, trust.ErrTrust
	}
	var seal SealFile
	if err = readPrivate(path+".seal.json", &seal); err != nil {
		return nil, err
	}
	key, err := base64.RawURLEncoding.Strict().DecodeString(seal.Key)
	if err != nil || len(key) != 32 || seal.Version == 0 {
		return nil, errors.New("invalid sealing key")
	}
	return &Keys{SignerRevoked: &atomic.Bool{}, RootPublicKey: root, PrivateKey: private, KeyID: cert.KeyID, SealKey: key, SealVersion: seal.Version, SealLegacy: seal.LegacyUnversioned, Certificate: &cert}, nil
}
func (k *Keys) PublicKey() string {
	return base64.RawURLEncoding.EncodeToString(k.PrivateKey.Public().(ed25519.PublicKey))
}
func (k *Keys) Seal(plain, context string) ([]byte, error) {
	b, e := aes.NewCipher(k.SealKey)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return nil, e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return nil, e
	}
	version := k.SealVersion
	if version == 0 {
		version = 1
	}
	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, version)
	return append(prefix, g.Seal(n, n, []byte(plain), append(prefix, []byte(context)...))...), nil
}
func (k *Keys) Open(value []byte, context string) (string, error) {
	b, e := aes.NewCipher(k.SealKey)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return "", e
	}
	version := k.SealVersion
	if version == 0 {
		version = 1
	}
	if len(value) >= 4+g.NonceSize() && binary.BigEndian.Uint32(value[:4]) == version {
		prefix, body := value[:4], value[4:]
		p, e := g.Open(nil, body[:g.NonceSize()], body[g.NonceSize():], append(append([]byte(nil), prefix...), []byte(context)...))
		if e == nil || !k.SealLegacy {
			return string(p), e
		}
	}
	if k.SealLegacy && len(value) >= g.NonceSize() {
		p, e := g.Open(nil, value[:g.NonceSize()], value[g.NonceSize():], []byte(context))
		return string(p), e
	}
	return "", errors.New("unknown sealing key version")
}

// ValidateSigner applies the offline-root contract at use time, not just at
// startup. An expired or revoked online certificate must stop issuing claims.
func (k *Keys) ValidateSigner(now time.Time) error {
	if k == nil || k.SignerRevoked != nil && k.SignerRevoked.Load() || len(k.PrivateKey) != ed25519.PrivateKeySize || k.Certificate == nil || k.Certificate.Revision > MaxSigningRevision || k.KeyID != k.Certificate.KeyID {
		return trust.ErrTrust
	}
	public, e := k.Certificate.Verify(k.RootPublicKey, "documents", now)
	if e != nil || !public.Equal(k.PrivateKey.Public()) {
		return trust.ErrTrust
	}
	return nil
}
func (k *Keys) Sign(raw []byte) (trust.Envelope, error) {
	if e := k.ValidateSigner(time.Now()); e != nil {
		return trust.Envelope{}, e
	}
	return trust.Sign(k.PrivateKey, k.Certificate, raw)
}
