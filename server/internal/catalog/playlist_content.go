package catalog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
)

func directoryRevision(q personalReader, a ResourceActor) (ContentRevision, error) {
	var r ContentRevision
	e := q.QueryRow(`SELECT COALESCE((SELECT revision FROM playlist_directory_revisions WHERE authority=? AND account_id=? AND profile_id=?),0)`, a.Authority, a.AccountID, a.ProfileID).Scan(&r.Catalog)
	return r, e
}
func resourceEnvelope(r ContentRequest, title string) ContentEnvelope {
	return ContentEnvelope{Scope: ContentScope{r.ServerID, "", "mixed", r.View, r.EntityID, r.ViewerFence}, Heading: ContentHeading{Key: "saved." + r.View, Fallback: title}, Query: ContentQuery{Sort: r.Sort, Direction: r.Direction, Limit: r.Limit, SearchMode: "none"}, Navigation: savedNavigation(), Sorts: []ContentSort{}, Filters: []ContentFilter{}, Sections: []ContentSection{}}
}
func (s *Service) PlaylistDirectory(r ContentRequest, a ResourceActor) (ContentEnvelope, error) {
	if r.Sort == "" {
		r.Sort = "title"
	}
	if r.Direction == "" {
		r.Direction = "asc"
	}
	if r.Limit == 0 {
		r.Limit = 40
	}
	out := resourceEnvelope(r, "Playlists")
	out.Sorts = []ContentSort{{"title", "sort.title", []string{"asc", "desc"}}}
	if r.Sort != "title" || (r.Direction != "asc" && r.Direction != "desc") || r.Limit < 1 || r.Limit > 100 {
		return out, errors.New("invalid playlist directory query")
	}
	ctx := s.Context()
	gated, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	rev, e := directoryRevision(tx, a)
	if e != nil {
		return out, e
	}
	out.Revision = rev
	actions := []string{}
	var owned int
	if err := tx.QueryRow(`SELECT count(*) FROM catalog_playlists WHERE owner_authority=? AND owner_account=? AND owner_profile=? AND deleted=0`, a.Authority, a.AccountID, a.ProfileID).Scan(&owned); err != nil {
		return out, err
	}
	if owned < 10000 {
		actions = append(actions, "create_playlist")
	}
	out.Actions = &actions
	scope := cursorScope{Profile: a.ProfileID, View: "playlists", Viewer: r.ViewerFence, Sort: r.Sort, Direction: r.Direction, Limit: r.Limit}
	where := ` FROM catalog_playlists p WHERE p.deleted=0 AND p.id IN(SELECT owned.id FROM catalog_playlists owned WHERE owned.owner_authority=? AND owned.owner_account=? AND owned.owner_profile=? AND owned.deleted=0 UNION SELECT shared.id FROM playlist_shares share JOIN catalog_playlists shared ON shared.token=share.playlist_id WHERE share.authority=? AND share.account_id=? AND share.profile_id=?)`
	args := []any{a.Authority, a.AccountID, a.ProfileID, a.Authority, a.AccountID, a.ProfileID}
	var count int
	if e = tx.QueryRow(`SELECT count(*)`+where, args...).Scan(&count); e != nil {
		return out, e
	}
	op := ">"
	if r.Direction == "desc" {
		op = "<"
	}
	if r.Cursor != "" {
		c, err := s.decodeRevisionCursor(r.Cursor, scope, rev)
		if err != nil {
			return out, err
		}
		where += ` AND (p.name COLLATE NOCASE` + op + `? OR (p.name COLLATE NOCASE=? AND p.token` + op + `?))`
		args = append(args, c.Value, c.Value, c.ID)
	}
	args = append(args, r.Limit+1)
	rows, e := tx.Query(`SELECT p.token,p.name,p.summary,p.entry_count`+where+` ORDER BY p.name COLLATE NOCASE `+r.Direction+`,p.token `+r.Direction+` LIMIT ?`, args...)
	if e != nil {
		return out, e
	}
	entries := []ContentEntry{}
	for rows.Next() {
		var v ContentEntry
		var n int
		if e = rows.Scan(&v.ID, &v.Title, &v.Overview, &n); e != nil {
			rows.Close()
			return out, e
		}
		v.Kind = "playlist"
		v.Count = &n
		v.Navigation = &ContentNavigation{View: "playlist", EntityID: v.ID}
		entries = append(entries, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	next := ""
	if len(entries) > r.Limit {
		entries = entries[:r.Limit]
		v := entries[len(entries)-1]
		next, e = s.encodeRevisionCursor(cursorValue{Scope: scope, Value: v.Title, ID: v.ID, Expires: time.Now().Add(30 * time.Minute).Unix()}, rev)
		if e != nil {
			return out, e
		}
	}
	if len(entries) > 0 {
		out.Sections = append(out.Sections, contentSection("playlists", "grid", "Playlists", entries, count, next))
	} else {
		out.Empty = &ContentHeading{Key: "playlists.empty", Fallback: "No playlists yet."}
	}
	if e = gated.Commit(); e != nil {
		return out, e
	}
	after, e := directoryRevision(s.read(), a)
	if e != nil {
		return out, e
	}
	if after != rev {
		return out, ErrStaleContinuation
	}
	return out, nil
}
func (s *Service) PlaylistContent(r ContentRequest, a ResourceActor, libraries []string) (ContentEnvelope, error) {
	if r.Profile != "" && r.Profile != r.Viewer.Profile || r.ViewerFence != "" && r.ViewerFence != r.Viewer.Fence {
		return ContentEnvelope{}, ErrCursor
	}
	r = r.scoped()
	libraries = r.Viewer.Libraries
	if err := s.prepareViewer(r.Viewer); err != nil {
		return ContentEnvelope{}, err
	}
	if r.Limit == 0 {
		r.Limit = 40
	}
	r.View = "playlist"
	r.Sort = "position"
	r.Direction = "asc"
	out := resourceEnvelope(r, "Playlist")
	if r.Limit < 1 || r.Limit > 100 {
		return out, errors.New("invalid playlist page limit")
	}
	role, resourceRevision, deleted, e := catalogPlaylistRole(s.read(), r.EntityID, a)
	if e != nil {
		return out, e
	}
	if deleted || role == "" {
		return out, sql.ErrNoRows
	}
	resource := Playlist{ServerID: r.ServerID, ViewerFence: r.ViewerFence, ID: r.EntityID, Role: role, Revision: resourceRevision}
	if e = s.read().QueryRow(`SELECT name,summary,entry_count FROM catalog_playlists WHERE token=? AND deleted=0`, r.EntityID).Scan(&resource.Name, &resource.Summary, &resource.EntryCount); e != nil {
		return out, e
	}
	out.Heading = ContentHeading{Key: "playlist.title", Fallback: resource.Name}
	out.PlaylistRevision = &resource.Revision
	rev, e := s.homeRevision(libraries, r.Profile)
	if e != nil {
		return out, e
	}
	rev.Catalog += resource.Revision
	out.Revision = rev
	scope := cursorScope{Profile: a.ProfileID, View: "playlist", Entity: r.EntityID, Viewer: r.ViewerFence, Sort: "position", Direction: "asc", Limit: r.Limit}
	position := ""
	if r.Cursor != "" {
		c, err := s.decodeRevisionCursor(r.Cursor, scope, rev)
		if err != nil {
			return out, err
		}
		position = c.Value
	}
	raw, _ := json.Marshal(libraries)
	visibility, visibleArgs := r.Viewer.itemVisibilitySQL("item.id")
	args := append([]any{string(raw)}, visibleArgs...)
	args = append(args, r.EntityID, position, position, int64(0), r.Limit+1)
	var afterEntry int64
	if r.Cursor != "" {
		c, err := s.decodeRevisionCursor(r.Cursor, scope, rev)
		if err != nil {
			return out, err
		}
		afterEntry, err = strconv.ParseInt(c.ID, 10, 64)
		if err != nil || afterEntry < 1 {
			return out, ErrCursor
		}
		args[len(args)-2] = afterEntry
	}
	rows, e := s.read().Query(`SELECT e.token,e.id,e.order_key,CASE WHEN l.library_id IN(SELECT value FROM json_each(?)) AND `+visibility+` THEN pid(item.public_id) ELSE NULL END FROM catalog_playlist_entries e JOIN catalog_playlists p ON p.id=e.playlist_id LEFT JOIN catalog_entities item ON item.id=e.item_id LEFT JOIN catalog_libraries l ON l.id=item.library_id WHERE p.token=? AND p.deleted=0 AND (e.order_key>? OR (e.order_key=? AND e.id>?)) ORDER BY e.order_key,e.id LIMIT ?`, args...)
	if e != nil {
		return out, e
	}
	type occurrence struct {
		token    string
		id       int64
		position string
		item     sql.NullString
	}
	selected := []occurrence{}
	for rows.Next() {
		var v occurrence
		if e = rows.Scan(&v.token, &v.id, &v.position, &v.item); e != nil {
			rows.Close()
			return out, e
		}
		selected = append(selected, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	next := ""
	if len(selected) > r.Limit {
		selected = selected[:r.Limit]
		last := selected[len(selected)-1]
		next, e = s.encodeRevisionCursor(cursorValue{Scope: scope, Value: last.position, ID: strconv.FormatInt(last.id, 10), Expires: time.Now().Add(30 * time.Minute).Unix()}, rev)
		if e != nil {
			return out, e
		}
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, v := range selected {
		if v.item.Valid && !seen[v.item.String] {
			ids = append(ids, v.item.String)
			seen[v.item.String] = true
		}
	}
	items, e := s.mediaPage(r.Profile, ids, false)
	if e != nil {
		return out, e
	}
	media := map[string]ContentEntry{}
	for _, v := range items {
		entry := contentItem(v)
		if !v.Available {
			entry.Playback = nil
		}
		media[v.ID] = entry
	}
	entries := []ContentEntry{}
	for _, v := range selected {
		hidden := true
		entry := ContentEntry{ID: v.token, Kind: "playlist_entry", Hidden: &hidden}
		if item, ok := media[v.item.String]; v.item.Valid && ok {
			hidden = false
			entry.Media = &item
		}
		entries = append(entries, entry)
	}
	if len(entries) > 0 {
		out.Sections = append(out.Sections, contentSection("entries", "list", resource.Name, entries, resource.EntryCount, next))
	} else {
		out.Empty = &ContentHeading{Key: "playlist.empty", Fallback: "This playlist has no entries yet."}
	}
	_, afterResource, deleted, e := catalogPlaylistRole(s.read(), r.EntityID, a)
	if e != nil {
		return out, e
	}
	after, e := s.homeRevision(libraries, r.Profile)
	if e != nil {
		return out, e
	}
	after.Catalog += afterResource
	if deleted || afterResource != resourceRevision || after != rev {
		return out, ErrStaleContinuation
	}
	return out, nil
}
