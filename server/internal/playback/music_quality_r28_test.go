package playback

import (
	"portico.local/server/internal/qualityladder"
	"testing"
)

func TestMusicQualityR28NetworkAndProvenance(t *testing.T) {
	for _, class := range []NetworkClass{NetworkLocal, NetworkRemote, NetworkUnknown} {
		for _, transport := range []string{"wifi", "ethernet", "unknown", "satellite", "cellular", "mobile", "wwan", " CELLULAR "} {
			values := WithPreferenceSources(stubPreferences{"music.qualityAway": "original", "music.qualityMobile": "320"}, map[string]string{"music.qualityAway": "default", "music.qualityMobile": "default"})
			if got := SavedMusicQualityPreference(values, class, transport); got != "" {
				t.Fatal("registry default became explicit", got)
			}
			for _, p := range qualityladder.Music() {
				values = WithPreferenceSources(stubPreferences{"music.qualityAway": p.ID, "music.qualityMobile": p.ID}, map[string]string{"music.qualityAway": "profile-device-class", "music.qualityMobile": "profile-device-class"})
				want := p.ID
				if class == NetworkLocal && NormalizeTransportClass(transport) != "cellular" {
					want = ""
				}
				if got := SavedMusicQualityPreference(values, class, transport); got != want {
					t.Fatal(class, transport, got, want)
				}
			}
		}
	}
	// A music request cannot widen the owner's sound ceiling or remote member cap.
	choice := V1Choice{Limit: true, MaxAudioBitrateBPS: 320000, Remote: true, AdminMaxVideoBitrateBPS: 1500000}
	cfg := DefaultDeliveryConfiguration()
	cfg.MaxAudioBitrateBPS = 96000
	cfg.MaxVideoBitrateBPS = 2000000
	target := choice.target(ResolvedDeliveryPolicy{}, cfg)
	if target.MaxAudioBitrateBPS != 96000 || target.MaxVideoBitrateBPS != 1500000 {
		t.Fatal(target)
	}
	if len(SavedQualities()) != 9 {
		t.Fatal("music expanded video vocabulary")
	}
}
