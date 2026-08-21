package app

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func upsertMediaRatingEvidenceTx(tx *sql.Tx, mediaID, provider, source, country, rawRating, now string) error {
	mediaID = strings.TrimSpace(mediaID)
	rawRating = normalizeContentRating(rawRating)
	if mediaID == "" || rawRating == "" {
		return nil
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "local"
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = provider
	}
	country, ratingSystem := ratingSystemFor(rawRating, country)
	_, err := tx.Exec(`
		INSERT INTO media_rating_evidence (
			media_id, provider, source, country, rating_system, raw_rating, normalized_rating, normalized_rank, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_id, provider, source) DO UPDATE SET
			country = excluded.country,
			rating_system = excluded.rating_system,
			raw_rating = excluded.raw_rating,
			normalized_rating = excluded.normalized_rating,
			normalized_rank = excluded.normalized_rank,
			updated_at = excluded.updated_at`,
		mediaID, provider, source, country, ratingSystem, rawRating, rawRating, contentRatingRank(rawRating), now)
	return err
}

func (s *Server) upsertMediaRatingEvidence(mediaID, provider, source, country, rawRating string) error {
	return s.withBackgroundTxTagged(context.Background(), []string{"media", "metadata", "library-items"}, func(tx *sql.Tx) error {
		return upsertMediaRatingEvidenceTx(tx, mediaID, provider, source, country, rawRating, time.Now().UTC().Format(time.RFC3339))
	})
}

func ratingSystemFor(rawRating, country string) (string, string) {
	rawRating = normalizeContentRating(rawRating)
	country = strings.ToUpper(strings.TrimSpace(country))
	country = strings.ReplaceAll(country, "UNITED STATES", "US")
	country = strings.ReplaceAll(country, "UNITED KINGDOM", "GB")
	if strings.HasPrefix(rawRating, "TV-") {
		return firstNonEmpty(country, "US"), "US-TV"
	}
	switch rawRating {
	case "G", "PG", "PG-13", "R", "NC-17":
		return firstNonEmpty(country, "US"), "MPA"
	case "14A", "18A", "A", "C", "C8":
		return firstNonEmpty(country, "CA"), "CHVRS"
	case "U", "12", "12A", "15", "18":
		return firstNonEmpty(country, "GB"), "BBFC"
	default:
		return country, ""
	}
}
