package postgres_test

import (
	"context"
	"testing"

	"portico.local/hosted/internal/postgres"
)

// A43: retirement queues one member-data cleanup job per member account. Those
// jobs belong to no account deletion, so a NOT NULL deletion_id made owner
// removal and "This replaces my old server" fail for every server.
func TestServerRetirementQueuesCleanupWithoutAnAccountDeletion(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tx, e := f.store.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if e = postgres.RetireServerTxForTest(ctx, tx, f.owner.Account.ID, f.server, 1, 1, "owner_removed", ""); e != nil {
		t.Fatalf("retire: %v", e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	var jobs, orphaned int
	if e = f.store.DB.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER (WHERE deletion_id IS NULL) FROM account_erasure_jobs WHERE server_id=$1`, f.server).Scan(&jobs, &orphaned); e != nil {
		t.Fatal(e)
	}
	if jobs == 0 || orphaned != jobs {
		t.Fatalf("cleanup jobs=%d without deletion=%d", jobs, orphaned)
	}
	var revoked bool
	var reason string
	if e = f.store.DB.QueryRowContext(ctx, `SELECT revoked_at IS NOT NULL,retired_reason FROM servers WHERE id=$1`, f.server).Scan(&revoked, &reason); e != nil {
		t.Fatal(e)
	}
	if !revoked || reason != "owner_removed" {
		t.Fatalf("server not retired: revoked=%v reason=%q", revoked, reason)
	}
	var live int
	if e = f.store.DB.QueryRowContext(ctx, `SELECT count(*) FROM memberships WHERE server_id=$1 AND NOT revoked`, f.server).Scan(&live); e != nil || live != 0 {
		t.Fatalf("memberships still live: %d %v", live, e)
	}
	// A stale generation tuple can never retire the server again.
	tx2, e := f.store.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx2.Rollback()
	if e = postgres.RetireServerTxForTest(ctx, tx2, f.owner.Account.ID, f.server, 1, 1, "owner_removed", ""); e == nil {
		t.Fatal("retired server retired twice")
	}
}
