package persistence

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type schemaMigration struct {
	version           int
	name, sql, digest string
}

// schemaMigrations returns the embedded migrations in ascending order.
//
// Numbers are unique and ascending, but they need not be contiguous: lanes
// develop in parallel with reserved number blocks, so 0040 can reach a database
// before 0023 does. The runner applies every embedded migration that isn't in
// schema_migrations, lowest first (see migrate).
//
// The rule every migration follows, because it can run after higher numbers:
//   - it depends only on the baseline and on lower-numbered migrations that
//     were already on the trunk when it was written, never on a higher number;
//   - it is safe to apply after any unrelated higher-numbered migration, so it
//     never assumes it is the newest change to a table (no rebuilding a table
//     from a hard-coded column list another migration may have extended).
//
// TestMigrationsApplyAfterHigherNumbers enforces the second half.
func schemaMigrations() ([]schemaMigration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	out := make([]schemaMigration, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		number, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(number)
		if !ok || err != nil || version < baselineSchemaVersion || len(out) == 0 && version != baselineSchemaVersion || len(out) > 0 && version <= out[len(out)-1].version {
			return nil, fmt.Errorf("invalid or out-of-order schema migration %s", name)
		}
		data, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(data)
		out = append(out, schemaMigration{version, name, string(data), hex.EncodeToString(digest[:])})
	}
	if len(out) == 0 || out[len(out)-1].version != schemaVersion {
		return nil, fmt.Errorf("schema version %d has no migration", schemaVersion)
	}
	return out, nil
}

// MigrationSQL returns one embedded migration's SQL. Package tests that build a
// subsystem's tables on a bare database (without the baseline) use it so their
// fixtures come from the same source a real install does.
func MigrationSQL(version int) (string, error) {
	steps, err := schemaMigrations()
	if err != nil {
		return "", err
	}
	for _, step := range steps {
		if step.version == version {
			return step.sql, nil
		}
	}
	return "", fmt.Errorf("no schema migration %d", version)
}
