package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"portico.local/server/internal/access"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
)

// accessRoutes registers the people and access surface.
//
// Tier: every /v1/admin/access route needs at least the admin tier, except the
// API key routes, which are owner-only because a full-scope key is a second
// credential for the whole server and belongs with the other security controls.
// Invitation acceptance is unauthenticated: the invitee has no account yet, so
// the invitation code is the whole authority and the route is rate-limited like
// the sign-in routes.
func (d Dependencies) accessRoutes(mux *http.ServeMux) {
	store := d.Access.Access
	if store == nil || d.Identity == nil {
		return
	}
	mux.HandleFunc("GET /v1/admin/access/members", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.administrator(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		out, e := store.Members(r.Context(), d.accessAuthority(p, identity.TierAdmin), access.MemberQuery{Cursor: r.URL.Query().Get("cursor"), Limit: r.URL.Query().Get("limit")})
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("PUT /v1/admin/access/members/{id}/role", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.administrator(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		var body access.RoleChange
		if e = decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := store.SetRole(r.Context(), d.accessAuthority(p, identity.TierAdmin), p, r.PathValue("id"), body)
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/admin/access/members/{id}/limits", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.administrator(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		out, e := store.Limits(r.Context(), d.accessAuthority(p, identity.TierAdmin), r.PathValue("id"))
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("PUT /v1/admin/access/members/{id}/limits", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.administrator(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		var body access.LimitsChange
		if e = decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := store.SetLimits(r.Context(), d.accessAuthority(p, identity.TierAdmin), r.PathValue("id"), body)
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/admin/access/invitations", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.administrator(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		query := r.URL.Query()
		out, e := store.Invitations(r.Context(), d.accessAuthority(p, identity.TierAdmin), access.InvitationQuery{Cursor: query.Get("cursor"), Limit: query.Get("limit"), State: query.Get("state")})
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/admin/access/invitations", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.administrator(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		var body access.InvitationRequest
		if e = decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := store.Invite(r.Context(), d.accessAuthority(p, identity.TierAdmin), p, body)
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 201, out)
	})
	mux.HandleFunc("POST /v1/admin/access/invitations/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.administrator(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		var body access.InvitationRevoke
		if e = decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := store.RevokeInvitation(r.Context(), d.accessAuthority(p, identity.TierAdmin), r.PathValue("id"), body)
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/access/invitations/accept", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			access.Acceptance
			// PorticoIdentity accepts as a Portico Account instead of creating
			// a password account; the answer is then a sign-in (DirectSignIn).
			PorticoIdentity *hosted.Signed `json:"porticoIdentity,omitempty"`
		}
		if e := decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		// The same limiter the sign-in routes use: a code is a secret, and an
		// unauthenticated route that creates accounts must not be brute-forced.
		if e := d.Identity.SetupLimit(r.Context(), setupPeerSubject(r), "invitation", 10); e != nil {
			setupFailure(w, e)
			return
		}
		if body.PorticoIdentity != nil {
			if d.Hosted == nil || body.Password != "" {
				failure(w, identity.ErrUnauthorized)
				return
			}
			who, e := d.Hosted.VerifyIdentity(r.Context(), *body.PorticoIdentity, r.Header.Get("X-Portico-Installation-Id"))
			if e != nil {
				failure(w, e)
				return
			}
			out, e := store.AcceptPortico(r.Context(), access.PorticoAcceptance{Code: body.Code, AccountID: who.AccountID, Username: who.Username, DisplayName: who.DisplayName, Consent: who.Consent}, func(ctx context.Context, tx *sql.Tx) (identity.DirectSignIn, error) {
				return d.Identity.PorticoSignInTx(ctx, tx, who)
			})
			if e != nil {
				if errors.Is(e, access.ErrExpired) || errors.Is(e, access.ErrInvalid) {
					accessFailure(w, e)
				} else {
					failure(w, e)
				}
				return
			}
			writeAuth(w, r, 201, out)
			return
		}
		hash, e := access.HashPassword(body.Password)
		if e != nil {
			accessFailure(w, e)
			return
		}
		out, e := store.Accept(r.Context(), body.Acceptance, hash)
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 201, out)
	})
	mux.HandleFunc("GET /v1/admin/access/devices", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.administrator(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		document, e := d.settingsDocument(r.Context())
		if e != nil {
			accessFailure(w, e)
			return
		}
		query := r.URL.Query()
		out, e := store.Devices(r.Context(), d.accessAuthority(p, identity.TierAdmin), access.DeviceQuery{Cursor: query.Get("cursor"), Limit: query.Get("limit"), Trust: query.Get("trust"), Account: query.Get("account")}, document.Effective.DeviceApprovalRequired)
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("PUT /v1/admin/access/devices/{id}/trust", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.administrator(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		var body access.TrustChange
		if e = decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := store.SetDeviceTrust(r.Context(), d.accessAuthority(p, identity.TierAdmin), r.PathValue("id"), body)
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/admin/access/api-keys", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		query := r.URL.Query()
		out, e := store.APIKeys(r.Context(), d.accessAuthority(p, identity.TierOwner), access.APIKeyQuery{Cursor: query.Get("cursor"), Limit: query.Get("limit")})
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/admin/access/api-keys", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.owner(r); e != nil {
			accessFailure(w, e)
			return
		}
		write(w, http.StatusForbidden, map[string]any{"error": map[string]any{"code": "api_keys_disabled", "message": "API key creation is disabled.", "retryable": false}})
	})
	mux.HandleFunc("POST /v1/admin/access/api-keys/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		var body access.APIKeyRevoke
		if e = decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := store.RevokeAPIKey(r.Context(), d.accessAuthority(p, identity.TierOwner), r.PathValue("id"), body)
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, out)
	})
}
