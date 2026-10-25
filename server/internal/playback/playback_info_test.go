package playback

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/decoder"
)

func TestPlaybackComparisonKeepsOriginalAndRepackagedDistinct(t *testing.T) {
	for _, container := range []string{"mp4", "mkv"} {
		t.Run(container, func(t *testing.T) {
			source := videoSource(container, "h264", assets.StreamDetail{Width: 1920, Height: 1080, BitDepth: 8, DynamicRange: assets.RangeSDR, StreamBreadth: assets.StreamBreadth{InterlaceAnalysis: assets.InterlaceFieldOrderProgressive}}, SourceAudio{Codec: "aac", Channels: 2, Default: true, StreamDetail: assets.StreamDetail{SampleRate: 48000}})
			plan := decide(t, source, appleTVProfile(), nil)
			plan.TargetReach = "remote"
			info := PlaybackInfoForPlan(plan, nil)
			want := "original"
			if container == "mkv" {
				want = "repackaged"
			}
			if info == nil || info.Route != want || info.Routes.Picture != want || info.Routes.Sound != want || info.Network != "away" || len(info.Losses) != 0 || info.Basis != "committed_delivery_plan" {
				t.Fatalf("comparison %+v plan=%+v", info, plan)
			}
			if info.Source.Video.Interlaced == nil || *info.Source.Video.Interlaced || info.Delivered.Video.Interlaced == nil || *info.Delivered.Video.Interlaced {
				t.Fatal("actual progressive fact lost", info)
			}
			if info.Source.Audio.Layout != "" || info.Delivered.Audio.Layout != "" {
				t.Fatal("count became layout", info)
			}
			raw, err := json.Marshal(info)
			if err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{"sourceId", "grant", "hardware", "deviceId", "sessionId", "url", "observed", "screen", "pcm"} {
				if strings.Contains(string(raw), `"`+private+`"`) {
					t.Fatalf("private/inferred field %s in %s", private, raw)
				}
			}
			*info.Delivered.Video.Interlaced = true
			if *plan.Trace.Video.Interlaced || *info.Source.Video.Interlaced {
				t.Fatal("comparison mutated source snapshot")
			}
		})
	}
	if PlaybackInfoForPlan(DeliveryPlan{}, nil) != nil || PlaybackInfoForPlan(DeliveryPlan{Trace: &DecisionTrace{}}, nil) != nil {
		t.Fatal("invented legacy source report")
	}
}

func TestPlaybackComparisonProfile7RewriteIsConvertedWithoutRelabelingRawRoute(t *testing.T) {
	for _, layer := range []string{"MEL", "FEL"} {
		t.Run(layer, func(t *testing.T) {
			plan := decide(t, observedProfile7Source(layer), appleTVProfile(), func(in *DeliveryInput) { in.Server = packetQualifiedUnitFacts(); in.FMP4Output = true })
			before, _ := json.Marshal(plan.Trace)
			info := PlaybackInfoForPlan(plan, nil)
			if plan.Strategy != DeliveryCopyRemux || plan.VideoAction != "copy" || info.Route != "converted" || info.Routes.Picture != "converted" || info.Source.Video.DolbyVisionProfile != 7 || info.Source.Video.DolbyVisionEnhancementType != layer || info.Source.Video.DolbyVisionRPU == nil || !*info.Source.Video.DolbyVisionRPU || info.Delivered.Video.DolbyVisionProfile != 8 || info.Delivered.Video.DolbyVisionCompatibility != 1 || info.Delivered.Video.DolbyVisionEnhancementType != "" {
				t.Fatalf("source/delivery/route mixed: %+v", info)
			}
			conversion := info.Delivered.Video.Conversion
			if conversion == nil || conversion.Kind != decoder.DolbyVisionProfile7To81 || !conversion.EnhancementDiscarded || conversion.FELResidualDiscarded != (layer == "FEL") || conversion.SourceEnhancementType != layer {
				t.Fatal("packet conversion loss omitted", conversion)
			}
			if len(info.Losses) != 0 {
				t.Fatal("DV preserved rewrite claimed DV lost", info.Losses)
			}
			after, _ := json.Marshal(plan.Trace)
			if string(before) != string(after) {
				t.Fatal("report changed committed source")
			}
		})
	}
}

func TestPlaybackComparisonTargetFactsNeverBecomeRendererObservations(t *testing.T) {
	progressive := false
	plan := DeliveryPlan{Strategy: DeliveryVideoConversion, VideoAction: "convert", AudioAction: "convert", SourceVideoCodec: "hevc", SourceAudioCodec: "truehd", OutputVideoCodec: "h264", OutputAudioCodec: "flac", OutputContainer: "fmp4_hls", OutputBitDepth: 8, OutputRange: assets.RangeSDR, TargetHeight: 720, AudioChannels: 8, AudioSampleRate: 96000, Deinterlace: true, ReasonCodes: []string{ReasonHeightExceedsPolicy}, VideoBitrateBPS: 3000000, AudioBitrateBPS: 0,
		Trace:            &DecisionTrace{Container: "matroska", Video: &TraceVideo{Codec: "hevc", BitDepth: 10, Width: 1920, Height: 1080, FrameRate: 24, FrameRateNumerator: 24, FrameRateDenominator: 1, FrameRateMode: "vfr", Interlaced: &progressive, DynamicRange: assets.RangeDolbyVision, DolbyVisionProfile: 7, HDR10Plus: true, MasterDisplay: "source mastering", MaxCLL: "1000,400"}, Audio: &TraceAudio{Codec: "truehd", Profile: "source profile", Variant: "truehd_atmos", ObjectAudio: "atmos", Channels: 8, Layout: "7.1", SampleRate: 96000, BitDepth: 24}},
		ConversionTarget: &decoder.ConversionTarget{VideoCodec: "h264", VideoProfile: "high", BitDepth: 8, DynamicRange: assets.RangeSDR, AudioCodec: "flac", AudioChannels: 8, AudioSampleRate: 96000, AudioBitDepth: 24, AudioChannelLayout: "7.1"},
	}
	subtitle := &PlaybackSubtitleFacts{Format: "pgs", Language: "eng", Origin: "embedded", Rendering: "burnedIn"}
	info := PlaybackInfoForPlan(plan, subtitle)
	if info.Route != "converted" || info.Reason.Kind != "subtitles" || info.Routes.Subtitles != "converted" || info.Delivered.Video.Width != 1280 || info.Delivered.Video.Height != 720 || info.Delivered.Video.Profile != "high" || info.Delivered.Video.MasterDisplay != "" || info.Delivered.Video.DolbyVisionProfile != 0 || info.Delivered.Video.HDR10Plus != nil || info.Delivered.Video.FrameRateMode != "" || info.Delivered.Video.Interlaced == nil || *info.Delivered.Video.Interlaced {
		t.Fatalf("stale source facts on target: %+v", info)
	}
	if info.Source.Audio.ObjectAudio != "atmos" || info.Delivered.Audio.ObjectAudio != "" || info.Delivered.Audio.Codec != "flac" || info.Delivered.Audio.BitDepth != 24 || info.Delivered.Audio.SampleRate != 96000 || info.Delivered.Audio.Layout != "7.1" {
		t.Fatal("FLAC target lost full rate/depth or invented object retention", info)
	}
	want := []PlaybackLoss{{Feature: "dolbyVision", Replacement: PlaybackReplacement{DynamicRange: "sdr"}}, {Feature: "hdr", Replacement: PlaybackReplacement{DynamicRange: "sdr"}}, {Feature: "atmos", Replacement: PlaybackReplacement{AudioCodec: "flac", Channels: 8, Layout: "7.1"}}}
	if !reflect.DeepEqual(info.Losses, want) || info.EstimatedBitrateKbps != 0 {
		t.Fatal("structured losses/unknown lossless total", info)
	}
	if info.Source.Subtitles.Rendering != "" || info.Source.Subtitles.Format != "pgs" || info.Delivered.Subtitles.Rendering != "burnedIn" || info.Delivered.Subtitles.Format != "" || subtitle.Rendering != "burnedIn" {
		t.Fatal("subtitle source and rendering mixed", info, subtitle)
	}
}

func TestPlaybackComparisonOmitsPartialOrUnappliedRoutesAndRates(t *testing.T) {
	plan := DeliveryPlan{VideoAction: "none", AudioAction: "none", Trace: &DecisionTrace{Video: &TraceVideo{Codec: "hevc"}}}
	if PlaybackInfoForPlan(plan, nil) != nil {
		t.Fatal("unknown/drop became Original")
	}
	plan = DeliveryPlan{Strategy: DeliveryVideoConversion, VideoAction: "convert", AudioAction: "convert", VideoBitrateBPS: 3000000, AudioBitrateBPS: 320000, OutputVideoCodec: "h264", OutputAudioCodec: "aac", Trace: &DecisionTrace{Video: &TraceVideo{Codec: "hevc"}, Audio: &TraceAudio{Codec: "truehd"}}}
	if info := PlaybackInfoForPlan(plan, nil); info.EstimatedBitrateKbps != 3320 {
		t.Fatal("known aggregate estimate", info)
	}
	plan.VideoBitrateBPS = 0
	if info := PlaybackInfoForPlan(plan, nil); info.EstimatedBitrateKbps != 0 {
		t.Fatal("audio-only rate misreported as aggregate", info)
	}
}

// VIDEO.md VS-10 asks for the sound's format and whether it is passed through
// or decoded. The committed recipe knows which: a passthrough route hands the
// bitstream to the sink, everything else is decoded to PCM first.
func TestPlaybackInfoReportsTheSoundDelivery(t *testing.T) {
	plan := DeliveryPlan{Strategy: DeliveryOriginal, VideoAction: "original", AudioAction: "original", OutputAudioCodec: "eac3", AudioPassthrough: true,
		Trace: &DecisionTrace{Audio: &TraceAudio{Codec: "eac3", ObjectAudio: "atmos", Channels: 8, Layout: "7.1"}}}
	info := PlaybackInfoForPlan(plan, nil)
	if info == nil || info.Source.Audio == nil || info.Source.Audio.Delivery != "" || info.Delivered.Audio == nil || info.Delivered.Audio.Delivery != "passthrough" {
		t.Fatalf("a passed-through track was not reported as one: %+v", info)
	}
	plan.AudioPassthrough = false
	plan.AudioAction = "convert"
	plan.OutputAudioCodec = "aac"
	info = PlaybackInfoForPlan(plan, nil)
	if info == nil || info.Delivered.Audio == nil || info.Delivered.Audio.Delivery != "decoded" || info.Delivered.Audio.Channels != 0 {
		t.Fatalf("a decoded track was not reported as one: %+v", info)
	}
}
