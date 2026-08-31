package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
	"github.com/PorticoMediaServer/portico-server/internal/optimized"
)

const (
	maxDownloadPreparationsPerProfile       = 50
	maxActiveDownloadPreparationsPerAccount = 20
)

var (
	errDownloadPreparationLimit    = errors.New("download preparation limit reached")
	errDownloadPreparationNotReady = errors.New("download preparation is not ready")
	errDownloadPreparationAction   = errors.New("download preparation action is invalid")
	errDownloadBatchEmpty          = errors.New("download batch is empty")
)

type downloadPreparationRecord struct {
	DownloadPreparation
	AccountID             string
	ProfileID             string
	AuthorizationRevision string
	ServerID              string
	MediaVersionID        string
	VersionFingerprint    string
	ArtifactSHA256        string
	CancelledAt           string
	RemovedAt             string
	SizeKind              string
	ArtifactExpiresAt     string
}

// downloadPreparationView deliberately distinguishes a measured artifact size
// from a planning estimate. An empty artifactExpiresAt means the server has no
// scheduled expiry for the exact artifact; it never means the grant is
// permanent (grants carry their own short expiry).
type downloadPreparationView struct {
	DownloadPreparation
	SizeKind          string `json:"sizeKind"`
	ArtifactExpiresAt string `json:"artifactExpiresAt,omitempty"`
}

type downloadPreparationBatchRequest struct {
	MediaIDs       []string `json:"mediaIds,omitempty"`
	ContainerID    string   `json:"containerId,omitempty"`
	QualityProfile string   `json:"qualityProfile,omitempty"`
}

type downloadPreparationBatchFailure struct {
	MediaID   string `json:"mediaId"`
	MessageID string `json:"messageId"`
}

type downloadPreparationBatchResponse struct {
	Items    []downloadPreparationView         `json:"items"`
	Rejected []downloadPreparationBatchFailure `json:"rejected"`
	Total    int                               `json:"total"`
}

type nextEpisodeDownloadRequest struct {
	MediaID        string `json:"mediaId"`
	QualityProfile string `json:"qualityProfile,omitempty"`
}

type downloadPreparationCreateEnvelope struct {
	MediaID          string   `json:"mediaId,omitempty"`
	MediaIDs         []string `json:"mediaIds,omitempty"`
	ContainerID      string   `json:"containerId,omitempty"`
	NextAfterMediaID string   `json:"nextAfterMediaId,omitempty"`
	QualityProfile   string   `json:"qualityProfile,omitempty"`
}

type downloadPreparationGrantRequest struct {
	Delivery string `json:"delivery"`
}

func (s *Server) handleDownloadPreparations(w http.ResponseWriter, r *http.Request, user User) {
	if !user.Permissions["downloadMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to download media.")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/download-preparations"), "/")
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			items, err := s.listDownloadPreparationsContext(r.Context(), user)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "download_preparations_failed", "Unable to load download preparations.")
				return
			}
			writeJSON(w, http.StatusOK, ListResponse[downloadPreparationView]{Items: items, Total: len(items)})
		case http.MethodPost:
			var req downloadPreparationCreateEnvelope
			if !decodeJSON(w, r, &req) {
				return
			}
			if len(req.MediaIDs) > 0 || strings.TrimSpace(req.ContainerID) != "" {
				response, err := s.createDownloadPreparationBatchContext(r.Context(), user, downloadPreparationBatchRequest{MediaIDs: req.MediaIDs, ContainerID: req.ContainerID, QualityProfile: req.QualityProfile})
				if err != nil {
					writeDownloadPreparationError(w, err)
					return
				}
				writeJSON(w, http.StatusCreated, response)
				return
			}
			if strings.TrimSpace(req.NextAfterMediaID) != "" {
				preparation, err := s.createNextEpisodeDownloadPreparationContext(r.Context(), user, nextEpisodeDownloadRequest{MediaID: req.NextAfterMediaID, QualityProfile: req.QualityProfile})
				if err != nil {
					writeDownloadPreparationError(w, err)
					return
				}
				writeJSON(w, http.StatusCreated, preparation)
				return
			}
			preparation, err := s.createDownloadPreparationContext(r.Context(), user, DownloadPreparationCreateRequest{MediaID: req.MediaID, QualityProfile: req.QualityProfile})
			if err != nil {
				writeDownloadPreparationError(w, err)
				return
			}
			s.recordAudit(r, user, "media.download_preparation_created", "media", preparation.MediaID, "info", map[string]string{"profile": preparation.QualityProfile, "preparationId": preparation.ID})
			writeJSON(w, http.StatusCreated, preparation)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
		}
		return
	}
	parts := strings.Split(path, "/")
	preparationID := strings.TrimSpace(parts[0])
	if preparationID == "" {
		writeError(w, http.StatusNotFound, "download_preparation_not_found", "Download preparation was not found.")
		return
	}
	if len(parts) == 2 && parts[1] == "grant" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		var request downloadPreparationGrantRequest
		if !decodeJSON(w, r, &request) {
			return
		}
		if request.Delivery != "browser" && request.Delivery != "native" {
			writeError(w, http.StatusBadRequest, "invalid_download_grant_delivery", "Choose browser or native grant delivery.")
			return
		}
		grant, err := s.issueDownloadPreparationGrantContext(r.Context(), user, preparationID, request.Delivery)
		if err != nil {
			writeDownloadPreparationError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if parsed, parseErr := url.Parse(grant.DownloadURL); parseErr == nil {
			parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(parts) == 4 {
				setMediaDownloadGrantCookie(w, r, parts[2], grant)
			}
		}
		if request.Delivery == "browser" {
			grant.GrantToken = ""
		}
		writeJSON(w, http.StatusCreated, grant)
		return
	}
	if len(parts) != 1 {
		writeError(w, http.StatusNotFound, "download_preparation_not_found", "Download preparation was not found.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		preparation, err := s.downloadPreparationForUserContext(r.Context(), user, preparationID)
		if err != nil {
			writeDownloadPreparationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, preparation)
	case http.MethodPatch:
		var req DownloadPreparationUpdateRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		preparation, err := s.updateDownloadPreparationContext(r.Context(), user, preparationID, req.Action)
		if err != nil {
			writeDownloadPreparationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, preparation)
	case http.MethodDelete:
		preparation, err := s.updateDownloadPreparationContext(r.Context(), user, preparationID, "remove")
		if err != nil {
			writeDownloadPreparationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, preparation)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET, PATCH, or DELETE for this endpoint.")
	}
}

func writeDownloadPreparationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "download_preparation_not_found", "Download preparation was not found.")
	case errors.Is(err, errDownloadPreparationLimit):
		w.Header().Set("Retry-After", "30")
		writeError(w, http.StatusTooManyRequests, "download_preparation_limit", "Too many download preparations are active for this account.")
	case errors.Is(err, errDownloadPreparationNotReady):
		writeError(w, http.StatusConflict, "download_preparation_not_ready", "This download is still being prepared.")
	case errors.Is(err, errDownloadBatchEmpty):
		writeError(w, http.StatusBadRequest, "download_batch_empty", "Choose at least one downloadable item or a season.")
	case errors.Is(err, errDownloadPreparationAction):
		writeError(w, http.StatusBadRequest, "invalid_download_action", "Choose pause, resume, cancel, retry, or remove.")
	case errors.Is(err, errInvalidDownloadGrantProfile):
		writeError(w, http.StatusBadRequest, "invalid_download_profile", "Choose the original source or a listed optimized quality.")
	case errors.Is(err, errDownloadGrantUnavailable), errors.Is(err, errUnsupportedPlaybackSource):
		writeError(w, http.StatusConflict, "download_unavailable", "That media version is not currently available for download.")
	default:
		writeError(w, http.StatusInternalServerError, "download_preparation_failed", "Unable to update this download preparation.")
	}
}

func (s *Server) createDownloadPreparationContext(ctx context.Context, user User, req DownloadPreparationCreateRequest) (downloadPreparationView, error) {
	mediaID := strings.TrimSpace(req.MediaID)
	if mediaID == "" {
		return downloadPreparationView{}, errDownloadBatchEmpty
	}
	profile, err := s.normalizeMediaDownloadGrantProfile(req.QualityProfile)
	if err != nil {
		return downloadPreparationView{}, err
	}
	item, err := s.getMediaDownloadSeedForUser(ctx, user, mediaID)
	if err != nil {
		return downloadPreparationView{}, err
	}
	identity, err := s.systemIdentityContext(ctx)
	if err != nil {
		return downloadPreparationView{}, err
	}
	authorizationRevision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil {
		return downloadPreparationView{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	record := downloadPreparationRecord{
		DownloadPreparation: DownloadPreparation{ID: randomID("dlp"), MediaID: mediaID, MediaTitle: item.Title, QualityProfile: profile, State: "queued", CreatedAt: now, UpdatedAt: now},
		ServerID:            identity.ServerID, AccountID: accountIDForUser(user), ProfileID: viewerProfileID(user), AuthorizationRevision: authorizationRevision,
		SizeKind: "unknown",
	}
	artifactAvailable := false
	if profile == "source" {
		target, targetErr := s.mediaDownloadGrantTargetContext(ctx, item, profile)
		if targetErr != nil {
			return downloadPreparationView{}, targetErr
		}
		artifactAvailable = true
		if source, sourceErr := s.downloadSourceForRequestContext(ctx, item, target.Profile); sourceErr == nil {
			record.SizeBytes, record.SizeKind = measuredPreparedDownloadSize(source)
		}
	} else if target, targetErr := s.mediaDownloadGrantTargetContext(ctx, item, profile); targetErr == nil {
		artifactAvailable = true
		if source, sourceErr := s.downloadSourceForRequestContext(ctx, item, target.Profile); sourceErr == nil {
			record.SizeBytes, record.SizeKind = measuredPreparedDownloadSize(source)
		}
	} else {
		record.SizeBytes, record.SizeKind = estimatedPreparedDownloadSize(item, profile), "estimated"
	}
	if artifactAvailable {
		record.ArtifactExpiresAt = s.preparedDownloadArtifactExpiryContext(ctx, record.MediaID, record.QualityProfile)
	}
	initialState := record.State
	initialProgress := record.Progress
	var existingID string
	jobCreated := false
	err = s.withUserTxTagged(ctx, []string{"downloads", "jobs"}, func(tx *sql.Tx) error {
		// WithTxRetryStats may rerun this callback after a SQLite writer
		// collision. Do not carry a rolled-back job or a losing idempotency
		// result into the next attempt.
		existingID = ""
		jobCreated = false
		record.JobID = ""
		record.State = initialState
		record.Progress = initialProgress
		if err := tx.QueryRowContext(ctx, `
			SELECT id FROM download_preparations
			WHERE server_id = ? AND account_id = ? AND profile_id = ? AND media_id = ? AND quality_profile = ? AND removed_at = ''
			LIMIT 1`, record.ServerID, record.AccountID, record.ProfileID, record.MediaID, record.QualityProfile).Scan(&existingID); err == nil {
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var total, active int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM download_preparations WHERE server_id = ? AND profile_id = ? AND removed_at = ''`, record.ServerID, record.ProfileID).Scan(&total); err != nil {
			return err
		}
		if total >= maxDownloadPreparationsPerProfile {
			return errDownloadPreparationLimit
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM download_preparations WHERE server_id = ? AND account_id = ? AND removed_at = '' AND state IN ('queued', 'running')`, record.ServerID, record.AccountID).Scan(&active); err != nil {
			return err
		}
		if active >= maxActiveDownloadPreparationsPerAccount {
			return errDownloadPreparationLimit
		}
		// Claim the preparation identity before attaching shared background work.
		// A losing writer therefore cannot leave an orphan job behind.
		result, err := tx.ExecContext(ctx, `
			INSERT INTO download_preparations (
				id, server_id, account_id, profile_id, authorization_revision, media_id, quality_profile, state,
				job_id, media_version_id, version_fingerprint, artifact_sha256, size_bytes, size_kind, artifact_expires_at,
				progress, error_code, created_at, updated_at, cancelled_at, removed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', '', ?, ?, ?, ?, '', ?, ?, '', '')
			ON CONFLICT(server_id, account_id, profile_id, media_id, quality_profile)
			WHERE removed_at = '' DO NOTHING`,
			record.ID, record.ServerID, record.AccountID, record.ProfileID, record.AuthorizationRevision, record.MediaID, record.QualityProfile,
			record.State, record.JobID, record.SizeBytes, record.SizeKind, record.ArtifactExpiresAt, record.Progress, record.CreatedAt, record.UpdatedAt)
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if inserted == 0 {
			// The partial unique index is the final idempotency authority across
			// server processes. If another writer won after our initial lookup,
			// return that committed preparation instead of surfacing an internal
			// SQLite constraint error or creating a duplicate viewer interest.
			return tx.QueryRowContext(ctx, `
				SELECT id FROM download_preparations
				WHERE server_id = ? AND account_id = ? AND profile_id = ? AND media_id = ? AND quality_profile = ? AND removed_at = ''
				LIMIT 1`, record.ServerID, record.AccountID, record.ProfileID, record.MediaID, record.QualityProfile).Scan(&existingID)
		}
		var job Job
		var created bool
		if artifactAvailable {
			job, created, err = attachDownloadArtifactVerificationJobTx(ctx, tx, record)
		} else {
			job, created, err = attachOptimizedDownloadJobTx(ctx, tx, item, profile)
		}
		if err != nil {
			return err
		}
		record.JobID, record.Progress, record.State = job.ID, job.Progress, normalizedDownloadPreparationJobState(job.Status)
		jobCreated = created
		result, err = tx.ExecContext(ctx, `
			UPDATE download_preparations
			SET state = ?, job_id = ?, progress = ?, updated_at = ?
			WHERE id = ? AND server_id = ? AND account_id = ? AND profile_id = ? AND removed_at = ''`,
			record.State, record.JobID, record.Progress, record.UpdatedAt,
			record.ID, record.ServerID, record.AccountID, record.ProfileID)
		if err != nil {
			return err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if updated != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err != nil {
		return downloadPreparationView{}, err
	}
	if existingID != "" {
		existing, err := s.downloadPreparationForUserContext(ctx, user, existingID)
		if err != nil {
			return downloadPreparationView{}, err
		}
		// POST is an explicit intent to prepare the current artifact. A stale
		// verified binding may therefore converge on the same durable interest
		// through the lifecycle writer; GET remains a pure projection.
		if existing.State == "failed" && (existing.ErrorCode == "prepared_artifact_changed" ||
			existing.ErrorCode == "artifact_verification_missing" || existing.ErrorCode == "prepared_artifact_missing") {
			return s.updateDownloadPreparationContext(ctx, user, existingID, "retry")
		}
		return existing, nil
	}
	if jobCreated {
		s.signalJobWake()
	}
	return downloadPreparationPublic(record), nil
}

func attachOptimizedDownloadJobTx(ctx context.Context, tx *sql.Tx, item MediaItem, profile string) (Job, bool, error) {
	preset, ok := optimized.Lookup(profile)
	if !ok {
		return Job{}, false, errInvalidDownloadGrantProfile
	}
	var job Job
	var metadataJSON string
	activeKey := "optimize_version|media|" + strings.TrimSpace(item.ID) + "|profile=" + strings.TrimSpace(profile)
	err := tx.QueryRowContext(ctx, `
		SELECT id, type, status, progress, message, resource_type, resource_id, COALESCE(metadata_json, '{}'), created_at, updated_at
		FROM jobs
		WHERE active_key = ?
			AND status IN ('queued', 'running')
		ORDER BY created_at DESC LIMIT 1`, activeKey).Scan(
		&job.ID, &job.Type, &job.Status, &job.Progress, &job.Message, &job.ResourceType, &job.ResourceID, &metadataJSON, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `
			SELECT id, type, status, progress, message, resource_type, resource_id, COALESCE(metadata_json, '{}'), created_at, updated_at
			FROM jobs
			WHERE type = 'optimize_version' AND resource_type = 'media' AND resource_id = ?
			AND status IN ('queued', 'running')
			AND COALESCE(json_extract(metadata_json, '$.profile'), '') = ?
			ORDER BY created_at DESC LIMIT 1`, item.ID, profile).Scan(
			&job.ID, &job.Type, &job.Status, &job.Progress, &job.Message, &job.ResourceType, &job.ResourceID, &metadataJSON, &job.CreatedAt, &job.UpdatedAt)
	}
	if err == nil {
		job.Metadata = decodeJobMetadata(metadataJSON)
		job.ActiveKey = activeKey
		return job, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	job = Job{
		ID: randomID("job"), Type: "optimize_version", Status: "queued", Progress: 0,
		Message:      fmt.Sprintf("Optimized %s download prepared for %s.", preset.Label, item.Title),
		ResourceType: "media", ResourceID: item.ID,
		Metadata: map[string]string{"profile": profile, "purpose": "download"}, ActiveKey: activeKey,
		Priority: foundationcontract.WorkClassBackgroundMedia, Phase: "queued", CreatedAt: now, UpdatedAt: now,
	}
	rawMetadata, err := json.Marshal(job.Metadata)
	if err != nil {
		return Job{}, false, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO jobs (
			id, type, status, progress, message, resource_type, resource_id, metadata_json,
			attempt_count, next_run_at, leased_by, lease_expires_at, last_error, failure_kind,
			deferred_until, active_key, priority, phase, progress_current, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, '', '', '', '', '', '', ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Type, job.Status, job.Progress, job.Message, job.ResourceType, job.ResourceID,
		string(rawMetadata), job.ActiveKey, job.Priority, job.Phase, job.Progress, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		if isActiveJobUniqueConflict(err) {
			if scanErr := tx.QueryRowContext(ctx, `
				SELECT id, type, status, progress, message, resource_type, resource_id, COALESCE(metadata_json, '{}'), created_at, updated_at
				FROM jobs WHERE active_key = ? AND status IN ('queued', 'running')
				ORDER BY created_at ASC LIMIT 1`, activeKey).Scan(
				&job.ID, &job.Type, &job.Status, &job.Progress, &job.Message, &job.ResourceType, &job.ResourceID, &metadataJSON, &job.CreatedAt, &job.UpdatedAt); scanErr == nil {
				job.Metadata = decodeJobMetadata(metadataJSON)
				job.ActiveKey = activeKey
				return job, false, nil
			}
		}
		return Job{}, false, err
	}
	return job, true, nil
}

func estimatedPreparedDownloadSize(item MediaItem, profile string) int64 {
	preset, ok := transcodePresets[profile]
	if !ok {
		// Optimized-version presets are a separate, versioned registry from the
		// live transcode profiles. Keep download planning estimates available for
		// those canonical IDs without treating them as live playback profiles.
		optimizedRates := map[string]transcodePreset{
			"universal-1080p": {videoK: 8000, audioK: 192}, "universal-720p": {videoK: 4000, audioK: 128}, "universal-480p": {videoK: 1800, audioK: 128},
			"efficient-4k": {videoK: 16000, audioK: 384}, "efficient-1080p": {videoK: 6000, audioK: 256}, "efficient-720p": {videoK: 3000, audioK: 192},
			"maximum-compression-source": {videoK: 5000, audioK: 192}, "maximum-compression-1080p": {videoK: 3500, audioK: 192},
		}
		preset, ok = optimizedRates[profile]
	}
	if !ok {
		return 0
	}
	var sourceBytes int64
	for _, mediaFile := range item.MediaFiles {
		if mediaFile.Available && mediaFile.SizeBytes > sourceBytes {
			sourceBytes = mediaFile.SizeBytes
		}
	}
	if item.DurationSeconds <= 0 {
		if sourceBytes > 0 {
			return saturatingEstimate(float64(sourceBytes) * 2)
		}
		return unknownMediaOutputReservation
	}
	bitrateKbps := preset.videoK + preset.audioK
	switch strings.ToLower(strings.TrimSpace(item.Type)) {
	case "track", "audio", "audiobook", "podcast", "book":
		bitrateKbps = preset.audioK
	}
	// Include three percent for the MP4 container and seek metadata. This value
	// is a planning estimate only and is replaced by a measured byte count when
	// the artifact becomes available.
	estimate := int64(item.DurationSeconds) * int64(bitrateKbps) * 1000 / 8 * 103 / 100
	if sourceBytes > 0 {
		if bySource := saturatingEstimate(float64(sourceBytes) * 2); bySource > estimate {
			return bySource
		}
	}
	return estimate
}

func measuredPreparedDownloadSize(source mediaDownloadSource) (int64, string) {
	if path := strings.TrimSpace(source.path); path != "" {
		if info, err := os.Stat(filepath.Clean(path)); err == nil && !info.IsDir() {
			return info.Size(), "exact"
		}
	}
	if source.sizeBytes > 0 {
		return source.sizeBytes, "estimated"
	}
	return 0, "unknown"
}

func (s *Server) preparedDownloadArtifactExpiryContext(ctx context.Context, mediaID, profile string) string {
	if profile == "source" {
		return ""
	}
	settings := s.optimizedVersionSettings()
	if !settings.AutoDelete || settings.RetentionDays <= 0 {
		return ""
	}
	var refreshedAt string
	if err := s.queryUserRow(ctx, `
		SELECT updated_at FROM optimized_versions
		WHERE media_id = ? AND profile = ?
		ORDER BY updated_at DESC LIMIT 1`, mediaID, profile).Scan(&refreshedAt); err != nil {
		return ""
	}
	refreshed, err := time.Parse(time.RFC3339, refreshedAt)
	if err != nil {
		return ""
	}
	return refreshed.Add(time.Duration(settings.RetentionDays) * 24 * time.Hour).UTC().Format(time.RFC3339)
}

func (s *Server) downloadPreparationRecordContext(ctx context.Context, where string, args ...any) (downloadPreparationRecord, error) {
	var record downloadPreparationRecord
	err := s.queryUserRow(ctx, `
		SELECT dp.id, dp.server_id, dp.account_id, dp.profile_id, dp.authorization_revision, dp.media_id, COALESCE(m.title, ''),
			dp.quality_profile, dp.state, dp.job_id, dp.media_version_id, dp.version_fingerprint, dp.artifact_sha256,
			dp.size_bytes, dp.size_kind, dp.artifact_expires_at, dp.progress, dp.error_code,
			dp.created_at, dp.updated_at, dp.cancelled_at, dp.removed_at
		FROM download_preparations dp
		JOIN media_items m ON m.id = dp.media_id `+where, args...).Scan(
		&record.ID, &record.ServerID, &record.AccountID, &record.ProfileID, &record.AuthorizationRevision, &record.MediaID, &record.MediaTitle,
		&record.QualityProfile, &record.State, &record.JobID, &record.MediaVersionID, &record.VersionFingerprint, &record.ArtifactSHA256,
		&record.SizeBytes, &record.SizeKind, &record.ArtifactExpiresAt, &record.Progress, &record.ErrorCode,
		&record.CreatedAt, &record.UpdatedAt, &record.CancelledAt, &record.RemovedAt)
	return record, err
}

func (s *Server) materializeDownloadPreparationContext(ctx context.Context, user User, record downloadPreparationRecord) (downloadPreparationView, error) {
	identity, err := s.systemIdentityContext(ctx)
	if err != nil {
		return downloadPreparationView{}, err
	}
	if record.ServerID != identity.ServerID || record.AccountID != accountIDForUser(user) || record.ProfileID != viewerProfileID(user) || record.RemovedAt != "" {
		return downloadPreparationView{}, sql.ErrNoRows
	}
	currentRevision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil {
		return downloadPreparationView{}, err
	}
	if record.AuthorizationRevision == "" || record.AuthorizationRevision != currentRevision {
		record.State, record.ErrorCode = "unavailable", "authorization_changed"
	}
	if record.State == "queued" || record.State == "running" {
		if record.JobID == "" {
			record.State, record.ErrorCode = "failed", "preparation_job_missing"
		} else if job, jobErr := s.getJob(record.JobID); jobErr == nil {
			record.Progress = max(0, min(100, job.Progress))
			record.State = normalizedDownloadPreparationJobState(job.Status)
			if record.State == "failed" {
				record.ErrorCode = safeDownloadPreparationFailureCode(job.FailureKind)
			}
		} else {
			record.State, record.ErrorCode = "failed", "preparation_failed"
		}
	}
	if record.State == "ready" {
		if !validOfflineArtifact(record.ArtifactSHA256, record.SizeBytes) || record.MediaVersionID == "" || record.VersionFingerprint == "" {
			record.State, record.ErrorCode = "failed", "artifact_verification_missing"
		} else if item, err := s.getMediaDownloadSeedForUser(ctx, user, record.MediaID); err == nil {
			if target, targetErr := s.mediaDownloadGrantTargetContext(ctx, item, record.QualityProfile); targetErr == nil {
				fingerprint, fenceErr := s.preparedDownloadArtifactFenceContext(ctx, item, target)
				if fenceErr != nil || target.VersionID != record.MediaVersionID || fingerprint != record.VersionFingerprint {
					record.State, record.ErrorCode = "failed", "prepared_artifact_changed"
				} else {
					record.State, record.Progress, record.ErrorCode = "ready", 100, ""
					record.ArtifactExpiresAt = s.preparedDownloadArtifactExpiryContext(ctx, record.MediaID, record.QualityProfile)
				}
			} else if record.State == "ready" {
				record.State, record.ErrorCode = "failed", "prepared_artifact_missing"
			}
		} else if record.State == "ready" {
			record.State, record.ErrorCode = "unavailable", "media_unavailable"
		}
	}
	return downloadPreparationPublic(record), nil
}

func normalizedDownloadPreparationJobState(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued":
		return "queued"
	case "running":
		return "running"
	case "complete", "completed":
		return "ready"
	case "cancelled":
		return "cancelled"
	default:
		return "failed"
	}
}

func downloadPreparationPublic(record downloadPreparationRecord) downloadPreparationView {
	preparation := record.DownloadPreparation
	preparation.FailureMessageID = downloadPreparationFailureMessageID(preparation.ErrorCode)
	preparation.ErrorCode = safeDownloadPreparationFailureCode(preparation.ErrorCode)
	preparation.CanPause = preparation.State == "queued" || preparation.State == "running"
	preparation.CanCancel = preparation.State == "queued" || preparation.State == "running" || preparation.State == "paused"
	preparation.CanRetry = preparation.State == "paused" || preparation.State == "failed" || preparation.State == "cancelled" || preparation.State == "unavailable"
	preparation.CanRemove = true
	sizeKind := record.SizeKind
	if sizeKind != "exact" && sizeKind != "estimated" {
		sizeKind = "unknown"
	}
	return downloadPreparationView{DownloadPreparation: preparation, SizeKind: sizeKind, ArtifactExpiresAt: record.ArtifactExpiresAt}
}

func downloadPreparationFailureMessageID(value string) string {
	switch safeDownloadPreparationFailureCode(value) {
	case "":
		return ""
	case "storage_full":
		return "download.storage-full"
	default:
		return "download.failed"
	}
}

func safeDownloadPreparationFailureCode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "authorization_changed", "preparation_job_missing", "preparation_failed", "artifact_verification_missing", "prepared_artifact_changed", "prepared_artifact_missing", "media_unavailable", "media_identity_changed", "storage_full", "source_missing", "cancelled":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "preparation_failed"
	}
}

func (s *Server) listDownloadPreparationsContext(ctx context.Context, user User) ([]downloadPreparationView, error) {
	identity, err := s.systemIdentityContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryUserRead(ctx, `SELECT id FROM download_preparations WHERE server_id = ? AND account_id = ? AND profile_id = ? AND removed_at = '' ORDER BY updated_at DESC LIMIT ?`, identity.ServerID, accountIDForUser(user), viewerProfileID(user), maxDownloadPreparationsPerProfile)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	items := make([]downloadPreparationView, 0, len(ids))
	for _, id := range ids {
		item, err := s.downloadPreparationForUserContext(ctx, user, id)
		if err == nil {
			items = append(items, item)
		}
	}
	return items, rows.Err()
}

func (s *Server) downloadPreparationForUserContext(ctx context.Context, user User, preparationID string) (downloadPreparationView, error) {
	identity, err := s.systemIdentityContext(ctx)
	if err != nil {
		return downloadPreparationView{}, err
	}
	record, err := s.downloadPreparationRecordContext(ctx, `WHERE dp.id = ? AND dp.server_id = ? AND dp.account_id = ? AND dp.profile_id = ? AND dp.removed_at = '' LIMIT 1`, preparationID, identity.ServerID, accountIDForUser(user), viewerProfileID(user))
	if err != nil {
		return downloadPreparationView{}, err
	}
	return s.materializeDownloadPreparationContext(ctx, user, record)
}

func (s *Server) updateDownloadPreparationContext(ctx context.Context, user User, preparationID, action string) (downloadPreparationView, error) {
	identity, err := s.systemIdentityContext(ctx)
	if err != nil {
		return downloadPreparationView{}, err
	}
	record, err := s.downloadPreparationRecordContext(ctx, `WHERE dp.id = ? AND dp.server_id = ? AND dp.account_id = ? AND dp.profile_id = ? AND dp.removed_at = '' LIMIT 1`, preparationID, identity.ServerID, accountIDForUser(user), viewerProfileID(user))
	if err != nil {
		return downloadPreparationView{}, err
	}
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "pause", "cancel", "retry", "resume", "remove":
	default:
		return downloadPreparationView{}, errDownloadPreparationAction
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var item MediaItem
	artifactAvailable := false
	artifactSize := int64(0)
	artifactSizeKind := "unknown"
	artifactExpiresAt := ""
	if action == "retry" || action == "resume" {
		item, err = s.getMediaDownloadSeedForUser(ctx, user, record.MediaID)
		if err != nil {
			return downloadPreparationView{}, err
		}
		_, err = s.mediaDownloadGrantTargetContext(ctx, item, record.QualityProfile)
		artifactAvailable = err == nil
		if artifactAvailable {
			if source, sourceErr := s.downloadSourceForRequestContext(ctx, item, record.QualityProfile); sourceErr == nil {
				artifactSize, artifactSizeKind = measuredPreparedDownloadSize(source)
			}
			artifactExpiresAt = s.preparedDownloadArtifactExpiryContext(ctx, record.MediaID, record.QualityProfile)
		}
	}
	authorizationRevision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil {
		return downloadPreparationView{}, err
	}
	jobCreated := false
	jobCancelled := false
	err = s.withUserTxTagged(ctx, []string{"downloads", "jobs"}, func(tx *sql.Tx) error {
		var currentState, currentRemovedAt string
		if err := tx.QueryRowContext(ctx, `
			SELECT state, removed_at FROM download_preparations
			WHERE id = ? AND server_id = ? AND account_id = ? AND profile_id = ?`,
			record.ID, record.ServerID, record.AccountID, record.ProfileID).Scan(&currentState, &currentRemovedAt); err != nil {
			return err
		}
		if currentRemovedAt != "" {
			return sql.ErrNoRows
		}
		record.State = currentState
		switch action {
		case "pause":
			if record.State != "queued" && record.State != "running" {
				return errDownloadPreparationAction
			}
			record.State = "paused"
		case "cancel":
			record.State, record.CancelledAt = "cancelled", now
		case "retry", "resume":
			{
				var active int
				if err := tx.QueryRowContext(ctx, `
					SELECT COUNT(*) FROM download_preparations
					WHERE server_id = ? AND account_id = ? AND id <> ? AND removed_at = '' AND state IN ('queued', 'running')`,
					record.ServerID, record.AccountID, record.ID).Scan(&active); err != nil {
					return err
				}
				if active >= maxActiveDownloadPreparationsPerAccount {
					return errDownloadPreparationLimit
				}
				var job Job
				var created bool
				if artifactAvailable {
					job, created, err = attachDownloadArtifactVerificationJobTx(ctx, tx, record)
				} else {
					job, created, err = attachOptimizedDownloadJobTx(ctx, tx, item, record.QualityProfile)
				}
				if err != nil {
					return err
				}
				jobCreated = created
				record.JobID, record.Progress, record.State, record.ErrorCode, record.CancelledAt = job.ID, job.Progress, normalizedDownloadPreparationJobState(job.Status), "", ""
				if artifactAvailable {
					record.SizeBytes, record.SizeKind, record.ArtifactExpiresAt = artifactSize, artifactSizeKind, artifactExpiresAt
				} else {
					record.SizeBytes, record.SizeKind, record.ArtifactExpiresAt = estimatedPreparedDownloadSize(item, record.QualityProfile), "estimated", ""
				}
				record.MediaVersionID, record.VersionFingerprint, record.ArtifactSHA256 = "", "", ""
			}
			record.AuthorizationRevision = authorizationRevision
		case "remove":
			record.RemovedAt = now
		}
		result, err := tx.ExecContext(ctx, `UPDATE download_preparations SET state = ?, job_id = ?, progress = ?, media_version_id = ?, version_fingerprint = ?, artifact_sha256 = ?, size_bytes = ?, size_kind = ?, artifact_expires_at = ?, error_code = ?, authorization_revision = ?, cancelled_at = ?, removed_at = ?, updated_at = ? WHERE id = ? AND server_id = ? AND account_id = ? AND profile_id = ? AND removed_at = ''`,
			record.State, record.JobID, record.Progress, record.MediaVersionID, record.VersionFingerprint, record.ArtifactSHA256, record.SizeBytes, record.SizeKind, record.ArtifactExpiresAt, record.ErrorCode, record.AuthorizationRevision, record.CancelledAt, record.RemovedAt, now,
			record.ID, record.ServerID, record.AccountID, record.ProfileID)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return sql.ErrNoRows
		}
		if (action == "pause" || action == "cancel" || action == "remove") && record.JobID != "" {
			result, err := tx.ExecContext(ctx, `
				UPDATE jobs SET status = 'cancelled', progress = 100, message = 'Download preparation cancelled.', updated_at = ?
				WHERE id = ? AND status IN ('queued', 'running')
					AND NOT EXISTS (
						SELECT 1 FROM download_preparations dp
						WHERE dp.server_id = ? AND dp.job_id = jobs.id AND dp.removed_at = '' AND dp.state IN ('queued', 'running')
					)`, now, record.JobID, record.ServerID)
			if err != nil {
				return err
			}
			cancelled, err := result.RowsAffected()
			if err != nil {
				return err
			}
			jobCancelled = cancelled == 1
		}
		return nil
	})
	if err != nil {
		return downloadPreparationView{}, err
	}
	if jobCancelled {
		s.jobCancelMu.Lock()
		cancel := s.jobCancels[record.JobID]
		s.jobCancelMu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	if jobCreated {
		s.signalJobWake()
	}
	record.UpdatedAt = now
	return downloadPreparationPublic(record), nil
}

func (s *Server) createDownloadPreparationBatchContext(ctx context.Context, user User, req downloadPreparationBatchRequest) (downloadPreparationBatchResponse, error) {
	ids := make([]string, 0, len(req.MediaIDs))
	seen := map[string]bool{}
	for _, rawID := range req.MediaIDs {
		id := strings.TrimSpace(rawID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if containerID := strings.TrimSpace(req.ContainerID); containerID != "" {
		if _, err := s.getMediaDownloadSeedForUser(ctx, user, containerID); err != nil {
			return downloadPreparationBatchResponse{}, sql.ErrNoRows
		}
		containerIDs, err := s.downloadableDescendantIDsContext(ctx, containerID, maxDownloadPreparationsPerProfile+1)
		if err != nil {
			return downloadPreparationBatchResponse{}, err
		}
		for _, id := range containerIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return downloadPreparationBatchResponse{}, errDownloadBatchEmpty
	}
	sort.Strings(ids)
	response := downloadPreparationBatchResponse{
		Items:    make([]downloadPreparationView, 0, min(len(ids), maxDownloadPreparationsPerProfile)),
		Rejected: []downloadPreparationBatchFailure{}, Total: len(ids),
	}
	for index, mediaID := range ids {
		if index >= maxDownloadPreparationsPerProfile {
			response.Rejected = append(response.Rejected, downloadPreparationBatchFailure{MediaID: mediaID, MessageID: "download.failed"})
			continue
		}
		preparation, err := s.createDownloadPreparationContext(ctx, user, DownloadPreparationCreateRequest{MediaID: mediaID, QualityProfile: req.QualityProfile})
		if err != nil {
			response.Rejected = append(response.Rejected, downloadPreparationBatchFailure{MediaID: mediaID, MessageID: downloadPreparationBatchFailureMessageID(err)})
			continue
		}
		response.Items = append(response.Items, preparation)
	}
	return response, nil
}

func downloadPreparationBatchFailureMessageID(err error) string {
	switch {
	case errors.Is(err, errDownloadPreparationLimit), errors.Is(err, errInvalidDownloadGrantProfile), errors.Is(err, errDownloadGrantUnavailable), errors.Is(err, errUnsupportedPlaybackSource), errors.Is(err, sql.ErrNoRows):
		return "download.failed"
	default:
		return "download.failed"
	}
}

func (s *Server) downloadableDescendantIDsContext(ctx context.Context, containerID string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = maxDownloadPreparationsPerProfile + 1
	}
	rows, err := s.queryUserRead(ctx, `
		WITH RECURSIVE descendants(id) AS (
			SELECT id FROM media_items WHERE id = ?
			UNION ALL
			SELECT child.id FROM media_items child JOIN descendants parent ON child.parent_id = parent.id
		)
		SELECT m.id
		FROM media_items m
		WHERE m.id IN (SELECT id FROM descendants) AND m.id <> ?
			AND NOT EXISTS (SELECT 1 FROM media_items child WHERE child.parent_id = m.id)
		ORDER BY COALESCE(m.season_number, 0), COALESCE(m.episode_number, 0), COALESCE(m.index_number, 0), m.sort_title, m.id
		LIMIT ?`, containerID, containerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Server) createNextEpisodeDownloadPreparationContext(ctx context.Context, user User, req nextEpisodeDownloadRequest) (downloadPreparationView, error) {
	currentID := strings.TrimSpace(req.MediaID)
	if currentID == "" {
		return downloadPreparationView{}, errDownloadBatchEmpty
	}
	current, err := s.getMediaDownloadSeedForUser(ctx, user, currentID)
	if err != nil || strings.ToLower(current.Type) != "episode" {
		return downloadPreparationView{}, sql.ErrNoRows
	}
	var nextID string
	err = s.queryUserRow(ctx, `
		WITH current_episode AS (
			SELECT episode.id,
				COALESCE(season.parent_id, episode.parent_id, '') AS show_id,
				COALESCE(episode.season_number, season.season_number, 0) AS season_number,
				COALESCE(episode.episode_number, episode.index_number, 0) AS episode_number,
				COALESCE(episode.index_number, 0) AS index_number
			FROM media_items episode
			LEFT JOIN media_items season ON season.id = episode.parent_id AND season.type = 'season'
			WHERE episode.id = ? AND episode.type = 'episode'
		), candidates AS (
			SELECT episode.id,
				COALESCE(episode.season_number, season.season_number, 0) AS season_number,
				COALESCE(episode.episode_number, episode.index_number, 0) AS episode_number,
				COALESCE(episode.index_number, 0) AS index_number,
				episode.sort_title
			FROM media_items episode
			LEFT JOIN media_items season ON season.id = episode.parent_id AND season.type = 'season'
			JOIN current_episode current ON COALESCE(season.parent_id, episode.parent_id, '') = current.show_id
			WHERE episode.type = 'episode' AND episode.id <> current.id
				AND (
					COALESCE(episode.season_number, season.season_number, 0) > current.season_number OR
					(COALESCE(episode.season_number, season.season_number, 0) = current.season_number AND COALESCE(episode.episode_number, episode.index_number, 0) > current.episode_number) OR
					(COALESCE(episode.season_number, season.season_number, 0) = current.season_number AND COALESCE(episode.episode_number, episode.index_number, 0) = current.episode_number AND COALESCE(episode.index_number, 0) > current.index_number)
				)
		)
		SELECT id FROM candidates ORDER BY season_number, episode_number, index_number, sort_title, id LIMIT 1`, currentID).Scan(&nextID)
	if err != nil {
		return downloadPreparationView{}, err
	}
	// The candidate query establishes order only. The normal preparation path
	// still performs the authoritative profile/library/policy authorization.
	return s.createDownloadPreparationContext(ctx, user, DownloadPreparationCreateRequest{MediaID: nextID, QualityProfile: req.QualityProfile})
}

func (s *Server) issueDownloadPreparationGrantContext(ctx context.Context, user User, preparationID, delivery string) (MediaDownloadGrantResponse, error) {
	if delivery == "native" {
		return s.issueNativeDownloadPreparationGrantContext(ctx, user, preparationID)
	}
	preparation, err := s.downloadPreparationForUserContext(ctx, user, preparationID)
	if err != nil {
		return MediaDownloadGrantResponse{}, err
	}
	if preparation.State != "ready" {
		return MediaDownloadGrantResponse{}, errDownloadPreparationNotReady
	}
	grant, _, _, err := s.issueMediaDownloadGrantForPreparation(ctx, user, preparation.MediaID, preparation.QualityProfile, preparationID)
	if err != nil {
		return MediaDownloadGrantResponse{}, err
	}
	return grant, nil
}
