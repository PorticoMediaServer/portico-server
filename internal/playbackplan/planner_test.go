package playbackplan

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
)

func ip(v int) *int { return &v }
func baseFacts() mediafacts.Facts {
	return mediafacts.Facts{Version: mediafacts.SchemaVersion, Source: mediafacts.Source{Fingerprint: "sha256:source", Revision: "r1"}, Container: "matroska", DurationUS: 10_000_000, DurationConfidence: mediafacts.ConfidenceExact, Video: []mediafacts.Video{{Index: 0, Codec: "h264", Profile: "high", CodedWidth: 1920, CodedHeight: 1080, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p", BitDepth: 8, FrameRate: mediafacts.Rational{Num: 24, Den: 1}}}, Audio: []mediafacts.Audio{{Index: 1, Codec: "aac", Layout: "stereo", Channels: 2}}, Subtitles: []mediafacts.Subtitle{{Index: 2, Codec: "subrip", Kind: "text", Language: "en", Disposition: mediafacts.Disposition{Forced: true}}}}
}
func avTuple(container, vcodec, hdr, acodec, layout string, channels int, sub playbackcap.Subtitle) playbackcap.DeliveryTuple {
	return playbackcap.DeliveryTuple{Kind: playbackcap.MediaAudiovisual, Protocol: "http", Container: container, Video: playbackcap.Video{Codec: vcodec, Profile: "high", PixelFormat: "yuv420p", HDR: hdr, BitDepth: 10, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60}, Audio: playbackcap.Audio{Codec: acodec, Layout: layout, MaxChannels: channels}, Subtitle: sub}
}
func request(f mediafacts.Facts, tuples ...playbackcap.DeliveryTuple) Request {
	return Request{Facts: f, Capabilities: playbackcap.Resolution{EvidenceID: "native:test", Tuples: tuples}, Policy: MaximumFidelity, Protocol: "http", Selection: Selection{VideoIndex: ip(0), AudioIndex: ip(1)}}
}

func TestPlannerModes(t *testing.T) {
	tests := []struct {
		name, container, v, a string
		mode                  Mode
	}{{"direct", "matroska", "h264", "aac", DirectPlay}, {"remux", "mp4", "h264", "aac", Remux}, {"audio convert", "matroska", "h264", "opus", DirectStream}, {"video convert", "matroska", "hevc", "aac", VideoTranscode}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, e := Build(request(baseFacts(), avTuple(tt.container, tt.v, "sdr", tt.a, "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})))
			if e != nil {
				t.Fatal(e)
			}
			if p.Mode != tt.mode {
				t.Fatalf("got %s", p.Mode)
			}
			if e = p.Validate(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestBrowserHLSMonoTupleDeliversMPEG2MP2WithoutUpmix(t *testing.T) {
	facts := baseFacts()
	facts.Container = "mpegts"
	facts.Video[0] = mediafacts.Video{Index: 0, Codec: "mpeg2video", Profile: "main", CodedWidth: 320, CodedHeight: 180, PixelFormat: "yuv420p", BitDepth: 8, FrameRate: mediafacts.Rational{Num: 25, Den: 1}, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}}
	facts.Audio[0] = mediafacts.Audio{Index: 1, Codec: "mp2", Layout: "mono", Channels: 1, SampleRate: 48000, Bitrate: 128000}
	tuple := avTuple("mpegts", "h264", "sdr", "aac", "mono", 1, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	tuple.Protocol = "hls"
	tuple.Video.Profile = "main"
	tuple.Video.BitDepth = 8
	tuple.Audio.Profile = "lc"
	tuple.Audio.Route = "decode"
	plan, err := Build(Request{Facts: facts, Capabilities: playbackcap.Resolution{EvidenceID: "web:chromium", Tuples: []playbackcap.DeliveryTuple{tuple}}, Policy: MaximumFidelity, Protocol: "hls", Selection: Selection{VideoIndex: ip(0), AudioIndex: ip(1)}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode == Unsupported || plan.Audio.Layout != "mono" || plan.Audio.Channels != 1 {
		t.Fatalf("mono MPEG2/MP2 plan = %#v", plan)
	}
}

func TestH264ConstrainedBaselineMatchesBrowserBaselineCapability(t *testing.T) {
	facts := baseFacts()
	facts.Video[0].Profile = "Constrained Baseline"
	tuple := avTuple("matroska", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	tuple.Video.Profile = "baseline"

	plan, err := Build(request(facts, tuple))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != DirectPlay {
		t.Fatalf("constrained baseline should direct play against baseline capability, got %#v", plan)
	}
}

func TestH264ConstrainedBaselineMatchesReviewedBrowserMainFallback(t *testing.T) {
	facts := baseFacts()
	facts.Container = "mp4"
	facts.Video[0].Profile = "Constrained Baseline"
	facts.Video[0].ChromaSubsampling = "4:2:0"
	facts.Audio[0].Profile = "lc"
	plan, err := Build(Request{
		Facts: facts,
		Capabilities: playbackcap.Resolution{
			EvidenceID: "static:chromium-120-159",
			Tuples: []playbackcap.DeliveryTuple{{
				Kind: playbackcap.MediaAudiovisual, Protocol: "http", Container: "mp4",
				Video:    playbackcap.Video{Codec: "h264", Profile: "main", PixelFormat: "yuv420p", Chroma: "4:2:0", HDR: "sdr", BitDepth: 8, MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 30},
				Audio:    playbackcap.Audio{Codec: "aac", Profile: "lc", Layout: "stereo", Route: "decode", MaxChannels: 2},
				Subtitle: playbackcap.Subtitle{Mode: playbackcap.SubtitleNone},
			}},
		},
		Policy: MaximumFidelity,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if plan.Mode != DirectPlay {
		t.Fatalf("expected direct play for a baseline source and main-capable client, got %s (%v)", plan.Mode, plan.Reasons)
	}
}

func TestPlannerConstraintsForceConversionForKnownAndUnknownBitrateAndHeight(t *testing.T) {
	tuple := avTuple("matroska", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	for _, tc := range []struct {
		name        string
		mutate      func(*mediafacts.Facts)
		constraints Constraints
		reason      ReasonCode
	}{
		{"known video bitrate", func(f *mediafacts.Facts) { f.Video[0].Bitrate = 9_000_000 }, Constraints{MaxVideoBitrate: 8_000_000}, ReasonVideoConstraint},
		{"unknown video bitrate", func(f *mediafacts.Facts) { f.Video[0].Bitrate = 0 }, Constraints{MaxVideoBitrate: 8_000_000}, ReasonVideoConstraint},
		{"known audio bitrate", func(f *mediafacts.Facts) { f.Audio[0].Bitrate = 256_000 }, Constraints{MaxAudioBitrate: 192_000}, ReasonAudioConstraint},
		{"unknown audio bitrate", func(f *mediafacts.Facts) { f.Audio[0].Bitrate = 0 }, Constraints{MaxAudioBitrate: 192_000}, ReasonAudioConstraint},
		{"width", func(*mediafacts.Facts) {}, Constraints{MaxWidth: 1280}, ReasonVideoConstraint},
		{"height", func(*mediafacts.Facts) {}, Constraints{MaxHeight: 720}, ReasonVideoConstraint},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := baseFacts()
			tc.mutate(&facts)
			req := request(facts, tuple)
			req.Constraints = tc.constraints
			plan, err := Build(req)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Mode != VideoTranscode && plan.Mode != DirectStream {
				t.Fatalf("constraint did not force conversion: %#v", plan)
			}
			if !has(plan.Reasons, tc.reason) {
				t.Fatalf("constraint reason missing: %#v", plan.Reasons)
			}
		})
	}
}

func TestHLSCompatibleStreamsStillRequirePackaging(t *testing.T) {
	facts := baseFacts()
	facts.Container = "mp4"
	safe := true
	facts.Video[0].ExactSeekSafe = &safe
	facts.Video[0].KeyframeEvidenceAt = time.Now().UTC().Format(time.RFC3339Nano)
	facts.Video[0].KeyframeEvidenceRevision = facts.Source.Revision
	tuple := avTuple("mp4", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	tuple.Protocol = "hls"
	r := request(facts, tuple)
	r.Protocol = "hls"
	plan, err := Build(r)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != Remux || plan.SegmentFormat != "fmp4" {
		t.Fatalf("progressive MP4 was treated as an existing HLS presentation: %#v", plan)
	}
}

func TestHLSCopyRequiresCurrentPositiveExactSeekEvidence(t *testing.T) {
	tuple := avTuple("mp4", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	tuple.Protocol = "hls"
	build := func(facts mediafacts.Facts) Plan {
		facts.Container = "mp4"
		r := request(facts, tuple)
		r.Protocol = "hls"
		plan, err := Build(r)
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}

	unknown := build(baseFacts())
	if unknown.Mode != VideoTranscode || !has(unknown.Reasons, ReasonExactSeekUnavailable) {
		t.Fatalf("unknown exact-seek evidence did not fail closed: %#v", unknown)
	}

	unsafeFacts := baseFacts()
	unsafe := false
	unsafeFacts.Video[0].ExactSeekSafe = &unsafe
	unsafeFacts.Video[0].KeyframeEvidenceAt = time.Now().UTC().Format(time.RFC3339Nano)
	unsafeFacts.Video[0].KeyframeEvidenceRevision = unsafeFacts.Source.Revision
	if plan := build(unsafeFacts); plan.Mode != VideoTranscode || !has(plan.Reasons, ReasonExactSeekUnavailable) {
		t.Fatalf("negative exact-seek evidence did not fail closed: %#v", plan)
	}

	staleFacts := baseFacts()
	safe := true
	staleFacts.Video[0].ExactSeekSafe = &safe
	staleFacts.Video[0].KeyframeEvidenceAt = time.Now().UTC().Format(time.RFC3339Nano)
	staleFacts.Video[0].KeyframeEvidenceRevision = "previous-source-revision"
	if plan := build(staleFacts); plan.Mode != VideoTranscode || !has(plan.Reasons, ReasonExactSeekUnavailable) {
		t.Fatalf("stale exact-seek evidence did not fail closed: %#v", plan)
	}
}

func TestPorticoSignalMKVChromiumRemuxRequiresExecutorCompatibleSeekEvidence(t *testing.T) {
	facts := baseFacts()
	facts.Source = mediafacts.Source{Fingerprint: "sha256-sampled:b36d7f8cd6691ac7f1d739c5fae5e3a107dc9f197984682151663f8279131b64", Revision: "834671a3e68d93d8bcce1561edd1c5b408edbb1984ecaeec4b47f1cdd7ef680d", SizeBytes: 2151186}
	facts.Container = "matroska"
	facts.DurationUS = 20_021_000
	facts.Video[0] = mediafacts.Video{Index: 0, Codec: "h264", Profile: "High", Level: "30", CodecTag: "[0][0][0][0]", CodedWidth: 640, CodedHeight: 360, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p", BitDepth: 8, ChromaSubsampling: "4:2:0", FieldOrder: "progressive", FrameRate: mediafacts.Rational{Num: 30, Den: 1}, Timing: mediafacts.Timing{StartTime: &mediafacts.Rational{Num: 21, Den: 1000}, TimeBase: &mediafacts.Rational{Num: 1, Den: 1000}}}
	facts.Audio[0] = mediafacts.Audio{Index: 1, Codec: "aac", Profile: "LC", Layout: "mono", Channels: 1, SampleRate: 48000, GaplessConfidence: mediafacts.ConfidenceUnknown, Timing: mediafacts.Timing{StartTime: &mediafacts.Rational{Num: 0, Den: 1}, TimeBase: &mediafacts.Rational{Num: 1, Den: 1000}}}
	httpTuple := avTuple("mp4", "h264", "sdr", "aac", "mono", 1, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	httpTuple.Video.BitDepth, httpTuple.Video.MaxWidth, httpTuple.Video.MaxHeight, httpTuple.Video.MaxFrameRate = 8, 2294, 1490, 60
	httpTuple.Video.Chroma = "4:2:0"
	httpTuple.Audio.Profile, httpTuple.Audio.Route = "lc", "decode"
	hlsTuple := httpTuple
	hlsTuple.Protocol, hlsTuple.Container = "hls", "mpegts"
	req := Request{Facts: facts, Capabilities: playbackcap.Resolution{EvidenceID: "authenticated_runtime:chromium-152", Tuples: []playbackcap.DeliveryTuple{httpTuple, hlsTuple}}, Policy: MaximumFidelity, Selection: Selection{VideoIndex: ip(0), AudioIndex: ip(1)}}

	progressive, err := Build(req)
	if err != nil {
		t.Fatal(err)
	}
	if progressive.Mode != Remux || progressive.Protocol != "http" || progressive.Streams[0].Action != Copy || progressive.Streams[1].Action != Copy {
		t.Fatalf("live Chromium facts did not select the supported progressive remux tuple: %#v", progressive)
	}

	// The current executor publishes every non-direct VOD graph as HLS. HLS
	// packet-copy remains fail-closed until source-revision-bound keyframe-grid
	// evidence proves segment-safe seeks; unknown AAC gapless facts alone must
	// neither force nor authorize video conversion.
	req.Protocol = "hls"
	hls, err := Build(req)
	if err != nil {
		t.Fatal(err)
	}
	if hls.Mode != VideoTranscode || !has(hls.Reasons, ReasonExactSeekUnavailable) || !has(hls.Reasons, ReasonGaplessFactsUnknown) {
		t.Fatalf("executor-constrained HLS plan did not preserve seek/gapless safety: %#v", hls)
	}
}

func TestAudioOnlyFirstClassAndGapless(t *testing.T) {
	f := baseFacts()
	f.Video = nil
	f.Subtitles = nil
	f.Audio[0].Codec = "flac"
	f.Audio[0].Layout = "5.1"
	f.Audio[0].Channels = 6
	f.Audio[0].EncoderDelaySamples = 1024
	r := Request{Facts: f, Capabilities: playbackcap.Resolution{EvidenceID: "audio", Tuples: []playbackcap.DeliveryTuple{{Kind: playbackcap.MediaAudio, Protocol: "http", Container: "matroska", Audio: playbackcap.Audio{Codec: "flac", Layout: "5.1", MaxChannels: 6}, Subtitle: playbackcap.Subtitle{Mode: playbackcap.SubtitleNone}}}}, Selection: Selection{AudioIndex: ip(1)}, Policy: MaximumFidelity}
	p, e := Build(r)
	if e != nil {
		t.Fatal(e)
	}
	if p.MediaKind != playbackcap.MediaAudio || p.Color != nil || p.Mode != DirectPlay {
		t.Fatalf("bad audio plan: %#v", p)
	}
	for _, s := range p.Stages {
		if s.Kind == "video" {
			t.Fatal("invented video stage")
		}
	}
}

func TestSilentVideoUsesExactNoAudioTuple(t *testing.T) {
	facts := baseFacts()
	facts.Audio = nil
	facts.Subtitles = nil
	tuple := avTuple("matroska", "h264", "sdr", "", "", 0, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	tuple.Audio = playbackcap.Audio{}
	plan, err := Build(Request{
		Facts:        facts,
		Capabilities: playbackcap.Resolution{EvidenceID: "silent-video", Tuples: []playbackcap.DeliveryTuple{tuple}},
		Policy:       MaximumFidelity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != DirectPlay || plan.MediaKind != playbackcap.MediaAudiovisual {
		t.Fatalf("silent video was not directly playable: %#v", plan)
	}
	if plan.Audio != (AudioDecision{}) {
		t.Fatalf("silent video invented audio output: %#v", plan.Audio)
	}
	for _, stream := range plan.Streams {
		if stream.Kind == "audio" {
			t.Fatalf("silent video invented an audio action: %#v", plan.Streams)
		}
	}

	withAudio := tuple
	withAudio.Audio = playbackcap.Audio{Codec: "aac", Layout: "stereo", MaxChannels: 2}
	plan, err = Build(Request{
		Facts:        facts,
		Capabilities: playbackcap.Resolution{EvidenceID: "audio-bearing-only", Tuples: []playbackcap.DeliveryTuple{withAudio}},
		Policy:       MaximumFidelity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != Unsupported {
		t.Fatalf("audio-bearing tuple was reinterpreted as silent capability: %#v", plan)
	}
}

func TestSubtitleSemantics(t *testing.T) {
	r := request(baseFacts(), avTuple("matroska", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Codec: "subrip", Kind: "text", Mode: playbackcap.SubtitleNative}))
	r.Selection.SubtitleIndex = ip(2)
	p, e := Build(r)
	if e != nil {
		t.Fatal(e)
	}
	if p.Subtitle.Action != ExternalText || !p.Subtitle.Forced || p.Mode != DirectPlay {
		t.Fatalf("bad text subtitle: %#v", p.Subtitle)
	}
	f := baseFacts()
	f.Subtitles[0].Codec = "hdmv_pgs_subtitle"
	f.Subtitles[0].Kind = "bitmap"
	r = request(f, avTuple("matroska", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Codec: "hdmv_pgs_subtitle", Kind: "bitmap", Mode: playbackcap.SubtitleBurn}))
	r.Selection.SubtitleIndex = ip(2)
	p, _ = Build(r)
	if p.Mode != VideoTranscode || p.Subtitle.Action != BurnIn {
		t.Fatalf("bitmap not burned: %#v", p)
	}
}

func TestHDRAndDolbyVisionTruthTable(t *testing.T) {
	tests := []struct {
		name, in, out string
		dv            *mediafacts.DolbyVision
		want          Mode
		action        string
		reason        ReasonCode
	}{{"pq preserve", "pq", "pq", nil, DirectPlay, "preserve", ReasonHDRPreserved}, {"hlg tone map", "hlg", "sdr", nil, VideoTranscode, "tone_map_sdr", ReasonHDRToneMapped}, {"hdr10 plus downgrade", "hdr10plus", "pq", nil, VideoTranscode, "downgrade_hdr10plus", ReasonHDR10PlusDowngraded}, {"dv8 base", "", "pq", &mediafacts.DolbyVision{Profile: 8, BaseLayerPresent: true, BaseLayerPresentKnown: true, BaseLayerCodec: "hevc", Fallback: "hdr10", Evidence: "probe"}, VideoTranscode, "use_verified_base", ReasonDVVerifiedBase}, {"dv5 rejected", "", "sdr", &mediafacts.DolbyVision{Profile: 5, Evidence: "probe"}, Unsupported, "", ReasonDVUnsupported}, {"dv8 unverified base rejected", "", "pq", &mediafacts.DolbyVision{Profile: 8, BaseLayerCodec: "hevc", Evidence: "probe"}, Unsupported, "", ReasonDVUnsupported}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := baseFacts()
			if tt.in == "pq" {
				f.Video[0].ColorTransfer = "smpte2084"
			}
			if tt.in == "hlg" {
				f.Video[0].ColorTransfer = "arib-std-b67"
			}
			if tt.in == "hdr10plus" {
				f.Video[0].HDR10Plus = true
				f.Video[0].HDR10PlusKnown = true
			}
			f.Video[0].DolbyVision = tt.dv
			r := request(f, avTuple("matroska", map[bool]string{true: "hevc", false: "h264"}[tt.dv != nil], tt.out, "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone}))
			p, e := Build(r)
			if e != nil {
				t.Fatal(e)
			}
			if p.Mode != tt.want {
				t.Fatalf("mode %s", p.Mode)
			}
			if tt.action != "" && (p.Color == nil || p.Color.Action != tt.action) {
				t.Fatalf("color %#v", p.Color)
			}
			if !has(p.Reasons, tt.reason) {
				t.Fatalf("reasons %v", p.Reasons)
			}
		})
	}
}

func TestNoDolbyVisionInvention(t *testing.T) {
	p, _ := Build(request(baseFacts(), avTuple("matroska", "h264", "dolby_vision", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})))
	if p.Mode != Unsupported || !has(p.Reasons, ReasonDVUnsupported) {
		t.Fatalf("invented DV: %#v", p)
	}
}

func TestOwnerToneMappingPolicyIsSealedAndCanFailClosed(t *testing.T) {
	f := baseFacts()
	f.Video[0].ColorTransfer = "arib-std-b67"
	tuple := avTuple("matroska", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	r := request(f, tuple)
	r.ToneMapAlgorithm = "hable"
	p, err := Build(r)
	if err != nil {
		t.Fatal(err)
	}
	if p.Color == nil || p.Color.Action != "tone_map_sdr" || p.Color.ToneMapAlgorithm != "hable" {
		t.Fatalf("owner tone-map algorithm was not sealed: %#v", p.Color)
	}
	r.DisableToneMapping = true
	p, err = Build(r)
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != Unsupported || !has(p.Reasons, ReasonHDRToneMapDisabled) {
		t.Fatalf("disabled tone mapping did not fail closed: %#v", p)
	}
}

func TestAudioLadderAndPolicyAreDeterministic(t *testing.T) {
	f := baseFacts()
	f.Audio[0].Codec = "truehd"
	f.Audio[0].Layout = "7.1"
	f.Audio[0].Channels = 8
	f.Audio[0].ObjectAudio = "atmos"
	f.Audio[0].ObjectAudioEvidence = "ffprobe-side-data"
	st := playbackcap.Subtitle{Mode: playbackcap.SubtitleNone}
	r := request(f, avTuple("matroska", "h264", "sdr", "aac", "stereo", 2, st), avTuple("matroska", "h264", "sdr", "eac3", "5.1", 6, st))
	p, _ := Build(r)
	if p.Audio.Codec != "eac3" || !p.Audio.Downmixed || p.Audio.ObjectsPreserved {
		t.Fatalf("wrong ladder %#v", p.Audio)
	}
	if !has(p.Reasons, ReasonObjectAudioLost) {
		t.Fatal(p.Reasons)
	}
}

func TestUnverifiedHardwareIsNeverClaimed(t *testing.T) {
	r := request(baseFacts(), avTuple("matroska", "hevc", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone}))
	r.Hardware = HardwareRoute{Backend: playbackhw.NVIDIA, Verified: false, Stages: []Stage{{Kind: "video", Operation: "encode", Execution: "hardware"}}}
	p, _ := Build(r)
	if p.Mode != Unsupported || p.Hardware.Backend != "" {
		t.Fatalf("unverified hardware exposed: %#v", p)
	}
}

func TestDigestCanonicalDeterminismAndImmutability(t *testing.T) {
	a := avTuple("matroska", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	b := avTuple("mp4", "hevc", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone})
	r1 := request(baseFacts(), a, b)
	r2 := request(baseFacts(), b, a)
	p1, _ := Build(r1)
	p2, _ := Build(r2)
	if p1.Digest != p2.Digest {
		t.Fatalf("order changed digest\n%s\n%s", p1.Digest, p2.Digest)
	}
	const golden = "playback-plan-v2:sha256:"
	if !strings.HasPrefix(p1.Digest, golden) || len(p1.Digest) != len(golden)+64 {
		t.Fatal(p1.Digest)
	}
	const exactGolden = "playback-plan-v2:sha256:f9ecd6a0a1f741425785c2ae9bf116d7829b91e3d80a143d19001cb49a702c62"
	if p1.Digest != exactGolden {
		t.Fatalf("digest golden changed: %s", p1.Digest)
	}
	q := p1.Clone()
	q.Reasons[0] = "mutated"
	if p1.Reasons[0] == q.Reasons[0] {
		t.Fatal("clone aliases")
	}
	raw, _ := json.Marshal(p1.PublicSummary())
	if strings.Contains(string(raw), "/dev/") || strings.Contains(string(raw), "DevicePath") {
		t.Fatal(string(raw))
	}
}

func TestPublicSummarySanitizesHardwareStageData(t *testing.T) {
	r := request(baseFacts(), avTuple("matroska", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone}))
	r.Hardware = HardwareRoute{Verified: true, Backend: playbackhw.NVIDIA, Stages: []Stage{{Kind: "/secret/media/file", Operation: "/dev/dri/renderD128", Execution: "hunter2"}, {Kind: "anything", Operation: "encode", Execution: "hardware"}}}
	p, err := Build(r)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p.PublicSummary())
	for _, secret := range []string{"/secret", "/dev/dri", "hunter2"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("public summary leaked %q: %s", secret, raw)
		}
	}
	if len(p.Hardware.Stages) != 1 || p.Hardware.Stages[0].Kind != "hardware" {
		t.Fatalf("hardware stages were not normalized: %#v", p.Hardware.Stages)
	}
}

func TestInvalidSelectionAndUnknownDuration(t *testing.T) {
	r := request(baseFacts(), avTuple("matroska", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone}))
	r.Selection.AudioIndex = ip(99)
	p, e := Build(r)
	if e == nil || p.Mode != Unsupported {
		t.Fatal("invalid selection accepted")
	}
	r = request(baseFacts(), avTuple("matroska", "h264", "sdr", "aac", "stereo", 2, playbackcap.Subtitle{Mode: playbackcap.SubtitleNone}))
	r.Facts.DurationUS = 0
	r.Facts.DurationConfidence = mediafacts.ConfidenceUnknown
	p, e = Build(r)
	if e != nil || !p.Timeline.Dynamic || p.Timeline.Mode != "event" {
		t.Fatalf("bad dynamic timeline %#v %v", p.Timeline, e)
	}
}
func has(rs []ReasonCode, w ReasonCode) bool {
	for _, r := range rs {
		if r == w {
			return true
		}
	}
	return false
}
