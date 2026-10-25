package directory

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"portico.local/hosted/internal/security"
	"portico.local/hosted/trust"
)

var ErrDenied = errors.New("not authorized")
var ErrConflict = errors.New("revision or operation conflict")
var ErrInvalid = errors.New("invalid directory input")

type ServerRole string

func (ServerRole) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"owner", "admin", "member"}}
}

type Server struct {
	Role             ServerRole    `json:"role"`
	LastSeenAt       time.Time     `json:"lastSeenAt"`
	Status           string        `json:"status"`
	OwnedByMe        bool          `json:"ownedByMe"`
	Routes           *SignedPolicy `json:"routes,omitempty"`
	EndpointVerified bool          `json:"endpointVerified"`
	ChallengeID      string        `json:"challengeId,omitempty"`
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	BaseURL          string        `json:"baseUrl"`
	PublicKey        string        `json:"publicKey"`
	PolicyRevision   int64         `json:"policyRevision"`
	Presence         Presence      `json:"presence"`
}

// Presence is when Hosted last heard from the server: its latest check-in
// (weekly when reachable, daily otherwise) or other authenticated call such as
// a start, clean shutdown or address change. It is "last seen", not a live
// online state: an app learns whether a server is up by contacting the server
// itself. Day-truncated for members who do not own the server; null if never
// seen. There is no periodic presence traffic to Hosted.
type Presence struct {
	LastSeenAt *time.Time `json:"lastSeenAt"`
}
type SignedPolicy = trust.Envelope

// Contact cadence. Hosted is contacted when something changed, not to prove liveness: a server
// Hosted can wake checks in weekly, one it cannot reach checks in daily (its only way to learn
// of an account-side change such as a departure).
const (
	CheckInWakeable      = 7 * 24 * time.Hour
	CheckInUnreachable   = 24 * time.Hour
	checkInPresenceSlack = 24 * time.Hour
)

// CheckIn returns the next check-in delay and the presence window that covers it.
func CheckIn(wakeable bool) (after, presence time.Duration) {
	after = CheckInUnreachable
	if wakeable {
		after = CheckInWakeable
	}
	return after, after + checkInPresenceSlack
}

// Heartbeat answers a check-in. There is no policy in it: membership is the
// server's own (Spec — Hosted at Scale).
type Heartbeat struct {
	// Root-signed current documents certificate, carried by existing check-ins.
	// Servers verify and persist its monotonic revocations before admission.
	// Null when no current root-verified certificate is available.
	SigningCertificate *trust.Certificate `json:"signingCertificate"`
	HeartbeatAfterMs   int64              `json:"heartbeatAfterMs"`
	PresenceExpiresAt  time.Time          `json:"presenceExpiresAt"`
	// RouteLabel is the server's current members-only label (A86). A member
	// removal rotates it, and the rotation wakes the server, whose check-in
	// then picks it up; there is no periodic contact.
	RouteLabel string `json:"routeLabel,omitempty"`
	// MemberSequence is the last membership push this server applied here;
	// MemberDigestMatches says whether the index equals the list the server
	// reported. Either one off and the server sends its full list.
	MemberSequence      int64 `json:"memberSequence"`
	MemberDigestMatches bool  `json:"memberDigestMatches"`
	// Departures are accounts that removed this server from their list here;
	// the server ends their membership and pushes the result, unless the
	// account joined again after leftAt.
	Departures []Departure `json:"departures"`
}

// Departure is one account leaving a server's list, and when.
type Departure struct {
	AccountID string    `json:"accountId"`
	LeftAt    time.Time `json:"leftAt"`
}

// CheckInReport is what a server says when it checks in: the wake it is
// answering (0 if none) and the state of its Portico Account member list.
type CheckInReport struct {
	WakeRevision   int64  `json:"wakeRevision"`
	MemberSequence int64  `json:"memberSequence"`
	MemberDigest   string `json:"memberDigest"`
}

// MemberStanding is one Portico Account's standing on a server, as the server
// reports it. Hosted indexes it for discovery and decides nothing with it.
// MembershipEnding is explicit server-authoritative evidence, not a grant.
// No invitation-ended cause exists until a real expiring access grant exists.
type MembershipEnding struct {
	Sequence  int64     `json:"sequence"`
	Cause     string    `json:"cause"`
	EndedAt   time.Time `json:"endedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type MembershipEndingNotice struct {
	Cause        string    `json:"cause"`
	ServerID     string    `json:"serverId"`
	MembershipID string    `json:"membershipId"`
	Sequence     int64     `json:"sequence"`
	ServerName   string    `json:"serverName,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
type MemberStanding struct {
	MembershipID string            `json:"membershipId,omitempty"`
	Ending       *MembershipEnding `json:"ending,omitempty"`
	AccountID    string            `json:"accountId"`
	Role         string            `json:"role"`
	Active       bool              `json:"active"`
	// Consent is the account's own identity assertion for this server, which
	// the server kept when the account signed in or accepted an invitation.
	Consent json.RawMessage `json:"consent,omitempty"`
	// ConsentAt is when a verified consent was issued; zero if there is none.
	// ConsentKind is its signed identity/registered-return kind, or CustodyKind
	// for an account accepting custody. These are set by the directory service,
	// never read from the wire.
	ConsentAt     time.Time `json:"-"`
	ConsentKind   string    `json:"-"`
	ConsentSigner string    `json:"-"`
}

// MemberDigest is the digest a server reports at check-in and Hosted computes
// over its index: "accountId:role" for each active member, byte-sorted,
// newline-joined, SHA-256, hex. It must equal the server's own MemberDigest
// (portico-server/server/internal/hosted).
func MemberDigest(members []MemberStanding) string {
	lines := make([]string, 0, len(members))
	for _, m := range members {
		if m.Active {
			lines = append(lines, m.AccountID+":"+m.Role)
		}
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// MemberPush is POST /v1/servers/{id}/members: the standing of every account
// the server's journal names after sequence After, up to Through, or (Full)
// every active member.
type MemberPush struct {
	After   int64 `json:"after"`
	Through int64 `json:"through"`
	Full    bool  `json:"full"`
	// A full list comes in pages from 0; More says another follows. The
	// sequence is recorded with the last page.
	Page    int              `json:"page,omitempty"`
	More    bool             `json:"more,omitempty"`
	Members []MemberStanding `json:"members"`
}

// MemberPushResult is the sequence Hosted now holds for the server and whether
// the push was applied (false: a gap; the server sends its full list).
type MemberPushResult struct {
	Sequence int64 `json:"sequence"`
	Applied  bool  `json:"applied"`
	// Skipped lists active members Hosted did not list: no consent it could
	// verify, or one older than the account's own departure (INT M6).
	Skipped []string `json:"skipped,omitempty"`
	// Custodian is the Portico Account that holds the server's claim here
	// (retires it, owns its certificates), or "" while custody is released
	// (INT M10). A Portico owner that is not the custodian accepts custody
	// with a custody assertion.
	Custodian string `json:"custodian"`
}

type Repository interface {
	List(context.Context, string) ([]Server, error)
	AuthenticateServer(context.Context, []byte) (string, error)
}
type Service struct {
	repo       Repository
	keys       *security.Keys
	proof      ProofClient
	proofSlots chan struct{}
	// The server-list version map (server_list.go), created on first use.
	versionsOnce sync.Once
	versions     *serverListVersions
}

func New(repo Repository, keys *security.Keys) *Service { return NewWithOptions(repo, keys, Options{}) }
func (s *Service) Keys() (string, string)               { return s.keys.KeyID, s.keys.PublicKey() }
func (s *Service) List(ctx context.Context, account string) ([]Server, error) {
	items, e := s.repo.List(ctx, account)
	if e != nil {
		return nil, e
	}
	if repo, ok := s.repo.(RouteRepository); ok {
		sets, e := repo.RouteSets(ctx, account)
		if e != nil {
			return nil, e
		}
		for i := range items {
			set, present := sets[items[i].ID]
			if !present {
				return nil, ErrDenied
			}
			items[i].Routes, e = s.signRoutes(account, items[i], set)
			if e != nil {
				return nil, e
			}
		}
	}
	return items, nil
}
func (s *Service) AuthenticateServer(ctx context.Context, token string) (string, error) {
	if len(token) < 20 || len(token) > 128 {
		return "", ErrDenied
	}
	return s.repo.AuthenticateServer(ctx, security.Hash(token))
}

func (s *Service) KeyCertificates() []*trust.Certificate {
	return []*trust.Certificate{s.keys.Certificate}
}

func (s *Service) TrustRoot() string {
	return base64.RawURLEncoding.EncodeToString(s.keys.RootPublicKey)
}

// The optional port returns no cause when the repository has no explicit evidence.
type MembershipEndingRepository interface {
	MembershipEndingNotice(context.Context, string) (*MembershipEndingNotice, error)
}

func (s *Service) MembershipEndingNotice(ctx context.Context, account string) (*MembershipEndingNotice, error) {
	if repo, ok := s.repo.(MembershipEndingRepository); ok {
		return repo.MembershipEndingNotice(ctx, account)
	}
	return nil, nil
}
