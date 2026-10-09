package dbwork

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// The write-ahead log is the one resource in this design with no natural ceiling
// and no observability. `wal_autocheckpoint=1000` (≈4 MiB) only runs a PASSIVE
// checkpoint, which copies frames through the oldest reader's end mark but
// cannot reset the log while that reader still needs it. A large library scan
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
	Busy               int                 `json:"busy"`
	LogFrames          int                 `json:"logFrames"`
	CheckpointedFrames int                 `json:"checkpointedFrames"`
	BacklogFrames      int                 `json:"backlogFrames"`
	FileBytes          int64               `json:"walFileBytes"`
	PeakFileBytes      int64               `json:"walPeakFileBytes"`
	LastError          string              `json:"lastError,omitempty"`
	Readers            ReaderLifetimeStats `json:"readers"`
	Checkpoints        CheckpointStats     `json:"checkpoints"`
}

// walPeak remembers the largest `-wal` ever observed in this process's life. A
// current size of zero says nothing about whether the log spiked to a gigabyte
// an hour ago, and the soak tier's pass criterion is about the peak.
var walPeak atomic.Int64

// WAL reports the write-ahead log's state, running the same PASSIVE checkpoint
// the autocheckpoint would run. PASSIVE does not invoke the busy handler and
// normally reports busy=0 even when readers prevent a complete checkpoint.
// Compare LogFrames with CheckpointedFrames to observe that incomplete work;
// copying available pages can still take time on slow storage.
func WAL(ctx context.Context, db *sql.DB) WALStats {
	out := WALStats{}
	if db == nil {
		return out
	}
	if err := db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`).Scan(&out.Busy, &out.LogFrames, &out.CheckpointedFrames); err != nil {
		out.LastError = err.Error()
	}
	out.FileBytes = walFileBytes(ctx, db)
	out.BacklogFrames = max(0, out.LogFrames-out.CheckpointedFrames)
	for {
		peak := walPeak.Load()
		if out.FileBytes <= peak || walPeak.CompareAndSwap(peak, out.FileBytes) {
			break
		}
	}
	out.PeakFileBytes = walPeak.Load()
	out.Readers = ReaderLifetimes()
	out.Checkpoints = Checkpoints()
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
// checkpoint cannot reset the log while a reader needs older frames. Overlapping
// browse snapshots can retain that boundary continuously. The log grows onto
// the same volume as the generated
// media until the disk fills and SQLite starts answering SQLITE_FULL.
//
// So there is a recurring checkpoint, in the maintenance class, and it escalates:
// PASSIVE while the log is small, TRUNCATE once the log is past a threshold AND
// nobody is writing or waiting to write, or at the hard threshold. A short
// SQLite busy budget bounds waiting for readers; the shared gate preserves
// writer priority. Page copying and filesystem sync still depend on storage.

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
	// CheckpointDeadline bounds connection/gate waiting and supplies query
	// cancellation. It is not a guaranteed wall-clock cap for SQLite's WAL
	// copying, fsync or busy callback; the lock wait has a separate short budget.
	CheckpointDeadline = 20 * time.Second
	// SQLite's WAL busy callback does not observe context interruption. Bound
	// its wait separately so a pinned reader cannot monopolize the write gate.
	CheckpointBusyTimeout     = 50 * time.Millisecond
	checkpointCleanupDeadline = time.Second
)

// CheckpointResult says what one maintenance checkpoint did.
type CheckpointResult struct {
	Mode                     string              `json:"mode"`
	Busy                     int                 `json:"busy"`
	LogFrames                int                 `json:"logFrames"`
	Checkpoint               int                 `json:"checkpointedFrames"`
	BeforeLogFrames          int                 `json:"beforeLogFrames"`
	BeforeCheckpointedFrames int                 `json:"beforeCheckpointedFrames"`
	DurationMs               int64               `json:"durationMs"`
	DrainMs                  int64               `json:"drainMs"`
	Outcome                  string              `json:"outcome"`
	BackoffMs                int64               `json:"backoffMs"`
	Readers                  ReaderLifetimeStats `json:"readers"`
	Err                      error               `json:"-"`
}

// CheckpointStats retains fixed counts and the last outcome; SQL, arguments,
// database paths and error messages never enter this diagnostic state.
type CheckpointStats struct {
	Attempts         uint64           `json:"attempts"`
	Truncates        uint64           `json:"truncates"`
	Busy             uint64           `json:"busy"`
	Errors           uint64           `json:"errors"`
	PressureTimeouts uint64           `json:"pressureTimeouts"`
	MaxDurationMs    int64            `json:"maxDurationMs"`
	Last             CheckpointResult `json:"last"`
}

var checkpointObservation struct {
	sync.Mutex
	stats CheckpointStats
}

func Checkpoints() CheckpointStats {
	checkpointObservation.Lock()
	defer checkpointObservation.Unlock()
	return checkpointObservation.stats
}

func observeCheckpoint(out CheckpointResult) {
	checkpointObservation.Lock()
	defer checkpointObservation.Unlock()
	s := &checkpointObservation.stats
	s.Attempts++
	if out.Mode == "truncate" {
		s.Truncates++
	}
	if out.Busy != 0 {
		s.Busy++
	}
	if out.Err != nil {
		s.Errors++
	}
	if out.Outcome == "reader-timeout" {
		s.PressureTimeouts++
	}
	s.MaxDurationMs = max(s.MaxDurationMs, out.DurationMs)
	out.Err = nil
	s.Last = out
}

// Checkpoint runs one maintenance checkpoint, escalating to TRUNCATE when the
// log has grown past the threshold and the write gate is idle.
func Checkpoint(ctx context.Context, db *sql.DB) (out CheckpointResult) {
	out = CheckpointResult{Mode: "passive"}
	if db == nil {
		return out
	}
	start := time.Now()
	defer func() {
		out.DurationMs = time.Since(start).Milliseconds()
		if out.Err != nil {
			out.Outcome = "error"
		} else if out.Outcome == "" {
			if out.Busy != 0 {
				out.Outcome = "busy"
			} else if out.LogFrames != out.Checkpoint {
				out.Outcome = "partial"
			} else if out.Mode == "truncate" {
				out.Outcome = "truncated"
			} else {
				out.Outcome = "copied"
			}
		}
		observeCheckpoint(out)
	}()
	ctx, cancel := context.WithTimeout(ctx, CheckpointDeadline)
	defer cancel()
	ctx = WithClass(ctx, ClassMaintenance)
	ctx = context.WithValue(ctx, checkpointReaderBypassKey{}, true)
	// One dedicated connection, because a PRAGMA through the pool lands on
	// whichever connection it is handed; the same rule the statistics pass learned.
	conn, err := ReadHandle(ctx, db).Conn(ctx)
	if err != nil {
		out.Err = err
		return out
	}
	defer conn.Close()
	var priorBusy int
	if err = conn.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&priorBusy); err != nil {
		out.Err = err
		return out
	}
	// Register restoration before changing the pragma: even a cancellation
	// racing a successful SET must not return altered policy to the pool.
	defer func() {
		if cleanupErr := restoreCheckpointBusyTimeout(conn, priorBusy); cleanupErr != nil {
			out.Err = errors.Join(out.Err, cleanupErr)
		}
	}()
	if _, err = conn.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout=%d", CheckpointBusyTimeout.Milliseconds())); err != nil {
		out.Err = err
		return out
	}
	if err = conn.QueryRowContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`).Scan(&out.Busy, &out.LogFrames, &out.Checkpoint); err != nil {
		out.Err = err
		return out
	}
	out.BeforeLogFrames = out.LogFrames
	out.BeforeCheckpointedFrames = out.Checkpoint
	scope := checkpointReaderScope(conn)
	if scope != nil {
		out.Readers = scope.stats()
	} else {
		out.Readers = ReaderLifetimes()
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
	gateCtx := ctx
	if out.LogFrames >= CheckpointHardFrames && scope != nil {
		reopen, drained, backoff := scope.pause()
		if reopen == nil {
			out.Outcome, out.BackoffMs = "reader-backoff", backoff.Milliseconds()
			return out
		}
		// Register immediately: cancellation, error and panic must all resume
		// normal readers. Failed drains never acquire the writer gate.
		defer func() {
			reopen(out.Err == nil && out.Mode == "truncate" && out.Busy == 0)
			out.BackoffMs = scope.pressureBackoff().Milliseconds()
		}()
		var drainCancel context.CancelFunc
		gateCtx, drainCancel = context.WithTimeout(ctx, CheckpointReaderDrainDeadline)
		defer drainCancel()
		drainStart := time.Now()
		err = waitReaderDrain(gateCtx, drained)
		out.DrainMs = time.Since(drainStart).Milliseconds()
		if err != nil {
			out.Outcome = "reader-timeout"
			if ctx.Err() != nil {
				out.Err = ctx.Err()
			}
			return out
		}
	}
	release, err := WriteGate().Acquire(gateCtx, ClassMaintenance)
	if err != nil {
		out.Err = err
		return out
	}
	defer release()
	out.Mode = "truncate"
	if err = conn.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&out.Busy, &out.LogFrames, &out.Checkpoint); err != nil {
		out.Err = err
	}
	return out
}

// Cleanup must survive the request context being cancelled. A connection whose
// original policy cannot be restored is discarded rather than silently loaned
// to subsequent background work with a different contention budget.
func restoreCheckpointBusyTimeout(conn *sql.Conn, prior int) error {
	ctx, cancel := context.WithTimeout(context.Background(), checkpointCleanupDeadline)
	defer cancel()
	_, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout=%d", prior))
	if err == nil {
		return nil
	}
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	return fmt.Errorf("dbwork: restore checkpoint busy timeout: %w", err)
}
