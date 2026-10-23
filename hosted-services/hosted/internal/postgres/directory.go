package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/hosted/internal/directory"
)

// List returns the servers the account can use. Retired servers are never
// listed: retirement revokes every membership (A24), so the list's status is
// active, quiet or dormant.
func (s *Store) List(ctx context.Context, account string) ([]directory.Server, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT s.id,s.name,COALESCE(e.origin,''),s.public_key,s.policy_revision,(e.server_id IS NOT NULL),s.last_seen_at,(COALESCE(s.owner_account_id=$1,false) AND NOT s.custody_released) OR EXISTS(SELECT 1 FROM server_members x WHERE x.server_id=s.id AND x.account_id=$1 AND x.role='owner'),CASE WHEN (COALESCE(s.owner_account_id=$1,false) AND NOT s.custody_released) OR EXISTS(SELECT 1 FROM server_members x WHERE x.server_id=s.id AND x.account_id=$1 AND x.role='owner') THEN 'owner' ELSE (SELECT role FROM server_members x WHERE x.server_id=s.id AND x.account_id=$1 AND x.role IN('admin','member')) END,CASE WHEN s.presence_expires_at>clock_timestamp() THEN 'active' WHEN s.last_seen_at>clock_timestamp()-interval '90 days' THEN 'quiet' ELSE 'dormant' END,GREATEST(s.last_heartbeat_at,s.last_seen_at) FROM servers s JOIN claim_operations o ON o.server_id=s.id AND o.account_id=s.owner_account_id AND o.claim_generation=s.claim_generation AND o.credential_generation=s.credential_generation AND o.committed_token_hash=s.token_hash AND o.state='installed' LEFT JOIN current_server_endpoints e ON e.server_id=s.id AND e.operation_id=o.id AND e.account_id=o.account_id AND e.claim_generation=o.claim_generation AND e.credential_generation=o.credential_generation WHERE s.revoked_at IS NULL AND s.id IN(SELECT id FROM servers WHERE owner_account_id=$1 AND NOT custody_released UNION SELECT server_id FROM server_members WHERE account_id=$1 AND role IN('owner','admin','member')) ORDER BY s.id`, account)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []directory.Server{}
	for rows.Next() {
		var v directory.Server
		var seen sql.NullTime
		if e = rows.Scan(&v.ID, &v.Name, &v.BaseURL, &v.PublicKey, &v.PolicyRevision, &v.EndpointVerified, &v.LastSeenAt, &v.OwnedByMe, &v.Role, &v.Status, &seen); e != nil {
			return nil, e
		}
		if seen.Valid {
			at := seen.Time.UTC()
			v.Presence.LastSeenAt = &at
		}
		if !v.OwnedByMe {
			v.LastSeenAt = v.LastSeenAt.UTC().Truncate(24 * time.Hour)
			if v.Presence.LastSeenAt != nil {
				day := v.Presence.LastSeenAt.Truncate(24 * time.Hour)
				v.Presence.LastSeenAt = &day
			}
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (s *Store) AuthenticateServer(ctx context.Context, hash []byte) (string, error) {
	var id string
	e := s.DB.QueryRowContext(ctx, `SELECT s.id FROM servers s JOIN claim_operations o ON o.server_id=s.id AND o.claim_generation=s.claim_generation AND o.credential_generation=s.credential_generation AND o.committed_token_hash=s.token_hash AND o.account_id=s.owner_account_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND o.state='installed'`, hash).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		e = directory.ErrDenied
	}
	return id, e
}
func enqueueWake(ctx context.Context, tx *sql.Tx, server string, revision int64) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO wake_state(server_id,kind,desired_revision) VALUES($1,'policy',$2) ON CONFLICT(server_id,kind) DO UPDATE SET first_attempt_at=CASE WHEN EXCLUDED.desired_revision>wake_state.desired_revision THEN NULL ELSE wake_state.first_attempt_at END,wake_unreachable=CASE WHEN EXCLUDED.desired_revision>wake_state.desired_revision THEN false ELSE wake_state.wake_unreachable END,attempts=CASE WHEN EXCLUDED.desired_revision>wake_state.desired_revision THEN 0 ELSE wake_state.attempts END,desired_revision=GREATEST(wake_state.desired_revision,EXCLUDED.desired_revision),next_attempt_at=LEAST(wake_state.next_attempt_at,now())`, server, revision)
	return e
}
func audit(ctx context.Context, tx *sql.Tx, actor, action, target string, rev int64) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO security_audit(actor_id,action,target_id,revision) VALUES($1,$2,$3,$4)`, actor, action, target, rev)
	return e
}
