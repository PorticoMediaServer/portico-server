package playback

import (
	"context"
	"portico.local/server/internal/decoder"
)

func (s *Service) ReprobeHardware(ctx context.Context) decoder.HardwareReprobe {
	if ctx.Err() != nil {
		return decoder.HardwareReprobe{Status: "cancelled"}
	}
	if s.hardware == nil || s.hardwareFFmpeg == "" {
		return decoder.HardwareReprobe{Status: "unavailable"}
	}
	return s.hardware.StartReprobe(s.hardwareFFmpeg)
}
