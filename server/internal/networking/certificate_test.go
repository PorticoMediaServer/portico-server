package networking

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const fixtureCertificateNamespace = "ptc-aaaaaaaaaaaaaaaaaaaa"

func certificateFixture(t *testing.T) (*sqlFixture, *CertificateManager, context.Context) {
	t.Helper()
	f := fixtureSQL(t)
	ctx := fixtureClaimContext(t)
	v := f.retrieving(t)
	result := resultFor(t, v)
	defer result.Credential.Clear()
	v, e := f.store.Install(ctx, v, result)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Acknowledged(ctx, v); e != nil {
		t.Fatal(e)
	}
	_, runner := lifecycleRunner(t)
	transport, e := NewHTTPTransport("https://hosted.example", f.store)
	if e != nil {
		t.Fatal(e)
	}
	m, e := NewCertificateManager(ctx, f.store, runner, transport, f.dir, CertificateOptions{})
	if e != nil || m.bootError != "" {
		t.Fatal("local certificate initialization", e)
	}
	return f, m, ctx
}
func syntheticCertificate(t *testing.T, m *CertificateManager, csrDER []byte) []byte {
	t.Helper()
	csr, e := x509.ParseCertificateRequest(csrDER)
	if e != nil {
		t.Fatal(e)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic P02 root"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour)}
	root, e := x509.CreateCertificate(rand.Reader, ca, ca, key.Public(), key)
	if e != nil {
		t.Fatal(e)
	}
	ca, e = x509.ParseCertificate(root)
	if e != nil {
		t.Fatal(e)
	}
	m.roots = x509.NewCertPool()
	m.roots.AddCert(ca)
	cert := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: csr.DNSNames, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(8 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, cert, ca, csr.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
func installSyntheticCertificate(t *testing.T, m *CertificateManager, ctx context.Context) {
	t.Helper()
	_, q, _, e := m.snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	v := q.Scope.Intent
	n := certificateNamespace{Namespace: fixtureCertificateNamespace, DNSName: "*." + fixtureCertificateNamespace + ".direct.getportico.tv", DNS01Target: fixtureCertificateNamespace + ".acme.getportico.tv", OperationID: v.OperationID, ClaimGeneration: v.ClaimGeneration, CredentialGeneration: v.CredentialGeneration, Issuer: "google", Environment: "production", Configured: true}
	var chain []byte
	id := "cert_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	m.transport.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
		var out any
		switch {
		case strings.HasSuffix(r.URL.Path, "/namespace"):
			out = n
		case strings.HasSuffix(r.URL.Path, "/orders"):
			var in struct {
				RequestID string `json:"requestId"`
				CSR       []byte `json:"csr"`
			}
			raw, _ := io.ReadAll(r.Body)
			if json.Unmarshal(raw, &in) != nil || len(in.CSR) == 0 {
				t.Fatal("invalid synthetic submit")
			}
			// The exact pending key/CSR must already exist durably before outbound submit.
			_, pending, _, e := m.snapshot(ctx)
			if e != nil {
				t.Fatal(e)
			}
			mat, e := m.material(ctx, pending.Scope, pending.PendingID)
			if e != nil || mat.RequestID != in.RequestID || !bytes.Equal(mat.CSR, in.CSR) {
				t.Fatal("submission preceded durable local key")
			}
			chain = syntheticCertificate(t, m, in.CSR)
			out = certificateOrder{ID: id, RequestID: in.RequestID, Namespace: n.Namespace, State: "issued", Issuer: n.Issuer, Environment: n.Environment, OperationID: v.OperationID, ClaimGeneration: v.ClaimGeneration, CredentialGeneration: v.CredentialGeneration, NextAttemptAt: time.Now()}
		case strings.HasSuffix(r.URL.Path, "/chain"):
			out = struct {
				Chain []byte `json:"chain"`
			}{chain}
		default:
			t.Fatalf("unexpected Hosted operation %s", r.URL.Path)
		}
		raw, _ := json.Marshal(out)
		return reply(http.StatusOK, string(raw)), nil
	})
	if e = m.step(ctx); e != nil {
		t.Fatal(e)
	}
	if e = m.step(ctx); e != nil {
		t.Fatal(e)
	}
	s, e := m.Status(ctx)
	if e != nil || !s.TLSReady || !s.PubliclyTrusted || s.Reachability != "probe_required" || s.ListenerBound {
		t.Fatal("incorrect certificate readiness", s, e)
	}
}
func TestCertificateConnectedLifecycleRestartAndAuthorityFencing(t *testing.T) {
	f, m, ctx := certificateFixture(t)
	installSyntheticCertificate(t, m, ctx)
	hello := &tls.ClientHelloInfo{ServerName: "current." + fixtureCertificateNamespace + ".direct.getportico.tv"}
	cert, e := m.GetCertificate(hello)
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256([]byte("synthetic handshake"))
	signer := cert.PrivateKey.(crypto.Signer)
	if _, e = signer.Sign(rand.Reader, hash[:], crypto.SHA256); e != nil {
		t.Fatal(e)
	}
	restarted, e := NewCertificateManager(ctx, f.store, m.runner, m.transport, f.dir, CertificateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	restarted.roots = m.roots
	if _, e = restarted.GetCertificate(hello); e != nil {
		t.Fatal("restart lost installed material", e)
	}
	if _, e = f.db.Exec(`UPDATE fixture_server_authority SET allowed=0`); e != nil {
		t.Fatal(e)
	}
	if _, e = signer.Sign(rand.Reader, hash[:], crypto.SHA256); e == nil {
		t.Fatal("previously captured signer survived revocation")
	}
	if _, e = restarted.GetCertificate(hello); e == nil {
		t.Fatal("revoked claim still served TLS")
	}
}

func TestCertificateServingReadPathAndTLSResumption(t *testing.T) {
	_, m, ctx := certificateFixture(t)
	installSyntheticCertificate(t, m, ctx)
	name := "current." + fixtureCertificateNamespace + ".direct.getportico.tv"
	for range 1000 {
		if _, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); err != nil {
			t.Fatal(err)
		}
	}
	server := &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12, GetCertificate: m.GetCertificate, NextProtos: []string{"h2", "http/1.1"}}
	client := &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12, RootCAs: m.roots, ServerName: name, ClientSessionCache: tls.NewLRUClientSessionCache(2)}
	for attempt := range 2 {
		serverConn, clientConn := net.Pipe()
		result := make(chan error, 1)
		go func() {
			peer := tls.Server(serverConn, server)
			result <- peer.Handshake()
			peer.Close()
		}()
		peer := tls.Client(clientConn, client)
		_ = peer.SetDeadline(time.Now().Add(5 * time.Second))
		if err := peer.Handshake(); err != nil {
			peer.Close()
			t.Fatal(err)
		}
		if attempt == 1 && !peer.ConnectionState().DidResume {
			peer.Close()
			t.Fatal("TLS session was not resumed")
		}
		peer.Close()
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
}
func TestCertificatePauseAndCurrentRevisionGuard(t *testing.T) {
	_, m, ctx := certificateFixture(t)
	installSyntheticCertificate(t, m, ctx)
	s, e := m.Status(ctx)
	if e != nil {
		t.Fatal(e)
	}
	paused := s.Config
	paused.Enabled = false
	if e = m.configure(ctx, s.AuthorityID, paused); e != nil {
		t.Fatal(e)
	}
	if e = m.configure(ctx, s.AuthorityID, paused); e == nil {
		t.Fatal("stale owner revision accepted")
	}
	result, e := m.Status(ctx)
	if e != nil || result.TLSReady || result.PubliclyTrusted || result.State != "paused" {
		t.Fatal(result, e)
	}
	if _, e = m.GetCertificate(&tls.ClientHelloInfo{ServerName: "probe." + fixtureCertificateNamespace + ".direct.getportico.tv"}); e == nil {
		t.Fatal("paused listener served a certificate")
	}
}
func TestCertificateKeyFilePlainAndStable(t *testing.T) {
	f, m, ctx := certificateFixture(t)
	installSyntheticCertificate(t, m, ctx)
	path := filepath.Join(f.dir, "networking-tls.pem")
	info, e := os.Stat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		t.Fatal("TLS key not a private PEM file", e)
	}
	if _, e = loadTLSKey(path); e != nil {
		t.Fatal("TLS key unreadable", e)
	}
	if _, e = os.Stat(filepath.Join(f.dir, "networking-tls-seal.key")); !os.IsNotExist(e) {
		t.Fatal("seal key file present")
	}
	hello := &tls.ClientHelloInfo{ServerName: "current." + fixtureCertificateNamespace + ".direct.getportico.tv"}
	before, e := m.GetCertificate(hello)
	if e != nil {
		t.Fatal(e)
	}
	restarted, e := NewCertificateManager(ctx, f.store, m.runner, m.transport, f.dir, CertificateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	restarted.roots = m.roots
	after, e := restarted.GetCertificate(hello)
	if e != nil {
		t.Fatal("restart lost installed material", e)
	}
	beforePub, e := x509.MarshalPKIXPublicKey(before.PrivateKey.(crypto.Signer).Public())
	if e != nil {
		t.Fatal(e)
	}
	afterPub, e := x509.MarshalPKIXPublicKey(after.PrivateKey.(crypto.Signer).Public())
	if e != nil || !bytes.Equal(beforePub, afterPub) {
		t.Fatal("TLS key changed across restart")
	}
}
func TestCertificateMaterialScopeAndRenewalMargin(t *testing.T) {
	_, m, ctx := certificateFixture(t)
	installSyntheticCertificate(t, m, ctx)
	_, q, _, e := m.snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	mat, e := m.material(ctx, q.Scope, q.ActiveID)
	if e != nil {
		t.Fatal(e)
	}
	other := q.Scope
	other.ID = strings.Repeat("b", 64)
	if _, e = m.privateKey(other, mat); e == nil {
		t.Fatal("key crossed claim/root scope")
	}
	lifetime := mat.NotAfter.Sub(mat.NotBefore)
	fraction := float64(mat.RenewAt.Sub(mat.NotBefore)) / float64(lifetime)
	if fraction < 0.5799 || fraction > 0.7601 {
		t.Fatal("renewal lacks expected margin", fraction)
	}
	// The whole window, not one draw: never before Hosted's 55% early-renewal gate, always with
	// a quarter of the lifetime left, and spread widely enough to disperse a cohort.
	low, high := 1.0, 0.0
	for i := range 4000 {
		at := certificateRenewAt("request-"+strconv.Itoa(i), mat.NotBefore, mat.NotAfter)
		f := float64(at.Sub(mat.NotBefore)) / float64(lifetime)
		low, high = min(low, f), max(high, f)
	}
	if low < 0.5799 || high > 0.7601 || high-low < 0.17 {
		t.Fatal("renewal window", low, high)
	}
	if _, e = m.validateMaterial(q.Scope, q.Namespace, mat, mat.Chain, mat.NotAfter); e == nil {
		t.Fatal("expired certificate accepted")
	}
}

func TestCertificatePauseResumeCannotInstallLatePendingResponse(t *testing.T) {
	_, m, ctx := certificateFixture(t)
	_, q, _, e := m.snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	n := certificateNamespace{Namespace: fixtureCertificateNamespace, Issuer: "google", Environment: "production"}
	if e = m.publishNamespace(ctx, q.Scope, n); e != nil {
		t.Fatal(e)
	}
	_, q, _, e = m.snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.prepareMaterial(ctx, q); e != nil {
		t.Fatal(e)
	}
	_, pending, _, e := m.snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	mat, e := m.material(ctx, pending.Scope, pending.PendingID)
	if e != nil {
		t.Fatal(e)
	}
	chain := syntheticCertificate(t, m, mat.CSR)
	// Simulate pausing and resuming while the order/chain response is in flight.
	for _, enabled := range []bool{false, true} {
		status, err := m.Status(ctx)
		if err != nil {
			t.Fatal(err)
		}
		config := status.Config
		config.Enabled = enabled
		if err = m.configure(ctx, status.AuthorityID, config); err != nil {
			t.Fatal(err)
		}
	}
	order := certificateOrder{ID: "cert_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32)), State: "issued"}
	if e = m.receiveOrder(ctx, pending, mat, order); e != nil {
		t.Fatal(e)
	}
	mat.OrderID = order.ID
	if e = m.install(ctx, pending, mat, chain); !errors.Is(e, ErrStale) {
		t.Fatal("late response installed after durable cancellation", e)
	}
	if e = m.progress(ctx, pending.Scope, "invalid", "certificate_retry", time.Now().Add(-time.Second)); e != nil {
		t.Fatal(e)
	}
	_, current, _, e := m.snapshot(ctx)
	if e != nil || current.State != "cancel_pending" || current.PendingID != mat.RequestID || current.ActiveID != "" {
		t.Fatal("late failure/receipt lost pending cancellation", current, e)
	}
	// Enabled=true must drain the exact old request, not poll, install or submit.
	calls := 0
	m.transport.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/"+order.ID+"/cancel") {
			t.Fatalf("resumed cancelled request went positive: %s %s", r.Method, r.URL.Path)
		}
		return reply(http.StatusOK, `{"cancelRequested":true}`), nil
	})
	if e = m.step(ctx); e != nil {
		t.Fatal(e)
	}
	_, current, _, e = m.snapshot(ctx)
	if e != nil || calls != 1 || current.PendingID != "" || current.ActiveID != "" {
		t.Fatal("exact cancellation did not finish", current, calls, e)
	}
}
