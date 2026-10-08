// Package playbackv1 is the resource layer of Playback Protocol v1 (Spec — Playback
// Protocol v1 §3–§16): device capabilities, playback options, sessions, timeline,
// queues, device commands and transfers, groups and the playback event topics. It
// owns resources, revisions, leases and idempotency; bytes are produced by the
// existing delivery machinery in package playback, one presentation per session
// generation.
package playbackv1

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"portico.local/server/internal/playback"
)

// Capabilities is a device's capability profile (spec §3). Lists are in order
// of preference. Unknown values are kept and simply never match (tolerant
// reader); unknown fields are refused, like every v1 request.
type Capabilities struct {
	Form       string         `json:"form"`
	Network    CapNetwork     `json:"network"`
	Containers []CapContainer `json:"containers"`
	Streaming  CapStreaming   `json:"streaming"`
	Video      []CapVideo     `json:"video"`
	Display    *CapDisplay    `json:"display,omitempty"`
	Audio      []CapAudio     `json:"audio"`
	// AudioDecode is what the client's own audio engine decodes (spec §3, §18.1):
	// direct plays of those files, conversion to FLAC or Opus for the rest.
	AudioDecode []CapAudioDecode `json:"audioDecode,omitempty"`
	Subtitles   []CapSubtitle    `json:"subtitles"`
	Features    map[string]bool  `json:"features"`
}
type CapNetwork struct {
	Class          string `json:"class"`
	MaxBitrateKbps int    `json:"maxBitrateKbps,omitempty"`
}
type CapContainer struct {
	Container string `json:"container"`
	Direct    bool   `json:"direct"`
}
type CapHLS struct {
	FMP4         bool `json:"fmp4"`
	TS           bool `json:"ts"`
	MaxSegmentMs int  `json:"maxSegmentMs,omitempty"`
}
type CapStreaming struct {
	HLS         CapHLS `json:"hls"`
	Progressive bool   `json:"progressive"`
}
type CapVideo struct {
	Codec          string   `json:"codec"`
	Profiles       []string `json:"profiles,omitempty"`
	MaxLevel       int      `json:"maxLevel,omitempty"`
	MaxBitDepth    int      `json:"maxBitDepth,omitempty"`
	MaxWidth       int      `json:"maxWidth,omitempty"`
	MaxHeight      int      `json:"maxHeight,omitempty"`
	MaxFps         float64  `json:"maxFps,omitempty"`
	MaxBitrateKbps int      `json:"maxBitrateKbps,omitempty"`
	HDR            []string `json:"hdr,omitempty"`
	ToneMapsToSdr  bool     `json:"toneMapsToSdr,omitempty"`
}
type CapDisplay struct {
	HDR       []string `json:"hdr"`
	MaxWidth  int      `json:"maxWidth,omitempty"`
	MaxHeight int      `json:"maxHeight,omitempty"`
}
type CapAudio struct {
	Codec         string `json:"codec"`
	MaxChannels   int    `json:"maxChannels,omitempty"`
	Passthrough   bool   `json:"passthrough,omitempty"`
	Atmos         bool   `json:"atmos,omitempty"`
	MaxSampleRate int    `json:"maxSampleRate,omitempty"`
	MaxBitDepth   int    `json:"maxBitDepth,omitempty"`
}
type CapAudioDecode struct {
	Codec         string   `json:"codec"`
	Containers    []string `json:"containers"`
	MaxSampleRate int      `json:"maxSampleRate,omitempty"`
	SampleRates   []int    `json:"sampleRates,omitempty"`
	MaxChannels   int      `json:"maxChannels,omitempty"`
	MaxBitDepth   int      `json:"maxBitDepth,omitempty"`
	// Via is the client's own note of which decoder path passed; the server ignores it.
	Via string `json:"via,omitempty"`
}

// AudioDecodeCaps are the device's audioDecode entries in the planner's terms.
func (c Capabilities) AudioDecodeCaps() []playback.AudioDecodeCap {
	out := make([]playback.AudioDecodeCap, 0, len(c.AudioDecode))
	for _, a := range c.AudioDecode {
		containers := make([]string, 0, len(a.Containers))
		for _, x := range a.Containers {
			containers = append(containers, strings.ToLower(x))
		}
		out = append(out, playback.AudioDecodeCap{Codec: strings.ToLower(a.Codec), Containers: containers, MaxSampleRate: a.MaxSampleRate, SampleRates: slices.Clone(a.SampleRates), MaxChannels: a.MaxChannels, MaxBitDepth: a.MaxBitDepth})
	}
	return out
}

type CapSubtitle struct {
	Format string `json:"format"`
	Render string `json:"render"`
}

var capToken = regexp.MustCompile(`^[a-z0-9][a-z0-9._:+-]{0,31}$`)
var featureKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,31}$`)

func tokenOK(v string) bool { return capToken.MatchString(strings.ToLower(v)) }

// Validate bounds the document. Values are open vocabularies; only their shape
// and sizes are checked, so a newer client isn't refused by an older server.
func (c Capabilities) Validate() error {
	bad := func(path string) error { return &FieldError{Path: path} }
	switch c.Form {
	case "tv", "phone", "tablet", "desktop", "browser", "cast", "speaker":
	default:
		if !tokenOK(c.Form) {
			return bad("form")
		}
	}
	if c.Network.Class != "local" && c.Network.Class != "remote" && c.Network.Class != "cellular" || c.Network.MaxBitrateKbps < 0 || c.Network.MaxBitrateKbps > 2_000_000 {
		return bad("network")
	}
	if len(c.Containers) > 32 || len(c.Video) > 32 || len(c.Audio) > 48 || len(c.Subtitles) > 32 || len(c.Features) > 64 {
		return bad("")
	}
	for i, x := range c.Containers {
		if !tokenOK(x.Container) {
			return bad("containers[" + strconv.Itoa(i) + "]")
		}
	}
	if c.Streaming.HLS.MaxSegmentMs < 0 || c.Streaming.HLS.MaxSegmentMs > 60_000 {
		return bad("streaming.hls.maxSegmentMs")
	}
	for i, v := range c.Video {
		if !tokenOK(v.Codec) || len(v.Profiles) > 32 || len(v.HDR) > 16 || v.MaxLevel < 0 || v.MaxLevel > 1000 || v.MaxBitDepth < 0 || v.MaxBitDepth > 16 || v.MaxWidth < 0 || v.MaxWidth > 32768 || v.MaxHeight < 0 || v.MaxHeight > 32768 || v.MaxFps < 0 || v.MaxFps > 1000 || v.MaxBitrateKbps < 0 || v.MaxBitrateKbps > 2_000_000 {
			return bad("video[" + strconv.Itoa(i) + "]")
		}
		for _, x := range append(slices.Clone(v.Profiles), v.HDR...) {
			if !tokenOK(x) {
				return bad("video[" + strconv.Itoa(i) + "]")
			}
		}
	}
	if d := c.Display; d != nil {
		if len(d.HDR) > 16 || d.MaxWidth < 0 || d.MaxWidth > 32768 || d.MaxHeight < 0 || d.MaxHeight > 32768 {
			return bad("display")
		}
		for _, x := range d.HDR {
			if !tokenOK(x) {
				return bad("display.hdr")
			}
		}
	}
	for i, a := range c.Audio {
		if !tokenOK(a.Codec) || a.MaxChannels < 0 || a.MaxChannels > 64 || a.MaxSampleRate < 0 || a.MaxSampleRate > 1<<22 || a.MaxBitDepth < 0 || a.MaxBitDepth > 64 {
			return bad("audio[" + strconv.Itoa(i) + "]")
		}
	}
	if len(c.AudioDecode) > 32 {
		return bad("audioDecode")
	}
	for i, a := range c.AudioDecode {
		bad := len(a.Containers) == 0 || len(a.Containers) > 16 || len(a.SampleRates) > 32 || !tokenOK(a.Codec) || a.MaxSampleRate < 0 || a.MaxSampleRate > 1<<22 || a.MaxChannels < 0 || a.MaxChannels > 64 || a.MaxBitDepth < 0 || a.MaxBitDepth > 64 || len(a.Via) > 32
		for _, x := range a.Containers {
			bad = bad || !tokenOK(x)
		}
		for _, r := range a.SampleRates {
			bad = bad || r <= 0 || r > 1<<22
		}
		if bad {
			return &FieldError{Path: "audioDecode[" + strconv.Itoa(i) + "]"}
		}
	}
	for i, s := range c.Subtitles {
		if !tokenOK(s.Format) || s.Render != "native" && s.Render != "client" && s.Render != "none" {
			return bad("subtitles[" + strconv.Itoa(i) + "]")
		}
	}
	for k := range c.Features {
		if !featureKey.MatchString(k) {
			return bad("features")
		}
	}
	return nil
}

var textSubtitles = map[string]bool{"webvtt": true, "vtt": true, "srt": true, "subrip": true, "ttml": true, "cea608": true, "mov_text": true}
var styledSubtitles = map[string]bool{"ass": true, "ssa": true}
var bitmapSubtitles = map[string]bool{"pgs": true, "hdmv_pgs_subtitle": true, "vobsub": true, "dvd_subtitle": true, "dvbsub": true, "dvb_subtitle": true}

// hdrRange maps a v1 HDR name to the planner's dynamic range and, for Dolby
// Vision, its profile ("dolbyvision:8.1" → dolby_vision, 8).
func hdrRange(v string) (string, int) {
	v = strings.ToLower(v)
	if rest, ok := strings.CutPrefix(v, "dolbyvision"); ok {
		profile := 0
		if major, _, _ := strings.Cut(strings.TrimPrefix(rest, ":"), "."); major != "" {
			profile, _ = strconv.Atoi(major)
		}
		return "dolby_vision", profile
	}
	switch v {
	case "hdr10", "hdr10plus", "hlg":
		return v, 0
	}
	return "", 0
}

// ClientProfile translates the v1 document into the planner's device profile,
// so v1 and every existing route plan from one set of facts.
func (c Capabilities) ClientProfile(platform, app, appVersion string) playback.ClientProfile {
	p := playback.ClientProfile{Version: playback.ClientProfileVersion, Evidence: playback.EvidenceDeclared}
	p.Client = playback.ClientIdentity{Family: strings.ToLower(c.Form), Platform: clip(platform, 64), App: clip(app, 64), AppVersion: clip(appVersion, 64), Engine: "v1"}
	if !tokenOK(p.Client.Family) {
		p.Client.Family = "unknown"
	}
	p.MaxBitrateBPS = c.Network.MaxBitrateKbps * 1000
	p.EmbeddedAudioSwitching = c.Features["embeddedAudioSwitching"]
	for _, a := range c.AudioDecode {
		p.AudioDecode = append(p.AudioDecode, playback.ClientAudioDecode{Codec: strings.ToLower(a.Codec), Containers: a.Containers, MaxSampleRate: a.MaxSampleRate, SampleRates: a.SampleRates, MaxChannels: a.MaxChannels, MaxBitDepth: a.MaxBitDepth})
	}
	display := map[string]bool{"sdr": true}
	if c.Display != nil {
		p.Display.Width, p.Display.Height = c.Display.MaxWidth, c.Display.MaxHeight
		for _, h := range c.Display.HDR {
			if r, _ := hdrRange(h); r != "" {
				display[r] = true
			}
		}
	}
	for r := range display {
		p.Display.DynamicRanges = append(p.Display.DynamicRanges, r)
	}
	var videoCodecs, audioCodecs []string
	maxChannels := 0
	for _, v := range c.Video {
		codec := strings.ToLower(v.Codec)
		out := playback.ClientVideoCodec{Codec: codec, Profiles: v.Profiles, MaxLevel: v.MaxLevel, MaxWidth: v.MaxWidth, MaxHeight: v.MaxHeight, MaxFrameRate: v.MaxFps, MaxBitrateBPS: v.MaxBitrateKbps * 1000, DynamicRanges: []string{"sdr"}, Evidence: playback.EvidenceDeclared}
		depth := v.MaxBitDepth
		if depth <= 0 {
			depth = 8
		}
		for _, d := range []int{8, 10, 12} {
			if d <= depth {
				out.BitDepths = append(out.BitDepths, d)
			}
		}
		for _, h := range v.HDR {
			r, dv := hdrRange(h)
			// A range the device decodes is presentable when the display shows it
			// or the device tone maps it to SDR itself (spec §5.6, D-FEAT-4).
			if r == "" || !(display[r] || v.ToneMapsToSdr) {
				continue
			}
			if !slices.Contains(out.DynamicRanges, r) {
				out.DynamicRanges = append(out.DynamicRanges, r)
			}
			if dv > 0 && !slices.Contains(out.DolbyVisionProfiles, dv) {
				out.DolbyVisionProfiles = append(out.DolbyVisionProfiles, dv)
			}
		}
		p.Video = append(p.Video, out)
		videoCodecs = append(videoCodecs, codec)
	}
	for _, a := range c.Audio {
		codec := strings.ToLower(a.Codec)
		out := playback.ClientAudioCodec{Codec: codec, MaxChannels: a.MaxChannels, MaxSampleRate: a.MaxSampleRate, Passthrough: a.Passthrough, Evidence: playback.EvidenceDeclared}
		if a.Atmos {
			out.ObjectAudio = []string{"atmos"}
		}
		p.Audio = append(p.Audio, out)
		audioCodecs = append(audioCodecs, codec)
		maxChannels = max(maxChannels, a.MaxChannels)
	}
	p.AudioOutput = playback.ClientAudioOutput{MaxChannels: maxChannels}
	if c.Streaming.Progressive {
		var direct []string
		for _, x := range c.Containers {
			if x.Direct {
				direct = append(direct, strings.ToLower(x.Container))
			}
		}
		if len(direct) > 0 {
			p.Transports = append(p.Transports, playback.ClientTransport{Transport: playback.TransportDirect, Containers: direct, Video: videoCodecs, Audio: audioCodecs})
		}
	}
	if c.Streaming.HLS.TS {
		var ts []string
		for _, v := range videoCodecs {
			if v == "h264" || v == "mpeg2" || v == "hevc" {
				ts = append(ts, v)
			}
		}
		p.Transports = append(p.Transports, playback.ClientTransport{Transport: playback.TransportHLSTS, Containers: []string{"mpegts"}, Video: ts, Audio: audioCodecs})
	}
	if c.Streaming.HLS.FMP4 {
		p.Transports = append(p.Transports, playback.ClientTransport{Transport: playback.TransportHLSFMP4, Containers: []string{"fmp4"}, Video: videoCodecs, Audio: audioCodecs})
	}
	for _, s := range c.Subtitles {
		f := strings.ToLower(s.Format)
		if s.Render == "none" {
			continue
		}
		switch {
		case textSubtitles[f]:
			p.Subtitles.Text = append(p.Subtitles.Text, f)
		case styledSubtitles[f] && s.Render == "client":
			p.Subtitles.Styled = append(p.Subtitles.Styled, f)
		case bitmapSubtitles[f] && s.Render == "client":
			p.Subtitles.Bitmap = append(p.Subtitles.Bitmap, f)
		}
	}
	return p
}

func clip(v string, n int) string {
	var b strings.Builder
	for _, r := range v {
		if r >= 0x20 && r != 0x7f && b.Len() < n {
			b.WriteRune(r)
		}
	}
	return b.String()
}
