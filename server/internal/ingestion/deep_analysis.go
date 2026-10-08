package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/supervise"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/mediaanalysis"
)

func (s *Service) analyzeDeep(parent context.Context, job, object, revision string, policy catalog.ScanPolicy, source catalog.LibrarySource) error {
	if s.DeepAnalysis == nil {
		return s.analysisOutcome(parent, job, object, revision, policy.Revision, true, "analysis_worker_unavailable")
	}
	ctx, cancel := context.WithTimeout(parent, 6*time.Hour)
	defer cancel()
	stopped := make(chan struct{})
	watchDone := make(chan struct{})
	defer func() { close(stopped); cancel(); <-watchDone }()
	supervise.Go("ingestion.deep-analysis-watch", func() {
		defer close(watchDone)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopped:
				return
			case <-ticker.C:
				var state string
				if s.db.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id=?`, job).Scan(&state) != nil || state != "running" {
					cancel()
					return
				}
				current, e := s.catalog.ScanPolicy(ctx, source.LibraryID)
				if e != nil || current.Revision != policy.Revision {
					cancel()
					return
				}
				if busy, e := s.playbackBusy(ctx, source.ID); e != nil || busy {
					cancel()
					return
				}
			}
		}
	})
	done, warning, err := s.DeepAnalysis.RunOne(ctx, mediaanalysis.Request{JobID: job, ObjectID: object, SourceRevision: revision, PolicyRevision: policy.Revision})
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return nil
	}
	if err != nil {
		return err
	}
	if done {
		return s.analysisOutcome(parent, job, object, revision, policy.Revision, warning != "", warning)
	}
	return nil
}

// QueueItemAnalysis uses the existing durable job, source deduplication and
// worker. It never broadens the saved Complete/Custom policy. An in-flight scan
// is coalesced rather than replaced or silently cancelled.
func (s *Service) QueueItemAnalysis(ctx context.Context, library, asset, revision string, authorize func(*sql.Tx) error) (Job, error) {
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if e != nil {
		return Job{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if authorize != nil {
		if e = authorize(tx); e != nil {
			return Job{}, e
		}
	}
	var source, object string
	e = tx.QueryRowContext(ctx, `SELECT o.source_id,o.id FROM inventory_objects o JOIN library_sources src ON src.id=o.source_id WHERE o.asset_id=? AND o.revision=? AND src.library_id=? AND src.enabled=1 AND o.retired=0 AND o.state='available' AND o.root_incarnation=src.incarnation ORDER BY o.id LIMIT 1`, asset, revision, library).Scan(&source, &object)
	if e != nil {
		return Job{}, e
	}
	p, e := catalog.ScanPolicyTx(ctx, tx, library)
	if e != nil {
		return Job{}, e
	}
	allowed := false
	for _, op := range catalog.DeepScanOperations {
		allowed = allowed || p.Allows(op)
	}
	if !allowed {
		return Job{}, catalog.ErrAdminQuery
	}
	j, e := queueSourceTx(ctx, tx, source, true)
	if e != nil {
		return Job{}, e
	}
	// Coalescing with an active source job must not lose an item that its enqueue
	// cursor already passed. Retry resets only failed attempts for this explicit
	// owner request; completed immutable stages remain reusable.
	if _, e = tx.ExecContext(ctx, `INSERT INTO inventory_analysis_queue(job_id,object_id,revision,policy_revision,state) VALUES(?,?,?,?,'pending') ON CONFLICT(job_id,object_id) DO UPDATE SET revision=excluded.revision,policy_revision=excluded.policy_revision,state='pending'`, j.ID, object, revision, p.Revision); e != nil {
		return Job{}, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE analysis_stage_runs SET state='pending',attempt=0,error_code='',next_ms=0 WHERE job_id=? AND object_id=? AND source_revision=? AND state IN('failed','unsupported')`, j.ID, object, revision); e != nil {
		return Job{}, e
	}
	// Analysis enqueue examines the whole source in bounded pages, but executes
	// only missing permitted stages. This is explicit in the owner UI.
	if e = gated.Commit(); e != nil {
		return Job{}, e
	}
	return j, nil
}
