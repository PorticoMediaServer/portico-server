package app

import (
	"database/sql"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestShowDetailExposesUserScopedPlaybackTarget(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	profileID := primaryProfileIDForAccount(t, db, userID)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_show_target', 'Show Target', 'tv', 991, '/tmp/show-target', '{}', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	insert := func(id, parentID, mediaType, title string, index, season, episode int) {
		t.Helper()
		var parent any
		if parentID != "" {
			parent = parentID
		}
		if _, err := db.Exec(`
			INSERT INTO media_items (
				id, library_id, parent_id, type, title, sort_title, duration_seconds,
				genres_json, tags_json, labels_json, added_at, typed_metadata_json,
				index_number, season_number, episode_number
			) VALUES (?, 'lib_show_target', ?, ?, ?, ?, 2880, '[]', '[]', '[]', ?, '{}', ?, ?, ?)`,
			id, parent, mediaType, title, title, now, index, season, episode); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("show_target", "", "show", "Fargo", 0, 0, 0)
	insert("season_target_2", "show_target", "season", "Season 2", 2, 2, 0)
	insert("episode_target_8", "season_target_2", "episode", "Loplop", 8, 2, 8)
	insert("episode_target_9", "season_target_2", "episode", "The Castle", 9, 2, 9)
	if _, err := db.Exec(`
		UPDATE media_items
		SET summary = 'Peggy and Ed agree to follow through with their plan.', tagline = 'The motel closes in.'
		WHERE id = 'episode_target_9'`); err != nil {
		t.Fatalf("update episode narrative: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_media_state (user_id, profile_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES (?, ?, 'episode_target_8', 1, 2880, ?, ?), (?, ?, 'episode_target_9', 0, 330, ?, ?)`,
		userID, profileID, now, now, userID, profileID, now, now); err != nil {
		t.Fatalf("insert progress: %v", err)
	}

	detail, err := server.getMediaDetailWithOptions(profileID, "show_target", mediaDetailOptions{})
	if err != nil {
		t.Fatalf("load show detail: %v", err)
	}
	if detail.PlaybackTarget == nil {
		t.Fatal("show detail omitted playbackTarget")
	}
	if detail.PlaybackTarget.ID != "episode_target_9" {
		t.Fatalf("playback target = %q, expected episode_target_9", detail.PlaybackTarget.ID)
	}
	if detail.PlaybackTarget.SeasonNumber != 2 || detail.PlaybackTarget.EpisodeNumber != 9 {
		t.Fatalf("playback target numbering = S%dE%d", detail.PlaybackTarget.SeasonNumber, detail.PlaybackTarget.EpisodeNumber)
	}
	if detail.PlaybackTarget.State.ProgressSeconds != 330 {
		t.Fatalf("playback target progress = %d", detail.PlaybackTarget.State.ProgressSeconds)
	}
	if detail.PlaybackTarget.Summary != "Peggy and Ed agree to follow through with their plan." {
		t.Fatalf("playback target summary = %q", detail.PlaybackTarget.Summary)
	}
	if len(detail.Children) != 1 || len(detail.Children[0].Children) != 2 {
		t.Fatalf("show hierarchy = %#v", detail.Children)
	}
	if detail.Children[0].Children[1].Summary != detail.PlaybackTarget.Summary || detail.Children[0].Children[1].Tagline != "The motel closes in." {
		t.Fatalf("episode child narrative = %+v", detail.Children[0].Children[1])
	}
	if !slices.Contains(detail.PlaybackTarget.Actions, mediaActionPlay) {
		t.Fatalf("playback target actions = %#v", detail.PlaybackTarget.Actions)
	}
}

func TestShowPlaybackTargetRequiresAdvertisedPlayCapability(t *testing.T) {
	target := mediaContainerPlaybackTarget(MediaItem{Type: "show"}, []MediaItem{{
		ID:   "season_without_play",
		Type: "season",
		Children: []MediaItem{{
			ID:      "episode_without_play",
			Type:    "episode",
			Actions: []string{mediaActionWatchlistAdd},
		}},
	}})
	if target != nil {
		t.Fatalf("target without play capability = %#v", target)
	}
}

func TestShowDetailPlaybackTargetSearchesBeyondInlineEpisodePreview(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	userID := adminUserID(t, db)
	profileID := primaryProfileIDForAccount(t, db, userID)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_deep_show_target', 'Deep Show Target', 'tv', 992, '/tmp/deep-show-target', '{}', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, tags_json, labels_json, added_at, typed_metadata_json)
		VALUES ('show_deep_target', 'lib_deep_show_target', 'show', 'Deep Target', 'Deep Target', '[]', '[]', '[]', ?, '{}'),
		       ('season_deep_target', 'lib_deep_show_target', 'season', 'Season 1', 'Season 1', '[]', '[]', '[]', ?, '{}')`, now, now); err != nil {
		t.Fatalf("insert show hierarchy: %v", err)
	}
	if _, err := db.Exec(`UPDATE media_items SET parent_id = 'show_deep_target', season_number = 1, index_number = 1 WHERE id = 'season_deep_target'`); err != nil {
		t.Fatalf("attach season: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin episode insert: %v", err)
	}
	statement, err := tx.Prepare(`
		INSERT INTO media_items (
			id, library_id, parent_id, type, title, sort_title, source_url, duration_seconds,
			genres_json, tags_json, labels_json, added_at, typed_metadata_json,
			index_number, season_number, episode_number
		) VALUES (?, 'lib_deep_show_target', 'season_deep_target', 'episode', ?, ?, ?, 1800, '[]', '[]', '[]', ?, '{}', ?, 1, ?)`)
	if err != nil {
		t.Fatalf("prepare episode insert: %v", err)
	}
	for episode := 1; episode <= 205; episode++ {
		id := fmt.Sprintf("episode_deep_target_%03d", episode)
		title := fmt.Sprintf("Episode %03d", episode)
		if _, err := statement.Exec(id, title, title, "https://media.example.test/"+id+".mp4", now, episode, episode); err != nil {
			_ = statement.Close()
			_ = tx.Rollback()
			t.Fatalf("insert episode %d: %v", episode, err)
		}
	}
	if err := statement.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatalf("close episode insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit episodes: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_media_state (user_id, profile_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES (?, ?, 'episode_deep_target_201', 0, 420, ?, ?)`, userID, profileID, now, now); err != nil {
		t.Fatalf("insert deep progress: %v", err)
	}

	detail, err := server.getMediaDetailWithOptions(profileID, "show_deep_target", mediaDetailOptions{})
	if err != nil {
		t.Fatalf("load deep show detail: %v", err)
	}
	if !detail.ChildrenTruncated {
		t.Fatal("deep show hierarchy should publish a bounded inline preview")
	}
	if detail.PlaybackTarget == nil || detail.PlaybackTarget.ID != "episode_deep_target_201" {
		t.Fatalf("playback target = %#v, expected episode 201 beyond the inline preview", detail.PlaybackTarget)
	}
	if detail.PlaybackTarget.State.ProgressSeconds != 420 {
		t.Fatalf("deep playback target progress = %d", detail.PlaybackTarget.State.ProgressSeconds)
	}
}

func primaryProfileIDForAccount(t *testing.T, db interface {
	QueryRow(query string, args ...any) *sql.Row
}, accountID string) string {
	t.Helper()
	var profileID string
	if err := db.QueryRow(`SELECT id FROM profiles WHERE account_id = ? AND is_primary = 1 AND disabled_at = ''`, accountID).Scan(&profileID); err != nil {
		t.Fatalf("load primary profile: %v", err)
	}
	return profileID
}
