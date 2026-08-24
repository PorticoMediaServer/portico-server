package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type storageInventoryResult struct {
	RunID, Generation, SyncToken string
	ObjectsSeen, ObjectsChanged  int
	Authoritative                bool
}

// inventoryRemoteSource is restart-safe at page boundaries. Object revisions
// are committed before cursors/tokens, so a crash can only replay an idempotent
// page. Absence is applied only after a complete authoritative full listing;
// delta sync and partial/error responses can never make media disappear.
func (s *Server) inventoryRemoteSource(ctx context.Context, sourceID string, backend remoteStorageBackend, pageLimit int) (storageInventoryResult, error) {
	return s.inventoryRemoteSourceWithCursorRecovery(ctx, sourceID, backend, pageLimit, true)
}

func (s *Server) inventoryRemoteSourceWithCursorRecovery(ctx context.Context, sourceID string, backend remoteStorageBackend, pageLimit int, allowCursorReset bool) (storageInventoryResult, error) {
	if backend == nil || strings.TrimSpace(sourceID) == "" {
		return storageInventoryResult{}, errors.New("remote storage source and backend are required")
	}
	now := time.Now().UTC()
	runID, generation, cursor, syncToken, err := s.beginOrResumeStorageInventory(ctx, sourceID, backend.Kind(), now)
	if err != nil {
		return storageInventoryResult{}, err
	}
	result := storageInventoryResult{RunID: runID, Generation: generation, SyncToken: syncToken}
	for {
		page, pageErr := backend.Inventory(ctx, firstNonEmpty(cursor, syncToken), pageLimit)
		if pageErr != nil {
			if allowCursorReset && errors.Is(pageErr, errRemoteInventoryCursorInvalid) {
				if resetErr := s.resetStorageInventoryCursor(ctx, runID, pageErr); resetErr != nil {
					return result, errors.Join(pageErr, resetErr)
				}
				return s.inventoryRemoteSourceWithCursorRecovery(ctx, sourceID, backend, pageLimit, false)
			}
			_ = s.failStorageInventory(context.Background(), runID, pageErr)
			return result, pageErr
		}
		changed, err := s.applyStorageInventoryPage(ctx, sourceID, runID, generation, page)
		if err != nil {
			_ = s.failStorageInventory(context.Background(), runID, err)
			return result, err
		}
		result.ObjectsSeen += len(page.Objects)
		result.ObjectsChanged += changed
		result.SyncToken = page.SyncToken
		result.Authoritative = page.Authoritative
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if err := s.completeStorageInventory(ctx, sourceID, runID, generation, result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Server) resetStorageInventoryCursor(ctx context.Context, runID string, cause error) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.execBackgroundWrite(ctx, `UPDATE storage_inventory_runs SET status='failed',cursor='',last_error=?,completed_at=?,updated_at=? WHERE id=? AND status IN ('running','degraded')`, boundedStorageError(cause), now, now, runID)
	return err
}

func (s *Server) beginOrResumeStorageInventory(ctx context.Context, sourceID, kind string, now time.Time) (runID, generation, cursor, syncToken string, err error) {
	err = s.queryBackgroundRow(ctx, `SELECT r.id,r.scan_generation,r.cursor,COALESCE(s.inventory_sync_token,'') FROM storage_inventory_runs r JOIN storage_sources s ON s.id=r.source_id WHERE r.source_id=? AND r.status IN ('running','degraded') ORDER BY r.started_at DESC LIMIT 1`, sourceID).Scan(&runID, &generation, &cursor, &syncToken)
	if err == nil {
		_, err = s.execBackgroundWrite(ctx, `UPDATE storage_inventory_runs SET status='running',last_error='',updated_at=? WHERE id=? AND status='degraded'`, now.Format(time.RFC3339Nano), runID)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", "", "", "", err
	}
	runID = randomID("inventory")
	generation = randomID("generation")
	stamp := now.Format(time.RFC3339Nano)
	err = s.withBackgroundTxTagged(ctx, []string{"storage_sources", "storage_inventory_runs"}, func(tx *sql.Tx) error {
		var storedKind string
		if e := tx.QueryRow(`SELECT backend_kind,inventory_sync_token FROM storage_sources WHERE id=?`, sourceID).Scan(&storedKind, &syncToken); e != nil {
			return e
		}
		if storedKind != kind {
			return fmt.Errorf("storage backend kind mismatch: catalog=%s runtime=%s", storedKind, kind)
		}
		_, e := tx.Exec(`INSERT INTO storage_inventory_runs(id,source_id,scan_generation,status,started_at,updated_at) VALUES(?,?,?,'running',?,?)`, runID, sourceID, generation, stamp, stamp)
		return e
	})
	return
}

func (s *Server) applyStorageInventoryPage(ctx context.Context, sourceID, runID, generation string, page storageInventoryPage) (changed int, err error) {
	if err := validateStorageInventoryPage(page); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withBackgroundTxTagged(ctx, []string{"storage_remote_objects", "storage_inventory_runs"}, func(tx *sql.Tx) error {
		for _, object := range page.Objects {
			p, e := normalizeRemoteObjectPath(object.Path)
			if e != nil {
				return e
			}
			var prior string
			e = tx.QueryRow(`SELECT revision FROM storage_remote_objects WHERE source_id=? AND object_path=?`, sourceID, p).Scan(&prior)
			if errors.Is(e, sql.ErrNoRows) || prior != object.Revision {
				changed++
			} else if e != nil {
				return e
			}
			size := object.Size
			if size < 0 {
				size = 0
			}
			_, e = tx.Exec(`INSERT INTO storage_remote_objects(source_id,object_path,object_id,revision,etag,content_hash,content_type,size_bytes,mod_time,first_seen_generation,last_seen_generation,missing_since,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,'',?) ON CONFLICT(source_id,object_path) DO UPDATE SET object_id=excluded.object_id,revision=excluded.revision,etag=excluded.etag,content_hash=excluded.content_hash,content_type=excluded.content_type,size_bytes=excluded.size_bytes,mod_time=excluded.mod_time,last_seen_generation=excluded.last_seen_generation,missing_since='',updated_at=excluded.updated_at`, sourceID, p, object.ObjectID, object.Revision, object.ETag, object.Hash, object.ContentType, size, object.ModTime.UTC().Format(time.RFC3339Nano), generation, generation, now)
			if e != nil {
				return e
			}
		}
		for _, deleted := range page.DeletedPaths {
			p, e := normalizeRemoteObjectPath(deleted)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(`UPDATE storage_remote_objects SET missing_since=CASE WHEN missing_since='' THEN ? ELSE missing_since END,updated_at=? WHERE source_id=? AND object_path=?`, now, now, sourceID, p); e != nil {
				return e
			}
		}
		_, e := tx.Exec(`UPDATE storage_inventory_runs SET cursor=?,sync_token=?,objects_seen=objects_seen+?,objects_changed=objects_changed+?,updated_at=? WHERE id=? AND status IN ('running','degraded')`, page.NextCursor, page.SyncToken, len(page.Objects), changed, now, runID)
		return e
	})
	return
}

func (s *Server) completeStorageInventory(ctx context.Context, sourceID, runID, generation string, result storageInventoryResult) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withBackgroundTxTagged(ctx, []string{"storage_sources", "storage_remote_objects", "storage_inventory_runs"}, func(tx *sql.Tx) error {
		if result.Authoritative {
			if _, e := tx.Exec(`UPDATE storage_remote_objects SET missing_since=CASE WHEN missing_since='' THEN ? ELSE missing_since END,updated_at=? WHERE source_id=? AND last_seen_generation<>?`, now, now, sourceID, generation); e != nil {
				return e
			}
		}
		if _, e := tx.Exec(`UPDATE storage_inventory_runs SET status='healthy',last_error='',absence_authoritative=?,completed_at=?,updated_at=? WHERE id=? AND status IN ('running','degraded')`, boolInt(result.Authoritative), now, now, runID); e != nil {
			return e
		}
		_, e := tx.Exec(`UPDATE storage_sources SET inventory_cursor='',inventory_sync_token=?,inventory_complete=?,last_success_at=?,updated_at=? WHERE id=?`, result.SyncToken, boolInt(result.Authoritative), now, now, sourceID)
		return e
	})
}
func (s *Server) failStorageInventory(ctx context.Context, runID string, cause error) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Preserve the committed page cursor and generation so a transient provider
	// failure resumes instead of restarting an expensive large-library walk.
	_, err := s.execBackgroundWrite(ctx, `UPDATE storage_inventory_runs SET status='degraded',last_error=?,completed_at='',updated_at=? WHERE id=? AND status='running'`, boundedStorageError(cause), now, runID)
	return err
}
