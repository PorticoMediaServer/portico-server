package playback

import (
	"context"
	"database/sql"
	"fmt"
	"portico.local/server/internal/capabilityreport"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

// DeliveryContext is what the HTTP edge knows about one request that planning
// cannot work out for itself: how far the client is and what it says it is on.
// It travels on the context because session creation is reached through several
// routes (direct create, queue transition, recovery) that share no signature.
type DeliveryContext struct {
	NetworkClass   NetworkClass
	ServerLocality string
	TransportClass string
	DeviceClass    string
}

type deliveryContextKey struct{}

// WithDeliveryContext attaches the edge's observation. A context without one
// resolves to the unknown lane, which is a real policy and not an error.
func WithDeliveryContext(ctx context.Context, d DeliveryContext) context.Context {
	if d.NetworkClass == "" {
		d.NetworkClass = NetworkUnknown
	}
	if d.ServerLocality == "" {
		d.ServerLocality = LocalityUnknown
	}
	d.TransportClass = NormalizeTransportClass(d.TransportClass)
	return context.WithValue(ctx, deliveryContextKey{}, d)
}

// DeliveryContextFrom reads the edge observation, defaulting to unknown.
func DeliveryContextFrom(ctx context.Context) DeliveryContext {
	if ctx != nil {
		if d, ok := ctx.Value(deliveryContextKey{}).(DeliveryContext); ok {
			return d
		}
	}
	return DeliveryContext{NetworkClass: NetworkUnknown, ServerLocality: LocalityUnknown, TransportClass: "unknown"}
}

// DeliveryPreferenceSource reads one viewer's effective preferences for a device
// class inside the caller's transaction. It is the seam to the preference
// registry; a nil source leaves planning on the registry defaults.
type DeliveryPreferenceSource func(*sql.Tx, identity.Viewer, string) (DeliveryPreferenceReader, error)

// ConfigureDelivery wires owner settings, viewer preferences and the hardware
// detector. Every argument is optional: a server that wires none still plans
// against the published defaults and software encoding.
func (s *Service) ConfigureDelivery(settings DeliverySettings, preferences DeliveryPreferenceSource, hardware *decoder.HardwareDetector, ffmpeg string) {
	s.deliverySettings = settings
	s.deliveryPreferences = preferences
	s.hardware = hardware
	s.hardwareFFmpeg = ffmpeg
}

// DeliveryConfiguration is the owner configuration in force, for callers that
// have to reason about it without re-reading the settings registry.
func (s *Service) DeliveryConfiguration() DeliveryConfiguration {
	return deliveryConfiguration(s.deliverySettings)
}

// HardwareReport probes every backend for the owner capacity read. A server
// without a detector reports software only, which is the truth for it.
func (s *Service) HardwareReport(ctx context.Context) []decoder.HardwareProbe {
	cfg := s.DeliveryConfiguration()
	if s.hardware == nil || s.hardwareFFmpeg == "" {
		return []decoder.HardwareProbe{{Backend: decoder.BackendSoftware, Available: true, Reason: decoder.ProbeAvailable, Encoder: "libx264"}}
	}
	return s.hardware.Report(ctx, s.hardwareFFmpeg, cfg.HardwareDevice)
}

// selectHardware resolves the backend for one session. Selection is cached by
// the detector, so this is a map read after the first probe on a host.
func (s *Service) selectHardware(ctx context.Context, cfg DeliveryConfiguration) decoder.HardwareProbe {
	if s.hardware == nil || s.hardwareFFmpeg == "" || cfg.HardwareBackend == string(decoder.BackendSoftware) {
		s.noteHardware(cfg.HardwareBackend, true, "")
		return decoder.HardwareProbe{Backend: decoder.BackendSoftware, Available: true, Reason: decoder.ProbeDisabled}
	}
	// Never probe on a request: a cold cache answers software now and warms itself.
	_ = ctx
	probe := s.hardware.SelectCached(s.hardwareFFmpeg, cfg.HardwareBackend, cfg.HardwareDevice)
	// A backend the owner chose explicitly that doesn't work is reported with the
	// probe's reason; "auto" falling back to software is normal, and clears a
	// failure reported while a backend was chosen.
	switch {
	case cfg.HardwareBackend != "" && cfg.HardwareBackend != "auto":
		if probe.Reason != decoder.ProbeWarming {
			s.noteHardware(cfg.HardwareBackend, string(probe.Backend) == cfg.HardwareBackend && probe.Available, string(probe.Reason))
		}
	default:
		s.noteHardware(cfg.HardwareBackend, true, "")
	}
	return probe
}

// hardwareState is what the hardware capability report last said.
type hardwareState struct {
	chosen, reason string
	ok             bool
}

// noteHardware reports the hardware capability only when it changes: a
// session start is a pointer compare, not a lock, a formatted time and an
// error per request (review P22).
func (s *Service) noteHardware(chosen string, ok bool, reason string) {
	next := hardwareState{chosen: chosen, reason: reason, ok: ok}
	if ok {
		next.chosen, next.reason = "", "" // Every usable state reads the same.
	}
	if last := s.hardwareSeen.Load(); last != nil && *last == next {
		return
	}
	if last := s.hardwareSeen.Load(); last == nil && ok {
		s.hardwareSeen.Store(&next) // Nothing was reported yet, and nothing is wrong.
		return
	}
	s.hardwareSeen.Store(&next)
	capabilityreport.Report(capabilityreport.Hardware, ok, "hardware_unavailable", fmt.Errorf("the %s encoder the owner chose isn't usable: %s", chosen, reason))
}

// viewerPreferences reads this viewer's effective preferences for the device
// class the edge observed. A failed read is nil, which resolves to the defaults.
func (s *Service) viewerPreferences(tx *sql.Tx, p identity.Principal, d DeliveryContext) DeliveryPreferenceReader {
	if s.deliveryPreferences != nil {
		if read, err := s.deliveryPreferences(tx, p.Viewer, d.DeviceClass); err == nil {
			return read
		}
	}
	return nil
}

// resolvePolicy combines the edge observation with this viewer's preferences.
func (s *Service) resolvePolicy(tx *sql.Tx, p identity.Principal, d DeliveryContext, cfg DeliveryConfiguration) ResolvedDeliveryPolicy {
	return ResolveDeliveryPolicy(s.viewerPreferences(tx, p, d), d.NetworkClass, d.ServerLocality, d.TransportClass, cfg)
}

// preferredAudioLanguages reads the viewer's ordered language list. The list
// accessor is optional on the reader, so a test double need not implement it.
func preferredAudioLanguages(values DeliveryPreferenceReader) []string {
	if lister, ok := values.(interface{ List(string) []string }); ok && values != nil {
		return lister.List("playback.preferredAudioLanguages")
	}
	return nil
}

// automaticSubtitles is what the viewer wants from subtitles without being
// asked, beside the audio language this session will actually play.
func automaticSubtitles(values DeliveryPreferenceReader, trace *DecisionTrace) subtitles.AutoChoice {
	var out subtitles.AutoChoice
	if trace != nil && trace.Audio != nil {
		out.AudioLanguage = trace.Audio.Language
		languages := preferredAudioLanguages(values)
		if languageKey(out.AudioLanguage) != "" && len(languages) > 0 {
			out.ForeignAudio = true
			for _, lang := range languages {
				if languageKey(lang) == languageKey(out.AudioLanguage) {
					out.ForeignAudio = false
					break
				}
			}
		}
	}
	if values == nil {
		return out
	}
	mode := values.Text("playback.subtitleMode")
	out.Enabled, out.Off = mode == "always", mode == "off"
	if lister, ok := values.(interface{ List(string) []string }); ok {
		out.Languages = lister.List("playback.preferredSubtitleLanguages")
	}
	return out
}
