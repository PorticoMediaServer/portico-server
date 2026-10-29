package catalog

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

// ErrInventoryItemOperation is a retryable publication refusal. The scan keeps
// its durable cursor; the existing source-wide intent records the owed rerun.
var ErrInventoryItemOperation = errors.New("inventory publication waits for an admitted item operation")

// DeferInventorySourceTx is a bounded library-prefix check, not an asset→all
// titles scan. The caller commits this mark even when it defers its own page.
func DeferInventorySourceTx(ctx context.Context, tx *sql.Tx, source string) (bool, error) {
	var library string
	if err := tx.QueryRowContext(ctx, `SELECT library_id FROM library_sources WHERE id=?`, source).Scan(&library); err != nil {
		return false, err
	}
	held, err := compactcatalog.LibraryInventoryHeldTx(ctx, tx, library)
	if err != nil || !held {
		return false, err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_strict_intents WHERE source_id=? AND relative_path='.')`, source).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		// Preserve the existing4096-intent store cap. A full store still keeps this
		// scan's unchanged durable page/cursor; it is never acknowledged as applied.
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM inventory_strict_intents`).Scan(&count); err != nil {
			return false, err
		}
		if count < 4096 {
			_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_strict_intents(source_id,relative_path,library_id,source_generation,root_incarnation,root_identity,root,configured_root,source_kind,follow_symlinks,mode,created_ms)
 SELECT id,'.',library_id,generation,incarnation,root_identity,root,configured_root,kind,follow_symlinks,'incremental',CAST(strftime('%s','now') AS INTEGER)*1000 FROM library_sources WHERE id=?`, source)
			if err != nil {
				return false, err
			}
		}
	}
	if err = dbwork.CatalogItemIngestDeferred(tx, library); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) DeferInventorySource(ctx context.Context, source string) (bool, error) {
	return DeferInventorySource(ctx, s.db, source)
}

// DeferInventorySource also serves the independent verification worker, which
// needs only the database and has no catalogue read-service dependency.
func DeferInventorySource(ctx context.Context, db *sql.DB, source string) (deferred bool, err error) {
	err = dbwork.WithWriteTx(ctx, db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		var e error
		deferred, e = DeferInventorySourceTx(ctx, tx, source)
		return e
	})
	return
}

func inventoryPublicationProgressTx(tx *sql.Tx, source LibrarySource) error {
	return dbwork.CatalogItemIngestProgress(tx, source.LibraryID)
}

func requireInventorySourceFreeTx(ctx context.Context, tx *sql.Tx, source string) error {
	var held bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM library_sources s WHERE s.id=?1 AND EXISTS(SELECT 1 FROM catalog_entity_mutations m INDEXED BY catalog_entity_mutations_inventory_custody WHERE m.library_id=(SELECT id FROM catalog_libraries WHERE library_id=s.library_id) AND m.status<>'terminal' LIMIT 1))`, source).Scan(&held)
	if err != nil {
		return err
	}
	if held {
		return ErrInventoryItemOperation
	}
	return nil
}
