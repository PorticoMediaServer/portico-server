package hosted

import (
	"context"
	"database/sql"
	"time"

	"portico.local/server/internal/identity"
)

// AuthorizationHorizonTx checks current cached membership and restrictions in
// the caller's authority transaction. It does not renew policy over the network.
// A role change requires a new attachment instead of preserving an old claim.
func (s *Service) AuthorizationHorizonTx(ctx context.Context, tx *sql.Tx, principal identity.Principal) (time.Time, error) {
	if tx == nil || principal.Authority != "hosted" || principal.ServerID != s.identity.ID() {
		return time.Time{}, identity.ErrUnauthorized
	}
	if err := s.AllowedTxContext(ctx, principal, "", tx); err != nil {
		return time.Time{}, err
	}
	policy, err := s.loadFrom(transactionPolicyReader{ctx, tx})
	if err != nil {
		return time.Time{}, err
	}
	horizon, err := time.Parse(time.RFC3339, policy.ExpiresAt)
	if err != nil || !horizon.After(time.Now()) {
		return time.Time{}, identity.ErrUnauthorized
	}
	for _, member := range policy.Members {
		if member.AccountID == principal.AccountID && member.ProfileID == principal.ProfileID && member.Role == principal.Role {
			return horizon, nil
		}
	}
	return time.Time{}, identity.ErrUnauthorized
}
