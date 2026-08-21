package app

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestHomePivotsDoNotExposeUnimplementedSocialDiscovery(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var home HomeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &home)
	if status != http.StatusOK {
		t.Fatalf("home status = %d, body: %s", status, body)
	}
	for _, pivot := range home.Pivots {
		if pivot == "Find Friends" {
			t.Fatalf("home exposed removed pivot: %+v", home.Pivots)
		}
	}
}

func TestHomeWatchlistRowReflectsMutation(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	before := fetchHomeRow(t, client, serverURL, "watchlist", 24)
	if mediaIDsContain(before.Items, "movie_meridian") {
		t.Fatalf("test fixture unexpectedly starts watchlisted: %#v", before.Items)
	}

	var item MediaItem
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/watchlist", map[string]bool{"watchlisted": true}, &item)
	if status != http.StatusOK {
		t.Fatalf("watchlist status = %d, body: %s", status, body)
	}

	after := fetchHomeRow(t, client, serverURL, "watchlist", 24)
	if !mediaIDsContain(after.Items, "movie_meridian") {
		t.Fatalf("watchlist row did not reflect mutation: %#v", after.Items)
	}
}

func TestHomeContinueWatchingResolvesTVContainersToPlayableEpisode(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	states := []struct {
		mediaID  string
		progress int
		playedAt time.Time
	}{
		{mediaID: "show_northbridge", progress: 120, playedAt: now},
		{mediaID: "season_northbridge_1", progress: 180, playedAt: now.Add(-time.Minute)},
		{mediaID: "episode_northbridge_102", progress: 240, playedAt: now.Add(-2 * time.Minute)},
	}
	for _, state := range states {
		if _, err := db.Exec(`
			INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
			VALUES (?, ?, ?, 0, ?, ?, ?)
			ON CONFLICT(profile_id, media_id) DO UPDATE SET
				watched = excluded.watched,
				progress_seconds = excluded.progress_seconds,
				last_played_at = excluded.last_played_at,
				updated_at = excluded.updated_at`,
			userID, userID, state.mediaID, state.progress, state.playedAt.Format(time.RFC3339), state.playedAt.Format(time.RFC3339)); err != nil {
			t.Fatalf("seed continue state for %s: %v", state.mediaID, err)
		}
	}
	if _, err := db.Exec(`UPDATE media_items SET source_url = '/container/leak.mkv' WHERE id IN ('show_northbridge', 'season_northbridge_1')`); err != nil {
		t.Fatalf("seed container source urls: %v", err)
	}

	var home HomeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &home)
	if status != http.StatusOK {
		t.Fatalf("home status = %d, body: %s", status, body)
	}
	row := homeRowByID(home, "continue")
	if row == nil {
		t.Fatalf("continue row missing: %#v", home.Rows)
	}
	if mediaIDsContain(row.Items, "show_northbridge") || mediaIDsContain(row.Items, "season_northbridge_1") {
		t.Fatalf("continue row exposed TV containers: %#v", row.Items)
	}
	count := 0
	for _, item := range row.Items {
		if item.ID == "episode_northbridge_102" {
			count++
			if item.Type != "episode" {
				t.Fatalf("continue item type = %q, expected episode", item.Type)
			}
		}
	}
	if count != 1 {
		t.Fatalf("continue row should include active episode once, got %d in %#v", count, row.Items)
	}
}

func TestHomeContinueWatchingShowsOnlyMostRecentEpisodePerShow(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	states := []struct {
		mediaID  string
		progress int
		playedAt time.Time
	}{
		{mediaID: "episode_northbridge_101", progress: 900, playedAt: now.Add(-10 * time.Minute)},
		{mediaID: "episode_northbridge_102", progress: 420, playedAt: now},
	}
	for _, state := range states {
		if _, err := db.Exec(`
			INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
			VALUES (?, ?, ?, 0, ?, ?, ?)
			ON CONFLICT(profile_id, media_id) DO UPDATE SET
				watched = excluded.watched,
				progress_seconds = excluded.progress_seconds,
				last_played_at = excluded.last_played_at,
				updated_at = excluded.updated_at`,
			userID, userID, state.mediaID, state.progress, state.playedAt.Format(time.RFC3339), state.playedAt.Format(time.RFC3339)); err != nil {
			t.Fatalf("seed continue state for %s: %v", state.mediaID, err)
		}
	}

	var home HomeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &home)
	if status != http.StatusOK {
		t.Fatalf("home status = %d, body: %s", status, body)
	}
	row := homeRowByID(home, "continue")
	if row == nil {
		t.Fatalf("continue row missing: %#v", home.Rows)
	}
	if !mediaIDsContain(row.Items, "episode_northbridge_102") {
		t.Fatalf("continue row did not include most recent episode: %#v", row.Items)
	}
	if mediaIDsContain(row.Items, "episode_northbridge_101") {
		t.Fatalf("continue row included multiple episodes from the same show: %#v", row.Items)
	}
}

func TestHomeContinueWatchingUsesNextEpisodeWhenOnlyTVContainerHasProgress(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES (?, ?, 'season_northbridge_1', 0, 180, ?, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET
			watched = excluded.watched,
			progress_seconds = excluded.progress_seconds,
			last_played_at = excluded.last_played_at,
			updated_at = excluded.updated_at`,
		userID, userID, now, now); err != nil {
		t.Fatalf("seed season continue state: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, updated_at)
		VALUES (?, ?, 'episode_northbridge_101', 1, 0, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET
			watched = excluded.watched,
			progress_seconds = excluded.progress_seconds,
			updated_at = excluded.updated_at`,
		userID, userID, now); err != nil {
		t.Fatalf("seed watched episode: %v", err)
	}

	var home HomeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &home)
	if status != http.StatusOK {
		t.Fatalf("home status = %d, body: %s", status, body)
	}
	row := homeRowByID(home, "continue")
	if row == nil {
		t.Fatalf("continue row missing: %#v", home.Rows)
	}
	if !mediaIDsContain(row.Items, "episode_northbridge_102") {
		t.Fatalf("continue row did not use next episode after watched episode: %#v", row.Items)
	}
	if mediaIDsContain(row.Items, "season_northbridge_1") {
		t.Fatalf("continue row exposed season container: %#v", row.Items)
	}
}

func TestHomeContinueWatchingShowsOnlyMostRecentTrackPerAlbum(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO media_items (
			id, library_id, parent_id, type, title, sort_title, year, duration_seconds, genres_json, tags_json, labels_json,
			added_at, index_number, art_seed, source_url
		)
		VALUES ('track_mara_02', 'lib_music', 'album_mara', 'track', 'Signal After Dark', 'Signal After Dark', 2024, 199, '[]', '[]', '[]', ?, 2, 'album-electric-indigo', '/media/music/track2.flac')
		ON CONFLICT(id) DO NOTHING`, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert second track: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (
			id, library_id, type, title, sort_title, year, duration_seconds, genres_json, tags_json, labels_json,
			added_at, index_number, art_seed, source_url, typed_metadata_json
		)
		VALUES ('track_mara_duplicate_album', 'lib_music', 'track', 'Duplicate Album Track', 'Duplicate Album Track', 2024, 188, '[]', '[]', '[]', ?, 3, 'album-electric-indigo', '/media/music/track3.flac', '{"albumTitle":"Late Trains for Bright Cities","albumArtist":"Mara Vale"}')
		ON CONFLICT(id) DO NOTHING`, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert duplicate album track: %v", err)
	}
	states := []struct {
		mediaID  string
		progress int
		playedAt time.Time
	}{
		{mediaID: "track_mara_02", progress: 0, playedAt: now},
		{mediaID: "track_mara_01", progress: 0, playedAt: now.Add(-time.Minute)},
		{mediaID: "track_mara_duplicate_album", progress: 0, playedAt: now.Add(-2 * time.Minute)},
	}
	for _, state := range states {
		if _, err := db.Exec(`
			INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
			VALUES (?, ?, ?, 0, ?, ?, ?)
			ON CONFLICT(profile_id, media_id) DO UPDATE SET
				watched = excluded.watched,
				progress_seconds = excluded.progress_seconds,
				last_played_at = excluded.last_played_at,
				updated_at = excluded.updated_at`,
			userID, userID, state.mediaID, state.progress, state.playedAt.Format(time.RFC3339), state.playedAt.Format(time.RFC3339)); err != nil {
			t.Fatalf("seed track state for %s: %v", state.mediaID, err)
		}
	}

	var home HomeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &home)
	if status != http.StatusOK {
		t.Fatalf("home status = %d, body: %s", status, body)
	}
	row := homeRowByID(home, "continue_listening")
	if row == nil {
		t.Fatalf("continue listening row missing: %#v", home.Rows)
	}
	if !mediaIDsContain(row.Items, "track_mara_02") {
		t.Fatalf("continue listening row did not include most recent album track: %#v", row.Items)
	}
	if mediaIDsContain(row.Items, "track_mara_01") {
		t.Fatalf("continue listening row included more than one track from the same album: %#v", row.Items)
	}
	if mediaIDsContain(row.Items, "track_mara_duplicate_album") {
		t.Fatalf("continue listening row included duplicate album identity without a shared parent id: %#v", row.Items)
	}
}

func TestHomeContinueListeningHasIndependentWindow(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	for i := 0; i < 14; i++ {
		id := fmt.Sprintf("movie_resume_%02d", i)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, year, duration_seconds, genres_json, tags_json, labels_json, added_at, art_seed, source_url)
			VALUES (?, 'lib_movies', 'movie', ?, ?, 2024, 5400, '[]', '[]', '[]', ?, ?, 'https://media.example.test/movie.mp4')
			ON CONFLICT(id) DO NOTHING`,
			id, id, id, now.Format(time.RFC3339), id); err != nil {
			t.Fatalf("insert resume movie: %v", err)
		}
		playedAt := now.Add(time.Duration(i) * time.Minute)
		if _, err := db.Exec(`
			INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
			VALUES (?, ?, ?, 0, 300, ?, ?)
			ON CONFLICT(profile_id, media_id) DO UPDATE SET
				watched = excluded.watched,
				progress_seconds = excluded.progress_seconds,
				last_played_at = excluded.last_played_at,
				updated_at = excluded.updated_at`,
			userID, userID, id, playedAt.Format(time.RFC3339), playedAt.Format(time.RFC3339)); err != nil {
			t.Fatalf("seed movie state: %v", err)
		}
	}
	trackPlayedAt := now.Add(-4 * time.Hour)
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES (?, ?, 'track_mara_01', 0, 0, ?, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET
			watched = excluded.watched,
			progress_seconds = excluded.progress_seconds,
			last_played_at = excluded.last_played_at,
			updated_at = excluded.updated_at`,
		userID, userID, trackPlayedAt.Format(time.RFC3339), trackPlayedAt.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed track state: %v", err)
	}

	continueRow := fetchHomeRow(t, client, serverURL, "continue", 24)
	if len(continueRow.Items) < 14 || !mediaIDsContain(continueRow.Items, "movie_resume_13") {
		t.Fatalf("continue watching row missing recent video items: %#v", continueRow)
	}
	listeningRow := fetchHomeRow(t, client, serverURL, "continue_listening", 24)
	if !mediaIDsContain(listeningRow.Items, "track_mara_01") {
		t.Fatalf("continue listening row was starved by video resume limit: %#v", listeningRow)
	}
}

func TestLibraryCanExcludeItemsFromContinueRows(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	states := []struct {
		mediaID  string
		progress int
	}{
		{mediaID: "movie_meridian", progress: 600},
		{mediaID: "track_mara_01", progress: 0},
	}
	for _, state := range states {
		if _, err := db.Exec(`
			INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
			VALUES (?, ?, ?, 0, ?, ?, ?)
			ON CONFLICT(profile_id, media_id) DO UPDATE SET
				watched = excluded.watched,
				progress_seconds = excluded.progress_seconds,
				last_played_at = excluded.last_played_at,
				updated_at = excluded.updated_at`,
			userID, userID, state.mediaID, state.progress, now, now); err != nil {
			t.Fatalf("seed continue state for %s: %v", state.mediaID, err)
		}
	}

	var before HomeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &before)
	if status != http.StatusOK {
		t.Fatalf("home status = %d, body: %s", status, body)
	}
	if row := homeRowByID(before, "continue"); row == nil || !mediaIDsContain(row.Items, "movie_meridian") {
		t.Fatalf("continue row missing seeded movie before exclusion: %#v", before.Rows)
	}
	if row := homeRowByID(before, "continue_listening"); row == nil || !mediaIDsContain(row.Items, "track_mara_01") {
		t.Fatalf("continue listening row missing seeded track before exclusion: %#v", before.Rows)
	}

	var movieLibrary Library
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/libraries/lib_movies", map[string]any{
		"settings": map[string]any{"includeInContinueWatching": false},
	}, &movieLibrary)
	if status != http.StatusOK {
		t.Fatalf("movie library update status = %d, body: %s", status, body)
	}
	if settingBool(movieLibrary.Settings, "includeInContinueWatching", true) {
		t.Fatalf("movie library did not persist includeInContinueWatching=false: %#v", movieLibrary.Settings)
	}
	var musicLibrary Library
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/libraries/lib_music", map[string]any{
		"settings": map[string]any{"includeInContinueWatching": false},
	}, &musicLibrary)
	if status != http.StatusOK {
		t.Fatalf("music library update status = %d, body: %s", status, body)
	}
	if settingBool(musicLibrary.Settings, "includeInContinueWatching", true) {
		t.Fatalf("music library did not persist includeInContinueWatching=false: %#v", musicLibrary.Settings)
	}

	var after HomeResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &after)
	if status != http.StatusOK {
		t.Fatalf("home after exclusion status = %d, body: %s", status, body)
	}
	if row := homeRowByID(after, "continue"); row != nil && mediaIDsContain(row.Items, "movie_meridian") {
		t.Fatalf("continue row included excluded movie library item: %#v", row.Items)
	}
	if row := homeRowByID(after, "continue_listening"); row != nil && mediaIDsContain(row.Items, "track_mara_01") {
		t.Fatalf("continue listening row included excluded music library item: %#v", row.Items)
	}
}

func TestHomeRecommendationsUsePersistentCache(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	if err := server.storeLocalRecommendationScores(userID, []scoredMedia{{
		ID:     "movie_saffron",
		Score:  99,
		Reason: "Cached recommendation fixture",
		Source: "test_cache",
	}}); err != nil {
		t.Fatalf("store recommendation cache: %v", err)
	}

	rows := server.recommendationRows(User{ID: userID})
	if len(rows) != 1 || len(rows[0].Items) != 1 || rows[0].Items[0].ID != "movie_saffron" {
		t.Fatalf("recommendation row did not use cache: %#v", rows)
	}

	var cached int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_recommendation_cache WHERE user_id = ?`, userID).Scan(&cached); err != nil {
		t.Fatalf("count recommendation cache: %v", err)
	}
	if cached != 1 {
		t.Fatalf("recommendation cache count = %d, expected 1", cached)
	}
	server.invalidateHomeCache()
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_recommendation_cache WHERE user_id = ?`, userID).Scan(&cached); err != nil {
		t.Fatalf("count recommendation cache after invalidation: %v", err)
	}
	if cached != 0 {
		t.Fatalf("recommendation cache survived home invalidation: %d", cached)
	}
}

func TestExpiredHomeRecommendationCacheRebuilds(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO user_recommendation_cache (profile_id, user_id, media_id, rank, score, reason, source, generated_at, expires_at)
		VALUES (?, ?, 'movie_saffron', 1, 99, 'Expired cache fixture', 'test_cache', ?, ?)`,
		userID, userID, now.Add(-2*time.Hour).Format(time.RFC3339), now.Add(-time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("insert expired recommendation cache: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, updated_at)
		VALUES (?, ?, 'movie_meridian', 1, 120, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET watched = excluded.watched, progress_seconds = excluded.progress_seconds, updated_at = excluded.updated_at`,
		userID, userID, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed recommendation input: %v", err)
	}
	if _, err := server.localRecommendationScores(userID, 12); err != nil {
		t.Fatalf("rebuild recommendation cache: %v", err)
	}
	var expiresAt string
	if err := db.QueryRow(`SELECT expires_at FROM user_recommendation_cache WHERE user_id = ? ORDER BY rank ASC LIMIT 1`, userID).Scan(&expiresAt); err != nil {
		t.Fatalf("load rebuilt recommendation cache expiry: %v", err)
	}
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || !expires.After(now) {
		t.Fatalf("recommendation cache expiry was not refreshed: %q err=%v", expiresAt, err)
	}
}
