package administration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"portico.local/server/internal/compactcatalog"

	"portico.local/server/internal/dbwork"
)

var ErrTrashRecovery = errors.New("trash filesystem recovery requires attention")

func (s *Service) settleTrashJob(j *fileOperation, problem error) error {
	if e := s.settleFileOperation(j, nil); e != nil {
		return errors.Join(ErrTrashRecovery, problem, e)
	}
	return problem
}

func trashTargetFence(target DeleteTarget) string {
	target.Bytes = 0
	target.Dependents = nil
	target.Files = append([]DeleteFile(nil), target.Files...)
	for i := range target.Files {
		target.Files[i].Present = false
		target.Files[i].Bytes = 0
	}
	return digestOf(target)
}

// TrashJobItem moves one item into recoverable trash. The filesystem journal
// precedes every move; the trash row and caller's job cursor commit together.
// Emptying trash remains the separate, confirmed irreversible operation.
func (s *Service) TrashJobItem(ctx context.Context, auth Authorize, actor, operation, item string, complete func(*sql.Tx) error) error {
	if auth == nil || complete == nil || !validOperationID(operation) || s.StateDirectory() == "" {
		return ErrInput
	}
	s.files.Lock()
	defer s.files.Unlock()
	if e := s.recoverFileOperations(ctx); e != nil {
		return e
	}
	var target DeleteTarget
	e := s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		var e error
		target, e = targetFor(ctx, tx, item)
		if e != nil {
			return e
		}
		if !target.AllowMediaDeletion {
			return ErrDenied
		}
		var held bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_trash WHERE item_id=? AND state='held')`, target.entityID).Scan(&held); e != nil {
			return e
		}
		if held {
			return ErrConflict
		}
		return nil
	})
	if e != nil {
		return e
	}
	entry, e := identifier()
	if e != nil {
		return e
	}
	root, e := s.trashRoot()
	if e != nil {
		return e
	}
	root = filepath.Join(root, entry)
	fence := trashTargetFence(target)
	journal := s.fileJournal("job-trash", operation, fence)
	journal.TrashEntryID = entry
	moved := []trashedFile{}
	var bytes int64
	for index, file := range target.Files {
		if file.Shared || !file.Present {
			continue
		}
		stored := fmt.Sprintf("%03d-%s", index, filepath.Base(file.Path))
		if e = journal.move(ctx, file.Path, filepath.Join(root, stored)); e != nil {
			return s.settleTrashJob(journal, e)
		}
		moved = append(moved, trashedFile{Path: file.Path, Stored: stored, Bytes: file.Bytes, AssetID: file.AssetID})
		bytes += file.Bytes
	}
	// No filesystem I/O occurs inside this background transaction.
	e = dbwork.WithWriteTx(ctx, s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		if e := auth(ctx, tx); e != nil {
			return e
		}
		current, e := targetFactsFor(ctx, tx, item, false)
		if e != nil {
			return e
		}
		if trashTargetFence(current) != fence {
			return ErrConflict
		}
		var held bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_trash WHERE item_id=? AND state='held')`, current.entityID).Scan(&held); e != nil {
			return e
		}
		if held {
			return ErrConflict
		}
		at := s.milliseconds()
		expires := int64(0)
		if current.TrashRetentionDays > 0 {
			expires = at + int64(current.TrashRetentionDays)*86400000
		}
		raw, _ := json.Marshal(moved)
		if _, e = tx.ExecContext(ctx, `INSERT INTO admin_trash VALUES(?,?,?,?,?,?,?,?,?,?,'held',?,?)`, entry, target.LibraryID, target.entityID, target.Title, target.Kind, string(raw), len(moved), bytes, at, expires, actor, operation); e != nil {
			return e
		}
		for _, file := range target.Files {
			if !file.Shared {
				assetID, e := compactcatalog.AssetByTokenTx(ctx, tx, file.AssetID)
				if e != nil {
					return e
				}
				if assetID != 0 {
					if e = compactcatalog.SetAssetAvailableTx(ctx, tx, assetID, false); e != nil {
						return e
					}
				}
				if _, e = tx.ExecContext(ctx, `UPDATE inventory_objects SET state='trashed',trashed_at=? WHERE asset_id=? AND source_id IN(SELECT id FROM library_sources WHERE library_id=?)`, timeFromMilliseconds(at), file.AssetID, target.LibraryID); e != nil {
					return e
				}
			}
		}
		if _, e = tx.ExecContext(ctx, `UPDATE library_revisions SET revision=revision+1 WHERE library_id=?`, target.LibraryID); e != nil {
			return e
		}
		return complete(tx)
	})
	return s.settleTrashJob(journal, e)
}
