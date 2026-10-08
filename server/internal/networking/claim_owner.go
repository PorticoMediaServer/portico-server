package networking

import (
	"context"
	"database/sql"
	"errors"
)

// GuardLocalOwner rechecks the current Direct Sign-In recovery-owner membership within the
// same SQLite writer transaction as claim progression. It is intentionally not
// the installed-credential guard: a password rotation invalidates pending
// consent but must not revoke the server's already installed Hosted credential.
// Root authenticates the initiating owner session before capturing LocalOwner;
// Store additionally matches operation revision, active key and reset generation.
func GuardLocalOwner(ctx context.Context, tx *sql.Tx, owner LocalOwner) error {
	if tx == nil || !validOwner(owner) {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	var profile string
	var epoch int64
	e := tx.QueryRowContext(ctx, `SELECT a.profile_id,a.epoch FROM accounts a JOIN direct_memberships m ON m.account_id=a.id WHERE a.id=? AND m.role='owner' AND m.disabled=0`, owner.AccountID).Scan(&profile, &epoch)
	if errors.Is(e, sql.ErrNoRows) {
		return ErrStale
	}
	if e != nil {
		return e
	}
	if profile != owner.ProfileID || epoch != owner.Epoch {
		return ErrStale
	}
	return ctx.Err()
}
