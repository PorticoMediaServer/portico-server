package httpapi

import (
	"net/http"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/serverreturn"
)

type SecurityReturnStartResponse struct {
	IdentityStart
	postgres.SecurityReturnDisplay
	Provider string `json:"provider"`
}

func (h *Handler) securityReturnRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/identity/transactions/{id}/cancel-security", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowAuth(r) {
			failure(w, identity.ErrRateLimited)
			return
		}
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		if r.URL.RawQuery != "" || r.Header.Get("Origin") != h.AccountOrigin || r.Header.Get("X-Portico-Continuation") != "" {
			failure(w, identity.ErrCredentials)
			return
		}
		var q struct{}
		if !decode(w, r, &q) {
			return
		}
		if e := h.OIDC.CancelSecurityTransaction(r.Context(), r.PathValue("id"), binding); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /v1/account/security-return-intents/{id}/cancel", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		if r.URL.RawQuery != "" {
			failure(w, identity.ErrInvalid)
			return
		}
		var q struct{}
		if !decode(w, r, &q) {
			return
		}
		if e := h.OIDC.CancelSecurityReturnIntent(r.Context(), r.PathValue("id"), bearer(r), r.Header.Get("Origin")); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /v1/account/security-return-intents", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		if r.URL.RawQuery != "" {
			failure(w, identity.ErrInvalid)
			return
		}
		var q serverreturn.SecurityReturnStart
		if !decode(w, r, &q) {
			return
		}
		out, e := h.OIDC.PrepareSecurityReturnIntent(r.Context(), bearer(r), r.Header.Get("Origin"), h.AccountOrigin, q)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 201, out)
	}))
	mux.HandleFunc("POST /v1/account/security-return-intents/{id}/start", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowAuth(r) {
			failure(w, identity.ErrRateLimited)
			return
		}
		binding, ok := h.continuation(w, r, true)
		if !ok {
			return
		}
		// This endpoint is browser-only on the fixed Hosted origin. It does not
		// accept a native binding or an Origin chosen from the intent body.
		if r.Header.Get("Origin") != h.AccountOrigin || r.Header.Get("X-Portico-Continuation") != "" {
			failure(w, identity.ErrCredentials)
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, identity.ErrInvalid)
			return
		}
		var q RequestIDRequest
		if !decode(w, r, &q) {
			return
		}
		origin, e := h.OIDC.SecurityReturnIntentOrigin(r.Context(), r.PathValue("id"))
		if e != nil {
			failure(w, e)
			return
		}
		name := h.browserCookieName(origin, true)
		cookie, count := "", 0
		for _, c := range r.Cookies() {
			if c.Name == name {
				cookie = c.Value
				count++
			}
		}
		if count != 1 || cookie == "" {
			failure(w, identity.ErrCredentials)
			return
		}
		v, location, e := h.OIDC.StartSecurityReturnFlow(r.Context(), r.PathValue("id"), binding, cookie, q.RequestID)
		if e != nil {
			failure(w, e)
			return
		}
		display, e := h.OIDC.SecurityReturnDisplay(r.Context(), r.PathValue("id"), v.ID)
		if e != nil {
			failure(w, e)
			return
		}
		if v.Provider == "apple" {
			setAppleCallbackCookie(w, binding)
		}
		write(w, 200, SecurityReturnStartResponse{IdentityStart: IdentityStart{AuthorizationURL: location, ExpiresAt: v.Expires, Mode: v.Mode, TransactionID: v.ID}, SecurityReturnDisplay: display, Provider: v.Provider})
	})
	mux.HandleFunc("POST /v1/identity/transactions/{id}/security-return", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowAuth(r) {
			failure(w, identity.ErrRateLimited)
			return
		}
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		if r.Header.Get("Origin") != h.AccountOrigin || r.Header.Get("X-Portico-Continuation") != "" {
			failure(w, identity.ErrCredentials)
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, identity.ErrInvalid)
			return
		}
		var q struct{}
		if !decode(w, r, &q) {
			return
		}
		out, e := h.OIDC.FinishSecurityReturn(r.Context(), r.PathValue("id"), binding)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/account/security-returns/{id}/complete", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		if r.URL.RawQuery != "" {
			failure(w, identity.ErrInvalid)
			return
		}
		var q postgres.SecurityReturnCompleteInput
		if !decode(w, r, &q) {
			return
		}
		out, e := h.OIDC.CompleteSecurityReturnFlow(r.Context(), r.PathValue("id"), bearer(r), r.Header.Get("Origin"), q)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
}
