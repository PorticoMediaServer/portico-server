package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// ErrBrowsePivot is returned when a caller names a pivot the library kind does
// not publish.
var ErrBrowsePivot = errors.New("pivot is not published for this library")

type BrowseSortSelection struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}
type BrowseRange struct {
	Start    int    `json:"start"`
	Revision string `json:"revision,omitempty"`
	AnchorID string `json:"anchorId,omitempty"`
}
type BrowseSeek struct {
	Prefix string `json:"prefix"`
}
type BrowsePositionAnchor struct {
	Key   string `json:"key"`
	Index int    `json:"index"`
}
type BrowsePageInfo struct {
	Start      int    `json:"start"`
	Total      int    `json:"total"`
	Revision   string `json:"revision"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor"`
}

type BrowseApplied struct {
	Query *BrowseNode           `json:"query"`
	Sort  []BrowseSortSelection `json:"sort"`
	Seek  *BrowseSeek           `json:"seek,omitempty"`
}
type BrowseResult struct {
	Pivot         string                 `json:"pivot"`
	Applied       BrowseApplied          `json:"applied"`
	Entries       []ContentEntry         `json:"entries"`
	PageInfo      BrowsePageInfo         `json:"pageInfo"`
	PositionIndex []BrowsePositionAnchor `json:"positionIndex"`
}

// BrowseRequest is the whole of the POST body plus the authorized scope the
// route resolved. Profile is the qualified personal key; ViewerFence folds the
// authorization facts into the revision so a permission change invalidates a
// range the same way a catalog change does.
type BrowseRequest struct {
	ServerID    string
	Library     string
	Profile     string
	ViewerFence string
	Pivot       string
	Query       *BrowseNode
	Sort        []BrowseSortSelection
	Limit       int
	Cursor      string
	Range       *BrowseRange
	Seek        *BrowseSeek
	// Restrictions is the viewer's content restriction. The browse engine applies
	// it in browseSelect; see restrictions.go for the predicate it compiles.
	Restrictions identity.ContentRestrictions
	viewer       Viewer
	// scope overrides the cursor scope so the legacy GET surface keeps its
	// existing continuation fencing.
	scope *cursorScope
}

func canonicalSort(selection []BrowseSortSelection) string {
	parts := make([]string, 0, len(selection))
	for _, entry := range selection {
		parts = append(parts, entry.Field+":"+entry.Direction)
	}
	return strings.Join(parts, ",")
}

func queryFingerprint(node *BrowseNode) string {
	if node == nil {
		return ""
	}
	raw, err := json.Marshal(node)
	if err != nil {
		return "invalid"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16])
}

// resolveBrowseSorts falls back to the pivot default and rejects anything the
// capabilities do not publish, naming the offending path.
func resolveBrowseSorts(pivot browsePivot, selection []BrowseSortSelection) ([]BrowseSortSelection, error) {
	if len(selection) == 0 {
		return append([]BrowseSortSelection{}, pivot.DefaultSort...), nil
	}
	if len(selection) > BrowseMaximumSorts {
		return nil, browseIssue("sort", fmt.Sprintf("at most %d sort entries are supported", BrowseMaximumSorts))
	}
	kindFilter := map[string]bool{}
	for _, kind := range entityKindMembers(pivot) {
		kindFilter[kind] = true
	}
	seen := map[string]bool{}
	out := make([]BrowseSortSelection, 0, len(selection))
	for index, entry := range selection {
		path := fmt.Sprintf("sort[%d]", index)
		definition, ok := browseSortByID(entry.Field)
		if !ok {
			return nil, browseIssue(path+".field", fmt.Sprintf("sort %q is not supported", entry.Field))
		}
		if !anyKind(definition.ApplicableKinds, kindFilter) {
			return nil, browseIssue(path+".field", fmt.Sprintf("sort %q does not apply to this pivot", entry.Field))
		}
		if seen[entry.Field] {
			return nil, browseIssue(path+".field", "a sort field may appear once")
		}
		if entry.Field == "forYou" && len(selection) > 1 {
			return nil, browseIssue(path+".field", "forYou orders on its own")
		}
		seen[entry.Field] = true
		direction := entry.Direction
		if direction == "" {
			direction = definition.DefaultDirection
		}
		if !containsString(definition.Directions, direction) {
			return nil, browseIssue(path+".direction", fmt.Sprintf("sort %q takes direction %s", entry.Field, strings.Join(definition.Directions, " or ")))
		}
		out = append(out, BrowseSortSelection{Field: entry.Field, Direction: direction})
	}
	return out, nil
}

// browseKeyedSorts reports whether every selected sort is a column of
// `catalog_browse_rows`, which is what lets a page seek to its cursor rather than
// skip to it. A profile sort is a correlated aggregate over the membership
// table, and a keyset over one would evaluate it for every row it skipped.
func browseKeyedSorts(selection []BrowseSortSelection) bool {
	for _, entry := range selection {
		definition, ok := browseSortByID(entry.Field)
		if !ok || definition.Ordering < 0 || definition.Profile {
			return false
		}
	}
	return len(selection) > 0
}

// browseKeysetSQL compiles "strictly after the row whose sort key is keys" for
// the selected sorts, followed by the entity id as the final tie-break (in the
// first sort's direction, as browseOrderSQL orders it). It is
// the lexicographic comparison written out, because SQLite's row-value form
// cannot carry a per-component collation.
func browseKeysetSQL(selection []BrowseSortSelection, keys []any, anchor int64) (string, []any) {
	args := []any{}
	clause := ""
	for index, entry := range selection {
		definition, ok := browseSortByID(entry.Field)
		if !ok {
			return "", nil
		}
		operator := ">"
		if entry.Direction == "desc" {
			operator = "<"
		}
		equal := ""
		equalArgs := []any{}
		for earlier := 0; earlier < index; earlier++ {
			previous, _ := browseSortByID(selection[earlier].Field)
			equal += previous.Expression + "=? AND "
			equalArgs = append(equalArgs, keys[earlier])
		}
		if clause != "" {
			clause += " OR "
		}
		clause += "(" + equal + definition.Expression + operator + "?)"
		args = append(args, equalArgs...)
		args = append(args, keys[index])
	}
	equal := ""
	for index, entry := range selection {
		definition, _ := browseSortByID(entry.Field)
		equal += definition.Expression + "=? AND "
		args = append(args, keys[index])
		_ = entry
	}
	tie := ">"
	if selection[0].Direction == "desc" {
		tie = "<"
	}
	clause += " OR (" + equal + "e.entity_id" + tie + "?)"
	args = append(args, anchor)
	return "(" + clause + ")", args
}

// browseKeyValues types the cursor's stored key for comparison. A year is an
// integer column and SQLite orders text after every integer, so binding it as
// text would seek past the whole library.
func browseKeyValues(selection []BrowseSortSelection, keys []string) ([]any, bool) {
	if len(keys) != len(selection) {
		return nil, false
	}
	out := make([]any, 0, len(keys))
	for index, entry := range selection {
		switch entry.Field {
		case "year":
			year, err := strconv.Atoi(keys[index])
			if err != nil {
				return nil, false
			}
			out = append(out, year)
			continue
		case "duration", "communityRating":
			value, err := strconv.ParseFloat(keys[index], 64)
			if err != nil {
				return nil, false
			}
			out = append(out, value)
			continue
		}
		out = append(out, keys[index])
	}
	return out, true
}

func browseSortValue(row browseRow, field string) string {
	switch field {
	case "title":
		return row.sortKey
	case "added":
		if row.added.Valid {
			return row.added.String
		}
		return ""
	case "year":
		return strconv.Itoa(row.year)
	case "duration":
		return strconv.FormatFloat(row.duration, 'g', -1, 64)
	case "communityRating":
		return strconv.FormatFloat(row.rating, 'g', -1, 64)
	}
	return ""
}

func browseOrderSQL(profile string, selection []BrowseSortSelection) (string, []any) {
	parts := []string{}
	args := []any{}
	for _, entry := range selection {
		definition, ok := browseSortByID(entry.Field)
		if !ok {
			continue
		}
		if definition.Profile {
			args = append(args, profile)
		}
		direction := "ASC"
		if entry.Direction == "desc" {
			direction = "DESC"
		}
		parts = append(parts, definition.Expression+" "+direction)
	}
	// Ties break in the first sort's direction, so a descending order is the
	// ascending one reversed and the counted blocks serve both.
	if len(selection) > 0 && selection[0].Direction == "desc" {
		parts = append(parts, "e.entity_id DESC")
	} else {
		parts = append(parts, "e.entity_id ASC")
	}
	return strings.Join(parts, ","), args
}

// browseScopeSQL bounds every query to the authorized library and the pivot's
// entity kinds before a single user predicate is applied.
func browseScopeSQL(library string, pivot browsePivot) (string, []any) {
	members := entityKindMembers(pivot)
	args := []any{library}
	for _, kind := range members {
		parsed, err := compactcatalog.ParseKind(kind)
		if err != nil {
			return "0", nil
		}
		args = append(args, int(parsed))
	}
	// One kind is written as an equality rather than a one-element IN, because an
	// equality on the leading columns of `catalog_browse_title` lets the index
	// satisfy the ORDER BY as well as the filter, and `IN` does not. Every pivot
	// this server publishes names exactly one kind, so this is the ordinary case.
	if len(members) == 1 {
		return "e.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND e.kind=?", args
	}
	return "e.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND e.kind IN (" + placeholders(len(members)) + ")", args
}

func compactBrowseKindSQL(column string) string {
	return `CASE ` + column + ` WHEN 1 THEN 'movie' WHEN 2 THEN 'show' WHEN 3 THEN 'season' WHEN 4 THEN 'episode' WHEN 5 THEN 'artist' WHEN 6 THEN 'album' WHEN 7 THEN 'song' WHEN 8 THEN 'book' WHEN 9 THEN 'audiobook_file' WHEN 10 THEN 'collection' WHEN 11 THEN 'extra' WHEN 12 THEN 'disc' WHEN 13 THEN 'author' END`
}

func (s *Service) browseRevision(request BrowseRequest, pivot browsePivot, sorts []BrowseSortSelection) (string, error) {
	revision, err := s.ContentRevision(request.Library, request.Profile)
	if err != nil {
		return "", err
	}
	extra := ""
	if len(sorts) > 0 && sorts[0].Field == "forYou" {
		// For you also reads the profile's taste, the recommendation data and
		// the day (rotation): a page from another of any of them is another list.
		if extra, err = s.recBrowseFence(request.Profile); err != nil {
			return "", err
		}
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%s", request.Library, pivot.ID, request.ViewerFence, queryFingerprint(request.Query), canonicalSort(sorts), revision.Catalog, revision.Viewer, extra)))
	return "browse_" + hex.EncodeToString(digest[:]), nil
}

type browseRow struct {
	id, kind, title string
	entity          int64
	sortKey         string
	year            int
	added           sql.NullString
	duration        float64
	rating          float64
}

// BrowseEntities executes one expression query and projects the page.
func (s *Service) BrowseEntities(viewer Viewer, request BrowseRequest) (BrowseResult, error) {
	if !viewer.AllowsLibrary(request.Library) {
		return BrowseResult{}, sql.ErrNoRows
	}
	if err := s.prepareViewer(viewer); err != nil {
		return BrowseResult{}, err
	}
	request.Profile, request.ViewerFence, request.Restrictions, request.viewer = viewer.Profile, viewer.Fence, viewer.EffectiveRestrictions(), viewer
	if dbwork.Snapshot(s.Context()) == nil {
		var result BrowseResult
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var readErr error
			result, readErr = s.WithContext(ctx).BrowseEntities(viewer, request)
			return readErr
		})
		return result, err
	}
	out, rows, err := s.browseSelect(request)
	if err != nil {
		return out, err
	}
	if len(rows) > 0 {
		entries, err := s.browseProjection(request.Profile, rows)
		if err != nil {
			return out, err
		}
		if err = s.attachViewerRatings(request.Profile, entries); err != nil {
			return out, err
		}
		if len(request.Sort) > 0 && request.Sort[0].Field == "latestAired" {
			if err = s.nameLatestEpisodes(rows, entries); err != nil {
				return out, err
			}
		}
		out.Entries = entries
	}
	return out, nil
}

// browseSelect resolves the pivot, compiles the expression and returns the
// selected entity rows. Ranges are absolute (start/limit) and cursors are
// revision-fenced offsets over the same compiled clause, so a virtualized grid
// and a paged list can never disagree about membership.
func (s *Service) browseSelect(request BrowseRequest) (BrowseResult, []browseRow, error) {
	out := BrowseResult{Entries: []ContentEntry{}, PositionIndex: []BrowsePositionAnchor{}}
	lib, err := s.library(request.Library)
	if err != nil {
		return out, nil, err
	}
	if request.Pivot == "" {
		request.Pivot = defaultPivot(lib.Kind)
	}
	pivot, ok := pivotForKind(lib.Kind, request.Pivot)
	if !ok {
		return out, nil, ErrBrowsePivot
	}
	out.Pivot = pivot.ID
	if err := s.browseReady(request.Library); err != nil {
		return out, nil, err
	}
	if !pivot.Browsable {
		if pivot.Aggregate == "" {
			return out, nil, browseIssue("pivot", "this pivot is not browsable; read it through its own surface")
		}
		aggregate, err := s.browseAggregate(request, pivot)
		return aggregate, nil, err
	}
	sorts, err := resolveBrowseSorts(pivot, request.Sort)
	if err != nil {
		return out, nil, err
	}
	out.Applied = BrowseApplied{Query: request.Query, Sort: sorts, Seek: request.Seek}
	request.Limit = pageLimit(request.Limit)
	if request.Cursor != "" && request.Range != nil {
		return out, nil, browseIssue("range", "a direct range cannot include a cursor")
	}
	if request.Seek != nil && (len(request.Seek.Prefix) == 0 || len(request.Seek.Prefix) > 8) {
		return out, nil, browseIssue("seek.prefix", "seek prefix must be between 1 and 8 characters")
	}
	seekable := len(sorts) > 0 && sorts[0].Field == "title" && sorts[0].Direction == "asc"
	if request.Seek != nil && !seekable {
		return out, nil, browseIssue("seek", "seek requires the first sort to be title ascending")
	}
	if request.Query != nil {
		if request.Seek != nil {
			return out, nil, browseIssue("seek", "seek is unavailable for filtered queries")
		}
		if request.Range != nil && request.Range.AnchorID != "" {
			return out, nil, browseIssue("range.anchorId", "anchors are unavailable for filtered queries")
		}
	}

	where, args := browseScopeSQL(request.Library, pivot)
	compiler := &browseCompiler{profile: request.Profile}
	clause, err := compiler.compile(request.Query)
	if err != nil {
		return out, nil, err
	}
	where += " AND " + clause
	args = append(args, compiler.args...)
	// The viewer's content restriction is the last clause, after the authorized
	// library scope and the client's own query, so neither can widen it.
	if restriction, restrictionArgs := EntityRestrictionSQL("e.entity_id", request.Restrictions); restriction != "1" {
		where += " AND " + restriction
		args = append(args, restrictionArgs...)
	}
	order, orderArgs := browseOrderSQL(request.Profile, sorts)

	revision, err := s.browseRevision(request, pivot, sorts)
	if err != nil {
		return out, nil, err
	}
	out.PageInfo.Revision = revision
	stale := request.Range != nil && request.Range.Revision != "" && request.Range.Revision != revision
	if stale && request.Range.AnchorID == "" {
		return out, nil, ErrStaleContinuation
	}

	// The count, the letter index and the anchor rank used to be three passes over
	// the whole pivot. When the client has posted no expression of its own — which
	// is the library grid, the case that is actually large — all three are
	// answered from the counting read model in one small read.
	summary := browseSummary{}
	if request.Query == nil {
		if summary, err = s.browseSummarise(request.Library, entityKindMembers(pivot), request.Restrictions); err != nil {
			return out, nil, err
		}
	}
	// A posted query's total is exact: it is counted when the list opens (a pass
	// over the query's matches, the price of an ad hoc filter) and carried in
	// the signed cursor, so later pages don't count again.
	var total int
	counted := false
	if request.Query == nil {
		if summary.exact {
			total = summary.total
		} else if err = s.read().QueryRow(`SELECT count(*) FROM catalog_browse_rows e WHERE `+where, args...).Scan(&total); err != nil {
			return out, nil, err
		}
		counted = true
	}

	start := 0
	scope := cursorScope{Library: request.Library, Viewer: request.ViewerFence, Profile: request.Profile, View: "browse:" + pivot.ID, Sort: canonicalSort(sorts), Category: queryFingerprint(request.Query), Search: seekPrefix(request.Seek), Limit: request.Limit}
	if request.scope != nil {
		scope = *request.scope
	}
	expires := time.Now().Add(BrowseCursorTTLSecond * time.Second).Unix()
	seekKeys := []string{}
	seekAnchor := ""
	if request.Cursor != "" {
		cursor, err := s.decodeCursor(request.Cursor, scope)
		if err != nil {
			return out, nil, err
		}
		expires = cursor.Expires
		if start, err = strconv.Atoi(cursor.Value); err != nil || start < 0 {
			return out, nil, ErrCursor
		}
		seekKeys, seekAnchor = cursor.Keys, cursor.ID
		if !counted && cursor.Total > 0 {
			total, counted = cursor.Total, true
		}
	}
	if !counted {
		if cached, hit := s.cachedBrowseTotal(revision); hit {
			total = cached
		} else if err = s.read().QueryRow(`SELECT count(*) FROM catalog_browse_rows e WHERE `+where, args...).Scan(&total); err != nil {
			return out, nil, err
		} else {
			s.storeBrowseTotal(revision, total)
		}
	}
	out.PageInfo.Total = total
	if request.Range != nil {
		if request.Range.Start < 0 || len(request.Range.Revision) > 128 || len(request.Range.AnchorID) > 256 {
			return out, nil, browseIssue("range", "range identity is invalid")
		}
		start = request.Range.Start
	}

	// The letter index is only meaningful under an ascending title sort; it is
	// computed once per range so a scrub rail and the grid share one ranking.
	if request.Query == nil && seekable && (request.Cursor == "" || request.Seek != nil) {
		if summary.exact {
			if summary.letter != nil {
				out.PositionIndex = summary.letter
			}
		} else {
			anchors, err := s.browsePositionIndex(where, order, args, orderArgs)
			if err != nil {
				return out, nil, err
			}
			out.PositionIndex = anchors
		}
	} else if request.Query == nil && request.Cursor == "" && len(sorts) > 0 {
		// Every other sort publishes anchors too (M10): title descending by
		// letter, the rest by value buckets.
		var anchors []BrowsePositionAnchor
		var err error
		cached, hit := s.cachedAnchors(out.PageInfo.Revision)
		if hit {
			anchors = cached
		} else {
			anchors, err = s.compactBrowseSortAnchors(request, pivot, sorts[0])
		}
		if err != nil {
			return out, nil, err
		}
		if !hit {
			s.storeAnchors(out.PageInfo.Revision, anchors)
		}
		out.PositionIndex = anchors
	}
	if request.Seek != nil {
		start = total
		wanted := strings.ToUpper(request.Seek.Prefix[:1])
		for _, anchor := range out.PositionIndex {
			if anchor.Key == wanted {
				start = anchor.Index
				break
			}
		}
	}
	shape := pageShape{request: request, sorts: sorts, where: where, args: args, order: order, orderArgs: orderArgs,
		limit: request.Limit, total: total, summary: summary, seekKeys: seekKeys, seekAnchor: seekAnchor}
	if members := entityKindMembers(pivot); len(members) == 1 {
		parsed, parseErr := compactcatalog.ParseKind(members[0])
		if parseErr != nil {
			return out, nil, parseErr
		}
		shape.kind = int(parsed)
	}
	// A surviving anchor re-locates the viewport after a publication rather than
	// throwing the reader back to the top of the library: the anchor's rank in a
	// counted order is its block's running total plus a count inside the block.
	// Shapes without blocks answer ErrStaleContinuation without ranking
	// anything; a current revision ignores the anchor entirely.
	if request.Range != nil && request.Range.AnchorID != "" && stale {
		primary, _ := shape.primary()
		if !shape.positional() || primary.Profile || primary.Ordering < 0 {
			return out, nil, ErrStaleContinuation
		}
		rank, found, err := s.browseAnchorRank(shape, request.Range.AnchorID)
		if err != nil {
			return out, nil, err
		}
		if !found {
			return out, nil, ErrStaleContinuation
		}
		start = rank
	}
	if request.Query == nil && start > 0 && start >= total {
		start = max(0, total-1)
	}
	out.PageInfo.Start = start
	shape.start = start
	if request.Query != nil {
		shape.limit++ // One extra match answers hasMore even after the count reaches its cap.
	}
	selected, err := s.browsePage(shape)
	if err != nil {
		return out, nil, err
	}
	if request.Query != nil {
		out.PageInfo.HasMore = len(selected) > request.Limit
		if out.PageInfo.HasMore {
			selected = selected[:request.Limit]
		}
	} else {
		out.PageInfo.HasMore = start+len(selected) < total
	}
	if out.PageInfo.HasMore && request.Range == nil && len(selected) > 0 {
		last := selected[len(selected)-1]
		keys := make([]string, 0, len(sorts))
		for _, entry := range sorts {
			keys = append(keys, browseSortValue(last, entry.Field))
		}
		// The cursor carries the integer entity id; cursors are opaque and signed.
		out.PageInfo.NextCursor, err = s.encodeCursor(cursorValue{Scope: scope, Value: strconv.Itoa(start + len(selected)), ID: strconv.FormatInt(last.entity, 10), Keys: keys, Total: total, Expires: expires})
		if err != nil {
			return out, nil, err
		}
	}
	after, err := s.browseRevision(request, pivot, sorts)
	if err != nil {
		return out, nil, err
	}
	if after != revision {
		return out, nil, ErrStaleContinuation
	}
	return out, selected, nil
}
func seekPrefix(seek *BrowseSeek) string {
	if seek == nil {
		return ""
	}
	return strings.ToUpper(seek.Prefix)
}

func (s *Service) browsePositionIndex(where, order string, args, orderArgs []any) ([]BrowsePositionAnchor, error) {
	out := []BrowsePositionAnchor{}
	query := `SELECT letter,MIN(position) FROM (SELECT portico_letter(e.sort_key) AS letter,ROW_NUMBER() OVER (ORDER BY ` + order + `)-1 AS position FROM catalog_browse_rows e WHERE ` + where + `) GROUP BY letter ORDER BY MIN(position)`
	rows, err := s.read().Query(query, append(append([]any{}, orderArgs...), args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var anchor BrowsePositionAnchor
		if err = rows.Scan(&anchor.Key, &anchor.Index); err != nil {
			return nil, err
		}
		out = append(out, anchor)
	}
	return out, rows.Err()
}

// browseProjection keeps the existing ContentEntry shape: item-backed rows carry
// the full media projection, container rows carry a count and a destination.
func (s *Service) browseProjection(profile string, rows []browseRow) ([]ContentEntry, error) {
	out := make([]ContentEntry, 0, len(rows))
	itemIDs := []string{}
	containerIDs := []int64{}
	for _, row := range rows {
		if isItemEntity(row.kind) {
			itemIDs = append(itemIDs, row.id)
		} else {
			containerIDs = append(containerIDs, row.entity)
		}
	}
	media := map[string]Item{}
	if len(itemIDs) > 0 {
		items, err := s.mediaPage(profile, itemIDs, false)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			media[item.ID] = item
		}
	}
	counts := map[int64]int{}
	if len(containerIDs) > 0 {
		args := make([]any, 0, len(containerIDs))
		for _, id := range containerIDs {
			args = append(args, id)
		}
		rows, err := s.read().Query(`SELECT m.entity_id,count(*) FROM catalog_browse_memberships m WHERE m.entity_id IN (`+placeholders(len(containerIDs))+`) GROUP BY m.entity_id`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var count int
			if err = rows.Scan(&id, &count); err != nil {
				rows.Close()
				return nil, err
			}
			counts[id] = count
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	targets := make([]artworkTarget, 0, len(rows))
	for _, row := range rows {
		if !isItemEntity(row.kind) {
			targets = append(targets, artworkTarget{row.kind, row.id})
		}
	}
	artwork, err := s.resolveArtwork(targets)
	if err != nil {
		return nil, err
	}
	columns, err := s.browseListColumns(rows)
	if err != nil {
		return nil, err
	}
	artists, err := s.browseAlbumArtists(rows)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if isItemEntity(row.kind) {
			item, ok := media[row.id]
			if !ok {
				return nil, ErrStaleContinuation
			}
			entry := contentItem(item)
			if item.Year > 0 {
				year := item.Year
				entry.Year = &year
			}
			if row.rating > 0 {
				rating := row.rating
				entry.Rating = &rating
			}
			extra := columns[row.entity]
			entry.Resolution = extra.resolution
			if row.kind == "episode" {
				entry.AirDate = extra.released
			}
			out = append(out, entry)
			continue
		}
		count := counts[row.entity]
		art := artwork[artworkTarget{row.kind, row.id}]
		entry := ContentEntry{PosterURL: art.PosterURL, BackdropURL: art.BackdropURL, ID: row.id, Kind: row.kind, Title: row.title, Count: &count, AddedAt: nullableString(row.added), Navigation: &ContentNavigation{View: containerView(row.kind), EntityID: row.id}}
		if row.year > 0 {
			entry.Subtitle = strconv.Itoa(row.year)
			year := row.year
			entry.Year = &year
		}
		if row.rating > 0 {
			rating := row.rating
			entry.Rating = &rating
		}
		if artist, ok := artists[row.entity]; ok {
			entry.Artist = &artist
		}
		out = append(out, entry)
	}
	if err = s.nameEntryMakers(out); err != nil {
		return nil, err
	}
	if err = s.countEntryPlays(profile, out); err != nil {
		return nil, err
	}
	return out, nil
}

// nameLatestEpisodes says, for each show on a Recently aired page, which
// episode aired last and how many aired with it. It reads the episodes of the
// page's shows only: the cost follows the page, not the library.
func (s *Service) nameLatestEpisodes(rows []browseRow, entries []ContentEntry) error {
	index := map[int64]int{}
	ids := []int64{}
	for i, row := range rows {
		if row.kind == "show" && i < len(entries) {
			index[row.entity] = i
			ids = append(ids, row.entity)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	raw, _ := json.Marshal(ids)
	found, err := s.read().Query(`SELECT ep.show_id,d.release_date,se.number,ep.number,ee.title
	 FROM json_each(?) j CROSS JOIN catalog_episodes ep ON ep.show_id=j.value
	 JOIN catalog_item_details d ON d.entity_id=ep.entity_id AND COALESCE(d.release_date,'')<>''
	 JOIN catalog_entities ee ON ee.id=ep.entity_id LEFT JOIN catalog_seasons se ON se.entity_id=ep.season_id`, string(raw))
	if err != nil {
		return err
	}
	defer found.Close()
	latest := map[int64]*LatestEpisode{}
	for found.Next() {
		var show int64
		var date, title string
		var season sql.NullInt64
		var number int
		if err = found.Scan(&show, &date, &season, &number, &title); err != nil {
			return err
		}
		current := latest[show]
		if current != nil && date < current.AirDate {
			continue
		}
		if current == nil || date > current.AirDate {
			current = &LatestEpisode{AirDate: date}
			latest[show] = current
		}
		current.Count++
		// Of the episodes that aired that day, the card names the last in order.
		seasonNumber := -1
		if season.Valid {
			seasonNumber = int(season.Int64)
		}
		named := -1
		if current.SeasonNumber != nil {
			named = *current.SeasonNumber
		}
		if current.Count == 1 || seasonNumber > named || seasonNumber == named && number > current.EpisodeNumber {
			current.EpisodeNumber, current.Title = number, title
			current.SeasonNumber = nil
			if season.Valid {
				n := seasonNumber
				current.SeasonNumber = &n
			}
		}
	}
	if err = found.Err(); err != nil {
		return err
	}
	for show, episode := range latest {
		entries[index[show]].LatestEpisode = episode
	}
	return nil
}

// browseAlbumArtists names the artist of each album row on the page, so an
// album card always says whose it is.
func (s *Service) browseAlbumArtists(rows []browseRow) (map[int64]ContentEntityRef, error) {
	out := map[int64]ContentEntityRef{}
	args := []any{}
	for _, row := range rows {
		if row.kind == "album" {
			args = append(args, row.entity)
		}
	}
	if len(args) == 0 {
		return out, nil
	}
	found, err := s.read().Query(`SELECT a.entity_id,pid(ar.public_id),ar.title FROM catalog_albums a JOIN catalog_entities ar ON ar.id=a.artist_id WHERE a.entity_id IN (`+placeholders(len(args))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer found.Close()
	for found.Next() {
		var album int64
		var ref ContentEntityRef
		if err = found.Scan(&album, &ref.ID, &ref.Name); err != nil {
			return nil, err
		}
		if ref.Name != "" {
			out[album] = ref
		}
	}
	return out, found.Err()
}

// browseListColumn is what a list view shows beside an item that the browse
// row does not carry: its best file's picture class and its release date.
type browseListColumn struct{ resolution, released string }

// browseListColumns reads those for the page's item rows: two lookups by the
// page's ids, whatever the library's size.
func (s *Service) browseListColumns(rows []browseRow) (map[int64]browseListColumn, error) {
	out := map[int64]browseListColumn{}
	args := []any{}
	for _, row := range rows {
		if isItemEntity(row.kind) {
			args = append(args, row.entity)
		}
	}
	if len(args) == 0 {
		return out, nil
	}
	in := placeholders(len(args))
	heights, err := s.read().Query(`SELECT ia.entity_id,max(ast.height) FROM catalog_asset_links ia JOIN catalog_assets ast ON ast.id=ia.asset_id WHERE ia.entity_id IN (`+in+`) GROUP BY ia.entity_id`, args...)
	if err != nil {
		return nil, err
	}
	for heights.Next() {
		var entity int64
		var height sql.NullInt64
		if err = heights.Scan(&entity, &height); err != nil {
			heights.Close()
			return nil, err
		}
		if height.Valid && height.Int64 > 0 {
			column := out[entity]
			switch {
			case height.Int64 >= 2000:
				column.resolution = "4k"
			case height.Int64 >= 1000:
				column.resolution = "1080p"
			case height.Int64 >= 700:
				column.resolution = "720p"
			default:
				column.resolution = "sd"
			}
			out[entity] = column
		}
	}
	err = heights.Err()
	heights.Close()
	if err != nil {
		return nil, err
	}
	dates, err := s.read().Query(`SELECT entity_id,release_date FROM catalog_item_details WHERE entity_id IN (`+in+`) AND release_date<>''`, args...)
	if err != nil {
		return nil, err
	}
	defer dates.Close()
	for dates.Next() {
		var entity int64
		var released string
		if err = dates.Scan(&entity, &released); err != nil {
			return nil, err
		}
		column := out[entity]
		column.released = released
		out[entity] = column
	}
	return out, dates.Err()
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copied := value.String
	return &copied
}

func isItemEntity(kind string) bool {
	switch kind {
	case "show", "season", "artist", "album", "book", "author", "collection":
		return false
	}
	return true
}

func containerView(kind string) string {
	switch kind {
	case "author":
		return "author"
	default:
		return kind
	}
}

// browseAggregate projects a facet as a navigable grid. Decades, genres and
// series have no entity rows of their own; they are counted groupings that lead
// back into an entity pivot with a preset predicate.
func (s *Service) browseAggregate(request BrowseRequest, pivot browsePivot) (BrowseResult, error) {
	out := BrowseResult{Pivot: pivot.ID, Entries: []ContentEntry{}, PositionIndex: []BrowsePositionAnchor{}, Applied: BrowseApplied{Sort: pivot.DefaultSort}}
	if request.Query != nil {
		return out, browseIssue("query", "aggregate pivots do not accept an expression query")
	}
	revision, err := s.browseRevision(request, pivot, pivot.DefaultSort)
	if err != nil {
		return out, err
	}
	out.PageInfo.Revision = revision
	values, err := s.Facets(request.viewer, FacetRequest{Library: request.Library, Profile: request.Profile, Field: pivot.Aggregate, Limit: browseFacetLimit})
	if err != nil {
		return out, err
	}
	for _, value := range values.Values {
		count := value.Count
		out.Entries = append(out.Entries, ContentEntry{ID: pivot.Aggregate + ":" + value.Value, Kind: pivot.EntityKinds[0], Title: value.Label, Count: &count, Navigation: &ContentNavigation{View: "browse", Category: pivot.Aggregate + ":" + value.Value}})
	}
	out.PageInfo.Total = len(out.Entries)
	return out, nil
}
