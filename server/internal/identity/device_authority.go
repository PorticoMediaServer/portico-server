package identity

import (
	"context"
	"database/sql"
	"errors"
)

// A device record is local state about a person's own hardware: the television in the living
// room, the phone they signed out from the kitchen. Nothing about it belongs to an account
// service, and nothing about it needs one — which is why a Portico Account viewer gets exactly
// the same record, kept here, with no call to Hosted Services at all.
//
// Until this existed the whole feature was local-accounts-only. Every device route resolved
// its caller through directCallerTx, which refuses any principal whose authority is not
// "local" (direct_accounts.go:241), and identity_devices.account_id had a foreign key into
// accounts, a table a Portico Account holder has no row in. So the owner — who signs in with a
// Portico Account, which is the common case, not an edge — saw an empty Devices screen, could
// not sign one device out, and got an empty Top Shelf on an Apple TV, because the feed token
// is issued against a bound device.

// deviceCaller is who is asking, in the one shape both authorities share.
type deviceCaller struct {
	authority string
	account   string
	role      string
	// manage is the right to approve, sign out or forget a device rather than merely
	// register and list. For a local account that is an account session or the account's
	// primary profile; for a Portico Account it is the policy's own role, which is the same
	// distinction the account service already draws.
	manage bool
}

// deviceCallerTx resolves a bearer for the device routes. The local path is unchanged, down to
// which errors it returns; a hosted viewing session is accepted the same way any other
// authority-neutral read accepts it, from its own live family.
func (s *Service) deviceCallerTx(ctx context.Context, tx *sql.Tx, bearer string, manage bool) (deviceCaller, error) {
	c, e := s.directCallerTx(ctx, tx, bearer, manage)
	if e == nil {
		return deviceCaller{authority: "local", account: c.account.ID, role: c.account.Role, manage: true}, nil
	}
	if !errors.Is(e, ErrUnauthorized) {
		return deviceCaller{}, e
	}
	record, hostedErr := s.familyTokenTx(ctx, tx, Digest(bearer))
	if hostedErr != nil || record.principal.Authority != "hosted" {
		// Not a hosted session either: the local refusal is the honest answer, and a
		// hosted-shaped failure must not turn a local "manage required" into something else.
		return deviceCaller{}, e
	}
	// The same liveness check every hosted request makes: a revoked family, a bumped epoch or
	// a lapsed policy horizon is refused here rather than at the device record.
	if _, err := s.SessionFamilyTx(ctx, tx, record.principal); err != nil {
		return deviceCaller{}, err
	}
	out := deviceCaller{authority: "hosted", account: record.principal.AccountID, role: record.principal.Role, manage: record.principal.Role == "owner"}
	if manage && !out.manage {
		return deviceCaller{}, ErrForbidden
	}
	return out, ctx.Err()
}

// local says whether a caller owns the local-account-only side state — the remembered account
// list a browser shows before sign-in, and remembered profile trust. Neither has a hosted
// equivalent, because a Portico Account is remembered by the account service.
func (c deviceCaller) local() bool { return c.authority == "local" }
