package playback

import (
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/decoder"
)

// COMPAT-04: converting a Dolby Vision Profile 5 picture for a screen without
// Dolby Vision says, in the decision, whether its colors are approximate: they
// are unless the tone mapper is libplacebo (on Vulkan), which applies the
// file's reshaping. Profile 8.1 falls back to its HDR10 layer and never is.
func TestDolbyVisionProfile5ConversionSaysWhenColorsAreApproximate(t *testing.T) {
	saved := decoder.CurrentToolchain()
	defer decoder.RestoreToolchain(saved)
	filters := map[string]bool{"zscale": true, "tonemap": true, "libplacebo": true, "scale": true}
	eac3 := SourceAudio{Codec: "eac3", Channels: 6, Language: "eng", Default: true}
	detail := hdr10Detail
	detail.DynamicRange, detail.DolbyVisionProfile, detail.DolbyVisionCompatibility, detail.CodecTag = assets.RangeDolbyVision, 5, 0, "dvh1"
	source := videoSource("mp4", "hevc", detail, eac3)
	noDV := appleTVProfile()
	noDV.Video[1].DolbyVisionProfiles = []int{}

	decoder.ConfigureToolchain(decoder.ToolchainFacts{Filters: filters})
	p := decide(t, source, noDV, nil)
	if !p.ToneMap || !contains(p.ReasonCodes, ReasonDolbyVisionApproximate) {
		t.Fatalf("software tone mapping of Profile 5 must be flagged: %v", p.ReasonCodes)
	}
	decoder.ConfigureToolchain(decoder.ToolchainFacts{Filters: filters, VulkanAvailable: true})
	if p = decide(t, source, noDV, nil); !p.ToneMap || contains(p.ReasonCodes, ReasonDolbyVisionApproximate) {
		t.Fatalf("libplacebo reshapes Profile 5; nothing to warn about: %v", p.ReasonCodes)
	}
	decoder.ConfigureToolchain(decoder.ToolchainFacts{Filters: filters})
	if p = decide(t, source, appleTVProfile(), nil); contains(p.ReasonCodes, ReasonDolbyVisionApproximate) {
		t.Fatalf("a Dolby Vision screen plays it as it is: %v", p.ReasonCodes)
	}
	hdr10 := videoSource("mp4", "hevc", hdr10Detail, eac3)
	if p = decide(t, hdr10, chromeProfile(), nil); !p.ToneMap || contains(p.ReasonCodes, ReasonDolbyVisionApproximate) {
		t.Fatalf("HDR10 tone mapping is not approximate: %v", p.ReasonCodes)
	}
}
