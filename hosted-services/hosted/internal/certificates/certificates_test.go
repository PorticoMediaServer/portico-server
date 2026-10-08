package certificates

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/acme"
	"portico.local/hosted/internal/directory"
)

const testNamespace = "ptc-aaaaaaaaaaaaaaaaaaaa"

func testCSR(t *testing.T, names []string, extra []pkix.Extension) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	der, e := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{DNSNames: names, ExtraExtensions: extra}, key)
	if e != nil {
		t.Fatal(e)
	}
	return der, key
}
func TestCSRStrictAuthorizedWildcard(t *testing.T) {
	name := "*." + testNamespace + ".direct.getportico.tv"
	good, _ := testCSR(t, []string{name}, nil)
	if _, e := ValidateCSR(good, testNamespace); e != nil {
		t.Fatal(e)
	}
	for _, names := range [][]string{{name, "attacker.example"}, {"attacker.example"}, {"probe." + testNamespace + ".direct.getportico.tv"}} {
		der, _ := testCSR(t, names, nil)
		if _, e := ValidateCSR(der, testNamespace); e == nil {
			t.Fatal("unauthorized SAN")
		}
	}
	bad := append([]byte(nil), good...)
	bad[len(bad)-1] ^= 1
	if _, e := ValidateCSR(bad, testNamespace); e == nil {
		t.Fatal("bad signature")
	}
	san, _ := asn1.Marshal([]asn1.RawValue{{Class: 2, Tag: 2, Bytes: []byte(name)}, {Class: 2, Tag: 0, IsCompound: true, Bytes: []byte{}}})
	hidden, _ := testCSR(t, nil, []pkix.Extension{{Id: sanOID, Value: san}})
	if _, e := ValidateCSR(hidden, testNamespace); e == nil {
		t.Fatal("ignored otherName accepted")
	}
	if _, e := ValidateCSR(good, "ptc-bbbbbbbbbbbbbbbbbbbb"); e == nil {
		t.Fatal("noncanonical namespace")
	}
}
func TestIssuedChainMustMatchCSRTrustNameAndTime(t *testing.T) {
	now := time.Now()
	name := "*." + testNamespace + ".direct.getportico.tv"
	csr, key := testCSR(t, []string{name}, nil)
	rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic test root"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, e := x509.CreateCertificate(rand.Reader, root, root, rootKey.Public(), rootKey)
	if e != nil {
		t.Fatal(e)
	}
	root, e = x509.ParseCertificate(rootDER)
	if e != nil {
		t.Fatal(e)
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{name}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, leaf, root, key.Public(), rootKey)
	if e != nil {
		t.Fatal(e)
	}
	chain := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if _, e = ValidateChain(chain, csr, testNamespace, roots, now); e != nil {
		t.Fatal(e)
	}
	if _, e = ValidateChain(chain, csr, testNamespace, x509.NewCertPool(), now); e == nil {
		t.Fatal("untrusted root")
	}
	if _, e = ValidateChain(chain, csr, testNamespace, roots, now.Add(2*time.Hour)); e == nil {
		t.Fatal("expired certificate")
	}
	if _, e = validateReceiptChain(chain, csr, testNamespace, roots, now.Add(2*time.Hour)); e != nil {
		t.Fatal("expired owned receipt cannot reconcile", e)
	}
	if _, e = ValidateChain(append([]byte("ignored prefix"), chain...), csr, testNamespace, roots, now); e == nil {
		t.Fatal("unexpected PEM prefix")
	}
	other, _ := testCSR(t, []string{name}, nil)
	if _, e = ValidateChain(chain, other, testNamespace, roots, now); e == nil {
		t.Fatal("wrong key")
	}
	if _, e = ValidateChain(append(chain, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1}})...), csr, testNamespace, roots, now); e == nil {
		t.Fatal("unexpected PEM material")
	}
}

type fixtureRepo struct {
	Repository
	deny     bool
	receipt  string
	state    string
	finished bool
	checked  []bool
}

func (r *fixtureRepo) CheckCertificateLease(_ context.Context, _ Order, positive bool) error {
	r.checked = append(r.checked, positive)
	if positive && r.deny {
		return directory.ErrDenied
	}
	return nil
}
func (r *fixtureRepo) RecordProviderOrder(_ context.Context, _ Order, url string) error {
	r.receipt = url
	return nil
}
func (r *fixtureRepo) SetCertificateState(_ context.Context, _ Order, state, code string, _ time.Time) error {
	r.state = state
	return nil
}
func (r *fixtureRepo) CertificateChallenges(context.Context, Order) ([]Challenge, error) {
	return nil, nil
}
func (r *fixtureRepo) FinishCertificateRevocation(context.Context, Order) error {
	r.finished = true
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type unreadBody struct {
	t    *testing.T
	repo *fixtureRepo
}

func (b unreadBody) Read([]byte) (int, error) {
	if b.repo.receipt == "" {
		b.t.Fatal("response read before receipt persistence")
	}
	return 0, io.ErrUnexpectedEOF
}
func (unreadBody) Close() error { return nil }
func TestReceiptIsSavedBeforeMalformedResponseBody(t *testing.T) {
	repo := &fixtureRepo{}
	s := &Service{repo: repo}
	origin := "https://dv.acme-v02.api.pki.goog"
	url := origin + "/new-order"
	receipt := origin + "/order/accepted"
	tr := &providerTransport{service: s, origin: origin, base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 201, Header: http.Header{"Location": []string{receipt}}, Body: unreadBody{t, repo}}, nil
	})}
	call := &providerCall{order: Order{ID: "known-local-intent"}, positive: true, newOrderURL: url}
	req, _ := http.NewRequestWithContext(context.WithValue(context.Background(), providerCallKey{}, call), "POST", url, bytes.NewBufferString("signed-body"))
	response, e := tr.RoundTrip(req)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if repo.receipt != receipt || call.receipt != receipt {
		t.Fatal("accepted order identity lost")
	}
	if _, e = io.ReadAll(response.Body); e == nil {
		t.Fatal("malformed synthetic body unexpectedly succeeded")
	}
	repo.deny = true
	if _, e = tr.RoundTrip(req); !errors.Is(e, directory.ErrDenied) {
		t.Fatal("new positive request after revocation")
	}
}
func TestRestartFromUncertainCreationNeverCallsNewOrder(t *testing.T) {
	repo := &fixtureRepo{}
	calls := 0
	client := &acme.Client{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected provider call")
	})}}
	s := &Service{repo: repo, cfg: &Config{Provider: "google", Environment: "production"}, client: client}
	_, code := s.step(context.Background(), Order{ID: "persisted", State: "creating", CreateAttempted: true})
	if code != "acceptance_uncertain" || repo.state != "reconciliation_required" || calls != 0 {
		t.Fatal("blind retry after unknown external acceptance", code, calls)
	}
}
func TestCancelledCreationKeepsUncertaintyButNeverCreatedJobCanFinish(t *testing.T) {
	for _, attempted := range []bool{false, true} {
		repo := &fixtureRepo{deny: true}
		s := &Service{repo: repo, cfg: &Config{Provider: "google", Environment: "production"}}
		_, code := s.step(context.Background(), Order{ID: "persisted", State: "queued", CreateAttempted: attempted})
		if attempted {
			if repo.finished || code != "acceptance_uncertain" || repo.state != "reconciliation_required" {
				t.Fatal("ambiguous cancellation discarded")
			}
		} else if !repo.finished {
			t.Fatal("never-created cancellation stalled")
		}
	}
}
func TestProviderOriginsAndExplicitEnvironmentIsolation(t *testing.T) {
	seen := map[string]bool{}
	for _, provider := range []string{"google", "letsencrypt"} {
		for _, env := range []string{"production", "staging"} {
			cfg := Config{Provider: provider, Environment: env}
			url, e := cfg.directory()
			if e != nil || seen[url] {
				t.Fatal("environment alias")
			}
			seen[url] = true
		}
	}
	origin := "https://dv.acme-v02.api.pki.goog"
	for _, bad := range []string{origin + ".attacker.example/order", origin + "/order?redirect=1", "http://dv.acme-v02.api.pki.goog/order", "https://user:password@dv.acme-v02.api.pki.goog/order"} {
		if sameProviderURL(bad, origin) {
			t.Fatal("unsafe provider URL", bad)
		}
	}
	if !strings.Contains((Config{EABMAC: "do-not-print"}).String(), "redacted") {
		t.Fatal("configuration disclosure")
	}
	if noACMERetry(1, nil, nil) >= 0 {
		t.Fatal("library blind retries enabled")
	}
}

func TestAccountRegistrationOnlyAfterDefiniteMissingAccount(t *testing.T) {
	for _, err := range []error{acme.ErrNoAccount, fmt.Errorf("lookup: %w", acme.ErrNoAccount), &acme.Error{ProblemType: "urn:ietf:params:acme:error:accountDoesNotExist"}} {
		if !accountDoesNotExist(err) {
			t.Fatal("first registration blocked", err)
		}
	}
	for _, err := range []error{nil, context.DeadlineExceeded, acme.ErrAccountAlreadyExists, &acme.Error{StatusCode: 503}, &acme.Error{ProblemType: "urn:ietf:params:acme:error:unauthorized"}} {
		if accountDoesNotExist(err) {
			t.Fatal("uncertain or existing account permits a fresh registration", err)
		}
	}
}

func TestRevokedOrderCleanupDoesNotReopenCancellation(t *testing.T) {
	repo := &fixtureRepo{deny: true}
	s := &Service{repo: repo, cfg: &Config{}}
	_, code := s.step(context.Background(), Order{ID: "revoked", State: "revoked", RevocationDone: true})
	if code != "" || repo.state != "" || repo.finished {
		t.Fatal("terminal revoked receipt was rewritten", code, repo.state, repo.finished)
	}
	if len(repo.checked) != 1 || repo.checked[0] {
		t.Fatal("cleanup attempted positive authority", repo.checked)
	}
}
