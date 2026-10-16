package playback

import "portico.local/server/internal/decoder"

// LinearPackaging is which segment formats a linear producer may publish. A
// buffer that already holds one kind keeps it: one playlist cannot switch.
type LinearPackaging int

const (
	LinearTSOnly LinearPackaging = iota
	LinearEither
	LinearFragmentedOnly
)

// PlanLinearDelivery uses the ordinary planner with the linear publisher's
// transports: MPEG-TS, and fragmented MP4 where the producer can publish it
// (a live input; a scheduled library channel keeps MPEG-TS, whose source clock
// its timeline is built on). Fragmented MP4 is what carries HEVC to Apple's
// players and FLAC or Opus at all. Source rates may be unknown on a live
// input; explicit ceilings still require a conversion rather than pretending
// those facts fit.
func PlanLinearDelivery(source DeliverySource, client ClientProfile, q ControlQuality, converting bool, packaging LinearPackaging) (DeliveryPlan, error) {
	transports := []ClientTransport{}
	for _, t := range client.Transports {
		if t.Transport == TransportHLSTS && packaging != LinearFragmentedOnly || t.Transport == TransportHLSFMP4 && packaging != LinearTSOnly {
			transports = append(transports, t)
		}
	}
	fragmented := packaging != LinearTSOnly
	client.Transports = transports
	// Narrowing decoder geometry applies equally to copies and conversions.
	client.Video = append([]ClientVideoCodec{}, client.Video...)
	for i := range client.Video {
		client.Video[i].narrowGeometry(q.MaxWidth, q.MaxHeight)
	}
	target := QualityTarget{RungID: "original", Kind: QualityAutomatic, AllowHDR: true}
	if q.MaxHeight != nil {
		target.MaxVideoHeight = *q.MaxHeight
	}
	if q.MaxBitrateBPS != nil {
		ceiling := *q.MaxBitrateBPS
		if len(source.Audio) > 0 {
			ceiling -= 192000
			target.MaxAudioBitrateBPS = 192000
		}
		if ceiling <= 0 {
			return DeliveryPlan{}, ErrDeliveryRefused{ReasonNoAdmissibleRoute}
		}
		target.MaxVideoBitrateBPS = ceiling
	}
	if source.Video != nil && source.BitRate == 0 && (target.MaxVideoBitrateBPS > 0 || client.MaxBitrateBPS > 0) {
		source.BitRate = assumedBitrate(source.Video.Height)
	}
	policy := ResolvedDeliveryPolicy{DirectPlay: "never", DirectStream: "allow", Transcode: "allow", AllowHDR: true}
	return planDelivery(DeliveryInput{Source: &source, Client: client, Policy: policy, Target: target, Config: DefaultDeliveryConfiguration(), TranscodingEnabled: converting, Server: decoder.CurrentToolchain(), FMP4Output: fragmented})
}
