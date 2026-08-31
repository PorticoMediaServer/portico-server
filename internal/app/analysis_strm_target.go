package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

const (
	strmTargetProbeTimeout     = 45 * time.Second
	strmTargetRequestTimeout   = 20 * time.Second
	strmTargetMaxRequests      = 32
	strmTargetMaxResponseBytes = 64 << 20
)

var (
	errSTRMTargetAnalysisDisabled = errors.New("STRM target analysis is no longer authorized")
	errSTRMDescriptorStale        = errors.New("STRM descriptor changed during analysis")
)

type strmAnalysisSourceRecord struct {
	FileID         string
	Path           string
	SourceRevision string
	SizeBytes      int64
	ModTime        string
}

// strmAnalysisSource identifies the already-inventoried descriptor without
// opening it. File List Only and Basic therefore stop here with zero descriptor
// reads, while Complete or an explicit Custom operation can continue through
// the separately authorized target boundary.
func (s *Server) strmAnalysisSource(ctx context.Context, item MediaItem) (strmAnalysisSourceRecord, bool) {
	files := item.MediaFiles
	if len(files) == 0 {
		files = s.primaryMediaFileForPlaybackContext(ctx, item.ID, item.SourceURL)
	}
	if len(files) != 1 || !files[0].Available || (!strings.EqualFold(files[0].SourceType, "strm") && !isSTRMDescriptor(files[0].Path)) {
		return strmAnalysisSourceRecord{}, false
	}
	file := files[0]
	var fingerprint, quickSignature string
	if err := s.queryBackgroundRow(ctx, `SELECT content_fingerprint,
		CASE WHEN identity_evidence LIKE 'scanner:v2:%' THEN substr(identity_evidence,12) ELSE '' END
		FROM media_files WHERE id=? AND media_id=? AND path=? AND available=1`, file.ID, item.ID, file.Path).Scan(&fingerprint, &quickSignature); err != nil {
		return strmAnalysisSourceRecord{}, false
	}
	// A STRM descriptor intentionally has no content fingerprint. Its revision
	// is the scanner's stable local identity/stat revision, never a hash of the
	// descriptor bytes or target media.
	revision := scannerAnalysisSourceRevision(scannerMediaFile{
		ID: item.ID, FileID: file.ID, SourcePath: file.Path, QuickSignature: quickSignature,
		FileSize: file.SizeBytes, FileModTime: file.ModTime,
	})
	return strmAnalysisSourceRecord{FileID: file.ID, Path: file.Path, SourceRevision: revision, SizeBytes: file.SizeBytes, ModTime: file.ModTime}, true
}

func (s *Server) analyzeSTRMTarget(ctx context.Context, item MediaItem, source strmAnalysisSourceRecord, options mediaAnalysisOptions) error {
	if !options.AnalyzeSTRMTarget {
		return nil
	}
	if expected := strings.TrimSpace(options.ExpectedSourceRevision); expected == "" || expected != source.SourceRevision {
		return errSTRMDescriptorStale
	}
	if err := s.validateSTRMDescriptorFile(ctx, source); err != nil {
		return err
	}
	locator, err := s.readSTRMLocatorForWork(ctx, source.Path, foundationcontract.WorkClassBackgroundMedia)
	if err != nil {
		return errors.New("STRM target analysis could not read a valid descriptor")
	}
	parsed, err := url.Parse(locator)
	if err != nil {
		return errors.New("STRM target analysis could not resolve the target format")
	}
	demuxer, err := remoteMediaDemuxer(parsed.Path)
	if err != nil {
		return err
	}
	client, err := strmTargetHTTPClient(ctx, locator)
	if err != nil {
		return errors.New("STRM target analysis rejected the target origin")
	}
	client.Timeout = strmTargetRequestTimeout
	return s.analyzeSTRMTargetResolved(ctx, item, source, options, locator, demuxer, client)
}

func (s *Server) analyzeSTRMTargetResolved(ctx context.Context, item MediaItem, source strmAnalysisSourceRecord, options mediaAnalysisOptions, locator, demuxer string, client *http.Client) error {
	probeCtx, cancel := context.WithTimeout(ctx, strmTargetProbeTimeout)
	defer cancel()
	proxyURL, closeProxy, err := startSTRMTargetProxy(probeCtx, locator, client)
	if err != nil {
		return errors.New("STRM target analysis could not start its private proxy")
	}
	defer closeProxy()

	ffprobePath := strings.TrimSpace(s.cfg.FFprobePath)
	if ffprobePath == "" {
		return errors.New("ffprobe is not configured")
	}
	if filepath.Base(ffprobePath) == ffprobePath {
		resolved, lookupErr := exec.LookPath(ffprobePath)
		if lookupErr != nil {
			return errors.New("ffprobe is not available on PATH")
		}
		ffprobePath = resolved
	}
	result, err := runBoundedAnalysisCommand(probeCtx, ffprobePath, []string{
		"-v", "error",
		"-protocol_whitelist", "http,tcp",
		"-rw_timeout", "15000000",
		"-f", demuxer,
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		proxyURL,
	}, "", 8<<20, 1<<20)
	if err != nil {
		if probeCtx.Err() != nil {
			return analysisContextError(probeCtx, probeCtx.Err())
		}
		return errors.New("STRM target stream probing failed")
	}
	var payload ffprobePayload
	if err := json.Unmarshal(result.Stdout, &payload); err != nil || len(payload.Streams) == 0 {
		return errors.New("STRM target stream probing returned no usable technical facts")
	}
	stripSTRMTargetDescriptiveFacts(&payload)

	exactSeekSafe, keyframeEvidenceAt := false, ""
	if options.ValidateSeekBehavior {
		exactSeekSafe, keyframeEvidenceAt = probeRemoteBoundedExactSeekEvidence(probeCtx, ffprobePath, proxyURL, demuxer, payload)
	}
	// Re-open the small descriptor at the final content boundary. The value is
	// compared only in memory and discarded; neither it nor query material can
	// enter the publication transaction.
	if err := s.validateSTRMDescriptorFile(ctx, source); err != nil {
		return err
	}
	currentLocator, err := s.readSTRMLocatorForWork(ctx, source.Path, foundationcontract.WorkClassBackgroundMedia)
	if err != nil || currentLocator != locator {
		return errSTRMDescriptorStale
	}
	publishOptions := mediaAnalysisOptions{
		Mode: options.Mode, ProbeStreams: true, ValidateSeekBehavior: options.ValidateSeekBehavior,
		AnalyzeSTRMTarget: true, ExpectedSourceRevision: source.SourceRevision,
	}
	return s.persistFFprobeAnalysisInputs(ctx, item, source.Path, proxyURL, payload, publishOptions, exactSeekSafe, keyframeEvidenceAt)
}

func (s *Server) validateSTRMDescriptorFile(ctx context.Context, source strmAnalysisSourceRecord) error {
	info, err := s.analysisSourceStat(ctx, source.Path, "validate STRM descriptor revision")
	if err != nil {
		if ctx.Err() != nil {
			return analysisContextError(ctx, ctx.Err())
		}
		return errSTRMDescriptorStale
	}
	if !info.Mode().IsRegular() || info.Size() != source.SizeBytes || fileModTime(info) != source.ModTime {
		return errSTRMDescriptorStale
	}
	return nil
}

// STRM analysis imports only technical container/stream/seek facts. Removing
// every descriptive tag also prevents an unusual target from reflecting its
// signed request locator into metadata.
func stripSTRMTargetDescriptiveFacts(payload *ffprobePayload) {
	if payload == nil {
		return
	}
	payload.Format.Tags = nil
	for index := range payload.Streams {
		payload.Streams[index].Tags = nil
	}
	payload.Chapters = nil
}

// assertSTRMTargetPublicationFenceTx runs inside the one background write
// owner immediately before stream/fact replacement. Settings changes use the
// same scheduler, so a downgrade cannot interleave with this check and commit.
func assertSTRMTargetPublicationFenceTx(tx *sql.Tx, item MediaItem, recordPath, expectedRevision string) error {
	allowed, err := analysisCapabilityAuthorizedTx(tx, item.LibraryID, recordPath, "analyzeSTRMTarget")
	if err != nil {
		return err
	}
	if !allowed {
		return errSTRMTargetAnalysisDisabled
	}
	var fileID, sourceType, quickSignature, modTime string
	var size int64
	if err := tx.QueryRow(`SELECT id,source_type,size_bytes,mod_time,
		CASE WHEN identity_evidence LIKE 'scanner:v2:%' THEN substr(identity_evidence,12) ELSE '' END
		FROM media_files WHERE media_id=? AND path=? AND available=1`, item.ID, recordPath).Scan(&fileID, &sourceType, &size, &modTime, &quickSignature); err != nil {
		return errSTRMDescriptorStale
	}
	if !strings.EqualFold(sourceType, "strm") && !isSTRMDescriptor(recordPath) {
		return errSTRMDescriptorStale
	}
	revision := scannerAnalysisSourceRevision(scannerMediaFile{
		ID: item.ID, FileID: fileID, SourcePath: recordPath, QuickSignature: quickSignature,
		FileSize: size, FileModTime: modTime,
	})
	if strings.TrimSpace(expectedRevision) == "" || revision != expectedRevision {
		return errSTRMDescriptorStale
	}
	return nil
}

func strmTargetHTTPClient(ctx context.Context, locator string) (*http.Client, error) {
	client, _, err := dlnaRemoteHTTPClient(ctx, locator)
	if err != nil {
		return nil, err
	}
	return client, nil
}

type strmTargetProxyState struct {
	mu                  sync.Mutex
	budget              remoteProbeBudget
	validatorMode       string
	strongETag          string
	lastModified        string
	totalLength         int64
	unvalidatedBodySeen bool
}

func startSTRMTargetProxy(ctx context.Context, locator string, client *http.Client) (string, func(), error) {
	if client == nil {
		return "", nil, errors.New("STRM target client is unavailable")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	proxyCtx, cancelProxy := context.WithCancel(ctx)
	token := randomID("strm-probe")
	localPath := "/" + token
	state := &strmTargetProxyState{budget: remoteProbeBudget{bytesRemaining: strmTargetMaxResponseBytes, requestsRemaining: strmTargetMaxRequests}}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peerHost, _, peerErr := net.SplitHostPort(r.RemoteAddr)
		peer := net.ParseIP(peerHost)
		if peerErr != nil || peer == nil || !peer.IsLoopback() || r.URL.Path != localPath || r.URL.RawQuery != "" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		if !state.budget.beginRequest() {
			http.Error(w, "STRM target probe budget exhausted", http.StatusTooManyRequests)
			return
		}
		upstream, requestErr := http.NewRequestWithContext(r.Context(), r.Method, locator, nil)
		if requestErr != nil {
			http.Error(w, "STRM target request rejected", http.StatusBadGateway)
			return
		}
		upstream.Header.Set("Accept", "*/*")
		upstream.Header.Set("User-Agent", "Portico-STRM-Analysis/1")
		if value := r.Header.Get("Range"); value != "" {
			upstream.Header.Set("Range", value)
		}
		response, requestErr := client.Do(upstream)
		if requestErr != nil {
			http.Error(w, "STRM target unavailable", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		if !state.acceptResponse(response) {
			http.Error(w, "STRM target changed during probing", http.StatusConflict)
			return
		}
		for _, name := range []string{"Accept-Ranges", "Content-Length", "Content-Range", "Content-Type"} {
			if value := response.Header.Get(name); value != "" {
				w.Header().Set(name, value)
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(response.StatusCode)
		if r.Method == http.MethodHead {
			return
		}
		reader := &remoteProbeBudgetReader{source: response.Body, budget: &state.budget}
		_, _ = io.Copy(w, reader)
	})
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second,
		ErrorLog:    log.New(io.Discard, "", 0),
		BaseContext: func(net.Listener) context.Context { return proxyCtx },
	}
	go func() { _ = server.Serve(listener) }()
	var once sync.Once
	closeProxy := func() {
		once.Do(func() {
			cancelProxy()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
			_ = listener.Close()
		})
	}
	return "http://" + listener.Addr().String() + localPath, closeProxy, nil
}

func (state *strmTargetProxyState) acceptResponse(response *http.Response) bool {
	if response == nil || (response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent) {
		return false
	}
	strongETag := strings.TrimSpace(response.Header.Get("ETag"))
	if strings.HasPrefix(strings.ToUpper(strongETag), "W/") {
		strongETag = ""
	}
	lastModified := strings.TrimSpace(response.Header.Get("Last-Modified"))
	totalLength := strmTargetResponseTotalLength(response)
	bodyResponse := response.Request == nil || response.Request.Method != http.MethodHead
	state.mu.Lock()
	defer state.mu.Unlock()
	mode := ""
	if strongETag != "" {
		mode = "etag"
	} else if lastModified != "" && totalLength >= 0 {
		mode = "last-modified-length"
	}
	if mode == "" {
		if state.validatorMode != "" {
			return !bodyResponse
		}
		if !bodyResponse {
			return true
		}
		if state.unvalidatedBodySeen {
			return false
		}
		state.unvalidatedBodySeen = true
		return true
	}
	if state.unvalidatedBodySeen {
		return false
	}
	if state.validatorMode == "" {
		state.validatorMode = mode
		state.strongETag = strongETag
		state.lastModified = lastModified
		state.totalLength = totalLength
		return true
	}
	if state.validatorMode != mode || mode == "etag" && state.strongETag != strongETag || mode == "last-modified-length" && state.lastModified != lastModified {
		return false
	}
	if state.totalLength >= 0 && totalLength >= 0 && state.totalLength != totalLength {
		return false
	}
	if state.totalLength < 0 && totalLength >= 0 {
		state.totalLength = totalLength
	}
	return true
}

func strmTargetResponseTotalLength(response *http.Response) int64 {
	if response == nil {
		return -1
	}
	if raw := strings.TrimSpace(response.Header.Get("Content-Range")); raw != "" {
		if slash := strings.LastIndex(raw, "/"); slash >= 0 && slash+1 < len(raw) {
			if total, err := strconv.ParseInt(strings.TrimSpace(raw[slash+1:]), 10, 64); err == nil && total >= 0 {
				return total
			}
		}
	}
	if response.StatusCode == http.StatusOK || response.Request != nil && response.Request.Method == http.MethodHead {
		if response.ContentLength >= 0 {
			return response.ContentLength
		}
		if total, err := strconv.ParseInt(strings.TrimSpace(response.Header.Get("Content-Length")), 10, 64); err == nil && total >= 0 {
			return total
		}
	}
	return -1
}

func strmTargetLimits() (int64, int, int64, time.Duration) {
	return strmDescriptorLimit, strmTargetMaxRequests, strmTargetMaxResponseBytes, strmTargetProbeTimeout
}

func strmTargetBudgetDescription() string {
	descriptor, requests, bytes, timeout := strmTargetLimits()
	return strconv.FormatInt(descriptor, 10) + ":" + strconv.Itoa(requests) + ":" + strconv.FormatInt(bytes, 10) + ":" + timeout.String()
}
