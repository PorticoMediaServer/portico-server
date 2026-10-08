package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
)

func NewTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}
func TOTPURI(secret, username string) string {
	return "otpauth://totp/" + url.PathEscape("Portico:"+username) + "?" + url.Values{"secret": {secret}, "issuer": {"Portico"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()
}

// TOTPStep implements RFC 6238, six digits and a bounded +/- one-step drift.
// The durable caller atomically compares and advances lastAcceptedStep.
func TOTPStep(secret, code string, now time.Time, lastAcceptedStep int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(key) < 20 {
		return 0, false
	}
	current := now.Unix() / 30
	accepted := int64(-1)
	for delta := int64(-1); delta <= 1; delta++ {
		step := current + delta
		value := HOTP(key, uint64(step), 6)
		if subtle.ConstantTimeCompare([]byte(code), []byte(value)) == 1 && step > lastAcceptedStep {
			accepted = step
		}
	}
	return accepted, accepted >= 0
}
func HOTP(key []byte, step uint64, digits int) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], step)
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	h := mac.Sum(nil)
	offset := h[len(h)-1] & 15
	n := (uint32(h[offset])&0x7f)<<24 | uint32(h[offset+1])<<16 | uint32(h[offset+2])<<8 | uint32(h[offset+3])
	modulus := uint32(1)
	for range digits {
		modulus *= 10
	}
	return fmt.Sprintf("%0*d", digits, n%modulus)
}
func NewEmailCode() (string, error) {
	n, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return "", e
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
func NewRecoveryCodes() ([]string, error) {
	out := make([]string, 10)
	for i := range out {
		b := make([]byte, 16)
		if _, e := rand.Read(b); e != nil {
			return nil, e
		}
		s := strings.ToUpper(hex.EncodeToString(b))
		out[i] = s[:8] + "-" + s[8:16] + "-" + s[16:24] + "-" + s[24:]
	}
	return out, nil
}
func NormalizeRecoveryCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

// Keyed, domain-separated hashes protect low-entropy email codes and rate keys.
func SecretDigest(key []byte, purpose, owner, value string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(purpose))
	mac.Write([]byte{0})
	mac.Write([]byte(owner))
	mac.Write([]byte{0})
	mac.Write([]byte(value))
	return mac.Sum(nil)
}
