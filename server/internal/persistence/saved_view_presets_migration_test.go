package persistence

import (
	"path/filepath"
	"testing"
)

func TestSavedViewPresetIndexesUpgradeBaselineWithoutChangingSavedData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	// Reconstruct an already-installed, digest-valid baseline. The migration
	// must accept malformed legacy definitions, without rewriting them.
	for _, q := range []string{
		`DROP INDEX saved_view_library_page`,
		`DROP INDEX saved_view_pivot_page`,
		`DELETE FROM schema_migrations WHERE version=2`,
		`UPDATE configuration SET value='1' WHERE key='schema_version'`,
		`INSERT INTO saved_resources(id,kind,owner_key,owner_authority,owner_account,owner_profile,name,definition,created_at) VALUES('old-preset','view','owner','local','account','profile','Old','malformed historical definition','2026-10-18T00:00:00Z')`,
	} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	db.Close()
	db, err = OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var definition string
	var revision int
	var indexes int
	if err = db.QueryRow(`SELECT definition,revision FROM saved_resources WHERE id='old-preset'`).Scan(&definition, &revision); err != nil {
		t.Fatal(err)
	}
	if definition != "malformed historical definition" || revision != 1 {
		t.Fatal("upgrade rewrote preset", definition, revision)
	}
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='index' AND name IN ('saved_view_library_page','saved_view_pivot_page')`).Scan(&indexes); err != nil || indexes != 2 {
		t.Fatal(indexes, err)
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "migration-backups", "before-*.sqlite"))
	if len(backups) != 1 {
		t.Fatal("missing pre-upgrade rollback copy", backups)
	}
}
