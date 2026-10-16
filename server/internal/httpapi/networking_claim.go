package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/networking"
	"strings"
)

// NetworkingClaimAuthorizer captures the actual current local owner token.
// Claiming stays with the server's own web app and the built-in Portico pages;
// native bearer requests may omit Origin. No cookies grant local ownership.
func NetworkingClaimAuthorizer(id *identity.Service, origins []string, policies ...Dependencies) networking.ClaimRequestAuthorizer {
	d := Dependencies{}
	if len(policies) > 0 {
		d = policies[0]
	}
	d.Origins = origins
	return func(r *http.Request) (networking.ClaimRequestOwner, error) {
		if id == nil || len(r.Header.Values("Authorization")) != 1 {
			return networking.ClaimRequestOwner{}, identity.ErrUnauthorized
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if !d.firstPartyOrigin(r, origin) {
				return networking.ClaimRequestOwner{}, identity.ErrUnauthorized
			}
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") || strings.ContainsAny(strings.TrimPrefix(header, "Bearer "), " ,\r\n") {
			return networking.ClaimRequestOwner{}, identity.ErrUnauthorized
		}
		p, e := id.Authenticate(strings.TrimPrefix(header, "Bearer "))
		if e == nil {
			e = id.CheckRecoveryRoute(r.Context(), p, d.privateSetupPeer(r))
		}
		if e != nil || p.Authority != "local" || p.Role != "owner" {
			return networking.ClaimRequestOwner{}, identity.ErrUnauthorized
		}
		return networking.ClaimRequestOwner{Owner: networking.LocalOwner{AccountID: p.AccountID, ProfileID: p.ProfileID, Epoch: int64(p.Epoch)}, Guard: func(ctx context.Context, tx *sql.Tx) error {
			_, e := id.SessionFamilyTx(ctx, tx, p)
			if errors.Is(e, identity.ErrUnauthorized) {
				return networking.ErrClaimOwnerRequired
			}
			return e
		}}, nil
	}
}
