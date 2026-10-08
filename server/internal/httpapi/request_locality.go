package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"portico.local/server/internal/operations"
)

// localityPolicy is the settings a locality decision needs, parsed once per
// settings save (secureSetting) rather than per request.
type localityPolicy struct {
	lan           []netip.Prefix
	proxies       []netip.Prefix
	treatWANAsLAN bool
}

func newLocalityPolicy(s operations.Settings) localityPolicy {
	out := localityPolicy{treatWANAsLAN: s.TreatWANAsLAN}
	for _, raw := range s.LANNetworks {
		if prefix, ok := operations.ParseNetwork(raw); ok {
			out.lan = append(out.lan, prefix)
		}
	}
	for _, raw := range s.TrustedProxies {
		if prefix, ok := operations.ParseNetwork(raw); ok {
			out.proxies = append(out.proxies, prefix)
		}
	}
	return out
}

// trustedProxyPrefixes is every proxy this server believes: the operator's
// PORTICO_TRUSTED_PROXY_CIDRS plus the owner's trustedProxies setting.
func (d Dependencies) trustedProxyPrefixes(ctx context.Context) []netip.Prefix {
	setting, err := d.connectionSetting(ctx)
	if err != nil || setting == nil || len(setting.locality.proxies) == 0 {
		return d.TrustedProxies
	}
	out := make([]netip.Prefix, 0, len(d.TrustedProxies)+len(setting.locality.proxies))
	out = append(out, d.TrustedProxies...)
	return append(out, setting.locality.proxies...)
}

// requestIsRemote is the one answer to "did this request come from outside
// this server's LAN?" — used by the remote sign-in policy, playback admission,
// the remote bitrate limits and the upload budget.
//
//  1. The client is the socket peer, unless the peer is a trusted proxy: then it
//     is the right-most X-Forwarded-For address that is not itself a trusted
//     proxy. A trusted proxy that forwards nothing readable is remote, so a
//     stripped or malformed header never turns an internet client into a LAN one.
//  2. Configured LAN networks are the whole LAN (as in Plex): any other address,
//     loopback included, is remote. That is how an owner makes a guest network,
//     or a proxy they have not marked trusted, count as remote.
//  3. With none configured, private, loopback and link-local addresses are LAN.
//  4. With treatWanAsLan on, the server's own public address is LAN: a router
//     that sends home devices through its public address (hairpin NAT).
//
// Unknown is remote: remote is the restrictive answer for every caller.
func (d Dependencies) requestIsRemote(r *http.Request) bool {
	setting, err := d.connectionSetting(r.Context())
	if err != nil {
		return true
	}
	policy := localityPolicy{treatWANAsLAN: true}
	if setting != nil {
		policy = setting.locality
	}
	var public []netip.Addr
	if policy.treatWANAsLAN && d.Networking != nil {
		public = d.Networking.PublicAddresses()
	}
	return remoteClient(r, d.trustedProxyPrefixes(r.Context()), policy, public)
}

func remoteClient(r *http.Request, proxies []netip.Prefix, policy localityPolicy, public []netip.Addr) bool {
	client, ok := peerAddress(r)
	if !ok {
		return true
	}
	if containsAddress(proxies, client) {
		if client, ok = forwardedClient(r, proxies); !ok {
			return true
		}
	}
	if policy.treatWANAsLAN {
		for _, own := range public {
			if own == client {
				return false
			}
		}
	}
	if len(policy.lan) > 0 {
		return !containsAddress(policy.lan, client)
	}
	return !client.IsLoopback() && !client.IsPrivate() && !client.IsLinkLocalUnicast()
}

func peerAddress(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = strings.Trim(r.RemoteAddr, "[]")
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || addr.Zone() != "" {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// forwardedClient reads X-Forwarded-For from the right: each hop appends the
// address it received from, so the first address that is not a trusted proxy
// is the one the outermost trusted proxy saw. Every value is parsed; any
// malformed entry makes the whole header unreadable.
func forwardedClient(r *http.Request, proxies []netip.Prefix) (netip.Addr, bool) {
	var hops []string
	for name, rows := range r.Header {
		if strings.EqualFold(name, "X-Forwarded-For") {
			for _, row := range rows {
				hops = append(hops, strings.Split(row, ",")...)
			}
		}
	}
	if len(hops) == 0 || len(hops) > 32 {
		return netip.Addr{}, false
	}
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil || addr.Zone() != "" || addr.IsUnspecified() || addr.IsMulticast() {
			return netip.Addr{}, false
		}
		addr = addr.Unmap()
		if !containsAddress(proxies, addr) || i == 0 {
			return addr, true
		}
	}
	return netip.Addr{}, false
}

func containsAddress(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
