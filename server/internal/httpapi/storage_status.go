package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/persistence"
)

func registerStorageStatus(registry *apikit.Registry, d Dependencies) {
	failures := []string{"unauthorized", "not_permitted", "not_found", "invalid_request", "internal"}
	v1Route(registry, apikit.Metadata{Admission: laneAdmin, ID: "get_storage_status", Method: "GET", Path: "/v1/admin/storage/status", Summary: "Read upgrade copies, active storage destinations and reclaimable database space", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Cost: apikit.PageSized, Status: 200, Errors: failures}, func(ctx context.Context, r *http.Request, _ noBody) (persistence.StorageStatus, error) {
		out, err := persistence.ReadStorageStatus(ctx, d.DB)
		out.CacheDirectory = d.CacheDirectory
		if d.Backups != nil {
			out.BackupDirectory = d.Backups.Directory()
		}
		return out, err
	})
	v1Route(registry, apikit.Metadata{Admission: laneAdmin, ID: "delete_upgrade_copy", Method: "DELETE", Path: "/v1/admin/storage/upgrade-copies/{id}", Summary: "Delete one automatic pre-upgrade database copy", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Cost: apikit.Constant, Status: 204, Errors: failures}, func(ctx context.Context, r *http.Request, _ noBody) (noBody, error) {
		if _, err := d.owner(r); err != nil {
			return noBody{}, err
		}
		err := persistence.DeleteUpgradeCopy(ctx, d.DB, r.PathValue("id"))
		if errors.Is(err, os.ErrNotExist) {
			return noBody{}, &apierror.Error{Code: "not_found"}
		}
		if errors.Is(err, os.ErrInvalid) {
			return noBody{}, &apierror.Error{Code: "invalid_request"}
		}
		return noBody{}, err
	})
}
