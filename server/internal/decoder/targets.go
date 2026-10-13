package decoder

import (
	"errors"
	"math"
	"regexp"
	"strconv"
)

// ConversionTarget is the planner's complete output contract. Empty packaging
// selects MPEG-TS for H.264/AAC/AC-3 and fragmented MP4 for the other codecs.
// Quality mode preserves source dimensions and uses a quality target rather
// than a fixed ladder rate. BitrateBPS, when supplied, still caps that mode.
// Copy actions remain on ConversionRequest; a copy target names source codecs.
type ConversionTarget struct {
	VideoCodec               string  `json:"videoCodec"`
	BitDepth                 int     `json:"bitDepth"`
	DynamicRange             string  `json:"dynamicRange"`
	ToneMap                  bool    `json:"toneMap"`
	Width                    int     `json:"width"`
	Height                   int     `json:"height"`
	FrameRate                float64 `json:"frameRate"`
	VideoBitrateBPS          int     `json:"videoBitrateBps"`
	RateControl              string  `json:"rateControl"`
	Quality                  int     `json:"quality"`
	AudioCodec               string  `json:"audioCodec"`
	AudioChannels            int     `json:"audioChannels"`
	AudioChannelLayout       string  `json:"audioChannelLayout"`
	AudioBitrateBPS          int     `json:"audioBitrateBps"`
	AudioSampleRate          int     `json:"audioSampleRate"`
	Packaging                string  `json:"packaging"`
	SourceRange              string  `json:"sourceRange"`
	DolbyVisionProfile       int     `json:"dolbyVisionProfile"`
	DolbyVisionCompatibility int     `json:"dolbyVisionCompatibility"`
	MasterDisplay            string  `json:"masterDisplay,omitempty"`
	MaxCLL                   string  `json:"maxCLL,omitempty"`
}

var masteringDisplayPattern = regexp.MustCompile(`^G\([0-9]{1,5},[0-9]{1,5}\)B\([0-9]{1,5},[0-9]{1,5}\)R\([0-9]{1,5},[0-9]{1,5}\)WP\([0-9]{1,5},[0-9]{1,5}\)L\([0-9]{1,10},[0-9]{1,10}\)$`)
var maxCLLPattern = regexp.MustCompile(`^[0-9]{1,5},[0-9]{1,5}$`)

func (t ConversionTarget) Normalized() (ConversionTarget, error) {
	if t.VideoCodec == "" {
		t.VideoCodec = "h264"
	}
	if t.BitDepth == 0 {
		t.BitDepth = 8
	}
	if t.DynamicRange == "" {
		t.DynamicRange = "sdr"
	}
	if t.AudioCodec == "" {
		t.AudioCodec = "aac"
	}
	if t.AudioChannels == 0 {
		t.AudioChannels = 2
	}
	if t.RateControl == "" {
		t.RateControl = "bitrate"
	}
	if t.Packaging == "" {
		t.Packaging = "mpegts_hls"
		if t.VideoCodec != "h264" || t.AudioCodec == "flac" || t.AudioCodec == "opus" || t.AudioCodec == "truehd" {
			t.Packaging = "fmp4_hls"
		}
	}
	if t.VideoCodec != "h264" && t.VideoCodec != "hevc" && t.VideoCodec != "av1" {
		return t, ErrInvalidConfiguration
	}
	if t.BitDepth != 8 && t.BitDepth != 10 || t.VideoCodec == "h264" && t.BitDepth != 8 {
		return t, ErrInvalidConfiguration
	}
	if t.DynamicRange != "sdr" && t.DynamicRange != "hdr10" && t.DynamicRange != "hlg" {
		return t, ErrInvalidConfiguration
	}
	if t.DynamicRange != "sdr" && (t.BitDepth != 10 || t.ToneMap || t.VideoCodec == "h264") {
		return t, ErrInvalidConfiguration
	}
	if t.SourceRange != "" && t.SourceRange != "sdr" && t.SourceRange != "hdr10" && t.SourceRange != "hdr10_plus" && t.SourceRange != "hlg" && t.SourceRange != "dolby_vision" {
		return t, ErrInvalidConfiguration
	}
	if t.SourceRange != "" && t.SourceRange != "sdr" && t.DynamicRange == "sdr" && !t.ToneMap {
		return t, errors.New("hdr_requires_tone_mapping")
	}
	if t.RateControl != "bitrate" && t.RateControl != "quality" || t.Quality < 0 || t.Quality > 51 {
		return t, ErrInvalidConfiguration
	}
	if t.Width < 0 || t.Width > 32768 || t.Height < 0 || t.Height > 4320 || t.VideoBitrateBPS < 0 || t.VideoBitrateBPS > 200_000_000 || t.AudioBitrateBPS < 0 || t.AudioBitrateBPS > 4_096_000 || math.IsNaN(t.FrameRate) || math.IsInf(t.FrameRate, 0) || t.FrameRate < 0 || t.FrameRate > 240 {
		return t, ErrInvalidConfiguration
	}
	if t.AudioChannels != 1 && t.AudioChannels != 2 && t.AudioChannels != 6 && t.AudioChannels != 8 {
		return t, ErrInvalidConfiguration
	}
	switch t.AudioCodec {
	case "aac", "ac3", "eac3", "opus", "flac", "dts", "dts_hd", "truehd", "mp3", "mp2":
	default:
		return t, ErrInvalidConfiguration
	}
	if t.AudioSampleRate < 0 || t.AudioSampleRate > 192000 {
		return t, ErrInvalidConfiguration
	}
	if t.AudioChannelLayout == "" {
		t.AudioChannelLayout = map[int]string{1: "mono", 2: "stereo", 6: "5.1", 8: "7.1"}[t.AudioChannels]
	}
	if t.AudioChannelLayout != map[int]string{1: "mono", 2: "stereo", 6: "5.1", 8: "7.1"}[t.AudioChannels] && !(t.AudioChannels == 6 && t.AudioChannelLayout == "5.1(side)") {
		return t, ErrInvalidConfiguration
	}
	if t.Packaging != "mpegts_hls" && t.Packaging != "fmp4_hls" {
		return t, ErrInvalidConfiguration
	}
	if t.Packaging == "mpegts_hls" && (t.VideoCodec != "h264" || t.AudioCodec == "flac" || t.AudioCodec == "opus" || t.AudioCodec == "truehd") {
		return t, errors.New("codec_requires_fragmented_mp4")
	}
	if t.MasterDisplay != "" && !masteringDisplayPattern.MatchString(t.MasterDisplay) || t.MaxCLL != "" && !maxCLLPattern.MatchString(t.MaxCLL) {
		return t, ErrInvalidConfiguration
	}
	if t.DolbyVisionProfile < 0 || t.DolbyVisionProfile > 9 || t.DolbyVisionCompatibility < 0 || t.DolbyVisionCompatibility > 6 {
		return t, ErrInvalidConfiguration
	}
	return t, nil
}

// CodecVideoBitrate supplies deliberately conservative ladders. It never changes
// a viewer's selected codec, range or dimensions in response to server load.
func CodecVideoBitrate(codec string, height int) int {
	rate := 35_000_000
	switch {
	case height <= 480:
		rate = 1_500_000
	case height <= 720:
		rate = 3_000_000
	case height <= 1080:
		rate = 8_000_000
	case height <= 1440:
		rate = 14_000_000
	}
	if codec == "hevc" {
		rate = rate * 7 / 10
	}
	if codec == "av1" {
		rate = rate * 6 / 10
	}
	return rate
}

func softwareVideoEncoder(codec string, f *ToolchainFacts) (string, error) {
	candidates := map[string][]string{"h264": {"libx264"}, "hevc": {"libx265"}, "av1": {"libsvtav1", "libaom-av1", "librav1e"}}[codec]
	for _, encoder := range candidates {
		if f == nil && codec != "av1" || f != nil && f.Encoders[encoder] {
			return encoder, nil
		}
	}
	return "", errors.New("video_encoder_unavailable")
}

func videoColorArgs(t ConversionTarget) []string {
	primaries, transfer, matrix := "bt709", "bt709", "bt709"
	if t.DynamicRange != "sdr" {
		primaries, matrix = "bt2020", "bt2020nc"
		transfer = "smpte2084"
		if t.DynamicRange == "hlg" {
			transfer = "arib-std-b67"
		}
	}
	return []string{"-color_primaries", primaries, "-color_trc", transfer, "-colorspace", matrix, "-color_range", "tv"}
}

func videoRateArgs(t ConversionTarget, encoder, preset string) []string {
	quality := t.Quality
	if quality == 0 {
		quality = 20
		if t.VideoCodec == "av1" {
			quality = 28
		}
	}
	out := []string{}
	switch encoder {
	case "libx264", "libx265":
		out = append(out, "-preset", preset)
	case "libsvtav1":
		out = append(out, "-preset", "8", "-svtav1-params", "lp=2")
	case "libaom-av1":
		out = append(out, "-cpu-used", "6", "-row-mt", "1")
	case "librav1e":
		out = append(out, "-speed", "8")
	}
	if t.RateControl == "quality" {
		switch {
		case encoder == "librav1e":
			out = append(out, "-qp", strconv.Itoa(quality*5), "-b:v", "0")
		case encoder == "libaom-av1" || encoder == "libsvtav1":
			out = append(out, "-crf", strconv.Itoa(quality), "-b:v", "0")
		default:
			out = append(out, "-crf", strconv.Itoa(quality))
		}
		if t.VideoBitrateBPS > 0 {
			out = append(out, "-maxrate", strconv.Itoa(t.VideoBitrateBPS), "-bufsize", strconv.Itoa(t.VideoBitrateBPS*2))
		}
	} else {
		rate := t.VideoBitrateBPS
		if rate == 0 {
			height := t.Height
			if height == 0 {
				height = 1080
			}
			rate = CodecVideoBitrate(t.VideoCodec, height)
		}
		out = append(out, "-b:v", strconv.Itoa(rate), "-maxrate", strconv.Itoa(rate), "-bufsize", strconv.Itoa(rate*2))
	}
	return out
}
