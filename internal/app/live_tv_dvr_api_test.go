package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveTVChannelPageFiltersAndReturnsAccessibleGroups(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_browse', 'Browse Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	channels := []struct {
		id       string
		name     string
		group    string
		favorite int
		hidden   int
	}{
		{"channel_news_alpha", "Alpha News", "News", 1, 0},
		{"channel_news_beta", "Beta News", "News", 0, 0},
		{"channel_sports_alpha", "Alpha Sports", "Sports", 1, 0},
		{"channel_hidden", "Alpha Hidden", "Hidden", 1, 1},
	}
	for index, channel := range channels {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, group_title, enabled, favorite, hidden, sort_order, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_browse', ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?)`,
			channel.id, index+1, channel.name, "https://example.test/"+channel.id, channel.group, channel.favorite, channel.hidden, index, now, now, now); err != nil {
			t.Fatalf("insert channel %s: %v", channel.id, err)
		}
	}

	user := User{ID: "usr_channel_browser", ProfileID: "usr_channel_browser", ProfileIsPrimary: true, Permissions: map[string]bool{"viewLiveTV": true}}
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES (?, 'channel-browser', 'channel-browser@example.test', 'Channel Browser', 'hash', 'user', '{}', '{}', ?, ?)`, user.ID, now, now); err != nil {
		t.Fatalf("insert channel browser account: %v", err)
	}
	for _, channel := range channels {
		if channel.favorite == 0 && channel.hidden == 0 {
			continue
		}
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channel_profile_state (profile_id, user_id, channel_id, favorite, hidden, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, user.ProfileID, user.ID, channel.id, channel.favorite, channel.hidden, now, now); err != nil {
			t.Fatalf("insert channel state %s: %v", channel.id, err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/live-tv/sources/src_browse/channels?query=alpha&favoritesOnly=true&group=News&limit=20", nil)
	recorder := httptest.NewRecorder()
	server.handleLiveTVSourceRoute(recorder, req, user, []string{"src_browse", "channels"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("channel page status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response LiveTVChannelPageResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode channel page: %v", err)
	}
	if len(response.Items) != 1 || response.Items[0].ID != "channel_news_alpha" {
		t.Fatalf("filtered channels = %#v", response.Items)
	}
	if strings.Join(response.Groups, ",") != "News,Sports" {
		t.Fatalf("accessible groups = %#v", response.Groups)
	}
	if response.PageInfo.HasMore || response.PageInfo.NextCursor != nil {
		t.Fatalf("unexpected pagination = %#v", response)
	}
}

func TestLiveTVChannelPageSerializesEmptyItemsAsArray(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_empty_filter', 'Empty Filter Source', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, sort_order, last_seen_at, created_at, updated_at)
		VALUES ('channel_present', 'src_empty_filter', '1', 'Present Channel', 'https://example.test/present', 1, 1, ?, ?, ?)`, now, now, now); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	user := User{ID: "usr_empty_filter", ProfileID: "usr_empty_filter", ProfileIsPrimary: true, Permissions: map[string]bool{"viewLiveTV": true}}
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES (?, 'empty-filter', 'empty-filter@example.test', 'Empty Filter', 'hash', 'user', '{}', '{}', ?, ?)`, user.ID, now, now); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live-tv/sources/src_empty_filter/channels?query=no-such-channel", nil)
	recorder := httptest.NewRecorder()
	server.handleLiveTVSourceRoute(recorder, req, user, []string{"src_empty_filter", "channels"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("channel page status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode channel page: %v", err)
	}
	if string(body["items"]) != "[]" {
		t.Fatalf("items must be an empty JSON array, body=%s", recorder.Body.String())
	}
}

func TestLiveTVGuideGroupFilterKeepsCompleteGroupNavigation(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_guide_groups', 'Grouped Guide', 'm3u', 1, ?, ?)`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	for index, group := range []string{"News", "Sports"} {
		channelID := "channel_group_" + strings.ToLower(group)
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, group_title, enabled, sort_order, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_guide_groups', ?, ?, ?, ?, 1, ?, ?, ?, ?)`,
			channelID, index+1, group+" Channel", "https://example.test/"+channelID, group, index, nowText, nowText, nowText); err != nil {
			t.Fatalf("insert %s channel: %v", group, err)
		}
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_programs (id, source_id, channel_id, channel_ref, title, start_at, end_at, created_at)
			VALUES (?, 'src_guide_groups', ?, ?, ?, ?, ?, ?)`,
			"program_"+strings.ToLower(group), channelID, channelID, group+" Now", nowText, now.Add(time.Hour).Format(time.RFC3339), nowText); err != nil {
			t.Fatalf("insert %s program: %v", group, err)
		}
	}

	guide, err := server.liveTVGuideContextWithGroup(t.Context(), "", "src_guide_groups", false, nowText, "2", "20", "0", "", "", "name", "asc", "Sports")
	if err != nil {
		t.Fatalf("load grouped guide: %v", err)
	}
	if len(guide.Channels) != 1 || guide.Channels[0].GroupTitle != "Sports" || len(guide.Programs) != 1 {
		t.Fatalf("grouped guide = %#v", guide)
	}
	if strings.Join(guide.ChannelGroups, ",") != "News,Sports" {
		t.Fatalf("guide group navigation = %#v", guide.ChannelGroups)
	}
}

func TestDVROperationalStatusReportsRealGuideConflictTunerAndStorageState(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Truncate(time.Second)
	nowText := now.Format(time.RFC3339)
	ffmpegPath := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(ffmpegPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	server.cfg.FFmpegPath = ffmpegPath
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, refresh_interval_hours, last_refreshed_at, created_at, updated_at)
		VALUES ('src_status', 'Living Room Tuner', 'm3u', 1, 12, ?, ?, ?)`, nowText, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, enabled, last_seen_at, created_at, updated_at)
		VALUES ('channel_status', 'src_status', '7', 'News 7', 'https://example.test/news', 1, ?, ?, ?)`, nowText, nowText, nowText); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_programs (id, source_id, channel_id, channel_ref, title, start_at, end_at, created_at)
		VALUES ('program_status', 'src_status', 'channel_status', 'channel_status', 'News Now', ?, ?, ?)`, nowText, now.Add(time.Hour).Format(time.RFC3339), nowText); err != nil {
		t.Fatalf("insert program: %v", err)
	}
	for _, recording := range []struct {
		id     string
		title  string
		start  time.Time
		end    time.Time
		status string
		size   int64
	}{
		{"recording_status_one", "First News", now.Add(time.Hour), now.Add(2 * time.Hour), "scheduled", 0},
		{"recording_status_two", "Second News", now.Add(90 * time.Minute), now.Add(150 * time.Minute), "scheduled", 0},
		{"recording_status_complete", "Archived News", now.Add(-2 * time.Hour), now.Add(-time.Hour), "complete", 4096},
	} {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_recordings (id, user_id, source_id, channel_id, title, status, starts_at, ends_at, size_bytes, created_at, updated_at)
			VALUES (?, ?, 'src_status', 'channel_status', ?, ?, ?, ?, ?, ?, ?)`,
			recording.id, user.ID, recording.title, recording.status, recording.start.Format(time.RFC3339), recording.end.Format(time.RFC3339), recording.size, nowText, nowText); err != nil {
			t.Fatalf("insert recording %s: %v", recording.id, err)
		}
	}

	recorder := httptest.NewRecorder()
	server.handleAdminDVROperationalStatus(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/dvr/status?sourceId=src_status", nil), user)
	if recorder.Code != http.StatusOK {
		t.Fatalf("DVR status route = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var status DVROperationalStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode DVR status: %v", err)
	}
	if !status.Configured || !status.Available || status.Guide.State != "current" {
		t.Fatalf("DVR availability = %#v", status)
	}
	if len(status.Conflicts) != 1 || len(status.Conflicts[0].RecordingIDs) != 2 {
		t.Fatalf("DVR conflicts = %#v", status.Conflicts)
	}
	if len(status.Tuners) != 1 || status.Tuners[0].State != "conflict" || status.Tuners[0].Name != "Living Room Tuner" {
		t.Fatalf("DVR tuners = %#v", status.Tuners)
	}
	if status.Storage.UsedBytes != 4096 || status.Storage.AvailableBytes <= 0 {
		t.Fatalf("DVR storage = %#v", status.Storage)
	}
	if !status.Capabilities.CanScheduleRecordings || !status.Capabilities.CanManageRecordingRules {
		t.Fatalf("DVR capabilities = %#v", status.Capabilities)
	}
}

func TestDVRRecordingConflictResponseIsStructuredAndUsesConflictStatus(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Truncate(time.Second)
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_conflict_api', 'Conflict Source', 'm3u', 1, ?, ?)`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_conflict_api", "channel_conflict_api", nowText)
	if _, err := server.createDVRRecording(user, DVRRecordingRequest{
		SourceID:  "src_conflict_api",
		ChannelID: "channel_conflict_api",
		Title:     "Evening News",
		StartsAt:  now.Add(time.Hour).Format(time.RFC3339),
		EndsAt:    now.Add(2 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("create first recording: %v", err)
	}
	body, err := json.Marshal(DVRRecordingRequest{
		SourceID:  "src_conflict_api",
		ChannelID: "channel_conflict_api",
		Title:     "Weather",
		StartsAt:  now.Add(90 * time.Minute).Format(time.RFC3339),
		EndsAt:    now.Add(150 * time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	recorder := performDVRRouteRequest(server, user, http.MethodPost, "/api/dvr/recordings", string(body))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var problem map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode conflict problem: %v", err)
	}
	if problem["code"] != "dvr_schedule_conflict" || problem["status"] != float64(http.StatusConflict) {
		t.Fatalf("conflict problem = %#v", problem)
	}
	conflict, ok := problem["conflict"].(map[string]any)
	if !ok || conflict["reason"] != "source_recording_overlap" || conflict["requestedStartsAt"] == "" {
		t.Fatalf("conflict extension = %#v", problem["conflict"])
	}
}

func TestDVRRecordingConflictResponseDoesNotExposeAnotherUsersRecording(t *testing.T) {
	server := newScannerTestServer(t)
	owner := dvrTestUser(t, server)
	now := time.Now().UTC().Truncate(time.Second)
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_private_conflict', 'Shared Tuner', 'm3u', 1, ?, ?)`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_private_conflict", "channel_private_conflict", nowText)
	if _, err := server.createDVRRecording(owner, DVRRecordingRequest{
		SourceID:  "src_private_conflict",
		ChannelID: "channel_private_conflict",
		Title:     "Private Program Title",
		StartsAt:  now.Add(time.Hour).Format(time.RFC3339),
		EndsAt:    now.Add(2 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("create owner's recording: %v", err)
	}
	schedulerPermissions := map[string]bool{"viewDVR": true, "scheduleDVR": true}
	encodedSchedulerPermissions, err := json.Marshal(schedulerPermissions)
	if err != nil {
		t.Fatalf("encode scheduler permissions: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_scheduler', 'scheduler', 'scheduler@example.test', 'Scheduler', 'hash', 'user', ?, '{}', ?, ?)`,
		string(encodedSchedulerPermissions), nowText, nowText); err != nil {
		t.Fatalf("insert scheduler: %v", err)
	}
	scheduler := User{ID: "usr_scheduler", AccountID: "usr_scheduler", ProfileID: "usr_scheduler", ProfileIsPrimary: true, Permissions: schedulerPermissions}
	body, err := json.Marshal(DVRRecordingRequest{
		SourceID:  "src_private_conflict",
		ChannelID: "channel_private_conflict",
		Title:     "Scheduler Program",
		StartsAt:  now.Add(90 * time.Minute).Format(time.RFC3339),
		EndsAt:    now.Add(150 * time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	recorder := performDVRRouteRequest(server, scheduler, http.MethodPost, "/api/dvr/recordings", string(body))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "Private Program Title") || strings.Contains(recorder.Body.String(), "recordingId") {
		t.Fatalf("cross-user conflict leaked private recording metadata: %s", recorder.Body.String())
	}
}
