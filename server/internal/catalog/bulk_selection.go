package catalog

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"strconv"

	"portico.local/server/internal/identity"
)

func (s *Service) bulkSelection(p identity.Principal, v Viewer, in JobRequest) (string, []any, error) {
	selector := in.Selector
	n := 0
	where := ""
	from := "catalog_entities i"
	distinct := ""
	args := []any{}
	library := ""
	revision := ""
	if selector.Items != nil {
		n++
		ids := selector.Items.IDs
		if len(ids) == 0 || len(ids) > 200 {
			return "", nil, jobInvalid("items.ids must contain 1 to 200 IDs")
		}
		seen := map[string]bool{}
		for _, id := range ids {
			if id == "" || len(id) > 256 || seen[id] {
				return "", nil, jobInvalid("invalid or duplicate item ID")
			}
			seen[id] = true
		}
		where = `i.public_id IN(SELECT pid_blob(value) FROM json_each(?))`
		args = append(args, idsJSON(ids))
	}
	if selector.Container != nil {
		n++
		c := selector.Container
		if c.ID == "" || len(c.ID) > 256 {
			return "", nil, jobInvalid("invalid container ID")
		}
		switch c.Kind {
		case "library":
			library = c.ID
			where = `i.library_id=(SELECT id FROM catalog_libraries WHERE library_id=? AND retired=0)`
			args = append(args, c.ID)
		case "playlist":
			_, _, deleted, e := catalogPlaylistRole(s.read(), c.ID, ResourceActor{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID})
			if e != nil {
				return "", nil, e
			}
			if deleted {
				return "", nil, sql.ErrNoRows
			}
			where = `i.id IN(SELECT entry.item_id FROM catalog_playlist_entries entry JOIN catalog_playlists playlist ON playlist.id=entry.playlist_id WHERE playlist.token=? AND playlist.deleted=0)`
			args = append(args, c.ID)
		case "show", "season", "album", "artist", "book", "collection":
			var kind string
			if e := s.read().QueryRow(`SELECT `+compactBrowseKindSQL("row.kind")+`,l.library_id FROM catalog_browse_rows row JOIN catalog_libraries l ON l.id=row.library_id WHERE row.entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) LIMIT 1`, c.ID).Scan(&kind, &library); e != nil {
				return "", nil, e
			}
			if kind != c.Kind {
				return "", nil, sql.ErrNoRows
			}
			from = `catalog_browse_memberships bm JOIN catalog_entities i ON i.id=bm.item_id`
			distinct = "DISTINCT "
			where = `bm.entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`
			args = append(args, c.ID)
		default:
			return "", nil, jobInvalid("unsupported container kind")
		}
	}
	if selector.Query != nil {
		n++
		q := selector.Query
		var node *BrowseNode
		if len(q.Filter) > 0 {
			decoder := json.NewDecoder(bytes.NewReader(q.Filter))
			decoder.DisallowUnknownFields()
			if e := decoder.Decode(&node); e != nil {
				return "", nil, jobInvalid("invalid query filter")
			}
		}
		library = q.LibraryID
		lib, e := s.library(library)
		if e != nil {
			return "", nil, e
		}
		pivot, ok := pivotForKind(lib.Kind, q.Pivot)
		if !ok || !pivot.Browsable {
			return "", nil, ErrBrowsePivot
		}
		sorts, e := resolveBrowseSorts(pivot, q.Sort)
		if e != nil {
			return "", nil, e
		}
		scope, values := browseScopeSQL(library, pivot)
		c := &browseCompiler{profile: v.Profile}
		filter, e := c.compile(node)
		if e != nil {
			return "", nil, e
		}
		where = `i.id IN(SELECT bm.item_id FROM catalog_browse_rows e JOIN catalog_browse_memberships bm ON bm.entity_id=e.entity_id WHERE ` + scope + ` AND ` + filter + `)`
		args = append(args, values...)
		args = append(args, c.args...)
		revision, e = s.browseRevision(BrowseRequest{Library: library, Profile: v.Profile, ViewerFence: v.Fence, Query: node}, pivot, sorts)
		if e != nil {
			return "", nil, e
		}
	}
	if n != 1 {
		return "", nil, jobInvalid("selector requires exactly one of container, query, items")
	}
	needsBrowse := selector.Query != nil || selector.Container != nil && selector.Container.Kind != "playlist" && selector.Container.Kind != "library"
	if needsBrowse {
		if err := s.compactProjectionReady(18, 19); err != nil {
			return "", nil, err
		}
	}
	if selector.Query != nil || selector.Container != nil && selector.Container.Kind != "playlist" && selector.Container.Kind != "library" {
		if err := s.browseReady(library); err != nil {
			return "", nil, err
		}
	}
	if library != "" && !v.AllowsLibrary(library) {
		return "", nil, sql.ErrNoRows
	}
	if in.Expected != nil && in.Expected.CatalogRevision != "" {
		if revision == "" {
			if library == "" {
				return "", nil, jobInvalid("catalogRevision requires a library-scoped selector")
			}
			r, e := s.ContentRevision(library, v.Profile)
			if e != nil {
				return "", nil, e
			}
			revision = strconv.FormatInt(r.Catalog, 10)
		}
		if in.Expected.CatalogRevision != revision {
			return "", nil, ErrStaleContinuation
		}
	}
	visible, bound := v.itemVisibilitySQL("i.id")
	args = append(args, bound...)
	return `SELECT ` + distinct + `i.id AS id FROM ` + from + ` JOIN catalog_kinds k ON k.id=i.kind AND k.playable=1 WHERE i.retired=0 AND ` + where + ` AND ` + visible + recordingsClause("i.id", v.EffectiveRestrictions()) + ` AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=i.id) ORDER BY i.id`, args, nil
}

func (s *Service) BulkFailures(profile, id, cursor string, v Viewer) (JobFailures, error) {
	out := JobFailures{Items: []JobFailure{}}
	if _, e := s.BulkJob(profile, id); e != nil {
		return out, e
	}
	after := int64(0)
	if cursor != "" {
		n, e := strconv.ParseInt(cursor, 10, 64)
		if e != nil || n < 0 {
			return out, ErrCursor
		}
		after = n
	}
	visible, args := v.itemVisibilitySQL("i.id")
	args = append([]any{id, after}, args...)
	rows, e := s.read().Query(`SELECT f.ordinal,pid(i.public_id),f.code FROM personal_job_failures f JOIN catalog_entities i ON i.id=f.item_id JOIN catalog_kinds k ON k.id=i.kind AND k.playable=1 WHERE f.job_id=? AND f.ordinal>? AND i.retired=0 AND `+visible+recordingsClause("i.id", v.EffectiveRestrictions())+` ORDER BY f.ordinal LIMIT 101`, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		var n int64
		var f JobFailure
		if e = rows.Scan(&n, &f.ItemID, &f.Code); e != nil {
			return out, e
		}
		if len(out.Items) == 100 {
			out.NextCursor = strconv.FormatInt(last, 10)
			break
		}
		out.Items = append(out.Items, f)
		last = n
	}
	return out, rows.Err()
}
