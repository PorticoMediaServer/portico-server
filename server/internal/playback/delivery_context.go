package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"portico.local/server/internal/capabilityreport"
	"strings"

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

// originalAudioLanguageTx reads the published original language of the exact
// selected title. Episodic and season titles inherit their show's fact; a
// shared file is never used to guess which title's metadata should win.
// metadata_language is the editor/provider locale and is deliberately not read.
func originalAudioLanguageTx(ctx context.Context, tx *sql.Tx, item string) (string, error) {
	if item == "" {
		return "", nil
	}
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT f.value
 FROM catalog_entities e
 LEFT JOIN catalog_episodes ep ON ep.entity_id=e.id
 LEFT JOIN catalog_seasons se ON se.entity_id=e.id
 JOIN screen_metadata_fields f ON f.target_kind=CASE WHEN e.kind=2 OR ep.show_id IS NOT NULL OR se.show_id IS NOT NULL THEN 'show' ELSE 'item' END
 AND f.target_id=COALESCE(ep.show_id,se.show_id,e.id) AND f.field='language'
 JOIN screen_metadata_work w ON w.target_kind=f.target_kind AND w.target_id=f.target_id
 AND w.accepted_publication=f.publication_id
 JOIN screen_metadata_publications publication ON publication.id=f.publication_id
 AND publication.target_kind=f.target_kind AND publication.target_id=f.target_id
 AND publication.decision='accepted' AND publication.provider=f.provider
 AND publication.provider IN('tmdb','tvdb','anilist')
 WHERE e.public_id=pid_blob(?) AND e.retired=0`, item).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var value string
	if json.Unmarshal([]byte(raw), &value) != nil {
		return "", nil
	}
	// Publication owns canonicalisation. Refuse unknown or malformed values
	// rather than treating the client locale or an untagged stream as evidence.
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "und" || value == "xx" || len(value) < 2 || len(value) > 3 {
		return "", nil
	}
	for _, ch := range value {
		if ch < 'a' || ch > 'z' {
			return "", nil
		}
	}
	return value, nil
}
