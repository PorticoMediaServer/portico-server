package directory

// P09 routes are location evidence, not membership or playback authority.
import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type RouteCandidate struct {
	BaseURL    string `json:"baseUrl"`
	Class      string `json:"class"`
	Quality    string `json:"quality,omitempty"`
	Generation int64  `json:"generation,string,omitempty"`
}
type RouteObservation struct {
	Generation    int64            `json:"generation,string"`
	RemoteEnabled bool             `json:"remoteEnabled"`
	CGNAT         bool             `json:"cgnat"`
	Candidates    []RouteCandidate `json:"candidates"`
}
type RouteObservationReceipt struct {
	Generation int64 `json:"generation,string"`
}
type RouteSet struct {
	Generation                            int64
	ClaimGeneration, CredentialGeneration int64
	Candidates                            []RouteCandidate
}
type RouteDocument struct {
	Kind                 string           `json:"kind"`
	Version              string           `json:"version"`
	Audience             string           `json:"audience"`
	ServerID             string           `json:"serverId"`
	Name                 string           `json:"name"`
	PublicKey            string           `json:"publicKey"`
	Fingerprint          string           `json:"fingerprint"`
	PolicyRevision       int64            `json:"policyRevision"`
	ClaimGeneration      int64            `json:"claimGeneration,string"`
	CredentialGeneration int64            `json:"credentialGeneration,string"`
	Generation           int64            `json:"generation,string"`
	Candidates           []RouteCandidate `json:"candidates"`
	IssuedAt             time.Time        `json:"issuedAt"`
	ExpiresAt            time.Time        `json:"expiresAt"`
	Algorithm            string           `json:"algorithm"`
	KeyID                string           `json:"keyId"`
}
type RouteRepository interface {
	ObserveRoutes(context.Context, ServerAuthority, RouteObservation, []byte) error
	RouteSets(context.Context, string) (map[string]RouteSet, error)
	RecordRouteFailure(context.Context, ServerAuthority, int64, string, string) error
}

// devLoopbackRoutes admits a loopback LAN route; only a devtrust development
// build sets it (dev_loopback_route.go).
var devLoopbackRoutes bool

func validRouteCandidate(c RouteCandidate) bool {
	if c.Quality != "" || c.Generation != 0 || len(c.BaseURL) > 300 {
		return false
	}
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || strings.ToLower(c.BaseURL) != c.BaseURL || strings.ContainsAny(c.BaseURL, "\\\r\n\t") || strings.HasSuffix(u.Host, ":") {
		return false
	}
	if u.Scheme == "https" && u.Port() == "443" || u.Scheme == "http" && u.Port() == "80" {
		return false
	}
	if u.Port() != "" {
		p, e := strconv.Atoi(u.Port())
		if e != nil || p < 1 || p > 65535 || strconv.Itoa(p) != u.Port() {
			return false
		}
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if c.Class == "lan" {
		// Hosted never resolves or probes a LAN report. Only explicit private literals
		// are distributed; .local names belong to native discovery, not public DNS.
		return err == nil && !ip.Is4In6() && (ip.IsPrivate() || ip.IsLinkLocalUnicast() || devLoopbackRoutes && ip.IsLoopback()) && ip.Zone() == "" && (u.Scheme == "http" || u.Scheme == "https")
	}
	if c.Class != "public" && c.Class != "manual" {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	if err == nil {
		return publicRouteIP(ip)
	}
	return !strings.HasSuffix(u.Hostname(), ".local") && u.Hostname() != "localhost" && strings.Contains(u.Hostname(), ".")
}
func publicRouteIP(ip netip.Addr) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.Is4In6() {
		return false
	}
	for _, p := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32"} {
		if netip.MustParsePrefix(p).Contains(ip) {
			return false
		}
	}
	return true
}
func (s *Service) ObserveRoutes(ctx context.Context, a ServerAuthority, q RouteObservation) (RouteObservationReceipt, error) {
	repo, ok := s.repo.(RouteRepository)
	if !ok || !a.Valid() {
		return RouteObservationReceipt{}, ErrDenied
	}
	if q.Generation < 1 || q.Candidates == nil || len(q.Candidates) > 12 {
		return RouteObservationReceipt{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, c := range q.Candidates {
		if !validRouteCandidate(c) || seen[c.BaseURL] || (!q.RemoteEnabled && c.Class != "lan") {
			return RouteObservationReceipt{}, ErrInvalid
		}
		seen[c.BaseURL] = true
	}
	raw, _ := json.Marshal(q)
	digest := sha256.Sum256(raw)
	if e := repo.ObserveRoutes(ctx, a, q, digest[:]); e != nil {
		return RouteObservationReceipt{}, e
	}
	return RouteObservationReceipt{q.Generation}, nil
}
func (s *Service) signRoutes(account string, server Server, set RouteSet) (*SignedPolicy, error) {
	if set.Generation == 0 {
		return nil, nil
	}
	key, e := base64.RawURLEncoding.Strict().DecodeString(server.PublicKey)
	if e != nil || len(key) != 32 {
		return nil, ErrInvalid
	}
	fingerprint := sha256.Sum256(key)
	now := time.Now().UTC()
	candidates := set.Candidates
	if candidates == nil {
		candidates = []RouteCandidate{}
	}
	d := RouteDocument{"portico.routes", "1", account, server.ID, server.Name, server.PublicKey, base64.RawURLEncoding.EncodeToString(fingerprint[:]), server.PolicyRevision, set.ClaimGeneration, set.CredentialGeneration, set.Generation, candidates, now, now.Add(45 * time.Minute), "Ed25519", s.keys.KeyID}
	raw, e := json.Marshal(d)
	if e != nil {
		return nil, e
	}
	out, e := s.keys.Sign(raw)
	if e != nil {
		return nil, e
	}
	return &out, nil
}
func (s *Service) Routes(ctx context.Context, account, serverID string) (*SignedPolicy, error) {
	// List performs the current account/membership/restriction projection. A route
	// endpoint is not a server-ID-only lookup, including after membership removal.
	servers, e := s.List(ctx, account)
	if e != nil {
		return nil, e
	}
	for _, server := range servers {
		if server.ID == serverID && server.Routes != nil {
			return server.Routes, nil
		}
	}
	return nil, ErrDenied
}
