package decoder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/mediaexec"
	"portico.local/server/internal/supervise"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HardwareBackend names an encoder family. Software is always available; every
// other backend must earn its place with a real bounded encode on this host.
type HardwareBackend string

const (
	BackendSoftware     HardwareBackend = "software"
	BackendVideoToolbox HardwareBackend = "videotoolbox"
	BackendVAAPI        HardwareBackend = "vaapi"
	BackendQSV          HardwareBackend = "qsv"
	BackendNVENC        HardwareBackend = "nvenc"
	BackendAMF          HardwareBackend = "amf"
)

// HardwareBackends is the probe order. The first backend that probes clean on a
// host wins automatic selection; an owner may still name one explicitly.
var HardwareBackends = []HardwareBackend{BackendVideoToolbox, BackendNVENC, BackendQSV, BackendVAAPI, BackendAMF}

// ValidBackend reports whether name is a backend this build understands. "auto"
// and "software" are configuration values, not probe targets.
func ValidBackend(name string) bool {
	if HardwareBackend(name) == BackendSoftware {
		return true
	}
	for _, b := range HardwareBackends {
		if HardwareBackend(name) == b {
			return true
		}
	}
	return false
}

// Probe outcome reason codes. They are published on diagnostics, so they are
// stable strings rather than free text.
const (
	ProbeAvailable           = "available"
	ProbeUnsupportedPlatform = "unsupported_platform"
	ProbeConfinementMissing  = "confinement_unavailable"
	ProbeEncodeFailed        = "probe_encode_failed"
	ProbeWarming             = "probe_warming"
	ProbeTimedOut            = "probe_timed_out"
	ProbeDisabled            = "probe_disabled"
	ProbeCircuitOpen         = "hardware_circuit_open"
)

// HardwareStage is one operation in the conversion graph and where it runs.
// Operations are decode, tone_map, scale and encode; execution is hardware or
// software. A hardware backend that cannot tone map in hardware still reports
// its encode stage as hardware.
type HardwareStage struct {
	Operation string `json:"operation"`
	Execution string `json:"execution"`
}

const (
	StageDecode  = "decode"
	StageToneMap = "tone_map"
	StageScale   = "scale"
	StageEncode  = "encode"
	// StageDeinterlace always runs on system frames; see BuildConversion.
	StageDeinterlace = "deinterlace"

	ExecutionHardware = "hardware"
	ExecutionSoftware = "software"
)

// HardwareProbe is the cached result of one bounded encode. NativeFilters says
// whether the backend's own tone-map/scale filters ran, not merely its encoder.
type HardwareProbe struct {
	Backend       HardwareBackend `json:"backend"`
	Available     bool            `json:"available"`
	Reason        string          `json:"reason"`
	Encoder       string          `json:"encoder,omitempty"`
	NativeFilters bool            `json:"nativeFilters"`
	Device        string          `json:"device,omitempty"`
	Identity      string          `json:"identity"`
	DurationMS    int64           `json:"durationMs"`
}

type backendSpec struct {
	encoder string
	// device builds the -init_hw_device argument pair, if the backend needs one.
	device func(device string) []string
	// upload is the filter prefix that moves frames into the backend's own
	// surface format. Empty when the encoder accepts ordinary system frames.
	upload string
	// toneMap is the backend's native tone-map filter, empty when it has none.
	toneMap func(algorithm string) string
	// scale is the backend's native scaler, empty when it has none.
	scale func(height int) string
	// platforms restricts a backend to the operating systems that can host it.
	platforms []string
}

func vaapiDevice(device string) []string {
	if device == "" {
		device = "/dev/dri/renderD128"
	}
	return []string{"-init_hw_device", "vaapi=hw:" + device, "-filter_hw_device", "hw"}
}

// qsvDevice differs by platform: Windows QSV binds through D3D11 and needs no
// selector, while Linux QSV is layered on a VAAPI render node.
func qsvDevice(device string) []string {
	if runtime.GOOS == "windows" {
		if device == "" {
			return []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"}
		}
		return []string{"-init_hw_device", "qsv=hw:" + device, "-filter_hw_device", "hw"}
	}
	if device == "" {
		device = "/dev/dri/renderD128"
	}
	return []string{"-init_hw_device", "vaapi=qsvva:" + device, "-init_hw_device", "qsv=hw@qsvva", "-filter_hw_device", "hw"}
}
func cudaDevice(device string) []string {
	if device == "" {
		device = "0"
	}
	return []string{"-init_hw_device", "cuda=hw:" + device, "-filter_hw_device", "hw"}
}

var backendSpecs = map[HardwareBackend]backendSpec{
	BackendVideoToolbox: {
		encoder:   "h264_videotoolbox",
		platforms: []string{"darwin"},
	},
	BackendVAAPI: {
		encoder:   "h264_vaapi",
		device:    vaapiDevice,
		upload:    "format=nv12,hwupload",
		toneMap:   func(string) string { return "tonemap_vaapi=format=nv12:matrix=bt709:primaries=bt709:transfer=bt709" },
		scale:     func(h int) string { return "scale_vaapi=w=-2:h=" + strconv.Itoa(h) },
		platforms: []string{"linux"},
	},
	BackendQSV: {
		encoder:   "h264_qsv",
		device:    qsvDevice,
		upload:    "format=nv12,hwupload=extra_hw_frames=32",
		toneMap:   func(string) string { return "vpp_qsv=tonemap=1:format=nv12" },
		scale:     func(h int) string { return "vpp_qsv=w=-1:h=" + strconv.Itoa(h) },
		platforms: []string{"linux", "windows"},
	},
	BackendNVENC: {
		encoder:   "h264_nvenc",
		device:    cudaDevice,
		upload:    "format=yuv420p,hwupload_cuda",
		toneMap:   func(a string) string { return "tonemap_cuda=tonemap=" + a + ":desat=0:format=yuv420p" },
		scale:     func(h int) string { return "scale_cuda=-2:" + strconv.Itoa(h) },
		platforms: []string{"linux", "windows"},
	},
	BackendAMF: {
		encoder:   "h264_amf",
		platforms: []string{"windows"},
	},
}

func (s backendSpec) supported() bool {
	if len(s.platforms) == 0 {
		return true
	}
	for _, p := range s.platforms {
		if p == runtime.GOOS {
			return true
		}
	}
	return false
}

// ToneMapAlgorithms are the algorithms the software tone mapper accepts. The
// default is hable; an unknown value is refused rather than silently replaced.
var ToneMapAlgorithms = []string{"clip", "linear", "gamma", "reinhard", "hable", "mobius"}

const DefaultToneMapAlgorithm = "hable"

func ValidToneMapAlgorithm(a string) bool {
	for _, v := range ToneMapAlgorithms {
		if v == a {
			return true
		}
	}
	return false
}

// SoftwareToneMap is the software HDR to SDR graph. It is also the fallback for
// a hardware backend whose own tone mapper did not probe clean.
func SoftwareToneMap(algorithm string) string {
	if !ValidToneMapAlgorithm(algorithm) {
		algorithm = DefaultToneMapAlgorithm
	}
	return "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=" + algorithm + ":desat=0,zscale=t=bt709:m=bt709:r=tv,format=yuv420p"
}

// HardwareDetector probes backends once per ffmpeg identity and caches the
// verdict. The cache key is the ffmpeg binary identity plus backend and device,
// so replacing ffmpeg re-probes without a restart.
type HardwareDetector struct {
	// Timeout bounds one probe encode. Zero uses the default.
	Timeout time.Duration
	// Libraries are the confinement's readable dependency files, as resolved by
	// ResolveLibraries for the rest of this package.
	Libraries []string
	// Run is the process seam. Tests substitute it; production runs the confined
	// child directly, because a synthetic clip borrows no source custody.
	Run func(ctx context.Context, cmd *exec.Cmd) error

	mu      sync.Mutex
	cache   map[string]HardwareProbe
	warming map[string]bool
}

const defaultProbeTimeout = 20 * time.Second

// ffmpegIdentity hashes what actually decides capability: the resolved binary
// path, its size and its modification time. Cheap, and it changes on upgrade.
func ffmpegIdentity(executable string) (string, error) {
	if !filepath.IsAbs(executable) || filepath.Clean(executable) != executable {
		return "", ErrInvalidConfiguration
	}
	info, err := os.Stat(executable)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(executable + "\x00" + strconv.FormatInt(info.Size(), 10) + "\x00" + strconv.FormatInt(info.ModTime().UnixNano(), 10)))
	return hex.EncodeToString(sum[:16]), nil
}

// probeCommand runs a hardware probe exactly as a hardware conversion runs:
// through mediaexec with the GPU devices and driver libraries a hardware job
// gets, sandboxed where the platform can. A sandbox that breaks a backend makes
// the probe fail, so the backend is reported unavailable and conversions use
// software, instead of every hardware session failing. The probe's input is a
// synthetic lavfi clip and its output the null muxer.
func probeCommand(ctx context.Context, executable string, args []string, libraries []string) (*exec.Cmd, error) {
	if !filepath.IsAbs(executable) || filepath.Clean(executable) != executable {
		return nil, ErrInvalidConfiguration
	}
	return mediaexec.CommandContext(ctx, mediaexec.Job{Executable: executable, Args: args, Hardware: true, Libraries: libraries})
}

// ReshapesDolbyVision says whether converting this request tone maps with
// libplacebo, the one FFmpeg tone mapper that applies a Dolby Vision stream's
// reshaping metadata (its apply_dolbyvision option is on by default). Profile 5
// files have no HDR10 or SDR base layer, so every other tone mapper treats
// their IPT-PQ-C2 picture as HDR10 and shows the well-known purple and green
// cast (COMPAT-04). It mirrors BuildConversion's choice of tone mapper; a test
// holds the two together.
func ReshapesDolbyVision(r ConversionRequest) bool {
	if !r.ConvertVideo || !r.ToneMap || r.Toolchain == nil || !r.Toolchain.VulkanAvailable {
		return false
	}
	backend, spec := BackendSoftware, backendSpec{}
	if r.Probe.Available && r.Probe.Backend != BackendSoftware {
		if s, ok := backendSpecs[r.Probe.Backend]; ok && s.supported() {
			backend, spec = r.Probe.Backend, s
		}
	}
	native := r.BurnIn == nil && backend != BackendSoftware && r.Probe.NativeFilters && spec.toneMap != nil && spec.scale != nil
	filter := map[HardwareBackend]string{BackendNVENC: "tonemap_cuda", BackendVAAPI: "tonemap_vaapi", BackendQSV: "vpp_qsv"}[backend]
	native = native && r.Toolchain.Filters[filter]
	return !native && spec.upload == ""
}

func probeArgs(spec backendSpec, device string, filters string) []string {
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}
	if spec.device != nil {
		args = append(args, spec.device(device)...)
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=160x120:rate=10:duration=0.5", "-frames:v", "5")
	chain := filters
	if spec.upload != "" {
		if chain != "" {
			chain += ","
		}
		chain += spec.upload
	}
	if chain != "" {
		args = append(args, "-vf", chain)
	}
	args = append(args, "-c:v", spec.encoder, "-b:v", "200k", "-an", "-sn", "-dn", "-f", "null", "-")
	return args
}

func (d *HardwareDetector) run(ctx context.Context, executable string, args []string) error {
	cmd, err := probeCommand(ctx, executable, args, d.Libraries)
	if err != nil {
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	if d.Run != nil {
		return d.Run(ctx, cmd)
	}
	return cmd.Run()
}

// Probe returns this host's verdict for one backend, running at most one bounded
// encode per ffmpeg identity. A failed probe is cached too: a host without the
// hardware must not pay for a subprocess on every session.
func (d *HardwareDetector) Probe(ctx context.Context, executable string, backend HardwareBackend, device string) HardwareProbe {
	out := HardwareProbe{Backend: backend, Reason: ProbeUnsupportedPlatform, Device: device}
	spec, ok := backendSpecs[backend]
	if !ok || backend == BackendSoftware {
		out.Reason = ProbeDisabled
		return out
	}
	identity, err := ffmpegIdentity(executable)
	if err != nil {
		out.Reason = ProbeEncodeFailed
		return out
	}
	out.Identity, out.Encoder = identity, spec.encoder
	if !spec.supported() {
		return out
	}
	if hardwareCircuitOpen(backend) {
		out.Reason = ProbeCircuitOpen
		return out
	}
	recovery := hardwareNeedsProbe(backend)
	key := identity + "/" + string(backend) + "/" + device
	d.mu.Lock()
	if cached, ok := d.cache[key]; ok && !recovery {
		d.mu.Unlock()
		return cached
	}
	d.mu.Unlock()

	timeout := d.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	started := time.Now()
	bounded, cancel := context.WithTimeout(ctx, timeout)
	err = d.run(bounded, executable, probeArgs(spec, device, ""))
	cancel()
	out.DurationMS = time.Since(started).Milliseconds()
	switch {
	case err == nil:
		out.Available, out.Reason = true, ProbeAvailable
	case errors.Is(err, ErrInvalidConfiguration), errors.Is(err, mediaexec.ErrSandboxRequired):
		out.Reason = ProbeConfinementMissing
	case errors.Is(err, context.DeadlineExceeded):
		out.Reason = ProbeTimedOut
	default:
		out.Reason = ProbeEncodeFailed
	}
	// A backend whose encoder works may still lack its own filters in this
	// build. Confirm them with the real production chain rather than assume.
	if out.Available && spec.toneMap != nil && spec.scale != nil {
		chain := spec.toneMap(DefaultToneMapAlgorithm) + "," + spec.scale(120)
		bounded, cancel = context.WithTimeout(ctx, timeout)
		out.NativeFilters = d.run(bounded, executable, probeArgs(spec, device, chain)) == nil
		cancel()
	}
	d.mu.Lock()
	if d.cache == nil {
		d.cache = map[string]HardwareProbe{}
	}
	d.cache[key] = out
	if out.Available {
		clearHardwareCircuit(backend)
	} else if recovery {
		// Retry after the same cooldown, not on every new playback request.
		for range 3 {
			RecordHardwareFailure(backend)
		}
	}
	d.mu.Unlock()
	return out
}

// SelectCached answers from cached verdicts only. A playback request must never
// wait on a probe encode, so a cold cache yields software for this session and
// starts the real probe in the background; the next session sees the verdict.
func (d *HardwareDetector) SelectCached(executable, configured, device string) HardwareProbe {
	if configured == "" {
		configured = "auto"
	}
	if configured == string(BackendSoftware) {
		return HardwareProbe{Backend: BackendSoftware, Available: true, Reason: ProbeDisabled}
	}
	identity, err := ffmpegIdentity(executable)
	if err != nil {
		return HardwareProbe{Backend: BackendSoftware, Available: true, Reason: ProbeEncodeFailed}
	}
	candidates := HardwareBackends
	if configured != "auto" {
		if !ValidBackend(configured) {
			return HardwareProbe{Backend: BackendSoftware, Available: true, Reason: ProbeDisabled}
		}
		candidates = []HardwareBackend{HardwareBackend(configured)}
	}
	cold := false
	d.mu.Lock()
	for _, backend := range candidates {
		spec, ok := backendSpecs[backend]
		if !ok || !spec.supported() {
			continue
		}
		if hardwareCircuitOpen(backend) {
			continue
		}
		cached, ok := d.cache[identity+"/"+string(backend)+"/"+device]
		if !ok || hardwareNeedsProbe(backend) {
			cold = true
			break
		}
		if cached.Available && !hardwareCircuitOpen(backend) {
			d.mu.Unlock()
			return cached
		}
	}
	d.mu.Unlock()
	if cold {
		d.warm(executable, configured, device)
		return HardwareProbe{Backend: BackendSoftware, Available: true, Reason: ProbeWarming, Identity: identity}
	}
	return HardwareProbe{Backend: BackendSoftware, Available: true, Reason: ProbeEncodeFailed, Identity: identity}
}

// warm runs one background Select per (identity, configuration, device) so a
// cold cache is filled exactly once, off every request path.
func (d *HardwareDetector) warm(executable, configured, device string) {
	key := executable + "/" + configured + "/" + device
	d.mu.Lock()
	if d.warming == nil {
		d.warming = map[string]bool{}
	}
	if d.warming[key] {
		d.mu.Unlock()
		return
	}
	d.warming[key] = true
	d.mu.Unlock()
	supervise.Go("decoder.hardware.warm", func() {
		defer func() { d.mu.Lock(); delete(d.warming, key); d.mu.Unlock() }()
		d.Select(context.Background(), executable, configured, device)
	})
}

// Select resolves the owner's configured backend. "auto" probes in order and
// takes the first that works; a named backend that fails its probe falls back to
// software rather than failing the session.
func (d *HardwareDetector) Select(ctx context.Context, executable, configured, device string) HardwareProbe {
	if configured == "" {
		configured = "auto"
	}
	if configured == string(BackendSoftware) {
		return HardwareProbe{Backend: BackendSoftware, Available: true, Reason: ProbeDisabled}
	}
	if configured != "auto" {
		if !ValidBackend(configured) {
			return HardwareProbe{Backend: BackendSoftware, Available: true, Reason: ProbeDisabled}
		}
		probe := d.Probe(ctx, executable, HardwareBackend(configured), device)
		if probe.Available && !hardwareCircuitOpen(probe.Backend) {
			return probe
		}
		fallback := HardwareProbe{Backend: BackendSoftware, Available: true, Reason: probe.Reason, Identity: probe.Identity}
		return fallback
	}
	for _, backend := range HardwareBackends {
		if probe := d.Probe(ctx, executable, backend, device); probe.Available {
			return probe
		}
	}
	identity, _ := ffmpegIdentity(executable)
	return HardwareProbe{Backend: BackendSoftware, Available: true, Reason: ProbeEncodeFailed, Identity: identity}
}

// Report lists every backend's verdict for the owner capacity read. Results are
// ordered by backend name so the published report is stable.
func (d *HardwareDetector) Report(ctx context.Context, executable, device string) []HardwareProbe {
	out := []HardwareProbe{{Backend: BackendSoftware, Available: true, Reason: ProbeAvailable, Encoder: "libx264"}}
	for _, backend := range HardwareBackends {
		out = append(out, d.Probe(ctx, executable, backend, device))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Backend < out[j].Backend })
	return out
}

// ConversionRequest describes one video output rung. It is deliberately free of
// session identity: the same request always produces the same argument graph.
type ConversionRequest struct {
	BurnIn           *BurnIn
	Toolchain        *ToolchainFacts
	AudioCodec       string
	Probe            HardwareProbe
	ConvertVideo     bool
	CopyVideo        bool
	ToneMap          bool
	ToneMapAlgorithm string
	MaxHeight        int
	MaxWidth         int
	VideoBitrateBPS  int
	AudioBitrateBPS  int
	AudioChannels    int
	ConvertAudio     bool
	CopyAudio        bool
	SoftwarePreset   string
	KeyframeSeconds  int
	// Deinterlace inserts a deinterlacer ahead of every other picture filter.
	Deinterlace bool
	// MaxFrameRate caps the output rate; zero leaves the source rate alone.
	MaxFrameRate float64
	// Downmix replaces ffmpeg's implicit channel reduction with a fixed matrix,
	// make-up gain and a limiter, so the result does not depend on which sample
	// format the source decoder happened to produce.
	Downmix bool
}

// ConversionGraph is the argument contribution of one conversion. Input arguments
// precede -i; Video and Audio follow it. Stages document what actually ran where.
type ConversionGraph struct {
	ExtraInput []string        `json:"-"`
	VideoMap   string          `json:"-"`
	Input      []string        `json:"-"`
	Video      []string        `json:"-"`
	Audio      []string        `json:"-"`
	Stages     []HardwareStage `json:"stages"`
	Backend    HardwareBackend `json:"backend"`
}

// SoftwarePresets are the x264 presets an owner may choose.
var SoftwarePresets = []string{"ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower"}

const DefaultSoftwarePreset = "veryfast"

func ValidSoftwarePreset(p string) bool {
	for _, v := range SoftwarePresets {
		if v == p {
			return true
		}
	}
	return false
}

// BuildConversion turns a rung into ffmpeg arguments. A hardware backend that
// probed without its own filters keeps its encoder and tone maps in software,
// downloading frames only where the graph actually needs it.
func BuildConversion(r ConversionRequest) (ConversionGraph, error) {
	out := ConversionGraph{Backend: BackendSoftware, Stages: []HardwareStage{}}
	if r.CopyVideo && r.ConvertVideo || r.CopyAudio && r.ConvertAudio {
		return out, ErrInvalidConfiguration
	}
	if math.IsNaN(r.MaxFrameRate) || math.IsInf(r.MaxFrameRate, 0) || r.MaxFrameRate < 0 || r.MaxFrameRate > 240 {
		return out, ErrInvalidConfiguration
	}
	if r.MaxWidth < 0 || r.MaxWidth > 32768 || r.MaxHeight < 0 || r.MaxHeight > 4320 || r.VideoBitrateBPS < 0 || r.VideoBitrateBPS > 200_000_000 || r.AudioBitrateBPS < 0 || r.AudioBitrateBPS > 4_096_000 || r.AudioChannels < 0 || r.AudioChannels > 8 {
		return out, ErrInvalidConfiguration
	}
	if (r.ToneMap || r.BurnIn != nil) && !r.ConvertVideo {
		return out, ErrInvalidConfiguration
	}
	algorithm := r.ToneMapAlgorithm
	if algorithm == "" {
		algorithm = DefaultToneMapAlgorithm
	}
	if !ValidToneMapAlgorithm(algorithm) {
		return out, ErrInvalidConfiguration
	}
	preset := r.SoftwarePreset
	if preset == "" {
		preset = DefaultSoftwarePreset
	}
	if !ValidSoftwarePreset(preset) {
		return out, ErrInvalidConfiguration
	}
	keyframes := r.KeyframeSeconds
	if keyframes <= 0 {
		keyframes = 6
	}
	backend := BackendSoftware
	spec := backendSpec{}
	if r.Probe.Available && r.Probe.Backend != BackendSoftware {
		if s, ok := backendSpecs[r.Probe.Backend]; ok && s.supported() {
			backend, spec = r.Probe.Backend, s
		}
	}
	out.Backend = backend
	if r.CopyVideo {
		out.Video = append(out.Video, "-c:v", "copy")
		out.Stages = append(out.Stages, HardwareStage{StageDecode, ExecutionSoftware}, HardwareStage{StageEncode, ExecutionSoftware})
	}
	if r.ConvertVideo {
		native := r.BurnIn == nil && backend != BackendSoftware && r.Probe.NativeFilters && spec.toneMap != nil && spec.scale != nil
		if r.Toolchain != nil && r.ToneMap {
			filter := map[HardwareBackend]string{BackendNVENC: "tonemap_cuda", BackendVAAPI: "tonemap_vaapi", BackendQSV: "vpp_qsv"}[backend]
			native = native && r.Toolchain.Filters[filter]
		}
		if r.ToneMap && !native && r.Toolchain != nil && (!r.Toolchain.Filters["zscale"] || !r.Toolchain.Filters["tonemap"]) && ((!r.Toolchain.VulkanAvailable && !r.Toolchain.OpenCLAvailable) || spec.upload != "") {
			return out, errors.New("hdr_tone_mapping_filter_unavailable")
		}
		decode := ExecutionSoftware
		if backend != BackendSoftware && spec.device != nil {
			out.Input = append(out.Input, spec.device(r.Probe.Device)...)
			// Device initialization alone is software decode.
		} else if backend == BackendVideoToolbox && r.BurnIn == nil && !r.ToneMap && !r.Deinterlace && r.MaxFrameRate == 0 && r.MaxHeight == 0 && r.MaxWidth == 0 {
			out.Input = append(out.Input, "-hwaccel", "videotoolbox")
			decode = ExecutionHardware
		}
		if native && !r.Deinterlace && r.MaxFrameRate == 0 {
			switch backend {
			case BackendNVENC:
				out.Input = append(out.Input, "-hwaccel", "cuda", "-hwaccel_output_format", "cuda")
				decode = ExecutionHardware
			case BackendVAAPI:
				out.Input = append(out.Input, "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi", "-hwaccel_device", "hw")
				decode = ExecutionHardware
			case BackendQSV:
				out.Input = append(out.Input, "-hwaccel", "qsv", "-hwaccel_output_format", "qsv")
				decode = ExecutionHardware
			}
		}
		filters := []string{}
		// Deinterlacing and the frame-rate ceiling run on system frames, ahead of
		// any upload, so they are the same graph on every backend. Only frames the
		// decoder flags as interlaced are touched: mixed content stays sharp.
		if r.Deinterlace {
			filter := "yadif"
			if r.Toolchain != nil {
				if r.Toolchain.Filters["bwdif"] {
					filter = "bwdif"
				} else if !r.Toolchain.Filters["yadif"] {
					return out, errors.New("deinterlace_filter_unavailable")
				}
			}
			filters = append(filters, filter+"=mode=send_frame:parity=auto:deint=interlaced")
		}
		if r.MaxFrameRate > 0 {
			filters = append(filters, "fps=fps="+strconv.FormatFloat(r.MaxFrameRate, 'f', -1, 64))
		}
		pre := len(filters)
		toneExecution, scaleExecution := ExecutionSoftware, ExecutionSoftware
		if native {
			if r.ToneMap {
				filters = append(filters, spec.toneMap(algorithm))
				toneExecution = ExecutionHardware
			}
			if r.MaxHeight > 0 || r.MaxWidth > 0 {
				filters = append(filters, boundedScaleFilter(backend, r.MaxWidth, r.MaxHeight))
				scaleExecution = ExecutionHardware
			}
			if spec.upload != "" && decode != ExecutionHardware {
				// The upload sits between the system-frame filters and the
				// backend's own, wherever either list happens to be empty.
				upload := spec.upload
				if r.ToneMap {
					upload = strings.NewReplacer("format=nv12", "format=p010", "format=yuv420p", "format=p010").Replace(upload)
				}
				hardware := append([]string{upload}, filters[pre:]...)
				filters = append(filters[:pre:pre], hardware...)
			}
			switch backend {
			case BackendNVENC:
				filters = append(filters, "scale_cuda=format=nv12")
			case BackendVAAPI:
				filters = append(filters, "scale_vaapi=format=nv12")
			case BackendQSV:
				filters = append(filters, "vpp_qsv=format=nv12")
			}
		} else {
			if r.ToneMap {
				if r.Toolchain != nil && r.Toolchain.VulkanAvailable && spec.upload == "" {
					out.Input = append(out.Input, "-init_hw_device", "vulkan=tone", "-filter_hw_device", "tone")
					filters = append(filters, "format=yuv420p10le,hwupload,libplacebo=tonemapping=bt.2390:colorspace=bt709:color_primaries=bt709:color_trc=bt709:format=yuv420p,hwdownload,format=yuv420p")
					toneExecution = ExecutionHardware
				} else if r.Toolchain != nil && r.Toolchain.OpenCLAvailable && spec.upload == "" {
					out.Input = append(out.Input, "-init_hw_device", "opencl=tone", "-filter_hw_device", "tone")
					filters = append(filters, "format=p010,hwupload,tonemap_opencl=tonemap=hable:desat=0:format=nv12,hwdownload,format=nv12")
					toneExecution = ExecutionHardware
				} else {
					filters = append(filters, SoftwareToneMap(algorithm))
				}
			}
			if r.BurnIn != nil {
				burn, extra, err := burnFilter(*r.BurnIn, r.Toolchain)
				if err != nil {
					return out, err
				}
				filters = append(filters, burn)
				out.ExtraInput = extra
				if len(extra) > 0 {
					out.VideoMap = "[burn_output]"
				}
				out.Stages = append(out.Stages, HardwareStage{"subtitle_burn_in", ExecutionSoftware})
			}
			if r.MaxHeight > 0 || r.MaxWidth > 0 {
				filters = append(filters, boundedScaleFilter(BackendSoftware, r.MaxWidth, r.MaxHeight))
			}
			if spec.upload != "" {
				filters = append(filters, "format=nv12", spec.upload)
			}
		}
		if len(filters) > 0 {
			if out.VideoMap != "" {
				out.Video = append(out.Video, "-filter_complex", "[0:v:0]"+strings.Join(filters, ",")+"[burn_output]")
			} else {
				out.Video = append(out.Video, "-vf", strings.Join(filters, ","))
			}
		}
		out.Video = append(out.Video, "-c:v", encoderFor(backend, spec))
		if backend == BackendSoftware {
			out.Video = append(out.Video, "-preset", preset, "-pix_fmt", "yuv420p")
		}
		rate := r.VideoBitrateBPS
		if rate == 0 {
			rate = 8_000_000
		}
		out.Video = append(out.Video, "-b:v", strconv.Itoa(rate), "-maxrate", strconv.Itoa(rate), "-bufsize", strconv.Itoa(rate*2))
		out.Video = append(out.Video, "-force_key_frames", "expr:gte(t,n_forced*"+strconv.Itoa(keyframes)+")")
		encode := ExecutionSoftware
		if backend != BackendSoftware {
			encode = ExecutionHardware
		}
		out.Stages = append(out.Stages, HardwareStage{StageDecode, decode})
		if r.Deinterlace {
			out.Stages = append(out.Stages, HardwareStage{StageDeinterlace, ExecutionSoftware})
		}
		if r.ToneMap {
			out.Stages = append(out.Stages, HardwareStage{StageToneMap, toneExecution})
		}
		if r.MaxHeight > 0 || r.MaxWidth > 0 {
			out.Stages = append(out.Stages, HardwareStage{StageScale, scaleExecution})
		}
		out.Stages = append(out.Stages, HardwareStage{StageEncode, encode})
	}
	if r.CopyAudio {
		out.Audio = append(out.Audio, "-c:a", "copy")
	}
	if r.ConvertAudio {
		bitrate := r.AudioBitrateBPS
		if bitrate == 0 {
			bitrate = 192_000
		}
		channels := r.AudioChannels
		if channels == 0 {
			channels = 2
		}
		if r.Downmix {
			out.Audio = append(out.Audio, "-af", DownmixFilter(channels))
		}
		codec := r.AudioCodec
		if codec == "" {
			codec = "aac"
		}
		if codec != "aac" && codec != "ac3" && codec != "eac3" {
			return out, ErrInvalidConfiguration
		}
		out.Audio = append(out.Audio, "-c:a", codec, "-b:a", strconv.Itoa(bitrate/1000)+"k", "-ac", strconv.Itoa(channels))
	}
	return out, nil
}

// DownmixFilter is the one channel reduction this server performs. The matrix is
// ffmpeg's ITU-R BS.775 default with the LFE left out, forced to normalise so it
// cannot clip whether the decoder produced floats or integers; a stereo result
// gets three decibels of make-up gain, because a normalised fold-down of a film
// mix is otherwise markedly quieter than the same film's own stereo track, and a
// limiter takes the rare full-scale passage that gain would push over.
func DownmixFilter(channels int) string {
	if channels >= 6 {
		return "aresample=rematrix_maxval=1.0,aformat=channel_layouts=5.1"
	}
	return "aresample=rematrix_maxval=1.0,aformat=channel_layouts=stereo,volume=3dB,alimiter=limit=0.95:level=false"
}

func encoderFor(backend HardwareBackend, spec backendSpec) string {
	if backend == BackendSoftware || spec.encoder == "" {
		return "libx264"
	}
	return spec.encoder
}

// HDRTransfers are the probe transfer characteristics that mean HDR: PQ and HLG.
// bt2020-10 and bt2020-12 are the BT.2020 *SDR* transfer (the BT.709 curve at a
// higher precision) and smpte428 is the DCI cinema curve; tone mapping any of
// them as PQ crushes a picture that was never HDR.
var HDRTransfers = []string{"smpte2084", "arib-std-b67"}

// IsHDR reports whether probe facts describe an HDR source.
func IsHDR(transfer, primaries string) bool {
	transfer, primaries = strings.ToLower(strings.TrimSpace(transfer)), strings.ToLower(strings.TrimSpace(primaries))
	for _, v := range HDRTransfers {
		if transfer == v {
			return true
		}
	}
	_ = primaries
	// BT.2020 primaries with no stated transfer are wide-gamut SDR far more often
	// than they are untagged PQ, and the two mistakes are not equal: an SDR
	// picture tone mapped as PQ is ruined, an untagged PQ picture left alone is
	// merely flat. Primaries alone therefore never mean HDR.
	return false
}

// All backends use the same fit expression; rounding down keeps codec-aligned
// dimensions within both client bounds and never enlarges the source.
func boundedScaleFilter(backend HardwareBackend, width, height int) string {
	ratio := "1"
	if width > 0 {
		ratio = "min(" + ratio + "," + strconv.Itoa(width) + "/iw)"
	}
	if height > 0 {
		ratio = "min(" + ratio + "," + strconv.Itoa(height) + "/ih)"
	}
	filter := "scale"
	switch backend {
	case BackendNVENC:
		filter = "scale_cuda"
	case BackendVAAPI:
		filter = "scale_vaapi"
	case BackendQSV:
		filter = "vpp_qsv"
	}
	return filter + "=w='max(2,trunc(iw*" + ratio + "/2)*2)':h='max(2,trunc(ih*" + ratio + "/2)*2)'"
}
