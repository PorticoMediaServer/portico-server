package httpapi

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/entityid"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/metadata"
)

// The new nonplayable Reading targets use the current library/family boundary,
// never LibraryForItem's playable-only lookup or an archive-access grant.
func (d Dependencies) readingMetadataTargetAuthorize(ctx context.Context, tx *sql.Tx, p identity.Principal, t metadata.RepairTarget) error {
	if t.Kind != "item" {
		return nil
	}
	entity, err := entityid.Resolve(ctx, tx, t.ID)
	if errors.Is(err, entityid.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var library string
	err = tx.QueryRowContext(ctx, `SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.id=? AND e.kind IN(20,21) AND e.retired=0 AND cl.retired=0`, entity).Scan(&library)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return d.resourceAuthorize(tx, p, library)
}
