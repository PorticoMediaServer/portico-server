package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/directory"
)

func (s *Store) Erasures(ctx context.Context, server string) ([]directory.ErasureJob, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,deletion_id,server_id,account_id,array_to_json(profile_ids),revision,disposition FROM account_erasure_jobs WHERE server_id=$1 AND acknowledged_at IS NULL ORDER BY id LIMIT 100`, server)
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
	return out, rows.Err()
}
func (s *Store) AcknowledgeErasure(ctx context.Context, server, id string, revision int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var deletion string
	e = tx.QueryRowContext(ctx, `SELECT deletion_id FROM account_erasure_jobs WHERE id=$1 AND server_id=$2 AND revision=$3`, id, server, revision).Scan(&deletion)
	if errors.Is(e, sql.ErrNoRows) {
		return directory.ErrDenied
	}
	if e != nil {
		return e
	}
	// Serialize multi-server completion around the parent tombstone.
	var ignored string
	if e = tx.QueryRowContext(ctx, `SELECT id FROM account_deletions WHERE id=$1 FOR UPDATE`, deletion).Scan(&ignored); e != nil {
		return e
	}
	var prior sql.NullTime
	if e = tx.QueryRowContext(ctx, `SELECT acknowledged_at FROM account_erasure_jobs WHERE id=$1 FOR UPDATE`, id).Scan(&prior); e != nil {
		return e
	}
	if !prior.Valid {
		if _, e = tx.ExecContext(ctx, `UPDATE account_erasure_jobs SET acknowledged_at=now(),account_id='',profile_ids='{}' WHERE id=$1`, id); e != nil {
			return e
		}
		var pr int64
		if e = tx.QueryRowContext(ctx, `UPDATE servers SET policy_revision=policy_revision+1,policy_issued_at=now() WHERE id=$1 RETURNING policy_revision`, server).Scan(&pr); e != nil {
			return e
		}
		if e = enqueueWake(ctx, tx, server, pr); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_deletions SET completed_at=COALESCE(completed_at,now()) WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM account_erasure_jobs WHERE deletion_id=$1 AND acknowledged_at IS NULL)`, deletion); e != nil {
		return e
	}
	return tx.Commit()
}
