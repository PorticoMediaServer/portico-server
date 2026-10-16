package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
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
		resume, _ := json.Marshal(claimResume{p.Hash, p.AccountID, p.ProfileID, p.Epoch})
		return networking.ClaimRequestOwner{Owner: networking.LocalOwner{AccountID: p.AccountID, ProfileID: p.ProfileID, Epoch: int64(p.Epoch)}, Guard: func(ctx context.Context, tx *sql.Tx) error {
			_, e := id.SessionFamilyTx(ctx, tx, p)
			if errors.Is(e, identity.ErrUnauthorized) {
				return networking.ErrClaimOwnerRequired
			}
			return e
		}, Resume: string(resume)}, nil
	}
}

// claimResume notes the owner sign-in that asked for a web approval: the
// digest the session store already keeps for its current token, and who it
// belonged to.
type claimResume struct {
	Digest    string `json:"digest"`
	AccountID string `json:"accountId"`
	ProfileID string `json:"profileId"`
	Epoch     int    `json:"epoch"`
}

// NetworkingClaimResume rebuilds, after a restart, the guard of the owner who
// asked for a pending web approval: it holds while that same sign-in is still
// current and still the local owner's, exactly as the guard it replaces.
func NetworkingClaimResume(id *identity.Service) func(string) networking.ClaimRequestGuard {
	return func(note string) networking.ClaimRequestGuard {
		var r claimResume
		if id == nil || json.Unmarshal([]byte(note), &r) != nil || r.Digest == "" {
			return nil
		}
		return func(ctx context.Context, tx *sql.Tx) error {
			p, e := id.PrincipalByDigestTx(ctx, tx, r.Digest)
			if errors.Is(e, identity.ErrUnauthorized) || e == nil && (p.Authority != "local" || p.Role != "owner" || p.AccountID != r.AccountID || p.ProfileID != r.ProfileID || p.Epoch != r.Epoch) {
				return networking.ErrClaimOwnerRequired
			}
			return e
		}
	}
}
