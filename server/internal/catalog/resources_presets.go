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
	if err := s.read().QueryRow(`SELECT revision FROM saved_resource_generation WHERE id=1`).Scan(&out.Revision); err != nil {
		return out, err
	}
	scope := cursorScope{Profile: actorKey(a), View: "saved_view", Viewer: fence, Limit: limit, Library: library, Entity: pivot}
	if pinned {
		scope.Category = "pinned"
	}
	revision := ContentRevision{Catalog: out.Revision}
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

// Owner rows seek through the library/pivot expression indexes. Explicitly
// shared legacy views start from the actor's existing share index. Keeping the
// branches separate avoids an owner/share OR scanning unrelated definitions.
// All selected metadata, roles and pins are hydrated in this one query.
func savedViewPresetSQL(a ResourceActor, library, pivot string, pinned bool, limit int, after *cursorValue) (string, []any) {
	scope := `r.kind='view' AND r.deleted=0 AND CASE WHEN json_valid(r.definition) THEN json_extract(r.definition,'$.libraryId') END=?`
	bound := []any{library}
	if pivot != "" {
		scope += ` AND CASE WHEN json_valid(r.definition) THEN json_extract(r.definition,'$.pivot') END=?`
		bound = append(bound, pivot)
	}
	index := "saved_view_library_page"
	if pivot != "" {
		index = "saved_view_pivot_page"
	}
	query := `WITH candidates AS (
 SELECT r.id,r.name,r.summary,r.visibility,r.revision,r.definition,'owner' AS role FROM saved_resources r INDEXED BY ` + index + ` WHERE r.owner_key=? AND ` + scope + `
 UNION ALL
 SELECT r.id,r.name,r.summary,r.visibility,r.revision,r.definition,sh.role FROM saved_resource_shares sh JOIN saved_resources r ON r.id=sh.resource_id WHERE sh.authority=? AND sh.account_id=? AND sh.profile_id=? AND r.owner_key<>? AND ` + scope + `
 ), hydrated AS (SELECT r.*,COALESCE(p.pinned,0) AS pinned,COALESCE(p.revision,0) AS pin_revision,`
	args := append([]any{actorKey(a)}, bound...)
	args = append(args, a.Authority, a.AccountID, a.ProfileID, actorKey(a))
	args = append(args, bound...)
	if pinned {
		query += `printf('%08d',COALESCE(o.position,99999999))||char(31)||lower(r.name)`
	} else {
		query += `lower(r.name)`
	}
	query += ` AS sort_key FROM candidates r LEFT JOIN saved_pins p ON p.owner_key=? AND p.kind='view' AND p.resource_id=r.id`
	args = append(args, actorKey(a))
	if pinned {
		query += ` LEFT JOIN saved_pin_order o ON o.owner_key=? AND o.kind='view' AND o.resource_id=r.id WHERE p.pinned=1`
		args = append(args, actorKey(a))
	}
	query += `) SELECT id,name,summary,visibility,revision,role,definition,pinned,pin_revision,sort_key FROM hydrated`
	if after != nil {
		query += ` WHERE (sort_key>? OR (sort_key=? AND id>?))`
		args = append(args, after.Value, after.Value, after.ID)
	}
	query += ` ORDER BY sort_key,id LIMIT ?`
	args = append(args, limit+1)
	return query, args
}
