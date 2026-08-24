package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/ffmpeggraph"
	"github.com/PorticoMediaServer/portico-server/internal/ffmpegsupervisor"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

var errPlannedTranscode = errors.New("stored playback plan cannot be executed")

var renamePlannedVODGeneration = os.Rename

type plannedTranscodeAdmissionClaim struct {
	hardware   bool
	background bool
}

// plannedTranscodeIdentity is the authorization and playback-session scope
// for one planned VOD producer. A plan digest alone is not sufficient: two
// playback sessions can legitimately carry the same plan while having
// different authorization lifetimes or playback generations.
type plannedTranscodeIdentity struct {
	UserID                string
	ProfileID             string
	PlaybackSessionID     string
	AuthorizationRevision string
	PlaybackGeneration    int
	GrantTokenHash        string
}

func (identity plannedTranscodeIdentity) canonical() string {
	return strings.Join([]string{
		strings.TrimSpace(identity.UserID),
		strings.TrimSpace(identity.ProfileID),
		strings.TrimSpace(identity.PlaybackSessionID),
		strings.TrimSpace(identity.AuthorizationRevision),
		strconv.Itoa(identity.PlaybackGeneration),
		strings.TrimSpace(identity.GrantTokenHash),
	}, "\x00")
}

func (identity plannedTranscodeIdentity) namespaceDigest() string {
	sum := sha256.Sum256([]byte(identity.canonical()))
	return hex.EncodeToString(sum[:])
}

func (identity plannedTranscodeIdentity) validForGeneration(generation int) bool {
	return strings.TrimSpace(identity.UserID) != "" &&
		strings.TrimSpace(identity.ProfileID) != "" &&
		strings.TrimSpace(identity.PlaybackSessionID) != "" &&
		strings.TrimSpace(identity.AuthorizationRevision) != "" &&
		strings.TrimSpace(identity.GrantTokenHash) != "" &&
		generation > 0 && identity.PlaybackGeneration == generation
}

// acquirePlannedTranscodeStart coalesces the manifest prewarm and the first
// segment request for one immutable generation. The winning caller owns all
// compilation/admission/publication work; followers wait for that attempt and
// then re-check the published session (or make one new attempt after failure).
func (s *Server) acquirePlannedTranscodeStart(key string) (release func(), waiting <-chan struct{}) {
	s.transcodeMu.Lock()
	defer s.transcodeMu.Unlock()
	if s.plannedTranscodeStarts == nil {
		s.plannedTranscodeStarts = make(map[string]chan struct{})
	}
	if started := s.plannedTranscodeStarts[key]; started != nil {
		return nil, started
	}
	started := make(chan struct{})
	s.plannedTranscodeStarts[key] = started
	var once sync.Once
	return func() {
		once.Do(func() {
			s.transcodeMu.Lock()
			if s.plannedTranscodeStarts[key] == started {
				delete(s.plannedTranscodeStarts, key)
				close(started)
			}
			s.transcodeMu.Unlock()
		})
	}, nil
}

// acquirePlannedTranscodeAdmission reserves canonical capacity before launch.
// The immutable claim classification is retained until the generation's
// supervisor releases it, so settings changes cannot corrupt accounting.
func (s *Server) acquirePlannedTranscodeAdmission(key string, generation int, settings transcodeSettings, hardware, background bool) (func(), error) {
	claimKey := key + ":g" + strconv.Itoa(generation)
	s.transcodeMu.Lock()
	defer s.transcodeMu.Unlock()
	if s.plannedTranscodeClaims == nil {
		s.plannedTranscodeClaims = make(map[string]plannedTranscodeAdmissionClaim)
	}
	if _, exists := s.plannedTranscodeClaims[claimKey]; exists {
		return nil, errors.New("the playback generation is already being admitted")
	}
	total, hardwareActive, softwareActive, backgroundActive := 0, 0, 0, 0
	for sessionKey, session := range s.transcodes {
		if session == nil || !session.isRunning() {
			continue
		}
		// A published canonical generation is already represented by its claim.
		claimed := false
		for existingClaimKey := range s.plannedTranscodeClaims {
			if strings.HasPrefix(existingClaimKey, sessionKey+":g") {
				claimed = true
				break
			}
		}
		if claimed {
			continue
		}
		total++
		if session.method == "hardware-encode" || session.method == "planned-v2-hardware" {
			hardwareActive++
		} else {
			softwareActive++
		}
		if session.background {
			backgroundActive++
		}
	}
	for _, claim := range s.plannedTranscodeClaims {
		total++
		if claim.hardware {
			hardwareActive++
		} else {
			softwareActive++
		}
		if claim.background {
			backgroundActive++
		}
	}
	if settings.MaxConcurrentSessions > 0 && total >= settings.MaxConcurrentSessions {
		return nil, errors.New("the transcode session limit has been reached")
	}
	if hardware && settings.MaxHardwareSessions > 0 && hardwareActive >= settings.MaxHardwareSessions {
		return nil, errors.New("the hardware transcode session limit has been reached")
	}
	if !hardware && settings.MaxSoftwareSessions > 0 && softwareActive >= settings.MaxSoftwareSessions {
		return nil, errors.New("the software transcode session limit has been reached")
	}
	// Legacy semantics intentionally treat zero background capacity as disabled.
	if background && (settings.MaxBackgroundSessions <= 0 || backgroundActive >= settings.MaxBackgroundSessions) {
		return nil, errors.New("the background transcode session limit has been reached")
	}
	s.plannedTranscodeClaims[claimKey] = plannedTranscodeAdmissionClaim{hardware: hardware, background: background}
	var once sync.Once
	return func() {
		once.Do(func() {
			s.transcodeMu.Lock()
			delete(s.plannedTranscodeClaims, claimKey)
			s.transcodeMu.Unlock()
		})
	}, nil
}

// plannedVODHLSRequest contains execution-only inputs which are deliberately
// absent from the persisted playback plan. Hardware must be the exact plan
// produced from verified probe evidence; this adapter never guesses a device.
type plannedVODHLSRequest struct {
	Item             MediaItem
	Binding          playbackExecutionBinding
	Identity         plannedTranscodeIdentity
	GenerationRoot   string
	SegmentSeconds   int
	StartNumber      int
	Hardware         *playbackhw.Plan
	RemoteSource     bool
	SourcePath       string
	RemoteObjectPath string
	ResolveSubtitle  func(context.Context, MediaItem, playbackplan.Plan) (string, error)
}

// plannedVODHLSCommand is safe to hand to the shared FFmpeg supervisor. Args
// excludes the executable and no process has been started.
type plannedVODHLSCommand struct {
	ExecutablePath   string
	Args             []string
	GenerationDir    string
	ManifestPath     string
	SegmentPattern   string
	InitFilename     string
	PlanDigest       string
	SourceRevision   string
	MediaFactsDigest string
	UsesHardware     bool
	PredictedBytes   int64
}

// compilePlannedVODHLS validates the sealed grant/session binding against the
// current source facts and compiles one deterministic VOD HLS generation. It
// creates only the private generation directory and never launches FFmpeg.
func (s *Server) compilePlannedVODHLS(ctx context.Context, req plannedVODHLSRequest) (plannedVODHLSCommand, error) {
	binding, err := decodePlaybackExecutionBinding(mustJSONPlaybackBinding(req.Binding))
	if err != nil {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: invalid binding", errPlannedTranscode)
	}
	if !req.Identity.validForGeneration(binding.Generation) {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: invalid playback identity", errPlannedTranscode)
	}
	var plan playbackplan.Plan
	if err := json.Unmarshal(binding.Plan, &plan); err != nil || plan.Validate() != nil {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: invalid plan", errPlannedTranscode)
	}
	if plan.Protocol != "hls" || plan.Mode == playbackplan.DirectPlay {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: plan is not VOD HLS", errPlannedTranscode)
	}
	if plan.Timeline.Dynamic && req.StartNumber != 0 {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: an event timeline must begin at its source origin", errPlannedTranscode)
	}
	if req.StartNumber < 0 || req.StartNumber > 10_000_000 || req.SegmentSeconds < 1 || req.SegmentSeconds > 30 {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: invalid HLS bounds", errPlannedTranscode)
	}
	if req.Hardware != nil && (!plan.Hardware.Verified || req.Hardware.RequiresRuntimeProbe) {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: unverified hardware route", errPlannedTranscode)
	}
	executablePath := strings.TrimSpace(firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg"))
	if req.Hardware != nil {
		if !s.playbackHardwareExecutionIdentityMatches(ctx, req.Hardware) {
			return plannedVODHLSCommand{}, fmt.Errorf("%w: hardware execution identity changed", errPlannedTranscode)
		}
		executablePath = req.Hardware.RuntimeIdentity.ExecutablePath
	} else {
		resolved, resolveErr := exec.LookPath(executablePath)
		if resolveErr != nil {
			return plannedVODHLSCommand{}, fmt.Errorf("%w: FFmpeg is unavailable", errPlannedTranscode)
		}
		executablePath, err = filepath.Abs(resolved)
		if err != nil {
			return plannedVODHLSCommand{}, fmt.Errorf("%w: FFmpeg path is invalid", errPlannedTranscode)
		}
	}

	facts, factsDigest, err := s.mediaFactsForPlayback(ctx, req.Item)
	if err != nil || factsDigest == "" || factsDigest != binding.MediaFactsDigest {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: media facts changed", errPlannedTranscode)
	}
	if facts.Source.Revision != binding.SourceRevision || facts.Source.Revision != plan.SourceRevision || facts.Source.Fingerprint != plan.SourceFingerprint {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: source identity changed", errPlannedTranscode)
	}
	sourcePath := "pipe:0"
	if req.RemoteSource && strings.TrimSpace(req.SourcePath) != "" {
		parsed, parseErr := url.Parse(strings.TrimSpace(req.SourcePath))
		hostname := ""
		if parsed != nil {
			hostname = parsed.Hostname()
		}
		ip := net.ParseIP(hostname)
		port, portErr := strconv.Atoi(parsed.Port())
		pathParts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
		if parseErr != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || ip == nil || !ip.IsLoopback() || portErr != nil || port < 1024 || port > 65535 || len(pathParts) != 1 || !strings.HasPrefix(pathParts[0], "probe_") || len(pathParts[0]) < 20 {
			return plannedVODHLSCommand{}, fmt.Errorf("%w: remote VOD transport must be an opaque loopback capability", errPlannedTranscode)
		}
		sourcePath = parsed.String()
	} else if !req.RemoteSource {
		var ok bool
		sourcePath, ok = localPathFromSourceURL(req.Item.SourceURL)
		if !ok || !filepath.IsAbs(sourcePath) {
			return plannedVODHLSCommand{}, fmt.Errorf("%w: VOD source must be an absolute local path", errPlannedTranscode)
		}
		sourcePath = filepath.Clean(sourcePath)
	}

	subtitlePath := ""
	var subtitleStreamOrdinal *int
	if plan.Subtitle.Action == playbackplan.BurnIn {
		if req.ResolveSubtitle != nil {
			subtitlePath, err = req.ResolveSubtitle(ctx, req.Item, plan)
		} else {
			subtitlePath, err = exactSidecarSubtitlePath(req.Item, plan)
			if err != nil && plan.Selection.SubtitleIndex != nil {
				for ordinal, subtitle := range facts.Subtitles {
					if subtitle.Index == *plan.Selection.SubtitleIndex {
						if req.RemoteSource {
							return plannedVODHLSCommand{}, fmt.Errorf("%w: embedded subtitle burn-in is unavailable for a remote source", errPlannedTranscode)
						}
						subtitlePath = sourcePath
						value := ordinal
						subtitleStreamOrdinal = &value
						err = nil
						break
					}
				}
			}
		}
		if err != nil || !filepath.IsAbs(subtitlePath) {
			return plannedVODHLSCommand{}, fmt.Errorf("%w: exact subtitle path unavailable", errPlannedTranscode)
		}
		subtitlePath = filepath.Clean(subtitlePath)
	}

	generationDir, err := privatePlaybackGenerationDir(req.GenerationRoot, req.Item.ID, binding.Generation, binding.Digest, req.StartNumber, req.Identity)
	if err != nil {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: %v", errPlannedTranscode, err)
	}
	generationOwned := true
	defer func() {
		if generationOwned {
			_ = os.RemoveAll(generationDir)
		}
	}()
	ext := ".ts"
	initFilename := ""
	if plan.SegmentFormat == "fmp4" {
		ext = ".m4s"
		initFilename = "init.mp4"
	}
	manifest := filepath.Join(generationDir, "index.m3u8")
	segments := filepath.Join(generationDir, "segment_%05d"+ext)
	compiled, err := ffmpeggraph.Compile(ffmpeggraph.Request{
		Plan: plan, Facts: facts, SourcePath: sourcePath, SubtitlePath: subtitlePath, SubtitleStreamOrdinal: subtitleStreamOrdinal, Hardware: req.Hardware,
		X264Preset: binding.X264Preset,
		Output:     ffmpeggraph.Output{ManifestPath: manifest, SegmentPattern: segments, InitFilename: initFilename, SegmentSeconds: req.SegmentSeconds, StartNumber: req.StartNumber, StartSeconds: req.StartNumber * req.SegmentSeconds, Event: plan.Timeline.Dynamic},
	})
	if err != nil {
		return plannedVODHLSCommand{}, fmt.Errorf("%w: %v", errPlannedTranscode, err)
	}
	if req.RemoteSource && sourcePath != "pipe:0" {
		demuxer, demuxErr := remoteMediaDemuxer(req.RemoteObjectPath)
		if demuxErr != nil {
			return plannedVODHLSCommand{}, fmt.Errorf("%w: remote media format is not approved", errPlannedTranscode)
		}
		compiled.Args, err = forceFFmpegInputDemuxer(compiled.Args, sourcePath, demuxer)
		if err != nil {
			return plannedVODHLSCommand{}, fmt.Errorf("%w: %v", errPlannedTranscode, err)
		}
	}
	generationOwned = false
	return plannedVODHLSCommand{
		ExecutablePath: executablePath, Args: append([]string(nil), compiled.Args...), GenerationDir: generationDir,
		ManifestPath: manifest, SegmentPattern: segments, InitFilename: initFilename,
		PlanDigest: binding.Digest, SourceRevision: binding.SourceRevision,
		MediaFactsDigest: factsDigest, UsesHardware: compiled.UsesHardware,
		PredictedBytes: predictedPlaybackOutputBytes(plan, facts),
	}, nil
}

func forceFFmpegInputDemuxer(args []string, sourcePath, demuxer string) ([]string, error) {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "-i" && args[index+1] == sourcePath {
			result := make([]string, 0, len(args)+2)
			result = append(result, args[:index]...)
			result = append(result, "-rw_timeout", "30000000", "-f", demuxer)
			result = append(result, args[index:]...)
			return result, nil
		}
	}
	return nil, errors.New("compiled FFmpeg graph did not contain the approved remote input")
}

func exactSidecarSubtitlePath(item MediaItem, plan playbackplan.Plan) (string, error) {
	if plan.Selection.SubtitleIndex == nil {
		return "", errPlannedTranscode
	}
	for _, stream := range playbackStreamsForSelectedVersion(item) {
		if stream.Kind != "subtitle" || stream.Index != *plan.Selection.SubtitleIndex {
			continue
		}
		path, ok := localPathFromSourceURL(stream.SourceURL)
		if !ok || !filepath.IsAbs(path) {
			return "", errPlannedTranscode
		}
		return filepath.Clean(path), nil
	}
	// Embedded subtitle extraction is intentionally delegated to an exact
	// resolver. Passing the media file to FFmpeg's subtitles filter would let
	// FFmpeg choose a stream and violate the immutable selection.
	return "", errPlannedTranscode
}

func privatePlaybackGenerationPath(root, mediaID string, generation int, digest string, startNumber int, identity plannedTranscodeIdentity) (string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if !filepath.IsAbs(root) || strings.TrimSpace(mediaID) == "" || generation < 1 || startNumber < 0 || strings.TrimSpace(digest) == "" || !identity.validForGeneration(generation) {
		return "", errors.New("invalid generation identity")
	}
	mediaKey := plannedVODMediaNamespaceToken(mediaID)
	digestSum := sha256.Sum256([]byte(strings.TrimSpace(digest)))
	digestKey := hex.EncodeToString(digestSum[:])[:24]
	identityKey := identity.namespaceDigest()[:24]
	dir := filepath.Join(root, mediaKey+"-g"+strconv.Itoa(generation)+"-s"+strconv.Itoa(startNumber)+"-"+digestKey+"-i"+identityKey)
	return dir, nil
}

func privatePlaybackGenerationDir(root, mediaID string, generation int, digest string, startNumber int, identity plannedTranscodeIdentity) (string, error) {
	dir, err := privatePlaybackGenerationPath(root, mediaID, generation, digest, startNumber, identity)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if err := ensurePrivateDirectory(root); err != nil {
		return "", err
	}
	if err := ensurePrivateDirectory(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("generation path is not a trusted directory")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	return nil
}

func safeExecutionPathToken(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func plannedVODMediaNamespaceToken(mediaID string) string {
	mediaID = strings.TrimSpace(mediaID)
	sum := sha256.Sum256([]byte(mediaID))
	return safeExecutionPathToken(mediaID) + "-m" + hex.EncodeToString(sum[:])[:16]
}

func plannedTranscodeSessionKey(mediaID string, binding playbackExecutionBinding, identity plannedTranscodeIdentity, startSeconds int) string {
	startSeconds = normalizePlannedVODSeekSeconds(startSeconds)
	executionInputs := strings.Join([]string{
		strings.TrimSpace(binding.Digest),
		normalizeTranscodeQuality(binding.Quality),
		normalizeTranscodeAudioMode(binding.AudioMode),
		normalizeSelectedAudioStreamID(binding.AudioStreamID),
		strings.ToLower(strings.TrimSpace(binding.SubtitleMode)),
		strings.ToLower(strings.TrimSpace(binding.SubtitleStreamID)),
		strconv.FormatBool(binding.DirectStream),
	}, "\x00")
	executionSum := sha256.Sum256([]byte(executionInputs))
	return "planned-v2:" + plannedVODMediaNamespaceToken(mediaID) + ":e" + hex.EncodeToString(executionSum[:]) + ":g" + strconv.Itoa(binding.Generation) + ":i" + identity.namespaceDigest() + ":" + strconv.Itoa(max(0, startSeconds))
}

func normalizePlannedVODSeekSeconds(startSeconds int) int {
	return max(0, startSeconds/hlsSegmentSeconds) * hlsSegmentSeconds
}

func (s *Server) reconcilePlannedVODGenerationDirectory(path string) error {
	rawPath := strings.TrimSpace(path)
	cleanPath := filepath.Clean(rawPath)
	if rawPath == "" || cleanPath == "." || !filepath.IsAbs(cleanPath) {
		return errors.New("invalid planned VOD generation path")
	}
	path, err := filepath.Abs(cleanPath)
	if err != nil {
		return errors.New("invalid planned VOD generation path")
	}
	s.transcodeMu.Lock()
	defer s.transcodeMu.Unlock()
	for _, session := range s.transcodes {
		if session == nil {
			continue
		}
		activePath, activeErr := filepath.Abs(filepath.Clean(strings.TrimSpace(session.dir)))
		if activeErr == nil && activePath == path {
			return errors.New("planned VOD generation namespace is still active")
		}
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("planned VOD generation namespace is not a trusted directory")
	}
	archive := fmt.Sprintf("%s.retired-%d", path, time.Now().UTC().UnixNano())
	if err := renamePlannedVODGeneration(path, archive); err != nil {
		return fmt.Errorf("unable to retire stale planned VOD generation: %w", err)
	}
	return nil
}

func archivePlannedTranscodeGeneration(session *transcodeSession) error {
	if session == nil || strings.TrimSpace(session.dir) == "" {
		return nil
	}
	info, err := os.Lstat(session.dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("planned VOD generation is not a trusted directory")
	}
	if !session.retireAndWait(transcodeReaderDrainTimeout) {
		return errors.New("planned VOD generation is still serving a reader")
	}
	if err := session.stopAndWait(transcodeRecoveryInterruptGrace); err != nil {
		return fmt.Errorf("unable to stop planned VOD generation: %w", err)
	}
	archive := fmt.Sprintf("%s.retired-%d", session.dir, time.Now().UTC().UnixNano())
	if err := renamePlannedVODGeneration(session.dir, archive); err != nil {
		return fmt.Errorf("unable to rename planned VOD generation: %w", err)
	}
	return nil
}

// ensurePlannedVODHLSSession is the only VOD launch path for a stored plan.
// It compiles the sealed graph, admits the source through W3, and transfers
// sole process ownership to the generation supervisor before publishing the
// session in the reusable transcode registry.
func (s *Server) ensurePlannedVODHLSSession(ctx context.Context, userID string, item MediaItem, binding playbackExecutionBinding, identity plannedTranscodeIdentity, startSeconds int, segmentName string, background bool) (*transcodeSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	settings := s.transcodeSettings()
	if !settings.Enabled {
		return nil, errors.New("transcoding is disabled")
	}
	if background && s.shouldDeferBackgroundJobsForPressure() {
		return nil, errors.New("background transcode prewarm deferred while server is under load")
	}
	quality, subtitleID, _, audioMode, audioStreamID, directStream, err := playbackBindingHLSValues(binding)
	if err != nil {
		return nil, err
	}
	if !identity.validForGeneration(binding.Generation) || strings.TrimSpace(userID) != strings.TrimSpace(identity.UserID) {
		return nil, fmt.Errorf("%w: invalid playback identity", errPlannedTranscode)
	}
	// FFmpeg and the published segment namespace operate on the HLS seek grid.
	// Normalize before deriving either identity so an off-grid request cannot
	// create two registry keys that point at the same generation directory.
	startSeconds = normalizePlannedVODSeekSeconds(boundedMediaHLSStartSeconds(startSeconds, item.DurationSeconds))
	key := plannedTranscodeSessionKey(item.ID, binding, identity, startSeconds)

	// The static manifest intentionally prewarms asynchronously. A client may
	// request its first segment before that prewarm publishes the session, so
	// join the in-flight start instead of rejecting the valid generation as a
	// duplicate admission.
	releaseStart, waiting := s.acquirePlannedTranscodeStart(key)
	if waiting != nil {
		select {
		case <-waiting:
			return s.ensurePlannedVODHLSSession(ctx, userID, item, binding, identity, startSeconds, segmentName, background)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	defer releaseStart()

	s.transcodeMu.Lock()
	if existing := s.transcodes[key]; existing != nil {
		state := existing.snapshot()
		canReuse := !state.stopped && state.err == nil && (existing.isRunning() || existing.completedSuccessfully() || segmentName != "" && transcodeSessionHasSegment(existing, segmentName))
		if canReuse && segmentName != "" && !plannedTranscodeSessionCanServeSegment(existing, segmentName) {
			canReuse = false
		}
		if canReuse {
			s.transcodeMu.Unlock()
			return existing, nil
		}
		if state.err != nil && time.Since(state.errAt) < 750*time.Millisecond {
			s.transcodeMu.Unlock()
			return nil, existing.transcodeError()
		}
		if !existing.retireAndWait(transcodeReaderDrainTimeout) {
			s.transcodeMu.Unlock()
			return nil, errors.New("prior playback generation is still serving a reader")
		}
		if strings.TrimSpace(existing.dir) != "" {
			if err := archivePlannedTranscodeGeneration(existing); err != nil {
				s.transcodeMu.Unlock()
				return nil, fmt.Errorf("unable to retire prior planned VOD generation: %w", err)
			}
		}
		delete(s.transcodes, key)
	}
	s.transcodeMu.Unlock()

	admissionCtx, releaseAdmission, err := s.restoreBarrier.acquire(ctx)
	if err != nil {
		return nil, errors.New("restore admission is quiescing")
	}
	defer releaseAdmission()
	if admissionCtx.Err() != nil {
		return nil, errors.New("restore admission is quiescing")
	}
	var sourcePath string
	err = s.withPlaybackStorageIO(admissionCtx, item.SourceURL, playbackStorageTranscode, "resolve VOD transcode source", func(_ context.Context, progress func()) error {
		resolved, resolveErr := s.sourcePathForHLSTranscode(item)
		if resolveErr == nil {
			sourcePath = resolved
			progress()
		}
		return resolveErr
	})
	if err != nil {
		return nil, err
	}
	storageRemote := strings.HasPrefix(strings.TrimSpace(sourcePath), "portico-storage://")
	remoteRequest := remoteTranscodeSourceRequest{}
	remoteSource := storageRemote
	if !storageRemote {
		remoteRequest, remoteSource, err = remoteTranscodeRequestForItem(item, sourcePath)
		if err != nil {
			return nil, err
		}
	}
	if !remoteSource {
		parsed, ok := localPathFromSourceURL(sourcePath)
		if !ok || !filepath.IsAbs(parsed) {
			return nil, errors.New("planned VOD source is neither a supported local path nor an admitted remote HTTP source")
		}
		sourcePath = parsed
		item.SourceURL = sourcePath
	}
	root := strings.TrimSpace(settings.TemporaryDirectory)
	if root == "" {
		root = s.cfg.TranscodeDir
	}
	root, err = filepath.Abs(filepath.Join(root, "planned-v2"))
	if err != nil {
		return nil, err
	}
	if err := ensureMediaWriteCapacity(filepath.Dir(root), mediaWriteMinimumFreeBytes); err != nil {
		return nil, err
	}
	generationDir, err := privatePlaybackGenerationPath(root, item.ID, binding.Generation, binding.Digest, startSeconds/hlsSegmentSeconds, identity)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errPlannedTranscode, err)
	}
	if err := s.reconcilePlannedVODGenerationDirectory(generationDir); err != nil {
		return nil, err
	}
	transcodeInputURL := ""
	remoteObjectPath := ""
	closeStorageTransport := func() {}
	storageTransportOwned := false
	if storageRemote {
		_, remoteObjectPath, err = parseRemoteStorageLocator(sourcePath)
		if err != nil {
			return nil, err
		}
		transcodeInputURL, closeStorageTransport, err = s.startRemoteStoragePlaybackTransport(s.backgroundCtx, item, sourcePath)
		if err != nil {
			return nil, err
		}
		storageTransportOwned = true
		defer func() {
			if storageTransportOwned {
				closeStorageTransport()
			}
		}()
	}
	command, err := s.compilePlannedVODHLS(admissionCtx, plannedVODHLSRequest{
		Item: item, Binding: binding, Identity: identity, GenerationRoot: root, SegmentSeconds: hlsSegmentSeconds,
		StartNumber: startSeconds / hlsSegmentSeconds, Hardware: binding.HardwarePlan, RemoteSource: remoteSource, SourcePath: transcodeInputURL, RemoteObjectPath: remoteObjectPath,
	})
	if err != nil {
		return nil, err
	}
	generationOwned := true
	defer func() {
		if generationOwned {
			_ = os.RemoveAll(command.GenerationDir)
		}
	}()
	admissionRelease, err := s.acquirePlannedTranscodeAdmission(key, binding.Generation, settings, command.UsesHardware, background)
	if err != nil {
		return nil, err
	}
	admissionOwned := true
	defer func() {
		if admissionOwned {
			admissionRelease()
		}
	}()
	diskRelease, err := s.mediaResourceGovernor().reserveMediaDisk(command.GenerationDir, command.PredictedBytes, mediaDiskReservationMinimum)
	if err != nil {
		return nil, err
	}
	var lease *playbackStorageLease
	if !remoteSource {
		lease, err = s.acquirePlaybackStorageLease(s.backgroundCtx, sourcePath, playbackStorageTranscode, "vod transcode source")
		if err != nil {
			diskRelease()
			return nil, err
		}
	}
	governor := s.mediaResourceGovernor()
	request := mediaResourceRequest{cpu: 1, disk: 2, background: background}
	var resourceRelease func()
	var acquired bool
	if !background {
		governor.preemptBackgroundForPlayback()
		admissionBaseCtx := s.backgroundCtx
		if admissionBaseCtx == nil {
			admissionBaseCtx = context.Background()
		}
		admissionCtx, admissionCancel := context.WithTimeout(admissionBaseCtx, 2*time.Second)
		resourceRelease, err = governor.acquireContext(admissionCtx, request)
		admissionCancel()
		acquired = err == nil
	} else {
		resourceRelease, acquired = governor.tryAcquire(request)
	}
	if !acquired {
		diskRelease()
		if lease != nil {
			lease.Release(errMediaResourcesBusy)
		}
		return nil, errMediaResourcesBusy
	}
	var combinedReleaseOnce sync.Once
	combinedResourceRelease := func() {
		combinedReleaseOnce.Do(func() {
			closeStorageTransport()
			resourceRelease()
			diskRelease()
			admissionRelease()
		})
	}
	diagnosticRecorder := newFFmpegDiagnosticRecorder(command.ExecutablePath, command.Args)
	session := &transcodeSession{
		key: key, userID: strings.TrimSpace(userID), mediaID: item.ID, quality: quality,
		subtitleID: subtitleID, audioMode: audioMode, audioStreamID: audioStreamID, directStream: directStream,
		start: startSeconds, method: map[bool]string{true: "planned-v2-hardware", false: "planned-v2-software"}[command.UsesHardware], filter: "stored-playback-plan", root: root,
		dir: command.GenerationDir, manifest: command.ManifestPath, startedAt: time.Now().UTC(), background: background,
		done: make(chan struct{}), updateCh: make(chan struct{}), segmentSeconds: hlsSegmentSeconds,
		playedRetentionSeconds: settings.PlayedRetentionSeconds, throttleBufferSeconds: settings.ThrottleBufferSeconds,
		lastServedSegment: -1, lastProducedSegment: startSeconds/hlsSegmentSeconds - 1,
		lastProducedAt: time.Now().UTC(), generation: binding.Generation, admissionActive: true,
		resourceRelease: combinedResourceRelease, storageLease: lease, supervisor: s.ffmpegSupervisor,
	}
	release := func(result ffmpegsupervisor.Release) {
		diagnostics := diagnosticRecorder.Report(result.Err)
		diagnosticAPI := diagnostics.API()
		if lease != nil {
			lease.Release(result.Err)
		}
		session.stateMu.Lock()
		session.admissionActive = false
		session.ffmpegDiagnostics = diagnosticAPI
		session.stderr = diagnostics.Text
		if !session.stopped && result.Err != nil {
			session.err = result.Err
			session.errAt = time.Now().UTC()
		}
		stderrText := session.stderr
		session.signalUpdateLocked()
		session.stateMu.Unlock()
		if stderrText != "" {
			_ = os.WriteFile(filepath.Join(session.dir, "ffmpeg.stderr.log"), []byte(stderrText), 0o600)
		}
		if diagnosticAPI != nil {
			s.updatePlaybackTranscodeDiagnostics(item.ID, PlaybackDiagnostics{FFmpeg: diagnosticAPI})
		}
		session.releaseMediaResources()
		session.doneOnce.Do(func() { close(session.done) })
	}
	processFactory := supervisedExecFactoryV2(func(processCtx context.Context) (*exec.Cmd, error) {
		cmd := exec.CommandContext(processCtx, command.ExecutablePath, command.Args...)
		cmd.Dir = command.GenerationDir
		cmd.Stderr = diagnosticRecorder
		return cmd, nil
	})
	if remoteSource && !storageRemote {
		open := func(processCtx context.Context) (*remoteTranscodeSource, error) {
			return openRemoteTranscodeSource(processCtx, remoteRequest)
		}
		processFactory = supervisedReaderExecFactoryV2(open, func(processCtx context.Context) (*exec.Cmd, error) {
			cmd := exec.CommandContext(processCtx, command.ExecutablePath, command.Args...)
			cmd.Dir = command.GenerationDir
			cmd.Stderr = diagnosticRecorder
			return cmd, nil
		})
	}
	handle, err := s.ffmpegSupervisor.Launch(transcodeLaunchV2{
		Kind: transcodeWorkPlaybackV2, Key: key, Mode: ffmpegsupervisor.ModeVOD,
		Start:   processFactory,
		Release: release,
	})
	if err != nil {
		combinedResourceRelease()
		if lease != nil {
			lease.Release(err)
		}
		return nil, err
	}
	generationOwned = false
	storageTransportOwned = false
	admissionOwned = false
	session.supervised = &handle
	s.transcodeMu.Lock()
	if existing := s.transcodes[key]; existing != nil {
		s.transcodeMu.Unlock()
		if stopErr := s.ffmpegSupervisor.Stop(handle); stopErr != nil && !errors.Is(stopErr, ffmpegsupervisor.ErrNotFound) {
			return nil, fmt.Errorf("unable to stop duplicate planned VOD generation: %w", stopErr)
		}
		return existing, nil
	}
	s.transcodes[key] = session
	s.transcodeMu.Unlock()
	go monitorTranscodeBuffer(session)
	if lease != nil {
		go func() {
			if storageErr := <-lease.Done(); storageErr != nil && session.isRunning() {
				session.markFailure(storageErr, true)
				_ = s.ffmpegSupervisor.Stop(handle)
			}
		}()
	}
	s.updatePlaybackTranscodeDiagnostics(item.ID, PlaybackDiagnostics{
		TranscodeQuality: quality, TranscodeMethod: session.method,
		TranscodeFilter: session.filter, FFmpegContext: redactedFFmpegContext(command.Args),
	})
	return session, nil
}

func plannedTranscodeSessionCanServeSegment(session *transcodeSession, segmentName string) bool {
	if session == nil || segmentName == "" || transcodeSessionHasSegment(session, segmentName) {
		return true
	}
	requested, ok := transcodeSegmentIndex(segmentName)
	if !ok {
		return false
	}
	state := session.snapshot()
	// A missing segment behind the producer's watermark is a retention miss,
	// not a segment that is still being generated. Reissue the seek-scoped
	// generation so completed VOD playback can recover from a backward seek.
	if state.lastProducedSegment >= requested {
		return false
	}
	return session.isRunning()
}
