package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"portico.local/apikit/idempotency"
)

type idempotencySQLTx struct{ tx *sql.Tx }

func (w idempotencySQLTx) Tx() *sql.Tx     { return w.tx }
func (w idempotencySQLTx) Commit() error   { return w.tx.Commit() }
func (w idempotencySQLTx) Rollback() error { return w.tx.Rollback() }

func TestAPIIdempotencyConcurrentPostgresReplay(t *testing.T) {
	store := database(t) // owns an isolated schema in the disposable test database
	db := store.DB
	db.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, statement := range []string{
		`CREATE TABLE api_idempotency(scope TEXT NOT NULL,key TEXT NOT NULL,digest TEXT NOT NULL,status INTEGER NOT NULL,body BYTEA NOT NULL,headers TEXT NOT NULL DEFAULT '{}',expires_at BIGINT NOT NULL,PRIMARY KEY(scope,key))`,
		`CREATE TABLE idempotency_applied(value INTEGER NOT NULL)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	begin := func(ctx context.Context) (idempotency.Transaction, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		return idempotencySQLTx{tx}, nil
	}
	receipts := idempotency.Store{Begin: begin, SweepBegin: begin, Lifetime: time.Hour}
	request := idempotency.Request{Method: "POST", Path: "/v1/items/concrete", RawQuery: "b=2&a=1", Body: []byte(`{"title":"Example"}`)}
	mutate := func(tx *sql.Tx) (idempotency.Result, error) {
		_, err := tx.ExecContext(ctx, `INSERT INTO idempotency_applied(value) VALUES(1)`)
		return idempotency.Result{Status: 201, Body: []byte(`{"id":"item"}`), Headers: map[string]string{"Location": "/v1/items/item"}}, err
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 12 {
		wg.Go(func() {
			out, err := receipts.Execute(ctx, "viewer/device", "postgres_request_0001", request, mutate)
			if err == nil && (out.Status != 201 || out.Headers["Location"] != "/v1/items/item") {
				err = fmt.Errorf("incomplete receipt: %+v", out)
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var applied int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM idempotency_applied`).Scan(&applied); err != nil || applied != 1 {
		t.Fatalf("concurrent requests applied %d times: %v", applied, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE api_idempotency SET expires_at=0`); err != nil {
		t.Fatal(err)
	}
	if removed, err := receipts.Sweep(ctx, 1); err != nil || removed != 1 {
		t.Fatalf("Postgres receipt sweep removed %d: %v", removed, err)
	}
}
