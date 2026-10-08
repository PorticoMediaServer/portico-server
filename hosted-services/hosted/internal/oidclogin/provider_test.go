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
	"golang.org/x/oauth2"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProviderVerifiesSignatureAudienceAndTime(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	enc := base64.RawURLEncoding.EncodeToString
	aud := "client"
	expires := time.Now().Add(time.Minute).Unix()
	badSignature := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/keys" {
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test", "alg": "RS256", "use": "sig", "n": enc(key.N.Bytes()), "e": enc(big.NewInt(int64(key.E)).Bytes())}}})
			return
		}
		if e := r.ParseForm(); e != nil || r.Form.Get("code_verifier") == "" {
			t.Error("missing PKCE verifier")
		}
		header := enc([]byte(`{"alg":"RS256","kid":"test"}`))
		body, _ := json.Marshal(map[string]any{"iss": "https://accounts.google.com", "sub": "subject", "aud": aud, "exp": expires, "iat": time.Now().Unix(), "nonce": "nonce", "email": "example@example.test", "email_verified": true})
		input := header + "." + enc(body)
		hash := sha256.Sum256([]byte(input))
		sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
		if badSignature {
			sig[0] ^= 1
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "discarded", "token_type": "Bearer", "id_token": input + "." + enc(sig)})
	}))
	defer server.Close()
	p := &Provider{ID: "google", Issuer: "https://accounts.google.com", Audience: "client", client: server.Client(), oauth: oauth2.Config{ClientID: "client", ClientSecret: "secret", Endpoint: oauth2.Endpoint{TokenURL: server.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}, verifier: oidc.NewVerifier("https://accounts.google.com", oidc.NewRemoteKeySet(context.Background(), server.URL+"/keys"), &oidc.Config{ClientID: "client"})}
	if a, e := p.Exchange(context.Background(), "code", "verifier"); e != nil || !a.EmailVerified || a.Subject != "subject" {
		t.Fatalf("valid assertion rejected %v", e)
	}
	aud = "other"
	if _, e := p.Exchange(context.Background(), "code", "verifier"); e == nil {
		t.Fatal("wrong audience accepted")
	}
	aud = "client"
	badSignature = true
	if _, e := p.Exchange(context.Background(), "code", "verifier"); e == nil {
		t.Fatal("forgery accepted")
	}
	badSignature = false
	expires = time.Now().Add(-time.Minute).Unix()
	if _, e := p.Exchange(context.Background(), "code", "verifier"); e == nil {
		t.Fatal("expired assertion accepted")
	}
}
