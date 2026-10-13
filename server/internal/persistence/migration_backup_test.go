package persistence

import (
	"context"
	"path/filepath"
	"testing"

	"portico.local/server/internal/dbwork"
)

func TestPendingMigrationCopiesWALAndPrunesSnapshots(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenFresh(filepath.Join(dir, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = Set(db, "migration-copy-probe", "present in WAL"); err != nil {
		t.Fatal(err)
	}
	steps, _ := schemaMigrations()
	steps = append(steps, schemaMigration{schemaVersion + 1, "next.sql", "CREATE TABLE next_step(id INTEGER)", "test"})
	if err = migrateWith(context.Background(), db, steps); err != nil {
		t.Fatal(err)
	}
	names, _ := filepath.Glob(filepath.Join(dir, "migration-backups", "before-*.sqlite"))
	if len(names) != 1 {
		t.Fatal(names)
	}
	copy, err := dbwork.OpenHandle(names[0], dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if Get(copy, "migration-copy-probe") != "present in WAL" {
		t.Fatal("copy omitted WAL")
	}
	var count int
	if err = copy.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='next_step'").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	copy.Close()
	for i := 0; i < 4; i++ {
		if err = backupBeforeMigrations(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	names, _ = filepath.Glob(filepath.Join(dir, "migration-backups", "before-*.sqlite"))
	if len(names) != 3 {
		t.Fatal(names)
	}
}

func TestNewDatabaseReclaimsPagesIncrementally(t *testing.T) {
	db, err := OpenFresh(filepath.Join(t.TempDir(), "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode int
	if err = db.QueryRow("PRAGMA auto_vacuum").Scan(&mode); err != nil || mode != 2 {
		t.Fatal("incremental mode", mode, err)
	}
	if _, err = db.Exec("CREATE TABLE space_fixture(body BLOB); INSERT INTO space_fixture VALUES(zeroblob(1048576)); DELETE FROM space_fixture"); err != nil {
		t.Fatal(err)
	}
	var before, after int
	db.QueryRow("PRAGMA freelist_count").Scan(&before)
	if before == 0 {
		t.Fatal("no pages to reclaim")
	}
	if err = ReclaimSpace(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	db.QueryRow("PRAGMA freelist_count").Scan(&after)
	if after >= before || before-after > 64 {
		t.Fatal("unpaced reclamation", before, after)
	}
}
