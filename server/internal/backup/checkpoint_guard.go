package backup

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"portico.local/server/internal/dbwork"
)

// A backup holds one read snapshot for its whole copy. While that snapshot is
// open SQLite's writer-side automatic checkpoint cannot copy frames past the
// snapshot's start mark, so it can neither bound the log nor reset it: the work
// is wasted inside a committing writer's transaction, where it holds the write
// gate. When the snapshot closes, the next writer commit would otherwise run
// that whole accumulated backlog through the same gate in one step.
//
// holdWriterCheckpoint therefore turns the writer's backstop off for the
// duration of the backup, relies on the read connection's PASSIVE checkpoint
// after the snapshot closes to copy the backlog outside the write gate, and then
// restores the configured backstop. The log was already unbounded by the
// automatic checkpoint while the snapshot existed, so nothing is given up; the
// backstop is restored even when the copy fails, and its configured value is
// read from the writer rather than assumed.
func (s *Service) holdWriterCheckpoint(ctx context.Context) func() {
	if s == nil || s.db == nil {
		return func() {}
	}
	s.checkpointMu.Lock()
	if s.checkpointHolds == 0 {
		prior := 0
		err := dbwork.WithWriteTx(ctx, s.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, `PRAGMA wal_autocheckpoint`).Scan(&prior); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `PRAGMA wal_autocheckpoint=0`)
			return err
		})
		if err != nil {
			s.checkpointMu.Unlock()
			return func() {}
		}
		s.checkpointPrior = prior
	}
	s.checkpointHolds++
	s.checkpointMu.Unlock()

	var once sync.Once
	return func() { once.Do(s.releaseWriterCheckpoint) }
}

func (s *Service) releaseWriterCheckpoint() {
	// Hold the guard's mutex across the drain so a concurrent backup cannot
	// re-suspend the backstop between our drain and our restore and then have
	// its own suspension undone by this restore.
	s.checkpointMu.Lock()
	defer s.checkpointMu.Unlock()
	s.checkpointHolds--
	if s.checkpointHolds > 0 {
		return
	}
	prior := s.checkpointPrior

	// Drain first, while the backstop is still off, so the next commit cannot
	// pick the backlog up inside the write gate. Both steps are maintenance on
	// the backup's own goroutine and must run even if the backup was cancelled:
	// the writer's configured backstop is restored regardless. The drain is
	// bounded so a cancelled backup still returns promptly.
	drain, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	dbwork.Checkpoint(drain, s.db)
	if _, err := dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassMaintenance, fmt.Sprintf(`PRAGMA wal_autocheckpoint=%d`, prior)); err != nil {
		// A failed restore leaves the configured backstop off until the next
		// successful hold or the process opens the database again. The drain
		// above already covered the backlog this hold was shielding.
		log.Printf("warn: Backup could not restore the writer checkpoint backstop: %v", err)
	}
}
