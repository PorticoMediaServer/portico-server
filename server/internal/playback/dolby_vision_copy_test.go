package playback

import (
	"strings"
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/decoder"
)

func observedProfile7Source(layer string) *DeliverySource {
	return videoSource("mkv", "hevc", assets.StreamDetail{
		Width: 1920, Height: 1080, BitDepth: 10, FrameRate: 24,
		DynamicRange: assets.RangeDolbyVision, DolbyVisionProfile: 7,
		DolbyVisionCompatibility: 6, DolbyVisionLevel: 6, EnhancementLayer: true,
		StreamBreadth: assets.StreamBreadth{DolbyVisionMetadataKnown: true, DolbyVisionRPU: true, DolbyVisionBaseLayer: true, DolbyVisionEnhancementType: layer},
	}, SourceAudio{Codec: "aac", Channels: 2, Default: true, StreamDetail: assets.StreamDetail{SampleRate: 48000}})
}

func packetQualifiedUnitFacts() *decoder.ToolchainFacts {
	// Synthetic qualification is confined to this unit fixture. Production
	// obtains these facts only from the selected canonical bundle verifier.
	return &decoder.ToolchainFacts{Identity: "unit-packet-qualified", DolbyVisionProfile7To81: true,
		BitstreamFilters: map[string]bool{"dovi_profile": true, "dovi_rpu": true, "filter_units": true}}
}

func TestProfile7CopyUsesQualifiedRewriteAndDelivered81Signalling(t *testing.T) {
	for _, layer := range []string{"MEL", "FEL"} {
		t.Run(layer, func(t *testing.T) {
			source := observedProfile7Source(layer)
			p := decide(t, source, appleTVProfile(), func(in *DeliveryInput) {
				in.Server, in.FMP4Output = packetQualifiedUnitFacts(), true
			})
			if p.Strategy != DeliveryCopyRemux || p.VideoAction != "copy" || p.OutputContainer != "fmp4_hls" || p.OutputRange != assets.RangeDolbyVision || p.ConversionTarget == nil || p.ConversionTarget.DolbyVisionConversion != decoder.DolbyVisionProfile7To81 {
				t.Fatalf("qualified copy not selected: %+v", p)
			}
			if p.Trace.Video.DolbyVisionProfile != 7 || p.Trace.Video.DolbyVisionCompatibility != 6 || source.Video.DolbyVisionProfile != 7 || !source.Video.EnhancementLayer {
				t.Fatal("delivered metadata overwrote source facts", p.Trace.Video, source.Video)
			}
			if !containsString(p.Trace.Notes, NoteDolbyVisionEnhancementDiscarded) || containsString(p.Trace.Notes, NoteDolbyVisionFELDiscarded) != (layer == "FEL") {
				t.Fatal("enhancement loss not exact", p.Trace.Notes)
			}
			_, args, _, err := p.CodecArgs()
			if err != nil || !strings.Contains(strings.Join(args, " "), "-bsf:v dovi_profile=mode=2 -c:v copy") || strings.Contains(strings.Join(args, " "), "dovi_rpu=mode") {
				t.Fatal("copy producer did not use checked helper", args, err)
			}
			manifest := string(hlsMasterManifest(&p))
			if hevcSampleEntry(&p) != "dvh1" || videoCodecString(&p) != "dvh1.08.06" || !strings.Contains(manifest, `SUPPLEMENTAL-CODECS="dvh1.08.06/db1p"`) || strings.Contains(manifest, "dvh1.07") {
				t.Fatal("HLS signalled source instead of delivered profile", manifest)
			}
		})
	}
}

func TestProfile7RewriteRequiresSourceBundleAndExactClientVariant(t *testing.T) {
	for name, edit := range map[string]func(*DeliverySource, *ClientProfile, *decoder.ToolchainFacts){
		"unqualified bundle": func(_ *DeliverySource, _ *ClientProfile, f *decoder.ToolchainFacts) {
			f.DolbyVisionProfile7To81 = false
		},
		"no identity": func(_ *DeliverySource, _ *ClientProfile, f *decoder.ToolchainFacts) { f.Identity = "" },
		"stock filter only": func(_ *DeliverySource, _ *ClientProfile, f *decoder.ToolchainFacts) {
			delete(f.BitstreamFilters, "dovi_profile")
		},
		"configuration only": func(s *DeliverySource, _ *ClientProfile, _ *decoder.ToolchainFacts) {
			s.Video.DolbyVisionMetadataKnown = false
		},
		"missing RPU": func(s *DeliverySource, _ *ClientProfile, _ *decoder.ToolchainFacts) { s.Video.DolbyVisionRPU = false },
		"missing base": func(s *DeliverySource, _ *ClientProfile, _ *decoder.ToolchainFacts) {
			s.Video.DolbyVisionBaseLayer = false
		},
		"unjoined enhancement": func(s *DeliverySource, _ *ClientProfile, _ *decoder.ToolchainFacts) { s.Video.EnhancementLayer = false },
		"client HLG only": func(_ *DeliverySource, c *ClientProfile, _ *decoder.ToolchainFacts) {
			c.Video[1].DolbyVisionProfile8Compatibility = []int{4}
		},
		"client lacks DV": func(_ *DeliverySource, c *ClientProfile, _ *decoder.ToolchainFacts) {
			c.Video[1].DolbyVisionProfiles = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			source, client, facts := observedProfile7Source("FEL"), appleTVProfile(), packetQualifiedUnitFacts()
			edit(source, &client, facts)
			p := decide(t, source, client, func(in *DeliveryInput) { in.Server, in.FMP4Output = facts, true })
			if p.OutputRange == assets.RangeDolbyVision || p.ConversionTarget != nil && p.ConversionTarget.DolbyVisionConversion != "" || containsString(p.Trace.Notes, NoteDolbyVisionFELDiscarded) {
				t.Fatal("unqualified rewrite selected", p)
			}
		})
	}
}

func TestDeclaredProfile7OriginalRemainsOriginal(t *testing.T) {
	source := observedProfile7Source("FEL")
	source.Container = "mp4"
	client := appleTVProfile()
	client.Video[1].DolbyVisionProfiles = append(client.Video[1].DolbyVisionProfiles, 7)
	p := decide(t, source, client, func(in *DeliveryInput) { in.Server, in.FMP4Output = packetQualifiedUnitFacts(), true })
	if p.Strategy != DeliveryOriginal || p.VideoAction != "original" || p.ConversionTarget != nil || containsString(p.Trace.Notes, NoteDolbyVisionEnhancementDiscarded) {
		t.Fatal("otherwise playable declared source was rewritten", p)
	}
}
