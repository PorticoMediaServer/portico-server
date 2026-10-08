package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"strconv"
)

func (s *Store) ClaimCleanup(ctx context.Context, w claims.CleanupWire) (bool, []directory.ErasureJob, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return false, nil, e
	}
	defer tx.Rollback()
	// The exact original key remains cleanup-only after retirement/reinstallation.
	// Never authorize this channel with a stale or successor bearer.
	if w.AcknowledgeJobID != "" {
		var deletion sql.NullString
		e = tx.QueryRowContext(ctx, `SELECT deletion_id FROM account_erasure_jobs WHERE id=$1 AND server_id=$2 AND claim_operation_id=$3 AND claim_generation=$4 AND credential_generation=$5 AND revision=$6`, w.AcknowledgeJobID, w.ServerID, w.OperationID, w.ClaimGeneration, w.CredentialGeneration, w.AcknowledgeRevision).Scan(&deletion)
		if e != nil {
			return false, nil, cleanupError(e)
		}
		var locked string
		if deletion.Valid {
			if e = tx.QueryRowContext(ctx, `SELECT id FROM account_deletions WHERE id=$1 FOR UPDATE`, deletion.String).Scan(&locked); e != nil {
				return false, nil, e
			}
		}
	}
	var server, state string
	var key []byte
	var claim, credential sql.NullInt64
	e = tx.QueryRowContext(ctx, `SELECT server_id,state,public_key,claim_generation,credential_generation FROM claim_operations WHERE id=$1 FOR SHARE`, w.OperationID).Scan(&server, &state, &key, &claim, &credential)
	if e != nil {
		return false, nil, cleanupError(e)
	}
	supplied, e := base64.RawURLEncoding.Strict().DecodeString(w.PublicKey)
	if e != nil || !bytes.Equal(key, supplied) || server != w.ServerID || !claim.Valid || !credential.Valid || strconv.FormatInt(claim.Int64, 10) != w.ClaimGeneration || strconv.FormatInt(credential.Int64, 10) != w.CredentialGeneration {
		return false, nil, claims.ErrStale
	}
	if w.AcknowledgeJobID != "" {
		// Remove target identifiers as soon as erasure is acknowledged, retaining
		// only the immutable job receipt for safe repeated acknowledgements.
		if _, e = tx.ExecContext(ctx, `UPDATE account_erasure_jobs SET acknowledged_at=COALESCE(acknowledged_at,clock_timestamp()),account_id='',profile_ids='{}' WHERE id=$1`, w.AcknowledgeJobID); e != nil {
			return false, nil, e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE account_deletions SET completed_at=COALESCE(completed_at,clock_timestamp()) WHERE id=(SELECT deletion_id FROM account_erasure_jobs WHERE id=$1) AND NOT EXISTS(SELECT 1 FROM account_erasure_jobs j WHERE j.deletion_id=account_deletions.id AND j.acknowledged_at IS NULL)`, w.AcknowledgeJobID); e != nil {
			return false, nil, e
		}
	}
	rows, e := tx.QueryContext(ctx, `SELECT id,COALESCE(deletion_id,id),server_id,account_id,array_to_json(profile_ids),revision,disposition FROM account_erasure_jobs WHERE server_id=$1 AND claim_operation_id=$2 AND claim_generation=$3 AND credential_generation=$4 AND acknowledged_at IS NULL ORDER BY id LIMIT 10`, w.ServerID, w.OperationID, claim.Int64, credential.Int64)
	if e != nil {
		return false, nil, e
	}
	jobs := []directory.ErasureJob{}
	for rows.Next() {
		var j directory.ErasureJob
		var raw []byte
		if e = rows.Scan(&j.ID, &j.DeletionID, &j.ServerID, &j.AccountID, &raw, &j.Revision, &j.Disposition); e != nil {
			rows.Close()
			return false, nil, e
		}
		if e = json.Unmarshal(raw, &j.ProfileIDs); e != nil {
			rows.Close()
			return false, nil, e
		}
		jobs = append(jobs, j)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return false, nil, e
	}
	return state == "cancelled", jobs, tx.Commit()
}
func cleanupError(e error) error {
	if errors.Is(e, sql.ErrNoRows) {
		return claims.ErrStale
	}
	return e
}
