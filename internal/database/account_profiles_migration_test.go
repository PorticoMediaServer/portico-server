package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestReleaseBaselineIncludesAccountProfilesAndPreservesThemOnReopen(t *testing.T) {
	t.Chdir("../..")
	appData := t.TempDir()
	cfg := config.Config{AppDataDir: appData, DatabasePath: filepath.Join(appData, "portico.db")}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}

	for table, columns := range map[string][]string{
		"users":                         {"allow_account_profiles"},
		"profiles":                      {"account_id", "origin", "external_profile_id", "is_primary", "sort_order", "avatar_url", "restrictions_json", "pin_required", "disabled_at"},
		"native_refresh_tokens":         {"profile_id"},
		"local_profile_pin_credentials": {"profile_id", "pin_hash", "failed_attempts", "locked_until"},
	} {
		for _, column := range columns {
			if !databaseColumnExistsForTest(t, db, table, column) {
				t.Fatalf("account profile migration omitted %s.%s", table, column)
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_profile_upgrade', 'profile-upgrade', 'profile-upgrade@example.test', 'Upgrade Account', 'user', '{"playMedia":true}', '{}', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert account after migration: %v", err)
	}
	var accountID, origin string
	var primary int
	if err := db.QueryRow(`SELECT account_id, origin, is_primary FROM profiles WHERE id = 'usr_profile_upgrade'`).Scan(&accountID, &origin, &primary); err != nil {
		t.Fatalf("read trigger-created primary profile: %v", err)
	}
	if accountID != "usr_profile_upgrade" || origin != "local" || primary != 1 {
		t.Fatalf("primary profile = account %q origin %q primary %d", accountID, origin, primary)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close migrated database: %v", err)
	}

	db, err = Open(cfg)
	if err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	defer db.Close()
	var baselineRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&baselineRows); err != nil || baselineRows != len(expectedMigrationFiles) {
		t.Fatalf("release migration ledger rows=%d err=%v", baselineRows, err)
	}
	if err := db.QueryRow(`SELECT account_id FROM profiles WHERE id = 'usr_profile_upgrade'`).Scan(&accountID); err != nil || accountID != "usr_profile_upgrade" {
		t.Fatalf("primary profile was not preserved after reopen: account=%q err=%v", accountID, err)
	}
}
