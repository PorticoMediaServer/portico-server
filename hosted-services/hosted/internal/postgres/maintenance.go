package postgres

import (
	"context"
	"errors"
	"runtime"
	"time"
)

// Each table gets its own bounded batch and deadline. Cancellation stops all work;
// an individual busy table never starves unrelated retention work.
func (s *Store) pruneStatements(ctx context.Context, statements []string, args ...any) error {
	_, err := s.pruneStatementsCounted(ctx, statements, args...)
	return err
}

// pruneStatementsCounted also reports the most rows any one statement touched,
// so a caller can tell a full batch (more to do) from a short one.
func (s *Store) pruneStatementsCounted(ctx context.Context, statements []string, args ...any) (int64, error) {
	var failures []error
	var most int64
	for _, q := range statements {
		if ctx.Err() != nil {
			return most, errors.Join(append(failures, ctx.Err())...)
		}
		c, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		result, e := s.DB.ExecContext(c, q, args...)
		cancel()
		runtime.Gosched()
		if e != nil {
			failures = append(failures, e)
			continue
		}
		if n, err := result.RowsAffected(); err == nil && n > most {
			most = n
		}
	}
	return most, errors.Join(failures...)
}
func (s *Store) PruneExpired(ctx context.Context, limit int) error {
	_, err := s.pruneExpiredOnce(ctx, limit)
	return err
}

// PruneUntilCaughtUp repeats the bounded retention pass until no table had a
// full batch, yielding between passes, or ctx ends (A44). Each statement is its
// own short transaction of at most limit rows.
func (s *Store) PruneUntilCaughtUp(ctx context.Context, limit int) error {
	for {
		more, err := s.pruneExpiredOnce(ctx, limit)
		if err != nil || !more || ctx.Err() != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (s *Store) pruneExpiredOnce(ctx context.Context, limit int) (bool, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	c, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	e := s.pruneDeletions(c, limit)
	cancel()
	most, err := s.pruneStatementsCounted(ctx, []string{
		`DELETE FROM server_member_endings WHERE (server_id,account_id) IN(SELECT server_id,account_id FROM server_member_endings WHERE expires_at<=clock_timestamp() ORDER BY expires_at LIMIT $1)`,
		`DELETE FROM server_member_incarnations a WHERE (server_id,account_id) IN(SELECT i.server_id,i.account_id FROM server_member_incarnations i WHERE i.retained_until<=clock_timestamp() AND NOT EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=i.server_id AND m.account_id=i.account_id) ORDER BY i.retained_until LIMIT $1)`,
		`DELETE FROM setup_claim_starts WHERE transaction_id IN(SELECT transaction_id FROM setup_claim_starts WHERE expires_at<clock_timestamp()-interval '1 day' ORDER BY expires_at LIMIT $1)`,
		`DELETE FROM owner_claim_starts WHERE transaction_id IN(SELECT transaction_id FROM owner_claim_starts WHERE expires_at<clock_timestamp()-interval '1 day' ORDER BY expires_at LIMIT $1)`,
		`DELETE FROM account_exports WHERE id IN(SELECT id FROM account_exports WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT $1)`,
		`UPDATE refresh_receipts SET ciphertext=NULL WHERE (family_id,request_id) IN (SELECT family_id,request_id FROM refresh_receipts WHERE ciphertext IS NOT NULL AND expires_at<now() ORDER BY expires_at LIMIT $1)`,
		`DELETE FROM app_authorizations WHERE id IN(SELECT id FROM app_authorizations WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT $1)`,
		`DELETE FROM device_rate_limits WHERE (subject_hash,kind) IN (SELECT subject_hash,kind FROM device_rate_limits WHERE window_start<now()-interval '1 day' ORDER BY window_start LIMIT $1)`,
		`UPDATE device_authorizations SET session_cipher=NULL,tv_grant=NULL WHERE id IN (SELECT id FROM device_authorizations WHERE recovery_until<now() AND session_cipher IS NOT NULL LIMIT $1)`,
		`UPDATE device_authorizations SET user_hash=NULL WHERE id IN (SELECT id FROM device_authorizations WHERE expires_at<now() AND user_hash IS NOT NULL LIMIT $1)`,
		`DELETE FROM device_authorizations WHERE id IN (SELECT id FROM device_authorizations WHERE expires_at<now()-interval '1 day' ORDER BY expires_at LIMIT $1)`,
		`DELETE FROM account_sessions WHERE token_hash IN (SELECT token_hash FROM account_sessions WHERE expires_at<now() ORDER BY expires_at LIMIT $1)`,
		`DELETE FROM refresh_receipts WHERE (family_id,request_id) IN(SELECT family_id,request_id FROM refresh_receipts WHERE expires_at<now()-interval '1 day' ORDER BY expires_at LIMIT $1)`,
		`DELETE FROM refresh_tokens WHERE token_hash IN(SELECT t.token_hash FROM refresh_tokens t WHERE t.consumed_at<now()-interval '1 day' AND NOT EXISTS(SELECT 1 FROM refresh_receipts r WHERE r.input_hash=t.token_hash AND r.ciphertext IS NOT NULL AND r.expires_at>now()) ORDER BY t.consumed_at LIMIT $1)`,
		`DELETE FROM session_families WHERE id IN(SELECT id FROM session_families WHERE LEAST(COALESCE(revoked_at,'infinity'),idle_expires_at,absolute_expires_at)<now()-interval '30 days' LIMIT $1)`,
		`DELETE FROM direct_address_events WHERE id IN(SELECT id FROM direct_address_events WHERE observed_at<now()-interval '2 days' ORDER BY observed_at LIMIT $1)`,
		`DELETE FROM security_audit WHERE id IN(SELECT id FROM security_audit WHERE created_at<now()-interval '7 days' ORDER BY created_at LIMIT $1)`,
		`DELETE FROM claim_nonces WHERE id IN(SELECT id FROM claim_nonces WHERE expires_at<now()-interval '1 day' ORDER BY expires_at LIMIT $1)`,
		`DELETE FROM certificate_order_admissions WHERE id IN(SELECT id FROM certificate_order_admissions WHERE admitted_at<now()-interval '1 day' ORDER BY admitted_at LIMIT $1)`,
		`DELETE FROM certificate_orders WHERE id IN(SELECT o.id FROM certificate_orders o WHERE o.updated_at<now()-interval '1 day' AND NOT EXISTS(SELECT 1 FROM certificate_challenges c WHERE c.order_id=o.id AND NOT c.cleanup_done) AND ((o.state IN('cancelled','revoked','failed') AND o.revocation_done) OR (o.state='issued' AND NOT o.cancel_requested AND (SELECT count(*) FROM (SELECT 1 FROM certificate_orders newer WHERE newer.namespace=o.namespace AND newer.state='issued' AND (newer.created_at,newer.id)>(o.created_at,o.id) LIMIT 2) n)=2)) ORDER BY o.updated_at LIMIT $1)`,
		`UPDATE appstore_notifications SET payload='{}',original_transaction_id='',app_account_token='',account_id=NULL WHERE uuid IN(SELECT uuid FROM appstore_notifications WHERE received_at<now()-interval '30 days' AND payload<>'{}'::jsonb ORDER BY received_at LIMIT $1)`,
		`DELETE FROM account_notification_events WHERE id IN(SELECT id FROM account_notification_events WHERE created_at<now()-interval '7 days' OR read_at<now()-interval '90 days' ORDER BY created_at LIMIT $1)`,
		`DELETE FROM account_password_backoff WHERE (account_id,network) IN(SELECT account_id,network FROM account_password_backoff WHERE window_start<clock_timestamp()-interval '1 day' AND locked_until<clock_timestamp() ORDER BY window_start LIMIT $1)`,
		`DELETE FROM claim_web_pending WHERE operation_id IN(SELECT operation_id FROM claim_web_pending WHERE expires_at<now()-interval '1 day' ORDER BY expires_at LIMIT $1)`,
	}, limit)
	return most >= int64(limit), errors.Join(e, err)
}
