package egress

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestOriginRejectsUnsafeForms(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://127.0.0.1", "https://[::1]", "https://[::ffff:127.0.0.1]", "https://10.0.0.1", "https://169.254.169.254", "https://100.100.100.200", "https://168.63.129.16", "https://example.com:8443", "https://example.com/path", "https://example.com?", "https://example.com?a=1", "https://user:secret@example.com", "https://example.com/#x", "https://example.com/%2f", "https://example.com.", "https://localhost", "https://host.local", "https://host.internal", "https://2130706433", "https://[fe80::1%25en0]"} {
		if _, _, e := Origin(raw, false); e == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"https://Example.COM/", "https://example.com:443", "https://8.8.8.8", "https://[2606:4700:4700::1111]"} {
		if _, dev, e := Origin(raw, false); e != nil || dev {
			t.Errorf("rejected %s: %v", raw, e)
		}
	}
	if _, dev, e := Origin("http://127.0.0.1:19411", true); e != nil || !dev {
		t.Fatal("explicit development origin rejected")
	}
	if _, _, e := Origin("http://localhost:19411", true); e == nil {
		t.Fatal("development DNS literal exception")
	}
}
func TestPublicIPReservedRanges(t *testing.T) {
	for _, v := range []string{"0.0.0.0", "10.1.1.1", "100.64.0.1", "127.0.0.1", "169.254.169.254", "172.16.0.1", "192.168.0.1", "192.0.2.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "255.255.255.255", "::", "::1", "::ffff:10.0.0.1", "64:ff9b::a00:1", "fc00::1", "fe80::1", "ff02::1", "2001:db8::1", "2002:7f00:1::", "3fff::1"} {
		if PublicIP(netip.MustParseAddr(v)) {
			t.Error("accepted reserved", v)
		}
	}
}

type resolverFunc func(context.Context, string, string) ([]netip.Addr, error)

func (f resolverFunc) LookupNetIP(c context.Context, n, h string) ([]netip.Addr, error) {
	return f(c, n, h)
}

type dialerFunc func(context.Context, string, string) (net.Conn, error)

func (f dialerFunc) DialContext(c context.Context, n, a string) (net.Conn, error) { return f(c, n, a) }
func TestDNSRebindingAndMixedAnswersNeverDialPrivate(t *testing.T) {
	calls := 0
	dials := 0
	resolver := resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		calls++
		if calls == 1 {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}, nil
	})
	dialer := dialerFunc(func(_ context.Context, _ string, address string) (net.Conn, error) {
		dials++
		if address != "8.8.8.8:443" {
			t.Fatal("not pinned", address)
		}
		return nil, errors.New("test stops before network")
	})
	client := newClient(false, resolver, dialer)
	transport := client.Transport.(*Transport)
	for range 2 {
		_, _ = transport.dial(context.Background(), "tcp", "example.com:443")
	}
	if calls != 2 || dials != 1 {
		t.Fatalf("DNS calls=%d dials=%d", calls, dials)
	}
	if transport.http.Proxy != nil || transport.http.TLSClientConfig.InsecureSkipVerify || !transport.http.DisableKeepAlives {
		t.Fatal("unsafe transport defaults")
	}
}
func publicTLSFixture(t *testing.T, handler http.Handler) (*http.Client, *int) {
	t.Helper()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "proof.example.com"}, DNSNames: []string{"proof.example.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true}
	der, e := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	calls := 0
	var mu sync.Mutex
	client := newClient(false, resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}), dialerFunc(func(ctx context.Context, n, a string) (net.Conn, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if a != "8.8.8.8:443" {
			return nil, errors.New("not pinned")
		}
		return (&net.Dialer{}).DialContext(ctx, n, server.Listener.Addr().String())
	}))
	pool := x509.NewCertPool()
	parsed, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	pool.AddCert(parsed)
	client.Transport.(*Transport).http.TLSClientConfig.RootCAs = pool
	return client, &calls
}
func TestTLSHostnameAndRedirectBoundary(t *testing.T) {
	client, calls := publicTLSFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://127.0.0.1/private", 302)
			return
		}
		w.WriteHeader(200)
	}))
	response, e := client.Get("https://proof.example.com/ok")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if _, e = client.Get("https://wrong.example.com/ok"); e == nil {
		t.Fatal("accepted TLS hostname mismatch")
	}
	before := *calls
	if _, e = client.Get("https://proof.example.com/redirect"); e == nil {
		t.Fatal("followed redirect")
	}
	if *calls != before+1 {
		t.Fatal("redirect caused another dial")
	}
}
func TestDNSLookupBoundedByClientDeadline(t *testing.T) {
	client := newClient(false, resolverFunc(func(ctx context.Context, _, _ string) ([]netip.Addr, error) { <-ctx.Done(); return nil, ctx.Err() }), dialerFunc(func(context.Context, string, string) (net.Conn, error) { t.Fatal("unexpected dial"); return nil, nil }))
	client.Timeout = 30 * time.Millisecond
	start := time.Now()
	if _, e := client.Get("https://proof.example.com/ok"); e == nil {
		t.Fatal("no timeout")
	}
	if time.Since(start) > time.Second {
		t.Fatal("unbounded lookup")
	}
}

func TestExplicitServerPortDoesNotWidenProviderEgress(t *testing.T) {
	for _, raw := range []string{"https://proof.example.com:32500", "https://proof.example.com:443", "https://[2606:4700:4700::1111]:4443"} {
		got, dev, e := ServerOrigin(raw, false)
		if e != nil || dev || got != raw {
			t.Fatal(raw, got, e)
		}
	}
	for _, raw := range []string{"https://proof.example.com:0", "https://proof.example.com:065535", "https://proof.example.com:65536", "https://proof.example.com:", "https://127.0.0.1:32500"} {
		if _, _, e := ServerOrigin(raw, false); e == nil {
			t.Fatal("unsafe server port", raw)
		}
	}
	calls := 0
	client := newClient(false, resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}), dialerFunc(func(_ context.Context, _, addr string) (net.Conn, error) {
		calls++
		if addr != "8.8.8.8:32500" {
			t.Fatal(addr)
		}
		return nil, errors.New("synthetic stop")
	}))
	tr := client.Transport.(*Transport)
	if _, e := tr.dial(context.Background(), "tcp", "proof.example.com:32500"); !errors.Is(e, ErrUnsafe) || calls != 0 {
		t.Fatal("CA egress widened")
	}
	tr.serverPorts = true
	_, _ = tr.dial(context.Background(), "tcp", "proof.example.com:32500")
	if calls != 1 {
		t.Fatal("server port was not pinned")
	}
}
