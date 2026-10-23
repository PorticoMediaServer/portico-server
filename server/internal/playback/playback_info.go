package playback

import "portico.local/server/internal/assets"

// PlaybackInfo is the allowlisted VS10 comparison. Source and Delivered are
// committed source/target facts. Observed is a separately attributed report
// from the playing device, never inferred from an encoder target or profile.
// It contains no grants, URLs, source paths or private device identifiers.
type PictureFacts struct {
	Container                       string  `json:"container,omitempty"`
	Codec                           string  `json:"codec"`
	Profile                         string  `json:"profile,omitempty"`
	BitDepth                        int     `json:"bitDepth,omitempty"`
	Width                           int     `json:"width,omitempty"`
	Height                          int     `json:"height,omitempty"`
	FrameRate                       float64 `json:"frameRate,omitempty"`
	FrameRateNumerator              int     `json:"frameRateNumerator,omitempty"`
	FrameRateDenominator            int     `json:"frameRateDenominator,omitempty"`
	FrameRateMode                   string  `json:"frameRateMode,omitempty"`
	DynamicRange                    string  `json:"dynamicRange,omitempty"`
	DolbyVisionProfile              int     `json:"dolbyVisionProfile,omitempty"`
	DolbyVisionCompatibility        int     `json:"dolbyVisionCompatibility,omitempty"`
	DolbyVisionEnhancementType      string  `json:"dolbyVisionEnhancementType,omitempty"`
	DolbyVisionCMVersion            string  `json:"dolbyVisionCmVersion,omitempty"`
	ConvertedFromDolbyVisionProfile int     `json:"convertedFromDolbyVisionProfile,omitempty"`
	ConvertedFromEnhancementType    string  `json:"convertedFromEnhancementType,omitempty"`
	HDR10Plus                       bool    `json:"hdr10Plus"`
	MasterDisplay                   string  `json:"masterDisplay,omitempty"`
	MaxCLL                          string  `json:"maxCll,omitempty"`
}
type SoundFacts struct {
	Codec       string `json:"codec"`
	Variant     string `json:"variant,omitempty"`
	ObjectAudio string `json:"objectAudio,omitempty"`
	Channels    int    `json:"channels,omitempty"`
	Layout      string `json:"layout,omitempty"`
	SampleRate  int    `json:"sampleRate,omitempty"`
	BitDepth    int    `json:"bitDepth,omitempty"`
	// PlannedHandling describes the selected route, not actual receiver output.
	PlannedHandling string `json:"plannedHandling,omitempty"`
}
type SubtitleFacts struct {
	Track     *TrackLabelParts `json:"track,omitempty"`
	Rendering string           `json:"rendering"`
}
type PlaybackMediaFacts struct {
	Video     *PictureFacts `json:"video,omitempty"`
	Audio     *SoundFacts   `json:"audio,omitempty"`
	Subtitles SubtitleFacts `json:"subtitles"`
}
type PlaybackRoutes struct {
	Picture   string `json:"picture,omitempty"`
	Sound     string `json:"sound,omitempty"`
	Subtitles string `json:"subtitles"`
}

// PlaybackReason is data for the canonical catalogue formatter. No raw server
// diagnostic text is a display sentence, and raw strategy enums stay intact.
type PlaybackReason struct {
	Kind               string `json:"kind"`
	VideoCodec         string `json:"videoCodec,omitempty"`
	AudioCodec         string `json:"audioCodec,omitempty"`
	Container          string `json:"container,omitempty"`
	DolbyVisionProfile int    `json:"dolbyVisionProfile,omitempty"`
	MaxHeight          int    `json:"maxHeight,omitempty"`
	MaxBitrateKbps     int    `json:"maxBitrateKbps,omitempty"`
}
type PlaybackReplacement struct {
	DynamicRange string `json:"dynamicRange,omitempty"`
	AudioCodec   string `json:"audioCodec,omitempty"`
	Channels     int    `json:"channels,omitempty"`
	Layout       string `json:"layout,omitempty"`
}
type PlaybackLoss struct {
	Feature     string              `json:"feature"`
	Replacement PlaybackReplacement `json:"replacement"`
}
type PlaybackInfo struct {
	Route     string             `json:"route"`
	Routes    PlaybackRoutes     `json:"routes"`
	Reason    PlaybackReason     `json:"reason"`
	Source    PlaybackMediaFacts `json:"source"`
	Delivered PlaybackMediaFacts `json:"delivered"`
	Losses    []PlaybackLoss     `json:"losses"`
	Network   string             `json:"network"`
	Observed  *ObservedPlayback  `json:"observed,omitempty"`
	// Estimates remain separate from the client's measured delivered bitrate.
	EstimatedBitrateKbps int    `json:"estimatedBitrateKbps,omitempty"`
	Basis                string `json:"basis"`
}

const reasonObjectAudioLost = "object_audio_not_preserved"
const reasonHDR10PlusNotApplied = "hdr10plus_not_applied"

func routeForAction(action string, repackaged bool) string {
	if action == "convert" {
		return "converted"
	}
	if repackaged {
		return "repackaged"
	}
	return "original"
}
func strongerRoute(a, b string) string {
	rank := map[string]int{"original": 0, "repackaged": 1, "converted": 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
func PlaybackInfoForPlan(p DeliveryPlan, subtitle string) *PlaybackInfo {
	out := &PlaybackInfo{Route: "original", Reason: PlaybackReason{Kind: "compatible"}, Losses: []PlaybackLoss{}, Network: "unknown", Basis: "committed_delivery_plan", Routes: PlaybackRoutes{Subtitles: "original"}}
	out.Source.Subtitles.Rendering = "source"
	switch subtitle {
	case "burn", "burn_in", "burnedIn":
		out.Delivered.Subtitles.Rendering = "burnedIn"
		out.Routes.Subtitles = "converted"
	case "device", "client", "drawnByDevice":
		out.Delivered.Subtitles.Rendering = "drawnByDevice"
	case "sidecar", "external_text", "externalText":
		out.Delivered.Subtitles.Rendering = "externalText"
	default:
		out.Delivered.Subtitles.Rendering = "off"
	}
	if p.Policy != nil {
		switch p.Policy.NetworkClass {
		case NetworkLocal:
			out.Network = "home"
		case NetworkRemote:
			out.Network = "away"
		}
	}
	remux := p.Strategy != DeliveryOriginal
	if p.SourceVideoCodec != "" {
		out.Routes.Picture = routeForAction(p.VideoAction, remux)
	}
	if p.SourceAudioCodec != "" {
		out.Routes.Sound = routeForAction(p.AudioAction, remux)
	}
	if out.Delivered.Subtitles.Rendering == "burnedIn" {
		out.Routes.Picture = "converted"
	}
	for _, r := range []string{out.Routes.Picture, out.Routes.Sound, out.Routes.Subtitles} {
		out.Route = strongerRoute(out.Route, r)
	}
	switch out.Route {
	case "repackaged":
		out.Reason.Kind = "container"
	case "converted":
		out.Reason.Kind = "format"
	}
	out.Reason.Container = p.OutputContainer
	if p.VideoAction == "convert" {
		out.Reason.VideoCodec = p.SourceVideoCodec
	}
	if p.AudioAction == "convert" {
		out.Reason.AudioCodec = p.SourceAudioCodec
	}
	if containsString(p.ReasonCodes, ReasonHeightExceedsPolicy) || containsString(p.ReasonCodes, ReasonBitrateExceedsPolicy) || containsString(p.ReasonCodes, ReasonQualityRungSelected) {
		out.Reason.Kind = "quality"
		out.Reason.MaxHeight = p.TargetHeight
		out.Reason.MaxBitrateKbps = p.VideoBitrateBPS / 1000
	}
	if out.Delivered.Subtitles.Rendering == "burnedIn" {
		out.Reason.Kind = "subtitles"
	}
	if p.Trace == nil {
		return out
	}
	if v := p.Trace.Video; v != nil {
		src := PictureFacts{Container: p.Trace.Container, Codec: v.Codec, Profile: v.Profile, BitDepth: v.BitDepth, Width: v.Width, Height: v.Height, FrameRate: v.FrameRate, FrameRateNumerator: v.Breadth.FrameRateNumerator, FrameRateDenominator: v.Breadth.FrameRateDenominator, FrameRateMode: v.Breadth.FrameRateMode, DynamicRange: v.DynamicRange, DolbyVisionProfile: v.DolbyVisionProfile, DolbyVisionCompatibility: v.DolbyVisionCompatibility, DolbyVisionEnhancementType: v.Breadth.DolbyVisionEnhancementType, DolbyVisionCMVersion: v.Breadth.DolbyVisionCMVersion, HDR10Plus: v.HDR10Plus, MasterDisplay: v.MasterDisplay, MaxCLL: v.MaxCLL}
		out.Source.Video = &src
		dst := src
		dst.Container = p.OutputContainer
		dst.Codec = p.OutputVideoCodec
		if v.OutputRange != "" {
			dst.DynamicRange = v.OutputRange
		}
		if p.VideoAction == "convert" {
			dst.Profile = ""
			dst.BitDepth = p.OutputBitDepth
			dst.DynamicRange = p.OutputRange
			dst.Width, dst.Height = convertedDimensions(p)
			dst.HDR10Plus = false
			dst.DolbyVisionProfile = 0
			dst.DolbyVisionCompatibility = 0
			dst.DolbyVisionEnhancementType = ""
			dst.DolbyVisionCMVersion = ""
			if p.ConversionTarget != nil {
				dst.Profile = p.ConversionTarget.VideoProfile
				dst.BitDepth = p.ConversionTarget.BitDepth
				dst.DynamicRange = p.ConversionTarget.DynamicRange
			}
			if p.MaxFrameRate > 0 && p.MaxFrameRate < dst.FrameRate {
				dst.FrameRate = p.MaxFrameRate
				dst.FrameRateNumerator = 0
				dst.FrameRateDenominator = 0
				// A rate cap alone does not establish constant frame timing.
				dst.FrameRateMode = ""
			}
		} else if p.ConversionTarget != nil && p.ConversionTarget.CopyDolbyVision {
			dst.DolbyVisionProfile = p.ConversionTarget.DolbyVisionProfile
			dst.DolbyVisionCompatibility = p.ConversionTarget.DolbyVisionCompatibility
			if src.DolbyVisionProfile != dst.DolbyVisionProfile {
				dst.ConvertedFromDolbyVisionProfile = src.DolbyVisionProfile
				dst.ConvertedFromEnhancementType = src.DolbyVisionEnhancementType
				dst.DolbyVisionEnhancementType = ""
			}
		}
		if dst.DynamicRange != assets.RangeDolbyVision {
			dst.DolbyVisionProfile = 0
			dst.DolbyVisionCompatibility = 0
			dst.DolbyVisionEnhancementType = ""
			dst.DolbyVisionCMVersion = ""
		}
		if src.DolbyVisionProfile == 5 && dst.DynamicRange == assets.RangeHDR10 && p.ConversionTarget != nil && p.ConversionTarget.ReshapeDolbyVision {
			dst.ConvertedFromDolbyVisionProfile = 5
			out.Reason.DolbyVisionProfile = 5
		}
		if dst.DynamicRange == assets.RangeSDR {
			dst.MasterDisplay = ""
			dst.MaxCLL = ""
		}
		if src.DynamicRange == assets.RangeDolbyVision && dst.DynamicRange != assets.RangeDolbyVision {
			out.Losses = append(out.Losses, PlaybackLoss{"dolbyVision", PlaybackReplacement{DynamicRange: dst.DynamicRange}})
		}
		if src.DynamicRange != "" && src.DynamicRange != assets.RangeSDR && dst.DynamicRange == assets.RangeSDR {
			out.Losses = append(out.Losses, PlaybackLoss{"hdr", PlaybackReplacement{DynamicRange: assets.RangeSDR}})
		}
		if src.HDR10Plus && (!dst.HDR10Plus || containsString(p.ReasonCodes, reasonHDR10PlusNotApplied)) {
			out.Losses = append(out.Losses, PlaybackLoss{"hdr10Plus", PlaybackReplacement{DynamicRange: dst.DynamicRange}})
		}
		out.Delivered.Video = &dst
	}
	if a := p.Trace.Audio; a != nil {
		src := SoundFacts{Codec: a.Codec, Variant: a.Variant, ObjectAudio: a.ObjectAudio, Channels: a.Channels, Layout: a.Layout, SampleRate: a.SampleRate, BitDepth: a.BitDepth}
		out.Source.Audio = &src
		dst := src
		dst.Codec = p.OutputAudioCodec
		if p.AudioPassthrough {
			dst.PlannedHandling = "passthrough"
		} else {
			dst.PlannedHandling = "deviceDecode"
		}
		if p.AudioAction == "convert" {
			dst.Variant = ""
			dst.ObjectAudio = ""
			dst.Channels = p.AudioChannels
			dst.Layout = ""
			dst.SampleRate = p.AudioSampleRate
			dst.BitDepth = 0
			dst.PlannedHandling = "serverDecode"
			if p.ConversionTarget != nil {
				dst.BitDepth = p.ConversionTarget.AudioBitDepth
				dst.Layout = p.ConversionTarget.AudioChannelLayout
			}
		}
		if containsString(p.ReasonCodes, ReasonDTSCoreExtracted) {
			dst.Variant = "dts_core"
			dst.Channels = 0
			dst.Layout = ""
			dst.SampleRate = 0
			dst.BitDepth = 0
			dst.ObjectAudio = ""
		}
		if containsString(p.ReasonCodes, reasonObjectAudioLost) {
			dst.ObjectAudio = ""
		}
		if src.ObjectAudio != "" && dst.ObjectAudio == "" {
			feature := ""
			if src.ObjectAudio == "atmos" {
				feature = "atmos"
			} else if src.ObjectAudio == "dtsx" {
				feature = "dtsx"
			}
			if feature != "" {
				out.Losses = append(out.Losses, PlaybackLoss{feature, PlaybackReplacement{AudioCodec: dst.Codec, Channels: dst.Channels, Layout: dst.Layout}})
			}
		}
		out.Delivered.Audio = &dst
	}
	if p.VideoAction == "convert" || p.AudioAction == "convert" {
		out.EstimatedBitrateKbps = (p.VideoBitrateBPS + p.AudioBitrateBPS) / 1000
	} else if p.Trace.BitRate > 0 {
		out.EstimatedBitrateKbps = int(p.Trace.BitRate / 1000)
	}
	return out
}

// SetSubtitleTrack attaches the actual selected track's typed facts after the
// subtitle transaction, rather than retaining the pre-selection presentation.
func (p *PlaybackInfo) SetSubtitleTrack(parts *TrackLabelParts) {
	if p == nil {
		return
	}
	p.Source.Subtitles.Track = parts
	p.Delivered.Subtitles.Track = parts
}
