package ffmpegprobe

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRuntimeConformanceExecutesRepresentativeDecode(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg runtime conformance skipped: ffmpeg is not installed")
	}
	version, err := exec.Command(ffmpeg, "-hide_banner", "-version").CombinedOutput()
	if err != nil {
		t.Skipf("FFmpeg runtime conformance skipped: version identity unavailable: %v", err)
	}
	build := strings.TrimSpace(strings.SplitN(string(version), "\n", 2)[0])
	id := Identity{
		BinaryFingerprint: "runtime-conformance:" + ffmpeg,
		FFmpegBuild:       build,
		FFmpegConfigure:   build,
		Backend:           "software",
		DeviceIdentity:    "host-cpu",
		DevicePath:        "none",
		DriverIdentity:    "software",
		DriverVersion:     "builtin",
		OS:                runtime.GOOS,
		Arch:              runtime.GOARCH,
		ConfigRevision:    "w5-conformance-v1",
	}
	probe := Probe{
		Name: "synthetic-sdr-h264-decode", Kind: Decode,
		Args: []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=12:duration=0.2", "-frames:v", "2", "-f", "null", "-"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	report, err := (Runner{Binary: ffmpeg, Timeout: 4 * time.Second, MaxOutputBytes: 16 << 10}).Probe(ctx, id, []Probe{probe})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].Status != Supported {
		t.Fatalf("representative runtime probe = %#v, want supported", report.Results)
	}
}
