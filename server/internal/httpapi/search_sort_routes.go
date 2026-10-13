package httpapi

import (
	"portico.local/apikit"
	"portico.local/server/internal/catalog"
)

func registerSearchSortRoutes(registry *apikit.Registry, d Dependencies) error {
	failures := []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}
	if err := apikit.RegisterRaw(registry, apikit.RawRoute[noBody, catalog.SearchEnvelope]{Metadata: apikit.Metadata{
		ID: "search", Method: "GET", Path: "/v1/search", Summary: "Search titles, people, years and collections", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200,
		Query: []string{"q", "group", "groups", "cursor", "limit", "sort", "direction", "libraryIds", "record"}, Errors: failures,
	}, Serve: d.searchHandler}); err != nil {
		return err
	}
	if err := apikit.RegisterRaw(registry, apikit.RawRoute[browseRequestBody, catalog.BrowseResult]{Metadata: apikit.Metadata{
		ID: "browse_library", Method: "POST", Path: "/v1/libraries/{id}/browse", Summary: "Read a window of a library pivot", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200, BodyLimit: 65536, Errors: failures,
	}, Serve: d.browseHandler}); err != nil {
		return err
	}
	return apikit.RegisterRaw(registry, apikit.RawRoute[noBody, catalog.FacetPage]{Metadata: apikit.Metadata{
		ID: "get_library_facets", Method: "GET", Path: "/v1/libraries/{id}/facets", Summary: "Read counted facet choices", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200,
		Query: []string{"field", "q", "limit"}, Errors: failures,
	}, Serve: d.facetsHandler})
}
