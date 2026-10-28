package backup

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/storagepaths"
	"testing"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

// The launch-audit fixture changed a text payload without violating SQLite's
// integrity_check. Reproduce that corruption through the production scrubber.
func TestBackupRejectsStructurallyValidDatabaseCorruption(t *testing.T) {
	for _, kind := range []Kind{KindManual, KindScheduled} {
		t.Run(string(kind), func(t *testing.T) {
			svc, state, db := testService(t)
			svc.SetCopyScrubber(identity.ScrubBackupCopy)
			if _, err := db.Exec(`INSERT INTO configuration(key,value) VALUES('launch_audit_marker','AUDIT_ORIGINAL_VALUE')`); err != nil {
				t.Fatal(err)
			}
			id, err := svc.Create(context.Background(), kind)
			if err != nil {
				t.Fatal(err)
			}
			folder := storagepaths.Path(state, BackupsDir, id)
			manifest, err := readManifest(folder)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, entry := range manifest.Files {
				found = found || entry.Name == DatabaseName
			}
			if manifest.Version != Version || !found {
				t.Fatalf("database checksum missing: %+v", manifest)
			}
			if err = checkSourceHashes(folder, manifest); err != nil {
				t.Fatalf("finalized copy hash: %v", err)
			}
			path := filepath.Join(folder, DatabaseName)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Count(raw, []byte("AUDIT_ORIGINAL_VALUE")) != 1 {
				t.Fatal("audit payload not unique")
			}
			raw = bytes.Replace(raw, []byte("AUDIT_ORIGINAL_VALUE"), []byte("BUDIT_ORIGINAL_VALUE"), 1)
			if err = writeTestFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			// Run SQLite inspection on a separate copy; inspection can checkpoint or
			// rewrite bytes, and must not alter the source whose hashes are tested.
			candidate := filepath.Join(t.TempDir(), DatabaseName)
			if err = writeTestFile(candidate, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = persistence.InspectCandidate(context.Background(), filepath.Dir(candidate), filepath.Base(candidate)); err != nil {
				t.Fatalf("corruption should be structurally valid: %v", err)
			}
			for _, source := range []Source{{BackupID: id}, {Path: folder}} {
				if _, err = svc.Stage(context.Background(), source); !errors.Is(err, ErrIntegrity) {
					t.Fatalf("corrupt source accepted: %+v %v", source, err)
				}
			}
			if _, err = os.Stat(storagepaths.Path(state, RestoreStagedDir, RestoreMarkerFile)); !os.IsNotExist(err) {
				t.Fatalf("corruption was staged: %v", err)
			}
		})
	}
}

func TestBackupDatabaseManifestVersionsAndStructuralChecks(t *testing.T) {
	for _, version := range []int{LegacyVersion, Version} {
		t.Run(itoa(version), func(t *testing.T) {
			svc, state, _ := testService(t)
			id, err := svc.Create(context.Background(), KindManual)
			if err != nil {
				t.Fatal(err)
			}
			folder := storagepaths.Path(state, BackupsDir, id)
			manifest, err := readManifest(folder)
			if err != nil {
				t.Fatal(err)
			}
			manifest.Version = version
			manifest.Files = nil // v1's ordinary backup shape
			if err = writeManifest(folder, manifest); err != nil {
				t.Fatal(err)
			}
			_, err = svc.Stage(context.Background(), Source{Path: folder})
			if version == Version {
				if !errors.Is(err, ErrSourceInvalid) {
					t.Fatalf("current format missing DB accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("legacy backup compatibility: %v", err)
			}
			// A legacy backup's missing database hash cannot waive SQLite validation.
			if err = writeTestFile(filepath.Join(folder, DatabaseName), []byte("not a SQLite database"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = svc.Stage(context.Background(), Source{Path: folder}); !errors.Is(err, persistence.ErrCandidateNotPortico) {
				t.Fatalf("legacy structural corruption accepted: %v", err)
			}
		})
	}
	// A matching current-format checksum cannot waive structural validation.
	svc, state, _ := testService(t)
	id, err := svc.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	folder := storagepaths.Path(state, BackupsDir, id)
	if err = writeTestFile(filepath.Join(folder, DatabaseName), []byte("not SQLite"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifest(folder)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := hashFile(context.Background(), folder, DatabaseName)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Files = []FileEntry{entry}
	if err = writeManifest(folder, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Stage(context.Background(), Source{Path: folder}); !errors.Is(err, persistence.ErrCandidateNotPortico) {
		t.Fatalf("hashed structural corruption accepted: %v", err)
	}
}

func TestBackupFinalizesScrubberWALBeforeHashing(t *testing.T) {
	svc, state, _ := testService(t)
	svc.SetCopyScrubber(func(ctx context.Context, copy *dbwork.Candidate) error {
		if _, err := copy.ExecContext(ctx, `INSERT INTO configuration(key,value) VALUES('wal_marker','scrubbed WAL content')`); err != nil {
			return err
		}
		if size, err := walBytes(copy.Path() + "-wal"); err != nil || size == 0 {
			t.Fatalf("scrubber WAL fixture: %d %v", size, err)
		}
		return nil // Finalization must fold the log before hashing.
	})
	id, err := svc.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	folder := storagepaths.Path(state, BackupsDir, id)
	manifest, err := readManifest(folder)
	if err != nil {
		t.Fatal(err)
	}
	if err = checkSourceHashes(folder, manifest); err != nil {
		t.Fatalf("hash changed after final SQLite close: %v", err)
	}
	if size, err := walBytes(filepath.Join(folder, DatabaseName) + "-wal"); err != nil || size != 0 {
		t.Fatalf("finalized WAL: %d %v", size, err)
	}
	if _, err = svc.Stage(context.Background(), Source{BackupID: id}); err != nil {
		t.Fatal(err)
	}
	staged := openReadOnly(t, storagepaths.Path(state, RestoreStagedDir, DatabaseName))
	var value string
	if err = staged.QueryRow(`SELECT value FROM configuration WHERE key='wal_marker'`).Scan(&value); err != nil || value != "scrubbed WAL content" {
		t.Fatalf("WAL content lost: %q %v", value, err)
	}
}

func TestBackupRejectsUnlistedWAL(t *testing.T) {
	for _, version := range []int{LegacyVersion, Version} {
		t.Run(itoa(version), func(t *testing.T) {
			svc, state, _ := testService(t)
			id, err := svc.Create(context.Background(), KindManual)
			if err != nil {
				t.Fatal(err)
			}
			folder := storagepaths.Path(state, BackupsDir, id)
			manifest, err := readManifest(folder)
			if err != nil {
				t.Fatal(err)
			}
			manifest.Version = version
			if version == LegacyVersion {
				manifest.Files = nil
			}
			if err = writeManifest(folder, manifest); err != nil {
				t.Fatal(err)
			}
			writer, err := sql.Open("sqlite", filepath.Join(folder, DatabaseName))
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			if _, err = writer.Exec(`PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; INSERT INTO configuration(key,value) VALUES('unlisted_wal','must not replay')`); err != nil {
				t.Fatal(err)
			}
			if size, err := walBytes(filepath.Join(folder, DatabaseName) + "-wal"); err != nil || size == 0 {
				t.Fatalf("missing WAL fixture: %d %v", size, err)
			}
			for _, source := range []Source{{BackupID: id}, {Path: folder}} {
				if _, err = svc.Stage(context.Background(), source); !errors.Is(err, ErrIntegrity) {
					t.Fatalf("unverified WAL accepted: %v", err)
				}
			}
		})
	}
}
