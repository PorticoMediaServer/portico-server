package reading

import (
	"context"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// DiscoverLimits bounds one Discover response. Zero values take the default.
type DiscoverLimits struct {
	Recent int
}

// Discover is the server-composed Discover payload built from facts already in
// this snapshot. This cohort publishes only the recently-added row: its order
// and membership are the catalogue's own added_ms under the List predicate.
//
// Series rows are deliberately absent. A capped scan cannot establish the
// authoritative owner/frequency label, global count or numbered-first members,
// and the approved model is the durable ready projection (Root Reading
// storage/cursor decision, 24 Oct 2026). They arrive with that projection, not
// as a sampled substitute here.
type Discover struct {
	Recent []LibraryRow
}

// Discover returns the bounded server-composed rows for one authorized library
// or for every library the viewer may read. It shares one snapshot and the same
// profile/library/restriction predicate as List.
func (s *Service) Discover(ctx context.Context, p identity.Principal, library string, limits DiscoverLimits) (Discover, error) {
	out := Discover{Recent: []LibraryRow{}}
	if s.Visible == nil {
		return out, identity.ErrForbidden
	}
	limit := limits.Recent
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
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
 JOIN reading_publications p ON p.item_id=e.id AND p.asset_id=` + primaryPublicationSQL + `
 JOIN catalog_asset_links link ON link.entity_id=e.id AND link.asset_id=p.asset_id
 JOIN catalog_assets a ON a.id=p.asset_id
 WHERE e.retired=0 AND cl.retired=0
 AND ((lib.kind='book' AND e.kind=20 AND p.format IN('epub','pdf')) OR (lib.kind='comic' AND e.kind=21 AND p.format IN('cbz','pdf')))
 AND e.added_ms IS NOT NULL AND (?='' OR cl.library_id=?) AND (` + filter + `)
 ORDER BY e.added_ms DESC,e.id DESC LIMIT ?`
	values := append([]any{library, library}, args...)
	values = append(values, limit)
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
	err = readingAttributionProjectionTx(ctx, tx, out.Recent)
	return out, err
}
