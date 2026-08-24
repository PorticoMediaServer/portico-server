package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/optimized"
)

type transcodeSession struct {
	stateMu                sync.RWMutex
	updateCh               chan struct{}
	key                    string
	userID                 string
	mediaID                string
	quality                string
	subtitleID             string
	audioMode              string
	audioStreamID          string
	directStream           bool
	start                  int
	method                 string
	filter                 string
	root                   string
	dir                    string
	manifest               string
	startedAt              time.Time
	background             bool
	live                   bool
	cmd                    *exec.Cmd
	cancel                 context.CancelFunc
	supervisor             *transcodeSupervisorV2
	supervised             *transcodeGenerationV2
	storageLease           *playbackStorageLease
	inputTransport         *dvrInputTransport
	done                   chan struct{}
	doneOnce               sync.Once
	err                    error
	errAt                  time.Time
	terminalErr            error
	stopped                bool
	throttled              bool
	stderr                 string
	ffmpegDiagnostics      *FFmpegDiagnostics
	recovering             bool
	recoveryDone           chan struct{}
	recoveryAttempts       int
	lastRecoveryAt         time.Time
	segmentsAfterRecovery  int
	cleanupMu              sync.Mutex
	readerMu               sync.Mutex
	readers                int
	retiring               bool
	readersDone            chan struct{}
	segmentSeconds         int
	playedRetentionSeconds int
	throttleBufferSeconds  int
	lastServedSegment      int
	lastProducedSegment    int
	lastProducedAt         time.Time
	generation             int
	admissionActive        bool
	resourceRelease        func()
}

type transcodeSessionSnapshot struct {
	err                   error
	errAt                 time.Time
	terminalErr           error
	stopped               bool
	throttled             bool
	stderr                string
	ffmpegDiagnostics     *FFmpegDiagnostics
	recovering            bool
	recoveryDone          chan struct{}
	recoveryAttempts      int
	lastRecoveryAt        time.Time
	segmentsAfterRecovery int
	lastProducedSegment   int
	lastProducedAt        time.Time
	generation            int
	admissionActive       bool
}

func (session *transcodeSession) snapshot() transcodeSessionSnapshot {
	if session == nil {
		return transcodeSessionSnapshot{}
	}
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	return transcodeSessionSnapshot{
		err: session.err, errAt: session.errAt, terminalErr: session.terminalErr,
		stopped: session.stopped, throttled: session.throttled, stderr: session.stderr, ffmpegDiagnostics: session.ffmpegDiagnostics,
		recovering: session.recovering, recoveryDone: session.recoveryDone,
		recoveryAttempts: session.recoveryAttempts, lastRecoveryAt: session.lastRecoveryAt,
		segmentsAfterRecovery: session.segmentsAfterRecovery,
		lastProducedSegment:   session.lastProducedSegment, lastProducedAt: session.lastProducedAt,
		generation: session.generation, admissionActive: session.admissionActive,
	}
}

func (session *transcodeSession) signalUpdateLocked() {
	if session.updateCh != nil {
		close(session.updateCh)
	}
	session.updateCh = make(chan struct{})
}

func (session *transcodeSession) updateSignal() <-chan struct{} {
	session.stateMu.Lock()
	defer session.stateMu.Unlock()
	if session.updateCh == nil {
		session.updateCh = make(chan struct{})
	}
	return session.updateCh
}

func (session *transcodeSession) releaseMediaResources() {
	if session == nil {
		return
	}
	session.stateMu.Lock()
	release := session.resourceRelease
	session.resourceRelease = nil
	session.signalUpdateLocked()
	session.stateMu.Unlock()
	if release != nil {
		release()
	}
}

func (session *transcodeSession) markFailure(err error, terminal bool) {
	if session == nil || err == nil {
		return
	}
	session.stateMu.Lock()
	if terminal {
		session.terminalErr = err
	}
	session.err = err
	session.errAt = time.Now().UTC()
	session.signalUpdateLocked()
	session.stateMu.Unlock()
}

func (session *transcodeSession) beginRecovery() (int, chan struct{}) {
	session.stateMu.Lock()
	defer session.stateMu.Unlock()
	session.recovering = true
	session.recoveryDone = make(chan struct{})
	session.recoveryAttempts++
	session.signalUpdateLocked()
	return session.recoveryAttempts, session.recoveryDone
}

func (session *transcodeSession) finishRecovery() {
	if session == nil {
		return
	}
	session.stateMu.Lock()
	if session.recoveryDone != nil {
		close(session.recoveryDone)
	}
	session.recovering = false
	session.recoveryDone = nil
	session.signalUpdateLocked()
	session.stateMu.Unlock()
}

func (session *transcodeSession) setRecoveryState(attempt, generation int) {
	session.stateMu.Lock()
	session.recoveryAttempts = attempt
	session.generation = generation
	session.lastRecoveryAt = time.Now().UTC()
	session.segmentsAfterRecovery = 0
	session.signalUpdateLocked()
	session.stateMu.Unlock()
}

const (
	maxTranscodeRecoveryAttempts     = 3
	transcodeRecoveryInterruptGrace  = 1500 * time.Millisecond
	transcodeRecoveryKillWait        = 2 * time.Second
	transcodeRecoveryHealthySegments = 3
	transcodeRecoveryHealthyDuration = 30 * time.Second
	transcodeReaderDrainTimeout      = 5 * time.Second
)

type transcodeStartRequest struct {
	userID         string
	item           MediaItem
	sourcePath     string
	quality        string
	subtitleID     string
	startSeconds   int
	audioMode      string
	audioStreamID  string
	directStream   bool
	background     bool
	inputTransport *dvrInputTransport
}

type subtitleBurnInSpec struct {
	streamID        string
	streamIndex     int
	subtitleOrdinal int
	codec           string
	external        bool
	path            string
	imageBased      bool
}

var (
	errSubtitleStreamNotFound    = errors.New("subtitle stream was not found")
	errSubtitleBurnInUnavailable = errors.New("subtitle stream is not available for burn-in")
)

type transcodeSettings struct {
	Enabled                 bool
	PlanningPolicy          string
	TemporaryDirectory      string
	MaxConcurrentSessions   int
	X264Preset              string
	ThrottleBufferSeconds   int
	PlayedRetentionSeconds  int
	HardwareAcceleration    bool
	HardwareEncoding        bool
	HardwareDecodeHEVC      bool
	HardwareDevice          string
	MaxHardwareSessions     int
	MaxSoftwareSessions     int
	MaxBackgroundSessions   int
	HDRToneMapping          bool
	HDRToneMappingFilters   bool
	HDRToneMappingAlgorithm string
	DirectStreamRemux       bool
}

type transcodePreset struct {
	id       string
	height   int
	videoK   int
	audioK   int
	crf      int
	label    string
	maxWidth int
}

type optimizedVersionSettings struct {
	DefaultProfile          string
	Templates               []optimizedVersionTemplate
	PreferOptimizedPlayback bool
	StorageDirectory        string
	MaxConcurrentJobs       int
	AutoDelete              bool
	RetentionDays           int
	MaxPerItem              int
	MaxStorageMB            int
}

type optimizedVersionTemplate struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Profile string `json:"profile"`
	Enabled bool   `json:"enabled"`
}

var transcodePresets = map[string]transcodePreset{
	"auto":             {id: "auto", height: 1080, videoK: 12000, audioK: 192, crf: 21, label: "Auto"},
	"1080p-high":       {id: "1080p-high", height: 1080, videoK: 20000, audioK: 192, crf: 20, label: "1080p High"},
	"1080p-medium":     {id: "1080p-medium", height: 1080, videoK: 12000, audioK: 160, crf: 22, label: "1080p Medium"},
	"1080p-standard":   {id: "1080p-standard", height: 1080, videoK: 10000, audioK: 160, crf: 22, label: "1080p"},
	"1080p-low":        {id: "1080p-low", height: 1080, videoK: 8000, audioK: 160, crf: 23, label: "1080p"},
	"720p-high":        {id: "720p-high", height: 720, videoK: 8000, audioK: 160, crf: 22, label: "720p High"},
	"720p-medium":      {id: "720p-medium", height: 720, videoK: 4000, audioK: 128, crf: 24, label: "720p Medium"},
	"720p-standard":    {id: "720p-standard", height: 720, videoK: 3000, audioK: 128, crf: 24, label: "720p Medium"},
	"720p-low":         {id: "720p-low", height: 720, videoK: 2000, audioK: 128, crf: 25, label: "720p"},
	"480p":             {id: "480p", height: 480, videoK: 1500, audioK: 128, crf: 26, label: "480p"},
	"328p":             {id: "328p", height: 328, videoK: 700, audioK: 96, crf: 28, label: "328p"},
	"video-high":       {id: "video-high", height: 2160, videoK: 20000, audioK: 320, crf: 20, label: "High"},
	"video-standard":   {id: "video-standard", height: 1080, videoK: 8000, audioK: 192, crf: 22, label: "Standard"},
	"video-data-saver": {id: "video-data-saver", height: 720, videoK: 4000, audioK: 128, crf: 24, label: "Data Saver"},
	"video-low":        {id: "video-low", height: 480, videoK: 1500, audioK: 96, crf: 26, label: "Low"},
	"audio-high":       {id: "audio-high", height: 1080, videoK: 8000, audioK: 320, crf: 22, label: "High"},
	"audio-standard":   {id: "audio-standard", height: 1080, videoK: 8000, audioK: 192, crf: 22, label: "Standard"},
	"audio-data-saver": {id: "audio-data-saver", height: 720, videoK: 4000, audioK: 128, crf: 24, label: "Data Saver"},
}

func (s *Server) plannedTranscodeIdentityForRequest(ctx context.Context, r *http.Request, user User, mediaID string, binding playbackExecutionBinding) (plannedTranscodeIdentity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	token := mediaGrantFromRequest(r)
	if !strings.HasPrefix(token, "ptc_mg_") || strings.TrimSpace(mediaID) == "" {
		return plannedTranscodeIdentity{}, fmt.Errorf("%w: missing media grant identity", errPlannedTranscode)
	}
	var playbackSessionID, authorizationRevision string
	var grantGeneration, sessionGeneration int
	err := s.queryUserRow(ctx, `
		SELECT g.playback_session_id, COALESCE(g.authorization_revision, ''), COALESCE(g.playback_generation, 0), COALESCE(ps.playback_generation, 0)
		FROM playback_media_grants g
		JOIN playback_sessions ps ON ps.id = g.playback_session_id
		WHERE g.token_hash = ? AND g.resource_kind = 'media' AND g.resource_id = ?
			AND g.principal_user_id = ? AND g.profile_id = ?
			AND g.revoked_at = '' AND g.expires_at > ?
			AND ps.user_id = g.principal_user_id AND ps.profile_id = g.profile_id
			AND ps.ended_at = '' AND ps.state <> 'stopped'
		LIMIT 1`, hashToken(token), strings.TrimSpace(mediaID), accountIDForUser(user), viewerProfileID(user), time.Now().UTC().Format(time.RFC3339)).Scan(&playbackSessionID, &authorizationRevision, &grantGeneration, &sessionGeneration)
	if err != nil || strings.TrimSpace(playbackSessionID) == "" || strings.TrimSpace(authorizationRevision) == "" || grantGeneration != binding.Generation || sessionGeneration != binding.Generation {
		return plannedTranscodeIdentity{}, fmt.Errorf("%w: playback authorization generation changed", errPlannedTranscode)
	}
	identity := plannedTranscodeIdentity{
		UserID:                strings.TrimSpace(user.ID),
		ProfileID:             strings.TrimSpace(viewerProfileID(user)),
		PlaybackSessionID:     strings.TrimSpace(playbackSessionID),
		AuthorizationRevision: strings.TrimSpace(authorizationRevision),
		PlaybackGeneration:    grantGeneration,
		GrantTokenHash:        hashToken(token),
	}
	if !identity.validForGeneration(binding.Generation) {
		return plannedTranscodeIdentity{}, fmt.Errorf("%w: incomplete playback identity", errPlannedTranscode)
	}
	return identity, nil
}

func (s *Server) plannedTranscodeSessionIsCurrent(session *transcodeSession, expectedKey string) bool {
	if s == nil || session == nil || strings.TrimSpace(expectedKey) == "" || session.key != expectedKey {
		return false
	}
	state := session.snapshot()
	if state.stopped || state.err != nil {
		return false
	}
	s.transcodeMu.Lock()
	defer s.transcodeMu.Unlock()
	return s.transcodes[expectedKey] == session
}

func (s *Server) handleMediaHLS(w http.ResponseWriter, r *http.Request, user User, mediaID string, parts []string) {
	if !user.Permissions["playMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to play this media.")
		return
	}
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "HLS route was not found.")
		return
	}
	switch parts[0] {
	case "master.m3u8":
		s.handleMediaHLSManifest(w, r, user, mediaID, true)
	case "variant.m3u8":
		s.handleMediaHLSManifest(w, r, user, mediaID, false)
	case "subtitles.m3u8":
		s.handleMediaHLSSubtitlePlaylist(w, r, user, mediaID)
	case "subtitle.vtt":
		s.handleMediaHLSSubtitleSegment(w, r, user, mediaID)
	case "segment":
		s.handleMediaHLSSegment(w, r, user, mediaID)
	default:
		writeError(w, http.StatusNotFound, "not_found", "HLS route was not found.")
	}
}

func (s *Server) handleMediaHLSManifest(w http.ResponseWriter, r *http.Request, user User, mediaID string, allowTextSubtitles bool) {
	binding, err := s.playbackPlanForMediaGrant(r.Context(), r, mediaID)
	if err != nil {
		writeError(w, http.StatusForbidden, "playback_plan_required", "This playback resource is not bound to a current server plan.")
		return
	}
	identity, err := s.plannedTranscodeIdentityForRequest(r.Context(), r, user, mediaID, binding)
	if err != nil {
		writeError(w, http.StatusForbidden, "playback_plan_required", "This playback resource is not bound to a current playback session.")
		return
	}
	quality, subtitleID, textSubtitleID, audioMode, audioStreamID, directStream, err := playbackBindingHLSParameters(binding, r)
	if err != nil {
		writeError(w, http.StatusForbidden, "playback_plan_mismatch", "Playback resource parameters do not match the authorized server plan.")
		return
	}
	item, err := s.getMediaPlaybackDetailForUser(r.Context(), user, mediaID)
	if err != nil {
		writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
		return
	}
	subtitleID = s.resolveMediaStreamIDContext(r.Context(), item.ID, subtitleID)
	textSubtitleID = s.resolveMediaStreamIDContext(r.Context(), item.ID, textSubtitleID)
	audioStreamID = s.resolveMediaStreamIDContext(r.Context(), item.ID, audioStreamID)
	if subtitleID != "" && textSubtitleID != "" {
		writeError(w, http.StatusBadRequest, "conflicting_subtitle_modes", "Choose either subtitle burn-in or text subtitles, not both.")
		return
	}
	_, err = s.sourcePathForHLSTranscode(item)
	if err != nil {
		writePlaybackSourceError(w, "transcode_unavailable", err)
		return
	}
	if subtitleID != "" {
		if err := s.validateSubtitleBurnInSelection(item, subtitleID); errors.Is(err, errSubtitleStreamNotFound) {
			writeError(w, http.StatusNotFound, "subtitle_stream_not_found", "The requested subtitle stream was not found.")
			return
		} else if err != nil {
			writeError(w, http.StatusConflict, "subtitle_burn_in_unavailable", "The requested subtitle stream is not currently available for burn-in.")
			return
		}
	}
	if _, err := ffmpegAudioMapForSelection(item, audioStreamID); err != nil {
		writeError(w, http.StatusBadRequest, "audio_stream_not_found", "The requested audio stream was not found.")
		return
	}
	if textSubtitleID != "" && !hlsTextSubtitleAvailable(item, textSubtitleID) {
		writeError(w, http.StatusNotFound, "subtitle_not_found", "Subtitle was not found.")
		return
	}
	// FFmpeg is the segment timeline authority for every HLS execution. A
	// catalog duration is not evidence of actual keyframe boundaries, so never
	// synthesize EXTINF entries before output exists. Publish only a validated
	// generated playlist and let the producer add ENDLIST at finite-source EOF.
	session, err := s.ensurePlannedVODHLSSession(r.Context(), user.ID, item, binding, identity, 0, "", false)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "transcode_unavailable", err.Error())
		return
	}
	if !s.plannedTranscodeSessionIsCurrent(session, plannedTranscodeSessionKey(item.ID, binding, identity, 0)) {
		writeError(w, http.StatusServiceUnavailable, "transcode_unavailable", "The playback generation changed before the manifest was published.")
		return
	}
	manifest, err := s.readTranscodeManifestContext(r.Context(), session, mediaID, quality, subtitleID, 0, audioMode, audioStreamID, directStream, playbackURLMediaGrant(r))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, errLongPollShutdown) {
			return
		}
		writeError(w, http.StatusServiceUnavailable, "transcode_unavailable", "The playback manifest is not ready.")
		return
	}
	releaseReader, ok := session.acquireReader()
	if !ok || !s.plannedTranscodeSessionIsCurrent(session, plannedTranscodeSessionKey(item.ID, binding, identity, 0)) {
		if ok {
			releaseReader()
		}
		writeError(w, http.StatusServiceUnavailable, "transcode_unavailable", "The playback generation changed before the manifest was published.")
		return
	}
	defer releaseReader()
	if allowTextSubtitles {
		masterItem, evidenceErr := s.hlsItemWithVerifiedDolbyVisionEvidence(r.Context(), item, directStream)
		if evidenceErr != nil {
			writeError(w, http.StatusUnprocessableEntity, "dolby_vision_output_unverified", evidenceErr.Error())
			return
		}
		manifest = buildMediaHLSMasterManifestWithBandwidth(masterItem, mediaID, quality, textSubtitleID, 0, audioMode, audioStreamID, directStream, playbackURLMediaGrant(r), "", manifest, measureGeneratedHLSBandwidth(session))
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(manifest))
}

func (s *Server) handleMediaHLSSubtitlePlaylist(w http.ResponseWriter, r *http.Request, user User, mediaID string) {
	binding, err := s.playbackPlanForMediaGrant(r.Context(), r, mediaID)
	if err != nil {
		writeError(w, http.StatusForbidden, "playback_plan_required", "This subtitle resource is not bound to a current server plan.")
		return
	}
	_, _, subtitleID, _, _, _, err := playbackBindingHLSParameters(binding, r)
	if err != nil || subtitleID == "" {
		writeError(w, http.StatusForbidden, "playback_plan_mismatch", "Subtitle resource parameters do not match the authorized server plan.")
		return
	}
	item, err := s.getMediaPlaybackDetailForUser(r.Context(), user, mediaID)
	if err != nil {
		writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
		return
	}
	subtitleID = s.resolveMediaStreamIDContext(r.Context(), item.ID, subtitleID)
	if subtitleID == "" || !hlsTextSubtitleAvailable(item, subtitleID) {
		writeError(w, http.StatusNotFound, "subtitle_not_found", "Subtitle was not found.")
		return
	}
	if _, _, err := s.subtitleStreamPathAndOffset(mediaID, subtitleID); err != nil {
		writeError(w, http.StatusNotFound, "subtitle_not_found", "Subtitle was not found.")
		return
	}
	duration := max(1, item.DurationSeconds)
	segmentCount, err := staticHLSegmentCount(duration)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "media_duration_unsupported", err.Error())
		return
	}
	startIndex := 0
	var playlist strings.Builder
	playlist.WriteString("#EXTM3U\n")
	playlist.WriteString("#EXT-X-VERSION:3\n")
	playlist.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", hlsSegmentSeconds))
	playlist.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", startIndex))
	playlist.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	for index := startIndex; index < segmentCount; index++ {
		remaining := duration - index*hlsSegmentSeconds
		segmentDuration := hlsSegmentSeconds
		if remaining > 0 && remaining < hlsSegmentSeconds {
			segmentDuration = remaining
		}
		playlist.WriteString(fmt.Sprintf("#EXTINF:%d.000,\n", max(1, segmentDuration)))
		playlist.WriteString(mediaHLSSubtitleSegmentRoute(mediaID, subtitleID, index, 0, playbackURLMediaGrant(r), ""))
		playlist.WriteByte('\n')
	}
	playlist.WriteString("#EXT-X-ENDLIST\n")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(playlist.String()))
}

func (s *Server) handleMediaHLSSubtitleSegment(w http.ResponseWriter, r *http.Request, user User, mediaID string) {
	binding, err := s.playbackPlanForMediaGrant(r.Context(), r, mediaID)
	if err != nil {
		writeError(w, http.StatusForbidden, "playback_plan_required", "This subtitle resource is not bound to a current server plan.")
		return
	}
	_, _, subtitleID, _, _, _, err := playbackBindingHLSParameters(binding, r)
	if err != nil || subtitleID == "" {
		writeError(w, http.StatusForbidden, "playback_plan_mismatch", "Subtitle resource parameters do not match the authorized server plan.")
		return
	}
	item, err := s.getMediaPlaybackDetailForUser(r.Context(), user, mediaID)
	if err != nil {
		writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
		return
	}
	subtitleID = s.resolveMediaStreamIDContext(r.Context(), item.ID, subtitleID)
	if subtitleID == "" || !hlsTextSubtitleAvailable(item, subtitleID) {
		writeError(w, http.StatusNotFound, "subtitle_not_found", "Subtitle was not found.")
		return
	}
	segmentIndex := mediaHLSSubtitleSegmentIndex(r)
	segmentStartSeconds := segmentIndex * hlsSegmentSeconds
	path, offsetMs, err := s.subtitleStreamPathAndOffset(mediaID, subtitleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "subtitle_not_found", "Subtitle was not found.")
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=300")
	data, err := s.readPlaybackSubtitleFile(r.Context(), path)
	if err != nil {
		if errors.Is(err, errPlaybackStorageOffline) || errors.Is(err, errPlaybackStorageStalled) || errors.Is(err, errPlaybackStorageTransient) {
			writePlaybackSourceError(w, "subtitle_unavailable", err)
			return
		}
		writeError(w, http.StatusNotFound, "subtitle_not_found", "Subtitle was not found.")
		return
	}
	segment := buildHLSWebVTTMediaSegment(data, offsetMs, segmentStartSeconds, hlsSegmentSeconds)
	w.Header().Set("Content-Length", strconv.Itoa(len(segment)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(segment)
	}
}

func (s *Server) handleMediaHLSSegment(w http.ResponseWriter, r *http.Request, user User, mediaID string) {
	binding, err := s.playbackPlanForMediaGrant(r.Context(), r, mediaID)
	if err != nil {
		writeError(w, http.StatusForbidden, "playback_plan_required", "This playback segment is not bound to a current server plan.")
		return
	}
	identity, err := s.plannedTranscodeIdentityForRequest(r.Context(), r, user, mediaID, binding)
	if err != nil {
		writeError(w, http.StatusForbidden, "playback_plan_required", "This playback segment is not bound to a current playback session.")
		return
	}
	quality, subtitleID, _, _, audioStreamID, _, err := playbackBindingHLSParameters(binding, r)
	if err != nil {
		writeError(w, http.StatusForbidden, "playback_plan_mismatch", "Playback segment parameters do not match the authorized server plan.")
		return
	}
	name := filepath.Base(r.URL.Query().Get("name"))
	if name == "." || name == string(filepath.Separator) || !validHLSSegmentName(name) {
		writeError(w, http.StatusBadRequest, "bad_segment", "HLS segment name is invalid.")
		return
	}
	item, err := s.getMediaPlaybackDetailForUser(r.Context(), user, mediaID)
	if err != nil {
		writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
		return
	}
	subtitleID = s.resolveMediaStreamIDContext(r.Context(), item.ID, subtitleID)
	audioStreamID = s.resolveMediaStreamIDContext(r.Context(), item.ID, audioStreamID)
	if _, err := s.sourcePathForHLSTranscode(item); err != nil {
		writePlaybackSourceError(w, "transcode_unavailable", err)
		return
	}
	if _, err := ffmpegAudioMapForSelection(item, audioStreamID); err != nil {
		writeError(w, http.StatusBadRequest, "audio_stream_not_found", "The requested audio stream was not found.")
		return
	}
	startSeconds := 0
	if item.DurationSeconds > 0 && name != "init.mp4" {
		startSeconds = mediaHLSSegmentStartSeconds(0, name, item.DurationSeconds)
	}
	for recoveryPass := 0; recoveryPass < 2; recoveryPass++ {
		session, err := s.ensurePlannedVODHLSSession(r.Context(), user.ID, item, binding, identity, startSeconds, name, false)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "transcode_unavailable", err.Error())
			return
		}
		expectedKey := plannedTranscodeSessionKey(item.ID, binding, identity, startSeconds)
		if !s.plannedTranscodeSessionIsCurrent(session, expectedKey) {
			continue
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
				// The next pass re-enters the plan-owned launcher. A failed
				// generation is retired before the supervisor starts its replacement.
				if recoveryPass == 0 {
					continue
				}
				writeError(w, http.StatusServiceUnavailable, "transcode_unavailable", session.transcodeError().Error())
				return
			}
			s.recordLog("warn", "HLS segment was not ready before the playback client timed out", map[string]string{
				"media":   item.ID,
				"segment": name,
				"quality": quality,
				"start":   strconv.Itoa(startSeconds),
				"method":  session.method,
				"error":   err.Error(),
			})
			if session.isRunning() {
				w.Header().Set("Retry-After", "2")
				writeError(w, http.StatusServiceUnavailable, "segment_starting", "HLS segment is still being prepared. Retry shortly.")
				return
			}
			writeError(w, http.StatusNotFound, "segment_not_found", "HLS segment is not available.")
			return
		}
		releaseReader, ok := session.acquireReader()
		if !ok || !s.plannedTranscodeSessionIsCurrent(session, expectedKey) {
			if ok {
				releaseReader()
			}
			continue
		}
		defer releaseReader()
		s.noteTranscodeSegmentServed(session, name)
		w.Header().Set("Content-Type", hlsSegmentContentType(name))
		// A producer may be repositioned or recovered for the same deterministic
		// grid slot. Never let an intermediary mix bytes across producer runs.
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeFile(w, r, path)
		return
	}
	writeError(w, http.StatusServiceUnavailable, "transcode_unavailable", "The HLS transcode session could not recover in time.")
}

func validHLSSegmentName(name string) bool {
	return strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".m4s") || name == "init.mp4"
}

func hlsSegmentContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".ts"):
		return "video/mp2t"
	case strings.HasSuffix(name, ".m4s"):
		return "video/iso.segment"
	case name == "init.mp4":
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

func mediaHLSStartSeconds(r *http.Request, item MediaItem) int {
	if r == nil {
		return 0
	}
	value := strings.TrimSpace(r.URL.Query().Get("start"))
	if value == "" {
		value = strings.TrimSpace(r.URL.Query().Get("startSeconds"))
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return boundedMediaHLSStartSeconds(seconds, item.DurationSeconds)
}

func boundedMediaHLSStartSeconds(seconds int, durationSeconds int) int {
	seconds = max(0, seconds)
	if durationSeconds <= 1 {
		return seconds
	}
	return min(seconds, max(0, durationSeconds-1))
}

func mediaHLSSegmentStartSeconds(manifestStartSeconds int, segmentName string, durationSeconds int) int {
	requestedIndex, ok := transcodeSegmentIndex(segmentName)
	if !ok {
		return boundedMediaHLSStartSeconds(manifestStartSeconds, durationSeconds)
	}
	manifestStartSeconds = boundedMediaHLSStartSeconds(manifestStartSeconds, durationSeconds)
	manifestStartIndex := manifestStartSeconds / hlsSegmentSeconds
	if requestedIndex <= manifestStartIndex {
		return manifestStartSeconds
	}
	return boundedMediaHLSStartSeconds(manifestStartSeconds+(requestedIndex-manifestStartIndex)*hlsSegmentSeconds, durationSeconds)
}

func mediaHLSSubtitleSegmentIndex(r *http.Request) int {
	if r == nil {
		return 0
	}
	value := strings.TrimSpace(r.URL.Query().Get("segment"))
	if value == "" {
		value = strings.TrimSpace(r.URL.Query().Get("index"))
	}
	index, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return max(0, index)
}

const hlsTimestampContractVersion = "hls_full_timeline_v1"
const hlsSegmentSeconds = 4

func transcodeSessionKey(mediaID, quality, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool) string {
	return mediaID + ":" + quality + ":" + normalizeBurnInSubtitleID(subtitleID) + ":" + strconv.Itoa(max(0, startSeconds)) + ":" + normalizeTranscodeAudioMode(audioMode) + ":" + normalizeSelectedAudioStreamID(audioStreamID) + ":" + transcodeModePathComponent(directStream) + ":" + hlsTimestampContractVersion
}

func transcodeStartPathComponent(startSeconds int) string {
	return fmt.Sprintf("start_%06d", max(0, startSeconds))
}

func transcodeAudioPathComponent(audioMode string) string {
	if normalizeTranscodeAudioMode(audioMode) == "transcode" {
		return "audio_transcode"
	}
	return "audio_copy"
}

func transcodeAudioStreamPathComponent(audioStreamID string) string {
	audioStreamID = normalizeSelectedAudioStreamID(audioStreamID)
	if audioStreamID == "" {
		return ""
	}
	return "stream_" + safePathComponent(audioStreamID)
}

func transcodeModePathComponent(directStream bool) string {
	if directStream {
		return "direct_stream"
	}
	return "video_transcode"
}

func hlsDirectStreamRemuxRequested(query url.Values) bool {
	// Full-timeline VOD currently guarantees exact segment boundaries by
	// encoding on a fixed grid. A caller cannot opt back into keyframe-snapped
	// copy-remux behavior through a query parameter.
	_ = query
	return false
}

func (s *Server) ensureTranscodeSession(userID string, item MediaItem, quality string, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool) (*transcodeSession, error) {
	return s.ensureTranscodeSessionForSegmentWithIntentAndInput(userID, item, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, "", false, nil)
}

func (s *Server) ensureTranscodeSessionWithInputTransport(userID string, item MediaItem, quality string, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, inputTransport *dvrInputTransport) (*transcodeSession, error) {
	return s.ensureTranscodeSessionForSegmentWithIntentAndInput(userID, item, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, "", false, inputTransport)
}

func (s *Server) ensureBackgroundTranscodeSession(userID string, item MediaItem, quality string, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool) (*transcodeSession, error) {
	return s.ensureTranscodeSessionForSegmentWithIntent(userID, item, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, "", true)
}

func isExpectedBackgroundTranscodeRefusal(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "background transcode prewarm deferred") ||
		strings.Contains(message, "background transcode session limit")
}

func (s *Server) ensureTranscodeSessionForSegment(userID string, item MediaItem, quality string, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, segmentName string) (*transcodeSession, error) {
	return s.ensureTranscodeSessionForSegmentWithIntentAndInput(userID, item, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, segmentName, false, nil)
}

func (s *Server) ensureTranscodeSessionForSegmentWithIntent(userID string, item MediaItem, quality string, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, segmentName string, background bool) (*transcodeSession, error) {
	return s.ensureTranscodeSessionForSegmentWithIntentAndInput(userID, item, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, segmentName, background, nil)
}

func (s *Server) ensureTranscodeSessionForSegmentWithIntentAndInput(userID string, item MediaItem, quality string, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, segmentName string, background bool, inputTransport *dvrInputTransport) (*transcodeSession, error) {
	settings := s.transcodeSettings()
	if !settings.Enabled {
		return nil, errors.New("transcoding is disabled")
	}
	if background && s.shouldDeferBackgroundJobsForPressure() {
		return nil, errors.New("background transcode prewarm deferred while server is under load")
	}
	sourcePath := strings.TrimSpace(item.SourceURL)
	if inputTransport != nil {
		sourcePath = strings.TrimSpace(inputTransport.URL)
		if sourcePath == "" {
			return nil, errors.New("provider input transport is unavailable")
		}
	} else {
		var err error
		sourcePath, err = s.sourcePathForHLSTranscode(item)
		if err != nil {
			return nil, err
		}
	}
	inputTransportOwned := inputTransport != nil
	defer func() {
		if inputTransportOwned && inputTransport != nil {
			inputTransport.Close()
		}
	}()
	if _, err := exec.LookPath(s.cfg.FFmpegPath); err != nil && filepath.Base(s.cfg.FFmpegPath) == s.cfg.FFmpegPath {
		return nil, errors.New("FFmpeg is not available on PATH")
	}
	startSeconds = boundedMediaHLSStartSeconds(startSeconds, item.DurationSeconds)
	audioMode = normalizeTranscodeAudioMode(audioMode)
	audioStreamID = normalizeSelectedAudioStreamID(audioStreamID)
	if _, err := ffmpegAudioMapForSelection(item, audioStreamID); err != nil {
		return nil, err
	}
	subtitleID = normalizeBurnInSubtitleID(subtitleID)
	if subtitleID != "" {
		if err := s.validateSubtitleBurnInSelection(item, subtitleID); err != nil {
			return nil, err
		}
	}
	if segmentName != "" {
		if session := s.findReusableTranscodeSession(item.ID, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, segmentName); session != nil {
			return session, nil
		}
	}
	admissionContext, releaseAdmission, admissionErr := s.restoreBarrier.acquire(context.Background())
	if admissionErr != nil {
		return nil, errors.New("restore admission is quiescing")
	}
	if admissionContext.Err() != nil {
		releaseAdmission()
		return nil, errors.New("restore admission is quiescing")
	}
	key := transcodeSessionKey(item.ID, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream)
	startRequest := transcodeStartRequest{
		userID:         userID,
		item:           item,
		sourcePath:     sourcePath,
		quality:        quality,
		subtitleID:     subtitleID,
		startSeconds:   startSeconds,
		audioMode:      audioMode,
		audioStreamID:  audioStreamID,
		directStream:   directStream,
		background:     background,
		inputTransport: inputTransport,
	}
	s.transcodeMu.Lock()
	if s.transcodes == nil {
		s.transcodes = map[string]*transcodeSession{}
	}
	if existing := s.transcodes[key]; existing != nil {
		existingState := existing.snapshot()
		if existingState.err != nil {
			if existingState.terminalErr != nil {
				releaseAdmission()
				s.transcodeMu.Unlock()
				return nil, existing.transcodeError()
			}
			if !background && existing.recoverableFailure() {
				s.transcodeMu.Unlock()
				session, recoveryErr := s.recoverTranscodeSessionForDemand(admissionContext, settings, startRequest, existing, existingState.err)
				if recoveryErr != nil {
					releaseAdmission()
					return nil, recoveryErr
				}
				if session != nil && session.inputTransport == inputTransport {
					inputTransportOwned = false
				}
				// The replacement is registered before this function returns. The
				// restore barrier protects only admission/start/registration; the
				// restore quiescer owns cancellation and draining after the lease is
				// released. Holding it until session.done would deadlock quiescence.
				releaseAdmission()
				return session, nil
			}
			if time.Since(existingState.errAt) < 30*time.Second {
				releaseAdmission()
				s.transcodeMu.Unlock()
				return nil, existing.transcodeError()
			}
			delete(s.transcodes, key)
		} else if existing.isRunning() || existing.completedSuccessfully() {
			releaseAdmission()
			s.transcodeMu.Unlock()
			return existing, nil
		} else {
			delete(s.transcodes, key)
		}
	}
	var superseded []*transcodeSession
	if !background && segmentName != "" {
		superseded = s.supersedeStaleSeekTranscodesLocked(userID, item.ID, quality, subtitleID, audioMode, audioStreamID, directStream, segmentName, key)
	}
	session, err := s.startAdmittedTranscodeLocked(settings, startRequest)
	if err != nil {
		releaseAdmission()
		s.transcodeMu.Unlock()
		if len(superseded) > 0 {
			go s.stopSupersededSeekTranscodes(superseded)
		}
		return nil, err
	}
	inputTransportOwned = false
	s.transcodes[key] = session
	s.transcodeMu.Unlock()
	// The session is now visible to quiesceForRestore, which will cancel and
	// drain it after sealing new admissions. Do not hold this lease for the
	// lifetime of the transcode.
	releaseAdmission()
	if len(superseded) > 0 {
		go s.stopSupersededSeekTranscodes(superseded)
	}
	return session, nil
}

func (s *Server) recoverTranscodeSessionForDemandGuarded(ctx context.Context, settings transcodeSettings, request transcodeStartRequest, failed *transcodeSession, cause error) (*transcodeSession, error) {
	admissionContext, releaseAdmission, err := s.restoreBarrier.acquire(ctx)
	if err != nil {
		return nil, err
	}
	if admissionContext.Err() != nil {
		releaseAdmission()
		return nil, errors.New("restore admission is quiescing")
	}
	session, recoveryErr := s.recoverTranscodeSessionForDemand(admissionContext, settings, request, failed, cause)
	if recoveryErr != nil {
		releaseAdmission()
		return nil, recoveryErr
	}
	// recoverTranscodeSessionForDemand has installed the replacement in the
	// registry before returning. The barrier lease ends at that registration
	// boundary; quiescence must be able to cancel/drain the active session.
	releaseAdmission()
	return session, nil
}

// supersedeStaleSeekTranscodesLocked gives the newest foreground seek an
// admission slot instead of allowing abandoned seek and prewarm producers to
// exhaust the per-user transcode limit. Adjacent segment requests are resolved
// by findReusableTranscodeSession before this point and continue to share one
// producer.
func (s *Server) supersedeStaleSeekTranscodesLocked(userID, mediaID, quality, subtitleID, audioMode, audioStreamID string, directStream bool, segmentName, keepKey string) []*transcodeSession {
	requestedIndex, ok := transcodeSegmentIndex(segmentName)
	if !ok {
		return nil
	}
	userID = strings.TrimSpace(userID)
	subtitleID = normalizeBurnInSubtitleID(subtitleID)
	audioMode = normalizeTranscodeAudioMode(audioMode)
	audioStreamID = normalizeSelectedAudioStreamID(audioStreamID)
	var superseded []*transcodeSession
	for key, session := range s.transcodes {
		if key == keepKey || session == nil || !session.isRunning() ||
			session.userID != userID || session.mediaID != mediaID ||
			session.quality != quality || session.subtitleID != subtitleID ||
			session.audioMode != audioMode || session.audioStreamID != audioStreamID ||
			session.directStream != directStream || transcodeSessionHasSegment(session, segmentName) {
			continue
		}
		sessionStartIndex := session.start / max(1, session.segmentSeconds)
		if requestedIndex >= sessionStartIndex && requestedIndex <= session.snapshot().lastProducedSegment+3 {
			continue
		}
		delete(s.transcodes, key)
		session.requestStop()
		superseded = append(superseded, session)
	}
	return superseded
}

func (s *Server) stopSupersededSeekTranscodes(sessions []*transcodeSession) {
	for _, session := range sessions {
		if session == nil {
			continue
		}
		_ = session.stopAndWait(transcodeRecoveryInterruptGrace)
		if err := cleanupTranscodeSessionFiles(session); err != nil {
			s.recordLog("warn", "Superseded seek transcode cleanup failed", map[string]string{"mediaId": session.mediaID, "quality": session.quality, "error": err.Error()})
		}
		s.recordLog("debug", "Superseded stale seek transcode", map[string]string{"mediaId": session.mediaID, "quality": session.quality, "start": strconv.Itoa(session.start)})
	}
}

func (s *Server) findReusableTranscodeSession(mediaID string, quality string, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, segmentName string) *transcodeSession {
	requestedIndex, ok := transcodeSegmentIndex(segmentName)
	if !ok {
		return nil
	}
	subtitleID = normalizeBurnInSubtitleID(subtitleID)
	audioMode = normalizeTranscodeAudioMode(audioMode)
	audioStreamID = normalizeSelectedAudioStreamID(audioStreamID)
	startIndex := startSeconds / hlsSegmentSeconds
	s.transcodeMu.Lock()
	defer s.transcodeMu.Unlock()
	var best *transcodeSession
	bestStartIndex := -1
	for _, session := range s.transcodes {
		if session == nil ||
			session.mediaID != mediaID ||
			session.quality != quality ||
			session.subtitleID != subtitleID ||
			session.audioMode != audioMode ||
			session.audioStreamID != audioStreamID ||
			session.directStream != directStream ||
			session.snapshot().err != nil {
			continue
		}
		sessionStartIndex := session.start / max(1, session.segmentSeconds)
		if sessionStartIndex > requestedIndex || sessionStartIndex > startIndex {
			continue
		}
		hasRequestedSegment := transcodeSessionHasSegment(session, segmentName)
		if !session.isRunning() && !session.completedSuccessfully() && !hasRequestedSegment {
			continue
		}
		if session.start == 0 && requestedIndex > session.servedSegment()+3 && !hasRequestedSegment {
			continue
		}
		if sessionStartIndex > bestStartIndex {
			best = session
			bestStartIndex = sessionStartIndex
		}
	}
	return best
}

func transcodeSessionHasSegment(session *transcodeSession, name string) bool {
	if session == nil || strings.TrimSpace(session.dir) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(session.dir, filepath.Base(name)))
	return err == nil && !info.IsDir() && info.Size() > 0
}

func (s *Server) recoverTranscodeSessionForDemand(ctx context.Context, settings transcodeSettings, request transcodeStartRequest, failed *transcodeSession, cause error) (*transcodeSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request.background = false
	key := transcodeSessionKey(request.item.ID, request.quality, request.subtitleID, request.startSeconds, request.audioMode, request.audioStreamID, request.directStream)
	for {
		s.transcodeMu.Lock()
		if s.transcodes == nil {
			s.transcodes = map[string]*transcodeSession{}
		}
		current := s.transcodes[key]
		if current == nil {
			session, err := s.startAdmittedTranscodeLocked(settings, request)
			if err == nil {
				s.transcodes[key] = session
			}
			s.transcodeMu.Unlock()
			return session, err
		}
		if failed == nil || current != failed {
			if current.isRunning() || current.completedSuccessfully() {
				s.transcodeMu.Unlock()
				return current, nil
			}
			failed = current
		}
		if !failed.recoverableFailure() {
			err := failed.transcodeError()
			if err == nil {
				err = cause
			}
			if err == nil {
				err = errors.New("transcode session is not recoverable")
			}
			s.transcodeMu.Unlock()
			return nil, err
		}
		failedState := failed.snapshot()
		if failedState.recoveryAttempts >= maxTranscodeRecoveryAttempts {
			err := failed.transcodeError()
			if err == nil {
				err = cause
			}
			if err == nil {
				err = errors.New("transcode session failed")
			}
			failed.markFailure(fmt.Errorf("transcode recovery attempts exhausted: %w", err), true)
			s.transcodeMu.Unlock()
			return nil, failed.transcodeError()
		}
		if failedState.recovering && failedState.recoveryDone != nil {
			done := failedState.recoveryDone
			s.transcodeMu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		attempt, _ := failed.beginRecovery()
		s.transcodeMu.Unlock()

		stopErr := failed.stopAndWait(transcodeRecoveryInterruptGrace)

		s.transcodeMu.Lock()
		if s.transcodes[key] != failed {
			failed.finishRecovery()
			s.transcodeMu.Unlock()
			continue
		}
		if stopErr != nil {
			failed.markFailure(fmt.Errorf("unable to stop failed transcode before recovery: %w", stopErr), true)
			failed.finishRecovery()
			s.transcodeMu.Unlock()
			return nil, failed.transcodeError()
		}
		if err := archiveFailedTranscodeGeneration(failed); err != nil {
			failed.markFailure(fmt.Errorf("unable to isolate failed transcode generation: %w", err), true)
			failed.finishRecovery()
			s.transcodeMu.Unlock()
			return nil, failed.transcodeError()
		}
		session, err := s.startAdmittedTranscodeLocked(settings, request)
		if err != nil {
			terminal := false
			if attempt >= maxTranscodeRecoveryAttempts {
				err = fmt.Errorf("transcode recovery attempts exhausted: %w", err)
				terminal = true
			}
			failed.markFailure(err, terminal)
			failed.finishRecovery()
			s.transcodeMu.Unlock()
			return nil, failed.transcodeError()
		}
		session.setRecoveryState(attempt, failedState.generation+1)
		s.transcodes[key] = session
		failed.finishRecovery()
		s.transcodeMu.Unlock()
		s.recordLog("warn", "Recovered transcode session after FFmpeg failure", map[string]string{
			"mediaId": request.item.ID,
			"quality": request.quality,
			"start":   strconv.Itoa(request.startSeconds),
			"attempt": strconv.Itoa(attempt),
		})
		return session, nil
	}
}

func archiveFailedTranscodeGeneration(session *transcodeSession) error {
	if session == nil || strings.TrimSpace(session.dir) == "" {
		return nil
	}
	if _, err := os.Stat(session.dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if !session.retireAndWait(transcodeReaderDrainTimeout) {
		return errors.New("transcode generation is still serving a reader")
	}
	archive := fmt.Sprintf("%s.generation-%d-%d", session.dir, max(0, session.snapshot().generation), time.Now().UTC().UnixNano())
	return os.Rename(session.dir, archive)
}

func (s *Server) startAdmittedTranscodeLocked(settings transcodeSettings, request transcodeStartRequest) (*transcodeSession, error) {
	if err := s.checkBaseTranscodeAdmissionLocked(settings, request.userID, request.background); err != nil {
		return nil, err
	}
	if request.inputTransport != nil {
		return s.startTranscodeLockedWithInputTransport(
			request.userID,
			request.item,
			request.sourcePath,
			request.quality,
			settings,
			request.subtitleID,
			request.startSeconds,
			request.audioMode,
			request.audioStreamID,
			request.directStream,
			request.background,
			request.inputTransport,
		)
	}
	return s.startTranscodeLocked(
		request.userID,
		request.item,
		request.sourcePath,
		request.quality,
		settings,
		request.subtitleID,
		request.startSeconds,
		request.audioMode,
		request.audioStreamID,
		request.directStream,
		request.background,
	)
}

func (s *Server) checkBaseTranscodeAdmissionLocked(settings transcodeSettings, userID string, background bool) error {
	active := 0
	for _, session := range s.transcodes {
		if session.isRunning() {
			active++
		}
	}
	if settings.MaxConcurrentSessions > 0 && active >= settings.MaxConcurrentSessions {
		s.transcodeRejected.Add(1)
		return errors.New("the transcode session limit has been reached")
	}
	if background && !s.backgroundTranscodeSlotAvailableLocked(settings) {
		s.transcodeRejected.Add(1)
		return errors.New("the background transcode session limit has been reached")
	}
	if !s.userCanStartTranscodeLocked(userID, settings) {
		s.transcodeUserRejected.Add(1)
		return errors.New("the user transcode session limit has been reached")
	}
	return nil
}

func (s *Server) userCanStartTranscodeLocked(userID string, settings transcodeSettings) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return true
	}
	limit := maxConcurrentTranscodesPerUser(settings)
	if limit == 0 {
		return true
	}
	active := 0
	for _, session := range s.transcodes {
		if session != nil && session.userID == userID && session.isRunning() {
			active++
		}
	}
	return active < limit
}

func (s *Server) backgroundTranscodeSlotAvailableLocked(settings transcodeSettings) bool {
	capacity := settings.MaxBackgroundSessions
	if capacity <= 0 {
		return false
	}
	active := 0
	for _, session := range s.transcodes {
		if session != nil && session.background && session.isRunning() {
			active++
		}
	}
	return active < capacity
}

func maxConcurrentTranscodesPerUser(settings transcodeSettings) int {
	if settings.MaxConcurrentSessions == 0 {
		return 0
	}
	return max(1, min(2, settings.MaxConcurrentSessions))
}

func (s *Server) startTranscodeLocked(userID string, item MediaItem, sourcePath string, quality string, settings transcodeSettings, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, background bool) (*transcodeSession, error) {
	return s.startTranscodeLockedWithInputTransport(userID, item, sourcePath, quality, settings, subtitleID, startSeconds, audioMode, audioStreamID, directStream, background, nil)
}

func (s *Server) startTranscodeLockedWithInputTransport(userID string, item MediaItem, sourcePath string, quality string, settings transcodeSettings, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, background bool, inputTransport *dvrInputTransport) (*transcodeSession, error) {
	if s.transcodeClosing {
		return nil, errors.New("the server is shutting down")
	}
	preset := transcodePresets[quality]
	if quality == "original" {
		preset = sourceEquivalentTranscodePreset(item)
	}
	audioMode = normalizeTranscodeAudioMode(audioMode)
	audioStreamID = normalizeSelectedAudioStreamID(audioStreamID)
	audioMap, err := ffmpegAudioMapForSelection(item, audioStreamID)
	if err != nil {
		return nil, err
	}
	directStreamRemux := directStream && directStreamRemuxAvailable(item, quality, settings, subtitleID)
	forceAudioTranscode := audioMode == "transcode"
	ctx, cancel := context.WithCancel(context.Background())
	cancelOnError := true
	defer func() {
		if cancelOnError {
			cancel()
			if inputTransport != nil {
				inputTransport.Close()
			}
		}
	}()
	var stdin io.ReadCloser
	outputRoot := settings.TemporaryDirectory
	if strings.TrimSpace(outputRoot) == "" {
		outputRoot = s.cfg.TranscodeDir
	}
	absoluteOutputRoot, err := filepath.Abs(outputRoot)
	if err != nil {
		return nil, err
	}
	outputRoot = absoluteOutputRoot
	if err := ensureMediaWriteCapacity(outputRoot, mediaWriteMinimumFreeBytes); err != nil {
		return nil, err
	}
	resourceRequest := mediaResourceRequest{cpu: 1, disk: 2, background: background}
	if item.Type == "live_channel" {
		resourceRequest.network = 1
	}
	governor := s.mediaResourceGovernor()
	var resourceRelease func()
	var acquired bool
	if !background {
		governor.preemptBackgroundForPlayback()
		admissionCtx, admissionCancel := context.WithTimeout(ctx, 2*time.Second)
		resourceRelease, err = governor.acquireContext(admissionCtx, resourceRequest)
		admissionCancel()
		acquired = err == nil
	} else {
		resourceRelease, acquired = governor.tryAcquire(resourceRequest)
	}
	if !acquired {
		return nil, errMediaResourcesBusy
	}
	defer func() {
		if cancelOnError {
			resourceRelease()
		}
	}()
	startSeconds = boundedMediaHLSStartSeconds(startSeconds, item.DurationSeconds)
	sessionBaseDir := filepath.Join(outputRoot, hlsTimestampContractVersion, safePathComponent(item.ID), safePathComponent(quality), transcodeStartPathComponent(startSeconds), transcodeModePathComponent(directStreamRemux), transcodeAudioPathComponent(audioMode))
	if audioStreamPath := transcodeAudioStreamPathComponent(audioStreamID); audioStreamPath != "" {
		sessionBaseDir = filepath.Join(sessionBaseDir, audioStreamPath)
	}
	absoluteSessionBaseDir, err := filepath.Abs(sessionBaseDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absoluteSessionBaseDir, 0o700); err != nil {
		return nil, err
	}
	sessionDir, err := os.MkdirTemp(absoluteSessionBaseDir, ".generation-")
	if err != nil {
		return nil, err
	}
	generationOwned := true
	defer func() {
		if generationOwned {
			_ = os.RemoveAll(sessionDir)
		}
	}()
	manifest := filepath.Join(sessionDir, "index.m3u8")
	segmentPattern := filepath.Join(sessionDir, "segment_%05d.ts")
	segmentType := "mpegts"
	playlistType := "vod"
	liveInput := item.Type == "live_channel"
	if liveInput {
		playlistType = ""
	}
	if directStreamRemux && directStreamRemuxUsesFragmentedMP4(item) {
		segmentPattern = filepath.Join(sessionDir, "segment_%05d.m4s")
		segmentType = "fmp4"
	}
	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-protocol_whitelist", transcodeProtocolWhitelist(sourcePath),
		"-fflags", "+genpts",
	}
	if startSeconds > 0 {
		args = append(args, "-ss", strconv.Itoa(startSeconds))
	}
	startSegmentIndex := startSeconds / hlsSegmentSeconds
	if liveInput {
		// A restarted live input must not reuse segment identities from its prior
		// generation. Wall-clock-derived media sequence numbers remain monotonic
		// across clean provider disconnects and FFmpeg recovery.
		startSegmentIndex = int(time.Now().UTC().Unix() / int64(hlsSegmentSeconds))
	}
	resolvedHardwareDevice := s.resolvedHardwareDevice(settings)
	method := "software"
	if directStreamRemux {
		method = "direct-stream-remux"
		if forceAudioTranscode {
			method = "direct-stream-audio-transcode"
		}
	} else if settings.HardwareAcceleration {
		if hardwareDecoder := hardwareAccelValue(resolvedHardwareDevice); hardwareDecoder != "" {
			args = append(args, "-hwaccel", hardwareDecoder)
			method = "hardware-decode"
		}
	}
	videoEncoder := "libx264"
	if directStreamRemux {
		videoEncoder = "copy"
	} else if settings.HardwareEncoding {
		if encoder := hardwareVideoEncoder(resolvedHardwareDevice); encoder != "" {
			if s.hardwareEncodeSlotAvailableLocked(settings) {
				videoEncoder = encoder
				method = "hardware-encode"
			}
		}
	}
	if videoEncoder == "libx264" && !s.softwareEncodeSlotAvailableLocked(settings) {
		cancel()
		return nil, errors.New("the software transcode session limit has been reached")
	}
	burnIn, err := s.subtitleBurnInForTranscode(item, sourcePath, subtitleID)
	if err != nil {
		return nil, err
	}
	filter := transcodeVideoFilter(preset, item, settings)
	if liveInput {
		if userAgent := safeTranscodeInputUserAgent(item.SourceUserAgent); userAgent != "" {
			args = append(args, "-user_agent", userAgent)
		}
	}
	args = append(args, "-i", sourcePath)
	if directStreamRemux {
		args = append(args, "-map", "0:v:0?", "-map", audioMap)
		filter = "video copy"
	} else if burnIn != nil && burnIn.imageBased {
		args = append(args,
			"-filter_complex", fmt.Sprintf("[0:v:0][0:%d]overlay,%s[vout]", burnIn.streamIndex, filter),
			"-map", "[vout]", "-map", audioMap,
		)
	} else {
		if burnIn != nil {
			filter = strings.Join([]string{filter, burnIn.videoFilter(sourcePath)}, ",")
		}
		args = append(args, "-map", "0:v:0?", "-map", audioMap)
	}
	args = append(args, "-c:v", videoEncoder)
	if directStreamRemux {
		if segmentType == "mpegts" {
			args = append(args, "-bsf:v", "h264_mp4toannexb")
		}
		if videoTag := directStreamRemuxVideoTag(item); videoTag != "" {
			args = append(args, "-tag:v", videoTag)
		}
	} else {
		args = append(args, videoEncodingArgs(videoEncoder, preset, settings)...)
	}
	if !directStreamRemux && (burnIn == nil || !burnIn.imageBased) {
		args = append(args, "-vf", filter)
	}
	args = append(args, hlsAudioEncodingArgs(item, preset, forceAudioTranscode, audioStreamID)...)
	args = append(args, normalizedHLSOutputTimestampArgs(startSeconds)...)
	hlsListSize := "0"
	hlsFlags := "independent_segments+temp_file"
	if liveInput {
		hlsListSize = "12"
		hlsFlags = "independent_segments+temp_file+delete_segments+omit_endlist+program_date_time"
	}
	args = append(args,
		"-f", "hls",
		"-hls_time", strconv.Itoa(hlsSegmentSeconds),
		"-hls_list_size", hlsListSize,
		"-start_number", strconv.Itoa(startSegmentIndex),
		"-hls_flags", hlsFlags,
	)
	if playlistType != "" {
		args = append(args, "-hls_playlist_type", playlistType)
	} else {
		args = append(args, "-hls_delete_threshold", "4")
	}
	if segmentOptions := normalizedHLSSegmentOptions(segmentType); segmentOptions != "" {
		args = append(args, "-hls_segment_options", segmentOptions)
	}
	if segmentType == "fmp4" {
		args = append(args,
			"-hls_segment_type", "fmp4",
			"-hls_fmp4_init_filename", "init.mp4",
		)
	}
	args = append(args,
		"-hls_segment_filename", segmentPattern,
		manifest,
	)
	cmd := exec.CommandContext(ctx, s.cfg.FFmpegPath, args...)
	cmd.Dir = sessionDir
	if stdin != nil {
		cmd.Stdin = stdin
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	diagnosticRecorder := newFFmpegDiagnosticRecorder(s.cfg.FFmpegPath, args)
	cmd.Stderr = diagnosticRecorder
	displayFilter := redactedTranscodeFilter(filter)
	throttleBufferSeconds := settings.ThrottleBufferSeconds
	if liveInput || directStreamRemux && segmentType == "fmp4" {
		throttleBufferSeconds = 0
	}
	session := &transcodeSession{key: transcodeSessionKey(item.ID, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream), userID: strings.TrimSpace(userID), mediaID: item.ID, quality: quality, subtitleID: subtitleID, audioMode: audioMode, audioStreamID: audioStreamID, directStream: directStream, start: startSeconds, method: method, filter: displayFilter, root: outputRoot, dir: sessionDir, manifest: manifest, startedAt: time.Now().UTC(), background: background, live: liveInput, cmd: cmd, cancel: cancel, done: make(chan struct{}), updateCh: make(chan struct{}), segmentSeconds: hlsSegmentSeconds, playedRetentionSeconds: settings.PlayedRetentionSeconds, throttleBufferSeconds: throttleBufferSeconds, lastServedSegment: -1, lastProducedSegment: startSegmentIndex - 1, lastProducedAt: time.Now().UTC(), resourceRelease: resourceRelease, inputTransport: inputTransport}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	generationOwned = false
	cancelOnError = false
	stdin = nil
	go monitorTranscodeBuffer(session)
	go func() {
		defer close(session.done)
		defer session.releaseMediaResources()
		if inputTransport != nil {
			defer inputTransport.Close()
		}
		err := cmd.Wait()
		cancel()
		if input, ok := cmd.Stdin.(io.Closer); ok {
			_ = input.Close()
		}
		diagnostics := diagnosticRecorder.Report(err)
		diagnosticAPI := diagnostics.API()
		state := session.snapshot()
		session.stateMu.Lock()
		session.ffmpegDiagnostics = diagnosticAPI
		session.stderr = diagnostics.Text
		session.stateMu.Unlock()
		if !state.stopped && (err != nil || session.live) {
			if err == nil {
				err = errors.New("Live TV provider stream ended")
			}
			session.stateMu.Lock()
			session.err = err
			session.errAt = time.Now().UTC()
			stderrText := session.stderr
			session.signalUpdateLocked()
			session.stateMu.Unlock()
			if stderrText != "" {
				_ = os.WriteFile(filepath.Join(session.dir, "ffmpeg.stderr.log"), []byte(stderrText), 0o600)
			}
		}
		if diagnosticAPI != nil {
			s.updatePlaybackTranscodeDiagnostics(item.ID, PlaybackDiagnostics{FFmpeg: diagnosticAPI})
		}
	}()
	transcodeDiagnostics := PlaybackDiagnostics{
		TranscodeQuality: quality,
		TranscodeMethod:  method,
		TranscodeFilter:  displayFilter,
		FFmpegContext:    redactedFFmpegContext(args),
	}
	if burnIn != nil {
		transcodeDiagnostics.SubtitleBurnIn = true
		transcodeDiagnostics.SubtitleBurnInReason = subtitleBurnInReason(burnIn)
	}
	s.updatePlaybackTranscodeDiagnostics(item.ID, transcodeDiagnostics)
	logFields := map[string]string{"media": item.Title, "quality": quality, "method": method, "filter": displayFilter}
	if background {
		logFields["intent"] = "background-prewarm"
	}
	if burnIn != nil {
		logFields["subtitle"] = burnIn.streamID
	}
	s.recordLog("info", "Transcode session started", logFields)
	return session, nil
}

func publishPlannedTranscodeProgress(session *transcodeSession, highest int, at time.Time) error {
	if session == nil || session.supervisor == nil || session.supervised == nil {
		return nil
	}
	if err := session.supervisor.Progress(*session.supervised, at); err != nil {
		return fmt.Errorf("publish transcode progress: %w", err)
	}
	if err := session.supervisor.ManifestReady(*session.supervised, uint64(max(0, session.generation))); err != nil {
		return fmt.Errorf("publish transcode manifest: %w", err)
	}
	if err := session.supervisor.SegmentReady(*session.supervised, uint64(highest)); err != nil {
		return fmt.Errorf("publish transcode segment: %w", err)
	}
	return nil
}

func monitorTranscodeBuffer(session *transcodeSession) {
	if session == nil {
		return
	}
	target := max(hlsSegmentSeconds*2, session.throttleBufferSeconds)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	nextDiskCheck := time.Now().Add(10 * time.Second)
	for {
		select {
		case <-session.done:
			return
		case <-ticker.C:
		}
		if !session.isRunning() {
			return
		}
		if time.Now().After(nextDiskCheck) {
			nextDiskCheck = time.Now().Add(10 * time.Second)
			if err := ensureMediaWriteCapacity(session.dir, mediaWriteMinimumFreeBytes); err != nil {
				session.markFailure(err, true)
				session.stop(1500 * time.Millisecond)
				return
			}
		}
		if highest, ok := transcodeManifestHighestSegmentIndex(session.manifest); ok {
			session.stateMu.Lock()
			publishProgress := false
			var producedAt time.Time
			if highest > session.lastProducedSegment {
				session.lastProducedSegment = highest
				session.lastProducedAt = time.Now().UTC()
				producedAt = session.lastProducedAt
				session.signalUpdateLocked()
				if session.storageLease != nil {
					session.storageLease.Progress()
				}
				publishProgress = session.supervisor != nil && session.supervised != nil
			}
			lastProducedAt := session.lastProducedAt
			session.stateMu.Unlock()
			if publishProgress {
				if err := publishPlannedTranscodeProgress(session, highest, producedAt); err != nil {
					session.markFailure(err, true)
					session.requestStop()
					return
				}
			}
			if time.Since(lastProducedAt) < transcodeStallTimeout(session) {
				if session.throttleBufferSeconds > 0 && transcodeBufferedAheadSeconds(session) >= target {
					session.stateMu.Lock()
					session.throttled = true
					session.signalUpdateLocked()
					session.stateMu.Unlock()
					session.stop(1500 * time.Millisecond)
					return
				}
				continue
			}
		}
		if time.Since(session.snapshot().lastProducedAt) >= transcodeStallTimeout(session) {
			session.markFailure(errors.New("FFmpeg stopped producing HLS segments"), false)
			session.stop(1500 * time.Millisecond)
			return
		}
	}
}

func transcodeStallTimeout(session *transcodeSession) time.Duration {
	if session == nil {
		return 60 * time.Second
	}
	switch session.method {
	case "software", "hardware-decode", "hardware-encode":
		return 90 * time.Second
	case "direct-stream-remux", "direct-stream-audio-transcode":
		return 45 * time.Second
	default:
		return 60 * time.Second
	}
}

func transcodeBufferedAheadSeconds(session *transcodeSession) int {
	if session == nil {
		return 0
	}
	bufferSeconds, _ := transcodeManifestBuffer(session.manifest)
	lastServedSegment := session.servedSegment()
	if lastServedSegment < 0 {
		return bufferSeconds
	}
	highest, ok := transcodeManifestHighestSegmentIndex(session.manifest)
	if !ok || highest <= lastServedSegment {
		return 0
	}
	return (highest - lastServedSegment) * max(1, session.segmentSeconds)
}

func transcodeManifestHighestSegmentIndex(path string) (int, bool) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	highest := -1
	for _, line := range strings.Split(string(bytes), "\n") {
		index, ok := transcodeSegmentIndex(strings.TrimSpace(line))
		if ok && index > highest {
			highest = index
		}
	}
	return highest, highest >= 0
}

func normalizedHLSOutputTimestampArgs(startSeconds int) []string {
	return []string{
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-output_ts_offset", strconv.Itoa(max(0, startSeconds)),
	}
}

func normalizedHLSSegmentOptions(segmentType string) string {
	switch segmentType {
	case "mpegts":
		return "mpegts_copyts=0"
	case "fmp4":
		return "movflags=+frag_keyframe+empty_moov+default_base_moof:use_editlist=0"
	default:
		return ""
	}
}

func redactedFFmpegContext(args []string) string {
	redacted := make([]string, 0, len(args)+1)
	redacted = append(redacted, "ffmpeg")
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-i":
			redacted = append(redacted, arg)
			if i+1 < len(args) {
				redacted = append(redacted, "<media-source>")
				i++
			}
		case "-hls_segment_filename":
			redacted = append(redacted, arg)
			if i+1 < len(args) {
				redacted = append(redacted, "<transcode-segment>")
				i++
			}
		case "-user_agent":
			redacted = append(redacted, arg)
			if i+1 < len(args) {
				redacted = append(redacted, "<provider-user-agent>")
				i++
			}
		case "-headers", "-cookies", "-http_proxy", "-https_proxy", "-referer":
			redacted = append(redacted, arg)
			if i+1 < len(args) {
				redacted = append(redacted, "<provider-transport-redacted>")
				i++
			}
		default:
			if strings.Contains(arg, "subtitles=filename=") {
				redacted = append(redacted, redactedTranscodeFilter(arg))
			} else if strings.Contains(arg, string(filepath.Separator)) && (strings.HasSuffix(arg, ".m3u8") || strings.HasSuffix(arg, ".ts") || strings.HasSuffix(arg, ".m4s") || strings.HasSuffix(arg, ".mp4") || strings.HasSuffix(arg, ".vtt") || strings.HasSuffix(arg, ".srt") || strings.HasSuffix(arg, ".ass")) {
				redacted = append(redacted, "<transcode-file>")
			} else {
				redacted = append(redacted, arg)
			}
		}
	}
	return strings.Join(redacted, " ")
}

func transcodeProtocolWhitelist(sourcePath string) string {
	parsed, err := url.Parse(strings.TrimSpace(sourcePath))
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return "file,pipe,http,https,tcp,tls,crypto"
	}
	return "file,pipe"
}

func safeTranscodeInputUserAgent(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", ""))
	if len(value) > 512 {
		value = value[:512]
	}
	return value
}

func redactedTranscodeFilter(filter string) string {
	const marker = "subtitles=filename="
	idx := strings.Index(filter, marker)
	if idx < 0 {
		return filter
	}
	suffix := ""
	after := filter[idx+len(marker):]
	if streamIdx := strings.Index(after, ":stream_index="); streamIdx >= 0 {
		suffix = after[streamIdx:]
	}
	return filter[:idx] + marker + "<subtitle>" + suffix
}

func subtitleBurnInReason(burnIn *subtitleBurnInSpec) string {
	if burnIn == nil {
		return ""
	}
	if burnIn.imageBased {
		return "Image-based subtitle overlay"
	}
	if burnIn.external {
		return "External subtitle burn-in"
	}
	return "Embedded subtitle burn-in"
}

func (session *transcodeSession) isRunning() bool {
	if session == nil || session.done == nil {
		return false
	}
	select {
	case <-session.done:
		return false
	default:
		state := session.snapshot()
		if session.supervisor != nil && session.supervised != nil {
			return !state.stopped && state.err == nil
		}
		if state.admissionActive {
			return !state.stopped && state.err == nil
		}
		return session.cmd != nil && session.cmd.Process != nil && !state.stopped && state.err == nil
	}
}

func (session *transcodeSession) completedSuccessfully() bool {
	if session == nil || session.done == nil {
		return false
	}
	state := session.snapshot()
	if state.stopped || state.err != nil {
		return false
	}
	select {
	case <-session.done:
	default:
		return false
	}
	manifest := strings.TrimSpace(session.manifest)
	if manifest == "" {
		return false
	}
	info, err := os.Stat(manifest)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func (session *transcodeSession) recoverableFailure() bool {
	state := session.snapshot()
	return session != nil && state.err != nil && state.terminalErr == nil && !state.throttled
}

func (session *transcodeSession) servedSegment() int {
	if session == nil {
		return -1
	}
	session.cleanupMu.Lock()
	defer session.cleanupMu.Unlock()
	return session.lastServedSegment
}

func (session *transcodeSession) acquireReader() (func(), bool) {
	if session == nil {
		return nil, false
	}
	session.readerMu.Lock()
	if session.retiring {
		session.readerMu.Unlock()
		return nil, false
	}
	session.readers++
	supervisor := session.supervisor
	supervised := session.supervised
	session.readerMu.Unlock()
	if supervisor != nil && supervised != nil {
		_ = supervisor.ClientActivity(*supervised, 1, time.Now().UTC())
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			session.readerMu.Lock()
			session.readers = max(0, session.readers-1)
			if session.retiring && session.readers == 0 && session.readersDone != nil {
				close(session.readersDone)
				session.readersDone = nil
			}
			session.readerMu.Unlock()
			if supervisor != nil && supervised != nil {
				_ = supervisor.ClientActivity(*supervised, -1, time.Now().UTC())
			}
		})
	}, true
}

func (session *transcodeSession) retireAndWait(timeout time.Duration) bool {
	if session == nil {
		return true
	}
	session.readerMu.Lock()
	session.retiring = true
	if session.readers == 0 {
		session.readerMu.Unlock()
		return true
	}
	if session.readersDone == nil {
		session.readersDone = make(chan struct{})
	}
	done := session.readersDone
	session.readerMu.Unlock()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func (session *transcodeSession) stop(grace time.Duration) {
	_ = session.stopAndWait(grace)
}

func (session *transcodeSession) stopAndWait(grace time.Duration) error {
	if session == nil {
		return nil
	}
	session.requestStop()
	if session.done == nil {
		return nil
	}
	if session.supervisor != nil && session.supervised != nil {
		_ = session.supervisor.Stop(*session.supervised)
		timer := time.NewTimer(grace + transcodeRecoveryKillWait)
		defer timer.Stop()
		select {
		case <-session.done:
			return nil
		case <-timer.C:
			return errors.New("supervised transcode process did not exit after stop")
		}
	}
	if session.cmd == nil || session.cmd.Process == nil {
		return nil
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-session.done:
		return nil
	case <-timer.C:
		_ = session.cmd.Process.Kill()
	}
	killTimer := time.NewTimer(transcodeRecoveryKillWait)
	defer killTimer.Stop()
	select {
	case <-session.done:
		return nil
	case <-killTimer.C:
		return errors.New("transcode process did not exit after kill")
	}
}

func (session *transcodeSession) requestStop() {
	if session == nil {
		return
	}
	session.stateMu.Lock()
	session.stopped = true
	cancel := session.cancel
	cmd := session.cmd
	supervisor := session.supervisor
	supervised := session.supervised
	session.signalUpdateLocked()
	session.stateMu.Unlock()
	if supervisor != nil && supervised != nil {
		_ = supervisor.Stop(*supervised)
		return
	}
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
	}
}

func (s *Server) stopTranscodeSessionForMedia(mediaID string) bool {
	s.transcodeMu.Lock()
	var sessions []*transcodeSession
	for key, session := range s.transcodes {
		if session != nil && session.mediaID == mediaID {
			sessions = append(sessions, session)
			delete(s.transcodes, key)
		}
	}
	s.transcodeMu.Unlock()
	s.stopTranscodeSessions(sessions)
	return len(sessions) > 0
}

func (s *Server) stopTranscodeSessions(sessions []*transcodeSession) {
	for _, session := range sessions {
		session.stop(1500 * time.Millisecond)
		if err := cleanupTranscodeSessionFiles(session); err != nil {
			s.recordLog("warn", "Transcode temp cleanup failed", map[string]string{"mediaId": session.mediaID, "quality": session.quality, "error": err.Error()})
		}
		s.recordLog("info", "Transcode session stopped", map[string]string{"mediaId": session.mediaID, "quality": session.quality})
	}
}

func (s *Server) shutdownLegacyTranscodes(ctx context.Context) error {
	s.transcodeMu.Lock()
	sessions := make([]*transcodeSession, 0, len(s.transcodes))
	for key, session := range s.transcodes {
		if session != nil && session.supervisor == nil {
			sessions = append(sessions, session)
			delete(s.transcodes, key)
		}
	}
	s.transcodeMu.Unlock()
	if len(sessions) == 0 {
		return nil
	}
	for _, session := range sessions {
		session.requestStop()
	}
	type stopResult struct {
		session *transcodeSession
		err     error
	}
	results := make(chan stopResult, len(sessions))
	for _, session := range sessions {
		go func(session *transcodeSession) {
			results <- stopResult{session: session, err: session.stopAndWait(1500 * time.Millisecond)}
		}(session)
	}
	var joined error
	for range sessions {
		select {
		case result := <-results:
			_ = cleanupTranscodeSessionFiles(result.session)
			joined = errors.Join(joined, result.err)
		case <-ctx.Done():
			return errors.Join(ctx.Err(), joined)
		}
	}
	return joined
}

func (s *Server) noteTranscodeSegmentServed(session *transcodeSession, name string) {
	session.noteRecoveryProgress()
	removed, err := cleanupPlayedTranscodeSegments(session, name)
	if err != nil {
		s.recordLog("warn", "Transcode rolling cleanup failed", map[string]string{"mediaId": session.mediaID, "quality": session.quality, "error": err.Error()})
		return
	}
	if removed > 0 {
		s.recordLog("debug", "Transcode played segment cleanup completed", map[string]string{"mediaId": session.mediaID, "quality": session.quality, "removed": strconv.Itoa(removed)})
	}
}

func (session *transcodeSession) noteRecoveryProgress() {
	if session == nil {
		return
	}
	session.stateMu.Lock()
	defer session.stateMu.Unlock()
	if session.recoveryAttempts <= 0 {
		return
	}
	session.segmentsAfterRecovery++
	if session.segmentsAfterRecovery >= transcodeRecoveryHealthySegments || (!session.lastRecoveryAt.IsZero() && time.Since(session.lastRecoveryAt) >= transcodeRecoveryHealthyDuration) {
		session.recoveryAttempts = 0
		session.segmentsAfterRecovery = 0
		session.lastRecoveryAt = time.Time{}
	}
	session.signalUpdateLocked()
}

func cleanupPlayedTranscodeSegments(session *transcodeSession, currentName string) (int, error) {
	if session == nil {
		return 0, nil
	}
	currentIndex, ok := transcodeSegmentIndex(currentName)
	if !ok {
		return 0, nil
	}
	session.cleanupMu.Lock()
	defer session.cleanupMu.Unlock()
	if currentIndex <= session.lastServedSegment {
		return 0, nil
	}
	session.lastServedSegment = currentIndex
	dir := strings.TrimSpace(session.dir)
	if dir == "" {
		return 0, nil
	}
	segmentSeconds := max(1, session.segmentSeconds)
	retentionSeconds := max(0, session.playedRetentionSeconds)
	retainedSegments := int(math.Ceil(float64(retentionSeconds) / float64(segmentSeconds)))
	deleteBefore := currentIndex - retainedSegments
	if deleteBefore <= 0 {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		segmentIndex, ok := transcodeSegmentIndex(entry.Name())
		if !ok || segmentIndex >= deleteBefore {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func transcodeSegmentIndex(name string) (int, bool) {
	base := filepath.Base(strings.TrimSpace(name))
	if !validHLSSegmentName(base) || base == "init.mp4" || !strings.HasPrefix(base, "segment_") {
		return 0, false
	}
	value := strings.TrimPrefix(base, "segment_")
	value = strings.TrimSuffix(value, filepath.Ext(value))
	index, err := strconv.Atoi(value)
	return index, err == nil && index >= 0
}

func cleanupTranscodeSessionFiles(session *transcodeSession) error {
	if session == nil {
		return nil
	}
	dir := strings.TrimSpace(session.dir)
	root := strings.TrimSpace(session.root)
	if dir == "" || root == "" {
		return nil
	}
	if !session.retireAndWait(transcodeReaderDrainTimeout) {
		return errors.New("transcode generation is still serving a reader")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if !pathInsideRoot(absDir, absRoot) {
		return fmt.Errorf("refusing to remove transcode directory outside root: %s", absDir)
	}
	if err := os.RemoveAll(absDir); err != nil {
		return err
	}
	for parent := filepath.Dir(absDir); pathInsideRoot(parent, absRoot); parent = filepath.Dir(parent) {
		if err := os.Remove(parent); err != nil {
			break
		}
	}
	return nil
}

func (s *Server) pruneOrphanedTranscodeGenerations(olderThan time.Duration) (int, error) {
	configuredRoot := strings.TrimSpace(s.transcodeSettings().TemporaryDirectory)
	if configuredRoot == "" {
		configuredRoot = strings.TrimSpace(s.cfg.TranscodeDir)
	}
	if configuredRoot == "" {
		return 0, nil
	}
	root, err := filepath.Abs(filepath.Clean(configuredRoot))
	if err != nil {
		return 0, err
	}
	active := map[string]struct{}{}
	s.transcodeMu.Lock()
	for _, session := range s.transcodes {
		if session == nil {
			continue
		}
		if absolute, err := filepath.Abs(session.dir); err == nil {
			active[absolute] = struct{}{}
		}
	}
	s.transcodeMu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	plannedRoot := filepath.Join(root, "planned-v2")
	removed := 0
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if !entry.IsDir() || path == root {
			return nil
		}
		name := entry.Name()
		plannedGeneration := filepath.Clean(filepath.Dir(path)) == filepath.Clean(plannedRoot) && isPlannedVODGenerationDirectory(name)
		if !isOrphanedTranscodeGenerationDirectory(name) && !plannedGeneration {
			return nil
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if _, ok := active[absolute]; ok {
			return filepath.SkipDir
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(cutoff) {
			return filepath.SkipDir
		}
		// The initial active-directory snapshot avoids most work, but a
		// generation can be registered after the walk starts. Recheck and
		// remove while holding the registry lock so pruning cannot delete a
		// namespace between its active check and removal.
		s.transcodeMu.Lock()
		activeNow := false
		for _, session := range s.transcodes {
			if session == nil {
				continue
			}
			if sessionDir, dirErr := filepath.Abs(session.dir); dirErr == nil && sessionDir == absolute {
				activeNow = true
				break
			}
		}
		if !activeNow {
			err = os.RemoveAll(path)
		}
		s.transcodeMu.Unlock()
		if activeNow {
			return filepath.SkipDir
		}
		if err != nil {
			return err
		}
		removed++
		return filepath.SkipDir
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	return removed, err
}

func isOrphanedTranscodeGenerationDirectory(name string) bool {
	return strings.HasPrefix(name, ".generation-") || strings.Contains(name, ".failed-") || strings.Contains(name, ".retired-")
}

func isPlannedVODGenerationDirectory(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\\`) {
		return false
	}
	withoutIdentity := name
	if marker := strings.LastIndex(withoutIdentity, "-i"); marker >= 0 && len(withoutIdentity)-marker-2 >= 16 {
		withoutIdentity = withoutIdentity[:marker]
	}
	digestMarker := strings.LastIndex(withoutIdentity, "-")
	if digestMarker <= 0 || digestMarker == len(withoutIdentity)-1 {
		return false
	}
	withoutDigest := withoutIdentity[:digestMarker]
	startMarker := strings.LastIndex(withoutDigest, "-s")
	if startMarker <= 0 || startMarker == len(withoutDigest)-2 {
		return false
	}
	start, err := strconv.Atoi(withoutDigest[startMarker+2:])
	if err != nil || start < 0 {
		return false
	}
	withoutStart := withoutDigest[:startMarker]
	generationMarker := strings.LastIndex(withoutStart, "-g")
	if generationMarker <= 0 || generationMarker == len(withoutStart)-2 {
		return false
	}
	generation, err := strconv.Atoi(withoutStart[generationMarker+2:])
	if err != nil || generation < 1 {
		return false
	}
	mediaKey := withoutStart[:generationMarker]
	return mediaKey != "" && safeExecutionPathToken(mediaKey) == mediaKey
}

func (s *Server) attachActiveTranscodeDetails(sessions []PlaybackSession) {
	if len(sessions) == 0 {
		return
	}
	s.transcodeMu.Lock()
	active := map[string][]*transcodeSession{}
	for _, session := range s.transcodes {
		if session == nil || !session.isRunning() {
			continue
		}
		active[session.mediaID] = append(active[session.mediaID], session)
	}
	s.transcodeMu.Unlock()
	for i := range sessions {
		if !strings.EqualFold(sessions[i].Decision, "Transcode") {
			continue
		}
		candidates := active[sessions[i].Media.ID]
		if len(candidates) == 0 {
			continue
		}
		session := candidates[0]
		if len(candidates) > 1 {
			for _, candidate := range candidates[1:] {
				if candidate.startedAt.After(session.startedAt) {
					session = candidate
				}
			}
		}
		detail := transcodeSessionRuntimeDetail(session, sessions[i])
		sessions[i].Transcode = &detail
	}
}

func transcodeSessionRuntimeDetail(session *transcodeSession, playback PlaybackSession) TranscodeSession {
	bufferSeconds, segmentCount := transcodeManifestBuffer(session.manifest)
	speedMultiplier := transcodeSessionSpeedMultiplier(session, time.Now().UTC())
	return TranscodeSession{
		ID:              session.key,
		Title:           playback.Media.Title,
		Source:          playback.VideoSource,
		Target:          playback.VideoTarget,
		Speed:           playbackSessionStatusLabel(playback),
		SpeedMultiplier: speedMultiplier,
		Progress:        playback.Progress,
		Device:          playback.Device,
		Quality:         session.quality,
		Method:          session.method,
		Filter:          session.filter,
		StartedAt:       session.startedAt.Format(time.RFC3339),
		BufferSeconds:   bufferSeconds,
		SegmentCount:    segmentCount,
	}
}

func transcodeSessionSpeedMultiplier(session *transcodeSession, now time.Time) float64 {
	if session == nil || session.startedAt.IsZero() {
		return 0
	}
	lastProducedSegment := session.snapshot().lastProducedSegment
	if lastProducedSegment < 0 {
		return 0
	}
	elapsed := now.Sub(session.startedAt).Seconds()
	if elapsed <= 0 {
		return 0
	}
	segmentSeconds := max(1, session.segmentSeconds)
	startIndex := session.start / segmentSeconds
	producedSegments := lastProducedSegment - startIndex + 1
	if producedSegments <= 0 {
		return 0
	}
	speed := (float64(producedSegments) * float64(segmentSeconds)) / elapsed
	if !isFinitePositive(speed) {
		return 0
	}
	return math.Round(speed*10) / 10
}

func isFinitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}

func transcodeManifestBuffer(path string) (int, int) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	return transcodeManifestBufferText(string(bytes))
}

func transcodeManifestBufferText(manifest string) (int, int) {
	total := 0.0
	segments := 0
	for _, line := range strings.Split(manifest, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#EXTINF:") {
			continue
		}
		value := strings.TrimPrefix(line, "#EXTINF:")
		if comma := strings.Index(value, ","); comma >= 0 {
			value = value[:comma]
		}
		seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || seconds <= 0 {
			continue
		}
		total += seconds
		segments++
	}
	return int(math.Round(total)), segments
}

func (session *transcodeSession) transcodeError() error {
	if session == nil {
		return nil
	}
	state := session.snapshot()
	if state.err == nil && state.terminalErr == nil {
		return nil
	}
	baseErr := state.err
	if state.terminalErr != nil {
		baseErr = state.terminalErr
	}
	detail := strings.TrimSpace(state.stderr)
	if detail == "" {
		return baseErr
	}
	lines := strings.Split(detail, "\n")
	if len(lines) > 16 {
		summary := append([]string{}, lines[:8]...)
		summary = append(summary, "...")
		summary = append(summary, lines[len(lines)-8:]...)
		lines = summary
	}
	return fmt.Errorf("%w: %s", baseErr, strings.Join(lines, "\n"))
}

func (s *Server) readTranscodeManifest(session *transcodeSession, mediaID string, quality string, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, accessToken string) (string, error) {
	return s.readTranscodeManifestContext(context.Background(), session, mediaID, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, accessToken)
}

func (s *Server) readTranscodeManifestContext(ctx context.Context, session *transcodeSession, mediaID string, quality string, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, accessToken string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.Now().Add(transcodeManifestReadTimeout(directStream))
	var lastManifestErr error
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		bytes, err := os.ReadFile(session.manifest)
		if err == nil && len(bytes) > 0 {
			manifest := string(bytes)
			lastManifestErr = validateGeneratedHLSManifest(session, manifest)
			if lastManifestErr == nil && transcodeManifestReadyForPlayback(session, manifest, directStream) {
				return rewriteMediaHLSManifest(mediaID, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, accessToken, manifest), nil
			}
		} else if err != nil {
			lastManifestErr = err
		}
		state := session.snapshot()
		if session.completedSuccessfully() && lastManifestErr != nil {
			return "", lastManifestErr
		}
		if time.Now().After(deadline) {
			if state.err != nil || state.terminalErr != nil {
				return "", session.transcodeError()
			}
			if lastManifestErr != nil {
				return "", lastManifestErr
			}
			return "", errors.New("HLS manifest did not become structurally ready")
		}
		wait := time.Until(deadline)
		if wait > time.Second {
			wait = time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return "", ctx.Err()
		case <-s.shutdownDone():
			if !timer.Stop() {
				<-timer.C
			}
			return "", errLongPollShutdown
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

func validateGeneratedHLSManifest(session *transcodeSession, manifest string) error {
	if session == nil || strings.TrimSpace(session.dir) == "" {
		return errors.New("HLS manifest has no bound output generation")
	}
	lines := strings.Split(manifest, "\n")
	first := ""
	targetDuration := 0.0
	mediaSequence := false
	pendingDuration := 0.0
	segments := 0
	seen := map[string]bool{}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if first == "" {
			first = line
		}
		switch {
		case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"):
			value := strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"))
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil || !isFinitePositive(parsed) {
				return errors.New("HLS manifest has an invalid target duration")
			}
			targetDuration = parsed
		case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
			value := strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"))
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed < 0 {
				return errors.New("HLS manifest has an invalid media sequence")
			}
			mediaSequence = true
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			name, ok := hlsManifestQuotedURI(line)
			if !ok || name != filepath.Base(name) || name != "init.mp4" {
				return errors.New("HLS manifest has an invalid initialization segment")
			}
			if err := validateGeneratedHLSSegmentFile(session.dir, name); err != nil {
				return err
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			if pendingDuration > 0 {
				return errors.New("HLS manifest has consecutive segment durations")
			}
			value := strings.TrimPrefix(line, "#EXTINF:")
			if comma := strings.IndexByte(value, ','); comma >= 0 {
				value = value[:comma]
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil || !isFinitePositive(parsed) {
				return errors.New("HLS manifest has an invalid segment duration")
			}
			pendingDuration = parsed
		case strings.HasPrefix(line, "#"):
			continue
		default:
			if pendingDuration <= 0 || targetDuration <= 0 || pendingDuration > targetDuration+0.001 {
				return errors.New("HLS segment duration exceeds or lacks its target duration")
			}
			if line != filepath.Base(line) {
				return errors.New("HLS manifest contains a non-local segment URI")
			}
			if _, ok := transcodeSegmentIndex(line); !ok || seen[line] {
				return errors.New("HLS manifest contains an invalid or duplicate segment")
			}
			if err := validateGeneratedHLSSegmentFile(session.dir, line); err != nil {
				return err
			}
			seen[line] = true
			segments++
			pendingDuration = 0
		}
	}
	if first != "#EXTM3U" || targetDuration <= 0 || !mediaSequence || segments == 0 || pendingDuration > 0 {
		return errors.New("HLS manifest is structurally incomplete")
	}
	return nil
}

func hlsManifestQuotedURI(line string) (string, bool) {
	const marker = `URI="`
	start := strings.Index(line, marker)
	if start < 0 {
		return "", false
	}
	start += len(marker)
	end := strings.IndexByte(line[start:], '"')
	if end < 0 {
		return "", false
	}
	value := strings.TrimSpace(line[start : start+end])
	return value, value != ""
}

func validateGeneratedHLSSegmentFile(dir, name string) error {
	info, err := os.Stat(filepath.Join(dir, name))
	if err != nil || info.IsDir() || info.Size() <= 0 {
		return errors.New("HLS manifest references a missing or empty generated segment")
	}
	return nil
}

func transcodeManifestReadTimeout(directStream bool) time.Duration {
	if directStream {
		return 14 * time.Second
	}
	return 6 * time.Second
}

func transcodeManifestReadyForPlayback(session *transcodeSession, manifest string, directStream bool) bool {
	if !directStream || session == nil || session.completedSuccessfully() {
		return true
	}
	bufferSeconds, segmentCount := transcodeManifestBufferText(manifest)
	return segmentCount >= 2 && bufferSeconds >= 6
}

func waitForHLSSegmentFile(session *transcodeSession, path string) error {
	return waitForHLSSegmentFileContext(context.Background(), nil, session, path)
}

func waitForHLSSegmentFileContext(ctx context.Context, shutdown <-chan struct{}, session *transcodeSession, path string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if session == nil {
		return os.ErrNotExist
	}
	deadline := time.Now().Add(hlsSegmentWaitTimeout(session))
	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Size() > 0 {
			return nil
		}
		lastErr = err
		state := session.snapshot()
		if state.err != nil || state.terminalErr != nil {
			return session.transcodeError()
		}
		if !session.isRunning() && !session.completedSuccessfully() || time.Now().After(deadline) {
			if lastErr != nil {
				return lastErr
			}
			return os.ErrNotExist
		}
		wait := time.Until(deadline)
		if wait > time.Second {
			wait = time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-shutdown:
			return errLongPollShutdown
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

func hlsSegmentWaitTimeout(session *transcodeSession) time.Duration {
	if session == nil {
		return 12 * time.Second
	}
	switch session.method {
	case "software", "hardware-decode", "hardware-encode":
		return 30 * time.Second
	case "direct-stream-audio-transcode":
		return 20 * time.Second
	case "direct-stream-remux":
		return 12 * time.Second
	default:
		return 18 * time.Second
	}
}

func rewriteMediaHLSManifest(mediaID, quality, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, accessToken, manifest string) string {
	startSeconds = 0
	var out strings.Builder
	for _, line := range strings.Split(manifest, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#EXT-X-MAP:") {
			out.WriteString(rewriteMediaHLSMapLine(mediaID, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, accessToken, line))
			out.WriteByte('\n')
			continue
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			out.WriteString(mediaHLSSegmentRoute(mediaID, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, accessToken, trimmed))
			out.WriteByte('\n')
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

const maximumStaticHLSDurationSeconds = 24 * 60 * 60

func staticHLSegmentCount(duration int) (int, error) {
	if duration < 1 || duration > maximumStaticHLSDurationSeconds {
		return 0, fmt.Errorf("HLS playback requires a duration between 1 second and %d hours", maximumStaticHLSDurationSeconds/(60*60))
	}
	count := (duration + hlsSegmentSeconds - 1) / hlsSegmentSeconds
	if count < 1 || count > maximumStaticHLSDurationSeconds/hlsSegmentSeconds {
		return 0, errors.New("HLS playback duration is outside the supported range")
	}
	return count, nil
}

func (s *Server) buildStaticMediaHLSManifest(item MediaItem, mediaID, quality, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, accessToken string) (string, error) {
	duration := max(1, item.DurationSeconds)
	segmentCount, err := staticHLSegmentCount(duration)
	if err != nil {
		return "", err
	}
	// VOD manifests always describe the complete media timeline. Resume and seek
	// positions are client concerns; segment demand repositions the producer.
	startSeconds = 0
	startIndex := 0
	segmentExtension := ".ts"
	if directStream && directStreamRemuxAvailable(item, quality, s.transcodeSettings(), subtitleID) && directStreamRemuxUsesFragmentedMP4(item) {
		segmentExtension = ".m4s"
	}
	var out strings.Builder
	out.WriteString("#EXTM3U\n")
	if segmentExtension == ".m4s" {
		out.WriteString("#EXT-X-VERSION:7\n")
	} else {
		out.WriteString("#EXT-X-VERSION:3\n")
	}
	out.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", hlsSegmentSeconds))
	out.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", startIndex))
	out.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	out.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")
	if segmentExtension == ".m4s" {
		out.WriteString(rewriteMediaHLSMapLine(mediaID, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, accessToken, `#EXT-X-MAP:URI="init.mp4"`))
		out.WriteByte('\n')
	}
	for index := startIndex; index < segmentCount; index++ {
		remaining := duration - index*hlsSegmentSeconds
		segmentDuration := hlsSegmentSeconds
		if remaining > 0 && remaining < hlsSegmentSeconds {
			segmentDuration = remaining
		}
		out.WriteString(fmt.Sprintf("#EXTINF:%d.000,\n", max(1, segmentDuration)))
		out.WriteString(mediaHLSSegmentRoute(mediaID, quality, subtitleID, 0, audioMode, audioStreamID, directStream, accessToken, fmt.Sprintf("segment_%05d%s", index, segmentExtension)))
		out.WriteByte('\n')
	}
	out.WriteString("#EXT-X-ENDLIST\n")
	return out.String(), nil
}

func buildMediaHLSMasterManifest(item MediaItem, mediaID, quality, textSubtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, accessToken, cacheKey, mediaPlaylist string) string {
	return buildMediaHLSMasterManifestWithBandwidth(item, mediaID, quality, textSubtitleID, startSeconds, audioMode, audioStreamID, directStream, accessToken, cacheKey, mediaPlaylist, hlsBandwidthMeasurement{})
}

type hlsBandwidthMeasurement struct {
	PeakBitsPerSecond    int
	AverageBitsPerSecond int
}

func measureGeneratedHLSBandwidth(session *transcodeSession) hlsBandwidthMeasurement {
	if session == nil || strings.TrimSpace(session.manifest) == "" || strings.TrimSpace(session.dir) == "" {
		return hlsBandwidthMeasurement{}
	}
	raw, err := os.ReadFile(session.manifest)
	if err != nil {
		return hlsBandwidthMeasurement{}
	}
	pendingDuration := 0.0
	totalDuration := 0.0
	totalBits := float64(0)
	peak := float64(0)
	for _, rawLine := range strings.Split(string(raw), "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "#EXTINF:") {
			value := strings.TrimPrefix(line, "#EXTINF:")
			if comma := strings.IndexByte(value, ','); comma >= 0 {
				value = value[:comma]
			}
			pendingDuration, _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") || pendingDuration <= 0 || line != filepath.Base(line) {
			continue
		}
		if _, ok := transcodeSegmentIndex(line); !ok {
			pendingDuration = 0
			continue
		}
		info, statErr := os.Stat(filepath.Join(session.dir, line))
		if statErr != nil || info.IsDir() || info.Size() <= 0 {
			pendingDuration = 0
			continue
		}
		bits := float64(info.Size()) * 8
		rate := bits / pendingDuration
		if rate > peak {
			peak = rate
		}
		totalBits += bits
		totalDuration += pendingDuration
		pendingDuration = 0
	}
	if totalDuration <= 0 || peak <= 0 {
		return hlsBandwidthMeasurement{}
	}
	return hlsBandwidthMeasurement{
		PeakBitsPerSecond:    int(math.Ceil(peak)),
		AverageBitsPerSecond: int(math.Ceil(totalBits / totalDuration)),
	}
}

func buildMediaHLSMasterManifestWithBandwidth(item MediaItem, mediaID, quality, textSubtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, accessToken, cacheKey, mediaPlaylist string, measured hlsBandwidthMeasurement) string {
	startSeconds = 0
	cacheKey = ""
	video := firstStreamOfKind(item.Streams, "video")
	audio := selectedAudioStreamOrDefault(item, audioStreamID)
	subtitle := streamByID(item.Streams, textSubtitleID)
	name := firstNonEmpty(subtitle.DisplayTitle, subtitle.Language, "Subtitles")
	language := strings.TrimSpace(subtitle.Language)
	averageBandwidth := max(1_000_000, video.Bitrate)
	if audio.Bitrate > 0 {
		averageBandwidth += audio.Bitrate
	}
	if averageBandwidth < 10_000 {
		averageBandwidth *= 1000
	}
	// Source probes expose representative bitrate rather than a measured peak.
	// Advertise a conservative peak envelope instead of mislabelling an encoder
	// target as BANDWIDTH; finalized output probing can tighten it later.
	bandwidth := averageBandwidth + max(1, averageBandwidth/10)
	if measured.AverageBitsPerSecond > 0 && measured.PeakBitsPerSecond >= measured.AverageBitsPerSecond {
		averageBandwidth = measured.AverageBitsPerSecond
		bandwidth = measured.PeakBitsPerSecond
	}
	videoCodec := video.Codec
	videoRange := hlsVideoRange(video)
	outputWidth, outputHeight := video.Width, video.Height
	if !directStream {
		videoCodec = "h264"
		videoRange = "SDR"
		outputWidth, outputHeight = hlsTranscodeGeometry(video, quality)
	}
	audioCodec := audio.Codec
	if normalizeTranscodeAudioMode(audioMode) == "transcode" && audio.ID != "" {
		audioCodec = "aac"
	}
	codecs := hlsCodecListForStream(video, videoCodec, audioCodec)
	subtitleURI := ""
	if textSubtitleID != "" {
		subtitleURI = mediaHLSSubtitlePlaylistRoute(mediaID, textSubtitleID, startSeconds, accessToken, cacheKey)
	}
	var out strings.Builder
	out.WriteString("#EXTM3U\n")
	out.WriteString("#EXT-X-VERSION:7\n")
	if subtitleURI != "" {
		out.WriteString(`#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="`)
		out.WriteString(hlsQuoteAttribute(name))
		out.WriteString(`",DEFAULT=YES,AUTOSELECT=YES,FORCED=`)
		if subtitle.Forced {
			out.WriteString("YES")
		} else {
			out.WriteString("NO")
		}
		if language != "" {
			out.WriteString(`,LANGUAGE="`)
			out.WriteString(hlsQuoteAttribute(language))
			out.WriteString(`"`)
		}
		out.WriteString(`,URI="`)
		out.WriteString(hlsQuoteAttribute(subtitleURI))
		out.WriteString("\"\n")
	}
	out.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=")
	out.WriteString(strconv.Itoa(bandwidth))
	out.WriteString(",AVERAGE-BANDWIDTH=")
	out.WriteString(strconv.Itoa(averageBandwidth))
	if codecs != "" {
		out.WriteString(`,CODECS="`)
		out.WriteString(hlsQuoteAttribute(codecs))
		out.WriteString(`"`)
	}
	if outputWidth > 0 && outputHeight > 0 {
		out.WriteString(",RESOLUTION=")
		out.WriteString(strconv.Itoa(outputWidth))
		out.WriteString("x")
		out.WriteString(strconv.Itoa(outputHeight))
	}
	if video.FrameRate > 0 {
		out.WriteString(",FRAME-RATE=")
		out.WriteString(strings.TrimRight(strings.TrimRight(strconv.FormatFloat(video.FrameRate, 'f', 3, 64), "0"), "."))
	}
	if videoRange != "" {
		out.WriteString(",VIDEO-RANGE=")
		out.WriteString(videoRange)
	}
	if subtitleURI != "" {
		out.WriteString(`,SUBTITLES="subs"`)
	}
	out.WriteString(",CLOSED-CAPTIONS=NONE")
	out.WriteByte('\n')
	out.WriteString(mediaHLSVariantRoute(mediaID, quality, startSeconds, audioMode, audioStreamID, directStream, accessToken))
	out.WriteByte('\n')
	if !strings.HasSuffix(mediaPlaylist, "\n") {
		out.WriteString("# PORTICO-MEDIA-PLAYLIST-READY\n")
	}
	return out.String()
}

func hlsTranscodeGeometry(video Stream, quality string) (width, height int) {
	width, height = video.Width, video.Height
	preset, ok := transcodePresets[normalizeTranscodeQuality(quality)]
	if !ok || preset.height <= 0 || height <= 0 || width <= 0 || height <= preset.height {
		return width, height
	}
	height = preset.height
	width = int(math.Round(float64(video.Width) * float64(height) / float64(video.Height)))
	// FFmpeg scale=-2 rounds the derived axis to a codec-safe even value.
	if width%2 != 0 {
		width++
	}
	return width, height
}

func hlsCodecList(videoCodec, videoProfile, audioCodec string) string {
	var codecs []string
	switch normalizeCodec(videoCodec) {
	case "h264", "avc1":
		switch strings.ToLower(strings.TrimSpace(videoProfile)) {
		case "baseline", "constrained baseline":
			codecs = append(codecs, "avc1.42E01F")
		case "main":
			codecs = append(codecs, "avc1.4D401F")
		default:
			codecs = append(codecs, "avc1.640028")
		}
	case "hevc", "h265", "hvc1":
		if strings.Contains(strings.ToLower(videoProfile), "10") {
			codecs = append(codecs, "hvc1.2.4.L153.B0")
		} else {
			codecs = append(codecs, "hvc1.1.6.L120.B0")
		}
	case "vp9", "vp09":
		codecs = append(codecs, "vp09.00.41.08")
	case "av1", "av01":
		codecs = append(codecs, "av01.0.08M.08")
	}
	switch normalizeCodec(audioCodec) {
	case "aac":
		codecs = append(codecs, "mp4a.40.2")
	case "ac3":
		codecs = append(codecs, "ac-3")
	case "eac3":
		codecs = append(codecs, "ec-3")
	case "opus":
		codecs = append(codecs, "opus")
	}
	return strings.Join(codecs, ",")
}

func hlsCodecListForStream(video Stream, outputVideoCodec, outputAudioCodec string) string {
	profile, _ := strconv.Atoi(strings.TrimSpace(video.DolbyVisionProfile))
	if profile > 0 && normalizeCodec(outputVideoCodec) == "hevc" {
		if video.HLSSampleEntry != "dvh1" || video.DolbyVisionLevel <= 0 {
			return ""
		}
		videoCodec := fmt.Sprintf("dvh1.%02d.%02d", profile, video.DolbyVisionLevel)
		audioOnly := hlsCodecList("", "", outputAudioCodec)
		if audioOnly != "" {
			return videoCodec + "," + audioOnly
		}
		return videoCodec
	}
	return hlsCodecList(outputVideoCodec, video.Profile, outputAudioCodec)
}

func (s *Server) hlsItemWithVerifiedDolbyVisionEvidence(ctx context.Context, item MediaItem, directStream bool) (MediaItem, error) {
	video := firstStreamOfKind(item.Streams, "video")
	if !directStream || strings.TrimSpace(video.DolbyVisionProfile) == "" {
		return item, nil
	}
	facts, _, err := s.mediaFactsForPlayback(ctx, item)
	if err != nil {
		return MediaItem{}, errors.New("Dolby Vision HLS requires current probed sample-entry and profile evidence")
	}
	for _, fact := range facts.Video {
		if fact.Index != video.Index || fact.DolbyVision == nil {
			continue
		}
		dv := fact.DolbyVision
		if dv.Profile <= 0 || dv.Level <= 0 || !dv.RPUKnown || !dv.RPU || strings.TrimSpace(dv.Evidence) == "" {
			break
		}
		copy := item
		copy.Streams = append([]Stream(nil), item.Streams...)
		for index := range copy.Streams {
			if copy.Streams[index].Kind == "video" && copy.Streams[index].Index == fact.Index {
				copy.Streams[index].DolbyVisionProfile = strconv.Itoa(dv.Profile)
				copy.Streams[index].DolbyVisionLevel = dv.Level
				// The canonical Apple premium HLS remux writes dvh1. The
				// master describes that generated sample entry, never a generic
				// HEVC tag or the provider's input sample entry.
				copy.Streams[index].HLSSampleEntry = "dvh1"
				return copy, nil
			}
		}
	}
	return MediaItem{}, errors.New("Dolby Vision HLS output sample-entry or profile evidence is incomplete")
}

func hlsVideoRange(video Stream) string {
	rangeValue := strings.ToLower(strings.TrimSpace(video.DynamicRange))
	transfer := strings.ToLower(strings.TrimSpace(video.ColorTransfer))
	switch {
	case rangeValue == "hlg" || strings.Contains(transfer, "arib-std-b67") || strings.Contains(transfer, "hlg"):
		return "HLG"
	case rangeValue == "pq", rangeValue == "hdr10", rangeValue == "hdr10plus", rangeValue == "dolby_vision",
		strings.Contains(transfer, "smpte2084") || strings.Contains(transfer, "pq"):
		return "PQ"
	default:
		return "SDR"
	}
}

func rewriteMediaHLSMapLine(mediaID, quality, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, accessToken, line string) string {
	const marker = `URI="`
	start := strings.Index(line, marker)
	if start < 0 {
		return line
	}
	valueStart := start + len(marker)
	valueEnd := strings.Index(line[valueStart:], `"`)
	if valueEnd < 0 {
		return line
	}
	valueEnd += valueStart
	route := mediaHLSSegmentRoute(mediaID, quality, subtitleID, startSeconds, audioMode, audioStreamID, directStream, accessToken, line[valueStart:valueEnd])
	return line[:valueStart] + route + line[valueEnd:]
}

func mediaHLSSegmentRoute(mediaID, quality, subtitleID string, startSeconds int, audioMode string, audioStreamID string, directStream bool, accessToken, name string) string {
	startSeconds = 0
	var out strings.Builder
	out.WriteString("/api/media/")
	out.WriteString(url.PathEscape(mediaID))
	out.WriteString("/hls/segment?quality=")
	out.WriteString(url.QueryEscape(quality))
	if subtitleID = normalizeBurnInSubtitleID(subtitleID); subtitleID != "" {
		out.WriteString("&subtitle=")
		out.WriteString(url.QueryEscape(subtitleID))
	}
	if startSeconds > 0 {
		out.WriteString("&start=")
		out.WriteString(strconv.Itoa(startSeconds))
	}
	if audioMode = normalizeTranscodeAudioMode(audioMode); audioMode != "" {
		out.WriteString("&audio=")
		out.WriteString(url.QueryEscape(audioMode))
	}
	if audioStreamID = normalizeSelectedAudioStreamID(audioStreamID); audioStreamID != "" {
		out.WriteString("&audioStream=")
		out.WriteString(url.QueryEscape(audioStreamID))
	}
	if directStream {
		out.WriteString("&directStream=1")
	}
	out.WriteString("&name=")
	out.WriteString(url.QueryEscape(filepath.Base(name)))
	_ = accessToken
	return out.String()
}

func mediaHLSVariantRoute(mediaID, quality string, startSeconds int, audioMode string, audioStreamID string, directStream bool, accessToken string) string {
	startSeconds = 0
	var out strings.Builder
	out.WriteString("/api/media/")
	out.WriteString(url.PathEscape(mediaID))
	out.WriteString("/hls/variant.m3u8?quality=")
	out.WriteString(url.QueryEscape(quality))
	if startSeconds > 0 {
		out.WriteString("&start=")
		out.WriteString(strconv.Itoa(startSeconds))
	}
	if audioMode = normalizeTranscodeAudioMode(audioMode); audioMode != "" {
		out.WriteString("&audio=")
		out.WriteString(url.QueryEscape(audioMode))
	}
	if audioStreamID = normalizeSelectedAudioStreamID(audioStreamID); audioStreamID != "" {
		out.WriteString("&audioStream=")
		out.WriteString(url.QueryEscape(audioStreamID))
	}
	if directStream {
		out.WriteString("&directStream=1")
	}
	_ = accessToken
	return out.String()
}

func mediaHLSSubtitlePlaylistRoute(mediaID, subtitleID string, startSeconds int, accessToken string, cacheKey string) string {
	startSeconds = 0
	cacheKey = ""
	var out strings.Builder
	out.WriteString("/api/media/")
	out.WriteString(url.PathEscape(mediaID))
	out.WriteString("/hls/subtitles.m3u8?textSubtitle=")
	out.WriteString(url.QueryEscape(subtitleID))
	if startSeconds > 0 {
		out.WriteString("&start=")
		out.WriteString(strconv.Itoa(startSeconds))
	}
	_ = accessToken
	_ = cacheKey
	return out.String()
}

func mediaHLSSubtitleSegmentRoute(mediaID, subtitleID string, segmentIndex int, startSeconds int, accessToken string, cacheKey string) string {
	startSeconds = 0
	cacheKey = ""
	var out strings.Builder
	out.WriteString("/api/media/")
	out.WriteString(url.PathEscape(mediaID))
	out.WriteString("/hls/subtitle.vtt?textSubtitle=")
	out.WriteString(url.QueryEscape(subtitleID))
	out.WriteString("&segment=")
	out.WriteString(strconv.Itoa(max(0, segmentIndex)))
	if startSeconds > 0 {
		out.WriteString("&start=")
		out.WriteString(strconv.Itoa(startSeconds))
	}
	_ = accessToken
	_ = cacheKey
	return out.String()
}

func normalizeHLSTextSubtitleID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "sub_none" || strings.HasPrefix(value, "native_subtitle_") {
		return ""
	}
	return value
}

func hlsTextSubtitleAvailable(item MediaItem, subtitleID string) bool {
	stream := streamByID(item.Streams, subtitleID)
	return stream.ID != "" && stream.Kind == "subtitle" && stream.SourceURL != "" && normalizeCodec(stream.Codec) == "webvtt"
}

func streamByID(streams []Stream, id string) Stream {
	for _, stream := range streams {
		if stream.ID == id {
			return stream
		}
	}
	return Stream{}
}

func hlsQuoteAttribute(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func (s *Server) localSourcePathForTranscode(item MediaItem) (string, error) {
	sourceURL := strings.TrimSpace(item.SourceURL)
	if sourceURL == "" {
		return "", errors.New("no local media source is available for transcoding")
	}
	if item.Type == "dvr_recording" {
		return cleanDVRRecordingFilePath(sourceURL, s.cfg.AppDataDir)
	}
	parsed, err := url.Parse(sourceURL)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return "", errors.New("remote source transcoding is not enabled")
	}
	path := sourceURL
	if err == nil && parsed.Scheme == "file" {
		path = parsed.Path
	} else if err == nil && parsed.Scheme != "" {
		return "", errUnsupportedPlaybackScheme
	}
	return s.validateLocalMediaPath(item.LibraryID, path)
}

func (s *Server) sourcePathForHLSTranscode(item MediaItem) (string, error) {
	sourceURL := strings.TrimSpace(item.SourceURL)
	if sourceURL == "" {
		return "", errors.New("no media source is available for HLS playback")
	}
	parsed, err := url.Parse(sourceURL)
	if err == nil && parsed.Scheme == "portico-storage" {
		sourceID, objectPath, parseErr := parseRemoteStorageLocator(sourceURL)
		if parseErr != nil {
			return "", parseErr
		}
		var exists int
		if queryErr := s.queryBackgroundRow(context.Background(), `SELECT COUNT(*) FROM storage_remote_objects object JOIN storage_sources source ON source.id=object.source_id WHERE object.source_id=? AND object.object_path=? AND object.missing_since='' AND source.library_id=?`, sourceID, objectPath, item.LibraryID).Scan(&exists); queryErr != nil || exists != 1 {
			if queryErr != nil {
				return "", queryErr
			}
			return "", errors.New("remote storage source is unavailable")
		}
		return sourceURL, nil
	}
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		if hlsTranscodeAllowsHDHomeRunLAN(item) {
			validated, err := validateHDHomeRunURL(sourceURL)
			if err != nil {
				return "", err
			}
			return validated.String(), nil
		}
		validated, err := validateExternalURL(sourceURL)
		if err != nil {
			return "", err
		}
		return validated.String(), nil
	}
	return s.localSourcePathForTranscode(item)
}

func hlsTranscodeAllowsHDHomeRunLAN(item MediaItem) bool {
	if item.Type != "live_channel" {
		return false
	}
	for _, label := range item.Labels {
		if strings.EqualFold(strings.TrimSpace(label), "hdhomerun") {
			return true
		}
	}
	return false
}

func (s *Server) validateSubtitleBurnInSelection(item MediaItem, subtitleID string) error {
	subtitleID = normalizeBurnInSubtitleID(subtitleID)
	if subtitleID == "" {
		return nil
	}
	stream := streamByID(item.Streams, subtitleID)
	if stream.ID == "" || stream.Kind != "subtitle" {
		return errSubtitleStreamNotFound
	}
	if strings.TrimSpace(stream.SourceURL) != "" {
		path, _, err := s.subtitleStreamPathAndOffset(item.ID, stream.ID)
		if err != nil {
			return fmt.Errorf("%w: %v", errSubtitleBurnInUnavailable, err)
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("%w: %v", errSubtitleBurnInUnavailable, err)
		}
		return nil
	}
	if _, ok := embeddedSubtitleStreamIndex(stream); !ok {
		return errSubtitleBurnInUnavailable
	}
	return nil
}

func (s *Server) subtitleBurnInForTranscode(item MediaItem, sourcePath string, subtitleID string) (*subtitleBurnInSpec, error) {
	subtitleID = normalizeBurnInSubtitleID(subtitleID)
	if subtitleID == "" {
		return nil, nil
	}
	subtitleOrdinal := 0
	for _, stream := range item.Streams {
		if stream.Kind != "subtitle" {
			continue
		}
		currentOrdinal := subtitleOrdinal
		subtitleOrdinal++
		if stream.ID != subtitleID {
			continue
		}
		spec := &subtitleBurnInSpec{
			streamID:        stream.ID,
			subtitleOrdinal: currentOrdinal,
			codec:           normalizeCodec(stream.Codec),
			imageBased:      isImageSubtitleCodec(stream.Codec),
		}
		if strings.TrimSpace(stream.SourceURL) != "" {
			path, err := s.subtitleStreamPathForBurnIn(item.ID, stream.ID)
			if err != nil {
				return nil, errors.New("subtitle file is not available for burn-in")
			}
			spec.external = true
			spec.path = path
			return spec, nil
		}
		index, ok := embeddedSubtitleStreamIndex(stream)
		if !ok {
			return nil, errors.New("subtitle stream cannot be mapped for burn-in")
		}
		spec.streamIndex = index
		spec.path = sourcePath
		return spec, nil
	}
	return nil, errors.New("subtitle stream was not found")
}

func (spec *subtitleBurnInSpec) videoFilter(sourcePath string) string {
	path := spec.path
	if path == "" {
		path = sourcePath
	}
	filter := "subtitles=filename=" + ffmpegFilterQuote(path)
	if !spec.external && spec.streamIndex >= 0 {
		filter += ":stream_index=" + strconv.Itoa(spec.subtitleOrdinal)
	}
	return filter
}

func embeddedSubtitleStreamIndex(stream Stream) (int, bool) {
	if stream.SourceKind != "ffprobe" || stream.Index < 0 {
		return 0, false
	}
	return stream.Index, true
}

func isImageSubtitleCodec(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "hdmv_pgs_subtitle", "pgs", "dvd_subtitle", "dvdsub", "vobsub", "xsub":
		return true
	default:
		return false
	}
}

func ffmpegFilterQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return "'" + value + "'"
}

func (s *Server) transcodeSettings() transcodeSettings {
	settings, err := s.loadSettings()
	if err != nil {
		return transcodeSettings{Enabled: true, PlanningPolicy: "maximum_fidelity", MaxConcurrentSessions: 2, X264Preset: "veryfast", ThrottleBufferSeconds: 60, PlayedRetentionSeconds: 300, HardwareDecodeHEVC: true, MaxHardwareSessions: 2, MaxSoftwareSessions: 0, MaxBackgroundSessions: 1, HDRToneMapping: true, HDRToneMappingAlgorithm: "hable", DirectStreamRemux: true}
	}
	group, _ := settings["transcoder"].(map[string]any)
	hdrToneMapping := settingBool(group, "hdrToneMapping", true)
	return transcodeSettings{
		Enabled:                 settingBool(group, "enabled", true),
		PlanningPolicy:          strings.ToLower(strings.TrimSpace(settingString(group, "planningPolicy", "maximum_fidelity"))),
		TemporaryDirectory:      settingString(group, "temporaryDirectory", s.cfg.TranscodeDir),
		MaxConcurrentSessions:   max(0, settingInt(group, "maxConcurrentSessions", 2)),
		X264Preset:              safeX264Preset(settingString(group, "x264Preset", "veryfast")),
		ThrottleBufferSeconds:   max(10, settingInt(group, "throttleBufferSeconds", 60)),
		PlayedRetentionSeconds:  max(0, settingInt(group, "playedRetentionSeconds", 300)),
		HardwareAcceleration:    settingBool(group, "hardwareAcceleration", false),
		HardwareEncoding:        settingBool(group, "hardwareEncoding", false),
		HardwareDecodeHEVC:      settingBool(group, "hardwareDecodeHEVC", true),
		HardwareDevice:          settingString(group, "hardwareDevice", "auto"),
		MaxHardwareSessions:     max(0, settingInt(group, "maxHardwareSessions", 2)),
		MaxSoftwareSessions:     max(0, settingInt(group, "maxSoftwareSessions", 0)),
		MaxBackgroundSessions:   max(0, settingInt(group, "maxBackgroundSessions", 1)),
		HDRToneMapping:          hdrToneMapping,
		HDRToneMappingFilters:   hdrToneMapping && s.ffmpegSupportsFilters("zscale", "tonemap"),
		HDRToneMappingAlgorithm: safeToneMappingAlgorithm(settingString(group, "hdrToneMappingAlgorithm", "hable")),
		DirectStreamRemux:       settingBool(group, "directStreamRemux", true),
	}
}

func (s *Server) ffmpegSupportsFilters(filters ...string) bool {
	if len(filters) == 0 {
		return true
	}
	ffmpegPath := firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg")
	if ffmpegPath == "" {
		return false
	}
	return s.cachedFFmpegFiltersAvailable(ffmpegPath, filters...)
}

func (s *Server) optimizedVersionSettings() optimizedVersionSettings {
	settings, err := s.loadSettings()
	if err != nil {
		return optimizedVersionSettings{DefaultProfile: "universal-720p", Templates: defaultOptimizedVersionTemplates()}
	}
	group, _ := settings["optimizedVersions"].(map[string]any)
	return optimizedVersionSettings{
		DefaultProfile:          optimizedDefaultProfile(settingString(group, "defaultProfile", "universal-720p")),
		Templates:               normalizedOptimizedTemplates(group["templates"]),
		PreferOptimizedPlayback: settingBool(group, "preferOptimizedPlayback", false),
		StorageDirectory:        normalizeOptimizedStorageDirectory(settingString(group, "storageDirectory", "")),
		MaxConcurrentJobs:       min(4, max(1, settingInt(group, "maxConcurrentJobs", 1))),
		AutoDelete:              settingBool(group, "autoDelete", false),
		RetentionDays:           max(0, settingInt(group, "retentionDays", 0)),
		MaxPerItem:              max(1, settingInt(group, "maxPerItem", 3)),
		MaxStorageMB:            max(0, settingInt(group, "maxStorageMB", 0)),
	}
}

func defaultOptimizedVersionTemplates() []optimizedVersionTemplate {
	presets := optimized.List()
	result := make([]optimizedVersionTemplate, 0, len(presets))
	for _, preset := range presets {
		result = append(result, optimizedVersionTemplate{ID: "preset-" + preset.ID, Name: preset.Label, Profile: preset.ID, Enabled: true})
	}
	return result
}

// Presets are checked-in product policy rather than user-defined encoder
// graphs. Ignore persisted template mutations and expose the exact registry.
func normalizedOptimizedTemplates(value any) []optimizedVersionTemplate {
	_ = value
	return defaultOptimizedVersionTemplates()
}

func optimizedDefaultProfile(value string) string {
	value = strings.TrimSpace(value)
	if _, ok := optimized.Lookup(value); ok {
		return value
	}
	return "universal-720p"
}

func normalizeOptimizedStorageDirectory(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	clean := filepath.Clean(value)
	if !filepath.IsAbs(clean) {
		return ""
	}
	return clean
}

func hardwareAccelValue(device string) string {
	switch strings.ToLower(strings.TrimSpace(device)) {
	case "videotoolbox":
		return "videotoolbox"
	case "vaapi":
		return "vaapi"
	case "qsv":
		return "qsv"
	case "cuda", "nvenc", "nvidia":
		return "cuda"
	case "amf":
		return ""
	default:
		return "auto"
	}
}

func (s *Server) resolvedHardwareDevice(settings transcodeSettings) string {
	normalized := strings.ToLower(strings.TrimSpace(settings.HardwareDevice))
	switch normalized {
	case "videotoolbox", "vaapi", "qsv", "cuda", "nvenc", "nvidia", "amf":
		if normalized == "nvenc" || normalized == "nvidia" {
			return "cuda"
		}
		return normalized
	case "", "auto":
	default:
		return ""
	}
	ffmpegPath := firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg")
	for _, candidate := range autoHardwareDeviceCandidates() {
		encoder := hardwareVideoEncoder(candidate)
		if encoder != "" && s.cachedFFmpegEncoderAvailable(ffmpegPath, encoder) {
			return candidate
		}
	}
	if runtime.GOOS == "darwin" {
		return "videotoolbox"
	}
	return "auto"
}

func resolveHardwareDevice(device string, ffmpegPath string) string {
	normalized := strings.ToLower(strings.TrimSpace(device))
	switch normalized {
	case "videotoolbox", "vaapi", "qsv", "cuda", "nvenc", "nvidia", "amf":
		if normalized == "nvenc" || normalized == "nvidia" {
			return "cuda"
		}
		return normalized
	case "", "auto":
	default:
		return ""
	}
	for _, candidate := range autoHardwareDeviceCandidates() {
		encoder := hardwareVideoEncoder(candidate)
		if encoder != "" && ffmpegEncoderAvailable(ffmpegPath, encoder) {
			return candidate
		}
	}
	if runtime.GOOS == "darwin" {
		return "videotoolbox"
	}
	return "auto"
}

func autoHardwareDeviceCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"videotoolbox", "qsv", "cuda", "vaapi"}
	case "windows":
		return []string{"qsv", "cuda", "amf"}
	default:
		return []string{"vaapi", "qsv", "cuda"}
	}
}

func hardwareVideoEncoder(device string) string {
	switch strings.ToLower(strings.TrimSpace(device)) {
	case "videotoolbox":
		return "h264_videotoolbox"
	case "qsv":
		return "h264_qsv"
	case "cuda", "nvenc", "nvidia":
		return "h264_nvenc"
	case "vaapi":
		return "h264_vaapi"
	case "amf":
		return "h264_amf"
	default:
		return ""
	}
}

func videoEncodingArgs(videoEncoder string, preset transcodePreset, settings transcodeSettings) []string {
	args := []string{}
	if videoEncoder == "libx264" {
		args = append(args,
			"-preset", safeX264Preset(settings.X264Preset),
			"-crf", strconv.Itoa(preset.crf),
		)
	} else {
		args = append(args, "-b:v", fmt.Sprintf("%dk", preset.videoK))
	}
	args = append(args,
		"-maxrate", fmt.Sprintf("%dk", preset.videoK),
		"-bufsize", fmt.Sprintf("%dk", preset.videoK*2),
		"-pix_fmt", "yuv420p",
		"-colorspace", "bt709",
		"-color_primaries", "bt709",
		"-color_trc", "bt709",
		"-color_range", "tv",
		"-force_key_frames", "expr:gte(t,n_forced*4)",
		"-sc_threshold", "0",
	)
	return args
}

func (s *Server) hardwareEncodeSlotAvailableLocked(settings transcodeSettings) bool {
	capacity := maxHardwareEncodeSessions(settings)
	if capacity <= 0 {
		return true
	}
	active := 0
	for _, session := range s.transcodes {
		if session != nil && session.method == "hardware-encode" && session.isRunning() {
			active++
		}
	}
	return active < capacity
}

func maxHardwareEncodeSessions(settings transcodeSettings) int {
	return settings.MaxHardwareSessions
}

func (s *Server) softwareEncodeSlotAvailableLocked(settings transcodeSettings) bool {
	capacity := maxSoftwareEncodeSessions(settings)
	if capacity <= 0 {
		return true
	}
	active := 0
	for _, session := range s.transcodes {
		if session != nil && session.method != "hardware-encode" && session.isRunning() {
			active++
		}
	}
	return active < capacity
}

func maxSoftwareEncodeSessions(settings transcodeSettings) int {
	return settings.MaxSoftwareSessions
}

func transcodeVideoFilter(preset transcodePreset, item MediaItem, settings transcodeSettings) string {
	scale := fmt.Sprintf("scale=-2:min(%d\\,ih):flags=bicubic", preset.height)
	if settings.HDRToneMapping && mediaHasHDRVideo(item) {
		if !settings.HDRToneMappingFilters {
			return strings.Join([]string{
				scale,
				"setsar=1",
				"format=yuv420p",
			}, ",")
		}
		return strings.Join([]string{
			"zscale=min=bt2020nc:tin=smpte2084:pin=bt2020:t=bt709:m=bt709:p=bt709:r=tv",
			"tonemap=tonemap=" + safeToneMappingAlgorithm(settings.HDRToneMappingAlgorithm) + ":desat=0",
			"zscale=t=bt709:m=bt709:p=bt709:r=tv",
			scale,
			"setsar=1",
			"format=yuv420p",
		}, ",")
	}
	return strings.Join([]string{scale, "setsar=1", "format=yuv420p"}, ",")
}

func mediaHasHDRVideo(item MediaItem) bool {
	for _, stream := range item.Streams {
		if stream.Kind != "video" {
			continue
		}
		title := strings.ToLower(stream.DisplayTitle + " " + stream.Codec)
		if strings.Contains(title, "hdr") || strings.Contains(title, "smpte2084") || strings.Contains(title, "hlg") || strings.Contains(title, "bt2020") {
			return true
		}
	}
	return false
}

func normalizeTranscodeQuality(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "auto"
	}
	if value == "original" {
		return value
	}
	if _, ok := transcodePresets[value]; ok {
		return value
	}
	return "auto"
}

func normalizeTranscodeAudioMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "transcode", "aac":
		return "transcode"
	default:
		return ""
	}
}

func normalizeSelectedAudioStreamID(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "audio_default", "default":
		return ""
	default:
		return value
	}
}

func transcodeAudioModeForDecision(decision PlaybackDecision) string {
	if decision.AudioTranscode {
		return "transcode"
	}
	return ""
}

func sourceEquivalentTranscodePreset(item MediaItem) transcodePreset {
	preset := transcodePresets["1080p-medium"]
	preset.id = "original"
	preset.label = "Original Quality"
	preset.height = sourceVideoHeight(item, preset.height)
	preset.videoK = sourceEquivalentVideoKbps(item, preset.videoK)
	preset.audioK = sourceAudioKbps(item, preset.audioK)
	preset.crf = 18
	return preset
}

func sourceEquivalentVideoKbps(item MediaItem, fallback int) int {
	base := sourceEffectiveVideoKbps(item, fallback)
	video := firstStreamOfKind(item.Streams, "video")
	switch normalizeCodec(video.Codec) {
	case "hevc", "h265", "av1":
		return clampTranscodeVideoKbps(max(base*2, sourceEquivalentH264FloorKbps(video.Height)))
	default:
		return base
	}
}

func sourceEquivalentH264FloorKbps(height int) int {
	switch {
	case height >= 2160:
		return 20000
	case height >= 1080:
		return 8000
	case height >= 720:
		return 4500
	case height >= 480:
		return 2500
	default:
		return 1500
	}
}

func directStreamRemuxAvailable(item MediaItem, quality string, settings transcodeSettings, subtitleID string) bool {
	if !settings.DirectStreamRemux || quality != "original" || normalizeBurnInSubtitleID(subtitleID) != "" {
		return false
	}
	video := firstStreamOfKind(item.Streams, "video")
	if !directStreamRemuxVideoCodec(video.Codec) {
		return false
	}
	if video.Width <= 0 || video.Height <= 0 {
		return false
	}
	return true
}

func directStreamRemuxVideoCodec(codec string) bool {
	switch normalizeCodec(codec) {
	case "h264", "hevc", "h265":
		return true
	default:
		return false
	}
}

func directStreamRemuxUsesFragmentedMP4(item MediaItem) bool {
	video := firstStreamOfKind(item.Streams, "video")
	switch normalizeCodec(video.Codec) {
	case "hevc", "h265":
		return true
	default:
		return false
	}
}

func directStreamRemuxNeedsHVC1Tag(item MediaItem) bool {
	return directStreamRemuxVideoTag(item) == "hvc1"
}

func directStreamRemuxVideoTag(item MediaItem) string {
	video := firstStreamOfKind(item.Streams, "video")
	if strings.TrimSpace(video.DolbyVisionProfile) != "" {
		return "dvh1"
	}
	switch normalizeCodec(video.Codec) {
	case "hevc", "h265":
		return "hvc1"
	default:
		return ""
	}
}

func hlsAudioEncodingArgs(item MediaItem, preset transcodePreset, forceTranscode bool, audioStreamID string) []string {
	audio := selectedAudioStreamOrDefault(item, audioStreamID)
	if !forceTranscode && hlsAudioCopyAvailable(audio) {
		return []string{"-c:a", "copy"}
	}
	return []string{"-c:a", "aac", "-ac", "2", "-b:a", fmt.Sprintf("%dk", preset.audioK)}
}

func hlsAudioCopyAvailable(audio Stream) bool {
	switch normalizeCodec(audio.Codec) {
	case "", "aac", "ac3", "eac3":
		return true
	default:
		return false
	}
}

func selectedAudioStreamOrDefault(item MediaItem, audioStreamID string) Stream {
	audioStreamID = normalizeSelectedAudioStreamID(audioStreamID)
	var firstAudio Stream
	for _, stream := range playbackStreamsForSelectedVersion(item) {
		if stream.Kind != "audio" {
			continue
		}
		if firstAudio.ID == "" {
			firstAudio = stream
		}
		if audioStreamID != "" && stream.ID == audioStreamID {
			return stream
		}
	}
	return firstAudio
}

func ffmpegAudioMapForSelection(item MediaItem, audioStreamID string) (string, error) {
	audioStreamID = normalizeSelectedAudioStreamID(audioStreamID)
	if audioStreamID == "" {
		return "0:a:0?", nil
	}
	audioOrdinal := 0
	for _, stream := range playbackStreamsForSelectedVersion(item) {
		if stream.Kind != "audio" {
			continue
		}
		if stream.ID != audioStreamID {
			audioOrdinal++
			continue
		}
		if stream.SourceKind == "ffprobe" && stream.Index >= 0 {
			return fmt.Sprintf("0:%d?", stream.Index), nil
		}
		return fmt.Sprintf("0:a:%d?", audioOrdinal), nil
	}
	return "", fmt.Errorf("audio stream %q was not found", audioStreamID)
}

func sourceVideoHeight(item MediaItem, fallback int) int {
	for _, stream := range item.Streams {
		if stream.Kind == "video" && stream.Height > 0 {
			return stream.Height
		}
	}
	return fallback
}

func sourceEffectiveVideoKbps(item MediaItem, fallback int) int {
	for _, stream := range item.Streams {
		if stream.Kind == "video" && stream.Bitrate > 0 {
			return clampTranscodeVideoKbps((stream.Bitrate + 999) / 1000)
		}
	}
	if item.DurationSeconds > 0 {
		for _, file := range item.MediaFiles {
			if !file.Selected && len(item.MediaFiles) > 1 {
				continue
			}
			if file.SizeBytes <= 0 {
				continue
			}
			totalKbps := int((file.SizeBytes * 8) / int64(item.DurationSeconds) / 1000)
			videoKbps := totalKbps - sourceAudioKbps(item, 0)
			if videoKbps > 0 {
				return clampTranscodeVideoKbps(videoKbps)
			}
		}
	}
	return fallback
}

func sourceAudioKbps(item MediaItem, fallback int) int {
	for _, stream := range item.Streams {
		if stream.Kind == "audio" && stream.Bitrate > 0 {
			return clampTranscodeAudioKbps((stream.Bitrate + 999) / 1000)
		}
	}
	return fallback
}

func clampTranscodeVideoKbps(kbps int) int {
	if kbps <= 0 {
		return 12000
	}
	return max(700, min(kbps, 50000))
}

func clampTranscodeAudioKbps(kbps int) int {
	if kbps <= 0 {
		return 192
	}
	return max(96, min(kbps, 640))
}

func safePathComponent(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "item"
	}
	return builder.String()
}

func safeX264Preset(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "veryfast"
	}
}

func safeToneMappingAlgorithm(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "clip", "linear", "gamma", "reinhard", "hable", "mobius":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "hable"
	}
}
