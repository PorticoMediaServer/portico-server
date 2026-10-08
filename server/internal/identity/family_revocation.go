package identity

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"
)

type RevocationReason string

const (
	RevokedRefreshReuse      RevocationReason = "refresh_reuse"
	RevokedExplicitSignout   RevocationReason = "explicit_signout"
	RevokedAdminRevoke       RevocationReason = "admin_revoke"
	RevokedMembershipRemoved RevocationReason = "membership_removed"
	RevokedDeviceDisapproved RevocationReason = "device_disapproved"
	RevokedPasswordChange    RevocationReason = "password_change"
)

func validRevocationReason(reason RevocationReason) bool {
	switch reason {
	case RevokedRefreshReuse, RevokedExplicitSignout, RevokedAdminRevoke, RevokedMembershipRemoved, RevokedDeviceDisapproved, RevokedPasswordChange:
		return true
	}
	return false
}

// RevokeFamilyTx stamps one active family and logs its identifier prefix. It
// never logs a bearer, refresh credential, or full family identifier.
func RevokeFamilyTx(ctx context.Context, tx *sql.Tx, id string, reason RevocationReason) error {
	if !validRevocationReason(reason) {
		return errors.New("invalid revocation reason")
	}
	var revoked string
	err := tx.QueryRowContext(ctx, `UPDATE authorization_session_families SET revoked=1,revoked_reason=?,revoked_at=? WHERE id=? AND revoked=0 RETURNING id`, string(reason), time.Now().UTC().Format(time.RFC3339), id).Scan(&revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	prefix := revoked
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	log.Printf("authorization family revoked family=%s reason=%s", prefix, reason)
	return nil
}

// RevokeFamiliesMatchingTx is for trusted, static internal predicates; values
// must always be passed as args. Each family gets its own reason and log line.
func RevokeFamiliesMatchingTx(ctx context.Context, tx *sql.Tx, reason RevocationReason, predicate string, args ...any) error {
	if !validRevocationReason(reason) {
		return errors.New("invalid revocation reason")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM authorization_session_families WHERE revoked=0 AND (`+predicate+`)`, args...)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = RevokeFamilyTx(ctx, tx, id, reason); err != nil {
			return err
		}
	}
	return nil
}
