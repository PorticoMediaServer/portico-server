package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// storageIORequest is the common admission contract for every consumer that
// touches owner-managed media storage. Playback, metadata and optimization can
// use the same boundary without inheriting scanner implementation details.
type storageIORequest struct {
	SourceID       string
	AdmissionKey   string
	Classification storageSourceClass
	Operation      string
	Timeout        time.Duration
	CircuitState   string
	TrackHealth    bool
	RecoveryProbe  bool
	// AdmissionLimit overrides the class policy when a consumer has stronger
	// source-health evidence. Zero selects the conservative class default.
	AdmissionLimit int
}

// storageIOAdmission keeps the live count independent from the current
// source policy. Classification overrides can therefore tighten or relax the
// limit immediately without replacing a channel while calls are in flight.
type storageIOAdmission struct {
	inFlight            int
	lastProgressPersist time.Time
}

var (
	errStorageIOStalled   = errors.New("storage operation made no progress")
	errStorageIOBusy      = errors.New("storage source already has an unresponsive operation")
	errStorageIOCapacity  = errors.New("storage I/O quarantine capacity is exhausted")
	errStorageCircuitOpen = errors.New("storage source circuit is open")
)

var storageIOGlobalAdmissionLimit = 64

var storageIOOperationTimeout = func(class storageSourceClass) time.Duration {
	switch class {
	case storageSourceNetwork:
		return 20 * time.Second
	case storageSourceFUSE:
		return 30 * time.Second
	case storageSourceUnknown:
		return 30 * time.Second
	default:
		return 90 * time.Second
	}
}

// boundedStorageIO contains a potentially blocking syscall in a quarantinable
// goroutine. A timed-out call intentionally retains the per-source admission
// token until the kernel call actually returns. Remote, FUSE and unknown
// sources admit one call; healthy local storage admits a small parallel set so
// ordinary playback/metadata work does not serialize behind scanner stats.
// Either way, a dead source has a strict finite quarantine bound.
func (s *Server) boundedStorageIO(ctx context.Context, request storageIORequest, operation func() error) error {
	return s.boundedStorageProgressIO(ctx, request, func(func()) error { return operation() })
}

func (s *Server) boundedStorageProgressIO(ctx context.Context, request storageIORequest, operation func(progress func()) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(request.CircuitState), "open") && !request.RecoveryProbe {
		return fmt.Errorf("%w: %s", errStorageCircuitOpen, request.Operation)
	}
	key := strings.TrimSpace(request.AdmissionKey)
	if key == "" {
		key = request.SourceID
	}
	if key == "" {
		key = "storage:unknown"
	}
	s.storageIOMu.Lock()
	if s.storageIOAdmissions == nil {
		s.storageIOAdmissions = map[string]*storageIOAdmission{}
	}
	admission := s.storageIOAdmissions[key]
	if admission == nil {
		admission = &storageIOAdmission{}
		s.storageIOAdmissions[key] = admission
	}
	if admission.inFlight >= storageIOAdmissionLimit(request) {
		s.storageIOMu.Unlock()
		return fmt.Errorf("%w: %s", errStorageIOBusy, request.Operation)
	}
	if storageIOGlobalAdmissionLimit > 0 && s.storageIOInFlight >= storageIOGlobalAdmissionLimit {
		s.storageIOMu.Unlock()
		return fmt.Errorf("%w: %s", errStorageIOCapacity, request.Operation)
	}
	admission.inFlight++
	s.storageIOInFlight++
	s.storageIOMu.Unlock()
	done := make(chan error, 1)
	progress := make(chan struct{}, 1)
	go func() {
		err := operation(func() {
			select {
			case progress <- struct{}{}:
			default:
			}
			s.recordStorageIOProgress(request, admission)
		})
		s.storageIOMu.Lock()
		admission.inFlight--
		s.storageIOInFlight--
		if admission.inFlight == 0 && s.storageIOAdmissions[key] == admission {
			delete(s.storageIOAdmissions, key)
		}
		s.storageIOMu.Unlock()
		done <- err
	}()
	started := time.Now()
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = storageIOOperationTimeout(request.Classification)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	for {
		select {
		case err := <-done:
			s.recordStorageIOOutcome(request, time.Since(started), err)
			return err
		case <-ctx.Done():
			s.recordStorageIOOutcome(request, time.Since(started), ctx.Err())
			return ctx.Err()
		case <-progress:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		case <-timer.C:
			err := fmt.Errorf("%w during %s after %s without progress", errStorageIOStalled, request.Operation, timeout)
			s.recordStorageIOOutcome(request, time.Since(started), err)
			return err
		}
	}
}

func (s *Server) recordStorageIOProgress(request storageIORequest, admission *storageIOAdmission) {
	if !request.TrackHealth || s == nil || s.db == nil || admission == nil || strings.TrimSpace(request.SourceID) == "" {
		return
	}
	nowTime := time.Now().UTC()
	s.storageIOMu.Lock()
	if !admission.lastProgressPersist.IsZero() && nowTime.Sub(admission.lastProgressPersist) < 5*time.Second {
		s.storageIOMu.Unlock()
		return
	}
	admission.lastProgressPersist = nowTime
	s.storageIOMu.Unlock()
	now := nowTime.Format(time.RFC3339Nano)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = s.execBackgroundWrite(ctx, `UPDATE storage_sources SET last_progress_at = ?, updated_at = ? WHERE id = ?`, now, now, request.SourceID)
	}()
}

func (s *Server) recordStorageIOOutcome(request storageIORequest, latency time.Duration, outcome error) {
	if !request.TrackHealth || s == nil || s.db == nil || strings.TrimSpace(request.SourceID) == "" {
		return
	}
	if errors.Is(outcome, context.Canceled) {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	latencyMS := latency.Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
	}
	if outcome == nil {
		_, _ = s.execBackgroundWrite(context.Background(), `
			UPDATE storage_sources SET health_state = 'healthy', circuit_state = 'closed',
				error_class = '', error_message = '', latency_ms = ?, consecutive_failures = 0,
				last_progress_at = ?, last_success_at = ?, updated_at = ? WHERE id = ?`,
			latencyMS, now, now, now, request.SourceID)
		return
	}
	class := storageErrorClass(outcome)
	health := "degraded"
	if errors.Is(outcome, errStorageIOStalled) || errors.Is(outcome, context.DeadlineExceeded) {
		health = "stalled"
	} else if errors.Is(outcome, os.ErrNotExist) {
		health = "offline"
	}
	_, _ = s.execBackgroundWrite(context.Background(), `
		UPDATE storage_sources SET health_state = ?, circuit_state = 'open', error_class = ?,
			error_message = ?, latency_ms = ?, consecutive_failures = consecutive_failures + 1,
			last_failure_at = ?, updated_at = ? WHERE id = ?`,
		health, class, boundedStorageError(outcome), latencyMS, now, now, request.SourceID)
}

func storageIOAdmissionLimit(request storageIORequest) int {
	if request.AdmissionLimit > 0 {
		return request.AdmissionLimit
	}
	if request.Classification == storageSourceLocal {
		return 4
	}
	return 1
}

func (s *Server) storageStat(ctx context.Context, request storageIORequest, path string) (os.FileInfo, error) {
	var result os.FileInfo
	err := s.boundedStorageIO(ctx, request, func() error {
		var err error
		result, err = os.Stat(path)
		return err
	})
	return result, err
}

func (s *Server) storageEvalSymlinks(ctx context.Context, request storageIORequest, path string) (string, error) {
	var result string
	err := s.boundedStorageIO(ctx, request, func() error {
		var err error
		result, err = filepath.EvalSymlinks(path)
		return err
	})
	return result, err
}

func (s *Server) storageReadDir(ctx context.Context, request storageIORequest, path string) ([]os.DirEntry, error) {
	var result []os.DirEntry
	err := s.boundedStorageIO(ctx, request, func() error {
		var err error
		result, err = os.ReadDir(path)
		return err
	})
	return result, err
}

const storageReadBufferLimit int64 = 16 << 20

// storageReadRange supervises open, seek and read as one bounded operation.
// The result buffer is owned by the worker until successful completion, so a
// timed-out kernel read cannot continue mutating memory returned to a caller.
func (s *Server) storageReadRange(ctx context.Context, request storageIORequest, path string, offset, length int64) ([]byte, error) {
	if offset < 0 || length < 0 || length > storageReadBufferLimit {
		return nil, fmt.Errorf("storage read range must be between 0 and %d bytes", storageReadBufferLimit)
	}
	var result []byte
	err := s.boundedStorageProgressIO(ctx, request, func(progress func()) error {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		if offset > 0 {
			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				return err
			}
		}
		buffer := make([]byte, length)
		read := 0
		for read < len(buffer) {
			count, readErr := file.Read(buffer[read:])
			if count > 0 {
				read += count
				progress()
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					break
				}
				return readErr
			}
			if count == 0 {
				return io.ErrNoProgress
			}
		}
		result = buffer[:read]
		return nil
	})
	return result, err
}

func (s *Server) storageReadFile(ctx context.Context, request storageIORequest, path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 || maxBytes > storageReadBufferLimit {
		maxBytes = storageReadBufferLimit
	}
	info, err := s.storageStat(ctx, request, path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("storage file exceeds the %d-byte supervised read limit", maxBytes)
	}
	return s.storageReadRange(ctx, request, path, 0, info.Size())
}

func storageRequestForRoot(root scanRoot, operation string) storageIORequest {
	return storageIORequest{
		SourceID: root.sourceID, AdmissionKey: storagePhysicalSourceKey(root.configured, root.classification),
		Classification: root.classification, Operation: operation,
	}
}

func storagePhysicalSourceKey(path string, class storageSourceClass) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	volume := strings.ToLower(filepath.VolumeName(clean))
	if volume != "" {
		return "storage:" + string(class) + ":" + volume
	}
	slashed := strings.Trim(filepath.ToSlash(clean), "/")
	parts := strings.Split(slashed, "/")
	count := 1
	if len(parts) > 1 {
		count = 2
	}
	if len(parts) > 2 && strings.EqualFold(parts[0], "media") {
		count = 3
	}
	if slashed == "" {
		return "storage:" + string(class) + ":/"
	}
	return "storage:" + string(class) + ":/" + strings.ToLower(strings.Join(parts[:count], "/"))
}

func (s *Server) storageRequestForPath(ctx context.Context, path, operation string) storageIORequest {
	clean := filepath.Clean(path)
	class, _ := classifyStorageSource(clean, "")
	request := storageIORequest{
		SourceID:     storageSourceID("unassigned", filepath.VolumeName(clean)+string(filepath.Separator)),
		AdmissionKey: storagePhysicalSourceKey(clean, class), Classification: class, Operation: operation,
	}
	rows, err := s.queryBackgroundRead(ctx, `SELECT id, configured_path, resolved_path, classification, circuit_state FROM storage_sources`)
	if err != nil {
		return request
	}
	defer rows.Close()
	bestLength := -1
	for rows.Next() {
		var id, configured, resolved, storedClass, circuitState string
		if rows.Scan(&id, &configured, &resolved, &storedClass, &circuitState) != nil {
			continue
		}
		root := resolved
		if root == "" {
			root = configured
		}
		if !pathWithinRoot(clean, root) || len(root) <= bestLength {
			continue
		}
		bestLength = len(root)
		request.SourceID = id
		request.AdmissionKey = storagePhysicalSourceKey(root, storageSourceClass(storedClass))
		request.CircuitState = circuitState
		request.TrackHealth = true
		switch storageSourceClass(storedClass) {
		case storageSourceLocal, storageSourceNetwork, storageSourceFUSE, storageSourceUnknown:
			request.Classification = storageSourceClass(storedClass)
		}
	}
	return request
}

func storageRootForPath(roots []scanRoot, path string) (scanRoot, bool) {
	for _, root := range roots {
		if pathWithinRoot(path, root.real) {
			return root, true
		}
	}
	return scanRoot{}, false
}

func storageErrorTransient(err error) bool {
	if err == nil {
		return false
	}
	_, errnoTransient := storageErrnoClass(err)
	lower := strings.ToLower(err.Error())
	textTransient := strings.Contains(lower, "stale file handle") || strings.Contains(lower, "estale") ||
		strings.Contains(lower, "input/output error") || strings.Contains(lower, "i/o error") || strings.Contains(lower, "eio")
	return errors.Is(err, errScannerRootStalled) || errors.Is(err, errStorageIOStalled) ||
		errors.Is(err, errStorageIOBusy) || errors.Is(err, errStorageIOCapacity) ||
		errors.Is(err, errStorageCircuitOpen) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, os.ErrPermission) || errnoTransient || textTransient
}

func storageErrorAffectsAuthority(err error) bool {
	if err == nil {
		return false
	}
	return storageErrorTransient(err) || errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission)
}
