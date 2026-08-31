package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

const scanTriggerMetadataKey = "trigger"

func scanModeRank(mode string) int {
	switch normalizeScanMode(mode) {
	case "targeted":
		return 1
	case "quick":
		return 2
	case "reconcile":
		return 3
	case "force_full":
		return 4
	case "remove_missing":
		return 5
	default:
		return 3
	}
}

func strongerScanMode(left, right string) string {
	left = normalizeScanMode(left)
	right = normalizeScanMode(right)
	if scanModeRank(right) > scanModeRank(left) {
		return right
	}
	return left
}

// mergeLibraryScanMetadata preserves one active library operation while
// allowing a later, stronger request to promote the work that operation must
// perform. The active worker re-reads this value before it completes.
func mergeLibraryScanMetadata(existing, requested map[string]string) map[string]string {
	merged := make(map[string]string, len(existing)+len(requested))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range requested {
		if key != "mode" {
			merged[key] = value
		}
	}
	merged["mode"] = strongerScanMode(existing["mode"], requested["mode"])
	return normalizeJobMetadata(merged)
}

func (s *Server) queueLibraryScan(library Library, mode, trigger, message string) (Job, error) {
	mode = normalizeScanMode(mode)
	trigger = strings.ToLower(strings.TrimSpace(trigger))
	if trigger == "" {
		trigger = "manual"
	}
	if strings.TrimSpace(message) == "" {
		message = fmt.Sprintf("Scan queued for %s.", library.Name)
	}
	return s.createJobForWithMetadata("library_scan", message, "library", library.ID, map[string]string{
		"mode":                 mode,
		scanTriggerMetadataKey: trigger,
	})
}

// completeLibraryScanOrContinue serializes with job enqueue/coalescing. It
// either claims a stronger mode that arrived while the scan was running, or
// completes the operation. A request arriving after completion creates a new
// operation, so no force-full request can be lost at the completion boundary.
func (s *Server) completeLibraryScanOrContinue(jobID, completedMode, message string) (string, bool, error) {
	if strings.TrimSpace(jobID) == "" {
		return "", true, nil
	}
	durableJobEnqueueMu.Lock()
	defer durableJobEnqueueMu.Unlock()

	nextMode := ""
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339)
	leaseOwner := s.jobLeaseOwner(jobID)
	err := s.withWorkClassTxTaggedForViewer(context.Background(), foundationcontract.WorkClassBackgroundMedia, "library_scan_complete", durableJobEnqueueRetry, "", "", nil, func(tx *sql.Tx) error {
		var raw, status, leasedBy, cancellationRequestedAt string
		if err := tx.QueryRowContext(context.Background(), `
			SELECT COALESCE(metadata_json, '{}'), status, COALESCE(leased_by, ''), COALESCE(cancellation_requested_at, '')
			FROM jobs WHERE id = ?`, jobID).Scan(&raw, &status, &leasedBy, &cancellationRequestedAt); err != nil {
			return err
		}
		if status != "running" || leasedBy != leaseOwner || cancellationRequestedAt != "" {
			return fmt.Errorf("library scan is no longer owned by this worker")
		}
		metadata := decodeJobMetadata(raw)
		requestedMode := normalizeScanMode(metadata["mode"])
		if scanModeRank(requestedMode) > scanModeRank(completedMode) {
			nextMode = requestedMode
			metadata["mode"] = requestedMode
			metadata["continuationMode"] = requestedMode
			encoded, err := json.Marshal(normalizeJobMetadata(metadata))
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(context.Background(), `
				UPDATE jobs
				SET metadata_json = ?, phase = 'running', progress = 5,
					message = ?, updated_at = ?
				WHERE id = ? AND status = 'running' AND leased_by = ? AND cancellation_requested_at = ''`, string(encoded), fmt.Sprintf("Continuing with %s scan.", strings.ReplaceAll(requestedMode, "_", " ")), now, jobID, leaseOwner)
			return err
		}
		_, err := tx.ExecContext(context.Background(), `
			UPDATE jobs SET status = 'complete', progress = 100, phase = 'complete',
				progress_current = 100, message = ?, retry_eligible = 0,
				retention_until = CASE WHEN retention_until = '' THEN ? ELSE retention_until END,
				leased_by = '', lease_expires_at = '', updated_at = ?
			WHERE id = ? AND status = 'running' AND leased_by = ? AND cancellation_requested_at = ''`, message, s.jobRetentionDeadline(nowTime), now, jobID, leaseOwner)
		return err
	})
	if err == nil && nextMode == "" {
		s.signalJobWake()
	}
	return nextMode, nextMode == "", err
}
