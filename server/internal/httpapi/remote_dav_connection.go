package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/remotesources"
	"portico.local/server/internal/storage"
)

type DAVConnectionResponse struct {
	Scope  catalog.AdminScope                `json:"scope"`
	Result remotesources.DAVConnectionResult `json:"result"`
}

func registerDAVConnectionTest(registry *apikit.Registry, d Dependencies) {
	v1Route(registry, apikit.Metadata{ID: "test_storage_source_webdav", Method: "POST", Path: "/v1/storage/sources/webdav/test", Summary: "Test a new WebDAV connection and bounded ranges without saving a source", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminLong, Cost: apikit.Constant, BodyLimit: 32768, Status: 200, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "source_credentials_required", "internal"}}, func(ctx context.Context, r *http.Request, in remotesources.DAVConnectionInput) (DAVConnectionResponse, error) {
		p, scope, err := d.storageOwner(r)
		if err != nil {
			return DAVConnectionResponse{}, identityRouteError(err)
		}
		if d.RemoteSources == nil || r.URL.RawQuery != "" {
			return DAVConnectionResponse{}, &apierror.Error{Code: "invalid_request"}
		}
		result, err := d.RemoteSources.TestDAVConnection(ctx, in, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if err != nil {
			switch {
			case errors.Is(err, storage.ErrRemoteConfig):
				return DAVConnectionResponse{}, &apierror.Error{Code: "invalid_request"}
			case errors.Is(err, storage.ErrRemoteCredentials):
				return DAVConnectionResponse{}, &apierror.Error{Code: "source_credentials_required"}
			default:
				return DAVConnectionResponse{}, identityRouteError(err)
			}
		}
		if _, _, err = d.storageOwner(r); err != nil {
			return DAVConnectionResponse{}, identityRouteError(err)
		}
		return DAVConnectionResponse{Scope: scope, Result: result}, nil
	})
}
