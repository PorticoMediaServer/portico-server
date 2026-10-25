package httpapi

import (
	"net/http"
	"reflect"
	"strconv"

	"portico.local/apikit"
	"portico.local/server/internal/reading"
)

type ReadingPeriodicalType string

func (ReadingPeriodicalType) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"magazine", "newspaper"}}
}

type ReadingIssueDate string

func (ReadingIssueDate) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}(-[0-9]{2})?$", "minLength": 7, "maxLength": 10}
}

type ReadingPeriodicalIssue struct {
	Item      ReadingItem      `json:"item"`
	IssueDate ReadingIssueDate `json:"issueDate"`
}

func (ReadingPeriodicalIssue) JSONSchema() map[string]any {
	type wire ReadingPeriodicalIssue
	shape, _ := apikit.Schema(reflect.TypeFor[wire]())
	shape["additionalProperties"] = false
	return shape
}

type ReadingPeriodicalIssuesResponse struct {
	LibraryID       string                   `json:"libraryId"`
	SeriesID        string                   `json:"seriesId"`
	PublicationType ReadingPeriodicalType    `json:"publicationType"`
	IssueDate       ReadingIssueDate         `json:"issueDate,omitempty"`
	Items           []ReadingPeriodicalIssue `json:"items"`
	NextCursor      string                   `json:"nextCursor,omitempty"`
}

func (d Dependencies) readingPeriodicalIssues(w http.ResponseWriter, r *http.Request) {
	p, ok := d.readingPrincipal(w, r)
	if !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			readingFailure(w, reading.ErrInvalid)
			return
		}
	}
	library, series, kind := r.PathValue("libraryId"), r.PathValue("seriesId"), r.URL.Query().Get("publicationType")
	date := r.URL.Query().Get("issueDate")
	page, err := d.readingService().PeriodicalIssuesAt(r.Context(), p, library, series, kind, date, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		readingFailure(w, err)
		return
	}
	out := ReadingPeriodicalIssuesResponse{LibraryID: library, SeriesID: series, PublicationType: ReadingPeriodicalType(kind), IssueDate: ReadingIssueDate(date), Items: make([]ReadingPeriodicalIssue, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		out.Items = append(out.Items, ReadingPeriodicalIssue{Item: readingItemProjection(item), IssueDate: ReadingIssueDate(item.IssueDate)})
	}
	write(w, 200, out)
}
func registerReadingPeriodicalIssuesHTTPContract(registry *apikit.Registry) error {
	m := apikit.Metadata{ID: "get_reading_periodical_issues", Method: "GET", Path: "/v1/libraries/{libraryId}/reading/periodicals/{seriesId}/issues", Summary: "Read dated issues in one accepted typed Reading series", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "revision_mismatch", "request_in_progress", "internal"}, Query: []string{"publicationType", "issueDate", "cursor", "limit"}}
	return apikit.RegisterHTTP[struct{}, ReadingPeriodicalIssuesResponse](registry, m)
}
