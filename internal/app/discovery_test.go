package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func TestTMDBTrendingCacheMatchesLocalLibraryItems(t *testing.T) {
	var requests int
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/trending/all/day" {
			t.Fatalf("unexpected TMDB path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("authorization header = %q", got)
		}
		if got := r.URL.Query().Get("api_key"); got != "test-api-key" {
			t.Fatalf("api_key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"results":[{"id":100,"title":"The Meridian Job","release_date":"2025-02-01","popularity":99}]}`)
	}))
	defer tmdb.Close()

	serverURL, db, server := newDiscoveryTestServer(t, config.Config{
		TMDBAPIKey:  "test-api-key",
		TMDBBaseURL: tmdb.URL,
	})
	if err := db.QueryRow(`SELECT COUNT(*) FROM tmdb_trending_cache`).Scan(new(int)); err != nil {
		t.Fatalf("discovery cache table missing: %v", err)
	}

	if err := server.refreshTMDBTrending(context.Background(), "all", "day"); err != nil {
		t.Fatalf("refresh TMDB trending: %v", err)
	}
	if requests != 1 {
		t.Fatalf("TMDB requests = %d, expected 1", requests)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	row := fetchHomeRow(t, client, serverURL, "tmdb_trending", 24)
	if len(row.Items) != 1 {
		t.Fatalf("expected one matched trending item, got %#v", row)
	}
	if row.Items[0].ID != "movie_meridian" {
		t.Fatalf("trending item = %s, expected movie_meridian", row.Items[0].ID)
	}
}

func TestTMDBTrendingRefreshQueuesMaintenanceJob(t *testing.T) {
	server := newScannerTestServer(t)
	server.cfg.TMDBAPIKey = "test-api-key"
	release := server.acquireJobLane("metadata_refresh")
	defer release()

	server.queueTMDBTrendingRefresh("all", "day")
	server.queueTMDBTrendingRefresh("all", "day")

	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'tmdb_trending_refresh' AND resource_type = 'discovery' AND resource_id = 'all/day'`).Scan(&count); err != nil {
		t.Fatalf("count tmdb trending jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("tmdb trending refresh jobs = %d, expected 1", count)
	}
	if jobLaneForType("tmdb_trending_refresh") != jobLaneMetadata {
		t.Fatalf("tmdb trending lane = %s, expected metadata", jobLaneForType("tmdb_trending_refresh"))
	}
	if _, err := server.db.Exec(`UPDATE jobs SET status = 'cancelled' WHERE type = 'tmdb_trending_refresh'`); err != nil {
		t.Fatalf("cancel queued tmdb trending job: %v", err)
	}
}

func TestHomeRecommendationsUseLocalSignals(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watchlisted, watched, progress_seconds, rating, last_played_at, updated_at)
		VALUES (?, ?, 'movie_meridian', 0, 1, 1200, 5, ?, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET
			watched = excluded.watched,
			progress_seconds = excluded.progress_seconds,
			rating = excluded.rating,
			last_played_at = excluded.last_played_at,
			updated_at = excluded.updated_at`,
		userID, userID, now, now,
	)
	if err != nil {
		t.Fatalf("insert user media state: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	row := fetchHomeRow(t, client, serverURL, "recommended_for_you", 24)
	if len(row.Items) == 0 {
		t.Fatalf("expected local recommendations, got %#v", row)
	}
	for _, item := range row.Items {
		if item.ID == "movie_meridian" {
			t.Fatalf("recommendations included consumed seed item: %#v", row.Items)
		}
	}
	hasSharedGenreRecommendation := false
	for _, item := range row.Items {
		switch item.ID {
		case "movie_blackwater", "movie_orchid", "movie_lantern":
			hasSharedGenreRecommendation = true
		}
	}
	if !hasSharedGenreRecommendation {
		t.Fatalf("recommendations did not include a movie with genres shared by the seed: %#v", row.Items)
	}

	var suggestions SuggestionsResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/suggestions?limit=6", nil, &suggestions)
	if status != http.StatusOK {
		t.Fatalf("suggestions status = %d, body: %s", status, body)
	}
	if suggestions.Total == 0 || suggestions.GeneratedAt == "" {
		t.Fatalf("expected explainable suggestions, got %#v", suggestions)
	}
	var cached SuggestionsResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/suggestions?limit=6", nil, &cached)
	if status != http.StatusOK {
		t.Fatalf("cached suggestions status = %d, body: %s", status, body)
	}
	if cached.GeneratedAt != suggestions.GeneratedAt {
		t.Fatalf("suggestions generatedAt first=%q cached=%q, expected cached response", suggestions.GeneratedAt, cached.GeneratedAt)
	}
	for _, suggestion := range suggestions.Items {
		if suggestion.Item.ID == "movie_meridian" {
			t.Fatalf("suggestions included consumed seed item: %#v", suggestions.Items)
		}
		if suggestion.Reason == "" || suggestion.Source == "" || suggestion.Score <= 0 {
			t.Fatalf("suggestion missing explanation: %#v", suggestion)
		}
	}
}

func TestSuggestionsExposeDiscoveryRowsAndWeeklyServerWatching(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	seedActivitySharingUser(t, db, "usr_discovery_peer", "discovery-peer")
	if _, err := db.Exec(`
			INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
			VALUES
				(?, ?, 'movie_blackwater', 0, 420, ?, ?),
				(?, ?, 'episode_northbridge_101', 0, 900, ?, ?),
				(?, ?, 'movie_lantern', 0, 510, ?, ?),
				('usr_discovery_peer', 'usr_discovery_peer', 'movie_blackwater', 0, 240, ?, ?),
				('usr_discovery_peer', 'usr_discovery_peer', 'episode_northbridge_101', 0, 360, ?, ?)
			ON CONFLICT(profile_id, media_id) DO UPDATE SET
				progress_seconds = excluded.progress_seconds,
				last_played_at = excluded.last_played_at,
				updated_at = excluded.updated_at`,
		userID, userID, now.Add(-2*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339),
		userID, userID, now.Add(-90*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
		userID, userID, now.Add(-9*24*time.Hour).Format(time.RFC3339), now.Add(-9*24*time.Hour).Format(time.RFC3339),
		now.Add(-75*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
		now.Add(-45*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed watching state: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var suggestions SuggestionsResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/suggestions?limit=12", nil, &suggestions)
	if status != http.StatusOK {
		t.Fatalf("suggestions status = %d, body: %s", status, body)
	}
	if len(suggestions.Rows) == 0 {
		t.Fatalf("expected explicit discover rows: %#v", suggestions)
	}
	watching := homeRowByID(HomeResponse{Rows: suggestions.Rows}, "server_watching_week")
	if watching == nil {
		t.Fatalf("weekly watching row missing: %#v", suggestions.Rows)
	}
	if !mediaIDsContain(watching.Items, "movie_blackwater") {
		t.Fatalf("weekly watching row did not include recent playback: %#v", watching.Items)
	}
	if !mediaIDsContain(watching.Items, "show_northbridge") {
		t.Fatalf("weekly watching row did not normalize episode activity to show poster: %#v", watching.Items)
	}
	if mediaIDsContain(watching.Items, "episode_northbridge_101") {
		t.Fatalf("weekly watching row exposed episode activity directly: %#v", watching.Items)
	}
	if mediaIDsContain(watching.Items, "movie_lantern") {
		t.Fatalf("weekly watching row included stale playback: %#v", watching.Items)
	}
}

func TestHomeWeeklyServerWatchingShowsOnlyTopLevelVideoItems(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	seedActivitySharingUser(t, db, "usr_home_peer", "home-peer")
	if _, err := db.Exec(`
			INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
			VALUES
				(?, ?, 'movie_blackwater', 0, 420, ?, ?),
				(?, ?, 'season_northbridge_1', 0, 300, ?, ?),
				(?, ?, 'episode_northbridge_101', 0, 900, ?, ?),
				('usr_home_peer', 'usr_home_peer', 'movie_blackwater', 0, 300, ?, ?),
				('usr_home_peer', 'usr_home_peer', 'season_northbridge_1', 0, 240, ?, ?),
				('usr_home_peer', 'usr_home_peer', 'episode_northbridge_101', 0, 480, ?, ?)
			ON CONFLICT(profile_id, media_id) DO UPDATE SET
				progress_seconds = excluded.progress_seconds,
				last_played_at = excluded.last_played_at,
				updated_at = excluded.updated_at`,
		userID, userID, now.Add(-2*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339),
		userID, userID, now.Add(-90*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
		userID, userID, now.Add(-30*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
		now.Add(-80*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
		now.Add(-70*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
		now.Add(-20*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed home watching state: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	watching := fetchHomeRow(t, client, serverURL, "server_watching_week", 24)
	if !mediaIDsContain(watching.Items, "movie_blackwater") || !mediaIDsContain(watching.Items, "show_northbridge") {
		t.Fatalf("weekly home watching row did not include top-level activity targets: %#v", watching.Items)
	}
	for _, item := range watching.Items {
		if item.Type == "episode" || item.Type == "season" || item.ParentID != "" {
			t.Fatalf("weekly home watching row exposed non-top-level item: %#v", item)
		}
	}
}

func TestHomeWeeklyServerWatchingHonorsMemberActivityVisibility(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	seedActivitySharingUser(t, db, "usr_visible_activity", "visible-activity")
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_hidden_activity', 'hidden-activity', 'hidden-activity@example.test', 'Hidden Activity', 'user', '{}', '{}', ?, ?)`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert hidden activity user: %v", err)
	}
	setUserPrivacyPreferencesForTest(t, db, "usr_hidden_activity", UserPrivacyPreferences{
		PauseWatchHistory:         false,
		ShowActivityToMembers:     false,
		IncludeInWatchWithFriends: true,
	})
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES
			(?, ?, 'movie_saffron', 0, 600, ?, ?),
			('usr_visible_activity', 'usr_visible_activity', 'movie_saffron', 0, 420, ?, ?),
			('usr_hidden_activity', 'usr_hidden_activity', 'movie_neon', 0, 600, ?, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET
			watched = excluded.watched,
			progress_seconds = excluded.progress_seconds,
			last_played_at = excluded.last_played_at,
			updated_at = excluded.updated_at`,
		userID, userID, now.Add(-30*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
		now.Add(-25*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
		now.Add(-20*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed visibility activity state: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	watching := fetchHomeRow(t, client, serverURL, "server_watching_week", 24)
	if !mediaIDsContain(watching.Items, "movie_saffron") {
		t.Fatalf("weekly home watching row did not include visible viewer activity: %#v", watching.Items)
	}
	if mediaIDsContain(watching.Items, "movie_neon") {
		t.Fatalf("weekly home watching row included hidden member activity: %#v", watching.Items)
	}
}

func TestLibrarySuggestionsBuildScopedTVDiscoverRows(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	seedActivitySharingUser(t, db, "usr_library_peer", "library-peer")
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES
			(?, ?, 'movie_blackwater', 0, 420, ?, ?),
			(?, ?, 'episode_northbridge_101', 0, 900, ?, ?),
			('usr_library_peer', 'usr_library_peer', 'episode_northbridge_101', 0, 360, ?, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET
			progress_seconds = excluded.progress_seconds,
			last_played_at = excluded.last_played_at,
			updated_at = excluded.updated_at`,
		userID, userID, now.Add(-90*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
		userID, userID, now.Add(-30*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
		now.Add(-20*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed library watching state: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var suggestions SuggestionsResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/suggestions?limit=12&libraryId=lib_tv", nil, &suggestions)
	if status != http.StatusOK {
		t.Fatalf("library suggestions status = %d, body: %s", status, body)
	}
	watching := homeRowByID(HomeResponse{Rows: suggestions.Rows}, "server_watching_week")
	if watching == nil {
		t.Fatalf("weekly library watching row missing: %#v", suggestions.Rows)
	}
	if !mediaIDsContain(watching.Items, "show_northbridge") {
		t.Fatalf("weekly library watching row did not normalize episode activity to show poster: %#v", watching.Items)
	}
	if mediaIDsContain(watching.Items, "episode_northbridge_101") || mediaIDsContain(watching.Items, "movie_blackwater") {
		t.Fatalf("weekly library watching row leaked episode or movie items: %#v", watching.Items)
	}
	for _, row := range suggestions.Rows {
		for _, item := range row.Items {
			if item.LibraryID != "lib_tv" {
				t.Fatalf("library discover row %s leaked item from %s: %#v", row.ID, item.LibraryID, item)
			}
			if item.Type == "episode" || item.Type == "season" {
				t.Fatalf("library discover row %s exposed TV child item: %#v", row.ID, item)
			}
		}
	}
}

func TestLibraryDiscoverEndpointComposesScopedRows(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES (?, ?, 'episode_northbridge_101', 0, 900, ?, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET
			watched = excluded.watched,
			progress_seconds = excluded.progress_seconds,
			last_played_at = excluded.last_played_at,
			updated_at = excluded.updated_at`,
		userID, userID, now.Add(-30*time.Minute).Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed library discover state: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var discover SuggestionsResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/lib_tv/discover?limit=12", nil, &discover)
	if status != http.StatusOK {
		t.Fatalf("library discover status = %d, body: %s", status, body)
	}
	if discover.GeneratedAt == "" {
		t.Fatalf("library discover missing generatedAt: %#v", discover)
	}
	if row := homeRowByID(HomeResponse{Rows: discover.Rows}, "library_continue"); row == nil || (!mediaIDsContain(row.Items, "episode_northbridge_101") && !mediaIDsContain(row.Items, "show_northbridge")) {
		t.Fatalf("library discover missing scoped continue row: %#v", discover.Rows)
	}
	if row := homeRowByID(HomeResponse{Rows: discover.Rows}, "library_recent"); row == nil || len(row.Items) == 0 {
		t.Fatalf("library discover missing scoped recent row: %#v", discover.Rows)
	}
	for _, row := range discover.Rows {
		for _, item := range row.Items {
			if item.LibraryID != "lib_tv" {
				t.Fatalf("library discover row %s leaked item from %s: %#v", row.ID, item.LibraryID, item)
			}
		}
	}
}

func TestLibraryDiscoverOmitsContinueRowWhenLibraryExcluded(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES (?, ?, 'movie_meridian', 0, 600, ?, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET
			watched = excluded.watched,
			progress_seconds = excluded.progress_seconds,
			last_played_at = excluded.last_played_at,
			updated_at = excluded.updated_at`,
		userID, userID, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed library discover state: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var before SuggestionsResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/lib_movies/discover?limit=12", nil, &before)
	if status != http.StatusOK {
		t.Fatalf("library discover before update status = %d, body: %s", status, body)
	}
	if row := homeRowByID(HomeResponse{Rows: before.Rows}, "library_continue"); row == nil || !mediaIDsContain(row.Items, "movie_meridian") {
		t.Fatalf("library discover missing scoped continue row before exclusion: %#v", before.Rows)
	}

	var updated Library
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/libraries/lib_movies", map[string]any{
		"settings": map[string]any{"includeInContinueWatching": false},
	}, &updated)
	if status != http.StatusOK {
		t.Fatalf("library update status = %d, body: %s", status, body)
	}

	var after SuggestionsResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/libraries/lib_movies/discover?limit=12", nil, &after)
	if status != http.StatusOK {
		t.Fatalf("library discover after update status = %d, body: %s", status, body)
	}
	if row := homeRowByID(HomeResponse{Rows: after.Rows}, "library_continue"); row != nil {
		t.Fatalf("library discover exposed continue row for excluded library: %#v", row)
	}
}

func TestMediaDetailRecommendationRowsUsePeopleAndLibraryContext(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := server.replaceProviderMediaPeople("movie_meridian", []MediaPerson{
		{Name: "Noor Patel", Role: "Director", SortOrder: 0},
		{Name: "Ari Vega", Role: "Actor", Character: "Captain Sol", SortOrder: 0},
	}, "tmdb"); err != nil {
		t.Fatalf("seed source people: %v", err)
	}
	if err := server.replaceProviderMediaPeople("movie_orchid", []MediaPerson{{Name: "Noor Patel", Role: "Director", SortOrder: 0}}, "tmdb"); err != nil {
		t.Fatalf("seed director match: %v", err)
	}
	if err := server.replaceProviderMediaPeople("movie_blackwater", []MediaPerson{{Name: "Ari Vega", Role: "Actor", Character: "Navigator", SortOrder: 0}}, "tmdb"); err != nil {
		t.Fatalf("seed actor match: %v", err)
	}
	for index := 0; index < 32; index++ {
		id := fmt.Sprintf("movie_recommendation_cap_%02d", index)
		title := fmt.Sprintf("Recommendation Cap %02d", index)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, genres_json, added_at)
			VALUES (?, 'lib_movies', 'movie', ?, ?, ?, '["Action"]', ?)`,
			id, title, title, "https://media.example.test/"+id+".mp4", now); err != nil {
			t.Fatalf("insert recommendation cap media %d: %v", index, err)
		}
		if _, err := db.Exec(`
			INSERT INTO media_people (id, media_id, name, role, source, sort_order, created_at)
			VALUES (?, ?, 'Ari Vega', 'Actor', 'tmdb', ?, ?)`,
			"person_"+id, id, index+1, now); err != nil {
			t.Fatalf("insert recommendation cap person %d: %v", index, err)
		}
	}

	item, err := server.getMediaDetail(userID, "movie_meridian")
	if err != nil {
		t.Fatalf("load media detail: %v", err)
	}
	if len(item.RecommendationRows) < 3 {
		t.Fatalf("expected multiple recommendation rows, got %#v", item.RecommendationRows)
	}
	if row := recommendationRowByTitle(item.RecommendationRows, "Related Movies"); row == nil || !mediaIDsContain(row.Items, "movie_blackwater") {
		t.Fatalf("related movies row missing shared-genre item: %#v", item.RecommendationRows)
	}
	if row := recommendationRowByTitle(item.RecommendationRows, "More by Noor Patel"); row == nil || !mediaIDsContain(row.Items, "movie_orchid") {
		t.Fatalf("director recommendation row missing: %#v", item.RecommendationRows)
	}
	if row := recommendationRowByTitle(item.RecommendationRows, "More with Ari Vega"); row == nil || !mediaIDsContain(row.Items, "movie_blackwater") {
		t.Fatalf("actor recommendation row missing: %#v", item.RecommendationRows)
	}
	for _, row := range item.RecommendationRows {
		if len(row.Items) > 24 {
			t.Fatalf("recommendation row %s returned %d items, expected at most 24", row.ID, len(row.Items))
		}
	}
	playbackDetail, err := server.getMediaPlaybackDetail(userID, "movie_meridian")
	if err != nil {
		t.Fatalf("load playback detail: %v", err)
	}
	if len(playbackDetail.RecommendationRows) != 0 {
		t.Fatalf("playback detail should not include recommendations: %#v", playbackDetail.RecommendationRows)
	}
	if len(playbackDetail.Children) != 0 || len(playbackDetail.Extras) != 0 || len(playbackDetail.ProviderIDs) != 0 || len(playbackDetail.MatchCandidates) != 0 || len(playbackDetail.IdentityEvidence) != 0 || len(playbackDetail.People) != 0 || len(playbackDetail.MediaImages) != 0 {
		t.Fatalf("playback detail included detail-only enrichment: children=%d extras=%d providerIDs=%d candidates=%d evidence=%d people=%d images=%d",
			len(playbackDetail.Children), len(playbackDetail.Extras), len(playbackDetail.ProviderIDs), len(playbackDetail.MatchCandidates), len(playbackDetail.IdentityEvidence), len(playbackDetail.People), len(playbackDetail.MediaImages))
	}
}

func TestHomeTrendingFallsBackToLocalItems(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	row := fetchHomeRow(t, client, serverURL, "tmdb_trending", 24)
	if len(row.Items) == 0 {
		t.Fatalf("expected local trending fallback row, got %#v", row)
	}
	for _, item := range row.Items {
		if item.Type != "movie" && item.Type != "show" && item.Type != "anime" {
			t.Fatalf("unexpected trending item type %q in %#v", item.Type, row.Items)
		}
	}
}

func TestHomeDiscoveryRowsAreAddressableManifestSections(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watchlisted, watched, progress_seconds, rating, last_played_at, updated_at)
		VALUES (?, ?, 'movie_meridian', 0, 1, 1200, 5, ?, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET
			watched = excluded.watched,
			progress_seconds = excluded.progress_seconds,
			rating = excluded.rating,
			last_played_at = excluded.last_played_at,
			updated_at = excluded.updated_at`,
		userID, userID, now, now,
	)
	if err != nil {
		t.Fatalf("insert user media state: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var home HomeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &home)
	if status != http.StatusOK {
		t.Fatalf("home status = %d, body: %s", status, body)
	}
	if len(home.Rows) < 5 {
		t.Fatalf("expected front page rows, got %#v", home.Rows)
	}
	for _, id := range []string{"recommended_for_you", "tmdb_trending", "watchlist", "favorites"} {
		row := homeRowByID(home, id)
		if row == nil || row.Endpoint != "/api/home/rows/"+id || !row.CursorCapable || len(row.Items) != 0 {
			t.Fatalf("discovery row %q is not a lazy manifest descriptor: %#v", id, row)
		}
	}
	if row := homeRowByID(home, "server_watching_week"); row != nil {
		t.Fatalf("single-member activity row should remain private: %#v", row)
	}
}

func fetchHomeRow(t *testing.T, client *http.Client, serverURL, rowID string, limit int) *HomeRow {
	t.Helper()
	var row HomeRow
	status, body := doJSON(t, client, http.MethodGet, fmt.Sprintf("%s/api/home/rows/%s?limit=%d", serverURL, rowID, limit), nil, &row)
	if status != http.StatusOK {
		t.Fatalf("home row %s status = %d, body: %s", rowID, status, body)
	}
	return &row
}

func seedActivitySharingUser(t *testing.T, db *sql.DB, userID, username string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'user', '{}', '{}', ?, ?)`, userID, username, username+"@example.test", username, now, now); err != nil {
		t.Fatalf("insert activity-sharing user %s: %v", userID, err)
	}
	setUserPrivacyPreferencesForTest(t, db, userID, UserPrivacyPreferences{
		PauseWatchHistory:         false,
		ShowActivityToMembers:     true,
		IncludeInWatchWithFriends: true,
	})
}

func homeRowByID(home HomeResponse, id string) *HomeRow {
	for i := range home.Rows {
		if home.Rows[i].ID == id {
			return &home.Rows[i]
		}
	}
	return nil
}

func recommendationRowByTitle(rows []HomeRow, title string) *HomeRow {
	for i := range rows {
		if rows[i].Title == title {
			return &rows[i]
		}
	}
	return nil
}

func adminUserID(t *testing.T, db *sql.DB) string {
	t.Helper()
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&userID); err != nil {
		t.Fatalf("load admin user id: %v", err)
	}
	return userID
}

func newDiscoveryTestServer(t *testing.T, overrides config.Config) (string, *sql.DB, *Server) {
	t.Helper()
	chdirRepoRoot(t)

	appDataDir := t.TempDir()
	webDistDir := filepath.Join(appDataDir, "web", "dist")
	if err := os.MkdirAll(webDistDir, 0o700); err != nil {
		t.Fatalf("create test web distribution: %v", err)
	}
	if err := os.WriteFile(filepath.Join(webDistDir, "index.html"), []byte("<!doctype html><title>Portico test</title>"), 0o600); err != nil {
		t.Fatalf("write test web distribution: %v", err)
	}
	cfg := config.Config{
		Addr:                   "127.0.0.1:0",
		AppDataDir:             appDataDir,
		DatabasePath:           filepath.Join(appDataDir, "portico.db"),
		WebDistDir:             webDistDir,
		SampleMediaURL:         "https://media.example.test/sample.mp4",
		TMDBReadAccessToken:    overrides.TMDBReadAccessToken,
		TMDBAPIKey:             overrides.TMDBAPIKey,
		TMDBBaseURL:            overrides.TMDBBaseURL,
		TVDBAPIKey:             overrides.TVDBAPIKey,
		TVDBBaseURL:            overrides.TVDBBaseURL,
		AniListBaseURL:         overrides.AniListBaseURL,
		MusicBrainzBaseURL:     overrides.MusicBrainzBaseURL,
		CoverArtArchiveBaseURL: overrides.CoverArtArchiveBaseURL,
		LRCLibBaseURL:          overrides.LRCLibBaseURL,
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	s := &Server{
		cfg:            cfg,
		db:             db,
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		logSubscribers: map[chan LogEvent]bool{},
		scannerWatch:   map[string]string{},
		transcodes:     map[string]*transcodeSession{},
	}
	testServer := httptest.NewServer(s.Handler())
	t.Cleanup(testServer.Close)

	status, body := doJSON(t, testServer.Client(), http.MethodPost, testServer.URL+"/api/auth/setup", map[string]string{
		"serverName":  "Discovery Test Server",
		"username":    "admin",
		"email":       "admin@example.test",
		"displayName": "Admin",
		"password":    "Password1234",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("setup status = %d, body: %s", status, body)
	}
	if _, err := db.Exec(`
		INSERT INTO user_library_access (user_id, library_id, created_at)
		SELECT users.id, libraries.id, ?
		FROM users
		CROSS JOIN libraries
		WHERE users.role IN ('owner', 'admin')
		ON CONFLICT(user_id, library_id) DO NOTHING`,
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("grant seeded library access: %v", err)
	}
	return testServer.URL, db, s
}
