package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/personalstate"
	"strconv"
	"time"
)

// savedSorts are all computed on the server. progress and duration are derived
// columns rather than client arithmetic, so paging stays stable across devices.
var savedSorts = map[string]string{
	"title":    `lower(i.title)`,
	"added":    `COALESCE(detail.added_text,'')`,
	"updated":  `COALESCE(NULLIF(p.last_played_at,''),COALESCE(detail.added_text,''))`,
	"year":     `CAST(COALESCE(i.year,0) AS REAL)`,
	"duration": `COALESCE((SELECT duration FROM catalog_browse_item_metrics WHERE item_id=i.id),0.0)`,
	"progress": `COALESCE((SELECT g.position FROM progress g WHERE g.profile_id=p.profile_id AND g.item_id=i.id),0.0)`,
}
var savedNumericSorts = map[string]bool{"year": true, "duration": true, "progress": true}

// savedFilters read the viewer's own state only. inProgress means started and
// not finished; a completed item is never "in progress" because its resume was
// cleared.
var savedFilters = map[string]string{
	"all":        ``,
	"unwatched":  ` AND ` + personalstate.CompactSQL("p.profile_id", "i.id") + `=0`,
	"inProgress": ` AND ` + personalstate.CompactSQL("p.profile_id", "i.id") + `=0 AND COALESCE((SELECT g.position FROM progress g WHERE g.profile_id=p.profile_id AND g.item_id=i.id),0)>0`,
}

func savedNavigation() []ContentTab {
	return []ContentTab{{"watchlist", "saved.watchlist", "watchlist"}, {"favorites", "saved.favorites", "favorites"}, {"playlists", "saved.playlists", "playlists"}}
}
func savedSortOptions() []ContentSort {
	return []ContentSort{{"title", "sort.title", []string{"asc", "desc"}}, {"added", "sort.added", []string{"asc", "desc"}}, {"updated", "sort.updated", []string{"asc", "desc"}}, {"year", "sort.year", []string{"asc", "desc"}}, {"duration", "sort.duration", []string{"asc", "desc"}}, {"progress", "sort.progress", []string{"asc", "desc"}}}
}
func savedFilterOptions(all, unwatched, inProgress int) []ContentFilter {
	return []ContentFilter{{ID: "filter", LabelKey: "saved.filter", Options: []ContentFilterOption{{ID: "all", Label: "All", Count: all}, {ID: "unwatched", Label: "Unwatched", Count: unwatched}, {ID: "inProgress", Label: "In progress", Count: inProgress}}}}
}
func (s *Service) Saved(r ContentRequest, libraries []string) (ContentEnvelope, error) {
	if dbwork.Snapshot(s.Context()) == nil {
		var out ContentEnvelope
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var readErr error
			out, readErr = s.WithContext(ctx).Saved(r, libraries)
			return readErr
		})
		return out, err
	}
	if err := s.compactProjectionReady(25, 26); err != nil {
		return ContentEnvelope{}, err
	}
	if r.Profile != "" && r.Profile != r.Viewer.Profile || r.ViewerFence != "" && r.ViewerFence != r.Viewer.Fence {
		return ContentEnvelope{}, ErrCursor
	}
	r = r.scoped()
	libraries = r.Viewer.Libraries
	if err := s.prepareViewer(r.Viewer); err != nil {
		return ContentEnvelope{}, err
	}
	out := ContentEnvelope{Scope: ContentScope{r.ServerID, "", "mixed", r.View, "", r.ViewerFence}, Navigation: savedNavigation(), Sorts: savedSortOptions(), Filters: []ContentFilter{}, Sections: []ContentSection{}}
	if r.View != "watchlist" && r.View != "favorites" {
		return out, errors.New("invalid Saved view")
	}
	if r.Sort == "" {
		r.Sort = "title"
	}
	if r.Direction == "" {
		r.Direction = "asc"
	}
	if r.Filter == "" {
		r.Filter = "all"
	}
	if r.Limit == 0 {
		r.Limit = 40
	}
	order, sortOK := savedSorts[r.Sort]
	filter, filterOK := savedFilters[r.Filter]
	if !sortOK || !filterOK || (r.Direction != "asc" && r.Direction != "desc") || r.Limit < 1 || r.Limit > 100 || r.Q != "" || r.Category != "" || r.EntityID != "" {
		return out, errors.New("invalid Saved query")
	}
	heading := "My List"
	flag := "watchlisted"
	if r.View == "favorites" {
		heading = "Favorites"
		flag = "favorite"
	}
	out.Heading = ContentHeading{Key: "saved." + r.View, Fallback: heading}
	out.Query = ContentQuery{Sort: r.Sort, Direction: r.Direction, Category: r.Filter, Limit: r.Limit, SearchMode: "none"}
	rev, e := s.homeRevision(libraries, r.Profile)
	if e != nil {
		return out, e
	}
	out.Revision = rev
	scope := cursorScope{Profile: r.Profile, View: r.View, Section: r.View, Viewer: r.ViewerFence, Sort: r.Sort, Direction: r.Direction, Category: r.Filter, Limit: r.Limit}
	raw, _ := json.Marshal(libraries)
	// A saved title is a playable item (movie, episode, song, book file) or a title that holds
	// them (show, artist, album, book): the flag sits in the same personal_items row either way.
	// An item is visible by its own rule; a container when it has a visible member.
	base := ` FROM personal_items p CROSS JOIN catalog_entities i ON i.id=p.item_id LEFT JOIN catalog_item_details detail ON detail.entity_id=i.id WHERE p.profile_id=? AND p.` + flag + `=1 AND i.retired=0 AND i.library_id IN(SELECT id FROM catalog_libraries WHERE library_id IN(SELECT value FROM json_each(?)) AND retired=0)`
	baseArgs := []any{r.Profile, string(raw)}
	visibility, visibleArgs := r.Viewer.itemVisibilitySQL("i.id")
	containers, containerArgs := r.Viewer.entityVisibilitySQL("i.id", "(SELECT library_id FROM catalog_libraries WHERE id=i.library_id)")
	base += ` AND ((i.kind IN(1,4,7,9) AND ` + visibility + `) OR (i.kind IN(2,5,6,8) AND ` + containers + `))`
	baseArgs = append(append(baseArgs, visibleArgs...), containerArgs...)
	var all, unwatched, inProgress int
	if e = s.read().QueryRow(`SELECT count(*)`+base, baseArgs...).Scan(&all); e != nil {
		return out, e
	}
	if e = s.read().QueryRow(`SELECT count(*)`+base+savedFilters["unwatched"], baseArgs...).Scan(&unwatched); e != nil {
		return out, e
	}
	if e = s.read().QueryRow(`SELECT count(*)`+base+savedFilters["inProgress"], baseArgs...).Scan(&inProgress); e != nil {
		return out, e
	}
	out.Filters = savedFilterOptions(all, unwatched, inProgress)
	count := map[string]int{"all": all, "unwatched": unwatched, "inProgress": inProgress}[r.Filter]
	where := base + filter
	args := append([]any{}, baseArgs...)
	op := ">"
	if r.Direction == "desc" {
		op = "<"
	}
	if r.Cursor != "" {
		c, err := s.decodeRevisionCursor(r.Cursor, scope, rev)
		if err != nil {
			return out, err
		}
		value, err := cursorOrderValue(c.Value, savedNumericSorts[r.Sort])
		if err != nil {
			return out, err
		}
		entityID, err := strconv.ParseInt(c.ID, 10, 64)
		if err != nil || entityID < 1 {
			return out, ErrCursor
		}
		where += ` AND (` + order + op + `? OR (` + order + `=? AND i.id` + op + `?))`
		args = append(args, value, value, entityID)
	}
	args = append(args, r.Limit+1)
	selected, e := s.homeCandidates(`SELECT i.id,`+order+where+` ORDER BY `+order+` `+r.Direction+`,i.id `+r.Direction+` LIMIT ?`, args...)
	if e != nil {
		return out, e
	}
	next := ""
	if len(selected) > r.Limit {
		selected = selected[:r.Limit]
		last := selected[len(selected)-1]
		next, e = s.encodeRevisionCursor(cursorValue{Scope: scope, Value: last.order, ID: last.id, Expires: time.Now().Add(30 * time.Minute).Unix()}, rev)
		if e != nil {
			return out, e
		}
	}
	ids := []string{}
	entityIDs := make([]int64, 0, len(selected))
	for _, row := range selected {
		entityID, err := strconv.ParseInt(row.id, 10, 64)
		if err != nil || entityID < 1 {
			return out, ErrCursor
		}
		entityIDs = append(entityIDs, entityID)
	}
	rawEntityIDs, _ := json.Marshal(entityIDs)
	publicRows, e := s.read().Query(`SELECT id,pid(public_id) FROM catalog_entities WHERE id IN(SELECT value FROM json_each(?))`, string(rawEntityIDs))
	if e != nil {
		return out, e
	}
	publicIDs := map[int64]string{}
	for publicRows.Next() {
		var id int64
		var public string
		if e = publicRows.Scan(&id, &public); e != nil {
			publicRows.Close()
			return out, e
		}
		publicIDs[id] = public
	}
	e = publicRows.Err()
	publicRows.Close()
	if e != nil {
		return out, e
	}
	for _, id := range entityIDs {
		public, ok := publicIDs[id]
		if !ok {
			return out, ErrStaleContinuation
		}
		ids = append(ids, public)
	}
	// The same entries a recommendation row gives these ids: items with their state, containers
	// with their own art (or a member's).
	entries, e := s.WithRecommendationRestrictions(r.Viewer.EffectiveRestrictions()).homeEngineEntries(r.Profile, ids)
	if e != nil {
		return out, e
	}
	for i := range entries {
		if entries[i].Available != nil && !*entries[i].Available {
			entries[i].Playback = nil
		}
	}
	if len(entries) > 0 {
		out.Sections = append(out.Sections, contentSection(r.View, "grid", heading, entries, count, next))
	} else {
		out.Empty = &ContentHeading{Key: "saved.empty", Fallback: "Nothing saved here yet."}
	}
	after, e := s.homeRevision(libraries, r.Profile)
	if e != nil {
		return out, e
	}
	if after != rev {
		return out, ErrStaleContinuation
	}
	return out, nil
}
