package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/serverreturn"
)

// Ordinary provider proofs are unchanged. A proof created by a server Settings
// continuation also retains that original intent's cancellation and expiry,
// current installation and registered-route guards in the accepting command.
func (s *AccountSecurity) securityReturnActionTx(ctx context.Context, tx *sql.Tx, a identity.CurrentFamilyExpectation, i identity.SecurityIntent) error {
	var id, account, family string
	var access, digest, cipher []byte
	var epoch, generation, claim, credential, route int64
	var consumed, cancelled sql.NullTime
	var expiry time.Time
	e := tx.QueryRowContext(ctx, `SELECT r.transaction_id,t.expected_account_id,t.expected_family_id,t.expected_account_epoch,t.expected_family_generation,t.expected_access_hash,t.intent_digest,r.context_cipher,r.claim_generation,r.credential_generation,r.route_generation,r.consumed_at,r.cancelled_at,LEAST(r.expires_at,r.confirmation_expires_at) FROM identity_security_returns r JOIN identity_transactions t ON t.id=r.transaction_id WHERE t.expected_account_id=$1 AND r.security_operation_id=$2 FOR UPDATE OF r`, a.AccountID, i.OperationID).Scan(&id, &account, &family, &epoch, &generation, &access, &digest, &cipher, &claim, &credential, &route, &consumed, &cancelled, &expiry)
	if errors.Is(e, sql.ErrNoRows) {
		return identity.ErrRecentAuth
	}
	if e != nil {
		return e
	}
	if account != a.AccountID || family != a.FamilyID || epoch != a.AuthEpoch || generation != a.FamilyGeneration || !equalSecret(access, a.TokenHash) || !equalSecret(digest, intentDigest(i)) || !consumed.Valid || cancelled.Valid || !expiry.After(time.Now()) {
		return identity.ErrRecentAuth
	}
	plain, e := s.Keys.Open(cipher, "security-return-context:"+id)
	if e != nil {
		return e
	}
	var c serverreturn.SecurityContext
	if json.Unmarshal([]byte(plain), &c) != nil || c.OperationID != i.OperationID || c.AccountID != a.AccountID {
		return identity.ErrRecentAuth
	}
	actual, e := returnTupleTx(ctx, tx, a.AccountID, c.ServerID, c.ClaimOperationID, "sign-in", c.RegistrationID)
	if e != nil || actual.claim != claim || actual.credential != credential || actual.route != route || actual.origin != c.Origin {
		return identity.ErrRecentAuth
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return e
	}
	if !securityReturnDeadline(c, expiry).After(now) {
		return identity.ErrRecentAuth
	}
	return nil
}
