// Package appauth defines public-client sign-in to one user-run server.
package appauth

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"portico.local/hosted/internal/directory"
)

var ErrGrant = errors.New("invalid app authorization")

const RequestLifetime = 5 * time.Minute
const CodeLifetime = time.Minute

// Request is immutable after review. Only S256 PKCE is supported.
type Request struct {
	ServerID      string `json:"serverId"`
	Challenge     string `json:"challenge"`
	RedirectURI   string `json:"redirectUri"`
	State         string `json:"state"`
	AppName       string `json:"appName"`
	CodeChallenge string `json:"codeChallenge"`
}
type Review struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"accountId"`
	AccountName string    `json:"accountName"`
	ServerName  string    `json:"serverName"`
	AppName     string    `json:"appName"`
	RedirectURI string    `json:"redirectUri"`
	ExpiresAt   time.Time `json:"expiresAt"`
}
type Decision struct {
	AccountID string `json:"accountId"`
	Approve   bool   `json:"approve"`
}
type Return struct {
	Location string `json:"location"`
}
type Exchange struct {
	Code        string `json:"code"`
	Verifier    string `json:"verifier"`
	RedirectURI string `json:"redirectUri"`
}
type Grant struct {
	Assertion directory.SignedPolicy `json:"assertion"`
}

var serverID = regexp.MustCompile(`^srv_[A-Za-z0-9_-]{43}$`)
var verifier = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
var appScheme = regexp.MustCompile(`^[a-z][a-z0-9+.-]{1,63}$`)

func (q Request) Validate() error {
	if !serverID.MatchString(q.ServerID) || !base64Bytes(q.ServerID[4:], 32) || !serverChallenge(q.Challenge) || !base64Bytes(q.CodeChallenge, 32) || !plain(q.State, 16, 512) || !plain(q.AppName, 1, 100) || strings.TrimSpace(q.AppName) != q.AppName || !ValidRedirect(q.RedirectURI) {
		return ErrGrant
	}
	return nil
}
func base64Bytes(v string, n int) bool {
	raw, e := base64.RawURLEncoding.Strict().DecodeString(v)
	return e == nil && len(raw) == n && base64.RawURLEncoding.EncodeToString(raw) == v
}

// serverChallenge is the challenge the chosen server issued to the app. Its
// size is the server's to choose (24 bytes today), so the bound here is the one
// the assertion issuer and the server's verifier apply: 22 to 128 characters.
func serverChallenge(v string) bool {
	raw, e := base64.RawURLEncoding.Strict().DecodeString(v)
	return e == nil && len(raw) >= 16 && len(raw) <= 96 && base64.RawURLEncoding.EncodeToString(raw) == v
}
func plain(v string, min, max int) bool {
	if len(v) < min || len(v) > max || !utf8.ValidString(v) {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

// ValidRedirect admits exact HTTPS URLs, numeric loopback HTTP URLs and
// hierarchical custom app schemes. It never resolves or fetches the address.
// Query strings are allowed except names reserved for the authorization return.
func ValidRedirect(raw string) bool {
	if !plain(raw, 1, 2048) || strings.ContainsAny(raw, "\\ ") {
		return false
	}
	for _, r := range raw {
		if r > 127 {
			return false
		}
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme == "" || u.Opaque != "" || u.User != nil || u.Fragment != "" || strings.Contains(raw, "#") || u.ForceQuery || strings.HasSuffix(u.Host, ":") {
		return false
	}
	query, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return false
	}
	for _, key := range []string{"code", "state", "error"} {
		if _, ok := query[key]; ok {
			return false
		}
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 || strconv.Itoa(n) != p {
			return false
		}
	}
	switch u.Scheme {
	case "https":
		if u.Host == "" || u.Hostname() == "" || strings.Contains(u.Hostname(), "%") {
			return false
		}
		host := u.Hostname()
		if ip, e := netip.ParseAddr(host); e == nil {
			return ip.Zone() == ""
		}
		if len(host) > 253 || strings.HasSuffix(host, ".") {
			return false
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return false
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
					return false
				}
			}
		}
		return true
	case "http":
		ip, e := netip.ParseAddr(u.Hostname())
		return e == nil && ip.IsLoopback() && ip.Zone() == "" && !ip.Is4In6() && u.Port() != ""
	default:
		if !appScheme.MatchString(u.Scheme) {
			return false
		}
		switch u.Scheme {
		case "javascript", "data", "file", "ftp", "blob", "about", "intent", "mailto", "tel", "sms", "ws", "wss":
			return false
		}
		return u.Port() == "" && (u.Host != "" || strings.HasPrefix(u.Path, "/"))
	}
}
func (q Exchange) Validate() error {
	if !strings.HasPrefix(q.Code, "pac_") || !base64Bytes(strings.TrimPrefix(q.Code, "pac_"), 32) || !verifier.MatchString(q.Verifier) || !ValidRedirect(q.RedirectURI) {
		return ErrGrant
	}
	return nil
}
func Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// ReturnLocation is the address the person was shown, exactly as the app gave
// it, with the answer added after it. ValidRedirect has already refused a
// fragment and a bare trailing "?", so appending is always well formed.
func ReturnLocation(q Request, code string) string {
	answer := "error=access_denied"
	if code != "" {
		answer = "code=" + url.QueryEscape(code)
	}
	separator := "?"
	if strings.Contains(q.RedirectURI, "?") {
		separator = "&"
	}
	return q.RedirectURI + separator + answer + "&state=" + url.QueryEscape(q.State)
}
