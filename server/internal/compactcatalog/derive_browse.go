package compactcatalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// Browse rows (domain 18) and memberships (19), their bucket counts, counted
// rows and Home buckets, and the duration/rating metrics fanned out from items
// to the containers they belong to (25, 26).
//
// A row is a function of its entity's facts. Every kind gets the generic row
// (title, sort key, year; an item's added time, availability, attributes and
// backdrop); a few shipped kinds refine it (a show's added time is its first
// episode's, a season is titled after its show, an author lives while a live
// book names it). An unknown kind always takes the generic path.

type browseRow struct {
	entity, library, kind        int64
	title, sortKey, head         string
	year                         int
	added                        sql.NullString
	item                         sql.NullInt64
	ratingKey, labelKey          string
	available, recent, backdrop  int
	durationBucket, ratingBucket string
}

// browseAttributeKeySQL is the '|'-joined distinct value keys of one attribute
// field of item e.
func browseAttributeKeySQL(field string) string {
	return `COALESCE((SELECT group_concat(value_key,'|') FROM (SELECT DISTINCT t.value_key FROM catalog_item_attribute_edges x
 JOIN catalog_attribute_terms t ON t.id=x.term_id JOIN catalog_attribute_fields f ON f.id=t.field_id
 WHERE x.item_id=e.id AND f.field='` + field + `' ORDER BY t.value_key)),'')`
}

// Each returns title, sort_key, year, added, item, rating_key, label_key,
// available, recent, backdrop for entity ?1, or no row when it has none.
const (
	playableRowSQL = `SELECT e.title,e.sort_key,e.year,d.added_text,e.id,` + "%s" + `,
 COALESCE((SELECT v.available FROM catalog_item_availability v WHERE v.entity_id=e.id),0),d.added_text IS NOT NULL AND e.kind<>11,COALESCE(d.backdrop_url,'')<>''
 FROM catalog_entities e LEFT JOIN catalog_item_details d ON d.entity_id=e.id
 WHERE e.id=?1 AND e.retired=0 AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=e.id)`
	containerRowSQL = `SELECT e.title,e.sort_key,e.year,NULL,NULL,'','',0,0,0 FROM catalog_entities e WHERE e.id=?1 AND e.retired=0`
)

// A show's and a season's added date is its earliest dated episode, read by
// index from catalog_container_added (one seek, whatever the episode count).
var refinedRowSQL = map[Kind]string{
	Show: `SELECT e.title,e.sort_key,e.year,
 (SELECT added_text FROM catalog_container_added WHERE container_id=e.id ORDER BY added_text LIMIT 1),NULL,'','',0,0,0
 FROM catalog_entities e WHERE e.id=?1 AND e.retired=0`,
	Season: `SELECT sh.title||' · Season '||c.number,portico_sort_title(sh.title||' · Season '||c.number,'',sh.metadata_language),sh.year,
 (SELECT added_text FROM catalog_container_added WHERE container_id=e.id ORDER BY added_text LIMIT 1),NULL,'','',0,0,0
 FROM catalog_entities e JOIN catalog_seasons c ON c.entity_id=e.id JOIN catalog_entities sh ON sh.id=c.show_id
 WHERE e.id=?1 AND e.retired=0`,
	Collection: `SELECT e.title,e.sort_key,0,c.created_at,NULL,'','',0,0,0
 FROM catalog_entities e JOIN catalog_collections c ON c.entity_id=e.id WHERE e.id=?1 AND e.retired=0`,
	// An author lives while a live book of its library names it.
	Author: `SELECT e.title,e.sort_key,0,NULL,NULL,'','',0,0,0
 FROM catalog_entities e WHERE e.id=?1 AND e.retired=0 AND EXISTS(SELECT 1 FROM catalog_books b INDEXED BY catalog_books_author_seek
  JOIN catalog_entities be ON be.id=b.entity_id AND be.retired=0
  WHERE b.library_id=e.library_id AND b.author=e.title COLLATE NOCASE)`,
}

var playableRow = fmt.Sprintf(playableRowSQL, browseAttributeKeySQL("contentRating")+","+browseAttributeKeySQL("label"))

// readBrowseRow computes an entity's browse row; sql.ErrNoRows when it has none.
func readBrowseRow(ctx context.Context, tx *sql.Tx, id int64) (browseRow, error) {
	row := browseRow{entity: id}
	var playable, browsable bool
	err := tx.QueryRowContext(ctx, `SELECT e.library_id,e.kind,k.playable,k.browsable FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind WHERE e.id=?`, id).Scan(&row.library, &row.kind, &playable, &browsable)
	if err != nil {
		return row, err
	}
	if !browsable {
		return row, sql.ErrNoRows
	}
	query, refined := refinedRowSQL[Kind(row.kind)]
	switch {
	case refined:
	case playable:
		query = playableRow
	default:
		query = containerRowSQL
	}
	err = tx.QueryRowContext(ctx, query, id).Scan(&row.title, &row.sortKey, &row.year, &row.added, &row.item, &row.ratingKey, &row.labelKey, &row.available, &row.recent, &row.backdrop)
	if err != nil {
		return row, err
	}
	if len(row.sortKey) > 0 {
		row.head = string([]rune(row.sortKey)[:1])
	}
	return row, nil
}

// Per-item statements that must stay primary-key or index seeks: each once
// walked a whole partition per item, which made derivation quadratic in the
// library. TestPerItemDerivationStatementsSeek holds their query plans.
const (
	browseBucketEmptyDelete = `DELETE FROM catalog_browse_buckets WHERE library_id=? AND kind=? AND container=? AND axis=? AND value=? AND rating_key=? AND label_key=? AND total=0`
	memberMetricDelete      = `DELETE FROM catalog_browse_member_metrics INDEXED BY catalog_browse_metric_item WHERE item_id=? AND entity_id=? AND source=?`
)

// browseStaticAxes: a row contributes one count to each static axis.
func browseStaticAxes(row browseRow) [7]string {
	year, decade := "", ""
	if row.year > 0 {
		year = strconv.Itoa(row.year)
		decade = strconv.Itoa((row.year/10)*10) + "s"
	}
	month, addedYear := "", ""
	if row.added.Valid && len(row.added.String) >= 7 {
		addedYear = row.added.String[:4]
		month = row.added.String[:7]
	}
	return [7]string{row.head, year, decade, month, addedYear, row.ratingBucket, row.durationBucket}
}

func rowContainer(row browseRow) int {
	if row.item.Valid {
		return 0
	}
	return 1
}

func adjustBrowseBuckets(ctx context.Context, tx *sql.Tx, row browseRow, delta int) error {
	container := rowContainer(row)
	for axis, value := range browseStaticAxes(row) {
		if delta > 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_browse_buckets(library_id,kind,container,axis,value,rating_key,label_key,total)
 VALUES(?,?,?,?,?,?,?,1) ON CONFLICT(library_id,kind,container,axis,value,rating_key,label_key) DO UPDATE SET total=total+1`, row.library, row.kind, container, axis, value, row.ratingKey, row.labelKey); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE catalog_browse_buckets SET total=total-1 WHERE library_id=? AND kind=? AND container=? AND axis=? AND value=? AND rating_key=? AND label_key=?`, row.library, row.kind, container, axis, value, row.ratingKey, row.labelKey); err != nil {
			return err
		}
		// Only the key just decremented can have emptied: one primary-key probe.
		if _, err := tx.ExecContext(ctx, browseBucketEmptyDelete, row.library, row.kind, container, axis, value, row.ratingKey, row.labelKey); err != nil {
			return err
		}
	}
	return nil
}

func readCountedRow(ctx context.Context, tx *sql.Tx, id int64) (browseRow, error) {
	row := browseRow{entity: id}
	var container int
	err := tx.QueryRowContext(ctx, `SELECT library_id,kind,container,head,year,added_text,rating_key,label_key,duration_bucket,rating_bucket FROM catalog_browse_counted_rows WHERE entity_id=?`, id).Scan(&row.library, &row.kind, &container, &row.head, &row.year, &row.added, &row.ratingKey, &row.labelKey, &row.durationBucket, &row.ratingBucket)
	if err == nil && container == 0 {
		row.item = sql.NullInt64{Int64: id, Valid: true}
	}
	return row, err
}

// deriveBrowseRow brings one entity's browse row, its bucket and Home
// contributions and its metrics up to date (or removes them).
func deriveBrowseRow(ctx context.Context, tx *sql.Tx, id int64) error {
	old, err := readCountedRow(ctx, tx, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	counted := err == nil
	row, err := readBrowseRow(ctx, tx, id)
	if errors.Is(err, sql.ErrNoRows) {
		if counted {
			if err = adjustBrowseBuckets(ctx, tx, old, -1); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_browse_counted_rows WHERE entity_id=?`, id); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_browse_rows WHERE entity_id=?`, id); err != nil {
			return err
		}
		return deriveHomeBuckets(ctx, tx, id)
	}
	if err != nil {
		return err
	}
	if counted {
		// Metric buckets move with the row; the metric refresh below re-adds them.
		row.durationBucket, row.ratingBucket = old.durationBucket, old.ratingBucket
		if err = adjustBrowseBuckets(ctx, tx, old, -1); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_browse_counted_rows WHERE entity_id=?`, id); err != nil {
			return err
		}
	}
	var item any
	if row.item.Valid {
		item = row.item.Int64
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_browse_rows(entity_id,library_id,kind,title,sort_key,head,year,added_text,item_id,rating_key,label_key,available,recent,backdrop)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(entity_id) DO UPDATE SET library_id=excluded.library_id,kind=excluded.kind,title=excluded.title,sort_key=excluded.sort_key,head=excluded.head,year=excluded.year,added_text=excluded.added_text,item_id=excluded.item_id,rating_key=excluded.rating_key,label_key=excluded.label_key,available=excluded.available,recent=excluded.recent,backdrop=excluded.backdrop`,
		id, row.library, row.kind, row.title, row.sortKey, row.head, row.year, row.added, item, row.ratingKey, row.labelKey, row.available, row.recent, row.backdrop); err != nil {
		return err
	}
	if err = deriveRecentText(ctx, tx, row); err != nil {
		return err
	}
	if err = adjustBrowseBuckets(ctx, tx, row, 1); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_browse_counted_rows(entity_id,library_id,kind,container,head,year,added_text,rating_key,label_key,duration_bucket,rating_bucket) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		id, row.library, row.kind, rowContainer(row), row.head, row.year, row.added, row.ratingKey, row.labelKey, row.durationBucket, row.ratingBucket); err != nil {
		return err
	}
	if err = refreshEntityMetrics(ctx, tx, id); err != nil {
		return err
	}
	return deriveHomeBuckets(ctx, tx, id)
}

// recentWorkKinds are the containers a Recently Added shelf lists in place of
// their members; foldedKinds are those members.
var (
	recentWorkKinds = map[int64]bool{int64(Show): true, int64(Album): true, int64(Book): true}
	foldedKinds     = map[int64]bool{int64(Episode): true, int64(Track): true, int64(Part): true}
)

// deriveRecentText keeps an item's side of Recently Added. The item's newest
// available added time is its metric (fanned out to its show, album or book,
// whose recent_text is their members' maximum: refreshEntityMetrics); a
// standalone item is a work of its own and carries it as recent_text.
func deriveRecentText(ctx context.Context, tx *sql.Tx, row browseRow) error {
	if !row.item.Valid {
		if recentWorkKinds[row.kind] {
			return nil
		}
		_, err := tx.ExecContext(ctx, `UPDATE catalog_browse_rows SET recent_text=NULL WHERE entity_id=? AND recent_text IS NOT NULL`, row.entity)
		return err
	}
	var added any
	if row.added.Valid && row.available == 1 && row.recent == 1 {
		added = row.added.String
	}
	own := added
	if foldedKinds[row.kind] {
		own = nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE catalog_browse_rows SET recent_text=? WHERE entity_id=? AND recent_text IS NOT ?`, own, row.entity, own); err != nil {
		return err
	}
	var changed bool
	if err := tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM catalog_browse_item_metrics WHERE item_id=?1 AND added IS ?2)`, row.entity, added).Scan(&changed); err != nil || !changed {
		return err
	}
	return TouchTx(ctx, tx, DomainItemMetrics, row.entity)
}

// deriveHomeBuckets moves one row's Home bucket contribution.
func deriveHomeBuckets(ctx context.Context, tx *sql.Tx, id int64) error {
	const columns = `library_id,kind,available,recent,backdrop,rating_key,label_key,dated,retired`
	for _, q := range []string{
		`UPDATE catalog_home_buckets AS b SET total=total-1 FROM catalog_home_counted_rows old WHERE old.entity_id=?1 AND (b.library_id,b.kind,b.available,b.recent,b.backdrop,b.rating_key,b.label_key,b.dated,b.retired)=(old.library_id,old.kind,old.available,old.recent,old.backdrop,old.rating_key,old.label_key,old.dated,old.retired)`,
		`DELETE FROM catalog_home_buckets WHERE total=0 AND (` + columns + `) IN(SELECT ` + columns + ` FROM catalog_home_counted_rows WHERE entity_id=?1)`,
		`DELETE FROM catalog_home_counted_rows WHERE entity_id=?1`,
		`INSERT INTO catalog_home_counted_rows SELECT entity_id,library_id,kind,available,recent,backdrop,rating_key,label_key,added_text IS NOT NULL,EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=entity_id) FROM catalog_browse_rows WHERE entity_id=?1 AND item_id IS NOT NULL`,
		`INSERT INTO catalog_home_buckets SELECT ` + columns + `,1 FROM catalog_home_counted_rows WHERE entity_id=?1 ON CONFLICT DO UPDATE SET total=total+1`,
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}

// membershipSQL is the membership set of item ?1 by source: itself (0), its
// show (1) and season (2), its album (3) and album artist (4), its credited
// artists (5), its book (6) and the book's author (7), and its collections (8).
const membershipSQL = `SELECT e.id,0 FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind AND k.playable=1 WHERE e.id=?1
 UNION SELECT ep.show_id,1 FROM catalog_episodes ep WHERE ep.entity_id=?1
 UNION SELECT ep.season_id,2 FROM catalog_episodes ep WHERE ep.entity_id=?1 AND ep.season_id IS NOT NULL
 UNION SELECT s.album_id,3 FROM catalog_songs s WHERE s.entity_id=?1
 UNION SELECT a.artist_id,4 FROM catalog_songs s JOIN catalog_albums a ON a.entity_id=s.album_id WHERE s.entity_id=?1
 UNION SELECT sa.artist_id,5 FROM catalog_song_artists sa WHERE sa.song_id=?1
 UNION SELECT f.book_id,6 FROM catalog_book_files f WHERE f.entity_id=?1
 UNION SELECT au.id,7 FROM catalog_book_files f JOIN catalog_books b ON b.entity_id=f.book_id
  JOIN catalog_libraries cl ON cl.id=b.library_id
  JOIN catalog_identities i ON i.root=cl.root AND i.source_key=portico_author_key(b.author)
  JOIN catalog_entities au ON au.public_id=i.public_id
  WHERE f.entity_id=?1 AND b.author<>''
 UNION SELECT m.collection_id,8 FROM catalog_collection_members m WHERE m.item_id=?1`

type browseEdge struct {
	entity int64
	source int
}

// deriveBrowseMembers brings the given items' memberships to their current
// parents. A new edge needs its container's row first; changed containers
// re-derive their rows once per batch; changed items requeue their metrics.
func deriveBrowseMembers(ctx context.Context, tx *sql.Tx, items []int64) error {
	containers := map[int64]bool{}
	for _, item := range items {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities WHERE id=?)`, item).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			continue // deleted: its memberships went with it
		}
		want, err := readEdges(ctx, tx, membershipSQL, item)
		if err != nil {
			return err
		}
		have, err := readEdges(ctx, tx, `SELECT entity_id,source FROM catalog_browse_memberships WHERE item_id=?1`, item)
		if err != nil {
			return err
		}
		changed := false
		for edge := range have {
			if want[edge] {
				continue
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_browse_memberships WHERE entity_id=? AND item_id=? AND source=?`, edge.entity, item, edge.source); err != nil {
				return err
			}
			if edge.entity != item {
				containers[edge.entity] = true
			}
			changed = true
		}
		for edge := range want {
			if have[edge] {
				continue
			}
			var published bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_browse_rows WHERE entity_id=?)`, edge.entity).Scan(&published); err != nil {
				return err
			}
			if !published {
				if err = deriveBrowseRow(ctx, tx, edge.entity); err != nil {
					return err
				}
			}
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO catalog_browse_memberships(entity_id,item_id,source) VALUES(?,?,?)`, edge.entity, item, edge.source); err != nil {
				return err
			}
			if edge.entity != item {
				containers[edge.entity] = true
			}
			changed = true
		}
		if changed {
			if err = TouchTx(ctx, tx, DomainItemMetrics, item); err != nil {
				return err
			}
		}
	}
	ordered := make([]int64, 0, len(containers))
	for id := range containers {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	for _, id := range ordered {
		if err := deriveBrowseRow(ctx, tx, id); err != nil {
			return fmt.Errorf("browse container %d: %w", id, err)
		}
	}
	return nil
}

func readEdges(ctx context.Context, tx *sql.Tx, query string, item int64) (map[browseEdge]bool, error) {
	rows, err := tx.QueryContext(ctx, query, item)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[browseEdge]bool{}
	for rows.Next() {
		var e browseEdge
		if err = rows.Scan(&e.entity, &e.source); err != nil {
			return nil, err
		}
		out[e] = true
	}
	return out, rows.Err()
}

// Metrics. An item's duration (its longest file) and rating (its best
// provider rating) are fanned out to every container it belongs to; a
// container's row shows the max over its members. The fanout is bounded per
// step and resumes from a cursor; the cursor restarts when the item is queued
// again (its catalog_dirty revision moved).

func metricCursor(ctx context.Context, tx *sql.Tx, domain int, k Key) (afterEntity int64, afterSource int, afterItem int64, err error) {
	var sequence, version int64
	err = tx.QueryRowContext(ctx, `SELECT sequence,projection_version,after_entity,after_source,after_item FROM catalog_browse_metric_cursors WHERE domain=? AND entity_id=?`, domain, k.ID).Scan(&sequence, &version, &afterEntity, &afterSource, &afterItem)
	if err == nil && sequence == k.Revision {
		return
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return
	}
	afterEntity, afterSource, afterItem = 0, -1, 0
	_, err = tx.ExecContext(ctx, `INSERT INTO catalog_browse_metric_cursors(domain,entity_id,sequence,projection_version,after_entity,after_source,after_item) VALUES(?,?,?,1,0,-1,0)
 ON CONFLICT(domain,entity_id) DO UPDATE SET sequence=excluded.sequence,after_entity=0,after_source=-1,after_item=0`, domain, k.ID, k.Revision)
	return
}

func finishMetricCursor(ctx context.Context, tx *sql.Tx, domain int, id int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM catalog_browse_metric_cursors WHERE domain=? AND entity_id=?`, domain, id)
	return err
}

// deriveAssetFanout queues the metrics of the items an asset backs.
func deriveAssetFanout(ctx context.Context, tx *sql.Tx, k Key, limit int) (bool, error) {
	_, _, afterItem, err := metricCursor(ctx, tx, DomainAssetMetrics, k)
	if err != nil {
		return false, err
	}
	items, err := scanIDs(ctx, tx, `SELECT entity_id FROM catalog_asset_links INDEXED BY catalog_asset_links_asset WHERE asset_id=? AND entity_id>? ORDER BY entity_id LIMIT ?`, k.ID, afterItem, limit)
	if err != nil {
		return false, err
	}
	if err = TouchTx(ctx, tx, DomainItemMetrics, items...); err != nil {
		return false, err
	}
	if len(items) < limit {
		return true, finishMetricCursor(ctx, tx, DomainAssetMetrics, k.ID)
	}
	_, err = tx.ExecContext(ctx, `UPDATE catalog_browse_metric_cursors SET after_item=? WHERE domain=? AND entity_id=?`, items[len(items)-1], DomainAssetMetrics, k.ID)
	return false, err
}

type metricEdge struct {
	entity int64
	source int
}

// metricEdges merges the item's recorded member metrics with its current
// memberships after the cursor: two indexed streams, each bounded by limit.
func metricEdges(ctx context.Context, tx *sql.Tx, item, afterEntity int64, afterSource, limit int) ([]metricEdge, error) {
	var all []metricEdge
	for _, query := range []string{
		`SELECT entity_id,source FROM catalog_browse_member_metrics INDEXED BY catalog_browse_metric_item WHERE item_id=? AND (entity_id,source)>(?,?) ORDER BY entity_id,source LIMIT ?`,
		`SELECT entity_id,source FROM catalog_browse_memberships INDEXED BY catalog_browse_membership_item WHERE item_id=? AND (entity_id,source)>(?,?) ORDER BY entity_id,source LIMIT ?`,
	} {
		rows, err := tx.QueryContext(ctx, query, item, afterEntity, afterSource, limit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var e metricEdge
			if err = rows.Scan(&e.entity, &e.source); err != nil {
				rows.Close()
				return nil, err
			}
			all = append(all, e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].entity != all[j].entity {
			return all[i].entity < all[j].entity
		}
		return all[i].source < all[j].source
	})
	unique := all[:0]
	for _, e := range all {
		if len(unique) > 0 && unique[len(unique)-1] == e {
			continue
		}
		unique = append(unique, e)
		if len(unique) == limit {
			break
		}
	}
	return unique, nil
}

// deriveItemMetrics recomputes one item's metrics and fans them out to its
// containers, at most limit containers per step.
func deriveItemMetrics(ctx context.Context, tx *sql.Tx, k Key, limit int, touched map[int64]bool) (bool, error) {
	afterEntity, afterSource, _, err := metricCursor(ctx, tx, DomainItemMetrics, k)
	if err != nil {
		return false, err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities WHERE id=?)`, k.ID).Scan(&exists); err != nil {
		return false, err
	}
	var duration, rating sql.NullFloat64
	var added sql.NullString
	if exists {
		if err = tx.QueryRowContext(ctx, `SELECT max(a.duration) FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id WHERE l.entity_id=?`, k.ID).Scan(&duration); err != nil {
			return false, err
		}
		if err = tx.QueryRowContext(ctx, `SELECT max(value) FROM metadata_ratings WHERE item_id=? AND provider IN('tmdb','imdb','musicbrainz','audible')`, k.ID).Scan(&rating); err != nil {
			return false, err
		}
		if err = tx.QueryRowContext(ctx, `SELECT added_text FROM catalog_browse_rows WHERE entity_id=? AND available=1 AND recent=1`, k.ID).Scan(&added); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_browse_item_metrics(item_id,duration,rating,added) VALUES(?,?,?,?) ON CONFLICT(item_id) DO UPDATE SET duration=excluded.duration,rating=excluded.rating,added=excluded.added`, k.ID, duration, rating, added); err != nil {
			return false, err
		}
	} else if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_browse_item_metrics WHERE item_id=?`, k.ID); err != nil {
		return false, err
	}
	edges, err := metricEdges(ctx, tx, k.ID, afterEntity, afterSource, limit)
	if err != nil {
		return false, err
	}
	for _, edge := range edges {
		if _, err = tx.ExecContext(ctx, memberMetricDelete, k.ID, edge.entity, edge.source); err != nil {
			return false, err
		}
		if exists {
			if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_browse_member_metrics(entity_id,item_id,source,duration,rating,added)
 SELECT m.entity_id,m.item_id,m.source,?,?,? FROM catalog_browse_memberships m JOIN catalog_browse_rows r ON r.entity_id=m.entity_id
 WHERE m.entity_id=? AND m.item_id=? AND m.source=?`, duration, rating, added, edge.entity, k.ID, edge.source); err != nil {
				return false, err
			}
		}
		touched[edge.entity] = true
	}
	if len(edges) < limit {
		return true, finishMetricCursor(ctx, tx, DomainItemMetrics, k.ID)
	}
	last := edges[len(edges)-1]
	_, err = tx.ExecContext(ctx, `UPDATE catalog_browse_metric_cursors SET after_entity=?,after_source=? WHERE domain=? AND entity_id=?`, last.entity, last.source, DomainItemMetrics, k.ID)
	return false, err
}

func browseDurationBucket(value sql.NullFloat64) string {
	if !value.Valid || math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0) {
		return ""
	}
	return strconv.FormatInt(int64(math.Trunc(value.Float64/1800))*30, 10)
}

func browseRatingBucket(value sql.NullFloat64) string {
	if !value.Valid || math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0) {
		return ""
	}
	return strconv.FormatInt(int64(math.Trunc(value.Float64)), 10)
}

// refreshEntityMetrics moves a row's metric buckets to its members' current
// maxima (two index starts, never a member scan).
func refreshEntityMetrics(ctx context.Context, tx *sql.Tx, entity int64) error {
	var library, kind int64
	var container int
	var ratingKey, labelKey, oldDuration, oldRating string
	err := tx.QueryRowContext(ctx, `SELECT library_id,kind,container,rating_key,label_key,duration_bucket,rating_bucket FROM catalog_browse_counted_rows WHERE entity_id=?`, entity).Scan(&library, &kind, &container, &ratingKey, &labelKey, &oldDuration, &oldRating)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var duration, rating sql.NullFloat64
	if err = tx.QueryRowContext(ctx, `SELECT duration FROM catalog_browse_member_metrics INDEXED BY catalog_browse_metric_duration WHERE entity_id=? AND duration IS NOT NULL ORDER BY duration DESC LIMIT 1`, entity).Scan(&duration); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT rating FROM catalog_browse_member_metrics INDEXED BY catalog_browse_metric_rating WHERE entity_id=? AND rating IS NOT NULL ORDER BY rating DESC LIMIT 1`, entity).Scan(&rating); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	newDuration, newRating := browseDurationBucket(duration), browseRatingBucket(rating)
	for _, axis := range []struct {
		n         int
		old, next string
	}{{5, oldRating, newRating}, {6, oldDuration, newDuration}} {
		if axis.old == axis.next {
			continue
		}
		if _, err = tx.ExecContext(ctx, `UPDATE catalog_browse_buckets SET total=total-1 WHERE library_id=? AND kind=? AND container=? AND axis=? AND value=? AND rating_key=? AND label_key=?`, library, kind, container, axis.n, axis.old, ratingKey, labelKey); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, browseBucketEmptyDelete, library, kind, container, axis.n, axis.old, ratingKey, labelKey); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_browse_buckets(library_id,kind,container,axis,value,rating_key,label_key,total) VALUES(?,?,?,?,?,?,?,1) ON CONFLICT(library_id,kind,container,axis,value,rating_key,label_key) DO UPDATE SET total=total+1`, library, kind, container, axis.n, axis.next, ratingKey, labelKey); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE catalog_browse_counted_rows SET duration_bucket=?,rating_bucket=? WHERE entity_id=?`, newDuration, newRating, entity); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE catalog_browse_rows SET duration_max=?,rating_max=? WHERE entity_id=?`, duration, rating, entity); err != nil {
		return err
	}
	if !recentWorkKinds[kind] {
		return nil
	}
	var added sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT added FROM catalog_browse_member_metrics INDEXED BY catalog_browse_metric_added WHERE entity_id=? AND added IS NOT NULL ORDER BY added DESC LIMIT 1`, entity).Scan(&added); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE catalog_browse_rows SET recent_text=? WHERE entity_id=? AND recent_text IS NOT ?`, added, entity, added)
	return err
}

func init() {
	Register(DomainBrowseRows, Derivation{
		Version: 1,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			ids := keyIDs(keys)
			for _, id := range ids {
				if err := deriveBrowseRow(ctx, tx, id); err != nil {
					return nil, fmt.Errorf("browse row %d: %w", id, err)
				}
			}
			return ids, nil
		},
		Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
			return scanIDs(ctx, tx, `SELECT id FROM catalog_entities WHERE id>? ORDER BY id LIMIT ?`, after, limit)
		},
	})
	Register(DomainBrowseEdges, Derivation{
		Version: 1,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			ids := keyIDs(keys)
			return ids, deriveBrowseMembers(ctx, tx, ids)
		},
		Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
			return scanIDs(ctx, tx, `SELECT e.id FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind AND k.playable=1 WHERE e.id>? ORDER BY e.id LIMIT ?`, after, limit)
		},
	})
	Register(DomainAssetMetrics, Derivation{
		Version: 1,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			var done []int64
			for _, k := range keys {
				finished, err := deriveAssetFanout(ctx, tx, k, limit)
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
			return nil, nil // item metrics rebuild from items
		},
	})
	Register(DomainItemMetrics, Derivation{
		Version: 1,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			touched := map[int64]bool{}
			var done []int64
			for _, k := range keys {
				finished, err := deriveItemMetrics(ctx, tx, k, limit, touched)
				if err != nil {
					return nil, err
				}
				if finished {
					done = append(done, k.ID)
				}
			}
			// Each container refreshes once per batch, not once per member.
			ordered := make([]int64, 0, len(touched))
			for id := range touched {
				ordered = append(ordered, id)
			}
			sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
			for _, id := range ordered {
				if err := refreshEntityMetrics(ctx, tx, id); err != nil {
					return nil, err
				}
			}
			return done, nil
		},
		Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
			return scanIDs(ctx, tx, `SELECT e.id FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind AND k.playable=1 WHERE e.id>? ORDER BY e.id LIMIT ?`, after, limit)
		},
	})
}
