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
 release_key=COALESCE((SELECT strftime('%Y-%m-%dT%H:%M:%fZ',release_date) FROM catalog_item_details WHERE entity_id=e.entity_id),''),
 latest_aired_key=COALESCE((SELECT air_key FROM catalog_browse_member_metrics WHERE entity_id=e.entity_id ORDER BY air_key DESC LIMIT 1),''),
 critic_key=COALESCE((SELECT critic FROM catalog_browse_member_metrics WHERE entity_id=e.entity_id ORDER BY critic DESC LIMIT 1),0),
 episode_count=COALESCE((SELECT total FROM catalog_browse_episode_counts WHERE entity_id=e.entity_id),0),
 album_key=COALESCE((SELECT lower(a.sort_key) FROM catalog_songs s JOIN catalog_entities a ON a.id=s.album_id WHERE s.entity_id=e.entity_id),''),
 artist_key=COALESCE((SELECT lower(ar.sort_key) FROM catalog_songs s JOIN catalog_albums a ON a.entity_id=s.album_id JOIN catalog_entities ar ON ar.id=a.artist_id WHERE s.entity_id=e.entity_id),(SELECT lower(ar.sort_key) FROM catalog_albums a JOIN catalog_entities ar ON ar.id=a.artist_id WHERE a.entity_id=e.entity_id),''),
 track_key=COALESCE((SELECT COALESCE(disc_number,1)*1000000+COALESCE(track_number,0) FROM catalog_songs WHERE entity_id=e.entity_id),0),
 file_size=COALESCE((SELECT file_size FROM catalog_browse_member_metrics WHERE entity_id=e.entity_id ORDER BY file_size DESC LIMIT 1),0),
 resolution_key=COALESCE((SELECT resolution FROM catalog_browse_member_metrics WHERE entity_id=e.entity_id ORDER BY resolution DESC LIMIT 1),0),
 random_key=? WHERE entity_id=?`, int64(hash.Sum32()), entity)
	return err
}

// An album or artist rename changes its members' sort keys. The existing
// durable fanout cursor bounds that work, including artists with many albums.
const DomainBrowseSortParents = 34

func deriveBrowseSortParent(ctx context.Context, tx *sql.Tx, k Key, limit int) (bool, error) {
	var after, revision int64
	err := tx.QueryRowContext(ctx, `SELECT after,revision FROM catalog_fanouts WHERE domain=? AND key=?`, DomainBrowseSortParents, k.ID).Scan(&after, &revision)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if revision != k.Revision {
		after = 0
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_fanouts(domain,key,revision,after) VALUES(?,?,?,?) ON CONFLICT(domain,key) DO UPDATE SET revision=excluded.revision,after=excluded.after`, DomainBrowseSortParents, k.ID, k.Revision, after); err != nil {
		return false, err
	}
	ids, err := scanIDs(ctx, tx, `WITH albums AS(SELECT entity_id AS id FROM catalog_albums WHERE artist_id=?1 AND entity_id>?2 ORDER BY entity_id LIMIT ?3),members AS(SELECT item_id AS id FROM catalog_browse_memberships WHERE entity_id=?1 AND item_id>?2 GROUP BY item_id ORDER BY item_id LIMIT ?3) SELECT id FROM albums UNION SELECT id FROM members ORDER BY id LIMIT ?3`, k.ID, after, limit)
	if err != nil {
		return false, err
	}
	if err = refreshBrowseSortFacts(ctx, tx, k.ID); err != nil {
		return false, err
	}
	for _, id := range ids {
		if err = refreshBrowseSortFacts(ctx, tx, id); err != nil {
			return false, err
		}
	}
	if len(ids) < limit {
		_, err = tx.ExecContext(ctx, `DELETE FROM catalog_fanouts WHERE domain=? AND key=?`, DomainBrowseSortParents, k.ID)
		return true, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE catalog_fanouts SET after=? WHERE domain=? AND key=?`, ids[len(ids)-1], DomainBrowseSortParents, k.ID)
	return false, err
}

func init() {
	Register(DomainBrowseSortParents, Derivation{Version: 1, Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
		done := []int64{}
		for _, k := range keys {
			finished, err := deriveBrowseSortParent(ctx, tx, k, limit)
			if err != nil {
				return nil, err
			}
			if finished {
				done = append(done, k.ID)
			}
		}
		return done, nil
	}, Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
		return scanIDs(ctx, tx, `SELECT id FROM catalog_entities WHERE id>? AND kind IN(5,6) ORDER BY id LIMIT ?`, after, limit)
	}})
}
