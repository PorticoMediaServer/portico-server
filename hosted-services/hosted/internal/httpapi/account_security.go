package httpapi

import (
	"net/http"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"regexp"
)

var continuationPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func (h *Handler) securityReady(w http.ResponseWriter) bool {
	if h.Security != nil {
		return true
	}
	fail(w, 503, "unavailable", "Account security is not configured.", true)
	return false
}
func (h *Handler) credentialOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == h.AccountOrigin {
		return true
	}
	if _, secure, ok := h.browserCookiePolicy(r); h.DevelopmentCookies && ok && !secure {
		return true
	}
	fail(w, 403, "origin_denied", "Use the Portico Account website to sign in.", false)
	return false
}

// Native clients supply an independently random, privately stored binding. A
// browser never receives it in JSON and cannot override its HttpOnly binding.
func (h *Handler) continuation(w http.ResponseWriter, r *http.Request, create bool) (string, bool) {
	native := r.Header.Get("X-Portico-Continuation")
	if native != "" {
		if r.Header.Get("Origin") != "" || h.oidcBinding(r) != "" || !continuationPattern.MatchString(native) {
			fail(w, 403, "continuation_denied", "Start account verification again.", false)
			return "", false
		}
		return native, true
	}
	// Identity cookies are shared for provider callbacks, not authority for media
	// origins to inspect account continuations. Apply the origin fence to reads too.
	if !h.credentialOrigin(w, r) {
		return "", false
	}
	binding := h.oidcBinding(r)
	if r.Method == http.MethodGet && binding != "" {
		return binding, true
	}
	if r.Header.Get("Origin") == "" {
		fail(w, 403, "continuation_required", "A private account continuation is required.", false)
		return "", false
	}
	if binding == "" && create {
		binding = security.RandomID("")
		h.setIdentityCookie(w, r, binding)
	}
	if binding == "" {
		fail(w, 401, "continuation_required", "Start account verification again.", false)
		return "", false
	}
	return binding, true
}
func (h *Handler) registrationCreate(w http.ResponseWriter, r *http.Request) {
	if !h.securityReady(w) {
		return
	}
	if !h.allowAuth(r) {
		failure(w, identity.ErrRateLimited)
		return
	}
	var q postgres.RegistrationInput
	if !decode(w, r, &q) {
		return
	}
	if !h.validSessionMode(w, r, q.SessionMode) {
		return
	}
	binding, ok := h.continuation(w, r, true)
	if !ok {
		return
	}
	result, e := h.Security.CreateRegistration(r.Context(), binding, q)
	if e != nil {
		failure(w, e)
		return
	}
	write(w, 202, result)
}
func (h *Handler) securityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/registrations", h.registrationCreate)
	mux.HandleFunc("GET /v1/registrations/current", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		if h.oidcBinding(r) == "" && r.Header.Get("X-Portico-Continuation") == "" {
			write(w, 200, nil)
			return
		}
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		v, e := h.Security.CurrentRegistration(r.Context(), binding)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	})
	mux.HandleFunc("GET /v1/registrations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		v, e := h.Security.RegistrationView(r.Context(), r.PathValue("id"), binding)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	})
	// A33: a mailed verification link, opened on any device. The token's code
	// proves control of the address; no browser binding is needed.
	mux.HandleFunc("POST /v1/email-links/confirm", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		if !h.allowAttempt("email-link:"+budgetIP(peer(r)), 30) {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "rate_limited", "Try again shortly.", true)
			return
		}
		var q TokenRequest
		if !decode(w, r, &q) {
			return
		}
		kind, resource, code, e := mailoutbox.ParseVerificationToken(q.Token)
		if e != nil {
			fail(w, 400, "invalid_link", "This link is not valid.", false)
			return
		}
		v, e := h.Security.ConfirmEmailLink(r.Context(), string(kind), resource, code)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	})
	mux.HandleFunc("POST /v1/registrations/{id}/verify", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		var q RegistrationVerifyRequest
		if !decode(w, r, &q) {
			return
		}
		v, e := h.Security.VerifyRegistration(r.Context(), r.PathValue("id"), binding, q.Code, q.Username, q.DisplayName)
		if e != nil {
			failure(w, e)
			return
		}
		h.sessionResponse(w, r, 201, v)
	})
	mux.HandleFunc("POST /v1/registrations/{id}/resend", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		var q RequestIDRequest
		if !decode(w, r, &q) {
			return
		}
		v, e := h.Security.ResendRegistration(r.Context(), r.PathValue("id"), binding, q.RequestID)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 202, v)
	})
	mux.HandleFunc("POST /v1/registrations/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		if e := h.Security.CancelRegistration(r.Context(), binding); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /v1/account/recovery/request", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) || !h.credentialOrigin(w, r) {
			return
		}
		if !h.allowAuth(r) {
			failure(w, identity.ErrRateLimited)
			return
		}
		var q postgres.RecoveryRequest
		if !decode(w, r, &q) {
			return
		}
		if e := h.Security.RequestRecovery(r.Context(), q); e != nil {
			failure(w, e)
			return
		}
		write(w, 202, StatusMessageResponse{Message: "If these details match a password account, recovery instructions will be sent. For provider-only accounts, recover access with Google or Apple.", Status: "requested"})
	})
	mux.HandleFunc("POST /v1/account/recovery/complete", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) || !h.credentialOrigin(w, r) {
			return
		}
		if !h.allowAuth(r) {
			failure(w, identity.ErrRateLimited)
			return
		}
		var q RecoveryCompleteRequest
		if !decode(w, r, &q) {
			return
		}
		if e := h.Security.ResetPassword(r.Context(), q.Token, q.NewPassword, q.Factor); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, StatusResponse{Status: "password_reset"})
	})
	mux.HandleFunc("POST /v1/identity/mfa/complete", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) || !h.credentialOrigin(w, r) {
			return
		}
		if !h.allowAuth(r) {
			failure(w, identity.ErrRateLimited)
			return
		}
		var q MFACompleteRequest
		if !decode(w, r, &q) {
			return
		}
		v, e := h.Security.CompleteMFA(r.Context(), q.ChallengeID, q.ChallengeToken, q.Factor)
		if e != nil {
			failure(w, e)
			return
		}
		h.sessionResponse(w, r, 200, v)
	})
	mux.HandleFunc("GET /v1/account/security", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		v, e := h.Security.State(r.Context(), bearer(r))
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	})
	mux.HandleFunc("POST /v1/account/security/proof", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		if !h.allowAuth(r) {
			failure(w, identity.ErrRateLimited)
			return
		}
		var q SecurityProofRequest
		if !decode(w, r, &q) {
			return
		}
		v, e := h.Security.PasswordProof(r.Context(), bearer(r), q.Intent, q.Password, q.Factor)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	})
	mux.HandleFunc("POST /v1/account/security/actions", h.securityAction)
	mux.HandleFunc("POST /v1/account/security/mfa/confirm", func(w http.ResponseWriter, r *http.Request) { h.securityConfirm(w, r, true) })
	mux.HandleFunc("POST /v1/account/security/email/confirm", func(w http.ResponseWriter, r *http.Request) { h.securityConfirm(w, r, false) })
	mux.HandleFunc("POST /v1/account/security/email/resend", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		if !h.allowAuth(r) {
			failure(w, identity.ErrRateLimited)
			return
		}
		var q SecurityEmailResendRequest
		if !decode(w, r, &q) {
			return
		}
		v, e := h.Security.ResendEmailChange(r.Context(), bearer(r), q.ChallengeID, q.OperationID)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	})
}
func (h *Handler) securityAction(w http.ResponseWriter, r *http.Request) {
	if !h.securityReady(w) {
		return
	}
	var q postgres.SecurityAction
	if !decode(w, r, &q) {
		return
	}
	v, e := h.Security.Mutate(r.Context(), bearer(r), q)
	if e != nil {
		failure(w, e)
		return
	}
	write(w, 200, v)
	// Do not clear a shared browser cookie here: a delayed response for account A
	// must not erase account B's subsequently installed family. Clients reconcile
	// only the returned accountId/familyId, through their credential coordinator.
}
func (h *Handler) securityConfirm(w http.ResponseWriter, r *http.Request, mfa bool) {
	if !h.securityReady(w) {
		return
	}
	var q SecurityConfirmRequest
	if !decode(w, r, &q) {
		return
	}
	var v identity.SecurityResult
	var e error
	if mfa {
		v, e = h.Security.ConfirmEnrollment(r.Context(), bearer(r), q.ChallengeID, q.Code, q.OperationID)
	} else {
		v, e = h.Security.ConfirmEmailChange(r.Context(), bearer(r), q.ChallengeID, q.Code, q.OperationID)
	}
	if e != nil {
		failure(w, e)
		return
	}
	write(w, 200, v)
}
