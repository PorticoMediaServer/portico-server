package postgres_test

import (
	"context"
	"testing"

	"portico.local/hosted/internal/postgres"
)

func TestOwnerSwitchCountFreshSchemaPairConstraints(t *testing.T) {
	f := newOwnerReturn(t, "https://owner.example")
	f.register(t)
	ctx := context.Background()
	var absent bool
	if e := f.store.DB.QueryRowContext(ctx, `SELECT switch_count_payload IS NULL AND switch_count_signature IS NULL FROM owner_claim_starts WHERE transaction_id=$1`, f.pending.OwnerTransactionID).Scan(&absent); e != nil || !absent {
		t.Fatal("ordinary owner start did not preserve absent optional proof")
	}
	for _, sizes := range [][2]int{{-1, 64}, {1, -1}, {0, 64}, {8193, 64}, {1, 0}, {1, 63}, {1, 65}} {
		var payload, signature []byte
		if sizes[0] >= 0 {
			payload = make([]byte, sizes[0])
		}
		if sizes[1] >= 0 {
			signature = make([]byte, sizes[1])
		}
		_, e := f.store.DB.ExecContext(ctx, `UPDATE owner_claim_starts SET switch_count_payload=$2,switch_count_signature=$3 WHERE transaction_id=$1`, f.pending.OwnerTransactionID, payload, signature)
		requireSQLState(t, e, "23514")
	}
	for _, size := range []int{1, 8192} {
		if _, e := f.store.DB.ExecContext(ctx, `UPDATE owner_claim_starts SET switch_count_payload=$2,switch_count_signature=$3 WHERE transaction_id=$1`, f.pending.OwnerTransactionID, make([]byte, size), make([]byte, 64)); e != nil {
			t.Fatal("bounded complete proof pair refused", e)
		}
	}
	if _, e := f.store.DB.ExecContext(ctx, `UPDATE owner_claim_starts SET switch_count_payload=NULL,switch_count_signature=NULL WHERE transaction_id=$1`, f.pending.OwnerTransactionID); e != nil {
		t.Fatal(e)
	}
	var noDefaults bool
	if e := f.store.DB.QueryRowContext(ctx, `SELECT count(*)=2 AND bool_and(column_default IS NULL AND is_nullable='YES') FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='owner_claim_starts' AND column_name IN('switch_count_payload','switch_count_signature')`).Scan(&noDefaults); e != nil || !noDefaults {
		t.Fatal("optional proof columns gained defaults or became required")
	}
}

func TestOwnerSwitchCountFreshFormatRefusesPreviousMarker(t *testing.T) {
	f := currentFamilySetup(t)
	ctx := context.Background()
	if postgres.CurrentHostedSchemaVersion != 30 {
		t.Fatal("count proof baseline did not advance its format")
	}
	if e := f.store.Migrate(ctx); e != nil {
		t.Fatal("fresh current-format restart refused", e)
	}
	tx, e := f.store.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `UPDATE hosted_schema_contract SET version=25`); e != nil {
		t.Fatal(e)
	}
	if e = postgres.InitializeCurrentHostedSchemaTx(ctx, tx); e == nil {
		t.Fatal("previous format was silently adopted")
	}
	var version int
	if e = tx.QueryRowContext(ctx, `SELECT version FROM hosted_schema_contract WHERE singleton`).Scan(&version); e != nil || version != 25 {
		t.Fatal("refused format was rewritten")
	}
}
