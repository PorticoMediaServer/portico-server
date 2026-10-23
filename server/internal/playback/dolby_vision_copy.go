package playback

import (
	"portico.local/server/internal/assets"
	"portico.local/server/internal/decoder"
)

// profile81CopyPicture is a prospective delivery fact, never a replacement for
// source metadata. Both observed source layers and the selected packet-qualified
// bundle are required before evaluating the client's exact 8.1 eligibility.
func profile81CopyPicture(video *SourceVideo, client ClientProfile, facts *decoder.ToolchainFacts) *SourceVideo {
	if video == nil || video.Codec != "hevc" || video.BitDepth != 10 || video.DynamicRange != assets.RangeDolbyVision || video.DolbyVisionProfile != 7 || video.DolbyVisionCompatibility != 6 || !video.DolbyVisionMetadataKnown || !video.DolbyVisionRPU || !video.DolbyVisionBaseLayer || !video.EnhancementLayer {
		return nil
	}
	target := decoder.ConversionTarget{VideoCodec: "hevc", BitDepth: 10, SourceRange: assets.RangeDolbyVision, DynamicRange: assets.RangeDolbyVision, DolbyVisionProfile: 7, DolbyVisionCompatibility: 6, CopyDolbyVision: true, DolbyVisionConversion: decoder.DolbyVisionProfile7To81, Packaging: "fmp4_hls"}
	if _, err := decoder.DolbyVisionCopyBitstreamFilter(target, facts); err != nil {
		return nil
	}
	copy := *video
	copy.DolbyVisionProfile, copy.DolbyVisionCompatibility = target.DeliveredDolbyVision()
	copy.EnhancementLayer = false
	if !nativeDolbyVision(&copy, client.videoCodec(copy.Codec), client) {
		return nil
	}
	return &copy
}

func fmp4CopyTransports(transports []ClientTransport) []ClientTransport {
	out := []ClientTransport{}
	for _, transport := range transports {
		if transport.Transport == TransportHLSFMP4 {
			out = append(out, transport)
		}
	}
	return out
}

func bestSourceCopyTransport(transports []ClientTransport, client ClientProfile, video *SourceVideo, audio *SourceAudio, allowHDR bool, facts *decoder.ToolchainFacts) (string, []RouteRejection) {
	if delivered := profile81CopyPicture(video, client, facts); delivered != nil && allowHDR {
		if name, rejects := bestCopyTransport(fmp4CopyTransports(transports), client, delivered, audio, allowHDR, true); len(rejects) == 0 {
			return name, nil
		}
	}
	name, rejects := bestCopyTransport(transports, client, video, audio, allowHDR, true)
	return name, append(rejects, baseCopyRejections(video, client, facts)...)
}

func bestSourceAudioConversionTransport(transports []ClientTransport, client ClientProfile, video *SourceVideo, audio *SourceAudio, target QualityTarget, outputs ConversionOutputs, facts *decoder.ToolchainFacts) (string, []RouteRejection) {
	if delivered := profile81CopyPicture(video, client, facts); delivered != nil && target.AllowHDR {
		if name, rejects := bestAudioConversionTransport(fmp4CopyTransports(transports), client, delivered, audio, target, outputs); len(rejects) == 0 {
			return name, nil
		}
	}
	name, rejects := bestAudioConversionTransport(transports, client, video, audio, target, outputs)
	return name, append(rejects, baseCopyRejections(video, client, facts)...)
}

// deliveredDolbyVision is used only by HLS signalling. The trace continues to
// describe the source, including profile 7 after an actual 7 -> 8.1 rewrite.
func deliveredDolbyVision(p *DeliveryPlan) (profile, compatibility int) {
	if p == nil || p.Trace == nil || p.Trace.Video == nil || p.Trace.Video.OutputRange != assets.RangeDolbyVision {
		return 0, 0
	}
	if p.ConversionTarget != nil {
		return p.ConversionTarget.DeliveredDolbyVision()
	}
	return p.Trace.Video.DolbyVisionProfile, p.Trace.Video.DolbyVisionCompatibility
}
