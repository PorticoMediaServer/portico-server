package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	scannerBacklogMetadata = "metadata"
	scannerBacklogAnalysis = "analysis"

	scannerBacklogMetadataMaxIDs = 250
	scannerBacklogAnalysisBatch  = 100
)

type scannerBacklogRow struct {
	ID             string
	MediaID        string
	SourceRevision string
}

func scannerAnalysisSourceRevision(file scannerMediaFile) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\x00%d\x00%s", file.ID, file.FileID, filepath.Clean(file.SourcePath), file.QuickSignature, file.FileSize, file.FileModTime)
	return hex.EncodeToString(hash.Sum(nil))
}

func enqueueScannerBacklogTx(tx *sql.Tx, libraryID, mediaID, kind, sourceRevision, now string) (int, error) {
	libraryID = strings.TrimSpace(libraryID)
	mediaID = strings.TrimSpace(mediaID)
	sourceRevision = strings.TrimSpace(sourceRevision)
	if tx == nil || libraryID == "" || mediaID == "" || sourceRevision == "" {
		return 0, nil
	}
	if kind != scannerBacklogMetadata && kind != scannerBacklogAnalysis {
		return 0, fmt.Errorf("unsupported scanner backlog kind %q", kind)
	}
	result, err := tx.Exec(`
		INSERT INTO scanner_backlog (
			id, library_id, media_id, kind, source_revision, status,
			attempts, next_run_at, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'queued', 0, '', '', ?, ?)
		ON CONFLICT(kind, media_id, source_revision) DO NOTHING`,
		randomID("scanq"), libraryID, mediaID, kind, sourceRevision, now, now)
	if err != nil {
		return 0, err
	}
	return int(rowsAffected(result)), nil
}

func (s *Server) scannerAnalysisToolAvailable() bool {
	ffprobePath := strings.TrimSpace(s.cfg.FFprobePath)
	if ffprobePath == "" {
		return false
	}
	if filepath.Base(ffprobePath) != ffprobePath {
		return true
	}
	_, err := exec.LookPath(ffprobePath)
	return err == nil
}

// dispatchScannerBacklog atomically transfers a bounded amount of scanner
// discovery work into the durable job queue. Backlog rows become complete only
// in the same transaction that creates (or confirms) the owning job.
func (s *Server) dispatchScannerBacklog(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dispatched := 0
	_, err := s.withJobAdmission(func() (Job, error) {
		err := s.withBackgroundTxTagged(ctx, []string{"jobs", "scanner_backlog"}, func(tx *sql.Tx) error {
			metadataCount, err := s.dispatchScannerMetadataBacklogTx(tx)
			if err != nil {
				return err
			}
			dispatched += metadataCount
			if s.scannerAnalysisToolAvailable() {
				analysisCount, err := s.dispatchScannerAnalysisBacklogTx(tx)
				if err != nil {
					return err
				}
				dispatched += analysisCount
			}
			return nil
		})
		return Job{}, err
	})
	if errors.Is(err, errJobAdmissionClosed) {
		return 0, nil
	}
	if err == nil && dispatched > 0 {
		s.recordLog("info", fmt.Sprintf("Dispatched %d durable scanner backlog item%s.", dispatched, pluralSuffix(dispatched)), map[string]string{"items": strconv.Itoa(dispatched)})
	}
	return dispatched, err
}

func (s *Server) dispatchScannerMetadataBacklogTx(tx *sql.Tx) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var libraryID, libraryName string
	err := tx.QueryRow(`
		SELECT b.library_id, l.name
		FROM scanner_backlog b
		JOIN libraries l ON l.id = b.library_id
		WHERE b.kind = 'metadata' AND b.status = 'queued'
			AND (b.next_run_at = '' OR b.next_run_at <= ?)
		ORDER BY b.created_at, b.id
		LIMIT 1`, now).Scan(&libraryID, &libraryName)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	rows, err := tx.Query(`
		SELECT id, media_id
		FROM scanner_backlog
		WHERE library_id = ? AND kind = 'metadata' AND status = 'queued'
			AND (next_run_at = '' OR next_run_at <= ?)
		ORDER BY created_at, id
		LIMIT ?`, libraryID, now, scannerBacklogMetadataMaxIDs)
	if err != nil {
		return 0, err
	}
	candidates := make([]scannerBacklogRow, 0, scannerBacklogMetadataMaxIDs)
	for rows.Next() {
		var candidate scannerBacklogRow
		if err := rows.Scan(&candidate.ID, &candidate.MediaID); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	selected := make([]scannerBacklogRow, 0, len(candidates))
	mediaIDs := make([]string, 0, len(candidates))
	serializedBytes := 0
	for _, candidate := range candidates {
		mediaID := strings.TrimSpace(candidate.MediaID)
		if mediaID == "" || strings.Contains(mediaID, ",") {
			continue
		}
		additional := len(mediaID)
		if len(mediaIDs) > 0 {
			additional++
		}
		if serializedBytes+additional > jobMaxMetadataValueBytes {
			break
		}
		serializedBytes += additional
		mediaIDs = append(mediaIDs, mediaID)
		selected = append(selected, candidate)
	}
	if len(selected) == 0 {
		return 0, fmt.Errorf("scanner metadata backlog contains no dispatchable media identifiers")
	}
	metadata := normalizeJobMetadata(map[string]string{
		"libraryId":    libraryID,
		"libraryName":  libraryName,
		"limit":        strconv.Itoa(len(mediaIDs)),
		"mediaIds":     strings.Join(mediaIDs, ","),
		"subtaskScope": "scan_discoveries",
	})
	job := Job{
		ID: randomID("job"), Type: "metadata_refresh_library", Status: "queued",
		Message:      fmt.Sprintf("Metadata refresh queued for new items in %s.", libraryName),
		ResourceType: "library", ResourceID: libraryID, Metadata: metadata,
		Priority: "normal", Phase: "queued", CreatedAt: now, UpdatedAt: now,
	}
	job.ActiveKey = jobActiveKeyFor(job.Type, job.ResourceType, job.ResourceID, metadata)
	inserted, conflict, err := insertScannerBacklogJobTx(tx, job)
	if err != nil {
		return 0, err
	}
	if conflict || !inserted {
		return 0, nil
	}
	if err := completeScannerBacklogRowsTx(tx, selected, now); err != nil {
		return 0, err
	}
	return len(selected), nil
}

func (s *Server) dispatchScannerAnalysisBacklogTx(tx *sql.Tx) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := tx.Query(`
		SELECT b.id, b.media_id, b.source_revision, COALESCE(m.title, 'media item')
		FROM scanner_backlog b
		JOIN media_items m ON m.id = b.media_id
		WHERE b.kind = 'analysis' AND b.status = 'queued'
			AND (b.next_run_at = '' OR b.next_run_at <= ?)
		ORDER BY b.created_at, b.id
		LIMIT ?`, now, scannerBacklogAnalysisBatch)
	if err != nil {
		return 0, err
	}
	type analysisCandidate struct {
		scannerBacklogRow
		Title string
	}
	candidates := make([]analysisCandidate, 0, scannerBacklogAnalysisBatch)
	for rows.Next() {
		var candidate analysisCandidate
		if err := rows.Scan(&candidate.ID, &candidate.MediaID, &candidate.SourceRevision, &candidate.Title); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	completed := make([]scannerBacklogRow, 0, len(candidates))
	for _, candidate := range candidates {
		metadata := normalizeJobMetadata(representativeFrameAnalysisMetadata())
		metadata["sourceRevision"] = strings.TrimSpace(candidate.SourceRevision)
		job := Job{
			ID: randomID("job"), Type: "media_analyze", Status: "queued",
			Message:      "Media stream analysis queued for " + candidate.Title + ".",
			ResourceType: "media", ResourceID: candidate.MediaID, Metadata: metadata,
			Priority: "normal", Phase: "queued", CreatedAt: now, UpdatedAt: now,
		}
		job.ActiveKey = jobActiveKeyFor(job.Type, job.ResourceType, job.ResourceID, metadata)
		inserted, conflict, err := insertScannerBacklogJobTx(tx, job)
		if err != nil {
			return 0, err
		}
		if inserted {
			completed = append(completed, candidate.scannerBacklogRow)
		} else if conflict {
			matches, err := activeScannerAnalysisRevisionTx(tx, job.ActiveKey, candidate.SourceRevision)
			if err != nil {
				return 0, err
			}
			if matches {
				completed = append(completed, candidate.scannerBacklogRow)
			}
		}
	}
	if err := completeScannerBacklogRowsTx(tx, completed, now); err != nil {
		return 0, err
	}
	return len(completed), nil
}

// activeScannerAnalysisRevisionTx proves that an active analysis job owns the
// exact source bytes represented by a backlog row. An active job for an older
// revision must not consume a newer discovery: that row remains queued and is
// dispatched after the older job leaves the active set.
func activeScannerAnalysisRevisionTx(tx *sql.Tx, activeKey, sourceRevision string) (bool, error) {
	activeKey = strings.TrimSpace(activeKey)
	sourceRevision = strings.TrimSpace(sourceRevision)
	if tx == nil || activeKey == "" || sourceRevision == "" {
		return false, nil
	}
	var encoded string
	err := tx.QueryRow(`
		SELECT COALESCE(metadata_json, '{}')
		FROM jobs
		WHERE active_key = ? AND status IN ('queued', 'running')
		ORDER BY created_at, id
		LIMIT 1`, activeKey).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(decodeJobMetadata(encoded)["sourceRevision"]) == sourceRevision, nil
}

func insertScannerBacklogJobTx(tx *sql.Tx, job Job) (inserted bool, activeConflict bool, err error) {
	metadataJSON, err := json.Marshal(normalizeJobMetadata(job.Metadata))
	if err != nil {
		return false, false, err
	}
	_, err = tx.Exec(`
		INSERT INTO jobs (
			id, type, status, progress, message, resource_type, resource_id, metadata_json,
			active_key, priority, phase, progress_current, progress_total, created_at, updated_at
		) VALUES (?, ?, 'queued', 0, ?, ?, ?, ?, ?, 'normal', 'queued', 0, 0, ?, ?)`,
		job.ID, job.Type, job.Message, job.ResourceType, job.ResourceID, string(metadataJSON),
		job.ActiveKey, job.CreatedAt, job.UpdatedAt)
	if err == nil {
		return true, false, nil
	}
	if isActiveJobUniqueConflict(err) {
		return false, true, nil
	}
	return false, false, err
}

func completeScannerBacklogRowsTx(tx *sql.Tx, rows []scannerBacklogRow, now string) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	sort.Strings(ids)
	args := make([]any, 0, len(ids)+1)
	args = append(args, now)
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := tx.Exec(`
		UPDATE scanner_backlog
		SET status = 'complete', attempts = attempts + 1, last_error = '', updated_at = ?
		WHERE status = 'queued' AND id IN (`+sqlPlaceholders(len(ids))+`)`, args...)
	return err
}

func (s *Server) recoverScannerBacklogClaims() error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.execBackgroundWrite(context.Background(), `
		UPDATE scanner_backlog
		SET status = 'queued', next_run_at = '',
			last_error = CASE WHEN last_error = '' THEN 'Recovered interrupted scanner backlog dispatch.' ELSE last_error END,
			updated_at = ?
		WHERE status = 'claimed'`, now)
	return err
}
