package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/optimized"
)

// playbackResponseWriter records whether delivery has crossed the HTTP commit
// boundary. Storage can fail after a range response has begun; at that point a
// JSON error would corrupt the media body and cannot change the status code.
// Unwrap preserves ResponseController access to the underlying writer.
type playbackResponseWriter struct {
	http.ResponseWriter
	committed bool
}

func (w *playbackResponseWriter) WriteHeader(status int) {
	w.committed = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *playbackResponseWriter) Write(value []byte) (int, error) {
	w.committed = true
	return w.ResponseWriter.Write(value)
}

func (w *playbackResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) handleMediaStream(w http.ResponseWriter, r *http.Request, user User, mediaID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !user.Permissions["playMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to play this media.")
		return
	}
	if r.Method == http.MethodGet {
		if !s.acquireUserStreamSlot(user.ID, user.MaxActiveStreams) {
			s.streamRejected.Add(1)
			w.Header().Set("Retry-After", "5")
			writeError(w, http.StatusTooManyRequests, "stream_busy", "This account already has several direct streams running. Try again shortly.")
			return
		}
		defer s.releaseUserStreamSlot(user.ID)
	}
	item, err := s.getMediaStreamSeedForUser(r.Context(), user, mediaID)
	if err != nil {
		writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
		return
	}
	// The lightweight authorization seed intentionally omits stream and file
	// facts. Rehydrate only the fields that participate in the canonical source
	// revision so this comparison uses the same inputs as session planning.
	item.Streams, err = s.listStreamsContext(r.Context(), item.ID)
	if err != nil {
		writeError(w, http.StatusConflict, "playback_source_changed", "The media source changed after playback was planned. Start playback again.")
		return
	}
	item.MediaFiles = s.primaryMediaFileForPlaybackContext(r.Context(), item.ID, item.SourceURL)
	binding, err := s.playbackPlanForMediaGrant(r.Context(), r, mediaID)
	if err != nil {
		writeError(w, http.StatusForbidden, "playback_plan_required", "This playback resource is not bound to a current server plan.")
		return
	}
	facts, _, err := s.mediaFactsForPlayback(r.Context(), item)
	if err != nil || !playbackSourceRevisionMatches(binding, facts.Source.Revision) {
		// Direct byte delivery must obey the same immutable source fence as HLS.
		// Otherwise a replaced file could execute under a plan that never
		// admitted its actual stream facts.
		writeError(w, http.StatusConflict, "playback_source_changed", "The media source changed after playback was planned. Start playback again.")
		return
	}
	sourceURL := strings.TrimSpace(item.SourceURL)
	if sourceURL == "" {
		sourceURL = strings.TrimSpace(s.cfg.SampleMediaURL)
	}
	if sourceURL == "" {
		writeError(w, http.StatusNotFound, "media_source_not_found", "No media source is configured for this item.")
		return
	}
	delivery := &playbackResponseWriter{ResponseWriter: w}
	if err := s.servePlaybackSource(delivery, r, item, sourceURL); err != nil && !delivery.committed {
		writePlaybackSourceError(delivery, "media_stream_failed", err)
	}
}

func playbackSourceRevisionMatches(binding playbackExecutionPlan, currentRevision string) bool {
	return strings.TrimSpace(binding.Plan.SourceRevision) != "" &&
		strings.TrimSpace(binding.Plan.SourceRevision) == strings.TrimSpace(currentRevision)
}

const maxConcurrentStreamsPerUser = 32

func (s *Server) acquireUserStreamSlot(accountID string, accountLimit int) bool {
	key := strings.TrimSpace(accountID)
	if key == "" {
		key = "anonymous"
	}
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.streamActive == nil {
		s.streamActive = map[string]int{}
	}
	limit := maxConcurrentStreamsPerUser
	if normalized := normalizeMaxActiveStreams(accountLimit); normalized > 0 && normalized < limit {
		limit = normalized
	}
	if s.streamActive[key] >= limit {
		return false
	}
	s.streamActive[key]++
	return true
}

func (s *Server) releaseUserStreamSlot(userID string) {
	key := strings.TrimSpace(userID)
	if key == "" {
		key = "anonymous"
	}
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.streamActive == nil || s.streamActive[key] <= 0 {
		return
	}
	s.streamActive[key]--
	if s.streamActive[key] == 0 {
		delete(s.streamActive, key)
	}
}

func (s *Server) handleMediaDownload(w http.ResponseWriter, r *http.Request, user User, mediaID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !user.Permissions["downloadMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to download media.")
		return
	}
	if r.Method == http.MethodGet {
		if !s.acquireUserDownloadSlot(user.ID) {
			s.downloadRejected.Add(1)
			w.Header().Set("Retry-After", "10")
			writeError(w, http.StatusTooManyRequests, "download_busy", "This account already has several downloads running. Try again shortly.")
			return
		}
		defer s.releaseUserDownloadSlot(user.ID)
	}
	item, err := s.getMediaDownloadSeedForUser(r.Context(), user, mediaID)
	if err != nil {
		writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
		return
	}
	profile := strings.TrimSpace(r.URL.Query().Get("profile"))
	source, err := s.downloadSourceForRequestContext(r.Context(), item, profile)
	if err != nil {
		writeError(w, http.StatusBadRequest, "download_unavailable", "This version is not currently available for download.")
		return
	}
	if source.versionID != "" {
		releaseArtifact, leased := s.acquireOptimizedArtifactLease(source.versionID)
		if !leased {
			writeError(w, http.StatusConflict, "optimized_stale", "This optimized version is being replaced or removed. Request the download again.")
			return
		}
		defer releaseArtifact()
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", source.filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	s.recordAudit(r, user, source.auditAction, "media", mediaID, "info", map[string]string{"title": item.Title, "profile": profile, "source": source.sourceKind})
	if source.sourceURL != "" {
		delivery := &playbackResponseWriter{ResponseWriter: w}
		if err := s.servePlaybackSource(delivery, r, item, source.sourceURL); err != nil && !delivery.committed {
			writePlaybackSourceError(delivery, "download_failed", err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Accept-Ranges", "bytes")
	delivery := &playbackResponseWriter{ResponseWriter: w}
	if err := s.serveLocalPlaybackFile(delivery, r, source.path, source.filename); err != nil && !delivery.committed {
		writePlaybackSourceError(delivery, "download_failed", err)
	}
}

func (s *Server) handleMediaDownloadOptions(w http.ResponseWriter, r *http.Request, user User, mediaID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	if !user.Permissions["playMedia"] && !user.Permissions["downloadMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view media downloads.")
		return
	}
	item, err := s.getMediaDownloadSeedForUser(r.Context(), user, mediaID)
	if err != nil {
		writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
		return
	}
	options, versions, err := s.downloadOptionsForMediaContext(r.Context(), item, user.Permissions["downloadMedia"])
	if err != nil {
		writeError(w, http.StatusInternalServerError, "download_options_failed", "Unable to load download options.")
		return
	}
	writeJSON(w, http.StatusOK, DownloadOptionsResponse{
		Media:             consumerMediaDetailProjection(item, user),
		Options:           options,
		OptimizedVersions: optimizedVersionProjection(versions, user),
		Profiles:          s.optimizedVersionProfiles(),
		DefaultProfile:    s.optimizedVersionSettings().DefaultProfile,
		CanDownload:       user.Permissions["downloadMedia"],
	})
}

const maxConcurrentDownloadsPerUser = 4

func (s *Server) acquireUserDownloadSlot(userID string) bool {
	key := strings.TrimSpace(userID)
	if key == "" {
		key = "anonymous"
	}
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	if s.downloadActive == nil {
		s.downloadActive = map[string]int{}
	}
	if s.downloadActive[key] >= maxConcurrentDownloadsPerUser {
		return false
	}
	s.downloadActive[key]++
	return true
}

func (s *Server) releaseUserDownloadSlot(userID string) {
	key := strings.TrimSpace(userID)
	if key == "" {
		key = "anonymous"
	}
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	if s.downloadActive == nil || s.downloadActive[key] <= 0 {
		return
	}
	s.downloadActive[key]--
	if s.downloadActive[key] == 0 {
		delete(s.downloadActive, key)
	}
}

func (s *Server) servePlaybackSource(w http.ResponseWriter, r *http.Request, item MediaItem, sourceURL string) error {
	if strings.HasPrefix(strings.TrimSpace(sourceURL), "portico-storage://") {
		return s.serveRemoteStorageObject(w, r, item, sourceURL)
	}
	parsed, err := url.Parse(strings.TrimSpace(sourceURL))
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return s.proxyRemotePlaybackSource(w, r, item, sourceURL)
	}
	path := sourceURL
	if err == nil && parsed.Scheme == "file" {
		path = parsed.Path
	} else if err == nil && parsed.Scheme != "" {
		return errUnsupportedPlaybackScheme
	}
	path, err = s.validateLocalMediaPath(item.LibraryID, path)
	if err != nil {
		return err
	}
	if contentType := dlnaContentType(item, path); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, no-store")
	return s.serveLocalPlaybackFile(w, r, path, filepath.Base(path))
}

// playbackStorageReadSeeker contains every potentially blocking file operation
// behind the W3 source admission/quarantine boundary. A stuck FUSE or network
// mount therefore consumes a bounded source slot, rather than an HTTP handler.
// Reads remain streaming; this wrapper never buffers more than ServeContent's
// current read buffer.
type playbackStorageReadSeeker struct {
	server *Server
	ctx    context.Context
	path   string
	file   io.ReadSeeker
	mu     sync.Mutex
	last   error
}

func (r *playbackStorageReadSeeker) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	type readResult struct {
		n   int
		err error
		buf []byte
	}
	result := make(chan readResult, 1)
	err = r.boundedOperation("read media", func(progress func()) error {
		// The filesystem call may outlive the request after quarantine. Never
		// allow such a call to retain ServeContent's reusable buffer.
		attemptBuffer := make([]byte, len(p))
		readN, readErr := r.file.Read(attemptBuffer)
		if readN > 0 {
			progress()
		}
		result <- readResult{n: readN, err: readErr, buf: attemptBuffer}
		return readErr
	})
	select {
	case completed := <-result:
		n = completed.n
		copy(p, completed.buf[:completed.n])
		// Preserve the exact Read result when the admitted operation completed.
		// The classified error only differs for a timeout/circuit outcome.
		if !errors.Is(err, errPlaybackStorageStalled) && !errors.Is(err, errPlaybackStorageOffline) && !errors.Is(err, errPlaybackStorageTransient) {
			err = completed.err
		}
	default:
	}
	if err != nil && !errors.Is(err, io.EOF) {
		r.last = err
	}
	return n, err
}

func (r *playbackStorageReadSeeker) Seek(offset int64, whence int) (position int64, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	type seekResult struct {
		position int64
		err      error
	}
	result := make(chan seekResult, 1)
	err = r.boundedOperation("seek media", func(progress func()) error {
		seekPosition, seekErr := r.file.Seek(offset, whence)
		if seekErr == nil {
			progress()
		}
		result <- seekResult{position: seekPosition, err: seekErr}
		return seekErr
	})
	select {
	case completed := <-result:
		position = completed.position
		if !errors.Is(err, errPlaybackStorageStalled) && !errors.Is(err, errPlaybackStorageOffline) && !errors.Is(err, errPlaybackStorageTransient) {
			err = completed.err
		}
	default:
	}
	if err != nil {
		r.last = err
	}
	return position, err
}

// A Read or Seek is not safely retryable: a failing filesystem can advance its
// shared file offset before returning an error. Keep one admitted attempt and
// let the client retry the HTTP range instead of risking duplicated bytes.
func (r *playbackStorageReadSeeker) boundedOperation(operation string, fn func(func()) error) error {
	request := r.server.playbackStorageRequest(r.ctx, r.path, playbackStorageDirect, operation)
	err := r.server.boundedStorageProgressIO(r.ctx, request, fn)
	return classifyPlaybackStorageError(playbackStorageDirect, operation, err)
}

func (r *playbackStorageReadSeeker) terminalError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

func (s *Server) serveLocalPlaybackFile(w http.ResponseWriter, r *http.Request, path, name string) error {
	type openResult struct {
		file *os.File
		err  error
	}
	opened := make(chan openResult, 1)
	openRequest := s.playbackStorageRequest(r.Context(), path, playbackStorageDirect, "open media")
	err := s.boundedStorageProgressIO(r.Context(), openRequest, func(progress func()) error {
		file, openErr := os.Open(path)
		if openErr == nil {
			progress()
		}
		opened <- openResult{file: file, err: openErr}
		return openErr
	})
	err = classifyPlaybackStorageError(playbackStorageDirect, "open media", err)
	if err != nil {
		// If the kernel open eventually returns after quarantine, reclaim the
		// descriptor without keeping the HTTP request alive.
		if errors.Is(err, errPlaybackStorageStalled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			go func() {
				late := <-opened
				if late.file != nil {
					_ = late.file.Close()
				}
			}()
		}
		return err
	}
	result := <-opened
	if result.err != nil {
		return result.err
	}
	file := result.file
	defer file.Close()

	type statResult struct {
		info os.FileInfo
		err  error
	}
	statted := make(chan statResult, 1)
	statRequest := s.playbackStorageRequest(r.Context(), path, playbackStorageDirect, "stat media")
	err = s.boundedStorageProgressIO(r.Context(), statRequest, func(progress func()) error {
		stat, statErr := file.Stat()
		if statErr == nil {
			progress()
		}
		statted <- statResult{info: stat, err: statErr}
		return statErr
	})
	err = classifyPlaybackStorageError(playbackStorageDirect, "stat media", err)
	if err != nil {
		return err
	}
	statResultValue := <-statted
	if statResultValue.err != nil {
		return statResultValue.err
	}
	stat := statResultValue.info
	if stat.IsDir() {
		return errUnsupportedPlaybackSource
	}

	reader := &playbackStorageReadSeeker{server: s, ctx: r.Context(), path: path, file: file}
	http.ServeContent(w, r, name, stat.ModTime(), reader)
	return reader.terminalError()
}

func (s *Server) localDownloadPath(item MediaItem) (string, error) {
	sourceURL := strings.TrimSpace(item.SourceURL)
	if sourceURL == "" {
		return "", errUnsupportedPlaybackSource
	}
	parsed, err := url.Parse(sourceURL)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return "", errUnsupportedPlaybackSource
	}
	path := sourceURL
	if err == nil && parsed.Scheme == "file" {
		path = parsed.Path
	} else if err == nil && parsed.Scheme != "" {
		return "", errUnsupportedPlaybackScheme
	}
	return s.validateLocalMediaPath(item.LibraryID, path)
}

type mediaDownloadSource struct {
	path           string
	sourceURL      string
	sourceRevision string
	versionID      string
	filename       string
	auditAction    string
	sourceKind     string
	sizeBytes      int64
}

func (s *Server) downloadPathForRequest(item MediaItem, profile string) (string, string, string, error) {
	source, err := s.downloadSourceForRequestContext(context.Background(), item, profile)
	if err != nil {
		return "", "", "", err
	}
	if source.path == "" {
		return "", "", "", errUnsupportedPlaybackSource
	}
	return source.path, source.filename, source.auditAction, nil
}

func (s *Server) downloadSourceForRequest(item MediaItem, profile string) (mediaDownloadSource, error) {
	return s.downloadSourceForRequestContext(context.Background(), item, profile)
}

func (s *Server) downloadSourceForRequestContext(ctx context.Context, item MediaItem, profile string) (mediaDownloadSource, error) {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == "" || profile == "source" || profile == "original" {
		sourceURL := strings.TrimSpace(item.SourceURL)
		if sourceURL == "" {
			return mediaDownloadSource{}, errUnsupportedPlaybackSource
		}
		parsed, err := url.Parse(sourceURL)
		if err == nil && parsed.Scheme == "file" {
			path, err := s.localDownloadPath(item)
			if err != nil {
				return mediaDownloadSource{}, err
			}
			return mediaDownloadSource{path: path, filename: safeDownloadFilename(item.Title, filepath.Ext(path)), auditAction: "media.downloaded", sourceKind: "local", sizeBytes: downloadSizeFromItem(item)}, nil
		}
		if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
			if _, err := validateExternalURL(sourceURL); err != nil {
				return mediaDownloadSource{}, err
			}
			return mediaDownloadSource{sourceURL: sourceURL, filename: safeDownloadFilename(item.Title, downloadExtensionFromItem(item, sourceURL)), auditAction: "media.downloaded", sourceKind: "remote", sizeBytes: downloadSizeFromItem(item)}, nil
		}
		if err == nil && parsed.Scheme == "portico-storage" {
			sourceID, objectPath, parseErr := parseRemoteStorageLocator(sourceURL)
			if parseErr != nil {
				return mediaDownloadSource{}, errUnsupportedPlaybackSource
			}
			var size int64
			var revision string
			if queryErr := s.queryUserRow(ctx, `
				SELECT object.size_bytes, object.revision
				FROM storage_remote_objects object
				JOIN storage_sources source ON source.id = object.source_id
				WHERE object.source_id = ? AND object.object_path = ?
					AND object.missing_since = '' AND source.library_id = ?`,
				sourceID, objectPath, item.LibraryID).Scan(&size, &revision); queryErr != nil {
				return mediaDownloadSource{}, errUnsupportedPlaybackSource
			}
			return mediaDownloadSource{
				sourceURL: sourceURL, sourceRevision: revision,
				filename:    safeDownloadFilename(item.Title, downloadExtensionFromItem(item, objectPath)),
				auditAction: "media.downloaded", sourceKind: "remote-storage", sizeBytes: size,
			}, nil
		}
		if err == nil && parsed.Scheme != "" {
			return mediaDownloadSource{}, errUnsupportedPlaybackScheme
		}
		path, err := s.localDownloadPath(item)
		if err != nil {
			return mediaDownloadSource{}, err
		}
		return mediaDownloadSource{path: path, filename: safeDownloadFilename(item.Title, filepath.Ext(path)), auditAction: "media.downloaded", sourceKind: "local", sizeBytes: downloadSizeFromItem(item)}, nil
	}
	if profile == "default" || profile == "optimized" {
		profile = s.optimizedVersionSettings().DefaultProfile
	}
	preset, ok := optimized.Lookup(profile)
	if !ok {
		return mediaDownloadSource{}, errors.New("optimized version is not available for that profile")
	}
	item.Streams, _ = s.listStreamsContext(ctx, item.ID)
	item.MediaFiles = s.primaryMediaFileForPlaybackContext(ctx, item.ID, item.SourceURL)
	facts, digest, err := s.mediaFactsForPlayback(ctx, item)
	if err != nil {
		return mediaDownloadSource{}, errors.New("optimized version is not available for the current source")
	}
	source, err := optimizedSourceIdentityFromFacts(facts, digest)
	if err != nil {
		return mediaDownloadSource{}, errors.New("optimized version is not available for the current source")
	}
	artifact, err := s.optimizedV2ReadyForSource(ctx, item.ID, preset.ID, source, func(path string, size int64) bool {
		return s.optimizedArtifactUsable(ctx, path, size)
	})
	if err != nil || artifact == nil {
		return mediaDownloadSource{}, errors.New("optimized version is not available for the current source")
	}
	clean := filepath.Clean(artifact.Path)
	extension := filepath.Ext(clean)
	if extension == "" {
		extension = preset.Artifact.Extension
	}
	return mediaDownloadSource{path: clean, versionID: artifact.ID, filename: safeDownloadFilename(item.Title+"-"+profile, extension), auditAction: "media.optimized_downloaded", sourceKind: "optimized", sizeBytes: artifact.SizeBytes}, nil
}

func (s *Server) downloadOptionsForMedia(item MediaItem, canDownload bool) ([]DownloadOption, []OptimizedVersion, error) {
	return s.downloadOptionsForMediaContext(context.Background(), item, canDownload)
}

func (s *Server) downloadOptionsForMediaContext(ctx context.Context, item MediaItem, canDownload bool) ([]DownloadOption, []OptimizedVersion, error) {
	versions, err := s.optimizedVersionsForMediaContext(ctx, item.ID)
	if err != nil {
		return nil, nil, err
	}
	versionByProfile := map[string]OptimizedVersion{}
	for _, version := range versions {
		versionByProfile[version.Profile] = version
	}
	options := []DownloadOption{}
	sourceKind := downloadSourceKind(item.SourceURL)
	original := DownloadOption{
		ID:          "source",
		Kind:        "source",
		Profile:     "source",
		Label:       "Original source",
		Description: "Download the selected source file without conversion.",
		Available:   canDownload,
		URL:         "",
		SizeBytes:   downloadSizeFromItem(item),
		SourceKind:  sourceKind,
	}
	if file := preferredDownloadFile(item); file != nil {
		original.Container = file.Container
		original.VideoCodec = file.VideoCodec
		original.AudioCodec = file.AudioCodec
		if file.SizeBytes > 0 {
			original.SizeBytes = file.SizeBytes
		}
	}
	if _, err := s.downloadSourceForRequestContext(ctx, item, "source"); err != nil {
		original.Available = false
		original.Description = "Original source is not currently available for download."
		original.URL = ""
	}
	options = append(options, original)

	activeJobs := s.activeOptimizedVersionJobsContext(ctx, item.ID)
	for _, profile := range s.optimizedVersionProfiles() {
		version, available := versionByProfile[profile.ID]
		preset, _ := optimized.Lookup(profile.ID)
		description := fmt.Sprintf("%s optimized artifact at source dimensions.", strings.ToUpper(string(preset.VideoCodec)))
		if profile.Height > 0 {
			description = fmt.Sprintf("%s optimized artifact for up to %dp playback.", strings.ToUpper(string(preset.VideoCodec)), profile.Height)
		}
		option := DownloadOption{
			ID:                       "optimized-" + profile.ID,
			Kind:                     "optimized",
			Profile:                  profile.ID,
			Label:                    profile.Label,
			Description:              description,
			Available:                canDownload && available,
			RequiresOptimizedVersion: !available,
			Container:                string(preset.Container),
			VideoCodec:               string(preset.VideoCodec),
			SourceKind:               "optimized",
		}
		if available {
			option.SizeBytes = version.SizeBytes
			option.Container = version.Container
			option.VideoCodec = version.VideoCodec
			option.AudioCodec = version.AudioCodec
		}
		if job := activeJobs[profile.ID]; job != nil {
			option.Job = job
		}
		options = append(options, option)
	}
	return options, versions, nil
}

func (s *Server) activeOptimizedVersionJobs(mediaID string) map[string]*Job {
	return s.activeOptimizedVersionJobsContext(context.Background(), mediaID)
}

func (s *Server) activeOptimizedVersionJobsContext(ctx context.Context, mediaID string) map[string]*Job {
	rows, err := s.queryUserRead(ctx, `
		SELECT id, type, status, progress, message, resource_type, resource_id, COALESCE(metadata_json, '{}'), created_at, updated_at
		FROM jobs
		WHERE type = 'optimize_version'
			AND resource_type = 'media'
			AND resource_id = ?
			AND status IN ('queued', 'running')
		ORDER BY created_at DESC`, mediaID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	jobs := map[string]*Job{}
	for rows.Next() {
		var job Job
		var metadataJSON string
		if err := rows.Scan(&job.ID, &job.Type, &job.Status, &job.Progress, &job.Message, &job.ResourceType, &job.ResourceID, &metadataJSON, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return jobs
		}
		job.Metadata = decodeJobMetadata(metadataJSON)
		profile := optimizedProfileFromJob(job, s.optimizedVersionSettings().DefaultProfile)
		copied := job
		jobs[profile] = &copied
	}
	return jobs
}

func preferredDownloadFile(item MediaItem) *MediaFileVersion {
	for i := range item.MediaFiles {
		if item.MediaFiles[i].Selected && item.MediaFiles[i].Available {
			return &item.MediaFiles[i]
		}
	}
	for i := range item.MediaFiles {
		if item.MediaFiles[i].Available {
			return &item.MediaFiles[i]
		}
	}
	if len(item.MediaFiles) == 0 {
		return nil
	}
	return &item.MediaFiles[0]
}

func downloadSizeFromItem(item MediaItem) int64 {
	if file := preferredDownloadFile(item); file != nil {
		return file.SizeBytes
	}
	return 0
}

func downloadExtensionFromItem(item MediaItem, sourceURL string) string {
	if file := preferredDownloadFile(item); file != nil {
		if ext := filepath.Ext(file.Path); ext != "" {
			return ext
		}
		if file.Container != "" {
			return "." + file.Container
		}
	}
	if parsed, err := url.Parse(sourceURL); err == nil {
		if ext := filepath.Ext(parsed.Path); ext != "" {
			return ext
		}
	}
	return ".bin"
}

func downloadSourceKind(sourceURL string) string {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return "unknown"
	}
	if parsed, err := url.Parse(sourceURL); err == nil {
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			return "remote"
		}
		if parsed.Scheme == "file" || parsed.Scheme == "" {
			return "local"
		}
		return parsed.Scheme
	}
	return "local"
}

func safeDownloadFilename(title string, ext string) string {
	name := safePathComponent(title)
	if name == "" {
		name = "media"
	}
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" || strings.ContainsAny(ext, `/\`) {
		ext = ".bin"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return name + ext
}

func (s *Server) proxyRemotePlaybackSource(w http.ResponseWriter, r *http.Request, item MediaItem, sourceURL string) error {
	parsed, err := validateExternalURL(sourceURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, parsed.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", playbackUserAgent(r))
	for _, key := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := r.Header.Get(key); value != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := liveTVHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	copyPlaybackHeaders(w.Header(), resp.Header)
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", dlnaContentType(item, sourceURL))
	}
	if w.Header().Get("Accept-Ranges") == "" {
		w.Header().Set("Accept-Ranges", "bytes")
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		if err := copyRemotePlaybackBody(r.Context(), w, resp.Body); err != nil && r.Context().Err() == nil {
			s.recordLog("warn", "Remote playback proxy copy failed", map[string]string{"mediaId": item.ID, "error": err.Error()})
			return err
		}
	}
	return nil
}

// copyRemotePlaybackBody applies a no-progress watchdog to the response body,
// not a total transfer deadline. Large remote files may stream indefinitely as
// long as bytes continue to arrive. Closing the upstream body interrupts the
// net/http transport's blocked Read while a response write deadline prevents a
// disconnected or non-reading client from pinning the proxy in Write.
func copyRemotePlaybackBody(ctx context.Context, dst http.ResponseWriter, src io.ReadCloser) error {
	const bufferSize = 64 * 1024
	quietWindow := storageIOOperationTimeout(storageSourceNetwork)
	if quietWindow <= 0 {
		quietWindow = 20 * time.Second
	}
	progress := make(chan struct{}, 1)
	done := make(chan struct{})
	watchdogExpired := make(chan struct{})
	var expireOnce sync.Once
	go func() {
		timer := time.NewTimer(quietWindow)
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
				timer.Reset(quietWindow)
			case <-timer.C:
				expireOnce.Do(func() { close(watchdogExpired) })
				_ = src.Close()
				return
			case <-ctx.Done():
				_ = src.Close()
				return
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	controller := http.NewResponseController(dst)
	buffer := make([]byte, bufferSize)
	for {
		n, readErr := src.Read(buffer)
		if n > 0 {
			select {
			case progress <- struct{}{}:
			default:
			}
			_ = controller.SetWriteDeadline(time.Now().Add(quietWindow))
			written, writeErr := dst.Write(buffer[:n])
			_ = controller.SetWriteDeadline(time.Time{})
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			select {
			case <-watchdogExpired:
				return &playbackStorageError{Kind: playbackStorageErrorStalled, Consumer: playbackStorageDirect, Operation: "read remote media", Cause: errStorageIOStalled}
			default:
			}
			return readErr
		}
		if n == 0 {
			select {
			case <-watchdogExpired:
				return &playbackStorageError{Kind: playbackStorageErrorStalled, Consumer: playbackStorageDirect, Operation: "read remote media", Cause: errStorageIOStalled}
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
	}
}

func writePlaybackSourceError(w http.ResponseWriter, fallbackCode string, err error) {
	code := fallbackCode
	status := http.StatusBadGateway
	message := "Playback could not start because the server could not read this media source."
	var sourceErr playbackSourceError
	if errors.As(err, &sourceErr) {
		switch sourceErr {
		case errUnsupportedPlaybackScheme:
			code = "unsupported_source_scheme"
			status = http.StatusBadRequest
			message = "This media source type is not supported for playback."
		case errUnsupportedPlaybackSource:
			code = "unsupported_source"
			status = http.StatusBadRequest
			message = "This media source is not playable."
		case errPlaybackPathNotAllowed:
			code = "source_path_not_allowed"
			status = http.StatusForbidden
			message = "This media source is outside the configured library folders."
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		code = "media_file_missing"
		status = http.StatusNotFound
		message = "This file is missing on the server. Check that the drive is connected, then rescan the library."
	}
	if errors.Is(err, errPlaybackStorageOffline) {
		code = "media_storage_offline"
		status = http.StatusServiceUnavailable
		message = "This media source is currently offline. Check that its drive or network mount is available."
	} else if errors.Is(err, errPlaybackStorageStalled) || errors.Is(err, errPlaybackStorageTransient) {
		code = "media_storage_unavailable"
		status = http.StatusServiceUnavailable
		message = "This media source is temporarily unavailable. Try again shortly."
	}
	writeError(w, status, code, message)
}

func playbackUserAgent(r *http.Request) string {
	if userAgent := strings.TrimSpace(r.Header.Get("User-Agent")); userAgent != "" {
		return userAgent
	}
	return "Portico/0.1 Playback"
}

func copyPlaybackHeaders(dst http.Header, src http.Header) {
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

var (
	errUnsupportedPlaybackScheme      = playbackSourceError("playback source scheme is not supported")
	errUnsupportedPlaybackSource      = playbackSourceError("playback source is not a playable file")
	errPlaybackPathNotAllowed         = playbackSourceError("playback source is outside the configured library folders")
	errLogicalDiscPlaybackUnsupported = playbackSourceError(logicalDiscPlaybackUnsupportedReason)
)

type playbackSourceError string

func (err playbackSourceError) Error() string {
	return string(err)
}

func (s *Server) validateLocalMediaPath(libraryID, path string) (string, error) {
	if strings.TrimSpace(libraryID) == "" || strings.TrimSpace(path) == "" {
		return "", errPlaybackPathNotAllowed
	}
	library, err := s.getLibrary(libraryID)
	if err != nil {
		return "", errPlaybackPathNotAllowed
	}
	roots, err := resolvedLibraryRoots(library.Paths)
	if err != nil || len(roots) == 0 {
		return "", errPlaybackPathNotAllowed
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errPlaybackPathNotAllowed
	}
	realPath, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	for _, root := range roots {
		if pathWithinRoot(realPath, root.real) {
			return filepath.Clean(realPath), nil
		}
	}
	return "", errPlaybackPathNotAllowed
}
