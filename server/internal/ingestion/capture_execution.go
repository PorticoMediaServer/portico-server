package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

// analysisExecution names only this claimed invocation. Possession grants no
// owner-scan purpose; that additionally requires its original stored admission.
// It is copied at selection and never recovered from a latest-execution lookup.
type analysisExecution struct {
	job, id, object, objectRevision string
	revision, policyRevision        int64
}

func (s *Service) claimAnalysisExecution(ctx context.Context, job, object, revision string, policy int64, original catalog.LibrarySource) (analysisExecution, error) {
	var out analysisExecution
	if dbwork.Snapshot(ctx) != nil || dbwork.InWriteTx(ctx) {
		return out, context.Canceled
	}
	gate, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return out, err
	}
	defer gate.Rollback()
	tx := gate.Tx()
	source, err := catalog.InventoryFenceTx(ctx, tx, job)
	if err != nil {
		return out, err
	}
	if source.ID != original.ID || source.LibraryID != original.LibraryID || source.Generation != original.Generation ||
		source.Incarnation != original.Incarnation || source.RootIdentity != original.RootIdentity || source.ResolvedPath != original.ResolvedPath {
		return out, context.Canceled
	}
	var selected, selectedRevision string
	var selectedPolicy int64
	err = tx.QueryRowContext(ctx, `SELECT q.object_id,q.revision,q.policy_revision FROM inventory_analysis_queue q JOIN inventory_objects o ON o.id=q.object_id JOIN inventory_source_active a ON a.job_id=q.job_id AND a.source_id=o.source_id WHERE q.job_id=? AND q.state='pending' ORDER BY q.object_id LIMIT 1`, job).Scan(&selected, &selectedRevision, &selectedPolicy)
	if err != nil {
		return out, err
	}
	if selected != object || selectedRevision != revision || selectedPolicy != policy {
		return out, context.Canceled
	}
	current, err := catalog.ScanPolicyTx(ctx, tx, source.LibraryID)
	if err != nil {
		return out, err
	}
	if current.Revision != policy || !current.Allows("probe") {
		return out, context.Canceled
	}
	var previous int64
	var state string
	err = tx.QueryRowContext(ctx, `SELECT revision,state FROM inventory_capture_executions WHERE job_id=?`, job).Scan(&previous, &state)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if state == "active" {
		return out, storage.ErrBusy
	}
	if previous == 1<<63-1 {
		return out, context.Canceled
	}
	out = analysisExecution{job: job, id: identity.Token(), object: object, objectRevision: revision, revision: previous + 1, policyRevision: policy}
	if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_capture_executions(job_id,execution_id,revision,object_id,object_revision,policy_revision,state) VALUES(?,?,?,?,?,?,'active') ON CONFLICT(job_id) DO UPDATE SET execution_id=excluded.execution_id,revision=excluded.revision,object_id=excluded.object_id,object_revision=excluded.object_revision,policy_revision=excluded.policy_revision,state='active'`, job, out.id, out.revision, object, revision, policy); err != nil {
		return analysisExecution{}, err
	}
	if err = gate.Commit(); err != nil {
		return analysisExecution{}, err
	}
	return out, nil
}

// Cleanup keeps the original invocation key even after its caller was canceled.
// It is bounded, does no provider work, and cannot retire a successor claim.
func (s *Service) releaseAnalysisExecution(parent context.Context, original analysisExecution) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	_, _ = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `UPDATE inventory_capture_executions SET state='retired' WHERE job_id=? AND execution_id=? AND revision=? AND object_id=? AND object_revision=? AND policy_revision=? AND state='active'`, original.job, original.id, original.revision, original.object, original.objectRevision, original.policyRevision)
}

// Called before this process starts any inventory workers. A crash never lets a
// later process adopt the previous invocation's still-active durable identity.
func (s *Service) recoverCaptureExecutions(ctx context.Context) error {
	_, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `UPDATE inventory_capture_executions SET state='retired' WHERE state='active'`)
	return err
}
