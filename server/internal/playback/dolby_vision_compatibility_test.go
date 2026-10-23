package playback

import (
	"portico.local/server/internal/assets"
	"testing"
)

func TestDolbyVisionProfile8CompatibilityUsesExactDeclaredVariant(t *testing.T) {
	client := appleTVProfile()
	codec := client.videoCodec("hevc")
	if codec == nil {
		t.Fatal("fixture codec missing")
	}
	v := &SourceVideo{Codec: "hevc", StreamDetail: assets.StreamDetail{DynamicRange: assets.RangeDolbyVision, DolbyVisionProfile: 8, DolbyVisionCompatibility: 1}}
	for _, ids := range [][]int{nil, {1}, {4}, {1, 4}, {2}} {
		codec.DolbyVisionProfile8Compatibility = ids
		for _, compat := range []int{1, 2, 4} {
			v.DolbyVisionCompatibility = compat
			want := len(ids) == 0 || containsInt(ids, compat)
			if got := nativeDolbyVision(v, codec, client); got != want {
				t.Fatalf("ids=%v source=%d got=%v", ids, compat, got)
			}
		}
	}
	v.DolbyVisionProfile = 5
	v.DolbyVisionCompatibility = 0
	codec.DolbyVisionProfile8Compatibility = []int{1}
	if !nativeDolbyVision(v, codec, client) {
		t.Fatal("profile-8 narrowing changed explicit profile 5")
	}
}
