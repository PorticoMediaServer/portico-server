package networking

import (
	"net/netip"
	"sort"
)

// PublicAddresses reports this server's own public addresses as the remote
// route step last observed them: routable interface addresses, the detected
// WAN address and mapped external addresses. Empty until a step has run with
// remote access on. Request locality uses it to count a LAN device that the
// router sends through the public address (hairpin NAT) as local.
func (h *ClaimHandler) PublicAddresses() []netip.Addr {
	if h == nil || h.remote == nil {
		return nil
	}
	if p := h.remote.public.Load(); p != nil {
		return *p
	}
	return nil
}

func (m *RemoteManager) recordPublic(addresses, observed map[string]int, configured string) {
	seen := map[netip.Addr]bool{}
	add := func(raw string) {
		if ip, err := netip.ParseAddr(raw); err == nil && publicAddress(ip.Unmap()) {
			seen[ip.Unmap()] = true
		}
	}
	for raw := range addresses {
		add(raw)
	}
	for raw := range observed {
		add(raw)
	}
	add(configured)
	out := make([]netip.Addr, 0, len(seen))
	for ip := range seen {
		out = append(out, ip)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Less(out[j]) })
	m.public.Store(&out)
}
