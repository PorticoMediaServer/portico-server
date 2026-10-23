package reading

import (
	"context"
	"database/sql"
	"encoding/base64"
	"strconv"
	"strings"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

type ContinueRow struct {
	Item     LibraryRow
	Progress Progress
}

type ContinuePage struct {
	Items      []ContinueRow
	NextCursor string
}

type continueCursor struct{ observed, item, asset int64 }

func (c continueCursor) encode() string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.observed, 10) + ":" + strconv.FormatInt(c.item, 10) + ":" + strconv.FormatInt(c.asset, 10)))
}

func parseContinueCursor(raw string) (continueCursor, error) {
	var out continueCursor
	if len(raw) > 128 {
		return out, ErrInvalid
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil {
		return out, ErrInvalid
	}
	parts := strings.Split(string(b), ":")
	if len(parts) != 3 {
		return out, ErrInvalid
	}
	for i, value := range []*int64{&out.observed, &out.item, &out.asset} {
		*value, err = strconv.ParseInt(parts[i], 10, 64)
		if err != nil || *value < 0 || i > 0 && *value == 0 {
			return continueCursor{}, ErrInvalid
		}
	}
	if out.encode() != raw {
		return continueCursor{}, ErrInvalid
	}
	return out, nil
}

// Continue projects the viewer's latest saved version of each unfinished
// Reading title. One current-authority snapshot filters before pagination;
// there is no session preparation, source open, provider call or AV history.
// The revision comparison is a catalogue observation, never a bytes grant.
func (s *Service) Continue(ctx context.Context, p identity.Principal, library, cursor string, limit int) (ContinuePage, error) {
	out := ContinuePage{Items: []ContinueRow{}}
	if s == nil || s.DB == nil || s.Visible == nil {
		return out, identity.ErrForbidden
	}
	if limit < 1 || limit > 100 {
		return out, ErrInvalid
	}
	var after continueCursor
	if cursor != "" {
		var err error
		if after, err = parseContinueCursor(cursor); err != nil {
			return out, err
		}
	}
	g, err := dbwork.BeginSnapshot(ctx, s.DB)
	if err != nil {
		return out, err
	}
	defer g.Rollback()
	filter, args, err := s.Visible(ctx, g.Tx(), p, "e.id")
	if err != nil {
		return out, err
	}
	if filter == "" {
		return out, identity.ErrForbidden
	}
	// Partition only after the actual owner's progress has joined eligible,
	// visible Reading publications. Multiple folder observations of one asset
	// choose the same lowest inventory-object id as selectPublication.
	query := `WITH candidates(entity,asset,format,title,language,library,token,path,size,modified,folder_source,root,generation,incarnation,root_identity,relative,object,inventory,evidence,item_id,stored_revision,stored_format,cfi,section,page_index,progression,progress_revision,observed,source_row) AS (
 SELECT ` + publicationFields + `,pid(e.public_id),rp.source_revision,rp.format,rp.cfi,rp.section,rp.page_index,rp.progression,rp.revision,rp.observed_at,
 row_number() OVER(PARTITION BY e.id,p.asset_id ORDER BY o.id)
 FROM reading_progress rp CROSS JOIN catalog_entities e ON e.id=rp.item_id` + publicationJoins + `
 WHERE rp.owner_key=? AND rp.asset_id=p.asset_id AND rp.progression<1
 AND NOT EXISTS(SELECT 1 FROM reading_progress newer WHERE newer.owner_key=rp.owner_key AND newer.item_id=rp.item_id
 AND (newer.observed_at>rp.observed_at OR (newer.observed_at=rp.observed_at AND newer.asset_id>rp.asset_id)))
 AND (?='' OR cl.library_id=?) AND ` + publicationEligible + ` AND (` + filter + `)
 AND (?=0 OR (rp.observed_at,e.id,p.asset_id)<(?,?,?)))
 SELECT entity,asset,format,title,language,library,token,path,size,modified,folder_source,root,generation,incarnation,root_identity,relative,object,inventory,evidence,item_id,stored_revision,stored_format,cfi,section,page_index,progression,progress_revision,observed
 FROM candidates WHERE source_row=1 ORDER BY observed DESC,entity DESC,asset DESC LIMIT ?`
	values := append([]any{identity.PersonalKey(p.Viewer), library, library}, args...)
	values = append(values, boolInt(cursor != ""), after.observed, after.item, after.asset, limit+1)
	rows, err := g.Tx().QueryContext(ctx, query, values...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	var last continueCursor
	for rows.Next() {
		var selected Selection
		var progress Progress
		var cfi, section sql.NullString
		var page sql.NullInt64
		columns := append(selectionColumns(&selected), &selected.ItemID, &progress.SourceRevision, &progress.Location.Format, &cfi, &section, &page, &progress.Location.Progression, &progress.Revision, &progress.ObservedAt)
		if err = rows.Scan(columns...); err != nil {
			return out, err
		}
		if len(out.Items) == limit {
			out.NextCursor = last.encode()
			break
		}
		progress.SourceID = selected.token
		progress.Location.CFI, progress.Location.Section = cfi.String, section.String
		if page.Valid {
			progress.Location.PageIndex = &page.Int64
		}
		progress.Stale = progress.SourceRevision != selected.fence() || progress.Location.Format != selected.Format
		out.Items = append(out.Items, ContinueRow{Item: LibraryRow{ID: selected.ItemID, LibraryID: selected.library, Title: selected.Title, Format: selected.Format, SourceID: selected.token, InventoryAvailable: true}, Progress: progress})
		last = continueCursor{progress.ObservedAt, selected.Entity, selected.Asset}
	}
	return out, rows.Err()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
