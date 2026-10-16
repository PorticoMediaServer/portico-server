package httpapi

import (
	"net/http"
	"slices"
	"strings"
)

// browserOriginAllowed decides whether a browser page on origin may make this
// request. The server never authenticates by cookie, so the only thing an
// origin rule protects is what a request without a token can do from a page
// a person happens to have open (the home network address it comes from):
//
//   - the server's own web app, at whatever address the browser used (an IP,
//     a hostname, a .local name, IPv6, or behind a reverse proxy), and the
//     built-in Portico web and Cast pages may make any request;
//   - any other origin may call the API with a token: an API key, a personal
//     access token or a session token in Authorization, or a media grant in
//     the path. Preflights are answered for every origin, since they carry no
//     credential and the real request is checked again here.
//
// Nothing is ever allowed with credentials (no Access-Control-Allow-Credentials),
// and a cookie sent with another origin's request is dropped before anything
// reads it. First-run setup keeps its own stricter same-origin rule.
func (d Dependencies) browserOriginAllowed(r *http.Request, origin string) bool {
	if d.firstPartyOrigin(r, origin) {
		return true
	}
	r.Header.Del("Cookie")
	if r.Method == http.MethodOptions {
		return true
	}
	return tokenCredentialPresented(r)
}

// tokenCredentialPresented reports whether the request carries a bearer
// credential of its own, without deciding whether it is valid (the route's
// authentication does that).
func tokenCredentialPresented(r *http.Request) bool {
	// A key or token in the address of a GET or HEAD is already here: the
	// router moves it into Authorization (liftQueryCredential) before this runs.
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		return true
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		// Media and download bytes are addressed by a grant minted for one
		// session; the grant is the credential.
		return strings.HasPrefix(r.URL.Path, "/v1/media/") || strings.HasPrefix(r.URL.Path, "/v1/downloads/artifacts/")
	}
	return false
}

// firstPartyOrigin is the server's own web app at whatever address the browser
// used, or one of the built-in Portico pages.
func (d Dependencies) firstPartyOrigin(r *http.Request, origin string) bool {
	if d.sameRequestOrigin(r, origin) || slices.Contains(d.Origins, origin) {
		return true
	}
	// A browser sends Sec-Fetch-Site itself and a page cannot set it, so
	// "same-origin" is the browser's own word that this page was served from the
	// address it is calling: the web app behind a proxy that rewrites Host, with
	// no forwarding trust configured. A client that is not a browser can write
	// the header, but it can equally write a matching Origin; it gains nothing.
	return r.Header.Get("Sec-Fetch-Site") == "same-origin"
}
