package dbwork

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// statementCacheWarmTurns is the number of turns the warm loop runs. Each turn
// is one gated write transaction plus a foreground read and a background read,
// so it covers the shape the server takes a hundred times over while a viewer
// browses: repeated SQL text, changing bindings, one transaction per turn.
const statementCacheWarmTurns = 100

// nativeReprepares reads SQLite's own reprepare counter off a cached handle.
// The second return reports whether the handle is a native statement at all;
// a cache that held its own bytecode here would report false, which is the
// difference this test is written to see.
func nativeReprepares(entry *statementEntry) (int64, bool) {
	if entry == nil || entry.leased || entry.stmt == nil {
		return 0, false
	}
	counter, ok := entry.stmt.(interface{ StatementReprepareCount() int64 })
	if !ok {
		return 0, false
	}
	return counter.StatementReprepareCount(), true
}

// TestWarmStatementCacheReusesNativePlansAcrossTurns holds three physical
// connections for a hundred read/write turns and asserts that the statement
// cache answered every one of them: no miss (so no PrepareContext), no bypass
// (so no PRAGMA or DDL flushed it), and no eviction. A native reprepare, which
// happens inside sqlite3_step and never reaches the cache, is read from the
// driver's own counter on the cached handle.
//
// The control at the end is the case this test exists for: another connection
// changes the schema. The cache still answers from the retained statement and
// the counters still show a hit and no miss, so a cache-miss metric cannot see
// the regeneration. Only the native counter can, and only when this SQLite
// build maintains it.
func TestWarmStatementCacheReusesNativePlansAcrossTurns(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "warm.db")

	// Separate handles for the read pool, the writer and the background reader.
	// The server runs those as separate pools; the statement cache is per
	// physical connection, so a shared handle would hide the per-connection fact
	// this test asserts.
	open := func(name string) *sql.DB {
		t.Helper()
		db, err := sql.Open(DriverName, path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { db.Close() })
		return db
	}
	readDB, writeDB, backgroundDB, controlDB := open("read"), open("write"), open("background"), open("control")

	const (
		createShape = `CREATE TABLE warm(id INTEGER PRIMARY KEY, label TEXT NOT NULL, added INTEGER NOT NULL)`
		insertShape = `INSERT INTO warm(id,label,added) VALUES(?,?,?)`
		readShape   = `SELECT id,label FROM warm WHERE added>=? ORDER BY added,id LIMIT ?`
	)
	if _, err := writeDB.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	if _, err := writeDB.ExecContext(ctx, createShape); err != nil {
		t.Fatal(err)
	}
	// Production refreshes sqlite_stat1 before readers plan, and every ANALYZE
	// expires statements on the connection that ran it. Do the same here so the
	// warm loop starts from the same state the server's pools start from.
	if _, err := writeDB.ExecContext(ctx, `ANALYZE`); err != nil {
		t.Fatal(err)
	}
	if _, err := writeDB.ExecContext(ctx, `INSERT INTO warm(id,label,added) VALUES(1,'seed',0)`); err != nil {
		t.Fatal(err)
	}

	reader, err := readDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	background, err := backgroundDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer background.Close()

	read := func(ctx context.Context, conn *sql.Conn, floor int64) error {
		rows, err := conn.QueryContext(ctx, readShape, floor, 5)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
		}
		return rows.Err()
	}
	turn := func(ctx context.Context, id int64) error {
		if err := WithWriteTx(ctx, writeDB, ClassBackgroundMedia, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, insertShape, id, "warm", id)
			return err
		}); err != nil {
			return err
		}
		if err := read(WithClass(ctx, ClassInteractive), reader, 0); err != nil {
			return err
		}
		return read(WithClass(ctx, ClassBackgroundMedia), background, 0)
	}

	// One warm turn before measuring: the first execution of each distinct text
	// is a real native prepare, and this test is about the turns after it.
	if err := turn(ctx, 1); err != nil {
		t.Fatal(err)
	}
	readerEntry := cacheEntryFor(t, reader, readShape)
	backgroundEntry := cacheEntryFor(t, background, readShape)
	if readerEntry == nil || backgroundEntry == nil {
		t.Fatal("the warm turn did not leave an idle cached handle")
	}
	if readerEntry == backgroundEntry {
		t.Fatal("two physical connections share one cache entry; the cache is not per connection")
	}
	warmReprepares, nativeHandle := nativeReprepares(readerEntry)
	if !nativeHandle {
		t.Fatal("the cache holds something other than the driver's prepared statement")
	}

	ResetReads()
	for id := int64(2); id <= statementCacheWarmTurns+1; id++ {
		if err := turn(ctx, id); err != nil {
			t.Fatalf("turn %d: %v", id, err)
		}
	}
	stats := statementCacheStats()
	if stats.Misses != 0 {
		t.Fatalf("%d of %d warm turns re-prepared SQL inside the cache; the text or its cache line changed", stats.Misses, statementCacheWarmTurns)
	}
	if stats.Bypasses != 0 || stats.Evictions != 0 {
		t.Fatalf("warm turns bypassed or evicted: %+v", stats)
	}
	if stats.Hits < statementCacheWarmTurns {
		t.Fatalf("%d hits for %d turns: the cache did not answer every turn", stats.Hits, statementCacheWarmTurns)
	}
	afterWarm, _ := nativeReprepares(cacheEntryFor(t, reader, readShape))
	if afterWarm != warmReprepares {
		t.Fatalf("a warm turn regenerated the native plan %d times with no Prepare and no cache miss", afterWarm-warmReprepares)
	}

	// Control: a third connection changes the schema. The retained programs on
	// every other connection are now stale, so sqlite3_step must regenerate them
	// before they can run — and the cache will still report a hit, because the
	// lookup found the same text and the same handle.
	if _, err := controlDB.ExecContext(ctx, `ALTER TABLE warm ADD COLUMN note TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatal(err)
	}
	hitsBefore := statementCacheStats().Hits
	if err := turn(ctx, statementCacheWarmTurns+2); err != nil {
		t.Fatalf("warm turn after another connection changed the schema: %v", err)
	}
	controlStats := statementCacheStats()
	if controlStats.Hits <= hitsBefore {
		t.Fatal("the schema change made the cache re-prepare the text instead of answering from the retained statement")
	}
	if controlStats.Misses != 0 {
		t.Fatalf("%d misses after the schema change: the text was re-prepared by the cache, not regenerated by SQLite", controlStats.Misses)
	}
	if got := cacheEntryFor(t, reader, readShape); got != readerEntry {
		t.Fatal("the schema change replaced the cached handle; this run did not exercise native regeneration")
	}
	afterControl, _ := nativeReprepares(readerEntry)
	if afterControl > afterWarm {
		t.Logf("native reprepares observed after the schema change: %d", afterControl-afterWarm)
	} else {
		// Slot 5 exists in this SQLite translation but has no increment site, so
		// the control cannot be read from it. Say so rather than pass a zero off
		// as evidence that no native work happened.
		t.Logf("SQLite build does not maintain SQLITE_STMTSTATUS_REPREPARE; the control is behavioural only")
	}
}
