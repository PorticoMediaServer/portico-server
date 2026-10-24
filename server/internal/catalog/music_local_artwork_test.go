package catalog

import (
	"context"
	"database/sql"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
)

func TestMusicAlbumBackgroundUsesNormalAutomaticSourceAndPreservesOwner(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("music", "Music", "music", "/music")
	album := c.Album(c.Artist(library, "Pink Floyd"), "The Wall", 1979)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := musicAlbumBackdropTx(ctx, tx, album.ID, map[string]string{"album/backdrop": "album-background"}); err != nil {
			return err
		}
		var value string
		if err := tx.QueryRowContext(ctx, `SELECT backdrop_url FROM catalog_item_details WHERE entity_id=?`, album.ID).Scan(&value); err != nil {
			return err
		}
		if value != "local:album-background" {
			t.Fatal(value)
		}
		if err := compactcatalog.SetFieldsTx(ctx, tx, album.ID, compactcatalog.Owner, map[string]any{"backdrop_url": "local:owner-background"}); err != nil {
			return err
		}
		if err := musicAlbumBackdropTx(ctx, tx, album.ID, map[string]string{"album/backdrop": "artist-fallback"}); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT backdrop_url FROM catalog_item_details WHERE entity_id=?`, album.ID).Scan(&value); err != nil {
			return err
		}
		if value != "local:owner-background" {
			t.Fatal("scan displaced owner background", value)
		}
		return nil
	})
}
