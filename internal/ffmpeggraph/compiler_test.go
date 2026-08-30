package ffmpeggraph

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func intptr(v int) *int { return &v }

func exactFacts() mediafacts.Facts {
	return mediafacts.Facts{
		Version: mediafacts.SchemaVersion, Source: mediafacts.Source{Fingerprint: "file-sha", Revision: "stat-v2"}, Container: "matroska",
		DurationUS: 10_000_000, DurationConfidence: mediafacts.ConfidenceExact, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown,
		Video:     []mediafacts.Video{{Index: 2, Codec: "hevc", CodedWidth: 3840, CodedHeight: 2160, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p10le", BitDepth: 10, ColorPrimaries: "bt2020", ColorTransfer: "smpte2084", ColorMatrix: "bt2020nc", FieldOrder: "tt", FrameRate: mediafacts.Rational{Num: 24000, Den: 1001}, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}},
		Audio:     []mediafacts.Audio{{Index: 5, Codec: "truehd", Layout: "7.1", Channels: 8, SampleRate: 96000, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}},
		Subtitles: []mediafacts.Subtitle{{Index: 7, Codec: "subrip", Kind: "text", ClosedCaptionKnown: true, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}},
	}
}

func planFor(f mediafacts.Facts) playbackplan.Plan {
	p := playbackplan.Plan{
		SchemaRevision: playbackplan.SchemaRevision, SourceFingerprint: f.Source.Fingerprint, SourceRevision: f.Source.Revision,
		CapabilityEvidenceID: "native-runtime:v1", Policy: playbackplan.MaximumFidelity, Mode: playbackplan.VideoTranscode,
		Protocol: "hls", Container: "mp4", SegmentFormat: "fmp4", Selection: playbackplan.Selection{VideoIndex: intptr(2), AudioIndex: intptr(5), SubtitleIndex: intptr(7)},
		Streams: []playbackplan.StreamAction{{Index: 2, Kind: "video", Action: playbackplan.Convert, InputCodec: "hevc", OutputCodec: "h264"}, {Index: 5, Kind: "audio", Action: playbackplan.Convert, InputCodec: "truehd", OutputCodec: "aac", InputLayout: "7.1", OutputLayout: "stereo"}, {Index: 7, Kind: "subtitle", Action: playbackplan.BurnIn, InputCodec: "subrip", OutputCodec: "subrip"}},
		Stages:  []playbackplan.Stage{{Kind: "video", Operation: "decode", Execution: "software"}, {Kind: "video", Operation: "tone_map_sdr", Execution: "software"}, {Kind: "subtitle", Operation: "burn_in", Execution: "software"}, {Kind: "video", Operation: "encode", Execution: "software"}, {Kind: "audio", Operation: "encode", Execution: "software"}, {Kind: "mux", Operation: "package", Execution: "stream"}},
		Color:   &playbackplan.ColorDecision{Input: "pq", Output: "sdr", Action: "tone_map_sdr"}, Audio: playbackplan.AudioDecision{Codec: "aac", Layout: "stereo", Channels: 2, Downmixed: true}, Subtitle: playbackplan.SubtitleDecision{Index: intptr(7), Codec: "subrip", Kind: "text", Action: playbackplan.BurnIn}, Constraints: playbackplan.Constraints{MaxWidth: 1920, MaxHeight: 1080, MaxVideoBitrate: 8_000_000},
	}
	p.Digest, _ = p.ComputeDigest()
	return p
}

func TestCompileDeterministicSoftwareHDRDownmixBurnAndCMAF(t *testing.T) {
	f := exactFacts()
	p := planFor(f)
	req := Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/media/movie.mkv", SubtitlePath: "/media/sub title.srt", Output: Output{ManifestPath: "/work/index.m3u8", SegmentPattern: "/work/seg_%05d.m4s", InitFilename: "boot.mp4", SegmentSeconds: 6, StartNumber: 3}}
	a, err := Compile(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compile(req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("compile is not deterministic")
	}
	joined := strings.Join(a.Args, " ")
	for _, want := range []string{"-map 0:2", "-map 0:5", "bwdif=", "zscale=matrixin=bt2020nc", "tonemap=tonemap=mobius", "scale=w='min(iw\\,1920)'", "subtitles=filename='/media/sub title.srt'", "-c:v libx264", "-ac 2 -channel_layout stereo -ar 48000 -b:a 192000", "-hls_segment_type fmp4", "-hls_fmp4_init_filename boot.mp4"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args lack %q: %s", want, joined)
		}
	}
}

func TestCompileUnknownDurationUsesAppendOnlyEventHLS(t *testing.T) {
	f := exactFacts()
	p := planFor(f)
	r, err := Compile(Request{
		X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/media/unknown.mkv", SubtitlePath: "/media/sub.srt",
		Output: Output{ManifestPath: "/work/index.m3u8", SegmentPattern: "/work/segment_%05d.m4s", SegmentSeconds: 6, Event: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.Args, " ")
	for _, want := range []string{"-hls_list_size 0", "-hls_playlist_type event"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("event HLS args lack %q: %s", want, joined)
		}
	}
	for _, forbidden := range []string{"delete_segments", "omit_endlist", "program_date_time", "-hls_playlist_type vod"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("event HLS args contain live/VOD behavior %q: %s", forbidden, joined)
		}
	}
}

func TestCompileUsesSealedToneMapAlgorithm(t *testing.T) {
	f := exactFacts()
	p := planFor(f)
	p.Color.ToneMapAlgorithm = "hable"
	p.Digest, _ = p.ComputeDigest()
	r, err := Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/media/hdr.mkv", SubtitlePath: "/media/sub.srt", Output: Output{ManifestPath: "/work/index.m3u8", SegmentPattern: "/work/seg_%05d.m4s", SegmentSeconds: 6}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.VideoFilter, "tonemap=tonemap=hable") || strings.Contains(r.VideoFilter, "tonemap=tonemap=mobius") {
		t.Fatalf("compiled graph ignored sealed algorithm: %s", r.VideoFilter)
	}
}

func TestCompileConvertsMain10SourceToCanonicalEightBitH264(t *testing.T) {
	f := exactFacts()
	f.Video[0].FieldOrder = "progressive"
	p := planFor(f)
	p.Color = &playbackplan.ColorDecision{Input: "sdr", Output: "sdr", Action: "preserve"}
	p.Subtitle = playbackplan.SubtitleDecision{Action: playbackplan.Drop}
	p.Selection.SubtitleIndex = nil
	p.Streams = p.Streams[:2]
	p.Stages = []playbackplan.Stage{{Kind: "video", Operation: "decode", Execution: "software"}, {Kind: "video", Operation: "encode", Execution: "software"}, {Kind: "audio", Operation: "encode", Execution: "software"}, {Kind: "mux", Operation: "package", Execution: "stream"}}
	p.Constraints = playbackplan.Constraints{}
	p.Digest, _ = p.ComputeDigest()

	result, err := Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/media/main10.mkv", Output: Output{ManifestPath: "/work/index.m3u8", SegmentPattern: "/work/segment_%05d.ts", SegmentSeconds: 4}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.VideoFilter, "format=yuv420p") || strings.Contains(result.VideoFilter, "format=yuv420p10le") {
		t.Fatalf("H.264 graph did not seal 8-bit 4:2:0 output: %q", result.VideoFilter)
	}
}

func TestCompileUsesAndValidatesSealedX264Preset(t *testing.T) {
	f := exactFacts()
	p := planFor(f)
	req := Request{X264Preset: "slower", Plan: p, Facts: f, SourcePath: "/media/movie.mkv", SubtitlePath: "/media/sub.srt", Output: Output{ManifestPath: "/work/index.m3u8", SegmentPattern: "/work/seg_%05d.m4s", SegmentSeconds: 6}}
	r, err := Compile(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(r.Args, " "), "-c:v libx264 -preset slower") {
		t.Fatalf("libx264 did not use sealed preset: %v", r.Args)
	}
	req.X264Preset = "placebo"
	if _, err := Compile(req); err == nil {
		t.Fatal("invalid sealed x264 preset was accepted")
	}
}

func TestCompilePreservesAnamorphicDisplayGeometryBeforeRotation(t *testing.T) {
	f := exactFacts()
	f.Video[0].FieldOrder = "progressive"
	f.Video[0].SampleAspectRatio = mediafacts.Rational{Num: 16, Den: 15}
	f.Video[0].DisplayAspectRatio = mediafacts.Rational{Num: 64, Den: 45}
	f.Video[0].Rotation = 90
	p := planFor(f)
	p.Color = &playbackplan.ColorDecision{Input: "sdr", Output: "sdr", Action: "preserve"}
	p.Subtitle = playbackplan.SubtitleDecision{Action: playbackplan.Drop}
	p.Selection.SubtitleIndex = nil
	p.Streams = p.Streams[:2]
	p.Stages = []playbackplan.Stage{{Kind: "video", Operation: "decode", Execution: "software"}, {Kind: "video", Operation: "encode", Execution: "software"}, {Kind: "audio", Operation: "encode", Execution: "software"}, {Kind: "mux", Operation: "package", Execution: "stream"}}
	p.Constraints = playbackplan.Constraints{}
	p.Digest, _ = p.ComputeDigest()

	r, err := Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/media/anamorphic.mkv", Output: Output{ManifestPath: "/work/index.m3u8", SegmentPattern: "/work/seg_%05d.m4s", SegmentSeconds: 6}})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.Args, " ")
	normalize := "scale=w='max(2\\,trunc(iw*16/15/2)*2)':h=ih:flags=lanczos,setsar=1,transpose=clock"
	if !strings.Contains(joined, "-noautorotate -i /media/anamorphic.mkv") {
		t.Fatalf("explicit rotation did not disable FFmpeg autorotation: %s", joined)
	}
	if !strings.Contains(r.VideoFilter, normalize) {
		t.Fatalf("anamorphic geometry was not normalized before rotation: %s", r.VideoFilter)
	}
}

func TestCompileRejectsEventLiveHLS(t *testing.T) {
	f := exactFacts()
	p := planFor(f)
	_, err := Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/media/a", SubtitlePath: "/media/s", Output: Output{ManifestPath: "/work/index.m3u8", SegmentPattern: "/work/segment_%05d.m4s", SegmentSeconds: 6, Event: true, Live: true}})
	var ce *CompileError
	if !errors.As(err, &ce) || ce.Code != InvalidOutput {
		t.Fatalf("expected invalid event/live output, got %v", err)
	}
}

func TestCompileUsesOnlyVerifiedDolbyVisionFallback(t *testing.T) {
	f := exactFacts()
	f.Video[0].DolbyVision = &mediafacts.DolbyVision{Profile: 8, RPU: true, RPUKnown: true, BaseLayerPresent: true, BaseLayerPresentKnown: true, BaseLayerCodec: "hevc", Fallback: "hdr10", Evidence: "ffprobe configuration record"}
	p := planFor(f)
	p.Color = &playbackplan.ColorDecision{Input: "dolby_vision", Output: "sdr", Action: "use_verified_base", DolbyVisionProfile: 8}
	p.Stages[1].Operation = "use_verified_base"
	p.Digest, _ = p.ComputeDigest()
	r, err := Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/media/dv8.mkv", SubtitlePath: "/media/sub.srt", Output: Output{ManifestPath: "/work/index.m3u8", SegmentPattern: "/work/seg_%05d.m4s", SegmentSeconds: 6}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.VideoFilter, "transferin=smpte2084") || !strings.Contains(r.VideoFilter, "tonemap=tonemap=mobius") {
		t.Fatalf("verified HDR10 base was not tone mapped: %s", r.VideoFilter)
	}
	f.Video[0].DolbyVision.BaseLayerPresent = false
	p.SourceRevision = f.Source.Revision
	p.Digest, _ = p.ComputeDigest()
	_, err = Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/media/dv8.mkv", SubtitlePath: "/media/sub.srt", Output: Output{ManifestPath: "/work/index.m3u8", SegmentPattern: "/work/seg_%05d.m4s", SegmentSeconds: 6}})
	if err == nil || !strings.Contains(err.Error(), "lacks exact fallback evidence") {
		t.Fatalf("unverified Dolby Vision base accepted: %v", err)
	}
}

func TestDolbyVisionMP4CopyUsesDVH1SampleEntry(t *testing.T) {
	plan := playbackplan.Plan{Container: "mp4", Color: &playbackplan.ColorDecision{Input: "dolby_vision", Output: "dolby_vision", Action: "preserve"}}
	if got := strings.Join(containerVideoArgs(plan, "hevc", true), " "); got != "-tag:v dvh1" {
		t.Fatalf("Dolby Vision MP4 tag args = %q", got)
	}
	plan.Color.Output = "pq"
	if got := strings.Join(containerVideoArgs(plan, "hevc", true), " "); got != "-tag:v hvc1" {
		t.Fatalf("HDR10 HEVC MP4 tag args = %q", got)
	}
}

func TestCompileBitmapSubtitleUsesExactOverlayGraph(t *testing.T) {
	f := exactFacts()
	f.Video[0].FieldOrder = "progressive"
	f.Video[0].ColorTransfer = "bt709"
	f.Subtitles[0].Codec = "hdmv_pgs_subtitle"
	f.Subtitles[0].Kind = "bitmap"
	p := planFor(f)
	p.Color = &playbackplan.ColorDecision{Input: "sdr", Output: "sdr", Action: "preserve"}
	p.Streams[2].InputCodec = "hdmv_pgs_subtitle"
	p.Streams[2].OutputCodec = "hdmv_pgs_subtitle"
	p.Subtitle.Codec = "hdmv_pgs_subtitle"
	p.Subtitle.Kind = "bitmap"
	p.Digest, _ = p.ComputeDigest()
	ordinal := 0
	r, err := Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/media/pgs.mkv", SubtitlePath: "/media/pgs.mkv", SubtitleStreamOrdinal: &ordinal, Output: Output{ManifestPath: "/work/index.m3u8", SegmentPattern: "/work/seg_%05d.m4s", SegmentSeconds: 6}})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.Args, " ")
	for _, want := range []string{"-filter_complex", "[0:7]setpts=PTS-STARTPTS[portico_sub]", "[0:2][portico_sub]overlay=eof_action=pass:shortest=0[portico_composited]", "[portico_composited]scale=", "[portico_video]", "-map [portico_video]", "-map 0:5"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("bitmap overlay args lack %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "subtitles=") || strings.Contains(joined, "-map 0:2") {
		t.Fatalf("bitmap stream used text filter or unfiltered video map: %s", joined)
	}
}

func TestCompileCopyRemuxProgressive(t *testing.T) {
	f := exactFacts()
	f.Video[0].ColorTransfer = "bt709"
	f.Video[0].FieldOrder = "progressive"
	p := planFor(f)
	p.Mode = playbackplan.Remux
	p.Protocol = "progressive"
	p.Container = "mp4"
	p.SegmentFormat = "progressive"
	p.Selection.SubtitleIndex = nil
	p.Streams = []playbackplan.StreamAction{{Index: 2, Kind: "video", Action: playbackplan.Copy, InputCodec: "hevc", OutputCodec: "hevc"}, {Index: 5, Kind: "audio", Action: playbackplan.Copy, InputCodec: "truehd", OutputCodec: "truehd"}}
	p.Stages = []playbackplan.Stage{{Kind: "video", Operation: "copy", Execution: "stream"}, {Kind: "audio", Operation: "copy", Execution: "stream"}, {Kind: "mux", Operation: "package", Execution: "stream"}}
	p.Color = &playbackplan.ColorDecision{Input: "sdr", Output: "sdr", Action: "preserve"}
	p.Audio = playbackplan.AudioDecision{Codec: "truehd", Layout: "7.1", Channels: 8, Passthrough: true}
	p.Subtitle = playbackplan.SubtitleDecision{Action: playbackplan.Drop}
	p.Constraints = playbackplan.Constraints{}
	p.Digest, _ = p.ComputeDigest()
	r, err := Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/a.mkv", Output: Output{ProgressivePath: "/o.mp4"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.Args, " "); !strings.Contains(got, "-map 0:2 -c:v copy -tag:v hvc1") || !strings.Contains(got, "-map 0:5 -c:a copy -f mp4 /o.mp4") {
		t.Fatalf("unexpected args: %s", got)
	}
}

func TestCompileRejectsIncompleteAndMismatchedGraphs(t *testing.T) {
	f := exactFacts()
	p := planFor(f)
	p.Stages = nil
	p.Digest, _ = p.ComputeDigest()
	_, err := Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/a", SubtitlePath: "/s", Output: Output{ManifestPath: "/m", SegmentPattern: "/s", SegmentSeconds: 6}})
	var ce *CompileError
	if !errors.As(err, &ce) || ce.Code != InvalidPlan {
		t.Fatalf("expected invalid plan, got %v", err)
	}
	p = planFor(f)
	f.Source.Revision = "changed"
	_, err = Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/a"})
	if !errors.As(err, &ce) || ce.Code != FactsMismatch {
		t.Fatalf("expected facts mismatch, got %v", err)
	}
}

func TestCompileRequiresExactVerifiedHardwarePlan(t *testing.T) {
	f := exactFacts()
	f.Video[0].Rotation = 0
	f.Video[0].SampleAspectRatio = mediafacts.Rational{Num: 1, Den: 1}
	p := planFor(f)
	p.Subtitle = playbackplan.SubtitleDecision{Action: playbackplan.Drop}
	p.Selection.SubtitleIndex = nil
	p.Streams = p.Streams[:2]
	p.Stages = []playbackplan.Stage{{Kind: "video", Operation: "decode", Execution: "hardware"}, {Kind: "video", Operation: "tone_map_sdr", Execution: "hardware"}, {Kind: "video", Operation: "encode", Execution: "hardware"}, {Kind: "audio", Operation: "encode", Execution: "software"}, {Kind: "mux", Operation: "package", Execution: "stream"}}
	p.Hardware = playbackplan.HardwareRoute{Verified: true, Backend: playbackhw.VAAPI, Stages: []playbackplan.Stage{{Kind: "hardware", Operation: "decode", Execution: "hardware"}, {Kind: "hardware", Operation: "tone_map", Execution: "hardware"}, {Kind: "hardware", Operation: "encode", Execution: "hardware"}}}
	p.Digest, _ = p.ComputeDigest()
	hw := &playbackhw.Plan{Backend: playbackhw.VAAPI, Stages: []playbackhw.Stage{{Operation: playbackhw.Decode, Execution: playbackhw.Hardware}, {Operation: playbackhw.ToneMap, Execution: playbackhw.Hardware}, {Operation: playbackhw.Encode, Execution: playbackhw.Hardware}}, InputArgs: []string{"-hwaccel", "vaapi"}, Filter: "tonemap_vaapi=format=nv12", OutputArgs: []string{"-c:v", "h264_vaapi"}}
	r, err := Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/a", Hardware: hw, Output: Output{ManifestPath: "/m", SegmentPattern: "/s", SegmentSeconds: 6}})
	if err != nil {
		t.Fatal(err)
	}
	if !r.UsesHardware || !strings.Contains(strings.Join(r.Args, " "), "-hwaccel vaapi -i /a") {
		t.Fatalf("hardware plan not incorporated: %#v", r)
	}
	hw.Backend = playbackhw.QSV
	_, err = Compile(Request{X264Preset: "medium", Plan: p, Facts: f, SourcePath: "/a", Hardware: hw, Output: Output{ManifestPath: "/m", SegmentPattern: "/s", SegmentSeconds: 6}})
	var ce *CompileError
	if !errors.As(err, &ce) || ce.Code != HardwareMismatch {
		t.Fatalf("expected hardware mismatch, got %v", err)
	}
}
