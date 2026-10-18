package persistence

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"portico.local/server/internal/dbwork"
)

func presetSchema2(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := dbwork.OpenHandle(path, dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	steps, err := schemaMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if err = migrateWith(context.Background(), db, steps[:2]); err != nil {
		t.Fatal(err)
	}
	return db, path
}

func presetExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(query, err)
	}
}

func TestSavedViewPickerUpgradeBackfillsCanonicalDataAndRollsBackAtomically(t *testing.T) {
	db, path := presetSchema2(t)
	owner := PersonalOwnerKey("local", "owner", "profile")
	shared := PersonalOwnerKey("hosted", "a\"<雪", "p:other")
	presetExec(t, db, `INSERT INTO saved_resources(id,kind,owner_key,owner_authority,owner_account,owner_profile,name,definition,created_at) VALUES('legacy','view',?,'local','owner','profile','Same','{"libraryId":"movies","pivot":"movies"}','old'),('malformed','view',?,'local','owner','profile','Malformed','{broken','old')`, owner, owner)
	presetExec(t, db, `INSERT INTO saved_resource_shares VALUES('legacy','hosted','a"<雪','p:other','editor')`)
	presetExec(t, db, `INSERT INTO saved_pins VALUES(?,'view','legacy',1,7)`, shared)
	presetExec(t, db, `INSERT INTO saved_pin_order VALUES(?,'view','legacy',4)`, shared)
	steps, err := schemaMigrations()
	if err != nil {
		t.Fatal(err)
	}
	// A failed backfill/install must leave schema 2 and all canonical rows intact.
	broken := append([]schemaMigration(nil), steps...)
	broken[2].sql += `; INSERT INTO missing_picker_migration_probe VALUES(1);`
	if err = migrateWith(context.Background(), db, broken); err == nil {
		t.Fatal("accepted failed migration")
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name LIKE 'saved_view_picker%'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial index installation", count, err)
	}
	if version, err := storedSchemaVersion(context.Background(), db); err != nil || version != 2 {
		t.Fatal(version, err)
	}
	// Reopen through the real migration boundary, then verify the derived rows.
	db.Close()
	db, err = OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var key, library, pivot, name, pinKey string
	var pinned int
	if err = db.QueryRow(`SELECT owner_key,library_id,pivot,name_key,pinned,pin_key FROM saved_view_picker WHERE resource_id='legacy' AND owner_key=?`, shared).Scan(&key, &library, &pivot, &name, &pinned, &pinKey); err != nil {
		t.Fatal(err)
	}
	if key != shared || library != "movies" || pivot != "movies" || name != "same" || pinned != 1 || pinKey != "00000004\x1fsame" {
		t.Fatal(key, library, pivot, name, pinned, pinKey)
	}
	if err = db.QueryRow(`SELECT count(*) FROM saved_view_picker`).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	var definition string
	var revision int
	if err = db.QueryRow(`SELECT definition,revision FROM saved_resources WHERE id='malformed'`).Scan(&definition, &revision); err != nil || definition != "{broken" || revision != 1 {
		t.Fatal("canonical data changed", definition, revision, err)
	}
	if version, err := storedSchemaVersion(context.Background(), db); err != nil || version != 3 {
		t.Fatal(version, err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "migration-backups", "before-*.sqlite"))
	if err != nil || len(backups) == 0 {
		t.Fatal(backups, err)
	}
	// Every pre-upgrade copy is usable by the preceding schema-2 binary.
	for _, backup := range backups {
		copy, err := sql.Open(dbwork.DriverName, backup)
		if err != nil {
			t.Fatal(err)
		}
		if version, err := storedSchemaVersion(context.Background(), copy); err != nil || version != 2 {
			t.Fatal(version, err)
		}
		if err = copy.QueryRow(`SELECT count(*) FROM saved_resources`).Scan(&count); err != nil || count != 2 {
			t.Fatal(count, err)
		}
		copy.Close()
	}
}

func TestSavedViewPickerTracksCanonicalMutationsAndQualifiedActors(t *testing.T) {
	db, err := OpenFresh(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner := PersonalOwnerKey("local", "owner", "profile")
	share := PersonalOwnerKey("hosted", "a\"<雪", "p:other")
	other := PersonalOwnerKey("local", "a\"<雪", "p:other")
	presetExec(t, db, `INSERT INTO saved_resources(id,kind,owner_key,owner_authority,owner_account,owner_profile,name,definition,created_at) VALUES('preset','view',?,'local','owner','profile','Original','{"libraryId":"library","pivot":"movies"}','old')`, owner)
	assert := func(want int, ownerKey, library, pivot, name, pinKey string, pinned int) {
		t.Helper()
		var count int
		if err = db.QueryRow(`SELECT count(*) FROM saved_view_picker`).Scan(&count); err != nil || count != want {
			t.Fatal("audiences", count, want, err)
		}
		// Symmetric equality proves every derived audience agrees after each write.
		if err = db.QueryRow(`SELECT count(*) FROM (SELECT * FROM (SELECT * FROM saved_view_picker EXCEPT SELECT * FROM saved_view_picker_source) UNION ALL SELECT * FROM (SELECT * FROM saved_view_picker_source EXCEPT SELECT * FROM saved_view_picker))`).Scan(&count); err != nil || count != 0 {
			t.Fatal("stale projection", count, err)
		}
		if want == 0 {
			return
		}
		var l, p, n, k string
		var b int
		if err = db.QueryRow(`SELECT library_id,pivot,name_key,pin_key,pinned FROM saved_view_picker WHERE owner_key=? AND resource_id='preset'`, ownerKey).Scan(&l, &p, &n, &k, &b); err != nil {
			t.Fatal(err)
		}
		if l != library || p != pivot || n != name || k != pinKey || b != pinned {
			t.Fatal(l, p, n, k, b)
		}
	}
	assert(1, owner, "library", "movies", "original", "99999999\x1foriginal", 0)
	// Pin and arrange before receiving a share: grant/backfill must pick them up.
	presetExec(t, db, `INSERT INTO saved_pins VALUES(?,'view','preset',1,7)`, share)
	presetExec(t, db, `INSERT INTO saved_pin_order VALUES(?,'view','preset',2)`, share)
	presetExec(t, db, `INSERT INTO saved_resource_shares VALUES('preset','hosted','a"<雪','p:other','viewer')`)
	assert(2, share, "library", "movies", "original", "00000002\x1foriginal", 1)
	presetExec(t, db, `INSERT INTO saved_resource_shares VALUES('preset','local','a"<雪','p:other','viewer')`)
	assert(3, other, "library", "movies", "original", "99999999\x1foriginal", 0)
	// A grant to the owner never creates a duplicate or removes ownership on revoke.
	presetExec(t, db, `INSERT INTO saved_resource_shares VALUES('preset','local','owner','profile','viewer')`)
	assert(3, owner, "library", "movies", "original", "99999999\x1foriginal", 0)
	presetExec(t, db, `DELETE FROM saved_resource_shares WHERE account_id='owner'`)
	assert(3, owner, "library", "movies", "original", "99999999\x1foriginal", 0)
	presetExec(t, db, `UPDATE saved_resource_shares SET role='editor' WHERE authority='hosted'`)
	assert(3, share, "library", "movies", "original", "00000002\x1foriginal", 1)
	presetExec(t, db, `UPDATE saved_resources SET name='Renamed',definition='{"libraryId":"elsewhere","pivot":"shows","sort":[]}' WHERE id='preset'`)
	assert(3, share, "elsewhere", "shows", "renamed", "00000002\x1frenamed", 1)
	presetExec(t, db, `UPDATE saved_pin_order SET position=8 WHERE owner_key=?`, share)
	assert(3, share, "elsewhere", "shows", "renamed", "00000008\x1frenamed", 1)
	presetExec(t, db, `DELETE FROM saved_pin_order WHERE owner_key=?`, share)
	assert(3, share, "elsewhere", "shows", "renamed", "99999999\x1frenamed", 1)
	presetExec(t, db, `INSERT INTO saved_pin_order VALUES(?,'view','preset',0)`, share)
	assert(3, share, "elsewhere", "shows", "renamed", "00000000\x1frenamed", 1)
	presetExec(t, db, `UPDATE saved_pins SET pinned=0 WHERE owner_key=?`, share)
	assert(3, share, "elsewhere", "shows", "renamed", "00000000\x1frenamed", 0)
	presetExec(t, db, `UPDATE saved_pins SET pinned=1 WHERE owner_key=?`, share)
	assert(3, share, "elsewhere", "shows", "renamed", "00000000\x1frenamed", 1)
	presetExec(t, db, `DELETE FROM saved_pins WHERE owner_key=?`, share)
	assert(3, share, "elsewhere", "shows", "renamed", "00000000\x1frenamed", 0)
	presetExec(t, db, `UPDATE saved_resources SET definition='not JSON' WHERE id='preset'`)
	assert(0, "", "", "", "", "", 0)
	presetExec(t, db, `UPDATE saved_resources SET definition='{"libraryId":"library","pivot":"movies"}' WHERE id='preset'`)
	assert(3, owner, "library", "movies", "renamed", "99999999\x1frenamed", 0)
	presetExec(t, db, `DELETE FROM saved_resource_shares WHERE authority='hosted'`)
	assert(2, other, "library", "movies", "renamed", "99999999\x1frenamed", 0)
	presetExec(t, db, `UPDATE saved_resource_shares SET profile_id='new-profile' WHERE authority='local'`)
	assert(2, PersonalOwnerKey("local", "a\"<雪", "new-profile"), "library", "movies", "renamed", "99999999\x1frenamed", 0)
	presetExec(t, db, `UPDATE saved_resources SET deleted=1 WHERE id='preset'`)
	assert(0, "", "", "", "", "", 0)
	presetExec(t, db, `UPDATE saved_resources SET deleted=0 WHERE id='preset'`)
	assert(2, owner, "library", "movies", "renamed", "99999999\x1frenamed", 0)
	presetExec(t, db, `DELETE FROM saved_resources WHERE id='preset'`)
	assert(0, "", "", "", "", "", 0)
}

func TestSavedViewPickerMaintenanceUsesResourceSeeks(t *testing.T) {
	db, err := OpenFresh(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT * FROM saved_view_picker_source WHERE resource_id IN (?,?)`, "one-resource", "one-resource")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, line := range plan {
		if strings.HasPrefix(line, "SCAN ") && line != "SCAN saved_view_picker_source" || strings.Contains(line, "AUTOMATIC") || strings.Contains(line, "saved_resource_visibility") {
			t.Fatal("per-resource maintenance scans canonical rows", strings.Join(plan, "\n"))
		}
	}
	t.Log(strings.Join(plan, "\n"))
}

func TestSavedViewPickerPreservesHistoricalScopeTypes(t *testing.T) {
	db, err := OpenFresh(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner := PersonalOwnerKey("local", "owner", "profile")
	for id, definition := range map[string]string{
		"numeric-library": `{"libraryId":1,"pivot":"movies"}`,
		"numeric-pivot":   `{"libraryId":"1","pivot":1}`,
		"null-pivot":      `{"libraryId":"1","pivot":null}`,
		"text-scope":      `{"libraryId":"1","pivot":"1"}`,
		"malformed":       `not JSON`,
		"null-library":    `{"libraryId":null}`,
	} {
		presetExec(t, db, `INSERT INTO saved_resources(id,kind,owner_key,owner_authority,owner_account,owner_profile,name,definition,created_at) VALUES(?,'view',?,'local','owner','profile','Name',?,'old')`, id, owner, definition)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM saved_view_picker WHERE owner_key=? AND library_id=?`, owner, "1").Scan(&count); err != nil || count != 3 {
		t.Fatal("library scope changed JSON type", count, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM saved_view_picker WHERE owner_key=? AND library_id=? AND pivot=?`, owner, "1", "1").Scan(&count); err != nil || count != 1 {
		t.Fatal("pivot scope changed JSON type", count, err)
	}
}

func TestSavedViewPickerRestoreRequiresSchemaButNotDerivedSeedRows(t *testing.T) {
	db, err := OpenFresh(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seeded, err := SeededTables()
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(seeded, "saved_view_picker") || !slices.Contains(seeded, "saved_resource_generation") {
		t.Fatal("derived data confused with fixed seeds", seeded)
	}
	if err = VerifySeededRows(context.Background(), db); err != nil {
		t.Fatal("empty picker rejected", err)
	}
	// Fixed revision/configuration seeds remain mandatory.
	presetExec(t, db, `DELETE FROM saved_resource_generation`)
	if err = VerifySeededRows(context.Background(), db); !errors.Is(err, ErrSeededRowMissing) || !strings.Contains(err.Error(), "saved_resource_generation") {
		t.Fatal("lost fixed-seed check", err)
	}
	presetExec(t, db, `INSERT INTO saved_resource_generation(id,revision) VALUES(1,0)`)
	// Empty derived data does not mean its physical schema is optional.
	presetExec(t, db, `DROP TABLE saved_view_picker`)
	if err = VerifySchemaObjects(context.Background(), db); !errors.Is(err, ErrSchemaIncomplete) || !strings.Contains(err.Error(), "saved_view_picker") {
		t.Fatal("lost derived schema check", err)
	}
}
