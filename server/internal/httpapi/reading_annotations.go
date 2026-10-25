package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"portico.local/apikit"
	"portico.local/server/internal/reading"
)

type ReadingAnnotationRevision int64

func (r *ReadingAnnotationRevision) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return reading.ErrInvalid
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if value < 0 || value > 9007199254740991 {
		return reading.ErrInvalid
	}
	*r = ReadingAnnotationRevision(value)
	return nil
}
func (ReadingAnnotationRevision) JSONSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991}
}

type ReadingAnnotationColour string

func (ReadingAnnotationColour) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"", "yellow", "green", "blue", "pink"}}
}

type ReadingAnnotationKind string

func (ReadingAnnotationKind) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"bookmark", "highlight"}}
}

type ReadingAnnotationOperation string

func (ReadingAnnotationOperation) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"upsert", "delete"}}
}

type ReadingAnnotation struct {
	ID        string                  `json:"id"`
	Kind      ReadingAnnotationKind   `json:"kind"`
	Location  ReadingLocation         `json:"location"`
	Text      string                  `json:"text"`
	Note      string                  `json:"note"`
	Colour    ReadingAnnotationColour `json:"colour"`
	CreatedAt int64                   `json:"createdAt"`
	UpdatedAt int64                   `json:"updatedAt"`
}
type ReadingAnnotationsResponse struct {
	SourceID       string              `json:"sourceId"`
	SourceRevision string              `json:"sourceRevision"`
	Revision       int64               `json:"revision"`
	Entries        []ReadingAnnotation `json:"entries"`
	NextCursor     string              `json:"nextCursor"`
}
type ReadingAnnotationRequest struct {
	ExpectedRevision ReadingAnnotationRevision  `json:"expectedRevision"`
	Operation        ReadingAnnotationOperation `json:"operation"`
	ID               string                     `json:"id"`
	Kind             ReadingAnnotationKind      `json:"kind,omitempty"`
	Location         *ReadingLocation           `json:"location,omitempty"`
	Text             string                     `json:"text,omitempty"`
	Note             string                     `json:"note,omitempty"`
	Colour           ReadingAnnotationColour    `json:"colour,omitempty"`
}
type ReadingAnnotationSavedResponse struct {
	Revision int64 `json:"revision"`
}

func readingAnnotationProjection(a reading.Annotation) ReadingAnnotation {
	l := a.Location
	return ReadingAnnotation{a.ID, ReadingAnnotationKind(a.Kind), ReadingLocation{ReadingFormat(l.Format), l.CFI, l.Section, l.PageIndex, l.Progression}, a.Text, a.Note, ReadingAnnotationColour(a.Colour), a.CreatedAt, a.UpdatedAt}
}
func (d Dependencies) readingAnnotations(w http.ResponseWriter, r *http.Request) {
	f, ok := d.openReadingBytes(w, r)
	if !ok {
		return
	}
	defer f.Close()
	var expected *int64
	if value, provided := r.URL.Query()["revision"]; provided {
		n, err := strconv.ParseInt(value[0], 10, 64)
		if err != nil || strconv.FormatInt(n, 10) != value[0] {
			readingFailure(w, reading.ErrInvalid)
			return
		}
		expected = &n
	}
	page, err := f.Annotations(r.URL.Query().Get("cursor"), expected)
	if err != nil {
		readingFailure(w, err)
		return
	}
	out := ReadingAnnotationsResponse{page.SourceID, page.SourceRevision, page.Revision, []ReadingAnnotation{}, page.NextCursor}
	for _, a := range page.Entries {
		out.Entries = append(out.Entries, readingAnnotationProjection(a))
	}
	if err = f.Validate(); err != nil {
		readingFailure(w, err)
		return
	}
	write(w, 200, out)
}
func (d Dependencies) readingMutateAnnotation(w http.ResponseWriter, r *http.Request) {
	f, ok := d.openReadingBytes(w, r)
	if !ok {
		return
	}
	defer f.Close()
	var request ReadingAnnotationRequest
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if err := decode(w, r, &request); err != nil {
		readingFailure(w, err)
		return
	}
	var location reading.Location
	if request.Operation == "upsert" && request.Location == nil {
		readingFailure(w, reading.ErrInvalid)
		return
	}
	if request.Location != nil {
		l := request.Location
		location = reading.Location{Format: string(l.Format), CFI: l.CFI, Section: l.Section, PageIndex: l.PageIndex, Progression: l.Progression}
	}
	revision, err := f.MutateAnnotation(reading.AnnotationMutation{ExpectedRevision: int64(request.ExpectedRevision), Operation: string(request.Operation), Annotation: reading.Annotation{ID: request.ID, Kind: string(request.Kind), Location: location, Text: request.Text, Note: request.Note, Colour: string(request.Colour)}})
	if err != nil {
		readingFailure(w, err)
		return
	}
	if err = f.Validate(); err != nil {
		readingFailure(w, err)
		return
	}
	write(w, 200, ReadingAnnotationSavedResponse{revision})
}
func registerReadingAnnotationHTTPContracts(registry *apikit.Registry) error {
	m := apikit.Metadata{ID: "get_reading_sessions_sessionId_annotations", Method: "GET", Path: "/v1/reading/sessions/{sessionId}/annotations", Summary: "Read the current profile's annotations for this exact publication version", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, Status: 200, Query: []string{"cursor", "revision"}, Headers: []string{"If-Match"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "revision_required", "revision_mismatch", "request_in_progress", "body_too_large", "internal"}}
	if err := apikit.RegisterHTTP[struct{}, ReadingAnnotationsResponse](registry, m); err != nil {
		return err
	}
	m.ID = "put_reading_sessions_sessionId_annotations"
	m.Method = "PUT"
	m.Summary = "Change an annotation under current profile, source version and collection CAS"
	m.Requires = apikit.ScopePlayback
	m.Query = nil
	m.Lane = apikit.Media
	m.Admission = laneMediaBody
	m.BodyLimit = 32768
	return apikit.RegisterHTTP[ReadingAnnotationRequest, ReadingAnnotationSavedResponse](registry, m)
}
