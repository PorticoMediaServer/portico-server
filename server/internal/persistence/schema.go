package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
)

// A new installation applies one baseline atomically. There are no upgrade
// shims for earlier development schemas. Subsequent changes add a numbered
// migration; applied SQL is immutable and verified against its recorded digest.
//
// schemaVersion is the highest migration number this binary embeds. The
// configuration key schema_version records the highest number a database has
// applied: it answers "is this database newer than me?" and "is it at least the
// baseline?". Which migrations have run is the schema_migrations set, not that
// number, because numbers arrive out of order (see schemaMigrations).
const baselineSchemaVersion = 1
const schemaVersion = 10
const schemaVersionKey = "schema_version"

var ErrSchemaNewer = errors.New("this database was written by a newer version of Portico")

// ErrSchemaResetRequired refuses a database written before the baseline was
// squashed (its ledger lacks the baseline migration). Its message is the whole
// explanation an owner sees.
var ErrSchemaResetRequired = errors.New("This database was created by an earlier pre-release Portico and can't be upgraded. Move it aside and start fresh.")

// Open returns a handle to the state database. A database already at this
// binary's schema version is opened and returned without a single DDL statement.
func Open(path string) (*sql.DB, error) {
	db, handled, err := openFromTemplate(path)
	if !handled {
		db, err = OpenFresh(path)
	}
	if err != nil {
		return nil, err
	}
	if err = dropEmptyTableStatistics(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// OpenFresh opens the database and brings its schema up to date, ignoring any
// schema template this process has cached. Tests that exercise the installers
// use it directly.
func OpenFresh(path string) (*sql.DB, error) {
	// The validated resource policy owns the pool size and every per-connection
	// pragma, and it carries them in the DSN so each pooled connection inherits
	// them. Reads run concurrently on a bounded set of WAL readers; write
	// ordering is the write gate's job, not the pool's. See internal/dbwork.
	db, err := dbwork.OpenHandle(path, dbwork.DefaultPolicy())
	if err != nil {
		return nil, err
	}
	if err = migrate(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	if err = dbwork.InstallWakeDependencies(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migrate installs or updates the schema, and runs no DDL at all when every
// embedded migration is already recorded.
func migrate(ctx context.Context, db *sql.DB) error {
	steps, err := schemaMigrations()
	if err != nil {
		return err
	}
	return migrateWith(ctx, db, steps)
}

// migrateWith applies every step whose version is not in schema_migrations,
// lowest first, and verifies the digest of every step that is.
func migrateWith(ctx context.Context, db *sql.DB, steps []schemaMigration) error {
	latest := steps[len(steps)-1].version
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type='table' AND name='configuration'`).Scan(&exists); err != nil {
		return err
	}
	stored := 0
	var err error
	if exists != 0 {
		stored, err = storedSchemaVersion(ctx, db)
		if err != nil {
			return err
		}
	}
	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}
	// Pre-release first: a database from before the baseline carries a schema
	// version up to 265, which is also "greater" than the baseline's, and it
	// must get the reset message, not "upgrade Portico".
	if exists != 0 && applied[baselineSchemaVersion] == "" {
		return ErrSchemaResetRequired
	}
	if stored > latest {
		return fmt.Errorf("%w: database %d, binary %d; upgrade Portico or restore an earlier backup", ErrSchemaNewer, stored, latest)
	}
	embedded := make(map[int]bool, len(steps))
	for _, step := range steps {
		embedded[step.version] = true
	}
	for version := range applied {
		if !embedded[version] {
			// A migration this binary doesn't know: a newer binary wrote this database.
			return fmt.Errorf("%w: database has migration %d, binary %d does not; upgrade Portico or restore an earlier backup", ErrSchemaNewer, version, latest)
		}
	}
	for _, step := range steps {
		if digest, ok := applied[step.version]; ok {
			if digest != step.digest {
				return fmt.Errorf("schema migration %d digest mismatch: applied migrations are immutable", step.version)
			}
			continue
		}
		gated, err := dbwork.Begin(ctx, db, dbwork.ClassMaintenance)
		if err != nil {
			return err
		}
		tx := gated.Tx()
		_, err = tx.ExecContext(ctx, step.sql)
		if err == nil {
			_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,digest TEXT NOT NULL,applied_at TEXT NOT NULL)`)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations VALUES(?,?,?,?)`, step.version, step.name, step.digest, time.Now().UTC().Format(time.RFC3339Nano))
		}
		if err == nil && step.version > stored {
			// A late lower number never lowers the recorded high-water mark.
			_, err = tx.ExecContext(ctx, `INSERT INTO configuration(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, schemaVersionKey, strconv.Itoa(step.version))
		}
		if err != nil {
			gated.Rollback()
			return fmt.Errorf("schema migration %s: %w", step.name, err)
		}
		if err = gated.Commit(); err != nil {
			return err
		}
		if step.version > stored {
			stored = step.version
		}
	}
	return nil
}

// appliedMigrations reads the ledger in one statement: version to digest.
func appliedMigrations(ctx context.Context, db *sql.DB) (map[int]string, error) {
	out := map[int]string{}
	var ledger int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type='table' AND name='schema_migrations'`).Scan(&ledger); err != nil || ledger == 0 {
		return out, err
	}
	rows, err := db.QueryContext(ctx, `SELECT version,digest FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var version int
		var digest string
		if err = rows.Scan(&version, &digest); err != nil {
			return nil, err
		}
		out[version] = digest
	}
	return out, rows.Err()
}

// storedSchemaVersion reads the recorded version. A database from before
// versioning existed reads as 0, which is the correct answer: it has to be
// brought up once.
func storedSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT value FROM configuration WHERE key=?`, schemaVersionKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	value, convErr := strconv.Atoi(raw)
	if convErr != nil {
		return 0, nil
	}
	return value, nil
}

// SchemaVersion reports the version this binary installs, for diagnostics.
func SchemaVersion() int { return schemaVersion }
