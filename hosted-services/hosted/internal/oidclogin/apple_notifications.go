package oidclogin

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const AppleIssuer = "https://appleid.apple.com"
const AppleNotificationMaxBytes = 24 << 10

// AppleNotification contains only verified event facts, never credentials.
type AppleNotification struct {
	ID, Audience, Type, Subject, Email string
	EventTime                          time.Time
}

type AppleNotificationVerifier struct {
	verifiers map[string]*oidc.IDTokenVerifier
}

func NewAppleNotificationVerifier(keys oidc.KeySet, audiences []string) *AppleNotificationVerifier {
	v := &AppleNotificationVerifier{verifiers: map[string]*oidc.IDTokenVerifier{}}
	for _, audience := range audiences {
		// Apple documents event JWTs without exp. Check exp ourselves when supplied.
		v.verifiers[audience] = oidc.NewVerifier(AppleIssuer, keys, &oidc.Config{ClientID: audience, SupportedSigningAlgs: []string{"RS256"}, SkipExpiryCheck: true})
	}
	return v
}

func (v *AppleNotificationVerifier) Verify(ctx context.Context, raw string) (AppleNotification, error) {
	var out AppleNotification
	reject := errors.New("Apple account notification rejected")
	if v == nil || len(raw) == 0 || len(raw) > AppleNotificationMaxBytes {
		return out, reject
	}
	var token *oidc.IDToken
	for audience, verifier := range v.verifiers {
		t, err := verifier.Verify(ctx, raw)
		if err == nil && len(t.Audience) == 1 && t.Audience[0] == audience {
			token = t
			break
		}
	}
	if token == nil {
		return out, reject
	}
	var claims struct {
		ID        string          `json:"jti"`
		Issued    int64           `json:"iat"`
		Expires   *int64          `json:"exp"`
		NotBefore *int64          `json:"nbf"`
		Events    json.RawMessage `json:"events"`
	}
	if token.Claims(&claims) != nil {
		return out, reject
	}
	now := time.Now().Unix()
	if len(claims.ID) == 0 || len(claims.ID) > 255 || claims.Issued <= 0 || claims.Issued > now+60 || (claims.Expires != nil && (*claims.Expires <= now || *claims.Expires < claims.Issued)) || (claims.NotBefore != nil && *claims.NotBefore > now+60) {
		return out, reject
	}
	events := claims.Events
	if len(events) > 0 && events[0] == '"' {
		var encoded string
		if json.Unmarshal(events, &encoded) != nil {
			return out, reject
		}
		events = []byte(encoded)
	}
	var event struct {
		Type    string          `json:"type"`
		Subject string          `json:"sub"`
		Email   string          `json:"email"`
		Private json.RawMessage `json:"is_private_email"`
		Time    int64           `json:"event_time"`
	}
	if json.Unmarshal(events, &event) != nil || len(event.Subject) == 0 || len(event.Subject) > 255 || event.Time <= 0 || event.Time > claims.Issued+60 || event.Time > now+60 {
		return out, reject
	}
	switch event.Type {
	case "email-enabled", "email-disabled":
		address, err := mail.ParseAddress(event.Email)
		if err != nil || address.Address != event.Email || len(event.Email) > 320 || !strings.HasSuffix(strings.ToLower(event.Email), "@privaterelay.appleid.com") || (string(event.Private) != "true" && string(event.Private) != `"true"`) {
			return out, reject
		}
	case "account-delete":
		event.Type = "account-deleted"
		event.Email = ""
	case "consent-revoked", "account-deleted":
		event.Email = ""
	default:
		return out, reject
	}
	return AppleNotification{ID: claims.ID, Audience: token.Audience[0], Type: event.Type, Subject: event.Subject, Email: strings.ToLower(event.Email), EventTime: time.Unix(event.Time, 0).UTC()}, nil
}
