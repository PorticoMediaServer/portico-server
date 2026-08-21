package app

import (
	"context"
	"os"
	"testing"
	"time"
)

// This opt-in check discovers the active LAN gateway without creating,
// replacing, or deleting a router mapping. It is safe to run while a manual
// Portico port forward exists.
func TestUPnPDiscoveryOnLocalNetwork(t *testing.T) {
	if os.Getenv("PORTICO_TEST_UPNP_DISCOVERY") != "1" {
		t.Skip("set PORTICO_TEST_UPNP_DISCOVERY=1 to inspect the current LAN gateway")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	service, err := discoverUPnPService(ctx)
	if err != nil {
		t.Fatalf("discover UPnP WAN service: %v", err)
	}
	if service.ControlURL == "" || service.ServiceType == "" {
		t.Fatalf("incomplete UPnP WAN service: %#v", service)
	}
}

// This opt-in lifecycle check uses external port 32501 and always attempts
// cleanup. It deliberately does not alter Portico's default/manual 32500 rule.
func TestUPnPMappingLifecycleOnLocalNetwork(t *testing.T) {
	if os.Getenv("PORTICO_TEST_UPNP_MAPPING") != "1" {
		t.Skip("set PORTICO_TEST_UPNP_MAPPING=1 to create and remove a temporary router mapping")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	mapper := UPnPRouterMapper{}
	result := mapper.AddMapping(ctx, 32500, 32501, "Portico UPnP verification")
	if result.Status != "mapped" {
		t.Fatalf("create temporary UPnP mapping: %#v", result)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cleanupCancel()
		cleanup := mapper.DeleteMapping(cleanupCtx, 32500, 32501)
		if cleanup.Status != "removed" {
			t.Errorf("remove temporary UPnP mapping: %#v", cleanup)
		}
	})
}
