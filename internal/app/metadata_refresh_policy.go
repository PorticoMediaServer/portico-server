package app

import (
	"context"
	"database/sql"
	"strings"
)

type metadataRefreshIntent string

const (
	metadataRefreshFillMissing metadataRefreshIntent = "fill_missing"
	metadataRefreshUnlocked    metadataRefreshIntent = "refresh_unlocked"
)

func normalizeMetadataRefreshIntent(value string) metadataRefreshIntent {
	switch metadataRefreshIntent(strings.ToLower(strings.TrimSpace(value))) {
	case metadataRefreshFillMissing:
		return metadataRefreshFillMissing
	default:
		return metadataRefreshUnlocked
	}
}

func metadataRefreshIntentFromJob(job Job) metadataRefreshIntent {
	if job.Metadata == nil {
		return metadataRefreshUnlocked
	}
	return normalizeMetadataRefreshIntent(job.Metadata["refreshIntent"])
}

// filterMetadataUpdateForIntent makes fallback supplementation a field-level
// operation. It retains the provider snapshot/evidence but prevents a lower
// priority provider from becoming a second blanket canonical writer.
func filterMetadataUpdateForIntent(tx *sql.Tx, state metadataCanonicalState, update *UpdateMediaRequest, intent metadataRefreshIntent) error {
	if update == nil || intent != metadataRefreshFillMissing {
		return nil
	}
	if strings.TrimSpace(state.Title) != "" {
		update.Title = nil
	}
	if strings.TrimSpace(state.SortTitle) != "" {
		update.SortTitle = nil
	}
	if strings.TrimSpace(state.OriginalTitle) != "" {
		update.OriginalTitle = nil
	}
	if strings.TrimSpace(state.Edition) != "" {
		update.Edition = nil
	}
	if state.Year > 0 {
		update.Year = nil
	}
	// Provider fallback is descriptive supplementation, never technical or
	// hierarchy authority. Zero is a valid coordinate (for example specials
	// season 0), so these fields cannot use zero as a "missing" sentinel.
	update.DurationSeconds = nil
	if strings.TrimSpace(state.Summary) != "" {
		update.Summary = nil
	}
	if strings.TrimSpace(state.Tagline) != "" {
		update.Tagline = nil
	}
	if strings.TrimSpace(state.ContentRating) != "" {
		update.ContentRating = nil
	}
	if state.CommunityRating > 0 {
		update.CommunityRating = nil
	}
	if state.CriticRating > 0 {
		update.CriticRating = nil
	}
	if strings.TrimSpace(state.Studio) != "" {
		update.Studio = nil
	}
	if strings.TrimSpace(state.Network) != "" {
		update.Network = nil
	}
	if strings.TrimSpace(state.Country) != "" {
		update.Country = nil
	}
	if len(state.Genres) > 0 {
		update.Genres = nil
	}
	if len(state.Tags) > 0 {
		update.Tags = nil
	}
	if len(state.Labels) > 0 {
		update.Labels = nil
	}
	update.SeasonNumber = nil
	update.EpisodeNumber = nil
	update.IndexNumber = nil
	if strings.TrimSpace(state.ArtSeed) != "" {
		update.ArtSeed = nil
	}
	if update.Artwork != nil {
		missing := map[string]string{}
		for key, value := range *update.Artwork {
			if strings.TrimSpace(state.Artwork[key]) == "" && strings.TrimSpace(value) != "" {
				missing[key] = value
			}
		}
		if len(missing) == 0 {
			update.Artwork = nil
		} else {
			update.Artwork = &missing
		}
	}
	if update.TypedMetadata != nil {
		missing := map[string]string{}
		for key, value := range *update.TypedMetadata {
			if strings.TrimSpace(state.TypedMetadata[key]) == "" && strings.TrimSpace(value) != "" {
				missing[key] = value
			}
		}
		if len(missing) == 0 {
			update.TypedMetadata = nil
		} else {
			update.TypedMetadata = &missing
		}
	}
	if update.People != nil {
		var present int
		if err := tx.QueryRow(`SELECT 1 FROM media_people WHERE media_id=? LIMIT 1`, state.ID).Scan(&present); err == nil {
			update.People = nil
		} else if err != sql.ErrNoRows {
			return err
		}
	}
	return nil
}

func (s *Server) prepareMetadataUpdateForIntent(ctx context.Context, mediaID string, update *UpdateMediaRequest, intent metadataRefreshIntent) error {
	if update == nil || intent != metadataRefreshFillMissing {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	state, err := loadMetadataCanonicalStateTx(tx, mediaID)
	if err != nil {
		return err
	}
	return filterMetadataUpdateForIntent(tx, state, update, intent)
}

func metadataNeedsSupplement(state MediaItem) bool {
	return strings.TrimSpace(state.Summary) == "" ||
		strings.TrimSpace(state.ContentRating) == "" ||
		len(state.Genres) == 0 || len(state.Artwork) == 0 || len(state.People) == 0
}

func (s *Server) establishedFallbackIdentity(item MediaItem, provider string) bool {
	externalType := ""
	switch normalizedMetadataProvider(provider) {
	case "tvdb":
		externalType = tvdbSearchType(item.Type)
	case "tmdb":
		externalType = tmdbSearchType(item.Type)
	case "musicbrainz":
		externalType = manualMusicBrainzExternalType(item.Type)
	}
	if externalType == "" {
		return false
	}
	_, ok := s.mediaProviderID(item.ID, provider, externalType)
	return ok
}
