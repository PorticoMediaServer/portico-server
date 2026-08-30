package playbackplan

import (
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
)

func audioOnlyTuple(container, codec, layout string, channels int) playbackcap.DeliveryTuple {
	return playbackcap.DeliveryTuple{Kind: playbackcap.MediaAudio, Protocol: "http", Container: container, Audio: playbackcap.Audio{Codec: codec, Layout: layout, MaxChannels: channels}, Subtitle: playbackcap.Subtitle{Mode: playbackcap.SubtitleNone}}
}

func audioOnlyRequest(f mediafacts.Facts, tuples ...playbackcap.DeliveryTuple) Request {
	return Request{Facts: f, Capabilities: playbackcap.Resolution{EvidenceID: "native:audio-test", Tuples: tuples}, Policy: MaximumFidelity, Protocol: "http", Selection: Selection{AudioIndex: ip(1)}}
}

func TestAudioOnlyCodecFamiliesDoNotRequireVideoCapabilities(t *testing.T) {
	for _, tc := range []struct{ codec, container string }{
		{"mp3", "mp3"}, {"aac", "m4a"}, {"alac", "m4a"}, {"flac", "flac"}, {"opus", "ogg"}, {"vorbis", "ogg"}, {"pcm_s24le", "wav"},
	} {
		t.Run(tc.codec, func(t *testing.T) {
			f := baseFacts()
			f.Video, f.Subtitles, f.Container = nil, nil, tc.container
			f.Audio[0].Codec, f.Audio[0].Layout, f.Audio[0].Channels = tc.codec, "stereo", 2
			p, err := Build(audioOnlyRequest(f, audioOnlyTuple(tc.container, tc.codec, "stereo", 2)))
			if err != nil || p.Mode != DirectPlay || p.MediaKind != playbackcap.MediaAudio || len(p.Streams) != 1 || p.Streams[0].Kind != "audio" {
				t.Fatalf("audio-only %s: plan=%#v err=%v", tc.codec, p, err)
			}
		})
	}
}

func TestAudioOnlyConversionCanUseGeneratedHLSAAC(t *testing.T) {
	f := baseFacts()
	f.Video, f.Subtitles, f.Container = nil, nil, "flac"
	f.Audio[0] = mediafacts.Audio{Index: 1, Codec: "flac", Layout: "mono", Channels: 1, SampleRate: 48000}
	hlsAAC := audioOnlyTuple("mpegts", "aac", "mono", 1)
	hlsAAC.Protocol = "hls"
	hlsAAC.Audio.Profile = "lc"
	hlsAAC.Audio.Route = "decode"
	plan, err := Build(Request{
		Facts: f,
		Capabilities: playbackcap.Resolution{EvidenceID: "authenticated_runtime:web-audio", Tuples: []playbackcap.DeliveryTuple{
			audioOnlyTuple("mp3", "mp3", "mono", 1), hlsAAC,
		}},
		Policy: MaximumFidelity, Protocol: "hls", Selection: Selection{AudioIndex: ip(1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != DirectStream || plan.MediaKind != playbackcap.MediaAudio || plan.Protocol != "hls" || plan.Container != "mpegts" || plan.SegmentFormat != "mpegts" || plan.Audio.Codec != "aac" || plan.Audio.Layout != "mono" || plan.Audio.Channels != 1 || plan.Audio.Passthrough {
		t.Fatalf("FLAC generated-HLS plan = %#v", plan)
	}
}

func TestGaplessDecisionIsSealedAndConversionNeverClaimsProof(t *testing.T) {
	f := baseFacts()
	f.Video, f.Subtitles, f.Container = nil, nil, "mp3"
	f.Audio[0].Codec, f.Audio[0].SampleRate = "mp3", 44100
	f.Audio[0].EncoderDelaySamples, f.Audio[0].EncoderPaddingSamples = 576, 288
	f.Audio[0].GaplessConfidence, f.Audio[0].GaplessEvidence = "exact", "ffprobe packet skip_samples side data"
	p, _ := Build(audioOnlyRequest(f, audioOnlyTuple("mp3", "mp3", "stereo", 2)))
	if p.Audio.Gapless.Status != "preserved" || p.Audio.Gapless.EncoderDelaySamples != 576 || !has(p.Reasons, ReasonGaplessPreserved) {
		t.Fatalf("copy did not seal exact gapless facts: %#v", p.Audio.Gapless)
	}
	p, _ = Build(audioOnlyRequest(f, audioOnlyTuple("m4a", "aac", "stereo", 2)))
	if p.Audio.Gapless.Status != "unverified" || !has(p.Reasons, ReasonGaplessUnverified) || p.Audio.Gapless.Reason == "" {
		t.Fatalf("lossy conversion overclaimed gapless output: %#v", p.Audio.Gapless)
	}
	p, _ = Build(audioOnlyRequest(f, audioOnlyTuple("mka", "mp3", "stereo", 2)))
	if p.Audio.Gapless.Status != "unverified" || p.Audio.Passthrough != true {
		t.Fatalf("container-changing packet copy overclaimed gapless preservation: %#v", p.Audio)
	}
}

func audioTuple(codec, layout string, channels int, objects bool) playbackcap.DeliveryTuple {
	t := avTuple("matroska", "h264", "sdr", codec, layout, channels, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	t.Audio.ObjectPassthrough = objects
	return t
}

func TestAudioTupleCopyAndConversionConformance(t *testing.T) {
	for _, tc := range []struct {
		name, source, target, inLayout, outLayout string
		inChannels, outChannels                   int
		wantMode                                  Mode
	}{
		{"aac copy", "aac", "aac", "stereo", "stereo", 2, 2, DirectPlay},
		{"ac3 copy", "ac3", "ac3", "5.1", "5.1", 6, 6, DirectPlay},
		{"eac3 copy", "eac3", "eac3", "5.1", "5.1", 6, 6, DirectPlay},
		{"truehd copy", "truehd", "truehd", "7.1", "7.1", 8, 8, DirectPlay},
		{"dts copy", "dts", "dts", "5.1", "5.1", 6, 6, DirectPlay},
		{"flac copy", "flac", "flac", "5.1", "5.1", 6, 6, DirectPlay},
		{"opus copy", "opus", "opus", "5.1", "5.1", 6, 6, DirectPlay},
		{"pcm copy", "pcm_s24le", "pcm_s24le", "stereo", "stereo", 2, 2, DirectPlay},
		{"truehd to eac3", "truehd", "eac3", "7.1", "5.1", 8, 6, DirectStream},
		{"dts to opus", "dts", "opus", "5.1", "stereo", 6, 2, DirectStream},
		{"side surround copy", "eac3", "eac3", "5.1(side)", "5.1(side)", 6, 6, DirectPlay},
		{"side surround to stereo", "eac3", "aac", "5.1(side)", "stereo", 6, 2, DirectStream},
		{"quad to stereo", "flac", "aac", "quad", "stereo", 4, 2, DirectStream},
		{"flac to pcm", "flac", "pcm_s24le", "stereo", "stereo", 2, 2, DirectStream},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := baseFacts()
			f.Audio[0].Codec, f.Audio[0].Layout, f.Audio[0].Channels = tc.source, tc.inLayout, tc.inChannels
			p, err := Build(request(f, audioTuple(tc.target, tc.outLayout, tc.outChannels, tc.source == tc.target)))
			if err != nil {
				t.Fatal(err)
			}
			if p.Mode != tc.wantMode || p.Audio.Codec != tc.target || p.Audio.Layout != tc.outLayout || p.Audio.Channels != tc.outChannels {
				t.Fatalf("unexpected decision: %#v", p)
			}
			if (tc.source == tc.target) != p.Audio.Passthrough {
				t.Fatalf("copy disclosure wrong: %#v", p.Audio)
			}
		})
	}
}

func TestAudioPlannerFailsClosedOnUnknownOrInconsistentLayouts(t *testing.T) {
	for _, mutate := range []func(*playbackcap.DeliveryTuple, *int, *string){
		func(_ *playbackcap.DeliveryTuple, channels *int, _ *string) { *channels = 5 },
		func(_ *playbackcap.DeliveryTuple, _ *int, layout *string) { *layout = "unknown-order" },
		func(tuple *playbackcap.DeliveryTuple, _ *int, _ *string) {
			tuple.Audio.Layout = "7.1(wide)"
			tuple.Audio.MaxChannels = 8
		},
	} {
		f := baseFacts()
		tuple := audioTuple("aac", "stereo", 2, false)
		mutate(&tuple, &f.Audio[0].Channels, &f.Audio[0].Layout)
		p, _ := Build(request(f, tuple))
		if p.Mode != Unsupported {
			t.Fatalf("unsafe layout accepted: %#v", p)
		}
	}
}

func TestObjectAudioOnlySurvivesExactSupportedPassthrough(t *testing.T) {
	f := baseFacts()
	f.Audio[0].Codec, f.Audio[0].Layout, f.Audio[0].Channels = "truehd", "7.1", 8
	f.Audio[0].ObjectAudio, f.Audio[0].ObjectAudioEvidence = "atmos", "verified-side-data"
	p, _ := Build(request(f, audioTuple("truehd", "7.1", 8, true), audioTuple("eac3", "5.1", 6, false)))
	if !p.Audio.Passthrough || !p.Audio.ObjectsPreserved || p.Audio.Codec != "truehd" {
		t.Fatalf("object passthrough lost: %#v", p)
	}

	p, _ = Build(request(f, audioTuple("eac3", "5.1", 6, false)))
	if p.Audio.ObjectsPreserved || !has(p.Reasons, ReasonObjectAudioLost) {
		t.Fatalf("object loss not disclosed: %#v", p)
	}
}

func TestMaximumFidelityAudioLadderIsIndependentOfTupleOrder(t *testing.T) {
	f := baseFacts()
	f.Audio[0].Codec, f.Audio[0].Layout, f.Audio[0].Channels = "truehd", "7.1", 8
	a := audioTuple("aac", "stereo", 2, false)
	b := audioTuple("eac3", "5.1", 6, false)
	p1, _ := Build(request(f, a, b))
	p2, _ := Build(request(f, b, a))
	if p1.Audio.Codec != "eac3" || p1.Digest != p2.Digest {
		t.Fatalf("unstable ladder: %#v %#v", p1.Audio, p2.Audio)
	}
}
