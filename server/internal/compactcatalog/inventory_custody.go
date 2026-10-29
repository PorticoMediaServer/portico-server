package compactcatalog

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/dbwork"
)

// Health is source state, not changed file evidence. Item operations pause
// before checking their captured digest and resume with exactly the same root.
var ErrMutationSourcePaused = errors.New("catalog item operation is waiting for its source")

const InventoryCustodySQL = `SELECT EXISTS(SELECT 1 FROM catalog_entity_mutations
 INDEXED BY catalog_entity_mutations_inventory_custody
 WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?1)
 AND status<>'terminal' LIMIT 1)`

func LibraryInventoryHeldTx(ctx context.Context, tx *sql.Tx, library string) (held bool, err error) {
	err = tx.QueryRowContext(ctx, InventoryCustodySQL, library).Scan(&held)
	return
}
func RequireLibraryInventoryFreeTx(ctx context.Context, tx *sql.Tx, library string) error {
	held, err := LibraryInventoryHeldTx(ctx, tx, library)
	if err != nil {
		return err
	}
	if held {
		return ErrEntityMutating
	}
	return nil
}

func admitInventoryFairMutationTx(ctx context.Context, tx *sql.Tx, library int64) error {
	var public string
	var pending bool
	err := tx.QueryRowContext(ctx, `SELECT library_id,EXISTS(SELECT 1 FROM inventory_strict_intents INDEXED BY inventory_strict_intents_library WHERE library_id=c.library_id LIMIT 1) FROM catalog_libraries c WHERE id=?`, library).Scan(&public, &pending)
	if err != nil {
		return err
	}
	allowed, err := dbwork.CatalogItemAdmission(tx, public, pending)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrEntityMutating
	}
	return nil
}

func mutationSourceAvailableTx(ctx context.Context, tx *sql.Tx, library int64) error {
	var paused bool
	// Source admission already caps one library at64 folders. No item updates,
	// asset walk or media read is performed when a drive becomes unavailable.
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM library_sources
 WHERE library_id=(SELECT library_id FROM catalog_libraries WHERE id=?1)
 AND health IN('offline','root_changed') LIMIT 1)`, library).Scan(&paused)
	if err != nil {
		return err
	}
	if paused {
		return ErrMutationSourcePaused
	}
	return nil
}
