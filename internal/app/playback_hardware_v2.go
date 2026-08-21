package app

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/ffmpegprobe"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
)

const playbackHardwareProbeRevision = "w5-hardware-runtime-v1"

// playbackHardwareRuntimeRequest contains the exact external identities and
// media characteristics which make hardware execution safe. Device and driver
// identity are deliberately caller-owned: a backend listing or encoder name is
// not evidence that a device can execute Portico's graph.
type playbackHardwareRuntimeRequest struct {
	Settings          transcodeSettings
	FFmpegPath        string
	BinaryFingerprint string
	FFmpegBuild       string
	FFmpegConfigure   string
	OS                playbackhw.OS
	Arch              string
	Vendor            playbackhw.DeviceVendor
	DeviceIdentity    string
	DevicePath        string
	DriverIdentity    string
	DriverVersion     string
	ProbeInputs       map[playbackhw.Codec]string
	Pipeline          playbackhw.Request
}

// playbackHardwareRuntime is process-scoped. Its executable-probe cache is
// keyed by FFmpeg, driver, device, platform and probe-definition identity by
// ffmpegprobe.Runner; changing any of those values forces a new execution.
type playbackHardwareRuntime struct {
	executor ffmpegprobe.Executor
	cache    ffmpegprobe.Cache
	clock    ffmpegprobe.Clock
	timeout  time.Duration
	ttl      time.Duration
}

type playbackHardwareRuntimeResult struct {
	Plan         playbackhw.Plan
	Evidence     *playbackhw.VerifiedEvidence
	Backend      playbackhw.Backend
	SoftwareOnly bool
	Reason       string
	Report       ffmpegprobe.Report
}

func newPlaybackHardwareRuntime(executor ffmpegprobe.Executor) *playbackHardwareRuntime {
	return &playbackHardwareRuntime{
		executor: executor,
		cache:    ffmpegprobe.NewMemoryCache(32),
		timeout:  15 * time.Second,
		ttl:      24 * time.Hour,
	}
}

// Resolve returns a hardware plan only after every operation used by that plan
// has succeeded in an executable, bounded probe against the exact device. Every
// incomplete, ambiguous, unavailable or failed probe falls back to software.
func (r *playbackHardwareRuntime) Resolve(ctx context.Context, req playbackHardwareRuntimeRequest) playbackHardwareRuntimeResult {
	software := func(reason string, report ffmpegprobe.Report) playbackHardwareRuntimeResult {
		return playbackHardwareRuntimeResult{SoftwareOnly: true, Reason: reason, Report: report}
	}
	if !req.Settings.HardwareAcceleration && !req.Settings.HardwareEncoding {
		return software("hardware execution is disabled", ffmpegprobe.Report{})
	}
	if !req.Settings.HardwareEncoding {
		return software("hardware encoding is disabled", ffmpegprobe.Report{})
	}
	backend, ok := configuredPlaybackHardwareBackend(req.Settings.HardwareDevice, req.OS, req.Vendor)
	if !ok {
		return software("configured hardware backend does not match this platform and device", ffmpegprobe.Report{})
	}
	if strings.TrimSpace(req.FFmpegPath) == "" {
		return software("FFmpeg is unavailable", ffmpegprobe.Report{})
	}
	identity := ffmpegprobe.Identity{
		BinaryFingerprint: strings.TrimSpace(req.BinaryFingerprint),
		FFmpegBuild:       strings.TrimSpace(req.FFmpegBuild),
		FFmpegConfigure:   strings.TrimSpace(req.FFmpegConfigure),
		Backend:           string(backend),
		DeviceIdentity:    strings.TrimSpace(req.DeviceIdentity),
		DevicePath:        normalizedHardwareDevicePath(backend, req.DevicePath),
		DriverIdentity:    strings.TrimSpace(req.DriverIdentity),
		DriverVersion:     strings.TrimSpace(req.DriverVersion),
		OS:                string(req.OS),
		Arch:              firstNonEmpty(strings.TrimSpace(req.Arch), runtime.GOARCH),
		ConfigRevision:    playbackHardwareProbeRevision,
	}
	if err := identity.Validate(); err != nil {
		return software("hardware identity is incomplete", ffmpegprobe.Report{})
	}

	pipeline := req.Pipeline
	pipeline.EncodeOnly = !req.Settings.HardwareAcceleration || (pipeline.InputCodec == playbackhw.HEVC && !req.Settings.HardwareDecodeHEVC)
	pipeline.Backend = backend
	pipeline.OS = req.OS
	pipeline.Vendor = req.Vendor
	pipeline.Availability = playbackhw.AvailabilityAvailable
	pipeline.Device = playbackhw.DeviceContext{
		Identity:          identity.DeviceIdentity,
		DevicePath:        identity.DevicePath,
		BinaryFingerprint: identity.BinaryFingerprint,
		ProbeRevision:     identity.ConfigRevision,
	}
	probes, spec, err := playbackHardwareProbes(backend, pipeline, req.ProbeInputs)
	if err != nil {
		return software(err.Error(), ffmpegprobe.Report{})
	}
	runner := ffmpegprobe.Runner{
		Binary:       req.FFmpegPath,
		Executor:     r.executor,
		Cache:        r.cache,
		Clock:        r.clock,
		Timeout:      r.timeout,
		TTL:          r.ttl,
		TransientTTL: 30 * time.Second,
	}
	report, err := runner.Probe(ctx, identity, probes)
	if err != nil {
		return software("hardware probe could not run", report)
	}
	evidence, ok := verifiedPlaybackHardwareEvidence(report, backend, req.OS, req.Vendor, pipeline.Device, spec)
	if !ok {
		return software("hardware graph was not completely verified", report)
	}
	pipeline.Evidence = evidence
	plan, err := playbackhw.PlanPipeline(pipeline)
	if err != nil {
		return software("verified hardware graph cannot satisfy this playback plan", report)
	}
	return playbackHardwareRuntimeResult{Plan: plan, Evidence: evidence, Backend: backend, Report: report}
}

func configuredPlaybackHardwareBackend(value string, os playbackhw.OS, vendor playbackhw.DeviceVendor) (playbackhw.Backend, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "nvenc" || normalized == "cuda" {
		normalized = "nvidia"
	}
	candidates := []playbackhw.Backend{}
	if normalized == "" || normalized == "auto" {
		switch os {
		case playbackhw.Darwin:
			candidates = []playbackhw.Backend{playbackhw.VideoToolbox}
		case playbackhw.Linux:
			if vendor == playbackhw.Nvidia {
				candidates = []playbackhw.Backend{playbackhw.NVIDIA}
			} else if vendor == playbackhw.Intel {
				candidates = []playbackhw.Backend{playbackhw.QSV, playbackhw.VAAPI}
			} else {
				candidates = []playbackhw.Backend{playbackhw.VAAPI}
			}
		case playbackhw.Windows:
			switch vendor {
			case playbackhw.Intel:
				candidates = []playbackhw.Backend{playbackhw.QSV}
			case playbackhw.Nvidia:
				candidates = []playbackhw.Backend{playbackhw.NVIDIA}
			case playbackhw.AMD:
				candidates = []playbackhw.Backend{playbackhw.AMF}
			}
		}
	} else {
		candidates = []playbackhw.Backend{playbackhw.Backend(normalized)}
	}
	for _, backend := range candidates {
		if _, found := playbackhw.Find(backend, os, vendor); found {
			return backend, true
		}
	}
	return "", false
}

func normalizedHardwareDevicePath(backend playbackhw.Backend, value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	// VideoToolbox and AMF do not accept an FFmpeg device selector, but an exact
	// identity field is still required so evidence cannot float between devices.
	if backend == playbackhw.VideoToolbox || backend == playbackhw.AMF {
		return "platform-default"
	}
	return ""
}

type playbackHardwareProbeSpec struct {
	decode         playbackhw.Codec
	encode         playbackhw.Codec
	bitDepths      []int
	pixelFormats   []playbackhw.PixelFormat
	hardwareStages []playbackhw.Operation
	softwareStages []playbackhw.Operation
	crossovers     []playbackhw.Operation
}

func verifiedPlaybackHardwareEvidence(report ffmpegprobe.Report, backend playbackhw.Backend, os playbackhw.OS, vendor playbackhw.DeviceVendor, device playbackhw.DeviceContext, spec playbackHardwareProbeSpec) (*playbackhw.VerifiedEvidence, bool) {
	if len(report.Results) == 0 {
		return nil, false
	}
	for _, result := range report.Results {
		if result.Status != ffmpegprobe.Supported {
			return nil, false
		}
	}
	e := &playbackhw.VerifiedEvidence{
		Complete: true, Executable: true, Backend: backend, OS: os, Vendor: vendor,
		DeviceIdentity: device.Identity, DevicePath: device.DevicePath,
		BinaryFingerprint: device.BinaryFingerprint, ProbeRevision: device.ProbeRevision,
		Encode: []playbackhw.Codec{spec.encode}, BitDepths: append([]int(nil), spec.bitDepths...),
		PixelFormats:    append([]playbackhw.PixelFormat(nil), spec.pixelFormats...),
		HardwareStages:  append([]playbackhw.Operation(nil), spec.hardwareStages...),
		SoftwareStages:  append([]playbackhw.Operation(nil), spec.softwareStages...),
		CrossoverStages: append([]playbackhw.Operation(nil), spec.crossovers...),
	}
	if spec.decode != "" {
		e.Decode = []playbackhw.Codec{spec.decode}
	}
	return e, true
}

func uniqueHardwareOperations(values []playbackhw.Operation) []playbackhw.Operation {
	seen := map[playbackhw.Operation]bool{}
	out := make([]playbackhw.Operation, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func hardwareProbeFailure(name string) error {
	return fmt.Errorf("hardware probe input is unavailable for %s", name)
}
