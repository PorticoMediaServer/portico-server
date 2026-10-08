package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/contentaccess"
	"portico.local/server/internal/downloads"
	"portico.local/server/internal/identity"
	"strconv"
)

func (d Dependencies) downloadRequestAccess(ctx context.Context, tx *sql.Tx, p identity.Principal, device, item string) (string, []any, error) {
	family, e := d.Identity.SessionFamilyTx(ctx, tx, p)
	if e != nil {
		return "", nil, e
	}
	if device != "" {
		var matches bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM identity_device_families WHERE family_id=? AND device_id=?)`, family.ID, device).Scan(&matches); e != nil {
			return "", nil, e
		}
		if !matches {
			return "", nil, identity.ErrUnauthorized
		}
	}
	return contentaccess.VisibleItemsSQL(ctx, tx, p, item, func(lib string) error { return d.allowedLibraryTx(ctx, p, lib, tx) })
}
func downloadRequestError(e error) error {
	switch {
	case errors.Is(e, downloads.ErrRequestConflict):
		return &apierror.Error{Code: "idempotency_key_reused", Cause: e}
	case errors.Is(e, downloads.ErrInput):
		return &apierror.Error{Code: "invalid_request", Cause: e}
	case errors.Is(e, downloads.ErrPolicy):
		return &apierror.Error{Code: "not_permitted", Cause: e}
	case errors.Is(e, downloads.ErrNotFound):
		return &apierror.Error{Code: "not_found", Cause: e}
	case errors.Is(e, downloads.ErrCapacity):
		return &apierror.Error{Code: "rate_limited", Cause: e}
	}
	return e
}
func registerDownloadRequests(r *apikit.Registry, d Dependencies) {
	failures := []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "idempotency_key_reused", "rate_limited", "internal"}
	v1Route(r, apikit.Metadata{ID: "create_download_request", Method: "POST", Path: "/v1/downloads/requests", Summary: "Queue all visible members of a container for offline preparation", Access: apikit.Device, Lane: apikit.Default, Cost: apikit.SelectionAsync, Status: 202, BodyLimit: 8192, Errors: failures}, func(ctx context.Context, q *http.Request, in downloads.ContainerRequest) (downloads.RequestView, error) {
		p, e := d.principal(q)
		if e != nil {
			return downloads.RequestView{}, e
		}
		if d.Downloads == nil {
			return downloads.RequestView{}, downloads.ErrPolicy
		}
		out, e := d.Downloads.SubmitContainer(ctx, p, in)
		return out, downloadRequestError(e)
	})
	v1Route(r, apikit.Metadata{ID: "get_download_request", Method: "GET", Path: "/v1/downloads/requests/{id}", Summary: "Read visible request counts and a window of preparations", Access: apikit.Device, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200, Query: []string{"cursor", "limit"}, Errors: failures}, func(ctx context.Context, q *http.Request, _ noBody) (downloads.RequestView, error) {
		p, e := d.principal(q)
		if e != nil {
			return downloads.RequestView{}, e
		}
		if d.Downloads == nil {
			return downloads.RequestView{}, downloads.ErrPolicy
		}
		limit := 100
		if v := q.URL.Query().Get("limit"); v != "" {
			limit, e = strconv.Atoi(v)
			if e != nil {
				return downloads.RequestView{}, downloadRequestError(downloads.ErrInput)
			}
		}
		out, e := d.Downloads.ContainerRequest(ctx, p, q.PathValue("id"), q.URL.Query().Get("cursor"), limit)
		return out, downloadRequestError(e)
	})
}
