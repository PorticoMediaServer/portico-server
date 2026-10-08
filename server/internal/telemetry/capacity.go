package telemetry

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/decoder"
	"strings"
	"sync"
	"time"
)

// TranscodeSettings is the transcoding half of the owner settings registry, as
// this package needs it. It is a plain copy rather than an import so telemetry
// stays a leaf package and the mapping from the registry is written once, in
// the HTTP layer, where both sides are already in view.
type TranscodeSettings struct {
	Enabled                 bool
	HardwareBackend         string
	HardwareDevice          string
	HDRToneMapping          bool
	HDRToneMappingAlgorithm string
	X264Preset              string
	DirectStreamRemux       bool
	PlanningPolicy          string
	ThrottleBufferSeconds   int
	PlayedRetentionSeconds  int
	TemporaryDirectory      string
	MaxConcurrentSessions   int
	MaxHardwareSessions     int
	MaxSoftwareSessions     int
	MaxBackgroundSessions   int
}

// HardwareProbe is one backend's answer to "can this server actually encode
// with you". Only the delivery service can answer it truthfully, so this
// package never guesses: an unprobed backend is reported unsupported with the
// reason named.
type HardwareProbe struct {
	Backend   string `json:"backend"`
	Supported bool   `json:"supported"`
	Detail    string `json:"detail"`
}

// HardwareProbeSource is implemented by the delivery service. Device is the
// owner's configured device selector, which some backends need to probe.
type HardwareProbeSource interface {
	Probe(ctx context.Context, device string) []HardwareProbe
}

// SoftwareOnlyProbes is the default source until the delivery service publishes
// real probe results. It claims nothing it cannot support.
type SoftwareOnlyProbes struct{}

const unprobed = "The delivery service has not published a probe result for this backend on this server."

func (SoftwareOnlyProbes) Probe(context.Context, string) []HardwareProbe {
	out := []HardwareProbe{{Backend: "software", Supported: true, Detail: "Software encoding is always available."}}
	for _, backend := range HardwareBackends {
		out = append(out, HardwareProbe{Backend: backend, Detail: unprobed})
	}
	return out
}

// HardwareBackends is the probe order: the accepted backends other than auto
// and software, which are resolutions rather than hardware.
var HardwareBackends = []string{"videotoolbox", "vaapi", "qsv", "nvenc", "amf"}

// Preset is one rung of the published conversion ladder. Clients render these
// as the quality choices an owner's server can produce.
type Preset struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Height    int    `json:"height"`
	VideoKbps int    `json:"videoKbps"`
	AudioKbps int    `json:"audioKbps"`
}

// Presets is the server's ladder. It is server policy, published rather than
// inferred by a client.
func Presets() []Preset {
	return []Preset{
		{"2160p", "4K", 2160, 40000, 384},
		{"1080p", "1080p", 1080, 10000, 256},
		{"720p", "720p", 720, 4000, 192},
		{"480p", "480p", 480, 1500, 128},
		{"360p", "360p", 360, 720, 96},
	}
}

type Dependency struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	OK      bool   `json:"ok"`
}

type SessionLimits struct {
	Concurrent int `json:"concurrent"`
	Hardware   int `json:"hardware"`
	Software   int `json:"software"`
	Background int `json:"background"`
}

// SessionCounts carries what the delivery service reports. A nil count means
// "the delivery service does not publish this figure", never zero sessions.
type SessionCounts struct {
	Active     *int          `json:"active"`
	Hardware   *int          `json:"hardware"`
	Software   *int          `json:"software"`
	Background *int          `json:"background"`
	Limits     SessionLimits `json:"limits"`
}

type HardwareStatus struct {
	Configured string          `json:"configured"`
	Effective  string          `json:"effective"`
	Device     string          `json:"device"`
	Probes     []HardwareProbe `json:"probes"`
}

type ToneMappingStatus struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type TemporaryDirectory struct {
	Path      string `json:"path"`
	Ready     bool   `json:"ready"`
	FreeBytes *int64 `json:"freeBytes"`
}

// Report is the wire shape of GET /v1/admin/transcode/capacity.
type Report struct {
	Toolchain              *decoder.ToolchainFacts `json:"toolchain,omitempty"`
	Enabled                bool                    `json:"enabled"`
	Hardware               HardwareStatus          `json:"hardware"`
	Sessions               SessionCounts           `json:"sessions"`
	HDRToneMapping         ToneMappingStatus       `json:"hdrToneMapping"`
	DirectStreamRemux      bool                    `json:"directStreamRemux"`
	X264Preset             string                  `json:"x264Preset"`
	PlanningPolicy         string                  `json:"planningPolicy"`
	ThrottleBufferSeconds  int                     `json:"throttleBufferSeconds"`
	PlayedRetentionSeconds int                     `json:"playedRetentionSeconds"`
	TemporaryDirectory     TemporaryDirectory      `json:"temporaryDirectory"`
	Presets                []Preset                `json:"presets"`
	Dependencies           map[string]Dependency   `json:"dependencies"`
	Warnings               []string                `json:"warnings"`
	ObservedAt             int64                   `json:"observedAt"`
}

// Capacity composes the report. Probe and dependency results are cached because
// they start reporter processes; settings-derived fields are never cached, so an
// owner's save is visible on the next read.
type Capacity struct {
	Probes  HardwareProbeSource
	FFmpeg  string
	FFprobe string
	// State is the server state directory, the fallback working location when no
	// temporary directory is configured.
	State string
	Now   func() time.Time
	// Run is the reporter seam, so tests compose a report without executables.
	Run func(ctx context.Context, name string, args ...string) (string, error)

	mu           sync.Mutex
	cachedAt     int64
	cachedProbes []HardwareProbe
	cachedDeps   map[string]Dependency
}

const probeCache = 60 * time.Second

func (c *Capacity) now() int64 {
	if c.Now != nil {
		return c.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
func (c *Capacity) run(ctx context.Context, name string, args ...string) (string, error) {
	if c.Run != nil {
		return c.Run(ctx, name, args...)
	}
	return runCommand(ctx, name, args...)
}

// Report answers one capacity read.
func (c *Capacity) Report(ctx context.Context, s TranscodeSettings, sessions SessionCounts) Report {
	now := c.now()
	probes, dependencies := c.observe(ctx, s.HardwareDevice, now)
	sessions.Limits = SessionLimits{Concurrent: s.MaxConcurrentSessions, Hardware: s.MaxHardwareSessions, Software: s.MaxSoftwareSessions, Background: s.MaxBackgroundSessions}
	effective, effectiveWarning := resolveBackend(s.HardwareBackend, probes)
	directory := resolveTemporaryDirectory(s.TemporaryDirectory, c.State)
	out := Report{
		Toolchain:              decoder.CurrentToolchain(),
		Enabled:                s.Enabled,
		Hardware:               HardwareStatus{Configured: s.HardwareBackend, Effective: effective, Device: s.HardwareDevice, Probes: probes},
		Sessions:               sessions,
		HDRToneMapping:         toneMapping(s, effective, dependencies["ffmpeg"].OK),
		DirectStreamRemux:      s.DirectStreamRemux,
		X264Preset:             s.X264Preset,
		PlanningPolicy:         s.PlanningPolicy,
		ThrottleBufferSeconds:  s.ThrottleBufferSeconds,
		PlayedRetentionSeconds: s.PlayedRetentionSeconds,
		TemporaryDirectory:     directory,
		Presets:                Presets(),
		Dependencies:           dependencies,
		Warnings:               []string{},
		ObservedAt:             now,
	}
	if out.Toolchain != nil {
		if missing := out.Toolchain.MissingRequired(); len(missing) > 0 {
			out.Warnings = append(out.Warnings, "Media toolchain is missing: "+strings.Join(missing, ", "))
		}
	}
	if !s.Enabled {
		out.Warnings = append(out.Warnings, "Transcoding is turned off. Viewers whose device cannot play a source directly will not be offered a converted stream.")
	}
	if effectiveWarning != "" {
		out.Warnings = append(out.Warnings, effectiveWarning)
	}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if !dependencies[name].OK {
			out.Warnings = append(out.Warnings, "The "+name+" executable could not be run. Conversions and probing will fail until it is installed and reachable.")
		}
	}
	if !directory.Ready {
		out.Warnings = append(out.Warnings, "The conversion working directory is not writable. Conversions will fail until it exists and this server can write to it.")
	}
	if free := directory.FreeBytes; free != nil && *free < 2<<30 {
		out.Warnings = append(out.Warnings, "The conversion working directory has less than 2 GB free.")
	}
	return out
}

// observe refreshes the reporter-backed parts of the report at most once per
// cache period, so a dashboard polling every few seconds starts no processes.
func (c *Capacity) observe(ctx context.Context, device string, now int64) ([]HardwareProbe, map[string]Dependency) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedProbes != nil && now-c.cachedAt < probeCache.Milliseconds() {
		return c.cachedProbes, c.cachedDeps
	}
	source := c.Probes
	if source == nil {
		source = SoftwareOnlyProbes{}
	}
	probes := source.Probe(ctx, device)
	if probes == nil {
		probes = []HardwareProbe{}
	}
	dependencies := map[string]Dependency{
		"ffmpeg":  c.dependency(ctx, c.FFmpeg, "ffmpeg"),
		"ffprobe": c.dependency(ctx, c.FFprobe, "ffprobe"),
	}
	c.cachedAt, c.cachedProbes, c.cachedDeps = now, probes, dependencies
	return probes, dependencies
}

func (c *Capacity) dependency(ctx context.Context, configured, fallback string) Dependency {
	name := configured
	if name == "" {
		name = fallback
	}
	out := Dependency{Path: name}
	if resolved, e := lookPath(name); e == nil {
		out.Path = resolved
	}
	raw, e := c.run(ctx, name, "-version")
	if e != nil {
		return out
	}
	line, _, _ := strings.Cut(strings.TrimSpace(raw), "\n")
	out.Version, out.OK = bounded(strings.TrimSpace(line)), line != ""
	return out
}

// resolveBackend turns the configured backend into the one this server will
// actually use, which is what an owner needs to see when auto falls back.
func resolveBackend(configured string, probes []HardwareProbe) (string, string) {
	supported := map[string]bool{}
	for _, probe := range probes {
		supported[probe.Backend] = probe.Supported
	}
	switch configured {
	case "", "software":
		return "software", ""
	case "auto":
		for _, backend := range HardwareBackends {
			if supported[backend] {
				return backend, ""
			}
		}
		return "software", "No hardware encoder probed successfully, so conversions run in software."
	}
	if supported[configured] {
		return configured, ""
	}
	return "software", "The selected hardware backend did not probe successfully, so conversions run in software."
}

// resolveTemporaryDirectory reports whether conversions can actually write
// where they have been told to. Readiness is a write test, not a path check.
func resolveTemporaryDirectory(configured, state string) TemporaryDirectory {
	path := configured
	if path == "" && state != "" {
		path = filepath.Join(state, "hls")
	}
	out := TemporaryDirectory{Path: path}
	if path == "" {
		return out
	}
	out.Ready = writable(path)
	if free, _, ok := VolumeUsage(path); ok {
		out.FreeBytes = &free
	}
	return out
}

func toneMapping(s TranscodeSettings, effective string, ffmpegOK bool) ToneMappingStatus {
	if !s.HDRToneMapping {
		return ToneMappingStatus{Status: "disabled", Detail: "HDR sources are delivered without tone mapping. A device that cannot display HDR may show washed-out colour."}
	}
	if !ffmpegOK {
		return ToneMappingStatus{Status: "unavailable", Detail: "Tone mapping needs a working ffmpeg."}
	}
	if effective == "software" {
		return ToneMappingStatus{Status: "limited", Detail: "Tone mapping with the " + s.HDRToneMappingAlgorithm + " curve runs in software and costs noticeably more CPU for each converted stream."}
	}
	return ToneMappingStatus{Status: "available", Detail: "Tone mapping with the " + s.HDRToneMappingAlgorithm + " curve runs on the " + effective + " backend."}
}
