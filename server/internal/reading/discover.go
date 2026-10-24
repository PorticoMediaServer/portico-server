package reading

import (
	"context"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// DiscoverLimits bounds one Discover response. Zero values take the defaults,
// and every value is clamped: the producer never scans or publishes more than
// discoverScanCap titles for its series row.
type DiscoverLimits struct {
	Recent, Series, SeriesItems, Scan int
}

const (
	discoverRecent      = 20
	discoverSeries      = 8
	discoverSeriesItems = 4
	discoverScanCap     = 2000
	discoverScanBatch   = 100
)

// DiscoverSeries is one accepted series with its display label, its exact count
// inside the scanned window, and its first items in reading order.
type DiscoverSeries struct {
	ID, Name string
	Count    int
	Items    []LibraryRow
}

// Discover is the server-composed Discover payload built from facts already in
// this snapshot: recently added titles and accepted series. It invents no dates,
// counts, kinds or people, and it never reads a path or archive.
type Discover struct {
	Recent           []LibraryRow
	Series           []DiscoverSeries
	SeriesLowerBound bool
}

func (l DiscoverLimits) bounded() DiscoverLimits {
	clamp := func(value, fallback, high int) int {
		if value <= 0 {
			value = fallback
		}
		if value > high {
			value = high
		}
		return value
	}
	return DiscoverLimits{
		Recent:      clamp(l.Recent, discoverRecent, 100),
		Series:      clamp(l.Series, discoverSeries, 50),
		SeriesItems: clamp(l.SeriesItems, discoverSeriesItems, 20),
		Scan:        clamp(l.Scan, discoverScanCap, 10000),
	}
}

// Discover returns the bounded server-composed rows for one authorized library
// or for every library the viewer may read. All reads share one snapshot and
// the same profile/library/restriction predicate as List.
func (s *Service) Discover(ctx context.Context, p identity.Principal, library string, limits DiscoverLimits) (Discover, error) {
	out := Discover{Recent: []LibraryRow{}, Series: []DiscoverSeries{}}
	if s.Visible == nil {
		return out, identity.ErrForbidden
	}
	limits = limits.bounded()
	g, err := dbwork.BeginSnapshot(ctx, s.DB)
	if err != nil {
		return out, err
	}
	defer g.Rollback()
	tx := g.Tx()
	filter, args, err := s.Visible(ctx, tx, p, "e.id")
	if err != nil {
		return out, err
	}
	if filter == "" {
		return out, identity.ErrForbidden
	}
	selectFrom := `SELECT e.id,pid(e.public_id),cl.library_id,e.title,p.format,a.token,
 EXISTS(SELECT 1 FROM inventory_item_availability available WHERE available.item_id=e.id AND available.asset_id=a.token AND available.available=1)
 FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id JOIN libraries lib ON lib.id=cl.library_id
 JOIN reading_publications p ON p.item_id=e.id AND p.asset_id=` + primaryPublicationSQL + `
 JOIN catalog_asset_links link ON link.entity_id=e.id AND link.asset_id=p.asset_id
 JOIN catalog_assets a ON a.id=p.asset_id
 WHERE e.retired=0 AND cl.retired=0
 AND ((lib.kind='book' AND e.kind=20 AND p.format IN('epub','pdf')) OR (lib.kind='comic' AND e.kind=21 AND p.format IN('cbz','pdf')))
 AND (?='' OR cl.library_id=?) AND (` + filter + `)`
	rowsFor := func(after int64, limit int) ([]LibraryRow, error) {
		query := selectFrom + ` AND e.id>? ORDER BY e.id LIMIT ?`
		values := append([]any{library, library}, args...)
		values = append(values, after, limit)
		rows, err := tx.QueryContext(ctx, query, values...)
		if err != nil {
			return nil, err
		}
		var page []LibraryRow
		for rows.Next() {
			var id int64
			var row LibraryRow
			if err = rows.Scan(&id, &row.ID, &row.LibraryID, &row.Title, &row.Format, &row.SourceID, &row.InventoryAvailable); err != nil {
				rows.Close()
				return nil, err
			}
			row.entity = id
			page = append(page, row)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err = rows.Close(); err != nil {
			return nil, err
		}
		return page, nil
	}
	// Recently added: the catalogue's own added_ms fact, newest first.
	{
		query := selectFrom + ` AND e.added_ms IS NOT NULL ORDER BY e.added_ms DESC,e.id DESC LIMIT ?`
		values := append([]any{library, library}, args...)
		values = append(values, limits.Recent)
		rows, err := tx.QueryContext(ctx, query, values...)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var id int64
			var row LibraryRow
			if err = rows.Scan(&id, &row.ID, &row.LibraryID, &row.Title, &row.Format, &row.SourceID, &row.InventoryAvailable); err != nil {
				rows.Close()
				return out, err
			}
			row.entity = id
			out.Recent = append(out.Recent, row)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return out, err
		}
		if err = rows.Close(); err != nil {
			return out, err
		}
		if err = readingAttributionProjectionTx(ctx, tx, out.Recent); err != nil {
			return out, err
		}
	}
	// Accepted series: one bounded keyset pass over eligible titles, the same
	// accepted-value projection List uses, then the facet/series reader. A series
	// id is library-scoped, so an all-libraries request returns no series row.
	if library == "" {
		return out, nil
	}
	scanned, after := 0, int64(0)
	facets := []compactcatalog.ReadingFacetItem{}
	for scanned < limits.Scan {
		want := limits.Scan - scanned
		if want > discoverScanBatch {
			want = discoverScanBatch
		}
		page, err := rowsFor(after, want)
		if err != nil {
			return out, err
		}
		if len(page) == 0 {
			break
		}
		ids := make([]int64, len(page))
		for i := range page {
			ids[i] = page[i].entity
		}
		values, err := compactcatalog.ReadingAcceptedValuesTx(ctx, tx, ids)
		if err != nil {
			return out, err
		}
		for i := range page {
			facets = append(facets, compactcatalog.ReadingFacetItem{Item: page[i].entity, TitleKey: page[i].Title, Values: values[page[i].entity]})
		}
		scanned += len(page)
		after = page[len(page)-1].entity
		if len(page) < want {
			break
		}
	}
	if scanned == limits.Scan {
		// A full window does not prove there are no further titles.
		rest, err := rowsFor(after, 1)
		if err != nil {
			return out, err
		}
		out.SeriesLowerBound = len(rest) != 0
	}
	groups := compactcatalog.ReadingFacets(library, facets).Series
	order := make([]int, len(groups))
	for i := range order {
		order[i] = i
	}
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && (groups[order[j]].Count > groups[order[j-1]].Count || groups[order[j]].Count == groups[order[j-1]].Count && compactcatalog.ReadingValueKey(groups[order[j]].Name) < compactcatalog.ReadingValueKey(groups[order[j-1]].Name)); j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	if len(order) > limits.Series {
		order = order[:limits.Series]
	}
	var chosen []int64
	members := map[string][]compactcatalog.ReadingFacetItem{}
	for _, index := range order {
		series := groups[index]
		list, ok := compactcatalog.ReadingSeriesItems(library, series.ID, facets)
		if !ok {
			continue
		}
		if len(list) > limits.SeriesItems {
			list = list[:limits.SeriesItems]
		}
		members[series.ID] = list
		for _, item := range list {
			chosen = append(chosen, item.Item)
		}
		out.Series = append(out.Series, DiscoverSeries{ID: series.ID, Name: series.Name, Count: series.Count})
	}
	if len(chosen) > 0 {
		marks := ""
		values := []any{library, library}
		values = append(values, args...)
		for i, id := range chosen {
			if i > 0 {
				marks += ","
			}
			marks += "?"
			values = append(values, id)
		}
		rows, err := tx.QueryContext(ctx, selectFrom+` AND e.id IN (`+marks+`) ORDER BY e.id`, values...)
		if err != nil {
			return out, err
		}
		byItem := map[int64]LibraryRow{}
		for rows.Next() {
			var id int64
			var row LibraryRow
			if err = rows.Scan(&id, &row.ID, &row.LibraryID, &row.Title, &row.Format, &row.SourceID, &row.InventoryAvailable); err != nil {
				rows.Close()
				return out, err
			}
			row.entity = id
			byItem[id] = row
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return out, err
		}
		if err = rows.Close(); err != nil {
			return out, err
		}
		var ordered []LibraryRow
		for i := range out.Series {
			out.Series[i].Items = []LibraryRow{}
			for _, item := range members[out.Series[i].ID] {
				if row, ok := byItem[item.Item]; ok {
					out.Series[i].Items = append(out.Series[i].Items, row)
					ordered = append(ordered, row)
				}
			}
		}
		if err = readingAttributionProjectionTx(ctx, tx, ordered); err != nil {
			return out, err
		}
		attributed := map[int64]LibraryRow{}
		for i := range ordered {
			attributed[ordered[i].entity] = ordered[i]
		}
		for i := range out.Series {
			for j := range out.Series[i].Items {
				out.Series[i].Items[j] = attributed[out.Series[i].Items[j].entity]
			}
		}
	}
	return out, nil
}
