package catalog

import (
	"context"
	"database/sql"

	"portico.local/server/internal/compactcatalog"
)

// Album sidecars use the same automatic local: field source as embedded track
// covers. The normal artwork resolver gives an owner selection precedence.
func musicAlbumBackdropTx(ctx context.Context, tx *sql.Tx, album int64, extra map[string]string) error {
	if key := extra["album/backdrop"]; key != "" {
		return compactcatalog.SetFieldsTx(ctx, tx, album, compactcatalog.Automatic, map[string]any{"backdrop_url": "local:" + key})
	}
	return nil
}
