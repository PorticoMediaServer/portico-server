package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestSavedMediaKeysetDoesNotShiftWhenNewerItemIsInserted(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	for index := 0; index < 4; index++ {
		id := fmt.Sprintf("movie_saved_keyset_%d", index)
		updated := time.Date(2030, 1, 1, 12-index, 0, 0, 0, time.UTC).Format(time.RFC3339)
		if _, err := db.Exec(`INSERT INTO media_items (id, library_id, type, title, sort_title, added_at) VALUES (?, 'lib_movies', 'movie', ?, ?, ?)`, id, id, id, updated); err != nil {
			t.Fatalf("insert saved media %s: %v", id, err)
		}
		if _, err := db.Exec(`INSERT INTO user_media_state (profile_id, user_id, media_id, watchlisted, updated_at) VALUES (?, ?, ?, 1, ?)`, userID, userID, id, updated); err != nil {
			t.Fatalf("insert saved state %s: %v", id, err)
		}
	}
	first, hasMore, _, sortMode, err := server.listSavedMediaStateKeysetPageContext(context.Background(), userID, "watchlisted", "all", "updated", "desc", 2, savedMediaCursor{})
	if err != nil || !hasMore || len(first) != 2 {
		t.Fatalf("first saved page err=%v hasMore=%v items=%#v", err, hasMore, first)
	}
	after, err := server.savedMediaCursorForItem(context.Background(), userID, first[len(first)-1].ID, sortMode)
	if err != nil {
		t.Fatalf("saved page cursor: %v", err)
	}
	insertedAt := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO media_items (id, library_id, type, title, sort_title, added_at) VALUES ('movie_saved_keyset_new', 'lib_movies', 'movie', 'New before cursor', 'New before cursor', ?)`, insertedAt); err != nil {
		t.Fatalf("insert newer saved media: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_media_state (profile_id, user_id, media_id, watchlisted, updated_at) VALUES (?, ?, 'movie_saved_keyset_new', 1, ?)`, userID, userID, insertedAt); err != nil {
		t.Fatalf("insert newer saved state: %v", err)
	}
	second, _, _, _, err := server.listSavedMediaStateKeysetPageContext(context.Background(), userID, "watchlisted", "all", "updated", "desc", 10, after)
	if err != nil {
		t.Fatalf("second saved page: %v", err)
	}
	assertNoCollectionPageOverlap(t, keysetMediaItemIDs(first), keysetMediaItemIDs(second), "movie_saved_keyset_new")
}

func TestLiveTVChannelKeysetDoesNotShiftWhenEarlierChannelIsInserted(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at) VALUES ('src_channel_keyset', 'Keyset', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	for index := 0; index < 4; index++ {
		id := fmt.Sprintf("channel_keyset_%d", index)
		if _, err := server.db.Exec(`INSERT INTO live_tv_channels (id, source_id, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at) VALUES (?, 'src_channel_keyset', ?, ?, 1, ?, ?, ?, ?)`, id, id, "https://example.test/"+id, index, now, now, now); err != nil {
			t.Fatalf("insert channel %s: %v", id, err)
		}
	}
	first, _, hasMore, err := server.listLiveTVChannelsForSourceKeysetPageFilteredContext(context.Background(), "src_channel_keyset", 2, liveTVChannelCursor{}, UserChannelPolicy{}, false, liveTVChannelBrowseFilter{})
	if err != nil || !hasMore || len(first) != 2 {
		t.Fatalf("first channel page err=%v hasMore=%v items=%#v", err, hasMore, first)
	}
	last := first[len(first)-1]
	after := liveTVChannelCursor{PrimaryNumber: last.SortOrder, SortOrder: last.SortOrder, Name: last.Name, ID: last.ID}
	from := time.Now().UTC().Add(-time.Hour)
	to := from.Add(2 * time.Hour)
	guideFirst, _, guideHasMore, err := server.listLiveTVGuideChannelsKeysetPageFilteredContext(context.Background(), "src_channel_keyset", 2, liveTVChannelCursor{}, UserChannelPolicy{}, false, from, to, "", "", "recent", "asc", "")
	if err != nil || !guideHasMore || len(guideFirst) != 2 {
		t.Fatalf("first guide channel page err=%v hasMore=%v items=%#v", err, guideHasMore, guideFirst)
	}
	guideLast := guideFirst[len(guideFirst)-1]
	guideAfter := liveTVChannelCursor{PrimaryNumber: guideLast.SortOrder, SortOrder: guideLast.SortOrder, Name: guideLast.Name, ID: guideLast.ID}
	if _, err := server.db.Exec(`INSERT INTO live_tv_channels (id, source_id, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at) VALUES ('channel_keyset_new', 'src_channel_keyset', 'Earlier', 'https://example.test/new', 1, -1, ?, ?, ?)`, now, now, now); err != nil {
		t.Fatalf("insert earlier channel: %v", err)
	}
	second, _, _, err := server.listLiveTVChannelsForSourceKeysetPageFilteredContext(context.Background(), "src_channel_keyset", 10, after, UserChannelPolicy{}, false, liveTVChannelBrowseFilter{})
	if err != nil {
		t.Fatalf("second channel page: %v", err)
	}
	assertNoCollectionPageOverlap(t, liveTVChannelIDs(first), liveTVChannelIDs(second), "channel_keyset_new")
	guideSecond, _, _, err := server.listLiveTVGuideChannelsKeysetPageFilteredContext(context.Background(), "src_channel_keyset", 10, guideAfter, UserChannelPolicy{}, false, from, to, "", "", "recent", "asc", "")
	if err != nil {
		t.Fatalf("second guide channel page: %v", err)
	}
	assertNoCollectionPageOverlap(t, liveTVChannelIDs(guideFirst), liveTVChannelIDs(guideSecond), "channel_keyset_new")
}

func TestDVRKeysetsDoNotShiftWhenRowsAreInsertedBeforeCursor(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	user := dvrTestUser(t, server)
	userID := accountIDForUser(user)
	now := time.Date(2030, 2, 1, 12, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at) VALUES ('src_dvr_keyset', 'DVR Keyset', 'm3u', 1, ?, ?)`, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert DVR source: %v", err)
	}
	for index := 0; index < 4; index++ {
		stamp := now.Add(-time.Duration(index) * time.Hour).Format(time.RFC3339)
		if _, err := db.Exec(`INSERT INTO live_tv_recording_rules (id, user_id, source_id, title, match_type, retention_days, enabled, created_at, updated_at) VALUES (?, ?, 'src_dvr_keyset', ?, 'single', 30, 1, ?, ?)`, fmt.Sprintf("rule_keyset_%d", index), userID, fmt.Sprintf("Rule %d", index), stamp, stamp); err != nil {
			t.Fatalf("insert DVR rule %d: %v", index, err)
		}
		if _, err := db.Exec(`INSERT INTO live_tv_recordings (id, user_id, source_id, title, folder, status, starts_at, ends_at, created_at, updated_at) VALUES (?, ?, 'src_dvr_keyset', ?, 'Keyset', 'complete', ?, ?, ?, ?)`, fmt.Sprintf("recording_keyset_%d", index), userID, fmt.Sprintf("Recording %d", index), stamp, now.Add(time.Hour).Format(time.RFC3339), stamp, stamp); err != nil {
			t.Fatalf("insert DVR recording %d: %v", index, err)
		}
		scheduledAt := now.Add(time.Duration(index) * time.Hour).Format(time.RFC3339)
		if _, err := db.Exec(`INSERT INTO live_tv_recordings (id, user_id, source_id, title, folder, status, starts_at, ends_at, created_at, updated_at) VALUES (?, ?, 'src_dvr_keyset', ?, 'Schedule', 'scheduled', ?, ?, ?, ?)`, fmt.Sprintf("schedule_keyset_%d", index), userID, fmt.Sprintf("Schedule %d", index), scheduledAt, now.Add(6*time.Hour).Format(time.RFC3339), stamp, stamp); err != nil {
			t.Fatalf("insert DVR schedule %d: %v", index, err)
		}
	}
	rules, _, hasMore, err := server.listDVRRulesKeysetPageForUser(context.Background(), user, 2, dvrRuleCursor{}, "none")
	if err != nil || !hasMore || len(rules) != 2 {
		t.Fatalf("first rules page err=%v hasMore=%v items=%#v", err, hasMore, rules)
	}
	ruleAfter := dvrRuleCursor{UpdatedAt: rules[1].UpdatedAt, ID: rules[1].ID}
	recordings, _, hasMore, err := server.listDVRRecordingsKeysetPageForUser(context.Background(), user, 2, dvrRecordingCursor{}, "none")
	if err != nil || !hasMore || len(recordings) != 2 {
		t.Fatalf("first recordings page err=%v hasMore=%v items=%#v", err, hasMore, recordings)
	}
	recordingAfter := dvrRecordingCursor{StartsAt: recordings[1].StartsAt, ID: recordings[1].ID}
	schedule, _, hasMore, err := server.listDVRScheduleKeysetPageForUser(context.Background(), user, 2, dvrRecordingCursor{})
	if err != nil || !hasMore || len(schedule) != 2 {
		t.Fatalf("first schedule page err=%v hasMore=%v items=%#v", err, hasMore, schedule)
	}
	scheduleAfter := dvrRecordingCursor{StartsAt: schedule[1].StartsAt, ID: schedule[1].ID}
	groups, _, hasMore, err := server.listDVRRecordingGroupsKeysetPageForUser(context.Background(), user, 2, dvrRecordingGroupCursor{})
	if err != nil || !hasMore || len(groups) != 2 {
		t.Fatalf("first recording groups page err=%v hasMore=%v items=%#v", err, hasMore, groups)
	}
	groupAfter := dvrRecordingGroupCursor{LatestRecordingAt: groups[1].LatestRecordingAt, Title: groups[1].Title, Folder: groups[1].CursorFolder}
	newer := now.Add(10 * time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO live_tv_recording_rules (id, user_id, source_id, title, match_type, retention_days, enabled, created_at, updated_at) VALUES ('rule_keyset_new', ?, 'src_dvr_keyset', 'New Rule', 'single', 30, 1, ?, ?)`, userID, newer, newer); err != nil {
		t.Fatalf("insert newer DVR rule: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO live_tv_recordings (id, user_id, source_id, title, folder, status, starts_at, ends_at, created_at, updated_at) VALUES ('recording_keyset_new', ?, 'src_dvr_keyset', 'New Recording', 'Keyset', 'complete', ?, ?, ?, ?)`, userID, newer, newer, newer, newer); err != nil {
		t.Fatalf("insert newer DVR recording: %v", err)
	}
	earlier := now.Add(-time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO live_tv_recordings (id, user_id, source_id, title, folder, status, starts_at, ends_at, created_at, updated_at) VALUES ('schedule_keyset_new', ?, 'src_dvr_keyset', 'Earlier Schedule', 'Schedule', 'scheduled', ?, ?, ?, ?)`, userID, earlier, newer, newer, newer); err != nil {
		t.Fatalf("insert earlier DVR schedule: %v", err)
	}
	nextRules, _, _, err := server.listDVRRulesKeysetPageForUser(context.Background(), user, 10, ruleAfter, "none")
	if err != nil {
		t.Fatalf("second rules page: %v", err)
	}
	nextRecordings, _, _, err := server.listDVRRecordingsKeysetPageForUser(context.Background(), user, 10, recordingAfter, "none")
	if err != nil {
		t.Fatalf("second recordings page: %v", err)
	}
	assertNoCollectionPageOverlap(t, dvrRuleIDs(rules), dvrRuleIDs(nextRules), "rule_keyset_new")
	assertNoCollectionPageOverlap(t, dvrRecordingIDs(recordings), dvrRecordingIDs(nextRecordings), "recording_keyset_new")
	nextSchedule, _, _, err := server.listDVRScheduleKeysetPageForUser(context.Background(), user, 10, scheduleAfter)
	if err != nil {
		t.Fatalf("second schedule page: %v", err)
	}
	assertNoCollectionPageOverlap(t, dvrRecordingIDs(schedule), dvrRecordingIDs(nextSchedule), "schedule_keyset_new")
	nextGroups, _, _, err := server.listDVRRecordingGroupsKeysetPageForUser(context.Background(), user, 10, groupAfter)
	if err != nil {
		t.Fatalf("second recording groups page: %v", err)
	}
	newGroupID := "recgrp_" + safePathComponent(sortableTitle("Keyset New Recording"))
	assertNoCollectionPageOverlap(t, dvrRecordingGroupIDs(groups), dvrRecordingGroupIDs(nextGroups), newGroupID)
}

func assertNoCollectionPageOverlap(t *testing.T, first, second []string, insertedBefore string) {
	t.Helper()
	seen := map[string]bool{}
	for _, id := range first {
		seen[id] = true
	}
	for _, id := range second {
		if seen[id] {
			t.Fatalf("item %s was duplicated across keyset pages", id)
		}
		if id == insertedBefore {
			t.Fatalf("item inserted before the cursor leaked into the next page: %s", id)
		}
	}
}

func keysetMediaItemIDs(items []MediaItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func dvrRuleIDs(items []DVRRecordingRule) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func dvrRecordingIDs(items []DVRRecording) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func dvrRecordingGroupIDs(items []DVRRecordingGroup) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}
