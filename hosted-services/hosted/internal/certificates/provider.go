package certificates

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/crypto/acme"
	"portico.local/hosted/internal/egress"
)

type providerCallKey struct{}
type providerCall struct {
	order                Order
	positive             bool
	newOrderURL, receipt string
	retryAfter           time.Time
}

type providerTransport struct {
	service *Service
	base    http.RoundTripper
	origin  string
}

func sameProviderURL(raw, origin string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Scheme+"://"+u.Host == origin && u.User == nil && u.Opaque == "" && u.Fragment == "" && u.RawQuery == "" && u.RawPath == "" && len(raw) <= 2048
}
func (t *providerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !sameProviderURL(r.URL.String(), t.origin) {
		return nil, ErrInvalid
	}
	call, _ := r.Context().Value(providerCallKey{}).(*providerCall)
	if call != nil {
		if e := t.service.repo.CheckCertificateLease(r.Context(), call.order, call.positive); e != nil {
			return nil, e
		}
	}
	response, e := t.base.RoundTrip(r)
	if e != nil {
		return nil, e
	}
	if call != nil {
		if next := retryAfter(response.Header.Get("Retry-After"), time.Now()); next.After(call.retryAfter) {
			call.retryAfter = next
		}
		if r.Method == http.MethodPost && call.newOrderURL == r.URL.String() && response.StatusCode >= 200 && response.StatusCode < 300 {
			location := response.Header.Get("Location")
			if !sameProviderURL(location, t.origin) {
				response.Body.Close()
				return nil, ErrUncertain
			}
			// Persist Location BEFORE reading the response body. Client cancellation or
			// malformed/truncated JSON cannot erase an already received order identity.
			receiptCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			e = t.service.repo.RecordProviderOrder(receiptCtx, call.order, location)
			cancel()
			if e != nil {
				response.Body.Close()
				return nil, ErrUncertain
			}
			call.receipt = location
		}
	}
	response.Body = &boundedProviderBody{ReadCloser: response.Body, remaining: 256 << 10}
	return response, nil
}

type boundedProviderBody struct {
	io.ReadCloser
	remaining int64
}

func (b *boundedProviderBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, ErrInvalid
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, e := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, e
}
func retryAfter(value string, now time.Time) time.Time {
	if value == "" {
		return time.Time{}
	}
	if d, e := time.ParseDuration(value + "s"); e == nil && d >= 0 {
		return now.Add(d)
	}
	if d, e := http.ParseTime(value); e == nil && d.After(now) {
		return d
	}
	return time.Time{}
}
func noACMERetry(int, *http.Request, *http.Response) time.Duration { return -1 }

// GetReg normally translates the RFC 8555 problem into ErrNoAccount. Keep the
// explicit problem form for transports/providers that return the typed error.
// Other failures (timeouts, authorization, outages) must never trigger Register.
func accountDoesNotExist(err error) bool {
	if errors.Is(err, acme.ErrNoAccount) {
		return true
	}
	var problem *acme.Error
	return errors.As(err, &problem) && problem.ProblemType == "urn:ietf:params:acme:error:accountDoesNotExist"
}

func (s *Service) account(ctx context.Context) (*acme.Client, error) {
	if s.cfg == nil {
		return nil, ErrConfiguration
	}
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	if s.client != nil {
		return s.client, nil
	}
	if time.Now().Before(s.nextAccountAttempt) {
		return nil, ErrConfiguration
	}
	s.nextAccountAttempt = time.Now().Add(time.Minute)
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return nil, e
	}
	der, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		return nil, e
	}
	defer clear(der)
	aad := "portico/acme-account/v1/" + s.cfg.ID()
	sealed, e := s.seal.Seal(base64.RawStdEncoding.EncodeToString(der), aad)
	if e != nil {
		return nil, e
	}
	directory, _ := s.cfg.directory()
	record, e := s.repo.ACMEAccount(ctx, s.cfg.ID(), directory, sealed)
	if e != nil {
		return nil, e
	}
	encoded, e := s.seal.Open(record.Cipher, aad)
	if e != nil {
		return nil, ErrConfiguration
	}
	saved, e := base64.RawStdEncoding.DecodeString(encoded)
	if e != nil {
		return nil, ErrConfiguration
	}
	defer clear(saved)
	parsed, e := x509.ParsePKCS8PrivateKey(saved)
	if e != nil {
		return nil, ErrConfiguration
	}
	accountKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || accountKey.Curve != elliptic.P256() {
		return nil, ErrConfiguration
	}
	originURL, _ := url.Parse(directory)
	clientHTTP := egress.NewClient(false)
	clientHTTP.Timeout = 15 * time.Second
	clientHTTP.Transport = &providerTransport{service: s, base: clientHTTP.Transport, origin: originURL.Scheme + "://" + originURL.Host}
	c := &acme.Client{Key: accountKey, DirectoryURL: directory, HTTPClient: clientHTTP, UserAgent: "Portico-P02/1", RetryBackoff: noACMERetry}
	if record.URL != "" {
		if !sameProviderURL(record.URL, originURL.Scheme+"://"+originURL.Host) {
			return nil, ErrConfiguration
		}
		c.KID = acme.KeyID(record.URL)
		s.client = c
		return c, nil
	}
	// Recover a lost newAccount acknowledgement using the same durable key.
	// EAB is consumed only if this key has no existing provider account.
	account, e := c.GetReg(ctx, "")
	if e != nil {
		if !accountDoesNotExist(e) {
			return nil, e
		}
		discovery, de := c.Discover(ctx)
		if de != nil {
			return nil, de
		}
		registration := &acme.Account{}
		if s.cfg.ContactEmail != "" {
			registration.Contact = []string{"mailto:" + s.cfg.ContactEmail}
		}
		if s.cfg.EABKeyID != "" {
			mac, me := decodeMAC(s.cfg.EABMAC)
			if me != nil {
				return nil, me
			}
			defer clear(mac)
			registration.ExternalAccountBinding = &acme.ExternalAccountBinding{KID: s.cfg.EABKeyID, Key: mac}
		} else if s.cfg.Provider == "google" || discovery.ExternalAccountRequired {
			return nil, ErrConfiguration
		}
		account, e = c.Register(ctx, registration, func(string) bool { return s.cfg.AcceptTerms })
		if errors.Is(e, acme.ErrAccountAlreadyExists) {
			// Another worker (or a lost acknowledgement) can have registered the
			// same durable key. Recover its account; never rotate to a new key.
			account, e = c.GetReg(ctx, "")
		}
		if e != nil {
			return nil, e
		}
	}
	if account == nil || account.Status != acme.StatusValid || !sameProviderURL(account.URI, originURL.Scheme+"://"+originURL.Host) {
		return nil, ErrConfiguration
	}
	if e = s.repo.SaveACMEAccountURL(ctx, s.cfg.ID(), account.URI); e != nil {
		return nil, e
	}
	c.KID = acme.KeyID(account.URI)
	s.client = c
	return c, nil
}

// Reconcile attaches a provider-discovered receipt, never creates an order. The
// CLI loads the durable candidate; this account must be able to read that URL.
// An issued certificate must also match the original CSR. No public HTTP route
// accepts arbitrary provider URLs or attempts to guess lost order identifiers.
func (s *Service) Reconcile(ctx context.Context, o Order, providerURL string) error {
	if s.cfg == nil || o.Issuer != s.cfg.ID() || o.State != "reconciliation_required" || o.ProviderURL != "" {
		return ErrConflict
	}
	c, e := s.account(ctx)
	if e != nil {
		return e
	}
	remote, e := c.GetOrder(ctx, providerURL)
	if e != nil {
		return e
	}
	if !validProviderOrder(o, remote) {
		return ErrInvalid
	}
	if remote.Status == acme.StatusValid {
		ders, e := c.FetchCert(ctx, remote.CertURL, true)
		if e != nil {
			return e
		}
		chain, e := encodeChain(ders)
		if e != nil {
			return e
		}
		roots, e := s.cfg.roots()
		if e != nil {
			return e
		}
		if _, e = validateReceiptChain(chain, o.CSR, o.Namespace, roots, time.Now()); e != nil {
			return e
		}
	}
	return s.repo.ReconcileCertificateURL(ctx, o.ID, o.Issuer, providerURL)
}
func validProviderOrder(o Order, r *acme.Order) bool {
	return r != nil && len(r.Identifiers) == 1 && r.Identifiers[0].Type == "dns" && r.Identifiers[0].Value == "*."+o.Namespace+".direct.getportico.tv" && len(r.AuthzURLs) <= 4
}
