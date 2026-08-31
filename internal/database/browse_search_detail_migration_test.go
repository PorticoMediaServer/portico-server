package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestReleaseBaselineIncludesBrowseSearchDetailAndIsStableOnReopen(t *testing.T) {
	t.Chdir("../..")
	appData := t.TempDir()
	cfg := config.Config{AppDataDir: appData, DatabasePath: filepath.Join(appData, "portico.db")}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	if !databaseColumnExistsForTest(t, db, "profile_search_history", "normalized_query") {
		t.Fatal("browse/search migration omitted profile_search_history")
	}
	if !databaseColumnExistsForTest(t, db, "media_people", "canonical_person_key") {
		t.Fatal("browse/search migration omitted durable canonical person identity")
	}
	if !databaseColumnExistsForTest(t, db, "audiobook_browse_entities", "identity_key") ||
		!databaseColumnExistsForTest(t, db, "audiobook_browse_entity_members", "media_id") {
		t.Fatal("browse/search migration omitted durable audiobook browse identities")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_browse_migration', 'browse-migration', 'browse-migration@example.test', 'Browse Migration', 'user', '{}', '{}', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert migration account: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO profile_search_history (profile_id, normalized_query, query, last_used_at)
		VALUES ('usr_browse_migration', 'fargo', 'Fargo', ?)`, now); err != nil {
		t.Fatalf("insert profile search history: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, type, title, sort_title, added_at)
		VALUES ('movie_browse_migration', 'movie', 'Browse Migration Movie', 'Browse Migration Movie', ?)`, now); err != nil {
		t.Fatalf("insert migration media: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_people (id, media_id, name, role, source, canonical_person_key, created_at)
		VALUES ('person_browse_migration', 'movie_browse_migration', 'Weak Performer', 'Actor', 'manual', 'manual:performer-1', ?)`, now); err != nil {
		t.Fatalf("insert canonical person mapping: %v", err)
	}
	var canonicalKey string
	if err := db.QueryRow(`SELECT canonical_person_key FROM media_people WHERE id = 'person_browse_migration'`).Scan(&canonicalKey); err != nil || canonicalKey != "manual:performer-1" {
		t.Fatalf("canonical person key=%q err=%v", canonicalKey, err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = 'usr_browse_migration'`); err != nil {
		t.Fatalf("delete migration account: %v", err)
	}
	var retainedHistory int
	if err := db.QueryRow(`SELECT COUNT(*) FROM profile_search_history WHERE profile_id = 'usr_browse_migration'`).Scan(&retainedHistory); err != nil || retainedHistory != 0 {
		t.Fatalf("profile search history survived account cascade: count=%d err=%v", retainedHistory, err)
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
}

func TestReleaseBaselineIndexesCanonicalPeopleAndPersonalRatings(t *testing.T) {
	t.Chdir("../..")
	appData := t.TempDir()
	db, err := Open(config.Config{AppDataDir: appData, DatabasePath: filepath.Join(appData, "portico.db")})
	if err != nil {
		t.Fatalf("open release database: %v", err)
	}
	defer db.Close()
	for table, index := range map[string]string{
		"media_people":     "idx_media_people_canonical_person",
		"user_media_state": "idx_media_browse_personal_rating",
	} {
		if !sqliteIndexExists(t, db, table, index) {
			t.Fatalf("release schema omitted %s on %s", index, table)
		}
	}
}
