package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/directory"
)

// These are the current installed-server channel only. Revoked predecessor
// cleanup requires its separate purpose-bound Local recovery contract.
func (s *Store) ServerErasures(ctx context.Context, a directory.ServerAuthority) ([]directory.ErasureJob, error) {
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT id,COALESCE(deletion_id,''),server_id,account_id,array_to_json(profile_ids),revision,disposition FROM account_erasure_jobs WHERE server_id=$1 AND claim_operation_id=$2 AND claim_generation=$3 AND credential_generation=$4 AND acknowledged_at IS NULL ORDER BY id LIMIT 100`, a.ServerID, a.OperationID, a.ClaimGeneration, a.CredentialGeneration)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []directory.ErasureJob{}
	for rows.Next() {
		var j directory.ErasureJob
		var raw []byte
		if e = rows.Scan(&j.ID, &j.DeletionID, &j.ServerID, &j.AccountID, &raw, &j.Revision, &j.Disposition); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &j.ProfileIDs); e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if e = rows.Close(); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}
func (s *Store) ServerAcknowledgeErasure(ctx context.Context, a directory.ServerAuthority, id string, revision int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = lockServerAuthorityOwnerTx(ctx, tx, a); e != nil {
		return e
	}
	// Jobs from server retirement or succession carry no account deletion.
	var deletion sql.NullString
	e = tx.QueryRowContext(ctx, `SELECT deletion_id FROM account_erasure_jobs WHERE id=$1 AND server_id=$2 AND revision=$3 AND claim_operation_id=$4 AND claim_generation=$5 AND credential_generation=$6`, id, a.ServerID, revision, a.OperationID, a.ClaimGeneration, a.CredentialGeneration).Scan(&deletion)
	if errors.Is(e, sql.ErrNoRows) {
		return directory.ErrDenied
	}
	if e != nil {
		return e
	}
	// Parent before server follows the account-deletion writer's order and avoids
	// inverting its deletion-parent -> affected-server lock sequence.
	if deletion.Valid {
		var ignored string
		if e = tx.QueryRowContext(ctx, `SELECT id FROM account_deletions WHERE id=$1 FOR UPDATE`, deletion.String).Scan(&ignored); e != nil {
			return e
		}
	}
	if e = checkServerAuthorityTx(ctx, tx, a); e != nil {
		return e
	}
	var prior sql.NullTime
	if e = tx.QueryRowContext(ctx, `SELECT acknowledged_at FROM account_erasure_jobs WHERE id=$1 AND claim_operation_id=$2 AND claim_generation=$3 AND credential_generation=$4 FOR UPDATE`, id, a.OperationID, a.ClaimGeneration, a.CredentialGeneration).Scan(&prior); e != nil {
		return e
	}
	if !prior.Valid {
		if _, e = tx.ExecContext(ctx, `UPDATE account_erasure_jobs SET acknowledged_at=clock_timestamp(),account_id='',profile_ids='{}' WHERE id=$1`, id); e != nil {
			return e
		}
		var pr int64
		if e = tx.QueryRowContext(ctx, `UPDATE servers SET policy_revision=CASE WHEN policy_revision<9223372036854775807 THEN policy_revision+1 ELSE policy_revision END,policy_issued_at=clock_timestamp() WHERE id=$1 RETURNING policy_revision`, a.ServerID).Scan(&pr); e != nil {
			return e
		}
		if e = enqueueWake(ctx, tx, a.ServerID, pr); e != nil {
			return e
		}
	}
	if deletion.Valid {
		if _, e = tx.ExecContext(ctx, `UPDATE account_deletions SET completed_at=COALESCE(completed_at,clock_timestamp()) WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM account_erasure_jobs WHERE deletion_id=$1 AND acknowledged_at IS NULL)`, deletion.String); e != nil {
			return e
		}
	}
	return tx.Commit()
}
