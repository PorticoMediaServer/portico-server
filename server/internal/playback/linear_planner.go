package playback

import "portico.local/server/internal/decoder"

// PlanLinearDelivery uses the ordinary planner with the current linear
// publisher's transport. Source rates may be unknown on a live input; explicit
// ceilings still require a conversion rather than pretending those facts fit.
func PlanLinearDelivery(source DeliverySource, client ClientProfile, q ControlQuality, converting bool) (DeliveryPlan, error) {
	transports := []ClientTransport{}
	for _, t := range client.Transports {
		if t.Transport == TransportHLSTS {
			transports = append(transports, t)
		}
	}
	client.Transports = transports
	// Narrowing decoder geometry applies equally to copies and conversions.
	client.Video = append([]ClientVideoCodec{}, client.Video...)
	for i := range client.Video {
		c := &client.Video[i]
		if q.MaxWidth != nil {
			c.MaxWidth = narrowCeiling(c.MaxWidth, *q.MaxWidth)
		}
		if q.MaxHeight != nil {
			c.MaxHeight = narrowCeiling(c.MaxHeight, *q.MaxHeight)
		}
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
	return planDelivery(DeliveryInput{Source: &source, Client: client, Policy: policy, Target: target, Config: DefaultDeliveryConfiguration(), TranscodingEnabled: converting, Server: decoder.CurrentToolchain()})
}
