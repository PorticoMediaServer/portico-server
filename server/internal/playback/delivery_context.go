package playback

import (
	"context"
	"database/sql"
	"fmt"
	"portico.local/server/internal/capabilityreport"
	"portico.local/server/internal/dbwork"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/metadatalanguage"
	"portico.local/server/internal/operations"
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
	AppLanguage    string
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

// HardwareSelection is the probe playback would execute with right now: it is
// the same call selectHardware makes, without that call's capability note, so a
// read cannot change what the server reports about itself. It never starts a
// probe: a cold cache answers software-and-warming now, exactly as a session
// would, and the background warm-up fills the cache for the next read.
func (s *Service) HardwareSelection(ctx context.Context) decoder.HardwareProbe {
	cfg := s.DeliveryConfiguration()
	if s.hardware == nil || s.hardwareFFmpeg == "" || cfg.HardwareBackend == string(decoder.BackendSoftware) {
		return decoder.HardwareProbe{Backend: decoder.BackendSoftware, Available: true, Reason: decoder.ProbeDisabled}
	}
	probe, err := s.hardware.SelectCachedContext(ctx, s.hardwareFFmpeg, cfg.HardwareBackend, cfg.HardwareDevice)
	if err != nil {
		return decoder.HardwareProbe{Backend: decoder.BackendSoftware, Reason: decoder.ProbeTimedOut}
	}
	return probe
}

// HardwareDevices names the adapters this host exposes, by the name their
// drivers give them. Discovery only: an adapter is not a capability until a
// probe has run on it.
func (s *Service) HardwareDevices(ctx context.Context) []decoder.HardwareDevice {
	if s.hardware == nil {
		return nil
	}
	discover := s.hardware.Devices
	if discover == nil {
		discover = decoder.DiscoverHardwareDevices
	}
	return discover(ctx)
}

// HardwareTest probes every backend on every adapter again now, for the owner's
// Test hardware button. It is the one deliberate re-probe: ordinary reads use
// compiled results so a settings page never starts a child process. A server
// without a detector has nothing to test.
func (s *Service) HardwareTest(ctx context.Context) {
	if s.hardware == nil || s.hardwareFFmpeg == "" {
		return
	}
	cfg := s.DeliveryConfiguration()
	// A re-test retires every ceiling measured for this runtime and adapter set
	// before it probes again: an old maximum must not outlive the configuration it
	// was measured on. Reservations held by live children are untouched.
	decoder.ForgetPermitIdentity(s.hardwareFFmpeg)
	decoder.HardwarePermits().Retire("", "", "")
	devices := s.HardwareDevices(ctx)
	if len(devices) == 0 {
		for _, backend := range decoder.HardwareBackends {
			if ctx.Err() != nil {
				return
			}
			s.hardware.Reprobe(ctx, s.hardwareFFmpeg, backend, cfg.HardwareDevice)
		}
		return
	}
	for _, device := range devices {
		for _, backend := range device.Backends {
			if ctx.Err() != nil {
				return
			}
			s.hardware.Reprobe(ctx, s.hardwareFFmpeg, backend, device.Path)
		}
	}
}

// selectHardware runs only in the request-cancellable preflight, outside SQL.
func (s *Service) selectHardware(ctx context.Context, cfg DeliveryConfiguration) (decoder.HardwareProbe, error) {
	if err := ctx.Err(); err != nil {
		return decoder.HardwareProbe{}, err
	}
	if s.hardware == nil || s.hardwareFFmpeg == "" || cfg.HardwareBackend == string(decoder.BackendSoftware) {
		s.noteHardware(cfg.HardwareBackend, true, "")
		return decoder.HardwareProbe{Backend: decoder.BackendSoftware, Available: true, Reason: decoder.ProbeDisabled}, nil
	}
	// Never probe on a request: a cold cache answers software now and warms itself.
	probe, err := s.hardware.SelectCachedContext(ctx, s.hardwareFFmpeg, cfg.HardwareBackend, cfg.HardwareDevice)
	if err != nil {
		return decoder.HardwareProbe{}, err
	}
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
	return probe, nil
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

// automaticSubtitles is what the viewer wants from subtitles without being
// asked, beside the audio language this session will actually play.
func preferenceReaderOrDefaults(values DeliveryPreferenceReader) DeliveryPreferenceReader {
	if values == nil {
		return operations.PreferenceValues{}
	}
	return values
}

func resolvePreferenceLanguages(values DeliveryPreferenceReader, key, appLanguage string) []string {
	values = preferenceReaderOrDefaults(values)
	var languages []string
	if lister, ok := values.(interface{ List(string) []string }); ok && values != nil {
		languages = lister.List(key)
	}
	locale := appLanguage
	if locale == "" && values != nil {
		locale = values.Text("region.locale")
	}
	if locale == "auto" {
		locale = ""
	}
	// The app's supported-catalogue fallback is English. A missing device fact
	// follows that fallback; it never becomes a guessed title language.
	if locale == "" {
		locale = "en-US"
	}
	out := make([]string, 0, len(languages))
	for _, lang := range languages {
		if lang == "@appLanguage" {
			lang = locale
		}
		out = append(out, lang)
	}
	return out
}

func automaticSubtitles(values DeliveryPreferenceReader, trace *DecisionTrace, appLanguages ...string) subtitles.AutoChoice {
	var out subtitles.AutoChoice
	appLanguage := ""
	if len(appLanguages) > 0 {
		appLanguage = appLanguages[0]
	}
	values = preferenceReaderOrDefaults(values)
	mode := values.Text("playback.subtitleMode")
	out.Enabled, out.Off = mode == "always", mode == "off"
	out.Languages = resolvePreferenceLanguages(values, "playback.preferredSubtitleLanguages", appLanguage)
	out.NoLanguage = len(out.Languages) == 0
	out.PreferSDH = valuesBool(values, "playback.preferSdhSubtitles", false)
	out.DisableUntaggedForced = !valuesBool(values, "playback.useUntaggedForced", true)
	out.FullFallback = values.Text("playback.forcedSubtitleFallback") == "full"
	if trace != nil && trace.Audio != nil {
		out.AudioLanguage = trace.Audio.Language
		if mode == "foreign" && languageKey(out.AudioLanguage) != "" {
			// Original language is not one of 'my languages'. With an explicit empty
			// audio list, no language is declared familiar.
			out.ForeignAudio = true
			for _, lang := range resolvePreferenceLanguages(values, "playback.preferredAudioLanguages", appLanguage) {
				if lang != "@original" && languageKey(lang) == languageKey(out.AudioLanguage) {
					out.ForeignAudio = false
					break
				}
			}
		}
	}
	return out
}

func valuesBool(values DeliveryPreferenceReader, key string, fallback bool) bool {
	if v, ok := values.(interface{ Bool(string) bool }); ok {
		return v.Bool(key)
	}
	return fallback
}

// publishedOriginalLanguageTx is server-only composition. Its caller must
// already have authorized this title in the same snapshot; neither the edge
// DeliveryContext nor a client track choice can supply or override the fact.
func publishedOriginalLanguageTx(ctx context.Context, tx *sql.Tx, item string) (string, error) {
	return metadatalanguage.Published(ctx, tx, item)
}

// KeepsSubtitleStyling is the viewer's "Styled subtitles" setting (on unless
// they turned it off). With it off, an engine that cannot draw a styled track
// shows its words as plain text instead of having it burned into the picture.
func (s *Service) KeepsSubtitleStyling(ctx context.Context, p identity.Principal) bool {
	if s.deliveryPreferences == nil {
		return true
	}
	tx, done, err := dbwork.BeginRead(ctx, s.db)
	if err != nil {
		return true
	}
	defer done()
	return s.KeepsSubtitleStylingTx(ctx, tx, p)
}

// KeepsSubtitleStylingTx is KeepsSubtitleStyling inside a read the caller holds.
func (s *Service) KeepsSubtitleStylingTx(ctx context.Context, tx *sql.Tx, p identity.Principal) bool {
	if s.deliveryPreferences == nil {
		return true
	}
	return valuesBool(preferenceReaderOrDefaults(s.viewerPreferences(tx, p, DeliveryContextFrom(ctx))), "playback.keepSubtitleStyling", true)
}
