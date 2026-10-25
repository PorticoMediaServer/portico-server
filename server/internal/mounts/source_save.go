package mounts

import (
	"context"
	"database/sql"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/rcloneprogram"
)

// RcloneSourceInput accepts connection material, never a process, mount action,
// executable path or a prior unsaved Test result as authority to save.
type RcloneSourceInput struct {
	OperationID string `json:"operationId"`
	Name        string `json:"name"`
	Remote      string `json:"remote"`
	Config      string `json:"config"`
}

// RcloneSourceReceipt projects only the durable source identity. The private
// allocation and stopped legacy mount record are not a public lifecycle.
type RcloneSourceReceipt struct {
	OperationID string `json:"operationId"`
	Accepted    bool   `json:"accepted"`
	SourceID    string `json:"sourceId"`
	Generation  int64  `json:"generation"`
	Removed     bool   `json:"removed,omitempty"`
}

func sourceReceipt(receipt Receipt) (RcloneSourceReceipt, error) {
	if !receipt.Accepted || receipt.Mount == nil || receipt.MountID == "" || receipt.Mount.ID != receipt.MountID || receipt.Mount.BackendRevision < 1 || receipt.Deleted != receipt.Mount.RemovalPending {
		return RcloneSourceReceipt{}, ErrCommandConflict
	}
	return RcloneSourceReceipt{OperationID: receipt.OperationID, Accepted: true, SourceID: receipt.MountID, Generation: receipt.Mount.BackendRevision, Removed: receipt.Deleted}, nil
}

func (s *Service) rcloneSourceOwner(ctx context.Context, authorize func(*sql.Tx) error) error {
	if s == nil || authorize == nil {
		return identity.ErrUnauthorized
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return err
	}
	defer snapshot.Rollback()
	return authorize(snapshot.Tx())
}

// RcloneSourceOperation recovers a durable receipt without rerunning the save,
// resolving a new executable or interpreting a test result as approval.
func (s *Service) RcloneSourceOperation(ctx context.Context, actor, operation string, authorize func(*sql.Tx) error) (RcloneSourceReceipt, error) {
	if actor == "" {
		return RcloneSourceReceipt{}, identity.ErrUnauthorized
	}
	if err := s.rcloneSourceOwner(ctx, authorize); err != nil {
		return RcloneSourceReceipt{}, err
	}
	receipt, err := s.Receipt(actor, operation)
	if err != nil {
		return RcloneSourceReceipt{}, err
	}
	if err = s.rcloneSourceOwner(ctx, authorize); err != nil {
		return RcloneSourceReceipt{}, err
	}
	return sourceReceipt(receipt)
}

// SaveRcloneSource uses the existing owner-save transaction and candidate test:
// configuration, endpoint approvals, observed range facts and receipt publish
// together. No result from TestRcloneConnection skips that independent test.
func (s *Service) SaveRcloneSource(ctx context.Context, actor string, input RcloneSourceInput, authorize func(*sql.Tx) error) (RcloneSourceReceipt, error) {
	if s == nil || authorize == nil || actor == "" {
		return RcloneSourceReceipt{}, identity.ErrUnauthorized
	}
	if err := s.rcloneSourceOwner(ctx, authorize); err != nil {
		return RcloneSourceReceipt{}, err
	}
	if err := validateConfig(input.Config, input.Remote); err != nil {
		return RcloneSourceReceipt{}, ErrConfigInvalid
	}
	program, err := rcloneprogram.Resolve()
	if err != nil {
		return RcloneSourceReceipt{}, ErrExecutableInvalid
	}
	receipt, err := s.Command(ctx, actor, "", Command{OperationID: input.OperationID, Action: "create", Name: input.Name,
		Executable: program.Path, Remote: input.Remote, Config: input.Config}, authorize)
	if err != nil {
		return RcloneSourceReceipt{}, err
	}
	return sourceReceipt(receipt)
}
