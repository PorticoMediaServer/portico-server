package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/trust"
)

// ServerHeartbeat is the check-in: weekly when Hosted can wake the server, daily
// when it cannot, and at once after a wake. It carries no policy. The server
// reports the wake it is answering and the state of its member list; the
// answer says whether Hosted's index agrees and whether erasure jobs wait.
func (s *Store) ServerHeartbeat(ctx context.Context, authority directory.ServerAuthority, q directory.CheckInReport) (directory.Heartbeat, error) {
	server := authority.ServerID
	tx, e := s.beginServerOperation(ctx, authority)
	if e != nil {
		return directory.Heartbeat{}, e
	}
	defer tx.Rollback()
	var result directory.Heartbeat
	var wakeable bool
	_, wakeablePresence := directory.CheckIn(true)
	_, unreachablePresence := directory.CheckIn(false)
	e = tx.QueryRowContext(ctx, `UPDATE servers SET last_heartbeat_at=now(),presence_expires_at=now()+CASE WHEN base_url IS NOT NULL AND endpoint_verified THEN $2::interval ELSE $3::interval END WHERE id=$1 RETURNING presence_expires_at,base_url IS NOT NULL AND endpoint_verified,COALESCE((SELECT member_label FROM certificate_namespaces WHERE server_identity=servers.id),'')`, server, pgInterval(wakeablePresence), pgInterval(unreachablePresence)).Scan(&result.PresenceExpiresAt, &wakeable, &result.RouteLabel)
	if errors.Is(e, sql.ErrNoRows) {
		return result, directory.ErrConflict
	}
	if e != nil {
		return result, e
	}
	after, _ := directory.CheckIn(wakeable)
	result.HeartbeatAfterSeconds = int(after / time.Second)
	if q.WakeRevision > 0 {
		if _, e = tx.ExecContext(ctx, `UPDATE wake_state SET acked_revision=GREATEST(acked_revision,LEAST(desired_revision,$2)) WHERE server_id=$1`, server, q.WakeRevision); e != nil {
			return result, e
		}
	}
	var digest string
	if result.MemberSequence, digest, e = memberIndexTx(ctx, tx, server); e != nil {
		return result, e
	}
	result.MemberDigestMatches = digest == q.MemberDigest && result.MemberSequence == q.MemberSequence
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_erasure_jobs WHERE server_id=$1 AND claim_operation_id=$2 AND claim_generation=$3 AND credential_generation=$4 AND acknowledged_at IS NULL)`, server, authority.OperationID, authority.ClaimGeneration, authority.CredentialGeneration).Scan(&result.CleanupPending); e != nil {
		return result, e
	}
	result.Departures = []directory.Departure{}
	rows, e := tx.QueryContext(ctx, `SELECT account_id,left_at FROM server_member_departures WHERE server_id=$1 AND pending ORDER BY left_at LIMIT 1000`, server)
	if e != nil {
		return result, e
	}
	for rows.Next() {
		var d directory.Departure
		if e = rows.Scan(&d.AccountID, &d.LeftAt); e != nil {
			rows.Close()
			return result, e
		}
		d.LeftAt = d.LeftAt.UTC()
		result.Departures = append(result.Departures, d)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return result, e
	}
	// The root-signed certificate is a public artifact, not this process's
	// signing authority. An old replica can deliver the newer certificate.
	var certificate []byte
	if e = tx.QueryRowContext(ctx, `SELECT certificate FROM hosted_signing_state WHERE singleton`).Scan(&certificate); e != nil {
		return result, e
	}
	if len(certificate) != 0 {
		result.SigningCertificate = &trust.Certificate{}
		// Invalid stored bytes are ignored by the service's root verification;
		// they never prevent presence or wake acknowledgment from committing.
		_ = json.Unmarshal(certificate, result.SigningCertificate)
	}
	return result, tx.Commit()
}
