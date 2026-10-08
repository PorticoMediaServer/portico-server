package administration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/compactcatalog"
	"sort"
	"strconv"
	"strings"
)

// MaxDeleteTargets bounds one bulk delete. A larger sweep is a scan-driven
// operation, not a page action.
const MaxDeleteTargets = 200

// DeleteFile is one file a delete would move or remove.
type DeleteFile struct {
	AssetID string `json:"assetId"`
	Path    string `json:"path"`
	Bytes   int64  `json:"bytes"`
	// Present is false when the file is already gone from storage; the catalog
	// row is still removed.
	Present bool `json:"present"`
	// Shared is true when another item also uses this file, so the delete keeps
	// the bytes and removes only this item's claim on them.
	Shared bool `json:"shared"`
}

// Dependent is something that refers to the item and will change when it goes.
type Dependent struct {
	Kind        string `json:"kind"`
	Count       int64  `json:"count"`
	Description string `json:"description"`
}

// DeleteTarget is one item's share of a preview.
type DeleteTarget struct {
	ItemID             string       `json:"itemId"`
	Title              string       `json:"title"`
	Kind               string       `json:"kind"`
	LibraryID          string       `json:"libraryId"`
	LibraryName        string       `json:"libraryName"`
	AllowMediaDeletion bool         `json:"allowMediaDeletion"`
	TrashRetentionDays int          `json:"trashRetentionDays"`
	Files              []DeleteFile `json:"files"`
	Bytes              int64        `json:"bytes"`
	Dependents         []Dependent  `json:"dependents"`
	// entityID is the catalogue integer the SQL statements use. It never
	// leaves the server: public ids stay the API's only identifier.
	entityID int64
}

// DeletePreview is what the delete dialog renders before anything is touched.
type DeletePreview struct {
	Revision int64          `json:"revision"`
	Targets  []DeleteTarget `json:"targets"`
	// Confirmation is exactly what the owner must type back: one item's title,
	// or the target count for a bulk delete.
	Confirmation     string   `json:"confirmation"`
	ConfirmationKind string   `json:"confirmationKind"`
	TotalBytes       int64    `json:"totalBytes"`
	TotalFiles       int      `json:"totalFiles"`
	TotalBytesText   string   `json:"totalBytesText"`
	Blocked          []string `json:"blocked"`
	// TrashRetentionDays is the shortest retention among the targets, which is
	// how long the bytes are recoverable for.
	TrashRetentionDays int `json:"trashRetentionDays"`
}

// DeleteRequest is the delete form. deleteFiles false removes the catalog
// entries and leaves the bytes in place; the next scan readopts them.
type DeleteRequest struct {
	ItemIDs          []string `json:"itemIds"`
	DeleteFiles      bool     `json:"deleteFiles"`
	Confirmation     string   `json:"confirmation"`
	ExpectedRevision int64    `json:"expectedRevision"`
	OperationID      string   `json:"operationId"`
}

// DeleteReceipt is one item's outcome.
type DeleteReceipt struct {
	ItemID       string `json:"itemId"`
	Title        string `json:"title"`
	Removed      bool   `json:"removed"`
	TrashEntryID string `json:"trashEntryId,omitempty"`
	FilesMoved   int    `json:"filesMoved"`
	FilesKept    int    `json:"filesKept"`
	Bytes        int64  `json:"bytes"`
	Code         string `json:"code,omitempty"`
}

// DeleteResult is the whole operation's outcome.
type DeleteResult struct {
	Revision   int64           `json:"revision"`
	Receipts   []DeleteReceipt `json:"receipts"`
	Removed    int             `json:"removed"`
	Failed     int             `json:"failed"`
	BytesMoved int64           `json:"bytesTrashed"`
	// FilesRetained is true when the caller asked to keep the files, so the page
	// can say the bytes are still on disk.
	FilesRetained bool `json:"filesRetained"`
}

func deleteFence(ctx context.Context, tx *sql.Tx, libraries []string) (int64, error) {
	if len(libraries) == 0 {
		return 0, nil
	}
	unique := map[string]bool{}
	ordered := []string{}
	for _, id := range libraries {
		if !unique[id] {
			unique[id] = true
			ordered = append(ordered, id)
		}
	}
	sort.Strings(ordered)
	var total int64
	for _, id := range ordered {
		var revision int64
		err := tx.QueryRowContext(ctx, `SELECT revision FROM library_revisions WHERE library_id=?`, id).Scan(&revision)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		total += revision
	}
	return total, nil
}

func tableExists(ctx context.Context, tx *sql.Tx, name string) bool {
	var found string
	err := tx.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&found)
	return err == nil
}

// dependents counts what refers to an item. Each probe is guarded by a table
// check so a build without one of these features still previews correctly.
func dependents(ctx context.Context, tx *sql.Tx, item string) ([]Dependent, error) {
	out := []Dependent{}
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return out, nil
		}
		return nil, err
	}
	probes := []struct{ table, kind, query, description string }{
		{"catalog_collection_members", "collection", `SELECT COUNT(*) FROM catalog_collection_members WHERE item_id=?`, "collections that list this item"},
		{"catalog_playlist_entries", "playlist", `SELECT COUNT(*) FROM catalog_playlist_entries WHERE item_id=?`, "playlist entries that point at this item"},
		{"saved_resource_entries", "saved", `SELECT COUNT(*) FROM saved_resource_entries WHERE item_id=?`, "saved resources that list this item"},
		{"progress", "progress", `SELECT COUNT(*) FROM progress WHERE item_id=?`, "profiles with saved playback position"},
		{"lc_entries", "library-channel", `SELECT COUNT(*) FROM lc_entries WHERE item_id=?`, "scheduled library channel slots"},
		{"dvr_recordings", "recording", `SELECT COUNT(*) FROM dvr_recordings WHERE item_id=? AND state!='deleted'`, "recordings published as this item"},
	}
	for _, probe := range probes {
		if !tableExists(ctx, tx, probe.table) {
			continue
		}
		var count int64
		if err := tx.QueryRowContext(ctx, probe.query, id).Scan(&count); err != nil {
			// A schema shape this build does not share is not a preview failure.
			continue
		}
		if count > 0 {
			out = append(out, Dependent{probe.kind, count, probe.description})
		}
	}
	return out, nil
}

func targetFor(ctx context.Context, tx *sql.Tx, item string) (DeleteTarget, error) {
	return targetFactsFor(ctx, tx, item, true)
}

func targetFactsFor(ctx context.Context, tx *sql.Tx, item string, statFiles bool) (DeleteTarget, error) {
	t := DeleteTarget{ItemID: item, Files: []DeleteFile{}, Dependents: []Dependent{}}
	err := tx.QueryRowContext(ctx, `SELECT e.id,e.title,k.name,cl.library_id,l.name FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind JOIN catalog_libraries cl ON cl.id=e.library_id JOIN libraries l ON l.id=cl.library_id WHERE e.public_id=pid_blob(?)`, item).Scan(&t.entityID, &t.Title, &t.Kind, &t.LibraryID, &t.LibraryName)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	if t.AllowMediaDeletion, t.TrashRetentionDays, err = deletionPolicy(ctx, tx, t.LibraryID); err != nil {
		return t, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.token,a.path,a.size,(EXISTS(SELECT 1 FROM catalog_asset_links o WHERE o.asset_id=a.id AND o.entity_id<>al.entity_id) OR EXISTS(SELECT 1 FROM item_extras x WHERE x.asset_id=a.token AND x.item_id<>e.id) OR EXISTS(SELECT 1 FROM playback_sessions p WHERE p.asset_id=a.token AND p.item_id<>e.id) OR EXISTS(SELECT 1 FROM lyric_resources r WHERE r.asset_id=a.token AND r.item_id<>e.id) OR EXISTS(SELECT 1 FROM lyric_candidates c WHERE c.asset_id=a.token AND c.item_id<>e.id) OR EXISTS(SELECT 1 FROM subtitle_resources r WHERE r.source_id=a.token AND r.item_id<>e.id) OR EXISTS(SELECT 1 FROM dvr_catalog_provenance p WHERE p.asset_id=a.token AND p.item_id<>e.id)) FROM catalog_entities e JOIN catalog_asset_links al ON al.entity_id=e.id JOIN catalog_assets a ON a.id=al.asset_id WHERE e.public_id=pid_blob(?) ORDER BY al.part_index,a.id`, item)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	for rows.Next() {
		var f DeleteFile
		var others int
		if err = rows.Scan(&f.AssetID, &f.Path, &f.Bytes, &others); err != nil {
			return t, err
		}
		f.Shared = others > 0
		if statFiles {
			if info, statErr := os.Stat(f.Path); statErr == nil && !info.IsDir() {
				f.Present = true
				f.Bytes = info.Size()
			}
		}
		if !f.Shared {
			t.Bytes += f.Bytes
		}
		t.Files = append(t.Files, f)
	}
	if err = rows.Err(); err != nil {
		return t, err
	}
	t.Dependents, err = dependents(ctx, tx, item)
	return t, err
}

func validTargets(ids []string) error {
	if len(ids) == 0 || len(ids) > MaxDeleteTargets {
		return ErrInput
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || len(id) > 128 || seen[id] || !safeText(id, 128) {
			return ErrInput
		}
		seen[id] = true
	}
	return nil
}

// PreviewDelete lists exactly what a delete would take. It never writes.
func (s *Service) PreviewDelete(ctx context.Context, auth Authorize, ids []string) (DeletePreview, error) {
	out := DeletePreview{Targets: []DeleteTarget{}, Blocked: []string{}}
	if err := validTargets(ids); err != nil {
		return out, err
	}
	err := s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		libraries := []string{}
		retention := -1
		for _, id := range ids {
			target, err := targetFor(ctx, tx, id)
			if err != nil {
				return err
			}
			if !target.AllowMediaDeletion {
				out.Blocked = append(out.Blocked, target.LibraryID)
			}
			if retention < 0 || target.TrashRetentionDays < retention {
				retention = target.TrashRetentionDays
			}
			libraries = append(libraries, target.LibraryID)
			out.TotalBytes += target.Bytes
			out.TotalFiles += len(target.Files)
			out.Targets = append(out.Targets, target)
		}
		if retention < 0 {
			retention = 0
		}
		out.TrashRetentionDays = retention
		fence, err := deleteFence(ctx, tx, libraries)
		out.Revision = fence
		return err
	})
	if err != nil {
		return out, err
	}
	out.TotalBytesText = formatBytes(out.TotalBytes)
	if len(out.Targets) == 1 {
		out.Confirmation, out.ConfirmationKind = out.Targets[0].Title, "title"
	} else {
		out.Confirmation, out.ConfirmationKind = strconv.Itoa(len(out.Targets)), "count"
	}
	// Duplicate library identifiers in the blocked list help nobody.
	seen, blocked := map[string]bool{}, []string{}
	for _, id := range out.Blocked {
		if !seen[id] {
			seen[id] = true
			blocked = append(blocked, id)
		}
	}
	sort.Strings(blocked)
	out.Blocked = blocked
	return out, nil
}

type trashedFile struct {
	Path    string `json:"path"`
	Stored  string `json:"stored"`
	Bytes   int64  `json:"bytes"`
	AssetID string `json:"assetId"`
}

// trashRoot is where trashed bytes live. It is inside the server state
// directory so it is on the same volume as nothing in particular: a move that
// crosses a volume falls back to a copy.
func (s *Service) trashRoot() (string, error) {
	if s.StateDirectory() == "" {
		return "", ErrUnavailable
	}
	root := filepath.Join(s.StateDirectory(), "trash")
	if err := durableMkdirAll(root, 0700); err != nil {
		return "", ErrUnavailable
	}
	return root, nil
}

// moveFile renames when it can and copies when the rename crosses a filesystem,
// which is the normal case for a library on a separate volume.
func moveFile(from, to string) error { return moveFileContext(context.Background(), from, to) }

// Delete removes catalog entries for the named items and, when asked, moves
// their files into the trash. Every removal is fenced on the preview's revision
// and on the typed-back confirmation.
func (s *Service) Delete(ctx context.Context, auth Authorize, actor string, request DeleteRequest) (DeleteResult, error) {
	out := DeleteResult{Receipts: []DeleteReceipt{}, FilesRetained: !request.DeleteFiles}
	if err := validTargets(request.ItemIDs); err != nil {
		return out, err
	}
	if !validOperationID(request.OperationID) || !safeText(request.Confirmation, 400) {
		return out, ErrInput
	}
	if s.StateDirectory() == "" {
		return out, ErrUnavailable
	}
	s.files.Lock()
	defer s.files.Unlock()
	scope := "media-delete"
	digest := digestOf(request)
	if request.OperationID == "" {
		var err error
		request.OperationID, err = identifier()
		if err != nil {
			return out, err
		}
	}
	if err := s.recoverFileOperations(ctx); err != nil {
		return out, err
	}
	journal := s.fileJournal(scope, request.OperationID, digest)
	err := s.transaction(ctx, auth, func(tx *sql.Tx) error {
		stored, replayed, err := receipt[DeleteResult](ctx, tx, scope, request.OperationID, digest)
		if err != nil {
			return err
		}
		if replayed {
			out = stored
			return nil
		}
		targets := make([]DeleteTarget, 0, len(request.ItemIDs))
		libraries := []string{}
		for _, id := range request.ItemIDs {
			target, err := targetFor(ctx, tx, id)
			if err != nil {
				return err
			}
			if !target.AllowMediaDeletion {
				return ErrDenied
			}
			libraries = append(libraries, target.LibraryID)
			targets = append(targets, target)
		}
		fence, err := deleteFence(ctx, tx, libraries)
		if err != nil {
			return err
		}
		if fence != request.ExpectedRevision {
			return ErrConflict
		}
		expected := strconv.Itoa(len(targets))
		if len(targets) == 1 {
			expected = targets[0].Title
		}
		if request.Confirmation != expected {
			return ErrConfirmation
		}
		at := s.milliseconds()
		for _, target := range targets {
			result, err := s.removeTarget(ctx, tx, actor, target, request, at, journal)
			if err != nil {
				return err
			}
			out.Receipts = append(out.Receipts, result)
			if result.Removed {
				out.Removed++
				out.BytesMoved += result.Bytes
			} else {
				out.Failed++
			}
		}
		if out.Revision, err = deleteFence(ctx, tx, libraries); err != nil {
			return err
		}
		return saveReceipt(ctx, tx, scope, request.OperationID, digest, out, at)
	})
	return out, s.settleFileOperation(journal, err)
}

func (s *Service) removeTarget(ctx context.Context, tx *sql.Tx, actor string, target DeleteTarget, request DeleteRequest, at int64, journal *fileOperation) (DeleteReceipt, error) {
	out := DeleteReceipt{ItemID: target.ItemID, Title: target.Title}
	var held bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admin_trash WHERE item_id=? AND state='held')`, target.entityID).Scan(&held); err != nil {
		return out, err
	}
	if held {
		return out, ErrConflict
	}
	entryID, err := identifier()
	if err != nil {
		return out, err
	}
	moved := []trashedFile{}
	if request.DeleteFiles {
		root, err := s.trashRoot()
		if err != nil {
			return out, err
		}
		root = filepath.Join(root, entryID)
		for index, file := range target.Files {
			if file.Shared {
				out.FilesKept++
				continue
			}
			if !file.Present {
				continue
			}
			stored := fmt.Sprintf("%03d-%s", index, filepath.Base(file.Path))
			if err = journal.move(ctx, file.Path, filepath.Join(root, stored)); err != nil {
				return out, err
			}
			moved = append(moved, trashedFile{Path: file.Path, Stored: stored, Bytes: file.Bytes, AssetID: file.AssetID})
			out.FilesMoved++
			out.Bytes += file.Bytes
		}
		if target.TrashRetentionDays > 0 {
			// Retain the original catalog identity and every metadata/artwork reference,
			// even for an already-missing item or one whose bytes are shared.
			raw, _ := json.Marshal(moved)
			expires := at + int64(target.TrashRetentionDays)*86400000
			if _, err = tx.ExecContext(ctx, `INSERT INTO admin_trash VALUES(?,?,?,?,?,?,?,?,?,?,'held',?,?)`, entryID, target.LibraryID, target.entityID, target.Title, target.Kind, string(raw), len(moved), out.Bytes, at, expires, actor, request.OperationID); err != nil {
				return out, err
			}
			out.TrashEntryID = entryID
			for _, file := range target.Files {
				if !file.Shared {
					assetID, err := compactcatalog.AssetByTokenTx(ctx, tx, file.AssetID)
					if err != nil {
						return out, err
					}
					if assetID != 0 {
						if err = compactcatalog.SetAssetAvailableTx(ctx, tx, assetID, false); err != nil {
							return out, err
						}
					}
					if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET state='trashed',trashed_at=? WHERE asset_id=? AND source_id IN(SELECT id FROM library_sources WHERE library_id=?)`, timeFromMilliseconds(at), file.AssetID, target.LibraryID); err != nil {
						return out, err
					}
				}
			}
		} else {
			// Even zero-retention deletion first moves to a reversible private stage.
			// Permanent unlink is forbidden until the catalog/receipt transaction commits.
			if err = catalog.PurgeItemTx(ctx, tx, target.ItemID); err != nil {
				return out, err
			}
			if err = journal.removeAfterCommit(root); err != nil {
				return out, err
			}
		}
	} else if err = catalog.PurgeItemTx(ctx, tx, target.ItemID); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE library_revisions SET revision=revision+1 WHERE library_id=?`, target.LibraryID); err != nil {
		return out, err
	}
	out.Removed = true
	return out, nil
}

func assetIDs(target DeleteTarget) string {
	ids := make([]string, 0, len(target.Files))
	for _, f := range target.Files {
		ids = append(ids, f.AssetID)
	}
	raw, _ := json.Marshal(ids)
	return string(raw)
}

// TrashEntry is one held deletion.
type TrashEntry struct {
	ID        string              `json:"id"`
	LibraryID string              `json:"libraryId"`
	ItemID    string              `json:"itemId"`
	Title     string              `json:"title"`
	Kind      string              `json:"kind"`
	FileCount int                 `json:"fileCount"`
	Bytes     int64               `json:"bytes"`
	BytesText string              `json:"bytesText"`
	TrashedAt string              `json:"trashedAt"`
	ExpiresAt string              `json:"expiresAt"`
	State     string              `json:"state"`
	Expired   bool                `json:"expired"`
	Files     []trashedFilePublic `json:"files"`
}

type trashedFilePublic struct {
	Path  string `json:"originalPath"`
	Bytes int64  `json:"bytes"`
}

// TrashPage is one page of the trash listing.
type TrashPage struct {
	Items      []TrashEntry `json:"items"`
	NextCursor string       `json:"nextCursor"`
	HeldBytes  int64        `json:"heldBytes"`
	HeldCount  int64        `json:"heldCount"`
}

func millisecondsToRFC3339(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return timeFromMilliseconds(ms)
}

// Trash pages held deletions newest first.
func (s *Service) Trash(ctx context.Context, auth Authorize, state, token string, limit int) (TrashPage, error) {
	out := TrashPage{Items: []TrashEntry{}}
	size, err := pageSize(limit)
	if err != nil {
		return out, err
	}
	if state == "" {
		state = "held"
	}
	if !oneOf(state, "held", "restored", "purged", "all") {
		return out, ErrInput
	}
	c, err := decodeCursor("trash", token)
	if err != nil {
		return out, err
	}
	now := s.milliseconds()
	err = s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(bytes),0) FROM admin_trash WHERE state='held'`).Scan(&out.HeldCount, &out.HeldBytes); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT t.id,t.library_id,COALESCE(pid(e.public_id),''),t.title,t.kind,t.files_json,t.file_count,t.bytes,t.trashed_ms,t.expires_ms,t.state FROM admin_trash t LEFT JOIN catalog_entities e ON e.id=t.item_id WHERE (?='all' OR t.state=?) AND (?=0 OR t.trashed_ms<? OR (t.trashed_ms=? AND t.id<?)) ORDER BY t.trashed_ms DESC,t.id DESC LIMIT ?`, state, state, c.Order, c.Order, c.Order, c.ID, size+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		orders := []int64{}
		for rows.Next() {
			var e TrashEntry
			var files string
			var trashed, expires int64
			if err = rows.Scan(&e.ID, &e.LibraryID, &e.ItemID, &e.Title, &e.Kind, &files, &e.FileCount, &e.Bytes, &trashed, &expires, &e.State); err != nil {
				return err
			}
			var stored []trashedFile
			_ = json.Unmarshal([]byte(files), &stored)
			e.Files = make([]trashedFilePublic, 0, len(stored))
			for _, f := range stored {
				e.Files = append(e.Files, trashedFilePublic{f.Path, f.Bytes})
			}
			e.BytesText = formatBytes(e.Bytes)
			e.TrashedAt, e.ExpiresAt = millisecondsToRFC3339(trashed), millisecondsToRFC3339(expires)
			e.Expired = e.State == "held" && expires > 0 && expires <= now
			out.Items = append(out.Items, e)
			orders = append(orders, trashed)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		if len(out.Items) > size {
			out.Items = out.Items[:size]
			out.NextCursor = encodeCursor("trash", orders[size-1], out.Items[size-1].ID)
		}
		return nil
	})
	return out, err
}

// RestoreResult reports what a restore put back.
type RestoreResult struct {
	EntryID   string   `json:"entryId"`
	Restored  int      `json:"restored"`
	Skipped   int      `json:"skipped"`
	Conflicts []string `json:"conflicts"`
	// Reindex says the files are back on disk but the catalog rows are not: the
	// next scan of the library readopts them.
	Reindex bool `json:"reindexRequired"`
}

// RestoreFromTrash puts an entry's files back where they came from. A path that
// is occupied again is reported rather than overwritten.
func (s *Service) RestoreFromTrash(ctx context.Context, auth Authorize, id, operationID string) (RestoreResult, error) {
	out := RestoreResult{EntryID: id, Conflicts: []string{}, Reindex: false}
	if !validOperationID(operationID) || !safeText(id, 128) || id == "" {
		return out, ErrInput
	}
	root, err := s.trashRoot()
	if err != nil {
		return out, err
	}
	s.files.Lock()
	defer s.files.Unlock()
	scope := "trash-restore"
	digest := digestOf([]string{id, operationID})
	if operationID == "" {
		operationID, err = identifier()
		if err != nil {
			return out, err
		}
	}
	if err = s.recoverFileOperations(ctx); err != nil {
		return out, err
	}
	journal := s.fileJournal(scope, operationID, digest)
	err = s.transaction(ctx, auth, func(tx *sql.Tx) error {
		stored, replayed, err := receipt[RestoreResult](ctx, tx, scope, operationID, digest)
		if err != nil {
			return err
		}
		if replayed {
			out = stored
			return nil
		}
		var files, state, library string
		var item int64
		if err = tx.QueryRowContext(ctx, `SELECT files_json,state,library_id,item_id FROM admin_trash WHERE id=?`, id).Scan(&files, &state, &library, &item); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if state != "held" {
			return ErrConflict
		}
		var entries []trashedFile
		if json.Unmarshal([]byte(files), &entries) != nil {
			return ErrUnavailable
		}
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities WHERE id=?)`, item).Scan(&exists); err != nil {
			return err
		}
		out.Reindex = !exists // Legacy trash from before catalog retention still needs a scan.
		for _, f := range entries {
			source := filepath.Join(root, id, f.Stored)
			if !safeTrashName(id) || !safeTrashName(f.Stored) {
				return ErrUnavailable
			}
			// Existing originals are conflicts, never an overwrite or a claimed restore.
			if _, err := os.Lstat(f.Path); err == nil {
				return ErrConflict
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err = journal.move(ctx, source, f.Path); err != nil {
				return err
			}
			assetID, err := compactcatalog.AssetByTokenTx(ctx, tx, f.AssetID)
			if err != nil {
				return err
			}
			if assetID != 0 {
				if err = compactcatalog.SetAssetAvailableTx(ctx, tx, assetID, true); err != nil {
					return err
				}
			}
			if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET state='available',trashed_at='',missing_since='',missing_observations=0 WHERE asset_id=? AND source_id IN(SELECT id FROM library_sources WHERE library_id=?) AND retired=0`, f.AssetID, library); err != nil {
				return err
			}
			out.Restored++
		}
		at := s.milliseconds()
		if out.Skipped == 0 {
			if err = journal.removeAfterCommit(filepath.Join(root, id)); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE admin_trash SET state='restored' WHERE id=?`, id); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE library_revisions SET revision=revision+1 WHERE library_id=?`, library); err != nil {
			return err
		}
		return saveReceipt(ctx, tx, scope, operationID, digest, out, at)
	})
	return out, s.settleFileOperation(journal, err)
}

// EmptyTrashRequest empties the trash. expiredOnly keeps entries whose retention
// has not run out; confirmation is the number of entries being purged.
type EmptyTrashRequest struct {
	ExpiredOnly  bool   `json:"expiredOnly"`
	Confirmation string `json:"confirmation"`
	OperationID  string `json:"operationId"`
}

// EmptyTrashResult reports the purge.
type EmptyTrashResult struct {
	Purged     int    `json:"purged"`
	BytesFreed int64  `json:"bytesFreed"`
	BytesText  string `json:"bytesFreedText"`
	Remaining  int64  `json:"remaining"`
	Incomplete int    `json:"incomplete"`
}

// EmptyTrash permanently removes held entries. Nothing here is recoverable
// afterwards, which is why it takes a typed-back count.
func (s *Service) EmptyTrash(ctx context.Context, auth Authorize, request EmptyTrashRequest) (EmptyTrashResult, error) {
	out := EmptyTrashResult{}
	if !validOperationID(request.OperationID) || !safeText(request.Confirmation, 40) {
		return out, ErrInput
	}
	root, err := s.trashRoot()
	if err != nil {
		return out, err
	}
	s.files.Lock()
	defer s.files.Unlock()
	scope := "trash-empty"
	digest := digestOf(request)
	if request.OperationID == "" {
		request.OperationID, err = identifier()
		if err != nil {
			return out, err
		}
	}
	if err = s.recoverFileOperations(ctx); err != nil {
		return out, err
	}
	journal := s.fileJournal(scope, request.OperationID, digest)
	now := s.milliseconds()
	err = s.transaction(ctx, auth, func(tx *sql.Tx) error {
		stored, replayed, err := receipt[EmptyTrashResult](ctx, tx, scope, request.OperationID, digest)
		if err != nil {
			return err
		}
		if replayed {
			out = stored
			return nil
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,bytes FROM admin_trash WHERE state='held' AND (?=0 OR (expires_ms>0 AND expires_ms<=?)) ORDER BY trashed_ms,id`, boolToInt(request.ExpiredOnly), now)
		if err != nil {
			return err
		}
		ids, bytes := []string{}, int64(0)
		for rows.Next() {
			var id string
			var size int64
			if err = rows.Scan(&id, &size); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
			bytes += size
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if request.Confirmation != strconv.Itoa(len(ids)) {
			return ErrConfirmation
		}
		for _, id := range ids {
			if !safeTrashName(id) {
				return ErrUnavailable
			}
			var item sql.NullString
			if err = tx.QueryRowContext(ctx, `SELECT pid(e.public_id) FROM admin_trash t LEFT JOIN catalog_entities e ON e.id=t.item_id WHERE t.id=?`, id).Scan(&item); err != nil {
				return err
			}
			if item.Valid {
				if err = catalog.PurgeItemTx(ctx, tx, item.String); err != nil {
					return err
				}
			}
			if err = journal.removeAfterCommit(filepath.Join(root, id)); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE admin_trash SET state='purged',files_json='[]',bytes=0,file_count=0 WHERE id=?`, id); err != nil {
				return err
			}
			out.Purged++
		}
		out.BytesFreed = bytes
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_trash WHERE state='held'`).Scan(&out.Remaining); err != nil {
			return err
		}
		out.BytesText = formatBytes(out.BytesFreed)
		return saveReceipt(ctx, tx, scope, request.OperationID, digest, out, now)
	})
	return out, s.settleFileOperation(journal, err)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// trashBytes measures what the trash currently holds, for the storage page.
func (s *Service) trashBytes(ctx context.Context, tx *sql.Tx) (int64, int64, error) {
	var count, bytes int64
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(bytes),0) FROM admin_trash WHERE state='held'`).Scan(&count, &bytes)
	return count, bytes, err
}

func safeTrashName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, "/\\")
}
