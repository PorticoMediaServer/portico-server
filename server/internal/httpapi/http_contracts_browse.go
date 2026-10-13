package httpapi

import (
	"portico.local/apikit"
	catalog "portico.local/server/internal/catalog"
	operations "portico.local/server/internal/operations"
)

func registerBrowseHTTPContracts(registry *apikit.Registry) error {
	// internal/httpapi/audio.go:11
	if err := apikit.RegisterHTTP[struct{}, struct {
		Artists    []catalog.Artist `json:"artists"`
		NextCursor string           `json:"nextCursor"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_artists", Method: "GET", Path: "/v1/libraries/{id}/artists", Summary: "Get artists", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/audio.go:33
	if err := apikit.RegisterHTTP[struct{}, struct {
		Books      []catalog.Book `json:"books"`
		NextCursor string         `json:"nextCursor"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_books", Method: "GET", Path: "/v1/libraries/{id}/books", Summary: "Get books", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/audio.go:56
	if err := apikit.RegisterHTTP[struct{}, struct {
		Albums     []catalog.Album `json:"albums"`
		NextCursor string          `json:"nextCursor"`
	}](registry, apikit.Metadata{ID: "get_artists_id_albums", Method: "GET", Path: "/v1/artists/{id}/albums", Summary: "Get albums", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/audio.go:56
	if err := apikit.RegisterHTTP[struct{}, struct {
		Songs      []catalog.Item `json:"songs"`
		NextCursor string         `json:"nextCursor"`
	}](registry, apikit.Metadata{ID: "get_albums_id_songs", Method: "GET", Path: "/v1/albums/{id}/songs", Summary: "Get songs", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/audio.go:56
	if err := apikit.RegisterHTTP[struct{}, struct {
		Files      []catalog.Item `json:"files"`
		NextCursor string         `json:"nextCursor"`
	}](registry, apikit.Metadata{ID: "get_books_id_files", Method: "GET", Path: "/v1/books/{id}/files", Summary: "Get files", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/audio.go:56
	if err := apikit.RegisterHTTP[struct{}, struct {
		Chapters   []catalog.BookChapter `json:"chapters"`
		NextCursor string                `json:"nextCursor"`
	}](registry, apikit.Metadata{ID: "get_books_id_chapters", Method: "GET", Path: "/v1/books/{id}/chapters", Summary: "Get chapters", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/browse_routes.go:79
	if err := apikit.RegisterHTTP[struct{}, catalog.BrowseCapabilities](registry, apikit.Metadata{ID: "get_libraries_id_browse_capabilities", Method: "GET", Path: "/v1/libraries/{id}/browse-capabilities", Summary: "Get browse capabilities", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"pivot"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/browse_routes.go:102
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64                   "json:\"expectedRevision\""
		Order            []catalog.PinOrderEntry "json:\"order\""
	}, struct {
		ServerID    string                  `json:"serverId"`
		ViewerFence string                  `json:"viewerFence"`
		Revision    int64                   `json:"revision"`
		Order       []catalog.PinOrderEntry `json:"order"`
	}](registry, apikit.Metadata{ID: "put_saved_pins_order", Method: "PUT", Path: "/v1/saved-pins/order", Summary: "Put order", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/browse_routes.go:125
	if err := apikit.RegisterHTTP[struct{}, struct {
		ServerID    string                  `json:"serverId"`
		ViewerFence string                  `json:"viewerFence"`
		Revision    int64                   `json:"revision"`
		Order       []catalog.PinOrderEntry `json:"order"`
	}](registry, apikit.Metadata{ID: "get_saved_pins_order", Method: "GET", Path: "/v1/saved-pins/order", Summary: "Get order", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/browse_routes.go:138
	if err := apikit.RegisterHTTP[struct{}, catalog.LibraryNavigation](registry, apikit.Metadata{ID: "get_me_library_navigation", Method: "GET", Path: "/v1/me/library-navigation", Summary: "Get library navigation", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/browse_routes.go:155
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64    "json:\"expectedRevision\""
		PinnedLibraryIDs []string "json:\"pinnedLibraryIds\""
	}, catalog.LibraryNavigation](registry, apikit.Metadata{ID: "put_me_library_navigation", Method: "PUT", Path: "/v1/me/library-navigation", Summary: "Put library navigation", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/detail.go:134
	if err := apikit.RegisterHTTP[struct{}, catalog.CreditPage](registry, apikit.Metadata{ID: "get_items_id_credits", Method: "GET", Path: "/v1/items/{id}/credits", Summary: "Get credits", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "group", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/episodes.go:36
	if err := apikit.RegisterHTTP[struct{}, struct {
		Shows      []catalog.Show `json:"shows"`
		NextCursor string         `json:"nextCursor"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_shows", Method: "GET", Path: "/v1/libraries/{id}/shows", Summary: "Get shows", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/episodes.go:60
	if err := apikit.RegisterHTTP[struct{}, struct {
		Seasons []catalog.Season `json:"seasons"`
	}](registry, apikit.Metadata{ID: "get_shows_id_seasons", Method: "GET", Path: "/v1/shows/{id}/seasons", Summary: "Get seasons", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/episodes.go:78
	if err := apikit.RegisterHTTP[struct{}, struct {
		Episodes   []catalog.Item `json:"episodes"`
		NextCursor string         `json:"nextCursor"`
	}](registry, apikit.Metadata{ID: "get_seasons_id_episodes", Method: "GET", Path: "/v1/seasons/{id}/episodes", Summary: "Get episodes", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/episodes.go:97
	if err := apikit.RegisterHTTP[struct{}, struct {
		Episodes   []catalog.Item `json:"episodes"`
		NextCursor string         `json:"nextCursor"`
		Group      string         `json:"group"`
	}](registry, apikit.Metadata{ID: "get_shows_id_episodes", Method: "GET", Path: "/v1/shows/{id}/episodes", Summary: "Get episodes", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/episodes.go:116
	if err := apikit.RegisterHTTP[struct{}, struct {
		ShowId   string                  `json:"showId"`
		Rows     []catalog.HomeRow       `json:"rows"`
		Revision catalog.ContentRevision `json:"revision"`
	}](registry, apikit.Metadata{ID: "get_shows_id_recommendations", Method: "GET", Path: "/v1/shows/{id}/recommendations", Summary: "Get recommendations", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneExpensive, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/episodes.go:154
	if err := apikit.RegisterHTTP[struct{}, struct {
		Issues     []catalog.EpisodeIssue `json:"issues"`
		NextCursor string                 `json:"nextCursor"`
	}](registry, apikit.Metadata{ID: "get_libraries_id_episode_issues", Method: "GET", Path: "/v1/libraries/{id}/episode-issues", Summary: "Get episode issues", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/episodes.go:167
	if err := apikit.RegisterHTTP[catalog.EpisodeAssignment, struct {
		Status string `json:"status"`
	}](registry, apikit.Metadata{ID: "post_libraries_id_episode_assignments", Method: "POST", Path: "/v1/libraries/{id}/episode-assignments", Summary: "Post episode assignments", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/home_routes.go:99
	if err := apikit.RegisterHTTP[struct{}, catalog.HomeLayoutView](registry, apikit.Metadata{ID: "get_home_layout", Method: "GET", Path: "/v1/home/layout", Summary: "Get layout", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/home_routes.go:121
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64    `json:"expectedRevision"`
		IdempotencyKey   string   `json:"idempotencyKey"`
		RowOrder         []string `json:"rowOrder"`
		HiddenRowIDs     []string `json:"hiddenRowIds"`
	}, operations.HomeLayoutDocument](registry, apikit.Metadata{ID: "put_home_layout", Method: "PUT", Path: "/v1/home/layout", Summary: "Put layout", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/home_routes.go:124
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision int64    `json:"expectedRevision"`
		IdempotencyKey   string   `json:"idempotencyKey"`
		RowOrder         []string `json:"rowOrder"`
		HiddenRowIDs     []string `json:"hiddenRowIds"`
	}, operations.HomeLayoutDocument](registry, apikit.Metadata{ID: "post_home_layout_reset", Method: "POST", Path: "/v1/home/layout/reset", Summary: "Post reset", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/home_routes.go:127
	if err := apikit.RegisterHTTP[struct{}, catalog.Suggestions](registry, apikit.Metadata{ID: "get_suggestions", Method: "GET", Path: "/v1/suggestions", Summary: "Get suggestions", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneExpensive, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/home_routes.go:164
	if err := apikit.RegisterHTTP[struct{}, struct {
		ItemId   string                  `json:"itemId"`
		Rows     []catalog.HomeRow       `json:"rows"`
		Revision catalog.ContentRevision `json:"revision"`
	}](registry, apikit.Metadata{ID: "get_items_id_recommendations", Method: "GET", Path: "/v1/items/{id}/recommendations", Summary: "Get recommendations", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneExpensive, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/listening.go:17
	if err := apikit.RegisterHTTP[struct{}, projectedViewerEnvelope[catalog.ListeningPreferences]](registry, apikit.Metadata{ID: "get_listening_preferences", Method: "GET", Path: "/v1/listening/preferences", Summary: "Get preferences", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/listening.go:31
	if err := apikit.RegisterHTTP[struct {
		Revision       *int64   "json:\"revision\""
		MusicRate      *float64 "json:\"musicRate\""
		BookRate       *float64 "json:\"bookRate\""
		AutoplayNext   *bool    "json:\"autoplayNext\""
		PassoutMinutes *int     "json:\"passoutMinutes\""
	}, projectedViewerEnvelope[catalog.ListeningPreferences]](registry, apikit.Metadata{ID: "put_listening_preferences", Method: "PUT", Path: "/v1/listening/preferences", Summary: "Put preferences", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/listening.go:97
	if err := apikit.RegisterHTTP[struct{}, projectedViewerEnvelope[catalog.ListeningItem]](registry, apikit.Metadata{ID: "get_items_id_listening", Method: "GET", Path: "/v1/items/{id}/listening", Summary: "Get listening", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/people_routes.go:18
	if err := apikit.RegisterHTTP[struct{}, catalog.PeopleDirectory](registry, apikit.Metadata{ID: "get_people", Method: "GET", Path: "/v1/people", Summary: "Get people", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"limit", "q"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/people_routes.go:86
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_people_id_portrait", Method: "GET", Path: "/v1/people/{id}/portrait", Summary: "Get portrait", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"v"}, ResponseContentType: "image/jpeg", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/people_routes.go:195
	if err := apikit.RegisterHTTP[struct {
		AddItemIDs    []string "json:\"addItemIds\""
		RemoveItemIDs []string "json:\"removeItemIds\""
	}, catalog.CollectionBatch](registry, apikit.Metadata{ID: "post_collections_id_items_batch", Method: "POST", Path: "/v1/collections/{id}/items:batch", Summary: "Post items:batch", Access: apikit.Owner, Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/search.go:62
	if err := apikit.RegisterHTTP[struct{}, catalog.SearchHistoryPage](registry, apikit.Metadata{ID: "get_search_history", Method: "GET", Path: "/v1/search/history", Summary: "Get history", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneExpensive, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/search.go:80
	if err := apikit.RegisterHTTP[struct{}, catalog.SearchHistoryPage](registry, apikit.Metadata{ID: "delete_search_history", Method: "DELETE", Path: "/v1/search/history", Summary: "Delete history", Access: apikit.Device, Requires: apikit.ScopePlayback, Lane: apikit.Default, Admission: laneExpensive, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/show_workspace.go:11
	if err := apikit.RegisterHTTP[struct{}, catalog.ShowWorkspace](registry, apikit.Metadata{ID: "get_libraries_id_show_workspace", Method: "GET", Path: "/v1/libraries/{id}/show-workspace", Summary: "Get show workspace", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Query: []string{"cursor", "episodeId", "group", "seasonCursor", "seasonId", "selectedSeasonId", "showId"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	return nil
}
