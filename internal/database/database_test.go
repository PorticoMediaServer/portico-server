package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	sqlite3 "modernc.org/sqlite/lib"
)

func databaseColumnExistsForTest(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("inspect %s columns: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan %s column: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	return false
}

func TestMediaAvailabilityReadModelMaintainsFileCounts(t *testing.T) {
	t.Chdir("../..")

	appData := t.TempDir()
	cfg := config.Config{
		AppDataDir:     appData,
		DatabasePath:   filepath.Join(appData, "portico.db"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	mediaID := "movie_meridian"
	var initialFiles, initialAvailable, initialMissing int
	if err := db.QueryRow(`SELECT file_count, available_file_count, missing_file_count FROM media_availability WHERE media_id = ?`, mediaID).Scan(&initialFiles, &initialAvailable, &initialMissing); err != nil {
		t.Fatalf("query initial availability: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, available, first_seen_at, last_seen_at)
		VALUES ('availability_test_file', ?, 'lib_movies', '/tmp/availability-test.mp4', 1, ?, ?)`,
		mediaID, now, now); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	assertAvailabilityCounts(t, db, mediaID, initialFiles+1, initialAvailable+1, initialMissing)
	if _, err := db.Exec(`UPDATE media_files SET available = 0, missing_since = ? WHERE id = 'availability_test_file'`, now); err != nil {
		t.Fatalf("mark media file missing: %v", err)
	}
	assertAvailabilityCounts(t, db, mediaID, initialFiles+1, initialAvailable, initialMissing+1)
	if _, err := db.Exec(`DELETE FROM media_files WHERE id = 'availability_test_file'`); err != nil {
		t.Fatalf("delete media file: %v", err)
	}
	assertAvailabilityCounts(t, db, mediaID, initialFiles, initialAvailable, initialMissing)
}

func TestMediaAvailabilityReadModelMaintainsSourceAndHDRFlags(t *testing.T) {
	t.Chdir("../..")

	appData := t.TempDir()
	cfg := config.Config{
		AppDataDir:     appData,
		DatabasePath:   filepath.Join(appData, "portico.db"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, added_at)
		VALUES ('availability_flags_movie', 'lib_movies', 'movie', 'Availability Flags', 'Availability Flags', '/media/flags.mkv', ?)`, now); err != nil {
		t.Fatalf("insert media item: %v", err)
	}
	assertAvailabilityFlags(t, db, "availability_flags_movie", 1, 0, 0)

	if _, err := db.Exec(`
		UPDATE media_items
		SET source_url = 'https://cdn.example.test/flags.mkv'
		WHERE id = 'availability_flags_movie'`); err != nil {
		t.Fatalf("update media source: %v", err)
	}
	assertAvailabilityFlags(t, db, "availability_flags_movie", 0, 1, 0)

	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, width, height, display_title)
		VALUES ('availability_flags_video_stream', 'availability_flags_movie', 'video', 'hevc', 3840, 2160, 'HEVC 4K')`); err != nil {
		t.Fatalf("insert media stream: %v", err)
	}
	assertAvailabilityFlags(t, db, "availability_flags_movie", 0, 1, 1)

	assertAvailabilityFlags(t, db, "availability_flags_movie", 0, 1, 1)
}

func TestMediaSourceAndStreamFilterIndexesExist(t *testing.T) {
	t.Chdir("../..")

	appData := t.TempDir()
	cfg := config.Config{
		AppDataDir:     appData,
		DatabasePath:   filepath.Join(appData, "portico.db"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for table, index := range map[string]string{
		"media_files":   "idx_media_files_media_source_profile",
		"media_streams": "idx_streams_media_kind",
	} {
		if !sqliteIndexExists(t, db, table, index) {
			t.Fatalf("expected %s on %s", index, table)
		}
	}
}

func sqliteIndexExists(t *testing.T, db *sql.DB, table, name string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA index_list(` + table + `)`)
	if err != nil {
		t.Fatalf("list indexes for %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var indexName string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &indexName, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan index row for %s: %v", table, err)
		}
		if indexName == name {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate indexes for %s: %v", table, err)
	}
	return false
}

func TestOpenDefersPlannerStatisticsMaintenance(t *testing.T) {
	t.Chdir("../..")

	appData := t.TempDir()
	cfg := config.Config{
		AppDataDir:     appData,
		DatabasePath:   filepath.Join(appData, "portico.db"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	var statTableCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'sqlite_stat1'`).Scan(&statTableCount); err != nil {
		t.Fatalf("query planner statistics table: %v", err)
	}
	if statTableCount != 0 {
		t.Fatalf("Open created sqlite_stat1 during blocking startup; expected maintenance to be deferred")
	}
	if err := maintainSQLiteAfterOpen(db); err != nil {
		t.Fatalf("run deferred sqlite maintenance: %v", err)
	}
	var statsRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_stat1`).Scan(&statsRows); err != nil {
		t.Fatalf("query deferred planner statistics: %v", err)
	}
	var checkpointed, logPages, checkpointedPages int
	if err := db.QueryRow(`PRAGMA wal_checkpoint(PASSIVE)`).Scan(&checkpointed, &logPages, &checkpointedPages); err != nil {
		t.Fatalf("query passive wal checkpoint: %v", err)
	}
	if checkpointed != 0 {
		t.Fatalf("passive wal checkpoint busy flag = %d, expected 0; log=%d checkpointed=%d", checkpointed, logPages, checkpointedPages)
	}
}

func TestOpenReportsStartupPhases(t *testing.T) {
	t.Chdir("../..")

	appData := t.TempDir()
	cfg := config.Config{
		AppDataDir:     appData,
		DatabasePath:   filepath.Join(appData, "portico.db"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	}
	var phases []StartupPhase
	db, err := OpenWithReporter(cfg, func(phase StartupPhase) {
		phases = append(phases, phase)
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	required := map[string]bool{
		"sqlite_open":    false,
		"sqlite_pragmas": false,
		"sqlite_migrate": false,
		"sqlite_seed":    false,
	}
	for _, phase := range phases {
		if _, ok := required[phase.ID]; !ok {
			continue
		}
		required[phase.ID] = true
		if phase.Label == "" || phase.StartedAt.IsZero() || phase.CompletedAt.IsZero() {
			t.Fatalf("startup phase missing timing metadata: %#v", phase)
		}
		if phase.Duration < 0 {
			t.Fatalf("startup phase duration was negative: %#v", phase)
		}
		if phase.Error != "" {
			t.Fatalf("startup phase unexpectedly failed: %#v", phase)
		}
	}
	for id, seen := range required {
		if !seen {
			t.Fatalf("startup phase %s was not reported; phases=%#v", id, phases)
		}
	}
}

func assertAvailabilityCounts(t *testing.T, db *sql.DB, mediaID string, files, available, missing int) {
	t.Helper()
	var gotFiles, gotAvailable, gotMissing int
	if err := db.QueryRow(`SELECT file_count, available_file_count, missing_file_count FROM media_availability WHERE media_id = ?`, mediaID).Scan(&gotFiles, &gotAvailable, &gotMissing); err != nil {
		t.Fatalf("query availability counts: %v", err)
	}
	if gotFiles != files || gotAvailable != available || gotMissing != missing {
		t.Fatalf("availability counts = files:%d available:%d missing:%d, expected files:%d available:%d missing:%d", gotFiles, gotAvailable, gotMissing, files, available, missing)
	}
}

func assertAvailabilityFlags(t *testing.T, db *sql.DB, mediaID string, local, remote, hdr int) {
	t.Helper()
	var gotLocal, gotRemote, gotHDR int
	if err := db.QueryRow(`
		SELECT has_local_source, has_remote_source, has_hdr_source
		FROM media_availability
		WHERE media_id = ?`, mediaID).Scan(&gotLocal, &gotRemote, &gotHDR); err != nil {
		t.Fatalf("query availability flags: %v", err)
	}
	if gotLocal != local || gotRemote != remote || gotHDR != hdr {
		t.Fatalf("availability flags = local:%d remote:%d hdr:%d, expected local:%d remote:%d hdr:%d",
			gotLocal, gotRemote, gotHDR, local, remote, hdr)
	}
}

func assertMediaSortKeys(t *testing.T, db *sql.DB, mediaID string, artist, albumArtist, label string) {
	t.Helper()
	var gotArtist, gotAlbumArtist, gotLabel, gotFilterArtist, gotFilterAlbumArtist, gotFilterLabel string
	if err := db.QueryRow(`SELECT sort_artist_key, sort_album_artist_key, sort_label_key, filter_artist_key, filter_album_artist_key, filter_label_key FROM media_items WHERE id = ?`, mediaID).Scan(&gotArtist, &gotAlbumArtist, &gotLabel, &gotFilterArtist, &gotFilterAlbumArtist, &gotFilterLabel); err != nil {
		t.Fatalf("query media sort keys: %v", err)
	}
	if gotArtist != artist || gotAlbumArtist != albumArtist || gotLabel != label {
		t.Fatalf("sort keys = artist:%q albumArtist:%q label:%q, expected artist:%q albumArtist:%q label:%q", gotArtist, gotAlbumArtist, gotLabel, artist, albumArtist, label)
	}
	if gotFilterArtist != artist || gotFilterAlbumArtist != albumArtist || gotFilterLabel != label {
		t.Fatalf("filter keys = artist:%q albumArtist:%q label:%q, expected artist:%q albumArtist:%q label:%q", gotFilterArtist, gotFilterAlbumArtist, gotFilterLabel, artist, albumArtist, label)
	}
}

func TestClassifySQLiteErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{name: "busy text", err: errString("database is locked (5) (SQLITE_BUSY)"), want: ErrorKindBusy},
		{name: "locked text", err: errString("database table is locked (6) (SQLITE_LOCKED)"), want: ErrorKindLocked},
		{name: "constraint text", err: errString("constraint failed (19) (SQLITE_CONSTRAINT)"), want: ErrorKindConstraint},
		{name: "corrupt text", err: errString("database disk image is malformed (11) (SQLITE_CORRUPT)"), want: ErrorKindCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.err); got != tt.want {
				t.Fatalf("ClassifyError() = %q, expected %q", got, tt.want)
			}
		})
	}
	if primarySQLiteCode(sqlite3.SQLITE_CONSTRAINT_UNIQUE) != sqlite3.SQLITE_CONSTRAINT {
		t.Fatalf("extended constraint code did not normalize to primary constraint code")
	}
}

func TestExecWithRetryWaitsForSQLiteLock(t *testing.T) {
	appData := t.TempDir()
	dbPath := filepath.Join(appData, "retry.db")
	db1, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		t.Fatalf("open db1: %v", err)
	}
	defer db1.Close()
	db2, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(1)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open db2: %v", err)
	}
	defer db2.Close()
	if _, err := db1.Exec(`CREATE TABLE writes (id TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	tx, err := db1.Begin()
	if err != nil {
		t.Fatalf("begin lock transaction: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO writes (id, value) VALUES ('held', 'held')`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("hold write lock: %v", err)
	}
	if _, err := ExecWithRetry(context.Background(), db2, RetryOptions{Attempts: 1}, `INSERT INTO writes (id, value) VALUES ('blocked', 'blocked')`); !IsRetryableLock(err) {
		_ = tx.Rollback()
		t.Fatalf("expected retryable lock error, got %v", err)
	}
	release := make(chan struct{})
	go func() {
		time.Sleep(120 * time.Millisecond)
		_ = tx.Commit()
		close(release)
	}()
	_, stats, err := ExecWithRetryStats(context.Background(), db2, RetryOptions{Attempts: 6, Base: 40 * time.Millisecond, Max: 250 * time.Millisecond}, `INSERT INTO writes (id, value) VALUES ('retry', 'ok')`)
	<-release
	if err != nil {
		t.Fatalf("retry insert: %v", err)
	}
	if stats.Retries == 0 || stats.Wait <= 0 || stats.Attempts < 2 {
		t.Fatalf("retry stats did not record lock wait: %#v", stats)
	}
	var value string
	if err := db1.QueryRow(`SELECT value FROM writes WHERE id = 'retry'`).Scan(&value); err != nil {
		t.Fatalf("query retry row: %v", err)
	}
	if value != "ok" {
		t.Fatalf("retry row value = %q, expected ok", value)
	}
}

type errString string

func (e errString) Error() string {
	return string(e)
}
