package mounts

import (
	"context"
	"database/sql"
)

// SourceCandidatePublication belongs to one exact owner save and candidate.
// Mounts calls CheckAfterTest only after its real connection test succeeds,
// then CommitTx in the same authorized transaction as config and receipt.
// Neither callback grants content/analysis access or runs provider I/O in SQL.
type SourceCandidatePublication struct {
	CheckAfterTest func(context.Context) error
	CommitTx       func(context.Context, *sql.Tx, Mount) error
}

func (s *Service) sourceCandidate(ctx context.Context, raw []byte, remote string, authorize func(*sql.Tx) error) (*SourceCandidatePublication, error) {
	if s.NativeSourceCandidate == nil {
		return nil, nil
	}
	publication, err := s.NativeSourceCandidate(ctx, raw, remote, authorize)
	if err == nil && publication != nil && (publication.CheckAfterTest == nil || publication.CommitTx == nil) {
		return nil, ErrConfigInvalid
	}
	return publication, err
}
