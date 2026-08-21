package database

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type ErrorKind string

const (
	ErrorKindUnknown    ErrorKind = "unknown"
	ErrorKindBusy       ErrorKind = "busy"
	ErrorKindLocked     ErrorKind = "locked"
	ErrorKindConstraint ErrorKind = "constraint"
	ErrorKindCorrupt    ErrorKind = "corrupt"
)

type RetryOptions struct {
	Attempts int
	Base     time.Duration
	Max      time.Duration
}

type RetryStats struct {
	Attempts int
	Retries  int
	Wait     time.Duration
}

var (
	UserWriteRetry       = RetryOptions{Attempts: 3, Base: 20 * time.Millisecond, Max: 150 * time.Millisecond}
	BackgroundWriteRetry = RetryOptions{Attempts: 3, Base: 50 * time.Millisecond, Max: 250 * time.Millisecond}
)

func ClassifyError(err error) ErrorKind {
	if err == nil {
		return ErrorKindUnknown
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch primarySQLiteCode(sqliteErr.Code()) {
		case sqlite3.SQLITE_BUSY:
			return ErrorKindBusy
		case sqlite3.SQLITE_LOCKED:
			return ErrorKindLocked
		case sqlite3.SQLITE_CONSTRAINT:
			return ErrorKindConstraint
		case sqlite3.SQLITE_CORRUPT, sqlite3.SQLITE_NOTADB:
			return ErrorKindCorrupt
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "sqlite_busy") || strings.Contains(msg, "database is locked"):
		return ErrorKindBusy
	case strings.Contains(msg, "sqlite_locked") || strings.Contains(msg, "database table is locked"):
		return ErrorKindLocked
	case strings.Contains(msg, "sqlite_constraint") || strings.Contains(msg, "constraint failed") || strings.Contains(msg, "constraint violation"):
		return ErrorKindConstraint
	case strings.Contains(msg, "sqlite_corrupt") || strings.Contains(msg, "database disk image is malformed") || strings.Contains(msg, "file is not a database"):
		return ErrorKindCorrupt
	default:
		return ErrorKindUnknown
	}
}

func IsRetryableLock(err error) bool {
	switch ClassifyError(err) {
	case ErrorKindBusy, ErrorKindLocked:
		return true
	default:
		return false
	}
}

func ExecWithRetry(ctx context.Context, db *sql.DB, opts RetryOptions, query string, args ...any) (sql.Result, error) {
	result, _, err := ExecWithRetryStats(ctx, db, opts, query, args...)
	return result, err
}

func ExecWithRetryStats(ctx context.Context, db *sql.DB, opts RetryOptions, query string, args ...any) (sql.Result, RetryStats, error) {
	if opts.Attempts <= 0 {
		opts.Attempts = 1
	}
	var result sql.Result
	stats, err := retrySQLiteLock(ctx, opts, func() error {
		var execErr error
		result, execErr = db.ExecContext(ctx, query, args...)
		return execErr
	})
	return result, stats, err
}

func WithTxRetry(ctx context.Context, db *sql.DB, opts RetryOptions, fn func(*sql.Tx) error) error {
	_, err := WithTxRetryStats(ctx, db, opts, fn)
	return err
}

func WithTxRetryStats(ctx context.Context, db *sql.DB, opts RetryOptions, fn func(*sql.Tx) error) (RetryStats, error) {
	if opts.Attempts <= 0 {
		opts.Attempts = 1
	}
	return retrySQLiteLock(ctx, opts, func() error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	})
}

func retrySQLiteLock(ctx context.Context, opts RetryOptions, fn func() error) (RetryStats, error) {
	var stats RetryStats
	var err error
	for attempt := 0; attempt < opts.Attempts; attempt++ {
		stats.Attempts++
		if ctxErr := ctx.Err(); ctxErr != nil {
			return stats, ctxErr
		}
		err = fn()
		if err == nil || !IsRetryableLock(err) || attempt == opts.Attempts-1 {
			return stats, err
		}
		delay := retryDelay(opts, attempt)
		stats.Retries++
		stats.Wait += delay
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return stats, ctx.Err()
		case <-timer.C:
		}
	}
	return stats, err
}

func retryDelay(opts RetryOptions, attempt int) time.Duration {
	base := opts.Base
	if base <= 0 {
		base = 25 * time.Millisecond
	}
	maxDelay := opts.Max
	if maxDelay <= 0 {
		maxDelay = time.Second
	}
	delay := base << min(attempt, 8)
	if delay > maxDelay {
		delay = maxDelay
	}
	jitter := time.Duration(rand.Int63n(int64(delay/4 + 1)))
	return delay + jitter
}

func primarySQLiteCode(code int) int {
	return code & 0xff
}
