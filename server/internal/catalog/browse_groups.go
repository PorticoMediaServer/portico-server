package catalog

import (
	"strconv"

	"time"
)

// Group lists use their own indexed keys; their predicates filter member
// titles with exactly the same grammar as the entity pivot they open.
func (s *Service) browseAggregate(request BrowseRequest, pivot browsePivot) (BrowseResult, error) {
	out := BrowseResult{Pivot: pivot.ID, Entries: []ContentEntry{}, PositionIndex: []BrowsePositionAnchor{}}
	sorts := request.Sort
	if len(sorts) == 0 {
		sorts = titleAscending()
	}
	if len(sorts) != 1 {
		return out, browseIssue("sort", "choose one grouping sort")
	}
	sorting := sorts[0]
	defaults := map[string]string{"title": "asc", "count": "desc", "recentActivity": "desc"}
	def, ok := defaults[sorting.Field]
	if !ok {
		return out, browseIssue("sort", "this grouping sorts by name, count or recent activity")
	}
	if sorting.Direction == "" {
		sorting.Direction = def
	}
	if sorting.Direction != "asc" && sorting.Direction != "desc" || sorting.Seed != 0 {
		return out, browseIssue("sort", "grouping sort is invalid")
	}
	sorts = []BrowseSortSelection{sorting}
	out.Applied = BrowseApplied{Query: request.Query, Sort: sorts}
	request.Limit = pageLimit(request.Limit)
	if request.Cursor != "" && request.Range != nil {
		return out, browseIssue("range", "a direct range cannot include a cursor")
	}
	if request.Seek != nil {
		return out, browseIssue("seek", "this grouping does not support letter seeking")
	}
	revision, err := s.browseRevision(request, pivot, sorts)
	if err != nil {
		return out, err
	}
	out.PageInfo.Revision = revision
	scope := cursorScope{Library: request.Library, Viewer: request.ViewerFence, Profile: request.Profile, View: "browse:" + pivot.ID, Sort: canonicalSort(sorts), Category: queryFingerprint(request.Query), Limit: request.Limit}
	key, id := "", ""
	start := 0
	expires := time.Now().Add(BrowseCursorTTLSecond * time.Second).Unix()
	if request.Cursor != "" {
		c, err := s.decodeCursor(request.Cursor, scope)
		if err != nil {
			return out, err
		}
		if len(c.Keys) != 1 {
			return out, ErrCursor
		}
		key, id = c.Keys[0], c.ID
		expires = c.Expires
		start, err = strconv.Atoi(c.Value)
		if err != nil || start < 0 {
			return out, ErrCursor
		}
	}
	if request.Range != nil {
		if request.Range.Start < 0 || len(request.Range.Revision) > 128 || len(request.Range.AnchorID) > 256 {
			return out, browseIssue("range", "range identity is invalid")
		}
		start = request.Range.Start
	}
	// Unfiltered group keys are maintained by per-member deltas. An arbitrary
	// predicate or restriction changes the count itself, so that shape derives
	// its exact keys in SQL over indexed facet memberships.
	from := `catalog_browse_groups g`
	where := `g.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND g.field=?`
	args := []any{request.Library, pivot.Aggregate}
	if request.Query != nil || request.Restrictions.Active() {
		compiler := browseCompiler{profile: request.Profile, restrictions: request.Restrictions}
		clause, err := compiler.compile(request.Query)
		if err != nil {
			return out, err
		}
		restriction, restrictionArgs := EntityRestrictionSQL("e.entity_id", request.Restrictions)
		from = `(SELECT m.value,lower(portico_sort_title(m.value,'','')) AS name_key,count(*) AS total,max(m.recent_key) AS recent_key
   FROM catalog_facet_members m JOIN catalog_browse_rows e ON e.entity_id=m.entity_id
   WHERE m.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND m.field=? AND (` + clause + `) AND (` + restriction + `) GROUP BY m.value) g`
		args = append(args, compiler.args...)
		args = append(args, restrictionArgs...)
		where = "1"
	}
	var total int
	if err = s.read().QueryRow(`SELECT count(*) FROM `+from+` WHERE `+where, args...).Scan(&total); err != nil {
		return out, err
	}
	out.PageInfo.Total = total
	out.PageInfo.Start = start
	orderKey := map[string]string{"title": "g.name_key", "count": "g.total", "recentActivity": "g.recent_key"}[sorting.Field]
	direction, op := "ASC", ">"
	if sorting.Direction == "desc" {
		direction, op = "DESC", "<"
	}
	type group struct {
		value, key string
		count      int
	}
	page := func(key, id string, limit int) ([]group, error) {
		bound := where
		a := append([]any{}, args...)
		if id != "" {
			var typed any = key
			if sorting.Field == "count" {
				n, e := strconv.Atoi(key)
				if e != nil {
					return nil, ErrCursor
				}
				typed = n
			}
			bound += ` AND (` + orderKey + op + `? OR ` + orderKey + `=? AND g.value` + op + `?)`
			a = append(a, typed, typed, id)
		}
		a = append(a, limit)
		rows, err := s.read().Query(`SELECT g.value,g.total,`+orderKey+` FROM `+from+` WHERE `+bound+` ORDER BY `+orderKey+` `+direction+`,g.value `+direction+` LIMIT ?`, a...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		result := []group{}
		for rows.Next() {
			var g group
			var k any
			if err = rows.Scan(&g.value, &g.count, &k); err != nil {
				return nil, err
			}
			g.key = formatOrderValue(k)
			if n, ok := k.(int64); ok {
				g.key = strconv.FormatInt(n, 10)
			}
			result = append(result, g)
		}
		return result, rows.Err()
	}
	if request.Range != nil {
		for skip := start; skip > 0; {
			rows, err := page(key, id, min(skip, BrowseMaximumLimit))
			if err != nil {
				return out, err
			}
			if len(rows) == 0 {
				break
			}
			skip -= len(rows)
			last := rows[len(rows)-1]
			key, id = last.key, last.value
		}
	}
	rows, err := page(key, id, request.Limit+1)
	if err != nil {
		return out, err
	}
	out.PageInfo.HasMore = len(rows) > request.Limit
	if out.PageInfo.HasMore {
		rows = rows[:request.Limit]
	}
	for _, g := range rows {
		label := g.value
		if pivot.Aggregate == "decade" {
			label += "s"
		}
		count := g.count
		out.Entries = append(out.Entries, ContentEntry{ID: pivot.Aggregate + ":" + g.value, Kind: pivot.EntityKinds[0], Title: label, Count: &count, Navigation: &ContentNavigation{View: "browse", Category: pivot.Aggregate + ":" + g.value}})
	}
	if out.PageInfo.HasMore && request.Range == nil && len(rows) > 0 {
		last := rows[len(rows)-1]
		out.PageInfo.NextCursor, err = s.encodeCursor(cursorValue{Scope: scope, Value: strconv.Itoa(start + len(rows)), ID: last.value, Keys: []string{last.key}, Total: total, Expires: expires})
	}
	return out, err
}
