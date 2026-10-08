package compactcatalog

import (
	"context"
	"database/sql"
	"errors"
)

// Availability (domain 20): whether an item can be played now, and which of
// its files can. It follows inventory evidence, source health, trash and DVR
// retirement through the inventory_item_availability view, and is queued by
// the write API (links, asset evidence) and the feed triggers (inventory
// objects, sources, trash, retirements). A source's health change queues its
// library (domain 28), whose items are fanned out in bounded steps.

const (
	DomainLibraryAvailability = 28
	DomainAssetIssues         = 29
)

func deriveAvailability(ctx context.Context, tx *sql.Tx, ids string) error {
	for _, q := range []string{
		// Only a changed flag is written: an unchanged row is not a link change.
		`UPDATE catalog_asset_links SET available=COALESCE((SELECT av.available FROM inventory_item_availability av JOIN catalog_assets a ON a.token=av.asset_id WHERE av.item_id=catalog_asset_links.entity_id AND a.id=catalog_asset_links.asset_id),0)
 WHERE entity_id IN(SELECT value FROM json_each(?1))
 AND available IS NOT COALESCE((SELECT av.available FROM inventory_item_availability av JOIN catalog_assets a ON a.token=av.asset_id WHERE av.item_id=catalog_asset_links.entity_id AND a.id=catalog_asset_links.asset_id),0)`,
		`DELETE FROM catalog_item_availability WHERE entity_id IN(SELECT value FROM json_each(?1)) AND NOT EXISTS(SELECT 1 FROM catalog_entities e WHERE e.id=catalog_item_availability.entity_id)`,
		`INSERT INTO catalog_item_availability(entity_id,available,retired)
 SELECT e.id,EXISTS(SELECT 1 FROM catalog_asset_links l WHERE l.entity_id=e.id AND l.available=1),EXISTS(SELECT 1 FROM dvr_catalog_retirements g WHERE g.item_id=e.id)
 FROM json_each(?1) j JOIN catalog_entities e ON e.id=j.value
 ON CONFLICT(entity_id) DO UPDATE SET available=excluded.available,retired=excluded.retired
 WHERE catalog_item_availability.available IS NOT excluded.available OR catalog_item_availability.retired IS NOT excluded.retired`,
	} {
		if _, err := tx.ExecContext(ctx, q, ids); err != nil {
			return err
		}
	}
	// A changed availability shows in the item's browse row and Home buckets.
	_, err := tx.ExecContext(ctx, `INSERT INTO catalog_dirty(domain,entity_id,revision) SELECT ?,value,1 FROM json_each(?) WHERE 1
 ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1`, DomainBrowseRows, ids)
	return err
}

// deriveLibraryAvailability queues the availability of limit more items of a
// library; true when the library is done.
func deriveLibraryAvailability(ctx context.Context, tx *sql.Tx, k Key, limit int) (bool, error) {
	var after, revision int64
	err := tx.QueryRowContext(ctx, `SELECT after,revision FROM catalog_fanouts WHERE domain=? AND key=?`, DomainLibraryAvailability, k.ID).Scan(&after, &revision)
	if errors.Is(err, sql.ErrNoRows) || err == nil && revision != k.Revision {
		after = 0
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_fanouts(domain,key,revision,after) VALUES(?,?,?,0) ON CONFLICT(domain,key) DO UPDATE SET revision=excluded.revision,after=0`, DomainLibraryAvailability, k.ID, k.Revision); err != nil {
			return false, err
		}
	} else if err != nil {
		return false, err
	}
	ids, err := scanIDs(ctx, tx, `SELECT e.id FROM catalog_entities e INDEXED BY catalog_entities_library JOIN catalog_kinds k ON k.id=e.kind AND k.playable=1
 WHERE e.library_id=? AND e.id>? ORDER BY e.id LIMIT ?`, k.ID, after, limit)
	if err != nil {
		return false, err
	}
	if err = TouchTx(ctx, tx, DomainAvailability, ids...); err != nil {
		return false, err
	}
	if len(ids) < limit {
		_, err = tx.ExecContext(ctx, `DELETE FROM catalog_fanouts WHERE domain=? AND key=?`, DomainLibraryAvailability, k.ID)
		return true, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE catalog_fanouts SET after=? WHERE domain=? AND key=?`, ids[len(ids)-1], DomainLibraryAvailability, k.ID)
	return false, err
}

// deriveAssetIssues mirrors an audio file's source issue into the catalogue.
func deriveAssetIssues(ctx context.Context, tx *sql.Tx, ids string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_asset_issues WHERE asset_id IN(SELECT value FROM json_each(?))`, ids); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO catalog_asset_issues(library_id,asset_id,issue)
 SELECT cl.id,a.id,m.issue FROM json_each(?) j JOIN catalog_assets a ON a.id=j.value
 JOIN audio_source_metadata m ON m.asset_id=a.token JOIN catalog_libraries cl ON cl.library_id=m.library_id`, ids)
	return err
}

// deriveRelated keeps a movie's related-title facets: its genres (by provider
// id and by name) and its cast (by provider person id). CROSS JOIN fixes the
// join order to start from the batch: left to itself the planner starts from
// every term of the vocabulary and walks every entity that carries one, which
// grows with the library.
func deriveRelated(ctx context.Context, tx *sql.Tx, ids string) error {
	for _, q := range []string{
		`DELETE FROM catalog_related_facets WHERE entity_id IN(SELECT value FROM json_each(?1))`,
		`INSERT INTO catalog_related_facets(entity_id,library_id,relation,provider,facet_id,year)
 SELECT e.id,e.library_id,1,s.provider,s.source_id,e.year FROM json_each(?1) j CROSS JOIN catalog_entities e ON e.id=j.value AND e.kind=1 AND e.retired=0
 CROSS JOIN catalog_term_sources s ON s.entity_id=e.id CROSS JOIN catalog_terms t ON t.id=s.term_id AND t.vocab=1
 ON CONFLICT(entity_id,relation,provider,facet_id) DO NOTHING`,
		`INSERT INTO catalog_related_facets(entity_id,library_id,relation,provider,facet_id,year)
 SELECT e.id,e.library_id,1,'name',t.key,e.year FROM json_each(?1) j CROSS JOIN catalog_entities e ON e.id=j.value AND e.kind=1 AND e.retired=0
 CROSS JOIN catalog_entity_terms m ON m.entity_id=e.id CROSS JOIN catalog_terms t ON t.id=m.term_id AND t.vocab=1
 ON CONFLICT(entity_id,relation,provider,facet_id) DO NOTHING`,
		`INSERT INTO catalog_related_facets(entity_id,library_id,relation,provider,facet_id,year)
 SELECT e.id,e.library_id,2,c.provider,c.provider_person_id,e.year FROM json_each(?1) j CROSS JOIN catalog_entities e ON e.id=j.value AND e.kind=1 AND e.retired=0
 CROSS JOIN catalog_credits c ON c.entity_id=e.id CROSS JOIN catalog_credit_labels d ON d.id=c.department_id
 WHERE c.provider_person_id<>'' AND d.label='Acting'
 ON CONFLICT(entity_id,relation,provider,facet_id) DO NOTHING`,
	} {
		if _, err := tx.ExecContext(ctx, q, ids); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	Register(DomainAvailability, Derivation{
		Version: 1,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			ids := keyIDs(keys)
			return ids, deriveAvailability(ctx, tx, mustJSON(ids))
		},
		Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
			return scanIDs(ctx, tx, `SELECT e.id FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind AND k.playable=1 WHERE e.id>? ORDER BY e.id LIMIT ?`, after, limit)
		},
	})
	Register(DomainLibraryAvailability, Derivation{
		Version: 1,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			var done []int64
			for _, k := range keys {
				finished, err := deriveLibraryAvailability(ctx, tx, k, limit)
				if err != nil {
					return nil, err
				}
				if finished {
					done = append(done, k.ID)
				}
			}
			return done, nil
		},
		Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
			return nil, nil // availability rebuilds from items
		},
	})
	Register(DomainAssetIssues, Derivation{
		Version: 1,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			ids := keyIDs(keys)
			return ids, deriveAssetIssues(ctx, tx, mustJSON(ids))
		},
		Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
			return scanIDs(ctx, tx, `SELECT id FROM catalog_assets WHERE id>? ORDER BY id LIMIT ?`, after, limit)
		},
	})
	Register(DomainRelated, Derivation{
		Version: 1,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			ids := keyIDs(keys)
			return ids, deriveRelated(ctx, tx, mustJSON(ids))
		},
		Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
			return scanIDs(ctx, tx, `SELECT id FROM catalog_entities WHERE kind=1 AND id>? ORDER BY id LIMIT ?`, after, limit)
		},
	})
}
