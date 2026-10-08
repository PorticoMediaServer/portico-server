package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/netip"
	"net/url"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/sessions"
	"strings"
	"time"
)

func (h *Handler) browserCookiePolicy(r *http.Request) (string, bool, bool) {
	origin := r.Header.Get("Origin")
	if origin == "" || !h.Origins[origin] {
		return "", false, false
	}
	u, e := url.Parse(origin)
	if e != nil {
		return "", false, false
	}
	if u.Scheme == "https" {
		// A16: the refresh cookie is SameSite=Lax, so a browser sends it only
		// from an origin on the same site as Hosted. A cross-site origin (every
		// <ns>.direct.getportico.tv name once direct.getportico.tv is on the
		// Public Suffix List) would get a session it can never refresh.
		if !h.sameSiteAsAccount(u.Hostname()) {
			return "", false, false
		}
		return h.browserCookieName(origin, true), true, true
	}
	ip, e := netip.ParseAddr(u.Hostname())
	if h.DevelopmentCookies && e == nil && ip.IsLoopback() && u.Scheme == "http" {
		return h.browserCookieName(origin, false), false, true
	}
	return "", false, false
}

// directSuffix is Portico's per-server name space. It is (or will be) a public
// suffix, so every name under it is its own site.
const directSuffix = "direct.getportico.tv"

// sameSiteAsAccount reports whether host is on the account website's site: the
// same registrable domain (last two labels) and not under the direct suffix.
func (h *Handler) sameSiteAsAccount(host string) bool {
	canonical := h.AccountOrigin
	if canonical == "" {
		canonical = "https://web.getportico.tv"
	}
	u, e := url.Parse(canonical)
	if e != nil {
		return false
	}
	host, account := strings.ToLower(strings.TrimSuffix(host, ".")), strings.ToLower(u.Hostname())
	if host == account {
		return true
	}
	if host == directSuffix || strings.HasSuffix(host, "."+directSuffix) {
		return false
	}
	site := func(name string) string {
		labels := strings.Split(name, ".")
		if len(labels) < 2 {
			return name
		}
		return strings.Join(labels[len(labels)-2:], ".")
	}
	return site(host) == site(account)
}

// Media origins receive independent browser-family cookies. They cannot rotate
// or replace the canonical account website's cookie or another media origin's.
func (h *Handler) browserCookieName(origin string, secure bool) string {
	name := "portico_refresh_dev"
	if secure {
		name = "__Host-portico_refresh"
	}
	canonical := h.AccountOrigin
	if canonical == "" {
		canonical = "https://web.getportico.tv"
	}
	if origin == canonical {
		return name
	}
	digest := sha256.Sum256([]byte(origin))
	return name + "_" + hex.EncodeToString(digest[:16])
}
func requireBrowserFamily(w http.ResponseWriter, mode, family string) bool {
	if mode == "browser" && (family == "" || len(family) > 128) {
		fail(w, 400, "family_required", "Use the session family that initiated this request.", false)
		return false
	}
	return true
}
func (h *Handler) validSessionMode(w http.ResponseWriter, r *http.Request, mode string) bool {
	if mode == "native" && r.Header.Get("Origin") == "" {
		return true
	}
	if mode == "browser" {
		if _, _, ok := h.browserCookiePolicy(r); ok {
			return true
		}
	}
	fail(w, 403, "session_mode_denied", "This session mode is not permitted for this origin.", false)
	return false
}
func (h *Handler) sessionResponse(w http.ResponseWriter, r *http.Request, status int, s identity.Session) {
	if !h.validSessionMode(w, r, s.Mode) {
		return
	}
	if s.Mode == "browser" {
		name, secure, ok := h.browserCookiePolicy(r)
		if !ok {
			fail(w, 403, "origin_denied", "Browser origin is required.", false)
			return
		}
		expiry := time.Now().Add(identity.SignInWindow)
		if s.RefreshExpiresAt != nil {
			expiry = *s.RefreshExpiresAt
		}
		http.SetCookie(w, &http.Cookie{Name: name, Value: s.RefreshToken, Path: "/", HttpOnly: true, Secure: secure, SameSite: refreshSameSite(name, secure), Expires: expiry, MaxAge: max(1, int(time.Until(expiry).Seconds()))})
		csrf := security.RandomID("")
		http.SetCookie(w, &http.Cookie{Name: name + "_csrf", Value: csrf, Path: "/", Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: max(1, int(time.Until(expiry).Seconds()))})
		w.Header().Set("X-CSRF-Token", csrf)
		s.RefreshToken = ""
	}
	write(w, status, s)
}
func (h *Handler) clearRefreshCookie(w http.ResponseWriter, r *http.Request) {
	if name, secure, ok := h.browserCookiePolicy(r); ok {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, Secure: secure, SameSite: refreshSameSite(name, secure), MaxAge: -1, Expires: time.Unix(1, 0)})
		http.SetCookie(w, &http.Cookie{Name: name + "_csrf", Value: "", Path: "/", Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	}
}
func (h *Handler) refreshInput(w http.ResponseWriter, r *http.Request, bodyToken string) (string, string, bool) {
	if bodyToken != "" {
		if r.Header.Get("Origin") != "" {
			fail(w, 403, "session_mode_denied", "A browser must use its HttpOnly refresh cookie.", false)
			return "", "", false
		}
		for _, c := range r.Cookies() {
			if strings.HasPrefix(c.Name, "__Host-portico_refresh") || strings.HasPrefix(c.Name, "portico_refresh_dev") {
				fail(w, 400, "ambiguous_credentials", "Supply one refresh credential.", false)
				return "", "", false
			}
		}
		return bodyToken, "native", true
	}
	name, _, ok := h.browserCookiePolicy(r)
	if !ok {
		fail(w, 403, "origin_denied", "An allowed browser origin is required.", false)
		return "", "", false
	}
	var token string
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == name {
			token = c.Value
			count++
		}
	}
	if count != 1 || token == "" {
		failure(w, sessions.ErrGrant)
		return "", "", false
	}
	csrf, e := r.Cookie(name + "_csrf")
	supplied := r.Header.Get("X-CSRF-Token")
	if e != nil || len(supplied) != 43 || subtle.ConstantTimeCompare([]byte(csrf.Value), []byte(supplied)) != 1 {
		fail(w, 403, "csrf_required", "Refresh the account page before continuing.", false)
		return "", "", false
	}
	return token, "browser", true
}
func (h *Handler) refreshRoutes(mux *http.ServeMux) {
	if h.Sessions == nil {
		return
	}
	h.sessionReviewRoutes(mux)
	mux.HandleFunc("POST /v1/sessions/refresh", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowRefresh(r) {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "rate_limited", "Wait before refreshing again.", true)
			return
		}
		var q RefreshRequest
		if !decode(w, r, &q) {
			return
		}
		token, mode, ok := h.refreshInput(w, r, q.RefreshToken)
		if !ok {
			return
		}
		if !requireBrowserFamily(w, mode, q.FamilyID) {
			return
		}
		result, e := h.Sessions.RefreshExpected(r.Context(), token, q.RequestID, mode, q.FamilyID)
		if e != nil {
			failure(w, e)
			return
		}
		h.sessionResponse(w, r, 200, result)
	})
	mux.HandleFunc("POST /v1/sessions/revoke", func(w http.ResponseWriter, r *http.Request) {
		var q RefreshRevokeRequest
		if !decode(w, r, &q) {
			return
		}
		token, mode, ok := h.refreshInput(w, r, q.RefreshToken)
		if !ok {
			return
		}
		if !requireBrowserFamily(w, mode, q.FamilyID) {
			return
		}
		clear, e := h.Sessions.RevokeExpected(r.Context(), token, mode, q.FamilyID)
		if e != nil {
			failure(w, e)
			return
		}
		if clear && mode == "browser" {
			h.clearRefreshCookie(w, r)
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /v1/sessions/logout-all", func(w http.ResponseWriter, r *http.Request) {
		if !h.securityReady(w) {
			return
		}
		if e := h.Security.LogoutAll(r.Context(), bearer(r)); e != nil {
			failure(w, e)
			return
		}
		// Account-scoped revocation must not clear a newly switched account cookie.
		w.WriteHeader(204)
	})
}
func (h *Handler) allowRefresh(r *http.Request) bool {
	return h.allowAttempt("refresh:"+budgetIP(peer(r)), 120)
}

// Browser sessions use same-site cookies; third-party media origins use native grants.
func refreshSameSite(name string, secure bool) http.SameSite { return http.SameSiteLaxMode }
