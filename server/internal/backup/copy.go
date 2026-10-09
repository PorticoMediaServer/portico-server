package backup

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"

	sqlite "portico.local/server/internal/thirdparty/sqlite"

	"portico.local/server/internal/dbwork"
)

// copyDatabase writes a consistent copy of the live database to dest with
// SQLite's online backup API while the server runs. It pins one WAL read
// snapshot on a background-pool connection so concurrent commits cannot restart
// the copy from page zero, steps in small increments so a writer never waits
// long behind it, and never takes the write gate. progress reports
// (PageCount-Remaining)*pageSize as the copy advances.
func copyDatabase(ctx context.Context, source *sql.DB, dest string, progress func(done, total int64)) error {
	if source == nil || dest == "" {
		return errors.New("backup: no database to copy")
	}
	ctx = dbwork.WithClass(ctx, dbwork.ClassMaintenance)
	conn, err := dbwork.ReadHandle(ctx, source).Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var pageSize int64
	if err = conn.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil || pageSize < 512 || pageSize > 65536 || pageSize&(pageSize-1) != 0 {
		return errors.New("backup: unexpected database page size")
	}
	// One pinned read snapshot on this same connection: the copy sees one WAL
	// frame boundary, takes no writer admission lease, and never blocks.
	snapshot, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer snapshot.Rollback()
	var objects int
	if err = snapshot.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&objects); err != nil {
		return err
	}
	file, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	err = conn.Raw(func(raw any) error {
		// The handle is instrumented; the backup API lives on the connection
		// underneath it.
		driver, ok := dbwork.RawConn(raw).(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("backup: online backup unavailable")
		}
		backup, err := driver.NewBackup(dest)
		if err != nil {
			return err
		}
		var copyErr error
		lastReport := time.Now()
		for {
			if ctx.Err() != nil {
				copyErr = ctx.Err()
				break
			}
			more, err := backup.Step(16)
			copied := int64(backup.PageCount()-backup.Remaining()) * pageSize
			total := int64(backup.PageCount()) * pageSize
			if progress != nil && (time.Since(lastReport) >= 200*time.Millisecond || !more && err == nil) {
				progress(copied, total)
				lastReport = time.Now()
			}
			if err != nil {
				var sqliteErr *sqlite.Error
				if errors.As(err, &sqliteErr) && (sqliteErr.Code()&255 == 5 || sqliteErr.Code()&255 == 6) {
					timer := time.NewTimer(20 * time.Millisecond)
					select {
					case <-ctx.Done():
						timer.Stop()
						copyErr = ctx.Err()
					case <-timer.C:
					}
					if copyErr != nil {
						break
					}
					continue
				}
				copyErr = err
				break
			}
			if !more {
				break
			}
		}
		if finishErr := backup.Finish(); copyErr == nil {
			copyErr = finishErr
		}
		return copyErr
	})
	if err != nil {
		return err
	}
	// Finish has made the destination independent of the source. Release its
	// WAL boundary and pooled connection before syncing or checking the copy;
	// target durability and integrity work must not hold back live checkpoints.
	if err = snapshot.Rollback(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if err = conn.Close(); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil || info.Size() < 512 || ctx.Err() != nil {
		return errors.New("backup: database copy is incomplete")
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = quickCheck(dest); err != nil {
		return err
	}
	return nil
}

// quickCheck runs PRAGMA quick_check over a finished copy: a torn copy must
// never become a listed backup.
func quickCheck(path string) error {
	db, err := dbwork.OpenHandle(path, dbwork.DefaultPolicy())
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query(`PRAGMA quick_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			return err
		}
		if line != "ok" {
			return errors.New("backup: database copy failed quick_check")
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if count != 1 {
		return errors.New("backup: database copy failed quick_check")
	}
	return nil
}
