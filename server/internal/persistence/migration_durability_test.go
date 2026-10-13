package persistence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationWaitsForDurableRecoveryCopyBeforePruning(t *testing.T) {
	state := t.TempDir()
	db, err := OpenFresh(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i := 0; i < 3; i++ {
		if err = backupBeforeMigrations(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	directory := filepath.Join(state, "migration-backups")
	old, err := filepath.Glob(filepath.Join(directory, "before-*.sqlite"))
	if err != nil || len(old) != 3 {
		t.Fatal(old, err)
	}
	prior := migrationBackupSyncDirectory
	t.Cleanup(func() { migrationBackupSyncDirectory = prior })
	injected := errors.New("copy publication could not be synced")
	migrationBackupSyncDirectory = func(path string) error {
		if path == directory {
			return injected
		}
		return prior(path)
	}
	steps, err := schemaMigrations()
	if err != nil {
		t.Fatal(err)
	}
	steps = append(steps, schemaMigration{schemaVersion + 1, "next.sql", "CREATE TABLE durable_upgrade_probe(id INTEGER)", "test"})
	if err = migrateWith(context.Background(), db, steps); !errors.Is(err, injected) {
		t.Fatal("migration ignored durability failure", err)
	}
	for _, name := range old {
		if _, err = os.Stat(name); err != nil {
			t.Fatal("pruned recovery copy before durable publication", err)
		}
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='durable_upgrade_probe'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("migration ran before durable publication", n, err)
	}
	migrationBackupSyncDirectory = prior
	if err = migrateWith(context.Background(), db, steps); err != nil {
		t.Fatal(err)
	}
	names, err := filepath.Glob(filepath.Join(directory, "before-*.sqlite"))
	if err != nil || len(names) != 3 {
		t.Fatal("recovery/pruning did not resume", names, err)
	}
}

func TestFailedMigrationCopyScrubRefusesPublicationAndMigration(t *testing.T) {
	state := t.TempDir()
	db, err := OpenFresh(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	refusal := errors.New("could not remove live sign-ins")
	prior := SetMigrationCopyScrubber(func(context.Context, string) error { return refusal })
	t.Cleanup(func() { SetMigrationCopyScrubber(prior) })
	steps, err := schemaMigrations()
	if err != nil {
		t.Fatal(err)
	}
	steps = append(steps, schemaMigration{schemaVersion + 1, "next.sql", "CREATE TABLE scrub_upgrade_probe(id INTEGER)", "test"})
	if err = migrateWith(context.Background(), db, steps); !errors.Is(err, refusal) {
		t.Fatal(err)
	}
	names, err := os.ReadDir(filepath.Join(state, "migration-backups"))
	if err != nil || len(names) != 0 {
		t.Fatal("unscrubbed copy left behind", names, err)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='scrub_upgrade_probe'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("migration ran after scrub failed", n, err)
	}
}
