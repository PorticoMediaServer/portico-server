package oidclogin

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/coreos/go-oidc/v3/oidc"
	"strings"
	"testing"
	"time"
)

func TestAppleAccountNotificationVerification(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewAppleNotificationVerifier(&oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, []string{"tv.getportico.ios", "tv.getportico.account"})
	now := time.Now().Unix()
	cases := []struct {
		name   string
		mutate func(map[string]any, map[string]any)
		valid  bool
	}{
		{"documented no expiry", func(c, e map[string]any) {}, true},
		{"encoded event", func(c, e map[string]any) { b, _ := json.Marshal(e); c["events"] = string(b) }, true},
		{"service audience", func(c, e map[string]any) { c["aud"] = "tv.getportico.account" }, true},
		{"email disabled", func(c, e map[string]any) {
			e["type"] = "email-disabled"
			e["email"] = "relay@privaterelay.appleid.com"
			e["is_private_email"] = "true"
		}, true},
		{"wrong issuer", func(c, e map[string]any) { c["iss"] = "https://evil.example" }, false},
		{"wrong audience", func(c, e map[string]any) { c["aud"] = "other.app" }, false},
		{"multiple audiences", func(c, e map[string]any) { c["aud"] = []string{"tv.getportico.ios", "other.app"} }, false},
		{"expired", func(c, e map[string]any) { c["exp"] = now - 10 }, false},
		{"future", func(c, e map[string]any) { c["iat"] = now + 3600 }, false},
		{"future event", func(c, e map[string]any) { e["event_time"] = now + 3600 }, false},
		{"not yet valid", func(c, e map[string]any) { c["nbf"] = now + 3600 }, false},
		{"missing id", func(c, e map[string]any) { delete(c, "jti") }, false},
		{"missing subject", func(c, e map[string]any) { delete(e, "sub") }, false},
		{"non relay mail", func(c, e map[string]any) {
			e["type"] = "email-disabled"
			e["email"] = "person@example.com"
			e["is_private_email"] = true
		}, false},
		{"unknown type", func(c, e map[string]any) { e["type"] = "delete-everything" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := map[string]any{"type": "consent-revoked", "sub": "apple-subject", "event_time": now - 5}
			c := map[string]any{"iss": AppleIssuer, "aud": "tv.getportico.ios", "iat": now, "jti": "notification-1", "events": e}
			tc.mutate(c, e)
			token := signAppleEvent(t, key, c, "RS256")
			_, err := verifier.Verify(context.Background(), token)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
	c := map[string]any{"iss": AppleIssuer, "aud": "tv.getportico.ios", "iat": now, "jti": "id", "events": map[string]any{"type": "account-deleted", "sub": "s", "event_time": now}}
	foreign, _ := rsa.GenerateKey(rand.Reader, 2048)
	for _, token := range []string{signAppleEvent(t, foreign, c, "RS256"), signAppleEvent(t, key, c, "none"), strings.Repeat("x", AppleNotificationMaxBytes+1)} {
		if _, err := verifier.Verify(context.Background(), token); err == nil {
			t.Fatal("invalid signature/algorithm/size accepted")
		}
	}
}
func signAppleEvent(t *testing.T, key *rsa.PrivateKey, c map[string]any, alg string) string {
	t.Helper()
	enc := base64.RawURLEncoding.EncodeToString
	header, _ := json.Marshal(map[string]string{"alg": alg, "kid": "fixture"})
	body, _ := json.Marshal(c)
	input := enc(header) + "." + enc(body)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + enc(sig)
}
