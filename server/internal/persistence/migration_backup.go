package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"portico.local/server/internal/dbwork"
	sqlite "portico.local/server/internal/thirdparty/sqlite"
)

// backupBeforeMigrations is called only after validating the existing ledger.
// The online backup includes WAL contents; a raw copy of the main file cannot.
func backupBeforeMigrations(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var sequence int
	var name, path string
	if err = conn.QueryRowContext(ctx, "PRAGMA database_list").Scan(&sequence, &name, &path); err != nil {
		return err
	}
	if path == "" {
		return nil
	} // In-memory fixtures have no persistent state.
	directory := filepath.Join(filepath.Dir(path), "migration-backups")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".partial-*")
	if err != nil {
		return err
	}
	partial := file.Name()
	defer os.Remove(partial)
	if err = file.Close(); err != nil {
		return err
	}
	err = conn.Raw(func(raw any) error {
		driver, ok := dbwork.RawConn(raw).(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("database backup unavailable")
		}
		copy, err := driver.NewBackup(partial)
		if err != nil {
			return err
		}
		finished := false
		defer func() {
			if !finished {
				_ = copy.Finish()
			}
		}()
		for {
			if err = ctx.Err(); err != nil {
				return err
			}
			more, err := copy.Step(128)
			if err != nil {
				return err
			}
			if !more {
				finished = true
				return copy.Finish()
			}
		}
	})
	if err != nil {
		return err
	}
	file, err = os.OpenFile(partial, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	final := filepath.Join(directory, "before-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".sqlite")
	if err = os.Rename(partial, final); err != nil {
		return err
	}
	names, err := filepath.Glob(filepath.Join(directory, "before-*.sqlite"))
	if err != nil {
		return err
	}
	sort.Strings(names)
	for len(names) > 3 {
		if err = os.Remove(names[0]); err != nil {
			return err
		}
		names = names[1:]
	}
	return nil
}

// ReclaimSpace moves at most 64 free pages per maintenance tick. It yields to
// foreground work and never converts a pre-release database with a full VACUUM.
func ReclaimSpace(ctx context.Context, db *sql.DB) error {
	if dbwork.WriteGate().ActiveOrWaiting() || dbwork.ForegroundWorkActive() {
		return nil
	}
	var free int
	if err := db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&free); err != nil || free == 0 {
		return err
	}
	write, err := dbwork.Begin(ctx, db, dbwork.ClassMaintenance)
	if err != nil {
		return err
	}
	defer write.Rollback()
	rows, err := write.Tx().QueryContext(ctx, "PRAGMA incremental_vacuum(64)")
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var remaining, pageSize int64
	if err = write.Tx().QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&remaining); err != nil {
		return err
	}
	if err = write.Tx().QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return err
	}
	if _, err = write.Tx().ExecContext(ctx, `UPDATE database_maintenance_status SET last_ran_ms=?,last_reclaimed_bytes=? WHERE singleton=1`, time.Now().UnixMilli(), max(0, int64(free)-remaining)*pageSize); err != nil {
		return err
	}
	return write.Commit()
}
