package decoder

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"portico.local/server/internal/decodertest"
	"portico.local/server/internal/mediaexec"
	"strings"
	"testing"
	"time"
)

func argValue(args []string, key string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}
func targetFacts() *ToolchainFacts {
	return &ToolchainFacts{Encoders: map[string]bool{"libx264": true, "libx265": true, "libaom-av1": true, "aac": true, "ac3": true, "eac3": true, "libopus": true, "flac": true}, Filters: map[string]bool{"zscale": true, "tonemap": true, "ass": true, "overlay": true}}
}

func TestConversionTargetsDescribeEveryOutput(t *testing.T) {
	for _, tc := range []struct {
		codec, rangeName, pixel, transfer string
		depth                             int
		encoder                           string
	}{{"h264", "sdr", "yuv420p", "bt709", 8, "libx264"}, {"hevc", "sdr", "yuv420p", "bt709", 8, "libx265"}, {"hevc", "hdr10", "yuv420p10le", "smpte2084", 10, "libx265"}, {"hevc", "hlg", "yuv420p10le", "arib-std-b67", 10, "libx265"}, {"av1", "hdr10", "yuv420p10le", "smpte2084", 10, "libaom-av1"}} {
		t.Run(tc.codec+tc.rangeName, func(t *testing.T) {
			target := ConversionTarget{VideoCodec: tc.codec, BitDepth: tc.depth, DynamicRange: tc.rangeName, RateControl: "quality", Height: 1080}
			g, err := BuildConversion(ConversionRequest{Target: &target, ConvertVideo: true, Toolchain: targetFacts()})
			if err != nil {
				t.Fatal(err)
			}
			if argValue(g.Video, "-c:v") != tc.encoder || argValue(g.Video, "-pix_fmt") != tc.pixel || argValue(g.Video, "-color_trc") != tc.transfer || argValue(g.Video, "-crf") == "" {
				t.Fatal(g.Video)
			}
			if tc.codec != "h264" && argValue(g.Video, "-tag:v") == "dvh1" {
				t.Fatal("conversion advertised Dolby Vision")
			}
		})
	}
	for _, codec := range []string{"aac", "ac3", "eac3", "opus", "flac"} {
		target := ConversionTarget{AudioCodec: codec, AudioChannels: 6}
		g, err := BuildConversion(ConversionRequest{Target: &target, ConvertAudio: true, Toolchain: targetFacts()})
		if err != nil {
			t.Fatal(codec, err)
		}
		if argValue(g.Audio, "-ac") != "6" || argValue(g.Audio, "-ar") != "48000" || argValue(g.Audio, "-channel_layout") != "5.1" {
			t.Fatal(g.Audio)
		}
		if codec == "flac" && argValue(g.Audio, "-b:a") != "" {
			t.Fatal("lossless audio got a lossy bitrate", g.Audio)
		}
	}
	for _, codec := range []string{"flac", "opus"} {
		target := ConversionTarget{AudioCodec: codec, AudioChannels: 8}
		g, err := BuildConversion(ConversionRequest{Target: &target, ConvertAudio: true, Downmix: true, Toolchain: targetFacts()})
		if err != nil || argValue(g.Audio, "-channel_layout") != "7.1" {
			t.Fatal(g, err)
		}
	}
	for _, codec := range []string{"ac3", "eac3", "dts", "dts_hd", "truehd"} {
		target := ConversionTarget{VideoCodec: "hevc", AudioCodec: codec, AudioChannels: 8}
		g, err := BuildConversion(ConversionRequest{Target: &target, CopyVideo: true, CopyAudio: true})
		if err != nil || argValue(g.Audio, "-c:a") != "copy" {
			t.Fatal(codec, g, err)
		}
	}
}

func TestConversionTargetRefusesWrongPicturesAndContainers(t *testing.T) {
	for _, target := range []ConversionTarget{{VideoCodec: "hevc", BitDepth: 8, DynamicRange: "hdr10"}, {VideoCodec: "hevc", Packaging: "mpegts_hls"}, {SourceRange: "hdr10", DynamicRange: "sdr"}, {VideoCodec: "hevc", DolbyVisionProfile: 5, ToneMap: true}, {VideoCodec: "hevc", MasterDisplay: "evil:params=1"}} {
		if _, err := BuildConversion(ConversionRequest{Target: &target, ConvertVideo: true, Toolchain: targetFacts()}); err == nil {
			t.Fatal("accepted invalid target", target)
		}
	}
	target := ConversionTarget{AudioCodec: "eac3", AudioChannels: 8}
	if _, err := BuildConversion(ConversionRequest{Target: &target, ConvertAudio: true}); err == nil {
		t.Fatal("claimed unsupported E-AC-3 7.1 encoder")
	}
}

func TestHEVCEncoderNeedsCodecAndDepthQualification(t *testing.T) {
	for _, backend := range HardwareBackends {
		if !backendSpecs[backend].supported() {
			continue
		}
		target := ConversionTarget{VideoCodec: "hevc", BitDepth: 10, DynamicRange: "hdr10"}
		probe := HardwareProbe{Backend: backend, Available: true, OutputEncoders: map[string]bool{"hevc:8": true}}
		g, err := BuildConversion(ConversionRequest{Target: &target, Probe: probe, ConvertVideo: true, Toolchain: targetFacts()})
		if err != nil || g.Backend != BackendSoftware {
			t.Fatal(g, err)
		}
		probe.OutputEncoders["hevc:10"] = true
		g, err = BuildConversion(ConversionRequest{Target: &target, Probe: probe, ConvertVideo: true, Toolchain: targetFacts()})
		if err != nil || argValue(g.Video, "-c:v") != "hevc_"+string(backend) {
			t.Fatal(g, err)
		}
		if backend == BackendVAAPI && !strings.Contains(argValue(g.Video, "-vf"), "p010") {
			t.Fatal(g.Video)
		}
	}
}

func runTinyMedia(t *testing.T, ctx context.Context, binary string, args []string, dir string) {
	t.Helper()
	cmd, err := mediaexec.CommandContext(ctx, mediaexec.Job{Executable: binary, Args: args, ReadPaths: []string{dir}, WriteDirs: []string{dir}, Fonts: true})
	if err != nil {
		t.Fatal(err)
	}
	cmd.WaitDelay = time.Second
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s\nargs: %v", err, output, args)
	}
}

func TestSoftwareTargetsEncodeTinySamples(t *testing.T) {
	binary, probe := decodertest.QualifiedFFmpeg(t), decodertest.QualifiedFFprobe(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	facts, err := ProbeToolchain(ctx, binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		codec, rangeName string
		depth            int
	}{{"h264", "sdr", 8}, {"hevc", "sdr", 8}, {"hevc", "hdr10", 10}, {"hevc", "hlg", 10}, {"av1", "sdr", 10}} {
		t.Run(tc.codec+tc.rangeName, func(t *testing.T) {
			if _, err := softwareVideoEncoder(tc.codec, &facts); err != nil {
				t.Skip(err)
			}
			dir := t.TempDir()
			target := ConversionTarget{VideoCodec: tc.codec, BitDepth: tc.depth, DynamicRange: tc.rangeName, Width: 160, Height: 90, FrameRate: 24, RateControl: "quality"}
			g, err := BuildConversion(ConversionRequest{Target: &target, ConvertVideo: true, SoftwarePreset: "ultrafast", Toolchain: &facts})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "sample.mp4")
			args := []string{"-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=s=160x90:r=24:d=0.5"}
			args = append(args, g.Video...)
			args = append(args, "-an", "-f", "mp4", path)
			runTinyMedia(t, ctx, binary, args, dir)
			cmd, err := mediaexec.CommandContext(ctx, mediaexec.Job{Executable: probe, Args: []string{"-v", "error", "-show_entries", "stream=codec_name,pix_fmt,color_transfer,color_primaries,color_space,width,height", "-of", "json", path}, ReadPaths: []string{path}})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Streams []struct {
					Codec    string `json:"codec_name"`
					Pixel    string `json:"pix_fmt"`
					Transfer string `json:"color_transfer"`
				}
			}
			if json.Unmarshal(raw, &got) != nil || len(got.Streams) != 1 || got.Streams[0].Codec != tc.codec {
				t.Fatalf("wrong output %s", raw)
			}
			if tc.depth == 10 && !strings.Contains(got.Streams[0].Pixel, "10") {
				t.Fatalf("lost ten-bit output %s", raw)
			}
			runTinyMedia(t, ctx, binary, []string{"-v", "error", "-i", path, "-f", "null", "-"}, dir)
		})
	}
	for _, codec := range []string{"aac", "ac3", "eac3", "opus", "flac"} {
		t.Run(codec, func(t *testing.T) {
			dir := t.TempDir()
			channels := 6
			if codec == "opus" || codec == "flac" {
				channels = 8
			}
			target := ConversionTarget{AudioCodec: codec, AudioChannels: channels}
			g, err := BuildConversion(ConversionRequest{Target: &target, ConvertAudio: true, Toolchain: &facts})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "audio.mp4")
			args := []string{"-v", "error", "-f", "lavfi", "-i", "anullsrc=r=96000:cl=7.1", "-t", "0.25"}
			args = append(args, g.Audio...)
			args = append(args, "-strict", "-2", path)
			runTinyMedia(t, ctx, binary, args, dir)
			runTinyMedia(t, ctx, binary, []string{"-v", "error", "-i", path, "-f", "null", "-"}, dir)
		})
	}
}

func TestHDRBurnKeepsSourceRange(t *testing.T) {
	binary := decodertest.QualifiedFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dir := t.TempDir()
	file := filepath.Join(dir, "cue.ass")
	if err := os.WriteFile(file, []byte("[Script Info]\nScriptType: v4.00+\nPlayResX: 160\nPlayResY: 90\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, Bold, Alignment\nStyle: Default,Arial,24,&H00FFFFFF,1,5\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:00.00,0:00:02.00,Default,BURN\n"), 0600); err != nil {
		t.Fatal(err)
	}
	target := ConversionTarget{VideoCodec: "hevc", BitDepth: 10, DynamicRange: "hdr10", Width: 160, Height: 90, FrameRate: 24}
	g, err := BuildConversion(ConversionRequest{Target: &target, ConvertVideo: true, BurnIn: &BurnIn{File: file, Format: "ass", Width: 160, Height: 90}, SoftwarePreset: "ultrafast"})
	if err != nil {
		t.Fatal(err)
	}
	if g.VideoMap == "" {
		t.Fatal("HDR burn did not use a separate subtitle plane")
	}
	args := []string{"-v", "error", "-f", "lavfi", "-i", "color=black:s=160x90:r=24:d=0.25,format=yuv420p10le,setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc"}
	args = append(args, g.Video...)
	args = append(args, "-map", g.VideoMap, "-an", filepath.Join(dir, "burn.mp4"))
	runTinyMedia(t, ctx, binary, args, dir)
}
