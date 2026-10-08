package persistence

import (
	"context"
	"errors"
	"os"

	"portico.local/server/internal/dbwork"
)

// Restore-candidate validation errors. The admin API reports them as
// restore_not_portico, restore_integrity_failed, restore_schema_newer and
// restore_pre_release.
var (
	ErrCandidateNotPortico = errors.New("this file is not a Portico database")
	ErrCandidateIntegrity  = errors.New("this database copy failed its integrity check")
	ErrCandidateNewer      = errors.New("this database was written by a newer version of Portico")
	ErrCandidatePreRelease = ErrSchemaResetRequired
)

// sqliteHeader opens every SQLite database file. A candidate without it is
// not a database at all, let alone a Portico one.
const sqliteHeader = "SQLite format 3\x00"

// InspectCandidate validates a restore candidate at path without changing the
// live database and without running migrations on the candidate. It returns
// the candidate's schema version: an older version is fine, because the
// ordinary migrations run when the restored server starts.
func InspectCandidate(ctx context.Context, path string) (int, error) {
	head, err := os.Open(path)
	if err != nil {
		return 0, ErrCandidateNotPortico
	}
	prefix := make([]byte, len(sqliteHeader))
	_, err = readFull(head, prefix)
	closeErr := head.Close()
	if err != nil || closeErr != nil || string(prefix) != sqliteHeader {
		return 0, ErrCandidateNotPortico
	}
	db, err := dbwork.OpenHandle(path, dbwork.DefaultPolicy())
	if err != nil {
		return 0, ErrCandidateIntegrity
	}
	defer db.Close()
	// Merge any WAL frames first so the check sees the whole database.
	_, _ = db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return 0, ErrCandidateIntegrity
	}
	lines := []string{}
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			rows.Close()
			return 0, ErrCandidateIntegrity
		}
		lines = append(lines, line)
	}
	scanErr := rows.Err()
	rows.Close()
	if scanErr != nil || len(lines) != 1 || lines[0] != "ok" {
		return 0, ErrCandidateIntegrity
	}
	var tables int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN('configuration','schema_migrations')`).Scan(&tables); err != nil || tables != 2 {
		return 0, ErrCandidateNotPortico
	}
	steps, err := schemaMigrations()
	if err != nil {
		return 0, err
	}
	latest := steps[len(steps)-1].version
	stored, err := storedSchemaVersion(ctx, db)
	if err != nil {
		return 0, err
	}
	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return 0, err
	}
	// Pre-release first (see migrateWith): its schema version is above the
	// baseline's too.
	if applied[baselineSchemaVersion] == "" {
		return 0, ErrCandidatePreRelease
	}
	if stored > latest {
		return 0, ErrCandidateNewer
	}
	embedded := make(map[int]bool, len(steps))
	for _, step := range steps {
		embedded[step.version] = true
	}
	for version := range applied {
		if !embedded[version] {
			return 0, ErrCandidateNewer
		}
	}
	return stored, nil
}

func readFull(file *os.File, buf []byte) (int, error) {
	done := 0
	for done < len(buf) {
		n, err := file.Read(buf[done:])
		done += n
		if err != nil {
			return done, err
		}
	}
	return done, nil
}
