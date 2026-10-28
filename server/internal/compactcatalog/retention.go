package compactcatalog

import (
	"context"
	"database/sql"
	"log"
	"portico.local/server/internal/apievents"
	"portico.local/server/internal/dbwork"
	"time"
)

// RunRetention owns both the daily relational walk and periodic native-FTS
// upkeep. It uses the existing goroutine and one timer; each writer is released
// before the next unit and retains the maintenance class below scan/user work.
func RunRetention(ctx context.Context, db *sql.DB) {
	nextPrune := time.Now()
	nextMerge := nextPrune.Add(titleGramsMergeInterval)
	for ctx.Err() == nil {
		now := time.Now()
		if !now.Before(nextMerge) {
			if e := maintainTitleGrams(ctx, db); e != nil && ctx.Err() == nil {
				log.Printf("Catalogue title index maintenance will retry: %v", e)
			}
			nextMerge = time.Now().Add(titleGramsMergeInterval)
		}
		if ctx.Err() != nil {
			return
		}
		if !time.Now().Before(nextPrune) {
			if !dbwork.WaitBackground(ctx) {
				return
			}
			workCtx := dbwork.WithBackgroundSpeed(ctx, dbwork.ReadBackgroundSpeed(ctx, db))
			started := time.Now()
			more := false
			e := dbwork.WithWriteTx(workCtx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
				var e error
				more, e = PruneTx(workCtx, tx, 32*dbwork.BackgroundConcurrency(workCtx))
				return e
			})
			if e == nil && more {
				if !dbwork.PaceBackground(workCtx, started) {
					return
				}
				continue
			}
			nextPrune = time.Now().Add(24 * time.Hour)
		}
		next := nextPrune
		if nextMerge.Before(next) {
			next = nextMerge
		}
		timer := time.NewTimer(max(time.Until(next), 0))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func PruneTx(ctx context.Context, tx *sql.Tx, limit int) (bool, error) {
	dictionaries := []struct{ table, predicate string }{
		{"catalog_search_prefix_terms", `NOT EXISTS(SELECT 1 FROM catalog_search_prefix_rows r WHERE r.term_id=d.id) AND NOT EXISTS(SELECT 1 FROM catalog_search_prefix_rows_next r WHERE r.term_id=d.id)`},
		{"catalog_people", `NOT EXISTS(SELECT 1 FROM catalog_credits c WHERE c.person_id=d.id) AND NOT EXISTS(SELECT 1 FROM show_people_credits c WHERE c.identity_key=d.identity_key)`},
		{"catalog_terms", `NOT EXISTS(SELECT 1 FROM catalog_entity_terms c WHERE c.term_id=d.id)`},
		{"catalog_attribute_terms", `NOT EXISTS(SELECT 1 FROM catalog_item_attribute_edges c WHERE c.term_id=d.id)`},
		{"catalog_credit_labels", `d.label<>'' AND NOT EXISTS(SELECT 1 FROM catalog_credits c WHERE c.role_id=d.id) AND NOT EXISTS(SELECT 1 FROM catalog_credits c WHERE c.department_id=d.id)`},
	}
	for _, d := range dictionaries {
		var after int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT after_id FROM catalog_retention_cursors WHERE name=?),0)`, d.table).Scan(&after); err != nil {
			return false, err
		}
		ids, err := scanIDs(ctx, tx, `SELECT id FROM `+d.table+` WHERE id>? ORDER BY id LIMIT ?`, after, limit)
		if err != nil {
			return false, err
		}
		if len(ids) == 0 {
			continue
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM `+d.table+` AS d WHERE id IN(SELECT value FROM json_each(?)) AND `+d.predicate, mustJSON(ids)); err != nil {
			return false, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_retention_cursors(name,after_id) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET after_id=excluded.after_id`, d.table, ids[len(ids)-1]); err != nil {
			return false, err
		}
		return true, nil
	}
	var root, key string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT after_root FROM catalog_retention_cursors WHERE name='identities'),''),COALESCE((SELECT after_key FROM catalog_retention_cursors WHERE name='identities'),'')`).Scan(&root, &key); err != nil {
		return false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT root,source_key FROM catalog_identities WHERE (root,source_key)>(?,?) ORDER BY root,source_key LIMIT ?`, root, key, limit)
	if err != nil {
		return false, err
	}
	var pairs [][2]string
	for rows.Next() {
		var pair [2]string
		if err = rows.Scan(&pair[0], &pair[1]); err != nil {
			break
		}
		pairs = append(pairs, pair)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return false, err
	}
	if len(pairs) > 0 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_identities WHERE (root,source_key) IN(SELECT json_extract(value,'$[0]'),json_extract(value,'$[1]') FROM json_each(?)) AND NOT EXISTS(SELECT 1 FROM catalog_entities e WHERE e.public_id=catalog_identities.public_id)`, mustJSON(pairs)); err != nil {
			return false, err
		}
		last := pairs[len(pairs)-1]
		_, err = tx.ExecContext(ctx, `INSERT INTO catalog_retention_cursors(name,after_id,after_root,after_key) VALUES('identities',0,?,?) ON CONFLICT(name) DO UPDATE SET after_root=excluded.after_root,after_key=excluded.after_key`, last[0], last[1])
		return true, err
	}
	for _, table := range ServingTables {
		result, err := tx.ExecContext(ctx, `DELETE FROM `+table+`_previous WHERE (generation,`+previousKeys[table]+`) IN(SELECT generation,`+previousKeys[table]+` FROM `+table+`_previous WHERE generation<(SELECT generation FROM catalog_generations) OR (generation=(SELECT generation FROM catalog_generations) AND (SELECT retaining FROM catalog_generations)=0) LIMIT ?)`, limit)
		if err != nil {
			return false, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM api_events WHERE id IN(SELECT id FROM api_events WHERE at_ms<? ORDER BY at_ms LIMIT ?)`, time.Now().Add(-7*24*time.Hour).UnixMilli(), limit)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	var maximum int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(id),0) FROM api_events`).Scan(&maximum); err != nil {
		return false, err
	}
	if err = apievents.Trim(tx, maximum); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM catalog_retention_cursors`)
	return false, err
}

var previousKeys = map[string]string{
	"catalog_browse_availability_counts": "library_id,kind,availability_state,rating_key,label_key,container",
	"catalog_movie_category_summaries":   "library_id,kind,value",
	"catalog_movie_category_members":     "library_id,kind,value,item_id",
	"catalog_browse_rows":                "entity_id",
	"catalog_browse_blocks":              "library_id,kind,ordering,sort_value,entity_id",
	"catalog_browse_buckets":             "library_id,kind,container,axis,value,rating_key,label_key",
	"catalog_browse_counted_rows":        "entity_id",
	"catalog_browse_memberships":         "entity_id,item_id,source",
	"catalog_browse_member_metrics":      "entity_id,item_id,source",
	"catalog_home_buckets":               "library_id,kind,available,recent,backdrop,rating_key,label_key,dated,retired",
	"catalog_item_availability":          "entity_id",
	"catalog_facet_counts":               "library_id,field,value,rating_key,label_key",
	"catalog_related_facets":             "entity_id,relation,provider,facet_id",
	"catalog_browse_groups":              "library_id,field,value",
	"catalog_facet_members":              "entity_id,field,value",
}
