package metadataprovider

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

type ArtworkImage struct {
	ID                  int64
	Role, URL, Language string
	Score               float64
}

// Artwork reuses the existing private token/rate-budget transport. Type IDs are
// resolved from the provider's own type registry, never guessed from magic IDs.
func (s *TVDB) Artwork(ctx context.Context, id int64) ([]ArtworkImage, error) {
	if id <= 0 {
		return nil, errors.New("invalid artwork entity")
	}
	var types struct {
		Status string `json:"status"`
		Data   []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
	}
	if err := s.get(ctx, "/artwork/types", &types); err != nil {
		return nil, err
	}
	if types.Status != "success" || len(types.Data) > 128 {
		return nil, &Error{Provider: "tvdb", Code: "invalid_artwork_types"}
	}
	roles := map[int64]string{}
	for _, t := range types.Data {
		name := strings.ToLower(t.Name)
		role := ""
		switch {
		case strings.Contains(name, "poster"):
			role = "poster"
		case strings.Contains(name, "background"):
			role = "backdrop"
		case strings.Contains(name, "banner"):
			role = "banner"
		case strings.Contains(name, "icon"):
			role = "square"
		case strings.Contains(name, "logo"):
			role = "logo"
		case strings.Contains(name, "thumbnail"):
			role = "thumbnail"
		}
		if role != "" {
			roles[t.ID] = role
		}
	}
	var result struct {
		Status string `json:"status"`
		Data   struct {
			ID       int64 `json:"id"`
			Artworks []struct {
				ID       int64   `json:"id"`
				Type     int64   `json:"type"`
				Image    string  `json:"image"`
				Language string  `json:"language"`
				Score    float64 `json:"score"`
			}
		}
	}
	if err := s.get(ctx, "/series/"+strconv.FormatInt(id, 10)+"/artworks", &result); err != nil {
		return nil, err
	}
	if result.Status != "success" || result.Data.ID != id || len(result.Data.Artworks) > 4000 {
		return nil, &Error{Provider: "tvdb", Code: "invalid_artwork"}
	}
	out := []ArtworkImage{}
	for _, a := range result.Data.Artworks {
		role := roles[a.Type]
		if role == "" || a.ID <= 0 || len(a.Image) > 4096 || len(a.Language) > 64 {
			continue
		}
		out = append(out, ArtworkImage{a.ID, role, a.Image, a.Language, a.Score})
		if len(out) == 80 {
			break
		}
	}
	return out, nil
}
