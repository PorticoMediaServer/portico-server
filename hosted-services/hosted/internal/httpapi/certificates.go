package httpapi

import (
	"encoding/base64"
	"errors"
	"net/http"

	"portico.local/hosted/internal/certificates"
	"portico.local/hosted/internal/directory"
)

func certificateFailure(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, certificates.ErrNotFound):
		fail(w, 404, "certificate_not_found", "No certificate request exists with this identity.", false)
	case errors.Is(e, certificates.ErrInvalid):
		fail(w, 400, "invalid_certificate_request", "Use the assigned namespace and a signed P-256 CSR.", false)
	case errors.Is(e, certificates.ErrConflict):
		fail(w, 409, "certificate_conflict", "Resume the existing certificate request before starting another.", false)
	case errors.Is(e, certificates.ErrRateLimited):
		w.Header().Set("Retry-After", "3600")
		fail(w, 429, "certificate_not_due", "Certificate issuance is not due yet.", true)
	case errors.Is(e, certificates.ErrConfiguration):
		w.Header().Set("Retry-After", "300")
		fail(w, 503, "certificate_configuration_required", "The Hosted certificate coordinator needs operator configuration.", true)
	default:
		failure(w, e)
	}
}
func (h *Handler) certificateRoutes(mux *http.ServeMux) {
	s := h.Certificates
	if s == nil {
		return
	}
	mux.HandleFunc("POST /v1/servers/{id}/certificates/namespace", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		n, e := s.Namespace(r.Context(), a)
		if e != nil {
			certificateFailure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, n)
	}))
	mux.HandleFunc("POST /v1/servers/{id}/certificates/orders/lookup", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		var q CertificateLookupRequest
		if !decode(w, r, &q) {
			return
		}
		out, e := s.Lookup(r.Context(), a, q.RequestID)
		if e != nil {
			certificateFailure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, out)
	}))
	mux.HandleFunc("POST /v1/servers/{id}/certificates/orders", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		var q CertificateOrderRequest
		if !decode(w, r, &q) {
			return
		}
		// The same decoding encoding/json applies to a []byte field, and the same
		// rejection decode gives it.
		csr, e := base64.StdEncoding.DecodeString(q.CSR)
		if e != nil {
			fail(w, 400, "invalid_json", "Invalid request.", false)
			return
		}
		out, e := s.Submit(r.Context(), a, q.RequestID, csr)
		if e != nil {
			certificateFailure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 202, out)
	}))
	mux.HandleFunc("GET /v1/servers/{id}/certificates/orders/{order}", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		out, e := s.Status(r.Context(), a, r.PathValue("order"))
		if e != nil {
			certificateFailure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, out)
	}))
	mux.HandleFunc("GET /v1/servers/{id}/certificates/orders/{order}/chain", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		out, e := s.Chain(r.Context(), a, r.PathValue("order"))
		if e != nil {
			certificateFailure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, CertificateChain{Chain: base64.StdEncoding.EncodeToString(out)})
	}))
	mux.HandleFunc("POST /v1/servers/{id}/certificates/orders/{order}/cancel", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		if e := s.Cancel(r.Context(), a, r.PathValue("order")); e != nil {
			certificateFailure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, CancelRequestedResponse{CancelRequested: true})
	}))
}
