package httpapi

import (
	"net/http"
	"portico.local/apikit"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
)

type porticoReturnRequest struct {
	hosted.RegisteredReturnExchange
	RequestID        string `json:"requestId,omitempty"`
	SetupToken       string `json:"setupToken,omitempty"`
	ExpectedRevision int64  `json:"expectedRevision,string,omitempty"`
	MoveTicket       string `json:"moveTicket,omitempty"`
	MoveCode         string `json:"moveCode,omitempty"`
}

func (d Dependencies) serverReturnRoutes(mux *http.ServeMux) {
	d.httpHandle(mux, "POST /v1/auth/portico-return/exchange", func(w http.ResponseWriter, r *http.Request) {
		var q porticoReturnRequest
		if e := decode(w, r, &q); e != nil {
			failure(w, e)
			return
		}
		if (q.MoveTicket != "" && q.MoveCode != "") || (q.Purpose != "sign-in" && (q.MoveTicket != "" || q.MoveCode != "")) {
			failure(w, identity.ErrDirectInput)
			return
		}
		if q.Purpose == "setup" && !d.setupRequestAllowed(r) {
			failure(w, identity.ErrUnauthorized)
			return
		}
		if d.Hosted == nil || q.InstallationID != r.Header.Get("X-Portico-Installation-Id") {
			failure(w, identity.ErrUnauthorized)
			return
		}
		if e := d.Identity.SetupLimit(r.Context(), d.setupPeerSubject(r), "portico-return", 30); e != nil {
			setupFailure(w, e)
			return
		}
		who, e := d.Hosted.ExchangeRegisteredReturn(r.Context(), q.RegisteredReturnExchange)
		if e != nil {
			failure(w, e)
			return
		}
		var out identity.DirectSignIn
		if who.Return.Purpose == "sign-in" {
			if q.MoveTicket != "" || q.MoveCode != "" {
				out, e = d.Identity.RedeemPorticoMove(r.Context(), identity.PorticoMoveProof{Ticket: q.MoveTicket, Code: q.MoveCode, Who: who})
			} else {
				out, e = d.Identity.PorticoSignIn(r.Context(), who)
			}
		} else if who.Return.Purpose == "setup" {
			out, e = d.Identity.CompleteAccountSetup(r.Context(), q.RequestID, q.SetupToken, who)
		} else {
			out, e = d.Identity.CompletePorticoOwnerClaim(r.Context(), directBearer(r), q.ExpectedRevision, who)
		}
		if e != nil {
			failure(w, e)
			return
		}
		if d.Hosted != nil {
			d.Hosted.MembershipChanged()
		}
		writeAuth(w, r, 200, out)
	})
}
func registerServerReturnHTTPContracts(registry *apikit.Registry) error {
	return apikit.RegisterHTTP[porticoReturnRequest, identity.DirectSignIn](registry, apikit.Metadata{ID: "post_auth_portico_return_exchange", Method: "POST", Path: "/v1/auth/portico-return/exchange", Summary: "Complete a registered Portico Account return", Access: apikit.Public, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 16384, Status: 200, Headers: []string{"X-Portico-Installation-Id"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "rate_limited", "sign_in_kind_portico", "sign_in_kind_local", "access_refused", "account_move_waiting", "move_collision", "move_code_invalid", "internal"}})
}
