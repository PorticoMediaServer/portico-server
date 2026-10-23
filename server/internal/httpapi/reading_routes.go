package httpapi

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/reading"
)

func readingQuery(r *http.Request, allowed ...string) bool {
	if len(r.URL.RawQuery) > 8192 {
		return false
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return false
	}
	for name, values := range query {
		if len(values) != 1 || !slices.Contains(allowed, name) {
			return false
		}
	}
	return true
}

func (d Dependencies) readingRoutes(mux *http.ServeMux) {
	register := func(pattern string, query []string, handler http.HandlerFunc) {
		d.httpHandle(mux, pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "private, no-store")
			if !readingQuery(r, query...) {
				readingFailure(w, reading.ErrInvalid)
				return
			}
			handler(w, r)
		})
	}
	register("GET /v1/reading/items", []string{"libraryId", "cursor", "limit"}, d.listReadingItems)
	register("GET /v1/items/{itemId}/reading/versions", nil, d.readingVersions)
	register("POST /v1/reading/sessions", nil, d.prepareReadingSession)
	register("GET /v1/reading/sessions/{sessionId}/progress", nil, d.readingSessionProgress)
	register("PUT /v1/reading/sessions/{sessionId}/progress", nil, d.readingSaveSessionProgress)
	register("DELETE /v1/reading/sessions/{sessionId}", nil, d.closeReadingSession)
	register("GET /v1/reading/sessions/{sessionId}/source", nil, d.serveReadingSource)
	register("HEAD /v1/reading/sessions/{sessionId}/source", nil, d.serveReadingSource)
	register("GET /v1/reading/sessions/{sessionId}/assets", []string{"path"}, d.serveReadingAsset)
	register("GET /v1/reading/sessions/{sessionId}/pages/{pageIndex}", []string{"width"}, d.serveReadingPage)
}

func (d Dependencies) readingPrincipal(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	p, err := d.strictPrincipalFor(r)
	if err != nil {
		readingFailure(w, err)
		return p, false
	}
	return p, true
}

func (d Dependencies) listReadingItems(w http.ResponseWriter, r *http.Request) {
	p, ok := d.readingPrincipal(w, r)
	if !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var err error
		if limit, err = strconv.Atoi(raw); err != nil {
			readingFailure(w, reading.ErrInvalid)
			return
		}
	}
	page, err := d.readingService().List(r.Context(), p, r.URL.Query().Get("libraryId"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		readingFailure(w, err)
		return
	}
	out := ReadingItemsResponse{Items: []ReadingItem{}, NextCursor: page.NextCursor}
	for _, item := range page.Items {
		out.Items = append(out.Items, ReadingItem{item.ID, item.LibraryID, item.Title, ReadingFormat(item.Format), item.SourceID, item.InventoryAvailable})
	}
	write(w, 200, out)
}

func (d Dependencies) readingVersions(w http.ResponseWriter, r *http.Request) {
	p, ok := d.readingPrincipal(w, r)
	if !ok {
		return
	}
	versions, err := d.readingService().Versions(r.Context(), p, r.PathValue("itemId"))
	if err != nil {
		readingFailure(w, err)
		return
	}
	out := ReadingVersionsResponse{Versions: []ReadingVersion{}}
	for _, version := range versions {
		out.Versions = append(out.Versions, ReadingVersion{version.SourceID, ReadingFormat(version.Format), version.Size, version.InventoryAvailable})
	}
	write(w, 200, out)
}

func (d Dependencies) prepareReadingSession(w http.ResponseWriter, r *http.Request) {
	p, ok := d.readingPrincipal(w, r)
	if !ok {
		return
	}
	var request ReadingSessionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := decode(w, r, &request); err != nil {
		readingFailure(w, err)
		return
	}
	if request.ItemID == "" || request.SourceID == "" {
		readingFailure(w, reading.ErrInvalid)
		return
	}
	session, document, err := d.readingService().Prepare(r.Context(), p, request.ItemID, request.SourceID)
	if err != nil {
		readingFailure(w, err)
		return
	}
	v, m := document.Selection, document.Metadata
	out := ReadingSessionResponse{SessionID: session.ID, ItemID: v.ItemID, SourceID: v.SourceID, Revision: v.Revision, Format: ReadingFormat(v.Format), Size: v.Size, Metadata: ReadingMetadata{m.Title, m.Author, m.Language, m.Direction}, Spine: document.Spine, PageNames: document.PageNames}
	for _, resource := range document.Resources {
		out.Resources = append(out.Resources, ReadingResource{resource.Path, resource.MIME, resource.Size})
	}
	readingHeaders(w, v.Revision)
	write(w, 200, out)
}

func readingProgressProjection(progress *reading.Progress) *ReadingProgress {
	if progress == nil {
		return nil
	}
	l := progress.Location
	return &ReadingProgress{progress.SourceID, progress.SourceRevision, ReadingLocation{ReadingFormat(l.Format), l.CFI, l.Section, l.PageIndex, l.Progression}, progress.Revision, progress.ObservedAt, progress.Stale}
}

func (d Dependencies) readingSessionProgress(w http.ResponseWriter, r *http.Request) {
	f, ok := d.openReadingBytes(w, r)
	if !ok {
		return
	}
	defer f.Close()
	p, ok := d.readingPrincipal(w, r)
	if !ok {
		return
	}
	progress, err := d.readingService().SessionProgress(r.Context(), p, r.PathValue("sessionId"))
	if err != nil {
		readingFailure(w, err)
		return
	}
	if err = f.Validate(); err != nil {
		readingFailure(w, err)
		return
	}
	write(w, 200, ReadingProgressResponse{readingProgressProjection(progress)})
}

func (d Dependencies) readingSaveSessionProgress(w http.ResponseWriter, r *http.Request) {
	f, ok := d.openReadingBytes(w, r)
	if !ok {
		return
	}
	defer f.Close()
	p, ok := d.readingPrincipal(w, r)
	if !ok {
		return
	}
	var request ReadingProgressRequest
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err := decode(w, r, &request); err != nil {
		readingFailure(w, err)
		return
	}
	l := request.Location
	progress, saved, err := d.readingService().SaveSessionProgress(r.Context(), p, r.PathValue("sessionId"), request.ExpectedRevision, reading.Location{Format: string(l.Format), CFI: l.CFI, Section: l.Section, PageIndex: l.PageIndex, Progression: l.Progression})
	if err != nil {
		readingFailure(w, err)
		return
	}
	if err = f.Validate(); err != nil {
		readingFailure(w, err)
		return
	}
	write(w, 200, ReadingProgressSavedResponse{readingProgressProjection(progress), saved})
}

func (d Dependencies) closeReadingSession(w http.ResponseWriter, r *http.Request) {
	p, ok := d.readingPrincipal(w, r)
	if !ok {
		return
	}
	if err := d.readingService().CloseSession(p, r.PathValue("sessionId")); err != nil {
		readingFailure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
