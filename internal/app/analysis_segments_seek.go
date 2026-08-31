package app

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	segmentDetectorVersion       = "portico-av-boundary-v1"
	segmentDetectorProvider      = "portico-av-boundary-v1"
	segmentDetectorTimeout       = 30 * time.Minute
	boundedSeekProbeTimeout      = 45 * time.Second
	boundedSeekProbeTargetLimit  = 24
	boundedSeekProbeWindow       = 0.30
	exactSeekBoundaryTolerance   = 0.12
	segmentBoundaryToleranceSecs = 3.0
)

var (
	blackIntervalPattern = regexp.MustCompile(`black_start:([0-9]+(?:\.[0-9]+)?)\s+black_end:([0-9]+(?:\.[0-9]+)?)\s+black_duration:([0-9]+(?:\.[0-9]+)?)`)
	silenceStartPattern  = regexp.MustCompile(`silence_start:\s*([0-9]+(?:\.[0-9]+)?)`)
	silenceEndPattern    = regexp.MustCompile(`silence_end:\s*([0-9]+(?:\.[0-9]+)?)`)
)

type analysisSignalInterval struct {
	Start float64
	End   float64
}

type segmentSignalEvidence struct {
	Black   []analysisSignalInterval
	Silence []analysisSignalInterval
}

type analyzedSegmentCandidate struct {
	Type         string
	StartSeconds int
	EndSeconds   int
	Confidence   float64
}

type segmentAnalysisEvidence struct {
	DetectorVersion string `json:"detectorVersion"`
	BlackIntervals  int    `json:"blackIntervals"`
	SilentIntervals int    `json:"silentIntervals"`
	ChapterCount    int    `json:"chapterCount"`
	FindingCount    int    `json:"findingCount"`
	AutomaticSafe   bool   `json:"automaticSafe"`
}

var errSegmentAnalysisSourceChanged = errors.New("media source changed during segment analysis")

// detectMediaSegments runs one content-wide, supervised FFmpeg pass and uses
// chapter labels only to name ranges whose boundaries are independently
// supported by audiovisual signal evidence. A chapter title can never create
// a marker by itself. Generated candidates remain advisory: automatic skip is
// reserved for a future detector with stronger cross-item evidence.
func (s *Server) detectMediaSegments(
	ctx context.Context,
	item MediaItem,
	recordPath string,
	analysisPath string,
	payload ffprobePayload,
	chapters []Chapter,
	file analysisFileIdentity,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.mediaAnalysisOptions(item, mediaAnalysisModeFull).DetectSegments {
		return nil
	}
	if s.segmentAnalysisAlreadyCurrent(ctx, item.ID, file) {
		return nil
	}

	evidence := segmentSignalEvidence{}
	if payloadHasVideoStream(payload) {
		if !s.waitForAnalysisForegroundWindow(ctx) {
			return ctx.Err()
		}
		var err error
		evidence, err = s.detectSegmentSignalsWithFFmpeg(ctx, analysisPath, payloadHasAudioStream(payload))
		if err != nil {
			return err
		}
	}
	duration, _ := strconv.ParseFloat(strings.TrimSpace(payload.Format.Duration), 64)
	candidates := analyzedSegmentsFromSignals(chapters, duration, evidence)
	return s.publishSegmentAnalysis(ctx, item, recordPath, file, chapters, evidence, candidates)
}

func (s *Server) detectSegmentSignalsWithFFmpeg(ctx context.Context, path string, hasAudio bool) (segmentSignalEvidence, error) {
	ffmpegPath := strings.TrimSpace(s.cfg.FFmpegPath)
	if ffmpegPath == "" {
		return segmentSignalEvidence{}, errors.New("ffmpeg is not configured")
	}
	if filepath.Base(ffmpegPath) == ffmpegPath {
		resolved, err := exec.LookPath(ffmpegPath)
		if err != nil {
			return segmentSignalEvidence{}, errors.New("ffmpeg is not available on PATH")
		}
		ffmpegPath = resolved
	}
	processCtx, cancel := context.WithTimeout(ctx, segmentDetectorTimeout)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostdin", "-nostats", "-threads", "1", "-filter_threads", "1",
		"-i", path, "-map", "0:v:0", "-vf", "blackdetect=d=0.75:pix_th=0.10",
	}
	if hasAudio {
		args = append(args, "-map", "0:a:0?", "-af", "silencedetect=n=-45dB:d=0.75")
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-sn", "-dn")
	args = append(args, analysisProgressArgs()...)
	args = append(args, "-f", "null", "-")
	result, err := s.runAnalysisSourceCommand(processCtx, path, "detect playback segments", ffmpegPath, args, "", 1<<20, 8<<20)
	if err != nil {
		return segmentSignalEvidence{}, fmt.Errorf("detect playback segments: %w", err)
	}
	return parseSegmentSignalEvidence(result.Stderr), nil
}

func parseSegmentSignalEvidence(output []byte) segmentSignalEvidence {
	evidence := segmentSignalEvidence{}
	pendingSilence := []float64{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if match := blackIntervalPattern.FindStringSubmatch(line); len(match) == 4 {
			start, startErr := strconv.ParseFloat(match[1], 64)
			end, endErr := strconv.ParseFloat(match[2], 64)
			if startErr == nil && endErr == nil && end > start {
				evidence.Black = append(evidence.Black, analysisSignalInterval{Start: start, End: end})
			}
		}
		if match := silenceStartPattern.FindStringSubmatch(line); len(match) == 2 {
			if start, err := strconv.ParseFloat(match[1], 64); err == nil && start >= 0 {
				pendingSilence = append(pendingSilence, start)
			}
		}
		if match := silenceEndPattern.FindStringSubmatch(line); len(match) == 2 && len(pendingSilence) > 0 {
			end, err := strconv.ParseFloat(match[1], 64)
			start := pendingSilence[0]
			pendingSilence = pendingSilence[1:]
			if err == nil && end > start {
				evidence.Silence = append(evidence.Silence, analysisSignalInterval{Start: start, End: end})
			}
		}
	}
	return evidence
}

func analyzedSegmentsFromSignals(chapters []Chapter, duration float64, evidence segmentSignalEvidence) []analyzedSegmentCandidate {
	if duration <= 0 || len(evidence.Black) == 0 || len(evidence.Silence) == 0 {
		return nil
	}
	candidates := make([]analyzedSegmentCandidate, 0)
	for _, chapter := range chapters {
		kind := analyzedSegmentTypeFromChapterTitle(chapter.Title)
		if kind == "" || chapter.StartSeconds < 0 || chapter.EndSeconds <= chapter.StartSeconds || float64(chapter.EndSeconds) > duration+1 {
			continue
		}
		segmentDuration := chapter.EndSeconds - chapter.StartSeconds
		if !plausibleAnalyzedSegmentDuration(kind, chapter.StartSeconds, segmentDuration, duration) {
			continue
		}
		startBlack := intervalEvidenceNear(evidence.Black, float64(chapter.StartSeconds), segmentBoundaryToleranceSecs)
		startSilence := intervalEvidenceNear(evidence.Silence, float64(chapter.StartSeconds), segmentBoundaryToleranceSecs)
		endBlack := intervalEvidenceNear(evidence.Black, float64(chapter.EndSeconds), segmentBoundaryToleranceSecs)
		endSilence := intervalEvidenceNear(evidence.Silence, float64(chapter.EndSeconds), segmentBoundaryToleranceSecs)
		startAV, endAV := startBlack && startSilence, endBlack && endSilence
		if !startAV && !endAV {
			continue
		}
		// Mid-program ranges need evidence at both edges. A single terminal AV
		// boundary is accepted only for an opening range or a credits/outro
		// range at the end of the item, and remains advisory.
		terminal := chapter.StartSeconds == 0 || kind == "credits" && float64(chapter.EndSeconds) >= duration-3
		if !terminal && !(startAV && endAV) {
			continue
		}
		confidence := 0.72
		if startAV && endAV {
			confidence = 0.86
		}
		candidates = append(candidates, analyzedSegmentCandidate{
			Type: kind, StartSeconds: chapter.StartSeconds, EndSeconds: chapter.EndSeconds, Confidence: confidence,
		})
	}
	return candidates
}

func analyzedSegmentTypeFromChapterTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	switch {
	case title == "" || title == "chapter":
		return ""
	case strings.Contains(title, "previously"), strings.Contains(title, "recap"), strings.Contains(title, "last time"):
		return "recap"
	case strings.Contains(title, "commercial"), strings.Contains(title, "ad break"), strings.Contains(title, "intermission"):
		return "commercial"
	case strings.Contains(title, "intro"), strings.Contains(title, "opening title"), strings.Contains(title, "opening credit"):
		return "intro"
	case strings.Contains(title, "end credit"), strings.Contains(title, "closing credit"), strings.Contains(title, "credits"), strings.Contains(title, "outro"):
		return "credits"
	default:
		return ""
	}
}

func plausibleAnalyzedSegmentDuration(kind string, start, length int, duration float64) bool {
	if length < 5 {
		return false
	}
	switch kind {
	case "intro", "recap":
		return length <= 300 && float64(start) <= duration*0.25
	case "commercial":
		return length <= 600
	case "credits":
		return length <= 1200 && float64(start) >= duration*0.50
	default:
		return false
	}
}

func intervalEvidenceNear(intervals []analysisSignalInterval, boundary, tolerance float64) bool {
	for _, interval := range intervals {
		if math.Abs(interval.Start-boundary) <= tolerance || math.Abs(interval.End-boundary) <= tolerance || interval.Start <= boundary && interval.End >= boundary {
			return true
		}
	}
	return false
}

func (s *Server) segmentAnalysisAlreadyCurrent(ctx context.Context, mediaID string, file analysisFileIdentity) bool {
	var exists int
	err := s.queryBackgroundRow(ctx, `SELECT 1 FROM media_segment_analysis_runs
		WHERE media_id=? AND media_file_id=? AND source_revision=? AND detector_version=?`,
		mediaID, file.ID, file.revision(), segmentDetectorVersion).Scan(&exists)
	return err == nil && exists == 1
}

func (s *Server) publishSegmentAnalysis(
	ctx context.Context,
	item MediaItem,
	recordPath string,
	file analysisFileIdentity,
	chapters []Chapter,
	evidence segmentSignalEvidence,
	candidates []analyzedSegmentCandidate,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	evidenceBody, err := json.Marshal(segmentAnalysisEvidence{
		DetectorVersion: segmentDetectorVersion, BlackIntervals: len(evidence.Black), SilentIntervals: len(evidence.Silence),
		ChapterCount: len(chapters), FindingCount: len(candidates), AutomaticSafe: false,
	})
	if err != nil {
		return err
	}
	return s.withBackgroundTxTagged(ctx, []string{"media", "library-items"}, func(tx *sql.Tx) error {
		var fingerprint, modTime string
		var size int64
		if err := tx.QueryRow(`SELECT content_fingerprint,size_bytes,mod_time FROM media_files
			WHERE id=? AND media_id=? AND path=? AND available=1`, file.ID, item.ID, recordPath).Scan(&fingerprint, &size, &modTime); err != nil {
			return errSegmentAnalysisSourceChanged
		}
		if canonicalAnalysisFileIdentity(file.ID, fingerprint, size, modTime).revision() != file.revision() {
			return errSegmentAnalysisSourceChanged
		}
		allowed, err := analysisCapabilityAuthorizedTx(tx, item.LibraryID, recordPath, "detectSegments")
		if err != nil {
			return err
		}
		if !allowed {
			return context.Canceled
		}
		if _, err := tx.Exec(`DELETE FROM media_segments WHERE media_id=? AND source='generated' AND provider=?`, item.ID, segmentDetectorProvider); err != nil {
			return err
		}
		for _, candidate := range candidates {
			identity := strings.Join([]string{item.ID, file.revision(), candidate.Type, strconv.Itoa(candidate.StartSeconds), strconv.Itoa(candidate.EndSeconds)}, "\x00")
			id, err := stableOpaquePublicResourceIDTx(tx, "media-segment", identity)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO media_segments
				(id,media_id,segment_type,start_seconds,end_seconds,automatic_safe,source,provider,confidence,created_at)
				VALUES (?,?,?,?,?,0,'generated',?,?,?)`,
				id, item.ID, candidate.Type, candidate.StartSeconds, candidate.EndSeconds, segmentDetectorProvider, candidate.Confidence, now); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`INSERT INTO media_segment_analysis_runs
			(media_id,media_file_id,source_revision,detector_version,finding_count,evidence_json,analyzed_at)
			VALUES (?,?,?,?,?,?,?)
			ON CONFLICT(media_id,media_file_id) DO UPDATE SET
				source_revision=excluded.source_revision,detector_version=excluded.detector_version,
				finding_count=excluded.finding_count,evidence_json=excluded.evidence_json,analyzed_at=excluded.analyzed_at`,
			item.ID, file.ID, file.revision(), segmentDetectorVersion, len(candidates), string(evidenceBody), now)
		return err
	})
}

// probeBoundedExactSeekEvidence is the Moderate-I/O form used by Basic and an
// independently selected Custom option. It probes at most a fixed number of
// exact HLS-grid targets. One missing sampled keyframe is conclusive unsafe
// evidence; all sampled targets are conclusive safe only when the complete
// grid fit within the cap. Otherwise the canonical fact stays unknown.
func (s *Server) probeBoundedExactSeekEvidence(ctx context.Context, path string, payload ffprobePayload) (bool, string) {
	return s.probeExactSeekEvidenceMode(ctx, path, payload, true)
}

func (s *Server) probeExactSeekEvidenceMode(ctx context.Context, path string, payload ffprobePayload, bounded bool) (bool, string) {
	stdoutLimit := 64 << 20
	if bounded {
		stdoutLimit = 8 << 20
	}
	return probeExactSeekEvidenceUsing(ctx, path, payload, bounded, []string{"-protocol_whitelist", "file,pipe"}, func(runCtx context.Context, args []string) (analysisCommandOutput, error) {
		return s.runAnalysisSourceCommand(runCtx, path, "probe keyframes", s.cfg.FFprobePath, args, "", stdoutLimit, 1<<20)
	})
}

func probeRemoteBoundedExactSeekEvidence(ctx context.Context, ffprobePath, input, demuxer string, payload ffprobePayload) (bool, string) {
	inputOptions := []string{"-protocol_whitelist", "http,tcp", "-rw_timeout", "15000000", "-f", demuxer}
	return probeExactSeekEvidenceUsing(ctx, input, payload, true, inputOptions, func(runCtx context.Context, args []string) (analysisCommandOutput, error) {
		return runBoundedAnalysisCommand(runCtx, ffprobePath, args, "", 8<<20, 1<<20)
	})
}

func probeExactSeekEvidenceUsing(
	ctx context.Context,
	input string,
	payload ffprobePayload,
	bounded bool,
	inputOptions []string,
	run func(context.Context, []string) (analysisCommandOutput, error),
) (bool, string) {
	if !payloadHasVideoStream(payload) {
		return false, ""
	}
	duration, _ := strconv.ParseFloat(strings.TrimSpace(payload.Format.Duration), 64)
	targetLimit := boundedSeekProbeTargetLimit
	if !bounded {
		targetLimit = int(^uint(0) >> 1)
	}
	targets, complete := exactSeekProbeTargets(duration, hlsSegmentSeconds, targetLimit)
	if len(targets) == 0 {
		return false, ""
	}
	probeCtx := ctx
	cancel := func() {}
	if bounded {
		probeCtx, cancel = context.WithTimeout(ctx, boundedSeekProbeTimeout)
	} else {
		probeCtx, cancel = context.WithTimeout(ctx, 2*time.Minute)
	}
	defer cancel()
	args := []string{"-v", "error"}
	args = append(args, inputOptions...)
	args = append(args, "-select_streams", "v:0", "-skip_frame", "nokey")
	if bounded {
		args = append(args, "-read_intervals", exactSeekReadIntervals(targets, boundedSeekProbeWindow))
	}
	args = append(args, "-show_frames", "-show_entries", "frame=best_effort_timestamp_time", "-print_format", "json", input)
	result, err := run(probeCtx, args)
	if err != nil {
		return false, ""
	}
	var keyframes ffprobeKeyframePayload
	if json.Unmarshal(result.Stdout, &keyframes) != nil {
		return false, ""
	}
	times := keyframeTimes(keyframes)
	if !keyframesCoverExactSeekTargets(times, targets, exactSeekBoundaryTolerance) {
		return false, time.Now().UTC().Format(time.RFC3339)
	}
	if complete {
		return true, time.Now().UTC().Format(time.RFC3339)
	}
	return false, ""
}

func exactSeekProbeTargets(duration float64, segmentSeconds, limit int) ([]float64, bool) {
	if duration <= 0 || segmentSeconds <= 0 || limit <= 0 {
		return nil, false
	}
	all := []float64{0}
	for boundary := float64(segmentSeconds); boundary < duration-exactSeekBoundaryTolerance; boundary += float64(segmentSeconds) {
		all = append(all, boundary)
	}
	if len(all) <= limit {
		return all, true
	}
	selected := make([]float64, 0, limit)
	lastIndex := -1
	for index := 0; index < limit; index++ {
		position := int(math.Round(float64(index) * float64(len(all)-1) / float64(limit-1)))
		if position != lastIndex {
			selected = append(selected, all[position])
			lastIndex = position
		}
	}
	return selected, false
}

func exactSeekReadIntervals(targets []float64, window float64) string {
	if window <= 0 {
		window = boundedSeekProbeWindow
	}
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		start := math.Max(0, target-window)
		parts = append(parts, strconv.FormatFloat(start, 'f', 3, 64)+"%+"+strconv.FormatFloat(window*2, 'f', 3, 64))
	}
	return strings.Join(parts, ",")
}

func keyframeTimes(payload ffprobeKeyframePayload) []float64 {
	times := make([]float64, 0, len(payload.Frames))
	for _, frame := range payload.Frames {
		value, err := strconv.ParseFloat(strings.TrimSpace(frame.BestEffortTimestampTime), 64)
		if err == nil && value >= 0 {
			times = append(times, value)
		}
	}
	sort.Float64s(times)
	return times
}

func keyframesCoverExactSeekTargets(times, targets []float64, tolerance float64) bool {
	if len(times) == 0 || len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		index := sort.SearchFloat64s(times, target-tolerance)
		if index >= len(times) || times[index] > target+tolerance {
			return false
		}
	}
	return true
}
