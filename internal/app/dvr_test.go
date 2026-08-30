package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDVROutputPathStaysInsideAppData(t *testing.T) {
	server := newScannerTestServer(t)
	start := time.Date(2026, 5, 3, 1, 2, 3, 0, time.UTC)
	path, err := server.dvrOutputPath(DVRRecording{ID: "rec_test", Title: "../Night News", Folder: "../Sports/Finals"}, start)
	if err != nil {
		t.Fatalf("dvr output path: %v", err)
	}
	root := filepath.Join(server.cfg.AppDataDir, "recordings")
	if !strings.HasPrefix(path, root+string(filepath.Separator)) {
		t.Fatalf("recording path %s escaped root %s", path, root)
	}
	if !strings.HasSuffix(path, ".mp4") {
		t.Fatalf("recording path should be mp4: %s", path)
	}
	if !strings.Contains(path, string(filepath.Separator)+"SportsFinals"+string(filepath.Separator)) {
		t.Fatalf("recording path should include sanitized folder: %s", path)
	}
}

func TestPruneExpiredDVRRecordingsDeletesFilesAndRows(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, name, stream_url, enabled, last_seen_at, created_at, updated_at)
		VALUES ('channel_test', 'src_test', 'Test Channel', 'https://media.example.test/test.m3u8', 1, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText, nowText); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, created_at, updated_at)
		VALUES ('usr_test', 'tester', 'tester@example.test', 'Tester', 'hash', 'owner', '{}', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recording_rules (id, user_id, source_id, title, match_type, retention_days, created_at, updated_at)
		VALUES ('rule_test', 'usr_test', 'src_test', 'News', 'title', 1, ?, ?)`,
		nowText, nowText); err != nil {
		t.Fatalf("insert rule: %v", err)
	}
	recordingPath := filepath.Join(server.cfg.AppDataDir, "recordings", "2026", "05", "old-news.mp4")
	if err := os.MkdirAll(filepath.Dir(recordingPath), 0o700); err != nil {
		t.Fatalf("mkdir recording: %v", err)
	}
	if err := os.WriteFile(recordingPath, []byte("recording"), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recordings (id, rule_id, user_id, source_id, title, status, starts_at, ends_at, path, size_bytes, created_at, updated_at)
		VALUES ('rec_old', 'rule_test', 'usr_test', 'src_test', 'Old News', 'complete', ?, ?, ?, 9, ?, ?)`,
		now.Add(-72*time.Hour).Format(time.RFC3339), now.Add(-70*time.Hour).Format(time.RFC3339), recordingPath, nowText, nowText); err != nil {
		t.Fatalf("insert recording: %v", err)
	}
	if err := server.importDVRRecordingMedia(DVRRecording{
		ID:        "rec_old",
		Title:     "Old News",
		StartsAt:  now.Add(-72 * time.Hour).Format(time.RFC3339),
		EndsAt:    now.Add(-70 * time.Hour).Format(time.RFC3339),
		Path:      recordingPath,
		SizeBytes: 9,
	}, nowText); err != nil {
		t.Fatalf("import recording media: %v", err)
	}
	if _, err := server.deleteLiveTVSource("src_test"); !errors.Is(err, errLiveTVSourceHasRecordings) {
		t.Fatalf("source deletion with retained recording error = %v", err)
	}
	if _, err := os.Stat(recordingPath); err != nil {
		t.Fatalf("source deletion touched retained recording file: %v", err)
	}
	mediaID := dvrRecordingMediaID("rec_old")
	releasePlaybackUse := server.acquireDVRPlaybackUse("rec_old")
	releasePlaybackUse(true)
	removed, err := server.pruneExpiredDVRRecordings()
	if err != nil {
		t.Fatalf("prune recently streamed dvr recording: %v", err)
	}
	if removed != 0 {
		t.Fatalf("recently streamed recording was removed: %d", removed)
	}
	server.dvrPlaybackMu.Lock()
	server.dvrPlaybackLastSeen["rec_old"] = time.Now().UTC().Add(-dvrPlaybackRetentionGrace - time.Second)
	server.dvrPlaybackMu.Unlock()
	server.transcodes = map[string]*transcodeSession{"active-dvr": {mediaID: mediaID, done: make(chan struct{})}}
	removed, err = server.pruneExpiredDVRRecordings()
	if err != nil {
		t.Fatalf("prune active dvr recording: %v", err)
	}
	if removed != 0 {
		t.Fatalf("active recording was removed: %d", removed)
	}
	if _, err := os.Stat(recordingPath); err != nil {
		t.Fatalf("active recording file was removed: %v", err)
	}
	server.transcodes = map[string]*transcodeSession{}
	removed, err = server.pruneExpiredDVRRecordings()
	if err != nil {
		t.Fatalf("prune dvr recordings: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(recordingPath); !os.IsNotExist(err) {
		t.Fatalf("expected recording file to be removed, stat err=%v", err)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_recordings WHERE id = 'rec_old'`).Scan(&count); err != nil {
		t.Fatalf("count recording: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected recording row to be deleted")
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE id = ?`, dvrRecordingMediaID("rec_old")).Scan(&count); err != nil {
		t.Fatalf("count imported recording media: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected imported recording media to be deleted")
	}
}

func TestPruneDVRRecordingsHonorsSeriesCaps(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_cap', 'Cap Source', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recording_rules (id, user_id, source_id, title, match_type, retention_days, max_recordings_per_series, created_at, updated_at)
		VALUES ('rule_cap', ?, 'src_cap', 'Daily News', 'series', 365, 2, ?, ?)`,
		user.ID, nowText, nowText); err != nil {
		t.Fatalf("insert rule: %v", err)
	}
	for index, startOffset := range []time.Duration{-72 * time.Hour, -48 * time.Hour, -24 * time.Hour} {
		start := now.Add(startOffset).Truncate(time.Second)
		id := "rec_cap_" + strconv.Itoa(index+1)
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_recordings (id, rule_id, user_id, source_id, title, status, starts_at, ends_at, size_bytes, created_at, updated_at)
			VALUES (?, 'rule_cap', ?, 'src_cap', 'Daily News', 'complete', ?, ?, 10, ?, ?)`,
			id, user.ID, start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339), nowText, nowText); err != nil {
			t.Fatalf("insert recording %s: %v", id, err)
		}
	}
	removed, err := server.pruneExpiredDVRRecordings()
	if err != nil {
		t.Fatalf("prune capped dvr recordings: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_recordings WHERE rule_id = 'rule_cap'`).Scan(&count); err != nil {
		t.Fatalf("count capped recordings: %v", err)
	}
	if count != 2 {
		t.Fatalf("remaining recordings = %d, want 2", count)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_recordings WHERE id = 'rec_cap_1'`).Scan(&count); err != nil {
		t.Fatalf("count oldest recording: %v", err)
	}
	if count != 0 {
		t.Fatalf("oldest recording was not pruned")
	}
}

func TestImportDVRRecordingCreatesRecordedTVLibraryMediaAndSearch(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	recordingPath := filepath.Join(server.cfg.AppDataDir, "recordings", "2026", "05", "night-news.mp4")
	if err := os.MkdirAll(filepath.Dir(recordingPath), 0o700); err != nil {
		t.Fatalf("mkdir recording: %v", err)
	}
	if err := os.WriteFile(recordingPath, []byte("recording"), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}
	recording := DVRRecording{
		ID:        "rec_import",
		UserID:    user.ID,
		SourceID:  "src_test",
		Title:     "Night News",
		Status:    "complete",
		StartsAt:  now.Add(-time.Hour).Format(time.RFC3339),
		EndsAt:    now.Format(time.RFC3339),
		Path:      recordingPath,
		SizeBytes: 9,
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', 1, ?, ?)`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recordings (id, user_id, profile_id, source_id, title, status, starts_at, ends_at, path, size_bytes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, recording.ID, recording.UserID, viewerProfileID(user), recording.SourceID,
		recording.Title, recording.Status, recording.StartsAt, recording.EndsAt, recording.Path, recording.SizeBytes, nowText, nowText); err != nil {
		t.Fatalf("insert recording: %v", err)
	}
	if err := server.importDVRRecordingMedia(recording, nowText); err != nil {
		t.Fatalf("import recording media: %v", err)
	}
	var libraryType, mediaType, sourceURL string
	if err := server.db.QueryRow(`
		SELECT l.type, m.type, m.source_url
		FROM media_items m
		JOIN libraries l ON l.id = m.library_id
		WHERE m.id = ?`, dvrRecordingMediaID(recording.ID)).Scan(&libraryType, &mediaType, &sourceURL); err != nil {
		t.Fatalf("load imported recording media: %v", err)
	}
	if libraryType != "recorded-tv" || mediaType != "recording" || sourceURL != recordingPath {
		t.Fatalf("unexpected imported recording media: library=%s media=%s source=%s", libraryType, mediaType, sourceURL)
	}
	var searchCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_search WHERE media_id = ? AND media_search MATCH 'Night'`, dvrRecordingMediaID(recording.ID)).Scan(&searchCount); err != nil {
		t.Fatalf("search imported recording: %v", err)
	}
	if searchCount != 1 {
		t.Fatalf("search count = %d, expected 1", searchCount)
	}
}

func TestReconcileCompletedDVRMediaRepairsInterruptedCatalogImport(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	path := filepath.Join(server.cfg.AppDataDir, "recordings", "repair.mp4")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("recording"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources(id,name,type,enabled,created_at,updated_at) VALUES('src_repair','Repair','m3u',1,?,?)`, nowText, nowText); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recordings(id,user_id,profile_id,source_id,title,status,starts_at,ends_at,path,size_bytes,created_at,updated_at)
		VALUES('rec_repair',?,?,'src_repair','Recovered recording','complete',?,?,?,?,?,?)`,
		user.ID, viewerProfileID(user), now.Add(-time.Hour).Format(time.RFC3339), nowText, path, 9, nowText, nowText); err != nil {
		t.Fatal(err)
	}
	if err := server.reconcileCompletedDVRMedia(context.Background(), nowText); err != nil {
		t.Fatal(err)
	}
	var mediaID string
	if err := server.db.QueryRow(`SELECT media_id FROM dvr_recording_media WHERE recording_id='rec_repair'`).Scan(&mediaID); err != nil {
		t.Fatalf("missing repaired media mapping: %v", err)
	}
	if mediaID != dvrRecordingMediaID("rec_repair") {
		t.Fatalf("media id=%q", mediaID)
	}
	// Reconciliation is intentionally idempotent.
	if err := server.reconcileCompletedDVRMedia(context.Background(), nowText); err != nil {
		t.Fatal(err)
	}
}

func TestClaimDVRRecordingOnlyClaimsScheduledRows(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_test", "channel_test", now)
	_ = dvrTestUser(t, server)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recordings (id, user_id, profile_id, source_id, channel_id, title, status, starts_at, ends_at, created_at, updated_at)
		VALUES ('rec_test', 'usr_test', 'usr_test', 'src_test', 'channel_test', 'News', 'scheduled', ?, ?, ?, ?)`,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339), time.Now().UTC().Add(2*time.Hour).Format(time.RFC3339), now, now); err != nil {
		t.Fatalf("insert recording: %v", err)
	}
	if !server.claimDVRRecording("rec_test") {
		t.Fatalf("expected scheduled recording to be claimed")
	}
	if server.claimDVRRecording("rec_test") {
		t.Fatalf("expected running recording not to be claimed twice")
	}
}

func TestDVRRecordingConflictBlocksOverlap(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_test", "channel_test", nowText)
	first, err := server.createDVRRecording(user, DVRRecordingRequest{
		SourceID:  "src_test",
		ChannelID: "channel_test",
		Title:     "Morning News",
		StartsAt:  now.Add(time.Hour).Format(time.RFC3339),
		EndsAt:    now.Add(2 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("create first recording: %v", err)
	}
	if first.Status != "scheduled" {
		t.Fatalf("first status = %s", first.Status)
	}
	_, err = server.createDVRRecording(user, DVRRecordingRequest{
		SourceID:  "src_test",
		ChannelID: "channel_test",
		Title:     "Weather",
		StartsAt:  now.Add(90 * time.Minute).Format(time.RFC3339),
		EndsAt:    now.Add(150 * time.Minute).Format(time.RFC3339),
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestDVRRecordingUpdateAndCancelOnlyScheduledRows(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_test", "channel_test", nowText)
	recording, err := server.createDVRRecording(user, DVRRecordingRequest{
		SourceID:  "src_test",
		ChannelID: "channel_test",
		Title:     "Morning News",
		StartsAt:  now.Add(time.Hour).Format(time.RFC3339),
		EndsAt:    now.Add(2 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("create recording: %v", err)
	}
	updated, err := server.updateDVRRecording(recording.ID, DVRRecordingRequest{
		Title:    "Morning News Extended",
		StartsAt: now.Add(2 * time.Hour).Format(time.RFC3339),
		EndsAt:   now.Add(3 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("update recording: %v", err)
	}
	if updated.Title != "Morning News Extended" || updated.StartsAt != now.Add(2*time.Hour).UTC().Format(time.RFC3339) {
		t.Fatalf("unexpected updated recording: %#v", updated)
	}
	if err := server.cancelDVRRecording(recording.ID); err != nil {
		t.Fatalf("cancel recording: %v", err)
	}
	if _, err := server.getDVRRecording(recording.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected cancelled recording to be removed, err=%v", err)
	}

	running, err := server.createDVRRecording(user, DVRRecordingRequest{
		SourceID:  "src_test",
		ChannelID: "channel_test",
		Title:     "Evening News",
		StartsAt:  now.Add(4 * time.Hour).Format(time.RFC3339),
		EndsAt:    now.Add(5 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("create running candidate: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE live_tv_recordings SET status = 'running' WHERE id = ?`, running.ID); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if _, err := server.updateDVRRecording(running.ID, DVRRecordingRequest{Title: "Should Fail"}); err == nil || !strings.Contains(err.Error(), "scheduled") {
		t.Fatalf("expected scheduled-only update error, got %v", err)
	}
	if err := server.cancelDVRRecording(running.ID); err == nil || !strings.Contains(err.Error(), "scheduled") {
		t.Fatalf("expected scheduled-only cancel error, got %v", err)
	}
}

func TestDeleteDVRRecordingRemovesFinishedRecordingFileAndMedia(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_test", "channel_test", nowText)
	recordingPath := filepath.Join(server.cfg.AppDataDir, "recordings", "2026", "05", "delete-me.mp4")
	if err := os.MkdirAll(filepath.Dir(recordingPath), 0o700); err != nil {
		t.Fatalf("mkdir recording: %v", err)
	}
	if err := os.WriteFile(recordingPath, []byte("recording"), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}
	recording := DVRRecording{
		ID:        "rec_delete",
		UserID:    user.ID,
		SourceID:  "src_test",
		ChannelID: "channel_test",
		Title:     "Delete Me",
		Status:    "complete",
		StartsAt:  now.Add(-time.Hour).Format(time.RFC3339),
		EndsAt:    now.Format(time.RFC3339),
		Path:      recordingPath,
		SizeBytes: 9,
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recordings (id, user_id, profile_id, source_id, channel_id, title, status, starts_at, ends_at, path, size_bytes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		recording.ID, recording.UserID, viewerProfileID(user), recording.SourceID, recording.ChannelID, recording.Title, recording.Status, recording.StartsAt, recording.EndsAt, recording.Path, recording.SizeBytes, nowText, nowText); err != nil {
		t.Fatalf("insert recording: %v", err)
	}
	if err := server.importDVRRecordingMedia(recording, nowText); err != nil {
		t.Fatalf("import recording media: %v", err)
	}
	releasePlayback := server.acquireDVRPlaybackUse(recording.ID)
	if err := server.deleteDVRRecording(recording.ID); !errors.Is(err, errDVRRecordingInUse) {
		t.Fatalf("active recording delete error = %v, want in-use conflict", err)
	}
	if _, err := os.Stat(recordingPath); err != nil {
		t.Fatalf("active recording file was touched: %v", err)
	}
	releasePlayback(false)
	if err := server.deleteDVRRecording(recording.ID); err != nil {
		t.Fatalf("delete recording: %v", err)
	}
	if _, err := os.Stat(recordingPath); !os.IsNotExist(err) {
		t.Fatalf("expected recording file to be removed, stat err=%v", err)
	}
	for label, query := range map[string]string{
		"recording":    `SELECT COUNT(*) FROM live_tv_recordings WHERE id = 'rec_delete'`,
		"media item":   `SELECT COUNT(*) FROM media_items WHERE id = ?`,
		"media stream": `SELECT COUNT(*) FROM media_streams WHERE media_id = ?`,
		"media search": `SELECT COUNT(*) FROM media_search WHERE media_id = ?`,
	} {
		var count int
		var err error
		if label == "recording" {
			err = server.db.QueryRow(query).Scan(&count)
		} else {
			err = server.db.QueryRow(query, dvrRecordingMediaID(recording.ID)).Scan(&count)
		}
		if err != nil {
			t.Fatalf("count %s: %v", label, err)
		}
		if count != 0 {
			t.Fatalf("expected %s rows to be removed, got %d", label, count)
		}
	}

	running, err := server.createDVRRecording(user, DVRRecordingRequest{
		SourceID:  "src_test",
		ChannelID: "channel_test",
		Title:     "Still Recording",
		StartsAt:  now.Add(2 * time.Hour).Format(time.RFC3339),
		EndsAt:    now.Add(3 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("create running recording: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE live_tv_recordings SET status = 'running' WHERE id = ?`, running.ID); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if err := server.deleteDVRRecording(running.ID); err == nil || !strings.Contains(err.Error(), "Running recordings") {
		t.Fatalf("expected running delete error, got %v", err)
	}
}

func TestLiveTVSourceDeletionRejectsActiveAllocation(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id,name,type,created_at,updated_at) VALUES ('source_active_delete','Active source','m3u',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	insertDVRTestChannel(t, server, "source_active_delete", "channel_active_delete", now)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_tuner_allocations
			(id,source_id,channel_id,allocation_kind,consumer_id,allocation_key,lease_token,acquired_at,heartbeat_at)
		VALUES ('allocation_active_delete','source_active_delete','channel_active_delete','live_session','session_active_delete','live_session:session_active_delete','lease_active_delete',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.deleteLiveTVSource("source_active_delete"); !errors.Is(err, errLiveTVSourceInUse) {
		t.Fatalf("active source delete error = %v, want in-use conflict", err)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_sources WHERE id='source_active_delete'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("active source was deleted: count=%d err=%v", count, err)
	}
}

func TestDVRRecordingStreamServesCompletedRecordingWithRangeSupport(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_test", "channel_test", nowText)
	recordingPath := filepath.Join(server.cfg.AppDataDir, "recordings", "2026", "05", "stream-me.mp4")
	if err := os.MkdirAll(filepath.Dir(recordingPath), 0o700); err != nil {
		t.Fatalf("mkdir recording: %v", err)
	}
	if err := os.WriteFile(recordingPath, []byte("recording-data"), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recordings (id, user_id, profile_id, source_id, channel_id, title, status, starts_at, ends_at, path, size_bytes, created_at, updated_at)
		VALUES ('rec_stream', ?, ?, 'src_test', 'channel_test', 'Stream Me', 'complete', ?, ?, ?, 14, ?, ?)`,
		user.ID, viewerProfileID(user), now.Add(-time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), recordingPath, nowText, nowText); err != nil {
		t.Fatalf("insert recording: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dvr/recordings/rec_stream/stream", nil)
	req.Header.Set("Range", "bytes=0-8")
	recorder := httptest.NewRecorder()
	server.handleDVRRecordingStream(recorder, req, user, "rec_stream")
	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.String(); got != "recording" {
		t.Fatalf("range body = %q", got)
	}
	if recorder.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("expected byte range support")
	}

	if _, err := server.db.Exec(`UPDATE live_tv_recordings SET path = ? WHERE id = 'rec_stream'`, filepath.Join(server.cfg.AppDataDir, "..", "escape.mp4")); err != nil {
		t.Fatalf("mark escaped path: %v", err)
	}
	recorder = httptest.NewRecorder()
	server.handleDVRRecordingStream(recorder, httptest.NewRequest(http.MethodGet, "/api/dvr/recordings/rec_stream/stream", nil), user, "rec_stream")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("escaped path status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDVRRecordingPlaybackStartsCanonicalSession(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_playback', 'Playback Source', 'm3u', ?, ?)`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_playback", "channel_playback", nowText)
	recordingPath := filepath.Join(server.cfg.AppDataDir, "recordings", "canonical-playback.mp4")
	if err := os.MkdirAll(filepath.Dir(recordingPath), 0o700); err != nil {
		t.Fatalf("mkdir recording: %v", err)
	}
	if err := os.WriteFile(recordingPath, []byte("recording-data"), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}
	recording := DVRRecording{
		ID: "rec_playback", UserID: user.ID, ProfileID: viewerProfileID(user), SourceID: "src_playback", ChannelID: "channel_playback", Title: "Canonical Recording", Status: "complete",
		StartsAt: now.Add(-time.Hour).Format(time.RFC3339), EndsAt: now.Format(time.RFC3339), Path: recordingPath, SizeBytes: 14,
		CreatedAt: nowText, UpdatedAt: nowText,
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recordings (id, user_id, profile_id, source_id, channel_id, title, status, starts_at, ends_at, path, size_bytes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, recording.ID, recording.UserID, recording.ProfileID, recording.SourceID, recording.ChannelID, recording.Title,
		recording.Status, recording.StartsAt, recording.EndsAt, recording.Path, recording.SizeBytes, recording.CreatedAt, recording.UpdatedAt); err != nil {
		t.Fatalf("insert recording: %v", err)
	}
	if err := server.importDVRRecordingMedia(recording, nowText); err != nil {
		t.Fatalf("import recording media: %v", err)
	}
	seedExactPlaybackFactsForFixture(t, server, dvrRecordingMediaID(recording.ID))

	payload, err := json.Marshal(DVRPlaybackSessionCreateRequest{ClientInstanceID: "dvr-web", StartSeconds: 12, ClientProfile: authenticatedPlaybackRuntimeProfile()})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/dvr/recordings/rec_playback/playback", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.handleDVRRecordingPlayback(recorder, req, user, recording.ID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode playback: %v", err)
	}
	if playback.SessionID == "" || playback.MediaGrant.Token == "" {
		t.Fatalf("canonical playback identifiers missing: %#v", playback)
	}
	if playback.Media.ID != dvrRecordingMediaID(recording.ID) || playback.Timeline.Type != "vod" || playback.IsLive {
		t.Fatalf("unexpected DVR playback response: %#v", playback)
	}
	var sessionMediaID, clientInstanceID string
	if err := server.db.QueryRow(`SELECT media_id, client_instance_id FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&sessionMediaID, &clientInstanceID); err != nil {
		t.Fatalf("load playback session: %v", err)
	}
	if sessionMediaID != playback.Media.ID || clientInstanceID != "dvr-web" {
		t.Fatalf("session media/client = %q/%q", sessionMediaID, clientInstanceID)
	}
}

func TestDVRRoutePermissionMatrixForScopedUsers(t *testing.T) {
	t.Run("view DVR reads are scoped to owned rows", func(t *testing.T) {
		server := newScannerTestServer(t)
		fixture := seedDVRPermissionFixture(t, server)
		rec := performDVRRouteRequest(server, fixture.viewUser, http.MethodGet, "/api/dvr/recordings?count=exact", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("view recordings status=%d body=%s", rec.Code, rec.Body.String())
		}
		if body := rec.Body.String(); !strings.Contains(body, "rec_view_complete") || strings.Contains(body, "rec_other_complete") {
			t.Fatalf("view user recordings were not scoped to owned rows: %s", body)
		}
		rec = performDVRRouteRequest(server, fixture.viewUser, http.MethodGet, "/api/dvr/rules?count=exact", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("view rules status=%d body=%s", rec.Code, rec.Body.String())
		}
		if body := rec.Body.String(); !strings.Contains(body, "rule_view_owned") || strings.Contains(body, "rule_other") {
			t.Fatalf("view user rules were not scoped to owned rows: %s", body)
		}
	})

	t.Run("manage DVR remains scoped to the active profile", func(t *testing.T) {
		server := newScannerTestServer(t)
		fixture := seedDVRPermissionFixture(t, server)
		rec := performDVRRouteRequest(server, fixture.manageUser, http.MethodGet, "/api/dvr/recordings?count=exact", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("manage recordings status=%d body=%s", rec.Code, rec.Body.String())
		}
		if body := rec.Body.String(); strings.Contains(body, "rec_view_complete") || strings.Contains(body, "rec_other_complete") {
			t.Fatalf("manage DVR recording projection escaped active profile: %s", body)
		}
		rec = performDVRRouteRequest(server, fixture.manageUser, http.MethodGet, "/api/dvr/rules?count=exact", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("manage rules status=%d body=%s", rec.Code, rec.Body.String())
		}
		if body := rec.Body.String(); strings.Contains(body, "rule_view_owned") || strings.Contains(body, "rule_other") {
			t.Fatalf("manage DVR rule projection escaped active profile: %s", body)
		}
	})

	t.Run("scheduling requires schedule or manage DVR", func(t *testing.T) {
		server := newScannerTestServer(t)
		fixture := seedDVRPermissionFixture(t, server)
		body := `{"sourceId":"src_perm","channelId":"channel_perm","title":"New Scheduled","startsAt":"` + fixture.futureStart + `","endsAt":"` + fixture.futureEnd + `"}`
		rec := performDVRRouteRequest(server, fixture.viewUser, http.MethodPost, "/api/dvr/recordings", body)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("view-only schedule status=%d body=%s", rec.Code, rec.Body.String())
		}
		rec = performDVRRouteRequest(server, fixture.scheduleUser, http.MethodPost, "/api/dvr/recordings", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("schedule user create status=%d body=%s", rec.Code, rec.Body.String())
		}
		body = `{"sourceId":"src_perm","channelId":"channel_perm","title":"Managed Scheduled","startsAt":"` + fixture.laterFutureStart + `","endsAt":"` + fixture.laterFutureEnd + `"}`
		rec = performDVRRouteRequest(server, fixture.manageUser, http.MethodPost, "/api/dvr/recordings", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("manage user create status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("scheduled recording deletes require schedule or manage DVR", func(t *testing.T) {
		server := newScannerTestServer(t)
		fixture := seedDVRPermissionFixture(t, server)
		rec := performDVRRouteRequest(server, fixture.deleteUser, http.MethodDelete, "/api/dvr/recordings/rec_delete_scheduled", "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("delete-only scheduled delete status=%d body=%s", rec.Code, rec.Body.String())
		}
		rec = performDVRRouteRequest(server, fixture.scheduleUser, http.MethodDelete, "/api/dvr/recordings/rec_schedule_scheduled", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("schedule user scheduled delete status=%d body=%s", rec.Code, rec.Body.String())
		}
		rec = performDVRRouteRequest(server, fixture.manageUser, http.MethodDelete, "/api/dvr/recordings/rec_other_scheduled", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("manage DVR cross-profile scheduled delete status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("finished recording deletes require delete or manage DVR", func(t *testing.T) {
		server := newScannerTestServer(t)
		fixture := seedDVRPermissionFixture(t, server)
		rec := performDVRRouteRequest(server, fixture.scheduleUser, http.MethodDelete, "/api/dvr/recordings/rec_schedule_complete", "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("schedule-only completed delete status=%d body=%s", rec.Code, rec.Body.String())
		}
		rec = performDVRRouteRequest(server, fixture.deleteUser, http.MethodDelete, "/api/dvr/recordings/rec_delete_complete", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("delete user completed delete status=%d body=%s", rec.Code, rec.Body.String())
		}
		rec = performDVRRouteRequest(server, fixture.deleteUser, http.MethodDelete, "/api/dvr/recordings/rec_delete_failed", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("delete user failed delete status=%d body=%s", rec.Code, rec.Body.String())
		}
		rec = performDVRRouteRequest(server, fixture.manageUser, http.MethodDelete, "/api/dvr/recordings/rec_other_complete", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("manage DVR cross-profile completed delete status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("streaming requires DVR access plus play media", func(t *testing.T) {
		server := newScannerTestServer(t)
		fixture := seedDVRPermissionFixture(t, server)
		rec := performDVRRouteRequest(server, fixture.viewUser, http.MethodGet, "/api/dvr/recordings/rec_view_complete/stream", "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("view-only stream status=%d body=%s", rec.Code, rec.Body.String())
		}
		rec = performDVRRouteRequest(server, fixture.playOnlyUser, http.MethodGet, "/api/dvr/recordings/rec_player_complete/stream", "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("play-only stream should fail top-level DVR access, status=%d body=%s", rec.Code, rec.Body.String())
		}
		rec = performDVRRouteRequest(server, fixture.viewAndPlayUser, http.MethodGet, "/api/dvr/recordings/rec_player_complete/stream", "")
		if rec.Code != http.StatusOK || rec.Body.String() != "recording-data" {
			t.Fatalf("view+play stream status=%d body=%q", rec.Code, rec.Body.String())
		}
	})
}

func TestDVRRecordingHLSHelpersUseValidatedRecordingPath(t *testing.T) {
	server := newScannerTestServer(t)
	root := filepath.Join(server.cfg.AppDataDir, "recordings")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create recordings root: %v", err)
	}
	recordingPath := filepath.Join(root, "Movie.ts")
	if err := os.WriteFile(recordingPath, []byte("recording-data"), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}
	recording := DVRRecording{
		ID:       "rec_hls",
		Title:    "HLS Recording",
		Status:   "complete",
		Path:     recordingPath,
		StartsAt: "2026-05-23T20:00:00Z",
		EndsAt:   "2026-05-23T21:30:00Z",
	}
	item, err := server.dvrRecordingTranscodeItem(recording)
	if err != nil {
		t.Fatalf("dvr transcode item: %v", err)
	}
	if item.Type != "dvr_recording" || item.SourceURL != recordingPath || item.DurationSeconds != 5400 {
		t.Fatalf("unexpected dvr transcode item: %+v", item)
	}
	sourcePath, err := server.sourcePathForHLSTranscode(item)
	if err != nil {
		t.Fatalf("dvr hls source path: %v", err)
	}
	if sourcePath != recordingPath {
		t.Fatalf("dvr hls source path = %q, expected %q", sourcePath, recordingPath)
	}

	recording.Path = filepath.Join(server.cfg.AppDataDir, "..", "escape.ts")
	if _, err := server.dvrRecordingTranscodeItem(recording); err == nil {
		t.Fatalf("expected escaped dvr recording path to be rejected")
	}
}

func TestRewriteDVRRecordingHLSManifestUsesDVRSegmentRoutes(t *testing.T) {
	manifest := "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:4.000,\nsegment_00001.ts\n"
	rewritten := rewriteDVRRecordingHLSManifest("rec hls", "ptc_token", manifest)
	if !strings.Contains(rewritten, "/api/dvr/recordings/rec%20hls/hls/segment?name=segment_00001.ts") {
		t.Fatalf("manifest did not rewrite dvr segment route:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "media_grant=") {
		t.Fatalf("manifest exposed a media grant in its URL:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "/api/media/") {
		t.Fatalf("dvr manifest must not use media HLS routes:\n%s", rewritten)
	}
}

func TestDVRRecordingGroupsExcludeScheduledAndAggregateByTitle(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Truncate(time.Second)
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	for index, status := range []string{"complete", "running", "scheduled"} {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_recordings (id, user_id, source_id, title, status, starts_at, ends_at, size_bytes, created_at, updated_at)
			VALUES (?, ?, 'src_test', 'Daily News', ?, ?, ?, ?, ?, ?)`,
			"rec_group_"+strconv.Itoa(index+1), user.ID, status, now.Add(time.Duration(index)*time.Hour).Format(time.RFC3339), now.Add(time.Duration(index+1)*time.Hour).Format(time.RFC3339), int64(10+index), nowText, nowText); err != nil {
			t.Fatalf("insert recording %d: %v", index, err)
		}
	}
	groups, err := server.listDVRRecordingGroups()
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %#v", groups)
	}
	if groups[0].Title != "Daily News" || groups[0].Count != 2 || groups[0].SizeBytes != 21 {
		t.Fatalf("unexpected group: %#v", groups[0])
	}
	if len(groups[0].Recordings) != 2 || groups[0].Recordings[0].Status != "running" {
		t.Fatalf("unexpected grouped recordings: %#v", groups[0].Recordings)
	}
}

func TestDVRSeriesRuleCreatesFutureMatchingRecordings(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_test", "channel_series", nowText)
	for index, startOffset := range []time.Duration{time.Hour, 3 * time.Hour} {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_programs (id, source_id, channel_id, title, start_at, end_at, created_at)
			VALUES (?, 'src_test', 'channel_series', 'Daily News', ?, ?, ?)`,
			"program_news_"+strconv.Itoa(index+1), now.Add(startOffset).Format(time.RFC3339), now.Add(startOffset+time.Hour).Format(time.RFC3339), nowText); err != nil {
			t.Fatalf("insert program: %v", err)
		}
	}
	rule, err := server.createDVRRule(user, DVRRecordingRuleRequest{
		SourceID:      "src_test",
		Title:         "Daily News",
		MatchType:     "series",
		RetentionDays: 7,
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create series rule: %v", err)
	}
	if rule.MatchType != "series" {
		t.Fatalf("match type = %s", rule.MatchType)
	}
	recordings, err := server.listDVRRecordings()
	if err != nil {
		t.Fatalf("list recordings: %v", err)
	}
	if len(recordings) != 2 {
		t.Fatalf("expected two series recordings, got %#v", recordings)
	}
}

func TestDVRSeriesRuleFiltersGuideMatches(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_filters', 'Filtered Source', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	for _, channel := range []struct {
		id     string
		number string
		name   string
	}{
		{id: "channel_cbc", number: "7", name: "CBC"},
		{id: "channel_kids", number: "11", name: "Kids"},
	} {
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_channels (id, source_id, number, name, stream_url, last_seen_at, created_at, updated_at)
			VALUES (?, 'src_filters', ?, ?, 'https://media.example.test/live.m3u8', ?, ?, ?)
			ON CONFLICT(id) DO NOTHING`, channel.id, channel.number, channel.name, nowText, nowText, nowText); err != nil {
			t.Fatalf("insert channel %s: %v", channel.id, err)
		}
	}
	programs := []struct {
		id          string
		channelID   string
		description string
	}{
		{id: "program_good", channelID: "channel_cbc", description: "Live evening coverage."},
		{id: "program_repeat", channelID: "channel_cbc", description: "Live repeat broadcast."},
		{id: "program_blocked_channel", channelID: "channel_kids", description: "Live evening coverage."},
	}
	for index, program := range programs {
		start := now.Add(time.Duration(index+1) * time.Hour).Truncate(time.Second)
		if _, err := server.db.Exec(`
			INSERT INTO live_tv_programs (id, source_id, channel_id, title, description, start_at, end_at, created_at)
			VALUES (?, 'src_filters', ?, 'Daily News', ?, ?, ?, ?)`,
			program.id, program.channelID, program.description, start.Format(time.RFC3339), start.Add(time.Hour).Format(time.RFC3339), nowText); err != nil {
			t.Fatalf("insert program %s: %v", program.id, err)
		}
	}
	rule, err := server.createDVRRule(user, DVRRecordingRuleRequest{
		SourceID:         "src_filters",
		Title:            "Daily News",
		MatchType:        "series",
		RetentionDays:    7,
		RequiredKeywords: []string{"live"},
		BlockedKeywords:  []string{"repeat"},
		AllowedChannels:  []string{"CBC"},
		BlockedChannels:  []string{"Kids"},
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("create filtered series rule: %v", err)
	}
	recordings, err := server.listDVRRecordings()
	if err != nil {
		t.Fatalf("list recordings: %v", err)
	}
	matched := []DVRRecording{}
	for _, recording := range recordings {
		if recording.RuleID == rule.ID {
			matched = append(matched, recording)
		}
	}
	if len(matched) != 1 || matched[0].ProgramID != "program_good" {
		t.Fatalf("filtered recordings = %#v", matched)
	}
}

func TestDVRRuleUpdatePersistsEditableTimerDefaults(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	rule, err := server.createDVRRule(user, DVRRecordingRuleRequest{
		SourceID:            "src_test",
		Title:               "News",
		MatchType:           "series",
		StartPaddingMinutes: 1,
		EndPaddingMinutes:   2,
		RetentionDays:       7,
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	updated, err := server.updateDVRRule(rule.ID, DVRRecordingRuleRequest{
		SourceID:            "src_test",
		Title:               "News Updated",
		MatchType:           "series",
		StartPaddingMinutes: 5,
		EndPaddingMinutes:   10,
		RetentionDays:       14,
		Enabled:             false,
	})
	if err != nil {
		t.Fatalf("update rule: %v", err)
	}
	if updated.Title != "News Updated" || updated.MatchType != "series" || updated.StartPaddingMinutes != 5 || updated.EndPaddingMinutes != 10 || updated.RetentionDays != 14 || updated.Enabled {
		t.Fatalf("unexpected updated rule: %#v", updated)
	}
	loaded, err := server.getDVRRule(rule.ID)
	if err != nil {
		t.Fatalf("load updated rule: %v", err)
	}
	if loaded.Title != updated.Title || loaded.MatchType != updated.MatchType || loaded.Enabled != updated.Enabled {
		t.Fatalf("rule was not persisted: before=%#v after=%#v", rule, loaded)
	}
}

func TestDVRTimerDefaultsApplyToRulesAndOneOffRecordings(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO settings (key, value_json, updated_at)
		VALUES ('dvr', '{"defaultStartPaddingMinutes":5,"defaultEndPaddingMinutes":7,"defaultRetentionDays":21,"defaultFolder":"Daily News","defaultMaxRecordingsPerSeries":5,"defaultRuleRequiredKeywords":["live"],"defaultRuleBlockedKeywords":["repeat"],"defaultRuleAllowedChannels":["CBC"],"defaultRuleBlockedChannels":["Kids"]}', ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, nowText); err != nil {
		t.Fatalf("save dvr settings: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_test', 'Test', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_test", "channel_defaults", nowText)
	programStart := now.Add(2 * time.Hour).Truncate(time.Second)
	programEnd := programStart.Add(time.Hour)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_programs (id, source_id, channel_id, title, start_at, end_at, created_at)
		VALUES ('program_defaults', 'src_test', 'channel_defaults', 'Default News', ?, ?, ?)`,
		programStart.Format(time.RFC3339), programEnd.Format(time.RFC3339), nowText); err != nil {
		t.Fatalf("insert program: %v", err)
	}
	rule, err := server.createDVRRule(user, DVRRecordingRuleRequest{
		SourceID:  "src_test",
		ProgramID: "program_defaults",
		Title:     "Default News",
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if rule.StartPaddingMinutes != 5 || rule.EndPaddingMinutes != 7 || rule.RetentionDays != 21 || rule.Folder != "Daily News" || rule.MaxRecordingsPerSeries != 5 {
		t.Fatalf("rule defaults = start %d end %d retention %d folder %q max %d", rule.StartPaddingMinutes, rule.EndPaddingMinutes, rule.RetentionDays, rule.Folder, rule.MaxRecordingsPerSeries)
	}
	if strings.Join(rule.RequiredKeywords, ",") != "live" || strings.Join(rule.BlockedKeywords, ",") != "repeat" || strings.Join(rule.AllowedChannels, ",") != "CBC" || strings.Join(rule.BlockedChannels, ",") != "Kids" {
		t.Fatalf("rule filters = required %#v blocked %#v allowed %#v blocked channels %#v", rule.RequiredKeywords, rule.BlockedKeywords, rule.AllowedChannels, rule.BlockedChannels)
	}
	recordings, err := server.listDVRRecordings()
	if err != nil {
		t.Fatalf("list recordings: %v", err)
	}
	var ruleRecording DVRRecording
	for _, recording := range recordings {
		if recording.RuleID == rule.ID {
			ruleRecording = recording
			break
		}
	}
	if ruleRecording.ID == "" {
		t.Fatalf("expected rule-created recording")
	}
	if ruleRecording.Folder != "Daily News" {
		t.Fatalf("recording folder = %q", ruleRecording.Folder)
	}
	if ruleRecording.StartsAt != programStart.Add(-5*time.Minute).Format(time.RFC3339) || ruleRecording.EndsAt != programEnd.Add(7*time.Minute).Format(time.RFC3339) {
		t.Fatalf("recording times = %s to %s", ruleRecording.StartsAt, ruleRecording.EndsAt)
	}

	oneOffStart := now.Add(5 * time.Hour).Truncate(time.Second)
	oneOffEnd := oneOffStart.Add(time.Hour)
	oneOff, err := server.createDVRRecording(user, DVRRecordingRequest{
		SourceID:  "src_test",
		ChannelID: "channel_defaults",
		Title:     "One Off Defaults",
		StartsAt:  oneOffStart.Format(time.RFC3339),
		EndsAt:    oneOffEnd.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("create one-off recording: %v", err)
	}
	if oneOff.StartsAt != oneOffStart.Add(-5*time.Minute).Format(time.RFC3339) || oneOff.EndsAt != oneOffEnd.Add(7*time.Minute).Format(time.RFC3339) {
		t.Fatalf("one-off times = %s to %s", oneOff.StartsAt, oneOff.EndsAt)
	}
	if oneOff.Folder != "Daily News" {
		t.Fatalf("one-off folder = %q", oneOff.Folder)
	}
}

func TestDVROutputPathHonorsSanitizedTemplate(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO settings (key, value_json, updated_at)
		VALUES ('dvr', '{"recordingPathTemplate":"{folder}/{channel}/{year}/{title}-{start}"}', ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, now); err != nil {
		t.Fatalf("save dvr settings: %v", err)
	}
	start := time.Date(2026, 5, 4, 21, 30, 45, 0, time.UTC)
	path, err := server.dvrOutputPath(DVRRecording{
		ID:        "rec_template",
		Title:     "Evening News: Unsafe / Name",
		Folder:    "Daily / News",
		ChannelID: "channel:5.1",
	}, start)
	if err != nil {
		t.Fatalf("output path: %v", err)
	}
	expected := filepath.Join(server.cfg.AppDataDir, "recordings", "DailyNews", "channel51", "2026", "EveningNewsUnsafeName-20260504-213045-rec_template.mp4")
	if path != expected {
		t.Fatalf("output path = %q, expected %q", path, expected)
	}
	if !strings.HasPrefix(path, filepath.Join(server.cfg.AppDataDir, "recordings")+string(filepath.Separator)) {
		t.Fatalf("output path escaped recordings root: %s", path)
	}
}

func TestDVRRecordingNFOWriteAndDeleteStayInsideRecordings(t *testing.T) {
	server := newScannerTestServer(t)
	recordingPath := filepath.Join(server.cfg.AppDataDir, "recordings", "DailyNews", "EveningNews.mp4")
	if err := os.MkdirAll(filepath.Dir(recordingPath), 0o700); err != nil {
		t.Fatalf("create recording dir: %v", err)
	}
	if err := os.WriteFile(recordingPath, []byte("video"), 0o600); err != nil {
		t.Fatalf("write recording: %v", err)
	}
	recording := DVRRecording{
		ID:       "rec_nfo",
		Title:    "Evening News & Weather",
		StartsAt: time.Date(2026, 5, 4, 21, 30, 0, 0, time.UTC).Format(time.RFC3339),
		Path:     recordingPath,
	}
	if err := server.writeDVRRecordingNFO(recording); err != nil {
		t.Fatalf("write nfo: %v", err)
	}
	if err := server.writeDVRRecordingImageSidecars(recording); err != nil {
		t.Fatalf("write image sidecars: %v", err)
	}
	nfoPath := filepath.Join(server.cfg.AppDataDir, "recordings", "DailyNews", "EveningNews.nfo")
	bytes, err := os.ReadFile(nfoPath)
	if err != nil {
		t.Fatalf("read nfo: %v", err)
	}
	nfo := string(bytes)
	if !strings.Contains(nfo, "<title>Evening News &amp; Weather</title>") || !strings.Contains(nfo, "<genre>Recorded TV</genre>") || !strings.Contains(nfo, "<aired>2026-05-04</aired>") {
		t.Fatalf("unexpected nfo: %s", nfo)
	}
	posterPath := filepath.Join(server.cfg.AppDataDir, "recordings", "DailyNews", "EveningNews-poster.svg")
	thumbPath := filepath.Join(server.cfg.AppDataDir, "recordings", "DailyNews", "EveningNews-thumb.svg")
	poster, err := os.ReadFile(posterPath)
	if err != nil {
		t.Fatalf("read poster sidecar: %v", err)
	}
	if !strings.Contains(string(poster), "<svg") || !strings.Contains(string(poster), "Evening News &amp; Weather") {
		t.Fatalf("unexpected poster sidecar: %s", string(poster))
	}
	if _, err := os.Stat(thumbPath); err != nil {
		t.Fatalf("thumb sidecar missing: %v", err)
	}
	if err := removeDVRRecordingFile(recordingPath, server.cfg.AppDataDir); err != nil {
		t.Fatalf("remove recording: %v", err)
	}
	if _, err := os.Stat(recordingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recording still exists or stat failed differently: %v", err)
	}
	if _, err := os.Stat(nfoPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nfo still exists or stat failed differently: %v", err)
	}
	if _, err := os.Stat(posterPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("poster sidecar still exists or stat failed differently: %v", err)
	}
	if _, err := os.Stat(thumbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("thumb sidecar still exists or stat failed differently: %v", err)
	}
}

func TestDVRRecordingFFmpegArgsPreserveAllStreams(t *testing.T) {
	args := dvrRecordingFFmpegArgs("http://example.test/live.m3u8", 90*time.Second, "/tmp/out.mp4", "copy", true)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-map 0") || !strings.Contains(joined, "-c copy") {
		t.Fatalf("expected preserve-all-stream copy args, got %v", args)
	}
	legacyArgs := dvrRecordingFFmpegArgs("http://example.test/live.m3u8", 90*time.Second, "/tmp/out.mp4", "copy", false)
	if strings.Contains(strings.Join(legacyArgs, " "), "-map 0") {
		t.Fatalf("legacy args should not force map 0: %v", legacyArgs)
	}
	encodedArgs := dvrRecordingFFmpegArgs("http://example.test/live.m3u8", 90*time.Second, "/tmp/out.mp4", "h264-1080p-8m", true)
	encodedJoined := strings.Join(encodedArgs, " ")
	if strings.Contains(encodedJoined, "-map 0 ") || !strings.Contains(encodedJoined, "-c:v libx264") || !strings.Contains(encodedJoined, "-b:v 8M") || !strings.Contains(encodedJoined, "scale=-2:min(1080\\,ih)") || !strings.Contains(encodedJoined, "-c:a aac") {
		t.Fatalf("1080p encoded args should re-encode first video/audio stream without preserve-all map: %v", encodedArgs)
	}
	compactArgs := dvrRecordingFFmpegArgs("http://example.test/live.m3u8", 90*time.Second, "/tmp/out.mp4", "h264-720p-4m", true)
	compactJoined := strings.Join(compactArgs, " ")
	if !strings.Contains(compactJoined, "-b:v 4M") || !strings.Contains(compactJoined, "scale=-2:min(720\\,ih)") {
		t.Fatalf("720p encoded args should use the compact profile: %v", compactArgs)
	}
}

func TestDVROutputPathIsUniqueEvenWhenTemplateOmitsRecordingID(t *testing.T) {
	server := newScannerTestServer(t)
	start := time.Date(2026, 5, 4, 21, 30, 45, 0, time.UTC)
	first, err := server.dvrOutputPath(DVRRecording{ID: "rec_first", Title: "News", ChannelID: "channel"}, start)
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.dvrOutputPath(DVRRecording{ID: "rec_second", Title: "News", ChannelID: "channel"}, start)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.Contains(first, "rec_first") || !strings.Contains(second, "rec_second") {
		t.Fatalf("recording paths are not uniquely owned: first=%q second=%q", first, second)
	}
}

func TestDVRRecordingDurationDoesNotRoundPositiveCaptureToZero(t *testing.T) {
	args := dvrRecordingFFmpegArgs("https://example.test/live.m3u8", 1400*time.Millisecond, "/tmp/out.mp4", "copy", true)
	for index, argument := range args {
		if argument == "-t" && index+1 < len(args) {
			if args[index+1] == "0" || args[index+1] == "1" {
				t.Fatalf("duration was rounded down: %v", args)
			}
			return
		}
	}
	t.Fatalf("duration argument missing: %v", args)
}

type dvrPermissionFixture struct {
	viewUser         User
	scheduleUser     User
	deleteUser       User
	manageUser       User
	playOnlyUser     User
	viewAndPlayUser  User
	futureStart      string
	futureEnd        string
	laterFutureStart string
	laterFutureEnd   string
}

func seedDVRPermissionFixture(t *testing.T, server *Server) dvrPermissionFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	nowText := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, created_at, updated_at)
		VALUES ('src_perm', 'Permission Source', 'm3u', ?, ?)
		ON CONFLICT(id) DO NOTHING`, nowText, nowText); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	insertDVRTestChannel(t, server, "src_perm", "channel_perm", nowText)
	users := []User{
		dvrPermissionUser("usr_view", map[string]bool{"viewDVR": true}),
		dvrPermissionUser("usr_schedule", map[string]bool{"scheduleDVR": true}),
		dvrPermissionUser("usr_delete", map[string]bool{"deleteDVRRecordings": true}),
		dvrPermissionUser("usr_manage", map[string]bool{"manageDVR": true}),
		dvrPermissionUser("usr_play_only", map[string]bool{"playMedia": true}),
		dvrPermissionUser("usr_view_play", map[string]bool{"viewDVR": true, "playMedia": true}),
		dvrPermissionUser("usr_other", map[string]bool{"viewDVR": true}),
	}
	for _, user := range users {
		if _, err := server.db.Exec(`
			INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'hash', 'user', '{}', '{}', ?, ?)
			ON CONFLICT(id) DO NOTHING`,
			user.ID, user.Username, user.Email, user.DisplayName, nowText, nowText); err != nil {
			t.Fatalf("insert user %s: %v", user.ID, err)
		}
		if _, err := server.db.Exec(`
			INSERT INTO profiles (id, account_id, display_name, role, permissions_json, preferences_json, is_primary, restrictions_json, created_at, updated_at)
			VALUES (?, ?, ?, 'user', '{}', '{}', 1, '{}', ?, ?)
			ON CONFLICT(id) DO NOTHING`, user.ID, user.ID, user.DisplayName, nowText, nowText); err != nil {
			t.Fatalf("insert profile %s: %v", user.ID, err)
		}
	}
	insertDVRPermissionRule(t, server, "rule_view_owned", "usr_view", nowText)
	insertDVRPermissionRule(t, server, "rule_other", "usr_other", nowText)
	pastStart := now.Add(-2 * time.Hour).Format(time.RFC3339)
	pastEnd := now.Add(-time.Hour).Format(time.RFC3339)
	scheduledStart := now.Add(2 * time.Hour).Format(time.RFC3339)
	scheduledEnd := now.Add(3 * time.Hour).Format(time.RFC3339)
	insertDVRPermissionRecording(t, server, "rec_view_complete", "usr_view", "complete", pastStart, pastEnd, writeDVRPermissionRecordingFile(t, server, "view-complete.mp4"), nowText)
	insertDVRPermissionRecording(t, server, "rec_view_scheduled", "usr_view", "scheduled", scheduledStart, scheduledEnd, "", nowText)
	insertDVRPermissionRecording(t, server, "rec_schedule_scheduled", "usr_schedule", "scheduled", scheduledStart, scheduledEnd, "", nowText)
	insertDVRPermissionRecording(t, server, "rec_schedule_complete", "usr_schedule", "complete", pastStart, pastEnd, writeDVRPermissionRecordingFile(t, server, "schedule-complete.mp4"), nowText)
	insertDVRPermissionRecording(t, server, "rec_delete_scheduled", "usr_delete", "scheduled", scheduledStart, scheduledEnd, "", nowText)
	insertDVRPermissionRecording(t, server, "rec_delete_complete", "usr_delete", "complete", pastStart, pastEnd, writeDVRPermissionRecordingFile(t, server, "delete-complete.mp4"), nowText)
	insertDVRPermissionRecording(t, server, "rec_delete_failed", "usr_delete", "failed", pastStart, pastEnd, "", nowText)
	insertDVRPermissionRecording(t, server, "rec_player_complete", "usr_view_play", "complete", pastStart, pastEnd, writeDVRPermissionRecordingFile(t, server, "player-complete.mp4"), nowText)
	insertDVRPermissionRecording(t, server, "rec_other_scheduled", "usr_other", "scheduled", scheduledStart, scheduledEnd, "", nowText)
	insertDVRPermissionRecording(t, server, "rec_other_complete", "usr_other", "complete", pastStart, pastEnd, writeDVRPermissionRecordingFile(t, server, "other-complete.mp4"), nowText)
	return dvrPermissionFixture{
		viewUser:         users[0],
		scheduleUser:     users[1],
		deleteUser:       users[2],
		manageUser:       users[3],
		playOnlyUser:     users[4],
		viewAndPlayUser:  users[5],
		futureStart:      now.Add(72 * time.Hour).Format(time.RFC3339),
		futureEnd:        now.Add(73 * time.Hour).Format(time.RFC3339),
		laterFutureStart: now.Add(74 * time.Hour).Format(time.RFC3339),
		laterFutureEnd:   now.Add(75 * time.Hour).Format(time.RFC3339),
	}
}

func TestRecordedTVGenericMediaQueriesRemainProfileScoped(t *testing.T) {
	server := newScannerTestServer(t)
	fixture := seedDVRPermissionFixture(t, server)
	permissionsJSON := `{"playMedia":true,"viewDVR":true}`
	if _, err := server.db.Exec(`UPDATE users SET permissions_json = ? WHERE id = ?`, permissionsJSON, fixture.viewAndPlayUser.ID); err != nil {
		t.Fatalf("update account DVR permissions: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE profiles SET permissions_json = ? WHERE id = ?`, permissionsJSON, fixture.viewAndPlayUser.ProfileID); err != nil {
		t.Fatalf("update profile DVR permissions: %v", err)
	}
	for _, recordingID := range []string{"rec_player_complete", "rec_other_complete"} {
		recording, err := server.getDVRRecording(recordingID)
		if err != nil {
			t.Fatalf("load recording %s: %v", recordingID, err)
		}
		if err := server.importDVRRecordingMedia(recording, time.Now().UTC().Format(time.RFC3339)); err != nil {
			t.Fatalf("import recording %s: %v", recordingID, err)
		}
	}
	if _, err := server.db.Exec(`UPDATE live_tv_sources SET enabled=0 WHERE id='src_perm'`); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE live_tv_channels SET enabled=0 WHERE id='channel_perm'`); err != nil {
		t.Fatal(err)
	}

	items, err := server.queryMediaListItemsContext(context.Background(), fixture.viewAndPlayUser.ProfileID, `
		WHERE lower(m.type) = 'recording'
		ORDER BY m.id ASC`, nil)
	if err != nil {
		t.Fatalf("query recorded TV through generic media helper: %v", err)
	}
	wantID := dvrRecordingMediaID("rec_player_complete")
	if len(items) != 1 || items[0].ID != wantID {
		t.Fatalf("generic recorded TV query escaped the active profile: %#v, want %s", items, wantID)
	}
}

func dvrPermissionUser(id string, permissions map[string]bool) User {
	return User{
		ID:               id,
		AccountID:        id,
		ProfileID:        id,
		ProfileIsPrimary: true,
		Username:         id,
		Email:            id + "@example.test",
		DisplayName:      id,
		Role:             "user",
		Permissions:      permissions,
	}
}

func insertDVRPermissionRule(t *testing.T, server *Server, id string, userID string, nowText string) {
	t.Helper()
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recording_rules (id, user_id, profile_id, source_id, title, match_type, retention_days, created_at, updated_at)
		VALUES (?, ?, ?, 'src_perm', ?, 'title', 30, ?, ?)`,
		id, userID, userID, id, nowText, nowText); err != nil {
		t.Fatalf("insert rule %s: %v", id, err)
	}
}

func insertDVRPermissionRecording(t *testing.T, server *Server, id string, userID string, status string, startsAt string, endsAt string, path string, nowText string) {
	t.Helper()
	size := int64(0)
	if path != "" {
		size = int64(len("recording-data"))
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recordings (id, user_id, profile_id, source_id, channel_id, title, status, starts_at, ends_at, path, size_bytes, created_at, updated_at)
		VALUES (?, ?, ?, 'src_perm', 'channel_perm', ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, userID, userID, id, status, startsAt, endsAt, path, size, nowText, nowText); err != nil {
		t.Fatalf("insert recording %s: %v", id, err)
	}
}

func writeDVRPermissionRecordingFile(t *testing.T, server *Server, name string) string {
	t.Helper()
	path := filepath.Join(server.cfg.AppDataDir, "recordings", "permission", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir recording file: %v", err)
	}
	if err := os.WriteFile(path, []byte("recording-data"), 0o600); err != nil {
		t.Fatalf("write recording file: %v", err)
	}
	return path
}

func performDVRRouteRequest(server *Server, user User, method string, path string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	server.handleDVR(rec, req, user)
	return rec
}

func TestDVRGuideGenerationRebindsScheduledDecisionsAtomically(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Truncate(time.Second)
	stamp := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, active_import_generation, created_at, updated_at)
		VALUES ('src_dvr_generation', 'DVR Generation', 'm3u', 1, 'generation-old', ?, ?)`, stamp, stamp); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, name, stream_url, enabled, last_seen_at, created_at, updated_at, import_generation)
		VALUES ('channel_dvr_generation', 'src_dvr_generation', 'DVR Channel', 'https://media.example.test/dvr.m3u8', 1, ?, ?, ?, 'generation-old')`, stamp, stamp, stamp); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recording_rules (id, user_id, profile_id, source_id, title, match_type, revision, created_at, updated_at)
		VALUES ('rule_dvr_generation', ?, ?, 'src_dvr_generation', 'DVR Rule', 'single', 1, ?, ?)`, user.ID, viewerProfileID(user), stamp, stamp); err != nil {
		t.Fatalf("insert rule: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_recordings (id, rule_id, user_id, profile_id, source_id, channel_id, title, status, starts_at, ends_at, revision, created_at, updated_at)
		VALUES ('recording_dvr_generation_scheduled', 'rule_dvr_generation', ?, ?, 'src_dvr_generation', 'channel_dvr_generation', 'Scheduled DVR', 'scheduled', ?, ?, 1, ?, ?),
		       ('recording_dvr_generation_running', 'rule_dvr_generation', ?, ?, 'src_dvr_generation', 'channel_dvr_generation', 'Running DVR', 'running', ?, ?, 1, ?, ?)`,
		user.ID, viewerProfileID(user), now.Add(time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), stamp, stamp,
		user.ID, viewerProfileID(user), now.Add(time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), stamp, stamp); err != nil {
		t.Fatalf("insert recordings: %v", err)
	}
	var initialRuleGeneration, initialScheduledGeneration, initialRunningGeneration string
	if err := server.db.QueryRow(`SELECT guide_generation FROM live_tv_recording_rules WHERE id = 'rule_dvr_generation'`).Scan(&initialRuleGeneration); err != nil {
		t.Fatalf("read initial rule generation: %v", err)
	}
	if err := server.db.QueryRow(`SELECT guide_generation FROM live_tv_recordings WHERE id = 'recording_dvr_generation_scheduled'`).Scan(&initialScheduledGeneration); err != nil {
		t.Fatalf("read initial scheduled generation: %v", err)
	}
	if err := server.db.QueryRow(`SELECT guide_generation FROM live_tv_recordings WHERE id = 'recording_dvr_generation_running'`).Scan(&initialRunningGeneration); err != nil {
		t.Fatalf("read initial running generation: %v", err)
	}
	if initialRuleGeneration != "generation-old" || initialScheduledGeneration != "generation-old" || initialRunningGeneration != "generation-old" {
		t.Fatalf("insert bindings rule=%q scheduled=%q running=%q", initialRuleGeneration, initialScheduledGeneration, initialRunningGeneration)
	}

	source, err := server.getLiveTVSourceRecord("src_dvr_generation")
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	if err := server.storeLiveTVImport(source, []liveTVChannelImport{{
		ID:          "channel_dvr_generation",
		Name:        "DVR Channel",
		StreamURL:   "https://media.example.test/dvr-new.m3u8",
		ProviderKey: "dvr-channel",
	}}, []liveTVProgramImport{{
		ID:        "program_dvr_generation",
		ChannelID: "channel_dvr_generation",
		Title:     "DVR Guide",
		StartAt:   now.Format(time.RFC3339),
		EndAt:     now.Add(time.Hour).Format(time.RFC3339),
	}}); err != nil {
		t.Fatalf("store refreshed guide: %v", err)
	}
	var activeGeneration, ruleGeneration, scheduledGeneration, runningGeneration string
	var ruleRevision, scheduledRevision, runningRevision int64
	if err := server.db.QueryRow(`SELECT active_import_generation FROM live_tv_sources WHERE id = 'src_dvr_generation'`).Scan(&activeGeneration); err != nil {
		t.Fatalf("read active generation: %v", err)
	}
	if err := server.db.QueryRow(`SELECT guide_generation, revision FROM live_tv_recording_rules WHERE id = 'rule_dvr_generation'`).Scan(&ruleGeneration, &ruleRevision); err != nil {
		t.Fatalf("read rebound rule: %v", err)
	}
	if err := server.db.QueryRow(`SELECT guide_generation, revision FROM live_tv_recordings WHERE id = 'recording_dvr_generation_scheduled'`).Scan(&scheduledGeneration, &scheduledRevision); err != nil {
		t.Fatalf("read rebound scheduled recording: %v", err)
	}
	if err := server.db.QueryRow(`SELECT guide_generation, revision FROM live_tv_recordings WHERE id = 'recording_dvr_generation_running'`).Scan(&runningGeneration, &runningRevision); err != nil {
		t.Fatalf("read historical running recording: %v", err)
	}
	if activeGeneration == "" || activeGeneration == "generation-old" || ruleGeneration != activeGeneration || scheduledGeneration != activeGeneration {
		t.Fatalf("scheduled decisions were not rebound: active=%q rule=%q scheduled=%q", activeGeneration, ruleGeneration, scheduledGeneration)
	}
	if ruleRevision != 2 || scheduledRevision != 2 {
		t.Fatalf("rebound revisions rule=%d scheduled=%d, expected 2", ruleRevision, scheduledRevision)
	}
	if runningGeneration != "generation-old" || runningRevision != 1 {
		t.Fatalf("in-flight decision was rewritten: generation=%q revision=%d", runningGeneration, runningRevision)
	}
	if _, _, err := server.loadDVRRecordingForRun("recording_dvr_generation_running"); err == nil {
		t.Fatal("historical in-flight recording was runnable after its guide generation became inactive")
	}
}

func dvrTestUser(t *testing.T, server *Server) User {
	t.Helper()
	var existingOwnerID string
	if err := server.db.QueryRow(`SELECT id FROM users WHERE role = 'owner' ORDER BY created_at, id LIMIT 1`).Scan(&existingOwnerID); err == nil {
		owner, err := server.getUser(existingOwnerID)
		if err != nil {
			t.Fatalf("load existing DVR test owner: %v", err)
		}
		owner.ProfileID = existingOwnerID
		owner.ProfileIsPrimary = true
		return owner
	}
	now := time.Now().UTC().Format(time.RFC3339)
	permissions, err := json.Marshal(ownerPermissions())
	if err != nil {
		t.Fatalf("encode owner permissions: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_test', 'tester', 'tester@example.test', 'Tester', 'hash', 'owner', ?, '{}', ?, ?)
		ON CONFLICT(id) DO UPDATE SET role = excluded.role, permissions_json = excluded.permissions_json, updated_at = excluded.updated_at`, string(permissions), now, now); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return User{ID: "usr_test", AccountID: "usr_test", ProfileID: "usr_test", ProfileIsPrimary: true, Username: "tester", Email: "tester@example.test", DisplayName: "Tester", Role: "owner", Permissions: ownerPermissions(), LibraryIDs: []string{"lib_movies", "lib_tv", "lib_music", "lib_recorded_tv"}}
}

func insertDVRTestChannel(t *testing.T, server *Server, sourceID, channelID, nowText string) {
	t.Helper()
	if _, err := server.db.Exec(`
		INSERT INTO live_tv_channels (id, source_id, name, stream_url, enabled, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING`, channelID, sourceID, channelID, "https://media.example.test/"+channelID+".m3u8", nowText, nowText, nowText); err != nil {
		t.Fatalf("insert DVR test channel %s: %v", channelID, err)
	}
}

func TestDeleteDVRRuleCancelsFutureRowsAndPreservesRunningRecording(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Truncate(time.Second)
	stamp := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_sources (id,name,type,created_at,updated_at) VALUES ('src_rule_delete','Rule delete','m3u',?,?)`, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO live_tv_recording_rules (id,user_id,profile_id,source_id,title,created_at,updated_at) VALUES ('rule_delete_atomic',?,?, 'src_rule_delete','Rule',?,?)`, user.ID, viewerProfileID(user), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, status string }{{"scheduled_delete_atomic", "scheduled"}, {"running_delete_atomic", "running"}} {
		if _, err := server.db.Exec(`INSERT INTO live_tv_recordings (id,rule_id,user_id,profile_id,source_id,title,status,starts_at,ends_at,created_at,updated_at) VALUES (?,'rule_delete_atomic',?,?,'src_rule_delete',?,?,?, ?,?,?)`, row.id, user.ID, viewerProfileID(user), row.id, row.status, now.Add(time.Hour).Format(time.RFC3339), now.Add(2*time.Hour).Format(time.RFC3339), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.deleteDVRRule("rule_delete_atomic"); err != nil {
		t.Fatal(err)
	}
	var scheduled, running int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_recordings WHERE id='scheduled_delete_atomic'`).Scan(&scheduled); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM live_tv_recordings WHERE id='running_delete_atomic' AND status='running' AND rule_id IS NULL`).Scan(&running); err != nil {
		t.Fatal(err)
	}
	if scheduled != 0 || running != 1 {
		t.Fatalf("scheduled=%d running=%d", scheduled, running)
	}
}

func TestDVRCommandOutputWatchdogStopsSilentProducer(t *testing.T) {
	output := filepath.Join(t.TempDir(), "silent.ts")
	cmd := exec.Command("sh", "-c", "sleep 10")
	started := time.Now()
	err := runDVRCommandWithOutputWatchdog(context.Background(), cmd, output, 100*time.Millisecond)
	if !errors.Is(err, errDVROutputStalled) {
		t.Fatalf("watchdog error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("watchdog took %s", elapsed)
	}
}
