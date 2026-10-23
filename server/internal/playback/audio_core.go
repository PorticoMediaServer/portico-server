package playback

import (
	"portico.local/server/internal/assets"
	"portico.local/server/internal/decoder"
)

const ReasonDTSCoreExtracted = "dts_core_extracted"

func coreAudioCandidate(a *SourceAudio, client ClientProfile, facts *decoder.ToolchainFacts) *SourceAudio {
	if a == nil || a.Codec != "dts" || facts == nil || !facts.BitstreamFilters["dca_core"] {
		return nil
	}
	switch sourceVariant(a) {
	case "dts_hd_hra", "dts_hd_ma", "dtsx":
	default:
		return nil
	}
	core := *a
	core.StreamDetail = assets.StreamDetail{SampleRate: a.SampleRate}
	core.StreamBreadth.AudioVariant = "dts_core"
	// HD/96-24 extensions may change channel/rate facts. Do not advertise a
	// fabricated core layout or bit depth; the extractor retains the core as is.
	core.Channels = 0
	core.Layout = ""
	core.BitsPerSample = 0
	if core.SampleRate > 48000 {
		core.SampleRate = 0
	}
	codec := client.audioCodec("dts")
	passed := client.passthroughCodec("dts")
	if codec != nil && acceptsAudioVariant(codec.Variants, &core, codec.ObjectAudio, false) || passed != nil && acceptsAudioVariant(passed.Variants, &core, passed.ObjectAudio, true) {
		return &core
	}
	return nil
}
