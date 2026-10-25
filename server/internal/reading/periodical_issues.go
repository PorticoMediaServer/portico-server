package reading

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// PeriodicalIssues reads dated issues from one accepted typed series in a complete ready generation. Its anchor
// names an indexed member, then verifies the full ordering tuple digest in this
// same authorized snapshot. The cursor never carries a truncated sort key.
func (s *Service) PeriodicalIssues(ctx context.Context, p identity.Principal, library, series, publicationType, cursor string, limit int) (SeriesPage, error) {
	return s.PeriodicalIssuesAt(ctx, p, library, series, publicationType, "", cursor, limit)
}

// PeriodicalIssuesAt optionally restricts membership to one actual accepted date.
// A month value is exact month precision, never an inferred day or date range.
func (s *Service) PeriodicalIssuesAt(ctx context.Context, p identity.Principal, library, series, publicationType, issueDate, cursor string, limit int) (SeriesPage, error) {
	out := SeriesPage{Items: []LibraryRow{}}
	if s == nil || s.DB == nil || s.Visible == nil || s.SeriesScope == nil {
		return out, identity.ErrForbidden
	}
	if publicationType != "magazine" && publicationType != "newspaper" {
		return out, ErrInvalid
	}
	if issueDate != "" && (!compactcatalog.ValidReadingIssueDate(issueDate) || publicationType == "newspaper" && len(issueDate) != 10) {
		return out, ErrInvalid
	}
	if library == "" || len(library) > 128 || len(series) != 43 || limit < 1 || limit > 100 || len(cursor) > 4096 {
		return out, ErrInvalid
	}
	g, err := dbwork.BeginSnapshot(ctx, s.DB)
	if err != nil {
		return out, err
	}
	defer g.Rollback()
	tx := g.Tx()
	scope, err := s.SeriesScope(ctx, tx, p, library)
	if err != nil {
		return out, err
	}
	var handle int64
	var kind string
	if err = tx.QueryRowContext(ctx, `SELECT cl.id,lib.kind FROM catalog_libraries cl JOIN libraries lib ON lib.id=cl.library_id WHERE cl.library_id=? AND cl.retired=0 AND lib.kind='book'`, library).Scan(&handle, &kind); err != nil {
		return out, err
	}
	generation, err := compactcatalog.ReadingProjectionReadyTx(ctx, tx, handle)
	if errors.Is(err, compactcatalog.ErrBuilding) {
		return out, ErrChanged
	}
	if err != nil {
		return out, err
	}
	filter, visibleArgs, err := s.Visible(ctx, tx, p, "e.id")
	if err != nil {
		return out, err
	}
	if filter == "" {
		return out, identity.ErrForbidden
	}
	cursorDomain := "periodical-issues-v1:" + publicationType
	if issueDate != "" {
		cursorDomain += ":date:" + issueDate
	}
	digest, err := seriesScopeDigest(library, series, cursorDomain, scope, limit)
	if err != nil {
		return out, err
	}
	var encoded string
	if err = tx.QueryRowContext(ctx, `SELECT value FROM configuration WHERE key='catalog_cursor_key'`).Scan(&encoded); err != nil {
		return out, err
	}
	key, err := hex.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return out, ErrChanged
	}
	current := seriesCursor{Namespace: generation.Namespace, Input: generation.Input, Ready: generation.Ready, Catalog: scope.Catalog, Viewer: scope.Viewer, Expiry: time.Now().Add(30 * time.Minute).Unix(), Scope: digest}
	base := ` FROM reading_attribution_members m JOIN catalog_entities e ON e.id=m.item_id AND e.library_id=m.library_id
 JOIN catalog_libraries cl ON cl.id=e.library_id JOIN libraries lib ON lib.id=cl.library_id
 JOIN reading_publications p ON p.item_id=e.id AND p.asset_id=` + primaryPublicationSQL + `
 JOIN catalog_asset_links link ON link.entity_id=e.id AND link.asset_id=p.asset_id JOIN catalog_assets a ON a.id=p.asset_id
 WHERE m.library_id=? AND m.generation=? AND m.series_id=? AND e.retired=0 AND cl.retired=0
 AND lib.kind='book' AND e.kind=20 AND p.format IN('epub','pdf')
 AND m.publication_type=? AND m.issue_date IS NOT NULL AND (?='magazine' OR length(m.issue_date)=10)
 AND (` + filter + `)`
	args := append([]any{handle, generation.Ready, series, publicationType, publicationType}, visibleArgs...)
	if issueDate != "" {
		base += ` AND m.issue_date=?`
		args = append(args, issueDate)
	}
	var after *seriesTuple
	if cursor != "" {
		captured, e := decodeSeriesCursor(cursor, key, time.Now())
		if e != nil {
			return out, e
		}
		if captured.Namespace != current.Namespace || captured.Input != current.Input || captured.Ready != current.Ready || captured.Catalog != current.Catalog || captured.Viewer != current.Viewer || captured.Scope != current.Scope {
			return out, ErrChanged
		}
		anchor := seriesTuple{}
		anchorArgs := append(append([]any{}, args...), captured.Anchor)
		if err = tx.QueryRowContext(ctx, `SELECT 0,0,m.issue_date,m.item_id`+base+` AND m.item_id=?`, anchorArgs...).Scan(&anchor.Numbered, &anchor.Number, &anchor.Sort, &anchor.Item); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return out, ErrChanged
			}
			return out, err
		}
		tuple, e := anchor.digest()
		if e != nil || tuple != captured.Tuple {
			return out, ErrChanged
		}
		after = &anchor
		current.Expiry = captured.Expiry
	}
	query := `SELECT e.id,pid(e.public_id),cl.library_id,e.title,p.format,a.token,
 EXISTS(SELECT 1 FROM inventory_item_availability available WHERE available.item_id=e.id AND available.asset_id=a.token AND available.available=1),
 0,0,m.issue_date` + base
	if after != nil {
		query += ` AND (m.issue_date,m.item_id)<(?,?)`
		args = append(args, after.Sort, after.Item)
	}
	query += ` ORDER BY m.issue_date DESC,m.item_id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	var tuples []seriesTuple
	for rows.Next() {
		var item LibraryRow
		var tuple seriesTuple
		if err = rows.Scan(&item.entity, &item.ID, &item.LibraryID, &item.Title, &item.Format, &item.SourceID, &item.InventoryAvailable, &tuple.Numbered, &tuple.Number, &tuple.Sort); err != nil {
			rows.Close()
			return out, err
		}
		if !compactcatalog.ValidReadingIssueDate(tuple.Sort) || publicationType == "newspaper" && len(tuple.Sort) != 10 {
			rows.Close()
			return out, ErrChanged
		}
		item.IssueDate = tuple.Sort
		tuple.Item = item.entity
		if _, err = tuple.digest(); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, item)
		tuples = append(tuples, tuple)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) == 0 && cursor == "" && issueDate == "" {
		return out, ErrUnavailable
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		tuples = tuples[:limit]
		last := tuples[len(tuples)-1]
		current.Anchor = last.Item
		current.Tuple, err = last.digest()
		if err != nil {
			return out, err
		}
		out.NextCursor, err = current.encode(key)
		if err != nil {
			return out, err
		}
	}
	if err = readingAttributionProjectionTx(ctx, tx, out.Items); err != nil {
		return SeriesPage{}, err
	}
	if !time.Unix(current.Expiry, 0).After(time.Now()) {
		return SeriesPage{}, ErrChanged
	}
	return out, nil
}
