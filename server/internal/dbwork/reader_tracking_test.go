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
	ctx := context.Background()
	baseline := ReaderLifetimes().Snapshots
	check := func(want int) {
		t.Helper()
		if stats := ReaderLifetimes(); stats.Snapshots != baseline+want {
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
	waitReaderCondition(t, func() bool { return ReaderLifetimes().Snapshots == baseline })
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
