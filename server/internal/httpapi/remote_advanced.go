package httpapi

import (
	"context"
	"errors"
	"net/http"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/remotesources"
	"portico.local/server/internal/storage"
)

type SourceProgramRequest struct {
	OperationID      string `json:"operationId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Executable       string `json:"executable"`
}

type SourceProgramResponse struct {
	Scope   catalog.AdminScope          `json:"scope"`
	Program remotesources.SourceProgram `json:"program"`
}

type SourceAdvancedResponse struct {
	Scope    catalog.AdminScope                   `json:"scope"`
	Settings remotesources.SourceAdvancedSettings `json:"settings"`
}

func advancedError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, storage.ErrRemoteChanged), errors.Is(err, mounts.ErrCommandConflict):
		return &apierror.Error{Code: "revision_mismatch", Cause: err}
	case errors.Is(err, storage.ErrRemoteConfig), errors.Is(err, mounts.ErrInvalid):
		return &apierror.Error{Code: "invalid_request", Cause: err}
	case errors.Is(err, mounts.ErrUnavailable):
		return &apierror.Error{Code: "administration_unavailable", Cause: err}
	case errors.Is(err, storage.ErrBusy):
		return &apierror.Error{Code: "request_in_progress", Cause: err}
	default:
		return folderError(err)
	}
}

// registerSourceAdvanced publishes a source's drawn Advanced page: the scan
// schedule its folders keep, its real read cache, and the analysis stages its
// files use. Every value is read by the scanner, the scheduler or the analysis
// path; none of them is stored without a consumer.
func registerSourceAdvanced(registry *apikit.Registry, d Dependencies) {
	failures := []string{"unauthorized", "not_permitted", "not_found", "invalid_request", "revision_mismatch", "request_in_progress", "administration_unavailable", "internal"}
	v1Route(registry, apikit.Metadata{ID: "get_storage_source_advanced", Method: "GET", Path: "/v1/storage/sources/{id}/settings/advanced", Summary: "Read one remote source's scan, cache and analysis settings", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, Status: 200, Errors: failures}, func(ctx context.Context, r *http.Request, _ noBody) (SourceAdvancedResponse, error) {
		p, scope, err := d.storageOwner(r)
		if err != nil {
			return SourceAdvancedResponse{}, folderError(err)
		}
		if d.RemoteSources == nil {
			return SourceAdvancedResponse{}, folderError(mounts.ErrUnavailable)
		}
		if r.URL.RawQuery != "" {
			return SourceAdvancedResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		settings, err := d.RemoteSources.ReadSourceAdvanced(ctx, d.Identity, p, r.PathValue("id"))
		if err != nil {
			return SourceAdvancedResponse{}, advancedError(err)
		}
		return SourceAdvancedResponse{Scope: scope, Settings: settings}, nil
	})
	v1Route(registry, apikit.Metadata{ID: "put_storage_source_advanced", Method: "PUT", Path: "/v1/storage/sources/{id}/settings/advanced", Summary: "Save one remote source's scan, cache and analysis settings", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 8192, Status: 200, Errors: failures}, func(ctx context.Context, r *http.Request, in remotesources.SourceAdvancedChange) (SourceAdvancedResponse, error) {
		p, scope, err := d.storageOwner(r)
		if err != nil {
			return SourceAdvancedResponse{}, folderError(err)
		}
		if d.RemoteSources == nil {
			return SourceAdvancedResponse{}, folderError(mounts.ErrUnavailable)
		}
		if r.URL.RawQuery != "" {
			return SourceAdvancedResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		settings, err := d.RemoteSources.SaveSourceAdvanced(ctx, d.Identity, p, r.PathValue("id"), in)
		if err != nil {
			return SourceAdvancedResponse{}, advancedError(err)
		}
		return SourceAdvancedResponse{Scope: scope, Settings: settings}, nil
	})
}

// registerSourceProgram publishes the mounted source's program change: the
// mount's own approval runs the chosen binary with version, and the captured
// source association is re-checked before it is accepted.
func registerSourceProgram(registry *apikit.Registry, d Dependencies) {
	failures := []string{"unauthorized", "not_permitted", "not_found", "invalid_request", "revision_mismatch", "idempotency_key_reused", "request_in_progress", "administration_unavailable", "internal"}
	v1Route(registry, apikit.Metadata{ID: "put_storage_source_program", Method: "PUT", Path: "/v1/storage/sources/{id}/settings/program", Summary: "Change the rclone program that serves one mounted source", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminLong, Cost: apikit.Constant, BodyLimit: 8192, Status: 200, Errors: failures}, func(ctx context.Context, r *http.Request, in SourceProgramRequest) (SourceProgramResponse, error) {
		p, scope, err := d.storageOwner(r)
		if err != nil {
			return SourceProgramResponse{}, folderError(err)
		}
		if d.RemoteSources == nil {
			return SourceProgramResponse{}, folderError(mounts.ErrUnavailable)
		}
		if r.URL.RawQuery != "" {
			return SourceProgramResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		program, err := d.RemoteSources.SetSourceProgram(ctx, d.Identity, p, r.PathValue("id"), in.OperationID, in.Executable, in.ExpectedRevision)
		if err != nil {
			return SourceProgramResponse{}, advancedError(err)
		}
		return SourceProgramResponse{Scope: scope, Program: program}, nil
	})
}
