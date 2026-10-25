package reading

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
)

// One canonical archive slot, never an invented "on this day" recommendation.
func (s *Service) discoverNewspaperTx(ctx context.Context, tx *sql.Tx, p identity.Principal, library, visible string, visibleArgs []any) (*PeriodicalGroup, error) {
	if s.SeriesScope == nil {
		return nil, identity.ErrForbidden
	}
	if _, err := s.SeriesScope(ctx, tx, p, library); err != nil {
		return nil, err
	}
	var handle int64
	if err := tx.QueryRowContext(ctx, `SELECT cl.id FROM catalog_libraries cl JOIN libraries lib ON lib.id=cl.library_id WHERE cl.library_id=? AND cl.retired=0 AND lib.kind='book'`, library).Scan(&handle); err != nil {
		return nil, err
	}
	generation, err := compactcatalog.ReadingProjectionReadyTx(ctx, tx, handle)
	if errors.Is(err, compactcatalog.ErrBuilding) {
		return nil, ErrChanged
	}
	if err != nil {
		return nil, err
	}
	query := s.periodicalGroupsCTE(visible) + `SELECT series_id,series_value,issue_count,entity,public_id,library_id,title,format,token,available,issue_date FROM groups_page ORDER BY series_key COLLATE BINARY,series_id LIMIT 1`
	args := append([]any{handle, generation.Ready, "newspaper", "newspaper"}, visibleArgs...)
	var group PeriodicalGroup
	item := &group.Latest
	err = tx.QueryRowContext(ctx, query, args...).Scan(&group.ID, &group.Name, &group.IssueCount, &item.entity, &item.ID, &item.LibraryID, &item.Title, &item.Format, &item.SourceID, &item.InventoryAvailable, &item.IssueDate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(group.ID) != 43 || group.Name == "" || group.IssueCount < 1 || group.IssueCount > 9007199254740991 || len(item.IssueDate) != 10 || !compactcatalog.ValidReadingIssueDate(item.IssueDate) {
		return nil, ErrChanged
	}
	items := []LibraryRow{*item}
	if err = readingAttributionProjectionTx(ctx, tx, items); err != nil {
		return nil, err
	}
	group.Latest = items[0]
	if group.Latest.PublicationType == nil || group.Latest.PublicationType.Value != "newspaper" || group.Latest.Series == nil || group.Latest.Series.ID != group.ID {
		return nil, ErrChanged
	}
	return &group, nil
}
