package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"unicode"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/rcloneprogram"
	"portico.local/server/internal/storage"
)

// SourceProgram is the rclone program that serves a mounted source right now.
type SourceProgram struct {
	Executable string `json:"executable"`
	Version    string `json:"version"`
	Revision   int64  `json:"revision"`
	Included   bool   `json:"included"`
}

// SetSourceProgram changes the program that serves one mounted source, using
// the mount's own approval path (it is run with version before it is accepted)
// and the captured source association. It restarts that source's serve process
// through the ordinary mount command; nothing about the source's identity,
// folders, read profile or admission changes here.
func (s *Service) SetSourceProgram(ctx context.Context, identities *identity.Service, owner identity.Principal, source, operationID, executable string, useIncluded bool, expectedRevision int64) (SourceProgram, error) {
	if s == nil || s.db == nil || s.mounts == nil || source == "" || expectedRevision < 1 || len(operationID) > 128 || operationID == "" {
		return SourceProgram{}, storage.ErrRemoteConfig
	}
	includedPath := ""
	if bundled, e := rcloneprogram.Resolve(); e == nil {
		includedPath = bundled.Path
	}
	if useIncluded && strings.TrimSpace(executable) != "" {
		// Two answers to one question is ambiguous; refuse instead of picking.
		return SourceProgram{}, storage.ErrRemoteConfig
	}
	if useIncluded {
		if includedPath == "" {
			return SourceProgram{}, storage.ErrRemoteConfig
		}
		executable = includedPath
	}
	executable = strings.TrimSpace(executable)
	if !filepath.IsAbs(executable) || len(executable) > 8192 || strings.IndexFunc(executable, unicode.IsControl) >= 0 || strings.Contains(executable, "\x00") {
		return SourceProgram{}, storage.ErrRemoteConfig
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return SourceProgram{}, err
	}
	if err = currentLocalOwnerTx(ctx, snapshot.Tx(), identities, owner); err != nil {
		snapshot.Rollback()
		return SourceProgram{}, err
	}
	row, err := sourceRowTx(ctx, snapshot.Tx(), source)
	snapshot.Rollback()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SourceProgram{}, storage.ErrRemoteOffline
		}
		return SourceProgram{}, err
	}
	if row.MountID == "" {
		return SourceProgram{}, storage.ErrRemoteConfig
	}
	admitted := row
	// No control pre-check here: the mount command resolves the actor-bound
	// receipt for this exact operation before it evaluates a new intent, so a
	// lost reply is recovered rather than read as a stale revision. A genuinely
	// changed intent still CASes against expectedRevision and is refused.
	authorize := func(tx *sql.Tx) error {
		if err := currentLocalOwnerTx(ctx, tx, identities, owner); err != nil {
			return err
		}
		current, err := sourceRowTx(ctx, tx, admitted.ID)
		if err != nil {
			return storage.ErrRemoteChanged
		}
		if current.Generation != admitted.Generation || current.RootPath != admitted.RootPath || current.MountID != admitted.MountID || current.Kind != admitted.Kind {
			return storage.ErrRemoteChanged
		}
		return nil
	}
	receipt, err := s.mounts.Command(ctx, owner.Hash, admitted.MountID, mounts.Command{
		OperationID:      operationID,
		Action:           "configure",
		ExpectedRevision: expectedRevision,
		Executable:       executable,
		UseIncluded:      &useIncluded,
	}, authorize)
	if err != nil {
		return SourceProgram{}, err
	}
	if receipt.Mount == nil {
		return SourceProgram{}, storage.ErrRemoteConfig
	}
	return SourceProgram{Executable: receipt.Mount.Executable, Version: receipt.Mount.ExecutableVersion, Revision: receipt.Mount.ControlRevision, Included: receipt.Mount.ProgramIncluded}, nil
}
