package postgres

import (
	"context"
	"database/sql"
)

// Each arm has a matching partial index. Completed receipts without outstanding
// TXT ownership cannot enter any arm. LIMIT bounds candidate locks, not just
// returned jobs. Inactive quarantine history is not traversed on normal ticks.
const certificateWorkSQL = `SELECT c.id FROM certificate_orders c JOIN (
 (SELECT id,next_attempt_at AS due,created_at FROM certificate_orders
  WHERE issuer=$1 AND next_attempt_at<=now() AND (lease_until IS NULL OR lease_until<now()) AND
  (state IN ('queued','creating','ordering','finalizing') OR (state='failed' AND NOT revocation_done) OR (cancel_requested AND NOT revocation_done AND state<>'reconciliation_required'))
  ORDER BY next_attempt_at,created_at,id LIMIT 32)
 UNION
 (SELECT id,next_attempt_at AS due,created_at FROM certificate_orders
  WHERE $2 AND issuer=$1 AND next_attempt_at<=now() AND (lease_until IS NULL OR lease_until<now()) AND state='reconciliation_required' AND NOT cancel_requested
  ORDER BY next_attempt_at,created_at,id LIMIT 32)
 UNION
 (SELECT id,GREATEST(next_attempt_at,cleanup_due_at) AS due,created_at FROM certificate_orders
  WHERE issuer=$1 AND cleanup_due_at IS NOT NULL AND GREATEST(next_attempt_at,cleanup_due_at)<=now() AND (lease_until IS NULL OR lease_until<now())
  AND state IN ('issued','failed','cancelled','revoked','reconciliation_required')
  ORDER BY GREATEST(next_attempt_at,cleanup_due_at),created_at,id LIMIT 32)
 ) work ON work.id=c.id
 WHERE c.lease_until IS NULL OR c.lease_until<now()
 ORDER BY work.due,work.created_at,c.id FOR UPDATE OF c SKIP LOCKED LIMIT 1`

// Issued and quarantined custody is reconciled in durable, due-time order. Every
// inspected row is moved out of the due prefix for five minutes, even when no
// job is leased. Thus one worker tick examines at most 32 authority tuples,
// rather than all issued receipts or all terminal history. This is deny-only:
// positive provider operations still use the full locking authority check.
const certificateAuthoritySQL = `SELECT id FROM certificate_orders
 WHERE issuer=$1 AND state IN ('issued','reconciliation_required') AND NOT cancel_requested
 AND authority_check_at<=now() AND (lease_until IS NULL OR lease_until<now())
 ORDER BY authority_check_at,id FOR UPDATE SKIP LOCKED LIMIT 32`

func reconcileCertificateAuthorityTx(ctx context.Context, tx *sql.Tx, issuer string) error {
	rows, err := tx.QueryContext(ctx, certificateAuthoritySQL, issuer)
	if err != nil {
		return err
	}
	ids := make([]string, 0, 32)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	readErr := rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if readErr != nil {
		return readErr
	}
	for _, id := range ids {
		// Lock order is intentionally jobs only, like queue selection. A stale
		// observation can delay deny-only cleanup, never authorize issuance.
		_, err = tx.ExecContext(ctx, `UPDATE certificate_orders c SET authority_check_at=now()+interval '5 minutes',
   cancel_requested=NOT EXISTS(SELECT 1 FROM servers s JOIN claim_operations o ON o.id=c.operation_id JOIN accounts a ON a.id=c.account_id
    WHERE s.id=c.server_id AND s.owner_account_id=c.account_id AND s.revoked_at IS NULL
    AND s.claim_generation=c.claim_generation AND s.credential_generation=c.credential_generation AND s.token_hash=c.token_hash
    AND o.state='installed' AND o.account_id=c.account_id AND o.server_id=c.server_id
    AND o.claim_generation=c.claim_generation AND o.credential_generation=c.credential_generation
    AND o.public_key=c.public_key AND s.public_key=rtrim(translate(encode(c.public_key,'base64'),'+/','-_'),'=') AND o.committed_token_hash=c.token_hash)
   WHERE c.id=$1`, id)
		if err != nil {
			return err
		}
		// A revoked ambiguous creation stays owned. The worker will quarantine it
		// again until an operator recovers its provider URL; never lose custody.
		_, err = tx.ExecContext(ctx, `UPDATE certificate_orders SET state='cancelled',next_attempt_at=now(),updated_at=now() WHERE id=$1 AND cancel_requested`, id)
		if err != nil {
			return err
		}
	}
	return nil
}
func updateCertificateCleanupDueTx(ctx context.Context, tx *sql.Tx, id string) error {
	_, err := tx.ExecContext(ctx, `UPDATE certificate_orders SET cleanup_due_at=(SELECT min(cleanup_after) FROM certificate_challenges WHERE order_id=$1 AND NOT cleanup_done) WHERE id=$1`, id)
	return err
}
