package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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

type ffprobePayload struct {
	Format   ffprobeFormat    `json:"format"`
	Streams  []ffprobeStream  `json:"streams"`
	Chapters []ffprobeChapter `json:"chapters"`
}

type ffprobeKeyframePayload struct {
	Frames []struct {
		BestEffortTimestampTime string `json:"best_effort_timestamp_time"`
	} `json:"frames"`
}

type ffprobeFormat struct {
	Duration   string            `json:"duration"`
	StartTime  string            `json:"start_time"`
	BitRate    string            `json:"bit_rate"`
	FormatName string            `json:"format_name"`
	Tags       map[string]string `json:"tags"`
}

type ffprobeStream struct {
	Index             int               `json:"index"`
	CodecType         string            `json:"codec_type"`
	CodecName         string            `json:"codec_name"`
	Profile           string            `json:"profile"`
	Level             int               `json:"level"`
	CodecTagString    string            `json:"codec_tag_string"`
	Duration          string            `json:"duration"`
	StartTime         string            `json:"start_time"`
	TimeBase          string            `json:"time_base"`
	Width             int               `json:"width"`
	Height            int               `json:"height"`
	AverageFrameRate  string            `json:"avg_frame_rate"`
	FrameRate         string            `json:"r_frame_rate"`
	AspectRatio       string            `json:"display_aspect_ratio"`
	SampleAspectRatio string            `json:"sample_aspect_ratio"`
	Channels          int               `json:"channels"`
	ChannelLayout     string            `json:"channel_layout"`
	SampleRate        string            `json:"sample_rate"`
	SampleFormat      string            `json:"sample_fmt"`
	InitialPadding    int64             `json:"initial_padding"`
	TrailingPadding   int64             `json:"trailing_padding"`
	BitRate           string            `json:"bit_rate"`
	PixelFormat       string            `json:"pix_fmt"`
	BitsPerRawSample  string            `json:"bits_per_raw_sample"`
	ColorTransfer     string            `json:"color_transfer"`
	ColorRange        string            `json:"color_range"`
	ColorPrimaries    string            `json:"color_primaries"`
	ColorSpace        string            `json:"color_space"`
	ChromaLocation    string            `json:"chroma_location"`
	FieldOrder        string            `json:"field_order"`
	Disposition       map[string]int    `json:"disposition"`
	Tags              map[string]string `json:"tags"`
	SideDataList      []ffprobeSideData `json:"side_data_list"`
}

func ffprobeFrameRate(raw ffprobeStream) float64 {
	value := firstNonEmpty(raw.AverageFrameRate, raw.FrameRate)
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) == 2 {
		numerator, numeratorErr := strconv.ParseFloat(parts[0], 64)
		denominator, denominatorErr := strconv.ParseFloat(parts[1], 64)
		if numeratorErr == nil && denominatorErr == nil && denominator > 0 {
			return math.Round((numerator/denominator)*1000) / 1000
		}
	}
	parsed, _ := strconv.ParseFloat(value, 64)
	return math.Round(parsed*1000) / 1000
}

type ffprobeSideData struct {
	SideDataType            string  `json:"side_data_type"`
	DVProfile               int     `json:"dv_profile"`
	DVLevel                 int     `json:"dv_level"`
	RPUPresent              *int    `json:"rpu_present_flag"`
	ELPresent               *int    `json:"el_present_flag"`
	BLPresent               *int    `json:"bl_present_flag"`
	BLSignalCompatibilityID int     `json:"dv_bl_signal_compatibility_id"`
	Rotation                float64 `json:"rotation"`
	DisplayMatrix           string  `json:"displaymatrix"`
	RedX                    string  `json:"red_x"`
	RedY                    string  `json:"red_y"`
	GreenX                  string  `json:"green_x"`
	GreenY                  string  `json:"green_y"`
	BlueX                   string  `json:"blue_x"`
	BlueY                   string  `json:"blue_y"`
	WhitePointX             string  `json:"white_point_x"`
	WhitePointY             string  `json:"white_point_y"`
	MinLuminance            string  `json:"min_luminance"`
	MaxLuminance            string  `json:"max_luminance"`
	MaxContent              int     `json:"max_content"`
	MaxAverage              int     `json:"max_average"`
}

type ffprobeChapter struct {
	ID        int               `json:"id"`
	StartTime string            `json:"start_time"`
	EndTime   string            `json:"end_time"`
	Tags      map[string]string `json:"tags"`
}

const (
	mediaAnalysisModeFull  = "full"
	mediaAnalysisModeProbe = "probe"
	mediaThumbnailVersion  = "representative-midpoint-v1"
)

type mediaAnalysisOptions struct {
	Mode                       string
	ProbeStreams               bool
	ReadEmbeddedTags           bool
	ReadEmbeddedIndexes        bool
	ExtractEmbeddedAttachments bool
	GenerateThumbnails         bool
	ChapterThumbnails          bool
	GenerateTrickplay          bool
	AnalyzeAudio               bool
	SonicFingerprinting        bool
	FullFileChecksum           bool
	GenerateWaveforms          bool
	ValidateSeekBehavior       bool
	FullSeekValidation         bool
	DetectSegments             bool
	ExtractEmbeddedCovers      bool
	AnalyzeSTRMTarget          bool
	ExpectedSourceRevision     string
}

func normalizeMediaAnalysisMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case mediaAnalysisModeProbe:
		return mediaAnalysisModeProbe
	default:
		return mediaAnalysisModeFull
	}
}

func mediaAnalysisMetadata(mode string) map[string]string {
	return map[string]string{"analysisMode": normalizeMediaAnalysisMode(mode)}
}

func (s *Server) mediaAnalysisMetadataForItem(ctx context.Context, item MediaItem, mode string) map[string]string {
	metadata := mediaAnalysisMetadata(mode)
	if revision, err := s.currentMediaAnalysisSourceRevision(ctx, item); err == nil && strings.TrimSpace(revision) != "" {
		metadata["sourceRevision"] = revision
	}
	return metadata
}

func representativeFrameAnalysisMetadata() map[string]string {
	return map[string]string{"analysisMode": mediaAnalysisModeProbe, "representativeFrame": "true"}
}

func mediaAnalysisModeFromJob(job Job) string {
	if job.Metadata == nil {
		return mediaAnalysisModeFull
	}
	return normalizeMediaAnalysisMode(job.Metadata["analysisMode"])
}

func (s *Server) mediaAnalysisOptions(item MediaItem, mode string) mediaAnalysisOptions {
	mode = normalizeMediaAnalysisMode(mode)
	full := mode == mediaAnalysisModeFull
	settings := map[string]any(nil)
	tier := s.analysisTierForItem(item)
	if strings.TrimSpace(item.LibraryID) != "" {
		if library, err := s.getLibrary(item.LibraryID); err == nil {
			settings = s.libraryAnalysisSettingsFor(library)
		}
	}
	custom := tier == analysisTierCustom
	complete := tier == analysisTierComplete
	customEnabled := func(key string) bool { return custom && settingBool(settings, key, false) }
	probeStreams := !custom || customEnabled("probeStreams")
	readEmbeddedIndexes := probeStreams && (!custom || customEnabled("readEmbeddedIndexes"))
	allowFull := full && (complete || custom)
	options := mediaAnalysisOptions{
		Mode:                       mode,
		ProbeStreams:               probeStreams,
		ReadEmbeddedTags:           probeStreams && (!custom || customEnabled("readEmbeddedTags")),
		ReadEmbeddedIndexes:        readEmbeddedIndexes,
		ExtractEmbeddedAttachments: allowFull && readEmbeddedIndexes && (complete || customEnabled("extractAllEmbeddedAttachments")),
		GenerateThumbnails:         allowFull && probeStreams && (complete || customEnabled("generateRepresentativeThumbnail")),
		ChapterThumbnails:          allowFull && readEmbeddedIndexes && (complete || customEnabled("generateChapterThumbnails")),
		GenerateTrickplay:          allowFull && probeStreams && (complete || customEnabled("generateTrickplay")),
		AnalyzeAudio:               allowFull && probeStreams && (complete || customEnabled("analyzeLoudness")),
		SonicFingerprinting:        allowFull && probeStreams && (complete || customEnabled("sonicFingerprinting")),
		FullFileChecksum:           allowFull && (complete || customEnabled("fullFileChecksum")),
		GenerateWaveforms:          allowFull && probeStreams && (complete || customEnabled("generateWaveforms")),
		ValidateSeekBehavior:       probeStreams && (tier == analysisTierBasic || complete || customEnabled("validateSeekBehavior")),
		FullSeekValidation:         full && complete,
		DetectSegments:             allowFull && readEmbeddedIndexes && (complete || customEnabled("detectSegments")),
		ExtractEmbeddedCovers:      readEmbeddedIndexes && (complete || customEnabled("extractSelectedEmbeddedAssets")),
		AnalyzeSTRMTarget:          allowFull && probeStreams && (complete || customEnabled("analyzeSTRMTarget")),
	}
	return options
}

func (s *Server) analysisTierForItem(item MediaItem) string {
	if sourceID, _, err := parseRemoteStorageLocator(strings.TrimSpace(item.SourceURL)); err == nil {
		var mode string
		if s.queryBackgroundRow(context.Background(), `SELECT analysis_mode FROM storage_sources WHERE id=? AND library_id=?`, sourceID, item.LibraryID).Scan(&mode) == nil {
			return normalizeAnalysisTier(mode)
		}
		return analysisTierFileListOnly
	}
	if library, err := s.getLibrary(item.LibraryID); err == nil {
		return s.libraryRuntimeSettingsFor(library).AnalysisTier
	}
	return analysisTierFileListOnly
}

func (s *Server) mediaAnalysisQueueEnabled(item MediaItem) bool {
	if s.analysisTierForItem(item) == analysisTierFileListOnly {
		return false
	}
	options := s.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	return options.ProbeStreams || options.FullFileChecksum
}

func (s *Server) runMediaAnalyze(ctx context.Context, job Job) {
	if job.ResourceType != "media" || strings.TrimSpace(job.ResourceID) == "" {
		_ = s.setJobMessage(job.ID, "failed", 100, "Media analysis failed because no media item was selected.")
		return
	}
	_ = s.setJobMessage(job.ID, "running", 12, "Preparing media analysis.")
	item, err := s.getMediaBackgroundSourceSeedContext(ctx, job.ResourceID)
	if err != nil {
		_ = s.setJobMessage(job.ID, "failed", 100, "Media analysis failed because the media item was not found.")
		return
	}
	// Re-resolve policy after the worker claim and before any media content is
	// opened. This closes the race where an owner switches a source to File
	// List Only while a queued analysis job is being admitted.
	if !s.mediaAnalysisQueueEnabled(item) {
		_ = s.setJobMessage(job.ID, "complete", 100, "Media analysis skipped because this source is configured for File List Only.")
		return
	}
	if expected := strings.TrimSpace(job.Metadata["sourceRevision"]); expected != "" {
		current, revisionErr := s.currentMediaAnalysisSourceRevision(ctx, item)
		if revisionErr != nil || current != expected {
			_ = s.setJobMessage(job.ID, "complete", 100, "Media analysis skipped because the source changed after it was queued.")
			return
		}
	}
	options := s.mediaAnalysisOptions(item, mediaAnalysisModeFromJob(job))
	if options.Mode == mediaAnalysisModeFull && !s.analysisTierWantsFull(item) {
		_ = s.setJobMessage(job.ID, "complete", 100, "Complete analysis skipped because this source no longer uses the Complete tier.")
		return
	}
	if options.Mode == mediaAnalysisModeProbe && strings.EqualFold(strings.TrimSpace(job.Metadata["representativeFrame"]), "true") {
		options.GenerateThumbnails = s.analysisTierWantsRepresentativeThumbnail(item) && s.mediaNeedsRepresentativeFrameContext(ctx, item)
	}
	if !options.ProbeStreams && options.Mode == mediaAnalysisModeProbe && s.analysisTierWantsFull(item) {
		metadata := mediaAnalysisMetadata(mediaAnalysisModeFull)
		metadata["sourceRevision"] = strings.TrimSpace(job.Metadata["sourceRevision"])
		metadata["tierChained"] = "true"
		if _, err := s.createJobForWithMetadata("media_analyze", "Full media analysis queued for "+item.Title+".", "media", item.ID, metadata); err != nil {
			_ = s.setJobMessage(job.ID, "failed", 100, "Portico could not durably queue the authorized full-file analysis stage.")
			return
		}
		_ = s.setJobMessage(job.ID, "complete", 100, "Bounded stream probing is disabled; the authorized full-file analysis stage was queued directly.")
		return
	}
	if !options.ProbeStreams && !options.FullFileChecksum {
		_ = s.setJobMessage(job.ID, "complete", 100, "Media analysis skipped for "+item.Title+" because this library has stream analysis disabled.")
		return
	}
	ctx, unregisterBackground := s.mediaResourceGovernor().registerBackgroundContext(ctx)
	defer unregisterBackground()
	resources := mediaResourceRequest{class: foundationcontract.WorkClassBackgroundMedia, cpu: 1}
	if options.Mode == mediaAnalysisModeFull || options.GenerateThumbnails {
		resources.disk = 1
		if err := ensureMediaWriteCapacity(s.cfg.AppDataDir, mediaWriteMinimumFreeBytes); err != nil {
			if s.deferMaintenanceJob(job.ID, err) {
				return
			}
			_ = s.setJobMessage(job.ID, "failed", 100, "Media analysis paused because server storage is low.")
			return
		}
	}
	release, err := s.mediaResourceGovernor().acquireContext(ctx, resources)
	if err != nil {
		if (errors.Is(err, errRemoteStoragePreempted) || errors.Is(err, errRemoteStorageBusy)) && s.deferAnalysisForPlayback(job.ID) {
			return
		}
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		_ = s.setJobMessage(job.ID, "failed", 100, "Media analysis could not acquire processing capacity.")
		return
	}
	defer release()
	// Capacity waits can outlive a settings mutation. Re-resolve the profile at
	// the final launch boundary so no newly admitted process starts under the
	// superseded authorization. Already-running work is cancelled by the
	// settings/source mutation fence.
	if !s.mediaAnalysisQueueEnabled(item) || options.Mode == mediaAnalysisModeFull && !s.analysisTierWantsFull(item) {
		_ = s.setJobMessage(job.ID, "complete", 100, "Media analysis skipped because its scan profile changed before content was opened.")
		return
	}
	options = s.mediaAnalysisOptions(item, mediaAnalysisModeFromJob(job))
	currentRevision, revisionErr := s.currentMediaAnalysisSourceRevision(ctx, item)
	if revisionErr != nil || strings.TrimSpace(job.Metadata["sourceRevision"]) != "" && currentRevision != strings.TrimSpace(job.Metadata["sourceRevision"]) {
		_ = s.setJobMessage(job.ID, "complete", 100, "Media analysis skipped because the source changed before content was opened.")
		return
	}
	options.ExpectedSourceRevision = currentRevision
	if err := s.analyzeMediaForItem(ctx, item, options); err != nil {
		if (errors.Is(err, errRemoteStoragePreempted) || errors.Is(err, errRemoteStorageBusy)) && s.deferAnalysisForPlayback(job.ID) {
			return
		}
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Media analysis failed for " + item.Title + ": " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("warn", message, map[string]string{"job": job.ID, "media": item.ID})
		return
	}
	if options.Mode == mediaAnalysisModeProbe && s.analysisTierWantsFull(item) {
		metadata := mediaAnalysisMetadata(mediaAnalysisModeFull)
		metadata["sourceRevision"] = strings.TrimSpace(job.Metadata["sourceRevision"])
		metadata["tierChained"] = "true"
		if _, err := s.createJobForWithMetadata("media_analyze", "Full media analysis queued for "+item.Title+".", "media", item.ID, metadata); err != nil {
			s.log.Warn("full media analysis queue failed", "media", item.ID, "error", err)
			if s.deferMaintenanceJob(job.ID, err) {
				return
			}
			_ = s.setJobMessage(job.ID, "failed", 100, "Basic analysis completed, but Portico could not durably queue the Complete stage.")
			return
		}
	}
	message := "Media analysis completed for " + item.Title + "."
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, map[string]string{"job": job.ID, "media": item.ID})
}

func (s *Server) deferAnalysisForPlayback(jobID string) bool {
	now := time.Now().UTC()
	next := now.Add(5 * time.Second).Format(time.RFC3339Nano)
	result, err := s.execBackgroundWrite(context.Background(), `
		UPDATE jobs
		SET status='queued', phase='queued', progress=CASE WHEN progress >= 100 THEN 99 ELSE progress END,
			progress_current=CASE WHEN progress_current >= 100 THEN 99 ELSE progress_current END,
			message='Analysis paused for playback; it will resume automatically.',
			next_run_at=?, deferred_until=?, leased_by='', lease_expires_at='',
			attempt_count=CASE WHEN attempt_count > 0 THEN attempt_count - 1 ELSE 0 END,
			last_error='', failure_kind='', error_code='', retry_eligible=1, updated_at=?
		WHERE id=? AND status='running' AND leased_by=? AND cancellation_requested_at=''`,
		next, next, now.Format(time.RFC3339Nano), jobID, s.jobLeaseOwner(jobID))
	if err != nil {
		s.log.Warn("analysis playback preemption defer failed", "job", jobID, "error", err)
		return false
	}
	affected, _ := result.RowsAffected()
	return affected == 1
}

func (s *Server) analysisTierWantsRepresentativeThumbnail(item MediaItem) bool {
	tier := s.analysisTierForItem(item)
	library, err := s.getLibrary(item.LibraryID)
	if err != nil {
		return false
	}
	if tier == analysisTierBasic || tier == analysisTierComplete {
		return true
	}
	return tier == analysisTierCustom && settingBool(s.libraryAnalysisSettingsFor(library), "generateRepresentativeThumbnail", false)
}

func (s *Server) currentMediaAnalysisSourceRevision(ctx context.Context, item MediaItem) (string, error) {
	fileID := selectedPlaybackVersionID(item)
	if fileID == "" {
		item.MediaFiles = s.primaryMediaFileForPlaybackContext(ctx, item.ID, item.SourceURL)
		fileID = selectedPlaybackVersionID(item)
	}
	var path, quickSignature, modTime string
	var size int64
	if err := s.queryBackgroundRow(ctx, `SELECT path,size_bytes,mod_time,CASE WHEN identity_evidence LIKE 'scanner:v2:%' THEN substr(identity_evidence,12) ELSE '' END FROM media_files WHERE id=? AND media_id=? AND available=1`, fileID, item.ID).Scan(&path, &size, &modTime, &quickSignature); err != nil {
		return "", err
	}
	return scannerAnalysisSourceRevision(scannerMediaFile{ID: item.ID, FileID: fileID, SourcePath: path, QuickSignature: quickSignature, FileSize: size, FileModTime: modTime}), nil
}

func (s *Server) analysisTierWantsFull(item MediaItem) bool {
	if strings.TrimSpace(item.LibraryID) == "" {
		return false
	}
	tier := s.analysisTierForItem(item)
	if tier == analysisTierComplete {
		return true
	}
	if tier != analysisTierCustom {
		return false
	}
	options := s.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	return options.ExtractEmbeddedAttachments || options.ChapterThumbnails || options.GenerateTrickplay || options.AnalyzeAudio || options.SonicFingerprinting || options.FullFileChecksum || options.GenerateWaveforms || options.DetectSegments || options.AnalyzeSTRMTarget
}

func (s *Server) analyzeMediaForItem(ctx context.Context, item MediaItem, options mediaAnalysisOptions) error {
	return s.analyzeMediaWithFFprobe(ctx, item, options)
}

func (s *Server) analyzeMediaWithFFprobe(ctx context.Context, item MediaItem, options mediaAnalysisOptions) error {
	if strings.TrimSpace(options.ExpectedSourceRevision) == "" {
		revision, err := s.currentMediaAnalysisSourceRevision(ctx, item)
		if err != nil {
			return err
		}
		options.ExpectedSourceRevision = revision
	}
	if _, _, err := parseRemoteStorageLocator(strings.TrimSpace(item.SourceURL)); err == nil {
		if !options.ProbeStreams {
			return s.analyzeRemoteFullFileOperations(withRemoteStorageBackgroundRead(ctx), item, item.SourceURL, options)
		}
		return s.analyzeRemoteMediaFacts(withRemoteStorageBackgroundRead(ctx), item, item.SourceURL, options)
	}
	if source, ok := s.strmAnalysisSource(ctx, item); ok {
		return s.analyzeSTRMTarget(ctx, item, source, options)
	}
	path, err := s.localSourcePathForTranscode(item)
	if err != nil {
		return err
	}
	if !options.ProbeStreams {
		return s.runApprovedFullFileOperations(ctx, item, path, path, ffprobePayload{}, options)
	}
	if _, err := exec.LookPath(s.cfg.FFprobePath); err != nil && filepath.Base(s.cfg.FFprobePath) == s.cfg.FFprobePath {
		return errors.New("ffprobe is not available on PATH")
	}
	probeCtx, cancelProbe := context.WithTimeout(ctx, 30*time.Second)
	defer cancelProbe()
	result, err := s.runAnalysisSourceCommand(probeCtx, path, "probe media facts", s.cfg.FFprobePath, []string{
		"-v", "error",
		"-protocol_whitelist", "file,pipe",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_chapters",
		path,
	}, "", 8<<20, 1<<20)
	if err != nil {
		return err
	}
	var payload ffprobePayload
	if err := json.Unmarshal(result.Stdout, &payload); err != nil {
		return err
	}
	exactSeekSafe, keyframeEvidenceAt := false, ""
	if options.ValidateSeekBehavior && options.FullSeekValidation {
		exactSeekSafe, keyframeEvidenceAt = s.probeExactSeekEvidence(ctx, path, payload)
	} else if options.ValidateSeekBehavior {
		exactSeekSafe, keyframeEvidenceAt = s.probeBoundedExactSeekEvidence(ctx, path, payload)
	}
	return s.persistFFprobeAnalysis(ctx, item, path, payload, options, exactSeekSafe, keyframeEvidenceAt)
}

func (s *Server) probeExactSeekEvidence(ctx context.Context, path string, payload ffprobePayload) (bool, string) {
	return s.probeExactSeekEvidenceMode(ctx, path, payload, false)
}

func keyframesCoverExactSeekGrid(times []float64, duration float64, segmentSeconds int, tolerance float64) bool {
	if duration <= 0 || segmentSeconds <= 0 || len(times) == 0 || times[0] > tolerance {
		return false
	}
	index := 0
	for boundary := float64(segmentSeconds); boundary < duration-tolerance; boundary += float64(segmentSeconds) {
		for index < len(times) && times[index] < boundary-tolerance {
			index++
		}
		if index >= len(times) || times[index] > boundary+tolerance {
			return false
		}
	}
	return true
}

func payloadHasVideoStream(payload ffprobePayload) bool {
	for _, stream := range payload.Streams {
		if stream.CodecType == "video" && !ffprobeStreamIsAttachedPicture(stream) {
			return true
		}
	}
	return false
}

func (s *Server) persistFFprobeAnalysis(ctx context.Context, item MediaItem, path string, payload ffprobePayload, options mediaAnalysisOptions, exactSeekSafe bool, keyframeEvidenceAt string) error {
	return s.persistFFprobeAnalysisInputs(ctx, item, path, path, payload, options, exactSeekSafe, keyframeEvidenceAt)
}

func (s *Server) persistFFprobeAnalysisInputs(ctx context.Context, item MediaItem, recordPath, analysisPath string, payload ffprobePayload, options mediaAnalysisOptions, exactSeekSafe bool, keyframeEvidenceAt string) error {
	var analyzedFileID, sourceFingerprint, sourceModTime string
	var sourceSize int64
	if err := s.queryUserRow(ctx, `SELECT id, content_fingerprint, size_bytes, mod_time FROM media_files WHERE media_id = ? AND path = ?`, item.ID, recordPath).Scan(&analyzedFileID, &sourceFingerprint, &sourceSize, &sourceModTime); err != nil {
		return errors.New("authoritative media file record is unavailable for analysis")
	}
	analysisFile := canonicalAnalysisFileIdentity(analyzedFileID, sourceFingerprint, sourceSize, sourceModTime)
	analysisIdentity := item.ID
	if analyzedFileID != "" {
		analysisIdentity = analyzedFileID
	}
	duration, streams, chapters, attachments := mediaAnalysisFromFFprobe(analysisIdentity, payload)
	if len(attachments) > 0 && options.ExtractEmbeddedAttachments {
		var err error
		attachments, err = s.extractMediaAttachments(ctx, item, analysisPath, attachments)
		if err != nil {
			return err
		}
	}
	err := s.withBackgroundTxTagged(ctx, []string{"media", "metadata", "library-items"}, func(tx *sql.Tx) error {
		currentFence, err := loadAnalysisSourceFenceTx(tx, item.ID, recordPath)
		if err != nil || strings.TrimSpace(options.ExpectedSourceRevision) == "" ||
			currentFence.SourceRevision != options.ExpectedSourceRevision || currentFence.MediaFileID != analyzedFileID {
			return errMediaAnalysisSourceStale
		}
		probeAuthorized, err := analysisCapabilityAuthorizedTx(tx, item.LibraryID, recordPath, "probeStreams")
		if err != nil {
			return err
		}
		if !probeAuthorized {
			return errMediaAnalysisOperationDisabled
		}
		if options.AnalyzeSTRMTarget && isSTRMDescriptor(recordPath) {
			if err := assertSTRMTargetPublicationFenceTx(tx, item, recordPath, options.ExpectedSourceRevision); err != nil {
				return err
			}
		}
		fileID := analyzedFileID
		type persistedSeekEvidence struct {
			safe bool
			at   string
		}
		priorSeekEvidence := map[int]persistedSeekEvidence{}
		if strings.TrimSpace(keyframeEvidenceAt) == "" {
			rows, err := tx.Query(`SELECT stream_index,exact_seek_safe,keyframe_evidence_at
				FROM media_streams WHERE media_id=? AND file_id=? AND kind='video' AND source_kind='ffprobe'`, item.ID, fileID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var index, safe int
				var at string
				if err := rows.Scan(&index, &safe, &at); err != nil {
					rows.Close()
					return err
				}
				if strings.TrimSpace(at) != "" {
					priorSeekEvidence[index] = persistedSeekEvidence{safe: safe != 0, at: at}
				}
			}
			if err := rows.Close(); err != nil {
				return err
			}
		}
		for index := range streams {
			identity := strings.Join([]string{firstNonEmpty(fileID, item.ID), strconv.Itoa(streams[index].Index), streams[index].Kind}, "\x00")
			var streamID string
			_ = tx.QueryRow(`SELECT id FROM media_streams WHERE media_id = ? AND source_kind = 'ffprobe' AND file_id = ? AND stream_index = ? AND kind = ? LIMIT 1`, item.ID, fileID, streams[index].Index, streams[index].Kind).Scan(&streamID)
			if streamID == "" {
				var identityErr error
				streamID, identityErr = stableOpaquePublicResourceIDTx(tx, "media-stream", "ffprobe\x00"+identity)
				if identityErr != nil {
					return identityErr
				}
			}
			streams[index].ID = streamID
			streams[index].SourceKind = "ffprobe"
			streams[index].StorageKey = streamID
		}
		if duration > 0 {
			if _, err := tx.Exec(`UPDATE media_items SET duration_seconds = ? WHERE id = ?`, duration, item.ID); err != nil {
				return err
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if err := s.upsertAudioNormalizationFromFFprobe(tx, item.ID, payload, now); err != nil {
			return err
		}
		// Analysis replaces only technical streams belonging to the analyzed
		// source version. Sidecar subtitles share that file identity but are
		// scanner-owned, and technical streams for other versions remain valid.
		if _, err := tx.Exec(`DELETE FROM media_streams WHERE media_id = ? AND file_id = ? AND source_kind IN ('ffprobe', 'scanner')`, item.ID, fileID); err != nil {
			return err
		}
		for _, stream := range streams {
			if stream.Kind == "video" {
				if prior, ok := priorSeekEvidence[stream.Index]; ok {
					stream.ExactSeekSafe = prior.safe
					stream.KeyframeEvidenceAt = prior.at
				} else {
					stream.ExactSeekSafe = exactSeekSafe
					stream.KeyframeEvidenceAt = keyframeEvidenceAt
				}
			}
			stream.FileID = fileID
			if _, err := tx.Exec(`INSERT INTO media_streams (
				id, media_id, file_id, source_kind, source_identity, storage_key, kind, codec, language, channels, bitrate, width, height,
				stream_index, frame_rate, aspect_ratio, sample_rate, channel_layout, is_default, is_forced, hearing_impaired, display_title,
				profile, level, pixel_format, bit_depth, color_transfer, color_primaries, color_space,
				chroma_location, field_order, dynamic_range, dolby_vision_profile, exact_seek_safe, keyframe_evidence_at
			) VALUES (?, ?, ?, 'ffprobe', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				stream.ID, item.ID, fileID, strings.Join([]string{firstNonEmpty(fileID, item.ID), strconv.Itoa(stream.Index), stream.Kind}, "\x1f"), stream.StorageKey, stream.Kind, stream.Codec, stream.Language, stream.Channels, stream.Bitrate, stream.Width, stream.Height,
				stream.Index, stream.FrameRate, stream.AspectRatio, stream.SampleRate, stream.ChannelLayout, boolToInt(stream.Default), boolToInt(stream.Forced), boolToInt(stream.HearingImpaired), stream.DisplayTitle,
				stream.Profile, stream.Level, stream.PixelFormat, stream.BitDepth, stream.ColorTransfer, stream.ColorPrimaries, stream.ColorSpace,
				stream.ChromaLocation, stream.FieldOrder, stream.DynamicRange, stream.DolbyVisionProfile, boolToInt(stream.ExactSeekSafe), stream.KeyframeEvidenceAt); err != nil {
				return err
			}
		}
		if options.ReadEmbeddedIndexes {
			if _, err := tx.Exec(`DELETE FROM media_attachments WHERE media_id = ?`, item.ID); err != nil {
				return err
			}
			for _, attachment := range attachments {
				if _, err := tx.Exec(`INSERT INTO media_attachments (id, media_id, stream_id, filename, mime_type, codec, path, size_bytes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					attachment.ID, item.ID, attachment.StreamID, attachment.Filename, attachment.MimeType, attachment.Codec, attachment.URL, attachment.SizeBytes, now); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`DELETE FROM media_chapters WHERE media_id = ?`, item.ID); err != nil {
				return err
			}
			for index, chapter := range chapters {
				if _, err := tx.Exec(`INSERT INTO media_chapters (id, media_id, title, start_seconds, end_seconds, sort_order) VALUES (?, ?, ?, ?, ?, ?)`,
					chapter.ID, item.ID, chapter.Title, chapter.StartSeconds, chapter.EndSeconds, index); err != nil {
					return err
				}
			}
		}
		// Older pre-release builds briefly projected chapter labels as generated
		// markers. Chapter titles are not content evidence, so this legacy owner
		// is always removed; the signal detector publishes through its own owner.
		if _, err := tx.Exec(`DELETE FROM media_segments WHERE media_id = ? AND source = 'generated' AND provider = 'chapter-markers'`, item.ID); err != nil {
			return err
		}
		if err := persistPlaybackFacts(tx, item.ID, analysisFile, payload, now); err != nil {
			return err
		}
		return updateAnalyzedMediaFile(tx, item, recordPath, duration, streams, payload)
	})
	if err != nil {
		return err
	}
	if options.ReadEmbeddedTags {
		if err := s.updateMediaTagsFromFFprobe(ctx, item, payload); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if options.AnalyzeAudio && s.audioLoudnessAnalysisEnabled(item, payload) && !s.mediaHasAudioNormalization(ctx, item.ID) {
		if !s.waitForAnalysisForegroundWindow(ctx) {
			return ctx.Err()
		}
		if err := s.analyzeAudioNormalizationWithFFmpeg(ctx, item, analysisPath); err != nil {
			s.log.Warn("audio loudness analysis failed", "media", item.ID, "error", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if hasVideoStream(streams) {
		if options.GenerateThumbnails {
			if !s.waitForAnalysisForegroundWindow(ctx) {
				return ctx.Err()
			}
			if err := s.generateMediaThumbnailFromPath(ctx, item, analysisPath); err != nil {
				s.log.Warn("thumbnail generation failed", "media", item.ID, "error", err)
			}
		}
		if options.ChapterThumbnails {
			if !s.waitForAnalysisForegroundWindow(ctx) {
				return ctx.Err()
			}
			if err := s.generateChapterThumbnailsFromPath(ctx, item, analysisPath, chapters); err != nil {
				s.log.Warn("chapter thumbnail generation failed", "media", item.ID, "error", err)
			}
		}
		if options.GenerateTrickplay {
			if !s.waitForAnalysisForegroundWindow(ctx) {
				return ctx.Err()
			}
			if err := s.generateMediaTrickplay(ctx, item, analysisPath, duration, streams); err != nil {
				s.log.Warn("trickplay generation failed", "media", item.ID, "error", err)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if options.ExtractEmbeddedCovers {
		if !s.waitForAnalysisForegroundWindow(ctx) {
			return ctx.Err()
		}
		if err := s.extractEmbeddedCoverImage(ctx, item, analysisPath, payload); err != nil {
			s.log.Warn("embedded cover extraction failed", "media", item.ID, "error", err)
		}
	}
	if options.SonicFingerprinting && item.Type == "track" && s.sonicFingerprintingEnabled(item) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if _, err := s.refreshTrackMetadataFromAcoustID(ctx, item); err != nil {
			s.log.Warn("sonic fingerprint match failed", "media", item.ID, "error", err)
		}
	}
	if options.DetectSegments {
		if err := s.detectMediaSegments(ctx, item, recordPath, analysisPath, payload, chapters, analysisFile); err != nil {
			return err
		}
	}
	return s.runApprovedFullFileOperations(ctx, item, recordPath, analysisPath, payload, options)
}

func updateAnalyzedMediaFile(tx *sql.Tx, item MediaItem, analyzedPath string, duration int, streams []Stream, payload ffprobePayload) error {
	sourceURL := strings.TrimSpace(analyzedPath)
	if sourceURL == "" {
		sourceURL = strings.TrimSpace(item.SourceURL)
	}
	if sourceURL == "" {
		return nil
	}
	video := firstStreamOfKind(streams, "video")
	audio := firstStreamOfKind(streams, "audio")
	container := containerFromFFprobe(payload, sourceURL)
	bitrate, _ := strconv.Atoi(strings.TrimSpace(payload.Format.BitRate))
	_, err := tx.Exec(`
		UPDATE media_files
		SET container = COALESCE(NULLIF(?, ''), container),
			video_codec = COALESCE(NULLIF(?, ''), video_codec),
			audio_codec = COALESCE(NULLIF(?, ''), audio_codec),
			resolution = COALESCE(NULLIF(?, ''), resolution),
			duration_seconds = ?, bitrate = ?, width = ?, height = ?, frame_rate = ?, aspect_ratio = ?,
			video_profile = ?, video_level = ?, bit_depth = ?, pixel_format = ?, color_transfer = ?,
			color_primaries = ?, color_space = ?, chroma_location = ?, audio_channels = ?,
			audio_channel_layout = ?, audio_sample_rate = ?, audio_bitrate = ?
		WHERE media_id = ? AND path = ?`,
		container, video.Codec, audio.Codec, resolutionLabel(video.Width, video.Height),
		duration, bitrate, video.Width, video.Height, video.FrameRate, video.AspectRatio,
		video.Profile, video.Level, video.BitDepth, video.PixelFormat, video.ColorTransfer,
		video.ColorPrimaries, video.ColorSpace, video.ChromaLocation, audio.Channels,
		audio.ChannelLayout, audio.SampleRate, audio.Bitrate, item.ID, sourceURL)
	return err
}

func containerFromFFprobe(payload ffprobePayload, fallbackSource string) string {
	formatName := strings.ToLower(strings.TrimSpace(payload.Format.FormatName))
	switch {
	case strings.Contains(formatName, "matroska"):
		return "matroska"
	case strings.Contains(formatName, "mov") || strings.Contains(formatName, "mp4") || strings.Contains(formatName, "m4a"):
		return "mp4"
	case strings.Contains(formatName, "mpegts"):
		return "mpegts"
	case strings.Contains(formatName, "webm"):
		return "webm"
	case strings.Contains(formatName, "mp3"):
		return "mp3"
	case strings.Contains(formatName, "flac"):
		return "flac"
	case strings.Contains(formatName, "ogg") || strings.Contains(formatName, "opus"):
		return "ogg"
	}
	majorBrand := strings.ToLower(strings.TrimSpace(payload.Format.Tags["major_brand"]))
	switch majorBrand {
	case "isom", "iso2", "mp41", "mp42", "m4v", "qt":
		return "mp4"
	case "webm":
		return "webm"
	}
	return playbackContainerFor(fallbackSource)
}

func resolutionLabel(width, height int) string {
	switch {
	case height >= 2160 || width >= 3800:
		return "4K"
	case height >= 1440:
		return "1440p"
	case height >= 1080:
		return "1080p"
	case height >= 720:
		return "720p"
	case height > 0:
		return strconv.Itoa(height) + "p"
	default:
		return ""
	}
}

func mediaSegmentsFromChapters(mediaID string, chapters []Chapter, duration int) []MediaSegment {
	// Chapters are navigation metadata, not evidence that a range is safe to
	// skip. Intro/credits segments must come from an explicit editor or a future
	// analyzed provider that can set automatic_safe deliberately.
	return []MediaSegment{}
}

func mediaSegmentTypeFromChapterTitle(title string) (string, float64) {
	return "", 0
}

func (s *Server) audioLoudnessAnalysisEnabled(item MediaItem, payload ffprobePayload) bool {
	if item.Type != "track" && item.Type != "audiobook" {
		return false
	}
	return payloadHasAudioStream(payload)
}

func payloadHasAudioStream(payload ffprobePayload) bool {
	for _, stream := range payload.Streams {
		if stream.CodecType == "audio" {
			return true
		}
	}
	return false
}

func (s *Server) mediaHasAudioNormalization(ctx context.Context, mediaID string) bool {
	var exists int
	if err := s.queryBackgroundRow(ctx, `SELECT 1 FROM audio_normalization WHERE media_id = ?`, mediaID).Scan(&exists); err != nil {
		return false
	}
	return exists == 1
}

func (s *Server) analyzeAudioNormalizationWithFFmpeg(ctx context.Context, item MediaItem, path string) error {
	if _, err := exec.LookPath(s.cfg.FFmpegPath); err != nil && filepath.Base(s.cfg.FFmpegPath) == s.cfg.FFmpegPath {
		return errors.New("ffmpeg is not available on PATH")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner",
		"-nostdin",
		"-nostats",
		"-threads", "1",
		"-filter_threads", "1",
		"-i", path,
		"-af", "loudnorm=I=-16:TP=-1.5:LRA=11:print_format=json",
	}
	args = append(args, analysisProgressArgs()...)
	args = append(args, "-f", "null", "-")
	result, err := s.runAnalysisSourceCommand(ctx, path, "analyze audio loudness", s.cfg.FFmpegPath, args, "", 4<<20, 4<<20)
	output := append(append([]byte(nil), result.Stdout...), result.Stderr...)
	normalization, parseErr := audioNormalizationFromLoudnormOutput(output)
	if parseErr != nil {
		if err != nil {
			return fmt.Errorf("%w: %v", err, parseErr)
		}
		return parseErr
	}
	normalization.Source = "ffmpeg-loudnorm"
	normalization.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err = s.execBackgroundWrite(ctx, `
		INSERT INTO audio_normalization (
			media_id, track_gain_db, track_peak, album_gain_db, album_peak, integrated_lufs, source, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_id) DO UPDATE SET
			track_gain_db = excluded.track_gain_db,
			track_peak = excluded.track_peak,
			album_gain_db = excluded.album_gain_db,
			album_peak = excluded.album_peak,
			integrated_lufs = excluded.integrated_lufs,
			source = excluded.source,
			updated_at = excluded.updated_at`,
		item.ID,
		normalization.TrackGainDB,
		normalization.TrackPeak,
		normalization.AlbumGainDB,
		normalization.AlbumPeak,
		normalization.IntegratedLUFS,
		normalization.Source,
		normalization.UpdatedAt,
	)
	return err
}

func (s *Server) chapterThumbnailGenerationEnabled(item MediaItem) bool {
	library, err := s.getLibrary(item.LibraryID)
	if err != nil {
		return true
	}
	return settingBool(library.Settings, "generateChapterThumbnails", true)
}

func (s *Server) thumbnailGenerationEnabled(item MediaItem) bool {
	library, err := s.getLibrary(item.LibraryID)
	if err != nil {
		return true
	}
	return settingBool(library.Settings, "generateRepresentativeThumbnail", true)
}

func (s *Server) trickplayGenerationEnabled(item MediaItem) bool {
	library, err := s.getLibrary(item.LibraryID)
	if err != nil {
		return true
	}
	return settingBool(library.Settings, "generateTrickplay", true)
}

func (s *Server) shouldDeferAnalysisForForeground() bool {
	settings := s.transcodeSettings()
	active := s.activeTranscodeSessionCount()
	// Direct play alone must not permanently starve analysis on an active
	// server. Defer only when foreground encoding has consumed the available
	// transcode capacity; the shared media resource governor supplies the
	// additional CPU/disk reservation for background work.
	return settings.MaxConcurrentSessions > 0 && active >= settings.MaxConcurrentSessions
}

func (s *Server) waitForAnalysisForegroundWindow(ctx context.Context) bool {
	for s.shouldDeferAnalysisForForeground() {
		if !sleepContext(ctx, 2*time.Second) {
			return false
		}
	}
	return true
}

type trickplayGenerationOptions struct {
	IntervalSeconds int
	TileWidth       int
	MaxTiles        int
}

func (s *Server) trickplayGenerationOptions(item MediaItem, duration int) trickplayGenerationOptions {
	options := trickplayGenerationOptions{TileWidth: 160, MaxTiles: 240}
	settings, err := s.loadSettings()
	if err == nil {
		group, _ := settings["scheduledTasks"].(map[string]any)
		options.IntervalSeconds = settingInt(group, "trickplayIntervalSeconds", 0)
		options.TileWidth = settingInt(group, "trickplayTileWidth", options.TileWidth)
		options.MaxTiles = settingInt(group, "trickplayMaxTiles", options.MaxTiles)
	}
	if library, err := s.getLibrary(item.LibraryID); err == nil {
		options.IntervalSeconds = settingInt(library.Settings, "trickplayIntervalSeconds", options.IntervalSeconds)
		options.TileWidth = settingInt(library.Settings, "trickplayTileWidth", options.TileWidth)
		options.MaxTiles = settingInt(library.Settings, "trickplayMaxTiles", options.MaxTiles)
	}
	options.TileWidth = max(96, min(640, options.TileWidth))
	options.MaxTiles = max(24, min(2000, options.MaxTiles))
	if options.IntervalSeconds <= 0 {
		options.IntervalSeconds = trickplayIntervalSeconds(duration, options.MaxTiles)
	} else {
		options.IntervalSeconds = max(2, min(600, options.IntervalSeconds))
	}
	return options
}

func (s *Server) updateMediaTagsFromFFprobe(ctx context.Context, item MediaItem, payload ffprobePayload) error {
	if item.Type != "track" && item.Type != "audiobook" {
		return nil
	}
	if !s.embeddedTagsEnabledForItem(item) {
		return nil
	}
	tags := normalizedFFprobeTags(payload.Format.Tags)
	for _, stream := range payload.Streams {
		for key, value := range normalizedFFprobeTags(stream.Tags) {
			if strings.TrimSpace(tags[key]) == "" {
				tags[key] = value
			}
		}
	}
	metadata := musicMetadataFromTags(tags)
	title := metadata.Title
	if item.Type == "track" && !s.preferEmbeddedTitles(item) {
		title = ""
	}
	artist := firstNonEmpty(metadata.Artist, metadata.AlbumArtist)
	album := metadata.AlbumTitle
	trackNumber := metadata.TrackNumber
	year := metadata.Year
	genres := metadata.Genres

	update := UpdateMediaRequest{}
	if title != "" {
		sortTitle := sortableTitle(title)
		update.Title = &title
		update.SortTitle = &sortTitle
	}
	if artist != "" {
		update.Studio = &artist
	}
	if trackNumber > 0 {
		update.IndexNumber = &trackNumber
	}
	if year > 0 && item.Year == 0 {
		update.Year = &year
	}
	if len(genres) > 0 {
		update.Genres = &genres
	}
	if typedMetadata := typedMetadataFromMusicMetadata(item, metadata); len(typedMetadata) > 0 {
		update.TypedMetadata = &typedMetadata
	}
	if len(metadataChangedFields(update)) > 0 {
		current, err := s.getMedia("", item.ID)
		if err != nil {
			return err
		}
		if _, err := s.applyMetadata(ctx, metadataApplyRequest{MediaID: item.ID, ExpectedRevision: current.MetadataRevision, Origin: metadataSourceEmbedded, Source: "ffprobe", Update: update}); err != nil {
			return err
		}
	}
	if item.ParentID != "" && (album != "" || artist != "") {
		parentUpdate := UpdateMediaRequest{}
		if album != "" {
			sortAlbum := sortableTitle(album)
			parentUpdate.Title = &album
			parentUpdate.SortTitle = &sortAlbum
		}
		if artist != "" {
			parentUpdate.Studio = &artist
		}
		parent, err := s.getMedia("", item.ParentID)
		if err != nil {
			return err
		}
		if parent.Type == "album" {
			if _, err := s.applyMetadata(ctx, metadataApplyRequest{MediaID: parent.ID, ExpectedRevision: parent.MetadataRevision, Origin: metadataSourceEmbedded, Source: "ffprobe", Update: parentUpdate}); err != nil {
				return err
			}
		}
	}
	if artist != "" && item.GrandparentID != "" {
		grandparent, err := s.getMedia("", item.GrandparentID)
		if err != nil {
			return err
		}
		if grandparent.Type == "artist" {
			sortArtist := sortableTitle(artist)
			grandparentUpdate := UpdateMediaRequest{Title: &artist, SortTitle: &sortArtist}
			if _, err := s.applyMetadata(ctx, metadataApplyRequest{MediaID: grandparent.ID, ExpectedRevision: grandparent.MetadataRevision, Origin: metadataSourceEmbedded, Source: "ffprobe", Update: grandparentUpdate}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) embeddedTagsEnabledForItem(item MediaItem) bool {
	if !s.metadataAgentSettings().EmbeddedTags {
		return false
	}
	if library, err := s.getLibrary(item.LibraryID); err == nil && localMetadataModeForLibrary(library) == "off" {
		return false
	}
	return true
}

func (s *Server) preferEmbeddedTitles(item MediaItem) bool {
	if strings.TrimSpace(item.LibraryID) == "" {
		return true
	}
	library, err := s.getLibrary(item.LibraryID)
	if err != nil {
		return true
	}
	return settingBool(library.Settings, "preferEmbeddedTitles", true)
}

func typedMetadataFromMusicMetadata(item MediaItem, metadata scannerLocalMetadata) map[string]string {
	out := map[string]string{}
	for key, value := range item.TypedMetadata {
		if strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	for key, value := range metadata.TypedMetadata {
		if strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	add := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			out[key] = value
		}
	}
	addInt := func(key string, value int) {
		if value > 0 {
			out[key] = strconv.Itoa(value)
		}
	}
	if item.Type == "audiobook" {
		add("author", metadata.Artist)
		if !strings.EqualFold(metadata.Studio, metadata.Artist) {
			add("narrator", metadata.Studio)
		}
		add("series", metadata.Series)
		add("seriesIndex", metadata.SeriesIndex)
		add("publisher", metadata.Publisher)
	} else {
		add("artist", firstNonEmpty(metadata.AlbumArtist, metadata.Artist))
		add("albumArtist", metadata.AlbumArtist)
		add("trackArtist", metadata.Artist)
		add("albumTitle", metadata.AlbumTitle)
		add("label", metadata.Label)
		add("releaseCountry", metadata.ReleaseCountry)
		addInt("trackNumber", metadata.TrackNumber)
		addInt("trackCount", metadata.TrackCount)
		addInt("discNumber", metadata.DiscNumber)
		addInt("discCount", metadata.DiscCount)
		addInt("bpm", metadata.BPM)
		add("explicit", metadata.Explicit)
	}
	return normalizeTypedMetadataMap(out)
}

func musicMetadataFromFFprobe(payload ffprobePayload) scannerLocalMetadata {
	tags := normalizedFFprobeTags(payload.Format.Tags)
	for _, stream := range payload.Streams {
		for key, value := range normalizedFFprobeTags(stream.Tags) {
			if strings.TrimSpace(tags[key]) == "" {
				tags[key] = value
			}
		}
	}
	return musicMetadataFromTags(tags)
}

func (s *Server) upsertAudioNormalizationFromFFprobe(tx *sql.Tx, mediaID string, payload ffprobePayload, now string) error {
	normalization, ok := audioNormalizationFromFFprobe(payload)
	if !ok {
		_, err := tx.Exec(`DELETE FROM audio_normalization WHERE media_id = ?`, mediaID)
		return err
	}
	normalization.UpdatedAt = now
	_, err := tx.Exec(`
		INSERT INTO audio_normalization (
			media_id, track_gain_db, track_peak, album_gain_db, album_peak, integrated_lufs, source, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_id) DO UPDATE SET
			track_gain_db = excluded.track_gain_db,
			track_peak = excluded.track_peak,
			album_gain_db = excluded.album_gain_db,
			album_peak = excluded.album_peak,
			integrated_lufs = excluded.integrated_lufs,
			source = excluded.source,
			updated_at = excluded.updated_at`,
		mediaID,
		normalization.TrackGainDB,
		normalization.TrackPeak,
		normalization.AlbumGainDB,
		normalization.AlbumPeak,
		normalization.IntegratedLUFS,
		normalization.Source,
		normalization.UpdatedAt,
	)
	return err
}

func audioNormalizationFromFFprobe(payload ffprobePayload) (AudioNormalization, bool) {
	tags := normalizedFFprobeTags(payload.Format.Tags)
	for _, stream := range payload.Streams {
		for key, value := range normalizedFFprobeTags(stream.Tags) {
			if strings.TrimSpace(tags[key]) == "" {
				tags[key] = value
			}
		}
	}
	var out AudioNormalization
	var hasReplayGain bool
	var hasLoudness bool
	if value, ok := firstFloatTag(tags, "replaygain_track_gain", "replaygain track gain", "track_gain", "track gain"); ok {
		out.TrackGainDB = value
		hasReplayGain = true
	}
	if value, ok := firstFloatTag(tags, "replaygain_track_peak", "replaygain track peak", "track_peak", "track peak"); ok {
		out.TrackPeak = value
		hasReplayGain = true
	}
	if value, ok := firstFloatTag(tags, "replaygain_album_gain", "replaygain album gain", "album_gain", "album gain"); ok {
		out.AlbumGainDB = value
		hasReplayGain = true
	}
	if value, ok := firstFloatTag(tags, "replaygain_album_peak", "replaygain album peak", "album_peak", "album peak"); ok {
		out.AlbumPeak = value
		hasReplayGain = true
	}
	if value, ok := firstFloatTag(tags, "integrated_lufs", "integrated lufs", "integrated_loudness", "integrated loudness", "lufs", "loudness"); ok {
		out.IntegratedLUFS = value
		hasLoudness = true
	}
	switch {
	case hasReplayGain:
		out.Source = "replaygain"
	case hasLoudness:
		out.Source = "loudness-tags"
	default:
		return AudioNormalization{}, false
	}
	return out, true
}

func audioNormalizationFromLoudnormOutput(output []byte) (AudioNormalization, error) {
	text := string(output)
	start := strings.LastIndex(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return AudioNormalization{}, errors.New("ffmpeg loudnorm output did not contain JSON")
	}
	var payload struct {
		InputI  string `json:"input_i"`
		InputTP string `json:"input_tp"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return AudioNormalization{}, err
	}
	lufs, ok := floatFromTaggedValue(payload.InputI)
	if !ok {
		return AudioNormalization{}, errors.New("ffmpeg loudnorm output did not contain integrated loudness")
	}
	normalization := AudioNormalization{IntegratedLUFS: lufs}
	if peak, ok := floatFromTaggedValue(payload.InputTP); ok {
		normalization.TrackPeak = peak
	}
	return normalization, nil
}

func firstFloatTag(tags map[string]string, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := floatFromTaggedValue(tags[key])
		if ok {
			return value, true
		}
	}
	return 0, false
}

func floatFromTaggedValue(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	for _, field := range strings.Fields(value) {
		if parsed, ok := parseLeadingFloat(field); ok {
			return parsed, true
		}
	}
	return parseLeadingFloat(value)
}

func parseLeadingFloat(value string) (float64, bool) {
	var builder strings.Builder
	seenDigit := false
	seenDecimal := false
	started := false
	for _, r := range strings.TrimSpace(value) {
		if !started && (r == '+' || r == '-' || r == '.' || r >= '0' && r <= '9') {
			started = true
		}
		if !started {
			continue
		}
		switch {
		case r >= '0' && r <= '9':
			seenDigit = true
			builder.WriteRune(r)
		case (r == '+' || r == '-') && builder.Len() == 0:
			builder.WriteRune(r)
		case r == '.' && !seenDecimal:
			seenDecimal = true
			builder.WriteRune(r)
		default:
			if seenDigit {
				parsed, err := strconv.ParseFloat(builder.String(), 64)
				return parsed, err == nil
			}
			return 0, false
		}
	}
	if !seenDigit {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(builder.String(), 64)
	return parsed, err == nil
}

func musicMetadataFromTags(tags map[string]string) scannerLocalMetadata {
	title := firstNonEmpty(tags["title"], tags["tracktitle"])
	author := firstNonEmpty(tags["author"], tags["authors"], tags["bookauthor"], tags["book_author"])
	narrator := firstNonEmpty(tags["narrator"], tags["narrators"], tags["narratedby"], tags["narrated_by"], tags["readby"], tags["read_by"])
	artist := firstNonEmpty(tags["artist"], tags["artists"], tags["performer"], tags["composer"], author)
	albumArtist := firstNonEmpty(tags["album_artist"], tags["albumartist"], tags["album artist"], artist)
	album := firstNonEmpty(tags["album"], tags["show"])
	trackTag := firstNonEmpty(tags["track"], tags["tracknumber"], tags["track_number"], tags["tracktotal"])
	discTag := firstNonEmpty(tags["disc"], tags["discnumber"], tags["disc_number"], tags["disctotal"])
	trackNumber := trackNumberFromTag(trackTag)
	trackCount := tagTotalFromTag(firstNonEmpty(tags["tracktotal"], tags["totaltracks"], tags["track_total"], trackTag))
	discNumber := trackNumberFromTag(discTag)
	discCount := tagTotalFromTag(firstNonEmpty(tags["disctotal"], tags["totaldiscs"], tags["disc_total"], discTag))
	bpm := trackNumberFromTag(firstNonEmpty(tags["bpm"], tags["tempo"]))
	year := yearFromTag(firstNonEmpty(tags["date"], tags["year"], tags["originaldate"], tags["releasedate"]))
	genres := genresFromTag(strings.Join(uniqueNonEmptyStrings([]string{tags["genre"], tags["genres"]}), ";"))
	label := firstNonEmpty(tags["label"], tags["publisher"], tags["organization"], tags["recordlabel"], tags["record_label"])
	publisher := firstNonEmpty(tags["publisher"], tags["bookpublisher"], tags["book_publisher"], label)
	releaseCountry := firstNonEmpty(tags["releasecountry"], tags["release_country"], tags["musicbrainz_albumreleasecountry"], tags["musicbrainz album release country"])
	moods := genresFromTag(firstNonEmpty(tags["mood"], tags["moods"]))
	series := firstNonEmpty(tags["series"], tags["calibre:series"], tags["bookseries"], tags["book_series"])
	seriesIndex := firstNonEmpty(tags["seriesindex"], tags["series_index"], tags["calibre:series_index"], tags["bookseriesindex"], tags["book_series_index"])
	typed := embeddedTypedMetadataFromTags(tags)
	return scannerLocalMetadata{
		Title:          title,
		SortTitle:      sortableTitle(title),
		Studio:         firstNonEmpty(narrator, artist, albumArtist),
		Artist:         firstNonEmpty(author, artist),
		AlbumArtist:    albumArtist,
		AlbumTitle:     album,
		Series:         series,
		SeriesIndex:    seriesIndex,
		Label:          label,
		Publisher:      publisher,
		ReleaseCountry: releaseCountry,
		TrackNumber:    trackNumber,
		TrackCount:     trackCount,
		DiscNumber:     discNumber,
		DiscCount:      discCount,
		BPM:            bpm,
		Explicit:       explicitValueFromTags(tags),
		Year:           year,
		Genres:         genres,
		Tags:           moods,
		ProviderIDs:    musicBrainzProviderIDsFromTags(tags),
		TypedMetadata:  typed,
		People:         musicPeopleFromTags(tags),
		Source:         "embedded",
	}
}

func embeddedTypedMetadataFromTags(tags map[string]string) map[string]string {
	out := map[string]string{}
	add := func(key string, candidates ...string) {
		if value := strings.TrimSpace(firstNonEmpty(candidates...)); value != "" {
			out[key] = value
		}
	}
	add("isrc", tags["isrc"])
	add("barcode", tags["barcode"], tags["upc"], tags["ean"])
	add("catalogNumber", tags["catalognumber"], tags["catalog_number"], tags["catalog number"])
	add("workID", tags["musicbrainz_workid"], tags["musicbrainz work id"], tags["musicbrainz_work_id"])
	add("trackID", tags["musicbrainz_trackid"], tags["musicbrainz track id"])
	add("recordingID", tags["musicbrainz_recordingid"], tags["musicbrainz recording id"], tags["musicbrainz_trackid"])
	add("releaseID", tags["musicbrainz_albumid"], tags["musicbrainz release id"], tags["musicbrainz_releaseid"])
	add("releaseGroupID", tags["musicbrainz_releasegroupid"], tags["musicbrainz release group id"])
	add("artistID", tags["musicbrainz_artistid"], tags["musicbrainz artist id"])
	add("albumArtistID", tags["musicbrainz_albumartistid"], tags["musicbrainz album artist id"])
	add("compilation", tags["compilation"], tags["itunescompilation"])
	add("grouping", tags["grouping"], tags["contentgroup"], tags["content_group"])
	add("releaseDate", exactDateFromNFO(firstNonEmpty(tags["date"], tags["releasedate"], tags["release_date"], tags["originaldate"])))
	add("description", tags["description"])
	add("comment", tags["comment"], tags["comments"])
	add("titleSort", tags["titlesort"], tags["title_sort"], tags["sorttitle"])
	add("artistSort", tags["artistsort"], tags["artist_sort"])
	add("albumSort", tags["albumsort"], tags["album_sort"])
	add("albumArtistSort", tags["albumartistsort"], tags["album_artist_sort"])
	if len(out) == 0 {
		return nil
	}
	return out
}

func musicBrainzProviderIDsFromTags(tags map[string]string) map[string]string {
	ids := map[string]string{}
	add := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			ids[key] = value
		}
	}
	add("musicbrainz:recording", firstNonEmpty(tags["musicbrainz_trackid"], tags["musicbrainz recording id"], tags["musicbrainz_recordingid"], tags["musicbrainz/trackid"]))
	add("musicbrainz:release", firstNonEmpty(tags["musicbrainz_albumid"], tags["musicbrainz album id"], tags["musicbrainz_releaseid"], tags["musicbrainz/releaseid"]))
	add("musicbrainz:release-group", firstNonEmpty(tags["musicbrainz_releasegroupid"], tags["musicbrainz release group id"], tags["musicbrainz_releasegroup_id"], tags["musicbrainz/releasegroupid"]))
	add("musicbrainz:artist", firstNonEmpty(tags["musicbrainz_artistid"], tags["musicbrainz artist id"], tags["musicbrainz_artist_id"], tags["musicbrainz/artistid"]))
	add("musicbrainz:album-artist", firstNonEmpty(tags["musicbrainz_albumartistid"], tags["musicbrainz album artist id"], tags["musicbrainz_albumartist_id"], tags["musicbrainz/albumartistid"]))
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func musicPeopleFromTags(tags map[string]string) []MediaPerson {
	roleTags := []struct {
		role string
		keys []string
	}{
		{"Composer", []string{"composer"}},
		{"Conductor", []string{"conductor"}},
		{"Lyricist", []string{"lyricist"}},
		{"Performer", []string{"performer", "performers"}},
		{"Writer", []string{"writer"}},
		{"Arranger", []string{"arranger"}},
		{"Engineer", []string{"engineer"}},
		{"Mixer", []string{"mixer"}},
		{"Remixer", []string{"remixer"}},
		{"Producer", []string{"producer"}},
	}
	var people []MediaPerson
	for _, entry := range roleTags {
		for _, key := range entry.keys {
			for _, name := range peopleNamesFromTag(tags[key]) {
				people = append(people, MediaPerson{Name: name, Role: entry.role})
			}
		}
	}
	for key, value := range tags {
		role, ok := musicPerformerRoleFromTagKey(key)
		if !ok {
			continue
		}
		for _, name := range peopleNamesFromTag(value) {
			people = append(people, MediaPerson{Name: name, Role: role})
		}
	}
	return people
}

func musicPerformerRoleFromTagKey(key string) (string, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, prefix := range []string{"performer:", "performer/", "performer_", "performer-"} {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		instrument := strings.TrimSpace(strings.TrimPrefix(key, prefix))
		instrument = strings.NewReplacer("_", " ", "-", " ", "/", " ").Replace(instrument)
		instrument = cleanMediaTitle(instrument)
		if instrument == "" || instrument == "Untitled" {
			return "", false
		}
		return "Performer: " + instrument, true
	}
	return "", false
}

func normalizedFFprobeTags(tags map[string]string) map[string]string {
	normalized := map[string]string{}
	for key, value := range tags {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			normalized[key] = value
		}
	}
	return normalized
}

func trackNumberFromTag(value string) int {
	value = strings.TrimSpace(value)
	if slash := strings.Index(value, "/"); slash >= 0 {
		value = value[:slash]
	}
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return 0
	}
	parsed, _ := strconv.Atoi(value)
	return max(0, parsed)
}

func tagTotalFromTag(value string) int {
	value = strings.TrimSpace(value)
	if slash := strings.Index(value, "/"); slash >= 0 {
		return trackNumberFromTag(value[slash+1:])
	}
	return 0
}

func explicitValueFromTags(tags map[string]string) string {
	for _, key := range []string{"explicit", "itunesadvisory", "advisory", "parental_advisory"} {
		value := strings.ToLower(strings.TrimSpace(tags[key]))
		switch value {
		case "1", "true", "yes", "explicit", "e":
			return "true"
		case "0", "false", "no", "clean", "none":
			return "false"
		}
	}
	return ""
}

func yearFromTag(value string) int {
	value = strings.TrimSpace(value)
	if len(value) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(value[:4])
	return year
}

func genresFromTag(value string) []string {
	return normalizeStringList(strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == ',' || r == '/'
	}))
}

func mediaAnalysisFromFFprobe(mediaID string, payload ffprobePayload) (int, []Stream, []Chapter, []MediaAttachment) {
	duration := secondsFromFFprobe(payload.Format.Duration)
	streams := []Stream{}
	attachments := []MediaAttachment{}
	for _, raw := range payload.Streams {
		if ffprobeStreamIsAttachedPicture(raw) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(raw.CodecType), "attachment") {
			attachments = append(attachments, mediaAttachmentFromFFprobe(mediaID, raw))
			continue
		}
		kind := ffprobeKind(raw.CodecType)
		if kind == "" {
			continue
		}
		language := firstNonEmpty(raw.Tags["language"], raw.Tags["LANGUAGE"])
		codec := normalizeCodec(raw.CodecName)
		bitrate := intFromString(raw.BitRate)
		dynamicRange := ffprobeDynamicRange(raw)
		displayTitle := ffprobeDisplayTitle(kind, codec, language, raw)
		streams = append(streams, Stream{
			ID:                 randomOpaquePublicID(),
			Index:              raw.Index,
			Kind:               kind,
			Codec:              codec,
			Language:           language,
			Channels:           raw.Channels,
			Bitrate:            bitrate,
			Width:              raw.Width,
			Height:             raw.Height,
			FrameRate:          ffprobeFrameRate(raw),
			AspectRatio:        strings.TrimSpace(raw.AspectRatio),
			SampleRate:         intFromString(raw.SampleRate),
			ChannelLayout:      strings.TrimSpace(raw.ChannelLayout),
			Default:            raw.Disposition["default"] == 1,
			Forced:             raw.Disposition["forced"] == 1,
			HearingImpaired:    raw.Disposition["hearing_impaired"] == 1,
			DisplayTitle:       displayTitle,
			Profile:            strings.ToLower(strings.TrimSpace(raw.Profile)),
			Level:              raw.Level,
			PixelFormat:        strings.ToLower(strings.TrimSpace(raw.PixelFormat)),
			BitDepth:           ffprobeBitDepth(raw),
			ColorTransfer:      strings.ToLower(strings.TrimSpace(raw.ColorTransfer)),
			ColorPrimaries:     strings.ToLower(strings.TrimSpace(raw.ColorPrimaries)),
			ColorSpace:         strings.ToLower(strings.TrimSpace(raw.ColorSpace)),
			ChromaLocation:     strings.ToLower(strings.TrimSpace(raw.ChromaLocation)),
			FieldOrder:         strings.ToLower(strings.TrimSpace(raw.FieldOrder)),
			DynamicRange:       dynamicRange,
			DolbyVisionProfile: ffprobeDolbyVisionProfile(raw),
		})
	}
	chapters := chaptersFromFFprobe(mediaID, payload.Chapters)
	return duration, streams, chapters, attachments
}

func mediaAttachmentFromFFprobe(mediaID string, raw ffprobeStream) MediaAttachment {
	filename := firstNonEmpty(raw.Tags["filename"], raw.Tags["FILENAME"], raw.Tags["title"], raw.Tags["TITLE"], fmt.Sprintf("attachment_%d", raw.Index))
	filename = safeAttachmentFilename(filename)
	return MediaAttachment{
		ID:       randomOpaquePublicID(),
		StreamID: randomOpaquePublicID(),
		Filename: filename,
		MimeType: firstNonEmpty(raw.Tags["mimetype"], raw.Tags["MIMETYPE"], raw.Tags["mime_type"], raw.Tags["MIME_TYPE"]),
		Codec:    normalizeCodec(raw.CodecName),
	}
}

func safeAttachmentFilename(value string) string {
	base := filepath.Base(strings.TrimSpace(value))
	base = strings.Trim(base, ". ")
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "attachment.bin"
	}
	var builder strings.Builder
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == ' ' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "attachment.bin"
	}
	return builder.String()
}

func (s *Server) extractMediaAttachments(ctx context.Context, item MediaItem, sourcePath string, attachments []MediaAttachment) ([]MediaAttachment, error) {
	ffmpegPath := strings.TrimSpace(s.cfg.FFmpegPath)
	if ffmpegPath == "" || len(attachments) == 0 {
		return attachments, nil
	}
	if _, err := exec.LookPath(ffmpegPath); err != nil && filepath.Base(ffmpegPath) == ffmpegPath {
		return attachments, nil
	}
	outputDir := filepath.Join(s.cfg.AppDataDir, "attachments", safePathComponent(item.ID))
	if err := os.RemoveAll(outputDir); err != nil {
		s.log.Warn("embedded attachment cleanup failed", "media", item.ID, "error", err)
		return attachments, nil
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		s.log.Warn("embedded attachment directory failed", "media", item.ID, "error", err)
		return attachments, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	args := []string{"-hide_banner", "-nostdin", "-threads", "1", "-dump_attachment:t", "", "-i", sourcePath}
	args = append(args, analysisProgressArgs()...)
	args = append(args, "-f", "null", "-")
	if result, err := s.runAnalysisSourceCommand(ctx, sourcePath, "extract embedded attachments", ffmpegPath, args, outputDir, 2<<20, 4<<20); err != nil {
		if ctx.Err() != nil {
			return attachments, ctx.Err()
		}
		s.log.Warn("embedded attachment extraction failed", "media", item.ID, "error", err, "output", redactedAnalysisOutput(result.Stderr, sourcePath))
		return attachments, nil
	}
	for i := range attachments {
		path := filepath.Join(outputDir, safeAttachmentFilename(attachments[i].Filename))
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		attachments[i].URL = path
		attachments[i].SizeBytes = info.Size()
	}
	return attachments, nil
}

func ffprobeStreamIsAttachedPicture(stream ffprobeStream) bool {
	if stream.Disposition != nil && stream.Disposition["attached_pic"] == 1 {
		return true
	}
	title := strings.ToLower(firstNonEmpty(stream.Tags["title"], stream.Tags["TITLE"], stream.Tags["comment"], stream.Tags["COMMENT"]))
	return strings.Contains(title, "cover") || strings.Contains(title, "front")
}

func ffprobeKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "video":
		return "video"
	case "audio":
		return "audio"
	case "subtitle":
		return "subtitle"
	default:
		return ""
	}
}

func secondsFromFFprobe(value string) int {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return int(math.Round(parsed))
}

func intFromString(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return max(0, parsed)
}

func ffprobeDisplayTitle(kind, codec, language string, stream ffprobeStream) string {
	parts := []string{}
	if language != "" {
		parts = append(parts, language)
	}
	if codec != "" {
		parts = append(parts, strings.ToUpper(codec))
	}
	if kind == "video" && stream.Width > 0 && stream.Height > 0 {
		parts = append(parts, fmt.Sprintf("%dx%d", stream.Width, stream.Height))
	}
	if kind == "video" && ffprobeStreamIsHDR(stream) {
		parts = append(parts, "HDR")
	}
	if kind == "audio" && stream.Channels > 0 {
		parts = append(parts, fmt.Sprintf("%d ch", stream.Channels))
	}
	if len(parts) == 0 {
		return strings.Title(kind)
	}
	return strings.Join(parts, " - ")
}

func ffprobeStreamIsHDR(stream ffprobeStream) bool {
	transfer := strings.ToLower(stream.ColorTransfer)
	primaries := strings.ToLower(stream.ColorPrimaries)
	return strings.Contains(transfer, "smpte2084") ||
		strings.Contains(transfer, "arib-std-b67") ||
		strings.Contains(transfer, "hlg") ||
		strings.Contains(primaries, "bt2020")
}

func ffprobeDynamicRange(stream ffprobeStream) string {
	if profile := ffprobeDolbyVisionProfile(stream); profile != "" {
		return "dolby_vision_profile_" + profile
	}
	transfer := strings.ToLower(stream.ColorTransfer)
	primaries := strings.ToLower(stream.ColorPrimaries)
	if strings.Contains(transfer, "arib-std-b67") || strings.Contains(transfer, "hlg") {
		return "hlg"
	}
	if strings.Contains(transfer, "smpte2084") || strings.Contains(primaries, "bt2020") {
		return "hdr10"
	}
	return "sdr"
}

func ffprobeDolbyVisionProfile(stream ffprobeStream) string {
	for _, sideData := range stream.SideDataList {
		if sideData.DVProfile > 0 {
			return strconv.Itoa(sideData.DVProfile)
		}
		if strings.Contains(strings.ToLower(sideData.SideDataType), "dovi") || strings.Contains(strings.ToLower(sideData.SideDataType), "dolby vision") {
			for _, key := range []string{"DOVI profile", "dovi_profile", "dolby_vision_profile"} {
				if value := strings.TrimSpace(stream.Tags[key]); value != "" {
					return value
				}
			}
		}
	}
	for _, key := range []string{"DOVI profile", "dovi_profile", "dolby_vision_profile"} {
		if value := strings.TrimSpace(stream.Tags[key]); value != "" {
			return value
		}
	}
	return ""
}

func ffprobeBitDepth(stream ffprobeStream) int {
	if depth := intFromString(stream.BitsPerRawSample); depth > 0 {
		return depth
	}
	pix := strings.ToLower(stream.PixelFormat)
	switch {
	case strings.Contains(pix, "12"):
		return 12
	case strings.Contains(pix, "10"):
		return 10
	case strings.Contains(pix, "16"):
		return 16
	case pix != "":
		return 8
	default:
		return 0
	}
}

func chaptersFromFFprobe(mediaID string, rawChapters []ffprobeChapter) []Chapter {
	chapters := []Chapter{}
	for index, raw := range rawChapters {
		start := secondsFromFFprobe(raw.StartTime)
		end := secondsFromFFprobe(raw.EndTime)
		if start < 0 || (end > 0 && end <= start) {
			continue
		}
		title := strings.TrimSpace(firstNonEmpty(raw.Tags["title"], raw.Tags["TITLE"]))
		if title == "" {
			title = fmt.Sprintf("Chapter %d", index+1)
		}
		chapters = append(chapters, Chapter{
			ID:           randomOpaquePublicID(),
			Title:        title,
			StartSeconds: start,
			EndSeconds:   end,
		})
	}
	return chapters
}

func (s *Server) chaptersForMedia(mediaID string) []Chapter {
	return s.chaptersForMediaContext(context.Background(), mediaID)
}

func (s *Server) chaptersForMediaContext(ctx context.Context, mediaID string) []Chapter {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT id, title, start_seconds, end_seconds
		FROM media_chapters
		WHERE media_id = ?
		ORDER BY sort_order ASC, start_seconds ASC`, mediaID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	chapters := []Chapter{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return chapters
		}
		var chapter Chapter
		if err := rows.Scan(&chapter.ID, &chapter.Title, &chapter.StartSeconds, &chapter.EndSeconds); err != nil {
			return chapters
		}
		if _, ok := s.generatedArtworkPath(mediaID, chapter.ID); ok {
			chapter.ThumbURL = "/api/artwork/" + url.PathEscape(mediaID) + "/" + url.PathEscape(chapter.ID) + ".jpg"
		}
		chapters = append(chapters, chapter)
	}
	return chapters
}

func hasVideoStream(streams []Stream) bool {
	for _, stream := range streams {
		if stream.Kind == "video" {
			return true
		}
	}
	return false
}

func (s *Server) generateMediaThumbnail(ctx context.Context, item MediaItem) error {
	path, err := s.localSourcePathForTranscode(item)
	if err != nil {
		return err
	}
	return s.generateMediaThumbnailFromPath(ctx, item, path)
}

func (s *Server) generateMediaThumbnailFromPath(ctx context.Context, item MediaItem, path string) error {
	if _, err := exec.LookPath(s.cfg.FFmpegPath); err != nil && filepath.Base(s.cfg.FFmpegPath) == s.cfg.FFmpegPath {
		return errors.New("FFmpeg is not available on PATH")
	}
	outputDir := filepath.Join(s.cfg.AppDataDir, "artwork", safePathComponent(item.ID))
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return err
	}
	outputPath := filepath.Join(outputDir, "thumb.jpg")
	versionPath := filepath.Join(outputDir, "thumb.version")
	if outputInfo, outputErr := os.Stat(outputPath); outputErr == nil && !outputInfo.IsDir() {
		version, versionErr := os.ReadFile(versionPath)
		sourceInfo, sourceErr := s.analysisSourceStat(ctx, path, "inspect thumbnail source")
		if versionErr == nil && strings.TrimSpace(string(version)) == mediaThumbnailVersion && (sourceErr != nil || !outputInfo.ModTime().Before(sourceInfo.ModTime())) {
			return nil
		}
	}
	tempPath := outputPath + ".tmp"
	_ = os.Remove(tempPath)
	// A representative midpoint frame is substantially more useful than the
	// title cards, fades, and recaps commonly found in the opening minute. The
	// generated file remains a thumbnail fallback only; it is never recorded as
	// provider or user artwork, so a later metadata refresh can replace it.
	seekSeconds := representativeThumbnailSecond(item.DurationSeconds)
	seek := strconv.FormatFloat(seekSeconds, 'f', 3, 64)
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-threads", "1",
		"-filter_threads", "1",
		"-protocol_whitelist", "file,pipe",
		"-ss", seek,
		"-i", path,
	}
	args = append(args, analysisProgressArgs()...)
	args = append(args,
		"-frames:v", "1",
		"-vf", "scale=640:-2",
		"-q:v", "4",
		tempPath,
	)
	if _, err := s.runAnalysisSourceCommand(ctx, path, "generate representative thumbnail", s.cfg.FFmpegPath, args, "", 1<<20, 4<<20); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, outputPath); err != nil {
		return err
	}
	if err := os.Chmod(outputPath, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(versionPath, []byte(mediaThumbnailVersion+"\n"), 0o600); err != nil {
		return err
	}
	// art_seed is the cache revision for artwork URLs. Advancing it only after
	// the replacement file is durable gives every client a new URL and clears
	// any earlier optional-resource miss without exposing a filesystem path.
	if _, err := s.execBackgroundWriteTagged(ctx, []string{"media", "metadata", "library-items", "artwork"}, `
		UPDATE media_items SET art_seed = art_seed + 1 WHERE id = ?`, item.ID); err != nil {
		return err
	}
	return nil
}

func (s *Server) mediaNeedsRepresentativeFrameContext(ctx context.Context, item MediaItem) bool {
	switch strings.ToLower(strings.TrimSpace(item.Type)) {
	case "movie", "episode", "extra", "video":
	default:
		return false
	}
	if _, ok := s.generatedArtworkPath(item.ID, "thumb"); ok {
		return false
	}
	rows, err := s.queryBackgroundRead(ctx, `
		SELECT path FROM media_images
		WHERE media_id = ? AND image_type IN ('thumb', 'poster', 'backdrop') AND trim(path) <> ''
		UNION ALL
		SELECT CAST(value AS TEXT) FROM media_items, json_each(
			CASE WHEN json_valid(media_items.artwork_json) THEN media_items.artwork_json ELSE '{}' END
		)
		WHERE media_items.id = ? AND key IN ('thumb', 'poster', 'backdrop') AND trim(CAST(value AS TEXT)) <> ''`, item.ID, item.ID)
	if err != nil {
		return true
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		if rows.Scan(&path) == nil && localArtworkFileExists(path) {
			return false
		}
	}
	return true
}

func representativeThumbnailSecond(durationSeconds int) float64 {
	if durationSeconds > 20 {
		return float64(durationSeconds) / 2
	}
	return 10
}

func (s *Server) extractEmbeddedCoverImage(ctx context.Context, item MediaItem, sourcePath string, payload ffprobePayload) error {
	if item.Type != "track" && item.Type != "audiobook" {
		return nil
	}
	streamIndex, ok := embeddedCoverStreamIndex(payload)
	if !ok {
		return nil
	}
	if _, err := exec.LookPath(s.cfg.FFmpegPath); err != nil && filepath.Base(s.cfg.FFmpegPath) == s.cfg.FFmpegPath {
		return errors.New("FFmpeg is not available on PATH")
	}
	outputPath, tempPath, err := s.prepareEmbeddedCoverOutput(item.ID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-threads", "1",
		"-filter_threads", "1",
		"-protocol_whitelist", "file,pipe",
		"-i", sourcePath,
	}
	args = append(args, analysisProgressArgs()...)
	args = append(args,
		"-map", fmt.Sprintf("0:%d", streamIndex),
		"-frames:v", "1",
		"-update", "1",
		tempPath,
	)
	if result, err := s.runAnalysisSourceCommand(ctx, sourcePath, "extract embedded cover", s.cfg.FFmpegPath, args, "", 1<<20, 4<<20); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(redactedAnalysisOutput(result.Stderr, sourcePath)))
	}
	return s.publishEmbeddedCoverOutput(item.ID, outputPath, tempPath)
}

func (s *Server) prepareEmbeddedCoverOutput(mediaID string) (string, string, error) {
	outputDir := filepath.Join(s.cfg.AppDataDir, "artwork", safePathComponent(mediaID))
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return "", "", err
	}
	outputPath := filepath.Join(outputDir, "embedded_cover.jpg")
	tempPath := outputPath + ".tmp.jpg"
	_ = os.Remove(tempPath)
	return outputPath, tempPath, nil
}

func (s *Server) publishEmbeddedCoverOutput(mediaID, outputPath, tempPath string) error {
	if err := os.Rename(tempPath, outputPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Chmod(outputPath, 0o600); err != nil {
		return err
	}
	return s.upsertEmbeddedCoverImage(mediaID, outputPath)
}

func embeddedCoverStreamIndex(payload ffprobePayload) (int, bool) {
	for _, stream := range payload.Streams {
		if stream.CodecType == "video" && ffprobeStreamIsAttachedPicture(stream) {
			return stream.Index, true
		}
	}
	return 0, false
}

func (s *Server) upsertEmbeddedCoverImage(mediaID, path string) error {
	if strings.TrimSpace(mediaID) == "" || strings.TrimSpace(path) == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return s.withBackgroundTxTagged(context.Background(), []string{"media", "metadata", "library-items"}, func(tx *sql.Tx) error {
		id := ""
		_ = tx.QueryRow(`SELECT id FROM media_images WHERE media_id = ? AND image_type = 'poster' AND source = 'embedded' ORDER BY preferred DESC, created_at DESC LIMIT 1`, mediaID).Scan(&id)
		if strings.TrimSpace(id) == "" {
			id = randomOpaquePublicID()
		}
		if _, err := tx.Exec(`DELETE FROM media_images WHERE media_id = ? AND image_type = 'poster' AND source = 'embedded'`, mediaID); err != nil {
			return err
		}
		_, err := tx.Exec(`
			INSERT INTO media_images (
				id, media_id, image_type, source, provider, path, remote_url, width, height, language, rating, preferred, created_at
			) VALUES (?, ?, 'poster', 'embedded', '', ?, '', 0, 0, '', 0, 1, ?)
			ON CONFLICT(media_id, image_type, source, path, remote_url) DO UPDATE SET
				preferred = excluded.preferred,
				created_at = excluded.created_at`,
			id, mediaID, path, now)
		return err
	})
}

func (s *Server) generateChapterThumbnails(ctx context.Context, item MediaItem, chapters []Chapter) error {
	if len(chapters) == 0 {
		return nil
	}
	path, err := s.localSourcePathForTranscode(item)
	if err != nil {
		return err
	}
	return s.generateChapterThumbnailsFromPath(ctx, item, path, chapters)
}

func (s *Server) generateChapterThumbnailsFromPath(ctx context.Context, item MediaItem, path string, chapters []Chapter) error {
	if len(chapters) == 0 {
		return nil
	}
	if _, err := exec.LookPath(s.cfg.FFmpegPath); err != nil && filepath.Base(s.cfg.FFmpegPath) == s.cfg.FFmpegPath {
		return errors.New("FFmpeg is not available on PATH")
	}
	outputDir := filepath.Join(s.cfg.AppDataDir, "artwork", safePathComponent(item.ID))
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return err
	}
	for _, chapter := range chapters {
		if chapter.StartSeconds < 0 {
			continue
		}
		outputPath := filepath.Join(outputDir, safePathComponent(chapter.ID)+".jpg")
		tempPath := outputPath + ".tmp"
		_ = os.Remove(tempPath)
		chapterCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		args := []string{
			"-hide_banner", "-nostdin", "-y",
			"-threads", "1",
			"-filter_threads", "1",
			"-protocol_whitelist", "file,pipe",
			"-ss", strconv.Itoa(chapter.StartSeconds),
			"-i", path,
		}
		args = append(args, analysisProgressArgs()...)
		args = append(args,
			"-frames:v", "1",
			"-vf", "scale=480:-2",
			"-q:v", "5",
			tempPath,
		)
		_, err := s.runAnalysisSourceCommand(chapterCtx, path, "generate chapter thumbnail", s.cfg.FFmpegPath, args, "", 1<<20, 4<<20)
		cancel()
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
	}
	return nil
}

func (s *Server) generateMediaTrickplay(ctx context.Context, item MediaItem, sourcePath string, duration int, streams []Stream) error {
	if duration <= 0 || !hasVideoStream(streams) {
		return nil
	}
	if _, err := exec.LookPath(s.cfg.FFmpegPath); err != nil && filepath.Base(s.cfg.FFmpegPath) == s.cfg.FFmpegPath {
		return errors.New("FFmpeg is not available on PATH")
	}
	options := s.trickplayGenerationOptions(item, duration)
	interval := options.IntervalSeconds
	tileWidth, tileHeight := trickplayTileDimensions(streams, options.TileWidth)
	setID := ""
	_ = s.queryBackgroundRow(ctx, `
		SELECT id FROM media_trickplay_sets
		WHERE media_id = ? AND media_file_id = '' AND tile_width = ? AND interval_seconds = ?
		ORDER BY stale ASC, created_at DESC LIMIT 1`, item.ID, tileWidth, interval).Scan(&setID)
	if strings.TrimSpace(setID) == "" {
		setID = randomOpaquePublicID()
	}
	outputDir, err := s.trickplayOutputDir(item, sourcePath, setID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(outputDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return err
	}
	outputPattern := filepath.Join(outputDir, "tile_%05d.jpg")
	filter := fmt.Sprintf("fps=1/%d,scale=%d:-2", interval, tileWidth)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-threads", "1",
		"-filter_threads", "1",
		"-protocol_whitelist", "file,pipe",
		"-i", sourcePath,
	}
	args = append(args, analysisProgressArgs()...)
	args = append(args,
		"-vf", filter,
		"-frames:v", strconv.Itoa(options.MaxTiles),
		"-q:v", "5",
		outputPattern,
	)
	if result, err := s.runAnalysisSourceCommand(ctx, sourcePath, "generate trickplay tiles", s.cfg.FFmpegPath, args, "", 1<<20, 8<<20); err != nil {
		_ = os.RemoveAll(outputDir)
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(redactedAnalysisOutput(result.Stderr, sourcePath)))
	}
	tiles, err := filepath.Glob(filepath.Join(outputDir, "tile_*.jpg"))
	if err != nil {
		return err
	}
	sort.Strings(tiles)
	if len(tiles) == 0 {
		return errors.New("FFmpeg did not produce trickplay tiles")
	}
	for _, tile := range tiles {
		if err := os.Chmod(tile, 0o600); err != nil {
			return err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return s.withBackgroundTxTagged(context.Background(), []string{"media", "metadata", "library-items"}, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE media_trickplay_sets SET stale = 1 WHERE media_id = ?`, item.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM media_trickplay_tiles WHERE set_id = ?`, setID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM media_trickplay_sets WHERE id = ?`, setID); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO media_trickplay_sets (
				id, media_id, media_file_id, width, height, tile_width, tile_height,
				interval_seconds, duration_seconds, tile_count, path, stale, created_at
			) VALUES (?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`,
			setID, item.ID, tileWidth, tileHeight, tileWidth, tileHeight, interval, duration, len(tiles), outputDir, now); err != nil {
			return err
		}
		for index, tile := range tiles {
			end := min(duration, (index+1)*interval)
			if _, err := tx.Exec(`
				INSERT INTO media_trickplay_tiles (id, set_id, tile_index, start_seconds, end_seconds, row, col, path, created_at)
				VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)`,
				scannedID("tricktile", strings.Join([]string{setID, strconv.Itoa(index)}, "\x00")),
				setID, index, index*interval, end, index, tile, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Server) trickplayOutputDir(item MediaItem, sourcePath string, setID string) (string, error) {
	if library, err := s.getLibrary(item.LibraryID); err == nil && normalizeTrickplayStorageLocation(settingString(library.Settings, "trickplayStorageLocation", "")) == "with_media" {
		cleanSource, err := s.validateLocalMediaPath(item.LibraryID, sourcePath)
		if err != nil {
			return "", err
		}
		return filepath.Join(filepath.Dir(cleanSource), ".portico", "trickplay", safePathComponent(setID)), nil
	}
	return filepath.Join(s.cfg.AppDataDir, "trickplay", safePathComponent(item.ID), safePathComponent(setID)), nil
}

func normalizeTrickplayStorageLocation(value string) string {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "-", "_"))) {
	case "with_media", "beside_media":
		return "with_media"
	default:
		return "app_data"
	}
}

func trickplayIntervalSeconds(duration int, targetTiles int) int {
	if duration <= 0 {
		return 10
	}
	targetTiles = max(24, min(2000, targetTiles))
	return max(10, int(math.Ceil(float64(duration)/float64(targetTiles))))
}

func trickplayTileDimensions(streams []Stream, targetWidth int) (int, int) {
	if targetWidth <= 0 {
		targetWidth = 160
	}
	for _, stream := range streams {
		if stream.Kind != "video" || stream.Width <= 0 || stream.Height <= 0 {
			continue
		}
		height := int(math.Round(float64(targetWidth) * float64(stream.Height) / float64(stream.Width)))
		if height%2 != 0 {
			height++
		}
		return targetWidth, max(2, height)
	}
	return targetWidth, 90
}

type trickplayStorageRecord struct {
	id        string
	mediaID   string
	path      string
	stale     bool
	createdAt string
	sizeBytes int64
}

func (s *Server) pruneTrickplaySets(retentionDays int, maxStorageMB int) (int, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(max(0, retentionDays)) * 24 * time.Hour).Format(time.RFC3339)
	records, err := s.trickplayStorageRecords(`
		WHERE stale = 1 AND created_at <= ?
		ORDER BY created_at ASC`, []any{cutoff})
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, record := range records {
		if err := s.deleteTrickplayStorageRecord(record, true); err != nil {
			return removed, err
		}
		removed++
	}

	maxStorageBytes := int64(max(0, maxStorageMB)) * 1024 * 1024
	if maxStorageBytes <= 0 {
		return removed, nil
	}
	allRecords, err := s.trickplayStorageRecords(`ORDER BY stale DESC, created_at ASC`, nil)
	if err != nil {
		return removed, err
	}
	totalBytes := int64(0)
	for index := range allRecords {
		size, _, available, err := pathUsage(allRecords[index].path)
		if err != nil {
			return removed, err
		}
		if !available {
			allRecords[index].sizeBytes = 0
			continue
		}
		allRecords[index].sizeBytes = size
		totalBytes += size
	}
	for _, record := range allRecords {
		if totalBytes <= maxStorageBytes {
			break
		}
		if err := s.deleteTrickplayStorageRecord(record, false); err != nil {
			return removed, err
		}
		totalBytes -= record.sizeBytes
		removed++
	}
	return removed, nil
}

func (s *Server) trickplayStorageRecords(clause string, args []any) ([]trickplayStorageRecord, error) {
	rows, err := s.queryBackgroundRead(context.Background(), `
		SELECT id, media_id, path, stale, created_at
		FROM media_trickplay_sets
		`+clause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []trickplayStorageRecord{}
	for rows.Next() {
		var record trickplayStorageRecord
		var stale int
		if err := rows.Scan(&record.id, &record.mediaID, &record.path, &stale, &record.createdAt); err != nil {
			return nil, err
		}
		record.stale = stale != 0
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Server) deleteTrickplayStorageRecord(record trickplayStorageRecord, staleOnly bool) error {
	root := filepath.Clean(filepath.Join(s.cfg.AppDataDir, "trickplay"))
	cleanPath := filepath.Clean(record.path)
	if record.path != "" && (pathInsideRoot(cleanPath, root) || s.withMediaTrickplayPathAllowed(record.mediaID, record.id, cleanPath)) {
		if err := os.RemoveAll(cleanPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	query := `DELETE FROM media_trickplay_sets WHERE id = ?`
	args := []any{record.id}
	if staleOnly {
		query += ` AND stale = 1`
	}
	_, err := s.execBackgroundWrite(context.Background(), query, args...)
	return err
}

func (s *Server) withMediaTrickplayPathAllowed(mediaID string, setID string, path string) bool {
	item, err := s.getMedia("", mediaID)
	if err != nil {
		return false
	}
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	validated, err := s.validateLocalMediaPath(item.LibraryID, cleanPath)
	if err != nil {
		return false
	}
	suffix := "/.portico/trickplay/" + safePathComponent(setID)
	return strings.HasSuffix(filepath.ToSlash(validated), suffix) || strings.Contains(filepath.ToSlash(validated), suffix+"/")
}

func (s *Server) generatedArtworkPath(mediaID, kind string) (string, bool) {
	if kind != "thumb" && !strings.HasPrefix(kind, mediaID+"_chapter_") {
		return "", false
	}
	filename := "thumb.jpg"
	if strings.HasPrefix(kind, mediaID+"_chapter_") {
		filename = safePathComponent(kind) + ".jpg"
	}
	path := filepath.Join(s.cfg.AppDataDir, "artwork", safePathComponent(mediaID), filename)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}

func (s *Server) localArtworkPath(item MediaItem, kind string) (string, bool) {
	if path, ok := s.mediaImagePath(item.ID, kind); ok {
		return path, true
	}
	sourcePath, ok := localPathFromSourceURL(item.SourceURL)
	if !ok {
		return "", false
	}
	sourcePath, err := s.validateLocalMediaPath(item.LibraryID, sourcePath)
	if err != nil {
		return "", false
	}
	dir := filepath.Dir(sourcePath)
	for _, name := range localArtworkNames(kind) {
		candidate := filepath.Join(dir, name)
		validated, err := s.validateLocalMediaPath(item.LibraryID, candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(validated)
		if err == nil && !info.IsDir() {
			return validated, true
		}
	}
	return "", false
}

func (s *Server) artworkPolicyForItem(item MediaItem) string {
	if library, err := s.getLibrary(item.LibraryID); err == nil {
		if policy := normalizeArtworkPolicy(settingString(library.Settings, "imagePolicy", "")); policy != "" {
			return policy
		}
	}
	return "local_first"
}

func normalizeArtworkPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "local_first", "provider_first", "local_only", "provider_only":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func artworkSourceOrder(policy string) []string {
	switch normalizeArtworkPolicy(policy) {
	case "provider_first":
		return []string{"provider", "local"}
	case "local_only":
		return []string{"local"}
	case "provider_only":
		return []string{"provider"}
	default:
		return []string{"local", "provider"}
	}
}

func localPathFromSourceURL(sourceURL string) (string, bool) {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return "", false
	}
	parsed, err := url.Parse(sourceURL)
	if err == nil && parsed.Scheme == "file" {
		return parsed.Path, true
	}
	if err == nil && parsed.Scheme != "" {
		return "", false
	}
	return sourceURL, true
}

func localArtworkNames(kind string) []string {
	return localArtworkNamesForType("", kind)
}
