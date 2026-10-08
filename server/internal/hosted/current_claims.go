package hosted

import (
	"context"
	"database/sql"
	"net/http"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/networking"
)

type currentClaims struct {
	cleanup   *claimCleanup
	store     *networking.SQLiteStore
	runner    *networking.AuthorityRunner
	transport *networking.HTTPTransport
}

func (s *Service) UseCurrentClaims(store *networking.SQLiteStore, runner *networking.AuthorityRunner, transport *networking.HTTPTransport) error {
	if store == nil || runner == nil || transport == nil || !s.Configured() {
		return networking.ErrInvalid
	}
	s.current = &currentClaims{store: store, runner: runner, transport: transport}
	return nil
}
func (s *Service) currentCall(ctx context.Context, method, path string, in, out any) error {
	if s.current == nil {
		return networking.ErrUnavailable
	}
	return s.current.runner.Do(ctx, func(ctx context.Context) error {
		v, e := s.current.store.InstalledIntent(ctx)
		if e != nil {
			return e
		}
		if method != http.MethodPost {
			return networking.ErrInvalid
		}
		var operation networking.ServerOperation
		switch path {
		case "/v1/servers/" + v.ServerID + "/heartbeat":
			operation = networking.SendHeartbeat
		case "/v1/servers/" + v.ServerID + "/members":
			operation = networking.PushMembers
		default:
			return networking.ErrInvalid
		}
		return s.current.transport.CallServer(ctx, v, operation, in, out)
	})
}

// RevokeClaimTx is the actual same-transaction terminal/cancel hook. It retains
// immutable keys and cancellation history while fencing every Hosted family.
func (s *Service) RevokeClaimTx(ctx context.Context, tx *sql.Tx, v networking.Intent, state string) error {
	if tx == nil || v.ServerID != s.identity.ID() {
		return networking.ErrStale
	}
	if e := networking.RevokeCertificatesTx(ctx, tx, v.OperationID); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `DELETE FROM policy WHERE server_id=?`, v.ServerID); e != nil {
		return e
	}
	if e := identity.RevokeFamiliesMatchingTx(ctx, tx, identity.RevokedMembershipRemoved, `authority='hosted'`); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `UPDATE playback_sessions SET state='stopped' WHERE session_hash IN(SELECT hash FROM authorization_access WHERE authority='hosted')`)
	return e
}
