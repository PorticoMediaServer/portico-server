package reading

import (
	"context"
	"database/sql"
	"net/url"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// Metadata-only projection for rows already selected by the real library/title
// predicate in this snapshot. One bounded query, never per-item Prepare or I/O.
func readingAttributionProjectionTx(ctx context.Context, tx *sql.Tx, items []LibraryRow) error {
	ids := make([]int64, len(items))
	for i := range items {
		ids[i] = items[i].entity
	}
	values, err := compactcatalog.ReadingAcceptedValuesTx(ctx, tx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		v := values[items[i].entity]
		items[i].Author = v.Author
		items[i].PublicationType = v.PublicationType
		items[i].Series = nil
		if v.Series != nil {
			s := &SeriesValue{ID: compactcatalog.ReadingSeriesID(items[i].LibraryID, v.Series.Value), Name: v.Series.Value, Source: v.Series.Source}
			if v.SeriesNumber != nil {
				n := v.SeriesNumber.Value
				s.Number = &n
			}
			items[i].Series = s
		}
	}
	return readingArtworkProjectionTx(ctx, tx, items)
}

func (s *Service) publicationAttributions(ctx context.Context, p identity.Principal, selected Selection) (compactcatalog.ReadingValues, string, error) {
	g, err := dbwork.BeginSnapshot(ctx, s.DB)
	if err != nil {
		return compactcatalog.ReadingValues{}, "", err
	}
	defer g.Rollback()
	current, err := s.selectTx(ctx, g.Tx(), p, selected.ItemID, selected.SourceID)
	if err != nil {
		return compactcatalog.ReadingValues{}, "", err
	}
	if current.Revision != selected.Revision {
		return compactcatalog.ReadingValues{}, "", ErrChanged
	}
	values, err := compactcatalog.ReadingAcceptedValuesTx(ctx, g.Tx(), []int64{selected.Entity})
	if err != nil {
		return compactcatalog.ReadingValues{}, "", err
	}
	rows := []LibraryRow{{ID: selected.ItemID, Format: selected.Format, SourceID: selected.SourceID, entity: selected.Entity}}
	if err = readingArtworkProjectionTx(ctx, g.Tx(), rows); err != nil {
		return compactcatalog.ReadingValues{}, "", err
	}
	return values[selected.Entity], rows[0].CoverURL, nil
}

// One bounded lookup for the visible selected entities, never archive I/O or a provider fetch.
// This projection admits only image folders, so it cannot select a MOBI source
// without the Service's configured converter readiness.
const selectImageFolderCoverPublication = `SELECT ` + publicationFields + ` FROM catalog_entities e` + publicationJoins + `
 WHERE e.public_id=pid_blob(?) AND (?='' OR a.token=?) AND ` + publicationEligible + ` AND p.format='image-folder' ORDER BY a.id,o.id LIMIT 1`

func readingArtworkProjectionTx(ctx context.Context, tx *sql.Tx, items []LibraryRow) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, len(items))
	for i := range items {
		ids[i] = items[i].entity
	}
	covers, err := compactcatalog.ReadingCoverDigestsTx(ctx, tx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		if digest := covers[items[i].entity]; digest != "" {
			items[i].CoverURL = "/v1/items/" + url.PathEscape(items[i].ID) + "/art/cover?v=" + digest
		} else if items[i].Format == "image-folder" {
			var selected Selection
			err := tx.QueryRowContext(ctx, selectImageFolderCoverPublication, items[i].ID, items[i].SourceID, items[i].SourceID).Scan(selectionColumns(&selected)...)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return err
			}
			if selected.Format != "image-folder" || selected.folderRevision == "" {
				continue
			}
			items[i].CoverURL = "/v1/items/" + url.PathEscape(items[i].ID) + "/reading/cover?sourceId=" + url.QueryEscape(selected.token) + "&sourceRevision=" + selected.fence()
		}
	}
	return nil
}
