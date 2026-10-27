package compactcatalog_test

import (
	"context"
	"database/sql"
	"testing"

	"portico.local/server/internal/compactcatalog"
)

func TestReadingIdempotentAssetAndLinkWritesKeepInputGeneration(t *testing.T) {
	c, library, items := readingProjectionFixture(t, 1)
	item := items[0]
	var asset int64
	var path, token string
	var size, modified int64
	if err := c.DB.QueryRow(`SELECT a.id,a.token,a.path,a.size,a.modified_ns FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id WHERE l.entity_id=?`, item.ID).Scan(&asset, &token, &path, &size, &modified); err != nil {
		t.Fatal(err)
	}
	generation := func() int64 {
		t.Helper()
		var g int64
		if err := c.DB.QueryRow(`SELECT input_generation FROM reading_attribution_state WHERE library_id=?`, library).Scan(&g); err != nil {
			t.Fatal(err)
		}
		return g
	}
	for _, tc := range []struct {
		name  string
		write func(context.Context, *sql.Tx) error
	}{
		{"asset", func(ctx context.Context, tx *sql.Tx) error {
			id, got, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: path, Size: size, ModifiedNS: modified, Container: "pdf"})
			if err == nil && (id != asset || got != token) {
				t.Fatalf("asset identity changed: %d/%s", id, got)
			}
			return err
		}},
		{"link", func(ctx context.Context, tx *sql.Tx) error {
			return compactcatalog.LinkAssetTx(ctx, tx, item.ID, asset, compactcatalog.Link{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := generation()
			c.Write(tc.write)
			if after := generation(); after != before {
				t.Fatalf("unchanged %s invalidated Reading: %d -> %d", tc.name, before, after)
			}
		})
	}
	before := generation()
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetAssetAvailableTx(ctx, tx, asset, false)
	})
	if generation() <= before {
		t.Fatal("real availability loss did not invalidate")
	}
	before = generation()
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		_, _, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: path, Size: size, ModifiedNS: modified, Container: "pdf"})
		return err
	})
	if generation() <= before {
		t.Fatal("asset reappearance did not invalidate")
	}
	before = generation()
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.LinkAssetTx(ctx, tx, item.ID, asset, compactcatalog.Link{Edition: "New edition"})
	})
	if generation() <= before {
		t.Fatal("real link change did not invalidate")
	}
}
