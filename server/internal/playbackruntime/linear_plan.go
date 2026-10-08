package playbackruntime

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/playback"
)

type linearProbeStream struct {
	FieldOrder  string `json:"field_order"`
	Level       int    `json:"level"`
	FrameRate   string `json:"avg_frame_rate"`
	Kind        string `json:"codec_type"`
	Codec       string `json:"codec_name"`
	Profile     string `json:"profile"`
	PixelFormat string `json:"pix_fmt"`
	Transfer    string `json:"color_transfer"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Channels    int    `json:"channels"`
	SampleRate  string `json:"sample_rate"`
	Layout      string `json:"channel_layout"`
	Bitrate     string `json:"bit_rate"`
}

// Plan from probed elementary streams and the admitted engine's declarations.
// A conversion must produce a supported tuple, not merely a supported codec.
func linearPlan(raw []byte, w playback.LinearWork) (decoder.LinearEncoding, error) {
	var facts struct {
		Streams []linearProbeStream `json:"streams"`
		Format  struct {
			Bitrate string `json:"bit_rate"`
		} `json:"format"`
	}
	var plan decoder.LinearEncoding
	if len(raw) > decoder.MaxProbeOutputBytes || json.Unmarshal(raw, &facts) != nil || len(facts.Streams) == 0 || len(facts.Streams) > 128 {
		return plan, channelFault("source_probe_invalid", 422)
	}
	var video, audio *linearProbeStream
	for i := range facts.Streams {
		s := &facts.Streams[i]
		if s.Kind == "video" && video == nil {
			video = s
		}
		if s.Kind == "audio" && audio == nil {
			audio = s
		}
	}
	if video == nil && audio == nil {
		return plan, channelFault("source_has_no_playable_stream", 422)
	}
	client := w.ClientProfile
	if client.Version == 0 {
		client = playback.BaselineClientProfile()
	}
	if !slices.ContainsFunc(client.Transports, func(t playback.ClientTransport) bool { return t.Transport == playback.TransportHLSTS }) {
		return plan, channelFault("engine_transport_unavailable", 422)
	}
	var videoCaps *playback.ClientVideoCodec
	var vc *playback.ControlCodec
	var ac *playback.ControlCodec
	for _, c := range client.Video {
		if c.Codec == "h264" {
			videoCaps = &c
			v := playback.ControlCodec{Codec: "h264", BitDepths: c.BitDepths, DynamicRanges: c.DynamicRanges}
			if c.MaxHeight > 0 {
				h := c.MaxHeight
				v.MaxHeight = &h
			}
			if c.MaxWidth > 0 {
				w := c.MaxWidth
				v.MaxWidth = &w
			}
			vc = &v
			break
		}
	}
	for _, c := range client.Audio {
		if c.Codec == "aac" {
			v := playback.ControlCodec{Codec: "aac"}
			if c.MaxChannels > 0 {
				n := c.MaxChannels
				v.MaxChannels = &n
			}
			ac = &v
			break
		}
	}
	var sourceVideo *playback.SourceVideo
	var sourceAudio *playback.SourceAudio
	if video != nil {
		fps := 0.0
		parts := strings.Split(video.FrameRate, "/")
		if len(parts) == 2 {
			a, _ := strconv.ParseFloat(parts[0], 64)
			b, _ := strconv.ParseFloat(parts[1], 64)
			if b > 0 {
				fps = a / b
			}
		}
		depth := 8
		if strings.Contains(video.PixelFormat, "10") {
			depth = 10
		}
		sourceVideo = &playback.SourceVideo{Codec: video.Codec, StreamDetail: assets.StreamDetail{Profile: video.Profile, Level: video.Level, Width: video.Width, Height: video.Height, BitDepth: depth, FrameRate: fps, DynamicRange: assets.DynamicRangeOf(video.Transfer, 0), Interlaced: video.FieldOrder != "" && video.FieldOrder != "unknown" && video.FieldOrder != "progressive"}}
	}
	if audio != nil {
		rate, _ := strconv.Atoi(audio.SampleRate)
		sourceAudio = &playback.SourceAudio{Codec: audio.Codec, Channels: audio.Channels, StreamDetail: assets.StreamDetail{SampleRate: rate}}
	}
	videoRules, audioRules := playback.HLSTSRejections(sourceVideo, sourceAudio, client)
	q := w.Selection.Quality
	plan.Video, plan.Audio = video != nil, audio != nil
	hdr := video != nil && (video.Transfer == "smpte2084" || video.Transfer == "arib-std-b67")
	if video != nil {
		if video.Width < 1 || video.Height < 1 || video.Width > 32768 || video.Height > 32768 {
			return plan, channelFault("source_video_invalid", 422)
		}
		if vc == nil || len(vc.BitDepths) > 0 && !slices.Contains(vc.BitDepths, 8) || len(vc.DynamicRanges) > 0 && !slices.Contains(vc.DynamicRanges, "sdr") {
			return plan, channelFault("engine_codec_unavailable", 422)
		}
		height := video.Height
		for _, limit := range []*int{q.MaxHeight, vc.MaxHeight} {
			if limit != nil {
				height = min(height, *limit)
			}
		}
		for _, limit := range []*int{q.MaxWidth, vc.MaxWidth} {
			if limit != nil && video.Width > *limit {
				height = min(height, video.Height**limit/video.Width)
			}
		}
		if height < 2 {
			return plan, channelFault("unsupported_tuple", 422)
		}
		// Bound the encoded tuple as well as the copied input. H.264 levels
		// constrain macroblocks per frame and per second, not just pixel height.
		fps := sourceVideo.FrameRate
		limit := 60.0
		if videoCaps.MaxFrameRate > 0 {
			limit = min(limit, videoCaps.MaxFrameRate)
		}
		if fps <= 0 || fps > limit {
			fps = limit
			plan.MaxFrameRate = limit
		}
		if videoCaps.MaxLevel > 0 {
			plan.Level = videoCaps.MaxLevel
			maxFS, maxMBPS := h264LevelBudget(plan.Level)
			for height > 2 && ((video.Width*height/video.Height+15)/16)*((height+15)/16) > maxFS {
				height -= 2
			}
			blocks := ((video.Width*height/video.Height + 15) / 16) * ((height + 15) / 16)
			if blocks < 1 {
				return plan, channelFault("unsupported_tuple", 422)
			}
			if cap := float64(maxMBPS) / float64(blocks); fps > cap {
				fps = cap
				plan.MaxFrameRate = cap
			}
		}
		if len(videoCaps.Profiles) > 0 {
			for _, supported := range []string{"high", "main", "baseline"} {
				if slices.ContainsFunc(videoCaps.Profiles, func(v string) bool {
					return strings.EqualFold(strings.ReplaceAll(v, "_", " "), supported) || supported == "baseline" && strings.EqualFold(strings.ReplaceAll(v, "_", " "), "constrained baseline")
				}) {
					plan.Profile = supported
					break
				}
			}
			if plan.Profile == "" {
				return plan, channelFault("unsupported_tuple", 422)
			}
		}
		plan.Deinterlace = sourceVideo.Interlaced
		plan.ConvertVideo = plan.MaxFrameRate > 0 || len(videoRules) > 0 || plan.Deinterlace || video.Codec != "h264" || video.PixelFormat != "yuv420p" || hdr || height < video.Height
		if height < video.Height {
			plan.MaxHeight = height - height%2
		}
		bitrate, _ := strconv.Atoi(video.Bitrate)
		if bitrate == 0 {
			bitrate, _ = strconv.Atoi(facts.Format.Bitrate)
		}
		if videoCaps.MaxBitrateBPS > 0 {
			plan.VideoBitrate = videoCaps.MaxBitrateBPS
			if bitrate == 0 || bitrate > plan.VideoBitrate {
				plan.ConvertVideo = true
			}
		}
		if q.MaxBitrateBPS != nil {
			audioBudget := 0
			if audio != nil {
				audioBudget = 192000
			}
			if *q.MaxBitrateBPS < audioBudget+128000 {
				return plan, channelFault("channel_quality_too_low", 422)
			}
			if plan.VideoBitrate == 0 {
				plan.VideoBitrate = *q.MaxBitrateBPS - audioBudget
			} else {
				plan.VideoBitrate = min(plan.VideoBitrate, *q.MaxBitrateBPS-audioBudget)
			}
			if bitrate == 0 || bitrate > plan.VideoBitrate {
				plan.ConvertVideo = true
			}
		}
	}
	if audio != nil {
		if ac == nil || ac.MaxChannels != nil && *ac.MaxChannels < 2 || len(ac.SampleRates) > 0 && !slices.Contains(ac.SampleRates, 48000) || len(ac.ChannelLayouts) > 0 && !slices.Contains(ac.ChannelLayouts, "stereo") {
			return plan, channelFault("engine_codec_unavailable", 422)
		}
		if len(ac.Profiles) > 0 || len(ac.Levels) > 0 {
			return plan, channelFault("unsupported_tuple", 422)
		}
		sampleRate, _ := strconv.Atoi(audio.SampleRate)
		layout := audio.Layout
		if layout == "" && audio.Channels == 1 {
			layout = "mono"
		}
		if layout == "" && audio.Channels == 2 {
			layout = "stereo"
		}
		plan.ConvertAudio = len(audioRules) > 0 || audio.Codec != "aac" || !strings.EqualFold(audio.Profile, "LC") || audio.Channels < 1 || audio.Channels > 2 || ac.MaxChannels != nil && audio.Channels > *ac.MaxChannels || len(ac.SampleRates) > 0 && !slices.Contains(ac.SampleRates, sampleRate) || len(ac.ChannelLayouts) > 0 && !slices.Contains(ac.ChannelLayouts, layout)
		// A requested aggregate ceiling also applies to copied audio.
		if q.MaxBitrateBPS != nil {
			rate, _ := strconv.Atoi(audio.Bitrate)
			if *q.MaxBitrateBPS < 192000 {
				return plan, channelFault("channel_quality_too_low", 422)
			}
			if rate == 0 || rate > 192000 {
				plan.ConvertAudio = true
			}
		}
	}
	if plan.ConvertVideo || plan.ConvertAudio {
		if q.Mode == "original" || !q.AllowLossy {
			return plan, channelFault("channel_quality_requires_original", 422)
		}
		if !w.TranscodingEnabled {
			return plan, channelFault("transcoding_disabled", 422)
		}
		if hdr && !q.AllowHDRToSDR {
			return plan, channelFault("hdr_conversion_not_allowed", 422)
		}
		plan.HDRToSDR = hdr
	}
	return plan, nil
}

func h264LevelBudget(level int) (int, int) {
	for _, v := range []struct{ level, frames, second int }{{10, 99, 1485}, {11, 396, 3000}, {12, 396, 6000}, {13, 396, 11880}, {20, 396, 11880}, {21, 792, 19800}, {22, 1620, 20250}, {30, 1620, 40500}, {31, 3600, 108000}, {32, 5120, 216000}, {40, 8192, 245760}, {41, 8192, 245760}, {42, 8704, 522240}, {50, 22080, 589824}, {51, 36864, 983040}, {52, 36864, 2073600}, {60, 139264, 4177920}, {61, 139264, 8355840}, {62, 139264, 16711680}} {
		if level <= v.level {
			return v.frames, v.second
		}
	}
	return 139264, 16711680
}
