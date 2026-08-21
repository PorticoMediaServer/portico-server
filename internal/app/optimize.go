package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/ffmpeggraph"
	"github.com/PorticoMediaServer/portico-server/internal/ffmpegsupervisor"
	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/optimized"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

type optimizedVersionRecord struct {
	ID           string
	MediaID      string
	Profile      string
	State        string
	Path         string
	SizeBytes    int64
	UpdatedAt    string
	SupersededAt string
	modTime      time.Time
}

var errOptimizedArtifactLeased = errors.New("optimized artifact is currently in use")

func (s *Server) defaultOptimizedVersionStorageDir() string {
	return filepath.Clean(filepath.Join(s.cfg.AppDataDir, "optimized"))
}

func (s *Server) optimizedVersionStorageDir() string {
	settings := s.optimizedVersionSettings()
	if settings.StorageDirectory != "" {
		return settings.StorageDirectory
	}
	return s.defaultOptimizedVersionStorageDir()
}

func (s *Server) optimizedVersionStorageRoots() []string {
	primary := s.optimizedVersionStorageDir()
	fallback := s.defaultOptimizedVersionStorageDir()
	if primary == fallback {
		return []string{primary}
	}
	return []string{primary, fallback}
}

func (s *Server) optimizedVersionPathAllowed(path string) bool {
	clean := filepath.Clean(path)
	for _, root := range s.optimizedVersionStorageRoots() {
		if pathInsideRoot(clean, root) {
			return true
		}
	}
	return false
}

func (s *Server) optimizedPlaybackPreferenceSource(ctx context.Context, item MediaItem, originalFacts mediafacts.Facts, originalDigest string) (MediaItem, mediafacts.Facts, string, *optimizedV2ReadyArtifact, bool) {
	settings := s.optimizedVersionSettings()
	if !settings.PreferOptimizedPlayback || strings.TrimSpace(item.ID) == "" || item.Type == "live_channel" {
		return MediaItem{}, mediafacts.Facts{}, "", nil, false
	}
	profile := strings.TrimSpace(settings.DefaultProfile)
	preset, ok := optimized.Lookup(profile)
	if !ok {
		return MediaItem{}, mediafacts.Facts{}, "", nil, false
	}
	source, err := optimizedSourceIdentityFromFacts(originalFacts, originalDigest)
	if err != nil {
		return MediaItem{}, mediafacts.Facts{}, "", nil, false
	}
	artifact, err := s.optimizedV2ReadyForSource(ctx, item.ID, preset.ID, source, func(path string, size int64) bool {
		return s.optimizedArtifactUsable(ctx, path, size)
	})
	if err != nil || artifact == nil {
		return MediaItem{}, mediafacts.Facts{}, "", nil, false
	}
	var probe ffprobePayload
	if json.Unmarshal(artifact.OutputFactsJSON, &probe) != nil {
		return MediaItem{}, mediafacts.Facts{}, "", nil, false
	}
	identity := canonicalAnalysisFileIdentity("optimized:"+artifact.ID, artifact.OutputFactsDigest, artifact.SizeBytes, artifact.PlanDigest)
	facts, err := playbackFactsFromFFprobe(identity, probe)
	if err != nil {
		return MediaItem{}, mediafacts.Facts{}, "", nil, false
	}
	digest, err := facts.Digest()
	if err != nil {
		return MediaItem{}, mediafacts.Facts{}, "", nil, false
	}
	next := item
	next.SourceURL = optimizedStreamURL(item.ID, preset.ID)
	next.MediaFiles = nil
	return next, facts, digest, artifact, true
}

func (s *Server) optimizedArtifactUsable(ctx context.Context, path string, expectedSize int64) bool {
	clean := filepath.Clean(path)
	if !s.optimizedVersionPathAllowed(clean) || expectedSize <= 0 {
		return false
	}
	root := ""
	for _, candidate := range s.optimizedVersionStorageRoots() {
		if pathInsideRoot(clean, candidate) {
			root = candidate
			break
		}
	}
	if root == "" || validateOptimizedManagedPath(root, clean, false, false) != nil {
		return false
	}
	return s.withPlaybackStorageIO(ctx, clean, playbackStorageDirect, "stat optimized artifact", func(_ context.Context, progress func()) error {
		info, err := os.Lstat(clean)
		if err == nil {
			progress()
		}
		if err != nil || info.IsDir() || info.Size() != expectedSize {
			if err != nil {
				return err
			}
			return errors.New("optimized artifact identity does not match stored output facts")
		}
		return nil
	}) == nil
}

func (s *Server) runOptimizeVersion(ctx context.Context, job Job) {
	if job.ResourceType != "media" || strings.TrimSpace(job.ResourceID) == "" {
		_ = s.setJobMessage(job.ID, "failed", 100, "Optimized version failed because no media item was selected.")
		return
	}
	_ = s.setJobMessage(job.ID, "running", 8, "Preparing optimized version.")
	item, err := s.getMediaBackgroundSourceSeedContext(ctx, job.ResourceID)
	if err != nil {
		_ = s.setJobMessage(job.ID, "failed", 100, "Optimized version failed because the media item was not found.")
		return
	}
	item.Streams, err = s.listStreamsContext(ctx, item.ID)
	if err != nil {
		_ = s.setJobMessage(job.ID, "failed", 100, "Optimized version failed because analyzed streams could not be loaded.")
		return
	}
	item.MediaFiles = s.primaryMediaFileForPlaybackContext(ctx, item.ID, item.SourceURL)
	profile := optimizedProfileFromJob(job, s.optimizedVersionSettings().DefaultProfile)
	release, err := s.mediaResourceGovernor().acquireContext(ctx, mediaResourceRequest{cpu: 1, disk: 2, background: true})
	if err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		_ = s.setJobMessage(job.ID, "failed", 100, "Optimized version could not acquire processing capacity.")
		return
	}
	defer release()
	version, err := s.createOptimizedVersion(ctx, job.ID, item, profile)
	if err != nil {
		message := "Optimized version failed for " + item.Title + ": " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("warn", message, map[string]string{"job": job.ID, "media": item.ID})
		return
	}
	message := fmt.Sprintf("Optimized version completed for %s (%s).", item.Title, version.Profile)
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, map[string]string{"job": job.ID, "media": item.ID})
}

func (s *Server) handleOptimizedMediaStream(w http.ResponseWriter, r *http.Request, user User, mediaID string, profile string) {
	if !user.Permissions["playMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to play this media.")
		return
	}
	profile = strings.TrimSpace(profile)
	preset, ok := optimized.Lookup(profile)
	if !ok {
		writeError(w, http.StatusNotFound, "optimized_not_found", "Optimized version was not found.")
		return
	}
	binding, bindingErr := s.playbackPlanForMediaGrant(r.Context(), r, mediaID)
	if bindingErr != nil || binding.OptimizedArtifactID == "" || binding.OptimizedPresetID != preset.ID {
		writeError(w, http.StatusUnauthorized, "media_grant_denied", "This optimized artifact is not authorized by the active playback plan.")
		return
	}
	item, err := s.getMediaBackgroundSourceSeedContext(r.Context(), mediaID)
	if err == nil {
		item.Streams, err = s.listStreamsContext(r.Context(), mediaID)
		item.MediaFiles = s.primaryMediaFileForPlaybackContext(r.Context(), mediaID, item.SourceURL)
	}
	var artifact *optimizedV2ReadyArtifact
	if err == nil {
		var facts mediafacts.Facts
		var digest string
		facts, digest, err = s.mediaFactsForPlayback(r.Context(), item)
		if err == nil {
			var source optimizedV2SourceIdentity
			source, err = optimizedSourceIdentityFromFacts(facts, digest)
			if err == nil {
				artifact, err = s.optimizedV2ReadyForSource(r.Context(), mediaID, preset.ID, source, func(path string, size int64) bool {
					return s.optimizedArtifactUsable(r.Context(), path, size)
				})
			}
		}
	}
	if err != nil || artifact == nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "optimized_not_found", "Optimized version was not found.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "optimized_failed", "Unable to load optimized version.")
			return
		}
		writeError(w, http.StatusNotFound, "optimized_not_found", "Optimized version was not found.")
		return
	}
	if artifact.ID != binding.OptimizedArtifactID {
		writeError(w, http.StatusConflict, "optimized_stale", "The optimized artifact changed. Start playback again.")
		return
	}
	releaseArtifact, leased := s.acquireOptimizedArtifactLease(artifact.ID)
	if !leased {
		writeError(w, http.StatusConflict, "optimized_stale", "The optimized artifact is being replaced or removed. Start playback again.")
		return
	}
	defer releaseArtifact()
	path := artifact.Path
	clean := filepath.Clean(path)
	if !s.optimizedVersionPathAllowed(clean) {
		writeError(w, http.StatusForbidden, "forbidden", "Optimized version path is outside configured optimized-version storage.")
		return
	}
	w.Header().Set("Content-Type", optimizedArtifactContentType(artifact.Container))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	delivery := &playbackResponseWriter{ResponseWriter: w}
	if err := s.serveLocalPlaybackFile(delivery, r, clean, filepath.Base(clean)); err != nil && !delivery.committed {
		writePlaybackSourceError(delivery, "optimized_stream_failed", err)
	}
}

func optimizedArtifactContentType(container string) string {
	switch strings.ToLower(strings.TrimSpace(container)) {
	case "mp4", "mov,mp4,m4a,3gp,3g2,mj2":
		return "video/mp4"
	case "matroska", "mkv":
		return "video/x-matroska"
	default:
		return "application/octet-stream"
	}
}

func optimizedProfileFromJob(job Job, fallback string) string {
	if job.Metadata != nil {
		if profile := strings.TrimSpace(job.Metadata["profile"]); profile != "" {
			return profile
		}
	}
	return strings.TrimSpace(fallback)
}

func (s *Server) requestedOptimizedProfile(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || normalized == "default" || normalized == "optimized" {
		return s.optimizedVersionSettings().DefaultProfile
	}
	return normalized
}

func (s *Server) createOptimizedVersion(ctx context.Context, jobID string, item MediaItem, profile string) (OptimizedVersion, error) {
	if item.Type == "show" || item.Type == "anime" || item.Type == "season" {
		return OptimizedVersion{}, errors.New("optimized versions can only be created for playable media items")
	}
	sourcePath, err := s.localSourcePathForTranscode(item)
	if err != nil {
		return OptimizedVersion{}, err
	}
	if _, err := exec.LookPath(s.cfg.FFmpegPath); err != nil && filepath.Base(s.cfg.FFmpegPath) == s.cfg.FFmpegPath {
		return OptimizedVersion{}, errors.New("FFmpeg is not available on PATH")
	}
	profile = strings.TrimSpace(profile)
	preset, ok := optimized.Lookup(profile)
	if !ok {
		return OptimizedVersion{}, fmt.Errorf("unknown optimized preset %q", profile)
	}
	facts, factsDigest, err := s.mediaFactsForPlayback(ctx, item)
	if err != nil {
		return OptimizedVersion{}, fmt.Errorf("load analyzed source facts: %w", err)
	}
	source, err := optimizedSourceIdentityFromFacts(facts, factsDigest)
	if err != nil {
		return OptimizedVersion{}, err
	}
	if ready, readyErr := s.optimizedV2ReadyForSource(ctx, item.ID, profile, source, func(path string, size int64) bool {
		return s.optimizedArtifactUsable(ctx, path, size)
	}); readyErr != nil {
		return OptimizedVersion{}, readyErr
	} else if ready != nil {
		return s.optimizedVersionByIDContext(ctx, ready.ID)
	}
	route, hardwarePlan := s.optimizedEncoderRoute(ctx, preset, source, facts, sourcePath)
	executablePath := ""
	if hardwarePlan != nil {
		if !s.playbackHardwareExecutionIdentityMatches(ctx, hardwarePlan) {
			return OptimizedVersion{}, errors.New("optimized hardware execution identity changed")
		}
		executablePath = hardwarePlan.RuntimeIdentity.ExecutablePath
	} else {
		resolved, resolveErr := exec.LookPath(strings.TrimSpace(firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg")))
		if resolveErr != nil {
			return OptimizedVersion{}, errors.New("FFmpeg is not available")
		}
		executablePath, err = filepath.Abs(resolved)
		if err != nil {
			return OptimizedVersion{}, errors.New("FFmpeg path is invalid")
		}
	}
	publication, err := newOptimizedV2Publication(s, s.optimizedVersionStorageDir(), item.ID, profile, route, source)
	if err != nil {
		return OptimizedVersion{}, err
	}
	if len(facts.Audio) == 0 {
		return OptimizedVersion{}, errors.New("optimized source has no analyzed audio stream")
	}
	args, err := optimizedFFmpegArgs(publication.plan, sourcePath, hardwarePlan, facts.Audio[0])
	if err != nil {
		return OptimizedVersion{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Hour)
	defer cancel()
	_ = s.setJobMessage(jobID, "running", 35, "FFmpeg is creating the optimized version.")
	predictedBytes := predictedOptimizedOutputBytes(publication.plan, facts, item.DurationSeconds)
	sourceLease, err := s.acquirePlaybackStorageLease(ctx, sourcePath, playbackStorageOptimization, "read optimized source")
	if err != nil {
		return OptimizedVersion{}, err
	}
	leaseOutcome := error(nil)
	defer func() { sourceLease.Release(leaseOutcome) }()
	result, err := publication.Publish(ctx, optimizedLocalFilesystem{root: s.optimizedVersionStorageDir(), server: s}, predictedBytes,
		func(runCtx context.Context, output io.Writer) error {
			return s.runOptimizedFFmpeg(runCtx, item.ID, publication.identity.GenerationID, executablePath, sourcePath, args, output, sourceLease.Progress, hardwarePlan)
		}, func(validateCtx context.Context, path string) (optimizedV2OutputFacts, error) {
			probe, probeErr := s.validateOptimizedOutput(validateCtx, path, item.DurationSeconds)
			if probeErr != nil {
				return optimizedV2OutputFacts{}, probeErr
			}
			outputFacts, factsErr := optimizedOutputFactsFromProbe(path, probe)
			if factsErr != nil {
				return optimizedV2OutputFacts{}, factsErr
			}
			if factsErr = validateOptimizedOutputAgainstPlan(outputFacts, publication.plan); factsErr != nil {
				return optimizedV2OutputFacts{}, factsErr
			}
			return outputFacts, nil
		})
	if err != nil {
		leaseOutcome = err
		return OptimizedVersion{}, err
	}
	sourceLease.Release(nil)
	version, err := s.optimizedVersionByIDContext(ctx, result.Metadata.GenerationID)
	if err != nil {
		return OptimizedVersion{}, err
	}
	if removed, err := s.pruneOptimizedVersions(); err != nil {
		s.log.Warn("optimized version retention failed", "error", err)
	} else if removed > 0 {
		s.recordLog("info", "Optimized version retention cleanup completed", map[string]string{"removed": strconv.Itoa(removed)})
	}
	_ = s.setJobMessage(jobID, "running", 92, "Optimized version stored.")
	return version, nil
}

func (s *Server) validateOptimizedOutput(ctx context.Context, path string, sourceDurationSeconds int) (ffprobePayload, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ffprobePayload{}, err
	}
	if info.IsDir() || info.Size() <= 0 {
		return ffprobePayload{}, errors.New("optimized output is empty")
	}
	if _, err := exec.LookPath(s.cfg.FFprobePath); err != nil && filepath.Base(s.cfg.FFprobePath) == s.cfg.FFprobePath {
		return ffprobePayload{}, errors.New("ffprobe is not available on PATH")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.FFprobePath,
		"-v", "error",
		"-protocol_whitelist", "file,pipe",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	diagnosticRecorder := newFFmpegDiagnosticRecorder(s.cfg.FFprobePath, []string{
		"-v", "error", "-protocol_whitelist", "file,pipe", "-print_format", "json",
		"-show_format", "-show_streams", path,
	})
	cmd.Stderr = diagnosticRecorder
	output, err := cmd.Output()
	if err != nil {
		message := diagnosticRecorder.Report(err).Text
		if message != "" {
			return ffprobePayload{}, fmt.Errorf("ffprobe optimized output failed: %w: %s", err, message)
		}
		return ffprobePayload{}, fmt.Errorf("ffprobe optimized output failed: %w", err)
	}
	var payload ffprobePayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return ffprobePayload{}, err
	}
	if !optimizedOutputLooksPlayable(payload, sourceDurationSeconds) {
		return ffprobePayload{}, errors.New("optimized output does not contain playable media streams")
	}
	return payload, nil
}

func optimizedSourceIdentityFromFacts(facts mediafacts.Facts, digest string) (optimizedV2SourceIdentity, error) {
	if strings.TrimSpace(digest) == "" || strings.TrimSpace(facts.Source.Revision) == "" || strings.TrimSpace(facts.Source.Fingerprint) == "" {
		return optimizedV2SourceIdentity{}, errors.New("optimized source identity is incomplete")
	}
	if len(facts.Video) == 0 {
		return optimizedV2SourceIdentity{}, errors.New("optimized video presets require an analyzed video stream")
	}
	v := facts.Video[0]
	sarNum, sarDen := int(v.SampleAspectRatio.Num), int(v.SampleAspectRatio.Den)
	if sarNum < 1 || sarDen < 1 {
		sarNum, sarDen = 1, 1
	}
	dynamicRange := optimized.RangeSDR
	switch v.DynamicRange() {
	case mediafacts.DynamicRangePQ:
		dynamicRange = optimized.RangeHDR10
	case mediafacts.DynamicRangeHLG:
		dynamicRange = optimized.RangeHLG
	case mediafacts.DynamicRangeHDR10Plus:
		dynamicRange = optimized.RangeHDR10Plus
	case mediafacts.DynamicRangeDolbyVision:
		dynamicRange = optimized.RangeDolbyVision
	}
	sourceFacts := optimized.SourceFacts{Width: v.CodedWidth, Height: v.CodedHeight, SARNumerator: sarNum,
		SARDenominator: sarDen, Rotation: v.Rotation,
		Interlaced: strings.TrimSpace(v.FieldOrder) != "" && !strings.EqualFold(v.FieldOrder, "progressive"), DynamicRange: dynamicRange}
	if v.DolbyVision != nil {
		sourceFacts.DolbyVisionProfile = strconv.Itoa(v.DolbyVision.Profile)
		if v.DolbyVision.BaseLayerPresentKnown && v.DolbyVision.BaseLayerPresent {
			switch strings.ToLower(strings.TrimSpace(v.DolbyVision.Fallback)) {
			case "hdr10", "pq":
				sourceFacts.VerifiedBaseRange = optimized.RangeHDR10
			case "hlg":
				sourceFacts.VerifiedBaseRange = optimized.RangeHLG
			case "sdr":
				sourceFacts.VerifiedBaseRange = optimized.RangeSDR
			}
		}
	}
	if len(facts.Audio) > 0 {
		a := facts.Audio[0]
		sourceFacts.AudioCodec, sourceFacts.AudioChannels, sourceFacts.AudioLayout = a.Codec, a.Channels, a.Layout
		sourceFacts.AudioHasObjects = strings.TrimSpace(a.ObjectAudio) != ""
	}
	return optimizedV2SourceIdentity{Revision: facts.Source.Revision, Fingerprint: facts.Source.Fingerprint,
		FactsDigest: digest, Facts: sourceFacts}, nil
}

func optimizedFFmpegArgs(plan optimized.OutputPlan, sourcePath string, hardwarePlan *playbackhw.Plan, sourceAudio mediafacts.Audio) ([]string, error) {
	preset, presetOK := optimized.Lookup(plan.PresetID)
	if !presetOK {
		return nil, fmt.Errorf("unknown optimized preset %q", plan.PresetID)
	}
	codecPrefix := map[optimized.VideoCodec]string{optimized.CodecH264: "h264", optimized.CodecHEVC: "hevc", optimized.CodecAV1: "av1"}[preset.VideoCodec]
	encoder := map[optimized.EncoderRoute]string{
		optimized.RouteSoftwareH264: "libx264", optimized.RouteSoftwareHEVC: "libx265", optimized.RouteSoftwareAV1: "libsvtav1",
		optimized.RouteVideoToolbox: codecPrefix + "_videotoolbox", optimized.RouteQSV: codecPrefix + "_qsv",
		optimized.RouteVAAPI: codecPrefix + "_vaapi", optimized.RouteNVENC: codecPrefix + "_nvenc", optimized.RouteAMF: codecPrefix + "_amf",
	}[plan.EncoderRoute]
	if encoder == "" {
		return nil, fmt.Errorf("unsupported optimized encoder route %q", plan.EncoderRoute)
	}
	args := []string{"-hide_banner", "-nostdin", "-y"}
	if plan.EncoderRoute == optimized.RouteVAAPI {
		if hardwarePlan == nil || strings.TrimSpace(hardwarePlan.RuntimeIdentity.DevicePath) == "" {
			return nil, errors.New("verified VAAPI device identity is unavailable")
		}
		args = append(args, "-vaapi_device", hardwarePlan.RuntimeIdentity.DevicePath)
	}
	if plan.EncoderRoute == optimized.RouteQSV {
		if hardwarePlan == nil || strings.TrimSpace(hardwarePlan.RuntimeIdentity.DevicePath) == "" {
			return nil, errors.New("verified QSV device identity is unavailable")
		}
		args = append(args, "-init_hw_device", "qsv=portico_hw:"+hardwarePlan.RuntimeIdentity.DevicePath, "-filter_hw_device", "portico_hw")
	}
	if plan.Geometry.Rotation != 0 {
		args = append(args, "-noautorotate")
	}
	args = append(args, "-protocol_whitelist", "file,pipe", "-i", sourcePath,
		"-map", "0:v:0", "-map", "0:a:0?", "-map_metadata", "0", "-map_chapters", "0")
	filters := []string{}
	if plan.Deinterlace {
		filters = append(filters, "bwdif=mode=send_frame:parity=auto:deint=interlaced")
	}
	switch plan.Geometry.Rotation {
	case 0:
	case 90:
		filters = append(filters, "transpose=clock")
	case 180:
		filters = append(filters, "hflip", "vflip")
	case 270:
		filters = append(filters, "transpose=cclock")
	default:
		return nil, errors.New("optimized plan contains an unsupported rotation")
	}
	if plan.HDRAction == optimized.HDRToneMapSDR {
		algorithm := safeToneMappingAlgorithm(plan.ToneMapAlgorithm)
		filters = append(filters, "zscale=t=linear:npl=100", "tonemap="+algorithm+":desat=0", "zscale=p=bt709:t=bt709:m=bt709:r=tv")
	} else if plan.HDRAction == optimized.HDRDowngradeHDR10 {
		filters = append(filters, "zscale=p=bt2020:t=smpte2084:m=bt2020nc:r=tv")
	} else if plan.HDRAction == optimized.HDRUnsupported || plan.DolbyVisionAction == optimized.DVUnsupported {
		return nil, errors.New("source color format is unsupported by the selected optimized preset")
	}
	filters = append(filters, fmt.Sprintf("scale=%d:%d:flags=lanczos", plan.Geometry.Width, plan.Geometry.Height), "setsar=1")
	if plan.EncoderRoute == optimized.RouteVAAPI || plan.EncoderRoute == optimized.RouteQSV {
		pixelFormat := "nv12"
		if plan.DynamicRange == optimized.RangeHDR10 {
			pixelFormat = "p010le"
		}
		filters = append(filters, "format="+pixelFormat, "hwupload=extra_hw_frames=64")
	}
	args = append(args, "-vf", strings.Join(filters, ","), "-c:v", encoder)
	if plan.Quality.Control != "" {
		args = append(args, "-"+plan.Quality.Control, strconv.Itoa(plan.Quality.Value))
	}
	if plan.Quality.Speed != "" && strings.HasPrefix(encoder, "lib") {
		args = append(args, "-preset", plan.Quality.Speed)
	}
	audioArgs, err := ffmpeggraph.CompileAudio(ffmpeggraph.AudioRequest{
		InputCodec: sourceAudio.Codec, InputLayout: sourceAudio.Layout, InputChannels: sourceAudio.Channels, InputSampleRate: sourceAudio.SampleRate,
		OutputCodec: plan.Audio.Codec, OutputLayout: plan.Audio.Layout, OutputChannels: plan.Audio.Channels, Copy: plan.Audio.Copy,
	})
	if err != nil {
		return nil, fmt.Errorf("compile optimized audio graph: %w", err)
	}
	args = append(args, audioArgs...)
	switch plan.DynamicRange {
	case optimized.RangeSDR:
		args = append(args, "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709")
	case optimized.RangeHDR10:
		args = append(args, "-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc")
		if encoder == "libx265" {
			args = append(args, "-x265-params", "colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:range=limited:hdr10=1:repeat-headers=1")
		}
	case optimized.RangeHLG:
		args = append(args, "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc")
		if encoder == "libx265" {
			args = append(args, "-x265-params", "colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc:range=limited:repeat-headers=1")
		}
	}
	format := "matroska"
	if strings.HasPrefix(plan.PresetID, "universal-") {
		format = "mp4"
		args = append(args, "-movflags", "+frag_keyframe+empty_moov+default_base_moof")
	}
	return append(args, "-f", format, "pipe:1"), nil
}

func (s *Server) optimizedEncoderRoute(ctx context.Context, preset optimized.Preset, source optimizedV2SourceIdentity, facts mediafacts.Facts, sourcePath string) (optimized.EncoderRoute, *playbackhw.Plan) {
	software := map[optimized.VideoCodec]optimized.EncoderRoute{
		optimized.CodecH264: optimized.RouteSoftwareH264,
		optimized.CodecHEVC: optimized.RouteSoftwareHEVC,
		optimized.CodecAV1:  optimized.RouteSoftwareAV1,
	}[preset.VideoCodec]
	if !optimizedRouteAllowed(preset, software) {
		return preset.EncoderRoutes[0], nil
	}
	softwarePlan, err := optimized.PlanForRoute(preset, source.Facts, software)
	if err != nil {
		return software, nil
	}
	color := &playbackplan.ColorDecision{Input: string(source.Facts.DynamicRange), Output: string(softwarePlan.DynamicRange), Action: string(softwarePlan.HDRAction)}
	probePlan := playbackplan.Plan{
		Mode: playbackplan.VideoTranscode,
		Streams: []playbackplan.StreamAction{{Index: facts.Video[0].Index, Kind: "video", Action: playbackplan.Convert,
			InputCodec: facts.Video[0].Codec, OutputCodec: string(preset.VideoCodec)}},
		Color: color, Constraints: playbackplan.Constraints{MaxWidth: softwarePlan.Geometry.Width, MaxHeight: softwarePlan.Geometry.Height},
	}
	_, executable := s.resolvePlaybackHardwareRoute(ctx, s.transcodeSettings(), facts, probePlan, sourcePath)
	if executable == nil {
		return software, nil
	}
	route := map[playbackhw.Backend]optimized.EncoderRoute{
		playbackhw.VideoToolbox: optimized.RouteVideoToolbox,
		playbackhw.QSV:          optimized.RouteQSV,
		playbackhw.VAAPI:        optimized.RouteVAAPI,
		playbackhw.NVIDIA:       optimized.RouteNVENC,
		playbackhw.AMF:          optimized.RouteAMF,
	}[executable.Backend]
	if !optimizedRouteAllowed(preset, route) {
		return software, nil
	}
	return route, executable
}

func optimizedRouteAllowed(preset optimized.Preset, route optimized.EncoderRoute) bool {
	for _, candidate := range preset.EncoderRoutes {
		if candidate == route {
			return true
		}
	}
	return false
}

type optimizedProgressWriter struct {
	writer   io.Writer
	progress func()
}

func (w optimizedProgressWriter) Write(value []byte) (int, error) {
	n, err := w.writer.Write(value)
	if n > 0 && w.progress != nil {
		w.progress()
	}
	return n, err
}

func (s *Server) runOptimizedFFmpeg(ctx context.Context, mediaID, generationID, executablePath, sourcePath string, args []string, output io.Writer, progress func(), hardwarePlan *playbackhw.Plan) error {
	if s.ffmpegSupervisor == nil {
		return errors.New("FFmpeg supervisor is unavailable")
	}
	if !filepath.IsAbs(executablePath) {
		return errors.New("optimized FFmpeg executable identity is invalid")
	}
	if hardwarePlan != nil && !s.playbackHardwareExecutionIdentityMatches(ctx, hardwarePlan) {
		return errors.New("optimized hardware execution identity changed")
	}
	done := make(chan error, 1)
	diagnostics := make(chan ffmpegDiagnosticReport, 1)
	diagnosticRecorder := newFFmpegDiagnosticRecorder(executablePath, args)
	key := mediaID + ":" + generationID
	generation, err := s.ffmpegSupervisor.Launch(transcodeLaunchV2{Kind: transcodeWorkOptimizationV2, Key: key, Mode: ffmpegsupervisor.ModeVOD,
		Start: supervisedExecFactoryV2(func(runCtx context.Context) (*exec.Cmd, error) {
			cmd := exec.CommandContext(runCtx, executablePath, args...)
			cmd.Stdout = optimizedProgressWriter{writer: output, progress: progress}
			cmd.Stderr = diagnosticRecorder
			cmd.Dir = filepath.Dir(sourcePath)
			return cmd, nil
		}), Release: func(release ffmpegsupervisor.Release) {
			diagnostics <- diagnosticRecorder.Report(release.Err)
			done <- release.Err
		}})
	if err != nil {
		return err
	}
	select {
	case err := <-done:
		report := <-diagnostics
		if err != nil {
			s.recordLog("warn", "Optimized FFmpeg execution failed", map[string]string{
				"media": mediaID, "generation": generationID, "command": report.CommandIdentity,
				"stderrBytes": strconv.FormatInt(report.Bytes, 10), "stderrLines": strconv.FormatInt(report.Lines, 10),
				"stderrTruncated": strconv.FormatBool(report.Truncated), "errorLines": strconv.FormatInt(report.ErrorLines, 10),
				"exitCode": strconv.Itoa(report.ExitCode), "signal": report.Signal,
			})
		}
		return err
	case <-ctx.Done():
		_ = s.ffmpegSupervisor.Stop(generation)
		<-done
		return ctx.Err()
	}
}

func optimizedOutputFactsFromProbe(path string, probe ffprobePayload) (optimizedV2OutputFacts, error) {
	info, err := os.Stat(path)
	if err != nil {
		return optimizedV2OutputFacts{}, err
	}
	body, err := json.Marshal(probe)
	if err != nil {
		return optimizedV2OutputFacts{}, err
	}
	f := optimizedV2OutputFacts{SizeBytes: info.Size(), Container: containerFromFFprobe(probe, path),
		DurationSeconds: ffprobeDurationSeconds(probe.Format.Duration), FactsJSON: body, FactsDigest: optimizedV2Digest(body)}
	f.Bitrate, _ = strconv.Atoi(strings.TrimSpace(probe.Format.BitRate))
	for _, stream := range probe.Streams {
		switch strings.ToLower(strings.TrimSpace(stream.CodecType)) {
		case "video":
			if f.VideoCodec == "" {
				f.VideoCodec, f.Width, f.Height = stream.CodecName, stream.Width, stream.Height
				f.SampleAspectRatio, f.FieldOrder, f.PixelFormat = stream.SampleAspectRatio, stream.FieldOrder, stream.PixelFormat
				f.ColorPrimaries, f.ColorTransfer, f.ColorMatrix = stream.ColorPrimaries, stream.ColorTransfer, stream.ColorSpace
			}
		case "audio":
			if f.AudioCodec == "" {
				f.AudioCodec, f.AudioChannels, f.AudioLayout = stream.CodecName, stream.Channels, stream.ChannelLayout
			}
		}
	}
	return f, nil
}

func validateOptimizedOutputAgainstPlan(f optimizedV2OutputFacts, plan optimized.OutputPlan) error {
	expectedContainer := map[string]string{"universal-1080p": "mp4", "universal-720p": "mp4", "universal-480p": "mp4"}[plan.PresetID]
	if expectedContainer == "" {
		expectedContainer = "matroska"
	}
	container := strings.ToLower(strings.TrimSpace(f.Container))
	if expectedContainer == "matroska" {
		if container != "matroska" && container != "mkv" {
			return fmt.Errorf("optimized output container %q does not match sealed plan", f.Container)
		}
	} else if container != expectedContainer && container != "mov,mp4,m4a,3gp,3g2,mj2" {
		return fmt.Errorf("optimized output container %q does not match sealed plan", f.Container)
	}
	expectedVideo := map[optimized.VideoCodec][]string{optimized.CodecH264: {"h264", "avc"}, optimized.CodecHEVC: {"hevc", "h265"}, optimized.CodecAV1: {"av1"}}[func() optimized.VideoCodec {
		p, _ := optimized.Lookup(plan.PresetID)
		return p.VideoCodec
	}()]
	videoOK := false
	for _, candidate := range expectedVideo {
		if strings.EqualFold(f.VideoCodec, candidate) {
			videoOK = true
		}
	}
	if !videoOK || f.Width != plan.Geometry.Width || f.Height != plan.Geometry.Height || f.SampleAspectRatio != "1:1" {
		return errors.New("optimized output video facts do not match the sealed plan")
	}
	if plan.Deinterlace && strings.TrimSpace(f.FieldOrder) != "" && !strings.EqualFold(f.FieldOrder, "progressive") {
		return errors.New("optimized output remains interlaced despite the sealed plan")
	}
	if !strings.EqualFold(f.AudioCodec, plan.Audio.Codec) || f.AudioChannels != plan.Audio.Channels || !strings.EqualFold(f.AudioLayout, plan.Audio.Layout) {
		return errors.New("optimized output audio codec does not match the sealed plan")
	}
	switch plan.DynamicRange {
	case optimized.RangeSDR:
		if !strings.EqualFold(f.ColorPrimaries, "bt709") || !strings.EqualFold(f.ColorTransfer, "bt709") || !strings.EqualFold(f.ColorMatrix, "bt709") {
			return errors.New("optimized SDR output lacks BT.709 color facts")
		}
	case optimized.RangeHDR10:
		if !strings.EqualFold(f.ColorPrimaries, "bt2020") || !strings.EqualFold(f.ColorTransfer, "smpte2084") || !strings.EqualFold(f.ColorMatrix, "bt2020nc") || !strings.Contains(strings.ToLower(f.PixelFormat), "10") {
			return errors.New("optimized HDR10 output lacks exact 10-bit PQ/BT.2020 facts")
		}
	case optimized.RangeHLG:
		if !strings.EqualFold(f.ColorPrimaries, "bt2020") || !strings.EqualFold(f.ColorTransfer, "arib-std-b67") || !strings.EqualFold(f.ColorMatrix, "bt2020nc") || !strings.Contains(strings.ToLower(f.PixelFormat), "10") {
			return errors.New("optimized HLG output lacks exact 10-bit HLG/BT.2020 facts")
		}
	}
	return nil
}

func (s *Server) optimizedVersionByIDContext(ctx context.Context, id string) (OptimizedVersion, error) {
	var v OptimizedVersion
	err := s.queryUserRow(ctx, `SELECT id, media_id, profile, path, size_bytes, created_at, updated_at,
		container, video_codec, audio_codec, width, height, bitrate, duration_seconds
		FROM optimized_versions WHERE id = ? AND state = 'ready'`, id).Scan(&v.ID, &v.MediaID, &v.Profile, &v.Path,
		&v.SizeBytes, &v.CreatedAt, &v.UpdatedAt, &v.Container, &v.VideoCodec, &v.AudioCodec, &v.Width, &v.Height,
		&v.Bitrate, &v.DurationSeconds)
	if err != nil {
		return OptimizedVersion{}, err
	}
	if p, ok := optimized.Lookup(v.Profile); ok {
		v.ProfileName = p.Label
	}
	v.DownloadURL, v.StreamURL, v.Available = optimizedDownloadURL(v.MediaID, v.Profile), optimizedStreamURL(v.MediaID, v.Profile), true
	return v, nil
}

func optimizedOutputLooksPlayable(payload ffprobePayload, sourceDurationSeconds int) bool {
	durationSeconds := ffprobeDurationSeconds(payload.Format.Duration)
	if sourceDurationSeconds > 0 {
		minDuration := max(1, sourceDurationSeconds/2)
		if durationSeconds < minDuration {
			return false
		}
	} else if durationSeconds <= 0 {
		return false
	}
	for _, stream := range payload.Streams {
		if strings.EqualFold(stream.CodecType, "video") || strings.EqualFold(stream.CodecType, "audio") {
			return true
		}
	}
	return false
}

func ffprobeDurationSeconds(value string) int {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return int(math.Round(seconds))
}

func (s *Server) handleOptimizedVersions(w http.ResponseWriter, r *http.Request, user User, mediaID string) {
	switch r.Method {
	case http.MethodGet:
		if !user.Permissions["playMedia"] {
			writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view optimized versions.")
			return
		}
		if _, err := s.getMediaAccessSummaryContext(r.Context(), viewerProfileID(user), mediaID); err != nil {
			writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
			return
		}
		versions, err := s.optimizedVersionsForMedia(mediaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "optimized_failed", "Unable to load optimized versions.")
			return
		}
		writeJSON(w, http.StatusOK, OptimizedVersionListResponse{
			Items:          optimizedVersionProjection(versions, user),
			Profiles:       s.optimizedVersionProfiles(),
			DefaultProfile: s.optimizedVersionSettings().DefaultProfile,
			Total:          len(versions),
		})
	case http.MethodPost:
		if !canInteractivelyManageServer(user) {
			writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to create optimized versions.")
			return
		}
		var req OptimizedVersionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		item, err := s.getMediaAccessSummaryContext(r.Context(), viewerProfileID(user), mediaID)
		if err != nil {
			writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
			return
		}
		profile := s.requestedOptimizedProfile(req.Profile)
		preset, ok := optimized.Lookup(profile)
		if !ok {
			writeError(w, http.StatusBadRequest, "optimized_profile_invalid", "Select one of the available optimized presets.")
			return
		}
		job, err := s.createJobForWithMetadata("optimize_version", fmt.Sprintf("Optimized %s version queued for %s.", preset.Label, item.Title), "media", item.ID, map[string]string{"profile": profile})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "job_failed", "Unable to queue optimized version.")
			return
		}
		s.recordAudit(r, user, "media.optimize_queued", "media", mediaID, "info", map[string]string{"profile": profile})
		writeJSON(w, http.StatusCreated, job)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
	}
}

func optimizedVersionProjection(versions []OptimizedVersion, user User) []OptimizedVersion {
	if canInteractivelyManageServer(user) {
		return versions
	}
	projected := append([]OptimizedVersion(nil), versions...)
	for index := range projected {
		projected[index].Path = ""
	}
	return projected
}

func (s *Server) handleDeleteOptimizedVersion(w http.ResponseWriter, r *http.Request, user User, mediaID string, profile string) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use DELETE for this endpoint.")
		return
	}
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to delete optimized versions.")
		return
	}
	if _, err := s.getMediaAccessSummaryContext(r.Context(), viewerProfileID(user), mediaID); err != nil {
		writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
		return
	}
	if err := s.deleteOptimizedVersionContext(r.Context(), mediaID, profile); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "optimized_not_found", "Optimized version was not found.")
			return
		}
		if errors.Is(err, errOptimizedArtifactLeased) {
			writeError(w, http.StatusConflict, "optimized_in_use", "This optimized version is currently in use. Try again after playback or download finishes.")
			return
		}
		writeError(w, http.StatusInternalServerError, "optimized_failed", "Unable to delete optimized version.")
		return
	}
	s.recordAudit(r, user, "media.optimized_deleted", "media", mediaID, "info", map[string]string{"profile": strings.TrimSpace(profile)})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) optimizedVersionsForMedia(mediaID string) ([]OptimizedVersion, error) {
	return s.optimizedVersionsForMediaContext(context.Background(), mediaID)
}

func (s *Server) optimizedVersionsForMediaContext(ctx context.Context, mediaID string) ([]OptimizedVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	item, err := s.getMediaBackgroundSourceSeedContext(ctx, mediaID)
	if err != nil {
		return []OptimizedVersion{}, nil
	}
	item.Streams, _ = s.listStreamsContext(ctx, mediaID)
	item.MediaFiles = s.primaryMediaFileForPlaybackContext(ctx, mediaID, item.SourceURL)
	facts, digest, err := s.mediaFactsForPlayback(ctx, item)
	if err != nil {
		return []OptimizedVersion{}, nil
	}
	source, err := optimizedSourceIdentityFromFacts(facts, digest)
	if err != nil {
		return []OptimizedVersion{}, nil
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT id, media_id, profile, path, size_bytes, created_at, updated_at,
			container, video_codec, audio_codec, width, height, bitrate, duration_seconds,
			preset_version, output_facts_digest, output_facts_json
		FROM optimized_versions
		WHERE media_id = ? AND state = 'ready' AND planner_revision = ?
		  AND source_revision = ? AND source_fingerprint = ? AND source_facts_digest = ?
		  AND plan_digest <> '' AND plan_json <> '{}' AND output_facts_digest <> ''
		ORDER BY updated_at DESC, profile ASC`, mediaID, optimized.PlannerRevision, source.Revision, source.Fingerprint, source.FactsDigest)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := []OptimizedVersion{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return versions, err
		}
		var version OptimizedVersion
		var presetVersion int
		var outputFactsDigest, outputFactsJSON string
		if err := rows.Scan(&version.ID, &version.MediaID, &version.Profile, &version.Path, &version.SizeBytes, &version.CreatedAt, &version.UpdatedAt,
			&version.Container, &version.VideoCodec, &version.AudioCodec, &version.Width, &version.Height, &version.Bitrate, &version.DurationSeconds,
			&presetVersion, &outputFactsDigest, &outputFactsJSON); err != nil {
			return nil, err
		}
		preset, ok := optimized.Lookup(version.Profile)
		if !ok || preset.Version != presetVersion || !json.Valid([]byte(outputFactsJSON)) || optimizedV2Digest([]byte(outputFactsJSON)) != outputFactsDigest ||
			!s.optimizedArtifactUsable(ctx, version.Path, version.SizeBytes) {
			continue
		}
		version.ProfileName = preset.Label
		version.DownloadURL = optimizedDownloadURL(version.MediaID, version.Profile)
		version.StreamURL = optimizedStreamURL(version.MediaID, version.Profile)
		version.Available = true
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Server) deleteOptimizedVersion(mediaID, profile string) error {
	return s.deleteOptimizedVersionContext(context.Background(), mediaID, profile)
}

func (s *Server) deleteOptimizedVersionContext(ctx context.Context, mediaID, profile string) error {
	profile = strings.TrimSpace(profile)
	if _, ok := optimized.Lookup(profile); !ok {
		return sql.ErrNoRows
	}
	var id, path string
	err := s.queryUserRow(ctx, `SELECT id, path FROM optimized_versions WHERE media_id = ? AND profile = ? AND state = 'ready' ORDER BY updated_at DESC LIMIT 1`, mediaID, profile).Scan(&id, &path)
	if err != nil {
		return err
	}
	releaseDeletion, claimed := s.claimOptimizedArtifactDeletion(id)
	if !claimed {
		return errOptimizedArtifactLeased
	}
	defer releaseDeletion()
	result, err := s.execUserWrite(ctx, `UPDATE optimized_versions SET state = 'deleting', updated_at = ? WHERE id = ? AND state = 'ready'`, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	if rowsAffected(result) != 1 {
		return sql.ErrNoRows
	}
	clean := filepath.Clean(path)
	if s.optimizedVersionPathAllowed(clean) {
		root := ""
		for _, candidate := range s.optimizedVersionStorageRoots() {
			if pathInsideRoot(clean, candidate) {
				root = candidate
				break
			}
		}
		if root == "" || validateOptimizedManagedPath(root, clean, true, false) != nil {
			return errors.New("optimized artifact path is not trusted")
		}
		if err := (optimizedLocalFilesystem{root: root, server: s}).Remove(clean); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	_, err = s.execUserWrite(ctx, `DELETE FROM optimized_versions WHERE id = ? AND state = 'deleting'`, id)
	return err
}

func (s *Server) optimizedVersionProfiles() []OptimizedVersionProfile {
	settings := s.optimizedVersionSettings()
	defaultProfile := settings.DefaultProfile
	presets := optimized.List()
	profiles := make([]OptimizedVersionProfile, 0, len(presets))
	for _, preset := range presets {
		profiles = append(profiles, OptimizedVersionProfile{
			ID: preset.ID, Label: preset.Label, Height: preset.MaxHeight, Default: preset.ID == defaultProfile,
		})
	}
	return profiles
}

func optimizedProfileOrder() []string {
	ids := make([]string, 0, 8)
	for _, preset := range optimized.List() {
		ids = append(ids, preset.ID)
	}
	return ids
}

func optimizedProfileInOrder(ids []string, candidate string) bool {
	for _, id := range ids {
		if id == candidate {
			return true
		}
	}
	return false
}

func optimizedDownloadURL(mediaID, profile string) string {
	return "/api/media/" + url.PathEscape(mediaID) + "/download?profile=" + url.QueryEscape(profile)
}

func optimizedStreamURL(mediaID, profile string) string {
	return "/api/media/" + url.PathEscape(mediaID) + "/optimized/" + url.PathEscape(profile)
}

func mediaPlaybackURLForDecision(mediaID string, decision PlaybackDecision) string {
	if decision.Mode == "optimized_version" && decision.execution != nil &&
		strings.TrimSpace(decision.execution.OptimizedArtifactID) != "" && strings.TrimSpace(decision.execution.OptimizedPresetID) != "" {
		return optimizedStreamURL(mediaID, decision.execution.OptimizedPresetID)
	}
	if decision.Mode == "direct_play" {
		return mediaPlaybackStreamURL(mediaID)
	}
	return mediaPlaybackHLSURL(mediaID, "")
}

func (s *Server) pruneOptimizedVersions() (int, error) {
	settings := s.optimizedVersionSettings()
	rows, err := s.queryBackgroundRead(context.Background(), `
		SELECT id, media_id, profile, state, path, size_bytes, updated_at, superseded_at
		FROM optimized_versions
		WHERE state IN ('ready', 'superseded')
		ORDER BY media_id ASC, updated_at DESC`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	now := time.Now().UTC()
	var records []optimizedVersionRecord
	removeIDs := map[string]bool{}
	totalBytes := int64(0)
	perItem := map[string]int{}

	for rows.Next() {
		var record optimizedVersionRecord
		if err := rows.Scan(&record.ID, &record.MediaID, &record.Profile, &record.State, &record.Path, &record.SizeBytes, &record.UpdatedAt, &record.SupersededAt); err != nil {
			return 0, err
		}
		cleanPath := filepath.Clean(record.Path)
		root := ""
		for _, candidate := range s.optimizedVersionStorageRoots() {
			if pathInsideRoot(cleanPath, candidate) {
				root = candidate
				break
			}
		}
		if root == "" || validateOptimizedManagedPath(root, cleanPath, false, false) != nil {
			removeIDs[record.ID] = true
			continue
		}
		info, statErr := os.Lstat(cleanPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				removeIDs[record.ID] = true
				continue
			}
			return 0, statErr
		}
		record.Path = cleanPath
		record.SizeBytes = info.Size()
		record.modTime = info.ModTime().UTC()
		if updatedAt, err := time.Parse(time.RFC3339, record.UpdatedAt); err == nil {
			record.modTime = updatedAt.UTC()
		}
		if !s.optimizedVersionPathAllowed(cleanPath) {
			removeIDs[record.ID] = true
			continue
		}
		if record.State == "superseded" {
			preset, ok := optimized.Lookup(record.Profile)
			if !ok {
				removeIDs[record.ID] = true
			} else {
				supersededAt, parseErr := time.Parse(time.RFC3339Nano, record.SupersededAt)
				if parseErr != nil {
					supersededAt, parseErr = time.Parse(time.RFC3339, record.SupersededAt)
				}
				if parseErr != nil || now.Sub(supersededAt) < time.Duration(preset.Artifact.RetainSupersededHours)*time.Hour {
					// A newly superseded generation is a rollback artifact, not
					// part of ordinary owner retention or storage-cap pruning.
					records = append(records, record)
					continue
				}
			}
		}
		if settings.RetentionDays > 0 && record.modTime.Before(now.Add(-time.Duration(settings.RetentionDays)*24*time.Hour)) {
			removeIDs[record.ID] = true
		}
		if record.State == "ready" {
			perItem[record.MediaID]++
		}
		if record.State == "ready" && settings.MaxPerItem > 0 && perItem[record.MediaID] > settings.MaxPerItem {
			removeIDs[record.ID] = true
		}
		records = append(records, record)
		if !removeIDs[record.ID] {
			totalBytes += record.SizeBytes
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	maxStorageBytes := int64(settings.MaxStorageMB) * 1024 * 1024
	if maxStorageBytes > 0 && totalBytes > maxStorageBytes {
		sort.Slice(records, func(i, j int) bool {
			return records[i].modTime.Before(records[j].modTime)
		})
		for _, record := range records {
			if totalBytes <= maxStorageBytes {
				break
			}
			if removeIDs[record.ID] {
				continue
			}
			removeIDs[record.ID] = true
			totalBytes -= record.SizeBytes
		}
	}

	removed := 0
	for _, record := range records {
		if !removeIDs[record.ID] {
			continue
		}
		releaseDeletion, claimed := s.claimOptimizedArtifactDeletion(record.ID)
		if !claimed {
			continue
		}
		result, claimErr := s.execBackgroundWrite(context.Background(), `
			UPDATE optimized_versions SET state = 'deleting', updated_at = ?
			WHERE id = ? AND state = ? AND path = ? AND updated_at = ?`,
			time.Now().UTC().Format(time.RFC3339Nano), record.ID, record.State, record.Path, record.UpdatedAt)
		if claimErr != nil {
			releaseDeletion()
			return removed, claimErr
		}
		if rowsAffected(result) != 1 {
			releaseDeletion()
			continue
		}
		if s.optimizedVersionPathAllowed(record.Path) {
			root := ""
			for _, candidate := range s.optimizedVersionStorageRoots() {
				if pathInsideRoot(record.Path, candidate) {
					root = candidate
					break
				}
			}
			if root == "" {
				releaseDeletion()
				return removed, errors.New("optimized artifact path is not trusted")
			}
			if err := (optimizedLocalFilesystem{root: root, server: s}).Remove(record.Path); err != nil && !os.IsNotExist(err) {
				releaseDeletion()
				return removed, err
			}
		}
		if _, err := s.execBackgroundWrite(context.Background(), `DELETE FROM optimized_versions WHERE id = ? AND state = 'deleting'`, record.ID); err != nil {
			releaseDeletion()
			return removed, err
		}
		releaseDeletion()
		removed++
	}
	for id := range removeIDs {
		found := false
		for _, record := range records {
			if record.ID == id {
				found = true
				break
			}
		}
		if found {
			continue
		}
		if _, err := s.execBackgroundWrite(context.Background(), `DELETE FROM optimized_versions WHERE id = ?`, id); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func pathInsideRoot(path string, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	return cleanPath != cleanRoot && strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator))
}
