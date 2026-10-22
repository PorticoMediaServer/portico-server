package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"

	"portico.local/server/internal/persistence"
	"portico.local/server/internal/supervise"
)

type RestoreJob struct {
	JobID      string       `json:"jobId"`
	Phase      string       `json:"phase"`
	BytesDone  int64        `json:"bytesDone"`
	BytesTotal int64        `json:"bytesTotal"`
	Result     *StageResult `json:"result,omitempty"`
	ErrorCode  string       `json:"errorCode,omitempty"`
}

// StartRestore returns immediately. Copy and integrity work keep their own
// lifetime, so an HTTP timeout or a disconnected browser cannot interrupt them.
func (s *Service) StartRestore(ctx context.Context, source Source, operation string) (string, error) {
	if !validOperationID(operation) {
		return "", ErrInvalid
	}
	if s.restoreUnavailable {
		return "", ErrRestoreLockUnavailable
	}
	last, err := s.LastRestore()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	if last != nil && last.OperationID == operation && last.JobID != "" && last.Outcome == "restored" {
		if last.SourceDigest != restoreSourceDigest(source) {
			s.mu.Unlock()
			return "", ErrInvalid
		}
		s.restoreOperation, s.restoreSource = operation, source
		s.restoreJob = &RestoreJob{JobID: last.JobID, Phase: "staged"}
		if last.Outcome == "restored" {
			s.restoreJob.Result = &StageResult{Staged: true, RestartRequired: true, Validation: Validation{SchemaVersion: last.SchemaVersion, Integrity: "ok"}}
		} else {
			s.restoreJob.Phase = "failed"
			s.restoreJob.ErrorCode = "restore_integrity_failed"
		}
		s.mu.Unlock()
		return last.JobID, nil
	}
	if operation == s.restoreOperation && s.restoreJob != nil {
		if source != s.restoreSource {
			s.mu.Unlock()
			return "", ErrInvalid
		}
		if s.restoreJob.Phase != "failed" {
			id := s.restoreJob.JobID
			s.mu.Unlock()
			return id, nil
		}
	}
	if s.restoreJob != nil && s.restoreJob.Phase != "failed" {
		s.mu.Unlock()
		return "", ErrRestorePending
	}
	id := newToken()
	s.restoreOperation, s.restoreSource = operation, source
	s.restoreJob = &RestoreJob{JobID: id, Phase: "queued"}
	s.mu.Unlock()
	supervise.Go("backup.restore", func() {
		result, err := s.Stage(context.WithoutCancel(ctx), source)
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			s.restoreJob.Phase = "failed"
			s.restoreJob.ErrorCode = restoreCode(err)
			return
		}
		s.restoreJob.Phase = "staged"
		s.restoreJob.Result = &result
	})
	return id, nil
}

func (s *Service) RestoreProgress() *RestoreJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restoreJob == nil {
		return nil
	}
	copy := *s.restoreJob
	return &copy
}

func (s *Service) restoreProgress(phase string, done, total int64) {
	s.mu.Lock()
	if s.restoreJob == nil {
		s.mu.Unlock()
		return
	}
	changed := s.restoreJob.Phase != phase
	id := s.restoreJob.JobID
	s.restoreJob.Phase = phase
	s.restoreJob.BytesDone, s.restoreJob.BytesTotal = done, total
	s.mu.Unlock()
	if changed {
		log.Printf("debug: [server] Restore %s entered %s", id, phase)
	}
}

func restoreCode(err error) string {
	switch {
	case errors.Is(err, ErrSourceInvalid):
		return "restore_source_invalid"
	case errors.Is(err, ErrInsufficientDisk):
		return "insufficient_disk"

	case errors.Is(err, persistence.ErrCandidatePreRelease):
		return "restore_pre_release"
	case errors.Is(err, persistence.ErrCandidateNotPortico):
		return "restore_not_portico"
	case errors.Is(err, persistence.ErrCandidateIntegrity), errors.Is(err, ErrIntegrity):
		return "restore_integrity_failed"
	default:
		return "internal"
	}
}

func restoreSourceDigest(source Source) string {
	raw, _ := json.Marshal(source)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
