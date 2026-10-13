package playback

import (
	"portico.local/server/internal/assets"
	"portico.local/server/internal/decoder"
)

func videoOf(s DeliverySource) *SourceVideo { return s.Video }

// Compare transports after selecting audio, so fragmented MP4 can keep a
// lossless or wider layout even when MPEG-TS appeared first.
func bestAudioConversionTransport(transports []ClientTransport, client ClientProfile, video *SourceVideo, audio *SourceAudio, target QualityTarget, outputs ConversionOutputs) (string, []RouteRejection) {
	name := ""
	var best convertedAudio
	var nearest []RouteRejection
	for _, t := range transports {
		_, rejects := bestCopyTransport([]ClientTransport{t}, client, video, nil, target.AllowHDR, true)
		if len(rejects) > 0 {
			if name == "" && nearest == nil {
				nearest = rejects
			}
			continue
		}
		out := audioOutputFor(audio, client, target, &t, outputs)
		if out.NoRate {
			continue
		}
		better := name == "" || out.Channels > best.Channels || out.Channels == best.Channels && out.Lossless && !best.Lossless
		if better {
			name, best = t.Transport, out
		}
	}
	if name == "" {
		if nearest == nil {
			nearest = []RouteRejection{{RejectServerOutput, "no supported audio target"}}
		}
		return "", nearest
	}
	return name, nil
}

// Complete and validate the contract before persisting it for the producer.
func finishConversionTarget(p *DeliveryPlan, video *SourceVideo, audio *SourceAudio, client ClientProfile, probe decoder.HardwareProbe) error {
	t := decoder.ConversionTarget{VideoCodec: p.OutputVideoCodec, BitDepth: p.OutputBitDepth, DynamicRange: p.OutputRange, ToneMap: p.ToneMap, Width: p.TargetWidth, Height: p.TargetHeight, FrameRate: p.MaxFrameRate, VideoBitrateBPS: p.VideoBitrateBPS, AudioCodec: p.OutputAudioCodec, AudioChannels: p.AudioChannels, AudioSampleRate: p.AudioSampleRate, AudioBitrateBPS: p.AudioBitrateBPS, Packaging: p.OutputContainer}
	if p.VideoAction == "convert" {
		t.RateControl = "bitrate"
		if p.QualityID == "original" || p.QualityID == "auto" || p.QualityID == "automatic" || p.QualityID == "" {
			t.RateControl = "quality"
		}
	}
	if video != nil {
		t.SourceRange = video.DynamicRange
		if p.VideoAction == "copy" {
			t.BitDepth = video.BitDepth
			t.DynamicRange = effectiveRange(video, client.videoCodec(video.Codec), client)
			// An accepted native DV track stays intact. A base fallback strips it.
			if t.DynamicRange == assets.RangeDolbyVision {
				t.DynamicRange, t.SourceRange = assets.RangeHDR10, assets.RangeHDR10
			} else {
				t.DolbyVisionProfile, t.DolbyVisionCompatibility = video.DolbyVisionProfile, video.DolbyVisionCompatibility
			}
		} else {
			t.DolbyVisionProfile, t.DolbyVisionCompatibility = video.DolbyVisionProfile, video.DolbyVisionCompatibility
			boundConvertedPicture(p, video, client.videoCodec(p.OutputVideoCodec))
			t.Width, t.Height, t.FrameRate = p.TargetWidth, p.TargetHeight, p.MaxFrameRate
			w, h := convertedDimensions(*p)
			rate := video.FrameRate
			if t.FrameRate > 0 {
				rate = min(rate, t.FrameRate)
			}
			if p.OutputVideoCodec == "h264" {
				t.VideoLevel = decoder.H264OutputLevel(w, h, rate)
			}
			if p.OutputVideoCodec == "hevc" {
				t.VideoLevel = decoder.HEVCOutputLevel(w, h, rate)
			}
		}
	}
	if audio != nil {
		if p.AudioAction == "copy" {
			t.AudioChannels, t.AudioSampleRate, t.AudioBitrateBPS = audio.Channels, audio.SampleRate, int(audio.BitRate)
			if t.AudioCodec == "flac" {
				t.AudioBitrateBPS = 0
			}
		}
		if t.AudioChannels == audio.Channels && (audio.Layout == "5.1" || audio.Layout == "5.1(side)" || audio.Layout == "7.1" || audio.Layout == "mono" || audio.Layout == "stereo") {
			t.AudioChannelLayout = audio.Layout
		}
		if t.AudioCodec == "flac" {
			t.AudioBitDepth = audio.BitDepth
			if t.AudioBitDepth == 0 {
				t.AudioBitDepth = 24
			}
			if t.AudioSampleRate != audio.SampleRate && audio.SampleRate > 0 {
				p.AudioLossless = false
			}
		}
	}
	var err error
	t, err = t.Normalized()
	if err != nil {
		return err
	}
	p.ConversionTarget = &t
	if probe.Available {
		copy := probe
		p.ConversionProbe = &copy
	}
	graph, err := p.conversionGraph(nil)
	if err != nil {
		return err
	}
	p.HardwareBackend = string(graph.Backend)
	if graph.Backend == decoder.BackendSoftware {
		p.HardwareNative = false
		p.ConversionProbe = nil
	}
	return nil
}

func boundConvertedPicture(p *DeliveryPlan, v *SourceVideo, cap *ClientVideoCodec) {
	if cap == nil {
		return
	}
	height := v.Height
	if height == 0 {
		height = 1080
	}
	if p.TargetHeight > 0 {
		height = min(height, p.TargetHeight)
	}
	if cap.MaxHeight > 0 {
		height = min(height, cap.MaxHeight)
	}
	if cap.MaxWidth > 0 && v.Width > cap.MaxWidth {
		height = min(height, v.Height*cap.MaxWidth/v.Width)
	}
	rate := v.FrameRate
	if cap.MaxFrameRate > 0 && rate > cap.MaxFrameRate {
		rate = cap.MaxFrameRate
		p.MaxFrameRate = rate
	}
	if cap.MaxLevel > 0 && (p.OutputVideoCodec == "h264" || p.OutputVideoCodec == "hevc") {
		for height > 2 {
			width := v.Width
			if v.Height > 0 {
				width = width * height / v.Height
			}
			level := decoder.H264OutputLevel(width, height, rate)
			if p.OutputVideoCodec == "hevc" {
				level = decoder.HEVCOutputLevel(width, height, rate)
			}
			if level <= cap.MaxLevel {
				break
			}
			height -= 2
		}
	}
	if height < v.Height || p.TargetHeight > 0 {
		p.TargetHeight = height - height%2
	}
}
