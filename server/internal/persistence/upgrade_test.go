package persistence

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"portico.local/server/internal/dbwork"
)

// schemaObjects is a database's whole schema: every table, index and trigger
// by name, with the SQL that made it.
func schemaObjects(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT type||' '||name,COALESCE(sql,'') FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, statement string
		if err = rows.Scan(&name, &statement); err != nil {
			t.Fatal(err)
		}
		out[name] = statement
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func ledger(t *testing.T, db *sql.DB) map[int]string {
	t.Helper()
	applied, err := appliedMigrations(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return applied
}

// Every server installed so far, the demo's and each tester's, holds a database
// the baseline alone wrote. This binary has to open it in place: the new
// migrations run, nothing the database held is lost, and the result is the
// schema a fresh install gets.
func TestADatabaseAtTheBaselineUpgradesInPlace(t *testing.T) {
	ctx := context.Background()
	steps, err := schemaMigrations()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "state.sqlite")
	// What a schema-1 binary does on first start: the baseline, then its wake
	// triggers.
	old, err := dbwork.OpenHandle(path, dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err = migrateWith(ctx, old, steps[:1]); err != nil {
		t.Fatal(err)
	}
	if err = dbwork.InstallWakeDependencies(ctx, old); err != nil {
		t.Fatal(err)
	}
	if v, e := storedSchemaVersion(ctx, old); e != nil || v != baselineSchemaVersion {
		t.Fatalf("the baseline recorded version %d (%v)", v, e)
	}
	// Some of what a running server holds by then.
	if err = Set(old, "upgrade-probe", "kept"); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO libraries(id,name,kind,root) VALUES('tv','TV','tv','/media/tv')`,
		`INSERT INTO admin_documents(scope,revision,body,updated_ms) VALUES('library:tv',3,'{"allowMediaDeletion":true}',1)`,
		`INSERT INTO console_documents(scope,revision,body,updated_ms) VALUES('server',2,'{"name":"Den"}',1)`,
	} {
		if _, err = old.Exec(statement); err != nil {
			t.Fatal(statement, err)
		}
	}
	var segments int
	if err = old.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name LIKE 'segment_%'`).Scan(&segments); err != nil || segments != 0 {
		t.Fatalf("the baseline already has the segment tables (%d, %v)", segments, err)
	}
	if err = old.Close(); err != nil {
		t.Fatal(err)
	}
	// A backup taken at the baseline is a valid restore candidate; it migrates
	// when the restored server starts.
	if v, e := InspectCandidate(ctx, path); e != nil || v != baselineSchemaVersion {
		t.Fatalf("a baseline backup inspected as version %d (%v)", v, e)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("this binary refused a baseline database: %v", err)
	}
	defer db.Close()
	if v, e := storedSchemaVersion(ctx, db); e != nil || v != schemaVersion {
		t.Fatalf("upgraded to version %d (%v), not %d", v, e, schemaVersion)
	}
	applied := ledger(t, db)
	for _, step := range steps {
		if applied[step.version] != step.digest {
			t.Fatalf("the ledger has migration %d as %q, not %q", step.version, applied[step.version], step.digest)
		}
	}
	if Get(db, "upgrade-probe") != "kept" {
		t.Fatal("a configuration value was lost")
	}
	var name, body, server string
	if err = db.QueryRow(`SELECT l.name,d.body,(SELECT body FROM console_documents WHERE scope='server') FROM libraries l JOIN admin_documents d ON d.scope='library:'||l.id WHERE l.id='tv'`).Scan(&name, &body, &server); err != nil || name != "TV" || body != `{"allowMediaDeletion":true}` || server != `{"name":"Den"}` {
		t.Fatalf("library and settings rows changed: %q %q %q %v", name, body, server, err)
	}
	// The new tables work, with their constraints.
	if _, err = db.Exec(`INSERT INTO segment_files(object_id,source_revision,library_id,mode,state,updated_ms) VALUES('o','r','tv','standard','pending',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO segment_files(object_id,source_revision,library_id,mode,state,updated_ms) VALUES('p','r','tv','fast','pending',1)`); err == nil {
		t.Fatal("segment_files accepted a mode it does not know")
	}
	if _, err = db.Exec(`INSERT INTO segment_shows(show_id,updated_ms) VALUES(7,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO segment_catchup(singleton) VALUES(1)`); err != nil {
		t.Fatal(err)
	}
	// The pre-migration copy holds the database as the old binary left it.
	copies, _ := filepath.Glob(filepath.Join(dir, "migration-backups", "before-*.sqlite"))
	if len(copies) != 1 {
		t.Fatalf("pre-migration copies: %v", copies)
	}
	before, err := dbwork.OpenHandle(copies[0], dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if Get(before, "upgrade-probe") != "kept" || len(ledger(t, before)) != 1 {
		t.Fatal("the pre-migration copy is not the baseline database")
	}
	before.Close()

	fresh, err := OpenFresh(filepath.Join(t.TempDir(), "fresh.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if got, want := schemaObjects(t, db), schemaObjects(t, fresh); !reflect.DeepEqual(got, want) {
		for name, statement := range want {
			if got[name] != statement {
				t.Errorf("upgraded %s differs from a fresh install's:\n%s\nwant\n%s", name, got[name], statement)
			}
		}
		for name := range got {
			if _, ok := want[name]; !ok {
				t.Errorf("upgraded database has %s, a fresh install does not", name)
			}
		}
	}
	db.Close()
	// Opening it again runs nothing.
	again, err := OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if copies, _ = filepath.Glob(filepath.Join(dir, "migration-backups", "before-*.sqlite")); len(copies) != 1 {
		t.Fatalf("a current database was copied for migration again: %v", copies)
	}
}

func TestAFreshInstallAppliesEveryMigration(t *testing.T) {
	ctx := context.Background()
	steps, err := schemaMigrations()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	db, err := OpenFresh(filepath.Join(dir, "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if v, e := storedSchemaVersion(ctx, db); e != nil || v != schemaVersion {
		t.Fatalf("a fresh install recorded version %d (%v)", v, e)
	}
	applied := ledger(t, db)
	if len(applied) != len(steps) {
		t.Fatalf("the ledger has %d migrations of %d", len(applied), len(steps))
	}
	for _, step := range steps {
		if applied[step.version] != step.digest {
			t.Fatalf("migration %d missing from a fresh install's ledger", step.version)
		}
	}
	var tables int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='table' AND name IN('segment_files','segment_shows','segment_catchup')`).Scan(&tables); err != nil || tables != 3 {
		t.Fatalf("a fresh install has %d of the segment tables (%v)", tables, err)
	}
	// A new database has nothing to protect, so it is never copied first.
	if _, err = os.Stat(filepath.Join(dir, "migration-backups")); !os.IsNotExist(err) {
		t.Fatalf("a fresh install made a pre-migration copy: %v", err)
	}
}
