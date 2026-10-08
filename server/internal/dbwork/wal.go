package dbwork

import (
	"context"
	"database/sql"
	"os"
	"sync/atomic"
	"time"
)

// The write-ahead log is the one resource in this design with no natural ceiling
// and no observability. `wal_autocheckpoint=1000` (≈4 MiB) only runs a PASSIVE
// checkpoint, and a PASSIVE checkpoint silently does nothing while any reader
// holds a snapshot older than the WAL head. A scan of a two-million-item library
// writing continuously while two hundred viewers hold browse snapshots is
// exactly that shape, and the failure mode — a `-wal` file growing into the tens
// of gigabytes on the same volume as the generated media — is invisible in every
// diagnostic the server publishes.
//
// Measuring it is read-only and costs nothing, so it comes first.

// WALStats is the write-ahead log's observable state. Busy, LogFrames and
// CheckpointedFrames are the three values `PRAGMA wal_checkpoint` itself returns;
// FileBytes is the `-wal` file on disk, which is the number that fills a volume.
type WALStats struct {
	Busy               int    `json:"busy"`
	LogFrames          int    `json:"logFrames"`
	CheckpointedFrames int    `json:"checkpointedFrames"`
	FileBytes          int64  `json:"walFileBytes"`
	PeakFileBytes      int64  `json:"walPeakFileBytes"`
	LastError          string `json:"lastError,omitempty"`
}

// walPeak remembers the largest `-wal` ever observed in this process's life. A
// current size of zero says nothing about whether the log spiked to a gigabyte
// an hour ago, and the soak tier's pass criterion is about the peak.
var walPeak atomic.Int64

// WAL reports the write-ahead log's state, running the same PASSIVE checkpoint
// the autocheckpoint would run. PASSIVE never blocks: if a reader holds an older
// snapshot it returns busy=1 and checkpoints what it can, which is precisely the
// signal worth publishing.
func WAL(ctx context.Context, db *sql.DB) WALStats {
	out := WALStats{}
	if db == nil {
		return out
	}
	if err := db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`).Scan(&out.Busy, &out.LogFrames, &out.CheckpointedFrames); err != nil {
		out.LastError = err.Error()
	}
	out.FileBytes = walFileBytes(ctx, db)
	for {
		peak := walPeak.Load()
		if out.FileBytes <= peak || walPeak.CompareAndSwap(peak, out.FileBytes) {
			break
		}
	}
	out.PeakFileBytes = walPeak.Load()
	return out
}

// walFileBytes sizes the `-wal` beside the main database. The path comes from
// the database itself rather than from a value threaded through forty
// constructors: `PRAGMA database_list` is the handle's own answer to "which file
// am I?" and stays correct across a recycled handle.
func walFileBytes(ctx context.Context, db *sql.DB) int64 {
	path := DatabaseFile(ctx, db)
	if path == "" {
		return 0
	}
	info, err := os.Stat(path + "-wal")
	if err != nil {
		return 0
	}
	return info.Size()
}

// DatabaseFile returns the main database's path, or "" for a memory database.
func DatabaseFile(ctx context.Context, db *sql.DB) string {
	if db == nil {
		return ""
	}
	rows, err := db.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name, file string
		if err = rows.Scan(&seq, &name, &file); err != nil {
			return ""
		}
		if name == "main" {
			return file
		}
	}
	return ""
}

// One checkpoint ran in the whole life of the process: a PASSIVE pass at
// startup. `wal_autocheckpoint=1000` covers roughly four mebibytes, and a PASSIVE
// checkpoint does nothing at all while any reader holds a snapshot older than the
// log head — which, during a scan of a two-million-item library with viewers
// browsing, is continuously. The log grows onto the same volume as the generated
// media until the disk fills and SQLite starts answering SQLITE_FULL.
//
// So there is a recurring checkpoint, in the maintenance class, and it escalates:
// PASSIVE while the log is small, TRUNCATE once the log is past a threshold AND
// nobody is writing or waiting to write. TRUNCATE blocks until readers drain and
// can stall a writer, which is exactly why it is gated on the same quiet-period
// signals the deferred maintenance already consults rather than run on a timer.

const (
	// CheckpointTruncateFrames is the log length past which a PASSIVE pass is
	// clearly not keeping up and a TRUNCATE is worth its cost. Roughly 64 MiB at
	// the 4 KiB page size this policy uses.
	CheckpointTruncateFrames = 16000
	// CheckpointHardFrames is the length past which the quiet period stops being
	// a reason to wait. Roughly 192 MiB. TRUNCATE stalls writers until readers
	// drain, which is why it is normally gated on the server being quiet — but a
	// server that is never quiet is exactly the one whose log grows until the
	// volume fills, and a stall is recoverable where a full disk is not.
	CheckpointHardFrames = 48000
	// CheckpointDeadline bounds one checkpoint attempt. A checkpoint that cannot
	// finish in this long is being held back by a reader, and the next pass will
	// find it again.
	CheckpointDeadline = 20 * time.Second
)

// CheckpointResult says what one maintenance checkpoint did.
type CheckpointResult struct {
	Mode       string `json:"mode"`
	Busy       int    `json:"busy"`
	LogFrames  int    `json:"logFrames"`
	Checkpoint int    `json:"checkpointedFrames"`
	Err        error  `json:"-"`
}

// Checkpoint runs one maintenance checkpoint, escalating to TRUNCATE when the
// log has grown past the threshold and the write gate is idle.
func Checkpoint(ctx context.Context, db *sql.DB) CheckpointResult {
	out := CheckpointResult{Mode: "passive"}
	if db == nil {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, CheckpointDeadline)
	defer cancel()
	// One dedicated connection, because a PRAGMA through the pool lands on
	// whichever connection it is handed; the same rule the statistics pass learned.
	conn, err := db.Conn(ctx)
	if err != nil {
		out.Err = err
		return out
	}
	defer conn.Close()
	if err = conn.QueryRowContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`).Scan(&out.Busy, &out.LogFrames, &out.Checkpoint); err != nil {
		out.Err = err
		return out
	}
	if out.LogFrames < CheckpointTruncateFrames {
		return out
	}
	// TRUNCATE blocks until every reader has drained, so it only runs when
	// nothing is writing or waiting to write and no foreground request is in
	// flight — unless the log has grown past the point where waiting for a quiet
	// moment that may never come is the worse risk.
	if out.LogFrames < CheckpointHardFrames && (WriteGate().ActiveOrWaiting() || ForegroundWorkActive()) {
		return out
	}
	release, err := WriteGate().Acquire(ctx, ClassMaintenance)
	if err != nil {
		return out
	}
	defer release()
	out.Mode = "truncate"
	if err = conn.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&out.Busy, &out.LogFrames, &out.Checkpoint); err != nil {
		out.Err = err
	}
	return out
}
