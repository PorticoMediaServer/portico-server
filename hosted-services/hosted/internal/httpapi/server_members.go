package httpapi

import (
	"errors"
	"net/http"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
)

// memberIndexRoutes are Hosted's whole part in membership now (Spec — Hosted
// at Scale): a server reports its Portico Account members for discovery, a
// device checks from memory whether its server list moved, and a device asks
// for a short identity assertion to present to a server, which decides
// membership itself.
func (h *Handler) memberIndexRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/servers/{id}/members", h.server(func(w http.ResponseWriter, r *http.Request, authority directory.ServerAuthority) {
		var q directory.MemberPush
		if !decodeLimit(w, r, &q, MemberPushBody) {
			return
		}
		result, e := h.Directory.PushMembers(r.Context(), authority, q)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, result)
	}))
	mux.HandleFunc("POST /v1/servers/{id}/identity", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) {
		var q IdentityRequest
		if !decode(w, r, &q) {
			return
		}
		signed, e := h.Directory.IssueIdentity(v.Account.ID, v.Account.Username, v.Account.DisplayName, r.PathValue("id"), q.Challenge, q.Purpose)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, signed)
	}))
	// An account removes a server from its own list: the server is told at
	// its next check-in (it is woken for it) and ends the membership, and it
	// can list the account again only with a newer consent (INT M6).
	mux.HandleFunc("POST /v1/servers/{id}/leave", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) {
		if e := h.Directory.LeaveServer(r.Context(), v.Account.ID, r.PathValue("id")); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, CodeResponse{Code: "server_left"})
	}))
	// The version check needs no account session and reads no database: the
	// watch token is a MAC over the account id, and the version is in memory.
	// It is most of Hosted's traffic at scale, so it must stay this cheap. The
	// held version is in the path rather than If-None-Match so a browser sends
	// it as a simple request, without a CORS preflight.
	mux.HandleFunc("GET /v1/server-list/{watch}/{version}", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowAttempt("serverlist:"+budgetIP(peer(r)), 120) {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "rate_limited", "Try again shortly.", true)
			return
		}
		version, same, e := h.Directory.CheckServerList(r.PathValue("watch"), r.PathValue("version"))
		if errors.Is(e, directory.ErrUnavailable) {
			w.Header().Set("Retry-After", "30")
			fail(w, 503, "unavailable", "Try again shortly.", true)
			return
		}
		if e != nil {
			fail(w, 404, "not_found", "Not found.", false)
			return
		}
		w.Header().Set("ETag", `"`+version+`"`)
		if same {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		write(w, 200, ServerListVersion{Version: version})
	})
}

// IdentityRequest carries the challenge the server gave the asking
// installation; the assertion is good only for that server and installation.
type IdentityRequest struct {
	Challenge string `json:"challenge"`
	// Purpose "custody" asks for a custody assertion: the account accepts
	// custody of the server's claim (INT M6). Empty for a sign-in.
	Purpose string `json:"purpose,omitempty"`
}

// MemberPushBody bounds a full member list (10,000 members at ~100 bytes).
const MemberPushBody = 1 << 20

// ServerListVersion answers a version check whose version moved.
type ServerListVersion struct {
	Version string `json:"version"`
}
