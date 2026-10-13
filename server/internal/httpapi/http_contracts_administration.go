package httpapi

import (
	"portico.local/apikit"
	access "portico.local/server/internal/access"
	administration "portico.local/server/internal/administration"
	diagnostics "portico.local/server/internal/diagnostics"
	hosted "portico.local/server/internal/hosted"
	operations "portico.local/server/internal/operations"
	persistence "portico.local/server/internal/persistence"
	servicelog "portico.local/server/internal/servicelog"
	telemetry "portico.local/server/internal/telemetry"
)

func registerAdministrationHTTPContracts(registry *apikit.Registry) error {
	// internal/httpapi/admin_access.go:27
	if err := apikit.RegisterHTTP[struct{}, access.MemberPage](registry, apikit.Metadata{ID: "get_admin_access_members", Method: "GET", Path: "/v1/admin/access/members", Summary: "Get members", Access: apikit.Admin, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, AdminScope: "access", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_access.go:40
	if err := apikit.RegisterHTTP[access.RoleChange, access.Member](registry, apikit.Metadata{ID: "put_admin_access_members_id_role", Method: "PUT", Path: "/v1/admin/access/members/{id}/role", Summary: "Put role", Access: apikit.Admin, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, AdminScope: "access", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_access.go:58
	if err := apikit.RegisterHTTP[struct{}, access.LimitsDocument](registry, apikit.Metadata{ID: "get_admin_access_members_id_limits", Method: "GET", Path: "/v1/admin/access/members/{id}/limits", Summary: "Get limits", Access: apikit.Admin, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, AdminScope: "access", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_access.go:71
	if err := apikit.RegisterHTTP[access.LimitsChange, access.LimitsDocument](registry, apikit.Metadata{ID: "put_admin_access_members_id_limits", Method: "PUT", Path: "/v1/admin/access/members/{id}/limits", Summary: "Put limits", Access: apikit.Admin, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, AdminScope: "access", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_access.go:89
	if err := apikit.RegisterHTTP[struct{}, access.InvitationPage](registry, apikit.Metadata{ID: "get_admin_access_invitations", Method: "GET", Path: "/v1/admin/access/invitations", Summary: "Get invitations", Access: apikit.Admin, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit", "state"}, AdminScope: "access", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_access.go:103
	if err := apikit.RegisterHTTP[access.InvitationRequest, access.Invitation](registry, apikit.Metadata{ID: "post_admin_access_invitations", Method: "POST", Path: "/v1/admin/access/invitations", Summary: "Post invitations", Access: apikit.Admin, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 201, AdminScope: "access", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_access.go:121
	if err := apikit.RegisterHTTP[access.InvitationRevoke, access.Invitation](registry, apikit.Metadata{ID: "post_admin_access_invitations_id_revoke", Method: "POST", Path: "/v1/admin/access/invitations/{id}/revoke", Summary: "Post revoke", Access: apikit.Admin, Requires: apikit.ScopeAdmin, Lane: apikit.Security, Admission: laneSecurityFence, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, AdminScope: "access", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_access.go:139
	if err := apikit.RegisterHTTP[struct {
		access.Acceptance
		PorticoIdentity *hosted.Signed "json:\"porticoIdentity,omitempty\""
	}, access.Member](registry, apikit.Metadata{ID: "post_access_invitations_accept", Method: "POST", Path: "/v1/access/invitations/accept", Summary: "Post accept", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 201, Query: []string{"X-Portico-Installation-Id"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_concurrency.go:182
	if err := apikit.RegisterHTTP[struct{}, ConcurrencyDiagnostics](registry, apikit.Metadata{ID: "get_admin_diagnostics_concurrency", Method: "GET", Path: "/v1/admin/diagnostics/concurrency", Summary: "Get concurrency", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_concurrency.go:196
	if err := apikit.RegisterHTTP[struct{}, IntegrityDiagnostics](registry, apikit.Metadata{ID: "post_admin_diagnostics_integrity", Method: "POST", Path: "/v1/admin/diagnostics/integrity", Summary: "Post integrity", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminLong, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_concurrency.go:208
	if err := apikit.RegisterHTTP[struct{}, struct {
		Repair     persistence.OrphanRepairReport `json:"repair"`
		Incomplete bool                           `json:"incomplete"`
	}](registry, apikit.Metadata{ID: "post_admin_diagnostics_integrity_repair", Method: "POST", Path: "/v1/admin/diagnostics/integrity/repair", Summary: "Post repair", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminLong, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_concurrency.go:226
	if err := apikit.RegisterHTTP[struct{}, struct {
		Ready  bool   `json:"ready"`
		Status string `json:"status"`
	}](registry, apikit.Metadata{ID: "get_readiness", Method: "GET", Path: "/v1/readiness", Summary: "Get readiness", Access: apikit.Public, Requires: apikit.ScopeOpen, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_diagnostics.go:44
	if err := apikit.RegisterHTTP[struct{}, servicelog.Result](registry, apikit.Metadata{ID: "get_admin_logs", Method: "GET", Path: "/v1/admin/logs", Summary: "Get logs", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"category", "cursor", "level", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_diagnostics.go:66
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_admin_logs_events", Method: "GET", Path: "/v1/admin/logs/events", Summary: "Get events", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Realtime, Admission: laneRealtime, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"Last-Event-ID", "level"}, ResponseContentType: "text/event-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_diagnostics.go:67
	if err := apikit.RegisterHTTP[struct{}, LogSettings](registry, apikit.Metadata{ID: "get_admin_logs_settings", Method: "GET", Path: "/v1/admin/logs/settings", Summary: "Get settings", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_diagnostics.go:79
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64                     "json:\"expectedRevision\""
		OperationID      string                    "json:\"operationId\""
		Level            string                    "json:\"logLevel\""
		Retention        []operations.LogRetention "json:\"retention\""
	}, LogSettings](registry, apikit.Metadata{ID: "patch_admin_logs_settings", Method: "PATCH", Path: "/v1/admin/logs/settings", Summary: "Patch settings", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_diagnostics.go:110
	if err := apikit.RegisterHTTP[struct {
		Minutes     int    "json:\"minutes\""
		OperationID string "json:\"operationId\""
	}, LogSettings](registry, apikit.Metadata{ID: "post_admin_logs_debug_window", Method: "POST", Path: "/v1/admin/logs/debug-window", Summary: "Post debug window", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_diagnostics.go:148
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_admin_diagnostics_bundle", Method: "GET", Path: "/v1/admin/diagnostics/bundle", Summary: "Get bundle", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 0, Status: 200, ResponseContentType: "application/zip", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/admin_pprof.go:28
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_admin_diagnostics_pprof_profile", Method: "GET", Path: "/v1/admin/diagnostics/pprof/{profile}", Summary: "Get profile", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 0, Status: 200, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:179
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.DeletePreview]](registry, apikit.Metadata{ID: "post_items_id_delete_preview", Method: "POST", Path: "/v1/items/{id}/delete/preview", Summary: "Post preview", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:185
	if err := apikit.RegisterHTTP[struct {
		ItemIDs []string "json:\"itemIds\""
	}, LiveOwnerResult[administration.DeletePreview]](registry, apikit.Metadata{ID: "post_admin_media_delete_preview", Method: "POST", Path: "/v1/admin/media/delete/preview", Summary: "Post preview", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:194
	if err := apikit.RegisterHTTP[administration.DeleteRequest, LiveOwnerResult[administration.DeleteResult]](registry, apikit.Metadata{ID: "post_items_id_delete", Method: "POST", Path: "/v1/items/{id}/delete", Summary: "Post delete", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:206
	if err := apikit.RegisterHTTP[administration.DeleteRequest, LiveOwnerResult[administration.DeleteResult]](registry, apikit.Metadata{ID: "post_admin_media_delete", Method: "POST", Path: "/v1/admin/media/delete", Summary: "Post delete", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:218
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.FilesystemPage]](registry, apikit.Metadata{ID: "get_admin_filesystem", Method: "GET", Path: "/v1/admin/filesystem", Summary: "Get filesystem", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:232
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.LibrarySettingsIndex]](registry, apikit.Metadata{ID: "get_admin_library_settings", Method: "GET", Path: "/v1/admin/library-settings", Summary: "Get library settings", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:243
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.AnalysisMatrix]](registry, apikit.Metadata{ID: "get_admin_analysis_operations", Method: "GET", Path: "/v1/admin/analysis-operations", Summary: "Get analysis operations", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:249
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[struct {
		Providers []administration.MetadataProvider `json:"providers"`
	}]](registry, apikit.Metadata{ID: "get_admin_metadata_providers", Method: "GET", Path: "/v1/admin/metadata-providers", Summary: "Get metadata providers", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:262
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.ChannelMapPage]](registry, apikit.Metadata{ID: "get_admin_live_sources_id_channel_map", Method: "GET", Path: "/v1/admin/live-sources/{id}/channel-map", Summary: "Get channel map", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:273
	if err := apikit.RegisterHTTP[administration.ChannelMapChange, LiveOwnerResult[administration.ChannelMapPage]](registry, apikit.Metadata{ID: "put_admin_live_sources_id_channel_map", Method: "PUT", Path: "/v1/admin/live-sources/{id}/channel-map", Summary: "Put channel map", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:280
	if err := apikit.RegisterHTTP[struct {
		OperationID string "json:\"operationId\""
	}, LiveOwnerResult[administration.LogoImportResult]](registry, apikit.Metadata{ID: "post_admin_live_sources_id_logos", Method: "POST", Path: "/v1/admin/live-sources/{id}/logos", Summary: "Post logos", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:289
	if err := apikit.RegisterHTTP[struct {
		WindowSeconds int "json:\"windowSeconds\""
	}, LiveOwnerResult[administration.TunerDiscovery]](registry, apikit.Metadata{ID: "post_admin_live_sources_discover", Method: "POST", Path: "/v1/admin/live-sources/discover", Summary: "Post discover", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:308
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.DVRDocument]](registry, apikit.Metadata{ID: "get_admin_dvr_settings", Method: "GET", Path: "/v1/admin/dvr/settings", Summary: "Get settings", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:314
	if err := apikit.RegisterHTTP[administration.Change[administration.DVRDefaults], LiveOwnerResult[administration.DVRDocument]](registry, apikit.Metadata{ID: "put_admin_dvr_settings", Method: "PUT", Path: "/v1/admin/dvr/settings", Summary: "Put settings", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:322
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.RecordingGroupPage]](registry, apikit.Metadata{ID: "get_admin_dvr_recording_groups", Method: "GET", Path: "/v1/admin/dvr/recording-groups", Summary: "Get recording groups", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:333
	if err := apikit.RegisterHTTP[administration.RecordingGroupChange, LiveOwnerResult[administration.RecordingGroup]](registry, apikit.Metadata{ID: "post_admin_dvr_recording_groups", Method: "POST", Path: "/v1/admin/dvr/recording-groups", Summary: "Post recording groups", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:340
	if err := apikit.RegisterHTTP[administration.RecordingGroupChange, LiveOwnerResult[administration.RecordingGroup]](registry, apikit.Metadata{ID: "put_admin_dvr_recording_groups_id", Method: "PUT", Path: "/v1/admin/dvr/recording-groups/{id}", Summary: "Put id", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:347
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[struct {
		Deleted bool   `json:"deleted"`
		ID      string `json:"id"`
	}]](registry, apikit.Metadata{ID: "delete_admin_dvr_recording_groups_id", Method: "DELETE", Path: "/v1/admin/dvr/recording-groups/{id}", Summary: "Delete id", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:369
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.CriteriaPage]](registry, apikit.Metadata{ID: "get_admin_library_channels_criteria", Method: "GET", Path: "/v1/admin/library-channels/criteria", Summary: "Get criteria", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:380
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.BlockPresetPage]](registry, apikit.Metadata{ID: "get_admin_library_channels_block_presets", Method: "GET", Path: "/v1/admin/library-channels/block-presets", Summary: "Get block presets", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:391
	if err := apikit.RegisterHTTP[administration.BlockPresetChange, LiveOwnerResult[administration.BlockPreset]](registry, apikit.Metadata{ID: "post_admin_library_channels_block_presets", Method: "POST", Path: "/v1/admin/library-channels/block-presets", Summary: "Post block presets", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:398
	if err := apikit.RegisterHTTP[administration.BlockPresetChange, LiveOwnerResult[administration.BlockPreset]](registry, apikit.Metadata{ID: "put_admin_library_channels_block_presets_id", Method: "PUT", Path: "/v1/admin/library-channels/block-presets/{id}", Summary: "Put id", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:405
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[struct {
		Deleted bool   `json:"deleted"`
		ID      string `json:"id"`
	}]](registry, apikit.Metadata{ID: "delete_admin_library_channels_block_presets_id", Method: "DELETE", Path: "/v1/admin/library-channels/block-presets/{id}", Summary: "Delete id", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:419
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.ChannelHealth]](registry, apikit.Metadata{ID: "get_admin_library_channels_id_health", Method: "GET", Path: "/v1/admin/library-channels/{id}/health", Summary: "Get health", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:425
	if err := apikit.RegisterHTTP[struct {
		File             string `json:"file"`
		ExpectedRevision string `json:"expectedRevision"`
		OperationID      string `json:"operationId"`
	}, LiveOwnerResult[administration.ChannelLogo]](registry, apikit.Metadata{ID: "post_admin_library_channels_id_logo", Method: "POST", Path: "/v1/admin/library-channels/{id}/logo", Summary: "Post logo", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 11534336, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:439
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_library_channels_id_logo", Method: "GET", Path: "/v1/library-channels/{id}/logo", Summary: "Get logo", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"If-None-Match"}, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:504
	if err := apikit.RegisterHTTP[struct{}, LiveOwnerResult[administration.StorageReport]](registry, apikit.Metadata{ID: "get_admin_storage_usage", Method: "GET", Path: "/v1/admin/storage-usage", Summary: "Get storage usage", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/administration.go:510
	if err := apikit.RegisterHTTP[administration.CleanupRequest, LiveOwnerResult[administration.CleanupResult]](registry, apikit.Metadata{ID: "post_admin_storage_usage_category_cleanup", Method: "POST", Path: "/v1/admin/storage-usage/{category}/cleanup", Summary: "Post cleanup", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/console.go:119
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[operations.PreferenceSnapshot]](registry, apikit.Metadata{ID: "get_preferences", Method: "GET", Path: "/v1/preferences", Summary: "Get preferences", Access: apikit.Owner, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/console.go:119
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[operations.PreferenceSnapshot]](registry, apikit.Metadata{ID: "patch_preferences", Method: "PATCH", Path: "/v1/preferences", Summary: "Patch preferences", Access: apikit.Owner, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/console.go:119
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[map[string]bool]](registry, apikit.Metadata{ID: "post_notifications_id_read", Method: "POST", Path: "/v1/notifications/{id}/read", Summary: "Post read", Access: apikit.Owner, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneRealtime, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/console.go:119
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[operations.StreamOptions]](registry, apikit.Metadata{ID: "get_admin_console_stream_options", Method: "GET", Path: "/v1/admin/console/stream-options", Summary: "Get stream options", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/console.go:119
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[[]operations.StreamOption]](registry, apikit.Metadata{ID: "get_admin_console_accounts", Method: "GET", Path: "/v1/admin/console/accounts", Summary: "Get accounts", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/console.go:119
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[map[string]bool]](registry, apikit.Metadata{ID: "post_admin_console_alerts_id_acknowledge", Method: "POST", Path: "/v1/admin/console/alerts/{id}/acknowledge", Summary: "Post acknowledge", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/console.go:119
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[[]operations.JobKind]](registry, apikit.Metadata{ID: "get_admin_console_job_kinds", Method: "GET", Path: "/v1/admin/console/job-kinds", Summary: "Get job kinds", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/notifications_routes.go:60
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[operations.NoticeVocabulary]](registry, apikit.Metadata{ID: "get_notifications_capabilities", Method: "GET", Path: "/v1/notifications/capabilities", Summary: "Get capabilities", Access: apikit.Owner, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneRealtime, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/notifications_routes.go:60
	if err := apikit.RegisterHTTP[operations.NoticeBatch, projectedConsoleEnvelope[operations.NoticeBatchResult]](registry, apikit.Metadata{ID: "post_notifications_inbox_actions", Method: "POST", Path: "/v1/notifications/inbox/actions", Summary: "Post actions", Access: apikit.Owner, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneRealtime, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/notifications_routes.go:60
	if err := apikit.RegisterHTTP[operations.NoticeBroadcast, projectedConsoleEnvelope[operations.NoticeBroadcastResult]](registry, apikit.Metadata{ID: "post_admin_notifications_broadcast", Method: "POST", Path: "/v1/admin/notifications/broadcast", Summary: "Post broadcast", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/notifications_routes.go:60
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[operations.NotificationSettings]](registry, apikit.Metadata{ID: "get_admin_notifications_settings", Method: "GET", Path: "/v1/admin/notifications/settings", Summary: "Get settings", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/notifications_routes.go:60
	if err := apikit.RegisterHTTP[operations.NotificationSettingsChange, projectedConsoleEnvelope[operations.NotificationSettings]](registry, apikit.Metadata{ID: "patch_admin_notifications_settings", Method: "PATCH", Path: "/v1/admin/notifications/settings", Summary: "Patch settings", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/notifications_routes.go:60
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[operations.FeedbackCapabilities]](registry, apikit.Metadata{ID: "get_feedback_capabilities", Method: "GET", Path: "/v1/feedback/capabilities", Summary: "Get capabilities", Access: apikit.Owner, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/operations.go:16
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope map[string]string `json:"scope"`
		Panel operations.Panel  `json:"panel"`
	}](registry, apikit.Metadata{ID: "get_admin_operations_panel", Method: "GET", Path: "/v1/admin/operations/{panel}", Summary: "Get panel", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/support.go:20
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope  projectedScope     `json:"scope"`
		Report diagnostics.Report `json:"report"`
	}](registry, apikit.Metadata{ID: "get_admin_support_report", Method: "GET", Path: "/v1/admin/support-report", Summary: "Get support report", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/telemetry_routes.go:42
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[telemetry.Report]](registry, apikit.Metadata{ID: "get_admin_transcode_capacity", Method: "GET", Path: "/v1/admin/transcode/capacity", Summary: "Get capacity", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/telemetry_routes.go:42
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[telemetry.Reading]](registry, apikit.Metadata{ID: "get_admin_telemetry", Method: "GET", Path: "/v1/admin/telemetry", Summary: "Get telemetry", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"window"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/telemetry_routes.go:42
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[telemetry.Sample]](registry, apikit.Metadata{ID: "get_admin_telemetry_now", Method: "GET", Path: "/v1/admin/telemetry/now", Summary: "Get now", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/telemetry_routes.go:42
	if err := apikit.RegisterHTTP[struct{}, projectedConsoleEnvelope[struct {
		Items []telemetry.Item `json:"items"`
	}]](registry, apikit.Metadata{ID: "get_admin_attention", Method: "GET", Path: "/v1/admin/attention", Summary: "Get attention", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/telemetry_routes.go:42
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_admin_playback_history_csv", Method: "GET", Path: "/v1/admin/playback/history.csv", Summary: "Get history.csv", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Media, Admission: lanePlayback, Cost: apikit.Constant, BodyLimit: 0, Status: 200, ResponseContentType: "text/csv", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	return nil
}
