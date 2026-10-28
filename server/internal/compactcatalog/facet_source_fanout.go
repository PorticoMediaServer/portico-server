package compactcatalog

import (
	"context"
	"database/sql"
	"errors"
)

// These permanent domains are source maintenance, never query jobs. A source
// write queues one key; its dependants are visited later under the writer gate.
const (
	DomainFacetTermFanout  = 36
	DomainFacetAssetFanout = 37
	facetSourceQuantum    = 64
)

// The WITHOUT ROWID secondary index appends the remaining PK columns
// (provider,source_id). Retain the complete raw key: many source rows can belong
// to one holder. Do not DISTINCT the holder or filter overrides before LIMIT.
const facetTermFanoutSQL = `SELECT entity_id,provider,source_id
 FROM catalog_term_sources INDEXED BY catalog_term_sources_term
 WHERE term_id=? AND (entity_id,provider,source_id)>(?,?,?)
 ORDER BY entity_id,provider,source_id LIMIT ?`

const facetAssetFanoutSQL = `SELECT entity_id FROM catalog_asset_links INDEXED BY catalog_asset_links_asset
 WHERE asset_id=? AND entity_id>? ORDER BY entity_id LIMIT ?`

// drainFacetSource advances at most limit raw edges across the ENTIRE call,
// rather than limit edges per source key. The dirty revision fences the cursor.
// Cursor writes and accepted downstream markers share the caller's transaction;
// rollback cannot skip an edge, and a source re-touch restarts from the beginning.
// Insertions/deletions/moves of edges independently queue both affected holders.
// That covers edits behind an in-progress cursor as well as source ID reuse.
func drainFacetSource(ctx context.Context, tx *sql.Tx, domain int, keys []Key, limit int) ([]int64, error) {
	if limit <= 0 {
		return nil, nil
	}
	remaining := min(limit, facetSourceQuantum)
	done := make([]int64, 0, min(len(keys), remaining))
	for _, key := range keys {
		if remaining == 0 {
			break
		}
		var revision, entity int64
		var hasAfter bool
		var provider, source string
		err := tx.QueryRowContext(ctx, `SELECT revision,has_after,after_entity,after_provider,after_source
 FROM catalog_facet_source_fanouts WHERE domain=? AND source_id=?`, domain, key.ID).Scan(&revision, &hasAfter, &entity, &provider, &source)
		if errors.Is(err, sql.ErrNoRows) || err == nil && revision != key.Revision {
			hasAfter = false
			entity, provider, source = 0, "", ""
			_, err = tx.ExecContext(ctx, `INSERT INTO catalog_facet_source_fanouts(domain,source_id,revision) VALUES(?,?,?)
 ON CONFLICT(domain,source_id) DO UPDATE SET revision=excluded.revision,has_after=0,after_entity=0,after_provider='',after_source=''`, domain, key.ID, key.Revision)
		}
		if err != nil {
			return nil, err
		}
		var rows *sql.Rows
		if !hasAfter {
			if domain == DomainFacetTermFanout {
				rows, err = tx.QueryContext(ctx, `SELECT entity_id,provider,source_id FROM catalog_term_sources INDEXED BY catalog_term_sources_term WHERE term_id=? ORDER BY entity_id,provider,source_id LIMIT ?`, key.ID, remaining)
			} else {
				rows, err = tx.QueryContext(ctx, `SELECT entity_id FROM catalog_asset_links INDEXED BY catalog_asset_links_asset WHERE asset_id=? ORDER BY entity_id LIMIT ?`, key.ID, remaining)
			}
		} else if domain == DomainFacetTermFanout {
			rows, err = tx.QueryContext(ctx, facetTermFanoutSQL, key.ID, entity, provider, source, remaining)
		} else {
			rows, err = tx.QueryContext(ctx, facetAssetFanoutSQL, key.ID, entity, remaining)
		}
		if err != nil {
			return nil, err
		}
		ids := make([]int64, 0, remaining)
		for rows.Next() {
			if domain == DomainFacetTermFanout {
				err = rows.Scan(&entity, &provider, &source)
			} else {
				err = rows.Scan(&entity)
			}
			if err != nil {
				rows.Close()
				return nil, err
			}
			// Conservative acceptance also visits overridden/non-genre rows:
			// a coalesced vocab change can withdraw their previous genre value.
			ids = append(ids, entity)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if err = TouchTx(ctx, tx, DomainFacetCounts, ids...); err != nil {
			return nil, err
		}
		if len(ids) < remaining {
			// Empty sources still consume one unit, so zero-edge keys cannot
			// turn this into an unbounded checkpoint/acknowledgement batch.
			remaining -= max(1, len(ids))
			if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_facet_source_fanouts WHERE domain=? AND source_id=? AND revision=?`, domain, key.ID, key.Revision); err != nil {
				return nil, err
			}
			done = append(done, key.ID)
		} else {
			remaining -= len(ids)
			if _, err = tx.ExecContext(ctx, `UPDATE catalog_facet_source_fanouts SET has_after=1,after_entity=?,after_provider=?,after_source=?
 WHERE domain=? AND source_id=? AND revision=?`, entity, provider, source, domain, key.ID, key.Revision); err != nil {
				return nil, err
			}
		}
	}
	return done, nil
}

func init() {
	for _, domain := range []int{DomainFacetTermFanout, DomainFacetAssetFanout} {
		Register(domain, Derivation{
			Version: 1,
			Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
				return drainFacetSource(ctx, tx, domain, keys, limit)
			},
			Backfill: func(context.Context, *sql.Tx, int64, int) ([]int64, error) {
				// Domain33 rebuilds holder facts, including previous owners.
				return nil, nil
			},
		})
	}
}
