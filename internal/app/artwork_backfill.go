package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

type ArtworkRenditionBackfillResult struct {
	Prepared int `json:"prepared"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
}

// BackfillArtworkRenditions is an explicit pre-release migration for catalogs
// created before rendition classes were stable. Normal ingestion remains the
// only production writer for newly discovered artwork.
func BackfillArtworkRenditions(ctx context.Context, cfg config.Config, db *sql.DB) (ArtworkRenditionBackfillResult, error) {
	if db == nil {
		return ArtworkRenditionBackfillResult{}, errors.New("artwork rendition backfill requires a database")
	}
	rows, err := db.QueryContext(ctx, `
		SELECT COALESCE(image_type, ''), COALESCE(path, '') FROM media_images WHERE trim(COALESCE(path, '')) <> ''
		UNION ALL
		SELECT 'person', COALESCE(image_url, '') FROM media_people WHERE trim(COALESCE(image_url, '')) <> ''`)
	if err != nil {
		return ArtworkRenditionBackfillResult{}, err
	}
	defer rows.Close()
	server := &Server{cfg: cfg}
	seen := make(map[string]struct{})
	result := ArtworkRenditionBackfillResult{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var kind, path string
		if err := rows.Scan(&kind, &path); err != nil {
			return result, err
		}
		kind, path = artworkRenditionKind(kind), strings.TrimSpace(path)
		key := kind + "\x00" + path
		if path == "" {
			result.Skipped++
			continue
		}
		if _, ok := seen[key]; ok {
			result.Skipped++
			continue
		}
		seen[key] = struct{}{}
		if err := server.prepareArtworkRenditions(path, kind); err != nil {
			result.Failed++
			continue
		}
		result.Prepared++
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("%d artwork source(s) could not be prepared", result.Failed)
	}
	return result, nil
}
