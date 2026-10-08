package httpapi

import (
	"net/http"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/postgres"
)

func (h *Handler) deletionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/account/deletion/preview", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		var q postgres.SecurityAction
		if !decode(w, r, &q) {
			return
		}
		if q.Purpose != "account_deletion_preview" {
			fail(w, 400, "invalid_input", "Request a deletion preview proof.", false)
			return
		}
		v, e := h.Security.Mutate(r.Context(), bearer(r), q)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v.DeletionPreview)
	})
	mux.HandleFunc("POST /v1/account/deletion", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		var q AccountDeletionRequest
		if !decode(w, r, &q) {
			return
		}
		v, e := h.Security.AcceptDeletion(r.Context(), bearer(r), q.Token, q.Disposition, q.Confirmation)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	})
	mux.HandleFunc("POST /v1/account/deletion/status", func(w http.ResponseWriter, r *http.Request) {
		var q DeletionTokenRequest
		if !decode(w, r, &q) {
			return
		}
		v, e := h.Identity.DeletionStatus(r.Context(), q.Token)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	})
	mux.HandleFunc("GET /v1/servers/{id}/erasures", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		v, e := h.Directory.ServerErasures(r.Context(), a)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, ErasureList{Items: v})
	}))
	mux.HandleFunc("POST /v1/servers/{id}/erasures/{erasure}/ack", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		var q RevisionRequest
		if !decode(w, r, &q) {
			return
		}
		if e := h.Directory.ServerAcknowledgeErasure(r.Context(), a, r.PathValue("erasure"), q.Revision); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	}))
}
