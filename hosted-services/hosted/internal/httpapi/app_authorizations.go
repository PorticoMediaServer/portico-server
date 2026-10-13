package httpapi

import (
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"portico.local/hosted/internal/appauth"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/packages/i18n"
)

//go:embed appauthassets/*
var appAuthorizationAssets embed.FS

var appAuthorizationPage = template.Must(template.New("authorize").Parse(appPageHTML))

type AppAuthorizations struct {
	Store     *postgres.Store
	Directory *directory.Service
}

func appAuthorizationQuery(r *http.Request) (appauth.Request, error) {
	var q appauth.Request
	if len(r.URL.RawQuery) > 8192 {
		return q, appauth.ErrGrant
	}
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return q, appauth.ErrGrant
	}
	fields := map[string]*string{"serverId": &q.ServerID, "challenge": &q.Challenge, "redirectUri": &q.RedirectURI, "state": &q.State, "appName": &q.AppName, "codeChallenge": &q.CodeChallenge}
	if len(values) != len(fields) {
		return q, appauth.ErrGrant
	}
	for k, v := range values {
		target, ok := fields[k]
		if !ok || len(v) != 1 {
			return q, appauth.ErrGrant
		}
		*target = v[0]
	}
	return q, q.Validate()
}
func appAuthorizationQueryAllowed(r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Path != "/auth/apps/authorize" {
		return false
	}
	_, e := appAuthorizationQuery(r)
	return e == nil
}
func (h *Handler) appAuthorizationRate(w http.ResponseWriter, r *http.Request) bool {
	address := h.allowAttempt("appauth:"+budgetIP(peer(r)), 60)
	network := h.allowAttempt("appauthnet:"+budgetNetwork(peer(r)), 600)
	if !address || !network {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "rate_limited", "Try again shortly.", true)
		return false
	}
	return true
}

// Canonical account cookies alone authorize consent. Other allowed media
// origins, bearer tokens and native refresh credentials cannot approve it.
func (h *Handler) appAuthorizationCookie(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	canonical := h.AccountOrigin
	if canonical == "" {
		canonical = "https://web.getportico.tv"
	}
	if r.Header.Get("Origin") != canonical || bearer(r) != "" {
		fail(w, 403, "origin_denied", i18n.AppSignIn["accountPage"], false)
		return nil, false
	}
	token, mode, ok := h.refreshInput(w, r, "")
	if !ok {
		return nil, false
	}
	if mode != "browser" {
		failure(w, appauth.ErrGrant)
		return nil, false
	}
	return security.Hash(token), true
}
func (h *Handler) appAuthorizationRoutes(mux *http.ServeMux) {
	if h.AppAuthorizations == nil {
		return
	}
	mux.HandleFunc("GET /auth/apps/authorize", func(w http.ResponseWriter, r *http.Request) {
		if !h.appAuthorizationRate(w, r) {
			return
		}
		q, e := appAuthorizationQuery(r)
		if e != nil {
			failure(w, e)
			return
		}
		raw, e := json.Marshal(q)
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		_ = appAuthorizationPage.Execute(w, struct {
			Request, Strings string
			Text             map[string]string
		}{string(raw), i18n.AppSignInJSON(), i18n.AppSignIn})
	})
	mux.HandleFunc("GET /auth/apps/authorize.js", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowCredentialLookup(w, r, "appasset") {
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		raw, _ := appAuthorizationAssets.ReadFile("appauthassets/authorize.js")
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("GET /auth/apps/authorize.css", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowCredentialLookup(w, r, "appasset") {
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		raw, _ := appAuthorizationAssets.ReadFile("appauthassets/authorize.css")
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("POST /v1/app-authorizations", func(w http.ResponseWriter, r *http.Request) {
		if !h.appAuthorizationRate(w, r) {
			return
		}
		cookie, ok := h.appAuthorizationCookie(w, r)
		if !ok {
			return
		}
		var q appauth.Request
		if !decode(w, r, &q) {
			return
		}
		out, e := h.AppAuthorizations.Store.ReviewAppAuthorization(r.Context(), cookie, q)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 201, out)
	})
	mux.HandleFunc("POST /v1/app-authorizations/{id}/decision", func(w http.ResponseWriter, r *http.Request) {
		if !h.appAuthorizationRate(w, r) {
			return
		}
		cookie, ok := h.appAuthorizationCookie(w, r)
		if !ok {
			return
		}
		var q appauth.Decision
		if !decode(w, r, &q) {
			return
		}
		out, e := h.AppAuthorizations.Store.DecideAppAuthorization(r.Context(), cookie, r.PathValue("id"), q)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/app-authorizations/token", func(w http.ResponseWriter, r *http.Request) {
		if !h.appAuthorizationRate(w, r) {
			return
		}
		// This credentialless exchange never accepts or emits browser credentials.
		if bearer(r) != "" {
			failure(w, appauth.ErrGrant)
			return
		}
		for _, c := range r.Cookies() {
			if strings.HasPrefix(c.Name, "__Host-portico_refresh") || strings.HasPrefix(c.Name, "portico_refresh_dev") {
				failure(w, appauth.ErrGrant)
				return
			}
		}
		var q appauth.Exchange
		if !decode(w, r, &q) {
			return
		}
		out, e := h.AppAuthorizations.Store.ExchangeAppAuthorization(r.Context(), q, func(a identity.Account, server, challenge string) (directory.SignedPolicy, error) {
			return h.AppAuthorizations.Directory.IssueIdentity(a.ID, a.Username, a.DisplayName, server, challenge, "")
		})
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
}

// App exchange is a public-client API. Browser callers use credentials: omit;
// wildcard CORS here conveys no account authority, cookies, or directory access.
func appExchange(r *http.Request) bool {
	return r.URL.Path == "/v1/app-authorizations/token" && (r.Method == http.MethodPost || r.Method == http.MethodOptions)
}

const appPageHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{index .Text "title"}}</title><link rel="stylesheet" href="/auth/apps/authorize.css"><script src="/auth/apps/authorize.js" defer></script></head><body><main><a class="brand" href="/account">Portico</a><h1>{{index .Text "title"}}</h1><section id="signin"><p>{{index .Text "signIn"}}</p><a class="button" href="/account" target="_blank" rel="noopener noreferrer">{{index .Text "openAccount"}}</a><button id="review">{{index .Text "continue"}}</button></section><section id="consent" hidden><p id="consent-line"></p><dl><dt>{{index .Text "account"}}</dt><dd id="account-name"></dd><dt>{{index .Text "returnAddress"}}</dt><dd id="return-address"></dd></dl><button id="approve">{{index .Text "allow"}}</button><button id="deny" class="secondary">{{index .Text "cancel"}}</button></section><p id="status" role="status" aria-live="polite"></p><p id="request" hidden>{{.Request}}</p><p id="strings" hidden>{{.Strings}}</p></main></body></html>`

// appGrantError is separate from account authentication: an expired, spent,
// mistargeted or PKCE-mismatched code has one non-enumerating refusal.
func appGrantError(e error) bool { return errors.Is(e, appauth.ErrGrant) }
