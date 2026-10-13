package httpapi

import (
	"portico.local/apikit"
	catalog "portico.local/server/internal/catalog"
	hosted "portico.local/server/internal/hosted"
	identity "portico.local/server/internal/identity"
	ingestion "portico.local/server/internal/ingestion"
	lyrics "portico.local/server/internal/lyrics"
	mediaanalysis "portico.local/server/internal/mediaanalysis"
	operations "portico.local/server/internal/operations"
	playback "portico.local/server/internal/playback"
	preparedmedia "portico.local/server/internal/preparedmedia"
)

func registerPlaybackHTTPContracts(registry *apikit.Registry) error {
	// internal/httpapi/activity.go:36
	if err := apikit.RegisterHTTP[struct{}, playback.ActivityPage](registry, apikit.Metadata{ID: "get_admin_playback_sessions", Method: "GET", Path: "/v1/admin/playback-sessions", Summary: "Get playback sessions", Access: apikit.Owner, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit", "status"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/activity.go:36
	if err := apikit.RegisterHTTP[playback.ActivityStop, playback.ActivityReceipt](registry, apikit.Metadata{ID: "post_admin_playback_sessions_id_stop", Method: "POST", Path: "/v1/admin/playback-sessions/{id}/stop", Summary: "Post stop", Access: apikit.Owner, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneAdminHeavy, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/activity.go:36
	if err := apikit.RegisterHTTP[struct{}, playback.ActivityReceipt](registry, apikit.Metadata{ID: "get_admin_playback_operations_operationId", Method: "GET", Path: "/v1/admin/playback-operations/{operationId}", Summary: "Get operationId", Access: apikit.Owner, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneAdmin, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/audio_media.go:133
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_media_grant_audio", Method: "GET", Path: "/v1/media/{grant}/audio", Summary: "Get audio", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Media, Admission: laneMediaBody, Cost: apikit.Constant, BodyLimit: 0, Status: 200, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/audio_media.go:134
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "head_media_grant_audio", Method: "HEAD", Path: "/v1/media/{grant}/audio", Summary: "Head audio", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Media, Admission: laneMediaBody, Cost: apikit.Constant, BodyLimit: 0, Status: 200, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/audio_media.go:137
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_media_grant_audio_converted", Method: "GET", Path: "/v1/media/{grant}/audio-converted", Summary: "Get audio converted", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Media, Admission: laneMediaBody, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"fromFrame"}, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/delivery_routes.go:68
	if err := apikit.RegisterHTTP[struct{}, struct {
		ServerId         string                          `json:"serverId"`
		Policy           playback.ResolvedDeliveryPolicy `json:"policy"`
		Ladder           []playback.QualityPreset        `json:"ladder"`
		PlanningPolicies []string                        `json:"planningPolicies"`
		TransportClasses []string                        `json:"transportClasses"`
		SavedQualities   []string                        `json:"savedQualities"`
	}](registry, apikit.Metadata{ID: "get_playback_delivery_policy", Method: "GET", Path: "/v1/playback/delivery-policy", Summary: "Get delivery policy", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Media, Admission: lanePlayback, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/linear_media.go:83
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_media_linear_playbackId_generation_token_name", Method: "GET", Path: "/v1/media/linear/{playbackId}/{generation}/{token}/{name}", Summary: "Get name", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Media, Admission: laneMediaBody, Cost: apikit.Constant, BodyLimit: 0, Status: 200, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/lyrics.go:118
	if err := apikit.RegisterHTTP[struct{}, lyrics.View](registry, apikit.Metadata{ID: "get_items_id_lyrics", Method: "GET", Path: "/v1/items/{id}/lyrics", Summary: "Get lyrics", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"sessionId", "sourceId", "sourceVersion"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/lyrics.go:118
	if err := apikit.RegisterHTTP[struct{}, lyrics.Revision](registry, apikit.Metadata{ID: "get_items_id_lyrics_resource_revisions_revision", Method: "GET", Path: "/v1/items/{id}/lyrics/{resource}/revisions/{revision}", Summary: "Get revision", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"sessionId", "sourceId", "sourceVersion"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/lyrics.go:118
	if err := apikit.RegisterHTTP[lyrics.Mutation, lyrics.Revision](registry, apikit.Metadata{ID: "post_items_id_lyrics", Method: "POST", Path: "/v1/items/{id}/lyrics", Summary: "Post lyrics", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 2097152, Status: 200, Query: []string{"sessionId", "sourceId", "sourceVersion"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/lyrics.go:118
	if err := apikit.RegisterHTTP[lyrics.Mutation, lyrics.Revision](registry, apikit.Metadata{ID: "patch_items_id_lyrics_resource_offset", Method: "PATCH", Path: "/v1/items/{id}/lyrics/{resource}/offset", Summary: "Patch offset", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 2097152, Status: 200, Query: []string{"sessionId", "sourceId", "sourceVersion"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/lyrics.go:118
	if err := apikit.RegisterHTTP[lyrics.Mutation, lyrics.Revision](registry, apikit.Metadata{ID: "delete_items_id_lyrics_resource", Method: "DELETE", Path: "/v1/items/{id}/lyrics/{resource}", Summary: "Delete resource", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 2097152, Status: 200, Query: []string{"sessionId", "sourceId", "sourceVersion"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/lyrics.go:118
	if err := apikit.RegisterHTTP[lyrics.SearchInput, lyrics.SearchResult](registry, apikit.Metadata{ID: "post_items_id_lyrics_search", Method: "POST", Path: "/v1/items/{id}/lyrics/search", Summary: "Post search", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 2097152, Status: 200, Query: []string{"sessionId", "sourceId", "sourceVersion"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/lyrics.go:118
	if err := apikit.RegisterHTTP[lyrics.Choose, lyrics.Selection](registry, apikit.Metadata{ID: "put_items_id_lyrics_selection", Method: "PUT", Path: "/v1/items/{id}/lyrics/selection", Summary: "Put selection", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 2097152, Status: 200, Query: []string{"sessionId", "sourceId", "sourceVersion"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/lyrics.go:118
	if err := apikit.RegisterHTTP[lyrics.Mutation, struct{}](registry, apikit.Metadata{ID: "put_lyrics_provider", Method: "PUT", Path: "/v1/lyrics/provider", Summary: "Put provider", Access: apikit.Device, Requires: apikit.ScopeAdmin, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Query: []string{"sessionId", "sourceId", "sourceVersion"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/media_analysis.go:108
	if err := apikit.RegisterHTTP[struct{}, mediaanalysis.View](registry, apikit.Metadata{ID: "get_items_id_analysis", Method: "GET", Path: "/v1/items/{id}/analysis", Summary: "Get analysis", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/media_analysis.go:108
	if err := apikit.RegisterHTTP[struct{}, mediaanalysis.Artifact](registry, apikit.Metadata{ID: "get_items_id_analysis_artifacts_artifact", Method: "GET", Path: "/v1/items/{id}/analysis/artifacts/{artifact}", Summary: "Get artifact", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/media_analysis.go:108
	if err := apikit.RegisterHTTP[struct {
		mediaanalysis.Target
		Action string `json:"action"`
	}, ingestion.Job](registry, apikit.Metadata{ID: "post_items_id_analysis_jobs", Method: "POST", Path: "/v1/items/{id}/analysis/jobs", Summary: "Post jobs", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 2097152, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/media_analysis.go:108
	if err := apikit.RegisterHTTP[struct {
		mediaanalysis.Target
		Action string `json:"action"`
	}, ingestion.Job](registry, apikit.Metadata{ID: "post_items_id_analysis_jobs_job_control", Method: "POST", Path: "/v1/items/{id}/analysis/jobs/{job}/control", Summary: "Post control", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 2097152, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/media_analysis.go:108
	if err := apikit.RegisterHTTP[mediaanalysis.Mutation, mediaanalysis.MutationResult](registry, apikit.Metadata{ID: "post_items_id_analysis_markers", Method: "POST", Path: "/v1/items/{id}/analysis/markers", Summary: "Post markers", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 2097152, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/media_analysis.go:108
	if err := apikit.RegisterHTTP[struct {
		mediaanalysis.Target
		Tier             string                    `json:"tier"`
		Operations       []string                  `json:"operations"`
		Trickplay        catalog.TrickplaySettings `json:"trickplay"`
		ExpectedRevision int64                     `json:"expectedRevision"`
	}, catalog.ScanPolicy](registry, apikit.Metadata{ID: "put_items_id_analysis_policy", Method: "PUT", Path: "/v1/items/{id}/analysis/policy", Summary: "Put policy", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 2097152, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playback_chapters.go:12
	if err := apikit.RegisterHTTP[struct{}, playback.ChapterProjection](registry, apikit.Metadata{ID: "get_playback_sessions_id_chapters", Method: "GET", Path: "/v1/playback/sessions/{id}/chapters", Summary: "Get chapters", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Security, Admission: laneSecurityFence, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit", "revision"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playback_diagnostics.go:15
	if err := apikit.RegisterHTTP[struct{}, struct {
		Scope       map[string]string `json:"scope"`
		ObservedAt  string            `json:"observedAt"`
		FreshUntil  string            `json:"freshUntil"`
		Diagnostics playbackOverview  `json:"diagnostics"`
	}](registry, apikit.Metadata{ID: "get_admin_playback_diagnostics", Method: "GET", Path: "/v1/admin/playback/diagnostics", Summary: "Get diagnostics", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Media, Admission: lanePlayback, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/playback_offers.go:12
	if err := apikit.RegisterHTTP[struct{}, playback.Offers](registry, apikit.Metadata{ID: "get_items_id_playback_offers", Method: "GET", Path: "/v1/items/{id}/playback-offers", Summary: "Get playback offers", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Media, Admission: lanePlayback, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"revision", "sessionId"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/prepared_media.go:69
	if err := apikit.RegisterHTTP[struct{}, struct{}](registry, apikit.Metadata{ID: "get_items_id_prepared_versions", Method: "GET", Path: "/v1/items/{id}/prepared-versions", Summary: "Get prepared versions", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/prepared_media.go:69
	if err := apikit.RegisterHTTP[struct{}, struct{}](registry, apikit.Metadata{ID: "post_items_id_optimizations", Method: "POST", Path: "/v1/items/{id}/optimizations", Summary: "Post optimizations", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/prepared_media.go:69
	if err := apikit.RegisterHTTP[struct{}, struct{}](registry, apikit.Metadata{ID: "post_prepared_versions_id_delete", Method: "POST", Path: "/v1/prepared-versions/{id}/delete", Summary: "Post delete", Access: apikit.Device, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:844
	if err := apikit.RegisterHTTP[struct{}, struct {
		Providers []map[string]string `json:"providers"`
	}](registry, apikit.Metadata{ID: "get_attribution", Method: "GET", Path: "/v1/attribution", Summary: "Get attribution", Access: apikit.Public, Requires: apikit.ScopeOpen, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:856
	if err := apikit.RegisterHTTP[struct{}, struct {
		Id               string `json:"id"`
		Name             string `json:"name"`
		SetupRequired    bool   `json:"setupRequired"`
		HostedConfigured bool   `json:"hostedConfigured"`
		HostedAttached   bool   `json:"hostedAttached"`
		Version          string `json:"version"`
	}](registry, apikit.Metadata{ID: "get_system", Method: "GET", Path: "/v1/system", Summary: "Get system", Access: apikit.Public, Requires: apikit.ScopeOpen, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:859
	if err := apikit.RegisterHTTP[struct{}, struct {
		Viewer identity.Viewer `json:"viewer"`
	}](registry, apikit.Metadata{ID: "get_me", Method: "GET", Path: "/v1/me", Summary: "Get me", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:867
	if err := apikit.RegisterHTTP[struct{}, struct{}](registry, apikit.Metadata{ID: "delete_sessions_current", Method: "DELETE", Path: "/v1/sessions/current", Summary: "Delete current", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneSecurityFence, Cost: apikit.Constant, BodyLimit: 0, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:868
	if err := apikit.RegisterHTTP[hosted.Signed, struct{}](registry, apikit.Metadata{ID: "post_hosted_wake", Method: "POST", Path: "/v1/hosted/wake", Summary: "Post wake", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 202, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:1018
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_items_id_art_kind", Method: "GET", Path: "/v1/items/{id}/art/{kind}", Summary: "Get kind", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Media, Admission: laneMedia, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"v"}, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:1053
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_media_grant_file", Method: "GET", Path: "/v1/media/{grant}/{file}", Summary: "Get file", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Media, Admission: laneMediaBody, Cost: apikit.Constant, BodyLimit: 0, Status: 200, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/router.go:1113
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_media_grant", Method: "GET", Path: "/v1/media/{grant}", Summary: "Get grant", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Media, Admission: laneMediaBody, Cost: apikit.Constant, BodyLimit: 0, Status: 200, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/trickplay.go:24
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_items_id_trickplay_set_thumbnails_vtt", Method: "GET", Path: "/v1/items/{id}/trickplay/{set}/thumbnails.vtt", Summary: "Get thumbnails.vtt", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Media, Admission: laneMedia, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/trickplay.go:61
	if err := apikit.RegisterHTTP[struct{}, mediaanalysis.TrickplayView](registry, apikit.Metadata{ID: "get_items_id_trickplay", Method: "GET", Path: "/v1/items/{id}/trickplay", Summary: "Get trickplay", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/activity.go:36
	if err := apikit.RegisterHTTP[preparedmedia.Command, preparedmedia.Job](registry, apikit.Metadata{ID: "post_optimization_jobs_id_cancel", Method: "POST", Path: "/v1/optimization-jobs/{id}/cancel", Summary: "Post cancel", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/activity.go:36
	if err := apikit.RegisterHTTP[preparedmedia.Command, preparedmedia.Job](registry, apikit.Metadata{ID: "post_optimization_jobs_id_retry", Method: "POST", Path: "/v1/optimization-jobs/{id}/retry", Summary: "Post retry", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/activity.go:36
	if err := apikit.RegisterHTTP[struct {
		IdempotencyKey string `json:"idempotencyKey"`
	}, operations.Job](registry, apikit.Metadata{ID: "post_libraries_id_lyrics_fetch", Method: "POST", Path: "/v1/libraries/{id}/lyrics/fetch", Summary: "Post fetch", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	return nil
}
