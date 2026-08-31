package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestDefaultSQLiteResourcePolicyIsBounded(t *testing.T) {
	policy := sqliteResourcePolicyForConfig(config.Config{RestoreMaxDatabaseBytes: 64 << 30})
	if err := policy.validate(); err != nil {
		t.Fatalf("default SQLite resource policy is invalid: %v", err)
	}
	if policy.MaxOpenConns != sqliteSafeMaxOpenConns {
		t.Fatalf("max open connections = %d, expected %d", policy.MaxOpenConns, sqliteSafeMaxOpenConns)
	}
	if policy.MaxIdleConns != sqliteSafeMaxIdleConns {
		t.Fatalf("max idle connections = %d, expected %d", policy.MaxIdleConns, sqliteSafeMaxIdleConns)
	}
	if policy.MaxIdleConns > policy.MaxOpenConns {
		t.Fatalf("max idle connections = %d exceeds max open connections = %d", policy.MaxIdleConns, policy.MaxOpenConns)
	}
	if policy.CacheSizeKiB != sqliteSafeCacheSizeKiB {
		t.Fatalf("cache size = %d KiB, expected %d KiB", policy.CacheSizeKiB, sqliteSafeCacheSizeKiB)
	}
	if int64(policy.MaxOpenConns)*policy.CacheSizeKiB > int64(sqliteSafeMaxOpenConns)*int64(sqliteSafeCacheSizeKiB) {
		t.Fatalf("configured page-cache ceiling exceeds bounded preset: open=%d cache=%d KiB", policy.MaxOpenConns, policy.CacheSizeKiB)
	}
	if policy.MmapSizeBytes != sqliteSafeMmapSizeBytes {
		t.Fatalf("mmap size = %d bytes, expected disabled value %d", policy.MmapSizeBytes, sqliteSafeMmapSizeBytes)
	}

	invalid := []struct {
		name   string
		policy sqliteResourcePolicy
	}{
		{name: "too many open connections", policy: sqliteResourcePolicy{MaxOpenConns: sqliteSafeMaxOpenConns + 1, MaxIdleConns: 1, CacheSizeKiB: 1, ConnMaxLifetime: 1}},
		{name: "too many idle connections", policy: sqliteResourcePolicy{MaxOpenConns: 1, MaxIdleConns: sqliteSafeMaxIdleConns + 1, CacheSizeKiB: 1, ConnMaxLifetime: 1}},
		{name: "cache too large", policy: sqliteResourcePolicy{MaxOpenConns: 1, MaxIdleConns: 1, CacheSizeKiB: sqliteSafeCacheSizeKiB + 1, ConnMaxLifetime: 1}},
		{name: "mmap enabled", policy: sqliteResourcePolicy{MaxOpenConns: 1, MaxIdleConns: 1, CacheSizeKiB: 1, MmapSizeBytes: 1, ConnMaxLifetime: 1}},
		{name: "missing lifetime", policy: sqliteResourcePolicy{MaxOpenConns: 1, MaxIdleConns: 1, CacheSizeKiB: 1}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if err := test.policy.validate(); err == nil {
				t.Fatal("invalid SQLite resource policy unexpectedly passed validation")
			}
		})
	}
}

func TestSQLiteResourcePolicyIsEffectiveOnRuntimeHandle(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		AppDataDir:   root,
		DatabasePath: filepath.Join(root, "portico.db"),
	}
	db, err := OpenRuntimeHandle(cfg)
	if err != nil {
		t.Fatalf("open runtime SQLite handle: %v", err)
	}
	defer db.Close()

	policy := sqliteResourcePolicyForConfig(cfg)
	stats := db.Stats()
	if stats.MaxOpenConnections != policy.MaxOpenConns {
		t.Fatalf("effective max open connections = %d, expected %d", stats.MaxOpenConnections, policy.MaxOpenConns)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("pin runtime SQLite connection: %v", err)
	}
	defer conn.Close()
	var cacheSize, mmapSize int64
	if err := conn.QueryRowContext(context.Background(), `PRAGMA cache_size`).Scan(&cacheSize); err != nil {
		t.Fatalf("read effective SQLite cache size: %v", err)
	}
	if err := conn.QueryRowContext(context.Background(), `PRAGMA mmap_size`).Scan(&mmapSize); err != nil {
		t.Fatalf("read effective SQLite mmap size: %v", err)
	}
	if cacheSize != -int64(policy.CacheSizeKiB) {
		t.Fatalf("effective cache size = %d KiB, expected %d KiB", cacheSize, -policy.CacheSizeKiB)
	}
	if mmapSize != policy.MmapSizeBytes {
		t.Fatalf("effective mmap size = %d bytes, expected %d", mmapSize, policy.MmapSizeBytes)
	}
}

func TestMigrationRestoresBoundedSQLiteResourcePolicy(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "portico.db")
	db, err := sql.Open("sqlite", sqliteDSN(databasePath))
	if err != nil {
		t.Fatalf("open SQLite migration handle: %v", err)
	}
	defer db.Close()

	if err := migrateWithPathContext(context.Background(), db, databasePath); err != nil {
		t.Fatalf("run SQLite migrations: %v", err)
	}

	policy := sqliteResourcePolicyForConfig(config.Config{})
	stats := db.Stats()
	if stats.MaxOpenConnections != policy.MaxOpenConns {
		t.Fatalf("post-migration max open connections = %d, expected %d", stats.MaxOpenConnections, policy.MaxOpenConns)
	}
	if stats.Idle > policy.MaxIdleConns {
		t.Fatalf("post-migration idle connections = %d, exceeds %d", stats.Idle, policy.MaxIdleConns)
	}
	var migrationRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationRows); err != nil {
		t.Fatalf("read migration ledger after migration: %v", err)
	}
	if migrationRows != len(expectedMigrationFiles) {
		t.Fatalf("migration ledger rows = %d, expected %d", migrationRows, len(expectedMigrationFiles))
	}
}
