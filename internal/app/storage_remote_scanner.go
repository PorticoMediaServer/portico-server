package app

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

func (s *Server) remoteLibraryRootEvidence(ctx context.Context, libraryID string) ([]scanRootEvidence, error) {
	rows, err := s.queryBackgroundRead(ctx, `SELECT id,configured_path,backend_kind,health_state,error_class,error_message FROM storage_sources WHERE library_id=? AND backend_kind IN ('rclone','webdav') ORDER BY configured_path,id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []scanRootEvidence
	for rows.Next() {
		var item scanRootEvidence
		var kind string
		if err := rows.Scan(&item.SourceID, &item.ConfiguredPath, &kind, &item.Health, &item.ErrorClass, &item.ErrorMessage); err != nil {
			return nil, err
		}
		item.ResolvedPath = "portico-storage://" + item.SourceID
		item.Classification = storageSourceNetwork
		item.ClassificationSource = "owner"
		result = append(result, item)
	}
	return result, rows.Err()
}

func remoteStorageLocator(sourceID, objectPath string) string {
	return (&url.URL{Scheme: "portico-storage", Host: sourceID, Path: "/" + strings.TrimPrefix(objectPath, "/")}).String()
}

func (s *Server) scanRemoteStorageSources(ctx context.Context, library Library, run libraryScanRun, generation, now string) (libraryScanResult, error) {
	var result libraryScanResult
	rows, err := s.queryBackgroundRead(ctx, `SELECT id,backend_kind,analysis_mode FROM storage_sources WHERE library_id=? AND backend_kind IN ('rclone','webdav') ORDER BY id`, library.ID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	type source struct{ id, kind, analysisMode string }
	var sources []source
	for rows.Next() {
		var item source
		if err := rows.Scan(&item.id, &item.kind, &item.analysisMode); err != nil {
			return result, err
		}
		sources = append(sources, item)
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	analysisSettings := s.libraryAnalysisSettingsFor(library)
	for _, source := range sources {
		contentPolicy := scanContentPolicy(source.analysisMode, analysisSettings)
		sourceSettings := mergeScanProfileSettings(analysisSettings, map[string]any{"analysisTier": source.analysisMode})
		enqueueAnalysis := capabilitiesIntersect(effectiveScanProfile(sourceSettings), analysisCapability)
		root := scanRoot{sourceID: source.id, configured: "portico-storage://" + source.id, display: "Remote storage", real: "portico-storage://" + source.id, classification: storageSourceNetwork}
		_ = s.updateScanRootEvidence(ctx, run, root, "running", "", "", 0, 0)
		backend, err := s.remoteBackendForSource(ctx, source.id)
		if err != nil {
			result.DegradedRoots++
			_ = s.updateScanRootEvidence(ctx, run, root, "degraded", storageErrorClass(err), boundedStorageError(err), 0, 0)
			continue
		}
		inventory, err := s.inventoryRemoteSource(ctx, source.id, backend, rcloneInventoryPageLimit)
		if err != nil {
			result.DegradedRoots++
			_ = s.updateScanRootEvidence(ctx, run, root, "degraded", storageErrorClass(err), boundedStorageError(err), 0, 0)
			continue
		}
		objectQuery := `SELECT object_path,revision,size_bytes,mod_time FROM storage_remote_objects WHERE source_id=? AND missing_since=''`
		objectArgs := []any{source.id}
		if !inventory.Authoritative {
			// Delta inventories touch only objects returned by the provider. Keep
			// catalog work O(changes), rather than rewriting a million-row source
			// after every sync-token report.
			objectQuery += ` AND last_seen_generation=?`
			objectArgs = append(objectArgs, inventory.Generation)
		}
		objectQuery += ` ORDER BY object_path`
		objectRows, err := s.queryBackgroundRead(ctx, objectQuery, objectArgs...)
		if err != nil {
			return result, err
		}
		batch := make([]scannerMediaFile, 0, scannerWriteBatchSize)
		files := 0
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			// Inventory-only is a strict zero-content-read scan. Basic, Complete,
			// and explicitly enabled Custom work are queued only after the catalog
			// page commits; the remote background lane yields to playback.
			_, indexed, metadata, analysis, err := s.writeScannedMediaBatch(ctx, library, batch, now, generation, contentPolicy.FetchDescriptiveMetadata, enqueueAnalysis)
			result.FilesIndexed += indexed
			result.MetadataRefreshQueued += metadata
			result.AnalysisQueued += analysis
			batch = batch[:0]
			return err
		}
		for objectRows.Next() {
			var objectPath, revision, modTime string
			var size int64
			if err := objectRows.Scan(&objectPath, &revision, &size, &modTime); err != nil {
				objectRows.Close()
				return result, err
			}
			// PC-LIBRARY-SCANNING defines STRM as a first-class local descriptor.
			// A descriptor inside WebDAV/rclone is neither remote media nor a local
			// descriptor that can be revalidated safely at playback.
			if isSTRMDescriptor(objectPath) || !isMediaFileForLibrary(library.Type, objectPath) {
				result.FilesSkipped++
				continue
			}
			virtualPath := string(filepath.Separator) + filepath.FromSlash(objectPath)
			file := scannerFileForPath(library, string(filepath.Separator), virtualPath, false, false)
			file.SourcePath = remoteStorageLocator(source.id, objectPath)
			file.DisplayPath = objectPath
			file.SourceType = source.kind
			file.FileSize = size
			file.FileModTime = modTime
			file.ContentFingerprint = "remote:" + revision
			file.QuickSignature = revision
			file.FileID = scannerFileID(file.ID, file.SourcePath)
			// Remote inventory is authoritative for discovery. Local sidecar and
			// artwork filesystem probes are inapplicable to opaque object locators
			// and would violate inventory-only's zero-content-read contract.
			file.FilesystemPrepared = true
			batch = append(batch, file)
			files++
			if len(batch) >= scannerWriteBatchSize {
				if err := flush(); err != nil {
					objectRows.Close()
					return result, err
				}
			}
		}
		if err := objectRows.Close(); err != nil {
			return result, err
		}
		if err := flush(); err != nil {
			return result, err
		}
		result.FilesUnchanged += max(0, inventory.ObjectsSeen-inventory.ObjectsChanged)
		marked, err := s.markRemoteMissingAfterGrace(ctx, library.ID, source.id)
		if err != nil {
			return result, err
		}
		result.MissingMarked += marked
		_ = s.updateScanRootEvidence(ctx, run, root, "healthy", "", "", 0, files)
	}
	return result, nil
}

func (s *Server) markRemoteMissingAfterGrace(ctx context.Context, libraryID, sourceID string) (int, error) {
	var grace int
	if err := s.queryBackgroundRow(ctx, `SELECT missing_grace_seconds FROM storage_sources WHERE id=?`, sourceID).Scan(&grace); err != nil {
		return 0, err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(grace) * time.Second).Format(time.RFC3339Nano)
	marked := 0
	lastPath := ""
	for {
		rows, err := s.queryBackgroundRead(ctx, `SELECT object_path FROM storage_remote_objects WHERE source_id=? AND missing_since<>'' AND missing_since<=? AND object_path>? ORDER BY object_path LIMIT 10000`, sourceID, cutoff, lastPath)
		if err != nil {
			return marked, err
		}
		var paths []string
		for rows.Next() {
			var objectPath string
			if err := rows.Scan(&objectPath); err != nil {
				rows.Close()
				return marked, err
			}
			lastPath = objectPath
			paths = append(paths, remoteStorageLocator(sourceID, objectPath))
		}
		if err := rows.Close(); err != nil {
			return marked, err
		}
		if len(paths) == 0 {
			break
		}
		err = s.withBackgroundTxTagged(ctx, []string{"media", "library-items"}, func(tx *sql.Tx) error {
			stamp := time.Now().UTC().Format(time.RFC3339Nano)
			for _, p := range paths {
				result, e := tx.Exec(`UPDATE media_files SET available=0,missing_since=CASE WHEN missing_since='' THEN ? ELSE missing_since END WHERE library_id=? AND path=? AND source_type IN ('rclone','webdav') AND available=1`, stamp, libraryID, p)
				if e != nil {
					return e
				}
				count, _ := result.RowsAffected()
				marked += int(count)
			}
			return nil
		})
		if err != nil {
			return marked, err
		}
	}
	err := s.withBackgroundTxTagged(ctx, []string{"media", "library-items"}, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE media_items SET source_url=COALESCE((SELECT path FROM media_files WHERE media_id=media_items.id AND available=1 ORDER BY quality_rank DESC,size_bytes DESC,path LIMIT 1),'') WHERE library_id=? AND EXISTS(SELECT 1 FROM media_files f WHERE f.media_id=media_items.id AND f.source_type IN ('rclone','webdav'))`, libraryID)
		return err
	})
	return marked, err
}

func mergeLibraryScanResult(target *libraryScanResult, source libraryScanResult) {
	target.FilesIndexed += source.FilesIndexed
	target.FilesUnchanged += source.FilesUnchanged
	target.FilesSkipped += source.FilesSkipped
	target.MissingMarked += source.MissingMarked
	target.MetadataRefreshQueued += source.MetadataRefreshQueued
	target.AnalysisQueued += source.AnalysisQueued
	target.DegradedRoots += source.DegradedRoots
}
