package backup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"portico.local/server/internal/dbwork"
)

func TestBackupPressureReleasesSnapshot(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "state.sqlite")
	db, err := dbwork.OpenHandle(source, dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE sample(id INTEGER PRIMARY KEY,data BLOB);INSERT INTO sample(data) VALUES(zeroblob(1048576))`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err = copyDatabaseGuarded(context.Background(), db, filepath.Join(dir, "copy.sqlite"), nil, func(done, total int64) error {
		calls++
		if calls == 2 {
			return ErrSnapshotPressure
		}
		return nil
	})
	if !errors.Is(err, ErrSnapshotPressure) || calls != 2 {
		t.Fatal(err, calls)
	}
	if _, err = db.Exec(`INSERT INTO sample(data) VALUES(zeroblob(1000))`); err != nil {
		t.Fatal("backup left source blocked", err)
	}
	result := dbwork.Checkpoint(context.Background(), db)
	if result.Err != nil {
		t.Fatal("backup left its snapshot pinned", result)
	}
	var log, checkpointed, busy int
	if err = db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &log, &checkpointed); err != nil || busy != 0 {
		t.Fatal(busy, log, checkpointed, err)
	}
}
