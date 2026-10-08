package catalog

import (
	"context"
	"database/sql"
)

// Called only after TrashInventory's fresh absence proof, source/scan fences and
// authorization, in the same transaction as retiring the exact object. Missing
// and trashed alternatives are still recoverable and retain logical identity.
func purgeForgottenInventoryItemsTx(ctx context.Context, tx *sql.Tx, library, asset string) error {
	ids, err := scanInt64s(tx.QueryContext(ctx, `SELECT i.id FROM catalog_assets target_asset JOIN catalog_asset_links target INDEXED BY catalog_asset_links_asset ON target.asset_id=target_asset.id
 JOIN catalog_entities i ON i.id=target.entity_id JOIN catalog_libraries cl ON cl.id=i.library_id
 WHERE cl.library_id=?1 AND target_asset.token=?2 AND NOT EXISTS(SELECT 1 FROM admin_trash t WHERE t.item_id=i.id AND t.state='held') AND NOT EXISTS(
 SELECT 1 FROM catalog_asset_links alt JOIN catalog_assets a ON a.id=alt.asset_id WHERE alt.entity_id=i.id AND (
 NOT EXISTS(SELECT 1 FROM inventory_objects o JOIN library_sources s ON s.id=o.source_id WHERE o.asset_id=a.token AND s.library_id=?1)
 OR EXISTS(SELECT 1 FROM inventory_objects o JOIN library_sources s ON s.id=o.source_id WHERE o.asset_id=a.token AND s.library_id=?1 AND o.retired=0)))`, library, asset))
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = purgeEntityTx(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}
