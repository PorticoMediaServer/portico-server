package catalog

import (
	"context"
	"database/sql"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

// A maintenance attempt keeps its captured source target and matching library
// revision across retries. New events belong to the following attempt.
func captureVisibilityTargetTx(ctx context.Context, tx *sql.Tx, classID int64, library string) (int64, int64, error) {
	var target, covered, revision int64
	err := tx.QueryRowContext(ctx, `SELECT source_target,captured_sequence,source_revision FROM compact_visibility_classes WHERE id=?`, classID).Scan(&target, &covered, &revision)
	if err != nil {
		return 0, 0, err
	}
	if target <= covered {
		err = tx.QueryRowContext(ctx, `SELECT s.sequence,COALESCE(r.revision,0) FROM compact_visibility_source_state s LEFT JOIN library_revisions r ON r.library_id=? WHERE s.id=1`, library).Scan(&target, &revision)
		if err != nil {
			return 0, 0, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE compact_visibility_classes SET source_target=?,source_revision=? WHERE id=?`, target, revision, classID)
	}
	return target, revision, err
}

func visibilityTargetReadyTx(ctx context.Context, tx *sql.Tx, classID, target int64) (bool, error) {
	ready, err := compactcatalog.VisibilitySourceTargetReadyTx(ctx, tx, target)
	if err != nil || !ready {
		return false, err
	}
	var pending bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM compact_visibility_dirty INDEXED BY compact_visibility_dirty_frontier WHERE class_id=? AND first_sequence<=?)`, classID, target).Scan(&pending)
	return !pending, err
}

func (s *Service) clearVisibilityDirtyThrough(ctx context.Context, classID, target int64) error {
	for {
		var n int64
		err := dbwork.WithWriteTx(ctx, s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
			r, err := tx.ExecContext(ctx, `DELETE FROM compact_visibility_dirty WHERE (class_id,entity_id) IN(SELECT class_id,entity_id FROM compact_visibility_dirty INDEXED BY compact_visibility_dirty_frontier WHERE class_id=? AND first_sequence<=? ORDER BY first_sequence,entity_id LIMIT ?)`, classID, target, compactcatalog.BatchLimit)
			if err != nil {
				return err
			}
			n, err = r.RowsAffected()
			return err
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if !dbwork.PaceBackground(ctx, time.Now()) {
			return ctx.Err()
		}
	}
}
