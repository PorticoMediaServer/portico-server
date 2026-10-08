package postgres_test

import (
	"context"
	_ "embed"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

//go:embed testdata/legacy_v2_schema.sql
var legacyV2Schema string

// openSchema returns a store on a new, empty schema (cleaned up after the test).
func openSchema(t *testing.T) *postgres.Store {
	t.Helper()
	path := os.Getenv("PORTICO_TEST_DATABASE_URL_FILE")
	if path == "" {
		t.Skip("set PORTICO_TEST_DATABASE_URL_FILE to isolated PostgreSQL test URL file")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	dsn, e := url.Parse(strings.TrimSpace(string(raw)))
	if e != nil || !strings.Contains(dsn.Path, "test") {
		t.Fatal("refusing database not named test")
	}
	ctx := context.Background()
	admin, e := postgres.Open(ctx, dsn.String())
	if e != nil {
		t.Fatal("test PostgreSQL unavailable")
	}
	schema := "legacy" + strings.ToLower(strings.NewReplacer("-", "x", "_", "x").Replace(security.RandomID("")[:15]))
	if _, e = admin.DB.ExecContext(ctx, `CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		c, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_, _ = admin.DB.ExecContext(c, `DROP SCHEMA `+schema+` CASCADE`)
		admin.DB.Close()
	})
	q := dsn.Query()
	q.Set("search_path", schema)
	dsn.RawQuery = q.Encode()
	store, e := postgres.Open(ctx, dsn.String())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.DB.Close() })
	return store
}

// catalogue describes a schema independently of column order: columns,
// constraints, indexes, triggers and function bodies.
func catalogue(t *testing.T, s *postgres.Store) string {
	t.Helper()
	rows, e := s.DB.Query(`
SELECT 'column '||c.table_name||'.'||c.column_name||' '||c.data_type||' '||c.is_nullable||' '||COALESCE(c.column_default,'') FROM information_schema.columns c WHERE c.table_schema=current_schema() AND c.table_name<>'hosted_schema_migrations'
UNION ALL SELECT 'constraint '||r.relname||'.'||x.conname||' '||pg_get_constraintdef(x.oid) FROM pg_constraint x JOIN pg_class r ON r.oid=x.conrelid JOIN pg_namespace n ON n.oid=r.relnamespace WHERE n.nspname=current_schema()
UNION ALL SELECT 'index '||indexname||' '||regexp_replace(indexdef,' ON [^ .]+\.',' ON ') FROM pg_indexes WHERE schemaname=current_schema()
UNION ALL SELECT 'trigger '||tgname||' '||regexp_replace(pg_get_triggerdef(t.oid),' ON [^ .]+\.',' ON ') FROM pg_trigger t JOIN pg_class r ON r.oid=t.tgrelid JOIN pg_namespace n ON n.oid=r.relnamespace WHERE n.nspname=current_schema() AND NOT t.tgisinternal
UNION ALL SELECT 'function '||p.proname||' '||md5(p.prosrc) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema()`)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		if e = rows.Scan(&line); e != nil {
			t.Fatal(e)
		}
		if strings.HasPrefix(line, "constraint ") {
			// A dump reload can regroup a CHECK's parentheses; the condition is the same.
			line = strings.NewReplacer("(", "", ")", "").Replace(line)
		}
		out = append(out, line)
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// A45: production Hosted predates numbered migrations (contract version 2, no
// history). It adopts into the migration chain with its rows intact and ends in
// exactly the schema a fresh install has.
func TestLegacyVersion2DatabaseAdoptsIntoMigrations(t *testing.T) {
	ctx := context.Background()
	legacy := openSchema(t)
	if _, e := legacy.DB.ExecContext(ctx, legacyV2Schema); e != nil {
		t.Fatal(e)
	}
	key := strings.Repeat("A", 43)
	if _, e := legacy.DB.ExecContext(ctx, `
INSERT INTO accounts(id,username,display_name,password_hash) VALUES('acct_legacy','legacy','Legacy','\x01');
INSERT INTO profiles(id,account_id,name) VALUES('prof_legacy','acct_legacy','Legacy');
INSERT INTO servers(id,owner_account_id,name,public_key,token_hash,token_ciphertext,claim_generation,credential_generation) VALUES('srv_`+key+`','acct_legacy','Den','`+key+`','\x`+strings.Repeat("11", 32)+`','\x`+strings.Repeat("22", 32)+`',1,1);
INSERT INTO memberships(server_id,account_id,profile_id,role) VALUES('srv_`+key+`','acct_legacy','prof_legacy','owner');`); e != nil {
		t.Fatal(e)
	}
	if e := legacy.Migrate(ctx); e != nil {
		t.Fatalf("legacy database refused: %v", e)
	}
	if e := legacy.Migrate(ctx); e != nil {
		t.Fatalf("restart after adoption: %v", e)
	}
	var version, history, accounts, memberships int
	var name string
	if e := legacy.DB.QueryRow(`SELECT (SELECT version FROM hosted_schema_contract),(SELECT count(*) FROM hosted_schema_migrations),(SELECT count(*) FROM accounts),(SELECT count(*) FROM memberships),(SELECT name FROM servers)`).Scan(&version, &history, &accounts, &memberships, &name); e != nil {
		t.Fatal(e)
	}
	if version != postgres.CurrentHostedSchemaVersion || history != postgres.CurrentHostedSchemaVersion || accounts != 1 || memberships != 1 || name != "Den" {
		t.Fatalf("version=%d history=%d accounts=%d memberships=%d server=%q", version, history, accounts, memberships, name)
	}
	fresh := openSchema(t)
	if e := fresh.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if got, want := catalogue(t, legacy), catalogue(t, fresh); got != want {
		gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
		have := map[string]bool{}
		for _, l := range gotLines {
			have[l] = true
		}
		need := map[string]bool{}
		for _, l := range wantLines {
			need[l] = true
		}
		for _, l := range wantLines {
			if !have[l] {
				t.Errorf("adopted schema lacks: %s", l)
			}
		}
		for _, l := range gotLines {
			if !need[l] {
				t.Errorf("adopted schema has extra: %s", l)
			}
		}
	}
}

// The pre-deploy rehearsal: a dry run on a restored production copy reports the
// upgrade and leaves the database exactly as it was.
func TestMigrateDryRunLeavesTheDatabaseUnchanged(t *testing.T) {
	ctx := context.Background()
	legacy := openSchema(t)
	if _, e := legacy.DB.ExecContext(ctx, legacyV2Schema); e != nil {
		t.Fatal(e)
	}
	before, after, e := legacy.MigrateDryRun(ctx)
	if e != nil || before != 2 || after != postgres.CurrentHostedSchemaVersion {
		t.Fatal(before, after, e)
	}
	var history bool
	var version int
	if e = legacy.DB.QueryRow(`SELECT to_regclass(current_schema()||'.hosted_schema_migrations') IS NOT NULL,(SELECT version FROM hosted_schema_contract)`).Scan(&history, &version); e != nil || history || version != 2 {
		t.Fatal("dry run changed the database", history, version, e)
	}
}

// A76: a dry run on an empty database installs everything, then rolls back.
func TestMigrateDryRunOnAnEmptyDatabase(t *testing.T) {
	empty := openSchema(t)
	before, after, e := empty.MigrateDryRun(context.Background())
	if e != nil || before != 0 || after != postgres.CurrentHostedSchemaVersion {
		t.Fatal(before, after, e)
	}
	var tables int
	if e = empty.DB.QueryRow(`SELECT count(*) FROM pg_tables WHERE schemaname=current_schema()`).Scan(&tables); e != nil || tables != 0 {
		t.Fatal("dry run left tables behind", tables, e)
	}
}
