package directory

import (
	"context"
	"regexp"
	"time"

	"portico.local/hosted/internal/security"
)

// ServerAuthority is a server-captured current credential expectation, never a
// client-controlled body or a server-ID-only grant. Consumers recheck its full
// tuple inside their own mutation/read transaction.
type ServerAuthority struct {
	ServerID, AccountID, OperationID string
	// AccountID is current ownership; ClaimAccountID preserves the installation proof audience.
	ClaimAccountID                        string
	TokenHash                             []byte
	PublicKey                             []byte
	ClaimGeneration, CredentialGeneration int64
}

func (ServerAuthority) String() string     { return "[server authority redacted]" }
func (a ServerAuthority) GoString() string { return a.String() }
func (a ServerAuthority) Valid() bool {
	return len(a.ServerID) > 0 && len(a.ServerID) <= 128 && len(a.AccountID) > 0 && len(a.AccountID) <= 128 && len(a.OperationID) > 0 && len(a.OperationID) <= 128 && len(a.TokenHash) == 32 && len(a.PublicKey) == 32 && a.ClaimGeneration > 0 && a.CredentialGeneration > 0
}

// WireAccountID is not account authority. It is the immutable claim proof binding.
func (a ServerAuthority) WireAccountID() string {
	if a.ClaimAccountID != "" {
		return a.ClaimAccountID
	}
	return a.AccountID
}

type CurrentServerRepository interface {
	CaptureServerAuthority(context.Context, []byte) (ServerAuthority, error)
	ServerHeartbeat(context.Context, ServerAuthority, CheckInReport) (Heartbeat, error)
	PushMembers(context.Context, ServerAuthority, MemberPush) (MemberPushResult, error)
}

func (s *Service) CaptureServerAuthority(ctx context.Context, token string) (ServerAuthority, error) {
	if len(token) < 20 || len(token) > 128 {
		return ServerAuthority{}, ErrDenied
	}
	repo, ok := s.repo.(CurrentServerRepository)
	if !ok {
		return ServerAuthority{}, ErrDenied
	}
	return repo.CaptureServerAuthority(ctx, security.Hash(token))
}

func (s *Service) ServerHeartbeat(ctx context.Context, a ServerAuthority, q CheckInReport) (Heartbeat, error) {
	if q.WakeRevision < 0 || q.MemberSequence < 0 || len(q.MemberDigest) > 64 {
		return Heartbeat{}, ErrInvalid
	}
	repo, ok := s.repo.(CurrentServerRepository)
	if !ok {
		return Heartbeat{}, ErrDenied
	}
	out, e := repo.ServerHeartbeat(ctx, a, q)
	if e != nil {
		return Heartbeat{}, e
	}
	// A check-in is not a signed document, so an expired or revoked signer does
	// not refuse it: presence and wake acknowledgement carry on. Such a process
	// only stops handing out its own certificate. The repository may supply
	// a newer root-certified public certificate from another replica.
	if out.SigningCertificate != nil {
		if s.keys == nil || out.SigningCertificate.Revision > security.MaxSigningRevision {
			out.SigningCertificate = nil
		} else if _, err := out.SigningCertificate.Verify(s.keys.RootPublicKey, "documents", time.Now()); err != nil {
			out.SigningCertificate = nil
		}
	} else if s.keys.ValidateSigner(time.Now()) == nil {
		out.SigningCertificate = s.keys.Certificate
	}
	return out, nil
}

// Push limits. A delta names at most what one server pass sends (2,000 journal
// rows); a full list is every member of one server.
const (
	maxDeltaMembers = 2000
	maxFullMembers  = 10000
)

// PushMembers records a server's report of its Portico Account members in the
// discovery index, then brings this process's version map up to date so the
// affected accounts' devices see the change on their next check.
func (s *Service) PushMembers(ctx context.Context, a ServerAuthority, p MemberPush) (MemberPushResult, error) {
	limit := maxDeltaMembers
	if p.Full {
		limit = maxFullMembers
	}
	if p.After < 0 || p.Through < p.After || (p.Full && p.After != 0) || (!p.Full && (p.Page != 0 || p.More)) || p.Page < 0 || p.Members == nil || len(p.Members) > limit {
		return MemberPushResult{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, m := range p.Members {
		if m.AccountID == "" || len(m.AccountID) > 128 || seen[m.AccountID] || (m.Active && !memberRole(m.Role)) || (!m.Active && m.Role != "" && !memberRole(m.Role)) {
			return MemberPushResult{}, ErrInvalid
		}
		if m.MembershipID != "" && !membershipIncarnation.MatchString(m.MembershipID) {
			return MemberPushResult{}, ErrInvalid
		}
		if m.Ending != nil {
			e := m.Ending
			if m.Active || !membershipIncarnation.MatchString(m.MembershipID) || e.Sequence < 1 || e.Sequence > p.Through || e.Sequence > 9007199254740991 || e.Cause != "owner_removed" || e.EndedAt.IsZero() || e.EndedAt.After(time.Now().Add(5*time.Minute)) || !e.ExpiresAt.Equal(e.EndedAt.Add(30*24*time.Hour)) {
				return MemberPushResult{}, ErrInvalid
			}
		}
		seen[m.AccountID] = true
	}
	for i := range p.Members {
		if p.Members[i].Active {
			p.Members[i].ConsentAt, p.Members[i].ConsentKind, p.Members[i].ConsentSigner = s.verifyConsent(p.Members[i], a.ServerID)
		}
	}
	repo, ok := s.repo.(CurrentServerRepository)
	if !ok {
		return MemberPushResult{}, ErrDenied
	}
	result, e := repo.PushMembers(ctx, a, p)
	if e == nil && result.Applied {
		s.refreshVersionsSoon()
	}
	return result, e
}

var membershipIncarnation = regexp.MustCompile(`^[0-9a-f]{32}$`)

func memberRole(role string) bool { return role == "owner" || role == "admin" || role == "member" }

// LeaveRepository removes a server from an account's own list.
type LeaveRepository interface {
	LeaveServer(ctx context.Context, account, server string) error
}

// LeaveServer is an account removing a server from its list (INT M6).
func (s *Service) LeaveServer(ctx context.Context, account, server string) error {
	if !validServerID(server) {
		return ErrInvalid
	}
	repo, ok := s.repo.(LeaveRepository)
	if !ok {
		return ErrDenied
	}
	return repo.LeaveServer(ctx, account, server)
}
