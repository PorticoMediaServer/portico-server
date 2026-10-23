package downloads

import "portico.local/server/internal/playback"

const ReasonDownloadQualityCap = "download_quality_exceeds_server_limit"

// ConfigureDelivery is startup-only. The same dynamic settings adapter used by
// playback supplies download ceilings; no second registry or stored toggle.
func (s *Service) ConfigureDelivery(settings playback.DeliverySettings) {
	s.deliverySettings = settings
}
func (s *Service) deliveryConfiguration() playback.DeliveryConfiguration {
	if s.deliverySettings == nil {
		return playback.DefaultDeliveryConfiguration()
	}
	return s.deliverySettings.Delivery().Normalized()
}
func clampDownloadQuality(requested string, found source, cfg playback.DeliveryConfiguration) string {
	if audioKind(found.Kind) {
		return requested
	}
	cap, limited := playback.LadderPreset(cfg.Normalized().DownloadMaxQuality)
	if !limited {
		return requested
	}
	if cap.SameAsSource(found.Height, found.VideoBitrateBPS) {
		return requested
	}
	if rung, ok := playback.LadderPreset(requested); ok && rung.Height <= cap.Height && rung.VideoBitrateBPS <= cap.VideoBitrateBPS {
		return requested
	}
	return cap.ID
}
