package app

import (
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestPlaybackHardwareOutputDimensionsPreserveAspectAndEncoderAlignment(t *testing.T) {
	video := mediafacts.Video{CodedWidth: 3840, CodedHeight: 2160}
	w, h := playbackHardwareOutputDimensions(video, playbackplan.Constraints{MaxHeight: 1080})
	if w != 1920 || h != 1080 {
		t.Fatalf("dimensions = %dx%d, want 1920x1080", w, h)
	}
	w, h = playbackHardwareOutputDimensions(mediafacts.Video{CodedWidth: 1919, CodedHeight: 1079}, playbackplan.Constraints{})
	if w != 0 || h != 0 {
		t.Fatalf("unconstrained dimensions = %dx%d, want no scale operation", w, h)
	}
}

func TestPlaybackHardwareRouteContainsOnlyVerifiedStageSummary(t *testing.T) {
	plan := playbackhw.Plan{Backend: playbackhw.QSV, Stages: []playbackhw.Stage{
		{Operation: playbackhw.Decode, Execution: playbackhw.Hardware, Args: []string{"/dev/dri/renderD128", "secret"}},
		{Operation: playbackhw.Encode, Execution: playbackhw.Hardware, Args: []string{"-c:v", "h264_qsv"}},
	}}
	route := playbackHardwareRouteFromPlan(plan)
	if !route.Verified || route.Backend != playbackhw.QSV || len(route.Stages) != 2 {
		t.Fatalf("unexpected route: %#v", route)
	}
	for _, stage := range route.Stages {
		if stage.Kind != "hardware" || stage.Operation == "/dev/dri/renderD128" || stage.Execution == "secret" {
			t.Fatalf("private execution detail escaped into immutable public route: %#v", stage)
		}
	}
}

func TestPlaybackHardwareVersionIdentityBindsConfigureLine(t *testing.T) {
	version, configure := playbackHardwareVersionIdentity("ffmpeg version 7.1-portico\nconfiguration: --enable-videotoolbox --enable-libx264\n")
	if version != "ffmpeg version 7.1-portico" || len(configure) != len("sha256:")+64 {
		t.Fatalf("unexpected identity: version=%q configure=%q", version, configure)
	}
	_, changed := playbackHardwareVersionIdentity("ffmpeg version 7.1-portico\nconfiguration: --enable-libx264\n")
	if changed == configure {
		t.Fatal("configure change did not change executable identity")
	}
}

func TestPlaybackHardwareAutoCandidatesTryVerifiedLinuxFallback(t *testing.T) {
	candidates := playbackHardwareConfiguredCandidates("auto", playbackhw.Linux, playbackhw.Intel)
	if len(candidates) != 2 || candidates[0] != "qsv" || candidates[1] != "vaapi" {
		t.Fatalf("Linux Intel candidates = %#v, want QSV then VAAPI", candidates)
	}
	if explicit := playbackHardwareConfiguredCandidates("vaapi", playbackhw.Linux, playbackhw.Intel); len(explicit) != 1 || explicit[0] != "vaapi" {
		t.Fatalf("explicit backend was rewritten: %#v", explicit)
	}
}

func TestPlaybackHardwareH264OutputSealsEightBitTarget(t *testing.T) {
	video := mediafacts.Video{Codec: "hevc", PixelFormat: "yuv420p10le", BitDepth: 10}
	plan := playbackplan.Plan{Color: &playbackplan.ColorDecision{Input: "sdr", Output: "sdr", Action: "preserve"}}
	if got := playbackHardwareOutputBitDepth(video, playbackhw.H264, plan); got != 8 {
		t.Fatalf("H.264 output depth = %d, want 8", got)
	}
	if got := playbackHardwareOutputBitDepth(video, playbackhw.HEVC, playbackplan.Plan{}); got != 10 {
		t.Fatalf("HEVC Main10 output depth = %d, want 10", got)
	}
}
