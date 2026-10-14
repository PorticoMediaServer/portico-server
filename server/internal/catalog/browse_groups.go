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
	// A library-wide title sort may be carried onto a grouping. Keep the
	// grouping's meaningful sorts and report name order for other sorts.
	if len(sorts) != 1 || sorts[0].Field != "title" && sorts[0].Field != "count" && sorts[0].Field != "recentActivity" {
		sorts = titleAscending()
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
	// Groupings have no letter index; a carried letter jump leaves the
	// ordinary page position unchanged and is absent from Applied.
	request.Seek = nil
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
		from = `(WITH matched AS MATERIALIZED (
 SELECT e.entity_id FROM catalog_browse_rows e
 WHERE e.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND (` + clause + `) AND (` + restriction + `))
 SELECT m.value,lower(portico_sort_title(m.value,'','')) AS name_key,count(*) AS total,max(m.recent_key) AS recent_key
 FROM matched CROSS JOIN catalog_facet_members m ON m.entity_id=matched.entity_id
 WHERE m.field=? GROUP BY m.value) g`
		args = append([]any{request.Library}, compiler.args...)
		args = append(args, restrictionArgs...)
		args = append(args, pivot.Aggregate)
		where = "1"
	}
	orderKey := map[string]string{"title": "g.name_key", "count": "g.total", "recentActivity": "g.recent_key"}[sorting.Field]
	direction, op := "ASC", ">"
	if sorting.Direction == "desc" {
		direction, op = "DESC", "<"
	}
	type group struct {
		value, key string
		count      int
	}
	page := func(key, id string, limit int) ([]group, int, error) {
		bound := where
		a := append([]any{}, args...)
		if id != "" {
			var typed any = key
			if sorting.Field == "count" {
				n, e := strconv.Atoi(key)
				if e != nil {
					return nil, 0, ErrCursor
				}
				typed = n
			}
			bound += ` AND (` + orderKey + `,g.value)` + op + `(?,?)`
			a = append(a, typed, id)
		}
		// Evaluate membership once, then rank the much smaller group vocabulary.
		// A direct range must not re-evaluate every title for each skipped page.
		query := `WITH groups AS MATERIALIZED (SELECT g.value,g.total,` + orderKey + ` AS order_key FROM ` + from + ` WHERE ` + where + `),
 metadata AS (SELECT count(*) AS total FROM groups),
 ranked AS (SELECT value,total,order_key,row_number() OVER(ORDER BY order_key ` + direction + `,value ` + direction + `)-1 AS position FROM groups)
 SELECT metadata.total,ranked.value,ranked.total,ranked.order_key FROM metadata LEFT JOIN ranked ON ranked.position>=min(?,max(0,metadata.total-1)) AND ranked.position<min(?,max(0,metadata.total-1))+? ORDER BY ranked.position`
		if request.Range == nil {
			// Continuations seek indexed maintained keys when there is no filter.
			query = `WITH metadata AS (SELECT count(*) AS total FROM ` + from + ` WHERE ` + where + `),
 page AS (SELECT g.value,g.total,` + orderKey + ` AS order_key FROM ` + from + ` WHERE ` + bound + ` ORDER BY ` + orderKey + ` ` + direction + `,g.value ` + direction + ` LIMIT ?)
 SELECT metadata.total,page.value,page.total,page.order_key FROM metadata LEFT JOIN page ON 1 ORDER BY page.order_key ` + direction + `,page.value ` + direction
			if request.Query != nil || request.Restrictions.Active() {
				query = `WITH groups AS MATERIALIZED (SELECT * FROM ` + from + ` WHERE ` + where + `),
 metadata AS (SELECT count(*) AS total FROM groups),
 page AS (SELECT g.value,g.total,` + orderKey + ` AS order_key FROM groups g WHERE ` + bound + ` ORDER BY ` + orderKey + ` ` + direction + `,g.value ` + direction + ` LIMIT ?)
 SELECT metadata.total,page.value,page.total,page.order_key FROM metadata LEFT JOIN page ON 1 ORDER BY page.order_key ` + direction + `,page.value ` + direction
				a = append(append([]any{}, args...), a[len(args):]...)
				a = append(a, limit)
			} else {
				a = append(append(append([]any{}, args...), a...), limit)
			}
		} else {
			a = append(append([]any{}, args...), start, start, limit)
		}
		rows, err := s.read().Query(query, a...)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		result := []group{}
		total := 0
		for rows.Next() {
			var g group
			var k any
			var value *string
			var count *int
			if err = rows.Scan(&total, &value, &count, &k); err != nil {
				return nil, 0, err
			}
			if value == nil {
				continue
			}
			g.value, g.count = *value, *count
			g.key = formatOrderValue(k)
			if n, ok := k.(int64); ok {
				g.key = strconv.FormatInt(n, 10)
			}
			result = append(result, g)
		}
		return result, total, rows.Err()
	}
	rows, total, err := page(key, id, request.Limit+1)
	if err != nil {
		return out, err
	}
	if request.Range != nil && start >= total {
		start = max(0, total-1)
	}
	out.PageInfo.Total = total
	out.PageInfo.Start = start
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
