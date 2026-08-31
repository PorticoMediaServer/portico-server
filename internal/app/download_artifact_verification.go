package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const downloadArtifactVerificationJobType = "download_artifact_verify"

type verifiedPreparedDownloadArtifact struct {
	SHA256             string
	SizeBytes          int64
	MediaVersionID     string
	VersionFingerprint string
}

func attachDownloadArtifactVerificationJobTx(ctx context.Context, tx *sql.Tx, record downloadPreparationRecord) (Job, bool, error) {
	descriptor, ok := durableJobDescriptorForType(downloadArtifactVerificationJobType)
	if !ok {
		return Job{}, false, errors.New("download artifact verification job is not registered")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	job := Job{
		ID: randomID("job"), Type: downloadArtifactVerificationJobType, Status: "queued", Progress: 0,
		Message: "Verifying prepared download bytes.", ResourceType: "download_preparation", ResourceID: record.ID,
		Priority: descriptor.WorkClass, Phase: "queued", CreatedAt: now, UpdatedAt: now,
	}
	job.ActiveKey = jobActiveKeyFor(job.Type, job.ResourceType, job.ResourceID, nil)
	var metadataJSON string
	err := tx.QueryRowContext(ctx, `
		SELECT id, type, status, progress, message, resource_type, resource_id, COALESCE(metadata_json, '{}'), created_at, updated_at
		FROM jobs WHERE active_key = ? AND status IN ('queued', 'running') LIMIT 1`, job.ActiveKey).Scan(
		&job.ID, &job.Type, &job.Status, &job.Progress, &job.Message, &job.ResourceType, &job.ResourceID,
		&metadataJSON, &job.CreatedAt, &job.UpdatedAt)
	if err == nil {
		job.Metadata = decodeJobMetadata(metadataJSON)
		return job, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO jobs (
			id, type, status, progress, message, resource_type, resource_id, metadata_json,
			attempt_count, next_run_at, leased_by, lease_expires_at, last_error, failure_kind,
			deferred_until, active_key, priority, phase, progress_current, progress_total,
			result_reference, error_code, retry_eligible, cancellation_requested_at,
			worker_acknowledged_at, interrupted_at, retention_until, created_at, updated_at
		) VALUES (?, ?, 'queued', 0, ?, ?, ?, '{}', 0, '', '', '', '', '', '', ?, ?, 'queued', 0, 0, '', '', 0, '', '', '', '', ?, ?)`,
		job.ID, job.Type, job.Message, job.ResourceType, job.ResourceID, job.ActiveKey, job.Priority, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

// completeOptimizeVersionAndQueueDownloadVerifications atomically hands every
// viewer preparation attached to an optimized producer to its own durable
// artifact-finalization job. No read/poll endpoint owns this transition.
func (s *Server) completeOptimizeVersionAndQueueDownloadVerifications(job Job, message string) error {
	writeCtx, cancel := boundedJobWriteContext()
	defer cancel()
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339Nano)
	message = s.sanitizeJobErrorMessage(message)
	err := s.withBackgroundTxTagged(writeCtx, []string{"downloads", "jobs"}, func(tx *sql.Tx) error {
		var active int
		if err := tx.QueryRowContext(writeCtx, `
			SELECT COUNT(*) FROM jobs
			WHERE id = ? AND type = 'optimize_version' AND status = 'running' AND leased_by = ?
				AND cancellation_requested_at = ''`, job.ID, s.jobLeaseOwner(job.ID)).Scan(&active); err != nil {
			return err
		}
		if active != 1 {
			return errors.New("optimized producer is no longer authorized to complete")
		}
		rows, err := tx.QueryContext(writeCtx, `
			SELECT id, server_id, account_id, profile_id, authorization_revision, media_id, quality_profile,
				state, job_id, created_at, updated_at
			FROM download_preparations
			WHERE job_id = ? AND state IN ('queued', 'running') AND removed_at = ''
			ORDER BY id`, job.ID)
		if err != nil {
			return err
		}
		records := []downloadPreparationRecord{}
		for rows.Next() {
			var record downloadPreparationRecord
			if err := rows.Scan(&record.ID, &record.ServerID, &record.AccountID, &record.ProfileID,
				&record.AuthorizationRevision, &record.MediaID, &record.QualityProfile, &record.State,
				&record.JobID, &record.CreatedAt, &record.UpdatedAt); err != nil {
				_ = rows.Close()
				return err
			}
			records = append(records, record)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, record := range records {
			verification, _, err := attachDownloadArtifactVerificationJobTx(writeCtx, tx, record)
			if err != nil {
				return err
			}
			result, err := tx.ExecContext(writeCtx, `
				UPDATE download_preparations
				SET state = 'queued', job_id = ?, progress = 0, media_version_id = '', version_fingerprint = '',
					artifact_sha256 = '', error_code = '', updated_at = ?
				WHERE id = ? AND job_id = ? AND state IN ('queued', 'running') AND removed_at = ''`,
				verification.ID, now, record.ID, job.ID)
			if err != nil {
				return err
			}
			if affected, err := result.RowsAffected(); err != nil || affected != 1 {
				return sql.ErrNoRows
			}
		}
		result, err := tx.ExecContext(writeCtx, `
			UPDATE jobs SET status = 'complete', progress = 100, message = ?, phase = 'complete',
				progress_current = 100, retry_eligible = 0,
				retention_until = CASE WHEN retention_until = '' THEN ? ELSE retention_until END,
				leased_by = '', lease_expires_at = '', updated_at = ?
			WHERE id = ? AND type = 'optimize_version' AND status = 'running' AND leased_by = ?
				AND cancellation_requested_at = ''`,
			message, s.jobRetentionDeadline(nowTime), now, job.ID, s.jobLeaseOwner(job.ID))
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err == nil {
		s.signalJobWake()
	}
	return err
}

// failOptimizeVersionAndDownloadPreparations is the terminal failure owner for
// an optimized producer and every profile-owned preparation currently waiting
// on it. A cancelled or lease-lost producer cannot overwrite either lifecycle.
func (s *Server) failOptimizeVersionAndDownloadPreparations(job Job, message string) error {
	writeCtx, cancel := boundedJobWriteContext()
	defer cancel()
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339Nano)
	message = s.sanitizeJobErrorMessage(message)
	return s.withBackgroundTxTagged(writeCtx, []string{"downloads", "jobs"}, func(tx *sql.Tx) error {
		var active int
		if err := tx.QueryRowContext(writeCtx, `
			SELECT COUNT(*) FROM jobs
			WHERE id = ? AND type = 'optimize_version' AND status = 'running' AND leased_by = ?
				AND cancellation_requested_at = ''`, job.ID, s.jobLeaseOwner(job.ID)).Scan(&active); err != nil {
			return err
		}
		if active != 1 {
			return nil
		}
		if _, err := tx.ExecContext(writeCtx, `
			UPDATE download_preparations
			SET state = 'failed', progress = 100, media_version_id = '', version_fingerprint = '',
				artifact_sha256 = '', size_bytes = 0, size_kind = 'unknown', artifact_expires_at = '',
				error_code = 'preparation_failed', updated_at = ?
			WHERE job_id = ? AND state IN ('queued', 'running') AND removed_at = ''`, now, job.ID); err != nil {
			return err
		}
		result, err := tx.ExecContext(writeCtx, `
			UPDATE jobs SET status = 'failed', progress = 100, message = ?, phase = 'failed',
				progress_current = 100, last_error = ?, failure_kind = 'optimize_failed',
				error_code = 'optimize_failed', retry_eligible = 1,
				retention_until = CASE WHEN retention_until = '' THEN ? ELSE retention_until END,
				leased_by = '', lease_expires_at = '', updated_at = ?
			WHERE id = ? AND type = 'optimize_version' AND status = 'running' AND leased_by = ?
				AND cancellation_requested_at = ''`,
			message, message, s.jobRetentionDeadline(nowTime), now, job.ID, s.jobLeaseOwner(job.ID))
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
}

func (s *Server) runDownloadArtifactVerification(ctx context.Context, job Job) {
	if job.ResourceType != "download_preparation" || strings.TrimSpace(job.ResourceID) == "" {
		s.failDownloadArtifactVerification(ctx, job, "artifact_verification_invalid", "Download verification failed because the preparation is invalid.")
		return
	}
	_ = s.setJobMessage(job.ID, "running", 10, "Verifying prepared download bytes.")
	record, err := s.downloadPreparationRecordContext(ctx, `WHERE dp.id = ? LIMIT 1`, job.ResourceID)
	if err != nil || record.RemovedAt != "" || record.JobID != job.ID || (record.State != "queued" && record.State != "running") {
		s.failDownloadArtifactVerification(ctx, job, "artifact_verification_unavailable", "Download verification no longer has an active preparation.")
		return
	}
	user, err := s.getUser(record.AccountID)
	if err != nil {
		s.failDownloadArtifactVerification(ctx, job, "authorization_changed", "Download verification lost its account authority.")
		return
	}
	user.AccountID, user.ProfileID = record.AccountID, record.ProfileID
	user = s.hydratePlaybackVisibilityUserContext(ctx, user)
	revision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil || revision != record.AuthorizationRevision || !user.Permissions["downloadMedia"] {
		s.failDownloadArtifactVerification(ctx, job, "authorization_changed", "Download verification lost its viewer authority.")
		return
	}
	item, err := s.getMediaDownloadSeedForUser(ctx, user, record.MediaID)
	if err != nil {
		s.failDownloadArtifactVerification(ctx, job, "media_unavailable", "Download verification could not access the media item.")
		return
	}
	target, err := s.mediaDownloadGrantTargetContext(ctx, item, record.QualityProfile)
	if err != nil {
		s.failDownloadArtifactVerification(ctx, job, "prepared_artifact_missing", "Download verification could not access the prepared artifact.")
		return
	}
	artifact, err := s.verifyPreparedDownloadArtifactContext(ctx, item, target)
	if err != nil {
		s.failDownloadArtifactVerification(ctx, job, "artifact_verification_failed", "Download verification could not prove the prepared bytes.")
		return
	}
	postTarget, err := s.mediaDownloadGrantTargetContext(ctx, item, record.QualityProfile)
	if err != nil || postTarget.VersionID != target.VersionID {
		s.failDownloadArtifactVerification(ctx, job, "artifact_changed", "The prepared artifact changed during verification.")
		return
	}
	postFingerprint, err := s.preparedDownloadArtifactFenceContext(ctx, item, postTarget)
	if err != nil || postFingerprint != artifact.VersionFingerprint {
		s.failDownloadArtifactVerification(ctx, job, "artifact_changed", "The prepared artifact changed during verification.")
		return
	}
	postRevision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil || postRevision != revision {
		s.failDownloadArtifactVerification(ctx, job, "authorization_changed", "Download verification lost its viewer authority.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = s.withBackgroundTxTagged(ctx, []string{"downloads", "jobs"}, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE download_preparations
			SET state = 'ready', progress = 100, media_version_id = ?, version_fingerprint = ?, artifact_sha256 = ?,
				size_bytes = ?, size_kind = 'exact', error_code = '', updated_at = ?
			WHERE id = ? AND server_id = ? AND account_id = ? AND profile_id = ? AND authorization_revision = ?
				AND job_id = ? AND state IN ('queued', 'running') AND removed_at = ''`,
			artifact.MediaVersionID, artifact.VersionFingerprint, artifact.SHA256, artifact.SizeBytes, now,
			record.ID, record.ServerID, record.AccountID, record.ProfileID, revision, job.ID)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return sql.ErrNoRows
		}
		result, err = tx.ExecContext(ctx, `
				UPDATE jobs SET status = 'complete', progress = 100, phase = 'complete', progress_current = 100,
				message = 'Prepared download bytes verified.', last_error = '', failure_kind = '', error_code = '',
				retry_eligible = 0, leased_by = '', lease_expires_at = '', updated_at = ?
			WHERE id = ? AND type = ? AND status = 'running' AND leased_by = ? AND cancellation_requested_at = ''`,
			now, job.ID, downloadArtifactVerificationJobType, s.jobLeaseOwner(job.ID))
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err != nil {
		s.failDownloadArtifactVerification(ctx, job, "artifact_verification_publish_failed", "Download verification could not publish the verified artifact.")
	}
}

func (s *Server) failDownloadArtifactVerification(ctx context.Context, job Job, code, message string) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_ = s.withBackgroundTxTagged(context.Background(), []string{"downloads", "jobs"}, func(tx *sql.Tx) error {
		_, _ = tx.ExecContext(context.Background(), `
			UPDATE download_preparations SET state = 'failed', progress = 100, error_code = ?, updated_at = ?
			WHERE id = ? AND job_id = ? AND state IN ('queued', 'running') AND removed_at = ''`, code, now, job.ResourceID, job.ID)
		_, err := tx.ExecContext(context.Background(), `
			UPDATE jobs SET status = 'failed', progress = 100, phase = 'failed', progress_current = 100,
				message = ?, last_error = ?, failure_kind = ?, error_code = ?, retry_eligible = 1,
				leased_by = '', lease_expires_at = '', updated_at = ?
			WHERE id = ? AND status IN ('queued', 'running') AND cancellation_requested_at = ''`, message, message, code, code, now, job.ID)
		return err
	})
}

func (s *Server) verifyPreparedDownloadArtifactContext(ctx context.Context, item MediaItem, target mediaDownloadGrantTarget) (verifiedPreparedDownloadArtifact, error) {
	source, err := s.downloadSourceForRequestContext(ctx, item, target.Profile)
	if err != nil {
		return verifiedPreparedDownloadArtifact{}, err
	}
	result := verifiedPreparedDownloadArtifact{MediaVersionID: target.VersionID}
	switch source.sourceKind {
	case "local", "optimized":
		if source.sourceKind == "optimized" {
			result.SHA256, result.SizeBytes, err = s.verifiedOptimizedPreparedArtifactContext(ctx, source)
		} else {
			result.SHA256, result.SizeBytes, err = s.hashLocalPreparedDownloadArtifact(ctx, source.path)
		}
		if err == nil {
			result.VersionFingerprint, err = s.preparedDownloadArtifactFenceContext(ctx, item, target)
		}
	case "remote-storage":
		result.SHA256, result.SizeBytes, err = s.hashRemoteStoragePreparedDownloadArtifact(ctx, item, source)
		if err == nil {
			result.VersionFingerprint, err = s.preparedDownloadArtifactFenceContext(ctx, item, target)
		}
	case "remote":
		result.SHA256, result.SizeBytes, result.VersionFingerprint, err = s.hashHTTPPreparedDownloadArtifact(ctx, source, target)
	default:
		err = errUnsupportedPlaybackSource
	}
	if err != nil || !validOfflineArtifact(result.SHA256, result.SizeBytes) || result.MediaVersionID == "" || result.VersionFingerprint == "" {
		return verifiedPreparedDownloadArtifact{}, errors.New("prepared download artifact verification failed")
	}
	return result, nil
}

func (s *Server) verifiedOptimizedPreparedArtifactContext(ctx context.Context, source mediaDownloadSource) (string, int64, error) {
	versionID := strings.TrimSpace(source.versionID)
	clean := filepath.Clean(source.path)
	if versionID == "" || !s.optimizedVersionPathAllowed(clean) {
		return "", 0, errors.New("optimized artifact identity is invalid")
	}
	var path, digest, state string
	var size int64
	if err := s.queryBackgroundRow(ctx, `
		SELECT path, artifact_sha256, size_bytes, state
		FROM optimized_versions WHERE id = ? AND state = 'ready'`, versionID).Scan(&path, &digest, &size, &state); err != nil {
		return "", 0, err
	}
	if state != "ready" || filepath.Clean(path) != clean || source.sizeBytes != size || !validOfflineArtifact(digest, size) ||
		!s.optimizedArtifactUsable(ctx, clean, size) {
		return "", 0, errors.New("optimized producer artifact facts are invalid")
	}
	return digest, size, nil
}

func (s *Server) preparedDownloadArtifactFenceContext(ctx context.Context, item MediaItem, target mediaDownloadGrantTarget) (string, error) {
	source, err := s.downloadSourceForRequestContext(ctx, item, target.Profile)
	if err != nil {
		return "", err
	}
	if source.sourceKind != "remote" {
		return target.VersionFingerprint, nil
	}
	parsed, err := validateExternalURL(source.sourceURL)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept-Encoding", "identity")
	response, err := liveTVHTTPClientForContext(ctx).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", errors.New("remote artifact identity is unavailable")
	}
	return remoteHTTPPreparedArtifactFingerprint(target.VersionFingerprint, response.Header, response.ContentLength)
}

func (s *Server) hashLocalPreparedDownloadArtifact(ctx context.Context, path string) (string, int64, error) {
	clean := filepath.Clean(path)
	type result struct {
		digest string
		size   int64
	}
	completed := make(chan result, 4)
	err := s.withPlaybackStorageIO(ctx, clean, playbackStorageAnalysis, "verify prepared download artifact", func(_ context.Context, progress func()) error {
		file, err := os.Open(clean)
		if err != nil {
			return err
		}
		defer file.Close()
		before, err := file.Stat()
		if err != nil || before.IsDir() || before.Size() <= 0 {
			return errors.New("prepared artifact is not a regular non-empty file")
		}
		hasher := sha256.New()
		buffer := make([]byte, 256*1024)
		var size int64
		for {
			count, readErr := file.Read(buffer)
			if count > 0 {
				_, _ = hasher.Write(buffer[:count])
				size += int64(count)
				progress()
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil || count == 0 {
				if readErr == nil {
					readErr = io.ErrNoProgress
				}
				return readErr
			}
		}
		after, err := file.Stat()
		if err != nil || after.Size() != before.Size() || after.ModTime() != before.ModTime() || size != before.Size() {
			return errors.New("prepared artifact changed during hashing")
		}
		completed <- result{digest: hex.EncodeToString(hasher.Sum(nil)), size: size}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	verified := <-completed
	return verified.digest, verified.size, nil
}

func (s *Server) hashRemoteStoragePreparedDownloadArtifact(ctx context.Context, item MediaItem, source mediaDownloadSource) (string, int64, error) {
	sourceID, objectPath, err := parseRemoteStorageLocator(source.sourceURL)
	if err != nil {
		return "", 0, err
	}
	var size int64
	var revision string
	if err := s.queryBackgroundRow(ctx, `
		SELECT object.size_bytes, object.revision FROM storage_remote_objects object
		JOIN storage_sources storage ON storage.id = object.source_id
		WHERE object.source_id = ? AND object.object_path = ? AND object.missing_since = '' AND storage.library_id = ?`,
		sourceID, objectPath, item.LibraryID).Scan(&size, &revision); err != nil || size <= 0 || revision != source.sourceRevision {
		return "", 0, errors.New("remote storage artifact identity changed")
	}
	backend, err := s.remoteBackendForSource(ctx, sourceID)
	if err != nil {
		return "", 0, err
	}
	reader, err := backend.OpenRange(ctx, objectPath, 0, size)
	if err != nil {
		return "", 0, err
	}
	return hashPreparedDownloadReader(ctx, reader, size)
}

func (s *Server) hashHTTPPreparedDownloadArtifact(ctx context.Context, source mediaDownloadSource, target mediaDownloadGrantTarget) (string, int64, string, error) {
	parsed, err := validateExternalURL(source.sourceURL)
	if err != nil {
		return "", 0, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", 0, "", err
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Accept-Encoding", "identity")
	response, err := liveTVHTTPClientForContext(ctx).Do(request)
	if err != nil {
		return "", 0, "", err
	}
	if response.StatusCode != http.StatusOK || response.ContentLength <= 0 {
		_ = response.Body.Close()
		return "", 0, "", errors.New("remote artifact does not expose a stable complete representation")
	}
	fingerprint, err := remoteHTTPPreparedArtifactFingerprint(target.VersionFingerprint, response.Header, response.ContentLength)
	if err != nil {
		_ = response.Body.Close()
		return "", 0, "", err
	}
	digest, size, err := hashPreparedDownloadReader(ctx, response.Body, response.ContentLength)
	return digest, size, fingerprint, err
}

func remoteHTTPPreparedArtifactFingerprint(base string, header http.Header, size int64) (string, error) {
	etag := strings.TrimSpace(header.Get("ETag"))
	if etag == "" || strings.HasPrefix(strings.ToLower(etag), "w/") || size <= 0 {
		return "", errors.New("remote artifact does not expose a strong entity tag and exact size")
	}
	material := strings.Join([]string{base, etag, strconv.FormatInt(size, 10)}, "\x00")
	return hashToken(material), nil
}

func hashPreparedDownloadReader(ctx context.Context, reader io.ReadCloser, expectedSize int64) (string, int64, error) {
	if reader == nil || expectedSize <= 0 {
		return "", 0, errors.New("prepared artifact reader is invalid")
	}
	defer reader.Close()
	quiet := storageIOOperationTimeout(storageSourceNetwork)
	if quiet <= 0 {
		quiet = 20 * time.Second
	}
	progress := make(chan struct{}, 1)
	done := make(chan struct{})
	timedOut := make(chan struct{})
	go func() {
		timer := time.NewTimer(quiet)
		defer timer.Stop()
		for {
			select {
			case <-progress:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(quiet)
			case <-timer.C:
				close(timedOut)
				_ = reader.Close()
				return
			case <-ctx.Done():
				_ = reader.Close()
				return
			case <-done:
				return
			}
		}
	}()
	defer close(done)
	hasher := sha256.New()
	buffer := make([]byte, 256*1024)
	var size int64
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			_, _ = hasher.Write(buffer[:count])
			size += int64(count)
			select {
			case progress <- struct{}{}:
			default:
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil || count == 0 {
			select {
			case <-timedOut:
				return "", 0, errors.New("prepared artifact read made no progress")
			default:
			}
			if readErr == nil {
				readErr = io.ErrNoProgress
			}
			return "", 0, readErr
		}
	}
	if size != expectedSize {
		return "", 0, fmt.Errorf("prepared artifact length %d did not match %d", size, expectedSize)
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}
