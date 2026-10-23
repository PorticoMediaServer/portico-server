//go:build devtrust && !release

package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
)

// GET /dev/e2e/session?account=<username>&origin=<web origin>[&return=<url>]
// signs a throwaway test account in to a local development Hosted in a
// browser, without a password, the way a real browser sign-in does: it sets
// the HttpOnly refresh cookie (and its CSRF cookie) for that web origin and
// sends the browser back, so the web app then runs the real refresh path. It
// exists only in devtrust builds, and only answers for a database whose name
// contains "e2e" or "test", a loopback HTTP web origin on the allow list, and
// a Hosted started with -allow-loopback-enrollment.
func init() {
	devQueryPaths["/dev/e2e/session"] = true
	devRoutes = append(devRoutes, func(h *Handler, mux *http.ServeMux) {
		mux.HandleFunc("GET /dev/e2e/session", h.devE2ESession)
	})
}

func (h *Handler) devE2ESession(w http.ResponseWriter, r *http.Request) {
	if h.Security == nil || !h.DevelopmentCookies {
		fail(w, 404, "not_found", "Not found.", false)
		return
	}
	var database string
	if e := h.Security.Store.DB.QueryRowContext(r.Context(), `SELECT current_database()`).Scan(&database); e != nil || !(strings.Contains(database, "e2e") || strings.Contains(database, "test")) {
		fail(w, 404, "not_found", "Not found.", false)
		return
	}
	q := r.URL.Query()
	origin := q.Get("origin")
	u, e := url.Parse(origin)
	if e != nil || !h.Origins[origin] || u.Scheme != "http" {
		fail(w, 400, "invalid_request", "origin must be an allowed loopback http web origin.", false)
		return
	}
	if ip, e := netip.ParseAddr(u.Hostname()); u.Hostname() != "localhost" && (e != nil || !ip.IsLoopback()) {
		fail(w, 400, "invalid_request", "origin must be an allowed loopback http web origin.", false)
		return
	}
	back := q.Get("return")
	if back == "" {
		back = origin + "/"
	}
	if !strings.HasPrefix(back, origin+"/") {
		fail(w, 400, "invalid_request", "return must be on the web origin.", false)
		return
	}
	var account, username, displayName string
	var password []byte
	if e = h.Security.Store.DB.QueryRowContext(r.Context(), `SELECT id,username,display_name,password_hash FROM accounts WHERE username=$1`, strings.ToLower(q.Get("account"))).Scan(&account, &username, &displayName, &password); e != nil {
		fail(w, 404, "not_found", "No such test account.", false)
		return
	}
	family, refresh := identity.NewFamily("browser")
	token := security.RandomID("hat_")
	if e = h.Security.Store.CreateSession(r.Context(), account, security.Hash(token), time.Now().UTC().Add(time.Hour), password, family); e != nil {
		failure(w, e)
		return
	}
	name := h.browserCookieName(origin, false)
	expiry := family.RefreshExpiresAt
	http.SetCookie(w, &http.Cookie{Name: name, Value: refresh, Path: "/", HttpOnly: true, SameSite: refreshSameSite(name, false), Expires: expiry, MaxAge: max(1, int(time.Until(expiry).Seconds()))})
	http.SetCookie(w, &http.Cookie{Name: name + "_csrf", Value: security.RandomID(""), Path: "/", SameSite: http.SameSiteLaxMode, MaxAge: max(1, int(time.Until(expiry).Seconds()))})
	// The web keeps its sign-in metadata (never a credential) in localStorage
	// under portico.account.v1:<Hosted origin>, which this origin cannot write.
	// The fragment carries that record; the web's dev build (or the test
	// driver) stores it and reloads, and the first refresh uses the cookie.
	stamp := func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }
	record, e := json.Marshal(map[string]any{"version": 1, "revocations": []any{}, "active": map[string]any{
		"session": map[string]any{"accessToken": "", "expiresAt": stamp(time.Now()), "refreshExpiresAt": stamp(expiry), "familyId": family.ID,
			"account": map[string]string{"id": account, "username": username, "displayName": displayName}},
		"authority": map[string]string{"familyId": family.ID}, "context": map[string]any{}}})
	if e != nil {
		failure(w, e)
		return
	}
	hosted := "http://" + r.Host
	fragment := url.Values{"portico-e2e-key": {"portico.account.v1:" + hosted}, "portico-e2e-record": {base64.RawURLEncoding.EncodeToString(record)}}
	http.Redirect(w, r, strings.SplitN(back, "#", 2)[0]+"#"+fragment.Encode(), http.StatusSeeOther)
}
