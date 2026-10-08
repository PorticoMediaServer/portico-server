package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
)

type clientIPKey struct{}

func ParseTrustedProxies(value string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	if strings.TrimSpace(value) == "" {
		return out, nil
	}
	loop4 := netip.MustParsePrefix("127.0.0.0/8")
	for _, raw := range strings.Split(value, ",") {
		p, e := netip.ParsePrefix(strings.TrimSpace(raw))
		if e != nil || p.Addr().Is4In6() || p.Addr().Zone() != "" {
			return nil, errors.New("invalid trusted proxy CIDR")
		}
		p = p.Masked()
		if !(p.Addr().Is4() && p.Bits() >= 8 && loop4.Contains(p.Addr())) && p != netip.MustParsePrefix("::1/128") {
			return nil, errors.New("trusted proxy CIDR must be loopback-only")
		}
		out = append(out, p)
	}
	return out, nil
}
func (h *Handler) clientAddress(r *http.Request) (string, error) {
	remote, e := netip.ParseAddrPort(r.RemoteAddr)
	if e != nil {
		return "", errors.New("invalid remote address")
	}
	ip := remote.Addr().Unmap()
	if ip.Zone() != "" {
		return "", errors.New("invalid remote address")
	}
	trusted := false
	for _, p := range h.TrustedProxies {
		if p.Contains(ip) {
			trusted = true
			break
		}
	}
	if !trusted {
		return ip.String(), nil
	}
	values := r.Header.Values("X-Forwarded-For")
	if len(values) == 0 {
		return ip.String(), nil
	}
	if len(values) != 1 {
		return "", errors.New("multiple forwarded addresses")
	}
	forwarded, e := netip.ParseAddr(strings.TrimSpace(values[0]))
	if e != nil || forwarded.Zone() != "" || forwarded.IsUnspecified() || forwarded.IsMulticast() {
		return "", errors.New("invalid forwarded address")
	}
	return forwarded.Unmap().String(), nil
}
func peer(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	p, e := netip.ParseAddrPort(r.RemoteAddr)
	if e != nil {
		return "invalid"
	}
	return p.Addr().Unmap().String()
}
func withClientAddress(r *http.Request, ip string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), clientIPKey{}, ip))
}
