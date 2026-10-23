package postgres

import (
	"context"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/egress"
)

func (s *Store) RegisterEndpoint(ctx context.Context, a directory.ServerAuthority, v directory.EndpointEvidence) (directory.EndpointReceipt, error) {
	var out directory.EndpointReceipt
	origin, _, safe := egress.ServerOrigin(v.BaseURL, false)
	if safe != nil || origin != v.BaseURL || len(v.Digest) != 32 {
		return out, directory.ErrInvalid
	}
	tx, err := s.beginServerOperation(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if v.Generation > 0 {
		var current bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM networking_route_state s JOIN networking_route_candidates c ON c.server_id=s.server_id AND c.generation=s.generation WHERE s.server_id=$1 AND s.operation_id=$2 AND s.generation=$3 AND c.origin=$4 AND c.class<>'lan' AND c.state<>'identity_mismatch')`, a.ServerID, a.OperationID, v.Generation, v.BaseURL).Scan(&current); err != nil {
			return out, err
		}
		if !current {
			return out, directory.ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE networking_route_candidates SET state='verified',verified_at=clock_timestamp(),updated_at=clock_timestamp() WHERE server_id=$1 AND generation=$2 AND origin=$3`, a.ServerID, v.Generation, v.BaseURL); err != nil {
			return out, err
		}
	}
	var fresh bool
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()<$1`, v.ExpiresAt).Scan(&fresh); err != nil {
		return out, err
	}
	if !fresh {
		return out, directory.ErrDenied
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO current_server_endpoints(server_id,operation_id,account_id,claim_generation,credential_generation,origin,proof_digest) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(server_id) DO UPDATE SET operation_id=EXCLUDED.operation_id,account_id=EXCLUDED.account_id,claim_generation=EXCLUDED.claim_generation,credential_generation=EXCLUDED.credential_generation,origin=EXCLUDED.origin,proof_digest=EXCLUDED.proof_digest,verified_at=clock_timestamp() RETURNING origin,verified_at`, a.ServerID, a.OperationID, a.AccountID, a.ClaimGeneration, a.CredentialGeneration, v.BaseURL, v.Digest).Scan(&out.BaseURL, &out.VerifiedAt)
	if err != nil {
		return out, err
	}
	var moved bool
	if err = tx.QueryRowContext(ctx, `SELECT base_url IS DISTINCT FROM $2 OR NOT endpoint_verified FROM servers WHERE id=$1`, a.ServerID, v.BaseURL).Scan(&moved); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE servers SET base_url=$2,endpoint_verified=true WHERE id=$1`, a.ServerID, v.BaseURL); err != nil {
		return out, err
	}
	if moved {
		if err = touchServerListTx(ctx, tx, a.ServerID); err != nil {
			return out, err
		}
	}
	return out, tx.Commit()
}
