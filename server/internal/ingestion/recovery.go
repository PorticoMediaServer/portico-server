package ingestion

import (
	"context"
	"errors"
	"os"
	"portico.local/server/internal/dbwork"
	"time"
)

// Persist only this stable reason. Owner responses map it to safe recovery copy.
const sourceUnavailable = "scan_source_unavailable"

// A scan's durable inventory is not proof that its source is still accessible.
// Never reconcile missing files when the final source check cannot succeed.
func (s *Service) sourceReady(ctx context.Context, library string) error {
	var root string
	if err := s.db.QueryRowContext(ctx, `SELECT root FROM libraries WHERE id=?`, library).Scan(&root); err != nil {
		return err
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if s.storage != nil {
		_, err := s.storage.InspectRoot(check, root)
		return err
	}
	if err := check.Err(); err != nil {
		return err
	}
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("scan source is not a directory")
	}
	return check.Err()
}

func (s *Service) stopForUnavailableSource(ctx context.Context, job, library string) bool {
	err := s.sourceReady(ctx, library)
	if err == nil && s.storage != nil {
		var root string
		err = s.db.QueryRowContext(ctx, `SELECT root FROM libraries WHERE id=?`, library).Scan(&root)
		if err == nil && s.storage.IsRemote(root) {
			err = s.storage.Remote.CheckScan(ctx, job, root)
		}
	}
	if err != nil {
		// Shutdown leaves the durable continuation available for the next worker.
		if ctx.Err() == nil {
			_, _ = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `UPDATE jobs SET status='failed',error=? WHERE id=? AND status='running'`, sourceUnavailable, job)
		}
		return true
	}
	return false
}
