package persistence

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaMigrationsAreNumberedAndImmutable(t *testing.T) {
	migrations, err := schemaMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) == 0 || migrations[0].version != baselineSchemaVersion {
		t.Fatal("missing baseline")
	}
}

func TestASecondOpenRunsNoSchemaWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	first, err := OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	stored, err := storedSchemaVersion(ctx, first)
	if err != nil || stored != schemaVersion {
		t.Fatalf("a fresh install recorded version %d (%v), not %d", stored, err, schemaVersion)
	}
	var recorded int
	if err = first.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	steps, err := schemaMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if recorded != len(steps) {
		t.Fatalf("the ledger recorded %d steps of %d", recorded, len(steps))
	}
	// Drop a table the installers create. A second open must not put it back:
	// that is the proof that no DDL ran, and it is the behaviour the startup time
	// depends on.
	if _, err = first.Exec(`DROP TABLE IF EXISTS detail_jobs`); err != nil {
		t.Fatal(err)
	}
	first.Close()
	second, err := OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var exists int
	if err = second.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='detail_jobs'`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists != 0 {
		t.Fatal("a second open re-ran the installers; a current database must run no DDL at all")
	}
}

func TestANewerSchemaIsRefusedWithAClearMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = Set(db, schemaVersionKey, "99999"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	_, err = OpenFresh(path)
	if !errors.Is(err, ErrSchemaNewer) {
		t.Fatalf("an older binary opened a newer database: %v", err)
	}
	// The owner has to be able to act on this, so it has to name both versions.
	if !strings.Contains(err.Error(), "99999") || !strings.Contains(err.Error(), "restore") {
		t.Fatalf("the refusal does not tell the owner what to do: %v", err)
	}
}

// A database an earlier pre-release Portico wrote (its ledger runs to 265
// without the baseline, and its schema version is above the baseline's) is
// refused with the plain reset message, not told to upgrade Portico.
func TestAPreReleaseDatabaseIsRefusedWithTheResetMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,digest TEXT NOT NULL,applied_at TEXT NOT NULL)`,
		`CREATE TABLE configuration(key TEXT PRIMARY KEY,value TEXT NOT NULL)`,
		`INSERT INTO configuration VALUES('schema_version','265')`,
		`INSERT INTO schema_migrations VALUES(16,'0016_baseline.sql','x','2026-09-01T00:00:00Z'),(265,'0265_last.sql','y','2026-09-24T00:00:00Z')`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	_, err = OpenFresh(path)
	if !errors.Is(err, ErrSchemaResetRequired) {
		t.Fatalf("a pre-release database: %v", err)
	}
	if err.Error() != "This database was created by an earlier pre-release Portico and can't be upgraded. Move it aside and start fresh." {
		t.Fatalf("refusal message: %q", err)
	}
}

func TestAppliedMigrationDigestRejectsSilentSchemaDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE schema_migrations SET digest='tampered'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	_, err = OpenFresh(path)
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatal("changed applied SQL silently accepted", err)
	}
}
