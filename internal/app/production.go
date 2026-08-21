package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
	"github.com/PorticoMediaServer/portico-server/internal/mediatoolchain"
)

const systemAPIVersion = "v1"

const maxPlaylistItemsResponse = 250

const readinessProbeTimeout = 750 * time.Millisecond

var (
	errDeviceNotTrusted     = errors.New("device is not trusted")
	errDeviceNotAllowed     = errors.New("device is not allowed")
	errActiveSessionLimit   = errors.New("active session limit reached")
	errPlaybackSessionLimit = errors.New("playback session limit reached")
	errAccessSchedule       = errors.New("access schedule blocked")
)

func (s *Server) handleSystemDiagnostics(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view diagnostics.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	writeJSON(w, http.StatusOK, s.systemDiagnostics())
}

func (s *Server) handleSystemStartup(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view startup state.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	writeJSON(w, http.StatusOK, s.startupDiagnostics())
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	readiness := s.readinessReport(r.Context())
	status := http.StatusOK
	if !readiness.Ready {
		status = http.StatusServiceUnavailable
		w.Header().Set("Retry-After", "2")
	}
	publicStatus := "ready"
	if !readiness.Ready {
		publicStatus = "unavailable"
	}
	writeJSON(w, status, map[string]any{
		"ready":  readiness.Ready,
		"status": publicStatus,
	})
}

func (s *Server) readinessReport(ctx context.Context) ReadinessResponse {
	startup := s.startupDiagnostics()
	webDistReady := pathExists(filepath.Join(s.cfg.WebDistDir, "index.html"))
	appDataReady := directoryWritable(s.cfg.AppDataDir)
	databaseFileReady := pathExists(s.cfg.DatabasePath)
	workloadLanes := s.workloadLaneDiagnostics()
	sqliteHealth := s.sqliteHealthSnapshot()
	databaseProbe := s.readinessProbe(ctx, func(probeCtx context.Context) error {
		db := s.dbHandle()
		if db == nil {
			return errors.New("sqlite handle is not available")
		}
		var ok int
		if err := db.QueryRowContext(probeCtx, `SELECT 1`).Scan(&ok); err != nil {
			return err
		}
		if ok != 1 {
			return fmt.Errorf("unexpected sqlite probe result %d", ok)
		}
		return nil
	})
	authBootstrapProbe := s.readinessProbe(ctx, func(probeCtx context.Context) error {
		db := s.dbHandle()
		if db == nil {
			return errors.New("sqlite handle is not available")
		}
		_, err := database.HasNoUsersContext(probeCtx, db)
		return err
	})
	backgroundDeferred := s.readinessBackgroundDeferred(workloadLanes, sqliteHealth)
	sqliteHealthReady := sqliteHealthAllowsBackground(sqliteHealth)
	databaseReady := databaseFileReady && databaseProbe.Ready && authBootstrapProbe.Ready && sqliteHealthReady
	sqliteStatus := "ready"
	if sqliteHealth.Status == sqliteHealthStatusCorrupt {
		sqliteStatus = sqliteHealthStatusCorrupt
	} else if !databaseReady {
		sqliteStatus = "degraded"
	}
	ready := startup.HTTPReady && startup.Status != "degraded" &&
		webDistReady && appDataReady && databaseReady
	status := "ready"
	signals := []string{}
	if !startup.HTTPReady {
		signals = append(signals, "http listener is not marked ready")
	}
	if startup.Status == "degraded" {
		signals = append(signals, "startup diagnostics are degraded")
	}
	if !webDistReady {
		signals = append(signals, "web dist index.html is missing")
	}
	if !appDataReady {
		signals = append(signals, "app data directory is not writable")
	}
	if !databaseFileReady {
		signals = append(signals, "database file is missing")
	}
	if !databaseProbe.Ready {
		signals = append(signals, "sqlite probe is not ready: "+databaseProbe.Status)
	}
	if !authBootstrapProbe.Ready {
		signals = append(signals, "auth bootstrap probe is not ready: "+authBootstrapProbe.Status)
	}
	if !sqliteHealthReady {
		signals = append(signals, "sqlite health watchdog is "+sqliteHealth.Status)
	}
	if backgroundDeferred {
		if !sqliteHealthReady {
			signals = append(signals, "background work is deferred by sqlite health watchdog")
		} else {
			signals = append(signals, "background work is deferred by foreground pressure")
		}
	}
	if !ready {
		status = "degraded"
	}
	if len(signals) == 0 {
		signals = append(signals, "runtime readiness checks passed")
	}
	return ReadinessResponse{
		Status:                 status,
		Ready:                  ready,
		HTTPReady:              startup.HTTPReady,
		WebDistReady:           webDistReady,
		AppDataReady:           appDataReady,
		DatabaseFileReady:      databaseFileReady,
		DatabaseReady:          databaseReady,
		BackgroundJobsDeferred: backgroundDeferred,
		Startup:                startup,
		DatabaseProbe:          databaseProbe,
		AuthBootstrapProbe:     authBootstrapProbe,
		SQLite:                 s.readinessSQLiteSnapshot(sqliteStatus),
		SQLiteHealth:           sqliteHealth,
		WorkloadLanes:          workloadLanes,
		Signals:                signals,
		GeneratedAt:            time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) readinessProbe(ctx context.Context, fn func(context.Context) error) ReadinessProbe {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	probeCtx, cancel := context.WithTimeout(ctx, readinessProbeTimeout)
	defer cancel()
	err := fn(probeCtx)
	duration := time.Since(start).Milliseconds()
	if err == nil {
		return ReadinessProbe{Ready: true, Status: "ready", DurationMillis: duration}
	}
	status := "failed"
	errorKind := string(database.ClassifyError(err))
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		status = "timeout"
		errorKind = "timeout"
	case errors.Is(err, context.Canceled):
		status = "canceled"
		errorKind = "canceled"
	case database.IsRetryableLock(err):
		status = "busy"
	}
	return ReadinessProbe{
		Ready:          false,
		Status:         status,
		DurationMillis: duration,
		Error:          err.Error(),
		ErrorKind:      errorKind,
	}
}

func (s *Server) readinessBackgroundDeferred(workloadLanes []WorkloadLaneDiagnostic, sqliteHealth SQLiteHealthDiagnostic) bool {
	if !sqliteHealthAllowsBackground(sqliteHealth) {
		return true
	}
	for _, lane := range workloadLanes {
		if lane.Capacity > 0 && lane.Active >= lane.Capacity {
			switch lane.ID {
			case workloadLaneAuth, workloadLaneBrowsing, workloadLaneExpensive, workloadLanePlayback, workloadLaneMedia:
				return true
			}
		}
	}
	stats := s.db.Stats()
	return stats.MaxOpenConnections > 0 && stats.InUse >= stats.MaxOpenConnections
}

func (s *Server) readinessSQLiteSnapshot(status string) ReadinessSQLiteSnapshot {
	stats := s.db.Stats()
	s.sqliteMetricsMu.Lock()
	metrics := s.sqliteMetrics
	s.sqliteMetricsMu.Unlock()
	return ReadinessSQLiteSnapshot{
		Status:              status,
		MaxOpenConnections:  stats.MaxOpenConnections,
		OpenConnections:     stats.OpenConnections,
		InUseConnections:    stats.InUse,
		IdleConnections:     stats.Idle,
		WaitCount:           stats.WaitCount,
		WaitDurationMillis:  stats.WaitDuration.Milliseconds(),
		ReadOperations:      metrics.ReadOperations,
		ReadErrors:          metrics.ReadErrors,
		WriteOperations:     metrics.WriteOperations,
		WriteAttempts:       metrics.WriteAttempts,
		LockRetries:         metrics.LockRetries,
		LockRetryWaitMillis: metrics.LockRetryWaitMillis,
		LastRetryAt:         metrics.LastRetryAt,
		LastRetryLane:       metrics.LastRetryLane,
		LastErrorKind:       metrics.LastErrorKind,
		LastErrorAt:         metrics.LastErrorAt,
		SlowestReadMillis:   metrics.SlowestReadMillis,
		SlowestReadLane:     metrics.SlowestReadLane,
		SlowestWriteMillis:  metrics.SlowestWriteMillis,
		SlowestWriteLane:    metrics.SlowestWriteLane,
	}
}

func (s *Server) handleSystemCapabilities(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	operational := s.operationalCapabilityStatuses()
	capabilityAvailable := func(id string) bool {
		status, ok := operational[id]
		return ok && status.Supported && status.State == "available"
	}
	writeJSON(w, http.StatusOK, ServerCapabilitiesResponse{
		Version:    s.compatibilityEnvelope().Build.Version,
		APIVersion: systemAPIVersion,
		Features: map[string]bool{
			"watchWithFriends":        true,
			"liveTV":                  capabilityAvailable("live-tv"),
			"dvr":                     true,
			"libraryChannels":         true,
			"libraryChannelsAdmin":    isLibraryChannelOwner(user),
			"playbackNext":            true,
			"playbackQueue":           true,
			"markerTaxonomy":          true,
			"extrasRelationships":     true,
			"clientLogUpload":         true,
			"personalDiagnostics":     true,
			"adminSystemDiagnostics":  canInteractivelyManageServer(user),
			"serverBackedSubtitles":   true,
			"serverBackedPreferences": true,
			"downloads":               capabilityAvailable("downloads"),
			"cast":                    capabilityAvailable("playback.google-cast-custom-receiver"),
		},
		OperationalCapabilities: operational,
		Permissions:             user.Permissions,
		PermissionCatalog:       permissionCatalog(),
		MarkerTypes:             mediaSegmentTypes(),
		ExtraTypes:              mediaExtraTypes(),
		GeneratedAt:             time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleSystemTime(w http.ResponseWriter, r *http.Request, _ User) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	receivedAt := time.Now().UTC()
	response := SystemTimeSync{
		RequestReceivedAt: receivedAt.Format(time.RFC3339Nano),
	}
	sentAt := time.Now().UTC()
	response.ResponseSentAt = sentAt.Format(time.RFC3339Nano)
	response.ServerUnixMillis = sentAt.UnixMilli()
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleSystemRelease(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view release readiness.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	writeJSON(w, http.StatusOK, s.systemReleaseInfo())
}

func (s *Server) handleSystemStorage(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view storage usage.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	force := queryBool(r, "refresh", false)
	writeJSON(w, http.StatusOK, s.cachedSystemStorageReport(force))
}

func (s *Server) handleSystemStoragePaths(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "Only the server owner can change storage paths.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		response, err := s.storagePathsResponse()
		if err != nil {
			writeProductError(w, http.StatusServiceUnavailable, "storage_paths_unavailable", "Portico could not read the authoritative storage-path document; no default path was selected.")
			return
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodPatch:
		var req StoragePathsRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		paths, err := config.LoadRuntimePaths(s.cfg.ConfigPath)
		if err != nil {
			writeProductError(w, http.StatusServiceUnavailable, "storage_paths_unavailable", "Portico could not read the authoritative storage-path document; no default path was selected.")
			return
		}
		if strings.TrimSpace(paths.DatabasePath) == "" {
			paths.DatabasePath = s.cfg.DatabasePath
		}
		if strings.TrimSpace(paths.BackupDirectory) == "" {
			paths.BackupDirectory = s.backupDir()
		}
		if databasePath := strings.TrimSpace(req.DatabasePath); databasePath != "" {
			normalized, err := validateStorageFilePath(databasePath)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_database_path", err.Error())
				return
			}
			if !sameHostPath(normalized, s.cfg.DatabasePath) {
				if !req.CopyDatabase {
					writeError(w, http.StatusBadRequest, "database_copy_required", "Changing the database file requires copying the current database and restarting Portico.")
					return
				}
				if err := s.copyActiveDatabaseTo(normalized); err != nil {
					writeError(w, http.StatusBadRequest, "database_move_failed", err.Error())
					return
				}
			}
			paths.DatabasePath = normalized
		}
		if backupDirectory := strings.TrimSpace(req.BackupDirectory); backupDirectory != "" {
			normalized, err := validateStorageDirectoryPath(backupDirectory)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_backup_directory", err.Error())
				return
			}
			if err := ensureWritableDirectory(normalized); err != nil {
				writeError(w, http.StatusBadRequest, "backup_directory_unwritable", "Backup directory is not writable: "+err.Error())
				return
			}
			paths.BackupDirectory = normalized
		}
		if err := config.SaveRuntimePaths(s.cfg.ConfigPath, paths); err != nil {
			writeError(w, http.StatusInternalServerError, "storage_paths_failed", "Unable to save storage path configuration.")
			return
		}
		s.clearSystemStorageCache()
		s.recordAudit(r, user, "storage.paths_updated", "settings", "storage-paths", "warn", nil)
		response, err := s.storagePathsResponse()
		if err != nil {
			writeProductError(w, http.StatusServiceUnavailable, "storage_paths_unavailable", "Storage paths were saved but could not be re-read from the authoritative document.")
			return
		}
		writeJSON(w, http.StatusOK, response)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or PATCH for this endpoint.")
	}
}

func (s *Server) handleSystemStorageCleanup(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage storage cleanup.")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	existing := false
	job, ok, err := s.activeJobFor("system_storage_cleanup", "maintenance", "storage")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_cleanup_failed", "Unable to inspect storage cleanup jobs.")
		return
	}
	if ok {
		existing = true
	} else {
		job, err = s.createJobFor("system_storage_cleanup", "Storage cleanup queued.", "maintenance", "storage")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage_cleanup_failed", "Unable to queue storage cleanup.")
			return
		}
	}
	s.recordAudit(r, user, "storage.cleanup_queued", "system", "storage", "info", map[string]string{"job": job.ID})
	writeJSON(w, http.StatusAccepted, SystemStorageCleanupResponse{
		OK:       true,
		Queued:   true,
		Job:      job,
		Existing: existing,
	})
}

func (s *Server) runSystemStorageCleanup(ctx context.Context, job Job) {
	removed := map[string]int{}
	if err := ctx.Err(); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		s.recordLog("info", "Storage cleanup cancelled.", map[string]string{"job": job.ID})
		return
	}
	_ = s.setJobMessage(job.ID, "running", 10, "Applying server storage retention.")
	if count, err := s.pruneOptimizedVersions(); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Storage cleanup failed while pruning optimized versions: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	} else {
		removed["optimized"] = count
	}
	if count, err := s.pruneOrphanedTranscodeGenerations(6 * time.Hour); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Storage cleanup failed while pruning stale transcode data: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	} else {
		removed["transcodes"] = count
	}
	_ = s.setJobMessage(job.ID, "running", 30, "Applying backup retention.")
	if count, err := s.pruneDatabaseBackups(s.scheduledTaskSettings().BackupRetentionDays); err != nil && !errors.Is(err, os.ErrNotExist) {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Storage cleanup failed while pruning backups: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	} else {
		removed["backups"] = count
	}
	_ = s.setJobMessage(job.ID, "running", 50, "Emptying expired media trash.")
	if count, err := s.pruneMediaTrash(30); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Storage cleanup failed while pruning media trash: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	} else {
		removed["mediaTrash"] = count
	}
	taskSettings := s.scheduledTaskSettings()
	_ = s.setJobMessage(job.ID, "running", 70, "Applying trickplay retention.")
	if count, err := s.pruneTrickplaySets(taskSettings.TrickplayRetentionDays, taskSettings.TrickplayMaxStorageMB); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Storage cleanup failed while pruning trickplay previews: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	} else {
		removed["trickplay"] = count
	}
	_ = s.setJobMessage(job.ID, "running", 90, "Clearing expired image cache.")
	if count, err := s.pruneImageCache(); err != nil && !errors.Is(err, os.ErrNotExist) {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Storage cleanup failed while pruning image cache: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	} else {
		removed["imageCache"] = count
	}
	s.libraryChannelPlayoutMu.Lock()
	before := s.libraryChannelCacheFiles
	s.libraryChannelPlayoutMu.Unlock()
	if err := s.maybePruneLibraryChannelSegmentCache(time.Now().UTC(), true); err != nil {
		message := "Storage cleanup failed while pruning Library Channel segments: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	}
	s.libraryChannelPlayoutMu.Lock()
	after := s.libraryChannelCacheFiles
	s.libraryChannelPlayoutMu.Unlock()
	removed["libraryChannelSegments"] = max(0, before-after)
	_ = s.setJobMessage(job.ID, "running", 95, "Applying bounded privacy retention.")
	if err := s.pruneOperationalTables(ctx); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Storage cleanup failed while applying privacy retention: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	}
	metadata := map[string]string{
		"optimized":              strconv.Itoa(removed["optimized"]),
		"transcodes":             strconv.Itoa(removed["transcodes"]),
		"backups":                strconv.Itoa(removed["backups"]),
		"mediaTrash":             strconv.Itoa(removed["mediaTrash"]),
		"trickplay":              strconv.Itoa(removed["trickplay"]),
		"imageCache":             strconv.Itoa(removed["imageCache"]),
		"libraryChannelSegments": strconv.Itoa(removed["libraryChannelSegments"]),
	}
	total := 0
	for _, count := range removed {
		total += count
	}
	message := fmt.Sprintf("Storage cleanup completed. Removed %d item%s.", total, pluralSuffix(total))
	s.clearSystemStorageCache()
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, metadata)
}

func (s *Server) handleTranscodeCapacity(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view transcode capacity.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	writeJSON(w, http.StatusOK, s.transcodeCapacityReport())
}

func (s *Server) systemDiagnostics() SystemDiagnostics {
	runtimeDiagnostics := s.runtimeDiagnostics()
	sqliteDiagnostics := s.sqliteDiagnostics()
	sqliteHealth := s.sqliteHealthSnapshot()
	jobLanes := s.jobLaneDiagnostics()
	workloadLanes := s.workloadLaneDiagnostics()
	return SystemDiagnostics{
		Version:        s.compatibilityEnvelope().Build.Version,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Addr:           s.cfg.Addr,
		WebDistReady:   pathExists(filepath.Join(s.cfg.WebDistDir, "index.html")),
		AppDataReady:   directoryWritable(s.cfg.AppDataDir),
		DatabaseReady:  pathExists(s.cfg.DatabasePath),
		Startup:        s.startupDiagnostics(),
		Runtime:        runtimeDiagnostics,
		SQLite:         sqliteDiagnostics,
		SQLiteHealth:   sqliteHealth,
		AuthCaches:     s.authorizationCacheDiagnostics(),
		Resources:      s.resourceDiagnostics(sqliteDiagnostics, jobLanes, workloadLanes),
		Admission:      s.admissionDiagnostics(),
		JobLanes:       jobLanes,
		WorkloadLanes:  workloadLanes,
		Dependencies:   s.runtimeDependencyDiagnostics(),
		MediaToolchain: s.mediaToolchainDiagnostics(),
		Playback:       s.playbackRuntimeDiagnostics(context.Background()),
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) mediaToolchainDiagnostics() MediaToolchainDiagnostic {
	s.mediaToolchainOnce.Do(func() {
		snapshot := mediatoolchain.InspectInstalled(firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg"), firstNonEmpty(s.cfg.FFprobePath, "ffprobe"), runtime.GOOS+"-"+runtime.GOARCH)
		features := make([]MediaToolchainFeature, 0, len(snapshot.Features))
		for _, feature := range snapshot.Features {
			features = append(features, MediaToolchainFeature{ID: feature.ID, Status: feature.Status, Detail: feature.Detail})
		}
		s.mediaToolchainDiagnostic = MediaToolchainDiagnostic{Source: snapshot.Source, Status: snapshot.Status, ReasonCode: snapshot.ReasonCode, Target: snapshot.Target, BuildID: snapshot.BuildID, FFmpegVersion: snapshot.FFmpegVersion, LicenseMode: snapshot.LicenseMode, ManifestPresent: snapshot.ManifestPresent, Verified: snapshot.Verified, Features: features}
	})
	return s.mediaToolchainDiagnostic
}

func (s *Server) startupDiagnostics() StartupDiagnostics {
	s.startupMu.Lock()
	phases := make([]startupPhaseState, 0, len(s.startupPhases))
	for _, phase := range s.startupPhases {
		phases = append(phases, phase)
	}
	httpReadyAt := s.startupHTTPReadyAt
	httpAddr := s.startupHTTPAddr
	startedAt := s.startedAt
	s.startupMu.Unlock()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	if len(phases) == 0 {
		phases = append(phases, startupPhaseState{
			ID:          "server_constructed",
			Label:       "Server constructed",
			Status:      "complete",
			StartedAt:   startedAt,
			CompletedAt: startedAt,
		})
	}

	sort.SliceStable(phases, func(i, j int) bool {
		if phases[i].StartedAt.Equal(phases[j].StartedAt) {
			return phases[i].ID < phases[j].ID
		}
		return phases[i].StartedAt.Before(phases[j].StartedAt)
	})
	phaseDiagnostics := make([]StartupPhaseDiagnostic, 0, len(phases))
	nonCriticalReady := true
	failed := false
	for _, phase := range phases {
		if phase.Status == "running" {
			nonCriticalReady = false
		}
		if phase.Status == "failed" {
			failed = true
		}
		diagnostic := StartupPhaseDiagnostic{
			ID:     phase.ID,
			Label:  firstNonEmpty(phase.Label, phase.ID),
			Status: phase.Status,
			Error:  phase.Error,
		}
		if !phase.StartedAt.IsZero() {
			diagnostic.StartedAt = phase.StartedAt.Format(time.RFC3339Nano)
		}
		if !phase.CompletedAt.IsZero() {
			diagnostic.CompletedAt = phase.CompletedAt.Format(time.RFC3339Nano)
			started := phase.StartedAt
			if started.IsZero() {
				started = phase.CompletedAt
			}
			diagnostic.DurationMillis = phase.CompletedAt.Sub(started).Milliseconds()
		}
		phaseDiagnostics = append(phaseDiagnostics, diagnostic)
	}
	status := "starting"
	if !httpReadyAt.IsZero() {
		status = "warming"
		if nonCriticalReady {
			status = "ready"
		}
	}
	if failed {
		status = "degraded"
	}
	response := StartupDiagnostics{
		Status:               status,
		StartedAt:            startedAt.Format(time.RFC3339Nano),
		HTTPReady:            !httpReadyAt.IsZero(),
		HTTPAddr:             httpAddr,
		NonCriticalWorkReady: nonCriticalReady,
		Phases:               phaseDiagnostics,
	}
	if !httpReadyAt.IsZero() {
		response.HTTPReadyAt = httpReadyAt.Format(time.RFC3339Nano)
	}
	return response
}

func (s *Server) storagePathsResponse() (StoragePathsResponse, error) {
	paths, err := config.LoadRuntimePaths(s.cfg.ConfigPath)
	if err != nil {
		return StoragePathsResponse{}, err
	}
	configuredDatabasePath := strings.TrimSpace(paths.DatabasePath)
	if configuredDatabasePath == "" {
		configuredDatabasePath = s.cfg.DatabasePath
	}
	backupDirectory := strings.TrimSpace(paths.BackupDirectory)
	if backupDirectory == "" {
		backupDirectory = s.backupDir()
	}
	activeDatabasePath := absoluteHostPath(s.cfg.DatabasePath)
	configuredDatabasePath = absoluteHostPath(configuredDatabasePath)
	backupDirectory = absoluteHostPath(backupDirectory)
	return StoragePathsResponse{
		AppDataDir:              absoluteHostPath(s.cfg.AppDataDir),
		ConfigPath:              absoluteHostPath(s.cfg.ConfigPath),
		ActiveDatabasePath:      activeDatabasePath,
		ConfiguredDatabasePath:  configuredDatabasePath,
		BackupDirectory:         backupDirectory,
		DatabaseRestartRequired: !sameHostPath(configuredDatabasePath, activeDatabasePath),
	}, nil
}

func validateStorageFilePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsRune(path, 0) {
		return "", errors.New("Path is invalid.")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("Path must be absolute.")
	}
	if strings.HasSuffix(path, string(filepath.Separator)) {
		return "", errors.New("Database path must include a file name.")
	}
	if strings.TrimSpace(filepath.Base(path)) == "" || filepath.Base(path) == "." || filepath.Base(path) == string(filepath.Separator) {
		return "", errors.New("Database path must include a file name.")
	}
	return filepath.Clean(path), nil
}

func validateStorageDirectoryPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsRune(path, 0) {
		return "", errors.New("Folder path is invalid.")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("Folder path must be absolute.")
	}
	return filepath.Clean(path), nil
}

func ensureWritableDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	probe, err := os.CreateTemp(path, ".portico-write-test-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func sameHostPath(a, b string) bool {
	a = absoluteHostPath(a)
	b = absoluteHostPath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func absoluteHostPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func (s *Server) copyActiveDatabaseTo(path string) error {
	if !s.sqliteExclusiveMaintenanceAllowed() {
		return errors.New("SQLite copy maintenance is deferred while playback or interactive work is active")
	}
	if sameHostPath(path, s.cfg.DatabasePath) {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return errors.New("A database file already exists at that path.")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := ensureWritableDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if _, err := s.execBackgroundWrite(context.Background(), `PRAGMA wal_checkpoint(FULL)`); err != nil {
		return err
	}
	if _, err := s.execBackgroundWrite(context.Background(), `VACUUM INTO ?`, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func (s *Server) runtimeDiagnostics() RuntimeDiagnostics {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	startedAt := s.startedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	lastPauseMillis := uint64(0)
	if mem.NumGC > 0 {
		lastPauseMillis = mem.PauseNs[(mem.NumGC+255)%256] / uint64(time.Millisecond)
	}
	return RuntimeDiagnostics{
		StartedAt:          startedAt.Format(time.RFC3339),
		UptimeSeconds:      int64(time.Since(startedAt).Seconds()),
		Goroutines:         runtime.NumGoroutine(),
		HeapAllocBytes:     mem.HeapAlloc,
		HeapSysBytes:       mem.HeapSys,
		HeapIdleBytes:      mem.HeapIdle,
		HeapReleasedBytes:  mem.HeapReleased,
		StackInUseBytes:    mem.StackInuse,
		NextGCBytes:        mem.NextGC,
		NumGC:              mem.NumGC,
		LastGCPauseMillis:  lastPauseMillis,
		TotalGCPauseMillis: mem.PauseTotalNs / uint64(time.Millisecond),
		IOPressure:         s.ioPressureDiagnostics(),
	}
}

func (s *Server) admissionDiagnostics() AdmissionDiagnostics {
	settings := s.transcodeSettings()
	maxTranscodes := settings.MaxConcurrentSessions
	searchActive := 0
	s.searchMu.Lock()
	for _, active := range s.searchActive {
		searchActive += active
	}
	s.searchMu.Unlock()

	downloadActive := 0
	s.downloadMu.Lock()
	for _, active := range s.downloadActive {
		downloadActive += active
	}
	s.downloadMu.Unlock()

	streamActive := 0
	s.streamMu.Lock()
	for _, active := range s.streamActive {
		streamActive += active
	}
	s.streamMu.Unlock()

	return AdmissionDiagnostics{
		SearchActive:             searchActive,
		SearchCapacityPerUser:    maxConcurrentSearchesPerUser,
		SearchCapacityGlobal:     maxConcurrentSearchesGlobal,
		SearchRejected:           s.searchRejected.Load(),
		DownloadActive:           downloadActive,
		DownloadCapacityPerUser:  maxConcurrentDownloadsPerUser,
		DownloadRejected:         s.downloadRejected.Load(),
		StreamActive:             streamActive,
		StreamCapacityPerUser:    maxConcurrentStreamsPerUser,
		StreamRejected:           s.streamRejected.Load(),
		TranscodeActive:          s.activeTranscodeSessionCount(),
		TranscodeCapacity:        maxTranscodes,
		TranscodeCapacityPerUser: maxConcurrentTranscodesPerUser(settings),
		TranscodeRejected:        s.transcodeRejected.Load(),
		TranscodeUserRejected:    s.transcodeUserRejected.Load(),
		GeneratedAt:              time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) resourceDiagnostics(sqlite SQLiteDiagnostics, jobLanes []JobLaneDiagnostic, workloadLanes []WorkloadLaneDiagnostic) ResourceDiagnostics {
	settings := s.transcodeSettings()
	maxTranscodes := settings.MaxConcurrentSessions
	activeTranscodes := s.activeTranscodeSessionCount()
	activePlayback := s.activePlaybackSessionCount()
	saturatedWorkload := []string{}
	saturatedJobs := []string{}
	signals := []string{}
	actions := []string{}
	queuedJobs := 0
	runningJobs := 0
	criticalWorkloadSaturated := false
	sqliteHealth := s.sqliteHealthSnapshot()
	sqliteHealthDegraded := !sqliteHealthAllowsBackground(sqliteHealth)
	deferredMaintenanceJobs := s.deferredMaintenanceJobCount()
	failedMaintenanceJobs := s.failedMaintenanceJobCount()

	for _, lane := range workloadLanes {
		if lane.Capacity > 0 && lane.Active >= lane.Capacity {
			saturatedWorkload = append(saturatedWorkload, lane.ID)
			signals = append(signals, lane.ID+" workload lane is saturated")
			switch lane.ID {
			case workloadLaneAuth, workloadLaneBrowsing, workloadLaneExpensive, workloadLanePlayback, workloadLaneMedia:
				criticalWorkloadSaturated = true
			}
		}
	}
	for _, lane := range jobLanes {
		queuedJobs += lane.Queued
		runningJobs += lane.Running
		if lane.Capacity > 0 && lane.Active >= lane.Capacity {
			saturatedJobs = append(saturatedJobs, lane.ID)
			signals = append(signals, lane.ID+" job lane is saturated")
		}
	}
	if sqlite.MaxOpenConnections > 0 && sqlite.InUseConnections >= sqlite.MaxOpenConnections {
		signals = append(signals, "sqlite connection pool is fully in use")
		criticalWorkloadSaturated = true
	}
	if activePlayback > 0 && sqlite.MaxOpenConnections > 0 && sqlite.InUseConnections >= max(1, sqlite.MaxOpenConnections-2) {
		signals = append(signals, "active playback is protected from sqlite pressure")
		criticalWorkloadSaturated = true
	}
	if sqliteHealthDegraded {
		signals = append(signals, "sqlite health watchdog is "+sqliteHealth.Status)
	}
	if maxTranscodes > 0 && activeTranscodes >= maxTranscodes {
		signals = append(signals, "transcode session capacity is exhausted")
		criticalWorkloadSaturated = true
	}

	status := "normal"
	backgroundDeferred := false
	if criticalWorkloadSaturated {
		status = "overloaded"
		backgroundDeferred = true
		actions = append(actions, "Defer starting queued background jobs until critical lanes recover.")
		actions = append(actions, "Return explicit retryable overload responses for saturated request lanes.")
	} else if sqliteHealthDegraded {
		status = "degraded"
		backgroundDeferred = true
		actions = append(actions, "Defer background SQLite work until the watchdog observes consecutive clean probes.")
	} else if len(saturatedWorkload) > 0 || len(saturatedJobs) > 0 || queuedJobs > runningJobs+4 {
		status = "degraded"
		actions = append(actions, "Keep background work bounded and monitor queue depth.")
	}
	if maxTranscodes > 0 && activeTranscodes >= maxTranscodes {
		actions = append(actions, "Queue or reject new transcodes while preserving active playback sessions.")
	}
	if deferredMaintenanceJobs > 0 {
		signals = append(signals, fmt.Sprintf("%d maintenance job(s) are waiting for retry/backoff", deferredMaintenanceJobs))
	}
	if len(signals) == 0 {
		signals = append(signals, "resource pressure is within configured limits")
	}
	if len(actions) == 0 {
		actions = append(actions, "No graceful degradation action is active.")
	}

	return ResourceDiagnostics{
		Status:                   status,
		BackgroundJobsDeferred:   backgroundDeferred,
		ActivePlaybackSessions:   activePlayback,
		ActiveTranscodeSessions:  activeTranscodes,
		MaxTranscodeSessions:     maxTranscodes,
		AvailableTranscodeSlots:  max(0, maxTranscodes-activeTranscodes),
		SQLiteInUseConnections:   sqlite.InUseConnections,
		SQLiteMaxOpenConnections: sqlite.MaxOpenConnections,
		SaturatedWorkloadLanes:   saturatedWorkload,
		SaturatedJobLanes:        saturatedJobs,
		QueuedBackgroundJobs:     queuedJobs,
		RunningBackgroundJobs:    runningJobs,
		DeferredMaintenanceJobs:  deferredMaintenanceJobs,
		FailedMaintenanceJobs:    failedMaintenanceJobs,
		Signals:                  signals,
		DegradationActions:       actions,
	}
}

func (s *Server) failedMaintenanceJobCount() int {
	db := s.dbHandle()
	if db == nil {
		return 0
	}
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM jobs
		WHERE status = 'failed'
			AND type IN ('library_scan', 'library_change_check', 'library_read_model_repair', 'metadata_refresh_library', 'metadata_refresh', 'media_analyze', 'lyrics_fetch_missing', 'optimize_version', 'tmdb_trending_refresh', 'live_tv_refresh', 'database_backup', 'library_trash_cleanup', 'optimized_version_prune', 'trickplay_prune', 'dvr_retention_cleanup', 'system_storage_cleanup')`).Scan(&count); err != nil {
		if s.log != nil {
			s.log.Warn("failed maintenance diagnostics failed", "error", err)
		}
		return 0
	}
	return count
}

func (s *Server) deferredMaintenanceJobCount() int {
	now := time.Now().UTC().Format(time.RFC3339)
	var deferred int
	db := s.dbHandle()
	if db == nil {
		return 0
	}
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM jobs
		WHERE status = 'queued'
			AND type IN ('library_scan', 'library_change_check', 'library_read_model_repair', 'metadata_refresh_library', 'metadata_refresh', 'media_analyze', 'lyrics_fetch_missing', 'optimize_version', 'tmdb_trending_refresh', 'database_backup', 'library_trash_cleanup', 'optimized_version_prune', 'trickplay_prune', 'dvr_retention_cleanup', 'system_storage_cleanup')
			AND ((next_run_at <> '' AND next_run_at > ?) OR (deferred_until <> '' AND deferred_until > ?))`, now, now).Scan(&deferred); err != nil {
		s.log.Warn("maintenance deferred diagnostics failed", "error", err)
	}
	return deferred
}

func (s *Server) activePlaybackSessionCount() int {
	cutoff := time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339)
	var count int
	db := s.dbHandle()
	if db == nil {
		return 0
	}
	err := db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE ended_at = '' AND last_seen_at >= ?`, cutoff).Scan(&count)
	if err != nil {
		s.log.Warn("active playback session count failed", "error", err)
		return 0
	}
	return count
}

func (s *Server) shouldDeferBackgroundJobsForPressure() bool {
	if !sqliteHealthAllowsBackground(s.sqliteHealthSnapshot()) {
		return true
	}
	sqliteDiagnostics := s.sqliteDiagnostics()
	jobLanes := s.jobLaneDiagnostics()
	workloadLanes := s.workloadLaneDiagnostics()
	return s.resourceDiagnostics(sqliteDiagnostics, jobLanes, workloadLanes).BackgroundJobsDeferred
}

func (s *Server) jobLaneDiagnostics() []JobLaneDiagnostic {
	lanes := jobLaneDefinitions()
	counts := s.jobLaneBacklogCounts()
	s.jobRunnerMu.Lock()
	defer s.jobRunnerMu.Unlock()
	diagnostics := make([]JobLaneDiagnostic, 0, len(lanes))
	for _, lane := range lanes {
		active := 0
		if tokens := s.jobLaneTokens[lane.id]; tokens != nil {
			active = len(tokens)
		}
		diagnostics = append(diagnostics, JobLaneDiagnostic{
			ID:       lane.id,
			Label:    lane.label,
			Active:   active,
			Capacity: jobLaneCapacity(lane.id),
			Queued:   counts[lane.id]["queued"],
			Running:  counts[lane.id]["running"],
		})
	}
	return diagnostics
}

func (s *Server) workloadLaneDiagnostics() []WorkloadLaneDiagnostic {
	lanes := []string{
		workloadLaneAuth,
		workloadLaneBrowsing,
		workloadLaneExpensive,
		workloadLanePlayback,
		workloadLaneMedia,
		workloadLaneMediaBody,
		workloadLaneBulkTransfer,
		workloadLaneRealtime,
		workloadLaneDLNA,
		workloadLaneAdmin,
		workloadLaneAdminHeavy,
		workloadLaneDefault,
	}
	s.workloadMu.Lock()
	defer s.workloadMu.Unlock()
	if s.workloadLanes == nil {
		s.workloadLanes = newWorkloadLanes()
	}
	diagnostics := make([]WorkloadLaneDiagnostic, 0, len(lanes))
	for _, id := range lanes {
		lane := s.workloadLanes[id]
		if lane == nil {
			continue
		}
		queued := lane.queued.Load()
		averageWaitMillis := int64(0)
		if queued > 0 {
			averageWaitMillis = int64(lane.waitNanos.Load()/queued) / int64(time.Millisecond)
		}
		queueLatency, serviceLatency := s.latencyMetrics.routeSnapshot(id)
		diagnostics = append(diagnostics, WorkloadLaneDiagnostic{
			ID:                     lane.id,
			Label:                  lane.label,
			Active:                 len(lane.tokens),
			Capacity:               lane.capacity,
			Rejected:               lane.rejected.Load(),
			Queued:                 queued,
			AverageQueueWaitMillis: averageWaitMillis,
			QueueWaitLatency:       queueLatency,
			ServiceLatency:         serviceLatency,
		})
	}
	return diagnostics
}

func (s *Server) jobLaneBacklogCounts() map[string]map[string]int {
	counts := map[string]map[string]int{}
	for _, lane := range jobLaneDefinitions() {
		counts[lane.id] = map[string]int{"queued": 0, "running": 0}
	}
	db := s.dbHandle()
	if db == nil {
		return counts
	}
	rows, err := db.Query(`
		SELECT type, status, COUNT(*)
		FROM jobs
		WHERE status IN ('queued', 'running')
		GROUP BY type, status`)
	if err != nil {
		s.log.Warn("job lane backlog diagnostics failed", "error", err)
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var jobType string
		var status string
		var count int
		if err := rows.Scan(&jobType, &status, &count); err != nil {
			s.log.Warn("job lane backlog diagnostics scan failed", "error", err)
			return counts
		}
		lane := jobLaneForType(jobType)
		if counts[lane] == nil {
			counts[lane] = map[string]int{"queued": 0, "running": 0}
		}
		counts[lane][status] += count
	}
	if err := rows.Err(); err != nil {
		s.log.Warn("job lane backlog diagnostics rows failed", "error", err)
	}
	return counts
}

func (s *Server) transcodeCapacityReport() TranscodeCapacityReport {
	settings := s.transcodeSettings()
	tempDir := strings.TrimSpace(settings.TemporaryDirectory)
	if tempDir == "" {
		tempDir = s.cfg.TranscodeDir
	}
	if tempDir == "" {
		tempDir = filepath.Join(s.cfg.AppDataDir, "transcodes")
	}
	ffmpegPath := firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg")
	ffprobePath := firstNonEmpty(s.cfg.FFprobePath, "ffprobe")
	dependencies := s.runtimeDependencyDiagnostics()
	ffmpeg := runtimeDependencyByName(dependencies, "ffmpeg", ffmpegPath)
	ffprobe := runtimeDependencyByName(dependencies, "ffprobe", ffprobePath)
	active := s.activeTranscodeSessionCount()
	maxSessions := settings.MaxConcurrentSessions
	availableSlots := 0
	if maxSessions > 0 {
		availableSlots = max(0, maxSessions-active)
	}
	encoder := ""
	encoderAvailable := false
	resolvedDevice := s.resolvedHardwareDevice(settings)
	if settings.HardwareEncoding {
		encoder = hardwareVideoEncoder(resolvedDevice)
		if encoder != "" && ffmpeg.Available {
			encoderAvailable = s.cachedFFmpegEncoderAvailable(ffmpeg.ResolvedPath, encoder)
		}
	}
	hardwareLevel, hardwareProbes := s.transcodeHardwareProbes(settings, ffmpeg, resolvedDevice, encoder, encoderAvailable)
	toneMappingAvailable, toneMappingStatus, toneMappingDetail := transcodeToneMappingStatus(settings, ffmpeg)
	warnings := transcodeCapacityWarnings(settings, ffmpeg, ffprobe, encoder, encoderAvailable, directoryWritable(tempDir), hardwareLevel, toneMappingStatus)
	return TranscodeCapacityReport{
		Enabled:                  settings.Enabled,
		MaxConcurrentSessions:    maxSessions,
		ActiveSessions:           active,
		AvailableSlots:           availableSlots,
		TemporaryDirectory:       tempDir,
		TemporaryDirectoryReady:  directoryWritable(tempDir),
		X264Preset:               settings.X264Preset,
		ThrottleBufferSeconds:    settings.ThrottleBufferSeconds,
		PlayedRetentionSeconds:   settings.PlayedRetentionSeconds,
		HardwareAcceleration:     settings.HardwareAcceleration,
		HardwareEncoding:         settings.HardwareEncoding,
		HardwareDevice:           firstNonEmpty(strings.TrimSpace(settings.HardwareDevice), "auto"),
		HardwareDecodeValue:      hardwareAccelValue(resolvedDevice),
		HardwareEncoder:          encoder,
		HardwareEncoderAvailable: encoderAvailable,
		HardwareSupportLevel:     hardwareLevel,
		HardwareProbes:           hardwareProbes,
		MaxHardwareSessions:      settings.MaxHardwareSessions,
		MaxSoftwareSessions:      settings.MaxSoftwareSessions,
		MaxBackgroundSessions:    settings.MaxBackgroundSessions,
		HDRToneMapping:           settings.HDRToneMapping,
		HDRToneMappingAlgorithm:  settings.HDRToneMappingAlgorithm,
		HDRToneMappingAvailable:  toneMappingAvailable,
		HDRToneMappingStatus:     toneMappingStatus,
		HDRToneMappingDetail:     toneMappingDetail,
		DirectStreamRemux:        settings.DirectStreamRemux,
		FFmpeg:                   ffmpeg,
		FFprobe:                  ffprobe,
		Presets:                  transcodePresetInfos(),
		Warnings:                 warnings,
		GeneratedAt:              time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) activeTranscodeSessionCount() int {
	s.transcodeMu.Lock()
	defer s.transcodeMu.Unlock()
	active := 0
	for _, session := range s.transcodes {
		if session.isRunning() {
			active++
		}
	}
	return active
}

func transcodePresetInfos() []TranscodePresetInfo {
	ids := make([]string, 0, len(transcodePresets))
	for id := range transcodePresets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	presets := make([]TranscodePresetInfo, 0, len(ids))
	for _, id := range ids {
		preset := transcodePresets[id]
		presets = append(presets, TranscodePresetInfo{
			ID:             preset.id,
			Label:          preset.label,
			Height:         preset.height,
			VideoKbps:      preset.videoK,
			AudioKbps:      preset.audioK,
			CRF:            preset.crf,
			MaxWidth:       preset.maxWidth,
			RequiresFFmpeg: id != "auto",
		})
	}
	return presets
}

func transcodeCapacityWarnings(settings transcodeSettings, ffmpeg RuntimeDependency, ffprobe RuntimeDependency, encoder string, encoderAvailable bool, tempReady bool, hardwareLevel string, toneMappingStatus string) []string {
	warnings := []string{}
	if !settings.Enabled {
		warnings = append(warnings, "Transcoding is disabled.")
	}
	if !ffmpeg.Available {
		warnings = append(warnings, "FFmpeg is not available, so HLS transcoding and optimized versions cannot run.")
	}
	if !ffprobe.Available {
		warnings = append(warnings, "FFprobe is not available, so media analysis and chapter detection cannot run.")
	}
	if !tempReady {
		warnings = append(warnings, "The transcode temporary directory is not writable.")
	}
	if settings.HardwareEncoding && encoder == "" {
		warnings = append(warnings, "Hardware encoding is enabled, but the selected hardware device does not map to an H.264 encoder.")
	}
	if settings.HardwareEncoding && encoder != "" && !encoderAvailable {
		warnings = append(warnings, "The selected hardware encoder was not reported by FFmpeg.")
	}
	if (settings.HardwareAcceleration || settings.HardwareEncoding) && hardwareLevel == "software_only" {
		warnings = append(warnings, "Hardware acceleration is configured, but FFmpeg did not report a usable decode or encode path.")
	} else if (settings.HardwareAcceleration || settings.HardwareEncoding) && hardwareLevel == "partial" {
		warnings = append(warnings, "Hardware acceleration is only partially available; some transcodes will use software.")
	}
	if settings.HDRToneMapping && toneMappingStatus == "unavailable" {
		warnings = append(warnings, "HDR tone mapping is enabled, but FFmpeg does not report both zscale and tonemap filters.")
	}
	return warnings
}

func ffmpegEncoderAvailable(ffmpegPath string, encoder string) bool {
	if ffmpegPath == "" || encoder == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-encoders").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(output)), strings.ToLower(encoder))
}

const (
	ffmpegCapabilityProbeCacheTTL        = 10 * time.Minute
	ffmpegCapabilityProbePrewarmInterval = 5 * time.Minute
)

func (s *Server) runFFmpegCapabilityPrewarm(ctx context.Context, ffmpegPath string) {
	ffmpegPath = strings.TrimSpace(ffmpegPath)
	if ffmpegPath == "" {
		return
	}
	s.refreshFFmpegCapabilityCache(ctx, ffmpegPath)
	ticker := time.NewTicker(ffmpegCapabilityProbePrewarmInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshFFmpegCapabilityCache(ctx, ffmpegPath)
		}
	}
}

func (s *Server) refreshFFmpegCapabilityCache(ctx context.Context, ffmpegPath string) {
	for _, probe := range []string{"-filters", "-encoders", "-hwaccels"} {
		s.refreshFFmpegCapabilityOutput(ctx, ffmpegPath, probe)
	}
}

func (s *Server) cachedFFmpegEncoderAvailable(ffmpegPath string, encoder string) bool {
	if ffmpegPath == "" || encoder == "" {
		return false
	}
	output, ok := s.cachedFFmpegCapabilityOutput(ffmpegPath, "-encoders")
	return ok && strings.Contains(strings.ToLower(output), strings.ToLower(encoder))
}

func (s *Server) cachedFFmpegHardwareAccelerationAvailable(ffmpegPath string, hwaccel string) bool {
	if ffmpegPath == "" || hwaccel == "" || hwaccel == "auto" {
		return false
	}
	output, ok := s.cachedFFmpegCapabilityOutput(ffmpegPath, "-hwaccels")
	if !ok {
		return false
	}
	for _, line := range strings.Split(strings.ToLower(output), "\n") {
		if strings.TrimSpace(line) == strings.ToLower(hwaccel) {
			return true
		}
	}
	return false
}

func (s *Server) cachedFFmpegFiltersAvailable(ffmpegPath string, filters ...string) bool {
	if ffmpegPath == "" || len(filters) == 0 {
		return len(filters) == 0
	}
	output, ok := s.cachedFFmpegCapabilityOutput(ffmpegPath, "-filters")
	if !ok {
		return false
	}
	for _, filter := range filters {
		if !ffmpegCapabilityOutputContainsToken(output, filter) {
			return false
		}
	}
	return true
}

func ffmpegCapabilityOutputContainsToken(output string, token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return true
	}
	for _, line := range strings.Split(strings.ToLower(output), "\n") {
		for _, field := range strings.Fields(line) {
			if field == token {
				return true
			}
		}
	}
	return false
}

func (s *Server) cachedFFmpegCapabilityOutput(ffmpegPath string, probe string) (string, bool) {
	key := ffmpegPath + "\x00" + probe
	now := time.Now()
	var stale *ffmpegProbeCacheEntry
	shouldRefresh := false
	s.ffmpegProbeCacheMu.Lock()
	if s.ffmpegProbeCache == nil {
		s.ffmpegProbeCache = map[string]ffmpegProbeCacheEntry{}
	}
	if s.ffmpegProbeRefreshes == nil {
		s.ffmpegProbeRefreshes = map[string]bool{}
	}
	if entry, ok := s.ffmpegProbeCache[key]; ok {
		if now.Before(entry.expiresAt) {
			s.ffmpegProbeCacheMu.Unlock()
			return entry.output, entry.ok
		}
		entryCopy := entry
		stale = &entryCopy
	}
	if !s.ffmpegProbeRefreshes[key] {
		s.ffmpegProbeRefreshes[key] = true
		shouldRefresh = true
	}
	s.ffmpegProbeCacheMu.Unlock()

	if shouldRefresh {
		s.startBackground("ffmpeg-capability-refresh", func(ctx context.Context) {
			defer func() {
				s.ffmpegProbeCacheMu.Lock()
				delete(s.ffmpegProbeRefreshes, key)
				s.ffmpegProbeCacheMu.Unlock()
			}()
			s.refreshFFmpegCapabilityOutput(ctx, ffmpegPath, probe)
		})
	}
	if stale != nil {
		return stale.output, stale.ok
	}
	return "", false
}

func (s *Server) refreshFFmpegCapabilityOutput(ctx context.Context, ffmpegPath string, probe string) (string, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, ffmpegPath, "-hide_banner", probe).CombinedOutput()
	entry := ffmpegProbeCacheEntry{
		output:    string(output),
		ok:        err == nil,
		expiresAt: time.Now().Add(ffmpegCapabilityProbeCacheTTL),
	}
	s.ffmpegProbeCacheMu.Lock()
	if s.ffmpegProbeCache == nil {
		s.ffmpegProbeCache = map[string]ffmpegProbeCacheEntry{}
	}
	s.ffmpegProbeCache[ffmpegPath+"\x00"+probe] = entry
	s.ffmpegProbeCacheMu.Unlock()
	return entry.output, entry.ok
}

const runtimeDependencyDiagnosticsPrewarmInterval = 5 * time.Minute

func (s *Server) runRuntimeDependencyPrewarm(ctx context.Context) {
	s.refreshRuntimeDependencyDiagnostics(ctx)
	ticker := time.NewTicker(runtimeDependencyDiagnosticsPrewarmInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshRuntimeDependencyDiagnostics(ctx)
		}
	}
}

func (s *Server) queueRuntimeDependencyRefresh() {
	s.dependencyCacheMu.Lock()
	if s.dependencyRefreshRunning {
		s.dependencyCacheMu.Unlock()
		return
	}
	s.dependencyRefreshRunning = true
	s.dependencyCacheMu.Unlock()
	s.startBackground("runtime-dependency-refresh", func(ctx context.Context) {
		defer func() {
			s.dependencyCacheMu.Lock()
			s.dependencyRefreshRunning = false
			s.dependencyCacheMu.Unlock()
		}()
		s.refreshRuntimeDependencyDiagnostics(ctx)
	})
}

func (s *Server) transcodeHardwareProbes(settings transcodeSettings, ffmpeg RuntimeDependency, resolvedDevice string, encoder string, encoderAvailable bool) (string, []TranscodeProbe) {
	probes := []TranscodeProbe{}
	hwaccel := hardwareAccelValue(resolvedDevice)
	decodeStatus := "disabled"
	decodeDetail := "Hardware decoding is not enabled."
	if settings.HardwareAcceleration {
		switch {
		case !ffmpeg.Available:
			decodeStatus = "unavailable"
			decodeDetail = "FFmpeg is not available."
		case hwaccel == "" || hwaccel == "auto":
			decodeStatus = "unknown"
			decodeDetail = "No concrete hardware acceleration backend was resolved."
		case s.cachedFFmpegHardwareAccelerationAvailable(ffmpeg.ResolvedPath, hwaccel):
			decodeStatus = "available"
			decodeDetail = "FFmpeg reports the " + hwaccel + " hardware acceleration backend."
		default:
			decodeStatus = "unavailable"
			decodeDetail = "FFmpeg does not report the " + hwaccel + " hardware acceleration backend."
		}
	}
	probes = append(probes, TranscodeProbe{Key: "hardware_decode", Label: "Hardware decode", Status: decodeStatus, Detail: decodeDetail})

	encodeStatus := "disabled"
	encodeDetail := "Hardware encoding is not enabled."
	if settings.HardwareEncoding {
		switch {
		case !ffmpeg.Available:
			encodeStatus = "unavailable"
			encodeDetail = "FFmpeg is not available."
		case encoder == "":
			encodeStatus = "unavailable"
			encodeDetail = "The selected hardware device does not map to an H.264 encoder."
		case encoderAvailable:
			encodeStatus = "available"
			encodeDetail = "FFmpeg reports " + encoder + "."
		default:
			encodeStatus = "unavailable"
			encodeDetail = "FFmpeg does not report " + encoder + "."
		}
	}
	probes = append(probes, TranscodeProbe{Key: "hardware_encode", Label: "Hardware encode", Status: encodeStatus, Detail: encodeDetail})

	enabled := settings.HardwareAcceleration || settings.HardwareEncoding
	if !enabled {
		return "disabled", probes
	}
	available := 0
	unavailable := 0
	for _, probe := range probes {
		if probe.Status == "available" {
			available++
		}
		if probe.Status == "unavailable" || probe.Status == "unknown" {
			unavailable++
		}
	}
	if available > 0 && unavailable == 0 {
		return "available", probes
	}
	if available > 0 {
		return "partial", probes
	}
	return "software_only", probes
}

func transcodeToneMappingStatus(settings transcodeSettings, ffmpeg RuntimeDependency) (bool, string, string) {
	if !settings.HDRToneMapping {
		return false, "disabled", "HDR tone mapping is not enabled."
	}
	if !ffmpeg.Available {
		return false, "unavailable", "FFmpeg is not available."
	}
	if settings.HDRToneMappingFilters {
		return true, "available", "FFmpeg reports zscale and tonemap filters."
	}
	return false, "unavailable", "FFmpeg does not report both zscale and tonemap filters; playback plans that require HDR-to-SDR conversion are rejected rather than producing incorrect color."
}

func ffmpegHardwareAccelerationAvailable(ffmpegPath string, hwaccel string) bool {
	if ffmpegPath == "" || hwaccel == "" || hwaccel == "auto" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-hwaccels").CombinedOutput()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.ToLower(string(output)), "\n") {
		if strings.TrimSpace(line) == strings.ToLower(hwaccel) {
			return true
		}
	}
	return false
}

func (s *Server) systemReleaseInfo() SystemReleaseInfo {
	dbReady := pathExists(s.cfg.DatabasePath)
	webReady := pathExists(filepath.Join(s.cfg.WebDistDir, "index.html"))
	appDataReady := directoryWritable(s.cfg.AppDataDir)
	migrationStatus := "ready"
	if !dbReady {
		migrationStatus = "database_missing"
	}
	return SystemReleaseInfo{
		Version:         s.compatibilityEnvelope().Build.Version,
		APIVersion:      systemAPIVersion,
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
		DatabaseReady:   dbReady,
		WebDistReady:    webReady,
		AppDataReady:    appDataReady,
		MigrationStatus: migrationStatus,
		InstallMethod:   "manual",
		UpdateStatus:    "unavailable",
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) systemStorageReport() SystemStorageReport {
	transcodeDir := s.transcodeSettings().TemporaryDirectory
	if strings.TrimSpace(transcodeDir) == "" {
		transcodeDir = s.cfg.TranscodeDir
	}
	categories := []SystemStorageCategory{
		s.storageCategory("database", "Database", s.cfg.DatabasePath, false),
		s.storageCategory("backups", "Database Backups", s.backupDir(), true),
		s.storageCategory("optimized", "Optimized Versions", s.optimizedVersionStorageDir(), true),
		s.storageCategory("trickplay", "Trickplay Previews", filepath.Join(s.cfg.AppDataDir, "trickplay"), true),
		s.storageCategory("imageCache", "Image Cache", filepath.Join(s.cfg.AppDataDir, "image-cache"), true),
		s.storageCategory("recordings", "DVR Recordings", filepath.Join(s.cfg.AppDataDir, "recordings"), false),
		s.storageCategory("transcodes", "Transcode Cache", transcodeDir, false),
		s.storageCategory("libraryChannelSegments", "Library Channel Segment Cache", s.libraryChannelSegmentCacheRoot(), true),
		s.storageCategory("mediaTrash", "Media Trash", filepath.Join(s.cfg.AppDataDir, "media-trash"), true),
	}
	total := int64(0)
	for _, category := range categories {
		total += category.SizeBytes
	}
	return SystemStorageReport{
		TotalBytes:  total,
		Categories:  categories,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

const (
	systemStorageReportCacheTTL                 = 30 * time.Second
	systemStorageReportForcedRefreshMinInterval = 10 * time.Second
)

func (s *Server) cachedSystemStorageReport(force bool) SystemStorageReport {
	now := time.Now()
	s.storageCacheMu.Lock()
	hasCached := !s.storageCacheAt.IsZero()
	if force && hasCached && now.Sub(s.storageCacheAt) < systemStorageReportForcedRefreshMinInterval {
		report := s.storageCache
		s.storageCacheMu.Unlock()
		return report
	}
	if force && hasCached {
		report := s.storageCache
		if !s.storageRefreshRunning {
			s.storageRefreshRunning = true
			s.startSystemStorageRefresh()
		}
		s.storageCacheMu.Unlock()
		return report
	}
	if hasCached && now.Sub(s.storageCacheAt) < systemStorageReportCacheTTL {
		report := s.storageCache
		s.storageCacheMu.Unlock()
		return report
	}
	if !force && hasCached {
		report := s.storageCache
		if !s.storageRefreshRunning {
			s.storageRefreshRunning = true
			s.startSystemStorageRefresh()
		}
		s.storageCacheMu.Unlock()
		return report
	}
	if s.storageRefreshRunning && hasCached {
		report := s.storageCache
		s.storageCacheMu.Unlock()
		return report
	}
	if !hasCached {
		report := s.fastSystemStorageReport()
		s.storageCache = report
		s.storageCacheAt = now
		if !s.storageRefreshRunning {
			s.storageRefreshRunning = true
			s.startSystemStorageRefresh()
		}
		s.storageCacheMu.Unlock()
		return report
	}
	s.storageRefreshRunning = true
	report := s.storageCache
	s.startSystemStorageRefresh()
	s.storageCacheMu.Unlock()
	return report
}

func (s *Server) fastSystemStorageReport() SystemStorageReport {
	transcodeDir := s.transcodeSettings().TemporaryDirectory
	if strings.TrimSpace(transcodeDir) == "" {
		transcodeDir = s.cfg.TranscodeDir
	}
	categories := []SystemStorageCategory{
		s.fastStorageCategory("database", "Database", s.cfg.DatabasePath, false),
		s.fastStorageCategory("backups", "Database Backups", s.backupDir(), true),
		s.fastStorageCategory("optimized", "Optimized Versions", s.optimizedVersionStorageDir(), true),
		s.fastStorageCategory("trickplay", "Trickplay Previews", filepath.Join(s.cfg.AppDataDir, "trickplay"), true),
		s.fastStorageCategory("imageCache", "Image Cache", filepath.Join(s.cfg.AppDataDir, "image-cache"), true),
		s.fastStorageCategory("recordings", "DVR Recordings", filepath.Join(s.cfg.AppDataDir, "recordings"), false),
		s.fastStorageCategory("transcodes", "Transcode Cache", transcodeDir, false),
		s.fastStorageCategory("libraryChannelSegments", "Library Channel Segment Cache", s.libraryChannelSegmentCacheRoot(), true),
		s.fastStorageCategory("mediaTrash", "Media Trash", filepath.Join(s.cfg.AppDataDir, "media-trash"), true),
	}
	total := int64(0)
	for _, category := range categories {
		total += category.SizeBytes
	}
	return SystemStorageReport{
		TotalBytes:  total,
		Categories:  categories,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) fastStorageCategory(key string, label string, path string, cleanupSupported bool) SystemStorageCategory {
	category := SystemStorageCategory{
		Key:              key,
		Label:            label,
		CleanupSupported: cleanupSupported,
	}
	info, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			category.Error = err.Error()
		}
		return category
	}
	category.Available = true
	if !info.IsDir() {
		category.SizeBytes = info.Size()
		category.FileCount = 1
		category.Writable = directoryWritable(filepath.Dir(path))
		return category
	}
	category.Writable = directoryWritable(path)
	return category
}

func (s *Server) refreshSystemStorageCache() {
	report := s.systemStorageReport()
	s.storageCacheMu.Lock()
	s.storageCache = report
	s.storageCacheAt = time.Now()
	s.storageRefreshRunning = false
	s.storageCacheMu.Unlock()
}

func (s *Server) startSystemStorageRefresh() {
	if s.startOwnedAsync("system-storage-refresh", func(context.Context) { s.refreshSystemStorageCache() }) {
		return
	}
	// The caller marks the refresh in flight before invoking this helper while
	// holding storageCacheMu. Closing generations must not strand that flag.
	s.storageRefreshRunning = false
}

func (s *Server) clearSystemStorageCache() {
	s.storageCacheMu.Lock()
	s.storageCache = SystemStorageReport{}
	s.storageCacheAt = time.Time{}
	s.storageRefreshRunning = false
	s.storageCacheMu.Unlock()
}

func (s *Server) storageCategory(key string, label string, path string, cleanupSupported bool) SystemStorageCategory {
	category := SystemStorageCategory{
		Key:              key,
		Label:            label,
		CleanupSupported: cleanupSupported,
	}
	size, files, available, err := pathUsage(path)
	category.SizeBytes = size
	category.FileCount = files
	category.Available = available
	if err != nil {
		category.Error = err.Error()
	}
	if available {
		if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
			category.Writable = directoryWritable(filepath.Dir(path))
		} else {
			category.Writable = directoryWritable(path)
		}
	}
	return category
}

func (s *Server) pruneImageCache() (int, error) {
	root := filepath.Join(s.cfg.AppDataDir, "image-cache")
	count, err := countRegularFiles(root)
	if err != nil {
		return 0, err
	}
	if err := os.RemoveAll(root); err != nil {
		return 0, err
	}
	return count, nil
}

func countRegularFiles(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	return count, err
}

func pathUsage(path string) (int64, int, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	if !info.IsDir() {
		return info.Size(), 1, true, nil
	}
	var size int64
	files := 0
	err = filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		files++
		return nil
	})
	return size, files, true, err
}

func probeRuntimeDependency(name, configuredPath string) RuntimeDependency {
	dependency := RuntimeDependency{Name: name, ConfiguredPath: configuredPath}
	resolved := configuredPath
	if filepath.Base(configuredPath) == configuredPath {
		path, err := exec.LookPath(configuredPath)
		if err != nil {
			dependency.Error = err.Error()
			return dependency
		}
		resolved = path
	}
	dependency.ResolvedPath = resolved
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, resolved, "-version").CombinedOutput()
	if err != nil {
		dependency.Error = err.Error()
		return dependency
	}
	dependency.Available = true
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 0 {
		dependency.VersionLine = strings.TrimSpace(lines[0])
	}
	dependency.Capabilities = dependencyCapabilities(name, string(output))
	return dependency
}

const runtimeDependencyDiagnosticsTTL = 30 * time.Second

type runtimeDependencyCache struct {
	expiresAt    time.Time
	ffmpegPath   string
	ffprobePath  string
	fpcalcPath   string
	dependencies []RuntimeDependency
}

func (s *Server) runtimeDependencyDiagnostics() []RuntimeDependency {
	ffmpegPath := firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg")
	ffprobePath := firstNonEmpty(s.cfg.FFprobePath, "ffprobe")
	fpcalcPath := firstNonEmpty(s.cfg.FPcalcPath, "fpcalc")
	now := time.Now()

	s.dependencyCacheMu.Lock()
	cached := s.dependencyCache
	cacheMatches := cached.ffmpegPath == ffmpegPath &&
		cached.ffprobePath == ffprobePath &&
		cached.fpcalcPath == fpcalcPath &&
		len(cached.dependencies) > 0
	if cacheMatches && now.Before(cached.expiresAt) {
		dependencies := append([]RuntimeDependency(nil), cached.dependencies...)
		s.dependencyCacheMu.Unlock()
		return dependencies
	}
	if cacheMatches {
		dependencies := append([]RuntimeDependency(nil), cached.dependencies...)
		s.dependencyCacheMu.Unlock()
		s.queueRuntimeDependencyRefresh()
		return dependencies
	}
	s.dependencyCacheMu.Unlock()

	s.queueRuntimeDependencyRefresh()
	return pendingRuntimeDependencies(ffmpegPath, ffprobePath, fpcalcPath)
}

func (s *Server) refreshRuntimeDependencyDiagnostics(ctx context.Context) []RuntimeDependency {
	ffmpegPath := firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg")
	ffprobePath := firstNonEmpty(s.cfg.FFprobePath, "ffprobe")
	fpcalcPath := firstNonEmpty(s.cfg.FPcalcPath, "fpcalc")
	now := time.Now()
	dependencies := []RuntimeDependency{
		probeRuntimeDependency("ffmpeg", ffmpegPath),
		probeRuntimeDependency("ffprobe", ffprobePath),
		probeRuntimeDependency("fpcalc", fpcalcPath),
	}
	s.dependencyCacheMu.Lock()
	s.dependencyCache = runtimeDependencyCache{
		expiresAt:    now.Add(runtimeDependencyDiagnosticsTTL),
		ffmpegPath:   ffmpegPath,
		ffprobePath:  ffprobePath,
		fpcalcPath:   fpcalcPath,
		dependencies: append([]RuntimeDependency(nil), dependencies...),
	}
	s.dependencyCacheMu.Unlock()
	return dependencies
}

func pendingRuntimeDependencies(ffmpegPath string, ffprobePath string, fpcalcPath string) []RuntimeDependency {
	return []RuntimeDependency{
		pendingRuntimeDependency("ffmpeg", ffmpegPath),
		pendingRuntimeDependency("ffprobe", ffprobePath),
		pendingRuntimeDependency("fpcalc", fpcalcPath),
	}
}

func pendingRuntimeDependency(name string, configuredPath string) RuntimeDependency {
	return RuntimeDependency{Name: name, ConfiguredPath: configuredPath, Error: "probe pending"}
}

func runtimeDependencyByName(dependencies []RuntimeDependency, name string, configuredPath string) RuntimeDependency {
	for _, dependency := range dependencies {
		if dependency.Name == name {
			return dependency
		}
	}
	return RuntimeDependency{Name: name, ConfiguredPath: configuredPath}
}

func dependencyCapabilities(name string, output string) []string {
	capabilities := []string{}
	lower := strings.ToLower(output)
	if strings.Contains(lower, "--enable-libx264") {
		capabilities = append(capabilities, "libx264")
	}
	if strings.Contains(lower, "--enable-libx265") {
		capabilities = append(capabilities, "libx265")
	}
	if strings.Contains(lower, "--enable-videotoolbox") || strings.Contains(lower, "videotoolbox") {
		capabilities = append(capabilities, "videotoolbox")
	}
	if strings.Contains(lower, "--enable-vaapi") || strings.Contains(lower, "vaapi") {
		capabilities = append(capabilities, "vaapi")
	}
	if strings.Contains(lower, "--enable-libvmaf") {
		capabilities = append(capabilities, "libvmaf")
	}
	if name == "ffprobe" && strings.Contains(lower, "ffprobe version") {
		capabilities = append(capabilities, "probe")
	}
	return capabilities
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func directoryWritable(path string) bool {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return false
	}
	probe := filepath.Join(path, ".portico-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return false
	}
	_ = os.Remove(probe)
	return true
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "owner_required", "Only the server owner can manage devices.")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/devices"), "/")
	if path == "" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
			return
		}
		devices, err := s.listDevices()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "devices_failed", "Unable to load devices.")
			return
		}
		writeJSON(w, http.StatusOK, ListResponse[Device]{Items: devices, Total: len(devices)})
		return
	}
	deviceID := strings.Split(path, "/")[0]
	switch r.Method {
	case http.MethodPatch:
		var req DeviceUpdateRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		device, err := s.updateDevice(deviceID, req)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "device_not_found", "Device was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "device_update_failed", "Unable to update device.")
			return
		}
		fields := map[string]string{}
		if req.Name != nil {
			fields["name"] = device.Name
		}
		if req.Trusted != nil {
			fields["trusted"] = strconv.FormatBool(device.Trusted)
		}
		if req.RemoteBitrateLimitMbps != nil {
			fields["remoteBitrateLimitMbps"] = strconv.Itoa(device.RemoteBitrateLimitMbps)
		}
		if req.Options != nil {
			fields["deviceOptions"] = "updated"
		}
		s.recordAudit(r, user, "device.updated", "device", deviceID, "info", fields)
		writeJSON(w, http.StatusOK, device)
	case http.MethodDelete:
		if err := s.revokeDevice(deviceID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "device_not_found", "Device was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "device_revoke_failed", "Unable to revoke device.")
			return
		}
		s.recordAudit(r, user, "device.revoked", "device", deviceID, "warn", nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PATCH or DELETE for this endpoint.")
	}
}

func (s *Server) listDevices() ([]Device, error) {
	rows, err := s.queryUserRead(context.Background(), `
		SELECT d.id, COALESCE(d.installation_id, ''), d.user_id, u.display_name, d.name, COALESCE(d.display_name, ''), d.app, d.platform, d.user_agent, d.client_ip,
			d.trusted, COALESCE(d.remote_bitrate_limit_mbps, 0), COALESCE(d.options_json, '{}'), d.revoked_at, COUNT(s.id), d.created_at, d.last_seen_at
		FROM devices d
		JOIN users u ON u.id = d.user_id
		LEFT JOIN sessions s ON s.device_id = d.id AND s.expires_at > ? AND d.revoked_at = ''
		GROUP BY d.id, d.installation_id, d.user_id, u.display_name, d.name, d.display_name, d.app, d.platform, d.user_agent, d.client_ip,
			d.trusted, d.remote_bitrate_limit_mbps, d.options_json, d.revoked_at, d.created_at, d.last_seen_at
		ORDER BY d.last_seen_at DESC`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := []Device{}
	for rows.Next() {
		var device Device
		var trusted int
		var displayName string
		var optionsJSON string
		if err := rows.Scan(&device.ID, &device.InstallationID, &device.UserID, &device.User, &device.AutoName, &displayName, &device.App, &device.Platform, &device.UserAgent, &device.ClientIP, &trusted, &device.RemoteBitrateLimitMbps, &optionsJSON, &device.RevokedAt, &device.SessionCount, &device.CreatedAt, &device.LastSeenAt); err != nil {
			return nil, err
		}
		device.Name = firstNonEmpty(displayName, device.AutoName)
		device.Trusted = trusted == 1
		device.Options = decodeDeviceOptions(optionsJSON)
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (s *Server) updateDevice(deviceID string, req DeviceUpdateRequest) (Device, error) {
	assignments := []string{}
	args := []any{}
	if req.Name != nil {
		name := truncateDeviceName(*req.Name)
		assignments = append(assignments, "display_name = ?")
		args = append(args, name)
	}
	if req.Trusted != nil {
		trusted := 0
		if *req.Trusted {
			trusted = 1
		}
		assignments = append(assignments, "trusted = ?")
		args = append(args, trusted)
	}
	if req.RemoteBitrateLimitMbps != nil {
		assignments = append(assignments, "remote_bitrate_limit_mbps = ?")
		args = append(args, normalizeRemoteBitrateLimitMbps(*req.RemoteBitrateLimitMbps))
	}
	if req.Options != nil {
		optionsJSON, err := json.Marshal(normalizeDeviceOptions(*req.Options))
		if err != nil {
			return Device{}, err
		}
		assignments = append(assignments, "options_json = ?")
		args = append(args, string(optionsJSON))
	}
	if len(assignments) == 0 {
		return s.getDevice(deviceID)
	}
	args = append(args, deviceID)
	result, err := s.execUserWrite(context.Background(), `UPDATE devices SET `+strings.Join(assignments, ", ")+` WHERE id = ? AND revoked_at = ''`, args...)
	if err != nil {
		return Device{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Device{}, err
	}
	if affected == 0 {
		return Device{}, sql.ErrNoRows
	}
	return s.getDevice(deviceID)
}

func (s *Server) getDevice(deviceID string) (Device, error) {
	devices, err := s.listDevices()
	if err != nil {
		return Device{}, err
	}
	for _, device := range devices {
		if device.ID == deviceID {
			return device, nil
		}
	}
	return Device{}, sql.ErrNoRows
}

func truncateDeviceName(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) > 80 {
		value = value[:80]
	}
	return value
}

func decodeDeviceOptions(raw string) DeviceOptions {
	var options DeviceOptions
	if strings.TrimSpace(raw) == "" {
		return options
	}
	if err := json.Unmarshal([]byte(raw), &options); err != nil {
		return DeviceOptions{}
	}
	return normalizeDeviceOptions(options)
}

func normalizeDeviceOptions(options DeviceOptions) DeviceOptions {
	normalized := DeviceOptions{
		PreferredAudioLanguage:    normalizeDeviceLanguage(options.PreferredAudioLanguage),
		PreferredSubtitleLanguage: normalizeDeviceLanguage(options.PreferredSubtitleLanguage),
		SubtitleMode:              strings.ToLower(strings.TrimSpace(options.SubtitleMode)),
	}
	switch normalized.SubtitleMode {
	case "", "default", "off", "manual", "always":
	default:
		normalized.SubtitleMode = ""
	}
	return normalized
}

func normalizeDeviceLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	if len(value) > 16 {
		return ""
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return ""
		}
	}
	return value
}

func (s *Server) upsertDeviceForRequest(r *http.Request, userID string) (string, error) {
	userAgent := strings.TrimSpace(r.Header.Get("User-Agent"))
	clientIP := clientIPFromRequest(r)
	app, platform := classifyUserAgent(userAgent)
	if app == "" {
		app = "Portico Web"
	}
	if platform == "" {
		platform = clientLocationLabel(clientIP)
	}
	fingerprint := hashToken(userID + "\n" + userAgent + "\n" + platform)
	deviceID := "dev_" + fingerprint[:24]
	now := time.Now().UTC().Format(time.RFC3339)
	name := strings.TrimSpace(platform)
	if name == "" || name == "Unknown" {
		name = app
	}
	defaultTrusted := 1
	if s.requireTrustedDevices() {
		defaultTrusted = 0
	}
	_, err := s.execUserWrite(r.Context(), `
		INSERT INTO devices (id, user_id, name, app, platform, user_agent, client_ip, trusted, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			app = excluded.app,
			platform = excluded.platform,
			user_agent = excluded.user_agent,
			client_ip = excluded.client_ip,
			last_seen_at = excluded.last_seen_at
		WHERE devices.revoked_at = ''`,
		deviceID, userID, name, app, platform, userAgent, clientIP, defaultTrusted, now, now)
	if err != nil {
		return "", err
	}
	var revokedAt string
	if err := s.queryUserRow(r.Context(), `SELECT COALESCE(revoked_at, '') FROM devices WHERE id = ? AND user_id = ?`, deviceID, userID).Scan(&revokedAt); err != nil {
		return "", err
	}
	if revokedAt != "" {
		return "", errDeviceNotAllowed
	}
	return deviceID, nil
}

func (s *Server) requireTrustedDevices() bool {
	return s.requireTrustedDevicesContext(context.Background())
}

func (s *Server) requireTrustedDevicesContext(ctx context.Context) bool {
	settings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return false
	}
	group, _ := settings["devices"].(map[string]any)
	return settingBool(group, "requireTrustedDevices", false)
}

func (s *Server) userCanApproveQuickConnect(user User) bool {
	if user.AuthProvider == "api_key" || user.APIKeyID != "" || !user.ProfileIsPrimary {
		return false
	}
	settings, err := s.loadSettings()
	if err != nil {
		return true
	}
	group, _ := settings["devices"].(map[string]any)
	mode := strings.ToLower(strings.TrimSpace(settingString(group, "quickConnectApprovalMode", "allUsers")))
	if mode != "owneronly" {
		return true
	}
	return canInteractivelyManageServer(user)
}

func (s *Server) deviceTrusted(deviceID string) bool {
	return s.deviceTrustedContext(context.Background(), deviceID)
}

func (s *Server) deviceTrustedContext(ctx context.Context, deviceID string) bool {
	if strings.TrimSpace(deviceID) == "" {
		return false
	}
	var trusted int
	if err := s.queryUserRow(ctx, `SELECT trusted FROM devices WHERE id = ? AND revoked_at = ''`, deviceID).Scan(&trusted); err != nil {
		return false
	}
	return trusted == 1
}

func (s *Server) userDevicePolicyAllows(userID string, deviceID string) bool {
	return s.userDevicePolicyAllowsContext(context.Background(), userID, deviceID)
}

func (s *Server) userDevicePolicyAllowsContext(ctx context.Context, userID string, deviceID string) bool {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(deviceID) == "" {
		return false
	}
	var role string
	var raw string
	if err := s.queryUserRow(ctx, `SELECT role, preferences_json FROM users WHERE id = ?`, userID).Scan(&role, &raw); err != nil {
		return false
	}
	if role == "owner" {
		return true
	}
	policy := decodeUserDevicePolicy(raw)
	switch policy.Mode {
	case "trusted":
		return s.deviceTrustedContext(ctx, deviceID)
	case "allowlist":
		for _, allowedID := range policy.AllowedDeviceIDs {
			if allowedID == deviceID {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func (s *Server) touchDevice(deviceID, clientIP string) {
	s.touchDeviceAt(deviceID, clientIP, time.Now().UTC().Format(time.RFC3339))
}

func (s *Server) touchDeviceAt(deviceID, clientIP, timestamp string) {
	if deviceID == "" {
		return
	}
	if _, err := s.execUserWriteTagged(context.Background(), []string{}, `UPDATE devices SET last_seen_at = ?, client_ip = CASE WHEN ? <> '' THEN ? ELSE client_ip END WHERE id = ? AND revoked_at = ''`,
		timestamp, clientIP, clientIP, deviceID); err != nil {
		s.log.Warn("device presence touch failed", "device", deviceID, "error", err)
	}
}

func (s *Server) revokeDevice(deviceID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return s.withUserTx(context.Background(), func(tx *sql.Tx) error {
		result, err := tx.Exec(`UPDATE devices SET revoked_at = ?, trusted = 0 WHERE id = ?`, now, deviceID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return sql.ErrNoRows
		}
		if _, err := tx.Exec(`UPDATE native_refresh_tokens SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE device_id = ?`, now, deviceID); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM sessions WHERE device_id = ?`, deviceID); err != nil {
			return err
		}
		return s.revokeBrowserEntryForDeviceTx(context.Background(), tx, deviceID, now)
	})
}

func classifyUserAgent(userAgent string) (string, string) {
	ua := strings.ToLower(userAgent)
	app := "Browser"
	switch {
	case strings.Contains(ua, "portico"):
		app = "Portico"
	case strings.Contains(ua, "firefox"):
		app = "Firefox"
	case strings.Contains(ua, "edg/"):
		app = "Edge"
	case strings.Contains(ua, "chrome"):
		app = "Chrome"
	case strings.Contains(ua, "safari"):
		app = "Safari"
	}
	platform := "Unknown"
	switch {
	case strings.Contains(ua, "tvos") || strings.Contains(ua, "apple tv") || strings.Contains(ua, "appletv"):
		platform = "Apple TV"
	case strings.Contains(ua, "portico ios"):
		platform = "iPhone"
	case strings.Contains(ua, "iphone"):
		platform = "iPhone"
	case strings.Contains(ua, "ipad"):
		platform = "iPad"
	case strings.Contains(ua, "android"):
		platform = "Android"
	case strings.Contains(ua, "windows"):
		platform = "Windows"
	case strings.Contains(ua, "mac os") || strings.Contains(ua, "macintosh"):
		platform = "macOS"
	case strings.Contains(ua, "linux"):
		platform = "Linux"
	}
	return app, platform
}

func (s *Server) handleAuditEvents(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view audit events.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	limit := max(1, min(500, queryInt(r, "limit", 100)))
	cursorCreatedAt, cursorID, err := decodeTimeIDCursor(strings.TrimSpace(r.URL.Query().Get("cursor")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "Audit cursor is invalid.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), navigationRequestTimeout)
	defer cancel()
	events, nextCursor, err := s.listAuditEventsPageContext(ctx, limit, cursorCreatedAt, cursorID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "audit_timeout", "Audit events exceeded the foreground request budget. Try again shortly.")
			return
		}
		writeError(w, http.StatusInternalServerError, "audit_failed", "Unable to load audit events.")
		return
	}
	writeJSON(w, http.StatusOK, ListResponse[AuditEvent]{Items: events, Total: len(events), Limit: limit, HasMore: nextCursor != "", NextCursor: nextCursor})
}

func (s *Server) recordAudit(r *http.Request, user User, action, resourceType, resourceID, severity string, metadata map[string]string) {
	if action == "" {
		return
	}
	if severity == "" {
		severity = "info"
	}
	if metadata == nil {
		metadata = map[string]string{}
	}
	metadata = s.sanitizeDiagnosticFields(metadata)
	metadataBytes, _ := json.Marshal(metadata)
	clientIP := ""
	userAgent := ""
	if r != nil {
		clientIP = clientIPFromRequest(r)
		userAgent = s.sanitizeDiagnosticText(r.Header.Get("User-Agent"), 240)
	}
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	input := securityAuditInput{
		ID: randomID("aud"), ActorUserID: strings.TrimSpace(user.ID),
		ActorEmail: s.sanitizeDiagnosticText(user.Email, 240), Action: s.sanitizeDiagnosticText(action, 160),
		ResourceType: s.sanitizeDiagnosticText(resourceType, 120), ResourceID: s.sanitizeDiagnosticText(resourceID, 240),
		Severity: s.sanitizeDiagnosticText(severity, 32), MetadataJSON: string(metadataBytes),
		ClientIP: s.sanitizeDiagnosticText(clientIP, 80), UserAgent: userAgent, CreatedAt: createdAt,
	}
	if err := s.withBackgroundTxTagged(context.Background(), []string{"audit"}, func(tx *sql.Tx) error {
		if err := s.backfillSecurityAuditEventsTx(context.Background(), tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(context.Background(), `
			INSERT INTO audit_events (id, actor_user_id, actor_email, action, resource_type, resource_id, severity, metadata_json, client_ip, user_agent, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			input.ID, input.ActorUserID, input.ActorEmail, input.Action, input.ResourceType, input.ResourceID,
			input.Severity, input.MetadataJSON, input.ClientIP, input.UserAgent, input.CreatedAt); err != nil {
			return err
		}
		return recordSecurityAuditEventTx(context.Background(), tx, input)
	}); err != nil {
		s.log.Error("security audit persistence failed", "action", input.Action, "resourceType", input.ResourceType, "error", err)
		s.diagnosticServerDropped.Add(1)
	}
}

func (s *Server) listAuditEvents(limit int) ([]AuditEvent, error) {
	events, _, err := s.listAuditEventsPageContext(context.Background(), limit, "", "")
	return events, err
}

func (s *Server) listAuditEventsContext(ctx context.Context, limit int) ([]AuditEvent, error) {
	events, _, err := s.listAuditEventsPageContext(ctx, limit, "", "")
	return events, err
}

func (s *Server) listAuditEventsPageContext(ctx context.Context, limit int, cursorCreatedAt, cursorID string) ([]AuditEvent, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit = max(1, min(500, limit))
	// Verify the retained chain once at the start of a pagination walk. Cursor
	// pages inherit that verification so a complete walk is O(n), not O(n^2).
	if cursorCreatedAt == "" || cursorID == "" {
		if err := s.ensureSecurityAuditChainCoverage(ctx); err != nil {
			return nil, "", fmt.Errorf("security audit history could not be anchored: %w", err)
		}
		if err := s.verifySecurityAuditChain(ctx); err != nil {
			s.log.Error("security audit chain verification failed", "error", err)
			return nil, "", fmt.Errorf("security audit history failed integrity verification: %w", err)
		}
	}
	args := []any{}
	where := ""
	if cursorCreatedAt != "" && cursorID != "" {
		where = "WHERE (created_at < ? OR (created_at = ? AND id < ?))"
		args = append(args, cursorCreatedAt, cursorCreatedAt, cursorID)
	}
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT id, actor_user_id, actor_email, action, resource_type, resource_id, severity, metadata_json, client_ip, user_agent, created_at
		FROM security_audit_events
		`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	events := []AuditEvent{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		var event AuditEvent
		var raw string
		if err := rows.Scan(&event.ID, &event.ActorUserID, &event.ActorEmail, &event.Action, &event.ResourceType, &event.ResourceID, &event.Severity, &raw, &event.ClientIP, &event.UserAgent, &event.CreatedAt); err != nil {
			return nil, "", err
		}
		_ = json.Unmarshal([]byte(raw), &event.Metadata)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	nextCursor := ""
	if len(events) > limit {
		events = events[:limit]
		last := events[len(events)-1]
		nextCursor = encodeTimeIDCursor(last.CreatedAt, last.ID)
	}
	return events, nextCursor, nil
}

func (s *Server) handleBackups(w http.ResponseWriter, r *http.Request, user User) {
	if !s.checkRestorePrincipal(w, user) {
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/backups"), "/")
	if (path == "restore" || path == "restore/upload") && r.Method == http.MethodPost {
		response, ok := s.enqueueUploadedRestore(w, r, user)
		if !ok {
			return
		}
		s.recordAudit(r, user, "backup.restore_enqueued", "backup", response.Name, "warn", map[string]string{"sourceKind": response.SourceKind})
		writeJSON(w, http.StatusAccepted, response)
		return
	}
	if strings.HasPrefix(path, "restore/") && r.Method == http.MethodGet {
		operationID := strings.TrimPrefix(path, "restore/")
		if !validRestoreOperationIDForHTTP(operationID) {
			writeProductError(w, http.StatusNotFound, "restore_not_found", "The restore operation was not found.")
			return
		}
		response, status, ok := s.restoreStatusResponse(r, operationID, true, &user)
		if !ok {
			code := "restore_status_unauthorized"
			if status == http.StatusNotFound {
				code = "restore_not_found"
			}
			writeProductError(w, status, code, "The restore operation could not be loaded.")
			return
		}
		writeJSON(w, status, response)
		return
	}
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			backups, err := s.listBackups()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "backups_failed", "Unable to list backups.")
				return
			}
			writeJSON(w, http.StatusOK, ListResponse[BackupInfo]{Items: backups, Total: len(backups)})
		case http.MethodPost:
			backupPath, err := s.createDatabaseBackup()
			var publicationWarning *BackupPublicationWarning
			if err != nil && !errors.As(err, &publicationWarning) {
				writeError(w, http.StatusInternalServerError, "backup_failed", "Unable to create backup.")
				return
			}
			s.recordAudit(r, user, "backup.created", "backup", filepath.Base(backupPath), "info", nil)
			info, _ := s.backupInfo(backupPath)
			if publicationWarning != nil {
				info.PublicationState = "degraded"
				info.WarningCode = publicationWarning.Code
				info.WarningMessage = publicationWarning.Message
			}
			writeJSON(w, http.StatusCreated, info)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
		}
		return
	}
	name := filepath.Base(strings.Split(path, "/")[0])
	if !isSafeBackupName(name) {
		writeError(w, http.StatusBadRequest, "invalid_backup", "Backup name is not valid.")
		return
	}
	if strings.HasSuffix(path, "/restore") && r.Method == http.MethodPost {
		response, ok := s.enqueueExistingRestore(w, r, user, name)
		if !ok {
			return
		}
		s.recordAudit(r, user, "backup.restore_enqueued", "backup", name, "warn", map[string]string{"sourceKind": response.SourceKind})
		writeJSON(w, http.StatusAccepted, response)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Backup route was not found.")
}

func validRestoreOperationIDForHTTP(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || filepath.Base(value) != value || strings.ContainsAny(value, `/\\`) {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func (s *Server) backupDir() string {
	paths, err := config.LoadRuntimePaths(s.cfg.ConfigPath)
	if err != nil {
		// A malformed runtime-path document is an unknown authority. Keep
		// callers from silently switching to an environment/default directory.
		return filepath.Join(s.cfg.AppDataDir, ".invalid-runtime-path-authority")
	}
	if backupDirectory := strings.TrimSpace(paths.BackupDirectory); backupDirectory != "" {
		return backupDirectory
	}
	if strings.TrimSpace(s.cfg.BackupDir) != "" {
		return s.cfg.BackupDir
	}
	return filepath.Join(s.cfg.AppDataDir, "backups")
}

func (s *Server) listBackups() ([]BackupInfo, error) {
	entries, err := os.ReadDir(s.backupDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, err
	}
	backups := []BackupInfo{}
	for _, entry := range entries {
		if entry.IsDir() || !isSafeBackupName(entry.Name()) {
			continue
		}
		info, err := s.backupInfo(filepath.Join(s.backupDir(), entry.Name()))
		if err == nil || info.Name != "" {
			backups = append(backups, info)
		}
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt > backups[j].CreatedAt })
	return backups, nil
}

func (s *Server) backupInfo(path string) (BackupInfo, error) {
	if err := database.ValidateRegularNonSymlinkFile(path); err != nil {
		return BackupInfo{}, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return BackupInfo{}, err
	}
	info := BackupInfo{
		Name:      filepath.Base(path),
		SizeBytes: stat.Size(),
		CreatedAt: stat.ModTime().UTC().Format(time.RFC3339),
		Integrity: "invalid",
	}
	manifest, manifestErr := readDatabaseBackupManifest(path)
	if manifestErr == nil {
		info.ManifestPresent = true
		info.ChecksumSHA256 = manifest.ChecksumSHA256
		info.Release = manifest.Release
		info.DatabaseFormatVersion = manifest.DatabaseFormatVersion
		info.MigrationHead = manifest.MigrationHead
		info.MigrationLedgerSHA256 = manifest.MigrationLedgerSHA256
		info.MigrationLedgerRows = manifest.MigrationLedgerRows
		info.MinimumReader = manifest.MinimumReader
		if strings.TrimSpace(manifest.CreatedAt) != "" {
			info.CreatedAt = manifest.CreatedAt
		}
		applyBackupPublicationEvidence(&info, path)
	} else {
		if os.IsNotExist(manifestErr) {
			info.ValidationCode = "restore_unidentified_database"
		} else {
			info.ValidationCode = "restore_corrupt_database"
		}
		return info, nil
	}
	validation, validationErr := database.ValidateRestoreCandidateWithLimit(context.Background(), path, &manifest, s.restoreDatabaseLimit())
	if validationErr != nil {
		var typed *database.RestoreValidationError
		if errors.As(validationErr, &typed) {
			info.ValidationCode = typed.Code
		} else {
			info.ValidationCode = "restore_corrupt_database"
		}
		return info, nil
	}
	info.Integrity = validation.IntegrityResult
	info.RestoreReady = true
	info.ChecksumSHA256 = validation.ChecksumSHA256
	info.DatabaseFormatVersion = validation.Migration.FormatVersion
	info.MigrationHead = validation.Migration.MigrationHead
	info.MigrationLedgerSHA256 = validation.Migration.LedgerSHA256
	info.MigrationLedgerRows = validation.MigrationRows
	return info, nil
}

type databaseBackupManifest = database.BackupManifest

func (s *Server) writeDatabaseBackupManifest(path string, backupName ...string) error {
	if err := database.ValidateRegularNonSymlinkFile(path); err != nil {
		return err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	checksum, err := database.FileSHA256(path)
	if err != nil {
		return err
	}
	inspection, err := database.InspectRestoreDatabaseWithLimit(context.Background(), path, s.restoreDatabaseLimit())
	if err != nil {
		return err
	}
	name := filepath.Base(path)
	if len(backupName) > 0 && strings.TrimSpace(backupName[0]) != "" {
		name = backupName[0]
	}
	manifest := databaseBackupManifest{
		FormatVersion:         database.RestoreManifestFormatVersion,
		Release:               firstNonEmpty(s.cfg.Release, "unknown"),
		DatabaseFormatVersion: inspection.Migration.FormatVersion,
		MigrationHead:         inspection.Migration.MigrationHead,
		MigrationLedgerSHA256: inspection.Migration.LedgerSHA256,
		MigrationLedgerRows:   inspection.MigrationRows,
		MinimumReader:         inspection.Migration.MinimumReader,
		BackupName:            name,
		CreatedAt:             time.Now().UTC().Format(time.RFC3339Nano),
		DatabaseBytes:         stat.Size(),
		ChecksumSHA256:        checksum,
		IntegrityResult:       inspection.IntegrityResult,
		ForeignKeyErrors:      inspection.ForeignKeyErrors,
		ArtifactSet: []database.BackupArtifact{{
			Name: name, Kind: "database", SizeBytes: stat.Size(), ChecksumSHA256: checksum,
		}},
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return database.WriteAtomicPrivateFile(backupManifestPath(path), body)
}

func readDatabaseBackupManifest(path string) (databaseBackupManifest, error) {
	return database.ReadBackupManifest(path)
}

func backupManifestPath(path string) string {
	return path + ".manifest.json"
}

const backupPublicationEvidenceVersion = 1

type backupPublicationEvidence struct {
	Version     int    `json:"version"`
	State       string `json:"state"`
	WarningCode string `json:"warningCode"`
	Message     string `json:"message"`
	UpdatedAt   string `json:"updatedAt"`
}

func backupPublicationEvidencePath(path string) string {
	return path + ".publication.json"
}

func persistBackupPublicationWarning(path string, warning *BackupPublicationWarning) error {
	if warning == nil {
		return nil
	}
	evidence := backupPublicationEvidence{
		Version:     backupPublicationEvidenceVersion,
		State:       "degraded",
		WarningCode: strings.TrimSpace(warning.Code),
		Message:     strings.TrimSpace(warning.Message),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if evidence.WarningCode == "" || evidence.Message == "" {
		return errors.New("backup publication warning evidence is incomplete")
	}
	body, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	return writeBackupPublicationEvidence(backupPublicationEvidencePath(path), body)
}

func clearBackupPublicationEvidence(path string) error {
	return removeBackupArtifact(backupPublicationEvidencePath(path))
}

func readBackupPublicationEvidence(path string) (backupPublicationEvidence, error) {
	evidencePath := backupPublicationEvidencePath(path)
	if err := database.ValidateRegularNonSymlinkFile(evidencePath); err != nil {
		return backupPublicationEvidence{}, err
	}
	info, err := os.Stat(evidencePath)
	if err != nil {
		return backupPublicationEvidence{}, err
	}
	if info.Size() > 16<<10 {
		return backupPublicationEvidence{}, errors.New("backup publication evidence exceeds the bounded size")
	}
	body, err := os.ReadFile(evidencePath)
	if err != nil {
		return backupPublicationEvidence{}, err
	}
	var evidence backupPublicationEvidence
	if err := json.Unmarshal(body, &evidence); err != nil {
		return backupPublicationEvidence{}, err
	}
	if evidence.Version != backupPublicationEvidenceVersion || evidence.State != "degraded" || strings.TrimSpace(evidence.WarningCode) == "" || strings.TrimSpace(evidence.Message) == "" {
		return backupPublicationEvidence{}, errors.New("backup publication evidence is invalid")
	}
	return evidence, nil
}

func applyBackupPublicationEvidence(info *BackupInfo, path string) {
	if info == nil {
		return
	}
	evidence, err := readBackupPublicationEvidence(path)
	if err != nil {
		if !os.IsNotExist(err) {
			info.PublicationState = "degraded"
			info.WarningCode = "backup_publication_evidence_invalid"
			info.WarningMessage = "Backup durability evidence could not be verified."
		}
		return
	}
	info.PublicationState = evidence.State
	info.WarningCode = evidence.WarningCode
	info.WarningMessage = evidence.Message
}

func isSafeBackupName(name string) bool {
	return strings.HasPrefix(name, "portico-") && strings.HasSuffix(name, ".db") && filepath.Base(name) == name
}

func queryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil {
		return fallback
	}
	return value
}

func queryBool(r *http.Request, key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func (s *Server) handlePlaylists(w http.ResponseWriter, r *http.Request, user User) {
	resourcePath := "/api/playlists"
	resourceKind := "playlist"
	resourceLabel := "Playlist"
	if strings.HasPrefix(r.URL.Path, "/api/collections") {
		resourcePath = "/api/collections"
		resourceKind = "collection"
		resourceLabel = "Collection"
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, resourcePath), "/")
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			playlists, err := s.listPlaylistsContext(r.Context(), user, resourceKind, false)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "playlists_failed", "Unable to load playlists.")
				return
			}
			writeJSON(w, http.StatusOK, ListResponse[Playlist]{Items: playlists, Total: len(playlists), Filter: resourceKind})
		case http.MethodPost:
			var req PlaylistRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			req.Kind = resourceKind
			playlist, err := s.createPlaylist(user, req, queryBool(r, "includeItems", false))
			if err != nil {
				writeError(w, http.StatusBadRequest, "playlist_create_failed", err.Error())
				return
			}
			s.recordAudit(r, user, "playlist.created", "playlist", playlist.ID, "info", playlistAuditMetadata(playlist, ""))
			if req.Shares != nil {
				s.recordAudit(r, user, "playlist.shares_updated", "playlist", playlist.ID, "warn", playlistAuditMetadata(playlist, ""))
			}
			writeJSON(w, http.StatusCreated, playlist)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
		}
		return
	}
	parts := strings.Split(path, "/")
	playlistID := parts[0]
	existing, err := s.getPlaylistContext(r.Context(), user, playlistID, false)
	if err != nil || existing.Kind != resourceKind {
		writeError(w, http.StatusNotFound, resourceKind+"_not_found", resourceLabel+" was not found.")
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			ctx, cancel := context.WithTimeout(r.Context(), navigationRequestTimeout)
			defer cancel()
			playlist, err := s.getPlaylistContext(ctx, user, playlistID, queryBool(r, "includeItems", true))
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					w.Header().Set("Retry-After", "1")
					writeError(w, http.StatusServiceUnavailable, "playlist_timeout", "Playlist detail exceeded the foreground request budget. Try again shortly.")
					return
				}
				writeError(w, http.StatusNotFound, "playlist_not_found", "Playlist was not found.")
				return
			}
			writeJSON(w, http.StatusOK, playlist)
		case http.MethodPatch:
			var req PlaylistRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			playlist, err := s.updatePlaylist(user, playlistID, req, queryBool(r, "includeItems", false))
			if err != nil {
				writeError(w, http.StatusBadRequest, "playlist_update_failed", err.Error())
				return
			}
			s.recordAudit(r, user, "playlist.updated", "playlist", playlist.ID, "info", playlistAuditMetadata(playlist, ""))
			if req.Shares != nil {
				s.recordAudit(r, user, "playlist.shares_updated", "playlist", playlist.ID, "warn", playlistAuditMetadata(playlist, ""))
			}
			writeJSON(w, http.StatusOK, playlist)
		case http.MethodDelete:
			playlist, _ := s.getPlaylistContext(r.Context(), user, playlistID, false)
			if err := s.deletePlaylist(user, playlistID); err != nil {
				writeError(w, http.StatusNotFound, "playlist_not_found", "Playlist was not found.")
				return
			}
			s.recordAudit(r, user, "playlist.deleted", "playlist", playlistID, "warn", playlistAuditMetadata(playlist, ""))
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET, PATCH, or DELETE for this endpoint.")
		}
		return
	}
	if len(parts) == 2 && parts[1] == "items" && r.Method == http.MethodPost {
		var req PlaylistItemRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		playlist, err := s.addPlaylistItem(user, playlistID, req.MediaID, queryBool(r, "includeItems", false))
		if err != nil {
			writeError(w, http.StatusBadRequest, "playlist_item_failed", err.Error())
			return
		}
		s.recordAudit(r, user, "playlist.item_added", "playlist", playlist.ID, "info", playlistAuditMetadata(playlist, req.MediaID))
		writeJSON(w, http.StatusOK, playlist)
		return
	}
	if len(parts) == 2 && parts[1] == "items" && r.Method == http.MethodGet {
		ctx, cancel := context.WithTimeout(r.Context(), navigationRequestTimeout)
		defer cancel()
		limit, offset := paginationFromRequest(r, 100, maxPlaylistItemsResponse)
		if existing.Smart {
			allItems, smartErr := s.smartPlaylistItemsContext(ctx, user, existing.SmartFilter)
			if smartErr != nil {
				if errors.Is(smartErr, context.DeadlineExceeded) || errors.Is(smartErr, context.Canceled) {
					w.Header().Set("Retry-After", "1")
					writeError(w, http.StatusServiceUnavailable, "playlist_items_timeout", resourceLabel+" items exceeded the foreground request budget. Try again shortly.")
					return
				}
				writeError(w, http.StatusInternalServerError, "playlist_items_failed", "Unable to load rule-backed "+strings.ToLower(resourceLabel)+" items.")
				return
			}
			total := len(allItems)
			start := min(offset, total)
			end := min(start+limit, total)
			writeJSON(w, http.StatusOK, ListResponse[MediaItem]{Items: allItems[start:end], Total: total, Limit: limit, Offset: start, HasMore: end < total})
			return
		}
		items, total, hasMore, err := s.playlistItemsPageContext(ctx, user, playlistID, limit, offset)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusServiceUnavailable, "playlist_items_timeout", "Playlist items exceeded the foreground request budget. Try again shortly.")
				return
			}
			writeError(w, http.StatusNotFound, "playlist_not_found", "Playlist was not found.")
			return
		}
		writeJSON(w, http.StatusOK, ListResponse[MediaItem]{Items: items, Total: total, Limit: limit, Offset: offset, HasMore: hasMore})
		return
	}
	if len(parts) == 3 && parts[1] == "items" && parts[2] == "bulk" && r.Method == http.MethodPost {
		var req PlaylistItemOrderRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		response, err := s.addPlaylistItemsBulk(user, playlistID, req.MediaIDs, queryBool(r, "includeItems", false))
		if err != nil {
			writeError(w, http.StatusBadRequest, "playlist_items_bulk_failed", err.Error())
			return
		}
		s.recordAudit(r, user, "playlist.items_added_bulk", "playlist", response.Playlist.ID, "info", map[string]string{
			"title":     response.Playlist.Title,
			"itemCount": strconv.Itoa(response.Playlist.ItemCount),
			"added":     strconv.Itoa(response.Added),
			"skipped":   strconv.Itoa(response.Skipped),
		})
		writeJSON(w, http.StatusOK, response)
		return
	}
	if len(parts) == 2 && parts[1] == "items" && r.Method == http.MethodPatch {
		var req PlaylistItemOrderRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		playlist, err := s.reorderPlaylistItems(user, playlistID, req.MediaIDs, queryBool(r, "includeItems", false))
		if err != nil {
			writeError(w, http.StatusBadRequest, "playlist_item_order_failed", err.Error())
			return
		}
		s.recordAudit(r, user, "playlist.items_reordered", "playlist", playlist.ID, "info", playlistAuditMetadata(playlist, ""))
		writeJSON(w, http.StatusOK, playlist)
		return
	}
	if len(parts) == 3 && parts[1] == "items" && r.Method == http.MethodDelete {
		playlist, err := s.removePlaylistItem(user, playlistID, parts[2], queryBool(r, "includeItems", false))
		if err != nil {
			writeError(w, http.StatusBadRequest, "playlist_item_failed", err.Error())
			return
		}
		s.recordAudit(r, user, "playlist.item_removed", "playlist", playlist.ID, "info", playlistAuditMetadata(playlist, parts[2]))
		writeJSON(w, http.StatusOK, playlist)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "Playlist route was not found.")
}

func normalizePlaylistKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "collection":
		return "collection"
	default:
		return "playlist"
	}
}

func normalizePlaylistVisibility(visibility string) string {
	switch strings.ToLower(strings.TrimSpace(visibility)) {
	case "server":
		return "server"
	default:
		return "private"
	}
}

func playlistAuditMetadata(playlist Playlist, mediaID string) map[string]string {
	editableShares := 0
	for _, share := range playlist.Shares {
		if share.CanEdit {
			editableShares++
		}
	}
	metadata := map[string]string{
		"title":          playlist.Title,
		"kind":           playlist.Kind,
		"visibility":     playlist.Visibility,
		"ownerUserId":    playlist.UserID,
		"smart":          strconv.FormatBool(playlist.Smart),
		"itemCount":      strconv.Itoa(playlist.ItemCount),
		"shareCount":     strconv.Itoa(len(playlist.Shares)),
		"editableShares": strconv.Itoa(editableShares),
	}
	if strings.TrimSpace(mediaID) != "" {
		metadata["mediaId"] = strings.TrimSpace(mediaID)
	}
	return metadata
}

func (s *Server) listPlaylists(user User, kind string, includeItems bool) ([]Playlist, error) {
	return s.listPlaylistsContext(context.Background(), user, kind, includeItems)
}

func (s *Server) listPlaylistsContext(ctx context.Context, user User, kind string, includeItems bool) ([]Playlist, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	args := []any{user.ID, viewerProfileID(user)}
	where := `WHERE (p.profile_id = ? OR p.visibility = 'server' OR ps_self.user_id IS NOT NULL)`
	if strings.TrimSpace(kind) != "" {
		where += ` AND p.kind = ?`
		args = append(args, normalizePlaylistKind(kind))
	}
	if canInteractivelyManageServer(user) {
		where = `WHERE (p.profile_id = ? OR p.visibility = 'server' OR ps_self.user_id IS NOT NULL)`
		if strings.TrimSpace(kind) != "" {
			where += ` AND p.kind = ?`
		}
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT p.id, p.user_id, p.profile_id, p.kind, p.title, p.summary, p.visibility, p.smart_filter_json, COUNT(pi.media_id), COALESCE(MAX(ps_self.can_edit), 0), p.created_at, p.updated_at
		FROM playlists p
		LEFT JOIN playlist_items pi ON pi.playlist_id = p.id
		LEFT JOIN playlist_shares ps_self ON ps_self.playlist_id = p.id AND ps_self.user_id = ?
		`+where+`
		GROUP BY p.id, p.user_id, p.profile_id, p.kind, p.title, p.summary, p.visibility, p.smart_filter_json, p.created_at, p.updated_at
		ORDER BY p.updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	playlists := []Playlist{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		var playlist Playlist
		var smartFilterJSON string
		var sharedCanEdit int
		if err := rows.Scan(&playlist.ID, &playlist.UserID, &playlist.ProfileID, &playlist.Kind, &playlist.Title, &playlist.Summary, &playlist.Visibility, &smartFilterJSON, &playlist.ItemCount, &sharedCanEdit, &playlist.CreatedAt, &playlist.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		playlist.SmartFilter = decodeSmartFilter(smartFilterJSON)
		playlist.Smart = playlist.SmartFilter.Enabled
		playlist.CanEdit = playlist.ProfileID == viewerProfileID(user) || canInteractivelyManageServer(user) || sharedCanEdit != 0
		playlists = append(playlists, playlist)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range playlists {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		playlist := &playlists[index]
		shares, err := s.playlistSharesContext(ctx, playlist.ID)
		if err != nil {
			return nil, err
		}
		playlist.Shares = shares
		if playlist.Smart {
			if includeItems {
				items, err := s.smartPlaylistItemsWithLimitContext(ctx, user, playlist.SmartFilter, playlist.SmartFilter.Limit)
				if err != nil {
					return nil, err
				}
				playlist.ItemCount = len(items)
				playlist.Items = items
			}
			continue
		}
		if includeItems {
			items, err := s.playlistItemsContext(ctx, viewerProfileID(user), playlist.ID)
			if err != nil {
				return nil, err
			}
			playlist.Items = items
		}
	}
	return playlists, nil
}

func (s *Server) getPlaylist(user User, playlistID string, includeItems bool) (Playlist, error) {
	return s.getPlaylistContext(context.Background(), user, playlistID, includeItems)
}

func (s *Server) getPlaylistContext(ctx context.Context, user User, playlistID string, includeItems bool) (Playlist, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	args := []any{user.ID, playlistID, viewerProfileID(user)}
	rows, err := s.queryUserRead(ctx, `
		SELECT p.id, p.user_id, p.profile_id, p.kind, p.title, p.summary, p.visibility, p.smart_filter_json, COUNT(pi.media_id), COALESCE(MAX(ps_self.can_edit), 0), p.created_at, p.updated_at
		FROM playlists p
		LEFT JOIN playlist_items pi ON pi.playlist_id = p.id
		LEFT JOIN playlist_shares ps_self ON ps_self.playlist_id = p.id AND ps_self.user_id = ?
		WHERE p.id = ?
			AND (p.profile_id = ? OR p.visibility = 'server' OR ps_self.user_id IS NOT NULL)
		GROUP BY p.id, p.user_id, p.profile_id, p.kind, p.title, p.summary, p.visibility, p.smart_filter_json, p.created_at, p.updated_at`, args...)
	if err != nil {
		return Playlist{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Playlist{}, err
		}
		return Playlist{}, sql.ErrNoRows
	}
	var playlist Playlist
	var smartFilterJSON string
	var sharedCanEdit int
	if err := rows.Scan(&playlist.ID, &playlist.UserID, &playlist.ProfileID, &playlist.Kind, &playlist.Title, &playlist.Summary, &playlist.Visibility, &smartFilterJSON, &playlist.ItemCount, &sharedCanEdit, &playlist.CreatedAt, &playlist.UpdatedAt); err != nil {
		return Playlist{}, err
	}
	if err := rows.Err(); err != nil {
		return Playlist{}, err
	}
	playlist.SmartFilter = decodeSmartFilter(smartFilterJSON)
	playlist.Smart = playlist.SmartFilter.Enabled
	playlist.CanEdit = playlist.ProfileID == viewerProfileID(user) || canInteractivelyManageServer(user) || sharedCanEdit != 0
	shares, err := s.playlistSharesContext(ctx, playlist.ID)
	if err != nil {
		return Playlist{}, err
	}
	playlist.Shares = shares
	if includeItems {
		if playlist.Smart {
			items, err := s.smartPlaylistItemsContext(ctx, user, playlist.SmartFilter)
			if err != nil {
				return Playlist{}, err
			}
			playlist.Items = items
			playlist.ItemCount = len(playlist.Items)
		} else {
			items, err := s.playlistItemsContext(ctx, viewerProfileID(user), playlist.ID)
			if err != nil {
				return Playlist{}, err
			}
			playlist.Items = items
		}
	}
	return playlist, nil
}

func (s *Server) createPlaylist(user User, req PlaylistRequest, includeItems bool) (Playlist, error) {
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		return Playlist{}, errors.New("Playlist title is required.")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id := randomOpaquePublicID()
	kind := normalizePlaylistKind(req.Kind)
	visibility := normalizePlaylistVisibility(req.Visibility)
	if visibility == "server" && !canInteractivelyManageServer(user) {
		visibility = "private"
	}
	smartFilter, smartFilterJSON, err := normalizeSmartFilter(req.SmartFilter)
	if err != nil {
		return Playlist{}, err
	}
	ids := []string{}
	if len(req.MediaIDs) > 0 {
		ids, err = normalizePlaylistBulkMediaIDs(req.MediaIDs)
		if err != nil {
			return Playlist{}, err
		}
	}
	if smartFilter.Enabled && len(ids) > 0 {
		return Playlist{}, errors.New("Smart playlists are populated by filters and cannot be created with manual media items.")
	}
	if len(ids) > 0 {
		items, err := s.mediaListItemsByOrderedIDs(viewerProfileID(user), ids)
		if err != nil {
			return Playlist{}, err
		}
		if len(items) != len(ids) {
			return Playlist{}, errors.New("One or more media items were not found.")
		}
	}
	if err := s.withUserTxTagged(context.Background(), []string{"playlists", "saved"}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO playlists (id, user_id, profile_id, kind, title, summary, visibility, smart_filter_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, accountIDForUser(user), viewerProfileID(user), kind, req.Title, strings.TrimSpace(req.Summary), visibility, smartFilterJSON, now, now); err != nil {
			return err
		}
		for index, mediaID := range ids {
			if _, err := tx.Exec(`
				INSERT INTO playlist_items (playlist_id, media_id, sort_order, added_at)
				VALUES (?, ?, ?, ?)`,
				id, mediaID, index+1, now); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Playlist{}, err
	}
	if req.Shares != nil {
		if err := s.replacePlaylistShares(id, *req.Shares); err != nil {
			return Playlist{}, err
		}
	}
	_ = smartFilter
	return s.getPlaylist(user, id, includeItems)
}

func (s *Server) updatePlaylist(user User, playlistID string, req PlaylistRequest, includeItems bool) (Playlist, error) {
	current, err := s.getPlaylist(user, playlistID, false)
	if err != nil {
		return Playlist{}, err
	}
	if current.ProfileID != viewerProfileID(user) && !canInteractivelyManageServer(user) {
		return Playlist{}, errors.New("You cannot edit this playlist.")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = current.Title
	}
	visibility := normalizePlaylistVisibility(req.Visibility)
	if visibility == "server" && !canInteractivelyManageServer(user) {
		visibility = "private"
	}
	_, smartFilterJSON, err := normalizeSmartFilter(req.SmartFilter)
	if err != nil {
		return Playlist{}, err
	}
	_, err = s.execUserWriteTagged(context.Background(), []string{"playlists", "saved"}, `UPDATE playlists SET kind = ?, title = ?, summary = ?, visibility = ?, smart_filter_json = ?, updated_at = ? WHERE id = ?`,
		normalizePlaylistKind(req.Kind), title, strings.TrimSpace(req.Summary), visibility, smartFilterJSON, time.Now().UTC().Format(time.RFC3339), playlistID)
	if err != nil {
		return Playlist{}, err
	}
	if req.Shares != nil {
		if err := s.replacePlaylistShares(playlistID, *req.Shares); err != nil {
			return Playlist{}, err
		}
	}
	return s.getPlaylist(user, playlistID, includeItems)
}

func (s *Server) deletePlaylist(user User, playlistID string) error {
	current, err := s.getPlaylist(user, playlistID, false)
	if err != nil {
		return err
	}
	if current.ProfileID != viewerProfileID(user) && !canInteractivelyManageServer(user) {
		return errors.New("You cannot delete this playlist.")
	}
	result, err := s.execUserWriteTagged(context.Background(), []string{"playlists", "saved"}, `DELETE FROM playlists WHERE id = ?`, playlistID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) addPlaylistItem(user User, playlistID string, mediaID string, includeItems bool) (Playlist, error) {
	current, err := s.getPlaylist(user, playlistID, false)
	if err != nil {
		return Playlist{}, err
	}
	if current.Smart {
		return Playlist{}, errors.New("Smart playlists are populated by filters and cannot be edited manually.")
	}
	if !current.CanEdit {
		return Playlist{}, errors.New("You cannot edit this playlist.")
	}
	if _, err := s.getMediaListItem(viewerProfileID(user), mediaID); err != nil {
		return Playlist{}, errors.New("Media item was not found.")
	}
	var sortOrder int
	_ = s.queryUserRow(context.Background(), `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM playlist_items WHERE playlist_id = ?`, playlistID).Scan(&sortOrder)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.execUserWriteTagged(context.Background(), []string{"playlists", "saved"}, `
		INSERT INTO playlist_items (playlist_id, media_id, sort_order, added_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(playlist_id, media_id) DO UPDATE SET sort_order = excluded.sort_order`,
		playlistID, mediaID, sortOrder, now)
	if err != nil {
		return Playlist{}, err
	}
	_, _ = s.execUserWriteTagged(context.Background(), []string{}, `UPDATE playlists SET updated_at = ? WHERE id = ?`, now, playlistID)
	return s.getPlaylist(user, playlistID, includeItems)
}

func (s *Server) addPlaylistItemsBulk(user User, playlistID string, mediaIDs []string, includeItems bool) (PlaylistBulkItemsResponse, error) {
	current, err := s.getPlaylist(user, playlistID, false)
	if err != nil {
		return PlaylistBulkItemsResponse{}, err
	}
	if current.Smart {
		return PlaylistBulkItemsResponse{}, errors.New("Smart playlists are populated by filters and cannot be edited manually.")
	}
	if !current.CanEdit {
		return PlaylistBulkItemsResponse{}, errors.New("You cannot edit this playlist.")
	}
	ids, err := normalizePlaylistBulkMediaIDs(mediaIDs)
	if err != nil {
		return PlaylistBulkItemsResponse{}, err
	}
	items, err := s.mediaListItemsByOrderedIDs(viewerProfileID(user), ids)
	if err != nil {
		return PlaylistBulkItemsResponse{}, err
	}
	if len(items) != len(ids) {
		return PlaylistBulkItemsResponse{}, errors.New("One or more media items were not found.")
	}
	var sortOrder int
	now := time.Now().UTC().Format(time.RFC3339)
	added := 0
	if err := s.withUserTxTagged(context.Background(), []string{"playlists", "saved"}, func(tx *sql.Tx) error {
		if err := tx.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) FROM playlist_items WHERE playlist_id = ?`, playlistID).Scan(&sortOrder); err != nil {
			return err
		}
		for _, id := range ids {
			sortOrder++
			result, err := tx.Exec(`
				INSERT OR IGNORE INTO playlist_items (playlist_id, media_id, sort_order, added_at)
				VALUES (?, ?, ?, ?)`,
				playlistID, id, sortOrder, now)
			if err != nil {
				return err
			}
			added += int(rowsAffected(result))
		}
		_, err := tx.Exec(`UPDATE playlists SET updated_at = ? WHERE id = ?`, now, playlistID)
		return err
	}); err != nil {
		return PlaylistBulkItemsResponse{}, err
	}
	playlist, err := s.getPlaylist(user, playlistID, includeItems)
	if err != nil {
		return PlaylistBulkItemsResponse{}, err
	}
	return PlaylistBulkItemsResponse{
		Playlist: playlist,
		Added:    added,
		Skipped:  len(ids) - added,
		Total:    len(ids),
	}, nil
}

func normalizePlaylistBulkMediaIDs(rawIDs []string) ([]string, error) {
	seen := map[string]bool{}
	ids := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		id := strings.TrimSpace(rawID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.New("Choose at least one media item.")
	}
	if len(ids) > maxBulkMediaItems {
		return nil, fmt.Errorf("Bulk playlist requests are limited to %d media items.", maxBulkMediaItems)
	}
	return ids, nil
}

func (s *Server) removePlaylistItem(user User, playlistID string, mediaID string, includeItems bool) (Playlist, error) {
	current, err := s.getPlaylist(user, playlistID, false)
	if err != nil {
		return Playlist{}, err
	}
	if current.Smart {
		return Playlist{}, errors.New("Smart playlists are populated by filters and cannot be edited manually.")
	}
	if !current.CanEdit {
		return Playlist{}, errors.New("You cannot edit this playlist.")
	}
	_, err = s.execUserWriteTagged(context.Background(), []string{"playlists", "saved"}, `DELETE FROM playlist_items WHERE playlist_id = ? AND media_id = ?`, playlistID, mediaID)
	if err != nil {
		return Playlist{}, err
	}
	_, _ = s.execUserWriteTagged(context.Background(), []string{}, `UPDATE playlists SET updated_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), playlistID)
	return s.getPlaylist(user, playlistID, includeItems)
}

func (s *Server) reorderPlaylistItems(user User, playlistID string, mediaIDs []string, includeItems bool) (Playlist, error) {
	if len(mediaIDs) > maxPlaylistItemsResponse {
		return Playlist{}, fmt.Errorf("Playlist reorder requests are limited to %d media items.", maxPlaylistItemsResponse)
	}
	current, err := s.getPlaylist(user, playlistID, false)
	if err != nil {
		return Playlist{}, err
	}
	if current.Smart {
		return Playlist{}, errors.New("Smart playlists are populated by filters and cannot be reordered manually.")
	}
	if !current.CanEdit {
		return Playlist{}, errors.New("You cannot edit this playlist.")
	}
	var existingCount int
	if err := s.queryUserRow(context.Background(), `SELECT COUNT(*) FROM playlist_items WHERE playlist_id = ?`, playlistID).Scan(&existingCount); err != nil {
		return Playlist{}, err
	}
	if existingCount > maxPlaylistItemsResponse {
		return Playlist{}, fmt.Errorf("Playlist reorder is limited to playlists with %d or fewer items.", maxPlaylistItemsResponse)
	}
	existingRows, err := s.queryUserRead(context.Background(), `SELECT media_id FROM playlist_items WHERE playlist_id = ? ORDER BY sort_order ASC, added_at ASC`, playlistID)
	if err != nil {
		return Playlist{}, err
	}
	existing := []string{}
	existingSet := map[string]bool{}
	for existingRows.Next() {
		var mediaID string
		if err := existingRows.Scan(&mediaID); err != nil {
			_ = existingRows.Close()
			return Playlist{}, err
		}
		existing = append(existing, mediaID)
		existingSet[mediaID] = true
	}
	if err := existingRows.Err(); err != nil {
		_ = existingRows.Close()
		return Playlist{}, err
	}
	if err := existingRows.Close(); err != nil {
		return Playlist{}, err
	}
	if len(existing) == 0 {
		return s.getPlaylist(user, playlistID, includeItems)
	}
	ordered := make([]string, 0, len(existing))
	seen := map[string]bool{}
	for _, mediaID := range mediaIDs {
		mediaID = strings.TrimSpace(mediaID)
		if mediaID == "" || seen[mediaID] || !existingSet[mediaID] {
			continue
		}
		ordered = append(ordered, mediaID)
		seen[mediaID] = true
	}
	for _, mediaID := range existing {
		if !seen[mediaID] {
			ordered = append(ordered, mediaID)
		}
	}
	if len(ordered) != len(existing) {
		return Playlist{}, errors.New("Playlist item order could not be normalized.")
	}
	if err := s.withUserTxTagged(context.Background(), []string{"playlists", "saved"}, func(tx *sql.Tx) error {
		for index, mediaID := range ordered {
			if _, err := tx.Exec(`UPDATE playlist_items SET sort_order = ? WHERE playlist_id = ? AND media_id = ?`, index+1, playlistID, mediaID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`UPDATE playlists SET updated_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), playlistID)
		return err
	}); err != nil {
		return Playlist{}, err
	}
	return s.getPlaylist(user, playlistID, includeItems)
}

func (s *Server) playlistShares(playlistID string) ([]PlaylistShare, error) {
	return s.playlistSharesContext(context.Background(), playlistID)
}

func (s *Server) playlistSharesContext(ctx context.Context, playlistID string) ([]PlaylistShare, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT ps.user_id, u.display_name, u.email, ps.can_edit, ps.created_at, ps.updated_at
		FROM playlist_shares ps
		JOIN users u ON u.id = ps.user_id
		WHERE ps.playlist_id = ?
		ORDER BY lower(u.display_name), lower(u.email)`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shares := []PlaylistShare{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var share PlaylistShare
		var canEdit int
		if err := rows.Scan(&share.UserID, &share.DisplayName, &share.Email, &canEdit, &share.CreatedAt, &share.UpdatedAt); err != nil {
			return nil, err
		}
		share.CanEdit = canEdit != 0
		shares = append(shares, share)
	}
	return shares, rows.Err()
}

func (s *Server) replacePlaylistShares(playlistID string, shares []PlaylistShareRequest) error {
	now := time.Now().UTC().Format(time.RFC3339)
	seen := map[string]bool{}
	return s.withUserTxTagged(context.Background(), []string{"playlists", "saved"}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM playlist_shares WHERE playlist_id = ?`, playlistID); err != nil {
			return err
		}
		var ownerID string
		if err := tx.QueryRow(`SELECT user_id FROM playlists WHERE id = ?`, playlistID).Scan(&ownerID); err != nil {
			return err
		}
		for _, share := range shares {
			userID := strings.TrimSpace(share.UserID)
			if userID == "" || userID == ownerID || seen[userID] {
				continue
			}
			var exists int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				return errors.New("Playlist share target was not found.")
			}
			canEdit := 0
			if share.CanEdit {
				canEdit = 1
			}
			if _, err := tx.Exec(`
				INSERT INTO playlist_shares (playlist_id, user_id, can_edit, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?)`, playlistID, userID, canEdit, now, now); err != nil {
				return err
			}
			seen[userID] = true
		}
		return nil
	})
}

func decodeSmartFilter(raw string) SmartFilter {
	var filter SmartFilter
	if strings.TrimSpace(raw) == "" {
		return filter
	}
	_ = json.Unmarshal([]byte(raw), &filter)
	filter, _, _ = normalizeSmartFilter(filter)
	return filter
}

func normalizeSmartFilter(filter SmartFilter) (SmartFilter, string, error) {
	filter.LibraryID = strings.TrimSpace(filter.LibraryID)
	filter.Type = strings.ToLower(strings.TrimSpace(filter.Type))
	filter.Genre = strings.TrimSpace(filter.Genre)
	filter.Studio = strings.TrimSpace(filter.Studio)
	filter.Sort = strings.TrimSpace(filter.Sort)
	if filter.Sort == "" {
		filter.Sort = "title"
	}
	switch filter.Sort {
	case "title", "added", "year", "release", "critic", "audience", "rating", "contentRating", "unwatched", "lastEpisode", "viewed", "random":
	default:
		filter.Sort = "title"
	}
	if filter.Type != "" {
		switch filter.Type {
		case "movie", "show", "anime", "season", "episode", "album", "track", "audiobook":
		default:
			return SmartFilter{}, "", errors.New("Smart filter media type is not supported.")
		}
	}
	if filter.YearMin < 0 || filter.YearMax < 0 {
		return SmartFilter{}, "", errors.New("Smart filter years cannot be negative.")
	}
	if filter.YearMin > 0 && filter.YearMax > 0 && filter.YearMin > filter.YearMax {
		return SmartFilter{}, "", errors.New("Smart filter minimum year cannot be after maximum year.")
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 250 {
		filter.Limit = 250
	}
	if !filter.Enabled {
		filter = SmartFilter{}
	}
	data, err := json.Marshal(filter)
	if err != nil {
		return SmartFilter{}, "", err
	}
	return filter, string(data), nil
}

func (s *Server) smartPlaylistItems(user User, filter SmartFilter) ([]MediaItem, error) {
	return s.smartPlaylistItemsWithLimit(user, filter, filter.Limit)
}

func (s *Server) smartPlaylistItemsContext(ctx context.Context, user User, filter SmartFilter) ([]MediaItem, error) {
	return s.smartPlaylistItemsWithLimitContext(ctx, user, filter, filter.Limit)
}

func (s *Server) smartPlaylistItemsWithLimit(user User, filter SmartFilter, limit int) ([]MediaItem, error) {
	return s.smartPlaylistItemsWithLimitContext(context.Background(), user, filter, limit)
}

func (s *Server) smartPlaylistItemsWithLimitContext(ctx context.Context, user User, filter SmartFilter, limit int) ([]MediaItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !filter.Enabled {
		return []MediaItem{}, nil
	}
	if limit <= 0 || limit > filter.Limit {
		limit = filter.Limit
	}
	if limit <= 0 {
		limit = 50
	}
	limit = clampInt(limit, 1, 250)
	accessible := map[string]bool{}
	for _, libraryID := range user.LibraryIDs {
		accessible[libraryID] = true
	}
	if len(accessible) == 0 {
		return []MediaItem{}, nil
	}
	libraryIDs := []string{}
	if filter.LibraryID != "" {
		if accessible[filter.LibraryID] {
			libraryIDs = append(libraryIDs, filter.LibraryID)
		}
	} else {
		for libraryID := range accessible {
			libraryIDs = append(libraryIDs, libraryID)
		}
		sort.Strings(libraryIDs)
	}
	cacheKey := smartPlaylistCacheKey(user, filter, libraryIDs, limit)
	if items, ok := s.smartPlaylistCacheGet(cacheKey); ok {
		return items, nil
	}
	items := []MediaItem{}
	candidateBudget := smartPlaylistCandidateBudget(limit, len(libraryIDs))
	var err error
	items, err = s.smartLibraryItemsForLibrariesContext(ctx, user.ID, libraryIDs, filter, candidateBudget)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if filter.Sort == "random" {
		sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	} else {
		sort.SliceStable(items, smartItemLess(items, filter.Sort))
	}
	if len(items) > limit {
		items = items[:limit]
	}
	s.smartPlaylistCacheSet(cacheKey, items)
	return items, nil
}

const smartPlaylistCacheTTL = 15 * time.Second

func smartPlaylistCacheKey(user User, filter SmartFilter, libraryIDs []string, limit int) string {
	if strings.TrimSpace(user.ID) == "" || len(libraryIDs) == 0 {
		return ""
	}
	normalizedLibraryIDs := append([]string(nil), libraryIDs...)
	sort.Strings(normalizedLibraryIDs)
	payload := struct {
		UserID      string      `json:"userId"`
		Role        string      `json:"role"`
		LibraryIDs  []string    `json:"libraryIds"`
		MaxRating   string      `json:"maxRating"`
		Filter      SmartFilter `json:"filter"`
		ResultLimit int         `json:"resultLimit"`
	}{
		UserID:      user.ID,
		Role:        user.Role,
		LibraryIDs:  normalizedLibraryIDs,
		MaxRating:   user.MaxContentRating,
		Filter:      filter,
		ResultLimit: limit,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Server) smartPlaylistCacheGet(key string) ([]MediaItem, bool) {
	if strings.TrimSpace(key) == "" {
		return nil, false
	}
	now := time.Now()
	s.smartPlaylistCacheMu.Lock()
	defer s.smartPlaylistCacheMu.Unlock()
	entry, ok := s.smartPlaylistCache[key]
	if !ok || now.After(entry.expiresAt) {
		if ok {
			delete(s.smartPlaylistCache, key)
		}
		return nil, false
	}
	return append([]MediaItem(nil), entry.items...), true
}

func (s *Server) smartPlaylistCacheSet(key string, items []MediaItem) {
	if strings.TrimSpace(key) == "" {
		return
	}
	s.smartPlaylistCacheMu.Lock()
	defer s.smartPlaylistCacheMu.Unlock()
	if s.smartPlaylistCache == nil {
		s.smartPlaylistCache = map[string]smartPlaylistCacheEntry{}
	}
	s.smartPlaylistCache[key] = smartPlaylistCacheEntry{
		items:     append([]MediaItem(nil), items...),
		expiresAt: time.Now().Add(smartPlaylistCacheTTL),
	}
}

func (s *Server) invalidateSmartPlaylistCache() {
	s.smartPlaylistCacheMu.Lock()
	defer s.smartPlaylistCacheMu.Unlock()
	if len(s.smartPlaylistCache) > 0 {
		s.smartPlaylistCache = map[string]smartPlaylistCacheEntry{}
	}
}

func smartPlaylistPerLibraryLimit(limit int) int {
	return clampInt(limit*2, limit, 250)
}

func smartPlaylistCandidateBudget(limit int, libraryCount int) int {
	if libraryCount <= 1 {
		return smartPlaylistPerLibraryLimit(limit)
	}
	return clampInt(limit*4, limit, 500)
}

func (s *Server) smartLibraryItems(userID, libraryID string, filter SmartFilter, limit int) ([]MediaItem, error) {
	return s.smartLibraryItemsForLibraries(userID, []string{libraryID}, filter, limit)
}

func (s *Server) smartLibraryItemsForLibraries(userID string, libraryIDs []string, filter SmartFilter, limit int) ([]MediaItem, error) {
	return s.smartLibraryItemsForLibrariesContext(context.Background(), userID, libraryIDs, filter, limit)
}

func (s *Server) smartLibraryItemsForLibrariesContext(ctx context.Context, userID string, libraryIDs []string, filter SmartFilter, limit int) ([]MediaItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	normalizedLibraryIDs := make([]string, 0, len(libraryIDs))
	seen := map[string]bool{}
	for _, rawID := range libraryIDs {
		libraryID := strings.TrimSpace(rawID)
		if libraryID == "" || seen[libraryID] {
			continue
		}
		seen[libraryID] = true
		normalizedLibraryIDs = append(normalizedLibraryIDs, libraryID)
	}
	if len(normalizedLibraryIDs) == 0 {
		return []MediaItem{}, nil
	}
	where := "WHERE m.library_id IN (" + sqlPlaceholders(len(normalizedLibraryIDs)) + ")"
	args := make([]any, 0, len(normalizedLibraryIDs)+8)
	for _, libraryID := range normalizedLibraryIDs {
		args = append(args, libraryID)
	}
	if filter.Type == "" {
		where += " AND m.parent_id IS NULL"
	} else {
		where += " AND lower(m.type) = ?"
		args = append(args, filter.Type)
	}
	if filter.Genre != "" {
		where += ` AND EXISTS (
			SELECT 1
			FROM media_category_facets mcf
			WHERE mcf.media_id = m.id
				AND mcf.library_id = m.library_id
				AND mcf.facet_type = 'genre'
				AND mcf.sort_value = ?
		)`
		args = append(args, strings.ToLower(strings.TrimSpace(filter.Genre)))
	}
	if filter.Studio != "" {
		where += ` AND EXISTS (
			SELECT 1
			FROM media_category_facets mcf
			WHERE mcf.media_id = m.id
				AND mcf.library_id = m.library_id
				AND mcf.facet_type = 'studio'
				AND mcf.sort_value = ?
		)`
		args = append(args, strings.ToLower(filter.Studio))
	}
	if filter.YearMin > 0 {
		where += " AND m.year >= ?"
		args = append(args, filter.YearMin)
	}
	if filter.YearMax > 0 {
		where += " AND m.year <= ?"
		args = append(args, filter.YearMax)
	}
	if filter.UnwatchedOnly {
		where += " AND COALESCE(ums.watched, 0) = 0"
	}
	where, args = s.applyMediaVisibilityRestrictionSQL(userID, where, args)
	args = append(args, clampInt(limit, 1, 500))
	return s.queryMediaListItemsContext(ctx, userID, where+" ORDER BY "+smartPlaylistSQLOrder(filter.Sort)+" LIMIT ?", args)
}

func smartPlaylistSQLOrder(sortMode string) string {
	switch sortMode {
	case "added":
		return "m.added_at DESC, m.sort_title ASC, m.id ASC"
	case "year", "release", "lastEpisode":
		return "m.year DESC, m.sort_title ASC, m.id ASC"
	case "critic":
		return "m.critic_rating DESC, m.sort_title ASC, m.id ASC"
	case "audience", "rating":
		return "m.community_rating DESC, m.sort_title ASC, m.id ASC"
	case "contentRating":
		return "m.content_rating ASC, m.sort_title ASC, m.id ASC"
	case "unwatched":
		return "COALESCE(ums.watched, 0) ASC, m.sort_title ASC, m.id ASC"
	case "viewed":
		return "COALESCE(ums.last_played_at, '') DESC, m.sort_title ASC, m.id ASC"
	case "random":
		return "m.random_key ASC, m.id ASC"
	default:
		return "m.sort_title ASC, m.id ASC"
	}
}

func smartItemLess(items []MediaItem, sortMode string) func(i, j int) bool {
	return func(i, j int) bool {
		left, right := items[i], items[j]
		switch sortMode {
		case "added":
			return left.AddedAt > right.AddedAt
		case "year", "release":
			if left.Year != right.Year {
				return left.Year > right.Year
			}
		case "critic":
			if left.CriticRating != right.CriticRating {
				return left.CriticRating > right.CriticRating
			}
		case "audience", "rating":
			if left.CommunityRating != right.CommunityRating {
				return left.CommunityRating > right.CommunityRating
			}
		case "contentRating":
			if left.ContentRating != right.ContentRating {
				return left.ContentRating < right.ContentRating
			}
		case "unwatched":
			if left.State.Watched != right.State.Watched {
				return !left.State.Watched
			}
		case "viewed":
			if left.State.LastPlayedAt != right.State.LastPlayedAt {
				return left.State.LastPlayedAt > right.State.LastPlayedAt
			}
		}
		return left.SortTitle < right.SortTitle
	}
}

func (s *Server) playlistItems(userID, playlistID string) ([]MediaItem, error) {
	return s.playlistItemsContext(context.Background(), userID, playlistID)
}

func (s *Server) playlistItemsContext(ctx context.Context, userID, playlistID string) ([]MediaItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ids, _, err := s.playlistItemIDsPageContext(ctx, playlistID, maxPlaylistItemsResponse, 0)
	if err != nil {
		return nil, err
	}
	return s.mediaListItemsByOrderedIDsContext(ctx, userID, ids)
}

func (s *Server) playlistItemsPageContext(ctx context.Context, user User, playlistID string, limit int, offset int) ([]MediaItem, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit = clampInt(limit, 1, maxPlaylistItemsResponse)
	offset = max(0, offset)
	playlist, err := s.getPlaylistContext(ctx, user, playlistID, false)
	if err != nil {
		return nil, 0, false, err
	}
	if playlist.Smart {
		items, err := s.smartPlaylistItemsWithLimitContext(ctx, user, playlist.SmartFilter, offset+limit+1)
		if err != nil {
			return nil, 0, false, err
		}
		total := len(items)
		hasMore := false
		if offset >= len(items) {
			return []MediaItem{}, total, false, nil
		}
		end := min(len(items), offset+limit)
		hasMore = len(items) > end
		if hasMore {
			total = offset + limit + 1
		}
		return items[offset:end], total, hasMore, nil
	}
	ids, hasMore, err := s.playlistItemIDsPageContext(ctx, playlistID, limit, offset)
	if err != nil {
		return nil, 0, false, err
	}
	items, err := s.mediaListItemsByOrderedIDsContext(ctx, viewerProfileID(user), ids)
	if err != nil {
		return nil, 0, false, err
	}
	return items, playlist.ItemCount, hasMore, nil
}

func (s *Server) playlistItemIDsPageContext(ctx context.Context, playlistID string, limit int, offset int) ([]string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit = clampInt(limit, 1, maxPlaylistItemsResponse)
	offset = max(0, offset)
	rows, err := s.queryUserRead(ctx, `SELECT media_id FROM playlist_items WHERE playlist_id = ? ORDER BY sort_order ASC, added_at ASC LIMIT ? OFFSET ?`, playlistID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	ids := []string{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			_ = rows.Close()
			return nil, false, err
		}
		var mediaID string
		if err := rows.Scan(&mediaID); err != nil {
			_ = rows.Close()
			return nil, false, err
		}
		ids = append(ids, mediaID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, false, err
	}
	if err := rows.Close(); err != nil {
		return nil, false, err
	}
	hasMore := len(ids) > limit
	if hasMore {
		ids = ids[:limit]
	}
	return ids, hasMore, nil
}

func (s *Server) handleDVR(w http.ResponseWriter, r *http.Request, user User) {
	if !canViewDVR(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view DVR.")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/dvr"), "/")
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "DVR route was not found.")
		return
	}
	switch parts[0] {
	case "status":
		s.handleDVROperationalStatus(w, r, user, parts[1:])
	case "rules":
		s.handleDVRRules(w, r, user, parts[1:])
	case "schedule":
		s.handleDVRSchedule(w, r, user, parts[1:])
	case "recording-groups":
		s.handleDVRRecordingGroups(w, r, user, parts[1:])
	case "recordings":
		s.handleDVRRecordings(w, r, user, parts[1:])
	default:
		writeError(w, http.StatusNotFound, "not_found", "DVR route was not found.")
	}
}

func (s *Server) handleDVRSchedule(w http.ResponseWriter, r *http.Request, user User, parts []string) {
	if len(parts) != 0 {
		writeError(w, http.StatusNotFound, "not_found", "DVR schedule route was not found.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	limit := clampInt(queryInt(r, "limit", 50), 1, 100)
	var after dvrRecordingCursor
	err := s.decodeCollectionCursor(r, "dvr-schedule", viewerProfileID(user), time.Now().UTC(), &after)
	if err != nil {
		writeCollectionCursorError(w, err, "DVR schedule")
		return
	}
	recordings, total, hasMore, err := s.listDVRScheduleKeysetPageForUser(r.Context(), user, limit, after)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "dvr_schedule_failed", "Unable to load DVR schedule.")
		return
	}
	applyDVRRecordingsActions(recordings, user)
	var next dvrRecordingCursor
	if hasMore && len(recordings) > 0 {
		last := recordings[len(recordings)-1]
		next = dvrRecordingCursor{StartsAt: last.StartsAt, ID: last.ID}
	}
	pageInfo, err := s.collectionPageInfo("dvr-schedule", viewerProfileID(user), next, hasMore, nil, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "dvr_cursor_failed", "Unable to continue DVR schedule.")
		return
	}
	_ = total
	writeJSON(w, http.StatusOK, CursorListResponse[DVRRecording]{Items: recordings, PageInfo: pageInfo})
}

func (s *Server) handleDVRRules(w http.ResponseWriter, r *http.Request, user User, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			if !canViewDVR(user) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view DVR rules.")
				return
			}
			limit := clampInt(queryInt(r, "limit", 100), 1, 250)
			var after dvrRuleCursor
			cursorErr := s.decodeCollectionCursor(r, "dvr-rules", viewerProfileID(user), time.Now().UTC(), &after)
			if cursorErr != nil {
				writeCollectionCursorError(w, cursorErr, "DVR rules")
				return
			}
			countMode := dvrCountModeFromRequest(r)
			rules, total, hasMore, err := s.listDVRRulesKeysetPageForUser(r.Context(), user, limit, after, countMode)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "dvr_rules_failed", "Unable to load recording rules.")
				return
			}
			applyDVRRulesActions(rules, user)
			var pageTotal *int
			if countMode == "exact" {
				pageTotal = &total
			}
			var next dvrRuleCursor
			if hasMore && len(rules) > 0 {
				last := rules[len(rules)-1]
				next = dvrRuleCursor{UpdatedAt: last.UpdatedAt, ID: last.ID}
			}
			pageInfo, err := s.collectionPageInfo("dvr-rules", viewerProfileID(user), next, hasMore, pageTotal, time.Now().UTC())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "dvr_cursor_failed", "Unable to continue DVR rules.")
				return
			}
			writeJSON(w, http.StatusOK, CursorListResponse[DVRRecordingRule]{Items: rules, PageInfo: pageInfo})
		case http.MethodPost:
			if !canScheduleDVR(user) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to schedule DVR recordings.")
				return
			}
			var req DVRRecordingRuleRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			rule, err := s.createDVRRule(user, req)
			if err != nil {
				writeError(w, http.StatusBadRequest, "dvr_rule_failed", err.Error())
				return
			}
			applyDVRRuleActions(&rule, user)
			writeJSON(w, http.StatusCreated, rule)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
		}
		return
	}
	if len(parts) == 1 {
		ruleID := strings.TrimSpace(parts[0])
		switch r.Method {
		case http.MethodGet:
			rule, err := s.getDVRRuleForUser(user, ruleID)
			if err != nil {
				writeError(w, http.StatusNotFound, "dvr_rule_not_found", "Recording rule was not found.")
				return
			}
			applyDVRRuleActions(&rule, user)
			writeJSON(w, http.StatusOK, rule)
		case http.MethodPatch:
			var patch DVRRecordingRulePatchRequest
			if !decodeJSON(w, r, &patch) {
				return
			}
			current, err := s.getDVRRuleForUser(user, ruleID)
			if err != nil {
				writeError(w, http.StatusNotFound, "dvr_rule_not_found", "Recording rule was not found.")
				return
			}
			if !userCanModifyDVRRule(user, current) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to edit this recording rule.")
				return
			}
			req := dvrRuleRequestFromPatch(current, patch)
			rule, err := s.updateDVRRuleForUser(user, ruleID, req)
			if err != nil {
				if errors.Is(err, errDVRRevisionConflict) {
					writeError(w, http.StatusConflict, "dvr_revision_conflict", "This DVR rule changed elsewhere. Reload it and try again.")
					return
				}
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "dvr_rule_not_found", "Recording rule was not found.")
					return
				}
				writeError(w, http.StatusBadRequest, "dvr_rule_failed", err.Error())
				return
			}
			applyDVRRuleActions(&rule, user)
			writeJSON(w, http.StatusOK, rule)
		case http.MethodDelete:
			current, err := s.getDVRRuleForUser(user, ruleID)
			if err != nil {
				writeError(w, http.StatusNotFound, "dvr_rule_not_found", "Recording rule was not found.")
				return
			}
			if !userCanModifyDVRRule(user, current) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to delete this recording rule.")
				return
			}
			if err := s.deleteDVRRule(ruleID); err != nil {
				writeError(w, http.StatusNotFound, "dvr_rule_not_found", "Recording rule was not found.")
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET, PATCH, or DELETE for this endpoint.")
		}
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "DVR rule route was not found.")
}

func dvrRuleRequestFromPatch(current DVRRecordingRule, patch DVRRecordingRulePatchRequest) DVRRecordingRuleRequest {
	req := DVRRecordingRuleRequest{
		SourceID: current.SourceID, ChannelID: current.ChannelID, ProgramID: current.ProgramID, Title: current.Title,
		MatchType: current.MatchType, Folder: current.Folder, StartPaddingMinutes: current.StartPaddingMinutes,
		EndPaddingMinutes: current.EndPaddingMinutes, RetentionDays: current.RetentionDays,
		MaxRecordingsPerSeries: &current.MaxRecordingsPerSeries, RequiredKeywords: append([]string(nil), current.RequiredKeywords...),
		BlockedKeywords: append([]string(nil), current.BlockedKeywords...), AllowedChannels: append([]string(nil), current.AllowedChannels...),
		BlockedChannels: append([]string(nil), current.BlockedChannels...), Enabled: current.Enabled, Priority: &current.Priority,
		ExpectedRevision: patch.ExpectedRevision,
	}
	if patch.SourceID != nil {
		req.SourceID = *patch.SourceID
	}
	if patch.ChannelID != nil {
		req.ChannelID = *patch.ChannelID
	}
	if patch.ProgramID != nil {
		req.ProgramID = *patch.ProgramID
	}
	if patch.Title != nil {
		req.Title = *patch.Title
	}
	if patch.MatchType != nil {
		req.MatchType = *patch.MatchType
	}
	if patch.Folder != nil {
		req.Folder = *patch.Folder
	}
	if patch.StartPaddingMinutes != nil {
		req.StartPaddingMinutes = *patch.StartPaddingMinutes
	}
	if patch.EndPaddingMinutes != nil {
		req.EndPaddingMinutes = *patch.EndPaddingMinutes
	}
	if patch.RetentionDays != nil {
		req.RetentionDays = *patch.RetentionDays
	}
	if patch.MaxRecordingsPerSeries != nil {
		req.MaxRecordingsPerSeries = patch.MaxRecordingsPerSeries
	}
	if patch.RequiredKeywords != nil {
		req.RequiredKeywords = append([]string(nil), (*patch.RequiredKeywords)...)
	}
	if patch.BlockedKeywords != nil {
		req.BlockedKeywords = append([]string(nil), (*patch.BlockedKeywords)...)
	}
	if patch.AllowedChannels != nil {
		req.AllowedChannels = append([]string(nil), (*patch.AllowedChannels)...)
	}
	if patch.BlockedChannels != nil {
		req.BlockedChannels = append([]string(nil), (*patch.BlockedChannels)...)
	}
	if patch.Enabled != nil {
		req.Enabled = *patch.Enabled
	}
	if patch.Priority != nil {
		req.Priority = patch.Priority
	}
	return req
}

func (s *Server) handleDVRRecordingGroups(w http.ResponseWriter, r *http.Request, user User, parts []string) {
	if len(parts) != 0 {
		writeError(w, http.StatusNotFound, "not_found", "DVR recording group route was not found.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !canViewDVR(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view DVR recordings.")
		return
	}
	limit := clampInt(queryInt(r, "limit", 50), 1, 200)
	var after dvrRecordingGroupCursor
	cursorErr := s.decodeCollectionCursor(r, "dvr-recording-groups", viewerProfileID(user), time.Now().UTC(), &after)
	if cursorErr != nil {
		writeCollectionCursorError(w, cursorErr, "DVR recording groups")
		return
	}
	groups, total, hasMore, err := s.listDVRRecordingGroupsKeysetPageForUser(r.Context(), user, limit, after)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "dvr_recording_groups_failed", "Unable to load recording groups.")
		return
	}
	applyDVRRecordingGroupActions(groups, user)
	var pageTotal *int
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("count")), "exact") {
		exactTotal, countErr := s.countDVRRecordingGroupsForUser(r.Context(), user)
		if countErr != nil {
			writeError(w, http.StatusInternalServerError, "dvr_recording_groups_failed", "Unable to count recording groups.")
			return
		}
		pageTotal = &exactTotal
	}
	_ = total
	var next dvrRecordingGroupCursor
	if hasMore && len(groups) > 0 {
		last := groups[len(groups)-1]
		next = dvrRecordingGroupCursor{LatestRecordingAt: last.LatestRecordingAt, Title: last.Title, Folder: last.CursorFolder}
	}
	pageInfo, err := s.collectionPageInfo("dvr-recording-groups", viewerProfileID(user), next, hasMore, pageTotal, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "dvr_cursor_failed", "Unable to continue DVR recording groups.")
		return
	}
	writeJSON(w, http.StatusOK, CursorListResponse[DVRRecordingGroup]{Items: groups, PageInfo: pageInfo})
}

func (s *Server) handleDVRRecordings(w http.ResponseWriter, r *http.Request, user User, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			if !canViewDVR(user) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view DVR recordings.")
				return
			}
			limit := clampInt(queryInt(r, "limit", 100), 1, 250)
			var after dvrRecordingCursor
			cursorErr := s.decodeCollectionCursor(r, "dvr-recordings", viewerProfileID(user), time.Now().UTC(), &after)
			if cursorErr != nil {
				writeCollectionCursorError(w, cursorErr, "DVR recordings")
				return
			}
			countMode := dvrCountModeFromRequest(r)
			recordings, total, hasMore, err := s.listDVRRecordingsKeysetPageForUser(r.Context(), user, limit, after, countMode)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "dvr_recordings_failed", "Unable to load recordings.")
				return
			}
			applyDVRRecordingsActions(recordings, user)
			var pageTotal *int
			if countMode == "exact" {
				pageTotal = &total
			}
			var next dvrRecordingCursor
			if hasMore && len(recordings) > 0 {
				last := recordings[len(recordings)-1]
				next = dvrRecordingCursor{StartsAt: last.StartsAt, ID: last.ID}
			}
			pageInfo, err := s.collectionPageInfo("dvr-recordings", viewerProfileID(user), next, hasMore, pageTotal, time.Now().UTC())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "dvr_cursor_failed", "Unable to continue DVR recordings.")
				return
			}
			writeJSON(w, http.StatusOK, CursorListResponse[DVRRecording]{Items: recordings, PageInfo: pageInfo})
		case http.MethodPost:
			if !canScheduleDVR(user) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to schedule DVR recordings.")
				return
			}
			var req DVRRecordingRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			recording, err := s.createDVRRecording(user, req)
			if err != nil {
				writeDVRRecordingFailure(w, user, err)
				return
			}
			applyDVRRecordingActions(&recording, user)
			writeJSON(w, http.StatusCreated, recording)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
		}
		return
	}
	if len(parts) == 2 && parts[1] == "stream" {
		s.handleDVRRecordingStream(w, r, user, strings.TrimSpace(parts[0]))
		return
	}
	if len(parts) == 2 && parts[1] == "playback" {
		s.handleDVRRecordingPlayback(w, r, user, strings.TrimSpace(parts[0]))
		return
	}
	if len(parts) == 3 && parts[1] == "hls" {
		s.handleDVRRecordingHLS(w, r, user, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[2]))
		return
	}
	if len(parts) == 1 {
		recordingID := strings.TrimSpace(parts[0])
		switch r.Method {
		case http.MethodGet:
			recording, err := s.getDVRRecordingForUser(user, recordingID)
			if err != nil {
				writeError(w, http.StatusNotFound, "dvr_recording_not_found", "Recording was not found.")
				return
			}
			applyDVRRecordingActions(&recording, user)
			writeJSON(w, http.StatusOK, recording)
		case http.MethodPatch:
			var req DVRRecordingRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			current, err := s.getDVRRecordingForUser(user, recordingID)
			if err != nil {
				writeError(w, http.StatusNotFound, "dvr_recording_not_found", "Recording was not found.")
				return
			}
			if !userCanModifyDVRRecording(user, current) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to edit this recording.")
				return
			}
			recording, err := s.updateDVRRecordingForUser(user, recordingID, req)
			if err != nil {
				if errors.Is(err, errDVRRevisionConflict) {
					writeError(w, http.StatusConflict, "dvr_revision_conflict", "This recording changed elsewhere. Reload it and try again.")
					return
				}
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "dvr_recording_not_found", "Recording was not found.")
					return
				}
				writeDVRRecordingFailure(w, user, err)
				return
			}
			applyDVRRecordingActions(&recording, user)
			writeJSON(w, http.StatusOK, recording)
		case http.MethodDelete:
			current, err := s.getDVRRecordingForUser(user, recordingID)
			if err != nil {
				writeError(w, http.StatusNotFound, "dvr_recording_not_found", "Recording was not found.")
				return
			}
			if !userCanDeleteDVRRecording(user, current) {
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to delete this recording.")
				return
			}
			if err := s.deleteDVRRecording(recordingID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "dvr_recording_not_found", "Recording was not found.")
					return
				}
				writeError(w, http.StatusBadRequest, "dvr_recording_failed", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET, PATCH, or DELETE for this endpoint.")
		}
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "DVR recording route was not found.")
}

func (s *Server) handleDVRRecordingPlayback(w http.ResponseWriter, r *http.Request, user User, recordingID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if !user.Permissions["playMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to play DVR recordings.")
		return
	}
	var req DVRPlaybackSessionCreateRequest
	if r.Body != nil && r.ContentLength != 0 && !decodeJSON(w, r, &req) {
		return
	}
	if req.StartSeconds < 0 {
		writeError(w, http.StatusBadRequest, "invalid_start_position", "startSeconds must be zero or greater.")
		return
	}
	playback, startErr := s.startDVRRecordingPlaybackForRequest(r, user, recordingID, req)
	if startErr != nil {
		if startErr.retryAfter != "" {
			w.Header().Set("Retry-After", startErr.retryAfter)
		}
		writeError(w, startErr.status, startErr.code, startErr.message)
		return
	}
	setPlaybackMediaGrantCookie(w, r, playback)
	writeJSON(w, http.StatusOK, playback)
}

func (s *Server) startDVRRecordingPlaybackForRequest(r *http.Request, user User, recordingID string, req DVRPlaybackSessionCreateRequest) (PlaybackResponse, *playbackStartHTTPError) {
	if !user.Permissions["playMedia"] {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusForbidden, code: "forbidden", message: "You do not have permission to play DVR recordings."}
	}
	if req.StartSeconds < 0 {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusBadRequest, code: "invalid_start_position", message: "startSeconds must be zero or greater."}
	}
	recording, err := s.getDVRRecordingForUser(user, recordingID)
	if err != nil {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusNotFound, code: "dvr_recording_not_found", message: "Recording was not found."}
	}
	if !s.dvrRecordingChannelAllowed(user, recording) {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusNotFound, code: "dvr_recording_not_found", message: "Recording was not found."}
	}
	if !stringSet(dvrRecordingActionsForUser(recording, user))[liveTVActionDVRPlay] {
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusConflict, code: "dvr_recording_not_playable", message: "Recording is not available for playback."}
	}
	if strings.EqualFold(strings.TrimSpace(recording.Status), "running") {
		return s.startLiveTVPlaybackForRequest(r, user, recording.ChannelID, req.ClientProfile, req.Intent, req.ClientInstanceID)
	}

	playback, startErr := s.startPlaybackForRequest(r, user, PlaybackSessionCreateRequest{
		MediaID:          dvrRecordingMediaID(recording.ID),
		VersionID:        req.VersionID,
		ClientInstanceID: req.ClientInstanceID,
		ClientProfile:    req.ClientProfile,
		Intent:           req.Intent,
		SkipPreroll:      true,
		BurnInSubtitleID: req.BurnInSubtitleID,
		SubtitleStreamID: req.SubtitleStreamID,
		AudioStreamID:    req.AudioStreamID,
		StartSeconds:     req.StartSeconds,
		SourceContext: PlaybackSourceContext{
			Type:  "library",
			ID:    "lib_recorded_tv",
			Title: "Recorded TV",
		},
	})
	if startErr != nil {
		return PlaybackResponse{}, startErr
	}
	return playback, nil
}

func (s *Server) handleDVRRecordingStream(w http.ResponseWriter, r *http.Request, user User, recordingID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !user.Permissions["playMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to play DVR recordings.")
		return
	}
	recording, err := s.getDVRRecordingForUser(user, recordingID)
	if err != nil {
		writeError(w, http.StatusNotFound, "dvr_recording_not_found", "Recording was not found.")
		return
	}
	if !s.dvrRecordingChannelAllowed(user, recording) {
		writeProductError(w, http.StatusNotFound, "dvr_recording_not_found", "Recording was not found.")
		return
	}
	if recording.Status == "running" && strings.TrimSpace(recording.ChannelID) != "" {
		writeProductError(w, http.StatusConflict, "dvr_running_playback_session_required", "Open the in-progress recording through DVR playback so Portico can reserve a tuner.")
		return
	}
	if !dvrRecordingFinishedStatus(recording.Status) && recording.Status != "running" {
		writeError(w, http.StatusConflict, "dvr_recording_not_ready", "Recording is not ready to stream.")
		return
	}
	if strings.TrimSpace(recording.Path) == "" {
		writeError(w, http.StatusNotFound, "dvr_recording_file_missing", "Recording file was not found.")
		return
	}
	cleanPath, err := cleanDVRRecordingFilePath(recording.Path, s.cfg.AppDataDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "dvr_recording_path_invalid", "Recording file path is invalid.")
		return
	}
	file, err := os.Open(cleanPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "dvr_recording_file_missing", "Recording file was not found.")
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		writeError(w, http.StatusNotFound, "dvr_recording_file_missing", "Recording file was not found.")
		return
	}
	if contentType := mime.TypeByExtension(filepath.Ext(cleanPath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, filepath.Base(cleanPath), stat.ModTime(), file)
}

func (s *Server) handleDVRRecordingHLS(w http.ResponseWriter, r *http.Request, user User, recordingID string, route string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !user.Permissions["playMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to play DVR recordings.")
		return
	}
	recording, err := s.getDVRRecordingForUser(user, recordingID)
	if err != nil {
		writeError(w, http.StatusNotFound, "dvr_recording_not_found", "Recording was not found.")
		return
	}
	if !s.dvrRecordingChannelAllowed(user, recording) {
		writeProductError(w, http.StatusNotFound, "dvr_recording_not_found", "Recording was not found.")
		return
	}
	if recording.Status == "running" && strings.TrimSpace(recording.ChannelID) != "" {
		writeProductError(w, http.StatusConflict, "dvr_running_playback_session_required", "Open the in-progress recording through DVR playback so Portico can reserve a tuner.")
		return
	}
	if !dvrRecordingFinishedStatus(recording.Status) && recording.Status != "running" {
		writeError(w, http.StatusConflict, "dvr_recording_not_ready", "Recording is not ready to stream.")
		return
	}
	item, err := s.dvrRecordingTranscodeItem(recording)
	if err != nil {
		if errors.Is(err, errDVRRecordingFileMissing) {
			writeError(w, http.StatusNotFound, "dvr_recording_file_missing", "Recording file was not found.")
			return
		}
		writeError(w, http.StatusBadRequest, "dvr_recording_path_invalid", "Recording file path is invalid.")
		return
	}
	switch route {
	case "playlist.m3u8":
		s.handleDVRRecordingHLSManifest(w, r, user, recording.ID, item)
	case "segment":
		s.handleDVRRecordingHLSSegment(w, r, user, recording.ID, item)
	default:
		writeError(w, http.StatusNotFound, "not_found", "DVR HLS route was not found.")
	}
}

func (s *Server) handleDVRRecordingHLSManifest(w http.ResponseWriter, r *http.Request, user User, recordingID string, item MediaItem) {
	session, err := s.ensureTranscodeSession(user.ID, item, "original", "", 0, "", "", false)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "dvr_hls_unavailable", err.Error())
		return
	}
	manifest, err := s.readDVRRecordingHLSManifest(session, recordingID, playbackURLMediaGrant(r))
	if err != nil {
		writeError(w, http.StatusAccepted, "dvr_hls_starting", "The DVR recording HLS session is starting. Retry shortly.")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(manifest))
}

func (s *Server) handleDVRRecordingHLSSegment(w http.ResponseWriter, r *http.Request, user User, recordingID string, item MediaItem) {
	name := filepath.Base(r.URL.Query().Get("name"))
	if name == "." || name == string(filepath.Separator) || !validHLSSegmentName(name) {
		writeError(w, http.StatusBadRequest, "bad_segment", "HLS segment name is invalid.")
		return
	}
	sourcePath, err := s.sourcePathForHLSTranscode(item)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "dvr_hls_unavailable", err.Error())
		return
	}
	startRequest := transcodeStartRequest{userID: user.ID, item: item, sourcePath: sourcePath, quality: "original"}
	for recoveryPass := 0; recoveryPass < 2; recoveryPass++ {
		session, err := s.ensureTranscodeSessionForSegment(user.ID, item, "original", "", 0, "", "", false, name)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "dvr_hls_unavailable", err.Error())
			return
		}
		path := filepath.Join(session.dir, name)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(session.dir)+string(filepath.Separator)) {
			writeError(w, http.StatusBadRequest, "bad_segment", "HLS segment name is invalid.")
			return
		}
		if err := waitForHLSSegmentFileContext(r.Context(), s.shutdownDone(), session, path); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, errLongPollShutdown) {
				return
			}
			if session.snapshot().err != nil {
				if recovered, recoverErr := s.recoverTranscodeSessionForDemandGuarded(r.Context(), s.transcodeSettings(), startRequest, session, err); recoverErr == nil && recovered != nil && recovered != session {
					continue
				}
				writeError(w, http.StatusServiceUnavailable, "dvr_hls_unavailable", session.transcodeError().Error())
				return
			}
			if session.isRunning() {
				w.Header().Set("Retry-After", "2")
				writeError(w, http.StatusServiceUnavailable, "segment_starting", "HLS segment is still being prepared. Retry shortly.")
				return
			}
			writeError(w, http.StatusNotFound, "segment_not_found", "HLS segment is not available.")
			return
		}
		releaseReader, ok := session.acquireReader()
		if !ok {
			continue
		}
		defer releaseReader()
		s.noteTranscodeSegmentServed(session, name)
		w.Header().Set("Content-Type", hlsSegmentContentType(name))
		w.Header().Set("Cache-Control", "private, max-age=30")
		http.ServeFile(w, r, path)
		return
	}
	writeError(w, http.StatusServiceUnavailable, "dvr_hls_unavailable", "The DVR HLS session could not recover in time.")
}

func (s *Server) readDVRRecordingHLSManifest(session *transcodeSession, recordingID string, accessToken string) (string, error) {
	deadline := time.Now().Add(transcodeManifestReadTimeout(false))
	for {
		bytes, err := os.ReadFile(session.manifest)
		if err == nil && len(bytes) > 0 {
			return rewriteDVRRecordingHLSManifest(recordingID, accessToken, string(bytes)), nil
		}
		state := session.snapshot()
		if time.Now().After(deadline) {
			if state.err != nil || state.terminalErr != nil {
				return "", session.transcodeError()
			}
			return "", err
		}
		wait := time.Until(deadline)
		if wait > time.Second {
			wait = time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-session.updateSignal():
			if !timer.Stop() {
				<-timer.C
			}
		case <-session.done:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func rewriteDVRRecordingHLSManifest(recordingID string, accessToken string, manifest string) string {
	var out strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(manifest, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		out.WriteString(dvrRecordingHLSSegmentRoute(recordingID, accessToken, trimmed))
		out.WriteByte('\n')
	}
	return out.String()
}

func dvrRecordingHLSPlaylistRoute(recordingID string, accessToken string) string {
	var out strings.Builder
	out.WriteString("/api/dvr/recordings/")
	out.WriteString(url.PathEscape(recordingID))
	out.WriteString("/hls/playlist.m3u8")
	_ = accessToken
	return out.String()
}

func dvrRecordingHLSSegmentRoute(recordingID string, accessToken string, name string) string {
	var out strings.Builder
	out.WriteString("/api/dvr/recordings/")
	out.WriteString(url.PathEscape(recordingID))
	out.WriteString("/hls/segment?name=")
	out.WriteString(url.QueryEscape(filepath.Base(name)))
	_ = accessToken
	return out.String()
}

var errDVRRecordingFileMissing = errors.New("dvr recording file missing")

func (s *Server) dvrRecordingTranscodeItem(recording DVRRecording) (MediaItem, error) {
	if strings.TrimSpace(recording.Path) == "" {
		return MediaItem{}, errDVRRecordingFileMissing
	}
	cleanPath, err := cleanDVRRecordingFilePath(recording.Path, s.cfg.AppDataDir)
	if err != nil {
		return MediaItem{}, err
	}
	stat, err := os.Stat(cleanPath)
	if err != nil || stat.IsDir() {
		return MediaItem{}, errDVRRecordingFileMissing
	}
	return MediaItem{
		ID:              dvrRecordingTranscodeMediaID(recording.ID),
		Type:            "dvr_recording",
		Title:           recording.Title,
		SourceURL:       cleanPath,
		DurationSeconds: dvrRecordingDurationSeconds(recording),
	}, nil
}

func dvrRecordingTranscodeMediaID(recordingID string) string {
	return "dvr_recording_" + strings.TrimSpace(recordingID)
}

func dvrRecordingDurationSeconds(recording DVRRecording) int {
	start, errStart := time.Parse(time.RFC3339, strings.TrimSpace(recording.StartsAt))
	end, errEnd := time.Parse(time.RFC3339, strings.TrimSpace(recording.EndsAt))
	if errStart != nil || errEnd != nil || !end.After(start) {
		return 0
	}
	return max(0, int(end.Sub(start).Seconds()))
}

func normalizeRecordingMatchType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "series":
		return "series"
	default:
		return "single"
	}
}

type dvrTimerDefaults struct {
	StartPaddingMinutes         int
	EndPaddingMinutes           int
	RetentionDays               int
	Folder                      string
	MaxRecordingsPerSeries      int
	DefaultRuleRequiredKeywords []string
	DefaultRuleBlockedKeywords  []string
	DefaultRuleAllowedChannels  []string
	DefaultRuleBlockedChannels  []string
	RecordingPathTemplate       string
	SaveNFO                     bool
	SaveImageSidecars           bool
	ConvertRecordings           bool
	RecordingProfile            string
	PreserveAllStreams          bool
}

func (s *Server) dvrTimerDefaults() dvrTimerDefaults {
	defaults := dvrTimerDefaults{RetentionDays: 30, RecordingPathTemplate: defaultDVRRecordingPathTemplate, RecordingProfile: "copy", PreserveAllStreams: true}
	settings, err := s.loadSettings()
	if err != nil {
		return defaults
	}
	group, _ := settings["dvr"].(map[string]any)
	defaults.StartPaddingMinutes = max(0, settingInt(group, "defaultStartPaddingMinutes", defaults.StartPaddingMinutes))
	defaults.EndPaddingMinutes = max(0, settingInt(group, "defaultEndPaddingMinutes", defaults.EndPaddingMinutes))
	defaults.RetentionDays = max(1, settingInt(group, "defaultRetentionDays", defaults.RetentionDays))
	defaults.Folder = normalizeDVRFolder(settingString(group, "defaultFolder", defaults.Folder))
	defaults.MaxRecordingsPerSeries = max(0, settingInt(group, "defaultMaxRecordingsPerSeries", defaults.MaxRecordingsPerSeries))
	defaults.DefaultRuleRequiredKeywords = normalizeLiveTVList(settingStringList(group, "defaultRuleRequiredKeywords"), 120)
	defaults.DefaultRuleBlockedKeywords = normalizeLiveTVList(settingStringList(group, "defaultRuleBlockedKeywords"), 120)
	defaults.DefaultRuleAllowedChannels = normalizeLiveTVList(settingStringList(group, "defaultRuleAllowedChannels"), 120)
	defaults.DefaultRuleBlockedChannels = normalizeLiveTVList(settingStringList(group, "defaultRuleBlockedChannels"), 120)
	defaults.RecordingPathTemplate = normalizeDVRRecordingPathTemplate(settingString(group, "recordingPathTemplate", defaults.RecordingPathTemplate))
	defaults.SaveNFO = settingBool(group, "saveNFO", defaults.SaveNFO)
	defaults.SaveImageSidecars = settingBool(group, "saveImageSidecars", defaults.SaveImageSidecars)
	defaults.ConvertRecordings = settingBool(group, "convertRecordings", defaults.ConvertRecordings)
	defaults.RecordingProfile = normalizeDVRRecordingProfile(settingString(group, "recordingProfile", defaults.RecordingProfile))
	defaults.PreserveAllStreams = settingBool(group, "preserveAllStreams", defaults.PreserveAllStreams)
	return defaults
}

const defaultDVRRecordingPathTemplate = "{folder}/{year}/{month}/{title}-{start}"

func normalizeDVRRecordingProfile(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "h264-aac", "h264-1080p-8m":
		return "h264-1080p-8m"
	case "h264-720p-4m":
		return "h264-720p-4m"
	default:
		return "copy"
	}
}

func normalizeDVRRecordingPathTemplate(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return defaultDVRRecordingPathTemplate
	}
	parts := strings.Split(value, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		cleaned = append(cleaned, part)
	}
	if len(cleaned) == 0 {
		return defaultDVRRecordingPathTemplate
	}
	normalized := strings.Join(cleaned, "/")
	if len(normalized) > 240 {
		return normalized[:240]
	}
	return normalized
}

func normalizeDVRFolder(value string) string {
	clean := strings.NewReplacer("/", " ", "\\", " ").Replace(strings.TrimSpace(value))
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > 80 {
		clean = strings.TrimSpace(clean[:80])
	}
	return clean
}

func (s *Server) applyDVRRuleDefaults(req DVRRecordingRuleRequest) DVRRecordingRuleRequest {
	defaults := s.dvrTimerDefaults()
	req.Folder = normalizeDVRFolder(req.Folder)
	if req.Folder == "" {
		req.Folder = defaults.Folder
	}
	if req.StartPaddingMinutes <= 0 {
		req.StartPaddingMinutes = defaults.StartPaddingMinutes
	}
	if req.EndPaddingMinutes <= 0 {
		req.EndPaddingMinutes = defaults.EndPaddingMinutes
	}
	if req.RetentionDays <= 0 {
		req.RetentionDays = defaults.RetentionDays
	}
	if req.MaxRecordingsPerSeries == nil {
		value := defaults.MaxRecordingsPerSeries
		req.MaxRecordingsPerSeries = &value
	} else if *req.MaxRecordingsPerSeries < 0 {
		value := 0
		req.MaxRecordingsPerSeries = &value
	}
	if req.RequiredKeywords == nil {
		req.RequiredKeywords = defaults.DefaultRuleRequiredKeywords
	}
	if req.BlockedKeywords == nil {
		req.BlockedKeywords = defaults.DefaultRuleBlockedKeywords
	}
	if req.AllowedChannels == nil {
		req.AllowedChannels = defaults.DefaultRuleAllowedChannels
	}
	if req.BlockedChannels == nil {
		req.BlockedChannels = defaults.DefaultRuleBlockedChannels
	}
	return req
}

func (s *Server) applyDVRRecordingDefaults(req DVRRecordingRequest) DVRRecordingRequest {
	defaults := s.dvrTimerDefaults()
	req.Folder = normalizeDVRFolder(req.Folder)
	if req.Folder == "" {
		req.Folder = defaults.Folder
	}
	if strings.TrimSpace(req.RuleID) != "" {
		return req
	}
	start, startErr := time.Parse(time.RFC3339, strings.TrimSpace(req.StartsAt))
	end, endErr := time.Parse(time.RFC3339, strings.TrimSpace(req.EndsAt))
	if startErr == nil && endErr == nil && end.After(start) {
		req.StartsAt = start.Add(-time.Duration(defaults.StartPaddingMinutes) * time.Minute).UTC().Format(time.RFC3339)
		req.EndsAt = end.Add(time.Duration(defaults.EndPaddingMinutes) * time.Minute).UTC().Format(time.RFC3339)
	}
	return req
}

func (s *Server) listDVRRules() ([]DVRRecordingRule, error) {
	rows, err := s.queryUserRead(context.Background(), `
		SELECT id, user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title, match_type,
			COALESCE(folder, ''), start_padding_minutes, end_padding_minutes, retention_days,
			max_recordings_per_series, required_keywords, blocked_keywords, allowed_channels, blocked_channels,
			enabled, priority, revision, created_at, updated_at
		FROM live_tv_recording_rules
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []DVRRecordingRule{}
	for rows.Next() {
		rule, err := scanDVRRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func dvrCountModeFromRequest(r *http.Request) string {
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("count")), "exact") {
		return "exact"
	}
	return "none"
}

type dvrRuleCursor struct {
	UpdatedAt string `json:"updatedAt"`
	ID        string `json:"id"`
}

type dvrRecordingCursor struct {
	StartsAt string `json:"startsAt"`
	ID       string `json:"id"`
}

type dvrRecordingGroupCursor struct {
	LatestRecordingAt string `json:"latestRecordingAt"`
	Title             string `json:"title"`
	Folder            string `json:"folder"`
}

func appendDVRKeysetCondition(where, condition string) string {
	if strings.TrimSpace(where) == "" {
		return "WHERE " + condition
	}
	return where + " AND " + condition
}

func (s *Server) listDVRRulesKeysetPageForUser(ctx context.Context, user User, limit int, after dvrRuleCursor, countMode string) ([]DVRRecordingRule, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	where := "WHERE profile_id = ?"
	args := []any{viewerProfileID(user)}
	countWhere := where
	countArgs := append([]any{}, args...)
	if after.ID != "" {
		where = appendDVRKeysetCondition(where, "(updated_at < ? OR (updated_at = ? AND id > ?))")
		args = append(args, after.UpdatedAt, after.UpdatedAt, after.ID)
	}
	total := 0
	if countMode == "exact" {
		if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM live_tv_recording_rules `+countWhere, countArgs...).Scan(&total); err != nil {
			return nil, 0, false, err
		}
	}
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT id, user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title, match_type,
			COALESCE(folder, ''), start_padding_minutes, end_padding_minutes, retention_days,
			max_recordings_per_series, required_keywords, blocked_keywords, allowed_channels, blocked_channels,
			enabled, priority, revision, created_at, updated_at
		FROM live_tv_recording_rules `+where+`
		ORDER BY updated_at DESC, id ASC LIMIT ?`, args...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	rules := []DVRRecordingRule{}
	for rows.Next() {
		rule, scanErr := scanDVRRule(rows)
		if scanErr != nil {
			return nil, 0, false, scanErr
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	hasMore := len(rules) > limit
	if hasMore {
		rules = rules[:limit]
	}
	if countMode != "exact" {
		total = len(rules) + boolInt(hasMore)
	}
	return rules, total, hasMore, nil
}

func (s *Server) listDVRRulesPageForUser(ctx context.Context, user User, limit int, offset int, countMode string) ([]DVRRecordingRule, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	where := "WHERE profile_id = ?"
	args := []any{viewerProfileID(user)}
	total := 0
	if countMode == "exact" {
		if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM live_tv_recording_rules `+where, args...).Scan(&total); err != nil {
			return nil, 0, false, err
		}
	}
	queryLimit := limit + 1
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, queryLimit, offset)
	rows, err := s.queryUserRead(ctx, `
		SELECT id, user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title, match_type,
			COALESCE(folder, ''), start_padding_minutes, end_padding_minutes, retention_days,
			max_recordings_per_series, required_keywords, blocked_keywords, allowed_channels, blocked_channels,
			enabled, priority, revision, created_at, updated_at
		FROM live_tv_recording_rules
		`+where+`
		ORDER BY updated_at DESC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	rules := []DVRRecordingRule{}
	for rows.Next() {
		rule, err := scanDVRRule(rows)
		if err != nil {
			return nil, 0, false, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	hasMore := len(rules) > limit
	if hasMore {
		rules = rules[:limit]
	}
	if countMode != "exact" {
		total = offset + len(rules) + boolInt(hasMore)
	}
	return rules, total, hasMore, nil
}

func (s *Server) getDVRRule(ruleID string) (DVRRecordingRule, error) {
	ruleID = strings.TrimSpace(ruleID)
	row := s.queryUserRow(context.Background(), `
		SELECT id, user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title, match_type,
			COALESCE(folder, ''), start_padding_minutes, end_padding_minutes, retention_days,
			max_recordings_per_series, required_keywords, blocked_keywords, allowed_channels, blocked_channels,
			enabled, priority, revision, created_at, updated_at
		FROM live_tv_recording_rules
		WHERE id = ?`, ruleID)
	return scanDVRRule(row)
}

type dvrRuleScanner interface {
	Scan(dest ...any) error
}

func scanDVRRule(scanner dvrRuleScanner) (DVRRecordingRule, error) {
	var rule DVRRecordingRule
	var enabled int
	var requiredKeywords string
	var blockedKeywords string
	var allowedChannels string
	var blockedChannels string
	if err := scanner.Scan(&rule.ID, &rule.UserID, &rule.ProfileID, &rule.SourceID, &rule.ChannelID, &rule.ProgramID, &rule.Title, &rule.MatchType, &rule.Folder, &rule.StartPaddingMinutes, &rule.EndPaddingMinutes, &rule.RetentionDays, &rule.MaxRecordingsPerSeries, &requiredKeywords, &blockedKeywords, &allowedChannels, &blockedChannels, &enabled, &rule.Priority, &rule.Revision, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
		return DVRRecordingRule{}, err
	}
	rule.RequiredKeywords = decodeLiveTVList(requiredKeywords)
	rule.BlockedKeywords = decodeLiveTVList(blockedKeywords)
	rule.AllowedChannels = decodeLiveTVList(allowedChannels)
	rule.BlockedChannels = decodeLiveTVList(blockedChannels)
	rule.Enabled = enabled == 1
	return rule, nil
}

func (s *Server) listDVRRulesForUser(user User) ([]DVRRecordingRule, error) {
	rules, _, _, err := s.listDVRRulesPageForUser(context.Background(), user, 200, 0, "none")
	return rules, err
}

func (s *Server) getDVRRuleForUser(user User, ruleID string) (DVRRecordingRule, error) {
	rule, err := s.getDVRRule(ruleID)
	if err != nil {
		return DVRRecordingRule{}, err
	}
	if dvrRuleOwnerProfileID(rule) != viewerProfileID(user) {
		return DVRRecordingRule{}, sql.ErrNoRows
	}
	return rule, nil
}

func dvrRuleOwnerProfileID(rule DVRRecordingRule) string {
	if profileID := strings.TrimSpace(rule.ProfileID); profileID != "" {
		return profileID
	}
	return strings.TrimSpace(rule.UserID)
}

func userCanModifyDVRRule(user User, rule DVRRecordingRule) bool {
	return dvrRuleOwnerProfileID(rule) == viewerProfileID(user) && canScheduleDVR(user)
}

func (s *Server) validateDVRRuleRecordingBinding(user User, ruleID, sourceID, channelID, programID string, allowSeriesProgram bool) error {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return nil
	}
	rule, err := s.getDVRRuleForUser(user, ruleID)
	if err != nil || rule.SourceID != sourceID || strings.TrimSpace(rule.ChannelID) != "" && rule.ChannelID != channelID || strings.TrimSpace(rule.ProgramID) != "" && rule.ProgramID != programID && (!allowSeriesProgram || rule.MatchType != "series") {
		return errDVRLiveTVReferenceDenied
	}
	return nil
}

func validateDVRRuleRecordingBindingTx(tx *sql.Tx, profileID, ruleID, sourceID, channelID, programID string, allowSeriesProgram bool) error {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return nil
	}
	var valid int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM live_tv_recording_rules
		WHERE id = ? AND profile_id = ? AND source_id = ?
			AND (COALESCE(channel_id, '') = '' OR channel_id = ?)
			AND (program_id = '' OR program_id = ? OR (? = 1 AND match_type = 'series'))`,
		ruleID, profileID, sourceID, channelID, programID, boolInt(allowSeriesProgram)).Scan(&valid); err != nil || valid != 1 {
		return errDVRLiveTVReferenceDenied
	}
	return nil
}

func (s *Server) createDVRRule(user User, req DVRRecordingRuleRequest) (DVRRecordingRule, error) {
	req = s.applyDVRRuleDefaults(req)
	req.SourceID = strings.TrimSpace(req.SourceID)
	req.Title = strings.TrimSpace(req.Title)
	if req.SourceID == "" || req.Title == "" {
		return DVRRecordingRule{}, errors.New("Source and title are required.")
	}
	ref, err := s.resolveAuthorizedDVRLiveTVReference(context.Background(), user, req.SourceID, req.ChannelID, req.ProgramID, normalizeRecordingMatchType(req.MatchType) == "series", false)
	if err != nil {
		return DVRRecordingRule{}, err
	}
	req.SourceID, req.ChannelID, req.ProgramID = ref.SourceID, ref.ChannelID, ref.ProgramID
	now := time.Now().UTC().Format(time.RFC3339)
	id := randomID("dvr")
	_, err = s.execUserWrite(context.Background(), `
		INSERT INTO live_tv_recording_rules (
			id, user_id, profile_id, source_id, channel_id, program_id, title, match_type, folder, start_padding_minutes,
			end_padding_minutes, retention_days, max_recordings_per_series, required_keywords, blocked_keywords,
			allowed_channels, blocked_channels, enabled, priority, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, accountIDForUser(user), viewerProfileID(user), req.SourceID, strings.TrimSpace(req.ChannelID), strings.TrimSpace(req.ProgramID), req.Title,
		normalizeRecordingMatchType(req.MatchType), req.Folder, max(0, req.StartPaddingMinutes), max(0, req.EndPaddingMinutes), max(1, req.RetentionDays),
		max(0, intValue(req.MaxRecordingsPerSeries, 0)), encodeLiveTVList(req.RequiredKeywords), encodeLiveTVList(req.BlockedKeywords),
		encodeLiveTVList(req.AllowedChannels), encodeLiveTVList(req.BlockedChannels), boolInt(req.Enabled), boundedDVRPriority(intValue(req.Priority, 50)), now, now)
	if err != nil {
		return DVRRecordingRule{}, err
	}
	s.createDVRRecordingFromRule(user, id, req)
	return s.getDVRRule(id)
}

func (s *Server) updateDVRRule(ruleID string, req DVRRecordingRuleRequest) (DVRRecordingRule, error) {
	current, err := s.getDVRRule(ruleID)
	if err != nil {
		return DVRRecordingRule{}, err
	}
	user, err := s.currentDVRUser(current.UserID, current.ProfileID)
	if err != nil {
		return DVRRecordingRule{}, err
	}
	return s.updateDVRRuleForUser(user, ruleID, req)
}

func (s *Server) updateDVRRuleForUser(user User, ruleID string, req DVRRecordingRuleRequest) (DVRRecordingRule, error) {
	current, err := s.getDVRRule(ruleID)
	if err != nil {
		return DVRRecordingRule{}, err
	}
	sourceID := firstNonEmpty(strings.TrimSpace(req.SourceID), current.SourceID)
	title := firstNonEmpty(strings.TrimSpace(req.Title), current.Title)
	if sourceID == "" || title == "" {
		return DVRRecordingRule{}, errors.New("Source and title are required.")
	}
	channelID := firstNonEmpty(strings.TrimSpace(req.ChannelID), current.ChannelID)
	programID := firstNonEmpty(strings.TrimSpace(req.ProgramID), current.ProgramID)
	matchType := normalizeRecordingMatchType(firstNonEmpty(req.MatchType, current.MatchType))
	ref, err := s.resolveAuthorizedDVRLiveTVReference(context.Background(), user, sourceID, channelID, programID, matchType == "series", false)
	if err != nil {
		return DVRRecordingRule{}, err
	}
	sourceID, channelID, programID = ref.SourceID, ref.ChannelID, ref.ProgramID
	retentionDays := req.RetentionDays
	if retentionDays <= 0 {
		retentionDays = current.RetentionDays
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.execUserWrite(context.Background(), `
		UPDATE live_tv_recording_rules
		SET source_id = ?, channel_id = NULLIF(?, ''), program_id = ?, title = ?, match_type = ?, folder = ?,
			start_padding_minutes = ?, end_padding_minutes = ?, retention_days = ?, max_recordings_per_series = ?,
			required_keywords = ?, blocked_keywords = ?, allowed_channels = ?, blocked_channels = ?, enabled = ?, priority = ?, revision = revision + 1, updated_at = ?
		WHERE id = ? AND (? IS NULL OR revision = ?)`,
		sourceID, channelID, programID,
		title, matchType, normalizeDVRFolder(firstNonEmpty(req.Folder, current.Folder)), max(0, req.StartPaddingMinutes), max(0, req.EndPaddingMinutes),
		max(1, retentionDays), max(0, intValue(req.MaxRecordingsPerSeries, current.MaxRecordingsPerSeries)),
		encodeLiveTVList(firstNonNilStringList(req.RequiredKeywords, current.RequiredKeywords)),
		encodeLiveTVList(firstNonNilStringList(req.BlockedKeywords, current.BlockedKeywords)),
		encodeLiveTVList(firstNonNilStringList(req.AllowedChannels, current.AllowedChannels)),
		encodeLiveTVList(firstNonNilStringList(req.BlockedChannels, current.BlockedChannels)),
		boolInt(req.Enabled), boundedDVRPriority(intValue(req.Priority, current.Priority)), now, ruleID, req.ExpectedRevision, req.ExpectedRevision)
	if err != nil {
		return DVRRecordingRule{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return DVRRecordingRule{}, errDVRRevisionConflict
	}
	return s.getDVRRule(ruleID)
}

func (s *Server) createDVRRecordingFromRule(user User, ruleID string, req DVRRecordingRuleRequest) {
	if normalizeRecordingMatchType(req.MatchType) == "series" {
		s.createDVRSeriesRecordingsFromRule(user, ruleID, req)
		return
	}
	if strings.TrimSpace(req.ProgramID) == "" {
		return
	}
	s.createDVRSingleProgramRecording(user, ruleID, req.ProgramID)
}

func (s *Server) createDVRSingleProgramRecording(user User, ruleID string, programID string) {
	var program LiveTVProgram
	err := s.queryUserRow(context.Background(), `
		SELECT id, source_id, COALESCE(channel_id, ''), title, start_at, end_at
		FROM live_tv_programs
		WHERE id = ?`, strings.TrimSpace(programID)).Scan(&program.ID, &program.SourceID, &program.ChannelID, &program.Title, &program.StartAt, &program.EndAt)
	if err != nil {
		return
	}
	start, _ := time.Parse(time.RFC3339, program.StartAt)
	end, _ := time.Parse(time.RFC3339, program.EndAt)
	rule, err := s.getDVRRule(ruleID)
	if err == nil && !start.IsZero() && !end.IsZero() {
		program.StartAt = start.Add(-time.Duration(rule.StartPaddingMinutes) * time.Minute).UTC().Format(time.RFC3339)
		program.EndAt = end.Add(time.Duration(rule.EndPaddingMinutes) * time.Minute).UTC().Format(time.RFC3339)
	}
	_, _ = s.createDVRRecording(user, DVRRecordingRequest{
		RuleID:    ruleID,
		SourceID:  program.SourceID,
		ChannelID: program.ChannelID,
		ProgramID: program.ID,
		Title:     program.Title,
		Folder:    rule.Folder,
		StartsAt:  program.StartAt,
		EndsAt:    program.EndAt,
		Priority:  &rule.Priority,
	})
}

func (s *Server) createDVRSeriesRecordingsFromRule(user User, ruleID string, req DVRRecordingRuleRequest) {
	title := strings.TrimSpace(req.Title)
	if req.ProgramID != "" {
		_ = s.queryUserRow(context.Background(), `SELECT title FROM live_tv_programs WHERE id = ?`, strings.TrimSpace(req.ProgramID)).Scan(&title)
	}
	if title == "" {
		return
	}
	args := []any{req.SourceID, strings.ToLower(title), time.Now().UTC().Format(time.RFC3339)}
	where := `WHERE p.source_id = ? AND lower(p.title) = ? AND p.end_at > ?`
	if strings.TrimSpace(req.ChannelID) != "" {
		where += ` AND p.channel_id = ?`
		args = append(args, strings.TrimSpace(req.ChannelID))
	}
	rows, err := s.queryUserRead(context.Background(), `
		SELECT p.id, p.source_id, COALESCE(p.channel_id, ''), p.title, p.subtitle, p.description, p.category, p.start_at, p.end_at,
			COALESCE(c.name, ''), COALESCE(c.number, '')
		FROM live_tv_programs p
		LEFT JOIN live_tv_channels c ON c.id = p.channel_id
		`+where+`
		ORDER BY p.start_at ASC
		LIMIT 100`, args...)
	if err != nil {
		return
	}
	type dvrProgramCandidate struct {
		program       LiveTVProgram
		channelName   string
		channelNumber string
	}
	programs := []dvrProgramCandidate{}
	for rows.Next() {
		var program LiveTVProgram
		var candidate dvrProgramCandidate
		if err := rows.Scan(&program.ID, &program.SourceID, &program.ChannelID, &program.Title, &program.Subtitle, &program.Description, &program.Category, &program.StartAt, &program.EndAt, &candidate.channelName, &candidate.channelNumber); err != nil {
			_ = rows.Close()
			return
		}
		candidate.program = program
		programs = append(programs, candidate)
	}
	if rows.Err() != nil {
		_ = rows.Close()
		return
	}
	if err := rows.Close(); err != nil {
		return
	}
	for _, candidate := range programs {
		if !dvrProgramMatchesRuleFilters(candidate.program, candidate.channelName, candidate.channelNumber, req) {
			continue
		}
		program := candidate.program
		if _, err := s.resolveAuthorizedDVRLiveTVReference(context.Background(), user, program.SourceID, program.ChannelID, program.ID, false, true); err != nil {
			continue
		}
		start, _ := time.Parse(time.RFC3339, program.StartAt)
		end, _ := time.Parse(time.RFC3339, program.EndAt)
		if !start.IsZero() && !end.IsZero() {
			program.StartAt = start.Add(-time.Duration(req.StartPaddingMinutes) * time.Minute).UTC().Format(time.RFC3339)
			program.EndAt = end.Add(time.Duration(req.EndPaddingMinutes) * time.Minute).UTC().Format(time.RFC3339)
		}
		_, _ = s.createDVRRecordingBound(user, DVRRecordingRequest{
			RuleID:    ruleID,
			SourceID:  program.SourceID,
			ChannelID: program.ChannelID,
			ProgramID: program.ID,
			Title:     program.Title,
			Folder:    req.Folder,
			StartsAt:  program.StartAt,
			EndsAt:    program.EndAt,
			Priority:  req.Priority,
		}, true)
	}
}

func (s *Server) reconcileDVRSeriesRulesForSource(sourceID string) {
	rows, err := s.queryBackgroundRead(context.Background(), `
		SELECT id, user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title, match_type,
			COALESCE(folder, ''), start_padding_minutes, end_padding_minutes, retention_days,
			max_recordings_per_series, required_keywords, blocked_keywords, allowed_channels, blocked_channels,
			enabled, priority, revision, created_at, updated_at
		FROM live_tv_recording_rules
		WHERE source_id = ? AND enabled = 1 AND match_type = 'series'
		ORDER BY priority DESC, id ASC`, sourceID)
	if err != nil {
		s.log.Warn("DVR series rule reconciliation failed", "source", sourceID, "error", err)
		return
	}
	rules := []DVRRecordingRule{}
	for rows.Next() {
		rule, scanErr := scanDVRRule(rows)
		if scanErr != nil {
			s.log.Warn("DVR series rule reconciliation scan failed", "source", sourceID, "error", scanErr)
			_ = rows.Close()
			return
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		s.log.Warn("DVR series rule reconciliation rows failed", "source", sourceID, "error", err)
		return
	}
	if err := rows.Close(); err != nil {
		s.log.Warn("DVR series rule reconciliation close failed", "source", sourceID, "error", err)
		return
	}
	for _, rule := range rules {
		user, allowed := s.dvrReconciliationUser(rule)
		if !allowed {
			s.disableUnauthorizedDVRSeriesRule(rule)
			continue
		}
		if _, err := s.resolveAuthorizedDVRLiveTVReference(context.Background(), user, rule.SourceID, rule.ChannelID, rule.ProgramID, true, false); err != nil {
			if rule.ChannelID != "" || rule.ProgramID != "" {
				s.disableUnauthorizedDVRSeriesRule(rule)
			}
			continue
		}
		s.pruneUnauthorizedScheduledDVRRecordings(user, rule.ID)
		priority := rule.Priority
		req := DVRRecordingRuleRequest{
			SourceID: rule.SourceID, ChannelID: rule.ChannelID, ProgramID: rule.ProgramID, Title: rule.Title,
			MatchType: rule.MatchType, Folder: rule.Folder, StartPaddingMinutes: rule.StartPaddingMinutes,
			EndPaddingMinutes: rule.EndPaddingMinutes, RetentionDays: rule.RetentionDays,
			MaxRecordingsPerSeries: &rule.MaxRecordingsPerSeries, RequiredKeywords: rule.RequiredKeywords,
			BlockedKeywords: rule.BlockedKeywords, AllowedChannels: rule.AllowedChannels, BlockedChannels: rule.BlockedChannels,
			Enabled: rule.Enabled, Priority: &priority,
		}
		s.createDVRSeriesRecordingsFromRule(user, rule.ID, req)
	}
}

func (s *Server) dvrReconciliationUser(rule DVRRecordingRule) (User, bool) {
	ctx := context.Background()
	principal, err := s.resolveRequestPrincipalContext(ctx, rule.UserID, rule.ProfileID)
	if err != nil {
		return User{}, false
	}
	user := User{ID: rule.UserID, AccountID: rule.UserID, ProfileID: rule.ProfileID}
	applyRequestPrincipal(&user, principal)
	user = s.hydratePlaybackVisibilityUserContext(ctx, user)
	if !canScheduleDVR(user) || !canViewDVR(user) {
		return User{}, false
	}
	return user, true
}

func (s *Server) disableUnauthorizedDVRSeriesRule(rule DVRRecordingRule) {
	now := time.Now().UTC().Format(time.RFC3339)
	err := s.withBackgroundTxTagged(context.Background(), []string{"dvr", "live-tv"}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`
			UPDATE live_tv_recording_rules
			SET enabled = 0, revision = revision + 1, updated_at = ?
			WHERE id = ? AND enabled = 1`, now, rule.ID); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM live_tv_recordings WHERE rule_id = ? AND status = 'scheduled'`, rule.ID)
		return err
	})
	if err != nil {
		s.log.Warn("unauthorized DVR series rule could not be disabled", "rule", rule.ID, "profile", rule.ProfileID, "error", err)
		return
	}
	s.recordLog("info", "DVR series rule disabled after permission changed.", map[string]string{"rule": rule.ID, "profile": rule.ProfileID})
}

func dvrProgramMatchesRuleFilters(program LiveTVProgram, channelName string, channelNumber string, req DVRRecordingRuleRequest) bool {
	text := strings.ToLower(strings.Join([]string{
		program.Title,
		program.Subtitle,
		program.Description,
		program.Category,
		channelName,
		channelNumber,
		program.ChannelID,
	}, " "))
	required := normalizeLiveTVList(req.RequiredKeywords, 120)
	if len(required) > 0 {
		matched := false
		for _, keyword := range required {
			if strings.Contains(text, strings.ToLower(keyword)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, keyword := range normalizeLiveTVList(req.BlockedKeywords, 120) {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return false
		}
	}
	if !dvrChannelAllowedByFilters(program.ChannelID, channelName, channelNumber, req.AllowedChannels, req.BlockedChannels) {
		return false
	}
	return true
}

func dvrChannelAllowedByFilters(channelID string, channelName string, channelNumber string, allowed []string, blocked []string) bool {
	tokens := []string{channelID, channelName, channelNumber}
	for _, value := range normalizeLiveTVList(blocked, 120) {
		if dvrChannelFilterMatches(tokens, value) {
			return false
		}
	}
	allowed = normalizeLiveTVList(allowed, 120)
	if len(allowed) == 0 {
		return true
	}
	for _, value := range allowed {
		if dvrChannelFilterMatches(tokens, value) {
			return true
		}
	}
	return false
}

func dvrChannelFilterMatches(tokens []string, filter string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return false
	}
	for _, token := range tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token != "" && token == filter {
			return true
		}
	}
	return false
}

func intValue(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func boundedDVRPriority(value int) int {
	return max(0, min(100, value))
}

func firstNonNilStringList(value []string, fallback []string) []string {
	if value == nil {
		return fallback
	}
	return value
}

func (s *Server) deleteDVRRule(ruleID string) error {
	var result sql.Result
	err := s.withUserTxTagged(context.Background(), []string{"dvr", "live-tv"}, func(tx *sql.Tx) error {
		// Running rows deliberately survive via ON DELETE SET NULL. Cancelling
		// future rows in this same transaction prevents scheduler reconciliation
		// from observing a deleted rule with recordings it still promises to run.
		if _, err := tx.Exec(`DELETE FROM live_tv_recordings WHERE rule_id = ? AND status = 'scheduled'`, ruleID); err != nil {
			return err
		}
		var err error
		result, err = tx.Exec(`DELETE FROM live_tv_recording_rules WHERE id = ?`, ruleID)
		return err
	})
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) listDVRRecordings() ([]DVRRecording, error) {
	rows, err := s.queryUserRead(context.Background(), `
		SELECT id, COALESCE(rule_id, ''), user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title,
			COALESCE(folder, ''), status, starts_at, ends_at, path, size_bytes, error, failure_code, priority, revision, created_at, updated_at
		FROM live_tv_recordings
		ORDER BY starts_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recordings := []DVRRecording{}
	for rows.Next() {
		recording, err := scanDVRRecording(rows)
		if err != nil {
			return nil, err
		}
		recordings = append(recordings, recording)
	}
	return recordings, rows.Err()
}

func (s *Server) listDVRRecordingsKeysetPageForUser(ctx context.Context, user User, limit int, after dvrRecordingCursor, countMode string) ([]DVRRecording, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	where := "WHERE profile_id = ?"
	args := []any{viewerProfileID(user)}
	countWhere := where
	countArgs := append([]any{}, args...)
	if after.ID != "" {
		where = appendDVRKeysetCondition(where, "(starts_at < ? OR (starts_at = ? AND id > ?))")
		args = append(args, after.StartsAt, after.StartsAt, after.ID)
	}
	total := 0
	if countMode == "exact" {
		if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM live_tv_recordings `+countWhere, countArgs...).Scan(&total); err != nil {
			return nil, 0, false, err
		}
	}
	args = append(args, limit+1)
	recordings, hasMore, err := s.queryDVRRecordingsKeyset(ctx, where, args, "starts_at DESC, id ASC", limit)
	if err != nil {
		return nil, 0, false, err
	}
	if countMode != "exact" {
		total = len(recordings) + boolInt(hasMore)
	}
	return recordings, total, hasMore, nil
}

func (s *Server) listDVRScheduleKeysetPageForUser(ctx context.Context, user User, limit int, after dvrRecordingCursor) ([]DVRRecording, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	where := "WHERE status IN ('scheduled', 'running') AND profile_id = ?"
	args := []any{viewerProfileID(user)}
	countWhere := where
	countArgs := append([]any{}, args...)
	if after.ID != "" {
		where += " AND (starts_at > ? OR (starts_at = ? AND id > ?))"
		args = append(args, after.StartsAt, after.StartsAt, after.ID)
	}
	var total int
	if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM live_tv_recordings `+countWhere, countArgs...).Scan(&total); err != nil {
		return nil, 0, false, err
	}
	args = append(args, limit+1)
	recordings, hasMore, err := s.queryDVRRecordingsKeyset(ctx, where, args, "starts_at ASC, id ASC", limit)
	return recordings, total, hasMore, err
}

func (s *Server) queryDVRRecordingsKeyset(ctx context.Context, where string, args []any, order string, limit int) ([]DVRRecording, bool, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT id, COALESCE(rule_id, ''), user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title,
			COALESCE(folder, ''), status, starts_at, ends_at, path, size_bytes, error, failure_code, priority, revision, created_at, updated_at
		FROM live_tv_recordings `+where+`
		ORDER BY `+order+` LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	recordings := []DVRRecording{}
	for rows.Next() {
		recording, scanErr := scanDVRRecording(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		recordings = append(recordings, recording)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(recordings) > limit
	if hasMore {
		recordings = recordings[:limit]
	}
	return recordings, hasMore, nil
}

func (s *Server) listDVRRecordingsPageForUser(ctx context.Context, user User, limit int, offset int, countMode string) ([]DVRRecording, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	where := "WHERE profile_id = ?"
	args := []any{viewerProfileID(user)}
	total := 0
	if countMode == "exact" {
		if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM live_tv_recordings `+where, args...).Scan(&total); err != nil {
			return nil, 0, false, err
		}
	}
	queryLimit := limit + 1
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, queryLimit, offset)
	rows, err := s.queryUserRead(ctx, `
		SELECT id, COALESCE(rule_id, ''), user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title,
			COALESCE(folder, ''), status, starts_at, ends_at, path, size_bytes, error, failure_code, priority, revision, created_at, updated_at
		FROM live_tv_recordings
		`+where+`
		ORDER BY starts_at DESC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	recordings := []DVRRecording{}
	for rows.Next() {
		recording, err := scanDVRRecording(rows)
		if err != nil {
			return nil, 0, false, err
		}
		recordings = append(recordings, recording)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	hasMore := len(recordings) > limit
	if hasMore {
		recordings = recordings[:limit]
	}
	if countMode != "exact" {
		total = offset + len(recordings) + boolInt(hasMore)
	}
	return recordings, total, hasMore, nil
}

func (s *Server) listDVRSchedulePageForUser(ctx context.Context, user User, limit, offset int) ([]DVRRecording, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	where := "WHERE status IN ('scheduled', 'running') AND profile_id = ?"
	args := []any{viewerProfileID(user)}
	var total int
	if err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM live_tv_recordings `+where, args...).Scan(&total); err != nil {
		return nil, 0, false, err
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, limit+1, offset)
	rows, err := s.queryUserRead(ctx, `
		SELECT id, COALESCE(rule_id, ''), user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title,
			COALESCE(folder, ''), status, starts_at, ends_at, path, size_bytes, error, failure_code, priority, revision, created_at, updated_at
		FROM live_tv_recordings
		`+where+`
		ORDER BY starts_at ASC, id ASC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	recordings := []DVRRecording{}
	for rows.Next() {
		recording, err := scanDVRRecording(rows)
		if err != nil {
			return nil, 0, false, err
		}
		recordings = append(recordings, recording)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	hasMore := len(recordings) > limit
	if hasMore {
		recordings = recordings[:limit]
	}
	return recordings, total, hasMore, nil
}

func (s *Server) listDVRRecordingGroups() ([]DVRRecordingGroup, error) {
	recordings, err := s.listDVRRecordings()
	if err != nil {
		return nil, err
	}
	return dvrRecordingGroupsFromRecordings(recordings), nil
}

const dvrRecordingGroupSampleLimit = 3

func (s *Server) listDVRRecordingGroupsKeysetPageForUser(ctx context.Context, user User, limit int, after dvrRecordingGroupCursor) ([]DVRRecordingGroup, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit = clampInt(limit, 1, 200)
	where := "WHERE status <> 'scheduled' AND profile_id = ?"
	args := []any{viewerProfileID(user)}
	having := ""
	if after.Title != "" || after.Folder != "" || after.LatestRecordingAt != "" {
		having = `HAVING (MAX(starts_at) < ? OR (MAX(starts_at) = ? AND (COALESCE(NULLIF(TRIM(title), ''), 'Untitled Recording') > ? OR (COALESCE(NULLIF(TRIM(title), ''), 'Untitled Recording') = ? AND COALESCE(folder, '') > ?))))`
		args = append(args, after.LatestRecordingAt, after.LatestRecordingAt, after.Title, after.Title, after.Folder)
	}
	args = append(args, limit+1)
	rows, err := s.queryUserRead(ctx, `
		SELECT COALESCE(NULLIF(TRIM(title), ''), 'Untitled Recording') AS group_title,
			COALESCE(folder, '') AS group_folder, COUNT(*) AS recording_count,
			COALESCE(SUM(size_bytes), 0) AS size_bytes, MAX(starts_at) AS latest_recording_at
		FROM live_tv_recordings `+where+`
		GROUP BY group_folder, group_title `+having+`
		ORDER BY latest_recording_at DESC, group_title ASC, group_folder ASC LIMIT ?`, args...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	groups := []DVRRecordingGroup{}
	for rows.Next() {
		var group DVRRecordingGroup
		var folder string
		if err := rows.Scan(&group.Title, &folder, &group.Count, &group.SizeBytes, &group.LatestRecordingAt); err != nil {
			return nil, 0, false, err
		}
		group.Title = strings.TrimSpace(group.Title)
		if group.Title == "" {
			group.Title = "Untitled Recording"
		}
		group.Folder = normalizeDVRFolder(folder)
		group.CursorFolder = folder
		group.ID = "recgrp_" + safePathComponent(sortableTitle(group.Folder+" "+group.Title))
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	hasMore := len(groups) > limit
	if hasMore {
		groups = groups[:limit]
	}
	for index := range groups {
		recordings, err := s.listDVRRecordingGroupSamples(ctx, user, groups[index].Title, groups[index].Folder, dvrRecordingGroupSampleLimit)
		if err != nil {
			return nil, 0, false, err
		}
		groups[index].Recordings = recordings
	}
	return groups, len(groups) + boolInt(hasMore), hasMore, nil
}

func (s *Server) listDVRRecordingGroupsPageForUser(ctx context.Context, user User, limit int, offset int) ([]DVRRecordingGroup, int, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	where := "WHERE status <> 'scheduled' AND profile_id = ?"
	args := []any{viewerProfileID(user)}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, limit+1, offset)
	rows, err := s.queryUserRead(ctx, `
		SELECT
			COALESCE(NULLIF(TRIM(title), ''), 'Untitled Recording') AS group_title,
			COALESCE(folder, '') AS group_folder,
			COUNT(*) AS recording_count,
			COALESCE(SUM(size_bytes), 0) AS size_bytes,
			MAX(starts_at) AS latest_recording_at
		FROM live_tv_recordings
		`+where+`
		GROUP BY group_folder, group_title
		ORDER BY latest_recording_at DESC, group_title ASC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	groups := []DVRRecordingGroup{}
	for rows.Next() {
		var group DVRRecordingGroup
		var folder string
		if err := rows.Scan(&group.Title, &folder, &group.Count, &group.SizeBytes, &group.LatestRecordingAt); err != nil {
			return nil, 0, false, err
		}
		group.Title = strings.TrimSpace(group.Title)
		if group.Title == "" {
			group.Title = "Untitled Recording"
		}
		group.Folder = normalizeDVRFolder(folder)
		group.ID = "recgrp_" + safePathComponent(sortableTitle(group.Folder+" "+group.Title))
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	hasMore := len(groups) > limit
	if hasMore {
		groups = groups[:limit]
	}
	for index := range groups {
		recordings, err := s.listDVRRecordingGroupSamples(ctx, user, groups[index].Title, groups[index].Folder, dvrRecordingGroupSampleLimit)
		if err != nil {
			return nil, 0, false, err
		}
		groups[index].Recordings = recordings
	}
	total := offset + len(groups) + boolInt(hasMore)
	return groups, total, hasMore, nil
}

func (s *Server) countDVRRecordingGroupsForUser(ctx context.Context, user User) (int, error) {
	where := "WHERE status <> 'scheduled' AND profile_id = ?"
	args := []any{viewerProfileID(user)}
	var total int
	err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM (SELECT 1 FROM live_tv_recordings `+where+` GROUP BY COALESCE(folder, ''), COALESCE(NULLIF(TRIM(title), ''), 'Untitled Recording'))`, args...).Scan(&total)
	return total, err
}

func (s *Server) listDVRRecordingGroupSamples(ctx context.Context, user User, title string, folder string, limit int) ([]DVRRecording, error) {
	if limit <= 0 {
		return nil, nil
	}
	where := "WHERE status <> 'scheduled' AND COALESCE(NULLIF(TRIM(title), ''), 'Untitled Recording') = ? AND COALESCE(folder, '') = ?"
	args := []any{title, folder}
	where += " AND profile_id = ?"
	args = append(args, viewerProfileID(user))
	args = append(args, limit)
	rows, err := s.queryUserRead(ctx, `
		SELECT id, COALESCE(rule_id, ''), user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title,
			COALESCE(folder, ''), status, starts_at, ends_at, path, size_bytes, error, failure_code, priority, revision, created_at, updated_at
		FROM live_tv_recordings
		`+where+`
		ORDER BY starts_at DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recordings := []DVRRecording{}
	for rows.Next() {
		recording, err := scanDVRRecording(rows)
		if err != nil {
			return nil, err
		}
		recordings = append(recordings, recording)
	}
	return recordings, rows.Err()
}

func dvrRecordingGroupsFromRecordings(recordings []DVRRecording) []DVRRecordingGroup {
	byID := map[string]*DVRRecordingGroup{}
	for _, recording := range recordings {
		if recording.Status == "scheduled" {
			continue
		}
		title := strings.TrimSpace(recording.Title)
		if title == "" {
			title = "Untitled Recording"
		}
		folder := normalizeDVRFolder(recording.Folder)
		groupID := "recgrp_" + safePathComponent(sortableTitle(folder+" "+title))
		group := byID[groupID]
		if group == nil {
			group = &DVRRecordingGroup{ID: groupID, Title: title, Folder: folder}
			byID[groupID] = group
		}
		group.Count++
		group.SizeBytes += recording.SizeBytes
		group.Recordings = append(group.Recordings, recording)
		if recording.StartsAt > group.LatestRecordingAt {
			group.LatestRecordingAt = recording.StartsAt
		}
	}
	groups := make([]DVRRecordingGroup, 0, len(byID))
	for _, group := range byID {
		sort.SliceStable(group.Recordings, func(i, j int) bool {
			return group.Recordings[i].StartsAt > group.Recordings[j].StartsAt
		})
		groups = append(groups, *group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].LatestRecordingAt == groups[j].LatestRecordingAt {
			return groups[i].Title < groups[j].Title
		}
		return groups[i].LatestRecordingAt > groups[j].LatestRecordingAt
	})
	return groups
}

func (s *Server) getDVRRecording(recordingID string) (DVRRecording, error) {
	recordingID = strings.TrimSpace(recordingID)
	row := s.queryUserRow(context.Background(), `
	SELECT id, COALESCE(rule_id, ''), user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title,
			COALESCE(folder, ''), status, starts_at, ends_at, path, size_bytes, error, failure_code, priority, revision, created_at, updated_at
		FROM live_tv_recordings
		WHERE id = ?`, recordingID)
	return scanDVRRecording(row)
}

type dvrRecordingScanner interface {
	Scan(dest ...any) error
}

func scanDVRRecording(scanner dvrRecordingScanner) (DVRRecording, error) {
	var recording DVRRecording
	if err := scanner.Scan(&recording.ID, &recording.RuleID, &recording.UserID, &recording.ProfileID, &recording.SourceID, &recording.ChannelID, &recording.ProgramID, &recording.Title, &recording.Folder, &recording.Status, &recording.StartsAt, &recording.EndsAt, &recording.Path, &recording.SizeBytes, &recording.Error, &recording.FailureCode, &recording.Priority, &recording.Revision, &recording.CreatedAt, &recording.UpdatedAt); err != nil {
		return DVRRecording{}, err
	}
	if recording.FailureCode != "" {
		recording.FailureMessageID = "dvr.recording-failed"
	}
	return recording, nil
}

func (s *Server) listDVRRecordingsForUser(user User) ([]DVRRecording, error) {
	recordings, _, _, err := s.listDVRRecordingsPageForUser(context.Background(), user, 250, 0, "none")
	return recordings, err
}

func (s *Server) listDVRRecordingGroupsForUser(user User) ([]DVRRecordingGroup, error) {
	groups, _, _, err := s.listDVRRecordingGroupsPageForUser(context.Background(), user, 200, 0)
	return groups, err
}

func (s *Server) getDVRRecordingForUser(user User, recordingID string) (DVRRecording, error) {
	recording, err := s.getDVRRecording(recordingID)
	if err != nil {
		return DVRRecording{}, err
	}
	if dvrRecordingOwnerProfileID(recording) != viewerProfileID(user) {
		return DVRRecording{}, sql.ErrNoRows
	}
	return recording, nil
}

func dvrRecordingOwnerProfileID(recording DVRRecording) string {
	if profileID := strings.TrimSpace(recording.ProfileID); profileID != "" {
		return profileID
	}
	return strings.TrimSpace(recording.UserID)
}

func userCanModifyDVRRecording(user User, recording DVRRecording) bool {
	return dvrRecordingOwnerProfileID(recording) == viewerProfileID(user) && recording.Status == "scheduled" && canScheduleDVR(user)
}

func userCanDeleteDVRRecording(user User, recording DVRRecording) bool {
	if dvrRecordingOwnerProfileID(recording) != viewerProfileID(user) {
		return false
	}
	if recording.Status == "scheduled" {
		return canScheduleDVR(user)
	}
	return canDeleteDVRRecording(user)
}

func (s *Server) createDVRRecording(user User, req DVRRecordingRequest) (DVRRecording, error) {
	return s.createDVRRecordingBound(user, req, false)
}

func (s *Server) createDVRRecordingBound(user User, req DVRRecordingRequest, allowSeriesProgram bool) (DVRRecording, error) {
	s.dvrAllocationMu.Lock()
	defer s.dvrAllocationMu.Unlock()
	req = s.applyDVRRecordingDefaults(req)
	req.SourceID = strings.TrimSpace(req.SourceID)
	req.Title = strings.TrimSpace(req.Title)
	if req.SourceID == "" || req.Title == "" {
		return DVRRecording{}, errors.New("Source and title are required.")
	}
	ref, err := s.resolveAuthorizedDVRLiveTVReference(context.Background(), user, req.SourceID, req.ChannelID, req.ProgramID, false, true)
	if err != nil {
		return DVRRecording{}, err
	}
	req.SourceID, req.ChannelID, req.ProgramID = ref.SourceID, ref.ChannelID, ref.ProgramID
	if err := s.validateDVRRuleRecordingBinding(user, req.RuleID, req.SourceID, req.ChannelID, req.ProgramID, allowSeriesProgram); err != nil {
		return DVRRecording{}, err
	}
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(req.StartsAt))
	if err != nil {
		return DVRRecording{}, errors.New("Recording start time must be RFC3339.")
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(req.EndsAt))
	if err != nil || !end.After(start) {
		return DVRRecording{}, errors.New("Recording end time must be after the start time.")
	}
	priority := boundedDVRPriority(intValue(req.Priority, 50))
	now := time.Now().UTC().Format(time.RFC3339)
	id := randomID("rec")
	err = s.withUserTxTagged(context.Background(), []string{"dvr", "live-tv"}, func(tx *sql.Tx) error {
		ruleID := strings.TrimSpace(req.RuleID)
		programID := strings.TrimSpace(req.ProgramID)
		if bindingErr := validateDVRRuleRecordingBindingTx(tx, viewerProfileID(user), ruleID, req.SourceID, req.ChannelID, programID, allowSeriesProgram); bindingErr != nil {
			return bindingErr
		}
		if ruleID != "" && programID != "" {
			if lookupErr := tx.QueryRow(`SELECT id FROM live_tv_recordings WHERE rule_id = ? AND program_id = ? LIMIT 1`, ruleID, programID).Scan(&id); lookupErr == nil {
				return nil
			} else if !errors.Is(lookupErr, sql.ErrNoRows) {
				return lookupErr
			}
		}
		conflict, conflictErr := findDVRRecordingConflictWithPriorityTx(tx, req.SourceID, start, end, "", priority)
		if conflictErr != nil {
			return conflictErr
		}
		if conflict.ID != "" {
			return newDVRScheduleConflictError(conflict, start, end)
		}
		result, insertErr := tx.Exec(`
			INSERT INTO live_tv_recordings (id, rule_id, user_id, profile_id, source_id, channel_id, program_id, title, folder, status, starts_at, ends_at, priority, revision, created_at, updated_at)
			VALUES (?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), ?, ?, ?, 'scheduled', ?, ?, ?, 1, ?, ?)
			ON CONFLICT(rule_id, program_id) WHERE rule_id IS NOT NULL AND program_id <> '' DO NOTHING`,
			id, ruleID, accountIDForUser(user), viewerProfileID(user), req.SourceID, strings.TrimSpace(req.ChannelID), programID, req.Title,
			req.Folder, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), priority, now, now)
		if insertErr != nil {
			return insertErr
		}
		if affected, _ := result.RowsAffected(); affected == 0 && ruleID != "" && programID != "" {
			return tx.QueryRow(`SELECT id FROM live_tv_recordings WHERE rule_id = ? AND program_id = ? LIMIT 1`, ruleID, programID).Scan(&id)
		}
		return nil
	})
	if err != nil {
		return DVRRecording{}, err
	}
	return s.getDVRRecording(id)
}

func (s *Server) updateDVRRecording(recordingID string, req DVRRecordingRequest) (DVRRecording, error) {
	current, err := s.getDVRRecording(recordingID)
	if err != nil {
		return DVRRecording{}, err
	}
	user, err := s.currentDVRUser(current.UserID, current.ProfileID)
	if err != nil {
		return DVRRecording{}, err
	}
	return s.updateDVRRecordingForUser(user, recordingID, req)
}

func (s *Server) updateDVRRecordingForUser(user User, recordingID string, req DVRRecordingRequest) (DVRRecording, error) {
	s.dvrAllocationMu.Lock()
	defer s.dvrAllocationMu.Unlock()
	current, err := s.getDVRRecording(recordingID)
	if err != nil {
		return DVRRecording{}, err
	}
	if dvrRecordingOwnerProfileID(current) != viewerProfileID(user) {
		return DVRRecording{}, sql.ErrNoRows
	}
	if current.Status != "scheduled" {
		return DVRRecording{}, errors.New("Only scheduled recordings can be updated.")
	}
	sourceID := firstNonEmpty(strings.TrimSpace(req.SourceID), current.SourceID)
	title := firstNonEmpty(strings.TrimSpace(req.Title), current.Title)
	if sourceID == "" || title == "" {
		return DVRRecording{}, errors.New("Source and title are required.")
	}
	channelID := firstNonEmpty(strings.TrimSpace(req.ChannelID), current.ChannelID)
	programID := firstNonEmpty(strings.TrimSpace(req.ProgramID), current.ProgramID)
	ref, err := s.resolveAuthorizedDVRLiveTVReference(context.Background(), user, sourceID, channelID, programID, false, true)
	if err != nil {
		return DVRRecording{}, err
	}
	sourceID, channelID, programID = ref.SourceID, ref.ChannelID, ref.ProgramID
	ruleID := firstNonEmpty(strings.TrimSpace(req.RuleID), current.RuleID)
	if err := s.validateDVRRuleRecordingBinding(user, ruleID, sourceID, channelID, programID, false); err != nil {
		return DVRRecording{}, err
	}
	startsAt := firstNonEmpty(strings.TrimSpace(req.StartsAt), current.StartsAt)
	endsAt := firstNonEmpty(strings.TrimSpace(req.EndsAt), current.EndsAt)
	start, err := time.Parse(time.RFC3339, startsAt)
	if err != nil {
		return DVRRecording{}, errors.New("Recording start time must be RFC3339.")
	}
	end, err := time.Parse(time.RFC3339, endsAt)
	if err != nil || !end.After(start) {
		return DVRRecording{}, errors.New("Recording end time must be after the start time.")
	}
	priority := boundedDVRPriority(intValue(req.Priority, current.Priority))
	now := time.Now().UTC().Format(time.RFC3339)
	var result sql.Result
	err = s.withUserTxTagged(context.Background(), []string{"dvr", "live-tv"}, func(tx *sql.Tx) error {
		if bindingErr := validateDVRRuleRecordingBindingTx(tx, viewerProfileID(user), ruleID, sourceID, channelID, programID, false); bindingErr != nil {
			return bindingErr
		}
		conflict, conflictErr := findDVRRecordingConflictWithPriorityTx(tx, sourceID, start, end, recordingID, priority)
		if conflictErr != nil {
			return conflictErr
		}
		if conflict.ID != "" {
			return newDVRScheduleConflictError(conflict, start, end)
		}
		var updateErr error
		result, updateErr = tx.Exec(`
			UPDATE live_tv_recordings
			SET rule_id = NULLIF(?, ''), source_id = ?, channel_id = NULLIF(?, ''), program_id = ?,
				title = ?, folder = ?, starts_at = ?, ends_at = ?, priority = ?, error = '', failure_code = '', revision = revision + 1, updated_at = ?
			WHERE id = ? AND profile_id = ? AND status = 'scheduled' AND (? IS NULL OR revision = ?)`,
			ruleID, sourceID, channelID,
			programID, title, normalizeDVRFolder(firstNonEmpty(req.Folder, current.Folder)), start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), priority, now, recordingID, viewerProfileID(user), req.ExpectedRevision, req.ExpectedRevision)
		return updateErr
	})
	if err != nil {
		return DVRRecording{}, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		if req.ExpectedRevision != nil {
			return DVRRecording{}, errDVRRevisionConflict
		}
		return DVRRecording{}, sql.ErrNoRows
	}
	return s.getDVRRecording(recordingID)
}

func (s *Server) cancelDVRRecording(recordingID string) error {
	recording, err := s.getDVRRecording(recordingID)
	if err != nil {
		return err
	}
	if recording.Status != "scheduled" {
		return errors.New("Only scheduled recordings can be cancelled.")
	}
	result, err := s.execUserWriteTagged(context.Background(), []string{"dvr", "live-tv"}, `DELETE FROM live_tv_recordings WHERE id = ? AND status = 'scheduled'`, recordingID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) deleteDVRRecording(recordingID string) error {
	recording, err := s.getDVRRecording(recordingID)
	if err != nil {
		return err
	}
	if recording.Status == "scheduled" {
		return s.cancelDVRRecording(recordingID)
	}
	if recording.Status == "running" {
		return errors.New("Running recordings cannot be deleted until recording stops.")
	}
	if !dvrRecordingFinishedStatus(recording.Status) {
		return errors.New("Only scheduled, completed, or failed recordings can be deleted.")
	}
	if strings.TrimSpace(recording.Path) != "" {
		if err := removeDVRRecordingFile(recording.Path, s.cfg.AppDataDir); err != nil {
			return err
		}
	}
	var result sql.Result
	if err := s.withUserTxTagged(context.Background(), []string{"dvr", "live-tv", "media", "library-items", "home"}, func(tx *sql.Tx) error {
		if err := deleteImportedDVRRecordingMedia(tx, recordingID); err != nil {
			return err
		}
		var err error
		result, err = tx.Exec(`DELETE FROM live_tv_recordings WHERE id = ? AND status IN ('complete', 'completed', 'incomplete', 'failed')`, recordingID)
		return err
	}); err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) findDVRRecordingConflict(sourceID string, start time.Time, end time.Time, ignoreID string) (DVRRecording, error) {
	return s.findDVRRecordingConflictWithPriority(sourceID, start, end, ignoreID, 50)
}

func (s *Server) findDVRRecordingConflictWithPriority(sourceID string, start time.Time, end time.Time, ignoreID string, requestedPriority int) (DVRRecording, error) {
	var conflict DVRRecording
	err := s.withUserTxTagged(context.Background(), []string{"dvr", "live-tv"}, func(tx *sql.Tx) error {
		var findErr error
		conflict, findErr = findDVRRecordingConflictWithPriorityTx(tx, sourceID, start, end, ignoreID, requestedPriority)
		return findErr
	})
	return conflict, err
}

func findDVRRecordingConflictWithPriorityTx(tx *sql.Tx, sourceID string, start time.Time, end time.Time, ignoreID string, requestedPriority int) (DVRRecording, error) {
	cutoff := time.Now().UTC().Add(-liveTVAllocationStaleAfter).Format(time.RFC3339)
	if _, err := pruneStaleLiveTVTunerAllocationsTx(tx, cutoff); err != nil {
		return DVRRecording{}, err
	}
	var capacity int
	if err := tx.QueryRow(`SELECT MAX(1, COALESCE(tuner_count, 1)) FROM live_tv_sources WHERE id = ?`, sourceID).Scan(&capacity); err != nil {
		return DVRRecording{}, err
	}
	sourceCapacity := capacity
	liveAllocations := 0
	now := time.Now().UTC()
	if start.Before(now.Add(liveTVAllocationStaleAfter)) && end.After(now) {
		if err := tx.QueryRow(`
			SELECT COUNT(*) FROM live_tv_tuner_allocations
			WHERE source_id = ? AND allocation_kind = 'live_session'`, sourceID).Scan(&liveAllocations); err != nil {
			return DVRRecording{}, err
		}
		capacity -= liveAllocations
		if capacity <= 0 {
			return DVRRecording{ID: "active_live_tv", SourceID: sourceID, Title: "Active Live TV playback", Status: "running", StartsAt: now.Format(time.RFC3339), EndsAt: end.UTC().Format(time.RFC3339), Priority: 100, AllocationCapacity: sourceCapacity, AllocationDemand: liveAllocations + 1}, nil
		}
	}
	rows, err := tx.Query(`
		SELECT id, COALESCE(rule_id, ''), user_id, profile_id, source_id, COALESCE(channel_id, ''), program_id, title,
			COALESCE(folder, ''), status, starts_at, ends_at, path, size_bytes, error, failure_code, priority, revision, created_at, updated_at
		FROM live_tv_recordings
		WHERE source_id = ?
			AND id <> ?
			AND (status = 'scheduled' OR (status = 'running' AND EXISTS (
				SELECT 1 FROM live_tv_tuner_allocations allocation
				WHERE allocation.allocation_kind = 'dvr_recording' AND allocation.consumer_id = live_tv_recordings.id
			)))
			AND starts_at < ?
			AND ends_at > ?
		ORDER BY CASE WHEN status = 'running' THEN 0 ELSE 1 END, priority DESC, starts_at ASC, id ASC
		LIMIT 2000`,
		sourceID, ignoreID, end.UTC().Format(time.RFC3339), start.UTC().Format(time.RFC3339))
	if err != nil {
		return DVRRecording{}, err
	}
	defer rows.Close()
	overlaps := make([]DVRRecording, 0, capacity)
	for rows.Next() {
		recording, scanErr := scanDVRRecording(rows)
		if scanErr != nil {
			return DVRRecording{}, scanErr
		}
		overlaps = append(overlaps, recording)
	}
	if err := rows.Err(); err != nil || len(overlaps) == 0 {
		return DVRRecording{}, err
	}
	activeAtCapacity := []DVRRecording{}
	points := []time.Time{start, end}
	for _, recording := range overlaps {
		recordingStart, startErr := time.Parse(time.RFC3339, recording.StartsAt)
		recordingEnd, endErr := time.Parse(time.RFC3339, recording.EndsAt)
		if startErr != nil || endErr != nil || !recordingEnd.After(recordingStart) {
			continue
		}
		if recordingStart.After(start) && recordingStart.Before(end) {
			points = append(points, recordingStart)
		}
		if recordingEnd.After(start) && recordingEnd.Before(end) {
			points = append(points, recordingEnd)
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })
	for index := 0; index+1 < len(points); index++ {
		intervalStart, intervalEnd := points[index], points[index+1]
		if !intervalEnd.After(intervalStart) {
			continue
		}
		active := []DVRRecording{}
		for _, recording := range overlaps {
			recordingStart, _ := time.Parse(time.RFC3339, recording.StartsAt)
			recordingEnd, _ := time.Parse(time.RFC3339, recording.EndsAt)
			if recordingStart.Before(intervalEnd) && recordingEnd.After(intervalStart) {
				active = append(active, recording)
			}
		}
		if len(active) >= capacity {
			activeAtCapacity = active
			break
		}
	}
	if len(activeAtCapacity) < capacity {
		return DVRRecording{}, nil
	}
	// The conflict projection picks the strongest competing allocation so the
	// client can explain why changing priority did or did not help. The server
	// never silently cancels another profile's recording.
	conflict := activeAtCapacity[0]
	for _, candidate := range activeAtCapacity[1:] {
		if candidate.Status == "running" || candidate.Priority > conflict.Priority || (candidate.Priority == conflict.Priority && candidate.ID < conflict.ID) {
			conflict = candidate
		}
	}
	conflict.AllocationCapacity = sourceCapacity
	conflict.AllocationDemand = liveAllocations + len(activeAtCapacity) + 1
	_ = requestedPriority
	return conflict, nil
}

func mustList[T any](value []T, err error) []T {
	if err != nil {
		return nil
	}
	return value
}

func mapKeysRaw(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
