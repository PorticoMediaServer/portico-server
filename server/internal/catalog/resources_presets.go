package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/requestinput"
)

// SavedViewPresets returns the existing saved views for an authorized library,
// optionally narrowed to its data pivot. Ordinary last-used choices remain
// device-local. A page carries complete definitions and resource revisions.
func (s *Service) SavedViewPresets(viewer Viewer, server, fence string, a ResourceActor, library, pivot, cursor string, pinned bool, limit int) (SavedResourcePage, error) {
	if dbwork.Snapshot(s.Context()) == nil {
		var out SavedResourcePage
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var err error
			out, err = s.WithContext(ctx).SavedViewPresets(viewer, server, fence, a, library, pivot, cursor, pinned, limit)
			return err
		})
		return out, err
	}
	out := SavedResourcePage{ServerID: server, ViewerFence: fence, Resources: []SavedResource{}}
	if library == "" || !viewer.AllowsLibrary(library) {
		return out, sql.ErrNoRows
	}
	var kind string
	if err := s.read().QueryRow(`SELECT kind FROM libraries WHERE id=?`, library).Scan(&kind); err != nil {
		return out, err
	}
	if pivot != "" {
		p, known := pivotForKind(kind, pivot)
		if !known || !p.Browsable {
			return out, requestinput.New("saved view preset pivot is not browsable")
		}
	}
	if limit < 1 || limit > 100 {
		return out, ErrCursor
	}
	var arrangementRevision int64
	if err := s.read().QueryRow(`SELECT revision, COALESCE((SELECT revision FROM saved_pin_order_revisions WHERE owner_key=?),0) FROM saved_resource_generation WHERE id=1`, actorKey(a)).Scan(&out.Revision, &arrangementRevision); err != nil {
		return out, err
	}
	scope := cursorScope{Profile: actorKey(a), View: "saved_view", Viewer: fence, Limit: limit, Library: library, Entity: pivot}
	if pinned {
		scope.Category = "pinned"
	}
	revision := ContentRevision{Catalog: out.Revision}
	if pinned {
		revision.Viewer = arrangementRevision
	}
	var after *cursorValue
	if cursor != "" {
		v, err := s.decodeRevisionCursor(cursor, scope, revision)
		if err != nil {
			return out, err
		}
		if v.CatalogRevision != revision.Catalog || v.ViewerRevision != revision.Viewer {
			return out, ErrStaleContinuation
		}
		after = &v
	}
	query, args := savedViewPresetSQL(a, library, pivot, pinned, limit, after)
	rows, err := s.read().Query(query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		r := SavedResource{ServerID: server, ViewerFence: fence, Kind: "view", Actions: []string{"pin"}, Status: "ready", InvalidComponents: []string{}}
		var definition, key string
		if err = rows.Scan(&r.ID, &r.Name, &r.Summary, &r.Visibility, &r.Revision, &r.Role, &definition, &r.Pinned, &r.PinRevision, &key); err != nil {
			return out, err
		}
		if r.Role == "owner" {
			r.Actions = append(r.Actions, "update", "delete")
		}
		var v SavedViewDefinition
		if json.Unmarshal([]byte(definition), &v) != nil {
			r.InvalidComponents = []string{"definition"}
		} else {
			r.Definition = &v
			r.InvalidComponents = validateSavedViewKind(kind, v)
		}
		if len(r.InvalidComponents) > 0 {
			r.Status = "needs-review"
		}
		out.Resources = append(out.Resources, r)
		keys = append(keys, key)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Resources) > limit {
		out.Resources = out.Resources[:limit]
		out.NextCursor, err = s.encodeRevisionCursor(cursorValue{Scope: scope, Value: keys[limit-1], ID: out.Resources[limit-1].ID, Expires: time.Now().Add(30 * time.Minute).Unix()}, revision)
	}
	return out, err
}

// Selection is an indexed seek over derived access/sort keys, limited before
// reading definitions. Hydration rechecks canonical ownership/grants and scope;
// neither the index nor its stored pin flag is an authorization authority.
func savedViewPresetSQL(a ResourceActor, library, pivot string, pinned bool, limit int, after *cursorValue) (string, []any) {
	index, key := "saved_view_picker_library", "name_key"
	if pivot != "" {
		index = "saved_view_picker_pivot"
	}
	if pinned {
		index, key = "saved_view_picker_pinned_library", "pin_key"
		if pivot != "" {
			index = "saved_view_picker_pinned_pivot"
		}
	}
	query := `WITH selected AS MATERIALIZED (SELECT resource_id,` + key + ` AS sort_key FROM saved_view_picker INDEXED BY ` + index + ` WHERE owner_key=? AND library_id=?`
	args := []any{actorKey(a), library}
	if pivot != "" {
		query += ` AND pivot=?`
		args = append(args, pivot)
	}
	if pinned {
		query += ` AND pinned=1`
	}
	if after != nil {
		query += ` AND (` + key + `,resource_id)>(?,?)`
		args = append(args, after.Value, after.ID)
	}
	query += ` ORDER BY ` + key + `,resource_id LIMIT ?) SELECT r.id,r.name,r.summary,r.visibility,r.revision,
 CASE WHEN r.owner_key=? THEN 'owner' ELSE sh.role END,r.definition,COALESCE(p.pinned,0),COALESCE(p.revision,0),selected.sort_key
 FROM selected CROSS JOIN saved_resources r ON r.id=selected.resource_id
 LEFT JOIN saved_resource_shares sh ON sh.resource_id=r.id AND sh.authority=? AND sh.account_id=? AND sh.profile_id=?
 LEFT JOIN saved_pins p ON p.owner_key=? AND p.kind='view' AND p.resource_id=r.id
 WHERE r.kind='view' AND r.deleted=0 AND (r.owner_key=? OR sh.role IS NOT NULL)
 AND CASE WHEN json_valid(r.definition) THEN json_extract(r.definition,'$.libraryId') END=?`
	args = append(args, limit+1, actorKey(a), a.Authority, a.AccountID, a.ProfileID, actorKey(a), actorKey(a), library)
	if pivot != "" {
		query += ` AND CASE WHEN json_valid(r.definition) THEN json_extract(r.definition,'$.pivot') END=?`
		args = append(args, pivot)
	}
	if pinned {
		query += ` AND p.pinned=1`
	}
	query += ` ORDER BY selected.sort_key,r.id`
	return query, args
}
