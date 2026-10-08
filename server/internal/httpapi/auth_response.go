package httpapi

import "net/http"

// HTTP and HTTPS have identical first-party credential semantics. A claimed
// server's owner can require HTTPS through the server-wide transport policy;
// when they do, that policy refuses the request before it reaches this writer.
func writeAuth(w http.ResponseWriter, _ *http.Request, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	write(w, status, value)
}
