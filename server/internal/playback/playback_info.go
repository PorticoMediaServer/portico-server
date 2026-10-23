package playback

import "portico.local/server/internal/assets"

// PlaybackInfo is an allowlisted explanation. It is deliberately separate from
// Presentation: no source IDs, filenames, URLs, grants, device paths or tokens.
// Encoded output facts describe the committed target; they are not observations
// of a physical display or receiver. Actual client output may be reported later.
type PictureFacts struct {
	Codec                      string  `json:"codec"`
	Profile                    string  `json:"profile,omitempty"`
	BitDepth                   int     `json:"bitDepth,omitempty"`
	Width                      int     `json:"width,omitempty"`
	Height                     int     `json:"height,omitempty"`
	FrameRate                  float64 `json:"frameRate,omitempty"`
	FrameRateNumerator         int     `json:"frameRateNumerator,omitempty"`
	FrameRateDenominator       int     `json:"frameRateDenominator,omitempty"`
	FrameRateMode              string  `json:"frameRateMode,omitempty"`
	DynamicRange               string  `json:"dynamicRange,omitempty"`
	DolbyVisionProfile         int     `json:"dolbyVisionProfile,omitempty"`
	DolbyVisionEnhancementType string  `json:"dolbyVisionEnhancementType,omitempty"`
	HDR10Plus                  bool    `json:"hdr10Plus"`
}
type SoundFacts struct {
	Codec       string `json:"codec"`
	Variant     string `json:"variant,omitempty"`
	ObjectAudio string `json:"objectAudio,omitempty"`
	Channels    int    `json:"channels,omitempty"`
	Layout      string `json:"layout,omitempty"`
	SampleRate  int    `json:"sampleRate,omitempty"`
	BitDepth    int    `json:"bitDepth,omitempty"`
	Handling    string `json:"handling"`
}
type PlaybackMediaFacts struct {
	Video             *PictureFacts `json:"video,omitempty"`
	Audio             *SoundFacts   `json:"audio,omitempty"`
	SubtitleRendering string        `json:"subtitleRendering"`
}
type PlaybackLoss struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type PlaybackInfo struct {
	Route       string             `json:"route"`
	Reason      string             `json:"reason"`
	ReasonCodes []string           `json:"reasonCodes"`
	Source      PlaybackMediaFacts `json:"source"`
	Delivered   PlaybackMediaFacts `json:"delivered"`
	Losses      []PlaybackLoss     `json:"losses"`
	Basis       string             `json:"basis"`
}

const reasonObjectAudioLost = "object_audio_not_preserved"
const reasonHDR10PlusNotApplied = "hdr10plus_not_applied"

func PlaybackInfoForPlan(p DeliveryPlan, subtitle string) *PlaybackInfo {
	out := &PlaybackInfo{Route: "Original", Reason: "The device can play the source as it is.", ReasonCodes: append([]string{}, p.ReasonCodes...), Losses: []PlaybackLoss{}, Basis: "committed_delivery_plan"}
	out.Source.SubtitleRendering = "source"
	out.Delivered.SubtitleRendering = subtitle
	if subtitle == "" {
		out.Delivered.SubtitleRendering = "off"
	}
	switch p.Strategy {
	case DeliveryCopyRemux:
		out.Route = "Repackaged"
		out.Reason = "The server repackaged the streams for this device."
	case DeliveryAudioConversion:
		out.Route = "Converted"
		out.Reason = "The device needs a different sound format."
	case DeliveryVideoConversion:
		out.Route = "Converted"
		out.Reason = "The picture was converted to fit the device, subtitles or quality limit."
	}
	if p.Trace == nil {
		return out
	}
	if v := p.Trace.Video; v != nil {
		src := PictureFacts{Codec: v.Codec, Profile: v.Profile, BitDepth: v.BitDepth, Width: v.Width, Height: v.Height, FrameRate: v.FrameRate, FrameRateNumerator: v.Breadth.FrameRateNumerator, FrameRateDenominator: v.Breadth.FrameRateDenominator, FrameRateMode: v.Breadth.FrameRateMode, DynamicRange: v.DynamicRange, DolbyVisionProfile: v.DolbyVisionProfile, DolbyVisionEnhancementType: v.Breadth.DolbyVisionEnhancementType, HDR10Plus: v.HDR10Plus}
		out.Source.Video = &src
		dst := src
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
			dst.DolbyVisionEnhancementType = ""
			if p.ConversionTarget != nil {
				dst.Profile = p.ConversionTarget.VideoProfile
				dst.BitDepth = p.ConversionTarget.BitDepth
				dst.DynamicRange = p.ConversionTarget.DynamicRange
			}
			if p.MaxFrameRate > 0 && p.MaxFrameRate < dst.FrameRate {
				dst.FrameRate = p.MaxFrameRate
				dst.FrameRateNumerator = 0
				dst.FrameRateDenominator = 0
				dst.FrameRateMode = "constant"
			}
		}
		if dst.DynamicRange != assets.RangeDolbyVision {
			dst.DolbyVisionProfile = 0
			dst.DolbyVisionEnhancementType = ""
		}
		if src.DynamicRange == assets.RangeDolbyVision && dst.DynamicRange != assets.RangeDolbyVision {
			out.Losses = append(out.Losses, PlaybackLoss{"dolby_vision_lost", "Dolby Vision is not available on this route."})
		}
		if src.DynamicRange != "" && src.DynamicRange != assets.RangeSDR && dst.DynamicRange == assets.RangeSDR {
			out.Losses = append(out.Losses, PlaybackLoss{"hdr_lost", "The picture is being delivered in standard dynamic range."})
		}
		if src.HDR10Plus && (!dst.HDR10Plus || containsString(p.ReasonCodes, reasonHDR10PlusNotApplied)) {
			out.Losses = append(out.Losses, PlaybackLoss{"hdr10plus_lost", "HDR10+ dynamic metadata is not applied on this route."})
		}
		out.Delivered.Video = &dst
	}
	if a := p.Trace.Audio; a != nil {
		src := SoundFacts{Codec: a.Codec, Variant: a.Variant, ObjectAudio: a.ObjectAudio, Channels: a.Channels, Layout: a.Layout, SampleRate: a.SampleRate, BitDepth: a.BitDepth, Handling: "source"}
		out.Source.Audio = &src
		dst := src
		dst.Codec = p.OutputAudioCodec
		dst.Handling = "device_decode"
		if p.AudioPassthrough {
			dst.Handling = "passthrough"
		}
		if p.AudioAction == "convert" {
			dst.Variant = ""
			dst.ObjectAudio = ""
			dst.Channels = p.AudioChannels
			dst.Layout = ""
			dst.SampleRate = p.AudioSampleRate
			dst.BitDepth = 0
			dst.Handling = "server_decode"
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
			message := "Object audio is not available on this device. Playing the channel-based track."
			if src.ObjectAudio == "atmos" {
				message = "Atmos is not available on this device. Playing the channel-based track."
			}
			out.Losses = append(out.Losses, PlaybackLoss{"object_audio_lost", message})
		}
		out.Delivered.Audio = &dst
	}
	return out
}
