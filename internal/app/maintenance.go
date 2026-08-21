package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/database"
)

type scheduledTaskSettings struct {
	Enabled                  bool
	MaintenanceWindow        string
	MaintenanceDays          string
	MaintenanceTimezone      string
	StartHour                int
	EndHour                  int
	BackupDatabase           bool
	BackupCadence            string
	BackupRetentionDays      int
	ScanLibraries            bool
	LibraryScanCadence       string
	LibraryScanIntervalHours int
	RefreshMetadata          bool
	MetadataRefreshCadence   string
	MetadataRefreshDays      int
	AnalyzeMedia             bool
	AnalysisCadence          string
	EmptyTrash               bool
	TrashRetentionDays       int
	TrickplayRetentionDays   int
	TrickplayMaxStorageMB    int
	TaskTriggers             map[string]scheduledTaskTrigger
}

var backupSyncTestState struct {
	sync.Mutex
	hook func(string) error
}

var backupVacuumTestState struct {
	sync.Mutex
	hook func(string) error
}

var backupRemoveTestState struct {
	sync.Mutex
	hook func(string) error
}

var backupPublicationEvidenceTestState struct {
	sync.Mutex
	hook func(string, []byte) error
}

func syncBackupDirectory(path string) error {
	backupSyncTestState.Lock()
	hook := backupSyncTestState.hook
	backupSyncTestState.Unlock()
	if hook != nil {
		return hook(path)
	}
	return database.SyncDirectory(path)
}

func setBackupSyncDirectoryForTest(hook func(string) error) func() {
	backupSyncTestState.Lock()
	previous := backupSyncTestState.hook
	backupSyncTestState.hook = hook
	backupSyncTestState.Unlock()
	return func() {
		backupSyncTestState.Lock()
		backupSyncTestState.hook = previous
		backupSyncTestState.Unlock()
	}
}

func setBackupVacuumForTest(hook func(string) error) func() {
	backupVacuumTestState.Lock()
	previous := backupVacuumTestState.hook
	backupVacuumTestState.hook = hook
	backupVacuumTestState.Unlock()
	return func() {
		backupVacuumTestState.Lock()
		backupVacuumTestState.hook = previous
		backupVacuumTestState.Unlock()
	}
}

func removeBackupFile(path string) error {
	backupRemoveTestState.Lock()
	hook := backupRemoveTestState.hook
	backupRemoveTestState.Unlock()
	if hook != nil {
		return hook(path)
	}
	return os.Remove(path)
}

func setBackupRemoveForTest(hook func(string) error) func() {
	backupRemoveTestState.Lock()
	previous := backupRemoveTestState.hook
	backupRemoveTestState.hook = hook
	backupRemoveTestState.Unlock()
	return func() {
		backupRemoveTestState.Lock()
		backupRemoveTestState.hook = previous
		backupRemoveTestState.Unlock()
	}
}

func writeBackupPublicationEvidence(path string, body []byte) error {
	backupPublicationEvidenceTestState.Lock()
	hook := backupPublicationEvidenceTestState.hook
	backupPublicationEvidenceTestState.Unlock()
	if hook != nil {
		return hook(path, body)
	}
	return database.WriteAtomicPrivateFile(path, body)
}

func setBackupPublicationEvidenceForTest(hook func(string, []byte) error) func() {
	backupPublicationEvidenceTestState.Lock()
	previous := backupPublicationEvidenceTestState.hook
	backupPublicationEvidenceTestState.hook = hook
	backupPublicationEvidenceTestState.Unlock()
	return func() {
		backupPublicationEvidenceTestState.Lock()
		backupPublicationEvidenceTestState.hook = previous
		backupPublicationEvidenceTestState.Unlock()
	}
}

func (s *Server) vacuumDatabaseBackupInto(path string) error {
	backupVacuumTestState.Lock()
	hook := backupVacuumTestState.hook
	backupVacuumTestState.Unlock()
	if hook != nil {
		return hook(path)
	}
	_, err := s.execBackgroundWrite(context.Background(), `VACUUM INTO ?`, path)
	return err
}

type scheduledTaskTrigger struct {
	Enabled       bool
	HasEnabled    bool
	IntervalHours int
}

func (s *Server) runScheduledTaskScheduler(ctx context.Context) {
	timer := time.NewTimer(75 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.queueScheduledTasks(time.Now().UTC())
		timer.Reset(10 * time.Minute)
	}
}

func (s *Server) queueScheduledTasks(now time.Time) {
	s.queueScheduledLiveTVSourceRefreshes(now)

	settings := s.scheduledTaskSettings()
	localNow := now.In(maintenanceLocation(settings.MaintenanceTimezone))
	if !settings.Enabled || !withinScheduledDays(localNow, settings.MaintenanceDays) || !withinScheduledWindow(localNow, settings.StartHour, settings.EndHour) {
		return
	}
	runKey := localNow.Format("2006-01-02-15")
	if settings.taskEnabled("database-backup", settings.BackupDatabase) && !s.jobAlreadyQueuedWithin("database_backup", "database", "scheduled", settings.taskIntervalHours("database-backup", settings.BackupCadence, 24)) {
		if s.reserveScheduledJob("database_backup", "database", "scheduled", now.Format("2006-01-02-15")) {
			if _, err := s.createJobFor("database_backup", "Scheduled database backup queued.", "database", "scheduled"); err != nil {
				s.releaseScheduledJob("database_backup", "database", "scheduled", now.Format("2006-01-02-15"))
				s.log.Warn("schedule database backup failed", "error", err)
			}
		}
	}
	if settings.globallyEnabledTask("library-scan", settings.ScanLibraries) {
		s.queueScheduledLibraryScans(now, settings.taskIntervalHours("library-scan", settings.LibraryScanCadence, settings.LibraryScanIntervalHours))
	}
	if settings.taskEnabled("metadata-refresh", settings.RefreshMetadata) {
		s.queueScheduledMetadataRefresh(now, runKey, settings.MetadataRefreshDays, settings.taskIntervalHours("metadata-refresh", settings.MetadataRefreshCadence, 24))
	}
	if settings.globallyEnabledTask("media-analysis", settings.AnalyzeMedia) {
		s.queueScheduledMediaAnalysis(now, runKey, settings.taskIntervalHours("media-analysis", settings.AnalysisCadence, 24))
	}
	if settings.EmptyTrash {
		s.queueScheduledSingletonMaintenanceJob("library_trash_cleanup", "Scheduled library trash cleanup queued.")
	}
	s.queueScheduledSingletonMaintenanceJob("optimized_version_prune", "Optimized version retention cleanup queued.")
	s.queueScheduledSingletonMaintenanceJob("trickplay_prune", "Trickplay preview retention cleanup queued.")
}

func (s *Server) scheduledTaskSettings() scheduledTaskSettings {
	settings, err := s.loadSettings()
	if err != nil {
		return scheduledTaskSettings{Enabled: false}
	}
	group, _ := settings["scheduledTasks"].(map[string]any)
	maintenanceWindow := normalizeMaintenanceWindow(settingString(group, "maintenanceWindow", ""))
	startHour, endHour := scheduledWindowHours(maintenanceWindow, settingInt(group, "startHour", 2), settingInt(group, "endHour", 5))
	return scheduledTaskSettings{
		Enabled:                  settingBool(group, "enabled", true),
		MaintenanceWindow:        maintenanceWindow,
		MaintenanceDays:          normalizeMaintenanceDays(settingString(group, "maintenanceDays", "every-day")),
		MaintenanceTimezone:      normalizeMaintenanceTimezone(settingString(group, "maintenanceTimezone", "UTC")),
		StartHour:                startHour,
		EndHour:                  endHour,
		BackupDatabase:           settingBool(group, "backupDatabase", true),
		BackupCadence:            normalizeScheduledCadence(settingString(group, "backupCadence", "daily")),
		BackupRetentionDays:      max(1, settingInt(group, "backupRetentionDays", 14)),
		ScanLibraries:            settingBool(group, "scanLibraries", true),
		LibraryScanCadence:       normalizeScheduledCadence(settingString(group, "libraryScanCadence", "daily")),
		LibraryScanIntervalHours: max(1, settingInt(group, "libraryScanIntervalHours", 24)),
		RefreshMetadata:          settingBool(group, "refreshMetadata", false),
		MetadataRefreshCadence:   normalizeScheduledCadence(settingString(group, "metadataRefreshCadence", "daily")),
		MetadataRefreshDays:      max(1, settingInt(group, "metadataRefreshDays", 14)),
		AnalyzeMedia:             settingBool(group, "analyzeMedia", true),
		AnalysisCadence:          normalizeScheduledCadence(settingString(group, "analysisCadence", "daily")),
		EmptyTrash:               settingBool(group, "emptyTrash", false),
		TrashRetentionDays:       max(0, settingInt(group, "trashRetentionDays", 30)),
		TrickplayRetentionDays:   max(0, settingInt(group, "trickplayRetentionDays", 14)),
		TrickplayMaxStorageMB:    max(0, settingInt(group, "trickplayMaxStorageMB", 0)),
		TaskTriggers:             decodeScheduledTaskTriggers(group["taskTriggers"]),
	}
}

func (settings scheduledTaskSettings) taskEnabled(taskID string, fallback bool) bool {
	trigger, ok := settings.TaskTriggers[taskID]
	if !ok || !trigger.HasEnabled {
		return fallback
	}
	return trigger.Enabled
}

func (settings scheduledTaskSettings) globallyEnabledTask(taskID string, globalEnabled bool) bool {
	return globalEnabled && settings.taskEnabled(taskID, globalEnabled)
}

func (settings scheduledTaskSettings) taskIntervalHours(taskID string, cadence string, fallback int) int {
	if hours := scheduledCadenceHours(cadence); hours > 0 {
		return hours
	}
	trigger, ok := settings.TaskTriggers[taskID]
	if !ok || trigger.IntervalHours <= 0 {
		return max(1, fallback)
	}
	return max(1, trigger.IntervalHours)
}

func decodeScheduledTaskTriggers(raw any) map[string]scheduledTaskTrigger {
	source, ok := raw.(map[string]any)
	if !ok {
		return map[string]scheduledTaskTrigger{}
	}
	triggers := map[string]scheduledTaskTrigger{}
	for taskID, value := range source {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		group, ok := value.(map[string]any)
		if !ok {
			continue
		}
		trigger := scheduledTaskTrigger{}
		if enabled, ok := group["enabled"].(bool); ok {
			trigger.Enabled = enabled
			trigger.HasEnabled = true
		}
		trigger.IntervalHours = max(0, settingInt(group, "intervalHours", 0))
		triggers[taskID] = trigger
	}
	return triggers
}

func normalizeMaintenanceWindow(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "overnight", "late-night", "always", "custom":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "custom"
	}
}

func normalizeMaintenanceDays(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "weekdays", "weekends":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "every-day"
	}
}

// normalizeMaintenanceTimezone is fail-closed for persisted legacy settings:
// an absent value means the explicit UTC authority, while an invalid supplied
// value is rejected by the settings validator before it can be published.
func normalizeMaintenanceTimezone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "UTC"
	}
	if _, err := time.LoadLocation(value); err != nil {
		return "UTC"
	}
	return value
}

func maintenanceLocation(value string) *time.Location {
	zone := normalizeMaintenanceTimezone(value)
	location, err := time.LoadLocation(zone)
	if err != nil {
		return time.UTC
	}
	return location
}

func scheduledWindowHours(window string, startHour, endHour int) (int, int) {
	switch normalizeMaintenanceWindow(window) {
	case "overnight":
		return 2, 5
	case "late-night":
		return 0, 6
	case "always":
		return 0, 0
	default:
		return clampHour(startHour), clampHour(endHour)
	}
}

func normalizeScheduledCadence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hourly", "daily", "weekly", "monthly", "custom":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "custom"
	}
}

func scheduledCadenceHours(value string) int {
	switch normalizeScheduledCadence(value) {
	case "hourly":
		return 1
	case "daily":
		return 24
	case "weekly":
		return 168
	case "monthly":
		return 720
	default:
		return 0
	}
}

func scheduledCadenceLabel(value string, hours int) string {
	switch normalizeScheduledCadence(value) {
	case "hourly":
		return "Hourly"
	case "daily":
		return "Daily"
	case "weekly":
		return "Weekly"
	case "monthly":
		return "Monthly"
	default:
		if hours == 1 {
			return "Every hour"
		}
		if hours%24 == 0 {
			days := hours / 24
			if days == 1 {
				return "Daily"
			}
			return fmt.Sprintf("Every %d days", days)
		}
		return fmt.Sprintf("Every %d hours", max(1, hours))
	}
}

func scheduledWindowLabel(window string, startHour, endHour int) string {
	switch normalizeMaintenanceWindow(window) {
	case "overnight":
		return "Overnight (2:00-5:00 UTC)"
	case "late-night":
		return "Late night (0:00-6:00 UTC)"
	case "always":
		return "Any time"
	default:
		return fmt.Sprintf("%02d:00-%02d:00 UTC", clampHour(startHour), clampHour(endHour))
	}
}

func scheduledWindowLabelInTimezone(window, timezone string, startHour, endHour int) string {
	label := scheduledWindowLabel(window, startHour, endHour)
	if normalizeMaintenanceTimezone(timezone) == "UTC" || normalizeMaintenanceWindow(window) == "always" {
		return label
	}
	return strings.TrimSuffix(label, " UTC") + " " + normalizeMaintenanceTimezone(timezone)
}

func scheduledDaysLabel(value string) string {
	switch normalizeMaintenanceDays(value) {
	case "weekdays":
		return "Weekdays"
	case "weekends":
		return "Weekends"
	default:
		return "Every day"
	}
}

func (s *Server) queueScheduledLibraryScans(now time.Time, intervalHours int) {
	libraries, err := s.listLibraries()
	if err != nil {
		s.log.Warn("scheduled library scan lookup failed", "error", err)
		return
	}
	for _, library := range libraries {
		librarySettings := s.libraryRuntimeSettingsFor(library)
		if !settingBool(library.Settings, "scannerEnabled", true) || !librarySettings.ScanAutomatically {
			continue
		}
		within := time.Duration(libraryScheduledScanIntervalHours(library, intervalHours)) * time.Hour
		if s.libraryScanRecentlyQueued(library.ID, within) {
			continue
		}
		if !s.reserveScheduledJob("library_scan", "library", library.ID, now.Format("2006-01-02-15")) {
			continue
		}
		if _, err := s.queueLibraryScan(library, "reconcile", "schedule", fmt.Sprintf("Scheduled scan queued for %s.", library.Name)); err != nil {
			s.releaseScheduledJob("library_scan", "library", library.ID, now.Format("2006-01-02-15"))
			s.log.Warn("scheduled library scan queue failed", "library", library.ID, "error", err)
		}
	}
}

func libraryScheduledScanIntervalHours(library Library, fallbackHours int) int {
	cadence := strings.TrimSpace(settingString(library.Settings, "scheduledScanCadence", ""))
	if cadence == "" {
		return max(1, fallbackHours)
	}
	if hours := scheduledCadenceHours(cadence); hours > 0 {
		return hours
	}
	return max(1, settingInt(library.Settings, "scheduledScanIntervalHours", fallbackHours))
}

func (s *Server) queueScheduledMetadataRefresh(now time.Time, runKey string, refreshDays int, intervalHours int) {
	libraries, err := s.listLibraries()
	if err != nil {
		s.log.Warn("scheduled metadata refresh lookup failed", "error", err)
		return
	}
	for _, library := range libraries {
		libraryRefreshDays, enabled := libraryScheduledMetadataRefreshDays(library, refreshDays)
		if !enabled {
			continue
		}
		if s.jobAlreadyQueuedWithin("metadata_refresh_library", "library", library.ID, intervalHours) {
			continue
		}
		if !s.reserveScheduledJob("metadata_refresh_library", "library", library.ID, runKey) {
			continue
		}
		if s.jobAlreadyQueuedTodayFor("metadata_refresh_library", "library", library.ID, runKey) {
			s.releaseScheduledJob("metadata_refresh_library", "library", library.ID, runKey)
			continue
		}
		metadata := map[string]string{
			"libraryId":    library.ID,
			"libraryName":  library.Name,
			"refreshDays":  strconv.Itoa(libraryRefreshDays),
			"subtaskScope": "library",
		}
		if _, err := s.createJobForWithMetadata("metadata_refresh_library", fmt.Sprintf("Scheduled metadata refresh queued for %s.", library.Name), "library", library.ID, metadata); err != nil {
			s.releaseScheduledJob("metadata_refresh_library", "library", library.ID, runKey)
			s.log.Warn("scheduled metadata refresh queue failed", "library", library.ID, "error", err)
		}
	}
}

func libraryScheduledMetadataRefreshDays(library Library, fallbackDays int) (int, bool) {
	if enabled, ok := library.Settings["scheduledMetadataRefreshEnabled"].(bool); ok && !enabled {
		return 0, false
	}
	return max(1, settingInt(library.Settings, "scheduledMetadataRefreshDays", fallbackDays)), true
}

func (s *Server) queueScheduledMediaAnalysis(now time.Time, runKey string, intervalHours int) {
	libraries, err := s.listLibraries()
	if err != nil {
		s.log.Warn("scheduled media analysis library lookup failed", "error", err)
		return
	}
	eligibleLibraryIDs := []string{}
	for _, library := range libraries {
		libraryInterval, enabled := libraryScheduledAnalysisIntervalHours(library, intervalHours)
		if !enabled {
			continue
		}
		within := time.Duration(libraryInterval) * time.Hour
		if s.libraryMediaAnalysisRecentlyQueued(library.ID, within) {
			continue
		}
		eligibleLibraryIDs = append(eligibleLibraryIDs, library.ID)
	}
	if len(eligibleLibraryIDs) == 0 {
		return
	}
	items, err := s.scheduledAnalysisItemsForLibraries(eligibleLibraryIDs, 50)
	if err != nil {
		s.log.Warn("scheduled media analysis lookup failed", "error", err)
		return
	}
	for _, item := range items {
		if !s.mediaAnalysisQueueEnabled(item) {
			continue
		}
		if _, ok, err := s.activeJobFor("media_analyze", "media", item.ID); err != nil {
			s.log.Warn("scheduled media analysis active job lookup failed", "media", item.ID, "error", err)
			continue
		} else if ok {
			continue
		}
		if !s.reserveScheduledJob("media_analyze", "media", item.ID, runKey) {
			continue
		}
		if s.jobAlreadyQueuedTodayFor("media_analyze", "media", item.ID, runKey) {
			continue
		}
		if _, err := s.createJobForWithMetadata("media_analyze", fmt.Sprintf("Scheduled media stream analysis queued for %s.", item.Title), "media", item.ID, representativeFrameAnalysisMetadata()); err != nil {
			s.releaseScheduledJob("media_analyze", "media", item.ID, runKey)
			s.log.Warn("scheduled media analysis queue failed", "media", item.ID, "error", err)
		}
	}
}

func libraryScheduledAnalysisIntervalHours(library Library, fallbackHours int) (int, bool) {
	if enabled, ok := library.Settings["scheduledAnalysisEnabled"].(bool); ok && !enabled {
		return 0, false
	}
	cadence := strings.TrimSpace(settingString(library.Settings, "scheduledAnalysisCadence", ""))
	if cadence == "" {
		return max(1, fallbackHours), true
	}
	if hours := scheduledCadenceHours(cadence); hours > 0 {
		return hours, true
	}
	return max(1, settingInt(library.Settings, "scheduledAnalysisIntervalHours", fallbackHours)), true
}

func (s *Server) queueScheduledSingletonMaintenanceJob(jobType string, message string) {
	const intervalHours = 24
	if s.jobAlreadyQueuedWithin(jobType, "maintenance", "scheduled", intervalHours) {
		return
	}
	if _, err := s.createJobFor(jobType, message, "maintenance", "scheduled"); err != nil {
		s.log.Warn("scheduled maintenance job queue failed", "type", jobType, "error", err)
	}
}

func (s *Server) scheduledMetadataItems(now time.Time, refreshDays int, limit int) ([]MediaItem, error) {
	return s.scheduledMetadataItemsContext(context.Background(), now, refreshDays, limit)
}

func (s *Server) scheduledMetadataItemsContext(ctx context.Context, now time.Time, refreshDays int, limit int) ([]MediaItem, error) {
	cutoff := now.UTC().Add(-time.Duration(max(1, refreshDays)) * 24 * time.Hour).Format(time.RFC3339)
	rows, err := s.queryBackgroundRead(ctx, `
		SELECT id, COALESCE(library_id, ''), COALESCE(parent_id, ''), type, title, sort_title, year, duration_seconds, added_at, source_url
		FROM media_items
		WHERE (parent_id IS NULL OR type = 'track')
			AND type IN ('movie', 'show', 'anime', 'artist', 'album', 'track')
			AND (metadata_refreshed_at = '' OR metadata_refreshed_at <= ?)
		ORDER BY added_at ASC
		LIMIT ?`, cutoff, max(1, limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledMediaItems(rows)
}

func (s *Server) scheduledAnalysisItems(limit int) ([]MediaItem, error) {
	return s.scheduledAnalysisItemsContext(context.Background(), limit)
}

func (s *Server) scheduledAnalysisItemsContext(ctx context.Context, limit int) ([]MediaItem, error) {
	return s.scheduledAnalysisItemsForLibrariesContext(ctx, nil, limit)
}

func (s *Server) scheduledAnalysisItemsForLibraries(libraryIDs []string, limit int) ([]MediaItem, error) {
	return s.scheduledAnalysisItemsForLibrariesContext(context.Background(), libraryIDs, limit)
}

func (s *Server) scheduledAnalysisItemsForLibrariesContext(ctx context.Context, libraryIDs []string, limit int) ([]MediaItem, error) {
	where := `
		WHERE m.source_url <> ''
			AND m.type IN ('movie', 'episode', 'track', 'audiobook', 'extra')
			AND (
				m.duration_seconds = 0
				OR NOT EXISTS (SELECT 1 FROM media_streams ms WHERE ms.media_id = m.id AND ms.source_kind = 'ffprobe')
			)`
	args := []any{}
	if len(libraryIDs) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(libraryIDs)), ",")
		where += ` AND m.library_id IN (` + placeholders + `)`
		for _, libraryID := range libraryIDs {
			args = append(args, libraryID)
		}
	}
	args = append(args, max(1, limit))
	rows, err := s.queryBackgroundRead(ctx, `
		SELECT id, COALESCE(library_id, ''), COALESCE(parent_id, ''), type, title, sort_title, year, duration_seconds, added_at, source_url
		FROM media_items m
		`+where+`
		ORDER BY m.added_at ASC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledMediaItems(rows)
}

func scanScheduledMediaItems(rows *sql.Rows) ([]MediaItem, error) {
	items := []MediaItem{}
	for rows.Next() {
		var item MediaItem
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.ParentID, &item.Type, &item.Title, &item.SortTitle, &item.Year, &item.DurationSeconds, &item.AddedAt, &item.SourceURL); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) runDatabaseBackup(ctx context.Context, job Job) {
	if err := ctx.Err(); err != nil {
		return
	}
	settings := s.scheduledTaskSettings()
	if err := s.setJobMessage(job.ID, "running", 20, "Preparing database backup."); err != nil {
		s.log.Warn("backup job update failed", "job", job.ID, "error", err)
	}
	// createDatabaseBackup ultimately delegates to SQLite's synchronous backup
	// and filesystem rename primitives. They cannot be interrupted safely once
	// started; the supervised job remains joined and the post-call context
	// check prevents a cancelled backup from being reported as successful.
	path, err := s.createDatabaseBackup()
	if err == nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return
		}
	}
	var publicationWarning *BackupPublicationWarning
	if err != nil {
		if errors.As(err, &publicationWarning) {
			s.log.Warn("database backup published with durability warning", "code", publicationWarning.Code)
		} else {
			if s.retryJobLater(job.ID, err) {
				return
			}
			message := "Database backup failed: " + err.Error()
			_ = s.setJobMessage(job.ID, "failed", 100, message)
			s.recordLog("error", message, map[string]string{"job": job.ID})
			return
		}
	}
	if err := ctx.Err(); err != nil {
		return
	}
	_ = s.setJobMessage(job.ID, "running", 82, "Applying backup retention policy.")
	removed, retentionErr := s.pruneDatabaseBackups(settings.BackupRetentionDays)
	if retentionErr != nil {
		s.log.Warn("backup retention failed", "error", retentionErr)
	}
	if err := ctx.Err(); err != nil {
		return
	}
	message := fmt.Sprintf("Database backup completed: %s", filepath.Base(path))
	if publicationWarning != nil {
		message += ". Durability confirmation is pending."
	}
	if removed > 0 {
		message = fmt.Sprintf("%s. Removed %d expired backups.", message, removed)
	}
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, map[string]string{"job": job.ID, "path": filepath.Base(path)})
}

func (s *Server) runLibraryTrashCleanup(ctx context.Context, job Job) {
	settings := s.scheduledTaskSettings()
	retentionDays := settings.TrashRetentionDays
	if job.ResourceType == "maintenance" && job.ResourceID == "manual" {
		retentionDays = 0
	}
	if err := ctx.Err(); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		s.recordLog("info", "Library trash cleanup cancelled.", map[string]string{"job": job.ID})
		return
	}
	_ = s.setJobMessage(job.ID, "running", 20, "Emptying expired library trash.")
	removed, err := s.emptyMissingMediaTrashForLibraryContext(ctx, "", retentionDays)
	if err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Library trash cleanup failed: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	}
	message := fmt.Sprintf("Library trash cleanup completed. Removed %d item%s.", removed, pluralSuffix(removed))
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, map[string]string{"job": job.ID, "removed": strconv.Itoa(removed)})
}

func (s *Server) runOptimizedVersionPrune(ctx context.Context, job Job) {
	if err := ctx.Err(); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		s.recordLog("info", "Optimized version pruning cancelled.", map[string]string{"job": job.ID})
		return
	}
	_ = s.setJobMessage(job.ID, "running", 20, "Applying optimized version retention.")
	removed, err := s.pruneOptimizedVersions()
	if err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Optimized version pruning failed: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	}
	message := fmt.Sprintf("Optimized version pruning completed. Removed %d version%s.", removed, pluralSuffix(removed))
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, map[string]string{"job": job.ID, "removed": strconv.Itoa(removed)})
}

func (s *Server) runTrickplayPrune(ctx context.Context, job Job) {
	settings := s.scheduledTaskSettings()
	if err := ctx.Err(); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		s.recordLog("info", "Trickplay pruning cancelled.", map[string]string{"job": job.ID})
		return
	}
	_ = s.setJobMessage(job.ID, "running", 20, "Applying trickplay retention.")
	removed, err := s.pruneTrickplaySets(settings.TrickplayRetentionDays, settings.TrickplayMaxStorageMB)
	if err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Trickplay pruning failed: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	}
	message := fmt.Sprintf("Trickplay pruning completed. Removed %d set%s.", removed, pluralSuffix(removed))
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, map[string]string{"job": job.ID, "removed": strconv.Itoa(removed)})
}

func (s *Server) createDatabaseBackup() (string, error) {
	if strings.TrimSpace(s.cfg.DatabasePath) == "" {
		return "", fmt.Errorf("database path is not configured")
	}
	if !s.sqliteExclusiveMaintenanceAllowed() {
		return "", fmt.Errorf("SQLite backup maintenance is deferred while playback or interactive work is active")
	}
	backupDir := s.backupDir()
	if !database.IsAppOwnedPath(s.cfg.AppDataDir, backupDir) {
		return "", fmt.Errorf("automatic backup creation is disabled for external storage; use an app-data backup root or an operator-managed encrypted export")
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", err
	}
	backupPath, releaseReservation, err := reserveUniqueBackupPath(backupDir)
	if err != nil {
		return "", err
	}
	defer releaseReservation()
	partialPath := backupPath + ".partial"
	if err := database.ValidateNoSymlinkPath(partialPath); err != nil {
		return "", fmt.Errorf("validate backup destination: %w", err)
	}
	if err := s.vacuumDatabaseBackupInto(partialPath); err != nil {
		if cleanupErr := cleanupFailedBackupPair(partialPath); cleanupErr != nil {
			s.log.Warn("failed to clean or quarantine partial database backup", "error", cleanupErr)
		}
		return "", err
	}
	if err := database.ProtectCreatedFile(partialPath); err != nil {
		_ = cleanupFailedBackupPair(partialPath)
		return "", err
	}
	if err := database.SyncFile(partialPath); err != nil {
		_ = cleanupFailedBackupPair(partialPath)
		return "", err
	}
	if err := s.writeDatabaseBackupManifest(partialPath, filepath.Base(backupPath)); err != nil {
		_ = cleanupFailedBackupPair(partialPath)
		return "", err
	}
	// Publish the manifest first. The final .db name is enumerable only after
	// both the database and its declared artifact metadata are durable.
	if err := database.ReplaceFileAtomically(backupManifestPath(partialPath), backupManifestPath(backupPath)); err != nil {
		_ = cleanupFailedBackupPair(partialPath)
		return "", err
	}
	if err := syncBackupDirectory(backupDir); err != nil {
		_ = cleanupFailedBackupPair(partialPath)
		_ = cleanupFailedBackupPair(backupPath)
		return "", err
	}
	// Publish a durable pending marker before the final database rename. The
	// marker is the conservative catalog authority if the final directory sync
	// fails or the process dies between the rename and that sync. A failure to
	// persist it aborts publication before the enumerable .db exists.
	pendingWarning := &BackupPublicationWarning{Code: "backup_directory_sync_pending", Message: "The backup pair is published and catalogued; directory durability confirmation is pending."}
	if err := persistBackupPublicationWarning(backupPath, pendingWarning); err != nil {
		_ = cleanupFailedBackupPair(partialPath)
		_ = cleanupFailedBackupPair(backupPath)
		return "", fmt.Errorf("persist backup publication evidence: %w", err)
	}
	if err := database.ReplaceFileAtomically(partialPath, backupPath); err != nil {
		_ = cleanupFailedBackupPair(partialPath)
		_ = cleanupFailedBackupPair(backupPath)
		return "", err
	}
	if err := syncBackupDirectory(backupDir); err != nil {
		return backupPath, pendingWarning
	}
	// Do not clear the pending evidence until the final database directory sync
	// has succeeded. If cleanup itself fails, the marker remains enumerable and
	// every later catalog read conservatively reports degraded publication.
	if err := clearBackupPublicationEvidence(backupPath); err != nil {
		return backupPath, pendingWarning
	}
	return backupPath, nil
}

// BackupPublicationWarning means the final database+manifest pair is already
// enumerable and validated, but a post-publication directory durability step
// could not be confirmed. It is a successful backup with degraded durability,
// not a failed creation; API, job, and catalog callers must report the same
// identity and warning.
type BackupPublicationWarning struct {
	Code    string
	Message string
}

func (w *BackupPublicationWarning) Error() string {
	if w == nil || w.Message == "" {
		return "backup published with a durability warning"
	}
	return w.Message
}

// cleanupFailedBackupPair removes only trusted regular artifacts and
// collision-safely quarantines anything that cannot be removed. A failed
// VACUUM INTO must not leave an unbounded near-full .partial file behind.
func cleanupFailedBackupPair(base string) error {
	var firstErr error
	for _, path := range []string{base + "-wal", base + "-shm", base + "-journal", backupPublicationEvidencePath(base), backupManifestPath(base), base} {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		removed := false
		if err := database.ValidateRegularNonSymlinkFile(path); err == nil {
			if err := removeBackupArtifact(path); err == nil {
				removed = true
			} else if firstErr == nil {
				firstErr = err
			}
		}
		if removed {
			continue
		}
		quarantine := path + ".restore-debris-" + strings.TrimPrefix(randomID("backup"), "backup_")
		if err := database.ReplaceFileAtomically(path, quarantine); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := syncBackupDirectory(filepath.Dir(path)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func reserveUniqueBackupPath(backupDir string) (string, func(), error) {
	for attempt := 0; attempt < 32; attempt++ {
		name := "portico-" + time.Now().UTC().Format("20060102-150405.000000000") + "-" + strings.TrimPrefix(randomID("backup"), "backup_") + ".db"
		path := filepath.Join(backupDir, name)
		reservation := path + ".reserve"
		if err := os.Mkdir(reservation, 0o700); err == nil {
			ownerLock, lockErr := database.AcquireRestoreArtifactLock(filepath.Join(reservation, "owner.lock"))
			if lockErr != nil {
				_ = os.Remove(reservation)
				continue
			}
			_ = syncBackupDirectory(backupDir)
			return path, func() {
				ownerLock()
				_ = os.Remove(filepath.Join(reservation, "owner.lock"))
				_ = os.Remove(reservation)
				_ = syncBackupDirectory(backupDir)
			}, nil
		} else if !os.IsExist(err) {
			return "", func() {}, err
		}
	}
	return "", func() {}, fmt.Errorf("unable to reserve a collision-proof backup name")
}

func (s *Server) pruneDatabaseBackups(retentionDays int) (int, error) {
	backupDir := s.backupDir()
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(max(0, retentionDays)) * 24 * time.Hour)
	excluded := map[string]bool{}
	if operation, operationErr := database.ReadRestoreOperation(s.cfg.AppDataDir); operationErr == nil {
		for _, path := range []string{operation.SourcePath, operation.StagedPath, operation.SafetyCopyPath, operation.OldActivePath, operation.InstallPath} {
			if strings.TrimSpace(path) != "" {
				excluded[filepath.Clean(path)] = true
				excluded[filepath.Clean(path)+".manifest.json"] = true
			}
		}
	}
	type recoveryPoint struct {
		path     string
		manifest string
		created  time.Time
	}
	type invalidCandidate struct {
		path    string
		created time.Time
		size    int64
	}
	points := make([]recoveryPoint, 0, len(entries))
	invalid := make([]invalidCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".manifest.json") || !isSafeBackupName(entry.Name()) {
			continue
		}
		path := filepath.Join(backupDir, entry.Name())
		if excluded[filepath.Clean(path)] {
			continue
		}
		info, infoErr := s.backupInfo(path)
		if infoErr != nil || !info.RestoreReady || !info.ManifestPresent {
			// Invalid recognizable candidates remain visible to the catalog and
			// are retained for a bounded diagnostic grace period before the
			// pair-aware debris policy quarantines/removes them.
			created := time.Time{}
			if parsed, parseErr := time.Parse(time.RFC3339Nano, info.CreatedAt); parseErr == nil {
				created = parsed
			} else if stat, statErr := os.Stat(path); statErr == nil {
				created = stat.ModTime().UTC()
			}
			invalidSize := info.SizeBytes
			if invalidSize < 0 {
				invalidSize = 0
			}
			invalid = append(invalid, invalidCandidate{path: path, created: created, size: invalidSize})
			continue
		}
		created, parseErr := time.Parse(time.RFC3339Nano, info.CreatedAt)
		if parseErr != nil {
			if stat, statErr := os.Stat(path); statErr == nil {
				created = stat.ModTime().UTC()
			} else {
				continue
			}
		}
		points = append(points, recoveryPoint{path: path, manifest: backupManifestPath(path), created: created})
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].created.Equal(points[j].created) {
			return points[i].path > points[j].path
		}
		return points[i].created.After(points[j].created)
	})
	removed := 0
	var firstErr error
	const invalidGrace = 7 * 24 * time.Hour
	invalidGraceCutoff := time.Now().UTC().Add(-invalidGrace)
	// Always retain the newest verified recovery point, even when every point
	// is older than the configured retention window or the clock moved back.
	for index, point := range points {
		if index == 0 || point.created.After(cutoff) {
			continue
		}
		if err := removeDatabaseBackupPair(point.path, point.manifest); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		removed++
	}
	// Remove old, unclaimed publication debris and manifest-only orphans. Do
	// not touch the active operation's paths or an in-progress reservation.
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(backupDir, name)
		if excluded[filepath.Clean(path)] || strings.HasSuffix(name, ".reserve") || entry.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".partial") || strings.Contains(name, ".tmp-") {
			if info, statErr := entry.Info(); statErr == nil && info.ModTime().UTC().Before(cutoff) {
				if removeErr := cleanupFailedBackupPair(path); removeErr != nil && firstErr == nil {
					firstErr = removeErr
				}
			}
			continue
		}
		if strings.HasSuffix(name, ".manifest.json") {
			base := strings.TrimSuffix(path, ".manifest.json")
			if _, statErr := os.Lstat(base); os.IsNotExist(statErr) {
				if info, infoErr := entry.Info(); infoErr == nil && info.ModTime().UTC().Before(invalidGraceCutoff) {
					if removeErr := removeBackupArtifact(path); removeErr != nil && firstErr == nil {
						firstErr = removeErr
					} else if removeErr == nil {
						removed++
					}
				}
			}
		} else if strings.HasSuffix(name, ".publication.json") {
			base := strings.TrimSuffix(path, ".publication.json")
			if _, statErr := os.Lstat(base); os.IsNotExist(statErr) {
				if info, infoErr := entry.Info(); infoErr == nil && info.ModTime().UTC().Before(invalidGraceCutoff) {
					if removeErr := removeBackupArtifact(path); removeErr != nil && firstErr == nil {
						firstErr = removeErr
					} else if removeErr == nil {
						removed++
					}
				}
			}
		}
	}
	// Invalid recognizable candidates remain enumerable through listBackups for
	// the grace window, but cannot consume storage forever. Keep a bounded
	// newest diagnostic set, then remove/quarantine the entire artifact pair.
	sort.Slice(invalid, func(i, j int) bool {
		if invalid[i].created.Equal(invalid[j].created) {
			return invalid[i].path > invalid[j].path
		}
		return invalid[i].created.After(invalid[j].created)
	})
	const invalidKeep = 32
	const invalidBytesKeep = int64(512 << 20)
	now := time.Now().UTC()
	keptInvalidBytes := int64(0)
	for index, candidate := range invalid {
		withinGrace := !candidate.created.IsZero() && now.Sub(candidate.created) >= 0 && now.Sub(candidate.created) <= invalidGrace
		withinByteCap := candidate.size <= invalidBytesKeep-keptInvalidBytes
		if index < invalidKeep && withinGrace && withinByteCap {
			keptInvalidBytes += candidate.size
			continue
		}
		if err := cleanupFailedBackupPair(candidate.path); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		removed++
	}
	// Quarantine names are deliberately outside the normal backup catalog, but
	// they can still be database-sized after a failed delete. Keep only a
	// bounded, recent diagnostic set under the trusted backup prefix and never
	// remove an artifact whose source is selected by an active restore or has a
	// live reservation.
	type debrisCandidate struct {
		path    string
		created time.Time
		size    int64
	}
	debris := make([]debrisCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		marker := strings.Index(name, ".restore-debris-")
		if marker <= 0 || !strings.HasPrefix(name, "portico-") {
			continue
		}
		sourceName := name[:marker]
		sourcePath := filepath.Join(backupDir, sourceName)
		if excluded[filepath.Clean(sourcePath)] || excluded[filepath.Clean(sourcePath)+".manifest.json"] {
			continue
		}
		if _, reserveErr := os.Stat(sourcePath + ".reserve"); reserveErr == nil {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		size := info.Size()
		if size < 0 {
			size = 0
		}
		debris = append(debris, debrisCandidate{path: filepath.Join(backupDir, name), created: info.ModTime().UTC(), size: size})
	}
	sort.Slice(debris, func(i, j int) bool {
		if debris[i].created.Equal(debris[j].created) {
			return debris[i].path > debris[j].path
		}
		return debris[i].created.After(debris[j].created)
	})
	const debrisKeep = 32
	const debrisBytesKeep = int64(512 << 20)
	const debrisGrace = 7 * 24 * time.Hour
	debrisCutoff := time.Now().UTC().Add(-debrisGrace)
	keptDebrisBytes := int64(0)
	for index, candidate := range debris {
		withinGrace := !candidate.created.IsZero() && !candidate.created.Before(debrisCutoff) && !candidate.created.After(time.Now().UTC().Add(time.Minute))
		withinByteCap := candidate.size <= debrisBytesKeep-keptDebrisBytes
		if index < debrisKeep && withinGrace && withinByteCap {
			keptDebrisBytes += candidate.size
			continue
		}
		if removeErr := removeBackupArtifact(candidate.path); removeErr != nil {
			if firstErr == nil {
				firstErr = removeErr
			}
			continue
		}
		removed++
	}
	if err := s.pruneStaleBackupReservations(backupDir, entries, cutoff); err != nil && firstErr == nil {
		firstErr = err
	}
	return removed, firstErr
}

func (s *Server) pruneStaleBackupReservations(backupDir string, entries []os.DirEntry, cutoff time.Time) error {
	var firstErr error
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".reserve") {
			continue
		}
		reservation := filepath.Join(backupDir, entry.Name())
		info, err := entry.Info()
		if err != nil || info.ModTime().UTC().After(cutoff) {
			continue
		}
		ownerLock := filepath.Join(reservation, "owner.lock")
		release, acquired, lockErr := database.TryAcquireRestoreArtifactLock(ownerLock)
		if lockErr != nil {
			if firstErr == nil {
				firstErr = lockErr
			}
			continue
		}
		if !acquired {
			// A live backup creator still owns the reservation; never remove it.
			continue
		}
		release()
		if err := os.Remove(ownerLock); err != nil && !os.IsNotExist(err) {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := os.Remove(reservation); err != nil && !os.IsNotExist(err) {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := syncBackupDirectory(backupDir); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func removeBackupArtifact(path string) error {
	if err := database.ValidateRegularNonSymlinkFile(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := removeBackupFile(path); err != nil {
		return err
	}
	return syncBackupDirectory(filepath.Dir(path))
}

func removeDatabaseBackupPair(databasePath, manifestPath string) error {
	// Unpublish the manifest first, then remove sidecars and the enumerable
	// database last. A crash at any boundary leaves at most an unmanifested
	// database candidate, never a manifest that can make a partially-deleted
	// pair look restore-ready. Pair cleanup is surfaced as degraded when any
	// member cannot be removed.
	if err := removeBackupArtifact(databasePath + ".publication.json"); err != nil {
		return fmt.Errorf("remove backup publication evidence: %w", err)
	}
	if err := removeBackupArtifact(manifestPath); err != nil {
		return fmt.Errorf("remove backup manifest: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := removeBackupArtifact(databasePath + suffix); err != nil {
			return fmt.Errorf("remove backup sidecar %s: %w", suffix, err)
		}
	}
	if err := removeBackupArtifact(databasePath); err != nil {
		return fmt.Errorf("remove backup database: %w", err)
	}
	return nil
}

func (s *Server) jobAlreadyQueuedToday(jobType, dayKey string) bool {
	var count int
	like := dayKey + "%"
	if err := s.queryBackgroundRow(context.Background(), `SELECT COUNT(*) FROM jobs WHERE type = ? AND created_at LIKE ?`, jobType, like).Scan(&count); err != nil {
		s.log.Warn("scheduled job duplicate check failed", "type", jobType, "error", err)
		return true
	}
	return count > 0
}

func (s *Server) jobAlreadyQueuedTodayFor(jobType, resourceType, resourceID, dayKey string) bool {
	var count int
	like := dayKey + "%"
	if err := s.queryBackgroundRow(context.Background(), `SELECT COUNT(*) FROM jobs WHERE type = ? AND resource_type = ? AND resource_id = ? AND created_at LIKE ?`, jobType, resourceType, resourceID, like).Scan(&count); err != nil {
		s.log.Warn("scheduled resource job duplicate check failed", "type", jobType, "resourceType", resourceType, "resourceID", resourceID, "error", err)
		return true
	}
	return count > 0
}

func (s *Server) jobAlreadyQueuedWithin(jobType, resourceType, resourceID string, intervalHours int) bool {
	cutoff := time.Now().UTC().Add(-time.Duration(max(1, intervalHours)) * time.Hour).Format(time.RFC3339)
	query := `SELECT COUNT(*) FROM jobs WHERE type = ? AND created_at >= ?`
	args := []any{jobType, cutoff}
	if strings.TrimSpace(resourceType) != "" {
		query += ` AND resource_type = ?`
		args = append(args, resourceType)
	}
	if strings.TrimSpace(resourceID) != "" {
		query += ` AND resource_id = ?`
		args = append(args, resourceID)
	}
	var count int
	if err := s.queryBackgroundRow(context.Background(), query, args...).Scan(&count); err != nil {
		s.log.Warn("scheduled job interval check failed", "type", jobType, "error", err)
		return true
	}
	return count > 0
}

func (s *Server) libraryMediaAnalysisRecentlyQueued(libraryID string, within time.Duration) bool {
	if strings.TrimSpace(libraryID) == "" {
		return true
	}
	if within <= 0 {
		within = time.Hour
	}
	cutoff := time.Now().UTC().Add(-within).Format(time.RFC3339)
	var count int
	if err := s.queryBackgroundRow(context.Background(), `
		SELECT COUNT(*)
		FROM jobs j
		JOIN media_items m ON m.id = j.resource_id
		WHERE j.type = 'media_analyze'
			AND j.resource_type = 'media'
			AND m.library_id = ?
			AND j.created_at >= ?`, libraryID, cutoff).Scan(&count); err != nil {
		s.log.Warn("scheduled media analysis interval check failed", "library", libraryID, "error", err)
		return true
	}
	return count > 0
}

func (s *Server) reserveScheduledJob(jobType, resourceType, resourceID, dayKey string) bool {
	key := strings.Join([]string{jobType, resourceType, resourceID, dayKey}, "\x00")
	s.scheduledJobMu.Lock()
	defer s.scheduledJobMu.Unlock()
	if s.scheduledJobs == nil {
		s.scheduledJobs = map[string]bool{}
	}
	if s.scheduledJobReservationAt == nil {
		s.scheduledJobReservationAt = map[string]time.Time{}
	}
	now := time.Now().UTC()
	const maxReservations = 1024
	const reservationTTL = 48 * time.Hour
	for existing, seenAt := range s.scheduledJobReservationAt {
		if now.Sub(seenAt) > reservationTTL {
			delete(s.scheduledJobReservationAt, existing)
			delete(s.scheduledJobs, existing)
		}
	}
	if s.scheduledJobs[key] {
		return false
	}
	if len(s.scheduledJobReservationAt) >= maxReservations {
		oldestKey := ""
		oldestAt := now
		for existing, seenAt := range s.scheduledJobReservationAt {
			if oldestKey == "" || seenAt.Before(oldestAt) {
				oldestKey = existing
				oldestAt = seenAt
			}
		}
		if oldestKey != "" {
			delete(s.scheduledJobReservationAt, oldestKey)
			delete(s.scheduledJobs, oldestKey)
		}
	}
	s.scheduledJobs[key] = true
	s.scheduledJobReservationAt[key] = now
	return true
}

func (s *Server) releaseScheduledJob(jobType, resourceType, resourceID, dayKey string) {
	key := strings.Join([]string{jobType, resourceType, resourceID, dayKey}, "\x00")
	s.scheduledJobMu.Lock()
	delete(s.scheduledJobs, key)
	delete(s.scheduledJobReservationAt, key)
	s.scheduledJobMu.Unlock()
}

func withinScheduledWindow(now time.Time, startHour, endHour int) bool {
	hour := now.Hour()
	if startHour == endHour {
		return true
	}
	if startHour < endHour {
		return hour >= startHour && hour < endHour
	}
	return hour >= startHour || hour < endHour
}

func withinScheduledDays(now time.Time, days string) bool {
	weekday := now.Weekday()
	isWeekend := weekday == time.Saturday || weekday == time.Sunday
	switch normalizeMaintenanceDays(days) {
	case "weekdays":
		return !isWeekend
	case "weekends":
		return isWeekend
	default:
		return true
	}
}

func clampHour(value int) int {
	if value < 0 {
		return 0
	}
	if value > 23 {
		return 23
	}
	return value
}
