package dbwork

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestWriterProgressesWithEntireReadPoolOccupied(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var readers []*sql.Conn
	for range db.Stats().MaxOpenConnections {
		c, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		readers = append(readers, c)
	}
	defer func() {
		for _, c := range readers {
			c.Close()
		}
	}()
	for _, class := range []Class{ClassInteractive, ClassSecurityFence} {
		if err := WithWriteTx(ctx, db, class, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO rows(value) VALUES('write')`)
			return err
		}); err != nil {
			t.Fatal("readers starved writer", class, err)
		}
	}
	if _, err := ExecWrite(ctx, db, ClassInteractive, `INSERT INTO rows(value) VALUES('exec')`); err != nil {
		t.Fatal("readers starved ExecWrite", err)
	}
	reserved, conn, release, err := ReserveWriterConn(ctx, db, ClassMaintenance)
	if err != nil {
		t.Fatal("readers starved writer reservation", err)
	}
	defer release()
	w, err := BeginConn(reserved, conn, ClassMaintenance)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Rollback()
	if _, err = w.Tx().ExecContext(reserved, `INSERT INTO rows(value) VALUES('reserved')`); err != nil {
		t.Fatal(err)
	}
	if err = w.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestClosingPublicHandleClosesWriterPool(t *testing.T) {
	db := testDB(t)
	writer := writerHandle(db)
	background := ReadHandle(WithClass(context.Background(), ClassBackgroundMedia), db)
	if writer == db {
		t.Fatal("writer not separate")
	}
	if background == db || background == writer || background.Stats().MaxOpenConnections != 2 || db.Stats().MaxOpenConnections != 6 {
		t.Fatal("foreground/background pools are not isolated")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Ping(); err == nil {
		t.Fatal("writer survived database close")
	}
	if err := background.Ping(); err == nil {
		t.Fatal("background reader survived database close")
	}
}

func TestBackgroundAndForegroundReadPoolsCannotStarveEachOther(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	foreground := []*sql.Conn{}
	for range 6 {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		foreground = append(foreground, conn)
	}
	defer func() {
		for _, conn := range foreground {
			conn.Close()
		}
	}()
	backgroundCtx := WithClass(ctx, ClassBackgroundMedia)
	if err := QueryRow(backgroundCtx, db, `SELECT 1`).Scan(new(int)); err != nil {
		t.Fatal("foreground readers starved background", err)
	}
	for _, conn := range foreground {
		conn.Close()
	}
	foreground = nil
	background := ReadHandle(backgroundCtx, db)
	backgroundConns := []*sql.Conn{}
	for range 2 {
		conn, err := background.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		backgroundConns = append(backgroundConns, conn)
	}
	defer func() {
		for _, conn := range backgroundConns {
			conn.Close()
		}
	}()
	if err := QueryRow(ctx, db, `SELECT 1`).Scan(new(int)); err != nil {
		t.Fatal("background readers starved foreground", err)
	}
}
