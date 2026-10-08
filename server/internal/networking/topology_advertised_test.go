package networking

import (
	"context"
	"net"
	"testing"
)

func TestVirtualInterfacePrefixes(t *testing.T) {
	for _, name := range []string{"docker0", "br-1a2b3c", "veth9f2a", "virbr0", "vmnet8", "vboxnet0", "cni0", "flannel.1", "caliabc", "lxcbr0", "lxdbr0", "podman0", "zt0", "wg0", "tun0", "tap0", "utun3"} {
		if !VirtualInterface(name) {
			t.Fatalf("VirtualInterface(%q)=false, want true", name)
		}
	}
	// br0 is a real Linux bridge on NAS boxes and must not match (br- has the dash).
	for _, name := range []string{"br0", "eth0", "en0", "enp3s0", "wlan0", "lo"} {
		if VirtualInterface(name) {
			t.Fatalf("VirtualInterface(%q)=true, want false", name)
		}
	}
}

// stubTopology replaces the interface enumeration and the gateway probe for
// one test. The gateway probe returns local for the given gateway.
func stubTopology(t *testing.T, ifaces []ifaceAddrs, gateway, local string) {
	t.Helper()
	oldIfaces, oldDial := topologyInterfaces, topologyDialLocal
	topologyInterfaces = func() ([]ifaceAddrs, error) { return ifaces, nil }
	topologyDialLocal = func(g string) (string, error) {
		if g == gateway {
			return local, nil
		}
		return "", errTopologyStubNoRoute
	}
	t.Cleanup(func() { topologyInterfaces, topologyDialLocal = oldIfaces, oldDial })
}

var errTopologyStubNoRoute = errTopologyNoRoute{}

type errTopologyNoRoute struct{}

func (errTopologyNoRoute) Error() string { return "no route" }

func topoIface(name string, addrs ...string) ifaceAddrs {
	return ifaceAddrs{Name: name, Flags: net.FlagUp, Addrs: addrs}
}

func lanSet(t *testing.T, lan []string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, ip := range lan {
		out[ip] = true
	}
	return out
}

func TestAdvertisedInterfaceRestrictsLAN(t *testing.T) {
	stubTopology(t, []ifaceAddrs{
		topoIface("eth0", "192.168.1.20/24"),
		topoIface("eth1", "192.168.1.21/24"),
		topoIface("docker0", "172.17.0.1/16"),
	}, "192.168.1.1", "192.168.1.20")
	topo, e := observeTopology(context.Background(), "0.0.0.0:32500", "192.168.1.1", "eth0")
	if e != nil {
		t.Fatal(e)
	}
	got := lanSet(t, topo.LAN)
	if !got["192.168.1.20"] || got["192.168.1.21"] || got["172.17.0.1"] {
		t.Fatalf("advertised eth0 should leave only its addresses in LAN: %v", topo.LAN)
	}
	if topo.LocalAddress != "192.168.1.20" {
		t.Fatalf("local address: %q", topo.LocalAddress)
	}
	if topo.ErrorCode != "" {
		t.Fatalf("error code: %q", topo.ErrorCode)
	}
}

func TestAdvertisedInterfaceMissingFallsBack(t *testing.T) {
	stubTopology(t, []ifaceAddrs{
		topoIface("eth0", "192.168.1.20/24"),
		topoIface("eth1", "192.168.1.21/24"),
		topoIface("docker0", "172.17.0.1/16"),
	}, "192.168.1.1", "192.168.1.20")
	topo, e := observeTopology(context.Background(), "0.0.0.0:32500", "192.168.1.1", "eth99")
	if e != nil {
		t.Fatal(e)
	}
	got := lanSet(t, topo.LAN)
	if !got["192.168.1.20"] || !got["192.168.1.21"] {
		t.Fatalf("a missing interface falls back to automatic: %v", topo.LAN)
	}
	if got["172.17.0.1"] {
		t.Fatalf("automatic still skips virtual interfaces: %v", topo.LAN)
	}
	if topo.ErrorCode != "advertised_interface_missing" {
		t.Fatalf("error code: %q", topo.ErrorCode)
	}
}

func TestGatewayInterfaceNeverSkipped(t *testing.T) {
	stubTopology(t, []ifaceAddrs{
		topoIface("eth0", "192.168.1.20/24"),
		topoIface("docker0", "172.17.0.1/16"),
		topoIface("veth9f2a", "10.5.0.2/16"),
	}, "172.17.0.1", "172.17.0.1")
	topo, e := observeTopology(context.Background(), "0.0.0.0:32500", "172.17.0.1", "")
	if e != nil {
		t.Fatal(e)
	}
	got := lanSet(t, topo.LAN)
	if !got["172.17.0.1"] {
		t.Fatalf("the gateway interface is never skipped: %v", topo.LAN)
	}
	if !got["192.168.1.20"] {
		t.Fatalf("a physical interface stays: %v", topo.LAN)
	}
	if got["10.5.0.2"] {
		t.Fatalf("a non-gateway virtual interface is skipped: %v", topo.LAN)
	}
	if topo.LocalAddress != "172.17.0.1" {
		t.Fatalf("local address: %q", topo.LocalAddress)
	}
}
