package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/workpolicy"
)

func testService(t *testing.T) (*Service, string, *sql.DB) {
	t.Helper()
	state := t.TempDir()
	db, err := persistence.Open(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(state, db, "srv-test", "test-version", persistence.SchemaVersion()), state, db
}

func fileHashOrEmpty(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// checkpointDB merges the WAL so later byte comparisons are stable.
func checkpointDB(t *testing.T, path string) {
	t.Helper()
	handle, err := dbwork.OpenHandle(path, dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if _, err = handle.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
}

func openReadOnly(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func checkOK(t *testing.T, db *sql.DB) {
	t.Helper()
	var line string
	rows, err := db.Query(`PRAGMA integrity_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		if line != "ok" {
			t.Fatalf("integrity_check: %s", line)
		}
		count++
	}
	if err = rows.Err(); err != nil || count != 1 {
		t.Fatalf("integrity_check rows: %d %v", count, err)
	}
}

// A backup during concurrent writes: writers never wait long, and the copy
// passes integrity_check.
func TestBackupDuringConcurrentWrites(t *testing.T) {
	svc, state, db := testService(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO configuration(key,value) VALUES('backup_write_probe','0') ON CONFLICT(key) DO UPDATE SET value='0'`); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var maxWait atomic.Int64
	var writes atomic.Int64
	go func() {
		n := 0
		for {
			select {
			case <-done:
				return
			default:
			}
			start := time.Now()
			_, err := dbwork.ExecWrite(ctx, db, dbwork.ClassInteractive, `UPDATE configuration SET value=? WHERE key='backup_write_probe'`, fmt.Sprint(n))
			wait := time.Since(start)
			if wait > time.Duration(maxWait.Load()) {
				maxWait.Store(int64(wait))
			}
			if err != nil {
				return
			}
			n++
			writes.Add(1)
		}
	}()
	id, err := svc.Create(ctx, KindManual)
	close(done)
	if err != nil {
		t.Fatal(err)
	}
	if writes.Load() == 0 {
		t.Fatal("writer loop never ran during the backup")
	}
	if max := time.Duration(maxWait.Load()); max > 250*time.Millisecond {
		t.Fatalf("writer waited %v during backup", max)
	}
	checkOK(t, openReadOnly(t, filepath.Join(state, BackupsDir, id, DatabaseName)))
	var probe string
	if err = openReadOnly(t, filepath.Join(state, BackupsDir, id, DatabaseName)).QueryRow(`SELECT value FROM configuration WHERE key='backup_write_probe'`).Scan(&probe); err != nil {
		t.Fatal(err)
	}
}

// Backups list newest first, delete removes, and prune keeps the newest keep
// scheduled and manual backups plus the newest three pre-restore folders.
func TestListDeletePrune(t *testing.T) {
	svc, state, _ := testService(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 20, 3, 0, 0, 0, time.UTC)
	n := 0
	svc.now = func() time.Time {
		n++
		return base.Add(time.Duration(n) * time.Minute)
	}
	manual := []string{}
	for i := 0; i < 3; i++ {
		id, err := svc.Create(ctx, KindManual)
		if err != nil {
			t.Fatal(err)
		}
		manual = append(manual, id)
	}
	scheduled := []string{}
	for i := 0; i < 2; i++ {
		id, err := svc.CreateKind(ctx, KindScheduled)
		if err != nil {
			t.Fatal(err)
		}
		scheduled = append(scheduled, id)
	}
	// Four pre-restore folders, older than the backups above.
	for i := 0; i < 4; i++ {
		id := base.Add(-time.Duration(i+1)*time.Hour).Format(timestampLayout) + "-pre-restore"
		dir := filepath.Join(state, BackupsDir, id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		manifest := Manifest{Format: Format, Version: Version, ServerID: "srv-test", SchemaVersion: 1, CreatedAt: base.Add(-time.Duration(i+1) * time.Hour).Format(time.RFC3339), Kind: KindPreRestore}
		raw, _ := json.Marshal(manifest)
		if err := os.WriteFile(filepath.Join(dir, ManifestName), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 9 {
		t.Fatalf("listed %d backups", len(listed))
	}
	for i := 1; i < len(listed); i++ {
		if listed[i-1].ID < listed[i].ID {
			t.Fatal("not newest first")
		}
	}
	if err = svc.Delete(manual[0]); err != nil {
		t.Fatal(err)
	}
	if err = svc.Delete("2026-01-01T000000Z"); err != ErrNotFound {
		t.Fatalf("unknown id removed: %v", err)
	}
	if err = svc.Delete("../escape"); err != ErrInvalid {
		t.Fatalf("traversal accepted: %v", err)
	}
	if err = svc.Prune(2); err != nil {
		t.Fatal(err)
	}
	listed, err = svc.List()
	if err != nil {
		t.Fatal(err)
	}
	kept, keptPre := 0, 0
	for _, info := range listed {
		if info.Kind == KindPreRestore {
			keptPre++
			continue
		}
		kept++
	}
	// Two manual and two scheduled existed; one manual was deleted, so three
	// remain and prune keeps the newest two. Pre-restore keeps three of four.
	if kept != 2 || keptPre != 3 {
		t.Fatalf("prune kept %d + %d pre-restore", kept, keptPre)
	}
}

// Start is idempotent per operation id; a finished job replays its id.
func TestStartIdempotent(t *testing.T) {
	svc, _, _ := testService(t)
	ctx := context.Background()
	first, err := svc.Start(ctx, KindManual, "op-12345678")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Start(ctx, KindManual, "op-12345678")
	if err != nil || second != first {
		t.Fatalf("replay returned %q, %v", second, err)
	}
	job, ok := svc.Job(first)
	if !ok {
		t.Fatal("job missing")
	}
	if err = job.Result(); err != nil {
		t.Fatal(err)
	}
	if _, ok = svc.Running(); ok {
		t.Fatal("finished job still running")
	}
	if _, err = svc.Start(ctx, KindManual, "short"); err != ErrInvalid {
		t.Fatalf("bad operation id accepted: %v", err)
	}
	if _, err = svc.Start(ctx, KindManual, "op-12345678"); err != nil {
		t.Fatal(err)
	}
}

// Stage by id, by folder path and by bare database; error cases map to the
// published codes.
func TestStageSources(t *testing.T) {
	svc, state, db := testService(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO configuration(key,value) VALUES('stage_probe','here') ON CONFLICT(key) DO UPDATE SET value='here'`); err != nil {
		t.Fatal(err)
	}
	id, err := svc.Create(ctx, KindManual)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := svc.Stage(ctx, Source{BackupID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !staged.Staged || !staged.RestartRequired || staged.Validation.Integrity != "ok" || staged.Validation.SchemaVersion != persistence.SchemaVersion() {
		t.Fatalf("bad stage result: %+v", staged)
	}
	marker, err := os.ReadFile(filepath.Join(state, RestoreStagedDir, RestoreMarkerFile))
	if err != nil || !bytes.Contains(marker, []byte(`"state": "staged"`)) {
		t.Fatal("marker missing")
	}
	_ = os.RemoveAll(filepath.Join(state, RestoreStagedDir))

	folder := filepath.Join(state, BackupsDir, id)
	staged, err = svc.Stage(ctx, Source{Path: folder})
	if err != nil {
		t.Fatal(err)
	}
	if !staged.Staged {
		t.Fatal("folder source not staged")
	}
	_ = os.RemoveAll(filepath.Join(state, RestoreStagedDir))

	bare := filepath.Join(t.TempDir(), "bare.db")
	checkpointDB(t, filepath.Join(state, "server.sqlite"))
	raw, err := os.ReadFile(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(bare, raw, 0600); err != nil {
		t.Fatal(err)
	}
	staged, err = svc.Stage(ctx, Source{Path: bare})
	if err != nil {
		t.Fatal(err)
	}
	if staged.Validation.SchemaVersion != persistence.SchemaVersion() {
		t.Fatal("bare source validation wrong")
	}
	_ = os.RemoveAll(filepath.Join(state, RestoreStagedDir))

	for _, source := range []Source{
		{},
		{BackupID: id, Path: folder},
		{BackupID: "nope"},
		{BackupID: "../escape"},
		{Path: "relative.db"},
		{Path: filepath.Join(state, "missing.db")},
		{Path: t.TempDir()},
	} {
		if _, err = svc.Stage(ctx, source); !errors.Is(err, ErrSourceInvalid) {
			t.Fatalf("source %+v staged: %v", source, err)
		}
	}
	text := filepath.Join(t.TempDir(), "note.db")
	if err = os.WriteFile(text, []byte("not a database"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Stage(ctx, Source{Path: text}); !errors.Is(err, persistence.ErrCandidateNotPortico) {
		t.Fatalf("text file staged: %v", err)
	}
}

// A corrupt copy fails integrity; a newer schema refuses.
func TestStageValidation(t *testing.T) {
	svc, state, _ := testService(t)
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	truncated := filepath.Join(t.TempDir(), "truncated.db")
	if err = os.WriteFile(truncated, raw[:len(raw)/2], 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Stage(ctx, Source{Path: truncated}); !errors.Is(err, persistence.ErrCandidateIntegrity) {
		t.Fatalf("truncated database staged: %v", err)
	}
	flipped := filepath.Join(t.TempDir(), "flipped.db")
	mangled := append([]byte(nil), raw...)
	for i := 100; i < 356 && i < len(mangled); i++ {
		mangled[i] ^= 0xff
	}
	if err = os.WriteFile(flipped, mangled, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Stage(ctx, Source{Path: flipped}); !errors.Is(err, persistence.ErrCandidateIntegrity) {
		t.Fatalf("flipped database staged: %v", err)
	}
	newer := filepath.Join(t.TempDir(), "newer.db")
	if err = os.WriteFile(newer, raw, 0600); err != nil {
		t.Fatal(err)
	}
	handle, err := dbwork.OpenHandle(newer, dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = handle.Exec(`INSERT INTO configuration(key,value) VALUES('schema_version','999999') ON CONFLICT(key) DO UPDATE SET value='999999'`); err != nil {
		t.Fatal(err)
	}
	handle.Close()
	if _, err = svc.Stage(ctx, Source{Path: newer}); !errors.Is(err, persistence.ErrCandidateNewer) {
		t.Fatalf("newer schema staged: %v", err)
	}
}

// A full cycle stages and applies; the pre-restore copy, manifest and outcome
// are recorded, and playback state retires.
func TestApplyStagedCycle(t *testing.T) {
	svc, state, db := testService(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO configuration(key,value) VALUES('apply_probe','live') ON CONFLICT(key) DO UPDATE SET value='live'`); err != nil {
		t.Fatal(err)
	}
	id, err := svc.Create(ctx, KindManual)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Stage(ctx, Source{BackupID: id}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err = ApplyStaged(ctx, state, nil); err != nil {
		t.Fatal(err)
	}
	restored, err := persistence.Open(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var probe string
	if err = restored.QueryRow(`SELECT value FROM configuration WHERE key='apply_probe'`).Scan(&probe); err != nil || probe != "live" {
		t.Fatalf("restored probe: %q %v", probe, err)
	}
	last, err := svc.LastRestore()
	if err != nil || last == nil || last.Outcome != "restored" {
		t.Fatalf("last restore: %+v %v", last, err)
	}
	if _, err = os.Stat(filepath.Join(state, RestoreStagedDir)); !os.IsNotExist(err) {
		t.Fatal("staged folder left behind")
	}
	pres, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	pre := 0
	for _, info := range pres {
		if info.Kind == KindPreRestore {
			pre++
			manifest, err := readManifest(filepath.Join(state, BackupsDir, info.ID))
			if err != nil || manifest.Kind != KindPreRestore {
				t.Fatal("pre-restore manifest wrong")
			}
		}
	}
	if pre != 1 {
		t.Fatalf("pre-restore folders: %d", pre)
	}
}

// A failing verification rolls back to the pre-restore copy and records it.
func TestApplyStagedRollback(t *testing.T) {
	svc, state, db := testService(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO configuration(key,value) VALUES('rollback_probe','live') ON CONFLICT(key) DO UPDATE SET value='live'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	checkpointDB(t, filepath.Join(state, "server.sqlite"))
	beforeDB := fileHashOrEmpty(t, filepath.Join(state, "server.sqlite"))
	beforeWAL := fileHashOrEmpty(t, filepath.Join(state, "server.sqlite-wal"))
	db, err := persistence.Open(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc.db = db
	id, err := svc.Create(ctx, KindManual)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Stage(ctx, Source{BackupID: id}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	injected := errors.New("injected migration failure")
	if err = ApplyStaged(ctx, state, func(context.Context, string) error { return injected }); !errors.Is(err, injected) {
		t.Fatalf("apply returned %v", err)
	}
	if after := fileHashOrEmpty(t, filepath.Join(state, "server.sqlite")); after != beforeDB {
		t.Fatal("database not rolled back")
	}
	if after := fileHashOrEmpty(t, filepath.Join(state, "server.sqlite-wal")); after != beforeWAL {
		t.Fatal("WAL not rolled back")
	}
	last, err := svc.LastRestore()
	if err != nil || last == nil || last.Outcome != "rolled_back" || last.Reason == "" {
		t.Fatalf("last restore: %+v %v", last, err)
	}
	entries, err := os.ReadDir(filepath.Join(state, BackupsDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if len(entry.Name()) > 12 && entry.Name()[len(entry.Name())-12:] == "-pre-restore" {
			t.Fatal("pre-restore folder left behind")
		}
	}
	if _, err = os.Stat(filepath.Join(state, RestoreStagedDir)); !os.IsNotExist(err) {
		t.Fatal("staged folder left behind")
	}
}

// A bare .db restore replaces only the database; other state files stay.
func TestApplyBareDatabaseKeepsStateFiles(t *testing.T) {
	svc, state, db := testService(t)
	ctx := context.Background()
	pemPath := filepath.Join(state, "networking-tls.pem")
	if err := os.WriteFile(pemPath, []byte("pem-bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	logoPath := filepath.Join(state, "channel-logos", "logo.png")
	if err := os.MkdirAll(filepath.Dir(logoPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logoPath, []byte("logo-bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(t.TempDir(), "bare.db")
	if err = os.WriteFile(bare, raw, 0600); err != nil {
		t.Fatal(err)
	}
	handle, err := dbwork.OpenHandle(bare, dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = handle.Exec(`INSERT INTO configuration(key,value) VALUES('bare_probe','bare') ON CONFLICT(key) DO UPDATE SET value='bare'`); err != nil {
		t.Fatal(err)
	}
	handle.Close()
	if _, err = svc.Stage(ctx, Source{Path: bare}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err = ApplyStaged(ctx, state, nil); err != nil {
		t.Fatal(err)
	}
	if pem, err := os.ReadFile(pemPath); err != nil || string(pem) != "pem-bytes" {
		t.Fatal("TLS key replaced by a bare restore")
	}
	if logo, err := os.ReadFile(logoPath); err != nil || string(logo) != "logo-bytes" {
		t.Fatal("channel logo replaced by a bare restore")
	}
	restored, err := persistence.Open(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var probe string
	if err = restored.QueryRow(`SELECT value FROM configuration WHERE key='bare_probe'`).Scan(&probe); err != nil || probe != "bare" {
		t.Fatalf("bare database not applied: %q %v", probe, err)
	}
}

// Permissions: a loosened tree warns, the fix tightens only Portico's files,
// and owner bits survive.
func TestStatePermissionsCheckAndFix(t *testing.T) {
	state := t.TempDir()
	loose := filepath.Join(state, "subtitles", "loose.srt")
	if err := os.MkdirAll(filepath.Dir(loose), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loose, []byte("subtitles"), 0644); err != nil {
		t.Fatal(err)
	}
	execPath := filepath.Join(state, "run-helper")
	if err := os.WriteFile(execPath, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(state, 0755); err != nil {
		t.Fatal(err)
	}
	exposed, err := CheckPermissions(state)
	if err != nil || !exposed {
		t.Fatalf("loose tree not reported: %v %v", exposed, err)
	}
	if err = FixPermissions(state); err != nil {
		t.Fatal(err)
	}
	exposed, err = CheckPermissions(state)
	if err != nil || exposed {
		t.Fatalf("fixed tree still exposed: %v %v", exposed, err)
	}
	for _, tc := range []struct {
		path string
		want os.FileMode
	}{
		{state, 0700},
		{filepath.Dir(loose), 0700},
		{loose, 0600},
		{execPath, 0700},
	} {
		info, err := os.Stat(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != tc.want {
			t.Fatalf("%s mode %o, want %o", tc.path, info.Mode().Perm(), tc.want)
		}
	}
}

// buildOldDatabase assembles a database at the migrations through the given
// version, with a matching ledger, the way an earlier release left it.
// buildPreReleaseDatabase writes a database as an earlier pre-release Portico
// left it: a migration ledger that runs through 265 without the baseline
// (migration 1), and data of its own.
func buildPreReleaseDatabase(t *testing.T, path string) {
	t.Helper()
	db, err := dbwork.OpenHandle(path, dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, q := range []string{
		`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,digest TEXT NOT NULL,applied_at TEXT NOT NULL)`,
		`CREATE TABLE configuration(key TEXT PRIMARY KEY,value TEXT NOT NULL)`,
		`INSERT INTO configuration(key,value) VALUES('schema_version','265'),('old_db_marker','vintaged')`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []int{16, 100, 265} {
		sum := sha256.Sum256([]byte(fmt.Sprint(v)))
		if _, err = db.Exec(`INSERT INTO schema_migrations VALUES(?,?,?,?)`, v, fmt.Sprintf("migration-%04d.sql", v), hex.EncodeToString(sum[:]), now); err != nil {
			t.Fatal(err)
		}
	}
}

// A backup made by an earlier pre-release Portico (before the baseline) is
// refused when it is staged, with the plain reset message, and the server's
// own database is left as it was.
func TestRestorePreReleaseBackupIsRefused(t *testing.T) {
	oldPath := filepath.Join(t.TempDir(), "old.sqlite")
	buildPreReleaseDatabase(t, oldPath)
	state := t.TempDir()
	db, err := persistence.Open(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO configuration(key,value) VALUES('current_marker','kept') ON CONFLICT(key) DO UPDATE SET value='kept'`); err != nil {
		t.Fatal(err)
	}
	svc := New(state, db, "srv-test", "test-version", persistence.SchemaVersion())
	if _, err = svc.Stage(context.Background(), Source{Path: oldPath}); !errors.Is(err, persistence.ErrCandidatePreRelease) {
		t.Fatalf("pre-release backup staged: %v", err)
	}
	if !strings.Contains(persistence.ErrSchemaResetRequired.Error(), "can't be upgraded") {
		t.Fatalf("refusal message: %q", persistence.ErrSchemaResetRequired)
	}
	var marker string
	if err = db.QueryRow(`SELECT value FROM configuration WHERE key='current_marker'`).Scan(&marker); err != nil || marker != "kept" {
		t.Fatalf("current database after the refusal: %q %v", marker, err)
	}
}

// The scheduler creates one backup per open occurrence, then prunes.
func TestScheduledBackupFiresOncePerOccurrence(t *testing.T) {
	svc, _, db := testService(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-30 * time.Minute)
	policy := workpolicy.Policy{BackgroundPriority: "lower", Windows: []workpolicy.Window{{
		ID: "test", Name: "Test", Enabled: true, Cadence: "custom",
		Days:            []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"},
		StartMinute:     start.Hour()*60 + start.Minute() - 60,
		DurationMinutes: 180, Timezone: "UTC", Tasks: []string{"backup"},
	}}}
	if policy.Windows[0].StartMinute < 0 {
		policy.Windows[0].StartMinute += 24 * 60
	}
	err := dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		return workpolicy.SaveTx(ctx, tx, policy)
	})
	if err != nil {
		t.Fatal(err)
	}
	sched := svc.Schedule(db, func(context.Context) (int, error) { return 7, nil })
	sched.now = func() time.Time { return now }
	sched.check(dbwork.WithClass(ctx, dbwork.ClassMaintenance))
	listed, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	scheduled := 0
	for _, info := range listed {
		if info.Kind == KindScheduled {
			scheduled++
		}
	}
	if scheduled != 1 {
		t.Fatalf("scheduled backups: %d", scheduled)
	}
	sched.check(dbwork.WithClass(ctx, dbwork.ClassMaintenance))
	listed, err = svc.List()
	if err != nil {
		t.Fatal(err)
	}
	again := 0
	for _, info := range listed {
		if info.Kind == KindScheduled {
			again++
		}
	}
	if again != 1 {
		t.Fatalf("second check created another backup: %d", again)
	}
}

// A pre-restore backup is made by moving the live files aside, so its manifest
// lists (and hashes) the database too. It must be restorable by id and by
// folder like any other backup.
func TestPreRestoreBackupCanBeStaged(t *testing.T) {
	svc, state, _ := testService(t)
	ctx := context.Background()
	checkpointDB(t, filepath.Join(state, "server.sqlite"))
	raw, err := os.ReadFile(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	id := time.Now().UTC().Format(timestampLayout) + "-pre-restore"
	folder := filepath.Join(state, BackupsDir, id)
	if err = os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(folder, DatabaseName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	hashed, err := hashTree(folder)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeManifest(folder, Manifest{Format: Format, Version: Version, SchemaVersion: persistence.SchemaVersion(), CreatedAt: "2026-09-25T00:00:00Z", Kind: KindPreRestore, Files: hashed}); err != nil {
		t.Fatal(err)
	}
	for _, source := range []Source{{BackupID: id}, {Path: folder}} {
		staged, err := svc.Stage(ctx, source)
		if err != nil || !staged.Staged {
			t.Fatalf("pre-restore backup %+v not staged: %+v %v", source, staged, err)
		}
		_ = os.RemoveAll(filepath.Join(state, RestoreStagedDir))
	}
}
