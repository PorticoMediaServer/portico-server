package playback

import "testing"

func TestSavedQualityReadsTheNetworkLaneAndItsProvenance(t *testing.T) {
	values := stubPreferences{"quality.home": "original", "quality.remote": "480p"}
	sources := map[string]string{"quality.home": "default", "quality.remote": "profile-device-class"}
	for _, tc := range []struct {
		class NetworkClass
		want  string
	}{
		{NetworkLocal, "720p-high"},
		{NetworkRemote, "480p"},
		{NetworkUnknown, "480p"},
	} {
		got := ResolveDeliveryPolicy(WithPreferenceSources(values, sources), tc.class, LocalityUnknown, "cellular", DefaultDeliveryConfiguration())
		if got.QualityMode != tc.want {
			t.Fatalf("%s selected %s, want %s", tc.class, got.QualityMode, tc.want)
		}
	}
	// Explicit Original must not become the cellular default, and a reset must
	// restore the transport default without changing the other network's choice.
	sources["quality.home"] = "profile-device-class"
	if got := ResolveDeliveryPolicy(WithPreferenceSources(values, sources), NetworkLocal, LocalityLocal, "cellular", DefaultDeliveryConfiguration()); got.QualityMode != "original" {
		t.Fatal(got)
	}
	sources["quality.remote"] = "default"
	if got := ResolveDeliveryPolicy(WithPreferenceSources(values, sources), NetworkRemote, LocalityRemote, "cellular", DefaultDeliveryConfiguration()); got.QualityMode != "720p-high" {
		t.Fatal(got)
	}
}

func TestQualityDefaultsR25NetworkProvenanceAndCaps(t *testing.T) {
	cfg := DefaultDeliveryConfiguration()
	if cfg.MaxVideoBitrateBPS != 0 || cfg.MaxAudioBitrateBPS != 0 || cfg.MaxVideoHeight != 0 {
		t.Fatal(cfg)
	}
	for _, class := range []NetworkClass{NetworkLocal, NetworkRemote, NetworkUnknown} {
		for _, transport := range []string{"wifi", "ethernet", "unknown", "cellular", "mobile", "wwan"} {
			values := WithPreferenceSources(stubPreferences{"quality.home": "original", "quality.remote": "original"}, map[string]string{"quality.home": "default", "quality.remote": "default"})
			got := ResolveDeliveryPolicy(values, class, LocalityUnknown, transport, cfg)
			want := "original"
			if NormalizeTransportClass(transport) == "cellular" {
				want = "720p-high"
			}
			if got.QualityMode != want || got.NetworkClass != class || got.MaxVideoBitrateBPS != 0 {
				t.Fatalf("%s %s %+v", class, transport, got)
			}
			for _, chosen := range SavedQualities() {
				explicit := WithPreferenceSources(stubPreferences{"quality.home": chosen, "quality.remote": chosen}, map[string]string{"quality.home": "deviceClass", "quality.remote": "deviceClass"})
				if got := ResolveDeliveryPolicy(explicit, class, LocalityUnknown, transport, cfg); got.QualityMode != chosen {
					t.Fatal(chosen, got)
				}
			}
		}
	}
	cfg.MaxVideoBitrateBPS = 2000000
	policy := ResolveDeliveryPolicy(nil, NetworkRemote, LocalityRemote, "cellular", cfg)
	choice, _ := namedQualityChoice(policy.QualityMode)
	target := choice.target(policy, cfg)
	if target.MaxVideoBitrateBPS != 2000000 || target.MaxVideoHeight != 720 {
		t.Fatal(target)
	}
	// A member's inherited zero leaves the server cap in force; explicit smaller caps narrow it.
	choice.AdminMaxVideoBitrateBPS = 1500000
	if got := choice.target(policy, cfg); got.MaxVideoBitrateBPS != 1500000 || got.MaxVideoHeight != 480 {
		t.Fatal(got)
	}
	r, ok := SelectedRung(QualityOffers(1080, true, true, openPolicy(), 1000000), "")
	if !ok || r.ID != "original" {
		t.Fatal(r, ok)
	}
	plan := planFor(t, DeliveryInput{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Policy: openPolicy(), Config: DefaultDeliveryConfiguration()})
	if plan.QualityID != "original" || plan.VideoAction != "original" {
		t.Fatal(plan)
	}
}
