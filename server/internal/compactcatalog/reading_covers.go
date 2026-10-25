package compactcatalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// ReadingCoverDigestsTx projects ready selected covers for an already admitted
// bounded set of entities. It supplies no visibility or source-access grant.
func ReadingCoverDigestsTx(ctx context.Context, tx *sql.Tx, entities []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(entities) == 0 {
		return out, nil
	}
	if tx == nil || len(entities) > 4096 {
		return nil, fmt.Errorf("invalid Reading cover projection")
	}
	encoded, err := json.Marshal(entities)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.entity_id,o.digest FROM json_each(?) selected
 JOIN artwork_selections a ON a.kind='item' AND a.entity_id=selected.value AND a.role='cover' AND a.subject=''
 JOIN artwork_objects o ON o.digest=a.digest AND o.status='ready'`, string(encoded))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entity int64
		var digest string
		if err = rows.Scan(&entity, &digest); err != nil {
			return nil, err
		}
		out[entity] = digest
	}
	return out, rows.Err()
}
