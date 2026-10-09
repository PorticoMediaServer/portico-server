package identity

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ReauthorizeTx rechecks an authenticated principal inside the transaction that
// will mutate its resources. It never acquires another database connection.
// Hosted callers must additionally check cached membership/library policy in tx.
// Token-family renewal and rotation are separate, not implied by this check.
func (s *Service) ReauthorizeTx(ctx context.Context, tx *sql.Tx, expected Principal) (Principal, error) {
	if tx == nil || expected.Hash == "" {
		return Principal{}, ErrUnauthorized
	}
	if err := ctx.Err(); err != nil {
		return Principal{}, err
	}
	// Preserve already verified accounting metadata. No authority guard uses
	// DeviceID; identity is still checked against the live token and account.
	current := Principal{Hash: expected.Hash, DeviceID: expected.DeviceID}
	current.ServerID = s.serverID
	var expiry string
	var revoked int
	err := tx.QueryRowContext(ctx, `SELECT account_id,profile_id,authority,role,epoch,expires_at,revoked FROM authorization_access WHERE hash=?`, expected.Hash).
		Scan(&current.AccountID, &current.ProfileID, &current.Authority, &current.Role, &current.Epoch, &expiry, &revoked)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, err
	}
	expires, err := time.Parse(time.RFC3339, expiry)
	if err != nil || revoked != 0 || !expires.After(time.Now()) || current.Viewer != expected.Viewer || current.Epoch != expected.Epoch {
		return Principal{}, ErrUnauthorized
	}
	switch current.Authority {
	case "local":
		if err = s.checkDirectPrincipalTx(tx, current); err != nil {
			return Principal{}, err
		}
	case "hosted":
		// Local hosted accounts do not have password-account rows. Membership and
		// restrictions are checked by the domain's authority callback in this tx.
	default:
		return Principal{}, ErrUnauthorized
	}
	if err := ctx.Err(); err != nil {
		return Principal{}, err
	}
	return current, nil
}
