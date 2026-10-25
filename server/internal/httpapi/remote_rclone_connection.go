package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/mounts"
)

type RcloneConnectionResponse struct {
	Scope  catalog.AdminScope            `json:"scope"`
	Result mounts.RcloneConnectionResult `json:"result"`
}

func rcloneConnectionFailure(err error) error {
	switch {
	case errors.Is(err, mounts.ErrConfigInvalid), errors.Is(err, mounts.ErrInvalid):
		return &apierror.Error{Code: "invalid_request"}
	case errors.Is(err, mounts.ErrExecutableInvalid), errors.Is(err, mounts.ErrCommandCapacity):
		return &apierror.Error{Code: "administration_unavailable"}
	default:
		// This projection deliberately discards transport/credential causes.
		return davConnectionFailure(err)
	}
}

func registerRcloneConnectionTest(registry *apikit.Registry, d Dependencies) {
	v1Route(registry, apikit.Metadata{ID: "test_storage_source_rclone", Method: "POST", Path: "/v1/storage/sources/rclone/test", Summary: "Test an unsaved imported rclone source configuration", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminLong, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "source_credentials_required", "administration_unavailable", "internal"}}, func(ctx context.Context, r *http.Request, in mounts.RcloneConnectionInput) (RcloneConnectionResponse, error) {
		p, scope, err := d.storageOwner(r)
		if err != nil {
			return RcloneConnectionResponse{}, rcloneConnectionFailure(err)
		}
		if d.Mounts == nil || d.RemoteSources == nil {
			return RcloneConnectionResponse{}, rcloneConnectionFailure(mounts.ErrUnavailable)
		}
		if r.URL.RawQuery != "" {
			return RcloneConnectionResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		result, err := d.Mounts.TestRcloneConnection(ctx, in, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if err != nil {
			return RcloneConnectionResponse{}, rcloneConnectionFailure(err)
		}
		if _, _, err := d.storageOwner(r); err != nil {
			return RcloneConnectionResponse{}, rcloneConnectionFailure(err)
		}
		return RcloneConnectionResponse{Scope: scope, Result: result}, nil
	})
}
