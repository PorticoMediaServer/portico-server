package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/sessions"
)

func sessionReviewQueryAllowed(r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Path != "/v1/account/sessions" || len(r.URL.RawQuery) > 4096 {
		return false
	}
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil || len(values) != 1 {
		return false
	}
	cursors, ok := values["cursor"]
	return ok && len(cursors) == 1 && len(cursors[0]) > 0 && len(cursors[0]) <= 2048
}
func sessionReviewFailure(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, sessions.ErrReviewNotFound):
		fail(w, 404, "session_not_found", "This account session is not available.", false)
	case errors.Is(e, sessions.ErrReviewCursor):
		fail(w, 409, "sessions_changed", "Account sessions changed. Refresh the list.", true)
	case errors.Is(e, sessions.ErrReviewInvalid):
		fail(w, 400, "invalid_input", "Check the supplied session values.", false)
	default:
		failure(w, e)
	}
}
func (h *Handler) sessionReviewRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/account/sessions", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) {
		out, e := h.Sessions.Review(r.Context(), v.Account.ID, bearer(r), r.URL.Query().Get("cursor"))
		if e != nil {
			sessionReviewFailure(w, e)
			return
		}
		write(w, 200, out)
	}))
	// No Set-Cookie: ending a different family must never replace or clear the
	// caller's browser credentials. Current-family cleanup uses normal logout.
	mux.HandleFunc("DELETE /v1/account/sessions/{family}", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) {
		out, e := h.Sessions.RevokeReviewed(r.Context(), v.Account.ID, bearer(r), r.PathValue("family"))
		if e != nil {
			sessionReviewFailure(w, e)
			return
		}
		write(w, 200, out)
	}))
}
