package metadata

import (
	"context"
	"errors"
	"sort"
	"strconv"

	"portico.local/server/internal/metadataprovider"
)

func (s *Service) discoverTVDBArtwork(ctx context.Context, id string) ([]discoveredArtwork, error) {
	return s.discoverTVDBArtworkFor(ctx, id, "show", "en")
}
func (s *Service) discoverTVDBArtworkFor(ctx context.Context, id, kind, locale string) ([]discoveredArtwork, error) {
	preference, photos, settingsErr := s.artworkPreferences(ctx)
	if settingsErr != nil {
		return nil, settingsErr
	}
	if preference == "en" {
		locale = "en"
	}
	p, ok := s.tvdb.(interface {
		Artwork(context.Context, int64) ([]metadataprovider.ArtworkImage, error)
	})
	if !ok {
		return nil, errors.New("artwork_provider_unavailable")
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return nil, ErrRepairInput
	}
	var images []metadataprovider.ArtworkImage
	if modern, ok := s.tvdb.(interface {
		ArtworkForKind(context.Context, string, int64, string) ([]metadataprovider.ArtworkImage, error)
	}); ok {
		typ := "show"
		if kind != "show" {
			typ = "movie"
		}
		images, err = modern.ArtworkForKind(ctx, typ, n, locale)
	} else {
		images, err = p.Artwork(ctx, n)
	}
	if err != nil {
		return nil, err
	}
	out := []discoveredArtwork{}
	for _, im := range images {
		if (photos || im.Role != "portrait") && validArtworkRole(im.Role) && artworkURLAllowed(im.URL, "tvdb") {
			out = append(out, discoveredArtwork{role: im.Role, provider: "tvdb", imageID: strconv.FormatInt(im.ID, 10), origin: im.URL, locale: im.Language, attribution: artworkAttribution("tvdb"), rank: im.Score, votes: -1})
		}
	}
	// Season posters and cast photos (Spec — Page Content §2).
	if kind == "show" {
		out = append(out, s.tvdbPageArtwork(ctx, n)...)
	}
	if !photos {
		filtered := out[:0]
		for _, im := range out {
			if im.role != "portrait" {
				filtered = append(filtered, im)
			}
		}
		out = filtered
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].role != out[j].role {
			return out[i].role < out[j].role
		}
		a, b := preferredArtworkLanguageRank(out[i].locale, locale, preference), preferredArtworkLanguageRank(out[j].locale, locale, preference)
		if a != b {
			return a < b
		}
		return out[i].rank > out[j].rank
	})
	return out, nil
}
