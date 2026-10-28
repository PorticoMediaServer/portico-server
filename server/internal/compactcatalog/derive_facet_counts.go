package compactcatalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
// An episode's change reaches its show through what the episode says about
// it (catalog_show_facet_episodes): only a change to that moves the show's
// value counts and derives the show again, and the show reads the counts,
// never its episodes (Drain, and the episode triggers of the baseline).

// FacetHolderSQL is the browse rows that can hold facet values, over the
// alias e: shows, and items other than episodes. It is the condition
// deriveFacetCounts applies, so a filtered grouping tests its predicate on
// these rows only.
//
// The kind set (shows and the playable kinds other than episodes, whose
// browse rows carry their item) lets a reader seek the (library, kind)
// indexes instead of reading every row of the library.
func FacetHolderSQL(e string) string {
	show, episode := strconv.Itoa(int(Show)), strconv.Itoa(int(Episode))
	return `(` + e + `.kind IN(SELECT id FROM catalog_kinds WHERE id=` + show + ` OR playable=1 AND id<>` + episode + `) AND (` + e + `.kind=` + show + ` OR ` + e + `.item_id IS NOT NULL))`
}

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

// showOwnFacetSQL reads a show's own values (?1 is the show): its genres,
// year and collections. Everything its episodes say about it (their files'
// resolution, their collections, their descriptive attributes) is read from
// catalog_show_facet_values, which episodeFacetSourcesSQL keeps.
var showOwnFacetSQL = map[string]string{
	"genre":      facetValueSQL["genre"],
	"decade":     facetValueSQL["decade"],
	"year":       facetValueSQL["year"],
	"collection": facetValueSQL["collection"],
}

// episodeFacetSourcesSQL is what one episode (?1) says about its show, as
// (field, value) pairs; ?2 is the descriptive attribute fields.
const episodeFacetSourcesSQL = `SELECT 'resolution',CASE WHEN ast.height>=2000 THEN '4k' WHEN ast.height>=1000 THEN '1080p' WHEN ast.height>=700 THEN '720p' ELSE 'sd' END
 FROM catalog_asset_links ia JOIN catalog_assets ast ON ast.id=ia.asset_id WHERE ia.entity_id=?1
 UNION SELECT 'collection',CAST(collection_id AS TEXT) FROM catalog_collection_members WHERE item_id=?1
 UNION SELECT f.field,ca.source_value FROM catalog_item_attribute_edges ca CROSS JOIN catalog_attribute_terms t ON t.id=ca.term_id
 CROSS JOIN catalog_attribute_fields f ON f.id=t.field_id WHERE ca.item_id=?1 AND f.field IN(SELECT value FROM json_each(?2))`

// deriveEpisodeFacetSources brings what one episode says about its show up to
// date, moving its show's value counts by the difference. It returns the
// shows whose values changed (the old and the new one when it moved). Its
// cost is the episode's own facts, whatever the size of the show.
func deriveEpisodeFacetSources(ctx context.Context, tx *sql.Tx, id int64) ([]int64, error) {
	type source struct {
		show int64
		k    facetKey
	}
	old := map[source]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT show_id,field,value FROM catalog_show_facet_episodes WHERE episode_id=?`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s source
		if err = rows.Scan(&s.show, &s.k.field, &s.k.value); err != nil {
			rows.Close()
			return nil, err
		}
		old[s] = true
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	next := map[source]bool{}
	var show int64
	err = tx.QueryRowContext(ctx, `SELECT show_id FROM catalog_episodes WHERE entity_id=?`, id).Scan(&show)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return nil, err
	default:
		fields, _ := json.Marshal(FacetAttributeFields)
		rows, err := tx.QueryContext(ctx, episodeFacetSourcesSQL, id, string(fields))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var field string
			var value sql.NullString
			if err = rows.Scan(&field, &value); err != nil {
				rows.Close()
				return nil, err
			}
			if value.Valid && value.String != "" {
				next[source{show, facetKey{field, value.String}}] = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	changed := map[int64]bool{}
	for s := range old {
		if next[s] {
			continue
		}
		changed[s.show] = true
		if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_show_facet_episodes WHERE episode_id=? AND field=? AND value=?`, id, s.k.field, s.k.value); err != nil {
			return nil, err
		}
		// The last episode saying a value removes it; otherwise one fewer says it.
		res, err := tx.ExecContext(ctx, `DELETE FROM catalog_show_facet_values WHERE show_id=? AND field=? AND value=? AND total=1`, s.show, s.k.field, s.k.value)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			if _, err = tx.ExecContext(ctx, `UPDATE catalog_show_facet_values SET total=total-1 WHERE show_id=? AND field=? AND value=?`, s.show, s.k.field, s.k.value); err != nil {
				return nil, err
			}
		}
	}
	for s := range next {
		if old[s] {
			continue
		}
		changed[s.show] = true
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_show_facet_episodes(episode_id,show_id,field,value) VALUES(?,?,?,?)`, id, s.show, s.k.field, s.k.value); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_show_facet_values(show_id,field,value,total) VALUES(?,?,?,1) ON CONFLICT(show_id,field,value) DO UPDATE SET total=total+1`, s.show, s.k.field, s.k.value); err != nil {
			return nil, err
		}
	}
	out := make([]int64, 0, len(changed))
	for show := range changed {
		out = append(out, show)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

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
	// browse row, an episode, or any other container holds none
	// (FacetHolderSQL states the same condition for readers).
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
		values, subject := facetValueSQL, item.Int64
		if show {
			values, subject = showOwnFacetSQL, id
		}
		for field, query := range values {
			if err = read(field, query, subject); err != nil {
				return err
			}
		}
		// Resolve descriptive fields together. A show reads what its episodes
		// say from their maintained value counts, never its episodes.
		attributeQuery := `SELECT DISTINCT f.field,ca.source_value FROM catalog_item_attribute_edges ca CROSS JOIN catalog_attribute_terms t ON t.id=ca.term_id CROSS JOIN catalog_attribute_fields f ON f.id=t.field_id WHERE ca.item_id=?1 AND f.field IN(SELECT value FROM json_each(?2))`
		if show {
			attributeQuery = `SELECT field,value FROM catalog_show_facet_values WHERE show_id=?1 AND ?2 IS NOT NULL`
		}
		fields, _ := json.Marshal(FacetAttributeFields)
		rows, err := tx.QueryContext(ctx, attributeQuery, subject, string(fields))
		if err != nil {
			return err
		}
		for rows.Next() {
			var field string
			var value sql.NullString
			if err = rows.Scan(&field, &value); err != nil {
				rows.Close()
				return err
			}
			if value.Valid && value.String != "" {
				next[facetKey{field, value.String}] = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
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
	_, err = tx.ExecContext(ctx, `UPDATE catalog_facet_members SET recent_key=COALESCE((SELECT COALESCE(recent_text,added_text) FROM catalog_browse_rows WHERE entity_id=?1),'') WHERE entity_id=?1`, id)
	return err
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
		Version: 6,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			done := make([]int64, 0, len(keys))
			derived := make(map[int64]bool, len(keys))
			// An episode's facts are its show's facets: each show the batch
			// touches is derived once, however many of its episodes changed.
			shows := []int64{}
			// Episodes' contributions first, so a show in the same batch reads
			// its counts after them.
			for _, key := range keys {
				changed, err := deriveEpisodeFacetSources(ctx, tx, key.ID)
				if err != nil {
					return done, err
				}
				shows = append(shows, changed...)
			}
			for _, key := range keys {
				if err := deriveFacetCounts(ctx, tx, key.ID); err != nil {
					return done, err
				}
				derived[key.ID] = true
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
			// Visit current AND previously accepted owners. Missing browse rows
			// must still withdraw their old counts/episode contributions. Seek
			// each prefix separately instead of DISTINCT/UNION over every value
			// of a holder with arbitrarily many accepted values.
			ids := make([]int64, 0, limit)
			for len(ids) < limit {
				var next sql.NullInt64
				if err := tx.QueryRowContext(ctx, `SELECT min(id) FROM (
 SELECT (SELECT entity_id FROM catalog_browse_rows WHERE entity_id>?1 ORDER BY entity_id LIMIT 1) id
 UNION ALL SELECT (SELECT entity_id FROM catalog_facet_members WHERE entity_id>?1 ORDER BY entity_id LIMIT 1)
 UNION ALL SELECT (SELECT episode_id FROM catalog_show_facet_episodes WHERE episode_id>?1 ORDER BY episode_id LIMIT 1))`, after).Scan(&next); err != nil {
					return nil, err
				}
				if !next.Valid {
					break
				}
				after = next.Int64
				ids = append(ids, after)
			}
			return ids, nil
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
