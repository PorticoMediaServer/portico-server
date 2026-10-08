package certificates

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/security"
)

type Service struct {
	cleanupOnly        bool
	repo               Repository
	cfg                *Config
	seal               *security.Keys
	accountMu          sync.Mutex
	client             *acme.Client
	nextAccountAttempt time.Time
	// notify is signalled when an order is submitted. It is an optimisation: a lost signal
	// costs up to idleCertificatePoll of latency and nothing else.
	notify <-chan struct{}
}

// Announce gives the service the channel its workers watch for newly submitted orders, so an
// idle deployment stops asking the database every few seconds whether one has appeared.
func (s *Service) Announce(notify <-chan struct{}) {
	if s != nil {
		s.notify = notify
	}
}

func New(repo Repository, cfg *Config) (*Service, error) {
	s := &Service{repo: repo, cfg: cfg}
	if repo == nil {
		return nil, ErrConfiguration
	}
	if cfg != nil {
		var e error
		s.seal, e = cfg.sealing()
		if e != nil {
			return nil, e
		}
		if _, e = cfg.roots(); e != nil {
			return nil, e
		}
	}
	return s, nil
}
func (s *Service) Namespace(ctx context.Context, a directory.ServerAuthority) (Namespace, error) {
	n := Namespace{OperationID: a.OperationID, ClaimGeneration: strconv.FormatInt(a.ClaimGeneration, 10), CredentialGeneration: strconv.FormatInt(a.CredentialGeneration, 10), Configured: s.cfg != nil && !s.cleanupOnly}
	if s.cfg != nil {
		n.Issuer = s.cfg.Provider
		n.Environment = s.cfg.Environment
	}
	e := s.repo.WithDirectNamespace(ctx, a, func(ns string) error {
		n.Namespace = ns
		n.DNSName = "*." + ns + ".direct.getportico.tv"
		n.DNS01Target = ns + ".acme.getportico.tv"
		return nil
	})
	if e != nil {
		return Namespace{}, e
	}
	return n, nil
}
func (s *Service) Submit(ctx context.Context, a directory.ServerAuthority, request string, csr []byte) (Status, error) {
	if s.cfg == nil || s.cleanupOnly {
		return Status{}, ErrConfiguration
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(request)
	if e != nil || len(raw) != 24 || base64.RawURLEncoding.EncodeToString(raw) != request {
		return Status{}, ErrInvalid
	}
	n, e := s.Namespace(ctx, a)
	if e != nil {
		return Status{}, e
	}
	if _, e = ValidateCSR(csr, n.Namespace); e != nil {
		return Status{}, e
	}
	o, e := s.repo.SubmitCertificate(ctx, a, request, n.Namespace, s.cfg.ID(), csr)
	return projection(o), e
}
func (s *Service) Status(ctx context.Context, a directory.ServerAuthority, id string) (Status, error) {
	o, e := s.repo.ReadCertificate(ctx, a, id)
	return projection(o), e
}
func (s *Service) Chain(ctx context.Context, a directory.ServerAuthority, id string) ([]byte, error) {
	o, e := s.repo.ReadCertificate(ctx, a, id)
	if e != nil {
		return nil, e
	}
	if o.State != "issued" || o.CancelRequested || !time.Now().Before(o.NotAfter) || len(o.Chain) == 0 {
		return nil, ErrConflict
	}
	return o.Chain, nil
}
func (s *Service) Cancel(ctx context.Context, a directory.ServerAuthority, id string) error {
	return s.repo.CancelCertificate(ctx, a, id)
}
func projection(o Order) Status {
	provider, environment, _ := strings.Cut(o.Issuer, "-")
	p := Status{ID: o.ID, RequestID: o.RequestID, Namespace: o.Namespace, State: o.State, Issuer: provider, Environment: environment, OperationID: o.Authority.OperationID, ClaimGeneration: strconv.FormatInt(o.Authority.ClaimGeneration, 10), CredentialGeneration: strconv.FormatInt(o.Authority.CredentialGeneration, 10), NextAttempt: o.NextAttempt, ErrorCode: o.ErrorCode}
	if !o.NotAfter.IsZero() {
		p.NotAfter = &o.NotAfter
	}
	return p
}
func Hash(b []byte) []byte { h := sha256.Sum256(b); return h[:] }

func (s *Service) Lookup(ctx context.Context, a directory.ServerAuthority, request string) (Status, error) {
	raw, e := base64.RawURLEncoding.Strict().DecodeString(request)
	if e != nil || len(raw) != 24 {
		return Status{}, ErrInvalid
	}
	o, e := s.repo.LookupCertificate(ctx, a, request)
	return projection(o), e
}

// NewCleanupOnly services must not be exposed as the active issuance API. They
// retain an explicitly configured former issuer's account for cancellation and
// exact TXT cleanup during an operator-selected issuer/environment transition.
func NewCleanupOnly(repo Repository, cfg *Config) (*Service, error) {
	if cfg == nil {
		return nil, ErrConfiguration
	}
	s, e := New(repo, cfg)
	if e != nil {
		return nil, e
	}
	s.cleanupOnly = true
	return s, nil
}
