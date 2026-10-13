package httpapi

import (
	"portico.local/apikit"
	catalog "portico.local/server/internal/catalog"
	metadata "portico.local/server/internal/metadata"
)

func registerMetadataHTTPContracts(registry *apikit.Registry) error {
	// internal/httpapi/metadata_editor.go:25
	if err := apikit.RegisterHTTP[metadata.BulkEdit, metadata.BulkReceipt](registry, apikit.Metadata{ID: "post_metadata_bulk", Method: "POST", Path: "/v1/metadata/bulk", Summary: "Post bulk", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/metadata_editor.go:65
	if err := apikit.RegisterHTTP[struct {
		File             string `json:"file"`
		Subject          string `json:"subject"`
		ExpectedRevision string `json:"expectedRevision"`
	}, metadata.RepairState](registry, apikit.Metadata{ID: "post_metadata_kind_id_art_role_upload", Method: "POST", Path: "/v1/metadata/{kind}/{id}/art/{role}/upload", Summary: "Post upload", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 11534336, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/metadata_editor.go:68
	if err := apikit.RegisterHTTP[struct{}, metadata.RepairState](registry, apikit.Metadata{ID: "delete_metadata_kind_id_art_role_upload_candidateId", Method: "DELETE", Path: "/v1/metadata/{kind}/{id}/art/{role}/upload/{candidateId}", Summary: "Delete candidateId", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"expectedRevision"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/metadata_repair.go:32
	if err := apikit.RegisterHTTP[struct{}, metadata.RepairState](registry, apikit.Metadata{ID: "get_metadata_kind_id_preview", Method: "GET", Path: "/v1/metadata/{kind}/{id}/preview", Summary: "Get preview", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/metadata_repair.go:32
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_metadata_kind_id_art_role", Method: "GET", Path: "/v1/metadata/{kind}/{id}/art/{role}", Summary: "Get role", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Media, Admission: laneMedia, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/musicbrainz.go:12
	if err := apikit.RegisterHTTP[metadata.LocalAudioPolicyChange, struct {
		EntityId string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_items_id_metadata_local_audio_policy", Method: "POST", Path: "/v1/items/{id}/metadata/local-audio/policy", Summary: "Post policy", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/screen_metadata.go:29
	if err := apikit.RegisterHTTP[metadata.ScreenSelection, struct {
		EntityId string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "put_shows_id_metadata_screen_season", Method: "PUT", Path: "/v1/shows/{id}/metadata/screen/season", Summary: "Put season", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/screen_metadata.go:29
	if err := apikit.RegisterHTTP[struct{}, struct {
		EntityId string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_metadata_agent", Method: "GET", Path: "/v1/libraries/{id}/metadata/agent", Summary: "Get agent", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/screen_metadata.go:29
	if err := apikit.RegisterHTTP[metadata.ScreenSelection, struct {
		EntityId string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "put_libraries_id_metadata_agent", Method: "PUT", Path: "/v1/libraries/{id}/metadata/agent", Summary: "Put agent", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/screen_metadata.go:29
	if err := apikit.RegisterHTTP[struct{}, struct {
		EntityId string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "get_library_kinds_kind_metadata_agents", Method: "GET", Path: "/v1/library-kinds/{kind}/metadata-agents", Summary: "Get metadata agents", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/screen_metadata.go:29
	if err := apikit.RegisterHTTP[struct{}, struct {
		EntityId string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_metadata_screen", Method: "GET", Path: "/v1/libraries/{id}/metadata/screen", Summary: "Get screen", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/screen_metadata.go:29
	if err := apikit.RegisterHTTP[metadata.ScreenSelection, struct {
		EntityId string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "put_libraries_id_metadata_screen", Method: "PUT", Path: "/v1/libraries/{id}/metadata/screen", Summary: "Put screen", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/tvdb.go:11
	if err := apikit.RegisterHTTP[struct{}, metadata.TVDBState](registry, apikit.Metadata{ID: "get_shows_id_metadata_tvdb", Method: "GET", Path: "/v1/shows/{id}/metadata/tvdb", Summary: "Get tvdb", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/tvdb.go:39
	if err := apikit.RegisterHTTP[metadata.TVDBSelection, struct {
		ShowId string `json:"showId"`
		Status string `json:"status"`
	}](registry, apikit.Metadata{ID: "put_shows_id_metadata_tvdb", Method: "PUT", Path: "/v1/shows/{id}/metadata/tvdb", Summary: "Put tvdb", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/tvdb.go:63
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64 "json:\"expectedRevision\""
	}, struct {
		ShowId string `json:"showId"`
		Status string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_shows_id_metadata_tvdb_retry", Method: "POST", Path: "/v1/shows/{id}/metadata/tvdb/retry", Summary: "Post retry", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[struct{}, metadata.MBState](registry, apikit.Metadata{ID: "get_albums_id_metadata_musicbrainz", Method: "GET", Path: "/v1/albums/{id}/metadata/musicbrainz", Summary: "Get musicbrainz", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[metadata.MBSelection, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "put_albums_id_metadata_musicbrainz", Method: "PUT", Path: "/v1/albums/{id}/metadata/musicbrainz", Summary: "Put musicbrainz", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64 `json:"expectedRevision"`
	}, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_albums_id_metadata_musicbrainz_retry", Method: "POST", Path: "/v1/albums/{id}/metadata/musicbrainz/retry", Summary: "Post retry", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64 `json:"expectedRevision"`
	}, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_albums_id_metadata_musicbrainz_search", Method: "POST", Path: "/v1/albums/{id}/metadata/musicbrainz/search", Summary: "Post search", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[metadata.MusicPolicyChange, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_albums_id_metadata_musicbrainz_policy", Method: "POST", Path: "/v1/albums/{id}/metadata/musicbrainz/policy", Summary: "Post policy", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[struct{}, metadata.MBState](registry, apikit.Metadata{ID: "get_items_id_metadata_musicbrainz", Method: "GET", Path: "/v1/items/{id}/metadata/musicbrainz", Summary: "Get musicbrainz", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[metadata.MBSelection, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "put_items_id_metadata_musicbrainz", Method: "PUT", Path: "/v1/items/{id}/metadata/musicbrainz", Summary: "Put musicbrainz", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64 `json:"expectedRevision"`
	}, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_items_id_metadata_musicbrainz_retry", Method: "POST", Path: "/v1/items/{id}/metadata/musicbrainz/retry", Summary: "Post retry", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64 `json:"expectedRevision"`
	}, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_items_id_metadata_musicbrainz_search", Method: "POST", Path: "/v1/items/{id}/metadata/musicbrainz/search", Summary: "Post search", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[metadata.MusicPolicyChange, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_items_id_metadata_musicbrainz_policy", Method: "POST", Path: "/v1/items/{id}/metadata/musicbrainz/policy", Summary: "Post policy", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[metadata.ScreenSelection, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "put_items_id_metadata_screen", Method: "PUT", Path: "/v1/items/{id}/metadata/screen", Summary: "Put screen", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[metadata.ScreenSearch, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_items_id_metadata_screen_search", Method: "POST", Path: "/v1/items/{id}/metadata/screen/search", Summary: "Post search", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64 `json:"expectedRevision"`
	}, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_items_id_metadata_screen_retry", Method: "POST", Path: "/v1/items/{id}/metadata/screen/retry", Summary: "Post retry", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[metadata.ScreenSelection, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "put_shows_id_metadata_screen", Method: "PUT", Path: "/v1/shows/{id}/metadata/screen", Summary: "Put screen", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[metadata.ScreenSearch, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_shows_id_metadata_screen_search", Method: "POST", Path: "/v1/shows/{id}/metadata/screen/search", Summary: "Post search", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64 `json:"expectedRevision"`
	}, struct {
		EntityID string `json:"entityId"`
		Status   string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_shows_id_metadata_screen_retry", Method: "POST", Path: "/v1/shows/{id}/metadata/screen/retry", Summary: "Post retry", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[struct{}, metadata.RepairState](registry, apikit.Metadata{ID: "get_metadata_kind_id", Method: "GET", Path: "/v1/metadata/{kind}/{id}", Summary: "Get id", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[metadata.RepairCommand, metadata.RepairState](registry, apikit.Metadata{ID: "post_metadata_kind_id", Method: "POST", Path: "/v1/metadata/{kind}/{id}", Summary: "Post id", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[struct{}, catalog.ManualMetadata](registry, apikit.Metadata{ID: "get_items_id_metadata_manual", Method: "GET", Path: "/v1/items/{id}/metadata/manual", Summary: "Get manual", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/manual_metadata.go:53
	if err := apikit.RegisterHTTP[catalog.ManualMetadataMutation, catalog.ManualMetadata](registry, apikit.Metadata{ID: "patch_items_id_metadata_manual", Method: "PATCH", Path: "/v1/items/{id}/metadata/manual", Summary: "Patch manual", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 262144, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	return nil
}
