package postgres

import (
	"context"
	"database/sql"

	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
)

type ServerRemoval struct {
	OperationID string `json:"operationId"`
	// Proof is ignored: removing a server needs a signed-in session (A80).
	Proof                string `json:"proof,omitempty"`
	ClaimGeneration      int64  `json:"claimGeneration,string"`
	CredentialGeneration int64  `json:"credentialGeneration,string"`
}

func (s *AccountSecurity) RemoveServer(ctx context.Context, token, server string, q ServerRemoval) error {
	intent := identity.SecurityIntent{Purpose: "server_remove", Target: server, OperationID: q.OperationID}
	if !intent.Valid() || q.ClaimGeneration < 1 || q.CredentialGeneration < 1 {
		return identity.ErrInvalid
	}
	tx, a, e := s.authorize(ctx, token)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	released, e := custodyReleasedTx(ctx, tx, server)
	if e != nil {
		return e
	}
	if released {
		return directory.ErrConflict
	}
	if e = retireServerTx(ctx, tx, a.AccountID, server, q.ClaimGeneration, q.CredentialGeneration, "owner_removed", ""); e != nil {
		return e
	}
	return tx.Commit()
}

// Caller holds the account lock. Retirement is bound to the complete generation
// tuple; an old request can never retire a re-claimed successor.
func retireServerTx(ctx context.Context, tx *sql.Tx, owner, server string, claim, credential int64, reason, replacement string) error {
	var op string
	e := tx.QueryRowContext(ctx, `SELECT o.id FROM claim_operations o JOIN servers s ON s.id=o.server_id WHERE s.id=$1 AND s.owner_account_id=$2 AND s.revoked_at IS NULL AND s.claim_generation=$3 AND s.credential_generation=$4 AND o.claim_generation=s.claim_generation AND o.credential_generation=s.credential_generation AND o.state IN('installed','committed_pending_install') FOR UPDATE OF o`, server, owner, claim, credential).Scan(&op)
	if e != nil {
		return claims.ErrStale
	}
	var id string
	if e = tx.QueryRowContext(ctx, `SELECT id FROM servers WHERE id=$1 AND owner_account_id=$2 AND claim_generation=$3 AND credential_generation=$4 FOR UPDATE`, server, owner, claim, credential).Scan(&id); e != nil {
		return claims.ErrStale
	}
	if e = withdrawClaimAddressesTx(ctx, tx, server, op); e != nil {
		return e
	}
	for _, q := range []string{
		`UPDATE servers SET owner_account_id=NULL,token_hash=NULL,token_ciphertext=NULL,revoked_at=clock_timestamp(),base_url=NULL,endpoint_verified=false,presence_expires_at=NULL,policy_revision=policy_revision+1,retired_reason=$2,replaced_by=NULLIF($3,'') WHERE id=$1`,
		`UPDATE claim_operations SET state='cancelled',cancelled_at=clock_timestamp() WHERE server_id=$1 AND state IN('installed','committed_pending_install') AND $2<>'' AND $3::text IS NOT NULL`,
		`DELETE FROM current_server_endpoints WHERE server_id=$1 AND $2<>'' AND $3::text IS NOT NULL`,
		`DELETE FROM wake_state WHERE server_id=$1 AND $2<>'' AND $3::text IS NOT NULL`,
		`UPDATE certificate_orders SET cancel_requested=true,next_attempt_at=clock_timestamp() WHERE server_id=$1 AND $2<>'' AND $3::text IS NOT NULL`,
	} {
		if _, e = tx.ExecContext(ctx, q, server, reason, replacement); e != nil {
			return e
		}
	}
	if e = dropServerIndexTx(ctx, tx, server, owner); e != nil {
		return e
	}
	if e = notifyTx(ctx, tx, CertificateChannel); e != nil {
		return e
	}
	return audit(ctx, tx, owner, "server.retired", server, claim)
}
