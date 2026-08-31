package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSegmentDetectorRequiresContentEvidenceAndKeepsCandidatesAdvisory(t *testing.T) {
	chapters := []Chapter{
		{Title: "Previously On", StartSeconds: 0, EndSeconds: 45},
		{Title: "Opening Credits", StartSeconds: 45, EndSeconds: 90},
		{Title: "Commercial Break", StartSeconds: 900, EndSeconds: 1020},
		{Title: "End Credits", StartSeconds: 3300, EndSeconds: 3600},
	}
	if candidates := analyzedSegmentsFromSignals(chapters, 3600, segmentSignalEvidence{}); len(candidates) != 0 {
		t.Fatalf("chapter labels created markers without content evidence: %#v", candidates)
	}
	evidence := segmentSignalEvidence{
		Black:   []analysisSignalInterval{{Start: 0, End: 1}, {Start: 44.5, End: 45.5}, {Start: 89.5, End: 90.5}, {Start: 899, End: 901}, {Start: 1019, End: 1021}, {Start: 3598, End: 3600}},
		Silence: []analysisSignalInterval{{Start: 0, End: 1}, {Start: 44.5, End: 45.5}, {Start: 89.5, End: 90.5}, {Start: 899, End: 901}, {Start: 1019, End: 1021}, {Start: 3598, End: 3600}},
	}
	candidates := analyzedSegmentsFromSignals(chapters, 3600, evidence)
	if len(candidates) != 4 {
		t.Fatalf("content-supported candidates = %#v", candidates)
	}
	want := []string{"recap", "intro", "commercial", "credits"}
	for index, candidate := range candidates {
		if candidate.Type != want[index] {
			t.Fatalf("candidate %d type=%q want=%q", index, candidate.Type, want[index])
		}
		if candidate.Confidence <= 0 || candidate.Confidence >= 1 {
			t.Fatalf("candidate confidence must remain advisory: %#v", candidate)
		}
	}
}

func TestSegmentSignalParserRequiresMeasuredIntervals(t *testing.T) {
	parsed := parseSegmentSignalEvidence([]byte(strings.Join([]string{
		"[blackdetect] black_start:0 black_end:1.25 black_duration:1.25",
		"[silencedetect] silence_start: 0",
		"[silencedetect] silence_end: 1.1 | silence_duration: 1.1",
		"chapter title: Opening Credits",
	}, "\n")))
	if len(parsed.Black) != 1 || len(parsed.Silence) != 1 {
		t.Fatalf("signal evidence was not parsed: %#v", parsed)
	}
	if parsed.Black[0].End != 1.25 || parsed.Silence[0].End != 1.1 {
		t.Fatalf("signal boundaries were altered: %#v", parsed)
	}
}

func TestBoundedSeekValidationCapsTargetsAndDoesNotPromotePartialSuccess(t *testing.T) {
	payload := ffprobePayload{
		Format:  ffprobeFormat{Duration: "3600"},
		Streams: []ffprobeStream{{CodecType: "video"}},
	}
	targets, complete := exactSeekProbeTargets(3600, hlsSegmentSeconds, boundedSeekProbeTargetLimit)
	if complete || len(targets) != boundedSeekProbeTargetLimit {
		t.Fatalf("bounded targets complete=%v count=%d", complete, len(targets))
	}
	run := func(_ context.Context, args []string) (analysisCommandOutput, error) {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "-read_intervals") {
			t.Fatalf("bounded seek probe did not use targeted intervals: %s", joined)
		}
		frames := make([]string, 0, len(targets))
		for _, target := range targets {
			frames = append(frames, fmt.Sprintf(`{"best_effort_timestamp_time":"%.3f"}`, target))
		}
		return analysisCommandOutput{Stdout: []byte(`{"frames":[` + strings.Join(frames, ",") + `]}`)}, nil
	}
	safe, observedAt := probeExactSeekEvidenceUsing(context.Background(), "fixture.mp4", payload, true, []string{"-protocol_whitelist", "file,pipe"}, run)
	if safe || observedAt != "" {
		t.Fatalf("partial positive samples were promoted to whole-file truth: safe=%v observedAt=%q", safe, observedAt)
	}

	missing := func(_ context.Context, _ []string) (analysisCommandOutput, error) {
		return analysisCommandOutput{Stdout: []byte(`{"frames":[{"best_effort_timestamp_time":"0"}]}`)}, nil
	}
	safe, observedAt = probeExactSeekEvidenceUsing(context.Background(), "fixture.mp4", payload, true, []string{"-protocol_whitelist", "file,pipe"}, missing)
	if safe || observedAt == "" {
		t.Fatalf("sampled seek failure was not retained as conclusive negative evidence: safe=%v observedAt=%q", safe, observedAt)
	}
}

func TestBoundedSeekValidationCanProveShortCompleteGrid(t *testing.T) {
	payload := ffprobePayload{Format: ffprobeFormat{Duration: "18"}, Streams: []ffprobeStream{{CodecType: "video"}}}
	run := func(_ context.Context, _ []string) (analysisCommandOutput, error) {
		return analysisCommandOutput{Stdout: []byte(`{"frames":[` +
			`{"best_effort_timestamp_time":"0"},` +
			`{"best_effort_timestamp_time":"4"},` +
			`{"best_effort_timestamp_time":"8"},` +
			`{"best_effort_timestamp_time":"12"},` +
			`{"best_effort_timestamp_time":"16"}]}`)}, nil
	}
	safe, observedAt := probeExactSeekEvidenceUsing(context.Background(), "short.mp4", payload, true, []string{"-protocol_whitelist", "file,pipe"}, run)
	if !safe || observedAt == "" {
		t.Fatalf("complete bounded grid was not published: safe=%v observedAt=%q", safe, observedAt)
	}
}

func TestAnalysisOptionsKeepSeekBoundedAndSegmentDetectionExplicit(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	custom, err := server.createLibrary(CreateLibraryRequest{
		Name: "Custom", Type: "movie", Paths: []string{root},
		Settings: map[string]any{
			"analysisTier": analysisTierCustom, "probeStreams": true, "readEmbeddedIndexes": true,
			"validateSeekBehavior": true, "detectSegments": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := MediaItem{ID: "custom-seek-segments", Type: "movie", LibraryID: custom.ID}
	probe := server.mediaAnalysisOptions(item, mediaAnalysisModeProbe)
	if !probe.ValidateSeekBehavior || probe.FullSeekValidation || probe.DetectSegments {
		t.Fatalf("Custom probe did not isolate bounded seek work: %#v", probe)
	}
	full := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	if !full.ValidateSeekBehavior || full.FullSeekValidation || !full.DetectSegments {
		t.Fatalf("Custom full pass widened seek authority or omitted selected detection: %#v", full)
	}
	if !server.analysisTierWantsFull(item) {
		t.Fatal("selected segment detection did not queue a full analysis stage")
	}

	complete, err := server.createLibrary(CreateLibraryRequest{Name: "Complete", Type: "movie", Paths: []string{root}, Settings: map[string]any{"analysisTier": analysisTierComplete}})
	if err != nil {
		t.Fatal(err)
	}
	completeOptions := server.mediaAnalysisOptions(MediaItem{ID: "complete-seek", Type: "movie", LibraryID: complete.ID}, mediaAnalysisModeFull)
	if !completeOptions.ValidateSeekBehavior || !completeOptions.FullSeekValidation || !completeOptions.DetectSegments {
		t.Fatalf("Complete omitted full seek/segment stages: %#v", completeOptions)
	}
}

func TestSegmentPublicationIsRevisionFencedAndPreservesOtherOwners(t *testing.T) {
	server, item, sourcePath, file := newSegmentAnalysisFixture(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`INSERT INTO media_segments
		(id,media_id,segment_type,start_seconds,end_seconds,automatic_safe,source,provider,confidence,created_at) VALUES
		('manual-segment',?,'intro',0,15,1,'manual','editor',1,?),
		('provider-segment',?,'credits',3300,3600,1,'provider','trusted-provider',0.99,?),
		('old-analysis-segment',?,'intro',0,30,0,'generated',?,0.5,?)`,
		item.ID, now, item.ID, now, item.ID, segmentDetectorProvider, now); err != nil {
		t.Fatal(err)
	}
	candidate := analyzedSegmentCandidate{Type: "recap", StartSeconds: 0, EndSeconds: 45, Confidence: 0.86}
	if err := server.publishSegmentAnalysis(context.Background(), item, sourcePath, file, nil, segmentSignalEvidence{}, []analyzedSegmentCandidate{candidate}); err != nil {
		t.Fatal(err)
	}
	rows, err := server.db.Query(`SELECT id,automatic_safe FROM media_segments WHERE media_id=? ORDER BY id`, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]int{}
	for rows.Next() {
		var id string
		var safe int
		if err := rows.Scan(&id, &safe); err != nil {
			t.Fatal(err)
		}
		seen[id] = safe
	}
	if _, ok := seen["manual-segment"]; !ok {
		t.Fatal("manual segment was deleted by analysis publication")
	}
	if _, ok := seen["provider-segment"]; !ok {
		t.Fatal("provider segment was deleted by analysis publication")
	}
	if _, ok := seen["old-analysis-segment"]; ok {
		t.Fatal("superseded analysis-owned segment survived replacement")
	}
	for id, safe := range seen {
		if id != "manual-segment" && id != "provider-segment" && safe != 0 {
			t.Fatalf("analysis candidate %q was incorrectly marked automatic-safe", id)
		}
	}

	if _, err := server.db.Exec(`UPDATE media_files SET mod_time='changed-revision' WHERE id=?`, file.ID); err != nil {
		t.Fatal(err)
	}
	err = server.publishSegmentAnalysis(context.Background(), item, sourcePath, file, nil, segmentSignalEvidence{}, nil)
	if !errors.Is(err, errSegmentAnalysisSourceChanged) {
		t.Fatalf("stale publication error=%v", err)
	}
}

func TestSegmentNoFindingsAreDurableAndPolicyIsCheckedBeforeOpenAndPublish(t *testing.T) {
	server, item, sourcePath, file := newSegmentAnalysisFixture(t)
	if err := server.publishSegmentAnalysis(context.Background(), item, sourcePath, file, nil, segmentSignalEvidence{}, nil); err != nil {
		t.Fatal(err)
	}
	if !server.segmentAnalysisAlreadyCurrent(context.Background(), item.ID, file) {
		t.Fatal("no-findings result was not durable/idempotent for the exact source revision")
	}
	var count int
	if err := server.db.QueryRow(`SELECT finding_count FROM media_segment_analysis_runs WHERE media_id=? AND media_file_id=?`, item.ID, file.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("no-findings evidence count=%d err=%v", count, err)
	}

	if _, err := server.db.Exec(`UPDATE libraries SET settings_json='{"analysisTier":"custom","probeStreams":true,"readEmbeddedIndexes":true,"detectSegments":false}' WHERE id=?`, item.LibraryID); err != nil {
		t.Fatal(err)
	}
	server.cfg.FFmpegPath = filepath.Join(t.TempDir(), "ffmpeg-must-not-open")
	payload := ffprobePayload{Format: ffprobeFormat{Duration: "3600"}, Streams: []ffprobeStream{{CodecType: "video"}}}
	if err := server.detectMediaSegments(context.Background(), item, sourcePath, sourcePath, payload, nil, file); err != nil {
		t.Fatalf("disabled operation reached the content-open boundary: %v", err)
	}
	if err := server.publishSegmentAnalysis(context.Background(), item, sourcePath, file, nil, segmentSignalEvidence{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("disabled operation published after the policy fence: %v", err)
	}
}

func TestUnknownBoundedSeekResultPreservesConclusiveCanonicalEvidence(t *testing.T) {
	server, item, sourcePath, _ := newSegmentAnalysisFixture(t)
	positiveAt := "2026-08-30T12:00:00Z"
	payload := ffprobePayload{
		Format: ffprobeFormat{Duration: "18", FormatName: "mov,mp4"},
		Streams: []ffprobeStream{{
			Index: 0, CodecType: "video", CodecName: "h264", Width: 1920, Height: 1080,
			PixelFormat: "yuv420p", AverageFrameRate: "24/1", AspectRatio: "16:9",
		}},
	}
	revision, err := server.currentMediaAnalysisSourceRevision(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	options := mediaAnalysisOptions{Mode: mediaAnalysisModeProbe, ProbeStreams: true, ValidateSeekBehavior: true, ExpectedSourceRevision: revision}
	if err := server.persistFFprobeAnalysis(context.Background(), item, sourcePath, payload, options, true, positiveAt); err != nil {
		t.Fatal(err)
	}
	// A clean bounded sample on a long file is unknown, encoded as no evidence
	// timestamp. It must not erase the conclusive same-revision fact.
	if err := server.persistFFprobeAnalysis(context.Background(), item, sourcePath, payload, options, false, ""); err != nil {
		t.Fatal(err)
	}
	var safe int
	var persistedAt string
	if err := server.db.QueryRow(`SELECT exact_seek_safe,keyframe_evidence_at FROM media_streams WHERE media_id=? AND kind='video'`, item.ID).Scan(&safe, &persistedAt); err != nil {
		t.Fatal(err)
	}
	if safe != 1 || persistedAt != positiveAt {
		t.Fatalf("unknown sample erased positive stream truth safe=%d at=%q", safe, persistedAt)
	}
	var factsJSON string
	if err := server.db.QueryRow(`SELECT facts_json FROM media_analysis_facts WHERE media_id=?`, item.ID).Scan(&factsJSON); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(factsJSON, positiveAt) {
		t.Fatalf("canonical facts did not reuse persisted seek evidence: %s", factsJSON)
	}

	negativeAt := "2026-08-30T13:00:00Z"
	if err := server.persistFFprobeAnalysis(context.Background(), item, sourcePath, payload, options, false, negativeAt); err != nil {
		t.Fatal(err)
	}
	if err := server.persistFFprobeAnalysis(context.Background(), item, sourcePath, payload, options, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT exact_seek_safe,keyframe_evidence_at FROM media_streams WHERE media_id=? AND kind='video'`, item.ID).Scan(&safe, &persistedAt); err != nil {
		t.Fatal(err)
	}
	if safe != 0 || persistedAt != negativeAt {
		t.Fatalf("unknown sample erased conclusive negative stream truth safe=%d at=%q", safe, persistedAt)
	}
}

func TestFFprobePublicationUsesGenericSourceAndProbePolicyFence(t *testing.T) {
	payload := ffprobePayload{
		Format: ffprobeFormat{Duration: "18", FormatName: "mov,mp4"},
		Streams: []ffprobeStream{{
			Index: 0, CodecType: "video", CodecName: "h264", Width: 1920, Height: 1080,
			PixelFormat: "yuv420p", AverageFrameRate: "24/1", AspectRatio: "16:9",
		}},
	}
	t.Run("stale source", func(t *testing.T) {
		server, item, sourcePath, _ := newSegmentAnalysisFixture(t)
		revision, err := server.currentMediaAnalysisSourceRevision(context.Background(), item)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := server.db.Exec(`UPDATE media_files SET mod_time='2026-08-30T14:00:00Z' WHERE media_id=?`, item.ID); err != nil {
			t.Fatal(err)
		}
		err = server.persistFFprobeAnalysis(context.Background(), item, sourcePath, payload,
			mediaAnalysisOptions{Mode: mediaAnalysisModeProbe, ProbeStreams: true, ExpectedSourceRevision: revision}, false, "")
		if !errors.Is(err, errMediaAnalysisSourceStale) {
			t.Fatalf("stale technical facts publication error=%v", err)
		}
		var duration int
		if err := server.db.QueryRow(`SELECT duration_seconds FROM media_items WHERE id=?`, item.ID).Scan(&duration); err != nil {
			t.Fatal(err)
		}
		if duration != 0 {
			t.Fatalf("stale FFprobe duration published: %d", duration)
		}
	})

	t.Run("probe disabled", func(t *testing.T) {
		server, item, sourcePath, _ := newSegmentAnalysisFixture(t)
		revision, err := server.currentMediaAnalysisSourceRevision(context.Background(), item)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := server.db.Exec(`UPDATE libraries SET settings_json='{"analysisTier":"custom","probeStreams":false}' WHERE id=?`, item.LibraryID); err != nil {
			t.Fatal(err)
		}
		err = server.persistFFprobeAnalysis(context.Background(), item, sourcePath, payload,
			mediaAnalysisOptions{Mode: mediaAnalysisModeProbe, ProbeStreams: true, ExpectedSourceRevision: revision}, false, "")
		if !errors.Is(err, errMediaAnalysisOperationDisabled) {
			t.Fatalf("disabled probe publication error=%v", err)
		}
		var streams int
		if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_streams WHERE media_id=?`, item.ID).Scan(&streams); err != nil {
			t.Fatal(err)
		}
		if streams != 0 {
			t.Fatalf("disabled probe published %d streams", streams)
		}
	})
}

func newSegmentAnalysisFixture(t *testing.T) (*Server, MediaItem, string, analysisFileIdentity) {
	t.Helper()
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{
		Name: "Segment Analysis", Type: "movie", Paths: []string{root},
		Settings: map[string]any{
			"analysisTier": analysisTierCustom, "probeStreams": true,
			"readEmbeddedIndexes": true, "detectSegments": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(root, "feature.mp4")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := MediaItem{ID: "segment-analysis-item", Type: "movie", Title: "Feature", LibraryID: library.ID, SourceURL: sourcePath}
	if _, err := server.db.Exec(`INSERT INTO media_items
		(id,library_id,type,title,sort_title,genres_json,source_url,added_at) VALUES (?,?,?,?,?,'[]',?,?)`,
		item.ID, library.ID, item.Type, item.Title, item.Title, sourcePath, now); err != nil {
		t.Fatal(err)
	}
	file := canonicalAnalysisFileIdentity("segment-analysis-file", "fixture-fingerprint", 4096, now)
	if _, err := server.db.Exec(`INSERT INTO media_files
		(id,media_id,library_id,path,source_type,size_bytes,mod_time,content_fingerprint,available,first_seen_at,last_seen_at)
		VALUES (?,?,?,?, 'local',?,?,?,?,?,?)`,
		file.ID, item.ID, library.ID, sourcePath, file.SizeBytes, file.ModTime, file.Fingerprint, 1, now, now); err != nil {
		t.Fatal(err)
	}
	return server, item, sourcePath, file
}
