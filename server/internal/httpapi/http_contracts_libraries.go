package httpapi

import (
	"portico.local/apikit"
	catalog "portico.local/server/internal/catalog"
	ingestion "portico.local/server/internal/ingestion"
	mounts "portico.local/server/internal/mounts"
	remotemedia "portico.local/server/internal/remotemedia"
	remotesources "portico.local/server/internal/remotesources"
)

func registerLibrariesHTTPContracts(registry *apikit.Registry) error {
	// internal/httpapi/admin.go:62
	if err := apikit.RegisterHTTP[struct{}, catalog.AdminLibraries](registry, apikit.Metadata{ID: "get_admin_libraries", Method: "GET", Path: "/v1/admin/libraries", Summary: "Get libraries", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin.go:62
	if err := apikit.RegisterHTTP[struct{}, catalog.AdminJobs](registry, apikit.Metadata{ID: "get_admin_jobs", Method: "GET", Path: "/v1/admin/jobs", Summary: "Get jobs", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin.go:62
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope   catalog.AdminScope   `json:"scope"`
		Library catalog.AdminLibrary `json:"library"`
	}](registry, apikit.Metadata{ID: "get_admin_libraries_id", Method: "GET", Path: "/v1/admin/libraries/{id}", Summary: "Get id", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin.go:94
	if err := apikit.RegisterHTTP[struct {
		LibraryIDs []string "json:\"libraryIds\""
	}, struct {
		LibraryIds []string `json:"libraryIds"`
	}](registry, apikit.Metadata{ID: "put_admin_libraries_order", Method: "PUT", Path: "/v1/admin/libraries/order", Summary: "Put order", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin.go:114
	if err := apikit.RegisterHTTP[catalog.ShowSettings, catalog.ShowSettings](registry, apikit.Metadata{ID: "put_admin_libraries_id_shows_show_settings", Method: "PUT", Path: "/v1/admin/libraries/{id}/shows/{show}/settings", Summary: "Put settings", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin.go:132
	if err := apikit.RegisterHTTP[struct {
		Name             string "json:\"name\""
		ExpectedRevision int64  "json:\"expectedRevision\""
	}, struct {
		Scope   catalog.AdminScope   `json:"scope"`
		Library catalog.AdminLibrary `json:"library"`
	}](registry, apikit.Metadata{ID: "patch_admin_libraries_id", Method: "PATCH", Path: "/v1/admin/libraries/{id}", Summary: "Patch id", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/inventory.go:10
	if err := apikit.RegisterHTTP[struct{}, struct {
		ServerId  string                  `json:"serverId"`
		Inventory catalog.InventoryStatus `json:"inventory"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_inventory_status", Method: "GET", Path: "/v1/libraries/{id}/inventory-status", Summary: "Get inventory status", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/inventory.go:36
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope     catalog.AdminScope      `json:"scope"`
		LibraryId string                  `json:"libraryId"`
		Revision  int64                   `json:"revision"`
		Sources   []catalog.LibrarySource `json:"sources"`
		Policy    catalog.ScanPolicy      `json:"policy"`
	}](registry, apikit.Metadata{ID: "get_admin_libraries_id_inventory_config", Method: "GET", Path: "/v1/admin/libraries/{id}/inventory-config", Summary: "Get inventory config", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/inventory.go:74
	if err := apikit.RegisterHTTP[catalog.SourceSettings, catalog.LibrarySource](registry, apikit.Metadata{ID: "post_admin_libraries_id_sources", Method: "POST", Path: "/v1/admin/libraries/{id}/sources", Summary: "Post sources", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/inventory.go:74
	if err := apikit.RegisterHTTP[catalog.SourceSettings, catalog.LibrarySource](registry, apikit.Metadata{ID: "patch_admin_libraries_id_sources_source", Method: "PATCH", Path: "/v1/admin/libraries/{id}/sources/{source}", Summary: "Patch source", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/inventory.go:74
	if err := apikit.RegisterHTTP[catalog.SourceSettings, catalog.LibrarySource](registry, apikit.Metadata{ID: "post_admin_libraries_id_sources_source_check", Method: "POST", Path: "/v1/admin/libraries/{id}/sources/{source}/check", Summary: "Post check", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/inventory.go:74
	if err := apikit.RegisterHTTP[catalog.SourceSettings, catalog.LibrarySource](registry, apikit.Metadata{ID: "delete_admin_libraries_id_sources_source", Method: "DELETE", Path: "/v1/admin/libraries/{id}/sources/{source}", Summary: "Delete source", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/inventory.go:74
	if err := apikit.RegisterHTTP[catalog.SourceSettings, catalog.LibrarySource](registry, apikit.Metadata{ID: "post_admin_libraries_id_sources_source_scans", Method: "POST", Path: "/v1/admin/libraries/{id}/sources/{source}/scans", Summary: "Post scans", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/inventory.go:74
	if err := apikit.RegisterHTTP[catalog.SourceSettings, catalog.LibrarySource](registry, apikit.Metadata{ID: "put_admin_libraries_id_scan_policy", Method: "PUT", Path: "/v1/admin/libraries/{id}/scan-policy", Summary: "Put scan policy", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/inventory.go:74
	if err := apikit.RegisterHTTP[catalog.SourceSettings, catalog.LibrarySource](registry, apikit.Metadata{ID: "post_admin_ingestion_jobs_id_action", Method: "POST", Path: "/v1/admin/ingestion/jobs/{id}/{action}", Summary: "Post action", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/inventory.go:74
	if err := apikit.RegisterHTTP[catalog.SourceSettings, catalog.LibrarySource](registry, apikit.Metadata{ID: "post_admin_libraries_id_inventory_object_action", Method: "POST", Path: "/v1/admin/libraries/{id}/inventory/{object}/{action}", Summary: "Post action", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/mounts.go:52
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope          catalog.AdminScope `json:"scope"`
		Revision       int64              `json:"revision"`
		Limits         map[string]int     `json:"limits"`
		Mounts         []mounts.Mount     `json:"mounts"`
		MountSupported bool               `json:"mountSupported"`
	}](registry, apikit.Metadata{ID: "get_storage_mounts", Method: "GET", Path: "/v1/storage/mounts", Summary: "Get mounts", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/mounts.go:87
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope catalog.AdminScope "json:\"scope\""
		mounts.Receipt
	}](registry, apikit.Metadata{ID: "get_storage_operations_id", Method: "GET", Path: "/v1/storage/operations/{id}", Summary: "Get id", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/mounts.go:113
	if err := apikit.RegisterHTTP[mounts.Command, struct {
		Scope catalog.AdminScope "json:\"scope\""
		mounts.Receipt
	}](registry, apikit.Metadata{ID: "post_storage_mounts", Method: "POST", Path: "/v1/storage/mounts", Summary: "Post mounts", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/mounts.go:113
	if err := apikit.RegisterHTTP[mounts.Command, struct {
		Scope catalog.AdminScope "json:\"scope\""
		mounts.Receipt
	}](registry, apikit.Metadata{ID: "post_storage_mounts_id_actions", Method: "POST", Path: "/v1/storage/mounts/{id}/actions", Summary: "Post actions", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/mounts.go:113
	if err := apikit.RegisterHTTP[mounts.Command, struct {
		Scope catalog.AdminScope "json:\"scope\""
		mounts.Receipt
	}](registry, apikit.Metadata{ID: "delete_storage_mounts_id", Method: "DELETE", Path: "/v1/storage/mounts/{id}", Summary: "Delete id", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/remote_sources.go:50
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope          catalog.AdminScope     `json:"scope"`
		Sources        []remotesources.Source `json:"sources"`
		MountSupported bool                   `json:"mountSupported"`
	}](registry, apikit.Metadata{ID: "get_storage_sources", Method: "GET", Path: "/v1/storage/sources", Summary: "Get sources", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/remote_sources.go:71
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope   catalog.AdminScope    `json:"scope"`
		Receipt remotesources.Receipt `json:"receipt"`
	}](registry, apikit.Metadata{ID: "get_storage_source_operations_id", Method: "GET", Path: "/v1/storage/source-operations/{id}", Summary: "Get id", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/remote_sources.go:90
	if err := apikit.RegisterHTTP[remotesources.DAVInput, struct {
		Scope   catalog.AdminScope    `json:"scope"`
		Receipt remotesources.Receipt `json:"receipt"`
	}](registry, apikit.Metadata{ID: "post_storage_sources_webdav", Method: "POST", Path: "/v1/storage/sources/webdav", Summary: "Post webdav", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/remote_sources.go:90
	if err := apikit.RegisterHTTP[remotesources.DAVInput, struct {
		Scope   catalog.AdminScope    `json:"scope"`
		Receipt remotesources.Receipt `json:"receipt"`
	}](registry, apikit.Metadata{ID: "put_storage_sources_webdav_id", Method: "PUT", Path: "/v1/storage/sources/webdav/{id}", Summary: "Put id", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/remote_sources.go:90
	if err := apikit.RegisterHTTP[remotesources.DAVInput, struct {
		Scope   catalog.AdminScope    `json:"scope"`
		Receipt remotesources.Receipt `json:"receipt"`
	}](registry, apikit.Metadata{ID: "delete_storage_sources_webdav_id", Method: "DELETE", Path: "/v1/storage/sources/webdav/{id}", Summary: "Delete id", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/remote_sources.go:90
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope   catalog.AdminScope    `json:"scope"`
		Receipt remotesources.Receipt `json:"receipt"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_network_roots", Method: "GET", Path: "/v1/libraries/{id}/network-roots", Summary: "Get network roots", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/remote_sources.go:90
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope   catalog.AdminScope    `json:"scope"`
		Receipt remotesources.Receipt `json:"receipt"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_strm_analysis_policy", Method: "GET", Path: "/v1/libraries/{id}/strm-analysis-policy", Summary: "Get strm analysis policy", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/remote_sources.go:90
	if err := apikit.RegisterHTTP[remotesources.DAVInput, struct {
		Scope   catalog.AdminScope    `json:"scope"`
		Receipt remotesources.Receipt `json:"receipt"`
	}](registry, apikit.Metadata{ID: "put_libraries_id_strm_analysis_policy", Method: "PUT", Path: "/v1/libraries/{id}/strm-analysis-policy", Summary: "Put strm analysis policy", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/remote_sources.go:90
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope   catalog.AdminScope    `json:"scope"`
		Receipt remotesources.Receipt `json:"receipt"`
	}](registry, apikit.Metadata{ID: "get_items_id_source_availability", Method: "GET", Path: "/v1/items/{id}/source-availability", Summary: "Get source availability", Access: apikit.Owner, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:880
	if err := apikit.RegisterHTTP[struct{}, struct {
		Items []catalog.Library `json:"items"`
	}](registry, apikit.Metadata{ID: "get_libraries", Method: "GET", Path: "/v1/libraries", Summary: "Get libraries", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:893
	if err := apikit.RegisterHTTP[struct {
		Name             string "json:\"name\""
		Kind             string "json:\"kind\""
		Path             string "json:\"path\""
		MetadataAgent    string "json:\"metadataAgent,omitempty\""
		MetadataLanguage string "json:\"metadataLanguage,omitempty\""
	}, catalog.Library](registry, apikit.Metadata{ID: "post_libraries", Method: "POST", Path: "/v1/libraries", Summary: "Post libraries", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 201, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:959
	if err := apikit.RegisterHTTP[struct {
		Roots            []string "json:\"roots\""
		ExpectedRevision *int64   "json:\"expectedRevision,omitempty\""
	}, struct {
		Approvals []remotemedia.Approval `json:"approvals"`
	}](registry, apikit.Metadata{ID: "post_libraries_id_network_roots", Method: "POST", Path: "/v1/libraries/{id}/network-roots", Summary: "Post network roots", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:980
	if err := apikit.RegisterHTTP[struct{}, ingestion.Job](registry, apikit.Metadata{ID: "post_libraries_id_scans", Method: "POST", Path: "/v1/libraries/{id}/scans", Summary: "Post scans", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 202, Query: []string{"mode"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:993
	if err := apikit.RegisterHTTP[struct{}, ingestion.Job](registry, apikit.Metadata{ID: "get_admin_ingestion_jobs_id", Method: "GET", Path: "/v1/admin/ingestion/jobs/{id}", Summary: "Get id", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:1005
	if err := apikit.RegisterHTTP[struct{}, struct{}](registry, apikit.Metadata{ID: "delete_admin_ingestion_jobs_id", Method: "DELETE", Path: "/v1/admin/ingestion/jobs/{id}", Summary: "Delete id", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	return nil
}
