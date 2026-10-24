package catalog

import (
	"context"
	"database/sql"

	"portico.local/server/internal/compactcatalog"
)

// Album sidecars use the same automatic local: field source as embedded track
// covers. Owner field/selected-art overrides keep their existing precedence.
func musicAlbumBackdropTx(ctx context.Context, tx *sql.Tx, album int64, extra map[string]string) error {
	if key := extra["album/backdrop"]; key != "" {
		return compactcatalog.SetFieldsTx(ctx, tx, album, compactcatalog.Automatic, map[string]any{"backdrop_url": "local:" + key})
	}
	return nil
}
