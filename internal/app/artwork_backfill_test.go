package app

import (
	"context"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestArtworkRenditionBackfillUsesStablePersonClass(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	var mediaID string
	if err := db.QueryRow(`SELECT id FROM media_items WHERE type = 'movie' ORDER BY id LIMIT 1`).Scan(&mediaID); err != nil {
		t.Fatal(err)
	}
	portraitPath := writeArtworkRenditionTestPNG(t, server.cfg.AppDataDir, 600, 900, false)
	if _, err := db.Exec(`INSERT INTO media_people (id, media_id, name, role, source, sort_order, image_url, provider_ids_json, created_at)
		VALUES ('backfill-person', ?, 'Backfill Person', 'Actor', 'tmdb', 0, ?, '{}', '2026-01-01T00:00:00Z')`, mediaID, portraitPath); err != nil {
		t.Fatal(err)
	}
	result, err := BackfillArtworkRenditions(context.Background(), server.cfg, db)
	if err != nil || result.Prepared < 1 {
		t.Fatalf("backfill=%#v err=%v", result, err)
	}
	if _, ok := server.preparedArtworkRenditionPath(portraitPath, "person", artworkRenditionSmall); !ok {
		t.Fatal("person small rendition was not prepared")
	}
}

func TestArtworkRenditionBackfillSkipsProviderEvidenceRecords(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	var mediaID string
	if err := db.QueryRow(`SELECT id FROM media_items WHERE type = 'movie' ORDER BY id LIMIT 1`).Scan(&mediaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO media_images (id, media_id, image_type, path, source, created_at)
		VALUES ('backfill-provider-evidence', ?, 'poster', 'provider-evidence:deadbeef', 'tmdb', '2026-01-01T00:00:00Z')`, mediaID); err != nil {
		t.Fatal(err)
	}
	result, err := BackfillArtworkRenditions(context.Background(), server.cfg, db)
	if err != nil {
		t.Fatalf("backfill=%#v err=%v", result, err)
	}
	if result.Skipped < 1 {
		t.Fatalf("backfill=%#v, want at least one skipped provenance record", result)
	}
}
