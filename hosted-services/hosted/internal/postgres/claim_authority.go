package postgres

import (
	"context"
	"portico.local/hosted/internal/identity"
)

// CaptureClaimAuthority returns a candidate tuple from the presented access
// hash. It is not authorization; ApproveClaim rechecks it with the shared strict
// current-family helper inside the mutation transaction.
func (s *Store) CaptureClaimAuthority(ctx context.Context, hash []byte) (identity.CurrentFamilyExpectation, error) {
	var out identity.CurrentFamilyExpectation
	if len(hash) != 32 {
		return out, identity.ErrCredentials
	}
	e := s.DB.QueryRowContext(ctx, `SELECT a.id,f.id,a.auth_epoch,f.generation,f.mode FROM account_sessions s JOIN accounts a ON a.id=s.account_id JOIN session_families f ON f.id=s.family_id AND f.account_id=a.id WHERE s.token_hash=$1`, hash).Scan(&out.AccountID, &out.FamilyID, &out.AuthEpoch, &out.FamilyGeneration, &out.Mode)
	if e != nil {
		return identity.CurrentFamilyExpectation{}, currentFamilyError(ctx, e)
	}
	out.TokenHash = append([]byte(nil), hash...)
	return out, nil
}
