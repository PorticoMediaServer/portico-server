// Package optimized defines Portico's versioned, deterministic optimized-media
// registry. It is deliberately independent of FFmpeg and persistence so the
// same policy can be consumed by planners, executors, and artifact reconcilers.
package optimized

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	RegistryVersion = "optimized-presets-v1"
	PlannerRevision = "optimized-output-planner-v2"
)

type Container string
type VideoCodec string
type QualityMode string
type DynamicRange string
type HDRAction string
type DVAction string
type EncoderRoute string

const (
	ContainerMP4           Container    = "mp4"
	ContainerMKV           Container    = "matroska"
	CodecH264              VideoCodec   = "h264"
	CodecHEVC              VideoCodec   = "hevc"
	CodecAV1               VideoCodec   = "av1"
	QualityCRF             QualityMode  = "crf"
	QualityCQ              QualityMode  = "constant-quality"
	RangeSDR               DynamicRange = "sdr"
	RangeHDR10             DynamicRange = "hdr10"
	RangeHLG               DynamicRange = "hlg"
	RangeHDR10Plus         DynamicRange = "hdr10-plus"
	RangeDolbyVision       DynamicRange = "dolby-vision"
	HDRPreserve            HDRAction    = "preserve"
	HDRToneMapSDR          HDRAction    = "tone-map-sdr"
	HDRDowngradeHDR10      HDRAction    = "downgrade-hdr10"
	HDRUnsupported         HDRAction    = "unsupported"
	DVUseVerifiedBaseSDR   DVAction     = "use-verified-base-tone-map-sdr"
	DVUseVerifiedBaseHDR10 DVAction     = "use-verified-base-hdr10"
	DVUnsupported          DVAction     = "unsupported"
	RouteSoftwareH264      EncoderRoute = "software-libx264"
	RouteSoftwareHEVC      EncoderRoute = "software-libx265"
	RouteSoftwareAV1       EncoderRoute = "software-libsvtav1"
	RouteVideoToolbox      EncoderRoute = "videotoolbox"
	RouteQSV               EncoderRoute = "qsv"
	RouteVAAPI             EncoderRoute = "vaapi"
	RouteNVENC             EncoderRoute = "nvenc"
	RouteAMF               EncoderRoute = "amf"
)

type Quality struct {
	Mode    QualityMode `json:"mode"`
	Control string      `json:"control"`
	Value   int         `json:"value"`
	Speed   string      `json:"speed"`
}
type RouteQuality struct {
	Route   EncoderRoute `json:"route"`
	Quality Quality      `json:"quality"`
}
type AudioStep struct {
	Codec       string `json:"codec"`
	MinChannels int    `json:"minChannels,omitempty"`
	MaxChannels int    `json:"maxChannels"`
	Copy        bool   `json:"copy,omitempty"`
}
type AudioPolicy struct {
	Steps                 []AudioStep `json:"steps"`
	PreserveLayout        bool        `json:"preserveLayout"`
	StereoDownmix         bool        `json:"stereoDownmix"`
	PreserveObjectsOnCopy bool        `json:"preserveObjectsOnCopy"`
}
type ColorPolicy struct {
	SDR                 HDRAction           `json:"sdr"`
	HDR10               HDRAction           `json:"hdr10"`
	HLG                 HDRAction           `json:"hlg"`
	HDR10Plus           HDRAction           `json:"hdr10Plus"`
	DolbyVisionProfiles map[string]DVAction `json:"dolbyVisionProfiles"`
	UnknownDolbyVision  DVAction            `json:"unknownDolbyVision"`
}
type ArtifactPolicy struct {
	Extension             string   `json:"extension"`
	RetainSupersededHours int      `json:"retainSupersededHours"`
	ReprobeBeforePublish  bool     `json:"reprobeBeforePublish"`
	AtomicDurablePublish  bool     `json:"atomicDurablePublish"`
	InvalidateOn          []string `json:"invalidateOn"`
}

type Preset struct {
	ID                string         `json:"id"`
	Version           int            `json:"version"`
	Order             int            `json:"order"`
	Label             string         `json:"label"`
	Container         Container      `json:"container"`
	VideoCodec        VideoCodec     `json:"videoCodec"`
	MaxWidth          int            `json:"maxWidth,omitempty"`
	MaxHeight         int            `json:"maxHeight,omitempty"`
	SourceSize        bool           `json:"sourceSize,omitempty"`
	RouteQualities    []RouteQuality `json:"routeQualities"`
	Audio             AudioPolicy    `json:"audio"`
	Color             ColorPolicy    `json:"color"`
	EncoderRoutes     []EncoderRoute `json:"encoderRoutes"`
	CompatibilityTags []string       `json:"compatibilityTags"`
	Artifact          ArtifactPolicy `json:"artifact"`
}

type SourceFacts struct {
	Width              int
	Height             int
	SARNumerator       int
	SARDenominator     int
	Rotation           int
	Interlaced         bool
	DynamicRange       DynamicRange
	DolbyVisionProfile string
	VerifiedBaseRange  DynamicRange
	AudioCodec         string
	AudioChannels      int
	AudioLayout        string
	AudioHasObjects    bool
}
type Geometry struct {
	Width               int     `json:"width"`
	Height              int     `json:"height"`
	SampleAspectRatio   string  `json:"sampleAspectRatio"`
	SourceDisplayAspect float64 `json:"sourceDisplayAspect"`
	Rotation            int     `json:"rotation,omitempty"`
}
type AudioDecision struct {
	Codec            string `json:"codec"`
	Channels         int    `json:"channels"`
	Layout           string `json:"layout"`
	Copy             bool   `json:"copy"`
	ObjectsPreserved bool   `json:"objectsPreserved"`
	Downmixed        bool   `json:"downmixed"`
}
type OutputPlan struct {
	PresetID            string        `json:"presetId"`
	PresetVersion       int           `json:"presetVersion"`
	EncoderRoute        EncoderRoute  `json:"encoderRoute"`
	Quality             Quality       `json:"quality"`
	Geometry            Geometry      `json:"geometry"`
	DynamicRange        DynamicRange  `json:"dynamicRange"`
	HDRAction           HDRAction     `json:"hdrAction"`
	DolbyVisionAction   DVAction      `json:"dolbyVisionAction,omitempty"`
	ToneMapAlgorithm    string        `json:"toneMapAlgorithm,omitempty"`
	Deinterlace         bool          `json:"deinterlace"`
	Audio               AudioDecision `json:"audio"`
	CompatibilityTags   []string      `json:"compatibilityTags"`
	RevisionFingerprint string        `json:"revisionFingerprint"`
}

var registry = buildRegistry()

func buildRegistry() []Preset {
	artifact := func(ext string) ArtifactPolicy {
		return ArtifactPolicy{ext, 168, true, true, []string{"source-fingerprint", "planner-revision", "preset-revision"}}
	}
	universalColor := ColorPolicy{SDR: HDRPreserve, HDR10: HDRToneMapSDR, HLG: HDRToneMapSDR, HDR10Plus: HDRToneMapSDR, DolbyVisionProfiles: map[string]DVAction{"5": DVUnsupported, "7": DVUseVerifiedBaseSDR, "8": DVUseVerifiedBaseSDR}, UnknownDolbyVision: DVUnsupported}
	efficientColor := ColorPolicy{SDR: HDRPreserve, HDR10: HDRPreserve, HLG: HDRPreserve, HDR10Plus: HDRDowngradeHDR10, DolbyVisionProfiles: map[string]DVAction{"5": DVUnsupported, "7": DVUseVerifiedBaseHDR10, "8": DVUseVerifiedBaseHDR10}, UnknownDolbyVision: DVUnsupported}
	universalAudio := AudioPolicy{Steps: []AudioStep{{Codec: "aac", MinChannels: 2, MaxChannels: 2}}, PreserveLayout: false, StereoDownmix: true}
	efficientAudio := AudioPolicy{Steps: []AudioStep{{Codec: "copy", MaxChannels: 8, Copy: true}, {Codec: "eac3", MaxChannels: 6}, {Codec: "aac", MaxChannels: 6}, {Codec: "aac", MaxChannels: 2}}, PreserveLayout: true, StereoDownmix: true, PreserveObjectsOnCopy: true}
	h264Routes := []EncoderRoute{RouteSoftwareH264, RouteVideoToolbox, RouteQSV, RouteVAAPI, RouteNVENC, RouteAMF}
	hevcRoutes := []EncoderRoute{RouteSoftwareHEVC, RouteVideoToolbox, RouteQSV, RouteVAAPI, RouteNVENC, RouteAMF}
	av1Routes := []EncoderRoute{RouteSoftwareAV1, RouteQSV, RouteVAAPI, RouteNVENC, RouteAMF}
	mk := func(id, label string, order, width, height int, codec VideoCodec, container Container, quality int, speed string, audio AudioPolicy, color ColorPolicy, routes []EncoderRoute, tags []string) Preset {
		routeQualities := make([]RouteQuality, 0, len(routes))
		for _, route := range routes {
			mode, control, routeValue, routeSpeed := QualityCQ, "", quality, "balanced"
			if route == RouteSoftwareH264 || route == RouteSoftwareHEVC || route == RouteSoftwareAV1 {
				mode, control, routeSpeed = QualityCRF, "crf", speed
			} else {
				switch route {
				case RouteVideoToolbox:
					control, routeValue = "q:v", 100-quality
				case RouteQSV:
					control = "global_quality"
				case RouteVAAPI:
					control = "global_quality"
				case RouteNVENC:
					control = "cq"
				case RouteAMF:
					control = "qp_i_p"
				}
			}
			routeQualities = append(routeQualities, RouteQuality{Route: route, Quality: Quality{Mode: mode, Control: control, Value: routeValue, Speed: routeSpeed}})
		}
		return Preset{ID: id, Version: 1, Order: order, Label: label, Container: container, VideoCodec: codec, MaxWidth: width, MaxHeight: height, RouteQualities: routeQualities, Audio: audio, Color: color, EncoderRoutes: routes, CompatibilityTags: tags, Artifact: artifact(map[Container]string{ContainerMP4: ".mp4", ContainerMKV: ".mkv"}[container])}
	}
	p := []Preset{
		mk("universal-1080p", "Universal 1080p", 1, 1920, 1080, CodecH264, ContainerMP4, 20, "medium", universalAudio, universalColor, h264Routes, []string{"universal", "h264", "sdr", "1080p"}),
		mk("universal-720p", "Universal 720p", 2, 1280, 720, CodecH264, ContainerMP4, 21, "medium", universalAudio, universalColor, h264Routes, []string{"universal", "h264", "sdr", "720p"}),
		mk("universal-480p", "Universal 480p", 3, 854, 480, CodecH264, ContainerMP4, 22, "medium", universalAudio, universalColor, h264Routes, []string{"universal", "h264", "sdr", "480p"}),
		mk("efficient-4k", "Efficient 4K", 4, 3840, 2160, CodecHEVC, ContainerMKV, 20, "medium", efficientAudio, efficientColor, hevcRoutes, []string{"efficient", "hevc", "hdr-capable", "4k"}),
		mk("efficient-1080p", "Efficient 1080p", 5, 1920, 1080, CodecHEVC, ContainerMKV, 21, "medium", efficientAudio, efficientColor, hevcRoutes, []string{"efficient", "hevc", "hdr-capable", "1080p"}),
		mk("efficient-720p", "Efficient 720p", 6, 1280, 720, CodecHEVC, ContainerMKV, 22, "medium", efficientAudio, efficientColor, hevcRoutes, []string{"efficient", "hevc", "hdr-capable", "720p"}),
		mk("maximum-compression-source", "Maximum Compression Source Size", 7, 0, 0, CodecAV1, ContainerMKV, 28, "slow", efficientAudio, efficientColor, av1Routes, []string{"maximum-compression", "av1", "hdr-capable", "source-size"}),
		mk("maximum-compression-1080p", "Maximum Compression 1080p", 8, 1920, 1080, CodecAV1, ContainerMKV, 30, "slow", efficientAudio, efficientColor, av1Routes, []string{"maximum-compression", "av1", "hdr-capable", "1080p"}),
	}
	p[6].SourceSize = true
	return p
}

func List() []Preset {
	out := make([]Preset, len(registry))
	for i := range registry {
		out[i] = clonePreset(registry[i])
	}
	return out
}
func Lookup(id string) (Preset, bool) {
	for _, p := range registry {
		if p.ID == id {
			return clonePreset(p), true
		}
	}
	return Preset{}, false
}

func clonePreset(p Preset) Preset {
	color := p.Color.DolbyVisionProfiles
	p.Audio.Steps = append([]AudioStep(nil), p.Audio.Steps...)
	p.RouteQualities = append([]RouteQuality(nil), p.RouteQualities...)
	p.EncoderRoutes = append([]EncoderRoute(nil), p.EncoderRoutes...)
	p.CompatibilityTags = append([]string(nil), p.CompatibilityTags...)
	p.Artifact.InvalidateOn = append([]string(nil), p.Artifact.InvalidateOn...)
	p.Color.DolbyVisionProfiles = make(map[string]DVAction, len(color))
	for k, v := range color {
		p.Color.DolbyVisionProfiles[k] = v
	}
	return p
}

func ValidatePreset(p Preset) error {
	if strings.TrimSpace(p.ID) == "" || p.Version < 1 || p.Order < 1 || strings.TrimSpace(p.Label) == "" {
		return errors.New("identity, positive version/order, and label are required")
	}
	if p.Container != ContainerMP4 && p.Container != ContainerMKV {
		return fmt.Errorf("unsupported container %q", p.Container)
	}
	if p.VideoCodec != CodecH264 && p.VideoCodec != CodecHEVC && p.VideoCodec != CodecAV1 {
		return fmt.Errorf("unsupported video codec %q", p.VideoCodec)
	}
	if p.SourceSize == (p.MaxWidth > 0 || p.MaxHeight > 0) {
		return errors.New("preset must use either source-size or positive width and height ceilings")
	}
	if !p.SourceSize && (p.MaxWidth < 2 || p.MaxHeight < 2 || p.MaxWidth%2 != 0 || p.MaxHeight%2 != 0) {
		return errors.New("resolution ceilings must be positive even dimensions")
	}
	if len(p.Audio.Steps) == 0 || !p.Audio.StereoDownmix {
		return errors.New("audio ladder and stereo fallback are required")
	}
	for _, s := range p.Audio.Steps {
		if s.Codec == "" || s.MinChannels < 0 || s.MinChannels > s.MaxChannels || s.MaxChannels < 1 || s.MaxChannels > 8 || (s.Copy && s.Codec != "copy") {
			return errors.New("invalid audio step")
		}
	}
	if p.Color.SDR == "" || p.Color.HDR10 == "" || p.Color.HLG == "" || p.Color.HDR10Plus == "" || p.Color.UnknownDolbyVision == "" {
		return errors.New("complete color policy is required")
	}
	validHDR := func(a HDRAction) bool {
		return a == HDRPreserve || a == HDRToneMapSDR || a == HDRDowngradeHDR10 || a == HDRUnsupported
	}
	if !validHDR(p.Color.SDR) || !validHDR(p.Color.HDR10) || !validHDR(p.Color.HLG) || !validHDR(p.Color.HDR10Plus) {
		return errors.New("invalid HDR action")
	}
	for _, profile := range []string{"5", "7", "8"} {
		a := p.Color.DolbyVisionProfiles[profile]
		if a != DVUseVerifiedBaseSDR && a != DVUseVerifiedBaseHDR10 && a != DVUnsupported {
			return fmt.Errorf("dolby vision profile %s policy required", profile)
		}
	}
	if p.Color.UnknownDolbyVision != DVUseVerifiedBaseSDR && p.Color.UnknownDolbyVision != DVUseVerifiedBaseHDR10 && p.Color.UnknownDolbyVision != DVUnsupported {
		return errors.New("invalid unknown Dolby Vision action")
	}
	if len(p.EncoderRoutes) == 0 || len(p.CompatibilityTags) == 0 {
		return errors.New("encoder routes and compatibility tags are required")
	}
	eligible := make(map[EncoderRoute]bool, len(p.EncoderRoutes))
	for _, route := range p.EncoderRoutes {
		if route == "" || eligible[route] {
			return errors.New("encoder routes must be non-empty and unique")
		}
		eligible[route] = true
	}
	qualities := make(map[EncoderRoute]bool, len(p.RouteQualities))
	for _, rq := range p.RouteQualities {
		if !eligible[rq.Route] || qualities[rq.Route] {
			return errors.New("each route quality must name one unique eligible encoder route")
		}
		if rq.Quality.Mode != QualityCRF && rq.Quality.Mode != QualityCQ {
			return errors.New("unsupported quality mode")
		}
		expectedMode, expectedControl := QualityCQ, ""
		switch rq.Route {
		case RouteSoftwareH264, RouteSoftwareHEVC, RouteSoftwareAV1:
			expectedMode, expectedControl = QualityCRF, "crf"
		case RouteVideoToolbox:
			expectedControl = "q:v"
		case RouteQSV, RouteVAAPI:
			expectedControl = "global_quality"
		case RouteNVENC:
			expectedControl = "cq"
		case RouteAMF:
			expectedControl = "qp_i_p"
		default:
			return fmt.Errorf("unknown encoder route %q", rq.Route)
		}
		if rq.Quality.Mode != expectedMode || rq.Quality.Control != expectedControl {
			return fmt.Errorf("encoder route %q requires %s/%s quality", rq.Route, expectedMode, expectedControl)
		}
		maxQuality := 63
		if rq.Quality.Control == "q:v" {
			maxQuality = 100
		}
		if rq.Quality.Control == "" || rq.Quality.Value < 0 || rq.Quality.Value > maxQuality || rq.Quality.Speed == "" {
			return errors.New("invalid quality value or speed")
		}
		qualities[rq.Route] = true
	}
	if len(qualities) != len(eligible) {
		return errors.New("every eligible encoder route requires exactly one quality policy")
	}
	if p.Artifact.Extension == "" || p.Artifact.RetainSupersededHours < 1 || !p.Artifact.ReprobeBeforePublish || !p.Artifact.AtomicDurablePublish || len(p.Artifact.InvalidateOn) != 3 {
		return errors.New("complete crash-durable artifact policy is required")
	}
	return nil
}

func ValidateRegistry(presets []Preset) error {
	if len(presets) != 8 {
		return fmt.Errorf("registry must contain exactly eight presets, got %d", len(presets))
	}
	ids := map[string]bool{}
	orders := map[int]bool{}
	for _, p := range presets {
		if err := ValidatePreset(p); err != nil {
			return fmt.Errorf("preset %q: %w", p.ID, err)
		}
		if ids[p.ID] || orders[p.Order] {
			return errors.New("duplicate preset id or order")
		}
		ids[p.ID] = true
		orders[p.Order] = true
	}
	for i := 1; i <= 8; i++ {
		if !orders[i] {
			return errors.New("preset ordering must be contiguous")
		}
	}
	expected := []string{"universal-1080p", "universal-720p", "universal-480p", "efficient-4k", "efficient-1080p", "efficient-720p", "maximum-compression-source", "maximum-compression-1080p"}
	sorted := append([]Preset(nil), presets...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Order < sorted[j].Order })
	for i := range expected {
		if sorted[i].ID != expected[i] {
			return fmt.Errorf("order %d must be stable preset %q", i+1, expected[i])
		}
	}
	return nil
}

func RevisionFingerprint() string {
	payload := struct {
		Registry string   `json:"registry"`
		Planner  string   `json:"planner"`
		Presets  []Preset `json:"presets"`
	}{RegistryVersion, PlannerRevision, registry}
	b, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func Plan(p Preset, source SourceFacts) (OutputPlan, error) {
	if len(p.EncoderRoutes) == 0 {
		return OutputPlan{}, errors.New("preset has no eligible encoder route")
	}
	return PlanForRoute(p, source, p.EncoderRoutes[0])
}

// PlanForRoute projects output using the exact quality semantics of an
// eligible encoder route.
func PlanForRoute(p Preset, source SourceFacts, route EncoderRoute) (OutputPlan, error) {
	if err := ValidatePreset(p); err != nil {
		return OutputPlan{}, err
	}
	var quality Quality
	foundRoute := false
	for _, rq := range p.RouteQualities {
		if rq.Route == route {
			quality, foundRoute = rq.Quality, true
			break
		}
	}
	if !foundRoute {
		return OutputPlan{}, fmt.Errorf("encoder route %q is not eligible for preset %q", route, p.ID)
	}
	if source.Width < 1 || source.Height < 1 {
		return OutputPlan{}, errors.New("positive source dimensions required")
	}
	g, err := planGeometry(p, source)
	if err != nil {
		return OutputPlan{}, err
	}
	hdr, dv, outRange, err := planColor(p.Color, source)
	if err != nil {
		return OutputPlan{}, err
	}
	audio, err := planAudio(p.Audio, source)
	if err != nil {
		return OutputPlan{}, err
	}
	tags := append([]string(nil), p.CompatibilityTags...)
	sort.Strings(tags)
	toneMap := ""
	if hdr == HDRToneMapSDR {
		toneMap = "hable"
	}
	return OutputPlan{PresetID: p.ID, PresetVersion: p.Version, EncoderRoute: route, Quality: quality, Geometry: g,
		DynamicRange: outRange, HDRAction: hdr, DolbyVisionAction: dv, ToneMapAlgorithm: toneMap, Deinterlace: source.Interlaced,
		Audio: audio, CompatibilityTags: tags, RevisionFingerprint: RevisionFingerprint()}, nil
}

func planGeometry(p Preset, s SourceFacts) (Geometry, error) {
	sn, sd := s.SARNumerator, s.SARDenominator
	if sn == 0 && sd == 0 {
		sn, sd = 1, 1
	}
	if sn < 1 || sd < 1 {
		return Geometry{}, errors.New("sample aspect ratio must be positive")
	}
	displayW, displayH := float64(s.Width)*float64(sn)/float64(sd), float64(s.Height)
	rot := ((s.Rotation % 360) + 360) % 360
	if rot == 90 || rot == 270 {
		displayW, displayH = displayH, displayW
	}
	if displayW <= 0 || displayH <= 0 {
		return Geometry{}, errors.New("invalid display geometry")
	}
	scale := 1.0
	if !p.SourceSize {
		scale = math.Min(1, math.Min(float64(p.MaxWidth)/displayW, float64(p.MaxHeight)/displayH))
	}
	w, h := evenFloor(displayW*scale), evenFloor(displayH*scale)
	if w < 2 || h < 2 {
		return Geometry{}, errors.New("output geometry is too small")
	}
	return Geometry{Width: w, Height: h, SampleAspectRatio: "1:1", SourceDisplayAspect: displayW / displayH, Rotation: rot}, nil
}

func evenFloor(v float64) int {
	n := int(math.Floor(v + 1e-9))
	if n%2 != 0 {
		n--
	}
	return n
}

func planColor(c ColorPolicy, s SourceFacts) (HDRAction, DVAction, DynamicRange, error) {
	switch s.DynamicRange {
	case "", RangeSDR:
		return applyHDRAction(c.SDR, RangeSDR)
	case RangeHDR10:
		return applyHDRAction(c.HDR10, RangeHDR10)
	case RangeHLG:
		return applyHDRAction(c.HLG, RangeHLG)
	case RangeHDR10Plus:
		return applyHDRAction(c.HDR10Plus, RangeHDR10Plus)
	case RangeDolbyVision:
		a := c.DolbyVisionProfiles[strings.TrimSpace(s.DolbyVisionProfile)]
		if a == "" {
			a = c.UnknownDolbyVision
		}
		if a == DVUnsupported {
			return HDRUnsupported, a, "", fmt.Errorf("dolby vision profile %q is unsupported", s.DolbyVisionProfile)
		}
		if s.VerifiedBaseRange != RangeSDR && s.VerifiedBaseRange != RangeHDR10 && s.VerifiedBaseRange != RangeHLG {
			return HDRUnsupported, DVUnsupported, "", errors.New("dolby vision fallback requires a verified base layer")
		}
		if a == DVUseVerifiedBaseSDR {
			if s.VerifiedBaseRange == RangeSDR {
				return HDRPreserve, a, RangeSDR, nil
			}
			return HDRToneMapSDR, a, RangeSDR, nil
		}
		if s.VerifiedBaseRange != RangeHDR10 {
			return HDRUnsupported, DVUnsupported, "", errors.New("dolby vision HDR10 output requires a verified HDR10 base layer")
		}
		return HDRDowngradeHDR10, a, RangeHDR10, nil
	default:
		return HDRUnsupported, "", "", fmt.Errorf("unsupported dynamic range %q", s.DynamicRange)
	}
}

func applyHDRAction(action HDRAction, original DynamicRange) (HDRAction, DVAction, DynamicRange, error) {
	if action == HDRUnsupported {
		return HDRUnsupported, "", "", fmt.Errorf("dynamic range %q is unsupported by the selected preset", original)
	}
	return action, "", rangeFor(action, original), nil
}

func rangeFor(a HDRAction, original DynamicRange) DynamicRange {
	if a == HDRToneMapSDR {
		return RangeSDR
	}
	if a == HDRDowngradeHDR10 {
		return RangeHDR10
	}
	return original
}

func planAudio(a AudioPolicy, s SourceFacts) (AudioDecision, error) {
	channels := s.AudioChannels
	if channels == 0 {
		channels = 2
	}
	if channels < 1 || channels > 32 {
		return AudioDecision{}, errors.New("invalid source audio channel count")
	}
	for _, step := range a.Steps {
		if step.Copy && !copyCompatible(s.AudioCodec) {
			continue
		}
		out := channels
		if out > step.MaxChannels {
			out = step.MaxChannels
		}
		if out < step.MinChannels {
			out = step.MinChannels
		}
		layout := layoutFor(out, s.AudioLayout, channels == out && a.PreserveLayout)
		codec := step.Codec
		if step.Copy {
			codec = strings.ToLower(strings.TrimSpace(s.AudioCodec))
		}
		return AudioDecision{codec, out, layout, step.Copy, s.AudioHasObjects && step.Copy && a.PreserveObjectsOnCopy, out < channels}, nil
	}
	return AudioDecision{}, errors.New("audio ladder has no eligible step")
}

func copyCompatible(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "aac", "ac3", "eac3", "e-ac-3", "truehd", "mlp", "dts", "dca", "dts-hd", "dts-hd ma", "flac", "opus", "pcm", "pcm_s16le", "pcm_s24le":
		return true
	}
	return false
}
func layoutFor(ch int, source string, preserve bool) string {
	if preserve && source != "" {
		return source
	}
	switch ch {
	case 1:
		return "mono"
	case 2:
		return "stereo"
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	default:
		return fmt.Sprintf("%dch", ch)
	}
}
