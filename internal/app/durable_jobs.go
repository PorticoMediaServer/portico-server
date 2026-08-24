package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/database"
)

var errJobAdmissionClosed = errors.New("job admission is closed")

const (
	jobDefaultRetentionMax   = 5000
	jobMaxActiveKeyBytes     = 240
	jobMaxMetadataEntries    = 32
	jobMaxMetadataKeyBytes   = 80
	jobMaxMetadataValueBytes = 500
)

// durableJobSelectColumns is deliberately shared by list/claim/detail reads.
// The job envelope is part of the same durable row; selecting it with the
// display fields avoids the old N+1 hydration query for every dashboard row.
const durableJobSelectColumns = `
	id, type, status, progress, message, resource_type, resource_id,
	COALESCE(metadata_json, '{}'), attempt_count, next_run_at, last_error,
	failure_kind, created_at, updated_at, COALESCE(parent_operation_id, ''),
	COALESCE(idempotency_key, ''), COALESCE(active_key, ''),
	COALESCE(priority, 'normal'), COALESCE(phase, ''),
	COALESCE(progress_current, 0), COALESCE(progress_total, 0),
	COALESCE(result_reference, ''), COALESCE(error_code, ''),
	COALESCE(retry_eligible, 0), COALESCE(cancellation_requested_at, ''),
	COALESCE(worker_acknowledged_at, ''), COALESCE(interrupted_at, ''),
	COALESCE(retention_until, '')`

type durableJobScanner interface {
	Scan(dest ...any) error
}

func scanDurableJob(scanner durableJobScanner) (Job, error) {
	var job Job
	var metadataJSON string
	var retryEligible int
	err := scanner.Scan(
		&job.ID, &job.Type, &job.Status, &job.Progress, &job.Message,
		&job.ResourceType, &job.ResourceID, &metadataJSON, &job.AttemptCount,
		&job.NextRunAt, &job.LastError, &job.FailureKind, &job.CreatedAt,
		&job.UpdatedAt, &job.ParentOperationID, &job.IdempotencyKey,
		&job.ActiveKey, &job.Priority, &job.Phase, &job.ProgressCurrent,
		&job.ProgressTotal, &job.ResultReference, &job.ErrorCode,
		&retryEligible, &job.CancellationRequestedAt,
		&job.WorkerAcknowledgedAt, &job.InterruptedAt, &job.RetentionUntil,
	)
	if err != nil {
		return Job{}, err
	}
	job.Metadata = decodeJobMetadata(metadataJSON)
	job.RetryEligible = retryEligible != 0
	return job, nil
}

// A process-wide gate keeps multiple Server generations/fixtures sharing one
// *sql.DB from stampeding SQLite's single writer. The partial unique index is
// still the durable cross-process authority; this gate only reduces avoidable
// SQLITE_BUSY churn within one process.
var durableJobEnqueueMu sync.Mutex

var durableJobEnqueueRetry = database.RetryOptions{
	Attempts: 8,
	Base:     50 * time.Millisecond,
	Max:      time.Second,
}

const jobTerminalWriteTimeout = 5 * time.Second

func boundedJobWriteContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), jobTerminalWriteTimeout)
}

func (s *Server) jobAdmissionIsClosed() bool {
	if s == nil {
		return true
	}
	s.jobLifecycleMu.Lock()
	closed := s.jobAdmissionClosed
	s.jobLifecycleMu.Unlock()
	return closed
}

func (s *Server) jobRuntimeIsActive() bool {
	if s == nil {
		return false
	}
	s.jobLifecycleMu.Lock()
	active := s.jobRuntimeActive && !s.jobAdmissionClosed
	s.jobLifecycleMu.Unlock()
	return active
}

// withJobAdmission holds the admission gate over the durable enqueue
// transaction. That makes shutdown's gate linearization point explicit: once
// BeginShutdown obtains this mutex, no enqueue can still be between its
// admission check and its database commit.
func (s *Server) withJobAdmission(fn func() (Job, error)) (Job, error) {
	if s == nil || fn == nil {
		return Job{}, errJobAdmissionClosed
	}
	s.jobLifecycleMu.Lock()
	defer s.jobLifecycleMu.Unlock()
	if s.jobAdmissionClosed || s.restoreBarrier.isBlocked() {
		return Job{}, errJobAdmissionClosed
	}
	return fn()
}

func (s *Server) setJobRuntimeActive(active bool) {
	if s == nil {
		return
	}
	s.jobLifecycleMu.Lock()
	s.jobRuntimeActive = active && !s.jobAdmissionClosed
	s.jobLifecycleMu.Unlock()
}

func (s *Server) signalJobWake() {
	if s == nil {
		return
	}
	s.jobLifecycleMu.Lock()
	active := s.jobRuntimeActive && !s.jobAdmissionClosed
	wake := s.jobWake
	s.jobLifecycleMu.Unlock()
	if !active || wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

func (s *Server) beginJobRun(job Job, release func()) bool {
	if s == nil || release == nil {
		if release != nil {
			release()
		}
		return false
	}
	s.jobLifecycleMu.Lock()
	if s.jobAdmissionClosed || s.restoreBarrier.isBlocked() {
		s.jobLifecycleMu.Unlock()
		release()
		return false
	}
	if s.jobRuns == nil {
		s.jobRuns = map[string]struct{}{}
	}
	// The database claim remains authoritative, but tracking the selected ID
	// here closes the dispatcher-to-worker and shutdown join race.
	s.jobRuns[job.ID] = struct{}{}
	s.jobRunWG.Add(1)
	s.jobLifecycleMu.Unlock()
	go func() {
		defer release()
		defer s.finishJobRun(job.ID)
		defer func() {
			if recovered := recover(); recovered != nil {
				s.recordLog("error", "Durable job worker panicked", map[string]string{
					"job":  job.ID,
					"type": job.Type,
				})
				_ = s.setJobFailure(job.ID, "job_worker_panic", "Job worker failed unexpectedly.")
			}
		}()
		s.runJobWithLane(job)
	}()
	return true
}

func (s *Server) finishJobRun(jobID string) {
	if s == nil {
		return
	}
	s.jobLifecycleMu.Lock()
	delete(s.jobRuns, jobID)
	s.jobLifecycleMu.Unlock()
	s.jobRunWG.Done()
}

func (s *Server) waitForJobRuns(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan struct{})
	go func() {
		s.jobRunWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// waitForTrackedJobRuns is the restore-generation counterpart to
// waitForJobRuns. Restore quiescence seals the admission barrier but leaves
// ordinary job admission open for the generation's eventual resume, so the
// WaitGroup cannot be used as the only synchronization primitive here: a
// dispatcher may still be between its database claim and beginJobRun. The
// lifecycle map is checked after the barrier has drained and is therefore the
// authoritative join point before detaching the old generation's database.
func (s *Server) waitForTrackedJobRuns(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		s.jobLifecycleMu.Lock()
		active := len(s.jobRuns)
		s.jobLifecycleMu.Unlock()
		if active == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (s *Server) jobLeaseOwner(jobID string) string {
	prefix := "portico"
	if s != nil && strings.TrimSpace(s.jobWorkerID) != "" {
		prefix = strings.TrimSpace(s.jobWorkerID)
	}
	return prefix + "-" + strings.TrimSpace(jobID)
}

func (s *Server) jobWorkerPrefix() string {
	prefix := "portico"
	if s != nil && strings.TrimSpace(s.jobWorkerID) != "" {
		prefix = strings.TrimSpace(s.jobWorkerID)
	}
	return prefix + "-%"
}

func jobActiveKeyFor(jobType, resourceType, resourceID string, metadata map[string]string) string {
	if !jobTypeUsesActiveKey(jobType) {
		return ""
	}
	jobType = strings.TrimSpace(jobType)
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	if jobType == "" || resourceType == "" || resourceID == "" {
		return ""
	}
	parts := []string{jobType, resourceType, resourceID}
	switch jobType {
	case "media_analyze":
		parts = append(parts,
			"mode="+normalizeMediaAnalysisMode(metadata["analysisMode"]),
			"representativeFrame="+strings.ToLower(strings.TrimSpace(metadata["representativeFrame"])),
			"sourceRevision="+strings.TrimSpace(metadata["sourceRevision"]),
		)
	case "optimize_version":
		parts = append(parts, "profile="+strings.TrimSpace(metadata["profile"]))
	}
	key := strings.Join(parts, "|")
	if len(key) <= jobMaxActiveKeyBytes {
		return key
	}
	sum := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func jobTypeUsesActiveKey(jobType string) bool {
	switch strings.TrimSpace(jobType) {
	case "library_scan", "library_change_check", "library_read_model_repair",
		"metadata_refresh", "metadata_refresh_library", "lyrics_fetch_missing",
		"live_tv_refresh", "dvr_retention_cleanup", "tmdb_trending_refresh",
		"system_storage_cleanup", "library_trash_cleanup", "optimized_version_prune",
		"trickplay_prune", "database_backup", "media_analyze", "optimize_version",
		"dashboard_rollup_refresh":
		return true
	default:
		return false
	}
}

func isActiveJobUniqueConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") && strings.Contains(message, "active_key")
}

func (s *Server) hydrateJobEnvelope(job *Job) {
	if s == nil || job == nil || strings.TrimSpace(job.ID) == "" {
		return
	}
	var (
		parentOperationID, idempotencyKey, activeKey, priority, phase string
		progressCurrent, progressTotal                                int
		resultReference, errorCode                                    string
		retryEligible                                                 int
		cancellationRequestedAt, workerAcknowledgedAt                 string
		interruptedAt, retentionUntil                                 string
	)
	err := s.queryUserRow(context.Background(), `
		SELECT COALESCE(parent_operation_id, ''), COALESCE(idempotency_key, ''),
			COALESCE(active_key, ''), COALESCE(priority, 'normal'), COALESCE(phase, ''),
			COALESCE(progress_current, 0), COALESCE(progress_total, 0),
			COALESCE(result_reference, ''), COALESCE(error_code, ''),
			COALESCE(retry_eligible, 0), COALESCE(cancellation_requested_at, ''),
			COALESCE(worker_acknowledged_at, ''), COALESCE(interrupted_at, ''),
			COALESCE(retention_until, '')
		FROM jobs WHERE id = ?`, job.ID).Scan(
		&parentOperationID, &idempotencyKey, &activeKey, &priority, &phase,
		&progressCurrent, &progressTotal, &resultReference, &errorCode,
		&retryEligible, &cancellationRequestedAt, &workerAcknowledgedAt,
		&interruptedAt, &retentionUntil)
	if err != nil {
		return
	}
	job.ParentOperationID = parentOperationID
	job.IdempotencyKey = idempotencyKey
	job.ActiveKey = activeKey
	job.Priority = priority
	job.Phase = phase
	job.ProgressCurrent = progressCurrent
	job.ProgressTotal = progressTotal
	job.ResultReference = resultReference
	job.ErrorCode = errorCode
	job.RetryEligible = retryEligible != 0
	job.CancellationRequestedAt = cancellationRequestedAt
	job.WorkerAcknowledgedAt = workerAcknowledgedAt
	job.InterruptedAt = interruptedAt
	job.RetentionUntil = retentionUntil
}

func (s *Server) hydrateJobEnvelopeList(jobs []Job) {
	for index := range jobs {
		s.hydrateJobEnvelope(&jobs[index])
	}
}

func isTerminalJobStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "complete", "succeeded", "failed", "cancelled", "interrupted", "degraded":
		return true
	default:
		return false
	}
}

func (s *Server) jobRetentionDeadline(now time.Time) string {
	days := s.ownerRetentionSettings().JobHistoryDays
	if days <= 0 {
		return ""
	}
	return now.UTC().AddDate(0, 0, days).Format(time.RFC3339Nano)
}

func (s *Server) setJobFailure(jobID, code, message string) error {
	now := time.Now().UTC()
	code = sanitizeJobCode(code)
	message = s.sanitizeJobErrorMessage(message)
	ctx, cancel := boundedJobWriteContext()
	defer cancel()
	_, err := s.execBackgroundWrite(ctx, `
		UPDATE jobs
		SET status = 'failed', progress = 100, message = ?, last_error = ?,
			error_code = ?, retry_eligible = 1, phase = 'failed',
			leased_by = '', lease_expires_at = '', retention_until = ?, updated_at = ?
		WHERE id = ? AND status NOT IN ('cancelled', 'complete', 'succeeded')
			AND status = 'running' AND leased_by = ?
			AND cancellation_requested_at = ''`,
		message, message, strings.TrimSpace(code), s.jobRetentionDeadline(now), now.Format(time.RFC3339Nano), jobID, s.jobLeaseOwner(jobID))
	return err
}

func sanitizeJobCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if len(code) > 80 {
		code = code[:80]
	}
	if code == "" {
		return "job_failed"
	}
	for _, character := range code {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.' {
			continue
		}
		return "job_failed"
	}
	return code
}

func (s *Server) sanitizeJobErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "Job failed."
	}
	message = redactLogValue(message)
	if len(message) > 600 {
		message = message[:600]
	}
	return message
}

func (s *Server) terminalJobEnvelopeUpdate(jobID, status string, now time.Time) {
	if !isTerminalJobStatus(status) {
		return
	}
	retryEligible := 0
	if status == "failed" || status == "interrupted" {
		retryEligible = 1
	}
	errorCode := ""
	if status == "cancelled" {
		errorCode = "cancelled"
	}
	ctx, cancel := boundedJobWriteContext()
	defer cancel()
	_, _ = s.execBackgroundWrite(ctx, `
		UPDATE jobs
		SET phase = ?, progress_current = progress, retry_eligible = ?,
			error_code = CASE WHEN ? <> '' THEN ? ELSE error_code END,
			retention_until = CASE WHEN retention_until = '' THEN ? ELSE retention_until END,
			leased_by = '', lease_expires_at = '', updated_at = ?
		WHERE id = ?`,
		strings.ToLower(status), retryEligible, errorCode, errorCode,
		s.jobRetentionDeadline(now), now.UTC().Format(time.RFC3339Nano), jobID)
}

func (s *Server) recordInterruptedOwnedJobs(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.dbHandle() == nil {
		return nil
	}
	now := time.Now().UTC()
	_, err := s.execBackgroundWrite(ctx, `
		UPDATE jobs
		SET status = 'interrupted', phase = 'interrupted',
			progress_current = progress, retry_eligible = 1,
			interrupted_at = ?, error_code = 'shutdown_interrupted',
			last_error = CASE WHEN last_error = '' THEN 'Server shutdown interrupted this job.' ELSE last_error END,
			leased_by = '', lease_expires_at = '', retention_until = ?, updated_at = ?
		WHERE status = 'running' AND leased_by LIKE ?`,
		now.Format(time.RFC3339Nano), s.jobRetentionDeadline(now), now.Format(time.RFC3339Nano), s.jobWorkerPrefix())
	return err
}

func (s *Server) requeueOwnedJobsForRestore(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.dbHandle() == nil {
		return nil
	}
	now := time.Now().UTC()
	_, err := s.execBackgroundWrite(ctx, `
		UPDATE jobs
		SET status = 'queued', phase = 'queued',
			progress_current = progress, retry_eligible = 1,
			next_run_at = ?, error_code = 'restore_interrupted',
			last_error = CASE WHEN last_error = '' THEN 'Restore quiescence interrupted this job.' ELSE last_error END,
			leased_by = '', lease_expires_at = '', updated_at = ?
		WHERE status = 'running' AND leased_by LIKE ? AND cancellation_requested_at = ''`,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), s.jobWorkerPrefix())
	return err
}

func (s *Server) finalizeRequestedJobCancellation(jobID string) {
	if s == nil || strings.TrimSpace(jobID) == "" {
		return
	}
	now := time.Now().UTC()
	ctx, cancel := boundedJobWriteContext()
	defer cancel()
	_, _ = s.execBackgroundWrite(ctx, `
		UPDATE jobs
		SET status = 'cancelled', phase = 'cancelled', progress = 100,
			progress_current = 100, retry_eligible = 0,
			worker_acknowledged_at = ?, error_code = 'cancelled',
			message = CASE WHEN message = '' OR message = 'Job cancellation requested.'
				THEN 'Job cancelled after the worker acknowledged cancellation.' ELSE message END,
			leased_by = '', lease_expires_at = '',
			retention_until = CASE WHEN retention_until = '' THEN ? ELSE retention_until END,
			updated_at = ?
		WHERE id = ? AND leased_by = ? AND cancellation_requested_at <> ''
			AND status NOT IN ('cancelled', 'complete', 'succeeded')`,
		now.Format(time.RFC3339Nano), s.jobRetentionDeadline(now), now.Format(time.RFC3339Nano), jobID, s.jobLeaseOwner(jobID))
}

func (s *Server) activeJobClaimError(jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return sql.ErrNoRows
	}
	return fmt.Errorf("active job claim conflict for %s", jobID)
}
