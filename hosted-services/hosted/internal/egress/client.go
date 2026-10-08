// Package egress confines calls to server-advertised endpoints. DNS validation and
// dialing are one operation; a checked hostname is never resolved again by Dial.
package egress

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrUnsafe = errors.New("unsafe server endpoint")
var blocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/3"), netip.MustParsePrefix("168.63.129.16/32"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

func PublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, p := range blocked {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// Origin permits only ASCII public HTTPS origins on port443. Development HTTP
// requires an explicit IP loopback literal and an explicit caller option.
func Origin(raw string, allowLoopback bool) (string, bool, error) {
	return origin(raw, allowLoopback, false)
}

// ServerOrigin retains an explicitly advertised TLS port (including 443).
// It is only for server endpoint proof/wake, never CA or identity-provider egress.
func ServerOrigin(raw string, allowLoopback bool) (string, bool, error) {
	return origin(raw, allowLoopback, true)
}
func numericPort(raw string) bool {
	n, e := strconv.Atoi(raw)
	return e == nil && n > 0 && n <= 65535 && strconv.Itoa(n) == raw
}
func origin(raw string, allowLoopback, serverPorts bool) (string, bool, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || u.Host == "" {
		return "", false, ErrUnsafe
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.HasSuffix(host, ".") {
		return "", false, ErrUnsafe
	}
	for _, r := range host {
		if r > 127 || r <= 32 || r == '%' || r == '\\' {
			return "", false, ErrUnsafe
		}
	}
	ip, iperr := netip.ParseAddr(host)
	if allowLoopback && u.Scheme == "http" && iperr == nil && ip.IsLoopback() && ip.Zone() == "" {
		port := u.Port()
		if port == "" {
			port = "80"
		}
		if _, e = net.LookupPort("tcp", port); e != nil {
			return "", false, ErrUnsafe
		}
		return "http://" + net.JoinHostPort(host, port), true, nil
	}
	if u.Scheme != "https" || (u.Port() != "" && (!numericPort(u.Port()) || !serverPorts && u.Port() != "443")) || strings.HasSuffix(u.Host, ":") {
		return "", false, ErrUnsafe
	}
	if iperr == nil {
		if !PublicIP(ip) {
			return "", false, ErrUnsafe
		}
		if ip.Is6() {
			host = "[" + host + "]"
		}
	} else {
		if !strings.Contains(host, ".") || len(host) > 253 {
			return "", false, ErrUnsafe
		}
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", false, ErrUnsafe
			}
			for _, r := range label {
				if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
					return "", false, ErrUnsafe
				}
			}
		}
		for _, suffix := range []string{".localhost", ".local", ".internal", ".lan", ".home", ".invalid", ".test"} {
			if strings.HasSuffix(host, suffix) {
				return "", false, ErrUnsafe
			}
		}
	}
	if serverPorts && u.Port() != "" {
		host += ":" + u.Port()
	}
	return "https://" + host, false, nil
}

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}
type Transport struct {
	resolver      Resolver
	dialer        Dialer
	allowLoopback bool
	serverPorts   bool
	http          *http.Transport
}

func NewClient(allowLoopback bool) *http.Client {
	return newClient(allowLoopback, net.DefaultResolver, &net.Dialer{Timeout: 3 * time.Second, KeepAlive: -1})
}

// NewServerClient retains DNS-rebinding, public-address, TLS and redirect checks,
// but permits the server's explicit TCP port. NewClient stays HTTPS/443 only.
func NewServerClient(allowLoopback bool) *http.Client {
	client := newClient(allowLoopback, net.DefaultResolver, &net.Dialer{Timeout: 3 * time.Second, KeepAlive: -1})
	client.Transport.(*Transport).serverPorts = true
	return client
}
func newClient(allow bool, resolver Resolver, dialer Dialer) *http.Client {
	t := &Transport{resolver: resolver, dialer: dialer, allowLoopback: allow}
	t.http = &http.Transport{Proxy: nil, DialContext: t.dial, DisableKeepAlives: true, DisableCompression: true, ForceAttemptHTTP2: false, MaxResponseHeaderBytes: 16 << 10, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	return &http.Client{Transport: t, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrUnsafe }}
}
func originURL(raw string, allow, serverPorts bool) (string, bool, error) {
	return origin(raw, allow, serverPorts)
}
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	origin := r.URL.Scheme + "://" + r.URL.Host
	if _, _, e := originURL(origin, t.allowLoopback, t.serverPorts); e != nil {
		return nil, e
	}
	if r.URL.User != nil || r.URL.RawQuery != "" || r.URL.Fragment != "" {
		return nil, ErrUnsafe
	}
	return t.http.RoundTrip(r)
}
func (t *Transport) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, ErrUnsafe
	}
	if ip, e := netip.ParseAddr(host); e == nil {
		if t.allowLoopback && ip.IsLoopback() && ip.Zone() == "" {
			return t.dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
		}
		if !PublicIP(ip) || !numericPort(port) || !t.serverPorts && port != "443" {
			return nil, ErrUnsafe
		}
		return t.dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
	}
	if !numericPort(port) || !t.serverPorts && port != "443" {
		return nil, ErrUnsafe
	}
	ips, e := t.resolver.LookupNetIP(ctx, "ip", host)
	if e != nil {
		return nil, e
	}
	if len(ips) == 0 || len(ips) > 16 {
		return nil, ErrUnsafe
	}
	for _, ip := range ips {
		if !PublicIP(ip) {
			return nil, ErrUnsafe
		}
	}
	// Connect to the validated literal. TLS still verifies the original URL host.
	return t.dialer.DialContext(ctx, "tcp", net.JoinHostPort(ips[0].String(), port))
}
