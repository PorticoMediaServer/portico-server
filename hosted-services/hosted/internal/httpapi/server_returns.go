package httpapi

import (
	"context"
	"net/http"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
	"time"
)

type ServerReturns struct {
	Store     *postgres.Store
	Directory *directory.Service
}

func returnFailure(w http.ResponseWriter) {
	fail(w, 401, "invalid_return", "This return is no longer valid. Start again.", false)
}
func (h *Handler) serverReturnRoutes(mux *http.ServeMux) {
	if h.ServerReturns == nil {
		return
	}
	mux.HandleFunc("POST /v1/servers/{id}/return-registration", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		if r.URL.RawQuery != "" {
			returnFailure(w)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		var q serverreturn.RegistrationRequest
		if !decode(w, r, &q) {
			return
		}
		out, e := h.ServerReturns.Store.ServerReturnRegistration(ctx, a, q)
		if e != nil {
			returnFailure(w)
			return
		}
		write(w, 200, out)
	}))
	prepare := func(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
		if !h.allowAuth(r) {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "rate_limited", "Try again shortly.", true)
			return r, false
		}
		if r.URL.RawQuery != "" {
			returnFailure(w)
			return r, false
		}
		return r, true
	}
	authority := func(w http.ResponseWriter, r *http.Request) (identity.CurrentFamilyExpectation, bool) {
		token := bearer(r)
		if token == "" {
			returnFailure(w)
			return identity.CurrentFamilyExpectation{}, false
		}
		a, e := h.ServerReturns.Store.CaptureClaimAuthority(r.Context(), security.Hash(token))
		if e != nil {
			returnFailure(w)
			return a, false
		}
		return a, true
	}
	mux.HandleFunc("POST /v1/server-returns/start", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := prepare(w, r); !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		a, ok := authority(w, r)
		if !ok {
			return
		}
		var q serverreturn.Start
		if !decode(w, r, &q) {
			return
		}
		out, e := h.ServerReturns.Store.StartServerReturn(ctx, a, q)
		if e != nil {
			returnFailure(w)
			return
		}
		write(w, 201, out)
	})
	mux.HandleFunc("POST /v1/server-returns/{id}/complete", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := prepare(w, r); !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		a, ok := authority(w, r)
		if !ok {
			return
		}
		var body serverreturn.Complete
		if !decode(w, r, &body) {
			return
		}
		out, e := h.ServerReturns.Store.CompleteServerReturn(ctx, a, r.PathValue("id"))
		if e != nil {
			returnFailure(w)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/server-returns/exchange", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := prepare(w, r); !ok {
			return
		}
		if len(r.Cookies()) != 0 {
			returnFailure(w)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		var q serverreturn.ExchangeRequest
		if !decode(w, r, &q) {
			return
		}
		var out serverreturn.Grant
		var e error
		if q.Owner != nil || q.Setup != nil {
			if !h.allowCredentialLookup(w, r, "server") {
				return
			}
			token, ok := claimBearer(r)
			if !ok {
				returnFailure(w)
				return
			}
			a, err := h.ServerReturns.Directory.CaptureServerAuthority(ctx, token)
			if err != nil {
				returnFailure(w)
				return
			}
			if !h.allowAttempt("server:"+a.ServerID, 120) {
				w.Header().Set("Retry-After", "60")
				fail(w, 429, "rate_limited", "Try again shortly.", true)
				return
			}
			if q.Setup != nil {
				out, e = h.ServerReturns.Store.ExchangeSetupReturn(ctx, a, *q.Setup, h.ServerReturns.Directory.SignServerReturn)
			} else {
				out, e = h.ServerReturns.Store.ExchangeOwnerReturn(ctx, a, *q.Owner, h.ServerReturns.Directory.SignServerReturn)
			}
		} else {
			if len(r.Header.Values("Authorization")) != 0 || q.Registered == nil {
				returnFailure(w)
				return
			}
			out, e = h.ServerReturns.Store.ExchangeServerReturn(ctx, *q.Registered, h.ServerReturns.Directory.SignServerReturn)
		}
		if e != nil {
			returnFailure(w)
			return
		}
		write(w, 200, out)
	})
}
