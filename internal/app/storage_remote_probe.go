package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const remotePlaybackProbeTimeout = 45 * time.Second

const (
	remotePlaybackProbeMaxRequests = 64
	remotePlaybackProbeMaxBytes    = 128 << 20
)

type remotePlaybackProbeLockEntry struct {
	mu   sync.Mutex
	refs int
}

var remotePlaybackProbeLocks = struct {
	sync.Mutex
	entries map[string]*remotePlaybackProbeLockEntry
}{entries: map[string]*remotePlaybackProbeLockEntry{}}

// acquireRemotePlaybackProbeLock prevents duplicate first-play provider reads
// without retaining one mutex for every file ever played. The reference count
// is incremented before waiting, so an entry is deleted only when no holder or
// waiter can still observe it.
func acquireRemotePlaybackProbeLock(key string) func() {
	remotePlaybackProbeLocks.Lock()
	entry := remotePlaybackProbeLocks.entries[key]
	if entry == nil {
		entry = &remotePlaybackProbeLockEntry{}
		remotePlaybackProbeLocks.entries[key] = entry
	}
	entry.refs++
	remotePlaybackProbeLocks.Unlock()

	entry.mu.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			entry.mu.Unlock()
			remotePlaybackProbeLocks.Lock()
			entry.refs--
			if entry.refs == 0 && remotePlaybackProbeLocks.entries[key] == entry {
				delete(remotePlaybackProbeLocks.entries, key)
			}
			remotePlaybackProbeLocks.Unlock()
		})
	}
}

// probeRemotePlaybackFacts gives ffprobe a short-lived, loopback-only HTTP
// origin backed by authenticated provider range reads. This keeps provider
// credentials and URLs out of child-process arguments while still allowing
// ffprobe to seek to tail indexes in MP4/MKV objects.
func (s *Server) probeRemotePlaybackFacts(ctx context.Context, item MediaItem, locator string) error {
	return s.analyzeRemoteMediaFacts(ctx, item, locator, mediaAnalysisOptions{Mode: mediaAnalysisModeProbe, ProbeStreams: true, DetectChapterSegments: true})
}

func (s *Server) analyzeRemoteMediaFacts(ctx context.Context, item MediaItem, locator string, options mediaAnalysisOptions) error {
	ffprobePath := strings.TrimSpace(s.cfg.FFprobePath)
	if ffprobePath == "" {
		return errors.New("ffprobe is not configured")
	}
	if filepath.Base(ffprobePath) == ffprobePath {
		resolved, err := exec.LookPath(ffprobePath)
		if err != nil {
			return errors.New("ffprobe is not available on PATH")
		}
		ffprobePath = resolved
	}
	sourceID, objectPath, err := parseRemoteStorageLocator(locator)
	if err != nil {
		return err
	}
	var size int64
	var sourceRevision string
	if err := s.queryBackgroundRow(ctx, `SELECT object.size_bytes,object.revision FROM storage_remote_objects object JOIN storage_sources source ON source.id=object.source_id WHERE object.source_id=? AND object.object_path=? AND object.missing_since='' AND source.library_id=?`, sourceID, objectPath, item.LibraryID).Scan(&size, &sourceRevision); err != nil {
		return err
	}
	if size <= 0 {
		return errors.New("remote storage object is empty")
	}
	backend, err := s.remoteBackendForSource(ctx, sourceID)
	if err != nil {
		return err
	}
	var budget *remoteProbeBudget
	if options.Mode != mediaAnalysisModeFull {
		budget = &remoteProbeBudget{bytesRemaining: remotePlaybackProbeMaxBytes, requestsRemaining: remotePlaybackProbeMaxRequests}
	}
	proxyCtx, probeURL, closeProxy, err := startRemoteObjectProxy(ctx, backend, objectPath, size, budget)
	if err != nil {
		return err
	}
	defer closeProxy()
	demuxer, err := remoteMediaDemuxer(objectPath)
	if err != nil {
		return err
	}

	probeCtx, cancel := context.WithTimeout(proxyCtx, remotePlaybackProbeTimeout)
	defer cancel()
	result, err := runBoundedAnalysisCommand(probeCtx, ffprobePath, []string{
		"-v", "error",
		"-protocol_whitelist", "http,tcp",
		"-rw_timeout", "15000000",
		"-f", demuxer,
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_chapters",
		probeURL,
	}, "", 8<<20, 1<<20)
	if err != nil {
		return err
	}
	var payload ffprobePayload
	if err := json.Unmarshal(result.Stdout, &payload); err != nil {
		return err
	}
	if len(payload.Streams) == 0 {
		return errors.New("remote media probe did not find playable streams")
	}
	if options.Mode == mediaAnalysisModeFull {
		providerBefore, statErr := statRemoteStorageObject(ctx, backend, objectPath)
		if statErr != nil {
			return statErr
		}
		if providerBefore.Revision != sourceRevision || providerBefore.Size != size {
			return errors.New("remote storage object changed before Complete analysis staging")
		}
		localPath, cleanup, materializeErr := s.materializeRemoteObjectForCompleteAnalysis(ctx, backend, objectPath, providerBefore.Size)
		if materializeErr != nil {
			return materializeErr
		}
		defer cleanup()
		providerAfter, statErr := statRemoteStorageObject(ctx, backend, objectPath)
		if statErr != nil {
			return statErr
		}
		if providerAfter.Revision != providerBefore.Revision || providerAfter.Size != providerBefore.Size {
			return errors.New("remote storage object changed during Complete analysis staging")
		}
		exactSeekSafe, keyframeEvidenceAt := s.probeExactSeekEvidence(ctx, localPath, payload)
		return s.persistFFprobeAnalysisInputs(ctx, item, locator, localPath, payload, options, exactSeekSafe, keyframeEvidenceAt)
	}
	// First-play probing populates only authoritative stream/container facts.
	// Expensive full-file keyframe, loudness, image, and trickplay work remains
	// explicitly deferred for remote sources.
	if err := s.persistFFprobeAnalysis(ctx, item, locator, payload, mediaAnalysisOptions{
		Mode: options.Mode, ProbeStreams: true, ReadEmbeddedTags: options.ReadEmbeddedTags,
		ReadEmbeddedIndexes: options.ReadEmbeddedIndexes, DetectChapterSegments: options.DetectChapterSegments,
	}, false, ""); err != nil {
		return err
	}
	if options.GenerateThumbnails && payloadHasVideoStream(payload) && s.mediaNeedsRepresentativeFrameContext(ctx, item) {
		if err := s.generateRemoteMediaThumbnail(ctx, item, probeURL, demuxer); err != nil {
			return err
		}
	}
	return nil
}

func statRemoteStorageObject(ctx context.Context, backend remoteStorageBackend, objectPath string) (storageObject, error) {
	statter, ok := backend.(remoteStorageObjectStatter)
	if !ok {
		return storageObject{}, errors.New("remote storage backend cannot verify object revisions for Complete analysis")
	}
	object, err := statter.Stat(withRemoteStorageBackgroundRead(ctx), objectPath)
	if err != nil {
		return storageObject{}, err
	}
	if strings.TrimSpace(object.Revision) == "" || object.Size <= 0 {
		return storageObject{}, errors.New("remote storage provider returned an invalid object revision")
	}
	if strings.TrimSpace(object.ObjectID) == "" && strings.TrimSpace(object.ETag) == "" && strings.TrimSpace(object.Hash) == "" && object.ModTime.IsZero() {
		return storageObject{}, errors.New("remote storage provider did not return a stable object validator for Complete analysis")
	}
	return object, nil
}

func (s *Server) materializeRemoteObjectForCompleteAnalysis(ctx context.Context, backend remoteStorageBackend, objectPath string, size int64) (string, func(), error) {
	if size <= 0 {
		return "", nil, errors.New("remote storage object is empty")
	}
	stagingDir := filepath.Join(s.cfg.AppDataDir, "analysis-staging")
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		return "", nil, err
	}
	s.cleanupStaleRemoteAnalysisStaging(stagingDir, 24*time.Hour)
	releaseReservation, err := s.mediaResourceGovernor().reserveMediaDisk(stagingDir, size, mediaWriteMinimumFreeBytes)
	if err != nil {
		return "", nil, err
	}
	extension := strings.ToLower(filepath.Ext(objectPath))
	file, err := os.CreateTemp(stagingDir, "remote-complete-*"+extension)
	if err != nil {
		releaseReservation()
		return "", nil, err
	}
	path := file.Name()
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			_ = os.Remove(path)
			releaseReservation()
		})
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	const chunkSize int64 = 8 << 20
	for offset := int64(0); offset < size; offset += chunkSize {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			cleanup()
			return "", nil, analysisContextError(ctx, err)
		}
		length := chunkSize
		if remaining := size - offset; remaining < length {
			length = remaining
		}
		reader, openErr := backend.OpenRange(ctx, objectPath, offset, length)
		if openErr != nil {
			_ = file.Close()
			cleanup()
			return "", nil, openErr
		}
		written, copyErr := io.Copy(file, io.LimitReader(reader, length))
		closeErr := reader.Close()
		if copyErr != nil || closeErr != nil || written != length {
			_ = file.Close()
			cleanup()
			if copyErr != nil {
				return "", nil, copyErr
			}
			if closeErr != nil {
				return "", nil, closeErr
			}
			return "", nil, io.ErrUnexpectedEOF
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

func (s *Server) cleanupStaleRemoteAnalysisStaging(directory string, maxAge time.Duration) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "remote-complete-") {
			continue
		}
		info, statErr := entry.Info()
		if statErr == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(directory, entry.Name()))
		}
	}
}

func (s *Server) generateRemoteMediaThumbnail(ctx context.Context, item MediaItem, inputURL, demuxer string) error {
	ffmpegPath := strings.TrimSpace(s.cfg.FFmpegPath)
	if filepath.Base(ffmpegPath) == ffmpegPath {
		resolved, err := exec.LookPath(ffmpegPath)
		if err != nil {
			return errors.New("ffmpeg is not available on PATH")
		}
		ffmpegPath = resolved
	}
	outputDir := filepath.Join(s.cfg.AppDataDir, "artwork", safePathComponent(item.ID))
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return err
	}
	outputPath := filepath.Join(outputDir, "thumb.jpg")
	tempPath := outputPath + ".tmp"
	_ = os.Remove(tempPath)
	thumbnailCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	seek := strconv.FormatFloat(representativeThumbnailSecond(item.DurationSeconds), 'f', 3, 64)
	result, err := runBoundedAnalysisCommand(thumbnailCtx, ffmpegPath, []string{
		"-hide_banner", "-nostdin", "-y", "-threads", "1", "-filter_threads", "1",
		"-protocol_whitelist", "http,tcp", "-rw_timeout", "15000000", "-ss", seek,
		"-f", demuxer, "-i", inputURL, "-frames:v", "1", "-vf", "scale=640:-2", "-q:v", "4", tempPath,
	}, "", 1<<20, 4<<20)
	_ = result
	if err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, outputPath); err != nil {
		return err
	}
	if err := os.Chmod(outputPath, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "thumb.version"), []byte(mediaThumbnailVersion+"\n"), 0o600); err != nil {
		return err
	}
	_, err = s.execBackgroundWriteTagged(ctx, []string{"media", "metadata", "library-items", "artwork"}, `UPDATE media_items SET art_seed=art_seed+1 WHERE id=?`, item.ID)
	return err
}

func remoteMediaDemuxer(objectPath string) (string, error) {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(objectPath))) {
	case ".mp4", ".m4v", ".mov", ".m4a", ".3gp", ".3g2":
		return "mov", nil
	case ".mkv", ".webm":
		return "matroska", nil
	case ".avi":
		return "avi", nil
	case ".ts", ".mts", ".m2ts":
		return "mpegts", nil
	case ".mpg", ".mpeg", ".vob":
		return "mpeg", nil
	case ".mp3":
		return "mp3", nil
	case ".flac":
		return "flac", nil
	case ".ogg", ".oga", ".opus":
		return "ogg", nil
	case ".wav":
		return "wav", nil
	case ".aac":
		return "aac", nil
	case ".ac3":
		return "ac3", nil
	case ".eac3":
		return "eac3", nil
	case ".dts":
		return "dts", nil
	default:
		return "", errors.New("remote media format is not approved for decoder analysis")
	}
}

func startRemoteProbeProxy(ctx context.Context, backend remoteStorageBackend, objectPath string, size int64) (string, func(), error) {
	budget := &remoteProbeBudget{bytesRemaining: remotePlaybackProbeMaxBytes, requestsRemaining: remotePlaybackProbeMaxRequests}
	_, probeURL, closeProxy, err := startRemoteObjectProxy(ctx, backend, objectPath, size, budget)
	return probeURL, closeProxy, err
}

func (s *Server) startRemoteStoragePlaybackTransport(ctx context.Context, item MediaItem, locator string) (string, func(), error) {
	sourceID, objectPath, err := parseRemoteStorageLocator(locator)
	if err != nil {
		return "", nil, err
	}
	var size int64
	if err := s.queryBackgroundRow(ctx, `SELECT object.size_bytes FROM storage_remote_objects object JOIN storage_sources source ON source.id=object.source_id WHERE object.source_id=? AND object.object_path=? AND object.missing_since='' AND source.library_id=?`, sourceID, objectPath, item.LibraryID).Scan(&size); err != nil {
		return "", nil, err
	}
	backend, err := s.remoteBackendForSource(ctx, sourceID)
	if err != nil {
		return "", nil, err
	}
	_, playbackURL, closeProxy, err := startRemoteObjectProxy(ctx, backend, objectPath, size, nil)
	return playbackURL, closeProxy, err
}

func startRemoteObjectProxy(ctx context.Context, backend remoteStorageBackend, objectPath string, size int64, budget *remoteProbeBudget) (context.Context, string, func(), error) {
	if backend == nil || size <= 0 {
		return nil, "", nil, errors.New("invalid remote probe source")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, "", nil, err
	}
	proxyCtx, cancelProxy := context.WithCancelCause(ctx)
	token := randomID("probe")
	path := "/" + token
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peerHost, _, peerErr := net.SplitHostPort(r.RemoteAddr)
		peer := net.ParseIP(peerHost)
		if peerErr != nil || peer == nil || !peer.IsLoopback() || r.URL.Path != path || r.URL.RawQuery != "" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		if budget != nil && !budget.beginRequest() {
			http.Error(w, "remote probe budget exhausted", http.StatusTooManyRequests)
			return
		}
		start, end, partial, rangeErr := parseStorageHTTPRange(r.Header.Get("Range"), size)
		if rangeErr != nil {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		length := end - start + 1
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		if partial {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		}
		if r.Method == http.MethodHead {
			if partial {
				w.WriteHeader(http.StatusPartialContent)
			}
			return
		}
		reader, openErr := backend.OpenRange(r.Context(), objectPath, start, length)
		if openErr != nil {
			if errors.Is(openErr, errRemoteStoragePreempted) {
				cancelProxy(errRemoteStoragePreempted)
			}
			http.Error(w, "remote source unavailable", http.StatusBadGateway)
			return
		}
		if partial {
			w.WriteHeader(http.StatusPartialContent)
		}
		readSource := io.Reader(reader)
		if budget != nil {
			readSource = &remoteProbeBudgetReader{source: reader, budget: budget}
		}
		limited := &io.LimitedReader{R: readSource, N: length}
		_, copyErr := io.Copy(w, limited)
		closeErr := reader.Close()
		if errors.Is(copyErr, errRemoteStoragePreempted) || errors.Is(closeErr, errRemoteStoragePreempted) {
			cancelProxy(errRemoteStoragePreempted)
		}
		if copyErr != nil || closeErr != nil || limited.N != 0 {
			// Headers may already be committed. ErrAbortHandler terminates the
			// loopback response instead of disguising a truncated provider range
			// as a successful media read.
			panic(http.ErrAbortHandler)
		}
	})
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       10 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return proxyCtx
		},
	}
	go func() {
		_ = server.Serve(listener)
	}()
	var once sync.Once
	closeServer := func() {
		once.Do(func() {
			cancelProxy(context.Canceled)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
			_ = listener.Close()
		})
	}
	return proxyCtx, "http://" + listener.Addr().String() + path, closeServer, nil
}

type remoteProbeBudget struct {
	mu                sync.Mutex
	bytesRemaining    int64
	requestsRemaining int
}

func (b *remoteProbeBudget) beginRequest() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.requestsRemaining <= 0 || b.bytesRemaining <= 0 {
		return false
	}
	b.requestsRemaining--
	return true
}

type remoteProbeBudgetReader struct {
	source io.Reader
	budget *remoteProbeBudget
}

func (r *remoteProbeBudgetReader) Read(buffer []byte) (int, error) {
	r.budget.mu.Lock()
	defer r.budget.mu.Unlock()
	if r.budget.bytesRemaining <= 0 {
		return 0, errors.New("remote probe byte budget exhausted")
	}
	if int64(len(buffer)) > r.budget.bytesRemaining {
		buffer = buffer[:r.budget.bytesRemaining]
	}
	n, err := r.source.Read(buffer)
	r.budget.bytesRemaining -= int64(n)
	return n, err
}
