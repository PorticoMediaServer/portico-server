package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func profileIsolationUsers(t *testing.T, server *Server) (User, User) {
	t.Helper()
	primary := dvrTestUser(t, server)
	primary.ProfileID = primary.ID
	primary.ProfileIsPrimary = true
	primary.AccountID = primary.ID
	childID := "profile_isolation_child"
	now := time.Now().UTC().Format(time.RFC3339)
	restrictionsJSON, err := encodeProfileRestrictions(defaultProfileRestrictions())
	if err != nil {
		t.Fatalf("encode permissive child profile restrictions: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO profiles (
			id, account_id, origin, external_profile_id, is_primary, sort_order,
			display_name, role, permissions_json, preferences_json, restrictions_json,
			created_at, updated_at
		) VALUES (?, ?, 'hosted', 'hosted-isolation-child', 0, 1, 'Child', 'user', '{}', '{}', ?, ?, ?)`,
		childID, primary.ID, restrictionsJSON, now, now); err != nil {
		t.Fatalf("insert child profile: %v", err)
	}
	child := primary
	child.ProfileID = childID
	child.ProfileIsPrimary = false
	child.DisplayName = "Child"
	child.Permissions = map[string]bool{
		"viewDVR": true, "scheduleDVR": true, "viewLiveTV": true,
		"playMedia": true, "watchWithFriends": true,
	}
	return primary, child
}

func TestAuthorizationSensitiveCachesChangeWithEffectiveProfilePolicy(t *testing.T) {
	maximumAge := 12
	base := User{
		ID: "account-cache", AccountID: "account-cache", ProfileID: "profile-cache", Role: "user",
		Permissions: map[string]bool{"playMedia": true, "watchWithFriends": true},
		LibraryIDs:  []string{"library-b", "library-a"}, AllowUnrated: true, AllowFeedback: true,
		Preferences: UserPreferences{Privacy: UserPrivacyPreferences{IncludeInWatchWithFriends: true}},
	}
	restricted := base
	restricted.MaximumAgeRating = &maximumAge
	restricted.AllowUnrated = false
	restricted.BlockedProfileLabels = []string{"mature"}
	restricted.Permissions = map[string]bool{"playMedia": true, "watchWithFriends": false}
	restricted.Preferences.Privacy.IncludeInWatchWithFriends = false

	if homeCacheKey(base) == homeCacheKey(restricted) {
		t.Fatal("Home cache key ignored tightened profile restrictions")
	}
	request := BrowseLibraryRequest{Pivot: "all", Limit: 50}
	baseBrowse, _ := canonicalBrowseCacheKey(base, "library-a", request)
	restrictedBrowse, _ := canonicalBrowseCacheKey(restricted, "library-a", request)
	if baseBrowse == restrictedBrowse {
		t.Fatal("browse cache key ignored tightened profile restrictions")
	}
	equivalent := base
	equivalent.ProfileID = "profile-cache-equivalent"
	if emptyMediaStateCacheScope(base, false) != emptyMediaStateCacheScope(equivalent, false) {
		t.Fatal("equivalent empty-state profiles did not share the catalog projection scope")
	}
	if emptyMediaStateCacheScope(base, false) == emptyMediaStateCacheScope(restricted, false) {
		t.Fatal("empty-state catalog projection scope ignored tightened profile restrictions")
	}
	if emptyMediaStateCacheScope(base, true) == emptyMediaStateCacheScope(equivalent, true) {
		t.Fatal("profiles with personal media state shared a cache scope")
	}
	dashboardView := dashboardFilters{Mode: "live", Period: "5m"}
	if dashboardCacheKey(base, dashboardView) != dashboardCacheKey(equivalent, dashboardView) {
		t.Fatal("equivalent administrative authorization scopes did not share dashboard work")
	}
	if dashboardCacheKey(base, dashboardView) == dashboardCacheKey(restricted, dashboardView) {
		t.Fatal("dashboard cache key ignored effective authorization policy")
	}
	options := mediaDetailOptions{Recommendations: true}
	if mediaDetailCacheKey(base, "movie-neon", options) == mediaDetailCacheKey(restricted, "movie-neon", options) {
		t.Fatal("detail cache key ignored tightened profile restrictions")
	}
}

func TestWatchWithFriendsProfilesOnOneAccountRemainDistinctAndPrivate(t *testing.T) {
	server := newScannerTestServer(t)
	primary, child := profileIsolationUsers(t, server)
	primary.Permissions = map[string]bool{"watchWithFriends": true}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO watch_with_friends_groups (
			id, owner_user_id, owner_profile_id, name, media_id, media_title, created_at, updated_at
		) VALUES ('wwf_profile_isolation', ?, ?, 'Family Night', 'media_profile_isolation', 'Family Night', ?, ?)`,
		primary.ID, primary.ProfileID, now, now); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	for _, viewer := range []User{primary, child} {
		if _, err := server.db.Exec(`
			INSERT INTO watch_with_friends_members (group_id, user_id, profile_id, joined_at, last_seen_at)
			VALUES ('wwf_profile_isolation', ?, ?, ?, ?)`, primary.ID, viewer.ProfileID, now, now); err != nil {
			t.Fatalf("insert member %s: %v", viewer.ProfileID, err)
		}
	}
	if _, err := server.db.Exec(`
		INSERT INTO watch_with_friends_queue (
			group_id, media_id, media_title, sort_order, added_by_user_id, added_by_profile_id, added_at
		) VALUES ('wwf_profile_isolation', 'media_profile_isolation', 'Family Night', 0, ?, ?, ?)`,
		primary.ID, child.ProfileID, now); err != nil {
		t.Fatalf("insert queue item: %v", err)
	}

	group, err := server.watchWithFriendsGroupContext(context.Background(), "wwf_profile_isolation")
	if err != nil {
		t.Fatalf("load group: %v", err)
	}
	if len(group.Members) != 2 || group.Members[0].ProfileID == group.Members[1].ProfileID {
		t.Fatalf("same-account profiles collapsed into one participant: %#v", group.Members)
	}
	if group.OwnerProfileID != primary.ProfileID || group.Queue[0].AddedByProfileID != child.ProfileID {
		t.Fatalf("profile attribution was lost: %#v", group)
	}
	payload, err := json.Marshal(group)
	if err != nil {
		t.Fatalf("marshal group: %v", err)
	}
	if strings.Contains(string(payload), "ownerUserId") || strings.Contains(string(payload), "addedByUserId") || strings.Contains(string(payload), `"userId"`) {
		t.Fatalf("consumer payload leaked household account IDs: %s", payload)
	}
	if !strings.Contains(string(payload), primary.ProfileID) || !strings.Contains(string(payload), child.ProfileID) {
		t.Fatalf("consumer payload omitted distinct profile identities: %s", payload)
	}

	privatePreferences, _ := json.Marshal(map[string]any{"privacy": UserPrivacyPreferences{
		ShowActivityToMembers: false, IncludeInWatchWithFriends: true,
	}})
	if _, err := server.db.Exec(`UPDATE profiles SET preferences_json = ? WHERE id = ?`, string(privatePreferences), child.ProfileID); err != nil {
		t.Fatalf("set child privacy: %v", err)
	}
	visible := server.visibleWatchWithFriendsMembersContext(context.Background(), primary, group)
	if len(visible) != 1 || visible[0].ProfileID != primary.ProfileID {
		t.Fatalf("profile privacy did not hide only the child participant: %#v", visible)
	}
}

func TestDVRViewerRowsAreProfileScopedWhileOperationalConflictsAggregate(t *testing.T) {
	server := newScannerTestServer(t)
	primary, child := profileIsolationUsers(t, server)
	primary.Permissions = map[string]bool{"viewDVR": true, "scheduleDVR": true}
	now := time.Now().UTC().Truncate(time.Second)
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_profile_dvr', 'Profile DVR', 'm3u', 1, ?, ?)`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	for index, viewer := range []User{primary, child} {
		start := now.Add(time.Duration(index+2) * time.Hour).Format(time.RFC3339)
		end := now.Add(time.Duration(index+3) * time.Hour).Format(time.RFC3339)
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_recording_rules (
				id, user_id, profile_id, source_id, title, match_type, retention_days, created_at, updated_at
			) VALUES (?, ?, ?, 'src_profile_dvr', ?, 'single', 30, ?, ?)`,
			"rule_profile_"+viewer.ProfileID, primary.ID, viewer.ProfileID, viewer.DisplayName, nowText, nowText); err != nil {
			t.Fatalf("insert rule for %s: %v", viewer.ProfileID, err)
		}
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_recordings (
				id, user_id, profile_id, source_id, title, status, starts_at, ends_at, created_at, updated_at
			) VALUES (?, ?, ?, 'src_profile_dvr', ?, 'scheduled', ?, ?, ?, ?)`,
			"recording_profile_"+viewer.ProfileID, primary.ID, viewer.ProfileID, viewer.DisplayName, start, end, nowText, nowText); err != nil {
			t.Fatalf("insert recording for %s: %v", viewer.ProfileID, err)
		}
	}

	for _, viewer := range []User{primary, child} {
		rules, _, _, err := server.listDVRRulesKeysetPageForUser(context.Background(), viewer, 10, dvrRuleCursor{}, "exact")
		if err != nil || len(rules) != 1 || rules[0].ProfileID != viewer.ProfileID {
			t.Fatalf("rules for %s = %#v, err=%v", viewer.ProfileID, rules, err)
		}
		rulesPayload, err := json.Marshal(rules)
		if err != nil {
			t.Fatalf("marshal rules for %s: %v", viewer.ProfileID, err)
		}
		if strings.Contains(string(rulesPayload), `"userId"`) || !strings.Contains(string(rulesPayload), viewer.ProfileID) {
			t.Fatalf("DVR rule payload leaked account identity or omitted profile identity: %s", rulesPayload)
		}
		recordings, _, _, err := server.listDVRRecordingsKeysetPageForUser(context.Background(), viewer, 10, dvrRecordingCursor{}, "exact")
		if err != nil || len(recordings) != 1 || recordings[0].ProfileID != viewer.ProfileID {
			t.Fatalf("recordings for %s = %#v, err=%v", viewer.ProfileID, recordings, err)
		}
		recordingsPayload, err := json.Marshal(recordings)
		if err != nil {
			t.Fatalf("marshal recordings for %s: %v", viewer.ProfileID, err)
		}
		if strings.Contains(string(recordingsPayload), `"userId"`) || !strings.Contains(string(recordingsPayload), viewer.ProfileID) {
			t.Fatalf("DVR recording payload leaked account identity or omitted profile identity: %s", recordingsPayload)
		}
		otherID := "recording_profile_" + primary.ProfileID
		if viewer.ProfileID == primary.ProfileID {
			otherID = "recording_profile_" + child.ProfileID
		}
		if _, err := server.getDVRRecordingForUser(viewer, otherID); err == nil {
			t.Fatalf("profile %s could access another profile's recording", viewer.ProfileID)
		}
	}
	manager := primary
	manager.Permissions = map[string]bool{"manageDVR": true}
	all, _, _, err := server.listDVRRecordingsKeysetPageForUser(context.Background(), manager, 10, dvrRecordingCursor{}, "exact")
	if err != nil || len(all) != 1 || all[0].ProfileID != manager.ProfileID {
		t.Fatalf("manager viewer projection escaped active profile = %#v, err=%v", all, err)
	}
	conflictStart := now.Add(2*time.Hour + 15*time.Minute)
	conflictEnd := conflictStart.Add(30 * time.Minute)
	conflict, err := server.findDVRRecordingConflict("src_profile_dvr", conflictStart, conflictEnd, "")
	if err != nil || conflict.ProfileID != primary.ProfileID {
		t.Fatalf("account/server tuner conflict did not aggregate profiles: %#v, err=%v", conflict, err)
	}
}

func TestLiveTVChannelStateAndSummariesAreProfileScoped(t *testing.T) {
	server := newScannerTestServer(t)
	primary, child := profileIsolationUsers(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_profile_live', 'Profile Live', 'm3u', 1, ?, ?);
		INSERT INTO live_tv_channels (
			id, source_id, number, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at
		) VALUES ('channel_profile_live', 'src_profile_live', '1', 'Profile Channel', 'https://example.test/live.m3u8', 1, 0, ?, ?, ?);`,
		now, now, now, now, now); err != nil {
		t.Fatalf("insert Live TV fixture: %v", err)
	}
	if _, err := server.updateLiveTVChannelStateForUser(context.Background(), primary, "channel_profile_live", LiveTVChannelStateRequest{Favorite: boolPtr(true)}); err != nil {
		t.Fatalf("favorite primary channel: %v", err)
	}
	if _, err := server.updateLiveTVChannelStateForUser(context.Background(), child, "channel_profile_live", LiveTVChannelStateRequest{Hidden: boolPtr(true)}); err != nil {
		t.Fatalf("hide child channel: %v", err)
	}
	primaryChannel, err := server.getLiveTVChannelForProfileContext(context.Background(), primary.ProfileID, "channel_profile_live")
	if err != nil || !primaryChannel.Favorite || primaryChannel.Hidden {
		t.Fatalf("primary channel state = %#v, err=%v", primaryChannel, err)
	}
	childChannel, err := server.getLiveTVChannelForProfileContext(context.Background(), child.ProfileID, "channel_profile_live")
	if err != nil || childChannel.Favorite || !childChannel.Hidden {
		t.Fatalf("child channel state = %#v, err=%v", childChannel, err)
	}
	primaryCtx := withLiveTVViewerProfile(context.Background(), primary.ProfileID)
	primaryRows, _, _, err := server.listLiveTVChannelsForSourcePageFilteredContext(primaryCtx, "src_profile_live", 10, 0, UserChannelPolicy{}, false, liveTVChannelBrowseFilter{FavoritesOnly: true})
	if err != nil || len(primaryRows) != 1 {
		t.Fatalf("primary favorites = %#v, err=%v", primaryRows, err)
	}
	childCtx := withLiveTVViewerProfile(context.Background(), child.ProfileID)
	childRows, _, _, err := server.listLiveTVChannelsForSourcePageFilteredContext(childCtx, "src_profile_live", 10, 0, UserChannelPolicy{}, false, liveTVChannelBrowseFilter{})
	if err != nil || len(childRows) != 0 {
		t.Fatalf("child hidden browse = %#v, err=%v", childRows, err)
	}
	primarySources, err := server.listLiveTVSourcesForProfile(primary.ProfileID, false)
	if err != nil || len(primarySources) != 1 || primarySources[0].FavoriteChannelCount != 1 || primarySources[0].HiddenChannelCount != 0 {
		t.Fatalf("primary source summary = %#v, err=%v", primarySources, err)
	}
	childSources, err := server.listLiveTVSourcesForProfile(child.ProfileID, false)
	if err != nil || len(childSources) != 1 || childSources[0].FavoriteChannelCount != 0 || childSources[0].HiddenChannelCount != 1 {
		t.Fatalf("child source summary = %#v, err=%v", childSources, err)
	}
	var globalFavorite, globalHidden int
	if err := server.db.QueryRow(`SELECT favorite, hidden FROM live_tv_channels WHERE id = 'channel_profile_live'`).Scan(&globalFavorite, &globalHidden); err != nil {
		t.Fatalf("load catalog flags: %v", err)
	}
	if globalFavorite != 0 || globalHidden != 0 {
		t.Fatalf("viewer state leaked into global channel catalog: favorite=%d hidden=%d", globalFavorite, globalHidden)
	}
	if _, err := server.db.Exec(`UPDATE live_tv_sources SET favorite_channel_count = 7, hidden_channel_count = 9 WHERE id = 'src_profile_live'`); err != nil {
		t.Fatalf("seed legacy source summaries: %v", err)
	}
	neutralSources, err := server.listLiveTVSourcesForProfile("", false)
	if err != nil || len(neutralSources) != 1 || neutralSources[0].FavoriteChannelCount != 0 || neutralSources[0].HiddenChannelCount != 0 {
		t.Fatalf("missing profile identity did not fail closed with neutral summaries: %#v, err=%v", neutralSources, err)
	}
}

func TestProfileOwnedViewerTablesRejectAccountProfileMismatches(t *testing.T) {
	server := newScannerTestServer(t)
	first := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_profile_integrity_two', 'profile-integrity-two', 'profile-integrity-two@example.test', 'Second Account', 'hash', 'user', '{}', '{}', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert second account: %v", err)
	}
	second := User{ID: "usr_profile_integrity_two", AccountID: "usr_profile_integrity_two", ProfileID: "usr_profile_integrity_two"}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_profile_integrity', 'Profile Integrity', 'm3u', 1, ?, ?);
		INSERT INTO live_tv_channels (
			id, source_id, name, stream_url, enabled, last_seen_at, created_at, updated_at
		) VALUES ('channel_profile_integrity', 'src_profile_integrity', 'Integrity', 'https://example.test/integrity.m3u8', 1, ?, ?, ?);`,
		now, now, now, now, now); err != nil {
		t.Fatalf("seed profile integrity fixtures: %v", err)
	}
	cases := []struct {
		name  string
		query string
		args  []any
	}{
		{
			name: "DVR rule",
			query: `INSERT INTO live_tv_recording_rules (
				id, user_id, profile_id, source_id, title, created_at, updated_at
			) VALUES ('rule_profile_integrity', ?, ?, 'src_profile_integrity', 'Integrity', ?, ?)`,
			args: []any{accountIDForUser(first), viewerProfileID(second), now, now},
		},
		{
			name: "Live TV state",
			query: `INSERT INTO live_tv_channel_profile_state (
				profile_id, user_id, channel_id, favorite, created_at, updated_at
			) VALUES (?, ?, 'channel_profile_integrity', 1, ?, ?)`,
			args: []any{viewerProfileID(second), accountIDForUser(first), now, now},
		},
		{
			name: "Watch With Friends group",
			query: `INSERT INTO watch_with_friends_groups (
				id, owner_user_id, owner_profile_id, media_id, created_at, updated_at
			) VALUES ('wwf_profile_integrity', ?, ?, 'media_profile_integrity', ?, ?)`,
			args: []any{accountIDForUser(first), viewerProfileID(second), now, now},
		},
	}
	for _, testCase := range cases {
		if _, err := server.db.Exec(testCase.query, testCase.args...); err == nil || !strings.Contains(err.Error(), "profile does not belong to account") {
			t.Fatalf("%s accepted mismatched account/profile ownership: %v", testCase.name, err)
		}
	}
}

func TestActiveProfileMediaProjectionAcrossSavedDiscoveryAndPlaybackQueue(t *testing.T) {
	server := newScannerTestServer(t)
	primary, child := profileIsolationUsers(t, server)
	ctx := context.Background()
	for _, mediaID := range []string{"movie_neon", "movie_meridian"} {
		if err := server.upsertState(primary.ProfileID, mediaID, "favorite", 1); err != nil {
			t.Fatalf("favorite %s for primary profile: %v", mediaID, err)
		}
		if err := server.upsertState(child.ProfileID, mediaID, "watchlisted", 1); err != nil {
			t.Fatalf("watchlist %s for child profile: %v", mediaID, err)
		}
	}

	probe, probeErr := server.mediaListItemsByOrderedIDsContext(ctx, child.ProfileID, []string{"movie_neon"})
	playlist, err := server.createCanonicalSavedResource(ctx, child, "playlist", "Child Queue", "", "private", nil, []string{"movie_neon"})
	if err != nil {
		t.Fatalf("create child saved playlist: %v (projection=%#v projectionErr=%v)", err, probe, probeErr)
	}
	page, err := server.savedPlaylistEntriesPage(ctx, child, playlist.ID, "", 20, time.Now().UTC())
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("load child saved playlist entries: %#v, err=%v", page, err)
	}
	savedState := page.Items[0].Media.UserState
	if !savedState.Watchlisted || savedState.Favorite {
		t.Fatalf("saved entry projected primary profile state instead of child state: %#v", savedState)
	}

	discovery := server.trendingRowContext(ctx, child)
	foundChildState := false
	for _, item := range discovery.Items {
		if item.ID != "movie_neon" {
			continue
		}
		foundChildState = true
		if !item.State.Watchlisted || item.State.Favorite {
			t.Fatalf("discovery row projected primary profile state instead of child state: %#v", item.State)
		}
	}
	if !foundChildState {
		t.Fatalf("discovery row did not contain the seeded child-state item: %#v", discovery.Items)
	}
	if homeCacheKey(primary) == homeCacheKey(child) {
		t.Fatalf("same-account profiles shared a home cache key")
	}

	current, err := server.getMediaListItemContext(ctx, child.ProfileID, "movie_neon")
	if err != nil {
		t.Fatalf("load child playback item: %v", err)
	}
	queue := server.playbackQueueContext(ctx, child.ProfileID, current, current, []string{"movie_neon", "movie_meridian"})
	if len(queue) != 1 || queue[0].ID != "movie_meridian" || !queue[0].State.Watchlisted || queue[0].State.Favorite {
		t.Fatalf("playback queue projected primary profile state instead of child state: %#v", queue)
	}

	maximumAge := 0
	restrictions, err := json.Marshal(ProfileRestrictions{
		Version: "v1", MaximumAgeRating: &maximumAge, AllowUnrated: false,
		BlockedLabels: []string{}, AllowDownloads: true, AllowLiveTV: true,
		AllowDVR: true, AllowWatchWithFriends: true, AllowFeedback: true,
	})
	if err != nil {
		t.Fatalf("marshal child restrictions: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE profiles SET restrictions_json = ?, policy_updated_at = ? WHERE id = ?`, string(restrictions), time.Now().UTC().Format(time.RFC3339), child.ProfileID); err != nil {
		t.Fatalf("apply child restrictions: %v", err)
	}
	childRestrictedQueue := server.playbackQueueContext(ctx, child.ProfileID, current, current, []string{"movie_neon", "movie_meridian"})
	if len(childRestrictedQueue) != 0 {
		t.Fatalf("child playback queue ignored active profile restrictions: %#v", childRestrictedQueue)
	}
	primaryCurrent, err := server.getMediaListItemContext(ctx, primary.ProfileID, "movie_neon")
	if err != nil {
		t.Fatalf("load primary playback item: %v", err)
	}
	primaryQueue := server.playbackQueueContext(ctx, primary.ProfileID, primaryCurrent, primaryCurrent, []string{"movie_neon", "movie_meridian"})
	if len(primaryQueue) != 1 || primaryQueue[0].ID != "movie_meridian" {
		t.Fatalf("child restrictions leaked into primary playback queue: %#v", primaryQueue)
	}
}
