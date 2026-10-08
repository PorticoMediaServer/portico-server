package networking

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"portico.local/server/internal/dbwork"
	"strconv"
	"strings"
	"time"
)

// RouteIdentity is public information. Signing it grants no session or claim.
// A fresh client nonce proves possession of the already pinned private key.
type RouteIdentity struct {
	Kind        string    `json:"kind"`
	Version     string    `json:"version"`
	ServerID    string    `json:"serverId"`
	PublicKey   string    `json:"publicKey"`
	Fingerprint string    `json:"fingerprint"`
	BaseURL     string    `json:"baseUrl"`
	Nonce       string    `json:"nonce"`
	IssuedAt    time.Time `json:"issuedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}
type RouteIdentityHandler struct {
	keys     *ProtectedKeys
	runner   *AuthorityRunner
	slots    chan struct{}
	failures *failureLog
}

func NewRouteIdentityHandler(keys *ProtectedKeys, runner *AuthorityRunner) *RouteIdentityHandler {
	return &RouteIdentityHandler{keys, runner, make(chan struct{}, 32), sharedFailureLog}
}

// RouteOrigin is shared by owner configuration and the public identity proof.
// It deliberately accepts private LAN HTTP and never public plaintext.
func RouteOrigin(raw string) bool {
	if len(raw) > 300 || strings.ContainsAny(raw, "\\\r\n\t") || strings.ToLower(raw) != raw {
		return false
	}
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || !validEndpointPort(u) {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	h := u.Hostname()
	if h == "localhost" || strings.HasSuffix(h, ".local") {
		return true
	}
	ip, e := netip.ParseAddr(h)
	return e == nil && !ip.Is4In6() && ip.Zone() == "" && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}
func (h *RouteIdentityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		writeClaimFailure(w, 429, "probe_busy", "Connection verification is busy.", true)
		return
	}
	var q struct {
		BaseURL string `json:"baseUrl"`
		Nonce   string `json:"nonce"`
	}
	if readClaimBody(w, r, &q) != nil || !RouteOrigin(q.BaseURL) {
		writeClaimFailure(w, 400, "invalid_probe", "Invalid connection verification request.", false)
		return
	}
	nonce, e := base64.RawURLEncoding.Strict().DecodeString(q.Nonce)
	u, _ := url.Parse(q.BaseURL)
	if e != nil || len(nonce) != 32 || base64.RawURLEncoding.EncodeToString(nonce) != q.Nonce || u.Host != strings.ToLower(r.Host) {
		writeClaimFailure(w, 400, "invalid_probe", "Invalid connection verification request.", false)
		return
	}
	if u.Scheme == "http" && !localHTTPConnection(rRemoteAddr(r.RemoteAddr)) {
		writeClaimFailure(w, 403, "tls_required", "Public connections require HTTPS.", false)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	var out struct {
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}
	e = h.runner.Do(ctx, func(ctx context.Context) error {
		gated, e := dbwork.BeginSnapshot(ctx, h.keys.db)
		if e != nil {
			return stepFailure("snapshot", e)
		}
		tx := gated.Tx()
		defer gated.Rollback()
		id, e := CurrentIdentityTx(ctx, tx)
		if e != nil {
			return stepFailure("identity row", e)
		}
		key, e := h.keys.read(id.KeyIncarnation)
		if e != nil {
			return stepFailure("key file", e)
		}
		defer clear(key)
		if !bytes.Equal(key.Public().(ed25519.PublicKey), id.PublicKey) {
			return stepFailure("key mismatch", ErrStale)
		}
		fp := sha256.Sum256(id.PublicKey)
		now := time.Now().UTC()
		raw, e := json.Marshal(RouteIdentity{"portico.route-proof", "1", id.ServerID, base64.RawURLEncoding.EncodeToString(id.PublicKey), base64.RawURLEncoding.EncodeToString(fp[:]), q.BaseURL, q.Nonce, now, now.Add(30 * time.Second)})
		if e != nil {
			return e
		}
		signature := ed25519.Sign(key, raw)
		out.Payload = base64.RawURLEncoding.EncodeToString(raw)
		out.Signature = base64.RawURLEncoding.EncodeToString(signature)
		return stepFailure("commit", commitClaimTx(ctx, func() error {
			w.Header().Set("Content-Type", "application/json")
			return json.NewEncoder(w).Encode(out)
		}))
	})
	if e != nil {
		h.failures.report(routeLabel(r, "POST /v1/networking/identity-proof"), 503, r.Context(), e)
		writeClaimFailure(w, 503, "identity_unavailable", "Server identity verification is temporarily unavailable.", true)
		return
	}
}
func rRemoteAddr(raw string) net.Addr {
	a, e := net.ResolveTCPAddr("tcp", raw)
	if e != nil {
		return &net.TCPAddr{}
	}
	return a
}
func routeURL(ip netip.Addr, port int, scheme string) string {
	host := ip.String()
	if ip.Is6() {
		host = "[" + host + "]"
	}
	if scheme == "https" && port == 443 || scheme == "http" && port == 80 {
		return scheme + "://" + host
	}
	return scheme + "://" + net.JoinHostPort(ip.String(), strconv.Itoa(port))
}

// ChallengeProof is this server's signed answer to a Portico challenge
// request (POST /v1/direct/portico-challenge): the challenge Hosted is to sign
// into an identity assertion, the client's nonce and installation, under the
// server's pinned identity key. A client verifies it against the server id it
// meant to reach (the id is the key's own digest) before it asks Hosted for an
// assertion, so a server that does not hold that key cannot collect one for
// it (INT M7).
type ChallengeProof struct {
	Kind           string    `json:"kind"`
	Version        string    `json:"version"`
	ServerID       string    `json:"serverId"`
	PublicKey      string    `json:"publicKey"`
	Fingerprint    string    `json:"fingerprint"`
	Challenge      string    `json:"challenge"`
	Nonce          string    `json:"nonce"`
	InstallationID string    `json:"installationId"`
	IssuedAt       time.Time `json:"issuedAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

// SignChallenge signs a ChallengeProof with the current identity key. The
// nonce is the client's: 32 random bytes, base64url.
func (h *RouteIdentityHandler) SignChallenge(ctx context.Context, challenge, nonce, installation string, expires time.Time) (payload, signature string, err error) {
	raw, e := base64.RawURLEncoding.Strict().DecodeString(nonce)
	if e != nil || len(raw) != 32 || challenge == "" || len(installation) > 128 {
		return "", "", ErrInvalid
	}
	err = h.runner.Do(ctx, func(ctx context.Context) error {
		gated, e := dbwork.BeginSnapshot(ctx, h.keys.db)
		if e != nil {
			return e
		}
		tx := gated.Tx()
		defer gated.Rollback()
		id, e := CurrentIdentityTx(ctx, tx)
		if e != nil {
			return e
		}
		key, e := h.keys.read(id.KeyIncarnation)
		if e != nil {
			return e
		}
		defer clear(key)
		if !bytes.Equal(key.Public().(ed25519.PublicKey), id.PublicKey) {
			return ErrStale
		}
		fp := sha256.Sum256(id.PublicKey)
		body, e := json.Marshal(ChallengeProof{"portico.challenge-proof", "1", id.ServerID, base64.RawURLEncoding.EncodeToString(id.PublicKey), base64.RawURLEncoding.EncodeToString(fp[:]), challenge, nonce, installation, time.Now().UTC(), expires.UTC()})
		if e != nil {
			return e
		}
		payload = base64.RawURLEncoding.EncodeToString(body)
		signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, body))
		return nil
	})
	return payload, signature, err
}
