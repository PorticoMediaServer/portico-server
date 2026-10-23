package reading

import (
	"context"
	"fmt"
	"strconv"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

type Publication struct {
	Selection Selection
	Metadata  Metadata
	Resources []Resource
	Spine     []string
	PageNames []string
}

// Inspect returns only actual, bounded package/archival facts. PDF page count
// and outline belong to the real client PDF parser, never a server estimate.
func Inspect(f *Source) (Publication, error) {
	out := Publication{Selection: f.Selection, Metadata: Metadata{Title: f.Title, Language: f.Language}}
	switch f.Format {
	case "epub":
		e, err := OpenEPUB(f.ctx, f, f.Size)
		if err != nil {
			return out, err
		}
		out.Metadata.Direction = e.Metadata.Direction
		out.Resources = e.Resources
		out.Spine = e.Spine
	case "cbz":
		c, err := OpenComic(f.ctx, f, f.Size)
		if err != nil {
			return out, err
		}
		out.Metadata.Direction = c.Metadata.Direction
		out.PageNames = c.Names
	case "pdf":
		var header [5]byte
		if _, err := f.ReadAt(header[:], 0); err != nil {
			return out, err
		}
		if string(header[:]) != "%PDF-" {
			return out, ErrInvalid
		}
	default:
		return out, ErrInvalid
	}
	if err := f.Validate(); err != nil {
		return out, err
	}
	return out, nil
}

type LibraryRow struct {
	ID, LibraryID, Title, Format, SourceID string
	InventoryAvailable                     bool
}
type LibraryPage struct {
	Items      []LibraryRow
	NextCursor string
}

// List applies the same profile/member/library/title predicate as an item
// acquisition, before LIMIT. It reads one bounded page and no artwork/hidden
// title data, and does not infer that inventory availability proves decoding.
func (s *Service) List(ctx context.Context, p identity.Principal, library, cursor string, limit int) (LibraryPage, error) {
	out := LibraryPage{Items: []LibraryRow{}}
	if s.Visible == nil {
		return out, identity.ErrForbidden
	}
	if limit < 1 || limit > 100 {
		return out, ErrInvalid
	}
	after := int64(0)
	if cursor != "" {
		var err error
		after, err = strconv.ParseInt(cursor, 10, 64)
		if err != nil || after < 0 {
			return out, ErrInvalid
		}
	}
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
	query := `SELECT e.id,pid(e.public_id),cl.library_id,e.title,p.format,a.token,
 EXISTS(SELECT 1 FROM inventory_item_availability available WHERE available.item_id=e.id AND available.asset_id=a.token AND available.available=1)
 FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id JOIN libraries lib ON lib.id=cl.library_id
 JOIN reading_publications p ON p.item_id=e.id AND p.asset_id=(SELECT min(candidate.asset_id) FROM reading_publications candidate WHERE candidate.item_id=e.id)
 JOIN catalog_assets a ON a.id=p.asset_id
 WHERE e.kind IN(20,21) AND e.retired=0 AND cl.retired=0 AND lib.kind IN('book','comic')
 AND e.id>? AND (?='' OR cl.library_id=?) AND (` + filter + `) ORDER BY e.id LIMIT ?`
	values := append([]any{after, library, library}, args...)
	values = append(values, limit+1)
	rows, err := tx.QueryContext(ctx, query, values...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		var id int64
		var row LibraryRow
		if err = rows.Scan(&id, &row.ID, &row.LibraryID, &row.Title, &row.Format, &row.SourceID, &row.InventoryAvailable); err != nil {
			return out, err
		}
		if len(out.Items) == limit {
			out.NextCursor = fmt.Sprint(last)
			break
		}
		out.Items = append(out.Items, row)
		last = id
	}
	return out, rows.Err()
}

type Version struct {
	SourceID, Format   string
	Size               int64
	InventoryAvailable bool
}

func (s *Service) Versions(ctx context.Context, p identity.Principal, item string) ([]Version, error) {
	g, err := dbwork.BeginSnapshot(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	defer g.Rollback()
	tx := g.Tx()
	var library string
	if err = tx.QueryRowContext(ctx, `SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id JOIN libraries lib ON lib.id=cl.library_id WHERE e.public_id=pid_blob(?) AND e.kind IN(20,21) AND e.retired=0 AND cl.retired=0 AND lib.kind IN('book','comic')`, item).Scan(&library); err != nil {
		return nil, ErrUnavailable
	}
	if s.Authorize == nil {
		return nil, identity.ErrForbidden
	}
	if err = s.Authorize(ctx, tx, p, library, item); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.token,p.format,a.size,EXISTS(SELECT 1 FROM inventory_item_availability available WHERE available.item_id=p.item_id AND available.asset_id=a.token AND available.available=1) FROM reading_publications p JOIN catalog_entities e ON e.id=p.item_id JOIN catalog_assets a ON a.id=p.asset_id WHERE e.public_id=pid_blob(?) ORDER BY a.id LIMIT 129`, item)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		var v Version
		if err = rows.Scan(&v.SourceID, &v.Format, &v.Size, &v.InventoryAvailable); err != nil {
			return nil, err
		}
		if len(out) == 128 {
			return nil, ErrLimit
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
