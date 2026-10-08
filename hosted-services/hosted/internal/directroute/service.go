// Package directroute records each server's current public address and names its one
// permanent route, only while the exact captured installed and acknowledged current server
// authority is transactionally valid. A route name is not a certificate or an
// endpoint-verification receipt.
//
// The name is `c-<label>.<namespace>.direct.getportico.tv`. The namespace is permanent for
// the server's key; the members-only label is replaced whenever a member is removed (Hosted
// migration 0012), so a former member's name stops resolving. The DNS authority answers it
// from a small table it reads from here (Changes), so a WAN address change reaches clients
// through DNS and costs this service one request, from the server that moved.
package directroute

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"portico.local/dns/directdns"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
)

var ErrUnverified = errors.New("direct address requires source match or dial-back proof")
var ErrInvalid = errors.New("invalid direct route request")

// Zone is the public DNS zone server routes are named in.
const Zone = "direct.getportico.tv"

// AddressChange is one row of the DNS feed. Sequence is strictly increasing in commit order,
// so a reader that remembers the last one it applied never misses a change.
type AddressChange struct {
	Label string `json:"label"`
	// PreviousLabel keeps resolving until PreviousLabelUntil (RFC 3339) after a
	// member removal rotated Label (A86).
	PreviousLabel      string `json:"previousLabel,omitempty"`
	PreviousLabelUntil string `json:"previousLabelUntil,omitempty"`
	LANIPv4            string `json:"lanIpv4,omitempty"`
	LANIPv6            string `json:"lanIpv6,omitempty"`
	Sequence           int64  `json:"sequence,string"`
	Namespace          string `json:"namespace"`
	IPv4               string `json:"ipv4,omitempty"`
	IPv6               string `json:"ipv6,omitempty"`
}

type Repository interface {
	// RecordDirectAddress stores the address for the server's namespace under the current
	// account/server/operation locks and reports whether it changed.
	RecordDirectAddress(ctx context.Context, a directory.ServerAuthority, address netip.Addr) (namespace string, changed bool, err error)
	DirectRouteLabel(context.Context, string) (string, error)
	// WithdrawDirectAddress clears one address family for the server's namespace under the
	// same locks and reports whether anything was there to clear. A server whose ISP stops
	// handing out (say) IPv6 says so once; nothing here is polled.
	WithdrawDirectAddress(ctx context.Context, a directory.ServerAuthority, family string) (namespace string, changed bool, err error)
	DirectAddressChanges(ctx context.Context, after int64, limit int) ([]AddressChange, error)
}

// IPv4 and IPv6 are the two address families a server can report or withdraw.
const (
	FamilyIPv4 = "ipv4"
	FamilyIPv6 = "ipv6"
)

// ValidFamily is the whole of the wire vocabulary for a withdrawal.
func ValidFamily(family string) bool { return family == FamilyIPv4 || family == FamilyIPv6 }

type Service struct {
	repo    Repository
	feedKey []byte
	// feedHead caches the highest feed sequence seen by a completed (non-truncated) read,
	// so an unchanged feed answers 304 with no database read. Writes observed through this
	// service invalidate it (lastWrite); writes that bypass it (LAN reports, label rotations
	// via triggers) age out via feedHeadTTL. head only ever moves forward.
	mu           sync.Mutex
	head         int64
	headAt       time.Time
	headComplete bool
	headValid    bool
	lastWrite    time.Time
}

// feedHeadTTL bounds staleness from writes that bypass this service. WAN writes through
// Issue/Withdraw invalidate immediately, so a newly claimed server still reaches the DNS
// nodes within one poll interval (30–60 s).
const feedHeadTTL = 30 * time.Second
type Route struct {
	Hostname           string `json:"hostname"`
	BaseURL            string `json:"baseUrl"`
	Namespace          string `json:"namespace"`
	CertificateDNSName string `json:"certificateDnsName"`
	DNS01Target        string `json:"dns01Target"`
}

// New takes the secret the DNS nodes present to read the address feed.
func New(repo Repository, feedKey []byte) (*Service, error) {
	if repo == nil || len(feedKey) < 32 {
		return nil, ErrInvalid
	}
	return &Service{repo: repo, feedKey: append([]byte(nil), feedKey...)}, nil
}
func (s *Service) Issue(ctx context.Context, a directory.ServerAuthority, address string) (Route, error) {
	return s.IssuePort(ctx, a, address, 32500)
}
func (s *Service) IssuePort(ctx context.Context, a directory.ServerAuthority, address string, port int) (Route, error) {
	if port < 1 || port > 65535 {
		return Route{}, ErrInvalid
	}
	if !a.Valid() {
		return Route{}, directory.ErrDenied
	}
	derived, err := claims.ServerID(a.PublicKey)
	if err != nil || a.ServerID != derived {
		return Route{}, directory.ErrDenied
	}
	ip, e := netip.ParseAddr(address)
	if e != nil || ip.String() != address || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.Is4In6() || netip.MustParsePrefix("100.64.0.0/10").Contains(ip) {
		return Route{}, ErrInvalid
	}
	namespace, changed, e := s.repo.RecordDirectAddress(ctx, a, ip)
	if e != nil {
		return Route{}, e
	}
	if changed {
		s.invalidateFeedHead()
	}
	if !directdns.ValidNamespace(namespace) {
		return Route{}, ErrInvalid
	}
	label, e := s.repo.DirectRouteLabel(ctx, namespace)
	if e != nil {
		return Route{}, e
	}
	if !directdns.ValidMemberLabel(label) {
		return Route{}, ErrInvalid
	}
	name := directdns.CurrentHostname(namespace, Zone, label)
	baseURL := "https://" + name
	if port != 443 {
		baseURL += ":" + strconv.Itoa(port)
	}
	return Route{Hostname: name, BaseURL: baseURL, Namespace: namespace, CertificateDNSName: "*." + namespace + "." + Zone, DNS01Target: namespace + ".acme.getportico.tv"}, nil
}

// Withdraw clears one address family for a server. A server reports this once, when its own
// observer has been affirmatively unable to see the family for long enough to be sure it is
// gone; without it the DNS nodes would keep answering AAAA with a dead address and dual-stack
// clients would stall on it before falling back.
//
// It is idempotent: a family that is already absent is a success that changes nothing, so a
// lost response costs a repeat and never a spurious feed sequence.
func (s *Service) Withdraw(ctx context.Context, a directory.ServerAuthority, family string) (string, bool, error) {
	if !ValidFamily(family) {
		return "", false, ErrInvalid
	}
	if !a.Valid() {
		return "", false, directory.ErrDenied
	}
	derived, err := claims.ServerID(a.PublicKey)
	if err != nil || a.ServerID != derived {
		return "", false, directory.ErrDenied
	}
	namespace, changed, e := s.repo.WithdrawDirectAddress(ctx, a, family)
	if e != nil {
		return "", false, e
	}
	if changed {
		s.invalidateFeedHead()
	}
	if !directdns.ValidNamespace(namespace) {
		return "", false, ErrInvalid
	}
	return namespace, changed, nil
}

// FeedAuthorized compares in constant time; the feed discloses every server's address.
func (s *Service) FeedAuthorized(presented string) bool {
	return s != nil && subtle.ConstantTimeCompare([]byte(presented), s.feedKey) == 1
}

// invalidateFeedHead drops the cached head after a service-observed address write, so the
// next feed read goes to the database and picks up the new sequence.
func (s *Service) invalidateFeedHead() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.headValid = false
	s.lastWrite = time.Now()
}

// Changes returns up to limit address rows after a sequence, oldest first.
func (s *Service) Changes(ctx context.Context, after int64, limit int) ([]AddressChange, error) {
	items, _, _, err := s.ChangesWithCache(ctx, after, limit, "")
	return items, err
}

// ChangesWithCache is Changes plus conditional-request support. When the caller's
// If-None-Match matches the current head it reports notModified (HTTP 304) and, on a fresh
// cache hit, reaches no database at all. etag always names the head the response reflects:
// the last row's sequence when rows are returned, else the head (which equals after when the
// caller is synced).
func (s *Service) ChangesWithCache(ctx context.Context, after int64, limit int, ifNoneMatch string) (items []AddressChange, etag string, notModified bool, err error) {
	if after < 0 {
		return nil, "", false, ErrInvalid
	}
	if limit < 1 || limit > 5000 {
		limit = 5000
	}
	start := time.Now()
	s.mu.Lock()
	valid := s.headValid && time.Since(s.headAt) < feedHeadTTL
	head, complete := s.head, s.headComplete
	s.mu.Unlock()
	if valid && complete && after >= head {
		// No row can exist beyond after: head is a completed (non-truncated) observed head
		// and the caller is at or ahead of it. Answer without touching the database.
		etag = directdns.FeedETag(head)
		if directdns.MatchFeedETag(ifNoneMatch, head) {
			return nil, etag, true, nil
		}
		return []AddressChange{}, etag, false, nil
	}
	step, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, e := s.repo.DirectAddressChanges(step, after, limit)
	if e != nil {
		return nil, "", false, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if start.Before(s.lastWrite) {
		// A service-observed write landed during this read; the snapshot may miss it.
		// Return what the database showed without caching it or claiming a 304.
		if len(rows) == 0 {
			return []AddressChange{}, directdns.FeedETag(after), false, nil
		}
		return rows, directdns.FeedETag(rows[len(rows)-1].Sequence), false, nil
	}
	if after >= s.head || !s.headValid {
		// This read extends (or starts) the cache; a read from behind leaves it alone.
		if len(rows) == 0 {
			s.head, s.headAt, s.headComplete, s.headValid = after, time.Now(), true, true
		} else {
			last := rows[len(rows)-1].Sequence
			if last > s.head || !s.headValid {
				s.head = last
			}
			s.headAt = time.Now()
			s.headComplete = len(rows) < limit
			s.headValid = true
		}
	}
	if len(rows) == 0 {
		etag = directdns.FeedETag(after)
		if directdns.MatchFeedETag(ifNoneMatch, after) {
			return []AddressChange{}, etag, true, nil
		}
		return []AddressChange{}, etag, false, nil
	}
	etag = directdns.FeedETag(rows[len(rows)-1].Sequence)
	return rows, etag, false, nil
}

func (s *Service) IssuePortFrom(ctx context.Context, a directory.ServerAuthority, address string, port int, source string) (Route, error) {
	ip, err := netip.ParseAddr(address)
	if err != nil {
		return Route{}, ErrInvalid
	}
	peer, _ := netip.ParseAddr(source)
	if ip != peer.Unmap() {
		proof, ok := s.repo.(interface {
			DirectAddressVerified(context.Context, directory.ServerAuthority, netip.Addr, int) (bool, error)
		})
		if !ok {
			return Route{}, ErrUnverified
		}
		valid, e := proof.DirectAddressVerified(ctx, a, ip, port)
		if e != nil {
			return Route{}, e
		}
		if !valid {
			return Route{}, ErrUnverified
		}
	}
	return s.IssuePort(ctx, a, address, port)
}
