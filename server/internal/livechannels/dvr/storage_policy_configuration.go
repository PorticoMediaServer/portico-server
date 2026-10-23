package dvr

import (
	"context"
	"database/sql"
)

// DefaultStoragePolicy is the factory policy for resetting recording storage.
// A current stored policy is always read instead of inferred from this value.
func DefaultStoragePolicy() StoragePolicy {
	return StoragePolicy{Revision: 1, FloorBytes: 10000000000}
}

func validStoragePolicy(p StoragePolicy) bool {
	return p.RetentionDays >= 0 && p.RetentionDays <= 3650 && p.EpisodeLimit >= 0 && p.EpisodeLimit <= 100000 && p.FloorBytes >= 0 && p.CapBytes >= 0 && p.FloorBytes <= 1<<53-1 && p.CapBytes <= 1<<53-1
}

type storagePolicyReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readStoragePolicy(ctx context.Context, db storagePolicyReader) (StoragePolicy, error) {
	var p StoragePolicy
	err := db.QueryRowContext(ctx, `SELECT revision,retention_days,episode_limit,floor_bytes,cap_bytes FROM dvr_storage_policy WHERE singleton=1`).Scan(&p.Revision, &p.RetentionDays, &p.EpisodeLimit, &p.FloorBytes, &p.CapBytes)
	if err != nil {
		return StoragePolicy{}, err
	}
	if p.Revision < 1 || !validStoragePolicy(p) {
		return StoragePolicy{}, ErrInvalid
	}
	return p, nil
}

// ReadStoragePolicyTx uses the caller's authorized snapshot, without a second
// policy document or a fallback when the normalized singleton cannot be read.
func ReadStoragePolicyTx(ctx context.Context, tx *sql.Tx) (StoragePolicy, error) {
	if tx == nil {
		return StoragePolicy{}, ErrInvalid
	}
	return readStoragePolicy(ctx, tx)
}

// ReplaceStoragePolicyTx shares the actual writer between the owner facade and
// configuration import. The caller supplies authorization, physical capability
// checks where applicable, its transaction, receipt and configuration source.
func ReplaceStoragePolicyTx(ctx context.Context, tx *sql.Tx, expected int64, p StoragePolicy) (StoragePolicy, error) {
	if tx == nil || expected < 1 || !validStoragePolicy(p) {
		return StoragePolicy{}, ErrInvalid
	}
	result, err := tx.ExecContext(ctx, `UPDATE dvr_storage_policy SET revision=revision+1,retention_days=?,episode_limit=?,floor_bytes=?,cap_bytes=? WHERE singleton=1 AND revision=?`, p.RetentionDays, p.EpisodeLimit, p.FloorBytes, p.CapBytes, expected)
	if err != nil {
		return StoragePolicy{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return StoragePolicy{}, err
	}
	if n != 1 {
		return StoragePolicy{}, ErrConflict
	}
	return ReadStoragePolicyTx(ctx, tx)
}

// RefreshStoragePolicy adopts the current committed policy for physical writes.
// Startup ConfigureCapture and bounded capture passes also use this reader.
func (s *Store) RefreshStoragePolicy(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	return s.refreshStoragePolicy(ctx)
}
