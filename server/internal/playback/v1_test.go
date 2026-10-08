package playback

import "testing"

// Review P4: remote caps (the member's and the server-wide one) apply only to a
// remote request; a LAN viewer is never capped or told "admin_cap_remote_bitrate".
func TestV1ChoiceAppliesRemoteCapsOnlyRemotely(t *testing.T) {
	cfg := DeliveryConfiguration{MaxVideoBitrateBPS: 4_000_000}
	lan := V1Choice{AdminMaxVideoBitrateBPS: 2_000_000}
	if got := lan.target(ResolvedDeliveryPolicy{NetworkClass: NetworkLocal}, cfg); got.MaxVideoBitrateBPS != 0 {
		t.Fatalf("LAN original capped at %d", got.MaxVideoBitrateBPS)
	}
	if lan.AdminCapped() {
		t.Fatal("LAN request reported as admin-capped")
	}
	remote := V1Choice{AdminMaxVideoBitrateBPS: 2_000_000, Remote: true}
	if got := remote.target(ResolvedDeliveryPolicy{}, cfg); got.MaxVideoBitrateBPS != 2_000_000 || !remote.AdminCapped() {
		t.Fatalf("remote member cap: %d", got.MaxVideoBitrateBPS)
	}
	server := V1Choice{Remote: true, Limit: true, MaxVideoBitrateBPS: 20_000_000}
	if got := server.target(ResolvedDeliveryPolicy{}, cfg); got.MaxVideoBitrateBPS != 4_000_000 {
		t.Fatalf("server-wide remote limit: %d", got.MaxVideoBitrateBPS)
	}
	cellular := V1Choice{}
	if got := cellular.target(ResolvedDeliveryPolicy{NetworkClass: NetworkCellular}, cfg); got.MaxVideoBitrateBPS != 4_000_000 {
		t.Fatalf("cellular is remote: %d", got.MaxVideoBitrateBPS)
	}
	limited := V1Choice{Limit: true, MaxVideoBitrateBPS: 1_000_000, MaxVideoHeight: 720}
	if got := limited.target(ResolvedDeliveryPolicy{NetworkClass: NetworkLocal}, cfg); got.MaxVideoBitrateBPS != 1_000_000 || got.MaxVideoHeight != 720 || got.Kind != QualityAutomatic {
		t.Fatalf("client limit %+v", got)
	}
}
