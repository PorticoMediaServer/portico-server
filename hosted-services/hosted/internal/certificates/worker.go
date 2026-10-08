package certificates

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/security"
)

// Run is owned by Hosted's shutdown context. Each step is bounded below the
// durable lease deadline. No goroutine or work item can multiply without bound.
func (s *Service) Run(ctx context.Context) {
	if s.cfg == nil {
		return
	}
	var wg sync.WaitGroup
	for i := 0; i < s.cfg.Workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.worker(ctx) }()
	}
	wg.Wait()
}

// idleCertificatePoll is how long a worker with nothing to do sleeps before looking again. It
// was three seconds per worker, which on an idle deployment is a lease query every one and a
// half seconds for ever; a submitted order now arrives as a notification instead, and this is
// only the backstop for a retry deadline or a notification nobody was listening for. The
// per-order backoff floor is fifteen seconds and is jittered, so waking on this cadence costs
// a retry nothing anyone can measure. It also bounds the authority sweep LeaseCertificate
// performs on every attempt, whose own cadence is five minutes.
const idleCertificatePoll = 30 * time.Second

func (s *Service) worker(ctx context.Context) {
	for ctx.Err() == nil {
		o, e := s.repo.LeaseCertificate(ctx, s.cfg.ID(), security.RandomID("lease_"), s.cleanupOnly)
		if e != nil {
			if !idle(ctx, idleCertificatePoll, s.notify) {
				return
			}
			continue
		}
		stepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		next, code := s.step(stepCtx, o)
		cancel()
		releaseCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_ = s.repo.ReleaseCertificate(releaseCtx, o, next, code)
		done()
	}
}

var errQuotaWait = errors.New("certificate order budget is spent for this minute")

func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// idle waits for work to be announced, for the backstop to elapse, or for shutdown. A nil
// notify channel is never ready, so a service wired without a listener simply falls back to
// the backstop.
func idle(ctx context.Context, d time.Duration, notify <-chan struct{}) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-notify:
		return true
	case <-t.C:
		return true
	}
}
func backoff(o Order) time.Duration {
	n := o.Attempts
	if n > 7 {
		n = 7
	}
	if n < 1 {
		n = 1
	}
	h := sha256.Sum256([]byte(o.ID))
	return time.Duration(1<<n)*15*time.Second + time.Duration(h[0]%17)*time.Second
}
func (s *Service) step(ctx context.Context, o Order) (time.Time, string) {
	next := time.Now().Add(backoff(o))
	// Revoked receipts may still own a deferred TXT cleanup. Their positive
	// lease is intentionally invalid; never rewrite a completed revocation as
	// a fresh cancellation merely to finish that independent cleanup.
	if o.State == "revoked" && o.RevocationDone {
		if e := s.repo.CheckCertificateLease(ctx, o, false); e != nil {
			return next, "cleanup_retry"
		}
		if e := s.cleanup(ctx, o); e != nil {
			return next, "cleanup_retry"
		}
		return time.Now().Add(6 * time.Minute), ""
	}
	current := s.repo.CheckCertificateLease(ctx, o, true)
	if errors.Is(current, directory.ErrDenied) || errors.Is(current, ErrLease) || (o.CancelRequested || s.cleanupOnly && o.State != "issued" && o.State != "revoked") {
		// Stale lease is not permission to mutate; Set... still checks its token.
		if e := s.repo.SetCertificateState(ctx, o, "cancelled", "authority_changed", next); e != nil {
			return next, "authority_changed"
		}
		o.CancelRequested = true
		o.State = "cancelled"
	} else if current != nil {
		return next, "coordinator_retry"
	}
	if o.CancelRequested || o.State == "issued" || o.State == "failed" || o.State == "revoked" || o.State == "reconciliation_required" {
		e := s.cleanup(ctx, o)
		if e != nil {
			return next, "cleanup_retry"
		}
		if o.CancelRequested && !o.RevocationDone {
			if o.ProviderURL == "" {
				// queued definitely never crossed the boundary; creating without a receipt
				// is ambiguous, and remains quarantined until operator reconciliation.
				if o.CreateAttempted {
					_ = s.repo.SetCertificateState(ctx, o, "reconciliation_required", "acceptance_uncertain", next)
					return next, "acceptance_uncertain"
				}
				_ = s.repo.FinishCertificateRevocation(ctx, o)
				return next, ""
			}
			if e = s.revoke(ctx, o); e != nil {
				return next, "revocation_retry"
			}
		}
		return time.Now().Add(6 * time.Minute), ""
	}
	c, e := s.account(ctx)
	if e != nil {
		return next, "acme_account_configuration_or_retry"
	}
	call := &providerCall{order: o, positive: true}
	ctx = context.WithValue(ctx, providerCallKey{}, call)
	e = s.advance(ctx, c, &o, call)
	if call.retryAfter.After(next) {
		next = call.retryAfter
	}
	if e == nil {
		next = time.Now().Add(20 * time.Second)
		if call.retryAfter.After(next) {
			next = call.retryAfter
		}
		return next, ""
	}
	if errors.Is(e, errQuotaWait) {
		return call.retryAfter, ""
	}
	if errors.Is(e, ErrUncertain) {
		return next, "acceptance_uncertain"
	}
	if errors.Is(e, ErrInvalid) {
		_ = s.repo.SetCertificateState(ctx, o, "failed", "provider_response_invalid", next)
		return next, "provider_response_invalid"
	}
	if errors.Is(e, ErrConfiguration) {
		return next, "dns_or_issuer_configuration"
	}
	if delay, limited := acme.RateLimit(e); limited && time.Now().Add(delay).After(next) {
		next = time.Now().Add(delay)
	}
	return next, "provider_retry"
}
func (s *Service) advance(ctx context.Context, c *acme.Client, o *Order, call *providerCall) error {
	if o.ProviderURL == "" {
		if o.State == "creating" {
			if e := s.repo.SetCertificateState(ctx, *o, "reconciliation_required", "acceptance_uncertain", time.Now().Add(time.Hour)); e != nil {
				return e
			}
			return ErrUncertain
		}
		if o.State != "queued" {
			return ErrConflict
		}
		if e := s.checkDNSPolicy(ctx, o.Namespace); e != nil {
			return e
		}
		directory, e := c.Discover(ctx)
		if e != nil {
			return e
		}
		call.newOrderURL = directory.OrderURL
		// The authority's order quota is shared by the whole fleet. Spend it deliberately here,
		// before the request, rather than finding the ceiling by being refused: a refusal costs
		// quota standing, and after an outage every queued order would otherwise go at once.
		wait, e := s.repo.AdmitCertificateCreation(ctx, *o, s.cfg.OrdersPerMinute)
		if e != nil {
			return e
		}
		if wait > 0 {
			call.retryAfter = time.Now().Add(wait + time.Duration(rand.Int64N(int64(2*time.Minute))))
			return errQuotaWait
		}
		if e = s.repo.SetCertificateState(ctx, *o, "creating", "creation_started", time.Now().Add(time.Minute)); e != nil {
			return e
		}
		o.State = "creating"
		o.ErrorCode = "creation_started"
		o.CreateAttempted = true
		remote, e := c.AuthorizeOrder(ctx, acme.DomainIDs("*."+o.Namespace+".direct.getportico.tv"))
		if call.receipt != "" {
			o.ProviderURL = call.receipt
			o.State = "ordering"
		}
		if e != nil {
			if o.ProviderURL != "" {
				return e
			} // next step reads the recorded provider order
			var ae *acme.Error
			if errors.As(e, &ae) && ae.StatusCode >= 400 && ae.StatusCode < 500 {
				// A definite rejection can be retried. Transport/5xx/truncation cannot.
				if er := s.repo.SetCertificateState(ctx, *o, "queued", "provider_rejected", time.Now().Add(backoff(*o))); er != nil {
					return er
				}
				return e
			}
			if er := s.repo.SetCertificateState(ctx, *o, "reconciliation_required", "acceptance_uncertain", time.Now().Add(time.Hour)); er != nil {
				return er
			}
			return ErrUncertain
		}
		if remote == nil || remote.URI == "" || !validProviderOrder(*o, remote) {
			return ErrInvalid
		}
		if o.ProviderURL == "" {
			if e = s.repo.RecordProviderOrder(ctx, *o, remote.URI); e != nil {
				return e
			}
			o.ProviderURL = remote.URI
		}
	}
	remote, e := c.GetOrder(ctx, o.ProviderURL)
	if e != nil {
		return e
	}
	if !validProviderOrder(*o, remote) {
		return ErrInvalid
	}
	switch remote.Status {
	case acme.StatusInvalid:
		return s.repo.SetCertificateState(ctx, *o, "failed", "provider_order_invalid", time.Now().Add(time.Minute))
	case acme.StatusPending:
		if len(remote.AuthzURLs) == 0 {
			return ErrInvalid
		}
		for _, url := range remote.AuthzURLs {
			auth, e := c.GetAuthorization(ctx, url)
			if e != nil {
				return e
			}
			if auth.Identifier.Type != "dns" || auth.Identifier.Value != o.Namespace+".direct.getportico.tv" || !auth.Wildcard {
				return ErrInvalid
			}
			if auth.Status == acme.StatusValid {
				continue
			}
			if auth.Status != acme.StatusPending {
				return s.repo.SetCertificateState(ctx, *o, "failed", "dns_validation_failed", time.Now().Add(time.Minute))
			}
			var challenge *acme.Challenge
			for _, ch := range auth.Challenges {
				if ch.Type == "dns-01" {
					challenge = ch
					break
				}
			}
			if challenge == nil {
				return ErrInvalid
			}
			value, e := c.DNS01ChallengeRecord(challenge.Token)
			if e != nil {
				return e
			}
			owned := Challenge{AuthorizationURL: url, ChallengeURL: challenge.URI, Namespace: o.Namespace, Value: value}
			if e = s.repo.OwnCertificateChallenge(ctx, *o, owned); e != nil {
				return e
			}
			if e = s.repo.CheckCertificateLease(ctx, *o, true); e != nil {
				return e
			}
			dns := s.cfg.dns()
			if e = dns.Present(ctx, owned.Namespace, owned.Value); e != nil {
				return e
			}
			if e = s.propagated(ctx, owned); e != nil {
				return e
			}
			// Authorization/challenge may already be processing after a lost Accept ack.
			if challenge.Status == acme.StatusPending {
				if _, e = c.Accept(ctx, challenge); e != nil {
					return e
				}
			}
		}
		return nil
	case acme.StatusReady:
		if e = s.repo.SetCertificateState(ctx, *o, "finalizing", "", time.Now().Add(time.Minute)); e != nil {
			return e
		}
		// CreateOrderCert polls internally, but context + transport authority checks
		// bound every request. A lost finalize ack resumes GetOrder, not newOrder.
		ders, _, e := c.CreateOrderCert(ctx, remote.FinalizeURL, o.CSR, true)
		if e != nil {
			return e
		}
		return s.installResult(ctx, *o, ders)
	case acme.StatusProcessing:
		return nil
	case acme.StatusValid:
		ders, e := c.FetchCert(ctx, remote.CertURL, true)
		if e != nil {
			return e
		}
		return s.installResult(ctx, *o, ders)
	default:
		return ErrInvalid
	}
}
func encodeChain(ders [][]byte) ([]byte, error) {
	if len(ders) == 0 || len(ders) > 8 {
		return nil, ErrInvalid
	}
	var out []byte
	for _, der := range ders {
		if len(der) == 0 || len(der) > 16<<10 {
			return nil, ErrInvalid
		}
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	if len(out) > 64<<10 {
		return nil, ErrInvalid
	}
	return out, nil
}
func (s *Service) installResult(ctx context.Context, o Order, ders [][]byte) error {
	chain, e := encodeChain(ders)
	if e != nil {
		return e
	}
	roots, e := s.cfg.roots()
	if e != nil {
		return e
	}
	leaf, e := ValidateChain(chain, o.CSR, o.Namespace, roots, time.Now())
	if e != nil {
		return e
	}
	return s.repo.SaveCertificateChain(ctx, o, chain, leaf)
}
func (s *Service) cleanup(ctx context.Context, o Order) error {
	challenges, e := s.repo.CertificateChallenges(ctx, o)
	if e != nil {
		return e
	}
	dns := s.cfg.dns()
	for _, ch := range challenges {
		if ch.CleanupDone || time.Now().Before(ch.CleanupAfter) {
			continue
		}
		if e = s.repo.CheckCertificateLease(ctx, o, false); e != nil {
			return e
		}
		if e = dns.Cleanup(ctx, ch.Namespace, ch.Value); e != nil {
			return e
		}
		if e = s.repo.FinishCertificateCleanup(ctx, o, ch); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) revoke(ctx context.Context, o Order) error {
	c, e := s.account(ctx)
	if e != nil {
		return e
	}
	ctx = context.WithValue(ctx, providerCallKey{}, &providerCall{order: o, positive: false})
	if len(o.Chain) == 0 {
		remote, e := c.GetOrder(ctx, o.ProviderURL)
		if e != nil {
			return e
		}
		if !validProviderOrder(o, remote) {
			return ErrInvalid
		}
		switch remote.Status {
		case acme.StatusValid:
			ders, e := c.FetchCert(ctx, remote.CertURL, true)
			if e != nil {
				return e
			}
			o.Chain, e = encodeChain(ders)
			if e != nil {
				return e
			}
			roots, e := s.cfg.roots()
			if e != nil {
				return e
			}
			leaf, e := validateReceiptChain(o.Chain, o.CSR, o.Namespace, roots, time.Now())
			if e != nil {
				return e
			}
			// Persist cancellation first, so even a historically valid receipt
			// cannot take SaveCertificateChain's positive publication branch.
			if e = s.repo.SetCertificateState(ctx, o, "cancelled", "authority_changed", time.Now()); e != nil {
				return e
			}
			if e = s.repo.SaveCertificateChain(ctx, o, o.Chain, leaf); e != nil {
				return e
			}
		case acme.StatusInvalid:
			return s.repo.FinishCertificateRevocation(ctx, o)
		default:
			// Revoking owned authorizations is deny-only. If finalization was already in
			// flight, keep polling its durable order until invalid or certificate issued.
			for _, a := range remote.AuthzURLs {
				auth, e := c.GetAuthorization(ctx, a)
				if e != nil {
					return e
				}
				if auth.Status == acme.StatusPending || auth.Status == acme.StatusValid {
					if e = c.RevokeAuthorization(ctx, a); e != nil {
						return e
					}
				}
			}
			return errors.New("provider cancellation pending")
		}
	}
	leaf, _ := pem.Decode(o.Chain)
	if leaf == nil {
		return ErrInvalid
	}
	parsed, e := x509.ParseCertificate(leaf.Bytes)
	if e != nil {
		return ErrInvalid
	}
	if !time.Now().Before(parsed.NotAfter) {
		// Already expired is terminal deny-only state, not a claim that a CA
		// accepted a revocation request. Its exact TXT cleanup remains durable.
		return s.repo.FinishCertificateRevocation(ctx, o)
	}
	e = c.RevokeCert(ctx, nil, leaf.Bytes, acme.CRLReasonCessationOfOperation)
	var ae *acme.Error
	if e != nil && !(errors.As(e, &ae) && strings.HasSuffix(ae.ProblemType, ":alreadyRevoked")) {
		return e
	}
	return s.repo.FinishCertificateRevocation(ctx, o)
}
