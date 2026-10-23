package telemetry

import "portico.local/server/internal/decoder"

// HardwareFormat is the server-wide union of passed per-device probes. False
// means no verified hardware path; it does not assert a driver limitation.
// An absent EncodeHardware means this is not a Portico output format.
type HardwareFormat struct {
	ID             string `json:"id"`
	DecodeHardware bool   `json:"decodeHardware"`
	EncodeHardware *bool  `json:"encodeHardware"`
}

func hardwareStatus(settings TranscodeSettings, effective string, probes []HardwareProbe, source HardwareProbeSource) HardwareStatus {
	out := HardwareStatus{Configured: settings.HardwareBackend, Effective: effective, Device: settings.HardwareDevice, Probes: probes,
		Environment: decoder.DetectHardwareEnvironment(), Reprobe: decoder.HardwareReprobe{Status: "unavailable"}}
	if live, ok := source.(interface {
		HardwareReprobeState() decoder.HardwareReprobe
	}); ok {
		out.Reprobe = live.HardwareReprobeState()
	}
	formats := []struct {
		id, codec string
		encode    bool
	}{{"h264", "h264:8", true}, {"hevc8", "hevc:8", true}, {"hevc10", "hevc:10", true}, {"av1", "av1:8", true}, {"vp9", "vp9:8", false}, {"vc1", "vc1:8", false}, {"mpeg2", "mpeg2:8", false}}
	out.Formats = make([]HardwareFormat, 0, len(formats))
	for _, format := range formats {
		cell := HardwareFormat{ID: format.id}
		encoded := false
		for _, probe := range probes {
			if probe.Backend == "software" || !probe.Supported {
				continue
			}
			cell.DecodeHardware = cell.DecodeHardware || probe.DecodeCodecs[format.codec]
			encoded = encoded || probe.OutputEncoders[format.codec]
		}
		if format.encode {
			cell.EncodeHardware = &encoded
		}
		out.Formats = append(out.Formats, cell)
	}
	for _, probe := range probes {
		if probe.Backend == "software" {
			continue
		}
		out.Available = out.Available || probe.Supported
		out.LastProbeAt = max(out.LastProbeAt, probe.ProbedAt)
	}
	return out
}
