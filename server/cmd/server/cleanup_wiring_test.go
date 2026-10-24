package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

func TestMinuteCleanupWakesForItsInputsAndUnknownWrites(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "cleanup.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO server_settings(singleton) VALUES(1) ON CONFLICT(singleton) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	wake, stop := minuteCleanupWake()
	defer stop()
	write := func(query string) {
		t.Helper()
		if err := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
			_, err := tx.Exec(query)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	write(`INSERT INTO telemetry_rollups(bucket_ms,metric,samples,total,status,detail) VALUES(0,'cpu',1,1,'available','')`)
	if wake.Take() {
		t.Fatal("unrelated known telemetry publication woke retention")
	}
	write(`UPDATE server_settings SET updated_ms=updated_ms+1 WHERE singleton=1`)
	if !wake.Take() {
		t.Fatal("changed missing-title settings did not wake retention")
	}
	write(`WITH changed(value) AS (SELECT 1) UPDATE telemetry_rollups SET total=total+(SELECT value FROM changed) WHERE bucket_ms=0`)
	if !wake.Take() {
		t.Fatal("unknown write scope lost the conservative retention wake")
	}
	stop()
	write(`UPDATE server_settings SET updated_ms=updated_ms+1 WHERE singleton=1`)
	if wake.Take() {
		t.Fatal("retired retention subscription was woken")
	}
}
