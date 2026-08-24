package playbackcap

import (
	"fmt"
	"time"
)

// Resolution contains one internally compatible evidence set. Tuples from
// lower-priority sources are never unioned into it, preventing cross-product
// capability invention.
type Resolution struct {
	EvidenceID string
	Source     EvidenceSource
	Band       string
	Tuples     []DeliveryTuple
}

type Resolver struct {
	now        func() time.Time
	fallbacks  []FallbackBand
	catalogErr error
}

func NewResolver(fallbacks []FallbackBand) *Resolver {
	r := &Resolver{now: time.Now, fallbacks: append([]FallbackBand(nil), fallbacks...)}
	r.catalogErr = ValidateFallbackBands(r.fallbacks, time.Now())
	return r
}

func DefaultResolver() *Resolver { return NewResolver(DefaultFallbackBands()) }

func (r *Resolver) Resolve(client Client, runtime []Evidence) (Resolution, error) {
	if r.catalogErr != nil {
		return Resolution{}, fmt.Errorf("invalid fallback catalog: %w", r.catalogErr)
	}
	client = client.Normalized()
	var best *Evidence
	bestRank := -1
	for i := range runtime {
		e := runtime[i]
		if err := e.Validate(r.now()); err != nil {
			continue
		} // malformed evidence fails closed
		if !matchesClient(e, client) || e.Provenance.Source == SourceStaticFallback {
			continue
		}
		rank := sourceRank(e.Provenance.Source)*10 + confidenceRank(e.Provenance.Confidence)
		if rank > bestRank || (rank == bestRank && (best == nil || e.Provenance.ReviewedAt.After(best.Provenance.ReviewedAt) || (e.Provenance.ReviewedAt.Equal(best.Provenance.ReviewedAt) && e.ID < best.ID))) {
			best, bestRank = &e, rank
		}
	}
	if best != nil {
		return Resolution{EvidenceID: best.ID, Source: best.Provenance.Source, Band: "runtime", Tuples: cloneTuples(best.Tuples)}, nil
	}
	for _, band := range r.fallbacks {
		if !band.matches(client) {
			continue
		}
		if err := band.Evidence.Validate(r.now()); err != nil {
			continue
		}
		return Resolution{EvidenceID: band.Evidence.ID, Source: SourceStaticFallback, Band: band.Name, Tuples: cloneTuples(band.Evidence.Tuples)}, nil
	}
	return Resolution{}, fmt.Errorf("no conservative capability band for %s %s", client.Family, client.Version)
}

func matchesClient(e Evidence, client Client) bool {
	ec := e.Client.Normalized()
	if ec.Family != client.Family || (ec.Platform != "" && ec.Platform != client.Platform) || (ec.Device != "" && ec.Device != client.Device) {
		return false
	}
	return withinVersion(client.Version, e.Provenance.MinVersion, e.Provenance.MaxVersion)
}

func withinVersion(version, min, max string) bool {
	if min == "" && max == "" {
		return true
	}
	if version == "" {
		return false
	}
	if min != "" {
		c, ok := compareVersions(version, min)
		if !ok || c < 0 {
			return false
		}
	}
	if max != "" {
		c, ok := compareVersions(version, max)
		if !ok || c > 0 {
			return false
		}
	}
	return true
}

func sourceRank(source EvidenceSource) int {
	switch source {
	case SourceNativeRuntime:
		return 4
	case SourceAuthenticatedRuntime:
		return 3
	default:
		return -1
	}
}

func confidenceRank(confidence Confidence) int {
	switch confidence {
	case ConfidenceHigh:
		return 3
	case ConfidenceMedium:
		return 2
	case ConfidenceLow:
		return 1
	default:
		return 0
	}
}

func cloneTuples(in []DeliveryTuple) []DeliveryTuple { return append([]DeliveryTuple(nil), in...) }

// Supports returns true only when a single declared tuple satisfies the
// complete request. It never combines video from one tuple with audio or
// subtitles from another.
func (r Resolution) Supports(want DeliveryTuple) bool {
	if err := want.Validate(); err != nil {
		return false
	}
	for _, have := range r.Tuples {
		if tupleMatches(have, want) {
			return true
		}
	}
	return false
}

func tupleMatches(have, want DeliveryTuple) bool {
	if have.Kind != want.Kind {
		return false
	}
	if normalize(have.Protocol) != normalize(want.Protocol) || normalize(have.Container) != normalize(want.Container) {
		return false
	}
	if have.Kind == MediaAudio {
		return sameOrWildcard(have.Audio.Codec, want.Audio.Codec) && sameOrWildcard(have.Audio.Profile, want.Audio.Profile) && sameOrWildcard(have.Audio.Layout, want.Audio.Layout) && sameOrWildcard(have.Audio.Route, want.Audio.Route) && !exceeds(want.Audio.MaxChannels, have.Audio.MaxChannels) && (!want.Audio.ObjectPassthrough || have.Audio.ObjectPassthrough)
	}
	if !sameOrWildcard(have.Video.Codec, want.Video.Codec) || !sameOrWildcard(have.Video.Profile, want.Video.Profile) || !sameOrWildcard(have.Video.Level, want.Video.Level) || !sameOrWildcard(have.Video.Tag, want.Video.Tag) || !sameOrWildcard(have.Video.PixelFormat, want.Video.PixelFormat) || !sameOrWildcard(have.Video.Chroma, want.Video.Chroma) || !sameOrWildcard(have.Video.HDR, want.Video.HDR) {
		return false
	}
	if exceeds(want.Video.BitDepth, have.Video.BitDepth) || exceeds(want.Video.MaxWidth, have.Video.MaxWidth) || exceeds(want.Video.MaxHeight, have.Video.MaxHeight) || exceedsFloat(want.Video.MaxFrameRate, have.Video.MaxFrameRate) {
		return false
	}
	if want.Video.DolbyVisionProfile != 0 && want.Video.DolbyVisionProfile != have.Video.DolbyVisionProfile {
		return false
	}
	if want.Audio == (Audio{}) {
		return have.Audio == (Audio{})
	}
	if have.Audio == (Audio{}) {
		return false
	}
	if !sameOrWildcard(have.Audio.Codec, want.Audio.Codec) || !sameOrWildcard(have.Audio.Profile, want.Audio.Profile) || !sameOrWildcard(have.Audio.Layout, want.Audio.Layout) || !sameOrWildcard(have.Audio.Route, want.Audio.Route) || exceeds(want.Audio.MaxChannels, have.Audio.MaxChannels) {
		return false
	}
	if want.Audio.ObjectPassthrough && !have.Audio.ObjectPassthrough {
		return false
	}
	return normalize(string(have.Subtitle.Mode)) == normalize(string(want.Subtitle.Mode)) && sameOrWildcard(have.Subtitle.Codec, want.Subtitle.Codec) && sameOrWildcard(have.Subtitle.Kind, want.Subtitle.Kind)
}

func sameOrWildcard(have, want string) bool { return want == "" || normalize(have) == normalize(want) }
func exceeds(want, max int) bool            { return max > 0 && want > max }
func exceedsFloat(want, max float64) bool   { return max > 0 && want > max }
