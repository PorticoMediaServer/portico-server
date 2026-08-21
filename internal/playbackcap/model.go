// Package playbackcap models media delivery as compatible tuples.
//
// A tuple is an assertion that all of its protocol, container, elementary
// stream, audio-route, and subtitle properties work together. It is not proof
// that a particular device can physically decode a stream; static entries are
// deliberately treated as conservative potential compatibility.
package playbackcap

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type EvidenceSource string

const (
	SchemaVersion                             = "playback-capability-v2"
	SourceStaticFallback       EvidenceSource = "static_fallback"
	SourceUnauthenticatedProbe EvidenceSource = "unauthenticated_probe"
	SourceAuthenticatedRuntime EvidenceSource = "authenticated_runtime"
	SourceNativeRuntime        EvidenceSource = "native_runtime"
)

type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

type Client struct {
	Family, Version, Platform, Device string
}

func (c Client) Normalized() Client {
	return Client{normalize(c.Family), strings.TrimSpace(c.Version), normalize(c.Platform), normalize(c.Device)}
}

type Provenance struct {
	Source          EvidenceSource
	Confidence      Confidence
	Producer        string
	ProducerVersion string
	SchemaVersion   string
	MinVersion      string
	MaxVersion      string
	ReviewedAt      time.Time
	Authenticated   bool
}

type Video struct {
	Codec, Profile, Level, Tag, PixelFormat, Chroma, HDR string
	BitDepth, DolbyVisionProfile                         int
	MaxWidth, MaxHeight                                  int
	MaxFrameRate                                         float64
}

type Audio struct {
	Codec, Profile, Layout, Route string
	MaxChannels                   int
	ObjectPassthrough             bool
}

type SubtitleMode string

const (
	SubtitleNone    SubtitleMode = "none"
	SubtitleNative  SubtitleMode = "native"
	SubtitleConvert SubtitleMode = "convert"
	SubtitleBurn    SubtitleMode = "burn"
)

type Subtitle struct {
	Codec, Kind string
	Mode        SubtitleMode
}

type MediaKind string

const (
	MediaAudiovisual MediaKind = "audiovisual"
	MediaAudio       MediaKind = "audio"
)

type DeliveryTuple struct {
	Kind                MediaKind
	Protocol, Container string
	Video               Video
	Audio               Audio
	Subtitle            Subtitle
}

type Evidence struct {
	ID         string
	Client     Client
	Provenance Provenance
	Tuples     []DeliveryTuple
}

func (e Evidence) Validate(now time.Time) error {
	if strings.TrimSpace(e.ID) == "" || normalize(e.Client.Family) == "" {
		return errors.New("evidence id and client family are required")
	}
	if strings.TrimSpace(e.Provenance.Producer) == "" {
		return errors.New("evidence producer is required")
	}
	switch e.Provenance.Source {
	case SourceStaticFallback, SourceUnauthenticatedProbe, SourceAuthenticatedRuntime, SourceNativeRuntime:
	default:
		return fmt.Errorf("invalid evidence source %q", e.Provenance.Source)
	}
	switch e.Provenance.Confidence {
	case ConfidenceLow, ConfidenceMedium, ConfidenceHigh:
	default:
		return fmt.Errorf("invalid confidence %q", e.Provenance.Confidence)
	}
	if e.Provenance.ReviewedAt.IsZero() || e.Provenance.ReviewedAt.After(now.Add(24*time.Hour)) {
		return errors.New("review date is missing or invalid")
	}
	if maxAge := evidenceMaximumAge(e.Provenance.Source); maxAge > 0 && e.Provenance.ReviewedAt.Before(now.Add(-maxAge)) {
		return errors.New("runtime evidence is stale")
	}
	if (e.Provenance.Source == SourceStaticFallback || e.Provenance.Source == SourceUnauthenticatedProbe) && e.Provenance.Authenticated {
		return errors.New("static/unauthenticated evidence cannot be authenticated")
	}
	if (e.Provenance.Source == SourceAuthenticatedRuntime || e.Provenance.Source == SourceNativeRuntime) && !e.Provenance.Authenticated {
		return errors.New("authenticated/native runtime evidence must be authenticated")
	}
	if e.Provenance.Source == SourceAuthenticatedRuntime || e.Provenance.Source == SourceNativeRuntime {
		if e.Provenance.SchemaVersion != SchemaVersion || strings.TrimSpace(e.Provenance.ProducerVersion) == "" {
			return errors.New("runtime evidence schema and producer version are required")
		}
	}
	if len(e.Tuples) == 0 {
		return errors.New("at least one compatible tuple is required")
	}
	for i, tuple := range e.Tuples {
		if err := tuple.Validate(); err != nil {
			return fmt.Errorf("tuple %d: %w", i, err)
		}
	}
	if e.Provenance.MinVersion != "" && e.Provenance.MaxVersion != "" {
		cmp, ok := compareVersions(e.Provenance.MinVersion, e.Provenance.MaxVersion)
		if !ok || cmp > 0 {
			return errors.New("invalid evidence version bounds")
		}
	}
	return nil
}

func evidenceMaximumAge(source EvidenceSource) time.Duration {
	switch source {
	case SourceUnauthenticatedProbe:
		return 5 * time.Minute
	case SourceAuthenticatedRuntime:
		return 15 * time.Minute
	case SourceNativeRuntime:
		return 24 * time.Hour
	default:
		return 0
	}
}

func (t DeliveryTuple) Validate() error {
	if normalize(t.Protocol) == "" || normalize(t.Container) == "" || normalize(t.Audio.Codec) == "" {
		return errors.New("protocol, container, and audio codec are required")
	}
	if t.Audio.MaxChannels <= 0 {
		return errors.New("audio channel limit must be positive")
	}
	switch t.Kind {
	case MediaAudiovisual:
		if normalize(t.Video.Codec) == "" {
			return errors.New("audiovisual tuple requires a video codec")
		}
		if t.Video.BitDepth <= 0 || t.Video.MaxWidth <= 0 || t.Video.MaxHeight <= 0 || t.Video.MaxFrameRate <= 0 {
			return errors.New("audiovisual bit depth, dimensions, and frame rate must be positive")
		}
	case MediaAudio:
		if t.Video != (Video{}) {
			return errors.New("audio-only tuple cannot declare video capabilities")
		}
		if t.Subtitle != (Subtitle{Mode: SubtitleNone}) {
			return errors.New("audio-only tuple cannot declare subtitle capabilities")
		}
	default:
		return errors.New("invalid media kind")
	}
	if t.Video.DolbyVisionProfile < 0 || t.Video.DolbyVisionProfile > 9 {
		return errors.New("invalid Dolby Vision profile")
	}
	if t.Video.DolbyVisionProfile != 0 && normalize(t.Video.HDR) != "dolby_vision" {
		return errors.New("Dolby Vision profile requires Dolby Vision HDR mode")
	}
	if t.Audio.ObjectPassthrough && normalize(t.Audio.Route) != "passthrough" {
		return errors.New("object audio passthrough requires passthrough route")
	}
	switch t.Subtitle.Mode {
	case SubtitleNone:
		if t.Subtitle.Codec != "" || t.Subtitle.Kind != "" {
			return errors.New("subtitle none cannot declare codec or kind")
		}
	case SubtitleNative, SubtitleConvert, SubtitleBurn:
		if normalize(t.Subtitle.Codec) == "" || normalize(t.Subtitle.Kind) == "" {
			return errors.New("subtitle mode requires codec and kind")
		}
	default:
		return errors.New("invalid subtitle mode")
	}
	return nil
}

func normalize(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func compareVersions(a, b string) (int, bool) {
	parse := func(s string) ([]int, bool) {
		s = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(s), "v"))
		if s == "" {
			return nil, false
		}
		parts := strings.Split(s, ".")
		out := make([]int, len(parts))
		for i, part := range parts {
			end := 0
			for end < len(part) && part[end] >= '0' && part[end] <= '9' {
				end++
			}
			if end == 0 {
				return nil, false
			}
			n, err := strconv.Atoi(part[:end])
			if err != nil {
				return nil, false
			}
			out[i] = n
		}
		return out, true
	}
	av, aok := parse(a)
	bv, bok := parse(b)
	if !aok || !bok {
		return 0, false
	}
	for len(av) < len(bv) {
		av = append(av, 0)
	}
	for len(bv) < len(av) {
		bv = append(bv, 0)
	}
	for i := range av {
		if av[i] < bv[i] {
			return -1, true
		}
		if av[i] > bv[i] {
			return 1, true
		}
	}
	return 0, true
}
