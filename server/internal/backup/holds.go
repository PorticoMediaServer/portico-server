package backup

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"portico.local/server/internal/dbwork"
)

// A backup copies the database first and the files it names afterwards. If a
// collector deleted one of those files in between, the backup would either
// fail or carry a database that points at artwork it does not have. So, from
// before the first database step until the backup is published or abandoned,
// one row in backup_blob_holds tells every physical blob collector not to
// delete anything. It stops deletion only: writes, imports and revocations
// carry on.
//
// The row is durable so a restart cannot guess: a hold left by a process that
// is no longer running is that process's interrupted backup. Start-up removes
// it (ReleaseInterruptedHold) after the scratch it named has been cleaned, and
// a new backup replaces it in any case. There is no expiry: a hold that is
// still owned is never timed out from under a running copy.

// processEpoch identifies this process's holds.
var processEpoch = newToken()

// holdBlobs records the running backup's hold. It runs before the first
// database step: under hard WAL pressure it waits like any maintenance write,
// so copying never starts without the hold.
func (s *Service) holdBlobs(ctx context.Context, backupID, scratch string) error {
	result, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassMaintenance,
		`INSERT INTO backup_blob_holds(singleton,backup_id,process_epoch,scratch_token,created_ms,state) VALUES(1,?,?,?,?,'copying')
		 ON CONFLICT(singleton) DO UPDATE SET backup_id=excluded.backup_id,process_epoch=excluded.process_epoch,scratch_token=excluded.scratch_token,created_ms=excluded.created_ms,state='copying'
		 WHERE backup_blob_holds.process_epoch<>excluded.process_epoch`,
		backupID, processEpoch, scratch, s.now().UnixMilli())
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		// Another backup of this process holds it; backups run one at a time.
		return ErrBackupRunning
	}
	return nil
}

// advanceHold moves the hold to the backup's next phase.
func (s *Service) advanceHold(ctx context.Context, backupID, state string) error {
	_, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassMaintenance,
		`UPDATE backup_blob_holds SET state=? WHERE singleton=1 AND backup_id=? AND process_epoch=?`, state, backupID, processEpoch)
	return err
}

// releaseHold ends the hold whatever happened to the backup. It runs after a
// cancellation too, so it has its own short budget; a release that still
// fails leaves the hold to the next backup or the next start, which only
// delays artwork clean-up.
func (s *Service) releaseHold(backupID string) {
	ctx, cancel := context.WithTimeout(dbwork.WithClass(context.Background(), dbwork.ClassMaintenance), time.Minute)
	defer cancel()
	if _, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassMaintenance,
		`DELETE FROM backup_blob_holds WHERE singleton=1 AND backup_id=? AND process_epoch=?`, backupID, processEpoch); err != nil {
		log.Printf("warn: Backup could not release its artwork hold; artwork clean-up waits for the next backup or restart: %v", err)
	}
}

// ReleaseInterruptedHold removes a hold left by an earlier process. Start-up
// calls it after CleanupScratch, once the interrupted backup's scratch and
// partial folders are gone; a published backup is never part of a hold.
func ReleaseInterruptedHold(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("backup: no database")
	}
	_, err := dbwork.ExecWrite(dbwork.WithClass(ctx, dbwork.ClassMaintenance), db, dbwork.ClassMaintenance,
		`DELETE FROM backup_blob_holds WHERE process_epoch<>?`, processEpoch)
	return err
}
