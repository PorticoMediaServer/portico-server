package compactcatalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

// Facet counts (catalog_facet_members, catalog_facet_counts). An item's facet
// values are read from its facts; the item is counted once per value, under
// its library and its browse row's restriction keys (rating_key, label_key),
// so a reader decides each count against a viewer's restriction the way the
// browse buckets are decided. A change moves the item's counts by one, never
// recounts a value. Triggers queue the item on every source fact (feeds.sql).
//
// In a show library the show holds the facets, not its episodes: Genre, Year
// and the rest then count shows ("Drama 12" beside "24 shows"), which is what a
// Shows grid filters. A show's values are its own genres and year, and whatever
// any of its episodes says about the file or the title (resolution, network,
// studio…), the same facts a filter on the show reaches through its episodes.
// An episode's change is therefore its show's change (Drain, and the episode
// triggers of migration 0008).

// DomainFacetCounts keeps catalog_facet_members and catalog_facet_counts.
const DomainFacetCounts = 33

// facetValueSQL reads one item's values of each field (?1 is the item).
var facetValueSQL = map[string]string{
	"genre":  `SELECT COALESCE(ts.label_override,mg.label) FROM catalog_term_sources ts JOIN catalog_terms mg ON mg.id=ts.term_id AND mg.vocab=1 WHERE ts.entity_id=?1`,
	"decade": `SELECT CAST(((year/10)*10) AS TEXT) FROM catalog_browse_rows WHERE entity_id=?1 AND year BETWEEN 1800 AND 2199`,
	"year":   `SELECT CAST(year AS TEXT) FROM catalog_browse_rows WHERE entity_id=?1 AND year BETWEEN 1800 AND 2199`,
	"resolution": `SELECT CASE WHEN ast.height>=2000 THEN '4k' WHEN ast.height>=1000 THEN '1080p' WHEN ast.height>=700 THEN '720p' ELSE 'sd' END
	 FROM catalog_asset_links ia JOIN catalog_assets ast ON ast.id=ia.asset_id WHERE ia.entity_id=?1`,
	// A collection is kept by its entity id (a public id is spelled at read
	// time, so the count never holds a stale spelling).
	"collection": `SELECT CAST(collection_id AS TEXT) FROM catalog_collection_members WHERE item_id=?1`,
}

// FacetAttributeFields are the attribute fields counted as facets.
var FacetAttributeFields = []string{"contentRating", "audioLanguage", "studio", "network", "tag", "label", "series"}

// showFacetValueSQL reads one show's values of each field (?1 is the show):
// its own genres and year, and the union of its episodes' file facts.
var showFacetValueSQL = map[string]string{
	"genre":  facetValueSQL["genre"],
	"decade": facetValueSQL["decade"],
	"year":   facetValueSQL["year"],
	"resolution": `SELECT DISTINCT CASE WHEN ast.height>=2000 THEN '4k' WHEN ast.height>=1000 THEN '1080p' WHEN ast.height>=700 THEN '720p' ELSE 'sd' END
	 FROM catalog_episodes ep JOIN catalog_asset_links ia ON ia.entity_id=ep.entity_id JOIN catalog_assets ast ON ast.id=ia.asset_id WHERE ep.show_id=?1`,
	"collection": `SELECT CAST(collection_id AS TEXT) FROM catalog_collection_members WHERE item_id=?1
	 UNION SELECT CAST(cm.collection_id AS TEXT) FROM catalog_episodes ep JOIN catalog_collection_members cm ON cm.item_id=ep.entity_id WHERE ep.show_id=?1`,
}

const showFacetAttributeSQL = `SELECT DISTINCT ca.source_value FROM catalog_episodes ep JOIN catalog_item_attribute_edges ca ON ca.item_id=ep.entity_id
 JOIN catalog_attribute_terms t ON t.id=ca.term_id JOIN catalog_attribute_fields f ON f.id=t.field_id AND f.field=?2 WHERE ep.show_id=?1`

const facetAttributeSQL = `SELECT ca.source_value FROM catalog_item_attribute_edges ca JOIN catalog_attribute_terms t ON t.id=ca.term_id
 JOIN catalog_attribute_fields f ON f.id=t.field_id AND f.field=?2 WHERE ca.item_id=?1`

type facetKey struct{ field, value string }

type facetPlace struct {
	library             int64
	ratingKey, labelKey string
}

func deriveFacetCounts(ctx context.Context, tx *sql.Tx, id int64) error {
	old := map[facetKey]bool{}
	var was facetPlace
	rows, err := tx.QueryContext(ctx, `SELECT field,value,library_id,rating_key,label_key FROM catalog_facet_members WHERE entity_id=?`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var k facetKey
		if err = rows.Scan(&k.field, &k.value, &was.library, &was.ratingKey, &was.labelKey); err != nil {
			rows.Close()
			return err
		}
		old[k] = true
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	next := map[facetKey]bool{}
	var now facetPlace
	var item sql.NullInt64
	var kind int64
	err = tx.QueryRowContext(ctx, `SELECT library_id,rating_key,label_key,item_id,kind FROM catalog_browse_rows WHERE entity_id=?`, id).Scan(&now.library, &now.ratingKey, &now.labelKey, &item, &kind)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	// Facets count items, and in a show library shows: an entity without a
	// browse row, an episode, or any other container holds none.
	show := err == nil && kind == int64(Show)
	if err == nil && (show || item.Valid && kind != int64(Episode)) {
		read := func(field, query string, args ...any) error {
			values, err := tx.QueryContext(ctx, query, args...)
			if err != nil {
				return err
			}
			defer values.Close()
			for values.Next() {
				var value sql.NullString
				if err = values.Scan(&value); err != nil {
					return err
				}
				if value.Valid && value.String != "" {
					next[facetKey{field, value.String}] = true
				}
			}
			return values.Err()
		}
		values, attributes, subject := facetValueSQL, facetAttributeSQL, item.Int64
		if show {
			values, attributes, subject = showFacetValueSQL, showFacetAttributeSQL, id
		}
		for field, query := range values {
			if err = read(field, query, subject); err != nil {
				return err
			}
		}
		for _, field := range FacetAttributeFields {
			if err = read(field, attributes, subject, field); err != nil {
				return err
			}
		}
	}
	moved := len(old) > 0 && now != was
	for k := range old {
		if next[k] && !moved {
			continue
		}
		if err = adjustFacetCount(ctx, tx, was, k, -1); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_facet_members WHERE entity_id=? AND field=? AND value=?`, id, k.field, k.value); err != nil {
			return err
		}
	}
	for k := range next {
		if old[k] && !moved {
			continue
		}
		if err = adjustFacetCount(ctx, tx, now, k, 1); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_facet_members(entity_id,field,value,library_id,rating_key,label_key) VALUES(?,?,?,?,?,?)`,
			id, k.field, k.value, now.library, now.ratingKey, now.labelKey); err != nil {
			return err
		}
	}
	return nil
}

func adjustFacetCount(ctx context.Context, tx *sql.Tx, at facetPlace, k facetKey, delta int) error {
	if delta > 0 {
		_, err := tx.ExecContext(ctx, `INSERT INTO catalog_facet_counts(library_id,field,value,rating_key,label_key,total) VALUES(?,?,?,?,?,1)
		 ON CONFLICT(library_id,field,value,rating_key,label_key) DO UPDATE SET total=total+1`, at.library, k.field, k.value, at.ratingKey, at.labelKey)
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE catalog_facet_counts SET total=total-1 WHERE library_id=? AND field=? AND value=? AND rating_key=? AND label_key=?`,
		at.library, k.field, k.value, at.ratingKey, at.labelKey); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM catalog_facet_counts WHERE library_id=? AND field=? AND value=? AND rating_key=? AND label_key=? AND total=0`,
		at.library, k.field, k.value, at.ratingKey, at.labelKey)
	return err
}

func init() {
	Register(DomainFacetCounts, Derivation{
		Version: 3,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			done := make([]int64, 0, len(keys))
			derived := make(map[int64]bool, len(keys))
			// An episode's facts are its show's facets: each show the batch
			// touches is derived once, however many of its episodes changed.
			shows := []int64{}
			for _, key := range keys {
				if err := deriveFacetCounts(ctx, tx, key.ID); err != nil {
					return done, err
				}
				derived[key.ID] = true
				var show sql.NullInt64
				if err := tx.QueryRowContext(ctx, `SELECT show_id FROM catalog_episodes WHERE entity_id=?`, key.ID).Scan(&show); err != nil && !errors.Is(err, sql.ErrNoRows) {
					return done, err
				}
				if show.Valid {
					shows = append(shows, show.Int64)
				}
				done = append(done, key.ID)
			}
			for _, show := range shows {
				if derived[show] {
					continue
				}
				derived[show] = true
				if err := deriveFacetCounts(ctx, tx, show); err != nil {
					return done, err
				}
			}
			return done, nil
		},
		Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
			// Episodes are visited too: version 2 counted them, and their rows are cleared here.
			return scanIDs(ctx, tx, `SELECT entity_id FROM catalog_browse_rows WHERE entity_id>? AND (item_id IS NOT NULL OR kind=`+strconv.Itoa(int(Show))+`) ORDER BY entity_id LIMIT ?`, after, limit)
		},
	})
}

// CheckFacetCounts verifies every facet count against its member rows (for
// tests and diagnostics; it reads both tables whole).
func CheckFacetCounts(ctx context.Context, db *sql.DB) error {
	var diff int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM (
	 SELECT library_id,field,value,rating_key,label_key,total FROM catalog_facet_counts
	 EXCEPT SELECT library_id,field,value,rating_key,label_key,count(*) FROM catalog_facet_members GROUP BY 1,2,3,4,5)`).Scan(&diff)
	if err != nil {
		return err
	}
	var missing int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM (
	 SELECT library_id,field,value,rating_key,label_key,count(*) FROM catalog_facet_members GROUP BY 1,2,3,4,5
	 EXCEPT SELECT library_id,field,value,rating_key,label_key,total FROM catalog_facet_counts)`).Scan(&missing); err != nil {
		return err
	}
	if diff != 0 || missing != 0 {
		return fmt.Errorf("facet counts: %d counts disagree with their members, %d member groups uncounted", diff, missing)
	}
	return nil
}
