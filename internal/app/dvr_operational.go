package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	dvrStorageFullThresholdBytes     = int64(512 << 20)
	dvrStoragePressureThresholdBytes = int64(10 << 30)
)

var errDVRRevisionConflict = errors.New("DVR resource revision conflict")

type dvrOperationalSource struct {
	ID                   string
	Name                 string
	Enabled              bool
	ChannelCount         int
	ProgramCount         int
	RefreshIntervalHours int
	LastRefreshedAt      string
	LastError            string
	TunerCount           int
}

type dvrScheduleConflictError struct {
	Conflict          DVRRecording
	RequestedStartsAt string
	RequestedEndsAt   string
	Capacity          int
	Demand            int
}

func (e *dvrScheduleConflictError) Error() string {
	if e == nil {
		return "Recording conflicts with another scheduled recording."
	}
	return fmt.Sprintf("Recording conflicts with %q from %s to %s.", e.Conflict.Title, e.Conflict.StartsAt, e.Conflict.EndsAt)
}

func newDVRScheduleConflictError(conflict DVRRecording, start time.Time, end time.Time) error {
	return &dvrScheduleConflictError{
		Conflict:          conflict,
		RequestedStartsAt: start.UTC().Format(time.RFC3339),
		RequestedEndsAt:   end.UTC().Format(time.RFC3339),
		Capacity:          max(1, conflict.AllocationCapacity),
		Demand:            max(2, conflict.AllocationDemand),
	}
}

func writeDVRRecordingFailure(w http.ResponseWriter, user User, err error) {
	var conflict *dvrScheduleConflictError
	if !errors.As(err, &conflict) {
		writeError(w, http.StatusBadRequest, "dvr_recording_failed", err.Error())
		return
	}
	reveal := dvrRecordingOwnerProfileID(conflict.Conflict) == viewerProfileID(user)
	detail := fmt.Sprintf("This request would need %d tuners while the Live TV source provides %d.", conflict.Demand, conflict.Capacity)
	conflictingRecording := map[string]any{
		"startsAt": conflict.Conflict.StartsAt,
		"endsAt":   conflict.Conflict.EndsAt,
	}
	if reveal {
		detail = fmt.Sprintf("%s This request would need %d tuners while the Live TV source provides %d.", conflict.Error(), conflict.Demand, conflict.Capacity)
		conflictingRecording["id"] = conflict.Conflict.ID
		conflictingRecording["title"] = conflict.Conflict.Title
		conflictingRecording["status"] = conflict.Conflict.Status
	}
	requestID := strings.TrimSpace(w.Header().Get(requestIDHeader))
	if requestID == "" {
		requestID = randomID("req")
		w.Header().Set(requestIDHeader, requestID)
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	writeJSON(w, http.StatusConflict, map[string]any{
		"type":      "https://portico.media/problems/dvr-schedule-conflict",
		"title":     http.StatusText(http.StatusConflict),
		"status":    http.StatusConflict,
		"code":      "dvr_schedule_conflict",
		"detail":    detail,
		"messageId": "dvr.conflict",
		"details": map[string]any{
			"capacity": conflict.Capacity,
			"demand":   conflict.Demand,
		},
		"requestId": requestID,
		"conflict": map[string]any{
			"reason":               "source_recording_overlap",
			"sourceId":             conflict.Conflict.SourceID,
			"requestedStartsAt":    conflict.RequestedStartsAt,
			"requestedEndsAt":      conflict.RequestedEndsAt,
			"capacity":             conflict.Capacity,
			"demand":               conflict.Demand,
			"conflictingRecording": conflictingRecording,
		},
	})
}

func (s *Server) handleDVROperationalStatus(w http.ResponseWriter, r *http.Request, user User, parts []string) {
	if len(parts) != 0 {
		writeError(w, http.StatusNotFound, "not_found", "DVR status route was not found.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	sourceID := strings.TrimSpace(r.URL.Query().Get("sourceId"))
	if sourceID != "" {
		source, err := s.getLiveTVSourceRecord(sourceID)
		if err != nil {
			writeError(w, http.StatusNotFound, "live_tv_source_not_found", "Live TV source was not found.")
			return
		}
		if !source.Enabled && !canManageLiveTVSources(user) {
			writeError(w, http.StatusNotFound, "live_tv_source_not_found", "Live TV source was not found.")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), navigationRequestTimeout)
	defer cancel()
	conflicts, _, err := s.dvrOperationalConflictsContext(ctx, user, sourceID, false)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "dvr_status_timeout", "DVR status exceeded the foreground request budget. Try again shortly.")
			return
		}
		writeError(w, http.StatusInternalServerError, "dvr_status_failed", "Unable to load DVR operational status.")
		return
	}
	storage, _, err := s.dvrOperationalStorageContext(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "dvr_status_failed", "Unable to load DVR operational status.")
		return
	}
	canOwnRules := canScheduleDVR(user)
	canManageAllRules := false
	writeJSON(w, http.StatusOK, DVRConsumerStatus{
		Capabilities: DVROperationalCapabilities{
			CanScheduleRecordings: canOwnRules, CanManageRecordingRules: canManageAllRules,
			CanCreateOwnRules: canOwnRules, CanEditOwnRules: canOwnRules, CanDeleteOwnRules: canOwnRules,
			CanManageAllRules: canManageAllRules,
			Actions: func() []string {
				if canOwnRules {
					return []string{liveTVActionDVRRuleCreate}
				}
				return []string{}
			}(),
		},
		Conflicts: conflicts, Storage: storage, GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAdminDVROperationalStatus(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeProductError(w, http.StatusForbidden, "forbidden", "Only the server owner can view DVR operations.")
		return
	}
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), navigationRequestTimeout)
	defer cancel()
	status, err := s.dvrOperationalStatusContext(ctx, user, strings.TrimSpace(r.URL.Query().Get("sourceId")))
	if err != nil {
		writeProductError(w, http.StatusInternalServerError, "dvr_status_failed", "Unable to load DVR operational status.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) dvrOperationalStatusContext(ctx context.Context, user User, sourceID string) (DVROperationalStatus, error) {
	now := time.Now().UTC()
	sources, err := s.dvrOperationalSourcesContext(ctx, sourceID, canManageLiveTVSources(user))
	if err != nil {
		return DVROperationalStatus{}, err
	}
	conflicts, conflictSourceIDs, err := s.dvrOperationalConflictsContext(ctx, user, sourceID, true)
	if err != nil {
		return DVROperationalStatus{}, err
	}
	tuners, err := s.dvrOperationalTunersContext(ctx, user, sources, conflictSourceIDs, now)
	if err != nil {
		return DVROperationalStatus{}, err
	}
	storage, storageWritable, err := s.dvrOperationalStorageContext(ctx)
	if err != nil {
		return DVROperationalStatus{}, err
	}
	guide := dvrGuideStatus(sources, now)
	configured := len(sources) > 0
	usableSource := false
	for _, tuner := range tuners {
		if tuner.State != "offline" {
			usableSource = true
			break
		}
	}
	ffmpegPath := firstNonEmpty(strings.TrimSpace(s.cfg.FFmpegPath), "ffmpeg")
	_, ffmpegErr := exec.LookPath(ffmpegPath)
	return DVROperationalStatus{
		Configured: configured,
		Available:  configured && usableSource && storageWritable && ffmpegErr == nil,
		Capabilities: DVROperationalCapabilities{
			CanScheduleRecordings:   canScheduleDVR(user),
			CanManageRecordingRules: canManageDVR(user), CanCreateOwnRules: canScheduleDVR(user),
			CanEditOwnRules: canScheduleDVR(user), CanDeleteOwnRules: canScheduleDVR(user), CanManageAllRules: canManageDVR(user),
			Actions: func() []string {
				if canScheduleDVR(user) {
					return []string{liveTVActionDVRRuleCreate}
				}
				return []string{}
			}(),
		},
		Guide:       guide,
		Conflicts:   conflicts,
		Tuners:      tuners,
		Storage:     storage,
		GeneratedAt: now.Format(time.RFC3339),
	}, nil
}

func (s *Server) dvrOperationalSourcesContext(ctx context.Context, sourceID string, includeDisabled bool) ([]dvrOperationalSource, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if sourceID != "" {
		where = append(where, "s.id = ?")
		args = append(args, sourceID)
	}
	if !includeDisabled {
		where = append(where, "s.enabled = 1")
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT s.id, s.name, s.enabled,
			(SELECT COUNT(*) FROM live_tv_channels c WHERE c.source_id = s.id AND c.enabled = 1),
			(SELECT COUNT(*) FROM live_tv_programs p WHERE p.source_id = s.id),
			s.refresh_interval_hours, s.last_refreshed_at, s.last_error, MAX(1, COALESCE(s.tuner_count, 1))
		FROM live_tv_sources s
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY s.sort_order ASC, LOWER(s.name) ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []dvrOperationalSource{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var source dvrOperationalSource
		var enabled int
		if err := rows.Scan(&source.ID, &source.Name, &enabled, &source.ChannelCount, &source.ProgramCount, &source.RefreshIntervalHours, &source.LastRefreshedAt, &source.LastError, &source.TunerCount); err != nil {
			return nil, err
		}
		source.Enabled = enabled == 1
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func dvrGuideStatus(sources []dvrOperationalSource, now time.Time) DVRGuideOperationalStatus {
	if len(sources) == 0 {
		return DVRGuideOperationalStatus{State: "missing", MessageID: "dvr.guide-missing"}
	}
	type candidate struct {
		priority int
		status   DVRGuideOperationalStatus
	}
	worst := candidate{priority: -1}
	for _, source := range sources {
		status := DVRGuideOperationalStatus{State: "current", LastRefreshedAt: source.LastRefreshedAt}
		priority := 0
		switch {
		case !source.Enabled || (source.ChannelCount == 0 && strings.TrimSpace(source.LastError) != ""):
			priority = 3
			status.State = "source-offline"
			status.MessageID = "dvr.guide-source-offline"
		case source.ProgramCount == 0:
			priority = 2
			status.State = "missing"
			status.MessageID = "dvr.guide-missing"
		default:
			refreshedAt, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(source.LastRefreshedAt))
			refreshHours := source.RefreshIntervalHours
			if refreshHours <= 0 {
				refreshHours = 12
			}
			staleAfter := time.Duration(max(2, refreshHours*2)) * time.Hour
			if strings.TrimSpace(source.LastError) != "" || parseErr != nil || now.Sub(refreshedAt) > staleAfter {
				priority = 1
				status.State = "stale"
				status.MessageID = "dvr.guide-stale"
			}
		}
		if priority > worst.priority {
			worst = candidate{priority: priority, status: status}
		}
	}
	return worst.status
}

func (s *Server) dvrOperationalConflictsContext(ctx context.Context, user User, sourceID string, includeAllProfiles bool) ([]DVRConflict, map[string]bool, error) {
	where := "WHERE r.status IN ('scheduled', 'running')"
	args := []any{}
	if sourceID != "" {
		where += " AND r.source_id = ?"
		args = append(args, sourceID)
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT r.id, r.profile_id, r.source_id, r.starts_at, r.ends_at,
			MAX(1, COALESCE(s.tuner_count, 1)), COALESCE(s.name, '')
		FROM live_tv_recordings r
		JOIN live_tv_sources s ON s.id = r.source_id
		`+where+`
		ORDER BY r.source_id ASC, r.starts_at ASC, r.id ASC
		LIMIT 2000`, args...)
	if err != nil {
		return nil, nil, err
	}
	type allocation struct {
		id, profileID, sourceID, sourceName string
		start, end                          time.Time
		capacity                            int
		kind                                string
	}
	bySource := map[string][]allocation{}
	for rows.Next() {
		var next allocation
		var startsAt, endsAt string
		if err := rows.Scan(&next.id, &next.profileID, &next.sourceID, &startsAt, &endsAt, &next.capacity, &next.sourceName); err != nil {
			_ = rows.Close()
			return nil, nil, err
		}
		next.kind = "recording"
		next.start, _ = time.Parse(time.RFC3339, startsAt)
		next.end, _ = time.Parse(time.RFC3339, endsAt)
		if next.end.After(next.start) {
			bySource[next.sourceID] = append(bySource[next.sourceID], next)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	liveWhere := "WHERE a.allocation_kind = 'live_session' AND a.heartbeat_at >= ?"
	liveArgs := []any{now.Add(-liveTVAllocationStaleAfter).Format(time.RFC3339)}
	if sourceID != "" {
		liveWhere += " AND a.source_id = ?"
		liveArgs = append(liveArgs, sourceID)
	}
	liveRows, err := s.queryUserRead(ctx, `
		SELECT a.consumer_id, COALESCE(ps.profile_id, ''), a.source_id,
			MAX(1, COALESCE(s.tuner_count, 1)), COALESCE(s.name, '')
		FROM live_tv_tuner_allocations a
		JOIN live_tv_sources s ON s.id = a.source_id
		LEFT JOIN playback_sessions ps ON ps.id = a.consumer_id
		`+liveWhere, liveArgs...)
	if err != nil {
		return nil, nil, err
	}
	for liveRows.Next() {
		var next allocation
		if err := liveRows.Scan(&next.id, &next.profileID, &next.sourceID, &next.capacity, &next.sourceName); err != nil {
			_ = liveRows.Close()
			return nil, nil, err
		}
		next.kind, next.start, next.end = "live", now, now.Add(liveTVAllocationStaleAfter)
		bySource[next.sourceID] = append(bySource[next.sourceID], next)
	}
	if err := liveRows.Close(); err != nil {
		return nil, nil, err
	}
	conflicts := []DVRConflict{}
	conflictSourceIDs := map[string]bool{}
	profileID := viewerProfileID(user)
	for conflictSourceID, allocations := range bySource {
		points := []time.Time{}
		for _, allocation := range allocations {
			points = append(points, allocation.start, allocation.end)
		}
		sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })
		for index := 0; index+1 < len(points) && len(conflicts) < 100; index++ {
			start, end := points[index], points[index+1]
			if !end.After(start) {
				continue
			}
			active := []allocation{}
			capacity := allocations[0].capacity
			for _, allocation := range allocations {
				if allocation.start.Before(end) && allocation.end.After(start) {
					active = append(active, allocation)
				}
			}
			if len(active) <= capacity {
				continue
			}
			visibleIDs := []string{}
			for _, allocation := range active {
				if allocation.kind == "recording" && (includeAllProfiles || allocation.profileID == profileID) {
					visibleIDs = append(visibleIDs, allocation.id)
				}
			}
			if len(visibleIDs) == 0 {
				continue
			}
			sort.Strings(visibleIDs)
			idMaterial := strings.Join(visibleIDs, "_") + "_" + start.Format(time.RFC3339)
			next := DVRConflict{
				ID: "conflict_" + safePathComponent(idMaterial), RecordingIDs: visibleIDs,
				StartsAt: start.Format(time.RFC3339), EndsAt: end.Format(time.RFC3339),
				Reason:   fmt.Sprintf("%d simultaneous tuner demands exceed %s capacity of %d tuner%s.", len(active), firstNonEmpty(active[0].sourceName, "this source's"), capacity, pluralSuffix(capacity)),
				Capacity: capacity, Demand: len(active), MessageID: "dvr.conflict",
				Actions: []string{},
			}
			if len(conflicts) > 0 {
				previous := &conflicts[len(conflicts)-1]
				if previous.EndsAt == next.StartsAt && strings.Join(previous.RecordingIDs, "\x00") == strings.Join(next.RecordingIDs, "\x00") && previous.Reason == next.Reason {
					previous.EndsAt = next.EndsAt
					continue
				}
			}
			conflicts = append(conflicts, next)
			conflictSourceIDs[conflictSourceID] = true
		}
	}
	return conflicts, conflictSourceIDs, nil
}

func (s *Server) dvrOperationalTunersContext(ctx context.Context, user User, sources []dvrOperationalSource, conflictSourceIDs map[string]bool, now time.Time) ([]DVRTunerAllocation, error) {
	tuners := make([]DVRTunerAllocation, 0, len(sources))
	bySource := map[string][]int{}
	for _, source := range sources {
		for tunerIndex := 0; tunerIndex < max(1, source.TunerCount); tunerIndex++ {
			state := "idle"
			if !source.Enabled || source.ChannelCount == 0 {
				state = "offline"
			}
			if conflictSourceIDs[source.ID] && tunerIndex == max(1, source.TunerCount)-1 {
				state = "conflict"
			}
			index := len(tuners)
			bySource[source.ID] = append(bySource[source.ID], index)
			name := source.Name
			if source.TunerCount > 1 {
				name = fmt.Sprintf("%s · Tuner %d", source.Name, tunerIndex+1)
			}
			tuners = append(tuners, DVRTunerAllocation{ID: fmt.Sprintf("%s-tuner-%d", source.ID, tunerIndex+1), Name: name, State: state})
		}
	}
	if len(tuners) == 0 {
		return tuners, nil
	}
	sourceIDs := make([]string, 0, len(sources))
	for _, source := range sources {
		sourceIDs = append(sourceIDs, source.ID)
	}
	args := make([]any, 0, len(sourceIDs)+1)
	for _, id := range sourceIDs {
		args = append(args, id)
	}
	args = append(args, now.Add(-liveTVAllocationStaleAfter).Format(time.RFC3339))
	rows, err := s.queryUserRead(ctx, `
		SELECT a.source_id, a.allocation_kind, a.channel_id, a.consumer_id,
			COALESCE(ps.profile_id, ''), COALESCE(r.profile_id, '')
		FROM live_tv_tuner_allocations a
		LEFT JOIN playback_sessions ps ON ps.id = a.consumer_id AND a.allocation_kind = 'live_session'
		LEFT JOIN live_tv_recordings r ON r.id = a.consumer_id AND a.allocation_kind = 'dvr_recording'
		WHERE a.source_id IN (`+sqlPlaceholders(len(sourceIDs))+`) AND a.heartbeat_at >= ?
		ORDER BY a.acquired_at ASC, a.id ASC`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var activeSourceID, kind, channelID, consumerID, sessionProfileID, recordingProfileID string
		if err := rows.Scan(&activeSourceID, &kind, &channelID, &consumerID, &sessionProfileID, &recordingProfileID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		index, ok := firstAvailableDVRTuner(tuners, bySource[activeSourceID])
		if !ok {
			continue
		}
		if kind == "live_session" {
			tuners[index].State = "live"
			if canManageLiveTVSources(user) || sessionProfileID == viewerProfileID(user) {
				tuners[index].ChannelID = channelID
			}
		} else {
			tuners[index].State = "recording"
			if canManageDVR(user) || recordingProfileID == viewerProfileID(user) {
				tuners[index].RecordingID = consumerID
				tuners[index].ChannelID = channelID
			}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return tuners, nil
}

func firstAvailableDVRTuner(tuners []DVRTunerAllocation, indexes []int) (int, bool) {
	for _, index := range indexes {
		if index >= 0 && index < len(tuners) && tuners[index].State == "idle" {
			return index, true
		}
	}
	return 0, false
}

func (s *Server) dvrOperationalStorageContext(ctx context.Context) (DVRStorageOperationalStatus, bool, error) {
	var usedBytes int64
	if err := s.queryUserRow(ctx, `SELECT COALESCE(SUM(size_bytes), 0) FROM live_tv_recordings WHERE status IN ('running', 'complete', 'completed')`).Scan(&usedBytes); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return DVRStorageOperationalStatus{}, false, err
	}
	root := filepath.Join(s.cfg.AppDataDir, "recordings")
	writable := directoryWritable(root)
	availableBytes, totalBytes, spaceErr := filesystemSpace(root)
	state := "healthy"
	switch {
	case !writable:
		state = "full"
	case spaceErr != nil:
		state = "pressure"
	case availableBytes <= dvrStorageFullThresholdBytes:
		state = "full"
	case availableBytes <= dvrStoragePressureThresholdBytes || (totalBytes > 0 && availableBytes*10 <= totalBytes):
		state = "pressure"
	}
	if availableBytes < 0 {
		availableBytes = 0
	}
	return DVRStorageOperationalStatus{UsedBytes: usedBytes, AvailableBytes: availableBytes, State: state}, writable && spaceErr == nil && state != "full", nil
}

func anyStrings(values []string) []any {
	result := make([]any, len(values))
	for index := range values {
		result[index] = values[index]
	}
	return result
}
