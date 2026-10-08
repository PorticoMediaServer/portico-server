package httpapi

import (
	"net/http"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
)

func (h *Handler) exportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/account/export", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		if !h.securityReady(w) {
			return
		}
		var q postgres.ExportRequest
		if !decode(w, r, &q) {
			return
		}
		out, e := h.Security.RequestExport(r.Context(), bearer(r), q)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 202, out)
	}))
	mux.HandleFunc("GET /v1/account/exports/{export}", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		if !h.securityReady(w) {
			return
		}
		out, _, e := h.Security.Export(r.Context(), bearer(r), r.PathValue("export"), false)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
	mux.HandleFunc("POST /v1/account/exports/{export}/download", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		if !h.securityReady(w) {
			return
		}
		_, data, e := h.Security.Export(r.Context(), bearer(r), r.PathValue("export"), true)
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="portico-account.json"`)
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
	}))
}
