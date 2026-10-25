package mailoutbox

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
)

const WebOrigin = "https://web.getportico.tv"

// BrandWordmarkURL is the wordmark drawn in both mail frames (account.html#18
// and #19, design assets/wordmark.svg, byte-identical). It points at the stable
// public brand path: the account web host emits the same file only as a
// content-hashed build asset, so a /assets/ URL there goes stale on every build
// and 404s (frames 18/19 draw a wordmark, not a broken image).
const BrandWordmarkURL = "https://getportico.tv/brand/portico-wordmark-white.svg"

// WebHost is the host of the configured public web origin. Mail templates
// must derive it from WebOrigin, never from a literal, so the origin-bound
// autofill line stays correct if the origin changes.
func WebHost() string {
	u, e := url.Parse(WebOrigin)
	if e != nil || u.Host == "" {
		return "web.getportico.tv"
	}
	return u.Host
}

const (
	ResetPath        = "/reset"
	VerifyEmailPath  = "/verify-email"
	JoinPath         = "/join"
	TogetherJoinPath = "/together/join"
	DevicePath       = "/device"
	AccountPath      = "/account"
	ClaimPath        = "/claim"
	MediaPath        = "/media/"
	DevicesPath      = "/account?tab=sessions"
)

// Verification links carry the resource binding with its code. The web route
// decodes this versioned tuple and invokes the existing bound verification API.
func VerificationToken(kind Kind, resource, code string) string {
	raw, _ := json.Marshal([]string{"1", string(kind), resource, code})
	return base64.RawURLEncoding.EncodeToString(raw)
}

// ParseVerificationToken reverses VerificationToken for the link-confirmation
// endpoint.
func ParseVerificationToken(token string) (kind Kind, resource, code string, err error) {
	raw, e := base64.RawURLEncoding.DecodeString(token)
	var parts []string
	if e != nil || len(raw) > 1024 || json.Unmarshal(raw, &parts) != nil || len(parts) != 4 || parts[0] != "1" {
		return "", "", "", errors.New("invalid verification token")
	}
	switch Kind(parts[1]) {
	case Registration, OIDCContact, EmailChange:
		return Kind(parts[1]), parts[2], parts[3], nil
	}
	return "", "", "", errors.New("invalid verification token")
}
func Link(d Delivery) string {
	switch d.Kind {
	case Registration, OIDCContact, EmailChange:
		return WebOrigin + VerifyEmailPath + "#token=" + url.QueryEscape(VerificationToken(d.Kind, d.ResourceID, d.Payload.Code))
	case PasswordReset:
		return WebOrigin + ResetPath + "#token=" + url.QueryEscape(d.Payload.Code)
	default:
		return WebOrigin + AccountPath
	}
}
