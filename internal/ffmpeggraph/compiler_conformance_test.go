package ffmpeggraph

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

// These tests intentionally create their tiny sources at runtime. That keeps
// the repository free of opaque binary fixtures while proving that a compiled
// graph is accepted by a real FFmpeg and that its output has the promised
// stream properties according to a real ffprobe.

type conformanceStream struct {
	Index              int    `json:"index"`
	CodecType          string `json:"codec_type"`
	CodecName          string `json:"codec_name"`
	Width              int    `json:"width"`
	Height             int    `json:"height"`
	SampleAspectRatio  string `json:"sample_aspect_ratio"`
	DisplayAspectRatio string `json:"display_aspect_ratio"`
	FieldOrder         string `json:"field_order"`
	ColorSpace         string `json:"color_space"`
	ColorTransfer      string `json:"color_transfer"`
	ColorPrimaries     string `json:"color_primaries"`
	Channels           int    `json:"channels"`
	ChannelLayout      string `json:"channel_layout"`
}

type conformanceProbe struct {
	Streams []conformanceStream `json:"streams"`
}

func conformanceTools(t *testing.T) (string, string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg conformance skipped: ffmpeg is not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("FFmpeg conformance skipped: ffprobe is not installed")
	}
	return ffmpeg, ffprobe
}

func conformanceRun(t *testing.T, binary string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", filepath.Base(binary), err, output)
	}
	return string(output)
}

func conformanceProbeFile(t *testing.T, ffprobe, path string) conformanceProbe {
	t.Helper()
	output := conformanceRun(t, ffprobe, "-v", "error", "-show_streams", "-of", "json", path)
	var result conformanceProbe
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode ffprobe output: %v\n%s", err, output)
	}
	return result
}

func conformanceStreamOf(t *testing.T, probe conformanceProbe, kind string) conformanceStream {
	t.Helper()
	for _, stream := range probe.Streams {
		if stream.CodecType == kind {
			return stream
		}
	}
	t.Fatalf("ffprobe output has no %s stream: %#v", kind, probe.Streams)
	return conformanceStream{}
}

func conformanceFixture(t *testing.T, ffmpeg, output string, inputArgs, outputArgs []string) {
	t.Helper()
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	args = append(args, inputArgs...)
	args = append(args, outputArgs...)
	args = append(args, output)
	conformanceRun(t, ffmpeg, args...)
}

func conformanceFacts(video *mediafacts.Video, audio mediafacts.Audio) mediafacts.Facts {
	facts := mediafacts.Facts{
		Version:   mediafacts.SchemaVersion,
		Source:    mediafacts.Source{Fingerprint: "conformance-source", Revision: "conformance-revision"},
		Container: "matroska", DurationUS: 800_000, DurationConfidence: mediafacts.ConfidenceExact,
		VariableFrameRateConfidence: mediafacts.ConfidenceUnknown,
		Audio:                       []mediafacts.Audio{audio},
	}
	if video != nil {
		facts.Video = []mediafacts.Video{*video}
	}
	return facts
}

func conformancePlan(facts mediafacts.Facts, video bool) playbackplan.Plan {
	audio := facts.Audio[0]
	p := playbackplan.Plan{
		SchemaRevision: playbackplan.SchemaRevision, SourceFingerprint: facts.Source.Fingerprint, SourceRevision: facts.Source.Revision,
		CapabilityEvidenceID: "conformance:native-runtime", Policy: playbackplan.MaximumFidelity,
		Mode: playbackplan.VideoTranscode, MediaKind: playbackcap.MediaAudiovisual, Protocol: "progressive", Container: "mp4", SegmentFormat: "progressive",
		Selection: playbackplan.Selection{AudioIndex: intptr(audio.Index)},
		Streams:   []playbackplan.StreamAction{{Index: audio.Index, Kind: "audio", Action: playbackplan.Convert, InputCodec: audio.Codec, OutputCodec: "aac", InputLayout: audio.Layout, OutputLayout: "stereo"}},
		Stages:    []playbackplan.Stage{{Kind: "audio", Operation: "encode", Execution: "software"}, {Kind: "mux", Operation: "package", Execution: "stream"}},
		Audio:     playbackplan.AudioDecision{Codec: "aac", Layout: "stereo", Channels: 2, Downmixed: audio.Channels > 2},
		Subtitle:  playbackplan.SubtitleDecision{Action: playbackplan.Drop},
	}
	if video {
		v := facts.Video[0]
		p.Selection.VideoIndex = intptr(v.Index)
		p.Streams = append([]playbackplan.StreamAction{{Index: v.Index, Kind: "video", Action: playbackplan.Convert, InputCodec: v.Codec, OutputCodec: "h264"}}, p.Streams...)
		p.Stages = append([]playbackplan.Stage{{Kind: "video", Operation: "decode", Execution: "software"}, {Kind: "video", Operation: "encode", Execution: "software"}}, p.Stages...)
		p.Color = &playbackplan.ColorDecision{Input: "sdr", Output: "sdr", Action: "preserve"}
	} else {
		p.Mode = playbackplan.DirectStream
		p.MediaKind = playbackcap.MediaAudio
	}
	p.Digest, _ = p.ComputeDigest()
	return p
}

func conformanceExecute(t *testing.T, ffmpeg string, req Request) conformanceProbe {
	t.Helper()
	result, err := Compile(req)
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"-loglevel", "error"}, result.Args...)
	cmd := exec.Command(ffmpeg, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compiled FFmpeg graph failed: %v\nargs: %s\n%s", err, strings.Join(args, " "), output)
	}
	_, ffprobe := conformanceTools(t)
	return conformanceProbeFile(t, ffprobe, req.Output.ProgressivePath)
}

func TestFFmpegConformanceSDRH264AAC(t *testing.T) {
	ffmpeg, ffprobe := conformanceTools(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "sdr.mkv")
	conformanceFixture(t, ffmpeg, source,
		[]string{"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24:duration=0.8", "-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=48000:duration=0.8"},
		[]string{"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest"})
	input := conformanceProbeFile(t, ffprobe, source)
	if got := conformanceStreamOf(t, input, "video").CodecName; got != "h264" {
		t.Fatalf("fixture video codec = %q, want h264", got)
	}
	if got := conformanceStreamOf(t, input, "audio").CodecName; got != "aac" {
		t.Fatalf("fixture audio codec = %q, want aac", got)
	}

	facts := conformanceFacts(&mediafacts.Video{Index: 0, Codec: "h264", CodedWidth: 160, CodedHeight: 90, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p", BitDepth: 8, FieldOrder: "progressive", FrameRate: mediafacts.Rational{Num: 24, Den: 1}, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}, mediafacts.Audio{Index: 1, Codec: "aac", Layout: "mono", Channels: 1, SampleRate: 48000, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}})
	out := filepath.Join(dir, "sdr-output.mp4")
	probe := conformanceExecute(t, ffmpeg, Request{X264Preset: "medium", Plan: conformancePlan(facts, true), Facts: facts, SourcePath: source, Output: Output{ProgressivePath: out}})
	if got := conformanceStreamOf(t, probe, "video").CodecName; got != "h264" {
		t.Fatalf("compiled output video codec = %q, want h264", got)
	}
	if got := conformanceStreamOf(t, probe, "audio"); got.CodecName != "aac" || got.Channels != 2 {
		t.Fatalf("compiled output audio = %#v, want stereo AAC", got)
	}
}

func TestFFmpegConformanceAnamorphicPreservesDisplayAspect(t *testing.T) {
	ffmpeg, ffprobe := conformanceTools(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "anamorphic.mkv")
	conformanceFixture(t, ffmpeg, source,
		[]string{"-f", "lavfi", "-i", "testsrc2=size=120x90:rate=24:duration=0.8", "-f", "lavfi", "-i", "sine=sample_rate=48000:duration=0.8"},
		[]string{"-vf", "setsar=4/3", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest"})
	inputVideo := conformanceStreamOf(t, conformanceProbeFile(t, ffprobe, source), "video")
	if inputVideo.SampleAspectRatio != "4:3" || inputVideo.DisplayAspectRatio != "16:9" {
		t.Fatalf("anamorphic fixture geometry = %#v", inputVideo)
	}
	facts := conformanceFacts(&mediafacts.Video{Index: 0, Codec: "h264", CodedWidth: 120, CodedHeight: 90, SampleAspectRatio: mediafacts.Rational{Num: 4, Den: 3}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p", BitDepth: 8, FieldOrder: "progressive", FrameRate: mediafacts.Rational{Num: 24, Den: 1}, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}, mediafacts.Audio{Index: 1, Codec: "aac", Layout: "mono", Channels: 1, SampleRate: 48000, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}})
	out := filepath.Join(dir, "anamorphic-output.mp4")
	outputVideo := conformanceStreamOf(t, conformanceExecute(t, ffmpeg, Request{X264Preset: "medium", Plan: conformancePlan(facts, true), Facts: facts, SourcePath: source, Output: Output{ProgressivePath: out}}), "video")
	if outputVideo.SampleAspectRatio != "1:1" || outputVideo.DisplayAspectRatio != "16:9" {
		t.Fatalf("compiled graph changed display geometry: got SAR %s DAR %s (%dx%d), want square-pixel 16:9", outputVideo.SampleAspectRatio, outputVideo.DisplayAspectRatio, outputVideo.Width, outputVideo.Height)
	}
}

func TestFFmpegConformanceInterlacedInputIsDeinterlaced(t *testing.T) {
	ffmpeg, _ := conformanceTools(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "interlaced.mkv")
	conformanceFixture(t, ffmpeg, source,
		[]string{"-f", "lavfi", "-i", "testsrc2=size=160x96:rate=50:duration=0.8", "-f", "lavfi", "-i", "sine=sample_rate=48000:duration=0.8"},
		[]string{"-vf", "tinterlace=mode=interleave_top", "-flags", "+ildct+ilme", "-top", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest"})
	facts := conformanceFacts(&mediafacts.Video{Index: 0, Codec: "h264", CodedWidth: 160, CodedHeight: 96, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 5, Den: 3}, PixelFormat: "yuv420p", BitDepth: 8, FieldOrder: "tt", FrameRate: mediafacts.Rational{Num: 25, Den: 1}, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}, mediafacts.Audio{Index: 1, Codec: "aac", Layout: "mono", Channels: 1, SampleRate: 48000, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}})
	out := filepath.Join(dir, "deinterlaced.mp4")
	video := conformanceStreamOf(t, conformanceExecute(t, ffmpeg, Request{X264Preset: "medium", Plan: conformancePlan(facts, true), Facts: facts, SourcePath: source, Output: Output{ProgressivePath: out}}), "video")
	if video.FieldOrder != "progressive" {
		t.Fatalf("compiled output field order = %q, want progressive", video.FieldOrder)
	}
}

func TestFFmpegConformanceAudioOnly(t *testing.T) {
	ffmpeg, _ := conformanceTools(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "audio.flac")
	conformanceFixture(t, ffmpeg, source, []string{"-f", "lavfi", "-i", "sine=sample_rate=96000:duration=0.8"}, []string{"-c:a", "flac"})
	facts := conformanceFacts(nil, mediafacts.Audio{Index: 0, Codec: "flac", Layout: "mono", Channels: 1, SampleRate: 96000, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}})
	out := filepath.Join(dir, "audio.m4a")
	audio := conformanceStreamOf(t, conformanceExecute(t, ffmpeg, Request{X264Preset: "medium", Plan: conformancePlan(facts, false), Facts: facts, SourcePath: source, Output: Output{ProgressivePath: out}}), "audio")
	if audio.CodecName != "aac" || audio.Channels != 2 {
		t.Fatalf("audio-only output = %#v, want stereo AAC", audio)
	}
}

func TestFFmpegConformanceHDRMetadataWhenEncoderAvailable(t *testing.T) {
	ffmpeg, _ := conformanceTools(t)
	encoders := conformanceRun(t, ffmpeg, "-hide_banner", "-encoders")
	if !strings.Contains(encoders, "libx265") {
		t.Skip("HDR conformance skipped: local FFmpeg lacks libx265")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "hdr.mkv")
	conformanceFixture(t, ffmpeg, source,
		[]string{"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24:duration=0.5", "-f", "lavfi", "-i", "sine=sample_rate=48000:duration=0.5"},
		[]string{"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc", "-c:a", "aac", "-shortest"})
	facts := conformanceFacts(&mediafacts.Video{Index: 0, Codec: "hevc", CodedWidth: 160, CodedHeight: 90, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p10le", BitDepth: 10, ColorPrimaries: "bt2020", ColorTransfer: "smpte2084", ColorMatrix: "bt2020nc", FieldOrder: "progressive", FrameRate: mediafacts.Rational{Num: 24, Den: 1}, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}, mediafacts.Audio{Index: 1, Codec: "aac", Layout: "mono", Channels: 1, SampleRate: 48000, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}})
	p := conformancePlan(facts, true)
	p.Streams[0].OutputCodec = "hevc"
	p.Color = &playbackplan.ColorDecision{Input: "pq", Output: "pq", Action: "preserve"}
	p.Digest, _ = p.ComputeDigest()
	out := filepath.Join(dir, "hdr-output.mp4")
	video := conformanceStreamOf(t, conformanceExecute(t, ffmpeg, Request{X264Preset: "medium", Plan: p, Facts: facts, SourcePath: source, Output: Output{ProgressivePath: out}}), "video")
	if video.CodecName != "hevc" || video.ColorPrimaries != "bt2020" || video.ColorTransfer != "smpte2084" || video.ColorSpace != "bt2020nc" {
		t.Fatalf("HDR metadata not preserved in actual output: %#v", video)
	}
}

func TestFFmpegConformanceSubtitleBurnAndBitmapFailClosed(t *testing.T) {
	ffmpeg, _ := conformanceTools(t)
	filters := conformanceRun(t, ffmpeg, "-hide_banner", "-filters")
	if !strings.Contains(filters, " subtitles ") {
		t.Skip("text subtitle burn execution skipped: local FFmpeg lacks the libass subtitles filter")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "subtitle-source.mkv")
	conformanceFixture(t, ffmpeg, source,
		[]string{"-f", "lavfi", "-i", "color=c=black:size=160x90:rate=24:duration=0.8", "-f", "lavfi", "-i", "sine=sample_rate=48000:duration=0.8"},
		[]string{"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest"})
	subtitle := filepath.Join(dir, "caption.srt")
	if err := os.WriteFile(subtitle, []byte("1\n00:00:00,000 --> 00:00:00,700\nPORTICO\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	facts := conformanceFacts(&mediafacts.Video{Index: 0, Codec: "h264", CodedWidth: 160, CodedHeight: 90, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p", BitDepth: 8, FieldOrder: "progressive", FrameRate: mediafacts.Rational{Num: 24, Den: 1}, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}, mediafacts.Audio{Index: 1, Codec: "aac", Layout: "mono", Channels: 1, SampleRate: 48000, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}})
	facts.Subtitles = []mediafacts.Subtitle{{Index: 2, Codec: "subrip", Kind: "text", ClosedCaptionKnown: true, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}}
	p := conformancePlan(facts, true)
	p.Selection.SubtitleIndex = intptr(2)
	p.Streams = append(p.Streams, playbackplan.StreamAction{Index: 2, Kind: "subtitle", Action: playbackplan.BurnIn, InputCodec: "subrip"})
	p.Stages = append(p.Stages, playbackplan.Stage{Kind: "subtitle", Operation: "burn_in", Execution: "software"})
	p.Subtitle = playbackplan.SubtitleDecision{Index: intptr(2), Codec: "subrip", Kind: "text", Action: playbackplan.BurnIn}
	p.Digest, _ = p.ComputeDigest()
	out := filepath.Join(dir, "subtitle-output.mp4")
	_ = conformanceExecute(t, ffmpeg, Request{X264Preset: "medium", Plan: p, Facts: facts, SourcePath: source, SubtitlePath: subtitle, Output: Output{ProgressivePath: out}})

	bitmapFacts := facts.Clone()
	bitmapFacts.Subtitles[0].Codec = "hdmv_pgs_subtitle"
	bitmapFacts.Subtitles[0].Kind = "bitmap"
	bitmapPlan := p.Clone()
	bitmapPlan.Streams[len(bitmapPlan.Streams)-1].InputCodec = "hdmv_pgs_subtitle"
	bitmapPlan.Subtitle.Codec = "hdmv_pgs_subtitle"
	bitmapPlan.Subtitle.Kind = "bitmap"
	bitmapPlan.Digest, _ = bitmapPlan.ComputeDigest()
	_, err := Compile(Request{X264Preset: "medium", Plan: bitmapPlan, Facts: bitmapFacts, SourcePath: source, Output: Output{ProgressivePath: filepath.Join(dir, "must-not-exist.mp4")}})
	var compileErr *CompileError
	if !errors.As(err, &compileErr) || compileErr.Code != UnsupportedGraph || !strings.Contains(compileErr.Detail, "exact subtitle path") {
		t.Fatalf("bitmap burn without exact source did not fail closed: %v", err)
	}
}

func TestFFmpegConformanceBitmapSubtitleBurnRequiresExactSource(t *testing.T) {
	facts := conformanceFacts(&mediafacts.Video{Index: 0, Codec: "h264", CodedWidth: 160, CodedHeight: 90, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p", BitDepth: 8, FieldOrder: "progressive", FrameRate: mediafacts.Rational{Num: 24, Den: 1}, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}, mediafacts.Audio{Index: 1, Codec: "aac", Layout: "mono", Channels: 1, SampleRate: 48000, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}})
	facts.Subtitles = []mediafacts.Subtitle{{Index: 2, Codec: "hdmv_pgs_subtitle", Kind: "bitmap", ClosedCaptionKnown: true, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}}}
	p := conformancePlan(facts, true)
	p.Selection.SubtitleIndex = intptr(2)
	p.Streams = append(p.Streams, playbackplan.StreamAction{Index: 2, Kind: "subtitle", Action: playbackplan.BurnIn, InputCodec: "hdmv_pgs_subtitle"})
	p.Stages = append(p.Stages, playbackplan.Stage{Kind: "subtitle", Operation: "burn_in", Execution: "software"})
	p.Subtitle = playbackplan.SubtitleDecision{Index: intptr(2), Codec: "hdmv_pgs_subtitle", Kind: "bitmap", Action: playbackplan.BurnIn}
	p.Digest, _ = p.ComputeDigest()
	_, err := Compile(Request{X264Preset: "medium", Plan: p, Facts: facts, SourcePath: "/media/movie.mkv", Output: Output{ProgressivePath: "/work/must-not-exist.mp4"}})
	var compileErr *CompileError
	if !errors.As(err, &compileErr) || compileErr.Code != UnsupportedGraph || !strings.Contains(compileErr.Detail, "exact subtitle path") {
		t.Fatalf("bitmap burn without exact source did not fail closed: %v", err)
	}
}

func TestFFmpegConformanceIsRuntimeBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("runtime smoke is redundant in short mode")
	}
	start := time.Now()
	ffmpeg, _ := conformanceTools(t)
	output := filepath.Join(t.TempDir(), "bounded.m4a")
	conformanceFixture(t, ffmpeg, output, []string{"-f", "lavfi", "-i", "sine=duration=0.1"}, []string{"-c:a", "aac"})
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("single tiny conformance fixture took %s", elapsed)
	}
	if info, err := os.Stat(output); err != nil || info.Size() <= 0 {
		t.Fatalf("bounded fixture missing: size=%s err=%v", strconv.FormatInt(func() int64 {
			if info == nil {
				return 0
			}
			return info.Size()
		}(), 10), err)
	}
}
