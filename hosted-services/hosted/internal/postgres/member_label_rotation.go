package postgres

import (
	"context"
	"database/sql"
	"errors"
)

// RotateQueuedMemberLabels rotates up to limit queued servers, each in its own
// short transaction, so the direct-address feed lock is held only for one
// server's rotation and never inside a user's transaction (A87). It reports
// whether more may be queued.
func (s *Store) RotateQueuedMemberLabels(ctx context.Context, limit int) (bool, error) {
	for i := 0; i < limit; i++ {
		done, e := s.rotateOneQueuedLabel(ctx)
		if e != nil || done {
			return false, e
		}
	}
	return true, nil
}
func (s *Store) rotateOneQueuedLabel(ctx context.Context) (bool, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return false, e
	}
	defer tx.Rollback()
	var server string
	// A server whose previous label is still in its grace period waits: rotating
	// again would replace that label, and names in use would stop resolving
	// before their promised 24 hours. A removed member loses access through
	// policy at once regardless; only the rename waits (the worker's safety
	// wake picks it up after the grace ends).
	e = tx.QueryRowContext(ctx, `DELETE FROM member_label_rotations WHERE server_id=(SELECT r.server_id FROM member_label_rotations r WHERE NOT EXISTS(SELECT 1 FROM certificate_namespaces n WHERE n.server_identity=r.server_id AND n.previous_label_until>clock_timestamp()) ORDER BY r.requested_at LIMIT 1 FOR UPDATE OF r SKIP LOCKED) RETURNING server_id`).Scan(&server)
	if errors.Is(e, sql.ErrNoRows) {
		return true, nil
	}
	if e != nil {
		return false, e
	}
	if _, e = tx.ExecContext(ctx, `SELECT rotate_member_label($1)`, server); e != nil {
		return false, e
	}
	// The server learns its new label from its check-in answer, so wake it
	// (Spec — Hosted at Scale: a removal is pushed by the server, and nothing
	// else would make it check in before its weekly deadline).
	var wake int64
	e = tx.QueryRowContext(ctx, `UPDATE servers SET policy_revision=CASE WHEN policy_revision<9223372036854775807 THEN policy_revision+1 ELSE policy_revision END WHERE id=$1 AND revoked_at IS NULL RETURNING policy_revision`, server).Scan(&wake)
	if e == nil {
		e = enqueueWake(ctx, tx, server, wake)
	}
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return false, e
	}
	return false, tx.Commit()
}
