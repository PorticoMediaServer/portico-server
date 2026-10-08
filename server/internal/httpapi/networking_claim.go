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
// Browser origins use the same explicit list as the outer API middleware;
// native bearer requests may omit Origin. No cookies grant local ownership.
func NetworkingClaimAuthorizer(id *identity.Service, origins []string) networking.ClaimRequestAuthorizer {
	return func(r *http.Request) (networking.ClaimRequestOwner, error) {
		if id == nil || len(r.Header.Values("Authorization")) != 1 {
			return networking.ClaimRequestOwner{}, identity.ErrUnauthorized
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			allowed := false
			for _, want := range origins {
				if origin == want {
					allowed = true
					break
				}
			}
			if !allowed {
				return networking.ClaimRequestOwner{}, identity.ErrUnauthorized
			}
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") || strings.ContainsAny(strings.TrimPrefix(header, "Bearer "), " ,\r\n") {
			return networking.ClaimRequestOwner{}, identity.ErrUnauthorized
		}
		p, e := id.Authenticate(strings.TrimPrefix(header, "Bearer "))
		if e == nil {
			e = id.CheckRecoveryRoute(r.Context(), p, privateSetupPeer(r))
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
