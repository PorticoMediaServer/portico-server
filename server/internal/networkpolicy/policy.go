// Package networkpolicy owns inbound client address and home-network decisions.
// It does not govern outbound fetches, which have separate SSRF restrictions.
package networkpolicy

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
)

var defaultOnce sync.Once
var defaultNetworks []string

// DefaultHomeNetworks includes the attached subnets without naming or detecting
// particular VPNs, proxies or container runtimes. Owners can replace this list.
func DefaultHomeNetworks() []string {
	defaultOnce.Do(func() {
		defaultNetworks = []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "fe80::/10", "fc00::/7"}
		seen := map[string]bool{}
		for _, s := range defaultNetworks {
			seen[s] = true
		}
		interfaces, _ := net.Interfaces()
		for _, iface := range interfaces {
			if iface.Flags&net.FlagUp == 0 {
				continue
			}
			addresses, _ := iface.Addrs()
			for _, a := range addresses {
				p, err := netip.ParsePrefix(a.String())
				if err != nil || p.Addr().IsUnspecified() || p.Addr().IsMulticast() {
					continue
				}
				s := p.Masked().String()
				if !seen[s] {
					defaultNetworks = append(defaultNetworks, s)
					seen[s] = true
				}
			}
		}
	})
	return append([]string{}, defaultNetworks...)
}

func Contains(prefixes []netip.Prefix, addr netip.Addr) bool {
	addr = addr.Unmap().WithZone("")
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsMulticast() {
		return false
	}
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func ParseAddress(raw string) (netip.Addr, bool) {
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	addr, err := netip.ParseAddr(strings.Trim(raw, "[]"))
	if err != nil || addr.IsUnspecified() || addr.IsMulticast() {
		return netip.Addr{}, false
	}
	return addr.Unmap().WithZone(""), true
}

func Peer(r *http.Request) (netip.Addr, bool) { return ParseAddress(r.RemoteAddr) }

// headerValues also handles deliberately noncanonical header maps.
func headerValues(r *http.Request, key string) []string {
	var out []string
	for k, values := range r.Header {
		if strings.EqualFold(k, key) {
			out = append(out, values...)
		}
	}
	return out
}

// Forwarded is bounded and rejects ambiguous or malformed chains in full,
// including attacker-supplied entries to the left of the real client.
func Forwarded(r *http.Request, proxies []netip.Prefix) (netip.Addr, bool) {
	xff := headerValues(r, "X-Forwarded-For")
	var hops []netip.Addr
	if len(xff) > 0 {
		for _, row := range xff {
			if len(row) > 8192 {
				return netip.Addr{}, false
			}
			for _, raw := range strings.Split(row, ",") {
				a, err := netip.ParseAddr(strings.TrimSpace(raw))
				if err != nil || a.Zone() != "" || a.IsUnspecified() || a.IsMulticast() {
					return netip.Addr{}, false
				}
				hops = append(hops, a.Unmap())
				if len(hops) > 32 {
					return netip.Addr{}, false
				}
			}
		}
	} else {
		elements, ok := forwardedElements(r)
		if !ok {
			return netip.Addr{}, false
		}
		for _, fields := range elements {
			raw := fields["for"]
			a, ok := ParseAddress(raw)
			if !ok || strings.Contains(raw, "%") {
				return netip.Addr{}, false
			}
			hops = append(hops, a)
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		if !Contains(proxies, hops[i]) || i == 0 {
			return hops[i], true
		}
	}
	return netip.Addr{}, false
}

func Client(r *http.Request, proxies []netip.Prefix) (netip.Addr, bool) {
	peer, ok := Peer(r)
	if !ok || !Contains(proxies, peer) {
		return peer, ok
	}
	return Forwarded(r, proxies)
}

func forwardedElements(r *http.Request) ([]map[string]string, bool) {
	var out []map[string]string
	for _, row := range headerValues(r, "Forwarded") {
		if len(row) > 8192 {
			return nil, false
		}
		// IPs and origins do not contain escaped quotes, semicolons or commas.
		for _, element := range strings.Split(row, ",") {
			fields := map[string]string{}
			for _, part := range strings.Split(element, ";") {
				key, value, found := strings.Cut(strings.TrimSpace(part), "=")
				key = strings.ToLower(key)
				if !found || fields[key] != "" {
					return nil, false
				}
				if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
					value = value[1 : len(value)-1]
				}
				if value == "" || strings.ContainsAny(value, "\"\\\r\n\t ") {
					return nil, false
				}
				fields[key] = value
			}
			out = append(out, fields)
			if len(out) > 32 {
				return nil, false
			}
		}
	}
	return out, len(out) > 0
}

// Origin honors the nearest proxy's scheme and host only when that socket peer
// is trusted. Untrusted forwarding headers never change the request origin.
func Origin(r *http.Request, proxies []netip.Prefix) string {
	scheme, host := "http", r.Host
	if r.TLS != nil {
		scheme = "https"
	}
	peer, ok := Peer(r)
	if ok && Contains(proxies, peer) {
		if elements, ok := forwardedElements(r); ok {
			last := elements[len(elements)-1]
			if p := last["proto"]; p == "http" || p == "https" {
				scheme = p
			}
			if h := last["host"]; validHost(h) {
				host = h
			}
		} else {
			if p := singleHeader(r, "X-Forwarded-Proto"); p == "http" || p == "https" {
				scheme = p
			}
			if h := singleHeader(r, "X-Forwarded-Host"); validHost(h) {
				host = h
			}
		}
	}
	if !validHost(host) {
		return ""
	}
	return scheme + "://" + strings.ToLower(host)
}

func singleHeader(r *http.Request, key string) string {
	values := headerValues(r, key)
	if len(values) != 1 || len(values[0]) > 300 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
func validHost(host string) bool {
	if host == "" || len(host) > 255 || strings.ContainsAny(host, "/\\@?#% \r\n\t,") {
		return false
	}
	u, err := url.Parse("http://" + host)
	return err == nil && u.Hostname() != "" && u.User == nil
}
