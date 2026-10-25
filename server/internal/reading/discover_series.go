package reading

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/personalstate"
	"portico.local/server/internal/readingcap"
)

// DiscoverSeries names a library-scoped accepted series, with exact visible
// membership/read counts and at most three canonical members for its cover fan.
// These facts confer neither source admission nor a person/work identity.
type DiscoverSeries struct {
	ID, LibraryID, Name string
	Count, ReadCount    int
	Preview             []LibraryRow
}

func (s *Service) discoverSeriesTx(ctx context.Context, tx *sql.Tx, p identity.Principal, library string, limit int, visible string, visibleArgs []any) ([]DiscoverSeries, error) {
	out := []DiscoverSeries{}
	if s.SeriesScope == nil {
		return out, identity.ErrForbidden
	}
	scope, err := s.SeriesScope(ctx, tx, p, library)
	if err != nil {
		return out, err
	}
	var handle int64
	if err = tx.QueryRowContext(ctx, `SELECT cl.id FROM catalog_libraries cl JOIN libraries lib ON lib.id=cl.library_id WHERE cl.library_id=? AND cl.retired=0 AND lib.kind IN('book','comic')`, library).Scan(&handle); err != nil {
		return out, err
	}
	generation, err := compactcatalog.ReadingProjectionReadyTx(ctx, tx, handle)
	if errors.Is(err, compactcatalog.ErrBuilding) {
		return out, ErrChanged
	}
	if err != nil {
		return out, err
	}
	// Count the entire eligible scope before limiting the set of series or their
	// previews. Label choice uses owner spelling, then frequency and binary ties.
	// The optional mark is personal state, so it is read only for a viewer this server lets
	// use personal state at all. A guest or grant context keeps the columns absent even when a
	// personal key exists; the aggregate read count keeps its own unchanged query.
	personal := identity.CanUsePersonalState(p)
	markReadSQL, markRevisionSQL := "NULL", "NULL"
	if personal {
		markReadSQL = "(SELECT wi.watched FROM personal_watched_intents wi WHERE wi.profile_id=? AND wi.item_id=e.id)"
		markRevisionSQL = "(SELECT revision FROM personal_items WHERE profile_id=? AND item_id=e.id)"
	}
	query := `WITH eligible AS MATERIALIZED(
 SELECT m.series_id,m.series_key,m.series_value,m.series_source,m.numbered_order,m.number_value,m.sort_key,
 e.id AS entity,pid(e.public_id) AS public_id,cl.library_id,e.title,p.format,a.token,
 EXISTS(SELECT 1 FROM inventory_item_availability available WHERE available.item_id=e.id AND available.asset_id=a.token AND available.available=1) AS available,
 ` + personalstate.CompactSQL("?", "e.id") + ` AS read,
 ` + markReadSQL + ` AS mark_read,
 ` + markRevisionSQL + ` AS mark_revision
 FROM reading_attribution_members m JOIN catalog_entities e ON e.id=m.item_id AND e.library_id=m.library_id
 JOIN catalog_libraries cl ON cl.id=e.library_id JOIN libraries lib ON lib.id=cl.library_id
 JOIN reading_publications p ON p.item_id=e.id AND p.asset_id=` + s.primaryPublicationSQL() + `
 JOIN catalog_asset_links link ON link.entity_id=e.id AND link.asset_id=p.asset_id JOIN catalog_assets a ON a.id=p.asset_id
 WHERE m.library_id=? AND m.generation=? AND m.series_id IS NOT NULL AND e.retired=0 AND cl.retired=0
 AND ((lib.kind='book' AND e.kind=20 AND p.format IN(` + s.bookFormatsSQL() + `)) OR (lib.kind='comic' AND e.kind=21 AND p.format IN(` + readingcap.ComicFormatsSQL() + `)))
 AND (` + visible + `)),
 spellings AS(SELECT series_id,series_key,series_value,count(*) AS frequency,sum(series_source='owner') AS owners FROM eligible GROUP BY series_id,series_key,series_value),
 labels AS(SELECT series_id,series_key,series_value,row_number() OVER(PARTITION BY series_id ORDER BY owners>0 DESC,CASE WHEN owners>0 THEN owners ELSE frequency END DESC,series_value COLLATE BINARY) AS label_rank FROM spellings),
 selected AS(SELECT labels.series_id,labels.series_key,labels.series_value,count(*) AS total,sum(e.read) AS read_count FROM labels JOIN eligible e ON e.series_id=labels.series_id WHERE labels.label_rank=1 GROUP BY labels.series_id,labels.series_key,labels.series_value ORDER BY labels.series_key COLLATE BINARY,labels.series_id LIMIT ?),
 members AS(SELECT e.*,row_number() OVER(PARTITION BY e.series_id ORDER BY e.numbered_order,e.number_value,e.sort_key COLLATE BINARY,e.entity) AS member_rank FROM eligible e JOIN selected s ON s.series_id=e.series_id)
 SELECT s.series_id,s.series_value,s.total,s.read_count,m.entity,m.public_id,m.library_id,m.title,m.format,m.token,m.available,m.mark_read,m.mark_revision
 FROM selected s JOIN members m ON m.series_id=s.series_id AND m.member_rank<=3
 ORDER BY s.series_key COLLATE BINARY,s.series_id,m.member_rank`
	args := []any{scope.Profile}
	if personal {
		args = append(args, scope.Profile, scope.Profile)
	}
	args = append(args, handle, generation.Ready)
	args = append(args, visibleArgs...)
	args = append(args, limit)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	var preview []LibraryRow
	for rows.Next() {
		var id, name string
		var total, read int
		var item LibraryRow
		var markRead sql.NullBool
		var markRevision sql.NullInt64
		if err = rows.Scan(&id, &name, &total, &read, &item.entity, &item.ID, &item.LibraryID, &item.Title, &item.Format, &item.SourceID, &item.InventoryAvailable, &markRead, &markRevision); err != nil {
			break
		}
		// A mark needs both its explicit intent and the revision that carries it.
		item.MarkRead, item.MarkRevision, item.MarkKnown = markRead.Bool, markRevision.Int64, markRead.Valid && markRevision.Valid
		if total < 1 || read < 0 || read > total || item.LibraryID != library {
			err = ErrChanged
			break
		}
		if len(out) == 0 || out[len(out)-1].ID != id {
			out = append(out, DiscoverSeries{ID: id, LibraryID: library, Name: name, Count: total, ReadCount: read, Preview: []LibraryRow{}})
		}
		out[len(out)-1].Preview = append(out[len(out)-1].Preview, item)
		preview = append(preview, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	// This is metadata-only projection for the bounded, already visible preview.
	if err = readingAttributionProjectionTx(ctx, tx, preview); err != nil {
		return nil, err
	}
	at := 0
	for i := range out {
		for j := range out[i].Preview {
			out[i].Preview[j] = preview[at]
			at++
		}
	}
	return out, nil
}
