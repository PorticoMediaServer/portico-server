package httpapi

import (
	"net/http"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
)

func (h *Handler) endpointRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/servers/{id}/routes", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		var q directory.RouteObservation
		if !decode(w, r, &q) {
			return
		}
		out, e := h.Directory.ObserveRoutes(r.Context(), a, q)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
	mux.HandleFunc("POST /v1/servers/{id}/routes/prove", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		var q RouteProofRequest
		if !decode(w, r, &q) {
			return
		}
		out, e := h.Directory.RegisterRouteEndpoint(r.Context(), a, q.Generation, q.Endpoint)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
	mux.HandleFunc("GET /v1/servers/{id}/routes", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) {
		out, e := h.Directory.Routes(r.Context(), v.Account.ID, r.PathValue("id"))
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, out)
	}))
	mux.HandleFunc("POST /v1/servers/{id}/endpoint", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		var q directory.EndpointRequest
		if !decode(w, r, &q) {
			return
		}
		out, err := h.Directory.RegisterEndpoint(r.Context(), a, q)
		if err != nil {
			failure(w, err)
			return
		}
		write(w, http.StatusOK, out)
	}))
}
