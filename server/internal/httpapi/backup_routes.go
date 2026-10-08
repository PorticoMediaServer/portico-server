package httpapi

import (
	"context"
	"errors"
	"net/http"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/backup"
	"portico.local/server/internal/persistence"
)

// Plex-model server backups (Spec — Backups, §4): plain database copies with
// a manifest, listed, created, deleted and restored here. Every route is
// owner-only and published with the API registry.

// BackupsDocument is GET /v1/admin/backups.
type BackupsDocument struct {
	Backups     []backup.Info         `json:"backups"`
	Running     *backup.RunningJob    `json:"running,omitempty"`
	LastRestore *backup.RestoreResult `json:"lastRestore,omitempty"`
}

// StartBackupRequest is POST /v1/admin/backups.
type StartBackupRequest struct {
	OperationID string `json:"operationId"`
}

// StartBackupResponse answers 202 with the job to poll through running.
type StartBackupResponse struct {
	JobID string `json:"jobId"`
}

// RestoreBackupRequest is POST /v1/admin/backups/restore.
type RestoreBackupRequest struct {
	OperationID string        `json:"operationId"`
	Source      backup.Source `json:"source"`
}

// StatePermissionsDocument is POST /v1/admin/state-permissions:fix: whether
// the state folder is still readable by other users after the fix ran.
type StatePermissionsDocument struct {
	Exposed bool `json:"exposed"`
}

// backupRouteError maps service errors to the published codes.
func backupRouteError(err error) error {
	switch {
	case errors.Is(err, backup.ErrNotFound):
		return &apierror.Error{Code: "not_found"}
	case errors.Is(err, backup.ErrInvalid):
		return &apierror.Error{Code: "invalid_request"}
	case errors.Is(err, backup.ErrSourceInvalid):
		return &apierror.Error{Code: "restore_source_invalid"}
	case errors.Is(err, backup.ErrIntegrity):
		return &apierror.Error{Code: "restore_integrity_failed"}
	case errors.Is(err, persistence.ErrCandidateNotPortico):
		return &apierror.Error{Code: "restore_not_portico"}
	case errors.Is(err, persistence.ErrCandidateIntegrity):
		return &apierror.Error{Code: "restore_integrity_failed"}
	case errors.Is(err, persistence.ErrCandidateNewer):
		return &apierror.Error{Code: "restore_schema_newer"}
	case errors.Is(err, persistence.ErrCandidatePreRelease):
		return &apierror.Error{Code: "restore_pre_release"}
	case errors.Is(err, backup.ErrInsufficientDisk):
		return &apierror.Error{Code: "insufficient_disk"}
	case errors.Is(err, backup.ErrRestorePending):
		return &apierror.Error{Code: "request_in_progress"}
	default:
		return &apierror.Error{Code: "internal", Cause: err}
	}
}

func registerBackups(registry *apikit.Registry, d Dependencies) error {
	err := apikit.Register(registry, apikit.Route[struct{}, BackupsDocument]{Metadata: apikit.Metadata{
		ID: "get_admin_backups", Method: "GET", Path: "/v1/admin/backups", Summary: "List server backups and restore status", Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.Constant, Status: 200, Errors: []string{"unauthorized", "administration_unavailable", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (BackupsDocument, error) {
		out := BackupsDocument{Backups: []backup.Info{}}
		if d.Backups == nil {
			return out, &apierror.Error{Code: "administration_unavailable"}
		}
		listed, err := d.Backups.List()
		if err != nil {
			return out, backupRouteError(err)
		}
		out.Backups = listed
		if running, ok := d.Backups.Running(); ok {
			running := running
			out.Running = &running
		}
		last, err := d.Backups.LastRestore()
		if err != nil {
			return out, backupRouteError(err)
		}
		out.LastRestore = last
		if _, err = d.ownerContext(ctx, r); err != nil {
			return out, foundationAuthError(err)
		}
		return out, nil
	}})
	if err != nil {
		return err
	}
	err = apikit.Register(registry, apikit.Route[StartBackupRequest, StartBackupResponse]{Metadata: apikit.Metadata{
		ID: "post_admin_backups", Method: "POST", Path: "/v1/admin/backups", Summary: "Start a server backup", Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.Constant, BodyLimit: 4096, Status: 202, Errors: []string{"unauthorized", "invalid_request", "insufficient_disk", "administration_unavailable", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, body StartBackupRequest) (StartBackupResponse, error) {
		if d.Backups == nil {
			return StartBackupResponse{}, &apierror.Error{Code: "administration_unavailable"}
		}
		id, err := d.Backups.Start(ctx, backup.KindManual, body.OperationID)
		if err != nil {
			return StartBackupResponse{}, backupRouteError(err)
		}
		if _, err = d.ownerContext(ctx, r); err != nil {
			return StartBackupResponse{}, foundationAuthError(err)
		}
		return StartBackupResponse{JobID: id}, nil
	}})
	if err != nil {
		return err
	}
	err = apikit.Register(registry, apikit.Route[struct{}, struct{}]{Metadata: apikit.Metadata{
		ID: "delete_admin_backup", Method: "DELETE", Path: "/v1/admin/backups/{id}", Summary: "Delete a server backup", Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.Constant, Status: 204, Errors: []string{"unauthorized", "invalid_request", "not_found", "administration_unavailable", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (struct{}, error) {
		if d.Backups == nil {
			return struct{}{}, &apierror.Error{Code: "administration_unavailable"}
		}
		if err := d.Backups.Delete(r.PathValue("id")); err != nil {
			return struct{}{}, backupRouteError(err)
		}
		if _, err := d.ownerContext(ctx, r); err != nil {
			return struct{}{}, foundationAuthError(err)
		}
		return struct{}{}, nil
	}})
	if err != nil {
		return err
	}
	err = apikit.Register(registry, apikit.Route[RestoreBackupRequest, backup.StageResult]{Metadata: apikit.Metadata{
		ID: "post_admin_backups_restore", Method: "POST", Path: "/v1/admin/backups/restore", Summary: "Stage a backup for restore on restart", Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.Constant, BodyLimit: 4096, Status: 200, Errors: []string{"unauthorized", "invalid_request", "restore_source_invalid", "restore_schema_newer", "restore_pre_release", "restore_integrity_failed", "restore_not_portico", "insufficient_disk", "administration_unavailable", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, body RestoreBackupRequest) (backup.StageResult, error) {
		if d.Backups == nil {
			return backup.StageResult{}, &apierror.Error{Code: "administration_unavailable"}
		}
		if !validRestoreOperationID(body.OperationID) {
			return backup.StageResult{}, &apierror.Error{Code: "invalid_request"}
		}
		out, err := d.Backups.Stage(ctx, body.Source)
		if err != nil {
			return backup.StageResult{}, backupRouteError(err)
		}
		if _, err = d.ownerContext(ctx, r); err != nil {
			return backup.StageResult{}, foundationAuthError(err)
		}
		return out, nil
	}})
	if err != nil {
		return err
	}
	err = apikit.Register(registry, apikit.Route[struct{}, StatePermissionsDocument]{Metadata: apikit.Metadata{
		ID: "post_admin_state_permissions_fix", Method: "POST", Path: "/v1/admin/state-permissions:fix", Summary: "Tighten state folder permissions", Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.Constant, BodyLimit: 4096, OptionalBody: true, Status: 200, Errors: []string{"unauthorized", "administration_unavailable", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (StatePermissionsDocument, error) {
		if d.Administration == nil || d.Console == nil || d.Administration.StateDirectory() == "" {
			return StatePermissionsDocument{}, &apierror.Error{Code: "administration_unavailable"}
		}
		if err := backup.FixPermissions(d.Administration.StateDirectory()); err != nil {
			return StatePermissionsDocument{}, backupRouteError(err)
		}
		exposed, err := backup.CheckPermissions(d.Administration.StateDirectory())
		if err != nil {
			return StatePermissionsDocument{}, backupRouteError(err)
		}
		// Best effort: the fix stands even if the bookkeeping fails, and the
		// owner can dismiss the warning from the console.
		_ = d.Console.Alert(ctx, "state-permissions", "warning", exposed)
		if _, err = d.ownerContext(ctx, r); err != nil {
			return StatePermissionsDocument{}, foundationAuthError(err)
		}
		return StatePermissionsDocument{Exposed: exposed}, nil
	}})
	return err
}

// validRestoreOperationID mirrors the backup idempotency rule: the operation
// id is required and shaped, so a retried tap cannot stage twice by accident.
func validRestoreOperationID(v string) bool {
	if len(v) < 8 || len(v) > 128 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
