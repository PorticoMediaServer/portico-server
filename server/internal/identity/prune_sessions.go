package identity

import (
	"context"
	"time"

	"portico.local/server/internal/dbwork"
)

// PruneSessionHistory removes expired encrypted replay payloads and retired
// access tokens in bounded, lowest-priority writes. Refresh predecessor hashes
// remain while a family is live so reuse still revokes it after receipt expiry.
func (s *Service) PruneSessionHistory(ctx context.Context) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	tasks := []struct {
		pending string
		remove  string
		args    []any
	}{
		{
			`SELECT EXISTS(SELECT 1 FROM identity_refresh_receipts WHERE expires_at<=? LIMIT 1)`,
			`DELETE FROM identity_refresh_receipts WHERE predecessor_hash IN (SELECT predecessor_hash FROM identity_refresh_receipts WHERE expires_at<=? ORDER BY expires_at LIMIT 128)`,
			[]any{now},
		},
		{
			`SELECT EXISTS(SELECT 1 FROM authorization_family_tokens t WHERE t.retired=1 AND t.expires_at<=? AND NOT EXISTS(SELECT 1 FROM authorization_family_renewals r WHERE r.predecessor_hash=t.token_hash) LIMIT 1)`,
			`DELETE FROM authorization_family_tokens WHERE token_hash IN (SELECT t.token_hash FROM authorization_family_tokens t WHERE t.retired=1 AND t.expires_at<=? AND NOT EXISTS(SELECT 1 FROM authorization_family_renewals r WHERE r.predecessor_hash=t.token_hash) ORDER BY t.expires_at LIMIT 128)`,
			[]any{now},
		},
		{
			`SELECT EXISTS(SELECT 1 FROM authorization_session_families f INDEXED BY authorization_families_retention JOIN identity_refresh_predecessors p ON p.family_id=f.id WHERE f.revoked=1 LIMIT 1)
			 OR EXISTS(SELECT 1 FROM authorization_session_families f INDEXED BY authorization_families_horizon JOIN identity_refresh_predecessors p ON p.family_id=f.id WHERE f.authorization_horizon<=? LIMIT 1)`,
			`DELETE FROM identity_refresh_predecessors WHERE predecessor_hash IN (SELECT predecessor_hash FROM (
			 SELECT p.predecessor_hash FROM authorization_session_families f INDEXED BY authorization_families_retention JOIN identity_refresh_predecessors p ON p.family_id=f.id WHERE f.revoked=1
			 UNION ALL
			 SELECT p.predecessor_hash FROM authorization_session_families f INDEXED BY authorization_families_horizon JOIN identity_refresh_predecessors p ON p.family_id=f.id WHERE f.authorization_horizon<=?
			 ) LIMIT 128)`,
			[]any{now},
		},
	}
	var total int64
	for _, task := range tasks {
		var due bool
		if err := s.db.QueryRowContext(ctx, task.pending, task.args...).Scan(&due); err != nil {
			return total, err
		}
		if !due {
			continue
		}
		result, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassMaintenance, task.remove, task.args...)
		if err != nil {
			return total, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
