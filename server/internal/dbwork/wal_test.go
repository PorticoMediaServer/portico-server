package dbwork

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWALReportsFramesAndFileSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := OpenHandle(path, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err = db.ExecContext(ctx, `CREATE TABLE sample(id INTEGER PRIMARY KEY, body TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	// The driver reports the path with symlinks resolved, which is what a stat of
	// the -wal beside it needs; compare the resolved forms.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if file := DatabaseFile(ctx, db); file != resolved {
		t.Fatalf("the handle reported its file as %q, not %q", file, resolved)
	}
	// Write enough to put frames in the log without reaching the autocheckpoint.
	for index := 0; index < 64; index++ {
		if _, err = db.ExecContext(ctx, `INSERT INTO sample(body) VALUES(?)`, "a reasonably sized row of text"); err != nil {
			t.Fatal(err)
		}
	}
	stats := WAL(ctx, db)
	if stats.LastError != "" {
		t.Fatalf("the WAL probe failed: %s", stats.LastError)
	}
	if stats.LogFrames <= 0 {
		t.Fatalf("64 committed rows produced %d log frames", stats.LogFrames)
	}
	if stats.CheckpointedFrames <= 0 {
		t.Fatal("a PASSIVE checkpoint with no competing reader checkpointed nothing")
	}
	if stats.PeakFileBytes < stats.FileBytes {
		t.Fatalf("the peak (%d) is below the current size (%d)", stats.PeakFileBytes, stats.FileBytes)
	}
}

func TestWALOnANilHandleIsEmptyRatherThanAPanic(t *testing.T) {
	if stats := WAL(context.Background(), nil); stats.LogFrames != 0 || stats.FileBytes != 0 {
		t.Fatalf("a nil handle reported %#v", stats)
	}
}
