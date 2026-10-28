package backup

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/diskspace"
	sqlite "portico.local/server/internal/thirdparty/sqlite"
)

// ErrBackupPressure ends a backup attempt that could not finish without
// holding up the server: it made no progress for a minute, used up its copy
// allowance, met the database busy too often, or (automatic backups only)
// could not keep each step short. The last good backup stays; the schedule
// tries again later and the owner is warned.
var ErrBackupPressure = errors.New("backup could not finish without holding up the server")

// The online backup copies the live database from the writer connection, in
// short steps, each taken as one writer-gated ticket.
//
// Why the writer: SQLite keeps a backup current with every change made
// through its own source connection, and restarts it after a change made
// through any other. Every write in this server goes through the one writer
// connection, so writes made while a backup runs update the copy instead of
// restarting it, and a library that is always being written to still
// finishes. A reader-side copy would restart on every commit, or pin one
// snapshot and hold the WAL back for the whole copy (what this replaced).
//
// Why steps: between steps the backup holds nothing. No read transaction
// stays open on the source, so checkpoints proceed, and the writer is free.
// One step copies at most 256 KiB; a slow step drops the size to 64 KiB.
// Never the whole file in one call.
//
// Why tickets: a step must not run while another writer holds the
// connection, and the gate is what says whose turn it is. A scheduled backup
// takes maintenance tickets, so it only runs when no user write is waiting;
// under constant user writes it stops making progress and gives up after a
// minute, to retry later (ErrBackupPressure). A backup the owner starts takes
// interactive tickets and shares turns with users: the brief interruption the
// owner is warned about.
type copyLimits struct {
	// class is the gate class of every step's ticket.
	class dbwork.Class
	// deadline bounds one attempt, waiting included.
	deadline time.Duration
	// noProgress ends an attempt whose copy has not advanced for this long.
	noProgress time.Duration
	// busyLimit ends an attempt after this many consecutive BUSY/LOCKED steps.
	busyLimit int
	// stepBytes and slowStepBytes are the step sizes; a step slower than
	// slowStep switches to the smaller size for the rest of the attempt.
	stepBytes, slowStepBytes int64
	slowStep                 time.Duration
	// quiet, when set, ends the attempt if a step at the small size still
	// holds the writer longer than this. Automatic backups set it: they may
	// never keep the server waiting more than a couple of seconds.
	quiet time.Duration
	// reportEvery spaces progress reports.
	reportEvery time.Duration
}

const (
	backupStepBytes     = 256 << 10
	backupSlowStepBytes = 64 << 10
	backupBusyBackoff   = 20 * time.Millisecond
	backupDiskEvery     = 250 * time.Millisecond
	backupSourceReserve = 128 << 20
)

func scheduledCopyLimits() copyLimits {
	return copyLimits{
		class: dbwork.ClassMaintenance, deadline: 30 * time.Minute, noProgress: time.Minute, busyLimit: 8,
		stepBytes: backupStepBytes, slowStepBytes: backupSlowStepBytes, slowStep: 200 * time.Millisecond,
		quiet: 2 * time.Second, reportEvery: 200 * time.Millisecond,
	}
}

func manualCopyLimits() copyLimits {
	limits := scheduledCopyLimits()
	limits.class = dbwork.ClassInteractive
	limits.quiet = 0
	return limits
}

func copyLimitsFor(kind Kind) copyLimits {
	if kind == KindManual || kind == KindPreRestore {
		return manualCopyLimits()
	}
	return scheduledCopyLimits()
}

// copyAllowance is one attempt's copy work: SQLite restarts a backup after a
// change it did not make itself (a log reset, an outside writer), and every
// requested step counts against this, even when a restart moves the reported
// progress backwards.
func copyAllowance(initial int64) int64 {
	return max(4*initial, initial+1<<30)
}

// copyDatabase writes a consistent copy of the live database to dest. The
// destination must not exist. progress reports bytes copied of the current
// total as the copy advances; it runs between steps, holding nothing.
func copyDatabase(ctx context.Context, source *sql.DB, dest string, progress func(done, total int64), limits copyLimits) error {
	return copyDatabaseGuarded(ctx, source, dest, progress, limits, nil)
}

// The optional guard lets an embedding abort a copy at a step boundary. The
// built-in time, progress, work and disk limits apply regardless.
func copyDatabaseGuarded(ctx context.Context, source *sql.DB, dest string, progress func(done, total int64), limits copyLimits, guard func(done, total int64) error) error {
	if source == nil || dest == "" {
		return errors.New("backup: no database to copy")
	}
	ctx = dbwork.WithClass(ctx, limits.class)
	ctx, cancel := context.WithTimeout(ctx, limits.deadline)
	defer cancel()
	sourcePath := dbwork.DatabaseFile(ctx, source)
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	run := &copyRun{source: source, dest: dest, limits: limits}
	defer run.abandon()
	if err = run.copy(ctx, sourcePath, progress, guard); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil || info.Size() < 512 || ctx.Err() != nil {
		return errors.New("backup: database copy is incomplete")
	}
	return file.Sync()
}

// copyRun is one attempt's backup object and the writer connection it reads
// from. The object belongs to that physical connection: every step, and the
// finish, run on it while a ticket is held.
type copyRun struct {
	source *sql.DB
	dest   string
	limits copyLimits
	backup *sqlite.Backup
	owner  any
	// pageSize and the step accounting are read on the first step.
	pageSize int64
	charged  int64
	allowed  int64
}

var errWriterReplaced = errors.New("backup: the writer connection was replaced during the copy")

func (r *copyRun) copy(ctx context.Context, sourcePath string, progress func(done, total int64), guard func(done, total int64) error) error {
	stepBytes := r.limits.stepBytes
	lastProgress := time.Now()
	lastRemaining := -1
	busy := 0
	var lastReport, lastDisk time.Time
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Since(lastProgress) >= r.limits.noProgress {
			return ErrBackupPressure
		}
		if lastDisk.IsZero() || time.Since(lastDisk) >= backupDiskEvery {
			lastDisk = time.Now()
			if err := r.checkDisk(sourcePath); err != nil {
				return err
			}
		}
		// The wait for a ticket is part of the no-progress budget: a backup
		// that never gets a turn ends there, rather than at the deadline.
		wait, cancelWait := context.WithDeadline(ctx, lastProgress.Add(r.limits.noProgress))
		step, err := r.step(wait, stepBytes)
		cancelWait()
		if err != nil {
			switch {
			case ctx.Err() != nil:
				return ctx.Err()
			case errors.Is(err, context.DeadlineExceeded):
				return ErrBackupPressure
			case errors.Is(err, dbwork.ErrWriterCapacity):
				// A full writer queue refuses the newest least important
				// ticket; this one waits a moment and asks again.
				if err = sleepContext(ctx, backupBusyBackoff); err != nil {
					return err
				}
				continue
			}
			return err
		}
		if step.busy {
			busy++
			if busy >= r.limits.busyLimit {
				return ErrBackupPressure
			}
			if err = sleepContext(ctx, backupBusyBackoff); err != nil {
				return err
			}
			continue
		}
		busy = 0
		if r.charged > r.allowed {
			return ErrBackupPressure
		}
		// Forward progress is fewer pages left than after the last step. A
		// restart moves it backwards; the copy allowance bounds those.
		if step.remaining < lastRemaining || lastRemaining < 0 || !step.more {
			lastProgress = time.Now()
		}
		lastRemaining = step.remaining
		if step.more && step.took > r.limits.slowStep && stepBytes > r.limits.slowStepBytes {
			stepBytes = r.limits.slowStepBytes
		} else if step.more && r.limits.quiet > 0 && step.took > r.limits.quiet {
			return ErrBackupPressure
		}
		copied := int64(step.pages-step.remaining) * r.pageSize
		total := int64(step.pages) * r.pageSize
		if guard != nil {
			if err = guard(copied, total); err != nil {
				return err
			}
		}
		if progress != nil && (!step.more || time.Since(lastReport) >= r.limits.reportEvery) {
			progress(copied, total)
			lastReport = time.Now()
		}
		if !step.more {
			return nil
		}
	}
}

type stepResult struct {
	more, busy       bool
	pages, remaining int
	took             time.Duration
}

// step takes one writer ticket and copies at most stepBytes. The first step
// starts the backup; the step that reaches the end finishes it.
func (r *copyRun) step(ctx context.Context, stepBytes int64) (out stepResult, err error) {
	ctx, conn, release, err := dbwork.ReserveWriterConn(ctx, r.source, r.limits.class)
	if err != nil {
		return out, err
	}
	defer release()
	if r.backup == nil {
		if err = conn.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&r.pageSize); err != nil {
			return out, err
		}
		if r.pageSize < 512 || r.pageSize > 65536 || r.pageSize&(r.pageSize-1) != 0 {
			return out, errors.New("backup: unexpected database page size")
		}
		var pages int64
		if err = conn.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages); err != nil {
			return out, err
		}
		r.allowed = copyAllowance(pages * r.pageSize)
	}
	err = conn.Raw(func(raw any) error {
		driver := dbwork.RawConn(raw)
		if r.backup == nil {
			// The handle is instrumented; the backup API lives on the
			// connection underneath it.
			starter, ok := driver.(interface {
				NewBackup(string) (*sqlite.Backup, error)
			})
			if !ok {
				return errors.New("backup: online backup unavailable")
			}
			backup, err := starter.NewBackup(destinationURI(r.dest))
			if err != nil {
				return err
			}
			r.backup, r.owner = backup, driver
		} else if driver != r.owner {
			// The pool replaced the writer connection, which finished this
			// backup as it closed (thirdparty/sqlite/backup.go).
			r.backup, r.owner = nil, nil
			return errWriterReplaced
		}
		pages := max(int32(stepBytes/r.pageSize), 1)
		r.charged += int64(pages) * r.pageSize
		start := time.Now()
		more, stepErr := r.backup.Step(pages)
		out.took = time.Since(start)
		out.more = more
		out.pages, out.remaining = r.backup.PageCount(), r.backup.Remaining()
		if stepErr != nil {
			var sqliteErr *sqlite.Error
			if errors.As(stepErr, &sqliteErr) && (sqliteErr.Code()&255 == 5 || sqliteErr.Code()&255 == 6) {
				out.busy = true
				return nil
			}
			return stepErr
		}
		if !more {
			// DONE, then a successful finish, is the only completion.
			backup := r.backup
			r.backup, r.owner = nil, nil
			return backup.Finish()
		}
		return nil
	})
	return out, err
}

// abandon finishes a backup an attempt left running, on its own connection
// and under a ticket. A cancelled attempt still needs this, so it uses its own
// short budget; if even that fails the backup stays attached to the writer
// connection, which finishes it whenever it closes.
func (r *copyRun) abandon() {
	if r.backup == nil {
		return
	}
	ctx, cancel := context.WithTimeout(dbwork.WithClass(context.Background(), r.limits.class), 30*time.Second)
	defer cancel()
	_, conn, release, err := dbwork.ReserveWriterConn(ctx, r.source, r.limits.class)
	if err == nil {
		err = conn.Raw(func(raw any) error {
			if dbwork.RawConn(raw) != r.owner {
				return errWriterReplaced
			}
			return r.backup.Finish()
		})
		release()
	}
	if err != nil && !errors.Is(err, errWriterReplaced) && !errors.Is(err, sqlite.ErrBackupClosed) {
		log.Printf("warn: Backup could not release its unfinished copy: %v", err)
	}
	r.backup, r.owner = nil, nil
}

func (r *copyRun) checkDisk(sourcePath string) error {
	if sourcePath != "" {
		free, err := diskspace.Free(filepath.Dir(sourcePath))
		if err != nil {
			return err
		}
		if free < backupSourceReserve {
			return ErrInsufficientDisk
		}
	}
	info, err := os.Stat(r.dest)
	if err != nil {
		return err
	}
	free, err := diskspace.Free(filepath.Dir(r.dest))
	if err != nil {
		return err
	}
	remaining := int64(0)
	if sourcePath != "" {
		if source, err := os.Stat(sourcePath); err == nil {
			remaining = max(source.Size()-info.Size(), 0)
		}
	}
	if remaining > free || free-remaining < backupSourceReserve {
		return ErrInsufficientDisk
	}
	return nil
}

// destinationURI opens the copy without journalling or syncing: a failed
// attempt discards the file, and the finished copy is synced once, after the
// last step, outside the writer ticket. Otherwise the step that completes the
// backup would hold the writer through an fsync of the whole file.
func destinationURI(dest string) string {
	values := url.Values{}
	values.Add("_pragma", "journal_mode(OFF)")
	values.Add("_pragma", "synchronous(OFF)")
	values.Add("_pragma", "locking_mode(EXCLUSIVE)")
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(dest), RawQuery: values.Encode()}).String()
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// candidateDatabase opens the backup's own copy privately (dbwork.Candidate):
// checking and scrubbing it is inert artifact work, never a live write.
func candidateDatabase(partial string) (*dbwork.Candidate, error) {
	return dbwork.OpenCandidate(partial, DatabaseName)
}

// quickCheck runs PRAGMA quick_check over a candidate: a torn copy must never
// become a listed backup.
func quickCheck(ctx context.Context, candidate *dbwork.Candidate) error {
	lines, err := candidate.Check(ctx, true, 1)
	if err != nil {
		return err
	}
	if !dbwork.Healthy(lines) {
		return errors.New("backup: database copy failed quick_check")
	}
	return nil
}

func walBytes(path string) (int64, error) {
	if path == "" {
		return 0, nil
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// finalizeDatabase makes the scrubbed backup independent of any WAL before its
// checksum is recorded. A busy/incomplete checkpoint is a failed backup, never
// permission to publish a hash covering only part of the logical database.
// It closes the candidate.
func finalizeDatabase(ctx context.Context, partial string, candidate *dbwork.Candidate) (FileEntry, error) {
	empty := FileEntry{}
	err := candidate.Fold(ctx)
	if closeErr := candidate.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return empty, err
	}
	path := filepath.Join(partial, filepath.FromSlash(DatabaseName))
	if size, err := walBytes(path + "-wal"); err != nil {
		return empty, err
	} else if size != 0 {
		return empty, errors.New("backup: finalized database still has WAL frames")
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return empty, err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return empty, err
	}
	if closeErr != nil {
		return empty, closeErr
	}
	return hashFile(ctx, partial, DatabaseName)
}
