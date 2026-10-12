package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"

	"portico.local/server/internal/dbwork"
)

// A database the server can run on is more than one that opens and passes its
// integrity check. Start-up, and every service it constructs, runs statements
// against the whole schema and reads rows a fresh install always has. Nothing
// recreates those: a migration already recorded is never run again. The two
// checks here say whether a database still has them. They read only, and they
// are derived from the same sources a fresh install is built from, so a new
// table or a new seeded row is covered the day it is added.
var (
	// ErrSchemaIncomplete is a database missing a table, index, trigger or view
	// this build's migrations create.
	ErrSchemaIncomplete = errors.New("the database is missing part of the schema this version requires")
	// ErrSeededRowMissing is a database missing a row every install starts with.
	ErrSeededRowMissing = errors.New("the database is missing a row every install starts with")
)

// VerifySchemaObjects reports whether the database holds every schema object a
// fresh install of this build creates (the generated list a test keeps equal
// to a fresh install), and whether each of its tables still has every column a
// fresh install gives it. An extra object or column is not an error here; a
// missing one is. Trigger and view bodies are not compared.
func VerifySchemaObjects(ctx context.Context, db *sql.DB) error {
	rows, err := dbwork.Query(ctx, db, `SELECT type,name FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	present := map[string]bool{}
	for rows.Next() {
		var kind, name string
		if err = rows.Scan(&kind, &name); err != nil {
			return err
		}
		present[kind+" "+name] = true
	}
	if err = rows.Err(); err != nil {
		return err
	}
	rows.Close()
	required := make([]string, 0, len(present))
	for object := range parseSchemaObjects(knownSchemaObjects) {
		required = append(required, object)
	}
	sort.Strings(required)
	for _, object := range required {
		if !present[object] {
			return fmt.Errorf("%w: %s", ErrSchemaIncomplete, object)
		}
	}
	fresh, err := freshInstallColumns()
	if err != nil {
		return err
	}
	held, err := tableColumns(ctx, db)
	if err != nil {
		return err
	}
	for _, column := range fresh {
		if !held[column] {
			return fmt.Errorf("%w: column %s", ErrSchemaIncomplete, column)
		}
	}
	return nil
}

// tableColumns answers every "table.column" the database has.
func tableColumns(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := dbwork.Query(ctx, db, `SELECT m.name,c.name FROM sqlite_schema m JOIN pragma_table_xinfo(m.name) c WHERE m.type='table' AND m.name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var table, column string
		if err = rows.Scan(&table, &column); err != nil {
			return nil, err
		}
		out[table+"."+column] = true
	}
	return out, rows.Err()
}

var (
	freshColumnsOnce sync.Once
	freshColumns     []string
	freshColumnsErr  error
)

// freshInstallColumns answers every "table.column" of a fresh install. A
// column list cannot be read off the migrations' text, so the schema is
// installed once, into a throwaway file, and asked; later calls in the same
// process reuse the answer. It costs about what a first start does.
func freshInstallColumns() ([]string, error) {
	freshColumnsOnce.Do(func() {
		directory, err := os.MkdirTemp("", "portico-schema-reference-")
		if err != nil {
			freshColumnsErr = err
			return
		}
		defer os.RemoveAll(directory)
		db, err := OpenFresh(filepath.Join(directory, "reference.sqlite"))
		if err != nil {
			freshColumnsErr = err
			return
		}
		defer db.Close()
		columns, err := tableColumns(context.Background(), db)
		if err != nil {
			freshColumnsErr = err
			return
		}
		for column := range columns {
			freshColumns = append(freshColumns, column)
		}
		sort.Strings(freshColumns)
	})
	return freshColumns, freshColumnsErr
}

var (
	seededTable = regexp.MustCompile(`(?m)^INSERT INTO "?([A-Za-z0-9_]+)"?\s*(?:\([^)]*\))?\s*VALUES\b`)

	seededOnce   sync.Once
	seededTables []string
	seededErr    error
)

// ownerReplacedSeeds are seeded tables the owner's own settings replace: the
// maintenance schedule starts with one nightly window, and saving a schedule
// with none leaves these empty. Every other seeded table is only ever read or
// updated in place.
var ownerReplacedSeeds = map[string]bool{"maintenance_windows": true, "maintenance_window_days": true, "maintenance_window_tasks": true}

// SeededTables lists the tables this build's migrations populate with fixed
// rows: the settings, policies, revisions, cursors and vocabularies that exist
// from the first start. Start-up and the services it constructs read them and
// nothing recreates them. Tables the owner's settings replace are left out.
func SeededTables() ([]string, error) {
	seededOnce.Do(func() {
		steps, err := schemaMigrations()
		if err != nil {
			seededErr = err
			return
		}
		seeded := map[string]bool{}
		for _, step := range steps {
			for _, match := range seededTable.FindAllStringSubmatch(step.sql, -1) {
				seeded[match[1]] = true
			}
		}
		for name := range seeded {
			if !ownerReplacedSeeds[name] {
				seededTables = append(seededTables, name)
			}
		}
		sort.Strings(seededTables)
	})
	return seededTables, seededErr
}

// VerifySeededRows reports whether every one of those tables still holds rows.
// Most hold exactly one, pinned by their primary key, so for them this is
// whether that row is there. A missing table is ErrSchemaIncomplete's
// business; run that check first.
func VerifySeededRows(ctx context.Context, db *sql.DB) error {
	gated, err := dbwork.BeginSnapshot(ctx, db)
	if err != nil {
		return err
	}
	defer gated.Rollback()
	return verifySeededRows(ctx, gated.Tx())
}

func verifySeededRows(ctx context.Context, tx *sql.Tx) error {
	tables, err := SeededTables()
	if err != nil {
		return err
	}
	for _, table := range tables {
		// The name comes from this build's own migrations, never from a caller.
		var present bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM "`+table+`")`).Scan(&present); err != nil {
			return err
		}
		if !present {
			return fmt.Errorf("%w: %s", ErrSeededRowMissing, table)
		}
	}
	return nil
}
