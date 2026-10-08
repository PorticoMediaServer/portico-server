package playback

import (
	"fmt"
	"strings"

	"portico.local/server/internal/assets"
)

// Rejection codes. One is recorded for every rule that closed a route, so the
// trace of a plan reads as "why not the cheaper thing" without re-deriving it.
// They are wire strings: added to, never reworded.
const (
	RejectContainer           = "container_unsupported"
	RejectVideoCodec          = "video_codec_unsupported"
	RejectVideoProfile        = "video_profile_unsupported"
	RejectVideoLevel          = "video_level_exceeds_device"
	RejectVideoBitDepth       = "video_bit_depth_unsupported"
	RejectResolution          = "resolution_exceeds_device"
	RejectFrameRate           = "frame_rate_exceeds_device"
	RejectVideoBitrate        = "video_bitrate_exceeds_device"
	RejectInterlaced          = "interlaced_unsupported"
	RejectDynamicRange        = "dynamic_range_unsupported"
	RejectDolbyVision         = "dolby_vision_profile_unsupported"
	RejectDolbyVisionEntry    = "dolby_vision_sample_entry_unsupported"
	RejectRotation            = "rotation_requires_conversion"
	RejectAudioCodec          = "audio_codec_unsupported"
	RejectAudioChannels       = "audio_channels_exceed_device"
	RejectAudioSampleRate     = "audio_sample_rate_exceeds_device"
	RejectAudioTrack          = "audio_track_not_selectable_in_original"
	RejectTransport           = "transport_unsupported"
	RejectServerOutput        = "server_output_unavailable"
	RejectClientFailure       = "client_reported_failure"
	RejectOwnerRemuxDisabled  = "owner_remux_disabled"
	RejectTranscodingDisabled = "transcoding_disabled"
	RejectNoAudio             = "source_has_no_audio_to_convert"

	NoteDetailUnobserved   = "source_detail_unobserved"
	NoteBaselineProfile    = "client_published_no_profile"
	NoteToneMapOwnerOff    = "hdr_tone_mapping_disabled_by_owner"
	NoteDolbyVisionNoLayer = "dolby_vision_without_fallback_converted_approximately"
	NoteDeviceBitrate      = "device_bitrate_ceiling_applied"
	// NoteFirstAudioTrackPlayed marks a plan that plays the original although the
	// wanted audio track is not the first, because no other route existed.
	NoteFirstAudioTrackPlayed = "wanted_audio_track_unreachable_first_track_played"
)

type RouteRejection struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// RouteEvaluation is one route's verdict. A route is admissible when nothing
// rejected it; exactly one admissible route is chosen.
type RouteEvaluation struct {
	Route      string           `json:"route"`
	Transport  string           `json:"transport,omitempty"`
	Admissible bool             `json:"admissible"`
	Chosen     bool             `json:"chosen"`
	Rejections []RouteRejection `json:"rejections"`
}

type TraceVideo struct {
	OutputRange              string  `json:"outputRange,omitempty"`
	DolbyVisionLevel         int     `json:"dolbyVisionLevel,omitempty"`
	DolbyVisionCompatibility int     `json:"dolbyVisionCompatibility,omitempty"`
	Codec                    string  `json:"codec"`
	Profile                  string  `json:"profile,omitempty"`
	Level                    int     `json:"level,omitempty"`
	BitDepth                 int     `json:"bitDepth,omitempty"`
	Width                    int     `json:"width,omitempty"`
	Height                   int     `json:"height,omitempty"`
	FrameRate                float64 `json:"frameRate,omitempty"`
	Interlaced               bool    `json:"interlaced,omitempty"`
	DynamicRange             string  `json:"dynamicRange,omitempty"`
	DolbyVisionProfile       int     `json:"dolbyVisionProfile,omitempty"`
	BitRate                  int64   `json:"bitRate,omitempty"`
}

type TraceAudio struct {
	StreamIndex int    `json:"streamIndex"`
	Codec       string `json:"codec"`
	Channels    int    `json:"channels,omitempty"`
	Language    string `json:"language,omitempty"`
	ObjectAudio string `json:"objectAudio,omitempty"`
	// Why this track: explicit, preferred_language, default, first.
	ChosenBy string `json:"chosenBy"`
	Tracks   int    `json:"tracks"`
}

// DecisionTrace is the stored explanation of one plan. It names no path and no
// source identifier; it is safe to show to the viewer who owns the session.
type DecisionTrace struct {
	Version   int               `json:"version"`
	Client    ClientSummary     `json:"client"`
	Container string            `json:"container"`
	BitRate   int64             `json:"bitRate,omitempty"`
	Video     *TraceVideo       `json:"video,omitempty"`
	Audio     *TraceAudio       `json:"audio,omitempty"`
	Routes    []RouteEvaluation `json:"routes"`
	Notes     []string          `json:"notes"`
}

func containsString(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func containsInt(values []int, v int) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func profileName(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "_"))
}

// dolbyVisionFallback is the range a device without Dolby Vision sees when it
// decodes the base layer. Profile 5 has no such layer: its base is IPT-PQ-C2,
// which an HDR10 pipeline shows in the wrong colours.
func dolbyVisionFallback(v *SourceVideo) string {
	switch v.DolbyVisionCompatibility {
	case 1, 6:
		return assets.RangeHDR10
	case 4:
		return assets.RangeHLG
	case 2:
		return assets.RangeSDR
	}
	if v.DolbyVisionProfile == 7 {
		return assets.RangeHDR10
	}
	return ""
}

// effectiveRange is the dynamic range the device will actually be shown for this
// source: Dolby Vision where it has the profile, the fallback layer otherwise.
func effectiveRange(v *SourceVideo, c *ClientVideoCodec) string {
	r := v.DynamicRange
	if r == "" {
		return assets.RangeSDR
	}
	if r == assets.RangeDolbyVision && (c == nil || !containsInt(c.DolbyVisionProfiles, v.DolbyVisionProfile)) {
		return dolbyVisionFallback(v)
	}
	return r
}

// videoRejections checks one source video stream against one transport of the
// device. copying is true for the HLS copy routes, where rotation metadata and
// the Dolby Vision sample entry do not survive the repackaging the same way.
func videoRejections(v *SourceVideo, client ClientProfile, t ClientTransport, allowHDR, copying bool) []RouteRejection {
	var out []RouteRejection
	c := client.videoCodec(v.Codec)
	if c == nil || !containsString(t.Video, v.Codec) {
		return append(out, RouteRejection{RejectVideoCodec, v.Codec})
	}
	if len(c.Profiles) > 0 && v.Profile != "" && !containsString(c.Profiles, profileName(v.Profile)) {
		out = append(out, RouteRejection{RejectVideoProfile, v.Profile})
	}
	if c.MaxLevel > 0 && v.Level > c.MaxLevel {
		out = append(out, RouteRejection{RejectVideoLevel, fmt.Sprintf("%d>%d", v.Level, c.MaxLevel)})
	}
	if len(c.BitDepths) > 0 && v.BitDepth > 0 && !containsInt(c.BitDepths, v.BitDepth) {
		out = append(out, RouteRejection{RejectVideoBitDepth, fmt.Sprintf("%d-bit", v.BitDepth)})
	}
	if (c.MaxWidth > 0 && v.Width > c.MaxWidth) || (c.MaxHeight > 0 && v.Height > c.MaxHeight) {
		out = append(out, RouteRejection{RejectResolution, fmt.Sprintf("%dx%d", v.Width, v.Height)})
	}
	if c.MaxFrameRate > 0 && v.FrameRate > c.MaxFrameRate+0.01 {
		out = append(out, RouteRejection{RejectFrameRate, fmt.Sprintf("%.3f", v.FrameRate)})
	}
	if c.MaxBitrateBPS > 0 && max(v.BitRate, v.MaxBitRate) > int64(c.MaxBitrateBPS) {
		out = append(out, RouteRejection{RejectVideoBitrate, fmt.Sprintf("%d", max(v.BitRate, v.MaxBitRate))})
	}
	if v.Interlaced && !c.Interlaced {
		out = append(out, RouteRejection{RejectInterlaced, ""})
	}
	if copying && v.Rotation != 0 {
		out = append(out, RouteRejection{RejectRotation, fmt.Sprintf("%d", v.Rotation)})
	}
	if v.DynamicRange != "" && v.DynamicRange != assets.RangeSDR {
		native := v.DynamicRange == assets.RangeDolbyVision && containsInt(c.DolbyVisionProfiles, v.DolbyVisionProfile)
		shown := effectiveRange(v, c)
		switch {
		case !allowHDR && shown != assets.RangeSDR:
			out = append(out, RouteRejection{ReasonHDRNotAllowed, v.DynamicRange})
		case shown == "":
			out = append(out, RouteRejection{RejectDolbyVision, fmt.Sprintf("profile %d", v.DolbyVisionProfile)})
		case shown != assets.RangeSDR && !containsString(c.DynamicRanges, shown):
			out = append(out, RouteRejection{RejectDynamicRange, shown})
		case !copying && !native && v.DynamicRange == assets.RangeDolbyVision && (v.CodecTag == "dvh1" || v.CodecTag == "dvhe" || v.CodecTag == "dva1" || v.CodecTag == "dvav"):
			// A Dolby Vision sample entry is refused outright by decoders that do
			// not know it, even though the base layer underneath is ordinary HEVC.
			out = append(out, RouteRejection{RejectDolbyVisionEntry, v.CodecTag})
		}
	}
	return out
}

func audioRejections(a *SourceAudio, client ClientProfile, t ClientTransport) []RouteRejection {
	var out []RouteRejection
	c := client.audioCodec(a.Codec)
	if c == nil || !containsString(t.Audio, a.Codec) {
		return append(out, RouteRejection{RejectAudioCodec, a.Codec})
	}
	if c.Passthrough && !passthroughRoute(client.AudioOutput.Route) {
		out = append(out, RouteRejection{RejectAudioCodec, "passthrough output unavailable"})
	}
	if c.MaxChannels > 0 && a.Channels > c.MaxChannels {
		out = append(out, RouteRejection{RejectAudioChannels, fmt.Sprintf("%d>%d", a.Channels, c.MaxChannels)})
	}
	if c.MaxSampleRate > 0 && a.SampleRate > c.MaxSampleRate {
		out = append(out, RouteRejection{RejectAudioSampleRate, fmt.Sprintf("%d", a.SampleRate)})
	}
	return out
}

// chooseAudio picks the programme audio for a session: the viewer's explicit
// choice, else the first track in a preferred language, else the container's
// default, else the first track. A commentary is never chosen automatically
// while an ordinary track exists.
func chooseAudio(tracks []SourceAudio, explicit int, preferred []string) (*SourceAudio, string) {
	if len(tracks) == 0 {
		return nil, ""
	}
	if explicit >= 0 {
		for i := range tracks {
			if tracks[i].Index == explicit {
				return &tracks[i], "explicit"
			}
		}
	}
	for _, want := range preferred {
		want = languageKey(want)
		if want == "" {
			continue
		}
		for pass := 0; pass < 2; pass++ {
			for i := range tracks {
				if languageKey(tracks[i].Language) == want && (pass == 1 || !tracks[i].Commentary) {
					return &tracks[i], "preferred_language"
				}
			}
		}
	}
	for i := range tracks {
		if tracks[i].Default && !tracks[i].Commentary {
			return &tracks[i], "default"
		}
	}
	for i := range tracks {
		if !tracks[i].Commentary {
			return &tracks[i], "first"
		}
	}
	return &tracks[0], "first"
}

// languageKey folds the spellings a file and a preference use for one language:
// ISO 639-1, 639-2/B, 639-2/T and a BCP 47 tag with a region all compare equal.
func languageKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, "-_"); i > 0 {
		s = s[:i]
	}
	if len(s) == 3 {
		if two, ok := iso639Three[s]; ok {
			return two
		}
	}
	if s == "und" || s == "unknown" {
		return ""
	}
	return s
}

var iso639Three = map[string]string{
	"eng": "en", "fre": "fr", "fra": "fr", "ger": "de", "deu": "de", "spa": "es", "ita": "it", "jpn": "ja", "kor": "ko",
	"chi": "zh", "zho": "zh", "por": "pt", "rus": "ru", "dut": "nl", "nld": "nl", "swe": "sv", "nor": "no", "nob": "no",
	"dan": "da", "fin": "fi", "pol": "pl", "tur": "tr", "ara": "ar", "heb": "he", "hin": "hi", "tha": "th", "vie": "vi",
	"cze": "cs", "ces": "cs", "gre": "el", "ell": "el", "hun": "hu", "rum": "ro", "ron": "ro", "ukr": "uk", "ind": "id",
	"may": "ms", "msa": "ms", "cat": "ca", "hrv": "hr", "srp": "sr", "slo": "sk", "slk": "sk", "slv": "sl", "bul": "bg",
	"est": "et", "lav": "lv", "lit": "lt", "ice": "is", "isl": "is", "per": "fa", "fas": "fa", "ben": "bn", "tam": "ta",
	"tel": "te", "urd": "ur", "fil": "tl", "tgl": "tl",
}

// Server output limits. They describe what this server's producers can write,
// independent of any device: which codecs can be copied into each HLS container.
var (
	tsCopyVideo   = []string{"h264", "hevc", "mpeg2video"}
	tsCopyAudio   = []string{"aac", "mp3", "mp2", "ac3", "eac3"}
	fmp4CopyVideo = []string{"h264", "hevc", "av1", "vp9"}
	fmp4CopyAudio = []string{"aac", "mp3", "ac3", "eac3", "flac", "opus", "alac"}
)

// hlsTransports lists the device's HLS transports this server can produce,
// MPEG-TS first: it is the older, more forgiving container, and fragmented MP4
// is reached for when TS cannot carry the stream to this device.
func hlsTransports(client ClientProfile, fmp4 bool) []ClientTransport {
	var out []ClientTransport
	for _, kind := range []string{TransportHLSTS, TransportHLSFMP4} {
		if kind == TransportHLSFMP4 && !fmp4 {
			continue
		}
		for _, t := range client.Transports {
			if t.Transport == kind {
				out = append(out, t)
			}
		}
	}
	return out
}

func serverCopyVideo(transport, codec string) bool {
	if transport == TransportHLSFMP4 {
		return containsString(fmp4CopyVideo, codec)
	}
	return containsString(tsCopyVideo, codec)
}

func serverCopyAudio(transport, codec string) bool {
	if transport == TransportHLSFMP4 {
		return containsString(fmp4CopyAudio, codec)
	}
	return containsString(tsCopyAudio, codec)
}

// convertedAudio is the target of an audio conversion for one device: AAC, in
// 5.1 when the source has it and both the decoder and the output route take it,
// stereo otherwise.
type convertedAudio struct {
	Codec    string
	Channels int
	Bitrate  int
	Downmix  bool
}

func audioConversionTarget(a *SourceAudio, client ClientProfile, target QualityTarget) convertedAudio {
	out := convertedAudio{Codec: "aac", Channels: 2, Bitrate: audioBudget(target)}
	source := 2
	if a != nil && a.Channels > 0 {
		source = a.Channels
	}
	if source >= 6 {
		aac := client.audioCodec("aac")
		if aac != nil && aac.MaxChannels >= 6 && client.AudioOutput.MaxChannels >= 6 && (target.MaxAudioBitrateBPS == 0 || target.MaxAudioBitrateBPS >= 256_000) {
			out.Channels, out.Bitrate = 6, 384_000
			if target.MaxAudioBitrateBPS > 0 && target.MaxAudioBitrateBPS < out.Bitrate {
				out.Bitrate = target.MaxAudioBitrateBPS
			}
		}
	}
	if source >= 6 && out.Channels < 6 && passthroughRoute(client.AudioOutput.Route) && client.AudioOutput.MaxChannels >= 6 && (target.MaxAudioBitrateBPS == 0 || target.MaxAudioBitrateBPS >= 640000) {
		for _, codec := range []string{"eac3", "ac3"} {
			if c := client.audioCodec(codec); c != nil && c.Passthrough && c.MaxChannels >= 6 {
				out.Codec, out.Channels, out.Bitrate = codec, 6, 640000
				break
			}
		}
	}
	out.Downmix = source > out.Channels
	return out
}

func passthroughRoute(route string) bool {
	return route == "hdmi" || route == "arc" || route == "optical"
}

// HLSTSRejections shares on-demand codec, range and output-route checks with Live TV.
func HLSTSRejections(video *SourceVideo, audio *SourceAudio, client ClientProfile) ([]RouteRejection, []RouteRejection) {
	if client.Version == 0 {
		client = BaselineClientProfile()
	}
	t := transportNamed(client.Transports, TransportHLSTS)
	if t == nil {
		return []RouteRejection{{RejectTransport, TransportHLSTS}}, []RouteRejection{{RejectTransport, TransportHLSTS}}
	}
	var vr, ar []RouteRejection
	if video != nil {
		vr = videoRejections(video, client, *t, true, true)
	}
	if audio != nil {
		ar = audioRejections(audio, client, *t)
	}
	return vr, ar
}
