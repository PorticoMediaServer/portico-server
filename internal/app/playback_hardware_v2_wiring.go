package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

// resolvePlaybackHardwareRoute turns exact host and media evidence into both
// the public, path-free route and the private executable plan. It deliberately
// returns no route whenever any identity input cannot be established.
func (s *Server) resolvePlaybackHardwareRoute(ctx context.Context, settings transcodeSettings, facts mediafacts.Facts, software playbackplan.Plan, sourceURL string) (playbackplan.HardwareRoute, *playbackhw.Plan) {
	if software.Mode != playbackplan.VideoTranscode || s.playbackHardwareRuntime == nil || software.Subtitle.Action == playbackplan.BurnIn {
		return playbackplan.HardwareRoute{}, nil
	}
	if software.Color != nil && software.Color.Action != "preserve" {
		// Current backend plans do not yet seal an algorithm-specific color
		// transform. Prefer the exact software graph to silently ignoring the
		// owner's tone-map choice or mishandling dynamic metadata.
		return playbackplan.HardwareRoute{}, nil
	}
	video, outputCodec, ok := playbackHardwareVideoTuple(facts, software)
	if !ok || video.Rotation != 0 || video.SampleAspectRatio.Num != video.SampleAspectRatio.Den {
		return playbackplan.HardwareRoute{}, nil
	}
	sourcePath, ok := localPathFromSourceURL(sourceURL)
	if !ok || !filepath.IsAbs(sourcePath) {
		return playbackplan.HardwareRoute{}, nil
	}
	identity, ok := s.playbackHardwareHostIdentity(ctx, settings)
	if !ok {
		return playbackplan.HardwareRoute{}, nil
	}
	inputCodec := playbackHardwareCodec(video.Codec)
	output := playbackHardwareCodec(outputCodec)
	if inputCodec == "" || output == "" {
		return playbackplan.HardwareRoute{}, nil
	}
	outputDepth := playbackHardwareOutputBitDepth(video, output, software)
	// H.264 browser targets are the canonical 8-bit 4:2:0 delivery tuple. A
	// Main10 source is input evidence, not permission to silently emit High10.
	// Seal the requested output depth before probing so VideoToolbox selection
	// and the eventual compiler graph describe the same executable pipeline.
	inputDepth := video.BitDepth
	if inputDepth != 10 {
		inputDepth = 8
	}
	width, height := playbackHardwareOutputDimensions(video, software.Constraints)
	pipeline := playbackhw.Request{
		InputCodec: inputCodec, OutputCodec: output,
		InputBitDepth: inputDepth, OutputBitDepth: outputDepth,
		InputPixelFormat: playbackHardwarePixelFormat(inputDepth), OutputPixelFormat: playbackHardwarePixelFormat(outputDepth),
		HDRInput: video.DynamicRange() != mediafacts.DynamicRangeSDR, HDROutput: software.Color != nil && software.Color.Output != "sdr",
		Width: width, Height: height,
		Deinterlace: strings.TrimSpace(video.FieldOrder) != "" && !strings.EqualFold(video.FieldOrder, "progressive"),
		ToneMap:     software.Color != nil && software.Color.Action == "tone_map_sdr",
	}
	identity.Pipeline = pipeline
	identity.ProbeInputs = map[playbackhw.Codec]string{inputCodec: sourcePath}
	for _, candidate := range playbackHardwareConfiguredCandidates(settings.HardwareDevice, identity.OS, identity.Vendor) {
		identity.Settings = settings
		identity.Settings.HardwareDevice = candidate
		result := s.playbackHardwareRuntime.Resolve(ctx, identity)
		if result.SoftwareOnly {
			continue
		}
		plan := result.Plan
		plan.RuntimeIdentity = playbackhw.RuntimeIdentity{
			ExecutablePath: identity.FFmpegPath, BinaryFingerprint: identity.BinaryFingerprint,
			DeviceIdentity: identity.DeviceIdentity, DevicePath: identity.DevicePath,
			DriverIdentity: identity.DriverIdentity, DriverVersion: identity.DriverVersion,
		}
		return playbackHardwareRouteFromPlan(plan), &plan
	}
	return playbackplan.HardwareRoute{}, nil
}

func playbackHardwareOutputBitDepth(video mediafacts.Video, output playbackhw.Codec, plan playbackplan.Plan) int {
	depth := video.BitDepth
	if output == playbackhw.H264 || plan.Color != nil && plan.Color.Output == "sdr" || depth != 10 {
		return 8
	}
	return 10
}

func (s *Server) playbackHardwareExecutionIdentityMatches(ctx context.Context, plan *playbackhw.Plan) bool {
	if plan == nil || plan.Backend == "" {
		return false
	}
	want := plan.RuntimeIdentity
	if !filepath.IsAbs(want.ExecutablePath) || want.BinaryFingerprint == "" || want.DeviceIdentity == "" || want.DriverIdentity == "" || want.DriverVersion == "" {
		return false
	}
	settings := s.transcodeSettings()
	settings.HardwareDevice = string(plan.Backend)
	got, ok := s.playbackHardwareHostIdentity(ctx, settings)
	if !ok {
		return false
	}
	return filepath.Clean(got.FFmpegPath) == filepath.Clean(want.ExecutablePath) &&
		got.BinaryFingerprint == want.BinaryFingerprint && got.DeviceIdentity == want.DeviceIdentity &&
		got.DevicePath == want.DevicePath && got.DriverIdentity == want.DriverIdentity && got.DriverVersion == want.DriverVersion
}

func playbackHardwareConfiguredCandidates(configured string, os playbackhw.OS, vendor playbackhw.DeviceVendor) []string {
	configured = strings.ToLower(strings.TrimSpace(configured))
	if configured != "" && configured != "auto" {
		return []string{configured}
	}
	switch {
	case os == playbackhw.Darwin:
		return []string{"videotoolbox"}
	case os == playbackhw.Linux && vendor == playbackhw.Intel:
		return []string{"qsv", "vaapi"}
	case os == playbackhw.Linux && vendor == playbackhw.AMD:
		return []string{"vaapi"}
	case vendor == playbackhw.Nvidia:
		return []string{"nvidia"}
	case os == playbackhw.Windows && vendor == playbackhw.Intel:
		return []string{"qsv"}
	case os == playbackhw.Windows && vendor == playbackhw.AMD:
		return []string{"amf"}
	default:
		return []string{"auto"}
	}
}

func playbackHardwareVideoTuple(facts mediafacts.Facts, plan playbackplan.Plan) (mediafacts.Video, string, bool) {
	for _, action := range plan.Streams {
		if action.Kind != "video" || action.Action != playbackplan.Convert {
			continue
		}
		for _, video := range facts.Video {
			if video.Index == action.Index {
				return video, action.OutputCodec, true
			}
		}
	}
	return mediafacts.Video{}, "", false
}

func playbackHardwareCodec(value string) playbackhw.Codec {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "h264", "avc":
		return playbackhw.H264
	case "hevc", "h265":
		return playbackhw.HEVC
	case "av1":
		return playbackhw.AV1
	default:
		return ""
	}
}

func playbackHardwarePixelFormat(depth int) playbackhw.PixelFormat {
	if depth == 10 {
		return playbackhw.YUV420P10LE
	}
	return playbackhw.YUV420P
}

func playbackHardwareOutputDimensions(video mediafacts.Video, constraints playbackplan.Constraints) (int, int) {
	w, h := video.CodedWidth, video.CodedHeight
	changed := false
	if constraints.MaxWidth > 0 && w > constraints.MaxWidth {
		h = max(2, h*constraints.MaxWidth/w)
		w = constraints.MaxWidth
		changed = true
	}
	if constraints.MaxHeight > 0 && h > constraints.MaxHeight {
		w = max(2, w*constraints.MaxHeight/h)
		h = constraints.MaxHeight
		changed = true
	}
	if !changed {
		return 0, 0
	}
	// Hardware encoders require even 4:2:0 dimensions.
	return w &^ 1, h &^ 1
}

func playbackHardwareRouteFromPlan(plan playbackhw.Plan) playbackplan.HardwareRoute {
	route := playbackplan.HardwareRoute{Verified: true, Backend: plan.Backend}
	for _, stage := range plan.Stages {
		route.Stages = append(route.Stages, playbackplan.Stage{Kind: "hardware", Operation: string(stage.Operation), Execution: string(stage.Execution)})
	}
	return route
}

func (s *Server) playbackHardwareHostIdentity(ctx context.Context, settings transcodeSettings) (playbackHardwareRuntimeRequest, bool) {
	ffmpegPath := strings.TrimSpace(firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg"))
	resolved, err := exec.LookPath(ffmpegPath)
	if err != nil {
		if !filepath.IsAbs(ffmpegPath) {
			return playbackHardwareRuntimeRequest{}, false
		}
		resolved = ffmpegPath
	}
	fingerprint, ok := playbackHardwareFileFingerprint(resolved)
	if !ok {
		return playbackHardwareRuntimeRequest{}, false
	}
	versionOutput, ok := playbackHardwareCommand(ctx, resolved, "-hide_banner", "-version")
	if !ok {
		return playbackHardwareRuntimeRequest{}, false
	}
	version, configure := playbackHardwareVersionIdentity(versionOutput)
	if version == "" || configure == "" {
		return playbackHardwareRuntimeRequest{}, false
	}
	vendor, deviceID, devicePath, driverID, driverVersion, ok := s.playbackHardwareDeviceIdentity(ctx, settings)
	if !ok {
		return playbackHardwareRuntimeRequest{}, false
	}
	return playbackHardwareRuntimeRequest{
		FFmpegPath: resolved, BinaryFingerprint: fingerprint, FFmpegBuild: version, FFmpegConfigure: configure,
		OS: playbackhw.OS(runtime.GOOS), Arch: runtime.GOARCH, Vendor: vendor,
		DeviceIdentity: deviceID, DevicePath: devicePath, DriverIdentity: driverID, DriverVersion: driverVersion,
	}, true
}

func playbackHardwareFileFingerprint(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.CopyBuffer(h, f, make([]byte, 256*1024)); err != nil {
		return "", false
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), true
}

func playbackHardwareVersionIdentity(output string) (string, string) {
	var version, configure string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if version == "" && strings.HasPrefix(line, "ffmpeg version ") {
			version = line
		}
		if strings.HasPrefix(line, "configuration:") {
			sum := sha256.Sum256([]byte(line))
			configure = "sha256:" + hex.EncodeToString(sum[:])
		}
	}
	return version, configure
}

func (s *Server) playbackHardwareDeviceIdentity(ctx context.Context, settings transcodeSettings) (playbackhw.DeviceVendor, string, string, string, string, bool) {
	configured := strings.ToLower(strings.TrimSpace(settings.HardwareDevice))
	if runtime.GOOS == "darwin" {
		version, ok := playbackHardwareCommand(ctx, "sw_vers", "-productVersion")
		platform, platformOK := playbackHardwareCommand(ctx, "ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
		if !ok || !platformOK || strings.TrimSpace(platform) == "" {
			return "", "", "", "", "", false
		}
		stablePlatform, stable := playbackHardwareDarwinPlatformIdentity(platform)
		if !stable {
			return "", "", "", "", "", false
		}
		platformSum := sha256.Sum256([]byte(stablePlatform))
		device := "apple-platform:sha256:" + hex.EncodeToString(platformSum[:])
		return playbackhw.Apple, device, "platform-default", "videotoolbox-macos", strings.TrimSpace(version), true
	}
	if configured == "nvidia" || configured == "nvenc" || configured == "cuda" || (configured == "auto" && strings.Contains(strings.ToLower(s.gpuInfo.Provider), "nvidia")) {
		line, ok := playbackHardwareCommand(ctx, "nvidia-smi", "--query-gpu=uuid,index,driver_version", "--format=csv,noheader,nounits")
		lines := strings.Split(strings.TrimSpace(line), "\n")
		parts := strings.Split(lines[0], ",")
		// Without a configured selector there is no authority to choose among
		// multiple UUID/ordinal bindings. A single row proves the binding used.
		if !ok || len(lines) != 1 || len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
			return "", "", "", "", "", false
		}
		return playbackhw.Nvidia, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), "nvidia", strings.TrimSpace(parts[2]), true
	}
	// QSV/VAAPI require an exact render node. Its kernel driver and version are
	// read from sysfs, preventing evidence from floating between swapped GPUs.
	if runtime.GOOS == "linux" {
		path := "/dev/dri/renderD128"
		driverLink, err := filepath.EvalSymlinks("/sys/class/drm/renderD128/device/driver")
		kernelBytes, kernelErr := os.ReadFile("/proc/sys/kernel/osrelease")
		deviceBytes, factsErr := os.ReadFile("/sys/class/drm/renderD128/device/uevent")
		deviceLink, deviceErr := filepath.EvalSymlinks("/sys/class/drm/renderD128/device")
		if err != nil || kernelErr != nil || factsErr != nil || deviceErr != nil {
			return "", "", "", "", "", false
		}
		driver := filepath.Base(driverLink)
		vendor := playbackhw.Intel
		if driver == "amdgpu" {
			vendor = playbackhw.AMD
		}
		deviceSum := sha256.Sum256(deviceBytes)
		deviceID := filepath.Base(deviceLink) + ":sha256:" + hex.EncodeToString(deviceSum[:])
		return vendor, deviceID, path, driver, strings.TrimSpace(string(kernelBytes)), true
	}
	if runtime.GOOS == "windows" {
		// FFmpeg's QSV "auto" value is selection policy, not a device identity.
		// Until the configured adapter can be bound to an exact initialization
		// selector, conservatively decline Windows QSV.
		if configured == "qsv" {
			return "", "", "", "", "", false
		}
		output, ok := playbackHardwareCommand(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
			`Get-CimInstance Win32_VideoController | ForEach-Object { "$($_.PNPDeviceID)|$($_.DriverVersion)|$($_.Name)|$($_.AdapterCompatibility)" }`)
		if !ok {
			return "", "", "", "", "", false
		}
		var matchedVendor playbackhw.DeviceVendor
		var matchedID, matchedDriverVersion string
		matches := 0
		for _, line := range strings.Split(output, "\n") {
			parts := strings.Split(strings.TrimSpace(line), "|")
			if len(parts) != 4 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				continue
			}
			label := strings.ToLower(parts[2] + " " + parts[3])
			vendor := playbackhw.DeviceVendor("")
			switch {
			case strings.Contains(label, "amd") && (configured == "" || configured == "auto" || configured == "amf"):
				vendor = playbackhw.AMD
			default:
				continue
			}
			matches++
			matchedVendor, matchedID, matchedDriverVersion = vendor, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}
		if matches == 1 {
			return matchedVendor, matchedID, "", "windows-display-driver", matchedDriverVersion, true
		}
	}
	return "", "", "", "", "", false
}

func playbackHardwareDarwinPlatformIdentity(output string) (string, bool) {
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		for _, key := range []string{"IOPlatformUUID", "model"} {
			prefix := "\"" + key + "\" = "
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			value = strings.Trim(value, "<>\"")
			if value != "" {
				values[key] = value
			}
		}
	}
	if values["IOPlatformUUID"] == "" || values["model"] == "" {
		return "", false
	}
	return "uuid=" + strings.ToLower(values["IOPlatformUUID"]) + "\nmodel=" + strings.ToLower(values["model"]), true
}

func playbackHardwareCommand(parent context.Context, binary string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	return string(output), err == nil && ctx.Err() == nil
}
