package httpapi

import (
	"portico.local/apikit"
	catalog "portico.local/server/internal/catalog"
)

func registerPersonalHTTPContracts(registry *apikit.Registry) error {
	// internal/httpapi/discovery.go:144
	if err := apikit.RegisterHTTP[struct{}, struct {
		Categories []catalog.Category `json:"categories"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_categories", Method: "GET", Path: "/v1/libraries/{id}/categories", Summary: "Get categories", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/discovery.go:166
	if err := apikit.RegisterHTTP[struct{}, struct {
		Collections []catalog.Collection `json:"collections"`
		NextCursor  string               `json:"nextCursor"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_collections", Method: "GET", Path: "/v1/libraries/{id}/collections", Summary: "Get collections", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/discovery.go:189
	if err := apikit.RegisterHTTP[struct {
		Name string "json:\"name\""
	}, catalog.Collection](registry, apikit.Metadata{ID: "post_libraries_id_collections", Method: "POST", Path: "/v1/libraries/{id}/collections", Summary: "Post collections", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 201, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/discovery.go:239
	if err := apikit.RegisterHTTP[struct{}, catalog.Collection](registry, apikit.Metadata{ID: "get_collections_id", Method: "GET", Path: "/v1/collections/{id}", Summary: "Get id", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/discovery.go:267
	if err := apikit.RegisterHTTP[struct {
		Name string "json:\"name\""
	}, catalog.Collection](registry, apikit.Metadata{ID: "patch_collections_id", Method: "PATCH", Path: "/v1/collections/{id}", Summary: "Patch id", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/discovery.go:286
	if err := apikit.RegisterHTTP[struct{}, struct{}](registry, apikit.Metadata{ID: "delete_collections_id", Method: "DELETE", Path: "/v1/collections/{id}", Summary: "Delete id", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/discovery.go:298
	if err := apikit.RegisterHTTP[struct{}, struct{}](registry, apikit.Metadata{ID: "put_collections_id_items_itemId", Method: "PUT", Path: "/v1/collections/{id}/items/{itemId}", Summary: "Put itemId", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/discovery.go:298
	if err := apikit.RegisterHTTP[struct{}, struct{}](registry, apikit.Metadata{ID: "delete_collections_id_items_itemId", Method: "DELETE", Path: "/v1/collections/{id}/items/{itemId}", Summary: "Delete itemId", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playlists.go:28
	if err := apikit.RegisterHTTP[struct{}, catalog.Playlist](registry, apikit.Metadata{ID: "get_playlists_id", Method: "GET", Path: "/v1/playlists/{id}", Summary: "Get id", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playlists.go:50
	if err := apikit.RegisterHTTP[struct{}, catalog.ContentEnvelope](registry, apikit.Metadata{ID: "get_playlists_id_content", Method: "GET", Path: "/v1/playlists/{id}/content", Summary: "Get content", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playlists.go:93
	if err := apikit.RegisterHTTP[struct{}, catalog.PlaylistCandidates](registry, apikit.Metadata{ID: "get_playlists_id_share_candidates", Method: "GET", Path: "/v1/playlists/{id}/share-candidates", Summary: "Get share candidates", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playlists.go:131
	if err := apikit.RegisterHTTP[catalog.PlaylistMutation, catalog.PlaylistReceipt](registry, apikit.Metadata{ID: "post_playlists", Method: "POST", Path: "/v1/playlists", Summary: "Post playlists", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playlists.go:131
	if err := apikit.RegisterHTTP[catalog.PlaylistMutation, catalog.PlaylistReceipt](registry, apikit.Metadata{ID: "patch_playlists_id", Method: "PATCH", Path: "/v1/playlists/{id}", Summary: "Patch id", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playlists.go:131
	if err := apikit.RegisterHTTP[catalog.PlaylistMutation, catalog.PlaylistReceipt](registry, apikit.Metadata{ID: "delete_playlists_id", Method: "DELETE", Path: "/v1/playlists/{id}", Summary: "Delete id", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playlists.go:131
	if err := apikit.RegisterHTTP[catalog.PlaylistMutation, catalog.PlaylistReceipt](registry, apikit.Metadata{ID: "post_playlists_id_entries", Method: "POST", Path: "/v1/playlists/{id}/entries", Summary: "Post entries", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playlists.go:131
	if err := apikit.RegisterHTTP[catalog.PlaylistMutation, catalog.PlaylistReceipt](registry, apikit.Metadata{ID: "delete_playlists_id_entries_entryId", Method: "DELETE", Path: "/v1/playlists/{id}/entries/{entryId}", Summary: "Delete entryId", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playlists.go:131
	if err := apikit.RegisterHTTP[catalog.PlaylistMutation, catalog.PlaylistReceipt](registry, apikit.Metadata{ID: "put_playlists_id_shares", Method: "PUT", Path: "/v1/playlists/{id}/shares", Summary: "Put shares", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playlists.go:131
	if err := apikit.RegisterHTTP[catalog.PlaylistMutation, catalog.PlaylistReceipt](registry, apikit.Metadata{ID: "delete_playlists_id_shares", Method: "DELETE", Path: "/v1/playlists/{id}/shares", Summary: "Delete shares", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:55
	if err := apikit.RegisterHTTP[struct{}, catalog.SavedResourcePage](registry, apikit.Metadata{ID: "get_saved_resources", Method: "GET", Path: "/v1/saved-resources", Summary: "Get saved resources", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "kind", "pinned"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:81
	if err := apikit.RegisterHTTP[struct{}, catalog.SavedResource](registry, apikit.Metadata{ID: "get_saved_resources_id", Method: "GET", Path: "/v1/saved-resources/{id}", Summary: "Get id", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:121
	if err := apikit.RegisterHTTP[struct{}, catalog.SavedResourceCandidates](registry, apikit.Metadata{ID: "get_saved_resources_id_share_candidates", Method: "GET", Path: "/v1/saved-resources/{id}/share-candidates", Summary: "Get share candidates", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:140
	if err := apikit.RegisterHTTP[catalog.SavedResourceMutation, struct {
		ServerID    string                       `json:"serverId"`
		ViewerFence string                       `json:"viewerFence"`
		Receipt     catalog.SavedResourceReceipt `json:"receipt"`
		Current     *struct {
			ResourceID string `json:"resourceId"`
			Revision   int64  `json:"revision"`
			Deleted    bool   `json:"deleted"`
		} `json:"current,omitempty"`
		ReceiptLifetimeSeconds int `json:"receiptLifetimeSeconds"`
	}](registry, apikit.Metadata{ID: "post_saved_resources", Method: "POST", Path: "/v1/saved-resources", Summary: "Post saved resources", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:140
	if err := apikit.RegisterHTTP[catalog.SavedResourceMutation, struct {
		ServerID    string                       `json:"serverId"`
		ViewerFence string                       `json:"viewerFence"`
		Receipt     catalog.SavedResourceReceipt `json:"receipt"`
		Current     *struct {
			ResourceID string `json:"resourceId"`
			Revision   int64  `json:"revision"`
			Deleted    bool   `json:"deleted"`
		} `json:"current,omitempty"`
		ReceiptLifetimeSeconds int `json:"receiptLifetimeSeconds"`
	}](registry, apikit.Metadata{ID: "patch_saved_resources_id", Method: "PATCH", Path: "/v1/saved-resources/{id}", Summary: "Patch id", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:140
	if err := apikit.RegisterHTTP[catalog.SavedResourceMutation, struct {
		ServerID    string                       `json:"serverId"`
		ViewerFence string                       `json:"viewerFence"`
		Receipt     catalog.SavedResourceReceipt `json:"receipt"`
		Current     *struct {
			ResourceID string `json:"resourceId"`
			Revision   int64  `json:"revision"`
			Deleted    bool   `json:"deleted"`
		} `json:"current,omitempty"`
		ReceiptLifetimeSeconds int `json:"receiptLifetimeSeconds"`
	}](registry, apikit.Metadata{ID: "delete_saved_resources_id", Method: "DELETE", Path: "/v1/saved-resources/{id}", Summary: "Delete id", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:140
	if err := apikit.RegisterHTTP[catalog.SavedResourceMutation, struct {
		ServerID    string                       `json:"serverId"`
		ViewerFence string                       `json:"viewerFence"`
		Receipt     catalog.SavedResourceReceipt `json:"receipt"`
		Current     *struct {
			ResourceID string `json:"resourceId"`
			Revision   int64  `json:"revision"`
			Deleted    bool   `json:"deleted"`
		} `json:"current,omitempty"`
		ReceiptLifetimeSeconds int `json:"receiptLifetimeSeconds"`
	}](registry, apikit.Metadata{ID: "put_saved_resources_id_entries", Method: "PUT", Path: "/v1/saved-resources/{id}/entries", Summary: "Put entries", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:140
	if err := apikit.RegisterHTTP[catalog.SavedResourceMutation, struct {
		ServerID    string                       `json:"serverId"`
		ViewerFence string                       `json:"viewerFence"`
		Receipt     catalog.SavedResourceReceipt `json:"receipt"`
		Current     *struct {
			ResourceID string `json:"resourceId"`
			Revision   int64  `json:"revision"`
			Deleted    bool   `json:"deleted"`
		} `json:"current,omitempty"`
		ReceiptLifetimeSeconds int `json:"receiptLifetimeSeconds"`
	}](registry, apikit.Metadata{ID: "put_saved_resources_id_shares", Method: "PUT", Path: "/v1/saved-resources/{id}/shares", Summary: "Put shares", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:140
	if err := apikit.RegisterHTTP[catalog.SavedResourceMutation, struct {
		ServerID    string                       `json:"serverId"`
		ViewerFence string                       `json:"viewerFence"`
		Receipt     catalog.SavedResourceReceipt `json:"receipt"`
		Current     *struct {
			ResourceID string `json:"resourceId"`
			Revision   int64  `json:"revision"`
			Deleted    bool   `json:"deleted"`
		} `json:"current,omitempty"`
		ReceiptLifetimeSeconds int `json:"receiptLifetimeSeconds"`
	}](registry, apikit.Metadata{ID: "delete_saved_resources_id_shares", Method: "DELETE", Path: "/v1/saved-resources/{id}/shares", Summary: "Delete shares", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:140
	if err := apikit.RegisterHTTP[catalog.SavedResourceMutation, struct {
		ServerID    string `json:"serverId"`
		ViewerFence string `json:"viewerFence"`
		OperationID string `json:"operationId"`
		ResourceID  string `json:"resourceId"`
		PlaylistID  string `json:"playlistId"`
		Revision    int64  `json:"revision"`
		PinRevision int64  `json:"pinRevision"`
		Pinned      bool   `json:"pinned"`
		Deleted     bool   `json:"deleted"`
	}](registry, apikit.Metadata{ID: "put_saved_pins_kind_id", Method: "PUT", Path: "/v1/saved-pins/{kind}/{id}", Summary: "Put id", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:140
	if err := apikit.RegisterHTTP[catalog.SavedResourceMutation, struct {
		ServerID               string `json:"serverId"`
		ViewerFence            string `json:"viewerFence"`
		OperationID            string `json:"operationId"`
		ClearedNotInterested   int    `json:"clearedNotInterested"`
		ReceiptLifetimeSeconds int    `json:"receiptLifetimeSeconds"`
	}](registry, apikit.Metadata{ID: "post_me_recommendations_reset", Method: "POST", Path: "/v1/me/recommendations:reset", Summary: "Post recommendations:reset", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/saved_resources.go:140
	if err := apikit.RegisterHTTP[catalog.SavedResourceMutation, struct {
		ServerID               string                  `json:"serverId"`
		ViewerFence            string                  `json:"viewerFence"`
		Receipt                catalog.ActivityReceipt `json:"receipt"`
		ReceiptLifetimeSeconds int                     `json:"receiptLifetimeSeconds"`
	}](registry, apikit.Metadata{ID: "post_personal_history_actions", Method: "POST", Path: "/v1/personal-history/actions", Summary: "Post actions", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	return nil
}
