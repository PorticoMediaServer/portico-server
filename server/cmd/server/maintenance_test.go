package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"portico.local/apikit/idempotency"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

func TestReceiptSweepSkipsEmptyGateAndDrainsBacklogInBatches(t *testing.T) {
	ctx := context.Background()
	db, err := persistence.Open(filepath.Join(t.TempDir(), "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	starts := 0
	receipts := idempotency.Store{SweepBegin: func(ctx context.Context) (idempotency.Transaction, error) {
		starts++
		return dbwork.Begin(ctx, db, dbwork.ClassMaintenance)
	}}
	if backlog, err := sweepDueReceipts(ctx, db, receipts); err != nil || backlog || starts != 0 {
		t.Fatalf("empty sweep: backlog=%v starts=%d err=%v", backlog, starts, err)
	}
	_, err = db.ExecContext(ctx, `WITH RECURSIVE seq(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM seq WHERE x<1050)
		INSERT INTO api_idempotency(scope,key,digest,status,body,expires_at)
		SELECT 'test',printf('key-%04d',x),'digest',200,x'',? FROM seq`, time.Now().Add(-time.Minute).Unix())
	if err != nil {
		t.Fatal(err)
	}
	if backlog, err := sweepDueReceipts(ctx, db, receipts); err != nil || !backlog || starts != 10 {
		t.Fatalf("first sweep: backlog=%v starts=%d err=%v", backlog, starts, err)
	}
	if backlog, err := sweepDueReceipts(ctx, db, receipts); err != nil || backlog || starts != 11 {
		t.Fatalf("remaining sweep: backlog=%v starts=%d err=%v", backlog, starts, err)
	}
	var remaining int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM api_idempotency`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("remaining receipts=%d err=%v", remaining, err)
	}
}
