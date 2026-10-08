package catalog

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/entityid"
)

// PurgeItemTx deletes an item by its public id; see purgeEntityTx. An unknown
// id has nothing to purge.
func PurgeItemTx(ctx context.Context, tx *sql.Tx, item string) error {
	id, err := entityid.Resolve(ctx, tx, item)
	if errors.Is(err, entityid.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return purgeEntityTx(ctx, tx, id)
}

// purgeEntityTx releases item-owned rows first, then asset-only evidence only
// when no surviving item owns the asset. Restrictive references are explicit; a
// future schema edge fails the transaction while bytes are still recoverable.
func purgeEntityTx(ctx context.Context, tx *sql.Tx, item int64) error {
	type file struct {
		id    int64
		token string
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.token FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id WHERE l.entity_id=?`, item)
	if err != nil {
		return err
	}
	files := []file{}
	for rows.Next() {
		var f file
		if err = rows.Scan(&f.id, &f.token); err != nil {
			rows.Close()
			return err
		}
		files = append(files, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, query := range []string{
		`DELETE FROM playback_sessions WHERE item_id=?`,
		`DELETE FROM lyric_selections WHERE resource_id IN(SELECT id FROM lyric_resources WHERE item_id=?)`,
	} {
		if _, err = tx.ExecContext(ctx, query, item); err != nil {
			return err
		}
	}
	if err = compactcatalog.DeleteEntityTx(ctx, tx, item); err != nil {
		return err
	}
	for _, f := range files {
		var shared bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_asset_links INDEXED BY catalog_asset_links_asset WHERE asset_id=?1) OR EXISTS(SELECT 1 FROM item_extras WHERE asset_id=?2) OR EXISTS(SELECT 1 FROM playback_sessions WHERE asset_id=?2) OR EXISTS(SELECT 1 FROM lyric_resources WHERE asset_id=?2) OR EXISTS(SELECT 1 FROM lyric_candidates WHERE asset_id=?2) OR EXISTS(SELECT 1 FROM subtitle_resources WHERE source_id=?2) OR EXISTS(SELECT 1 FROM dvr_catalog_provenance WHERE asset_id=?2)`, f.id, f.token).Scan(&shared); err != nil {
			return err
		}
		if shared {
			continue
		}
		for _, query := range []string{
			`DELETE FROM inventory_analysis_queue WHERE object_id IN(SELECT id FROM inventory_objects WHERE asset_id=?)`,
			`DELETE FROM inventory_objects WHERE asset_id=?`,
			`DELETE FROM audio_tag_evidence WHERE asset_id=?`,
			`DELETE FROM audio_source_metadata WHERE asset_id=?`,
			`DELETE FROM episodic_sources WHERE asset_id=?`,
		} {
			if _, err = tx.ExecContext(ctx, query, f.token); err != nil {
				return err
			}
		}
		if err = compactcatalog.DeleteAssetTx(ctx, tx, f.id); err != nil {
			return err
		}
	}
	return nil
}
