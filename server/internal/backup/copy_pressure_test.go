package backup

import (
	"context"
	"errors"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"testing"
	"time"
)

func TestBackupReleasesSnapshotUnderWALPressure(t *testing.T) {
	_, state, db := testService(t)
	if _, err := db.Exec(`CREATE TABLE pressure_data(id INTEGER PRIMARY KEY,body BLOB);INSERT INTO pressure_data(body) VALUES(zeroblob(1048576))`); err != nil {
		t.Fatal(err)
	}
	wrote := false
	err := copyDatabaseLimited(context.Background(), db, filepath.Join(state, "pressure-copy.sqlite"), func(done, total int64) {
		if !wrote {
			wrote = true
			if _, err := db.Exec(`INSERT INTO pressure_data(body) VALUES(zeroblob(65536))`); err != nil {
				t.Fatal(err)
			}
		}
	}, copyLimits{time.Second, 1, 0})
	if !errors.Is(err, ErrBackupPressure) {
		t.Fatalf("unbounded snapshot under writes: %v", err)
	}
	if !wrote {
		t.Fatal("fixture did not create WAL pressure")
	}
	// An aborted backup releases its reader: SQLite can truncate the WAL immediately.
	var busy, log, checkpointed int
	if err := db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &log, &checkpointed); err != nil || busy != 0 {
		t.Fatalf("snapshot remained pinned: busy=%d %v", busy, err)
	}
	if size, err := walBytes(dbwork.DatabaseFile(context.Background(), db) + "-wal"); err != nil || size != 0 {
		t.Fatalf("WAL did not truncate: %d %v", size, err)
	}
}

func TestBackupSnapshotHasTimeBound(t *testing.T) {
	_, state, db := testService(t)
	if _, err := db.Exec(`CREATE TABLE slow_data(body BLOB);INSERT INTO slow_data VALUES(zeroblob(1048576))`); err != nil {
		t.Fatal(err)
	}
	err := copyDatabaseLimited(context.Background(), db, filepath.Join(state, "slow-copy.sqlite"), func(done, total int64) { time.Sleep(300 * time.Millisecond) }, copyLimits{250 * time.Millisecond, 128 << 20, 0})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("snapshot exceeded time limit: %v", err)
	}
}
