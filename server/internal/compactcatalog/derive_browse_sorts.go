package compactcatalog

import (
	"context"
	"database/sql"
	"hash/fnv"
)

// Sort facts are published with the browse row, and refreshed by the bounded
// item-to-container metric fanout when a member's facts change.
func refreshBrowseSortFacts(ctx context.Context, tx *sql.Tx, entity int64) error {
	var public []byte
	if err := tx.QueryRowContext(ctx, `SELECT public_id FROM catalog_entities WHERE id=?`, entity).Scan(&public); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	hash := fnv.New32a()
	_, _ = hash.Write(public)
	_, err := tx.ExecContext(ctx, `UPDATE catalog_browse_rows AS e SET
 release_key=COALESCE((SELECT release_date FROM catalog_item_details WHERE entity_id=e.entity_id),''),
 latest_aired_key=COALESCE((SELECT max(d.release_date) FROM catalog_browse_memberships m JOIN catalog_item_details d ON d.entity_id=m.item_id WHERE m.entity_id=e.entity_id),''),
 critic_key=COALESCE((SELECT max(r.value) FROM catalog_browse_memberships m JOIN metadata_ratings r ON r.item_id=m.item_id AND r.provider IN('metacritic','rottentomatoes') WHERE m.entity_id=e.entity_id),0),
 episode_count=(SELECT count(*) FROM catalog_episodes WHERE show_id=e.entity_id OR season_id=e.entity_id),
 album_key=COALESCE((SELECT lower(a.sort_key) FROM catalog_songs s JOIN catalog_entities a ON a.id=s.album_id WHERE s.entity_id=e.entity_id),''),
 artist_key=COALESCE((SELECT lower(ar.sort_key) FROM catalog_songs s JOIN catalog_albums a ON a.entity_id=s.album_id JOIN catalog_entities ar ON ar.id=a.artist_id WHERE s.entity_id=e.entity_id),(SELECT lower(ar.sort_key) FROM catalog_albums a JOIN catalog_entities ar ON ar.id=a.artist_id WHERE a.entity_id=e.entity_id),''),
 track_key=COALESCE((SELECT COALESCE(disc_number,1)*1000000+COALESCE(track_number,0) FROM catalog_songs WHERE entity_id=e.entity_id),0),
 file_size=COALESCE((SELECT max(a.size) FROM catalog_browse_memberships m JOIN catalog_asset_links l ON l.entity_id=m.item_id JOIN catalog_assets a ON a.id=l.asset_id WHERE m.entity_id=e.entity_id),0),
 resolution_key=COALESCE((SELECT max(a.height) FROM catalog_browse_memberships m JOIN catalog_asset_links l ON l.entity_id=m.item_id JOIN catalog_assets a ON a.id=l.asset_id WHERE m.entity_id=e.entity_id),0),
 random_key=? WHERE entity_id=?`, int64(hash.Sum32()), entity)
	return err
}
