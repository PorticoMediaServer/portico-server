package catalog

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"portico.local/server/internal/persistence"
)

// A fixture that projects a few hundred titles costs one to two seconds, and
// the same fixture is opened by dozens of tests. fixtureCopy builds each named
// fixture once per test binary, settles it, and gives every test its own byte
// copy, so each test still starts from the identical state and nothing one test
// writes is seen by another.
var fixtureTemplates struct {
	mu    sync.Mutex
	built map[string]*fixtureTemplate
}

type fixtureTemplate struct {
	once sync.Once
	raw  []byte
	err  error
}

// fixtureCopy opens a private copy of the named fixture; build writes the
// fixture into an empty current database the first time it is asked for.
func fixtureCopy(t *testing.T, name string, build func(t *testing.T, db *sql.DB)) *sql.DB {
	t.Helper()
	fixtureTemplates.mu.Lock()
	if fixtureTemplates.built == nil {
		fixtureTemplates.built = map[string]*fixtureTemplate{}
	}
	template := fixtureTemplates.built[name]
	if template == nil {
		template = &fixtureTemplate{}
		fixtureTemplates.built[name] = template
	}
	fixtureTemplates.mu.Unlock()
	template.once.Do(func() {
		dir, err := os.MkdirTemp("", "portico-fixture-")
		if err != nil {
			template.err = err
			return
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, name+".db")
		db, err := persistence.Open(path)
		if err != nil {
			template.err = err
			return
		}
		build(t, db)
		// Fold the log into the main file so one file is the whole fixture.
		if _, err = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			db.Close()
			template.err = err
			return
		}
		if err = db.Close(); err != nil {
			template.err = err
			return
		}
		template.raw, template.err = os.ReadFile(path)
	})
	if template.err != nil {
		t.Fatal(template.err)
	}
	if template.raw == nil {
		t.Fatalf("fixture %s failed to build in an earlier test", name)
	}
	path := filepath.Join(t.TempDir(), name+".db")
	if err := os.WriteFile(path, template.raw, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
