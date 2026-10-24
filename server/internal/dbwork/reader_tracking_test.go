package dbwork

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func readerFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenHandle(filepath.Join(t.TempDir(), "readers.sqlite"), DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = ExecWrite(context.Background(), db, ClassInteractive, `CREATE TABLE sample(id INTEGER); INSERT INTO sample VALUES(1),(2)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestReaderLifetimesCoverSnapshotHelpersAndDirectTransactions(t *testing.T) {
	db := readerFixture(t)
	// ReaderLifetimes is process-wide, and another fixture can still be
	// physically releasing a reader after its test has returned: database/sql
	// rolls a canceled transaction back from a goroutine, so joining the test's
	// own goroutines does not join that cleanup. Measure this fixture's scope so
	// a foreign lease cannot move the baseline under the assertions.
	scope := readerScopeForTest(t, db)
	ctx := context.Background()
	baseline := scope.stats().Snapshots
	defer func() {
		if t.Failed() {
			t.Logf("reader lifetime baseline=%d final=%+v", baseline, scope.stats())
		}
	}()
	check := func(want int) {
		t.Helper()
		if stats := scope.stats(); stats.Snapshots != baseline+want {
			t.Fatalf("snapshot lifetime: %+v, want%d", stats, baseline+want)
		}
	}
	read, closeRead, err := BeginRead(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	check(1)
	var count int
	if err = read.QueryRowContext(ctx, `SELECT count(*) FROM sample`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	check(1) // Closing a statement must not release its transaction snapshot.
	closeRead()
	closeRead()
	check(0)
	snapshot, err := BeginSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	check(1)
	if err = snapshot.Commit(); err != nil {
		t.Fatal(err)
	}
	check(0)
	if err = WithReadSnapshot(ctx, db, func(snap context.Context) error {
		check(1)
		return QueryRow(snap, db, `SELECT count(*) FROM sample`).Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	check(0)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	check(1)
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	check(0)
	cancelCtx, cancel := context.WithCancel(ctx)
	tx, err = db.BeginTx(cancelCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	check(1)
	cancel()
	waitReaderCondition(t, func() bool { return scope.stats().Snapshots == baseline })
	tx, err = db.Begin() // Legacy/default transactions also own a snapshot.
	if err != nil {
		t.Fatal(err)
	}
	check(1)
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	check(0)
}

func TestReaderLifetimesCoverImplicitRowsPreparedRowsAndErrors(t *testing.T) {
	db := readerFixture(t)
	baseline := ReaderLifetimes().ImplicitRows
	check := func(want int) {
		t.Helper()
		if stats := ReaderLifetimes(); stats.ImplicitRows != baseline+want {
			t.Fatalf("implicit lifetime: %+v, want%d", stats, baseline+want)
		}
	}
	rows, err := db.Query(`SELECT id FROM sample`)
	if err != nil {
		t.Fatal(err)
	}
	check(1)
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	check(0)
	stmt, err := db.PrepareContext(context.Background(), `SELECT id FROM sample`)
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	rows, err = stmt.QueryContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	check(1)
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	check(0) // database/sql's EOF auto-close must release the implicit reader.
	if _, err = db.Query(`SELECT missing_column FROM sample`); err == nil {
		t.Fatal("invalid query succeeded")
	}
	check(0)
	cancelCtx, cancel := context.WithCancel(context.Background())
	rows, err = db.QueryContext(cancelCtx, `SELECT id FROM sample`)
	if err != nil {
		t.Fatal(err)
	}
	check(1)
	cancel()
	waitReaderCondition(t, func() bool { return ReaderLifetimes().ImplicitRows == baseline })
	if !errors.Is(rows.Err(), context.Canceled) {
		t.Fatalf("returned rows did not observe cancellation: %v", rows.Err())
	}
	if err = WithWriteTx(context.Background(), db, ClassInteractive, func(tx *sql.Tx) error {
		if stats := ReaderLifetimes(); stats.Snapshots != 0 {
			t.Fatalf("gated writer counted as snapshot: %+v", stats)
		}
		var count int
		err := tx.QueryRow(`SELECT count(*) FROM sample`).Scan(&count)
		check(0)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReaderLifetimeScopeIsolatesForeignFixture(t *testing.T) {
	fixture := readerFixture(t)
	foreign := readerFixture(t)
	scope := readerScopeForTest(t, fixture)
	foreignScope := readerScopeForTest(t, foreign)
	baseline := scope.stats().Snapshots

	_, closeForeign, err := BeginRead(context.Background(), foreign)
	if err != nil {
		t.Fatal(err)
	}
	defer closeForeign()
	if stats := foreignScope.stats(); stats.Snapshots != 1 {
		t.Fatalf("foreign snapshot lifetime: %+v, want1", stats)
	}
	if stats := scope.stats(); stats.Snapshots != baseline {
		t.Fatalf("foreign lease entered the fixture scope: %+v, want%d", stats, baseline)
	}
	if stats := ReaderLifetimes(); stats.Snapshots < baseline+1 {
		t.Fatalf("process-wide lifetime missed the foreign lease: %+v", stats)
	}

	ctx, cancel := context.WithCancel(context.Background())
	tx, err := fixture.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sample`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if stats := scope.stats(); stats.Snapshots != baseline+1 {
		t.Fatalf("fixture snapshot lifetime: %+v, want%d", stats, baseline+1)
	}
	cancel()
	waitReaderCondition(t, func() bool { return scope.stats().Snapshots == baseline })
	// The foreign lease is deliberately still held: convergence here proves the
	// fixture measures its own scope, not the process-wide aggregate that made
	// the old baseline non-deterministic.
	if stats := foreignScope.stats(); stats.Snapshots != 1 {
		t.Fatalf("foreign snapshot ended during fixture convergence: %+v", stats)
	}
	if stats := ReaderLifetimes(); stats.Snapshots < baseline+1 {
		t.Fatalf("process-wide lifetime lost the foreign lease: %+v", stats)
	}
}
