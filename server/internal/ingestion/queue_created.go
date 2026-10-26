package ingestion

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"time"
)

// CreatedScanJob names one scan job this request newly created. It is a private
// Go value passed to QueueModeCreated's callback: it has no wire shape, no
// public route and no purpose token, and it is never reported on Job.
type CreatedScanJob struct {
	JobID     string
	LibraryID string
	SourceID  string
}

// queueContextCreated is QueueContext with the private created-job seam. The
// callback runs inside the accepting enqueue transaction, once per job this call
// creates, after that job, its inventory run and its active-source row are
// inserted; a coalesced (reused) job never reaches it. A callback error aborts
// the whole enqueue transaction, so every child created by a library fan-out is
// rolled back with it. The callback must be DB-only and must not open another
// transaction.
func (s *Service) queueContextCreated(ctx context.Context, library, sourceID string, authorize func(*sql.Tx) error, onCreated func(context.Context, *sql.Tx, CreatedScanJob) error) (Job, error) {
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return Job{}, err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if authorize != nil {
		if err = authorize(tx); err != nil {
			return Job{}, err
		}
	}
	j, err := s.queueTxCreated(ctx, tx, library, sourceID, onCreated)
	if err != nil {
		return Job{}, err
	}
	if err = gated.Commit(); err != nil {
		return Job{}, err
	}
	return j, nil
}

// queueTxCreated is queueTx with the created-job callback. It resolves the same
// enabled sources in the same order and passes the same Job shape; each child it
// actually creates reports itself once, and any refusal aborts the fan-out.
func (s *Service) queueTxCreated(ctx context.Context, tx *sql.Tx, library, sourceID string, onCreated func(context.Context, *sql.Tx, CreatedScanJob) error) (Job, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM library_sources WHERE library_id=? AND enabled=1 AND NOT EXISTS(SELECT 1 FROM dvr_private_libraries d WHERE d.library_id=library_sources.library_id) AND (?='' OR id=?) ORDER BY CASE WHEN id=library_id THEN 0 ELSE 1 END,id`, library, sourceID, sourceID)
	if err != nil {
		return Job{}, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return Job{}, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Job{}, err
	}
	if len(ids) == 0 {
		return Job{}, sql.ErrNoRows
	}
	var first Job
	all := []string{}
	for _, id := range ids {
		j, e := queueSourceTxCreated(ctx, tx, id, false, onCreated)
		if e != nil {
			return Job{}, e
		}
		if first.ID == "" {
			first = j
		}
		all = append(all, j.ID)
	}
	first.Jobs = all
	return first, nil
}

// queueSourceTxCreated is queueSourceTx with the created-job callback. It
// returns the active job unchanged when the source already has one; only the
// insert path runs onCreated, and only after the job, its run, its directories
// and its active-source row exist in this transaction.
func queueSourceTxCreated(ctx context.Context, tx *sql.Tx, source string, analysisOnly bool, onCreated func(context.Context, *sql.Tx, CreatedScanJob) error) (Job, error) {
	var j Job
	j.SourceID = source
	err := tx.QueryRowContext(ctx, `SELECT j.id,j.status,j.processed,r.phase,r.analyzed,r.warnings FROM inventory_source_active a JOIN jobs j ON j.id=a.job_id JOIN inventory_runs r ON r.job_id=j.id WHERE a.source_id=? AND j.status IN('queued','running','paused')`, source).Scan(&j.ID, &j.Status, &j.Processed, &j.Phase, &j.Analyzed, &j.Warnings)
	if err == nil {
		return j, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return j, err
	}
	var library, incarnation, rootIdentity string
	var generation, policy int64
	err = tx.QueryRowContext(ctx, `SELECT s.library_id,s.generation,s.incarnation,s.root_identity,p.revision FROM library_sources s JOIN library_scan_policies p ON p.library_id=s.library_id WHERE s.id=? AND s.enabled=1 AND NOT EXISTS(SELECT 1 FROM dvr_private_libraries d WHERE d.library_id=s.library_id)`, source).Scan(&library, &generation, &incarnation, &rootIdentity, &policy)
	if err != nil {
		return j, err
	}
	j.ID, j.Status, j.Phase = identity.Token(), "queued", "inventory"
	if analysisOnly {
		j.Phase = "analysis_enqueue"
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,library_id,status,created_at) VALUES(?,?,'queued',?)`, j.ID, library, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return j, err
	}
	mode, err := scanModeTx(ctx, tx, source)
	if err != nil {
		return j, err
	}
	j.ScanMode = mode
	if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision,phase,scan_mode) VALUES(?,?,?,?,?,?,?,?)`, j.ID, source, generation, incarnation, rootIdentity, policy, j.Phase, mode); err != nil {
		return j, err
	}
	if !analysisOnly {
		if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_directories(job_id,relative_path) VALUES(?,'.')`, j.ID); err != nil {
			return j, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_source_active(source_id,job_id) VALUES(?,?) ON CONFLICT(source_id) DO UPDATE SET job_id=excluded.job_id`, source, j.ID); err != nil {
		return j, err
	}
	if onCreated != nil {
		if err = onCreated(ctx, tx, CreatedScanJob{JobID: j.ID, LibraryID: library, SourceID: source}); err != nil {
			return Job{}, err
		}
	}
	return j, nil
}
