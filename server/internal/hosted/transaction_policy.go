package hosted

import (
	"context"
	"database/sql"

	"portico.local/server/internal/identity"
)

type transactionPolicyReader struct {
	ctx context.Context
	tx  *sql.Tx
}

func (r transactionPolicyReader) QueryRow(query string, args ...any) *sql.Row {
	return r.tx.QueryRowContext(r.ctx, query, args...)
}

// AllowedTxContext reads the same cached membership and local restrictions as
// Allowed, using only the caller's transaction and cancellation context.
func (s *Service) AllowedTxContext(ctx context.Context, principal identity.Principal, library string, tx *sql.Tx) error {
	if tx == nil {
		return identity.ErrUnauthorized
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := s.allowed(principal, library, transactionPolicyReader{ctx, tx})
	if cancelled := ctx.Err(); cancelled != nil {
		return cancelled
	}
	return err
}
