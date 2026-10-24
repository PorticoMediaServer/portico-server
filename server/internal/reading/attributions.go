package reading

import (
	"context"
	"database/sql"

	"portico.local/server/internal/compactcatalog"
)

// Metadata-only projection for rows already selected by the real library/title
// predicate in this snapshot. One bounded query, never per-item Prepare or I/O.
func readingOwnerProjectionTx(ctx context.Context, tx *sql.Tx, items []LibraryRow) error {
	ids := make([]int64, len(items))
	for i := range items {
		ids[i] = items[i].entity
	}
	values, err := compactcatalog.ReadingOwnerValuesTx(ctx, tx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		v := values[items[i].entity]
		items[i].Author = v.Author
		if v.Series != nil {
			s := &SeriesValue{ID: compactcatalog.ReadingSeriesID(items[i].LibraryID, v.Series.Value), Name: v.Series.Value, Source: v.Series.Source}
			if v.SeriesNumber != nil {
				n := v.SeriesNumber.Value
				s.Number = &n
			}
			items[i].Series = s
		}
	}
	return nil
}
