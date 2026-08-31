package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	embeddedMigrations "github.com/PorticoMediaServer/portico-server/migrations"
)

func TestMigrationBaselineFreshAndReopenIsNoOp(t *testing.T) {
	if err := ValidateEmbeddedMigrationBundle(); err != nil {
		t.Fatalf("validate embedded migration bundle: %v", err)
	}
	root := t.TempDir()
	cfg := config.Config{AppDataDir: root, DatabasePath: filepath.Join(root, "portico.db")}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("open fresh release baseline: %v", err)
	}
	identity, rows, err := ReadMigrationIdentity(t.Context(), db)
	if err != nil {
		t.Fatalf("read release identity: %v", err)
	}
	if identity.FormatVersion != currentDatabaseFormatVersion || identity.MigrationHead != expectedMigrationHead || rows != len(expectedMigrationFiles) || len(identity.LedgerSHA256) != sha256.Size*2 {
		t.Fatalf("unexpected release identity: %+v rows=%d", identity, rows)
	}
	if !sqliteIndexExists(t, db, "user_media_state", "idx_media_browse_personal_rating") {
		t.Fatal("post-baseline personal-rating browse index was not applied")
	}
	for _, obsolete := range []string{"portico_schema_reconciliation", "media_legacy_ids", "public_resource_legacy_ids"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, obsolete).Scan(&count); err != nil {
			t.Fatalf("inspect obsolete table %s: %v", obsolete, err)
		}
		if count != 0 {
			t.Fatalf("obsolete table %s exists in release baseline", obsolete)
		}
	}
	firstSchema := releaseSchemaSnapshot(t, db)
	var firstUpdatedAt string
	if err := db.QueryRow(`SELECT updated_at FROM portico_database_identity WHERE id = 1`).Scan(&firstUpdatedAt); err != nil {
		t.Fatalf("read initial migration identity timestamp: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close release baseline: %v", err)
	}

	reopened, err := Open(cfg)
	if err != nil {
		t.Fatalf("reopen current release database: %v", err)
	}
	defer reopened.Close()
	if _, reopenedRows, err := ReadMigrationIdentity(t.Context(), reopened); err != nil || reopenedRows != len(expectedMigrationFiles) {
		t.Fatalf("reopened release identity rows=%d err=%v", reopenedRows, err)
	}
	if reopenedSchema := releaseSchemaSnapshot(t, reopened); reopenedSchema != firstSchema {
		t.Fatal("current-database reopen mutated the release schema")
	}
	var reopenedUpdatedAt string
	if err := reopened.QueryRow(`SELECT updated_at FROM portico_database_identity WHERE id = 1`).Scan(&reopenedUpdatedAt); err != nil {
		t.Fatalf("read reopened migration identity timestamp: %v", err)
	}
	if reopenedUpdatedAt != firstUpdatedAt {
		t.Fatalf("exact reopen rewrote migration identity: before=%q after=%q", firstUpdatedAt, reopenedUpdatedAt)
	}
}

func TestShippingMigrationInventoryContainsCanonicalReviewedHistory(t *testing.T) {
	entries, err := fs.ReadDir(embeddedMigrations.FS(), ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	var embeddedSQL []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			embeddedSQL = append(embeddedSQL, entry.Name())
		}
	}
	if want := []string{"001_initial.sql", "002_playback_receiver_authority.sql"}; !slices.Equal(embeddedSQL, want) {
		t.Fatalf("embedded SQL inventory=%v want=%v", embeddedSQL, want)
	}
}

func TestMigrationBaselineHasNoLegacyAdministratorAuthority(t *testing.T) {
	body, err := fs.ReadFile(embeddedMigrations.FS(), "001_initial.sql")
	if err != nil {
		t.Fatalf("read release baseline: %v", err)
	}
	for _, legacy := range []string{"role = 'admin'", "role IN ('owner', 'admin')", "role IN ('admin', 'owner')"} {
		if strings.Contains(string(body), legacy) {
			t.Fatalf("release baseline retains legacy administrator authority predicate %q", legacy)
		}
	}
}

func releaseSchemaSnapshot(t *testing.T, db interface {
	Query(string, ...any) (*sql.Rows, error)
}) string {
	t.Helper()
	rows, err := db.Query(`SELECT type, name, tbl_name, sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		t.Fatalf("read release schema: %v", err)
	}
	defer rows.Close()
	var snapshot strings.Builder
	for rows.Next() {
		var objectType, name, table, statement string
		if err := rows.Scan(&objectType, &name, &table, &statement); err != nil {
			t.Fatalf("scan release schema: %v", err)
		}
		snapshot.WriteString(objectType + "\x00" + name + "\x00" + table + "\x00" + statement + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate release schema: %v", err)
	}
	return snapshot.String()
}

func TestCanonicalMigrationBundleRejectsInventoryAndManifestDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(fstest.MapFS)
		want   string
	}{
		{name: "missing SQL", mutate: func(f fstest.MapFS) { delete(f, "001_initial.sql"); delete(f, "002_playback_receiver_authority.sql") }, want: "contains no SQL migrations"},
		{name: "extra unmanifested SQL", mutate: func(f fstest.MapFS) { f["003_unreviewed.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;\n")} }, want: "unexpected migration files"},
		{name: "checksum mismatch", mutate: func(f fstest.MapFS) {
			f["001_initial.sql"].Data = append(f["001_initial.sql"].Data, []byte("\n-- unauthorized edit\n")...)
		}, want: "checksum mismatch"},
		{name: "duplicate sequence", mutate: func(f fstest.MapFS) { f["001_duplicate.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;\n")} }, want: "duplicate migration version 001"},
		{name: "invalid filename", mutate: func(f fstest.MapFS) { f["bad.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;\n")} }, want: "invalid migration filename"},
		{name: "missing manifest", mutate: func(f fstest.MapFS) { delete(f, reviewedMigrationManifestName) }, want: "read reviewed migration manifest"},
		{name: "unknown manifest field", mutate: func(f fstest.MapFS) {
			f[reviewedMigrationManifestName].Data = []byte(strings.Replace(string(f[reviewedMigrationManifestName].Data), "{", `{"unknown":true,`, 1))
		}, want: "unknown field"},
		{name: "trailing manifest content", mutate: func(f fstest.MapFS) {
			f[reviewedMigrationManifestName].Data = append(f[reviewedMigrationManifestName].Data, []byte("{}\n")...)
		}, want: "trailing content"},
	}
	for _, test := range []struct {
		name   string
		mutate func(*reviewedMigrationManifest)
		want   string
	}{
		{name: "wrong manifest file", mutate: func(m *reviewedMigrationManifest) { m.Migrations[0].Filename = "001_wrong.sql" }, want: "expected ordered file"},
		{name: "duplicate manifest row", mutate: func(m *reviewedMigrationManifest) { m.Migrations = append(m.Migrations, m.Migrations[0]) }, want: "repeats"},
		{name: "wrong manifest head", mutate: func(m *reviewedMigrationManifest) { m.ReviewedHead = "999_wrong" }, want: "does not match"},
		{name: "wrong manifest format", mutate: func(m *reviewedMigrationManifest) { m.FormatVersion = 2 }, want: "unsupported"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := canonicalMigrationFixture(t)
			var manifest reviewedMigrationManifest
			if err := json.Unmarshal(fixture[reviewedMigrationManifestName].Data, &manifest); err != nil {
				t.Fatalf("decode fixture manifest: %v", err)
			}
			test.mutate(&manifest)
			body, err := json.Marshal(manifest)
			if err != nil {
				t.Fatalf("encode fixture manifest: %v", err)
			}
			fixture[reviewedMigrationManifestName].Data = body
			_, err = loadMigrationBundle(migrationSource{FS: fixture, Root: ".", Label: test.name, CanonicalBundle: true})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("manifest error=%v want substring %q", err, test.want)
			}
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := canonicalMigrationFixture(t)
			test.mutate(fixture)
			_, err := loadMigrationBundle(migrationSource{FS: fixture, Root: ".", Label: test.name, CanonicalBundle: true})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("bundle error=%v want substring %q", err, test.want)
			}
		})
	}

	nonContiguous := fstest.MapFS{
		"001_initial.sql": &fstest.MapFile{Data: []byte("CREATE TABLE one (id INTEGER);\n")},
		"003_gap.sql":     &fstest.MapFile{Data: []byte("CREATE TABLE three (id INTEGER);\n")},
	}
	if _, err := loadMigrationBundle(migrationSource{FS: nonContiguous, Root: ".", Label: "sequence gap"}); err == nil || !strings.Contains(err.Error(), "non-contiguous") {
		t.Fatalf("sequence-gap error=%v", err)
	}
}

func canonicalMigrationFixture(t *testing.T) fstest.MapFS {
	t.Helper()
	fixture := fstest.MapFS{}
	for _, name := range []string{"001_initial.sql", "002_playback_receiver_authority.sql", reviewedMigrationManifestName} {
		body, err := fs.ReadFile(embeddedMigrations.FS(), name)
		if err != nil {
			t.Fatalf("read embedded %s: %v", name, err)
		}
		fixture[name] = &fstest.MapFile{Data: append([]byte(nil), body...)}
	}
	return fixture
}

func openCanonical001Fixture(t *testing.T) (config.Config, *sql.DB) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{AppDataDir: root, DatabasePath: filepath.Join(root, "portico.db")}
	db, err := sql.Open("sqlite", sqliteDSN(cfg.DatabasePath))
	if err != nil {
		t.Fatalf("open canonical 001 fixture: %v", err)
	}
	body, err := fs.ReadFile(embeddedMigrations.FS(), "001_initial.sql")
	if err != nil {
		db.Close()
		t.Fatalf("read canonical 001: %v", err)
	}
	source := migrationSource{FS: fstest.MapFS{"001_initial.sql": &fstest.MapFile{Data: body}}, Root: ".", Label: "canonical 001 fixture"}
	if err := runMigrationsOnConnection(t.Context(), db, source); err != nil {
		db.Close()
		t.Fatalf("apply canonical 001 fixture: %v", err)
	}
	return cfg, db
}

func TestDeployed074Migration001UpgradesToCanonical002WithoutDataLoss(t *testing.T) {
	cfg, db := openCanonical001Fixture(t)
	if _, err := db.Exec(`CREATE TABLE upgrade_preservation_probe (id TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO upgrade_preservation_probe VALUES ('keep', 'preserve-me')`); err != nil {
		db.Close()
		t.Fatalf("seed deployed 0.1.74 fixture: %v", err)
	}
	if _, err := db.Exec(`UPDATE schema_migrations SET checksum_sha256 = ?, checksum_source = 'embedded-bundle' WHERE version = '001_initial'`, legacy074Migration001Checksum); err != nil {
		db.Close()
		t.Fatalf("shape deployed 0.1.74 ledger: %v", err)
	}
	if _, err := db.Exec(`UPDATE portico_database_identity SET migration_head = ?, migration_ledger_sha256 = ? WHERE id = 1`, legacy074MigrationHead, legacy074Migration001Ledger); err != nil {
		db.Close()
		t.Fatalf("shape deployed 0.1.74 identity: %v", err)
	}
	db.Close()

	upgraded, err := Open(cfg)
	if err != nil {
		t.Fatalf("upgrade exact deployed 0.1.74 ledger: %v", err)
	}
	defer upgraded.Close()
	var value string
	if err := upgraded.QueryRow(`SELECT value FROM upgrade_preservation_probe WHERE id = 'keep'`).Scan(&value); err != nil || value != "preserve-me" {
		t.Fatalf("upgrade changed application data: value=%q err=%v", value, err)
	}
	identity, rows, err := ReadMigrationIdentity(t.Context(), upgraded)
	if err != nil || identity.MigrationHead != expectedMigrationHead || rows != 2 {
		t.Fatalf("upgraded identity=%+v rows=%d err=%v", identity, rows, err)
	}
	var checksum, source string
	if err := upgraded.QueryRow(`SELECT checksum_sha256, checksum_source FROM schema_migrations WHERE version = '001_initial'`).Scan(&checksum, &source); err != nil {
		t.Fatal(err)
	}
	canonical, _ := migrationForNumber(mustLoadEmbeddedMigrationBundle(t), 1)
	if checksum != canonical.Checksum || source != "forward-equivalent:0.1.74+002" {
		t.Fatalf("legacy ledger was not narrowly canonicalized: checksum=%q source=%q", checksum, source)
	}
	var receiverColumns, authorizationTable int
	if err := upgraded.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('playback_receivers') WHERE name IN ('receiver_public_key','receiver_public_key_fingerprint','authorization_revision','expires_at','client_instance_id')`).Scan(&receiverColumns); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='playback_receiver_authorizations'`).Scan(&authorizationTable); err != nil {
		t.Fatal(err)
	}
	if receiverColumns != 5 || authorizationTable != 1 {
		t.Fatalf("receiver migration incomplete: columns=%d authorizationTable=%d", receiverColumns, authorizationTable)
	}
}

func mustLoadEmbeddedMigrationBundle(t *testing.T) migrationBundle {
	t.Helper()
	bundle, err := loadMigrationBundle(embeddedMigrationSource())
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestCanonicalMigration001UpgradesToCanonical002(t *testing.T) {
	cfg, db := openCanonical001Fixture(t)
	db.Close()
	upgraded, err := Open(cfg)
	if err != nil {
		t.Fatalf("upgrade canonical 001 prefix: %v", err)
	}
	defer upgraded.Close()
	identity, rows, err := ReadMigrationIdentity(t.Context(), upgraded)
	if err != nil || identity.MigrationHead != expectedMigrationHead || rows != 2 {
		t.Fatalf("canonical prefix identity=%+v rows=%d err=%v", identity, rows, err)
	}
}

func TestUnknownMigration001ChecksumRemainsRejected(t *testing.T) {
	cfg, db := openCanonical001Fixture(t)
	const unknownChecksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ledger := sha256.Sum256([]byte("001|001_initial.sql|" + unknownChecksum + "\n"))
	if _, err := db.Exec(`UPDATE schema_migrations SET checksum_sha256 = ? WHERE version = '001_initial'`, unknownChecksum); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE portico_database_identity SET migration_ledger_sha256 = ? WHERE id = 1`, fmt.Sprintf("%x", ledger[:])); err != nil {
		db.Close()
		t.Fatal(err)
	}
	before := migrationMetadataSnapshot(t, db)
	db.Close()
	opened, err := Open(cfg)
	if opened != nil {
		opened.Close()
	}
	assertMigrationIntegrityError(t, err)
	preserved, openErr := sql.Open("sqlite", sqliteDSN(cfg.DatabasePath))
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer preserved.Close()
	if after := migrationMetadataSnapshot(t, preserved); after != before {
		t.Fatalf("unknown checksum refusal mutated metadata:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestDeployed074PartialLedgerIsRejectedWithoutCanonicalization(t *testing.T) {
	cfg, db := openCanonical001Fixture(t)
	if _, err := db.Exec(`UPDATE schema_migrations SET checksum_sha256 = ? WHERE version = '001_initial'`, legacy074Migration001Checksum); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, filename, applied_at, checksum_sha256, checksum_source)
		VALUES ('002_partial', '002_partial.sql', 'fixture', 'partial', 'fixture')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE portico_database_identity SET migration_head = ?, migration_ledger_sha256 = ? WHERE id = 1`, legacy074MigrationHead, legacy074Migration001Ledger); err != nil {
		db.Close()
		t.Fatal(err)
	}
	before := migrationMetadataSnapshot(t, db)
	db.Close()
	opened, err := Open(cfg)
	if opened != nil {
		opened.Close()
	}
	assertMigrationLayoutError(t, err)
	preserved, openErr := sql.Open("sqlite", sqliteDSN(cfg.DatabasePath))
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer preserved.Close()
	if after := migrationMetadataSnapshot(t, preserved); after != before {
		t.Fatalf("partial legacy refusal mutated metadata:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestDeployed074PartialMigrationFailureRollsBackLedgerCanonicalization(t *testing.T) {
	cfg, db := openCanonical001Fixture(t)
	if _, err := db.Exec(`ALTER TABLE playback_receivers ADD COLUMN receiver_public_key TEXT NOT NULL DEFAULT ''`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE schema_migrations SET checksum_sha256 = ? WHERE version = '001_initial'`, legacy074Migration001Checksum); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE portico_database_identity SET migration_head = ?, migration_ledger_sha256 = ? WHERE id = 1`, legacy074MigrationHead, legacy074Migration001Ledger); err != nil {
		db.Close()
		t.Fatal(err)
	}
	before := migrationMetadataSnapshot(t, db)
	db.Close()
	opened, err := Open(cfg)
	if opened != nil {
		opened.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "run migration 002_playback_receiver_authority") {
		t.Fatalf("partial 002 error=%v", err)
	}
	preserved, openErr := sql.Open("sqlite", sqliteDSN(cfg.DatabasePath))
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer preserved.Close()
	if after := migrationMetadataSnapshot(t, preserved); after != before {
		t.Fatalf("failed partial migration canonicalized ledger outside transaction:\nbefore=%s\nafter=%s", before, after)
	}
	var receiverColumns, authorizationTable int
	if err := preserved.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('playback_receivers') WHERE name IN ('receiver_public_key','receiver_public_key_fingerprint','authorization_revision','expires_at','client_instance_id')`).Scan(&receiverColumns); err != nil {
		t.Fatal(err)
	}
	if err := preserved.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='playback_receiver_authorizations'`).Scan(&authorizationTable); err != nil {
		t.Fatal(err)
	}
	if receiverColumns != 1 || authorizationTable != 0 {
		t.Fatalf("failed partial migration changed application schema: columns=%d authorizationTable=%d", receiverColumns, authorizationTable)
	}
}

func TestMigrationBaselineRejectsUnsupportedLayoutsWithoutAlternateDatabase(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "portico.db")
	legacy, err := sql.Open("sqlite", sqliteDSN(databasePath))
	if err != nil {
		t.Fatalf("open legacy fixture: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE legacy_household_data (id TEXT PRIMARY KEY, value TEXT NOT NULL); INSERT INTO legacy_household_data VALUES ('keep', 'preserve-me')`); err != nil {
		t.Fatalf("seed legacy fixture: %v", err)
	}
	_ = legacy.Close()

	db, err := Open(config.Config{AppDataDir: root, DatabasePath: databasePath})
	if db != nil {
		_ = db.Close()
	}
	assertMigrationLayoutError(t, err)

	preserved, err := sql.Open("sqlite", sqliteDSN(databasePath))
	if err != nil {
		t.Fatalf("reopen preserved legacy fixture: %v", err)
	}
	defer preserved.Close()
	var value string
	if err := preserved.QueryRow(`SELECT value FROM legacy_household_data WHERE id = 'keep'`).Scan(&value); err != nil || value != "preserve-me" {
		t.Fatalf("unsupported layout was mutated: value=%q err=%v", value, err)
	}
	for _, metadata := range []string{"schema_migrations", "portico_database_identity"} {
		var count int
		if err := preserved.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, metadata).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unsupported unversioned rollback retained %s: count=%d err=%v", metadata, count, err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, "*.db"))
	if err != nil || !slices.Equal(matches, []string{databasePath}) {
		t.Fatalf("startup created an alternate database: matches=%v err=%v", matches, err)
	}
}

func TestMigrationLedgerRefusalsPreserveApplicationDataAndConfiguredPath(t *testing.T) {
	tests := []struct {
		name      string
		mutate    string
		integrity bool
	}{
		{name: "extra row", mutate: `INSERT INTO schema_migrations (version, filename, applied_at, checksum_sha256, checksum_source) VALUES ('003_unknown', '003_unknown.sql', 'fixture', 'fixture', 'fixture')`},
		{name: "version mismatch", mutate: `UPDATE schema_migrations SET version = '001_wrong' WHERE version = '001_initial'`},
		{name: "filename mismatch", mutate: `UPDATE schema_migrations SET filename = '001_wrong.sql' WHERE version = '001_initial'`},
		{name: "missing row", mutate: `DELETE FROM schema_migrations WHERE version = '001_initial'`},
		{name: "original Foundation 001 ledger", mutate: `UPDATE schema_migrations SET checksum_sha256 = '3efae2a9860854930ffe680d7acb65c48baf4491051cbf7357a7da7c9a3ce994' WHERE version = '001_initial'; UPDATE portico_database_identity SET migration_ledger_sha256 = '49b356a346fdda57dd3cfd8b635c2ba1930b4be49da7d4db57b0f10e1a044088' WHERE id = 1`, integrity: true},
		{name: "provisional d982 public 001 plus 002 ledger", mutate: `UPDATE schema_migrations SET checksum_sha256 = '3efae2a9860854930ffe680d7acb65c48baf4491051cbf7357a7da7c9a3ce994' WHERE version = '001_initial'; INSERT INTO schema_migrations (version, filename, applied_at, checksum_sha256, checksum_source) VALUES ('002_media_browse_personal_rating_index', '002_media_browse_personal_rating_index.sql', 'fixture', 'f258724596ff81d3a52b666df87a91500d068093aa9b2c569f3a8d7195ab659b', 'embedded-bundle'); UPDATE portico_database_identity SET migration_head = '002_media_browse_personal_rating_index', migration_ledger_sha256 = '3ff0d35cb0bfaffe7daeea18aae40b934b547bb26a83598946c18e602e5e673d' WHERE id = 1`},
		{name: "checksum corruption", mutate: `UPDATE schema_migrations SET checksum_sha256 = 'corrupt' WHERE version = '001_initial'`, integrity: true},
		{name: "identity ledger digest corruption", mutate: `UPDATE portico_database_identity SET migration_ledger_sha256 = 'corrupt' WHERE id = 1`, integrity: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			databasePath := filepath.Join(root, "portico.db")
			cfg := config.Config{AppDataDir: root, DatabasePath: databasePath}
			db, err := Open(cfg)
			if err != nil {
				t.Fatalf("open release database: %v", err)
			}
			if _, err := db.Exec(`CREATE TABLE ledger_preservation_probe (id TEXT PRIMARY KEY, value TEXT NOT NULL); INSERT INTO ledger_preservation_probe VALUES ('keep', 'preserve-me')`); err != nil {
				t.Fatalf("seed application preservation probe: %v", err)
			}
			if _, err := db.Exec(test.mutate); err != nil {
				t.Fatalf("mutate migration ledger: %v", err)
			}
			metadataBefore := migrationMetadataSnapshot(t, db)
			_ = db.Close()

			reopened, err := Open(cfg)
			if reopened != nil {
				_ = reopened.Close()
			}
			if test.integrity {
				assertMigrationIntegrityError(t, err)
			} else {
				assertMigrationLayoutError(t, err)
			}

			preserved, err := sql.Open("sqlite", sqliteDSN(databasePath))
			if err != nil {
				t.Fatalf("reopen refused database directly: %v", err)
			}
			defer preserved.Close()
			var schema, value string
			if err := preserved.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'ledger_preservation_probe'`).Scan(&schema); err != nil {
				t.Fatalf("read preserved application schema: %v", err)
			}
			if err := preserved.QueryRow(`SELECT value FROM ledger_preservation_probe WHERE id = 'keep'`).Scan(&value); err != nil || value != "preserve-me" {
				t.Fatalf("read preserved application data: value=%q err=%v", value, err)
			}
			if !strings.Contains(schema, "ledger_preservation_probe") {
				t.Fatalf("application schema was not preserved: %q", schema)
			}
			if metadataAfter := migrationMetadataSnapshot(t, preserved); metadataAfter != metadataBefore {
				t.Fatalf("refusal mutated migration metadata:\nbefore=%s\nafter=%s", metadataBefore, metadataAfter)
			}
			matches, err := filepath.Glob(filepath.Join(root, "*.db"))
			if err != nil || !slices.Equal(matches, []string{databasePath}) {
				t.Fatalf("refusal created an alternate database: matches=%v err=%v", matches, err)
			}
		})
	}
}

func migrationMetadataSnapshot(t *testing.T, db interface {
	Query(string, ...any) (*sql.Rows, error)
}) string {
	t.Helper()
	rows, err := db.Query(`
		SELECT 'identity', CAST(id AS TEXT), CAST(format_version AS TEXT), migration_head,
		       migration_ledger_sha256, minimum_reader, updated_at
		FROM portico_database_identity
		UNION ALL
		SELECT 'migration', CAST(rowid AS TEXT), version, filename,
		       checksum_sha256, checksum_source, applied_at
		FROM schema_migrations
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatalf("snapshot migration metadata: %v", err)
	}
	defer rows.Close()
	var snapshot strings.Builder
	for rows.Next() {
		var fields [7]string
		if err := rows.Scan(&fields[0], &fields[1], &fields[2], &fields[3], &fields[4], &fields[5], &fields[6]); err != nil {
			t.Fatalf("scan migration metadata snapshot: %v", err)
		}
		snapshot.WriteString(strings.Join(fields[:], "\x00") + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migration metadata snapshot: %v", err)
	}
	return snapshot.String()
}

func TestMigrationBaselineRejectsNoncanonicalIdentityAndLedger(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate string
	}{
		{name: "wrong format", mutate: `UPDATE portico_database_identity SET format_version = 0 WHERE id = 1`},
		{name: "wrong head", mutate: `UPDATE portico_database_identity SET migration_head = 'unexpected' WHERE id = 1`},
		{name: "blank checksum", mutate: `UPDATE schema_migrations SET checksum_sha256 = '' WHERE version = '001_initial'`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			cfg := config.Config{AppDataDir: root, DatabasePath: filepath.Join(root, "portico.db")}
			db, err := Open(cfg)
			if err != nil {
				t.Fatalf("open release database: %v", err)
			}
			if _, err := db.Exec(test.mutate); err != nil {
				t.Fatalf("mutate release identity: %v", err)
			}
			_ = db.Close()
			reopened, err := Open(cfg)
			if reopened != nil {
				_ = reopened.Close()
			}
			assertMigrationLayoutError(t, err)
		})
	}
}

func assertMigrationLayoutError(t *testing.T, err error) {
	t.Helper()
	var layout *MigrationLayoutError
	if !errors.As(err, &layout) {
		t.Fatalf("error=%v is not a MigrationLayoutError", err)
	}
	if layout.Code != MigrationLayoutRebuildRequiredCode || !strings.Contains(err.Error(), "rebuild required") || !strings.Contains(err.Error(), "application schema and data left unchanged") {
		t.Fatalf("unsupported-layout contract=%#v error=%v", layout, err)
	}
}

func assertMigrationIntegrityError(t *testing.T, err error) {
	t.Helper()
	var integrity *MigrationIntegrityError
	if !errors.As(err, &integrity) {
		t.Fatalf("error=%v is not a MigrationIntegrityError", err)
	}
	if integrity.Code != MigrationIntegrityFailureCode || !strings.Contains(err.Error(), "mismatch") || !strings.Contains(err.Error(), "application schema and data left unchanged") {
		t.Fatalf("migration-integrity contract=%#v error=%v", integrity, err)
	}
}
