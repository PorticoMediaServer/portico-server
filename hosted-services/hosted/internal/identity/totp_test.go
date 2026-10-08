package identity

import (
	"encoding/base32"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestTOTPRFC6238AndReplay(t *testing.T) {
	key := []byte("12345678901234567890")
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(key)
	for _, tc := range []struct {
		at   int64
		want string
	}{{59, "94287082"}, {1111111109, "07081804"}, {1111111111, "14050471"}, {1234567890, "89005924"}, {2000000000, "69279037"}, {20000000000, "65353130"}} {
		if got := HOTP(key, uint64(tc.at/30), 8); got != tc.want {
			t.Fatalf("RFC vector %d: %s", tc.at, got)
		}
		code := tc.want[2:]
		step, ok := TOTPStep(secret, code, time.Unix(tc.at, 0), -1)
		if !ok || step != tc.at/30 {
			t.Fatalf("six-digit projection %d", tc.at)
		}
		if _, ok = TOTPStep(secret, code, time.Unix(tc.at, 0), step); ok {
			t.Fatal("replayed TOTP accepted")
		}
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef", "123 45"} {
		if _, ok := TOTPStep(secret, bad, time.Now(), -1); ok {
			t.Fatal("malformed factor accepted")
		}
	}
	if _, ok := TOTPStep("not-base32", "123456", time.Now(), -1); ok {
		t.Fatal("malformed secret accepted")
	}
	code := HOTP(key, 100, 6)
	if _, ok := TOTPStep(secret, code, time.Unix(103*30, 0), -1); ok {
		t.Fatal("unbounded clock drift accepted")
	}
}
func TestSecurityRandomCodesAndDomainSeparation(t *testing.T) {
	secret, e := NewTOTPSecret()
	if e != nil || len(secret) != 32 {
		t.Fatalf("secret %v", e)
	}
	u, e := url.Parse(TOTPURI(secret, "example-user"))
	if e != nil || u.Scheme != "otpauth" || u.Host != "totp" || u.Query().Get("secret") != secret || u.Query().Get("issuer") != "Portico" {
		t.Fatal("invalid authenticator URI")
	}
	codes, e := NewRecoveryCodes()
	if e != nil || len(codes) != 10 {
		t.Fatal("recovery codes")
	}
	seen := map[string]bool{}
	for _, code := range codes {
		normalized := NormalizeRecoveryCode(strings.ToLower(code))
		if len(normalized) != 32 || seen[normalized] {
			t.Fatal("recovery code collision or truncation")
		}
		seen[normalized] = true
	}
	a := SecretDigest([]byte("key"), "registration-code", "A", "123456")
	if string(a) == string(SecretDigest([]byte("key"), "registration-code", "B", "123456")) || string(a) == string(SecretDigest([]byte("key"), "email-change", "A", "123456")) {
		t.Fatal("purpose/account separation lost")
	}
	for range 30 {
		code, e := NewEmailCode()
		if e != nil || len(code) != 6 {
			t.Fatal("email code")
		}
		for _, c := range code {
			if c < '0' || c > '9' {
				t.Fatal("nonnumeric code")
			}
		}
	}
}
