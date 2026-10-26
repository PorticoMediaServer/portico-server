package ingestion

import (
	"context"
	"database/sql"
	"portico.local/server/internal/requestinput"
	"time"
)

type scanModeKey struct{}

// QueueMode preserves the existing queue and authorization boundary. Empty mode
// chooses first import, weekly integrity, or incremental automatically.
func (s *Service) QueueMode(ctx context.Context, library, source, mode string, authorize func(*sql.Tx) error) (Job, error) {
	return s.QueueModeCreated(ctx, library, source, mode, authorize, nil)
}

// QueueModeCreated is the private created-job seam over QueueMode: each scan job
// this request actually creates is reported to onCreated inside the accepting
// enqueue transaction, and a coalesced job is never reported. It is not a route,
// body, purpose token or authority of its own; callers still pass their real
// authorize callback, and a nil onCreated is exactly QueueMode.
func (s *Service) QueueModeCreated(ctx context.Context, library, source, mode string, authorize func(*sql.Tx) error, onCreated func(context.Context, *sql.Tx, CreatedScanJob) error) (Job, error) {
	if mode != "" && mode != "incremental" && mode != "full" && mode != "integrity" {
		return Job{}, requestinput.New("invalid scan mode")
	}
	return s.queueContextCreated(context.WithValue(ctx, scanModeKey{}, mode), library, source, authorize, onCreated)
}
func scanModeTx(ctx context.Context, tx *sql.Tx, source string) (string, error) {
	mode, _ := ctx.Value(scanModeKey{}).(string)
	if mode == "full" || mode == "integrity" {
		return mode, nil
	}
	var complete, integrity string
	err := tx.QueryRowContext(ctx, `SELECT last_complete_at,last_integrity_at FROM library_sources s WHERE id=?`, source).Scan(&complete, &integrity)
	if err != nil {
		return "", err
	}
	if complete == "" {
		return "first-import", nil
	}
	checked, err := time.Parse(time.RFC3339Nano, integrity)
	if err != nil || time.Since(checked) >= 7*24*time.Hour {
		return "integrity", nil
	}
	return "incremental", nil
}
