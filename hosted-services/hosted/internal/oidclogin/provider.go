// Package oidclogin verifies provider assertions. Providers never issue Portico credentials.
package oidclogin

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Config struct {
	WebOrigin                  string   `json:"webOrigin"`
	GoogleClientID             string   `json:"googleClientId"`
	GoogleClientSecret         string   `json:"googleClientSecret"`
	AppleClientID              string   `json:"appleClientId"`
	AppleNotificationAudiences []string `json:"appleNotificationAudiences"`
	AppleNativeClientID        string   `json:"appleNativeClientId"`
	AppleNativeClientIDs       []string `json:"appleNativeClientIds"`
	AppleTeamID                string   `json:"appleTeamId"`
	AppleKeyID                 string   `json:"appleKeyId"`
	AppleKeyFile               string   `json:"appleKeyFile"`
}
type Provider struct {
	AppleNotifications             *AppleNotificationVerifier
	ID, Issuer, Audience, Callback string
	NativeAudience                 string
	NativeAudiences                []string
	oauth                          oauth2.Config
	verifier                       *oidc.IDTokenVerifier
	nativeVerifiers                map[string]*oidc.IDTokenVerifier
	apple                          *ecdsa.PrivateKey
	team, keyID                    string
	client                         *http.Client
}
type Assertion struct {
	Issuer, Subject, Audience, Email string
	EmailVerified, PrivateRelay      bool
	IssuedAt                         time.Time
	Nonce                            string
}

func Load(ctx context.Context, path string) (Config, map[string]*Provider, error) {
	var c Config
	out := map[string]*Provider{}
	if path == "" {
		return c, out, nil
	}
	info, e := os.Stat(path)
	if e != nil {
		return c, nil, e
	}
	if info.Mode().Perm()&0077 != 0 {
		return c, nil, errors.New("OIDC configuration must be private")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return c, nil, e
	}
	if e = json.Unmarshal(raw, &c); e != nil {
		return c, nil, e
	}
	if c.WebOrigin != "https://web.getportico.tv" {
		return c, nil, errors.New("unexpected account origin")
	}
	if c.AppleNativeClientID != "" {
		c.AppleNativeClientIDs = append(c.AppleNativeClientIDs, c.AppleNativeClientID)
	}
	unique := map[string]bool{}
	ids := []string{}
	for _, id := range c.AppleNativeClientIDs {
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{2,254}$`).MatchString(id) {
			return c, nil, errors.New("invalid Apple native bundle identifier")
		}
		if !unique[id] {
			unique[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > 8 || (len(ids) > 0 && c.AppleClientID == "") {
		return c, nil, errors.New("invalid Apple native configuration")
	}
	if len(c.AppleNotificationAudiences) > 8 || (len(c.AppleNotificationAudiences) > 0 && c.AppleClientID == "") {
		return c, nil, errors.New("invalid Apple notification configuration")
	}
	for _, id := range c.AppleNotificationAudiences {
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{2,254}$`).MatchString(id) {
			return c, nil, errors.New("invalid Apple notification audience")
		}
	}
	sort.Strings(ids)
	c.AppleNativeClientIDs = ids
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("provider redirect refused") }}
	ctx = oidc.ClientContext(ctx, client)
	for _, id := range []string{"google", "apple"} {
		p := &Provider{ID: id, Callback: c.WebOrigin + "/auth/sso/" + id + "/callback", client: client}
		secret := ""
		authURL := ""
		tokenURL := ""
		jwks := ""
		if id == "google" {
			if c.GoogleClientID == "" {
				continue
			}
			p.Issuer = "https://accounts.google.com"
			p.Audience = c.GoogleClientID
			secret = c.GoogleClientSecret
			authURL = "https://accounts.google.com/o/oauth2/v2/auth"
			tokenURL = "https://oauth2.googleapis.com/token"
			jwks = "https://www.googleapis.com/oauth2/v3/certs"
			if secret == "" {
				return c, nil, errors.New("Google secret missing")
			}
		} else {
			if c.AppleClientID == "" {
				continue
			}
			p.Issuer = "https://appleid.apple.com"
			p.Audience = c.AppleClientID
			p.NativeAudiences = append([]string(nil), c.AppleNativeClientIDs...)
			p.NativeAudience = strings.Join(p.NativeAudiences, ",")
			p.team = c.AppleTeamID
			p.keyID = c.AppleKeyID
			authURL = "https://appleid.apple.com/auth/authorize"
			tokenURL = "https://appleid.apple.com/auth/token"
			jwks = "https://appleid.apple.com/auth/keys"
			keyInfo, e := os.Stat(c.AppleKeyFile)
			if e != nil || !keyInfo.Mode().IsRegular() || keyInfo.Mode().Perm()&0077 != 0 {
				return c, nil, errors.New("Apple signing key must be private")
			}
			key, e := os.ReadFile(c.AppleKeyFile)
			if e != nil {
				return c, nil, e
			}
			block, _ := pem.Decode(key)
			if block == nil {
				return c, nil, errors.New("Apple key missing")
			}
			parsed, e := x509.ParsePKCS8PrivateKey(block.Bytes)
			if e != nil {
				return c, nil, e
			}
			var ok bool
			p.apple, ok = parsed.(*ecdsa.PrivateKey)
			if !ok || p.apple.Curve.Params().BitSize != 256 || p.team == "" || p.keyID == "" {
				return c, nil, errors.New("invalid Apple signing configuration")
			}
		}
		p.oauth = oauth2.Config{ClientID: p.Audience, ClientSecret: secret, RedirectURL: p.Callback, Scopes: []string{"openid", "email"}, Endpoint: oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInParams}}
		if id == "apple" {
			p.oauth.Scopes = []string{"email", "name"}
		}
		p.verifier = oidc.NewVerifier(p.Issuer, oidc.NewRemoteKeySet(ctx, jwks), &oidc.Config{ClientID: p.Audience, SupportedSigningAlgs: []string{"RS256"}})
		if id == "apple" && len(c.AppleNotificationAudiences) > 0 {
			p.AppleNotifications = NewAppleNotificationVerifier(oidc.NewRemoteKeySet(ctx, jwks), c.AppleNotificationAudiences)
		}
		p.nativeVerifiers = map[string]*oidc.IDTokenVerifier{}
		nativeKeys := oidc.NewRemoteKeySet(ctx, jwks)
		for _, audience := range p.NativeAudiences {
			p.nativeVerifiers[audience] = oidc.NewVerifier(p.Issuer, nativeKeys, &oidc.Config{ClientID: audience, SupportedSigningAlgs: []string{"RS256"}})
		}
		out[id] = p
	}
	return c, out, nil
}
func (p *Provider) Authorization(state, nonce, verifier string) string {
	options := []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("nonce", nonce)}
	if p.ID == "google" {
		options = append(options, oauth2.S256ChallengeOption(verifier))
	} else {
		options = append(options, oauth2.SetAuthURLParam("response_mode", "form_post"))
	}
	return p.oauth.AuthCodeURL(state, options...)
}
func (p *Provider) Exchange(ctx context.Context, code, verifier string) (Assertion, error) {
	var a Assertion
	c := p.oauth
	if p.apple != nil {
		secret, e := p.appleSecret()
		if e != nil {
			return a, e
		}
		c.ClientSecret = secret
	}
	opts := []oauth2.AuthCodeOption{}
	if p.ID == "google" {
		opts = append(opts, oauth2.VerifierOption(verifier))
	}
	ctx = oidc.ClientContext(ctx, p.client)
	token, e := c.Exchange(ctx, code, opts...)
	if e != nil {
		return a, errors.New("provider exchange failed")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok || len(raw) > 16384 {
		return a, errors.New("provider ID token absent")
	}
	return p.verify(ctx, raw, p.verifier, p.Audience)
}
func (p *Provider) VerifyNative(ctx context.Context, raw string) (Assertion, error) {
	if p.ID != "apple" || len(p.nativeVerifiers) == 0 {
		return Assertion{}, errors.New("native Apple sign-in is not configured")
	}
	for _, audience := range p.NativeAudiences {
		assertion, err := p.verify(oidc.ClientContext(ctx, p.client), raw, p.nativeVerifiers[audience], audience)
		if err == nil {
			return assertion, nil
		}
	}
	return Assertion{}, errors.New("native Apple assertion rejected")
}
func (p *Provider) verify(ctx context.Context, raw string, verifier *oidc.IDTokenVerifier, audience string) (Assertion, error) {
	var a Assertion
	if len(raw) == 0 || len(raw) > 16384 {
		return a, errors.New("invalid provider token")
	}
	verified, e := verifier.Verify(ctx, raw)
	if e != nil || len(verified.Audience) != 1 || verified.Audience[0] != audience {
		return a, errors.New("provider token rejected")
	}
	var claims struct {
		Email         string          `json:"email"`
		EmailVerified json.RawMessage `json:"email_verified"`
		PrivateRelay  json.RawMessage `json:"is_private_email"`
		Nonce         string          `json:"nonce"`
		Iat           int64           `json:"iat"`
	}
	if e = verified.Claims(&claims); e != nil {
		return a, e
	}
	if len(verified.Subject) == 0 || len(verified.Subject) > 255 || claims.Nonce == "" || claims.Iat < time.Now().Add(-10*time.Minute).Unix() || claims.Iat > time.Now().Add(time.Minute).Unix() {
		return a, errors.New("provider assertion outside sign-in window")
	}
	truth := func(v json.RawMessage) bool { return string(v) == "true" || string(v) == `"true"` }
	email := strings.TrimSpace(claims.Email)
	valid := false
	if parsed, e := mail.ParseAddress(email); e == nil && parsed.Address == email && len(email) <= 320 {
		valid = truth(claims.EmailVerified)
	}
	return Assertion{p.Issuer, verified.Subject, audience, email, valid, truth(claims.PrivateRelay), time.Unix(claims.Iat, 0).UTC(), claims.Nonce}, nil
}
func (p *Provider) appleSecret() (string, error) {
	enc := base64.RawURLEncoding.EncodeToString
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": p.keyID, "typ": "JWT"})
	now := time.Now().Unix()
	body, _ := json.Marshal(map[string]any{"iss": p.team, "iat": now, "exp": now + 300, "aud": "https://appleid.apple.com", "sub": p.Audience})
	input := enc(header) + "." + enc(body)
	digest := sha256.Sum256([]byte(input))
	r, s, e := ecdsa.Sign(rand.Reader, p.apple, digest[:])
	if e != nil {
		return "", e
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return fmt.Sprintf("%s.%s", input, enc(sig)), nil
}
