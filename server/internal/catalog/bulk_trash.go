package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

func (s *Service) bulkTrashStep(ctx context.Context, id string, access BulkAccess) (out BulkJob, err error) {
	var raw string
	var cursor, ordinal int64
	var entityID int64
	var item string
	if err = s.WithContext(ctx).read().QueryRow(`SELECT principal,cursor FROM personal_jobs WHERE id=?`, id).Scan(&raw, &cursor); err != nil {
		return out, err
	}
	var p identity.Principal
	if err = json.Unmarshal([]byte(raw), &p); err != nil {
		return out, err
	}
	err = s.WithContext(ctx).read().QueryRow(`SELECT m.ordinal,m.item_id,COALESCE(pid(i.public_id),'') FROM personal_job_items m LEFT JOIN catalog_entities i ON i.id=m.item_id WHERE m.job_id=? AND m.ordinal>? ORDER BY m.ordinal LIMIT 1`, id, cursor).Scan(&ordinal, &entityID, &item)
	if errors.Is(err, sql.ErrNoRows) {
		ordinal = cursor
		entityID = 0
		item = ""
	} else if err != nil {
		return out, err
	}
	authorize := func(ctx context.Context, tx *sql.Tx) error {
		if e := s.authorizeBulkCommand(ctx, tx, p, "trash", JobArguments{}, false); e != nil {
			return e
		}
		visible, bound, e := access(ctx, tx, p, "i.id")
		if e != nil {
			return e
		}
		if entityID != 0 {
			var allowed bool
			bound = append(bound, entityID)
			if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities i WHERE (`+visible+`) AND i.id=? AND i.retired=0)`, bound...).Scan(&allowed); e != nil {
				return e
			}
			if !allowed {
				return &JobItemError{Code: "not_found"}
			}
		}
		var live int64
		var state string
		if e = tx.QueryRowContext(ctx, `SELECT cursor,state FROM personal_jobs WHERE id=?`, id).Scan(&live, &state); e != nil {
			return e
		}
		if live != cursor || state != "queued" && state != "running" {
			return ErrPersonalConflict
		}
		return nil
	}
	complete := func(tx *sql.Tx) error {
		return s.finishTrashMember(ctx, tx, p, id, cursor, ordinal, entityID, item, "")
	}
	if entityID == 0 {
		err = dbwork.WithWriteTx(ctx, s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
			if e := authorize(ctx, tx); e != nil {
				return e
			}
			return complete(tx)
		})
	} else if item == "" {
		err = dbwork.WithWriteTx(ctx, s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
			if e := authorize(ctx, tx); e != nil {
				return e
			}
			return s.finishTrashMember(ctx, tx, p, id, cursor, ordinal, entityID, item, "not_found")
		})
	} else {
		if s.BulkCommands.Trash == nil {
			return out, errors.New("trash worker unavailable")
		}
		err = s.BulkCommands.Trash(ctx, p, id+"_"+strconv.FormatInt(ordinal, 10), item, authorize, complete)
	}
	if err != nil {
		if ctx.Err() != nil || dbwork.Retryable(err) {
			return out, err
		}
		// Another worker/ambiguous commit may already have advanced the cursor.
		var current int64
		if e := s.WithContext(ctx).read().QueryRow(`SELECT cursor FROM personal_jobs WHERE id=?`, id).Scan(&current); e != nil {
			return out, e
		}
		if current >= ordinal && item != "" {
			return s.WithContext(ctx).BulkJob(identity.PersonalKey(p.Viewer), id)
		}
		var failure *JobItemError
		switch {
		case errors.Is(err, identity.ErrUnauthorized), errors.Is(err, identity.ErrContentRestricted):
			return s.failBulkJob(ctx, id, "authority_revoked")
		case errors.As(err, &failure):
			err = dbwork.WithWriteTx(ctx, s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
				return s.finishTrashMember(ctx, tx, p, id, cursor, ordinal, entityID, item, failure.Code)
			})
		default:
			return out, err
		}
	}
	if err != nil {
		return out, err
	}
	return s.WithContext(ctx).BulkJob(identity.PersonalKey(p.Viewer), id)
}
func (s *Service) finishTrashMember(ctx context.Context, tx *sql.Tx, p identity.Principal, id string, cursor, ordinal, entityID int64, item, code string) error {
	var current int64
	if e := tx.QueryRowContext(ctx, `SELECT cursor FROM personal_jobs WHERE id=?`, id).Scan(&current); e != nil {
		return e
	}
	if current != cursor {
		return ErrPersonalConflict
	}
	observeBulkBatch(ctx, "apply", map[bool]int{true: 1, false: 0}[entityID != 0])
	done, failed := 0, 0
	if entityID != 0 {
		if code == "" {
			done = 1
		} else {
			failed = 1
			if _, e := tx.ExecContext(ctx, `INSERT INTO personal_job_failures VALUES(?,?,?,?)`, id, ordinal, entityID, code); e != nil {
				return e
			}
		}
	}
	if _, e := tx.ExecContext(ctx, `UPDATE personal_jobs SET cursor=?,done=done+?,failed=failed+?,revision=revision+1,updated_ms=?,state='running' WHERE id=?`, ordinal, done, failed, time.Now().UnixMilli(), id); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `UPDATE personal_jobs SET state=CASE WHEN failed=0 THEN 'complete' WHEN done=0 THEN 'failed' ELSE 'partial' END WHERE id=? AND done+failed=total`, id); e != nil {
		return e
	}
	return bulkEvent(tx, p, id)
}
