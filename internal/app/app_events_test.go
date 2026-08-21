package app

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestAppEventsStreamPublishesAuthenticatedDataChanges(t *testing.T) {
	serverURL, _, _ := newRemoteAccessTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/events", nil)
	if err != nil {
		t.Fatalf("app events request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("app events response: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("app events status=%d content-type=%s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	var remoteStatus RemoteAccessStatus
	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{
		"enabled":          true,
		"hostedBaseUrl":    "https://api.getportico.tv",
		"publicPortMode":   "manual",
		"manualPublicPort": 32401,
	}, &remoteStatus)
	if status != http.StatusOK {
		t.Fatalf("remote access settings status = %d, body: %s", status, body)
	}

	reader := bufio.NewReader(resp.Body)
	event := readAppEventWithTags(t, reader, "remote-access", "settings")
	if event.Type != "data.changed" {
		t.Fatalf("event type = %q", event.Type)
	}
	for _, tag := range []string{"settings", "remote-access"} {
		if !stringSliceContains(event.Tags, tag) {
			t.Fatalf("expected tag %q in %#v", tag, event.Tags)
		}
	}
	if stringSliceContains(event.Tags, "dashboard") {
		t.Fatalf("remote access settings should not publish broad dashboard tag: %#v", event.Tags)
	}
}

func TestAppEventsStreamClosesWhenCredentialExpires(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	previousInterval := eventStreamAuthorizationCheck
	eventStreamAuthorizationCheck = 15 * time.Millisecond
	t.Cleanup(func() { eventStreamAuthorizationCheck = previousInterval })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/events", nil)
	if err != nil {
		t.Fatalf("app events request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("app events response: %v", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	if line, err := reader.ReadString('\n'); err != nil || !strings.HasPrefix(line, ": connected") {
		t.Fatalf("initial stream frame = %q, err=%v", line, err)
	}
	if line, err := reader.ReadString('\n'); err != nil || strings.TrimSpace(line) != "" {
		t.Fatalf("initial stream separator = %q, err=%v", line, err)
	}

	cookie := sessionCookieFromJar(t, jar, serverURL)
	if _, err := db.Exec(`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), hashToken(cookie.Value)); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, err := reader.ReadString('\n'); err != nil {
				return
			}
		}
	}()
	select {
	case <-closed:
	case <-time.After(750 * time.Millisecond):
		t.Fatal("application event stream remained open after its credential expired")
	}
}

func TestUserMediaStateDataTagsStayLightweight(t *testing.T) {
	tags := dataTagsForSQL(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id, media_id) DO UPDATE SET progress_seconds = excluded.progress_seconds`)
	if !stringSliceContains(tags, "playback-progress") {
		t.Fatalf("expected playback-progress tag in %#v", tags)
	}
	if !stringSliceContains(tags, "media-state") {
		t.Fatalf("expected media-state tag when user_media_state updates watched state: %#v", tags)
	}
	for _, heavy := range []string{"home", "media", "playlists", "library-items", "dashboard"} {
		if stringSliceContains(tags, heavy) {
			t.Fatalf("user_media_state write should not publish heavyweight tag %q: %#v", heavy, tags)
		}
	}

	progressTags := dataTagsForSQL(`
		UPDATE user_media_state
		SET progress_seconds = ?, last_played_at = ?, updated_at = ?
		WHERE user_id = ? AND media_id = ?`)
	if !stringSliceContains(progressTags, "playback-progress") {
		t.Fatalf("expected playback-progress tag in %#v", progressTags)
	}
	if stringSliceContains(progressTags, "media-state") {
		t.Fatalf("progress-only write should not publish media-state tag: %#v", progressTags)
	}
}

func TestSavedMutationPublishesEveryAffectedViewerProjection(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	profileID := adminUserID(t, db)
	events := server.subscribeAppEvents()
	defer server.unsubscribeAppEvents(events)

	if err := server.upsertState(profileID, "movie_meridian", "watchlisted", 1); err != nil {
		t.Fatalf("watchlist media: %v", err)
	}
	event := receiveAppEvent(t, events)
	for _, expected := range []string{"saved", "media-state", "library-items", "home"} {
		if !stringSliceContains(event.Tags, expected) {
			t.Fatalf("saved mutation omitted %q from %#v", expected, event.Tags)
		}
	}
	if stringSliceContains(event.Tags, "playback-progress") {
		t.Fatalf("saved mutation unnecessarily invalidated playback progress: %#v", event.Tags)
	}
}

func TestLibraryScanCompletionPublishesAllClientBrowseProjections(t *testing.T) {
	server := newScannerTestServer(t)
	events := server.subscribeAppEvents()
	defer server.unsubscribeAppEvents(events)

	server.publishLibraryScanCompleted("library_movies")
	event := receiveAppEvent(t, events)
	if event.Type != "library.scan.completed" || event.Resource != "library" || event.ResourceID != "library_movies" {
		t.Fatalf("unexpected scan event: %#v", event)
	}
	for _, expected := range []string{"home", "libraries", "library-items", "media", "metadata", "search"} {
		if !stringSliceContains(event.Tags, expected) {
			t.Fatalf("scan completion omitted %q from %#v", expected, event.Tags)
		}
	}
}

func TestSlowApplicationEventSubscriberDisconnectsInsteadOfLosingContinuity(t *testing.T) {
	server := newScannerTestServer(t)
	events := server.subscribeAppEvents()
	defer server.unsubscribeAppEvents(events)

	for index := 0; index < cap(events)+1; index++ {
		server.publishDataChanged("data.changed", []string{"home"}, "test", "", nil)
	}
	for index := 0; index < cap(events); index++ {
		if _, ok := <-events; !ok {
			t.Fatalf("subscriber closed before draining the %d already-delivered frames", cap(events))
		}
	}
	if _, ok := <-events; ok {
		t.Fatal("overflowed subscriber remained open after continuity was lost")
	}
}

func TestSQLInvalidationVocabularyCoversUserVisibleDomains(t *testing.T) {
	cases := []struct {
		query string
		tags  []string
	}{
		{`UPDATE profiles SET display_name = ? WHERE id = ?`, []string{"profiles", "account"}},
		{`INSERT INTO viewer_notifications (id) VALUES (?)`, []string{"notifications"}},
		{`DELETE FROM collection_items WHERE collection_id = ?`, []string{"collections"}},
		{`UPDATE downloads SET status = ? WHERE id = ?`, []string{"downloads"}},
		{`UPDATE media_images SET preferred = 1 WHERE id = ?`, []string{"media", "metadata", "library-items"}},
		{`DELETE FROM playback_session_history WHERE session_id = ?`, []string{"playback", "dashboard:history"}},
	}
	for _, test := range cases {
		tags := dataTagsForSQL(test.query)
		for _, expected := range test.tags {
			if !stringSliceContains(tags, expected) {
				t.Fatalf("query %q omitted %q from %#v", test.query, expected, tags)
			}
		}
	}
}

func TestApplicationEventsDoNotCrossProfileOrLibraryAccessBoundaries(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	owner, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatalf("load owner: %v", err)
	}
	owner.AccountID = owner.ID
	owner.ProfileID = owner.ID
	other := User{ID: "account_other", AccountID: "account_other", ProfileID: "profile_other", Role: "user"}

	profileEvent := AppEvent{ID: 1, Type: "data.changed", Tags: []string{"playback-progress"}, CreatedAt: time.Now().UTC().Format(time.RFC3339), audienceAccountID: owner.ID, audienceProfileID: owner.ID}
	if _, ok := server.projectAppEventForUserContext(context.Background(), owner, profileEvent); !ok {
		t.Fatal("exact profile did not receive its own progress invalidation")
	}
	if _, ok := server.projectAppEventForUserContext(context.Background(), other, profileEvent); ok {
		t.Fatal("profile-scoped progress invalidation crossed into another profile")
	}

	mediaEvent := AppEvent{ID: 2, Type: "data.changed", Tags: []string{"metadata"}, Resource: "media", ResourceID: "movie_meridian", Fields: map[string]string{"provider": "private"}, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if projected, ok := server.projectAppEventForUserContext(context.Background(), owner, mediaEvent); !ok || projected.ResourceID != "movie_meridian" {
		t.Fatalf("authorized owner projection = %#v, ok=%v", projected, ok)
	}
	if _, ok := server.projectAppEventForUserContext(context.Background(), other, mediaEvent); ok {
		t.Fatal("restricted profile received an inaccessible media identifier")
	}

	unscoped := AppEvent{ID: 3, Type: "data.changed", Tags: []string{"settings"}, Resource: "internal", ResourceID: "secret-id", Fields: map[string]string{"detail": "secret"}, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	projected, ok := server.projectAppEventForUserContext(context.Background(), other, unscoped)
	if !ok || projected.ResourceID != "" || len(projected.Fields) != 0 {
		t.Fatalf("unproven global audience was not content-free: %#v, ok=%v", projected, ok)
	}
}

func TestApplicationEventSSEAndLongPollUseIdenticalAudienceProjection(t *testing.T) {
	server := newScannerTestServer(t)
	server.longPoll = newLongPollRuntime()
	viewer := User{ID: "account_a", AccountID: "account_a", ProfileID: "profile_a", Role: "user"}
	other := User{ID: "account_b", AccountID: "account_b", ProfileID: "profile_b", Role: "user"}
	sse := server.subscribeAppEvents()
	defer server.unsubscribeAppEvents(sse)

	server.publishDataChangedForViewer("data.changed", []string{"playback-progress"}, "database", "", nil, viewer.AccountID, viewer.ProfileID)
	sseRaw := receiveAppEvent(t, sse)
	pollRaw, _, _, overflow := server.longPoll.broker.appEventsAfter(0)
	if overflow || len(pollRaw) != 1 {
		t.Fatalf("long-poll retained events = %#v, overflow=%v", pollRaw, overflow)
	}
	for _, principal := range []User{viewer, other} {
		sseProjected := server.projectAppEventsForUserContext(context.Background(), principal, []AppEvent{sseRaw})
		pollProjected := server.projectAppEventsForUserContext(context.Background(), principal, pollRaw)
		if !reflect.DeepEqual(sseProjected, pollProjected) {
			t.Fatalf("transport projections differ for %s: sse=%#v poll=%#v", principal.ProfileID, sseProjected, pollProjected)
		}
	}
	if projected := server.projectAppEventsForUserContext(context.Background(), other, pollRaw); len(projected) != 0 {
		t.Fatalf("other profile received scoped long-poll events: %#v", projected)
	}
}

func receiveAppEvent(t *testing.T, events <-chan AppEvent) AppEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for application event")
		return AppEvent{}
	}
}

func TestPlaybackProgressDoesNotInvalidateSmartPlaylistCache(t *testing.T) {
	server := newScannerTestServer(t)
	server.smartPlaylistCacheMu.Lock()
	if server.smartPlaylistCache == nil {
		server.smartPlaylistCache = map[string]smartPlaylistCacheEntry{}
	}
	server.smartPlaylistCache["progress-only"] = smartPlaylistCacheEntry{items: []MediaItem{{ID: "cached"}}, expiresAt: time.Now().Add(time.Minute)}
	server.smartPlaylistCacheMu.Unlock()

	server.publishDataChanged("data.changed", []string{"playback-progress"}, "media", "cached", nil)

	server.smartPlaylistCacheMu.Lock()
	_, ok := server.smartPlaylistCache["progress-only"]
	server.smartPlaylistCacheMu.Unlock()
	if !ok {
		t.Fatalf("progress-only playback event invalidated smart playlist cache")
	}

	server.publishDataChanged("data.changed", []string{"media-state"}, "media", "cached", nil)

	server.smartPlaylistCacheMu.Lock()
	_, ok = server.smartPlaylistCache["progress-only"]
	server.smartPlaylistCacheMu.Unlock()
	if ok {
		t.Fatalf("media-state event did not invalidate smart playlist cache")
	}
}

func TestMediaWriteDataTagsAvoidSearchAndDashboardFanout(t *testing.T) {
	tags := dataTagsForSQL(`
		UPDATE media_items
		SET title = ?, updated_at = ?
		WHERE id = ?`)
	for _, expected := range []string{"media", "library-items"} {
		if !stringSliceContains(tags, expected) {
			t.Fatalf("expected media write tag %q in %#v", expected, tags)
		}
	}
	for _, heavy := range []string{"home", "search", "dashboard"} {
		if stringSliceContains(tags, heavy) {
			t.Fatalf("media item write should not publish broad tag %q: %#v", heavy, tags)
		}
	}

	searchTags := dataTagsForSQL(`
		INSERT INTO media_search (media_id, title, summary, genres)
		VALUES (?, ?, ?, ?)`)
	if !stringSliceContains(searchTags, "search") {
		t.Fatalf("media_search write should publish search tag: %#v", searchTags)
	}
	if stringSliceContains(searchTags, "dashboard") {
		t.Fatalf("media_search write should not publish dashboard tag: %#v", searchTags)
	}
}

func TestPlaylistWriteDataTagsStayScopedToPlaylists(t *testing.T) {
	tags := dataTagsForSQL(`
		INSERT INTO playlist_items (playlist_id, media_id, sort_order, added_at)
		VALUES (?, ?, ?, ?)`)
	if !stringSliceContains(tags, "playlists") {
		t.Fatalf("expected playlists tag in %#v", tags)
	}
	for _, heavy := range []string{"home", "media", "library-items", "dashboard"} {
		if stringSliceContains(tags, heavy) {
			t.Fatalf("playlist write should not publish heavyweight tag %q: %#v", heavy, tags)
		}
	}
}

func TestDashboardDataTagsStayScoped(t *testing.T) {
	jobTags := dataTagsForSQL(`
		UPDATE jobs
		SET status = ?, updated_at = ?
		WHERE id = ?`)
	if !stringSliceContains(jobTags, "dashboard:jobs") || !stringSliceContains(jobTags, "jobs") {
		t.Fatalf("expected scoped dashboard job tags in %#v", jobTags)
	}
	if stringSliceContains(jobTags, "dashboard") || stringSliceContains(jobTags, "dashboard:history") {
		t.Fatalf("job write should not publish broad/history dashboard tags: %#v", jobTags)
	}

	playbackTags := dataTagsForSQL(`
		UPDATE playback_sessions
		SET last_seen_at = ?, state = ?
		WHERE id = ?`)
	for _, expected := range []string{"playback", "dashboard:live"} {
		if !stringSliceContains(playbackTags, expected) {
			t.Fatalf("expected playback dashboard tag %q in %#v", expected, playbackTags)
		}
	}
	for _, unexpected := range []string{"dashboard", "dashboard:history", "jobs"} {
		if stringSliceContains(playbackTags, unexpected) {
			t.Fatalf("playback heartbeat write should not publish %q: %#v", unexpected, playbackTags)
		}
	}

	closedPlaybackTags := dataTagsForSQL(`
		UPDATE playback_sessions
		SET ended_at = ?, state = 'stopped'
		WHERE id = ?`)
	if !stringSliceContains(closedPlaybackTags, "dashboard:history") {
		t.Fatalf("playback close should refresh dashboard history tags: %#v", closedPlaybackTags)
	}
	if stringSliceContains(closedPlaybackTags, "dashboard") || stringSliceContains(closedPlaybackTags, "jobs") {
		t.Fatalf("playback close should not publish broad dashboard/job tags: %#v", closedPlaybackTags)
	}
}

func TestLiveTVDataTagsAvoidBroadDashboardFanout(t *testing.T) {
	liveTags := dataTagsForSQL(`
		UPDATE live_tv_channels
		SET hidden = ?
		WHERE id = ?`)
	for _, expected := range []string{"live-tv", "dvr", "dashboard:live"} {
		if !stringSliceContains(liveTags, expected) {
			t.Fatalf("expected live tv tag %q in %#v", expected, liveTags)
		}
	}
	if stringSliceContains(liveTags, "dashboard") {
		t.Fatalf("live tv write should not publish broad dashboard tag: %#v", liveTags)
	}
}

func readAppEventWithTags(t *testing.T, reader *bufio.Reader, tags ...string) AppEvent {
	t.Helper()
	for i := 0; i < 8; i++ {
		data := readSSEDataLine(t, reader)
		var event AppEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("decode app event %q: %v", data, err)
		}
		matches := true
		for _, tag := range tags {
			if !stringSliceContains(event.Tags, tag) {
				matches = false
				break
			}
		}
		if matches {
			return event
		}
	}
	t.Fatalf("did not receive app event containing tags %#v", tags)
	return AppEvent{}
}
