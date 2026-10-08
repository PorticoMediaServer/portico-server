package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"strings"
	"time"
	"unicode/utf8"
)

// These are personal resources. The existing library collections remain catalog
// groupings and are intentionally not repurposed as a viewer-owned store.
// A saved view stores the whole browse query: the pivot it was written against,
// the expression tree and the sort. It is validated against the same
// capabilities the browse engine publishes, so a saved view can never name a
// field or sort the engine cannot execute.
type SavedViewDefinition struct {
	LibraryID    string                `json:"libraryId"`
	Pivot        string                `json:"pivot"`
	Query        *BrowseNode           `json:"query"`
	Sort         []BrowseSortSelection `json:"sort"`
	Presentation string                `json:"presentation"`
}
type SavedResource struct {
	ServerID          string               `json:"serverId"`
	ViewerFence       string               `json:"viewerFence"`
	ID                string               `json:"id"`
	Kind              string               `json:"kind"`
	Name              string               `json:"name"`
	Summary           string               `json:"summary"`
	Visibility        string               `json:"visibility"`
	Revision          int64                `json:"revision"`
	Role              string               `json:"role"`
	EntryCount        int                  `json:"entryCount"`
	Actions           []string             `json:"actions"`
	Definition        *SavedViewDefinition `json:"definition,omitempty"`
	Status            string               `json:"status"`
	InvalidComponents []string             `json:"invalidComponents"`
	Pinned            bool                 `json:"pinned"`
	PinRevision       int64                `json:"pinRevision"`
	Shares            []PlaylistShare      `json:"shares,omitempty"`
}
type SavedResourcePage struct {
	ServerID    string          `json:"serverId"`
	ViewerFence string          `json:"viewerFence"`
	Revision    int64           `json:"revision"`
	Resources   []SavedResource `json:"resources"`
	NextCursor  string          `json:"nextCursor"`
}
type SavedResourceEntry struct {
	ID     string        `json:"id"`
	Hidden bool          `json:"hidden"`
	Media  *ContentEntry `json:"media,omitempty"`
}
type SavedResourceContent struct {
	Resource   SavedResource        `json:"resource"`
	Entries    []SavedResourceEntry `json:"entries"`
	NextCursor string               `json:"nextCursor"`
	Browse     *BrowseResult        `json:"browse,omitempty"`
}
type SavedResourceMutation struct {
	OperationID      string               `json:"operationId"`
	ExpectedRevision int64                `json:"expectedRevision"`
	Kind             string               `json:"kind,omitempty"`
	Name             *string              `json:"name,omitempty"`
	Summary          *string              `json:"summary,omitempty"`
	Visibility       *string              `json:"visibility,omitempty"`
	Definition       *SavedViewDefinition `json:"definition,omitempty"`
	AddItemIDs       []string             `json:"addItemIds,omitempty"`
	RemoveEntryIDs   []string             `json:"removeEntryIds,omitempty"`
	Authority        string               `json:"authority,omitempty"`
	AccountID        string               `json:"accountId,omitempty"`
	ProfileID        string               `json:"profileId,omitempty"`
	Role             string               `json:"role,omitempty"`
}
type SavedResourceReceipt struct {
	OperationID string         `json:"operationId"`
	ResourceID  string         `json:"resourceId"`
	Revision    int64          `json:"revision"`
	Deleted     bool           `json:"deleted"`
	Entries     *EntryOutcomes `json:"entries,omitempty"`
}

func actorKey(a ResourceActor) string {
	return identity.PersonalKey(identity.Viewer{Authority: a.Authority, AccountID: a.AccountID, ProfileID: a.ProfileID})
}
func savedResourceRole(q personalReader, id string, a ResourceActor) (string, string, int64, bool, error) {
	var owner, kind, visibility string
	var rev int64
	var deleted bool
	e := q.QueryRow(`SELECT owner_key,kind,revision,deleted,visibility FROM saved_resources WHERE id=?`, id).Scan(&owner, &kind, &rev, &deleted, &visibility)
	if e != nil {
		return "", "", 0, false, e
	}
	if owner == actorKey(a) {
		return "owner", kind, rev, deleted, nil
	}
	var role string
	e = q.QueryRow(`SELECT role FROM saved_resource_shares WHERE resource_id=? AND authority=? AND account_id=? AND profile_id=?`, id, a.Authority, a.AccountID, a.ProfileID).Scan(&role)
	if e == nil {
		return role, kind, rev, deleted, nil
	}
	// A server-visible collection is readable by every member. Editing it still
	// needs an explicit share; visibility publishes, it does not delegate.
	if visibility == "server" && kind == "collection" && !deleted {
		return "viewer", kind, rev, deleted, nil
	}
	return "", "", 0, false, sql.ErrNoRows
}
func validationPath(e error, fallback string) string {
	var issue *BrowseValidationError
	if errors.As(e, &issue) && issue.Path != "" {
		return issue.Path
	}
	return fallback
}

// validateSavedView reports the definition paths that fail, so a client can
// attach each failure to the control that produced it.
func validateSavedView(q personalReader, v SavedViewDefinition) []string {
	bad := []string{}
	var kind string
	if e := q.QueryRow(`SELECT kind FROM libraries WHERE id=?`, v.LibraryID).Scan(&kind); e != nil {
		return append(bad, "libraryId")
	}
	pivot, known := pivotForKind(kind, v.Pivot)
	if !known || !pivot.Browsable {
		bad = append(bad, "pivot")
	}
	if v.Presentation != "grid" && v.Presentation != "list" {
		bad = append(bad, "presentation")
	}
	if known && pivot.Browsable {
		if len(v.Sort) == 0 {
			bad = append(bad, "sort")
		} else if _, e := resolveBrowseSorts(pivot, v.Sort); e != nil {
			bad = append(bad, validationPath(e, "sort"))
		}
	}
	raw, e := json.Marshal(v.Query)
	if e != nil {
		bad = append(bad, "query")
	} else if _, e = ParseBrowseQuery(raw, "query"); e != nil {
		bad = append(bad, validationPath(e, "query"))
	}
	return bad
}
func (s *Service) SavedResource(server, fence, id string, a ResourceActor) (SavedResource, error) {
	out := SavedResource{ServerID: server, ViewerFence: fence, ID: id, Actions: []string{"pin"}, Status: "ready", InvalidComponents: []string{}}
	role, kind, rev, deleted, e := savedResourceRole(s.read(), id, a)
	if e != nil {
		return out, e
	}
	if deleted {
		return out, sql.ErrNoRows
	}
	out.Role = role
	out.Kind = kind
	out.Revision = rev
	var definition string
	if e = s.read().QueryRow(`SELECT name,summary,visibility,definition,(SELECT count(*) FROM saved_resource_entries WHERE resource_id=saved_resources.id) FROM saved_resources WHERE id=?`, id).Scan(&out.Name, &out.Summary, &out.Visibility, &definition, &out.EntryCount); e != nil {
		return out, e
	}
	if e = s.read().QueryRow(`SELECT COALESCE((SELECT pinned FROM saved_pins WHERE owner_key=? AND kind=? AND resource_id=?),0),COALESCE((SELECT revision FROM saved_pins WHERE owner_key=? AND kind=? AND resource_id=?),0)`, actorKey(a), kind, id, actorKey(a), kind, id).Scan(&out.Pinned, &out.PinRevision); e != nil {
		return out, e
	}
	if kind == "view" {
		var v SavedViewDefinition
		if e = json.Unmarshal([]byte(definition), &v); e != nil {
			out.Status = "needs-review"
			out.InvalidComponents = []string{"definition"}
		} else {
			out.Definition = &v
			out.InvalidComponents = validateSavedView(s.read(), v)
			if len(out.InvalidComponents) > 0 {
				out.Status = "needs-review"
			}
		}
	}
	if role == "owner" {
		out.Actions = append(out.Actions, "update", "delete")
		if kind == "collection" {
			out.Actions = append(out.Actions, "entries", "share")
		}
	}
	if role == "editor" && kind == "collection" {
		out.Actions = append(out.Actions, "entries")
	}
	if role == "owner" && kind == "collection" {
		out.Shares = []PlaylistShare{}
		rows, e := s.read().Query(`SELECT authority,account_id,profile_id,role FROM saved_resource_shares WHERE resource_id=? ORDER BY authority,account_id,profile_id`, id)
		if e != nil {
			return out, e
		}
		for rows.Next() {
			var share PlaylistShare
			if e = rows.Scan(&share.Authority, &share.AccountID, &share.ProfileID, &share.Role); e != nil {
				rows.Close()
				return out, e
			}
			share.DisplayName = profileLabel(share.ProfileID)
			out.Shares = append(out.Shares, share)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return out, e
		}
	}
	_, _, after, dead, e := savedResourceRole(s.read(), id, a)
	if e != nil {
		return out, e
	}
	if after != rev || dead {
		return out, ErrStaleContinuation
	}
	return out, nil
}
func (s *Service) SavedResources(server, fence string, a ResourceActor, kind, cursor string, pinned bool, limit int) (SavedResourcePage, error) {
	out := SavedResourcePage{ServerID: server, ViewerFence: fence, Resources: []SavedResource{}}
	if (kind != "collection" && kind != "view") || limit < 1 || limit > 100 {
		return out, ErrCursor
	}
	if e := s.read().QueryRow(`SELECT revision FROM saved_resource_generation WHERE id=1`).Scan(&out.Revision); e != nil {
		return out, e
	}
	scope := cursorScope{Profile: actorKey(a), View: "saved_" + kind, Viewer: fence, Limit: limit}
	if pinned {
		scope.Category = "pinned"
	}
	// A pinned listing is returned in the viewer's own arrangement; anything
	// pinned but never arranged sorts after the arranged entries, by name.
	sortKey := `lower(r.name)`
	selectArgs := []any{}
	if pinned {
		sortKey = `printf('%08d',COALESCE((SELECT o.position FROM saved_pin_order o WHERE o.owner_key=? AND o.kind=r.kind AND o.resource_id=r.id),99999999))||char(31)||lower(r.name)`
		selectArgs = append(selectArgs, actorKey(a))
	}
	// Server-visible collections are part of every member's collection list.
	where := ` FROM saved_resources r WHERE r.deleted=0 AND r.kind=? AND (r.owner_key=? OR EXISTS(SELECT 1 FROM saved_resource_shares s WHERE s.resource_id=r.id AND s.authority=? AND s.account_id=? AND s.profile_id=?) OR (r.kind='collection' AND r.visibility='server'))`
	args := append(selectArgs, kind, actorKey(a), a.Authority, a.AccountID, a.ProfileID)
	if pinned {
		where += ` AND EXISTS(SELECT 1 FROM saved_pins p WHERE p.owner_key=? AND p.kind=r.kind AND p.resource_id=r.id AND p.pinned=1)`
		args = append(args, actorKey(a))
	}
	rev := ContentRevision{Catalog: out.Revision}
	outer := ``
	if cursor != "" {
		c, e := s.decodeRevisionCursor(cursor, scope, rev)
		if e != nil {
			return out, e
		}
		outer = ` WHERE (sort_key>? OR (sort_key=? AND id>?))`
		args = append(args, c.Value, c.Value, c.ID)
	}
	args = append(args, limit+1)
	rows, e := s.read().Query(`SELECT id,name,sort_key FROM (SELECT r.id AS id,r.name AS name,`+sortKey+` AS sort_key`+where+`)`+outer+` ORDER BY sort_key,id LIMIT ?`, args...)
	if e != nil {
		return out, e
	}
	ids, keys := []string{}, []string{}
	for rows.Next() {
		var id, name, key string
		if e = rows.Scan(&id, &name, &key); e != nil {
			rows.Close()
			return out, e
		}
		ids = append(ids, id)
		keys = append(keys, key)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(ids) > limit {
		ids = ids[:limit]
		out.NextCursor, e = s.encodeRevisionCursor(cursorValue{Scope: scope, Value: keys[limit-1], ID: ids[limit-1], Expires: time.Now().Add(30 * time.Minute).Unix()}, rev)
		if e != nil {
			return out, e
		}
	}
	for _, id := range ids {
		r, e := s.SavedResource(server, fence, id, a)
		if e != nil {
			return out, e
		}
		out.Resources = append(out.Resources, r)
	}
	var after int64
	if e = s.read().QueryRow(`SELECT revision FROM saved_resource_generation WHERE id=1`).Scan(&after); e != nil {
		return out, e
	}
	if after != out.Revision {
		return out, ErrStaleContinuation
	}
	return out, nil
}
func (s *Service) SavedResourceContent(viewer Viewer, server, fence, id string, a ResourceActor, cursor string, limit int) (SavedResourceContent, error) {
	if dbwork.Snapshot(s.Context()) == nil {
		var out SavedResourceContent
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var readErr error
			out, readErr = s.WithContext(ctx).SavedResourceContent(viewer, server, fence, id, a, cursor, limit)
			return readErr
		})
		return out, err
	}
	libraries := viewer.Libraries
	if err := s.prepareViewer(viewer); err != nil {
		return SavedResourceContent{}, err
	}
	out := SavedResourceContent{Entries: []SavedResourceEntry{}}
	if limit < 1 || limit > 100 {
		return out, ErrCursor
	}
	r, e := s.SavedResource(server, fence, id, a)
	if e != nil {
		return out, e
	}
	out.Resource = r
	if r.Kind == "view" {
		if r.Status == "needs-review" {
			return out, nil
		}
		v := r.Definition
		allowed := false
		for _, library := range libraries {
			if library == v.LibraryID {
				allowed = true
			}
		}
		if !allowed {
			return out, identity.ErrUnauthorized
		}
		// The saved query runs through the same engine the browse routes use; a
		// saved view is a stored request, never a second query implementation.
		viewScope := viewer
		viewScope.Fence = fence + ":" + id + ":" + operationHash(r.Revision)
		result, e := s.BrowseEntities(viewScope, BrowseRequest{ServerID: server, Profile: actorKey(a), ViewerFence: viewScope.Fence, Library: v.LibraryID, Pivot: v.Pivot, Query: v.Query, Sort: v.Sort, Limit: limit, Cursor: cursor})
		if e != nil {
			return out, e
		}
		out.Browse = &result
		out.NextCursor = result.PageInfo.NextCursor
	} else {
		raw, _ := json.Marshal(libraries)
		rev, e := s.homeRevision(libraries, actorKey(a))
		if e != nil {
			return out, e
		}
		rev.Catalog += r.Revision
		scope := cursorScope{Profile: actorKey(a), View: "saved_collection", Entity: id, Viewer: fence, Limit: limit}
		// Inaccessible membership has an opaque stable entry ID and no media facts.
		// Collections are ordered only for display, never by insertion/playback intent.
		restriction, bound := ItemRestrictionSQL("i.id", viewer.EffectiveRestrictions())
		from := ` FROM saved_resource_entries e LEFT JOIN catalog_entities i ON i.id=e.item_id AND i.retired=0 AND EXISTS(SELECT 1 FROM catalog_item_details detail WHERE detail.entity_id=i.id) AND i.library_id IN(SELECT id FROM catalog_libraries WHERE library_id IN(SELECT value FROM json_each(?)) AND retired=0) WHERE e.resource_id=? AND (i.id IS NULL OR ` + restriction + `)`
		args := append([]any{string(raw), id}, bound...)
		if cursor != "" {
			c, e := s.decodeRevisionCursor(cursor, scope, rev)
			if e != nil {
				return out, e
			}
			from += ` AND (COALESCE(i.title,'') COLLATE NOCASE>? OR (COALESCE(i.title,'') COLLATE NOCASE=? AND e.id>?))`
			args = append(args, c.Value, c.Value, c.ID)
		}
		args = append(args, limit+1)
		rows, e := s.read().Query(`SELECT e.id,COALESCE(pid(i.public_id),''),COALESCE(i.title,'')`+from+` ORDER BY COALESCE(i.title,'') COLLATE NOCASE,e.id LIMIT ?`, args...)
		if e != nil {
			return out, e
		}
		ids, names := []string{}, []string{}
		for rows.Next() {
			var entry SavedResourceEntry
			var item, title string
			if e = rows.Scan(&entry.ID, &item, &title); e != nil {
				rows.Close()
				return out, e
			}
			entry.Hidden = item == ""
			out.Entries = append(out.Entries, entry)
			ids = append(ids, item)
			names = append(names, title)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return out, e
		}
		if len(out.Entries) > limit {
			out.Entries = out.Entries[:limit]
			ids = ids[:limit]
			out.NextCursor, e = s.encodeRevisionCursor(cursorValue{Scope: scope, Value: names[limit-1], ID: out.Entries[limit-1].ID, Expires: time.Now().Add(30 * time.Minute).Unix()}, rev)
			if e != nil {
				return out, e
			}
		}
		items, e := s.mediaPage(actorKey(a), ids, false)
		if e != nil {
			return out, e
		}
		byID := map[string]Item{}
		for _, item := range items {
			byID[item.ID] = item
		}
		for i, item := range ids {
			if item == "" {
				continue
			}
			media, ok := byID[item]
			if !ok {
				return out, ErrStaleContinuation
			}
			entry := contentItem(media)
			if !media.Available {
				entry.Playback = nil
			}
			out.Entries[i].Media = &entry
		}
		after, e := s.homeRevision(libraries, actorKey(a))
		if e != nil {
			return out, e
		}
		after.Catalog += r.Revision
		if after != rev {
			return out, ErrStaleContinuation
		}
	}
	_, _, after, dead, e := savedResourceRole(s.read(), id, a)
	if e != nil {
		return out, e
	}
	if after != r.Revision || dead {
		return out, ErrStaleContinuation
	}
	return out, nil
}
func (s *Service) MutateSavedResource(a ResourceActor, id, action string, m SavedResourceMutation, authorize func(*sql.Tx) error) (SavedResourceReceipt, error) {
	out := SavedResourceReceipt{OperationID: m.OperationID, ResourceID: id}
	if !personalOperation.MatchString(m.OperationID) || m.ExpectedRevision < 0 || a.Authority == "" || a.AccountID == "" || a.ProfileID == "" {
		return out, errors.New("invalid resource operation")
	}
	if m.Name != nil && !resourceName(*m.Name) {
		return out, errors.New("resource name must contain 1-200 characters")
	}
	if m.Summary != nil && (!utf8.ValidString(*m.Summary) || utf8.RuneCountInString(*m.Summary) > 4000 || strings.ContainsRune(*m.Summary, 0)) {
		return out, errors.New("invalid resource summary")
	}
	if len(m.AddItemIDs) > 100 || len(m.RemoveEntryIDs) > 100 {
		return out, errors.New("membership batches are limited to 100 entries")
	}
	if m.Visibility != nil && *m.Visibility != "private" && *m.Visibility != "server" {
		return out, errors.New("collection visibility must be private or server")
	}
	if m.Kind != "" && action != "create" || (m.Name != nil || m.Summary != nil || m.Definition != nil || m.Visibility != nil) && action != "create" && action != "update" || (m.AddItemIDs != nil || m.RemoveEntryIDs != nil) && action != "entries" || (m.Authority != "" || m.AccountID != "" || m.ProfileID != "") && action != "share" && action != "unshare" || m.Role != "" && action != "share" {
		return out, errors.New("fields do not apply to resource operation")
	}
	hash := operationHash(struct {
		ID, Action string
		Mutation   SavedResourceMutation
	}{id, action, m})
	gated, e := dbwork.Begin(context.Background(), s.db, dbwork.ClassFrom(context.Background(), dbwork.ClassInteractive))
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if e = authorize(tx); e != nil {
		return out, e
	}
	role, kind, rev, deleted := "owner", m.Kind, int64(0), false
	if action != "create" {
		role, kind, rev, deleted, e = savedResourceRole(tx, id, a)
		if e != nil {
			return out, e
		}
		if role != "owner" && (role != "editor" || action != "entries") {
			return out, identity.ErrUnauthorized
		}
	}
	var prior, raw string
	e = tx.QueryRow(`SELECT request_hash,response FROM saved_resource_receipts WHERE owner_key=? AND operation_id=? AND created_at>=?`, actorKey(a), m.OperationID, time.Now().Add(-30*24*time.Hour).UTC().Format(time.RFC3339)).Scan(&prior, &raw)
	if e == nil {
		if prior != hash {
			return out, ErrOperationConflict
		}
		if e = json.Unmarshal([]byte(raw), &out); e != nil {
			return out, e
		}
		_, _, _, _, e = savedResourceRole(tx, out.ResourceID, a)
		return out, e
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	if e = checkRetired(tx, actorKey(a), "resource", m.OperationID, hash); e != nil {
		return out, e
	}
	if deleted {
		return out, sql.ErrNoRows
	}
	if rev != m.ExpectedRevision {
		return out, ErrPlaylistConflict
	}
	switch action {
	case "create":
		if m.Name == nil || (kind != "collection" && kind != "view") {
			return out, errors.New("resource kind and name are required")
		}
		if kind == "view" && m.Definition == nil || kind == "collection" && m.Definition != nil {
			return out, errors.New("only saved views require a query definition")
		}
		definition := "{}"
		if m.Definition != nil {
			if len(validateSavedView(tx, *m.Definition)) > 0 {
				return out, errors.New("saved view needs review")
			}
			b, _ := json.Marshal(m.Definition)
			definition = string(b)
		}
		id = identity.Token()
		out.ResourceID = id
		summary := ""
		if m.Summary != nil {
			summary = *m.Summary
		}
		visibility := "private"
		if m.Visibility != nil {
			if kind != "collection" {
				return out, errors.New("only collections carry visibility")
			}
			visibility = *m.Visibility
		}
		_, e = tx.Exec(`INSERT INTO saved_resources(id,kind,owner_key,owner_authority,owner_account,owner_profile,name,summary,visibility,definition,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, kind, actorKey(a), a.Authority, a.AccountID, a.ProfileID, *m.Name, summary, visibility, definition, time.Now().UTC().Format(time.RFC3339))
		rev = 1
	case "update":
		if m.Name == nil && m.Summary == nil && m.Definition == nil && m.Visibility == nil {
			return out, errors.New("set name, summary, visibility or saved query")
		}
		if m.Visibility != nil && kind != "collection" {
			return out, errors.New("only collections carry visibility")
		}
		var definition *string
		if m.Definition != nil {
			if kind != "view" || len(validateSavedView(tx, *m.Definition)) > 0 {
				return out, errors.New("invalid saved view definition")
			}
			b, _ := json.Marshal(m.Definition)
			v := string(b)
			definition = &v
		}
		_, e = tx.Exec(`UPDATE saved_resources SET name=COALESCE(?,name),summary=COALESCE(?,summary),visibility=COALESCE(?,visibility),definition=COALESCE(?,definition) WHERE id=?`, m.Name, m.Summary, m.Visibility, definition, id)
	case "delete":
		_, e = tx.Exec(`UPDATE saved_resources SET deleted=1 WHERE id=?`, id)
		if e == nil {
			_, e = tx.Exec(`DELETE FROM saved_resource_entries WHERE resource_id=?`, id)
		}
		if e == nil {
			_, e = tx.Exec(`DELETE FROM saved_resource_shares WHERE resource_id=?`, id)
		}
		if e == nil {
			_, e = tx.Exec(`DELETE FROM saved_pins WHERE resource_id=? AND kind=?`, id, kind)
		}
		out.Deleted = true
	case "entries":
		if kind != "collection" || len(m.AddItemIDs)+len(m.RemoveEntryIDs) == 0 {
			return out, errors.New("collection membership changes are required")
		}
		// Membership is reported per item: an item that vanished between the
		// client's read and this write must not discard the rest of the batch.
		outcomes := newEntryOutcomes()
		e = nil
		for _, item := range m.AddItemIDs {
			var entityID int64
			if err := tx.QueryRow(`SELECT id FROM catalog_entities WHERE public_id=pid_blob(?) AND retired=0`, item).Scan(&entityID); errors.Is(err, sql.ErrNoRows) {
				outcomes.fail(item, "not_found")
				continue
			} else if err != nil {
				return out, err
			}
			var member int
			if e = tx.QueryRow(`SELECT count(*) FROM saved_resource_entries WHERE resource_id=? AND item_id=?`, id, entityID).Scan(&member); e != nil {
				return out, e
			}
			if member > 0 {
				outcomes.Unchanged = append(outcomes.Unchanged, item)
				continue
			}
			if _, e = tx.Exec(`INSERT INTO saved_resource_entries(id,resource_id,item_id) VALUES(?,?,?) ON CONFLICT(resource_id,item_id) DO NOTHING`, identity.Token(), id, entityID); e != nil {
				return out, e
			}
			outcomes.Added = append(outcomes.Added, item)
		}
		for _, entry := range m.RemoveEntryIDs {
			result, err := tx.Exec(`DELETE FROM saved_resource_entries WHERE resource_id=? AND id=?`, id, entry)
			if err != nil {
				return out, err
			}
			if n, _ := result.RowsAffected(); n == 0 {
				outcomes.fail(entry, "not_found")
				continue
			}
			outcomes.Removed = append(outcomes.Removed, entry)
		}
		out.Entries = &outcomes
	case "share", "unshare":
		if kind != "collection" || m.Authority == "" || m.AccountID == "" || m.ProfileID == "" || (ResourceActor{m.Authority, m.AccountID, m.ProfileID}) == a {
			return out, errors.New("invalid collection share")
		}
		if action == "share" {
			if m.Role != "viewer" && m.Role != "editor" {
				return out, errors.New("invalid share role")
			}
			var n int
			if e = tx.QueryRow(`SELECT count(*) FROM saved_resource_shares WHERE resource_id=? AND NOT(authority=? AND account_id=? AND profile_id=?)`, id, m.Authority, m.AccountID, m.ProfileID).Scan(&n); e != nil {
				return out, e
			}
			if n >= MaxPlaylistShares {
				return out, ErrPlaylistCapacity
			}
			_, e = tx.Exec(`INSERT INTO saved_resource_shares VALUES(?,?,?,?,?) ON CONFLICT(resource_id,authority,account_id,profile_id) DO UPDATE SET role=excluded.role`, id, m.Authority, m.AccountID, m.ProfileID, m.Role)
		} else {
			_, e = tx.Exec(`DELETE FROM saved_resource_shares WHERE resource_id=? AND authority=? AND account_id=? AND profile_id=?`, id, m.Authority, m.AccountID, m.ProfileID)
		}
	default:
		return out, errors.New("unsupported resource operation")
	}
	if e != nil {
		return out, e
	}
	if action != "create" {
		rev++
		if _, e = tx.Exec(`UPDATE saved_resources SET revision=? WHERE id=?`, rev, id); e != nil {
			return out, e
		}
	}
	out.Revision = rev
	if e = fenceOperation(tx, actorKey(a), "resource", m.OperationID, hash); e != nil {
		return out, e
	}
	b, _ := json.Marshal(out)
	if _, e = tx.Exec(`INSERT INTO saved_resource_receipts VALUES(?,?,?,?,?)`, actorKey(a), m.OperationID, hash, string(b), time.Now().UTC().Format(time.RFC3339)); e != nil {
		return out, e
	}
	return out, gated.Commit()
}

func (s *Service) SavedResourceReceiptStatus(id string, a ResourceActor) (bool, int64, error) {
	role, _, revision, deleted, e := savedResourceRole(s.read(), id, a)
	if e != nil {
		return false, 0, e
	}
	if deleted && role != "owner" {
		return false, 0, sql.ErrNoRows
	}
	return deleted, revision, nil
}
