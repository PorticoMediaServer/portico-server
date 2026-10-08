package assets

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// StreamDetailVersion is bumped whenever inspect learns a new fact. Delivery
// planning refreshes an asset whose stored detail is older, so a library scanned
// before a fact existed gains it the first time the title is played.
const StreamDetailVersion = 1

// Dynamic range families. They name what a display has to be able to do, not the
// transfer function: Dolby Vision profile 8.1 is "dolby_vision" with an HDR10
// fallback, profile 5 is "dolby_vision" with none.
const (
	RangeSDR         = "sdr"
	RangeHDR10       = "hdr10"
	RangeHLG         = "hlg"
	RangeDolbyVision = "dolby_vision"
)

// StreamDetail is everything delivery planning needs beyond the codec name. Every
// field is optional: zero means "not observed", and planning treats an unobserved
// fact conservatively rather than as a match.
type StreamDetail struct {
	MaxBitRate int64 `json:"maxBitRate,omitempty"`
	HDR10Plus bool `json:"hdr10Plus,omitempty"`
	Profile       string  `json:"profile,omitempty"`
	Level         int     `json:"level,omitempty"`
	PixelFormat   string  `json:"pixelFormat,omitempty"`
	BitDepth      int     `json:"bitDepth,omitempty"`
	Width         int     `json:"width,omitempty"`
	Height        int     `json:"height,omitempty"`
	FrameRate     float64 `json:"frameRate,omitempty"`
	Interlaced    bool    `json:"interlaced,omitempty"`
	Rotation      int     `json:"rotation,omitempty"`
	Anamorphic    bool    `json:"anamorphic,omitempty"`
	RefFrames     int     `json:"refFrames,omitempty"`
	CodecTag      string  `json:"codecTag,omitempty"`
	BitRate       int64   `json:"bitRate,omitempty"`
	SampleRate    int     `json:"sampleRate,omitempty"`
	BitsPerSample int     `json:"bitsPerSample,omitempty"`
	// DynamicRange is one of the Range constants for video, empty for audio.
	DynamicRange string `json:"dynamicRange,omitempty"`
	// Dolby Vision configuration. Compatibility is dv_bl_signal_compatibility_id:
	// 0 none (profile 5), 1 HDR10, 2 SDR, 4 HLG. EnhancementLayer marks profile 7.
	DolbyVisionProfile       int  `json:"dolbyVisionProfile,omitempty"`
	DolbyVisionLevel         int  `json:"dolbyVisionLevel,omitempty"`
	DolbyVisionCompatibility int  `json:"dolbyVisionCompatibility,omitempty"`
	EnhancementLayer         bool `json:"enhancementLayer,omitempty"`
	// Object audio carried inside a channel-based codec: "atmos" or "dtsx".
	ObjectAudio string `json:"objectAudio,omitempty"`
}

// IsZero reports whether nothing at all was observed.
func (d StreamDetail) IsZero() bool { return d == StreamDetail{} }

// probeStreamDetail is the part of one ffprobe stream that detail is read from.
type probeStreamDetail struct {
	Profile        string            `json:"profile"`
	Level          int               `json:"level"`
	PixelFormat    string            `json:"pix_fmt"`
	BitsRaw        string            `json:"bits_per_raw_sample"`
	BitsPerSample  int               `json:"bits_per_sample"`
	RealFrameRate  string            `json:"r_frame_rate"`
	AvgFrameRate   string            `json:"avg_frame_rate"`
	FieldOrder     string            `json:"field_order"`
	SampleAspect   string            `json:"sample_aspect_ratio"`
	BitRate        string            `json:"bit_rate"`
	SampleRate     string            `json:"sample_rate"`
	Refs           int               `json:"refs"`
	CodecTag       string            `json:"codec_tag_string"`
	ColorTransfer  string            `json:"color_transfer"`
	ColorPrimaries string            `json:"color_primaries"`
	Tags           map[string]string `json:"tags"`
	SideData       []probeSideData   `json:"side_data_list"`
}

type probeSideData struct {
	Type          string      `json:"side_data_type"`
	Rotation      json.Number `json:"rotation"`
	Profile       int         `json:"dv_profile"`
	Level         int         `json:"dv_level"`
	Enhancement   int         `json:"el_present_flag"`
	Compatibility int         `json:"dv_bl_signal_compatibility_id"`
}

func probeRate(s string) float64 {
	a, b, ok := strings.Cut(s, "/")
	if !ok {
		return 0
	}
	n, e1 := strconv.ParseFloat(a, 64)
	d, e2 := strconv.ParseFloat(b, 64)
	if e1 != nil || e2 != nil || d <= 0 || n <= 0 {
		return 0
	}
	v := n / d
	if math.IsNaN(v) || math.IsInf(v, 0) || v > 1000 {
		return 0
	}
	return math.Round(v*1000) / 1000
}

// pixelBitDepth reads the depth out of an ffmpeg pixel format name. Names carry
// it as a trailing "10le", "12be" or a "p010"/"p016" family name; everything else
// in the formats a real file uses is eight bits.
func pixelBitDepth(format string) int {
	format = strings.ToLower(format)
	if format == "" {
		return 0
	}
	for _, depth := range []int{16, 14, 12, 10, 9} {
		d := strconv.Itoa(depth)
		if strings.HasSuffix(format, d+"le") || strings.HasSuffix(format, d+"be") || strings.HasPrefix(format, "p0"+d) || strings.HasPrefix(format, "p2"+d) || strings.HasPrefix(format, "p4"+d) {
			return depth
		}
	}
	return 8
}

// DynamicRangeOf classifies colour characteristics. A BT.2020 source without a
// PQ or HLG transfer is wide-gamut SDR and is deliberately not called HDR: tone
// mapping it as PQ crushes the picture.
func DynamicRangeOf(transfer string, dolbyVisionProfile int) string {
	if dolbyVisionProfile > 0 {
		return RangeDolbyVision
	}
	switch strings.ToLower(strings.TrimSpace(transfer)) {
	case "smpte2084":
		return RangeHDR10
	case "arib-std-b67":
		return RangeHLG
	}
	return RangeSDR
}

func objectAudio(codec, profile string) string {
	p := strings.ToLower(profile)
	switch {
	case strings.Contains(p, "atmos"):
		return "atmos"
	case strings.Contains(p, "dts:x"), strings.Contains(p, "dts-x"):
		return "dtsx"
	}
	_ = codec
	return ""
}

func streamDetail(kind, codec string, width, height int, s probeStreamDetail) StreamDetail {
	d := StreamDetail{Profile: streamText(s.Profile, 64), CodecTag: streamText(strings.Trim(s.CodecTag, "[]"), 16)}
	if s.Level > 0 && s.Level < 1000 {
		d.Level = s.Level
	}
	if rate, err := strconv.ParseInt(s.BitRate, 10, 64); err == nil && rate > 0 && rate < 1<<40 {
		d.BitRate = rate
	} else {
		// Matroska keeps the rate in statistics tags rather than the stream header.
		for key, value := range s.Tags {
			if upper := strings.ToUpper(key); upper == "BPS" || strings.HasPrefix(upper, "BPS-") {
				if rate, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && rate > 0 && rate < 1<<40 {
					d.BitRate = rate
					break
				}
			}
		}
	}
	switch kind {
	case "video":
		d.PixelFormat = streamText(s.PixelFormat, 32)
		if depth, err := strconv.Atoi(s.BitsRaw); err == nil && depth > 0 && depth <= 16 {
			d.BitDepth = depth
		} else {
			d.BitDepth = pixelBitDepth(s.PixelFormat)
		}
		if width > 0 && height > 0 && width <= 65536 && height <= 65536 {
			d.Width, d.Height = width, height
		}
		d.FrameRate = probeRate(s.AvgFrameRate)
		if d.FrameRate == 0 {
			d.FrameRate = probeRate(s.RealFrameRate)
		}
		switch strings.ToLower(s.FieldOrder) {
		case "tt", "bb", "tb", "bt":
			d.Interlaced = true
		}
		if a, b, ok := strings.Cut(s.SampleAspect, ":"); ok && a != b && a != "0" && b != "0" {
			d.Anamorphic = true
		}
		if s.Refs > 0 && s.Refs <= 64 {
			d.RefFrames = s.Refs
		}
		for _, side := range s.SideData {
			switch side.Type {
			case "Display Matrix":
				if r, err := side.Rotation.Float64(); err == nil && !math.IsNaN(r) && math.Abs(r) <= 360 {
					d.Rotation = ((int(math.Round(r)) % 360) + 360) % 360
				}
			case "DOVI configuration record":
				if side.Profile > 0 && side.Profile < 32 {
					d.DolbyVisionProfile, d.DolbyVisionLevel = side.Profile, side.Level
					d.DolbyVisionCompatibility = side.Compatibility
					d.EnhancementLayer = side.Enhancement != 0
				}
			}
		}
		d.DynamicRange = DynamicRangeOf(s.ColorTransfer, d.DolbyVisionProfile)
	case "audio":
		if rate, err := strconv.Atoi(s.SampleRate); err == nil && rate > 0 && rate <= 1<<22 {
			d.SampleRate = rate
		}
		if s.BitsPerSample > 0 && s.BitsPerSample <= 64 {
			d.BitsPerSample = s.BitsPerSample
		} else if depth, err := strconv.Atoi(s.BitsRaw); err == nil && depth > 0 && depth <= 64 {
			d.BitsPerSample = depth
		}
		d.ObjectAudio = objectAudio(codec, s.Profile)
	}
	return d
}
