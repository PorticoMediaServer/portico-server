package catalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"

	"portico.local/server/internal/compactcatalog"
)

// readingFormat is deliberately separate from audio Book/Part and timed video.
// An extension admits inventory only; the rooted reading runtime validates the
// actual package/PDF/image bytes before exposing a readable representation.
func readingFormat(path, kind string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if kind == "book" && (ext == ".epub" || ext == ".pdf") {
		return ext[1:]
	}
	if kind == "comic" && (ext == ".cbz" || ext == ".pdf") {
		return ext[1:]
	}
	return ""
}

func attachReadingTx(ctx context.Context, tx *sql.Tx, library, libraryKind, asset, path string) (int64, error) {
	format := readingFormat(path, libraryKind)
	if format == "" {
		return 0, ErrAdminQuery
	}
	kindName := "ebook"
	if libraryKind == "comic" {
		kindName = "comic"
	}
	// The steward owns kind registration; absent canonical registration fails
	// closed rather than accidentally attaching the publication as a movie.
	kind, err := compactcatalog.ParseKind(kindName)
	if err != nil {
		return 0, err
	}
	handle, err := compactcatalog.LibraryTx(ctx, tx, library)
	if err != nil {
		return 0, err
	}
	assetID, err := compactcatalog.AssetByTokenTx(ctx, tx, asset)
	if err != nil {
		return 0, err
	}
	root, err := compactcatalog.LibraryRootTx(ctx, tx, handle)
	if err != nil {
		return 0, err
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var item int64
	err = tx.QueryRowContext(ctx, `SELECT p.item_id FROM reading_publications p JOIN catalog_entities e ON e.id=p.item_id WHERE p.asset_id=? AND e.library_id=? AND e.kind=?`, assetID, handle, int(kind)).Scan(&item)
	created := false
	if err == sql.ErrNoRows {
		item, created, err = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: kind, Key: "reading:" + kindName + ":" + compactcatalog.ItemKey(root, path, 0), Title: name, Added: catalogedNow()})
	}
	if err != nil {
		return 0, err
	}
	if err = compactcatalog.LinkAssetTx(ctx, tx, item, assetID, compactcatalog.Link{}); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO reading_publications(item_id,asset_id,format) VALUES(?,?,?) ON CONFLICT(item_id,asset_id) DO UPDATE SET format=excluded.format`, item, assetID, format); err != nil {
		return 0, err
	}
	if created {
		err = restoreRetainedTx(ctx, tx, item)
	}
	return item, err
}
