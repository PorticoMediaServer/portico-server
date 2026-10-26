package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"strings"
	"time"
)

func (h *Handler) identityCookieName(r *http.Request) (string, bool) {
	if _, secure, ok := h.browserCookiePolicy(r); ok && !secure {
		return "portico_identity_dev", secure
	}
	return "__Host-portico_identity", true
}
func (h *Handler) oidcBinding(r *http.Request) string {
	if r.Method == http.MethodPost && r.URL.Path == "/auth/sso/apple/callback" {
		return h.appleCallbackBinding(r)
	}
	var value string
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == "__Host-portico_identity" || (h.DevelopmentCookies && c.Name == "portico_identity_dev") {
			value = c.Value
			count++
		}
	}
	if count != 1 || !continuationPattern.MatchString(value) {
		return ""
	}
	return value
}

const appleCallbackCookie = "__Secure-portico_apple_callback"

func (h *Handler) appleCallbackBinding(r *http.Request) string {
	var value string
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == appleCallbackCookie {
			value = c.Value
			count++
		}
	}
	if count != 1 || !continuationPattern.MatchString(value) {
		return ""
	}
	return value
}

func setAppleCallbackCookie(w http.ResponseWriter, value string) {
	c := &http.Cookie{Name: appleCallbackCookie, Value: value, Secure: true, HttpOnly: true, Path: "/auth/sso/apple/callback", SameSite: http.SameSiteNoneMode, MaxAge: 600}
	if value == "" {
		c.MaxAge = -1
		c.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, c)
}
func (h *Handler) setIdentityCookie(w http.ResponseWriter, r *http.Request, value string) {
	name, secure := h.identityCookieName(r)
	sameSite := http.SameSiteLaxMode
	cookie := &http.Cookie{Name: name, Value: value, Secure: secure, HttpOnly: true, Path: "/", SameSite: sameSite, MaxAge: 86400}
	if value == "" {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, cookie)
}
func (h *Handler) oidcCallback(r *http.Request) bool {
	return h.OIDC != nil && ((r.Method == "GET" && r.URL.Path == "/auth/sso/google/callback") || (r.Method == "POST" && r.URL.Path == "/auth/sso/apple/callback"))
}
func (h *Handler) oidcRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/identity/options", func(w http.ResponseWriter, r *http.Request) {
		providers := []IdentityProvider{}
		for _, id := range []string{"google", "apple"} {
			enabled, native := false, false
			nativeIDs := []string{}
			if h.OIDC != nil {
				p := h.OIDC.Providers[id]
				enabled = p != nil
				native = p != nil && p.NativeAudience != ""
				if p != nil {
					nativeIDs = append(nativeIDs, p.NativeAudiences...)
				}
			}
			providers = append(providers, IdentityProvider{Enabled: enabled, ID: id, Native: native, NativeClientIDs: nativeIDs})
		}
		mail := h.Security != nil && h.Security.MailEnabled
		write(w, 200, IdentityOptions{AccountSecurity: h.Security != nil, NativeCompletion: postgres.NativeCompletionCode, Password: h.Security != nil, Providers: providers, Recovery: mail, Registration: mail, VerifiedContactRequired: true})
	})
	if h.OIDC == nil {
		return
	}
	h.securityReturnRoutes(mux)
	mux.HandleFunc("POST /v1/identity/{provider}/start", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowAuth(r) {
			failure(w, identity.ErrRateLimited)
			return
		}
		var q postgres.OIDCStartInput
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
		v, authorization, nonce, e := h.OIDC.StartFlow(r.Context(), r.PathValue("provider"), binding, bearer(r), q)
		if e != nil {
			failure(w, e)
			return
		}
		if r.PathValue("provider") == "apple" {
			setAppleCallbackCookie(w, binding)
		}
		write(w, 200, IdentityStart{AuthorizationURL: authorization, ExpiresAt: v.Expires, Mode: v.Mode, NativeNonce: nonce, TransactionID: v.ID})
	})
	callback := func(w http.ResponseWriter, r *http.Request) {
		bad := func() { http.Redirect(w, r, h.AccountOrigin+"/account?identityError=1", http.StatusSeeOther) }
		if !h.allowAuth(r) {
			bad()
			return
		}
		provider := "google"
		if r.Method == "POST" {
			provider = "apple"
			// A57: clear the one-use callback cookie before any redirect writes
			// the headers. The binding is read from the request, not the response.
			setAppleCallbackCookie(w, "")
			if r.Header.Get("Origin") != "https://appleid.apple.com" || strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/x-www-form-urlencoded" {
				bad()
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 24<<10)
		if e := r.ParseForm(); e != nil {
			bad()
			return
		}
		if len(r.Form["state"]) != 1 || len(r.Form["code"]) != 1 || r.Form.Get("error") != "" {
			bad()
			return
		}
		name := ""
		if provider == "apple" && len(r.Form.Get("user")) <= 4096 {
			var user struct {
				Name struct {
					First string `json:"firstName"`
					Last  string `json:"lastName"`
				} `json:"name"`
			}
			if json.Unmarshal([]byte(r.Form.Get("user")), &user) == nil {
				name = strings.TrimSpace(user.Name.First + " " + user.Name.Last)
			}
		}
		result, e := h.OIDC.CallbackWithName(r.Context(), provider, h.oidcBinding(r), r.Form.Get("state"), r.Form.Get("code"), name)
		if e != nil {
			bad()
			return
		}
		target := h.AccountOrigin + "/account?identityTransaction=" + url.QueryEscape(result.TransactionID)
		if result.SessionMode == "native" && result.CompletionCode != "" {
			// Claimed HTTPS app link only. A custom scheme can be hijacked by
			// another installed app; polling never reveals this callback proof.
			target = h.AccountOrigin + "/app/account-return#" + url.Values{"tx": {result.TransactionID}, "code": {result.CompletionCode}}.Encode()
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
	}
	mux.HandleFunc("GET /auth/sso/google/callback", callback)
	mux.HandleFunc("POST /auth/sso/apple/callback", callback)
	view := func(w http.ResponseWriter, r *http.Request) {
		if h.oidcBinding(r) == "" && r.Header.Get("X-Portico-Continuation") == "" && r.PathValue("id") == "" {
			write(w, 200, nil)
			return
		}
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		// Native apps present the callback's completion code in a header (never
		// the URL) to see onboarding details; without it they see the status.
		v, e := h.OIDC.View(r.Context(), r.PathValue("id"), binding, r.Header.Get("X-Portico-Completion"))
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	}
	mux.HandleFunc("GET /v1/identity/current", view)
	mux.HandleFunc("GET /v1/identity/transactions/{id}", view)
	mux.HandleFunc("POST /v1/identity/transactions/{id}/complete", func(w http.ResponseWriter, r *http.Request) {
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		var q postgres.OIDCCompleteInput
		if !decode(w, r, &q) {
			return
		}
		result, e := h.OIDC.CompleteFlow(r.Context(), r.PathValue("id"), binding, bearer(r), q, true)
		if e != nil {
			failure(w, e)
			return
		}
		if result.Session != nil {
			h.sessionResponse(w, r, 200, *result.Session)
		} else if result.Challenge != nil {
			write(w, 202, result.Challenge)
		} else {
			write(w, 200, result)
		}
	})
	mux.HandleFunc("POST /v1/identity/transactions/{id}/native-apple", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			fail(w, 403, "native_required", "Use the native Apple sign-in flow.", false)
			return
		}
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		var q NativeAppleRequest
		if !decode(w, r, &q) {
			return
		}
		v, e := h.OIDC.NativeAssertion(r.Context(), r.PathValue("id"), binding, q.IDToken, q.DisplayName)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	})
	mux.HandleFunc("POST /v1/identity/transactions/{id}/contact", func(w http.ResponseWriter, r *http.Request) {
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		var q ContactRequest
		if !decode(w, r, &q) {
			return
		}
		v, e := h.OIDC.SendContact(r.Context(), r.PathValue("id"), binding, q.ContactAddress, q.RequestID)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 202, v)
	})
	mux.HandleFunc("POST /v1/identity/transactions/{id}/contact/verify", func(w http.ResponseWriter, r *http.Request) {
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		var q ChallengeCodeRequest
		if !decode(w, r, &q) {
			return
		}
		v, e := h.OIDC.VerifyContact(r.Context(), r.PathValue("id"), binding, q.ChallengeID, q.Code)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, v)
	})
	mux.HandleFunc("POST /v1/identity/cancel", func(w http.ResponseWriter, r *http.Request) {
		binding, ok := h.continuation(w, r, false)
		if !ok {
			return
		}
		if e := h.OIDC.Cancel(r.Context(), binding); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	})
}
