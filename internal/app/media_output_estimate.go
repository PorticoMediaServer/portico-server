package app

import (
	"math"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/optimized"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

const unknownMediaOutputReservation = int64(2 << 30)

func predictedPlaybackOutputBytes(plan playbackplan.Plan, facts mediafacts.Facts) int64 {
	if facts.Source.SizeBytes > 0 && playbackPlanCopiesVideo(plan) {
		return saturatingEstimate(float64(facts.Source.SizeBytes) * 1.10)
	}
	durationSeconds := float64(plan.Timeline.DurationUS) / 1_000_000
	if durationSeconds <= 0 {
		if facts.Source.SizeBytes > 0 {
			return saturatingEstimate(float64(facts.Source.SizeBytes) * 1.5)
		}
		return unknownMediaOutputReservation
	}
	videoRate := plan.Constraints.MaxVideoBitrate
	if videoRate <= 0 && len(facts.Video) > 0 {
		pixels := int64(facts.Video[0].CodedWidth) * int64(facts.Video[0].CodedHeight)
		switch {
		case pixels >= 3840*2160:
			videoRate = 24_000_000
		case pixels >= 1920*1080:
			videoRate = 10_000_000
		case pixels >= 1280*720:
			videoRate = 5_000_000
		default:
			videoRate = 2_500_000
		}
	}
	audioRate := plan.Constraints.MaxAudioBitrate
	if audioRate <= 0 {
		audioRate = 768_000
	}
	// Include HLS/container overhead plus a bounded safety margin for VBR peaks.
	return saturatingEstimate(durationSeconds * float64(videoRate+audioRate) / 8 * 1.20)
}

func predictedOptimizedOutputBytes(plan optimized.OutputPlan, facts mediafacts.Facts, durationSeconds int) int64 {
	if durationSeconds <= 0 && facts.DurationUS > 0 {
		durationSeconds = int(math.Ceil(float64(facts.DurationUS) / 1_000_000))
	}
	if durationSeconds <= 0 {
		if facts.Source.SizeBytes > 0 {
			return saturatingEstimate(float64(facts.Source.SizeBytes) * 2)
		}
		return unknownMediaOutputReservation
	}
	pixels := int64(plan.Geometry.Width) * int64(plan.Geometry.Height)
	videoRate := int64(4_000_000)
	switch {
	case pixels >= 3840*2160:
		videoRate = 40_000_000
	case pixels >= 1920*1080:
		videoRate = 18_000_000
	case pixels >= 1280*720:
		videoRate = 10_000_000
	default:
		videoRate = 5_000_000
	}
	// Efficient codecs normally use less, but reservations must cover difficult
	// grain/noise sources and quality-based encoders rather than average files.
	if plan.PresetID != "" && (plan.EncoderRoute == optimized.RouteSoftwareHEVC || plan.EncoderRoute == optimized.RouteSoftwareAV1) {
		videoRate = videoRate * 4 / 5
	}
	audioRate := int64(1_000_000)
	if plan.Audio.Channels <= 2 {
		audioRate = 320_000
	}
	byRate := saturatingEstimate(float64(durationSeconds) * float64(videoRate+audioRate) / 8 * 1.25)
	if facts.Source.SizeBytes > 0 {
		bySource := saturatingEstimate(float64(facts.Source.SizeBytes) * 2)
		if bySource > byRate {
			return bySource
		}
	}
	return byRate
}

func playbackPlanCopiesVideo(plan playbackplan.Plan) bool {
	for _, stream := range plan.Streams {
		if stream.Kind == "video" {
			return stream.Action == playbackplan.Copy
		}
	}
	return false
}

func saturatingEstimate(value float64) int64 {
	if math.IsNaN(value) || value <= float64(mediaDiskReservationMinimum) {
		return mediaDiskReservationMinimum
	}
	if math.IsInf(value, 0) || value >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(math.Ceil(value))
}
