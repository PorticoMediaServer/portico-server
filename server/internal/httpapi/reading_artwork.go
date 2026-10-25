package httpapi

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/reading"
)

// Artwork alone admits nonplayable Reading items through the current Reading
// library/profile/family predicate. This never grants playback or changes itemAccess.
func (d Dependencies) itemArtworkAccess(ctx context.Context, p identity.Principal, item string) error {
	err := d.itemAccess(ctx, p, item)
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = d.readingService().Versions(ctx, p, item)
	if errors.Is(err, reading.ErrUnavailable) {
		return sql.ErrNoRows
	}
	return err
}
