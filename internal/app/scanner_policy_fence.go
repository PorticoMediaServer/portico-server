package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

// scanProfileField is the single list of executable settings that can change
// scanner or analysis authorization. The runtime stores the contract's
// option.enabled projection as flat booleans; impact/dependency metadata is
// static contract data, not a second mutable settings shape.
var scanProfileField = map[string]bool{
	"analysisTier":                    true,
	"readLocalMetadata":               true,
	"readExternalSubtitlesAndLyrics":  true,
	"discoverLocalArtwork":            true,
	"fetchDescriptiveMetadata":        true,
	"probeStreams":                    true,
	"readEmbeddedTags":                true,
	"readEmbeddedIndexes":             true,
	"generateRepresentativeThumbnail": true,
	"extractSelectedEmbeddedAssets":   true,
	"validateSeekBehavior":            true,
	"fullFileChecksum":                true,
	"generateTrickplay":               true,
	"generateChapterThumbnails":       true,
	"generateWaveforms":               true,
	"analyzeLoudness":                 true,
	"sonicFingerprinting":             true,
	"detectSegments":                  true,
	"extractAllEmbeddedAttachments":   true,
	"analyzeSTRMTarget":               true,
}

var scannerReadCapability = map[string]bool{
	"readLocalMetadata":              true,
	"readExternalSubtitlesAndLyrics": true,
	"discoverLocalArtwork":           true,
	"fetchDescriptiveMetadata":       true,
	"readEmbeddedTags":               true,
}

var analysisCapability = map[string]bool{
	"probeStreams":                    true,
	"readEmbeddedTags":                true,
	"readEmbeddedIndexes":             true,
	"generateRepresentativeThumbnail": true,
	"extractSelectedEmbeddedAssets":   true,
	"validateSeekBehavior":            true,
	"fullFileChecksum":                true,
	"generateTrickplay":               true,
	"generateChapterThumbnails":       true,
	"generateWaveforms":               true,
	"analyzeLoudness":                 true,
	"sonicFingerprinting":             true,
	"detectSegments":                  true,
	"extractAllEmbeddedAttachments":   true,
	"analyzeSTRMTarget":               true,
}

type scanProfileChange struct {
	Before  map[string]bool
	After   map[string]bool
	Added   map[string]bool
	Removed map[string]bool
}

func effectiveScanProfile(settings map[string]any) map[string]bool {
	result := map[string]bool{}
	tier := normalizeAnalysisTier(settingString(settings, "analysisTier", analysisTierBasic))
	switch tier {
	case analysisTierFileListOnly:
		return result
	case analysisTierCustom:
		for field := range scanProfileField {
			if field != "analysisTier" && settingBool(settings, field, false) {
				result[field] = true
			}
		}
		return result
	case analysisTierComplete:
		for field := range scanProfileField {
			if field != "analysisTier" {
				result[field] = true
			}
		}
		return result
	default: // Basic is a fixed, bounded profile rather than Custom defaults.
		for _, field := range []string{
			"readLocalMetadata", "readExternalSubtitlesAndLyrics", "discoverLocalArtwork",
			"fetchDescriptiveMetadata", "probeStreams", "readEmbeddedTags",
			"readEmbeddedIndexes", "generateRepresentativeThumbnail", "validateSeekBehavior",
		} {
			result[field] = true
		}
		return result
	}
}

func compareScanProfiles(before, after map[string]any) scanProfileChange {
	change := scanProfileChange{
		Before: effectiveScanProfile(before), After: effectiveScanProfile(after),
		Added: map[string]bool{}, Removed: map[string]bool{},
	}
	for field := range change.Before {
		if !change.After[field] {
			change.Removed[field] = true
		}
	}
	for field := range change.After {
		if !change.Before[field] {
			change.Added[field] = true
		}
	}
	return change
}

func (change scanProfileChange) changed() bool {
	return len(change.Added) > 0 || len(change.Removed) > 0
}

func capabilitiesIntersect(values, class map[string]bool) bool {
	for field := range values {
		if class[field] {
			return true
		}
	}
	return false
}

func scanProfileRevision(capabilities map[string]bool) string {
	fields := make([]string, 0, len(capabilities))
	for field := range capabilities {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	digest := sha256.Sum256([]byte(strings.Join(fields, "\x00")))
	return "scan-profile-v1:" + hex.EncodeToString(digest[:])
}

func mergeScanProfileSettings(base, overrides map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

func decodeScanProfileSettings(raw any) map[string]any {
	switch value := raw.(type) {
	case map[string]any:
		return cloneSettingMap(value)
	case json.RawMessage:
		var decoded map[string]any
		_ = json.Unmarshal(value, &decoded)
		return decoded
	case []byte:
		var decoded map[string]any
		_ = json.Unmarshal(value, &decoded)
		return decoded
	case string:
		var decoded map[string]any
		_ = json.Unmarshal([]byte(value), &decoded)
		return decoded
	default:
		return map[string]any{}
	}
}

type scanProfileFollowup struct {
	LibraryID         string
	ProfileRevision   string
	ReconcileContent  bool
	ReconcileAnalysis bool
	Metadata          bool
}

func followupForScanProfileChange(libraryID string, change scanProfileChange) scanProfileFollowup {
	followup := scanProfileFollowup{LibraryID: strings.TrimSpace(libraryID), ProfileRevision: scanProfileRevision(change.After)}
	// New source-read or analysis permission must be applied to already indexed
	// media. A source-read downgrade also replaces a cancelled in-flight scan
	// with work that resolves the new profile.
	followup.ReconcileContent = capabilitiesIntersect(change.Added, scannerReadCapability)
	followup.ReconcileAnalysis = capabilitiesIntersect(change.Added, analysisCapability) ||
		capabilitiesIntersect(change.Removed, analysisCapability) && capabilitiesIntersect(change.After, analysisCapability)
	// Provider fetch is an independent metadata job. Enabling it fills missing
	// fields for the existing catalogue; disabling it only fences scan-origin
	// work and never cancels an owner-requested/scheduled metadata refresh.
	followup.Metadata = change.Added["fetchDescriptiveMetadata"]
	return followup
}

func (s *Server) enqueueScanProfileFollowup(followup scanProfileFollowup) {
	if followup.LibraryID == "" || !followup.ReconcileContent && !followup.ReconcileAnalysis && !followup.Metadata {
		return
	}
	library, err := s.getLibrary(followup.LibraryID)
	if err != nil {
		s.log.Warn("scan profile follow-up library lookup failed", "library", followup.LibraryID, "error", err)
		return
	}
	if followup.ReconcileContent || followup.ReconcileAnalysis {
		metadata := map[string]string{
			"mode": "reconcile", scanTriggerMetadataKey: "profile-change",
			"profileRevision": followup.ProfileRevision,
			"profileContent":  fmt.Sprintf("%t", followup.ReconcileContent),
			"profileAnalysis": fmt.Sprintf("%t", followup.ReconcileAnalysis),
		}
		if _, err := s.createJobForWithMetadata("library_scan", "Scan-profile stage reconciliation queued for "+library.Name+".", "library", library.ID, metadata); err != nil {
			s.log.Warn("scan profile stage reconciliation queue failed", "library", library.ID, "error", err)
		}
	}
	if followup.Metadata {
		metadata := map[string]string{
			"libraryId": library.ID, "libraryName": library.Name,
			"subtaskScope": "profile_change", "profileRevision": followup.ProfileRevision,
			"refreshIntent": string(metadataRefreshFillMissing),
		}
		if _, err := s.createJobForWithMetadata("metadata_refresh_library", "Metadata supplementation queued for "+library.Name+".", "library", library.ID, metadata); err != nil {
			s.log.Warn("scan profile metadata follow-up queue failed", "library", library.ID, "error", err)
		}
	}
}

func (s *Server) reconcileScanProfileStages(ctx context.Context, library Library, metadata map[string]string) (int, int, error) {
	settings := s.libraryAnalysisSettingsFor(library)
	currentRevision := scanProfileRevision(effectiveScanProfile(settings))
	if expected := strings.TrimSpace(metadata["profileRevision"]); expected == "" || expected != currentRevision {
		return 0, 0, nil
	}
	contentIndexed := 0
	if strings.EqualFold(metadata["profileContent"], "true") {
		indexed, err := s.reconcileScanProfileContent(ctx, library, currentRevision)
		if err != nil {
			return 0, 0, err
		}
		contentIndexed = indexed
	}
	analysisQueued := 0
	if strings.EqualFold(metadata["profileAnalysis"], "true") && capabilitiesIntersect(effectiveScanProfile(settings), analysisCapability) {
		queued, err := s.reconcileScanProfileAnalysis(ctx, library, currentRevision)
		if err != nil {
			return contentIndexed, 0, err
		}
		analysisQueued = queued
	}
	return contentIndexed, analysisQueued, nil
}

// reconcileScanProfileAnalysis schedules current-revision analysis directly
// from committed inventory. It never walks source directories or repeats
// unchanged inventory merely because an owner enabled a new analysis stage.
func (s *Server) reconcileScanProfileAnalysis(ctx context.Context, library Library, profileRevision string) (int, error) {
	lastMediaID, queued := "", 0
	capabilities := effectiveScanProfile(s.libraryAnalysisSettingsFor(library))
	for {
		rows, err := s.queryBackgroundRead(ctx, `
			SELECT media.id, media.title, file.id, file.path, file.size_bytes, file.mod_time,
				CASE WHEN file.identity_evidence LIKE 'scanner:v2:%' THEN substr(file.identity_evidence,12) ELSE '' END,
				file.source_type
			FROM media_items media
			JOIN media_files file ON file.media_id=media.id AND file.available=1
			WHERE media.library_id=? AND media.id>?
				AND NOT EXISTS (
					SELECT 1 FROM media_files preferred
					WHERE preferred.media_id=file.media_id AND preferred.available=1 AND (
						preferred.quality_rank>file.quality_rank OR
						(preferred.quality_rank=file.quality_rank AND preferred.size_bytes>file.size_bytes) OR
						(preferred.quality_rank=file.quality_rank AND preferred.size_bytes=file.size_bytes AND preferred.path<file.path)
					)
				)
			ORDER BY media.id LIMIT 100`, library.ID, lastMediaID)
		if err != nil {
			return queued, err
		}
		type candidate struct {
			file  scannerMediaFile
			title string
		}
		candidates := []candidate{}
		for rows.Next() {
			var candidate candidate
			if err := rows.Scan(&candidate.file.ID, &candidate.title, &candidate.file.FileID, &candidate.file.SourcePath,
				&candidate.file.FileSize, &candidate.file.FileModTime, &candidate.file.QuickSignature, &candidate.file.SourceType); err != nil {
				rows.Close()
				return queued, err
			}
			candidate.file.LibraryID = library.ID
			candidates = append(candidates, candidate)
			lastMediaID = candidate.file.ID
		}
		if err := rows.Close(); err != nil {
			return queued, err
		}
		if len(candidates) == 0 {
			return queued, nil
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		err = s.withBackgroundTxTagged(ctx, []string{"jobs"}, func(tx *sql.Tx) error {
			for _, candidate := range candidates {
				if !scannerAnalysisEligibleForProfile(candidate.file, capabilities) {
					continue
				}
				metadata := representativeFrameAnalysisMetadata()
				metadata["sourceRevision"] = scannerAnalysisSourceRevision(candidate.file)
				metadata["profileRevision"] = profileRevision
				job := Job{
					ID: randomID("job"), Type: "media_analyze", Status: "queued",
					Message:      "Scan-profile analysis queued for " + candidate.title + ".",
					ResourceType: "media", ResourceID: candidate.file.ID, Metadata: metadata,
					Priority: foundationcontract.WorkClassBackgroundMedia, Phase: "queued", CreatedAt: now, UpdatedAt: now,
				}
				job.ActiveKey = jobActiveKeyFor(job.Type, job.ResourceType, job.ResourceID, metadata)
				inserted, _, err := insertScannerBacklogJobTx(tx, job)
				if err != nil {
					return err
				}
				if inserted {
					queued++
				}
			}
			return nil
		})
		if err != nil {
			return queued, err
		}
		if len(candidates) < 100 {
			return queued, nil
		}
	}
}

// reconcileScanProfileContent revisits committed local file inventory without
// a directory walk. Only the newly authorized bounded scanner enrichments run;
// remote objects remain inventory-derived and analysis-owned.
func (s *Server) reconcileScanProfileContent(ctx context.Context, library Library, profileRevision string) (int, error) {
	roots, err := resolvedLibraryRoots(library.Paths)
	if err != nil {
		return 0, err
	}
	initialProfile, err := s.resolveLibraryScanProfile(library.ID)
	if err != nil {
		return 0, err
	}
	if scanProfileRevision(initialProfile.Capabilities) != profileRevision {
		return 0, nil
	}
	metadataSettings := s.metadataAgentSettings()
	lastPath, indexed := "", 0
	for {
		rows, err := s.queryBackgroundRead(ctx, `
			SELECT path, CASE WHEN identity_evidence LIKE 'scanner:v2:%' THEN substr(identity_evidence,12) ELSE '' END
			FROM media_files
			WHERE library_id=? AND available=1 AND path>? AND source_type NOT IN ('rclone','webdav')
			ORDER BY path LIMIT ?`, library.ID, lastPath, scannerWriteBatchSize)
		if err != nil {
			return indexed, err
		}
		type inventoryFile struct{ path, quickSignature string }
		files := []inventoryFile{}
		for rows.Next() {
			var file inventoryFile
			if err := rows.Scan(&file.path, &file.quickSignature); err != nil {
				rows.Close()
				return indexed, err
			}
			files = append(files, file)
			lastPath = file.path
		}
		if err := rows.Close(); err != nil {
			return indexed, err
		}
		if len(files) == 0 {
			return indexed, nil
		}
		batch := []scannerMediaFile{}
		authorizedReads := map[string]bool{}
		for _, inventory := range files {
			if !s.waitForForegroundPressureToEase(ctx) {
				return indexed, ctx.Err()
			}
			root, ok := storageRootForPath(roots, inventory.path)
			if !ok {
				continue
			}
			var file scannerMediaFile
			err := s.boundedStorageIO(ctx, storageRequestForRoot(root, "scan-profile content reconciliation"), func() error {
				if err := ctx.Err(); err != nil {
					return err
				}
				live, err := s.resolveLibraryScanProfile(library.ID)
				if err != nil {
					return err
				}
				if scanProfileRemovedReadPermission(initialProfile.Capabilities, live.Capabilities) {
					return context.Canceled
				}
				policy := live.Content
				if !scanContentPolicyOpensObjects(policy) {
					return nil
				}
				for field := range live.Capabilities {
					if scannerReadCapability[field] {
						authorizedReads[field] = true
					}
				}
				file = scannerFileForPath(library, root.real, inventory.path, policy.ReadLocalMetadata && metadataSettings.LocalNFO, policy.ProbeStreams)
				file.QuickSignature = inventory.quickSignature
				file.ReadExternalSidecars = policy.ReadExternalSidecars
				file.DiscoverLocalArtwork = policy.DiscoverLocalArtwork
				if library.Type == "music" {
					custom := metadataSettings
					custom.EmbeddedTags = metadataSettings.EmbeddedTags && policy.ReadEmbeddedTags
					file = s.enrichScannedMusicFileWithSettings(ctx, file, library, custom)
				}
				return nil
			})
			if err != nil {
				return indexed, err
			}
			if strings.TrimSpace(file.SourcePath) != "" {
				batch = append(batch, expandMultiEpisodeScannedFile(file)...)
			}
		}
		if len(batch) > 0 {
			publishProfile, err := s.resolveLibraryScanProfile(library.ID)
			if err != nil {
				return indexed, err
			}
			if scanProfileRemovedReadPermission(authorizedReads, publishProfile.Capabilities) {
				return indexed, context.Canceled
			}
			_, written, _, _, err := s.writeScannedMediaBatch(ctx, library, batch, time.Now().UTC().Format(time.RFC3339), profileRevision, false, false)
			if err != nil {
				return indexed, err
			}
			indexed += written
		}
		if len(files) < scannerWriteBatchSize {
			return indexed, nil
		}
	}
}

func (s *Server) enqueueScanProfileFollowups(followups []scanProfileFollowup) {
	for _, followup := range followups {
		s.enqueueScanProfileFollowup(followup)
	}
}

// fenceJobsTx marks queued/deferred work terminal and running work
// cancellation-requested inside the same transaction as the policy change.
// The caller signals returned in-process contexts only after commit.
func (s *Server) fenceJobsTx(tx *sql.Tx, predicate, message string, args ...any) ([]string, error) {
	rows, err := tx.Query(`SELECT id FROM jobs WHERE status='running' AND (`+predicate+`) ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	running := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		running = append(running, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339Nano)
	terminalArgs := []any{message, now, s.jobRetentionDeadline(nowTime), now}
	terminalArgs = append(terminalArgs, args...)
	if _, err := tx.Exec(`
		UPDATE jobs
		SET status='cancelled', phase='cancelled', progress=100, progress_current=100,
			retry_eligible=0, error_code='cancelled', message=?,
			worker_acknowledged_at=?, retention_until=CASE WHEN retention_until='' THEN ? ELSE retention_until END,
			leased_by='', lease_expires_at='', next_run_at='', deferred_until='', updated_at=?
		WHERE status IN ('queued','deferred') AND (`+predicate+`)`, terminalArgs...); err != nil {
		return nil, err
	}
	runningArgs := []any{now, message, now}
	runningArgs = append(runningArgs, args...)
	if _, err := tx.Exec(`
		UPDATE jobs
		SET phase='cancelling', cancellation_requested_at=?, message=?, updated_at=?
		WHERE status='running' AND (`+predicate+`)`, runningArgs...); err != nil {
		return nil, err
	}
	return running, nil
}

func appendUniqueJobIDs(target []string, values ...string) []string {
	seen := map[string]bool{}
	for _, value := range target {
		seen[value] = true
	}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !seen[value] {
			seen[value] = true
			target = append(target, value)
		}
	}
	return target
}

func (s *Server) fenceContentWorkForScanProfileTx(tx *sql.Tx, libraryID string, change scanProfileChange) ([]string, error) {
	if tx == nil {
		return nil, sql.ErrTxDone
	}
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil, fmt.Errorf("scan-profile fence requires one library")
	}
	running := []string{}
	if capabilitiesIntersect(change.Removed, scannerReadCapability) {
		ids, err := s.fenceJobsTx(tx,
			`type='library_scan' AND resource_type='library' AND resource_id=?`,
			"Job cancelled because its scan profile removed source-read permission.", libraryID)
		if err != nil {
			return nil, err
		}
		running = appendUniqueJobIDs(running, ids...)
	}
	if capabilitiesIntersect(change.Removed, analysisCapability) {
		ids, err := s.fenceJobsTx(tx,
			`type='media_analyze' AND resource_type='media' AND EXISTS (SELECT 1 FROM media_items media WHERE media.id=jobs.resource_id AND media.library_id=?)`,
			"Job cancelled because its scan profile removed media-analysis permission.", libraryID)
		if err != nil {
			return nil, err
		}
		running = appendUniqueJobIDs(running, ids...)
		if _, err := tx.Exec(`
			UPDATE scanner_backlog SET status='complete', last_error='Disabled by a scan-profile change.', updated_at=?
			WHERE library_id=? AND kind='analysis' AND status='queued'`, time.Now().UTC().Format(time.RFC3339Nano), libraryID); err != nil {
			return nil, err
		}
	}
	if change.Removed["fetchDescriptiveMetadata"] {
		ids, err := s.fenceJobsTx(tx, `(
			(type='metadata_refresh_library' AND resource_type='library' AND resource_id=?
			 AND json_extract(CASE WHEN json_valid(metadata_json) THEN metadata_json ELSE '{}' END,'$.subtaskScope')='scan_discoveries')
			OR
			(type='metadata_refresh' AND resource_type='media'
			 AND json_extract(CASE WHEN json_valid(metadata_json) THEN metadata_json ELSE '{}' END,'$.subtaskScope')='scan_discoveries'
			 AND EXISTS (SELECT 1 FROM media_items media WHERE media.id=jobs.resource_id AND media.library_id=?))
		)`, "Scan-origin metadata work cancelled because provider fetching was disabled.", libraryID, libraryID)
		if err != nil {
			return nil, err
		}
		running = appendUniqueJobIDs(running, ids...)
		if _, err := tx.Exec(`
			UPDATE scanner_backlog SET status='complete', last_error='Provider fetching disabled by a scan-profile change.', updated_at=?
			WHERE library_id=? AND kind='metadata' AND status='queued'`, time.Now().UTC().Format(time.RFC3339Nano), libraryID); err != nil {
			return nil, err
		}
	}
	return running, nil
}

func (s *Server) fenceGlobalScanProfileChangeTx(tx *sql.Tx, before, after map[string]any) ([]string, []scanProfileFollowup, error) {
	rows, err := tx.Query(`SELECT id, COALESCE(settings_json,'{}') FROM libraries ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	type libraryProfile struct{ id, raw string }
	libraries := []libraryProfile{}
	for rows.Next() {
		var library libraryProfile
		if err := rows.Scan(&library.id, &library.raw); err != nil {
			rows.Close()
			return nil, nil, err
		}
		libraries = append(libraries, library)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	running, followups := []string{}, []scanProfileFollowup{}
	for _, library := range libraries {
		overrides := decodeScanProfileSettings(library.raw)
		change := compareScanProfiles(mergeScanProfileSettings(before, overrides), mergeScanProfileSettings(after, overrides))
		if !change.changed() {
			continue
		}
		ids, err := s.fenceContentWorkForScanProfileTx(tx, library.id, change)
		if err != nil {
			return nil, nil, err
		}
		running = appendUniqueJobIDs(running, ids...)
		followups = append(followups, followupForScanProfileChange(library.id, change))
	}
	return running, followups, nil
}

func (s *Server) cancelRunningJobContexts(jobIDs []string) {
	for _, jobID := range jobIDs {
		s.jobCancelMu.Lock()
		cancel := s.jobCancels[jobID]
		s.jobCancelMu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
}

func (s *Server) fenceRemoteAnalysisJobsTx(tx *sql.Tx, sourceID string, change scanProfileChange) ([]string, error) {
	if tx == nil {
		return nil, sql.ErrTxDone
	}
	if !capabilitiesIntersect(change.Removed, analysisCapability) {
		return nil, nil
	}
	prefix := "portico-storage://" + strings.TrimSpace(sourceID) + "/"
	running, err := s.fenceJobsTx(tx,
		`type='media_analyze' AND resource_type='media' AND EXISTS (
			SELECT 1 FROM media_files file WHERE file.media_id=jobs.resource_id AND substr(file.path,1,length(?))=?
		)`, "Remote analysis cancelled because its source scan profile removed permission.", prefix, prefix)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`
		UPDATE scanner_backlog SET status='complete', last_error='Disabled by a remote scan-profile change.', updated_at=?
		WHERE kind='analysis' AND status='queued' AND EXISTS (
			SELECT 1 FROM media_files file WHERE file.media_id=scanner_backlog.media_id AND substr(file.path,1,length(?))=?
		)`, time.Now().UTC().Format(time.RFC3339Nano), prefix, prefix); err != nil {
		return nil, err
	}
	return running, nil
}
