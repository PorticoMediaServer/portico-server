package playback

import (
	"strings"
	"sync/atomic"

	"portico.local/server/internal/assets"
)

// ConversionVideoOutput is one picture format this server's converter can
// produce. Ranges are the dynamic ranges it can write: "sdr" (a standard
// picture, tone-mapped from an HDR source) and any HDR range it keeps.
type ConversionVideoOutput struct {
	Codec    string   `json:"codec"`
	BitDepth int      `json:"bitDepth"`
	Ranges   []string `json:"ranges"`
	// Transports are the HLS containers the converted picture can be written
	// into, in order of preference.
	Transports []string `json:"transports"`
}

// ConversionAudioOutput is one audio format this server's converter can
// produce, with the channel counts it can write and the rate for each.
type ConversionAudioOutput struct {
	Codec    string `json:"codec"`
	Lossless bool   `json:"lossless,omitempty"`
	// Bitrates maps a channel count to the rate it is encoded at when no
	// ceiling applies; a lossless output maps each count to zero.
	Bitrates   map[int]int `json:"bitrates"`
	Transports []string    `json:"transports"`
}

// ConversionOutputs is everything the converter can be asked for. The planner
// chooses among these against the device's capability document; it never plans
// an output that is not listed. Video is in the server's order of preference.
type ConversionOutputs struct {
	Video []ConversionVideoOutput `json:"video"`
	Audio []ConversionAudioOutput `json:"audio"`
}

// DefaultConversionOutputs is what the converter produces today: an 8-bit
// standard-range H.264 picture, AAC in stereo or 5.1, and AC-3 or E-AC-3 in 5.1.
func DefaultConversionOutputs() ConversionOutputs {
	hls := []string{TransportHLSTS, TransportHLSFMP4}
	return ConversionOutputs{
		Video: []ConversionVideoOutput{{Codec: "h264", BitDepth: 8, Ranges: []string{assets.RangeSDR}, Transports: hls}},
		Audio: []ConversionAudioOutput{
			{Codec: "aac", Bitrates: map[int]int{2: 192_000, 6: 384_000}, Transports: hls},
			{Codec: "eac3", Bitrates: map[int]int{6: 640_000}, Transports: hls},
			{Codec: "ac3", Bitrates: map[int]int{6: 640_000}, Transports: hls},
		},
	}
}

var conversionOutputs atomic.Pointer[ConversionOutputs]

// SetConversionOutputs replaces what the planner may ask the converter for.
// The composition root calls it once the converter's own abilities are known
// (HEVC and HDR-preserving pictures, lossless and wider audio).
func SetConversionOutputs(o ConversionOutputs) {
	if len(o.Video) == 0 || len(o.Audio) == 0 {
		return
	}
	conversionOutputs.Store(&o)
}

func currentConversionOutputs() ConversionOutputs {
	if o := conversionOutputs.Load(); o != nil {
		return *o
	}
	return DefaultConversionOutputs()
}

func (o ConversionOutputs) audio(codec string) *ConversionAudioOutput {
	for i := range o.Audio {
		if o.Audio[i].Codec == codec {
			return &o.Audio[i]
		}
	}
	return nil
}

// videoOutput is the picture a conversion is planned to produce.
type videoOutput struct {
	Codec     string
	BitDepth  int
	Range     string
	Transport string
}

// keptRange is the HDR range a conversion could keep for this source on this
// device: the range the device would be shown from the original, when it is
// HDR10 or HLG. Dolby Vision is kept through its base layer.
func keptRange(v *SourceVideo, client ClientProfile, codec *ClientVideoCodec) string {
	if v == nil || v.DynamicRange == "" || v.DynamicRange == assets.RangeSDR {
		return ""
	}
	r := v.DynamicRange
	if r == assets.RangeDolbyVision {
		r = dolbyVisionFallback(v)
	}
	if r != assets.RangeHDR10 && r != assets.RangeHLG {
		return ""
	}
	if codec == nil || !containsString(codec.DynamicRanges, r) || !client.presents(r) {
		return ""
	}
	return r
}

// chooseVideoOutput picks the picture a conversion produces for this device:
// the first of the server's outputs the engine takes on one of its HLS
// transports, keeping the source's HDR where both sides can. ok is false when
// no output of this server reaches the device.
func chooseVideoOutput(v *SourceVideo, client ClientProfile, transports []ClientTransport, allowHDR bool, outputs ConversionOutputs) (videoOutput, bool) {
	for _, o := range outputs.Video {
		codec := client.videoCodec(o.Codec)
		if codec == nil || (len(codec.BitDepths) > 0 && !containsInt(codec.BitDepths, o.BitDepth)) {
			continue
		}
		ranges := []string{}
		if kept := keptRange(v, client, codec); kept != "" && allowHDR && containsString(o.Ranges, kept) && o.BitDepth >= 10 {
			ranges = append(ranges, kept)
		}
		if containsString(o.Ranges, assets.RangeSDR) {
			ranges = append(ranges, assets.RangeSDR)
		}
		for _, r := range ranges {
			for _, name := range o.Transports {
				for _, t := range transports {
					if t.Transport == name && containsString(t.Video, o.Codec) {
						return videoOutput{Codec: o.Codec, BitDepth: o.BitDepth, Range: r, Transport: name}, true
					}
				}
			}
		}
	}
	return videoOutput{}, false
}

// losslessAudio reports whether a source track is losslessly coded, so that
// converting it to a lossy codec gives something up.
func losslessAudio(a *SourceAudio) bool {
	if a == nil {
		return false
	}
	switch a.Codec {
	case "flac", "alac", "truehd", "pcm", "mlp", "wavpack", "ape", "tta":
		return true
	case "dts":
		profile := strings.ToLower(a.Profile)
		return strings.Contains(profile, "ma") && strings.Contains(profile, "hd")
	}
	return false
}
