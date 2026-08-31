package app

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

const (
	dvrMinimumRecordingDuration = time.Second
	dvrMinimumRecordingBytes    = int64(1024)
	dvrPlaybackRetentionGrace   = 10 * time.Minute
)

func (s *Server) acquireDVRPlaybackUse(recordingID string) func(bool) {
	recordingID = strings.TrimSpace(recordingID)
	if recordingID == "" {
		return func(bool) {}
	}
	s.dvrPlaybackMu.Lock()
	if s.dvrPlaybackActive == nil {
		s.dvrPlaybackActive = map[string]int{}
		s.dvrPlaybackLastSeen = map[string]time.Time{}
	}
	s.dvrPlaybackActive[recordingID]++
	s.dvrPlaybackMu.Unlock()
	return func(retainGrace bool) {
		s.dvrPlaybackMu.Lock()
		s.dvrPlaybackActive[recordingID]--
		if s.dvrPlaybackActive[recordingID] <= 0 {
			delete(s.dvrPlaybackActive, recordingID)
		}
		if retainGrace {
			s.dvrPlaybackLastSeen[recordingID] = time.Now().UTC()
		} else if s.dvrPlaybackActive[recordingID] == 0 {
			delete(s.dvrPlaybackLastSeen, recordingID)
		}
		s.dvrPlaybackMu.Unlock()
	}
}

func (s *Server) runDVRScheduler(ctx context.Context) {
	if err := s.reconcileDVRStateAfterRestart(ctx, time.Now().UTC()); err != nil {
		s.log.Warn("DVR startup reconciliation failed", "error", err)
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		s.queueDueDVRRecordings()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type dvrRestartCandidate struct {
	id        string
	status    string
	startsAt  string
	endsAt    string
	path      string
	heartbeat string
}

func dvrGuideGenerationPredicate(alias string) string {
	return alias + ".guide_generation = COALESCE((SELECT source_generation.active_import_generation FROM live_tv_sources source_generation WHERE source_generation.id = " + alias + ".source_id), '')"
}

// bindDVRGuideGenerationTx keeps scheduling decisions attached to the same
// source generation that Live TV publishes. Scheduled recordings are rebound
// and revisioned for the new guide; completed or in-flight artifacts retain
// their historical decision context.
func bindDVRGuideGenerationTx(tx *sql.Tx, sourceID, generation, now string) error {
	sourceID = strings.TrimSpace(sourceID)
	generation = strings.TrimSpace(generation)
	if sourceID == "" || generation == "" {
		return nil
	}
	if _, err := tx.Exec(`
		UPDATE live_tv_recording_rules
		SET guide_generation = ?, revision = revision + 1, updated_at = ?
		WHERE source_id = ? AND guide_generation <> ?`, generation, now, sourceID, generation); err != nil {
		return err
	}
	_, err := tx.Exec(`
		UPDATE live_tv_recordings
		SET guide_generation = ?, revision = revision + 1, updated_at = ?
		WHERE source_id = ? AND status = 'scheduled' AND guide_generation <> ?`, generation, now, sourceID, generation)
	return err
}

func (s *Server) reconcileDVRStateAfterRestart(ctx context.Context, now time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.queryBackgroundRead(ctx, `
		SELECT r.id, r.status, r.starts_at, r.ends_at, r.path, COALESCE(a.heartbeat_at, '')
		FROM live_tv_recordings r
		JOIN live_tv_sources source_generation ON source_generation.id = r.source_id AND `+dvrGuideGenerationPredicate("r")+`
		LEFT JOIN live_tv_tuner_allocations a
			ON a.allocation_kind = 'dvr_recording' AND a.consumer_id = r.id
		WHERE r.status IN ('running', 'scheduled')
		ORDER BY r.starts_at ASC, r.id ASC`)
	if err != nil {
		return err
	}
	candidates := []dvrRestartCandidate{}
	for rows.Next() {
		var candidate dvrRestartCandidate
		if err := rows.Scan(&candidate.id, &candidate.status, &candidate.startsAt, &candidate.endsAt, &candidate.path, &candidate.heartbeat); err != nil {
			_ = rows.Close()
			return err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	nowText := now.Format(time.RFC3339)
	for _, candidate := range candidates {
		if candidate.status == "running" {
			heartbeat, _ := time.Parse(time.RFC3339, candidate.heartbeat)
			if !heartbeat.IsZero() && now.Sub(heartbeat) < liveTVAllocationStaleAfter {
				// Another process still owns a fresh recording lease. Startup and
				// periodic repair must not touch its row or partial artifact.
				continue
			}
			// The process that owned this stale lease is gone. Remove the
			// persisted allocation before changing the recording row so a resumed
			// job can claim capacity immediately and an old worker cannot release
			// the replacement lease by consumer ID.
			if _, err := s.execBackgroundWrite(ctx, `DELETE FROM live_tv_tuner_allocations WHERE allocation_kind = 'dvr_recording' AND consumer_id = ?`, candidate.id); err != nil {
				return err
			}
			// A failure path publishes to a distinguishable incomplete filename
			// before changing the row. Recover that fenced artifact after a crash;
			// never interpret it as proof of successful completion.
			if incompletePath, ok := dvrIncompletePathFromLeaseWorkingPath(candidate.path); ok {
				if _, statErr := os.Stat(incompletePath); errors.Is(statErr, os.ErrNotExist) {
					if workingInfo, workingErr := os.Stat(candidate.path); workingErr == nil && workingInfo.Mode().IsRegular() && workingInfo.Size() >= 376 && s.probeDVRPartialMedia(candidate.path) {
						_ = os.Rename(candidate.path, incompletePath)
					}
				}
				if info, statErr := os.Stat(incompletePath); statErr == nil && info.Mode().IsRegular() && info.Size() > 0 {
					if _, updateErr := s.execBackgroundWrite(ctx, `
						UPDATE live_tv_recordings SET status = 'incomplete', path = ?, size_bytes = ?,
							error = 'Recording output was recovered after an interrupted failure transition.',
							failure_code = 'interrupted_restart', revision = revision + 1, updated_at = ?
						WHERE id = ? AND status = 'running'`, incompletePath, info.Size(), nowText, candidate.id); updateErr != nil {
						return updateErr
					}
					if recording, loadErr := s.getDVRRecording(candidate.id); loadErr == nil {
						_ = s.importDVRRecordingMedia(recording, nowText)
					}
					continue
				}
			}
			// FFmpeg writes to a lease-specific .partial path and renames only
			// after a successful exit. A non-partial artifact therefore proves the
			// provider job completed even if the process crashed before the row
			// transition; finalize it instead of deleting valid recorded bytes.
			if completedPath, ok := dvrFinalPathFromLeaseWorkingPath(candidate.path); ok {
				if info, statErr := os.Stat(completedPath); statErr == nil && info.Mode().IsRegular() && info.Size() > 0 {
					if _, updateErr := s.execBackgroundWrite(ctx, `
						UPDATE live_tv_recordings SET status = 'complete', path = ?, size_bytes = ?, error = '', failure_code = '', revision = revision + 1, updated_at = ?
						WHERE id = ? AND status = 'running'`, completedPath, info.Size(), nowText, candidate.id); updateErr != nil {
						return updateErr
					}
					if recording, loadErr := s.getDVRRecording(candidate.id); loadErr == nil {
						_ = s.importDVRRecordingMedia(recording, nowText)
					}
					s.recordLog("info", "DVR recording finalized from a completed restart artifact.", map[string]string{"recording": candidate.id})
					continue
				}
			}
		}
		end, parseErr := time.Parse(time.RFC3339, candidate.endsAt)
		if parseErr != nil || !end.After(now) {
			if candidate.path != "" {
				if err := removeDVRRecordingFile(candidate.path, s.cfg.AppDataDir); err != nil {
					s.log.Warn("DVR partial artifact cleanup failed", "recording", candidate.id, "error", err)
				}
			}
			failureCode := "missed_window"
			message := "The recording window ended while Portico was offline."
			if candidate.status == "running" {
				failureCode = "interrupted_restart"
				message = "The recording was interrupted by a server restart and its unfinished artifact was removed."
			}
			if _, err := s.execBackgroundWrite(ctx, `
				UPDATE live_tv_recordings
				SET status = 'failed', path = '', size_bytes = 0, error = ?, failure_code = ?, revision = revision + 1, updated_at = ?
				WHERE id = ? AND status = ?`, message, failureCode, nowText, candidate.id, candidate.status); err != nil {
				return err
			}
			s.recordLog("warn", "DVR recording could not be recovered after restart.", map[string]string{"recording": candidate.id, "failureCode": failureCode})
			continue
		}
		if candidate.status != "running" {
			continue
		}
		// A row is only marked complete after FFmpeg exits successfully. Therefore
		// any artifact attached to a persisted running row is untrusted partial
		// output. Remove it and resume the remaining provider window from now.
		if candidate.path != "" {
			if err := removeDVRRecordingFile(candidate.path, s.cfg.AppDataDir); err != nil {
				return err
			}
		}
		if _, err := s.execBackgroundWrite(ctx, `
			UPDATE live_tv_recordings
			SET status = 'scheduled', starts_at = ?, path = '', size_bytes = 0,
				error = 'Recording resumed after a server restart.', failure_code = '', revision = revision + 1, updated_at = ?
			WHERE id = ? AND status = 'running'`, nowText, nowText, candidate.id); err != nil {
			return err
		}
		s.recordLog("info", "DVR recording queued to resume after restart.", map[string]string{"recording": candidate.id})
	}
	// Completion and catalog import are deliberately separate transactions so a
	// media-index failure cannot roll a valid recording back to running. Repair
	// the small crash window idempotently on startup and every scheduler pass.
	if err := s.reconcileCompletedDVRMedia(ctx, nowText); err != nil {
		return err
	}
	return nil
}

func (s *Server) reconcileCompletedDVRMedia(ctx context.Context, importedAt string) error {
	rows, err := s.queryBackgroundRead(ctx, `
		SELECT r.id
		FROM live_tv_recordings r
		LEFT JOIN dvr_recording_media mapping ON mapping.recording_id = r.id
		WHERE lower(r.status) IN ('complete', 'completed')
			AND r.path <> '' AND mapping.recording_id IS NULL
		ORDER BY r.updated_at ASC, r.id ASC
		LIMIT 100`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		recording, loadErr := s.getDVRRecording(id)
		if loadErr != nil {
			s.log.Warn("DVR catalog reconciliation could not reload recording", "recording", id, "error", loadErr)
			continue
		}
		if importErr := s.importDVRRecordingMedia(recording, importedAt); importErr != nil {
			s.log.Warn("DVR catalog reconciliation deferred recording import", "recording", id, "error", importErr)
			continue
		}
		s.recordLog("info", "DVR recording restored to the media catalog.", map[string]string{"recording": id})
	}
	return nil
}

func (s *Server) queueDueDVRRecordings() {
	if err := s.reconcileDVRStateAfterRestart(context.Background(), time.Now().UTC()); err != nil {
		s.log.Warn("DVR state reconciliation failed", "error", err)
	}
	s.queueDVRRetentionCleanup()
	now := time.Now().UTC().Format(time.RFC3339)
	evaluatedDueRecordings := 0
	defer func() {
		if evaluatedDueRecordings > 0 {
			s.log.Info("DVR scheduler evaluated overdue recordings", "overdueRecordings", evaluatedDueRecordings, "activeWorkers", s.dvrActiveWorkers.Load())
		}
	}()
	afterPriority, afterStartsAt, afterID := 101, "", ""
	for {
		rows, err := s.queryBackgroundRead(context.Background(), `
			SELECT r.id, r.priority, r.starts_at
			FROM live_tv_recordings r
			JOIN live_tv_sources source_generation ON source_generation.id = r.source_id AND `+dvrGuideGenerationPredicate("r")+`
			WHERE r.status = 'scheduled' AND r.starts_at <= ? AND r.ends_at > ?
				AND (? = '' OR r.priority < ? OR (r.priority = ? AND r.starts_at > ?) OR (r.priority = ? AND r.starts_at = ? AND r.id > ?))
			ORDER BY r.priority DESC, r.starts_at ASC, r.id ASC
			LIMIT 256`, now, now, afterID, afterPriority, afterPriority, afterStartsAt, afterPriority, afterStartsAt, afterID)
		if err != nil {
			s.log.Warn("dvr schedule query failed", "error", err)
			return
		}
		type dueRecording struct {
			id, startsAt string
			priority     int
		}
		batch := []dueRecording{}
		for rows.Next() {
			var next dueRecording
			if err := rows.Scan(&next.id, &next.priority, &next.startsAt); err == nil {
				batch = append(batch, next)
			}
		}
		if err := rows.Close(); err != nil {
			s.log.Warn("dvr schedule close failed", "error", err)
			return
		}
		if len(batch) == 0 {
			return
		}
		evaluatedDueRecordings += len(batch)
		for _, due := range batch {
			if !s.tryAcquireDVRWorker() {
				return
			}
			if lease, claimed := s.claimDVRRecordingLease(due.id); claimed {
				id := due.id
				if !s.startBackground("dvr-recording", func(ctx context.Context) {
					defer func() {
						s.dvrActiveWorkers.Add(-1)
						s.queueDueDVRRecordings()
					}()
					s.runDVRRecording(ctx, id, lease.Token)
				}) {
					s.dvrActiveWorkers.Add(-1)
					s.releaseLiveTVTunerAllocationLease(context.Background(), "dvr_recording", id, lease.Token)
				}
			} else {
				s.dvrActiveWorkers.Add(-1)
			}
		}
		last := batch[len(batch)-1]
		afterPriority, afterStartsAt, afterID = last.priority, last.startsAt, last.id
		if len(batch) < 256 {
			return
		}
	}
}

const maxConcurrentDVRWorkers = 32

const dvrOutputProgressTimeout = 45 * time.Second

var errDVROutputStalled = errors.New("DVR output stalled without file progress")

func (s *Server) tryAcquireDVRWorker() bool {
	for {
		active := s.dvrActiveWorkers.Load()
		if active >= maxConcurrentDVRWorkers {
			return false
		}
		if s.dvrActiveWorkers.CompareAndSwap(active, active+1) {
			return true
		}
	}
}

func (s *Server) queueDVRRetentionCleanup() {
	const intervalHours = 24
	if s.jobAlreadyQueuedWithin("dvr_retention_cleanup", "maintenance", "dvr", intervalHours) {
		return
	}
	if _, err := s.createJobFor("dvr_retention_cleanup", "DVR retention cleanup queued.", "maintenance", "dvr"); err != nil {
		s.log.Warn("dvr retention cleanup queue failed", "error", err)
	}
}

func (s *Server) runDVRRetentionCleanup(ctx context.Context, job Job) {
	if err := ctx.Err(); err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		s.recordLog("info", "DVR retention cleanup cancelled.", map[string]string{"job": job.ID})
		return
	}
	_ = s.setJobMessage(job.ID, "running", 20, "Removing expired DVR recordings.")
	removed, err := s.pruneExpiredDVRRecordings()
	if err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "DVR retention cleanup failed: " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("error", message, map[string]string{"job": job.ID})
		return
	}
	message := fmt.Sprintf("DVR retention cleanup completed. Removed %d recording%s.", removed, pluralSuffix(removed))
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, map[string]string{"job": job.ID, "removed": strconv.Itoa(removed)})
}

type dvrCleanupCandidate struct {
	id                     string
	path                   string
	endsAt                 string
	retentionDays          int
	maxRecordingsPerSeries int
	ruleID                 string
	title                  string
	folder                 string
	startsAt               string
}

func (s *Server) pruneExpiredDVRRecordings() (int, error) {
	rows, err := s.queryBackgroundRead(context.Background(), `
		SELECT r.id, r.path, r.ends_at, COALESCE(rule.retention_days, 30),
			COALESCE(rule.max_recordings_per_series, 0), COALESCE(r.rule_id, ''), r.title, COALESCE(r.folder, ''), r.starts_at
		FROM live_tv_recordings r
		LEFT JOIN live_tv_recording_rules rule ON rule.id = r.rule_id
		WHERE r.status IN ('complete', 'completed', 'incomplete', 'failed')`)
	if err != nil {
		return 0, err
	}
	candidates := []dvrCleanupCandidate{}
	for rows.Next() {
		var next dvrCleanupCandidate
		if err := rows.Scan(&next.id, &next.path, &next.endsAt, &next.retentionDays, &next.maxRecordingsPerSeries, &next.ruleID, &next.title, &next.folder, &next.startsAt); err != nil {
			_ = rows.Close()
			return 0, err
		}
		candidates = append(candidates, next)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	removed := 0
	now := time.Now().UTC()
	removeIDs := map[string]bool{}
	for _, candidate := range candidates {
		if candidate.retentionDays <= 0 {
			continue
		}
		endedAt, err := time.Parse(time.RFC3339, candidate.endsAt)
		if err != nil || now.Before(endedAt.Add(time.Duration(candidate.retentionDays)*24*time.Hour)) {
			continue
		}
		removeIDs[candidate.id] = true
	}
	for id := range dvrRecordingsOverSeriesCaps(candidates) {
		removeIDs[id] = true
	}
	for _, candidate := range candidates {
		if !removeIDs[candidate.id] {
			continue
		}
		s.dvrPlaybackMu.Lock()
		inUse, err := s.dvrRecordingInUseLocked(context.Background(), candidate.id)
		if err != nil {
			s.dvrPlaybackMu.Unlock()
			return removed, err
		}
		if inUse {
			s.dvrPlaybackMu.Unlock()
			// Retention is periodic, so skipping is a durable pending-delete state:
			// the same candidate will be reconsidered after playback ends.
			continue
		}
		if candidate.path != "" {
			if err := removeDVRRecordingFile(candidate.path, s.cfg.AppDataDir); err != nil {
				s.dvrPlaybackMu.Unlock()
				return removed, err
			}
		}
		var result sql.Result
		err = s.withBackgroundTxTagged(context.Background(), []string{"dvr", "live-tv"}, func(tx *sql.Tx) error {
			if err := deleteImportedDVRRecordingMedia(tx, candidate.id); err != nil {
				return err
			}
			var execErr error
			result, execErr = tx.Exec(`DELETE FROM live_tv_recordings WHERE id = ? AND status IN ('complete', 'completed', 'incomplete', 'failed')`, candidate.id)
			return execErr
		})
		if err != nil {
			s.dvrPlaybackMu.Unlock()
			return removed, err
		}
		affected, _ := result.RowsAffected()
		removed += int(affected)
		s.dvrPlaybackMu.Unlock()
	}
	return removed, nil
}

func (s *Server) dvrRecordingInUse(ctx context.Context, recordingID string) (bool, error) {
	s.dvrPlaybackMu.Lock()
	defer s.dvrPlaybackMu.Unlock()
	return s.dvrRecordingInUseLocked(ctx, recordingID)
}

// dvrRecordingInUseLocked requires dvrPlaybackMu. Retention holds that lock
// through file and database deletion, making admission and deletion atomic.
func (s *Server) dvrRecordingInUseLocked(ctx context.Context, recordingID string) (bool, error) {
	now := time.Now().UTC()
	activeRequests := s.dvrPlaybackActive[recordingID]
	lastSeen := s.dvrPlaybackLastSeen[recordingID]
	if activeRequests == 0 && !lastSeen.IsZero() && now.Sub(lastSeen) >= dvrPlaybackRetentionGrace {
		delete(s.dvrPlaybackLastSeen, recordingID)
	}
	if activeRequests > 0 || !lastSeen.IsZero() && now.Sub(lastSeen) < dvrPlaybackRetentionGrace {
		return true, nil
	}
	var active int
	err := s.queryBackgroundRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM dvr_recording_media mapping
			JOIN playback_sessions session ON session.media_id = mapping.media_id
			WHERE mapping.recording_id = ?
				AND session.ended_at = '' AND lower(session.state) <> 'stopped'
		)`, recordingID).Scan(&active)
	if err != nil || active != 0 {
		return active != 0, err
	}
	var mediaID string
	if err := s.queryBackgroundRow(ctx, `SELECT media_id FROM dvr_recording_media WHERE recording_id = ?`, recordingID).Scan(&mediaID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	s.transcodeMu.Lock()
	defer s.transcodeMu.Unlock()
	for _, session := range s.transcodes {
		if session != nil && session.mediaID == mediaID && !session.snapshot().stopped {
			return true, nil
		}
	}
	return false, nil
}

func dvrRecordingsOverSeriesCaps(candidates []dvrCleanupCandidate) map[string]bool {
	groups := map[string][]dvrCleanupCandidate{}
	caps := map[string]int{}
	for _, candidate := range candidates {
		if candidate.maxRecordingsPerSeries <= 0 {
			continue
		}
		key := strings.TrimSpace(candidate.ruleID)
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(candidate.folder)) + "\x00" + strings.ToLower(strings.TrimSpace(candidate.title))
		}
		if key == "\x00" {
			continue
		}
		groups[key] = append(groups[key], candidate)
		if current := caps[key]; current == 0 || candidate.maxRecordingsPerSeries < current {
			caps[key] = candidate.maxRecordingsPerSeries
		}
	}
	removeIDs := map[string]bool{}
	for key, recordings := range groups {
		cap := caps[key]
		if cap <= 0 || len(recordings) <= cap {
			continue
		}
		sort.SliceStable(recordings, func(i, j int) bool {
			return recordings[i].startsAt > recordings[j].startsAt
		})
		for _, recording := range recordings[cap:] {
			removeIDs[recording.id] = true
		}
	}
	return removeIDs
}

func dvrRecordingFinishedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "complete", "completed", "incomplete", "failed":
		return true
	default:
		return false
	}
}

func deleteImportedDVRRecordingMedia(tx *sql.Tx, recordingID string) error {
	mediaID := dvrRecordingMediaID(recordingID)
	if _, err := tx.Exec(`DELETE FROM dvr_recording_media WHERE recording_id = ?`, recordingID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM media_streams WHERE media_id = ?`, mediaID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM media_search WHERE media_id = ?`, mediaID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM media_items WHERE id = ?`, mediaID); err != nil {
		return err
	}
	return nil
}

func removeDVRRecordingFile(path, appDataDir string) error {
	clean, err := cleanDVRRecordingFilePath(path, appDataDir)
	if err != nil {
		return err
	}
	if err := os.Remove(clean); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	nfoPath := strings.TrimSuffix(clean, filepath.Ext(clean)) + ".nfo"
	if err := os.Remove(nfoPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, imagePath := range dvrRecordingImageSidecarPaths(clean) {
		if err := os.Remove(imagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func cleanDVRRecordingFilePath(path, appDataDir string) (string, error) {
	clean := filepath.Clean(path)
	root := filepath.Clean(filepath.Join(appDataDir, "recordings"))
	rel, err := filepath.Rel(root, clean)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("recording path escaped app data")
	}
	return clean, nil
}

func (s *Server) claimDVRRecording(id string) bool {
	_, claimed := s.claimDVRRecordingLease(id)
	return claimed
}

func (s *Server) dvrRecordingGuideGenerationCurrent(id string) (bool, error) {
	var valid int
	err := s.queryBackgroundRow(context.Background(), `
		SELECT COUNT(*)
		FROM live_tv_recordings r
		JOIN live_tv_sources source_generation ON source_generation.id = r.source_id AND `+dvrGuideGenerationPredicate("r")+`
		LEFT JOIN live_tv_programs p ON p.id = r.program_id
			AND p.source_id = r.source_id
			AND p.import_generation = source_generation.active_import_generation
		LEFT JOIN live_tv_channels c ON c.id = r.channel_id
			AND c.source_id = r.source_id
			AND c.enabled = 1
			AND c.import_generation = source_generation.active_import_generation
		WHERE r.id = ?
		  AND (COALESCE(r.program_id, '') = '' OR p.id IS NOT NULL)
		  AND (COALESCE(r.channel_id, '') = '' OR c.id IS NOT NULL)`, strings.TrimSpace(id)).Scan(&valid)
	if err != nil {
		return false, err
	}
	return valid == 1, nil
}

func (s *Server) failDVRRecordingForGuideGeneration(id string, expectedRevision int64) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.execBackgroundWrite(context.Background(), `
		UPDATE live_tv_recordings
		SET status = 'failed', error = 'The Live TV guide changed before this recording started.',
			failure_code = 'guide_generation_changed', revision = revision + 1, updated_at = ?
		WHERE id = ? AND status = 'scheduled' AND revision = ? AND `+dvrGuideGenerationPredicate("live_tv_recordings"), now, id, expectedRevision)
}

func (s *Server) claimDVRRecordingLease(id string) (liveTVTunerLease, bool) {
	s.dvrAllocationMu.Lock()
	defer s.dvrAllocationMu.Unlock()
	recording, err := s.getDVRRecording(id)
	if err != nil || recording.Status != "scheduled" {
		return liveTVTunerLease{}, false
	}
	generationCurrent, generationErr := s.dvrRecordingGuideGenerationCurrent(id)
	if generationErr != nil {
		return liveTVTunerLease{}, false
	}
	if !generationCurrent {
		s.failDVRRecordingForGuideGeneration(id, recording.Revision)
		return liveTVTunerLease{}, false
	}
	user, authErr := s.currentDVRUser(recording.UserID, recording.ProfileID)
	if authErr == nil {
		_, authErr = s.resolveAuthorizedDVRLiveTVReference(context.Background(), user, recording.SourceID, recording.ChannelID, recording.ProgramID, false, true)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if authErr != nil {
		_, _ = s.execBackgroundWrite(context.Background(), `
			UPDATE live_tv_recordings
			SET status = 'failed', error = 'Recording authorization changed before its start time.', failure_code = 'authorization_revoked', revision = revision + 1, updated_at = ?
			WHERE id = ? AND status = 'scheduled' AND revision = ? AND `+dvrGuideGenerationPredicate("live_tv_recordings"), now, id, recording.Revision)
		return liveTVTunerLease{}, false
	}
	lease, err := s.reserveLiveTVTunerAllocation(context.Background(), recording.SourceID, recording.ChannelID, "dvr_recording", id)
	if err != nil {
		if !errors.Is(err, errLiveTVTunerCapacity) {
			s.log.Warn("dvr tuner allocation failed", "error", err, "recording", id)
			return liveTVTunerLease{}, false
		}
		_, err := s.execBackgroundWrite(context.Background(), `
			UPDATE live_tv_recordings
			SET status = 'failed', error = 'No tuner allocation was available when the recording started.', failure_code = 'tuner_conflict', revision = revision + 1, updated_at = ?
			WHERE id = ? AND status = 'scheduled' AND revision = ? AND `+dvrGuideGenerationPredicate("live_tv_recordings"), now, id, recording.Revision)
		if err != nil {
			s.log.Warn("dvr recording conflict finalization failed", "error", err, "recording", id)
		}
		return liveTVTunerLease{}, false
	}
	if !lease.Created {
		// Another process already owns this recording lease. Never inherit or
		// release its allocation merely because the row has not transitioned yet.
		return liveTVTunerLease{}, false
	}
	generationCurrent, generationErr = s.dvrRecordingGuideGenerationCurrent(id)
	if generationErr != nil || !generationCurrent {
		s.releaseLiveTVTunerAllocationLease(context.Background(), "dvr_recording", id, lease.Token)
		if generationErr == nil {
			s.failDVRRecordingForGuideGeneration(id, recording.Revision)
		}
		return liveTVTunerLease{}, false
	}
	// Close the reservation/authentication gap. A revocation that lands while
	// capacity is being acquired prevents the recording from entering running.
	user, authErr = s.currentDVRUser(recording.UserID, recording.ProfileID)
	if authErr == nil {
		_, authErr = s.resolveAuthorizedDVRLiveTVReference(context.Background(), user, recording.SourceID, recording.ChannelID, recording.ProgramID, false, true)
	}
	if authErr != nil {
		s.releaseLiveTVTunerAllocationLease(context.Background(), "dvr_recording", id, lease.Token)
		_, _ = s.execBackgroundWrite(context.Background(), `
			UPDATE live_tv_recordings SET status = 'failed', error = 'Recording authorization changed before its start time.',
				failure_code = 'authorization_revoked', revision = revision + 1, updated_at = ?
			WHERE id = ? AND status = 'scheduled' AND revision = ? AND `+dvrGuideGenerationPredicate("live_tv_recordings"), now, id, recording.Revision)
		return liveTVTunerLease{}, false
	}
	result, err := s.execBackgroundWrite(context.Background(), `UPDATE live_tv_recordings SET status = 'running', failure_code = '', revision = revision + 1, updated_at = ? WHERE id = ? AND status = 'scheduled' AND revision = ? AND `+dvrGuideGenerationPredicate("live_tv_recordings"), now, id, recording.Revision)
	if err != nil {
		s.releaseLiveTVTunerAllocationLease(context.Background(), "dvr_recording", id, lease.Token)
		s.log.Warn("dvr recording claim failed", "error", err, "recording", id)
		return liveTVTunerLease{}, false
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		s.releaseLiveTVTunerAllocationLease(context.Background(), "dvr_recording", id, lease.Token)
	}
	return lease, affected == 1
}

func (s *Server) runDVRRecording(parentCtx context.Context, id, leaseToken string) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	recording, streamURL, err := s.loadDVRRecordingForRunContext(parentCtx, id)
	if err != nil {
		s.failDVRRecordingLease(id, leaseToken, "", err)
		return
	}
	defer s.releaseLiveTVTunerAllocationLease(parentCtx, "dvr_recording", id, leaseToken)
	runCtx, cancelRun := context.WithCancel(parentCtx)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if !s.heartbeatLiveTVTunerAllocationLease(runCtx, "dvr_recording", id, leaseToken) {
					cancelRun()
					return
				}
			}
		}
	}()
	defer func() {
		cancelRun()
		<-heartbeatDone
	}()
	if _, err := exec.LookPath(s.cfg.FFmpegPath); err != nil && filepath.Base(s.cfg.FFmpegPath) == s.cfg.FFmpegPath {
		s.failDVRRecordingLease(id, leaseToken, "", errors.New("FFmpeg is not available on PATH"))
		return
	}
	start, err := time.Parse(time.RFC3339, recording.StartsAt)
	if err != nil {
		s.failDVRRecordingLease(id, leaseToken, "", errors.New("recording start time is invalid"))
		return
	}
	end, err := time.Parse(time.RFC3339, recording.EndsAt)
	if err != nil || !end.After(time.Now().UTC()) {
		s.failDVRRecordingLease(id, leaseToken, "", errors.New("recording end time is invalid or already elapsed"))
		return
	}
	duration := time.Until(end)
	if duration < dvrMinimumRecordingDuration {
		s.failDVRRecordingLease(id, leaseToken, "", errors.New("recording window elapsed before enough media could be captured"))
		return
	}
	outputPath, err := s.dvrOutputPath(recording, start)
	if err != nil {
		s.failDVRRecordingLease(id, leaseToken, "", err)
		return
	}
	if err := ensureMediaWriteCapacity(filepath.Dir(outputPath), mediaWriteMinimumFreeBytes); err != nil {
		s.failDVRRecordingLease(id, leaseToken, "", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		s.failDVRRecordingLease(id, leaseToken, "", err)
		return
	}
	if err := validateDVRRecordingInputURL(streamURL); err != nil {
		s.failDVRRecordingLease(id, leaseToken, "", err)
		return
	}
	inputTransport, err := startDVRInputTransport(runCtx, id, leaseToken, streamURL, nil)
	if err != nil {
		s.failDVRRecordingLease(id, leaseToken, "", err)
		return
	}
	defer inputTransport.Close()
	defaults := s.dvrTimerDefaults()
	resourceRequest := mediaResourceRequest{class: foundationcontract.WorkClassProtectedCapture, disk: 2, network: 1}
	if defaults.ConvertRecordings {
		resourceRequest.cpu = 1
	}
	release, err := s.mediaResourceGovernor().acquireContext(runCtx, resourceRequest)
	if err != nil {
		s.failDVRRecordingLease(id, leaseToken, "", err)
		return
	}
	defer release()
	diskPressure := make(chan error, 1)
	diskPressureDone := make(chan struct{})
	go func() {
		defer close(diskPressureDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if err := ensureMediaWriteCapacity(filepath.Dir(outputPath), mediaWriteMinimumFreeBytes); err != nil {
					select {
					case diskPressure <- err:
					default:
					}
					cancelRun()
					return
				}
			}
		}
	}()
	defer func() {
		cancelRun()
		<-diskPressureDone
	}()
	workingPath := dvrLeaseWorkingPath(outputPath, leaseToken)
	completed := false
	defer func() {
		if !completed {
			_ = os.Remove(workingPath)
		}
	}()
	now := time.Now().UTC().Format(time.RFC3339)
	result, pathErr := s.execBackgroundWrite(parentCtx, `
		UPDATE live_tv_recordings SET path = ?, error = '', failure_code = '', updated_at = ?
		WHERE id = ? AND status = 'running' AND EXISTS (
			SELECT 1 FROM live_tv_tuner_allocations
			WHERE allocation_kind = 'dvr_recording' AND consumer_id = ? AND lease_token = ?
		)`, workingPath, now, id, id, leaseToken)
	if pathErr != nil {
		s.log.Warn("dvr recording path update failed", "error", pathErr, "recording", id)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return
	}

	ctx, cancel := context.WithTimeout(runCtx, duration+2*time.Minute)
	defer cancel()
	recordingProfile := "copy"
	if defaults.ConvertRecordings {
		recordingProfile = defaults.RecordingProfile
	}
	args := dvrRecordingFFmpegArgs(inputTransport.URL, duration, workingPath, recordingProfile, defaults.PreserveAllStreams)
	cmd := exec.CommandContext(ctx, s.cfg.FFmpegPath, args...)
	s.recordLog("info", "DVR recording started", map[string]string{"recording": id, "title": recording.Title})
	if err := runDVRCommandWithOutputWatchdog(ctx, cmd, workingPath, dvrOutputProgressTimeout); err != nil {
		select {
		case pressureErr := <-diskPressure:
			err = pressureErr
		default:
		}
		s.failDVRRecordingLease(id, leaseToken, workingPath, err)
		return
	}
	select {
	case pressureErr := <-diskPressure:
		s.failDVRRecordingLease(id, leaseToken, workingPath, pressureErr)
		return
	default:
	}
	info, err := os.Stat(workingPath)
	if err != nil {
		s.failDVRRecordingLease(id, leaseToken, workingPath, err)
		return
	}
	if !info.Mode().IsRegular() || info.Size() < dvrMinimumRecordingBytes {
		s.failDVRRecordingLease(id, leaseToken, workingPath, errors.New("recording output did not contain valid media bytes"))
		return
	}
	if !s.heartbeatLiveTVTunerAllocationLease(parentCtx, "dvr_recording", id, leaseToken) {
		// FFmpeg has already exited successfully and the artifact has been
		// statted. Losing the allocation fence here means another owner may now
		// control the row, so do not mark it complete; retain the bytes under the
		// recovery-only name and let the fenced repair pass reconcile the row.
		if incompletePath, ok := dvrIncompletePathFromLeaseWorkingPath(workingPath); ok {
			if renameErr := os.Rename(workingPath, incompletePath); renameErr == nil {
				completed = true
				s.log.Warn("DVR lease was lost after recording completed; retained output for recovery", "recording", id, "path", incompletePath)
				return
			} else {
				// Do not delete a successfully recorded artifact merely because the
				// recovery rename also encountered a transient filesystem failure.
				completed = true
				s.log.Error("DVR lease and recovery rename failed after recording completed", "recording", id, "path", workingPath, "error", renameErr)
				return
			}
		}
		completed = true
		return
	}
	if err := os.Rename(workingPath, outputPath); err != nil {
		s.failDVRRecordingLease(id, leaseToken, workingPath, err)
		return
	}
	now = time.Now().UTC().Format(time.RFC3339)
	result, err = s.execBackgroundWrite(parentCtx, `
		UPDATE live_tv_recordings SET status = 'complete', path = ?, size_bytes = ?, error = '', failure_code = '', revision = revision + 1, updated_at = ?
		WHERE id = ? AND status = 'running' AND EXISTS (
			SELECT 1 FROM live_tv_tuner_allocations
			WHERE allocation_kind = 'dvr_recording' AND consumer_id = ? AND lease_token = ?
		)`, outputPath, info.Size(), now, id, id, leaseToken)
	if err != nil {
		s.log.Warn("dvr recording completion update failed", "error", err, "recording", id)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return
	}
	completed = true
	recording.Path = outputPath
	recording.SizeBytes = info.Size()
	if err := s.importDVRRecordingMedia(recording, now); err != nil {
		s.log.Warn("dvr recording media import failed", "error", err, "recording", id)
	}
	if s.dvrTimerDefaults().SaveNFO {
		if err := s.writeDVRRecordingNFO(recording); err != nil {
			s.log.Warn("dvr recording nfo write failed", "error", err, "recording", id)
		}
	}
	if s.dvrTimerDefaults().SaveImageSidecars {
		if err := s.writeDVRRecordingImageSidecars(recording); err != nil {
			s.log.Warn("dvr recording image sidecar write failed", "error", err, "recording", id)
		}
	}
	s.recordLog("info", "DVR recording completed", map[string]string{"recording": id, "title": recording.Title})
}

func runDVRCommandWithOutputWatchdog(ctx context.Context, cmd *exec.Cmd, outputPath string, progressTimeout time.Duration) error {
	if progressTimeout <= 0 {
		progressTimeout = dvrOutputProgressTimeout
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	tickEvery := progressTimeout / 3
	if tickEvery > 5*time.Second {
		tickEvery = 5 * time.Second
	}
	if tickEvery <= 0 {
		tickEvery = time.Second
	}
	ticker := time.NewTicker(tickEvery)
	defer ticker.Stop()
	lastProgress := time.Now()
	lastSize := int64(-1)
	for {
		select {
		case err := <-wait:
			return err
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			<-wait
			return ctx.Err()
		case now := <-ticker.C:
			if info, err := os.Stat(outputPath); err == nil && info.Size() > lastSize {
				lastSize, lastProgress = info.Size(), now
			}
			if now.Sub(lastProgress) >= progressTimeout {
				_ = cmd.Process.Kill()
				<-wait
				return errDVROutputStalled
			}
		}
	}
}

type dvrRecordingNFO struct {
	XMLName xml.Name `xml:"episodedetails"`
	Title   string   `xml:"title"`
	Plot    string   `xml:"plot"`
	Aired   string   `xml:"aired,omitempty"`
	Studio  string   `xml:"studio"`
	Genre   string   `xml:"genre"`
}

func (s *Server) writeDVRRecordingNFO(recording DVRRecording) error {
	clean, err := cleanDVRRecordingFilePath(recording.Path, s.cfg.AppDataDir)
	if err != nil {
		return err
	}
	start, _ := time.Parse(time.RFC3339, recording.StartsAt)
	aired := ""
	plot := "Recorded from Live TV."
	if !start.IsZero() {
		aired = start.UTC().Format("2006-01-02")
		plot = "Recorded from Live TV on " + start.UTC().Format("2006-01-02 15:04") + " UTC."
	}
	payload, err := xml.MarshalIndent(dvrRecordingNFO{
		Title:  recording.Title,
		Plot:   plot,
		Aired:  aired,
		Studio: "Portico DVR",
		Genre:  "Recorded TV",
	}, "", "  ")
	if err != nil {
		return err
	}
	nfoPath := strings.TrimSuffix(clean, filepath.Ext(clean)) + ".nfo"
	return os.WriteFile(nfoPath, append([]byte(xml.Header), payload...), 0o600)
}

func (s *Server) writeDVRRecordingImageSidecars(recording DVRRecording) error {
	clean, err := cleanDVRRecordingFilePath(recording.Path, s.cfg.AppDataDir)
	if err != nil {
		return err
	}
	start, _ := time.Parse(time.RFC3339, recording.StartsAt)
	subtitle := "Recorded TV"
	if !start.IsZero() {
		subtitle = start.UTC().Format("2006-01-02 15:04 UTC")
	}
	base := strings.TrimSuffix(clean, filepath.Ext(clean))
	if err := os.WriteFile(base+"-poster.svg", dvrRecordingArtworkSVG(recording.Title, subtitle, 600, 900), 0o600); err != nil {
		return err
	}
	return os.WriteFile(base+"-thumb.svg", dvrRecordingArtworkSVG(recording.Title, subtitle, 1280, 720), 0o600)
}

func dvrRecordingImageSidecarPaths(recordingPath string) []string {
	base := strings.TrimSuffix(recordingPath, filepath.Ext(recordingPath))
	return []string{base + "-poster.svg", base + "-thumb.svg"}
}

func dvrRecordingArtworkSVG(title, subtitle string, width, height int) []byte {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Recorded TV"
	}
	subtitle = strings.TrimSpace(subtitle)
	if subtitle == "" {
		subtitle = "Portico DVR"
	}
	titleSize := max(28, width/16)
	subtitleSize := max(18, width/28)
	body := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#111827"/>
      <stop offset="0.55" stop-color="#0f766e"/>
      <stop offset="1" stop-color="#f59e0b"/>
    </linearGradient>
  </defs>
  <rect width="100%%" height="100%%" fill="url(#bg)"/>
  <rect x="%d" y="%d" width="%d" height="%d" rx="18" fill="rgba(0,0,0,0.35)"/>
  <text x="%d" y="%d" fill="#f9fafb" font-family="Inter, Arial, sans-serif" font-size="%d" font-weight="700">%s</text>
  <text x="%d" y="%d" fill="#d1d5db" font-family="Inter, Arial, sans-serif" font-size="%d">%s</text>
</svg>
`, width, height, width, height, width/12, height*2/3, width*5/6, height/5, width/10, height*3/4, titleSize, xmlEscapeText(truncateArtworkText(title, 42)), width/10, height*3/4+subtitleSize+16, subtitleSize, xmlEscapeText(truncateArtworkText(subtitle, 64)))
	return []byte(body)
}

func xmlEscapeText(value string) string {
	var builder strings.Builder
	_ = xml.EscapeText(&builder, []byte(value))
	return builder.String()
}

func truncateArtworkText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:max(0, limit-1)]) + "..."
}

func dvrRecordingFFmpegArgs(streamURL string, duration time.Duration, outputPath string, recordingProfile string, preserveAllStreams bool) []string {
	args := []string{
		"-hide_banner", "-nostdin", "-y",
	}
	if parsed, err := url.Parse(strings.TrimSpace(streamURL)); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "10")
	}
	args = append(args, "-i", streamURL, "-t", strconv.FormatFloat(duration.Seconds(), 'f', 3, 64))
	switch normalizeDVRRecordingProfile(recordingProfile) {
	case "h264-1080p-8m":
		args = appendDVRH264ProfileArgs(args, 1080, "8M", "16M")
	case "h264-720p-4m":
		args = appendDVRH264ProfileArgs(args, 720, "4M", "8M")
	default:
		if preserveAllStreams {
			args = append(args, "-map", "0")
		}
		args = append(args, "-c", "copy")
	}
	args = append(args,
		"-movflags", "+faststart",
		outputPath,
	)
	return args
}

func validateDVRRecordingInputURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("recording source URL must use HTTP or HTTPS")
	}
	return nil
}

func appendDVRH264ProfileArgs(args []string, maxHeight int, videoBitrate string, bufferSize string) []string {
	return append(args,
		"-map", "0:v:0?",
		"-map", "0:a:0?",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-vf", fmt.Sprintf("scale=-2:min(%d\\,ih)", maxHeight),
		"-b:v", videoBitrate,
		"-maxrate", videoBitrate,
		"-bufsize", bufferSize,
		"-c:a", "aac",
		"-b:a", "160k",
	)
}

func (s *Server) importDVRRecordingMedia(recording DVRRecording, importedAt string) error {
	if strings.TrimSpace(recording.Path) == "" {
		return errors.New("recording path is empty")
	}
	if _, err := os.Stat(recording.Path); err != nil {
		return err
	}
	libraryID, err := s.ensureRecordedTVLibrary()
	if err != nil {
		return err
	}
	start, _ := time.Parse(time.RFC3339, recording.StartsAt)
	end, _ := time.Parse(time.RFC3339, recording.EndsAt)
	durationSeconds := 0
	if !start.IsZero() && end.After(start) {
		durationSeconds = int(end.Sub(start).Seconds())
	}
	if strings.TrimSpace(importedAt) == "" {
		importedAt = time.Now().UTC().Format(time.RFC3339)
	}
	mediaID := dvrRecordingMediaID(recording.ID)
	summary := "Recorded from Live TV."
	if !start.IsZero() {
		summary = "Recorded from Live TV on " + start.UTC().Format("2006-01-02 15:04") + " UTC."
	}
	genresJSON := `["Recorded TV"]`
	return s.withBackgroundTxTagged(context.Background(), []string{"dvr", "live-tv"}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`
			INSERT INTO media_items (
				id, library_id, type, title, sort_title, duration_seconds, summary, genres_json, added_at, art_seed, source_url, random_key
			) VALUES (?, ?, 'recording', ?, ?, ?, ?, ?, ?, 'recorded-tv', ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				library_id = excluded.library_id,
				random_key = CASE WHEN COALESCE(media_items.random_key, '') = '' THEN excluded.random_key ELSE media_items.random_key END,
				title = excluded.title,
				sort_title = excluded.sort_title,
				duration_seconds = excluded.duration_seconds,
				summary = excluded.summary,
				genres_json = excluded.genres_json,
				source_url = excluded.source_url`,
			mediaID, libraryID, recording.Title, sortableTitle(recording.Title), durationSeconds, summary, genresJSON, importedAt, recording.Path, mediaRandomKey(mediaID)); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO dvr_recording_media (recording_id, media_id, created_at)
			VALUES (?, ?, ?)
			ON CONFLICT(recording_id) DO UPDATE SET media_id = excluded.media_id, created_at = excluded.created_at`,
			recording.ID, mediaID, importedAt); err != nil {
			return err
		}
		existingStreams := map[string]string{}
		rows, err := tx.Query(`SELECT kind, id FROM media_streams WHERE media_id = ? AND source_kind = 'dvr'`, mediaID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var kind, id string
			if err := rows.Scan(&kind, &id); err != nil {
				_ = rows.Close()
				return err
			}
			existingStreams[kind] = id
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM media_streams WHERE media_id = ? AND source_kind = 'dvr'`, mediaID); err != nil {
			return err
		}
		if strings.TrimSpace(recording.UserID) != "" {
			if _, err := tx.Exec(`
				INSERT INTO user_library_access (user_id, library_id, created_at)
				VALUES (?, ?, ?)
				ON CONFLICT(user_id, library_id) DO NOTHING`, recording.UserID, libraryID, importedAt); err != nil {
				return err
			}
		}
		videoStreamID := existingStreams["video"]
		if videoStreamID == "" {
			videoStreamID, err = stableOpaquePublicResourceIDTx(tx, "media-stream", "dvr\x00"+mediaID+"\x00video")
			if err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`INSERT INTO media_streams (id, media_id, source_kind, source_identity, storage_key, stream_index, kind, codec, display_title) VALUES (?, ?, 'dvr', ?, ?, -1, 'video', 'h264', 'DVR video')`,
			videoStreamID, mediaID, mediaID+"\x1fvideo", videoStreamID); err != nil {
			return err
		}
		audioStreamID := existingStreams["audio"]
		if audioStreamID == "" {
			audioStreamID, err = stableOpaquePublicResourceIDTx(tx, "media-stream", "dvr\x00"+mediaID+"\x00audio")
			if err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`INSERT INTO media_streams (id, media_id, source_kind, source_identity, storage_key, stream_index, kind, codec, language, channels, display_title) VALUES (?, ?, 'dvr', ?, ?, -1, 'audio', 'aac', 'und', 2, 'DVR audio')`,
			audioStreamID, mediaID, mediaID+"\x1faudio", audioStreamID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM media_search WHERE media_id = ?`, mediaID); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO media_search (media_id, title, summary, genres) VALUES (?, ?, ?, ?)`,
			mediaID, recording.Title, summary, "Recorded TV DVR")
		return err
	})
}

func (s *Server) ensureRecordedTVLibrary() (string, error) {
	const libraryID = "lib_recorded_tv"
	now := time.Now().UTC().Format(time.RFC3339)
	path := filepath.Join(s.cfg.AppDataDir, "recordings")
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	err := s.withBackgroundTxTagged(context.Background(), []string{"dvr", "live-tv"}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`
			INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
			VALUES (?, 'Recorded TV', 'recorded-tv', 60, ?, '{}', ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				type = excluded.type,
				path = excluded.path`,
			libraryID, path, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO library_paths (id, library_id, path, sort_order, created_at)
			VALUES (?, ?, ?, 0, ?)
			ON CONFLICT(id) DO UPDATE SET path = excluded.path`,
			"lp_recorded_tv", libraryID, path, now); err != nil {
			return err
		}
		_, err := tx.Exec(`
			INSERT INTO user_library_access (user_id, library_id, created_at)
			SELECT id, ?, ?
			FROM users
			WHERE role = 'owner'
			ON CONFLICT(user_id, library_id) DO NOTHING`,
			libraryID, now)
		return err
	})
	return libraryID, err
}

func dvrRecordingMediaID(recordingID string) string {
	return "dvr_" + safePathComponent(recordingID)
}

func (s *Server) loadDVRRecordingForRun(id string) (DVRRecording, string, error) {
	return s.loadDVRRecordingForRunContext(context.Background(), id)
}

func (s *Server) loadDVRRecordingForRunContext(ctx context.Context, id string) (DVRRecording, string, error) {
	var recording DVRRecording
	var streamURL string
	err := s.queryBackgroundRow(ctx, `
		SELECT r.id, COALESCE(r.rule_id, ''), r.user_id, r.profile_id, r.source_id, COALESCE(r.channel_id, ''), r.program_id,
			r.title, COALESCE(r.folder, ''), r.status, r.starts_at, r.ends_at, r.path, r.size_bytes, r.error, r.created_at, r.updated_at,
			COALESCE(c.stream_url, '')
		FROM live_tv_recordings r
		JOIN live_tv_sources source_generation ON source_generation.id = r.source_id AND `+dvrGuideGenerationPredicate("r")+`
		LEFT JOIN live_tv_programs p ON p.id = r.program_id
			AND p.source_id = r.source_id
			AND p.import_generation = source_generation.active_import_generation
		LEFT JOIN live_tv_channels c ON c.id = r.channel_id
			AND c.source_id = r.source_id
			AND c.enabled = 1
			AND c.import_generation = source_generation.active_import_generation
		WHERE r.id = ?
		  AND (COALESCE(r.program_id, '') = '' OR p.id IS NOT NULL)`, id).
		Scan(&recording.ID, &recording.RuleID, &recording.UserID, &recording.ProfileID, &recording.SourceID, &recording.ChannelID, &recording.ProgramID,
			&recording.Title, &recording.Folder, &recording.Status, &recording.StartsAt, &recording.EndsAt, &recording.Path, &recording.SizeBytes,
			&recording.Error, &recording.CreatedAt, &recording.UpdatedAt, &streamURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DVRRecording{}, "", errors.New("recording was not found")
		}
		return DVRRecording{}, "", err
	}
	if strings.TrimSpace(streamURL) == "" {
		return DVRRecording{}, "", errors.New("recording channel does not have a stream URL")
	}
	return recording, streamURL, nil
}

func (s *Server) dvrOutputPath(recording DVRRecording, start time.Time) (string, error) {
	parts := []string{s.cfg.AppDataDir, "recordings"}
	relativeParts := dvrRecordingPathParts(recording, start, s.dvrTimerDefaults().RecordingPathTemplate)
	parts = append(parts, relativeParts[:len(relativeParts)-1]...)
	dir := filepath.Join(parts...)
	filename := relativeParts[len(relativeParts)-1] + "-" + safePathComponent(recording.ID) + ".mp4"
	path := filepath.Join(dir, filename)
	clean := filepath.Clean(path)
	root := filepath.Clean(filepath.Join(s.cfg.AppDataDir, "recordings"))
	if !strings.HasPrefix(clean, root+string(filepath.Separator)) {
		return "", errors.New("recording output path escaped app data")
	}
	return clean, nil
}

func dvrRecordingPathParts(recording DVRRecording, start time.Time, template string) []string {
	template = normalizeDVRRecordingPathTemplate(template)
	replacements := map[string]string{
		"{folder}":  recording.Folder,
		"{title}":   recording.Title,
		"{id}":      recording.ID,
		"{channel}": recording.ChannelID,
		"{year}":    start.UTC().Format("2006"),
		"{month}":   start.UTC().Format("01"),
		"{day}":     start.UTC().Format("02"),
		"{start}":   start.UTC().Format("20060102-150405"),
	}
	rawParts := strings.Split(strings.ReplaceAll(template, "\\", "/"), "/")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		for token, value := range replacements {
			part = strings.ReplaceAll(part, token, value)
		}
		clean := safePathComponent(part)
		if clean != "" && clean != "item" {
			parts = append(parts, clean)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, safePathComponent(recording.ID))
	}
	last := parts[len(parts)-1]
	if last == "" || last == "item" {
		parts[len(parts)-1] = safePathComponent(recording.ID)
	}
	return parts
}

func dvrLeaseWorkingPath(outputPath, leaseToken string) string {
	ext := filepath.Ext(outputPath)
	base := strings.TrimSuffix(outputPath, ext)
	return base + ".lease-" + safePathComponent(leaseToken) + ".partial" + ext
}

func dvrFinalPathFromLeaseWorkingPath(workingPath string) (string, bool) {
	ext := filepath.Ext(workingPath)
	base := strings.TrimSuffix(workingPath, ext)
	leaseAt := strings.LastIndex(base, ".lease-")
	if leaseAt <= 0 || !strings.HasSuffix(base, ".partial") {
		return "", false
	}
	return base[:leaseAt] + ext, true
}

func dvrIncompletePathFromLeaseWorkingPath(workingPath string) (string, bool) {
	finalPath, ok := dvrFinalPathFromLeaseWorkingPath(workingPath)
	if !ok {
		return "", false
	}
	ext := filepath.Ext(finalPath)
	return strings.TrimSuffix(finalPath, ext) + ".incomplete" + ext, true
}

func (s *Server) failDVRRecordingLease(id, leaseToken, workingPath string, err error) {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "recording failed"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	retainedPath := ""
	retainedSize := int64(0)
	if incompletePath, ok := dvrIncompletePathFromLeaseWorkingPath(workingPath); ok {
		// MPEG-TS packets are 188 bytes. Requiring more than one packet avoids
		// publishing empty/container-header-only failures while retaining useful
		// late-stall output for probing and playback diagnostics.
		if info, statErr := os.Stat(workingPath); statErr == nil && info.Mode().IsRegular() && info.Size() >= 376 && s.probeDVRPartialMedia(workingPath) {
			if renameErr := os.Rename(workingPath, incompletePath); renameErr == nil {
				retainedPath, retainedSize = incompletePath, info.Size()
			}
		}
	}
	status := "failed"
	if retainedPath != "" {
		status = "incomplete"
	}
	result, updateErr := s.execBackgroundWrite(context.Background(), `
		UPDATE live_tv_recordings SET status = ?, path = ?, size_bytes = ?, error = ?, failure_code = ?, revision = revision + 1, updated_at = ?
		WHERE id = ? AND status = 'running' AND EXISTS (
			SELECT 1 FROM live_tv_tuner_allocations
			WHERE allocation_kind = 'dvr_recording' AND consumer_id = ? AND lease_token = ?
		)`, status, retainedPath, retainedSize, message, dvrFailureCode(err), now, id, id, leaseToken)
	if updateErr != nil {
		return
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return
	}
	if retainedPath == "" && strings.TrimSpace(workingPath) != "" {
		_ = os.Remove(workingPath)
	}
	if retainedPath != "" {
		if recording, loadErr := s.getDVRRecording(id); loadErr == nil {
			_ = s.importDVRRecordingMedia(recording, now)
		}
	}
	s.releaseLiveTVTunerAllocationLease(context.Background(), "dvr_recording", id, leaseToken)
	s.recordLog("warn", "DVR recording failed", map[string]string{"recording": id, "error": message})
}

func (s *Server) probeDVRPartialMedia(path string) bool {
	probePath := strings.TrimSpace(s.cfg.FFprobePath)
	if probePath == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, probePath, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	return err == nil && duration > 0
}

func dvrFailureCode(err error) string {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "ffmpeg") || strings.Contains(message, "not available on path"):
		return "dependency_unavailable"
	case strings.Contains(message, "start time") || strings.Contains(message, "end time"):
		return "invalid_schedule"
	case strings.Contains(message, "space") || strings.Contains(message, "mkdir") || strings.Contains(message, "path") || strings.Contains(message, "permission"):
		return "storage_unavailable"
	case strings.Contains(message, "stream") || strings.Contains(message, "source") || strings.Contains(message, "http"):
		return "source_unavailable"
	default:
		return "recording_failed"
	}
}
