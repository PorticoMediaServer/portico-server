package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The list of tables every install starts with rows in is derived from the
// migrations' text. It is only right if it is exactly the tables a fresh
// install leaves populated, so a new seeded table can never be forgotten here:
// it either joins the list by itself or this fails until someone says the
// owner's settings replace it.
func TestSeededTablesMatchAFreshInstall(t *testing.T) {
	db, err := OpenFresh(filepath.Join(t.TempDir(), "fresh.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name,sql FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables, indexes []string
	for rows.Next() {
		var name, statement string
		if err = rows.Scan(&name, &statement); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(strings.ToUpper(statement), "CREATE VIRTUAL TABLE") {
			indexes = append(indexes, name)
			continue
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	// A search index keeps its own bookkeeping in tables named after it. SQLite
	// writes those; they are not seeds.
	owned := func(name string) bool {
		for _, index := range indexes {
			if strings.HasPrefix(name, index+"_") {
				return true
			}
		}
		return false
	}
	// Not seeds: the migration runner's own ledger, whose absence Open refuses.
	runner := map[string]bool{"schema_migrations": true}
	var populated []string
	for _, name := range tables {
		var present bool
		if err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM "` + name + `")`).Scan(&present); err != nil {
			t.Fatal(err)
		}
		if present && !runner[name] && !ownerReplacedSeeds[name] && !owned(name) {
			populated = append(populated, name)
		}
		if ownerReplacedSeeds[name] && !present {
			t.Fatalf("%s is listed as a seed the owner's settings replace, but a fresh install does not seed it", name)
		}
	}
	sort.Strings(populated)
	derived, err := SeededTables()
	if err != nil {
		t.Fatal(err)
	}
	if len(populated) < 10 || strings.Join(derived, ",") != strings.Join(populated, ",") {
		t.Fatalf("the derived list is not what a fresh install seeds:\nderived:   %v\npopulated: %v", derived, populated)
	}
	if err = VerifySeededRows(context.Background(), db); err != nil {
		t.Fatalf("a fresh install fails its own check: %v", err)
	}
	if err = VerifySchemaObjects(context.Background(), db); err != nil {
		t.Fatalf("a fresh install fails its own schema check: %v", err)
	}
}

// Each seeded row is required on its own: removing any one of them is noticed,
// and the table is named.
func TestVerifySeededRowsNamesEachMissingRow(t *testing.T) {
	db, err := OpenFresh(filepath.Join(t.TempDir(), "fresh.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	tables, err := SeededTables()
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		// Some of these rows are protected or referenced; the check is about a
		// database that has lost one, however it came to.
		if _, err = tx.Exec(`PRAGMA defer_foreign_keys=ON`); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(`DELETE FROM "` + table + `"`)
		if err != nil {
			tx.Rollback()
			t.Logf("%s cannot be emptied here (%v); covered by the derived-list test", table, err)
			continue
		}
		err = verifySeededRows(ctx, tx)
		tx.Rollback()
		if !errors.Is(err, ErrSeededRowMissing) || !strings.Contains(err.Error(), table) {
			t.Fatalf("%s: a missing row was not reported by name: %v", table, err)
		}
	}
	if err = VerifySeededRows(ctx, db); err != nil {
		t.Fatalf("the rows are back and the check still fails: %v", err)
	}
}

// Start-up runs statements against the whole schema, so a database that has
// lost any one object, of any kind, is not one the server can run on.
func TestVerifySchemaObjectsNamesAMissingObjectOfEachKind(t *testing.T) {
	ctx := context.Background()
	for _, object := range []struct{ kind, name, statement string }{
		{"table", "dvr_retention_retry", `DROP TABLE dvr_retention_retry`},
		{"index", "dvr_recordings_due", `DROP INDEX dvr_recordings_due`},
		{"trigger", "networking_identity_no_delete", `DROP TRIGGER networking_identity_no_delete`},
		{"view", "inventory_item_availability", `DROP VIEW inventory_item_availability`},
		// The table is still there under its name; a statement that names the
		// column fails all the same.
		{"column", "dvr_retention_retry.not_before_ms", `ALTER TABLE dvr_retention_retry RENAME COLUMN not_before_ms TO later_ms`},
		{"column", "managed_mounts.observed", `ALTER TABLE managed_mounts RENAME COLUMN observed TO seen`},
	} {
		t.Run(object.kind+" "+object.name, func(t *testing.T) {
			db, err := OpenFresh(filepath.Join(t.TempDir(), "fresh.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if object.kind != "column" && !KnownSchemaObjects()[object.kind+" "+object.name] {
				t.Fatalf("the fixture names an object this build does not create: %s %s", object.kind, object.name)
			}
			if _, err = db.Exec(object.statement); err != nil {
				t.Fatal(err)
			}
			err = VerifySchemaObjects(ctx, db)
			if !errors.Is(err, ErrSchemaIncomplete) || !strings.Contains(err.Error(), object.name) {
				t.Fatalf("a missing %s was not reported by name: %v", object.kind, err)
			}
		})
	}
}
