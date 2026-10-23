package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/worker"
)

func TestListeningGroupWakesUseCanonicalPublications(t *testing.T) {
	ctx := context.Background()
	db, err := persistence.Open(filepath.Join(t.TempDir(), "listening-wakes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var book int64
	err = dbwork.WithWriteTx(ctx, db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO libraries(id,name,kind,root) VALUES('wake-books','Wake fixture','audiobook','/fixture/books')`); err != nil {
			return err
		}
		library, err := compactcatalog.LibraryTx(ctx, tx, "wake-books")
		if err != nil {
			return err
		}
		book, _, err = compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Book, Key: "book:wake", Title: "Wake fixture"})
		if err != nil {
			return err
		}
		if err = compactcatalog.SetFactsTx(ctx, tx, book, map[string]any{"library_id": library, "local_key": "wake", "author": "First author"}); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO catalog_book_context(book_id,library_id,series_name) VALUES(?,?,'First series') ON CONFLICT(book_id) DO UPDATE SET series_name=excluded.series_name`, book, library)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = dbwork.InstallWakeDependencies(ctx, db); err != nil {
		t.Fatal(err)
	}
	wake, changed := worker.NewSignal(), worker.NewSignal()
	stop := subscribeListeningGroups(wake, changed)
	defer stop()
	for _, tc := range []struct {
		name, statement string
		args            []any
		want            bool
	}{
		{"book-author", `UPDATE catalog_books SET author='Second author' WHERE entity_id=?`, []any{book}, true},
		{"derived-series", `UPDATE catalog_book_context SET series_name='Second series' WHERE book_id=?`, []any{book}, true},
		{"unrelated-maintenance-setting", `UPDATE maintenance_settings SET background_speed='fast' WHERE singleton=1`, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := dbwork.ChangingCommits()
			err := dbwork.WithWriteTx(ctx, db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, tc.statement, tc.args...)
				return err
			})
			if err != nil || dbwork.ChangingCommits() <= before {
				t.Fatalf("fixture did not commit a real change: %v", err)
			}
			gotWake, gotChanged := wake.Take(), changed.Take()
			if gotWake != tc.want || gotChanged != tc.want {
				t.Fatalf("wake=%t changed=%t want=%t", gotWake, gotChanged, tc.want)
			}
		})
	}
}
