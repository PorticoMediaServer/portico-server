package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

type storageSourceClass string

const (
	storageSourceLocal   storageSourceClass = "local"
	storageSourceNetwork storageSourceClass = "network"
	storageSourceFUSE    storageSourceClass = "fuse"
	storageSourceUnknown storageSourceClass = "unknown"
)

type scanRootEvidence struct {
	SourceID             string
	ConfiguredPath       string
	ResolvedPath         string
	Classification       storageSourceClass
	ClassificationSource string
	Health               string
	ErrorClass           string
	ErrorMessage         string
	Latency              time.Duration
	CircuitHeld          bool
}

type libraryScanRun struct {
	ID        string
	LibraryID string
	JobID     string
	Mode      string
	StartedAt string
}

var errScannerRootStalled = errors.New("storage source made no filesystem progress")

var scannerRootIdleTimeout = func(classification storageSourceClass) time.Duration {
	switch classification {
	case storageSourceNetwork:
		return 45 * time.Second
	case storageSourceFUSE:
		return 60 * time.Second
	case storageSourceUnknown:
		return 90 * time.Second
	default:
		return 5 * time.Minute
	}
}

var scannerStorageCircuitBackoff = 30 * time.Second

func storageCircuitBackoff(classification storageSourceClass, failures int) time.Duration {
	base := scannerStorageCircuitBackoff
	switch classification {
	case storageSourceNetwork:
		base *= 4
	case storageSourceFUSE:
		base *= 6
	case storageSourceUnknown:
		base *= 8
	}
	for step := 1; step < failures && step < 6; step++ {
		base *= 2
	}
	if base > 30*time.Minute {
		return 30 * time.Minute
	}
	return base
}

var scannerProbeRootPath = func(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("configured library root is not a directory")
	}
	return resolved, nil
}

var scannerRootProbeTimeout = func(classification storageSourceClass) time.Duration {
	switch classification {
	case storageSourceNetwork:
		return 30 * time.Second
	case storageSourceFUSE:
		return 45 * time.Second
	case storageSourceUnknown:
		return 30 * time.Second
	default:
		return 15 * time.Second
	}
}

func (s *Server) walkScannerRoot(ctx context.Context, root scanRoot, handle fs.WalkDirFunc) error {
	idle := scannerRootIdleTimeout(root.classification)
	if idle <= 0 {
		idle = 30 * time.Second
	}
	request := storageRequestForRoot(root, "scanner root traversal")
	// Traversal owns a physical-source walker lane rather than the ordinary
	// source lane. This bounds hidden WalkDir syscalls across libraries on the
	// same mount while allowing callback work and later foreground consumers to
	// use the normal source admission policy.
	request.AdmissionKey = "walk:" + request.AdmissionKey
	request.AdmissionLimit = 1
	if root.classification == storageSourceLocal {
		request.AdmissionLimit = 4
	}
	request.Timeout = idle
	return s.boundedStorageProgressIO(ctx, request, func(progress func()) error {
		return scannerWalkDir(root.real, func(path string, entry fs.DirEntry, walkErr error) error {
			progress()
			return handle(path, entry, walkErr)
		})
	})
}

func normalizeScanMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "targeted", "quick", "reconcile", "force_full", "remove_missing":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "reconcile"
	}
}

func storageSourceID(libraryID, configuredPath string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(libraryID) + "\x00" + filepath.Clean(strings.TrimSpace(configuredPath))))
	return "storage_" + hex.EncodeToString(sum[:16])
}

func classifyStorageSource(path, ownerOverride string) (storageSourceClass, string) {
	switch strings.ToLower(strings.TrimSpace(ownerOverride)) {
	case string(storageSourceLocal):
		return storageSourceLocal, "owner"
	case string(storageSourceNetwork):
		return storageSourceNetwork, "owner"
	case string(storageSourceFUSE):
		return storageSourceFUSE, "owner"
	case string(storageSourceUnknown):
		return storageSourceUnknown, "owner"
	}
	clean := filepath.Clean(strings.TrimSpace(path))
	lower := strings.ToLower(filepath.ToSlash(clean))
	if strings.HasPrefix(clean, `\\`) || strings.HasPrefix(lower, "//") {
		return storageSourceNetwork, "detected"
	}
	for _, marker := range []string{"/net/", "/nfs/", "/smb/", "/cifs/"} {
		if strings.Contains(lower+"/", marker) {
			return storageSourceNetwork, "detected"
		}
	}
	for _, marker := range []string{"rclone", "fuse", "sshfs"} {
		if strings.Contains(lower, marker) {
			return storageSourceFUSE, "detected"
		}
	}
	if runtime.GOOS == "windows" && filepath.VolumeName(clean) == "" {
		return storageSourceUnknown, "detected"
	}
	return storageSourceLocal, "detected"
}

func storageErrorClass(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, errScannerRootStalled), errors.Is(err, errStorageIOStalled), errors.Is(err, errStorageIOBusy):
		return "stalled"
	case errors.Is(err, errStorageIOCapacity):
		return "capacity"
	case errors.Is(err, errStorageCircuitOpen):
		return "circuit_open"
	case errors.Is(err, context.DeadlineExceeded):
		return "stalled"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, os.ErrNotExist):
		return "unavailable"
	case errors.Is(err, os.ErrPermission):
		return "permission"
	}
	if class, _ := storageErrnoClass(err); class != "" {
		return class
	}
	lower := strings.ToLower(err.Error())
	for _, token := range []string{"stale file handle", "estale"} {
		if strings.Contains(lower, token) {
			return "stale_handle"
		}
	}
	for _, token := range []string{"input/output error", "i/o error", "eio"} {
		if strings.Contains(lower, token) {
			return "io"
		}
	}
	return "filesystem"
}

func preliminaryLibraryRootEvidence(library Library) []scanRootEvidence {
	override := settingString(library.Settings, "storageClass", "")
	evidence := make([]scanRootEvidence, 0, len(library.Paths))
	for _, raw := range library.Paths {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		classification, source := classifyStorageSource(raw, override)
		item := scanRootEvidence{
			SourceID: storageSourceID(library.ID, raw), ConfiguredPath: raw,
			Classification: classification, ClassificationSource: source, Health: "healthy",
		}
		abs, err := filepath.Abs(raw)
		if err != nil {
			item.Health = "offline"
			item.ErrorClass = storageErrorClass(err)
			item.ErrorMessage = boundedStorageError(err)
		} else {
			item.ConfiguredPath = filepath.Clean(abs)
			item.SourceID = storageSourceID(library.ID, item.ConfiguredPath)
			item.Classification, item.ClassificationSource = classifyStorageSource(item.ConfiguredPath, override)
		}
		evidence = append(evidence, item)
	}
	return evidence
}

func (s *Server) probeLibraryRoot(ctx context.Context, item scanRootEvidence) scanRootEvidence {
	started := time.Now()
	timeout := scannerRootProbeTimeout(item.Classification)
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	request := storageIORequest{
		WorkClass: foundationcontract.WorkClassBackgroundMedia,
		SourceID:  item.SourceID, AdmissionKey: storagePhysicalSourceKey(item.ConfiguredPath, item.Classification),
		Classification: item.Classification, Operation: "scanner root admission", Timeout: timeout,
	}
	var resolved string
	probeErr := s.boundedStorageIO(ctx, request, func() error {
		var err error
		resolved, err = scannerProbeRootPath(item.ConfiguredPath)
		return err
	})
	item.Latency = time.Since(started)
	if probeErr != nil {
		item.Health = "offline"
		if errors.Is(probeErr, errScannerRootStalled) || errors.Is(probeErr, errStorageIOStalled) || errors.Is(probeErr, errStorageIOBusy) {
			item.Health = "stalled"
		}
		item.ErrorClass = storageErrorClass(probeErr)
		item.ErrorMessage = boundedStorageError(probeErr)
		return item
	}
	item.ResolvedPath = filepath.Clean(resolved)
	item.Health = "healthy"
	return item
}

func (s *Server) inspectLibraryRootsWithContext(ctx context.Context, library Library) []scanRootEvidence {
	evidence := preliminaryLibraryRootEvidence(library)
	// A recently stalled source is rejected from durable evidence without
	// touching the mount again. This prevents automatic scans from repeatedly
	// spawning blocked filesystem calls while the circuit is open.
	s.applyStoredStorageCircuits(ctx, evidence)
	for index := range evidence {
		if evidence[index].Health != "healthy" || evidence[index].CircuitHeld {
			continue
		}
		evidence[index] = s.probeLibraryRoot(ctx, evidence[index])
	}
	return evidence
}

func boundedStorageError(err error) string {
	if err == nil {
		return ""
	}
	return boundedStorageMessage(err.Error())
}

func boundedStorageMessage(message string) string {
	value := strings.TrimSpace(message)
	if len(value) > 2000 {
		value = value[:2000]
	}
	return value
}

func healthyScanRoots(evidence []scanRootEvidence) []scanRoot {
	roots := make([]scanRoot, 0, len(evidence))
	for _, item := range evidence {
		if item.Health != "healthy" || item.ResolvedPath == "" {
			continue
		}
		roots = append(roots, scanRoot{
			sourceID: item.SourceID, configured: item.ConfiguredPath, display: item.ConfiguredPath,
			real: item.ResolvedPath, classification: item.Classification,
		})
	}
	return roots
}

func (s *Server) applyStoredStorageCircuits(ctx context.Context, evidence []scanRootEvidence) {
	if scannerStorageCircuitBackoff <= 0 {
		return
	}
	for index := range evidence {
		item := &evidence[index]
		if item.Health != "healthy" {
			continue
		}
		var health, circuit, errorClass, errorMessage, lastFailure, storedClass, classSource string
		var failures int
		err := s.queryBackgroundRow(ctx, `
			SELECT health_state, circuit_state, error_class, error_message, last_failure_at,
				classification, classification_source, consecutive_failures
			FROM storage_sources WHERE id = ?`, item.SourceID).Scan(&health, &circuit, &errorClass, &errorMessage, &lastFailure, &storedClass, &classSource, &failures)
		if err != nil {
			continue
		}
		if classSource == "owner" {
			switch storageSourceClass(storedClass) {
			case storageSourceLocal, storageSourceNetwork, storageSourceFUSE, storageSourceUnknown:
				item.Classification = storageSourceClass(storedClass)
				item.ClassificationSource = "owner"
			}
		}
		if circuit != "open" || errorClass == "cancelled" {
			continue
		}
		failedAt, err := time.Parse(time.RFC3339Nano, lastFailure)
		if err != nil {
			failedAt, err = time.Parse(time.RFC3339, lastFailure)
		}
		if err != nil || time.Since(failedAt) >= storageCircuitBackoff(item.Classification, failures) {
			continue
		}
		item.Health = health
		if item.Health == "" || item.Health == "healthy" {
			item.Health = "degraded"
		}
		item.ErrorClass = errorClass
		item.ErrorMessage = errorMessage
		item.CircuitHeld = true
	}
}

func (s *Server) beginLibraryScanRun(ctx context.Context, library Library, jobID, mode string, roots []scanRootEvidence) (libraryScanRun, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	run := libraryScanRun{ID: randomID("scanrun"), LibraryID: library.ID, JobID: strings.TrimSpace(jobID), Mode: normalizeScanMode(mode), StartedAt: now}
	// Source and run admission are one background transaction. Traversal does
	// not begin unless every configured root has durable evidence.
	err := s.withBackgroundTxTagged(ctx, []string{"library_scan_runs", "storage_sources"}, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
		INSERT INTO library_scan_runs (id, library_id, job_id, mode, status, phase, started_at, updated_at)
		VALUES (?, ?, ?, ?, 'running', 'admission', ?, ?)`, run.ID, run.LibraryID, run.JobID, run.Mode, now, now); err != nil {
			return err
		}
		for _, root := range roots {
			failureAt, successAt := "", ""
			failures := 0
			if root.Health == "healthy" {
				successAt = now
			} else if !root.CircuitHeld {
				failureAt = now
				failures = 1
			}
			if _, err := tx.ExecContext(ctx, `
			INSERT INTO storage_sources (
				id, library_id, configured_path, resolved_path, classification, classification_source,
				health_state, circuit_state, error_class, error_message, latency_ms, consecutive_failures,
				last_progress_at, last_success_at, last_failure_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, CASE WHEN ? = 'healthy' THEN 'closed' ELSE 'open' END, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(library_id, configured_path) DO UPDATE SET
				resolved_path = excluded.resolved_path,
				classification = excluded.classification,
				classification_source = excluded.classification_source,
				health_state = excluded.health_state,
				circuit_state = excluded.circuit_state,
				error_class = excluded.error_class,
				error_message = excluded.error_message,
				latency_ms = excluded.latency_ms,
				consecutive_failures = CASE WHEN ? = 1 THEN storage_sources.consecutive_failures WHEN excluded.health_state = 'healthy' THEN 0 ELSE storage_sources.consecutive_failures + 1 END,
				last_progress_at = excluded.last_progress_at,
				last_success_at = CASE WHEN excluded.last_success_at = '' THEN storage_sources.last_success_at ELSE excluded.last_success_at END,
				last_failure_at = CASE WHEN excluded.last_failure_at = '' THEN storage_sources.last_failure_at ELSE excluded.last_failure_at END,
				updated_at = excluded.updated_at`,
				root.SourceID, library.ID, root.ConfiguredPath, root.ResolvedPath, root.Classification, root.ClassificationSource,
				root.Health, root.Health, root.ErrorClass, root.ErrorMessage, root.Latency.Milliseconds(), failures,
				now, successAt, failureAt, now, now, boolToInt(root.CircuitHeld)); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
			INSERT INTO library_scan_run_roots (
				run_id, source_id, configured_path, resolved_path, status, error_class, error_message,
				started_at, completed_at, last_progress_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, CASE WHEN ? = 'healthy' THEN '' ELSE ? END, ?)`,
				run.ID, root.SourceID, root.ConfiguredPath, root.ResolvedPath, root.Health,
				root.ErrorClass, root.ErrorMessage, now, root.Health, now, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return libraryScanRun{}, err
	}
	return run, nil
}

func (s *Server) updateScanRootEvidence(ctx context.Context, run libraryScanRun, root scanRoot, status, errorClass, errorMessage string, directories, files int) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	completed := ""
	if status != "running" {
		completed = now
	}
	_, err := s.execBackgroundWriteTagged(ctx, []string{}, `
		UPDATE library_scan_run_roots
		SET status = ?, error_class = ?, error_message = ?, directories_seen = ?, files_seen = ?,
			completed_at = ?, last_progress_at = ?
		WHERE run_id = ? AND source_id = ?`,
		status, errorClass, boundedStorageMessage(errorMessage), directories, files, completed, now, run.ID, root.sourceID)
	if err != nil {
		return err
	}
	_, err = s.execBackgroundWriteTagged(ctx, []string{}, `
		UPDATE storage_sources
		SET health_state = ?, circuit_state = CASE WHEN ? = 'healthy' THEN 'closed' ELSE 'open' END,
			error_class = ?, error_message = ?,
			consecutive_failures = CASE WHEN ? = 'healthy' THEN 0 ELSE consecutive_failures + 1 END,
			last_progress_at = ?,
			last_success_at = CASE WHEN ? = 'healthy' THEN ? ELSE last_success_at END,
			last_failure_at = CASE WHEN ? = 'healthy' THEN last_failure_at ELSE ? END,
			updated_at = ?
		WHERE id = ?`,
		status, status, errorClass, boundedStorageMessage(errorMessage), status, now,
		status, now, status, now, now, root.sourceID)
	return err
}

func (s *Server) finishLibraryScanRun(ctx context.Context, run libraryScanRun, status string, result libraryScanResult, warnings []string) error {
	if status != "healthy" && status != "degraded" && status != "failed" && status != "cancelled" {
		status = "failed"
	}
	warningBytes, _ := json.Marshal(warnings)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.execBackgroundWriteTagged(ctx, []string{}, `
		UPDATE library_scan_runs
		SET status = ?, phase = 'complete', files_indexed = ?, files_unchanged = ?, files_skipped = ?,
			missing_marked = ?, metadata_queued = ?, analysis_queued = ?, absence_authoritative = ?,
			cleanup_allowed = ?, warnings_json = ?, completed_at = ?, updated_at = ?
		WHERE id = ?`,
		status, result.FilesIndexed, result.FilesUnchanged, result.FilesSkipped, result.MissingMarked,
		result.MetadataRefreshQueued, result.AnalysisQueued, boolToInt(result.AbsenceAuthoritative),
		boolToInt(result.CleanupAllowed), string(warningBytes), now, now, run.ID)
	return err
}

func (s *Server) acquireLibraryScan(ctx context.Context, libraryID string) (func(), error) {
	key := strings.TrimSpace(libraryID)
	if key == "" {
		return nil, errors.New("library scan admission requires a library")
	}
	s.scannerMu.Lock()
	if s.scannerAdmissions == nil {
		s.scannerAdmissions = map[string]chan struct{}{}
	}
	gate := s.scannerAdmissions[key]
	if gate == nil {
		gate = make(chan struct{}, 1)
		s.scannerAdmissions[key] = gate
	}
	s.scannerMu.Unlock()
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
