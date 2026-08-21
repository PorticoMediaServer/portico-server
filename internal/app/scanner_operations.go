package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// LibraryScanRequest is the single owner-facing command envelope for library
// traversal. remove_missing is deliberately a scan mode (rather than a
// separate trash endpoint) so deletion can only follow fresh, authoritative
// absence evidence from the same operation.
type LibraryScanRequest struct {
	Mode           string `json:"mode"`
	ConfirmedRunID string `json:"confirmedRunId,omitempty"`
}

type LibraryScanRetryRequest struct {
	RunID string `json:"runId,omitempty"`
}

type StorageSourceClassificationRequest struct {
	Classification string `json:"classification"`
}

type LibraryScanWarning struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	SourceID string `json:"sourceId,omitempty"`
}

type LibraryScanPhase struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	State string `json:"state"`
}

type LibraryStorageSource struct {
	ID                   string `json:"id"`
	ConfiguredPath       string `json:"configuredPath"`
	ResolvedPath         string `json:"resolvedPath,omitempty"`
	Classification       string `json:"classification"`
	ClassificationSource string `json:"classificationSource"`
	Health               string `json:"health"`
	CircuitState         string `json:"circuitState"`
	ErrorClass           string `json:"errorClass,omitempty"`
	ErrorMessage         string `json:"errorMessage,omitempty"`
	LatencyMS            int64  `json:"latencyMs"`
	ConsecutiveFailures  int    `json:"consecutiveFailures"`
	LastProgressAt       string `json:"lastProgressAt,omitempty"`
	LastSuccessAt        string `json:"lastSuccessAt,omitempty"`
	LastFailureAt        string `json:"lastFailureAt,omitempty"`
	UpdatedAt            string `json:"updatedAt"`
}

type LibraryScanRootResult struct {
	SourceID        string `json:"sourceId"`
	ConfiguredPath  string `json:"configuredPath"`
	ResolvedPath    string `json:"resolvedPath,omitempty"`
	Status          string `json:"status"`
	ErrorClass      string `json:"errorClass,omitempty"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
	DirectoriesSeen int    `json:"directoriesSeen"`
	FilesSeen       int    `json:"filesSeen"`
	LastProgressAt  string `json:"lastProgressAt,omitempty"`
}

type LibraryMissingMediaReview struct {
	MediaID      string `json:"mediaId"`
	FileID       string `json:"fileId"`
	Title        string `json:"title"`
	Path         string `json:"path"`
	MissingSince string `json:"missingSince"`
	SourceID     string `json:"sourceId,omitempty"`
	SourceHealth string `json:"sourceHealth,omitempty"`
}

type LibraryScanReviewResponse struct {
	LibraryID         string                         `json:"libraryId"`
	ConfirmationRunID string                         `json:"confirmationRunId,omitempty"`
	CanConfirmRemoval bool                           `json:"canConfirmRemoval"`
	MissingItems      []LibraryMissingMediaReview    `json:"missingItems"`
	MissingTotal      int                            `json:"missingTotal"`
	IdentityReviews   []IdentityReconciliationReview `json:"identityReviews"`
	OpenIdentityTotal int                            `json:"openIdentityTotal"`
	Limit             int                            `json:"limit"`
	HasMore           bool                           `json:"hasMore"`
	NextCursor        string                         `json:"nextCursor,omitempty"`
	GeneratedAt       string                         `json:"generatedAt"`
}

type LibraryScanRunView struct {
	ID                   string                  `json:"id"`
	JobID                string                  `json:"jobId,omitempty"`
	Mode                 string                  `json:"mode"`
	Status               string                  `json:"status"`
	Phase                LibraryScanPhase        `json:"phase"`
	FilesIndexed         int                     `json:"filesIndexed"`
	FilesUnchanged       int                     `json:"filesUnchanged"`
	FilesSkipped         int                     `json:"filesSkipped"`
	MissingMarked        int                     `json:"missingMarked"`
	MetadataQueued       int                     `json:"metadataQueued"`
	AnalysisQueued       int                     `json:"analysisQueued"`
	AbsenceAuthoritative bool                    `json:"absenceAuthoritative"`
	CleanupAllowed       bool                    `json:"cleanupAllowed"`
	Warnings             []LibraryScanWarning    `json:"warnings"`
	Roots                []LibraryScanRootResult `json:"roots"`
	StartedAt            string                  `json:"startedAt"`
	CompletedAt          string                  `json:"completedAt,omitempty"`
	UpdatedAt            string                  `json:"updatedAt"`
}

type LibraryScanOperation struct {
	JobID                   string           `json:"jobId"`
	Status                  string           `json:"status"`
	Mode                    string           `json:"mode"`
	Trigger                 string           `json:"trigger"`
	Progress                int              `json:"progress"`
	Phase                   LibraryScanPhase `json:"phase"`
	Message                 string           `json:"message"`
	AttemptCount            int              `json:"attemptCount"`
	NextAttemptAt           string           `json:"nextAttemptAt,omitempty"`
	CancellationRequestedAt string           `json:"cancellationRequestedAt,omitempty"`
	CreatedAt               string           `json:"createdAt"`
	UpdatedAt               string           `json:"updatedAt"`
}

type LibraryScanActions struct {
	CanQuick         bool `json:"canQuick"`
	CanTarget        bool `json:"canTarget"`
	CanReconcile     bool `json:"canReconcile"`
	CanForceFull     bool `json:"canForceFull"`
	CanRemoveMissing bool `json:"canRemoveMissing"`
	CanCancel        bool `json:"canCancel"`
	CanRetry         bool `json:"canRetry"`
}

// LibraryScanOperationsResponse is intentionally one library-level
// projection. Management clients should render this operation rather than
// exposing the scanner's per-file metadata/analysis jobs as independent scan
// indicators.
type LibraryScanOperationsResponse struct {
	LibraryID       string                 `json:"libraryId"`
	Operation       *LibraryScanOperation  `json:"operation,omitempty"`
	LastRun         *LibraryScanRunView    `json:"lastRun,omitempty"`
	RecentRuns      []LibraryScanRunView   `json:"recentRuns"`
	Sources         []LibraryStorageSource `json:"sources"`
	Actions         LibraryScanActions     `json:"actions"`
	ScheduleEnabled bool                   `json:"scheduleEnabled"`
	LastRunAt       string                 `json:"lastRunAt,omitempty"`
	NextRunAt       string                 `json:"nextRunAt,omitempty"`
	GeneratedAt     string                 `json:"generatedAt"`
}

func scanPhase(code, status string) LibraryScanPhase {
	code = strings.TrimSpace(code)
	if code == "" {
		code = "queued"
	}
	label := strings.NewReplacer("_", " ", "-", " ").Replace(code)
	if label != "" {
		label = strings.ToUpper(label[:1]) + label[1:]
	}
	state := "pending"
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		state = "active"
	case "queued":
		state = "pending"
	case "healthy", "complete", "completed", "succeeded":
		state = "complete"
	case "degraded", "failed", "cancelled", "interrupted":
		state = "terminal"
	}
	return LibraryScanPhase{Code: code, Label: label, State: state}
}

func scanWarningCode(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "stale file handle"):
		return "stale_file_handle"
	case strings.Contains(lower, "timed out"), strings.Contains(lower, "timeout"):
		return "storage_timeout"
	case strings.Contains(lower, "permission"):
		return "storage_permission_denied"
	case strings.Contains(lower, "unavailable"), strings.Contains(lower, "offline"):
		return "storage_unavailable"
	case strings.Contains(lower, "cleanup"), strings.Contains(lower, "absence"):
		return "absence_not_authoritative"
	default:
		return "scan_warning"
	}
}

func decodeScanWarnings(raw string) []LibraryScanWarning {
	var messages []string
	_ = json.Unmarshal([]byte(raw), &messages)
	warnings := make([]LibraryScanWarning, 0, len(messages))
	for _, message := range messages {
		message = strings.TrimSpace(message)
		if message != "" {
			warnings = append(warnings, LibraryScanWarning{Code: scanWarningCode(message), Severity: "warning", Message: message})
		}
	}
	return warnings
}

func (s *Server) libraryScanRuns(ctx context.Context, libraryID string, limit int) ([]LibraryScanRunView, error) {
	limit = max(1, min(50, limit))
	rows, err := s.queryUserRead(ctx, `
		SELECT id, job_id, mode, status, phase, files_indexed, files_unchanged,
			files_skipped, missing_marked, metadata_queued, analysis_queued,
			absence_authoritative, cleanup_allowed, warnings_json, started_at,
			completed_at, updated_at
		FROM library_scan_runs WHERE library_id = ?
		ORDER BY started_at DESC, id DESC LIMIT ?`, libraryID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]LibraryScanRunView, 0, limit)
	for rows.Next() {
		var run LibraryScanRunView
		var phase, warningsJSON string
		var authoritative, cleanup int
		if err := rows.Scan(&run.ID, &run.JobID, &run.Mode, &run.Status, &phase,
			&run.FilesIndexed, &run.FilesUnchanged, &run.FilesSkipped, &run.MissingMarked,
			&run.MetadataQueued, &run.AnalysisQueued, &authoritative, &cleanup,
			&warningsJSON, &run.StartedAt, &run.CompletedAt, &run.UpdatedAt); err != nil {
			return nil, err
		}
		run.AbsenceAuthoritative = authoritative == 1
		run.CleanupAllowed = cleanup == 1
		run.Phase = scanPhase(phase, run.Status)
		run.Warnings = decodeScanWarnings(warningsJSON)
		run.Roots = []LibraryScanRootResult{}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range runs {
		rootRows, err := s.queryUserRead(ctx, `
			SELECT source_id, configured_path, resolved_path, status, error_class,
				error_message, directories_seen, files_seen, last_progress_at
			FROM library_scan_run_roots WHERE run_id = ?
			ORDER BY configured_path, source_id`, runs[index].ID)
		if err != nil {
			return nil, err
		}
		for rootRows.Next() {
			var root LibraryScanRootResult
			if err := rootRows.Scan(&root.SourceID, &root.ConfiguredPath, &root.ResolvedPath,
				&root.Status, &root.ErrorClass, &root.ErrorMessage, &root.DirectoriesSeen,
				&root.FilesSeen, &root.LastProgressAt); err != nil {
				rootRows.Close()
				return nil, err
			}
			runs[index].Roots = append(runs[index].Roots, root)
			if root.ErrorMessage != "" {
				runs[index].Warnings = append(runs[index].Warnings, LibraryScanWarning{
					Code: firstNonEmpty(root.ErrorClass, scanWarningCode(root.ErrorMessage)), Severity: "warning",
					Message: root.ErrorMessage, SourceID: root.SourceID,
				})
			}
		}
		if err := rootRows.Close(); err != nil {
			return nil, err
		}
	}
	return runs, nil
}

func (s *Server) libraryStorageSources(ctx context.Context, libraryID string) ([]LibraryStorageSource, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT id, configured_path, resolved_path, classification, classification_source,
			health_state, circuit_state, error_class, error_message, latency_ms,
			consecutive_failures, last_progress_at, last_success_at, last_failure_at, updated_at
		FROM storage_sources WHERE library_id = ? ORDER BY configured_path, id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []LibraryStorageSource{}
	for rows.Next() {
		var source LibraryStorageSource
		if err := rows.Scan(&source.ID, &source.ConfiguredPath, &source.ResolvedPath,
			&source.Classification, &source.ClassificationSource, &source.Health,
			&source.CircuitState, &source.ErrorClass, &source.ErrorMessage, &source.LatencyMS,
			&source.ConsecutiveFailures, &source.LastProgressAt, &source.LastSuccessAt,
			&source.LastFailureAt, &source.UpdatedAt); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (s *Server) libraryScanReview(ctx context.Context, libraryID string, limit int, cursorMissingSince, cursorFileID string) (LibraryScanReviewResponse, error) {
	limit = max(1, min(100, limit))
	args := []any{libraryID}
	cursorWhere := ""
	if cursorMissingSince != "" && cursorFileID != "" {
		cursorWhere = ` AND (f.missing_since < ? OR (f.missing_since = ? AND f.id < ?))`
		args = append(args, cursorMissingSince, cursorMissingSince, cursorFileID)
	}
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT f.media_id, f.id, m.title, f.path, f.missing_since
		FROM media_files f
		JOIN media_items m ON m.id = f.media_id
		WHERE f.library_id = ? AND f.available = 0 AND f.missing_since <> ''`+cursorWhere+`
		ORDER BY f.missing_since DESC, f.id DESC LIMIT ?`, args...)
	if err != nil {
		return LibraryScanReviewResponse{}, err
	}
	items := make([]LibraryMissingMediaReview, 0, limit+1)
	for rows.Next() {
		var item LibraryMissingMediaReview
		if err := rows.Scan(&item.MediaID, &item.FileID, &item.Title, &item.Path, &item.MissingSince); err != nil {
			rows.Close()
			return LibraryScanReviewResponse{}, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return LibraryScanReviewResponse{}, err
	}
	var missingTotal int
	if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM media_files WHERE library_id = ? AND available = 0 AND missing_since <> ''`, libraryID).Scan(&missingTotal); err != nil {
		return LibraryScanReviewResponse{}, err
	}
	sources, err := s.libraryStorageSources(ctx, libraryID)
	if err != nil {
		return LibraryScanReviewResponse{}, err
	}
	for index := range items {
		bestLength := -1
		for _, source := range sources {
			root := firstNonEmpty(source.ResolvedPath, source.ConfiguredPath)
			if pathWithinRoot(items[index].Path, root) && len(root) > bestLength {
				items[index].SourceID = source.ID
				items[index].SourceHealth = source.Health
				bestLength = len(root)
			}
		}
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	nextCursor := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeTimeIDCursor(last.MissingSince, last.FileID)
	}

	identityRows, err := s.queryUserRead(ctx, `
		SELECT id, domain, library_or_source_id, subject_id, candidate_locator, evidence_kind,
			evidence_value, candidate_ids_json, status, created_at, resolved_at, resolution,
			selected_candidate_id, resolved_by_user_id, resolution_note
		FROM identity_reconciliation_reviews
		WHERE library_or_source_id = ? AND status = 'open'
		ORDER BY created_at DESC, id DESC LIMIT 50`, libraryID)
	if err != nil {
		return LibraryScanReviewResponse{}, err
	}
	identityReviews := []IdentityReconciliationReview{}
	for identityRows.Next() {
		review, err := scanIdentityReconciliationReview(identityRows)
		if err != nil {
			identityRows.Close()
			return LibraryScanReviewResponse{}, err
		}
		identityReviews = append(identityReviews, review)
	}
	if err := identityRows.Close(); err != nil {
		return LibraryScanReviewResponse{}, err
	}
	var identityTotal int
	if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM identity_reconciliation_reviews WHERE library_or_source_id = ? AND status = 'open'`, libraryID).Scan(&identityTotal); err != nil {
		return LibraryScanReviewResponse{}, err
	}

	confirmationRunID, canConfirm, err := s.latestScanRemovalConfirmation(ctx, libraryID)
	if err != nil {
		return LibraryScanReviewResponse{}, err
	}
	return LibraryScanReviewResponse{
		LibraryID: libraryID, ConfirmationRunID: confirmationRunID, CanConfirmRemoval: canConfirm,
		MissingItems: items, MissingTotal: missingTotal, IdentityReviews: identityReviews,
		OpenIdentityTotal: identityTotal, Limit: limit, HasMore: hasMore, NextCursor: nextCursor,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Server) activeLibraryScanOperation(ctx context.Context, libraryID string) (*LibraryScanOperation, error) {
	var job Job
	var metadataJSON string
	var phase string
	err := s.queryUserRow(ctx, `
		SELECT id, status, progress, message, metadata_json, phase, attempt_count, next_run_at,
			cancellation_requested_at, created_at, updated_at
		FROM jobs WHERE type = 'library_scan' AND resource_type = 'library'
			AND resource_id = ? AND status IN ('running', 'queued')
		ORDER BY CASE status WHEN 'running' THEN 0 ELSE 1 END, created_at, id LIMIT 1`, libraryID).
		Scan(&job.ID, &job.Status, &job.Progress, &job.Message, &metadataJSON, &phase, &job.AttemptCount,
			&job.NextRunAt, &job.CancellationRequestedAt, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	job.Metadata = decodeJobMetadata(metadataJSON)
	phaseCode := firstNonEmpty(phase, job.Status)
	if job.CancellationRequestedAt != "" {
		phaseCode = "cancelling"
	}
	return &LibraryScanOperation{
		JobID: job.ID, Status: job.Status, Mode: normalizeScanMode(job.Metadata["mode"]),
		Trigger: firstNonEmpty(job.Metadata[scanTriggerMetadataKey], "unknown"), Progress: job.Progress,
		Phase: scanPhase(phaseCode, job.Status), Message: job.Message, AttemptCount: job.AttemptCount,
		NextAttemptAt: job.NextRunAt, CancellationRequestedAt: job.CancellationRequestedAt,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}, nil
}

func (s *Server) libraryScanSchedule(library Library, operation *LibraryScanOperation) (bool, string, string, error) {
	settings := s.scheduledTaskSettings()
	enabled := settings.Enabled && settings.globallyEnabledTask("library-scan", settings.ScanLibraries) &&
		settingBool(library.Settings, "scannerEnabled", true) && s.libraryRuntimeSettingsFor(library).ScanAutomatically
	var lastAt string
	err := s.queryUserRow(context.Background(), `
		SELECT COALESCE(MAX(created_at), '') FROM jobs
		WHERE type = 'library_scan' AND resource_type = 'library' AND resource_id = ?`, library.ID).Scan(&lastAt)
	if err != nil {
		return false, "", "", err
	}
	if !enabled {
		return false, lastAt, "", nil
	}
	if operation != nil && operation.Status == "queued" && operation.NextAttemptAt != "" {
		return true, lastAt, operation.NextAttemptAt, nil
	}
	last, err := time.Parse(time.RFC3339, lastAt)
	if err != nil {
		last, err = time.Parse(time.RFC3339Nano, lastAt)
	}
	if err != nil {
		return true, lastAt, time.Now().UTC().Format(time.RFC3339), nil
	}
	globalHours := settings.taskIntervalHours("library-scan", settings.LibraryScanCadence, settings.LibraryScanIntervalHours)
	next := last.Add(time.Duration(libraryScheduledScanIntervalHours(library, globalHours)) * time.Hour)
	return true, lastAt, next.UTC().Format(time.RFC3339), nil
}

func (s *Server) libraryScanOperations(ctx context.Context, library Library) (LibraryScanOperationsResponse, error) {
	operation, err := s.activeLibraryScanOperation(ctx, library.ID)
	if err != nil {
		return LibraryScanOperationsResponse{}, err
	}
	runs, err := s.libraryScanRuns(ctx, library.ID, 10)
	if err != nil {
		return LibraryScanOperationsResponse{}, err
	}
	sources, err := s.libraryStorageSources(ctx, library.ID)
	if err != nil {
		return LibraryScanOperationsResponse{}, err
	}
	enabled, lastAt, nextAt, err := s.libraryScanSchedule(library, operation)
	if err != nil {
		return LibraryScanOperationsResponse{}, err
	}
	var lastRun *LibraryScanRunView
	if len(runs) > 0 {
		copy := runs[0]
		lastRun = &copy
		lastAt = copy.StartedAt
	}
	_, canRemoveMissing, err := s.latestScanRemovalConfirmation(ctx, library.ID)
	if err != nil {
		return LibraryScanOperationsResponse{}, err
	}
	retry := lastRun != nil && (lastRun.Status == "failed" || lastRun.Status == "cancelled" || lastRun.Status == "degraded")
	return LibraryScanOperationsResponse{
		LibraryID: library.ID, Operation: operation, LastRun: lastRun, RecentRuns: runs, Sources: sources,
		Actions: LibraryScanActions{CanQuick: true, CanTarget: true, CanReconcile: true, CanForceFull: true,
			CanRemoveMissing: canRemoveMissing, CanCancel: operation != nil, CanRetry: operation == nil && retry},
		ScheduleEnabled: enabled, LastRunAt: lastAt, NextRunAt: nextAt, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func validStorageClassification(value string) bool {
	switch value {
	case "local", "network", "fuse", "unknown":
		return true
	default:
		return false
	}
}

func (s *Server) setStorageSourceClassification(ctx context.Context, libraryID, sourceID, classification string) (LibraryStorageSource, error) {
	classification = strings.ToLower(strings.TrimSpace(classification))
	if !validStorageClassification(classification) {
		return LibraryStorageSource{}, fmt.Errorf("classification must be local, network, fuse, or unknown")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.execUserWrite(ctx, `
		UPDATE storage_sources SET classification = ?, classification_source = 'owner', updated_at = ?
		WHERE id = ? AND library_id = ?`, classification, now, sourceID, libraryID)
	if err != nil {
		return LibraryStorageSource{}, err
	}
	if rowsAffected(result) != 1 {
		return LibraryStorageSource{}, sql.ErrNoRows
	}
	sources, err := s.libraryStorageSources(ctx, libraryID)
	if err != nil {
		return LibraryStorageSource{}, err
	}
	for _, source := range sources {
		if source.ID == sourceID {
			return source, nil
		}
	}
	return LibraryStorageSource{}, sql.ErrNoRows
}

func (s *Server) cancelLibraryScan(ctx context.Context, libraryID string) (Job, error) {
	var jobID string
	err := s.queryUserRow(ctx, `
		SELECT id FROM jobs WHERE type = 'library_scan' AND resource_type = 'library'
			AND resource_id = ? AND status IN ('running', 'queued')
		ORDER BY CASE status WHEN 'running' THEN 0 ELSE 1 END, created_at, id LIMIT 1`, libraryID).Scan(&jobID)
	if err != nil {
		return Job{}, err
	}
	return s.cancelJob(jobID)
}

func (s *Server) validateRemoveMissingConfirmation(ctx context.Context, libraryID, runID string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return errors.New("confirmedRunId is required after reviewing the latest reconciliation evidence")
	}
	latestID, valid, err := s.latestScanRemovalConfirmation(ctx, libraryID)
	if err != nil {
		return err
	}
	if latestID != runID {
		return errors.New("confirmedRunId must identify the latest completed scan run")
	}
	if !valid {
		return errors.New("the confirmed scan run does not contain healthy, authoritative absence evidence")
	}
	return nil
}

func (s *Server) latestScanRemovalConfirmation(ctx context.Context, libraryID string) (string, bool, error) {
	var runID, status string
	var authoritative, cleanup int
	err := s.queryUserRow(ctx, `
		SELECT id, status, absence_authoritative, cleanup_allowed
		FROM library_scan_runs
		WHERE library_id = ? AND status <> 'running'
		ORDER BY started_at DESC, id DESC LIMIT 1`, libraryID).
		Scan(&runID, &status, &authoritative, &cleanup)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var openIdentityReviews int
	if err := s.queryUserRow(ctx, `
		SELECT COUNT(*) FROM identity_reconciliation_reviews
		WHERE library_or_source_id = ? AND status = 'open'`, libraryID).Scan(&openIdentityReviews); err != nil {
		return "", false, err
	}
	return runID, status == "healthy" && authoritative == 1 && cleanup == 1 && openIdentityReviews == 0, nil
}

func (s *Server) retryLibraryScan(ctx context.Context, library Library, runID string) (Job, error) {
	query := `SELECT mode, status FROM library_scan_runs WHERE library_id = ?`
	args := []any{library.ID}
	if strings.TrimSpace(runID) != "" {
		query += ` AND id = ?`
		args = append(args, strings.TrimSpace(runID))
	} else {
		query += ` AND status IN ('failed', 'cancelled', 'degraded') ORDER BY started_at DESC, id DESC LIMIT 1`
	}
	var mode, status string
	if err := s.queryUserRow(ctx, query, args...).Scan(&mode, &status); err != nil {
		return Job{}, err
	}
	if status != "failed" && status != "cancelled" && status != "degraded" {
		return Job{}, fmt.Errorf("only failed, cancelled, or degraded scan runs can be retried")
	}
	return s.queueLibraryScan(library, mode, "retry", fmt.Sprintf("Retry scan queued for %s.", library.Name))
}

// handleLibraryScanOperations handles only the scanner management suffixes;
// returning false leaves unrelated library routes with their existing owner.
func (s *Server) handleLibraryScanOperations(w http.ResponseWriter, r *http.Request, user User, libraryID string, parts []string) bool {
	if len(parts) < 2 || (parts[1] != "scan" && parts[1] != "scan-operations" && parts[1] != "scan-review" && parts[1] != "scan-runs" && parts[1] != "storage-sources") {
		return false
	}
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "Only the server owner can manage library scans and storage sources.")
		return true
	}
	library, err := s.getLibraryContext(r.Context(), libraryID)
	if err != nil {
		writeError(w, http.StatusNotFound, "library_not_found", "Library was not found.")
		return true
	}
	if parts[1] == "scan-operations" && len(parts) == 2 && r.Method == http.MethodGet {
		response, err := s.libraryScanOperations(r.Context(), library)
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "scan_operations_failed", "Unable to load library scan operations.")
			return true
		}
		writeJSON(w, http.StatusOK, response)
		return true
	}
	if parts[1] == "scan-runs" && len(parts) == 2 && r.Method == http.MethodGet {
		limit := clampInt(queryInt(r, "limit", 25), 1, 50)
		runs, err := s.libraryScanRuns(r.Context(), library.ID, limit)
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "scan_runs_failed", "Unable to load library scan history.")
			return true
		}
		writeJSON(w, http.StatusOK, ListResponse[LibraryScanRunView]{Items: runs, Total: len(runs), Limit: limit})
		return true
	}
	if parts[1] == "scan-review" && len(parts) == 2 && r.Method == http.MethodGet {
		limit := clampInt(queryInt(r, "limit", 50), 1, 100)
		cursorTime, cursorID, err := decodeTimeIDCursor(strings.TrimSpace(r.URL.Query().Get("cursor")))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "Scan review cursor is invalid.")
			return true
		}
		review, err := s.libraryScanReview(r.Context(), library.ID, limit, cursorTime, cursorID)
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "scan_review_failed", "Unable to load scan reconciliation review.")
			return true
		}
		writeJSON(w, http.StatusOK, review)
		return true
	}
	if parts[1] == "storage-sources" && len(parts) == 2 && r.Method == http.MethodGet {
		sources, err := s.libraryStorageSources(r.Context(), library.ID)
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "storage_sources_failed", "Unable to load library storage sources.")
			return true
		}
		writeJSON(w, http.StatusOK, ListResponse[LibraryStorageSource]{Items: sources, Total: len(sources), Limit: len(sources)})
		return true
	}
	if parts[1] == "scan" && len(parts) == 3 && parts[2] == "cancel" && r.Method == http.MethodPost {
		job, err := s.cancelLibraryScan(r.Context(), library.ID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusConflict, "scan_not_active", "This library does not have an active scan.")
			return true
		}
		if err != nil {
			writeError(w, http.StatusConflict, "scan_cancel_failed", err.Error())
			return true
		}
		s.recordAudit(r, user, "library.scan_cancelled", "library", library.ID, "warn", map[string]string{"job": job.ID})
		writeJSON(w, http.StatusOK, job)
		return true
	}
	if parts[1] == "scan" && len(parts) == 3 && parts[2] == "retry" && r.Method == http.MethodPost {
		var req LibraryScanRetryRequest
		if r.ContentLength > 0 && !decodeJSON(w, r, &req) {
			return true
		}
		job, err := s.retryLibraryScan(r.Context(), library, req.RunID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "scan_run_not_found", "A retryable scan run was not found.")
			return true
		}
		if err != nil {
			writeError(w, http.StatusConflict, "scan_retry_failed", err.Error())
			return true
		}
		s.recordAudit(r, user, "library.scan_retried", "library", library.ID, "info", map[string]string{"job": job.ID, "run": req.RunID})
		writeJSON(w, http.StatusCreated, job)
		return true
	}
	if parts[1] == "storage-sources" && len(parts) == 3 && r.Method == http.MethodPatch {
		var req StorageSourceClassificationRequest
		if !decodeJSON(w, r, &req) {
			return true
		}
		source, err := s.setStorageSourceClassification(r.Context(), library.ID, parts[2], req.Classification)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "storage_source_not_found", "Storage source was not found.")
			return true
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "storage_classification_invalid", err.Error())
			return true
		}
		s.recordAudit(r, user, "library.storage_classification_updated", "storage_source", source.ID, "info", map[string]string{"classification": source.Classification})
		writeJSON(w, http.StatusOK, source)
		return true
	}
	return false
}
