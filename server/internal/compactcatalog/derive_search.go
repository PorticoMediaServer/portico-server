package compactcatalog

import (
	"context"
	"database/sql"
)

// Search documents: every live entity of a searchable kind has one document
// titled like the entity, and a row in the title index. The kind column holds
// two tokens, k<kind> and l<library>, so a search chooses its candidates
// inside the index already scoped to the viewer's libraries.

const searchLiveSQL = `e.retired=0 AND EXISTS(SELECT 1 FROM catalog_kinds k WHERE k.id=e.kind AND k.searchable=1)`

func deriveSearch(ctx context.Context, tx *sql.Tx, ids string) error {
	for _, query := range []string{
		`INSERT INTO catalog_search_titles(catalog_search_titles,rowid,title,kind)
 SELECT 'delete',d.entity_id,d.title,d.kind FROM catalog_search_documents d WHERE d.entity_id IN(SELECT value FROM json_each(?))`,
		`DELETE FROM catalog_search_documents WHERE entity_id IN(SELECT value FROM json_each(?))`,
		`INSERT INTO catalog_search_documents(entity_id,title,kind)
 SELECT e.id,e.title,'k'||e.kind||' l'||e.library_id FROM json_each(?) j JOIN catalog_entities e ON e.id=j.value WHERE ` + searchLiveSQL,
		`INSERT INTO catalog_search_titles(rowid,title,kind)
 SELECT d.entity_id,d.title,d.kind FROM catalog_search_documents d WHERE d.entity_id IN(SELECT value FROM json_each(?))`,
	} {
		if _, err := tx.ExecContext(ctx, query, ids); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	Register(DomainSearch, Derivation{
		Version: 2,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			ids := keyIDs(keys)
			return ids, deriveSearch(ctx, tx, mustJSON(ids))
		},
		Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
			return scanIDs(ctx, tx, `SELECT id FROM catalog_entities WHERE id>? ORDER BY id LIMIT ?`, after, limit)
		},
	})
}
