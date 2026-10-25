package dvr

import (
	"context"
	"database/sql"
)

// AdmitRootChange runs inside the caller's exclusive database write transaction.
// Hold its release through commit/rollback: no worker may register while root
// admission is being persisted. Any retained recording (including incomplete or
// cancelled rows), layout/artifact pin or prepared work refuses relocation.
func (s *Store) AdmitRootChange(ctx context.Context, tx *sql.Tx) (func(), error) {
	s.mu.Lock()
	if len(s.active) != 0 { s.mu.Unlock(); return nil, ErrConflict }
	var retained bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dvr_recordings) OR EXISTS(SELECT 1 FROM dvr_recording_layouts) OR EXISTS(SELECT 1 FROM dvr_recording_shares) OR EXISTS(SELECT 1 FROM dvr_catalog_provenance) OR EXISTS(SELECT 1 FROM live_allocations WHERE kind='recording' AND state IN('active','preempting')) OR EXISTS(SELECT 1 FROM dvr_rule_work WHERE phase<>'idle') OR EXISTS(SELECT 1 FROM dvr_delete_work) OR EXISTS(SELECT 1 FROM dvr_cleanup_work) OR EXISTS(SELECT 1 FROM dvr_retention_retry)`).Scan(&retained)
	if err != nil || retained {
		s.mu.Unlock()
		if err != nil { return nil, err }
		return nil, ErrConflict
	}
	return s.mu.Unlock, nil
}
