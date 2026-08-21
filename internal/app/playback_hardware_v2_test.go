package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/ffmpegprobe"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
)

type playbackHardwareFakeExecutor struct {
	mu       sync.Mutex
	commands []ffmpegprobe.Command
	failAt   int
}

func (f *playbackHardwareFakeExecutor) Run(_ context.Context, command ffmpegprobe.Command, _ int64) ffmpegprobe.Execution {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, command)
	if f.failAt > 0 && len(f.commands) == f.failAt {
		return ffmpegprobe.Execution{ExitCode: 1, Output: "device creation failed", Err: errors.New("exit status 1")}
	}
	return ffmpegprobe.Execution{ExitCode: 0}
}

func (f *playbackHardwareFakeExecutor) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.commands)
}

func playbackHardwareTestRequest() playbackHardwareRuntimeRequest {
	return playbackHardwareRuntimeRequest{
		Settings:   transcodeSettings{HardwareAcceleration: true, HardwareEncoding: true, HardwareDevice: "videotoolbox"},
		FFmpegPath: "/opt/portico/ffmpeg", BinaryFingerprint: "sha256:ffmpeg-one",
		FFmpegBuild: "ffmpeg 7.1", FFmpegConfigure: "sha256:configure-one",
		OS: playbackhw.Darwin, Arch: "arm64", Vendor: playbackhw.Apple,
		DeviceIdentity: "apple-gpu:0", DriverIdentity: "videotoolbox", DriverVersion: "15.0",
		ProbeInputs: map[playbackhw.Codec]string{playbackhw.H264: "/opt/portico/probes/h264-8bit.mp4"},
		Pipeline: playbackhw.Request{
			InputCodec: playbackhw.H264, OutputCodec: playbackhw.H264,
			InputBitDepth: 8, OutputBitDepth: 8,
			InputPixelFormat: playbackhw.YUV420P, OutputPixelFormat: playbackhw.YUV420P,
		},
	}
}

func TestPlaybackHardwareRuntimeRequiresExecutableEvidence(t *testing.T) {
	executor := &playbackHardwareFakeExecutor{}
	runtime := newPlaybackHardwareRuntime(executor)
	result := runtime.Resolve(context.Background(), playbackHardwareTestRequest())
	if result.SoftwareOnly || result.Evidence == nil || !result.Evidence.Executable || !result.Evidence.Complete {
		t.Fatalf("expected a verified hardware route, got %#v", result)
	}
	if result.Backend != playbackhw.VideoToolbox || len(result.Plan.Stages) != 2 {
		t.Fatalf("unexpected hardware plan: %#v", result.Plan)
	}
	for _, command := range executor.commands {
		joined := strings.Join(command.Args, " ")
		if strings.Contains(joined, "-encoders") || strings.Contains(joined, "-filters") || strings.Contains(joined, "-hwaccels") {
			t.Fatalf("listing-only command was treated as a probe: %s", joined)
		}
	}
}

func TestPlaybackHardwareRuntimeFailsClosedOnProbeFailure(t *testing.T) {
	executor := &playbackHardwareFakeExecutor{failAt: 2}
	result := newPlaybackHardwareRuntime(executor).Resolve(context.Background(), playbackHardwareTestRequest())
	if !result.SoftwareOnly || result.Evidence != nil || result.Plan.Backend != "" {
		t.Fatalf("failed executable probe must select software, got %#v", result)
	}
	if !strings.Contains(result.Reason, "not completely verified") {
		t.Fatalf("unexpected fallback reason %q", result.Reason)
	}
}

func TestPlaybackHardwareRuntimeCacheIsBoundToExactIdentity(t *testing.T) {
	executor := &playbackHardwareFakeExecutor{}
	runtime := newPlaybackHardwareRuntime(executor)
	req := playbackHardwareTestRequest()
	first := runtime.Resolve(context.Background(), req)
	if first.SoftwareOnly {
		t.Fatalf("first resolve failed: %s", first.Reason)
	}
	count := executor.count()
	second := runtime.Resolve(context.Background(), req)
	if second.SoftwareOnly || !second.Report.FromCache || executor.count() != count {
		t.Fatalf("exact identity should reuse executable evidence: %#v", second)
	}
	req.DriverVersion = "15.1"
	third := runtime.Resolve(context.Background(), req)
	if third.SoftwareOnly || third.Report.FromCache || executor.count() == count {
		t.Fatalf("driver change must execute a fresh probe set: %#v", third)
	}
}

func TestPlaybackHardwareRuntimeRejectsIncompleteIdentityBeforeExecution(t *testing.T) {
	executor := &playbackHardwareFakeExecutor{}
	req := playbackHardwareTestRequest()
	req.BinaryFingerprint = ""
	result := newPlaybackHardwareRuntime(executor).Resolve(context.Background(), req)
	if !result.SoftwareOnly || executor.count() != 0 {
		t.Fatalf("incomplete identity must fail before execution: %#v", result)
	}
}

func TestPlaybackHardwareRuntimeDoesNotTurnListingSettingsIntoEncodeAuthority(t *testing.T) {
	executor := &playbackHardwareFakeExecutor{}
	req := playbackHardwareTestRequest()
	req.Settings.HardwareEncoding = false
	result := newPlaybackHardwareRuntime(executor).Resolve(context.Background(), req)
	if !result.SoftwareOnly || executor.count() != 0 {
		t.Fatalf("decode-only configuration must not launch a hardware encoder: %#v", result)
	}
}

func TestPlaybackHardwareRuntimeAllowsVerifiedAMFEncodeOnly(t *testing.T) {
	executor := &playbackHardwareFakeExecutor{}
	req := playbackHardwareTestRequest()
	req.Settings = transcodeSettings{HardwareEncoding: true, HardwareDevice: "amf"}
	req.OS, req.Vendor = playbackhw.Windows, playbackhw.AMD
	req.DeviceIdentity, req.DevicePath = "pci-amd-1", "platform-default"
	req.DriverIdentity, req.DriverVersion = "windows-display-driver", "1.2.3"
	result := newPlaybackHardwareRuntime(executor).Resolve(context.Background(), req)
	if result.SoftwareOnly || result.Backend != playbackhw.AMF {
		t.Fatalf("verified encode-only AMF route was not reachable: %#v", result)
	}
	if len(result.Plan.Stages) != 1 || result.Plan.Stages[0].Operation != playbackhw.Encode || len(result.Plan.InputArgs) != 0 {
		t.Fatalf("AMF encode-only plan attempted hardware decode: %#v", result.Plan)
	}
}

func TestPlaybackHardwareRuntimeHonorsDisabledHEVCDecodeWithHardwareEncode(t *testing.T) {
	executor := &playbackHardwareFakeExecutor{}
	req := playbackHardwareTestRequest()
	req.Settings.HardwareDecodeHEVC = false
	req.Pipeline.InputCodec = playbackhw.HEVC
	req.ProbeInputs = map[playbackhw.Codec]string{playbackhw.HEVC: "/opt/portico/probes/hevc-8bit.mp4"}
	result := newPlaybackHardwareRuntime(executor).Resolve(context.Background(), req)
	if result.SoftwareOnly {
		t.Fatalf("hardware encode with software HEVC decode should remain available: %s", result.Reason)
	}
	if len(result.Plan.Stages) != 1 || result.Plan.Stages[0].Operation != playbackhw.Encode || len(result.Plan.InputArgs) != 0 {
		t.Fatalf("disabled HEVC decode produced a hardware decode stage: %#v", result.Plan)
	}
}

func TestPlaybackHardwareRuntimeDeclinesUnverifiedVideoToolboxCrossover(t *testing.T) {
	executor := &playbackHardwareFakeExecutor{}
	req := playbackHardwareTestRequest()
	req.Pipeline.Deinterlace = true
	result := newPlaybackHardwareRuntime(executor).Resolve(context.Background(), req)
	if !result.SoftwareOnly || result.Reason != "videotoolbox software-filter crossover has no verified device context" {
		t.Fatalf("unexpected VideoToolbox fallback: %#v", result)
	}
	if executor.count() != 0 {
		t.Fatal("an unconstructable VideoToolbox crossover must fail before probing")
	}
}

func TestConfiguredPlaybackHardwareBackendPlatformMatrix(t *testing.T) {
	tests := []struct {
		value  string
		os     playbackhw.OS
		vendor playbackhw.DeviceVendor
		want   playbackhw.Backend
		wantOK bool
	}{
		{"auto", playbackhw.Darwin, playbackhw.Apple, playbackhw.VideoToolbox, true},
		{"auto", playbackhw.Linux, playbackhw.Intel, playbackhw.QSV, true},
		{"auto", playbackhw.Linux, playbackhw.AMD, playbackhw.VAAPI, true},
		{"auto", playbackhw.Linux, playbackhw.Nvidia, playbackhw.NVIDIA, true},
		{"auto", playbackhw.Windows, playbackhw.AMD, playbackhw.AMF, true},
		{"nvenc", playbackhw.Windows, playbackhw.Nvidia, playbackhw.NVIDIA, true},
		{"videotoolbox", playbackhw.Linux, playbackhw.Intel, "", false},
		{"d3d11va", playbackhw.Windows, playbackhw.Intel, "", false},
	}
	for _, test := range tests {
		got, ok := configuredPlaybackHardwareBackend(test.value, test.os, test.vendor)
		if ok != test.wantOK || got != test.want {
			t.Errorf("configuredPlaybackHardwareBackend(%q,%q,%q)=(%q,%v), want (%q,%v)", test.value, test.os, test.vendor, got, ok, test.want, test.wantOK)
		}
	}
}

func TestPlaybackHardwareProbesUseExplicitQSVDeviceAndCorpus(t *testing.T) {
	pipeline := playbackhw.Request{
		Device:     playbackhw.DeviceContext{DevicePath: "/dev/dri/renderD128"},
		InputCodec: playbackhw.HEVC, OutputCodec: playbackhw.H264,
		InputBitDepth: 10, OutputBitDepth: 8,
		InputPixelFormat: playbackhw.YUV420P10LE, OutputPixelFormat: playbackhw.YUV420P,
	}
	probes, _, err := playbackHardwareProbes(playbackhw.QSV, pipeline, map[playbackhw.Codec]string{playbackhw.HEVC: "/vectors/hevc-main10.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, probe := range probes {
		joined += strings.Join(probe.Args, " ") + "\n"
	}
	for _, required := range []string{"vaapi=portico_vaapi:/dev/dri/renderD128", "qsv=portico_probe@portico_vaapi", "/vectors/hevc-main10.mkv", "h264_qsv"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("probe set does not bind %q:\n%s", required, joined)
		}
	}
}
