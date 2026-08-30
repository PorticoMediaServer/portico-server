package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/catalogkind"
)

const (
	homeManifestPreviewLimit      = 6
	homeRowDefaultPageLimit       = 24
	searchPreviewLimit            = 8
	maximumConcurrentHomePreviews = 3
	// Ranking an unbounded ubiquitous FTS term forces SQLite to score and sort
	// the entire catalogue before returning a preview. A bounded relevance
	// window preserves useful ranking and deterministic keyset pagination while
	// keeping foreground work independent of total catalogue size.
	searchRelevanceCandidateLimit = 16_384
)

type materializedPageCursor struct {
	Offset int `json:"offset"`
}

func (s *Server) encodeMaterializedPageCursor(scope, principal string, offset int, now time.Time) (string, error) {
	if offset < 0 {
		return "", fmt.Errorf("%w: negative offset", errInvalidCursor)
	}
	return s.encodeContractCursor(scope, principal, materializedPageCursor{Offset: offset}, now)
}

func (s *Server) decodeMaterializedPageCursor(value, scope, principal string, now time.Time) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	var cursor materializedPageCursor
	if err := s.decodeContractCursor(value, scope, principal, &cursor, now); err != nil {
		return 0, err
	}
	if cursor.Offset < 0 {
		return 0, fmt.Errorf("%w: negative offset", errInvalidCursor)
	}
	return cursor.Offset, nil
}

func homeRowDescriptor(id, title, kind, layout, explanation string, priority int, critical bool) HomeRow {
	required := id == "continue" || id == "continue_listening" || id == "ondeck"
	row := HomeRow{
		ID: id, Kind: kind, Title: title, Type: layout, Explanation: explanation,
		Endpoint: "/api/home/rows/" + id, Priority: priority, DefaultVisible: true,
		Critical: critical, CursorCapable: true, Required: required, Hideable: !required, Reorderable: true,
		CacheTTLSeconds: 45,
	}
	row.ArtworkShape = resolvedHomeRowArtworkShape(row)
	return row
}

func resolvedHomeRowArtworkShape(row HomeRow) string {
	if row.ArtworkShape == "square" || row.ArtworkShape == "poster" || row.ArtworkShape == "landscape" {
		return row.ArtworkShape
	}
	if row.ID == "continue_listening" || row.Type == "square" || strings.Contains(row.Kind, "listening") {
		return "square"
	}
	if row.Type == "landscape" {
		return "landscape"
	}
	return "poster"
}

func resolveHomeRowArtworkShapes(rows []HomeRow) []HomeRow {
	for index := range rows {
		rows[index].ArtworkShape = resolvedHomeRowArtworkShape(rows[index])
	}
	return rows
}

func resolveLibraryHomeRowArtworkShapes(rows []HomeRow, library Library) []HomeRow {
	for index := range rows {
		if library.Type == "music" && rows[index].Type != "landscape" {
			rows[index].ArtworkShape = "square"
			continue
		}
		rows[index].ArtworkShape = resolvedHomeRowArtworkShape(rows[index])
	}
	return rows
}

func paginateMaterializedHomeRow(row HomeRow, limit, offset int) HomeRow {
	row.ArtworkShape = resolvedHomeRowArtworkShape(row)
	items := row.Items
	if offset >= len(items) {
		row.Items = []MediaItem{}
		row.Total = len(items)
		row.Limit = limit
		row.Offset = offset
		row.HasMore = false
		return row
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	row.Items = append([]MediaItem(nil), items[offset:end]...)
	row.Total = len(items)
	row.Limit = limit
	row.Offset = offset
	row.HasMore = end < len(items)
	return row
}

func (s *Server) homeManifest(ctx context.Context, user User) (HomeResponse, error) {
	cacheKey := homeCacheKey(user)
	if hasState, err := s.profileHasMediaStateContext(ctx, viewerProfileID(user)); err == nil && !hasState {
		cacheKey = emptyMediaStateCacheScope(user, false)
	}
	if response, ok := s.cachedHomeResponse(cacheKey, viewerProfileID(user)); ok {
		return response, nil
	}
	wait, owner := s.beginHomeResponseBuild(cacheKey)
	if !owner {
		select {
		case <-wait:
		case <-ctx.Done():
			return HomeResponse{}, ctx.Err()
		}
		if response, ok := s.cachedHomeResponse(cacheKey, viewerProfileID(user)); ok {
			return response, nil
		}
		if err := ctx.Err(); err != nil {
			return HomeResponse{}, err
		}
		// The previous owner failed before publishing a value. Re-entering the
		// single-flight gate elects exactly one waiter to retry rather than
		// releasing the entire burst into SQLite.
		return s.homeManifest(ctx, user)
	}
	defer s.finishHomeResponseBuild(cacheKey)
	response, err := s.buildHomeManifest(ctx, user)
	if err == nil {
		s.storeHomeResponse(cacheKey, viewerProfileID(user), response)
	}
	return response, err
}

func (s *Server) buildHomeManifest(ctx context.Context, user User) (HomeResponse, error) {
	catalogueRows, err := s.sharedHomeCatalogueProjection(ctx, user)
	if err != nil {
		return HomeResponse{}, err
	}
	rows := []HomeRow{
		homeRowDescriptor("continue", "Continue Watching", "continue-watching", "continue", "Resume movies and episodes where you left off.", 10, true),
		homeRowDescriptor("continue_listening", "Continue Listening", "continue-listening", "square", "Resume music and audiobooks where you left off.", 20, true),
		homeRowDescriptor("ondeck", "On Deck", "next-up", "landscape", "The next playable episode from series you have started.", 30, true),
	}
	rows = append(rows, catalogueRows...)
	priority := 40
	if len(catalogueRows) > 0 {
		priority = catalogueRows[len(catalogueRows)-1].Priority + 10
	}
	if s.communityHomeRowEnabled(ctx) {
		row := homeRowDescriptor("server_watching_week", "People On This Server Are Watching", "people-on-this-server-are-watching", "poster", "Titles watched by at least two activity-sharing members during the last seven days.", priority, false)
		row.PrivacySensitivity = "aggregated-activity"
		row.PolicyState = "enabled"
		// Community activity is optional decoration, not part of the critical
		// home manifest. Never let its aggregate consume the entire navigation
		// deadline on a cold large catalogue.
		communityCtx, communityCancel := context.WithTimeout(ctx, 100*time.Millisecond)
		preview, loadErr, ok := s.homeRowPageContext(communityCtx, user, row.ID, 1, 0)
		communityCancel()
		if loadErr == nil && ok && len(preview.Items) > 0 {
			rows = append(rows, row)
			priority += 10
		}
	}
	rows = append(rows,
		homeRowDescriptor("recommended_for_you", "Recommended For You", "personalized", "poster", "Suggestions based on your own library activity and saved media.", priority, false),
		homeRowDescriptor("tmdb_trending", "Trending Now", "trending", "poster", "Popular titles matched to media available on this server.", priority+10, false),
		homeRowDescriptor("watchlist", "Watchlist", "watchlist", "poster", "Titles you saved to watch later.", priority+20, false),
		homeRowDescriptor("favorites", "Favorites", "favorites", "poster", "Titles you marked as favorites.", priority+30, false),
	)

	// Only critical rows get a small embedded preview. The preview queries are
	// independent, but concurrency remains explicitly bounded to protect SQLite.
	if err := s.hydrateHomeManifestPreviews(ctx, user, rows, s.homeRowPageContext); err != nil {
		return HomeResponse{}, err
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Priority < rows[j].Priority })
	return HomeResponse{Pivots: []string{"Home"}, Rows: rows}, nil
}

type homeManifestPreviewLoader func(context.Context, User, string, int, int) (HomeRow, error, bool)

type homeManifestPreviewResult struct {
	index int
	page  HomeRow
	err   error
	ok    bool
}

func (s *Server) hydrateHomeManifestPreviews(ctx context.Context, user User, rows []HomeRow, load homeManifestPreviewLoader) error {
	tasks := make([]int, 0, len(rows))
	for index := range rows {
		if rows[index].Critical {
			tasks = append(tasks, index)
		}
	}
	if len(tasks) == 0 {
		return nil
	}
	workers := maximumConcurrentHomePreviews
	if len(tasks) < workers {
		workers = len(tasks)
	}
	work := make(chan int)
	results := make(chan homeManifestPreviewResult, len(tasks))
	var workersWG sync.WaitGroup
	workersWG.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer workersWG.Done()
			for index := range work {
				page, err, ok := load(ctx, user, rows[index].ID, homeManifestPreviewLimit, 0)
				results <- homeManifestPreviewResult{index: index, page: page, err: err, ok: ok}
			}
		}()
	}
	go func() {
		defer close(work)
		for _, index := range tasks {
			select {
			case work <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workersWG.Wait()
		close(results)
	}()

	now := time.Now().UTC()
	for result := range results {
		if result.err != nil || !result.ok {
			continue
		}
		rows[result.index].Items = result.page.Items
		rows[result.index].HasMore = result.page.HasMore
		rows[result.index].Total = result.page.Total
		if result.page.HasMore {
			cursor, err := s.encodeMaterializedPageCursor("home:"+rows[result.index].ID, viewerProfileID(user), len(result.page.Items), now)
			if err != nil {
				return err
			}
			rows[result.index].NextCursor = cursor
		}
	}
	return nil
}

func (s *Server) communityHomeRowEnabled(ctx context.Context) bool {
	settings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return false
	}
	home, _ := settings["home"].(map[string]any)
	value, exists := home["communityActivityEnabled"]
	if !exists {
		return true
	}
	enabled, ok := value.(bool)
	return ok && enabled
}

type searchGroupDefinition struct {
	ID, Title, EntityKind string
	Types                 []string
	Live                  bool
	People                bool
}

type searchResultCursor struct {
	Mode        string  `json:"mode"`
	Sort        string  `json:"sort"`
	Direction   string  `json:"direction"`
	Offset      int     `json:"offset,omitempty"`
	Bucket      int     `json:"bucket,omitempty"`
	Rank        float64 `json:"rank,omitempty"`
	SortTitle   string  `json:"sortTitle,omitempty"`
	Year        int     `json:"year,omitempty"`
	YearMissing int     `json:"yearMissing,omitempty"`
	AddedAt     string  `json:"addedAt,omitempty"`
	IdentityKey string  `json:"identityKey,omitempty"`
	ID          string  `json:"id,omitempty"`
}

var searchGroupDefinitions = []searchGroupDefinition{
	{ID: "movies", Title: "Movies", EntityKind: "movie", Types: []string{"movie"}},
	{ID: "shows", Title: "Shows", EntityKind: "show", Types: []string{"show", "anime", "season"}},
	{ID: "episodes", Title: "Episodes", EntityKind: "episode", Types: []string{"episode"}},
	{ID: "music", Title: "Music", EntityKind: "track", Types: []string{"artist", "album", "track"}},
	{ID: "audiobooks", Title: "Audiobooks", EntityKind: "book", Types: []string{"audiobook"}},
	{ID: "people", Title: "People", EntityKind: "person", People: true},
	{ID: "live-tv", Title: "Live TV Channels", EntityKind: "live-channel", Types: []string{"live_channel"}, Live: true},
}

func (s *Server) executeSearch(ctx context.Context, user User, request SearchRequest) (SearchResponse, error) {
	profileID := viewerProfileID(user)
	hasState, stateErr := s.profileHasMediaStateContext(ctx, profileID)
	if stateErr != nil || hasState {
		return s.executeSearchWithGroupLoader(ctx, user, request, s.loadSearchGroupPage)
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return SearchResponse{}, err
	}
	digest := sha256.Sum256(requestBody)
	key := emptyMediaStateCacheScope(user, false) + "\x00" + hex.EncodeToString(digest[:])
	call, owner := s.beginSearchResponseInFlight(key)
	if !owner {
		select {
		case <-call.done:
			if call.err != nil {
				return SearchResponse{}, call.err
			}
			return s.rebindSearchResponseCursors(call.response, request, call.sourceProfile, profileID)
		case <-ctx.Done():
			return SearchResponse{}, ctx.Err()
		}
	}
	response, loadErr := s.executeSearchWithGroupLoader(ctx, user, request, s.loadSearchGroupPage)
	s.finishSearchResponseInFlight(key, call, response, profileID, loadErr)
	return response, loadErr
}

type searchResponseInFlightCall struct {
	done          chan struct{}
	response      SearchResponse
	sourceProfile string
	err           error
	expiresAt     time.Time
}

func (s *Server) beginSearchResponseInFlight(key string) (*searchResponseInFlightCall, bool) {
	s.searchResponseInFlightMu.Lock()
	defer s.searchResponseInFlightMu.Unlock()
	if s.searchResponseInFlight == nil {
		s.searchResponseInFlight = map[string]*searchResponseInFlightCall{}
	}
	now := time.Now()
	for candidateKey, candidate := range s.searchResponseInFlight {
		if candidate != nil && !candidate.expiresAt.IsZero() && !now.Before(candidate.expiresAt) {
			delete(s.searchResponseInFlight, candidateKey)
		}
	}
	if call := s.searchResponseInFlight[key]; call != nil {
		return call, false
	}
	for len(s.searchResponseInFlight) >= 128 {
		for candidateKey := range s.searchResponseInFlight {
			delete(s.searchResponseInFlight, candidateKey)
			break
		}
	}
	call := &searchResponseInFlightCall{done: make(chan struct{})}
	s.searchResponseInFlight[key] = call
	return call, true
}

func (s *Server) finishSearchResponseInFlight(key string, call *searchResponseInFlightCall, response SearchResponse, sourceProfile string, err error) {
	s.searchResponseInFlightMu.Lock()
	cacheable := err == nil && !searchResponseHasErrors(response)
	if !cacheable && s.searchResponseInFlight[key] == call {
		delete(s.searchResponseInFlight, key)
	}
	call.response, call.sourceProfile, call.err = response, sourceProfile, err
	if cacheable {
		call.expiresAt = time.Now().Add(60 * time.Second)
	}
	close(call.done)
	s.searchResponseInFlightMu.Unlock()
}

func searchResponseHasErrors(response SearchResponse) bool {
	for _, group := range response.Groups {
		if group.Status == "error" {
			return true
		}
	}
	return false
}

func (s *Server) invalidateSearchResponseCache() {
	s.searchResponseInFlightMu.Lock()
	for key, call := range s.searchResponseInFlight {
		if call != nil && !call.expiresAt.IsZero() {
			delete(s.searchResponseInFlight, key)
		}
	}
	s.searchResponseInFlightMu.Unlock()
}

func (s *Server) rebindSearchResponseCursors(response SearchResponse, request SearchRequest, sourceProfile, targetProfile string) (SearchResponse, error) {
	if sourceProfile == targetProfile {
		return response, nil
	}
	normalized, sortSpec, err := normalizeSearchRequest(request)
	if err != nil {
		return SearchResponse{}, err
	}
	groups := append([]SearchGroup(nil), response.Groups...)
	for index := range groups {
		if strings.TrimSpace(groups[index].NextCursor) == "" {
			continue
		}
		var definition *searchGroupDefinition
		for candidate := range searchGroupDefinitions {
			if searchGroupDefinitions[candidate].ID == groups[index].ID {
				definition = &searchGroupDefinitions[candidate]
				break
			}
		}
		if definition == nil {
			return SearchResponse{}, errInvalidCursor
		}
		scope := searchCursorScope(normalized.Query, definition.ID, normalized.EntityKinds, normalized.LibraryIDs, sortSpec)
		var cursor searchResultCursor
		if err := s.decodeContractCursor(groups[index].NextCursor, scope, sourceProfile, &cursor, time.Now().UTC()); err != nil {
			return SearchResponse{}, err
		}
		token, err := s.encodeContractCursor(scope, targetProfile, cursor, time.Now().UTC())
		if err != nil {
			return SearchResponse{}, err
		}
		groups[index].NextCursor = token
	}
	response.Groups = groups
	return response, nil
}

type searchGroupPageLoader func(context.Context, User, SearchRequest, searchGroupDefinition, int, searchResultCursor, searchSortSpec) ([]MediaItem, error)

func (s *Server) loadSearchGroupPage(ctx context.Context, user User, request SearchRequest, definition searchGroupDefinition, limit int, cursor searchResultCursor, sortSpec searchSortSpec) ([]MediaItem, error) {
	if definition.Live {
		return s.searchLiveTVChannelsPageContext(ctx, user, request.Query, limit, cursor)
	}
	if definition.People {
		return s.searchPeopleContext(ctx, user, request.Query, request.LibraryIDs, limit, cursor, sortSpec)
	}
	requestedKinds := stringSet(request.EntityKinds)
	mediaTypes := append([]string(nil), definition.Types...)
	if len(requestedKinds) > 0 && !requestedKinds[definition.ID] {
		mediaTypes = mediaTypes[:0]
		for _, storageType := range definition.Types {
			if requestedKinds[storageType] || requestedKinds[string(catalogkind.Public(storageType))] {
				mediaTypes = append(mediaTypes, storageType)
			}
		}
	}
	return s.searchMediaGroupContext(ctx, user, request.Query, mediaTypes, request.LibraryIDs, limit, cursor, sortSpec)
}

func (s *Server) executeSearchWithGroupLoader(ctx context.Context, user User, request SearchRequest, load searchGroupPageLoader) (SearchResponse, error) {
	request, sortSpec, err := normalizeSearchRequest(request)
	if err != nil {
		return SearchResponse{}, err
	}
	query := request.Query
	if query == "" {
		return SearchResponse{Query: "", Sort: sortSpec.Field, Direction: sortSpec.Direction, Groups: []SearchGroup{}}, nil
	}
	limit := request.Limit
	if limit <= 0 {
		limit = searchPreviewLimit
	}
	if limit > 50 {
		limit = 50
	}
	requestedKinds := stringSet(request.EntityKinds)
	groups := make([]SearchGroup, 0, len(searchGroupDefinitions))
	attemptedGroups := 0
	successfulGroups := 0
	var lastGroupErr error
	for _, definition := range searchGroupDefinitions {
		if request.Group != "" && request.Group != definition.ID {
			continue
		}
		if len(requestedKinds) > 0 && !searchKindsSelectDefinition(requestedKinds, definition) {
			continue
		}
		cursorScope := searchCursorScope(query, definition.ID, request.EntityKinds, request.LibraryIDs, sortSpec)
		var cursor searchResultCursor
		if request.Cursor != "" {
			if err := s.decodeContractCursor(request.Cursor, cursorScope, viewerProfileID(user), &cursor, time.Now().UTC()); err != nil {
				return SearchResponse{}, err
			}
		}
		if err := validateSearchResultCursor(cursor, definition, sortSpec); err != nil {
			return SearchResponse{}, err
		}
		if definition.Live && len(request.LibraryIDs) > 0 {
			continue
		}
		attemptedGroups++
		items, err := load(ctx, user, request, definition, limit+1, cursor, sortSpec)
		if err != nil {
			lastGroupErr = err
			errorCode := "search_group_unavailable"
			messageID := "search.group-unavailable"
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				errorCode = "search_group_timeout"
				messageID = "search.group-timeout"
			}
			groups = append(groups, SearchGroup{
				ID: definition.ID, Title: definition.Title, EntityKind: definition.EntityKind,
				Status: "error", ErrorCode: errorCode, MessageID: messageID,
				Items: []MediaItem{}, HasMore: false,
			})
			continue
		}
		successfulGroups++
		hasMore := len(items) > limit
		if hasMore {
			items = items[:limit]
		}
		if len(items) == 0 && request.Group == "" {
			continue
		}
		group := SearchGroup{ID: definition.ID, Title: definition.Title, EntityKind: definition.EntityKind, Status: "success", Items: items, HasMore: hasMore}
		if hasMore {
			next := searchResultCursor{}
			if definition.Live {
				last := items[len(items)-1]
				next = searchResultCursor{Mode: "live-keyset", Sort: sortSpec.Field, Direction: sortSpec.Direction, Bucket: last.SearchBucket, Rank: last.SearchRank, Year: last.IndexNumber, SortTitle: last.Title, ID: last.ID}
			} else if definition.People {
				last := items[len(items)-1]
				next = searchResultCursor{Mode: "media-keyset", Sort: sortSpec.Field, Direction: sortSpec.Direction, Rank: last.SearchRank, SortTitle: last.SortTitle, IdentityKey: last.RandomKey, ID: last.ID}
			} else {
				last := items[len(items)-1]
				rank := float64(0)
				if sortSpec.Field == searchSortRelevance {
					var rankErr error
					rank, rankErr = s.searchMediaRankContext(ctx, query, last.ID)
					if rankErr != nil {
						return SearchResponse{}, rankErr
					}
				}
				next = nextSearchMediaCursor(sortSpec, last, rank)
			}
			cursor, cursorErr := s.encodeContractCursor(cursorScope, viewerProfileID(user), next, time.Now().UTC())
			if cursorErr != nil {
				return SearchResponse{}, cursorErr
			}
			group.NextCursor = cursor
		}
		groups = append(groups, group)
	}
	if attemptedGroups > 0 && successfulGroups == 0 && lastGroupErr != nil {
		return SearchResponse{}, lastGroupErr
	}
	return SearchResponse{Query: query, Sort: sortSpec.Field, Direction: sortSpec.Direction, Groups: groups}, nil
}

func searchCursorScope(query, group string, entityKinds, libraryIDs []string, spec searchSortSpec) string {
	kinds := canonicalSearchCursorEntityKinds(entityKinds)
	libraries := append([]string(nil), libraryIDs...)
	sort.Strings(libraries)
	return "search:" + query + ":" + group + ":" + spec.Field + ":" + spec.Direction + ":kinds=" + strings.Join(kinds, ",") + ":libraries=" + strings.Join(libraries, ",")
}

// canonicalSearchCursorEntityKinds records the storage kinds selected by the
// canonical public entity kinds. This prevents a cursor issued for one subset
// of a multi-kind group from being replayed against another subset.
func canonicalSearchCursorEntityKinds(entityKinds []string) []string {
	requested := stringSet(entityKinds)
	if len(requested) == 0 {
		return nil
	}
	effective := map[string]bool{}
	for _, definition := range searchGroupDefinitions {
		if !searchKindsSelectDefinition(requested, definition) {
			continue
		}
		if definition.People {
			effective["person"] = true
			continue
		}
		if definition.Live {
			effective["live-channel"] = true
			continue
		}
		for _, storageType := range definition.Types {
			if requested[string(catalogkind.Public(storageType))] {
				effective[storageType] = true
			}
		}
	}
	result := make([]string, 0, len(effective))
	for kind := range effective {
		result = append(result, kind)
	}
	sort.Strings(result)
	return result
}

func (s *Server) searchMediaGroupContext(ctx context.Context, user User, query string, mediaTypes, requestedLibraries []string, limit int, cursor searchResultCursor, sortSpec searchSortSpec) ([]MediaItem, error) {
	match := mediaSearchQuery(query)
	if match == "" {
		return []MediaItem{}, nil
	}
	where := []string{"1 = 1"}
	args := []any{match}
	if len(mediaTypes) > 0 {
		where = append(where, "m.type IN ("+sqlPlaceholders(len(mediaTypes))+")")
		for _, mediaType := range mediaTypes {
			args = append(args, mediaType)
		}
	}
	allowedLibraries := make([]string, 0, len(requestedLibraries))
	for _, libraryID := range requestedLibraries {
		if s.canAccessLibrary(user, libraryID) {
			allowedLibraries = append(allowedLibraries, libraryID)
		}
	}
	if len(requestedLibraries) > 0 && len(allowedLibraries) == 0 {
		return []MediaItem{}, nil
	}
	if len(allowedLibraries) > 0 {
		where = append(where, "m.library_id IN ("+sqlPlaceholders(len(allowedLibraries))+")")
		for _, libraryID := range allowedLibraries {
			args = append(args, libraryID)
		}
	}
	// Avoid running the same broad FTS match once per empty result group. The
	// type/library probe is covered by ordinary B-tree indexes and is especially
	// important for homogeneous large catalogues.
	availabilityWhere := []string{"m.type IN (" + sqlPlaceholders(len(mediaTypes)) + ")"}
	availabilityArgs := make([]any, 0, len(mediaTypes)+len(allowedLibraries))
	for _, mediaType := range mediaTypes {
		availabilityArgs = append(availabilityArgs, mediaType)
	}
	if len(mediaTypes) == 0 {
		return []MediaItem{}, nil
	}
	if len(allowedLibraries) > 0 {
		availabilityWhere = append(availabilityWhere, "m.library_id IN ("+sqlPlaceholders(len(allowedLibraries))+")")
		for _, libraryID := range allowedLibraries {
			availabilityArgs = append(availabilityArgs, libraryID)
		}
	}
	var available int
	if err := s.queryUserRow(ctx, "SELECT 1 FROM media_items m WHERE "+strings.Join(availabilityWhere, " AND ")+" LIMIT 1", availabilityArgs...).Scan(&available); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []MediaItem{}, nil
		}
		return nil, err
	}
	visibilitySQL := librarySurfaceVisibilityRestrictionSQL("search")
	keyset, keysetArgs, order := searchMediaOrderAndKeyset(sortSpec, cursor)
	if keyset != "" {
		where = append(where, keyset)
		args = append(args, keysetArgs...)
	}
	args = append(args, limit)
	items, err := s.queryMediaListItemsContext(ctx, viewerProfileID(user), fmt.Sprintf(`
		JOIN (
			SELECT media_id, bm25(media_search) AS relevance
			FROM media_search
			WHERE media_search MATCH ?
			LIMIT %d
		) search_rank ON search_rank.media_id = m.id
		WHERE %s %s
		ORDER BY %s
		LIMIT ?`, searchRelevanceCandidateLimit, strings.Join(where, " AND "), visibilitySQL, order), args)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Server) searchMediaRankContext(ctx context.Context, query, mediaID string) (float64, error) {
	match := mediaSearchQuery(query)
	if match == "" || strings.TrimSpace(mediaID) == "" {
		return 0, fmt.Errorf("%w: search rank cursor is incomplete", errInvalidCursor)
	}
	var rank float64
	if err := s.queryUserRow(ctx, `
		SELECT bm25(media_search)
		FROM media_search
		WHERE media_search MATCH ? AND media_id = ?`, match, mediaID).Scan(&rank); err != nil {
		return 0, err
	}
	return rank, nil
}

func decodeSearchRequest(w http.ResponseWriter, r *http.Request) (SearchRequest, bool) {
	var request SearchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_search_request", "Search request must be valid JSON with known fields.")
		return SearchRequest{}, false
	}
	return request, true
}
