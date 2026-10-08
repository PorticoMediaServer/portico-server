package playback

import (
	"encoding/json"
	"strings"
	"testing"

	"portico.local/server/internal/assets"
)

// Device documents used across the decision tests. They are deliberately close
// to what the real clients publish, so the matrix below reads as "what happens
// when this file is played on that device".
func appleTVProfile() ClientProfile {
	p := ClientProfile{
		Version: ClientProfileVersion, Evidence: EvidenceMixed,
		Client:  ClientIdentity{Family: "apple", Platform: "tvos", Engine: "avplayer", Model: "AppleTV14,1"},
		Display: ClientDisplay{Width: 3840, Height: 2160, DynamicRanges: []string{"sdr", "hdr10", "hlg", "dolby_vision"}, MaxFrameRate: 60},
		Video: []ClientVideoCodec{
			{Codec: "h264", BitDepths: []int{8}, MaxWidth: 4096, MaxHeight: 2304, MaxFrameRate: 60, DynamicRanges: []string{"sdr"}},
			{Codec: "hevc", BitDepths: []int{8, 10}, MaxWidth: 4096, MaxHeight: 2304, MaxFrameRate: 60, DynamicRanges: []string{"sdr", "hdr10", "hlg", "dolby_vision"}, DolbyVisionProfiles: []int{5, 8}},
		},
		Audio: []ClientAudioCodec{
			{Codec: "aac", MaxChannels: 8}, {Codec: "mp3", MaxChannels: 2}, {Codec: "ac3", MaxChannels: 6},
			{Codec: "eac3", MaxChannels: 8, ObjectAudio: []string{"atmos"}}, {Codec: "alac", MaxChannels: 8}, {Codec: "flac", MaxChannels: 8},
		},
		AudioOutput: ClientAudioOutput{Route: "hdmi", MaxChannels: 8, Spatial: true},
		Transports: []ClientTransport{
			{Transport: TransportDirect, Containers: []string{"mp4", "m4v", "mov"}, Video: []string{"h264", "hevc"}, Audio: []string{"aac", "mp3", "ac3", "eac3", "alac", "flac"}},
			{Transport: TransportDirect, Containers: []string{"m4a", "mp3", "flac", "wav"}, Audio: []string{"aac", "mp3", "alac", "flac"}},
			{Transport: TransportHLSTS, Containers: []string{"mpegts"}, Video: []string{"h264"}, Audio: []string{"aac", "mp3", "ac3", "eac3"}},
			{Transport: TransportHLSFMP4, Containers: []string{"fmp4"}, Video: []string{"h264", "hevc"}, Audio: []string{"aac", "mp3", "ac3", "eac3", "alac", "flac"}},
		},
		EmbeddedAudioSwitching: true,
	}
	p.normalize()
	p.Revision = "apple-tv-test"
	return p
}

func chromeProfile() ClientProfile {
	p := ClientProfile{
		Version: ClientProfileVersion, Evidence: EvidenceProbed,
		Client:  ClientIdentity{Family: "web", Platform: "chrome", Engine: "hls.js"},
		Display: ClientDisplay{Width: 1920, Height: 1080, DynamicRanges: []string{"sdr"}, MaxFrameRate: 60},
		Video: []ClientVideoCodec{
			{Codec: "h264", BitDepths: []int{8}, MaxWidth: 4096, MaxHeight: 2160, MaxFrameRate: 60, DynamicRanges: []string{"sdr"}},
			{Codec: "vp9", BitDepths: []int{8, 10}, MaxWidth: 4096, MaxHeight: 2160, DynamicRanges: []string{"sdr"}},
			{Codec: "av1", BitDepths: []int{8, 10}, MaxWidth: 4096, MaxHeight: 2160, DynamicRanges: []string{"sdr"}},
		},
		Audio:       []ClientAudioCodec{{Codec: "aac", MaxChannels: 6}, {Codec: "mp3", MaxChannels: 2}, {Codec: "opus", MaxChannels: 6}, {Codec: "flac", MaxChannels: 6}, {Codec: "vorbis", MaxChannels: 6}},
		AudioOutput: ClientAudioOutput{Route: "speaker", MaxChannels: 2},
		Transports: []ClientTransport{
			{Transport: TransportDirect, Containers: []string{"mp4", "m4v", "mov"}, Video: []string{"h264", "vp9", "av1"}, Audio: []string{"aac", "mp3", "opus", "flac"}},
			{Transport: TransportDirect, Containers: []string{"webm"}, Video: []string{"vp9", "av1"}, Audio: []string{"opus", "vorbis"}},
			{Transport: TransportDirect, Containers: []string{"mp3", "m4a", "flac", "ogg", "opus", "wav"}, Audio: []string{"aac", "mp3", "opus", "flac", "vorbis"}},
			{Transport: TransportHLSTS, Containers: []string{"mpegts"}, Video: []string{"h264"}, Audio: []string{"aac", "mp3"}},
			{Transport: TransportHLSFMP4, Containers: []string{"fmp4"}, Video: []string{"h264"}, Audio: []string{"aac", "mp3", "flac", "opus"}},
		},
	}
	p.normalize()
	p.Revision = "chrome-test"
	return p
}

func videoSource(container, codec string, detail assets.StreamDetail, audio ...SourceAudio) *DeliverySource {
	for i := range audio {
		audio[i].Ordinal, audio[i].Index = i, i+1
	}
	return &DeliverySource{ID: "source", Container: container, Duration: 3600, BitRate: detail.BitRate, Video: &SourceVideo{Codec: codec, StreamDetail: detail}, Audio: audio, DetailKnown: true}
}

func decide(t *testing.T, source *DeliverySource, client ClientProfile, mutate func(*DeliveryInput)) DeliveryPlan {
	t.Helper()
	policy := openPolicy()
	height := 0
	if source.Video != nil {
		height = source.Video.Height
	}
	in := DeliveryInput{Source: source, Client: client, Policy: policy, Target: autoTarget(policy, height), Config: DefaultDeliveryConfiguration(), TranscodingEnabled: true}
	if mutate != nil {
		mutate(&in)
	}
	p, err := planDelivery(in)
	if err != nil {
		t.Fatal(err)
	}
	if p.Trace == nil || len(p.Trace.Routes) == 0 {
		t.Fatal("a plan was published without its decision trace")
	}
	chosen := 0
	for _, r := range p.Trace.Routes {
		if r.Chosen {
			chosen++
			if !r.Admissible || r.Route != string(p.Strategy) {
				t.Fatalf("trace marks %s chosen but the plan is %s", r.Route, p.Strategy)
			}
		}
		if !r.Admissible && len(r.Rejections) == 0 {
			t.Fatalf("route %s was rejected without a reason", r.Route)
		}
	}
	if chosen != 1 {
		t.Fatalf("trace marks %d routes chosen", chosen)
	}
	return p
}

func rejected(p DeliveryPlan, route DeliveryStrategy, code string) bool {
	r := routeEvaluation(p.Trace, route)
	return r != nil && !r.Admissible && hasRejection(r.Rejections, code)
}

var hdr10Detail = assets.StreamDetail{Profile: "Main 10", Level: 153, BitDepth: 10, Width: 3840, Height: 2160, FrameRate: 23.976, DynamicRange: assets.RangeHDR10, BitRate: 40_000_000}
var avcDetail = assets.StreamDetail{Profile: "High", Level: 41, BitDepth: 8, Width: 1920, Height: 1080, FrameRate: 23.976, DynamicRange: assets.RangeSDR, BitRate: 8_000_000}

func TestDecisionMatrixAcrossDevices(t *testing.T) {
	eac3 := SourceAudio{Codec: "eac3", Channels: 6, Language: "eng", Default: true}
	t.Run("hdr10 hevc mkv on apple tv copies into fragmented mp4", func(t *testing.T) {
		p := decide(t, videoSource("mkv", "hevc", hdr10Detail, eac3), appleTVProfile(), func(in *DeliveryInput) { in.FMP4Output = true })
		if p.Strategy != DeliveryCopyRemux || p.OutputContainer != "fmp4_hls" || p.ToneMap || p.AudioAction != "copy" {
			t.Fatal(p)
		}
		if !rejected(p, DeliveryOriginal, RejectContainer) {
			t.Fatal("the trace does not say the container was the obstacle", p.Trace.Routes)
		}
	})
	t.Run("without fragmented mp4 output the same file converts, tone maps and keeps its audio", func(t *testing.T) {
		p := decide(t, videoSource("mkv", "hevc", hdr10Detail, eac3), appleTVProfile(), nil)
		if p.Strategy != DeliveryVideoConversion || !p.ToneMap || p.AudioAction != "copy" || p.OutputAudioCodec != "eac3" {
			t.Fatal(p)
		}
	})
	t.Run("hdr10 hevc mp4 plays directly on apple tv and converts for chrome", func(t *testing.T) {
		source := videoSource("mp4", "hevc", hdr10Detail, eac3)
		if p := decide(t, source, appleTVProfile(), nil); p.Strategy != DeliveryOriginal {
			t.Fatal(p)
		}
		p := decide(t, source, chromeProfile(), nil)
		if p.Strategy != DeliveryVideoConversion || !p.ToneMap || p.AudioAction != "convert" || p.AudioChannels != 2 || p.Downmix != DownmixITULimited {
			t.Fatal(p)
		}
		if !rejected(p, DeliveryOriginal, RejectVideoCodec) || !rejected(p, DeliveryOriginal, RejectAudioCodec) {
			t.Fatal(p.Trace.Routes)
		}
	})
	t.Run("dolby vision profile 5 needs a dolby vision device", func(t *testing.T) {
		detail := hdr10Detail
		detail.DynamicRange, detail.DolbyVisionProfile, detail.DolbyVisionCompatibility, detail.CodecTag = assets.RangeDolbyVision, 5, 0, "dvh1"
		source := videoSource("mp4", "hevc", detail, eac3)
		if p := decide(t, source, appleTVProfile(), nil); p.Strategy != DeliveryOriginal {
			t.Fatal(p)
		}
		noDV := appleTVProfile()
		noDV.Video[1].DolbyVisionProfiles = []int{}
		p := decide(t, source, noDV, nil)
		if p.Strategy != DeliveryVideoConversion || !rejected(p, DeliveryOriginal, RejectDolbyVision) || !contains(p.Trace.Notes, NoteDolbyVisionNoLayer) {
			t.Fatal(p, p.Trace)
		}
	})
	t.Run("dolby vision 8.1 falls back to its hdr10 layer", func(t *testing.T) {
		detail := hdr10Detail
		detail.DynamicRange, detail.DolbyVisionProfile, detail.DolbyVisionCompatibility, detail.CodecTag = assets.RangeDolbyVision, 8, 1, "hvc1"
		noDV := appleTVProfile()
		noDV.Video[1].DolbyVisionProfiles = []int{}
		if p := decide(t, videoSource("mp4", "hevc", detail, eac3), noDV, nil); p.Strategy != DeliveryOriginal {
			t.Fatal(p, p.Trace.Routes)
		}
		detail.CodecTag = "dvh1"
		p := decide(t, videoSource("mp4", "hevc", detail, eac3), noDV, func(in *DeliveryInput) { in.FMP4Output = true })
		if p.Strategy != DeliveryCopyRemux || !rejected(p, DeliveryOriginal, RejectDolbyVisionEntry) {
			t.Fatal(p, p.Trace.Routes)
		}
	})
	t.Run("ten bit h264 is not an eight bit decoder's problem to discover", func(t *testing.T) {
		detail := avcDetail
		detail.Profile, detail.BitDepth = "High 10", 10
		p := decide(t, videoSource("mp4", "h264", detail, SourceAudio{Codec: "aac", Channels: 2}), chromeProfile(), nil)
		if p.Strategy != DeliveryVideoConversion || !rejected(p, DeliveryOriginal, RejectVideoBitDepth) || !rejected(p, DeliveryCopyRemux, RejectVideoBitDepth) {
			t.Fatal(p, p.Trace.Routes)
		}
	})
	t.Run("surround is kept when the output route can play it", func(t *testing.T) {
		source := videoSource("mp4", "h264", avcDetail, SourceAudio{Codec: "dts", Channels: 6})
		p := decide(t, source, chromeProfile(), nil)
		if p.Strategy != DeliveryAudioConversion || p.AudioChannels != 2 || p.Downmix == "" || p.VideoAction != "copy" {
			t.Fatal(p)
		}
		surround := chromeProfile()
		surround.AudioOutput.MaxChannels = 6
		p = decide(t, source, surround, nil)
		if p.AudioChannels != 6 || p.AudioBitrateBPS != 384_000 || p.Downmix != "" {
			t.Fatal(p)
		}
		if _, args, _, err := p.CodecArgs(); err != nil || !strings.Contains(strings.Join(args, " "), "-c:a aac -b:a 384k -ac 6") {
			t.Fatal(args, err)
		}
	})
	t.Run("a preferred language on the second track leaves the original behind", func(t *testing.T) {
		source := videoSource("mp4", "h264", avcDetail, SourceAudio{Codec: "aac", Channels: 2, Language: "eng", Default: true}, SourceAudio{Codec: "aac", Channels: 2, Language: "fra"})
		p := decide(t, source, chromeProfile(), func(in *DeliveryInput) { in.PreferredAudioLanguages = []string{"fr"} })
		if p.Strategy != DeliveryCopyRemux || p.AudioStream != 2 || p.Trace.Audio.ChosenBy != "preferred_language" || !rejected(p, DeliveryOriginal, RejectAudioTrack) {
			t.Fatal(p, p.Trace.Routes)
		}
		// A device that can switch tracks inside the file keeps the original.
		if p = decide(t, source, appleTVProfile(), func(in *DeliveryInput) { in.PreferredAudioLanguages = []string{"fre"} }); p.Strategy != DeliveryOriginal || p.AudioStream != 2 {
			t.Fatal(p)
		}
		// A commentary is never the automatic choice.
		source = videoSource("mp4", "h264", avcDetail, SourceAudio{Codec: "aac", Channels: 2, Language: "eng", Commentary: true, Default: true}, SourceAudio{Codec: "aac", Channels: 2, Language: "eng"})
		if p = decide(t, source, appleTVProfile(), nil); p.AudioStream != 2 {
			t.Fatal("commentary chosen automatically", p.Trace.Audio)
		}
	})
	t.Run("a network ceiling applies to the original too", func(t *testing.T) {
		detail := avcDetail
		detail.BitRate = 40_000_000
		source := videoSource("mp4", "h264", detail, SourceAudio{Codec: "aac", Channels: 2})
		cfg := DefaultDeliveryConfiguration()
		lane := ResolveDeliveryPolicy(stubPreferences{"quality.cellular.maxVideoBitrateMbps": 8, "quality.cellular.allowHDR": true}, NetworkCellular, LocalityRemote, "cellular", cfg)
		p := decide(t, source, chromeProfile(), func(in *DeliveryInput) { in.Policy, in.Target = lane, autoTarget(lane, 1080) })
		if p.Strategy != DeliveryVideoConversion || p.VideoBitrateBPS != 8_000_000 || !rejected(p, DeliveryOriginal, ReasonBitrateExceedsPolicy) || !contains(p.ReasonCodes, ReasonBitrateExceedsPolicy) {
			t.Fatal(p, p.Trace.Routes)
		}
	})
	t.Run("interlaced mpeg-2 is deinterlaced", func(t *testing.T) {
		detail := assets.StreamDetail{Profile: "Main", BitDepth: 8, Width: 720, Height: 576, FrameRate: 25, Interlaced: true, DynamicRange: assets.RangeSDR}
		p := decide(t, videoSource("ts", "mpeg2video", detail, SourceAudio{Codec: "mp2", Channels: 2}), chromeProfile(), nil)
		if p.Strategy != DeliveryVideoConversion || !p.Deinterlace {
			t.Fatal(p)
		}
		if _, args, _, err := p.CodecArgs(); err != nil || !strings.Contains(strings.Join(args, " "), "yadif=") {
			t.Fatal(args, err)
		}
	})
	t.Run("a high frame rate source is capped at what the decoder takes", func(t *testing.T) {
		detail := avcDetail
		detail.FrameRate = 119.88
		p := decide(t, videoSource("mkv", "hevc", detail, SourceAudio{Codec: "aac", Channels: 2}), chromeProfile(), nil)
		if p.MaxFrameRate != 60 {
			t.Fatal(p)
		}
	})
	t.Run("a route the device reported broken is not offered again", func(t *testing.T) {
		source := videoSource("mp4", "h264", avcDetail, SourceAudio{Codec: "aac", Channels: 2})
		p := decide(t, source, chromeProfile(), func(in *DeliveryInput) {
			in.Excluded = map[DeliveryStrategy]RouteRejection{DeliveryOriginal: {Code: "decode_error"}}
		})
		if p.Strategy != DeliveryCopyRemux || !rejected(p, DeliveryOriginal, RejectClientFailure) {
			t.Fatal(p, p.Trace.Routes)
		}
		p = decide(t, source, chromeProfile(), func(in *DeliveryInput) {
			in.Excluded = map[DeliveryStrategy]RouteRejection{DeliveryOriginal: {Code: "decode_error"}, DeliveryCopyRemux: {Code: "decode_error"}, DeliveryAudioConversion: {Code: "decode_error"}}
		})
		if p.Strategy != DeliveryVideoConversion {
			t.Fatal(p)
		}
	})
	t.Run("lossless music plays as it is where the device says so", func(t *testing.T) {
		source := &DeliverySource{ID: "song", Container: "flac", Duration: 200, Audio: []SourceAudio{{Codec: "flac", Channels: 2}}, DetailKnown: true}
		if p := decide(t, source, chromeProfile(), nil); p.Strategy != DeliveryOriginal || p.Reason != ReasonDirectAudioCompatible {
			t.Fatal(p)
		}
		if p := decide(t, source, BaselineClientProfile(), nil); p.Strategy != DeliveryAudioConversion || p.VideoAction != "none" {
			t.Fatal(p)
		}
	})
	t.Run("rotated video is never copied into a container that drops the rotation", func(t *testing.T) {
		detail := avcDetail
		detail.Rotation = 90
		p := decide(t, videoSource("mkv", "h264", detail, SourceAudio{Codec: "aac", Channels: 2}), chromeProfile(), nil)
		if p.Strategy != DeliveryVideoConversion || !rejected(p, DeliveryCopyRemux, RejectRotation) {
			t.Fatal(p, p.Trace.Routes)
		}
	})
}

func TestDecisionTraceIsBoundedAndPathFree(t *testing.T) {
	p := decide(t, videoSource("mkv", "hevc", hdr10Detail, SourceAudio{Codec: "truehd", Channels: 8, StreamDetail: assets.StreamDetail{ObjectAudio: "atmos"}}), chromeProfile(), nil)
	raw, err := json.Marshal(p.Trace)
	if err != nil || len(raw) > 8<<10 {
		t.Fatal(len(raw), err)
	}
	if strings.Contains(string(raw), "source\"") || strings.Contains(string(raw), "/") && strings.Contains(string(raw), "Users") {
		t.Fatal("trace carries an identifier or a path", string(raw))
	}
	if p.Trace.Audio.ObjectAudio != "atmos" || p.Trace.Client.Family != "web" {
		t.Fatal(p.Trace)
	}
}

func TestChooseAudioLanguageFolding(t *testing.T) {
	tracks := []SourceAudio{{Index: 1, Language: "ger"}, {Index: 2, Language: "en-US"}, {Index: 3, Language: "jpn", Default: true}}
	for want, index := range map[string]int{"de": 1, "deu": 1, "eng": 2, "en": 2, "ja": 3} {
		if a, by := chooseAudio(tracks, -1, []string{want}); a.Index != index || by != "preferred_language" {
			t.Fatal(want, a, by)
		}
	}
	if a, by := chooseAudio(tracks, -1, []string{"sv"}); a.Index != 3 || by != "default" {
		t.Fatal(a, by)
	}
	if a, by := chooseAudio(tracks, 2, []string{"de"}); a.Index != 2 || by != "explicit" {
		t.Fatal(a, by)
	}
}

func TestClientProfileParsing(t *testing.T) {
	raw, _ := json.Marshal(chromeProfile())
	p, err := ParseClientProfile(raw)
	if err != nil || p.Revision == "" || p.Client.Family != "web" {
		t.Fatal(p, err)
	}
	again, _ := ParseClientProfile(raw)
	if again.Revision != p.Revision {
		t.Fatal("equal documents produced different revisions")
	}
	for name, body := range map[string]string{
		"version":    `{"version":2}`,
		"wrong type": `{"version":1,"client":{"family":"web"},"evidence":"probed","video":"everything"}`,
		"family":     `{"version":1,"client":{"family":"Web Browser!"},"evidence":"probed"}`,
		"evidence":   `{"version":1,"client":{"family":"web"},"evidence":"trust me"}`,
		"range":      `{"version":1,"client":{"family":"web"},"evidence":"probed","display":{"dynamicRanges":["hdr11"]}}`,
		"transport":  `{"version":1,"client":{"family":"web"},"evidence":"probed","transports":[{"transport":"rtsp"}]}`,
	} {
		if _, err := ParseClientProfile([]byte(body)); err == nil {
			t.Fatal("accepted an invalid profile:", name)
		}
	}
	if _, err := ParseClientProfile([]byte(strings.Repeat(" ", MaxClientProfileBytes+1))); err == nil {
		t.Fatal("accepted an oversized profile")
	}
	// Version 1 grows by addition: a field this server has not met is ignored.
	if _, err := ParseClientProfile([]byte(`{"version":1,"client":{"family":"web"},"evidence":"probed","futureField":{"a":1}}`)); err != nil {
		t.Fatal("an additive field was refused", err)
	}
	// The minimal valid document plans like a device that can play nothing directly.
	minimal, err := ParseClientProfile([]byte(`{"version":1,"client":{"family":"web"},"evidence":"declared"}`))
	if err != nil {
		t.Fatal(err)
	}
	source := videoSource("mp4", "h264", avcDetail, SourceAudio{Codec: "aac", Channels: 2})
	policy := openPolicy()
	if _, err = planDelivery(DeliveryInput{Source: source, Client: minimal, Policy: policy, Target: autoTarget(policy, 1080), Config: DefaultDeliveryConfiguration(), TranscodingEnabled: true}); err == nil {
		t.Fatal("a device with no transports was given a route")
	}
}
