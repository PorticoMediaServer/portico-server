package recordingaccess

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/livechannels"
)

// Durable checks the account/profile that owns a recording, not the expired
// interactive token used to create it. It reads only current local authority.
func (p Policy) Durable(ctx context.Context, tx *sql.Tx, owner livechannels.Owner, source, channel string) error {
	if tx == nil || !owner.Valid() {
		return livechannels.ErrDenied
	}
	if err := GrantedTx(ctx, tx, owner); err != nil {
		return err
	}
	localOwner, err := p.MemberTx(ctx, tx, owner)
	if err != nil {
		return err
	}
	var access string
	e := tx.QueryRowContext(ctx, `SELECT COALESCE(x.viewer_access,'owner-only') FROM live_source_identities s LEFT JOIN live_source_settings x ON x.source_id=s.id WHERE s.id=?`, source).Scan(&access)
	if errors.Is(e, sql.ErrNoRows) {
		return livechannels.ErrDenied
	}
	if e != nil {
		return e
	}
	if !localOwner && access != "server-members" {
		return livechannels.ErrDenied
	}
	return ctx.Err()
}

// MemberTx verifies current account/profile membership without granting recording permission.
func (p Policy) MemberTx(ctx context.Context, tx *sql.Tx, owner livechannels.Owner) (bool, error) {
	if tx == nil || !owner.Valid() {
		return false, livechannels.ErrDenied
	}
	who := identity.Principal{Viewer: identity.Viewer{Authority: owner.Authority, AccountID: owner.AccountID, ProfileID: owner.ProfileID}}
	localOwner := false
	if owner.Authority == "local" {
		var e error
		localOwner, e = localAccess(ctx, tx, p.Schema, who, "")
		if errors.Is(e, identity.ErrUnauthorized) {
			return false, livechannels.ErrDenied
		}
		if e != nil {
			return false, e
		}
		who.Role = "member"
		if localOwner {
			who.Role = "owner"
		}
	}
	if p.Cached != nil {
		if e := p.Cached.AllowedTxContext(ctx, who, "", tx); e != nil {
			if errors.Is(e, identity.ErrUnauthorized) {
				return false, livechannels.ErrDenied
			}
			return false, e
		}
	} else if owner.Authority != "local" {
		return false, livechannels.ErrDenied
	}
	return localOwner, nil
}
