package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"portico.local/server/internal/catalog"
)

// playHistoryRoutes serves the server's play history to its owner: every play,
// newest first, filtered by person, library and time (Settings › Server › Play
// history). A viewer's own History is GET /v1/history.
func (d Dependencies) playHistoryRoutes(mux *http.ServeMux) {
	if d.Catalog == nil || d.Identity == nil {
		return
	}
	mux.HandleFunc("GET /v1/admin/play-history", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.administrator(r); e != nil {
			accessFailure(w, e)
			return
		}
		query := r.URL.Query()
		for key, values := range query {
			if len(values) != 1 || (key != "accountId" && key != "libraryId" && key != "period" && key != "cursor" && key != "limit") {
				failure(w, errors.New("play history accepts accountId, libraryId, period, cursor and limit"))
				return
			}
		}
		limit := 0
		if raw := query.Get("limit"); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 1 {
				failure(w, catalog.ErrCursor)
				return
			}
			limit = n
		}
		out, e := d.Catalog.PlayHistory(r.Context(), catalog.PlayHistoryQuery{AccountID: query.Get("accountId"), LibraryID: query.Get("libraryId"), Period: query.Get("period"), Cursor: query.Get("cursor"), Limit: limit})
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
	// The Dashboard's viewing statistics: totals of the same plays, for a period.
	mux.HandleFunc("GET /v1/admin/play-history/summary", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.administrator(r); e != nil {
			accessFailure(w, e)
			return
		}
		query := r.URL.Query()
		for key, values := range query {
			if len(values) != 1 || key != "period" {
				failure(w, errors.New("the play history summary accepts period"))
				return
			}
		}
		period := query.Get("period")
		if period == "" {
			period = "24h"
		}
		out, e := d.Catalog.PlaySummary(r.Context(), period)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
}
