package playback

import "portico.local/server/internal/decoder"

// encodedPictureProfile is the explicit profile option in BuildConversion's
// committed recipe, not a decoder capability or an observed bitstream profile.
// Other encoders without an explicit graph profile remain unknown here.
func encodedPictureProfile(target decoder.ConversionTarget) string {
	target, err := target.Normalized()
	if err != nil || target.CopyDolbyVision {
		return ""
	}
	switch target.VideoCodec {
	case "h264":
		if target.BitDepth != 8 || target.DynamicRange != "sdr" {
			return ""
		}
		if target.VideoProfile != "" {
			return target.VideoProfile
		}
		return "high"
	case "hevc":
		switch target.BitDepth {
		case 8:
			return "main"
		case 10:
			return "main10"
		}
	}
	return ""
}
