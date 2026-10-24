package httpapi

import (
	"net/http"
	"reflect"
	"strconv"

	"portico.local/apikit"
	"portico.local/server/internal/reading"
)

// ReadingDiscoverRow is one server-composed Discover row. The key names the row
// the client asked for; items use the same public projection as List/Continue.
type ReadingDiscoverRow struct {
	Key    string                  `json:"key"`
	Items  []ReadingItem           `json:"items"`
	Series []ReadingDiscoverSeries `json:"series,omitempty"`
}

// ReadingDiscoverSeries is a summary of actual eligible library membership.
// Preview is bounded canonical membership, not a replacement for the signed
// series endpoint. Counts are exact over all visible titles in this series.
type ReadingDiscoverSeries struct {
	ID        string        `json:"id"`
	LibraryID string        `json:"libraryId"`
	Name      string        `json:"name"`
	Count     int           `json:"count"`
	ReadCount int           `json:"readCount"`
	Preview   []ReadingItem `json:"preview"`
}

func (ReadingDiscoverSeries) JSONSchema() map[string]any {
	type wire ReadingDiscoverSeries
	shape, _ := apikit.Schema(reflect.TypeFor[wire]())
	shape["additionalProperties"] = false
	props := shape["properties"].(map[string]any)
	props["id"].(map[string]any)["pattern"] = "^[A-Za-z0-9_-]{43}$"
	props["libraryId"].(map[string]any)["minLength"] = 1
	props["name"].(map[string]any)["minLength"] = 1
	props["name"].(map[string]any)["maxLength"] = 4096
	props["count"].(map[string]any)["minimum"] = 1
	props["readCount"].(map[string]any)["minimum"] = 0
	props["preview"].(map[string]any)["minItems"] = 1
	props["preview"].(map[string]any)["maxItems"] = 3
	return shape
}

// ReadingDiscoverResponse publishes only rows supported by real producers.
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
	seriesLimit := 0
	if raw := r.URL.Query().Get("seriesLimit"); r.URL.Query().Has("seriesLimit") {
		var err error
		seriesLimit, err = strconv.Atoi(raw)
		if err != nil || seriesLimit < 1 || seriesLimit > 20 || r.URL.Query().Get("libraryId") == "" {
			readingFailure(w, reading.ErrInvalid)
			return
		}
	}
	discover, err := d.readingService().Discover(r.Context(), p, r.URL.Query().Get("libraryId"), reading.DiscoverLimits{Recent: limit, Series: seriesLimit})
	if err != nil {
		readingFailure(w, err)
		return
	}
	items := make([]ReadingItem, 0, len(discover.Recent))
	for _, row := range discover.Recent {
		items = append(items, readingItemProjection(row))
	}
	out := ReadingDiscoverResponse{Rows: []ReadingDiscoverRow{{Key: "recent", Items: items}}}
	if len(discover.Series) > 0 {
		row := ReadingDiscoverRow{Key: "series", Items: []ReadingItem{}, Series: []ReadingDiscoverSeries{}}
		for _, series := range discover.Series {
			summary := ReadingDiscoverSeries{ID: series.ID, LibraryID: series.LibraryID, Name: series.Name, Count: series.Count, ReadCount: series.ReadCount, Preview: []ReadingItem{}}
			for _, item := range series.Preview {
				summary.Preview = append(summary.Preview, readingItemProjection(item))
			}
			row.Series = append(row.Series, summary)
		}
		out.Rows = append(out.Rows, row)
	}
	write(w, 200, out)
}

// registerReadingDiscoverHTTPContract publishes only the additive Discover
// operation; the vocabulary is a rows-only payload of the existing item shape.
func registerReadingDiscoverHTTPContract(registry *apikit.Registry) error {
	errors := []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "revision_required", "revision_mismatch", "request_in_progress", "body_too_large", "internal"}
	m := apikit.Metadata{ID: "get_reading_discover", Method: "GET", Path: "/v1/reading/discover", Summary: "Read the server-composed Discover rows", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, Status: 200, Errors: errors, Query: []string{"libraryId", "limit", "seriesLimit"}}
	return apikit.RegisterHTTP[struct{}, ReadingDiscoverResponse](registry, m)
}
