package httpapi

import (
	"context"
	"net/http"
	"net/netip"
	"portico.local/server/internal/networkpolicy"
	"portico.local/server/internal/operations"
)

type localityPolicy struct{ lan, proxies []netip.Prefix }

func newLocalityPolicy(s operations.Settings) localityPolicy {
	out := localityPolicy{}
	for _, raw := range s.LANNetworks {
		if p, ok := operations.ParseNetwork(raw); ok {
			out.lan = append(out.lan, p)
		}
	}
	for _, raw := range s.TrustedProxies {
		if p, ok := operations.ParseNetwork(raw); ok {
			out.proxies = append(out.proxies, p)
		}
	}
	return out
}
func (d Dependencies) trustedProxyPrefixes(ctx context.Context) []netip.Prefix {
	setting, err := d.connectionSetting(ctx)
	if err != nil || setting == nil {
		return d.TrustedProxies
	}
	return setting.locality.proxies
}

// RequestIsRemote is the sole viewer locality decision. Unknown clients are remote.
func (d Dependencies) RequestIsRemote(r *http.Request) bool { return d.requestIsRemote(r) }
func (d Dependencies) requestIsRemote(r *http.Request) bool {
	setting, err := d.connectionSetting(r.Context())
	if err != nil {
		return true
	}
	policy := newLocalityPolicy(operations.DefaultSettings())
	if setting != nil {
		policy = setting.locality
	}
	return remoteClient(r, d.trustedProxyPrefixes(r.Context()), policy, nil)
}
func remoteClient(r *http.Request, proxies []netip.Prefix, policy localityPolicy, _ []netip.Addr) bool {
	client, ok := networkpolicy.Client(r, proxies)
	return !ok || !networkpolicy.Contains(policy.lan, client)
}
func peerAddress(r *http.Request) (netip.Addr, bool) { return networkpolicy.Peer(r) }
func forwardedClient(r *http.Request, proxies []netip.Prefix) (netip.Addr, bool) {
	return networkpolicy.Forwarded(r, proxies)
}
func containsAddress(prefixes []netip.Prefix, addr netip.Addr) bool {
	return networkpolicy.Contains(prefixes, addr)
}
func (d Dependencies) sameRequestOrigin(r *http.Request, origin string) bool {
	return origin != "" && origin == networkpolicy.Origin(r, d.trustedProxyPrefixes(r.Context()))
}
