package tableretention

import (
	"context"
	"fmt"
	"path/filepath"
	"portico.local/server/internal/persistence"
	"testing"
	"time"
)

func TestSweepBoundsReceiptsAndPreservesLiveWork(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for n := 0; n < 7; n++ {
		if _, err = db.Exec(`INSERT INTO admin_receipts VALUES('server',?,'digest','{}',0)`, fmt.Sprintf("old%d", n)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`UPDATE maintenance_retention_rows SET created_ms=0 WHERE table_name='admin_receipts'`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO admin_receipts VALUES('server','fresh','digest','{}',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO prepared_media_jobs(id,item_id,asset_id,library_id,profile_id,target_id,selection_json,source_revision,principal_json,state,phase,created_ms,updated_ms) VALUES('active',1,'a','l','p','t','{}','r','{}','running','copy',0,0),('retired',1,'a','l','p','t','{}','r','{}','failed','failed',0,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE maintenance_retention_rows SET created_ms=0 WHERE table_name='prepared_media_jobs'`); err != nil {
		t.Fatal(err)
	}
	more, err := Sweep(context.Background(), db, time.Now(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("backlog not reported")
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM admin_receipts`).Scan(&count); err != nil || count != 5 {
		t.Fatalf("batch count %d: %v", count, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM prepared_media_jobs WHERE id='active'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("active work deleted: %v", err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM prepared_media_jobs WHERE id='retired'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("terminal work retained: %v", err)
	}
	for more {
		more, err = Sweep(context.Background(), db, time.Now(), 3)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = db.QueryRow(`SELECT count(*) FROM admin_receipts`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("fresh receipt not retained: %v", err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM maintenance_retention_rows WHERE table_name='admin_receipts'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("age ledger leaked: %v", err)
	}
}

func TestPermanentBoundKeepsExactReplayAndUpdateAtTheCeiling(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE maintenance_table_bounds SET maximum=2 WHERE table_name='saved_operation_fences'`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		if _, err = db.Exec(`INSERT INTO saved_operation_fences VALUES('p','personal',?,'hash','now')`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT OR IGNORE INTO saved_operation_fences VALUES('p','personal','one','hash','now')`); err != nil {
		t.Fatalf("exact retry refused: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO saved_operation_fences VALUES('p','personal','three','hash','now')`); err == nil {
		t.Fatal("bound bypassed")
	}
	if _, err = db.Exec(`UPDATE saved_operation_fences SET request_hash='changed' WHERE operation_id='one'`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT rows FROM maintenance_table_bounds WHERE table_name='saved_operation_fences'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("count %d: %v", count, err)
	}
	if _, err = db.Exec(`DELETE FROM saved_operation_fences WHERE operation_id='one'`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO saved_operation_fences VALUES('p','personal','three','hash','now')`); err != nil {
		t.Fatalf("released capacity not usable: %v", err)
	}
	for table, maximum := range PermanentBounds {
		var got int
		if err = db.QueryRow(`SELECT maximum FROM maintenance_table_bounds WHERE table_name=?`, table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if table != "saved_operation_fences" && got != maximum {
			t.Fatalf("%s ceiling %d want %d", table, got, maximum)
		}
	}
}
