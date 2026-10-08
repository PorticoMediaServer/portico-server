package httpapi

import (
	"net/http"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
)

func (h *Handler) membershipLifecycleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("DELETE /v1/servers/{server}", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		if !h.securityReady(w) {
			return
		}
		var q postgres.ServerRemoval
		if !decode(w, r, &q) {
			return
		}
		if e := h.Security.RemoveServer(r.Context(), bearer(r), r.PathValue("server"), q); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, StatusCodeResponse{Code: "server_removed", Status: "retired"})
	}))
	// Membership, leaving and ownership are the server's own now (Spec —
	// Hosted at Scale); Hosted keeps retirement of the claim it issued.
}
