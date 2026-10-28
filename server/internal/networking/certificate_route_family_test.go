package networking

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type routeFamilyCredentials struct{ currentErr error }

func (*routeFamilyCredentials) InstalledCredential(context.Context, Intent) (*Secret, error) {
	return NewSecret([]byte(strings.Repeat("s", 32)))
}
func (f *routeFamilyCredentials) Current(context.Context, Intent) error { return f.currentErr }

func TestRoutePublicationTransportConfiguration(t *testing.T) {
	tr, _, _, _ := httpFixture(t)
	// Partial literals used by fixtures must remain safe to clean up.
	(&HTTPTransport{client: tr.client}).CloseIdleConnections()
	(&HTTPTransport{}).CloseIdleConnections()
	seen := map[*http.Transport]bool{}
	for _, client := range []*http.Client{tr.client, tr.routeIPv4, tr.routeIPv6} {
		rt := client.Transport.(*http.Transport)
		if seen[rt] {
			t.Fatal("route connection pools are shared")
		}
		seen[rt] = true
		if rt.Proxy != nil || rt.TLSClientConfig.InsecureSkipVerify || rt.TLSClientConfig.MinVersion != tls.VersionTLS12 || rt.TLSClientConfig.RootCAs != devHostedRoots ||
			rt.TLSHandshakeTimeout != 4*time.Second || rt.ResponseHeaderTimeout != 4*time.Second || rt.IdleConnTimeout != 30*time.Second ||
			rt.MaxIdleConns != 2 || rt.MaxIdleConnsPerHost != 2 || rt.MaxConnsPerHost != 2 || rt.MaxResponseHeaderBytes != claimHTTPBound || !rt.DisableCompression ||
			client.Timeout != 8*time.Second || client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
			t.Fatal("control transport safety policy changed")
		}
	}
}

// These fixtures only accept loopback TLS. Two listeners on the same port model
// a default IPv6 egress beside advertised IPv4, and the inverse. The real route
// dialers are retained; only ordinary traffic's default route is controlled.
func TestRoutePublicationFamilyAndConnectionReuse(t *testing.T) {
	for _, advertised := range []string{"66.94.102.39", "2605:a144:2327:3137::1"} {
		t.Run(advertised, func(t *testing.T) {
			type observed struct{ path, peer string }
			var mu sync.Mutex
			var requests []observed
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(io.LimitReader(r.Body, claimHTTPBound+1))
				if err != nil {
					t.Error("fixture request body failed")
				}
				if strings.HasSuffix(r.URL.Path, "/direct-route") {
					var in struct {
						Address string `json:"address"`
						Port    int    `json:"port"`
					}
					if json.Unmarshal(raw, &in) != nil || in.Address != advertised || in.Port != 32500 {
						t.Error("publication payload changed")
					}
				}
				mu.Lock()
				requests = append(requests, observed{r.URL.Path, r.RemoteAddr})
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, "{}")
			})
			start := func(network, address string) *httptest.Server {
				t.Helper()
				listener, err := net.Listen(network, address)
				if err != nil {
					t.Fatal(err)
				}
				s := httptest.NewUnstartedServer(handler)
				_ = s.Listener.Close()
				s.Listener = listener
				s.StartTLS()
				t.Cleanup(s.Close)
				return s
			}
			v4 := start("tcp4", "127.0.0.1:0")
			_, port, _ := net.SplitHostPort(v4.Listener.Addr().String())
			v6 := start("tcp6", net.JoinHostPort("::1", port))
			target, ordinary := v4, v6
			wrongAddress := "2605:a144:2327:3137::1"
			if strings.Contains(advertised, ":") {
				target, ordinary = v6, v4
				wrongAddress = "66.94.102.39"
			}
			tr, v, _, _ := httpFixture(t)
			tr.origin = target.URL
			tr.credentials = &routeFamilyCredentials{}
			v.InstallationAcknowledged = true
			for _, client := range []*http.Client{tr.client, tr.routeIPv4, tr.routeIPv6} {
				roots := x509.NewCertPool()
				roots.AddCert(target.Certificate())
				client.Transport.(*http.Transport).TLSClientConfig.RootCAs = roots
			}
			tr.client.Transport.(*http.Transport).DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", ordinary.Listener.Addr().String())
			}
			ctx := fixtureClaimContext(t)
			control := func() {
				t.Helper()
				if err := tr.controlRequest(ctx, http.MethodPost, "/v1/unit-fixture", nil, &struct{}{}, nil, claimHTTPBound); err != nil {
					t.Fatal(err)
				}
			}
			publication := func(address string) error {
				return tr.certificateCall(ctx, v, "route", "", struct {
					Address string `json:"address"`
					Port    int    `json:"port"`
				}{address, 32500}, &struct{}{})
			}
			control() // Warm an ordinary keepalive connection in the wrong family.
			for range 2 {
				if err := publication(advertised); err != nil {
					t.Fatal(err)
				}
			}
			// The endpoint is a literal of the opposite family. A warm default or
			// route pool must not make this request succeed through a fallback.
			if err := publication(wrongAddress); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("wrong-family endpoint did not fail closed: %v", err)
			}
			for _, operation := range []string{"namespace", "route_withdraw"} {
				if err := tr.certificateCall(ctx, v, operation, "", nil, &struct{}{}); err != nil {
					t.Fatal(err)
				}
			}
			control()
			mu.Lock()
			got := append([]observed(nil), requests...)
			mu.Unlock()
			if len(got) != 6 {
				t.Fatalf("unexpected requests or family fallback: %d", len(got))
			}
			for i, request := range got {
				want := ordinary.Listener.Addr().(*net.TCPAddr).IP
				if i == 1 || i == 2 {
					want = target.Listener.Addr().(*net.TCPAddr).IP
				}
				peer, _, err := net.SplitHostPort(request.peer)
				if err != nil || !net.ParseIP(peer).Equal(want) {
					t.Fatalf("request %d used the wrong family", i)
				}
			}
			if got[1].peer != got[2].peer || got[0].peer != got[3].peer || got[0].peer != got[4].peer || got[0].peer != got[5].peer {
				t.Fatal("same-family keepalive was not reused")
			}
			tr.CloseIdleConnections()
			control()
			if err := publication(advertised); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(requests) != 8 || requests[6].peer == got[0].peer || requests[7].peer == got[1].peer {
				t.Fatal("idle connections were not closed in every pool")
			}
		})
	}
}

func TestRoutePublicationRejectsMissingFamilyAndPreservesFences(t *testing.T) {
	tr, v, _, _ := httpFixture(t)
	credentials := &routeFamilyCredentials{}
	tr.credentials = credentials
	v.InstallationAcknowledged = true
	calls := 0
	for _, client := range []*http.Client{tr.client, tr.routeIPv4, tr.routeIPv6} {
		client.Transport = claimRoundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			return reply(200, "{}"), nil
		})
	}
	for _, address := range []string{"", "host.example", "::ffff:66.94.102.39", "fe80::1%eth0"} {
		if err := tr.certificateCall(fixtureClaimContext(t), v, "route", "", map[string]any{"address": address, "port": 32500}, &struct{}{}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid family accepted: %v", err)
		}
	}
	in := map[string]any{"address": "66.94.102.39", "port": 32500}
	if err := tr.certificateCall(context.Background(), v, "route", "", in, &struct{}{}); err == nil {
		t.Fatal("missing authority accepted")
	}
	credentials.currentErr = ErrStale
	if err := tr.certificateCall(fixtureClaimContext(t), v, "route", "", in, &struct{}{}); !errors.Is(err, ErrStale) {
		t.Fatal("credential fence bypassed")
	}
	v.InstallationAcknowledged = false
	if err := tr.certificateCall(fixtureClaimContext(t), v, "route", "", in, &struct{}{}); !errors.Is(err, ErrStale) {
		t.Fatal("installation fence bypassed")
	}
	if calls != 0 {
		t.Fatal("request escaped an authority or family fence")
	}
}
