package mediaanalysis

import (
	"bytes"
	"context"
	"image/jpeg"
	"io"

	"portico.local/server/internal/decoder"
)

const EpisodeThumbnailOperation = "episode_thumbnail"
const EpisodeThumbnailCostClass = "decode"
const EpisodeThumbnailDefaultMode = "provider_then_made"
const EpisodeThumbnailProviderMode = "provider"
const EpisodeThumbnailOffMode = "off"

// EpisodeThumbnailRequest is the seam consumed by the effective library/source
// policy owner. Authorize must recheck the effective library/source intersection
// for the supplied operation and cost class. Open acquires a retained range
// reader only after permission is granted; provider/off never acquire media.
// The publication transaction must independently recheck the same authority.
type EpisodeThumbnailRequest struct {
	Mode          string
	Manual        bool
	ProviderStill bool
	DurationUS    int64
	Open          func(context.Context) (Input, error)
	Decoder       func(Input) Decode
	Authorize     func(context.Context, string, string) error
	SourceVideo   decoder.PreviewVideoFacts
}
type EpisodeThumbnailResult struct {
	Origin        string
	JPEG          []byte
	Width, Height int
	PositionUS    int64
}

// EpisodeThumbnailMode is the product default independent of settings layout.
// Callers apply the library/source policy before invoking generation.
func EpisodeThumbnailMode(libraryKind string) string {
	if libraryKind == "tv" || libraryKind == "anime" {
		return EpisodeThumbnailDefaultMode
	}
	return EpisodeThumbnailOffMode
}

// MakeEpisodeThumbnail produces exactly one bounded picture, or chooses an
// already available provider still. Manual generation obeys the same permission
// callback and cannot bypass an off/provider choice. ProviderStill describes an
// already permitted provider picture; this function never performs a lookup.
func MakeEpisodeThumbnail(ctx context.Context, r EpisodeThumbnailRequest) (EpisodeThumbnailResult, error) {
	out := EpisodeThumbnailResult{Origin: "fallback"}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	switch r.Mode {
	case EpisodeThumbnailOffMode:
		if r.Manual {
			return out, ErrUnsupported
		}
		return out, nil
	case EpisodeThumbnailProviderMode:
		if r.Manual {
			return out, ErrUnsupported
		}
		if r.ProviderStill {
			out.Origin = "provider"
		}
		return out, nil
	case EpisodeThumbnailDefaultMode:
	default:
		return out, ErrInput
	}
	if r.ProviderStill && !r.Manual {
		out.Origin = "provider"
		return out, nil
	}
	if r.Open == nil || r.Decoder == nil || r.Authorize == nil || r.DurationUS <= 0 || r.DurationUS > (1<<53)-1 {
		return out, ErrInput
	}
	authorize := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return r.Authorize(ctx, EpisodeThumbnailOperation, EpisodeThumbnailCostClass)
	}
	if err := authorize(); err != nil {
		return out, err
	}
	input, err := r.Open(ctx)
	if err != nil {
		return out, err
	}
	if input == nil {
		return out, ErrInput
	}
	defer input.Close()
	if err := input.Validate(ctx); err != nil {
		return out, err
	}
	if err := authorize(); err != nil {
		return out, err
	}
	// Ten percent avoids most opening cards; a short episode still has a valid
	// seek strictly inside its duration. Decode uses the existing ranged bridge.
	position := min(r.DurationUS-1, r.DurationUS/10)
	spec := decoder.AnalysisSpec{Kind: "image", StartUS: position, MaxDurationUS: r.DurationUS, SourceVideo: r.SourceVideo}
	var data []byte
	decode := r.Decoder(input)
	if decode == nil {
		return out, ErrUnsupported
	}
	err = decode(ctx, EpisodeThumbnailOperation, spec, func(reader io.Reader) error {
		var err error
		data, err = io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		if len(data) > 1<<20 {
			return ErrBudget
		}
		return err
	})
	if err != nil {
		return out, err
	}
	shape, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || shape.Width != 320 || shape.Height != 180 {
		return out, ErrUnsupported
	}
	if _, err = jpeg.Decode(bytes.NewReader(data)); err != nil {
		return out, ErrUnsupported
	}
	if err = input.Validate(ctx); err != nil {
		return out, err
	}
	if err = authorize(); err != nil {
		return out, err
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	out = EpisodeThumbnailResult{Origin: "generated", JPEG: data, Width: 320, Height: 180, PositionUS: position}
	return out, nil
}

// produceEpisodeThumbnail stages immutable output for the existing durable
// publisher. The policy owner selects this operation and supplies Authorize.
func (s *Service) produceEpisodeThumbnail(ctx context.Context, r EpisodeThumbnailRequest, key string) (produced, error) {
	out := produced{Artifacts: []artifactOutput{}, Markers: []Marker{}}
	picture, err := MakeEpisodeThumbnail(ctx, r)
	if err != nil {
		return out, err
	}
	out.Summary = map[string]any{"origin": picture.Origin, "operation": EpisodeThumbnailOperation, "costClass": EpisodeThumbnailCostClass}
	if picture.Origin == "generated" {
		err = s.adder(ctx, key, &out)(picture.JPEG, EpisodeThumbnailOperation, "image/jpeg", 0, 0, r.DurationUS, picture.Width, picture.Height)
	}
	return out, err
}
