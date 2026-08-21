package app

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/ffmpegsupervisor"
)

func TestPruneOptimizedVersionsAppliesRetentionAndPathSafety(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	root := filepath.Join(server.cfg.AppDataDir, "optimized", "movie_meridian")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create optimized root: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	newPath := filepath.Join(root, "universal-720p.mp4")
	oldPath := filepath.Join(root, "universal-1080p.mp4")
	if err := os.WriteFile(newPath, []byte("new"), 0o600); err != nil {
		t.Fatalf("write new file: %v", err)
	}
	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	now := time.Now().UTC()
	saveOptimizedSettings(t, db, `{"defaultProfile":"universal-720p","autoDelete":false,"retentionDays":7,"maxPerItem":1,"maxStorageMB":0}`)
	insertOptimizedVersion(t, db, "opt_new", "movie_meridian", "universal-720p", newPath, now)
	insertOptimizedVersion(t, db, "opt_old", "movie_meridian", "universal-1080p", oldPath, now.Add(-10*24*time.Hour))
	insertOptimizedVersion(t, db, "opt_missing", "movie_meridian", "universal-480p", filepath.Join(root, "missing.mp4"), now)
	insertOptimizedVersion(t, db, "opt_outside", "movie_meridian", "efficient-720p", outside, now)

	removed, err := server.pruneOptimizedVersions()
	if err != nil {
		t.Fatalf("prune optimized versions: %v", err)
	}
	if removed != 3 {
		t.Fatalf("removed = %d, expected 3", removed)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new optimized file should remain: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old optimized file should be deleted, stat err = %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file must not be deleted: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM optimized_versions WHERE media_id = 'movie_meridian'`).Scan(&count); err != nil {
		t.Fatalf("count optimized rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("optimized rows = %d, expected 1", count)
	}
}

func TestPruneOptimizedVersionsAppliesStorageCap(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	root := filepath.Join(server.cfg.AppDataDir, "optimized", "movie_meridian")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create optimized root: %v", err)
	}
	oldPath := filepath.Join(root, "universal-1080p.mp4")
	newPath := filepath.Join(root, "universal-720p.mp4")
	payload := make([]byte, 700*1024)
	if err := os.WriteFile(oldPath, payload, 0o600); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	if err := os.WriteFile(newPath, payload, 0o600); err != nil {
		t.Fatalf("write new file: %v", err)
	}
	now := time.Now().UTC()
	saveOptimizedSettings(t, db, `{"defaultProfile":"universal-720p","autoDelete":false,"retentionDays":0,"maxPerItem":5,"maxStorageMB":1}`)
	insertOptimizedVersion(t, db, "opt_storage_old", "movie_meridian", "universal-1080p", oldPath, now.Add(-time.Hour))
	insertOptimizedVersion(t, db, "opt_storage_new", "movie_meridian", "universal-720p", newPath, now)

	removed, err := server.pruneOptimizedVersions()
	if err != nil {
		t.Fatalf("prune optimized versions: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, expected 1", removed)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("oldest optimized file should be deleted, stat err = %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("newest optimized file should remain: %v", err)
	}
}

func TestPruneOptimizedVersionsPreservesRollbackWindowAndCurrentReadyArtifact(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	root := filepath.Join(server.cfg.AppDataDir, "optimized", "movie_rollback")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 700*1024)
	paths := map[string]string{
		"ready_current":    filepath.Join(root, "ready-current.mp4"),
		"superseded_young": filepath.Join(root, "superseded-young.mp4"),
		"superseded_old":   filepath.Join(root, "superseded-old.mp4"),
	}
	for id, path := range paths {
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatalf("write %s: %v", id, err)
		}
	}
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib-rollback', 'Rollback', 'movie', ?);
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at) VALUES ('movie_rollback', 'lib-rollback', 'movie', 'Rollback', 'Rollback', ?)`, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	saveOptimizedSettings(t, db, `{"defaultProfile":"universal-720p","autoDelete":false,"retentionDays":1,"maxPerItem":1,"maxStorageMB":1}`)
	insertOptimizedVersion(t, db, "superseded_young", "movie_rollback", "universal-720p", paths["superseded_young"], now.Add(-30*24*time.Hour))
	if _, err := db.Exec(`UPDATE optimized_versions SET state = 'superseded', superseded_at = ? WHERE id = 'superseded_young'`, now.Add(-167*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	insertOptimizedVersion(t, db, "superseded_old", "movie_rollback", "universal-720p", paths["superseded_old"], now.Add(-30*24*time.Hour))
	if _, err := db.Exec(`UPDATE optimized_versions SET state = 'superseded', superseded_at = ? WHERE id = 'superseded_old'`, now.Add(-169*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	insertOptimizedVersion(t, db, "ready_current", "movie_rollback", "universal-720p", paths["ready_current"], now)

	removed, err := server.pruneOptimizedVersions()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want only the expired rollback artifact", removed)
	}
	for _, id := range []string{"ready_current", "superseded_young"} {
		if _, err := os.Stat(paths[id]); err != nil {
			t.Fatalf("protected %s artifact was pruned: %v", id, err)
		}
	}
	if _, err := os.Stat(paths["superseded_old"]); !os.IsNotExist(err) {
		t.Fatalf("expired superseded artifact survived pruning: %v", err)
	}
}

func TestOptimizedVersionSettingsUseCustomStorageDirectory(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	customRoot := filepath.Join(t.TempDir(), "optimized-root")
	saveOptimizedSettings(t, db, `{"defaultProfile":"universal-720p","preferOptimizedPlayback":true,"storageDirectory":"`+filepath.ToSlash(customRoot)+`","maxConcurrentJobs":1,"autoDelete":false,"retentionDays":0,"maxPerItem":3,"maxStorageMB":0}`)

	settings := server.optimizedVersionSettings()
	if !settings.PreferOptimizedPlayback {
		t.Fatalf("prefer optimized playback should be enabled")
	}
	if settings.StorageDirectory != customRoot {
		t.Fatalf("storage directory = %q, expected %q", settings.StorageDirectory, customRoot)
	}
	if got := server.optimizedVersionStorageDir(); got != customRoot {
		t.Fatalf("optimized storage root = %q, expected %q", got, customRoot)
	}
	if !server.optimizedVersionPathAllowed(filepath.Join(customRoot, "movie_meridian", "universal-720p.mp4")) {
		t.Fatalf("custom optimized storage path should be allowed")
	}
	if !server.optimizedVersionPathAllowed(filepath.Join(server.cfg.AppDataDir, "optimized", "movie_meridian", "universal-720p.mp4")) {
		t.Fatalf("default optimized storage path should remain allowed for existing versions")
	}
	if server.optimizedVersionPathAllowed(filepath.Join(t.TempDir(), "outside.mp4")) {
		t.Fatalf("unmanaged optimized path should not be allowed")
	}
}

func TestCreateOptimizedVersionUsesCustomStorageDirectory(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	tempDir := t.TempDir()
	customRoot := filepath.Join(tempDir, "optimized-root")
	mediaRoot := filepath.Join(tempDir, "media")
	if err := os.MkdirAll(mediaRoot, 0o700); err != nil {
		t.Fatalf("create media root: %v", err)
	}
	sourcePath := filepath.Join(mediaRoot, "source.mp4")
	if err := os.WriteFile(sourcePath, []byte("source"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at) VALUES ('lib_optimize_custom', 'Movies', 'movie', 1, ?, '{}', ?)`, mediaRoot, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO library_paths (id, library_id, path, sort_order, created_at) VALUES ('lp_optimize_custom', 'lib_optimize_custom', ?, 0, ?)`, mediaRoot, now); err != nil {
		t.Fatalf("insert library path: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES ('movie_optimize_custom', 'lib_optimize_custom', 'movie', 'Custom Storage', 'Custom Storage', ?, ?, 120)`, now, sourcePath); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	saveOptimizedSettings(t, db, `{"defaultProfile":"universal-720p","storageDirectory":"`+filepath.ToSlash(customRoot)+`","maxConcurrentJobs":1,"autoDelete":false,"retentionDays":0,"maxPerItem":3,"maxStorageMB":0}`)
	server.cfg.FFmpegPath = writeOptimizedToolStub(t, tempDir, "ffmpeg-stub", "#!/bin/sh\nprintf optimized\n")
	server.cfg.FFprobePath = writeOptimizedToolStub(t, tempDir, "ffprobe-stub", "#!/bin/sh\ncat <<'JSON'\n{\"format\":{\"format_name\":\"mp4\",\"duration\":\"120\",\"bit_rate\":\"1000000\"},\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\",\"width\":1280,\"height\":720,\"sample_aspect_ratio\":\"1:1\",\"field_order\":\"progressive\",\"pix_fmt\":\"yuv420p\",\"color_primaries\":\"bt709\",\"color_transfer\":\"bt709\",\"color_space\":\"bt709\"},{\"codec_type\":\"audio\",\"codec_name\":\"aac\",\"channels\":2,\"channel_layout\":\"stereo\"}]}\nJSON\n")
	server.ffmpegSupervisor = newTranscodeSupervisorV2(context.Background(), ffmpegsupervisor.Config{})
	t.Cleanup(func() { _ = server.ffmpegSupervisor.Shutdown(context.Background()) })

	version, err := server.createOptimizedVersion(context.Background(), "job_custom_storage", MediaItem{
		ID:              "movie_optimize_custom",
		LibraryID:       "lib_optimize_custom",
		Type:            "movie",
		Title:           "Custom Storage",
		SourceURL:       sourcePath,
		DurationSeconds: 120,
		Streams: []Stream{
			{ID: "video", Kind: "video", Codec: "h264", Width: 1280, Height: 720},
			{ID: "audio", Kind: "audio", Codec: "aac", Channels: 2, ChannelLayout: "stereo", SampleRate: 48000},
		},
	}, "universal-720p")
	if err != nil {
		t.Fatalf("create optimized version: %v", err)
	}
	if !strings.HasPrefix(version.Path, customRoot+string(filepath.Separator)) {
		t.Fatalf("optimized path = %q, expected under %q", version.Path, customRoot)
	}
	if _, err := os.Stat(version.Path); err != nil {
		t.Fatalf("optimized output missing: %v", err)
	}
}

func TestOptimizedOutputLooksPlayableRequiresMediaStream(t *testing.T) {
	if !optimizedOutputLooksPlayable(ffprobePayload{Format: ffprobeFormat{Duration: "120"}, Streams: []ffprobeStream{{CodecType: "video"}}}, 120) {
		t.Fatalf("video stream should be playable")
	}
	if !optimizedOutputLooksPlayable(ffprobePayload{Format: ffprobeFormat{Duration: "120"}, Streams: []ffprobeStream{{CodecType: "audio"}}}, 120) {
		t.Fatalf("audio stream should be playable")
	}
	if optimizedOutputLooksPlayable(ffprobePayload{Format: ffprobeFormat{Duration: "120"}, Streams: []ffprobeStream{{CodecType: "subtitle"}}}, 120) {
		t.Fatalf("subtitle-only output should not be considered playable")
	}
	if optimizedOutputLooksPlayable(ffprobePayload{Format: ffprobeFormat{Duration: "5"}, Streams: []ffprobeStream{{CodecType: "video"}}}, 120) {
		t.Fatalf("very short partial output should not be considered playable")
	}
}

func TestOptimizedVersionJobClaimHonorsConcurrentLimit(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	saveOptimizedSettings(t, db, `{"defaultProfile":"universal-720p","maxConcurrentJobs":2,"autoDelete":false,"retentionDays":0,"maxPerItem":3,"maxStorageMB":0}`)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range []string{"job_optimize_one", "job_optimize_two", "job_optimize_three"} {
		if _, err := db.Exec(`
			INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json, created_at, updated_at)
			VALUES (?, 'optimize_version', 'queued', 0, 'Queued optimized version.', 'media', ?, '{"profile":"universal-720p"}', ?, ?)`,
			id, strings.TrimPrefix(id, "job_optimize_"), now, now); err != nil {
			t.Fatalf("insert optimize job %s: %v", id, err)
		}
	}

	if !server.claimJobForRun("job_optimize_one") {
		t.Fatalf("first optimized job should claim")
	}
	if !server.claimJobForRun("job_optimize_two") {
		t.Fatalf("second optimized job should claim")
	}
	if server.claimJobForRun("job_optimize_three") {
		t.Fatalf("third optimized job should wait at the configured concurrency limit")
	}

	var running int
	if err := db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'optimize_version' AND status = 'running'`).Scan(&running); err != nil {
		t.Fatalf("count optimized jobs: %v", err)
	}
	if running != 2 {
		t.Fatalf("running optimized jobs = %d, expected 2", running)
	}
}

func saveOptimizedSettings(t *testing.T, db *sql.DB, raw string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'optimizedVersions'`, raw); err != nil {
		t.Fatalf("save optimized settings: %v", err)
	}
}

func writeOptimizedToolStub(t *testing.T, dir string, name string, script string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func insertOptimizedVersion(t *testing.T, db *sql.DB, id string, mediaID string, profile string, path string, updatedAt time.Time) {
	t.Helper()
	timestamp := updatedAt.UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO optimized_versions (id, media_id, profile, path, size_bytes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, mediaID, profile, path, 1, timestamp, timestamp); err != nil {
		t.Fatalf("insert optimized version %s: %v", id, err)
	}
}
