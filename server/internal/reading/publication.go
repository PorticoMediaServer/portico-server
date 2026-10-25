package reading

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

type Publication struct {
	Conversion *MobiProvenance
	Sequential bool
	Selection  Selection
	Metadata   Metadata
	// EmbeddedAuthor is the package's own single creator, when it carries
	// exactly one exact value. It is evidence for adoption only; it never
	// substitutes for the catalogue's accepted author.
	EmbeddedAuthor  string
	Resources       []Resource
	Spine           []string
	PageNames       []string
	Contents        []ContentsEntry
	PublicationType *compactcatalog.ReadingValue
}

// Inspect returns only actual, bounded package/archival facts. PDF page count
// and outline belong to the real client PDF parser, never a server estimate.
func Inspect(f *Source) (Publication, error) {
	out := Publication{Sequential: f.Format == "pdf" && f.Sequential(), Selection: f.PublicationSelection(), Conversion: f.ConversionProvenance(), Metadata: Metadata{Title: f.Title, Language: f.Language}}
	switch out.Selection.Format {
	case "epub":
		e, err := f.epub()
		if err != nil {
			return out, err
		}
		out.Metadata.Direction = e.Metadata.Direction
		out.Metadata.ExplicitPublicationType = e.Metadata.ExplicitPublicationType
		out.Metadata.PublicationTypeField, out.Metadata.PublicationTypeRaw, out.Metadata.PublicationTypeComplete = e.Metadata.PublicationTypeField, e.Metadata.PublicationTypeRaw, e.Metadata.PublicationTypeComplete
		out.EmbeddedAuthor = acceptedEmbeddedAuthor(e.Metadata.Author)
		out.Resources = e.Resources
		out.Spine = e.Spine
		out.Contents, err = e.Contents(f.ctx)
		if err != nil {
			return out, err
		}
	case "cbz":
		c, err := f.comic()
		if err != nil {
			return out, err
		}
		out.Metadata.Direction = c.Metadata.Direction
		out.Metadata.ExplicitPublicationType = c.Metadata.ExplicitPublicationType
		out.Metadata.PublicationTypeField, out.Metadata.PublicationTypeRaw, out.Metadata.PublicationTypeComplete = c.Metadata.PublicationTypeField, c.Metadata.PublicationTypeRaw, c.Metadata.PublicationTypeComplete
		out.EmbeddedAuthor = acceptedEmbeddedAuthor(c.Metadata.Author)
		out.PageNames = c.Names
	case "pdf":
		if err := f.CheckPDF(); err != nil {
			return out, err
		}
	default:
		return out, ErrInvalid
	}
	if err := f.Validate(); err != nil {
		return out, err
	}
	return out, nil
}

// acceptedEmbeddedAuthor keeps only an exact, storable creator value. A value
// the accepted-value reader would reject would make the whole title's accepted
// facts unreadable, so such a value publishes nothing rather than a repaired
// name. The file's own string is otherwise kept exactly, never split.
func acceptedEmbeddedAuthor(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") {
		return ""
	}
	return value
}

// adoptEmbeddedAuthor publishes the publication's own single creator as
// accepted file-metadata evidence. It is called only from the Prepare that
// opened this exact source. The writer rechecks the same current inventory
// observation and same-source association in its own transaction before it
// writes, and an unchanged observation writes nothing. Owner overrides keep
// their existing precedence over this row. The normalized accepted fields carry
// no title, so the embedded title is deliberately not published here; the
// catalogue title remains the path rule's title.
func (s *Service) adoptEmbeddedAuthor(ctx context.Context, p identity.Principal, selected Selection, author string) error {
	if author == "" || (selected.Format != "epub" && selected.Format != "cbz") {
		return nil
	}
	if s == nil || s.DB == nil {
		return ErrUnavailable
	}
	gate, err := dbwork.Begin(ctx, s.DB, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	defer gate.Rollback()
	tx := gate.Tx()
	current, err := s.selectTx(ctx, tx, p, selected.ItemID, selected.SourceID)
	if err != nil {
		return err
	}
	// The opened publication is the evidence. A different current revision is a
	// different file, so its package facts are never attributed to this one.
	if current.Revision != selected.Revision || current.object != selected.object || current.inventory != selected.inventory || current.inventoryRow != selected.inventoryRow {
		return ErrChanged
	}
	var text, revision, object string
	err = tx.QueryRowContext(ctx, `SELECT text_value,source_revision,inventory_object_id FROM reading_source_values WHERE item_id=? AND asset_id=? AND field='author' AND source='file-metadata'`, current.Entity, current.Asset).Scan(&text, &revision, &object)
	if err == nil && text == author && revision == current.inventory && object == current.inventoryRow {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM reading_source_values WHERE item_id=? AND asset_id=? AND field='author' AND source='file-metadata'`, current.Entity, current.Asset); err != nil {
		return err
	}
	// Recheck the exact observation at publication, as the path publisher does.
	// The insert selects only a current object of this same source and library.
	result, err := tx.ExecContext(ctx, `INSERT INTO reading_source_values(item_id,asset_id,field,text_value,number_value,source,inventory_object_id,source_revision)
 SELECT ?1,?2,'author',?3,NULL,'file-metadata',o.id,o.revision FROM inventory_objects o
 JOIN catalog_assets a ON a.token=o.asset_id AND a.id=?2
 JOIN reading_publications p ON p.item_id=?1 AND p.asset_id=a.id
 JOIN catalog_asset_links link ON link.entity_id=p.item_id AND link.asset_id=p.asset_id
 JOIN catalog_entities e ON e.id=p.item_id JOIN catalog_libraries cl ON cl.id=e.library_id
 JOIN library_sources s ON s.id=o.source_id AND s.library_id=cl.library_id
 WHERE o.id=?4 AND o.revision=?5 AND o.root_incarnation=s.incarnation AND o.retired=0`, current.Entity, current.Asset, author, current.inventoryRow, current.inventory)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrChanged
	}
	automatic, err := compactcatalog.ReadingAutomaticFieldsTx(ctx, tx, current.Entity)
	if err != nil {
		return err
	}
	value := ""
	if automatic.Author != nil {
		value = automatic.Author.Value
	}
	if _, err = tx.ExecContext(ctx, `UPDATE metadata_owner_fields SET automatic_value=? WHERE kind='item' AND entity_id=? AND field='author' AND automatic_value<>?`, value, current.Entity, value); err != nil {
		return err
	}
	return gate.Commit()
}

type LibraryRow struct {
	ID, LibraryID, Title, Format, SourceID string
	IssueDate                              string
	CoverURL                               string
	InventoryAvailable                     bool
	// Catalogue facts, distinct from package identity and read availability.
	Author          *compactcatalog.ReadingValue
	PublicationType *compactcatalog.ReadingValue
	Series          *SeriesValue
	entity          int64
}

type SeriesValue struct {
	ID, Name string
	Number   *float64
	Source   compactcatalog.ReadingValueSource
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
 JOIN reading_publications p ON p.item_id=e.id AND p.asset_id=` + s.primaryPublicationSQL() + `
 JOIN catalog_asset_links link ON link.entity_id=e.id AND link.asset_id=p.asset_id
 JOIN catalog_assets a ON a.id=p.asset_id
 WHERE e.retired=0 AND cl.retired=0
 AND ((lib.kind='book' AND e.kind=20 AND p.format IN(` + s.bookFormatsSQL() + `)) OR (lib.kind='comic' AND e.kind=21 AND p.format IN('cbz','pdf')))
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
		out.Items[len(out.Items)-1].entity = id
		last = id
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if err = rows.Close(); err != nil {
		return out, err
	}
	err = readingAttributionProjectionTx(ctx, tx, out.Items)
	return out, err
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
	if err = tx.QueryRowContext(ctx, `SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id JOIN libraries lib ON lib.id=cl.library_id WHERE e.public_id=pid_blob(?) AND e.retired=0 AND cl.retired=0 AND ((lib.kind='book' AND e.kind=20) OR (lib.kind='comic' AND e.kind=21))`, item).Scan(&library); err != nil {
		return nil, ErrUnavailable
	}
	if s.Authorize == nil {
		return nil, identity.ErrForbidden
	}
	if err = s.Authorize(ctx, tx, p, library, item); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.token,p.format,a.size,EXISTS(SELECT 1 FROM inventory_item_availability available WHERE available.item_id=p.item_id AND available.asset_id=a.token AND available.available=1) FROM reading_publications p JOIN catalog_entities e ON e.id=p.item_id JOIN catalog_asset_links link ON link.entity_id=e.id AND link.asset_id=p.asset_id JOIN catalog_assets a ON a.id=link.asset_id WHERE e.public_id=pid_blob(?) AND p.format IN(`+s.sourceFormatsSQL()+`) ORDER BY a.id LIMIT 129`, item)
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
