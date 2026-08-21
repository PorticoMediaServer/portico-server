package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/browsecontract"
	"github.com/PorticoMediaServer/portico-server/internal/catalogkind"
)

type BrowseLibraryRequest struct {
	Pivot        string             `json:"pivot"`
	Query        *BrowseExpression  `json:"query,omitempty"`
	Sort         []BrowseSort       `json:"sort,omitempty"`
	Presentation BrowsePresentation `json:"presentation,omitempty"`
	Seek         *BrowseSeek        `json:"seek,omitempty"`
	Cursor       string             `json:"cursor,omitempty"`
	Limit        int                `json:"limit,omitempty"`
}

type BrowseSeek struct {
	Prefix string `json:"prefix"`
}

type BrowseExpression struct {
	All      []BrowseExpression `json:"all,omitempty"`
	Any      []BrowseExpression `json:"any,omitempty"`
	Not      *BrowseExpression  `json:"not,omitempty"`
	Field    string             `json:"field,omitempty"`
	Operator string             `json:"operator,omitempty"`
	Value    json.RawMessage    `json:"value,omitempty"`
}

type BrowseSort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type BrowsePresentation struct {
	Fields []string `json:"fields,omitempty"`
}

type CursorPageInfo struct {
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
	Total      *int    `json:"total,omitempty"`
}

type MediaCard struct {
	ID              string                   `json:"id"`
	LibraryID       string                   `json:"libraryId"`
	LibraryName     string                   `json:"libraryName,omitempty"`
	Counts          *MediaHierarchyCounts    `json:"counts,omitempty"`
	EntityKind      string                   `json:"entityKind"`
	Title           string                   `json:"title"`
	Subtitle        string                   `json:"subtitle,omitempty"`
	Summary         string                   `json:"summary,omitempty"`
	Year            int                      `json:"year,omitempty"`
	DurationSeconds int                      `json:"durationSeconds,omitempty"`
	Artwork         ImageSet                 `json:"artwork"`
	UserState       MediaCardUserState       `json:"userState"`
	Availability    MediaAvailabilitySummary `json:"availability"`
	Actions         []string                 `json:"actions"`
	Fields          map[string]any           `json:"fields,omitempty"`
}

// populateMediaHierarchyProjectionContext enriches a bounded result window in
// two aggregate queries. Totals are computed from the catalog, never from the
// deliberately truncated inline Children projection.
func (s *Server) populateMediaHierarchyProjectionContext(ctx context.Context, items []MediaItem) error {
	if len(items) == 0 {
		return nil
	}
	byID := make(map[string]*MediaItem, len(items))
	libraryIDs := make([]string, 0, len(items))
	seenLibraries := map[string]bool{}
	ids := make([]string, 0, len(items))
	for index := range items {
		item := &items[index]
		if item.ID != "" {
			byID[item.ID] = item
			ids = append(ids, item.ID)
		}
		if item.LibraryID != "" && !seenLibraries[item.LibraryID] {
			seenLibraries[item.LibraryID] = true
			libraryIDs = append(libraryIDs, item.LibraryID)
		}
	}
	if len(libraryIDs) > 0 {
		args := make([]any, len(libraryIDs))
		for index := range libraryIDs {
			args[index] = libraryIDs[index]
		}
		rows, err := s.queryUserRead(ctx, `SELECT id, name FROM libraries WHERE id IN (`+sqlPlaceholders(len(args))+`)`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return err
			}
			for index := range items {
				if items[index].LibraryID == id {
					items[index].LibraryName = name
				}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, len(ids))
	values := make([]string, len(ids))
	for index := range ids {
		args[index] = ids[index]
		values[index] = "(?)"
	}
	rows, err := s.queryUserRead(ctx, `
		WITH RECURSIVE requested(id) AS (VALUES `+strings.Join(values, ",")+`),
		tree(root_id, id, parent_id, type, depth, visited) AS (
			SELECT r.id, m.id, m.parent_id, lower(m.type), 0, json_array(m.id) FROM requested r JOIN media_items m ON m.id = r.id
			UNION ALL
			SELECT tree.root_id, child.id, child.parent_id, lower(child.type), tree.depth + 1,
				json_insert(tree.visited, '$[#]', child.id)
			FROM tree JOIN media_items child ON child.parent_id = tree.id
			WHERE NOT EXISTS (SELECT 1 FROM json_each(tree.visited) WHERE json_each.value = child.id)
		)
		SELECT tree.root_id,
			SUM(CASE WHEN depth = 1 AND type = 'season' THEN 1 ELSE 0 END),
			SUM(CASE WHEN depth > 0 AND type = 'episode' THEN 1 ELSE 0 END),
			SUM(CASE WHEN depth = 1 AND type IN ('album', 'release') THEN 1 ELSE 0 END),
			SUM(CASE WHEN depth > 0 AND type = 'track' THEN 1 ELSE 0 END),
			SUM(CASE WHEN depth > 0 AND type = 'audiobook' THEN 1 ELSE 0 END),
			SUM(CASE WHEN depth > 0 THEN 1 ELSE 0 END),
			(SELECT COUNT(*) FROM media_chapters chapter WHERE chapter.media_id = tree.root_id)
		FROM tree GROUP BY tree.root_id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var seasons, episodes, releases, tracks, books, descendants, chapters int
		if err := rows.Scan(&id, &seasons, &episodes, &releases, &tracks, &books, &descendants, &chapters); err != nil {
			return err
		}
		item := byID[id]
		if item == nil {
			continue
		}
		count := func(value int) *int { result := value; return &result }
		switch canonicalEntityKind(item.Type) {
		case "show":
			item.Counts = &MediaHierarchyCounts{SeasonCount: count(seasons), EpisodeCount: count(episodes), ItemCount: count(descendants)}
		case "season":
			item.Counts = &MediaHierarchyCounts{EpisodeCount: count(episodes), ItemCount: count(descendants)}
		case "artist":
			item.Counts = &MediaHierarchyCounts{ReleaseCount: count(releases), TrackCount: count(tracks), ItemCount: count(descendants)}
		case "album", "release":
			item.Counts = &MediaHierarchyCounts{TrackCount: count(tracks), ItemCount: count(descendants)}
		case "author", "audiobook-series":
			item.Counts = &MediaHierarchyCounts{BookCount: count(books), ItemCount: count(descendants)}
		case "book":
			item.Counts = &MediaHierarchyCounts{ChapterCount: count(chapters)}
		}
	}
	return rows.Err()
}

type MediaCardUserState struct {
	Watchlisted     bool   `json:"watchlisted"`
	Favorite        bool   `json:"favorite"`
	Watched         bool   `json:"watched"`
	ProgressSeconds int    `json:"progressSeconds"`
	LastPlayedAt    string `json:"lastPlayedAt,omitempty"`
}

type MediaAvailabilitySummary struct {
	Status           string `json:"status"`
	FileCount        int    `json:"fileCount"`
	MissingFileCount int    `json:"missingFileCount"`
}

type BrowseApplied struct {
	Pivot              string       `json:"pivot"`
	Sort               []BrowseSort `json:"sort"`
	PresentationFields []string     `json:"presentationFields"`
	Seek               *BrowseSeek  `json:"seek,omitempty"`
}

type BrowseLibraryResponse struct {
	Items    []MediaCard    `json:"items"`
	PageInfo CursorPageInfo `json:"pageInfo"`
	Applied  BrowseApplied  `json:"applied"`
}

type BrowseValidationIssue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type browseValidationProblems struct {
	Issues []BrowseValidationIssue
}

func (e *browseValidationProblems) Error() string {
	if len(e.Issues) == 0 {
		return "browse request is invalid"
	}
	return e.Issues[0].Message
}

type browseSortSpec struct {
	Field     string
	Direction string
	SQL       string
	Value     func(MediaItem) any
	Numeric   bool
}

type browseCursorPayload struct {
	Values []any `json:"values"`
}

type browseCacheEntry struct {
	response      BrowseLibraryResponse
	sourceProfile string
	expiresAt     time.Time
}

type browseInFlightCall struct {
	done          chan struct{}
	response      BrowseLibraryResponse
	sourceProfile string
	err           error
}

func (s *Server) handleLibraryBrowse(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	libraryID, ok := libraryIDFromCanonicalBrowsePath(r.URL.Path, "browse")
	if !ok {
		writeProductError(w, http.StatusNotFound, "not_found", "Library browse resource was not found.")
		return
	}
	request, ok := decodeBrowseLibraryRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), browseRequestTimeout)
	defer cancel()
	library, kind, ok := s.authorizedCanonicalLibrary(ctx, user, libraryID, w)
	if !ok {
		return
	}
	response, err := s.browseLibraryContext(ctx, user, library, kind, request, time.Now().UTC())
	if err != nil {
		var validation *browseValidationProblems
		switch {
		case errors.As(err, &validation):
			writeBrowseValidationProblem(w, validation.Issues)
		case errors.Is(err, errCursorExpired):
			writeProductError(w, http.StatusBadRequest, "cursor_expired", "The browse cursor expired. Restart from the first page.")
		case errors.Is(err, errInvalidCursor):
			writeProductError(w, http.StatusBadRequest, "invalid_cursor", "The browse cursor is invalid for this request.")
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			w.Header().Set("Retry-After", "1")
			writeProductError(w, http.StatusServiceUnavailable, "library_browse_timeout", "Library browsing exceeded the foreground request budget. Try a narrower view.")
		default:
			writeProductError(w, http.StatusInternalServerError, "library_browse_failed", "Unable to browse this library.")
		}
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=10, stale-while-revalidate=30")
	writeJSON(w, http.StatusOK, response)
}

func decodeBrowseLibraryRequest(w http.ResponseWriter, r *http.Request) (BrowseLibraryRequest, bool) {
	defer r.Body.Close()
	var request BrowseLibraryRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeProductError(w, http.StatusBadRequest, "invalid_browse_request", "Browse request must be valid JSON with known fields.")
		return BrowseLibraryRequest{}, false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeProductError(w, http.StatusBadRequest, "invalid_browse_request", "Browse request must contain one JSON object.")
		return BrowseLibraryRequest{}, false
	}
	return request, true
}

func (s *Server) browseLibraryContext(ctx context.Context, user User, library Library, kind ProductLibraryKind, request BrowseLibraryRequest, now time.Time) (BrowseLibraryResponse, error) {
	key, cacheable := s.canonicalBrowseCacheKeyContext(ctx, user, library.ID, request)
	if key == "" {
		return s.loadBrowseLibraryContext(ctx, user, library, kind, request, now)
	}
	s.browseCacheMu.Lock()
	if entry, ok := s.browseCache[key]; ok && time.Now().Before(entry.expiresAt) {
		response := entry.response
		s.browseCacheMu.Unlock()
		return s.rebindBrowseResponseCursor(response, library.ID, request, entry.sourceProfile, viewerProfileID(user), now)
	}
	if call, ok := s.browseInFlight[key]; ok {
		s.browseCacheMu.Unlock()
		select {
		case <-call.done:
			if call.err != nil {
				return BrowseLibraryResponse{}, call.err
			}
			return s.rebindBrowseResponseCursor(call.response, library.ID, request, call.sourceProfile, viewerProfileID(user), now)
		case <-ctx.Done():
			return BrowseLibraryResponse{}, ctx.Err()
		}
	}
	if s.browseInFlight == nil {
		s.browseInFlight = map[string]*browseInFlightCall{}
	}
	call := &browseInFlightCall{done: make(chan struct{})}
	s.browseInFlight[key] = call
	s.browseCacheMu.Unlock()

	response, err := s.loadBrowseLibraryContext(ctx, user, library, kind, request, now)
	s.browseCacheMu.Lock()
	call.response, call.err = response, err
	call.sourceProfile = viewerProfileID(user)
	if err == nil && cacheable {
		if s.browseCache == nil {
			s.browseCache = map[string]browseCacheEntry{}
		}
		for cachedKey, entry := range s.browseCache {
			if !now.Before(entry.expiresAt) {
				delete(s.browseCache, cachedKey)
			}
		}
		for len(s.browseCache) >= 1024 {
			for cachedKey := range s.browseCache {
				delete(s.browseCache, cachedKey)
				break
			}
		}
		ttl := 10 * time.Second
		if strings.HasPrefix(key, "empty-state\x00") {
			// This projection contains no viewer state and is invalidated with
			// catalogue/authz mutations, so it can bridge successive viewer waves.
			ttl = 60 * time.Second
		}
		s.browseCache[key] = browseCacheEntry{response: response, sourceProfile: viewerProfileID(user), expiresAt: time.Now().Add(ttl)}
	}
	delete(s.browseInFlight, key)
	close(call.done)
	s.browseCacheMu.Unlock()
	return response, err
}

// canonicalBrowseCacheKeyContext shares a first-page catalogue projection only
// when the viewer has no media state at all. The authorization fingerprint
// includes library/content policy and response-shaping permissions; profiles
// with any personal state retain a profile-private cache key.
func (s *Server) canonicalBrowseCacheKeyContext(ctx context.Context, user User, libraryID string, request BrowseLibraryRequest) (string, bool) {
	key, cacheable := canonicalBrowseCacheKey(user, libraryID, request)
	if key == "" || !cacheable {
		return key, cacheable
	}
	profileID := viewerProfileID(user)
	hasState, err := s.profileHasMediaStateContext(ctx, profileID)
	if err != nil || hasState {
		return key, cacheable
	}
	request = normalizeBrowseRequest(request)
	body, err := json.Marshal(request)
	if err != nil {
		return key, cacheable
	}
	digest := sha256.Sum256(body)
	return strings.Join([]string{emptyMediaStateCacheScope(user, false), strings.TrimSpace(libraryID), hex.EncodeToString(digest[:])}, "\x00"), cacheable
}

func (s *Server) rebindBrowseResponseCursor(response BrowseLibraryResponse, libraryID string, request BrowseLibraryRequest, sourceProfile, targetProfile string, now time.Time) (BrowseLibraryResponse, error) {
	if response.PageInfo.NextCursor == nil || sourceProfile == targetProfile {
		return response, nil
	}
	scope, err := browseCursorScope(libraryID, normalizeBrowseRequest(request))
	if err != nil {
		return BrowseLibraryResponse{}, err
	}
	var payload browseCursorPayload
	if err := s.decodeContractCursor(*response.PageInfo.NextCursor, scope, sourceProfile, &payload, now); err != nil {
		return BrowseLibraryResponse{}, err
	}
	token, err := s.encodeContractCursor(scope, targetProfile, payload, now)
	if err != nil {
		return BrowseLibraryResponse{}, err
	}
	response.PageInfo.NextCursor = &token
	return response, nil
}

func canonicalBrowseCacheKey(user User, libraryID string, request BrowseLibraryRequest) (string, bool) {
	request = normalizeBrowseRequest(request)
	body, err := json.Marshal(request)
	if err != nil || strings.TrimSpace(user.ID) == "" || strings.TrimSpace(libraryID) == "" {
		return "", false
	}
	digest := sha256.Sum256(body)
	return strings.Join([]string{homeCacheKey(user), strings.TrimSpace(libraryID), hex.EncodeToString(digest[:])}, "\x00"), request.Cursor == ""
}

func (s *Server) invalidateCanonicalBrowseCache() {
	s.browseCacheMu.Lock()
	if len(s.browseCache) > 0 {
		s.browseCache = map[string]browseCacheEntry{}
	}
	s.browseCacheMu.Unlock()
}

func (s *Server) loadBrowseLibraryContext(ctx context.Context, user User, library Library, kind ProductLibraryKind, request BrowseLibraryRequest, now time.Time) (BrowseLibraryResponse, error) {
	request = normalizeBrowseRequest(request)
	pivot, exists := browsePivotByID(kind, request.Pivot)
	if !exists {
		return BrowseLibraryResponse{}, &browseValidationProblems{Issues: []BrowseValidationIssue{{Field: "pivot", Code: "unsupported_pivot", Message: "Pivot is not available for this library kind."}}}
	}
	if !pivot.BrowseSupported {
		return BrowseLibraryResponse{}, &browseValidationProblems{Issues: []BrowseValidationIssue{{Field: "pivot", Code: "separate_resource", Message: "This pivot is loaded from its declared resource, not the media browse endpoint."}}}
	}
	if request.Limit == 0 {
		request.Limit = browseDefaultLimit
	}
	if request.Limit < 1 || request.Limit > browseMaximumLimit {
		return BrowseLibraryResponse{}, &browseValidationProblems{Issues: []BrowseValidationIssue{{Field: "limit", Code: "out_of_range", Message: fmt.Sprintf("Limit must be between 1 and %d.", browseMaximumLimit)}}}
	}
	if len(request.Sort) == 0 {
		request.Sort = append([]BrowseSort(nil), pivot.DefaultSort...)
	}
	sortSpecs, issues := compileBrowseSorts(request.Sort, pivot)
	if len(issues) > 0 {
		return BrowseLibraryResponse{}, &browseValidationProblems{Issues: issues}
	}
	presentation, issues := validateBrowsePresentation(request.Presentation.Fields)
	if len(issues) > 0 {
		return BrowseLibraryResponse{}, &browseValidationProblems{Issues: issues}
	}
	request.Presentation.Fields = presentation
	where, args, issues := compileBrowseWhere(library.ID, kind.ID, pivot, request.Query)
	if len(issues) > 0 {
		return BrowseLibraryResponse{}, &browseValidationProblems{Issues: issues}
	}
	where, args, issues = applyBrowseSeek(where, args, request, sortSpecs)
	if len(issues) > 0 {
		return BrowseLibraryResponse{}, &browseValidationProblems{Issues: issues}
	}
	scope, err := browseCursorScope(library.ID, request)
	if err != nil {
		return BrowseLibraryResponse{}, err
	}
	if strings.TrimSpace(request.Cursor) != "" {
		var cursor browseCursorPayload
		if err := s.decodeContractCursor(request.Cursor, scope, viewerProfileID(user), &cursor, now); err != nil {
			return BrowseLibraryResponse{}, err
		}
		if len(cursor.Values) != len(sortSpecs)+1 {
			return BrowseLibraryResponse{}, fmt.Errorf("%w: sort key count does not match", errInvalidCursor)
		}
		values, err := validateBrowseCursorValues(cursor.Values, sortSpecs)
		if err != nil {
			return BrowseLibraryResponse{}, err
		}
		where, args = appendBrowseKeysetPredicate(where, args, sortSpecs, values)
	}
	order := browseOrderSQL(sortSpecs)
	queryArgs := append(append([]any{}, args...), request.Limit+1)
	items, err := s.queryMediaListItemsContext(ctx, viewerProfileID(user), where+" ORDER BY "+order+" LIMIT ?", queryArgs)
	if err != nil {
		return BrowseLibraryResponse{}, err
	}
	hasMore := len(items) > request.Limit
	if hasMore {
		items = items[:request.Limit]
	}
	var nextCursor *string
	if hasMore && len(items) > 0 {
		values := browseSortValues(items[len(items)-1], sortSpecs)
		token, err := s.encodeContractCursor(scope, viewerProfileID(user), browseCursorPayload{Values: values}, now)
		if err != nil {
			return BrowseLibraryResponse{}, err
		}
		nextCursor = &token
	}
	cards := make([]MediaCard, 0, len(items))
	for _, item := range items {
		cards = append(cards, mediaCardForBrowse(item, user, presentation))
	}
	return BrowseLibraryResponse{
		Items:    cards,
		PageInfo: CursorPageInfo{NextCursor: nextCursor, HasMore: hasMore},
		Applied:  BrowseApplied{Pivot: pivot.ID, Sort: request.Sort, PresentationFields: presentation, Seek: request.Seek},
	}, nil
}

func normalizeBrowseRequest(request BrowseLibraryRequest) BrowseLibraryRequest {
	request.Pivot = strings.TrimSpace(request.Pivot)
	request.Cursor = strings.TrimSpace(request.Cursor)
	if request.Seek != nil {
		request.Seek.Prefix = strings.ToUpper(strings.TrimSpace(request.Seek.Prefix))
	}
	for index := range request.Sort {
		request.Sort[index].Field = strings.TrimSpace(request.Sort[index].Field)
		request.Sort[index].Direction = strings.ToLower(strings.TrimSpace(request.Sort[index].Direction))
	}
	for index := range request.Presentation.Fields {
		request.Presentation.Fields[index] = strings.TrimSpace(request.Presentation.Fields[index])
	}
	request.Query = normalizeBrowseExpression(request.Query)
	return request
}

func applyBrowseSeek(where string, args []any, request BrowseLibraryRequest, specs []browseSortSpec) (string, []any, []BrowseValidationIssue) {
	if request.Seek == nil {
		return where, args, nil
	}
	prefix := strings.ToUpper(strings.TrimSpace(request.Seek.Prefix))
	if len(specs) == 0 || (request.Sort[0].Field != "title" && request.Sort[0].Field != "sortTitle") || specs[0].Direction != "ASC" {
		return where, args, []BrowseValidationIssue{{Field: "seek", Code: "seek_requires_title_sort", Message: "Alphabetical seek requires ascending Title or Sort title order."}}
	}
	if prefix == "#" {
		return where, args, nil
	}
	if len(prefix) != 1 || prefix[0] < 'A' || prefix[0] > 'Z' {
		return where, args, []BrowseValidationIssue{{Field: "seek.prefix", Code: "invalid_seek_prefix", Message: "Alphabetical seek must be # or one letter from A to Z."}}
	}
	return where + " AND m.sort_title COLLATE NOCASE >= ?", append(args, prefix), nil
}

func normalizeBrowseExpression(expression *BrowseExpression) *BrowseExpression {
	if expression == nil {
		return nil
	}
	normalized := *expression
	normalized.Field = strings.TrimSpace(normalized.Field)
	normalized.Operator = strings.ToLower(strings.TrimSpace(normalized.Operator))
	if normalized.Not != nil {
		normalized.Not = normalizeBrowseExpression(normalized.Not)
	}
	for index := range normalized.All {
		child := normalizeBrowseExpression(&normalized.All[index])
		normalized.All[index] = *child
	}
	for index := range normalized.Any {
		child := normalizeBrowseExpression(&normalized.Any[index])
		normalized.Any[index] = *child
	}
	canonicalSortExpressions(normalized.All)
	canonicalSortExpressions(normalized.Any)
	return &normalized
}

func canonicalSortExpressions(expressions []BrowseExpression) {
	sort.SliceStable(expressions, func(i, j int) bool {
		left, _ := json.Marshal(expressions[i])
		right, _ := json.Marshal(expressions[j])
		return bytes.Compare(left, right) < 0
	})
}

func browseCursorScope(libraryID string, request BrowseLibraryRequest) (string, error) {
	request.Cursor = ""
	request.Limit = 0
	request.Presentation.Fields = append([]string(nil), request.Presentation.Fields...)
	sort.Strings(request.Presentation.Fields)
	body, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal normalized browse request: %w", err)
	}
	digest := sha256.Sum256(body)
	return "library-browse:" + strings.TrimSpace(libraryID) + ":" + hex.EncodeToString(digest[:]), nil
}

func compileBrowseWhere(libraryID, libraryKind string, pivot BrowsePivotCapability, query *BrowseExpression) (string, []any, []BrowseValidationIssue) {
	where := "WHERE m.library_id = ?"
	args := []any{libraryID}
	pivotSQL, pivotArgs := browsePivotPredicate(libraryKind, pivot.ID)
	if pivotSQL == "" {
		return where, args, []BrowseValidationIssue{{Field: "pivot", Code: "unsupported_pivot", Message: "Pivot cannot be represented by the media browse projection."}}
	}
	where += " AND (" + pivotSQL + ")"
	args = append(args, pivotArgs...)
	if query == nil {
		return where, args, nil
	}
	allowedFields := make(map[string]struct{})
	for _, capability := range browseFieldsForPivot(pivot) {
		allowedFields[capability.ID] = struct{}{}
	}
	rawQuery, err := json.Marshal(query)
	if err != nil {
		return where, args, []BrowseValidationIssue{{Field: "query", Code: "invalid_query_contract", Message: "Browse query could not be represented canonically."}}
	}
	if err := browsecontract.ValidateQuery(rawQuery, browsecontract.ValidationOptions{AllowedFields: allowedFields}); err != nil {
		var contractError *browsecontract.ValidationError
		if errors.As(err, &contractError) {
			return where, args, []BrowseValidationIssue{{Field: contractError.Path, Code: "invalid_query_contract", Message: contractError.Message}}
		}
		return where, args, []BrowseValidationIssue{{Field: "query", Code: "invalid_query_contract", Message: "Browse query is not valid."}}
	}
	if issues := validateBrowseQueryCapabilities(*query, pivot, "query"); len(issues) > 0 {
		return where, args, issues
	}
	clauses := 0
	querySQL, queryArgs, issues := compileBrowseExpression(*query, "query", 1, &clauses)
	if len(issues) > 0 {
		return where, args, issues
	}
	where += " AND (" + querySQL + ")"
	args = append(args, queryArgs...)
	return where, args, nil
}

func validateBrowseQueryCapabilities(expression BrowseExpression, pivot BrowsePivotCapability, path string) []BrowseValidationIssue {
	if expression.Not != nil {
		return validateBrowseQueryCapabilities(*expression.Not, pivot, path+".not")
	}
	if expression.All != nil || expression.Any != nil {
		children, field := expression.All, ".all"
		if expression.Any != nil {
			children, field = expression.Any, ".any"
		}
		for index, child := range children {
			if issues := validateBrowseQueryCapabilities(child, pivot, fmt.Sprintf("%s%s[%d]", path, field, index)); len(issues) > 0 {
				return issues
			}
		}
		return nil
	}
	field := strings.TrimSpace(expression.Field)
	if field == "" {
		return nil
	}
	allowed := map[string]bool{}
	for _, capability := range browseFieldsForPivot(pivot) {
		allowed[capability.ID] = true
	}
	if !allowed[field] {
		return []BrowseValidationIssue{{Field: path + ".field", Code: "unsupported_field_for_pivot", Message: "Browse field is not available for this pivot."}}
	}
	if field != "entityKind" {
		return nil
	}
	values, issue := browseStringsForOperator(expression.Value, strings.ToLower(strings.TrimSpace(expression.Operator)), path+".value")
	if issue != nil {
		return nil // The canonical predicate compiler returns the more precise value error.
	}
	for _, value := range values {
		if !containsString(pivot.EntityKinds, value) {
			return []BrowseValidationIssue{{Field: path + ".value", Code: "unsupported_value_for_pivot", Message: "Media type is not available for this pivot."}}
		}
	}
	return nil
}

func browsePivotPredicate(libraryKind, pivot string) (string, []any) {
	switch pivot {
	case "movies":
		return "m.type = 'movie' AND m.parent_id IS NULL", nil
	case "shows":
		if libraryKind == "anime" {
			return "m.type = 'anime' AND m.parent_id IS NULL", nil
		}
		if libraryKind == "recorded-tv" {
			return "m.type = 'recording'", nil
		}
		return "m.type = 'show' AND m.parent_id IS NULL", nil
	case "episodes":
		return "m.type = 'episode'", nil
	case "artists":
		return "m.type = 'artist' AND m.parent_id IS NULL", nil
	case "albums":
		return "m.type = 'album'", nil
	case "tracks":
		return "m.type = 'track'", nil
	case "books":
		return "m.type = 'audiobook'", nil
	case "recordings":
		return "m.type = 'recording'", nil
	default:
		return "", nil
	}
}

func compileBrowseExpression(expression BrowseExpression, path string, depth int, clauses *int) (string, []any, []BrowseValidationIssue) {
	if depth > browsecontract.MaximumDepth {
		return "", nil, []BrowseValidationIssue{{Field: path, Code: "maximum_depth_exceeded", Message: fmt.Sprintf("Browse query nesting cannot exceed %d levels.", browsecontract.MaximumDepth)}}
	}
	hasAll := expression.All != nil
	hasAny := expression.Any != nil
	hasNot := expression.Not != nil
	hasLeaf := expression.Field != "" || expression.Operator != "" || expression.Value != nil
	shapeCount := boolInt(hasAll) + boolInt(hasAny) + boolInt(hasNot) + boolInt(hasLeaf)
	if shapeCount != 1 {
		return "", nil, []BrowseValidationIssue{{Field: path, Code: "invalid_expression", Message: "Each query expression must contain exactly one of all, any, not, or a field predicate."}}
	}
	if hasNot {
		sql, args, issues := compileBrowseExpression(*expression.Not, path+".not", depth+1, clauses)
		if len(issues) > 0 {
			return "", nil, issues
		}
		return "NOT (" + sql + ")", args, nil
	}
	if hasAll || hasAny {
		children := expression.All
		field := ".all"
		join := " AND "
		if hasAny {
			children = expression.Any
			field = ".any"
			join = " OR "
		}
		if len(children) == 0 {
			return "", nil, []BrowseValidationIssue{{Field: path + field, Code: "empty_group", Message: "Query groups must contain at least one expression."}}
		}
		parts := make([]string, 0, len(children))
		args := []any{}
		for index, child := range children {
			childSQL, childArgs, issues := compileBrowseExpression(child, fmt.Sprintf("%s%s[%d]", path, field, index), depth+1, clauses)
			if len(issues) > 0 {
				return "", nil, issues
			}
			parts = append(parts, "("+childSQL+")")
			args = append(args, childArgs...)
		}
		return strings.Join(parts, join), args, nil
	}
	*clauses++
	if *clauses > browsecontract.MaximumClauses {
		return "", nil, []BrowseValidationIssue{{Field: path, Code: "maximum_clauses_exceeded", Message: fmt.Sprintf("Browse queries cannot exceed %d predicates.", browsecontract.MaximumClauses)}}
	}
	return compileBrowsePredicate(expression, path)
}

func compileBrowsePredicate(predicate BrowseExpression, path string) (string, []any, []BrowseValidationIssue) {
	field := strings.TrimSpace(predicate.Field)
	operator := strings.ToLower(strings.TrimSpace(predicate.Operator))
	capability, exists := browseFieldCapability(field)
	if !exists {
		return "", nil, []BrowseValidationIssue{{Field: path + ".field", Code: "unsupported_field", Message: "Browse field is not supported."}}
	}
	if !containsString(capability.Operators, operator) {
		return "", nil, []BrowseValidationIssue{{Field: path + ".operator", Code: "unsupported_operator", Message: "Operator is not valid for this field."}}
	}
	if (operator == "is-present" || operator == "is-missing") && string(bytes.TrimSpace(predicate.Value)) != "null" {
		return "", nil, []BrowseValidationIssue{{Field: path + ".value", Code: "invalid_value", Message: "Presence predicates require a null value."}}
	}
	switch field {
	case "entityKind":
		return compileEntityKindPredicate(operator, predicate.Value, path+".value")
	case "title":
		return compileStringPredicate("m.title", operator, predicate.Value, path+".value")
	case "alternateTitle":
		return compileMetadataValueStringPredicate([]string{"alternateTitle", "alias", "title"}, operator, predicate.Value, path+".value")
	case "originalTitle":
		return compileStringPredicate("m.original_title", operator, predicate.Value, path+".value")
	case "status":
		return compileMetadataValueStringPredicate([]string{"status"}, operator, predicate.Value, path+".value")
	case "language":
		return compileFacetPredicate("language", operator, predicate.Value, path+".value")
	case "contentRating":
		return compileStringPredicate("m.content_rating", operator, predicate.Value, path+".value")
	case "studio":
		return compileFacetStringPredicate([]string{"studio"}, operator, predicate.Value, path+".value")
	case "company":
		return compileFacetStringPredicate([]string{"company"}, operator, predicate.Value, path+".value")
	case "network":
		return compileFacetStringPredicate([]string{"network"}, operator, predicate.Value, path+".value")
	case "country":
		return compileFacetStringPredicate([]string{"country"}, operator, predicate.Value, path+".value")
	case "year":
		return compileNumberPredicate("m.year", operator, predicate.Value, path+".value")
	case "decade":
		value, issue := browseNumber(predicate.Value, path+".value")
		if issue != nil {
			return "", nil, []BrowseValidationIssue{*issue}
		}
		start := int(value/10) * 10
		return "m.year >= ? AND m.year < ?", []any{start, start + 10}, nil
	case "durationSeconds":
		return compileNumberPredicate("m.duration_seconds", operator, predicate.Value, path+".value")
	case "dateAdded":
		return compileDatePredicate("m.added_at", operator, predicate.Value, path+".value")
	case "releaseDate":
		value := normalizedReleaseDateSQL("m")
		if operator == "is-present" {
			return value + " <> ''", nil, nil
		}
		if operator == "is-missing" {
			return value + " = ''", nil, nil
		}
		return compileDatePredicate(value, operator, predicate.Value, path+".value")
	case "lastPlayedAt":
		if operator == "is-present" {
			return "COALESCE(ums.last_played_at, '') <> ''", nil, nil
		}
		if operator == "is-missing" {
			return "COALESCE(ums.last_played_at, '') = ''", nil, nil
		}
		return compileDatePredicate("COALESCE(ums.last_played_at, '')", operator, predicate.Value, path+".value")
	case "favorite":
		return compileBooleanPredicate("COALESCE(ums.favorite, 0)", predicate.Value, path+".value")
	case "watchlisted":
		return compileBooleanPredicate("COALESCE(ums.watchlisted, 0)", predicate.Value, path+".value")
	case "personalRating":
		if operator == "is-present" {
			return "COALESCE(ums.rating, 0) > 0", nil, nil
		}
		if operator == "is-missing" {
			return "COALESCE(ums.rating, 0) = 0", nil, nil
		}
		return compileNumberPredicate("COALESCE(ums.rating, 0)", operator, predicate.Value, path+".value")
	case "communityRating":
		return compileNumberPredicate("m.community_rating", operator, predicate.Value, path+".value")
	case "criticRating":
		return compileNumberPredicate("m.critic_rating", operator, predicate.Value, path+".value")
	case "audienceRating":
		return compileMetadataValueNumberPredicate([]string{"audienceRating"}, operator, predicate.Value, path+".value")
	case "playState":
		return compilePlayStatePredicate(operator, predicate.Value, path+".value")
	case "genre":
		return compileFacetPredicate("genre", operator, predicate.Value, path+".value")
	case "tag":
		return compileFacetPredicate("tag", operator, predicate.Value, path+".value")
	case "label":
		return compileJSONListPredicate("m.labels_json", operator, predicate.Value, path+".value")
	case "author":
		return compileFacetStringPredicate([]string{"author"}, operator, predicate.Value, path+".value")
	case "narrator":
		return compileFacetStringPredicate([]string{"narrator"}, operator, predicate.Value, path+".value")
	case "series":
		return compileFacetStringPredicate([]string{"series"}, operator, predicate.Value, path+".value")
	case "keyword", "collection", "franchise":
		return compileFacetPredicate(field, operator, predicate.Value, path+".value")
	case "regionalCertification":
		return compileMetadataRelationshipStringPredicate([]string{"certification", "contentRating"}, true, operator, predicate.Value, path+".value")
	case "acceptedProviderIdentity":
		return compileAcceptedProviderIdentityPredicate(operator, predicate.Value, path+".value")
	case "availability":
		return compileAvailabilityPredicate(operator, predicate.Value, path+".value")
	case "actor":
		return compilePersonRolePredicate([]string{"actor", "cast", "voice"}, operator, predicate.Value, path+".value")
	case "director":
		return compilePersonRolePredicate([]string{"director"}, operator, predicate.Value, path+".value")
	case "writer":
		return compilePersonRolePredicate([]string{"writer", "screenplay"}, operator, predicate.Value, path+".value")
	case "creator":
		return compileFacetStringPredicate([]string{"creator"}, operator, predicate.Value, path+".value")
	case "credit":
		return compilePersonRolePredicate([]string{"actor", "cast", "voice", "director", "writer", "screenplay", "creator", "created by", "producer", "composer", "narrator", "author"}, operator, predicate.Value, path+".value")
	case "resolution", "dynamicRange", "source", "mediaVersion":
		column := map[string]string{"resolution": "resolution", "dynamicRange": "dynamic_range", "source": "source_type", "mediaVersion": "version_label"}[field]
		return compileMediaFilePredicate(column, operator, predicate.Value, path+".value")
	default:
		return "", nil, []BrowseValidationIssue{{Field: path + ".field", Code: "unsupported_field", Message: "Browse field is not supported."}}
	}
}

func compileJSONListPredicate(column, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	values, issue := browseStringsForOperator(raw, operator, path)
	if issue != nil {
		return "", nil, []BrowseValidationIssue{*issue}
	}
	parts := make([]string, 0, len(values))
	args := []any{}
	for _, value := range values {
		parts = append(parts, "EXISTS (SELECT 1 FROM json_each("+column+") browse_value WHERE lower(CAST(browse_value.value AS TEXT)) = lower(?))")
		args = append(args, value)
	}
	join := " OR "
	if operator == "contains-all" {
		join = " AND "
	}
	sql := "(" + strings.Join(parts, join) + ")"
	if operator == "not-contains" {
		sql = "NOT " + sql
	}
	return sql, args, nil
}

func compileFacetStringPredicate(facetTypes []string, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	negate := operator == "not-equals" || operator == "not-contains"
	positiveOperator := operator
	if operator == "not-equals" {
		positiveOperator = "equals"
	} else if operator == "not-contains" {
		positiveOperator = "contains"
	}
	valueSQL, valueArgs, issues := compileStringPredicate("browse_facet.value", positiveOperator, raw, path)
	if len(issues) > 0 {
		return "", nil, issues
	}
	args := make([]any, 0, len(facetTypes)+len(valueArgs))
	for _, facetType := range facetTypes {
		args = append(args, facetType)
	}
	args = append(args, valueArgs...)
	sqlText := "EXISTS (SELECT 1 FROM media_category_facets browse_facet WHERE browse_facet.media_id = m.id AND browse_facet.library_id = COALESCE(m.library_id, '') AND browse_facet.facet_type IN (" + sqlPlaceholders(len(facetTypes)) + ") AND (" + valueSQL + "))"
	if negate {
		sqlText = "NOT " + sqlText
	}
	return sqlText, args, nil
}

func compileMetadataValueStringPredicate(fieldKeys []string, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	negate := operator == "not-equals" || operator == "not-contains"
	positiveOperator := operator
	if operator == "not-equals" {
		positiveOperator = "equals"
	} else if operator == "not-contains" {
		positiveOperator = "contains"
	}
	valueSQL, valueArgs, issues := compileStringPredicate("CAST(json_extract(browse_value.value_json, '$') AS TEXT)", positiveOperator, raw, path)
	if len(issues) > 0 {
		return "", nil, issues
	}
	args := make([]any, 0, len(fieldKeys)+len(valueArgs))
	for _, fieldKey := range fieldKeys {
		args = append(args, strings.ToLower(strings.TrimSpace(fieldKey)))
	}
	args = append(args, valueArgs...)
	sqlText := `EXISTS (
		SELECT 1 FROM media_metadata_field_values browse_value
		JOIN media_metadata_revisions browse_revision ON browse_revision.id = browse_value.revision_id
		WHERE browse_value.media_id = m.id AND browse_revision.media_id = m.id
			AND browse_revision.revision = m.metadata_revision AND browse_revision.state = 'applied'
			AND browse_value.decision IN ('accepted','locked')
			AND lower(browse_value.field_key) IN (` + sqlPlaceholders(len(fieldKeys)) + `)
			AND (` + valueSQL + `))`
	if negate {
		sqlText = "NOT " + sqlText
	}
	return sqlText, args, nil
}

func compileMetadataValueNumberPredicate(fieldKeys []string, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	base := `EXISTS (
		SELECT 1 FROM media_metadata_field_values browse_value
		JOIN media_metadata_revisions browse_revision ON browse_revision.id = browse_value.revision_id
		WHERE browse_value.media_id = m.id AND browse_revision.media_id = m.id
			AND browse_revision.revision = m.metadata_revision AND browse_revision.state = 'applied'
			AND browse_value.decision IN ('accepted','locked')
			AND lower(browse_value.field_key) IN (` + sqlPlaceholders(len(fieldKeys)) + `)`
	args := make([]any, len(fieldKeys))
	for index, fieldKey := range fieldKeys {
		args[index] = strings.ToLower(strings.TrimSpace(fieldKey))
	}
	if operator == "is-present" {
		return base + `)`, args, nil
	}
	if operator == "is-missing" {
		return "NOT " + base + `)`, args, nil
	}
	valueSQL, valueArgs, issues := compileNumberPredicate("CAST(json_extract(browse_value.value_json, '$') AS REAL)", operator, raw, path)
	if len(issues) > 0 {
		return "", nil, issues
	}
	return base + ` AND (` + valueSQL + `))`, append(args, valueArgs...), nil
}

func compileMetadataRelationshipStringPredicate(relationshipTypes []string, includeCountry bool, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	negate := operator == "not-equals" || operator == "not-contains"
	positiveOperator := operator
	if operator == "not-equals" {
		positiveOperator = "equals"
	} else if operator == "not-contains" {
		positiveOperator = "contains"
	}
	expression := "browse_relationship.display_value"
	if includeCountry {
		expression = "CASE WHEN trim(browse_relationship.country) <> '' THEN browse_relationship.country || ':' || browse_relationship.display_value ELSE browse_relationship.display_value END"
	}
	valueSQL, valueArgs, issues := compileStringPredicate(expression, positiveOperator, raw, path)
	if len(issues) > 0 {
		return "", nil, issues
	}
	args := make([]any, 0, len(relationshipTypes)+len(valueArgs))
	for _, relationshipType := range relationshipTypes {
		args = append(args, relationshipType)
	}
	args = append(args, valueArgs...)
	sqlText := `EXISTS (
		SELECT 1 FROM media_metadata_relationships browse_relationship
		JOIN media_metadata_revisions browse_revision ON browse_revision.id = browse_relationship.revision_id
		WHERE browse_relationship.media_id = m.id AND browse_revision.media_id = m.id
			AND browse_revision.revision = m.metadata_revision AND browse_revision.state = 'applied'
			AND browse_relationship.decision IN ('accepted','locked')
			AND browse_relationship.relationship_type IN (` + sqlPlaceholders(len(relationshipTypes)) + `)
			AND (` + valueSQL + `))`
	if negate {
		sqlText = "NOT " + sqlText
	}
	return sqlText, args, nil
}

func compileAcceptedProviderIdentityPredicate(operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	values, issue := browseStringsForOperator(raw, operator, path)
	if issue != nil {
		return "", nil, []BrowseValidationIssue{*issue}
	}
	parts := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for _, value := range values {
		parts = append(parts, `EXISTS (
			SELECT 1 FROM media_provider_ids browse_identity
			WHERE browse_identity.media_id = m.id AND browse_identity.status = 'accepted'
				AND browse_identity.evidence_revision = m.metadata_revision
				AND lower(browse_identity.provider || ':' || browse_identity.external_type || ':' || browse_identity.external_id) = lower(?))`)
		args = append(args, strings.TrimSpace(value))
	}
	join := " OR "
	if operator == "contains-all" {
		join = " AND "
	}
	sqlText := "(" + strings.Join(parts, join) + ")"
	if operator == "not-contains" {
		sqlText = "NOT " + sqlText
	}
	return sqlText, args, nil
}

func compilePersonRolePredicate(roles []string, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	negate := operator == "not-equals" || operator == "not-contains"
	positiveOperator := operator
	if operator == "not-equals" {
		positiveOperator = "equals"
	} else if operator == "not-contains" {
		positiveOperator = "contains"
	}
	nameSQL, nameArgs, issues := compileStringPredicate("browse_person.name", positiveOperator, raw, path)
	if len(issues) > 0 {
		return "", nil, issues
	}
	placeholders := sqlPlaceholders(len(roles))
	args := make([]any, 0, len(roles)+len(nameArgs))
	for _, role := range roles {
		args = append(args, role)
	}
	args = append(args, nameArgs...)
	sql := "EXISTS (SELECT 1 FROM media_people browse_person WHERE browse_person.media_id = m.id AND lower(browse_person.role) IN (" + placeholders + ") AND (" + nameSQL + "))"
	if negate {
		sql = "NOT " + sql
	}
	return sql, args, nil
}

func compileMediaFilePredicate(column, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	negate := operator == "not-equals" || operator == "not-contains"
	positiveOperator := operator
	if operator == "not-equals" {
		positiveOperator = "equals"
	} else if operator == "not-contains" {
		positiveOperator = "contains"
	}
	valueSQL, args, issues := compileStringPredicate("browse_file."+column, positiveOperator, raw, path)
	if len(issues) > 0 {
		return "", nil, issues
	}
	sql := "EXISTS (SELECT 1 FROM media_files browse_file WHERE browse_file.media_id = m.id AND browse_file.available = 1 AND (" + valueSQL + "))"
	if negate {
		sql = "NOT " + sql
	}
	return sql, args, nil
}

func browseFieldCapability(id string) (BrowseFieldCapability, bool) {
	for _, field := range canonicalBrowseFields() {
		if field.ID == id {
			return field, true
		}
	}
	return BrowseFieldCapability{}, false
}

func compileStringPredicate(expression, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	value, issue := browseString(raw, path)
	if issue != nil {
		return "", nil, []BrowseValidationIssue{*issue}
	}
	switch operator {
	case "equals":
		return "lower(" + expression + ") = lower(?)", []any{value}, nil
	case "not-equals":
		return "lower(" + expression + ") <> lower(?)", []any{value}, nil
	case "contains":
		return "lower(" + expression + ") LIKE lower(?) ESCAPE '\\'", []any{"%" + escapeSQLLike(value) + "%"}, nil
	case "not-contains":
		return "lower(" + expression + ") NOT LIKE lower(?) ESCAPE '\\'", []any{"%" + escapeSQLLike(value) + "%"}, nil
	case "starts-with":
		return "lower(" + expression + ") LIKE lower(?) ESCAPE '\\'", []any{escapeSQLLike(value) + "%"}, nil
	default:
		return "", nil, []BrowseValidationIssue{{Field: path, Code: "unsupported_operator", Message: "String operator is not supported."}}
	}
}

func compileNumberPredicate(expression, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	if operator == "between" {
		values, issue := browseNumberRange(raw, path)
		if issue != nil {
			return "", nil, []BrowseValidationIssue{*issue}
		}
		return expression + " BETWEEN ? AND ?", []any{values[0], values[1]}, nil
	}
	value, issue := browseNumber(raw, path)
	if issue != nil {
		return "", nil, []BrowseValidationIssue{*issue}
	}
	operators := map[string]string{"equals": "=", "less-than": "<", "at-most": "<=", "greater-than": ">", "at-least": ">="}
	sqlOperator, exists := operators[operator]
	if !exists {
		return "", nil, []BrowseValidationIssue{{Field: path, Code: "unsupported_operator", Message: "Numeric operator is not supported."}}
	}
	return expression + " " + sqlOperator + " ?", []any{value}, nil
}

func compileDatePredicate(expression, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	if operator == "between" {
		values, issue := browseStringList(raw, path)
		if issue != nil || len(values) != 2 {
			if issue == nil {
				issue = &BrowseValidationIssue{Field: path, Code: "invalid_value", Message: "Between requires exactly two date values."}
			}
			return "", nil, []BrowseValidationIssue{*issue}
		}
		for _, value := range values {
			if !validBrowseDate(value) {
				return "", nil, []BrowseValidationIssue{{Field: path, Code: "invalid_date", Message: "Date values must use YYYY-MM-DD or RFC 3339."}}
			}
		}
		return expression + " BETWEEN ? AND ?", []any{values[0], values[1]}, nil
	}
	value, issue := browseString(raw, path)
	if issue != nil {
		return "", nil, []BrowseValidationIssue{*issue}
	}
	if !validBrowseDate(value) {
		return "", nil, []BrowseValidationIssue{{Field: path, Code: "invalid_date", Message: "Date values must use YYYY-MM-DD or RFC 3339."}}
	}
	operators := map[string]string{"equals": "=", "less-than": "<", "at-most": "<=", "greater-than": ">", "at-least": ">="}
	sqlOperator, exists := operators[operator]
	if !exists {
		return "", nil, []BrowseValidationIssue{{Field: path, Code: "unsupported_operator", Message: "Date operator is not supported."}}
	}
	return expression + " " + sqlOperator + " ?", []any{value}, nil
}

func compileBooleanPredicate(expression string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", nil, []BrowseValidationIssue{{Field: path, Code: "invalid_value", Message: "Boolean field requires true or false."}}
	}
	return expression + " = ?", []any{boolInt(value)}, nil
}

func compileEntityKindPredicate(operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	values, issue := browseStringsForOperator(raw, operator, path)
	if issue != nil {
		return "", nil, []BrowseValidationIssue{*issue}
	}
	storage := []string{}
	for _, value := range values {
		mapped := storageTypesForEntityKind(value)
		if len(mapped) == 0 {
			return "", nil, []BrowseValidationIssue{{Field: path, Code: "invalid_entity_kind", Message: "Entity kind is not browseable media."}}
		}
		storage = append(storage, mapped...)
	}
	storage = uniqueStrings(storage)
	negate := operator == "not-equals" || operator == "not-in"
	comparison := " IN (" + sqlPlaceholders(len(storage)) + ")"
	if negate {
		comparison = " NOT" + comparison
	}
	args := make([]any, len(storage))
	for index := range storage {
		args[index] = storage[index]
	}
	return "m.type" + comparison, args, nil
}

func compilePlayStatePredicate(operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	values, issue := browseStringsForOperator(raw, operator, path)
	if issue != nil {
		return "", nil, []BrowseValidationIssue{*issue}
	}
	parts := []string{}
	for _, value := range values {
		switch value {
		case "unplayed":
			parts = append(parts, "(COALESCE(ums.watched, 0) = 0 AND COALESCE(ums.progress_seconds, 0) = 0)")
		case "in-progress":
			parts = append(parts, "(COALESCE(ums.watched, 0) = 0 AND COALESCE(ums.progress_seconds, 0) > 0)")
		case "played":
			parts = append(parts, "COALESCE(ums.watched, 0) = 1")
		default:
			return "", nil, []BrowseValidationIssue{{Field: path, Code: "invalid_play_state", Message: "Playback state must be unplayed, in-progress, or played."}}
		}
	}
	sql := "(" + strings.Join(parts, " OR ") + ")"
	if operator == "not-equals" || operator == "not-in" {
		sql = "NOT " + sql
	}
	return sql, nil, nil
}

func compileFacetPredicate(facetType, operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	values, issue := browseStringsForOperator(raw, operator, path)
	if issue != nil {
		return "", nil, []BrowseValidationIssue{*issue}
	}
	for index := range values {
		values[index] = strings.ToLower(strings.TrimSpace(values[index]))
	}
	parts := make([]string, 0, len(values))
	args := []any{}
	for _, value := range values {
		parts = append(parts, "EXISTS (SELECT 1 FROM media_category_facets browse_facet WHERE browse_facet.media_id = m.id AND browse_facet.library_id = m.library_id AND browse_facet.facet_type = ? AND browse_facet.sort_value = ?)")
		args = append(args, facetType, value)
	}
	join := " OR "
	if operator == "contains-all" {
		join = " AND "
	}
	sql := "(" + strings.Join(parts, join) + ")"
	if operator == "not-contains" {
		sql = "NOT " + sql
	}
	return sql, args, nil
}

func compileAvailabilityPredicate(operator string, raw json.RawMessage, path string) (string, []any, []BrowseValidationIssue) {
	values, issue := browseStringsForOperator(raw, operator, path)
	if issue != nil {
		return "", nil, []BrowseValidationIssue{*issue}
	}
	parts := []string{}
	for _, value := range values {
		switch value {
		case "available":
			parts = append(parts, "COALESCE(availability.missing_file_count, 0) = 0")
		case "partial":
			parts = append(parts, "COALESCE(availability.missing_file_count, 0) > 0 AND COALESCE(availability.missing_file_count, 0) < COALESCE(availability.file_count, 0)")
		case "unavailable":
			parts = append(parts, "COALESCE(availability.file_count, 0) > 0 AND availability.missing_file_count >= availability.file_count")
		default:
			return "", nil, []BrowseValidationIssue{{Field: path, Code: "invalid_availability", Message: "Availability must be available, partial, or unavailable."}}
		}
	}
	sql := "(" + strings.Join(parts, " OR ") + ")"
	if operator == "not-equals" || operator == "not-in" {
		sql = "NOT " + sql
	}
	return sql, nil, nil
}

func compileBrowseSorts(sorts []BrowseSort, pivot BrowsePivotCapability) ([]browseSortSpec, []BrowseValidationIssue) {
	if len(sorts) == 0 || len(sorts) > 3 {
		return nil, []BrowseValidationIssue{{Field: "sort", Code: "invalid_sort_count", Message: "Browse sort must contain between one and three fields."}}
	}
	seen := map[string]bool{}
	result := make([]browseSortSpec, 0, len(sorts))
	for index, item := range sorts {
		field := strings.TrimSpace(item.Field)
		direction := strings.ToLower(strings.TrimSpace(item.Direction))
		path := fmt.Sprintf("sort[%d]", index)
		if direction != "asc" && direction != "desc" {
			return nil, []BrowseValidationIssue{{Field: path + ".direction", Code: "invalid_direction", Message: "Sort direction must be asc or desc."}}
		}
		if seen[field] {
			return nil, []BrowseValidationIssue{{Field: path + ".field", Code: "duplicate_sort", Message: "A sort field may appear only once."}}
		}
		seen[field] = true
		capability, exists := browseSortCapability(field)
		if !exists || !sortAppliesToPivot(capability, pivot) {
			return nil, []BrowseValidationIssue{{Field: path + ".field", Code: "unsupported_sort", Message: "Sort is not available for this pivot."}}
		}
		spec, ok := browseSortSpecFor(field, direction)
		if !ok {
			return nil, []BrowseValidationIssue{{Field: path + ".field", Code: "unsupported_sort", Message: "Sort is not supported."}}
		}
		result = append(result, spec)
	}
	return result, nil
}

func browseSortCapability(id string) (BrowseSortCapability, bool) {
	for _, capability := range canonicalBrowseSorts() {
		if capability.ID == id {
			return capability, true
		}
	}
	return BrowseSortCapability{}, false
}

func sortAppliesToPivot(capability BrowseSortCapability, pivot BrowsePivotCapability) bool {
	if len(capability.ApplicableKinds) == 0 {
		return true
	}
	for _, entity := range pivot.EntityKinds {
		if containsString(capability.ApplicableKinds, entity) {
			return true
		}
	}
	return false
}

func browseSortSpecFor(field, direction string) (browseSortSpec, bool) {
	spec := browseSortSpec{Field: field, Direction: strings.ToUpper(direction)}
	switch field {
	case "title", "sortTitle":
		spec.SQL = "m.sort_title COLLATE NOCASE"
		spec.Value = func(item MediaItem) any { return item.SortTitle }
	case "dateAdded":
		spec.SQL = "m.added_at"
		spec.Value = func(item MediaItem) any { return item.AddedAt }
	case "releaseDate":
		spec.SQL = normalizedReleaseDateSQL("m")
		spec.Value = func(item MediaItem) any { return normalizedMediaReleaseDate(item) }
	case "year":
		spec.SQL, spec.Numeric = "m.year", true
		spec.Value = func(item MediaItem) any { return item.Year }
	case "communityRating":
		spec.SQL, spec.Numeric = "m.community_rating", true
		spec.Value = func(item MediaItem) any { return item.CommunityRating }
	case "criticRating":
		spec.SQL, spec.Numeric = "m.critic_rating", true
		spec.Value = func(item MediaItem) any { return item.CriticRating }
	case "durationSeconds":
		spec.SQL, spec.Numeric = "m.duration_seconds", true
		spec.Value = func(item MediaItem) any { return item.DurationSeconds }
	case "lastPlayedAt":
		spec.SQL = "COALESCE(ums.last_played_at, '')"
		spec.Value = func(item MediaItem) any { return item.State.LastPlayedAt }
	case "seasonNumber":
		spec.SQL, spec.Numeric = "m.season_number", true
		spec.Value = func(item MediaItem) any { return item.SeasonNumber }
	case "episodeNumber":
		spec.SQL, spec.Numeric = "m.episode_number", true
		spec.Value = func(item MediaItem) any { return item.EpisodeNumber }
	case "artist":
		spec.SQL = "m.sort_artist_key"
		spec.Value = func(item MediaItem) any { return item.SortArtistKey }
	case "album":
		spec.SQL = "COALESCE(parent.sort_title, '') COLLATE NOCASE"
		spec.Value = func(item MediaItem) any { return sortableTitle(item.ParentTitle) }
	case "trackNumber":
		spec.SQL, spec.Numeric = "m.index_number", true
		spec.Value = func(item MediaItem) any { return item.IndexNumber }
	case "author":
		spec.SQL = "m.sort_author_key"
		spec.Value = func(item MediaItem) any { return item.SortAuthorKey }
	default:
		return browseSortSpec{}, false
	}
	return spec, true
}

func normalizedReleaseDateSQL(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "m"
	}
	raw := "TRIM(COALESCE(json_extract(" + alias + ".typed_metadata_json, '$.release_date'), json_extract(" + alias + ".typed_metadata_json, '$.releaseDate'), ''))"
	date := "SUBSTR(" + raw + ", 1, 10)"
	return "CASE WHEN " + raw + " GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]*' AND DATE(" + date + ") = " + date + " THEN " + date + " ELSE '' END"
}

func normalizedMediaReleaseDate(item MediaItem) string {
	if item.ReleaseDateKey != "" {
		return item.ReleaseDateKey
	}
	raw := strings.TrimSpace(firstNonEmpty(item.TypedMetadata["release_date"], item.TypedMetadata["releaseDate"]))
	if len(raw) < len("2006-01-02") {
		return ""
	}
	value := raw[:len("2006-01-02")]
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return ""
	}
	return value
}

func browseOrderSQL(specs []browseSortSpec) string {
	parts := make([]string, 0, len(specs)+1)
	for _, spec := range specs {
		parts = append(parts, spec.SQL+" "+spec.Direction)
	}
	parts = append(parts, "m.id ASC")
	return strings.Join(parts, ", ")
}

func appendBrowseKeysetPredicate(where string, args []any, specs []browseSortSpec, values []any) (string, []any) {
	predicate, cursorArgs := browseKeysetPredicate(specs, values, 0)
	if predicate == "" {
		return where, args
	}
	return where + " AND (" + predicate + ")", append(args, cursorArgs...)
}

func browseKeysetPredicate(specs []browseSortSpec, values []any, index int) (string, []any) {
	if index >= len(specs) {
		id := values[len(values)-1]
		return "m.id > ?", []any{id}
	}
	operator := ">"
	if specs[index].Direction == "DESC" {
		operator = "<"
	}
	tail, tailArgs := browseKeysetPredicate(specs, values, index+1)
	return fmt.Sprintf("%s %s ? OR (%s = ? AND (%s))", specs[index].SQL, operator, specs[index].SQL, tail), append([]any{values[index], values[index]}, tailArgs...)
}

func browseSortValues(item MediaItem, specs []browseSortSpec) []any {
	values := make([]any, 0, len(specs)+1)
	for _, spec := range specs {
		values = append(values, spec.Value(item))
	}
	return append(values, item.ID)
}

func validateBrowseCursorValues(values []any, specs []browseSortSpec) ([]any, error) {
	if len(values) != len(specs)+1 {
		return nil, fmt.Errorf("%w: sort key count does not match", errInvalidCursor)
	}
	result := append([]any(nil), values...)
	for index, spec := range specs {
		if spec.Numeric {
			switch value := result[index].(type) {
			case float64:
				result[index] = value
			case json.Number:
				parsed, err := value.Float64()
				if err != nil {
					return nil, fmt.Errorf("%w: numeric sort key is malformed", errInvalidCursor)
				}
				result[index] = parsed
			default:
				return nil, fmt.Errorf("%w: numeric sort key has the wrong type", errInvalidCursor)
			}
		} else if _, ok := result[index].(string); !ok {
			return nil, fmt.Errorf("%w: string sort key has the wrong type", errInvalidCursor)
		}
	}
	if id, ok := result[len(result)-1].(string); !ok || strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("%w: stable identity is missing", errInvalidCursor)
	}
	return result, nil
}

func validateBrowsePresentation(fields []string) ([]string, []BrowseValidationIssue) {
	allowed := map[string]bool{}
	for _, field := range canonicalPresentationFields() {
		allowed[field] = true
	}
	result := []string{}
	seen := map[string]bool{}
	for index, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || !allowed[field] {
			return nil, []BrowseValidationIssue{{Field: fmt.Sprintf("presentation.fields[%d]", index), Code: "unsupported_presentation_field", Message: "Presentation field is not supported."}}
		}
		if !seen[field] {
			result = append(result, field)
			seen[field] = true
		}
	}
	return result, nil
}

func mediaCardForBrowse(item MediaItem, user User, presentation []string) MediaCard {
	availability := MediaAvailabilitySummary{Status: "available", FileCount: item.FileCount, MissingFileCount: item.MissingFileCount}
	if item.FileCount > 0 && item.MissingFileCount >= item.FileCount {
		availability.Status = "unavailable"
	} else if item.MissingFileCount > 0 {
		availability.Status = "partial"
	}
	actions := mediaActionsForItem(item, user)
	fields := map[string]any{}
	for _, field := range presentation {
		switch field {
		case "year":
			fields[field] = item.Year
		case "durationSeconds":
			fields[field] = item.DurationSeconds
		case "contentRating":
			fields[field] = item.ContentRating
		case "communityRating":
			fields[field] = item.CommunityRating
		case "criticRating":
			fields[field] = item.CriticRating
		case "playState":
			fields[field] = mediaPlayState(item.State)
		case "progressSeconds":
			fields[field] = item.State.ProgressSeconds
		case "availability":
			fields[field] = availability.Status
		case "parentTitle":
			fields[field] = item.ParentTitle
		case "seasonNumber":
			fields[field] = item.SeasonNumber
		case "episodeNumber":
			fields[field] = item.EpisodeNumber
		}
	}
	if len(fields) == 0 {
		fields = nil
	}
	return MediaCard{
		ID: item.ID, LibraryID: item.LibraryID, LibraryName: item.LibraryName, Counts: item.Counts, EntityKind: canonicalEntityKind(item.Type),
		Title: item.Title, Subtitle: browseMediaSubtitle(item), Summary: strings.TrimSpace(item.Summary), Year: item.Year,
		DurationSeconds: item.DurationSeconds, Artwork: item.Images,
		UserState: MediaCardUserState{
			Watchlisted: item.State.Watchlisted, Favorite: item.State.Favorite, Watched: item.State.Watched,
			ProgressSeconds: item.State.ProgressSeconds, LastPlayedAt: item.State.LastPlayedAt,
		},
		Availability: availability, Actions: actions, Fields: fields,
	}
}

func browseMediaSubtitle(item MediaItem) string {
	if item.Type == "episode" {
		series := firstNonEmpty(item.GrandparentTitle, item.ParentTitle)
		if series != "" && item.SeasonNumber > 0 && item.EpisodeNumber > 0 {
			return fmt.Sprintf("%s · S%d E%d", series, item.SeasonNumber, item.EpisodeNumber)
		}
		return series
	}
	if item.Type == "track" || item.Type == "album" {
		return item.ParentTitle
	}
	return ""
}

func canonicalEntityKind(storageType string) string {
	return string(catalogkind.Public(storageType))
}

func storageTypesForEntityKind(kind string) []string {
	return catalogkind.StorageTypes(kind)
}

func playableMediaType(kind string) bool {
	switch kind {
	case "movie", "episode", "track", "audiobook", "recording":
		return true
	default:
		return false
	}
}

func mediaPlayState(state MediaState) string {
	if state.Watched {
		return "played"
	}
	if state.ProgressSeconds > 0 {
		return "in-progress"
	}
	return "unplayed"
}

func browseStringsForOperator(raw json.RawMessage, operator, path string) ([]string, *BrowseValidationIssue) {
	if operator == "in" || operator == "not-in" || operator == "contains-any" || operator == "contains-all" {
		return browseStringList(raw, path)
	}
	value, issue := browseString(raw, path)
	if issue != nil {
		return nil, issue
	}
	return []string{value}, nil
}

func browseString(raw json.RawMessage, path string) (string, *BrowseValidationIssue) {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return "", &BrowseValidationIssue{Field: path, Code: "invalid_value", Message: "Field requires a non-empty string value."}
	}
	return strings.TrimSpace(value), nil
}

func browseStringList(raw json.RawMessage, path string) ([]string, *BrowseValidationIssue) {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil || len(values) == 0 {
		return nil, &BrowseValidationIssue{Field: path, Code: "invalid_value", Message: "Field requires a non-empty string array."}
	}
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
		if values[index] == "" {
			return nil, &BrowseValidationIssue{Field: fmt.Sprintf("%s[%d]", path, index), Code: "invalid_value", Message: "Array values cannot be empty."}
		}
	}
	return uniqueStrings(values), nil
}

func browseNumber(raw json.RawMessage, path string) (float64, *BrowseValidationIssue) {
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if len(raw) == 0 || decoder.Decode(&number) != nil {
		return 0, &BrowseValidationIssue{Field: path, Code: "invalid_value", Message: "Field requires a numeric value."}
	}
	value, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		return 0, &BrowseValidationIssue{Field: path, Code: "invalid_value", Message: "Field requires a finite numeric value."}
	}
	return value, nil
}

func browseNumberRange(raw json.RawMessage, path string) ([2]float64, *BrowseValidationIssue) {
	var values []json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if len(raw) == 0 || decoder.Decode(&values) != nil || len(values) != 2 {
		return [2]float64{}, &BrowseValidationIssue{Field: path, Code: "invalid_value", Message: "Between requires exactly two numeric values."}
	}
	result := [2]float64{}
	for index, number := range values {
		value, err := strconv.ParseFloat(number.String(), 64)
		if err != nil {
			return [2]float64{}, &BrowseValidationIssue{Field: fmt.Sprintf("%s[%d]", path, index), Code: "invalid_value", Message: "Between values must be finite numbers."}
		}
		result[index] = value
	}
	if result[0] > result[1] {
		return [2]float64{}, &BrowseValidationIssue{Field: path, Code: "invalid_range", Message: "Between lower bound cannot exceed upper bound."}
	}
	return result, nil
}

func validBrowseDate(value string) bool {
	if _, err := time.Parse("2006-01-02", value); err == nil {
		return true
	}
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func escapeSQLLike(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(value)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func writeBrowseValidationProblem(w http.ResponseWriter, issues []BrowseValidationIssue) {
	requestID := strings.TrimSpace(w.Header().Get(requestIDHeader))
	if requestID == "" {
		requestID = randomID("req")
		w.Header().Set(requestIDHeader, requestID)
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"type":      "https://portico.media/problems/validation-failed",
		"title":     "The request could not be validated",
		"status":    http.StatusUnprocessableEntity,
		"code":      "validation_failed",
		"detail":    fmt.Sprintf("%d browse field%s could not be validated.", len(issues), pluralSuffix(len(issues))),
		"requestId": requestID,
		"errors":    issues,
	})
}
