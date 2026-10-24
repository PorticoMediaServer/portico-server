package playback

import (
	"portico.local/server/internal/assets"
	"testing"
)

// When only Original is available and the engine cannot switch embedded tracks,
// the actual file-first fallback must replace every selected-source fact. An
// unreachable Atmos track cannot lend its object metadata to a plain track.
func TestOriginalAudioFallbackReportsActualTrackObjects(t *testing.T) {
	for _, firstObjects := range []string{"", "atmos"} {
		t.Run("first-"+firstObjects, func(t *testing.T) {
			selectedObjects := "atmos"
			firstChannels, selectedChannels := 2, 8
			if firstObjects != "" {
				selectedObjects = ""
				firstChannels, selectedChannels = 8, 2
			}
			source := videoSource("mp4", "h264", assets.StreamDetail{Width: 1920, Height: 1080, BitDepth: 8, DynamicRange: assets.RangeSDR},
				SourceAudio{Codec: "eac3", Channels: firstChannels, Language: "eng", Default: true, StreamDetail: assets.StreamDetail{ObjectAudio: firstObjects, SampleRate: 48000, StreamBreadth: assets.StreamBreadth{AudioVariant: assets.AudioVariant("eac3", "", firstObjects)}}},
				SourceAudio{Codec: "eac3", Channels: selectedChannels, Language: "jpn", StreamDetail: assets.StreamDetail{ObjectAudio: selectedObjects, SampleRate: 44100, StreamBreadth: assets.StreamBreadth{AudioVariant: assets.AudioVariant("eac3", "", selectedObjects)}}})
			client := appleTVProfile()
			client.EmbeddedAudioSwitching = false
			plan := decide(t, source, client, func(in *DeliveryInput) {
				in.HasAudioChoice, in.AudioStream = true, 2
				in.Config.RemuxEnabled = false
				in.Config.ConversionMode = "never"
				in.TranscodingEnabled = false
				in.Policy.DirectStream = "never"
				in.Policy.Transcode = "never"
			})
			if plan.Strategy != DeliveryOriginal || plan.AudioStream != 1 || plan.Trace == nil || plan.Trace.Audio == nil {
				t.Fatal("file-first fallback did not occur", plan)
			}
			facts := plan.Trace.Audio
			if facts.StreamIndex != 1 || facts.ObjectAudio != firstObjects || facts.Codec != "eac3" || facts.Channels != firstChannels || facts.Language != "eng" || facts.ChosenBy != "only_reachable_track" || facts.Tracks != 2 || facts.SampleRate != 48000 || facts.Variant != assets.AudioVariant("eac3", "", firstObjects) {
				t.Fatalf("unreachable selection contaminated actual source facts: %+v", facts)
			}
		})
	}
}
