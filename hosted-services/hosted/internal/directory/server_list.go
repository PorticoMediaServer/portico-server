package directory

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"hash/maphash"
	"portico.local/hosted/internal/serverreturn"
	"portico.local/hosted/trust"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A device learns that its server list changed by asking "has it changed since
// version N?" a few times a day, while the app is open (Spec — Hosted at
// Scale). At a million accounts that is most of Hosted's traffic, so it is
// answered from memory: no database read and no account-session lookup.
//
// The version of an account is accounts.server_list_version, which takes a
// value from one global sequence whenever the account's server set changes.
// Memory holds, per slot of a hash of the account id, the largest version of
// any account in that slot. A collision can make a device fetch its list when
// nothing of its own changed. An account's own versions rise in commit order,
// so a change to it is never hidden; only if two accounts share a slot and one
// account's change commits after a larger value of the other could a check
// answer "unchanged" once, and the next change or Refresh Servers corrects it.

// ServerListVersionRepository reads versions above a point, in version order.
type ServerListVersionRepository interface {
	ServerListVersionsAfter(ctx context.Context, after int64, limit int, fn func(account string, version int64)) (int64, int, error)
}

type serverListVersions struct {
	mu     sync.RWMutex
	seed   maphash.Seed
	slots  map[uint64]int64
	high   int64
	loaded bool
	poke   chan struct{}
	// key authenticates watch tokens.
	key []byte
}

// versionPage bounds one read while following or loading the versions.
const versionPage = 10000

// versionWindow is how far behind the highest version each follow re-reads, to
// catch versions that committed late. Transactions that bump versions last
// milliseconds; this covers thousands of bumps committing meanwhile.
const versionWindow = 10000

// versionSafetyInterval is how often the map looks for versions it was not
// told about (a lost notification). One indexed read that finds nothing.
const versionSafetyInterval = 30 * time.Second

func (s *Service) listVersions() *serverListVersions {
	s.versionsOnce.Do(func() {
		// Watch tokens are keyed from the seal key, which every Hosted replica
		// loads from the same key file, so any replica verifies any token
		// (INT M18). Without one there are no watch tokens.
		var key []byte
		if s.keys != nil && len(s.keys.SealKey) > 0 {
			mac := hmac.New(sha256.New, s.keys.SealKey)
			mac.Write([]byte("portico.server-list.watch-key.v1"))
			key = mac.Sum(nil)
		}
		s.versions = &serverListVersions{seed: maphash.MakeSeed(), slots: map[uint64]int64{}, poke: make(chan struct{}, 1), key: key}
	})
	return s.versions
}

func (v *serverListVersions) slot(account string) uint64 { return maphash.String(v.seed, account) }

// RefreshServerListVersions reads every version above the highest one seen.
// At start it loads the whole map (one streaming pass over accounts that have
// ever had a server); afterwards it reads only what changed.
func (s *Service) RefreshServerListVersions(ctx context.Context) error {
	repo, ok := s.repo.(ServerListVersionRepository)
	if !ok {
		return ErrDenied
	}
	v := s.listVersions()
	// Re-read a trailing window: a version taken by a transaction that
	// commits after later ones would otherwise be skipped.
	v.mu.RLock()
	after := v.high - versionWindow
	v.mu.RUnlock()
	if after < 0 {
		after = 0
	}
	for {
		type row struct {
			account string
			version int64
		}
		var page []row
		last, n, e := repo.ServerListVersionsAfter(ctx, after, versionPage, func(account string, version int64) {
			page = append(page, row{account, version})
		})
		if e != nil {
			return e
		}
		v.mu.Lock()
		for _, r := range page {
			k := v.slot(r.account)
			if r.version > v.slots[k] {
				v.slots[k] = r.version
			}
		}
		if last > v.high {
			v.high = last
		}
		if n < versionPage {
			v.loaded = true
		}
		v.mu.Unlock()
		if n < versionPage {
			return nil
		}
		after = last
	}
}

// refreshVersionsSoon asks the follower to read new versions now. A push
// handled by this process is visible to devices within one read.
func (s *Service) refreshVersionsSoon() {
	select {
	case s.listVersions().poke <- struct{}{}:
	default:
	}
}

// FollowServerListVersions keeps the map current: on each notification, on a
// push this process handled, and on a slow safety tick. Failures back off with
// jitter like every other Hosted worker, and never block the answers already in
// memory.
func (s *Service) FollowServerListVersions(ctx context.Context, notified <-chan struct{}) {
	v := s.listVersions()
	tick := time.NewTicker(versionSafetyInterval)
	defer tick.Stop()
	failures := 0
	for {
		if e := s.RefreshServerListVersions(ctx); e != nil && ctx.Err() == nil {
			failures = min(failures+1, 6)
			delay := time.Duration(1<<failures) * time.Second
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay + jitter(delay)):
			}
			continue
		}
		failures = 0
		select {
		case <-ctx.Done():
			return
		case <-notified:
		case <-v.poke:
		case <-tick.C:
		}
	}
}

func jitter(d time.Duration) time.Duration {
	var b [8]byte
	rand.Read(b[:])
	n := int64(0)
	for _, x := range b {
		n = n<<8 | int64(x)
	}
	if n < 0 {
		n = -n
	}
	return time.Duration(n % (int64(d)/2 + 1))
}

// ServerListVersion is the version a device should hold for an account's
// list, as the ETag text. It is read before the list itself, so a change that
// commits in between moves the version past it rather than being missed.
func (s *Service) ServerListVersion(account string) string {
	v := s.listVersions()
	v.mu.RLock()
	defer v.mu.RUnlock()
	return strconv.FormatInt(v.slots[v.slot(account)], 10)
}

// ServerListReady reports whether the map has been loaded; until then a
// version check must not answer "unchanged".
func (s *Service) ServerListReady() bool {
	v := s.listVersions()
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.loaded
}

// WatchToken is the capability a device presents to check its version without
// an account session: the account id and a MAC over it. It says nothing but
// "changed or not", so it is long-lived; rotating the key costs each device
// one full list fetch.
func (s *Service) WatchToken(account string) string {
	if s.listVersions().key == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(account)) + "." + s.watchMAC(account)
}

func (s *Service) watchMAC(account string) string {
	mac := hmac.New(sha256.New, s.listVersions().key)
	mac.Write([]byte("portico.server-list.v1\x00" + account))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
}

// CheckServerList answers a version check from memory. It returns the
// current version, and whether the device's version is the current one.
func (s *Service) CheckServerList(watch, held string) (string, bool, error) {
	if len(watch) > 400 || s.listVersions().key == nil {
		return "", false, ErrDenied
	}
	encoded, mac, ok := strings.Cut(watch, ".")
	raw, e := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if !ok || e != nil || len(raw) == 0 || len(raw) > 128 {
		return "", false, ErrDenied
	}
	account := string(raw)
	if subtle.ConstantTimeCompare([]byte(mac), []byte(s.watchMAC(account))) != 1 {
		return "", false, ErrDenied
	}
	if !s.ServerListReady() {
		return "", false, ErrUnavailable
	}
	current := s.ServerListVersion(account)
	return current, held == current, nil
}

// ErrUnavailable is a check that cannot be answered yet (the map is loading).
var ErrUnavailable = unavailableError{}

type unavailableError struct{}

func (unavailableError) Error() string { return "server list versions are loading" }

// IdentityClaims is a Hosted identity assertion: "this is Portico Account A,
// speaking to server S". It grants nothing: the server decides membership
// from its own table and verifies this offline against its pinned root. The
// field order is the wire contract the server re-marshals and compares.
type IdentityClaims struct {
	Kind        string    `json:"kind"`
	AccountID   string    `json:"accountId"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	ServerID    string    `json:"serverId"`
	Challenge   string    `json:"challenge"`
	Nonce       string    `json:"nonce"`
	IssuedAt    time.Time `json:"issuedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// IdentityLifetime is how long an assertion is worth anything. A device asks
// for one just before it uses it.
const IdentityLifetime = 5 * time.Minute

// The kinds of assertion Hosted issues. An identity assertion signs an
// account in to a server and is its consent to be listed there; a custody
// assertion is the account accepting custody of the server's claim (retiring
// it, its certificates), asked for only when an owner accepts it (INT M6).
const (
	IdentityKind = "portico.identity.v1"
	CustodyKind  = "portico.custody.v1"
)

// ConsentLifetime bounds how old a consent may be to list an account a server
// pushes for the first time; CustodyConsentLifetime how old a custody
// assertion may be to move custody (INT M6). A member signs in often, and
// every sign-in renews its consent.
const (
	ConsentLifetime        = 30 * 24 * time.Hour
	CustodyConsentLifetime = 24 * time.Hour
)

// IssueIdentity signs an identity assertion for one server, carrying the
// challenge that server gave the asking installation. It reads nothing: the
// caller's account session was checked by the handler, and whether the server
// admits the account is the server's decision.
//
// purpose "" asks for an identity assertion, "custody" for a custody one.
func (s *Service) IssueIdentity(accountID, username, displayName, server, challenge, purpose string) (SignedPolicy, error) {
	kind := IdentityKind
	switch purpose {
	case "":
	case "custody":
		kind = CustodyKind
	default:
		return SignedPolicy{}, ErrInvalid
	}
	if !validServerID(server) || accountID == "" || s.keys == nil || len(challenge) < 22 || len(challenge) > 128 {
		return SignedPolicy{}, ErrInvalid
	}
	nonce := make([]byte, 24)
	if _, e := rand.Read(nonce); e != nil {
		return SignedPolicy{}, e
	}
	now := time.Now().UTC()
	claims := IdentityClaims{Kind: kind, AccountID: accountID, Username: username, DisplayName: displayName, ServerID: server, Challenge: challenge, Nonce: base64.RawURLEncoding.EncodeToString(nonce), IssuedAt: now, ExpiresAt: now.Add(IdentityLifetime)}
	raw, e := json.Marshal(claims)
	if e != nil {
		return SignedPolicy{}, e
	}
	return s.keys.Sign(raw)
}

// validServerID is the shape of a server id: "srv_" and 43 base64url
// characters (claims.ServerID).
func validServerID(v string) bool {
	if len(v) != 47 || !strings.HasPrefix(v, "srv_") {
		return false
	}
	_, e := base64.RawURLEncoding.Strict().DecodeString(v[4:])
	return e == nil
}

// verifyConsent checks that a member's consent is Hosted's own identity or
// custody assertion for that account and this server, and returns when it was
// issued and its kind.
// An old assertion is still consent (its signing certificate is checked as of
// its issue time); only its subject matters.
func (s *Service) verifyConsent(m MemberStanding, server string) (time.Time, string, string) {
	if len(m.Consent) == 0 || len(m.Consent) > 8192 || s.keys == nil {
		return time.Time{}, "", ""
	}
	var envelope SignedPolicy
	if json.Unmarshal(m.Consent, &envelope) != nil {
		return time.Time{}, "", ""
	}
	// A historical certificate proves the signature at issue time, but the
	// current root certificate's revocations are terminal for new consent.
	if s.keys.Certificate != nil {
		for _, id := range s.keys.Certificate.Revoked {
			if id == envelope.KeyID {
				return time.Time{}, "", ""
			}
		}
	}
	head, e := base64.RawURLEncoding.Strict().DecodeString(envelope.Payload)
	if e != nil {
		return time.Time{}, "", ""
	}
	var c IdentityClaims
	if json.Unmarshal(head, &c) != nil || (c.Kind != IdentityKind && c.Kind != CustodyKind) || c.AccountID != m.AccountID || c.ServerID != server || c.IssuedAt.After(time.Now().Add(5*time.Minute)) {
		return time.Time{}, "", ""
	}
	if len(s.keys.RootPublicKey) != ed25519.PublicKeySize || envelope.Certificate == nil {
		return time.Time{}, "", ""
	}
	if _, e = envelope.Verify(s.keys.RootPublicKey, "documents", c.IssuedAt); e != nil {
		return time.Time{}, "", ""
	}
	return c.IssuedAt, c.Kind, envelope.KeyID
}

// SignServerReturn is called only inside the registered-return consume. It
// cannot issue ordinary identity/custody assertions or infer contact identity.
func (s *Service) SignServerReturn(q serverreturn.Assertion) (trust.Envelope, error) {
	if s.keys == nil || q.Kind != "portico.server-return.v1" || !serverreturn.Binding(q.Purpose, q.CurrentOwnerAccountID, q.PreviousOwnerRole) || !validServerID(q.ServerID) || q.ClaimGeneration < 1 || q.CredentialGeneration < 1 {
		return trust.Envelope{}, ErrInvalid
	}
	raw, e := json.Marshal(q)
	if e != nil {
		return trust.Envelope{}, e
	}
	return s.keys.Sign(raw)
}
