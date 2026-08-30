package ffmpeggraph

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestFFmpegMP3PacketCopyPreservesDecodedGaplessTimeline(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	source, copied := filepath.Join(dir, "source.mp3"), filepath.Join(dir, "copied.mp3")
	run := func(args ...string) []byte {
		cmd := exec.Command(ffmpeg, args...)
		out, err := cmd.Output()
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) && bytes.Contains(exit.Stderr, []byte("Unknown encoder")) {
				t.Skip("local ffmpeg lacks libmp3lame")
			}
			stderr := []byte(nil)
			if exit != nil {
				stderr = exit.Stderr
			}
			t.Fatalf("ffmpeg %v: %v: %s", args, err, stderr)
		}
		return out
	}
	run("-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=44100:duration=0.25", "-c:a", "libmp3lame", "-write_xing", "1", source)
	copyArgs, err := CompileAudio(AudioRequest{InputCodec: "mp3", InputLayout: "mono", InputChannels: 1, OutputCodec: "mp3", OutputLayout: "mono", OutputChannels: 1, OutputContainer: "mp3", Copy: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(copyArgs, " "), "-write_xing 1") {
		t.Fatalf("compiled MP3 packet copy omitted Xing metadata: %v", copyArgs)
	}
	actualCopyArgs := []string{"-hide_banner", "-loglevel", "error", "-i", source, "-map", "0:a:0"}
	actualCopyArgs = append(actualCopyArgs, copyArgs...)
	actualCopyArgs = append(actualCopyArgs, copied)
	run(actualCopyArgs...)
	decode := func(path string) []byte {
		return run("-hide_banner", "-loglevel", "error", "-i", path, "-map", "0:a:0", "-f", "s16le", "-acodec", "pcm_s16le", "-")
	}
	a, b := decode(source), decode(copied)
	if len(a) == 0 || !bytes.Equal(a, b) {
		t.Fatalf("packet copy changed decoded samples: source=%d copied=%d", len(a), len(b))
	}
	if _, err := os.Stat(copied); err != nil {
		t.Fatal(err)
	}
}

func audioConversionFixture(inputCodec, inputLayout string, inputChannels int, outputCodec, outputLayout string, outputChannels int) Request {
	f := exactFacts()
	f.Video = nil
	f.Subtitles = nil
	f.Audio[0].Codec, f.Audio[0].Layout, f.Audio[0].Channels, f.Audio[0].SampleRate = inputCodec, inputLayout, inputChannels, 48000
	p := playbackplan.Plan{SchemaRevision: playbackplan.SchemaRevision, SourceFingerprint: f.Source.Fingerprint, SourceRevision: f.Source.Revision, CapabilityEvidenceID: "audio-test", Policy: playbackplan.MaximumFidelity, Mode: playbackplan.DirectStream, MediaKind: "audio", Protocol: "http", Container: "matroska", Selection: playbackplan.Selection{AudioIndex: intptr(5)}, Streams: []playbackplan.StreamAction{{Index: 5, Kind: "audio", Action: playbackplan.Convert, InputCodec: inputCodec, OutputCodec: outputCodec, InputLayout: inputLayout, OutputLayout: outputLayout}}, Stages: []playbackplan.Stage{{Kind: "audio", Operation: "encode", Execution: "software"}, {Kind: "mux", Operation: "package", Execution: "stream"}}, Audio: playbackplan.AudioDecision{Codec: outputCodec, Layout: outputLayout, Channels: outputChannels, Downmixed: inputChannels > outputChannels}, Subtitle: playbackplan.SubtitleDecision{Action: playbackplan.Drop}}
	p.Digest, _ = p.ComputeDigest()
	return Request{Plan: p, Facts: f, SourcePath: "/audio/source", Output: Output{ProgressivePath: "/audio/output.mka"}}
}

func TestAudioDownmixUsesExplicitMatrixHeadroomLFEAndLimiter(t *testing.T) {
	r, err := Compile(audioConversionFixture("truehd", "7.1", 8, "eac3", "stereo", 2))
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(r.Args, " ")
	for _, want := range []string{"-af pan=stereo", "0.25*LFE", "0.5*FC", "0.5*BL", "0.35*SL", "alimiter=limit=0.95", "-c:a eac3 -ac 2 -channel_layout stereo"} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in %s", want, args)
		}
	}
}

func TestAudioOnlyGeneratedHLSCompilesToAACMPEGTS(t *testing.T) {
	req := audioConversionFixture("flac", "mono", 1, "aac", "mono", 1)
	req.Plan.Protocol = "hls"
	req.Plan.Container = "mpegts"
	req.Plan.SegmentFormat = "mpegts"
	req.Plan.Digest, _ = req.Plan.ComputeDigest()
	req.Output = Output{ManifestPath: "/audio/master.m3u8", SegmentPattern: "/audio/segment-%05d.ts", SegmentSeconds: 4}
	result, err := Compile(req)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(result.Args, " ")
	for _, want := range []string{"-map 0:5", "-c:a aac", "-ac 1", "-channel_layout mono", "-f hls", "-hls_segment_options mpegts_copyts=0"} {
		if !strings.Contains(args, want) {
			t.Fatalf("audio-only HLS graph missing %q: %s", want, args)
		}
	}
	if result.VideoMap != "" || result.AudioMap != "0:5" || result.SegmentFormat != "mpegts" {
		t.Fatalf("audio-only HLS result = %#v", result)
	}
}

func TestAudioLayoutPreservationHasNoImplicitDownmix(t *testing.T) {
	r, err := Compile(audioConversionFixture("dts", "5.1", 6, "flac", "5.1", 6))
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(r.Args, " ")
	if strings.Contains(args, "pan=") || !strings.Contains(args, "-af asetpts=PTS-STARTPTS") || !strings.Contains(args, "-ac 6 -channel_layout 5.1") {
		t.Fatalf("layout not preserved: %s", args)
	}
}

func TestAudioTopologyAliasesKeepSideBackAndQuadChannelsDistinct(t *testing.T) {
	for _, tc := range []struct {
		layout string
		want   []string
		not    []string
	}{
		{"5.1(side)", []string{"0.5*SL", "0.5*SR"}, []string{"0.5*BL", "0.5*BR"}},
		{"5.1", []string{"0.5*BL", "0.5*BR"}, []string{"0.5*SL", "0.5*SR"}},
		{"quad", []string{"0.5*BL", "0.5*BR"}, []string{"0.5*FC", "0.35*BC"}},
		{"4.0", []string{"0.5*FC", "0.35*BC"}, []string{"0.5*BL", "0.5*BR"}},
	} {
		t.Run(tc.layout, func(t *testing.T) {
			r, err := Compile(audioConversionFixture("flac", tc.layout, exactAudioLayouts[tc.layout], "aac", "stereo", 2))
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Join(r.Args, " ")
			for _, want := range tc.want {
				if !strings.Contains(args, want) {
					t.Fatalf("%s missing %s: %s", tc.layout, want, args)
				}
			}
			for _, forbidden := range tc.not {
				if strings.Contains(args, forbidden) {
					t.Fatalf("%s used wrong topology %s: %s", tc.layout, forbidden, args)
				}
			}
		})
	}
}

func TestAudioCompilerFailsClosedOnUnknownLayoutAndUnsupportedReduction(t *testing.T) {
	for _, req := range []Request{
		audioConversionFixture("flac", "mystery", 6, "aac", "stereo", 2),
		audioConversionFixture("flac", "5.1", 6, "aac", "3.0", 3),
	} {
		_, err := Compile(req)
		var ce *CompileError
		if !errors.As(err, &ce) || ce.Code != UnsupportedGraph {
			t.Fatalf("expected closed graph, got %v", err)
		}
	}
}

func TestAudioEncoderFamiliesAreExplicit(t *testing.T) {
	for codec, encoder := range map[string]string{"aac": "aac", "ac3": "ac3", "eac3": "eac3", "opus": "libopus", "flac": "flac", "mp3": "libmp3lame", "pcm_s16le": "pcm_s16le", "pcm_s24le": "pcm_s24le"} {
		r, err := Compile(audioConversionFixture("truehd", "stereo", 2, codec, "stereo", 2))
		if err != nil {
			t.Fatalf("%s: %v", codec, err)
		}
		if !strings.Contains(strings.Join(r.Args, " "), "-c:a "+encoder) {
			t.Errorf("%s did not use %s", codec, encoder)
		}
	}
	for _, codec := range []string{"truehd", "dts", "dts-hd"} {
		_, err := Compile(audioConversionFixture("flac", "stereo", 2, codec, "stereo", 2))
		if err == nil {
			t.Errorf("lossy/unverified %s encoder accepted", codec)
		}
	}
}
