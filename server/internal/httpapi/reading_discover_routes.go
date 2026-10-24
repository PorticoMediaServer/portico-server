package httpapi

import (
	"net/http"
	"strconv"

	"portico.local/apikit"
	"portico.local/server/internal/reading"
)

// ReadingDiscoverRow is one server-composed Discover row. The key names the row
// the client asked for; items use the same public projection as List/Continue.
type ReadingDiscoverRow struct {
	Key   string        `json:"key"`
	Items []ReadingItem `json:"items"`
}

// ReadingDiscoverResponse carries only rows the server can compose from facts
// already in the snapshot. It publishes no series, counts or capabilities that
// would imply the authoritative ready projection.
type ReadingDiscoverResponse struct {
	Rows []ReadingDiscoverRow `json:"rows"`
}

func (d Dependencies) readingDiscover(w http.ResponseWriter, r *http.Request) {
	p, ok := d.readingPrincipal(w, r)
	if !ok {
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var err error
		if limit, err = strconv.Atoi(raw); err != nil || limit < 1 || limit > 100 {
			readingFailure(w, reading.ErrInvalid)
			return
		}
	}
	discover, err := d.readingService().Discover(r.Context(), p, r.URL.Query().Get("libraryId"), reading.DiscoverLimits{Recent: limit})
	if err != nil {
		readingFailure(w, err)
		return
	}
	items := make([]ReadingItem, 0, len(discover.Recent))
	for _, row := range discover.Recent {
		items = append(items, readingItemProjection(row))
	}
	write(w, 200, ReadingDiscoverResponse{Rows: []ReadingDiscoverRow{{Key: "recent", Items: items}}})
}

// registerReadingDiscoverHTTPContract publishes only the additive Discover
// operation; the vocabulary is a rows-only payload of the existing item shape.
func registerReadingDiscoverHTTPContract(registry *apikit.Registry) error {
	errors := []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "revision_required", "revision_mismatch", "request_in_progress", "body_too_large", "internal"}
	m := apikit.Metadata{ID: "get_reading_discover", Method: "GET", Path: "/v1/reading/discover", Summary: "Read the server-composed Discover rows", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, Status: 200, Errors: errors, Query: []string{"libraryId", "limit"}}
	return apikit.RegisterHTTP[struct{}, ReadingDiscoverResponse](registry, m)
}
