package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	embeddedMigrations "github.com/PorticoMediaServer/portico-server/migrations"
)

const (
	currentDatabaseFormatVersion  = 2
	expectedMigrationHead         = "002_playback_receiver_authority"
	reviewedMigrationManifestName = "migration-manifest.json"
	// 0.1.74 shipped this exact reviewed 001 ledger before receiver authority
	// became append-only migration 002. No other historical checksum is a
	// supported upgrade source.
	legacy074Migration001Checksum = "3efae2a9860854930ffe680d7acb65c48baf4491051cbf7357a7da7c9a3ce994"
	legacy074Migration001Ledger   = "49b356a346fdda57dd3cfd8b635c2ba1930b4be49da7d4db57b0f10e1a044088"
	legacy074MigrationHead        = "001_initial"
	legacy074Forward001Checksum   = "c80ee6d3e4fa1714240f2cf7ac3c08584b50990a1cbed260bd20ae3828b7f866"
	legacy074Forward002Checksum   = "417ef8b13c23a4f8c4a9214cc8af31364a21c6021ef97663078c877e5a91b749"
)

const MigrationLayoutRebuildRequiredCode = "database_layout_rebuild_required"

// MigrationLayoutError is a stable, machine-classifiable refusal for a
// database outside the reviewed public migration lineage. The migration
// transaction is rolled back before this error is returned; callers must keep
// the configured database at its original path and direct the operator to
// rebuild explicitly. The runner never resets or substitutes the database.
type MigrationLayoutError struct {
	Code   string
	Detail string
}

func (e *MigrationLayoutError) Error() string {
	return e.Code + ": unsupported database layout: " + e.Detail + "; rebuild required; application schema and data left unchanged"
}

func migrationLayoutError(detail string) error {
	return &MigrationLayoutError{Code: MigrationLayoutRebuildRequiredCode, Detail: detail}
}

const MigrationIntegrityFailureCode = "database_migration_integrity_failure"

// MigrationIntegrityError distinguishes corruption of a reviewed migration
// ledger from an unsupported historical layout. Operators must investigate
// and restore trusted metadata instead of resetting or adopting it.
type MigrationIntegrityError struct {
	Code   string
	Detail string
}

func (e *MigrationIntegrityError) Error() string {
	return e.Code + ": " + e.Detail + "; application schema and data left unchanged"
}

func migrationIntegrityError(detail string) error {
	return &MigrationIntegrityError{Code: MigrationIntegrityFailureCode, Detail: detail}
}

// MigrationIdentity is the application-level schema identity used by startup,
// backup validation, diagnostics, and restore. SQLite's schema_version is an
// internal change counter and is deliberately not part of this contract.
type MigrationIdentity struct {
	FormatVersion int    `json:"formatVersion"`
	MigrationHead string `json:"migrationHead"`
	LedgerSHA256  string `json:"migrationLedgerSha256"`
	MinimumReader string `json:"minimumReader"`
}

// CurrentMigrationIdentity returns the identity written by a fully migrated
// database in this release. Restore validation uses this application-level
// identity; SQLite's schema_version is deliberately not authoritative.
func CurrentMigrationIdentity() (MigrationIdentity, error) {
	bundle, err := loadMigrationBundle(embeddedMigrationSource())
	if err != nil {
		return MigrationIdentity{}, err
	}
	return MigrationIdentity{
		FormatVersion: currentDatabaseFormatVersion,
		MigrationHead: bundle.Head.Version,
		LedgerSHA256:  bundle.LedgerHash,
		MinimumReader: "1",
	}, nil
}

// ReadMigrationIdentity reads and validates the exact release schema identity
// on an already-open SQLite handle.
func ReadMigrationIdentity(ctx context.Context, db *sql.DB) (MigrationIdentity, int, error) {
	if db == nil {
		return MigrationIdentity{}, 0, errors.New("database handle is nil")
	}
	bundle, err := loadMigrationBundle(embeddedMigrationSource())
	if err != nil {
		return MigrationIdentity{}, 0, err
	}
	var identity MigrationIdentity
	err = db.QueryRowContext(ctx, `
		SELECT format_version, migration_head, migration_ledger_sha256,
		       COALESCE(minimum_reader, '')
		FROM portico_database_identity WHERE id = 1`).Scan(
		&identity.FormatVersion, &identity.MigrationHead, &identity.LedgerSHA256,
		&identity.MinimumReader)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MigrationIdentity{}, 0, errors.New("database migration identity is missing")
		}
		return MigrationIdentity{}, 0, err
	}
	if err := validateDatabaseIdentity(ctx, db, bundle); err != nil {
		return MigrationIdentity{}, 0, err
	}
	if identity.FormatVersion <= 0 {
		return MigrationIdentity{}, 0, errors.New("database format identity is invalid")
	}
	if !minimumReaderCompatible(identity.MinimumReader, "1") {
		return MigrationIdentity{}, 0, errors.New("database minimum-reader identity is invalid")
	}
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&rows); err != nil {
		return MigrationIdentity{}, 0, err
	}
	return identity, rows, nil
}

type migration struct {
	Number   int
	Version  string
	Filename string
	SQL      []byte
	Checksum string
}

type migrationBundle struct {
	Migrations []migration
	Head       migration
	LedgerHash string
}

type migrationSource struct {
	FS              fs.FS
	Root            string
	Label           string
	CanonicalBundle bool
}

type reviewedMigrationManifest struct {
	FormatVersion int                              `json:"formatVersion"`
	ReviewedHead  string                           `json:"reviewedHead"`
	Migrations    []reviewedMigrationManifestEntry `json:"migrations"`
}

type reviewedMigrationManifestEntry struct {
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
}

type migrationConnection interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

// expectedMigrationFiles is intentionally explicit. It catches a package
// that accidentally ships a partial SQL bundle. A future migration is append-only and
// must be introduced through one reviewed change that adds its SQL file,
// updates this map and expectedMigrationHead, and records its bytes in the
// checked-in manifest; auto-discovery is deliberately not supported.
var expectedMigrationFiles = map[int]string{
	1: "001_initial.sql",
	2: "002_playback_receiver_authority.sql",
}

// ValidateEmbeddedMigrationBundle performs the release-time bundle check
// before a database is opened or normal service startup begins.
func ValidateEmbeddedMigrationBundle() error {
	_, err := loadMigrationBundle(migrationSource{
		FS:              embeddedMigrations.FS(),
		Root:            ".",
		Label:           "embedded migrations",
		CanonicalBundle: true,
	})
	return err
}

func embeddedMigrationSource() migrationSource {
	return migrationSource{
		FS:              embeddedMigrations.FS(),
		Root:            ".",
		Label:           "embedded migrations",
		CanonicalBundle: true,
	}
}

func directoryMigrationSource(dir string) migrationSource {
	return migrationSource{
		FS:    os.DirFS(dir),
		Root:  ".",
		Label: filepath.Clean(dir),
	}
}

func loadMigrationBundle(source migrationSource) (migrationBundle, error) {
	entries, err := fs.ReadDir(source.FS, source.Root)
	if err != nil {
		return migrationBundle{}, fmt.Errorf("read %s: %w", source.Label, err)
	}

	migrations := make([]migration, 0, len(entries))
	seenNumbers := map[int]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := entry.Name()
		if len(name) < 5 || name[3] != '_' {
			return migrationBundle{}, fmt.Errorf("invalid migration filename %q in %s", name, source.Label)
		}
		number, err := strconv.Atoi(name[:3])
		if err != nil || number < 1 {
			return migrationBundle{}, fmt.Errorf("invalid migration version %q in %s", name, source.Label)
		}
		if previous, ok := seenNumbers[number]; ok {
			return migrationBundle{}, fmt.Errorf("duplicate migration version %03d: %s and %s", number, previous, name)
		}
		body, err := fs.ReadFile(source.FS, path.Join(source.Root, name))
		if err != nil {
			return migrationBundle{}, fmt.Errorf("read migration %s: %w", name, err)
		}
		if len(strings.TrimSpace(string(body))) == 0 {
			return migrationBundle{}, fmt.Errorf("migration %s is empty", name)
		}
		sum := sha256.Sum256(body)
		seenNumbers[number] = name
		migrations = append(migrations, migration{
			Number:   number,
			Version:  strings.TrimSuffix(name, ".sql"),
			Filename: name,
			SQL:      body,
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	if len(migrations) == 0 {
		return migrationBundle{}, fmt.Errorf("%s contains no SQL migrations", source.Label)
	}
	sort.Slice(migrations, func(i, j int) bool {
		if migrations[i].Number != migrations[j].Number {
			return migrations[i].Number < migrations[j].Number
		}
		return migrations[i].Filename < migrations[j].Filename
	})
	if migrations[0].Number != 1 || migrations[0].Filename != "001_initial.sql" {
		return migrationBundle{}, fmt.Errorf("%s must start with non-empty 001_initial.sql", source.Label)
	}
	for index, item := range migrations {
		expectedNumber := index + 1
		if item.Number != expectedNumber {
			return migrationBundle{}, fmt.Errorf("%s has non-contiguous migration sequence: expected %03d, found %03d", source.Label, expectedNumber, item.Number)
		}
	}

	if source.CanonicalBundle {
		for number, filename := range expectedMigrationFiles {
			actual, ok := seenNumbers[number]
			if !ok {
				return migrationBundle{}, fmt.Errorf("%s is incomplete: missing %s", source.Label, filename)
			}
			if actual != filename {
				return migrationBundle{}, fmt.Errorf("%s has migration %03d named %q; expected %q", source.Label, number, actual, filename)
			}
		}
		if len(seenNumbers) != len(expectedMigrationFiles) {
			return migrationBundle{}, fmt.Errorf("%s contains unexpected migration files", source.Label)
		}
		if err := validateReviewedMigrationManifest(source, migrations); err != nil {
			return migrationBundle{}, err
		}
	}

	ledger := sha256.New()
	for _, item := range migrations {
		_, _ = fmt.Fprintf(ledger, "%03d|%s|%s\n", item.Number, item.Filename, item.Checksum)
	}
	return migrationBundle{
		Migrations: migrations,
		Head:       migrations[len(migrations)-1],
		LedgerHash: hex.EncodeToString(ledger.Sum(nil)),
	}, nil
}

func validateReviewedMigrationManifest(source migrationSource, migrations []migration) error {
	body, err := fs.ReadFile(source.FS, path.Join(source.Root, reviewedMigrationManifestName))
	if err != nil {
		return fmt.Errorf("read reviewed migration manifest in %s: %w", source.Label, err)
	}
	var manifest reviewedMigrationManifest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode reviewed migration manifest in %s: %w", source.Label, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("reviewed migration manifest in %s contains trailing content", source.Label)
	}
	if manifest.FormatVersion != 1 {
		return fmt.Errorf("reviewed migration manifest format %d is unsupported", manifest.FormatVersion)
	}
	if manifest.ReviewedHead != expectedMigrationHead {
		return fmt.Errorf("reviewed migration manifest head %q does not match reviewed application head %q", manifest.ReviewedHead, expectedMigrationHead)
	}
	seen := make(map[string]struct{}, len(manifest.Migrations))
	for _, entry := range manifest.Migrations {
		if _, duplicate := seen[entry.Filename]; duplicate {
			return fmt.Errorf("reviewed migration manifest repeats %s", entry.Filename)
		}
		seen[entry.Filename] = struct{}{}
	}
	if len(manifest.Migrations) != len(expectedMigrationFiles) || len(manifest.Migrations) != len(migrations) {
		return fmt.Errorf("reviewed migration manifest has %d entries; expected %d", len(manifest.Migrations), len(expectedMigrationFiles))
	}
	byFilename := make(map[string]string, len(migrations))
	for _, item := range migrations {
		byFilename[item.Filename] = item.Checksum
	}
	for index, entry := range manifest.Migrations {
		if entry.Filename != migrations[index].Filename {
			return fmt.Errorf("reviewed migration manifest entry %d is %s; expected ordered file %s", index+1, entry.Filename, migrations[index].Filename)
		}
		expectedChecksum, ok := byFilename[entry.Filename]
		if !ok {
			return fmt.Errorf("reviewed migration manifest names unknown file %s", entry.Filename)
		}
		if !strings.EqualFold(entry.SHA256, expectedChecksum) {
			return fmt.Errorf("reviewed migration source checksum mismatch for %s: manifest=%s embedded=%s", entry.Filename, entry.SHA256, expectedChecksum)
		}
	}
	for number, filename := range expectedMigrationFiles {
		if _, ok := seen[filename]; !ok {
			return fmt.Errorf("reviewed migration manifest is missing %03d/%s", number, filename)
		}
	}
	return nil
}

func migrationPrefixLedgerHash(migrations []migration, headNumber int) string {
	ledger := sha256.New()
	for _, item := range migrations {
		if item.Number > headNumber {
			break
		}
		_, _ = fmt.Fprintf(ledger, "%03d|%s|%s\n", item.Number, item.Filename, item.Checksum)
	}
	return hex.EncodeToString(ledger.Sum(nil))
}

func runEmbeddedMigrations(db *sql.DB) error {
	if db == nil {
		return errors.New("database is required")
	}
	// A migration must have one reserved connection because SQLite PRAGMAs,
	// transaction state, and the migration lock are connection-local. Restore
	// the normal read pool only after the ledger has converged.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer func() {
		db.SetMaxOpenConns(32)
		db.SetMaxIdleConns(16)
	}()
	_, err := retrySQLiteLock(context.Background(), migrationRetryOptions, func() error {
		conn, err := db.Conn(context.Background())
		if err != nil {
			return fmt.Errorf("reserve migration connection: %w", err)
		}
		defer conn.Close()
		if _, err := conn.ExecContext(context.Background(), `
			PRAGMA foreign_keys = ON;
			PRAGMA busy_timeout = 15000;
			PRAGMA journal_mode = WAL;
			PRAGMA synchronous = NORMAL;
		`); err != nil {
			return fmt.Errorf("configure migration connection: %w", err)
		}
		return runMigrationsOnReservedConnection(context.Background(), conn, embeddedMigrationSource())
	})
	return err
}

// runSQLMigrations remains available to focused database tests and migration
// fixture tools. Production startup always uses runEmbeddedMigrations.
func runSQLMigrations(db *sql.DB, dir string) error {
	if db == nil {
		return errors.New("database is required")
	}
	if strings.TrimSpace(dir) == "" {
		return runEmbeddedMigrations(db)
	}
	return runMigrationsOnConnection(context.Background(), db, directoryMigrationSource(dir))
}

func runMigrationsOnConnection(ctx context.Context, conn migrationConnection, source migrationSource) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if db, ok := conn.(*sql.DB); ok {
		_, err := retrySQLiteLock(ctx, migrationRetryOptions, func() error {
			reserved, err := db.Conn(ctx)
			if err != nil {
				return fmt.Errorf("reserve migration connection: %w", err)
			}
			defer reserved.Close()
			return runMigrationsOnReservedConnection(ctx, reserved, source)
		})
		return err
	}
	_, err := retrySQLiteLock(ctx, migrationRetryOptions, func() error {
		return runMigrationsOnReservedConnection(ctx, conn, source)
	})
	return err
}

var migrationRetryOptions = RetryOptions{Attempts: 12, Base: 50 * time.Millisecond, Max: time.Second}

func runMigrationsOnReservedConnection(ctx context.Context, conn migrationConnection, source migrationSource) error {
	bundle, err := loadMigrationBundle(source)
	if err != nil {
		return err
	}
	if source.CanonicalBundle && bundle.Head.Version != expectedMigrationHead {
		return fmt.Errorf("embedded migration head is %s; expected %s", bundle.Head.Version, expectedMigrationHead)
	}
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return fmt.Errorf("acquire migration write lock: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	if err := ensureMigrationMetadata(ctx, conn); err != nil {
		return err
	}
	if err := validateDatabaseIdentity(ctx, conn, bundle); err != nil {
		return err
	}
	if source.CanonicalBundle {
		if _, err := canonicalizeLegacy074MigrationLedger(ctx, conn, bundle); err != nil {
			return err
		}
	}
	appliedAny := false
	for _, item := range bundle.Migrations {
		applied, checksum, filename, err := appliedMigration(ctx, conn, item.Version)
		if err != nil {
			return err
		}
		if applied {
			if filename != item.Filename {
				return fmt.Errorf("migration %s was recorded as %s", item.Version, filename)
			}
			if checksum != item.Checksum {
				return fmt.Errorf("migration checksum mismatch for %s: database=%s binary=%s", item.Version, checksum, item.Checksum)
			}
			continue
		}

		if _, err := conn.ExecContext(ctx, string(item.SQL)); err != nil {
			return fmt.Errorf("run migration %s: %w", item.Version, err)
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO schema_migrations (version, filename, applied_at, checksum_sha256, checksum_source)
			VALUES (?, ?, ?, ?, 'embedded-bundle')
		`, item.Version, item.Filename, time.Now().UTC().Format(time.RFC3339Nano), item.Checksum); err != nil {
			return fmt.Errorf("record migration %s: %w", item.Version, err)
		}
		appliedAny = true
	}

	if appliedAny {
		identity := MigrationIdentity{
			FormatVersion: currentDatabaseFormatVersion,
			MigrationHead: bundle.Head.Version,
			LedgerSHA256:  bundle.LedgerHash,
			MinimumReader: "1",
		}
		if _, err := conn.ExecContext(ctx, `
		INSERT INTO portico_database_identity (id, format_version, migration_head, migration_ledger_sha256, minimum_reader, updated_at)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			format_version = excluded.format_version,
			migration_head = excluded.migration_head,
			migration_ledger_sha256 = excluded.migration_ledger_sha256,
			minimum_reader = excluded.minimum_reader,
			updated_at = excluded.updated_at
		`, identity.FormatVersion, identity.MigrationHead, identity.LedgerSHA256, identity.MinimumReader, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record database migration identity: %w", err)
		}
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return fmt.Errorf("commit migration ledger: %w", err)
	}
	committed = true
	return nil
}

func ensureMigrationMetadata(ctx context.Context, conn migrationConnection) error {
	var existing int
	if err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name IN ('schema_migrations', 'portico_database_identity')
	`).Scan(&existing); err != nil {
		return fmt.Errorf("inspect migration metadata: %w", err)
	}
	if existing == 0 {
		if _, err := conn.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			filename TEXT NOT NULL,
			applied_at TEXT NOT NULL,
			checksum_sha256 TEXT NOT NULL,
			checksum_source TEXT NOT NULL
		);
		CREATE TABLE portico_database_identity (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			format_version INTEGER NOT NULL,
			migration_head TEXT NOT NULL,
			migration_ledger_sha256 TEXT NOT NULL,
			minimum_reader TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		`); err != nil {
			return fmt.Errorf("create migration metadata: %w", err)
		}
	} else if existing != 2 {
		return migrationLayoutError("migration metadata is incomplete")
	}
	if err := validateMetadataTable(ctx, conn, "schema_migrations", []string{"version", "filename", "applied_at", "checksum_sha256", "checksum_source"}); err != nil {
		return err
	}
	if err := validateMetadataTable(ctx, conn, "portico_database_identity", []string{"id", "format_version", "migration_head", "migration_ledger_sha256", "minimum_reader", "updated_at"}); err != nil {
		return err
	}
	return nil
}

func validateMetadataTable(ctx context.Context, conn migrationConnection, table string, expected []string) error {
	rows, err := conn.QueryContext(ctx, `PRAGMA table_info(`+quoteSQLiteIdentifier(table)+`)`)
	if err != nil {
		return fmt.Errorf("inspect migration metadata table %s: %w", table, err)
	}
	defer rows.Close()
	actual := make([]string, 0, len(expected))
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("read migration metadata table %s: %w", table, err)
		}
		actual = append(actual, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate migration metadata table %s: %w", table, err)
	}
	if len(actual) != len(expected) {
		return migrationLayoutError(table + " has obsolete migration metadata columns")
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return migrationLayoutError(table + " has obsolete migration metadata columns")
		}
	}
	return nil
}

func validateDatabaseIdentity(ctx context.Context, conn migrationConnection, bundle migrationBundle) error {
	var formatVersion int
	var head, ledgerHash string
	err := conn.QueryRowContext(ctx, `SELECT format_version, migration_head, migration_ledger_sha256 FROM portico_database_identity WHERE id = 1`).Scan(&formatVersion, &head, &ledgerHash)
	if errors.Is(err, sql.ErrNoRows) {
		var applied int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
			return fmt.Errorf("read migration ledger without identity: %w", err)
		}
		if applied != 0 {
			return migrationLayoutError("migration ledger exists without the canonical identity")
		}
		var applicationObjects int
		if err := conn.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sqlite_master
			WHERE sql IS NOT NULL
			  AND name NOT LIKE 'sqlite_%'
			  AND name NOT IN ('schema_migrations', 'portico_database_identity')
		`).Scan(&applicationObjects); err != nil {
			return fmt.Errorf("inspect empty database before baseline: %w", err)
		}
		if applicationObjects != 0 {
			return migrationLayoutError("an unversioned application schema exists")
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read database migration identity: %w", err)
	}
	if formatVersion != currentDatabaseFormatVersion {
		return migrationLayoutError("database format does not match the canonical release")
	}
	if head == legacy074MigrationHead && strings.EqualFold(strings.TrimSpace(ledgerHash), legacy074Migration001Ledger) {
		return validateLegacy074DatabasePrefix(ctx, conn)
	}
	headNumber, err := migrationNumber(head)
	if err != nil || headNumber > bundle.Head.Number {
		return migrationLayoutError("migration head does not match a reviewed canonical prefix")
	}
	headMigration, ok := migrationForNumber(bundle, headNumber)
	if !ok || headMigration.Version != head {
		return migrationLayoutError("migration head does not match a reviewed canonical prefix")
	}
	expectedLedger := migrationPrefixLedgerHash(bundle.Migrations, headNumber)
	if !strings.EqualFold(strings.TrimSpace(ledgerHash), expectedLedger) {
		return migrationIntegrityError(fmt.Sprintf("migration ledger digest mismatch: database=%s binary=%s", ledgerHash, expectedLedger))
	}
	return validateDatabasePrefix(ctx, conn, bundle, headNumber)
}

func validateLegacy074DatabasePrefix(ctx context.Context, conn migrationConnection) error {
	rows, err := conn.QueryContext(ctx, `SELECT rowid, version, filename, COALESCE(checksum_sha256, '') FROM schema_migrations ORDER BY rowid`)
	if err != nil {
		return fmt.Errorf("read deployed 0.1.74 migration prefix: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return migrationLayoutError("deployed 0.1.74 identity has no migration row")
	}
	var rowID int64
	var version, filename, checksum string
	if err := rows.Scan(&rowID, &version, &filename, &checksum); err != nil {
		return fmt.Errorf("read deployed 0.1.74 migration row: %w", err)
	}
	if version != legacy074MigrationHead || filename != "001_initial.sql" {
		return migrationLayoutError(fmt.Sprintf("deployed 0.1.74 migration row %d is %s/%s", rowID, version, filename))
	}
	if !strings.EqualFold(strings.TrimSpace(checksum), legacy074Migration001Checksum) {
		return migrationIntegrityError(fmt.Sprintf("deployed 0.1.74 migration checksum mismatch: database=%s reviewed=%s", checksum, legacy074Migration001Checksum))
	}
	if rows.Next() {
		return migrationLayoutError("deployed 0.1.74 migration ledger contains an ambiguous extra row")
	}
	return rows.Err()
}

// canonicalizeLegacy074MigrationLedger changes metadata only, inside the same
// transaction that applies 002. If 002 or identity publication fails, SQLite
// rolls this update back with every other migration effect.
func canonicalizeLegacy074MigrationLedger(ctx context.Context, conn migrationConnection, bundle migrationBundle) (bool, error) {
	var head, ledger string
	if err := conn.QueryRowContext(ctx, `SELECT migration_head, migration_ledger_sha256 FROM portico_database_identity WHERE id = 1`).Scan(&head, &ledger); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if head != legacy074MigrationHead || !strings.EqualFold(strings.TrimSpace(ledger), legacy074Migration001Ledger) {
		return false, nil
	}
	canonical001, ok := migrationForNumber(bundle, 1)
	canonical002, has002 := migrationForNumber(bundle, 2)
	if !ok || !has002 || canonical001.Version != legacy074MigrationHead || canonical001.Filename != "001_initial.sql" ||
		!strings.EqualFold(canonical001.Checksum, legacy074Forward001Checksum) ||
		canonical002.Version != expectedMigrationHead || canonical002.Filename != "002_playback_receiver_authority.sql" ||
		!strings.EqualFold(canonical002.Checksum, legacy074Forward002Checksum) {
		return false, migrationIntegrityError("canonical 001/002 bundle does not match the reviewed 0.1.74 forward bridge")
	}
	result, err := conn.ExecContext(ctx, `UPDATE schema_migrations
		SET checksum_sha256 = ?, checksum_source = 'forward-equivalent:0.1.74+002'
		WHERE version = ? AND filename = '001_initial.sql' AND checksum_sha256 = ?`,
		canonical001.Checksum, legacy074MigrationHead, legacy074Migration001Checksum)
	if err != nil {
		return false, fmt.Errorf("canonicalize deployed 0.1.74 migration ledger: %w", err)
	}
	affected, affectedErr := result.RowsAffected()
	if affectedErr != nil {
		return false, fmt.Errorf("verify deployed 0.1.74 migration ledger canonicalization: %w", affectedErr)
	}
	if affected != 1 {
		return false, migrationIntegrityError("deployed 0.1.74 migration ledger changed during upgrade")
	}
	return true, nil
}

func validateDatabasePrefix(ctx context.Context, conn migrationConnection, bundle migrationBundle, headNumber int) error {
	expected := make([]migration, 0, headNumber)
	for _, item := range bundle.Migrations {
		if item.Number > headNumber {
			break
		}
		expected = append(expected, item)
	}
	rows, err := conn.QueryContext(ctx, `SELECT rowid, version, filename, COALESCE(checksum_sha256, '') FROM schema_migrations ORDER BY rowid`)
	if err != nil {
		return fmt.Errorf("read applied migration prefix: %w", err)
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var rowID int64
		var version, filename, checksum string
		if err := rows.Scan(&rowID, &version, &filename, &checksum); err != nil {
			return fmt.Errorf("read applied migration row: %w", err)
		}
		if index >= len(expected) {
			return migrationLayoutError(fmt.Sprintf("migration ledger contains extra row %q beyond its declared head", version))
		}
		item := expected[index]
		if version != item.Version || filename != item.Filename {
			return migrationLayoutError(fmt.Sprintf("migration ledger row %d is %s/%s; expected %s/%s", rowID, version, filename, item.Version, item.Filename))
		}
		if strings.TrimSpace(checksum) == "" {
			return migrationLayoutError(fmt.Sprintf("migration %s has no reviewed checksum", version))
		}
		if !strings.EqualFold(checksum, item.Checksum) {
			return migrationIntegrityError(fmt.Sprintf("migration checksum mismatch for %s: database=%s binary=%s", version, checksum, item.Checksum))
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate applied migration prefix: %w", err)
	}
	if index != len(expected) {
		return migrationLayoutError(fmt.Sprintf("migration ledger has %d rows through head %03d; expected %d exact ordered rows", index, headNumber, len(expected)))
	}
	return nil
}

func migrationForNumber(bundle migrationBundle, number int) (migration, bool) {
	for _, item := range bundle.Migrations {
		if item.Number == number {
			return item, true
		}
	}
	return migration{}, false
}

func appliedMigration(ctx context.Context, conn migrationConnection, version string) (bool, string, string, error) {
	var checksum, filename string
	err := conn.QueryRowContext(ctx, `SELECT COALESCE(checksum_sha256, ''), filename FROM schema_migrations WHERE version = ?`, version).Scan(&checksum, &filename)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", "", nil
	}
	if err != nil {
		return false, "", "", fmt.Errorf("check migration %s: %w", version, err)
	}
	return true, checksum, filename, nil
}

func migrationNumber(version string) (int, error) {
	if len(version) < 4 || version[3] != '_' {
		return 0, errors.New("must use NNN_name format")
	}
	number, err := strconv.Atoi(version[:3])
	if err != nil || number < 1 {
		return 0, errors.New("invalid numeric prefix")
	}
	return number, nil
}
