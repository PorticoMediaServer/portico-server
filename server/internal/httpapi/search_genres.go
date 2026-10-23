package httpapi

import (
	"net/http"
	"strconv"

	"portico.local/server/internal/catalog"
)

// Mounted with its shared descriptor once generated; every surface uses this
// same read rather than deriving counts or representative art from Search hits.
func (d Dependencies) searchGenresHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	for key, values := range query {
		if (key != "limit" && key != "cursor") || len(values) != 1 {
			browseFailure(w, catalog.ErrCursor)
			return
		}
	}
	request := catalog.SearchGenreRequest{Cursor: query.Get("cursor")}
	if query.Has("limit") {
		var err error
		request.Limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil || request.Limit < 1 || request.Limit > catalog.SearchGenresMaximumLimit {
			browseFailure(w, catalog.ErrCursor)
			return
		}
	}
	principal, viewer, err := d.acrossViewer(r)
	if err != nil {
		failure(w, err)
		return
	}
	var out catalog.SearchGenrePage
	err = d.compose(r, viewer.Restrictions, func(cat *catalog.Service) error {
		var err error
		out, err = cat.SearchGenres(viewer, request)
		return err
	})
	if err != nil {
		browseFailure(w, err)
		return
	}
	if err = d.browseAuthorityAfter(r, principal, viewer); err != nil {
		browseFailure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, http.StatusOK, out)
}
