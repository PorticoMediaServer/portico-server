package postgres

import "context"

// Expire central deletion capabilities independently of departure delivery.
func (s *Store) pruneDeletions(ctx context.Context, limit int) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `DELETE FROM account_deletions WHERE id IN(SELECT id FROM account_deletions WHERE accepted_at IS NULL AND expires_at<now() ORDER BY expires_at LIMIT $1)`, limit); e != nil {
		return e
	}
	// The receipt window bounds replay credentials. Departure delivery retains
	// only the account ID needed by its server, independently of this capability.
	if _, e = tx.ExecContext(ctx, `UPDATE account_deletions SET expected_access_hash=NULL,expected_family_id=NULL,expected_account_epoch=NULL,expected_family_generation=NULL,expected_session_mode=NULL WHERE id IN(SELECT id FROM account_deletions WHERE accepted_at IS NOT NULL AND receipt_expires_at<clock_timestamp() AND expected_access_hash IS NOT NULL ORDER BY receipt_expires_at LIMIT $1)`, limit); e != nil {
		return e
	}
	rows, e := tx.QueryContext(ctx, `SELECT id FROM account_deletions WHERE completed_at IS NOT NULL AND receipt_expires_at<now() ORDER BY receipt_expires_at LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if _, e = tx.ExecContext(ctx, `DELETE FROM security_audit WHERE actor_id=$1 OR target_id=$1`, id); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM account_deletions WHERE id=$1`, id); e != nil {
			return e
		}
	}
	return tx.Commit()
}
