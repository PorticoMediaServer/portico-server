package mediaanalysis

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/dbwork"
)

// ContinueAdmission consumes the same admission hook as stage start. The
// ingestion worker calls it from its EXISTING policy/cancel watcher; no second
// timer, work window or background scheduler is introduced here.
func (s *Service) ContinueAdmission(ctx context.Context, job, object string) (bool, error) {
	if s == nil || s.Admission == nil {
		return true, nil
	}
	tx, closeRead, err := dbwork.BeginRead(ctx, s.db)
	if err != nil {
		return false, err
	}
	defer closeRead()
	var stage string
	err = tx.QueryRowContext(ctx, `SELECT stage FROM analysis_stage_runs WHERE job_id=? AND object_id=? AND state='running' ORDER BY updated_ms DESC LIMIT 1`, job, object).Scan(&stage)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	task := "analysis"
	if stage == "trickplay" {
		task = "trickplay"
	}
	return s.Admission(ctx, tx, task)
}
