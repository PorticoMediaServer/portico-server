package httpapi

import (
	"net/http"

	"portico.local/hosted/internal/identity"
)

func (h *Handler) lifecycleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/account/name", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) {
		var q DisplayNameRequest
		if !decode(w, r, &q) {
			return
		}
		if e := h.Identity.RenameAccount(r.Context(), v.Account.ID, q.DisplayName); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	}))
	// Password changes use the same purpose-bound proof path, including MFA.
	mux.HandleFunc("POST /v1/account/password", h.securityAction)
	// Membership and profiles are authoritative on each server.
}
