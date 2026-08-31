package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

func TestSQLiteHardeningConcurrentHomeServerWorkload(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_sqlite_stress', 'stress', 'stress@example.test', 'Stress User', 'owner', '{}', '{}', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("insert stress user: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, type, title, sort_title, genres_json, added_at)
		VALUES ('movie_sqlite_stress', 'movie', 'SQLite Stress', 'SQLite Stress', '[]', ?)`,
		now); err != nil {
		t.Fatalf("insert stress media: %v", err)
	}

	root := t.TempDir()
	for i := 0; i < scannerWriteBatchSize+12; i++ {
		name := filepath.Join(root, fmt.Sprintf("Stress.Movie.%03d.2026.mp4", i))
		if err := os.WriteFile(name, []byte("not real video"), 0o600); err != nil {
			t.Fatalf("write stress media %d: %v", i, err)
		}
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "SQLite Stress", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create stress library: %v", err)
	}
	releaseMaintenanceLane, ok := server.tryAcquireJobLane("metadata_refresh")
	if !ok {
		t.Fatalf("failed to reserve maintenance job lane")
	}
	defer releaseMaintenanceLane()

	// Exclusive SQLite maintenance must yield while an interactive writer is
	// active. Exercise that deferral deterministically, then retry the backup
	// after the concurrent home-server workload has drained.
	releaseInteractiveWrite, err := server.dbWriteScheduler.acquire(t.Context(), foundationcontract.WorkClassInteractive)
	if err != nil {
		t.Fatalf("acquire interactive write pressure: %v", err)
	}
	if _, err := server.createDatabaseBackup(); err == nil || !strings.Contains(err.Error(), "deferred") {
		releaseInteractiveWrite()
		t.Fatalf("backup during interactive pressure error = %v, expected safe deferral", err)
	}
	releaseInteractiveWrite()

	var wg sync.WaitGroup
	errs := make(chan error, 80)
	var scanResult libraryScanResult
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, err := server.performLibraryScan(library, "job_sqlite_stress_scan")
		if err != nil {
			errs <- fmt.Errorf("scan: %w", err)
			return
		}
		scanResult = result
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 24; i++ {
			if err := server.setProgress("usr_sqlite_stress", "movie_sqlite_stress", i*15, false); err != nil {
				errs <- fmt.Errorf("progress %d: %w", i, err)
				return
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 12; i++ {
			if _, err := server.createJobFor("metadata_refresh", fmt.Sprintf("Stress metadata refresh %d.", i), "media", "movie_sqlite_stress"); err != nil {
				errs <- fmt.Errorf("job %d: %w", i, err)
				return
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 24; i++ {
			if _, err := server.queryMedia("", `WHERE m.title LIKE ?`, []any{"%Stress%"}); err != nil {
				errs <- fmt.Errorf("browse %d: %w", i, err)
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	backupDeadline := time.Now().Add(2 * time.Second)
	for {
		_, err := server.createDatabaseBackup()
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "deferred") || time.Now().After(backupDeadline) {
			t.Fatalf("backup retry after interactive pressure ended: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if scanResult.FilesIndexed != scannerWriteBatchSize+12 {
		t.Fatalf("scan indexed %d files, expected %d", scanResult.FilesIndexed, scannerWriteBatchSize+12)
	}
	var progress int
	if err := server.db.QueryRow(`SELECT progress_seconds FROM user_media_state WHERE user_id = 'usr_sqlite_stress' AND media_id = 'movie_sqlite_stress'`).Scan(&progress); err != nil {
		t.Fatalf("query stress progress: %v", err)
	}
	if progress == 0 {
		t.Fatalf("stress progress was not persisted")
	}
	var jobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE resource_id = 'movie_sqlite_stress'`).Scan(&jobs); err != nil {
		t.Fatalf("query stress jobs: %v", err)
	}
	if jobs != 1 {
		t.Fatalf("stress jobs persisted = %d, expected duplicate metadata refreshes to coalesce", jobs)
	}
}

func TestMusicTrackProgressUpdatesLastPlayedWithoutResumePosition(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_progress_policy', 'progress-policy', 'progress@example.test', 'Progress Policy', 'owner', '{}', '{}', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, type, title, sort_title, genres_json, added_at)
		VALUES
			('track_progress_policy', 'track', 'Replay From Start', 'Replay From Start', '[]', ?),
			('audiobook_progress_policy', 'audiobook', 'Resume Chapter', 'Resume Chapter', '[]', ?)`,
		now, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if err := server.setProgress("usr_progress_policy", "track_progress_policy", 93, true); err != nil {
		t.Fatalf("set track progress: %v", err)
	}
	if err := server.setProgress("usr_progress_policy", "audiobook_progress_policy", 93, false); err != nil {
		t.Fatalf("set audiobook progress: %v", err)
	}
	var trackProgress, trackWatched, audiobookProgress int
	var trackLastPlayed string
	if err := server.db.QueryRow(`
		SELECT progress_seconds, watched, last_played_at
		FROM user_media_state
		WHERE user_id = 'usr_progress_policy' AND media_id = 'track_progress_policy'`).Scan(&trackProgress, &trackWatched, &trackLastPlayed); err != nil {
		t.Fatalf("read track state: %v", err)
	}
	if trackProgress != 0 || trackWatched != 0 || trackLastPlayed == "" {
		t.Fatalf("track state progress=%d watched=%d lastPlayed=%q, expected progress reset with last played", trackProgress, trackWatched, trackLastPlayed)
	}
	if err := server.db.QueryRow(`
		SELECT progress_seconds
		FROM user_media_state
		WHERE user_id = 'usr_progress_policy' AND media_id = 'audiobook_progress_policy'`).Scan(&audiobookProgress); err != nil {
		t.Fatalf("read audiobook state: %v", err)
	}
	if audiobookProgress != 93 {
		t.Fatalf("audiobook progress = %d, expected 93", audiobookProgress)
	}
}

func TestAudioPlaybackSessionsSurviveMobileBackgroundGrace(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	oldSeen := now.Add(-10 * time.Minute).Format(time.RFC3339)
	veryOldSeen := now.Add(-13 * time.Hour).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_audio_restore', 'audio-restore', 'audio-restore@example.test', 'Audio Restore', 'owner', '{}', '{}', ?, ?)`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, type, title, sort_title, genres_json, added_at)
		VALUES
			('track_audio_restore', 'track', 'Background Song', 'Background Song', '[]', ?),
			('movie_audio_restore', 'movie', 'Expired Movie', 'Expired Movie', '[]', ?),
			('audiobook_audio_restore', 'audiobook', 'Background Book', 'Background Book', '[]', ?)`,
		now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, client_instance_id, state, is_live)
		VALUES
			('play_track_restore', 'usr_audio_restore', 'usr_audio_restore', 'track_audio_restore', 'track', 'Background Song', ?, ?, 'web_mobile_music', 'playing', 0),
			('play_movie_restore', 'usr_audio_restore', 'usr_audio_restore', 'movie_audio_restore', 'movie', 'Expired Movie', ?, ?, 'web_mobile_video', 'playing', 0),
			('play_book_restore', 'usr_audio_restore', 'usr_audio_restore', 'audiobook_audio_restore', 'audiobook', 'Background Book', ?, ?, 'web_mobile_book', 'paused', 0),
			('play_old_book_restore', 'usr_audio_restore', 'usr_audio_restore', 'audiobook_audio_restore', 'audiobook', 'Old Background Book', ?, ?, 'web_mobile_old_book', 'paused', 0)`,
		now.Format(time.RFC3339), oldSeen,
		now.Format(time.RFC3339), oldSeen,
		now.Format(time.RFC3339), oldSeen,
		now.Format(time.RFC3339), veryOldSeen); err != nil {
		t.Fatalf("insert sessions: %v", err)
	}
	if err := server.expireStalePlaybackSessions(now); err != nil {
		t.Fatalf("expire stale sessions: %v", err)
	}
	var trackEnded, movieEnded, bookEnded, oldBookEnded string
	if err := server.db.QueryRow(`SELECT ended_at FROM playback_sessions WHERE id = 'play_track_restore'`).Scan(&trackEnded); err != nil {
		t.Fatalf("read track session: %v", err)
	}
	if err := server.db.QueryRow(`SELECT ended_at FROM playback_sessions WHERE id = 'play_movie_restore'`).Scan(&movieEnded); err != nil {
		t.Fatalf("read movie session: %v", err)
	}
	if err := server.db.QueryRow(`SELECT ended_at FROM playback_sessions WHERE id = 'play_book_restore'`).Scan(&bookEnded); err != nil {
		t.Fatalf("read audiobook session: %v", err)
	}
	if err := server.db.QueryRow(`SELECT ended_at FROM playback_sessions WHERE id = 'play_old_book_restore'`).Scan(&oldBookEnded); err != nil {
		t.Fatalf("read old audiobook session: %v", err)
	}
	if trackEnded != "" || bookEnded != "" {
		t.Fatalf("audio sessions should remain restorable: track=%q audiobook=%q", trackEnded, bookEnded)
	}
	if movieEnded == "" || oldBookEnded == "" {
		t.Fatalf("stale non-audio or very old audio sessions should expire: movie=%q oldBook=%q", movieEnded, oldBookEnded)
	}
	sessionID, mediaID, _, _, ok, err := server.activePlaybackSessionForRestore("usr_audio_restore", "web_mobile_music", now)
	if err != nil {
		t.Fatalf("restore lookup: %v", err)
	}
	if !ok || sessionID != "play_track_restore" || mediaID != "track_audio_restore" {
		t.Fatalf("restore lookup session=%q media=%q ok=%v", sessionID, mediaID, ok)
	}
	_, _, _, _, ok, err = server.activePlaybackSessionForRestore("usr_audio_restore", "web_mobile_video", now)
	if err != nil {
		t.Fatalf("movie restore lookup: %v", err)
	}
	if ok {
		t.Fatalf("expired movie session should not restore")
	}
}

func TestPlaybackMaintenanceRevokesAuthorityForAlreadyStoppedSession(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Date(2026, 8, 30, 1, 30, 0, 0, time.UTC)
	stamp := now.Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_terminal_residue', 'terminal-residue', 'terminal-residue@example.test', 'Terminal Residue', 'owner', '{}', '{}', ?, ?);
		INSERT INTO media_items (id, type, title, sort_title, genres_json, added_at)
		VALUES ('movie_terminal_residue', 'movie', 'Terminal Residue', 'Terminal Residue', '[]', ?);
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, client_instance_id, state)
		VALUES ('play_terminal_residue', 'usr_terminal_residue', 'usr_terminal_residue', 'movie_terminal_residue', 'movie', 'Terminal Residue', ?, ?, ?, 'ios-terminal-residue', 'stopped');
		INSERT INTO playback_media_grants (id, token_hash, playback_session_id, principal_user_id, profile_id, resource_kind, resource_id, operation_classes_json, issued_at, expires_at)
		VALUES ('mgr_terminal_residue', 'grant-terminal-residue', 'play_terminal_residue', 'usr_terminal_residue', 'usr_terminal_residue', 'media', 'movie_terminal_residue', '["media"]', ?, ?);
		INSERT INTO playback_session_continuation_credentials (playback_session_id, token_hash, user_id, profile_id, client_instance_id, origin, issued_at, expires_at, absolute_expires_at, previous_valid_until)
		VALUES ('play_terminal_residue', 'continuation-terminal-residue', 'usr_terminal_residue', 'usr_terminal_residue', 'ios-terminal-residue', 'https://demo.example.test', ?, ?, ?, ?)`,
		stamp, stamp, stamp,
		stamp, stamp, stamp,
		stamp, now.Add(time.Hour).Format(time.RFC3339),
		stamp, now.Add(time.Minute*5).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), now.Add(time.Second*30).Format(time.RFC3339)); err != nil {
		t.Fatalf("seed stopped-session authority residue: %v", err)
	}
	if err := server.expireStalePlaybackSessions(now); err != nil {
		t.Fatalf("reconcile stopped-session authority: %v", err)
	}
	var activeGrants, activeContinuations int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM playback_media_grants WHERE playback_session_id = 'play_terminal_residue' AND revoked_at = ''`).Scan(&activeGrants); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM playback_session_continuation_credentials WHERE playback_session_id = 'play_terminal_residue' AND revoked_at = ''`).Scan(&activeContinuations); err != nil {
		t.Fatal(err)
	}
	if activeGrants != 0 || activeContinuations != 0 {
		t.Fatalf("stopped session retained authority: grants=%d continuations=%d", activeGrants, activeContinuations)
	}
}
