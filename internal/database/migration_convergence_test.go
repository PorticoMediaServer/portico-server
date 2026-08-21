package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestMigrationBaselineAtomicRetry(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open memory database: %v", err)
	}
	defer db.Close()
	broken := fstest.MapFS{
		"001_initial.sql": &fstest.MapFile{Data: []byte("CREATE TABLE release_fixture (id TEXT PRIMARY KEY); SELECT missing_release_function();\n")},
	}
	if err := runMigrationsOnConnection(context.Background(), db, migrationSource{FS: broken, Root: ".", Label: "broken baseline"}); err == nil {
		t.Fatal("broken baseline unexpectedly committed")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('release_fixture', 'schema_migrations', 'portico_database_identity')`).Scan(&count); err != nil {
		t.Fatalf("inspect rollback: %v", err)
	}
	if count != 0 {
		t.Fatalf("atomic rollback left %d schema objects", count)
	}
	corrected := fstest.MapFS{
		"001_initial.sql": &fstest.MapFile{Data: []byte("CREATE TABLE release_fixture (id TEXT PRIMARY KEY);\n")},
	}
	if err := runMigrationsOnConnection(context.Background(), db, migrationSource{FS: corrected, Root: ".", Label: "corrected baseline"}); err != nil {
		t.Fatalf("retry corrected baseline: %v", err)
	}
}

func TestValidIncrementalMigrationAppliesExactlyOnce(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open memory database: %v", err)
	}
	defer db.Close()
	fixture := fstest.MapFS{
		"001_initial.sql":    &fstest.MapFile{Data: []byte("CREATE TABLE release_fixture (id TEXT PRIMARY KEY, applications INTEGER NOT NULL DEFAULT 0);\n")},
		"002_apply_once.sql": &fstest.MapFile{Data: []byte("INSERT INTO release_fixture (id, applications) VALUES ('incremental', 1);\n")},
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := runMigrationsOnConnection(context.Background(), db, migrationSource{FS: fixture, Root: ".", Label: "incremental fixture"}); err != nil {
			t.Fatalf("run incremental fixture attempt %d: %v", attempt, err)
		}
	}
	var applications, ledgerRows int
	if err := db.QueryRow(`SELECT applications FROM release_fixture WHERE id = 'incremental'`).Scan(&applications); err != nil {
		t.Fatalf("read incremental fixture: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&ledgerRows); err != nil {
		t.Fatalf("read incremental ledger: %v", err)
	}
	if applications != 1 || ledgerRows != 2 {
		t.Fatalf("incremental migration applications=%d ledgerRows=%d", applications, ledgerRows)
	}
}

func TestMigrationBaselineConcurrentOpenConverges(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{AppDataDir: root, DatabasePath: filepath.Join(root, "portico.db")}
	start := make(chan struct{})
	errorsFound := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			db, err := Open(cfg)
			if db != nil {
				_ = db.Close()
			}
			errorsFound <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatalf("concurrent release open: %v", err)
		}
	}
}
