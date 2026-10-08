package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"portico.local/apikit/idempotency"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

// Two pieces of housekeeping that were missing entirely, and one that is
// deliberately opt-in.
//
// The write-ahead log was checkpointed exactly once in the process's life — a
// PASSIVE pass at startup. `wal_autocheckpoint` covers about four mebibytes and
// a PASSIVE checkpoint does nothing at all while any reader holds a snapshot
// older than the log head, which during a scan with viewers browsing is
// continuously. The log grows onto the same volume as the generated media until
// the disk fills. It is now checkpointed on a timer, escalating to TRUNCATE only
// when the log is long AND nothing is writing or waiting to write.
//
// And nothing ever asked the database whether it was structurally sound.
// quick_check and foreign_key_check are both the size of the database, so they
// are opt-in and never near the startup path.

const (
	// checkpointInterval is how often the log is folded back into the database. A
	// PASSIVE pass on a quiet database is a no-op, so this is cheap enough to be
	// frequent, and frequent is what keeps the log from growing during a long
	// stretch of sustained writes.
	checkpointInterval = 30 * time.Second
	// integrityIntervalDefault is how often the opt-in structural check runs.
	integrityIntervalDefault = 24 * time.Hour
)

// integritySchedule reads the owner's choice. Off unless asked for, because on a
// multi-gigabyte library this is minutes of I/O and the owner should be the one
// deciding when to spend it.
func integritySchedule() (time.Duration, bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PORTICO_INTEGRITY_CHECK"))) {
	case "", "off", "0", "false":
		return 0, false
	case "startup":
		return 0, true
	case "hourly":
		return time.Hour, true
	case "daily", "1", "true", "on":
		return integrityIntervalDefault, true
	case "weekly":
		return 7 * integrityIntervalDefault, true
	}
	if parsed, err := time.ParseDuration(strings.TrimSpace(os.Getenv("PORTICO_INTEGRITY_CHECK"))); err == nil && parsed > 0 {
		return parsed, true
	}
	return 0, false
}

// runDatabaseHousekeeping is the recurring half of the database's care: the
// checkpoint every start needs, and the structural check an owner asked for.
func runDatabaseHousekeeping(ctx context.Context, db *sql.DB) {
	if db == nil {
		return
	}
	// How many schema objects this state directory holds that this build does not define —
	// a database written by a different or newer Portico. It is one sqlite_schema scan against
	// a set built once, so the cost does not grow with what it finds, and it happens once.
	// Objects a subsystem installs for itself on first use count as this build's own.
	// A directory this process just created cannot hold a foreign database, so its
	// warning is downgraded (the count is still published for diagnostics).
	if unknown, err := persistence.UnknownSchemaObjects(ctx, db); err == nil {
		dbwork.PublishForeignSchemaObjects(len(unknown))
		if warnForeignSchema(unknown) {
			log.Printf("This state directory holds %d schema objects this build does not define; they are left untouched. First: %s", len(unknown), strings.Join(unknown[:min(3, len(unknown))], ", "))
		}
	}
	interval, integrity := integritySchedule()
	lastIntegrity := time.Time{}
	lastReceiptSweep := time.Now()
	lastUnknownSweep := time.Now()
	receipts := idempotency.Store{SweepBegin: func(ctx context.Context) (idempotency.Transaction, error) {
		return dbwork.Begin(ctx, db, dbwork.ClassMaintenance)
	}}
	if integrity && interval == 0 {
		// "startup" means once, after the handler is live and things are quiet.
		if dbwork.Yield(ctx) {
			reportIntegrity(ctx, db)
		}
		integrity = false
	}
	ticker := time.NewTicker(checkpointInterval)
	defer ticker.Stop()
	// A checkpoint that found every frame already copied and nothing has
	// committed since has nothing to do: an idle server skips it rather than
	// asking the database the same question twice a minute.
	settled, settledAt := false, uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !dbwork.Yield(ctx) {
			return
		}
		if commits := dbwork.ChangingCommits(); !settled || commits != settledAt {
			result := dbwork.Checkpoint(ctx, db)
			if result.Mode == "truncate" && result.Err == nil {
				log.Printf("Write-ahead log truncated from %d frames", result.LogFrames)
			}
			settled = result.Err == nil && result.Busy == 0 && result.Checkpoint == result.LogFrames
			settledAt = commits
		}
		if time.Since(lastReceiptSweep) >= 15*time.Minute {
			// Check the indexed expiry on a read connection first. Backlog is
			// drained in short gated transactions rather than one long writer.
			backlog, err := sweepDueReceipts(ctx, db, receipts)
			if err != nil {
				log.Printf("Idempotency receipt pruning failed: %v", err)
				lastReceiptSweep = time.Now()
			} else if !backlog {
				lastReceiptSweep = time.Now()
			}
		}
		if time.Since(lastUnknownSweep) >= 15*time.Minute {
			// Guessed names are stored only as keyed digests. Retain their
			// decaying attempt debt for one day, then prune a bounded batch at
			// maintenance priority even while foreground work continues.
			result, err := dbwork.ExecWrite(ctx, db, dbwork.ClassMaintenance, `DELETE FROM identity_unknown_attempts WHERE rowid IN (SELECT rowid FROM identity_unknown_attempts WHERE last_failure<? ORDER BY last_failure LIMIT 100)`, time.Now().Add(-24*time.Hour).Unix())
			if err != nil {
				log.Printf("Unknown sign-in attempt pruning failed: %v", err)
				lastUnknownSweep = time.Now()
			} else if count, _ := result.RowsAffected(); count < 100 {
				lastUnknownSweep = time.Now()
			}
		}
		if !integrity || time.Since(lastIntegrity) < interval {
			continue
		}
		// The structural check runs only when nothing is waiting on the database,
		// because it is the size of the database.
		if dbwork.WriteGate().ActiveOrWaiting() || dbwork.ForegroundWorkActive() {
			continue
		}
		lastIntegrity = time.Now()
		reportIntegrity(ctx, db)
	}
}

// warnForeignSchema reports whether the foreign-schema warning is logged.
// A directory this process just created cannot hold a foreign database, so its
// warning is downgraded (the count is still published for diagnostics).
func warnForeignSchema(unknown []string) bool {
	return len(unknown) > 0 && !stateDirFreshThisProcess
}

// sweepDueReceipts never takes the writer gate for an empty expiry index.
// Under a backlog it re-enters the gate for each small batch, giving queued
// foreground requests a chance between batches.
func sweepDueReceipts(ctx context.Context, db *sql.DB, receipts idempotency.Store) (bool, error) {
	var due bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM api_idempotency WHERE expires_at<=? LIMIT 1)`, time.Now().Unix()).Scan(&due); err != nil {
		return false, err
	}
	if !due {
		return false, nil
	}
	for batch := 0; batch < 10; batch++ {
		removed, err := receipts.Sweep(ctx, 100)
		if err != nil {
			return false, err
		}
		if removed < 100 {
			return false, nil
		}
		runtime.Gosched()
	}
	return true, nil
}

// reportIntegrity runs one structural check, publishes it for the diagnostic and
// says anything an owner needs to act on out loud. A check whose result only
// reaches a diagnostic nobody opened is a check that found nothing.
func reportIntegrity(ctx context.Context, db *sql.DB) {
	report := dbwork.Integrity(ctx, db)
	dbwork.PublishIntegrity(report)
	if report.Error != "" {
		log.Printf("Database structural check could not finish: %s", report.Error)
		return
	}
	if report.QuickCheck != "ok" && report.QuickCheck != "" {
		log.Printf("Database structural check reported: %s", report.QuickCheck)
	}
	if report.ForeignKeyViolations > 0 {
		// Named by child table, because "2,419 orphaned rows" is not something
		// anybody can act on and "1,310 of them in screen_metadata_fields" is.
		worst := ""
		for _, table := range dbwork.SortedTables(report.ByChildTable) {
			worst += fmt.Sprintf(" %s=%d", table, report.ByChildTable[table])
		}
		log.Printf("Database holds %d orphaned rows across %d tables:%s", report.ForeignKeyViolations, len(report.ByChildTable), worst)
	}
}
