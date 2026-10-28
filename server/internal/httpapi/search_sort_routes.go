package httpapi

import (
	"portico.local/apikit"
	"portico.local/server/internal/catalog"
)

func registerSearchSortRoutes(registry *apikit.Registry, d Dependencies) error {
	failures := []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}
	libraryFailures := append(append([]string{}, failures...), "revision_mismatch")
	if err := apikit.RegisterRaw(registry, apikit.RawRoute[noBody, catalog.SearchEnvelope]{Metadata: apikit.Metadata{Audience: apikit.Integrations, Admission: laneExpensive, ID: "search", Method: "GET", Path: "/v1/search", Summary: "Search titles, people, years and collections", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200,
		Query: []string{"q", "group", "groups", "cursor", "limit", "sort", "direction", "libraryIds", "record"}, Errors: failures,
	}, Serve: d.searchHandler}); err != nil {
		return err
	}
	if err := apikit.RegisterRaw(registry, apikit.RawRoute[browseRequestBody, catalog.BrowseResult]{Metadata: apikit.Metadata{Audience: apikit.Clients, Admission: laneBrowsing, ID: "browse_library", Method: "POST", Path: "/v1/libraries/{id}/browse", Summary: "Read a window of a library pivot", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200, BodyLimit: 65536, Errors: append(append([]string{}, libraryFailures...), "stale_continuation", "rate_limited")}, Serve: d.browseHandler}); err != nil {
		return err
	}
	if err := apikit.RegisterRaw(registry, apikit.RawRoute[browseRequestBody, catalog.BrowseResult]{Metadata: apikit.Metadata{Audience: apikit.Clients, Admission: laneBrowsing, ID: "browse_across", Method: "POST", Path: "/v1/browse", Summary: "Read a window of a group of titles across the viewer's libraries", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200, BodyLimit: 65536, Errors: failures}, Serve: d.browseAcrossHandler}); err != nil {
		return err
	}
	if err := apikit.RegisterRaw(registry, apikit.RawRoute[noBody, catalog.BrowseCapabilities]{Metadata: apikit.Metadata{Audience: apikit.Clients, Admission: laneBrowsing, ID: "get_browse_capabilities", Method: "GET", Path: "/v1/browse-capabilities", Summary: "Read the vocabulary of lists across libraries", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Cost: apikit.Constant, Status: 200,
		Query: []string{"pivot"}, Errors: failures,
	}, Serve: d.browseAcrossCapabilitiesHandler}); err != nil {
		return err
	}
	if err := apikit.RegisterRaw(registry, apikit.RawRoute[noBody, catalog.FacetPage]{Metadata: apikit.Metadata{Audience: apikit.Clients, Admission: laneBrowsing, ID: "get_browse_facets", Method: "GET", Path: "/v1/browse/facets", Summary: "Read counted facet choices across the viewer's libraries, or within one list's titles", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200,
		Query: []string{"field", "q", "limit", "group", "value", "query", "pivot"}, Errors: failures,
	}, Serve: d.browseAcrossFacetsHandler}); err != nil {
		return err
	}
	return apikit.RegisterRaw(registry, apikit.RawRoute[noBody, ReadingFacetQueryResponse]{Metadata: apikit.Metadata{Audience: apikit.Clients, Admission: laneBrowsing, ID: "get_library_facets", Method: "GET", Path: "/v1/libraries/{id}/facets", Summary: "Read counted facet choices", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Cost: apikit.PageSized, Status: 200,
		Query: []string{"field", "q", "limit", "offset", "query", "pivot"}, SuccessStatuses: []int{202}, Errors: append(append([]string{}, libraryFailures...), "stale_continuation", "search_busy"),
		ResponseHeaders: []apikit.ResponseHeader{
			{Status: 202, Name: "Location", Description: "Same-library Reading facet preparation", Required: true},
			{Status: 202, Name: "Retry-After", Description: "Minimum delay before polling, in seconds", Required: true},
		},
	}, Serve: d.facetsHandler})
}
