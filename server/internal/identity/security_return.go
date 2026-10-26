package identity

import (
	"context"
	"database/sql"
	"encoding/json"
)

// SecurityReturnActorTx is a read-only current-session proof for the distinct
// provider Security purpose. It grants no sign-in or account management itself.
// The caller signs/stores the result in this same accepting transaction.
func (s *Service) SecurityReturnActorTx(ctx context.Context, tx *sql.Tx, bearer string) (account, binding string, err error) {
	if tx == nil || CredentialShaped(bearer) {
		return "", "", ErrUnauthorized
	}
	if err = RequireSignInKindTx(ctx, tx, SignInPortico); err != nil {
		return
	}
	c, err := s.directCallerTx(ctx, tx, bearer, true)
	if err != nil {
		return "", "", err
	}
	record, err := s.familyTokenTx(ctx, tx, Digest(bearer))
	if err != nil {
		return "", "", err
	}
	if record.TokenGeneration != record.currentGeneration || record.retired {
		return "", "", ErrUnauthorized
	}
	if err = s.RequireMainProfileTx(ctx, tx, record.principal); err != nil {
		return "", "", err
	}
	if err = tx.QueryRowContext(ctx, `SELECT hosted_account_id FROM account_portico_links WHERE account_id=?`, c.account.ID).Scan(&account); err != nil {
		return "", "", ErrUnauthorized
	}
	if !validHostedAccountID(account) {
		return "", "", ErrUnauthorized
	}
	raw, _ := json.Marshal([]any{c.account.ID, c.profileID, c.familyID, c.epoch, c.account.Revision, c.account.Role, c.deviceID, record.TokenGeneration, account})
	return account, s.setupHash("security-return-actor", string(raw)), nil
}
