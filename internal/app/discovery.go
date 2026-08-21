package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	tmdbTrendingCacheTTL       = 24 * time.Hour
	tmdbRequestTimeout         = 8 * time.Second
	userRecommendationCacheTTL = 15 * time.Minute
	recommendationInputLimit   = 2000
)

type tmdbTrendingPayload struct {
	Results []tmdbTrendingEntry `json:"results"`
}

type tmdbTrendingEntry struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Name         string  `json:"name"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	GenreIDs     []int   `json:"genre_ids"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	Popularity   float64 `json:"popularity"`
	VoteAverage  float64 `json:"vote_average"`
}

type localRecommendationSeed struct {
	ID              string
	LibraryID       string
	Type            string
	Title           string
	Year            int
	Genres          []string
	CommunityRating float64
	ProgressSeconds int
	Rating          int
	LastPlayedAt    string
	Watchlisted     bool
	Watched         bool
	AddedAt         string
	Metadata        metadataRecommendationInput
}

type scoredMedia struct {
	ID     string
	Score  float64
	Reason string
	Source string
}

func (s *Server) handleSuggestions(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	limit := 24
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = max(1, min(50, parsed))
		}
	}
	libraryID := strings.TrimSpace(r.URL.Query().Get("libraryId"))
	ctx, cancel := context.WithTimeout(r.Context(), navigationRequestTimeout)
	defer cancel()
	cacheKey := suggestionsCacheKey(viewerProfileID(user), libraryID, limit)
	if cached, ok := s.cachedSuggestions(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}
	if err := ctx.Err(); err != nil {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "suggestions_timeout", "Suggestions exceeded the foreground request budget. Try again shortly.")
		return
	}
	if libraryID != "" {
		library, ok, err := s.discoveryLibraryForUser(user, libraryID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "suggestions_failed", "Unable to load suggestions.")
			return
		}
		if !ok {
			writeError(w, http.StatusForbidden, "forbidden", "You do not have access to that library.")
			return
		}
		suggestions, err := s.mediaSuggestionsForLibraryContext(ctx, user, library, limit)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusServiceUnavailable, "suggestions_timeout", "Suggestions exceeded the foreground request budget. Try again shortly.")
				return
			}
			writeError(w, http.StatusInternalServerError, "suggestions_failed", "Unable to load suggestions.")
			return
		}
		if err := ctx.Err(); err != nil {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "suggestions_timeout", "Suggestions exceeded the foreground request budget. Try again shortly.")
			return
		}
		response := SuggestionsResponse{
			Items:       suggestions,
			Rows:        s.discoveryRowsForLibraryContext(ctx, user, library),
			Total:       len(suggestions),
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		s.storeSuggestions(cacheKey, response)
		writeJSON(w, http.StatusOK, response)
		return
	}
	suggestions, err := s.mediaSuggestionsContext(ctx, user, limit)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "suggestions_timeout", "Suggestions exceeded the foreground request budget. Try again shortly.")
			return
		}
		writeError(w, http.StatusInternalServerError, "suggestions_failed", "Unable to load suggestions.")
		return
	}
	if err := ctx.Err(); err != nil {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "suggestions_timeout", "Suggestions exceeded the foreground request budget. Try again shortly.")
		return
	}
	response := SuggestionsResponse{
		Items:       suggestions,
		Rows:        s.discoveryRowsContext(ctx, user),
		Total:       len(suggestions),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.storeSuggestions(cacheKey, response)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) libraryDiscoverContext(ctx context.Context, user User, library Library, limit int) (SuggestionsResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit = clampInt(limit, 1, 50)
	if err := ctx.Err(); err != nil {
		return SuggestionsResponse{}, err
	}
	suggestions, err := s.mediaSuggestionsForLibraryContext(ctx, user, library, limit)
	if err != nil {
		return SuggestionsResponse{}, err
	}
	rows := []HomeRow{}
	if row, err := s.libraryContinueRowContext(ctx, user, library, limit); err != nil {
		return SuggestionsResponse{}, err
	} else if len(row.Items) > 0 {
		rows = append(rows, row)
	}
	if row, err := s.libraryRecentRowContext(ctx, user, library, limit); err != nil {
		return SuggestionsResponse{}, err
	} else if len(row.Items) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, s.discoveryRowsForLibraryContext(ctx, user, library)...)
	return SuggestionsResponse{
		Items:       suggestions,
		Rows:        rows,
		Total:       len(suggestions),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, ctx.Err()
}

func (s *Server) libraryContinueRowContext(ctx context.Context, user User, library Library, limit int) (HomeRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit = clampInt(limit, 1, 50)
	title := "Continue Watching"
	rowType := "continue"
	if library.Type == "music" || library.Type == "audiobook" {
		title = "Continue Listening"
	}
	if library.Type == "music" {
		rowType = "square"
	}
	if libraryHiddenFrom(library, "continue") {
		row := HomeRow{ID: "library_continue", Title: title, Type: rowType, LibraryID: library.ID, Limit: limit}
		row.ArtworkShape = resolvedHomeRowArtworkShape(row)
		return row, nil
	}
	progressPreferences := normalizePlaybackProgressPreferences(user.Preferences.PlaybackProgress)
	where := "WHERE m.library_id = ?"
	args := []any{library.ID}
	if library.Type == "music" || library.Type == "audiobook" {
		where += " AND ums.watched = 0 AND ((m.type = 'track' AND COALESCE(ums.last_played_at, '') <> '') OR (m.type = 'audiobook' AND ums.progress_seconds > 0 AND (m.duration_seconds <= 0 OR ((COALESCE(ums.progress_seconds, 0) * 100.0 / m.duration_seconds) >= ? AND (COALESCE(ums.progress_seconds, 0) * 100.0 / m.duration_seconds) < ?))))"
		args = append(args, progressPreferences.StartedThresholdPercent, progressPreferences.PlayedThresholdPercent)
	} else {
		where += " AND ums.progress_seconds > 0 AND ums.watched = 0 AND m.type NOT IN ('track', 'album', 'artist', 'audiobook') AND (m.duration_seconds <= 0 OR ((COALESCE(ums.progress_seconds, 0) * 100.0 / m.duration_seconds) >= ? AND (COALESCE(ums.progress_seconds, 0) * 100.0 / m.duration_seconds) < ?))"
		args = append(args, progressPreferences.StartedThresholdPercent, progressPreferences.PlayedThresholdPercent)
	}
	where, args = s.applyMediaVisibilityRestrictionSQL(viewerProfileID(user), where, args)
	args = append(args, limit)
	items, err := s.queryMediaListItemsContext(ctx, viewerProfileID(user), where+" ORDER BY COALESCE(ums.last_played_at, ums.updated_at) DESC, m.id ASC LIMIT ?", args)
	if err != nil {
		return HomeRow{}, err
	}
	if library.Type != "music" && library.Type != "audiobook" {
		items = s.resolveContinueWatchingItems(viewerProfileID(user), items)
	}
	items = filterLibraryMediaItems(library.ID, items)
	row := HomeRow{ID: "library_continue", Title: title, Type: rowType, LibraryID: library.ID, Items: items, Total: len(items), Limit: limit}
	row.ArtworkShape = resolvedHomeRowArtworkShape(row)
	return row, nil
}

func (s *Server) libraryRecentRowContext(ctx context.Context, user User, library Library, limit int) (HomeRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit = clampInt(limit, 1, 50)
	where := "WHERE m.library_id = ? AND m.parent_id IS NULL"
	args := []any{library.ID}
	where, args = s.applyMediaVisibilityRestrictionSQL(viewerProfileID(user), where, args)
	args = append(args, limit)
	items, err := s.queryMediaListItemsContext(ctx, viewerProfileID(user), where+" ORDER BY m.added_at DESC, m.sort_title ASC, m.id ASC LIMIT ?", args)
	if err != nil {
		return HomeRow{}, err
	}
	rowType := "poster"
	if library.Type == "music" {
		rowType = "square"
	}
	row := HomeRow{ID: "library_recent", Title: "Recently Added", Type: rowType, LibraryID: library.ID, Items: items, Total: len(items), Limit: limit}
	row.ArtworkShape = resolvedHomeRowArtworkShape(row)
	return row, nil
}

func filterLibraryMediaItems(libraryID string, items []MediaItem) []MediaItem {
	filtered := items[:0]
	for _, item := range items {
		if item.LibraryID == libraryID {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func suggestionsCacheKey(userID, libraryID string, limit int) string {
	return strings.TrimSpace(userID) + "\x00" + strings.TrimSpace(libraryID) + "\x00" + strconv.Itoa(limit)
}

func (s *Server) cachedSuggestions(key string) (SuggestionsResponse, bool) {
	if key == "" {
		return SuggestionsResponse{}, false
	}
	now := time.Now()
	s.suggestionsCacheMu.Lock()
	defer s.suggestionsCacheMu.Unlock()
	if s.suggestionsCache == nil {
		s.suggestionsCache = map[string]suggestionsCacheEntry{}
		return SuggestionsResponse{}, false
	}
	entry, ok := s.suggestionsCache[key]
	if !ok {
		return SuggestionsResponse{}, false
	}
	if now.After(entry.expiresAt) {
		delete(s.suggestionsCache, key)
		return SuggestionsResponse{}, false
	}
	return cloneSuggestionsResponse(entry.response), true
}

func (s *Server) storeSuggestions(key string, response SuggestionsResponse) {
	if key == "" {
		return
	}
	s.suggestionsCacheMu.Lock()
	defer s.suggestionsCacheMu.Unlock()
	if s.suggestionsCache == nil {
		s.suggestionsCache = map[string]suggestionsCacheEntry{}
	}
	now := time.Now()
	for cachedKey, entry := range s.suggestionsCache {
		if !now.Before(entry.expiresAt) {
			delete(s.suggestionsCache, cachedKey)
		}
	}
	for len(s.suggestionsCache) >= 512 {
		for cachedKey := range s.suggestionsCache {
			delete(s.suggestionsCache, cachedKey)
			break
		}
	}
	s.suggestionsCache[key] = suggestionsCacheEntry{
		response:  cloneSuggestionsResponse(response),
		expiresAt: now.Add(15 * time.Second),
	}
}

func (s *Server) invalidateSuggestionsCache() {
	s.suggestionsCacheMu.Lock()
	defer s.suggestionsCacheMu.Unlock()
	if len(s.suggestionsCache) > 0 {
		s.suggestionsCache = map[string]suggestionsCacheEntry{}
	}
}

func cloneSuggestionsResponse(response SuggestionsResponse) SuggestionsResponse {
	response.Items = append([]MediaSuggestion(nil), response.Items...)
	response.Rows = append([]HomeRow(nil), response.Rows...)
	for index := range response.Rows {
		response.Rows[index].Items = append([]MediaItem(nil), response.Rows[index].Items...)
	}
	return response
}

func (s *Server) discoveryLibraryForUser(user User, libraryID string) (Library, bool, error) {
	libraries, err := s.listLibrariesForUser(user)
	if err != nil {
		return Library{}, false, err
	}
	for _, library := range libraries {
		if library.ID == libraryID {
			return library, true, nil
		}
	}
	return Library{}, false, nil
}

func (s *Server) mediaSuggestions(user User, limit int) ([]MediaSuggestion, error) {
	return s.mediaSuggestionsContext(context.Background(), user, limit)
}

func (s *Server) mediaSuggestionsContext(ctx context.Context, user User, limit int) ([]MediaSuggestion, error) {
	if limit <= 0 {
		limit = 24
	}
	scored, err := s.localRecommendationScoresContext(ctx, viewerProfileID(user), limit)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	orderedIDs := make([]string, 0, limit)
	scores := map[string]scoredMedia{}
	for _, item := range scored {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		orderedIDs = append(orderedIDs, item.ID)
		scores[item.ID] = item
		if len(orderedIDs) >= limit {
			break
		}
	}
	if len(orderedIDs) < limit {
		for _, item := range s.localTrendingItemsContext(ctx, viewerProfileID(user)) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			orderedIDs = append(orderedIDs, item.ID)
			scores[item.ID] = scoredMedia{ID: item.ID, Score: item.CommunityRating, Reason: "Trending in your accessible libraries", Source: "local_trending"}
			if len(orderedIDs) >= limit {
				break
			}
		}
	}
	items, err := s.mediaByOrderedIDsContext(ctx, viewerProfileID(user), orderedIDs)
	if err != nil {
		return nil, err
	}
	suggestions := make([]MediaSuggestion, 0, len(items))
	for _, item := range items {
		score := scores[item.ID]
		reason := firstNonEmpty(score.Reason, "Recommended from library activity")
		source := firstNonEmpty(score.Source, "local_recommendations")
		suggestions = append(suggestions, MediaSuggestion{Item: item, Reason: reason, Source: source, Score: score.Score})
	}
	return suggestions, nil
}

func (s *Server) discoveryRows(user User) []HomeRow {
	return s.discoveryRowsContext(context.Background(), user)
}

func (s *Server) discoveryRowsContext(ctx context.Context, user User) []HomeRow {
	var rows []HomeRow
	if err := ctx.Err(); err != nil {
		return rows
	}
	if trending := s.trendingRowContext(ctx, user); len(trending.Items) > 0 {
		rows = append(rows, trending)
	}
	if err := ctx.Err(); err != nil {
		return rows
	}
	if watching := s.serverWatchingRowContext(ctx, user); len(watching.Items) > 0 {
		rows = append(rows, watching)
	}
	if err := ctx.Err(); err != nil {
		return rows
	}
	rows = append(rows, s.recommendationRowsContext(ctx, user)...)
	return resolveHomeRowArtworkShapes(rows)
}

func (s *Server) discoveryRowsForLibrary(user User, library Library) []HomeRow {
	return s.discoveryRowsForLibraryContext(context.Background(), user, library)
}

func (s *Server) discoveryRowsForLibraryContext(ctx context.Context, user User, library Library) []HomeRow {
	var rows []HomeRow
	if err := ctx.Err(); err != nil {
		return rows
	}
	if watching := s.serverWatchingRowForLibraryContext(ctx, user, library); len(watching.Items) > 0 {
		rows = append(rows, watching)
	}
	if err := ctx.Err(); err != nil {
		return rows
	}
	if trending := s.trendingRowForLibraryContext(ctx, user, library); len(trending.Items) > 0 {
		rows = append(rows, trending)
	}
	if err := ctx.Err(); err != nil {
		return rows
	}
	rows = append(rows, s.recommendationRowsForLibraryContext(ctx, user, library)...)
	return resolveLibraryHomeRowArtworkShapes(rows, library)
}

func (s *Server) trendingRow(user User) HomeRow {
	return s.trendingRowContext(context.Background(), user)
}

func (s *Server) trendingRowContext(ctx context.Context, user User) HomeRow {
	items := s.cachedTrendingMatchesContext(ctx, viewerProfileID(user))
	if len(items) == 0 && s.tmdbConfigured() {
		s.queueTMDBTrendingRefresh("all", "day")
	}
	if len(items) == 0 {
		items = s.localTrendingItemsContext(ctx, viewerProfileID(user))
	}
	return HomeRow{ID: "tmdb_trending", Title: "Trending Now", Type: "poster", Items: items}
}

func (s *Server) trendingRowForLibrary(user User, library Library) HomeRow {
	return s.trendingRowForLibraryContext(context.Background(), user, library)
}

func (s *Server) trendingRowForLibraryContext(ctx context.Context, user User, library Library) HomeRow {
	items := s.cachedTrendingMatchesForLibraryContext(ctx, viewerProfileID(user), library)
	if len(items) == 0 && s.tmdbConfigured() && (library.Type == "movie" || library.Type == "show" || library.Type == "anime") {
		mediaType := "movie"
		if library.Type == "show" || library.Type == "anime" {
			mediaType = "tv"
		}
		s.queueTMDBTrendingRefresh(mediaType, "day")
	}
	if len(items) == 0 {
		items = s.localTrendingItemsForLibraryContext(ctx, viewerProfileID(user), library, 0)
	}
	items = s.normalizeLibraryDiscoveryItems(viewerProfileID(user), library, items, 0)
	return HomeRow{ID: "tmdb_trending", Title: "Trending Now", Type: "poster", LibraryID: library.ID, Items: items}
}

func (s *Server) localTrendingItems(userID string) []MediaItem {
	return s.localTrendingItemsContext(context.Background(), userID)
}

func (s *Server) localTrendingItemsContext(ctx context.Context, userID string) []MediaItem {
	items, err := s.queryMediaContext(ctx, userID, `
		WHERE m.parent_id IS NULL
			AND m.type IN ('movie', 'show', 'anime')
		ORDER BY
			(m.community_rating * 10)
			+ (m.critic_rating * 0.35)
			+ (COALESCE(ums.progress_seconds, 0) / 1800)
			+ (COALESCE(ums.rating, 0) * 4)
			+ (COALESCE(ums.watchlisted, 0) * 8)
			+ CASE WHEN m.added_at >= datetime('now', '-30 days') THEN 10 ELSE 0 END
			DESC,
			m.added_at DESC
		LIMIT ?`, []any{homeRowItemLimit})
	if err != nil {
		return nil
	}
	return items
}

func (s *Server) localTrendingItemsForLibrary(userID string, library Library, limit int) []MediaItem {
	return s.localTrendingItemsForLibraryContext(context.Background(), userID, library, limit)
}

func (s *Server) localTrendingItemsForLibraryContext(ctx context.Context, userID string, library Library, limit int) []MediaItem {
	types := libraryDiscoveryCandidateTypes(library)
	if len(types) == 0 {
		return nil
	}
	typeClause, typeArgs := sqlInList("m.type", types)
	clause := `
		WHERE m.library_id = ?
			AND ` + typeClause + `
			` + libraryDiscoveryParentClause(library) + `
		ORDER BY
			(m.community_rating * 10)
			+ (m.critic_rating * 0.35)
			+ (COALESCE(ums.progress_seconds, 0) / 1800)
			+ (COALESCE(ums.rating, 0) * 4)
			+ (COALESCE(ums.watchlisted, 0) * 8)
			+ CASE WHEN m.added_at >= datetime('now', '-30 days') THEN 10 ELSE 0 END
			DESC,
			m.added_at DESC` + optionalLimitClause(limit)
	args := append([]any{library.ID}, typeArgs...)
	if limit > 0 {
		args = append(args, limit)
	}
	items, err := s.queryMediaContext(ctx, userID, clause, args)
	if err != nil {
		return nil
	}
	return items
}

func (s *Server) serverWatchingRow(user User) HomeRow {
	return s.serverWatchingRowContext(context.Background(), user)
}

func (s *Server) serverWatchingRowContext(ctx context.Context, user User) HomeRow {
	items := s.recentServerWatchingItemsContext(ctx, viewerProfileID(user), homeRowItemLimit)
	return HomeRow{ID: "server_watching_week", Title: "People On This Server Are Watching", Type: "poster", Items: items}
}

func (s *Server) serverWatchingRowForLibrary(user User, library Library) HomeRow {
	return s.serverWatchingRowForLibraryContext(context.Background(), user, library)
}

func (s *Server) serverWatchingRowForLibraryContext(ctx context.Context, user User, library Library) HomeRow {
	profileID := viewerProfileID(user)
	items := s.recentServerWatchingItemsForLibraryContext(ctx, profileID, library, 0)
	items = s.normalizeLibraryDiscoveryItems(profileID, library, items, 0)
	return HomeRow{ID: "server_watching_week", Title: libraryListeningLabel(library.Type, "People On This Server Are Watching", "People On This Server Are Listening"), Type: "poster", LibraryID: library.ID, Items: items}
}

func (s *Server) recentServerWatchingItems(userID string, limit int) []MediaItem {
	return s.recentServerWatchingItemsContext(context.Background(), userID, limit)
}

func (s *Server) recentServerWatchingItemsContext(ctx context.Context, profileID string, limit int) []MediaItem {
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	queryLimit := limit
	if queryLimit > 0 {
		queryLimit = queryLimit * 3
	}
	items, err := s.queryMediaContext(ctx, profileID, `
		JOIN (
			SELECT display_media_id AS media_id, COUNT(*) AS activity_count, COUNT(DISTINCT profile_id) AS viewer_count, MAX(activity_at) AS last_activity_at
			FROM (
				SELECT
					CASE
						WHEN activity_media.type IN ('movie', 'show', 'anime') AND COALESCE(activity_media.parent_id, '') = '' THEN activity_media.id
						WHEN activity_media.type = 'season' THEN COALESCE(activity_media.parent_id, '')
						WHEN activity_media.type = 'episode' AND COALESCE(activity_grandparent.id, '') <> '' THEN activity_grandparent.id
						WHEN activity_media.type = 'episode' AND activity_parent.type IN ('show', 'anime') THEN activity_parent.id
						ELSE ''
					END AS display_media_id,
					activity.profile_id,
					activity.activity_at
				FROM (
					SELECT media_id, profile_id, COALESCE(NULLIF(ended_at, ''), NULLIF(last_seen_at, ''), started_at) AS activity_at
					FROM playback_sessions
					WHERE is_live = 0
						AND COALESCE(history_paused, 0) = 0
						AND TRIM(media_id) <> ''
						AND COALESCE(NULLIF(ended_at, ''), NULLIF(last_seen_at, ''), started_at) >= ?
					UNION ALL
					SELECT media_id, profile_id, COALESCE(NULLIF(last_played_at, ''), updated_at) AS activity_at
					FROM user_media_state
					WHERE COALESCE(NULLIF(last_played_at, ''), updated_at) >= ?
						AND (progress_seconds > 0 OR watched = 1)
				) activity
				JOIN profiles activity_profile ON activity_profile.id = activity.profile_id AND activity_profile.disabled_at = ''
				JOIN media_items activity_media ON activity_media.id = activity.media_id
				LEFT JOIN media_items activity_parent ON activity_parent.id = activity_media.parent_id
				LEFT JOIN media_items activity_grandparent ON activity_grandparent.id = activity_parent.parent_id
				WHERE activity.profile_id = ? OR COALESCE(json_extract(activity_profile.preferences_json, '$.privacy.showActivityToMembers'), 1) <> 0
			)
			WHERE TRIM(display_media_id) <> ''
			GROUP BY display_media_id
			HAVING COUNT(DISTINCT profile_id) >= 2
		) watching ON watching.media_id = m.id
		WHERE m.parent_id IS NULL
			AND m.type IN ('movie', 'show', 'anime')
		ORDER BY watching.viewer_count DESC, watching.activity_count DESC, watching.last_activity_at DESC, m.community_rating DESC, m.sort_title ASC`+optionalLimitClause(queryLimit), append([]any{cutoff, cutoff, profileID}, optionalLimitArgs(queryLimit)...))
	if err != nil {
		return nil
	}
	items = s.normalizeServerWatchingItems(profileID, items, limit)
	return s.filterItemsByLibraryVisibility(items, "discover")
}

func (s *Server) recentServerWatchingItemsForLibrary(userID string, library Library, limit int) []MediaItem {
	return s.recentServerWatchingItemsForLibraryContext(context.Background(), userID, library, limit)
}

func (s *Server) recentServerWatchingItemsForLibraryContext(ctx context.Context, profileID string, library Library, limit int) []MediaItem {
	types := libraryDiscoveryActivityTypes(library)
	if len(types) == 0 {
		return nil
	}
	typeClause, typeArgs := sqlInList("m.type", types)
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	clause := `
		JOIN (
			SELECT activity.media_id, COUNT(*) AS activity_count, COUNT(DISTINCT activity.profile_id) AS viewer_count, MAX(activity.activity_at) AS last_activity_at
			FROM (
				SELECT media_id, profile_id, COALESCE(NULLIF(ended_at, ''), NULLIF(last_seen_at, ''), started_at) AS activity_at
				FROM playback_sessions
				WHERE is_live = 0
					AND COALESCE(history_paused, 0) = 0
					AND TRIM(media_id) <> ''
					AND COALESCE(NULLIF(ended_at, ''), NULLIF(last_seen_at, ''), started_at) >= ?
				UNION ALL
				SELECT media_id, profile_id, COALESCE(NULLIF(last_played_at, ''), updated_at) AS activity_at
				FROM user_media_state
				WHERE COALESCE(NULLIF(last_played_at, ''), updated_at) >= ?
					AND (progress_seconds > 0 OR watched = 1)
			) activity
			JOIN profiles activity_profile ON activity_profile.id = activity.profile_id AND activity_profile.disabled_at = ''
			WHERE TRIM(activity.media_id) <> ''
				AND (activity.profile_id = ? OR COALESCE(json_extract(activity_profile.preferences_json, '$.privacy.showActivityToMembers'), 1) <> 0)
			GROUP BY activity.media_id
			HAVING COUNT(DISTINCT activity.profile_id) >= 2
		) watching ON watching.media_id = m.id
		WHERE m.library_id = ?
			AND ` + typeClause + `
		ORDER BY watching.viewer_count DESC, watching.activity_count DESC, watching.last_activity_at DESC, m.community_rating DESC, m.sort_title ASC` + optionalLimitClause(limit)
	args := append([]any{cutoff, cutoff, profileID, library.ID}, typeArgs...)
	args = append(args, optionalLimitArgs(limit)...)
	items, err := s.queryMediaContext(ctx, profileID, clause, args)
	if err != nil {
		return nil
	}
	return items
}

func (s *Server) cachedTrendingMatches(userID string) []MediaItem {
	return s.cachedTrendingMatchesContext(context.Background(), userID)
}

func (s *Server) cachedTrendingMatchesContext(ctx context.Context, userID string) []MediaItem {
	entries, err := s.loadTMDBTrendingContext(ctx, "all", "day")
	if err != nil || len(entries) == 0 {
		return nil
	}
	return s.cachedTrendingMatchesForEntriesContext(ctx, userID, entries, nil)
}

func (s *Server) cachedTrendingMatchesForLibrary(userID string, library Library) []MediaItem {
	return s.cachedTrendingMatchesForLibraryContext(context.Background(), userID, library)
}

func (s *Server) cachedTrendingMatchesForLibraryContext(ctx context.Context, userID string, library Library) []MediaItem {
	mediaType := "all"
	if library.Type == "movie" {
		mediaType = "movie"
	}
	if library.Type == "show" || library.Type == "anime" {
		mediaType = "tv"
	}
	entries, err := s.loadTMDBTrendingContext(ctx, mediaType, "day")
	if err != nil || len(entries) == 0 {
		entries, err = s.loadTMDBTrendingContext(ctx, "all", "day")
		if err != nil || len(entries) == 0 {
			return nil
		}
	}
	return s.cachedTrendingMatchesForEntriesContext(ctx, userID, entries, &library)
}

func (s *Server) cachedTrendingMatchesForEntriesContext(ctx context.Context, userID string, entries []tmdbTrendingEntry, library *Library) []MediaItem {
	if len(entries) == 0 {
		return nil
	}
	types := []string{"movie", "show", "anime"}
	if library != nil {
		types = libraryDiscoveryCandidateTypes(*library)
	}
	if len(types) == 0 {
		return nil
	}
	orderedIDs := s.cachedTrendingProviderMatchIDsContext(ctx, entries, library, types)
	if len(orderedIDs) < homeRowItemLimit {
		seen := map[string]bool{}
		for _, id := range orderedIDs {
			seen[id] = true
		}
		entryKeys := trendingEntryKeySet(entries)
		for _, item := range s.cachedTrendingTitleCandidatesContext(ctx, userID, entries, library, types) {
			if err := ctx.Err(); err != nil {
				break
			}
			if item.ID == "" || seen[item.ID] {
				continue
			}
			for _, key := range mediaMatchKeys(item.Title, item.OriginalTitle, item.Year) {
				if !entryKeys[key] {
					continue
				}
				seen[item.ID] = true
				orderedIDs = append(orderedIDs, item.ID)
				break
			}
			if len(orderedIDs) >= homeRowItemLimit {
				break
			}
		}
	}
	items, err := s.mediaByOrderedIDsContext(ctx, userID, orderedIDs)
	if err != nil {
		return nil
	}
	if library != nil {
		items = s.normalizeLibraryDiscoveryItems(userID, *library, items, homeRowItemLimit)
	}
	return items
}

func (s *Server) cachedTrendingProviderMatchIDsContext(ctx context.Context, entries []tmdbTrendingEntry, library *Library, types []string) []string {
	externalIDs := make([]string, 0, len(entries))
	entryIndexByExternalID := map[string]int{}
	for index, entry := range entries {
		if entry.ID <= 0 {
			continue
		}
		externalID := strconv.Itoa(entry.ID)
		if _, exists := entryIndexByExternalID[externalID]; exists {
			continue
		}
		entryIndexByExternalID[externalID] = index
		externalIDs = append(externalIDs, externalID)
	}
	if len(externalIDs) == 0 {
		return nil
	}
	typeClause, typeArgs := sqlInList("m.type", types)
	args := make([]any, 0, 2+len(externalIDs)+len(typeArgs))
	for _, externalID := range externalIDs {
		args = append(args, externalID)
	}
	args = append(args, typeArgs...)
	libraryClause := ""
	if library != nil {
		libraryClause = "AND m.library_id = ?"
		args = append(args, library.ID)
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT p.external_id, m.id
		FROM media_provider_ids p
		JOIN media_items m ON m.id = p.media_id
		WHERE p.provider = 'tmdb'
			AND p.status = 'accepted'
			AND p.external_id IN (`+sqlPlaceholders(len(externalIDs))+`)
			AND m.parent_id IS NULL
			AND `+typeClause+`
			`+libraryClause, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	type providerMatch struct {
		index int
		id    string
	}
	var matches []providerMatch
	seen := map[string]bool{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			break
		}
		var externalID, mediaID string
		if err := rows.Scan(&externalID, &mediaID); err != nil {
			continue
		}
		if mediaID == "" || seen[mediaID] {
			continue
		}
		index, ok := entryIndexByExternalID[externalID]
		if !ok {
			continue
		}
		seen[mediaID] = true
		matches = append(matches, providerMatch{index: index, id: mediaID})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].index < matches[j].index
	})
	ids := make([]string, 0, min(len(matches), homeRowItemLimit))
	for _, match := range matches {
		ids = append(ids, match.id)
		if len(ids) >= homeRowItemLimit {
			break
		}
	}
	return ids
}

func (s *Server) cachedTrendingTitleCandidatesContext(ctx context.Context, userID string, entries []tmdbTrendingEntry, library *Library, types []string) []MediaItem {
	titles := trendingEntryTitleSet(entries)
	if len(titles) == 0 {
		return nil
	}
	orderedTitles := sortedRecommendationIDs(titles)
	typeClause, typeArgs := sqlInList("m.type", types)
	titlePlaceholders := sqlPlaceholders(len(orderedTitles))
	args := make([]any, 0, len(typeArgs)+len(orderedTitles)*3+3)
	for _, mediaType := range typeArgs {
		args = append(args, mediaType)
	}
	libraryClause := ""
	if library != nil {
		libraryClause = "AND m.library_id = ?"
		args = append(args, library.ID)
	}
	for i := 0; i < 3; i++ {
		for _, title := range orderedTitles {
			args = append(args, title)
		}
	}
	limit := min(max(len(entries)*4, homeRowItemLimit), homeRowItemLimit*4)
	args = append(args, limit)
	byKey := map[string]MediaItem{}
	items, err := s.queryMediaContext(ctx, userID, `
		WHERE m.parent_id IS NULL
			AND `+typeClause+`
			`+libraryClause+`
			AND (
				lower(trim(m.title)) IN (`+titlePlaceholders+`)
				OR lower(trim(m.original_title)) IN (`+titlePlaceholders+`)
				OR lower(trim(m.sort_title)) IN (`+titlePlaceholders+`)
			)
		ORDER BY m.community_rating DESC, m.added_at DESC
		LIMIT ?`, args)
	if err != nil {
		return nil
	}
	for _, item := range items {
		keys := mediaMatchKeys(item.Title, item.OriginalTitle, item.Year)
		for _, key := range keys {
			if _, exists := byKey[key]; !exists {
				byKey[key] = item
			}
		}
	}
	var matched []MediaItem
	seen := map[string]bool{}
	for _, entry := range entries {
		title := entry.displayTitle()
		year := entry.displayYear()
		for _, key := range mediaMatchKeys(title, "", year) {
			item, ok := byKey[key]
			if !ok || seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			matched = append(matched, item)
			break
		}
	}
	return matched
}

func trendingEntryKeySet(entries []tmdbTrendingEntry) map[string]bool {
	keys := map[string]bool{}
	for _, entry := range entries {
		for _, key := range mediaMatchKeys(entry.displayTitle(), "", entry.displayYear()) {
			keys[key] = true
		}
	}
	return keys
}

func trendingEntryTitleSet(entries []tmdbTrendingEntry) map[string]bool {
	titles := map[string]bool{}
	for _, entry := range entries {
		title := strings.ToLower(strings.TrimSpace(entry.displayTitle()))
		if title != "" {
			titles[title] = true
		}
	}
	return titles
}

func (s *Server) loadTMDBTrending(mediaType, timeWindow string) ([]tmdbTrendingEntry, error) {
	return s.loadTMDBTrendingContext(context.Background(), mediaType, timeWindow)
}

func (s *Server) loadTMDBTrendingContext(ctx context.Context, mediaType, timeWindow string) ([]tmdbTrendingEntry, error) {
	var payloadJSON string
	err := s.queryUserRow(ctx, `
		SELECT payload_json
		FROM tmdb_trending_cache
		WHERE media_type = ? AND time_window = ?`,
		mediaType, timeWindow,
	).Scan(&payloadJSON)
	if err != nil {
		return nil, err
	}
	var payload tmdbTrendingPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, err
	}
	return payload.Results, nil
}

func (s *Server) refreshTMDBTrending(ctx context.Context, mediaType, timeWindow string) error {
	if !s.tmdbConfigured() {
		return errors.New("TMDB API key is not configured")
	}
	if mediaType == "" {
		mediaType = "all"
	}
	if timeWindow == "" {
		timeWindow = "day"
	}
	baseURL := strings.TrimRight(s.cfg.TMDBBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.themoviedb.org/3"
	}
	endpoint, err := url.Parse(baseURL + "/trending/" + url.PathEscape(mediaType) + "/" + url.PathEscape(timeWindow))
	if err != nil {
		return err
	}
	query := endpoint.Query()
	query.Set("language", "en-US")
	if apiKey := s.tmdbAPIKey(); apiKey != "" && s.tmdbReadAccessToken() == "" {
		query.Set("api_key", apiKey)
	}
	endpoint.RawQuery = query.Encode()

	requestCtx, cancel := context.WithTimeout(ctx, tmdbRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if token := s.tmdbReadAccessToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("TMDB trending returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	var payload tmdbTrendingPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = s.execBackgroundWrite(ctx, `
		INSERT INTO tmdb_trending_cache (media_type, time_window, payload_json, fetched_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(media_type, time_window) DO UPDATE SET
			payload_json = excluded.payload_json,
			fetched_at = excluded.fetched_at,
			expires_at = excluded.expires_at`,
		mediaType, timeWindow, string(body), now.Format(time.RFC3339), now.Add(tmdbTrendingCacheTTL).Format(time.RFC3339),
	)
	return err
}

func (s *Server) refreshStaleTMDBTrending(ctx context.Context) {
	s.queueTMDBTrendingRefresh("all", "day")
}

func (s *Server) queueTMDBTrendingRefresh(mediaType, timeWindow string) {
	if !s.tmdbConfigured() {
		return
	}
	mediaType = firstNonEmpty(strings.TrimSpace(mediaType), "all")
	timeWindow = firstNonEmpty(strings.TrimSpace(timeWindow), "day")
	if !s.tmdbTrendingRefreshDue(mediaType, timeWindow) {
		return
	}
	resourceID := mediaType + "/" + timeWindow
	metadata := map[string]string{"mediaType": mediaType, "timeWindow": timeWindow}
	if _, err := s.createJobForWithMetadata("tmdb_trending_refresh", fmt.Sprintf("TMDB trending refresh queued for %s/%s.", mediaType, timeWindow), "discovery", resourceID, metadata); err != nil {
		s.log.Warn("tmdb trending refresh queue failed", "mediaType", mediaType, "timeWindow", timeWindow, "error", err)
	}
}

func (s *Server) tmdbTrendingRefreshDue(mediaType, timeWindow string) bool {
	return s.tmdbTrendingRefreshDueContext(context.Background(), mediaType, timeWindow)
}

func (s *Server) tmdbTrendingRefreshDueContext(ctx context.Context, mediaType, timeWindow string) bool {
	var expiresAt string
	err := s.queryUserRow(ctx, `
		SELECT expires_at
		FROM tmdb_trending_cache
		WHERE media_type = ? AND time_window = ?`,
		mediaType, timeWindow,
	).Scan(&expiresAt)
	if err == nil {
		if expires, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr == nil && time.Now().UTC().Before(expires) {
			return false
		}
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.recordLog("warn", "TMDB trending cache check failed", map[string]string{"error": err.Error()})
		return false
	}
	return true
}

func (s *Server) runTMDBTrendingRefresh(ctx context.Context, job Job) {
	mediaType := firstNonEmpty(job.Metadata["mediaType"], "all")
	timeWindow := firstNonEmpty(job.Metadata["timeWindow"], "day")
	if !s.tmdbTrendingRefreshDueContext(ctx, mediaType, timeWindow) {
		_ = s.setJobMessage(job.ID, "complete", 100, fmt.Sprintf("TMDB trending cache is already fresh for %s/%s.", mediaType, timeWindow))
		return
	}
	_ = s.setJobMessage(job.ID, "running", 20, fmt.Sprintf("Refreshing TMDB trending cache for %s/%s.", mediaType, timeWindow))
	if err := s.refreshTMDBTrending(ctx, mediaType, timeWindow); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := fmt.Sprintf("TMDB trending refresh failed for %s/%s: %s", mediaType, timeWindow, err.Error())
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("warn", message, map[string]string{"job": job.ID})
		return
	}
	message := fmt.Sprintf("TMDB trending refresh completed for %s/%s.", mediaType, timeWindow)
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, map[string]string{"job": job.ID})
}

func (s *Server) runDiscoveryRefresher(ctx context.Context) {
	s.refreshStaleTMDBTrending(ctx)
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshStaleTMDBTrending(ctx)
		}
	}
}

func (s *Server) recommendationRows(user User) []HomeRow {
	return s.recommendationRowsContext(context.Background(), user)
}

func (s *Server) recommendationRowsContext(ctx context.Context, user User) []HomeRow {
	scored, err := s.localRecommendationScoresContext(ctx, viewerProfileID(user), homeRowItemLimit)
	if len(scored) == 0 {
		return nil
	}
	var ids []string
	for _, item := range scored {
		ids = append(ids, item.ID)
	}
	items, err := s.mediaByOrderedIDsContext(ctx, viewerProfileID(user), ids)
	if err != nil || len(items) == 0 {
		return nil
	}
	return resolveHomeRowArtworkShapes([]HomeRow{{
		ID:    "recommended_for_you",
		Title: "Recommended For You",
		Type:  "poster",
		Items: items,
	}})
}

func (s *Server) recommendationRowsForLibrary(user User, library Library) []HomeRow {
	return s.recommendationRowsForLibraryContext(context.Background(), user, library)
}

func (s *Server) recommendationRowsForLibraryContext(ctx context.Context, user User, library Library) []HomeRow {
	scored, err := s.localRecommendationScoresForLibraryContext(ctx, viewerProfileID(user), library, homeRowItemLimit)
	if err != nil || len(scored) == 0 {
		return nil
	}
	var ids []string
	for _, item := range scored {
		ids = append(ids, item.ID)
	}
	items, err := s.mediaByOrderedIDsContext(ctx, viewerProfileID(user), ids)
	if err != nil || len(items) == 0 {
		return nil
	}
	items = s.normalizeLibraryDiscoveryItems(viewerProfileID(user), library, items, 0)
	if len(items) == 0 {
		return nil
	}
	return resolveLibraryHomeRowArtworkShapes([]HomeRow{{
		ID:        "recommended_for_you",
		Title:     "Recommended For You",
		Type:      "poster",
		LibraryID: library.ID,
		Items:     items,
	}}, library)
}

func (s *Server) mediaSuggestionsForLibrary(user User, library Library, limit int) ([]MediaSuggestion, error) {
	return s.mediaSuggestionsForLibraryContext(context.Background(), user, library, limit)
}

func (s *Server) mediaSuggestionsForLibraryContext(ctx context.Context, user User, library Library, limit int) ([]MediaSuggestion, error) {
	if limit <= 0 {
		limit = 24
	}
	scored, err := s.localRecommendationScoresForLibraryContext(ctx, viewerProfileID(user), library, limit)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	orderedIDs := make([]string, 0, limit)
	scores := map[string]scoredMedia{}
	for _, item := range scored {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		orderedIDs = append(orderedIDs, item.ID)
		scores[item.ID] = item
		if len(orderedIDs) >= limit {
			break
		}
	}
	if len(orderedIDs) < limit {
		for _, item := range s.localTrendingItemsForLibraryContext(ctx, viewerProfileID(user), library, limit*2) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			normalized := s.normalizeLibraryDiscoveryItems(viewerProfileID(user), library, []MediaItem{item}, 1)
			if len(normalized) == 0 {
				continue
			}
			item = normalized[0]
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			orderedIDs = append(orderedIDs, item.ID)
			scores[item.ID] = scoredMedia{ID: item.ID, Score: item.CommunityRating, Reason: "Trending in this library", Source: "local_trending"}
			if len(orderedIDs) >= limit {
				break
			}
		}
	}
	items, err := s.mediaByOrderedIDsContext(ctx, viewerProfileID(user), orderedIDs)
	if err != nil {
		return nil, err
	}
	items = s.normalizeLibraryDiscoveryItems(viewerProfileID(user), library, items, limit)
	suggestions := make([]MediaSuggestion, 0, len(items))
	for _, item := range items {
		score := scores[item.ID]
		reason := firstNonEmpty(score.Reason, "Recommended from this library")
		source := firstNonEmpty(score.Source, "local_recommendations")
		suggestions = append(suggestions, MediaSuggestion{Item: item, Reason: reason, Source: source, Score: score.Score})
	}
	return suggestions, nil
}

func (s *Server) mediaRecommendationRows(userID string, item MediaItem) []HomeRow {
	return s.mediaRecommendationRowsContext(context.Background(), userID, item)
}

func (s *Server) mediaRecommendationRowsContext(ctx context.Context, userID string, item MediaItem) []HomeRow {
	const maxRows = 8
	const rowLimit = 24

	rows := []HomeRow{}
	rowIDs := map[string]bool{}
	addRow := func(id, title, rowType string, items []MediaItem) {
		if len(rows) >= maxRows {
			return
		}
		id = recommendationRowID(id)
		title = strings.TrimSpace(title)
		if id == "" || title == "" || rowIDs[id] {
			return
		}
		items = dedupeRecommendationItems(items, mediaRecommendationContextExclusions(item), rowLimit)
		if len(items) == 0 {
			return
		}
		if rowType == "" {
			rowType = recommendationRowType(items)
		}
		rowIDs[id] = true
		row := HomeRow{ID: id, Title: title, Type: rowType, Items: items}
		row.ArtworkShape = resolvedHomeRowArtworkShape(row)
		rows = append(rows, row)
	}

	switch item.Type {
	case "episode":
		if title := firstNonEmpty(item.GrandparentTitle, item.ParentTitle); title != "" {
			addRow("more-from-"+firstNonEmpty(item.GrandparentID, item.ParentID), "More from "+title, "landscape", s.moreEpisodesFromShowContext(ctx, userID, item, rowLimit))
		}
	case "season":
		if item.ParentID != "" && item.ParentTitle != "" {
			addRow("more-seasons-"+item.ParentID, "More Seasons from "+item.ParentTitle, "poster", s.moreSeasonsFromShowContext(ctx, userID, item, rowLimit))
		}
	case "track":
		if item.ParentID != "" && item.ParentTitle != "" {
			addRow("more-from-album-"+item.ParentID, "More from "+item.ParentTitle, "landscape", s.moreTracksFromAlbumContext(ctx, userID, item, rowLimit))
		}
	}

	if err := ctx.Err(); err != nil {
		return rows
	}
	if related := s.relatedMediaItemsContext(ctx, userID, item, rowLimit); len(related) > 0 {
		addRow("related-"+item.ID, relatedRecommendationTitle(item), "", related)
	}

	if err := ctx.Err(); err != nil {
		return rows
	}
	for _, person := range s.importantRecommendationPeopleContext(ctx, s.recommendationPeopleForItemContext(ctx, userID, item), item.Type, 6) {
		if len(rows) >= maxRows {
			break
		}
		if err := ctx.Err(); err != nil {
			return rows
		}
		items, err := s.personMediaIdentityContext(ctx, userID, person.ID, rowLimit)
		if err != nil || len(items) == 0 {
			continue
		}
		items = dedupeRecommendationItems(items, mediaRecommendationContextExclusions(item), rowLimit)
		if len(items) == 0 {
			if identity, _, ok := s.personIdentityForPublicID(ctx, person.ID); ok && personIdentityNeedsRecommendationFallback(identity) {
				// Weak metadata is not sufficient to merge public person identities.
				// It can still contribute a name-and-role discovery signal without
				// changing the stable person URL or conflating person detail credits.
				items, err = s.personMediaRecommendationContext(ctx, userID, person.Name, person.Role, rowLimit)
				if err != nil {
					continue
				}
				items = dedupeRecommendationItems(items, mediaRecommendationContextExclusions(item), rowLimit)
			}
		}
		addRow("person-"+person.ID, personRecommendationRowTitle(person), "poster", items)
	}

	if len(rows) < maxRows {
		if network := strings.TrimSpace(firstNonEmpty(item.Network, item.TypedMetadata["network"])); network != "" {
			addRow("network-"+network, "More from "+network, "poster", s.mediaByNetworkContext(ctx, userID, item, network, rowLimit))
		}
	}
	if len(rows) < maxRows {
		if studio := strings.TrimSpace(firstNonEmpty(item.Studio, item.TypedMetadata["studio"], item.TypedMetadata["label"])); studio != "" {
			addRow("studio-"+studio, "More from "+studio, "poster", s.mediaByStudioContext(ctx, userID, item, studio, rowLimit))
		}
	}
	if len(rows) < maxRows {
		if artist := mediaArtistLabel(item); artist != "" {
			addRow("artist-"+artist, "More from "+artist, "", s.musicByArtistContext(ctx, userID, item, artist, rowLimit))
		}
	}
	if len(rows) < maxRows {
		if author := audiobookAuthorLabel(item); author != "" {
			addRow("author-"+author, "More by "+author, "poster", s.audiobooksByAuthorContext(ctx, userID, item, author, rowLimit))
		}
	}
	if len(rows) < maxRows {
		for _, genre := range topRecommendationGenres(item.Genres, 2) {
			addRow("genre-"+genre, genreRecommendationTitle(item, genre), "", s.mediaByGenreContext(ctx, userID, item, genre, rowLimit))
			if len(rows) >= maxRows {
				break
			}
		}
	}

	return rows
}

// Scanner-derived canonical keys keep a person's public URL stable across
// rescans without claiming that weak local metadata proves two credits are the
// same real person. When an exact identity has no additional media, the
// recommendation layer may still use matching name and role as a ranking
// signal. Explicit canonical keys and provider identities never take this
// fallback path.
func personIdentityNeedsRecommendationFallback(identity personIdentitySelector) bool {
	if identity.Kind == "unresolved" || identity.Kind == "fallback" {
		return true
	}
	if identity.Kind != "canonical" {
		return false
	}
	canonicalKey := strings.ToLower(strings.TrimSpace(identity.CanonicalKey))
	return strings.HasPrefix(canonicalKey, "credit:") || strings.HasPrefix(canonicalKey, "portrait:")
}

func (s *Server) recommendationPeopleForItem(userID string, item MediaItem) []MediaPerson {
	return s.recommendationPeopleForItemContext(context.Background(), userID, item)
}

func (s *Server) recommendationPeopleForItemContext(ctx context.Context, userID string, item MediaItem) []MediaPerson {
	people := append([]MediaPerson{}, item.People...)
	seen := map[string]bool{}
	for _, person := range people {
		key := strings.ToLower(strings.TrimSpace(person.Name)) + "\x00" + strings.ToLower(strings.TrimSpace(person.Role))
		if key != "\x00" {
			seen[key] = true
		}
	}
	var ids []string
	if item.ParentID != "" {
		ids = append(ids, item.ParentID)
	}
	if item.GrandparentID != "" {
		ids = append(ids, item.GrandparentID)
	}
	if len(ids) == 0 || (item.Type != "episode" && item.Type != "season") {
		return people
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT COALESCE(name, ''), COALESCE(role, ''), COALESCE(character, ''), COALESCE(image_url, ''), COALESCE(sort_order, 9999), COALESCE(source, '')
		FROM media_people
		WHERE media_id IN (`+sqlPlaceholders(len(ids))+`)
		ORDER BY sort_order ASC, name ASC`, args...)
	if err != nil {
		return people
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return people
		}
		var person MediaPerson
		if err := rows.Scan(&person.Name, &person.Role, &person.Character, &person.ImageURL, &person.SortOrder, &person.Source); err != nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(person.Name)) + "\x00" + strings.ToLower(strings.TrimSpace(person.Role))
		if key == "\x00" || seen[key] {
			continue
		}
		seen[key] = true
		people = append(people, person)
	}
	return people
}

func (s *Server) moreEpisodesFromShow(userID string, item MediaItem, limit int) []MediaItem {
	return s.moreEpisodesFromShowContext(context.Background(), userID, item, limit)
}

func (s *Server) moreEpisodesFromShowContext(ctx context.Context, userID string, item MediaItem, limit int) []MediaItem {
	showID := strings.TrimSpace(item.GrandparentID)
	if showID == "" {
		return nil
	}
	items, err := s.queryMediaListItemsContext(ctx, userID, `
		JOIN media_items recommendation_season ON recommendation_season.id = m.parent_id
		WHERE m.type = 'episode'
			AND recommendation_season.parent_id = ?
			AND m.id <> ?
		ORDER BY recommendation_season.index_number ASC, m.index_number ASC, m.sort_title ASC`+optionalLimitClause(limit), append([]any{showID, item.ID}, optionalLimitArgs(limit)...))
	if err != nil {
		return nil
	}
	return s.filterItemsByLibraryVisibility(items, "detail-recommendations")
}

func (s *Server) moreSeasonsFromShow(userID string, item MediaItem, limit int) []MediaItem {
	return s.moreSeasonsFromShowContext(context.Background(), userID, item, limit)
}

func (s *Server) moreSeasonsFromShowContext(ctx context.Context, userID string, item MediaItem, limit int) []MediaItem {
	if strings.TrimSpace(item.ParentID) == "" {
		return nil
	}
	items, err := s.queryMediaListItemsContext(ctx, userID, `
		WHERE m.type = 'season'
			AND m.parent_id = ?
			AND m.id <> ?
		ORDER BY m.index_number ASC, m.sort_title ASC`+optionalLimitClause(limit), append([]any{item.ParentID, item.ID}, optionalLimitArgs(limit)...))
	if err != nil {
		return nil
	}
	return s.filterItemsByLibraryVisibility(items, "detail-recommendations")
}

func (s *Server) moreTracksFromAlbum(userID string, item MediaItem, limit int) []MediaItem {
	return s.moreTracksFromAlbumContext(context.Background(), userID, item, limit)
}

func (s *Server) moreTracksFromAlbumContext(ctx context.Context, userID string, item MediaItem, limit int) []MediaItem {
	if strings.TrimSpace(item.ParentID) == "" {
		return nil
	}
	items, err := s.queryMediaListItemsContext(ctx, userID, `
		WHERE m.type = 'track'
			AND m.parent_id = ?
			AND m.id <> ?
		ORDER BY m.index_number ASC, m.sort_title ASC`+optionalLimitClause(limit), append([]any{item.ParentID, item.ID}, optionalLimitArgs(limit)...))
	if err != nil {
		return nil
	}
	return s.filterItemsByLibraryVisibility(items, "detail-recommendations")
}

func (s *Server) relatedMediaItems(userID string, item MediaItem, limit int) []MediaItem {
	return s.relatedMediaItemsContext(context.Background(), userID, item, limit)
}

func (s *Server) relatedMediaItemsContext(ctx context.Context, userID string, item MediaItem, limit int) []MediaItem {
	switch item.Type {
	case "movie", "show", "anime", "season", "episode":
		return s.relatedVisualMediaItemsContext(ctx, userID, item, limit)
	case "artist", "album", "track":
		artist := mediaArtistLabel(item)
		if artist != "" {
			return s.musicByArtistContext(ctx, userID, item, artist, limit)
		}
	case "audiobook":
		if author := audiobookAuthorLabel(item); author != "" {
			return s.audiobooksByAuthorContext(ctx, userID, item, author, limit)
		}
	}
	return nil
}

func (s *Server) relatedVisualMediaItems(userID string, item MediaItem, limit int) []MediaItem {
	return s.relatedVisualMediaItemsContext(context.Background(), userID, item, limit)
}

func (s *Server) relatedVisualMediaItemsContext(ctx context.Context, userID string, item MediaItem, limit int) []MediaItem {
	types := visualRecommendationTypes(item)
	if len(types) == 0 {
		return nil
	}
	genres := topRecommendationGenres(item.Genres, 4)
	exclusions := mediaRecommendationContextExclusions(item)
	var where []string
	var args []any
	where = append(where, "m.parent_id IS NULL")
	where = append(where, "m.type IN ("+sqlPlaceholders(len(types))+")")
	for _, mediaType := range types {
		args = append(args, mediaType)
	}
	if len(exclusions) > 0 {
		ids := sortedRecommendationIDs(exclusions)
		where = append(where, "m.id NOT IN ("+sqlPlaceholders(len(ids))+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}

	var match []string
	if len(genres) > 0 {
		match = append(match, `EXISTS (
			SELECT 1
			FROM media_category_facets recommendation_genre
			WHERE recommendation_genre.media_id = m.id
				AND recommendation_genre.library_id = COALESCE(m.library_id, '')
				AND recommendation_genre.facet_type = 'genre'
				AND recommendation_genre.sort_value IN (`+sqlPlaceholders(len(genres))+`)
		)`)
		for _, genre := range genres {
			args = append(args, strings.ToLower(genre))
		}
	}
	if studio := strings.ToLower(strings.TrimSpace(item.Studio)); studio != "" {
		match = append(match, "m.filter_studio_key = ?")
		args = append(args, studio)
	}
	if network := strings.ToLower(strings.TrimSpace(firstNonEmpty(item.Network, item.TypedMetadata["network"]))); network != "" {
		match = append(match, "m.filter_network_key = ?")
		args = append(args, network)
	}
	if item.Year > 0 {
		match = append(match, "(m.year / 10) = ?")
		args = append(args, item.Year/10)
	}
	if len(match) == 0 {
		return nil
	}
	where = append(where, "("+strings.Join(match, " OR ")+")")

	order := []string{}
	if len(genres) > 0 {
		order = append(order, `(SELECT COUNT(1)
			FROM media_category_facets recommendation_genre_score
			WHERE recommendation_genre_score.media_id = m.id
				AND recommendation_genre_score.library_id = COALESCE(m.library_id, '')
				AND recommendation_genre_score.facet_type = 'genre'
				AND recommendation_genre_score.sort_value IN (`+sqlPlaceholders(len(genres))+`)
		) DESC`)
		for _, genre := range genres {
			args = append(args, strings.ToLower(genre))
		}
	}
	if item.Year > 0 {
		order = append(order, "ABS(m.year - ?) ASC")
		args = append(args, item.Year)
	}
	order = append(order, "m.community_rating DESC", "m.critic_rating DESC", "m.added_at DESC", "m.sort_title ASC")

	args = append(args, optionalLimitArgs(limit)...)
	items, err := s.queryMediaListItemsContext(ctx, userID, `
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY `+strings.Join(order, ", ")+optionalLimitClause(limit), args)
	if err != nil {
		return nil
	}
	return s.filterItemsByLibraryVisibility(items, "detail-recommendations")
}

func (s *Server) mediaByNetwork(userID string, item MediaItem, network string, limit int) []MediaItem {
	return s.mediaByNetworkContext(context.Background(), userID, item, network, limit)
}

func (s *Server) mediaByNetworkContext(ctx context.Context, userID string, item MediaItem, network string, limit int) []MediaItem {
	network = strings.ToLower(strings.TrimSpace(network))
	if network == "" {
		return nil
	}
	types := visualRecommendationTypes(item)
	if len(types) == 0 {
		types = []string{"show", "anime"}
	}
	args := make([]any, 0, len(types)+3)
	for _, mediaType := range types {
		args = append(args, mediaType)
	}
	args = append(args, network, item.ID)
	args = append(args, optionalLimitArgs(limit)...)
	items, err := s.queryMediaListItemsContext(ctx, userID, `
		WHERE m.parent_id IS NULL
			AND m.type IN (`+sqlPlaceholders(len(types))+`)
			AND m.filter_network_key = ?
			AND m.id <> ?
		ORDER BY m.community_rating DESC, m.year DESC, m.sort_title ASC`+optionalLimitClause(limit), args)
	if err != nil {
		return nil
	}
	return s.filterItemsByLibraryVisibility(items, "detail-recommendations")
}

func (s *Server) mediaByStudio(userID string, item MediaItem, studio string, limit int) []MediaItem {
	return s.mediaByStudioContext(context.Background(), userID, item, studio, limit)
}

func (s *Server) mediaByStudioContext(ctx context.Context, userID string, item MediaItem, studio string, limit int) []MediaItem {
	studio = strings.ToLower(strings.TrimSpace(studio))
	if studio == "" {
		return nil
	}
	types := visualRecommendationTypes(item)
	if len(types) == 0 {
		types = []string{item.Type}
	}
	args := make([]any, 0, len(types)+4)
	for _, mediaType := range types {
		args = append(args, mediaType)
	}
	args = append(args, studio, studio, item.ID)
	args = append(args, optionalLimitArgs(limit)...)
	items, err := s.queryMediaListItemsContext(ctx, userID, `
		WHERE m.parent_id IS NULL
			AND m.type IN (`+sqlPlaceholders(len(types))+`)
			AND (m.filter_studio_key = ? OR m.filter_label_key = ?)
			AND m.id <> ?
		ORDER BY m.community_rating DESC, m.year DESC, m.sort_title ASC`+optionalLimitClause(limit), args)
	if err != nil {
		return nil
	}
	return s.filterItemsByLibraryVisibility(items, "detail-recommendations")
}

func (s *Server) mediaByGenre(userID string, item MediaItem, genre string, limit int) []MediaItem {
	return s.mediaByGenreContext(context.Background(), userID, item, genre, limit)
}

func (s *Server) mediaByGenreContext(ctx context.Context, userID string, item MediaItem, genre string, limit int) []MediaItem {
	genre = strings.ToLower(strings.TrimSpace(genre))
	if genre == "" {
		return nil
	}
	types := recommendationGenreTypes(item)
	if len(types) == 0 {
		return nil
	}
	args := make([]any, 0, len(types)+3)
	for _, mediaType := range types {
		args = append(args, mediaType)
	}
	args = append(args, genre, item.ID)
	args = append(args, optionalLimitArgs(limit)...)
	parentClause := "m.parent_id IS NULL"
	if item.Type == "track" {
		parentClause = "m.type = 'track'"
	}
	items, err := s.queryMediaListItemsContext(ctx, userID, `
		WHERE `+parentClause+`
			AND m.type IN (`+sqlPlaceholders(len(types))+`)
			AND EXISTS (
				SELECT 1
				FROM media_category_facets recommendation_genre
				WHERE recommendation_genre.media_id = m.id
					AND recommendation_genre.library_id = COALESCE(m.library_id, '')
					AND recommendation_genre.facet_type = 'genre'
					AND recommendation_genre.sort_value = ?
			)
			AND m.id <> ?
		ORDER BY m.community_rating DESC, m.critic_rating DESC, m.added_at DESC, m.sort_title ASC`+optionalLimitClause(limit), args)
	if err != nil {
		return nil
	}
	return s.filterItemsByLibraryVisibility(items, "detail-recommendations")
}

func (s *Server) musicByArtist(userID string, item MediaItem, artist string, limit int) []MediaItem {
	return s.musicByArtistContext(context.Background(), userID, item, artist, limit)
}

func (s *Server) musicByArtistContext(ctx context.Context, userID string, item MediaItem, artist string, limit int) []MediaItem {
	artist = strings.ToLower(strings.TrimSpace(artist))
	if artist == "" {
		return nil
	}
	items, err := s.queryMediaListItemsContext(ctx, userID, `
		WHERE m.id <> ?
			AND m.type IN ('artist', 'album', 'track')
			AND (m.filter_artist_key = ? OR (m.type = 'artist' AND lower(trim(m.title)) = ?))
		ORDER BY CASE m.type WHEN 'artist' THEN 0 WHEN 'album' THEN 1 ELSE 2 END, m.year DESC, m.index_number ASC, m.sort_title ASC`+optionalLimitClause(limit), append([]any{item.ID, artist, artist}, optionalLimitArgs(limit)...))
	if err != nil {
		return nil
	}
	return s.filterItemsByLibraryVisibility(items, "detail-recommendations")
}

func (s *Server) audiobooksByAuthor(userID string, item MediaItem, author string, limit int) []MediaItem {
	return s.audiobooksByAuthorContext(context.Background(), userID, item, author, limit)
}

func (s *Server) audiobooksByAuthorContext(ctx context.Context, userID string, item MediaItem, author string, limit int) []MediaItem {
	author = strings.ToLower(strings.TrimSpace(author))
	if author == "" {
		return nil
	}
	items, err := s.queryMediaListItemsContext(ctx, userID, `
		WHERE m.id <> ?
			AND m.type = 'audiobook'
			AND m.filter_author_key = ?
		ORDER BY m.year DESC, m.sort_title ASC`+optionalLimitClause(limit), append([]any{item.ID, author}, optionalLimitArgs(limit)...))
	if err != nil {
		return nil
	}
	return s.filterItemsByLibraryVisibility(items, "detail-recommendations")
}

func (s *Server) importantRecommendationPeople(people []MediaPerson, mediaType string, limit int) []MediaPerson {
	return s.importantRecommendationPeopleContext(context.Background(), people, mediaType, limit)
}

func (s *Server) importantRecommendationPeopleContext(ctx context.Context, people []MediaPerson, mediaType string, limit int) []MediaPerson {
	if limit <= 0 {
		limit = 4
	}
	counts := s.recommendationPersonCreditCountsContext(ctx, people)
	type personCandidate struct {
		Person MediaPerson
		Rank   int
		Count  int
	}
	byName := map[string]personCandidate{}
	for _, person := range people {
		person.Name = strings.TrimSpace(person.Name)
		person.Role = strings.TrimSpace(person.Role)
		if person.Name == "" {
			continue
		}
		rank := personRecommendationRoleRank(mediaType, person.Role)
		if rank >= 90 {
			continue
		}
		key := strings.ToLower(person.Name)
		candidate := personCandidate{Person: person, Rank: rank, Count: counts[key]}
		existing, ok := byName[key]
		if !ok || candidate.Rank < existing.Rank || candidate.Rank == existing.Rank && candidate.Person.SortOrder < existing.Person.SortOrder {
			byName[key] = candidate
		}
	}
	candidates := make([]personCandidate, 0, len(byName))
	for _, candidate := range byName {
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Rank != candidates[j].Rank {
			return candidates[i].Rank < candidates[j].Rank
		}
		if candidates[i].Person.SortOrder != candidates[j].Person.SortOrder {
			return candidates[i].Person.SortOrder < candidates[j].Person.SortOrder
		}
		if candidates[i].Count != candidates[j].Count {
			return candidates[i].Count > candidates[j].Count
		}
		return candidates[i].Person.Name < candidates[j].Person.Name
	})
	out := make([]MediaPerson, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return out
		}
		out = append(out, candidate.Person)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Server) recommendationPersonCreditCounts(people []MediaPerson) map[string]int {
	return s.recommendationPersonCreditCountsContext(context.Background(), people)
}

func (s *Server) recommendationPersonCreditCountsContext(ctx context.Context, people []MediaPerson) map[string]int {
	names := map[string]bool{}
	for _, person := range people {
		name := strings.ToLower(strings.TrimSpace(person.Name))
		if name != "" {
			names[name] = true
		}
	}
	if len(names) == 0 {
		return nil
	}
	ordered := sortedRecommendationIDs(names)
	args := make([]any, 0, len(ordered))
	for _, name := range ordered {
		args = append(args, name)
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT lower(trim(p.name)), COUNT(DISTINCT
			CASE
				WHEN credited.type IN ('movie', 'show', 'anime', 'artist', 'album', 'audiobook') THEN credited.id
				WHEN parent.type IN ('show', 'anime', 'artist') THEN parent.id
				WHEN grandparent.type IN ('show', 'anime', 'artist') THEN grandparent.id
				ELSE credited.id
			END
		)
		FROM media_people p
		JOIN media_items credited ON credited.id = p.media_id
		LEFT JOIN media_items parent ON parent.id = credited.parent_id
		LEFT JOIN media_items grandparent ON grandparent.id = parent.parent_id
		WHERE lower(trim(p.name)) IN (`+sqlPlaceholders(len(ordered))+`)
		GROUP BY lower(trim(p.name))`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return counts
		}
		var name string
		var count int
		if err := rows.Scan(&name, &count); err == nil {
			counts[name] = count
		}
	}
	return counts
}

func dedupeRecommendationItems(items []MediaItem, exclude map[string]bool, limit int) []MediaItem {
	seen := map[string]bool{}
	for id := range exclude {
		if strings.TrimSpace(id) != "" {
			seen[id] = true
		}
	}
	capacity := len(items)
	if limit > 0 {
		capacity = min(limit, len(items))
	}
	out := make([]MediaItem, 0, capacity)
	for _, item := range items {
		if item.ID == "" || seen[item.ID] || item.Missing || recommendationExtraItem(item) {
			continue
		}
		seen[item.ID] = true
		out = append(out, item)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func optionalLimitClause(limit int) string {
	if limit <= 0 {
		return ""
	}
	return "\n\t\tLIMIT ?"
}

func optionalLimitArgs(limit int) []any {
	if limit <= 0 {
		return nil
	}
	return []any{limit}
}

func recommendationExtraItem(item MediaItem) bool {
	mediaType := strings.ToLower(strings.TrimSpace(item.Type))
	if mediaType == "extra" || mediaType == "trailer" || mediaType == "featurette" || mediaType == "deleted_scene" || mediaType == "behind_the_scenes" {
		return true
	}
	for _, tag := range item.Tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if normalized == "extra" || normalized == "trailer" {
			return true
		}
	}
	return false
}

func mediaRecommendationContextExclusions(item MediaItem) map[string]bool {
	ids := map[string]bool{item.ID: true}
	if item.ParentID != "" {
		ids[item.ParentID] = true
	}
	if item.GrandparentID != "" {
		ids[item.GrandparentID] = true
	}
	return ids
}

func visualRecommendationTypes(item MediaItem) []string {
	switch item.Type {
	case "movie":
		return []string{"movie"}
	case "anime":
		return []string{"anime"}
	case "show", "season", "episode":
		return []string{"show", "anime"}
	default:
		return nil
	}
}

func recommendationGenreTypes(item MediaItem) []string {
	switch item.Type {
	case "movie":
		return []string{"movie"}
	case "show", "season", "episode":
		return []string{"show", "anime"}
	case "anime":
		return []string{"anime"}
	case "artist", "album":
		return []string{"artist", "album"}
	case "track":
		return []string{"track"}
	case "audiobook":
		return []string{"audiobook"}
	default:
		return nil
	}
}

func topRecommendationGenres(genres []string, limit int) []string {
	if limit <= 0 {
		limit = 3
	}
	seen := map[string]bool{}
	out := []string{}
	for _, genre := range genres {
		genre = strings.TrimSpace(genre)
		key := strings.ToLower(genre)
		if genre == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, genre)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func relatedRecommendationTitle(item MediaItem) string {
	switch item.Type {
	case "movie":
		return "Related Movies"
	case "show", "season", "episode":
		return "Related Shows"
	case "anime":
		return "Related Anime"
	case "track":
		return "Related Tracks"
	case "artist", "album":
		return "Related Music"
	case "audiobook":
		return "Related Audiobooks"
	default:
		return "Related Content"
	}
}

func genreRecommendationTitle(item MediaItem, genre string) string {
	genre = strings.TrimSpace(genre)
	switch item.Type {
	case "movie":
		return genre + " Movies"
	case "show", "season", "episode", "anime":
		return genre + " Shows"
	case "artist", "album", "track":
		return genre + " Music"
	case "audiobook":
		return genre + " Audiobooks"
	default:
		return "More " + genre
	}
}

func personRecommendationRowTitle(person MediaPerson) string {
	role := strings.ToLower(strings.TrimSpace(person.Role))
	if strings.Contains(role, "director") || strings.Contains(role, "writer") || strings.Contains(role, "screenplay") || strings.Contains(role, "teleplay") || strings.Contains(role, "creator") || strings.Contains(role, "showrunner") || strings.Contains(role, "composer") {
		return "More by " + person.Name
	}
	if strings.Contains(role, "producer") {
		return "More from " + person.Name
	}
	return "More with " + person.Name
}

func personRecommendationRoleRank(mediaType, role string) int {
	normalized := strings.ToLower(strings.TrimSpace(role))
	switch {
	case strings.Contains(normalized, "director"):
		return 0
	case strings.Contains(normalized, "creator") || strings.Contains(normalized, "showrunner"):
		return 0
	case strings.Contains(normalized, "writer") || strings.Contains(normalized, "screenplay") || strings.Contains(normalized, "teleplay"):
		return 1
	case strings.Contains(normalized, "actor") || strings.Contains(normalized, "cast") || strings.Contains(normalized, "voice"):
		return 2
	case strings.Contains(normalized, "producer"):
		return 3
	case mediaType == "track" || mediaType == "album" || mediaType == "artist":
		if strings.Contains(normalized, "performer") || strings.Contains(normalized, "composer") || strings.Contains(normalized, "lyricist") {
			return 2
		}
	}
	return 99
}

func mediaArtistLabel(item MediaItem) string {
	if item.Type != "artist" && item.Type != "album" && item.Type != "track" {
		return ""
	}
	if item.Type == "artist" {
		return strings.TrimSpace(item.Title)
	}
	return strings.TrimSpace(firstNonEmpty(
		item.TypedMetadata["albumArtist"],
		item.TypedMetadata["trackArtist"],
		item.TypedMetadata["artist"],
		item.GrandparentTitle,
		item.ParentTitle,
		item.Studio,
	))
}

func audiobookAuthorLabel(item MediaItem) string {
	if item.Type != "audiobook" {
		return ""
	}
	return strings.TrimSpace(firstNonEmpty(item.TypedMetadata["author"], item.Studio))
}

func recommendationRowType(items []MediaItem) string {
	for _, item := range items {
		switch item.Type {
		case "episode", "track":
			return "landscape"
		}
	}
	return "poster"
}

func recommendationRowID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, ch := range value {
		if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
			builder.WriteRune(ch)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func sortedRecommendationIDs(ids map[string]bool) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		if strings.TrimSpace(id) != "" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) localRecommendationScores(userID string, limit int) ([]scoredMedia, error) {
	return s.localRecommendationScoresContext(context.Background(), userID, limit)
}

func (s *Server) localRecommendationScoresContext(ctx context.Context, userID string, limit int) ([]scoredMedia, error) {
	if cached, err := s.cachedLocalRecommendationScoresContext(ctx, userID, limit); err == nil && len(cached) > 0 {
		return cached, nil
	}
	seeds, candidates, err := s.localRecommendationInputsContext(ctx, userID)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scored := scoreRecommendations(seeds, candidates)
	if len(scored) == 0 {
		return nil, nil
	}
	if err := s.storeLocalRecommendationScoresContext(ctx, userID, scored); err != nil {
		s.recordLog("warn", "Recommendation cache write failed", map[string]string{"error": err.Error()})
	}
	if limit > 0 && len(scored) > limit {
		return scored[:limit], nil
	}
	return scored, nil
}

func (s *Server) localRecommendationScoresForLibrary(userID string, library Library, limit int) ([]scoredMedia, error) {
	return s.localRecommendationScoresForLibraryContext(context.Background(), userID, library, limit)
}

func (s *Server) localRecommendationScoresForLibraryContext(ctx context.Context, userID string, library Library, limit int) ([]scoredMedia, error) {
	seeds, candidates, err := s.localRecommendationInputsForLibraryContext(ctx, userID, library)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scored := scoreRecommendations(seeds, candidates)
	if limit > 0 && len(scored) > limit {
		return scored[:limit], nil
	}
	return scored, nil
}

func (s *Server) cachedLocalRecommendationScores(userID string, limit int) ([]scoredMedia, error) {
	return s.cachedLocalRecommendationScoresContext(context.Background(), userID, limit)
}

func (s *Server) cachedLocalRecommendationScoresContext(ctx context.Context, userID string, limit int) ([]scoredMedia, error) {
	query := `
		SELECT media_id, score, reason, source
		FROM user_recommendation_cache
		WHERE profile_id = ? AND expires_at > ?
		ORDER BY rank ASC` + optionalLimitClause(limit)
	args := append([]any{userID, time.Now().UTC().Format(time.RFC3339)}, optionalLimitArgs(limit)...)
	rows, err := s.queryUserRead(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scored []scoredMedia
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var item scoredMedia
		if err := rows.Scan(&item.ID, &item.Score, &item.Reason, &item.Source); err != nil {
			return nil, err
		}
		scored = append(scored, item)
	}
	return scored, rows.Err()
}

func (s *Server) storeLocalRecommendationScores(userID string, scored []scoredMedia) error {
	return s.storeLocalRecommendationScoresContext(context.Background(), userID, scored)
}

func (s *Server) storeLocalRecommendationScoresContext(ctx context.Context, userID string, scored []scoredMedia) error {
	accountID, profileID := s.accountAndProfileIDsContext(ctx, userID)
	now := time.Now().UTC()
	generatedAt := now.Format(time.RFC3339)
	expiresAt := now.Add(userRecommendationCacheTTL).Format(time.RFC3339)
	return s.withBackgroundTxTagged(ctx, []string{"search", "media", "library-items"}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM user_recommendation_cache WHERE profile_id = ?`, profileID); err != nil {
			return err
		}
		limit := min(len(scored), 100)
		for i := 0; i < limit; i++ {
			item := scored[i]
			if _, err := tx.Exec(`
				INSERT INTO user_recommendation_cache (profile_id, user_id, media_id, rank, score, reason, source, generated_at, expires_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				profileID, accountID, item.ID, i+1, item.Score, item.Reason, item.Source, generatedAt, expiresAt); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Server) localRecommendationInputs(userID string) ([]localRecommendationSeed, []localRecommendationSeed, error) {
	return s.localRecommendationInputsContext(context.Background(), userID)
}

func (s *Server) localRecommendationInputsContext(ctx context.Context, userID string) ([]localRecommendationSeed, []localRecommendationSeed, error) {
	accountID, profileID := s.accountAndProfileIDsContext(ctx, userID)
	query := `
			SELECT
				m.id, COALESCE(m.library_id, ''), m.type, m.title, m.year, m.genres_json,
				m.community_rating, COALESCE(ums.progress_seconds, 0), COALESCE(ums.rating, 0),
				COALESCE(ums.last_played_at, ''), COALESCE(ums.watchlisted, 0), COALESCE(ums.watched, 0), m.added_at
		FROM media_items m
		LEFT JOIN user_media_state ums ON ums.media_id = m.id AND ums.profile_id = ?
		WHERE m.parent_id IS NULL
				AND m.type IN ('movie', 'show', 'anime')
			ORDER BY m.added_at DESC
			LIMIT ?`
	args := []any{
		profileID,
		recommendationInputLimit,
	}
	query, args = s.applyMediaVisibilityRestrictionToClause(profileID, query, args)
	query, args = s.applyLibraryCurationRestrictionToClause(ctx, accountID, query, args)
	rows, err := s.queryUserRead(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var seeds []localRecommendationSeed
	var candidates []localRecommendationSeed
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		var item localRecommendationSeed
		var genresJSON string
		var watchlisted, watched int
		if err := rows.Scan(
			&item.ID, &item.LibraryID, &item.Type, &item.Title, &item.Year, &genresJSON,
			&item.CommunityRating, &item.ProgressSeconds, &item.Rating, &item.LastPlayedAt,
			&watchlisted, &watched, &item.AddedAt,
		); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal([]byte(genresJSON), &item.Genres)
		item.Watchlisted = watchlisted == 1
		item.Watched = watched == 1
		if item.ProgressSeconds > 0 || item.Rating >= 4 || item.Watchlisted || item.Watched {
			seeds = append(seeds, item)
		} else {
			candidates = append(candidates, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	if err := s.attachLocalRecommendationMetadataContext(ctx, seeds, candidates); err != nil {
		return nil, nil, err
	}
	if len(seeds) == 0 {
		return nil, candidates, nil
	}
	return seeds, candidates, nil
}

func (s *Server) localRecommendationInputsForLibrary(userID string, library Library) ([]localRecommendationSeed, []localRecommendationSeed, error) {
	return s.localRecommendationInputsForLibraryContext(context.Background(), userID, library)
}

func (s *Server) localRecommendationInputsForLibraryContext(ctx context.Context, userID string, library Library) ([]localRecommendationSeed, []localRecommendationSeed, error) {
	accountID, profileID := s.accountAndProfileIDsContext(ctx, userID)
	types := libraryDiscoveryCandidateTypes(library)
	if len(types) == 0 {
		return nil, nil, nil
	}
	typeClause, typeArgs := sqlInList("m.type", types)
	query := `
			SELECT
				m.id, COALESCE(m.library_id, ''), m.type, m.title, m.year, m.genres_json,
				m.community_rating, COALESCE(ums.progress_seconds, 0), COALESCE(ums.rating, 0),
			COALESCE(ums.last_played_at, ''), COALESCE(ums.watchlisted, 0), COALESCE(ums.watched, 0), m.added_at
		FROM media_items m
		LEFT JOIN user_media_state ums ON ums.media_id = m.id AND ums.profile_id = ?
		WHERE m.library_id = ?
			AND ` + typeClause + `
			` + libraryDiscoveryParentClause(library) + `
			ORDER BY m.added_at DESC
			LIMIT ?`
	args := append([]any{profileID, library.ID}, typeArgs...)
	args = append(args, recommendationInputLimit)
	query, args = s.applyLibraryCurationRestrictionToClause(ctx, accountID, query, args)
	rows, err := s.queryUserRead(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var seeds []localRecommendationSeed
	var candidates []localRecommendationSeed
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		var item localRecommendationSeed
		var genresJSON string
		var watchlisted, watched int
		if err := rows.Scan(
			&item.ID, &item.LibraryID, &item.Type, &item.Title, &item.Year, &genresJSON,
			&item.CommunityRating, &item.ProgressSeconds, &item.Rating, &item.LastPlayedAt,
			&watchlisted, &watched, &item.AddedAt,
		); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal([]byte(genresJSON), &item.Genres)
		item.Watchlisted = watchlisted == 1
		item.Watched = watched == 1
		if item.ProgressSeconds > 0 || item.Rating >= 4 || item.Watchlisted || item.Watched {
			seeds = append(seeds, item)
		} else {
			candidates = append(candidates, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	if err := s.attachLocalRecommendationMetadataContext(ctx, seeds, candidates); err != nil {
		return nil, nil, err
	}
	if len(seeds) == 0 {
		return nil, candidates, nil
	}
	return seeds, candidates, nil
}

func (s *Server) attachLocalRecommendationMetadataContext(ctx context.Context, seeds, candidates []localRecommendationSeed) error {
	ids := make([]string, 0, len(seeds)+len(candidates))
	for _, item := range seeds {
		ids = append(ids, item.ID)
	}
	for _, item := range candidates {
		ids = append(ids, item.ID)
	}
	inputs, err := s.metadataRecommendationInputsContext(ctx, ids)
	if err != nil {
		return err
	}
	for index := range seeds {
		seeds[index].Metadata = inputs[seeds[index].ID]
	}
	for index := range candidates {
		candidates[index].Metadata = inputs[candidates[index].ID]
	}
	return nil
}

func scoreRecommendations(seeds, candidates []localRecommendationSeed) []scoredMedia {
	genreWeights := map[string]float64{}
	typeWeights := map[string]float64{}
	yearWeights := map[int]float64{}
	metadataWeights := map[string]float64{}
	for _, seed := range seeds {
		weight := 1.0
		if seed.ProgressSeconds > 0 {
			weight += 2.0
		}
		if seed.Watched {
			weight += 1.2
		}
		if seed.Watchlisted {
			weight += 0.8
		}
		if seed.Rating > 0 {
			weight += float64(seed.Rating) / 2
		}
		typeWeights[seed.Type] += weight
		if seed.Year > 0 {
			yearWeights[seed.Year/10*10] += weight * 0.45
		}
		for _, genre := range seed.Genres {
			normalized := strings.ToLower(strings.TrimSpace(genre))
			if normalized != "" {
				genreWeights[normalized] += weight
			}
		}
		for feature, featureWeight := range recommendationMetadataFeatures(seed.Metadata) {
			metadataWeights[feature] += weight * featureWeight
		}
	}
	var scored []scoredMedia
	for _, candidate := range candidates {
		score := candidate.CommunityRating * 0.15
		typeScore := typeWeights[candidate.Type] * 0.35
		score += typeScore
		decadeScore := 0.0
		if candidate.Year > 0 {
			decadeScore = yearWeights[candidate.Year/10*10]
			score += decadeScore
		}
		genreScore := 0.0
		bestGenre := ""
		for _, genre := range candidate.Genres {
			normalized := strings.ToLower(strings.TrimSpace(genre))
			value := genreWeights[normalized]
			score += value
			genreScore += value
			if value > 0 && bestGenre == "" {
				bestGenre = strings.TrimSpace(genre)
			}
		}
		metadataScore := 0.0
		bestMetadataFamily := ""
		bestMetadataValue := 0.0
		for feature, featureWeight := range recommendationMetadataFeatures(candidate.Metadata) {
			value := metadataWeights[feature] * featureWeight
			metadataScore += value
			if value > bestMetadataValue {
				bestMetadataValue = value
				bestMetadataFamily = strings.SplitN(feature, ":", 2)[0]
			}
		}
		// Rich metadata should refine a recommendation, not overwhelm strong
		// viewer behavior or genre affinity merely because a provider returned
		// a longer relationship list.
		metadataScore = math.Min(metadataScore, 12)
		score += metadataScore
		if score <= 0 {
			continue
		}
		reason := "Recommended from your library activity"
		if metadataScore > genreScore && metadataScore > typeScore && metadataScore > decadeScore {
			switch bestMetadataFamily {
			case "collection", "franchise":
				reason = "Because you watch titles from related collections"
			case "keyword", "tag":
				reason = "Because you watch titles with similar themes"
			case "person", "creator":
				reason = "Because you watch titles with familiar creators and cast"
			default:
				reason = "Because you watch titles with similar catalog details"
			}
		} else if bestGenre != "" && genreScore >= typeScore && genreScore >= decadeScore {
			reason = "Because you watched or saved " + bestGenre + " titles"
		} else if typeScore > 0 {
			reason = "Because you watch similar " + mediaTypePluralLabel(candidate.Type)
		} else if decadeScore > 0 && candidate.Year > 0 {
			reason = "Because you watch titles from the " + strconv.Itoa(candidate.Year/10*10) + "s"
		}
		scored = append(scored, scoredMedia{ID: candidate.ID, Score: score, Reason: reason, Source: "local_recommendations"})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].ID < scored[j].ID
		}
		return scored[i].Score > scored[j].Score
	})
	return scored
}

func recommendationMetadataFeatures(input metadataRecommendationInput) map[string]float64 {
	features := map[string]float64{}
	weights := map[string]float64{
		"keyword": 0.9, "tag": 0.45, "collection": 1.25, "franchise": 1.35,
		"person": 0.55, "creator": 0.75, "character": 0.25,
		"studio": 0.45, "company": 0.4, "network": 0.45,
		"country": 0.2, "language": 0.2, "genre": 0.8,
		"work": 1.0, "related_media": 1.0,
	}
	for relationshipType, values := range input.Relationships {
		weight := weights[relationshipType]
		if weight == 0 {
			continue
		}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				features[relationshipType+":"+value] = weight
			}
		}
	}
	return features
}

func mediaTypePluralLabel(mediaType string) string {
	switch mediaType {
	case "movie":
		return "movies"
	case "show":
		return "shows"
	case "anime":
		return "anime"
	default:
		return "titles"
	}
}

func libraryListeningLabel(libraryType, watching, listening string) string {
	if libraryType == "music" || libraryType == "audiobook" {
		return listening
	}
	return watching
}

func libraryDiscoveryCandidateTypes(library Library) []string {
	switch library.Type {
	case "movie":
		return []string{"movie"}
	case "show":
		return []string{"show"}
	case "anime":
		return []string{"anime"}
	case "music":
		return []string{"album"}
	case "audiobook":
		return []string{"audiobook"}
	default:
		return nil
	}
}

func libraryDiscoveryActivityTypes(library Library) []string {
	switch library.Type {
	case "movie":
		return []string{"movie"}
	case "show":
		return []string{"show", "season", "episode"}
	case "anime":
		return []string{"anime", "season", "episode"}
	case "music":
		return []string{"album", "track"}
	case "audiobook":
		return []string{"audiobook"}
	default:
		return nil
	}
}

func libraryDiscoveryParentClause(library Library) string {
	switch library.Type {
	case "movie", "show", "anime", "audiobook":
		return "AND m.parent_id IS NULL"
	default:
		return ""
	}
}

func sqlInList(column string, values []string) (string, []any) {
	if len(values) == 0 {
		return "1 = 0", nil
	}
	placeholders := make([]string, len(values))
	args := make([]any, 0, len(values))
	for i, value := range values {
		placeholders[i] = "?"
		args = append(args, value)
	}
	return column + " IN (" + strings.Join(placeholders, ",") + ")", args
}

func (s *Server) normalizeLibraryDiscoveryItems(userID string, library Library, items []MediaItem, limit int) []MediaItem {
	capacity := len(items)
	if limit > 0 {
		capacity = min(len(items), limit)
	}
	ids := make([]string, 0, capacity)
	seen := map[string]bool{}
	for _, item := range items {
		id := s.libraryDiscoveryDisplayID(library, item)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		if limit > 0 && len(ids) >= limit {
			break
		}
	}
	normalized, err := s.mediaByOrderedIDs(userID, ids)
	if err != nil {
		return nil
	}
	return normalized
}

func (s *Server) libraryDiscoveryDisplayID(library Library, item MediaItem) string {
	switch library.Type {
	case "show", "anime":
		if item.Type == "episode" && item.GrandparentID != "" {
			return item.GrandparentID
		}
		if item.Type == "season" && item.ParentID != "" {
			return item.ParentID
		}
		if item.Type == "show" || item.Type == "anime" {
			return item.ID
		}
	case "music":
		if item.Type == "track" && item.ParentID != "" {
			return item.ParentID
		}
		if item.Type == "album" {
			return item.ID
		}
	case "movie":
		if item.Type == "movie" {
			return item.ID
		}
	case "audiobook":
		if item.Type == "audiobook" {
			return item.ID
		}
	}
	if item.LibraryID == library.ID {
		return item.ID
	}
	return ""
}

func (s *Server) normalizeServerWatchingItems(userID string, items []MediaItem, limit int) []MediaItem {
	ids := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		id := ""
		switch item.Type {
		case "movie", "show", "anime":
			if item.ParentID == "" {
				id = item.ID
			}
		case "season":
			id = item.ParentID
		case "episode":
			id = item.GrandparentID
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		if limit > 0 && len(ids) >= limit {
			break
		}
	}
	normalized, err := s.mediaByOrderedIDs(userID, ids)
	if err != nil {
		return nil
	}
	return normalized
}

func (s *Server) mediaByOrderedIDs(userID string, ids []string) ([]MediaItem, error) {
	return s.mediaByOrderedIDsContext(context.Background(), userID, ids)
}

func (s *Server) mediaByOrderedIDsContext(ctx context.Context, userID string, ids []string) ([]MediaItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	items, err := s.queryMediaContext(ctx, userID, `WHERE m.id IN (`+strings.Join(placeholders, ",")+`)`, args)
	if err != nil {
		return nil, err
	}
	byID := map[string]MediaItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	ordered := make([]MediaItem, 0, len(items))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			ordered = append(ordered, item)
		}
	}
	return ordered, nil
}

func (s *Server) mediaListItemsByOrderedIDs(userID string, ids []string) ([]MediaItem, error) {
	return s.mediaListItemsByOrderedIDsContext(context.Background(), userID, ids)
}

func (s *Server) mediaListItemsByOrderedIDsContext(ctx context.Context, userID string, ids []string) ([]MediaItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	items, err := s.queryMediaListItemsContext(ctx, userID, `WHERE m.id IN (`+strings.Join(placeholders, ",")+`)`, args)
	if err != nil {
		return nil, err
	}
	byID := map[string]MediaItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	ordered := make([]MediaItem, 0, len(items))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			ordered = append(ordered, item)
		}
	}
	return ordered, nil
}

func (entry tmdbTrendingEntry) displayTitle() string {
	if entry.Title != "" {
		return entry.Title
	}
	return entry.Name
}

func (entry tmdbTrendingEntry) displayYear() int {
	date := entry.ReleaseDate
	if date == "" {
		date = entry.FirstAirDate
	}
	if len(date) < 4 {
		return 0
	}
	year := 0
	for _, ch := range date[:4] {
		if ch < '0' || ch > '9' {
			return 0
		}
		year = year*10 + int(ch-'0')
	}
	return year
}

func mediaMatchKeys(title, originalTitle string, year int) []string {
	titles := []string{title}
	if originalTitle != "" && !strings.EqualFold(originalTitle, title) {
		titles = append(titles, originalTitle)
	}
	var keys []string
	for _, candidate := range titles {
		normalized := normalizeDiscoveryTitle(candidate)
		if normalized == "" {
			continue
		}
		keys = append(keys, normalized)
		if year > 0 {
			keys = append(keys, fmt.Sprintf("%s:%d", normalized, year))
		}
	}
	return keys
}

func normalizeDiscoveryTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastSpace := false
	for _, ch := range value {
		if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
			builder.WriteRune(ch)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}
