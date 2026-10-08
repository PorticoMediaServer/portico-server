package httpapi

import (
	"errors"
	"net/http"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/directroute"
	"strconv"
	"strings"
)

// RegisterDirectRoutes is called only when the protected issuer is configured.
// The response is a name allocation, never endpoint readiness or a certificate.
func (h *Handler) RegisterDirectRoutes(mux *http.ServeMux, issuer *directroute.Service) {
	if issuer == nil {
		return
	}
	// The DNS authority's address feed. Only the DNS nodes read it (two or three callers in
	// total), and they poll every 30–60 s with conditional requests: the node's sequence as
	// If-None-Match, 304 when the feed has not moved. An unchanged feed is answered from an
	// in-memory head cache with no database read. Resuming from a sequence makes a restarted
	// node, a missed change and a new node the same case.
	mux.HandleFunc("GET /v1/dns/addresses", func(w http.ResponseWriter, r *http.Request) {
		presented, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !issuer.FeedAuthorized(presented) {
			fail(w, 401, "unauthorized", "DNS feed credentials are required.", false)
			return
		}
		after, e := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		if e != nil {
			fail(w, 400, "invalid_cursor", "Use the last applied sequence.", false)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items, etag, notModified, e := issuer.ChangesWithCache(r.Context(), after, limit, r.Header.Get("If-None-Match"))
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("ETag", etag)
		if notModified {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, http.StatusOK, AddressChangeList{Items: items})
	})
	mux.HandleFunc("POST /v1/servers/{id}/direct-route", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		var body DirectRouteRequest
		if !decode(w, r, &body) {
			return
		}
		if body.Port == 0 {
			body.Port = 32500
		}
		route, e := issuer.IssuePortFrom(r.Context(), a, body.Address, body.Port, peer(r))
		if errors.Is(e, directroute.ErrUnverified) {
			fail(w, 409, "address_unverified", "Verify the address before publishing it.", false)
			return
		}
		if errors.Is(e, directroute.ErrInvalid) {
			fail(w, 400, "invalid_address", "Use a canonical public IP address.", false)
			return
		}
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, http.StatusOK, route)
	}))
	// A server that has affirmatively stopped seeing an address family says so once. Without
	// it the DNS nodes keep answering a family the server no longer has, and dual-stack
	// clients try the dead one first. It is a change report, not a heartbeat: a server with
	// both families never calls it.
	mux.HandleFunc("POST /v1/servers/{id}/direct-route/withdraw", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		var body DirectRouteWithdrawRequest
		if !decode(w, r, &body) {
			return
		}
		if !directroute.ValidFamily(body.Family) {
			fail(w, 400, "invalid_family", "Withdraw ipv4 or ipv6.", false)
			return
		}
		namespace, changed, e := issuer.Withdraw(r.Context(), a, body.Family)
		if errors.Is(e, directroute.ErrInvalid) {
			fail(w, 400, "invalid_family", "Withdraw ipv4 or ipv6.", false)
			return
		}
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, http.StatusOK, DirectRouteWithdrawal{Family: body.Family, Namespace: namespace, Withdrawn: changed})
	}))
}
