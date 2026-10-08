package catalog

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/server/internal/storage"
)

// CleanupCatalogTrash is an explicitly scheduled/owner-run bounded catalog
// operation. It does not forget identities or remove any source media bytes.
// The callback rechecks the maintenance command and records progress in the same
// transaction as each successful domain mutation.
func (s *Service) CleanupCatalogTrash(ctx context.Context, library string, authorize func(*sql.Tx) error) error {
	rows, err := s.read().QueryContext(ctx, `SELECT o.id,o.revision FROM library_sources s CROSS JOIN inventory_objects o INDEXED BY inventory_objects_missing ON o.source_id=s.id WHERE s.library_id=? AND s.enabled=1 AND o.retired=0 AND o.state='missing' AND o.root_incarnation=s.incarnation AND o.missing_observations>=2 AND s.last_complete_at>=? AND NOT EXISTS(SELECT 1 FROM inventory_source_active a WHERE a.source_id=s.id) ORDER BY o.id LIMIT 64`, library, time.Now().UTC().Add(-10*time.Minute).Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	type target struct{ id, revision string }
	batch := []target{}
	for rows.Next() {
		var v target
		if err = rows.Scan(&v.id, &v.revision); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range batch {
		if err = ctx.Err(); err != nil {
			return err
		}
		err = s.TrashInventory(ctx, library, v.id, v.revision, "trash", authorize)
		// A source concurrently used or changed needs a fresh domain review. No
		// missing/stale proof is converted into deletion permission by maintenance.
		if errors.Is(err, ErrSourceBusy) || errors.Is(err, ErrStaleContinuation) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, storage.ErrBusy) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}
